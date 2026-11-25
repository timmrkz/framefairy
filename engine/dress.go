package engine

import "math"

// FrameDress is what the video preview draws over a frame of a clip,
// besides the captions: which piece holds the frame, which way the picture
// is turned, how large the picture is turned that way, where the short's
// crop stands on it, and the keyframe on the frame. It is worked out here,
// for the frame the window draws, the way the render crops and turns the
// same frame, so the window keeps no rule about pieces, turns or
// keyframes, see docs/decisions/2026-11-25-the-video-preview-draws-what-go-sends.md.
type FrameDress struct {
	// Piece is the piece that holds the frame, or the one nearest to it
	// for a frame of a cut or from outside the clip, whose crop the video
	// preview shows over it. In is whether a piece holds it.
	Piece int  `json:"piece"`
	In    bool `json:"in"`
	// Turn is how far the frame is rotated to the left, in degrees, see
	// RotationAt.
	Turn int `json:"turn"`
	// Width and Height are the picture turned, in pixels of the video, and
	// CropWidth how wide the short's crop is on it.
	Width     int `json:"width"`
	Height    int `json:"height"`
	CropWidth int `json:"cropWidth"`
	// Left is the left edge of the crop on the frame, on the picture
	// turned, held from a keyframe or gliding, or the piece's own.
	Left float64 `json:"left"`
	// Moved is whether the piece's crop was placed by hand or keyframes
	// steer it.
	Moved bool `json:"moved"`
	// Mark is the keyframe on the frame, or nil, and Movable whether a
	// drag may place the crop here: on a keyframe that does not open a
	// glide, or anywhere in a piece no keyframe steers, see SetCrop.
	Mark    *CropMarkView `json:"mark"`
	Movable bool          `json:"movable"`
}

// Dress is the dress of the frame that starts at a moment, for a clip of
// these pieces, turned by rotations, with these keyframes on their frames,
// see MarksOnFrames, cut to a short of outW by outH.
func (s SourceInfo) Dress(segments []SegmentView, rotations []Rotation, marks []CropMarkView, at float64, outW, outH int) FrameDress {
	d := FrameDress{Piece: -1, Turn: RotationAt(rotations, at)}
	for i, seg := range segments {
		if at >= seg.Start-frameHair && at < seg.End-frameHair {
			d.Piece, d.In = i, true
			break
		}
	}
	if d.Piece < 0 {
		near := math.Inf(1)
		for i, seg := range segments {
			gap := math.Max(seg.Start-at, at-seg.End)
			if gap < near {
				near, d.Piece = gap, i
			}
		}
	}
	upright := s.Rotated(d.Turn)
	d.Width, d.Height = upright.Width, upright.Height
	if d.Piece < 0 || upright.Width <= 0 || upright.Height <= 0 {
		return d
	}
	seg := segments[d.Piece]
	d.CropWidth, _ = CropWindow(upright, outW, outH)
	d.Moved = seg.Moved || len(seg.Marks) > 0
	k := s.frameHolding(at)
	d.Left = float64(ClampCropX(seg.CropX, d.CropWidth, upright.Width))
	if len(seg.Marks) > 0 && s.FPSNum > 0 && s.FPSDen > 0 {
		// A frame of a cut beside the piece shows the crop of the
		// piece's frame nearest to it, its first or its last.
		first, end := s.pieceFrames(seg.Start, seg.End)
		d.Left = stepEdge(s.runOf(segmentOf(seg), d.CropWidth, upright.Width).at(min(max(k, first), end-1)))
	}
	for i := range marks {
		if s.markFrame(marks[i].At) == k {
			m := marks[i]
			d.Mark = &m
			break
		}
	}
	d.Movable = len(seg.Marks) == 0 || (d.Mark != nil && !d.Mark.Opens)
	return d
}
