package engine

import (
	"bufio"
	"encoding/json"
	"fmt"
	"math"
	"os"
	"os/exec"
	"path/filepath"
	"runtime"
	"strings"
	"sync"
	"testing"
	"time"
)

// Every edit the app makes goes through editPlan, and the app has no save
// button, so a failed edit must leave the file exactly as it was and a
// successful one must leave a plan that still renders.

const editablePlan = `{
  "source": "ep.mp4",
  "plan_id": "p1",
  "custom": {"kept": "yes"},
  "clips": [
    {"id": "01", "slug": "eins", "note": "mine",
     "segments": [{"start": 10.0, "end": 11.1, "crop_x": 120},
                  {"start": 11.9, "end": 13.1, "crop_x": 120}],
     "words": [[10.0, 10.5, "eins"], [10.6, 11.0, "zwei"], [12.0, 12.4, "drei"]]},
    {"id": "02", "slug": "zwei",
     "segments": [{"start": 20.0, "end": 22.0}],
     "words": [[20.1, 20.6, "vier"]]}
  ]
}`

// editableTranscript is the episode behind editablePlan, made the way a
// transcript is read, so a word of it can be corrected.
func editableTranscript() *Transcript {
	return fromStored([]Cue{
		{10, 10.5, "eins"}, {10.6, 11, "zwei"}, {12, 12.4, "drei"}, {13, 13.5, "vier"},
		{20.1, 20.6, "fünf"}, {21, 21.5, "sechs"},
	}, nil, 0, 0, nil)
}

func editablePlanPath(t *testing.T) string {
	t.Helper()
	dir := filepath.Join(t.TempDir(), "ep.framefairy", "logs")
	if err := os.MkdirAll(dir, 0o755); err != nil {
		t.Fatal(err)
	}
	path := filepath.Join(dir, "clips.json")
	if err := os.WriteFile(path, []byte(editablePlan), 0o644); err != nil {
		t.Fatal(err)
	}
	return path
}

func TestAFailedEditLeavesThePlanAlone(t *testing.T) {
	path := editablePlanPath(t)
	tr := editableTranscript()

	failures := []struct {
		name string
		run  func() error
	}{
		{"a clip that is not there", func() error { return SetRejected(path, "99", true) }},
		{"a reason nobody knows", func() error { return SetRejected(path, "01", true, "langweilig") }},
		{"a clip under a second", func() error { return TrimClip(path, "01", 12, 12.2, tr, 0.1, ToWords) }},
		{"a crop to the left of the frame", func() error { return SetCrop(path, "01", 10.5, -4) }},
	}
	for _, c := range failures {
		t.Run(c.name, func(t *testing.T) {
			before, _ := os.ReadFile(path)
			if err := c.run(); err == nil {
				t.Fatalf("the edit was accepted")
			}
			after, err := os.ReadFile(path)
			if err != nil || string(after) != string(before) {
				t.Errorf("the plan changed:\n%s", after)
			}
		})
	}
	// A half-written plan is never left behind either.
	entries, _ := os.ReadDir(filepath.Dir(path))
	for _, entry := range entries {
		if strings.HasPrefix(entry.Name(), ".clips-") {
			t.Errorf("a temporary plan was left behind: %s", entry.Name())
		}
	}
}

func TestEveryEditRaisesTheRevision(t *testing.T) {
	path := editablePlanPath(t)
	tr := editableTranscript()
	revision := func() int {
		plan, _, err := LoadClips(path)
		if err != nil {
			t.Fatal(err)
		}
		n, _ := toInt(plan.Raw["revision"])
		return n
	}
	if revision() != 0 {
		t.Fatalf("a fresh plan starts at %d", revision())
	}
	steps := []func() error{
		func() error { return SetRejected(path, "02", true) },
		func() error { return TrimClip(path, "01", 10, 12.4, tr, 0.1, ToWords) },
		func() error { return SetCrop(path, "01", 10.5, 200) },
		func() error { return ResetCrop(path, "01", 10.5) },
	}
	for i, step := range steps {
		if err := step(); err != nil {
			t.Fatalf("step %d: %v", i, err)
		}
		if revision() != i+1 {
			t.Errorf("after step %d the revision is %d", i, revision())
		}
	}
	// The fields the loader knows nothing about are still there at the end.
	body, _ := os.ReadFile(path)
	for _, want := range []string{`"custom"`, `"note": "mine"`, `"source": "ep.mp4"`} {
		if !strings.Contains(string(body), want) {
			t.Errorf("%s was lost:\n%s", want, body)
		}
	}
}

