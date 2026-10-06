package engine

import (
	"bytes"
	"context"
	"encoding/binary"
	"fmt"
	"io"
	"math"
	"os/exec"
	"path/filepath"
	"strings"
	"testing"

	"framefairy/internal/ffmpegtest"
)

// A search parts a clip at every camera switch, so two pieces of a short
// can meet with nothing cut out between them. The render cuts nothing
// there: no fade in the sound, no frame lost or shown twice.

// The fade that keeps a cut from clicking is only where something is cut:
// at the clip's two ends and at every gap between pieces, never where one
// piece runs straight into the next.
func TestTheRenderFadesOnlyWhereSomethingIsCut(t *testing.T) {
	e := NewEngine(NewLog(&bytes.Buffer{}, false, false))
	clip := Clip{Segments: []Segment{{Start: 1, End: 2.6}, {Start: 2.6, End: 4}, {Start: 5, End: 6}}}
	graph, _, _, err := e.BuildFilterGraph(context.Background(), clip,
		SourceInfo{Width: 1920, Height: 1080, FPSNum: 25, FPSDen: 1},
		RenderSettings{OutW: 1080, OutH: 1920}, "")
	if err != nil {
		t.Fatal(err)
	}
	for i, want := range [][2]bool{{true, false}, {false, true}, {true, true}} {
		chain := ""
		// Found by the label it gives its piece, since a piece reads its
		// sound from an input of its own, see soundLead.
		for _, part := range strings.Split(graph, ";") {
			if strings.HasSuffix(part, fmt.Sprintf("[a%d]", i)) {
				chain = part
			}
		}
		in, out := strings.Contains(chain, "afade=t=in"), strings.Contains(chain, "afade=t=out")
		if in != want[0] || out != want[1] {
			t.Errorf("piece %d fades in %v and out %v, not %v and %v: %s", i+1, in, out, want[0], want[1], chain)
		}
	}
}

// Rendered, a clip of two shots that meet is heard and seen straight
// through the switch. The episode is a steady tone, and a picture that
// gets a step brighter on every frame, so a dip or a jump in the sound,
// or a frame lost or shown twice at the switch, would show.
func TestTwoShotsThatMeetRenderWithoutASeam(t *testing.T) {
	e := NewEngine(NewLog(&bytes.Buffer{}, false, false))
	if err := e.Preflight(context.Background()); err != nil {
		ffmpegtest.Unusable(t, "no usable ffmpeg here: %v", err)
	}
	seamless(t, e, "25")
}

// seamless renders a clip of two shots that meet from an episode at a frame
// rate, and checks the switch is neither seen nor heard.
func seamless(t *testing.T, e *Engine, rate string) {
	ctx := context.Background()
	dir := t.TempDir()
	source := filepath.Join(dir, "episode.mov")
	if out, err := exec.Command("ffmpeg", "-loglevel", "error", "-y",
		"-f", "lavfi", "-i", "color=c=black:s=640x360:r="+rate+":d=6,geq=lum='40+mod(N*6,180)':cb=128:cr=128",
		"-f", "lavfi", "-i", "sine=f=440:sample_rate=48000:d=6",
		"-shortest", "-c:v", "mpeg4", "-q:v", "2", "-c:a", "pcm_s16le", source).CombinedOutput(); err != nil {
		t.Fatalf("making the test episode: %s %s", err, out)
	}
	info, err := e.Probe(ctx, source)
	if err != nil {
		t.Fatal(err)
	}
	// The switch is at 2.6 in the episode, on the frame nearest it, about
	// 1.6 into the short.
	clip := info.OnFrames(Clip{ID: "01", Slug: "seam", Segments: []Segment{
		{Start: 1, End: 2.6, CropX: intPtr(0)}, {Start: 2.6, End: 4, CropX: intPtr(280)}}})
	const w, h = 180, 320
	rs := RenderSettings{OutW: w, OutH: h, CRF: 18, Preset: "ultrafast", AudioBitrate: "192k", ScaleUp: true}
	short, err := e.RenderClip(ctx, clip, source, info, filepath.Join(dir, "out"), nil, rs, nil,
		filepath.Join(dir, "captions"), false)
	if err != nil {
		t.Fatal(err)
	}

	// Seen: every frame a step brighter than the one before, the switch
	// too, and back to dark every 30 frames of the episode.
	fps := info.FPS()
	first := int(math.Round(clip.Segments[0].Start * fps))
	want := int(math.Round(clip.Segments[1].End*fps)) - first
	frames := frameRGB(t, short, w, h)
	if len(frames) != want {
		t.Errorf("the short has %d frames, not %d", len(frames), want)
	}
	mean := func(f []byte) float64 {
		sum := 0
		for _, b := range f {
			sum += int(b)
		}
		return float64(sum) / float64(len(f))
	}
	for i := 1; i < len(frames); i++ {
		step := mean(frames[i]) - mean(frames[i-1])
		if (first+i)%30 == 0 {
			if step > -100 {
				t.Errorf("frame %d is %.1f brighter than the one before, not back to dark", i, step)
			}
		} else if step < 4 || step > 10 {
			t.Errorf("frame %d is %.1f brighter than the one before, not a step of 7", i, step)
		}
	}

	// Heard: the tone as loud in every 5 ms around the switch as anywhere,
	// and moving from one sample to the next no faster than anywhere, which
	// it would where a few samples were lost or heard twice.
	out, err := exec.Command("ffmpeg", "-v", "error", "-i", short, "-ac", "1", "-ar", "48000", "-f", "f32le", "-").Output()
	if err != nil {
		t.Fatal(err)
	}
	samples := make([]float32, len(out)/4)
	if err := binary.Read(bytes.NewReader(out), binary.LittleEndian, samples); err != nil && err != io.EOF {
		t.Fatal(err)
	}
	loud := func(at float64) float64 {
		from := int(at * 48000)
		sum := 0.0
		for _, s := range samples[from : from+240] {
			sum += float64(s) * float64(s)
		}
		return math.Sqrt(sum / 240)
	}
	jump := func(from, to float64) float64 {
		most := 0.0
		for k := int(from * 48000); k < int(to*48000); k++ {
			most = math.Max(most, math.Abs(float64(samples[k]-samples[k-1])))
		}
		return most
	}
	at := clip.Segments[0].Duration()
	steady := loud(0.8)
	for x := at - 0.1; x < at+0.1; x += 0.005 {
		if got := loud(x); got < 0.8*steady {
			t.Errorf("at %.3f s the tone is %.3f, %.0f%% of the steady %.3f", x, got, 100*got/steady, steady)
		}
	}
	if got, usual := jump(at-0.05, at+0.05), jump(0.7, 0.9); got > 1.5*usual {
		t.Errorf("the sound jumps by %.4f at the switch, %.4f elsewhere", got, usual)
	}
}
