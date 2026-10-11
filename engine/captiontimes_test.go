package engine

import (
	"bytes"
	"fmt"
	"math"
	"os"
	"path/filepath"
	"reflect"
	"strings"
	"testing"
)

// Two captions in two pieces of a clip: "Als Kind stand ich dort." from 10
// to 12 in the episode, 0 to 2 in the clip, and "Und dann" from 20.2, 3.2
// in the clip.
func timedClip() Clip {
	return Clip{Segments: []Segment{{Start: 10, End: 13}, {Start: 20, End: 22}}}
}

// timedWords are the words of the episode timedClip says.
func timedWords() []Cue {
	return []Cue{
		{10.1, 10.4, "Als"}, {10.4, 10.7, "Kind"}, {10.7, 11.2, "stand"},
		{11.2, 11.5, "ich"}, {11.5, 12.0, "dort."},
		{20.2, 20.6, "Und"}, {20.6, 21.4, "dann"},
	}
}

func moved(start, end *float64) CaptionTime { return CaptionTime{Start: start, End: end} }

func at(v float64) *float64 { return &v }

func TestTheClocksOfAClipAndItsEpisodeMeet(t *testing.T) {
	clip := timedClip()
	for _, c := range []struct{ clip, episode float64 }{{0, 10}, {2.5, 12.5}, {3, 20}, {4.2, 21.2}} {
		if got := EpisodeTime(clip, c.clip); !near(got, c.episode) {
			t.Errorf("%.1f in the clip is %.2f in the episode, want %.1f", c.clip, got, c.episode)
		}
		if got := ClipTime(clip, c.episode); !near(got, c.clip) {
			t.Errorf("%.1f in the episode is %.2f in the clip, want %.1f", c.episode, got, c.clip)
		}
	}
	// A moment the clip cuts out is the moment the clip comes back.
	if got := ClipTime(clip, 16); !near(got, 3) {
		t.Errorf("a moment cut out is %.2f in the clip", got)
	}
}

func TestACaptionIsShownEarlier(t *testing.T) {
	clip := timedClip()
	natural := Captions(clip, timedWords(), 38, nil)
	// "Und dann" shown 0.15 s before "Und" is heard: 3.05 in the clip. It
	// lies in the pause after the first caption has gone, so nothing else
	// moves.
	clip.CaptionTimes = map[string]CaptionTime{wordKey(20.2): moved(at(20.05), nil)}
	got := Captions(clip, timedWords(), 38, nil)
	if !near(got[1].Start, 3.05) || !near(got[0].End, natural[0].End) {
		t.Errorf("got %+v, was %+v", got, natural)
	}
	// Earlier than the piece it is in begins is a moment the clip cuts out,
	// so it is shown the moment the clip comes back.
	clip.CaptionTimes = map[string]CaptionTime{wordKey(20.2): moved(at(19.8), nil)}
	if got := Captions(clip, timedWords(), 38, nil); !near(got[1].Start, 3.0) {
		t.Errorf("a start in what the clip cuts out: %+v", got[1])
	}
	// Earlier than the caption before it goes, which a plan of an older
	// version kept: it is shown where it was put, and the caption before
	// goes there, the way that version showed it. Moving an edge now never
	// puts it there, see TestACaptionEdgeStopsAtTheCaptionBeside.
	for _, early := range []float64{11.9, 0} {
		clip.CaptionTimes = map[string]CaptionTime{wordKey(20.2): moved(at(early), nil)}
		got = Captions(clip, timedWords(), 38, nil)
		want := math.Max(ClipTime(clip, early), natural[0].Start+shortestMoved)
		if !near(got[0].Start, natural[0].Start) || !near(got[1].Start, want) || !near(got[0].End, want) {
			t.Errorf("a start kept at %.1f over the caption before: %+v, was %+v", early, got, natural)
		}
	}
}

