package engine

import (
	"bytes"
	"context"
	"encoding/binary"
	"io"
	"math"
	"os/exec"
	"path/filepath"
	"sort"
	"strconv"
	"strings"
	"testing"
)

// A render shows, at every moment of a short, the frame of the episode
// that holds that moment, and plays the sound of that same moment, on
// episodes whose frames do not come every 1/rate seconds from the start
// of the file. TestARenderCutsOnWholeFrames holds the files that do.
//
// The picture of the episode carries its frame number as eight bars, top
// to bottom, light for one and dark for nought, so a frame of the short
// says which frame of the episode it is. The sound is a tone that grows
// from a tenth to nine tenths of full every second, so how loud it is
// says which moment of the second is heard.

// TestARenderShowsTheFrameThatHoldsEachMoment renders a clip of four
// pieces from episodes made the ways cameras and recorders make them:
// frames at uneven times, the way a phone records, frames left out where
// nothing moved, the way a screen recorder does, and a picture that starts
// a quarter of a second after the sound, by an edit list in MOV and by its
// first timestamp in Matroska. The clip's first piece starts before the
// picture does.
func TestARenderShowsTheFrameThatHoldsEachMoment(t *testing.T) {
	e := NewEngine(NewLog(&bytes.Buffer{}, false, false))
	if err := e.Preflight(context.Background()); err != nil {
		t.Skip("no usable ffmpeg here")
	}
	for _, c := range []struct {
		name  string
		input []string
		video []string
		out   []string
		file  string
	}{
		// A phone: a frame every 1/29.97 s, each 0, 8 or 16 ms late, in
		// ticks of 1/600 s.
		{name: "uneven", file: "uneven.mov",
			video: []string{"settb=1/600", "setpts='(N*1001/30000+0.008*mod(N\\,3))/TB'"},
			out:   []string{"-fps_mode", "passthrough", "-enc_time_base", "1/600", "-video_track_timescale", "600"}},
		// A screen recorder: frames 40 to 49 and 100 to 104 left out, so
		// the frames before them are held.
		{name: "held", file: "held.mov",
			video: []string{"select='not(between(n\\,40\\,49)+between(n\\,100\\,104))'"},
			out:   []string{"-fps_mode", "passthrough"}},
		// The picture a quarter of a second after the sound, which is
		// 7.4925 frames, so its frames are half a frame off the grid of
		// the file's start.
		{name: "late_mov", file: "late.mov", input: []string{"-itsoffset", "0.25"},
			out: []string{"-fps_mode", "passthrough"}},
		{name: "late_mkv", file: "late.mkv", input: []string{"-itsoffset", "0.25"},
			out: []string{"-fps_mode", "passthrough"}},
	} {
		t.Run(c.name, func(t *testing.T) {
			t.Parallel()
			dir := t.TempDir()
			source := filepath.Join(dir, c.file)
			picture := "color=c=black:s=2x8:r=30000/1001:d=7," +
				"geq=lum='if(bitand(N\\,pow(2\\,7-Y))\\,220\\,30)':cb=128:cr=128,scale=640:360:flags=neighbor"
			if len(c.video) > 0 {
				picture += "," + strings.Join(c.video, ",")
			}
			args := []string{"-loglevel", "error", "-y"}
			args = append(args, c.input...)
			args = append(args, "-f", "lavfi", "-i", picture,
				"-f", "lavfi", "-i", "aevalsrc='(0.1+0.8*mod(t\\,1))*sin(2*PI*440*t)':s=48000:d=7")
			args = append(args, c.out...)
			args = append(args, "-c:v", "mpeg4", "-q:v", "2", "-g", "30", "-c:a", "pcm_s16le", "-t", "7", source)
			if out, err := exec.Command("ffmpeg", args...).CombinedOutput(); err != nil {
				t.Fatalf("making the test episode: %s %s", err, out)
			}
			showsWhatItSays(t, e, source)
		})
	}
}

