package engine

import (
	"bytes"
	"context"
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
)

// pausedModel answers like llama-server with two clips, and stops after the
// first until it is let go. That is a model still writing, and it is the
// moment the first clip has to be in the plan already.
func pausedModel(t *testing.T, letGo <-chan struct{}) *httptest.Server {
	return httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		first := `{"clips": [{"slug": "erste", "title": "Erste", "reason": "Test", "keep": [[1, 1]]}`
		rest := `, {"slug": "zweite", "title": "Zweite", "reason": "Test", "keep": [[2, 2]]}]}`
		send := func(piece string) {
			body, _ := json.Marshal(map[string]any{"choices": []any{map[string]any{
				"delta": map[string]any{"content": piece}}}})
			fmt.Fprintf(w, "data: %s\n\n", body)
			w.(http.Flusher).Flush()
		}
		w.Header().Set("content-type", "text/event-stream")
		send(first)
		select {
		case <-letGo:
		case <-r.Context().Done():
			return
		}
		send(rest)
		fmt.Fprint(w, "data: {\"choices\":[{\"delta\":{},\"finish_reason\":\"stop\"}]}\n\n")
		fmt.Fprint(w, "data: [DONE]\n\n")
	}))
}

// searching is an episode transcribed and ready to be searched against the
// paused model.
func searching(t *testing.T, letGo <-chan struct{}) (*Project, string) {
	t.Helper()
	source := testEpisode(t, "40")
	SetTrainingDir(t.TempDir())
	server := pausedModel(t, letGo)
	t.Cleanup(server.Close)
	var heard int32
	e := NewEngine(NewLog(&bytes.Buffer{}, false, false))
	e.OpenRecognizer = func(string) (Recognizer, error) { return fakeRecognizer{&heard}, nil }
	base := DefaultOptions()
	base.LLMURL = server.URL
	base.ASRModel = t.TempDir()
	base.Width, base.Height = 360, 640
	p := NewProject(e, source, base)
	if err := p.Transcribe(context.Background()); err != nil {
		t.Fatalf("transcribe: %v", err)
	}
	return p, filepath.Join(p.LogsDir(), "clips-10-30.json")
}

// landed waits for the plan to hold n clips and gives them.
func landed(t *testing.T, path string, n int) []Clip {
	t.Helper()
	deadline := time.Now().Add(60 * time.Second)
	for time.Now().Before(deadline) {
		if _, clips, err := LoadClips(path); err == nil && len(clips) >= n {
			return clips
		}
		time.Sleep(20 * time.Millisecond)
	}
	t.Fatalf("the plan never held %d clip(s)", n)
	return nil
}

func TestAClipLandsWhileTheModelIsStillWriting(t *testing.T) {
	letGo := make(chan struct{})
	p, path := searching(t, letGo)

	type result struct {
		plan string
		err  error
	}
	finished := make(chan result, 1)
	go func() {
		plan, err := p.Plan(context.Background(), PlanRequest{From: 10, To: 30, Count: 2})
		finished <- result{plan, err}
	}()

	// The model has written one clip and is still going. That clip is in
	// the plan, whole, with the id it will be recorded under.
	clips := landed(t, path, 1)
	if len(clips) != 1 || clips[0].ID != "t10-01" {
		t.Fatalf("clips %+v", clips)
	}
	plan, _, _ := LoadClips(path)
	early, _ := plan.Raw["plan_id"].(string)
	if early == "" {
		t.Fatal("the first clip landed without the plan's id")
	}
	select {
	case r := <-finished:
		t.Fatalf("the search finished before the model did: %+v", r)
	default:
	}

	// Everything the app can do to that clip, from several places at
	// once, while the rest of the answer arrives. None of it may be lost
	// to the clip that lands next.
	var wg sync.WaitGroup
	for i := range 8 {
		wg.Add(1)
		go func() {
			defer wg.Done()
			switch i % 4 {
			case 0:
				if err := SetCaptionY(path, "t10-01", 0.3); err != nil {
					t.Errorf("caption height: %v", err)
				}
			case 1:
				if err := SetRejected(path, "t10-01", false); err != nil {
					t.Errorf("keep: %v", err)
				}
			case 2:
				if err := RecordDecision(path, "t10-01", DecisionKept, nil); err != nil {
					t.Errorf("decision: %v", err)
				}
			case 3:
				if _, _, err := LoadClips(path); err != nil {
					t.Errorf("a plan read while it was written: %v", err)
				}
			}
		}()
	}
	close(letGo)
	wg.Wait()

	r := <-finished
	if r.err != nil {
		t.Fatalf("plan: %v %s", r.err, p.LastError())
	}
	final, clips, err := LoadClips(r.plan)
	if err != nil || len(clips) != 2 {
		t.Fatalf("final plan: %v, %d clip(s)", err, len(clips))
	}
	if clips[0].CaptionY == nil {
		t.Errorf("an edit made while the search ran was lost: %+v", clips[0])
	}
	id, _ := final.Raw["plan_id"].(string)
	if id != early {
		t.Errorf("plan id changed from %s to %s", early, id)
	}
	// The decision made while the model was still writing counts, against
	// the plan it was made about.
	decisions, _ := os.ReadFile(filepath.Join(TrainingDir(), "decisions.jsonl"))
	if !strings.Contains(string(decisions), `"plan_id":"`+id+`"`) {
		t.Errorf("the decision made during the search was not recorded: %s", decisions)
	}
	if _, ok := loadPlanRecord(TrainingDir(), id); !ok {
		t.Errorf("no plan record under %s", id)
	}
}

func TestAStoppedSearchKeepsWhatArrived(t *testing.T) {
	letGo := make(chan struct{})
	defer close(letGo)
	p, path := searching(t, letGo)
	ctx, cancel := context.WithCancel(context.Background())
	finished := make(chan error, 1)
	go func() {
		_, err := p.Plan(ctx, PlanRequest{From: 10, To: 30, Count: 2})
		finished <- err
	}()
	landed(t, path, 1)
	cancel()
	if err := <-finished; err == nil {
		t.Fatal("a stopped search said it finished")
	}
	if _, clips, err := LoadClips(path); err != nil || len(clips) != 1 {
		t.Errorf("what arrived before the stop: %v, %d clip(s)", err, len(clips))
	}
}

func TestAClipDoesNotLandInAPartRemovedWhileItWasOnItsWay(t *testing.T) {
	letGo := make(chan struct{})
	p, path := searching(t, letGo)
	finished := make(chan error, 1)
	go func() {
		_, err := p.Plan(context.Background(), PlanRequest{From: 10, To: 30, Count: 2})
		finished <- err
	}()
	clips := landed(t, path, 1)
	_, end := ClipSpan(clips[0])
	// Everything after the first clip is given back while the second is
	// still being written.
	if _, err := RemoveRange(path, p.CaptionsDir(), end, 30, 40); err != nil {
		t.Fatal(err)
	}
	close(letGo)
	if err := <-finished; err != nil {
		t.Fatalf("plan: %v", err)
	}
	if _, clips, err := LoadClips(path); err != nil || len(clips) != 1 || clips[0].ID != "t10-01" {
		t.Errorf("after the removal: %v, %+v", err, clips)
	}
}
