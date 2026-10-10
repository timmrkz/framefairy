package engine

import (
	"context"
	"errors"
	"io"
	"math"
	"testing"
	"time"
)

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
func TestPreviewFramesFromAMoment(t *testing.T) {
	path := testEpisode(t, "6")
	e := NewEngine(NewLog(io.Discard, false, false))
	info, err := e.Probe(context.Background(), path)
	if err != nil {
		t.Fatal(err)
	}
	frame := 1 / info.FPS()
	const from, w, h = 2.0, 64, 36
	var ats []float64
	err = e.PreviewFrames(context.Background(), path, from, w, h, func(at float64, f []byte) error {
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
func TestPreviewFramesStopsWhenTheReaderDoes(t *testing.T) {
	path := testEpisode(t, "6")
	e := NewEngine(NewLog(io.Discard, false, false))
	enough := errors.New("enough")
	n := 0
	began := time.Now()
	err := e.PreviewFrames(context.Background(), path, 0, 64, 36, func(float64, []byte) error {
		n++
		if n == 3 {
			return enough
		}
		return nil
	})
	if !errors.Is(err, enough) || n != 3 {
		t.Fatalf("got %v after %d frames", err, n)
	}
	if time.Since(began) > 10*time.Second {
		t.Errorf("stopping took %s", time.Since(began))
	}
}

func TestPreviewFramesRefusesAnOddSize(t *testing.T) {
	e := &Engine{FFmpeg: "ffmpeg"}
	for _, s := range [][2]int{{63, 36}, {64, 0}, {1 << 14, 36}} {
		if err := e.PreviewFrames(context.Background(), "x.mp4", 0, s[0], s[1], nil); err == nil {
			t.Errorf("%d by %d was taken", s[0], s[1])
		}
	}
}
