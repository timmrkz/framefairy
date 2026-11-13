package main

import (
	"context"
	"encoding/json"
	"fmt"
	"math"
	"math/rand/v2"
	"path/filepath"
	"reflect"
	"slices"
	"strings"
	"testing"

	"github.com/spf13/afero"

	"framefairy/engine"
)

// The rules every edit of the captions keeps, whatever came before it:
// an edit stays where it was made, an edge moves its own caption only, a
// caption removed is gone, a removal changes no other caption, and Undo
// puts back what was there before the step it takes back, and Redo what
// was there after. They were checked after every step of every walk, in
// the window, through the whole app. Here they are checked after every
// step of random edits through the app's own calls, in memory, over fixed
// seeds, so a failure is played again by its seed.

// captionedPlan is one clip over two sentences with a pause between,
// where the speech runs on inside each, with a cut in the first.
const captionedPlan = `{
  "source": "ep.mp4",
  "clips": [
    {"id": "01", "slug": "schulhof", "segments": [{"start": 9.9, "end": 13.0}, {"start": 13.4, "end": 22.0}]}
  ]
}`

// captionedWords are the words the episode says: the sentence the walks'
// video says, a pause, and a second one, quick words among them.
func captionedWords() [][3]any {
	text := "Ich war vielleicht sechs Jahre alt, als mich auf dem Schulhof irgendein Typ geschubst hat."
	var words [][3]any
	at := 10.0
	for _, w := range strings.Fields(text) {
		long := 0.25 + 0.03*float64(len(w))
		words = append(words, [3]any{at, at + long, w})
		at += long + 0.05
	}
	at += 1.2
	for i, w := range strings.Fields("Und dann so habe ich mich gewehrt, weil das ein echtes Thema für mich war.") {
		long := 0.2 + 0.03*float64(len(w))
		if i == 2 {
			// "so" is said faster than a frame.
			long = 0.03
		}
		words = append(words, [3]any{at, at + long, w})
		at += long + 0.04
	}
	return words
}

// captionEditor is a video of the library with captionedPlan and its
// words, edited through the app's calls the way the interface makes them.
type captionEditor struct {
	t    *testing.T
	svc  *FrameFairy
	mine string
	plan string
}

func newCaptionEditor(t *testing.T) *captionEditor {
	t.Helper()
	svc, eps := memoryLibrary(t, "ep.mp4")
	mine := eps[0]
	svc.jobs = newQueue(svc.store, func(JobUpdate) {}, func(string) {})
	// A video of 25 frames a second, the rate of Tim's, so the captions
	// are laid out on its frames the way the app lays them out.
	svc.probeVideo = func(context.Context, string) (engine.SourceInfo, error) {
		return engine.SourceInfo{Width: 1920, Height: 1080, FPSNum: 25, FPSDen: 1, Duration: 30}, nil
	}
	plan := filepath.Join(engine.LogsDir(mine), "clips.json")
	if err := svc.store.files.MkdirAll(engine.LogsDir(mine), 0o755); err != nil {
		t.Fatal(err)
	}
	if err := afero.WriteFile(svc.store.files, plan, []byte(captionedPlan), 0o644); err != nil {
		t.Fatal(err)
	}
	heardEpisode(t, svc, mine, 30, captionedWords())
	e := &captionEditor{t: t, svc: svc, mine: mine, plan: plan}
	if n := len(e.view().Captions); n < 4 {
		t.Fatalf("the test needs four captions or more, the clip has %d", n)
	}
	return e
}

// view is the clip's captions as the interface is given them.
func (e *captionEditor) view() *engine.CaptionsView {
	e.t.Helper()
	v, err := e.svc.Captions(e.mine, e.plan, "01")
	if err != nil {
		e.t.Fatal(err)
	}
	return v
}

