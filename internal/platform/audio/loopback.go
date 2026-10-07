//go:build windows

package audio

import (
	"encoding/binary"
	"fmt"
	"math"
	"runtime"
	"time"
	"unsafe"

	ole "github.com/go-ole/go-ole"
	wca "github.com/moutend/go-wca/pkg/wca"

	"github.com/zolferfigueiredo/weej/internal/core"
)

// Loopback listens to what the default speakers play and hands it to a Sink, such as the
// core.Spectrum behind the button lights' EQ. It follows the default speakers when they change.
type Loopback struct {
	sink Sink
	log  func(string)
	stop chan struct{}
	done chan struct{}
}

// 200 ms of buffer, in REFERENCE_TIME's 100 ns units; it is read every 20 ms.
const loopbackBuffer = 2_000_000

// Sink takes what Loopback hears: the sample rate, then mono samples from -1 to 1.
type Sink interface {
	SetRate(hz float64)
	Add(samples []float32)
}

var _ Sink = (*core.Spectrum)(nil)

func StartLoopback(sink Sink, log func(string)) *Loopback {
	l := &Loopback{sink: sink, log: log, stop: make(chan struct{}), done: make(chan struct{})}
	go l.run()
	return l
}

func (l *Loopback) Stop() {
	close(l.stop)
	<-l.done
}

func (l *Loopback) run() {
	defer close(l.done)
	runtime.LockOSThread()
	defer runtime.UnlockOSThread()
	if err := ole.CoInitializeEx(0, ole.COINIT_MULTITHREADED); err != nil && !benignCoInitError(err) {
		l.log("EQ: could not start COM: " + err.Error())
		return
	}
	defer ole.CoUninitialize()

	var logged string
	for {
		err := l.capture()
		select {
		case <-l.stop:
			return
		default:
		}
		if err != nil && err.Error() != logged {
			logged = err.Error()
			l.log("EQ: could not listen to the speakers: " + logged)
		}
		select {
		case <-l.stop:
			return
		case <-time.After(time.Second):
		}
	}
}

// capture listens until it is stopped, the default speakers change, or something fails.
func (l *Loopback) capture() error {
	var enum *wca.IMMDeviceEnumerator
	if err := wca.CoCreateInstance(wca.CLSID_MMDeviceEnumerator, 0, wca.CLSCTX_ALL, wca.IID_IMMDeviceEnumerator, &enum); err != nil {
		return err
	}
	defer enum.Release()
	id, err := defaultSpeakers(enum)
	if err != nil {
		return err
	}
	var dev *wca.IMMDevice
	if err := enum.GetDefaultAudioEndpoint(wca.ERender, wca.EConsole, &dev); err != nil {
		return err
	}
	defer dev.Release()
	var ac *wca.IAudioClient
	if err := dev.Activate(wca.IID_IAudioClient, wca.CLSCTX_ALL, nil, &ac); err != nil {
		return err
	}
	defer ac.Release()
	var wfx *wca.WAVEFORMATEX
	if err := ac.GetMixFormat(&wfx); err != nil {
		return err
	}
	defer ole.CoTaskMemFree(uintptr(unsafe.Pointer(wfx)))
	float, ok := sampleFormat(wfx)
	if !ok {
		return fmt.Errorf("unexpected format: tag %#x, %d bits", wfx.WFormatTag, wfx.WBitsPerSample)
	}
	if err := initLoopback(ac, wfx); err != nil {
		return err
	}
	var acc *wca.IAudioCaptureClient
	if err := ac.GetService(wca.IID_IAudioCaptureClient, &acc); err != nil {
		return err
	}
	defer acc.Release()
	if err := ac.Start(); err != nil {
		return err
	}
	defer ac.Stop() //nolint:errcheck // stopping on the way out; nothing to do if it fails
	l.sink.SetRate(float64(wfx.NSamplesPerSec))

	read := time.NewTicker(20 * time.Millisecond)
	defer read.Stop()
	check := time.NewTicker(2 * time.Second)
	defer check.Stop()
	channels, align := int(wfx.NChannels), int(wfx.NBlockAlign)
	var mono []float32
	for {
		select {
		case <-l.stop:
			return nil
		case <-check.C:
			if now, err := defaultSpeakers(enum); err != nil || now != id {
				return err
			}
		case <-read.C:
			for {
				var frames, flags uint32
				if err := acc.GetNextPacketSize(&frames); err != nil {
					return err
				}
				if frames == 0 {
					break
				}
				var data *byte
				if err := acc.GetBuffer(&data, &frames, &flags, nil, nil); err != nil {
					return err
				}
				mono = mono[:0]
				if flags&wca.AUDCLNT_BUFFERFLAGS_SILENT != 0 || data == nil {
					mono = append(mono, make([]float32, frames)...)
				} else {
					mono = mixDown(mono, unsafe.Slice(data, int(frames)*align), channels, align, float)
				}
				if err := acc.ReleaseBuffer(frames); err != nil {
					return err
				}
				l.sink.Add(mono)
			}
		}
	}
}

func defaultSpeakers(enum *wca.IMMDeviceEnumerator) (string, error) {
	var dev *wca.IMMDevice
	if err := enum.GetDefaultAudioEndpoint(wca.ERender, wca.EConsole, &dev); err != nil {
		return "", err
	}
	defer dev.Release()
	var id string
	err := dev.GetId(&id)
	return id, err
}

// sampleFormat tells whether the mix format holds 32-bit floats or 16-bit integers, the two a
// shared-mode mix comes in.
func sampleFormat(wfx *wca.WAVEFORMATEX) (float, ok bool) {
	tag := wfx.WFormatTag
	if tag == 0xFFFE && wfx.CbSize >= 22 {
		// WAVEFORMATEXTENSIBLE: its SubFormat GUID starts 24 bytes in, and its first field is the
		// plain format tag.
		sub := (*[4]byte)(unsafe.Add(unsafe.Pointer(wfx), 24))
		tag = uint16(binary.LittleEndian.Uint32(sub[:]))
	}
	switch {
	case tag == 3 && wfx.WBitsPerSample == 32:
		return true, true
	case tag == 1 && wfx.WBitsPerSample == 16:
		return false, true
	}
	return false, false
}

// mixDown appends each frame of data as one sample, the average of its channels.
func mixDown(out []float32, data []byte, channels, align int, float bool) []float32 {
	for f := 0; f+align <= len(data); f += align {
		sum := float32(0)
		for c := range channels {
			if float {
				sum += math.Float32frombits(binary.LittleEndian.Uint32(data[f+4*c:]))
			} else {
				sum += float32(int16(binary.LittleEndian.Uint16(data[f+2*c:]))) / 32768
			}
		}
		out = append(out, sum/float32(channels))
	}
	return out
}
