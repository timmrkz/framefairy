package engine

import (
	"bytes"
	"context"
	"fmt"
	"math"
	"os/exec"
	"path/filepath"
	"strings"
	"testing"

	"framefairy/internal/ffmpegtest"
)

// The pop of the word being spoken, read back from a finished short: the
// word and its pill grow together, by one scale around one centre, in
// every direction, as far as popDrawn says on every frame, and at rest the
// pill leaves room round its word. Tim saw #201's bounce move the pill
// only up or down and the word hardly at all.

const bounceFPS = 50

// roomAround is the least room the pill leaves around its word on any
// frame, as a share of the caption size.
const roomAround = 0.10

// bounceBand is the rows of a 1080x1920 short the caption line is read
// from: the line sits 300 pixels over the bottom.
const bounceTop, bounceRows = 1400, 400

// shortRows renders words as the captions of a short from 0 to end of a
// black episode at bounceFPS, 1080x1920, and reads back rows of it from
// top, one rgb24 frame after another.
//
// On the real disk: ffmpeg renders the short and the test reads it back.
func shortRows(t *testing.T, words []Cue, end float64, top, rows int) (e *Engine, info SourceInfo, laid []LaidCaption, frames [][]byte) {
	t.Helper()
	ctx := context.Background()
	e = NewEngine(NewLog(&bytes.Buffer{}, false, false), disk)
	if err := e.Preflight(ctx); err != nil {
		ffmpegtest.Unusable(t, "no usable ffmpeg here: %v", err)
	}
	if _, err := e.SubtitleFilter(ctx); err != nil {
		ffmpegtest.Unusable(t, "this ffmpeg cannot burn captions in: %v", err)
	}
	source := filepath.Join(t.TempDir(), "episode.mp4")
	long := math.Ceil(end) + 1
	if out, err := exec.Command("ffmpeg", "-loglevel", "error", "-y",
		"-f", "lavfi", "-i", fmt.Sprintf("color=c=black:s=640x360:r=%d:d=%g", bounceFPS, long),
		"-f", "lavfi", "-i", fmt.Sprintf("anullsrc=r=48000:cl=stereo:d=%g", long),
		"-shortest", "-c:v", "mpeg4", "-q:v", "2", "-c:a", "aac", source).CombinedOutput(); err != nil {
		t.Fatalf("making the test episode: %s %s", err, out)
	}
	info, err := e.Probe(ctx, source)
	if err != nil {
		t.Fatal(err)
	}
	clip := info.OnFrames(Clip{ID: "01", Slug: "bounce", Segments: []Segment{{Start: 0, End: end}}})
	laid = ClipCaptions(clip, &Transcript{Words: words, Language: "de"}, ResolveStyle(nil))

	rs := RenderSettings{OutW: shortW, OutH: shortH, CRF: 12, Preset: "ultrafast", AudioBitrate: "128k", ScaleUp: true}
	dir := t.TempDir()
	short, err := e.RenderClip(ctx, clip, source, info, filepath.Join(dir, "out"), laid, rs, nil,
		filepath.Join(dir, "captions"), false)
	if err != nil {
		t.Fatal(err)
	}
	out, err := exec.Command("ffmpeg", "-v", "error", "-i", short, "-vf",
		fmt.Sprintf("crop=%d:%d:0:%d", shortW, rows, top),
		"-f", "rawvideo", "-pix_fmt", "rgb24", "-").Output()
	if err != nil {
		t.Fatal(err)
	}
	size := shortW * rows * 3
	for i := 0; i+size <= len(out); i += size {
		frames = append(frames, out[i:i+size])
	}
	return e, info, laid, frames
}

const shortW, shortH = 1080, 1920

// pillColour is the colour of the pill behind the word being spoken.
func pillColour(t *testing.T, s Style) [3]int {
	t.Helper()
	var pill [3]int
	var alpha float64
	if _, err := fmt.Sscanf(s.HighlightWeb(), "rgba(%d, %d, %d, %g)", &pill[0], &pill[1], &pill[2], &alpha); err != nil {
		t.Fatal(err)
	}
	return pill
}

