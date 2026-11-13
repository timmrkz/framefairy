//go:build unix

package engine

import (
	"context"
	"io"
	"os"
	"path/filepath"
	"syscall"
	"testing"

	"framefairy/engine/runlog"
	"framefairy/internal/standin"
)

// A video uneven in its first stretch, the way a phone records, is
// uneven, and the reads of the other three stretches are called off
// rather than read to their end. Here they never end by themselves: each
// says it hangs and waits, and only once all three wait does the first
// answer. Without the call, uneven waits on them until go test's timeout.
//
// On the real disk: the stand-in ffprobe is a program that is run, and it
// is held and let go through named pipes there.
func TestTheFirstUnevenStretchCallsOffTheOtherReads(t *testing.T) {
	t.Parallel()
	pipe, hangs := standin.Hangs(t)
	dir := t.TempDir()
	answer := filepath.Join(dir, "answer")
	if err := syscall.Mkfifo(answer, 0o600); err != nil {
		t.Fatal(err)
	}
	ffprobe := filepath.Join(dir, "ffprobe")
	// A frame at 0.02 s is off the grid of 25 frames a second.
	script := `#!/bin/sh
case "$*" in
*"-read_intervals 0.000%"*) read go < '` + answer + `'; printf '0.000000,48,K_\n0.020000,96,__\n' ;;
*-read_intervals*) echo hangs > '` + pipe + `'; exec sleep 600 ;;
esac
`
	if err := standin.Write(ffprobe, []byte(script), 0o755); err != nil {
		t.Fatal(err)
	}
	e := &Engine{Machine: &Machine{}, Log: runlog.New(io.Discard, false, false), FFprobe: ffprobe}
	info := SourceInfo{FPSNum: 25, FPSDen: 1, Duration: 60}

	uneven := make(chan bool, 1)
	go func() { uneven <- e.uneven(context.Background(), filepath.Join(dir, "phone.mov"), info) }()
	for range 3 {
		<-hangs
	}
	held, err := os.OpenFile(answer, os.O_WRONLY, 0)
	if err != nil {
		t.Fatal(err)
	}
	if _, err := held.WriteString("go\n"); err != nil {
		t.Fatal(err)
	}
	if err := held.Close(); err != nil {
		t.Fatal(err)
	}
	if !<-uneven {
		t.Fatal("a video uneven in its first stretch was taken as even")
	}
}
