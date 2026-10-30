package engine

import (
	"bytes"
	"context"
	"encoding/binary"
	"encoding/json"
	"math"
	"os"
	"os/exec"
	"reflect"
	"regexp"
	"strconv"
	"strings"
	"testing"

	"github.com/spf13/afero"

	"framefairy/engine/runlog"
	"framefairy/internal/ffmpegtest"
)

// A crop that follows, plan row 3.26: keyframes put on frames of a clip,
// and the crop gliding from each to the next.

// cropMarks writes keyframes into the clip of rotatablePlan as a person's
// edits would leave them.
func cropMarks(t *testing.T, fsys afero.Fs, path string, marks ...CropMark) {
	t.Helper()
	for _, m := range marks {
		if err := SetCropMark(fsys, path, "01", -1, m.At, m.X, at25); err != nil {
			t.Fatal(err)
		}
	}
}

func loadedClip(t *testing.T, fsys afero.Fs, path string) Clip {
	t.Helper()
	_, clips, err := LoadClips(fsys, path)
	if err != nil {
		t.Fatal(err)
	}
	return clips[0]
}

// The glide takes the whole time between two marks, starting and settling
// gently, stands still before the first and after the last, and two marks
// a frame apart are an instant move.
func TestTheGlideEasesBetweenMarksAndHoldsBeyondThem(t *testing.T) {
	t.Parallel()
	marks := []frameMark{{10, 100}, {20, 300}, {21, 40}, {31, 40}}
	for _, c := range []struct {
		k    int64
		want float64
	}{
		{0, 100}, {10, 100},
		// A tenth of the way, smoothstep has moved 2.8 %, to 105.6. Half
		// way is half way, 200.
		{11, 105.6}, {15, 200}, {19, 294.4},
		{20, 300}, {21, 40}, {25, 40}, {40, 40},
	} {
		if got := glideX(marks, c.k); math.Abs(got-c.want) > 1e-9 {
			t.Errorf("on frame %d the crop is at %v, not %v", c.k, got, c.want)
		}
	}
	// The glide starts more slowly than it moves in the middle.
	if early, middle := glideX(marks, 11)-glideX(marks, 10), glideX(marks, 15)-glideX(marks, 14); early >= middle {
		t.Errorf("the glide moves %v px on its first frame and %v in the middle", early, middle)
	}
}

// A slow glide moves on every frame of the short. Rounded to an even
// pixel of the picture, a glide of 300 pixels over four seconds stood
// still for three frames at a time and then jumped 2 pixels, 3.6 in the
// short, which Tim saw on #192 as a crop frame that stutters.
func TestASlowGlideMovesOnEveryFrameOfTheShort(t *testing.T) {
	t.Parallel()
	const cropW, width, scaled = 608, 1920, 1084
	marks := []frameMark{{0, 400}, {100, 700}}
	cut, off := glideCrop(glideX(marks, 0), cropW, width, scaled)
	was := float64(cut) + float64(off)*float64(cropW+glideMargin)/scaled
	for k := int64(1); k < 100; k++ {
		x := glideX(marks, k)
		cut, off := glideCrop(x, cropW, width, scaled)
		at := float64(cut) + float64(off)*float64(cropW+glideMargin)/scaled
		// The short shows the picture from at, within half a pixel of the
		// short of where the glide is.
		if math.Abs(at-x) > 0.5*float64(cropW+glideMargin)/scaled+1e-9 {
			t.Errorf("on frame %d the glide is at %.3f, the short shows it from %.3f", k, x, at)
		}
		if step := at - was; k > 5 && k < 95 && step <= 0 {
			t.Errorf("on frame %d the short stands still or goes back, %.3f px of the picture", k, step)
		}
		was = at
	}
}

// The video preview works the glide out in lib/glide.ts, and the render in
// ffmpeg. Both are held to the marks and the left edge on each frame in
// frontend/src/lib/glide.cases.json, so the video preview shows on every
// frame the crop the short has. glide.test.ts holds the window to it.
//
// On the real disk: what is read is the interface's own source.
func TestTheGlideIsTheOneTheVideoPreviewWorksOut(t *testing.T) {
	t.Parallel()
	data, err := os.ReadFile("../frontend/src/lib/glide.cases.json")
	if err != nil {
		t.Fatal(err)
	}
	var fixture struct {
		Marks  [][2]int64   `json:"marks"`
		Frames [][2]float64 `json:"frames"`
	}
	if err := json.Unmarshal(data, &fixture); err != nil {
		t.Fatal(err)
	}
	var marks []frameMark
	for _, m := range fixture.Marks {
		marks = append(marks, frameMark{k: m[0], x: int(m[1])})
	}
	for _, f := range fixture.Frames {
		if got := glideX(marks, int64(f[0])); got != f[1] {
			t.Errorf("on frame %v the crop is at %v, the window has %v", f[0], got, f[1])
		}
	}
}

