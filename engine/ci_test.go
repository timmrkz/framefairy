package engine

import (
	"os"
	"os/exec"
	"testing"
)

// The tests that render, frame or listen skip themselves where there is no
// ffmpeg, which is right on a machine without one and wrong in CI, where a
// skip reads as a pass. The macOS job ran without ffmpeg, so none of them
// ran on the system that ships first, and the only sign was that its tests
// took five seconds where Linux took three minutes. In CI a missing ffmpeg
// is a failure.
func TestCIHasFFmpeg(t *testing.T) {
	if os.Getenv("CI") == "" {
		t.Skip("not in CI")
	}
	if _, err := exec.LookPath("ffmpeg"); err != nil {
		t.Fatal("CI runs without ffmpeg, so every test that renders skips itself and reads as a pass")
	}
}
