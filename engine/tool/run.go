package tool

import (
	"bytes"
	"context"
	"errors"
	"io"
	"os"
	"os/exec"
	"path/filepath"
	"sync"
	"time"
)

// Runner runs programs as argument lists, never through a shell, and
// stops them once their work is called off. How it stops one is its two
// fields, each the default when left empty, so a test sets its own on
// its own runner and no other test sees it, the way an http.Client keeps
// its Transport.
type Runner struct {
	// Stop stops a program whose work is called off. Empty, it kills it.
	// A test stands in a stop that does not end the program.
	Stop func(*os.Process) error
	// Leave is the wait for a program that was asked to stop to end,
	// LeaveAfter for ffmpeg and a grace of its own for llama-server.
	// Empty, it is time.After. A test hands in a wait that is up at once.
	Leave func(time.Duration) <-chan time.Time
}

// LeaveAfter is how long a program that was stopped has to end before it
// is left behind. A killed program ends at once, unless it is inside the
// system and the system will not let it go: ffmpeg in VideoToolbox on CI's
// virtual Mac went on for eight minutes after it was killed, and the
// search that waited for it waited for good.
const LeaveAfter = 5 * time.Second

// LeftBehind is what is said of a program that was left behind.
const LeftBehind = "did not end when it was stopped, so it was left behind"

// Result is how a program run by Run ended and what it wrote.
type Result struct {
	Code   int
	Stdout string
	Stderr string
	// Left is set for a program that did not end when it was stopped,
	// see LeaveAfter.
	Left bool
}

// Leaving is the wait of d for a program that was asked to stop, see
// Leave.
func (r Runner) Leaving(d time.Duration) <-chan time.Time {
	if r.Leave != nil {
		return r.Leave(d)
	}
	return time.After(d)
}

// Run executes a command as an argument list, never through a shell. A
// missing binary is a normal outcome here, so it comes back as exit code
// 127 rather than as an error.
func (r Runner) Run(ctx context.Context, cwd string, name string, args ...string) Result {
	if name == "" {
		return Result{Code: 127, Stderr: "no tool that may run, see Preflight"}
	}
	cmd := r.Command(ctx, name, args...)
	cmd.Dir = cwd
	var stdout, stderr bytes.Buffer
	out, said := &CutWriter{w: &stdout}, &CutWriter{w: &stderr}
	cmd.Stdout, cmd.Stderr = out, said
	if err := cmd.Start(); err != nil {
		return Result{Code: 127, Stderr: err.Error()}
	}
	left, err := r.Finish(ctx, cmd, out, said)
	if left {
		return Result{Code: 1, Stderr: filepath.Base(name) + " " + LeftBehind, Left: true}
	}
	code := 0
	if err != nil {
		var exit *exec.ExitError
		if errors.As(err, &exit) {
			code = exit.ExitCode()
			if code < 0 {
				code = 1
			}
		} else {
			return Result{Code: 127, Stderr: err.Error()}
		}
	}
	return Result{Code: code, Stdout: stdout.String(), Stderr: stderr.String()}
}

// Bytes is Run for a program whose output is bytes, a picture or sound,
// with its exit code.
func (r Runner) Bytes(ctx context.Context, cwd string, name string, args ...string) ([]byte, int) {
	cmd := r.Command(ctx, name, args...)
	cmd.Dir = cwd
	var stdout bytes.Buffer
	out := &CutWriter{w: &stdout}
	cmd.Stdout = out
	if err := cmd.Start(); err != nil {
		return nil, 127
	}
	left, err := r.Finish(ctx, cmd, out)
	if left {
		return nil, 1
	}
	if err != nil {
		var exit *exec.ExitError
		if errors.As(err, &exit) {
			return stdout.Bytes(), max(exit.ExitCode(), 1)
		}
		return nil, 127
	}
	return stdout.Bytes(), 0
}

// Command is exec.CommandContext that stops the program with Stop once
// ctx is done. Run, Bytes and whoever watches ffmpeg as it goes start
// their programs with it and wait for them with Finish.
func (r Runner) Command(ctx context.Context, name string, args ...string) *exec.Cmd {
	cmd := exec.CommandContext(ctx, name, args...)
	stop := r.Stop
	if stop == nil {
		stop = func(p *os.Process) error { return p.Kill() }
	}
	cmd.Cancel = func() error { return stop(cmd.Process) }
	return cmd
}

// Finish waits for a command that was started to end. Once ctx is done
// the command is stopped, and one that has still not ended LeaveAfter
// later is left behind and left says so: what it writes from then on goes
// to none of outs, which the caller may read at once.
func (r Runner) Finish(ctx context.Context, cmd *exec.Cmd, outs ...*CutWriter) (left bool, err error) {
	finished := make(chan error, 1)
	go func() { finished <- cmd.Wait() }()
	select {
	case err := <-finished:
		return false, err
	case <-ctx.Done():
	}
	select {
	case err := <-finished:
		return false, err
	case <-r.Leaving(LeaveAfter):
	}
	for _, out := range outs {
		out.Cut()
	}
	return true, nil
}

// CutWriter passes what is written on to w until it is cut, and drops it
// after, so a program left behind writes into nothing anyone reads.
type CutWriter struct {
	mu sync.Mutex
	w  io.Writer
}

// NewCutWriter is a CutWriter that writes to w until it is cut.
func NewCutWriter(w io.Writer) *CutWriter { return &CutWriter{w: w} }

func (c *CutWriter) Write(p []byte) (int, error) {
	c.mu.Lock()
	defer c.mu.Unlock()
	if c.w == nil {
		return len(p), nil
	}
	return c.w.Write(p)
}

// Cut stops what is written from reaching w.
func (c *CutWriter) Cut() {
	c.mu.Lock()
	defer c.mu.Unlock()
	c.w = nil
}
