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
	"sort"
	"strings"
	"testing"

	"framefairy/internal/ffmpegtest"
)

// The captions burned into a short appear on the frames the subtitle file
// says, and they agree with the sound of the words they show. Everything
// else about caption timing is proved against the numbers: the captions,
// the caption blocks and the caption box all read one list. This is the
// one place the numbers are checked against the finished short itself.
//
// The episode is black with a tone where each word is said and silence
// between, so the short can be read back without knowing anything
// about how it was made: white ink is a caption, the highlight colour is
// the pill behind the lit word, and the tone is the word being said. The
// clip has a cut, and its first piece is not a whole number of frames
// long, so a caption drifting from its sound at a cut would show.

// renderTimingFPS is the frame rate of the test episode.
const renderTimingFPS = 25

// renderTimingWords are what the test episode says, on its own clock. The
// first three are in the first piece of the two-piece clip, the next two
// after its cut, and the last six one in each piece of the six-piece clip.
var renderTimingWords = []Cue{
	{1.20, 1.60, "Eins"}, {1.60, 2.10, "zwei"}, {2.10, 2.60, "drei."},
	{6.30, 6.80, "vier"}, {6.80, 7.40, "fünf."},
	{10.10, 10.45, "sechs"}, {13.10, 13.45, "sieben"}, {16.10, 16.45, "acht"},
	{19.10, 19.45, "neun"}, {22.10, 22.45, "zehn"}, {25.10, 25.45, "elf."},
}

func renderTimingEpisode(t *testing.T) string {
	t.Helper()
	var tone []string
	for _, w := range renderTimingWords {
		// The tone stops a little before the word ends, so the next word
		// starts out of silence and its start can be heard.
		tone = append(tone, fmt.Sprintf("between(t,%.2f,%.2f)", w.Start, w.End-0.1))
	}
	path := filepath.Join(t.TempDir(), "episode.mp4")
	out, err := exec.Command("ffmpeg", "-loglevel", "error", "-y",
		"-f", "lavfi", "-i", fmt.Sprintf("color=c=black:s=640x360:r=%d:d=28", renderTimingFPS),
		"-f", "lavfi", "-i", "aevalsrc='0.5*sin(2*PI*440*t)*("+strings.Join(tone, "+")+")':s=48000:d=28",
		"-shortest", "-c:v", "mpeg4", "-q:v", "2", "-c:a", "aac", "-b:a", "192k", path).CombinedOutput()
	if err != nil {
		t.Fatalf("making the test episode: %s %s", err, out)
	}
	return path
}

// frameRGB reads every frame of a video as rgb24.
func frameRGB(t *testing.T, path string, w, h int) [][]byte {
	t.Helper()
	out, err := exec.Command("ffmpeg", "-v", "error", "-i", path, "-f", "rawvideo", "-pix_fmt", "rgb24", "-").Output()
	if err != nil {
		t.Fatal(err)
	}
	size := w * h * 3
	var frames [][]byte
	for i := 0; i+size <= len(out); i += size {
		frames = append(frames, out[i:i+size])
	}
	return frames
}

// loudFrom is the first moment from which a short's sound is loud, read
// in 10 ms frames, searching from `from` to `to` in seconds.
func loudFrom(t *testing.T, path string, from, to float64) float64 {
	t.Helper()
	out, err := exec.Command("ffmpeg", "-v", "error", "-i", path, "-ac", "1", "-ar", "16000", "-f", "f32le", "-").Output()
	if err != nil {
		t.Fatal(err)
	}
	samples := make([]float32, len(out)/4)
	if err := binary.Read(bytes.NewReader(out), binary.LittleEndian, samples); err != nil && err != io.EOF {
		t.Fatal(err)
	}
	for f := int(from * 100); f < int(to*100) && (f+1)*160 <= len(samples); f++ {
		sum := 0.0
		for _, s := range samples[f*160 : (f+1)*160] {
			sum += float64(s) * float64(s)
		}
		if 10*math.Log10(sum/160+1e-20) > -30 {
			return float64(f) / 100
		}
	}
	t.Fatalf("no sound between %.2f and %.2f", from, to)
	return 0
}

// look is what one frame of the short shows: whether a caption is there,
// and where the pill of the lit word is, across, or -1 when there is none.
type look struct {
	caption bool
	pill    float64
}

