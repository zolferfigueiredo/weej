//go:build windows && (amd64 || arm64)

package sys

// PointArgs passes a POINT by value, as MonitorFromPoint takes it. The 64-bit calling
// convention packs the 8-byte struct into a single register.
func PointArgs(x, y int32) []uintptr {
	return []uintptr{uintptr(uint32(x)) | uintptr(uint32(y))<<32}
}
