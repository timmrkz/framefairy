package engine

import (
	"sync"
	"testing"

	"github.com/spf13/afero"
)

// lockWaiters holds, by the path of a lock, the channel a test hears on
// that an edit found that lock held and waits, see waitsOn.
var lockWaiters sync.Map

func init() {
	lockWaits = func(path string) {
		if c, ok := lockWaiters.Load(path); ok {
			select {
			case c.(chan struct{}) <- struct{}{}:
			default:
			}
		}
	}
}

// waitsOn has a value each time an edit of the file at path in fsys finds
// its lock held and waits for it.
func waitsOn(t *testing.T, fsys afero.Fs, path string) <-chan struct{} {
	t.Helper()
	c := make(chan struct{}, 1)
	key := ResolveIn(fsys, path)
	lockWaiters.Store(key, c)
	t.Cleanup(func() { lockWaiters.Delete(key) })
	return c
}

// memDir is a new empty folder in fsys, as t.TempDir is one on the disk.
func memDir(t testing.TB, fsys afero.Fs) string {
	t.Helper()
	dir, err := afero.TempDir(fsys, "", "test-")
	if err != nil {
		t.Fatal(err)
	}
	return dir
}

// A plan in one memory does not wait for a plan at the same path in
// another, so tests that each hold their own run side by side. The same
// memory waits, as the disk does.
func TestALockIsTheLockOfOneFileSystem(t *testing.T) {
	const path = "/ep.framefairy/logs/clips.json"
	a, b := afero.NewMemMapFs(), afero.NewMemMapFs()
	unlock := lockFile(a, path)
	// Both memories have their lock under the same path, so an edit of
	// either that waits says so here.
	waits := waitsOn(t, a, path)
	done := make(chan struct{})
	go func() {
		lockFile(b, path)()
		close(done)
	}()
	select {
	case <-done:
	case <-waits:
		unlock()
		t.Fatal("a plan in another memory waited for this one")
	}
	same := make(chan struct{})
	go func() {
		lockFile(a, path)()
		close(same)
	}()
	// An edit that did not wait never says so, and hangs here until go
	// test's -timeout.
	<-waits
	select {
	case <-same:
		t.Fatal("a second edit of the same plan did not wait")
	default:
	}
	unlock()
	<-same
}
