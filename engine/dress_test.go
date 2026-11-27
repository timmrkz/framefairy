package engine

import (
	"math"
	"testing"
)

// The video preview draws over each frame the crop the render crops that
// frame by, so the dress of every frame of a piece steered by keyframes,
// switching and gliding, is where the render has the crop.
func TestADressedFrameHasTheCropTheRenderCropsBy(t *testing.T) {
	t.Parallel()
	source := SourceInfo{Width: 480, Height: 270, FPSNum: 25, FPSDen: 1}
	view := SegmentView{Start: 0.4, End: 3.0, CropX: intPtr(50), Marks: []CropMarkView{
		{At: 0.2, X: 10}, {At: 0.8, X: 0}, {At: 1.6, X: 328}, {At: 2.0, X: 100, Glide: true}, {At: 2.6, X: 300, Glide: true},
	}}
	seg := segmentOf(view)
	run := source.runOf(seg, 152, 480)
	for k := int64(10); k < 75; k++ {
		at := source.keptAt(k)
		d := source.Dress([]SegmentView{view}, nil, view.Marks, at, 1080, 1920)
		if !d.In || !d.Clip || d.Piece != 0 || d.CropWidth != 152 || d.Width != 480 || d.Height != 270 {
			t.Fatalf("frame %d at %.3f s is dressed %+v", k, at, d)
		}
		if render := run.at(k); math.Abs(d.Left-render) > 0.005 {
			t.Errorf("frame %d is dressed with the crop at %v, the render crops at %v", k, d.Left, render)
		}
		if !d.Moved {
			t.Errorf("frame %d of a piece keyframes steer is dressed as not moved", k)
		}
	}
}

// A frame of a cut, or from outside the clip, shows the crop of the piece
// nearest to it, as that piece's own frame next to it has it, and says no
// piece holds it. A cut inside the clip is still the clip's, for its
// captions.
func TestAFrameOfACutIsDressedWithTheNearestPiecesCrop(t *testing.T) {
	t.Parallel()
	source := SourceInfo{Width: 480, Height: 270, FPSNum: 25, FPSDen: 1}
	first := SegmentView{Start: 0, End: 1, CropX: intPtr(20)}
	second := SegmentView{Start: 2, End: 3, CropX: intPtr(200), Marks: []CropMarkView{{At: 2, X: 100}, {At: 2.5, X: 300}}}
	pieces := []SegmentView{first, second}
	for _, c := range []struct {
		at    float64
		piece int
		left  float64
		clip  bool
	}{
		{1.2, 0, 20, true},
		{1.8, 1, 100, true},
		{3.4, 1, 300, false},
	} {
		d := source.Dress(pieces, nil, nil, c.at, 1080, 1920)
		if d.In || d.Piece != c.piece || d.Left != c.left || d.Clip != c.clip {
			t.Errorf("the frame at %.1f s is dressed %+v, not piece %d with the crop at %v, in the clip %v", c.at, d, c.piece, c.left, c.clip)
		}
	}
}

// A piece whose edges lie inside frames holds the frame its start falls
// in and the one its end falls in, which the video preview shows for
// those moments, and the crop frame stands over them.
func TestAPieceHoldsTheFramesItsEdgesFallIn(t *testing.T) {
	t.Parallel()
	source := SourceInfo{Width: 480, Height: 270, FPSNum: 25, FPSDen: 1}
	pieces := []SegmentView{{Start: 1.01, End: 2.01}, {Start: 3, End: 4}}
	for _, c := range []struct {
		at float64
		in bool
	}{{0.96, false}, {1, true}, {1.5, true}, {2, true}, {2.04, false}, {2.96, false}, {3, true}, {3.96, true}, {4, false}} {
		if d := source.Dress(pieces, nil, nil, c.at, 1080, 1920); d.In != c.in {
			t.Errorf("the frame at %.2f s is held by a piece %v, want %v", c.at, d.In, c.in)
		}
	}
}

