package core

import (
	"bytes"
	"errors"
	"math"
	"time"
)

// Reverse-engineered from four real CloudStore blobs captured on one PC (two "state", two
// "settings", each with and without the per-device marker byte). Everything here is literal:
// no byte outside the on/off flag and the colour temperature field is ever interpreted, only
// matched and copied through, and anything that does not match exactly is an error.
var (
	nlOuterPrefix1 = []byte{0x43, 0x42, 0x01, 0x00}
	nlOuterPrefix2 = []byte{0x0A, 0x02, 0x01, 0x00}
	nlTimeMarker   = []byte{0x2A, 0x06}
	nlInnerMarker  = []byte{0x2A, 0x2B, 0x0E}
	nlInnerHeader  = []byte{0x43, 0x42, 0x01, 0x00}
	nlOuterTrailer = []byte{0x00, 0x00, 0x00}

	// All four captures had this off; the spec's own notes say "on" is this 2-byte field right
	// after the inner header, but no capture confirms its exact placement once set.
	nlOnField = []byte{0x10, 0x00}

	// A state blob's fields after the optional on field: this, then a LEB128 FILETIME that
	// Windows rewrites whenever Night light actually switches, copied through untouched here.
	nlStatePrefix = []byte{0xD0, 0x0A, 0x02, 0xC6, 0x14}

	nlSettingsHead = []byte{0xCA, 0x14, 0x0E, 0x15, 0x00, 0xCA, 0x1E, 0x0E, 0x07, 0x00}
	nlSettingsTail = []byte{0xCA, 0x32, 0x00, 0xCA, 0x3C, 0x00}
	// Not confirmed by any sample either: every capture had colour temperature unset.
	nlKelvinMarker = []byte{0xCF, 0x28}
)

const (
	nlPerDeviceByte byte = 0x01
	nlTerminator    byte = 0x00
)

func encodeLEB128(v uint64) []byte {
	var out []byte
	for {
		b := byte(v & 0x7f)
		v >>= 7
		if v != 0 {
			b |= 0x80
		}
		out = append(out, b)
		if v == 0 {
			return out
		}
	}
}

func decodeLEB128(b []byte) (uint64, int, error) {
	var result uint64
	var shift uint
	for i, v := range b {
		if shift >= 64 {
			return 0, 0, errors.New("core: night light: leb128 overflow")
		}
		result |= uint64(v&0x7f) << shift
		if v&0x80 == 0 {
			return result, i + 1, nil
		}
		shift += 7
	}
	return 0, 0, errors.New("core: night light: leb128 truncated")
}

func parseOuter(blob []byte) ([]byte, error) {
	if len(blob) < 10 || !bytes.Equal(blob[0:4], nlOuterPrefix1) || !bytes.Equal(blob[4:8], nlOuterPrefix2) {
		return nil, errors.New("core: night light: bad header")
	}
	if !bytes.Equal(blob[8:10], nlTimeMarker) {
		return nil, errors.New("core: night light: bad timestamp marker")
	}
	i := 10
	_, n, err := decodeLEB128(blob[i:])
	if err != nil {
		return nil, err
	}
	i += n
	if i+3 > len(blob) || !bytes.Equal(blob[i:i+3], nlInnerMarker) {
		return nil, errors.New("core: night light: bad inner marker")
	}
	i += 3
	if i >= len(blob) {
		return nil, errors.New("core: night light: missing inner length")
	}
	innerLen := int(blob[i])
	i++
	if i+innerLen > len(blob) {
		return nil, errors.New("core: night light: inner length out of range")
	}
	inner := blob[i : i+innerLen]
	i += innerLen
	if !bytes.Equal(blob[i:], nlOuterTrailer) {
		return nil, errors.New("core: night light: bad trailer")
	}
	if len(inner) < 4 || !bytes.Equal(inner[0:4], nlInnerHeader) {
		return nil, errors.New("core: night light: bad inner header")
	}
	return inner[4:], nil
}

func buildOuter(fields []byte, now time.Time) []byte {
	var buf bytes.Buffer
	buf.Write(nlOuterPrefix1)
	buf.Write(nlOuterPrefix2)
	buf.Write(nlTimeMarker)
	buf.Write(encodeLEB128(uint64(now.Unix())))
	buf.Write(nlInnerMarker)
	inner := append(append([]byte{}, nlInnerHeader...), fields...)
	buf.WriteByte(byte(len(inner)))
	buf.Write(inner)
	buf.Write(nlOuterTrailer)
	return buf.Bytes()
}

type nlState struct {
	on         bool
	transition uint64
	perDevice  bool
}