// renderedLefts runs the crop filters of a piece on a picture 480 wide
// whose every column says how far from the left it is, in 16 bits, and
// gives the column each frame starts on.
func renderedLefts(t *testing.T, source SourceInfo, seg Segment, frames int) []int {
	t.Helper()
	filters := source.cropFilters(seg, 152, 270, 0, 480, 152, 270, "flags=lanczos", "gray16le")
	got := readFrames(t, exec.Command("ffmpeg", "-v", "error", "-f", "lavfi",
		"-i", "color=black:s=480x270:r=25:d=10,format=gray16le,geq=lum=X*64",
		"-vf", strings.Join(filters, ",")+",format=gray16le", "-frames:v", strconv.Itoa(frames),
		"-f", "rawvideo", "-"), 152*270*2)
	if len(got) != frames {
		t.Fatalf("ffmpeg cropping by %s gave %d frames of 152 by 270, not %d", filters, len(got), frames)
	}
	lefts := make([]int, frames)
	for n, f := range got {
		lefts[n] = int(binary.LittleEndian.Uint16(f)) / 64
	}
	return lefts
}

// The render crops every frame where the glide is, to the pixel: the
// crop filters of a piece with keyframes leave on each frame the column
// glideCrop puts the left edge on, within half a pixel of glideX.
//
// On the real disk: ffmpeg renders the short.
func TestTheRenderGlidesTheCropOverTheMarks(t *testing.T) {
	ffmpegtest.Need(t)
	t.Parallel()
	source := SourceInfo{Width: 480, Height: 270, FPSNum: 25, FPSDen: 1}
	seg := Segment{Start: 0.4, End: 3.0, Marks: []CropMark{
		{At: 0.8, X: 0}, {At: 1.6, X: 328}, {At: 1.64, X: 100}, {At: 2.6, X: 201},
	}}
	first, _ := source.pieceFrames(seg.Start, seg.End)
	marks := source.onFrames(seg.Marks, 152, 480)
	for n, got := range renderedLefts(t, source, seg, 65) {
		x := glideX(marks, first+int64(n))
		cut, off := glideCrop(x, 152, 480, 154)
		if got != cut+off || math.Abs(float64(got)-x) > 0.5 {
			t.Errorf("frame %d of the piece is cropped at %d, the glide is at %.3f", n, got, x)
		}
	}

	// Scaled to the short, the glide is cut glideMargin wider, scaled,
	// and cropped again, and the frames come out the short's size.
	e := NewEngine(runlog.New(&bytes.Buffer{}, false, false), disk)
	graph, _, _, err := e.BuildFilterGraph(context.Background(), Clip{Segments: []Segment{seg}},
		source, RenderSettings{OutW: 1080, OutH: 1920, ScaleUp: true}, "")
	if err != nil {
		t.Fatal(err)
	}
	chain := regexp.MustCompile(`crop=154:270:'[^']+':0,scale=1094:1920:flags=lanczos,format=yuv444p,crop=1080:1920:'[^']+':0:exact=1`).FindString(graph)
	if chain == "" {
		t.Fatalf("the piece has no crop that glides to the pixel of the short: %s", graph)
	}
	frames := readFrames(t, exec.Command("ffmpeg", "-v", "error", "-f", "lavfi", "-i", "color=black:s=480x270:r=25:d=3",
		"-vf", chain, "-frames:v", "65", "-f", "rawvideo", "-pix_fmt", "gray", "-"), 1080*1920)
	if len(frames) != 65 {
		t.Errorf("the glide scaled to the short gave %d frames of 1080 by 1920, not 65", len(frames))
	}
}

