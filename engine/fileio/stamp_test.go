package fileio

import (
	"path/filepath"
	"runtime"
	"testing"
	"time"

	"github.com/spf13/afero"
)

// On the real disk, because the number the system gives a file is what
// is tested, and a file system in memory keeps none.
//
// Two writes that leave a file the same size, in the same tick of the
// drive's clock, still change its stamp: WriteAtomic puts a new file in
// the old one's place. The tick is made by setting both times the same.
func TestTwoWritesInOneTickChangeTheStamp(t *testing.T) {
	if runtime.GOOS == "windows" {
		t.Skip("Windows gives no file number, its times tell writes apart")
	}
	fsys := afero.NewOsFs()
	path := filepath.Join(t.TempDir(), "clips.json")
	tick := time.Date(2026, 11, 4, 12, 0, 0, 0, time.UTC)
	stampAfter := func(body string) string {
		t.Helper()
		if err := WriteAtomic(fsys, path, []byte(body)); err != nil {
			t.Fatal(err)
		}
		if err := fsys.Chtimes(path, tick, tick); err != nil {
			t.Fatal(err)
		}
		info, err := fsys.Stat(path)
		if err != nil {
			t.Fatal(err)
		}
		return Stamp(info)
	}
	first := stampAfter(`{"start": 12.34}`)
	second := stampAfter(`{"start": 12.56}`)
	if first == second {
		t.Errorf("two writes of the same size at the same time share the stamp %s", first)
	}
}
