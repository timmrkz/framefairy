//go:build !darwin

package engine

import "runtime"

// systemMemory is MemTotal on Linux, and zero on a system that will not
// say, see MachineMemory.
func systemMemory() int64 {
	if runtime.GOOS == "linux" {
		return memTotal("/proc/meminfo")
	}
	return 0
}

// systemFreeMemory is MemAvailable on Linux, and zero on a system that
// will not say, see FreeMemory.
func systemFreeMemory() int64 {
	if runtime.GOOS == "linux" {
		return meminfo("/proc/meminfo", "MemAvailable:")
	}
	return 0
}
