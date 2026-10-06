// Package ffmpegtest is the one way a test that needs ffmpeg ends when it
// finds none it can use.
//
// On a machine of one's own that is a skip with the reason, so the suite
// still passes without ffmpeg. In CI it is a failure with the reason. A
// skip reads as a pass there, because CI does not list skipped tests, so
// a run whose ffmpeg was missing or broken was as green as one that
// rendered. The macOS job once ran with no ffmpeg at all, and nothing that
// renders was tested on the system that ships first.
//
// Every test that renders, frames, listens or measures goes through Need
// or Unusable, and TestNoTestSkipsForFFmpegByItself holds every other
// test file to that.
package ffmpegtest

import (
	"fmt"
	"os"
	"os/exec"
	"strings"
	"testing"
)

// InCI says whether the tests run on a build runner. CI is the variable
// GitHub Actions sets, and the one the Makefile reads to install nothing.
func InCI() bool {
	return os.Getenv("CI") != ""
}

// Need ends the test the way Unusable does when ffmpeg or ffprobe is not
// on the search path.
func Need(t testing.TB) {
	t.Helper()
	for _, name := range []string{"ffmpeg", "ffprobe"} {
		if _, err := exec.LookPath(name); err != nil {
			Unusable(t, "%s is not installed", name)
			return
		}
	}
}

// Unusable ends a test that cannot have the ffmpeg it needs here, for the
// reason given: skipped on a machine of one's own, failed in CI.
func Unusable(t testing.TB, format string, args ...any) {
	t.Helper()
	reason := fmt.Sprintf(format, args...)
	if InCI() {
		t.Fatalf("%s. In CI that fails, because a skipped test reads as a pass",
			strings.TrimRight(reason, "."))
		return
	}
	t.Skip(reason)
}
