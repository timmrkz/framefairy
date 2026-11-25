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
		if !d.In || d.Piece != 0 || d.CropWidth != 152 || d.Width != 480 || d.Height != 270 {
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
// piece holds it.
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
	}{
		{1.2, 0, 20},
		{1.8, 1, 100},
		{3.4, 1, 300},
	} {
		d := source.Dress(pieces, nil, nil, c.at, 1080, 1920)
		if d.In || d.Piece != c.piece || d.Left != c.left {
			t.Errorf("the frame at %.1f s is dressed %+v, not piece %d with the crop at %v", c.at, d, c.piece, c.left)
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
