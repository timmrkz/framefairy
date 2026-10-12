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

// On the real disk: ffmpeg renders the short and the test reads it back.
func TestTheBounceKeepsTheSpaceBetweenWords(t *testing.T) {
	ctx := context.Background()
	e := NewEngine(NewLog(&bytes.Buffer{}, false, false), disk)
	if err := e.Preflight(ctx); err != nil {
		ffmpegtest.Unusable(t, "no usable ffmpeg here: %v", err)
	}
	if _, err := e.SubtitleFilter(ctx); err != nil {
		ffmpegtest.Unusable(t, "this ffmpeg cannot burn captions in: %v", err)
	}
	source := filepath.Join(t.TempDir(), "episode.mp4")
	if out, err := exec.Command("ffmpeg", "-loglevel", "error", "-y",
		"-f", "lavfi", "-i", fmt.Sprintf("color=c=black:s=640x360:r=%d:d=2", bounceFPS),
		"-f", "lavfi", "-i", "anullsrc=r=48000:cl=stereo:d=2",
		"-shortest", "-c:v", "mpeg4", "-q:v", "2", "-c:a", "aac", source).CombinedOutput(); err != nil {
		t.Fatalf("making the test episode: %s %s", err, out)
	}
	info, err := e.Probe(ctx, source)
	if err != nil {
		t.Fatal(err)
	}
	style := ResolveStyle(nil)
	words := []Cue{{0.20, 0.40, "Das"}, {0.50, 1.00, "respektvoll"}, {1.00, 1.40, "ist."}}
	clip := info.OnFrames(Clip{ID: "01", Slug: "bounce", Segments: []Segment{{Start: 0, End: 1.6}}})
	laid := ClipCaptions(clip, &Transcript{Words: words, Language: "de"}, style)
	if len(laid) != 1 || len(laid[0].Lines) != 1 {
		t.Fatalf("the words should make one caption of one line, not %+v", laid)
	}

	const w, h = 1080, 1920
	rs := RenderSettings{OutW: w, OutH: h, CRF: 12, Preset: "ultrafast", AudioBitrate: "128k", ScaleUp: true}
	dir := t.TempDir()
	short, err := e.RenderClip(ctx, clip, source, info, filepath.Join(dir, "out"), laid, rs, nil,
		filepath.Join(dir, "captions"), false)
	if err != nil {
		t.Fatal(err)
	}
	out, err := exec.Command("ffmpeg", "-v", "error", "-i", short, "-vf",
		fmt.Sprintf("crop=%d:%d:0:%d", w, bounceRows, bounceTop),
		"-f", "rawvideo", "-pix_fmt", "rgb24", "-").Output()
	if err != nil {
		t.Fatal(err)
	}
	size := w * bounceRows * 3
	var frames [][]byte
	for i := 0; i+size <= len(out); i += size {
		frames = append(frames, out[i:i+size])
	}

	var pill [3]int
	var alpha float64
	if _, err := fmt.Sscanf(style.HighlightWeb(), "rgba(%d, %d, %d, %g)", &pill[0], &pill[1], &pill[2], &alpha); err != nil {
		t.Fatal(err)
	}
	isPill := func(f []byte, x, y int) bool {
		i := (y*w + x) * 3
		return abs(int(f[i])-pill[0]) < 40 && abs(int(f[i+1])-pill[1]) < 40 && abs(int(f[i+2])-pill[2]) < 40
	}
	isInk := func(f []byte, x, y int) bool {
		i := (y*w + x) * 3
		return f[i] > 160 && f[i+1] > 160 && f[i+2] > 160
	}
	// The pill's box on a frame, and whether a column holds anything but
	// the caption's box.
	pillBox := func(f []byte) (left, right, top, bottom int) {
		left, right, top, bottom = w, -1, bounceRows, -1
		for y := range bounceRows {
			for x := range w {
				if isPill(f, x, y) {
					left, right = min(left, x), max(right, x)
					top, bottom = min(top, y), max(bottom, y)
				}
			}
		}
		return
	}
	empty := func(f []byte, x int) bool {
		for y := range bounceRows {
			if isPill(f, x, y) || isInk(f, x, y) {
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
	restL, restR, restT, restB := pillBox(frames[40])
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
		_, _, restT, restB := pillBox(frames[to])
		restLeft, restRight, _, _ := pillBox(frames[to])
		least := w
		defer func() {
			t.Logf("%s: the pill is %dx%d at rest and leaves at least %d pixels round the word", name,
				restRight-restLeft+1, restB-restT+1, least)
		}()
		for k := from; k <= to; k++ {
			f := frames[k]
			l, r, top, bottom := pillBox(f)
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
	frames0 := e.WordFrames(ctx, laid, w, h, nil, info.Light())
	if len(frames0) != 1 || len(frames0[0]) != len(words) {
		t.Fatalf("word frames %+v, want one caption of %d words", frames0, len(words))
	}
	// "Das" rests on frame 24, "respektvoll" on frame 40.
	for i, at := range []int{24, 40} {
		l, r, top, bottom := pillBox(frames[at])
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
		l, r, top, bottom := pillBox(frames[k])
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
