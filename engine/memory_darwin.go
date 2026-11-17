package engine

import "golang.org/x/sys/unix"

// systemMemory is hw.memsize, how much memory the Mac has, in bytes, or
// zero when it will not say.
func systemMemory() int64 {
	n, err := unix.SysctlUint64("hw.memsize")
	if err != nil {
		return 0
	}
	return int64(n)
}
