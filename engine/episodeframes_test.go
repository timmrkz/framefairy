package engine

import (
	"context"
	"encoding/binary"
	"errors"
	"fmt"
	"log/slog"
	"math"
	"os"
	"os/exec"
	"path/filepath"
	"regexp"
	"runtime"
	"slices"
	"strconv"
	"strings"
	"sync"
	"sync/atomic"
	"testing"

	"framefairy/internal/ffmpegtest"
	"framefairy/internal/framewire"
	"framefairy/internal/standin"
)

// The episode's decoder hands over the frames the ffmpeg program hands
// over, from any moment, with the same moments and the same pictures, and
// a stream after the first moves a cursor that is open rather than opening
// the file again.
// It needs framefairy-frames built against ffmpeg's libraries, which make
// does, named by FRAMEFAIRY_FRAMES, and skips without it.
//
// On the real disk: ffmpeg makes the episode and the frames decoder reads it.
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
	e := ffmpegOnTheProcessor()
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
	grid := gridOf(t, path)
	for i, from := range []float64{0, 2.1, 7.39, 3.98} {
		// From the frame that holds the moment, which on a file whose
		// frames keep to their rate is the file's own.
		want := collect(func(got func(float64, []byte) error) error {
			return e.PreviewFrames(context.Background(), path, grid.At(grid.Holding(from)), 160, 90, got)
		})
		times := NewPreviewTimes()
		got := collect(func(got func(float64, []byte) error) error {
			return dec.Stream(WithPreviewTimes(context.Background(), times), grid, from, 160, 90, false, nil, got)
		})
		if times.File.Load() != (i == 0) {
			t.Errorf("stream %d from %.2f opened the file: %v, want only the first to", i, from, times.File.Load())
		}
		if s, o, f := times.Started.Load(), times.Opened.Load(), times.First.Load(); s <= 0 || o < s || f < o {
			t.Errorf("stream %d told its times as %d, %d, %d, want the request, the place and the first frame in order", i, s, o, f)
		}
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
}

// A jump of the playhead is decoded no further than it has to be, plan row
// 2.243. A jump to a frame a little ahead of where a cursor already is,
// with no key frame between, goes on decoding from there rather than
// seeking back to the key frame and decoding it all again. And on the way
// to the frame asked for, the frames that no other frame is made from and
// that are shown before it are not decoded at all. The frames handed over
// are still the ones the ffmpeg program hands over.
//
// On the real disk: ffmpeg makes the episode and the frames decoder reads it.
func TestTheEpisodesDecoderDecodesAJumpNoFurtherThanItMust(t *testing.T) {
	ffmpegtest.Need(t)
	program := os.Getenv("FRAMEFAIRY_FRAMES")
	if program == "" {
		ffmpegtest.Unusable(t, "FRAMEFAIRY_FRAMES names no framefairy-frames, which make frames builds")
	}
	path := filepath.Join(t.TempDir(), "episode.mp4")
	// A key frame every two seconds and two B-frames between the others,
	// which no frame is made from, the way an export usually has them.
	if out, err := exec.Command("ffmpeg", "-loglevel", "error", "-y", "-f", "lavfi",
		"-i", "testsrc2=s=320x180:r=25:d=8", "-c:v", "mpeg4", "-q:v", "3", "-g", "50", "-bf", "2", path).CombinedOutput(); err != nil {
		ffmpegtest.Unusable(t, "ffmpeg could not make an episode: %s %s", err, out)
	}
	e := ffmpegOnTheProcessor()
	dec := NewEpisodeFrames(program, path)
	defer dec.Close()
	var mu sync.Mutex
	var lines []string
	// said has a value once the decoder has said more since it was last
	// read.
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

	collect := func(run func(got func(float64, []byte) error) error) (ats []float64, frames [][]byte) {
		err := run(func(at float64, data []byte) error {
			ats, frames = append(ats, at), append(frames, data)
			if len(ats) == 8 {
				return errStop
			}
			return nil
		})
		if err != nil && !errors.Is(err, errStop) {
			t.Fatal(err)
		}
		return ats, frames
	}
	grid := gridOf(t, path)
	// Each stream takes 8 frames, a third of a second. The decoder has
	// taken in the packets of more frames than it handed over, and on a
	// Mac at least 7 more: a stream from 1.4 s had taken in the key frame
	// at 2 s, and the jump to 3.1 s then rightly went on. So a jump that
	// should move past a key frame follows a stream that ended a second
	// before it.
	jumps := []struct {
		from float64
		how  string
	}{
		{0.2, "opened the file"},
		{0.6, "went on"}, // ahead, before the key frame at 2 s
		{3.1, "moved"},   // ahead, past the key frame at 2 s
		{2.4, "moved"},   // behind
		{4.3, "moved"},   // ahead, past the key frame at 4 s
		{5.5, "went on"}, // ahead, before the key frame at 6 s
		{1.92, "moved"},  // behind, 48 frames from its key frame
	}
	for _, j := range jumps {
		wantAts, want := collect(func(got func(float64, []byte) error) error {
			return e.PreviewFrames(context.Background(), path, grid.At(grid.Holding(j.from)), 160, 90, got)
		})
		times := NewPreviewTimes()
		gotAts, got := collect(func(got func(float64, []byte) error) error {
			return dec.Stream(WithPreviewTimes(context.Background(), times), grid, j.from, 160, 90, false, nil, got)
		})
		if len(got) != len(want) {
			t.Fatalf("from %.2f the decoder gave %d frames, ffmpeg %d", j.from, len(got), len(want))
		}
		// The stream says it went on, for the page to know no key frame
		// comes first, issue #511.
		if on := times.WentOn.Load(); on != (j.how == "went on") {
			t.Errorf("the jump to %.2f s, which %s, told the page it went on: %v", j.from, j.how, on)
		}
		for i := range want {
			if math.Abs(gotAts[i]-wantAts[i]) > 1e-6 {
				t.Fatalf("from %.2f frame %d starts at %.4f, ffmpeg says %.4f", j.from, i, gotAts[i], wantAts[i])
			}
			if d := meanDiff(got[i], want[i]); d > 2 {
				t.Fatalf("from %.2f frame %d differs from ffmpeg's by %.2f on average", j.from, i, d)
			}
		}
	}

	// What the decoder said of each jump: how it got there, and how many
	// frames it decoded on the way to the first one handed over.
	opened := regexp.MustCompile(`picture: (opened the file|moved|went on), at frame \d+, [\d.]+ s, for 160x90`)
	first := regexp.MustCompile(`picture: first frame (\d+) .*, (\d+) decoded`)
	type jumped struct {
		how     string
		decoded int
	}
	// The decoder says what it did after it handed the frames over, so
	// the test reads on each time it says more. A decoder that never says
	// it is go test's -timeout to end.
	var heard []jumped
	for {
		mu.Lock()
		heard = heard[:0]
		for _, line := range lines {
			if m := opened.FindStringSubmatch(line); m != nil {
				heard = append(heard, jumped{how: m[1], decoded: -1})
			} else if m := first.FindStringSubmatch(line); m != nil && len(heard) > 0 {
				heard[len(heard)-1].decoded, _ = strconv.Atoi(m[2])
			}
		}
		mu.Unlock()
		if len(heard) == len(jumps) && heard[len(heard)-1].decoded >= 0 {
			break
		}
		<-said
	}
	if len(heard) != len(jumps) {
		t.Fatalf("the decoder said %d jumps, want %d: %v", len(heard), len(jumps), heard)
	}
	for i, j := range jumps {
		if heard[i].decoded < 0 {
			t.Errorf("the jump to %.2f s did not say how many frames it decoded", j.from)
		}
		if heard[i].how != j.how {
			t.Errorf("the jump to %.2f s %s, want %s", j.from, heard[i].how, j.how)
		}
	}
	// From the key frame at 0 to 1.92 s are 48 frames, two in three of
	// them B-frames that nothing is made from: about 16 are needed.
	if d := heard[6].decoded; d > 30 {
		t.Errorf("the jump to 1.92 s decoded %d frames to the first, want the B-frames before it skipped", d)
	}
}

