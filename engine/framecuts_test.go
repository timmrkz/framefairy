package engine

import (
	"bytes"
	"context"
	"encoding/binary"
	"io"
	"math"
	"os/exec"
	"path/filepath"
	"strings"
	"testing"

	"framefairy/internal/ffmpegtest"
)

// A render cuts on whole frames at every frame rate. Every piece of a short
// is the frames of the episode from the one its start is on up to the one
// before its end, and its sound is exactly as long as those frames. From a
// 29.97 fps episode a piece once got one frame more and about 30 ms of
// padded silence at every cut, because the start and the length of a piece
// went to ffmpeg rounded to the millisecond.

// TestARenderCutsOnWholeFrames renders a clip of three pieces with two
// cuts from short episodes at the rates cameras record at. The picture of
// the episode is a step brighter on every frame and back to dark every 30,
// so a frame lost, shown twice or taken from the wrong place shows. The
// sound is a steady tone, so padded silence at a cut, or sound that runs
// on after its picture, shows.
func TestARenderCutsOnWholeFrames(t *testing.T) {
	e := NewEngine(NewLog(&bytes.Buffer{}, false, false))
	if err := e.Preflight(context.Background()); err != nil {
		ffmpegtest.Unusable(t, "no usable ffmpeg here: %v", err)
	}
	for _, c := range []struct{ rate, container string }{
		{"30000/1001", "mov"}, {"24000/1001", "mov"}, {"24", "mov"},
		{"30", "mov"}, {"60", "mov"}, {"25", "mov"},
		// Matroska keeps time in milliseconds, so no frame of a 29.97
		// episode is where its number says, only near it.
		{"30000/1001", "mkv"},
	} {
		t.Run(strings.ReplaceAll(c.rate, "/", "_")+"_"+c.container, func(t *testing.T) {
			t.Parallel()
			cutsOnWholeFrames(t, e, c.rate, c.container)
		})
	}
}

