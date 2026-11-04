//go:build !windows

package fileio

import (
	"os"
	"syscall"
)

// identity is the number the system gives the file, its inode, or nought
// for a file system that keeps none, like the one in memory.
func identity(info os.FileInfo) uint64 {
	if st, ok := info.Sys().(*syscall.Stat_t); ok {
		return st.Ino
	}
	return 0
}
