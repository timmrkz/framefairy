package engine

import (
	"bytes"
	"context"
	"encoding/json"
	"os"
	"os/exec"
	"regexp"
	"testing"

	"framefairy/internal/ffmpegtest"
)

// A crop that follows, plan row 3.26: crop marks put on frames of a clip,
// and the crop gliding from each to the next.

// cropMarks writes crop marks into the clip of rotatablePlan as a person's
// edits would leave them.
func cropMarks(t *testing.T, path string, marks ...CropMark) {
	t.Helper()
	for _, m := range marks {
		if err := SetCropMark(path, "01", -1, m.At, m.X, at25); err != nil {
			t.Fatal(err)
		}
	}
}

func loadedClip(t *testing.T, path string) Clip {
	t.Helper()
	_, clips, err := LoadClips(path)
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
		want int
	}{
		{0, 100}, {10, 100},
		// A tenth of the way, smoothstep has moved 2.8 %, 105.6 to the
		// even pixel 106. Half way is half way, 200.
		{11, 106}, {15, 200}, {19, 294},
		{20, 300}, {21, 40}, {25, 40}, {40, 40},
	} {
		if got := glideX(marks, c.k); got != c.want {
			t.Errorf("on frame %d the crop is at %d, not %d", c.k, got, c.want)
		}
	}
	// The glide starts more slowly than it moves in the middle.
	if early, middle := glideX(marks, 11)-glideX(marks, 10), glideX(marks, 15)-glideX(marks, 14); early >= middle {
		t.Errorf("the glide moves %d px on its first frame and %d in the middle", early, middle)
	}
}

// The video preview works the glide out in lib/glide.ts, and the render in
// ffmpeg. Both are held to the marks and the left edge on each frame in
// frontend/src/lib/glide.cases.json, so the video preview shows on every
// frame the crop the short has. glide.test.ts holds the window to it.
func TestTheGlideIsTheOneTheVideoPreviewWorksOut(t *testing.T) {
	t.Parallel()
	data, err := os.ReadFile("../frontend/src/lib/glide.cases.json")
	if err != nil {
		t.Fatal(err)
	}
	var fixture struct {
		Marks  [][2]int64 `json:"marks"`
		Frames [][2]int64 `json:"frames"`
	}
	if err := json.Unmarshal(data, &fixture); err != nil {
		t.Fatal(err)
	}
	var marks []frameMark
	for _, m := range fixture.Marks {
		marks = append(marks, frameMark{k: m[0], x: int(m[1])})
	}
	for _, f := range fixture.Frames {
		if got := glideX(marks, f[0]); got != int(f[1]) {
			t.Errorf("on frame %d the crop is at %d, the window has %d", f[0], got, f[1])
		}
	}
}

// The render crops every frame where glideX says: the crop filter of a
// piece with crop marks, run on a picture whose every column says how far
// from the left it is, leaves on each frame the column the glide put the
// crop's left edge on.
func TestTheRenderGlidesTheCropOverTheMarks(t *testing.T) {
	ffmpegtest.Need(t)
	t.Parallel()
	source := SourceInfo{Width: 480, Height: 270, FPSNum: 25, FPSDen: 1}
	seg := Segment{Start: 0.4, End: 3.0, Marks: []CropMark{
		{At: 0.8, X: 0}, {At: 1.6, X: 328}, {At: 1.64, X: 100}, {At: 2.6, X: 201},
	}}
	e := NewEngine(NewLog(&bytes.Buffer{}, false, false))
	graph, _, _, err := e.BuildFilterGraph(context.Background(), Clip{Segments: []Segment{seg}},
		source, RenderSettings{OutW: 1080, OutH: 1920}, "")
	if err != nil {
		t.Fatal(err)
	}
	crop := regexp.MustCompile(`crop=152:270:'[^']+':0`).FindString(graph)
	if crop == "" {
		t.Fatalf("the piece has no crop that glides: %s", graph)
	}
	// Each column of the picture is as bright as half its distance from
	// the left, so the left edge of what is cropped is twice the first
	// byte of each row.
	frames := 65
	out, err := exec.Command("ffmpeg", "-v", "error", "-f", "lavfi",
		"-i", "color=black:s=480x270:r=25:d=3,format=gray,geq=lum=X/2",
		"-vf", crop+",format=gray", "-frames:v", "65", "-f", "rawvideo", "-").Output()
	if err != nil {
		ffmpegtest.Unusable(t, "ffmpeg cropped nothing: %v", err)
	}
	if len(out) != frames*152*270 {
		t.Fatalf("ffmpeg gave %d bytes, not %d frames of 152 by 270", len(out), frames)
	}
	first, _ := source.pieceFrames(seg.Start, seg.End)
	marks := source.onFrames(seg.Marks, 152, 480)
	for n := range frames {
		got := 2 * int(out[n*152*270])
		if want := glideX(marks, first+int64(n)); got != want {
			t.Errorf("frame %d of the piece is cropped at %d, the glide says %d", n, got, want)
		}
	}
}

