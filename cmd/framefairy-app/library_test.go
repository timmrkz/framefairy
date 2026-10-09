package main

import (
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"testing"

	"framefairy/internal/ffmpegtest"
)

// Add takes a file only where the episode's decoder hands over its first
// frame: a file with no picture, and one that is no video at all, are
// left out with the reason, and a video beside them is added. The video
// preview would have nothing to show of them and the render nothing to
// cut. Step 3 of docs/VIDEO-PREVIEW.md.
func TestAddLeavesOutWhatItsDecoderCannotDecode(t *testing.T) {
	d := open(t)
	good := d.episode("gut", "12")
	sound := filepath.Join(d.home, "nur-ton.mp4")
	if out, err := exec.Command("ffmpeg", "-loglevel", "error", "-y", "-f", "lavfi", "-i", "sine=d=3",
		"-c:a", "aac", sound).CombinedOutput(); err != nil {
		ffmpegtest.Unusable(t, "ffmpeg could not make a file of sound: %s %s", err, out)
	}
	broken := filepath.Join(d.home, "kaputt.mp4")
	if err := os.WriteFile(broken, []byte("no video at all"), 0o644); err != nil {
		t.Fatal(err)
	}
	added, err := d.svc.addEpisodes([]string{sound, good, broken})
	if len(added) != 1 || added[0] != good {
		t.Fatalf("added %v, want only %s", added, filepath.Base(good))
	}
	for _, name := range []string{"nur-ton.mp4", "kaputt.mp4"} {
		if err == nil || !strings.Contains(err.Error(), "the picture of "+name+" cannot be decoded") {
			t.Errorf("Add said %v, want it to say why %s was left out", err, name)
		}
	}
	if err != nil && !strings.Contains(err.Error(), "has no picture") {
		t.Errorf("Add said %v, want ffmpeg's reason for the file of sound, that it has no picture", err)
	}
	for _, p := range d.svc.store.Episodes() {
		if p != good {
			t.Errorf("%s is in the library", filepath.Base(p))
		}
	}
}
