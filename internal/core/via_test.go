package core

import "testing"

func TestViaBacklightReports(t *testing.T) {
	reports := ViaReports(1)
	if len(reports) != 2 {
		t.Fatalf("len(reports) = %d, want 2", len(reports))
	}
	if got := reports[0][:4]; !bytesEqual(got, []byte{7, 0x80, 255, 0}) {
		t.Errorf("reports[0][:4] = %v, want [7 128 255 0]", got)
	}
	if got := reports[1][:4]; !bytesEqual(got, []byte{7, 3, 1, 255}) {
		t.Errorf("reports[1][:4] = %v, want [7 3 1 255]", got)
	}

	for _, r := range ViaReports(0.1) {
		if len(r) != 32 {
			t.Errorf("len(report) = %d, want 32", len(r))
		}
	}
}

func bytesEqual(a, b []byte) bool {
	if len(a) != len(b) {
		return false
	}
	for i := range a {
		if a[i] != b[i] {
			return false
		}
	}
	return true
}
