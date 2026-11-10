package engine

import (
	"bytes"
	"context"
	"encoding/binary"
	"errors"
	"log/slog"
	"math"
	"math/rand/v2"
	"os"
	"strings"
	"sync"
	"testing"

	"framefairy/engine/runlog"
	"framefairy/internal/ffmpegtest"
)

// The video preview shows the frames of the short: the episode's decoder
// hands over the grid the engine names, see SourceInfo.Grid, and at every
// frame of it the frame of the episode the render takes there, the one
// that holds its moment. On the episodes TestARenderShowsTheFrameThatHoldsEachMoment
// renders, frames at uneven times, frames left out, a picture that starts
// after the sound, on files whose frames keep to their rate, in MP4 and
// in Matroska, on what OBS records, H.264 with B-frames and AAC in
// Matroska, on MPEG-TS, which has no index of its key frames, so a jump
// lands between two, and on a Matroska recording cut off by a crash,
// which does not say how long it is, plan row 2.203. The video preview
// counts on the same grid and reads nothing of the file itself, so a
// Matroska file plays as an MP4 does. Plan row 2.198,
// docs/VIDEO-PREVIEW.md step 5.
//
// On the real disk: the frames decoder reads the episode.
func TestTheVideoPreviewShowsTheFramesOfTheShort(t *testing.T) {
	ffmpegtest.Need(t)
	program := os.Getenv("FRAMEFAIRY_FRAMES")
	if program == "" {
		ffmpegtest.Unusable(t, "FRAMEFAIRY_FRAMES names no framefairy-frames, which make frames builds")
	}
	e := NewEngine(runlog.New(&bytes.Buffer{}, false, false), NewMachine(disk))
	e.FFmpeg, e.FFprobe = "ffmpeg", "ffprobe"
	for _, c := range timesCases {
		t.Run(c.name, func(t *testing.T) {
			t.Parallel()
			previewShowsTheShort(t, e, program, c, "")
		})
	}
}

// The video preview goes on when the graphics chip fails to decode a
// frame, which VideoToolbox does now and then on a virtual Mac: CI's
// failed one run in four, "hardware accelerator failed to decode
// picture", on what OBS records and on MPEG-TS, and the cursor failed with
// it. The episode's decoder is made to fail on the third packet of every
// open, the first stream and every jump, whatever it decodes on, and has
// to say so and go on with the processor from the frame it is at. The
// decoder of a test decodes on the processor, see Media, so this proves
// the reopen, under the race detector on Linux, and not the chip's own
// way to it, which only a Mac handed its media engine takes. Plan row
// 2.215.
//
// On the real disk: the frames decoder reads the episode.
func TestTheVideoPreviewGoesOnWhenTheGraphicsChipFails(t *testing.T) {
	ffmpegtest.Need(t)
	program := os.Getenv("FRAMEFAIRY_FRAMES")
	if program == "" {
		ffmpegtest.Unusable(t, "FRAMEFAIRY_FRAMES names no framefairy-frames, which make frames builds")
	}
	e := NewEngine(runlog.New(&bytes.Buffer{}, false, false), NewMachine(disk))
	e.FFmpeg, e.FFprobe = "ffmpeg", "ffprobe"
	for _, c := range timesCases {
		if c.name == "obs_mkv" || c.name == "mpegts" {
			t.Run(c.name, func(t *testing.T) {
				t.Parallel()
				previewShowsTheShort(t, e, program, c, "FRAMEFAIRY_FRAMES_FAIL_AFTER=3")
			})
		}
	}
}

// The video preview goes on when the decoder hangs for good, as the
// graphics chip did on CI's virtual Mac while it streamed the recording
// cut off by a crash, issue #553: the stream waited on it until the test
// run was killed. The decoder, on the processor as a test's is, is made
// to hang on one packet of one cursor,
// says so at once, gives the cursor up and decodes on the
// processor from then on, and the stream goes on from a new cursor at the
// frame after the last one it had. The frames are checked as in
// TestTheVideoPreviewShowsTheFramesOfTheShort, so none is lost or doubled
// on the way.
//
// On the real disk: the frames decoder reads the episode.
func TestTheVideoPreviewGoesOnWhenTheDecoderHangs(t *testing.T) {
	ffmpegtest.Need(t)
	program := os.Getenv("FRAMEFAIRY_FRAMES")
	if program == "" {
		ffmpegtest.Unusable(t, "FRAMEFAIRY_FRAMES names no framefairy-frames, which make frames builds")
	}
	e := NewEngine(runlog.New(&bytes.Buffer{}, false, false), NewMachine(disk))
	e.FFmpeg, e.FFprobe = "ffmpeg", "ffprobe"
	// On the 3rd packet the decoder hangs in the open, before the stream
	// has a frame, and on the 12th in the middle of a stream, after the
	// first frames were handed over, so the new cursor goes on from the
	// frame after the last one.
	for _, c := range timesCases {
		if c.name == "even_mp4" || c.name == "crashed_mkv" {
			for _, after := range []string{"3", "12"} {
				t.Run(c.name+"/"+after, func(t *testing.T) {
					t.Parallel()
					previewShowsTheShort(t, e, program, c, "FRAMEFAIRY_FRAMES_HANG_AFTER="+after)
				})
			}
		}
	}
}