// A mark steers the pieces of its own camera angle in its own part: a
// glide never crosses a cut, nor carries one shot's framing to another.
// A mark in a cut steers nothing, and comes back with its piece.
func TestACropMarkSteersItsAngleInItsPart(t *testing.T) {
	path := rotatablePlanPath(t)
	cropMarks(t, path, CropMark{10.2, 100}, CropMark{10.8, 200}, CropMark{11.6, 500}, CropMark{13.6, 50})
	c := loadedClip(t, path)
	want := [][]CropMark{
		{{10.2, 100}, {10.8, 200}},
		{{11.6, 500}},
		{{13.6, 50}},
	}
	for i, seg := range c.Segments {
		if len(seg.Marks) != len(want[i]) {
			t.Fatalf("piece %d is steered by %v, not %v", i, seg.Marks, want[i])
		}
		for j := range want[i] {
			if seg.Marks[j] != want[i][j] {
				t.Errorf("piece %d is steered by %v, not %v", i, seg.Marks, want[i])
			}
		}
	}
	if len(c.CropMarks) != 4 {
		t.Errorf("the clip has the crop marks %v", c.CropMarks)
	}

	// The cut taken out of the clip's middle: the pieces of the first
	// angle now meet across it, so its two parts are one and glide on.
	reshape(t, path, Gesture{Kind: "join", From: 12.5})
	c = loadedClip(t, path)
	for _, seg := range c.Segments {
		if seg.CropX != nil && *seg.CropX == 120 && len(seg.Marks) != 3 {
			t.Errorf("with the cut put back, the piece at %.2f s is steered by %v", seg.Start, seg.Marks)
		}
	}
}

// A plan is untrusted: a crop mark that is not a moment and a whole left
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
	})
	want := []CropMark{{10.2, 40}, {10.5, 100}}
	if len(got) != len(want) || got[0] != want[0] || got[1] != want[1] {
		t.Errorf("the plan's crop marks read as %v, not %v", got, want)
	}
}

// C puts a mark on the start of the frame it means, a drag moves it to
// another frame with its crop, and a double-click removes it. A mark has
// to be in the clip, and one frame holds one.
func TestACropMarkIsPutMovedAndRemovedLikeAThumbnail(t *testing.T) {
	path := rotatablePlanPath(t)
	if err := SetCropMark(path, "01", -1, 10.557, 300, at25); err != nil {
		t.Fatal(err)
	}
	if got := loadedClip(t, path).CropMarks; len(got) != 1 || got[0] != (CropMark{10.52, 300}) {
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
		if err := SetCropMark(path, "01", c.from, c.to, 100, at25); err == nil {
			t.Errorf("a crop mark %s was taken", c.name)
		}
	}
	if err := SetCropMark(path, "01", 10.52, 11.3, -1, at25); err != nil {
		t.Fatal(err)
	}
	if got := loadedClip(t, path).CropMarks; len(got) != 1 || got[0] != (CropMark{11.28, 300}) {
		t.Fatalf("the mark dragged to 11.3 s is %v, not on the frame from 11.28 s", got)
	}
	if err := SetCropMark(path, "01", 11.28, -1, -1, at25); err != nil {
		t.Fatal(err)
	}
	if got := loadedClip(t, path).CropMarks; len(got) != 0 {
		t.Errorf("the mark removed is still there: %v", got)
	}
	data, _ := os.ReadFile(path)
	if bytes.Contains(data, []byte(keyCropMarks)) {
		t.Errorf("a clip with no crop marks keeps the key: %s", data)
	}
}

// Dragging the crop frame on a mark moves that mark. Anywhere else it
// moves the crop of the angle, and every mark of the angle by as far as
// the crop there moved, so the glide keeps its shape. Automatic crop
// removes the angle's marks, and only those.
func TestDraggingTheCropMovesTheMarkUnderThePlayheadOrTheWholeGlide(t *testing.T) {
	path := rotatablePlanPath(t)
	cropMarks(t, path, CropMark{10.2, 100}, CropMark{10.8, 200}, CropMark{11.6, 500}, CropMark{13.6, 60})

	if err := SetCrop(path, "01", 10.8, 300, at25, 152); err != nil {
		t.Fatal(err)
	}
	if got := loadedClip(t, path).CropMarks; got[1] != (CropMark{10.8, 300}) || got[0].X != 100 || got[3].X != 60 {
		t.Fatalf("the crop placed on the mark at 10.8 s left the marks %v", got)
	}

	// At 10.5 s, in the frame from 10.48 s, the crop of the first angle
	// has glided 45 % of the way from 100 to 300, to 190. Placed at 230
	// there, it moves 40, and so does every mark of the angle, the one in
	// the other part too.
	if err := SetCrop(path, "01", 10.5, 230, at25, 152); err != nil {
		t.Fatal(err)
	}
	got := loadedClip(t, path).CropMarks
	want := []CropMark{{10.2, 140}, {10.8, 340}, {11.6, 500}, {13.6, 100}}
	for i := range want {
		if got[i] != want[i] {
			t.Fatalf("the glide moved by 40 left the marks %v, not %v", got, want)
		}
	}
	if seg := pieceAt(t, path, 13.2); seg.CropX == nil || *seg.CropX != 230 {
		t.Errorf("the crop of the angle is %v, not 230", seg.CropX)
	}

	if err := ResetCrop(path, "01", 10.5); err != nil {
		t.Fatal(err)
	}
	if got := loadedClip(t, path).CropMarks; len(got) != 1 || got[0] != (CropMark{11.6, 500}) {
		t.Errorf("automatic crop left the marks %v, not the other angle's alone", got)
	}
}

// A rotated stretch has a picture of another shape, so the marks placed on
// it as it stood are removed with the turn. Marks outside it stay.
func TestRotatingAPartRemovesItsCropMarks(t *testing.T) {
	path := rotatablePlanPath(t)
	cropMarks(t, path, CropMark{10.2, 100}, CropMark{13.5, 50})
	rotate(t, path, 13.5)
	if got := loadedClip(t, path).CropMarks; len(got) != 1 || got[0] != (CropMark{10.2, 100}) {
		t.Errorf("rotating the part after the cut left the marks %v", got)
	}
}