// A frame in a stretch a person rotated is turned, the picture is as large
// as it stands turned, and the crop is cut out of it.
func TestARotatedFrameIsDressedTurned(t *testing.T) {
	t.Parallel()
	source := SourceInfo{Width: 1920, Height: 1080, FPSNum: 25, FPSDen: 1}
	pieces := []SegmentView{{Start: 0, End: 2}, {Start: 2, End: 4, Rotate: 90}}
	turns := []Rotation{{From: 2, To: math.Inf(1), Deg: 90}}
	upright := source.Dress(pieces, turns, nil, 1, 1080, 1920)
	turned := source.Dress(pieces, turns, nil, 3, 1080, 1920)
	if upright.Turn != 0 || upright.Width != 1920 || upright.CropWidth != 608 {
		t.Errorf("a frame before the rotated stretch is dressed %+v", upright)
	}
	if turned.Turn != 90 || turned.Width != 1080 || turned.Height != 1920 || turned.CropWidth != 1080 {
		t.Errorf("a frame of the rotated stretch is dressed %+v", turned)
	}
}

// The keyframe on a frame comes with its dress, and a drag may place the
// crop only on a keyframe that does not open a glide, or in a piece no
// keyframe steers, the way the engine takes a crop.
func TestADressSaysTheKeyframeOnItsFrameAndWhetherTheCropMoves(t *testing.T) {
	t.Parallel()
	source := SourceInfo{Width: 480, Height: 270, FPSNum: 25, FPSDen: 1}
	marks := []CropMarkView{{At: 1.2, X: 40}, {At: 1.6, X: 300, Glide: true, Opens: true}}
	steered := SegmentView{Start: 1, End: 2, CropX: intPtr(50), Marks: marks}
	free := SegmentView{Start: 3, End: 4, CropX: intPtr(50)}
	pieces := []SegmentView{steered, free}
	for _, c := range []struct {
		at      float64
		mark    float64
		movable bool
	}{
		{1.2, 1.2, true},
		{1.22, 1.2, true},
		{1.4, -1, false},
		{1.6, 1.6, false},
		{3.5, -1, true},
	} {
		d := source.Dress(pieces, nil, marks, c.at, 1080, 1920)
		got := -1.0
		if d.Mark != nil {
			got = d.Mark.At
		}
		if got != c.mark || d.Movable != c.movable {
			t.Errorf("the frame at %.2f s has the keyframe at %v and movable %v, not %v and %v", c.at, got, d.Movable, c.mark, c.movable)
		}
	}
}

// A dress says the stretch of frames around its frame dressed alike, so
// the video preview can dress a frame of it whose own dress is on its
// way: a part turned ends it, a cut ends it, and a keyframe stands alone.
func TestADressSaysTheFramesDressedAlike(t *testing.T) {
	t.Parallel()
	source := SourceInfo{Width: 1920, Height: 1080, FPSNum: 25, FPSDen: 1}
	pieces := []SegmentView{{Start: 0, End: 2}, {Start: 2, End: 4, Rotate: 90}, {Start: 5, End: 30, CropX: intPtr(100), Marks: []CropMarkView{{At: 6, X: 400}}}}
	turns := []Rotation{{From: 2, To: 4, Deg: 90}}
	marks := []CropMarkView{{At: 6, X: 400}}
	for _, c := range []struct {
		at, from, to float64
	}{
		{1, 0, 2},
		{2.4, 2, 4},
		{4.4, 4, 4.52},
		{5.2, 5, 6},
		{6, 6, 6.04},
		{7, 6.04, 12.04},
	} {
		d := source.Dress(pieces, turns, marks, c.at, 1080, 1920)
		if math.Abs(d.From-c.from) > 1e-9 || math.Abs(d.To-c.to) > 1e-9 {
			t.Errorf("the frame at %v s is dressed alike from %v to %v, want %v to %v", c.at, d.From, d.To, c.from, c.to)
		}
	}
}

// How long a frame takes to dress, with the stretch of frames dressed
// alike around it, in a clip of three pieces with keyframes.
func BenchmarkADress(b *testing.B) {
	source := SourceInfo{Width: 1920, Height: 1080, FPSNum: 25, FPSDen: 1}
	marks := []CropMarkView{{At: 6, X: 400}, {At: 9, X: 100, Glide: true}, {At: 20, X: 700}}
	pieces := []SegmentView{{Start: 0, End: 2}, {Start: 2, End: 4, Rotate: 90}, {Start: 5, End: 30, CropX: intPtr(100), Marks: marks}}
	turns := []Rotation{{From: 2, To: 4, Deg: 90}}
	for b.Loop() {
		source.Dress(pieces, turns, marks, 14, 1080, 1920)
	}
}
