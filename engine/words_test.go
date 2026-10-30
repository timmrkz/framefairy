package engine

import (
	"fmt"
	"math"
	"strings"
	"testing"

	"framefairy/engine/runes"
	"framefairy/engine/timed"
)

func near(a, b float64) bool { return math.Abs(a-b) < 1e-6 }

func cueTexts(cues []timed.Cue) string {
	texts := make([]string, len(cues))
	for i, c := range cues {
		texts[i] = c.Text
	}
	return strings.Join(texts, " ")
}

func TestTokensToWords(t *testing.T) {
	tokens := []Token{
		{" Al", 0.00, 0.24}, {"les", 0.24, 0.16}, {" hat", 0.40, 0.24},
		{" En", 0.80, 0.16}, {"de", 0.96, 0.16}, {",", 1.12, 0.16},
		{" nur", 1.28, 0.16}, {" .", 1.44, 0.08},
	}
	words := TokensToWords(tokens, 10)
	want := []timed.Cue{{Start: 10.00, End: 10.40, Text: "Alles"}, {Start: 10.40, End: 10.64, Text: "hat"},
		{Start: 10.80, End: 11.12, Text: "Ende,"}, {Start: 11.28, End: 11.44, Text: "nur."}}
	if len(words) != len(want) {
		t.Fatalf("got %v", words)
	}
	for i := range want {
		if words[i].Text != want[i].Text || !near(words[i].Start, want[i].Start) ||
			!near(words[i].End, want[i].End) {
			t.Errorf("word %d = %+v, want %+v", i, words[i], want[i])
		}
	}
	// A number comes as a lone space and then its digits, and the space
	// is what keeps it apart from the word before it.
	numbers := []Token{{" dass", 0, 0.2}, {" ", 0.2, 0.08}, {"5", 0.28, 0.08},
		{"0", 0.36, 0.08}, {"0", 0.44, 0.08}, {" Euro", 0.52, 0.2}, {".", 0.72, 0.04},
		{" ", 0.8, 0.08}, {"1", 0.88, 0.08}, {"4", 0.96, 0.08}}
	got := TokensToWords(numbers, 0)
	if texts := cueTexts(got); texts != "dass 500 Euro. 14" {
		t.Errorf("got %q", texts)
	}
	if !near(got[1].Start, 0.28) || !near(got[1].End, 0.52) {
		t.Errorf("500 is timed %+v", got[1])
	}
	// A chunk that starts without a leading space still starts a word.
	if got := TokensToWords([]Token{{"Hallo", 0, 0.2}}, 0); len(got) != 1 || got[0].Text != "Hallo" {
		t.Errorf("got %v", got)
	}
}

// frames builds 10 ms loudness frames, loud wherever a span says so.
func frames(length float64, loud ...timed.Span) []float32 {
	out := make([]float32, int(length/timed.FrameSeconds))
	for i := range out {
		at := float64(i) * timed.FrameSeconds
		out[i] = -70
		for _, s := range loud {
			if at >= s.Start-1e-9 && at < s.End-1e-9 {
				out[i] = -20
			}
		}
	}
	return out
}

func TestSnapWords(t *testing.T) {
	sound := frames(6, timed.Span{Start: 1.00, End: 1.50}, timed.Span{Start: 2.00, End: 2.60}, timed.Span{Start: 2.60, End: 3.00}, timed.Span{Start: 4.20, End: 4.30})
	words := []timed.Cue{
		{Start: 0.80, End: 1.36, Text: "early"},  // starts in the pause before its sound, and ends before it does
		{Start: 2.08, End: 2.64, Text: "late"},   // starts just after its onset
		{Start: 2.64, End: 3.60, Text: "long"},   // runs on into the pause after it
		{Start: 3.90, End: 4.00, Text: "inside"}, // placed entirely in the pause before its sound
	}
	got := SnapWords(words, sound, 0, -40)
	checks := []struct {
		start, end float64
	}{{1.00, 1.50}, {2.00, 2.64}, {2.64, 3.00}, {4.20, 4.30}}
	for i, c := range checks {
		if !near(got[i].Start, c.start) || !near(got[i].End, c.end) {
			t.Errorf("%s = %.2f-%.2f, want %.2f-%.2f", got[i].Text, got[i].Start, got[i].End,
				c.start, c.end)
		}
	}
}

