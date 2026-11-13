package engine

import (
	"encoding/json"
	"math"
	"math/rand/v2"
	"path/filepath"
	"strings"
	"testing"

	"github.com/spf13/afero"

	"framefairy/engine/timed"
)

// randomTalk is about seconds of words the way people talk, with pauses
// that part captions, pauses a little too short to, words too wide for
// a line, words removed and stretches nobody speaks in.
func randomTalk(r *rand.Rand, seconds float64) []timed.Cue {
	var words []timed.Cue
	at := 0.5
	for at < seconds {
		text := strings.Repeat("a", 2+r.IntN(8))
		switch k := r.IntN(40); {
		case k == 0:
			text = strings.Repeat("lang", 12)
		case k == 1:
			text = removedMark + text
		case k == 2:
			text += "."
		}
		long := 0.05 + r.Float64()*0.4
		words = append(words, timed.Cue{Start: at, End: at + long, Text: text})
		at += long + 0.02
		switch k := r.IntN(20); {
		case k < 3:
			at += captionPause + r.Float64()*2
		case k == 3:
			at += captionPause - 0.05
		case k == 4 && r.IntN(5) == 0:
			at += 40
		}
	}
	return words
}

// The captions of a clip a gesture changed, laid out again only near the
// change and the rest taken from before it, are the captions laid out
// whole: for an end and a start dragged in and out, and a cut made and
// taken away, with caption edges moved by hand, over 60 seeds.
func TestCaptionsLaidOutNearAChangeAreTheWholeClipsCaptions(t *testing.T) {
	t.Parallel()
	style := map[string]any{}
	near, all := 0, 0
	for seed := range uint64(60) {
		r := rand.New(rand.NewPCG(seed, 11))
		tr := fromStored(randomTalk(r, 300), nil, 0, 0, nil)
		was := Clip{ID: "01", Segments: []Segment{{Start: 20, End: 120}, {Start: 123, End: 260}}}
		// A few captions with an edge moved by hand.
		was.CaptionTimes = map[string]CaptionTime{}
		for _, c := range ClipDrawing(style, was, tr, SourceInfo{}, nil).Laid {
			switch r.IntN(12) {
			case 0:
				start := c.Opens - 0.2
				was.CaptionTimes[wordKey(c.Opens)] = CaptionTime{Start: &start}
			case 1:
				end := c.Closes + 0.5
				was.CaptionTimes[wordKey(c.Closes)] = CaptionTime{End: &end}
			}
		}
		before := ClipDrawing(style, was, tr, SourceInfo{}, nil)
		changes := map[string][]Segment{
			"end in":     {{Start: 20, End: 120}, {Start: 123, End: 200 + r.Float64()*50}},
			"end out":    {{Start: 20, End: 120}, {Start: 123, End: 262 + r.Float64()*30}},
			"start in":   {{Start: 22 + r.Float64()*80, End: 120}, {Start: 123, End: 260}},
			"start out":  {{Start: 1 + r.Float64()*18, End: 120}, {Start: 123, End: 260}},
			"cut made":   {{Start: 20, End: 120}, {Start: 123, End: 170}, {Start: 172 + r.Float64(), End: 260}},
			"cut gone":   {{Start: 20, End: 260}},
			"cut moved":  {{Start: 20, End: 118 + r.Float64()*4}, {Start: 124, End: 260}},
			"both edges": {{Start: 30, End: 120}, {Start: 123, End: 250}},
		}
		for name, pieces := range changes {
			now := was
			now.Segments = pieces
			want := ClipDrawing(style, now, tr, SourceInfo{}, nil).Laid
			got := clipDrawingFrom(before, style, now, tr, SourceInfo{}, nil)
			all++
			if got.splice != nil {
				near++
			}
			if len(got.Laid) != len(want) {
				t.Errorf("seed %d, %s: %d captions, laid out whole %d", seed, name, len(got.Laid), len(want))
				continue
			}
			for i := range want {
				if !sameLaid(got.Laid[i], want[i], 0) {
					t.Errorf("seed %d, %s: caption %d is %+v, laid out whole %+v", seed, name, i, got.Laid[i].Caption, want[i].Caption)
					break
				}
			}
		}
	}
	// A clip with pauses in it is laid out near the change, not whole.
	if near < all*9/10 {
		t.Errorf("%d of %d changes were laid out near the change, the rest whole", near, all)
	}
}