// A caption that appears later leaves the caption before it as it was,
// where the two met. It used to take it along.
func TestACaptionShownLaterLeavesTheOneBeforeAlone(t *testing.T) {
	clip := timedClip()
	words := []Cue{{10.1, 10.4, "Als"}, {10.4, 10.7, "Kind"}, {10.7, 11.0, "stand."},
		{11.0, 11.3, "Und"}, {11.3, 11.6, "dann"}, {11.6, 12.0, "ich."}}
	natural := Captions(clip, words, 12, nil)
	if len(natural) < 2 || !near(natural[0].End, natural[1].Start) {
		t.Fatalf("two captions that meet were wanted: %+v", natural)
	}
	clip.CaptionTimes = map[string]CaptionTime{wordKey(natural[1].Opens): moved(at(EpisodeTime(clip, natural[1].Start)+0.3), nil)}
	got := Captions(clip, words, 12, nil)
	if !near(got[0].End, natural[0].End) || !near(got[1].Start, natural[1].Start+0.3) {
		t.Errorf("shown later: %+v, was %+v", got, natural)
	}
}

func TestACaptionGoesEarlierAndStaysLonger(t *testing.T) {
	clip := timedClip()
	natural := Captions(clip, timedWords(), 38, nil)
	// Gone half a second after "dort." starts: the next is where it was.
	clip.CaptionTimes = map[string]CaptionTime{wordKey(11.5): moved(nil, at(12.0))}
	got := Captions(clip, timedWords(), 38, nil)
	if !near(got[0].End, 2.0) || !near(got[1].Start, natural[1].Start) {
		t.Errorf("gone earlier: %+v", got)
	}
	// Longer than the next one lets it: it stops where the next begins.
	clip.CaptionTimes = map[string]CaptionTime{wordKey(11.5): moved(nil, at(21))}
	got = Captions(clip, timedWords(), 38, nil)
	if !near(got[0].End, got[1].Start) {
		t.Errorf("stayed over the next caption: %+v", got)
	}
	// And a caption never ends before it began.
	clip.CaptionTimes = map[string]CaptionTime{wordKey(11.5): moved(nil, at(0))}
	got = Captions(clip, timedWords(), 38, nil)
	if got[0].End < got[0].Start+shortestMoved-1e-9 {
		t.Errorf("ended before it began: %+v", got)
	}
}

// A timing kept against a word that no longer begins a caption, because
// the captions break elsewhere at another size, is simply not used.
func TestATimingOnAWordInsideACaptionIsNotUsed(t *testing.T) {
	clip := timedClip()
	natural := Captions(clip, timedWords(), 38, nil)
	clip.CaptionTimes = map[string]CaptionTime{wordKey(10.7): moved(at(10.9), at(11.0))}
	got := Captions(clip, timedWords(), 38, nil)
	for i := range got {
		if !near(got[i].Start, natural[i].Start) || !near(got[i].End, natural[i].End) {
			t.Errorf("caption %d moved by a word in its middle: %+v", i, got[i])
		}
	}
}

func TestSetCaptionTimeKeepsItAgainstTheWord(t *testing.T) {
	path := editablePlanPath(t)
	tr := editableTranscript()
	if _, err := SetCaptionTime(filepath.Dir(path), path, "01", 10.0, "start", 9.8, tr, nil); err != nil {
		t.Fatal(err)
	}
	if _, err := SetCaptionTime(filepath.Dir(path), path, "01", 13.0, "end", 13.6, tr, nil); err != nil {
		t.Fatal(err)
	}
	c := clipsOf(t, path)["01"]
	if got := c.CaptionTimes[wordKey(10.0)].Start; got == nil || !near(*got, 9.8) {
		t.Errorf("start %v", got)
	}
	if got := c.CaptionTimes[wordKey(13.0)].End; got == nil || !near(*got, 13.6) {
		t.Errorf("end %v", got)
	}
	// Put back where the words put it, and nothing is left behind.
	if _, err := SetCaptionTime(filepath.Dir(path), path, "01", 10.0, "start", math.NaN(), tr, nil); err != nil {
		t.Fatal(err)
	}
	if _, err := SetCaptionTime(filepath.Dir(path), path, "01", 13.0, "end", math.NaN(), tr, nil); err != nil {
		t.Fatal(err)
	}
	body, _ := os.ReadFile(path)
	if strings.Contains(string(body), "caption_times") {
		t.Errorf("an empty record of moved captions was left:\n%s", body)
	}
	// Only a word of the clip, and only a moment of an episode.
	if _, err := SetCaptionTime(filepath.Dir(path), path, "01", 20.1, "start", 20, tr, nil); err == nil {
		t.Error("a word of another clip was taken")
	}
	if _, err := SetCaptionTime(filepath.Dir(path), path, "01", 10.0, "middle", 10, tr, nil); err == nil {
		t.Error("an edge that is not an edge was taken")
	}
	if _, err := SetCaptionTime(filepath.Dir(path), path, "01", 10.0, "start", math.Inf(1), tr, nil); err == nil {
		t.Error("a moment that is not one was taken")
	}
}

