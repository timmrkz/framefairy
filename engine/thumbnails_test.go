package engine

import (
	"context"
	"fmt"
	"image/jpeg"
	"os"
	"os/exec"
	"path/filepath"
	"testing"

	"framefairy/internal/ffmpegtest"
)

// The pictures of a short are frames of the short, taken on the short's
// own clock: a moment after a cut is further back in the short than in the
// episode. Pictures no longer asked for are removed, and nothing that
// belongs to another clip is touched.
//
// On the real disk: ffmpeg reads the short and writes its thumbnails.
func TestThumbnailsAreFramesOfTheShort(t *testing.T) {
	ffmpegtest.Need(t)
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
	e := NewEngine(nil, disk)
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

// A thumbnail is the frame the video preview showed where it was marked,
// which is the frame that holds the moment, see frameHolding. A moment is
// marked in the middle of its frame, and ffmpeg keeps the first frame that
// starts at or after a seek, so a seek to the moment itself took the frame
// after it. In a bounce one frame is a large step, and Tim's thumbnail
// caught a word a frame further into it than he had marked.
//
// On the real disk: ffmpeg renders the short and cuts the thumbnail from it.
func TestAThumbnailIsTheFrameThatWasMarked(t *testing.T) {
	ffmpegtest.Need(t)
	dir := t.TempDir()
	short := filepath.Join(dir, "01_eins.mp4")
	// Every frame its own grey: frame n is 16+8n.
	out, err := exec.Command("ffmpeg", "-loglevel", "error", "-y",
		"-f", "lavfi", "-i", "color=c=black:s=64x64:r=25:d=1,format=yuv420p,geq=lum='16+8*N':cb=128:cr=128",
		"-c:v", "mpeg4", "-q:v", "1", short).CombinedOutput()
	if err != nil {
		t.Fatalf("making the short: %s %s", err, out)
	}
	frame := func(n int) [3]int {
		t.Helper()
		px, err := exec.Command("ffmpeg", "-loglevel", "error", "-i", short,
			"-vf", fmt.Sprintf("select=eq(n\\,%d),scale=1:1", n), "-frames:v", "1",
			"-f", "rawvideo", "-pix_fmt", "rgb24", "-").Output()
		if err != nil || len(px) != 3 {
			t.Fatalf("reading frame %d of the short: %v", n, err)
		}
		return [3]int{int(px[0]), int(px[1]), int(px[2])}
	}
	// The middle of frames 0, 7 and 24, the last, as the video preview
	// marks a moment, see frameMiddle in lib/flow.ts. 10 is where the
	// clip starts in the episode.
	clip := Clip{ID: "01", Slug: "eins", Segments: []Segment{{Start: 10, End: 11}},
		Thumbnails: []float64{10.02, 10.30, 10.98}}
	if err := NewEngine(nil, disk).WriteThumbnails(context.Background(), clip, short); err != nil {
		t.Fatal(err)
	}
	for i, n := range []int{0, 7, 24} {
		got := jpegColour(t, filepath.Join(dir, thumbnailName("01_eins", i+1)))
		want := frame(n)
		if !closeColour(got, want, 3) {
			t.Errorf("thumbnail %d is %v, frame %d of the short is %v and frame %d is %v",
				i+1, got, n, want, n+1, frame(min(n+1, 24)))
		}
	}
}

// A thumbnail has the short's colours as a player shows them. The short
// keeps its colours the way HD video does, BT.709, and a JPEG is read the
// way JFIF says, BT.601, by every reader there is. Written with the short's
// numbers unchanged, the caption purple came out (138, 13, 141) rather
// than (147, 31, 145), the darker picture Tim saw.
//
// On the real disk: ffmpeg renders the short and cuts the thumbnail from it.
func TestAThumbnailHasTheColoursOfTheShort(t *testing.T) {
	ffmpegtest.Need(t)
	dir := t.TempDir()
	short := filepath.Join(dir, "01_eins.mp4")
	out, err := exec.Command("ffmpeg", "-loglevel", "error", "-y",
		"-f", "lavfi", "-i", "color=c=0x942192:s=64x64:r=25:d=1",
		"-vf", "scale=out_color_matrix=bt709:out_range=tv,format=yuv420p,"+
			"setparams=color_primaries=bt709:color_trc=bt709:colorspace=bt709:range=tv",
		"-c:v", "mpeg4", "-q:v", "1", short).CombinedOutput()
	if err != nil {
		t.Fatalf("making the short: %s %s", err, out)
	}
	// The short as a player shows it, from its own tags.
	px, err := exec.Command("ffmpeg", "-loglevel", "error", "-i", short, "-frames:v", "1",
		"-vf", "scale=1:1", "-f", "rawvideo", "-pix_fmt", "rgb24", "-").Output()
	if err != nil || len(px) != 3 {
		t.Fatalf("reading the short: %v", err)
	}
	shown := [3]int{int(px[0]), int(px[1]), int(px[2])}
	clip := Clip{ID: "01", Slug: "eins", Segments: []Segment{{Start: 0, End: 1}},
		Thumbnails: []float64{0.5}}
	if err := NewEngine(nil, disk).WriteThumbnails(context.Background(), clip, short); err != nil {
		t.Fatal(err)
	}
	if got := jpegColour(t, filepath.Join(dir, thumbnailName("01_eins", 1))); !closeColour(got, shown, 3) {
		t.Errorf("the thumbnail is %v where the short shows %v", got, shown)
	}
}

// jpegColour is the colour in the middle of a JPEG as Go's own decoder
// reads it, by JFIF, the way every other reader does.
//
// On the real disk: ffmpeg wrote the picture.
func jpegColour(t *testing.T, path string) [3]int {
	t.Helper()
	f, err := os.Open(path)
	if err != nil {
		t.Fatal(err)
	}
	defer f.Close()
	img, err := jpeg.Decode(f)
	if err != nil {
		t.Fatalf("reading %s: %v", path, err)
	}
	b := img.Bounds()
	r, g, bl, _ := img.At((b.Min.X+b.Max.X)/2, (b.Min.Y+b.Max.Y)/2).RGBA()
	return [3]int{int(r >> 8), int(g >> 8), int(bl >> 8)}
}

// closeColour says whether two colours are within so much of each other in
// every channel.
func closeColour(a, b [3]int, by int) bool {
	for i := range a {
		if d := a[i] - b[i]; d > by || d < -by {
			return false
		}
	}
	return true
}
