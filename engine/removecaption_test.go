package engine

import (
	"math"
	"os"
	"path/filepath"
	"strings"
	"testing"
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
func captionHeard() []Cue {
	return []Cue{{60, 60.4, "Da"}, {60.45, 61, "stand"}, {61.05, 61.4, "ein"},
		{61.45, 62.1, "Typ"}, {62.15, 62.4, "auf"}, {62.45, 63, "Schulhof."},
		{64.2, 64.6, "Und"}, {64.65, 65.1, "dann"}, {65.15, 65.6, "kam"}}
}

func captionPlanPath(t *testing.T) string {
	t.Helper()
	logs := filepath.Join(t.TempDir(), "ep.framefairy", "logs")
	if err := os.MkdirAll(logs, 0o755); err != nil {
		t.Fatal(err)
	}
	path := filepath.Join(logs, "clips.json")
	if err := os.WriteFile(path, []byte(captionPlan), 0o644); err != nil {
		t.Fatal(err)
	}
	return path
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
	path := captionPlanPath(t)
	logs := filepath.Dir(path)
	tr := fromStored(captionHeard(), nil, 0, 0, nil)
	before, err := ClipCaptionsView(path, "01", tr, nil)
	if err != nil {
		t.Fatal(err)
	}
	if len(before.Captions) < 2 {
		t.Fatalf("the test needs two captions, the clip has %d", len(before.Captions))
	}
	gone := before.Captions[0]
	if err := RemoveCaption(logs, path, "01", gone.First, tr, nil); err != nil {
		t.Fatal(err)
	}
	after, err := ClipCaptionsView(path, "01", tr, nil)
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
	if err := SetWordText(logs, *first.Said, "Hier", tr); err != nil {
		t.Fatal(err)
	}
	again, _ := ClipCaptionsView(path, "01", tr, nil)
	if !strings.HasPrefix(cueWords(again.Captions[0]), "Hier") {
		t.Errorf("the word put back reads %q", cueWords(again.Captions[0]))
	}
}

// A caption that is not there is refused, and nothing is written.
func TestRemovingACaptionThatIsNotThereIsRefused(t *testing.T) {
	path := captionPlanPath(t)
	logs := filepath.Dir(path)
	tr := fromStored(captionHeard(), nil, 0, 0, nil)
	if err := RemoveCaption(logs, path, "01", 63.5, tr, nil); err == nil {
		t.Error("a caption beginning where no word is was removed")
	}
	if _, err := os.Stat(correctionsPath(logs)); err == nil {
		t.Error("a refused removal wrote corrections")
	}
}
