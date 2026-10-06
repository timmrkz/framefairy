package main

import (
	"context"
	"os"
	"os/exec"
	"testing"
	"time"
)

// A copy of the test binary started the way Wails' updater starts its
// helper on a restart leaves at once and runs no test. Before, it ran the
// whole suite again, renders included, and TestUpdatesFromEverywhereAtOnce
// in it could start further copies, see TestMain.
func TestAnUpdaterHelperCopyOfTheTestsLeavesAtOnce(t *testing.T) {
	ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
	defer cancel()
	cmd := exec.CommandContext(ctx, os.Args[0], "-test.v")
	cmd.Env = append(os.Environ(), updaterHelper+"=1")
	// A copy that runs the tests starts ffmpeg, which keeps its output
	// open after the copy is stopped. Wait no longer than a second for it.
	cmd.WaitDelay = time.Second
	out, err := cmd.CombinedOutput()
	if ctx.Err() != nil {
		t.Fatalf("the helper copy was still running after 10 s: %s", out)
	}
	if err != nil {
		t.Fatalf("the helper copy failed: %v: %s", err, out)
	}
	if len(out) != 0 {
		t.Errorf("the helper copy ran tests: %s", out)
	}
}
