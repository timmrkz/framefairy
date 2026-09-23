package engine

import (
	"context"
	"io"
	"math"
	"os"
	"os/exec"
	"path/filepath"
	"runtime"
	"testing"
)

// twoCameras is an episode shot on two cameras: the first for ten seconds,
// the second for ten, then the first again.
func twoCameras(t *testing.T) (string, SourceInfo) {
	t.Helper()
	if _, err := exec.LookPath("ffmpeg"); err != nil {
		t.Skip("ffmpeg is not installed")
	}
	path := filepath.Join(t.TempDir(), "two.mp4")
	out, err := exec.Command("ffmpeg", "-loglevel", "error", "-y",
		"-f", "lavfi", "-i", "testsrc=s=640x360:r=25:d=10",
		"-f", "lavfi", "-i", "smptebars=s=640x360:r=25:d=10",
		"-f", "lavfi", "-i", "testsrc=s=640x360:r=25:d=10",
		"-filter_complex", "[0:v][1:v][2:v]concat=n=3:v=1:a=0",
		"-c:v", "libx264", "-preset", "ultrafast", "-g", "25", path).CombinedOutput()
	if err != nil {
		t.Fatalf("making the episode: %s %s", err, out)
	}
	return path, SourceInfo{Width: 640, Height: 360, Duration: 30}
}

// framingEngine frames by what is in focus rather than by faces. These
// tests are about which shots a clip is made of, and looking for faces in
// every sampled frame under the race detector is most of a minute.
func framingEngine() *Engine {
	e := NewEngine(NewLog(io.Discard, false, false))
	e.NoFaces = true
	return e
}

func TestFrameDifference(t *testing.T) {
	a := []byte{0, 10, 20, 30}
	if d := frameDifference(a, a); d != 0 {
		t.Errorf("the same frame differs by %v", d)
	}
	if d := frameDifference(a, []byte{10, 0, 30, 20}); d != 10 {
		t.Errorf("difference %v, want 10", d)
	}
	if d := frameDifference(nil, a); d != 255 {
		t.Errorf("nothing to compare reads %v", d)
	}
}

func TestAClipThatComesBackToItsCameraIsOneShot(t *testing.T) {
	path, source := twoCameras(t)
	e := framingEngine()
	ctx := context.Background()

	// Out of the first camera, over everything on the second, and back to
	// the first: one shot, measured once, framed the same on both sides.
	cache := newCropCache()
	segments, err := e.ClipSegments(ctx, path, []Span{{2, 8}, {22, 28}}, source, 202, cache)
	if err != nil {
		t.Fatal(err)
	}
	if len(segments) != 2 || len(cache.crops) != 1 {
		t.Fatalf("%d segment(s), %d shot(s) measured, want 2 and 1", len(segments), len(cache.crops))
	}
	if fmtCrop(segments[0].CropX) != fmtCrop(segments[1].CropX) {
		t.Errorf("one camera framed two ways: %s and %s", fmtCrop(segments[0].CropX), fmtCrop(segments[1].CropX))
	}

	// Out of the first camera and back on the second: two shots.
	cache = newCropCache()
	if _, err := e.ClipSegments(ctx, path, []Span{{2, 8}, {12, 18}}, source, 202, cache); err != nil {
		t.Fatal(err)
	}
	if len(cache.crops) != 2 {
		t.Errorf("coming back on another camera measured %d shot(s)", len(cache.crops))
	}

	// A kept span with a switch inside it is split at the switch.
	cache = newCropCache()
	segments, err = e.ClipSegments(ctx, path, []Span{{7, 13}}, source, 202, cache)
	if err != nil {
		t.Fatal(err)
	}
	if len(segments) != 2 || math.Abs(segments[0].End-10) > 0.1 {
		t.Errorf("a switch inside a span: %+v", segments)
	}
}

func fmtCrop(x *int) string {
	if x == nil {
		return "none"
	}
	return itoa(*x)
}

// A system decoder that will not take the file is gone round, not failed
// on: the same work is done on the processor, and so is everything after.
func TestFramingDecodesOnTheProcessorWhenTheSystemWillNot(t *testing.T) {
	if runtime.GOOS == "windows" {
		t.Skip("the stand-in ffmpeg is a shell script")
	}
	path, source := twoCameras(t)
	real, _ := exec.LookPath("ffmpeg")
	refusing := filepath.Join(t.TempDir(), "ffmpeg")
	script := "#!/bin/sh\ncase \"$*\" in *-hwaccel*) echo refused >&2; exit 1;; esac\nexec \"" + real + "\" \"$@\"\n"
	if err := os.WriteFile(refusing, []byte(script), 0o755); err != nil {
		t.Fatal(err)
	}
	e := framingEngine()
	e.FFmpeg = refusing
	segments, err := e.ClipSegments(context.Background(), path, []Span{{7, 13}}, source, 202, newCropCache())
	if err != nil {
		t.Fatalf("framing failed instead of going round: %v", err)
	}
	if len(segments) != 2 || !e.softDecode.Load() {
		t.Errorf("segments %+v, on the processor from now on: %v", segments, e.softDecode.Load())
	}
}

func TestDecoderIn(t *testing.T) {
	cases := map[string]string{
		"[vist#0:0/h264] Using auto hwaccel type videotoolbox with new default device.\n": "VideoToolbox",
		"Using auto hwaccel type vaapi with new default device.":                          "VA-API",
		"Stream #0:0: Video: h264\n":                                                      "the processor",
		"Using auto hwaccel type videotoolbox with new default device.\n" +
			"Failed setup for format videotoolbox_vld: hwaccel initialisation returned error.": "the processor",
	}
	for stderr, want := range cases {
		if got := decoderIn(stderr); got != want {
			t.Errorf("%q read as %q, want %q", stderr, got, want)
		}
	}
}

func TestHwaccelsIn(t *testing.T) {
	out := "Hardware acceleration methods:\nvideotoolbox\n\n"
	if got := hwaccelsIn(out); len(got) != 1 || got[0] != "videotoolbox" {
		t.Errorf("read %v", got)
	}
	if got := hwaccelsIn("Hardware acceleration methods:\n\n"); len(got) != 0 {
		t.Errorf("an ffmpeg with none read as %v", got)
	}
	if got := hwaccelsIn("some banner line\nvaapi\n"); len(got) != 0 {
		t.Errorf("names outside the list were read: %v", got)
	}
	if DecoderName("videotoolbox") != "VideoToolbox" || DecoderName("vaapi") != "VA-API" {
		t.Error("the names are not the ones a person knows")
	}
}
