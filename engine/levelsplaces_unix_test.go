//go:build unix

package engine

import (
	"bytes"
	"context"
	"math"
	"os"
	"path/filepath"
	"testing"

	"framefairy/engine/runlog"
	"framefairy/engine/timed"
	"framefairy/internal/ffmpegtest"
	"framefairy/internal/fixtures"
	"framefairy/internal/standin"
)

// A video is measured by several ffmpeg at once, each from a place of its
// own, and comes out the same as measured by one: every frame measured,
// in one part, and each frame written to the disk once, #968.
//
// On the real disk: ffmpeg is run, through a stand-in that notes where
// each starts, and measures the episode.
func TestAVideoIsMeasuredInPlacesSideBySide(t *testing.T) {
	t.Parallel()
	ffmpegtest.Need(t)
	dir := t.TempDir()
	source := fixtures.Copy(t, "grey-300s.mp4", filepath.Join(dir, "episode.mp4"))
	starts := filepath.Join(dir, "starts")
	ffmpeg := filepath.Join(dir, "ffmpeg")
	if err := standin.Write(ffmpeg, []byte("#!/bin/sh\necho \"$*\" >> '"+starts+"'\nexec ffmpeg \"$@\"\n"), 0o755); err != nil {
		t.Fatal(err)
	}
	w := &writes{Fs: disk, under: WorkDir(source), bytes: map[string]int{}, times: map[string]int{}}
	e := NewEngine(runlog.New(&bytes.Buffer{}, false, false), NewMachine(w))
	e.FFmpeg = ffmpeg
	e.levelsMeasurers = 3
	e.levelsReads = 4
	if err := e.MeasureLevels(context.Background(), source, nil, nil); err != nil {
		t.Fatal(err)
	}
	side := ReadLevels(disk, source)
	if !side.Done || len(side.Parts) != 1 {
		t.Fatalf("done %v, parts %v", side.Done, partSeconds(side.Parts))
	}
	if got, want := w.bytes["levels.frames"], 4*len(side.Frames); got != want {
		t.Errorf("%d bytes written for %d frames of 4 bytes", got, len(side.Frames))
	}
	// Where each ffmpeg starts hangs on which measurer claims first, so it
	// is held by TestMeasurersShareTheLongestWorkLeft on a table, not
	// here. It is logged for a failure to show.
	if raw, err := os.ReadFile(starts); err == nil {
		t.Logf("ffmpeg started with %s", raw)
	}

	// The same video measured by one ffmpeg.
	again := fixtures.Copy(t, "grey-300s.mp4", filepath.Join(dir, "again.mp4"))
	e.levelsMeasurers = 1
	if err := e.MeasureLevels(context.Background(), again, nil, nil); err != nil {
		t.Fatal(err)
	}
	one := ReadLevels(disk, again)
	if len(one.Frames) != len(side.Frames) {
		t.Fatalf("%d frames by one, %d side by side", len(one.Frames), len(side.Frames))
	}
	for i := range one.Frames {
		if d := math.Abs(float64(one.Frames[i] - side.Frames[i])); d > 1 {
			t.Fatalf("frame %d (%.2f s): %.2f dB by one, %.2f side by side", i, float64(i)*timed.FrameSeconds, one.Frames[i], side.Frames[i])
		}
	}
}
