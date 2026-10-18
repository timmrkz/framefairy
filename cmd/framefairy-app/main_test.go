package main

import (
	"context"
	"net/http"
	"net/http/httptest"
	"net/url"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"framefairy/engine"
)

func TestLanesRunSideBySide(t *testing.T) {
	st := memoryStore()
	q := newQueue(st, func(JobUpdate) {}, nil)
	release := make(chan struct{})
	started := make(chan string, 2)
	block := func(name string) func(context.Context, *engine.Project) (string, error) {
		return func(ctx context.Context, _ *engine.Project) (string, error) {
			started <- name
			select {
			case <-release:
				return "", nil
			case <-ctx.Done():
				return "", engine.ErrCancelled
			}
		}
	}
	q.add("a.mp4", "model", "Speech model", block("model"))
	q.add("a.mp4", "llm", "Language model", block("llm"))
	for i := 0; i < 2; i++ {
		select {
		case <-started:
		case <-time.After(5 * time.Second):
			t.Fatal("the two lanes did not run at the same time")
		}
	}
	close(release)
}

// Nothing serves the bytes of a file to the page, not even of an episode
// in the library: the video preview reads nothing of the file itself, see
// docs/VIDEO-PREVIEW.md, step 5. The frames route takes an episode's file
// and nothing else.
//
// On the real disk: the frames route hands an episode to its decoder,
// which reads it.
func TestNothingServesTheBytesOfAFile(t *testing.T) {
	home := t.TempDir()
	t.Setenv("HOME", home)
	t.Setenv("XDG_CONFIG_HOME", filepath.Join(home, "config"))
	source := filepath.Join(home, "ep.mp4")
	other := filepath.Join(home, "secret.txt")
	if err := os.WriteFile(source, []byte("0123456789"), 0o644); err != nil {
		t.Fatal(err)
	}
	_ = os.WriteFile(other, []byte("nope"), 0o644)
	st := openStore()
	if _, err := st.AddEpisodes([]string{source}); err != nil {
		t.Fatal(err)
	}
	handler := mediaMiddleware(st)(http.NotFoundHandler())

	rec := httptest.NewRecorder()
	handler.ServeHTTP(rec, httptest.NewRequest("GET", "/media/?path="+url.QueryEscape(source), nil))
	if rec.Code != http.StatusNotFound || strings.Contains(rec.Body.String(), "0123") {
		t.Errorf("the episode's file was served with %d: %q", rec.Code, rec.Body.String())
	}
	rec = httptest.NewRecorder()
	handler.ServeHTTP(rec, httptest.NewRequest("GET", "/frames/release?path="+url.QueryEscape(source), nil))
	if rec.Code != http.StatusNoContent {
		t.Errorf("the episode's file was not taken by the frames route: %d", rec.Code)
	}
	for _, path := range []string{other, filepath.Join(engine.WorkDir(source), "..", "secret.txt"), "relative.mp4"} {
		rec = httptest.NewRecorder()
		handler.ServeHTTP(rec, httptest.NewRequest("GET", "/frames/release?path="+url.QueryEscape(path), nil))
		if rec.Code != http.StatusNotFound {
			t.Errorf("%s was taken with %d", path, rec.Code)
		}
	}
}

// An episode that has not been transcribed yet is a state every episode
// starts in. The reads the workspace makes on opening must answer with
// nothing rather than fail, or the log fills with errors nobody can act on.
func TestAnEpisodeWithoutATranscriptReadsEmpty(t *testing.T) {
	svc, eps := memoryLibrary(t, "ep.mp4")
	source := eps[0]

	peaks, err := svc.Waveform(source, 0, 60, 100)
	if err != nil || len(peaks) != 0 {
		t.Errorf("waveform got %d peaks, %v", len(peaks), err)
	}
	words, err := svc.Words(source, 0, 60)
	if err != nil || words == nil || len(words) != 0 {
		t.Errorf("words got %v, %v", words, err)
	}
	sentences, err := svc.Sentences(source)
	if err != nil || sentences == nil || len(sentences) != 0 {
		t.Errorf("sentences got %v, %v", sentences, err)
	}
	room, err := svc.Room(source)
	if err != nil || len(room.Lines) != 0 || room.Chars <= 0 || room.Rate != engine.SpokenChars {
		t.Errorf("room %+v, %v", room, err)
	}
}
