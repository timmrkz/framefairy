package main

import (
	"bufio"
	"encoding/json"
	"fmt"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"strings"
	"sync"
	"testing"
	"time"

	"github.com/rs/zerolog"
)

// testLog opens the app's log in a folder of the test's own and gives
// what it holds, a line each, read as JSON. A line that is not whole JSON
// fails the test.
func testLog(t *testing.T) (path string, read func() []map[string]any) {
	t.Helper()
	path = filepath.Join(t.TempDir(), "logs", "app.log")
	theLog.open(path)
	t.Cleanup(func() {
		theLog.close()
		theLog.detailed(false)
	})
	return path, func() []map[string]any {
		t.Helper()
		f, err := os.Open(path)
		if err != nil {
			t.Fatal(err)
		}
		defer f.Close()
		var lines []map[string]any
		scan := bufio.NewScanner(f)
		scan.Buffer(nil, 1<<20)
		for scan.Scan() {
			var line map[string]any
			if err := json.Unmarshal(scan.Bytes(), &line); err != nil {
				t.Fatalf("a line of the log is not whole JSON: %v\n%s", err, scan.Text())
			}
			lines = append(lines, line)
		}
		return lines
	}
}

// find is the first line whose message starts so, or nil.
func find(lines []map[string]any, msg string) map[string]any {
	for _, l := range lines {
		if s, _ := l["msg"].(string); strings.HasPrefix(s, msg) {
			return l
		}
	}
	return nil
}

// has says whether a line holds every field given, as the log wrote it.
func has(t *testing.T, line map[string]any, want map[string]any) {
	t.Helper()
	if line == nil {
		t.Errorf("no line for %v", want)
		return
	}
	for k, v := range want {
		if fmt.Sprint(line[k]) != fmt.Sprint(v) {
			t.Errorf("%s is %v, want %v, in %v", k, line[k], v, line)
		}
	}
}

// The window's lines reach the app's log as JSON, each under the moment
// the window said it, with its level, the video it is about and the run
// of the app, with what one call may write held to a few hundred lines.
// Several windows' calls at once keep their lines whole. A trace line is
// written only while the log is detailed. Plan rows 2.185 and 2.188.
func TestSaidWritesTheAppLog(t *testing.T) {
	_, read := testLog(t)

	f := &FrameFairy{}
	before := time.Now().Add(-3 * time.Second).Truncate(time.Millisecond)
	f.Said([]SaidLine{
		{At: float64(before.UnixMilli()), Level: "error", Text: "frame queue: The app could not decode the picture."},
		{Video: "/videos/start.mp4", Text: "video preview: opening\n"},
		{Level: "trace", Text: "a frame, unseen"},
	})
	theLog.detailed(true)
	f.Said([]SaidLine{{Level: "trace", Text: "a frame, seen"}})
	theLog.detailed(false)
	var lots []SaidLine
	for i := range 430 {
		lots = append(lots, SaidLine{Text: fmt.Sprintf("line %d", i)})
	}
	f.Said(lots)
	var wg sync.WaitGroup
	for i := range 8 {
		wg.Go(func() { f.Said([]SaidLine{{Text: fmt.Sprintf("at once %d %s", i, strings.Repeat("x", 3000))}}) })
	}
	wg.Wait()

	lines := read()
	err := find(lines, "frame queue:")
	has(t, err, map[string]any{"level": "error", "from": "window", "run": theLog.run})
	if at, _ := time.Parse(zerolog.TimeFieldFormat, fmt.Sprint(err["time"])); !at.Equal(before) {
		t.Errorf("the line stands at %v, want the moment the window said it, %v", err["time"], before)
	}
	has(t, find(lines, "video preview: opening"), map[string]any{"level": "debug", "video": "start.mp4", "msg": "video preview: opening"})
	if find(lines, "a frame, unseen") != nil {
		t.Error("a trace line was written while the log was not detailed")
	}
	has(t, find(lines, "a frame, seen"), map[string]any{"level": "trace"})
	has(t, find(lines, "line 399"), map[string]any{"level": "debug"})
	has(t, find(lines, "and 30 lines more"), map[string]any{"level": "warn", "more": 30})
	if find(lines, "line 400") != nil {
		t.Error("one call wrote more than 400 lines")
	}
	for i := range 8 {
		start := fmt.Sprintf("at once %d ", i)
		if l := find(lines, start); l == nil || l["msg"] != start+strings.Repeat("x", 2000-len(start))+"…" {
			t.Errorf("the long line %d is not whole and cut at 2000 characters", i)
		}
	}
}

