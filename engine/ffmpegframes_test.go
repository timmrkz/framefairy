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
	"testing"

	"framefairy/engine/runlog"
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

func TestShowinfoTime(t *testing.T) {
	for _, c := range []struct {
		line string
		at   float64
		ok   bool
	}{
		{"[Parsed_showinfo_0 @ 0x5591] n:   3 pts:  13400 pts_time:13.4    duration:  40 fmt:yuv420p10le", 13.4, true},
		{"[Parsed_showinfo_0 @ 0x5591] n:   0 pts:      0 pts_time:0       duration:  40", 0, true},
		{"[Parsed_showinfo_0 @ 0x5591] config in time_base: 1/1000, frame_rate: 25/1", 0, false},
		{"frame=   10 fps=0.0 q=-0.0 size=N/A time=00:00:00.40", 0, false},
		{"[Parsed_showinfo_0 @ 0x5591] n: 1 pts: 1 pts_time:nan duration: 1", 0, false},
	} {
		at, ok := showinfoTime(c.line)
		if ok != c.ok || (ok && math.Abs(at-c.at) > 1e-9) {
			t.Errorf("%q: got %v %v, want %v %v", c.line, at, ok, c.at, c.ok)
		}
	}
}

// Every frame from the moment asked for on, in the order they are shown,
// each the size asked for and saying where it starts.
//
// On the real disk: ffmpeg reads the episode.
func TestFFmpegFramesFromAMoment(t *testing.T) {
	t.Parallel()
	path := testEpisode(t, "6")
	info, err := NewEngine(runlog.New(io.Discard, false, false), disk).Probe(context.Background(), path)
	if err != nil {
		t.Fatal(err)
	}
	frame := 1 / info.FPS()
	const from, w, h = 2.0, 64, 36
	var ats []float64
	err = ffmpegFrames(context.Background(), path, from, w, h, func(at float64, f []byte) error {
		if len(f) != w*h*4 {
			t.Fatalf("a frame of %d bytes, want %d", len(f), w*h*4)
		}
		ats = append(ats, at)
		return nil
	})
	if err != nil {
		t.Fatal(err)
	}
	if len(ats) == 0 {
		t.Fatal("no frame came")
	}
	if ats[0] < from-1e-6 || ats[0] > from+frame+1e-6 {
		t.Errorf("the first frame starts at %.3f, want the first one from %.3f on", ats[0], from)
	}
	for i := 1; i < len(ats); i++ {
		if ats[i] <= ats[i-1] {
			t.Fatalf("frame %d starts at %.3f, after %.3f", i, ats[i], ats[i-1])
		}
	}
	want := (info.Duration - from) / frame
	if math.Abs(float64(len(ats))-want) > 2 {
		t.Errorf("%d frames from %.1f s of %.2f s, want about %.0f", len(ats), from, info.Duration, want)
	}
}

// A reader that has enough stops ffmpeg, and hears its own reason back.
// One that went on would hand the reader a fourth frame.
//
// On the real disk: ffmpeg reads the episode.
func TestFFmpegFramesStopsWhenTheReaderDoes(t *testing.T) {
	path := testEpisode(t, "6")
	enough := errors.New("enough")
	n := 0
	err := ffmpegFrames(context.Background(), path, 0, 64, 36, func(float64, []byte) error {
		n++
		if n == 3 {
			return enough
		}
		return nil
	})
	if !errors.Is(err, enough) || n != 3 {
		t.Fatalf("got %v after %d frames", err, n)
	}
}
