package engine

import (
	"bytes"
	"context"
	"fmt"
	"os"
	"path/filepath"
	"testing"
)

// The default recipe asks exactly what was always asked, so a saved reply
// is found again and a search made before recipes existed reads the same.
func TestTheDefaultRecipeAsksWhatWasAlwaysAsked(t *testing.T) {
	r, err := RecipeNamed("")
	if err != nil {
		t.Fatal(err)
	}
	if r.Name != DefaultRecipe || r.System != SystemPrompt || r.Version != PromptVersion {
		t.Fatalf("the default recipe is %s, version %d", r.Name, r.Version)
	}
	lines := []Line{
		{Index: 1, Cues: []Cue{{Start: 0, End: 1, Text: "Hallo"}}},
		{Index: 2, Cues: []Cue{{Start: 1.8, End: 2.4, Text: "Welt."}}, GapBefore: 0.8},
	}
	opts := PlanOptions{Count: 3, MinLen: 20, MaxLen: 30}
	if got, want := r.Request(lines, r.units(lines), opts), buildPrompt(lines, opts); got != want {
		t.Errorf("the default recipe asks something new:\n%s\nrather than\n%s", got, want)
	}
	if got, want := r.Schema(2, 3), planSchema(2, 3); got != want {
		t.Error("the default recipe's answer has a new shape")
	}
}

func TestAnUnknownRecipeIsRefusedByName(t *testing.T) {
	if _, err := RecipeNamed("nonsense"); err == nil {
		t.Fatal("a recipe that does not exist was taken")
	}
	// Options that name one anyway are searched with the default, rather
	// than with nothing. The name is refused where it is given.
	if got := (PlanOptions{Recipe: "nonsense"}).recipe().Name; got != DefaultRecipe {
		t.Errorf("got %s", got)
	}
}

// A recipe may number runs of lines, sentences say. What the model answers
// in those numbers is read back into the lines they stand for.
func TestAnAnswerInUnitsBecomesLines(t *testing.T) {
	// Three units over seven lines: 1-2, 3-5, 6-7.
	units := [][2]int{{0, 1}, {2, 4}, {5, 6}}
	answer := map[string]any{"slug": "a", "title": "t", "reason": "r",
		"keep": []any{[]any{1, 1}, []any{3, 3}}}
	entry, problems, ok := readEntry(answer, 1, units)
	if !ok || len(problems) > 0 {
		t.Fatalf("not read: %v", problems)
	}
	if got := fmt.Sprint(entry.Keep); got != "[[1 2] [6 7]]" {
		t.Errorf("units 1 and 3 became lines %s", got)
	}
	// A unit that does not exist is not a line either.
	answer["keep"] = []any{[]any{1, 4}}
	if _, _, ok := readEntry(answer, 1, units); ok {
		t.Error("unit 4 of 3 was taken")
	}
}

func speech(start, gap float64, words ...string) Line {
	var cues []Cue
	at := start
	for _, w := range words {
		cues = append(cues, Cue{Start: at, End: at + 0.3, Text: w})
		at += 0.4
	}
	return Line{Cues: cues, GapBefore: gap}
}

// Sentences are runs of whole lines, one after the other, with none left
// out, ending where a line ends a sentence or before a long pause.
func TestStoriesNumbersSentences(t *testing.T) {
	lines := []Line{
		speech(0, 0, "Als", "ich", "klein", "war,"),
		speech(2, 0.5, "stand", "meine", "Oma", "in", "der", "Tür."),
		speech(5, 0.3, "Sie", "hatte", "einen", "Korb"),
		speech(8, 1.4, "und", "ich", "wusste", "es."),
	}
	for i := range lines {
		lines[i].Index = i + 1
	}
	units := sentenceUnits(lines)
	if got := fmt.Sprint(units); got != "[[0 1] [2 2] [3 3]]" {
		t.Fatalf("sentences %s", got)
	}
	written := writeSentences(lines, units)
	want := "(0:00) [1] Als ich klein war, stand meine Oma in der Tür. [2] Sie hatte einen Korb … [3] und ich wusste es."
	if written != want {
		t.Errorf("written as\n%s\nnot\n%s", written, want)
	}
}

