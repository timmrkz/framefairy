package engine

import (
	"fmt"
	"path/filepath"
	"strings"
	"testing"

	"github.com/spf13/afero"
)

// Shift and an arrow walk the words heard in a cut of the chosen clip,
// because the caption lights nothing there. The captions of the clip
// carry them, with the word either side of each cut, so the window never
// asks for the transcript to walk them, issue #932. editableTranscript
// hears eins 10-10.5, zwei 10.6-11, drei 12-12.4, vier 13-13.5, fünf
// 20.1-20.6 and sechs 21-21.5.
func TestTheCaptionsOfAClipCarryTheWordsHeardInItsCuts(t *testing.T) {
	for _, c := range []struct {
		name     string
		segments string
		want     string
	}{
		{"a clip with no cut", `[{"start": 10.0, "end": 13.6}]`, ""},
		{"a cut with nothing heard in it", `[{"start": 10.0, "end": 11.1}, {"start": 11.9, "end": 13.6}]`,
			"zwei drei"},
		{"a cut over two words", `[{"start": 10.0, "end": 10.55}, {"start": 12.9, "end": 13.6}]`,
			"eins zwei drei vier"},
		{"a cut that ends inside a word", `[{"start": 10.0, "end": 10.55}, {"start": 12.2, "end": 13.6}]`,
			"eins zwei drei vier"},
		{"two cuts that share the word between them", `[{"start": 10.0, "end": 10.55}, {"start": 10.6, "end": 11.0}, {"start": 11.5, "end": 13.6}]`,
			"eins zwei drei"},
		{"two cuts far apart", `[{"start": 10.0, "end": 10.55}, {"start": 10.6, "end": 13.6}, {"start": 20.0, "end": 20.7}, {"start": 21.6, "end": 22.0}]`,
			"eins zwei vier fünf sechs"},
		{"a cut after the last word", `[{"start": 20.0, "end": 21.6}, {"start": 30.0, "end": 31.0}]`,
			"sechs"},
		{"pieces written out of order", `[{"start": 20.0, "end": 21.6}, {"start": 10.0, "end": 10.55}]`,
			""},
	} {
		t.Run(c.name, func(t *testing.T) {
			fsys := afero.NewMemMapFs()
			path := filepath.Join("/", "ep.framefairy", "logs", "clips.json")
			plan := `{"source": "ep.mp4", "plan_id": "p1", "clips": [{"id": "01", "slug": "eins", "segments": ` + c.segments + `}]}`
			if err := afero.WriteFile(fsys, path, []byte(plan), 0o644); err != nil {
				t.Fatal(err)
			}
			view, err := ClipCaptionsView(planAt(t, fsys, path), "01", editableTranscript(), nil)
			if err != nil {
				t.Fatal(err)
			}
			var got []string
			for _, w := range view.InCuts {
				got = append(got, w.Text)
			}
			if strings.Join(got, " ") != c.want {
				t.Errorf("in the cuts %q, want %q", got, c.want)
			}
			if view.InCuts == nil {
				t.Error("no list at all, which the window reads as no words heard yet")
			}
		})
	}
}

// The words in a cut are the words as they were heard, never as a
// correction of the clip reads, because the clip shows none of them, and
// they keep their times on the episode's clock.
func TestTheWordsInACutAreTheWordsHeard(t *testing.T) {
	fsys := afero.NewMemMapFs()
	path := filepath.Join("/", "ep.framefairy", "logs", "clips.json")
	tr := editableTranscript()
	plan := fmt.Sprintf(`{"source": "ep.mp4", "plan_id": "p1", "clips": [{"id": "01", "slug": "eins",
	  "segments": [{"start": 10.0, "end": 10.55}, {"start": 12.9, "end": 13.6}],
	  "corrections": {%q: "ZWEI"}}]}`, tr.keyAt(1))
	if err := afero.WriteFile(fsys, path, []byte(plan), 0o644); err != nil {
		t.Fatal(err)
	}
	view, err := ClipCaptionsView(planAt(t, fsys, path), "01", tr, nil)
	if err != nil {
		t.Fatal(err)
	}
	if fixed := tr.Of(planAt(t, fsys, path).AllClips()[0]); fixed.Words[1].Text != "ZWEI" {
		t.Fatalf("the correction is not one the clip reads: %q", fixed.Words[1].Text)
	}
	if len(view.InCuts) != 4 {
		t.Fatalf("in the cuts %+v, want 4 words", view.InCuts)
	}
	zwei := view.InCuts[1]
	if zwei.Text != "zwei" || zwei.Start != 10.6 || zwei.End != 11 {
		t.Errorf("the second word in the cut is %+v, want zwei from 10.6 to 11", zwei)
	}
}
