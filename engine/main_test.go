package engine

import (
	"os"
	"path/filepath"
	"testing"
)

// The tests never write into the home folder of whoever runs them, so the
// one training folder is a temporary one for the whole package, and so is
// the record of how long searches take.
func TestMain(m *testing.M) {
	dir, err := os.MkdirTemp("", "framefairy-training")
	if err != nil {
		panic(err)
	}
	SetTrainingDir(filepath.Join(dir, "training"))
	// Nor into the timings of past searches, which would then measure
	// every search on the machine against a fake model.
	SetSpeedFile(filepath.Join(dir, "speed.json"))
	code := m.Run()
	_ = os.RemoveAll(dir)
	os.Exit(code)
}
