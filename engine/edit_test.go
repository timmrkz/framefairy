package engine

import (
	"encoding/json"
	"fmt"
	"math"
	"os"
	"path/filepath"
	"strings"
	"sync"
	"testing"
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

func editableTranscript() *Transcript {
	return &Transcript{Words: []Cue{
		{10, 10.5, "eins"}, {10.6, 11, "zwei"}, {12, 12.4, "drei"}, {13, 13.5, "vier"},
		{20.1, 20.6, "fünf"}, {21, 21.5, "sechs"},
	}}
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
		{"a clip under a second", func() error { return TrimClip(path, "01", 12, 12.2, tr, 0.1) }},
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
		func() error { return TrimClip(path, "01", 10, 12.4, tr, 0.1) },
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

// The picture in the app draws the captions of the selected clip itself, so
// it asks the engine for them, on the clip's own clock and already broken
// into lines.
func TestClipCaptionsComeBackOnTheClipClock(t *testing.T) {
	path := editablePlanPath(t)
	view, err := ClipCaptionsView(path, "01", nil)
	if err != nil {
		t.Fatal(err)
	}
	if len(view.Captions) != 1 {
		t.Fatalf("captions %+v", view.Captions)
	}
	caption := view.Captions[0]
	if caption.Start != 0 || caption.End <= caption.Start {
		t.Errorf("caption runs %v to %v", caption.Start, caption.End)
	}
	if len(caption.Lines) != 1 || len(caption.Lines[0].Words) != 3 {
		t.Fatalf("lines %+v", caption.Lines)
	}
	words := caption.Lines[0].Words
	if words[0].Text != "eins" || words[2].Text != "drei" || words[2].Start <= words[0].Start {
		t.Errorf("words %+v", words)
	}
	// The cut between the two pieces is gone from the clock, so the last
	// word sits a good deal earlier than in the episode.
	if words[2].Start > 2 {
		t.Errorf("the last word starts at %v, so the cut is still in", words[2].Start)
	}

	// The look comes as shares of the frame height and as web colours.
	s := view.Style
	if s.Size <= 0 || s.Size > 1 || s.MarginV <= 0 || s.MarginV > 1 {
		t.Errorf("style %+v", s)
	}
	if s.HighlightColour != "rgba(148, 33, 146, 1)" || s.Primary != "rgba(255, 255, 255, 1)" {
		t.Errorf("colours %+v", s)
	}

	if _, err := ClipCaptionsView(path, "99", nil); err == nil {
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
	view, err := ClipCaptionsView(editedPlan, "01", nil)
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
			switch script[i] % 9 {
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
				_ = TrimClip(path, clip, at(script[i+1]), at(script[i+2]), tr, 0.1)
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
				for _, w := range c.Words {
					if w.End < w.Start || !isFinite(w.Start) {
						t.Fatalf("clip %s carries the word %+v", c.ID, w)
					}
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
	captions := filepath.Join(work, "captions")
	if err := os.MkdirAll(captions, 0o755); err != nil {
		t.Fatal(err)
	}
	mine := filepath.Join(captions, "01_eins.srt")
	if err := os.WriteFile(mine, []byte("corrected by hand"), 0o644); err != nil {
		t.Fatal(err)
	}
	other := filepath.Join(captions, "07_anderes.srt")
	if err := os.WriteFile(other, []byte("another plan"), 0o644); err != nil {
		t.Fatal(err)
	}
	rendered := filepath.Join(work, "01_eins.mp4")
	if err := os.WriteFile(rendered, []byte("finished"), 0o644); err != nil {
		t.Fatal(err)
	}

	if err := RemovePlan(plan, captions); err != nil {
		t.Fatal(err)
	}
	if isFile(plan) {
		t.Error("the plan file is still there")
	}
	if isFile(mine) {
		t.Error("the captions of its clips were left where a later plan would find them")
	}
	if !isFile(other) {
		t.Error("the captions of another plan were taken away")
	}
	if !isFile(rendered) {
		t.Error("a rendered clip was taken away")
	}
	// Moved aside, not deleted: it may hold corrections made by hand.
	found, _ := filepath.Glob(filepath.Join(captions, "superseded-*", "01_eins.srt"))
	if len(found) != 1 {
		t.Fatalf("the corrected captions are nowhere: %v", found)
	}
	body, err := os.ReadFile(found[0])
	if err != nil || string(body) != "corrected by hand" {
		t.Errorf("moved aside as %q, %v", body, err)
	}
	// Doing it twice is not an error, because the part is gone either way.
	if err := RemovePlan(plan, captions); err != nil {
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
	captions := filepath.Join(filepath.Dir(filepath.Dir(path)), "captions")
	if err := os.MkdirAll(captions, 0o755); err != nil {
		t.Fatal(err)
	}
	mine := filepath.Join(captions, "01_eins.srt")
	if err := os.WriteFile(mine, []byte("corrected by hand"), 0o644); err != nil {
		t.Fatal(err)
	}
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

	gone, err := RemoveRange(path, captions, 9, 15, 3600)
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
	if isFile(mine) {
		t.Error("the captions of the clip that went were left behind")
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
	if _, err := RemoveRange(path, captions, 0, 60, 3600); err != nil {
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
	view, err := ClipCaptionsView(path, "01", nil)
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
	line := taggedText([]string{"eins", "zwei"}, [][]int{{0, 1}}, func(i int) bool { return i == 0 },
		shownTag("&H40FFFFFF"))
	if line != `{\alpha&H40&}eins {\alpha&HFF&}zwei` {
		t.Errorf("line %s", line)
	}
}
