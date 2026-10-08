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
	g := Gesture{Frames: SourceInfo{FPSNum: 25, FPSDen: 1, VideoStart: 0.25}}
	for _, c := range []struct{ at, want float64 }{
		{1.0, 1.01}, {0.989, 0.97}, {0.25, 0.25}, {0.269, 0.25}, {0.271, 0.29},
	} {
		if got := g.onFrame(c.at); math.Abs(got-c.want) > 1e-9 {
			t.Errorf("%.3f went to %.3f, the picture's frame there is %.3f", c.at, got, c.want)
		}
	}
	// Counted from the file's start when the picture starts with it.
	if got := (Gesture{Frames: at25}).onFrame(1.005); math.Abs(got-1.0) > 1e-9 {
		t.Errorf("1.005 went to %.3f, not 1.000", got)
	}
}

// A cut's edge moved up to its other edge stops a frame short of it on
// frames. Held only the least a cut may be away, 50 ms, both edges went
// onto the same frame at five frames a second, and the cut closed into
// two pieces that meet, which is a camera switch. The timeline walk found
// it.
func TestACutMovedOnFramesStaysOpen(t *testing.T) {
	path := editablePlanPath(t)
	tr := editableTranscript()
	for _, g := range []Gesture{
		{Kind: "move", Edge: "to", From: 11.1, To: 11.12, Frames: at5},
		{Kind: "move", Edge: "from", From: 11.88, To: 11.9, Frames: at5},
	} {
		shaped, err := ShapeClip(path, "01", g, tr, 0.1)
		if err != nil {
			t.Fatal(err)
		}
		if len(shaped.Pieces) != 2 || shaped.Pieces[1].Start <= shaped.Pieces[0].End {
			t.Errorf("the %s edge moved to the other leaves %+v", g.Edge, shaped.Pieces)
		}
	}
}
