package fileio

import (
	"errors"
	"fmt"
	"os"

	"github.com/spf13/afero"
)

// RemoveFolderIn deletes a folder in fsys and everything in it, and is
// done when there is nothing at path. A link is deleted itself, never what
// it leads to, wherever it is in the folder.
//
// On the disk it reaches the folder only through the folder itself, never
// through the folder it is in. os.RemoveAll opens the folder above to
// delete through it, and macOS can refuse that while it lets a program
// write there: a screen recording dragged from its thumbnail in the corner
// lives in a temporary folder of macOS's own, which the app could make the
// recording's work folder in and then never delete it again. The folder
// is opened as a root, so nothing is deleted through a link put in its
// place, the way os.RemoveAll deletes nothing through one either.
func RemoveFolderIn(fsys afero.Fs, path string) error {
	if !OnDisk(fsys) {
		return fsys.RemoveAll(path)
	}
	info, err := os.Lstat(path)
	if errors.Is(err, os.ErrNotExist) {
		return nil
	}
	if err != nil {
		return err
	}
	if info.IsDir() {
		if err := emptyFolder(path, info); err != nil {
			return fmt.Errorf("the folder %s could not be deleted: %w", path, err)
		}
	}
	if err := os.Remove(path); err != nil && !errors.Is(err, os.ErrNotExist) {
		return err
	}
	return nil
}

// emptyFolder deletes everything in the folder at path, which Lstat said
// was the folder info, through the folder opened as a root.
func emptyFolder(path string, info os.FileInfo) error {
	root, err := os.OpenRoot(path)
	if err != nil {
		return err
	}
	// Closed before the folder itself goes, since Windows deletes no
	// folder that is still open.
	defer root.Close()
	// OpenRoot follows a link, so a link put where the folder was since
	// Lstat would have been opened instead. Only the folder looked at is
	// emptied.
	opened, err := root.Stat(".")
	if err != nil {
		return err
	}
	if !os.SameFile(info, opened) {
		return fmt.Errorf("%s changed while it was being deleted, so nothing in it was", path)
	}
	dir, err := root.Open(".")
	if err != nil {
		return err
	}
	names, err := dir.Readdirnames(-1)
	dir.Close()
	if err != nil {
		return err
	}
	for _, name := range names {
		if err := root.RemoveAll(name); err != nil {
			return err
		}
	}
	return nil
}
