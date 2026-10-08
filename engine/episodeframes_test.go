package engine

import (
	"context"
	"errors"
	"math"
	"os"
	"os/exec"
	"path/filepath"
	"testing"

	"framefairy/internal/ffmpegtest"
)

// The episode's decoder hands over the frames the ffmpeg program hands
// over, from any moment, with the same moments and the same pictures, and
// a second stream moves a cursor that is open rather than opening one.
// It needs framefairy-frames built against ffmpeg's libraries, which make
// does, named by FRAMEFAIRY_FRAMES, and skips without it.
func TestTheEpisodesDecoderHandsOverTheFramesFFmpegDoes(t *testing.T) {
	ffmpegtest.Need(t)
	program := os.Getenv("FRAMEFAIRY_FRAMES")
	if program == "" {
		t.Skip("FRAMEFAIRY_FRAMES names no framefairy-frames built with ffmpeg's libraries")
	}
	path := filepath.Join(t.TempDir(), "episode.mp4")
	// Ten seconds at 25 frames a second with a key frame every two seconds,
	// so a jump decodes from the key frame before it.
	if out, err := exec.Command("ffmpeg", "-loglevel", "error", "-y", "-f", "lavfi",
		"-i", "testsrc2=s=320x180:r=25:d=10", "-c:v", "mpeg4", "-q:v", "3", "-g", "50", path).CombinedOutput(); err != nil {
		ffmpegtest.Unusable(t, "ffmpeg could not make an episode: %s %s", err, out)
	}
	e := NewEngine(NewLog(nil, false, false))
	e.FFmpeg, e.FFprobe = "ffmpeg", "ffprobe"
	dec := NewEpisodeFrames(program, path)
	defer dec.Close()

	type frame struct {
		at   float64
		data []byte
	}
	collect := func(run func(got func(float64, []byte) error) error) []frame {
		var out []frame
		err := run(func(at float64, data []byte) error {
			out = append(out, frame{at, data})
			if len(out) == 12 {
				return errStop
			}
			return nil
		})
		if err != nil && !errors.Is(err, errStop) {
			t.Fatal(err)
		}
		return out
	}
	for _, from := range []float64{0, 2.1, 7.39, 3.98} {
		want := collect(func(got func(float64, []byte) error) error {
			return e.PreviewFrames(context.Background(), path, from, 160, 90, got)
		})
		got := collect(func(got func(float64, []byte) error) error {
			return dec.Stream(context.Background(), from, 160, 90, got)
		})
		if len(got) != len(want) {
			t.Fatalf("from %.2f the decoder gave %d frames, ffmpeg %d", from, len(got), len(want))
		}
		for i := range want {
			if math.Abs(got[i].at-want[i].at) > 1e-6 {
				t.Fatalf("from %.2f frame %d starts at %.4f, ffmpeg says %.4f", from, i, got[i].at, want[i].at)
			}
			if d := meanDiff(got[i].data, want[i].data); d > 2 {
				t.Fatalf("from %.2f frame %d differs from ffmpeg's by %.2f on average", from, i, d)
			}
		}
	}
	dec.mu.Lock()
	opened := dec.nextID
	dec.mu.Unlock()
	if opened != 1 {
		t.Errorf("four streams one after another opened %d cursors, want one moved each time", opened)
	}
}

var errStop = errors.New("enough")

func meanDiff(a, b []byte) float64 {
	if len(a) != len(b) {
		return math.Inf(1)
	}
	sum := 0
	for i := range a {
		d := int(a[i]) - int(b[i])
		if d < 0 {
			d = -d
		}
		sum += d
	}
	return float64(sum) / float64(len(a))
}
