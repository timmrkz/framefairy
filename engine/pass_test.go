package engine

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"net/http"
	"net/http/httptest"
	"path/filepath"
	"strings"
	"sync"
	"testing"

	"framefairy/engine/timed"
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

// clipAnswer is a story a line, as middle answers: where it starts, a
// line inside it and where it ends, all that line.
func clipAnswer(lines ...int) string {
	var b strings.Builder
	for _, n := range lines {
		fmt.Fprintf(&b, "%d %d %d\n", n, n, n)
	}
	return b.String()
}

// On the real disk: ffmpeg reads the episode, and the project works beside it.
func passProject(t *testing.T, url string) *Project {
	t.Helper()
	source := testEpisode(t, "40")
	var heard int32
	e := NewEngine(NewLog(&bytes.Buffer{}, false, false), disk)
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
//
// On the real disk: ffmpeg reads the episode the search hears.
func TestAWindowSearchedAgainKeepsEveryClip(t *testing.T) {
	t.Parallel()
	server, asked := answering(t, clipAnswer(1), clipAnswer(1, 2))
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
	if _, clips, err := LoadClips(disk, first); err != nil || len(clips) != 1 || clips[0].ID != "t10-01" {
		t.Errorf("the first pass after the second: %v, %+v", err, clips)
	}
	_, clips, err := LoadClips(disk, second)
	if err != nil || len(clips) != 1 || clips[0].ID != "t10-2-01" {
		t.Errorf("the second pass should hold only the new moment: %v, %+v", err, clips)
	}
	prompts := asked()
	if len(prompts) != 2 {
		t.Fatalf("the model was asked %d times", len(prompts))
	}
	if strings.Contains(prompts[0], "none of them in") {
		t.Errorf("the first pass had nothing to leave, and was told to:\n%s", prompts[0])
	}
	if !strings.Contains(prompts[1], "none of them in line 1.") {
		t.Errorf("the second pass was not told which line is taken:\n%s", prompts[1])
	}
}

// A search carried on writes to the plan it began, not to a pass of its
// own: the pass is in its record.
func TestASearchCarriedOnKeepsItsPass(t *testing.T) {
	t.Parallel()
	server, _ := answering(t, clipAnswer(1), clipAnswer(2))
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
	if _, err := p.Files.Stat(filepath.Join(p.LogsDir(), "clips-10-30-2.json")); err == nil {
		t.Error("a search carried on made a second pass")
	}
}

// A later pass over the whole episode is a window from its start to its
// end, so its plan says which.
func TestPassNames(t *testing.T) {
	for _, c := range []struct {
		window *timed.Window
		pass   int
		want   string
	}{
		{nil, 0, "clips.json"},
		{nil, 1, "clips.json"},
		{&timed.Window{Start: 10, End: 30}, 1, "clips-10-30.json"},
		{&timed.Window{Start: 10, End: 30}, 3, "clips-10-30-3.json"},
		{&timed.Window{Start: 0, End: 360}, 2, "clips-0-360-2.json"},
	} {
		if got := PassName(c.window, c.pass); got != c.want || !IsPlanFile(got) {
			t.Errorf("PassName(%v, %d) = %s, want %s", c.window, c.pass, got, c.want)
		}
	}
	rec := JobRecord{Pass: 2}
	if got := rec.planName(360); got != "clips-0-360-2.json" {
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
		if got := c.rec.planName(360); got != c.want {
			t.Errorf("the record %+v names %s, want %s", c.rec, got, c.want)
		}
	}
}

func TestTakenLines(t *testing.T) {
	line := func(from, to float64) Line { return Line{Cues: []timed.Cue{{Start: from, End: to}}} }
	lines := []Line{line(0, 2), line(2, 4), line(4, 6), line(6, 8), line(8, 10)}
	got := takenLines(lines, []timed.Window{{Start: 1.5, End: 6}, {Start: 9.5, End: 12}})
	if fmt.Sprint(got) != "[[2 3]]" {
		t.Errorf("taken lines: %v", got)
	}
	if s := takenSentence([][2]int{{2, 3}, {7, 7}, {9, 12}}); s !=
		"Lines 2-3, 7 and 9-12 are in clips already. Find other moments, and keep none of those lines." {
		t.Errorf("sentence: %s", s)
	}
}

// --next searches the window the app would place, the whole of a video
// this short, and run again it searches the same window as a second pass
// of its own, told which line is a clip already, the way Search does in
// the app. Asked with --from or --clips too, it says it places the window
// itself.
//
// On the real disk: ffmpeg reads the episode the run hears.
func TestTheCommandLineSearchesTheWindowTheAppWouldPlace(t *testing.T) {
	t.Parallel()
	server, asked := answering(t, clipAnswer(1), clipAnswer(1, 2))
	p := passProject(t, server.URL)
	opts := p.Base
	opts.Next, opts.PlanOnly, opts.Count, opts.Min, opts.Max = true, true, 1, 1, 30
	for run := 1; run <= 2; run++ {
		if code := p.engine.Run(context.Background(), opts); code != 0 {
			t.Fatalf("run %d ended with %d", run, code)
		}
	}
	for _, name := range []string{"clips.json", "clips-0-40-2.json"} {
		if !exists(disk, filepath.Join(p.LogsDir(), name)) {
			t.Errorf("no plan %s after two runs", name)
		}
	}
	prompts := asked()
	if len(prompts) != 2 || !strings.Contains(prompts[1], "none of them in line 1.") {
		t.Errorf("the second run was not told which line is taken, asked %d times", len(prompts))
	}

	for _, set := range []func(*Options){
		func(o *Options) { o.From = "10" },
		func(o *Options) { o.ClipsPath = filepath.Join(t.TempDir(), "clips.json") },
	} {
		bad := opts
		set(&bad)
		if err := p.engine.execute(context.Background(), bad); err == nil || !strings.Contains(err.Error(), "--next") {
			t.Errorf("--next with --from or --clips: %v", err)
		}
	}
}

// A video searched in part is searched on where it was not, the way the
// app places its next window.
//
// On the real disk: ffmpeg reads the episode, and the plans are written
// beside it.
func TestTheCommandLineSearchesOnWhereTheVideoWasNotSearched(t *testing.T) {
	t.Parallel()
	server, _ := answering(t, clipAnswer(1))
	p := passProject(t, server.URL)
	opts := p.Base
	opts.PlanOnly, opts.Count, opts.Min, opts.Max = true, 1, 1, 30
	first := opts
	first.From, first.To = "0", "20"
	if code := p.engine.Run(context.Background(), first); code != 0 {
		t.Fatalf("the search of 0 to 20 ended with %d", code)
	}
	opts.Next = true
	if code := p.engine.Run(context.Background(), opts); code != 0 {
		t.Fatalf("the search with --next ended with %d", code)
	}
	if !exists(disk, filepath.Join(p.LogsDir(), "clips-20-40.json")) {
		t.Error("--next after a search of 0 to 20 did not search 20 to 40")
	}
}
