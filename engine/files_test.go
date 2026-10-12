package engine

import (
	"testing"
	"time"

	"github.com/spf13/afero"
)

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
	a, b := afero.NewMemMapFs(), afero.NewMemMapFs()
	unlock := lockFile(a, "/ep.framefairy/logs/clips.json")
	done := make(chan struct{})
	go func() {
		lockFile(b, "/ep.framefairy/logs/clips.json")()
		close(done)
	}()
	select {
	case <-done:
	case <-time.After(5 * time.Second):
		unlock()
		t.Fatal("a plan in another memory waited for this one")
	}
	same := make(chan struct{})
	go func() {
		lockFile(a, "/ep.framefairy/logs/clips.json")()
		close(same)
	}()
	select {
	case <-same:
		unlock()
		t.Fatal("a second edit of the same plan did not wait")
	case <-time.After(50 * time.Millisecond):
	}
	unlock()
	<-same
}
