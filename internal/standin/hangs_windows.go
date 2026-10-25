package standin

import "testing"

// Hangs needs a named pipe, see hangs_unix.go, so a test of it is skipped
// here, as the shell scripts it is for are.
func Hangs(t testing.TB) (string, <-chan struct{}) {
	t.Skip("a stand-in says it hangs through a named pipe")
	return "", nil
}
