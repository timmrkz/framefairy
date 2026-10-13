package engine

import (
	"math"
	"strconv"
	"strings"
	"testing"
)

// What libass shows at a moment is read off the times the caption file
// gives: a caption from its start to its end, each to the hundredth, and
// a word from its start until the next one starts. The caption box shows
// the caption and the word by it, so it never runs ahead of the layer
// under it. Tim saw the frame in the caption box move to the next caption
// a few frames before the pill did, on #201.
func TestShowingAtIsWhatTheCaptionFileShows(t *testing.T) {
	laid := []LaidCaption{
		{Caption: Caption{Start: 1.004, End: 2.5, Words: []Cue{{1.004, 1.3, "Ach"}, {1.3, 1.296, "doch."}, {1.8, 2.5, "Ja"}}}},
		{Caption: Caption{Start: 2.5, End: 4, Words: []Cue{{2.6, 3, "das"}, {3, 4, "war"}}}},
	}
	for _, c := range []struct {
		ms           int64
		caption, wrd int
	}{
		{0, -1, -1},
		{999, -1, -1},
		// 1.004 s is 100 hundredths, which libass shows from 1000 ms.
		{1000, 0, 0},
		{1299, 0, 0},
		{1300, 0, 1},
		{1799, 0, 1},
		{1800, 0, 2},
		{2499, 0, 2},
		// The next caption from its start, before its first word.
		{2500, 1, -1},
		{2599, 1, -1},
		{2600, 1, 0},
		{3999, 1, 1},
		{4000, -1, -1},
	} {
		got := ShowingAt(laid, c.ms)
		if got.Caption != c.caption || got.Word != c.wrd {
			t.Errorf("at %d ms libass shows caption %d word %d, ShowingAt says %d and %d",
				c.ms, c.caption, c.wrd, got.Caption, got.Word)
		}
	}
}

// The corners of a pill are quarters of a circle of its radius, the way
// the browser rounds the frame the caption box puts round the word, so
// the frame and the pill have the same corners. Tim saw them differ on
// #201: the handles of each corner both stood on the corner itself, which
// drew it much sharper than the radius.
func TestPillCornersAreQuartersOfACircle(t *testing.T) {
	const x0, y0, x1, y1, r = 0, 0, 300, 120, 40
	fields := strings.Fields(roundedRect(x0, y0, x1, y1, r))
	num := func(s string) float64 {
		v, err := strconv.ParseFloat(s, 64)
		if err != nil {
			t.Fatalf("%q in the drawing is no number", s)
		}
		return v
	}
	// The middle of each corner's curve, against the middle of the arc
	// round the corner's centre, which stands r in from both sides.
	var px, py float64
	corners := 0
	for i := 0; i < len(fields); {
		switch fields[i] {
		case "m", "l":
			px, py = num(fields[i+1]), num(fields[i+2])
			i += 3
		case "b":
			p := [8]float64{px, py}
			for k := 0; k < 6; k++ {
				p[2+k] = num(fields[i+1+k])
			}
			mx := (p[0] + 3*p[2] + 3*p[4] + p[6]) / 8
			my := (p[1] + 3*p[3] + 3*p[5] + p[7]) / 8
			cx := math.Max(x0+r, math.Min(x1-r, mx))
			cy := math.Max(y0+r, math.Min(y1-r, my))
			if d := math.Hypot(mx-cx, my-cy); math.Abs(d-r) > 0.5 {
				t.Errorf("corner %d runs %.2f from its centre in its middle, a circle of the radius %d", corners, d, r)
			}
			corners++
			px, py = p[6], p[7]
			i += 7
		default:
			t.Fatalf("%q in the drawing is no command", fields[i])
		}
	}
	if corners != 4 {
		t.Errorf("the pill has %d rounded corners, not 4", corners)
	}
}

// When the picture lights a word, by WordsLit, is when the layer of each
// frame says it does, frame for frame, on a clip whose pieces start off
// the frames' grid. And it is not when the word's own time says: a frame
// into a word by its time lands, for some word here, on a frame that
// still lights the word before, which is where Shift and an arrow put the
// keyboard's frame a frame ahead of the pill on #201.
func TestWordsLitIsWhatEachFrameShows(t *testing.T) {
	source := SourceInfo{FPSNum: 25, FPSDen: 1, Duration: 60}
	d := &CaptionDrawing{
		Laid: []LaidCaption{
			{Caption: Caption{Start: 0, End: 1.5, Words: []Cue{{0, 0.3, "Ich"}, {0.33, 0.6, "war"}, {0.61, 1.0, "vielleicht"}, {1.03, 1.5, "sechs"}}}},
			{Caption: Caption{Start: 1.5, End: 3.1, Words: []Cue{{1.5, 1.9, "Jahre"}, {1.9, 2.35, "alt,"}, {2.41, 2.43, "als"}, {2.43, 3.1, "ich"}}}},
		},
		Clip: Clip{Segments: []Segment{{Start: 10.023, End: 11.513}, {Start: 20.258, End: 21.9}}},
	}
	lit := d.WordsLit(source)
	if len(lit) != 2 {
		t.Fatalf("WordsLit gives %d captions, not 2", len(lit))
	}
	ahead := 0
	for _, seg := range d.Clip.Segments {
		first, end := source.pieceFrames(seg.Start, seg.End)
		for n := first; n < end; n++ {
			at := source.keptAt(n)
			on := d.Showing(source, at)
			want := CaptionShowing{-1, -1}
			for i := range lit {
				for k, span := range lit[i] {
					if at >= span[0] && at < span[1] {
						want = CaptionShowing{i, k}
					}
				}
			}
			if on.Word >= 0 && on != want {
				t.Errorf("the frame at %.3f s shows caption %d word %d, WordsLit has caption %d word %d there",
					at, on.Caption, on.Word, want.Caption, want.Word)
			}
		}
	}
	// A frame into each word by its own time, on the clip's clock.
	frame := 1 / 25.0
	for i, c := range d.Laid {
		for k, w := range c.Words {
			clip := min(w.Start+frame, (w.Start+w.End)/2)
			if on := d.Showing(source, source.frameStartHolding(EpisodeTime(d.Clip, clip))); on.Caption == i && on.Word != k {
				ahead++
			}
		}
	}
	if ahead == 0 {
		t.Errorf("a frame into every word by its time lights that word, so this clip does not show what WordsLit is for")
	}
	// The word too short to fall on a frame of its own is lit on none.
	if got := lit[1][2]; got != [2]float64{-1, -1} {
		t.Errorf(`"als", lit for 20 ms between two frames, is lit from %.3f to %.3f s, on no frame was wanted`, got[0], got[1])
	}
}
