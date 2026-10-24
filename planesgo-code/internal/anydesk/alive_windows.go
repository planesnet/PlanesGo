//go:build windows

package anydesk

// Alive: in Windows it is not checked (the sessions end through the traces).
func Alive(int) bool { return true }