// A stream told to skip hands over no frame before the moment it names,
// beyond the ones the decoder was asked for already: a jump forward in an
// open stream costs the decoding of the frames on the way, not making
// pictures of them that are thrown away. The frame at the moment is still
// the one the ffmpeg program hands over. Plan row 2.269.
//
// On the real disk: ffmpeg makes the episode and the frames decoder reads it.
func TestTheEpisodesDecoderMakesNoFrameBeforeTheSkip(t *testing.T) {
	ffmpegtest.Need(t)
	program := os.Getenv("FRAMEFAIRY_FRAMES")
	if program == "" {
		ffmpegtest.Unusable(t, "FRAMEFAIRY_FRAMES names no framefairy-frames, which make frames builds")
	}
	path := filepath.Join(t.TempDir(), "episode.mp4")
	if out, err := exec.Command("ffmpeg", "-loglevel", "error", "-y", "-f", "lavfi",
		"-i", "testsrc2=s=320x180:r=25:d=4", "-c:v", "mpeg4", "-q:v", "3", "-g", "50", "-bf", "2", path).CombinedOutput(); err != nil {
		ffmpegtest.Unusable(t, "ffmpeg could not make an episode: %s %s", err, out)
	}
	e := ffmpegOnTheProcessor()
	dec := NewEpisodeFrames(program, path)
	defer dec.Close()
	grid := gridOf(t, path)
	// Two frames from 0.2 s, then a jump to 1.6 s, before the key frame at
	// 2 s, in the same stream, the way a word jump reads on.
	const to = 1.6
	var skip atomic.Uint64
	var ats []float64
	var at []byte
	err := dec.Stream(context.Background(), grid, 0.2, 160, 90, false, func() float64 {
		return math.Float64frombits(skip.Load())
	}, func(a float64, data []byte) error {
		ats = append(ats, a)
		if len(ats) == 2 {
			skip.Store(math.Float64bits(grid.At(grid.Holding(to)) - 0.02))
		}
		if a >= to-0.02 {
			at = data
			return errStop
		}
		return nil
	})
	if err != nil && !errors.Is(err, errStop) {
		t.Fatal(err)
	}
	if last := ats[len(ats)-1]; math.Abs(last-grid.At(grid.Holding(to))) > 1e-6 {
		t.Fatalf("the stream went on to %.4f s, want the frame at %.4f s", last, grid.At(grid.Holding(to)))
	}
	// The two taken, the frames asked for before the skip was known, and
	// at most a batch more that was asked for as the skip was set. Without
	// the skip all 35 frames from 0.2 s come.
	if n := len(ats) - 1; n > 2+2*ahead {
		t.Errorf("the stream handed over %d frames before %.1f s, want no more than %d: %v", n, to, 2+2*ahead, ats)
	}
	var want []byte
	if err := e.PreviewFrames(context.Background(), path, grid.At(grid.Holding(to)), 160, 90, func(_ float64, data []byte) error {
		want = data
		return errStop
	}); err != nil && !errors.Is(err, errStop) {
		t.Fatal(err)
	}
	if d := meanDiff(at, want); d > 2 {
		t.Errorf("the frame at %.1f s differs from ffmpeg's by %.2f on average", to, d)
	}
}

