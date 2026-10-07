package engine

import (
	"math"
	"testing"
)

// An edge put on frames lands on the picture's own frames, counted from
// where the picture starts, not from the start of the file. A picture that
// starts a quarter of a second after its sound, at 25 frames a second, has
// its frames at 0.25, 0.29, 0.33 and so on, and an edge rounded from the
// file's start landed half a frame from them, on a frame the render does
// not cut on. Plan row 2.137.
func TestAnEdgeOnFramesLandsOnThePicturesOwnFrames(t *testing.T) {
	g := Gesture{Frame: 0.04, FrameStart: 0.25}
	for _, c := range []struct{ at, want float64 }{
		{1.0, 1.01}, {0.989, 0.97}, {0.25, 0.25}, {0.269, 0.25}, {0.271, 0.29},
	} {
		if got := g.onFrame(c.at); math.Abs(got-c.want) > 1e-9 {
			t.Errorf("%.3f went to %.3f, the picture's frame there is %.3f", c.at, got, c.want)
		}
	}
	// Counted from the file's start when the picture starts with it.
	if got := (Gesture{Frame: 0.04}).onFrame(1.005); math.Abs(got-1.0) > 1e-9 {
		t.Errorf("1.005 went to %.3f, not 1.000", got)
	}
	if _, _, err := (Gesture{Kind: "trim", Frame: 0.04, FrameStart: -1}).change(Plan{}, Clip{Segments: []Segment{{Start: 1, End: 2}}}, nil, 0); err == nil {
		t.Error("a picture said to start before the file was taken")
	}
}
