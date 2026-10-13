package engine

import (
	"fmt"
	"math"
	"testing"
)

// The pop of a word, frame by frame, for every number of frames a word
// can be lit for at the rates videos come in: at rest on its first frame,
// popPeak on a frame of its own, up to it and down from it without a step
// back, and at rest again by its last frame, in no more than popLength.
// A word of one frame is popPeak on it, because the frames on either side
// belong to its neighbours. The same number of frames always pops the
// same way.
func TestEveryWordReachesItsPeakOnAFrame(t *testing.T) {
	for _, fps := range []float64{24, 25, 30000.0 / 1001, 30, 50, 60} {
		for lit := 1; lit <= 20; lit++ {
			s := popScales(lit, fps)
			if len(s) > lit {
				t.Errorf("%.2f fps, lit for %d frames: %d frames of pop", fps, lit, len(s))
				continue
			}
			if lit == 1 {
				if len(s) != 1 || s[0] != popPeak {
					t.Errorf("%.2f fps, lit for one frame: %v, not the peak", fps, s)
				}
				continue
			}
			if s[0] != 1 {
				t.Errorf("%.2f fps, lit for %d frames: the first frame is at %.3f, not at rest", fps, lit, s[0])
			}
			peak := 0
			for i := range s {
				if s[i] > s[peak] {
					peak = i
				}
			}
			if s[peak] != popPeak {
				t.Errorf("%.2f fps, lit for %d frames: the pop reaches %.3f, not %.2f, %v", fps, lit, s[peak], popPeak, s)
			}
			for i := 1; i < len(s); i++ {
				if (i <= peak && s[i] <= s[i-1]) || (i > peak && s[i] >= s[i-1]) {
					t.Errorf("%.2f fps, lit for %d frames: frame %d goes from %.3f to %.3f, %v", fps, lit, i, s[i-1], s[i], s)
				}
			}
			if lit >= 3 && s[len(s)-1] != 1 {
				t.Errorf("%.2f fps, lit for %d frames: the pop ends at %.3f, not at rest, %v", fps, lit, s[len(s)-1], s)
			}
			if ms := float64(len(s)-1) * 1000 / fps; ms > popLength+1000/fps/2 {
				t.Errorf("%.2f fps, lit for %d frames: the pop lasts %.0f ms", fps, lit, ms)
			}
		}
	}
	// What Tim's 25 fps video shows for a long word, and for a word of
	// three frames.
	if got := fmt.Sprintf("%.3f", popScales(20, 25)); got != "[1.000 1.075 1.130 1.150 1.130 1.075 1.000]" {
		t.Errorf("a long word at 25 fps pops %s", got)
	}
	if got := fmt.Sprintf("%.3f", popScales(3, 25)); got != "[1.000 1.150 1.000]" {
		t.Errorf("a word of three frames at 25 fps pops %s", got)
	}
}

// PopFrames says which frames of the bounce a paused video preview plays
// after a word step, for the word on screen to bounce: from the word's
// first frame, at rest, where a step lands, to the first frame from the
// peak on no larger than the short draws on the frame paused on, so the
// bounce ends on the frame as the short shows it. None past the bounce or
// with no word lit.
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
		// 10.5 s of the episode: its whole pop, seven frames to rest.
		{15, 0, 7},
		// A frame in: from the word's first frame, and on to the first
		// frame past the peak no larger.
		{16, 1, 6},
		// Two frames in.
		{17, 2, 4},
		// On the peak, and a frame past it: up to the frame.
		{19, 4, 0},
		{20, 5, 0},
		// Past the pop.
		{23, 0, 0},
		// "war" lights at 1.21 s, between frames 36 and 37: its first
		// frame shows it at rest, and the whole pop plays.
		{37, 0, 7},
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

// A word step bounces its word from rest through its peak however briefly
// the short lights it. A word of one frame, which the short shows at its
// peak, rises from rest to the peak, where the frame stands. A word of
// three frames, which the short pops in three, bounces in all seven and
// comes to rest. A word long enough for its pop bounces in the short's own
// frames, with the short's own captions. Tim wanted a step from word to
// word to bounce, a word of one frame too, and a step from frame to frame
// to show the short's frames alone.
func TestAWordStepBouncesEveryWordWhole(t *testing.T) {
	source := SourceInfo{FPSNum: 30, FPSDen: 1, Duration: 60}
	d := &CaptionDrawing{
		Laid: []LaidCaption{
			{Caption: Caption{Start: 0, End: 2, Words: []Cue{{0.5, 0.53, "ja"}, {0.53, 0.6, "so"},
				{0.6, 1.5, "eben"}, {1.5, 2, "nicht"}}}},
			{Caption: Caption{Start: 2, End: 3, Words: []Cue{{2, 3, "Und"}}}},
		},
		Clip: Clip{Segments: []Segment{{Start: 10, End: 13}}},
	}
	at := func(frame int) float64 { return 10 + float64(frame)/30 }
	for _, c := range []struct {
		word   string
		frame  int
		ahead  int
		scales string
	}{
		{"ja", 15, 4, "[1.000 1.057 1.106 1.139 1.150]"},
		{"so", 16, 7, "[1.000 1.057 1.106 1.139 1.150 1.130 1.075 1.000]"},
		{"eben", 18, 7, "[1.000 1.057 1.106 1.139 1.150 1.130 1.075 1.000]"},
	} {
		if s := d.Showing(source, at(c.frame)); s.Word < 0 || d.Laid[s.Caption].Words[s.Word].Text != c.word {
			t.Fatalf("frame %d shows %+v, not %q", c.frame, s, c.word)
		}
		behind, ahead := d.PopFrames(source, at(c.frame))
		if behind != 0 || ahead != c.ahead {
			t.Errorf("%q bounces %d behind and %d ahead, want 0 and %d", c.word, behind, ahead, c.ahead)
		}
		b := d.Bouncing(source, at(c.frame))
		var scales []float64
		for n := 0; n <= ahead; n++ {
			scales = append(scales, b.PopScale(source, at(c.frame), n))
		}
		if got := fmt.Sprintf("%.3f", scales); got != c.scales {
			t.Errorf("%q bounces %s, want %s", c.word, got, c.scales)
		}
		if last, own := scales[len(scales)-1], d.PopScale(source, at(c.frame), 0); math.Abs(last-own) > 1e-9 {
			t.Errorf("%q ends its bounce at %.3f, and the short shows it at %.3f", c.word, last, own)
		}
	}
	if b := d.Bouncing(source, at(18)); b != d {
		t.Errorf("a word lit long enough bounces with captions of its own")
	}
	b := d.Bouncing(source, at(15))
	if len(b.Laid) != 1 || len(d.Laid) != 2 || d.Laid[0].Words[1].Start != 0.53 || d.Laid[0].End != 2 {
		t.Errorf("the bounce of a short word changed the captions it was made from, or kept the others")
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