func TestCaptionTimesFromAPlanAreChecked(t *testing.T) {
	got := readCaptionTimes(map[string]any{
		"10100":  map[string]any{"start": 9.9},
		"x":      map[string]any{"start": 1.0},
		"010100": map[string]any{"start": 1.0},
		"-5":     map[string]any{"start": 1.0},
		"11500":  map[string]any{"end": -1.0, "start": "soon"},
		"12000":  "not an object",
	})
	if len(got) != 1 || got["10100"].Start == nil || *got["10100"].Start != 9.9 {
		t.Errorf("got %+v", got)
	}
}

// shownCaptions is what the captions of clip 01 of editablePlan read, with
// when each appears and goes on the clip's clock.
func shownCaptions(t *testing.T, path string, tr *Transcript) []string {
	t.Helper()
	view, err := ClipCaptionsView(path, "01", tr, nil)
	if err != nil {
		t.Fatal(err)
	}
	var out []string
	for _, c := range view.Captions {
		var words []string
		for _, line := range c.Lines {
			for _, w := range line.Words {
				words = append(words, w.Text)
			}
		}
		out = append(out, fmt.Sprintf("%.2f-%.2f %s", c.Start, c.End, strings.Join(words, " ")))
	}
	return out
}

// An edge of a caption dragged in over a word takes the word out of it,
// and dragged back out over it puts it back. Clip 01 of editablePlan reads
// "eins zwei" from 0 to 1.1 on its clock, and "drei vier" from 1.2. Found
// by Tim, who shortened a caption on the clip timeline and found its word
// still there.
func TestDraggingACaptionEdgeOverAWordRemovesIt(t *testing.T) {
	path := editablePlanPath(t)
	logs := filepath.Dir(path)
	tr := editableTranscript()
	was := shownCaptions(t, path, tr)
	if len(was) != 2 || !strings.HasSuffix(was[0], " eins zwei") || !strings.HasSuffix(was[1], " drei vier") {
		t.Fatalf("the clip reads %q", was)
	}
	// The end of "eins zwei" over the middle of "zwei", 10.8 in the
	// episode, to 10.7. The edge stays where it was let go, which is where
	// the drag showed it.
	if _, err := SetCaptionTime(logs, path, "01", 10.6, "end", 10.7, tr, nil); err != nil {
		t.Fatal(err)
	}
	if got := shownCaptions(t, path, tr); got[0] != "0.00-0.70 eins" || got[1] != was[1] {
		t.Errorf("the right edge dragged over zwei: %q, was %q", got, was)
	}
	// Back out over it, and zwei is back, the edge where it was let go.
	if _, err := SetCaptionTime(logs, path, "01", 10.6, "end", 11.05, tr, nil); err != nil {
		t.Fatal(err)
	}
	if got := shownCaptions(t, path, tr); got[0] != "0.00-1.05 eins zwei" || got[1] != was[1] {
		t.Errorf("the right edge dragged back over zwei: %q, was %q", got, was)
	}
	before := shownCaptions(t, path, tr)[0]
	// The start of "drei vier" over the middle of "drei", 12.2 in the
	// episode, to 12.5, which is 1.7 on the clip's clock. The caption
	// before stays as it was.
	if _, err := SetCaptionTime(logs, path, "01", 12.0, "start", 12.5, tr, nil); err != nil {
		t.Fatal(err)
	}
	if got := shownCaptions(t, path, tr); got[0] != before || got[1] != "1.70-3.30 vier" {
		t.Errorf("the left edge dragged over drei: %q", got)
	}
	// And over every word: the last one stays, since delete removes a
	// caption whole.
	if _, err := SetCaptionTime(logs, path, "01", 10.0, "start", 10.95, tr, nil); err != nil {
		t.Fatal(err)
	}
	if got := shownCaptions(t, path, tr); got[0] != "0.95-1.05 zwei" {
		t.Errorf("the left edge dragged over the whole caption: %q", got)
	}
}