// planFile is what the plan holds of its clips. An edit that writes the
// plan again with what it held is no step to take back, and neither is
// the count of its writes nor the shorts rendered from it.
func (e *captionEditor) planFile() map[string]any {
	e.t.Helper()
	body, err := afero.ReadFile(e.svc.store.files, e.plan)
	if err != nil {
		e.t.Fatal(err)
	}
	var held map[string]any
	if err := json.Unmarshal(body, &held); err != nil {
		e.t.Fatal(err)
	}
	delete(held, "revision")
	delete(held, "shorts")
	return held
}

// captionEdit is one edit the interface can make of the captions, and
// what it is called in a failure.
type captionEdit struct {
	name string
	// word is the word of the edit, where the episode says it: the word
	// corrected, the first word of the caption removed, or the word the
	// edge moved is on.
	word float64
	// text is what a word is corrected to, empty to remove it.
	text string
	// remove removes the caption that begins on word.
	remove bool
	// deletes removes the word, or with part from nought up, one of the
	// words a correction made of it, as delete does on the word framed.
	deletes bool
	part    int
	// edge is "start" or "end" for an edge moved to at, below nought to
	// put it back, with toWords for shift held.
	edge    string
	at      float64
	toWords bool
}

// make makes the edit through the app. A refused edit changes nothing,
// which the rules then hold it to, so whether it was made is read from
// the plan and not from the answer.
func (e *captionEditor) make(c captionEdit) {
	ctx := context.Background()
	switch {
	case c.edge != "":
		_, _ = e.svc.SetCaptionTime(ctx, e.mine, e.plan, "01", c.word, c.edge, c.at, c.toWords)
	case c.remove:
		_, _ = e.svc.RemoveCaption(ctx, e.mine, e.plan, "01", c.word)
	case c.deletes:
		_, _ = e.svc.RemoveWord(ctx, e.mine, e.plan, "01", c.word, c.part)
	default:
		_, _ = e.svc.SetWord(ctx, e.mine, e.plan, "01", c.word, c.text)
	}
}

// wordsOf are a clip's words, each as its text and when it is shown on
// the clip's clock, with where the episode says it, or NaN for a word
// that stands for no word heard.
type shownWord struct {
	text       string
	start, end float64
	said       float64
	// part is which of the words a correction made of a word heard this
	// is, counted from nought, or below nought for the whole of it.
	part int
}

func wordsOf(v *engine.CaptionsView) []shownWord {
	var out []shownWord
	for _, c := range v.Captions {
		for _, l := range c.Lines {
			for _, w := range l.Words {
				said, part := math.NaN(), -1
				if w.Said != nil {
					said = *w.Said
				}
				if w.Part != nil {
					part = *w.Part
				}
				out = append(out, shownWord{w.Text, w.Start, w.End, said, part})
			}
		}
	}
	return out
}

// wordsAwayFrom says the words said before from or after to, and the words
// that stand for no word heard, as text with when each is shown.
func wordsAwayFrom(v *engine.CaptionsView, from, to float64) string {
	var out []string
	for _, w := range wordsOf(v) {
		if math.IsNaN(w.said) || w.said < from || w.said > to {
			out = append(out, fmt.Sprintf("%s@%.3f-%.3f", w.text, w.start, w.end))
		}
	}
	return strings.Join(out, " ")
}

func spansOf(v *engine.CaptionsView) string {
	var out []string
	for _, c := range v.Captions {
		var words []string
		for _, l := range c.Lines {
			for _, w := range l.Words {
				words = append(words, w.Text)
			}
		}
		out = append(out, fmt.Sprintf("%.3f-%.3f %s", c.Start, c.End, strings.Join(words, " ")))
	}
	return strings.Join(out, " | ")
}

// sameClip says whether two answers show the same clip: its captions,
// their look and its pieces.
func sameClip(a, b *engine.CaptionsView) bool {
	key := func(v *engine.CaptionsView) string {
		body, _ := json.Marshal(struct {
			C []engine.CaptionView
			S engine.CaptionStyleView
			P []engine.PieceView
		}{v.Captions, v.Style, v.Pieces})
		return string(body)
	}
	return key(a) == key(b)
}

