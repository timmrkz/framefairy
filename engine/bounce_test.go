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

	"framefairy/engine/num"
	"framefairy/engine/runlog"
	"framefairy/engine/timed"
	"framefairy/internal/ffmpegtest"
)

// The pop of the word being spoken, read back from the frames of a short: the
// word and its pill grow together, by one scale around one centre, in
// every direction, as far as popScales says on every frame, and at rest the
// pill leaves room round its word. Tim saw #201's bounce move the pill
// only up or down and the word hardly at all.

const bounceFPS = 50

// roomAround is the least room the pill leaves around its word on any
// frame, as a share of the caption size.
const roomAround = 0.10

// bounceBand is the rows of a 1080x1920 short the caption line is read
// from: the line sits 300 pixels over the bottom.
const bounceTop, bounceRows = 1400, 400

// shortRows draws words as the captions of a short from 0 to end of a
// black episode at bounceFPS, 1080x1920, and reads back rows of it from
// top, one rgb24 frame after another. shortRowsAt does it at fps.
//
// On the real disk: ffmpeg draws the short and the test reads it back.
func shortRows(t *testing.T, words []timed.Cue, end float64, top, rows int) (e *Engine, info SourceInfo, laid []LaidCaption, frames [][]byte) {
	t.Helper()
	return shortRowsAt(t, fmt.Sprint(bounceFPS), words, end, top, rows)
}

// shortRowsAt takes the rate as ffmpeg does, "25" or "30000/1001". What
// is checked is how libass draws the captions, so the frames are the
// render's own graph, the caption file and the face it burns in included,
// read straight out of it. Encoding them and decoding them again would
// only blur the edges the checks measure, and took most of the time.
// renderedRowsAt reads them out of a short that was rendered.
//
// On the real disk: ffmpeg draws the short and the test reads it back.
func shortRowsAt(t *testing.T, rate string, words []timed.Cue, end float64, top, rows int) (e *Engine, info SourceInfo, laid []LaidCaption, frames [][]byte) {
	t.Helper()
	ctx := context.Background()
	e, source, info, clip, laid := bounceEpisode(t, rate, words, end)
	rs := RenderSettings{OutW: shortW, OutH: shortH, ScaleUp: true}
	dir := t.TempDir()
	if err := e.writeASS(ctx, laid, filepath.Join(dir, "01.ass"), shortW, shortH, nil, info); err != nil {
		t.Fatal(err)
	}
	if err := InstallFont(dir, ResolveStyle(nil).Font); err != nil {
		t.Fatal(err)
	}
	return e, info, laid, graphFrames(t, e, clip, source, info, rs, dir, "01.ass",
		fmt.Sprintf("crop=%d:%d:0:%d,", shortW, rows, top), shortW, rows)
}

// renderedRowsAt is shortRowsAt read out of a short that was rendered,
// encoded and all, and decoded again.
//
// On the real disk: ffmpeg renders the short and the test reads it back.
func renderedRowsAt(t *testing.T, rate string, words []timed.Cue, end float64, top, rows int) (e *Engine, info SourceInfo, laid []LaidCaption, frames [][]byte) {
	t.Helper()
	ctx := context.Background()
	e, source, info, clip, laid := bounceEpisode(t, rate, words, end)
	rs := RenderSettings{OutW: shortW, OutH: shortH, CRF: 12, Preset: "ultrafast", AudioBitrate: "128k", ScaleUp: true}
	dir := t.TempDir()
	short, err := e.RenderClip(ctx, clip, source, info, filepath.Join(dir, "out"), laid, rs, nil,
		filepath.Join(dir, "captions"), false)
	if err != nil {
		t.Fatal(err)
	}
	cmd := exec.Command("ffmpeg", "-v", "error", "-i", short, "-vf",
		fmt.Sprintf("crop=%d:%d:0:%d", shortW, rows, top),
		"-f", "rawvideo", "-pix_fmt", "rgb24", "-")
	return e, info, laid, readFrames(t, cmd, shortW*rows*3)
}

