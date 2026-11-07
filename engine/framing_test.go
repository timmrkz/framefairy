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
	"framefairy/engine/tool"
	"framefairy/internal/ffmpegtest"
	"framefairy/internal/fixtures"
	"framefairy/internal/standin"
)

// twoCameras is an episode shot on two cameras: the first for ten seconds,
// the second for ten, then the first again.
//
// On the real disk: ffmpeg reads the episode.
func twoCameras(t *testing.T) (string, SourceInfo) {
	t.Helper()
	ffmpegtest.Need(t)
	return fixtures.Path(t, "two-cameras.mp4"), SourceInfo{Width: 640, Height: 360, Duration: 30}
}

// decodingMachine is a machine handed a Mac's media engine, whose system
// decoder a stand-in ffmpeg plays, so what framing does when the system's
// decoder fails is tested on any machine, and no test waits on a real
// one, see Media.
func decodingMachine() *Machine {
	m := NewMachine(afero.NewMemMapFs())
	m.useMedia(SystemMedia("darwin"))
	return m
}

// framingEngine frames by what is in focus rather than by faces. These
// tests are about how a clip is framed, and looking for faces in
// every sampled frame under the race detector is most of a minute.
func framingEngine() *Engine {
	e := NewEngine(runlog.New(io.Discard, false, false), NewMachine(afero.NewMemMapFs()))
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
// on: the same work is done on the processor, and so is everything after
// with that file. Another file still goes to the system's decoder, so one
// odd file does not take the graphics chip from every video until the app
// quits.
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
	e.useMedia(SystemMedia("darwin"))
	e.FFmpeg = refusing
	segments, err := e.ClipSegments(context.Background(), path, []timed.Span{{Start: 7, End: 13}}, source, 202, nil)
	if err != nil {
		t.Fatalf("framing failed instead of going round: %v", err)
	}
	if len(segments) != 1 || segments[0].CropX == nil {
		t.Errorf("segments %+v, want one with its crop", segments)
	}
	if e.decodeFlags(path) != nil {
		t.Error("the file the system's decoder would not take is still handed to it")
	}
	if e.softDecode.Load() || e.decodeFlags("another.mp4") == nil {
		t.Error("one file the system's decoder would not take took it from every other file")
	}
}

