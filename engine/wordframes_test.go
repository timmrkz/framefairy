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
