package engine

import (
	"math"
	"testing"

	"framefairy/engine/timed"
)

// A clip in two pieces with a two second cut between them. Its own clock
// runs 0 to 8, the episode's 10 to 20.
var twoPieces = Clip{Segments: []Segment{{Start: 10, End: 14}, {Start: 16, End: 20}}}

func TestTheClipsClockWalksItsPiecesAddingTheCutsBack(t *testing.T) {
	for _, c := range []struct{ at, start, end float64 }{
		{0, 10, 10},
		{3, 13, 13},
		// A caption that goes where the cut begins goes at 14, not at 16:
		// drawn to 16 it lay over the whole cut. What starts there starts
		// after the cut.
		{4, 16, 14},
		{4 + 1e-9, 16 + 1e-9, 14},
		{5, 17, 17},
		{8, 20, 20},
		{99, 20, 20},
	} {
		if got := EpisodeTime(twoPieces, c.at); math.Abs(got-c.start) > 1e-9 {
			t.Errorf("a start at %v on the clip is at %v in the episode, want %v", c.at, got, c.start)
		}
		if got := EpisodeEnd(twoPieces, c.at); math.Abs(got-c.end) > 1e-9 {
			t.Errorf("an end at %v on the clip is at %v in the episode, want %v", c.at, got, c.end)
		}
	}
	if got := EpisodeEnd(Clip{}, 7); got != 7 {
		t.Errorf("with no pieces an end is %v, want the moment back", got)
	}
}

func TestAWordIsTheSameWordWhereverACutBeforeItIs(t *testing.T) {
	// A word said at 24.5 is 1.5 into a clip of 20 to 21 and 24 to 26.
	// The cut's edges moved put it elsewhere on the clip's clock and never
	// elsewhere in the episode, so it is one word to the caption block and
	// to the pill, and neither pops for it.
	seen := map[float64]bool{}
	for _, segs := range [][]Segment{
		{{Start: 20, End: 21}, {Start: 24, End: 26}},
		{{Start: 20, End: 20.64}, {Start: 24, End: 26}},
		{{Start: 20, End: 21}, {Start: 24.28, End: 26}},
		{{Start: 20, End: 20.333}, {Start: 23.917, End: 26}},
	} {
		clip := Clip{Segments: segs}
		on := ClipTime(clip, 24.5)
		seen[on] = true
		if got := wordAt(clip, on); got != 24.5 {
			t.Errorf("pieces %v: the word at %v on the clip is %v, want 24.5", segs, on, got)
		}
	}
	if len(seen) != 4 {
		t.Errorf("the word stood on %d places of the clip's clock, want 4", len(seen))
	}
	// A word a cut takes the start of stands where the cut ends, which on
	// the clip's clock is also where the piece before it ends. It is the
	// piece after's.
	clip := Clip{Segments: []Segment{{Start: 20.1, End: 20.7}, {Start: 24.3, End: 26}}}
	if got := wordAt(clip, 0.6); got != 24.3 {
		t.Errorf("a word where a cut ends is at %v, want 24.3, the piece after's", got)
	}
}

func TestACaptionsPlayheadStandsOnTheFramesItIsUpOn(t *testing.T) {
	// 25 frames a second, frames every 0.04 s from the start of the file.
	s := SourceInfo{FPSNum: 25, FPSDen: 1}
	clip := Clip{Segments: []Segment{{Start: 10, End: 14}, {Start: 16, End: 20}}}
	word := func(start, end float64) LaidCaption {
		return LaidCaption{Start: start, End: end, Lines: [][]timed.Cue{{{Start: start, End: end}}}}
	}
	for _, c := range []struct {
		name       string
		caption    LaidCaption
		start, end float64
	}{
		// It begins on its word: a frame into it, so the word is lit.
		{"on its word", word(1, 2), 11.04, 11.96},
		// It begins between frames before its word: the first frame
		// after the hundredth its start is put on.
		{"between frames", LaidCaption{Start: 1.01, End: 2}, 11.04, 11.96},
		// A word too short for two frames is entered half way, which for
		// one of a frame is in the frame it starts on.
		{"a short word", word(1, 1.04), 11, 11},
		// It goes at the cut: its last frame is the piece before's last.
		{"to a cut", LaidCaption{Start: 3, End: 4}, 13, 13.96},
		// It begins after the cut, on the piece after's frames.
		{"after a cut", LaidCaption{Start: 4, End: 5}, 16, 16.96},
	} {
		if got := s.captionUp(clip, c.caption); math.Abs(got[0]-c.start) > 1e-9 || math.Abs(got[1]-c.end) > 1e-9 {
			t.Errorf("%s: the playhead stands at %v for the start and %v for the end, want %v and %v", c.name, got[0], got[1], c.start, c.end)
		}
	}
}

func TestAPiecesEdgesPutThePlayheadOnItsFirstAndLastFrames(t *testing.T) {
	s := SourceInfo{FPSNum: 25, FPSDen: 1}
	// A piece from 10 to 14 opens on the frame at 10 and closes after the
	// frame at 13.96, its last.
	if got := s.EdgePlayheads(10, 14); math.Abs(got[0]-10) > 1e-9 || math.Abs(got[1]-13.96) > 1e-9 {
		t.Errorf("the edges of 10 to 14 put the playhead at %v, want 10 and 13.96", got)
	}
	// Edges between frames go on frames first.
	if got := s.EdgePlayheads(10.01, 14.03); math.Abs(got[0]-10) > 1e-9 || math.Abs(got[1]-14) > 1e-9 {
		t.Errorf("the edges of 10.01 to 14.03 put the playhead at %v, want 10 and 14", got)
	}
	// With no rate known, a frame is a thirtieth of a second.
	if got := (SourceInfo{}).EdgePlayheads(1, 2); got[0] != 1 || math.Abs(got[1]-(2-1.0/30)) > 1e-9 {
		t.Errorf("with no rate the edges of 1 to 2 put the playhead at %v", got)
	}
}
