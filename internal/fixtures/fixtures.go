// Package fixtures holds the videos and sounds the tests read, made once
// and committed, so no test makes one while it runs.
//
// Tests once made their own with ffmpeg, about 160 on every run of the Go
// tests, most of them a few seconds of the same test picture. That cost
// about 36 s of ffmpeg work a run, and on CI's virtual Mac a file made on
// the graphics chip failed, or hung the whole package. A file that is
// already there can do neither.
//
// Each file is made by its recipe in fixtures_test.go, and
//
//	go test ./internal/fixtures -update
//
// makes them all again. A test asks for one by name with Path.
package fixtures

import (
	"os"
	"path/filepath"
	"runtime"
	"testing"
)

// Dir is the folder the fixtures are in, found from this file, so a test
// in any package of the module finds them wherever it runs.
func Dir() string {
	_, file, _, ok := runtime.Caller(0)
	if !ok {
		return "testdata"
	}
	return filepath.Join(filepath.Dir(file), "testdata")
}

// Path is the fixture called name, and fails the test when there is none,
// with the command that makes it.
func Path(t testing.TB, name string) string {
	t.Helper()
	path := filepath.Join(Dir(), name)
	if _, err := os.Stat(path); err != nil {
		var there []string
		entries, _ := os.ReadDir(Dir())
		for _, e := range entries {
			if filepath.Ext(e.Name()) != ".sum" {
				there = append(there, e.Name())
			}
		}
		t.Fatalf("no fixture %s, the fixtures are %v. Use one of them, or add a recipe in fixtures_test.go when none has what the test needs", name, there)
	}
	return path
}
