package main

import (
	"os"
	"os/exec"
	"path/filepath"
	"runtime"
	"strings"
	"testing"
	"time"

	"framefairy/internal/ffmpegtest"
)

// Add takes a file only where the episode's decoder hands over its first
// frame: a file with no picture, and one that is no video at all, are
// left out with the reason, and a video beside them is added. The video
// preview would have nothing to show of them and the render nothing to
// cut. Step 3 of docs/VIDEO-PREVIEW.md.
//
// On the real disk: the episode's decoder reads what is added.
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

// A video whose first frame does not come in time is taken, not left out.
// On CI's Mac, with its media engine hung, three tests had a good video
// left out after 30 s as "the picture cannot be decoded, context deadline
// exceeded". A wait says nothing about the file, only about the machine,
// and only the decoder's own words leave a video out. Plan row 2.233.
//
// The decoder is a stand-in that takes every request and answers none,
// the way one hung in the media engine does.
//
// On the real disk: the stand-in decoder is a program that is run. And
// FRAMEFAIRY_FRAMES names it for the whole process, which open does
// already with HOME, so the test does not run beside others.
func TestAddTakesAVideoWhoseFirstFrameIsLate(t *testing.T) {
	if runtime.GOOS == "windows" {
		t.Skip("the stand-in is a shell script")
	}
	d := open(t)
	good := d.episode("spaet", "6")
	hung := filepath.Join(t.TempDir(), "framefairy-frames")
	if err := os.WriteFile(hung, []byte("#!/bin/sh\nexec cat >/dev/null\n"), 0o755); err != nil {
		t.Fatal(err)
	}
	t.Setenv("FRAMEFAIRY_FRAMES", hung)
	abs, _ := filepath.Abs(good)
	t.Cleanup(func() {
		openPreviews.mu.Lock()
		dec := openPreviews.decoders[abs]
		delete(openPreviews.decoders, abs)
		openPreviews.mu.Unlock()
		if dec != nil {
			dec.Close()
		}
	})
	d.svc.firstFrame = time.Second
	added, err := d.svc.addEpisodes([]string{good})
	if err != nil {
		t.Fatalf("Add said %v", err)
	}
	if len(added) != 1 || added[0] != good {
		t.Fatalf("added %v, want %s", added, filepath.Base(good))
	}
}
