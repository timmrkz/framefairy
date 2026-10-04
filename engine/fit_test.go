package engine

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"net/http"
	"net/http/httptest"
	"os"
	"slices"
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
	t.Parallel()
	source := testEpisode(t, "40")
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
	log := NewLog(&said, false, false)
	// The clip held back is on its way from the moment it is named, being
	// fitted, and keeps its card when it goes to be framed. And once the
	// answer is whole, the list says so.
	var steps []string
	whole := false
	log.SetSink(func(ev Event) {
		if ev.Kind != EventUnderway {
			return
		}
		mu.Lock()
		defer mu.Unlock()
		whole = whole || ev.Whole
		for _, u := range ev.Underway {
			steps = append(steps, fmt.Sprintf("%d:%s", u.N, u.Step))
		}
	})
	e := NewEngine(log)
	e.OpenRecognizer = func(string) (Recognizer, error) { return fakeRecognizer{&heard}, nil }
	base := DefaultOptions()
	base.LLMURL = server.URL
	// Only lines is asked again about a clip off the length.
	base.Recipe = "lines"
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
	mu.Lock()
	if len(steps) == 0 || steps[0] != "1:fitting" || !slices.Contains(steps, "1:framing") || !whole {
		t.Errorf("the clip on its way went %v, whole %v", steps, whole)
	}
	mu.Unlock()
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

// A recipe that edits asks about every clip a second time, not only the
// ones off the length, splits the thinking between the two asks, and takes
// the edit as long as it runs no further off the length.
func TestAnEditRecipeAsksAboutEveryClip(t *testing.T) {
	// Not beside the others: it adds a recipe to the package's list.
	edit := linesRecipe
	edit.Name, edit.Edit = "lines-edit", true
	recipes[edit.Name] = edit
	defer delete(recipes, edit.Name)

	source := testEpisode(t, "40")
	var mu sync.Mutex
	var asks []map[string]any
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		var request map[string]any
		_ = json.NewDecoder(r.Body).Decode(&request)
		mu.Lock()
		asks = append(asks, request)
		mu.Unlock()
		// Two lines of the test episode fit 20 to 30 seconds either way.
		keep := "[[1, 2]]"
		if len(request["messages"].([]any)) > 2 {
			keep = "[[2, 3]]"
		}
		writeLocalStream(w, `{"clips": [{"slug": "ganz", "title": "Ganz", "reason": "r", "keep": `+keep+`}]}`, 11)
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
	base.Recipe = edit.Name
	p := NewProject(e, source, base)

	path, err := p.Plan(context.Background(), PlanRequest{Count: 1})
	if err != nil {
		t.Fatalf("plan: %v %s", err, p.LastError())
	}
	if len(asks) != 2 {
		t.Fatalf("the model was asked %d times", len(asks))
	}
	for i, ask := range asks {
		// Half of what the whole 40 s episode, its window, may think.
		if got := ask["reasoning_budget_tokens"]; got != float64(SuggestedThink(40)/2) {
			t.Errorf("ask %d may think %v tokens", i+1, got)
		}
	}
	messages := asks[1]["messages"].([]any)
	last := messages[len(messages)-1].(map[string]any)["content"].(string)
	if !strings.Contains(last, "Now the edit") || !strings.Contains(last, `"ganz" runs`) {
		t.Errorf("the second ask is not the edit:\n%s", last)
	}
	plan, clips, err := LoadClips(path)
	if err != nil || len(clips) != 1 {
		t.Fatalf("clips %v %v", clips, err)
	}
	if list := plan.Raw["clips"].([]any); len(list) != 1 {
		t.Fatalf("%d clips", len(list))
	}
	if !strings.Contains(said.String(), "ganz edited, [[2, 3]] rather than [[1, 2]]") {
		t.Errorf("the edit was not taken:\n%s", said.String())
	}
}

