//go:build unix

package engine

import (
	"bytes"
	"context"
	"os"
	"path/filepath"
	"slices"
	"strconv"
	"strings"
	"syscall"
	"testing"

	"framefairy/engine/runlog"
	"framefairy/internal/ffmpegtest"
	"framefairy/internal/standin"
)

// The frames of one measuring are drawn side by side. A clip with more
// words than one frame holds needs two, and its captions waited for both,
// one after the other. Here neither ffmpeg draws until both have started,
// so frames drawn one after the other wait until go test's timeout.
//
// On the real disk: ffmpeg measures the captions, through a script that
// says it started and is held through a named pipe there.
func TestTheFramesOfOneMeasuringAreDrawnSideBySide(t *testing.T) {
	t.Parallel()
	ctx := context.Background()
	e := NewEngine(runlog.New(&bytes.Buffer{}, false, false), NewMachine(disk))
	if err := e.Preflight(ctx); err != nil {
		ffmpegtest.Unusable(t, "no usable ffmpeg here: %v", err)
	}
	pipe, started := standin.Hangs(t)
	dir := t.TempDir()
	answer := filepath.Join(dir, "answer")
	if err := syscall.Mkfifo(answer, 0o600); err != nil {
		t.Fatal(err)
	}
	ffmpeg := filepath.Join(dir, "ffmpeg")
	// Only a run that draws a frame is held, not the questions about
	// ffmpeg's filters.
	script := "#!/bin/sh\ncase \"$*\" in\n*rawvideo*) echo started > '" + pipe + "'\n  read go < '" + answer + "' ;;\nesac\nexec '" + e.FFmpeg + "' \"$@\"\n"
	if err := standin.Write(ffmpeg, []byte(script), 0o755); err != nil {
		t.Fatal(err)
	}
	e.FFmpeg = ffmpeg

	s := new(Fonts).Style(map[string]any{"font": new(Fonts).Caption()[0].Name, "size": 96.0, "highlight": 0.0})
	// One block more than one frame holds.
	var blocks []string
	for height := 0; height <= measureFrameMost; {
		block := `{\alpha&H00&}Wort ` + strconv.Itoa(len(blocks))
		height += bandSlot(block, int(s.Size))
		blocks = append(blocks, block)
	}

	measured := make(chan []*inkBox, 1)
	go func() {
		boxes, ok := e.measureMany(ctx, blocks, s, 1080, int(s.Size))
		if !ok {
			boxes = nil
		}
		measured <- boxes
	}()
	<-started
	<-started
	held, err := os.OpenFile(answer, os.O_WRONLY, 0)
	if err != nil {
		t.Fatal(err)
	}
	if _, err := held.WriteString("go\ngo\n"); err != nil {
		t.Fatal(err)
	}
	boxes := <-measured
	if err := held.Close(); err != nil {
		t.Fatal(err)
	}
	if len(boxes) != len(blocks) || slices.Contains(boxes, nil) {
		t.Errorf("measured %d of %d blocks, or some found no ink", len(boxes), len(blocks))
	}
}

// A frame of measuring that fails fails the whole measuring, and the
// frames still waiting for their turn are not drawn: each would make a
// folder, install the face and start an ffmpeg for nothing. Here every
// frame fails, so only the frames that started at once run.
//
// On the real disk: ffmpeg is a script that counts its runs and fails.
func TestAFailedFrameOfMeasuringStopsTheFramesWaiting(t *testing.T) {
	t.Parallel()
	ctx := context.Background()
	e := NewEngine(runlog.New(&bytes.Buffer{}, false, false), NewMachine(disk))
	if err := e.Preflight(ctx); err != nil {
		ffmpegtest.Unusable(t, "no usable ffmpeg here: %v", err)
	}
	dir := t.TempDir()
	asked := filepath.Join(dir, "asked")
	ffmpeg := filepath.Join(dir, "ffmpeg")
	script := "#!/bin/sh\ncase \"$*\" in\n*rawvideo*) echo drawn >> '" + asked + "'\n  exit 1 ;;\nesac\nexec '" + e.FFmpeg + "' \"$@\"\n"
	if err := standin.Write(ffmpeg, []byte(script), 0o755); err != nil {
		t.Fatal(err)
	}
	e.FFmpeg = ffmpeg

	s := new(Fonts).Style(map[string]any{"font": new(Fonts).Caption()[0].Name, "size": 96.0, "highlight": 0.0})
	// More frames than are drawn at once.
	var blocks []string
	for height := 0; height <= (measureAtOnce+1)*measureFrameMost; {
		block := `{\alpha&H00&}Wort ` + strconv.Itoa(len(blocks))
		height += bandSlot(block, int(s.Size))
		blocks = append(blocks, block)
	}
	if _, ok := e.measureMany(ctx, blocks, s, 1080, int(s.Size)); ok {
		t.Fatal("a measuring whose every frame failed was taken as measured")
	}
	data, err := os.ReadFile(asked)
	if err != nil {
		t.Fatal(err)
	}
	if got := strings.Count(string(data), "\n"); got > measureAtOnce {
		t.Errorf("%d frames were drawn after the first failed, at most %d start at once", got, measureAtOnce)
	}
}