// A step of a drag sends only the captions near what the hand changed,
// and those put together with the captions of the clip as saved, the way
// the interface puts them together, are the captions the step would have
// sent whole, the words said in them and all: for an end and a start
// dragged a few seconds in and out of a clip of ten minutes, over 10
// seeds.
func TestADragStepSendsOnlyTheCaptionsNearTheChange(t *testing.T) {
	t.Parallel()
	for seed := range uint64(10) {
		r := rand.New(rand.NewPCG(seed, 13))
		fsys := afero.NewMemMapFs()
		path := filepath.Join("/", "ep.framefairy", "logs", "clips.json")
		plan := `{"source": "ep.mp4", "plan_id": "p1", "revision": 1, "clips": [
		  {"id": "01", "slug": "long", "title": "Long", "keep": true,
		   "segments": [{"start": 20, "end": 300}, {"start": 303, "end": 620}]}]}`
		if err := afero.WriteFile(fsys, path, []byte(plan), 0o644); err != nil {
			t.Fatal(err)
		}
		s := planAt(t, fsys, path)
		tr := fromStored(randomTalk(r, 700), nil, 0, 0, nil)
		saved, err := ClipCaptionsView(s, "01", tr, nil)
		if err != nil {
			t.Fatal(err)
		}
		// A few steps of the hand from where the clip was, the way each
		// step of a drag follows the one before.
		// A step's track names what it draws: one name is never two
		// drawings.
		named := map[string]string{}
		for _, g := range []Gesture{
			{Kind: "trim", Edge: "end", From: 615 + r.Float64()*5},
			{Kind: "trim", Edge: "end", From: 620 + r.Float64()*5},
			{Kind: "trim", Edge: "start", From: 20 + r.Float64()*5},
			{Kind: "trim", Edge: "start", From: 15 + r.Float64()*5},
		} {
			g.To, g.ToWords, g.Frames = g.From, true, at25
			whole, err := ShapeClipView(s, "01", g, tr, 0.1, nil, nil)
			if err != nil {
				t.Fatal(err)
			}
			step, err := ShapeClipView(s, "01", g, tr, 0.1, nil, saved.Drawing())
			if err != nil {
				t.Fatal(err)
			}
			if step.Captions.Drawing().was != nil {
				t.Errorf("seed %d, %s at %.2f: the step holds the drawing it was made from", seed, g.Edge, g.From)
			}
			if !step.Captions.Drawing().InHand() || saved.Drawing().InHand() {
				t.Errorf("seed %d, %s at %.2f: the step is in the hand %v and the saved clip %v, want only the step",
					seed, g.Edge, g.From, step.Captions.Drawing().InHand(), saved.Drawing().InHand())
			}
			cut := step.Captions.Splice
			if cut == nil {
				t.Errorf("seed %d, %s at %.2f: the step sent every caption", seed, g.Edge, g.From)
				continue
			}
			if n := len(step.Captions.Captions); n > 40 {
				t.Errorf("seed %d, %s at %.2f: the step sent %d of %d captions", seed, g.Edge, g.From, n, len(whole.Captions.Captions))
			}
			got := append(append([]CaptionView{}, saved.Captions[:cut.Head]...), step.Captions.Captions...)
			for _, c := range saved.Captions[cut.Tail:] {
				c.Start, c.End = c.Start+cut.Shift, c.End+cut.Shift
				lines := make([]CaptionLineView, len(c.Lines))
				for i, l := range c.Lines {
					for _, w := range l.Words {
						w.Start, w.End = w.Start+cut.Shift, w.End+cut.Shift
						lines[i].Words = append(lines[i].Words, w)
					}
				}
				c.Lines = lines
				got = append(got, c)
			}
			drawn := roundedJSON(t, whole.Captions.Drawing().Laid)
			if was, ok := named[step.Captions.Track]; ok && was != drawn {
				t.Errorf("seed %d, %s at %.2f: the track %s names two drawings", seed, g.Edge, g.From, step.Captions.Track)
			}
			named[step.Captions.Track] = drawn
			if step.Captions.Track == saved.Track {
				t.Errorf("seed %d, %s at %.2f: the step has the track of the clip as saved", seed, g.Edge, g.From)
			}
			if a, b := roundedJSON(t, got), roundedJSON(t, whole.Captions.Captions); a != b {
				t.Errorf("seed %d, %s at %.2f: put together\n%s\nwhole\n%s", seed, g.Edge, g.From, a, b)
			}
		}
	}
}

// roundedJSON is v as JSON with every number to six decimals, so captions
// moved on the clip's clock compare with captions laid out there.
func roundedJSON(t *testing.T, v any) string {
	t.Helper()
	raw, err := json.Marshal(v)
	if err != nil {
		t.Fatal(err)
	}
	var plain any
	if err := json.Unmarshal(raw, &plain); err != nil {
		t.Fatal(err)
	}
	var round func(any) any
	round = func(v any) any {
		switch x := v.(type) {
		case float64:
			return math.Round(x*1e6) / 1e6
		case []any:
			for i := range x {
				x[i] = round(x[i])
			}
		case map[string]any:
			for k := range x {
				x[k] = round(x[k])
			}
		}
		return v
	}
	out, err := json.Marshal(round(plain))
	if err != nil {
		t.Fatal(err)
	}
	return string(out)
}
