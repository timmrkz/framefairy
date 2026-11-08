package fileio

import (
	"errors"
	"os"
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

// flushes counts how often a file or a folder is flushed to the drive.
type flushes struct {
	afero.Fs
	n int
}

func (f *flushes) OpenFile(name string, flag int, perm os.FileMode) (afero.File, error) {
	file, err := f.Fs.OpenFile(name, flag, perm)
	return &flushed{file, f}, err
}

func (f *flushes) Open(name string) (afero.File, error) {
	file, err := f.Fs.Open(name)
	return &flushed{file, f}, err
}

type flushed struct {
	afero.File
	f *flushes
}

func (f *flushed) Sync() error {
	f.f.n++
	return f.File.Sync()
}

// Both writes replace a file whole, and only the one for what cannot be
// made again flushes it and its folder to the drive: on macOS that is a
// full flush, F_FULLFSYNC, which the loudness of a long video made twice
// a second.
func TestAFileIsFlushedOnlyWhenItCannotBeMadeAgain(t *testing.T) {
	for _, c := range []struct {
		name    string
		write   func(afero.Fs, string, []byte) error
		flushes int
	}{
		{"WriteAtomic", WriteAtomic, 2},
		{"WriteUnflushed", WriteUnflushed, 0},
	} {
		t.Run(c.name, func(t *testing.T) {
			fsys := &flushes{Fs: afero.NewMemMapFs()}
			path := "/logs/levels.json"
			if err := fsys.MkdirAll("/logs", 0o755); err != nil {
				t.Fatal(err)
			}
			for _, body := range []string{"before", "after"} {
				if err := c.write(fsys, path, []byte(body)); err != nil {
					t.Fatal(err)
				}
				if got, _ := afero.ReadFile(fsys, path); string(got) != body {
					t.Errorf("the file is %q, not %q", got, body)
				}
			}
			if left, _ := afero.Glob(fsys, "/logs/.*"); len(left) != 0 {
				t.Errorf("left beside it: %v", left)
			}
			if fsys.n != 2*c.flushes {
				t.Errorf("two writes flushed %d times, not %d", fsys.n, 2*c.flushes)
			}
		})
	}
}
