package core

import (
	"reflect"
	"testing"
)

func TestSerialLines(t *testing.T) {
	got, ok := ParseLine("7|1023|0\r")
	if !ok || !reflect.DeepEqual(got, []int{7, 1023, 0}) {
		t.Errorf("ParseLine(%q) = %v, %v", "7|1023|0\r", got, ok)
	}
	if _, ok := ParseLine("7||0"); ok {
		t.Error("ParseLine(7||0) should reject an empty field")
	}
	if _, ok := ParseLine("1024"); ok {
		t.Error("ParseLine(1024) should reject an out-of-range field")
	}
}

func TestLineReaderWidthRule(t *testing.T) {
	r := NewLineReader()
	out := r.Feed([]byte("1|2|3\n1|2|3\n"))
	if len(out) != 1 || !reflect.DeepEqual(out[0], []int{1, 2, 3}) {
		t.Errorf("Feed = %v, want one accepted line", out)
	}
}

func TestLineReaderRejectsAWidthChange(t *testing.T) {
	r := NewLineReader()
	r.Feed([]byte("1|2|3\n1|2|3\n"))
	out := r.Feed([]byte("1|2\n1|2|3\n"))
	if len(out) != 0 {
		t.Errorf("Feed after a width change = %v, want none accepted", out)
	}
	out = r.Feed([]byte("1|2|3\n"))
	if len(out) != 1 {
		t.Errorf("Feed after resync = %v, want one accepted line", out)
	}
}

func TestLineReaderUnparseableLineLeavesWidthUnchanged(t *testing.T) {
	r := NewLineReader()
	r.Feed([]byte("1|2|3\n1|2|3\n"))
	out := r.Feed([]byte("garbage\n1|2|3\n"))
	if len(out) != 1 || !reflect.DeepEqual(out[0], []int{1, 2, 3}) {
		t.Errorf("Feed = %v, want the good line still accepted", out)
	}
}

func TestLineReaderResyncsAfterTooManyBytesWithNoNewline(t *testing.T) {
	r := NewLineReader()
	r.Feed([]byte("1|2|3\n1|2|3\n"))
	junk := make([]byte, 2000)
	for i := range junk {
		junk[i] = 'x'
	}
	out := r.Feed(junk)
	if len(out) != 0 {
		t.Errorf("Feed with no newline = %v, want none accepted", out)
	}
	out = r.Feed([]byte("1|2|3\n"))
	if len(out) != 1 {
		t.Errorf("Feed after overflow = %v, want the old width still honored", out)
	}
}

func TestLineReaderReset(t *testing.T) {
	r := NewLineReader()
	r.Feed([]byte("1|2|3\n1|2|3\n"))
	r.Reset()
	out := r.Feed([]byte("1|2|3\n"))
	if len(out) != 0 {
		t.Errorf("Feed right after Reset = %v, want none accepted", out)
	}
}

func TestLineReaderEmptyFeedIsANoOp(t *testing.T) {
	r := NewLineReader()
	if out := r.Feed(nil); out != nil {
		t.Errorf("Feed(nil) = %v, want nil", out)
	}
	if out := r.Feed([]byte{}); out != nil {
		t.Errorf("Feed([]byte{}) = %v, want nil", out)
	}
	out := r.Feed([]byte("1|2|3\n1|2|3\n"))
	if len(out) != 1 {
		t.Errorf("Feed after empty feeds = %v, want normal operation", out)
	}
}
