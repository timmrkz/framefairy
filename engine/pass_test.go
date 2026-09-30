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
)

// answering answers like llama-server with the answers given, one a
// request, and keeps what each request asked.
func answering(t *testing.T, answers ...string) (*httptest.Server, func() []string) {
	var mu sync.Mutex
	var asked []string
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		var body struct {
			Messages []struct {
				Content string `json:"content"`
			} `json:"messages"`
		}
		_ = json.NewDecoder(r.Body).Decode(&body)
		mu.Lock()
		n := len(asked)
		prompt := ""
		if len(body.Messages) > 0 {
			prompt = body.Messages[len(body.Messages)-1].Content
		}
		asked = append(asked, prompt)
		mu.Unlock()
		answer := answers[min(n, len(answers)-1)]
		chunk, _ := json.Marshal(map[string]any{"choices": []any{map[string]any{
			"delta": map[string]any{"content": answer}}}})
		w.Header().Set("content-type", "text/event-stream")
		fmt.Fprintf(w, "data: %s\n\n", chunk)
		fmt.Fprint(w, "data: {\"choices\":[{\"delta\":{},\"finish_reason\":\"stop\"}]}\n\n")
		fmt.Fprint(w, "data: [DONE]\n\n")
	}))
	t.Cleanup(server.Close)
	return server, func() []string {
		mu.Lock()
		defer mu.Unlock()
		return append([]string(nil), asked...)
	}
}

func clipAnswer(keeps ...string) string {
	var clips []string
	for i, k := range keeps {
		clips = append(clips, fmt.Sprintf(`{"slug": "c%d", "title": "Clip %d", "reason": "Test", "keep": %s}`, i+1, i+1, k))
	}
	return `{"clips": [` + strings.Join(clips, ", ") + `]}`
}

func passProject(t *testing.T, url string) *Project {
	t.Helper()
	source := testEpisode(t, "40")
	var heard int32
	e := NewEngine(NewLog(&bytes.Buffer{}, false, false))
	e.OpenRecognizer = func(string) (Recognizer, error) { return fakeRecognizer{&heard}, nil }
	base := DefaultOptions()
	base.LLMURL = url
	base.ASRModel = t.TempDir()
	base.Width, base.Height = 360, 640
	return NewProject(e, source, base)
}

// A window searched again is a second pass with a plan of its own: the
// clips of the first stay as they are, the model is told which lines are
// clips already, and a clip that keeps them all the same is left out.
func TestAWindowSearchedAgainKeepsEveryClip(t *testing.T) {
	t.Parallel()
	server, asked := answering(t, clipAnswer("[[1, 1]]"), clipAnswer("[[1, 1]]", "[[2, 2]]"))
	p := passProject(t, server.URL)
	req := PlanRequest{From: 10, To: 30, Count: 2, Min: 1, Max: 30}

	first, err := p.Search(context.Background(), req, nil)
	if err != nil {
		t.Fatalf("first search: %v", err)
	}
	second, err := p.Search(context.Background(), req, nil)
	if err != nil {
		t.Fatalf("second search: %v", err)
	}
	if filepath.Base(first) != "clips-10-30.json" || filepath.Base(second) != "clips-10-30-2.json" {
		t.Fatalf("the two passes wrote %s and %s", filepath.Base(first), filepath.Base(second))
	}
	if _, clips, err := LoadClips(first); err != nil || len(clips) != 1 || clips[0].ID != "t10-01" {
		t.Errorf("the first pass after the second: %v, %+v", err, clips)
	}
	_, clips, err := LoadClips(second)
	if err != nil || len(clips) != 1 || clips[0].ID != "t10-2-01" || clips[0].Title != "Clip 2" {
		t.Errorf("the second pass should hold only the new moment: %v, %+v", err, clips)
	}
	prompts := asked()
	if len(prompts) != 2 {
		t.Fatalf("the model was asked %d times", len(prompts))
	}
	if strings.Contains(prompts[0], "clips already") || strings.Contains(prompts[0], "a clip already") {
		t.Errorf("the first pass had nothing to leave, and was told to:\n%s", prompts[0])
	}
	if !strings.Contains(prompts[1], "Line 1 is in a clip already") {
		t.Errorf("the second pass was not told which line is taken:\n%s", prompts[1])
	}
}

// A search carried on writes to the plan it began, not to a pass of its
// own: the pass is in its record.
func TestASearchCarriedOnKeepsItsPass(t *testing.T) {
	t.Parallel()
	server, _ := answering(t, clipAnswer("[[1, 1]]"), clipAnswer("[[2, 2]]"))
	p := passProject(t, server.URL)
	req := PlanRequest{From: 10, To: 30, Count: 1, Min: 1, Max: 30}
	if _, err := p.Search(context.Background(), req, nil); err != nil {
		t.Fatal(err)
	}
	carried := req
	carried.Pass = 1
	plan, err := p.Search(context.Background(), carried, nil)
	if err != nil {
		t.Fatal(err)
	}
	if filepath.Base(plan) != "clips-10-30.json" {
		t.Errorf("a search carried on wrote %s", filepath.Base(plan))
	}
	if _, err := os.Stat(filepath.Join(p.LogsDir(), "clips-10-30-2.json")); err == nil {
		t.Error("a search carried on made a second pass")
	}
}

// A later pass over the whole episode is a window from its start to its
// end, so its plan says which.
func TestPassNames(t *testing.T) {
	for _, c := range []struct {
		window *Window
		pass   int
		want   string
	}{
		{nil, 0, "clips.json"},
		{nil, 1, "clips.json"},
		{&Window{10, 30}, 1, "clips-10-30.json"},
		{&Window{10, 30}, 3, "clips-10-30-3.json"},
		{&Window{0, 360}, 2, "clips-0-360-2.json"},
	} {
		if got := PassName(c.window, c.pass); got != c.want || !IsPlanFile(got) {
			t.Errorf("PassName(%v, %d) = %s, want %s", c.window, c.pass, got, c.want)
		}
	}
	rec := JobRecord{Pass: 2}
	if got := rec.PlanName(360); got != "clips-0-360-2.json" {
		t.Errorf("a second pass of a whole episode is %s", got)
	}
	// A record names the plan its search wrote: the whole episode asked
	// for by its end is clips.json, as the search writes it, and an edge
	// a hair under a second is named the way Run names it.
	for _, c := range []struct {
		rec  JobRecord
		want string
	}{
		{JobRecord{To: 360}, "clips.json"},
		{JobRecord{From: 9.9996, To: 39.9996}, "clips-10-40.json"},
	} {
		if got := c.rec.PlanName(360); got != c.want {
			t.Errorf("the record %+v names %s, want %s", c.rec, got, c.want)
		}
	}
}

func TestTakenLines(t *testing.T) {
	line := func(from, to float64) Line { return Line{Cues: []Cue{{Start: from, End: to}}} }
	lines := []Line{line(0, 2), line(2, 4), line(4, 6), line(6, 8), line(8, 10)}
	got := takenLines(lines, []Window{{1.5, 6}, {9.5, 12}})
	if fmt.Sprint(got) != "[[2 3]]" {
		t.Errorf("taken lines: %v", got)
	}
	if s := takenSentence([][2]int{{2, 3}, {7, 7}, {9, 12}}); s !=
		"Lines 2-3, 7 and 9-12 are in clips already. Find other moments, and keep none of those lines." {
		t.Errorf("sentence: %s", s)
	}
}