// An edge is moved no further out than the caption beside it, so moving
// one changes no other: not the edge beside it, and not later, when that
// one is put back where its words put it. The caption before used to grow
// into the place a start had been moved to when its end was put back.
func TestACaptionEdgeStopsAtTheCaptionBeside(t *testing.T) {
	path := editablePlanPath(t)
	logs := filepath.Dir(path)
	tr := editableTranscript()
	was := shownCaptions(t, path, tr)
	// The start of "drei vier" out to 10.5, over all of "eins zwei".
	if _, err := SetCaptionTime(logs, path, "01", 12.0, "start", 10.5, tr, nil); err != nil {
		t.Fatal(err)
	}
	if got := shownCaptions(t, path, tr); got[0] != was[0] || got[1] != "1.10-3.30 drei vier" {
		t.Fatalf("a start moved over the caption before: %q, was %q", got, was)
	}
	// The end of "eins zwei" in to 10.9, and then put back: it stops where
	// "drei vier" appears, which stays where it was put.
	if _, err := SetCaptionTime(logs, path, "01", 10.6, "end", 10.9, tr, nil); err != nil {
		t.Fatal(err)
	}
	if _, err := SetCaptionTime(logs, path, "01", 10.6, "end", math.NaN(), tr, nil); err != nil {
		t.Fatal(err)
	}
	if got := shownCaptions(t, path, tr); got[0] != was[0] || got[1] != "1.10-3.30 drei vier" {
		t.Errorf("an end put back: %q, was %q", got, was)
	}
	// The end of "eins zwei" out over "drei vier" stops where it appears.
	if _, err := SetCaptionTime(logs, path, "01", 12.0, "start", math.NaN(), tr, nil); err != nil {
		t.Fatal(err)
	}
	if _, err := SetCaptionTime(logs, path, "01", 10.6, "end", 12.6, tr, nil); err != nil {
		t.Fatal(err)
	}
	if got := shownCaptions(t, path, tr); got[0] != "0.00-1.20 eins zwei" || got[1] != was[1] {
		t.Errorf("an end moved over the caption after: %q, was %q", got, was)
	}
}

// An edge moved by hand stays where it was put when the word it is kept
// against is removed in the caption box. It is kept against the last word
// the caption was laid out from, and a removed word is still laid out. It
// was kept against the last word shown, so the caption Tim had shortened
// grew back to where its words put it the moment he removed its last word.
// The moment it is moved to, 0.85 on the clip's clock, is not where the
// words would put it, 0.9, so the test sees which of the two it is.
func TestAMovedEdgeStaysWhenItsWordIsRemoved(t *testing.T) {
	path := editablePlanPath(t)
	logs := filepath.Dir(path)
	tr := editableTranscript()
	// The end of "eins zwei" moved from 1.1 to 0.85, short of the middle
	// of "zwei", 0.8, which stays.
	if _, err := SetCaptionTime(logs, path, "01", 10.6, "end", 10.85, tr, nil); err != nil {
		t.Fatal(err)
	}
	if got := shownCaptions(t, path, tr); got[0] != "0.00-0.85 eins zwei" {
		t.Fatalf("moved: %q", got)
	}
	if err := SetWordText(logs, 10.6, "", tr); err != nil {
		t.Fatal(err)
	}
	if got := shownCaptions(t, path, tr); got[0] != "0.00-0.85 eins" {
		t.Errorf("zwei removed in the caption box: %q", got)
	}
	if view, _ := ClipCaptionsView(path, "01", tr, nil); !view.Captions[0].EndMoved {
		t.Error("the edge where the hand put it is not said to be")
	}
}

// An edge moved out past where its words put it goes in to where the words
// left put it when its word is removed in the caption box, the same as
// when the edge is dragged in over the word, and it is then no longer where
// the hand put it. Typed back, the word takes the edge back out.
func TestAnEdgeMovedOutComesInWhenItsWordIsRemoved(t *testing.T) {
	path := editablePlanPath(t)
	logs := filepath.Dir(path)
	tr := editableTranscript()
	// The start of "drei vier" moved out from 1.2 to 1.15.
	if _, err := SetCaptionTime(logs, path, "01", 12.0, "start", 11.95, tr, nil); err != nil {
		t.Fatal(err)
	}
	if got := shownCaptions(t, path, tr); got[1] != "1.15-3.30 drei vier" {
		t.Fatalf("moved: %q", got)
	}
	if err := SetWordText(logs, 12.0, "", tr); err != nil {
		t.Fatal(err)
	}
	view, err := ClipCaptionsView(path, "01", tr, nil)
	if err != nil {
		t.Fatal(err)
	}
	if got := shownCaptions(t, path, tr); got[1] != "2.20-3.30 vier" || view.Captions[1].StartMoved {
		t.Errorf("drei removed in the caption box: %q, moved %v", got, view.Captions[1].StartMoved)
	}
	if err := SetWordText(logs, 12.0, "drei", tr); err != nil {
		t.Fatal(err)
	}
	if got := shownCaptions(t, path, tr); got[1] != "1.15-3.30 drei vier" {
		t.Errorf("drei typed back: %q", got)
	}
}

