package fileio

import (
	"errors"
	"path/filepath"
	"testing"

	"github.com/spf13/afero"
)

// A file is replaced whole or not at all: a write its check refuses
// leaves the file that was there, and nothing beside it.
func TestAFileIsReplacedWholeOrNotAtAll(t *testing.T) {
	fsys := afero.NewMemMapFs()
	dir := "/logs"
	path := filepath.Join(dir, "plan.json")
	if err := WriteAtomic(fsys, path, []byte("before")); err != nil {
		t.Fatal(err)
	}
	refuse := func(string) error { return errors.New("not a plan") }
	if err := Replace(fsys, path, ".plan-*", []byte("after"), refuse); err == nil {
		t.Fatal("a refused write went through")
	}
	if got, _ := afero.ReadFile(fsys, path); string(got) != "before" {
		t.Errorf("the file is now %q", got)
	}
	if left, _ := afero.Glob(fsys, filepath.Join(dir, ".*")); len(left) != 0 {
		t.Errorf("left beside it: %v", left)
	}
	if err := WriteAtomic(fsys, path, []byte("after")); err != nil {
		t.Fatal(err)
	}
	if got, _ := afero.ReadFile(fsys, path); string(got) != "after" {
		t.Errorf("the file is now %q", got)
	}
}
