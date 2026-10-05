//go:build windows && 386

package audio

// On 32-bit Windows the 20-byte PROPERTYKEY is passed by value: five more 4-byte argument
// slots, which the callback has to take so it pops the right number of bytes off the stack.
func notifierPropertyValueChanged(this, deviceID, key0, key1, key2, key3, key4 uintptr) uintptr {
	return sOK
}
