package engine

import (
	"encoding/json"
	"math"
	"os"
	"strconv"
	"strings"
	"testing"
)

// Where an edge is kept and which frame the render cuts it on, against
// the same table the video preview is held to, in
// frontend/src/lib/frame.cases.json, so the two never count a frame
// apart. See PieceOnFrames.
type frameCases struct {
	// An edge at a moment, as a search or a hand puts it, the moment the
	// plan keeps it at, on its frame's start to the millisecond, and that
	// frame.
	Edges []struct {
		Rate  string  `json:"rate"`
		Start float64 `json:"start"`
		At    float64 `json:"at"`
		Saved float64 `json:"saved"`
		Frame int64   `json:"frame"`
	} `json:"edges"`
	// Every frame start of a stretch of frames, saved to the millisecond
	// the way an edge is, Below of them a little before their frame.
	Stretches []struct {
		Rate   string  `json:"rate"`
		Start  float64 `json:"start"`
		Frames int64   `json:"frames"`
		Below  int     `json:"below"`
	} `json:"stretches"`
}

func readFrameCases(t *testing.T) frameCases {
	t.Helper()
	data, err := os.ReadFile("../frontend/src/lib/frame.cases.json")
	if err != nil {
		t.Fatal(err)
	}
	var c frameCases
	if err := json.Unmarshal(data, &c); err != nil {
		t.Fatal(err)
	}
	if len(c.Edges) == 0 || len(c.Stretches) == 0 {
		t.Fatal("no cases")
	}
	return c
}

// caseRate is a rate of the table as two whole numbers, 30000 and 1001
// for "30000/1001".
func caseRate(t *testing.T, rate string) (int64, int64) {
	t.Helper()
	n, d, ok := strings.Cut(rate, "/")
	if !ok {
		d = "1"
	}
	num, err1 := strconv.ParseInt(n, 10, 64)
	den, err2 := strconv.ParseInt(d, 10, 64)
	if err1 != nil || err2 != nil || num <= 0 || den <= 0 {
		t.Fatalf("%q is no rate", rate)
	}
	return num, den
}

func TestEveryEdgeIsKeptOnTheStartOfItsFrame(t *testing.T) {
	c := readFrameCases(t)
	for _, e := range c.Edges {
		num, den := caseRate(t, e.Rate)
		s := SourceInfo{FPSNum: int(num), FPSDen: int(den), VideoStart: e.Start}
		if got, _ := s.PieceOnFrames(e.At, e.At+1); got != e.Saved {
			t.Errorf("at %s fps from %.5f, an edge at %.3f is kept at %.3f, want %.3f", e.Rate, e.Start, e.At, got, e.Saved)
		}
		// Kept, it stays where it is, and the render cuts it on its frame.
		if got, _ := s.PieceOnFrames(e.Saved, e.Saved+1); got != e.Saved {
			t.Errorf("at %s fps from %.5f, an edge kept at %.3f moved to %.3f", e.Rate, e.Start, e.Saved, got)
		}
		if got, _ := s.pieceFrames(e.Saved, e.Saved+1); got != e.Frame {
			t.Errorf("at %s fps from %.5f, an edge kept at %.3f is cut on frame %d, want %d", e.Rate, e.Start, e.Saved, got, e.Frame)
		}
	}
	for _, st := range c.Stretches {
		num, den := caseRate(t, st.Rate)
		s := SourceInfo{FPSNum: int(num), FPSDen: int(den), VideoStart: st.Start}
		startMs := int64(math.Round(st.Start * 1000))
		below := 0
		for k := range st.Frames {
			// Frame k starts k·den/num seconds after the picture does, and
			// kept to the millisecond, rounded half up, it is this.
			ms := startMs + (2*k*den*1000+num)/(2*num)
			if (ms-startMs)*num < k*den*1000 {
				below++
			}
			at := float64(ms) / 1000
			if got, _ := s.PieceOnFrames(at, at+1); got != at {
				t.Errorf("at %s fps, frame %d kept at %.3f moved to %.3f", st.Rate, k, at, got)
			}
			if got, _ := s.pieceFrames(at, at+1); got != k {
				t.Errorf("at %s fps, frame %d kept at %.3f is cut on frame %d", st.Rate, k, at, got)
			}
			// The render cuts a piece starting there on that frame, to the
			// microsecond.
			clip := s.OnFrames(Clip{Segments: []Segment{{Start: at, End: at + 1}}})
			if want := st.Start + float64(k)/(float64(num)/float64(den)); math.Abs(clip.Segments[0].Start-want) > 1e-9 {
				t.Errorf("at %s fps, a piece from %.3f starts at %.6f, not on frame %d at %.6f",
					st.Rate, at, clip.Segments[0].Start, k, want)
			}
		}
		if below != st.Below {
			t.Errorf("at %s fps, %d of %d frames kept to the millisecond come before their start, the table says %d",
				st.Rate, below, st.Frames, st.Below)
		}
	}
}
