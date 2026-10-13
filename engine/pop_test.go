package engine

import (
	"math"
	"testing"
)

// The pop as drawn: from rest to popPeak and back, eased out on the way up
// and on the way down, in straight pieces that meet the curve at every
// knot.
func TestThePopRisesToItsPeakAndSettles(t *testing.T) {
	rise := popLength * popRise
	if got := popDrawn(0); got != 1 {
		t.Errorf("the word starts at %.3f, not at rest", got)
	}
	if got := popDrawn(popLength); got != 1 {
		t.Errorf("the word ends its pop at %.3f, not at rest", got)
	}
	top, at := 0.0, 0.0
	last := 1.0
	for ms := 0.0; ms <= popLength; ms++ {
		s := popDrawn(ms)
		if s < 1 || s > popPeak+1e-9 {
			t.Errorf("%.0f ms into its pop the word is at %.3f", ms, s)
		}
		// Up until the peak and down after it, never back.
		if (ms <= rise-10 && s < last-1e-9) || (ms >= rise+10 && s > last+1e-9) {
			t.Errorf("%.0f ms into its pop the word goes from %.4f to %.4f", ms, last, s)
		}
		if s > top {
			top, at = s, ms
		}
		last = s
	}
	if top < popPeak-0.01 || math.Abs(at-rise) > 20 {
		t.Errorf("the pop peaks at %.3f %.0f ms in, not at %.2f near %.0f ms", top, at, popPeak, rise)
	}
	// Eased out: further than a third of the way up a third into the
	// rise, where a straight line would be.
	if popDrawn(rise/3) < 1+(popPeak-1)*0.42 {
		t.Errorf("a third into the rise the word is at %.3f, not eased out", popDrawn(rise/3))
	}
}

// Every word pops on the one curve, a word spoken fast as much as a word
// spoken slowly, until the next word lights. A word lit for less than the
// pop had the whole curve squeezed into its time, so it rose almost twice
// as fast, and Tim saw the pop differ from word to word.
func TestEveryWordPopsOnTheSameCurve(t *testing.T) {
	source := SourceInfo{FPSNum: 100, FPSDen: 1, Duration: 60}
	d := &CaptionDrawing{
		Laid: []LaidCaption{
			{Caption: Caption{Start: 0, End: 2, Words: []Cue{{0.5, 0.62, "du"}, {0.62, 1.5, "warst"}}}},
		},
		Clip: Clip{Segments: []Segment{{Start: 10, End: 12}}},
	}
	for k := range 12 {
		fast, slow := d.PopScale(source, 10.5+float64(k)/100, 0), d.PopScale(source, 10.62+float64(k)/100, 0)
		if math.Abs(fast-slow) > 1e-9 {
			t.Errorf("%d ms into their pops a word lit for 120 ms is at %.4f, one lit for 880 ms at %.4f", k*10, fast, slow)
		}
	}
	// The next word lights before the pop is over: the word goes back to
	// its size with the pill.
	if got := d.PopScale(source, 10.62, 0); got != 1 {
		t.Errorf("the next word starts its pop at %.3f, not at rest", got)
	}
}

// Played over a paused frame, the pop comes back to how that frame shows
// it: PopAgain is the first moment of the settle that looks the same, and
// nothing past the peak.
func TestPopAgainComesBackToTheSameLook(t *testing.T) {
	for _, span := range []int{70, 15, 8} {
		length := min(popLength, float64(span*10))
		peak := 0.0
		for ms := 0.0; ms < popLength; ms++ {
			if popDrawn(ms) > popDrawn(peak) {
				peak = ms
			}
		}
		for since := 0.0; since < length; since++ {
			again := PopAgain(since, span)
			if since >= peak {
				if again != 0 {
					t.Errorf("lit for %d hundredths, %.0f ms after it lit, past the peak, PopAgain is %.0f", span, since, again)
				}
				continue
			}
			now := popDrawn(since)
			if since+again == length {
				// Lit for less than the pop: it plays until the next word.
				continue
			}
			if again <= 0 || popDrawn(since+again) > now+1e-9 || popDrawn(since+again-1) <= now {
				t.Errorf("lit for %d hundredths, %.0f ms after it lit at %.4f, PopAgain is %.0f ms, which is at %.4f",
					span, since, now, again, popDrawn(since+again))
			}
		}
	}
}