// The recogniser gives a word's last piece at most four of its 80 ms
// steps, so a word held before a pause ends while it is still being said.
// It ends where its sound does, when that sound stops before the next
// word and within holdOn of where the recogniser ended it.
func TestSnapWordsFollowsAWordToTheEndOfItsSound(t *testing.T) {
	sound := frames(8, timed.Span{Start: 1.00, End: 1.75}, timed.Span{Start: 2.00, End: 2.40}, timed.Span{Start: 3.00, End: 6.20}, timed.Span{Start: 6.50, End: 6.95})
	got := SnapWords([]timed.Cue{
		{Start: 1.00, End: 1.40, Text: "held"},  // said until 1.75, a pause follows
		{Start: 2.00, End: 2.10, Text: "zwei"},  // said until 2.40, ended at 2.10
		{Start: 3.00, End: 3.20, Text: "drei"},  // the sound goes on into the next word
		{Start: 3.50, End: 3.70, Text: "vier"},  // and on past holdOn, room noise or music
		{Start: 6.50, End: 6.60, Text: "fuenf"}, // said until 6.95, within holdOn
	}, sound, 0, -40)
	want := []timed.Span{{Start: 1.00, End: 1.75}, {Start: 2.00, End: 2.40}, {Start: 3.00, End: 3.20}, {Start: 3.50, End: 3.70}, {Start: 6.50, End: 6.95}}
	for i, w := range want {
		if !near(got[i].Start, w.Start) || !near(got[i].End, w.End) {
			t.Errorf("%s = %.2f-%.2f, want %.2f-%.2f", got[i].Text, got[i].Start, got[i].End, w.Start, w.End)
		}
	}
}

// The word before a clip ran on into the word the clip began with: "ich."
// ended where the clip began, its sound faded straight into "Also", and
// the clip's captions began with "ich.". From Tim's episode, 34:18.
func TestSnapWordsLeavesSoundThatRunsIntoTheNextWord(t *testing.T) {
	sound := frames(3, timed.Span{Start: 1.00, End: 2.30})
	got := SnapWords([]timed.Cue{{Start: 1.00, End: 1.23, Text: "ich."}, {Start: 1.31, End: 1.47, Text: "Also,"}}, sound, 0, -40)
	if !near(got[0].End, 1.23) {
		t.Errorf("ich. ends at %.2f, want 1.23", got[0].End)
	}
	clip := Clip{Segments: []Segment{{Start: 1.23, End: 2.3}}}
	if said := cueTexts(Said(clip, got)); said != "Also," {
		t.Errorf("the clip says %q", said)
	}
}

// A word the recogniser heard as one can hold a breath between its parts.
// That silence is not where the word ends, because more of it follows. Only
// the silence after its last sound is.
func TestSnapWordsKeepsAWordWholeAcrossABreathInside(t *testing.T) {
	sound := frames(4, timed.Span{Start: 1.00, End: 1.80}, timed.Span{Start: 2.00, End: 2.50})
	got := SnapWords([]timed.Cue{{Start: 1.00, End: 2.90, Text: "sweet-grundschulliebe."}}, sound, 0, -40)
	if !near(got[0].Start, 1.00) || !near(got[0].End, 2.50) {
		t.Errorf("the word runs %.2f-%.2f, want 1.00-2.50", got[0].Start, got[0].End)
	}
}

func TestSnapWordsLeavesSoftOnsetsAlone(t *testing.T) {
	// A single quiet frame at a word start is a soft consonant, not a pause.
	sound := frames(3, timed.Span{Start: 0.50, End: 1.00}, timed.Span{Start: 1.01, End: 2.00})
	got := SnapWords([]timed.Cue{{Start: 0.50, End: 1.00, Text: "eins"}, {Start: 1.00, End: 1.40, Text: "zwei"}}, sound, 0, -40)
	if !near(got[1].Start, 1.00) {
		t.Errorf("zwei moved to %.2f", got[1].Start)
	}
}

