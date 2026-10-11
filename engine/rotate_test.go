package engine

import (
	"bytes"
	"context"
	"encoding/json"
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

// A rotated part stays rotated when its edges move: what an edge brings back
// joins the part it is dragged out of, rotated as the part is. Tim dragged
// the end of a clip out past a rotated part, and the frames it brought back
// stood as filmed. Put back, a cut's moments join the part before it.
func TestARotatedPartKeepsItsRotationWhenItsEdgesMove(t *testing.T) {
	tr := editableTranscript()
	for _, c := range []struct {
		name     string
		at       float64
		g        Gesture
		from, to float64
	}{
		{"the end dragged out", 13.5, Gesture{Kind: "trim", Edge: "end", From: 15, Frames: at25}, 13, 15},
		{"the start dragged out", 10.5, Gesture{Kind: "trim", Edge: "start", From: 9, Frames: at25}, 9, 12},
		{"the cut after it put back", 10.5, Gesture{Kind: "join", From: 12.5}, 10, 14},
	} {
		t.Run(c.name, func(t *testing.T) {
			path := rotatablePlanPath(t)
			if err := RotatePart(path, "01", c.at); err != nil {
				t.Fatal(err)
			}
			if err := Reshape(path, "01", c.g, tr, 0.1); err != nil {
				t.Fatal(err)
			}
			_, clips, err := LoadClips(path)
			if err != nil {
				t.Fatal(err)
			}
			for _, seg := range clips[0].Segments {
				inside := seg.Start >= c.from-1e-9 && seg.End <= c.to+1e-9
				if inside && seg.Rotate != 90 {
					t.Errorf("%.3f to %.3f, in the rotated part, is rotated %d", seg.Start, seg.End, seg.Rotate)
				}
				if !inside && seg.Rotate != 0 {
					t.Errorf("%.3f to %.3f, outside the rotated part, is rotated %d", seg.Start, seg.End, seg.Rotate)
				}
			}
		})
	}
}

// A part has one rotation. Putting back the cut before a rotated part
// joins it to the part before, and the joined part stands the way that
// part does, rather than turning halfway through. And a plan written
// before this, by a build that let a piece dragged out of a rotated part
// come back as filmed, reads as the part's first piece stands. Tim saw the
// picture turn back between two frames of one part, #188.
func TestAPartHasOneRotation(t *testing.T) {
	t.Run("the cut before a rotated part put back", func(t *testing.T) {
		path := rotatablePlanPath(t)
		if err := RotatePart(path, "01", 13.5); err != nil {
			t.Fatal(err)
		}
		if err := Reshape(path, "01", Gesture{Kind: "join", From: 12.5}, editableTranscript(), 0.1); err != nil {
			t.Fatal(err)
		}
		turns, _ := rotations(t, path)
		for i, turn := range turns {
			if turn != 0 {
				t.Errorf("piece %d of the joined part is rotated %d, the part before the cut %d: %v", i, turn, 0, turns)
			}
		}
		// What the file holds, not only what it reads as: a cut made
		// again inside the joined part must not bring the rotation back.
		if err := Reshape(path, "01", Gesture{Kind: "cut", From: 12.4, To: 12.6}, editableTranscript(), 0.1); err != nil {
			t.Fatal(err)
		}
		turns, _ = rotations(t, path)
		for i, turn := range turns {
			if turn != 0 {
				t.Errorf("after a new cut, piece %d is rotated %d: %v", i, turn, turns)
			}
		}
	})
	t.Run("a plan with a part rotated halfway", func(t *testing.T) {
		path := rotatablePlanPath(t)
		mixed := strings.Replace(rotatablePlan, `{"start": 10.0, "end": 11.1, "crop_x": 120}`, `{"start": 10.0, "end": 11.1, "rotate": 90}`, 1)
		if err := os.WriteFile(path, []byte(mixed), 0o644); err != nil {
			t.Fatal(err)
		}
		turns, crops := rotations(t, path)
		if turns[0] != 90 || turns[1] != 90 || turns[2] != 0 {
			t.Fatalf("the pieces read as rotated %v, not 90, 90 and 0", turns)
		}
		if crops[1] != nil {
			t.Errorf("the piece read as rotated keeps the crop %d of its picture as filmed", *crops[1])
		}
		// The first edit writes it the way it reads.
		if err := Reshape(path, "01", Gesture{Kind: "trim", Edge: "end", From: 14.5, Frames: at25}, editableTranscript(), 0.1); err != nil {
			t.Fatal(err)
		}
		data, err := os.ReadFile(path)
		if err != nil {
			t.Fatal(err)
		}
		var plan struct {
			Clips []struct {
				Segments []struct {
					Rotate int `json:"rotate"`
				} `json:"segments"`
			} `json:"clips"`
		}
		if err := json.Unmarshal(data, &plan); err != nil {
			t.Fatal(err)
		}
		var held []int
		for _, seg := range plan.Clips[0].Segments {
			held = append(held, seg.Rotate)
		}
		if len(held) != 3 || held[0] != 90 || held[1] != 90 || held[2] != 0 {
			t.Errorf("the file holds pieces rotated %v, not 90, 90 and 0", held)
		}
	})
}
