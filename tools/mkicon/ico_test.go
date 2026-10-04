package main

import (
	"bytes"
	"encoding/binary"
	"image"
	"testing"
)

func TestEncodeICO(t *testing.T) {
	sizes := []int{16, 32, 256}
	var buf bytes.Buffer
	err := encodeICO(&buf, sizes, func(size int) image.Image {
		return image.NewNRGBA(image.Rect(0, 0, size, size))
	})
	if err != nil {
		t.Fatal(err)
	}

	data := buf.Bytes()
	if len(data) < 6+16*len(sizes) {
		t.Fatalf("ICO too short: %d bytes", len(data))
	}
	if reserved := binary.LittleEndian.Uint16(data[0:2]); reserved != 0 {
		t.Errorf("ICONDIR reserved = %d, want 0", reserved)
	}
	if typ := binary.LittleEndian.Uint16(data[2:4]); typ != 1 {
		t.Errorf("ICONDIR type = %d, want 1 (icon)", typ)
	}
	if count := binary.LittleEndian.Uint16(data[4:6]); int(count) != len(sizes) {
		t.Fatalf("ICONDIR count = %d, want %d", count, len(sizes))
	}

	for i, size := range sizes {
		e := data[6+i*16 : 6+i*16+16]
		wantDim := icoDim(size)
		if e[0] != wantDim || e[1] != wantDim {
			t.Errorf("entry %d: dims = %d,%d want %d", i, e[0], e[1], wantDim)
		}
		bytesInRes := binary.LittleEndian.Uint32(e[8:12])
		offset := binary.LittleEndian.Uint32(e[12:16])
		if offset+bytesInRes > uint32(len(data)) {
			t.Fatalf("entry %d: offset %d + size %d exceeds file length %d", i, offset, bytesInRes, len(data))
		}
		img := data[offset : offset+bytesInRes]
		if len(img) < 8 || string(img[1:4]) != "PNG" {
			t.Errorf("entry %d: image data is not a PNG", i)
		}
	}
}
