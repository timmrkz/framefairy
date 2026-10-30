package engine

import (
	"math"
	"strings"
	"testing"

	"framefairy/engine/timed"

	"github.com/spf13/afero"
)

const captionPlan = `{
  "source": "ep.mp4",
  "plan_id": "p1",
  "clips": [
    {"id": "01", "slug": "schulhof",
     "segments": [{"start": 59.9, "end": 66.0}]}
  ]
}`

// captionHeard says two sentences with a pause between, so the clip has a
// caption for each.
func captionHeard() []timed.Cue {
	return []timed.Cue{{Start: 60, End: 60.4, Text: "Da"}, {Start: 60.45, End: 61, Text: "stand"}, {Start: 61.05, End: 61.4, Text: "ein"},
		{Start: 61.45, End: 62.1, Text: "Typ"}, {Start: 62.15, End: 62.4, Text: "auf"}, {Start: 62.45, End: 63, Text: "Schulhof."},
		{Start: 64.2, End: 64.6, Text: "Und"}, {Start: 64.65, End: 65.1, Text: "dann"}, {Start: 65.15, End: 65.6, Text: "kam"}}
}

func captionPlanPath(t *testing.T) (afero.Fs, string) {
	t.Helper()
	return writePlanAt(t, captionPlan)
}

func cueWords(c CaptionView) string {
	var out []string
	for _, l := range c.Lines {
		for _, w := range l.Words {
			out = append(out, w.Text)
		}
	}
	return strings.Join(out, " ")
}

// Removing a caption removes every word in it, the way removing each of
// them would, and leaves every other caption as it was. Asked for by Tim:
// a caption block clicked on the clip timeline and taken out with delete.
func TestRemovingACaptionRemovesItsWordsAndNothingElse(t *testing.T) {
	fsys, path := captionPlanPath(t)
	tr := fromStored(captionHeard(), nil, 0, 0, nil)
	before, err := ClipCaptionsView(fsys, path, "01", tr, nil)
	if err != nil {
		t.Fatal(err)
	}
	if len(before.Captions) < 2 {
		t.Fatalf("the test needs two captions, the clip has %d", len(before.Captions))
	}
	gone := before.Captions[0]
	if err := RemoveCaption(fsys, path, "01", gone.First, tr, nil); err != nil {
		t.Fatal(err)
	}
	after, err := ClipCaptionsView(fsys, path, "01", tr, nil)
	if err != nil {
		t.Fatal(err)
	}
	if len(after.Captions) != len(before.Captions)-1 {
		t.Fatalf("%d captions before, %d after", len(before.Captions), len(after.Captions))
	}
	for i, c := range after.Captions {
		was := before.Captions[i+1]
		if cueWords(c) != cueWords(was) || math.Abs(c.Start-was.Start) > 1e-9 || math.Abs(c.End-was.End) > 1e-9 {
			t.Errorf("caption %d was %.2f-%.2f %q, is %.2f-%.2f %q", i+1, was.Start, was.End, cueWords(was), c.Start, c.End, cueWords(c))
		}
	}
	// Each word is removed as it would be on its own, so each comes back
	// on its own too: corrected again, it reads what it is given.
	first := gone.Lines[0].Words[0]
	if err := SetWordText(fsys, path, "01", *first.Said, "Hier", tr); err != nil {
		t.Fatal(err)
	}
	again, _ := ClipCaptionsView(fsys, path, "01", tr, nil)
	if !strings.HasPrefix(cueWords(again.Captions[0]), "Hier") {
		t.Errorf("the word put back reads %q", cueWords(again.Captions[0]))
	}
}

// A caption that is not there is refused, and nothing is written.
func TestRemovingACaptionThatIsNotThereIsRefused(t *testing.T) {
	fsys, path := captionPlanPath(t)
	tr := fromStored(captionHeard(), nil, 0, 0, nil)
	if err := RemoveCaption(fsys, path, "01", 63.5, tr, nil); err == nil {
		t.Error("a caption beginning where no word is was removed")
	}
	if body, _ := afero.ReadFile(fsys, path); string(body) != captionPlan {
		t.Errorf("a refused removal wrote the plan:\n%s", body)
	}
}

// Removing any one caption leaves every other caption as it was, its
// words and when it appears and goes. The caption before used to stay up
// through the time of the one removed, so its block on the clip timeline
// grew across the place, which read as the two merging. Found by Tim.
func TestRemovingAnyCaptionLeavesTheOthersAsTheyWere(t *testing.T) {
	text := "Ich habe ihn dort geschlagen und dann habe ich so Judostyle den Arm hochgerissen und dieser Regenschirm ist zersprungen."
	var heard []timed.Cue
	at := 60.0
	for _, w := range strings.Fields(text) {
		heard = append(heard, timed.Cue{Start: at, End: at + 0.3, Text: w})
		at += 0.35
	}
	tr := fromStored(heard, nil, 0, 0, nil)
	firstFS, firstPath := captionPlanPath(t)
	before, err := ClipCaptionsView(firstFS, firstPath, "01", tr, nil)
	if err != nil {
		t.Fatal(err)
	}
	if len(before.Captions) < 3 {
		t.Fatalf("the test needs three captions, the clip has %d", len(before.Captions))
	}
	for k := range before.Captions {
		fsys, path := captionPlanPath(t)
		if err := RemoveCaption(fsys, path, "01", before.Captions[k].First, tr, nil); err != nil {
			t.Fatal(err)
		}
		after, err := ClipCaptionsView(fsys, path, "01", tr, nil)
		if err != nil {
			t.Fatal(err)
		}
		var want []CaptionView
		want = append(want, before.Captions[:k]...)
		want = append(want, before.Captions[k+1:]...)
		if len(after.Captions) != len(want) {
			t.Fatalf("removing caption %d left %d captions, not %d", k, len(after.Captions), len(want))
		}
		for i, c := range after.Captions {
			was := want[i]
			if cueWords(c) != cueWords(was) || math.Abs(c.Start-was.Start) > 1e-9 || math.Abs(c.End-was.End) > 1e-9 {
				t.Errorf("removing caption %d: %.2f-%.2f %q became %.2f-%.2f %q", k, was.Start, was.End, cueWords(was), c.Start, c.End, cueWords(c))
			}
		}
	}
}
