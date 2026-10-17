package engine

import (
	"os"
	"path/filepath"

	"github.com/spf13/afero"
)

// The engine reads and writes the files of an episode through an afero.Fs,
// never through os directly. The programs hand it the real disk,
// afero.NewOsFs, and a test hands it memory, afero.NewMemMapFs, because a
// test is about what our code does with a plan or a transcript, not about
// the drive under it. The file system is held by what owns the files, the
// Engine, a Project and the app's store, and a function that has no owner
// takes it as its first argument. The training records and the timings
// of past searches are held the same way, by Records, which the engine and
// the app's store own and hand to the edits that record, see records.go.
//
// What an outside program reads or writes, ffmpeg, the frames decoder,
// llama-server, is on the real disk whatever the engine is given, because
// a separate process cannot see the memory of this one: the shorts, their
// pictures and caption files, llama-server's log, the models. The episode
// itself is read by ffmpeg from the disk, and by our own code, its size,
// its time and the fingerprint of the training records, through the file
// system given, which in the programs is the same disk. Every other file
// of the work folder, the plans, the saved answers, usage.json, the
// prompts and proof.txt, goes through the file system given.

// disk is the real disk, for the files that are on it whatever the engine
// is given: the models, which llama-server reads, and the notes about the
// programs this one started. It is never swapped.
var disk afero.Fs = afero.NewOsFs()

// OnDisk tells whether fsys is the real disk. Only the disk has links to
// follow and other programs to share a lock with.
func OnDisk(fsys afero.Fs) bool {
	_, ok := fsys.(*afero.OsFs)
	return ok
}

// ResolveIn is ResolvePath for a path in fsys. Memory holds no links, so a
// path there leads where it says, and cleaning it is all there is to do.
func ResolveIn(fsys afero.Fs, path string) string {
	if OnDisk(fsys) {
		return resolvePath(path)
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
