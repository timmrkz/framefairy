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

// bounceBand is the rows of a 1080x1920 short the caption line is read
// from: the line sits 300 pixels over the bottom.
const bounceTop, bounceRows = 1400, 400

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

	for k := lit; k <= lit+10; k++ {
		since := float64(k-lit) / bounceFPS
		b := BounceAt(since, span)
		l, r, top, bottom := pillBox(frames[k])
		pw, ph := r-l+1, bottom-top+1
		wantW := float64(restW) + 2*grow*b.Wide
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
