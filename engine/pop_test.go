package engine

import (
	"bytes"
	"context"
	"fmt"
	"strings"
	"testing"

	"framefairy/engine/timed"

	"framefairy/internal/ffmpegtest"
)

// The pop of a word, frame by frame, for every number of frames a word
// can be lit for at the rates videos come in: at rest on its first frame,
// popPeak on a frame of its own, up to it and down from it without a step
// back, and at rest again by its last frame, in no more than popLength.
// A word of one frame is popPeak on it, because the frames on either side
// belong to its neighbours. The same number of frames always pops the
// same way.
func TestEveryWordReachesItsPeakOnAFrame(t *testing.T) {
	for _, fps := range []float64{24, 25, 30000.0 / 1001, 30, 50, 60, 120} {
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

// PopScale is the pill's scale on the frame, at rest on a word's first
// frame, whether the word lights on a frame or between two, and grown on
// the frames of its pop.
func TestPopScaleIsThePillOnTheFrame(t *testing.T) {
	source := SourceInfo{FPSNum: 30, FPSDen: 1, Duration: 60}
	d := &CaptionDrawing{
		Laid: []LaidCaption{
			{Caption: Caption{Start: 0, End: 2, Words: []timed.Cue{{Start: 0.5, End: 1.21, Text: "Ich"}, {Start: 1.21, End: 2, Text: "war"}}}},
		},
		Clip: Clip{Segments: []Segment{{Start: 10, End: 12}}},
	}
	at := func(frame int) float64 { return 10 + float64(frame)/30 }
	for _, c := range []struct {
		frame    int
		at, grew bool
	}{{0, true, false}, {15, true, false}, {16, false, true}, {18, false, true}, {30, true, false},
		{37, true, false}, {38, false, true}} {
		s := d.PopScale(source, at(c.frame))
		if c.at && s != 1 || c.grew && s <= 1.01 {
			t.Errorf("on frame %d the pill is drawn at %.3f", c.frame, s)
		}
	}
}

// The pill of the word being spoken and the word on it lie over the rest
// of the line, so at the peak of the pop the words beside it never reach
// into the pill. Tim saw the letters of the next word drawn over it, and
// wanted the pop to stand out as a layer of its own.
//
// On the real disk: ffmpeg measures the captions.
func TestThePillAndItsWordLieOverTheLine(t *testing.T) {
	ffmpegtest.Need(t)
	e := NewEngine(NewLog(&bytes.Buffer{}, false, false), disk)
	clip := Clip{ID: "01", Slug: "over", Segments: []Segment{{Start: 0, End: 2}}}
	tr := &Transcript{Words: []timed.Cue{{Start: 0.2, End: 0.6, Text: "Das"}, {Start: 0.6, End: 1.2, Text: "respektvoll"}, {Start: 1.2, End: 1.5, Text: "ist."}}, Language: "de"}
	d := captionsView(Plan{}, clip, tr, nil).Drawing()
	track, err := e.CaptionTrack(context.Background(), d.Laid, 540, 960, d.Style, SourceInfo{})
	if err != nil {
		t.Fatal(err)
	}
	// The highest layer of the line, the lowest of a pill and of the word
	// on it.
	line, pill, word := -1, 1<<30, 1<<30
	for _, l := range strings.Split(string(track), "\n") {
		var layer int
		if _, err := fmt.Sscanf(l, "Dialogue: %d,", &layer); err != nil {
			continue
		}
		switch {
		case strings.Contains(l, ",Box,") && strings.Contains(l, `\fscx`):
			pill = min(pill, layer)
		case strings.Contains(l, ",Caption,") && strings.Contains(l, `\fscx`):
			word = min(word, layer)
		case strings.Contains(l, ",Caption,"):
			line = max(line, layer)
		}
	}
	if line < 0 || pill == 1<<30 || word == 1<<30 {
		t.Fatalf("the caption file has a line on %d, a pill on %d and a word on %d", line, pill, word)
	}
	if !(line < pill && pill < word) {
		t.Errorf("the line lies on layer %d, the pill on %d and its word on %d: the pill must lie over the line and the word over the pill", line, pill, word)
	}
}