// A piece steered by as many marks as a clip may have renders, and glides
// over all of them. One if per mark nested the expression past the
// hundred ffmpeg takes, and a clip of ninety marks could not be rendered.
func TestTheRenderGlidesOverAsManyMarksAsAClipMayHave(t *testing.T) {
	ffmpegtest.Need(t)
	t.Parallel()
	source := SourceInfo{Width: 480, Height: 270, FPSNum: 25, FPSDen: 1}
	seg := Segment{Start: 0, End: 9}
	for i := range MaxCropMarks {
		seg.Marks = append(seg.Marks, CropMark{At: 0.2 + float64(i)*0.08, X: (i * 37) % 329})
	}
	marks := source.onFrames(seg.Marks, 152, 480)
	for n, got := range renderedLefts(t, source, seg, 215) {
		x := glideX(marks, int64(n))
		if cut, off := glideCrop(x, 152, 480, 154); got != cut+off {
			t.Errorf("frame %d is cropped at %d, the glide is at %.3f", n, got, x)
		}
	}
}

// A mark steers every piece of the clip turned alike, cuts and all. Across
// a cut the glide goes on by the clip's clock, the time the short plays, so
// a piece is steered by the marks placed where they fall on its own
// stretch of the video as if the cuts were not there. A mark in a cut steers nothing,
// and comes back with its piece. Found by Tim: a keyframe before a cut and
// one after it did not glide at all, plan row 2.238.
func TestACropMarkSteersTheWholeClip(t *testing.T) {
	fsys, path := rotatablePlanPath(t)
	cropMarks(t, fsys, path, CropMark{At: 10.2, X: 100}, CropMark{At: 10.8, X: 200}, CropMark{At: 11.6, X: 500}, CropMark{At: 13.6, X: 50})
	c := loadedClip(t, fsys, path)
	// The pieces run 10 to 11.1, 11.1 to 12 and 13 to 14, with the cut 12
	// to 13 between. 13.6 is 2.6 s into the clip, which is 12.6 on the
	// first part's stretch, and 10.2 is 0.2 s into it, which is 11.2 on the
	// last piece's.
	before := []CropMark{{At: 10.2, X: 100}, {At: 10.8, X: 200}, {At: 11.6, X: 500}, {At: 12.6, X: 50}}
	want := [][]CropMark{
		before,
		before,
		{{At: 11.2, X: 100}, {At: 11.8, X: 200}, {At: 12.6, X: 500}, {At: 13.6, X: 50}},
	}
	steered := func(c Clip) {
		t.Helper()
		for i, seg := range c.Segments {
			if len(seg.Marks) != len(want[i]) {
				t.Fatalf("piece %d is steered by %v, not %v", i, seg.Marks, want[i])
			}
			for j := range want[i] {
				if math.Abs(seg.Marks[j].At-want[i][j].At) > 1e-6 || seg.Marks[j].X != want[i][j].X {
					t.Errorf("piece %d is steered by %v, not %v", i, seg.Marks, want[i])
				}
			}
		}
	}
	steered(c)
	if len(c.CropMarks) != 4 {
		t.Errorf("the clip has the keyframes %v", c.CropMarks)
	}

	// The glide is one across the cut: the first frame after it is where
	// the glide of the first piece would be 2 s into the clip, 12.0 on its
	// stretch, not where it would be a second later in the video, which is
	// how long the cut is.
	first, last := c.Segments[0], c.Segments[2]
	left := at25.CropAt(first, first.End-0.04, 152, 480)
	after := at25.CropAt(last, last.Start, 152, 480)
	if left == 200 || after == 50 {
		t.Errorf("the crop stands still either side of the cut, at %.1f and %.1f", left, after)
	}
	if want := at25.CropAt(first, 12.0, 152, 480); math.Abs(after-want) > 1e-9 {
		t.Errorf("after the cut the crop stands at %.3f, the glide from before it is at %.3f", after, want)
	}

	// The cut taken out of the clip's middle: the pieces now meet across
	// it, and the marks are where they are.
	reshape(t, fsys, path, Gesture{Kind: "join", From: 12.5})
	c = loadedClip(t, fsys, path)
	for _, seg := range c.Segments {
		if len(seg.Marks) != 4 {
			t.Errorf("with the cut put back, the piece at %.2f s is steered by %v", seg.Start, seg.Marks)
		}
	}
}

