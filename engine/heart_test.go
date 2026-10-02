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
	"sync/atomic"
	"testing"
)

// spoken makes lines of whole sentences, each lasting the seconds given,
// a second of pause between two.
func spoken(seconds ...float64) []Line {
	var parts []any
	for i, s := range seconds {
		gap := 1.0
		if i == 0 {
			gap = 0
		}
		parts = append(parts, gap, s, fmt.Sprintf("Satz %d endet hier.", i+1))
	}
	return said(parts...)
}

// lasting measures a clip the simple way, every run from the start of its
// first line to the end of its last.
func lasting(lines []Line) func([][2]int) float64 {
	return func(keep [][2]int) float64 {
		total := 0.0
		for _, r := range keep {
			total += lines[r[1]-1].End() - lines[r[0]-1].Start()
		}
		return total
	}
}

func noneTaken(int) bool { return false }

// A clip is fitted to 20 to 30 seconds around its heart, and the heart
// itself is never cut.
func TestAClipIsFittedAroundItsHeart(t *testing.T) {
	// Ten sentences of 5 seconds, a second apart: line n runs from 6(n-1)
	// to 6(n-1)+5.
	lines := spoken(5, 5, 5, 5, 5, 5, 5, 5, 5, 5)
	for _, c := range []struct {
		name      string
		keep      string
		heart     [2]int
		want, did string
		taken     int
	}{
		// 1 to 8 runs 47 seconds. What runs on past the heart at 5 goes
		// first: 1 to 5 is 29.
		{"landing first", "[[1 8]]", [2]int{4, 5}, "[[1 5]]", "shortened", 0},
		// Nothing runs on past the heart at 8, so setup goes from the
		// start: 4 to 8 is 29.
		{"then setup", "[[1 8]]", [2]int{7, 8}, "[[4 8]]", "shortened", 0},
		// A run wholly after the heart goes at once. 1 to 3 is then 17
		// seconds, and at the start it takes in the sentence after it.
		{"a run after", "[[1 3] [5 9]]", [2]int{2, 3}, "[[1 4]]", "lengthened", 0},
		// The heart alone runs 35 seconds and stays whole.
		{"a long heart", "[[1 8]]", [2]int{3, 8}, "[[3 8]]", "shortened", 0},
		// 5 and 6 run 11 seconds. The sentences before come in: 3 to 6 is
		// 23.
		{"too short", "[[5 6]]", [2]int{5, 6}, "[[3 6]]", "lengthened", 0},
		// At the start there is nothing before, so it takes the ones after.
		{"at the start", "[[1 2]]", [2]int{1, 2}, "[[1 4]]", "lengthened", 0},
		// Line 4 is in another clip, so it grows after instead.
		{"next to a clip", "[[5 6]]", [2]int{5, 6}, "[[5 8]]", "lengthened", 4},
		// Within the length, it stays as the model kept it.
		{"fits", "[[2 5]]", [2]int{3, 4}, "[[2 5]]", "", 0},
		// No heart, no fitting.
		{"no heart", "[[1 8]]", [2]int{}, "[[1 8]]", "", 0},
		// A heart outside what was kept is kept too, and what lies between
		// stays left out.
		{"heart outside", "[[1 2]]", [2]int{4, 5}, "[[1 2] [4 5]]", "", 0},
	} {
		taken := noneTaken
		if c.taken > 0 {
			taken = func(n int) bool { return n == c.taken }
		}
		keep, did := fitToHeart(lines, parseRuns(t, c.keep), c.heart, 0, 20, 30, taken, lasting(lines))
		if fmt.Sprint(keep) != c.want || did != c.did {
			t.Errorf("%s: %s became %v %q, want %s %q", c.name, c.keep, keep, did, c.want, c.did)
		}
	}
}

// A step that would leave the clip further off the length than it was is
// not taken: a 14 second sentence is not dropped from a clip of 33 to
// leave one of 19 that has lost its setup.
func TestFittingStopsShortOfMakingItWorse(t *testing.T) {
	lines := spoken(14, 4, 4, 9)
	// 1 to 4 runs 34 seconds, the heart 2 to 4. Without the first it is 19.
	keep, did := fitToHeart(lines, [][2]int{{1, 4}}, [2]int{2, 4}, 0, 20, 30, noneTaken, lasting(lines))
	if fmt.Sprint(keep) != "[[2 4]]" && fmt.Sprint(keep) != "[[1 4]]" {
		t.Fatalf("became %v", keep)
	}
	if off := max(0, 20-lasting(lines)(keep), lasting(lines)(keep)-30); off > 4 {
		t.Errorf("%v %q is %.1f seconds off the length", keep, did, off)
	}
}