// A stream that asks for the key frame first hands over the key frame its
// decoding starts from, the same picture the ffmpeg program makes of it,
// and goes on from there to the frame asked for once the skip names it,
// with no more than a batch of frames between. A frame that is itself a
// key frame comes at once. HEVC with open GOPs too, where a seek finds its
// key frame by when it is decoded. Issue #281.
//
// On the real disk: ffmpeg makes the episode and the frames decoder reads it.
func TestTheEpisodesDecoderHandsOverTheKeyFrameFirst(t *testing.T) {
	ffmpegtest.Need(t)
	program := os.Getenv("FRAMEFAIRY_FRAMES")
	if program == "" {
		ffmpegtest.Unusable(t, "FRAMEFAIRY_FRAMES names no framefairy-frames, which make frames builds")
	}
	// codecs are the encoders to make the episode with, the next one
	// only when the one before failed, see encodeTestFile.
	type kind struct {
		name   string
		codecs [][]string
	}
	kinds := []kind{{"MPEG-4", [][]string{{"-c:v", "mpeg4", "-q:v", "3", "-g", "50", "-bf", "2"}}}}
	// Our own ffmpeg on the Mac has no x265, which is GPL, and makes the
	// HEVC with the Mac's encoder, which writes no open GOPs.
	encoders, _ := exec.Command("ffmpeg", "-hide_banner", "-encoders").Output()
	has := func(name string) bool { return strings.Contains(string(encoders), " "+name+" ") }
	switch {
	case has("libx265"):
		kinds = append(kinds, kind{"HEVC with open GOPs", [][]string{{"-c:v", "libx265", "-preset", "ultrafast",
			"-x265-params", "keyint=50:min-keyint=50:scenecut=0:bframes=3:open-gop=1:log-level=error"}}})
	case has("hevc_videotoolbox"):
		kinds = append(kinds, kind{"HEVC", chipThenProcessor([]string{"-c:v", "hevc_videotoolbox", "-b:v", "2M", "-g", "50", "-tag:v", "hvc1"})})
	default:
		// Ends only this case, so MPEG-4 still runs.
		kinds = append(kinds, kind{"HEVC", nil})
	}
	for _, enc := range kinds {
		t.Run(enc.name, func(t *testing.T) {
			t.Parallel()
			if enc.codecs == nil {
				ffmpegtest.Unusable(t, "this ffmpeg has no HEVC encoder to make the episode with")
			}
			path := filepath.Join(t.TempDir(), "episode.mp4")
			// Four seconds at 25 frames a second, a key frame every two.
			args := []string{"-loglevel", "error", "-y", "-f", "lavfi", "-i", "testsrc2=s=320x180:r=25:d=4"}
			if out, err := encodeTestFile(t, "ffmpeg", args, enc.codecs, path); err != nil {
				ffmpegtest.Unusable(t, "ffmpeg could not make an episode: %s %s", err, out)
			}
			e := ffmpegOnTheProcessor()
			// How far apart on average the frames compared may be. On a
			// Mac the episode's decoder decodes HEVC on the chip, and the
			// ffmpeg it is compared with on the processor, issue #581.
			// The chip's scaler and the processor's round differently, see
			// TestTheEpisodesDecoderPlaysTheFilesPeopleBring.
			apart := 2.0
			if runtime.GOOS == "darwin" && strings.HasPrefix(enc.name, "HEVC") {
				apart = 6
			}
			grid := gridOf(t, path)
			ffmpegs := func(at float64) []byte {
				var want []byte
				if err := e.PreviewFrames(context.Background(), path, at, 160, 90, func(_ float64, data []byte) error {
					want = data
					return errStop
				}); err != nil && !errors.Is(err, errStop) {
					t.Fatal(err)
				}
				return want
			}
			// The key frames as ffprobe finds them: with B-frames the
			// second is not shown at 2 s but a frame or so later.
			out, err := exec.Command("ffprobe", "-v", "error", "-select_streams", "v", "-skip_frame", "nokey",
				"-show_entries", "frame=pts_time", "-of", "csv=p=0", path).Output()
			if err != nil {
				t.Fatal(err)
			}
			var keys []float64
			for _, l := range strings.Fields(string(out)) {
				k, err := strconv.ParseFloat(strings.TrimSuffix(l, ","), 64)
				if err != nil {
					t.Fatalf("ffprobe said %q", out)
				}
				keys = append(keys, k)
			}
			if len(keys) < 2 || keys[1] < 1.5 || keys[1] > 2.5 {
				t.Fatalf("the key frames are at %v, want one at 0 and one at about 2 s", keys)
			}
			// Before the second key frame, after it, and onto it. Each on a
			// decoder of its own, since a cursor that reads on to the frame
			// needs no seek and hands over the frame asked for, see the
			// decoder's open.
			for _, c := range []struct{ from, key float64 }{{keys[1] - 0.4, keys[0]}, {keys[1] + 1, keys[1]}, {keys[1], keys[1]}} {
				dec := NewEpisodeFrames(program, path)
				defer dec.Close()
				to := grid.At(grid.Holding(c.from))
				var skip atomic.Uint64
				var ats []float64
				var first, at []byte
				err := dec.Stream(context.Background(), grid, c.from, 160, 90, true, func() float64 {
					return math.Float64frombits(skip.Load())
				}, func(a float64, data []byte) error {
					ats = append(ats, a)
					if len(ats) == 1 {
						first = data
						skip.Store(math.Float64bits(to - 0.02))
					}
					if a >= to-1e-6 {
						at = data
						return errStop
					}
					return nil
				})
				if err != nil && !errors.Is(err, errStop) {
					t.Fatal(err)
				}
				if key := grid.At(grid.Holding(c.key)); math.Abs(ats[0]-key) > 1e-6 {
					t.Fatalf("a stream from %.1f s began at %.4f s, want the key frame at %.4f s", c.from, ats[0], key)
				}
				if last := ats[len(ats)-1]; math.Abs(last-to) > 1e-6 {
					t.Fatalf("a stream from %.1f s went on to %.4f s, want the frame at %.4f s", c.from, last, to)
				}
				// The frames between the key frame and the frame asked for
				// are never wanted, so none is made into a picture, issue
				// #281: the frame asked for comes right after the key frame.
				if n := len(ats) - 2; n > 0 {
					t.Errorf("a stream from %.1f s handed over %d frames between the key frame and the frame asked for, want none: %v", c.from, n, ats)
				}
				if d := meanDiff(first, ffmpegs(ats[0])); d > apart {
					t.Errorf("the key frame at %.1f s differs from ffmpeg's by %.2f on average", ats[0], d)
				}
				if d := meanDiff(at, ffmpegs(to)); d > apart {
					t.Errorf("the frame at %.1f s differs from ffmpeg's by %.2f on average", to, d)
				}
			}
		})
	}
}

// The decoder of an episode open in the video preview is started before
// any frame is asked for, with its cursors on the file, so the first frame
// asked for only moves one. Plan row 2.156.
//
// On the real disk: ffmpeg makes the episode and the frames decoder reads it.
func TestTheEpisodesDecoderIsReadyBeforeTheFirstFrame(t *testing.T) {
	ffmpegtest.Need(t)
	program := os.Getenv("FRAMEFAIRY_FRAMES")
	if program == "" {
		ffmpegtest.Unusable(t, "FRAMEFAIRY_FRAMES names no framefairy-frames, which make frames builds")
	}
	path := filepath.Join(t.TempDir(), "episode.mp4")
	if out, err := exec.Command("ffmpeg", "-loglevel", "error", "-y", "-f", "lavfi",
		"-i", "testsrc2=s=320x180:r=25:d=4", "-c:v", "mpeg4", path).CombinedOutput(); err != nil {
		ffmpegtest.Unusable(t, "ffmpeg could not make an episode: %s %s", err, out)
	}
	dec := NewEpisodeFrames(program, path)
	defer dec.Close()
	if err := dec.Ready(); err != nil {
		t.Fatal(err)
	}
	// The cursors open while nothing is asked. Should they never, the
	// test hangs here until go test's -timeout.
	untilCursors(dec, func() bool { return len(dec.idle[pictureCursor]) == cursorsKept[pictureCursor] })
	times := NewPreviewTimes()
	err := dec.Stream(WithPreviewTimes(context.Background(), times), gridOf(t, path), 1.5, 160, 90, false, nil, func(float64, []byte) error { return errStop })
	if !errors.Is(err, errStop) {
		t.Fatal(err)
	}
	if times.File.Load() {
		t.Error("the first frame asked for opened the file, want a cursor opened before moved")
	}
	dec.mu.Lock()
	opened := dec.nextID
	dec.mu.Unlock()
	// The cursors of picture and the one of sound.
	if want := uint32(cursorsKept[pictureCursor] + 1); opened != want {
		t.Errorf("%d cursors were opened, want the %d opened before the first frame and no more", opened, want)
	}
}

var errStop = errors.New("enough")

// ffmpegOnTheProcessor is an engine whose ffmpeg decodes on the
// processor, for the frames a test compares the episode's decoder with.
// On GitHub's virtual Mac a decode through VideoToolbox can hang for good,
// issue #581, and the frames to compare with are not what these tests
// are about.
//
// On the real disk: ffmpeg reads the episode the test made.
func ffmpegOnTheProcessor() *Engine {
	e := NewEngine(NewLog(nil, false, false), disk)
	e.FFmpeg, e.FFprobe = "ffmpeg", "ffprobe"
	e.softDecode.Store(true)
	return e
}

