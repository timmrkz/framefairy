package engine

import (
	"bytes"
	"context"
	"errors"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"testing"

	"framefairy/internal/ffmpegtest"
	"framefairy/internal/framewire"
)

// A phone turned while it records keeps writing the picture the way it
// began, so the part after the turn lies on its side. Rotate stands such a
// part up again, a quarter turn to the left at a time, and the crop is
// placed on the picture as it stands then. Plan row 3.25.

const rotatablePlan = `{
  "source": "ep.mp4",
  "clips": [
    {"id": "01", "slug": "eins",
     "segments": [{"start": 10.0, "end": 11.1, "crop_x": 120},
                  {"start": 11.1, "end": 12.0, "crop_x": 300},
                  {"start": 13.0, "end": 14.0, "crop_x": 120}]}
  ]
}`

func rotatablePlanPath(t *testing.T) string {
	t.Helper()
	dir := filepath.Join(t.TempDir(), "ep.framefairy", "logs")
	if err := os.MkdirAll(dir, 0o755); err != nil {
		t.Fatal(err)
	}
	path := filepath.Join(dir, "clips.json")
	if err := os.WriteFile(path, []byte(rotatablePlan), 0o644); err != nil {
		t.Fatal(err)
	}
	return path
}

func rotations(t *testing.T, path string) ([]int, []*int) {
	t.Helper()
	_, clips, err := LoadClips(path)
	if err != nil {
		t.Fatal(err)
	}
	var turns []int
	var crops []*int
	for _, seg := range clips[0].Segments {
		turns = append(turns, seg.Rotate)
		crops = append(crops, seg.CropX)
	}
	return turns, crops
}

// The part is what plays straight on: the two pieces before the cut turn
// together, the one after it stays as it is. Four turns bring the part
// back, with the framing its shots were found with.
func TestRotateTurnsThePartBetweenCutsAndFourTurnsBringItBack(t *testing.T) {
	path := rotatablePlanPath(t)
	for i, want := range []int{90, 180, 270, 0} {
		if err := RotatePart(path, "01", 11.5); err != nil {
			t.Fatal(err)
		}
		turns, crops := rotations(t, path)
		if turns[0] != want || turns[1] != want || turns[2] != 0 {
			t.Fatalf("after %d turns the pieces are rotated %v, not %d, %d and 0", i+1, turns, want, want)
		}
		if want != 0 && (crops[0] != nil || crops[1] != nil) {
			t.Errorf("after %d turns the rotated crop is not in the middle: %v %v", i+1, *crops[0], *crops[1])
		}
		if crops[2] == nil || *crops[2] != 120 {
			t.Errorf("after %d turns the piece after the cut lost its crop", i+1)
		}
	}
	_, crops := rotations(t, path)
	if crops[0] == nil || *crops[0] != 120 || crops[1] == nil || *crops[1] != 300 {
		t.Errorf("rotated back, the part does not have the crops it was found with")
	}
}

// A crop moved by hand on a rotated part moves the rotated part only, not
// the pieces of the same shot that stand as they were filmed.
func TestACropOnARotatedPartStaysOnIt(t *testing.T) {
	path := rotatablePlanPath(t)
	if err := RotatePart(path, "01", 13.5); err != nil {
		t.Fatal(err)
	}
	if err := SetCrop(path, "01", 13.5, 400); err != nil {
		t.Fatal(err)
	}
	_, crops := rotations(t, path)
	if crops[0] == nil || *crops[0] != 120 {
		t.Errorf("the crop of a piece that is not rotated moved with the rotated one")
	}
	if crops[2] == nil || *crops[2] != 400 {
		t.Errorf("the rotated part did not take its crop")
	}
}

func TestAPlanRotatesOnlyByQuarters(t *testing.T) {
	for _, value := range []string{`45`, `-90`, `360`, `"left"`, `90.5`} {
		dir := t.TempDir()
		path := filepath.Join(dir, "clips.json")
		plan := `{"clips": [{"id": "01", "segments": [{"start": 1, "end": 3, "rotate": ` + value + `}]}]}`
		if err := os.WriteFile(path, []byte(plan), 0o644); err != nil {
			t.Fatal(err)
		}
		if _, _, err := LoadClips(path); err == nil {
			t.Errorf("a piece rotated by %s was taken", value)
		}
	}
}

// The render rotates a piece before it crops it, and the crop is the shape
// of the short in the picture rotated: a portrait video rotated a quarter
// is landscape, so its crop is a window that can move sideways in it.
func TestARotatedPieceIsRotatedBeforeItsCrop(t *testing.T) {
	e := NewEngine(NewLog(&bytes.Buffer{}, false, false))
	clip := Clip{Segments: []Segment{{Start: 1, End: 2}, {Start: 3, End: 4, Rotate: 90}}}
	graph, _, _, err := e.BuildFilterGraph(context.Background(), clip,
		SourceInfo{Width: 1080, Height: 1920, FPSNum: 25, FPSDen: 1},
		RenderSettings{OutW: 1080, OutH: 1920}, "")
	if err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(graph, "crop=1080:1920:0:0") {
		t.Errorf("the piece standing as filmed is not cropped whole: %s", graph)
	}
	if !strings.Contains(graph, "transpose=cclock,crop=608:1080:656:0") {
		t.Errorf("the rotated piece is not rotated and then cropped in the middle: %s", graph)
	}
}