// What a gesture shows while the hand moves is what it saves when the hand
// lets go: the pieces, and the captions the clip has with them. If the two
// differ, the clip timeline jumps as the hand lets go, which is the thing
// the gesture is there to stop. And showing one writes nothing.
func TestAShapeIsWhatTheGestureSaves(t *testing.T) {
	tr := editableTranscript()
	for _, c := range []struct {
		name string
		g    Gesture
	}{
		{"the start to words", Gesture{Kind: "trim", Edge: "start", From: 9.7, ToWords: true, Frame: 0.04}},
		{"the end to words", Gesture{Kind: "trim", Edge: "end", From: 13.6, ToWords: true, Frame: 0.04}},
		{"the start to frames", Gesture{Kind: "trim", Edge: "start", From: 10.23, Frame: 0.04}},
		{"the end to frames", Gesture{Kind: "trim", Edge: "end", From: 12.87, Frame: 0.04}},
		{"a cut to frames", Gesture{Kind: "cut", From: 10.51, To: 10.58, Frame: 0.04}},
		{"a cut to words", Gesture{Kind: "cut", From: 10.7, To: 10.8, ToWords: true, Frame: 0.04}},
		{"a cut moved", Gesture{Kind: "move", Index: 0, From: 11.0, To: 11.95, Frame: 0.04}},
		{"a cut put back", Gesture{Kind: "join", From: 11.5}},
	} {
		t.Run(c.name, func(t *testing.T) {
			path := editablePlanPath(t)
			before, _ := os.ReadFile(path)
			shown, err := ShapeClipView(path, "01", c.g, tr, 0.1, nil)
			if err != nil {
				t.Fatal(err)
			}
			if after, _ := os.ReadFile(path); string(after) != string(before) {
				t.Fatal("showing a gesture changed the plan")
			}
			if err := Reshape(path, "01", c.g, tr, 0.1); err != nil {
				t.Fatal(err)
			}
			_, clips, err := LoadClips(path)
			if err != nil {
				t.Fatal(err)
			}
			var saved []PieceView
			for _, seg := range clips[0].Segments {
				saved = append(saved, PieceView{seg.Start, seg.End})
			}
			if fmt.Sprint(shown.Pieces) != fmt.Sprint(saved) {
				t.Errorf("shown %v, saved %v", shown.Pieces, saved)
			}
			landed, err := ClipCaptionsView(path, "01", tr, nil)
			if err != nil {
				t.Fatal(err)
			}
			// As JSON, which is what the interface gets, and which reads what
			// a pointer points at rather than where it is.
			a, _ := json.Marshal(shown.Captions.Captions)
			b, _ := json.Marshal(landed.Captions)
			if string(a) != string(b) {
				t.Errorf("captions shown %+v\nsaved %+v", shown.Captions.Captions, landed.Captions)
			}
		})
	}
	// A gesture that is not one is refused, and nothing is written.
	path := editablePlanPath(t)
	before, _ := os.ReadFile(path)
	for _, g := range []Gesture{
		{Kind: "trim", Edge: "start", From: math.NaN()},
		{Kind: "trim", Edge: "middle", From: 10},
		{Kind: "fold", From: 10},
		{Kind: "move", Index: 5, From: 10, To: 11},
		{Kind: "join", From: 20},
	} {
		if _, err := ShapeClip(path, "01", g, tr, 0.1); err == nil {
			t.Errorf("%+v was shown", g)
		}
		if err := Reshape(path, "01", g, tr, 0.1); err == nil {
			t.Errorf("%+v was saved", g)
		}
	}
	if after, _ := os.ReadFile(path); string(after) != string(before) {
		t.Error("a refused gesture changed the plan")
	}
}

// A word an edge cuts into is captioned exactly while it is said: while the
// clip holds some of its sound. It used to need the whole word inside the
// clip, so a word the clip said had no caption until the edge passed its
// start.
func TestAWordAnEdgeCutsIntoIsCaptionedWhileItIsSaid(t *testing.T) {
	tr := editableTranscript()
	for _, c := range []struct {
		name  string
		start float64
		said  bool
	}{
		// "eins" runs 10 to 10.5.
		{"edge inside the word", 10.24, true},
		{"edge at its last sound", 10.45, true},
		{"edge past it", 10.49, false},
	} {
		t.Run(c.name, func(t *testing.T) {
			path := editablePlanPath(t)
			if err := TrimClip(path, "01", c.start, 13.1, tr, 0.1, ToFrames); err != nil {
				t.Fatal(err)
			}
			view, err := ClipCaptionsView(path, "01", tr, nil)
			if err != nil {
				t.Fatal(err)
			}
			if got := strings.Contains(fmt.Sprint(view.Captions), "eins"); got != c.said {
				t.Errorf("eins is captioned: %v, said: %v\n%+v", got, c.said, view.Captions)
			}
			// The caption shows from the clip's first frame, and the word in
			// it keeps its own time, before that frame, so the parts of a
			// hyphenated word stay where they are said wherever the edge is.
			if c.said {
				first := view.Captions[0]
				if first.Start != 0 {
					t.Errorf("the caption appears at %v, not at the clip's first frame", first.Start)
				}
				if got := first.Lines[0].Words[0].Start; math.Abs(got-(10-c.start)) > 0.002 {
					t.Errorf("eins starts at %v on the clip's clock, want %v", got, 10-c.start)
				}
			}
		})
	}
}

func TestClipCaptionsComeBackOnTheClipClock(t *testing.T) {
	path := editablePlanPath(t)
	tr := editableTranscript()
	view, err := ClipCaptionsView(path, "01", tr, nil)
	if err != nil {
		t.Fatal(err)
	}
	// The cut between the two pieces lies in the second of quiet between
	// zwei and drei, and a cut takes out what is in it and nothing else: the
	// pause still parts the two captions the episode has there. They were
	// one, because the pause was measured on the clip's clock, where the
	// cut had made it short.
	if len(view.Captions) != 2 {
		t.Fatalf("captions %+v", view.Captions)
	}
	caption := view.Captions[0]
	if caption.Start != 0 || caption.End <= caption.Start {
		t.Errorf("caption runs %v to %v", caption.Start, caption.End)
	}
	// It is held after zwei until the cut begins, 1.1 into the clip, where
	// the episode would still have held it.
	if math.Abs(caption.End-1.1) > 0.002 {
		t.Errorf("the first caption goes at %v, want where the cut begins, 1.1", caption.End)
	}
	if got := fmt.Sprint(captionWords(caption)); got != "[eins zwei]" {
		t.Errorf("first caption %v", got)
	}
	second := view.Captions[1]
	// The clip says four words: the second piece ends a tenth into vier,
	// and a word is said while the clip holds some of its sound.
	if got := fmt.Sprint(captionWords(second)); got != "[drei vier]" {
		t.Errorf("second caption %v", got)
	}
	// The cut between the two pieces is gone from the clock, so the third
	// word sits a good deal earlier than in the episode.
	if drei := second.Lines[0].Words[0]; drei.Start > 2 || drei.Start <= caption.Lines[0].Words[0].Start {
		t.Errorf("drei starts at %v, so the cut is still in", drei.Start)
	}

	// The look comes as shares of the frame height and as web colours.
	s := view.Style
	if s.Size <= 0 || s.Size > 1 || s.MarginV <= 0 || s.MarginV > 1 {
		t.Errorf("style %+v", s)
	}
	if s.HighlightColour != "rgba(148, 33, 146, 1)" || s.Primary != "rgba(255, 255, 255, 1)" {
		t.Errorf("colours %+v", s)
	}

	if _, err := ClipCaptionsView(path, "99", tr, nil); err == nil {
		t.Error("a clip that is not in the plan was accepted")
	}
}