// PopFrames says which frames of the short a paused video preview plays
// for the word on screen to pop: from the word's first frame, at rest,
// where a step lands, to the last frame before the pop looks again as it
// does on the frame paused on.
// None for a frame past the peak or with no word lit.
func TestPopFramesCountsTheFramesOfThePop(t *testing.T) {
	source := SourceInfo{FPSNum: 30, FPSDen: 1, Duration: 60}
	d := &CaptionDrawing{
		Laid: []LaidCaption{
			{Caption: Caption{Start: 0, End: 2, Words: []Cue{{0.5, 1.21, "Ich"}, {1.21, 2, "war"}}}},
		},
		Clip: Clip{Segments: []Segment{{Start: 10, End: 12}}},
	}
	for _, c := range []struct {
		frame         int
		behind, ahead int
	}{
		// Before the first word: nothing lit.
		{0, 0, 0},
		// "Ich" lights at 0.5 s, frame 15 of the short, the frame at
		// 10.5 s of the episode: its whole pop, seven frames of it.
		{15, 0, 6},
		// A frame in, 33 ms up, where a click may land: from the word's
		// first frame, and back to its look 130 ms on.
		{16, 1, 3},
		// Two frames in.
		{17, 2, 2},
		// Past the peak.
		{20, 0, 0},
		// "war" lights at 1.21 s, between frames 36 and 37: its first
		// frame shows it at rest, its pop starts on the hundredth after,
		// and the whole of it plays, six frames to rest again.
		{37, 0, 6},
	} {
		at := 10 + float64(c.frame)/30
		if behind, ahead := d.PopFrames(source, at); behind != c.behind || ahead != c.ahead {
			t.Errorf("paused on frame %d PopFrames is %d behind and %d ahead, want %d and %d",
				c.frame, behind, ahead, c.behind, c.ahead)
		}
		m := source.CaptionMoment(d.Clip, at)
		for n := -c.behind; n <= c.ahead; n++ {
			want := m + int64(math.Round(float64(n)*1000/30))
			if got := source.captionMomentAhead(d.Clip, at, n); got < want-1 || got > want+1 {
				t.Errorf("paused on frame %d, %d frames ahead is %d ms, not %d frames from %d ms", c.frame, n, got, n, m)
			}
		}
	}
}

// PopScale is the pill's scale on the frame, at rest on a word's first
// frame, whether the word lights on a frame or between two, and grown on
// the frames of its pop.
func TestPopScaleIsThePillOnTheFrame(t *testing.T) {
	source := SourceInfo{FPSNum: 30, FPSDen: 1, Duration: 60}
	d := &CaptionDrawing{
		Laid: []LaidCaption{
			{Caption: Caption{Start: 0, End: 2, Words: []Cue{{0.5, 1.21, "Ich"}, {1.21, 2, "war"}}}},
		},
		Clip: Clip{Segments: []Segment{{Start: 10, End: 12}}},
	}
	at := func(frame int) float64 { return 10 + float64(frame)/30 }
	for _, c := range []struct {
		frame    int
		at, grew bool
	}{{0, true, false}, {15, true, false}, {16, false, true}, {18, false, true}, {30, true, false},
		{37, true, false}, {38, false, true}} {
		s := d.PopScale(source, at(c.frame), 0)
		if c.at && s != 1 || c.grew && s <= 1.01 {
			t.Errorf("on frame %d the pill is drawn at %.3f", c.frame, s)
		}
	}
	// Ahead counts frames of the short.
	if a, b := d.PopScale(source, at(15), 3), d.PopScale(source, at(18), 0); a != b {
		t.Errorf("three frames ahead of frame 15 the pill is at %.3f, on frame 18 at %.3f", a, b)
	}
}
