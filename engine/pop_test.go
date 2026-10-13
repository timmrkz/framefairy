package engine

import (
	"math"
	"testing"
)

// The pop as drawn: from rest to popPeak and back, eased out on the way up
// and on the way down, in straight pieces that meet the curve at every
// knot. A word lit for less time than a pop pops in the time it has.
func TestThePopRisesToItsPeakAndSettles(t *testing.T) {
	for _, span := range []int{70, 22, 12} {
		length, rise := popTimes(span)
		if got := popDrawn(0, span); got != 1 {
			t.Errorf("lit for %d hundredths, the word starts at %.3f, not at rest", span, got)
		}
		if got := popDrawn(length, span); got != 1 {
			t.Errorf("lit for %d hundredths, the word ends its pop at %.3f, not at rest", span, got)
		}
		top, at := 0.0, 0.0
		last := 1.0
		for ms := 0.0; ms <= length; ms++ {
			s := popDrawn(ms, span)
			if s < 1 || s > popPeak+1e-9 {
				t.Errorf("lit for %d hundredths, %.0f ms after it lit the word is at %.3f", span, ms, s)
			}
			// Up until the peak and down after it, never back.
			if (ms <= rise-10 && s < last-1e-9) || (ms >= rise+10 && s > last+1e-9) {
				t.Errorf("lit for %d hundredths, %.0f ms after it lit the word goes from %.4f to %.4f", span, ms, last, s)
			}
			if s > top {
				top, at = s, ms
			}
			last = s
		}
		if top < popPeak-0.01 || math.Abs(at-rise) > 20 {
			t.Errorf("lit for %d hundredths, the pop peaks at %.3f %.0f ms after it lit, not at %.2f near %.0f ms",
				span, top, at, popPeak, rise)
		}
		// Eased out: further than a third of the way up a third into the
		// rise, where a straight line would be.
		if popDrawn(rise/3, span) < 1+(popPeak-1)*0.42 && rise >= 60 {
			t.Errorf("lit for %d hundredths, a third into the rise the word is at %.3f, not eased out", span, popDrawn(rise/3, span))
		}
	}
}

// Played over a paused frame, the pop comes back to how that frame shows
// it: PopAgain is the first moment of the settle that looks the same, and
// nothing past the peak.
func TestPopAgainComesBackToTheSameLook(t *testing.T) {
	for _, span := range []int{70, 15} {
		length, _ := popTimes(span)
		peak := 0.0
		for ms := 0.0; ms < length; ms++ {
			if popDrawn(ms, span) > popDrawn(peak, span) {
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
			now := popDrawn(since, span)
			if again <= 0 || popDrawn(since+again, span) > now+1e-9 || popDrawn(since+again-1, span) <= now {
				t.Errorf("lit for %d hundredths, %.0f ms after it lit at %.4f, PopAgain is %.0f ms, which is at %.4f",
					span, since, now, again, popDrawn(since+again, span))
			}
		}
	}
}

// PopFrames counts the frames of the short a paused video preview plays
// for the word on screen to pop: every frame before the pop looks again as
// it does on the frame paused on, and none for a frame past the peak or
// with no word lit.
func TestPopFramesCountsTheFramesOfThePop(t *testing.T) {
	source := SourceInfo{FPSNum: 30, FPSDen: 1, Duration: 60}
	d := &CaptionDrawing{
		Laid: []LaidCaption{
			{Caption: Caption{Start: 0, End: 2, Words: []Cue{{0.5, 1.2, "Ich"}, {1.2, 2, "war"}}}},
		},
		Clip: Clip{Segments: []Segment{{Start: 10, End: 12}}},
	}
	for _, c := range []struct {
		frame int
		want  int
	}{
		// Before the first word: nothing lit.
		{0, 0},
		// "Ich" lights at 0.5 s, frame 15 of the short, the frame at
		// 10.5 s of the episode: its whole pop, seven frames of it.
		{15, 6},
		// A frame into the rise, 33 ms, comes back to its look 130 ms on.
		{16, 3},
		// Past the peak.
		{20, 0},
	} {
		at := 10 + float64(c.frame)/30
		if got := d.PopFrames(source, at); got != c.want {
			t.Errorf("paused on frame %d PopFrames is %d, want %d", c.frame, got, c.want)
		}
		m := source.CaptionMoment(d.Clip, at)
		for n := 1; n <= c.want; n++ {
			if ahead := source.captionMomentAhead(d.Clip, at, n); ahead != m+int64(math.Round(float64(n)*1000/30)) &&
				ahead != m+int64(math.Round(float64(n)*1000/30))+1 && ahead != m+int64(math.Round(float64(n)*1000/30))-1 {
				t.Errorf("paused on frame %d, %d frames ahead is %d ms, not %d frames after %d ms", c.frame, n, ahead, n, m)
			}
		}
	}
}
