// Package hangdump writes down what every program a test binary started
// is doing, just before go test's -timeout ends the binary.
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
	"errors"
	"flag"
	"fmt"
	"io"
	"os"
	"os/exec"
	"path/filepath"
	"runtime"
	"strconv"
	"strings"
	"sync"
	"time"
)

// Margin is how long before the -timeout the programs are written down.
// sample takes a second or two a program, and every program is sampled
// at once.
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
	// Under go test -json, test2json files a line under the last test
	// that spoke, which has mostly passed by then, and gotestsum shows
	// no output of a test that passed. So each part of the dump says
	// first that it belongs to no test.
	framed := false
	if v := flag.Lookup("test.v"); v != nil {
		framed = v.Value.String() == "test2json"
	}
	t := time.AfterFunc(timeout-Margin, func() { //nolint:forbidigo // writes down the programs still running before go test's -timeout ends them
		Dump(os.Stderr, os.Getpid(), framed)
	})
	return func() { t.Stop() }
}

// noTest is the line the testing package writes under go test -json for
// output that belongs to no test, which test2json files under the
// package. ^V marks a line of the testing package's own.
const noTest = "\x16=== NAME  \n"

// Dump writes every program parent started, and the programs they
// started, with their state, and on a Mac the stack of each. Each part
// goes out in one write, so no line of a test lands inside it, and with
// framed each part starts with the line that says it belongs to no test,
// for go test -json.
func Dump(w io.Writer, parent int, framed bool) {
	var mu sync.Mutex
	part := func(format string, a ...any) {
		var b bytes.Buffer
		if framed {
			b.WriteString(noTest)
		}
		fmt.Fprintf(&b, format, a...)
		mu.Lock()
		defer mu.Unlock()
		_, _ = w.Write(b.Bytes())
	}

	pids, err := descendants(parent)
	said := ""
	if err != nil {
		said = fmt.Sprintf("not all of them found: %v\n", err)
	}
	part("\n== hangdump: the programs process %d started, %d of them, before go test's -timeout\n%s%s",
		parent, len(pids), said, states(pids))

	// Every program is sampled at once, so a slow one, or one sample
	// never comes back from, holds up none of the others.
	if runtime.GOOS == "darwin" {
		var wg sync.WaitGroup
		for _, pid := range pids {
			wg.Go(func() {
				part("\n== process %d:\n%s", pid, sample(pid))
			})
		}
		wg.Wait()
	}
	part("\n== hangdump: end\n")
}

// descendants lists the programs parent started, and theirs, parents
// first. pgrep says 1 when a program started none, and anything else it
// says is an error, kept so the dump tells no programs from a pgrep that
// failed.
func descendants(parent int) ([]int, error) {
	var all []int
	var errs []error
	next := []int{parent}
	for len(next) > 0 {
		pid := next[0]
		next = next[1:]
		out, err := exec.Command("pgrep", "-P", strconv.Itoa(pid)).Output()
		var exit *exec.ExitError
		if errors.As(err, &exit) && exit.ExitCode() == 1 {
			continue
		}
		if err != nil {
			errs = append(errs, fmt.Errorf("pgrep -P %d: %w", pid, err))
			continue
		}
		for _, field := range strings.Fields(string(out)) {
			if child, err := strconv.Atoi(field); err == nil {
				all = append(all, child)
				next = append(next, child)
			}
		}
	}
	return all, errors.Join(errs...)
}

// states is what ps says of every pid, in one call, or why it said
// nothing.
func states(pids []int) string {
	if len(pids) == 0 {
		return ""
	}
	list := make([]string, len(pids))
	for i, pid := range pids {
		list[i] = strconv.Itoa(pid)
	}
	out, err := exec.Command("ps", "-o", "pid,ppid,stat,etime,time,command", "-p", strings.Join(list, ",")).CombinedOutput()
	if err != nil {
		return fmt.Sprintf("%sps: %v\n", out, err)
	}
	return string(out)
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
