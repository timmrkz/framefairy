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
	// Clip is whether the frame shows a moment between the clip's first
	// piece's start and its last piece's end, which the clip's captions
	// are drawn over.
	Clip bool `json:"clip"`
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
	// Auto is where the app places the crop on this frame, on the picture
	// turned, and Moved whether the crop stands anywhere else, so moving
	// the crop frame to Auto would change what the frame shows. A crop
	// placed by hand where the app placed it, or a keyframe on that
	// place, is not moved.
	Auto  float64 `json:"auto"`
	Moved bool    `json:"moved"`
	// Mark is the keyframe on the frame, or nil, and Movable whether a
	// drag may place the crop here: on a keyframe that does not open a
	// glide, or anywhere in a piece no keyframe steers, see SetCrop.
	Mark    *CropMarkView `json:"mark"`
	Movable bool          `json:"movable"`
	// From and To are the moments the stretch of frames around this one
	// that are dressed alike starts and ends at, at most dressReach frames
	// either side, so the video preview dresses a frame of it whose own
	// dress is still on its way, while the hand drags the playhead, with
	// this one. Dressed with the frame before, a frame past a cut stood
	// turned the way the part before it was turned.
	From float64 `json:"from"`
	To   float64 `json:"to"`
}

// dressReach is how many frames either side of a frame its stretch of
// frames dressed alike reaches at most, see FrameDress.From: five seconds
// of a video of 25 frames a second, more than a drag passes over before
// the dress of a frame on its way comes.
const dressReach = 125

// Dress is the dress of the frame that starts at a moment, for a clip of
// these pieces, turned by rotations, with these keyframes on their frames,
// see MarksOnFrames, cut to a short of outW by outH.
func (s SourceInfo) Dress(segments []SegmentView, rotations []Rotation, marks []CropMarkView, at float64, outW, outH int) FrameDress {
	runs := map[[3]int]cropRun{}
	d := s.dressOf(segments, rotations, marks, at, outW, outH, runs)
	d.From, d.To = at, at
	if s.FPSNum <= 0 || s.FPSDen <= 0 {
		return d
	}
	k := s.frameHolding(at)
	alike := func(j int64) bool {
		return j >= 0 && sameDress(d, s.dressOf(segments, rotations, marks, s.keptAt(j), outW, outH, runs))
	}
	lo, hi := k, k
	for lo > k-dressReach && alike(lo-1) {
		lo--
	}
	for hi < k+dressReach && alike(hi+1) {
		hi++
	}
	d.From, d.To = s.keptAt(lo), s.keptAt(hi+1)
	return d
}

// DressAlone is the dress of the frame that starts at a moment, as Dress has
// it, without the stretch of frames dressed alike around it: for asking
// where a framing puts the crop on one frame, which a stretch would ask
// up to dressReach frames either side for.
func (s SourceInfo) DressAlone(segments []SegmentView, rotations []Rotation, marks []CropMarkView, at float64, outW, outH int) FrameDress {
	d := s.dressOf(segments, rotations, marks, at, outW, outH, map[[3]int]cropRun{})
	d.From, d.To = at, at
	return d
}

// sameDress says two frames are dressed alike, their stretches aside.
func sameDress(a, b FrameDress) bool {
	if (a.Mark == nil) != (b.Mark == nil) || a.Mark != nil && *a.Mark != *b.Mark {
		return false
	}
	a.Mark, b.Mark = nil, nil
	a.From, a.To, b.From, b.To = 0, 0, 0, 0
	return a == b
}

// dressOf is the dress of a frame, its stretch aside, with the crop's run
// over each piece kept in runs by the piece, the crop's width and the picture's.
func (s SourceInfo) dressOf(segments []SegmentView, rotations []Rotation, marks []CropMarkView, at float64, outW, outH int, runs map[[3]int]cropRun) FrameDress {
	d := FrameDress{Piece: -1, Turn: RotationAt(rotations, at)}
	for i, seg := range segments {
		if s.holdsFrame(seg, at) {
			d.Piece, d.In = i, true
			break
		}
	}
	if len(segments) > 0 {
		length := 0.001
		if s.FPSNum > 0 && s.FPSDen > 0 {
			length = float64(s.FPSDen) / float64(s.FPSNum)
		}
		d.Clip = at+length > segments[0].Start+1e-6 && at < segments[len(segments)-1].End-1e-6
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
	k := s.frameHolding(at)
	d.Left = float64(ClampCropX(seg.CropX, d.CropWidth, upright.Width))
	d.Auto = float64(ClampCropX(seg.Auto, d.CropWidth, upright.Width))
	if len(seg.Marks) > 0 && s.FPSNum > 0 && s.FPSDen > 0 {
		// A frame of a cut beside the piece shows the crop of the
		// piece's frame nearest to it, its first or its last.
		first, end := s.pieceFrames(seg.Start, seg.End)
		key := [3]int{d.Piece, d.CropWidth, upright.Width}
		run, ok := runs[key]
		if !ok {
			run = s.runOf(segmentOf(seg), d.CropWidth, upright.Width)
			runs[key] = run
		}
		d.Left = stepEdge(run.at(min(max(k, first), end-1)))
	}
	d.Moved = d.Left != d.Auto
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

// holdsFrame says a piece holds the frame that starts at a moment: the
// frame its start falls in, up to the one its end falls in, so the frame
// the video preview shows for a moment of the piece is the piece's, also
// where an edge lies inside a frame. A piece ends where the frame its end
// starts begins, the frame before it is its last.
func (s SourceInfo) holdsFrame(seg SegmentView, at float64) bool {
	if s.FPSNum <= 0 || s.FPSDen <= 0 {
		return at >= seg.Start-frameHair && at < seg.End-frameHair
	}
	k := s.frameHolding(at)
	return k >= s.frameHolding(seg.Start) && k <= s.frameHolding(seg.End-2*frameHair) && seg.End > seg.Start
}
