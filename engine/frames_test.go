package engine

import (
	"bytes"
	"context"
	"image/jpeg"
	"os"
	"os/exec"
	"path/filepath"
	"sync"
	"testing"
)

// The app asks for a frame every time the playhead moves, and walking
// the clip list with the arrow keys moves it as fast as a key repeats. So
// several asks for the same frame, and for frames either side of it, are
// in the air at once. Each one wrote to a temporary file named after the
// frame alone, so two asks for the same frame wrote over each other and
// what was left behind was half of one and half of the other. The app
// then showed that, and went on showing it, because a frame once written
// is kept.
func TestTheSameFrameAskedForFromEveryDirectionAtOnce(t *testing.T) {
	source := testEpisode(t, "12")
	e := NewEngine(NewLog(&bytes.Buffer{}, false, false))

	// Every one of these lands on the same few frames, which is what
	// walking a list of clips up and down does.
	seconds := []float64{1, 2, 1, 3, 2, 1, 3, 2, 1, 2, 3, 1}
	got := make([]string, len(seconds))
	errs := make([]error, len(seconds))
	var wg sync.WaitGroup
	for i, at := range seconds {
		wg.Add(1)
		go func() {
			defer wg.Done()
			got[i], errs[i] = e.Still(context.Background(), source, at, 25, 320)
		}()
	}
	wg.Wait()

	for i, err := range errs {
		if err != nil {
			t.Fatalf("asking for the frame at %vs: %v", seconds[i], err)
		}
	}
	// Every frame that came back has to be a whole picture. A file half
	// written by one ask and half by another still has a name and a size.
	for i, path := range got {
		data, err := os.ReadFile(path)
		if err != nil {
			t.Fatalf("reading the frame at %vs: %v", seconds[i], err)
		}
		if len(data) < 128 {
			t.Errorf("the frame at %vs is %d bytes", seconds[i], len(data))
			continue
		}
		if data[0] != 0xFF || data[1] != 0xD8 {
			t.Errorf("the frame at %vs does not start like a jpeg", seconds[i])
		}
		// A jpeg ends with its own mark. Half a file does not.
		if end := data[len(data)-2:]; end[0] != 0xFF || end[1] != 0xD9 {
			t.Errorf("the frame at %vs is not a whole jpeg: it ends %x", seconds[i], end)
		}
	}

	// And nothing half written is left lying about in the folder.
	dir := filepath.Dir(got[0])
	entries, err := os.ReadDir(dir)
	if err != nil {
		t.Fatal(err)
	}
	for _, entry := range entries {
		if len(entry.Name()) > 4 && entry.Name()[len(entry.Name())-4:] != ".jpg" {
			t.Errorf("left behind: %s", entry.Name())
		}
		if bytes.Contains([]byte(entry.Name()), []byte(".part")) {
			t.Errorf("left behind: %s", entry.Name())
		}
	}
}

// The still is the frame the video preview shows at a moment: the one the
// moment falls in. It was the frame at the whole second, and the app asked
// for the nearest second, so a still laid over the video preview while it
// caught up was up to a second away from the frame that followed it.
func TestAStillIsTheFrameTheMomentFallsIn(t *testing.T) {
	if _, err := exec.LookPath("ffmpeg"); err != nil {
		t.Skip("ffmpeg is not installed")
	}
	// Ten frames a second, each a grey of its own: frame n is 20 times n.
	source := filepath.Join(t.TempDir(), "frames.mp4")
	out, err := exec.Command("ffmpeg", "-loglevel", "error", "-y",
		"-f", "lavfi", "-i", "color=c=black:s=64x36:r=10:d=2",
		"-vf", "geq=lum='20*mod(N,10)+10':cb=128:cr=128",
		"-c:v", "mpeg4", "-q:v", "5", "-g", "5", "-pix_fmt", "yuv420p", source).CombinedOutput()
	if err != nil {
		t.Fatalf("making the episode: %s %s", err, out)
	}
	e := NewEngine(NewLog(&bytes.Buffer{}, false, false))
	for n := range 10 {
		for _, into := range []float64{0, 0.3, 0.7, 0.99} {
			at := 1 + (float64(n)+into)/10
			path, err := e.Still(context.Background(), source, at, 10, 160)
			if err != nil {
				t.Fatal(err)
			}
			// The still is written in full range, which stretches video
			// levels, 16 to 235, over 0 to 255.
			want := (20*n + 10 - 16) * 255 / 219
			if got := greyOf(t, path); got < want-6 || got > want+6 {
				t.Errorf("at %.3fs the still is grey %d, frame %d is %d", at, got, n+10, want)
			}
		}
	}
}

// greyOf is the mean brightness of a jpeg.
func greyOf(t *testing.T, path string) int {
	t.Helper()
	f, err := os.Open(path)
	if err != nil {
		t.Fatal(err)
	}
	defer f.Close()
	img, err := jpeg.Decode(f)
	if err != nil {
		t.Fatal(err)
	}
	b := img.Bounds()
	sum, count := 0, 0
	for y := b.Min.Y; y < b.Max.Y; y++ {
		for x := b.Min.X; x < b.Max.X; x++ {
			r, _, _, _ := img.At(x, y).RGBA()
			sum += int(r >> 8)
			count++
		}
	}
	return sum / count
}
