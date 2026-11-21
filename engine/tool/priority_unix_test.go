//go:build unix

package tool

import (
	"os/exec"
	"testing"

	"golang.org/x/sys/unix"
)

// A program lowered runs at Background, below the work somebody waits on,
// which is how the loudness is measured out of the clip timeline's view
// beside the hearing, #968.
//
// On the real disk: a program is run, the system's own cat, which waits on
// its input until the test closes it.
func TestALoweredProgramRunsInTheBackground(t *testing.T) {
	t.Parallel()
	cmd := exec.Command("cat")
	in, err := cmd.StdinPipe()
	if err != nil {
		t.Fatal(err)
	}
	if err := cmd.Start(); err != nil {
		t.Skipf("no cat to run: %v", err)
	}
	defer func() {
		_ = in.Close()
		_ = cmd.Wait()
	}()
	before, err := unix.Getpriority(unix.PRIO_PROCESS, cmd.Process.Pid)
	if err != nil {
		t.Fatal(err)
	}
	if err := Lower(cmd.Process); err != nil {
		t.Fatal(err)
	}
	after, err := unix.Getpriority(unix.PRIO_PROCESS, cmd.Process.Pid)
	if err != nil {
		t.Fatal(err)
	}
	// Linux's getpriority answers 20 less the nice value, the Mac's the
	// nice value itself, and for Background, 10, both answer 10.
	if before == after || after != Background {
		t.Errorf("the program ran at %d and runs at %d after it was lowered, not at %d", before, after, Background)
	}
}
