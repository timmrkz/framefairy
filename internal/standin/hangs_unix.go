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
			select {
			case said <- struct{}{}:
			default:
			}
		}
	}()
	t.Cleanup(func() {
		f.Close()
		<-read
	})
	return pipe, said
}
