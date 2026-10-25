package main

import (
	"os"
	"os/exec"
	"testing"
)

// A copy of the test binary started the way Wails' updater starts its
// helper on a restart leaves at once and runs no test. Before, it ran the
// whole suite again, renders included, and TestUpdatesFromEverywhereAtOnce
// in it could start further copies, see TestMain.
func TestAnUpdaterHelperCopyOfTheTestsLeavesAtOnce(t *testing.T) {
	t.Parallel()
	// Asked for one quick test of its own, a copy that runs tests says so
	// at once and ends, and starts no copy of its own.
	cmd := exec.Command(os.Args[0], "-test.v", "-test.run=^TestOnlyAKeyIsEverKept$")
	cmd.Env = append(os.Environ(), updaterHelper+"=1")
	out, err := cmd.CombinedOutput()
	if err != nil {
		t.Fatalf("the helper copy failed: %v: %s", err, out)
	}
	if len(out) != 0 {
		t.Errorf("the helper copy ran tests: %s", out)
	}
}
