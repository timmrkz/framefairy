package engine

import "testing"

// stuckFFmpeg is a shell script on other systems, see stuck_unix_test.go.
func stuckFFmpeg(t *testing.T, _ bool) string {
	t.Skip("the stand-in is a shell script")
	return ""
}
