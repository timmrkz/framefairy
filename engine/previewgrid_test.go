package engine

import (
	"bytes"
	"context"
	"encoding/binary"
	"errors"
	"math"
	"os"
	"testing"

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
func TestTheVideoPreviewShowsTheFramesOfTheShort(t *testing.T) {
	ffmpegtest.Need(t)
	program := os.Getenv("FRAMEFAIRY_FRAMES")
	if program == "" {
		ffmpegtest.Unusable(t, "FRAMEFAIRY_FRAMES names no framefairy-frames, which make frames builds")
	}
	e := NewEngine(NewLog(&bytes.Buffer{}, false, false))
	e.FFmpeg, e.FFprobe = "ffmpeg", "ffprobe"
	for _, c := range timesCases {
		t.Run(c.name, func(t *testing.T) {
			t.Parallel()
			previewShowsTheShort(t, e, program, c)
		})
	}
}

// The video preview goes on when the graphics chip fails to decode a
// frame, which VideoToolbox does now and then on a virtual Mac: CI's
// failed one run in four on what OBS records, "hardware accelerator
// failed to decode picture", and the cursor failed with it. The episode's
// decoder is made to fail on the chip after ten frames of every cursor,
// and goes on with the processor from the frame it is at. Only a Mac has
// the chip, elsewhere this is the test above again.
func TestTheVideoPreviewGoesOnWhenTheGraphicsChipFails(t *testing.T) {
	ffmpegtest.Need(t)
	program := os.Getenv("FRAMEFAIRY_FRAMES")
	if program == "" {
		ffmpegtest.Unusable(t, "FRAMEFAIRY_FRAMES names no framefairy-frames, which make frames builds")
	}
	// Not parallel: the decoder reads this from the environment, which
	// the whole test process shares.
	t.Setenv("FRAMEFAIRY_FRAMES_CHIP_FAILS", "10")
	e := NewEngine(NewLog(&bytes.Buffer{}, false, false))
	e.FFmpeg, e.FFprobe = "ffmpeg", "ffprobe"
	for _, c := range timesCases {
		if c.name == "obs_mkv" || c.name == "mpegts" {
			t.Run(c.name, func(t *testing.T) { previewShowsTheShort(t, e, program, c) })
		}
	}
}

// previewShowsTheShort is TestTheVideoPreviewShowsTheFramesOfTheShort on
// the episode of one case.
func previewShowsTheShort(t *testing.T, e *Engine, program string, c timesCase) {
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
	dec := NewEpisodeFrames(program, source)
	defer dec.Close()
	const w, h = 64, 36
	// The whole grid, from the start.
	var got []int
	err = dec.Stream(context.Background(), grid, 0, w, h, func(at float64, data []byte) error {
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
		t.Fatalf("the decoder gave %d frames, the grid has %d", len(got), grid.Count)
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
	// frame that holds the moment and the ones after it.
	for _, from := range []float64{0.2, 1.25, 3.05, 3.3, 5.3, 2.0021} {
		first := grid.Holding(from)
		var near []int
		err := dec.Stream(context.Background(), grid, from, w, h, func(at float64, data []byte) error {
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
		for i, n := range near {
			if want := holds(grid.At(first + i)); n != want {
				t.Errorf("from %.4f, frame %d of the grid shows %d, the one that holds it is %d", from, first+i, n, want)
			}
		}
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