// bounceEpisode is a black episode at rate a little longer than end, and
// the clip from 0 to end of it with words as its captions.
//
// On the real disk: ffmpeg makes the episode.
func bounceEpisode(t *testing.T, rate string, words []timed.Cue, end float64) (e *Engine, source string, info SourceInfo, clip Clip, laid []LaidCaption) {
	t.Helper()
	ctx := context.Background()
	e = NewEngine(runlog.New(&bytes.Buffer{}, false, false), disk)
	if err := e.Preflight(ctx); err != nil {
		ffmpegtest.Unusable(t, "no usable ffmpeg here: %v", err)
	}
	if _, err := e.SubtitleFilter(ctx); err != nil {
		ffmpegtest.Unusable(t, "this ffmpeg cannot burn captions in: %v", err)
	}
	source = filepath.Join(t.TempDir(), "episode.mp4")
	long := math.Ceil(end) + 1
	if out, err := exec.Command("ffmpeg", "-loglevel", "error", "-y",
		"-f", "lavfi", "-i", fmt.Sprintf("color=c=black:s=640x360:r=%s:d=%g", rate, long),
		"-f", "lavfi", "-i", fmt.Sprintf("anullsrc=r=48000:cl=stereo:d=%g", long),
		"-shortest", "-c:v", "mpeg4", "-q:v", "2", "-c:a", "aac", source).CombinedOutput(); err != nil {
		t.Fatalf("making the test episode: %s %s", err, out)
	}
	info, err := e.Probe(ctx, source)
	if err != nil {
		t.Fatal(err)
	}
	clip = info.OnFrames(Clip{ID: "01", Slug: "bounce", Segments: []Segment{{Start: 0, End: end}}})
	laid = ClipCaptions(clip, &Transcript{Words: words, Language: "de"}, ResolveStyle(nil))
	return e, source, info, clip, laid
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

// On the real disk: ffmpeg draws the short's frames and the test reads them.
func TestThePillLeavesRoomRoundItsWord(t *testing.T) {
	t.Parallel()
	words := []timed.Cue{{Start: 0.20, End: 0.40, Text: "Das"}, {Start: 0.50, End: 1.00, Text: "respektvoll"}, {Start: 1.00, End: 1.40, Text: "ist."}}
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
			want := roomAround*float64(num.Round(style.Size)) - 1
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
	// The baseline the word being typed is set on is the one libass set
	// the word on: the bottom of "Das", which has no tail. Tim saw a word
	// he opened drop by a few pixels on #201, set by the browser in the
	// middle of its pill.
	f := frames[24]
	l, r, top, bottom := box(f)
	ink := -1
	for y := top; y <= bottom; y++ {
		for x := l; x <= r; x++ {
			if isInk(f, x, y) {
				ink = y
			}
		}
	}
	if base := frames0[0][0].Base*h - bounceTop; math.Abs(base-float64(ink+1)) > 1 {
		t.Errorf("Das: the word frame's baseline is at %.1f, libass set the word on %d", base, ink+1)
	}
}

// The word being spoken and its pill pop together: on every frame both
// are as large as popScales says, by one scale in width and in height,
// around the one centre they share, and at the top the scale reaches
// popPeak. Tim saw #201's bounce stretch the pill up or down alone while
// the word grew by 1 %, and on main the pill opened from smaller than the
// word.
//
// On the real disk: ffmpeg draws the short's frames and the test reads them.
func TestTheWordAndItsPillPopTogether(t *testing.T) {
	t.Parallel()
	words := []timed.Cue{{Start: 0.50, End: 1.20, Text: "respektvoll"}}
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
	// It lights at 0.50 s, frame 25, and is lit for 35 frames.
	const lit = 25
	scales := popScales(35, bounceFPS)
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
		want := 1.0
		if k-lit < len(scales) {
			want = scales[k-lit]
		}
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
				t.Errorf("%3.0f ms after the word lit %s is %.3f of its rest, the pop says %.3f", since, what, g[0], want)
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
// on the frame after, see popScales. A step onto a word lands on
// its first frame, and Tim saw the pill there still grown, bigger than
// the frame round it, after the pop had played over the paused frame:
// the pop started where the word lit, between two frames, and its first
// frame caught it on its way up.
//
// On the real disk: ffmpeg draws the short's frames and the test reads them.
func TestAWordsFirstFrameShowsItAtRest(t *testing.T) {
	t.Parallel()
	// It lights at 0.43 s, half a frame before the frame at 0.44 s.
	words := []timed.Cue{{Start: 0.43, End: 1.13, Text: "respektvoll"}}
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
// On the real disk: ffmpeg draws the short's frames and the test reads them.
func TestThePillAtRestNeverReachesAnotherLine(t *testing.T) {
	t.Parallel()
	// The caption Tim saw, the tail of the g over the L.
	words := []timed.Cue{
		{Start: 0.10, End: 0.40, Text: "Ganz"}, {Start: 0.40, End: 0.80, Text: "wichtig."}, {Start: 0.80, End: 1.00, Text: "So."}, {Start: 1.00, End: 1.30, Text: "Für"},
		{Start: 1.30, End: 1.80, Text: "Leute"}, {Start: 1.80, End: 2.10, Text: "ist"}, {Start: 2.10, End: 2.40, Text: "es"},
	}
	const top, rows = 1150, 650
	_, _, laid, frames := shortRows(t, words, 2.6, top, rows)
	if len(laid) != 1 || len(laid[0].Lines) != 2 || len(laid[0].Lines[1]) == 0 || laid[0].Lines[1][0].Text != "Leute" {
		t.Fatalf("the words should make one caption of two lines, the second from Leute, not %+v", laid)
	}
	style := ResolveStyle(nil)
	size := float64(num.Round(style.Size))
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
		from := lit + len(popScales(to-lit+1, bounceFPS))
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
// pixels. And each pill stands round the middle of its line, as far over
// it as under it: stopped short on one side only, the pills of the top
// line stood high over their words and those of the bottom line low under
// them, and Tim saw "so'n" pushed up in its pill and "einem" down. The
// line is measured by its capitals and tall letters, not by accents over
// capitals, which put the middle of every pill high over its words.
//
// On the real disk: libass, through ffmpeg, measures where the words are.
func TestEveryPillOfALineIsOneHeightAndClearOfTheOther(t *testing.T) {
	t.Parallel()
	ctx := context.Background()
	e := NewEngine(runlog.New(&bytes.Buffer{}, false, false), disk)
	if err := e.Preflight(ctx); err != nil {
		ffmpegtest.Unusable(t, "no usable ffmpeg here: %v", err)
	}
	if _, err := e.SubtitleFilter(ctx); err != nil {
		ffmpegtest.Unusable(t, "this ffmpeg cannot draw captions: %v", err)
	}
	style := ResolveStyle(nil)
	size := num.Round(style.Size)
	clear := math.Round(float64(size) * lineClear)
	for _, second := range []string{"Leute", "Ägypten", ""} {
		words := []timed.Cue{
			{Start: 0.10, End: 0.40, Text: "Ganz"}, {Start: 0.40, End: 0.80, Text: "wichtig."}, {Start: 0.80, End: 1.00, Text: "So."}, {Start: 1.00, End: 1.30, Text: "Für"},
			{Start: 1.30, End: 1.80, Text: second}, {Start: 1.80, End: 2.10, Text: "ist"}, {Start: 2.10, End: 2.40, Text: "es"},
		}
		if second == "" {
			second = "Regenschirm."
			words = []timed.Cue{{Start: 0.10, End: 0.40, Text: "so'n"}, {Start: 0.40, End: 0.80, Text: "Typen"}, {Start: 0.80, End: 1.00, Text: "mit"}, {Start: 1.00, End: 1.20, Text: "so"},
				{Start: 1.20, End: 1.50, Text: "einem"}, {Start: 1.50, End: 2.40, Text: "Regenschirm."}}
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
				rows = append(rows, fmt.Sprintf("%-12s %4.0f to %4.0f", L.words[k], top, bottom))
				// Round the line's reach and any letter of it that reaches
				// further, unless the other line leaves its letters no more
				// than lineClear.
				reach := L.reach[r]
				high, low := math.Min(float64(reach.top), float64(inkTop)), math.Max(float64(reach.bottom), float64(inkBottom))
				over, under := high-top, bottom-low
				pressed := float64(inkTop)-top < clear+0.5 || bottom-float64(inkBottom+1) < clear+0.5
				// A word of small letters only stands in the middle of its
				// pill. "so" stood 45 pixels under its top and 26 over its
				// bottom.
				if b := L.boxes[k]; L.words[k] == "so" {
					if over, under := float64(b.top)-top, bottom-float64(b.bottom); math.Abs(over-under) > 0.15*float64(size) {
						t.Errorf("%s: %q stands %.0f pixels under the top of its pill and %.0f over its bottom", second, L.words[k], over, under)
					}
				}
				if math.Abs(over-under) > 1 && !pressed {
					t.Errorf("%s: the pill of %q reaches %.0f pixels over its line and %.0f under it", second, L.words[k], over, under)
				}
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

// Every word of a short at 25 fps, the rate of Tim's videos, read back
// frame by frame: words lit for 1 to 10 frames each pop as popScales says
// for the frames they have, at rest on their first frame, at popPeak on a
// frame, and at rest again by their last when they have three or more.
// "ja" is lit for one frame first, and stands at popPeak on it, measured
// against the same word at rest at the end.
// Tim's recording of a caption at 25 fps showed "Typen" pop fully, "mit"
// rise and snap back, "so" and "einem" grow on one frame, and "so'n" not
// at all: the pop was a curve in milliseconds, and where the frames fell
// on it was chance.
// At 29.97 fps too, where frames start between hundredths of a second and
// the caption file rounds them to the hundredth: read back from libass
// only at 25 and 50 fps before, where every frame starts on a whole
// hundredth, found by the second review.
//
// On the real disk: ffmpeg draws the short's frames and the test reads them.
func TestEveryWordOfAShortPopsOnItsFrames(t *testing.T) {
	t.Parallel()
	for _, rate := range []string{"25", "30000/1001"} {
		t.Run(strings.ReplaceAll(rate, "/", " in "), func(t *testing.T) {
			t.Parallel()
			everyWordPops(t, rate)
		})
	}
}

// On the real disk: ffmpeg draws the short's frames and the test reads them.
func everyWordPops(t *testing.T, rate string) {
	words := []timed.Cue{{Start: 0.40, End: 0.48, Text: "Das"}, {Start: 0.48, End: 0.60, Text: "ist"}, {Start: 0.60, End: 0.64, Text: "ja"},
		{Start: 0.64, End: 0.76, Text: "so"}, {Start: 0.76, End: 0.96, Text: "ein"}, {Start: 0.96, End: 1.24, Text: "gutes"}, {Start: 1.24, End: 1.64, Text: "ja"}}
	_, info, _, frames := shortRowsAt(t, rate, words, 1.64, bounceTop, bounceRows)
	fps := float64(info.FPSNum) / float64(info.FPSDen)
	pill := pillColour(t, ResolveStyle(nil))

	// The frames each word is lit on, by where its pill stands: a word's
	// pill keeps its middle through its pop.
	type lit struct {
		x, y   float64
		widths []float64
	}
	var seen []lit
	for _, f := range frames {
		l, r, top, bottom := pillBox(f, pill, bounceRows)
		if r < 0 {
			continue
		}
		x, y := float64(l+r)/2, float64(top+bottom)/2
		if n := len(seen); n == 0 || math.Abs(seen[n-1].x-x) > 4 || math.Abs(seen[n-1].y-y) > 4 {
			seen = append(seen, lit{x: x, y: y})
		}
		seen[len(seen)-1].widths = append(seen[len(seen)-1].widths, float64(r-l+1))
	}
	if len(seen) != len(words) {
		t.Fatalf("%d words lit in the short, not %d", len(seen), len(words))
	}
	for i, w := range seen {
		name := words[i].Text
		want := popScales(len(w.widths), fps)
		rest := w.widths[0]
		if len(w.widths) == 1 {
			// A word of one frame has no frame at rest, so its rest is
			// the same word's at the end, where its pill starts at rest.
			rest = seen[len(seen)-1].widths[0]
		}
		var got []string
		top := 0.0
		for k, width := range w.widths {
			scale := width / rest
			top = max(top, scale)
			got = append(got, fmt.Sprintf("%.3f", scale))
			should := 1.0
			if k < len(want) {
				should = want[k]
			}
			// Three pixels of the pill, for the edges libass smooths,
			// which the Mac's build draws a pixel or two wider.
			if math.Abs(scale-should) > 3.5/rest {
				t.Errorf("%s, lit for %d frames, frame %d: the pill is at %.3f, not %.3f", name, len(w.widths), k, scale, should)
			}
		}
		if math.Abs(top-popPeak) > 3.5/rest {
			t.Errorf("%s, lit for %d frames, pops to %.3f, not %.2f: %v", name, len(w.widths), top, popPeak, got)
		}
		if n := len(w.widths); n >= 3 && math.Abs(w.widths[n-1]/rest-1) > 3.5/rest {
			t.Errorf("%s, lit for %d frames, ends its last frame at %.3f, not at rest: %v", name, n, w.widths[n-1]/rest, got)
		}
		t.Logf("%-10s %2d frames: %v", name, len(w.widths), got)
	}
}

// At 120 fps two frames of the short can start in one hundredth of a
// second, the caption file's step, and libass draws the second's scale on
// both. PopScale says what libass draws on every frame all the same, read
// back from the short's frames: it said the first's before, on 3240 frames of 3
// seconds of words in the second review's simulation.
//
// On the real disk: ffmpeg draws the short's frames and the test reads them.
func TestPopScaleIsWhatLibassDrawsAbove100Fps(t *testing.T) {
	t.Parallel()
	words := []timed.Cue{{Start: 0.10, End: 0.30, Text: "Das"}, {Start: 0.30, End: 0.42, Text: "ist"}, {Start: 0.42, End: 0.70, Text: "gut"}}
	_, info, laid, frames := shortRowsAt(t, "120", words, 0.70, bounceTop, bounceRows)
	d := &CaptionDrawing{Laid: laid, Clip: info.OnFrames(Clip{ID: "01", Slug: "bounce", Segments: []Segment{{Start: 0, End: 0.70}}})}
	pill := pillColour(t, ResolveStyle(nil))
	fps := float64(info.FPSNum) / float64(info.FPSDen)
	// Each word by where its pill stands, which keeps its middle through
	// its pop, and its width at rest from its first frame. That frame may
	// share its hundredth with the next and show the next one's scale,
	// 1.018 here, so its width is taken by the scale PopScale says.
	middle, rest := -100.0, 0.0
	checked, grown := 0, 0
	for k, f := range frames {
		l, r, _, _ := pillBox(f, pill, bounceRows)
		if r < 0 {
			continue
		}
		scale := d.PopScale(info, float64(k)/fps)
		if x := float64(l+r) / 2; math.Abs(x-middle) > 4 {
			middle, rest = x, float64(r-l+1)/scale
			continue
		}
		drawn := float64(r-l+1) / rest
		if math.Abs(drawn-scale) > 3.5/rest {
			t.Errorf("frame %d at %.4f s: libass draws the pill at %.3f, PopScale says %.3f", k, float64(k)/fps, drawn, scale)
		}
		checked++
		if scale > 1.01 {
			grown++
		}
	}
	if checked < 60 || grown < 30 {
		t.Errorf("only %d frames were checked, %d of them grown", checked, grown)
	}
}

// WordsLit is what a rendered short lights, frame for frame: which word's
// pill each frame shows, and that a frame before the captions shows none.
// Held only to Showing before, through the same moments, which proved the
// frames were counted one way and not that libass shows that, found by
// the second review. "so" is lit for 20 ms between two frames and on
// none.
//
// On the real disk: ffmpeg renders the short and the test reads it back.
func TestWordsLitIsWhatTheShortLights(t *testing.T) {
	t.Parallel()
	words := []timed.Cue{{Start: 0.30, End: 0.62, Text: "Ich"}, {Start: 0.62, End: 0.64, Text: "so"}, {Start: 0.64, End: 1.02, Text: "war"}, {Start: 1.02, End: 1.50, Text: "da"}}
	_, info, laid, frames := renderedRowsAt(t, "25", words, 1.60, bounceTop, bounceRows)
	clip := info.OnFrames(Clip{ID: "01", Slug: "bounce", Segments: []Segment{{Start: 0, End: 1.60}}})
	d := &CaptionDrawing{Laid: laid, Clip: clip}
	lit := d.WordsLit(info)
	pill := pillColour(t, ResolveStyle(nil))
	first, _ := info.pieceFrames(clip.Segments[0].Start, clip.Segments[0].End)
	none, changes := 0, 0
	was := CaptionShowing{-1, -1}
	middle := -100.0
	for k, f := range frames {
		at := info.keptAt(first + int64(k))
		want := CaptionShowing{-1, -1}
		for i := range lit {
			for w, span := range lit[i] {
				if at >= span[0] && at < span[1] {
					want = CaptionShowing{i, w}
				}
			}
		}
		l, r, _, _ := pillBox(f, pill, bounceRows)
		if (r < 0) != (want.Word < 0) {
			t.Errorf("frame %d at %.3f s shows a pill %v, WordsLit has caption %d word %d there", k, at, r >= 0, want.Caption, want.Word)
		}
		if r < 0 {
			none++
			middle, was = -100, want
			continue
		}
		// A new word lit is a pill somewhere else, on the frame WordsLit
		// says it begins, and on no other.
		x := float64(l+r) / 2
		moved := math.Abs(x-middle) > 4
		if k > 0 && moved != (want != was) {
			t.Errorf("frame %d at %.3f s: the pill moved %v, WordsLit goes from caption %d word %d to caption %d word %d",
				k, at, moved, was.Caption, was.Word, want.Caption, want.Word)
		}
		if moved {
			changes++
		}
		middle, was = x, want
	}
	if none == 0 || changes != 3 {
		t.Errorf("%d frames without a pill and %d words lit, not some and 3", none, changes)
	}
	if got := lit[0][1]; got != [2]float64{-1, -1} {
		t.Errorf(`"so", lit for 20 ms between two frames, is lit from %.3f to %.3f s, on no frame was wanted`, got[0], got[1])
	}
}
