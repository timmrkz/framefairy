package engine

import (
	"os"
	"path/filepath"
	"sync"

	"github.com/spf13/afero"
)

// Records is what a machine keeps of the work done on it: the training
// records, see TRAINING.md, and the timings of past searches, see
// searchclock.go. They are on Files, the disk in the programs and memory
// in a test. A program makes one and hands it to whatever records, the
// engine and the app's store, like every other file it owns, so a test
// has records of its own and runs beside the tests that record or search.
// A nil Records keeps nothing and remembers no search.
type Records struct {
	Files afero.Fs

	// mu guards the training folder, which the app's settings change
	// while jobs record from their own goroutines.
	mu  sync.RWMutex
	dir string
	// speeds is the file of the timings of past searches.
	speeds string
	// write is held while a record is appended, speedWrite while the
	// timings are read, changed and written back, so two searches that
	// finish at once do not each write over the other.
	write      sync.Mutex
	speedWrite sync.Mutex
}

// NewRecords keeps the records on fsys, in the training folder dir and
// the timings in the file speeds. Empty means the default of each, beside
// the models in the user's home, see DefaultTrainingDir and
// DefaultSpeedFile.
func NewRecords(fsys afero.Fs, dir, speeds string) *Records {
	if dir == "" {
		dir = DefaultTrainingDir()
	}
	if speeds == "" {
		speeds = DefaultSpeedFile()
	}
	return &Records{Files: fsys, dir: dir, speeds: speeds}
}

// DefaultSpeedFile is where the timings of past searches are kept unless
// something says otherwise. It is a file of its own rather than a setting,
// because it describes the machine, not a choice. Empty when there is no
// home to keep it in.
func DefaultSpeedFile() string {
	home, err := os.UserHomeDir()
	if err != nil {
		return ""
	}
	return filepath.Join(home, ".framefairy", "speed.json")
}

// SetTrainingDir points the records at another folder. An empty path
// means the default one.
func (r *Records) SetTrainingDir(dir string) {
	if dir == "" {
		dir = DefaultTrainingDir()
	}
	r.mu.Lock()
	defer r.mu.Unlock()
	r.dir = dir
}

// TrainingDir is the folder the records are written to and read from.
func (r *Records) TrainingDir() string {
	r.mu.RLock()
	defer r.mu.RUnlock()
	return r.dir
}
