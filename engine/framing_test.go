package engine

import (
	"bytes"
	"context"
	"io"
	"os"
	"os/exec"
	"path/filepath"
	"runtime"
	"strings"
	"sync"
	"testing"
	"time"

	"github.com/spf13/afero"

	"framefairy/engine/runlog"
	"framefairy/engine/timed"
	"framefairy/internal/ffmpegtest"
	"framefairy/internal/standin"
)

// twoCameras is an episode shot on two cameras: the first for ten seconds,
// the second for ten, then the first again.
//
// On the real disk: ffmpeg makes the episode and reads it.
func twoCameras(t *testing.T) (string, SourceInfo) {
	t.Helper()
	ffmpegtest.Need(t)
	path := filepath.Join(t.TempDir(), "two.mp4")
	out, err := exec.Command("ffmpeg", "-loglevel", "error", "-y",
		"-f", "lavfi", "-i", "testsrc=s=640x360:r=25:d=10",
		"-f", "lavfi", "-i", "smptebars=s=640x360:r=25:d=10",
		"-f", "lavfi", "-i", "testsrc=s=640x360:r=25:d=10",
		"-filter_complex", "[0:v][1:v][2:v]concat=n=3:v=1:a=0",
		"-c:v", "mpeg4", "-q:v", "5", "-g", "25", path).CombinedOutput()
	if err != nil {
		t.Fatalf("making the episode: %s %s", err, out)
	}
	return path, SourceInfo{Width: 640, Height: 360, Duration: 30}
}

// framingEngine frames by what is in focus rather than by faces. These
// tests are about how a clip is framed, and looking for faces in
// every sampled frame under the race detector is most of a minute.
func framingEngine() *Engine {
	e := NewEngine(runlog.New(io.Discard, false, false), afero.NewMemMapFs())
	e.NoFaces = true
	return e
}

