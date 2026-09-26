package engine

import (
	"bytes"
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"os"
	"strings"
	"sync"
	"testing"
)

// A clip well off the length is asked for again in the same conversation,
// told how long it runs and how long each line around it lasts, and the
// one that comes back nearer the length is the clip. The answer is saved
// with the reply, so a search that reuses the reply fits it the same way
// and asks nothing.
func TestAClipThatDoesNotFitIsAskedForAgain(t *testing.T) {
	source := testEpisode(t, "40")
	SetTrainingDir(t.TempDir())
	var mu sync.Mutex
	var asks [][]chatMessage
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		var request struct{ Messages []chatMessage }
		_ = json.NewDecoder(r.Body).Decode(&request)
		mu.Lock()
		asks = append(asks, request.Messages)
		mu.Unlock()
		// A line of the test episode is about 14 seconds. One is too short
		// for 20 to 30, two fit.
		keep := "[[1, 1]]"
		if len(request.Messages) > 2 {
			keep = "[[1, 2]]"
		}
		writeLocalStream(w, `{"clips": [{"slug": "kurz", "title": "Kurz", "reason": "r", "keep": `+keep+`}]}`, 11)
	}))
	defer server.Close()
	var heard int32
	var said bytes.Buffer
	e := NewEngine(NewLog(&said, false, false))
	e.OpenRecognizer = func(string) (Recognizer, error) { return fakeRecognizer{&heard}, nil }
	base := DefaultOptions()
	base.LLMURL = server.URL
	base.ASRModel = t.TempDir()
	base.Width, base.Height = 360, 640
	p := NewProject(e, source, base)
	ctx := context.Background()

	path, err := p.Plan(ctx, PlanRequest{Count: 1})
	if err != nil {
		t.Fatalf("plan: %v %s", err, p.LastError())
	}
	if len(asks) != 2 {
		t.Fatalf("the model was asked %d times", len(asks))
	}
	again := asks[1]
	if len(again) != 4 || again[1].Content != asks[0][1].Content || again[2].Role != "assistant" ||
		!strings.Contains(again[2].Content, `"kurz"`) {
		t.Fatalf("the second ask is not the same conversation: %+v", again)
	}
	for _, want := range []string{`"kurz" runs 14 seconds, 6 under the 20 second minimum`,
		"It keeps [[1, 1]].", "Seconds of each line around it: [1] 13.", "[2] 1", "same slugs"} {
		if !strings.Contains(again[3].Content, want) {
			t.Errorf("the second ask has no %q:\n%s", want, again[3].Content)
		}
	}
	_, clips, err := LoadClips(path)
	if err != nil || len(clips) != 1 {
		t.Fatalf("clips %v %v", clips, err)
	}
	if n := clips[0].Duration(); n < 20 || n > 30 {
		t.Errorf("the clip runs %.1f seconds", n)
	}
	if !strings.Contains(said.String(), "kurz fitted") {
		t.Errorf("the fit was not said:\n%s", said.String())
	}

	// The same search again, the plan gone, reuses the reply and its fit.
	if err := os.Remove(path); err != nil {
		t.Fatal(err)
	}
	path, err = p.Plan(ctx, PlanRequest{Count: 1})
	if err != nil {
		t.Fatalf("plan again: %v %s", err, p.LastError())
	}
	if len(asks) != 2 {
		t.Errorf("a reused reply asked the model %d more times", len(asks)-2)
	}
	if _, clips, _ := LoadClips(path); len(clips) != 1 || clips[0].Duration() < 20 {
		t.Errorf("the reused reply was not fitted: %+v", clips)
	}
}
