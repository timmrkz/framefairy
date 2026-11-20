//go:build unix

package engine

import (
	"bytes"
	"context"
	"math"
	"os"
	"path/filepath"
	"sync"
	"syscall"
	"testing"

	"framefairy/engine/runlog"
	"framefairy/engine/timed"
	"framefairy/internal/standin"
)

// Where the clip timeline looks is asked after every read of the sound,
// not only when the measuring writes. The sound comes from the test a
// read at a time, the writes by count never come, and the view moves to
// a part not measured between two reads, with nothing written in
// between: the measuring goes there at the next read. Before, it went
// there at the next write, half a second on, #968.
//
// On the real disk: the stand-in ffmpeg is a program that is run, its
// gate a named pipe, and ffprobe reads the episode.
func TestTheViewIsAskedAfterEveryReadOfTheSound(t *testing.T) {
	t.Parallel()
	source := sweep(t)
	dir := t.TempDir()
	_, raw := sound(0.5)
	chunk := filepath.Join(dir, "chunk.raw")
	if err := os.WriteFile(chunk, raw, 0o644); err != nil {
		t.Fatal(err)
	}
	// Each line the test writes to the gate lets the stand-in hand out
	// one read, half a second of sound. Open to write as well as to read,
	// so the stand-in never waits for a writer.
	gatePath := filepath.Join(dir, "gate")
	if err := syscall.Mkfifo(gatePath, 0o600); err != nil {
		t.Fatal(err)
	}
	gate, err := os.OpenFile(gatePath, os.O_RDWR, 0)
	if err != nil {
		t.Fatal(err)
	}
	defer gate.Close()
	pipe, started := standin.Hangs(t)
	ffmpeg := filepath.Join(dir, "ffmpeg")
	if err := standin.Write(ffmpeg, []byte("#!/bin/sh\necho started > '"+pipe+"'\n"+
		"while read go < '"+gatePath+"'; do cat '"+chunk+"'; done\n"), 0o755); err != nil {
		t.Fatal(err)
	}
	e := NewEngine(runlog.New(&bytes.Buffer{}, false, false), NewMachine(disk))
	e.FFmpeg = ffmpeg
	e.levelsReads = math.MaxInt
	var mu sync.Mutex
	view := [2]float64{50, 51}
	focus := func() (float64, float64) {
		mu.Lock()
		defer mu.Unlock()
		return view[0], view[1]
	}
	told := make(chan Levels, 16)
	ctx, cancel := context.WithCancel(context.Background())
	done := make(chan error, 1)
	go func() {
		done <- e.MeasureLevels(ctx, source, focus, func(float64) { told <- ReadLevels(disk, source) })
	}()
	defer func() {
		cancel()
		<-done
	}()
	read := func() {
		t.Helper()
		if _, err := gate.WriteString("go\n"); err != nil {
			t.Fatal(err)
		}
	}

	<-started
	read()
	if first := <-told; len(first.Parts) != 1 || first.Parts[0][0] != int(50/timed.FrameSeconds) {
		t.Fatalf("first written %v, want the first read from 50 s", partSeconds(first.Parts))
	}
	mu.Lock()
	view = [2]float64{20, 21}
	mu.Unlock()
	// One more read, and the measuring leaves 50 s for 20 s: a second
	// ffmpeg starts, and its first read is written at once.
	read()
	<-started
	read()
	moved := <-told
	if !covers(moved.Parts, 20, 20.4) {
		t.Errorf("after the view went to 20 s the parts were %v, want it measured there", partSeconds(moved.Parts))
	}
}