// A clip has one crop, whatever cameras it shows. It used to be parted at
// every camera switch, with a crop for each camera, #396.
func TestAClipHasOneCropAcrossCameraSwitches(t *testing.T) {
	t.Parallel()
	path, source := twoCameras(t)
	e := framingEngine()
	ctx := context.Background()

	// Out of the first camera and back on the second: one crop for both.
	segments, err := e.ClipSegments(ctx, path, []timed.Span{{Start: 2, End: 8}, {Start: 12, End: 18}}, source, 202, nil)
	if err != nil {
		t.Fatal(err)
	}
	if len(segments) != 2 || segments[0].CropX == nil ||
		fmtCrop(segments[0].CropX) != fmtCrop(segments[1].CropX) {
		t.Fatalf("two cameras in one clip: %+v, want two pieces with one crop", segments)
	}

	// A kept span with a switch inside it stays one piece.
	segments, err = e.ClipSegments(ctx, path, []timed.Span{{Start: 7, End: 13}}, source, 202, nil)
	if err != nil {
		t.Fatal(err)
	}
	if len(segments) != 1 || segments[0].Start != 7 || segments[0].End != 13 {
		t.Errorf("a switch inside a span: %+v, want the span as it is", segments)
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
//
// On the real disk: the stand-in ffmpeg is a program that is run.
func TestFramingDecodesOnTheProcessorWhenTheSystemWillNot(t *testing.T) {
	t.Parallel()
	if runtime.GOOS == "windows" {
		t.Skip("the stand-in is a shell script")
	}
	path, source := twoCameras(t)
	real, _ := exec.LookPath("ffmpeg")
	refusing := filepath.Join(t.TempDir(), "ffmpeg")
	script := "#!/bin/sh\ncase \"$*\" in *-hwaccel*) echo refused >&2; exit 1;; esac\nexec \"" + real + "\" \"$@\"\n"
	if err := standin.Write(refusing, []byte(script), 0o755); err != nil {
		t.Fatal(err)
	}
	e := framingEngine()
	e.FFmpeg = refusing
	segments, err := e.ClipSegments(context.Background(), path, []timed.Span{{Start: 7, End: 13}}, source, 202, nil)
	if err != nil {
		t.Fatalf("framing failed instead of going round: %v", err)
	}
	if len(segments) != 1 || segments[0].CropX == nil || !e.softDecode.Load() {
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

// A system decoder that never gives the one frame framing first asks it
// for is ended, and the framing goes on with the processor decoding. On
// CI's virtual Mac VideoToolbox hung there four times between 10 and 13
// October, and the search waited on it until the test run was killed at
// ten minutes, in TestNewAndCancelInEitherOrder and the engine's tests
// alike. Not parallel: stuckFFmpeg sets timeLimit, a package variable.
func TestFramingGoesOnWhenTheSystemDecoderHangs(t *testing.T) {
	e := NewEngine(runlog.New(io.Discard, false, false), afero.NewMemMapFs())
	e.FFmpeg = stuckFFmpeg(t, false)
	used := e.decoderUsed(context.Background(), "episode.mp4", 10.2)
	if used != "the processor" {
		t.Errorf("the decoder used is %q, want the processor", used)
	}
	if !e.softDecode.Load() {
		t.Error("the system's decoder is still used after it hung")
	}

	// A frame pulled on its own is ended the same way. One that is not
	// ends the test at go test's -timeout.
	e = NewEngine(runlog.New(io.Discard, false, false), afero.NewMemMapFs())
	e.FFmpeg = stuckFFmpeg(t, false)
	if got := e.sampleGray(context.Background(), "episode.mp4", 10, 0.1, FaceWidth, FaceHeight, 1); len(got) != 0 {
		t.Errorf("a frame pulled with a system decoder that hangs gave %d frames from a stand-in that has none", len(got))
	}
}

// An ffmpeg that stops working while it reads the frames for the crop is
// ended, and the frames are read again with the processor decoding. The
// first macOS test run with ffmpeg 7.1.5 had one hang there for six
// minutes, and nothing ended it: the search would have waited for good.
// Not parallel: stuckFFmpeg sets timeLimit, a package variable.
func TestReadingFramesThatHangsIsEnded(t *testing.T) {
	e := NewEngine(runlog.New(io.Discard, false, false), afero.NewMemMapFs())
	e.FFmpeg = stuckFFmpeg(t, false)
	if _, err := e.readSpan(context.Background(), "episode.mp4", 10, 20); err != nil {
		t.Fatal(err)
	}
	if !e.softDecode.Load() {
		t.Error("the system's decoder is still used after it hung")
	}

	// One that hangs either way gives no frames rather than no clip.
	e = NewEngine(runlog.New(io.Discard, false, false), afero.NewMemMapFs())
	e.FFmpeg = stuckFFmpeg(t, true)
	if frames, err := e.readSpan(context.Background(), "episode.mp4", 10, 20); err != nil || len(frames) != 0 {
		t.Errorf("an ffmpeg that always hangs: %d frames, %v", len(frames), err)
	}
}

// A system decoder that does not end even once it is stopped is left
// behind, and the search goes on. On CI's Mac, in run 38734171823 for
// #201, the ffmpeg of the probe was stopped after its fifteen seconds and
// went on sitting in the system's decoder, and the test waited on it for
// eight minutes, until the run was killed. Here the stop does nothing, as
// it did there. Not parallel: it stands in for leaving and stopProcess,
// and stuckFFmpeg sets timeLimit, package variables. A program stopped is
// left behind at once, rather than after leaveAfter of the clock.
func TestASystemDecoderThatWillNotEndIsLeftBehind(t *testing.T) {
	leave, stop := leaving, stopProcess
	gone := make(chan time.Time)
	close(gone)
	leaving = func(time.Duration) <-chan time.Time { return gone }
	var mu sync.Mutex
	var left []*os.Process
	stopProcess = func(p *os.Process) error {
		mu.Lock()
		defer mu.Unlock()
		left = append(left, p)
		return nil
	}
	t.Cleanup(func() {
		leaving, stopProcess = leave, stop
		mu.Lock()
		defer mu.Unlock()
		for _, p := range left {
			_ = p.Kill()
		}
	})

	var said bytes.Buffer
	e := NewEngine(runlog.New(&said, false, false), afero.NewMemMapFs())
	e.FFmpeg = stuckFFmpeg(t, false)
	used := e.decoderUsed(context.Background(), "episode.mp4", 10.2)
	if used != "the processor" || !e.softDecode.Load() {
		t.Errorf("the decoder used is %q, want the processor from now on", used)
	}
	if !strings.Contains(said.String(), "did not end") {
		t.Errorf("the log does not say the decoder was left behind: %q", said.String())
	}

	// Pulling one frame, and reading the frames of a span as ffmpeg
	// decodes it, are left the same way.
	e = NewEngine(runlog.New(io.Discard, false, false), afero.NewMemMapFs())
	e.FFmpeg = stuckFFmpeg(t, true)
	if got := e.sampleGray(context.Background(), "episode.mp4", 10, 0.1, FaceWidth, FaceHeight, 1); len(got) != 0 {
		t.Errorf("an ffmpeg left behind gave %d frames", len(got))
	}
	said.Reset()
	e = NewEngine(runlog.New(&said, false, false), afero.NewMemMapFs())
	e.FFmpeg = stuckFFmpeg(t, true)
	if frames, err := e.readSpan(context.Background(), "episode.mp4", 10, 20); err != nil || len(frames) != 0 {
		t.Errorf("an ffmpeg left behind while it reads the frames: %d frames, %v", len(frames), err)
	}
	if !strings.Contains(said.String(), "did not end") {
		t.Errorf("the log does not say ffmpeg was left behind: %q", said.String())
	}
}
