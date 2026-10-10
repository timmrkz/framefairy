package engine

import (
	"bytes"
	"context"
	"encoding/binary"
	"fmt"
	"math"
	"os"
	"os/exec"
	"path/filepath"
	"testing"

	"framefairy/internal/ffmpegtest"
	"framefairy/internal/framewire"
)

// A caption's white is BT.2408's 203 nits in an HDR short: 75 percent of
// HLG's signal and 58 percent of PQ's. Black stays black, and an ASS
// colour keeps its alpha and its form. Plan row 2.156, step 4.
func TestTheCaptionsOfAnHDRShortAreAtTheReferenceWhite(t *testing.T) {
	for _, c := range []struct {
		light       framewire.Light
		white, want float64
	}{
		{framewire.HLG, 0, 0.75},
		{framewire.PQ, 0, 0.5807},
	} {
		r, g, b := hdrSignal(c.light, 1, 1, 1)
		for _, v := range []float64{r, g, b} {
			if math.Abs(v-c.want) > 0.002 {
				t.Errorf("%s: white is %.4f of the signal, want %.4f", c.light, v, c.want)
			}
		}
		if r, g, b := hdrSignal(c.light, 0, 0, 0); r+g+b > 1e-5 {
			t.Errorf("%s: black is %.4f %.4f %.4f, want 0", c.light, r, g, b)
		}
	}
	for _, c := range []struct {
		colour string
		light  framewire.Light
		want   string
	}{
		{"&H00FFFFFF", framewire.HLG, "&H00BFBFBF"},
		{"&H80000000", framewire.HLG, "&H80000000"},
		{"&HFFFFFF&", framewire.PQ, "&H949494&"},
		{"&H00FFFFFF", framewire.SDR, "&H00BFBFBF"},
		{"rot", framewire.HLG, "rot"},
		{"&HZZFFFFFF", framewire.HLG, "&HZZBFBFBF"},
		{"&HFFFFZZ", framewire.HLG, "&HFFFFZZ"},
	} {
		if c.light == framewire.SDR {
			// Standard video keeps its colours, through the style.
			s := Style{Primary: c.colour}.inLight(c.light)
			if s.Primary != c.colour {
				t.Errorf("standard video: %s became %s", c.colour, s.Primary)
			}
			continue
		}
		if got := hdrColour(c.colour, c.light); got != c.want {
			t.Errorf("%s: %s became %s, want %s", c.light, c.colour, got, c.want)
		}
	}
	// A saturated colour stays inside the signal.
	for _, light := range []framewire.Light{framewire.HLG, framewire.PQ} {
		r, g, b := hdrSignal(light, 0x94/255.0, 0x21/255.0, 0x92/255.0)
		for _, v := range []float64{r, g, b} {
			if v < 0 || v > 1 || math.IsNaN(v) {
				t.Errorf("%s: the accent gives %.4f %.4f %.4f", light, r, g, b)
			}
		}
	}
}

// libass draws a white caption into an HDR short at 75 percent of HLG's
// signal and 58 percent of PQ's, with the frame's own matrix, so it is
// white and not tinted.
func TestACaptionIsBurnedIntoAnHDRShortAtTheReferenceWhite(t *testing.T) {
	ctx := context.Background()
	e := NewEngine(NewLog(&bytes.Buffer{}, false, false))
	if err := e.Preflight(ctx); err != nil {
		ffmpegtest.Unusable(t, "no usable ffmpeg here: %v", err)
	}
	for _, c := range []struct {
		light framewire.Light
		trc   string
		want  float64
	}{
		{framewire.HLG, "arib-std-b67", 0.75},
		{framewire.PQ, "smpte2084", 0.5807},
	} {
		dir := t.TempDir()
		s := ResolveStyle(nil).inLight(c.light)
		header := assHeader(s, 64, 64, 12, 3, 0, 0, 0, 0)
		ass := header + "\n[Events]\nFormat: Layer, Start, End, Style, Name, MarginL, MarginR, MarginV, Effect, Text\n" +
			"Dialogue: 0,0:00:00.00,0:00:05.00,Caption,,0,0,0,,{\\an7\\pos(0,0)\\bord0\\shad0\\p1}m 0 0 l 64 0 64 64 0 64{\\p0}\n"
		path := filepath.Join(dir, "captions.ass")
		if err := os.WriteFile(path, []byte(ass), 0o644); err != nil {
			t.Fatal(err)
		}
		out, err := exec.Command(e.FFmpeg, "-v", "error",
			"-f", "lavfi", "-i", "color=black:s=64x64:d=0.04",
			"-vf", fmt.Sprintf("format=yuv420p10le,setparams=range=tv:color_primaries=bt2020:color_trc=%s:colorspace=bt2020nc,subtitles=%s", c.trc, path),
			"-frames:v", "1", "-f", "rawvideo", "-pix_fmt", "yuv420p10le", "-").Output()
		if err != nil {
			ffmpegtest.Unusable(t, "%s: burning a caption: %v", c.light, err)
		}
		if len(out) < 3*64*64 {
			t.Fatalf("%s: %d bytes for a frame", c.light, len(out))
		}
		// Read from the Y plane as it is, as gray would be full range.
		y := float64(binary.LittleEndian.Uint16(out[2*(32*64+32):]))
		signal := (y - 64) / 876
		if math.Abs(signal-c.want) > 0.01 {
			t.Errorf("%s: a white caption is %.0f, %.3f of the signal, want %.3f", c.light, y, signal, c.want)
		}
		for i, plane := range []string{"Cb", "Cr"} {
			at := 2*64*64 + i*2*32*32 + 2*(16*32+16)
			if v := int(binary.LittleEndian.Uint16(out[at:])); v < 508 || v > 516 {
				t.Errorf("%s: a white caption's %s is %d, not grey's 512", c.light, plane, v)
			}
		}
	}
}