func parseStateFields(fields []byte) (nlState, error) {
	if len(fields) == 0 || fields[len(fields)-1] != nlTerminator {
		return nlState{}, errors.New("core: night light state: missing terminator")
	}
	body := fields[:len(fields)-1]
	on := bytes.HasPrefix(body, nlOnField)
	if on {
		body = body[len(nlOnField):]
	}
	if !bytes.HasPrefix(body, nlStatePrefix) {
		return nlState{}, errors.New("core: night light state: unexpected body")
	}
	transition, n, err := decodeLEB128(body[len(nlStatePrefix):])
	if err != nil {
		return nlState{}, err
	}
	rest := body[len(nlStatePrefix)+n:]
	switch {
	case len(rest) == 0:
		return nlState{on: on, transition: transition, perDevice: false}, nil
	case len(rest) == 1 && rest[0] == nlPerDeviceByte:
		return nlState{on: on, transition: transition, perDevice: true}, nil
	default:
		return nlState{}, errors.New("core: night light state: unexpected tail")
	}
}

func buildStateFields(s nlState) []byte {
	var body []byte
	if s.on {
		body = append(body, nlOnField...)
	}
	body = append(body, nlStatePrefix...)
	body = append(body, encodeLEB128(s.transition)...)
	if s.perDevice {
		body = append(body, nlPerDeviceByte)
	}
	body = append(body, nlTerminator)
	return body
}

type nlSettings struct {
	kelvin    int
	perDevice bool
}

func parseSettingsFields(fields []byte) (nlSettings, error) {
	if len(fields) == 0 || fields[len(fields)-1] != nlTerminator {
		return nlSettings{}, errors.New("core: night light settings: missing terminator")
	}
	body := fields[:len(fields)-1]
	if !bytes.HasPrefix(body, nlSettingsHead) {
		return nlSettings{}, errors.New("core: night light settings: unexpected head")
	}
	rest := body[len(nlSettingsHead):]

	kelvin := 0
	if bytes.HasPrefix(rest, nlKelvinMarker) {
		raw, n, err := decodeLEB128(rest[len(nlKelvinMarker):])
		if err != nil {
			return nlSettings{}, err
		}
		kelvin = int(raw) / 2
		rest = rest[len(nlKelvinMarker)+n:]
	}

	switch {
	case bytes.Equal(rest, nlSettingsTail):
		return nlSettings{kelvin: kelvin, perDevice: false}, nil
	case len(rest) == len(nlSettingsTail)+1 && bytes.Equal(rest[:len(nlSettingsTail)], nlSettingsTail) && rest[len(nlSettingsTail)] == nlPerDeviceByte:
		return nlSettings{kelvin: kelvin, perDevice: true}, nil
	default:
		return nlSettings{}, errors.New("core: night light settings: unexpected tail")
	}
}

func buildSettingsFields(s nlSettings) []byte {
	var body []byte
	body = append(body, nlSettingsHead...)
	if s.kelvin > 0 {
		body = append(body, nlKelvinMarker...)
		body = append(body, encodeLEB128(uint64(s.kelvin*2))...)
	}
	body = append(body, nlSettingsTail...)
	if s.perDevice {
		body = append(body, nlPerDeviceByte)
	}
	body = append(body, nlTerminator)
	return body
}

func clampKelvin(k int) int {
	switch {
	case k < 1200:
		return 1200
	case k > 6500:
		return 6500
	default:
		return k
	}
}

func SetOn(blob []byte, on bool, now time.Time) ([]byte, error) {
	fields, err := parseOuter(blob)
	if err != nil {
		return nil, err
	}
	state, err := parseStateFields(fields)
	if err != nil {
		return nil, err
	}
	state.on = on
	return buildOuter(buildStateFields(state), now), nil
}

func IsOn(blob []byte) (bool, error) {
	fields, err := parseOuter(blob)
	if err != nil {
		return false, err
	}
	state, err := parseStateFields(fields)
	if err != nil {
		return false, err
	}
	return state.on, nil
}

func SetKelvin(blob []byte, k int, now time.Time) ([]byte, error) {
	fields, err := parseOuter(blob)
	if err != nil {
		return nil, err
	}
	settings, err := parseSettingsFields(fields)
	if err != nil {
		return nil, err
	}
	settings.kelvin = clampKelvin(k)
	return buildOuter(buildSettingsFields(settings), now), nil
}

// NightLightStamp is the Unix second a blob was written at. Windows ignores a blob that is
// older than the one it last applied, so every write has to be stamped later than this.
func NightLightStamp(blob []byte) (int64, error) {
	if _, err := parseOuter(blob); err != nil {
		return 0, err
	}
	ts, _, err := decodeLEB128(blob[10:])
	if err != nil {
		return 0, err
	}
	return int64(ts), nil
}

// NightLightKelvin maps a knob position to the colour temperature Windows' strength slider
// uses: 6500 K (barely warm) just above the bottom, 1200 K at the top.
func NightLightKelvin(s float64) int {
	return clampKelvin(6500 - int(math.Round(s*(6500-1200))))
}

func Kelvin(blob []byte) (int, error) {
	fields, err := parseOuter(blob)
	if err != nil {
		return 0, err
	}
	settings, err := parseSettingsFields(fields)
	if err != nil {
		return 0, err
	}
	return settings.kelvin, nil
}