func cutsOnWholeFrames(t *testing.T, e *Engine, rate, container string) {
	ctx := context.Background()
	dir := t.TempDir()
	source := filepath.Join(dir, "episode."+container)
	if out, err := exec.Command("ffmpeg", "-loglevel", "error", "-y",
		"-f", "lavfi", "-i", "color=c=black:s=640x360:r="+rate+":d=4,geq=lum='20+7*mod(N,30)':cb=128:cr=128",
		"-f", "lavfi", "-i", "sine=f=440:sample_rate=48000:d=4",
		"-shortest", "-c:v", "mpeg4", "-q:v", "2", "-c:a", "pcm_s16le", source).CombinedOutput(); err != nil {
		t.Fatalf("making the test episode: %s %s", err, out)
	}
	info, err := e.Probe(ctx, source)
	if err != nil {
		t.Fatal(err)
	}
	// The way the render takes a clip, see Run. The first piece starts on
	// the episode's first frame, and none is a whole number of
	// milliseconds long at 29.97.
	clip := info.OnFrames(Clip{ID: "01", Slug: "cuts", Segments: []Segment{
		{Start: 0, End: 0.9}, {Start: 1.47, End: 2.31}, {Start: 2.93, End: 3.62}}})
	const w, h = 180, 320
	rs := RenderSettings{OutW: w, OutH: h, CRF: 18, Preset: "ultrafast", AudioBitrate: "192k", ScaleUp: true}
	short, err := e.RenderClip(ctx, clip, source, info, filepath.Join(dir, "out"), nil, rs, nil,
		filepath.Join(dir, "captions"), false)
	if err != nil {
		t.Fatal(err)
	}

	// Seen: the frames of each piece in order, each once, and nothing
	// else. Which frame of the episode a frame of the short is, out of
	// every 30, is told by its brightness, measured on the episode itself.
	frame := float64(info.FPSDen) / float64(info.FPSNum)
	at := func(t float64) int { return int(math.Round(t / frame)) }
	var want []int
	var cuts []float64
	for _, seg := range clip.Segments {
		for k := at(seg.Start); k < at(seg.End); k++ {
			want = append(want, k)
		}
		cuts = append(cuts, float64(len(want))*frame)
	}
	cuts = cuts[:len(cuts)-1]
	levels := grayLevels(t, source, 640, 360)
	if len(levels) < 30 {
		t.Fatalf("the episode has %d frames", len(levels))
	}
	which := func(level float64) int {
		best := 0
		for k := 1; k < 30; k++ {
			if math.Abs(levels[k]-level) < math.Abs(levels[best]-level) {
				best = k
			}
		}
		return best
	}
	got := grayLevels(t, short, w, h)
	if len(got) != len(want) {
		t.Errorf("the short has %d frames, its pieces hold %d", len(got), len(want))
	}
	wrong := 0
	for i := 0; i < min(len(got), len(want)); i++ {
		if k := which(got[i]); k != want[i]%30 {
			if wrong++; wrong <= 5 {
				t.Errorf("frame %d of the short is frame %d of the episode, or one 30 from it, not %d",
					i, k, want[i])
			}
		}
	}

	// Heard: the tone right up to each cut, quiet only for the fade out
	// and in the render puts there, and that quiet exactly where the
	// picture cuts. Padded silence would make it longer, and sound longer
	// than its picture would move every cut after it later.
	out, err := exec.Command("ffmpeg", "-v", "error", "-i", short, "-ac", "1", "-ar", "48000", "-f", "f32le", "-").Output()
	if err != nil {
		t.Fatal(err)
	}
	samples := make([]float32, len(out)/4)
	if err := binary.Read(bytes.NewReader(out), binary.LittleEndian, samples); err != nil && err != io.EOF {
		t.Fatal(err)
	}
	if long, pictured := float64(len(samples))/48000, float64(len(want))*frame; math.Abs(long-pictured) > 0.025 {
		t.Errorf("the sound is %.3f s long, the picture %.3f s", long, pictured)
	}
	// The loudest sample in the 2.5 ms around each one, which is more than
	// one swing of the tone, so it is how loud the tone is there.
	const half = 60
	loud := func(k int) float64 {
		most := 0.0
		for j := max(0, k-half); j < min(len(samples), k+half); j++ {
			most = math.Max(most, math.Abs(float64(samples[j])))
		}
		return most
	}
	steady := loud(int(0.5 * 48000))
	for n, cut := range cuts {
		from, to := int((cut-0.06)*48000), int((cut+0.06)*48000)
		if to > len(samples) {
			t.Errorf("the sound ends before cut %d at %.3f s", n+1, cut)
			continue
		}
		first, last := -1, -1
		for k := from; k < to; k++ {
			if loud(k) < 0.25*steady {
				if first < 0 {
					first = k
				}
				last = k
			}
		}
		if first < 0 {
			t.Errorf("cut %d at %.3f s has no fade in its sound", n+1, cut)
			continue
		}
		quiet, mid := float64(last-first)/48000, float64(first+last)/2/48000
		if quiet > 0.015 {
			t.Errorf("the sound is quiet for %.1f ms at cut %d, more than its fade", 1000*quiet, n+1)
		}
		if math.Abs(mid-cut) > 0.003 {
			t.Errorf("the sound cuts at %.4f s, the picture at %.4f s, cut %d", mid, cut, n+1)
		}
	}
}

// grayLevels is how bright the middle of each frame of a video is.
func grayLevels(t *testing.T, path string, w, h int) []float64 {
	t.Helper()
	out, err := exec.Command("ffmpeg", "-v", "error", "-i", path, "-f", "rawvideo", "-pix_fmt", "gray", "-").Output()
	if err != nil {
		t.Fatal(err)
	}
	var levels []float64
	for i := 0; i+w*h <= len(out); i += w * h {
		sum, count := 0, 0
		for y := h / 4; y < 3*h/4; y++ {
			for x := w / 4; x < 3*w/4; x++ {
				sum += int(out[i+y*w+x])
				count++
			}
		}
		levels = append(levels, float64(sum)/float64(count))
	}
	return levels
}