// A plan is untrusted: a keyframe that is not a moment and a whole left
// edge is left out, and the rest are read.
func TestACropMarkThatDoesNotHoldIsLeftOut(t *testing.T) {
	t.Parallel()
	got := readCropMarks([]any{
		map[string]any{"at": 10.5, "x": 101.0},
		map[string]any{"at": "10.6", "x": 100.0},
		map[string]any{"at": 10.7, "x": "100"},
		map[string]any{"at": 10.8, "x": -2.0},
		map[string]any{"at": 10.9, "x": 1.5},
		map[string]any{"at": 1e12, "x": 0.0},
		map[string]any{"at": 10.5, "x": 300.0},
		"10.0",
		map[string]any{"at": 10.2, "x": 40.0},
	}, nil)
	want := []CropMark{{At: 10.2, X: 40}, {At: 10.5, X: 100}}
	if len(got) != len(want) || got[0] != want[0] || got[1] != want[1] {
		t.Errorf("the plan's keyframes read as %v, not %v", got, want)
	}
}

// C puts a mark on the start of the frame it means, a drag moves it to
// another frame with its crop, and a double-click removes it. A mark has
// to be in the clip, and one frame holds one.
func TestACropMarkIsPutMovedAndRemovedLikeAThumbnail(t *testing.T) {
	fsys, path := rotatablePlanPath(t)
	if err := SetCropMark(fsys, path, "01", -1, 10.557, 300, at25); err != nil {
		t.Fatal(err)
	}
	if got := loadedClip(t, fsys, path).CropMarks; len(got) != 1 || got[0] != (CropMark{At: 10.52, X: 300}) {
		t.Fatalf("C at 10.557 s made %v, not a mark on the frame from 10.52 s", got)
	}
	for _, c := range []struct {
		name     string
		from, to float64
	}{
		{"in the cut", -1, 12.5},
		{"after the clip", -1, 20},
		{"on a frame that has one", -1, 10.53},
		{"from where there is none", 11, 11.5},
	} {
		if err := SetCropMark(fsys, path, "01", c.from, c.to, 100, at25); err == nil {
			t.Errorf("a keyframe %s was taken", c.name)
		}
	}
	if err := SetCropMark(fsys, path, "01", 10.52, 11.3, -1, at25); err != nil {
		t.Fatal(err)
	}
	if got := loadedClip(t, fsys, path).CropMarks; len(got) != 1 || got[0] != (CropMark{At: 11.28, X: 300}) {
		t.Fatalf("the mark dragged to 11.3 s is %v, not on the frame from 11.28 s", got)
	}
	if err := SetCropMark(fsys, path, "01", 11.28, -1, -1, at25); err != nil {
		t.Fatal(err)
	}
	if got := loadedClip(t, fsys, path).CropMarks; len(got) != 0 {
		t.Errorf("the mark removed is still there: %v", got)
	}
	data, _ := afero.ReadFile(fsys, path)
	if bytes.Contains(data, []byte(keyCropMarks)) {
		t.Errorf("a clip with no keyframes keeps the key: %s", data)
	}
}

// Dragging the crop frame on a mark moves that mark. Anywhere else it
// moves the crop of the clip, and every mark by as far as the crop there
// moved, so the glide keeps its shape. Automatic crop removes the marks.
func TestDraggingTheCropMovesTheMarkUnderThePlayheadOrTheWholeGlide(t *testing.T) {
	fsys, path := rotatablePlanPath(t)
	cropMarks(t, fsys, path, CropMark{At: 10.2, X: 100}, CropMark{At: 10.8, X: 200}, CropMark{At: 11.6, X: 500}, CropMark{At: 13.6, X: 60})

	if err := SetCrop(fsys, path, "01", 10.8, 300, at25, 152); err != nil {
		t.Fatal(err)
	}
	if got := loadedClip(t, fsys, path).CropMarks; got[1] != (CropMark{At: 10.8, X: 300}) || got[0].X != 100 || got[3].X != 60 {
		t.Fatalf("the crop placed on the mark at 10.8 s left the marks %v", got)
	}

	// At 10.5 s, in the frame from 10.48 s, the crop has glided 45 % of
	// the way from 100 to 300, to 190. Placed at 230 there, it moves 40,
	// and so does every mark, the one after the cut too.
	if err := SetCrop(fsys, path, "01", 10.5, 230, at25, 152); err != nil {
		t.Fatal(err)
	}
	got := loadedClip(t, fsys, path).CropMarks
	want := []CropMark{{At: 10.2, X: 140}, {At: 10.8, X: 340}, {At: 11.6, X: 540}, {At: 13.6, X: 100}}
	for i := range want {
		if got[i] != want[i] {
			t.Fatalf("the glide moved by 40 left the marks %v, not %v", got, want)
		}
	}
	if seg := pieceAt(t, fsys, path, 13.2); seg.CropX == nil || *seg.CropX != 230 {
		t.Errorf("the crop of the clip is %v, not 230", seg.CropX)
	}

	if err := ResetCrop(fsys, path, "01", 10.5); err != nil {
		t.Fatal(err)
	}
	if got := loadedClip(t, fsys, path).CropMarks; len(got) != 0 {
		t.Errorf("automatic crop left the marks %v", got)
	}
}