func close3(a, b float64) bool { return math.Abs(a-b) < 1e-3 }

// broken says which rule the edit c broke, going from before to after,
// or nothing.
func broken(c captionEdit, before, after *engine.CaptionsView) string {
	if c.edge != "" {
		return edgeBroken(c, before, after)
	}
	return removalBroken(c, before, after)
}

// An edge of a caption moved, or put back, moves that edge and nothing
// else: the other edge of the caption stays, every other caption appears
// and goes when it did, and the words outside the caption are as they
// were. The one exception: a start moved out into the time the caption
// before held over a word removed between them takes that time, so the
// caption before goes where this one now appears, #598, and a start put
// back gives it back. The caption before used to grow into the place a
// start was dragged in from, found by Tim.
func edgeBroken(c captionEdit, before, after *engine.CaptionsView) string {
	at := slices.IndexFunc(before.Captions, func(b engine.CaptionView) bool {
		if c.edge == "start" {
			return close3(b.First, c.word)
		}
		return close3(b.Last, c.word)
	})
	if at < 0 {
		return fmt.Sprintf("no caption had its %s on the word at %.3f", c.edge, c.word)
	}
	cue := before.Captions[at]
	if wordsAwayFrom(before, cue.First-1e-3, cue.Last+1e-3) != wordsAwayFrom(after, cue.First-1e-3, cue.Last+1e-3) {
		return "an edge moved changed a word outside its caption"
	}
	if len(after.Captions) != len(before.Captions) {
		return fmt.Sprintf("an edge moved left %d captions of %d", len(after.Captions), len(before.Captions))
	}
	moved := after.Captions[at]
	for k, now := range after.Captions {
		was := before.Captions[k]
		if k == at {
			if c.edge == "start" && !close3(now.End, was.End) || c.edge == "end" && !close3(now.Start, was.Start) {
				return "an edge moved moved the other edge of its caption"
			}
			continue
		}
		out := moved.Start < cue.Start-1e-3 || c.at < 0
		gives := c.edge == "start" && k == at-1 && out && close3(was.End, cue.Start) && close3(now.End, moved.Start)
		if !close3(now.Start, was.Start) || !close3(now.End, was.End) && !gives {
			return fmt.Sprintf("an edge moved moved caption %d beside it", k+1)
		}
	}
	return ""
}

// An edit of a word, or a caption removed, stays where it was made: every
// word more than three seconds of the episode away keeps its text and its
// time. A caption removed is gone, its words from the first to the last.
// Taking words out changes no other caption: every caption still there
// appears when it did and goes when it did, and the one a word was taken
// from only draws in to the words it still shows, at the edge the word
// was at. Where the speech runs on, the caption before one whose first
// word was taken out stays up until it appears, so nothing goes blank
// while someone talks, #597, and goes when it would on its own once the
// caption after it is removed.
func removalBroken(c captionEdit, before, after *engine.CaptionsView) string {
	till := c.word
	if c.remove {
		at := slices.IndexFunc(before.Captions, func(b engine.CaptionView) bool { return math.Abs(b.First-c.word) < 1e-6 })
		if at < 0 {
			return fmt.Sprintf("no caption began on %.3f", c.word)
		}
		till = before.Captions[at].Last
		if slices.ContainsFunc(after.Captions, func(a engine.CaptionView) bool { return math.Abs(a.First-c.word) < 1e-6 }) {
			return "a caption removed is still there"
		}
	}
	if wordsAwayFrom(before, c.word-3, till+3) != wordsAwayFrom(after, c.word-3, till+3) {
		return "an edit changed a word more than three seconds away"
	}
	if !c.remove && !c.deletes && c.text != "" {
		return ""
	}
	for k, now := range after.Captions {
		b := slices.IndexFunc(before.Captions, func(b engine.CaptionView) bool { return close3(b.First, now.First) })
		if b < 0 {
			return fmt.Sprintf("caption %d is new", k+1)
		}
		was := before.Captions[b]
		starts, ends := false, false
		if !c.remove {
			said := saidIn(was)
			starts, ends = close3(c.word, said[0]), close3(c.word, said[len(said)-1])
		}
		held := false
		if !c.remove && k+1 < len(after.Captions) {
			next := after.Captions[k+1]
			n := slices.IndexFunc(before.Captions, func(b engine.CaptionView) bool { return close3(b.First, next.First) })
			held = n >= 0 && close3(was.End, before.Captions[n].Start) && close3(now.End, next.Start)
		}
		// A caption that stayed up until the next appeared goes when it
		// would on its own once the next is gone, and leaves the time
		// empty.
		lets := b+1 < len(before.Captions) && close3(was.End, before.Captions[b+1].Start) && now.End < was.End &&
			!slices.ContainsFunc(after.Captions, func(a engine.CaptionView) bool { return close3(a.First, before.Captions[b+1].First) })
		startOK := close3(now.Start, was.Start) || starts && now.Start >= was.Start-1e-3
		endOK := close3(now.End, was.End) || held || lets || ends && now.End <= was.End+1e-3
		if !startOK || !endOK {
			return fmt.Sprintf("taking words out moved caption %d", k+1)
		}
	}
	return ""
}