// The face and the size of the captions belong to a whole clip set, and
// they are changed where the work happens, in the workspace.
func TestSetCaptionStyleTakesOnlyValuesTheRenderWouldKeep(t *testing.T) {
	path := editablePlanPath(t)
	if err := SetCaptionStyle(path, map[string]any{"font": "Anton", "size": 72.0}); err != nil {
		t.Fatal(err)
	}
	plan, _, err := LoadClips(path)
	if err != nil {
		t.Fatal(err)
	}
	if s := ResolveStyle(plan.CaptionStyle()); s.Font != "Anton" || s.Size != 72 {
		t.Errorf("the style came back as %+v", s)
	}
	// The switches of the captions column, text and box, are on or off.
	if err := SetCaptionStyle(path, map[string]any{"text": false, "box": false}); err != nil {
		t.Fatal(err)
	}
	if plan, _, err := LoadClips(path); err != nil {
		t.Fatal(err)
	} else if s := ResolveStyle(plan.CaptionStyle()); s.Text || s.Box {
		t.Errorf("switched off, the style came back as %+v", s)
	}
	if err := SetCaptionStyle(path, map[string]any{"text": true, "box": true}); err != nil {
		t.Fatal(err)
	}

	for _, bad := range []map[string]any{
		{"size": 4.0},
		{"size": 900.0},
		{"size": "gross"},
		{"font": "Fett,Bold"},
		{"font": "a\nStyle: b"},
		{"font": ""},
		{"unknown": 1.0},
		{"size": nil},
		{"primary": "#FFFFFF"},
		{"primary": "&H00FFFFFF\nStyle: b"},
		{"back_colour": "&H80000"},
		{"back_colour": 5.0},
		{"highlight_colour": "red"},
		{"highlight_colour": "#942192\nStyle: b"},
		{"highlight_colour": "&H922194&"},
		{"text": 0.0},
		{"box": "off"},
	} {
		if err := SetCaptionStyle(path, bad); err == nil {
			t.Errorf("%v was accepted", bad)
		}
	}

	// Nothing of the refused edits reached the file, and what was set stays.
	plan, _, err = LoadClips(path)
	if err != nil {
		t.Fatal(err)
	}
	if s := ResolveStyle(plan.CaptionStyle()); s.Font != "Anton" || s.Size != 72 {
		t.Errorf("the style came back as %+v", s)
	}
	// And the clips are still there, with the fields nobody knows.
	body, _ := os.ReadFile(path)
	for _, want := range []string{`"custom"`, `"note": "mine"`, `"caption_style"`} {
		if !strings.Contains(string(body), want) {
			t.Errorf("%s is missing:\n%s", want, body)
		}
	}
}

// The caption line of a clip is moved by hand in the app, in steps, and put
// back with one click. What the render draws has to follow.
func TestTheCaptionLineMovesInStepsAndComesBack(t *testing.T) {
	editedPlan := editablePlanPath(t)
	captionY := func() *float64 {
		t.Helper()
		_, clips, err := LoadClips(editedPlan)
		if err != nil {
			t.Fatal(err)
		}
		for _, c := range clips {
			if c.ID == "01" {
				return c.CaptionY
			}
		}
		t.Fatal("clip 01 is gone")
		return nil
	}

	if got := captionY(); got != nil {
		t.Fatalf("a fresh clip sits at %v", *got)
	}
	// Anywhere between two steps lands on the nearer one.
	if err := SetCaptionY(editedPlan, "01", 437); err != nil {
		t.Fatal(err)
	}
	if got := captionY(); got == nil || *got != 440 {
		t.Errorf("437 became %v", got)
	}
	// And nothing leaves the frame. The last one stands for the checks that
	// follow, so these run in the order they are written.
	for _, c := range []struct{ value, want float64 }{
		{9000, CaptionYMax},
		{-400, CaptionYMin},
	} {
		if err := SetCaptionY(editedPlan, "01", c.value); err != nil {
			t.Fatal(err)
		}
		if got := captionY(); got == nil || *got != c.want {
			t.Errorf("%v became %v, want %v", c.value, got, c.want)
		}
	}

	// The look the render uses for that clip carries the new line, and the
	// plan's own look is left alone for every other clip.
	_, clips, err := LoadClips(editedPlan)
	if err != nil {
		t.Fatal(err)
	}
	plan := map[string]any{"margin_v": 300.0}
	for _, c := range clips {
		style := clipStyle(plan, c)
		want := 300.0
		if c.CaptionY != nil {
			want = *c.CaptionY
		}
		if style["margin_v"] != want {
			t.Errorf("clip %s renders at %v, want %v", c.ID, style["margin_v"], want)
		}
	}
	if plan["margin_v"] != 300.0 {
		t.Errorf("the plan's own look was changed to %v", plan["margin_v"])
	}

	// What the app draws sits in the same place.
	view, err := ClipCaptionsView(editedPlan, "01", editableTranscript(), nil)
	if err != nil {
		t.Fatal(err)
	}
	if view.Style.MarginV*1920 != CaptionYMin {
		t.Errorf("the app draws at %v", view.Style.MarginV*1920)
	}

	if err := ResetCaptionY(editedPlan, "01"); err != nil {
		t.Fatal(err)
	}
	if got := captionY(); got != nil {
		t.Errorf("after putting it back the clip sits at %v", *got)
	}

	if err := SetCaptionY(editedPlan, "99", 300); err == nil {
		t.Error("a clip that is not in the plan was moved")
	}
	if err := SetCaptionY(editedPlan, "01", math.Inf(1)); err == nil {
		t.Error("a caption line at infinity was accepted")
	}
}