// gridOf is the grid of an episode, the frames the episode's decoder
// counts on, from the engine's probe.
//
// On the real disk: ffprobe reads the episode.
func gridOf(t *testing.T, path string) framewire.Grid {
	t.Helper()
	e := ffmpegOnTheProcessor()
	info, err := e.Probe(context.Background(), path)
	if err != nil {
		t.Fatal(err)
	}
	return info.Grid()
}

// meanDiff is how far apart two frames made by framewire.Picture are on
// average, colour by colour, in steps of 8 bits.
func meanDiff(a, b []byte) float64 {
	if len(a) != len(b) {
		return math.Inf(1)
	}
	ca, cb := colours(a), colours(b)
	sum := 0
	for i := range ca {
		d := ca[i] - cb[i]
		if d < 0 {
			d = -d
		}
		sum += d
	}
	return float64(sum) / 4 / float64(len(ca))
}

// Everything the Go side does to the episode's decoder at once: streams
// from many places on several goroutines, some called off at once, some
// after a few frames, some read to their end, while the decoder is closed
// and started again under them. Every stream either hands over frames in
// order or ends with a reason, none hangs, and none gets another's frames.
//
// On the real disk: ffmpeg makes the episode and the frames decoder reads it.
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
	grid := gridOf(t, path)
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
				err := dec.Stream(ctx, grid, from, 64, 36, false, nil, func(at float64, data []byte) error {
					if at < grid.At(grid.Holding(from))-1e-6 || at <= last {
						t.Errorf("a stream from %.1f got a frame at %.3f after %.3f", from, at, last)
					}
					if len(data) != 64*36*4 {
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
	// Streams that never end are go test's -timeout to end.
	wg.Wait()
}

// The files people bring, H.264 and HEVC with 10-bit colour, come through
// the episode's decoder from wherever they are opened, with the frames the
// ffmpeg program hands over. On a Mac they are decoded on the graphics
// chip, brought out as they are and scaled on the processor by the same
// chain as everywhere else. The first build of the decoder, which scaled
// on the chip as well, failed on every such file on Tim's Mac the moment
// an episode opened, "the picture could not be scaled, its chain: Invalid
// argument", and the tests had only an MPEG-4 file, which no Mac decodes
// on its chip. Plan row 2.156.
//
// On the real disk: ffmpeg makes the episodes and the frames decoder reads them.
func TestTheEpisodesDecoderPlaysTheFilesPeopleBring(t *testing.T) {
	ffmpegtest.Need(t)
	program := os.Getenv("FRAMEFAIRY_FRAMES")
	if program == "" {
		ffmpegtest.Unusable(t, "FRAMEFAIRY_FRAMES names no framefairy-frames, which make frames builds")
	}
	encoders, _ := exec.Command("ffmpeg", "-hide_banner", "-encoders").Output()
	has := func(name string) bool { return strings.Contains(string(encoders), " "+name+" ") }
	// codecs are the encoders to make the episode with, the next one
	// only when the one before failed, see encodeTestFile.
	type kind struct {
		name   string
		codecs [][]string
	}
	var kinds []kind
	switch {
	case has("h264_videotoolbox"):
		kinds = append(kinds, kind{"H.264", chipThenProcessor([]string{"-c:v", "h264_videotoolbox", "-b:v", "2M", "-g", "50"})})
	case has("libx264"):
		kinds = append(kinds, kind{"H.264", [][]string{{"-c:v", "libx264", "-preset", "ultrafast", "-g", "50"}}})
	default:
		ffmpegtest.Unusable(t, "this ffmpeg has no H.264 encoder to make an episode with")
	}
	switch {
	case has("hevc_videotoolbox"):
		kinds = append(kinds, kind{"HEVC 10-bit", chipThenProcessor([]string{"-c:v", "hevc_videotoolbox", "-profile:v", "main10", "-pix_fmt", "p010le", "-b:v", "2M", "-g", "50", "-tag:v", "hvc1"})})
	case has("libx265"):
		kinds = append(kinds, kind{"HEVC 10-bit", [][]string{{"-c:v", "libx265", "-preset", "ultrafast", "-pix_fmt", "yuv420p10le", "-x265-params", "keyint=50:log-level=error", "-tag:v", "hvc1"}}})
	}
	e := ffmpegOnTheProcessor()
	for _, k := range kinds {
		path := filepath.Join(t.TempDir(), "episode.mp4")
		args := []string{"-loglevel", "error", "-y", "-f", "lavfi", "-i", "testsrc2=s=640x360:r=25:d=6"}
		if out, err := encodeTestFile(t, "ffmpeg", args, k.codecs, path); err != nil {
			if k.name == "H.264" {
				ffmpegtest.Unusable(t, "ffmpeg could not make an %s episode: %s %s", k.name, err, out)
			}
			// A virtual Mac may have no HEVC encoder that takes 10 bits.
			t.Logf("no %s episode here: %s", k.name, out)
			continue
		}
		dec := NewEpisodeFrames(program, path)
		grid := gridOf(t, path)
		for _, from := range []float64{0, 2.3, 4.97} {
			var got, want []float64
			var gotData, wantData [][]byte
			collect := func(ats *[]float64, data *[][]byte) func(float64, []byte) error {
				return func(at float64, frame []byte) error {
					*ats = append(*ats, at)
					*data = append(*data, frame)
					if len(*ats) == 6 {
						return errStop
					}
					return nil
				}
			}
			if err := dec.Stream(context.Background(), grid, from, 160, 90, false, nil, collect(&got, &gotData)); err != nil && !errors.Is(err, errStop) {
				t.Fatalf("%s from %.2f: %v", k.name, from, err)
			}
			if err := e.PreviewFrames(context.Background(), path, grid.At(grid.Holding(from)), 160, 90, collect(&want, &wantData)); err != nil && !errors.Is(err, errStop) {
				t.Fatalf("%s from %.2f, the ffmpeg program: %v", k.name, from, err)
			}
			if len(got) == 0 || len(got) != len(want) {
				t.Fatalf("%s from %.2f: the decoder gave %d frames, the ffmpeg program %d", k.name, from, len(got), len(want))
			}
			for i := range want {
				if math.Abs(got[i]-want[i]) > 1e-6 {
					t.Fatalf("%s from %.2f: frame %d at %.4f, the ffmpeg program's at %.4f", k.name, from, i, got[i], want[i])
				}
				// The chip's scaler and the processor's round differently.
				if d := meanDiff(gotData[i], wantData[i]); d > 6 {
					t.Fatalf("%s from %.2f: frame %d differs from the ffmpeg program's by %.2f on average", k.name, from, i, d)
				}
			}
		}
		dec.Close()
	}
}

// A file whose picture starts after its sound, the way an export from
// DaVinci Resolve starts 44 ms in, and one that starts late as a whole,
// come through the episode's decoder on the grid the engine counts, from
// the start of the file: from before the picture starts, from inside its
// first frame and from further on, each frame of the grid the file's own
// frame that holds its moment, and its first before the picture begins.
// Times count from the start of the file, as the engine and the sound
// count them, so on the second file a frame the file stamps 0.3 s is at
// nought. The decoder once counted by the file's own stamps there, 0.3 s
// off the sound and the clips.
//
// On the real disk: ffmpeg makes the episodes and the frames decoder reads them.
func TestTheEpisodesDecoderAgreesOnAPictureThatStartsLate(t *testing.T) {
	ffmpegtest.Need(t)
	program := os.Getenv("FRAMEFAIRY_FRAMES")
	if program == "" {
		ffmpegtest.Unusable(t, "FRAMEFAIRY_FRAMES names no framefairy-frames, which make frames builds")
	}
	// The picture and the sound made apart and put together late, the way
	// only a copy of them keeps it.
	dir := t.TempDir()
	mux := func(args ...string) {
		t.Helper()
		if out, err := exec.Command("ffmpeg", append([]string{"-loglevel", "error", "-y"}, args...)...).CombinedOutput(); err != nil {
			ffmpegtest.Unusable(t, "ffmpeg could not make an episode: %s %s", err, out)
		}
	}
	picture, sound := filepath.Join(dir, "picture.mp4"), filepath.Join(dir, "sound.m4a")
	mux("-f", "lavfi", "-i", "testsrc2=s=320x180:r=25:d=6", "-c:v", "mpeg4", "-q:v", "3", "-g", "25", picture)
	mux("-f", "lavfi", "-i", "sine=f=440:sample_rate=48000:d=6", "-c:a", "aac", sound)
	pictureLate, allLate := filepath.Join(dir, "picture-late.mp4"), filepath.Join(dir, "all-late.mp4")
	mux("-itsoffset", "0.3", "-i", picture, "-i", sound, "-map", "0:v", "-map", "1:a", "-c", "copy", pictureLate)
	mux("-itsoffset", "0.3", "-i", picture, "-itsoffset", "0.3", "-i", sound, "-map", "0:v", "-map", "1:a", "-c", "copy", allLate)
	e := ffmpegOnTheProcessor()
	type frame struct {
		at   float64
		data []byte
	}
	for _, path := range []string{pictureLate, allLate} {
		name := filepath.Base(path)
		info, err := e.Probe(context.Background(), path)
		if err != nil {
			t.Fatal(err)
		}
		// Where the file starts on its own clock: its sound's priming
		// starts the second 21 ms before its picture.
		c := struct {
			path          string
			origin, start float64
		}{path, info.origin, info.VideoStart}
		if math.Abs(c.origin+c.start-0.3) > 1e-6 || c.start < 0.02 {
			t.Fatalf("%s starts at %.3f with its picture %.3f after, want the picture at 0.3 and after the start", name, c.origin, c.start)
		}
		grid := info.Grid()
		// Every frame of the file, decoded from its start by the ffmpeg
		// program, which keeps each at the moment the file stamps it.
		var all []frame
		if err := e.PreviewFrames(context.Background(), c.path, 0, 160, 90, func(at float64, data []byte) error {
			all = append(all, frame{at - c.origin, data})
			return nil
		}); err != nil {
			t.Fatalf("%s, the ffmpeg program: %v", name, err)
		}
		if len(all) != 150 || math.Abs(all[0].at-c.start) > 1e-6 {
			t.Fatalf("%s has %d frames from %.3f, want 150 from %.3f", name, len(all), all[0].at, c.start)
		}
		holds := func(at float64) frame {
			k := 0
			for k+1 < len(all) && all[k+1].at <= at+framewire.Hair {
				k++
			}
			return all[k]
		}
		// The frames a stream handed over, each at its moment of the grid
		// and the file's frame there: frame first and those after it, or
		// with the key frame first, frame key and then those from first.
		checkFrames := func(t *testing.T, what string, got []frame, key, first int) {
			t.Helper()
			for i, g := range got {
				moment := grid.At(first + i)
				if key < first {
					moment = grid.At(first + i - 1)
					if i == 0 {
						moment = grid.At(key)
					}
				}
				if math.Abs(g.at-moment) > 1e-9 {
					t.Fatalf("%s: frame %d at %.4f, its moment is %.4f", what, i, g.at, moment)
				}
				want := holds(moment)
				if d := meanDiff(g.data, want.data); d > 2 {
					t.Fatalf("%s: frame %d, at %.4f, differs from the file's frame at %.4f by %.2f on average",
						what, i, moment, want.at, d)
				}
			}
			if len(got) != 6 {
				t.Fatalf("%s: %d frames, want 6", what, len(got))
			}
		}
		dec := NewEpisodeFrames(program, c.path)
		for _, from := range []float64{0, 0.2, 0.31, 0.35, 1.0, 1.02, 2.3} {
			// With the key frame first too, issue #281: the first frame is
			// then the key frame, which stands for no frame before the
			// grid's first even where the picture starts late, and the
			// frame asked for and those after it follow.
			for _, key := range []bool{false, true} {
				first := grid.Holding(from)
				k := first
				var got []frame
				err := dec.Stream(context.Background(), grid, from, 160, 90, key, nil, func(at float64, data []byte) error {
					got = append(got, frame{at, data})
					if len(got) == 6 {
						return errStop
					}
					return nil
				})
				if err != nil && !errors.Is(err, errStop) {
					t.Fatalf("%s from %.2f, key %v: %v", name, from, key, err)
				}
				if key && len(got) > 0 {
					k = grid.Holding(got[0].at)
					if k < 0 || k > first || math.Abs(got[0].at-grid.At(k)) > 1e-9 {
						t.Fatalf("%s from %.2f: the key frame at %.4f is frame %d, want a frame from 0 to %d at its moment", name, from, got[0].at, k, first)
					}
				}
				checkFrames(t, fmt.Sprintf("%s from %.2f, key %v", name, from, key), got, k, first)
			}
		}
		dec.Close()
	}
}

// In an open GOP, the way HEVC encoders write it and DaVinci Resolve wrote
// Tim's start.mp4, the frames shown just before a key frame are decoded
// after it and need the GOP before. The episode's decoder gives each of
// them from its own moment, and the ones after. A seek lands on a key
// frame by when it is decoded, so it landed on the key frame those frames
// lead up to, the decoder dropped them, and the first frame came a key
// frame late: the frame asked for never came, and start.mp4 stood without
// a picture and would not play. Plan row 2.186.
//
// The file is in testdata so that the Mac, whose own ffmpeg has no x265,
// reads it too:
//
//	ffmpeg -f lavfi -i testsrc2=s=320x180:r=25:d=4 -c:v libx265 -preset ultrafast \
//		-pix_fmt yuv420p10le -x265-params keyint=24:min-keyint=24:bframes=3:scenecut=0:crf=40 \
//		-tag:v hvc1 -an open-gop.mp4
func TestTheEpisodesDecoderGivesTheFramesBeforeAnOpenKeyFrame(t *testing.T) {
	ffmpegtest.Need(t)
	program := os.Getenv("FRAMEFAIRY_FRAMES")
	if program == "" {
		ffmpegtest.Unusable(t, "FRAMEFAIRY_FRAMES names no framefairy-frames, which make frames builds")
	}
	path := filepath.Join("testdata", "open-gop.mp4")
	// The frames shown before the key frame they are decoded after, read
	// from the file's packets in the order they are decoded.
	out, err := exec.Command("ffprobe", "-v", "error", "-select_streams", "v:0",
		"-show_entries", "packet=pts_time,flags", "-of", "csv=p=0", path).Output()
	if err != nil {
		ffmpegtest.Unusable(t, "ffprobe could not read %s: %v", path, err)
	}
	var shown []float64
	var leading []float64
	key := math.Inf(-1)
	for _, line := range strings.Fields(string(out)) {
		var at float64
		var flags string
		if _, err := fmt.Sscanf(strings.Replace(line, ",", " ", 1), "%g %s", &at, &flags); err != nil {
			t.Fatalf("a packet ffprobe read: %q: %v", line, err)
		}
		shown = append(shown, at)
		if strings.HasPrefix(flags, "K") {
			key = at
		} else if at < key {
			leading = append(leading, at)
		}
	}
	if len(leading) < 3 {
		t.Fatalf("%s has %d frames shown before their key frame, not an open GOP", path, len(leading))
	}
	slices.Sort(shown)
	dec := NewEpisodeFrames(program, path)
	defer dec.Close()
	grid := gridOf(t, path)
	for _, from := range leading {
		var got []float64
		err := dec.Stream(context.Background(), grid, from, 160, 90, false, nil, func(at float64, _ []byte) error {
			got = append(got, at)
			if len(got) == 4 {
				return errStop
			}
			return nil
		})
		if err != nil && !errors.Is(err, errStop) {
			t.Fatalf("from %.3f: %v", from, err)
		}
		i := slices.IndexFunc(shown, func(at float64) bool { return math.Abs(at-from) < 1e-6 })
		want := shown[i:min(i+4, len(shown))]
		if len(got) != len(want) {
			t.Fatalf("from %.3f: frames at %v, want %v", from, got, want)
		}
		for j := range want {
			if math.Abs(got[j]-want[j]) > 1e-6 {
				t.Fatalf("from %.3f: frames at %v, want %v", from, got, want)
			}
		}
	}
}

// The episode's decoder hears what the ffmpeg program hears for the render
// and the transcript, sample by sample: from the start, from a moment, from
// before the picture starts in a file whose picture starts late, and from
// further on, at the episode's rate and at another, in one channel and in
// two. Read by the same rule, soundSeek, cut at the same sample. Plan row
// 2.156, step 3 of docs/VIDEO-PREVIEW.md.
//
// On the real disk: ffmpeg makes the episodes and the frames decoder reads them.
func TestTheEpisodesDecoderHearsWhatFFmpegDoes(t *testing.T) {
	ffmpegtest.Need(t)
	program := os.Getenv("FRAMEFAIRY_FRAMES")
	if program == "" {
		ffmpegtest.Unusable(t, "FRAMEFAIRY_FRAMES names no framefairy-frames, which make frames builds")
	}
	dir := t.TempDir()
	mux := func(args ...string) {
		t.Helper()
		if out, err := exec.Command("ffmpeg", append([]string{"-loglevel", "error", "-y"}, args...)...).CombinedOutput(); err != nil {
			ffmpegtest.Unusable(t, "ffmpeg could not make an episode: %s %s", err, out)
		}
	}
	// Tones whose pitch climbs, so a sample out of place is a sample that
	// differs. No noise: AAC carries noise as a band to make noise in, and
	// a decoder makes it up afresh, differently from wherever it started.
	picture, sound := filepath.Join(dir, "picture.mp4"), filepath.Join(dir, "sound.m4a")
	mux("-f", "lavfi", "-i", "testsrc2=s=160x90:r=25:d=8", "-c:v", "mpeg4", "-g", "25", picture)
	mux("-f", "lavfi", "-i", "aevalsrc=0.4*sin(2*PI*(200+60*t)*t)+0.1*sin(2*PI*(1500+300*t)*t)|0.3*sin(2*PI*330*t):s=48000:d=8",
		"-c:a", "aac", "-b:a", "192k", sound)
	plain, late := filepath.Join(dir, "plain.mp4"), filepath.Join(dir, "late.mp4")
	mux("-i", picture, "-i", sound, "-map", "0:v", "-map", "1:a", "-c", "copy", plain)
	mux("-itsoffset", "0.3", "-i", picture, "-i", sound, "-map", "0:v", "-map", "1:a", "-c", "copy", late)
	e := ffmpegOnTheProcessor()
	collect := func(run func(got func(float64, []byte) error) error) ([]float64, []byte) {
		var ats []float64
		var all []byte
		err := run(func(at float64, chunk []byte) error {
			ats = append(ats, at)
			all = append(all, chunk...)
			if len(ats) == 24 {
				return errStop
			}
			return nil
		})
		if err != nil && !errors.Is(err, errStop) {
			t.Fatal(err)
		}
		return ats, all
	}
	for _, path := range []string{plain, late} {
		dec := NewEpisodeFrames(program, path)
		for _, from := range []float64{0, 0.1, 0.25, 1.337, 4.02} {
			for _, form := range [][2]int{{48000, 2}, {44100, 1}} {
				rate, channels := form[0], form[1]
				name := fmt.Sprintf("%s from %.3f at %d Hz in %d", filepath.Base(path), from, rate, channels)
				wantAt, want := collect(func(got func(float64, []byte) error) error {
					return e.PreviewSound(context.Background(), path, from, rate, channels, got)
				})
				gotAt, got := collect(func(got func(float64, []byte) error) error {
					return dec.Sound(context.Background(), e, from, rate, channels, got)
				})
				if len(gotAt) != len(wantAt) || len(got) != len(want) {
					t.Fatalf("%s: %d chunks of %d bytes, the ffmpeg program %d of %d", name, len(gotAt), len(got), len(wantAt), len(want))
				}
				for i := range wantAt {
					if math.Abs(gotAt[i]-wantAt[i]) > 1e-9 {
						t.Fatalf("%s: chunk %d at %.6f, the ffmpeg program's at %.6f", name, i, gotAt[i], wantAt[i])
					}
				}
				worst := 0.0
				for i := 0; i+4 <= len(want); i += 4 {
					a := math.Float32frombits(binary.LittleEndian.Uint32(got[i:]))
					b := math.Float32frombits(binary.LittleEndian.Uint32(want[i:]))
					worst = max(worst, math.Abs(float64(a-b)))
				}
				if worst > 1e-4 {
					t.Errorf("%s: a sample differs from the ffmpeg program's by %.6f", name, worst)
				}
			}
		}
		dec.Close()
	}
}

// A cursor that read to the end of the episode hands over frames again
// from anywhere else. The chain it told the episode had ended was kept,
// because open asked whether it was told only after the seek had
// forgotten, so every stream after it on that cursor ended at once and
// the video preview stayed black. Plan row 2.166.
//
// On the real disk: ffmpeg makes the episode and the frames decoder reads it.
func TestTheEpisodesDecoderComesBackFromTheEnd(t *testing.T) {
	ffmpegtest.Need(t)
	program := os.Getenv("FRAMEFAIRY_FRAMES")
	if program == "" {
		ffmpegtest.Unusable(t, "FRAMEFAIRY_FRAMES names no framefairy-frames, which make frames builds")
	}
	path := filepath.Join(t.TempDir(), "episode.mp4")
	if out, err := exec.Command("ffmpeg", "-loglevel", "error", "-y", "-f", "lavfi",
		"-i", "testsrc2=s=320x180:r=25:d=4", "-c:v", "mpeg4", "-g", "25", path).CombinedOutput(); err != nil {
		ffmpegtest.Unusable(t, "ffmpeg could not make an episode: %s %s", err, out)
	}
	dec := NewEpisodeFrames(program, path)
	defer dec.Close()
	grid := gridOf(t, path)
	count := func(from float64, most int) int {
		n := 0
		err := dec.Stream(context.Background(), grid, from, 160, 90, false, nil, func(float64, []byte) error {
			n++
			if n == most {
				return errStop
			}
			return nil
		})
		if err != nil && !errors.Is(err, errStop) {
			t.Fatalf("from %.2f: %v", from, err)
		}
		return n
	}
	// The cursor a stream gives back is the next one taken, so each stream
	// after one that read to the end runs on the cursor that did.
	for round := range 3 {
		if n := count(3.5, 0); n == 0 {
			t.Fatalf("round %d: the stream to the end gave no frames", round)
		}
		if n := count(1, 10); n != 10 {
			t.Fatalf("round %d: after the end, a stream from 1 s gave %d frames, want 10", round, n)
		}
	}
}

// The frames of the episode's decoder are colours ffmpeg made from the
// file's own tags, value by value what the ffmpeg program makes of the
// same frame, for files tagged BT.601, BT.709 and BT.2020, in video and in
// full range, in 8 and 10 bits, and for a file that says nothing, where
// both take ffmpeg's defaults. And the tags count: the same picture tagged
// otherwise comes out in other colours. Plan row 2.156, step 4.
//
// On the real disk: ffmpeg makes the episodes and the frames decoder reads them.
func TestTheEpisodesDecoderMakesFFmpegsColours(t *testing.T) {
	ffmpegtest.Need(t)
	program := os.Getenv("FRAMEFAIRY_FRAMES")
	if program == "" {
		ffmpegtest.Unusable(t, "FRAMEFAIRY_FRAMES names no framefairy-frames, which make frames builds")
	}
	encoders, _ := exec.Command("ffmpeg", "-hide_banner", "-encoders").Output()
	has := func(name string) bool { return strings.Contains(string(encoders), " "+name+" ") }
	// Our own ffmpeg on the Mac has no x264 or x265, which are GPL, and
	// makes the files with the Mac's encoders instead.
	h264 := []string{"-c:v", "libx264", "-preset", "ultrafast"}
	hevc := []string{"-c:v", "libx265", "-preset", "ultrafast", "-x265-params", "log-level=error"}
	software := true
	switch {
	case has("libx264"):
	case has("h264_videotoolbox"):
		h264 = []string{"-c:v", "h264_videotoolbox", "-b:v", "2M"}
		hevc = []string{"-c:v", "hevc_videotoolbox", "-profile:v", "main10", "-b:v", "2M"}
		software = false
	default:
		ffmpegtest.Unusable(t, "this ffmpeg has no H.264 encoder to make the files with")
	}
	type file struct {
		name string
		args []string
	}
	// The tags are set on the frames rather than asked of the encoder. A
	// newer ffmpeg, ours among them, takes an encoder's tags from its
	// frames and drops -color_trc and the like, so a file made with those
	// came out with no tags at all from our own ffmpeg, 9, and only the
	// ffmpeg of a Linux distribution, 6, wrote them.
	tags := func(space, primaries, transfer, rng string) []string {
		return []string{"-vf", fmt.Sprintf("setparams=colorspace=%s:color_primaries=%s:color_trc=%s:range=%s", space, primaries, transfer, rng)}
	}
	x264 := append(append([]string{}, h264...), "-pix_fmt", "yuv420p")
	files := []file{
		{"BT.601 video range", append(append([]string{}, x264...), tags("smpte170m", "smpte170m", "smpte170m", "tv")...)},
		{"BT.601 full range", append(append([]string{}, x264...), tags("smpte170m", "smpte170m", "smpte170m", "pc")...)},
		{"BT.709 video range", append(append([]string{}, x264...), tags("bt709", "bt709", "bt709", "tv")...)},
		{"BT.709 full range", append(append([]string{}, x264...), tags("bt709", "bt709", "bt709", "pc")...)},
		{"no tags", x264},
	}
	if has("libx265") || !software && has("hevc_videotoolbox") {
		tenBits := "yuv420p10le"
		if !software {
			tenBits = "p010le"
		}
		x265 := append(append([]string{}, hevc...), "-pix_fmt", tenBits, "-tag:v", "hvc1")
		files = append(files,
			file{"BT.709 video range, 10 bits", append(append([]string{}, x265...), tags("bt709", "bt709", "bt709", "tv")...)},
			file{"BT.2020 video range, 10 bits", append(append([]string{}, x265...), tags("bt2020nc", "bt2020", "bt709", "tv")...)},
			file{"BT.2020 full range, 10 bits", append(append([]string{}, x265...), tags("bt2020nc", "bt2020", "bt709", "pc")...)},
			file{"HLG, 10 bits", append(append([]string{}, x265...), tags("bt2020nc", "bt2020", "arib-std-b67", "tv")...)},
			file{"PQ, 10 bits", append(append([]string{}, x265...), tags("bt2020nc", "bt2020", "smpte2084", "tv")...)},
		)
	}
	const w, h = 64, 36
	dir := t.TempDir()
	got := map[string][]byte{}
	for i, f := range files {
		path := filepath.Join(dir, fmt.Sprintf("%d.mp4", i))
		// The same numbers in every file, only the tags differ, so what
		// tells two files' colours apart is what the tags say.
		args := []string{"-loglevel", "error", "-y", "-f", "lavfi", "-i", "testsrc2=s=320x180:r=25:d=1"}
		codecs := [][]string{f.args}
		if !software {
			codecs = chipThenProcessor(f.args)
		}
		if out, err := encodeTestFile(t, "ffmpeg", args, codecs, path); err != nil {
			if strings.Contains(f.name, "10 bits") {
				// A virtual Mac may have no HEVC encoder that takes 10 bits.
				t.Logf("no file %s here: %s", f.name, out)
				continue
			}
			ffmpegtest.Unusable(t, "ffmpeg could not make the file %s: %s %s", f.name, err, out)
		}
		dec := NewEpisodeFrames(program, path)
		// Where the decoder decoded, which it says as it opens and again
		// when it gives the chip up.
		var whereMu sync.Mutex
		onChip := false
		dec.heard = func(_ slog.Level, line string) {
			whereMu.Lock()
			defer whereMu.Unlock()
			switch {
			case strings.Contains(line, "decodes on the graphics chip"):
				onChip = true
			case strings.Contains(line, "decodes on the processor"):
				onChip = false
			}
		}
		var frame []byte
		err := dec.Stream(context.Background(), gridOf(t, path), 0, w, h, false, nil, func(_ float64, data []byte) error {
			frame = data
			return errStop
		})
		light := dec.Light()
		dec.Close()
		whereMu.Lock()
		chip := onChip
		whereMu.Unlock()
		if err != nil && !errors.Is(err, errStop) {
			t.Fatalf("%s: %v", f.name, err)
		}
		// HDR stays in the file's own curve with 10 bits a colour, and the
		// decoder says which curve, so the page can turn it into light.
		wantLight := framewire.SDR
		switch {
		case strings.HasPrefix(f.name, "HLG"):
			wantLight = framewire.HLG
		case strings.HasPrefix(f.name, "PQ"):
			wantLight = framewire.PQ
		}
		if light != wantLight {
			t.Errorf("%s: the decoder says its light is %q, want %q", f.name, light, wantLight)
		}
		// The ffmpeg program decodes the way the decoder did: on the Mac's
		// graphics chip where the decoder said it did, with the frame
		// brought out as it is. On the processor as well, the way the
		// render decodes, which is said and not held to here: on the Mac's
		// runner the two came out up to 22 apart at the edges of colours,
		// 0.2 % on average, which only the video preview shows and is left
		// as it is, issue #256. A decoder that gave the chip up, as it
		// does when the chip hangs, is compared with the processor, and
		// the ffmpeg program never asks the chip that hung: it waited on
		// it for 3m25s on CI's Mac, issue #570.
		convert := func(hw bool) []byte {
			t.Helper()
			args := []string{"-loglevel", "error"}
			if hw {
				args = append(args, "-hwaccel", "videotoolbox")
			}
			args = append(args, "-i", path, "-frames:v", "1",
				"-vf", framewire.Picture(w, h), "-f", "rawvideo", "-")
			out, err := exec.Command("ffmpeg", args...).Output()
			if err != nil {
				t.Fatalf("%s, the ffmpeg program: %v", f.name, err)
			}
			return out
		}
		want := convert(chip)
		if chip {
			cpu := convert(false)
			t.Logf("%s: the chip's frame and the processor's are %.2f apart on average", f.name, meanDiff(want, cpu))
		}
		if len(frame) != w*h*4 || len(want) != w*h*4 {
			t.Fatalf("%s: a frame of %d bytes and the ffmpeg program's of %d, want %d", f.name, len(frame), len(want), w*h*4)
		}
		// Value by value, red, green and blue. The ffmpeg program in a test
		// may be another version than the one the decoder is built on, and
		// two versions of its scaler can round a value one apart, in 8 bits
		// or in 10.
		mine, theirs := colours(frame), colours(want)
		most, at := 0, 0
		for i := range mine {
			d := mine[i] - theirs[i]
			if d < 0 {
				d = -d
			}
			if d > most {
				most, at = d, i/3
			}
		}
		// 2 in 8 bits is 8 in 10.
		if most > 8 {
			t.Errorf("%s: at pixel %d the decoder's colour is %v, the ffmpeg program's %v, in 10 bits", f.name, at, mine[at*3:at*3+3], theirs[at*3:at*3+3])
		}
		got[f.name] = frame
	}
	differ := func(a, b string) {
		t.Helper()
		if got[a] == nil || got[b] == nil {
			return
		}
		if d := meanDiff(got[a], got[b]); d < 1 {
			t.Errorf("%s and %s come out in the same colours, %.2f apart on average: a tag was not read", a, b, d)
		}
	}
	differ("BT.601 video range", "BT.709 video range")
	differ("BT.709 video range", "BT.709 full range")
	differ("BT.709 video range, 10 bits", "BT.2020 video range, 10 bits")
}

// colours are the red, green and blue of every pixel of a frame made by
// framewire.Picture, 10 bits each.
func colours(frame []byte) []int {
	out := make([]int, 0, len(frame)/4*3)
	for p := 0; p+4 <= len(frame); p += 4 {
		v := binary.LittleEndian.Uint32(frame[p:])
		out = append(out, int(v&1023), int(v>>10&1023), int(v>>20&1023))
	}
	return out
}

// A decoder that stops at once, the way it does for a file with no
// picture, says why it stopped to a stream that asked while it was
// stopping. Wait shuts its input before the reason is known, and a stream
// that asked in between said "file already closed" instead of the reason,
// so Add left out a file of sound for the wrong reason. Plan row 2.171.
// Here the decoder's end is held between the two until the stream has
// asked, which it has once it took a cursor.
//
// On the real disk: the decoder is a program, a script here.
func TestADecoderThatStopsAtOnceSaysWhyToAStreamThatAskedMeanwhile(t *testing.T) {
	program := filepath.Join(t.TempDir(), "frames")
	script := "#!/bin/sh\necho 'the file has no picture' >&2\nexit 1\n"
	if err := standin.Write(program, []byte(script), 0o755); err != nil {
		t.Fatal(err)
	}
	dec := NewEpisodeFrames(program, "nur-ton.mp4")
	defer dec.Close()
	waited, release := make(chan struct{}), make(chan struct{})
	var once sync.Once
	dec.waited = func() {
		once.Do(func() { close(waited) })
		<-release
	}
	defer func() {
		select {
		case <-release:
		default:
			close(release)
		}
	}()
	if err := dec.Ready(); err != nil {
		t.Fatal(err)
	}
	<-waited
	dec.mu.Lock()
	before := dec.nextID
	dec.mu.Unlock()
	asked := make(chan error, 1)
	go func() {
		asked <- dec.Stream(context.Background(), framewire.Grid{Num: 1, Den: 1, Count: 1}, 0, 64, 36, false, nil,
			func(float64, []byte) error { return nil })
	}()
	var err error
wait:
	for {
		dec.mu.Lock()
		took := dec.nextID != before
		taken := dec.cursors.WaitLocked()
		dec.mu.Unlock()
		if took {
			// It took a cursor and asked: the end goes on.
			close(release)
			err = <-asked
			break
		}
		select {
		case err = <-asked:
			// It gave up without waiting for the reason.
			break wait
		case <-taken:
		}
	}
	if err == nil || !strings.Contains(err.Error(), "has no picture") {
		t.Errorf("the stream said %v, want the decoder's reason, that the file has no picture", err)
	}
}

// untilCursors goes on once ok says yes, and asks it again each time the
// decoder's cursors change, never on the clock. ok is asked with dec.mu
// held.
func untilCursors(dec *EpisodeFrames, ok func() bool) {
	dec.mu.Lock()
	defer dec.mu.Unlock()
	for !ok() {
		changed := dec.cursors.WaitLocked()
		dec.mu.Unlock()
		<-changed
		dec.mu.Lock()
	}
}
