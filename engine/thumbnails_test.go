package engine

import (
	"context"
	"os"
	"os/exec"
	"path/filepath"
	"testing"
)

// The pictures of a short are frames of the short, taken on the short's
// own clock: a moment after a cut is further back in the short than in the
// episode. Pictures no longer asked for are removed, and nothing that
// belongs to another clip is touched.
func TestThumbnailsAreFramesOfTheShort(t *testing.T) {
	if _, err := exec.LookPath("ffmpeg"); err != nil {
		t.Skip("ffmpeg is not installed")
	}
	dir := t.TempDir()
	short := filepath.Join(dir, "01_eins.mp4")
	// Two seconds of red, then two of blue: the two pieces of the clip.
	out, err := exec.Command("ffmpeg", "-loglevel", "error", "-y",
		"-f", "lavfi", "-i", "color=c=red:s=180x320:r=25:d=2",
		"-f", "lavfi", "-i", "color=c=blue:s=180x320:r=25:d=2",
		"-filter_complex", "[0:v][1:v]concat=n=2:v=1[v]", "-map", "[v]",
		"-c:v", "mpeg4", short).CombinedOutput()
	if err != nil {
		t.Fatalf("making the short: %s %s", err, out)
	}
	clip := Clip{ID: "01", Slug: "eins",
		Segments:   []Segment{{Start: 10, End: 12}, {Start: 20, End: 22}},
		Thumbnails: []float64{11, 21.5, 22}}
	// A picture of an earlier render that is no longer wanted, and pictures
	// of other clips, one of them with a name that starts the same.
	for _, name := range []string{"01_eins-4.jpg", "01_eins-extra-1.jpg", "02_zwei-1.jpg"} {
		if err := os.WriteFile(filepath.Join(dir, name), []byte("x"), 0o644); err != nil {
			t.Fatal(err)
		}
	}
	e := NewEngine(nil)
	if err := e.WriteThumbnails(context.Background(), clip, short); err != nil {
		t.Fatal(err)
	}
	colour := func(name string) (int, int, int) {
		t.Helper()
		px, err := exec.Command("ffmpeg", "-loglevel", "error", "-i", filepath.Join(dir, name),
			"-vf", "scale=1:1", "-f", "rawvideo", "-pix_fmt", "rgb24", "-").Output()
		if err != nil || len(px) != 3 {
			t.Fatalf("reading %s: %v", name, err)
		}
		return int(px[0]), int(px[1]), int(px[2])
	}
	if r, _, b := colour("01_eins-1.jpg"); r < 200 || b > 60 {
		t.Errorf("the first picture should be red, it is %d red %d blue", r, b)
	}
	// 21.5 in the episode is 3.5 in the short, and the last moment of the
	// clip is held inside it rather than falling off the end.
	for _, name := range []string{"01_eins-2.jpg", "01_eins-3.jpg"} {
		if r, _, b := colour(name); b < 200 || r > 60 {
			t.Errorf("%s should be blue, it is %d red %d blue", name, r, b)
		}
	}
	if _, err := os.Stat(filepath.Join(dir, "01_eins-4.jpg")); !os.IsNotExist(err) {
		t.Error("a picture no longer asked for was left")
	}
	for _, name := range []string{"01_eins-extra-1.jpg", "02_zwei-1.jpg", "01_eins.mp4"} {
		if _, err := os.Stat(filepath.Join(dir, name)); err != nil {
			t.Errorf("%s belongs to something else and is gone", name)
		}
	}
	// No pictures asked for removes the ones there were.
	clip.Thumbnails = nil
	if err := e.WriteThumbnails(context.Background(), clip, short); err != nil {
		t.Fatal(err)
	}
	if left, _ := filepath.Glob(filepath.Join(dir, "01_eins-[0-9].jpg")); len(left) != 0 {
		t.Errorf("pictures left behind: %v", left)
	}
}
