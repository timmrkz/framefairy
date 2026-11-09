package engine

import (
	"bytes"
	"context"
	"encoding/binary"
	"math"
	"os/exec"
	"reflect"
	"regexp"
	"strconv"
	"strings"
	"testing"

	"github.com/spf13/afero"

	"framefairy/engine/runlog"
	"framefairy/internal/ffmpegtest"
	"framefairy/internal/fixtures"
)

// Keyframes, plan row 3.26 and #750: a keyframe put on a frame of a clip
// switches its crop there, or the crop glides into it from the keyframe
// before, and the crop holds until the next one.

// cropMarks writes keyframes into the clip of rotatablePlan as a person's
// edits would leave them.
func cropMarks(t *testing.T, fsys afero.Fs, path string, marks ...CropMark) {
	t.Helper()
	for _, m := range marks {
		if err := SetCropMark(fsys, path, "01", -1, m.At, m.X, m.Glide, at25); err != nil {
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

// The crop is the piece's own before the first keyframe, switches on each
// keyframe's own frame, and holds until the next. Two keyframes a frame
// apart are two switches a frame apart.
func TestTheCropHoldsFromEachKeyframeToTheNext(t *testing.T) {
	t.Parallel()
	run := cropRun{marks: []frameMark{{k: 10, x: 100}, {k: 20, x: 300}, {k: 21, x: 40}, {k: 31, x: 40}}, base: 500}
	for _, c := range []struct {
		k    int64
		want int
	}{
		{0, 500}, {9, 500},
		{10, 100}, {11, 100}, {19, 100},
		{20, 300}, {21, 40}, {25, 40}, {40, 40},
	} {
		if got := run.at(c.k); got != float64(c.want) {
			t.Errorf("on frame %d the crop is at %v, not %d", c.k, got, c.want)
		}
	}
}

// Into a circle the crop glides from the keyframe before, over the whole
// time between them, easing out and in, and reaches it on its frame. Into
// the first keyframe it glides from the piece's own crop, from where the
// crop of its turn begins. After a circle it holds like after a diamond.
func TestTheCropGlidesIntoACircleFromTheKeyframeBefore(t *testing.T) {
	t.Parallel()
	run := cropRun{marks: []frameMark{
		{k: 10, x: 100}, {k: 20, x: 300, glide: true}, {k: 30, x: 300}, {k: 40, x: 0, glide: true}, {k: 41, x: 50, glide: true},
	}, base: 500, from: 4}
	for _, c := range []struct {
		k    int64
		want float64
	}{
		{0, 500}, {9, 500},
		{10, 100}, {15, 200}, {20, 300}, {25, 300}, {30, 300},
		{35, 150}, {40, 0}, {41, 50}, {60, 50},
	} {
		if got := run.at(c.k); math.Abs(got-c.want) > 1e-9 {
			t.Errorf("on frame %d the crop is at %v, not %v", c.k, got, c.want)
		}
	}
	// Eased: a third of the way in it has gone less than a third.
	if got := run.at(13); got <= 100 || got >= 100+200.0/3 {
		t.Errorf("a third of the way into the glide the crop is at %v", got)
	}

	first := cropRun{marks: []frameMark{{k: 10, x: 100, glide: true}}, base: 500, from: 4}
	for _, c := range []struct {
		k    int64
		want float64
	}{{0, 500}, {4, 500}, {7, 300}, {10, 100}, {20, 100}} {
		if got := first.at(c.k); math.Abs(got-c.want) > 1e-9 {
			t.Errorf("gliding into the first keyframe, on frame %d the crop is at %v, not %v", c.k, got, c.want)
		}
	}
}

// A slow glide moves on every frame of the short, within half a pixel of
// the short of where the glide is, and never stands still or goes back.
// Rounded to the picture's even pixels it stood still for frames and then
// jumped, the stutter Tim saw on #192.
func TestASlowGlideMovesOnEveryFrameOfTheShort(t *testing.T) {
	t.Parallel()
	const cropW, width, scaled = 608, 1920, 1084
	run := cropRun{marks: []frameMark{{k: 0, x: 400}, {k: 100, x: 700, glide: true}}}
	cut, off := glideCrop(run.at(0), cropW, width, scaled)
	was := float64(cut) + float64(off)*float64(cropW+glideMargin)/scaled
	for k := int64(1); k < 100; k++ {
		x := run.at(k)
		cut, off := glideCrop(x, cropW, width, scaled)
		at := float64(cut) + float64(off)*float64(cropW+glideMargin)/scaled
		if math.Abs(at-x) > 0.5*float64(cropW+glideMargin)/scaled+1e-9 {
			t.Errorf("on frame %d the glide is at %.3f, the short shows it from %.3f", k, x, at)
		}
		if step := at - was; k > 5 && k < 95 && step <= 0 {
			t.Errorf("on frame %d the short stands still or goes back, %.3f px of the picture", k, step)
		}
		was = at
	}
}

// The video preview takes the crop of each frame from the steps the engine
// sends, so it shows on every frame the crop the render crops by: the
// piece's start, and each switch inside it, on the frames heldX switches on.
func TestTheVideoPreviewIsSentTheCropsTheRenderCropsBy(t *testing.T) {
	t.Parallel()
	source := SourceInfo{Width: 480, Height: 270, FPSNum: 25, FPSDen: 1}
	view := SegmentView{Start: 0.4, End: 3.0, CropX: intPtr(50), Marks: []CropMarkView{
		{At: 0.2, X: 10}, {At: 0.8, X: 0}, {At: 1.6, X: 328}, {At: 1.64, X: 100}, {At: 2.0, X: 100}, {At: 2.6, X: 500}, {At: 3.2, X: 30},
	}}
	// Before the piece the keyframe at 0.2 s holds, 500 is kept inside the
	// picture, and the keyframes that change nothing, or come after the
	// piece, are no steps.
	want := []CropStep{{10, 10}, {20, 0}, {40, 328}, {41, 100}, {65, 328}}
	if got := source.CropSteps(view, 152, 480); !reflect.DeepEqual(got, want) {
		t.Errorf("the piece's crop steps are %v, not %v", got, want)
	}
	seg := Segment{Start: view.Start, End: view.End, CropX: view.CropX}
	for _, m := range view.Marks {
		seg.Marks = append(seg.Marks, CropMark{At: m.At, X: m.X, Glide: m.Glide})
	}
	run := source.runOf(seg, 152, 480)
	for k := int64(10); k < 75; k++ {
		if x, render := sentOn(want, k), run.at(k); x != render {
			t.Errorf("on frame %d the video preview is sent %v, the render crops at %v", k, x, render)
		}
	}

	// A glide is a step on every frame it moves, each where the render
	// has it.
	view.Marks[5].Glide = true
	seg.Marks[5].Glide = true
	steps := source.CropSteps(view, 152, 480)
	run = source.runOf(seg, 152, 480)
	for k := int64(10); k < 75; k++ {
		if x, render := sentOn(steps, k), run.at(k); x != render {
			t.Errorf("gliding, on frame %d the video preview is sent %v, the render crops at %v", k, x, render)
		}
	}
	if len(steps) < len(want)+8 {
		t.Errorf("the glide over 15 frames is sent as the steps %v", steps)
	}
	// A piece whose crop is its own throughout is sent none, and the
	// window shows its own crop.
	if got := source.CropSteps(SegmentView{Start: 1, End: 2, CropX: intPtr(50), Marks: []CropMarkView{{At: 1.5, X: 50}}}, 152, 480); got != nil {
		t.Errorf("a piece that never switches is sent the steps %v", got)
	}
}

// sentOn is the left edge the video preview takes from steps on frame k,
// the way lib/keyframes.ts looks it up.
func sentOn(steps []CropStep, k int64) float64 {
	x := steps[0].X
	for _, step := range steps {
		if step.Frame <= k {
			x = step.X
		}
	}
	return x
}

// renderedLefts runs the crop filters of a piece on a picture 480 wide
// whose every column says how far from the left it is, in 16 bits, and
// gives the column each frame starts on.
func renderedLefts(t *testing.T, source SourceInfo, seg Segment, frames int) []int {
	t.Helper()
	filters := source.cropFilters(seg, 152, 270, 0, 480, 152, 270, "flags=lanczos", "gray16le")
	got := readFrames(t, exec.Command("ffmpeg", "-v", "error", "-loop", "1", "-framerate", "25", "-t", "10",
		"-i", fixtures.Path(t, "ramp.png"),
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

// The render crops every frame where the crop is: the piece's own crop
// before its first keyframe, and from each keyframe's frame on, that
// keyframe's, switching from one frame to the next.
//
// On the real disk: ffmpeg renders the short.
func TestTheRenderSwitchesTheCropAtEachKeyframe(t *testing.T) {
	ffmpegtest.Need(t)
	t.Parallel()
	source := SourceInfo{Width: 480, Height: 270, FPSNum: 25, FPSDen: 1}
	seg := Segment{Start: 0.4, End: 3.0, CropX: intPtr(50), Marks: []CropMark{
		{At: 0.8, X: 0}, {At: 1.6, X: 328}, {At: 1.64, X: 100}, {At: 2.6, X: 200},
	}}
	first, _ := source.pieceFrames(seg.Start, seg.End)
	run := source.runOf(seg, 152, 480)
	for n, got := range renderedLefts(t, source, seg, 65) {
		if want := run.at(first + int64(n)); float64(got) != want {
			t.Errorf("frame %d of the piece is cropped at %d, not %v", n, got, want)
		}
	}

	// Scaled to the short, the crop that switches is one crop filter and
	// a scale, and the frames come out the short's size.
	e := NewEngine(runlog.New(&bytes.Buffer{}, false, false), NewMachine(disk))
	graph, _, _, err := e.BuildFilterGraph(context.Background(), Clip{Segments: []Segment{seg}},
		source, RenderSettings{OutW: 1080, OutH: 1920, ScaleUp: true}, "")
	if err != nil {
		t.Fatal(err)
	}
	chain := regexp.MustCompile(`crop=152:270:'if\([^']+':0,scale=1080:1920:flags=lanczos`).FindString(graph)
	if chain == "" {
		t.Fatalf("the piece has no crop that switches at its keyframes: %s", graph)
	}
	frames := readFrames(t, exec.Command("ffmpeg", "-v", "error", "-loop", "1", "-framerate", "25", "-t", "3",
		"-i", fixtures.Path(t, "ramp.png"), "-vf", chain, "-frames:v", "65", "-f", "rawvideo", "-pix_fmt", "gray", "-"), 1080*1920)
	if len(frames) != 65 {
		t.Errorf("the crop scaled to the short gave %d frames of 1080 by 1920, not 65", len(frames))
	}
}

// A keyframe that changes nothing, or one outside the piece, adds no
// switch, so a piece whose crop never changes is one still crop.
func TestAPieceWhoseCropNeverChangesIsCroppedStill(t *testing.T) {
	t.Parallel()
	source := SourceInfo{Width: 480, Height: 270, FPSNum: 25, FPSDen: 1}
	seg := Segment{Start: 1, End: 2, CropX: intPtr(50), Marks: []CropMark{
		{At: 0.5, X: 100}, {At: 1.5, X: 100}, {At: 2.5, X: 300},
	}}
	if got, want := source.cropFilters(seg, 152, 270, 0, 480, 152, 270, "flags=lanczos", "gray16le"), []string{"crop=152:270:100:0"}; !reflect.DeepEqual(got, want) {
		t.Errorf("the piece is cropped by %v, not %v", got, want)
	}
}

// A piece steered by as many keyframes as a clip may have renders, and
// switches at all of them. One if per keyframe would nest the expression
// past the hundred ffmpeg takes.
func TestTheRenderSwitchesAtAsManyKeyframesAsAClipMayHave(t *testing.T) {
	ffmpegtest.Need(t)
	t.Parallel()
	source := SourceInfo{Width: 480, Height: 270, FPSNum: 25, FPSDen: 1}
	seg := Segment{Start: 0, End: 9}
	for i := range MaxCropMarks {
		x := (i * 38) % 329
		seg.Marks = append(seg.Marks, CropMark{At: 0.2 + float64(i)*0.08, X: x - x%2})
	}
	run := source.runOf(seg, 152, 480)
	for n, got := range renderedLefts(t, source, seg, 215) {
		if want := run.at(int64(n)); float64(got) != want {
			t.Errorf("frame %d is cropped at %d, not %v", n, got, want)
		}
	}
}

// The render crops every frame where the glide is, to the pixel: the
// crop filters of a piece whose crop glides leave on each frame the column
// glideCrop puts the left edge on, within half a pixel of the glide, and
// a crop that switches or holds where it is.
//
// On the real disk: ffmpeg renders the short.
func TestTheRenderGlidesIntoACircleToThePixel(t *testing.T) {
	ffmpegtest.Need(t)
	t.Parallel()
	source := SourceInfo{Width: 480, Height: 270, FPSNum: 25, FPSDen: 1}
	seg := Segment{Start: 0.4, End: 3.0, CropX: intPtr(50), Marks: []CropMark{
		{At: 0.8, X: 0, Glide: true}, {At: 1.6, X: 328}, {At: 1.64, X: 100, Glide: true}, {At: 2.6, X: 201, Glide: true},
	}}
	first, _ := source.pieceFrames(seg.Start, seg.End)
	run := source.runOf(seg, 152, 480)
	for n, got := range renderedLefts(t, source, seg, 65) {
		x := run.at(first + int64(n))
		cut, off := glideCrop(x, 152, 480, 154)
		if got != cut+off || math.Abs(float64(got)-x) > 0.5 {
			t.Errorf("frame %d of the piece is cropped at %d, the crop is at %.3f", n, got, x)
		}
	}

	// Scaled to the short, the glide is cut glideMargin wider, scaled,
	// and cropped again, and the frames come out the short's size.
	e := NewEngine(runlog.New(&bytes.Buffer{}, false, false), NewMachine(disk))
	graph, _, _, err := e.BuildFilterGraph(context.Background(), Clip{Segments: []Segment{seg}},
		source, RenderSettings{OutW: 1080, OutH: 1920, ScaleUp: true}, "")
	if err != nil {
		t.Fatal(err)
	}
	chain := regexp.MustCompile(`crop=154:270:'[^']+':0,scale=1094:1920:flags=lanczos,format=yuv444p,crop=1080:1920:'[^']+':0:exact=1`).FindString(graph)
	if chain == "" {
		t.Fatalf("the piece has no crop that glides to the pixel of the short: %s", graph)
	}
	frames := readFrames(t, exec.Command("ffmpeg", "-v", "error", "-loop", "1", "-framerate", "25", "-t", "3",
		"-i", fixtures.Path(t, "ramp.png"), "-vf", chain, "-frames:v", "65", "-f", "rawvideo", "-pix_fmt", "gray", "-"), 1080*1920)
	if len(frames) != 65 {
		t.Errorf("the glide scaled to the short gave %d frames of 1080 by 1920, not 65", len(frames))
	}
}

// A piece steered by as many keyframes as a clip may have, every other one
// a circle, renders, and glides into each circle.
func TestTheRenderGlidesIntoAsManyKeyframesAsAClipMayHave(t *testing.T) {
	ffmpegtest.Need(t)
	t.Parallel()
	source := SourceInfo{Width: 480, Height: 270, FPSNum: 25, FPSDen: 1}
	seg := Segment{Start: 0, End: 9}
	for i := range MaxCropMarks {
		seg.Marks = append(seg.Marks, CropMark{At: 0.2 + float64(i)*0.08, X: (i * 37) % 329, Glide: i%2 == 1})
	}
	run := source.runOf(seg, 152, 480)
	for n, got := range renderedLefts(t, source, seg, 215) {
		x := run.at(int64(n))
		if cut, off := glideCrop(x, 152, 480, 154); got != cut+off {
			t.Errorf("frame %d is cropped at %d, the crop is at %.3f", n, got, x)
		}
	}
}

// A mark steers every piece of the clip turned alike, cuts and all. Across
// a cut the crop holds by the clip's clock, the time the short plays, so
// a piece is steered by the marks placed where they fall on its own
// stretch of the video as if the cuts were not there. A mark in a cut steers nothing,
// and comes back with its piece.
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

	// A keyframe holds across the cut, by the clip's clock: the crop
	// before it is the keyframe at 10.8 s, and after it the one at 11.6 s,
	// which is 1.6 s into the clip, before the 2 s the cut ends at on the
	// clip's clock. Before the first keyframe the crop is the clip's own.
	first, last := c.Segments[0], c.Segments[2]
	for _, h := range []struct {
		seg  Segment
		at   float64
		want float64
	}{
		{first, 10.0, 120}, {first, first.End - 0.04, 200},
		{last, last.Start, 500}, {last, 13.56, 500}, {last, 13.6, 50},
	} {
		if got := at25.CropAt(h.seg, h.at, 608, 1920); got != h.want {
			t.Errorf("at %.2f s the crop stands at %v, not %v", h.at, got, h.want)
		}
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
		map[string]any{"at": 10.2, "x": 40.0, "glide": "true"},
		map[string]any{"at": 11.2, "x": 40.0, "glide": true},
	}, nil)
	want := []CropMark{{At: 10.2, X: 40}, {At: 10.5, X: 100}, {At: 11.2, X: 40, Glide: true}}
	if !reflect.DeepEqual(got, want) {
		t.Errorf("the plan's keyframes read as %v, not %v", got, want)
	}
}

// K puts a mark on the start of the frame it means, a drag moves it to
// another frame with its crop, and a double-click removes it. A mark has
// to be in the clip, and one frame holds one.
func TestACropMarkIsPutMovedAndRemovedLikeAThumbnail(t *testing.T) {
	fsys, path := rotatablePlanPath(t)
	if err := SetCropMark(fsys, path, "01", -1, 10.557, 300, false, at25); err != nil {
		t.Fatal(err)
	}
	if got := loadedClip(t, fsys, path).CropMarks; len(got) != 1 || got[0] != (CropMark{At: 10.52, X: 300}) {
		t.Fatalf("K at 10.557 s made %v, not a mark on the frame from 10.52 s", got)
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
		if err := SetCropMark(fsys, path, "01", c.from, c.to, 100, false, at25); err == nil {
			t.Errorf("a keyframe %s was taken", c.name)
		}
	}
	if err := SetCropMark(fsys, path, "01", 10.52, 11.3, -1, false, at25); err != nil {
		t.Fatal(err)
	}
	if got := loadedClip(t, fsys, path).CropMarks; len(got) != 1 || got[0] != (CropMark{At: 11.28, X: 300}) {
		t.Fatalf("the mark dragged to 11.3 s is %v, not on the frame from 11.28 s", got)
	}
	if err := SetCropMark(fsys, path, "01", 11.28, -1, -1, false, at25); err != nil {
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

// Tim's camera switch, #750: one keyframe where the camera switches, put
// with K with the crop where it stands, and the crop dragged with the
// playhead on it. The part before keeps the clip's own crop, and the crop
// switches on the keyframe's own frame.
func TestOneKeyframeIsACameraSwitch(t *testing.T) {
	fsys, path := rotatablePlanPath(t)
	if err := SetCrop(fsys, path, "01", 10.2, 40, at25); err != nil {
		t.Fatal(err)
	}
	cropMarks(t, fsys, path, CropMark{At: 10.6, X: 40})
	if err := SetCrop(fsys, path, "01", 10.6, 600, at25); err != nil {
		t.Fatal(err)
	}
	c := loadedClip(t, fsys, path)
	for _, h := range []struct {
		piece int
		at    float64
		want  float64
	}{
		{0, 10.0, 40}, {0, 10.56, 40}, {0, 10.6, 600}, {1, 11.5, 600}, {2, 13.9, 600},
	} {
		if got := at25.CropAt(c.Segments[h.piece], h.at, 608, 1920); got != h.want {
			t.Errorf("at %.2f s the crop stands at %v, not %v", h.at, got, h.want)
		}
	}
}

// The crop is placed with the playhead on a keyframe, Tim on #750, and
// moves that keyframe, a diamond or a circle. Anywhere else in a piece the
// keyframes steer, the crop frame shows the crop as the short has it and
// a drag moves nothing. A piece no keyframe steers has one crop, the
// clip's, placed anywhere in it. Automatic crop removes the keyframes.
func TestTheCropIsPlacedOnlyOnAKeyframe(t *testing.T) {
	fsys, path := rotatablePlanPath(t)
	if err := SetCrop(fsys, path, "01", 11.5, 80, at25); err != nil {
		t.Fatal(err)
	}
	cropMarks(t, fsys, path, CropMark{At: 10.2, X: 100}, CropMark{At: 10.8, X: 200, Glide: true}, CropMark{At: 13.6, X: 60})
	marked := func(want ...CropMark) {
		t.Helper()
		if got := loadedClip(t, fsys, path).CropMarks; !reflect.DeepEqual(got, want) {
			t.Fatalf("the keyframes are %v, not %v", got, want)
		}
	}

	// On the circle at 10.8 s.
	if err := SetCrop(fsys, path, "01", 10.8, 300, at25); err != nil {
		t.Fatal(err)
	}
	marked(CropMark{At: 10.2, X: 100}, CropMark{At: 10.8, X: 300, Glide: true}, CropMark{At: 13.6, X: 60})

	// Between keyframes, after one, before the first and after a cut:
	// nothing moves.
	for _, at := range []float64{10.5, 11.5, 10.1, 13.2} {
		if err := SetCrop(fsys, path, "01", at, 400, at25); err == nil {
			t.Errorf("a crop dragged at %.2f s, on no keyframe, was taken", at)
		}
	}
	marked(CropMark{At: 10.2, X: 100}, CropMark{At: 10.8, X: 300, Glide: true}, CropMark{At: 13.6, X: 60})
	if seg := pieceAt(t, fsys, path, 13.2); seg.CropX == nil || *seg.CropX != 80 {
		t.Errorf("the crop of the clip is %v, not 80", seg.CropX)
	}

	if err := ResetCrop(fsys, path, "01", 10.5); err != nil {
		t.Fatal(err)
	}
	if got := loadedClip(t, fsys, path).CropMarks; len(got) != 0 {
		t.Errorf("automatic crop left the keyframes %v", got)
	}
}

// K puts a diamond and G a circle, a drag along the clip timeline keeps a
// keyframe's kind, and G or K on one of the other kind turns it into its
// own. The plan says glide only for a circle.
func TestAKeyframeIsPutAsADiamondOrACircle(t *testing.T) {
	fsys, path := rotatablePlanPath(t)
	if err := SetCropMark(fsys, path, "01", -1, 10.5, 300, true, at25); err != nil {
		t.Fatal(err)
	}
	if err := SetCropMark(fsys, path, "01", -1, 11.0, 100, false, at25); err != nil {
		t.Fatal(err)
	}
	if got, want := loadedClip(t, fsys, path).CropMarks, []CropMark{{At: 10.48, X: 300, Glide: true}, {At: 11, X: 100}}; !reflect.DeepEqual(got, want) {
		t.Fatalf("G at 10.5 s and K at 11 s made %v, not %v", got, want)
	}
	if err := SetCropMark(fsys, path, "01", 10.48, 10.2, -1, false, at25); err != nil {
		t.Fatal(err)
	}
	if err := SetCropGlide(fsys, path, "01", 11.0, true, at25); err != nil {
		t.Fatal(err)
	}
	if err := SetCropGlide(fsys, path, "01", 10.2, false, at25); err != nil {
		t.Fatal(err)
	}
	if got, want := loadedClip(t, fsys, path).CropMarks, []CropMark{{At: 10.2, X: 300}, {At: 11, X: 100, Glide: true}}; !reflect.DeepEqual(got, want) {
		t.Fatalf("the circle dragged to 10.2 s and both turned are %v, not %v", got, want)
	}
	if err := SetCropGlide(fsys, path, "01", 10.6, true, at25); err == nil {
		t.Error("a frame with no keyframe was made to glide")
	}
	data, _ := afero.ReadFile(fsys, path)
	if n := bytes.Count(data, []byte(`"glide"`)); n != 1 {
		t.Errorf("the plan says glide %d times for one circle: %s", n, data)
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
	if err := SetCropMark(fsys, path, "01", 10.55, 10.9, -1, false, at25); err != nil {
		t.Fatalf("the mark at 10.55 s could not be dragged: %v", err)
	}
	if err := SetCropMark(fsys, path, "01", 11.015, -1, -1, false, at25); err != nil {
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
// the crop the part had unrotated does. Four turns bring it back.
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
			t.Errorf("turned %d quarters the part is steered by keyframes placed upright, %v", turns, steering)
		}
		if turns == 4 && (len(steering) != 2 || steering[1].X != 50) {
			t.Errorf("four turns on the part is steered by %v, not the keyframes it had", steering)
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
	if err := SetCrop(fsys, path, "01", 13.52, 200, at25); err != nil {
		t.Fatal(err)
	}
	want := []CropMark{{At: 13.52, X: 200, Turn: 90}}
	if got := loadedClip(t, fsys, path).CropMarks; !reflect.DeepEqual(got, want) {
		t.Errorf("the keyframe under the crop placed is %v, not %v", got, want)
	}
	if got := pieceAt(t, fsys, path, 13.5).Marks; len(got) != 1 || got[0].X != 200 {
		t.Errorf("the rotated part is steered by %v", got)
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
		t.Errorf("the rotated part is steered by %v", got)
	}
}

// A keyframe a cut or a trim covers still switches the crop, from the
// first frame kept after it. A camera switch often lies in the pause a cut
// takes out, and the keyframe there was dropped, so the new angle played
// with the old crop, found by the review of #752.
func TestAKeyframeACutOrATrimCoversStillSwitchesTheCrop(t *testing.T) {
	for _, c := range []struct {
		name  string
		shape Gesture
		piece int
		at    float64
	}{
		{"a trim of the start past it", Gesture{Kind: "trim", Edge: "start", From: 10.8}, 0, 10.9},
		{"a cut over it", Gesture{Kind: "cut", From: 10.4, To: 10.8}, 1, 10.9},
	} {
		t.Run(c.name, func(t *testing.T) {
			fsys, path := rotatablePlanPath(t)
			cropMarks(t, fsys, path, CropMark{At: 10.6, X: 600})
			reshape(t, fsys, path, c.shape)
			clip := loadedClip(t, fsys, path)
			if len(clip.CropMarks) != 0 {
				t.Errorf("the hidden keyframe is on screen: %v", clip.CropMarks)
			}
			for i, seg := range clip.Segments {
				if seg.End <= c.at && i < c.piece {
					continue
				}
				at := max(c.at, seg.Start)
				if got := at25.CropAt(seg, at, 608, 1920); got != 600 {
					t.Errorf("piece %d at %.2f s has the crop at %v, not the hidden keyframe's 600", i, at, got)
				}
			}
			// The piece is steered, so a drag there, on no keyframe on
			// screen, moves nothing.
			if err := SetCrop(fsys, path, "01", c.at, 500, at25); err == nil {
				t.Error("a crop dragged on no keyframe was taken")
			}
		})
	}
}

// A glide into the first keyframe starts where the clip does, from the
// clip's own crop, and holds after it across a cut.
func TestAGlideIntoTheFirstKeyframeStartsWhereTheClipDoes(t *testing.T) {
	fsys, path := rotatablePlanPath(t)
	cropMarks(t, fsys, path, CropMark{At: 11.52, X: 600, Glide: true})
	c := loadedClip(t, fsys, path)
	for _, h := range []struct {
		piece int
		at    float64
		want  float64
	}{
		{0, 10.0, 120}, {0, 10.76, 360}, {1, 11.52, 600}, {2, 13.5, 600},
	} {
		if got := at25.CropAt(c.Segments[h.piece], h.at, 608, 1920); math.Abs(got-h.want) > 1e-9 {
			t.Errorf("at %.2f s the crop stands at %v, not %v", h.at, got, h.want)
		}
	}
}