// isPill and isInk say whether a pixel of a frame shortW wide is the pill
// or the white of a letter.
func isPill(f []byte, pill [3]int, x, y int) bool {
	i := (y*shortW + x) * 3
	return abs(int(f[i])-pill[0]) < 40 && abs(int(f[i+1])-pill[1]) < 40 && abs(int(f[i+2])-pill[2]) < 40
}

func isInk(f []byte, x, y int) bool {
	i := (y*shortW + x) * 3
	return f[i] > 160 && f[i+1] > 160 && f[i+2] > 160
}

// pillBox is the box of the pill on a frame of rows rows, right below zero
// when there is none.
func pillBox(f []byte, pill [3]int, rows int) (left, right, top, bottom int) {
	left, right, top, bottom = shortW, -1, rows, -1
	for y := range rows {
		for x := range shortW {
			if isPill(f, pill, x, y) {
				left, right = min(left, x), max(right, x)
				top, bottom = min(top, y), max(bottom, y)
			}
		}
	}
	return
}

// On the real disk: ffmpeg renders the short and the test reads it back.
func TestThePillLeavesRoomRoundItsWord(t *testing.T) {
	t.Parallel()
	words := []Cue{{0.20, 0.40, "Das"}, {0.50, 1.00, "respektvoll"}, {1.00, 1.40, "ist."}}
	e, info, laid, frames := shortRows(t, words, 1.6, bounceTop, bounceRows)
	if len(laid) != 1 || len(laid[0].Lines) != 1 {
		t.Fatalf("the words should make one caption of one line, not %+v", laid)
	}
	const w, h = shortW, shortH
	style := ResolveStyle(nil)
	pill := pillColour(t, style)
	box := func(f []byte) (left, right, top, bottom int) { return pillBox(f, pill, bounceRows) }
	// "respektvoll" lights at 0.50 s, frame 25, and is lit until "ist."
	// at 1.00 s. Frame 40, 0.30 s on, is long settled.
	lit := 25
	if len(frames) < 45 {
		t.Fatalf("the short has %d frames", len(frames))
	}

	// The room the pill leaves around its word, on every frame the word is
	// lit: at least roomAround of the caption size above, below and at both
	// sides of the word's ink, at its first frame and at rest. The pill
	// never comes in tighter than at rest.
	// Tim saw the pill hug the word on #201: it opened from inside its rest
	// on every side, so the frame a click on a word lands on, its first,
	// showed it with no room at all, and at rest it reached only a tenth
	// of the size past the tallest letter of the line.
	room := func(name string, from, to int) {
		t.Helper()
		_, _, restT, restB := box(frames[to])
		restLeft, restRight, _, _ := box(frames[to])
		least := w
		defer func() {
			t.Logf("%s: the pill is %dx%d at rest and leaves at least %d pixels round the word", name,
				restRight-restLeft+1, restB-restT+1, least)
		}()
		// Its first frame and at rest: in between the word and its pill
		// grow by one scale, which keeps the room, see
		// TestTheWordAndItsPillPopTogether, and at the top of the pop the
		// pill of a long word reaches the next word's letters.
		for _, k := range []int{from, to} {
			f := frames[k]
			l, r, top, bottom := box(f)
			if r < 0 {
				t.Errorf("%s: no pill on frame %d", name, k)
				continue
			}
			il, ir, it, ib := w, -1, bounceRows, -1
			for y := top; y <= bottom; y++ {
				for x := l; x <= r; x++ {
					if isInk(f, x, y) {
						il, ir = min(il, x), max(ir, x)
						it, ib = min(it, y), max(ib, y)
					}
				}
			}
			if ir < 0 {
				t.Errorf("%s: no word inside the pill on frame %d", name, k)
				continue
			}
			want := roomAround*float64(pyround(style.Size)) - 1
			sides := map[string]int{"left of": il - l, "right of": r - ir, "above": it - top, "below": bottom - ib}
			for side, px := range sides {
				least = min(least, px)
				if float64(px) < want {
					t.Errorf("%s, %3d ms after it lit: the pill leaves %d pixels %s the word, less than %.0f",
						name, (k-from)*1000/bounceFPS, px, side, want)
				}
			}
			if r-l < restRight-restLeft-1 || bottom-top < restB-restT-1 {
				t.Errorf("%s, %3d ms after it lit: the pill is %dx%d, smaller than at rest, %dx%d",
					name, (k-from)*1000/bounceFPS, r-l+1, bottom-top+1, restRight-restLeft+1, restB-restT+1)
			}
		}
	}
	// "Das" lights at 0.20 s, frame 10, "respektvoll" at 0.50 s, frame 25.
	room("Das", 10, 24)
	room("respektvoll", lit, 40)

	// The frame the caption box puts round a word is where its pill rests,
	// so it stands round the word libass drew. Tim saw it stand off its
	// word on #201, laid out by the browser's own font.
	frames0 := e.WordFrames(context.Background(), laid, w, h, nil, info.Light())
	if len(frames0) != 1 || len(frames0[0]) != len(words) {
		t.Fatalf("word frames %+v, want one caption of %d words", frames0, len(words))
	}
	// "Das" rests on frame 24, "respektvoll" on frame 40.
	for i, at := range []int{24, 40} {
		l, r, top, bottom := box(frames[at])
		fr := frames0[0][i]
		got := [4]float64{fr.X * w, fr.Y*h - bounceTop, (fr.X + fr.W) * w, (fr.Y+fr.H)*h - bounceTop}
		drawn := [4]float64{float64(l), float64(top), float64(r + 1), float64(bottom + 1)}
		for side, name := range []string{"left", "top", "right", "bottom"} {
			if math.Abs(got[side]-drawn[side]) > 2 {
				t.Errorf("%s: the word frame's %s edge is at %.1f, the pill libass drew at %.0f",
					words[i].Text, name, got[side], drawn[side])
			}
		}
	}
}

