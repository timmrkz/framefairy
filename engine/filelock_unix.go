//go:build !windows

package engine

import (
	"os"
	"path/filepath"
	"syscall"
)

// lockAcrossProcesses holds a lock that every program of ours takes before
// it edits the file at path, and gives it back when release is called. The
// command line and the app can edit one plan or one transcript at the same
// time, and a lock inside one program does not keep the other out: one of
// the two edits would be lost.
//
// The lock is on a file beside it, not on the file itself, because an edit
// replaces the file with another, see replaceFile, and a lock on the one
// replaced would keep nobody out of the new one. The system gives the lock
// back when a program ends, so a crash leaves nothing held. Where the
// system cannot lock, on some network drives, an edit goes ahead without
// it, as it did before.
func lockAcrossProcesses(path string) (release func()) {
	name := filepath.Join(filepath.Dir(path), "."+filepath.Base(path)+".lock")
	f, err := os.OpenFile(name, os.O_RDWR|os.O_CREATE, 0o644)
	if err != nil {
		return func() {}
	}
	if err := syscall.Flock(int(f.Fd()), syscall.LOCK_EX); err != nil {
		f.Close()
		return func() {}
	}
	return func() {
		_ = syscall.Flock(int(f.Fd()), syscall.LOCK_UN)
		f.Close()
	}
}
