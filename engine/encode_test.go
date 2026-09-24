package engine

import (
	"context"
	"io"
	"os"
	"path/filepath"
	"runtime"
	"strconv"
	"strings"
	"testing"
)

// A fake ffmpeg that answers -encoders with whatever the test wants, so the
// choice can be driven without an ffmpeg that has the encoder in question.
// The one on this machine has libx264 and no videotoolbox, and the one on
// Tim's has both, and neither of those is a test.
func fakeFFmpeg(t *testing.T, encoders ...string) string {
	t.Helper()
	if runtime.GOOS == "windows" {
		t.Skip("the fake is a shell script")
	}
	var rows strings.Builder
	rows.WriteString("Encoders:\\n V..... = Video\\n ------\\n")
	for _, name := range encoders {
		// The real listing is flags, then the name, then a description.
		rows.WriteString(" V....D " + name + " " + name + " encoder (codec h264)\\n")
	}
	path := filepath.Join(t.TempDir(), "ffmpeg")
	script := "#!/bin/sh\nprintf '" + rows.String() + "'\n"
	if err := os.WriteFile(path, []byte(script), 0o755); err != nil {
		t.Fatal(err)
	}
	return path
}

func encodingEngine(t *testing.T, encoders ...string) *Engine {
	t.Helper()
	e := &Engine{Log: NewLog(io.Discard, false, false), FFmpeg: fakeFFmpeg(t, encoders...)}
	return e
}

func TestTheEncoderIsChosenFromWhatThisFFmpegHas(t *testing.T) {
	ctx := context.Background()

	t.Run("an ffmpeg with nothing we can drive says so", func(t *testing.T) {
		e := encodingEngine(t, "wmv2", "mpeg4")
		_, err := e.VideoEncoder(ctx)
		if err == nil {
			t.Fatal("took an ffmpeg with no H.264 encoder")
		}
		if !strings.Contains(err.Error(), "libx264") {
			t.Errorf("the message does not say what it looked for: %v", err)
		}
	})

	t.Run("the name in the settings wins", func(t *testing.T) {
		e := encodingEngine(t, "libx264", "h264_videotoolbox")
		e.WantEncoder = "libx264"
		got, err := e.VideoEncoder(ctx)
		if err != nil {
			t.Fatal(err)
		}
		if got.Name != "libx264" {
			t.Errorf("chose %s over the one that was asked for", got.Name)
		}
	})

	t.Run("a name this ffmpeg does not have is refused", func(t *testing.T) {
		e := encodingEngine(t, "libx264")
		e.WantEncoder = "h264_nvenc"
		if _, err := e.VideoEncoder(ctx); err == nil {
			t.Error("took an encoder that is not there")
		}
	})

	// A description mentioning another encoder must not count as having it.
	// ffmpeg's own listing is full of lines like "libx264rgb H.264 ...".
	t.Run("a name is matched in its own column", func(t *testing.T) {
		e := encodingEngine(t, "libx264rgb")
		e.WantEncoder = "libx264"
		if _, err := e.VideoEncoder(ctx); err == nil {
			t.Error("libx264rgb was taken for libx264")
		}
	})
}

