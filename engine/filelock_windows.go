//go:build windows

package engine

// lockAcrossProcesses keeps no lock across programs on Windows yet, only
// the one inside each program. Windows builds are not shipped, and the
// lock there wants LockFileEx rather than flock.
func lockAcrossProcesses(string) (release func()) { return func() {} }
