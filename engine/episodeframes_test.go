package engine

import (
	"context"
	"errors"
	"math"
	"os"
	"os/exec"
	"path/filepath"
	"sync"
	"testing"
	"time"

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
		ffmpegtest.Unusable(t, "FRAMEFAIRY_FRAMES names no framefairy-frames, which make frames builds")
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

// Everything the Go side does to the episode's decoder at once: streams
// from many places on several goroutines, some called off at once, some
// after a few frames, some read to their end, while the decoder is closed
// and started again under them. Every stream either hands over frames in
// order or ends with a reason, none hangs, and none gets another's frames.
func TestTheEpisodesDecoderTakesStreamsFromEverywhereAtOnce(t *testing.T) {
	ffmpegtest.Need(t)
	program := os.Getenv("FRAMEFAIRY_FRAMES")
	if program == "" {
		ffmpegtest.Unusable(t, "FRAMEFAIRY_FRAMES names no framefairy-frames, which make frames builds")
	}
	path := filepath.Join(t.TempDir(), "episode.mp4")
	if out, err := exec.Command("ffmpeg", "-loglevel", "error", "-y", "-f", "lavfi",
		"-i", "testsrc2=s=320x180:r=25:d=6", "-c:v", "mpeg4", "-q:v", "3", "-g", "25", path).CombinedOutput(); err != nil {
		ffmpegtest.Unusable(t, "ffmpeg could not make an episode: %s %s", err, out)
	}
	dec := NewEpisodeFrames(program, path)
	defer dec.Close()
	var wg sync.WaitGroup
	for g := range 8 {
		wg.Go(func() {
			for i := range 12 {
				from := float64((g*7+i*3)%60) / 10
				ctx, cancel := context.WithCancel(context.Background())
				want := (g + i) % 4 * 5
				if want == 0 {
					cancel()
				}
				last, n := -1.0, 0
				err := dec.Stream(ctx, from, 64, 36, func(at float64, data []byte) error {
					if at < from-1e-6 || at <= last {
						t.Errorf("a stream from %.1f got a frame at %.3f after %.3f", from, at, last)
					}
					if len(data) != 64*36*3/2 {
						t.Errorf("a frame of %d bytes", len(data))
					}
					last = at
					if n++; n == want {
						return errStop
					}
					return nil
				})
				cancel()
				if err != nil && !errors.Is(err, errStop) && !errors.Is(err, context.Canceled) &&
					!errors.Is(err, ErrFramesClosed) {
					t.Errorf("a stream from %.1f ended with %v", from, err)
				}
				if g == 0 && i%5 == 4 {
					dec.Close()
				}
			}
		})
	}
	done := make(chan struct{})
	go func() {
		wg.Wait()
		close(done)
	}()
	select {
	case <-done:
	case <-time.After(60 * time.Second):
		t.Fatal("streams still running after a minute")
	}
}