func TestEachEncoderIsAskedInItsOwnLanguage(t *testing.T) {
	ctx := context.Background()
	rs := RenderSettings{CRF: 18, Preset: "slow"}

	t.Run("libx264 keeps the preset and the crf", func(t *testing.T) {
		e := encodingEngine(t, "libx264")
		got, err := e.VideoArgs(ctx, rs)
		if err != nil {
			t.Fatal(err)
		}
		want := []string{"-c:v", "libx264", "-preset", "slow", "-crf", "18"}
		if strings.Join(got, " ") != strings.Join(want, " ") {
			t.Errorf("got %v", got)
		}
	})

	// VideoToolbox has neither, and passing them is not a warning, it is a
	// render that fails. -q:v runs the other way round from CRF, so the
	// mapping is checked in both directions rather than for one number.
	t.Run("videotoolbox is asked with q:v and never with crf", func(t *testing.T) {
		e := encodingEngine(t, "h264_videotoolbox")
		e.WantEncoder = "h264_videotoolbox"
		got, err := e.VideoArgs(ctx, rs)
		if err != nil {
			t.Fatal(err)
		}
		line := strings.Join(got, " ")
		if strings.Contains(line, "-crf") || strings.Contains(line, "-preset") {
			t.Fatalf("passed an x264 idea to Apple's encoder: %v", got)
		}
		if !strings.Contains(line, "-q:v") {
			t.Fatalf("asked for no quality at all: %v", got)
		}
		worse, err := e.VideoArgs(ctx, RenderSettings{CRF: 30})
		if err != nil {
			t.Fatal(err)
		}
		// CRF 18 is better than CRF 30 and a higher q is better, so the
		// good one has to come out with the larger number. Compared as
		// numbers, not as text: "100" sorts below "73" as text and this
		// would then pass while being wrong.
		good, bad := quality(t, got), quality(t, worse)
		if bad >= good {
			t.Errorf("crf 30 gave q %d and crf 18 gave q %d, which is upside down", bad, good)
		}
		// The default is generous, because the first renders at 73 showed
		// blocks in the shadows.
		if def, _ := e.VideoArgs(ctx, RenderSettings{CRF: DefaultOptions().CRF}); quality(t, def) != 85 {
			t.Errorf("the default crf asks Apple's encoder for q %d, not 85", quality(t, def))
		}
	})

	t.Run("q:v stays inside the range whatever the crf", func(t *testing.T) {
		e := encodingEngine(t, "h264_videotoolbox")
		e.WantEncoder = "h264_videotoolbox"
		for _, crf := range []int{0, 18, 51, 200, -5} {
			got, err := e.VideoArgs(ctx, RenderSettings{CRF: crf})
			if err != nil {
				t.Fatal(err)
			}
			if q := quality(t, got); q < 1 || q > 100 {
				t.Errorf("crf %d gave q %d, which ffmpeg will refuse", crf, q)
			}
		}
	})

	// An encoder nobody has written a mapping for is still usable. It gets
	// its own defaults rather than a number that means the opposite to it.
	t.Run("an unknown encoder is left to its own defaults", func(t *testing.T) {
		e := encodingEngine(t, "h264_nvenc")
		e.WantEncoder = "h264_nvenc"
		got, err := e.VideoArgs(ctx, rs)
		if err != nil {
			t.Fatal(err)
		}
		if strings.Join(got, " ") != "-c:v h264_nvenc" {
			t.Errorf("got %v", got)
		}
	})
}

// The order each system reaches for, read straight from the table so it can
// be checked from anywhere. macOS is the one that matters and the machine
// running this in a cloud session is not a Mac, so a test that only ran on
// darwin would never run at all until CI reached the macOS job.
func TestEachSystemReachesForItsOwnEncoderFirst(t *testing.T) {
	mac := encoderOrderFor("darwin")
	if len(mac) == 0 || mac[0].Name != "h264_videotoolbox" {
		t.Errorf("a Mac would not reach for Apple's encoder first: %v", names(mac))
	}
	// libx264 stays as a fallback, because Tim's own ffmpeg has it and a
	// machine whose videotoolbox is missing should still render.
	if len(mac) < 2 || mac[len(mac)-1].Name != "libx264" {
		t.Errorf("libx264 is not the fallback on a Mac: %v", names(mac))
	}
	// Every entry has to be one we know how to ask for a quality, or the
	// render silently takes the encoder's defaults.
	for _, goos := range []string{"darwin", "linux", "windows"} {
		for _, candidate := range encoderOrderFor(goos) {
			if _, ok := known[candidate.Name]; !ok {
				t.Errorf("%s reaches for %s, which has no quality mapping",
					goos, candidate.Name)
			}
			if candidate.Quality == nil {
				t.Errorf("%s has no quality arguments", candidate.Name)
			}
		}
	}
}

// The number after -q:v in a built argument list.
func quality(t *testing.T, args []string) int {
	t.Helper()
	for i, a := range args {
		if a == "-q:v" && i+1 < len(args) {
			n, err := strconv.Atoi(args[i+1])
			if err != nil {
				t.Fatalf("q:v is %q, which is not a number", args[i+1])
			}
			return n
		}
	}
	t.Fatalf("no -q:v in %v", args)
	return 0
}

func names(list []Encoder) []string {
	out := make([]string, len(list))
	for i, e := range list {
		out[i] = e.Name
	}
	return out
}

// The whole reason for this change: an ffmpeg built the way the shipped one
// is renders, and the old code refused before it started. The fake has no
// libx264, which is exactly what our build will look like.
func TestAnFFmpegWithoutLibx264IsNotAFailure(t *testing.T) {
	e := encodingEngine(t, "h264_videotoolbox")
	e.WantEncoder = "h264_videotoolbox"
	if _, err := e.VideoEncoder(context.Background()); err != nil {
		t.Fatalf("an LGPL ffmpeg was refused: %v", err)
	}
}
