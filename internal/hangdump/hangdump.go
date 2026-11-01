// Package hangdump writes down what every program a test binary started
// is doing, just before go test's -timeout ends the binary, issue #638.
//
// A test waits on signals, so one whose signal never comes runs until the
// -timeout, which prints every goroutine of the test binary and nothing of
// the programs it started: the episode's decoder, ffmpeg and ffprobe. On
// CI's virtual Mac they are where the wait began, and the Go side's trace
// only shows that it waited, never which call inside them stopped. So a
// little before the -timeout, the binary lists its programs, with their
// state, and on a Mac samples each with the system's sample tool, which
// gives the stack of every thread, ffmpeg's and VideoToolbox's included.
//
// Only the tests use it. It changes nothing a test does, only what a test
// that hangs leaves behind.
package hangdump

import (
	"bytes"
	"flag"
	"fmt"
	"io"
	"os"
	"os/exec"
	"path/filepath"
	"runtime"
	"strconv"
	"strings"
	"time"
)

// Margin is how long before the -timeout the programs are written down.
// sample takes about a second a program and the binary starts a handful.
const Margin = 30 * time.Second

// BeforeTimeout writes Dump to stderr Margin before this test binary's
// -timeout, and does nothing when it has none. TestMain calls it after
// flag.Parse, and calls stop once the tests are done.
func BeforeTimeout() (stop func()) {
	f := flag.Lookup("test.timeout")
	if f == nil {
		return func() {}
	}
	timeout, ok := f.Value.(flag.Getter).Get().(time.Duration)
	if !ok || timeout <= Margin {
		return func() {}
	}
	t := time.AfterFunc(timeout-Margin, func() { //nolint:forbidigo // writes down the programs still running before go test's -timeout ends them, #638
		Dump(os.Stderr, os.Getpid())
	})
	return func() { t.Stop() }
}

// Dump writes every program parent started, and the programs they
// started, with their state, and on a Mac the stack of each.
func Dump(w io.Writer, parent int) {
	pids := descendants(parent)
	fmt.Fprintf(w, "\n== hangdump: the programs process %d started, %d of them, before go test's -timeout, #638\n", parent, len(pids))
	for _, pid := range pids {
		state, _ := exec.Command("ps", "-o", "pid=,ppid=,stat=,etime=,time=,command=", "-p", strconv.Itoa(pid)).Output()
		fmt.Fprintf(w, "\n== process %d: %s", pid, state)
		if runtime.GOOS == "darwin" {
			fmt.Fprintf(w, "%s", sample(pid))
		}
	}
	fmt.Fprintf(w, "\n== hangdump: end\n")
}

// descendants lists the programs parent started, and theirs, parents
// first.
func descendants(parent int) []int {
	var all []int
	next := []int{parent}
	for len(next) > 0 {
		pid := next[0]
		next = next[1:]
		out, _ := exec.Command("pgrep", "-P", strconv.Itoa(pid)).Output()
		for _, field := range strings.Fields(string(out)) {
			if child, err := strconv.Atoi(field); err == nil {
				all = append(all, child)
				next = append(next, child)
			}
		}
	}
	return all
}

// sample is the stack of every thread of pid, sampled for a second by
// macOS's sample tool, or why there is none.
//
// sample writes its report to a file, read back and removed here.
func sample(pid int) string {
	dir, err := os.MkdirTemp("", "hangdump-")
	if err != nil {
		return fmt.Sprintf("no sample: %v\n", err)
	}
	defer os.RemoveAll(dir)
	report := filepath.Join(dir, "sample.txt")
	var said bytes.Buffer
	cmd := exec.Command("sample", strconv.Itoa(pid), "1", "-mayDie", "-file", report)
	cmd.Stdout, cmd.Stderr = &said, &said
	if err := cmd.Run(); err != nil {
		return fmt.Sprintf("no sample: %v, %s\n", err, said.String())
	}
	b, err := os.ReadFile(report)
	if err != nil {
		return fmt.Sprintf("no sample: %v, %s\n", err, said.String())
	}
	return string(b)
}
