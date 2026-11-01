package hangdump

import (
	"bufio"
	"bytes"
	"encoding/json"
	"os"
	"os/exec"
	"runtime"
	"strconv"
	"strings"
	"testing"
)

// waiting starts cat reading a pipe the test holds open, so it waits for
// as long as the test needs, with no clock.
func waiting(t *testing.T) int {
	t.Helper()
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
	return cat.Process.Pid
}

// A program the tests started and that waits is written down with its
// state, and on a Mac with the stack of its threads, which is what a hang
// on CI's virtual Mac needs and the Go trace does not show.
//
// On the real disk: on a Mac, macOS's sample writes its report to a file.
func TestAProgramThatWaitsIsWrittenDown(t *testing.T) {
	cat := waiting(t)

	// cat starts nothing of its own, and pgrep saying so is no error.
	if got, err := descendants(cat); len(got) != 0 || err != nil {
		t.Errorf("cat started %v, %v", got, err)
	}

	var b bytes.Buffer
	Dump(&b, os.Getpid(), false)
	said := b.String()
	if !strings.Contains(said, "1 of them") || !strings.Contains(said, strconv.Itoa(cat)+" ") || !strings.Contains(said, "cat") {
		t.Fatalf("cat is not written down:\n%s", said)
	}
	if strings.Contains(said, "not all of them found") || strings.Contains(said, "ps:") {
		t.Errorf("the dump says a tool failed:\n%s", said)
	}
	if runtime.GOOS == "darwin" && (!strings.Contains(said, "== process "+strconv.Itoa(cat)+":\n") || !strings.Contains(said, "Call graph")) {
		t.Errorf("no stack of cat on a Mac:\n%s", said)
	}
	if !strings.HasSuffix(said, "== hangdump: end\n") {
		t.Errorf("the dump does not end where it says:\n%s", said)
	}
}

// Under go test -json, the dump belongs to no test. test2json files a
// line under the last test that spoke, which by the time of a hang has
// mostly passed, and gotestsum shows nothing of a test that passed, so a
// dump filed under it never reaches CI's log.
func TestUnderJSONTheDumpBelongsToNoTest(t *testing.T) {
	if _, err := exec.LookPath("go"); err != nil {
		t.Skipf("no go to run test2json: %v", err)
	}
	waiting(t)

	// What the testing package writes under -v=test2json for a test that
	// spoke and passed, then the dump, then the end of the package.
	var in bytes.Buffer
	in.WriteString("\x16=== RUN   TestSpoke\n    a line of TestSpoke\n\x16--- PASS: TestSpoke (0.00s)\n")
	Dump(&in, os.Getpid(), true)
	in.WriteString("\x16PASS\n")

	cmd := exec.Command("go", "tool", "test2json")
	cmd.Stdin = &in
	out, err := cmd.Output()
	if err != nil {
		t.Fatalf("test2json: %v", err)
	}
	type event struct{ Action, Test, Output string }
	dump := 0
	lines := bufio.NewScanner(bytes.NewReader(out))
	for lines.Scan() {
		var e event
		if err := json.Unmarshal(lines.Bytes(), &e); err != nil {
			t.Fatalf("%v: %s", err, lines.Text())
		}
		if e.Action != "output" || !strings.Contains(e.Output, "hangdump") && !strings.Contains(e.Output, "cat") {
			continue
		}
		dump++
		if e.Test != "" {
			t.Errorf("the dump is filed under %s: %q", e.Test, e.Output)
		}
	}
	if dump < 3 {
		t.Errorf("test2json shows %d lines of the dump:\n%s", dump, out)
	}
}
