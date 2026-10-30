package engine

import (
	"bytes"
	"context"
	"math"
	"strconv"
	"strings"
	"testing"

	"framefairy/engine/runlog"
	"framefairy/engine/timed"
	"framefairy/internal/ffmpegtest"
)

// What libass shows at a moment is read off the times the caption file
// gives: a caption from its start to its end, each to the hundredth, and
// a word from its start until the next one starts. The caption box shows
// the caption and the word by it, so it never runs ahead of the layer
// under it. Tim saw the frame in the caption box move to the next caption
// a few frames before the pill did, on #201.
func TestShowingAtIsWhatTheCaptionFileShows(t *testing.T) {
	laid := []LaidCaption{
		{Caption: Caption{Start: 1.004, End: 2.5, Words: []timed.Cue{{Start: 1.004, End: 1.3, Text: "Ach"}, {Start: 1.3, End: 1.296, Text: "doch."}, {Start: 1.8, End: 2.5, Text: "Ja"}}}},
		{Caption: Caption{Start: 2.5, End: 4, Words: []timed.Cue{{Start: 2.6, End: 3, Text: "das"}, {Start: 3, End: 4, Text: "war"}}}},
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
			{Caption: Caption{Start: 0, End: 1.5, Words: []timed.Cue{{Start: 0, End: 0.3, Text: "Ich"}, {Start: 0.33, End: 0.6, Text: "war"}, {Start: 0.61, End: 1.0, Text: "vielleicht"}, {Start: 1.03, End: 1.5, Text: "sechs"}}}},
			{Caption: Caption{Start: 1.5, End: 3.1, Words: []timed.Cue{{Start: 1.5, End: 1.9, Text: "Jahre"}, {Start: 1.9, End: 2.35, Text: "alt,"}, {Start: 2.41, End: 2.43, Text: "als"}, {Start: 2.43, End: 3.1, Text: "ich"}}}},
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

// The box round a caption leaves as much room over the pills of its first
// line as under the pills of its last, and is as tall for a caption with
// no tall or deep letter as for one with them. Tim saw the pill of "Und"
// touch the bottom of its box with room over it on #201: the box hugged
// the letters and the pill reached down for a tail the word did not have.
//
// On the real disk: ffmpeg's libass measures the words in a folder of
// its own.
func TestTheBoxStandsEvenlyRoundItsPills(t *testing.T) {
	t.Parallel()
	ctx := context.Background()
	e := NewEngine(runlog.New(&bytes.Buffer{}, false, false), disk)
	if err := e.Preflight(ctx); err != nil {
		ffmpegtest.Unusable(t, "no usable ffmpeg here: %v", err)
	}
	if _, err := e.SubtitleFilter(ctx); err != nil {
		ffmpegtest.Unusable(t, "this ffmpeg cannot burn captions in: %v", err)
	}
	caption := func(start float64, texts ...string) LaidCaption {
		var words []timed.Cue
		for i, text := range texts {
			words = append(words, timed.Cue{Start: start + float64(i)*0.5, End: start + float64(i+1)*0.5, Text: text})
		}
		return LaidCaption{Caption: Caption{Start: start, End: start + float64(len(texts))*0.5, Words: words},
			Lines: [][]timed.Cue{words}}
	}
	laid := []LaidCaption{caption(0, "Und"), caption(2, "jung"), caption(4, "so", "einem")}
	track, err := e.CaptionTrack(ctx, laid, shortW, shortH, nil, SourceInfo{})
	if err != nil {
		t.Fatal(err)
	}
	frames := e.WordFrames(ctx, laid, shortW, shortH, nil, "")
	if len(frames) != len(laid) {
		t.Fatalf("word frames for %d captions, not %d", len(frames), len(laid))
	}
	// The box of each caption, top and bottom, from its drawing.
	var boxes [][2]float64
	for _, line := range strings.Split(string(track), "\n") {
		if !strings.HasPrefix(line, "Dialogue: 0,") || !strings.Contains(line, ",Box,") {
			continue
		}
		end := strings.Index(line, `{\p0}`)
		shape := line[strings.LastIndex(line[:end], "}")+1 : end]
		fields := strings.Fields(shape)
		top, bottom := math.Inf(1), math.Inf(-1)
		for i := 0; i < len(fields); i++ {
			if _, err := strconv.ParseFloat(fields[i], 64); err != nil {
				continue
			}
			y, _ := strconv.ParseFloat(fields[i+1], 64)
			top, bottom = math.Min(top, y), math.Max(bottom, y)
			i++
		}
		boxes = append(boxes, [2]float64{top, bottom})
	}
	if len(boxes) != len(laid) {
		t.Fatalf("%d boxes drawn for %d captions", len(boxes), len(laid))
	}
	for i, c := range laid {
		top, bottom := math.Inf(1), math.Inf(-1)
		for _, f := range frames[i] {
			top, bottom = math.Min(top, f.Y*shortH), math.Max(bottom, (f.Y+f.H)*shortH)
		}
		over, under := top-boxes[i][0], boxes[i][1]-bottom
		t.Logf("%q: the box leaves %.0f pixels over its pills and %.0f under them", c.Words[0].Text, over, under)
		if math.Abs(over-under) > 1 {
			t.Errorf("%q: the box leaves %.0f pixels over its pills and %.0f under them", c.Words[0].Text, over, under)
		}
		if h, first := boxes[i][1]-boxes[i][0], boxes[0][1]-boxes[0][0]; math.Abs(h-first) > 1 {
			t.Errorf("%q: the box is %.0f pixels high, and %.0f round %q", c.Words[0].Text, h, first, laid[0].Words[0].Text)
		}
	}
}
