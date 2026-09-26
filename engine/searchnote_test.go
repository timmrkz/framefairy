package engine

import (
	"context"
	"errors"
	"os"
	"strings"
	"testing"
	"time"
)

// A search says it is running while it runs, and leaves nothing behind when
// it finds its clips.
func TestASearchThatFindsItsClipsLeavesNoNote(t *testing.T) {
	letGo := make(chan struct{})
	p, path := searching(t, letGo)
	done := make(chan error, 1)
	go func() {
		_, err := p.Plan(context.Background(), PlanRequest{From: 10, To: 30, Count: 2, Min: 10})
		done <- err
	}()
	landed(t, path, 1)
	note := ReadSearchNote(p.Source)
	if note == nil || note.State != "running" || note.From != 10 || note.To != 30 {
		t.Fatalf("while it runs: %+v", note)
	}
	close(letGo)
	if err := <-done; err != nil {
		t.Fatal(err)
	}
	if note := ReadSearchNote(p.Source); note != nil {
		t.Errorf("after it found its clips: %+v", note)
	}
	if st := Status(p.Source, p.Base.ASRModel); st.LastSearch != nil {
		t.Errorf("the status still has a note: %+v", st.LastSearch)
	}
}

// A search that fails leaves its reason, in the words the log gave it.
func TestASearchThatFailsLeavesItsReason(t *testing.T) {
	letGo := make(chan struct{})
	close(letGo)
	p, _ := searching(t, letGo)
	// Five clips of ten seconds do not fit in twenty.
	_, err := p.Plan(context.Background(), PlanRequest{From: 10, To: 30, Count: 5, Min: 10})
	if err == nil {
		t.Fatal("a window too short for its clips was searched")
	}
	note := Status(p.Source, p.Base.ASRModel).LastSearch
	if note == nil || note.State != "failed" || !strings.Contains(note.Error, "clips of at least") {
		t.Fatalf("after it failed: %+v", note)
	}
	// The next search starts afresh, and says it is running.
	_, _ = p.Plan(context.Background(), PlanRequest{From: 10, To: 30, Count: 2, Min: 10})
	if note := ReadSearchNote(p.Source); note != nil {
		t.Errorf("after a search that worked: %+v", note)
	}
}

// A search called off by hand has nothing to say: whoever called it off
// knows why.
func TestASearchCalledOffLeavesNoNote(t *testing.T) {
	letGo := make(chan struct{})
	defer close(letGo)
	p, path := searching(t, letGo)
	ctx, cancel := context.WithCancel(context.Background())
	done := make(chan error, 1)
	go func() {
		_, err := p.Plan(ctx, PlanRequest{From: 10, To: 30, Count: 2, Min: 10})
		done <- err
	}()
	landed(t, path, 1)
	cancel()
	select {
	case err := <-done:
		if !errors.Is(err, ErrCancelled) {
			t.Fatalf("called off: %v", err)
		}
	case <-time.After(60 * time.Second):
		t.Fatal("the search never stopped")
	}
	if note := ReadSearchNote(p.Source); note != nil {
		t.Errorf("after it was called off: %+v", note)
	}
}

// The note is read like anything else on disk. What is not a note is none.
func TestANoteThatIsNotOneIsNone(t *testing.T) {
	source := testEpisode(t, "5")
	if err := os.MkdirAll(WorkDir(source), 0o755); err != nil {
		t.Fatal(err)
	}
	for _, body := range []string{
		``, `{`, `[]`, `{"state":"done"}`, `{"state":"running","from":-1}`,
		`{"state":"failed","from":30,"to":10}`, `{"state":"running","from":1e400}`,
	} {
		if err := os.WriteFile(searchNotePath(source), []byte(body), 0o644); err != nil {
			t.Fatal(err)
		}
		if note := ReadSearchNote(source); note != nil {
			t.Errorf("%q read as %+v", body, note)
		}
	}
	long := `{"state":"failed","from":0,"to":0,"error":"` + strings.Repeat("ä", 2000) + `"}`
	if err := os.WriteFile(searchNotePath(source), []byte(long), 0o644); err != nil {
		t.Fatal(err)
	}
	if note := ReadSearchNote(source); note == nil || len([]rune(note.Error)) != noteLimit {
		t.Errorf("a long reason: %+v", note)
	}
}