// previewShowsTheShort is TestTheVideoPreviewShowsTheFramesOfTheShort on
// the episode of one case, with the decoder made to fail in every open or
// to hang once where made says so.
func previewShowsTheShort(t *testing.T, e *Engine, program string, c timesCase, made string) {
	t.Helper()
	source := timesEpisode(t, c)
	info, err := e.Probe(context.Background(), source)
	if err != nil {
		t.Fatal(err)
	}
	grid := info.Grid()
	if got := info.FPSString(); c.rate != "" && got != c.rate {
		t.Errorf("the grid has %s frames a second, the short %s", got, c.rate)
	}
	if info.SoundRate != 48000 || info.SoundChannels != 1 {
		t.Errorf("the sound is %d Hz in %d channels, want 48000 in 1", info.SoundRate, info.SoundChannels)
	}
	// The grid starts inside the file, at most a frame before the
	// picture, and reaches its end.
	frame := float64(grid.Den) / float64(grid.Num)
	if k := (info.VideoStart - grid.Start) / frame; grid.Start < 0 || grid.Start >= frame || math.Abs(k-math.Round(k)) > 1e-6 {
		t.Errorf("the grid starts at %.4f, the picture at %.4f", grid.Start, info.VideoStart)
	}
	if end := grid.At(grid.Count); end < info.Duration-1e-6 || end-frame >= info.Duration {
		t.Errorf("the grid ends at %.4f, the file at %.4f", end, info.Duration)
	}
	// The file is as long as it was made, one that does not say
	// how long it is too.
	if made := float64(max(c.seconds, 7)); info.Duration < made-0.1 || info.Duration > made+0.3 {
		t.Errorf("the file is %.3f s long, it was made %.0f s long", info.Duration, made)
	}
	holds := frameHolding(t, source)
	dec := newEpisodeFrames(program, source)
	defer dec.Close()
	var heardMu sync.Mutex
	var heard []string
	dec.heard = func(_ slog.Level, line string) {
		heardMu.Lock()
		heard = append(heard, line)
		heardMu.Unlock()
	}
	failing, hanging := strings.Contains(made, "FAIL_AFTER"), strings.Contains(made, "HANG_AFTER")
	if made != "" {
		dec.env = []string{made}
	}
	streams := 0
	const w, h = 64, 36
	// The whole grid, from the start.
	var got []int
	streams++
	err = dec.Stream(context.Background(), grid, 0, w, h, false, nil, func(at float64, data []byte) error {
		r := len(got)
		if math.Abs(at-grid.At(r)) > 1e-9 {
			t.Fatalf("frame %d of the grid came at %.6f, its moment is %.6f", r, at, grid.At(r))
		}
		got = append(got, previewBars(data, w, h))
		return nil
	})
	if err != nil {
		t.Fatal(err)
	}
	if len(got) != grid.Count {
		heardMu.Lock()
		said := strings.Join(heard, "\n")
		heardMu.Unlock()
		t.Fatalf("the decoder gave %d frames, the grid has %d, and said:\n%s", len(got), grid.Count, said)
	}
	wrong := 0
	for r, n := range got {
		if want := holds(grid.At(r)); n != want {
			if wrong++; wrong <= 6 {
				t.Errorf("frame %d of the grid, %.4f s, shows frame %d of the episode, the one that holds it is %d",
					r, grid.At(r), n, want)
			}
		}
	}
	if wrong > 6 {
		t.Errorf("and %d more frames are the wrong frame", wrong-6)
	}
	// From a jump to the edges of pieces, and inside a frame: the
	// frame that holds the moment and the ones after it. And after the
	// last key frame of the file, where a seek in MPEG-TS found no key
	// before the end and the decoder showed nothing, issue #287.
	for _, from := range []float64{0.2, 1.25, 3.05, 3.3, 5.3, 2.0021, 6.5} {
		first := grid.Holding(from)
		var near []int
		streams++
		err := dec.Stream(context.Background(), grid, from, w, h, false, nil, func(at float64, data []byte) error {
			if r := first + len(near); math.Abs(at-grid.At(r)) > 1e-9 {
				t.Fatalf("from %.4f frame %d came at %.6f, want %.6f", from, len(near), at, grid.At(r))
			}
			near = append(near, previewBars(data, w, h))
			if len(near) == 5 {
				return errStop
			}
			return nil
		})
		if err != nil && !errors.Is(err, errStop) {
			t.Fatalf("from %.4f: %v", from, err)
		}
		if want := min(5, grid.Count-first); len(near) != want {
			t.Errorf("from %.4f the decoder gave %d frames, want %d", from, len(near), want)
		}
		for i, n := range near {
			if want := holds(grid.At(first + i)); n != want {
				t.Errorf("from %.4f, frame %d of the grid shows %d, the one that holds it is %d", from, first+i, n, want)
			}
		}
	}
	// What the decoder said, all of it in once it stopped. Every time the
	// chip failed is in the test's log with ffmpeg's reason, and the
	// frames above say whether the processor read the file right.
	dec.Close()
	heardMu.Lock()
	defer heardMu.Unlock()
	fellBack, gaveUp := 0, 0
	for _, line := range heard {
		if strings.Contains(line, "decodes on the processor for the rest of this open") {
			fellBack++
			t.Logf("%s", line)
		}
		if strings.Contains(line, "gives it up and decodes on the processor from now on") {
			gaveUp++
			t.Logf("%s", line)
		}
	}
	if failing && fellBack < streams {
		t.Errorf("the decoder went on with the processor %d times in %d streams, made to fail in each", fellBack, streams)
	}
	if hanging && gaveUp != 1 {
		t.Errorf("the decoder gave up on %d cursors, made to hang once", gaveUp)
	}
	// Nothing made to hang, nothing is given up.
	if !hanging && gaveUp != 0 {
		t.Errorf("the decoder gave up on %d cursors, made to hang in none", gaveUp)
	}
}