func words(spec string) []timed.Cue {
	var out []timed.Cue
	at := 0.0
	for _, w := range strings.Fields(spec) {
		if strings.HasPrefix(w, "+") {
			var gap float64
			for _, r := range w[1:] {
				gap = gap*10 + float64(r-'0')
			}
			at += gap / 10
			continue
		}
		out = append(out, timed.Cue{Start: at, End: at + 0.3, Text: w})
		at += 0.3
	}
	return out
}

func TestBuildLines(t *testing.T) {
	// A pause ends a line, a sentence end only once the line has substance.
	ws := words("Ja. das war damals so eine Sache, die ich nie vergessen werde. +8 Und dann")
	lines := BuildLines(ws, nil, nil)
	if len(lines) != 2 {
		t.Fatalf("got %d lines: %v", len(lines), lines)
	}
	if lines[0].Text() != "Ja. das war damals so eine Sache, die ich nie vergessen werde." {
		t.Errorf("line 1 = %q", lines[0].Text())
	}
	if !near(lines[1].GapBefore, 0.8) {
		t.Errorf("gap = %v", lines[1].GapBefore)
	}

	// A --max-pause below the default makes shorter pauses end lines too.
	short := 0.2
	if n := len(BuildLines(words("eins +3 zwei"), nil, &short)); n != 2 {
		t.Errorf("got %d lines with a 0.2 s max pause", n)
	}

	// Past 14 seconds a line breaks at the last sentence end inside it.
	long := strings.Repeat("wort ", 30) + "Ende. " + strings.Repeat("wort ", 25)
	lines = BuildLines(words(long), nil, nil)
	if len(lines) < 2 || !strings.HasSuffix(lines[0].Text(), "Ende.") {
		t.Errorf("long line split as %q", lines[0].Text())
	}
	for _, l := range lines {
		if l.Duration() > maxLineSeconds+1e-9 {
			t.Errorf("line of %.1f s", l.Duration())
		}
	}
}

func TestSegmentsDoNotReachIntoDroppedWords(t *testing.T) {
	// Three lines with no pause between them. Keeping 1 and 3 must not let
	// the padding pull in the edges of line 2.
	lines := []Line{
		{Index: 1, Cues: []timed.Cue{{Start: 0.0, End: 1.0, Text: "eins."}}},
		{Index: 2, Cues: []timed.Cue{{Start: 1.02, End: 2.0, Text: "zwei."}}},
		{Index: 3, Cues: []timed.Cue{{Start: 2.03, End: 3.0, Text: "drei."}}},
	}
	segs := SegmentsFromRanges([][2]int{{1, 1}, {3, 3}}, lines, 0.1, nil)
	if len(segs) != 2 {
		t.Fatalf("got %v", segs)
	}
	if !near(segs[0].Start, 0) || !near(segs[0].End, 1.02) {
		t.Errorf("first segment %.2f-%.2f", segs[0].Start, segs[0].End)
	}
	if !near(segs[1].Start, 2.0) || !near(segs[1].End, 3.1) {
		t.Errorf("second segment %.2f-%.2f", segs[1].Start, segs[1].End)
	}
}

func TestCaptionsFollowWords(t *testing.T) {
	clip := Clip{Segments: []Segment{{Start: 10, End: 13}, {Start: 20, End: 22}}}
	words := []timed.Cue{
		{Start: 10.1, End: 10.4, Text: "Als"}, {Start: 10.4, End: 10.7, Text: "Kind"}, {Start: 10.7, End: 11.2, Text: "stand"},
		{Start: 11.2, End: 11.5, Text: "ich"}, {Start: 11.5, End: 12.0, Text: "dort."},
		{Start: 15.0, End: 15.5, Text: "weg"}, // not in any segment
		{Start: 20.2, End: 20.6, Text: "Und"}, {Start: 20.6, End: 21.4, Text: "dann"},
	}
	cues := Captions(clip, words, 38, nil)
	if len(cues) != 2 {
		t.Fatalf("got %v", cues)
	}
	if cues[0].Text != "Als Kind stand ich dort." || cues[0].Start != 0 {
		t.Errorf("caption 1 = %+v", cues[0])
	}
	// The second segment starts 3 s into the clip, so "Und" is at 3.2 s.
	if cues[1].Text != "Und dann" || !near(cues[1].Start, 3.2) {
		t.Errorf("caption 2 = %+v", cues[1])
	}
	// A pause of 1.2 s after "dort." means the caption goes shortly after it.
	if !near(cues[0].End, 2.0+captionHold) {
		t.Errorf("caption 1 ends at %.2f", cues[0].End)
	}
	// The clip goes on 0.6 s after "dann", so the last caption goes a hold
	// after its word, like any other, and is not stretched to the end.
	if !near(cues[1].End, 4.4+captionHold) {
		t.Errorf("last caption ends at %.2f, want %.2f", cues[1].End, 4.4+captionHold)
	}
	for _, c := range cues {
		if strings.Contains(c.Text, "weg") {
			t.Errorf("a word outside the segments was captioned")
		}
	}
}

