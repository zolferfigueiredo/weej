//go:build windows && 386

package sys

// PointArgs passes a POINT by value, as MonitorFromPoint takes it. The 32-bit calling
// convention pushes X and Y on the stack as two arguments.
func PointArgs(x, y int32) []uintptr {
	return []uintptr{uintptr(uint32(x)), uintptr(uint32(y))}
}
