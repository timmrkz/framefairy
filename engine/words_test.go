package engine

import (
	"math"
	"os"
	"strings"
	"testing"
)

func near(a, b float64) bool { return math.Abs(a-b) < 1e-6 }

func TestTokensToWords(t *testing.T) {
	tokens := []Token{
		{" Al", 0.00, 0.24}, {"les", 0.24, 0.16}, {" hat", 0.40, 0.24},
		{" En", 0.80, 0.16}, {"de", 0.96, 0.16}, {",", 1.12, 0.16},
		{" nur", 1.28, 0.16}, {" .", 1.44, 0.08},
	}
	words := TokensToWords(tokens, 10)
	want := []Cue{{10.00, 10.40, "Alles"}, {10.40, 10.64, "hat"},
		{10.80, 11.12, "Ende,"}, {11.28, 11.44, "nur."}}
	if len(words) != len(want) {
		t.Fatalf("got %v", words)
	}
	for i := range want {
		if words[i].Text != want[i].Text || !near(words[i].Start, want[i].Start) ||
			!near(words[i].End, want[i].End) {
			t.Errorf("word %d = %+v, want %+v", i, words[i], want[i])
		}
	}
	// A chunk that starts without a leading space still starts a word.
	if got := TokensToWords([]Token{{"Hallo", 0, 0.2}}, 0); len(got) != 1 || got[0].Text != "Hallo" {
		t.Errorf("got %v", got)
	}
}

