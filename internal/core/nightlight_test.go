package core

import (
	"bytes"
	"encoding/hex"
	"strings"
	"testing"
	"time"
)

func mustHex(t *testing.T, s string) []byte {
	t.Helper()
	b, err := hex.DecodeString(strings.ReplaceAll(s, " ", ""))
	if err != nil {
		t.Fatal(err)
	}
	return b
}

var nightLightFixtures = map[string]string{
	"state":             "43 42 01 00 0A 02 01 00 2A 06 EC B9 97 C6 06 2A 2B 0E 13 43 42 01 00 D0 0A 02 C6 14 AB C8 DD 9A A5 9E 89 EE 01 00 00 00 00",
	"settings":          "43 42 01 00 0A 02 01 00 2A 06 8D EF F7 B5 06 2A 2B 0E 15 43 42 01 00 CA 14 0E 15 00 CA 1E 0E 07 00 CA 32 00 CA 3C 00 00 00 00 00",
	"stateperdevice":    "43 42 01 00 0A 02 01 00 2A 06 81 91 AF C6 06 2A 2B 0E 14 43 42 01 00 D0 0A 02 C6 14 AB C8 DD 9A A5 9E 89 EE 01 01 00 00 00 00",
	"settingsperdevice": "43 42 01 00 0A 02 01 00 2A 06 FF 90 AF C6 06 2A 2B 0E 16 43 42 01 00 CA 14 0E 15 00 CA 1E 0E 07 00 CA 32 00 CA 3C 00 01 00 00 00 00",
}

func TestNightLightFixturesRoundTrip(t *testing.T) {
	for name, hexStr := range nightLightFixtures {
		blob := mustHex(t, hexStr)
		fields, err := parseOuter(blob)
		if err != nil {
			t.Fatalf("%s: parseOuter: %v", name, err)
		}

		var rebuiltFields []byte
		if strings.HasPrefix(name, "state") {
			s, err := parseStateFields(fields)
			if err != nil {
				t.Fatalf("%s: parseStateFields: %v", name, err)
			}
			if s.on {
				t.Errorf("%s: want off (no capture had night light on)", name)
			}
			rebuiltFields = buildStateFields(s)
		} else {
			s, err := parseSettingsFields(fields)
			if err != nil {
				t.Fatalf("%s: parseSettingsFields: %v", name, err)
			}
			if s.kelvin != 0 {
				t.Errorf("%s: want kelvin unset, got %d", name, s.kelvin)
			}
			rebuiltFields = buildSettingsFields(s)
		}
		if !bytes.Equal(rebuiltFields, fields) {
			t.Errorf("%s: rebuilt fields = %x, want %x", name, rebuiltFields, fields)
		}

		// Rebuild the whole blob with the timestamp the fixture itself carried.
		ts, _, err := decodeLEB128(blob[10:])
		if err != nil {
			t.Fatalf("%s: decode timestamp: %v", name, err)
		}
		rebuilt := buildOuter(fields, time.Unix(int64(ts), 0))
		if !bytes.Equal(rebuilt, blob) {
			t.Errorf("%s: rebuilt blob =\n%X\nwant\n%X", name, rebuilt, blob)
		}

		wantPerDevice := strings.HasSuffix(name, "perdevice")
		var gotPerDevice bool
		if strings.HasPrefix(name, "state") {
			s, _ := parseStateFields(fields)
			gotPerDevice = s.perDevice
		} else {
			s, _ := parseSettingsFields(fields)
			gotPerDevice = s.perDevice
		}
		if gotPerDevice != wantPerDevice {
			t.Errorf("%s: perDevice = %v, want %v", name, gotPerDevice, wantPerDevice)
		}
	}
}

func TestSetOnThenOffRestoresTheStructure(t *testing.T) {
	blob := mustHex(t, nightLightFixtures["state"])
	now := time.Now()

	on, err := SetOn(blob, true, now)
	if err != nil {
		t.Fatal(err)
	}
	isOn, err := IsOn(on)
	if err != nil {
		t.Fatal(err)
	}
	if !isOn {
		t.Fatal("want on after SetOn(true)")
	}

	off, err := SetOn(on, false, now)
	if err != nil {
		t.Fatal(err)
	}
	isOn, err = IsOn(off)
	if err != nil {
		t.Fatal(err)
	}
	if isOn {
		t.Fatal("want off after SetOn(false)")
	}

	originalFields, err := parseOuter(blob)
	if err != nil {
		t.Fatal(err)
	}
	roundTrippedFields, err := parseOuter(off)
	if err != nil {
		t.Fatal(err)
	}
	if !bytes.Equal(originalFields, roundTrippedFields) {
		t.Errorf("fields after on-then-off = %x, want %x", roundTrippedFields, originalFields)
	}
}

func TestSetKelvinClampsAndRoundTrips(t *testing.T) {
	blob := mustHex(t, nightLightFixtures["settings"])
	now := time.Now()

	updated, err := SetKelvin(blob, 3000, now)
	if err != nil {
		t.Fatal(err)
	}
	k, err := Kelvin(updated)
	if err != nil {
		t.Fatal(err)
	}
	if k != 3000 {
		t.Errorf("Kelvin() = %d, want 3000", k)
	}

	tooLow, err := SetKelvin(blob, 100, now)
	if err != nil {
		t.Fatal(err)
	}
	if k, _ := Kelvin(tooLow); k != 1200 {
		t.Errorf("Kelvin() = %d, want clamped to 1200", k)
	}

	tooHigh, err := SetKelvin(blob, 9000, now)
	if err != nil {
		t.Fatal(err)
	}
	if k, _ := Kelvin(tooHigh); k != 6500 {
		t.Errorf("Kelvin() = %d, want clamped to 6500", k)
	}
}

func TestNightLightUnknownHeaderErrors(t *testing.T) {
	blob := mustHex(t, nightLightFixtures["state"])
	blob[0] = 0xFF
	if _, err := IsOn(blob); err == nil {
		t.Error("want error for a corrupted header byte")
	}
	if _, err := SetOn(blob, true, time.Now()); err == nil {
		t.Error("want error for a corrupted header byte")
	}
}

func TestNightLightRejectsSettingsShapeForState(t *testing.T) {
	blob := mustHex(t, nightLightFixtures["settings"])
	if _, err := IsOn(blob); err == nil {
		t.Error("want error: a settings blob is not a state blob")
	}
}