// Marks moved with the rest of the clip's stay in the picture. A mark
// pushed past the edge would be held there by the render while the
// others moved on, and the next drag would move it from a place nobody
// sees: dragged back, the part would no longer stand where it was put.
func TestACropMarkMovedWithTheRestStaysInThePicture(t *testing.T) {
	src := SourceInfo{Width: 1920, Height: 1080, FPSNum: 25, FPSDen: 1}
	const cropW, most = 608, 1312
	fsys, path := rotatablePlanPath(t)
	cropMarks(t, fsys, path, CropMark{At: 10.2, X: 1000}, CropMark{At: 10.8, X: 1300}, CropMark{At: 13.6, X: 100})
	// After the last mark the crop stands where it is, at 100. Dragged to
	// the right edge there, every mark moves 1212.
	if err := SetCrop(fsys, path, "01", 13.8, most, src, cropW); err != nil {
		t.Fatal(err)
	}
	for _, m := range loadedClip(t, fsys, path).CropMarks {
		if m.X > most {
			t.Errorf("the mark %v lies outside the picture, a crop %d wide starts at %d at most", m, cropW, most)
		}
	}
	// The first part now stands at the right edge throughout. Dragged 100
	// to the left there, it stands 100 to the left throughout.
	if err := SetCrop(fsys, path, "01", 10.5, most-100, src, cropW); err != nil {
		t.Fatal(err)
	}
	seg := loadedClip(t, fsys, path).Segments[0]
	for _, at := range []float64{10.0, 10.2, 10.5, 10.8, 11.0} {
		if got := src.CropAt(seg, at, cropW, src.Width); got != most-100 {
			t.Errorf("dragged to %d at 10.5 s, the crop at %.1f s stands at %v", most-100, at, got)
		}
	}
}

// A plan written by hand, or by an older version, can have a mark that
// is not on the start of its frame, or two marks on one frame. The one
// shown on a frame is found by its own moment, and taking it away takes
// away every mark on that frame, so the frame has none left.
func TestACropMarkIsFoundOnItsOwnFrameWhereverInItItIs(t *testing.T) {
	fsys, path := writePlanAt(t, `{"source": "ep.mp4", "clips": [{"id": "01",
	  "segments": [{"start": 10.0, "end": 12.0, "crop_x": 100}],
	  "crop_marks": [{"at": 10.55, "x": 100}, {"at": 11.01, "x": 200}, {"at": 11.015, "x": 300}, {"at": 11.6, "x": 400}]}]}`)
	// 10.55 s is late in the frame from 10.52 s, nearer the next one.
	if err := SetCropMark(fsys, path, "01", 10.55, 10.9, -1, at25); err != nil {
		t.Fatalf("the mark at 10.55 s could not be dragged: %v", err)
	}
	if err := SetCropMark(fsys, path, "01", 11.015, -1, -1, at25); err != nil {
		t.Fatalf("the marks at 11.01 s and 11.015 s could not be removed: %v", err)
	}
	want := []CropMark{{At: 10.88, X: 100}, {At: 11.6, X: 400}}
	got := loadedClip(t, fsys, path).CropMarks
	if len(got) != len(want) || got[0] != want[0] || got[1] != want[1] {
		t.Errorf("the marks are %v, not %v", got, want)
	}
}