// FuzzPlanEdits runs whatever sequence of edits the fuzzer comes up with,
// the way a person clicking around the workspace would. Whatever happens,
// the plan on disk stays a plan that renders.
func FuzzPlanEdits(f *testing.F) {
	f.Add([]byte{0, 100, 130, 1, 60, 110, 2, 115, 0, 3, 105, 0})
	f.Add([]byte{1, 120, 124, 1, 120, 124})
	f.Add([]byte{4, 0, 0, 4, 1, 0, 2, 118, 1})
	// Cut a clip in two, move the cut, then put it back.
	f.Add([]byte{6, 112, 119, 8, 111, 120, 7, 115, 0})
	// Cut the same clip over and over, which is how a clip runs out of
	// pieces and out of length.
	f.Add([]byte{6, 100, 105, 6, 106, 110, 6, 112, 118, 6, 120, 130})
	// Thumbnails added, moved and removed while the clip is cut and
	// trimmed under them.
	f.Add([]byte{9, 104, 0, 9, 104, 120, 6, 110, 125, 9, 120, 0, 0, 100, 112})
	f.Fuzz(func(t *testing.T, script []byte) {
		path := editablePlanPath(t)
		tr := editableTranscript()
		// Times are read as tenths of a second from 0 to 25.5, which covers
		// the whole plan and a good part either side of it.
		at := func(b byte) float64 { return float64(b) / 10 }
		lastRevision := 0
		for i := 0; i+2 < len(script); i += 3 {
			clip := "01"
			if script[i+1]%2 == 1 {
				clip = "02"
			}
			switch script[i] % 10 {
			case 9:
				// Add at one moment, or move or remove the thumbnail at
				// another, with nought standing for none.
				from, to := at(script[i+1]), at(script[i+2])
				if script[i+2] == 0 {
					from, to = -1, from
				} else if script[i+1]%3 == 0 {
					to = -1
				}
				_ = SetThumbnail(path, clip, from, to)
			case 6:
				// A cut takes a part out of the middle, so this is the
				// one edit that makes pieces rather than only moving them.
				_ = CutClip(path, clip, at(script[i+1]), at(script[i+2]), tr, 0.1, ToWords)
			case 7:
				_ = JoinCut(path, clip, at(script[i+1]), tr)
			case 8:
				_ = MoveCut(path, clip, int(script[i+1])%4,
					at(script[i+1]), at(script[i+2]), tr, 0.1, ToWords)
			case 0:
				_ = TrimClip(path, clip, at(script[i+1]), at(script[i+2]), tr, 0.1, Snap(script[i+2]%2 == 0))
			case 1:
				_ = SetCaptionStyle(path, map[string]any{"size": float64(24 + int(script[i+2])%176)})
			case 2:
				_ = SetCrop(path, clip, at(script[i+1]), int(script[i+2])*4)
			case 3:
				_ = ResetCrop(path, clip, at(script[i+1]))
			case 4:
				_ = SetRejected(path, clip, script[i+2]%2 == 0)
			case 5:
				if script[i+2]%4 == 0 {
					_ = ResetCaptionY(path, clip)
				} else {
					_ = SetCaptionY(path, clip, float64(script[i+2])*8)
				}
			}

			plan, clips, err := LoadClips(path)
			if err != nil {
				t.Fatalf("step %d left a plan that does not load: %v", i/3, err)
			}
			if plan.Raw["custom"] == nil || plan.Raw["plan_id"] != "p1" {
				t.Fatalf("step %d lost a field the loader does not know", i/3)
			}
			revision, _ := toInt(plan.Raw["revision"])
			if revision < lastRevision {
				t.Fatalf("the revision went backwards, %d after %d", revision, lastRevision)
			}
			lastRevision = revision
			if len(clips) == 0 {
				t.Fatalf("step %d emptied the plan", i/3)
			}
			for _, c := range clips {
				if len(c.Thumbnails) > MaxThumbnails {
					t.Fatalf("clip %s asks for %d pictures", c.ID, len(c.Thumbnails))
				}
				for k, th := range c.Thumbnails {
					if !insidePieces(c.Segments, th) || (k > 0 && th <= c.Thumbnails[k-1]) {
						t.Fatalf("clip %s: thumbnail %v is outside the clip or out of order", c.ID, th)
					}
				}
				var previous float64
				for k, seg := range c.Segments {
					if seg.Start < previous {
						t.Fatalf("clip %s: piece %d starts at %v, before %v ends",
							c.ID, k, seg.Start, previous)
					}
					previous = seg.End
				}
				if c.Duration() <= 0 {
					t.Fatalf("clip %s is %v long", c.ID, c.Duration())
				}
				if y := c.CaptionY; y != nil && (*y < CaptionYMin || *y > CaptionYMax) {
					t.Fatalf("clip %s draws its captions at %v", c.ID, *y)
				}
			}
		}
		entries, _ := os.ReadDir(filepath.Dir(path))
		for _, entry := range entries {
			if strings.HasPrefix(entry.Name(), ".clips-") {
				t.Fatalf("a temporary plan was left behind: %s", entry.Name())
			}
		}
	})
}

// Letting go of a search has to leave nothing of it behind that a later
// search could pick up by accident, and has to leave finished work alone.
func TestRemovingAPlanKeepsTheWorkItLeavesBehind(t *testing.T) {
	plan := editablePlanPath(t)
	work := filepath.Dir(filepath.Dir(plan))
	rendered := filepath.Join(work, "01_eins.mp4")
	if err := os.WriteFile(rendered, []byte("finished"), 0o644); err != nil {
		t.Fatal(err)
	}

	if err := RemovePlan(plan); err != nil {
		t.Fatal(err)
	}
	if isFile(plan) {
		t.Error("the plan file is still there")
	}
	if !isFile(rendered) {
		t.Error("a rendered clip was taken away")
	}
	// Doing it twice is not an error, because the part is gone either way.
	if err := RemovePlan(plan); err != nil {
		t.Errorf("second removal: %s", err)
	}
}

// The parts carry the plans behind them, so letting go of one knows
// exactly what to take with it.
func TestSearchedPartsCarryTheirPlans(t *testing.T) {
	looked := SearchedPlans([]PlanSummary{
		{Path: "/a/clips-0-600.json", From: 0, To: 600, Clips: 4},
		{Path: "/a/clips-600-900.json", From: 600, To: 900, Clips: 2},
		{Path: "/a/clips-1800-2400.json", From: 1800, To: 2400, Clips: 3},
	}, 3600)
	if len(looked) != 2 {
		t.Fatalf("parts %v", looked)
	}
	if looked[0].Start != 0 || looked[0].End != 900 || looked[0].Clips != 6 ||
		len(looked[0].Plans) != 2 {
		t.Errorf("the two that meet did not become one: %v", looked[0])
	}
	if looked[1].Clips != 3 || len(looked[1].Plans) != 1 {
		t.Errorf("the one on its own: %v", looked[1])
	}
}