// The heart is read from the answer, and one that cannot be read is said
// and left out, the clip kept as the model gave it.
func TestTheHeartIsReadFromTheAnswer(t *testing.T) {
	read := func(answer string) ([]PlanEntry, []string) {
		var data map[string]any
		if err := json.Unmarshal([]byte(answer), &data); err != nil {
			t.Fatal(err)
		}
		entries, problems, err := ValidatePlan(data, lineUnits(20))
		if err != nil {
			t.Fatal(err)
		}
		return entries, problems
	}
	entries, problems := read(`{"clips": [{"slug": "a", "title": "A", "reason": "r", "heart": [4, 6], "keep": [[2, 8]]}]}`)
	if entries[0].Heart != [2]int{4, 6} || len(problems) > 0 {
		t.Errorf("heart %v, problems %v", entries[0].Heart, problems)
	}
	for _, bad := range []string{`[6, 4]`, `[0, 3]`, `[4, 30]`, `"4"`, `[4]`} {
		entries, problems := read(`{"clips": [{"slug": "a", "title": "A", "reason": "r", "heart": ` +
			bad + `, "keep": [[2, 8]]}]}`)
		if entries[0].Heart != [2]int{} || len(problems) != 1 || entries[0].Keep[0] != [2]int{2, 8} {
			t.Errorf("heart %s read as %v, problems %v", bad, entries[0].Heart, problems)
		}
	}
	// The brief tells the model the program keeps the length, so a change
	// to the brief that loses the paragraph about it is caught here.
	if strings.Contains(heartSystem, lengthParagraph) || !strings.Contains(heartSystem, heartLength) {
		t.Error("the heart brief still asks the model to keep the length")
	}
}

// A search with the heart recipe asks the model once, however far off the
// length its clip is, and the engine fits the clip.
func TestTheHeartRecipeAsksOnce(t *testing.T) {
	t.Parallel()
	source := testEpisode(t, "40")
	var mu sync.Mutex
	asks := 0
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		mu.Lock()
		asks++
		mu.Unlock()
		// A line of the test episode is about 14 seconds, so one is too
		// short for 20 to 30.
		writeLocalStream(w, `{"clips": [{"slug": "kurz", "title": "Kurz", "reason": "r", `+
			`"heart": [1, 1], "keep": [[1, 1]]}]}`, 11)
	}))
	defer server.Close()
	var heard int32
	e := NewEngine(NewLog(&bytes.Buffer{}, false, false))
	e.OpenRecognizer = func(string) (Recognizer, error) { return fakeRecognizer{&heard}, nil }
	base := DefaultOptions()
	base.LLMURL = server.URL
	base.ASRModel = t.TempDir()
	base.Width, base.Height = 360, 640
	base.Recipe = "heart"
	p := NewProject(e, source, base)
	path, err := p.Plan(context.Background(), PlanRequest{Count: 1})
	if err != nil {
		t.Fatalf("plan: %v %s", err, p.LastError())
	}
	if asks != 1 {
		t.Errorf("the model was asked %d times", asks)
	}
	_, clips, err := LoadClips(path)
	if err != nil || len(clips) != 1 {
		t.Fatalf("clips %v %v", clips, err)
	}
	if n := clips[0].Duration(); n < 20 || n > 30 {
		t.Errorf("the clip runs %.1f seconds", n)
	}
}

