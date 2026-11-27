package engine

import (
	"math"
	"strconv"
	"testing"
)

// Where an edge is kept and which frame the render cuts it on. See
// PieceOnFrames. The video preview counts no frame of its own: it looks a
// moment up where the engine says each frame holds from, see
// framewire.Grid.From.
//
// An edge at a moment at a rate of num/den frames a second, with the
// picture starting at start, as a search or a hand puts it, the moment
// the plan keeps it at, on its frame's start to the millisecond, and that
// frame.
var frameEdges = []struct {
	num, den         int64
	start, at, saved float64
	frame            int64
}{
	{30000, 1001, 0, 1.001, 1.001, 30},
	{30000, 1001, 0, 0.033, 0.033, 1},
	{30000, 1001, 0, 0.133, 0.133, 4},
	{30000, 1001, 0, 0.34, 0.334, 10},
	{30000, 1001, 0, 0.355, 0.367, 11},
	{30000, 1001, 0, 1001, 1001, 30000},
	{30000, 1001, 0.25, 0.483, 0.484, 7},
	{30000, 1001, 0.25, 0.25, 0.25, 0},
	{30000, 1001, 0.00095, 0.051, 0.068, 2},
	{24000, 1001, 0, 0.083, 0.083, 2},
	{24000, 1001, 0, 0.1, 0.083, 2},
	{24000, 1001, 0, 0.11, 0.125, 3},
	{25, 1, 0, 0.04, 0.04, 1},
	{25, 1, 0, 4.03, 4.04, 101},
	{25, 1, 0, 7.019, 7, 175},
	{25, 1, 0, 0.02, 0.04, 1},
	{25, 1, 0, 20.02, 20.04, 501},
	{25, 1, 0.25, 0.989, 0.97, 18},
	{25, 1, 0.1, 0.08, 0.1, 0},
	{25, 1, 0.25, 0.23, 0.25, 0},
	{25, 1, 0.03, 0, 0.03, 0},
	{30, 1, 0, 0.033, 0.033, 1},
	{30, 1, 0, 0.049, 0.033, 1},
	{30, 1, 0, 0.051, 0.067, 2},
	{30, 1, 0, 0.05, 0.067, 2},
	{50, 1, 0.1, 0.09, 0.1, 0},
	{60, 1, 0, 0.033, 0.033, 2},
	{60, 1, 0, 0.017, 0.017, 1},
	{60, 1, 0, 0.025, 0.033, 2},
	{5, 1, 0, 0.2, 0.2, 1},
	{5, 1, 0, 0.29, 0.2, 1},
	{5, 1, 0, 0.1, 0.2, 1},
	{5, 1, 0, 0.3, 0.4, 2},
	{5, 1, 0, 0.31, 0.4, 2},
}

// Every frame start of a stretch of frames, saved to the millisecond the
// way an edge is, below of them a little before their frame.
var frameStretches = []struct {
	num, den int64
	start    float64
	frames   int64
	below    int
}{
	{30000, 1001, 0, 3000, 1400},
	{30000, 1001, 0.25, 3000, 1400},
	{24000, 1001, 0, 3000, 1375},
	{25, 1, 0, 3000, 0},
	{30, 1, 0, 3000, 1000},
	{60, 1, 0, 3000, 1000},
	{5, 1, 0, 3000, 0},
}

func TestEveryEdgeIsKeptOnTheStartOfItsFrame(t *testing.T) {
	for _, e := range frameEdges {
		num, den := e.num, e.den
		s := SourceInfo{FPSNum: int(num), FPSDen: int(den), VideoStart: e.start}
		if got, _ := s.PieceOnFrames(e.at, e.at+1); got != e.saved {
			t.Errorf("at %s fps from %.5f, an edge at %.3f is kept at %.3f, want %.3f", fpsText(num, den), e.start, e.at, got, e.saved)
		}
		// Kept, it stays where it is, and the render cuts it on its frame.
		if got, _ := s.PieceOnFrames(e.saved, e.saved+1); got != e.saved {
			t.Errorf("at %s fps from %.5f, an edge kept at %.3f moved to %.3f", fpsText(num, den), e.start, e.saved, got)
		}
		if got, _ := s.pieceFrames(e.saved, e.saved+1); got != e.frame {
			t.Errorf("at %s fps from %.5f, an edge kept at %.3f is cut on frame %d, want %d", fpsText(num, den), e.start, e.saved, got, e.frame)
		}
	}
	for _, st := range frameStretches {
		num, den := st.num, st.den
		s := SourceInfo{FPSNum: int(num), FPSDen: int(den), VideoStart: st.start}
		startMs := int64(math.Round(st.start * 1000))
		below := 0
		for k := range st.frames {
			// Frame k starts k·den/num seconds after the picture does, and
			// kept to the millisecond, rounded half up, it is this.
			ms := startMs + (2*k*den*1000+num)/(2*num)
			if (ms-startMs)*num < k*den*1000 {
				below++
			}
			at := float64(ms) / 1000
			if got, _ := s.PieceOnFrames(at, at+1); got != at {
				t.Errorf("at %s fps, frame %d kept at %.3f moved to %.3f", fpsText(num, den), k, at, got)
			}
			if got, _ := s.pieceFrames(at, at+1); got != k {
				t.Errorf("at %s fps, frame %d kept at %.3f is cut on frame %d", fpsText(num, den), k, at, got)
			}
			// A moment a hair before it, a pause a millisecond before the
			// frame, is in it, the frame the video preview draws there,
			// looked up in the table of frames the engine sends. Unless the kept start lies before the
			// frame, as it does for the table's Below of them: then it is
			// in the frame before.
			if k > 0 {
				want := k
				if (ms-startMs)*num < k*den*1000 {
					want = k - 1
				}
				if got := s.frameHolding(float64(ms-1) / 1000); got != want {
					t.Errorf("at %s fps, %.3f, a hair before frame %d kept at %.3f, is frame %d, want %d", fpsText(num, den), float64(ms-1)/1000, k, at, got, want)
				}
			}
			// The render cuts a piece starting there on that frame, to the
			// microsecond.
			clip := s.OnFrames(Clip{Segments: []Segment{{Start: at, End: at + 1}}})
			if want := st.start + float64(k)/(float64(num)/float64(den)); math.Abs(clip.Segments[0].Start-want) > 1e-9 {
				t.Errorf("at %s fps, a piece from %.3f starts at %.6f, not on frame %d at %.6f",
					fpsText(num, den), at, clip.Segments[0].Start, k, want)
			}
		}
		if below != st.below {
			t.Errorf("at %s fps, %d of %d frames kept to the millisecond come before their start, the table says %d",
				fpsText(num, den), below, st.frames, st.below)
		}
	}
}

// fpsText is a rate as people write it, 30000/1001 or 25.
func fpsText(num, den int64) string {
	if den == 1 {
		return strconv.FormatInt(num, 10)
	}
	return strconv.FormatInt(num, 10) + "/" + strconv.FormatInt(den, 10)
}
