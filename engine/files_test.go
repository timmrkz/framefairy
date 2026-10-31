package engine

import (
	"testing"

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
