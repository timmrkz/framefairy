package engine

import (
	"path/filepath"
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