// The word being spoken and its pill pop together: on every frame both
// are as large as popDrawn says, by one scale in width and in height,
// around the one centre they share, and at the top the scale reaches
// popPeak. Tim saw #201's bounce stretch the pill up or down alone while
// the word grew by 1 %, and on main the pill opened from smaller than the
// word.
//
// On the real disk: ffmpeg renders the short and the test reads it back.
func TestTheWordAndItsPillPopTogether(t *testing.T) {
	t.Parallel()
	words := []Cue{{0.50, 1.20, "respektvoll"}}
	_, _, laid, frames := shortRows(t, words, 1.4, bounceTop, bounceRows)
	if len(laid) != 1 || len(laid[0].Words) != 1 {
		t.Fatalf("the word should make one caption, not %+v", laid)
	}
	pill := pillColour(t, ResolveStyle(nil))
	// The ink of the word, the white of its letters anywhere.
	ink := func(f []byte) (left, right, top, bottom int) {
		left, right, top, bottom = shortW, -1, bounceRows, -1
		for y := range bounceRows {
			for x := range shortW {
				if isInk(f, x, y) {
					left, right = min(left, x), max(right, x)
					top, bottom = min(top, y), max(bottom, y)
				}
			}
		}
		return
	}
	type box struct{ l, r, t, b float64 }
	read := func(k int) (p, w box) {
		l, r, top, bottom := pillBox(frames[k], pill, bounceRows)
		il, ir, it, ib := ink(frames[k])
		if r < 0 || ir < 0 {
			t.Fatalf("frame %d has no pill or no word", k)
		}
		return box{float64(l), float64(r + 1), float64(top), float64(bottom + 1)},
			box{float64(il), float64(ir + 1), float64(it), float64(ib + 1)}
	}
	// It lights at 0.50 s, frame 25, and is lit for 70 hundredths.
	const lit, span = 25, 70
	if len(frames) < lit+20 {
		t.Fatalf("the short has %d frames", len(frames))
	}
	restPill, restWord := read(lit + 15)
	cx, cy := (restPill.l+restPill.r)/2, (restPill.t+restPill.b)/2
	// How far the word's ink stands from the pill's middle, which grows
	// with the scale.
	wordUp, wordDown := cy-restWord.t, restWord.b-cy
	peak := 0.0
	for k := lit; k <= lit+12; k++ {
		since := float64(k-lit) * 1000 / bounceFPS
		want := popDrawn(since)
		peak = max(peak, want)
		p, w := read(k)
		got := map[string][2]float64{
			"the pill's width":  {(p.r - p.l) / (restPill.r - restPill.l), restPill.r - restPill.l},
			"the pill's height": {(p.b - p.t) / (restPill.b - restPill.t), restPill.b - restPill.t},
			"the word's width":  {(w.r - w.l) / (restWord.r - restWord.l), restWord.r - restWord.l},
			"the word's top":    {(cy - w.t) / wordUp, wordUp},
			"the word's bottom": {(w.b - cy) / wordDown, wordDown},
		}
		for what, g := range got {
			// Two pixels of what was measured, for the edges libass smooths.
			if math.Abs(g[0]-want) > 2.5/g[1] {
				t.Errorf("%3.0f ms after the word lit %s is %.3f of its rest, popDrawn says %.3f", since, what, g[0], want)
			}
		}
		if math.Abs((p.l+p.r)/2-cx) > 1 || math.Abs((p.t+p.b)/2-cy) > 1 || math.Abs((w.l+w.r)/2-cx) > 1.5 {
			t.Errorf("%3.0f ms after the word lit the pill's middle is at %.1f, %.1f and the word's across at %.1f, not where they rest, %.1f, %.1f",
				since, (p.l+p.r)/2, (p.t+p.b)/2, (w.l+w.r)/2, cx, cy)
		}
	}
	if peak < popPeak-0.01 {
		t.Errorf("the pop reaches %.3f, not %.2f", peak, popPeak)
	}
}

