package hangdump

import (
	"strconv"
	"strings"
	"testing"
)

// hasStack checks that a dump holds the stack of pid, which a Mac
// samples.
func hasStack(t *testing.T, said string, pid int) {
	t.Helper()
	if !strings.Contains(said, "== process "+strconv.Itoa(pid)+":\n") || !strings.Contains(said, "Call graph") {
		t.Errorf("no stack of %d on a Mac:\n%s", pid, said)
	}
}
