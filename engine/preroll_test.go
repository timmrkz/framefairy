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
)

// A render reads each piece of a clip from where the piece starts, and a
// decoder that starts there is not settled yet. Opus needs 80 ms of the
// sound before, RFC 7845 section 4.6, and an episode whose sound was Opus
// was heard a little quiet at the start of every piece, about 70% of its
// level for the first 80 ms. MP3 keeps part of a frame in the frames
// before it, and was silent for the first 40 ms.

// TestEveryPieceIsHeardFromItsFirstMoment renders a clip with two cuts and
// a camera switch from an episode with a steady tone in Opus, in each
// container that carries Opus, and in MP3 where this ffmpeg can make it,
// and checks that every piece is as loud from its first moment as
// anywhere, after the fade a cut puts there. Every frame of the episode
// is a keyframe, as after a camera switch, where an encoder puts one, so
// ffmpeg's seek lands right on each piece.
func TestEveryPieceIsHeardFromItsFirstMoment(t *testing.T) {
	e := NewEngine(NewLog(&bytes.Buffer{}, false, false))
	if err := e.Preflight(context.Background()); err != nil {
		t.Skip("no usable ffmpeg here")
	}
	listed, _ := exec.Command("ffmpeg", "-hide_banner", "-encoders").Output()
	has := func(name string) bool { return strings.Contains(string(listed), " "+name+" ") }
	// The ffmpeg we ship has no libopus, only ffmpeg's own encoder, which
	// it calls experimental. Episodes are mostly made with libopus. It has
	// no MP3 encoder at all.
	opus := []string{"opus", "-strict", "experimental"}
	if has("libopus") {
		opus = []string{"libopus"}
	}
	cases := []struct {
		sound, container string
		encoder          []string
	}{
		// WebM is Matroska with fewer codecs, read by the same demuxer,
		// and the ffmpeg we ship has no encoder for its pictures.
		{"opus", "mp4", opus}, {"opus", "mkv", opus},
	}
	if has("libmp3lame") {
		cases = append(cases, struct {
			sound, container string
			encoder          []string
		}{"mp3", "mp4", []string{"libmp3lame"}})
	}
	for _, c := range cases {
		t.Run(c.sound+"_"+c.container, func(t *testing.T) {
			t.Parallel()
			withoutADip(t, e, c.container, c.encoder)
		})
	}
}

func withoutADip(t *testing.T, e *Engine, container string, encoder []string) {
	ctx := context.Background()
	dir := t.TempDir()
	source := filepath.Join(dir, "episode."+container)
	args := append([]string{"-loglevel", "error", "-y",
		"-f", "lavfi", "-i", "color=c=black:s=640x360:r=30000/1001:d=7,geq=lum='40+mod(N*6,180)':cb=128:cr=128",
		"-f", "lavfi", "-i", "sine=f=440:sample_rate=48000:d=7",
		"-shortest", "-c:v", "mpeg4", "-q:v", "2", "-g", "1", "-b:a", "64k", "-c:a"}, encoder...)
	if out, err := exec.Command("ffmpeg", append(args, source)...).CombinedOutput(); err != nil {
		t.Fatalf("making the test episode: %s %s", err, out)
	}
	info, err := e.Probe(ctx, source)
	if err != nil {
		t.Fatal(err)
	}
	// A cut, a camera switch, where the second piece runs straight into
	// the third, and another cut.
	clip := info.OnFrames(Clip{ID: "01", Slug: "opus", Segments: []Segment{
		{Start: 1.13, End: 1.9, CropX: intPtr(0)}, {Start: 2.47, End: 3.21, CropX: intPtr(0)},
		{Start: 3.21, End: 4.05, CropX: intPtr(280)}, {Start: 4.83, End: 5.7, CropX: intPtr(280)}}})
	rs := RenderSettings{OutW: 180, OutH: 320, CRF: 18, Preset: "ultrafast", AudioBitrate: "192k", ScaleUp: true}
	short, err := e.RenderClip(ctx, clip, source, info, filepath.Join(dir, "out"), nil, rs, nil,
		filepath.Join(dir, "captions"), false)
	if err != nil {
		t.Fatal(err)
	}

	out, err := exec.Command("ffmpeg", "-v", "error", "-i", short, "-ac", "1", "-ar", "48000", "-f", "f32le", "-").Output()
	if err != nil {
		t.Fatal(err)
	}
	samples := make([]float32, len(out)/4)
	if err := binary.Read(bytes.NewReader(out), binary.LittleEndian, samples); err != nil && err != io.EOF {
		t.Fatal(err)
	}
	// How loud the tone is from a moment on, over this many samples.
	loud := func(at float64, n int) float64 {
		from := int(at * 48000)
		sum := 0.0
		for _, s := range samples[from : from+n] {
			sum += float64(s) * float64(s)
		}
		return math.Sqrt(sum / float64(n))
	}
	// Steady is the middle of the first piece, far from both its ends, and
	// each moment is the 10 ms from it, more than four swings of the tone.
	steady := loud(0.25, 9600)
	start := 0.0
	for i, seg := range clip.Segments {
		// After a cut the sound fades in for 15 ms, and that is meant.
		from := start
		if i == 0 || seg.Start-clip.Segments[i-1].End > 0.0005 {
			from += Fade + 0.005
		}
		least := math.Inf(1)
		for x := from; x < start+0.15; x += 0.005 {
			got := loud(x, 480)
			least = math.Min(least, got)
			if got < 0.9*steady {
				t.Errorf("piece %d, %.0f ms in, is %.0f%% of the steady tone",
					i+1, 1000*(x-start), 100*got/steady)
			}
		}
		t.Logf("piece %d is at least %.0f%% of the steady tone", i+1, 100*least/steady)
		start += seg.Duration()
	}
}
