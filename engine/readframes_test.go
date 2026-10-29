package engine

import (
	"bytes"
	"context"
	"errors"
	"io"
	"os"
	"os/exec"
	"testing"
)

// graphFrames runs the render's own filter graph for clip of the episode
// at source, with the caption file assName in dir and its faces beside it,
// or no captions when assName is empty, and gives its frames as rgb24
// before any encoder, w by h each. crop, when set, ends in a comma and
// cuts the rows read out of each frame. The colours are worked out at
// every pixel and rounded exactly, the same on every machine. The
// converter ffmpeg picks by itself takes each colour from its quarter of
// the pixels as it stands, and works otherwise on an Apple chip than on
// Linux, which put the captions a tenth of a step further apart there.
//
// On the real disk: ffmpeg reads the episode and the caption file.
func graphFrames(t *testing.T, e *Engine, clip Clip, source string, info SourceInfo, rs RenderSettings,
	dir, assName, crop string, w, h int) [][]byte {
	t.Helper()
	graph, video, sound, err := e.BuildFilterGraph(context.Background(), clip, info, rs, assName)
	if err != nil {
		t.Fatal(err)
	}
	graph += ";" + video + crop + "scale=flags=accurate_rnd+full_chroma_int+bitexact,format=rgb24[rgb]"
	args := append([]string{"-hide_banner", "-loglevel", "error"}, pieceInputs(clip, source, info)...)
	args = append(args, "-filter_complex", graph, "-map", "[rgb]", "-fps_mode", "cfr", "-r", info.FPSString(),
		"-f", "rawvideo", "-pix_fmt", "rgb24", "-", "-map", sound, "-f", "null", os.DevNull)
	cmd := exec.Command(e.FFmpeg, args...)
	cmd.Dir = dir
	return readFrames(t, cmd, w*h*3)
}

// readFrames runs cmd and reads what it writes as frames of size bytes,
// each into its own slice as it comes. Read whole into one buffer, the
// frames were copied over and over as it grew, which the race detector
// checks byte by byte: a few hundred megabytes for one test, and minutes
// of the tests' time. A frame cut short fails the test.
func readFrames(t *testing.T, cmd *exec.Cmd, size int) (frames [][]byte) {
	t.Helper()
	var stderr bytes.Buffer
	cmd.Stderr = &stderr
	out, err := cmd.StdoutPipe()
	if err != nil {
		t.Fatal(err)
	}
	if err := cmd.Start(); err != nil {
		t.Fatal(err)
	}
	var cut error
	for {
		f := make([]byte, size)
		_, err := io.ReadFull(out, f)
		if errors.Is(err, io.EOF) {
			break
		}
		if err != nil {
			// The rest is not read, so the program is ended rather than
			// left writing into a pipe nobody reads.
			cut = err
			_ = cmd.Process.Kill()
			break
		}
		frames = append(frames, f)
	}
	if err := cmd.Wait(); err != nil && cut == nil {
		t.Fatalf("%s: %v %s", cmd.Args[0], err, stderr.String())
	}
	if cut != nil {
		t.Fatalf("%s: frame %d of %d bytes: %v %s", cmd.Args[0], len(frames)+1, size, cut, stderr.String())
	}
	return frames
}
