package main

import (
	"fmt"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"regexp"
	"strings"
	"sync"
	"testing"
	"time"
)

// The window's lines reach the app's log, each under the moment the window
// said it and where it came from, with what one call may write held to a
// few hundred lines. Several windows' calls at once keep their lines
// whole. Plan row 2.185.
func TestSaidWritesTheAppLog(t *testing.T) {
	path := filepath.Join(t.TempDir(), "logs", "app.log")
	was := theLog.path
	theLog.path = path
	t.Cleanup(func() { theLog.path = was })

	f := &FrameFairy{}
	before := time.Now().Add(-3 * time.Second)
	f.Said([]SaidLine{
		{At: float64(before.UnixMilli()), Text: "error: frame queue: The app could not decode the picture."},
		{Text: "two\nlines"},
	})
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

	data, err := os.ReadFile(path)
	if err != nil {
		t.Fatal(err)
	}
	log := string(data)
	stamp := before.Format("2006-01-02 15:04:05.000")
	for _, want := range []string{stamp + " window error: frame queue: The app could not decode the picture.\n", " window two\n", " window lines\n", " window line 399\n", " window and 30 lines more\n"} {
		if !strings.Contains(log, want) {
			t.Errorf("the log has no %q:\n%s", want, log)
		}
	}
	if strings.Contains(log, "line 400\n") {
		t.Error("one call wrote more than 400 lines")
	}
	for i := range 8 {
		if !strings.Contains(log, fmt.Sprintf("at once %d %s…\n", i, strings.Repeat("x", 2000-len(fmt.Sprintf("at once %d ", i))))) {
			t.Errorf("the long line %d is not whole and cut at 2000 characters", i)
		}
	}
}

// The video's file and frames say in the log what was asked and how it
// went, and a pull of a stream that plays as it should says nothing, so
// the log is not filled by a play. Plan row 2.185.
func TestFramesRequestsAreLogged(t *testing.T) {
	path := filepath.Join(t.TempDir(), "app.log")
	was := theLog.path
	theLog.path = path
	t.Cleanup(func() { theLog.path = was })

	serve := func(target string, h func(w http.ResponseWriter, r *http.Request)) {
		logged(httptest.NewRecorder(), httptest.NewRequest("GET", target, nil), h)
	}
	serve("/frames/open?path=/videos/start.mp4&from=1.5&w=874&h=492", func(w http.ResponseWriter, r *http.Request) {
		_, _ = w.Write([]byte(`{"id":"1dcdec3203858e5a"}`))
	})
	serve("/frames/read?id=1dcdec3203858e5a&n=4", func(w http.ResponseWriter, r *http.Request) {
		_, _ = w.Write(make([]byte, 64))
	})
	serve("/frames/read?id=1dcdec3203858e5a&n=4", func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("X-Frames-End", "1")
		w.Header().Set("X-Frames-Error", "the picture could not be decoded")
		w.WriteHeader(http.StatusOK)
	})
	serve("/frames/close?id=none", func(w http.ResponseWriter, r *http.Request) { w.WriteHeader(http.StatusNoContent) })
	serve("/frames/hold?path=/videos/start.mp4", func(w http.ResponseWriter, r *http.Request) {
		http.Error(w, "the video's decoder could not start", http.StatusInternalServerError)
	})

	data, err := os.ReadFile(path)
	if err != nil {
		t.Fatal(err)
	}
	// How long each took is the machine's.
	log := regexp.MustCompile(`, \d+ ms`).ReplaceAllString(string(data), ", N ms")
	for _, want := range []string{
		` files open start.mp4 from 1.5 at 874x492: 200, 25 bytes, N ms, {"id":"1dcdec3203858e5a"}` + "\n",
		" files read 1dcdec n 4 skip , failed: the picture could not be decoded: 200, 0 bytes, N ms\n",
		" files hold start.mp4: 500, 36 bytes, N ms, the video's decoder could not start\n",
	} {
		if !strings.Contains(log, want) {
			t.Errorf("the log has no %q:\n%s", want, log)
		}
	}
	if n := strings.Count(log, " files read "); n != 1 {
		t.Errorf("%d pulls were written, want only the one that failed:\n%s", n, log)
	}
	if strings.Contains(log, "close") {
		t.Errorf("the worker's reach probe was written:\n%s", log)
	}
}
