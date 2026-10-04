package core

import (
	"reflect"
	"testing"
)

func TestCandidatePortsOrder(t *testing.T) {
	ports := []PortInfo{
		{Name: "COM10", IsUSB: true, VID: "1A86"},
		{Name: "COM3", IsUSB: false, VID: "0000"}, // Bluetooth SPP: never a candidate
		{Name: "COM1", IsUSB: false, VID: "0000"}, // legacy COM1: never a candidate
		{Name: "COM6", IsUSB: true, VID: "1A86"},
		{Name: "COM7", IsUSB: true, VID: "0403"},
		{Name: "COM5", IsUSB: true, VID: "2341"},
		{Name: "COM9", IsUSB: true, VID: "9999"},
	}
	got := CandidatePorts(ports)
	want := []string{"COM5", "COM6", "COM10", "COM7", "COM9"}
	if !reflect.DeepEqual(got, want) {
		t.Errorf("CandidatePorts() = %v, want %v", got, want)
	}
}