// previewBars is the number the eight bars of a frame of the episode's
// decoder say, read from its red, the lowest 10 bits of each pixel, see
// framewire.Picture.
func previewBars(frame []byte, w, h int) int {
	n := 0
	for b := 0; b < 8; b++ {
		at := ((2*b+1)*h/16*w + w/2) * 4
		if binary.LittleEndian.Uint32(frame[at:])&0x3ff > 512 {
			n |= 1 << (7 - b)
		}
	}
	return n
}

// Every frame of a short stands for a moment the video preview has a
// frame for: the render's moments and the video preview's grid are the
// same moments, so the frame the render takes for one and the frame the
// episode's decoder hands over for it are the same frame of the episode.
// TestARenderShowsTheFrameThatHoldsEachMoment and
// TestTheVideoPreviewShowsTheFramesOfTheShort check that each side shows
// the frame that holds its moment. This checks that the moments agree, on
// every frame of clips placed all over videos of every kind of rate and
// start, so a walk need not click through the frames of a short to find
// it out. A walk only looks at the app showing the frame at a clip's
// edges.
func TestEveryFrameOfAShortIsAFrameOfTheVideoPreview(t *testing.T) {
	videos := []struct {
		name string
		info SourceInfo
	}{
		{"25 fps", SourceInfo{FPSNum: 25, FPSDen: 1, Duration: 60}},
		{"29.97 fps", SourceInfo{FPSNum: 30000, FPSDen: 1001, Duration: 60}},
		{"29.97 fps, the picture a quarter second late", SourceInfo{FPSNum: 30000, FPSDen: 1001, VideoStart: 0.25, Duration: 60}},
		{"30 fps, the picture 4 ms late", SourceInfo{FPSNum: 30, FPSDen: 1, VideoStart: 0.004, Duration: 60.01}},
		{"23.976 fps, a length inside a frame", SourceInfo{FPSNum: 24000, FPSDen: 1001, VideoStart: 0.021, Duration: 59.99}},
		{"60 fps", SourceInfo{FPSNum: 60, FPSDen: 1, VideoStart: 0.0333, Duration: 60}},
	}
	for _, v := range videos {
		t.Run(v.name, func(t *testing.T) {
			grid := v.info.Grid()
			frame := float64(v.info.FPSDen) / float64(v.info.FPSNum)
			// Clips of three pieces, their edges kept to the millisecond
			// the way a plan keeps them, from before the picture starts
			// to past the video's end.
			r := rand.New(rand.NewPCG(1, 2))
			for c := range 200 {
				var segs []Segment
				at := -0.3 + r.Float64()*58
				for range 3 {
					start := math.Round(at*1000) / 1000
					end := math.Round((start+0.05+r.Float64()*3)*1000) / 1000
					segs = append(segs, Segment{Start: start, End: end})
					at = end + 0.05 + r.Float64()*2
				}
				clip := v.info.OnFrames(Clip{Segments: segs})
				for p, seg := range clip.Segments {
					n := int(math.Round(seg.Duration() / frame))
					for q := range n {
						moment := seg.Start + float64(q)*frame
						k := grid.Holding(moment)
						if math.Abs(grid.At(k)-moment) > 1e-6 {
							t.Fatalf("clip %d, piece %d, frame %d of the short stands for %.6f s, the video preview's frame there is %d at %.6f s",
								c, p+1, q, moment, k, grid.At(k))
						}
					}
				}
			}
		})
	}
}
