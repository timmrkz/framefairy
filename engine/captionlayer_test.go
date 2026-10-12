package engine

import (
	"bytes"
	"context"
	"fmt"
	"math"
	"os"
	"os/exec"
	"path/filepath"
	"testing"

	"framefairy/internal/ffmpegtest"
	"framefairy/internal/framewire"
)

// The captions the episode's decoder draws for the video preview are the
// short's, frame by frame, the bounce included: the short's own frames,
// made by the render's own graph, are its frames without captions with
// the decoder's layer for that frame laid over them, at the moment
// CaptionMoment names. Tim saw the video preview draw the captions
// otherwise, because a browser drew them there and libass in the short.
//
// The render is run with the ffmpeg built beside the decoder, whose libass
// the decoder has, and only its graph, without an encoder, which that
// ffmpeg has none of on Linux. A caption file in a folder whose name
// ffmpeg would read as part of its chain proves the names reach the
// decoder whole.
func TestTheVideoPreviewDrawsTheShortsCaptions(t *testing.T) {
	program := os.Getenv("FRAMEFAIRY_FRAMES")
	if program == "" {
		ffmpegtest.Unusable(t, "FRAMEFAIRY_FRAMES names no framefairy-frames, which make frames builds")
	}
	ffmpeg := builtFFmpeg(program)
	if _, err := os.Stat(ffmpeg); err != nil {
		ffmpegtest.Unusable(t, "the ffmpeg built beside the decoder is not at %s", ffmpeg)
	}
	ctx := context.Background()
	e := NewEngine(NewLog(&bytes.Buffer{}, false, false))
	e.FFmpeg, e.FFprobe = ffmpeg, filepath.Join(filepath.Dir(ffmpeg), "ffprobe")
	if _, err := e.SubtitleFilter(ctx); err != nil {
		t.Fatalf("the built ffmpeg cannot burn captions in: %v", err)
	}
	// Grey with something moving, so a frame of the short and a frame of
	// the layer are each one frame and no other, in BT.709 the way HD
	// video is.
	dir := t.TempDir()
	source := filepath.Join(dir, "episode.mkv")
	if out, err := exec.Command(ffmpeg, "-loglevel", "error", "-y",
		"-f", "lavfi", "-i", fmt.Sprintf("testsrc2=s=640x360:r=%d:d=3", bounceFPS),
		"-f", "lavfi", "-i", "anullsrc=r=48000:cl=stereo:d=3",
		"-vf", "format=yuv420p,setparams=color_primaries=bt709:color_trc=bt709:colorspace=bt709:range=tv",
		"-shortest", "-c:v", "ffv1", "-c:a", "flac", source).CombinedOutput(); err != nil {
		t.Fatalf("making the test episode: %s %s", err, out)
	}
	info, err := e.Probe(ctx, source)
	if err != nil {
		t.Fatal(err)
	}
	words := []Cue{{0.20, 0.40, "Das"}, {0.50, 1.00, "respektvoll"}, {1.00, 1.30, "ist."},
		{2.00, 2.30, "Und"}, {2.30, 2.70, "dann?"}}
	// Two pieces with a cut between them, so the short's clock is not the
	// episode's.
	clip := info.OnFrames(Clip{ID: "01", Slug: "layer",
		Segments: []Segment{{Start: 0.1, End: 1.4}, {Start: 1.9, End: 2.8}}})
	style := ResolveStyle(nil)
	laid := ClipCaptions(clip, &Transcript{Words: words, Language: "de"}, style)
	const w, h = 540, 960
	rs := RenderSettings{OutW: w, OutH: h, ScaleUp: true}

	captions := filepath.Join(dir, "it's [a], b; c:d")
	if err := os.MkdirAll(captions, 0o755); err != nil {
		t.Fatal(err)
	}
	track, err := e.CaptionTrack(ctx, laid, w, h, nil, info.Light())
	if err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(captions, "01.ass"), track, 0o644); err != nil {
		t.Fatal(err)
	}
	if err := InstallFont(captions, style.Font); err != nil {
		t.Fatal(err)
	}
	// The short's frames, with captions and without, through the render's
	// own graph.
	frames := func(assName string) [][]byte {
		t.Helper()
		graph, video, sound, err := e.BuildFilterGraph(ctx, clip, info, rs, assName)
		if err != nil {
			t.Fatal(err)
		}
		args := append([]string{"-hide_banner", "-loglevel", "error"}, pieceInputs(clip, source, info)...)
		args = append(args, "-filter_complex", graph, "-map", video, "-fps_mode", "cfr", "-r", info.FPSString(),
			"-f", "rawvideo", "-pix_fmt", "rgb24", "-", "-map", sound, "-f", "null", os.DevNull)
		cmd := exec.Command(ffmpeg, args...)
		cmd.Dir = captions
		var stderr bytes.Buffer
		cmd.Stderr = &stderr
		out, err := cmd.Output()
		if err != nil {
			t.Fatalf("the render's graph: %v %s", err, stderr.String())
		}
		var all [][]byte
		for i := 0; i+w*h*3 <= len(out); i += w * h * 3 {
			all = append(all, out[i:i+w*h*3])
		}
		return all
	}
	with, without := frames("01.ass"), frames("")
	if len(with) != len(without) || len(with) < 100 {
		t.Fatalf("the short has %d frames with captions and %d without", len(with), len(without))
	}

	dec := NewEpisodeFrames(program, source)
	defer dec.Close()
	fonts := filepath.Join(captions, FontsFolder)
	// The episode's moment each frame of the short shows.
	var moments []float64
	for _, seg := range clip.Segments {
		first, end := info.pieceFrames(seg.Start, seg.End)
		for k := first; k < end; k++ {
			moments = append(moments, info.frameBegins(k))
		}
	}
	if len(moments) != len(with) {
		t.Fatalf("the clip has %d frames and its short %d", len(moments), len(with))
	}
	laidOver := func(ms int64, under []byte) []byte {
		t.Helper()
		l, err := dec.Captions(ctx, framewire.Captions{Width: w, Height: h, ShortW: w, ShortH: h,
			File: filepath.Join(captions, "01.ass"), Fonts: fonts}, ms)
		if err != nil {
			t.Fatal(err)
		}
		return over(under, w, l)
	}
	inked, apart := 0, 0
	for k, at := range moments {
		ms := info.CaptionMoment(clip, at)
		got := laidOver(ms, without[k])
		diff, most := lumaDiff(got, with[k])
		if diff > 0.1 || most > 20 {
			t.Errorf("frame %d, %.3f s of the episode at %d ms: the video preview's captions differ "+
				"from the short's by %.3f on average and %d at most", k, at, ms, diff, most)
		}
		if !bytes.Equal(without[k], with[k]) {
			inked++
		}
		// The frame before and after are told apart: a layer a frame off in
		// the bounce does not pass for the right one.
		if k > 0 {
			if before, _ := lumaDiff(laidOver(info.CaptionMoment(clip, moments[k-1]), without[k]), with[k]); before > 2*diff+0.01 {
				apart++
			}
		}
	}
	if inked < 60 {
		t.Errorf("only %d frames of the short have captions, the test proves little", inked)
	}
	if apart < 10 {
		t.Errorf("only %d frames differ from the layer of the frame before, so a frame off would pass", apart)
	}
}