// A comparison counts every ask: lines asks again about a clip well off
// the length, heart fits it without asking, and the report says so.
func TestAComparisonCountsEveryAsk(t *testing.T) {
	t.Parallel()
	source := testEpisode(t, "40")
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		var request struct{ Messages []chatMessage }
		_ = json.NewDecoder(r.Body).Decode(&request)
		keep := "[[1, 1]]"
		if len(request.Messages) > 2 {
			keep = "[[1, 2]]"
		}
		writeLocalStream(w, `{"clips": [{"slug": "kurz", "title": "Kurz", "reason": "r", `+
			`"heart": [1, 1], "keep": `+keep+`}]}`, 11)
	}))
	defer server.Close()
	var heard int32
	e := NewEngine(NewLog(&bytes.Buffer{}, false, false))
	e.OpenRecognizer = func(string) (Recognizer, error) { return fakeRecognizer{&heard}, nil }
	opts := DefaultOptions()
	opts.Source = source
	opts.LLMURL = server.URL
	opts.ASRModel = t.TempDir()
	opts.Count = 1
	opts.Replan = true
	runs, report, err := e.Compare(context.Background(), opts, []string{"lines", "heart"})
	if err != nil {
		t.Fatal(err)
	}
	if len(runs) != 2 || runs[0].Use.Asks != 2 || runs[1].Use.Asks != 1 {
		t.Fatalf("runs %+v", runs)
	}
	for _, run := range runs {
		if len(run.Clips) != 1 || run.Clips[0].Seconds < 20 || run.Clips[0].Seconds > 30 {
			t.Errorf("%s: %+v", run.Recipe, run.Clips)
		}
	}
	body, err := os.ReadFile(report)
	if err != nil {
		t.Fatal(err)
	}
	// The table lines up: every row as long as its heads.
	var widths []int
	for _, line := range strings.Split(string(body), "\n") {
		if strings.HasPrefix(line, "| Recipe | Clips | Asks") {
			widths = append(widths, runeLen(line))
		} else if len(widths) > 0 && len(widths) < 4 && strings.HasPrefix(line, "|") {
			widths = append(widths, runeLen(line))
		}
	}
	if len(widths) != 4 || widths[1] != widths[0] || widths[2] != widths[0] || widths[3] != widths[0] {
		t.Errorf("the table does not line up, row widths %v:\n%s", widths, body)
	}
}

// A heart is whole sentences. One whose last line ends on a comma takes in
// the line that finishes the sentence, so the clip does not end there: the
// umbrella story's heart ended on "salienden Erinnerungen," once.
func TestAHeartIsWholeSentences(t *testing.T) {
	lines := said(
		0.0, 3.0, "Und dann ist der Regenschirm zersprungen.", // 1
		1.0, 4.0, "Das war eine der ersten echten Erinnerungen,", // 2
		0.5, 1.5, "die ich habe.", // 3
		1.0, 2.0, "Davor weiß ich nichts.", // 4
	)
	if got := wholeHeart(lines, [2]int{1, 2}); got != [2]int{1, 3} {
		t.Errorf("heart 1-2 became %v, not [1 3]", got)
	}
	if got := wholeHeart(lines, [2]int{3, 3}); got != [2]int{2, 3} {
		t.Errorf("heart 3 became %v, not [2 3]", got)
	}
	if got := wholeHeart(lines, [2]int{4, 9}); got != [2]int{} {
		t.Errorf("a heart past the end became %v", got)
	}
}

// A clip never starts after its opening: the setup goes no further, even
// when the clip then stays long, and a clip the model started after its
// own opening starts there.
func TestAClipKeepsItsOpening(t *testing.T) {
	lines := spoken(5, 5, 5, 5, 5, 5, 5, 5, 5, 5)
	for _, c := range []struct {
		name, keep, want, did string
		heart                 [2]int
		opening               int
	}{
		// Without an opening, setup goes until it fits: 6 to 10 is 29.
		{"no opening", "[[1 10]]", "[[6 10]]", "shortened", [2]int{9, 10}, 0},
		// With the opening at 3 it stops there, 47 seconds long.
		{"opening", "[[1 10]]", "[[3 10]]", "shortened", [2]int{9, 10}, 3},
		// Started after its opening, it starts there.
		{"started late", "[[5 8]]", "[[3 8]]", "", [2]int{7, 8}, 3},
		// An opening inside the heart is no opening.
		{"opening in the heart", "[[1 8]]", "[[4 8]]", "shortened", [2]int{7, 8}, 7},
	} {
		keep, did := fitToHeart(lines, parseRuns(t, c.keep), c.heart, c.opening, 20, 30, noneTaken,
			lasting(lines))
		if fmt.Sprint(keep) != c.want || did != c.did {
			t.Errorf("%s: %s became %v %q, want %s %q", c.name, c.keep, keep, did, c.want, c.did)
		}
	}
}

