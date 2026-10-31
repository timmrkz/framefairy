//go:build unix

package tool

import (
	"context"
	"os"
	"path/filepath"
	"testing"
	"time"

	"framefairy/internal/standin"
)

// A program whose work is called off is stopped the way its runner says,
// and one that still has not ended when the runner's wait is up is left
// behind once the wait it asked its runner for, LeaveAfter, is up. What
// it would write from then on goes nowhere, see
// TestACutWriterDropsWhatComesAfterTheCut. Each runner keeps its own stop and wait, so this test runs beside the
// others: the two tests in the engine that once swapped the package's
// stop could not.
//
// On the real disk: the stand-in is a program that is run.
func TestARunnerLeavesBehindAProgramItCouldNotStop(t *testing.T) {
	t.Parallel()
	pipe, hangs := standin.Hangs(t)
	path := filepath.Join(t.TempDir(), "stuck")
	script := "#!/bin/sh\necho hangs > '" + pipe + "'\nexec sleep 600\n"
	if err := standin.Write(path, []byte(script), 0o755); err != nil {
		t.Fatal(err)
	}
	stopped := make(chan *os.Process, 1)
	up := make(chan time.Time)
	asked := make(chan time.Duration, 1)
	r := Runner{
		Stop: func(p *os.Process) error { stopped <- p; return nil },
		Leave: func(d time.Duration) <-chan time.Time {
			asked <- d
			return up
		},
	}
	ctx, cancel := context.WithCancel(context.Background())
	ran := make(chan Result, 1)
	go func() { ran <- r.Run(ctx, "", path) }()
	<-hangs
	cancel()
	p := <-stopped
	t.Cleanup(func() { _ = p.Kill() })
	close(up)
	got := <-ran
	if !got.Left || got.Code != 1 || got.Stderr != "stuck "+LeftBehind {
		t.Errorf("a program that would not stop came back as %+v, want it left behind", got)
	}
	select {
	case d := <-asked:
		if d != LeaveAfter {
			t.Errorf("the runner was asked to wait %v, want LeaveAfter, %v", d, LeaveAfter)
		}
	default:
		t.Error("the program was left behind without the runner's wait being asked for")
	}
}

// A runner with nothing set kills a program whose work is called off.
//
// On the real disk: the stand-in is a program that is run.
func TestARunnerKillsByDefault(t *testing.T) {
	t.Parallel()
	pipe, hangs := standin.Hangs(t)
	path := filepath.Join(t.TempDir(), "stuck")
	script := "#!/bin/sh\necho hangs > '" + pipe + "'\nexec sleep 600\n"
	if err := standin.Write(path, []byte(script), 0o755); err != nil {
		t.Fatal(err)
	}
	ctx, cancel := context.WithCancel(context.Background())
	ran := make(chan Result, 1)
	go func() { ran <- Runner{}.Run(ctx, "", path) }()
	<-hangs
	cancel()
	if got := <-ran; got.Left || got.Code == 0 {
		t.Errorf("a killed program came back as %+v, want it ended by the kill", got)
	}
}
