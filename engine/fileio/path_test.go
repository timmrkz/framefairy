package fileio

import (
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/spf13/afero"
)

// On the real disk: what is tested is that a link on the disk is no way
// out of a folder, and memory has no links.
func TestSafeChildStaysInsideItsFolder(t *testing.T) {
	base := t.TempDir()
	for _, name := range []string{"01_a.mp4", "sub/01.mp4", "sub/../01.mp4", "./a"} {
		if _, err := SafeChild(base, name); err != nil {
			t.Errorf("SafeChild(%q): %v", name, err)
		}
	}
	for _, name := range []string{"..", "../out.mp4", "sub/../../out.mp4"} {
		if path, err := SafeChild(base, name); err == nil {
			t.Errorf("SafeChild(%q) allowed %s", name, path)
		}
	}
	// An absolute name is joined onto the folder rather than followed, so it
	// lands inside as well.
	if path, err := SafeChild(base, "/etc/passwd"); err != nil || !strings.HasPrefix(path, ResolvePath(base)) {
		t.Errorf("absolute name gave %q, %v", path, err)
	}

	// A symlink planted inside the folder does not open a way out either.
	outside := t.TempDir()
	if err := os.Symlink(outside, filepath.Join(base, "link")); err != nil {
		t.Skipf("symlinks are not available here: %v", err)
	}
	if path, err := SafeChild(base, filepath.Join("link", "out.mp4")); err == nil {
		t.Errorf("a symlink led out of the folder: %s", path)
	}
}

// A folder in memory is joined as it is named, even where the disk has a
// link of the same name. On a Mac /home is one, so the chosen clip of an
// episode under /home was written to one path and looked for at another.
//
// On the real disk: the link that must not be followed is one on the disk.
func TestSafeChildInMemoryFollowsNoLinkOnTheDisk(t *testing.T) {
	target := t.TempDir()
	link := filepath.Join(t.TempDir(), "home")
	if err := os.Symlink(target, link); err != nil {
		t.Skipf("symlinks are not available here: %v", err)
	}
	base := filepath.Join(link, "tim", "ep.framefairy")
	want := filepath.Join(base, "chosen.json")
	if got, err := SafeChildIn(afero.NewMemMapFs(), base, "chosen.json"); err != nil || got != want {
		t.Errorf("in memory gave %q, %v, wanted %q", got, err, want)
	}
	if got, err := SafeChild(base, "chosen.json"); err != nil || got == want {
		t.Errorf("on the disk gave %q, %v, wanted the path the link leads to", got, err)
	}
	if path, err := SafeChildIn(afero.NewMemMapFs(), base, "../out.json"); err == nil {
		t.Errorf("in memory a way out of the folder was allowed: %s", path)
	}
}

// FuzzSafeChild makes sure the check itself cannot be talked around. Every
// name it accepts has to land inside the folder.
//
// On the real disk: what is tested is that a link on the disk is no way
// out of a folder, and memory has no links.
func FuzzSafeChild(f *testing.F) {
	for _, name := range []string{"01_a.mp4", "..", "../x", "a/../../x", "/etc/passwd",
		"a//b", ".", "", "a\x00b", "C:\\x"} {
		f.Add(name)
	}
	f.Fuzz(func(t *testing.T, name string) {
		base := t.TempDir()
		path, err := SafeChild(base, name)
		if err != nil {
			return
		}
		rel, relErr := filepath.Rel(ResolvePath(base), path)
		if relErr != nil || rel == ".." || strings.HasPrefix(rel, ".."+string(filepath.Separator)) {
			t.Fatalf("SafeChild(%q) = %q, which is outside %q", name, path, base)
		}
	})
}
