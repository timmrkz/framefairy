package engine

import (
	"bytes"
	"context"
	"fmt"
	"math"
	"os/exec"
	"path/filepath"
	"testing"

	"framefairy/internal/ffmpegtest"
)

// The bounce of the word being spoken, read back from a finished short:
// the pill grows above and below by what BounceAt says on every frame,
// never wider than it rests, and a long word with its pill never reaches
// into the next word. Tim saw "respektvoll" on a thumbnail grow 15 % of
// its own width a side and its pill cover "ist".

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
func TestTheBounceKeepsTheSpaceBetweenWords(t *testing.T) {
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
	empty := func(f []byte, x int) bool {
		for y := range bounceRows {
			if isPill(f, pill, x, y) || isInk(f, x, y) {
				return false
			}
		}
		return true
	}

	// "respektvoll" lights at 0.50 s, frame 25, and is lit until "ist."
	// at 1.00 s. Frame 40, 0.30 s on, is long settled.
	lit := 25
	span := centiseconds(1.00) - centiseconds(0.50)
	if len(frames) < 45 {
		t.Fatalf("the short has %d frames", len(frames))
	}
	restL, restR, restT, restB := box(frames[40])
	if restR < 0 {
		t.Fatal("no pill behind respektvoll once it settled")
	}
	restW, restH := restR-restL+1, restB-restT+1
	next := restR + 1
	for next < w && empty(frames[40], next) {
		next++
	}
	if next-restR < 3 {
		t.Fatalf("at rest the pill ends at %d and ist starts at %d, no space to keep", restR, next)
	}
	grow := bounceGrow * float64(pyround(style.Size))

	// The room the pill leaves around its word, on every frame the word is
	// lit: at least roomAround of the caption size above, below and at both
	// sides of the word's ink, at its first frame, at the top of its
	// bounce and at rest. The pill never comes in tighter than at rest.
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
		for k := from; k <= to; k++ {
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

	for k := lit; k <= lit+10; k++ {
		since := float64(k-lit) / bounceFPS
		b := BounceAt(since, span)
		l, r, top, bottom := box(frames[k])
		pw, ph := r-l+1, bottom-top+1
		wantW := float64(restW)
		wantH := float64(restH) + 2*grow*b.Tall
		if math.Abs(float64(pw)-wantW) > 3 || math.Abs(float64(ph)-wantH) > 3 {
			t.Errorf("%3.0f ms after the word lit the pill is %dx%d, BounceAt says %.0fx%.0f", since*1000, pw, ph, wantW, wantH)
		}
		gap := false
		for x := restR + 1; x < next; x++ {
			if empty(frames[k], x) {
				gap = true
				break
			}
		}
		if !gap {
			t.Errorf("%3.0f ms after respektvoll lit nothing is left of the space before ist, between %d and %d: the pill reaches %d",
				since*1000, restR, next, r)
		}
	}
}

// bounceSeen is the least the pill of a word lit long enough to bounce
// grows in height at the top of its bounce, as a share of the caption
// size. Tim could hardly see a bounce of 0.16 on #201.
const bounceSeen = 0.25

// The pill of the word being spoken never reaches the letters of another
// line, at rest or at the top of its bounce, and its bounce is large
// enough to see. Tim saw the "g" of "wichtig" stand in the top right
// corner of the pill of "Leute" on the line under it, on #201: libass
// stands the lines of a caption as close as the face is tall, and the
// pill reached past its line. The lines stay where libass puts them.
//
// On the real disk: ffmpeg renders the short and the test reads it back.
func TestThePillNeverReachesAnotherLine(t *testing.T) {
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
		from, to := int(math.Round(word.Start*bounceFPS)), int(math.Round(word.End*bounceFPS))-1
		var restH, peakH int
		for k := from; k <= to; k++ {
			f := frames[k]
			ms := (k - from) * 1000 / bounceFPS
			l, r, ptop, pbottom := pillBox(f, pill, rows)
			if r < 0 {
				t.Errorf("%s, %3d ms after it lit: no pill", word.Text, ms)
				continue
			}
			restH, peakH = pbottom-ptop+1, max(peakH, pbottom-ptop+1)
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
		if grew := float64(peakH-restH) / size; grew < bounceSeen {
			t.Errorf("%s: the pill grows from %d to %d pixels at the top of its bounce, %.2f of the size, less than %.2f",
				word.Text, restH, peakH, grew, bounceSeen)
		}
	}
}
