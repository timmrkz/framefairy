package engine

import (
	"bytes"
	"context"
	"errors"
	"fmt"
	"math"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"testing"

	"github.com/spf13/afero"

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
                  {"start": 11.1, "end": 12.0, "crop_x": 120},
                  {"start": 13.0, "end": 14.0, "crop_x": 120}]}
  ]
}`

func rotatablePlanPath(t *testing.T) (afero.Fs, string) {
	t.Helper()
	return writePlanAt(t, rotatablePlan)
}

func rotations(t *testing.T, fsys afero.Fs, path string) ([]int, []*int) {
	t.Helper()
	_, clips, err := LoadClips(fsys, path)
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

// rotate presses R at a moment of the clip of rotatablePlan.
func rotate(t *testing.T, fsys afero.Fs, path string, at float64) {
	t.Helper()
	if err := RotatePart(fsys, path, "01", at, at25); err != nil {
		t.Fatal(err)
	}
}

// reshape makes a gesture on the clip of rotatablePlan.
func reshape(t *testing.T, fsys afero.Fs, path string, g Gesture) {
	t.Helper()
	if g.Frames.FPSNum == 0 {
		g.Frames = at25
	}
	if err := Reshape(fsys, nil, path, "01", g, editableTranscript(), 0.1); err != nil {
		t.Fatal(err)
	}
}

// pieceAt is the piece of the clip of rotatablePlan holding a moment.
func pieceAt(t *testing.T, fsys afero.Fs, path string, at float64) Segment {
	t.Helper()
	_, clips, err := LoadClips(fsys, path)
	if err != nil {
		t.Fatal(err)
	}
	for _, s := range clips[0].Segments {
		if at >= s.Start && at < s.End {
			return s
		}
	}
	t.Fatalf("no piece holds %.3f s", at)
	return Segment{}
}

// R turns the stretch of the video the part shows: the two pieces before
// the cut turn together, from the start of the video to the frame nearest
// the middle of the cut, 12.52 s at 25 frames a second, and the piece
// after it stays as it is. Four turns bring the part
// back, with the crops it had, which turning never touched.
func TestRotateTurnsTheStretchOfThePartAndFourTurnsBringItBack(t *testing.T) {
	fsys, path := rotatablePlanPath(t)
	for i, want := range []int{90, 180, 270, 0} {
		rotate(t, fsys, path, 11.5)
		turns, crops := rotations(t, fsys, path)
		if turns[0] != want || turns[1] != want || turns[2] != 0 {
			t.Fatalf("after %d turns the pieces are rotated %v, not %d, %d and 0", i+1, turns, want, want)
		}
		if want != 0 && (crops[0] != nil || crops[1] != nil) {
			t.Errorf("after %d turns the rotated crop is not in the middle: %v %v", i+1, crops[0], crops[1])
		}
		if crops[2] == nil || *crops[2] != 120 {
			t.Errorf("after %d turns the piece after the cut lost its crop", i+1)
		}
		_, clips, _ := LoadClips(fsys, path)
		if want != 0 {
			spans := clips[0].Rotations
			if len(spans) != 1 || !math.IsInf(spans[0].From, -1) || spans[0].To != 12.52 || spans[0].Deg != want {
				t.Errorf("after %d turns the rotations are %+v, not from the start to 12.52 at %d", i+1, spans, want)
			}
		} else if len(clips[0].Rotations) != 0 {
			t.Errorf("turned back, the clip keeps the rotations %+v", clips[0].Rotations)
		}
	}
	_, crops := rotations(t, fsys, path)
	if crops[0] == nil || *crops[0] != 120 || crops[1] == nil || *crops[1] != 120 {
		t.Errorf("rotated back, the part does not have the crop it had")
	}
}

// A crop placed by hand on a rotated piece moves the pieces of the clip
// rotated alike and no other: not the pieces standing as filmed. Rotating
// used to forget how a piece was framed, and a crop then spread to every
// rotated piece.
func TestACropOnARotatedPieceMovesThePiecesTurnedAlike(t *testing.T) {
	fsys, path := rotatablePlanPath(t)
	rotate(t, fsys, path, 13.5)
	if err := SetCrop(fsys, path, "01", 13.5, 400, SourceInfo{}, 0); err != nil {
		t.Fatal(err)
	}
	_, crops := rotations(t, fsys, path)
	if crops[0] == nil || *crops[0] != 120 || crops[1] == nil || *crops[1] != 120 {
		t.Errorf("the crop of the pieces standing as filmed moved with the rotated one")
	}
	if crops[2] == nil || *crops[2] != 400 {
		t.Errorf("the rotated part did not take its crop")
	}
	// Both parts turned: every piece is turned alike and shares the crop.
	rotate(t, fsys, path, 10.5)
	if err := SetCrop(fsys, path, "01", 13.5, 500, SourceInfo{}, 0); err != nil {
		t.Fatal(err)
	}
	turns, crops := rotations(t, fsys, path)
	if turns[0] != 90 || turns[1] != 90 || turns[2] != 90 {
		t.Fatalf("the pieces are rotated %v, not all 90", turns)
	}
	for i, c := range crops {
		if c == nil || *c != 500 {
			t.Errorf("piece %d of the pieces turned alike does not share the crop placed on one: %v", i, c)
		}
	}
}

// A part turned the way another part already is takes the crop placed on
// that one, since the pieces turned alike share one crop. It stood in the
// middle of the picture instead, and the clip had two crops turned 90 in
// its file, which the fuzzing found on main, #402. The app played both
// with the one placed by hand, so it showed only once the part with the
// crop was turned on: the other part then jumped to the middle.
func TestAPartTurnedLikeAnotherTakesItsCrop(t *testing.T) {
	fsys, path := rotatablePlanPath(t)
	rotate(t, fsys, path, 13.5)
	if err := SetCrop(fsys, path, "01", 13.5, 400, SourceInfo{}, 0); err != nil {
		t.Fatal(err)
	}
	rotate(t, fsys, path, 10.5)
	if err := cropsAlike(fsys, path); err != nil {
		t.Error(err)
	}
	rotate(t, fsys, path, 13.5) // the part with the crop turns on, to 180
	turns, crops := rotations(t, fsys, path)
	for i, c := range crops {
		if turns[i] == 90 && (c == nil || *c != 400) {
			t.Errorf("piece %d, turned 90, plays with %v, not the crop placed on the other part",
				i, fmtCrop(c))
		}
	}
	if turns[0] != 90 || turns[1] != 90 || turns[2] != 180 {
		t.Errorf("the pieces are turned %v, not 90, 90 and 180", turns)
	}
}

// The crop as filmed is the whole clip's, so a part that is turned keeps
// it for when it is turned back. Placed on the pieces standing as filmed
// alone, a part turned by mistake and turned back again brought its old
// crop with it, and the clip played with that one, found by the review of
// #398. The same with Automatic crop pressed while a part was turned.
func TestACropPlacedByHandSurvivesARotationTakenBack(t *testing.T) {
	fsys, path := rotatablePlanPath(t)
	rotate(t, fsys, path, 11.5) // the part before the cut turns 90
	if err := SetCrop(fsys, path, "01", 13.5, 400, SourceInfo{}, 0); err != nil {
		t.Fatal(err)
	}
	for range 3 { // three more quarter turns stand it as filmed again
		rotate(t, fsys, path, 11.5)
	}
	_, crops := rotations(t, fsys, path)
	for i, c := range crops {
		if c == nil || *c != 400 {
			t.Errorf("piece %d plays with %v, not the crop placed by hand", i, fmtCrop(c))
		}
	}

	rotate(t, fsys, path, 11.5)
	if err := ResetCrop(fsys, path, "01", 13.5); err != nil {
		t.Fatal(err)
	}
	for range 3 {
		rotate(t, fsys, path, 11.5)
	}
	_, crops = rotations(t, fsys, path)
	for i, c := range crops {
		if c == nil || *c != 120 {
			t.Errorf("after Automatic crop, piece %d plays with %v, not the automatic 120", i, fmtCrop(c))
		}
	}
}

// An edit leaves what it is not about alone: the crop placed by hand on a
// rotated part survives a crop put back on another part, in the file and
// as the render reads it.
func TestAnEditElsewhereLeavesARotatedCropAlone(t *testing.T) {
	fsys, path := rotatablePlanPath(t)
	rotate(t, fsys, path, 13.5)
	if err := SetCrop(fsys, path, "01", 13.5, 500, SourceInfo{}, 0); err != nil {
		t.Fatal(err)
	}
	before, err := afero.ReadFile(fsys, path)
	if err != nil {
		t.Fatal(err)
	}
	if err := ResetCrop(fsys, path, "01", 10.5); err != nil {
		t.Fatal(err)
	}
	after, err := afero.ReadFile(fsys, path)
	if err != nil {
		t.Fatal(err)
	}
	if got := pieceAt(t, fsys, path, 13.5); got.CropX == nil || *got.CropX != 500 || !got.Moved {
		t.Errorf("the rotated piece reads as %+v after a crop put back elsewhere", got)
	}
	if strings.Count(string(before), "crop_x_turned") != strings.Count(string(after), "crop_x_turned") {
		t.Errorf("putting back a crop elsewhere changed the rotated crops in the file:\n%s", after)
	}
}

// A rotation belongs to the video's time, so cutting, joining and
// trimming never change it: a frame is turned by the stretch its moment
// lies in, and a crop placed on a rotated piece goes where its frames go.
// Before, the rotation was kept on each piece, and a cut put back and made
// again lost it with its crop, found by the review of #188. Tim dragged
// the end of a clip out past a rotated part, and the frames it brought
// back stood as filmed.
func TestARotationStaysWithTheVideoAsTheClipIsCut(t *testing.T) {
	t.Run("the end dragged out", func(t *testing.T) {
		fsys, path := rotatablePlanPath(t)
		rotate(t, fsys, path, 13.5)
		reshape(t, fsys, path, Gesture{Kind: "trim", Edge: "end", From: 15})
		if got := pieceAt(t, fsys, path, 14.5); got.Rotate != 90 {
			t.Errorf("what the end brought back is rotated %d, not 90", got.Rotate)
		}
	})
	t.Run("the start dragged out", func(t *testing.T) {
		fsys, path := rotatablePlanPath(t)
		rotate(t, fsys, path, 10.5)
		reshape(t, fsys, path, Gesture{Kind: "trim", Edge: "start", From: 9})
		if got := pieceAt(t, fsys, path, 9.5); got.Rotate != 90 {
			t.Errorf("what the start brought back is rotated %d, not 90", got.Rotate)
		}
	})
	t.Run("the cut put back and made again", func(t *testing.T) {
		fsys, path := rotatablePlanPath(t)
		rotate(t, fsys, path, 13.5)
		if err := SetCrop(fsys, path, "01", 13.5, 700, SourceInfo{}, 0); err != nil {
			t.Fatal(err)
		}
		reshape(t, fsys, path, Gesture{Kind: "join", From: 12.5})
		for at, want := range map[float64]int{10.5: 0, 12.2: 0, 12.7: 90, 13.5: 90} {
			if got := pieceAt(t, fsys, path, at); got.Rotate != want {
				t.Errorf("joined, %.1f s is rotated %d, not %d", at, got.Rotate, want)
			}
		}
		reshape(t, fsys, path, Gesture{Kind: "cut", From: 12.2, To: 12.8})
		got := pieceAt(t, fsys, path, 13.5)
		if got.Rotate != 90 || got.CropX == nil || *got.CropX != 700 {
			t.Errorf("cut again, the part reads as %+v, not rotated 90 with its crop of 700", got)
		}
		if got := pieceAt(t, fsys, path, 10.5); got.Rotate != 0 {
			t.Errorf("cut again, the part before is rotated %d", got.Rotate)
		}
	})
}

// A plan's rotations are read as they are written or not at all: a
// stretch turned by anything but a quarter, a half or three quarters, one
// that ends before it starts, and two that overlap are refused, never
// read some other way.
func TestAPlansRotationsAreCheckedNotRepaired(t *testing.T) {
	for _, rotations := range []string{
		`[{"to": 3, "deg": 45}]`, `[{"to": 3, "deg": -90}]`, `[{"to": 3, "deg": 360}]`,
		`[{"to": 3, "deg": "left"}]`, `[{"to": 3, "deg": 90.5}]`, `[{"to": 3, "deg": 0}]`,
		`[{"to": 3}]`, `[{"from": 3, "to": 2, "deg": 90}]`,
		`[{"to": 3, "deg": 90}, {"from": 2, "deg": 180}]`, `{"deg": 90}`,
	} {
		plan := `{"clips": [{"id": "01", "segments": [{"start": 1, "end": 3}], "rotations": ` + rotations + `}]}`
		if _, _, err := LoadClips(writePlanAt(t, plan)); err == nil {
			t.Errorf("the rotations %s were taken", rotations)
		}
	}
	plan := `{"clips": [{"id": "01", "segments": [{"start": 1, "end": 2}, {"start": 2, "end": 3, "crop_x": 40, "crop_x_turned": 8}],
	  "rotations": [{"from": 2, "deg": 270}]}]}`
	_, clips, err := LoadClips(writePlanAt(t, plan))
	if err != nil {
		t.Fatal(err)
	}
	segs := clips[0].Segments
	if segs[0].Rotate != 0 || segs[1].Rotate != 270 || segs[1].CropX == nil || *segs[1].CropX != 8 || !segs[1].Moved {
		t.Errorf("the pieces read as %+v", segs)
	}
}

// The render rotates a piece before it crops it, and the crop is the shape
// of the short in the picture rotated: a portrait video rotated a quarter
// is landscape, so its crop is a window that can move sideways in it.
//
// On the real disk: ffmpeg renders the short.
func TestARotatedPieceIsRotatedBeforeItsCrop(t *testing.T) {
	e := NewEngine(NewLog(&bytes.Buffer{}, false, false), disk)
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

// phoneVideos writes, into dir, a video lying down 320 by 180, and the
// same turned by a display matrix 90, 180 and 270 degrees, the way a
// phone keeps what it records: once as mpeg4, and once as HEVC with
// B-frames, stored out of the order they show in, which is what a phone
// writes. A machine whose ffmpeg has no x265 skips the HEVC ones.
func phoneVideos(t *testing.T, dir string, seconds string) map[string]string {
	t.Helper()
	videos := map[string]string{}
	for _, codec := range []struct{ name, args string }{
		{"mpeg4", "-c:v mpeg4 -q:v 3"},
		{"hevc", "-c:v libx265 -x265-params bframes=3:log-level=error -tag:v hvc1"},
	} {
		lying := filepath.Join(dir, codec.name+".mp4")
		args := append([]string{"-loglevel", "error", "-y", "-f", "lavfi", "-i", "testsrc2=s=320x180:r=25:d=" + seconds},
			strings.Fields(codec.args)...)
		if out, err := exec.Command("ffmpeg", append(args, lying)...).CombinedOutput(); err != nil {
			if codec.name == "hevc" {
				t.Logf("no HEVC video, this ffmpeg cannot write one: %v %s", err, out)
				continue
			}
			ffmpegtest.Unusable(t, "ffmpeg made no video: %v %s", err, out)
		}
		if codec.name == "hevc" {
			out, err := exec.Command("ffprobe", "-v", "error", "-select_streams", "v:0",
				"-show_entries", "stream=has_b_frames", "-of", "csv=p=0", lying).Output()
			if err != nil || strings.TrimSpace(string(out)) == "0" {
				t.Fatalf("the HEVC video has no B-frames: %v %q", err, out)
			}
		}
		videos[codec.name+" 0"] = lying
		for _, turn := range []string{"90", "180", "270"} {
			phone := filepath.Join(dir, codec.name+"-"+turn+".mp4")
			if out, err := exec.Command("ffmpeg", "-loglevel", "error", "-y", "-display_rotation", turn,
				"-i", lying, "-c", "copy", phone).CombinedOutput(); err != nil {
				ffmpegtest.Unusable(t, "ffmpeg cannot write a display matrix: %v %s", err, out)
			}
			videos[codec.name+" "+turn] = phone
		}
	}
	return videos
}

// A phone that records upright stores the picture lying down and says how
// to stand it up. ffmpeg stands it up before anything else sees it, so the
// engine has to know the picture's size standing up, or the crop is worked
// out for a landscape picture and cut from a portrait one.
//
// On the real disk: ffmpeg reads the episode.
func TestProbeStandsAPhoneVideoUp(t *testing.T) {
	t.Parallel()
	ffmpegtest.Need(t)
	e := NewEngine(NewLog(&bytes.Buffer{}, false, false), disk)
	for name, path := range phoneVideos(t, t.TempDir(), "1") {
		info, err := e.Probe(context.Background(), path)
		if err != nil {
			t.Fatal(err)
		}
		turn := 0
		fmt.Sscanf(name[strings.LastIndex(name, " ")+1:], "%d", &turn)
		w, h := 320, 180
		if turn%180 != 0 {
			w, h = 180, 320
		}
		if info.Width != w || info.Height != h || info.Turned != turn {
			t.Errorf("%s stands %dx%d turned %d, not %dx%d turned %d",
				name, info.Width, info.Height, info.Turned, w, h, turn)
		}
	}
}

// The video preview of a phone's video stands up the way its short does:
// the episode's decoder turns the picture the file asks for, as the ffmpeg
// program does, so its frames agree with ffmpeg's at the size standing up,
// at every turn and for a phone's HEVC too.
//
// On the real disk: the episode's decoder reads the episode.
func TestTheEpisodesDecoderStandsAPhoneVideoUp(t *testing.T) {
	ffmpegtest.Need(t)
	program := os.Getenv("FRAMEFAIRY_FRAMES")
	if program == "" {
		ffmpegtest.Unusable(t, "FRAMEFAIRY_FRAMES names no framefairy-frames, which make frames builds")
	}
	e := NewEngine(NewLog(nil, false, false), disk)
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
	for name, phone := range phoneVideos(t, t.TempDir(), "2") {
		w, h := 160, 90
		if strings.HasSuffix(name, " 90") || strings.HasSuffix(name, " 270") {
			w, h = 90, 160
		}
		grid := gridOf(t, phone)
		want := first(func(got func(float64, []byte) error) error {
			return e.PreviewFrames(context.Background(), phone, grid.At(grid.Holding(0.5)), w, h, got)
		})
		dec := NewEpisodeFrames(program, phone)
		frame := first(func(got func(float64, []byte) error) error {
			return dec.Stream(context.Background(), grid, 0.5, w, h, nil, got)
		})
		dec.Close()
		if len(frame) != len(want) || len(want) == 0 {
			t.Errorf("%s: the decoder gave %d bytes and ffmpeg %d", name, len(frame), len(want))
			continue
		}
		if d := meanDiff(frame, want); d > 2 {
			t.Errorf("%s: the decoder's frame differs from ffmpeg's by %.2f on average, "+
				"so it does not stand up the way the short does", name, d)
		}
	}
}