// A search with another recipe is an experiment. Its plan goes in a folder
// of its own, the episode's plan and captions are left alone, nothing is
// rendered and nothing is recorded for training.
func TestAnExperimentKeepsToItself(t *testing.T) {
	source := testEpisode(t, "20")
	SetTrainingDir(t.TempDir())
	var heard, asked int32
	server := fakeModel(t, &asked)
	defer server.Close()
	e := NewEngine(NewLog(&bytes.Buffer{}, false, false))
	e.OpenRecognizer = func(string) (Recognizer, error) { return fakeRecognizer{&heard}, nil }

	base := DefaultOptions()
	base.LLMURL = server.URL
	base.ASRModel = t.TempDir()
	base.Recipe = "stories"
	p := NewProject(e, source, base)
	ctx := context.Background()

	captions := filepath.Join(p.WorkDir(), "captions")
	if err := os.MkdirAll(captions, 0o755); err != nil {
		t.Fatal(err)
	}
	kept := filepath.Join(captions, "01.srt")
	if err := os.WriteFile(kept, []byte("1\n"), 0o644); err != nil {
		t.Fatal(err)
	}

	plan, err := p.Plan(ctx, PlanRequest{Count: 1})
	if err != nil {
		t.Fatalf("plan: %v %s", err, p.LastError())
	}
	if want := filepath.Join(p.WorkDir(), "experiments", "stories", "clips.json"); plan != want {
		t.Errorf("the plan is %s, not %s", plan, want)
	}
	if _, err := os.Stat(plan); err != nil {
		t.Errorf("no plan was written: %v", err)
	}
	if _, err := os.Stat(filepath.Join(p.LogsDir(), "clips.json")); !os.IsNotExist(err) {
		t.Error("the experiment wrote the episode's own plan")
	}
	if _, err := os.Stat(kept); err != nil {
		t.Error("the experiment set the episode's captions aside")
	}
	if n := 0; readRecords(filepath.Join(TrainingDir(), "plans.jsonl"), func([]byte) { n++ }) == nil && n > 0 {
		t.Errorf("the experiment was recorded for training")
	}
	if asked != 1 {
		t.Errorf("the model was asked %d times", asked)
	}
}

// A comparison searches the same window once with each recipe, each as an
// experiment, the default recipe included, and writes a report that holds
// every one of them.
func TestAComparisonReportsEveryRecipe(t *testing.T) {
	source := testEpisode(t, "20")
	SetTrainingDir(t.TempDir())
	var heard, asked int32
	server := fakeModel(t, &asked)
	defer server.Close()
	e := NewEngine(NewLog(&bytes.Buffer{}, false, false))
	e.OpenRecognizer = func(string) (Recognizer, error) { return fakeRecognizer{&heard}, nil }

	opts := DefaultOptions()
	opts.Source = source
	opts.LLMURL = server.URL
	opts.ASRModel = t.TempDir()
	opts.Count = 1
	opts.Replan = true
	runs, report, err := e.Compare(context.Background(), opts, []string{"lines", "stories"})
	if err != nil {
		t.Fatal(err)
	}
	if len(runs) != 2 || asked != 2 {
		t.Fatalf("%d runs, the model asked %d times", len(runs), asked)
	}
	for _, run := range runs {
		if run.Failed != "" || len(run.Clips) != 1 || run.PromptChars == 0 {
			t.Errorf("%s: %+v", run.Recipe, run)
		}
		if want := filepath.Join(WorkDir(source), "experiments", run.Recipe); filepath.Dir(run.Plan) != want {
			t.Errorf("%s's plan is %s", run.Recipe, run.Plan)
		}
	}
	body, err := os.ReadFile(report)
	if err != nil {
		t.Fatal(err)
	}
	for _, want := range []string{"## lines", "## stories", "### 1. Erste"} {
		if !bytes.Contains(body, []byte(want)) {
			t.Errorf("the report has no %q:\n%s", want, body)
		}
	}
	if _, err := os.Stat(filepath.Join(WorkDir(source), "logs", "clips.json")); !os.IsNotExist(err) {
		t.Error("the comparison wrote the episode's own plan")
	}
}

// A comparison in which every search failed has nothing to report, and says
// so rather than pointing at a report of failures.
func TestAComparisonOfFailuresIsAFailure(t *testing.T) {
	source := testEpisode(t, "20")
	SetTrainingDir(t.TempDir())
	var heard int32
	e := NewEngine(NewLog(&bytes.Buffer{}, false, false))
	e.OpenRecognizer = func(string) (Recognizer, error) { return fakeRecognizer{&heard}, nil }

	opts := DefaultOptions()
	opts.Source = source
	opts.ASRModel = t.TempDir()
	opts.LLMModel = filepath.Join(t.TempDir(), "missing.gguf")
	opts.Count = 1
	opts.Replan = true
	runs, report, err := e.Compare(context.Background(), opts, []string{"lines", "stories"})
	if err == nil || report != "" {
		t.Fatalf("a comparison of two failures was written to %q", report)
	}
	if len(runs) != 2 || runs[0].Failed == "" || runs[1].Failed == "" {
		t.Errorf("runs %+v", runs)
	}
	if matches, _ := filepath.Glob(filepath.Join(WorkDir(source), "experiments", "compare-*.md")); len(matches) > 0 {
		t.Errorf("a report was written: %v", matches)
	}
}