// A quarter turn to the left puts the top of the picture on the left, the
// way Preview's Rotate Left does. Read from a picture with a red top and a
// blue bottom, rotated by the filter the render uses.
func TestRotatingLeftPutsTheTopOnTheLeft(t *testing.T) {
	ffmpegtest.Need(t)
	// A 64 by 64 picture, red over blue. Rotated, the pixel read is two
	// in from the left, half way down: red where the top went left.
	for _, c := range []struct {
		degrees int
		left    string
	}{{90, "red"}, {180, "blue"}, {270, "blue"}} {
		graph := "color=red:s=64x32:d=1[a];color=blue:s=64x32:d=1[b];[a][b]vstack," +
			framewire.Rotate(c.degrees) + ",format=rgb24"
		out, err := exec.Command("ffmpeg", "-v", "error", "-filter_complex", graph,
			"-frames:v", "1", "-f", "rawvideo", "-").Output()
		if err != nil {
			ffmpegtest.Unusable(t, "ffmpeg rotated nothing: %v", err)
		}
		at := (32*64 + 2) * 3
		if c.degrees == 180 {
			// Half way down is where red met blue, so read the top left,
			// which half a turn brought from the bottom right.
			at = 2 * 3
		}
		if len(out) != 64*64*3 {
			t.Fatalf("rotated %d, ffmpeg gave %d bytes, not a picture of 64 by 64", c.degrees, len(out))
		}
		got := "blue"
		if out[at] > out[at+2] {
			got = "red"
		}
		if got != c.left {
			t.Errorf("rotated %d to the left, the left is %s, not %s", c.degrees, got, c.left)
		}
	}
}

// A phone that records upright stores the picture lying down and says how
// to stand it up. ffmpeg stands it up before anything else sees it, so the
// engine has to know the picture's size standing up, or the crop is worked
// out for a landscape picture and cut from a portrait one.
func TestProbeStandsAPhoneVideoUp(t *testing.T) {
	ffmpegtest.Need(t)
	dir := t.TempDir()
	lying := filepath.Join(dir, "lying.mp4")
	phone := filepath.Join(dir, "phone.mp4")
	if out, err := exec.Command("ffmpeg", "-loglevel", "error", "-y", "-f", "lavfi",
		"-i", "testsrc2=s=320x180:r=25:d=1", "-c:v", "mpeg4", lying).CombinedOutput(); err != nil {
		ffmpegtest.Unusable(t, "ffmpeg made no video: %v %s", err, out)
	}
	if out, err := exec.Command("ffmpeg", "-loglevel", "error", "-y", "-display_rotation", "90",
		"-i", lying, "-c", "copy", phone).CombinedOutput(); err != nil {
		ffmpegtest.Unusable(t, "ffmpeg cannot write a display matrix: %v %s", err, out)
	}
	e := NewEngine(NewLog(&bytes.Buffer{}, false, false))
	info, err := e.Probe(context.Background(), phone)
	if err != nil {
		t.Fatal(err)
	}
	if info.Width != 180 || info.Height != 320 || info.Turned != 90 {
		t.Errorf("the phone's video stands %dx%d turned %d, not 180x320 turned 90",
			info.Width, info.Height, info.Turned)
	}
	info, err = e.Probe(context.Background(), lying)
	if err != nil {
		t.Fatal(err)
	}
	if info.Width != 320 || info.Height != 180 || info.Turned != 0 {
		t.Errorf("a video with no turn stands %dx%d turned %d", info.Width, info.Height, info.Turned)
	}
}

// The video preview of a phone's video stands up the way its short does:
// the episode's decoder turns the picture the file asks for, as the ffmpeg
// program does, so its frames agree with ffmpeg's at the size standing up.
func TestTheEpisodesDecoderStandsAPhoneVideoUp(t *testing.T) {
	ffmpegtest.Need(t)
	program := os.Getenv("FRAMEFAIRY_FRAMES")
	if program == "" {
		ffmpegtest.Unusable(t, "FRAMEFAIRY_FRAMES names no framefairy-frames, which make frames builds")
	}
	dir := t.TempDir()
	lying := filepath.Join(dir, "lying.mp4")
	phone := filepath.Join(dir, "phone.mp4")
	if out, err := exec.Command("ffmpeg", "-loglevel", "error", "-y", "-f", "lavfi",
		"-i", "testsrc2=s=320x180:r=25:d=2", "-c:v", "mpeg4", "-q:v", "3", lying).CombinedOutput(); err != nil {
		ffmpegtest.Unusable(t, "ffmpeg made no video: %v %s", err, out)
	}
	if out, err := exec.Command("ffmpeg", "-loglevel", "error", "-y", "-display_rotation", "90",
		"-i", lying, "-c", "copy", phone).CombinedOutput(); err != nil {
		ffmpegtest.Unusable(t, "ffmpeg cannot write a display matrix: %v %s", err, out)
	}
	e := NewEngine(NewLog(nil, false, false))
	e.FFmpeg, e.FFprobe = "ffmpeg", "ffprobe"
	first := func(run func(got func(float64, []byte) error) error) []byte {
		var frame []byte
		err := run(func(_ float64, data []byte) error {
			frame = data
			return errStop
		})
		if err != nil && !errors.Is(err, errStop) {
			t.Fatal(err)
		}
		return frame
	}
	want := first(func(got func(float64, []byte) error) error {
		return e.PreviewFrames(context.Background(), phone, 0.5, 90, 160, got)
	})
	dec := NewEpisodeFrames(program, phone)
	defer dec.Close()
	frame := first(func(got func(float64, []byte) error) error {
		return dec.Stream(context.Background(), 0.5, 90, 160, got)
	})
	if len(frame) != len(want) || len(want) == 0 {
		t.Fatalf("the decoder gave %d bytes and ffmpeg %d", len(frame), len(want))
	}
	if d := meanDiff(frame, want); d > 2 {
		t.Errorf("the decoder's frame of a phone's video differs from ffmpeg's by %.2f on average, "+
			"so it does not stand up the way the short does", d)
	}
}
