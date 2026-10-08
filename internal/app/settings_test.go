//go:build windows

package app

import "testing"

func TestProbeMessageTakesABoardOrItsID(t *testing.T) {
	whole, ok := probeMessage([]byte(`{"type":"setDevice","device":{"id":"d6","name":"Arduino","nextProfile":{"vk":50,"mods":5,"key":"2"}}}`))
	if !ok || whole.Type != "setDevice" || whole.Device != "" {
		t.Errorf("a whole board = %+v, %v, want setDevice kept", whole, ok)
	}
	id, ok := probeMessage([]byte(`{"type":"reconnect","device":"d1"}`))
	if !ok || id.Type != "reconnect" || id.Device != "d1" {
		t.Errorf("a board's id = %+v, %v", id, ok)
	}
	if _, ok := probeMessage([]byte(`not json`)); ok {
		t.Error("a broken message was taken")
	}
}