// What one job learned about the system's decoder, the next one knows.
// The app makes an engine for every job, and each one used to try the
// system's decoder again, so a Mac whose decoder fails paid for it on
// every search (#644). Two engines of one machine: once the decoder
// refused the first, the second never asks it.
//
// On the real disk: the stand-in ffmpeg is a program that is run.
func TestTheNextJobDecodesOnTheProcessorOnceTheSystemWouldNot(t *testing.T) {
	t.Parallel()
	if runtime.GOOS == "windows" {
		t.Skip("the stand-in is a shell script")
	}
	path, source := twoCameras(t)
	real, _ := exec.LookPath("ffmpeg")
	dir := t.TempDir()
	asked := filepath.Join(dir, "asked")
	refusing := filepath.Join(dir, "ffmpeg")
	script := "#!/bin/sh\ncase \"$*\" in *-hwaccel*) echo asked >>\"" + asked +
		"\"; echo refused >&2; exit 1;; esac\nexec \"" + real + "\" \"$@\"\n"
	if err := standin.Write(refusing, []byte(script), 0o755); err != nil {
		t.Fatal(err)
	}
	machine := decodingMachine()
	job := func() {
		e := NewEngine(runlog.New(io.Discard, false, false), machine)
		e.FFmpeg, e.NoFaces = refusing, true
		if _, err := e.ClipSegments(context.Background(), path, []timed.Span{{Start: 7, End: 13}},
			source, 202, nil); err != nil {
			t.Fatalf("framing failed: %v", err)
		}
	}
	times := func() int {
		said, _ := os.ReadFile(asked)
		return strings.Count(string(said), "asked")
	}

	job()
	if times() == 0 {
		t.Fatal("the first job never asked the system's decoder")
	}
	first := times()
	job()
	if more := times() - first; more != 0 {
		t.Errorf("the next job asked the system's decoder %d times more after it refused the first", more)
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

// A system decoder that gives its frame at once is kept, and so is every
// decode after it. The check once read its time limit after ending it,
// and took every decoder that worked for one that hung: on Tim's Mac each
// first framing of a run said the chip gave no frame in 15 s, a third of
// a second after it began, and framed on the processor from then on.
//
// On the real disk: the stand-in ffmpeg is a program that is run.
func TestASystemDecoderThatWorksIsKept(t *testing.T) {
	t.Parallel()
	if runtime.GOOS == "windows" {
		t.Skip("the stand-in is a shell script")
	}
	working := filepath.Join(t.TempDir(), "ffmpeg")
	script := "#!/bin/sh\necho 'Using auto hwaccel type videotoolbox with new default device.' >&2\n"
	if err := standin.Write(working, []byte(script), 0o755); err != nil {
		t.Fatal(err)
	}
	var said bytes.Buffer
	machine := decodingMachine()
	// A limit nothing ends, so the test does not hang on the clock's.
	machine.timeLimit = func(ctx context.Context, _ time.Duration) (context.Context, context.CancelFunc) {
		return context.WithCancel(ctx)
	}
	e := NewEngine(runlog.New(&said, false, false), machine)
	e.FFmpeg = working
	if used := e.decoderUsed(context.Background(), "episode.mp4", 10.2); used != "VideoToolbox" {
		t.Errorf("the decoder used is %q, want VideoToolbox", used)
	}
	if e.softDecode.Load() {
		t.Error("a system decoder that gave its frame is not used from now on")
	}
	if strings.Contains(said.String(), "gave no frame") {
		t.Errorf("the log says a decoder that worked gave no frame: %q", said.String())
	}
	next := NewEngine(runlog.New(io.Discard, false, false), machine)
	if next.decodeFlags("episode.mp4") == nil {
		t.Error("the next job does not decode on the system's decoder")
	}
}

// A clip framed with a system decoder that works is framed on it from the
// first check to the last frame read, and its crop is placed. The stand-in
// says it decodes through VideoToolbox, the way ffmpeg does on a Mac, and
// hands the work to the real ffmpeg on the processor.
//
// On the real disk: the stand-in ffmpeg is a program that is run.
func TestAClipIsFramedOnASystemDecoderThatWorks(t *testing.T) {
	t.Parallel()
	if runtime.GOOS == "windows" {
		t.Skip("the stand-in is a shell script")
	}
	path, source := twoCameras(t)
	real, _ := exec.LookPath("ffmpeg")
	working := filepath.Join(t.TempDir(), "ffmpeg")
	// It takes -hwaccel and its value off before the real ffmpeg runs,
	// so no test reaches a system decoder, see Media.
	script := "#!/bin/sh\nskip= hw=\nfor a; do shift\n" +
		"  if [ -n \"$skip\" ]; then skip=; continue; fi\n" +
		"  if [ \"$a\" = -hwaccel ]; then skip=1 hw=1; continue; fi\n" +
		"  set -- \"$@\" \"$a\"\ndone\n" +
		"[ -n \"$hw\" ] && echo 'Using auto hwaccel type videotoolbox with new default device.' >&2\n" +
		"exec \"" + real + "\" \"$@\"\n"
	if err := standin.Write(working, []byte(script), 0o755); err != nil {
		t.Fatal(err)
	}
	var said bytes.Buffer
	e := NewEngine(runlog.New(&said, false, false), decodingMachine())
	e.FFmpeg, e.NoFaces = working, true
	segments, err := e.ClipSegments(context.Background(), path, []timed.Span{{Start: 7, End: 13}}, source, 202, nil)
	if err != nil {
		t.Fatal(err)
	}
	if len(segments) != 1 || segments[0].CropX == nil {
		t.Errorf("segments %+v, want one with its crop", segments)
	}
	if e.softDecode.Load() {
		t.Errorf("framing left the system's decoder that worked:\n%s", said.String())
	}
	if !strings.Contains(said.String(), "framing decodes video on VideoToolbox") {
		t.Errorf("the log does not say framing decoded on VideoToolbox:\n%s", said.String())
	}
}

// A system decoder that never gives the one frame framing first asks it
// for is ended, and the framing goes on with the processor decoding. On
// CI's virtual Mac VideoToolbox hung there four times between 10 and 13
// October, and the search waited on it until the test run was killed at
// ten minutes, in TestNewAndCancelInEitherOrder and the engine's tests
// alike.
func TestFramingGoesOnWhenTheSystemDecoderHangs(t *testing.T) {
	t.Parallel()
	e := NewEngine(runlog.New(io.Discard, false, false), decodingMachine())
	e.FFmpeg = stuckFFmpeg(t, e.Machine, false)
	used := e.decoderUsed(context.Background(), "episode.mp4", 10.2)
	if used != "the processor" {
		t.Errorf("the decoder used is %q, want the processor", used)
	}
	if !e.softDecode.Load() {
		t.Error("the system's decoder is still used after it hung")
	}

	// A frame pulled on its own is ended the same way. One that is not
	// ends the test at go test's -timeout.
	e = NewEngine(runlog.New(io.Discard, false, false), decodingMachine())
	e.FFmpeg = stuckFFmpeg(t, e.Machine, false)
	if got := e.sampleGray(context.Background(), "episode.mp4", 10, 0.1, FaceWidth, FaceHeight, 1); len(got) != 0 {
		t.Errorf("a frame pulled with a system decoder that hangs gave %d frames from a stand-in that has none", len(got))
	}
}

// An ffmpeg that stops working while it reads the frames for the crop is
// ended, and the frames are read again with the processor decoding. The
// first macOS test run with ffmpeg 7.1.5 had one hang there for six
// minutes, and nothing ended it: the search would have waited for good.
func TestReadingFramesThatHangsIsEnded(t *testing.T) {
	t.Parallel()
	e := NewEngine(runlog.New(io.Discard, false, false), decodingMachine())
	e.FFmpeg = stuckFFmpeg(t, e.Machine, false)
	if _, err := e.readSpan(context.Background(), "episode.mp4", 10, 20); err != nil {
		t.Fatal(err)
	}
	if !e.softDecode.Load() {
		t.Error("the system's decoder is still used after it hung")
	}

	// One that hangs either way gives no frames rather than no clip.
	e = NewEngine(runlog.New(io.Discard, false, false), decodingMachine())
	e.FFmpeg = stuckFFmpeg(t, e.Machine, true)
	if frames, err := e.readSpan(context.Background(), "episode.mp4", 10, 20); err != nil || len(frames) != 0 {
		t.Errorf("an ffmpeg that always hangs: %d frames, %v", len(frames), err)
	}
}

// A system decoder that does not end even once it is stopped is left
// behind, and the search goes on. On CI's Mac, in run 38734171823 for
// #201, the ffmpeg of the probe was stopped after its fifteen seconds and
// went on sitting in the system's decoder, and the test waited on it for
// eight minutes, until the run was killed. Here the stop does nothing, as
// it did there. The engines' runner stops nothing and leaves a program it
// stopped behind at once, rather than after tool.LeaveAfter of the clock.
func TestASystemDecoderThatWillNotEndIsLeftBehind(t *testing.T) {
	t.Parallel()
	gone := make(chan time.Time)
	close(gone)
	var mu sync.Mutex
	var left []*os.Process
	stuck := tool.Runner{
		Stop: func(p *os.Process) error {
			mu.Lock()
			defer mu.Unlock()
			left = append(left, p)
			return nil
		},
		Leave: func(time.Duration) <-chan time.Time { return gone },
	}
	t.Cleanup(func() {
		mu.Lock()
		defer mu.Unlock()
		for _, p := range left {
			_ = p.Kill()
		}
	})

	var said bytes.Buffer
	e := NewEngine(runlog.New(&said, false, false), decodingMachine())
	e.FFmpeg, e.Runner = stuckFFmpeg(t, e.Machine, false), stuck
	used := e.decoderUsed(context.Background(), "episode.mp4", 10.2)
	if used != "the processor" || !e.softDecode.Load() {
		t.Errorf("the decoder used is %q, want the processor from now on", used)
	}
	if !strings.Contains(said.String(), "did not end") {
		t.Errorf("the log does not say the decoder was left behind: %q", said.String())
	}

	// Pulling one frame, and reading the frames of a span as ffmpeg
	// decodes it, are left the same way.
	e = NewEngine(runlog.New(io.Discard, false, false), decodingMachine())
	e.FFmpeg, e.Runner = stuckFFmpeg(t, e.Machine, true), stuck
	if got := e.sampleGray(context.Background(), "episode.mp4", 10, 0.1, FaceWidth, FaceHeight, 1); len(got) != 0 {
		t.Errorf("an ffmpeg left behind gave %d frames", len(got))
	}
	said.Reset()
	e = NewEngine(runlog.New(&said, false, false), decodingMachine())
	e.FFmpeg, e.Runner = stuckFFmpeg(t, e.Machine, true), stuck
	if frames, err := e.readSpan(context.Background(), "episode.mp4", 10, 20); err != nil || len(frames) != 0 {
		t.Errorf("an ffmpeg left behind while it reads the frames: %d frames, %v", len(frames), err)
	}
	if !strings.Contains(said.String(), "did not end") {
		t.Errorf("the log does not say ffmpeg was left behind: %q", said.String())
	}
}
