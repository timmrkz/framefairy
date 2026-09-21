package engine

import (
	"os"
	"path/filepath"
	"testing"
)

// The tests never write into the home folder of whoever runs them, so the
// one training folder is a temporary one for the whole package.
func TestMain(m *testing.M) {
	dir, err := os.MkdirTemp("", "framefairy-training")
	if err != nil {
		panic(err)
	}
	SetTrainingDir(filepath.Join(dir, "training"))
	code := m.Run()
	_ = os.RemoveAll(dir)
	os.Exit(code)
}
