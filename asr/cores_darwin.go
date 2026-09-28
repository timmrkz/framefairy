//go:build darwin

package asr

import "syscall"

// fastCores is how many performance cores the Mac has. Apple's chips have
// performance cores and efficiency cores, and the speech model is heard as
// fast as the performance cores go: on an M2 Max, 8 of its 12, threads past
// 8 made it slower. Zero when macOS does not say, on a Mac with one kind.
func fastCores() int {
	n, err := syscall.SysctlUint32("hw.perflevel0.physicalcpu")
	if err != nil {
		return 0
	}
	return int(n)
}
