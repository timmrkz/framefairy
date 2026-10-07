package engine

import (
	"encoding/json"
	"math"
	"os"
	"strconv"
	"strings"
	"testing"
)

// The frame an edge means, as the render cuts on it, against the same
// table the video preview is held to, in frontend/src/lib/frame.cases.json,
// so the two never count a frame apart. See frameOf.
type frameCases struct {
	Edges []struct {
		Rate  string  `json:"rate"`
		Start float64 `json:"start"`
		At    float64 `json:"at"`
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

func TestAnEdgeMeansTheFrameWhoseStartIsNearest(t *testing.T) {
	c := readFrameCases(t)
	for _, e := range c.Edges {
		num, den := caseRate(t, e.Rate)
		if got := frameOf(e.At, e.Start, float64(num)/float64(den)); got != e.Frame {
			t.Errorf("at %s fps from %.3f, an edge at %.3f is on frame %d, want %d", e.Rate, e.Start, e.At, got, e.Frame)
		}
	}
	for _, s := range c.Stretches {
		num, den := caseRate(t, s.Rate)
		rate := float64(num) / float64(den)
		startMs := int64(math.Round(s.Start * 1000))
		below := 0
		for k := range s.Frames {
			// Frame k starts k·den/num seconds after the picture does, and
			// saved to the millisecond, rounded half up, it is this.
			ms := startMs + (2*k*den*1000+num)/(2*num)
			if (ms-startMs)*num < k*den*1000 {
				below++
			}
			at := float64(ms) / 1000
			if got := frameOf(at, s.Start, rate); got != k {
				t.Errorf("at %s fps, frame %d saved as %.3f is on frame %d", s.Rate, k, at, got)
			}
			// The render cuts a piece starting there on that frame too.
			clip := SourceInfo{FPSNum: int(num), FPSDen: int(den), VideoStart: s.Start}.
				OnFrames(Clip{Segments: []Segment{{Start: at, End: at + 1}}})
			if want := s.Start + float64(k)/rate; math.Abs(clip.Segments[0].Start-want) > 1e-9 {
				t.Errorf("at %s fps, a piece from %.3f starts at %.6f, not on frame %d at %.6f",
					s.Rate, at, clip.Segments[0].Start, k, want)
			}
		}
		if below != s.Below {
			t.Errorf("at %s fps, %d of %d frames saved to the millisecond come before their start, the table says %d",
				s.Rate, below, s.Frames, s.Below)
		}
	}
}