// The app makes one edit per click, and a click can land while the last one
// is still being written. Every edit has to survive that, because there is
// no save button to put a lost one back.
func TestEditsAtTheSameTimeDoNotLoseEachOther(t *testing.T) {
	path := editablePlanPath(t)
	const rounds = 12
	var wg sync.WaitGroup
	errs := make(chan error, rounds*2)
	for i := 0; i < rounds; i++ {
		for _, id := range []string{"01", "02"} {
			wg.Add(1)
			go func(id string, y float64) {
				defer wg.Done()
				if err := SetCaptionY(path, id, y); err != nil {
					errs <- err
				}
			}(id, 200+float64(i))
		}
	}
	wg.Wait()
	close(errs)
	for err := range errs {
		t.Fatalf("an edit failed: %s", err)
	}
	data, err := os.ReadFile(path)
	if err != nil {
		t.Fatal(err)
	}
	var top struct {
		Revision int `json:"revision"`
		Clips    []struct {
			ID       string   `json:"id"`
			CaptionY *float64 `json:"caption_y"`
		} `json:"clips"`
	}
	if err := json.Unmarshal(data, &top); err != nil {
		t.Fatalf("the plan is no longer readable: %s", err)
	}
	if top.Revision != rounds*2 {
		t.Errorf("%d edits of %d landed", top.Revision, rounds*2)
	}
	for _, c := range top.Clips {
		if c.CaptionY == nil {
			t.Errorf("clip %s lost its caption line", c.ID)
		}
	}
}

// A part of a search can be given back on its own. The clips inside it
// go, the clips outside it stay, and the plan says the part may be read
// again, which is what leaves a hole in what was searched.
func TestAPartOfASearchCanBeGivenBack(t *testing.T) {
	path := editablePlanPath(t)
	// The plan was made over the first minute.
	if err := editPlan(path, func(top *object, _ []*object) error {
		made := newObject()
		made.set("from", 0.0)
		made.set("to", 60.0)
		top.set("planned_with", made)
		return nil
	}); err != nil {
		t.Fatal(err)
	}

	gone, err := RemoveRange(path, 9, 15, 3600)
	if err != nil {
		t.Fatal(err)
	}
	if gone != 1 {
		t.Fatalf("%d clips went, not 1", gone)
	}
	if !isFile(path) {
		t.Fatal("the plan went, though most of its window is still searched")
	}
	plan, clips, err := LoadClips(path)
	if err != nil {
		t.Fatal(err)
	}
	if len(clips) != 1 || clips[0].ID != "02" {
		t.Fatalf("what is left: %v", clips)
	}
	if kept, _ := plan.Raw["custom"].(map[string]any); kept["kept"] != "yes" {
		t.Error("an edit threw away what it did not understand")
	}

	// What is left of the window: everything but the part given back.
	summary := PlanSummaries(filepath.Dir(path))
	if len(summary) != 1 {
		t.Fatalf("plans: %v", summary)
	}
	looked := SearchedWindows(summary, 3600)
	if fmt.Sprint(looked) != "[{0 9} {15 60}]" {
		t.Fatalf("searched %v", looked)
	}
	// Giving back the rest takes the plan itself.
	if _, err := RemoveRange(path, 0, 60, 3600); err != nil {
		t.Fatal(err)
	}
	if isFile(path) {
		t.Error("a plan with nothing left of its window stayed")
	}
}

// The text and the box of the captions take a colour each, the box with how
// much of the picture shows through it, and the video preview is told the
// same colours the render will use.
func TestCaptionColoursReachTheRenderAndThePreview(t *testing.T) {
	text, ok := AssColour("#ffcc00", 1)
	if !ok || text != "&H0000CCFF" {
		t.Fatalf("text %q %v", text, ok)
	}
	if half, _ := AssColour("#ffcc00", 0.5); half != "&H8000CCFF" {
		t.Errorf("half clear text %q", half)
	}
	box, ok := AssColour("#102030", 0.25)
	if !ok || box != "&HBF302010" {
		t.Fatalf("box %q %v", box, ok)
	}
	for _, bad := range []string{"", "ffcc00", "#fc0", "#gggggg", "#ffcc00\n"} {
		if _, ok := AssColour(bad, 1); ok {
			t.Errorf("%q was taken for a colour", bad)
		}
	}
	if _, ok := AssColour("#ffcc00", math.NaN()); ok {
		t.Error("an opacity that is not a number was taken")
	}

	path := editablePlanPath(t)
	tr := editableTranscript()
	if err := SetCaptionStyle(path, map[string]any{"primary": text, "back_colour": box}); err != nil {
		t.Fatal(err)
	}
	plan, _, err := LoadClips(path)
	if err != nil {
		t.Fatal(err)
	}
	if s := ResolveStyle(plan.CaptionStyle()); s.Primary != text || s.BackColour != box {
		t.Errorf("the render would draw %s on %s", s.Primary, s.BackColour)
	}
	if err := SetCaptionStyle(path, map[string]any{"highlight_colour": "#00aa00"}); err != nil {
		t.Fatal(err)
	}
	if view, _ := ClipCaptionsView(path, "01", tr, nil); view.Style.HighlightColour != "rgba(0, 170, 0, 1)" {
		t.Errorf("the pill is %s", view.Style.HighlightColour)
	}
	// A highlight with an opacity is kept whole, and the pill is as clear
	// in the short as in the video preview.
	if err := SetCaptionStyle(path, map[string]any{"highlight_colour": "&H6600AA00"}); err != nil {
		t.Fatal(err)
	}
	if view, _ := ClipCaptionsView(path, "01", tr, nil); view.Style.HighlightColour != "rgba(0, 170, 0, 0.6)" {
		t.Errorf("the pill is %s", view.Style.HighlightColour)
	}
	if plan, _, err := LoadClips(path); err != nil {
		t.Fatal(err)
	} else if s := ResolveStyle(plan.CaptionStyle()); s.HighlightColour != "&H00AA00&" || s.HighlightAlpha != "66" {
		t.Errorf("the render would draw the pill %s at %s", s.HighlightColour, s.HighlightAlpha)
	}
	view, err := ClipCaptionsView(path, "01", tr, nil)
	if err != nil {
		t.Fatal(err)
	}
	if view.Style.Primary != "rgba(255, 204, 0, 1)" || view.Style.Box != "rgba(16, 32, 48, 0.251)" {
		t.Errorf("the preview draws %s on %s", view.Style.Primary, view.Style.Box)
	}
}