func lookAt(frame []byte, w int, pill [3]int) look {
	white, purple, sumX := 0, 0, 0
	for i := 0; i+2 < len(frame); i += 3 {
		r, g, b := int(frame[i]), int(frame[i+1]), int(frame[i+2])
		switch {
		case r > 200 && g > 200 && b > 200:
			white++
		case abs(r-pill[0]) < 30 && abs(g-pill[1]) < 30 && abs(b-pill[2]) < 30:
			purple++
			sumX += (i / 3) % w
		}
	}
	l := look{caption: white > 40, pill: -1}
	// The pill pops in from seven tenths of its size, so on the first frame
	// of a caption the pill behind a short word is small: 53 pixels behind
	// neun here, with x264, and under 40 with VideoToolbox on a Mac, which
	// read it as there a frame after its caption. A frame with no caption
	// has none of the colour at all, so a low count is still a pill.
	if purple > 10 {
		l.pill = float64(sumX) / float64(purple)
	}
	return l
}

func abs(v int) int {
	if v < 0 {
		return -v
	}
	return v
}

func TestTheShortShowsItsCaptionsWhenItSaysThem(t *testing.T) {
	// 3.03 s is 75.75 frames: the cut does not fall on a frame.
	checkRenderTiming(t, Clip{Segments: []Segment{{Start: 1.00, End: 4.03}, {Start: 6.00, End: 9.00}}})
}

// Every piece of a clip becomes a whole number of frames in the short,
// while the captions add up the pieces as they are. Six pieces, none a
// whole number of frames, so a caption that drifted from its word a
// little at every cut would be well away from it by the last.
func TestTheShortKeepsItsCaptionsOnTheirWordsAcrossCuts(t *testing.T) {
	var pieces []Segment
	for k := range 6 {
		start := 10.0 + 3*float64(k)
		pieces = append(pieces, Segment{Start: start, End: start + 0.53 + 0.017*float64(k)})
	}
	checkRenderTiming(t, Clip{Segments: pieces})
}