// A word's first frame in the short shows it at rest, and its pop starts
// on the frame after, see SourceInfo.popStart. A step onto a word lands on
// its first frame, and Tim saw the pill there still grown, bigger than
// the frame round it, after the pop had played over the paused frame:
// the pop started where the word lit, between two frames, and its first
// frame caught it on its way up.
//
// On the real disk: ffmpeg renders the short and the test reads it back.
func TestAWordsFirstFrameShowsItAtRest(t *testing.T) {
	t.Parallel()
	// It lights at 0.43 s, half a frame before the frame at 0.44 s.
	words := []Cue{{0.43, 1.13, "respektvoll"}}
	_, _, _, frames := shortRows(t, words, 1.4, bounceTop, bounceRows)
	const first, rest = 22, 45
	if len(frames) <= rest {
		t.Fatalf("the short has %d frames", len(frames))
	}
	pill := pillColour(t, ResolveStyle(nil))
	width := func(k int) float64 {
		l, r, _, _ := pillBox(frames[k], pill, bounceRows)
		if r < 0 {
			t.Fatalf("frame %d has no pill", k)
		}
		return float64(r - l + 1)
	}
	if got, want := width(first), width(rest); math.Abs(got-want) > 2 {
		t.Errorf("on its first frame the pill is %.0f pixels wide, at rest %.0f", got, want)
	}
	if got, want := width(first+1), width(rest); got < want+10 {
		t.Errorf("on its second frame the pill is %.0f pixels wide, at rest %.0f: it does not pop", got, want)
	}
}

