package engine

import (
	"path/filepath"
	"strings"
	"testing"
	"time"

	"github.com/spf13/afero"

	"framefairy/engine/num"
	"framefairy/engine/timed"
)

// The transcript search reads its cards again when Go's stamp of them
// changes, and only then, issue #933. So the stamp changes with every
// change the cards are made of, a transcript written, a clip set written
// or removed, by the app or from outside it, the look of the captions,
// and stays the same while nothing changed. The window decided this from
// what it held before, and missed a clip set changed outside the app.
func TestTheStampOfTheCardsChangesWithWhatTheyAreMadeOf(t *testing.T) {
	words := []timed.Cue{{Start: 0.5, End: 1.2, Text: "Hallo"}, {Start: 1.4, End: 2.0, Text: "Welt"}}
	fsys, source, transcript := storedTranscript(t, words, make([]float32, 300), 3, true)
	ep := NewMachine(fsys).Episode(source)
	logs := LogsDir(source)
	plan := filepath.Join(logs, "clips.json")
	// Every file is written at a moment of its own, set by hand, so the
	// test never depends on how fine the clock is.
	moment := time.Date(2026, 11, 19, 12, 0, 0, 0, time.UTC)
	touched := func(path string) {
		t.Helper()
		moment = moment.Add(time.Second)
		if err := fsys.Chtimes(path, moment, moment); err != nil {
			t.Fatal(err)
		}
	}
	writePlan := func(path string, clips int) {
		t.Helper()
		if err := afero.WriteFile(fsys, path, []byte(aRealisticPlan(clips)), 0o644); err != nil {
			t.Fatal(err)
		}
		touched(path)
	}
	writePlan(plan, 2)
	look := &Look{Style: map[string]any{"size": 80.0}}
	stamp := func() string {
		return ep.SaidStamp("/models/modell", nil, ep.Plans(), func(PlanSummary) *Look { return look })
	}

	seen := map[string]string{}
	saw := func(what string) {
		t.Helper()
		now := stamp()
		if again := stamp(); again != now {
			t.Fatalf("%s: asked twice with nothing changed, %q then %q", what, now, again)
		}
		for before, was := range seen {
			if was == now {
				t.Errorf("%s has the stamp %s had", what, before)
			}
		}
		seen[what] = now
	}

	saw("one clip set")
	writePlan(plan, 3)
	saw("the clip set written again")
	writePlan(filepath.Join(logs, "clips-0-1800.json"), 2)
	saw("a second clip set")
	look = &Look{Style: map[string]any{"size": 96.0}}
	saw("the captions in a bigger size")
	if err := fsys.Remove(filepath.Join(logs, "clips-0-1800.json")); err != nil {
		t.Fatal(err)
	}
	saw("the second clip set removed")

	at, err := stampOf(fsys, source)
	if err != nil {
		t.Fatal(err)
	}
	file := transcriptFile{Version: transcriptVersion, Source: at, Model: "modell",
		Mean: num.Float(-30), Parts: [][2]num.Float{{0, 13}}, End: num.Float(13), Words: storedWords(words)}
	if err := writeTranscript(fsys, transcript, file, make([]float32, 1300)); err != nil {
		t.Fatal(err)
	}
	touched(transcript)
	saw("the transcript heard further")
}

// The stamp changes with what the cards are made of, and only with that,
// found in the review of #934: a crop placed, a keyframe set, a colour
// chosen or the captions moved up read every card of the clip set again,
// with the search open, though no card changed. A clip set is written
// with one change to its first clip each time, and the stamp is compared
// with the one it had before.
func TestTheStampOfTheCardsStaysForWhatTheyAreNotMadeOf(t *testing.T) {
	t.Parallel()
	words := []timed.Cue{{Start: 0.5, End: 1.2, Text: "Hallo"}, {Start: 1.4, End: 2.0, Text: "Welt"}}
	base := &Look{Style: map[string]any{"size": 80.0, "primary": "&H00FFFFFF", "margin_v": 320.0}}
	in := func(change string) func(string) string {
		return func(plan string) string { return strings.Replace(plan, `"slug":"clip-01",`, `"slug":"clip-01",`+change, 1) }
	}
	for _, c := range []struct {
		name    string
		plan    func(string) string
		look    *Look
		changes bool
	}{
		{name: "a crop placed", plan: func(p string) string { return strings.Replace(p, `"crop_x":120`, `"crop_x":300`, 1) }},
		{name: "a keyframe set", plan: in(`"crop_marks":[{"at":1,"x":200,"glide":true}],`)},
		{name: "the clip rotated", plan: in(`"rotations":[{"from":0,"to":10,"deg":90}],`)},
		{name: "the captions moved up in one clip", plan: in(`"caption_y":480,`)},
		{name: "a title given", plan: in(`"title":"Ein Anfang",`)},
		{name: "a thumbnail chosen", plan: in(`"thumbnails":[3],`)},
		{name: "the captions in another colour",
			look: &Look{Style: map[string]any{"size": 80.0, "primary": "&H0000FFFF", "margin_v": 320.0}}},
		{name: "the captions higher",
			look: &Look{Style: map[string]any{"size": 80.0, "primary": "&H00FFFFFF", "margin_v": 640.0}}},
		{name: "a word corrected", plan: in(`"corrections":{"500":"Hey"},`), changes: true},
		{name: "the clip rejected", plan: in(`"rejected":true,`), changes: true},
		{name: "the clip trimmed", plan: func(p string) string { return strings.Replace(p, `"end":24.0`, `"end":12.0`, 1) }, changes: true},
		{name: "the captions in a smaller size",
			look: &Look{Style: map[string]any{"size": 64.0, "primary": "&H00FFFFFF", "margin_v": 320.0}}, changes: true},
	} {
		t.Run(c.name, func(t *testing.T) {
			t.Parallel()
			fsys, source, _ := storedTranscript(t, words, make([]float32, 300), 3, true)
			ep := NewMachine(fsys).Episode(source)
			path := filepath.Join(LogsDir(source), "clips.json")
			moment := time.Date(2026, 11, 19, 12, 0, 0, 0, time.UTC)
			write := func(body string) {
				t.Helper()
				if err := afero.WriteFile(fsys, path, []byte(body), 0o644); err != nil {
					t.Fatal(err)
				}
				moment = moment.Add(time.Second)
				if err := fsys.Chtimes(path, moment, moment); err != nil {
					t.Fatal(err)
				}
			}
			look := base
			stamp := func() string {
				return ep.SaidStamp("/models/modell", nil, ep.Plans(), func(PlanSummary) *Look { return look })
			}
			plan := aRealisticPlan(2)
			write(plan)
			before := stamp()
			if c.plan != nil {
				changed := c.plan(plan)
				if changed == plan {
					t.Fatal("the change is not in the clip set")
				}
				write(changed)
			}
			if c.look != nil {
				look = c.look
			}
			if len(ep.Plans()) != 1 {
				t.Fatal("the clip set written is not read as one")
			}
			after := stamp()
			if c.changes && after == before {
				t.Errorf("the stamp stayed %s, though the cards change", before)
			}
			if !c.changes && after != before {
				t.Errorf("the stamp went from %s to %s, though no card changes", before, after)
			}
		})
	}
}