func showsWhatItSays(t *testing.T, e *Engine, source string) {
	ctx := context.Background()
	info, err := e.Probe(ctx, source)
	if err != nil {
		t.Fatal(err)
	}
	// When each frame of the episode begins, by its own timestamp, and
	// which frame it is by its bars.
	out, err := exec.Command("ffprobe", "-v", "error", "-select_streams", "v:0",
		"-show_entries", "packet=pts_time", "-of", "csv=p=0", source).Output()
	if err != nil {
		t.Fatal(err)
	}
	var begins []float64
	for _, field := range strings.Fields(string(out)) {
		if v, err := strconv.ParseFloat(strings.Trim(field, ","), 64); err == nil {
			begins = append(begins, v)
		}
	}
	sort.Float64s(begins)
	numbers := barNumbers(t, source, 640, 360, "-fps_mode", "passthrough")
	if len(numbers) != len(begins) {
		t.Fatalf("the episode has %d frames and %d timestamps", len(numbers), len(begins))
	}
	// The frame that holds a moment: the last one that begins at or
	// before it, and the first one before the picture starts. A frame of
	// Matroska begins on a whole millisecond, so up to half of one away
	// from where it belongs, and one that begins less than frameHair after
	// a moment holds it.
	holds := func(at float64) int {
		k := sort.Search(len(begins), func(i int) bool { return begins[i] > at+frameHair })
		return numbers[max(k-1, 0)]
	}

	clip := info.OnFrames(Clip{ID: "01", Slug: "times", Segments: []Segment{
		{Start: 0.2, End: 0.9}, {Start: 1.25, End: 1.95}, {Start: 3.05, End: 3.7}, {Start: 5.3, End: 6.1}}})
	const w, h = 180, 320
	rs := RenderSettings{OutW: w, OutH: h, CRF: 18, Preset: "ultrafast", AudioBitrate: "192k", ScaleUp: true}
	work := t.TempDir()
	short, err := e.RenderClip(ctx, clip, source, info, filepath.Join(work, "out"), nil, rs, nil,
		filepath.Join(work, "captions"), false)
	if err != nil {
		t.Fatal(err)
	}
	got := barNumbers(t, short, w, h)
	sound := monoSamples(t, short)

	frame := float64(info.FPSDen) / float64(info.FPSNum)
	slots := 0
	for _, seg := range clip.Segments {
		slots += int(math.Round(seg.Duration() / frame))
	}
	if len(got) != slots {
		t.Errorf("the short has %d frames, its pieces last %d", len(got), slots)
	}
	if long := float64(len(sound)) / 48000; math.Abs(long-float64(slots)*frame) > 0.025 {
		t.Errorf("the sound is %.3f s long, the picture %.3f s", long, float64(slots)*frame)
	}

	// Seen: at every frame of the short, the frame of the episode that
	// holds the moment it stands for. Heard: in the middle of each, the
	// sound of that moment, away from the fades at the cuts and from where
	// the tone starts its second again. The short's AAC reads up to 15 ms
	// off on one frame and back on the next, so a piece is heard at the
	// mean of its frames.
	j, wrong := 0, 0
	for p, seg := range clip.Segments {
		n := int(math.Round(seg.Duration() / frame))
		sum, count := 0.0, 0
		for q := 0; q < n && j < len(got); q, j = q+1, j+1 {
			moment := seg.Start + float64(q)*frame
			if want := holds(moment); got[j] != want {
				if wrong++; wrong <= 6 {
					t.Errorf("piece %d, frame %d of the short shows frame %d of the episode, the one that holds %.4f s is %d",
						p+1, j, got[j], moment, want)
				}
			}
			middle := moment + frame/2
			inSecond := middle - math.Floor(middle)
			k := int((float64(j) + 0.5) * frame * 48000)
			if q == 0 || q == n-1 || inSecond < 0.015 || inSecond > 0.985 || k >= len(sound) {
				continue
			}
			sum += (toneAt(sound, k)-0.1)/0.8 - inSecond
			count++
		}
		if count == 0 {
			t.Errorf("piece %d has no frame to hear", p+1)
		} else if d := sum / float64(count); math.Abs(d) > 0.004 {
			t.Errorf("piece %d is heard %.1f ms away from the moments it shows", p+1, 1000*d)
		}
	}
	if wrong > 6 {
		t.Errorf("and %d more frames are the wrong frame", wrong-6)
	}
}

// barNumbers is the number the eight bars of each frame of a video say.
func barNumbers(t *testing.T, path string, w, h int, flags ...string) []int {
	t.Helper()
	args := append([]string{"-v", "error", "-i", path}, flags...)
	args = append(args, "-f", "rawvideo", "-pix_fmt", "gray", "-")
	out, err := exec.Command("ffmpeg", args...).Output()
	if err != nil {
		t.Fatal(err)
	}
	var numbers []int
	for i := 0; i+w*h <= len(out); i += w * h {
		n := 0
		for b := 0; b < 8; b++ {
			if out[i+(2*b+1)*h/16*w+w/2] > 125 {
				n |= 1 << (7 - b)
			}
		}
		numbers = append(numbers, n)
	}
	return numbers
}

// monoSamples is the sound of a file, in one channel at 48 kHz.
func monoSamples(t *testing.T, path string) []float32 {
	t.Helper()
	out, err := exec.Command("ffmpeg", "-v", "error", "-i", path, "-ac", "1", "-ar", "48000", "-f", "f32le", "-").Output()
	if err != nil {
		t.Fatal(err)
	}
	samples := make([]float32, len(out)/4)
	if err := binary.Read(bytes.NewReader(out), binary.LittleEndian, samples); err != nil && err != io.EOF {
		t.Fatal(err)
	}
	return samples
}

// toneAt is how loud the tone is at a sample: the root mean square of the
// 25 ms around it, which is 11 whole swings of 440 Hz, times the square
// root of two. A tone that grows evenly is as loud as its middle.
func toneAt(samples []float32, k int) float64 {
	sum, n := 0.0, 0
	for j := max(0, k-600); j < min(len(samples), k+600); j++ {
		sum += float64(samples[j]) * float64(samples[j])
		n++
	}
	return math.Sqrt(2 * sum / float64(max(n, 1)))
}