// saidIn are where the episode says the words a caption shows.
func saidIn(c engine.CaptionView) []float64 {
	var out []float64
	for _, l := range c.Lines {
		for _, w := range l.Words {
			if w.Said != nil {
				out = append(out, *w.Said)
			}
		}
	}
	if len(out) == 0 {
		return []float64{math.NaN()}
	}
	return out
}

// typed are the words typed in the random walks of the window: some the
// video says, some it never does, and two at once.
var typed = []string{"auf", "dem", "Schulhof", "Typ", "eine", "Arm", "ja", "Kaffee", "zwei Worte"}

// randomWordEdit is an edit of a word of v the caption box makes: a word
// replaced, a word typed at its end or its start, which puts a word
// removed back where the video says it, a word cleared, or a word framed
// and deleted.
func randomWordEdit(r *rand.Rand, v *engine.CaptionsView) captionEdit {
	said := saidWords(v)
	w := said[r.IntN(len(said))]
	word := typed[r.IntN(len(typed))]
	edit := captionEdit{word: w.said, part: -1}
	switch r.IntN(5) {
	case 0:
		edit.text = word
	case 1:
		edit.text = w.text + " " + word
	case 2:
		edit.text = word + " " + w.text
	case 3:
		edit.name = fmt.Sprintf("%q at %.3f cleared", w.text, w.said)
		return edit
	default:
		edit.deletes = true
		edit.name = fmt.Sprintf("%q at %.3f, part %d, deleted", w.text, w.said, w.part)
		return edit
	}
	edit.name = fmt.Sprintf("%q at %.3f corrected to %q", w.text, w.said, edit.text)
	return edit
}

func saidWords(v *engine.CaptionsView) []shownWord {
	var out []shownWord
	for _, w := range wordsOf(v) {
		if !math.IsNaN(w.said) {
			out = append(out, w)
		}
	}
	return out
}

