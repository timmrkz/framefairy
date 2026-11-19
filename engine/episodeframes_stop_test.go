//go:build unix

package engine

import (
	"bufio"
	"bytes"
	"context"
	"errors"
	"fmt"
	"log/slog"
	"math"
	"os"
	"os/exec"
	"regexp"
	"strconv"
	"sync"
	"syscall"
	"testing"

	"framefairy/internal/ffmpegtest"
	"framefairy/internal/fixtures"
	"framefairy/internal/framewire"
)

// A stream closed before its first frame came leaves the decoder no work
// for it: the cursor stops between two frames instead of decoding on from
// the key frame to the frame asked for, and is free for the next stream at
// once. The page had parked the place the hand let go of behind such a
// stream for 950 ms on Tim's Mac, issue #924. The cursor then still takes
// up from where it stopped and hands over the frames ffmpeg does. The
// decoder is held still while the stream asks and is closed, so the stop
// is in its input before it does any of the work, the order a quick
// machine gives and a busy one may not.
//
// On the real disk: the frames decoder reads the episode.
func TestTheEpisodesDecoderStopsAStreamNobodyReads(t *testing.T) {
	t.Parallel()
	ffmpegtest.Need(t)
	program := os.Getenv("FRAMEFAIRY_FRAMES")
	if program == "" {
		ffmpegtest.Unusable(t, "FRAMEFAIRY_FRAMES names no framefairy-frames, which make frames builds")
	}
	// A key frame every two seconds and two B-frames in three.
	path := fixtures.Path(t, "picture.mp4")
	dec := newEpisodeFrames(program, path)
	defer dec.Close()
	var mu sync.Mutex
	var lines []string
	said := make(chan struct{}, 1)
	dec.heard = func(_ slog.Level, line string) {
		mu.Lock()
		lines = append(lines, line)
		mu.Unlock()
		select {
		case said <- struct{}{}:
		default:
		}
	}
	grid := gridOf(t, path)
	if err := dec.Ready(); err != nil {
		t.Fatal(err)
	}
	dec.mu.Lock()
	decoder := dec.cmd.Process
	dec.mu.Unlock()
	if err := decoder.Signal(syscall.SIGSTOP); err != nil {
		t.Fatal(err)
	}
	// 48 frames after the key frame at 0, about 16 of them decoded on the
	// way, see TestTheEpisodesDecoderDecodesAJumpNoFurtherThanItMust.
	const from = 1.92

	// The page closed the stream before anything came, the way it closes
	// one for a place the hand has left.
	gone, cancel := context.WithCancel(context.Background())
	cancel()
	handed := 0
	err := dec.Stream(gone, grid, from, 160, 90, false, nil, func(float64, []byte) error {
		handed++
		return nil
	})
	if !errors.Is(err, context.Canceled) {
		t.Fatalf("the closed stream ended with %v", err)
	}
	if handed != 0 {
		t.Errorf("the closed stream handed over %d frames", handed)
	}
	if err := decoder.Signal(syscall.SIGCONT); err != nil {
		t.Fatal(err)
	}
	// The decoder says it stopped, or, where it decoded on, the frame it
	// made. A decoder that says neither is go test's -timeout to end.
	stopped := regexp.MustCompile(`picture: stopped after (\d+) frames of \d+ asked, (\d+) decoded`)
	madeIt := regexp.MustCompile(`picture: first frame .*, (\d+) decoded`)
	var m []string
	for m == nil {
		mu.Lock()
		for _, line := range lines {
			if m = stopped.FindStringSubmatch(line); m != nil {
				break
			}
			if made := madeIt.FindStringSubmatch(line); made != nil {
				mu.Unlock()
				t.Fatalf("the decoder decoded on for the closed stream to its frame, %s frames", made[1])
			}
		}
		mu.Unlock()
		if m == nil {
			<-said
		}
	}
	if m[1] != "0" {
		t.Errorf("the decoder made %s frames for the closed stream, want none", m[1])
	}
	if d, _ := strconv.Atoi(m[2]); d >= 16 {
		t.Errorf("the decoder decoded %d frames for the closed stream, as many as the frame asked for needs", d)
	}

	// The next stream to the same place gets the frames ffmpeg gives.
	var wantAts, gotAts []float64
	var want, got [][]byte
	take := func(ats *[]float64, frames *[][]byte) func(float64, []byte) error {
		return func(at float64, data []byte) error {
			*ats, *frames = append(*ats, at), append(*frames, data)
			if len(*ats) == 4 {
				return errStop
			}
			return nil
		}
	}
	if err := ffmpegFrames(context.Background(), path, grid.At(grid.Holding(from)), 160, 90, take(&wantAts, &want)); err != nil && !errors.Is(err, errStop) {
		t.Fatal(err)
	}
	if err := dec.Stream(context.Background(), grid, from, 160, 90, false, nil, take(&gotAts, &got)); err != nil && !errors.Is(err, errStop) {
		t.Fatal(err)
	}
	if len(got) != len(want) {
		t.Fatalf("after the stop the decoder gave %d frames, ffmpeg %d", len(got), len(want))
	}
	for i := range want {
		if math.Abs(gotAts[i]-wantAts[i]) > 1e-6 {
			t.Fatalf("after the stop frame %d starts at %.4f, ffmpeg says %.4f", i, gotAts[i], wantAts[i])
		}
		if d := meanDiff(got[i], want[i]); d > 2 {
			t.Fatalf("after the stop frame %d differs from ffmpeg's by %.2f on average", i, d)
		}
	}
}

