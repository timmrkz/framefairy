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
	"testing"

	"github.com/spf13/afero"

	"framefairy/engine/runlog"
	"framefairy/engine/timed"
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

// decodingMachine is a machine handed a Mac's media engine, so what
// framing asks ffmpeg there is tested on any machine, and no test reaches
// a real system decoder, see Media.
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

// A Mac frames a clip on the processor, never on the graphics chip. On
// Tim's M2 Max framing a clip on the chip took about four times as long,
// and a check of the chip had warned on every run that it gave no frame.
// The stand-in writes down what each ffmpeg was asked and hands the work
// to the real ffmpeg.
//
// On the real disk: the stand-in ffmpeg is a program that is run.
func TestAMacFramesOnTheProcessor(t *testing.T) {
	t.Parallel()
	if runtime.GOOS == "windows" {
		t.Skip("the stand-in is a shell script")
	}
	path, source := twoCameras(t)
	real, _ := exec.LookPath("ffmpeg")
	dir := t.TempDir()
	asked := filepath.Join(dir, "asked")
	recording := filepath.Join(dir, "ffmpeg")
	script := "#!/bin/sh\necho \"$*\" >>\"" + asked + "\"\nexec \"" + real + "\" \"$@\"\n"
	if err := standin.Write(recording, []byte(script), 0o755); err != nil {
		t.Fatal(err)
	}
	var said bytes.Buffer
	e := NewEngine(runlog.New(&said, false, false), decodingMachine())
	e.FFmpeg, e.NoFaces = recording, true
	segments, err := e.ClipSegments(context.Background(), path, []timed.Span{{Start: 7, End: 13}}, source, 202, nil)
	if err != nil {
		t.Fatal(err)
	}
	if len(segments) != 1 || segments[0].CropX == nil {
		t.Errorf("segments %+v, want one with its crop", segments)
	}
	runs, _ := os.ReadFile(asked)
	if !strings.Contains(string(runs), path) {
		t.Fatalf("framing never asked ffmpeg for the video: %q", runs)
	}
	if strings.Contains(string(runs), "-hwaccel") {
		t.Errorf("framing asked the system's decoder:\n%s", runs)
	}
	if strings.Contains(said.String(), "video decoder") {
		t.Errorf("framing said something about a video decoder:\n%s", said.String())
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