// A word is shown and hidden by its alpha. A shown word takes the alpha of
// the text colour, so text given an opacity keeps it in the short, and a
// colour without one is shown solid.
func TestAShownWordKeepsTheTextOpacity(t *testing.T) {
	if got := shownTag("&H8000CCFF"); got != `{\alpha&H80&}` {
		t.Errorf("half clear text is shown as %s", got)
	}
	if got := shownTag("&H00FFFFFF"); got != `{\alpha&H00&}` {
		t.Errorf("solid text is shown as %s", got)
	}
	for _, odd := range []string{"", "&HFFFFFF", "&H80XXCCFF", "junk"} {
		if got := shownTag(odd); got != `{\alpha&H00&}` {
			t.Errorf("%q is shown as %s", odd, got)
		}
	}
	// The pill's alpha: a colour given as #RRGGBB, or no colour, is solid.
	for value, want := range map[any]string{"#942192": "00", "&H6600AA00": "66", "&H00AA00&": "00",
		"942192AB": "00", nil: "00", "&HZZ00AA00": "00"} {
		if got := alphaOrSolid(highlightAlpha(value)); got != want {
			t.Errorf("the pill of %v is %s clear", value, got)
		}
	}
	line := taggedText([]string{"eins", "zwei"}, [][]int{{0, 1}}, func(i int) bool { return i == 0 },
		shownTag("&H40FFFFFF"))
	if line != `{\alpha&H40&}eins {\alpha&HFF&}zwei` {
		t.Errorf("line %s", line)
	}
}

// The highlight is on or off and nothing else, because the render reads a
// number and a string would quietly count as on.
func TestCaptionHighlightTakesOnlyOnOrOff(t *testing.T) {
	path := editablePlanPath(t)
	for _, bad := range []any{"off", 0.0, nil} {
		if err := SetCaptionStyle(path, map[string]any{"highlight": bad}); err == nil {
			t.Errorf("%v was taken for on or off", bad)
		}
	}
	if err := SetCaptionStyle(path, map[string]any{"highlight": false}); err != nil {
		t.Fatal(err)
	}
	plan, _, err := LoadClips(path)
	if err != nil {
		t.Fatal(err)
	}
	if ResolveStyle(plan.CaptionStyle()).Highlight {
		t.Error("the highlight is still on")
	}
}

// A thumbnail is a moment inside the clip. It is added, moved and removed
// by the moment it stands at, refused where the short has nothing to show,
// and a trim that leaves it outside hides it without losing it.
func TestThumbnailsAreAddedMovedAndRemoved(t *testing.T) {
	path := editablePlanPath(t)
	tr := editableTranscript()
	thumbs := func() []float64 {
		t.Helper()
		_, clips, err := LoadClips(path)
		if err != nil {
			t.Fatal(err)
		}
		return clips[0].Thumbnails
	}
	must := func(err error) {
		t.Helper()
		if err != nil {
			t.Fatal(err)
		}
	}
	must(SetThumbnail(path, "01", -1, 12.5))
	must(SetThumbnail(path, "01", -1, 10.25))
	if got := thumbs(); len(got) != 2 || got[0] != 10.25 || got[1] != 12.5 {
		t.Fatalf("the thumbnails are %v", got)
	}
	// Nothing where the short has no picture: in the cut between the two
	// pieces, before the clip and after it. Nor twice in one place.
	before, _ := os.ReadFile(path)
	for _, at := range []float64{11.5, 9, 13.5} {
		if err := SetThumbnail(path, "01", -1, at); err == nil {
			t.Errorf("a thumbnail was put at %v, which is not in the clip", at)
		}
	}
	if err := SetThumbnail(path, "01", -1, 12.5); err == nil {
		t.Error("a second thumbnail was put on the same moment")
	}
	if err := SetThumbnail(path, "01", 10.7, -1); err == nil {
		t.Error("a thumbnail that is not there was removed")
	}
	if err := SetThumbnail(path, "01", math.NaN(), 12); err == nil {
		t.Error("a thumbnail was moved from nowhere")
	}
	after, _ := os.ReadFile(path)
	if string(before) != string(after) {
		t.Fatal("a refused thumbnail changed the plan")
	}
	must(SetThumbnail(path, "01", 12.5, 10.75))
	if got := thumbs(); len(got) != 2 || got[0] != 10.25 || got[1] != 10.75 {
		t.Fatalf("after the move the thumbnails are %v", got)
	}
	// A trim that leaves one outside hides it, and it comes back with the
	// piece it was in.
	must(TrimClip(path, "01", 10.5, 13.1, tr, 0.1, ToWords))
	if got := thumbs(); len(got) != 1 || got[0] != 10.75 {
		t.Fatalf("after the trim the thumbnails are %v", got)
	}
	must(TrimClip(path, "01", 10.0, 13.1, tr, 0.1, ToWords))
	if got := thumbs(); len(got) != 2 {
		t.Fatalf("after the trim back the thumbnails are %v", got)
	}
	must(SetThumbnail(path, "01", 10.25, -1))
	must(SetThumbnail(path, "01", 10.75, -1))
	if got := thumbs(); len(got) != 0 {
		t.Fatalf("after removing both the thumbnails are %v", got)
	}
	if strings.Contains(string(mustRead(t, path)), "thumbnails") {
		t.Error("an empty list was left in the plan")
	}
}

func mustRead(t *testing.T, path string) []byte {
	t.Helper()
	b, err := os.ReadFile(path)
	if err != nil {
		t.Fatal(err)
	}
	return b
}

// What a plan file says about thumbnails is not trusted: only numbers
// inside the clip, each once, in order, and no more than the limit.
func TestThumbnailsInAPlanAreChecked(t *testing.T) {
	var list []any
	list = append(list, "12", 12.0, 12.0004, 11.5, -3.0, math.Inf(1), nil, 10.0)
	for i := 0; i < 80; i++ {
		list = append(list, 12.0+float64(i)/100)
	}
	pieces := []Segment{{Start: 10, End: 11.1}, {Start: 11.9, End: 13.1}}
	got := readThumbnails(list, pieces)
	if len(got) != MaxThumbnails || got[0] != 10 || got[1] != 12 {
		t.Fatalf("read %d thumbnails starting %v", len(got), got[:3])
	}
	for k := 1; k < len(got); k++ {
		if got[k] <= got[k-1] {
			t.Fatalf("thumbnail %d is not after the one before: %v", k, got)
		}
	}
}