// A trim that hides a keyframe keeps it in the clip's plan. Automatic
// crop removes it with the rest, so taking the trim back brings
// back no mark the person had already let go of.
func TestAutomaticCropRemovesTheMarksATrimHides(t *testing.T) {
	fsys, path := rotatablePlanPath(t)
	cropMarks(t, fsys, path, CropMark{At: 10.2, X: 100}, CropMark{At: 10.8, X: 200})
	reshape(t, fsys, path, Gesture{Kind: "trim", Edge: "start", From: 10.5})
	if err := ResetCrop(fsys, path, "01", 10.6); err != nil {
		t.Fatal(err)
	}
	reshape(t, fsys, path, Gesture{Kind: "trim", Edge: "start", From: 10.0})
	if got := loadedClip(t, fsys, path).CropMarks; len(got) != 0 {
		t.Errorf("after Automatic crop and the trim taken back the clip has the marks %v", got)
	}
}

// Rotating a part keeps its keyframes, found by Tim testing #340, issue
// #348: the diamonds went with the turn. A keyframe was placed on the
// picture as it stood, so it steers the part only while the part is turned
// that way again. A quarter or a half turn on, the picture has another
// shape and the part's crop is its own, and the keyframe waits, the way
// the crop the part had unrotated does. Four turns bring its glide back.
func TestRotatingAPartKeepsItsKeyframes(t *testing.T) {
	fsys, path := rotatablePlanPath(t)
	cropMarks(t, fsys, path, CropMark{At: 10.2, X: 100}, CropMark{At: 13.5, X: 50})
	want := []CropMark{{At: 10.2, X: 100}, {At: 13.48, X: 50}}
	for turns := 1; turns <= 4; turns++ {
		rotate(t, fsys, path, 13.5)
		if got := loadedClip(t, fsys, path).CropMarks; !reflect.DeepEqual(got, want) {
			t.Fatalf("turned %d quarters the clip has the keyframes %v, not %v", turns, got, want)
		}
		steering := pieceAt(t, fsys, path, 13.5).Marks
		if turns < 4 && len(steering) != 0 {
			t.Errorf("turned %d quarters the part glides over keyframes placed upright, %v", turns, steering)
		}
		if turns == 4 && (len(steering) != 2 || steering[1].X != 50) {
			t.Errorf("four turns on the part glides over %v, not the keyframes it had", steering)
		}
	}
}

// A crop placed on the frame of a keyframe that waits, placed on the
// picture turned another way, takes it to the picture as it stands now,
// so the diamond under the playhead is the one the drag moves.
func TestACropPlacedOnAWaitingKeyframeTakesItToThePictureAsItStands(t *testing.T) {
	fsys, path := rotatablePlanPath(t)
	cropMarks(t, fsys, path, CropMark{At: 13.52, X: 50})
	rotate(t, fsys, path, 13.5)
	if err := SetCrop(fsys, path, "01", 13.52, 200, at25, 400); err != nil {
		t.Fatal(err)
	}
	want := []CropMark{{At: 13.52, X: 200, Turn: 90}}
	if got := loadedClip(t, fsys, path).CropMarks; !reflect.DeepEqual(got, want) {
		t.Errorf("the keyframe under the crop placed is %v, not %v", got, want)
	}
	if got := pieceAt(t, fsys, path, 13.5).Marks; len(got) != 1 || got[0].X != 200 {
		t.Errorf("the rotated part glides over %v", got)
	}
}

// A plan made before keyframes said which way the picture was turned
// when they were placed keeps every keyframe where it was: one in a
// rotated part steers that part.
func TestAKeyframeOfAnOlderPlanBelongsToThePictureItLiesOn(t *testing.T) {
	t.Parallel()
	fsys, path := writePlanAt(t, `{"clips": [{"id": "01", "segments": [{"start": 1, "end": 2}, {"start": 2, "end": 3}],
	  "rotations": [{"from": 2, "deg": 90}],
	  "crop_marks": [{"at": 1.5, "x": 10}, {"at": 2.5, "x": 20}]}]}`)
	want := []CropMark{{At: 1.5, X: 10}, {At: 2.5, X: 20, Turn: 90}}
	if got := loadedClip(t, fsys, path).CropMarks; !reflect.DeepEqual(got, want) {
		t.Errorf("the older plan's keyframes read as %v, not %v", got, want)
	}
	if got := pieceAt(t, fsys, path, 2.5).Marks; len(got) != 1 || got[0].X != 20 {
		t.Errorf("the rotated part glides over %v", got)
	}
}
