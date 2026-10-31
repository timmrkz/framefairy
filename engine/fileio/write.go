package fileio

import (
	"path/filepath"

	"github.com/spf13/afero"
)

// WriteAtomic replaces the file at path with body in one step as far as
// any reader is concerned, see Replace.
func WriteAtomic(fsys afero.Fs, path string, body []byte) error {
	return Replace(fsys, path, "."+filepath.Base(path)+"-*", body, nil)
}

// Replace writes body beside path, under a temporary name made from
// pattern, and moves it onto path. check, if there is one, reads the
// temporary file first, and a file it refuses is never moved.
//
// The file is flushed to the disk before it is moved, and the folder after,
// so that after a power cut path holds either what was there or all of
// body. Without the flush the move can reach the disk before the bytes do,
// and a plan or a transcript comes back empty. On macOS Go's Sync is a
// full flush, F_FULLFSYNC, which is the one the drive does not keep in
// its own cache. In memory both flushes are nothing.
func Replace(fsys afero.Fs, path, pattern string, body []byte, check func(tmp string) error) error {
	dir := filepath.Dir(path)
	tmp, err := afero.TempFile(fsys, dir, pattern)
	if err != nil {
		return err
	}
	name := tmp.Name()
	_, err = tmp.Write(body)
	if err == nil {
		err = tmp.Sync()
	}
	if closeErr := tmp.Close(); err == nil {
		err = closeErr
	}
	if err == nil && check != nil {
		err = check(name)
	}
	if err == nil {
		err = fsys.Rename(name, path)
	}
	if err != nil {
		fsys.Remove(name)
		return err
	}
	// The move is kept by the folder, so the folder is flushed too. A
	// system that cannot open a folder for that, Windows, keeps it anyway.
	if d, err := fsys.Open(dir); err == nil {
		_ = d.Sync()
		d.Close()
	}
	return nil
}