func TestRandomCaptionEditsKeepTheirRules(t *testing.T) {
	for seed := range uint64(16) {
		t.Run(fmt.Sprint("seed ", seed), func(t *testing.T) {
			t.Parallel()
			r := rand.New(rand.NewPCG(seed, 0))
			e := newCaptionEditor(t)
			// The clip before each step, for Undo, and after each step
			// taken back, for Redo.
			var undone, redone []*engine.CaptionsView
			now := e.view()
			var done []string
			for range 40 {
				before, file := now, e.planFile()
				var step string
				switch k := r.IntN(8); {
				case k == 0 || k == 1:
					back := k == 0
					step = map[bool]string{true: "undo", false: "redo"}[back]
					var got Undone
					var err error
					if back {
						got, err = e.svc.Undo(e.mine)
					} else {
						got, err = e.svc.Redo(e.mine)
					}
					now = e.view()
					from, to := &undone, &redone
					if !back {
						from, to = to, from
					}
					switch {
					case err != nil:
						t.Fatalf("after %s\n%s: %v", strings.Join(done, "\n"), step, err)
					case len(*from) == 0 && (got.Done || !sameClip(now, before)):
						t.Fatalf("after %s\n%s with nothing to take back did %+v\nthen %s\nnow  %s", strings.Join(done, "\n"), step, got, spansOf(before), spansOf(now))
					case len(*from) > 0:
						want := (*from)[len(*from)-1]
						*from = (*from)[:len(*from)-1]
						*to = append(*to, before)
						if !got.Done || !sameClip(now, want) {
							t.Fatalf("after %s\n%s does not put it back\nwant %s\nnow  %s", strings.Join(done, "\n"), step, spansOf(want), spansOf(now))
						}
					}
				case len(now.Captions) == 0:
					// Every caption is removed, and there is nothing to
					// edit until one is taken back.
					step = "nothing to edit"
				default:
					c := randomWordEdit(r, now)
					step = c.name
					e.make(c)
					now = e.view()
					if reflect.DeepEqual(e.planFile(), file) {
						if !sameClip(now, before) {
							t.Fatalf("after %s\n%s changed the captions and not the plan", strings.Join(done, "\n"), step)
						}
						break
					}
					undone = append(undone, before)
					redone = nil
					if why := broken(c, before, now); why != "" {
						t.Fatalf("after %s\n%s: %s\nthen %s\nnow  %s", strings.Join(done, "\n"), step, why, spansOf(before), spansOf(now))
					}
				}
				done = append(done, step)
			}
		})
	}
}

// Every word corrected, removed or typed into two, and every caption
// removed, from the clip as it comes, keeps the rules, and so does
// every edge of every caption moved in, out and put back.
func TestEveryCaptionEditKeepsTheRules(t *testing.T) {
	t.Parallel()
	first := newCaptionEditor(t).view()
	var edits []captionEdit
	for _, w := range saidWords(first) {
		for _, text := range []string{"Hof", "", "zwei Worte"} {
			edits = append(edits, captionEdit{name: fmt.Sprintf("%q at %.3f corrected to %q", w.text, w.said, text), word: w.said, text: text})
		}
	}
	for _, c := range first.Captions {
		edits = append(edits, captionEdit{name: fmt.Sprintf("the caption on %.3f removed", c.First), word: c.First, remove: true})
		for _, m := range []struct {
			edge  string
			word  float64
			moves []float64
		}{{"start", c.First, []float64{0.3, -0.5, -2}}, {"end", c.Last, []float64{-0.3, 0.5, 2}}} {
			for _, by := range m.moves {
				for _, toWords := range []bool{false, true} {
					edits = append(edits, captionEdit{name: fmt.Sprintf("the %s of the caption on %.3f moved by %.1f, on words %v", m.edge, m.word, by, toWords),
						word: m.word, edge: m.edge, at: m.word + by, toWords: toWords})
				}
			}
		}
	}
	for _, c := range edits {
		t.Run(c.name, func(t *testing.T) {
			t.Parallel()
			e := newCaptionEditor(t)
			before := e.view()
			e.make(c)
			after := e.view()
			if why := broken(c, before, after); why != "" {
				t.Errorf("%s\nthen %s\nnow  %s", why, spansOf(before), spansOf(after))
			}
			if c.edge == "" {
				return
			}
			// And put back, the double-click on the edge.
			back := c
			back.at = -1
			e.make(back)
			if why := broken(back, after, e.view()); why != "" {
				t.Errorf("put back: %s\nthen %s\nnow  %s", why, spansOf(after), spansOf(e.view()))
			}
		})
	}
}
