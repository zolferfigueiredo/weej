package main

import (
	"bytes"
	"encoding/binary"
	"image"
	"image/png"
	"io"
)

// image/... has no ICO encoder, so the ICONDIR/ICONDIRENTRY headers are written by hand; PNG-
// compressed entries are valid since Vista.
func encodeICO(w io.Writer, sizes []int, iconAt func(size int) image.Image) error {
	pngs := make([][]byte, len(sizes))
	for i, size := range sizes {
		var buf bytes.Buffer
		if err := png.Encode(&buf, iconAt(size)); err != nil {
			return err
		}
		pngs[i] = buf.Bytes()
	}

	var out bytes.Buffer
	binary.Write(&out, binary.LittleEndian, uint16(0))
	binary.Write(&out, binary.LittleEndian, uint16(1))
	binary.Write(&out, binary.LittleEndian, uint16(len(sizes)))

	offset := uint32(6 + 16*len(sizes))
	for i, size := range sizes {
		d := icoDim(size)
		out.WriteByte(d)
		out.WriteByte(d)
		out.WriteByte(0)
		out.WriteByte(0)
		binary.Write(&out, binary.LittleEndian, uint16(1))
		binary.Write(&out, binary.LittleEndian, uint16(32))
		binary.Write(&out, binary.LittleEndian, uint32(len(pngs[i])))
		binary.Write(&out, binary.LittleEndian, offset)
		offset += uint32(len(pngs[i]))
	}
	for _, p := range pngs {
		out.Write(p)
	}

	_, err := w.Write(out.Bytes())
	return err
}

// The ICO format encodes a dimension of 256 as 0, since the field is a single byte.
func icoDim(size int) byte {
	if size >= 256 {
		return 0
	}
	return byte(size)
}
