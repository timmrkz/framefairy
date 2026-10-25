//go:build unix

package standin

import (
	"bufio"
	"os"
	"path/filepath"
	"syscall"
	"testing"
)

// Hangs makes a named pipe a stand-in writes a line into when it hangs,
// `echo hangs > pipe`, and gives a value on hangs for each line, until the
// test ends. A test that waits for a program to hang hears it then,
// rather than after a time limit of the clock.
func Hangs(t testing.TB) (pipe string, hangs <-chan struct{}) {
	t.Helper()
	pipe = filepath.Join(t.TempDir(), "hangs")
	if err := syscall.Mkfifo(pipe, 0o600); err != nil {
		t.Fatal(err)
	}
	// Open to write as well as to read, so the stand-in never waits for a
	// reader and the reading never ends until the test does.
	f, err := os.OpenFile(pipe, os.O_RDWR, 0)
	if err != nil {
		t.Fatal(err)
	}
	said := make(chan struct{}, 100)
	read := make(chan struct{})
	go func() {
		defer close(read)
		defer close(said)
		for lines := bufio.NewScanner(f); lines.Scan(); {
			if lines.Text() == over {
				return
			}
			select {
			case said <- struct{}{}:
			default:
			}
		}
	}()
	// On macOS a named pipe is read without the poller, so closing it
	// does not end a read under way, and the test waited on it until go
	// test's -timeout. The reading is told through the pipe instead.
	t.Cleanup(func() {
		_, _ = f.WriteString(over + "\n")
		<-read
		f.Close()
	})
	return pipe, said
}

// over is the line that ends the reading, one no stand-in writes.
const over = "\x00 the test is over"
