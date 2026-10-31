package fileio

import (
	"sync"

	"github.com/spf13/afero"
)

// locks holds a lock for every file an edit has taken one for, by key.
// Edits of one file happen one at a time, a plan's or a transcript's.
// Every edit reads the file, changes it and writes it back, and the app has
// no save button, so an edit that lands while another is being written
// must not be the one that disappears.
var locks sync.Map

// key names a file for its lock: the file system it is in, nil for the
// disk, and where it really is there.
type key struct {
	fsys afero.Fs
	path string
}

// Lock holds the lock of the file at path until the function it returns
// is called. Edits are one at a time inside the program, and one program
// at a time as well, the command line and the app, see acrossProcesses.
// The lock inside the program is taken first, so two edits in one program
// wait for each other without both waiting on the system.
//
// A file in memory has no other program to share it with, so its lock is
// the one inside the program alone, and it is the lock of that memory's
// file: two tests that each hold a plan at the same path in their own
// memory do not wait for each other. Every OsFs is the one disk.
func Lock(fsys afero.Fs, path string) (release func()) {
	return lock(fsys, path, nil)
}

// lock is Lock that tells waits, when it is given, the path of a lock it
// found held, before it waits for it. A test hands in its own, so it hears
// that an edit waits rather than sees it not finish for a while.
func lock(fsys afero.Fs, path string, waits func(path string)) (release func()) {
	k := key{path: ResolveIn(fsys, path)}
	if !OnDisk(fsys) {
		k.fsys = fsys
	}
	held, _ := locks.LoadOrStore(k, &sync.Mutex{})
	mu := held.(*sync.Mutex)
	if !mu.TryLock() {
		if waits != nil {
			waits(k.path)
		}
		mu.Lock()
	}
	other := func() {}
	if OnDisk(fsys) {
		other = acrossProcesses(k.path, waits)
	}
	return func() {
		other()
		mu.Unlock()
	}
}
