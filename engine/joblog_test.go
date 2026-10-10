package engine

import (
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"
)

func TestJobLogsAreKeptToTheNewest(t *testing.T) {
	source := filepath.Join(t.TempDir(), "ep.mp4")
	at := time.Date(2026, 10, 9, 21, 0, 0, 0, time.UTC)
	for i := range jobLogsKept + 5 {
		f, err := OpenJobLog(source, JobSearch, at.Add(time.Duration(i)*time.Second))
		if err != nil {
			t.Fatal(err)
		}
		f.WriteString("heard\n")
		f.Close()
	}
	entries, err := filepath.Glob(filepath.Join(JobsDir(source), "*.log"))
	if err != nil {
		t.Fatal(err)
	}
	if len(entries) != jobLogsKept {
		t.Fatalf("%d logs kept, want %d", len(entries), jobLogsKept)
	}
	// The logs lie beside the records and are none of them.
	if recs := ReadJobs(source); len(recs) != 0 {
		t.Errorf("logs read as records: %+v", recs)
	}
	// The five oldest went.
	if first := filepath.Base(entries[0]); !strings.HasPrefix(first, "2026-10-09-210005.000-search") {
		t.Errorf("oldest kept is %s", first)
	}
}

func TestAJobLogIsNoWayOutOfTheWorkFolder(t *testing.T) {
	source := filepath.Join(t.TempDir(), "ep.mp4")
	outside := t.TempDir()
	if err := os.MkdirAll(WorkDir(source), 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.Symlink(outside, JobsDir(source)); err != nil {
		t.Skip("no symlinks here")
	}
	if f, err := OpenJobLog(source, JobRender, time.Now()); err == nil {
		f.Close()
		t.Fatal("a log was written through a link out of the work folder")
	}
	if entries, _ := os.ReadDir(outside); len(entries) != 0 {
		t.Errorf("wrote %d files outside", len(entries))
	}
	if _, err := OpenJobLog(source, "../x", time.Now()); err == nil {
		t.Error("a kind that is a path was taken")
	}
}