// The pill of the word being spoken, at rest, never reaches the letters of
// another line. Tim saw the "g" of "wichtig" stand in the top right
// corner of the pill of "Leute" on the line under it, on #201: libass
// stands the lines of a caption as close as the face is tall, and the
// pill reached past its line. The lines stay where libass puts them. For
// the few frames of its pop the word and its pill grow in every direction
// and may reach over the other line, see TestTheWordAndItsPillPopTogether.
//
// On the real disk: ffmpeg renders the short and the test reads it back.
func TestThePillAtRestNeverReachesAnotherLine(t *testing.T) {
	t.Parallel()
	// The caption Tim saw, the tail of the g over the L.
	words := []Cue{
		{0.10, 0.40, "Ganz"}, {0.40, 0.80, "wichtig."}, {0.80, 1.00, "So."}, {1.00, 1.30, "Für"},
		{1.30, 1.80, "Leute"}, {1.80, 2.10, "ist"}, {2.10, 2.40, "es"},
	}
	const top, rows = 1150, 650
	_, _, laid, frames := shortRows(t, words, 2.6, top, rows)
	if len(laid) != 1 || len(laid[0].Lines) != 2 || len(laid[0].Lines[1]) == 0 || laid[0].Lines[1][0].Text != "Leute" {
		t.Fatalf("the words should make one caption of two lines, the second from Leute, not %+v", laid)
	}
	style := ResolveStyle(nil)
	size := float64(pyround(style.Size))
	pill := pillColour(t, style)

	// lines are the rows each line of letters stands in, top to bottom. A
	// line's letters leave rows free inside it, between the dot of an i and
	// its stem, so rows closer than a tenth of the size are one line.
	lines := func(f []byte) [][2]int {
		var found [][2]int
		for y := range rows {
			ink := false
			for x := range shortW {
				if isInk(f, x, y) {
					ink = true
					break
				}
			}
			switch {
			case !ink:
			case len(found) > 0 && float64(y-found[len(found)-1][1]) < 0.1*size:
				found[len(found)-1][1] = y
			default:
				found = append(found, [2]int{y, y})
			}
		}
		return found
	}

	clear := lineClear*size - 2
	for i, word := range words {
		lit, to := int(math.Round(word.Start*bounceFPS)), int(math.Round(word.End*bounceFPS))-1
		// The first frame after its pop.
		from := lit + int(math.Ceil(popLength*bounceFPS/1000))
		if from > to {
			// Lit too short a time to come to rest.
			continue
		}
		for k := from; k <= to; k++ {
			f := frames[k]
			ms := (k - lit) * 1000 / bounceFPS
			l, r, ptop, pbottom := pillBox(f, pill, rows)
			if r < 0 {
				t.Errorf("%s, %3d ms after it lit: no pill", word.Text, ms)
				continue
			}
			drawn := lines(f)
			if len(drawn) != 2 {
				t.Errorf("%s, %3d ms after it lit: the letters stand in %d bands of rows, %v, not one for each of the two lines: the lines touch",
					word.Text, ms, len(drawn), drawn)
				continue
			}
			// The letters of the other line over or under the pill: the
			// nearest row of their ink in the pill's columns.
			other := drawn[1]
			if i >= 4 {
				other = drawn[0]
			}
			nearest := -1
			for y := other[0]; y <= other[1]; y++ {
				for x := l; x <= r; x++ {
					if isInk(f, x, y) {
						if nearest < 0 || i >= 4 {
							nearest = y
						}
						break
					}
				}
				if nearest >= 0 && i < 4 {
					break
				}
			}
			if nearest < 0 {
				continue
			}
			room := nearest - pbottom - 1
			if i >= 4 {
				room = ptop - nearest - 1
			}
			if float64(room) < clear {
				t.Errorf("%s, %3d ms after it lit: the pill, rows %d to %d, comes %d pixels from a letter of the other line at row %d, closer than %.0f",
					word.Text, ms, ptop, pbottom, room, nearest, clear)
			}
		}
	}
}

