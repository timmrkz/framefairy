//go:build unix

package standin

import (
	"os"
	"os/exec"
	"testing"
)

// A program says it hangs through the pipe, and the test that made the
// pipe ends: t.Run returns once its cleanup has. On macOS the cleanup
// waited for good, since closing a named pipe there does not end a read
// under way.
//
// On the real disk: a named pipe is a file another program writes into.
func TestAHangIsHeardAndTheTestEnds(t *testing.T) {
	t.Parallel()
	t.Run("hangs", func(t *testing.T) {
		pipe, hangs := Hangs(t)
		out, err := exec.Command("sh", "-c", "echo hangs > '"+pipe+"'").CombinedOutput()
		if err != nil {
			t.Fatalf("%s %s", err, out)
		}
		<-hangs
		if _, err := os.Stat(pipe); err != nil {
			t.Fatal(err)
		}
	})
}