// lumaDiff is how far apart two pictures of red, green and blue are in
// brightness, on average over every pixel and at most, in steps of 8
// bits. Brightness, because the short keeps its colour at a quarter of
// its pixels, so beside a sharp edge its colours run into each other in a
// way a layer laid over the picture by the pixel does not, and neither
// the eye nor anything else here reads that.
func lumaDiff(a, b []byte) (float64, int) {
	sum, most := 0, 0
	for i := 0; i+2 < len(a); i += 3 {
		ya := 2126*int(a[i]) + 7152*int(a[i+1]) + 722*int(a[i+2])
		yb := 2126*int(b[i]) + 7152*int(b[i+1]) + 722*int(b[i+2])
		d := (max(ya-yb, yb-ya) + 5000) / 10000
		sum += d
		most = max(most, d)
	}
	return float64(sum) / float64(len(a)/3), most
}

// builtFFmpeg is the ffmpeg make builds, whose libraries the episode's
// decoder is built on: in .build/ffmpeg beside .build/frames.
func builtFFmpeg(decoder string) string {
	return filepath.Join(filepath.Dir(filepath.Dir(decoder)), "ffmpeg", "bin", "ffmpeg")
}

// over lays a layer of captions over a picture of red, green and blue as
// the video preview does: its colour plus the picture times one less its
// alpha, see framewire.Layer.
func over(picture []byte, width int, l framewire.Layer) []byte {
	out := append([]byte(nil), picture...)
	for y := 0; y < l.H; y++ {
		for x := 0; x < l.W; x++ {
			p := l.Pixels[(y*l.W+x)*4:]
			a := float64(p[3]) / 255
			o := ((l.Y+y)*width + l.X + x) * 3
			for c := range 3 {
				out[o+c] = byte(math.Round(math.Min(255, float64(p[c])+float64(out[o+c])*(1-a))))
			}
		}
	}
	return out
}
