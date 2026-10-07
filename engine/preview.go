package engine

import (
	"bufio"
	"context"
	"errors"
	"fmt"
	"io"
	"math"
	"os/exec"
	"runtime"
	"strconv"
	"strings"
	"sync/atomic"
)

// PreviewFrames decodes the picture of an episode for the video preview,
// for a file the webview cannot decode itself: HEVC with 10-bit colour
// made WebKit's decoder fail on its first frame, where it had said it
// would take it. ffmpeg decodes it, on the system's own decoder where
// there is one, from the key frame before from, and hands over every frame
// from from on, scaled to width by height in 8-bit I420, with the moment of
// the episode it starts at, in the order they are shown.
//
// It runs until got says no more, the context ends or the episode does. A
// frame is handed over as soon as ffmpeg has it and the next is decoded
// while got works on it, so a slow reader slows ffmpeg down rather than
// piling frames up.
func (e *Engine) PreviewFrames(ctx context.Context, path string, from float64, width, height int,
	got func(at float64, frame []byte) error) error {
	// On the Mac the frame is made smaller and 8-bit on the graphics chip,
	// scale_vt, before it is copied out of VideoToolbox: a frame of 10-bit
	// colour at 1080p is 6 MB, the frame at the size of the canvas a
	// fraction of it. A chain that fails before its first frame is made
	// again on the processor, and every one after it goes there straight
	// away.
	//
	// A chain that gives no frame is tried again on the processor too:
	// on a Mac with no graphics chip of its own, a virtual one, scale_vt
	// finds nothing to scale on and ffmpeg ends cleanly with no frame.
	if runtime.GOOS == "darwin" && !e.softDecode.Load() && !gpuScaleFails.Load() {
		came := false
		err := e.previewFrames(ctx, path, from, width, height, true, func(at float64, frame []byte) error {
			came = true
			return got(at, frame)
		})
		if came || ctx.Err() != nil {
			return err
		}
		came = false
		err = e.previewFrames(ctx, path, from, width, height, false, func(at float64, frame []byte) error {
			came = true
			return got(at, frame)
		})
		if came {
			gpuScaleFails.Store(true)
		}
		return err
	}
	return e.previewFrames(ctx, path, from, width, height, false, got)
}

// gpuScaleFails is set once scaling on the graphics chip has failed.
var gpuScaleFails atomic.Bool

func (e *Engine) previewFrames(ctx context.Context, path string, from float64, width, height int, gpu bool,
	got func(at float64, frame []byte) error) error {
	if e.FFmpeg == "" {
		return errors.New("there is no ffmpeg to decode the picture with")
	}
	if width < 2 || height < 2 || width%2 != 0 || height%2 != 0 || width > 7680 || height > 4320 {
		return fmt.Errorf("a preview of %d by %d cannot be made", width, height)
	}
	if math.IsNaN(from) || math.IsInf(from, 0) || from < 0 {
		from = 0
	}
	ctx, cancel := context.WithCancel(ctx)
	defer cancel()
	args := []string{"-hide_banner", "-nostdin", "-loglevel", "info"}
	scale := fmt.Sprintf("showinfo,scale=%d:%d:flags=bilinear:out_range=tv,format=yuv420p", width, height)
	if gpu {
		args = append(args, "-hwaccel", "videotoolbox", "-hwaccel_output_format", "videotoolbox_vld")
		scale = fmt.Sprintf("showinfo,scale_vt=w=%d:h=%d,hwdownload,format=nv12|p010le,format=yuv420p", width, height)
	} else {
		args = append(args, e.decodeFlags()...)
	}
	// Seeking on the input decodes from the key frame before from and
	// drops what comes before it. With the timestamps kept, each frame
	// says where in the episode it starts, the way showinfo prints it.
	args = append(args,
		"-ss", strconv.FormatFloat(from, 'f', 6, 64), "-copyts", "-i", path,
		"-map", "0:v:0", "-an", "-sn", "-dn",
		"-vf", scale,
		"-fps_mode", "passthrough", "-f", "rawvideo", "pipe:1")
	cmd := exec.CommandContext(ctx, e.FFmpeg, args...)
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
			line := lines.Text()
			at, ok := showinfoTime(line)
			if !ok {
				said.add(line)
				continue
			}
			select {
			case moments <- at:
			case <-ctx.Done():
				return
			}
		}
	}()
	size := width * height * 3 / 2
	frame := make([]byte, size)
	var failed error
	for {
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

// tail keeps the last lines ffmpeg said that were not about a frame, for
// the reason when it fails.
type tail struct {
	lines []string
}

func (t *tail) add(line string) {
	t.lines = append(t.lines, line)
	if len(t.lines) > 8 {
		t.lines = t.lines[1:]
	}
}

func (t *tail) String() string {
	return Scrub(strings.Join(t.lines, " "), 400)
}