// The stops an edge lands on with shift are the words the captions light
// up. A word too wide for a line is two stops, at the same times as its two
// halves in the captions, and a correction that reads as two words is two.
func TestTheWordsShownAreTheWordsTheCaptionsLight(t *testing.T) {
	long := "Donaudampfschifffahrtsgesellschaftskapitänsmütze"
	dir := filepath.Join(t.TempDir(), "ep.framefairy", "logs")
	if err := os.MkdirAll(dir, 0o755); err != nil {
		t.Fatal(err)
	}
	path := filepath.Join(dir, "clips.json")
	plan := `{"source": "ep.mp4", "clips": [{"id": "01", "slug": "x",
	  "segments": [{"start": 10.0, "end": 14.0}],
	  "words": [[10.2, 12.2, "` + long + `"], [12.5, 13.0, "Und da"]]}]}`
	if err := os.WriteFile(path, []byte(plan), 0o644); err != nil {
		t.Fatal(err)
	}
	tr := fromStored([]Cue{{10.2, 12.2, long}, {12.5, 13.0, "Und"}}, nil, 0, 0, nil)
	tr.Correct(map[string]string{wordKey(12.5): "Und da"})
	loaded, clip, err := planClip(path, "01")
	if err != nil {
		t.Fatal(err)
	}
	stops := ShowWords(tr.WordsBetween(10, 14), ResolveStyle(captionStyle(loaded, clip, nil)), tr.Language)
	view, err := ClipCaptionsView(path, "01", tr, nil)
	if err != nil {
		t.Fatal(err)
	}
	var lit []Cue
	for _, c := range view.Captions {
		for _, line := range c.Lines {
			for _, w := range line.Words {
				lit = append(lit, Cue{roundTo(10+w.Start, 3), roundTo(10+w.End, 3), w.Text})
			}
		}
	}
	if len(stops) < 4 || len(stops) != len(lit) {
		t.Fatalf("stops %+v\nlit %+v", stops, lit)
	}
	for i := range stops {
		if math.Abs(stops[i].Start-lit[i].Start) > 0.002 || math.Abs(stops[i].End-lit[i].End) > 0.002 ||
			stops[i].Text != lit[i].Text {
			t.Errorf("stop %d is %+v, the captions light %+v", i, stops[i], lit[i])
		}
	}
	// And they are where an edge dragged with shift lands: the start of the
	// clip dragged onto the second half of the long word starts the clip
	// there, and the playhead is in that half, so it is the one lit.
	half := stops[1]
	shaped, err := ShapeClip(path, "01", Gesture{Kind: "trim", Edge: "start", From: half.Start + 0.01,
		ToWords: true, Frame: 0.04}, tr, 0.1)
	if err != nil {
		t.Fatal(err)
	}
	if got := shaped.Pieces[0].Start; math.Abs(got-roundTo(half.Start, 3)) > 0.001 {
		t.Errorf("the clip starts at %v, not at the second half %+v", got, half)
	}
	if shaped.Playhead < half.Start || shaped.Playhead >= half.End {
		t.Errorf("the playhead is at %v, outside %+v", shaped.Playhead, half)
	}
}

// An edit waits for another program editing the same file, the command
// line or a second copy of the app, and goes ahead once it is done.
func TestAnEditWaitsForAnotherProgram(t *testing.T) {
	t.Parallel()
	if runtime.GOOS == "windows" {
		t.Skip("no lock across programs on Windows yet")
	}
	path := filepath.Join(t.TempDir(), "clips.json")
	other := exec.Command(os.Args[0])
	other.Env = append(os.Environ(), "FRAMEFAIRY_HOLD_LOCK="+path)
	stdin, err := other.StdinPipe()
	if err != nil {
		t.Fatal(err)
	}
	out, err := other.StdoutPipe()
	if err != nil {
		t.Fatal(err)
	}
	if err := other.Start(); err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { stdin.Close(); _ = other.Wait() })
	line, err := bufio.NewReader(out).ReadString('\n')
	if err != nil || strings.TrimSpace(line) != "held" {
		t.Fatalf("the other program said %q, %v", line, err)
	}

	got := make(chan struct{})
	go func() {
		release := lockFile(path)
		release()
		close(got)
	}()
	select {
	case <-got:
		t.Fatal("the lock was taken while another program held it")
	case <-time.After(300 * time.Millisecond):
	}
	stdin.Close()
	select {
	case <-got:
	case <-time.After(5 * time.Second):
		t.Fatal("the lock was not taken once the other program let go")
	}
}

// captionWords is the text of every word of a laid out caption, in order.
func captionWords(c CaptionView) []string {
	var out []string
	for _, l := range c.Lines {
		for _, w := range l.Words {
			out = append(out, w.Text)
		}
	}
	return out
}