// The video's file and frames say in the log what was asked and how it
// went, with the video and the stream as fields of their own. A pull of
// a stream that plays as it should is a trace line, so a play fills the
// log only while it is detailed. Plan rows 2.185 and 2.188.
func TestFramesRequestsAreLogged(t *testing.T) {
	_, read := testLog(t)

	serve := func(target string, h func(w http.ResponseWriter, r *http.Request)) {
		logged(httptest.NewRecorder(), httptest.NewRequest("GET", target, nil), h)
	}
	pull := func(w http.ResponseWriter, r *http.Request) { _, _ = w.Write(make([]byte, 64)) }
	serve("/frames/open?path=/videos/start.mp4&from=1.5&w=874&h=492", func(w http.ResponseWriter, r *http.Request) {
		_, _ = w.Write([]byte(`{"id":"1dcdec3203858e5a"}`))
	})
	serve("/frames/read?id=1dcdec3203858e5a&n=4&skip=0", pull)
	serve("/frames/read?id=1dcdec3203858e5a&n=4&skip=1", func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("X-Frames-End", "1")
		w.Header().Set("X-Frames-Error", "the picture could not be decoded")
		w.WriteHeader(http.StatusOK)
	})
	serve("/frames/close?id=none", func(w http.ResponseWriter, r *http.Request) { w.WriteHeader(http.StatusNoContent) })
	serve("/frames/hold?path=/videos/start.mp4", func(w http.ResponseWriter, r *http.Request) {
		http.Error(w, "the video's decoder could not start", http.StatusInternalServerError)
	})
	theLog.detailed(true)
	serve("/frames/read?id=1dcdec3203858e5a&n=4&skip=2", pull)

	lines := read()
	has(t, find(lines, "open "), map[string]any{
		"level": "debug", "from": "files", "video": "start.mp4", "msg": "open from 1.5 at 874x492",
		"status": 200, "bytes": 25, "said": `{"id":"1dcdec3203858e5a"}`,
	})
	has(t, find(lines, "read n 4 skip 1"), map[string]any{
		"level": "warn", "stream": "1dcdec", "msg": "read n 4 skip 1, failed: the picture could not be decoded", "status": 200,
	})
	has(t, find(lines, "hold"), map[string]any{
		"level": "warn", "video": "start.mp4", "status": 500, "said": "the video's decoder could not start",
	})
	if find(lines, "read n 4 skip 0") != nil {
		t.Error("a pull that went as it should was written while the log was not detailed")
	}
	has(t, find(lines, "read n 4 skip 2"), map[string]any{"level": "trace", "bytes": 64})
	if find(lines, "close") != nil {
		t.Error("the worker's reach probe was written")
	}
}

// Wails's own lines go into the same file, from info up, since its debug
// lines are one for every request the window makes. Plan row 2.188.
func TestWailsWritesTheAppLog(t *testing.T) {
	_, read := testLog(t)
	wails := theLog.slog("wails", zerolog.InfoLevel)
	wails.Debug("Asset Request:", "path", "/frames/read")
	wails.Error("the window could not be made", "error", "no display")

	lines := read()
	if find(lines, "Asset Request") != nil {
		t.Error("Wails's debug line was written")
	}
	has(t, find(lines, "the window could not be made"), map[string]any{
		"level": "error", "from": "wails", "run": theLog.run, "error": "no display",
	})
}

// The log keeps to its size: past 10 MB it starts a new file and keeps
// the one before beside it, so it never fills the disk. Lines written
// from many places at once while it does stay whole. Plan row 2.188.
func TestTheLogKeepsToItsSize(t *testing.T) {
	path, read := testLog(t)
	text := strings.Repeat("y", 1000)
	var wg sync.WaitGroup
	for range 8 {
		wg.Go(func() {
			for range 1500 {
				theLog.line(zerolog.DebugLevel, "app").Msg(text)
			}
		})
	}
	wg.Wait()
	info, err := os.Stat(path)
	if err != nil {
		t.Fatal(err)
	}
	if info.Size() > appLogMB<<20 {
		t.Errorf("the log is %d bytes, more than %d MB", info.Size(), appLogMB)
	}
	older, _ := filepath.Glob(filepath.Join(filepath.Dir(path), "app-*"))
	if len(older) == 0 {
		t.Error("no older file was kept beside the log")
	}
	read()
}