func TestCaptionsBreakAtWidth(t *testing.T) {
	var ws []timed.Cue
	for i := 0; i < 12; i++ {
		ws = append(ws, timed.Cue{Start: float64(i) * 0.3, End: float64(i)*0.3 + 0.3, Text: "Zielgruppe"})
	}
	clip := Clip{Segments: []Segment{{Start: 0, End: 4}}}
	captions := Captions(clip, ws, 38, nil)
	said := 0
	for _, c := range captions {
		if runes.Len(c.Text) > 38 {
			t.Errorf("caption %q is %d characters", c.Text, runes.Len(c.Text))
		}
		said += strings.Count(c.Text, "Zielgruppe")
	}
	// Every word is in a caption, so no caption was kept short by leaving
	// words out, and no more than three of them fit in 38 characters.
	if said != len(ws) || len(captions) < len(ws)/3 {
		t.Errorf("%d captions say the word %d times, want %d", len(captions), said, len(ws))
	}
}

func TestLevelsFromFrames(t *testing.T) {
	tr := &Transcript{Frames: frames(1, timed.Span{Start: 0, End: 0.5}), Start: 5}
	levels := tr.Levels()
	if len(levels) != 10 || !near(levels[0].At, 5) || math.Abs(levels[0].Level+20) > 0.01 ||
		math.Abs(levels[9].Level+70) > 0.01 {
		t.Errorf("levels %v", levels)
	}
}

func TestQuietestCut(t *testing.T) {
	f := frames(32, timed.Span{Start: 0, End: 22}, timed.Span{Start: 22.5, End: 32})
	cut := quietestCut(f)
	at := float64(cut) * timed.FrameSeconds
	if at < 22.0 || at > 22.5 {
		t.Errorf("cut at %.2f, want inside the pause at 22.0-22.5", at)
	}
}

// A cut takes out what lies in it and nothing else. Tim cut most of the
// pause between two captions out of a clip, and the two became one, held
// over the cut, because the pause was measured on the clip's clock. In the
// short that was another caption from the one the episode has there.
func TestACutLeavesTheCaptionsAroundIt(t *testing.T) {
	words := []timed.Cue{
		{Start: 0.2, End: 0.6, Text: "Erst"}, {Start: 0.6, End: 1.0, Text: "die"}, {Start: 1.0, End: 1.6, Text: "Idee,"},
		{Start: 2.6, End: 3.0, Text: "dann"}, {Start: 3.0, End: 3.4, Text: "die"}, {Start: 3.4, End: 4.2, Text: "Zielgruppe."},
	}
	whole := Clip{Segments: []Segment{{Start: 0, End: 5}}}
	texts := func(cs []Caption) []string {
		out := make([]string, len(cs))
		for i, c := range cs {
			out[i] = c.Text
		}
		return out
	}
	want := texts(Captions(whole, words, 38, nil))
	if len(want) != 2 {
		t.Fatalf("the episode has %v", want)
	}

	// Most of the second of quiet between Idee and dann taken out.
	cut := Clip{Segments: []Segment{{Start: 0, End: 1.8}, {Start: 2.5, End: 5}}}
	got := Captions(cut, words, 38, nil)
	if fmt.Sprint(texts(got)) != fmt.Sprint(want) {
		t.Fatalf("with the pause cut: %v, without: %v", texts(got), want)
	}
	// The first caption goes where the cut begins, 1.8 into the clip, a
	// little before its hold would have ended, and never over the cut.
	if !near(got[0].End, 1.8) {
		t.Errorf("the first caption goes at %.2f, want 1.8", got[0].End)
	}
	// The second appears with dann, a tenth after the cut.
	if !near(got[1].Start, 1.9) {
		t.Errorf("the second caption appears at %.2f, want 1.9", got[1].Start)
	}

	// A cut over a word takes that word and leaves the captions as they
	// were around it.
	overWord := Clip{Segments: []Segment{{Start: 0, End: 0.6}, {Start: 1.0, End: 5}}}
	got = Captions(overWord, words, 38, nil)
	if fmt.Sprint(texts(got)) != "[Erst Idee, dann die Zielgruppe.]" {
		t.Errorf("with die cut out: %v", texts(got))
	}
}

