//go:build unix

package engine

import (
	"bytes"
	"context"
	"errors"
	"os"
	"path/filepath"
	"testing"
	"time"

	"github.com/spf13/afero"

	"framefairy/engine/runlog"
	"framefairy/engine/tool"
	"framefairy/internal/standin"
)

// The loudness is read from an ffmpeg that, once the reading is called
// off, may not end: ffmpeg on CI's virtual Mac went on for eight minutes
// after it was killed. The reading is then let go of once LeaveAfter is
// up, and quit, removing a video and the hearing lane, which wait for it,
// go on. Before, it waited on the program with no limit.
//
// On the real disk: the stand-in is a program that is run.
func TestTheLoudnessLetsGoOfAnFFmpegThatWillNotStop(t *testing.T) {
	t.Parallel()
	pipe, hangs := standin.Hangs(t)
	path := filepath.Join(t.TempDir(), "ffmpeg")
	script := "#!/bin/sh\necho hangs > '" + pipe + "'\nexec sleep 600\n"
	if err := standin.Write(path, []byte(script), 0o755); err != nil {
		t.Fatal(err)
	}
	stopped := make(chan *os.Process, 1)
	up := make(chan time.Time)
	asked := make(chan time.Duration, 1)
	e := NewEngine(runlog.New(&bytes.Buffer{}, false, false), NewMachine(afero.NewMemMapFs()))
	e.FFmpeg = path
	e.Runner = tool.Runner{
		Stop: func(p *os.Process) error { stopped <- p; return nil },
		Leave: func(d time.Duration) <-chan time.Time {
			asked <- d
			return up
		},
	}
	ctx, cancel := context.WithCancel(context.Background())
	ran := make(chan error, 1)
	go func() { _, err := e.levelsAt(ctx, "/nowhere.mp4", 0, 100, 0); ran <- err }()
	<-hangs
	cancel()
	p := <-stopped
	t.Cleanup(func() { _ = p.Kill() })
	if d := <-asked; d != tool.LeaveAfter {
		t.Errorf("waited %v for the stopped ffmpeg, want LeaveAfter, %v", d, tool.LeaveAfter)
	}
	close(up)
	if err := <-ran; !errors.Is(err, context.Canceled) {
		t.Errorf("the loudness came back with %v, want it called off", err)
	}
}
