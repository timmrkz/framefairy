package engine

import (
	"context"
	"encoding/binary"
	"errors"
	"fmt"
	"math"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"sync"
	"testing"
	"time"

	"framefairy/internal/ffmpegtest"
)

// The episode's decoder hands over the frames the ffmpeg program hands
// over, from any moment, with the same moments and the same pictures, and
// a stream after the first moves a cursor that is open rather than opening
// the file again.
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
	for i, from := range []float64{0, 2.1, 7.39, 3.98} {
		want := collect(func(got func(float64, []byte) error) error {
			return e.PreviewFrames(context.Background(), path, from, 160, 90, got)
		})
		times := NewPreviewTimes()
		got := collect(func(got func(float64, []byte) error) error {
			return dec.Stream(WithPreviewTimes(context.Background(), times), from, 160, 90, got)
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

// The decoder of an episode open in the video preview is started before
// any frame is asked for, with its cursors on the file, so the first frame
// asked for only moves one. Plan row 2.156.
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
	until := time.Now().Add(10 * time.Second)
	for {
		dec.mu.Lock()
		kept := len(dec.idle[pictureCursor])
		dec.mu.Unlock()
		if kept == cursorsKept[pictureCursor] {
			break
		}
		if time.Now().After(until) {
			t.Fatalf("%d cursors open after ten seconds, want %d", kept, cursorsKept[pictureCursor])
		}
		time.Sleep(10 * time.Millisecond)
	}
	times := NewPreviewTimes()
	err := dec.Stream(WithPreviewTimes(context.Background(), times), 1.5, 160, 90, func(float64, []byte) error { return errStop })
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

// A cursor that played to the end of the episode is handed to the next
// stream like any other, and that stream gets its frames: a drag to the
// end of the range picker and back left every cursor at the end, and the
// video preview black until the app was started again.
func TestTheEpisodesDecoderComesBackFromTheEnd(t *testing.T) {
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
	// All kept, so each stream takes the cursor kept last, which is the
	// one the stream before it ended on.
	kept := func(round int) {
		until := time.Now().Add(10 * time.Second)
		for {
			dec.mu.Lock()
			n := len(dec.idle[pictureCursor])
			dec.mu.Unlock()
			if n == cursorsKept[pictureCursor] {
				return
			}
			if time.Now().After(until) {
				t.Fatalf("round %d: %d cursors kept, want %d", round, n, cursorsKept[pictureCursor])
			}
			time.Sleep(5 * time.Millisecond)
		}
	}
	kept(-1)
	for round := range 3 {
		ended := 0
		if err := dec.Stream(context.Background(), 3.7, 160, 90, func(float64, []byte) error { ended++; return nil }); err != nil {
			t.Fatal(err)
		}
		if ended == 0 {
			t.Fatalf("round %d: the stream to the end gave no frame", round)
		}
		kept(round)
		var got []float64
		err := dec.Stream(context.Background(), 1, 160, 90, func(at float64, _ []byte) error {
			got = append(got, at)
			if len(got) == 5 {
				return errStop
			}
			return nil
		})
		if !errors.Is(err, errStop) {
			t.Fatalf("round %d: the stream after the end gave %d frames and %v, want 5", round, len(got), err)
		}
		if math.Abs(got[0]-1) > 1e-6 {
			t.Fatalf("round %d: the stream after the end started at %.3f, want 1", round, got[0])
		}
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

// The files people bring, H.264 and HEVC with 10-bit colour, come through
// the episode's decoder from wherever they are opened, with the frames the
// ffmpeg program hands over. On a Mac they are decoded on the graphics
// chip, brought out as they are and scaled on the processor by the same
// chain as everywhere else. The first build of the decoder, which scaled
// on the chip as well, failed on every such file on Tim's Mac the moment
// an episode opened, "the picture could not be scaled, its chain: Invalid
// argument", and the tests had only an MPEG-4 file, which no Mac decodes
// on its chip. Plan row 2.156.
func TestTheEpisodesDecoderPlaysTheFilesPeopleBring(t *testing.T) {
	ffmpegtest.Need(t)
	program := os.Getenv("FRAMEFAIRY_FRAMES")
	if program == "" {
		ffmpegtest.Unusable(t, "FRAMEFAIRY_FRAMES names no framefairy-frames, which make frames builds")
	}
	encoders, _ := exec.Command("ffmpeg", "-hide_banner", "-encoders").Output()
	has := func(name string) bool { return strings.Contains(string(encoders), " "+name+" ") }
	type kind struct {
		name string
		args []string
	}
	var kinds []kind
	switch {
	case has("h264_videotoolbox"):
		kinds = append(kinds, kind{"H.264", []string{"-c:v", "h264_videotoolbox", "-b:v", "2M", "-g", "50"}})
	case has("libx264"):
		kinds = append(kinds, kind{"H.264", []string{"-c:v", "libx264", "-preset", "ultrafast", "-g", "50"}})
	default:
		ffmpegtest.Unusable(t, "this ffmpeg has no H.264 encoder to make an episode with")
	}
	switch {
	case has("hevc_videotoolbox"):
		kinds = append(kinds, kind{"HEVC 10-bit", []string{"-c:v", "hevc_videotoolbox", "-profile:v", "main10", "-pix_fmt", "p010le", "-b:v", "2M", "-g", "50", "-tag:v", "hvc1"}})
	case has("libx265"):
		kinds = append(kinds, kind{"HEVC 10-bit", []string{"-c:v", "libx265", "-preset", "ultrafast", "-pix_fmt", "yuv420p10le", "-x265-params", "keyint=50:log-level=error", "-tag:v", "hvc1"}})
	}
	e := NewEngine(NewLog(nil, false, false))
	e.FFmpeg, e.FFprobe = "ffmpeg", "ffprobe"
	for _, k := range kinds {
		path := filepath.Join(t.TempDir(), "episode.mp4")
		args := append([]string{"-loglevel", "error", "-y", "-f", "lavfi", "-i", "testsrc2=s=640x360:r=25:d=6"}, k.args...)
		if out, err := exec.Command("ffmpeg", append(args, path)...).CombinedOutput(); err != nil {
			if k.name == "H.264" {
				ffmpegtest.Unusable(t, "ffmpeg could not make an %s episode: %s %s", k.name, err, out)
			}
			// A virtual Mac may have no HEVC encoder that takes 10 bits.
			t.Logf("no %s episode here: %s", k.name, out)
			continue
		}
		dec := NewEpisodeFrames(program, path)
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
			if err := dec.Stream(context.Background(), from, 160, 90, collect(&got, &gotData)); err != nil && !errors.Is(err, errStop) {
				t.Fatalf("%s from %.2f: %v", k.name, from, err)
			}
			if err := e.PreviewFrames(context.Background(), path, from, 160, 90, collect(&want, &wantData)); err != nil && !errors.Is(err, errStop) {
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
// come through the episode's decoder with the file's own frames at the
// file's own moments, from before the picture starts, from inside its
// first frame and from further on: from any moment, the first frame that
// starts there or after, and the ones after it. What a seek of the
// ffmpeg program gives is not the measure here: its -ss counts from the
// file's start, so on the second file it starts 0.276 s late.
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
	e := NewEngine(NewLog(nil, false, false))
	e.FFmpeg, e.FFprobe = "ffmpeg", "ffprobe"
	type frame struct {
		at   float64
		data []byte
	}
	for _, path := range []string{pictureLate, allLate} {
		name := filepath.Base(path)
		// Every frame of the file, decoded from its start by the ffmpeg
		// program, which keeps each at the moment the file gives it.
		var all []frame
		if err := e.PreviewFrames(context.Background(), path, 0, 160, 90, func(at float64, data []byte) error {
			all = append(all, frame{at, data})
			return nil
		}); err != nil {
			t.Fatalf("%s, the ffmpeg program: %v", name, err)
		}
		if len(all) != 150 || math.Abs(all[0].at-0.3) > 1e-6 {
			t.Fatalf("%s has %d frames from %.3f, want 150 from 0.3", name, len(all), all[0].at)
		}
		dec := NewEpisodeFrames(program, path)
		for _, from := range []float64{0, 0.2, 0.31, 0.35, 1.0, 1.02, 2.3} {
			k := 0
			for k < len(all) && all[k].at < from-1e-6 {
				k++
			}
			var got []frame
			err := dec.Stream(context.Background(), from, 160, 90, func(at float64, data []byte) error {
				got = append(got, frame{at, data})
				if len(got) == 6 {
					return errStop
				}
				return nil
			})
			if err != nil && !errors.Is(err, errStop) {
				t.Fatalf("%s from %.2f: %v", name, from, err)
			}
			for i, g := range got {
				want := all[k+i]
				if math.Abs(g.at-want.at) > 1e-6 {
					t.Fatalf("%s from %.2f: frame %d at %.4f, want %.4f", name, from, i, g.at, want.at)
				}
				if d := meanDiff(g.data, want.data); d > 2 {
					t.Fatalf("%s from %.2f: frame %d differs from the file's by %.2f on average", name, from, i, d)
				}
			}
			if len(got) != 6 {
				t.Fatalf("%s from %.2f: %d frames, want 6", name, from, len(got))
			}
		}
		dec.Close()
	}
}

// The episode's decoder hears what the ffmpeg program hears for the render
// and the transcript, sample by sample: from the start, from a moment, from
// before the picture starts in a file whose picture starts late, and from
// further on, at the episode's rate and at another, in one channel and in
// two. Read by the same rule, soundSeek, cut at the same sample. Plan row
// 2.156, step 3 of docs/VIDEO-PREVIEW.md.
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
	e := NewEngine(NewLog(nil, false, false))
	e.FFmpeg, e.FFprobe = "ffmpeg", "ffprobe"
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