// The last caption is held past the end of a clip only when it is on screen
// at the end, for the frames the render lands beyond it. Trimming the end
// over the last word made the caption before it the last one, and it was
// then held to the end too, so Tim saw a short caption turn into a long one,
// and two captions look to become one, as the edge moved.
func TestTheLastCaptionIsHeldOnlyWhenItIsShownAtTheEnd(t *testing.T) {
	words := []timed.Cue{
		{Start: 0.2, End: 0.6, Text: "Das"}, {Start: 0.6, End: 1.0, Text: "war's."},
		{Start: 2.5, End: 2.8, Text: "Ja."},
	}
	// Ending inside the hold of "Ja.", it is on screen at the end.
	whole := Captions(Clip{Segments: []Segment{{Start: 0, End: 3.0}}}, words, 38, nil)
	if len(whole) != 2 || whole[1].End < 3.0+1.0 {
		t.Fatalf("with Ja. the last caption ends at %v: %+v", whole[len(whole)-1].End, whole)
	}
	// Trimmed over "Ja.", the caption before keeps its own time.
	trimmed := Captions(Clip{Segments: []Segment{{Start: 0, End: 2.4}}}, words, 38, nil)
	if len(trimmed) != 1 || !near(trimmed[0].End, whole[0].End) {
		t.Errorf("trimmed over Ja. the caption before goes at %v, it went at %v untrimmed", trimmed[0].End, whole[0].End)
	}
}

// A cut that takes a caption's first word shows the caption from where the
// clip comes back, as a cut into the word does. It used to wait for the
// next word: a cut moved a frame further into a short first word took it,
// and the caption jumped past the pause after it.
func TestACutThatTakesTheFirstWordShowsTheCaptionWhereItEnds(t *testing.T) {
	words := []timed.Cue{
		{Start: 3.0, End: 3.6, Text: "vorher"},
		{Start: 6.2, End: 6.4, Text: "Und"}, {Start: 6.6, End: 7.3, Text: "dann"}, {Start: 7.3, End: 7.9, Text: "kam"}, {Start: 7.9, End: 8.5, Text: "er."},
	}
	last := 0.0
	for _, end := range []float64{6.30, 6.35, 6.38, 6.39, 6.40, 6.45, 6.50, 6.55} {
		clip := Clip{Segments: []Segment{{Start: 2.8, End: 4.0}, {Start: end, End: 11}}}
		cs := Captions(clip, words, 38, nil)
		at := EpisodeTime(clip, cs[1].Start)
		if !near(at, end) {
			t.Errorf("with the cut ending at %.2f the caption %q appears at %.3f, want where the cut ends", end, cs[1].Text, at)
		}
		if at < last {
			t.Errorf("the caption went back from %.3f to %.3f", last, at)
		}
		last = at
	}
	// Words with a pause before them are not carried back to the cut.
	clip := Clip{Segments: []Segment{{Start: 2.8, End: 4.0}, {Start: 4.5, End: 11}}}
	if cs := Captions(clip, words, 38, nil); !near(EpisodeTime(clip, cs[1].Start), 6.2) {
		t.Errorf("a caption after a pause appears at %.3f, want with its word at 6.2", EpisodeTime(clip, cs[1].Start))
	}
}
