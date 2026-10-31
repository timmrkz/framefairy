package engine

import (
	"bufio"
	"context"
	"errors"
	"fmt"
	"io"
	"math"
	"os/exec"
	"strconv"
	"strings"

	"framefairy/internal/framewire"
)

// ffmpegFrames is the picture of an episode as the ffmpeg program decodes
// it on the processor, the frames the tests compare the episode's decoder
// with. Every frame of the video preview comes from the episode's decoder,
// so nothing else asks ffmpeg for frames, and the reference asks for no
// system decoder: on GitHub's virtual Mac a decode through VideoToolbox
// could hang for good, issue #581, and what the app does not run is no
// reference, issue #590.
//
// It hands over every frame from from on, from the key frame before it,
// scaled to width by height in colours as framewire.Picture makes them for
// the file's light, with the moment of the episode it starts at, in the
// order they are shown, each in a buffer of its own, which got may keep.
// It runs until got says no more, the context ends or the episode does.
func ffmpegFrames(ctx context.Context, path string, from float64, width, height int,
	got func(at float64, frame []byte) error) error {
	if width < 2 || height < 2 || width%2 != 0 || height%2 != 0 || width > 7680 || height > 4320 {
		return fmt.Errorf("a preview of %d by %d cannot be made", width, height)
	}
	if math.IsNaN(from) || math.IsInf(from, 0) || from < 0 {
		from = 0
	}
	ctx, cancel := context.WithCancel(ctx)
	defer cancel()
	// Seeking on the input decodes from the key frame before from and
	// drops what comes before it. With the timestamps kept, each frame
	// says where in the episode it starts, the way showinfo prints it.
	// Colours converted by ffmpeg from the file's own range and matrix,
	// the same chain as the episode's decoder.
	cmd := exec.CommandContext(ctx, "ffmpeg", "-hide_banner", "-nostdin", "-loglevel", "info",
		"-ss", strconv.FormatFloat(from, 'f', 6, 64), "-copyts", "-i", path,
		"-map", "0:v:0", "-an", "-sn", "-dn",
		"-vf", "showinfo,"+framewire.Picture(width, height),
		"-fps_mode", "passthrough", "-f", "rawvideo", "pipe:1")
	out, err := cmd.StdoutPipe()
	if err != nil {
		return err
	}
	errs, err := cmd.StderrPipe()
	if err != nil {
		return err
	}
	if err := cmd.Start(); err != nil {
		return err
	}
	// The moments come on the error stream, one line a frame, in the order
	// the frames come on the output.
	moments := make(chan float64, 64)
	said := &tail{}
	go func() {
		defer close(moments)
		lines := bufio.NewScanner(errs)
		lines.Buffer(make([]byte, 64<<10), 1<<20)
		for lines.Scan() {
			at, ok := showinfoTime(lines.Text())
			if !ok {
				said.add(lines.Text())
				continue
			}
			select {
			case moments <- at:
			case <-ctx.Done():
				return
			}
		}
	}()
	size := width * height * 4
	var failed error
	for {
		frame := make([]byte, size)
		if _, err := io.ReadFull(out, frame); err != nil {
			if !errors.Is(err, io.EOF) {
				failed = err
			}
			break
		}
		at, ok := <-moments
		if !ok {
			failed = errors.New("ffmpeg put out a frame without saying where it starts")
			break
		}
		if err := got(at, frame); err != nil {
			failed = err
			break
		}
	}
	cancel()
	_, _ = io.Copy(io.Discard, out)
	for range moments {
	}
	waited := cmd.Wait()
	if failed != nil {
		return failed
	}
	if waited != nil && ctx.Err() == nil {
		return fmt.Errorf("ffmpeg could not decode the picture: %s", said.String())
	}
	return nil
}

// showinfoTime reads the moment off a line showinfo prints for a frame.
func showinfoTime(line string) (float64, bool) {
	if !strings.Contains(line, "Parsed_showinfo") {
		return 0, false
	}
	at := strings.Index(line, " pts_time:")
	if at < 0 {
		return 0, false
	}
	rest := strings.TrimLeft(line[at+len(" pts_time:"):], " ")
	end := strings.IndexByte(rest, ' ')
	if end >= 0 {
		rest = rest[:end]
	}
	v, err := strconv.ParseFloat(rest, 64)
	if err != nil || math.IsNaN(v) || math.IsInf(v, 0) {
		return 0, false
	}
	return v, true
}
