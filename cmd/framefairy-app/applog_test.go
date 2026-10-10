package main

import (
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"sync"
	"testing"
)

// The window's lines reach the app's log, each under its moment and where
// it came from, with what one call may write held to a few dozen lines.
// Several windows' calls at once keep their lines whole. Plan row 2.185.
func TestSaidWritesTheAppLog(t *testing.T) {
	path := filepath.Join(t.TempDir(), "logs", "app.log")
	was := theLog.path
	theLog.path = path
	t.Cleanup(func() { theLog.path = was })

	f := &FrameFairy{}
	f.Said([]string{"error: frame queue: The app could not decode the picture.", "two\nlines"})
	var lots []string
	for i := range 80 {
		lots = append(lots, fmt.Sprintf("line %d", i))
	}
	f.Said(lots)
	var wg sync.WaitGroup
	for i := range 8 {
		wg.Go(func() { f.Said([]string{fmt.Sprintf("at once %d %s", i, strings.Repeat("x", 3000))}) })
	}
	wg.Wait()

	data, err := os.ReadFile(path)
	if err != nil {
		t.Fatal(err)
	}
	log := string(data)
	for _, want := range []string{" window error: frame queue: The app could not decode the picture.\n", " window two\n", " window lines\n", " window line 49\n", " window and 30 lines more\n"} {
		if !strings.Contains(log, want) {
			t.Errorf("the log has no %q:\n%s", want, log)
		}
	}
	if strings.Contains(log, "line 50\n") {
		t.Error("one call wrote more than 50 lines")
	}
	for i := range 8 {
		if !strings.Contains(log, fmt.Sprintf("at once %d %s…\n", i, strings.Repeat("x", 2000-len(fmt.Sprintf("at once %d ", i))))) {
			t.Errorf("the long line %d is not whole and cut at 2000 characters", i)
		}
	}
}
