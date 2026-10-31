//go:build windows

package fileio

// acrossProcesses keeps no lock across programs on Windows yet, only the
// one inside each program. Windows builds are not shipped, and the lock
// there wants LockFileEx rather than flock.
func acrossProcesses(string, func(string)) (release func()) { return func() {} }