// Every pill of a line of a caption rests at one height, clear of the
// letters of the other line by lineClear. Where the letters of the two
// lines come nearer than that leaves room for, the tail of the "g" of
// "wichtig" over the dots of an "Ä", the pill keeps lineClear over its own
// letters and touches the other line. Read off where libass puts
// the words, for Tim's caption and for that one. A pill stopped short word
// by word jumped as it walked along the line, from "ist" to "es" by 14
// pixels.
//
// On the real disk: libass, through ffmpeg, measures where the words are.
func TestEveryPillOfALineIsOneHeightAndClearOfTheOther(t *testing.T) {
	t.Parallel()
	ctx := context.Background()
	e := NewEngine(NewLog(&bytes.Buffer{}, false, false), disk)
	if err := e.Preflight(ctx); err != nil {
		ffmpegtest.Unusable(t, "no usable ffmpeg here: %v", err)
	}
	if _, err := e.SubtitleFilter(ctx); err != nil {
		ffmpegtest.Unusable(t, "this ffmpeg cannot draw captions: %v", err)
	}
	style := ResolveStyle(nil)
	size := pyround(style.Size)
	clear := math.Round(float64(size) * lineClear)
	for _, second := range []string{"Leute", "Ägypten"} {
		words := []Cue{
			{0.10, 0.40, "Ganz"}, {0.40, 0.80, "wichtig."}, {0.80, 1.00, "So."}, {1.00, 1.30, "Für"},
			{1.30, 1.80, second}, {1.80, 2.10, "ist"}, {2.10, 2.40, "es"},
		}
		clip := Clip{ID: "01", Segments: []Segment{{Start: 0, End: 2.6}}}
		laid := ClipCaptions(clip, &Transcript{Words: words, Language: "de"}, style)
		if len(laid) != 1 || len(laid[0].Lines) != 2 {
			t.Fatalf("%s: the words should make one caption of two lines, not %+v", second, laid)
		}
		layouts, ok := e.layWords(ctx, laid, style, shortW, size)
		if !ok {
			t.Fatalf("%s: the words could not be measured", second)
		}
		L := layouts[0]
		var rows []string
		for r, line := range L.lines {
			inkTop, inkBottom := L.lineInk(r)
			for _, k := range line {
				left, top, right, bottom := L.pill(k, size)
				rows = append(rows, fmt.Sprintf("%-9s %4.0f to %4.0f", L.words[k], top, bottom))
				first := line[0]
				_, firstTop, _, firstBottom := L.pill(first, size)
				if top != firstTop || bottom != firstBottom {
					t.Errorf("%s: the pill of %q rests at %.0f to %.0f, that of %q on its line at %.0f to %.0f",
						second, L.words[k], top, bottom, L.words[first], firstTop, firstBottom)
				}
				// The other line's letters over or under the pill.
				for o := range L.lines {
					if o == r {
						continue
					}
					for _, j := range L.lines[o] {
						b := L.boxes[j]
						if float64(b.right) < left || float64(b.left) > right {
							continue
						}
						// A pill runs from its top to its bottom, the row of its
						// bottom the first one under it, the way an ink box's
						// bottom is its last row.
						gap, own := top-float64(b.bottom+1), float64(inkTop)-top
						if o > r {
							gap, own = float64(b.top)-bottom, bottom-float64(inkBottom+1)
						}
						tight := own < clear+0.5
						if gap < clear-0.5 && !tight {
							t.Errorf("%s: at rest the pill of %q comes %.0f pixels from %q on the other line, less than %.0f",
								second, L.words[k], gap, L.words[j], clear)
						}
						if own < clear-0.5 {
							t.Errorf("%s: the pill of %q leaves its own line %.0f pixels, less than %.0f", second, L.words[k], own, clear)
						}
					}
				}
			}
		}
		t.Logf("%s:\n%s", second, strings.Join(rows, "\n"))
	}
}
