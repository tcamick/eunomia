//go:build !linux

package backup

import "syscall"

// flush writes every filesystem's dirty pages to disk (sync(2)).
func flush() error {
	return syscall.Sync()
}