// A double-click on a clip's edge puts it back where the clip was found, so
// the clip keeps where its edges were the first time anything changes them,
// and never after. A clip never changed is where it was found.
func TestAClipKeepsWhereItWasFound(t *testing.T) {
	path := editablePlanPath(t)
	tr := editableTranscript()
	view, err := ReadPlan(path)
	if err != nil {
		t.Fatal(err)
	}
	if got := view.Clips[0].Found; got != [2]float64{10, 13.1} {
		t.Fatalf("a clip never changed is found at %v, want its edges", got)
	}

	trim := func(edge string, at float64) {
		t.Helper()
		if err := Reshape(path, "01", Gesture{Kind: "trim", Edge: edge, From: at}, tr, 0.1); err != nil {
			t.Fatal(err)
		}
	}
	trim("start", 10.55)
	trim("end", 12.6)
	_, clips, err := LoadClips(path)
	if err != nil {
		t.Fatal(err)
	}
	if clips[0].Found == nil || *clips[0].Found != [2]float64{10, 13.1} {
		t.Fatalf("found %v after two trims, want the edges before the first", clips[0].Found)
	}
	if clips[0].Segments[0].Start == 10 {
		t.Fatal("the trim did not move the start")
	}

	// Putting the start back is a trim to where it was found.
	trim("start", 10)
	_, clips, _ = LoadClips(path)
	if clips[0].Segments[0].Start != 10 || *clips[0].Found != [2]float64{10, 13.1} {
		t.Errorf("start %v, found %v", clips[0].Segments[0].Start, clips[0].Found)
	}

	// Exactly, though the edge was found between two frames: at five
	// frames a second 13.1 is no frame's edge, and the end went back to
	// 13.2.
	if err := Reshape(path, "01", Gesture{Kind: "trim", Edge: "end", From: 13.1, Frame: 0.2}, tr, 0.1); err != nil {
		t.Fatal(err)
	}
	_, clips, _ = LoadClips(path)
	if got := clips[0].Segments[len(clips[0].Segments)-1].End; got != 13.1 {
		t.Errorf("the end put back with frames is at %v, want 13.1 where it was found", got)
	}
	// A drag that ends well away from it still lands on a frame.
	if err := Reshape(path, "01", Gesture{Kind: "trim", Edge: "end", From: 12.73, Frame: 0.2}, tr, 0.1); err != nil {
		t.Fatal(err)
	}
	_, clips, _ = LoadClips(path)
	if got := clips[0].Segments[len(clips[0].Segments)-1].End; got != 12.8 {
		t.Errorf("a drag to 12.73 lands at %v, want the frame at 12.8", got)
	}

	// The plan is untrusted: anything but two numbers in order is no edges.
	for _, raw := range []any{nil, "10", []any{1.0}, []any{5.0, 2.0}, []any{-1.0, 2.0}, []any{"a", 2.0}} {
		if got := readFound(raw); got != nil {
			t.Errorf("readFound(%v) = %v", raw, *got)
		}
	}
}

// Moving one edge of a cut never moves the other. Shift on the right edge
// put the left one on a word as well, and Tim saw it jump.
func TestMovingOneEdgeOfACutLeavesTheOther(t *testing.T) {
	tr := editableTranscript()
	// Each case starts from the clip with its cut's left edge moved into
	// zwei on a frame, where no word ends, so a word would move it.
	fresh := func() string {
		path := editablePlanPath(t)
		if err := Reshape(path, "01", Gesture{Kind: "move", Edge: "from", From: 10.8, To: 11.9, Frame: 0.04}, tr, 0.1); err != nil {
			t.Fatal(err)
		}
		return path
	}
	move := func(path string, g Gesture) []Segment {
		t.Helper()
		g.Kind, g.Index, g.Frame = "move", 0, 0.04
		if err := Reshape(path, "01", g, tr, 0.1); err != nil {
			t.Fatal(err)
		}
		_, clips, err := LoadClips(path)
		if err != nil {
			t.Fatal(err)
		}
		return clips[0].Segments
	}

	// The right edge onto words with shift: the left one stays at 10.8.
	got := move(fresh(), Gesture{Edge: "to", From: 10.8, To: 12.2, ToWords: true})
	if got[0].End != 10.8 {
		t.Errorf("the left edge moved to %v when the right one was put on words", got[0].End)
	}
	if got[1].Start == 11.9 {
		t.Errorf("the right edge did not move: %+v", got)
	}

	// The left edge onto words with shift: the right one stays at 11.9.
	got = move(fresh(), Gesture{Edge: "from", From: 10.7, To: 11.9, ToWords: true})
	if got[1].Start != 11.9 {
		t.Errorf("the right edge moved to %v when the left one was put on words", got[1].Start)
	}
	if got[0].End == 10.8 {
		t.Errorf("the left edge did not move: %+v", got)
	}

	// An edge dragged past the other stops at it rather than pushing it.
	got = move(fresh(), Gesture{Edge: "to", From: 10.8, To: 10.3})
	if got[0].End != 10.8 || got[1].Start <= 10.8 {
		t.Errorf("dragging the right edge past the left one gave %+v", got)
	}

	// An edge a cut does not have is refused.
	if err := Reshape(fresh(), "01", Gesture{Kind: "move", Edge: "middle", From: 10.8, To: 11.9}, tr, 0.1); err == nil {
		t.Error("a move of an edge called middle was taken")
	}
}

// A caption stays up a little after its last word, and the clip timeline
// draws its block that long. An edge put on words stops where that block
// ends before it goes on to the word. It went straight to the word, past
// the end of the block Tim was looking at, and the block shrank under it.
func TestAnEdgeOnWordsStopsWhereTheCaptionGoes(t *testing.T) {
	tr := editableTranscript()
	// zwei ends at 11.0 and drei begins at 12.0, so "eins zwei" stays up
	// until 11.4. The clip's cut takes 11.1 to 11.9 and with it that hold.
	segments := func(path string, g Gesture) []Segment {
		t.Helper()
		g.Frame, g.ToWords = 0.04, true
		if err := Reshape(path, "01", g, tr, 0.1); err != nil {
			t.Fatal(err)
		}
		_, clips, err := LoadClips(path)
		if err != nil {
			t.Fatal(err)
		}
		return clips[0].Segments
	}
	cases := []struct {
		name string
		g    Gesture
		end  float64
	}{
		{"the left edge of a cut past where the caption goes",
			Gesture{Kind: "move", Edge: "from", From: 11.6, To: 11.9}, 11.4},
		{"the left edge of a cut inside the time the caption stays",
			Gesture{Kind: "move", Edge: "from", From: 11.3, To: 11.9}, 11.1},
		{"the left edge of a cut into the word",
			Gesture{Kind: "move", Edge: "from", From: 10.9, To: 11.9}, 10.6},
		{"the end of the clip near where the caption goes",
			Gesture{Kind: "trim", Edge: "end", From: 11.45}, 11.4},
		{"the end of the clip near the word",
			Gesture{Kind: "trim", Edge: "end", From: 11.15}, 11.1},
	}
	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			got := segments(editablePlanPath(t), c.g)
			if got[0].End != c.end {
				t.Errorf("the edge landed at %v, want %v: %+v", got[0].End, c.end, got)
			}
			if c.g.Kind == "move" && got[1].Start != 11.9 {
				t.Errorf("the right edge moved to %v", got[1].Start)
			}
		})
	}
}
