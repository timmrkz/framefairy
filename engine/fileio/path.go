// Package fileio is how the engine and the app treat a path and a file of
// a video's work folder, in whatever file system they are handed, the real
// disk or memory: where a path really leads, whether a name stays inside
// its folder, a file replaced in one step, and one edit of a file at a
// time. It knows nothing of plans, transcripts or videos.
package fileio

import (
	"fmt"
	"os"
	"path/filepath"
	"strings"

	"github.com/spf13/afero"

	"framefairy/engine/runes"
)

// OnDisk tells whether fsys is the real disk. Only the disk has links to
// follow and other programs to share a lock with.
func OnDisk(fsys afero.Fs) bool {
	_, ok := fsys.(*afero.OsFs)
	return ok
}

// ResolvePath makes a path absolute and follows the links in whatever part
// of it exists. Two paths that lead to the same file give the same answer,
// which is what a check that one is inside another has to be made on.
func ResolvePath(path string) string {
	abs, err := filepath.Abs(path)
	if err != nil {
		abs = path
	}
	// Resolve whatever part of the path exists.
	existing, rest := abs, ""
	for {
		if real, err := filepath.EvalSymlinks(existing); err == nil {
			return filepath.Join(real, rest)
		}
		parent := filepath.Dir(existing)
		if parent == existing {
			return abs
		}
		rest = filepath.Join(filepath.Base(existing), rest)
		existing = parent
	}
}

// ResolveIn is ResolvePath for a path in fsys. Memory holds no links, so a
// path there leads where it says, and cleaning it is all there is to do.
func ResolveIn(fsys afero.Fs, path string) string {
	if OnDisk(fsys) {
		return ResolvePath(path)
	}
	if abs, err := filepath.Abs(path); err == nil {
		return abs
	}
	return filepath.Clean(path)
}

// LstatIn is os.Lstat in fsys: a link is told as the link, not as what it
// leads to. Memory holds no links, so there it is Stat.
func LstatIn(fsys afero.Fs, path string) (os.FileInfo, error) {
	if l, ok := fsys.(afero.Lstater); ok {
		info, _, err := l.LstatIfPossible(path)
		return info, err
	}
	return fsys.Stat(path)
}

// SafeChild joins a name onto a folder on the disk and checks that the
// result stays inside it.
//
// Belt and braces next to SanitiseName in the engine. A path that escapes
// its folder is a bug worth failing loudly on, not one worth silently
// correcting.
func SafeChild(base, name string) (string, error) {
	return SafeChildIn(afero.NewOsFs(), base, name)
}

// SafeChildIn is SafeChild for a folder in fsys. The links it follows are
// the ones in fsys, so a folder in memory is never read through a link of
// the same name on the disk, like /home on a Mac.
func SafeChildIn(fsys afero.Fs, base, name string) (string, error) {
	root := ResolveIn(fsys, base)
	joined := ResolveIn(fsys, filepath.Join(base, name))
	rel, err := filepath.Rel(root, joined)
	if err != nil || rel == ".." || strings.HasPrefix(rel, ".."+string(filepath.Separator)) ||
		filepath.IsAbs(rel) {
		return "", fmt.Errorf("refusing to write outside %s: %s", base, runes.Quote(name))
	}
	return joined, nil
}

// Exists says whether there is anything at path in fsys.
func Exists(fsys afero.Fs, path string) bool {
	_, err := fsys.Stat(path)
	return err == nil
}

// IsFile says whether there is a plain file at path in fsys, not a folder.
func IsFile(fsys afero.Fs, path string) bool {
	info, err := fsys.Stat(path)
	return err == nil && info.Mode().IsRegular()
}