// The opening is read from the answer, one that cannot be read is said and
// left out, and heart-opening asks for it in every clip.
func TestTheOpeningIsReadFromTheAnswer(t *testing.T) {
	read := func(opening string) (PlanEntry, []string) {
		var data map[string]any
		answer := `{"clips": [{"slug": "a", "title": "A", "reason": "r", "opening": ` + opening +
			`, "heart": [4, 6], "keep": [[2, 8]]}]}`
		if err := json.Unmarshal([]byte(answer), &data); err != nil {
			t.Fatal(err)
		}
		entries, problems, err := ValidatePlan(data, lineUnits(20))
		if err != nil {
			t.Fatal(err)
		}
		return entries[0], problems
	}
	if entry, problems := read("3"); entry.Opening != 3 || len(problems) > 0 {
		t.Errorf("opening %d, problems %v", entry.Opening, problems)
	}
	for _, bad := range []string{"0", "30", "[3]"} {
		if entry, problems := read(bad); entry.Opening != 0 || len(problems) != 1 {
			t.Errorf("opening %s read as %d, problems %v", bad, entry.Opening, problems)
		}
	}
	var schema map[string]any
	if err := json.Unmarshal([]byte(heartOpeningSchema(20, 6)), &schema); err != nil {
		t.Fatalf("the schema is not JSON: %v", err)
	}
	if !strings.Contains(heartOpeningSchema(20, 6), `"opening", "heart"`) ||
		strings.Contains(heartSchema(20, 6), "opening") {
		t.Error("only heart-opening asks for the opening")
	}
	if err := json.Unmarshal([]byte(heartSchema(20, 6)), &schema); err != nil {
		t.Fatalf("the heart schema is not JSON: %v", err)
	}
}

// The thought is counted in tokens as well as characters, one a piece as
// llama-server sends them, so a comparison can say how much was thought.
func TestThoughtIsCountedInTokens(t *testing.T) {
	stream := "data: {\"choices\":[{\"delta\":{\"reasoning_content\":\"Die \"}}]}\n\n" +
		"data: {\"choices\":[{\"delta\":{\"reasoning_content\":\"Geschichte\"}}]}\n\n" +
		"data: {\"choices\":[{\"delta\":{\"content\":\"{}\"},\"finish_reason\":\"stop\"}]}\n\n" +
		"data: [DONE]\n\n"
	answer, err := readLocalStream(strings.NewReader(stream), nil)
	if err != nil {
		t.Fatal(err)
	}
	if answer.ReasoningTokens != 2 || answer.Reasoning != 14 {
		t.Errorf("%d tokens and %d characters of thought", answer.ReasoningTokens, answer.Reasoning)
	}
}

// Every side of a comparison asks a llama-server of its own, so none finds
// the request of the side before in its cache and reads it for nothing.
func TestEverySideOfAComparisonHasItsOwnServer(t *testing.T) {
	source := testEpisode(t, "20")
	var asked int32
	server := fakeModel(t, &asked)
	defer server.Close()
	var started, stopped atomic.Int32
	was := launch
	launch = func(_ *Engine, _ context.Context, _ LocalModel, _ int, _ string) (string, func(), error) {
		started.Add(1)
		var once sync.Once
		return server.URL, func() { once.Do(func() { stopped.Add(1) }) }, nil
	}
	t.Cleanup(func() {
		StopModels()
		launch = was
	})
	StopModels()
	model := filepath.Join(t.TempDir(), "model.gguf")
	if err := os.WriteFile(model, []byte("gguf"), 0o644); err != nil {
		t.Fatal(err)
	}
	var heard int32
	e := NewEngine(NewLog(&bytes.Buffer{}, false, false))
	e.OpenRecognizer = func(string) (Recognizer, error) { return fakeRecognizer{&heard}, nil }
	opts := DefaultOptions()
	opts.Source = source
	opts.LLMModel = model
	// Any program will do: the stand-in above is what starts.
	opts.LLMServer = "true"
	opts.ASRModel = t.TempDir()
	opts.Count = 1
	opts.Replan = true
	if _, _, err := e.Compare(context.Background(), opts, []string{"heart@1024", "heart@0", "lines"}); err != nil {
		t.Fatal(err)
	}
	if started.Load() != 3 || stopped.Load() < 2 {
		t.Errorf("three sides started %d servers and stopped %d", started.Load(), stopped.Load())
	}
}