// A cursor stopped between two frames hands over the right frames when
// it is taken up again, wherever the stop came. Stopped once the key frame
// asked for first was decoded, and before it was handed over, the frames
// from it to the frame asked for had been let go unscaled or never
// decoded, so a cursor that went on from the key frame to one of them
// showed an earlier frame's picture in its place, issue #924. The decoder
// is spoken to directly, so the stop comes exactly there, and each frame
// is held against the one a fresh cursor makes.
//
// On the real disk: the frames decoder reads the episode.
func TestTheEpisodesDecoderGoesOnRightlyAfterAStopAtItsKeyFrame(t *testing.T) {
	t.Parallel()
	ffmpegtest.Need(t)
	program := os.Getenv("FRAMEFAIRY_FRAMES")
	if program == "" {
		ffmpegtest.Unusable(t, "FRAMEFAIRY_FRAMES names no framefairy-frames, which make frames builds")
	}
	// A key frame at 0 and at 2.04 s, two B-frames in three.
	path := fixtures.Path(t, "picture.mp4")
	grid := gridOf(t, path)
	cmd := exec.Command(program, path)
	// The job stops as a stop would once the key frame is decoded and
	// the frame after it, which says how long the key frame lasts.
	cmd.Env = append(os.Environ(), "FRAMEFAIRY_FRAMES_STOP_AFTER=2")
	in, err := cmd.StdinPipe()
	if err != nil {
		t.Fatal(err)
	}
	out, err := cmd.StdoutPipe()
	if err != nil {
		t.Fatal(err)
	}
	if err := cmd.Start(); err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() {
		_ = in.Close()
		_ = cmd.Wait()
	})
	answers := bufio.NewReader(out)
	// What one request and a next of n bring, up to the record that ends
	// the batch.
	ask := func(open string, cursor uint32, n int) [][]byte {
		t.Helper()
		if _, err := fmt.Fprintf(in, "%s\nnext %d %d 0\n", open, cursor, n); err != nil {
			t.Fatal(err)
		}
		var frames [][]byte
		for {
			r, err := framewire.Read(answers, nil)
			if err != nil {
				t.Fatal(err)
			}
			if r.Cursor != cursor {
				t.Fatalf("an answer for cursor %d while cursor %d was asked", r.Cursor, cursor)
			}
			switch r.Kind {
			case framewire.Frame:
				frames = append(frames, r.Body)
			case framewire.Failed:
				t.Fatalf("cursor %d failed: %s", cursor, r.Body)
			case framewire.Done, framewire.End:
				return frames
			}
		}
	}
	const w, h = 160, 90

	// Sent to frame 48 with its key frame first, and stopped there.
	if got := ask(framewire.OpenRequest(1, 48, w, h, grid, true), 1, 4); len(got) != 0 {
		t.Fatalf("the stopped cursor handed over %d frames", len(got))
	}
	// Taken up again for the frame after the key frame, a B-frame whose
	// packet came before the key frame was out, and was skipped then.
	got := ask(framewire.OpenRequest(1, 1, w, h, grid, false), 1, 1)
	want := ask(framewire.OpenRequest(2, 1, w, h, grid, false), 2, 1)
	if len(got) != 1 || len(want) != 1 {
		t.Fatalf("the cursors handed over %d and %d frames, want 1 each", len(got), len(want))
	}
	if !bytes.Equal(got[0], want[0]) {
		t.Errorf("frame 1 from the cursor stopped at its key frame differs from a fresh cursor's by %.2f on average", meanDiff(got[0], want[0]))
	}
}

// How long the frame the hand rests on takes when the hand passed four
// places before it, each closed before its first frame came, the way a
// drag leaves streams behind. Each place is about 1.9 s after its key
// frame, so a stream that decodes on to its frame does the most work. A
// decoder of its own for each round, ready before the clock starts.
//
// On the real disk: the frames decoder reads the episode.
func BenchmarkTheFrameTheHandRestsOnAfterFourPlacesPassed(b *testing.B) {
	ffmpegtest.Need(b)
	program := os.Getenv("FRAMEFAIRY_FRAMES")
	if program == "" {
		ffmpegtest.Unusable(b, "FRAMEFAIRY_FRAMES names no framefairy-frames, which make frames builds")
	}
	// Key frames at 0, 2.04, 3.96 and 5.88 s and on, 25 a second.
	path := fixtures.Path(b, "picture.mp4")
	info, err := ffmpegEngine().Probe(context.Background(), path)
	if err != nil {
		b.Fatal(err)
	}
	grid := info.Grid()
	first := func(dec *EpisodeFrames, ctx context.Context, from float64) {
		err := dec.Stream(ctx, grid, from, 320, 180, false, nil, func(float64, []byte) error { return errStop })
		if err != nil && !errors.Is(err, errStop) && !errors.Is(err, context.Canceled) {
			b.Fatal(err)
		}
	}
	gone, cancel := context.WithCancel(context.Background())
	cancel()
	for b.Loop() {
		b.StopTimer()
		dec := newEpisodeFrames(program, path)
		first(dec, context.Background(), 0)
		b.StartTimer()
		for _, at := range []float64{1.92, 3.84, 5.76, 7.68} {
			first(dec, gone, at)
		}
		first(dec, context.Background(), 9.6)
		b.StopTimer()
		dec.Close()
		b.StartTimer()
	}
}
