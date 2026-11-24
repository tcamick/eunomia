package backup

import "syscall"

// flush writes every filesystem's dirty pages to disk (sync(2)); on Linux it
// cannot fail.
func flush() error {
	syscall.Sync()
	return nil
}
