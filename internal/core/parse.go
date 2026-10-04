package core

import (
	"bytes"
	"strconv"
	"strings"
	"unicode/utf8"
)

func ParseLine(line string) ([]int, bool) {
	fields := strings.Split(line, "|")
	values := make([]int, 0, len(fields))
	for _, f := range fields {
		n, err := strconv.Atoi(strings.TrimSpace(f))
		if err != nil || n < 0 || n > 1023 {
			return nil, false
		}
		values = append(values, n)
	}
	return values, true
}

// width starts at -1 so the first parseable line after Reset can never match it: TheeJ never
// trusts a line until the one after it confirms the same field count.
type LineReader struct {
	buf   []byte
	width int
}

func NewLineReader() *LineReader { return &LineReader{width: -1} }

func (r *LineReader) Reset() {
	r.buf = nil
	r.width = -1
}

func (r *LineReader) Feed(chunk []byte) [][]int {
	if len(chunk) == 0 {
		return nil
	}
	r.buf = append(r.buf, chunk...)

	var out [][]int
	for {
		idx := bytes.IndexByte(r.buf, '\n')
		if idx < 0 {
			break
		}
		line := r.buf[:idx]
		r.buf = r.buf[idx+1:]
		if utf8.Valid(line) {
			if values, ok := ParseLine(string(line)); ok {
				if len(values) == r.width {
					out = append(out, values)
				}
				r.width = len(values)
			}
		}
	}
	if len(r.buf) > 1024 {
		r.buf = nil
	}
	return out
}
