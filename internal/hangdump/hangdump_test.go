package hangdump

import (
	"bytes"
	"os"
	"os/exec"
	"runtime"
	"strconv"
	"strings"
	"testing"
)

// A program the tests started and that waits is written down with its
// state, and on a Mac with the stack of its threads, which is what a hang
// on CI's virtual Mac needs and the Go trace does not show, issue #638.
// The program is cat reading a pipe this test holds open, so it waits for
// as long as the test needs, with no clock.
func TestAProgramThatWaitsIsWrittenDown(t *testing.T) {
	cat := exec.Command("cat")
	in, err := cat.StdinPipe()
	if err != nil {
		t.Fatal(err)
	}
	if err := cat.Start(); err != nil {
		t.Skipf("no cat to start: %v", err)
	}
	t.Cleanup(func() {
		_ = in.Close()
		_ = cat.Wait()
	})

	// cat starts nothing of its own.
	if got := descendants(cat.Process.Pid); len(got) != 0 {
		t.Errorf("cat started %v", got)
	}

	var b bytes.Buffer
	Dump(&b, os.Getpid())
	said := b.String()
	if !strings.Contains(said, "== process "+strconv.Itoa(cat.Process.Pid)+":") || !strings.Contains(said, "cat") {
		t.Fatalf("cat is not written down:\n%s", said)
	}
	if runtime.GOOS == "darwin" && !strings.Contains(said, "Call graph") {
		t.Errorf("no stack of cat on a Mac:\n%s", said)
	}
	if !strings.HasSuffix(said, "== hangdump: end\n") {
		t.Errorf("the dump does not end where it says:\n%s", said)
	}
}