func checkRenderTiming(t *testing.T, clip Clip) {
	t.Helper()
	ctx := context.Background()
	e := NewEngine(NewLog(&bytes.Buffer{}, false, false))
	if err := e.Preflight(ctx); err != nil {
		ffmpegtest.Unusable(t, "no usable ffmpeg here: %v", err)
	}
	if _, err := e.SubtitleFilter(ctx); err != nil {
		ffmpegtest.Unusable(t, "this ffmpeg cannot burn captions in: %v", err)
	}
	source := renderTimingEpisode(t)
	info, err := e.Probe(ctx, source)
	if err != nil {
		t.Fatal(err)
	}
	style := ResolveStyle(nil)
	if !style.Highlight {
		t.Fatal("the default style has no highlight to check")
	}

	clip.ID, clip.Slug = "01", "timing"
	// The way the render takes a clip, see Run.
	clip = info.OnFrames(clip)
	heard := &Transcript{Words: renderTimingWords, Language: "de"}
	laid := ClipCaptions(clip, heard, style)
	if len(laid) == 0 {
		t.Fatal("no captions")
	}

	const w, h = 360, 640
	rs := RenderSettings{OutW: w, OutH: h, CRF: 30, Preset: "ultrafast", AudioBitrate: "128k", ScaleUp: true}
	dir := t.TempDir()
	short, err := e.RenderClip(ctx, clip, source, info, filepath.Join(dir, "out"), laid, rs, nil,
		filepath.Join(dir, "captions"), false)
	if err != nil {
		t.Fatal(err)
	}

	var pill [3]int
	var alpha float64
	if _, err := fmt.Sscanf(style.HighlightWeb(), "rgba(%d, %d, %d, %g)",
		&pill[0], &pill[1], &pill[2], &alpha); err != nil {
		t.Fatalf("the highlight colour %q: %v", style.HighlightWeb(), err)
	}
	frames := frameRGB(t, short, w, h)
	looks := make([]look, len(frames))
	for i, f := range frames {
		looks[i] = lookAt(f, w, pill)
	}
	frame := 1.0 / renderTimingFPS

	// What the subtitle file says, in its own hundredths of a second, as
	// the moments something on screen changes. A caption comes and goes,
	// and the pill moves to each word as it starts, see highlight.go.
	type change struct {
		at   float64
		what string
	}
	var want []change
	for _, c := range laid {
		start, end := float64(centiseconds(c.Start))/100, float64(centiseconds(c.End))/100
		want = append(want, change{start, "caption " + c.Text + " appears"})
		if end < clip.Duration() {
			want = append(want, change{end, "caption " + c.Text + " goes"})
		}
		for _, line := range c.Lines {
			for _, word := range line {
				at := math.Max(float64(centiseconds(word.Start))/100, start)
				if at > start+1e-9 {
					want = append(want, change{at, "the pill moves to " + word.Text})
				}
			}
		}
	}
	sort.Slice(want, func(i, j int) bool { return want[i].at < want[j].at })

	// What the short shows: the frames on which something changed.
	var seen []float64
	for i := 1; i < len(looks); i++ {
		a, b := looks[i-1], looks[i]
		moved := (a.pill < 0) != (b.pill < 0) || (a.pill >= 0 && b.pill >= 0 && math.Abs(a.pill-b.pill) > 15)
		if a.caption != b.caption || moved {
			seen = append(seen, float64(i)*frame)
		}
	}
	if looks[0].caption {
		seen = append([]float64{0}, seen...)
	}

	// Every change the file asks for is on the first frame at or after
	// it, that frame and no other, and nothing changes that the file did
	// not ask for.
	if len(seen) != len(want) {
		t.Errorf("the short changes on %d frames, the subtitle file %d times: %v and %+v",
			len(seen), len(want), seen, want)
	}
	for i := 0; i < min(len(seen), len(want)); i++ {
		due := math.Ceil(want[i].at/frame-1e-6) * frame
		if math.Abs(seen[i]-due) > frame/2 {
			t.Errorf("%s at %.2f s, due on the frame at %.2f s, shows at %.2f s",
				want[i].what, want[i].at, due, seen[i])
		}
	}

	// And the words are heard where they are shown. Each word's tone in
	// the short starts at the moment its caption shows it, the caption for
	// the first word of each and the pill for the others, to within the
	// 10 ms the sound is read in. Before the pieces were put on frames, a
	// word after six cuts was heard 0.10 s after it was shown.
	for _, c := range laid {
		for k, word := range c.Words {
			shown := float64(centiseconds(word.Start)) / 100
			heardAt := loudFrom(t, short, math.Max(0, shown-0.08), shown+0.3)
			if math.Abs(heardAt-shown) > 0.015 {
				t.Errorf("%s (word %d of %q) is shown at %.2f s and heard at %.2f s",
					word.Text, k+1, c.Text, shown, heardAt)
			}
		}
	}
}

func TestOnFramesPutsEveryEdgeOnAFrame(t *testing.T) {
	at25 := SourceInfo{FPSNum: 25, FPSDen: 1}
	got := at25.OnFrames(Clip{Segments: []Segment{{Start: 1.00, End: 4.03}, {Start: 6.01, End: 6.02}, {Start: 7.019, End: 9.5}}})
	want := []Span{{1.00, 4.04}, {6.00, 6.04}, {7.00, 9.52}}
	for i, w := range want {
		if !near(got.Segments[i].Start, w.Start) || !near(got.Segments[i].End, w.End) {
			t.Errorf("piece %d is %+v, want %+v", i, got.Segments[i], w)
		}
	}
	// A source whose frame rate is not known is left as it is.
	if got := (SourceInfo{}).OnFrames(Clip{Segments: []Segment{{Start: 1.01, End: 2.02}}}); got.Segments[0].End != 2.02 {
		t.Errorf("got %+v", got.Segments)
	}
	// The clip it was made from keeps its own pieces.
	clip := Clip{Segments: []Segment{{Start: 1.01, End: 2.02}}}
	at25.OnFrames(clip)
	if clip.Segments[0].Start != 1.01 {
		t.Errorf("the clip given was changed: %+v", clip.Segments)
	}
	// At 30000/1001, a frame is 1001/30000 s.
	ntsc := SourceInfo{FPSNum: 30000, FPSDen: 1001}
	if got := ntsc.OnFrames(Clip{Segments: []Segment{{Start: 1.0, End: 2.0}}}); !near(got.Segments[0].Start, 30*1001.0/30000) {
		t.Errorf("got %+v", got.Segments)
	}
}
