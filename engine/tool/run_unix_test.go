//go:build unix

package tool

import (
	"bufio"
	"bytes"
	"context"
	"errors"
	"io"
	"os"
	"os/exec"
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

// A program read through a pipe that was stopped and does not end keeps
// its pipe open, and a read on it would wait for good. Once LeaveAfter
// is up the pipe is closed, which ends the read, and Finish says the
// program was left behind.
//
// On the real disk: the stand-in is a program that is run.
func TestAProgramReadThroughAPipeIsLeftBehindWhenItWillNotStop(t *testing.T) {
	t.Parallel()
	pipe, hangs := standin.Hangs(t)
	path := filepath.Join(t.TempDir(), "stuck")
	script := "#!/bin/sh\necho some sound\necho hangs > '" + pipe + "'\nexec sleep 600\n"
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
	cmd := r.Command(ctx, path)
	var said bytes.Buffer
	saying := NewCutWriter(&said)
	cmd.Stderr = saying
	reading, err := r.StartReading(ctx, cmd, saying)
	if err != nil {
		t.Fatal(err)
	}
	// What it wrote before it hung is read first: a pipe closed under a
	// read drops what is still in it unread.
	out := bufio.NewReader(reading.Stdout)
	if line, err := out.ReadString('\n'); line != "some sound\n" || err != nil {
		t.Fatalf("read %q, %v, want what it wrote", line, err)
	}
	read := make(chan error, 1)
	go func() {
		_, err := io.ReadAll(out)
		read <- err
	}()
	<-hangs
	cancel()
	p := <-stopped
	t.Cleanup(func() { _ = p.Kill() })
	if d := <-asked; d != LeaveAfter {
		t.Errorf("the runner was asked to wait %v, want LeaveAfter, %v", d, LeaveAfter)
	}
	close(up)
	if err := <-read; err == nil {
		t.Error("the read of a program left behind ended as if it had ended itself")
	}
	if left, err := reading.Finish(); !left || err != nil {
		t.Errorf("Finish said left %v, %v, want it left behind", left, err)
	}
}

// A program read through a pipe to its end is waited for as Run does,
// with what it said and how it ended.
//
// On the real disk: the stand-in is a program that is run.
func TestAProgramReadThroughAPipeToItsEndIsWaitedFor(t *testing.T) {
	t.Parallel()
	path := filepath.Join(t.TempDir(), "speaks")
	if err := standin.Write(path, []byte("#!/bin/sh\necho the sound\necho a word >&2\nexit 3\n"), 0o755); err != nil {
		t.Fatal(err)
	}
	var r Runner
	cmd := r.Command(context.Background(), path)
	var said bytes.Buffer
	saying := NewCutWriter(&said)
	cmd.Stderr = saying
	reading, err := r.StartReading(context.Background(), cmd, saying)
	if err != nil {
		t.Fatal(err)
	}
	got, _ := io.ReadAll(reading.Stdout)
	left, err := reading.Finish()
	var exit *exec.ExitError
	if left || !errors.As(err, &exit) || exit.ExitCode() != 3 {
		t.Errorf("Finish said left %v, %v, want the program's own end, code 3", left, err)
	}
	if string(got) != "the sound\n" || said.String() != "a word\n" {
		t.Errorf("read %q and %q, want what it wrote", got, said.String())
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
