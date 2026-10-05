//go:build windows && (amd64 || arm64)

package audio

// On 64-bit Windows the 20-byte PROPERTYKEY travels as a pointer, in one argument slot.
func notifierPropertyValueChanged(this, deviceID, key uintptr) uintptr {
	return sOK
}