// A word that the start of the clip cuts into is passed by the start of
// its caption in the part of it the clip shows: "eins" is 10.0 to 10.5 in
// the episode and the clip starts at 10.3, so it shows 0 to 0.2 of it, and
// a start moved to 0.15 takes it out. The word keeps its own time on the
// clip's clock, -0.3 to 0.2, whose middle the start can never pass, so it
// stayed, while the drag had hidden it.
func TestAWordTheClipCutsIntoIsPassed(t *testing.T) {
	path := editablePlanPath(t)
	logs := filepath.Dir(path)
	tr := editableTranscript()
	if err := TrimClip(path, "01", 10.3, 13.1, tr, 0, ToFrames); err != nil {
		t.Fatal(err)
	}
	if got := shownCaptions(t, path, tr); !strings.HasSuffix(got[0], " eins zwei") {
		t.Fatalf("the clip reads %q", got)
	}
	if _, err := SetCaptionTime(logs, path, "01", 10.0, "start", 10.45, tr, nil); err != nil {
		t.Fatal(err)
	}
	if got := shownCaptions(t, path, tr); !strings.HasSuffix(got[0], " zwei") || strings.Contains(got[0], "eins") {
		t.Errorf("the start moved over the part of eins the clip shows: %q", got)
	}
}

// What moving an edge shows while the hand moves is what letting go of it
// saves, and showing it writes nothing.
func TestAnEdgeShowsWhatItSaves(t *testing.T) {
	path := editablePlanPath(t)
	logs := filepath.Dir(path)
	tr := editableTranscript()
	plan, _ := os.ReadFile(path)
	for _, step := range []struct {
		word float64
		edge string
		at   float64
	}{{10.6, "end", 10.7}, {10.6, "end", 11.05}, {12.0, "start", 12.5}, {12.0, "start", 10.5}, {10.0, "start", 10.95}} {
		view, err := ShapeCaptionTime(logs, path, "01", step.word, step.edge, step.at, tr, nil)
		if err != nil {
			t.Fatal(err)
		}
		if now, _ := os.ReadFile(path); !bytes.Equal(now, plan) {
			t.Fatal("showing an edge wrote the plan")
		}
		if _, err := os.Stat(correctionsPath(logs)); err == nil && step.at == 10.7 {
			t.Fatal("showing an edge wrote the corrections")
		}
		if _, err := SetCaptionTime(logs, path, "01", step.word, step.edge, step.at, tr, nil); err != nil {
			t.Fatal(err)
		}
		saved, err := ClipCaptionsView(path, "01", tr, nil)
		if err != nil {
			t.Fatal(err)
		}
		if !reflect.DeepEqual(view.Captions, saved.Captions) {
			t.Errorf("%s moved to %.2f showed %+v and saved %+v", step.edge, step.at, view.Captions, saved.Captions)
		}
		plan, _ = os.ReadFile(path)
	}
}

// Only an edge of a caption is moved: a word in the middle of one is no
// edge, and a start is not an end.
func TestOnlyACaptionEdgeIsMoved(t *testing.T) {
	path := editablePlanPath(t)
	logs := filepath.Dir(path)
	tr := editableTranscript()
	for _, c := range []struct {
		word float64
		edge string
	}{{10.6, "start"}, {10.0, "end"}, {13.0, "start"}} {
		if _, err := SetCaptionTime(logs, path, "01", c.word, c.edge, 10.7, tr, nil); err == nil {
			t.Errorf("the %s of a caption on the word at %.1f was moved", c.edge, c.word)
		}
		if _, err := ShapeCaptionTime(logs, path, "01", c.word, c.edge, 10.7, tr, nil); err == nil {
			t.Errorf("the %s of a caption on the word at %.1f was shown moved", c.edge, c.word)
		}
	}
}