// frames builds 10 ms loudness frames, loud wherever a span says so.
func frames(length float64, loud ...Span) []float32 {
	out := make([]float32, int(length/FrameSeconds))
	for i := range out {
		at := float64(i) * FrameSeconds
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
	sound := frames(6, Span{1.00, 1.50}, Span{2.00, 2.60}, Span{2.60, 3.00}, Span{4.20, 4.30})
	words := []Cue{
		{0.80, 1.36, "early"},  // starts in the pause before its sound
		{2.08, 2.64, "late"},   // starts just after its onset
		{2.64, 3.60, "long"},   // runs on into the pause after it
		{3.90, 4.00, "inside"}, // placed entirely in the pause before its sound
	}
	got := SnapWords(words, sound, 0, -40)
	checks := []struct {
		start, end float64
	}{{1.00, 1.36}, {2.00, 2.64}, {2.64, 3.00}, {4.20, 4.25}}
	for i, c := range checks {
		if !near(got[i].Start, c.start) || !near(got[i].End, c.end) {
			t.Errorf("%s = %.2f-%.2f, want %.2f-%.2f", got[i].Text, got[i].Start, got[i].End,
				c.start, c.end)
		}
	}
}

func TestSnapWordsLeavesSoftOnsetsAlone(t *testing.T) {
	// A single quiet frame at a word start is a soft consonant, not a pause.
	sound := frames(3, Span{0.50, 1.00}, Span{1.01, 2.00})
	got := SnapWords([]Cue{{0.50, 1.00, "eins"}, {1.00, 1.40, "zwei"}}, sound, 0, -40)
	if !near(got[1].Start, 1.00) {
		t.Errorf("zwei moved to %.2f", got[1].Start)
	}
}

func words(spec string) []Cue {
	var out []Cue
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
		out = append(out, Cue{at, at + 0.3, w})
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
		{Index: 1, Cues: []Cue{{0.0, 1.0, "eins."}}},
		{Index: 2, Cues: []Cue{{1.02, 2.0, "zwei."}}},
		{Index: 3, Cues: []Cue{{2.03, 3.0, "drei."}}},
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
	clip := Clip{
		Segments: []Segment{{Start: 10, End: 13}, {Start: 20, End: 22}},
		Words: []Cue{
			{10.1, 10.4, "Als"}, {10.4, 10.7, "Kind"}, {10.7, 11.2, "stand"},
			{11.2, 11.5, "ich"}, {11.5, 12.0, "dort."},
			{15.0, 15.5, "weg"}, // not in any segment
			{20.2, 20.6, "Und"}, {20.6, 21.4, "dann"},
		},
	}
	cues := Captions(clip, 38)
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
	// The last caption is held past the end of the clip.
	if cues[1].End < clipLength(clip) {
		t.Errorf("last caption ends at %.2f", cues[1].End)
	}
	for _, c := range cues {
		if strings.Contains(c.Text, "weg") {
			t.Errorf("a word outside the segments was captioned")
		}
	}
}

func TestCaptionsBreakAtWidth(t *testing.T) {
	var ws []Cue
	for i := 0; i < 12; i++ {
		ws = append(ws, Cue{float64(i) * 0.3, float64(i)*0.3 + 0.3, "Zielgruppe"})
	}
	clip := Clip{Segments: []Segment{{Start: 0, End: 4}}, Words: ws}
	for _, c := range Captions(clip, 38) {
		if runeLen(c.Text) > 38 {
			t.Errorf("caption %q is %d characters", c.Text, runeLen(c.Text))
		}
	}
}

func TestLevelsFromFrames(t *testing.T) {
	tr := &Transcript{Frames: frames(1, Span{0, 0.5}), Start: 5}
	levels := tr.Levels()
	if len(levels) != 10 || !near(levels[0].At, 5) || math.Abs(levels[0].Level+20) > 0.01 ||
		math.Abs(levels[9].Level+70) > 0.01 {
		t.Errorf("levels %v", levels)
	}
}

func TestQuietestCut(t *testing.T) {
	f := frames(32, Span{0, 22}, Span{22.5, 32})
	cut := quietestCut(f)
	at := float64(cut) * FrameSeconds
	if at < 22.0 || at > 22.5 {
		t.Errorf("cut at %.2f, want inside the pause at 22.0-22.5", at)
	}
}

func TestAlignWordsKeepsUnchangedTiming(t *testing.T) {
	original := []Cue{{1.0, 1.3, "Ich"}, {1.3, 1.8, "merke,"}, {1.9, 2.2, "dass"},
		{2.2, 2.5, "das"}, {2.6, 3.0, "Werkstadt"}}
	got := AlignWords("Ich merke, dass das Werkstatt ist", 1.0, 3.4, original)
	if len(got) != 6 {
		t.Fatalf("got %v", got)
	}
	for i := 0; i < 4; i++ {
		if got[i].Start != original[i].Start || got[i].End != original[i].End {
			t.Errorf("unchanged word %q moved to %.2f-%.2f", got[i].Text, got[i].Start, got[i].End)
		}
	}
	// The corrected word and the added one share the time after "das".
	if got[4].Start < 2.5-1e-9 || got[5].End > 3.4+1e-9 || got[5].Start < got[4].End-1e-9 {
		t.Errorf("changed words at %v", got[4:])
	}
	if got[4].Text != "Werkstatt" || got[5].Text != "ist" {
		t.Errorf("texts %q %q", got[4].Text, got[5].Text)
	}
}

func TestAlignWordsWithoutOriginal(t *testing.T) {
	got := AlignWords("ab abcd", 2, 4, nil)
	if len(got) != 2 || !near(got[0].Start, 2) || !near(got[1].End, 4) ||
		!near(got[0].End, 2+2.0/3) {
		t.Errorf("got %v", got)
	}
}

func TestCaptionsRoundTripThroughEdits(t *testing.T) {
	dir := t.TempDir()
	path := dir + "/01_x.srt"
	caps := []Caption{{0, 2, "Als Kind stand", []Cue{{0.1, 0.4, "Als"}, {0.4, 0.8, "Kind"},
		{0.9, 1.5, "stand"}}}}
	if err := WriteCaptions(caps, path); err != nil {
		t.Fatal(err)
	}
	body, _ := os.ReadFile(path)
	edited := strings.Replace(string(body), "Als Kind stand", "Als kleines Kind stand", 1)
	os.WriteFile(path, []byte(edited), 0o644)
	got, err := LoadCaptions(path)
	if err != nil || len(got) != 1 || len(got[0].Words) != 4 {
		t.Fatalf("got %v, %v", got, err)
	}
	w := got[0].Words
	if !near(w[0].Start, 0.1) || !near(w[2].Start, 0.4) || !near(w[3].Start, 0.9) {
		t.Errorf("words %v", w)
	}
	if w[1].Text != "kleines" || w[1].Start < 0.4-1e-9 || w[1].End > 0.4+1e-9 {
		// "kleines" is squeezed into the zero gap before "Kind" and gets a
		// minimum slot, which is fine as long as it sits there.
		if w[1].Start < 0.1 || w[1].Start > 0.45 {
			t.Errorf("added word at %v", w[1])
		}
	}
}