// Asked about one clip, a model can answer with another clip of its first
// answer, the way Gemma gave the umbrella story when asked about the
// mirrors. That is no answer about the clip asked for, so it is asked for
// once more, alone. A reused reply replays both answers and asks nothing.
func TestAFitThatGivesAnotherClipIsAskedOnceMore(t *testing.T) {
	t.Parallel()
	source := testEpisode(t, "70")
	var mu sync.Mutex
	var asks [][]chatMessage
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		var request struct{ Messages []chatMessage }
		_ = json.NewDecoder(r.Body).Decode(&request)
		mu.Lock()
		asks = append(asks, request.Messages)
		mu.Unlock()
		// A line of the test episode is about 14 seconds. "kurz" is too
		// short, "andere" fits.
		answer := `{"clips": [{"slug": "kurz", "title": "Kurz", "reason": "r", "keep": [[1, 1]]}, ` +
			`{"slug": "andere", "title": "Andere", "reason": "r", "keep": [[3, 4]]}]}`
		if n := len(request.Messages); n > 2 {
			answer = `{"clips": [{"slug": "andere", "title": "Andere", "reason": "r", "keep": [[2, 4]]}]}`
			if strings.Contains(request.Messages[n-1].Content, `Give only "kurz", no other clip.`) {
				answer = `{"clips": [{"slug": "kurz", "title": "Kurz", "reason": "r", "keep": [[1, 2]]}]}`
			}
		}
		writeLocalStream(w, answer, 11)
	}))
	defer server.Close()
	var heard int32
	var said bytes.Buffer
	e := NewEngine(NewLog(&said, false, false))
	e.OpenRecognizer = func(string) (Recognizer, error) { return fakeRecognizer{&heard}, nil }
	base := DefaultOptions()
	base.LLMURL = server.URL
	// Only lines is asked again about a clip off the length.
	base.Recipe = "lines"
	base.ASRModel = t.TempDir()
	base.Width, base.Height = 360, 640
	p := NewProject(e, source, base)
	ctx := context.Background()

	path, err := p.Plan(ctx, PlanRequest{Count: 2})
	if err != nil {
		t.Fatalf("plan: %v %s", err, p.LastError())
	}
	if len(asks) != 3 {
		t.Fatalf("the model was asked %d times, want the search, the fit and once more", len(asks))
	}
	if !strings.Contains(said.String(), `kurz was not in the answer, which gave "andere", so it is asked for once more`) {
		t.Errorf("the other clip in the answer was not said:\n%s", said.String())
	}
	check := func(path string) {
		t.Helper()
		_, clips, err := LoadClips(path)
		if err != nil || len(clips) != 2 {
			t.Fatalf("clips %v %v", clips, err)
		}
		for _, c := range clips {
			if n := c.Duration(); n < 20 || n > 30 {
				t.Errorf("%s runs %.1f seconds", c.ID, n)
			}
		}
	}
	check(path)

	// The same search again, the plan gone, reuses the reply and both of
	// its fits.
	if err := os.Remove(path); err != nil {
		t.Fatal(err)
	}
	path, err = p.Plan(ctx, PlanRequest{Count: 2})
	if err != nil {
		t.Fatalf("plan again: %v %s", err, p.LastError())
	}
	if len(asks) != 3 {
		t.Errorf("a reused reply asked the model %d more times", len(asks)-3)
	}
	check(path)
}

// An answer about a clip is the one with its slug, or the one in its place
// when the model renamed it, but never one that carries another clip's slug.
func TestFitForTakesOnlyAnAnswerAboutTheClip(t *testing.T) {
	slugs := map[string]bool{"spiegel": true, "regenschirm": true}
	asked := PlanEntry{Slug: "spiegel"}
	for _, c := range []struct {
		name  string
		again []string
		want  string
	}{
		{"its own slug", []string{"regenschirm", "spiegel"}, "spiegel"},
		{"renamed, in its place", []string{"spiegel-flur"}, "spiegel-flur"},
		{"another clip in its place", []string{"regenschirm"}, ""},
		{"nothing", nil, ""},
	} {
		var again []PlanEntry
		for _, slug := range c.again {
			again = append(again, PlanEntry{Slug: slug})
		}
		got, ok := fitFor(asked, 0, again, slugs)
		if ok != (c.want != "") || got.Slug != c.want {
			t.Errorf("%s: took %q (%v), want %q", c.name, got.Slug, ok, c.want)
		}
	}
}
