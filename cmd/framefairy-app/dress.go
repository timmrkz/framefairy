package main

import (
	"encoding/json"
	"fmt"
	"hash/maphash"
	"strconv"

	"framefairy/engine"
)

// framed is what the frames of a clip are dressed from, as the window
// was sent it in a Framing: the episode, its picture, the clip's pieces
// with the keyframes that steer each, the keyframes on their frames, its
// rotations and the size of the short, see engine.SourceInfo.Dress.
type framed struct {
	episode   string
	source    engine.SourceInfo
	segments  []engine.SegmentView
	marks     []engine.CropMarkView
	rotations []engine.Rotation
	width     int
	height    int
}

// name is what tells this framing from any other: all it dresses a frame
// from, so a clip that changed anything over its frames has a new name,
// and one that changed nothing keeps it.
func (f framed) name(rotations []engine.RotationView) string {
	var h maphash.Hash
	h.SetSeed(clipSeed)
	fmt.Fprintf(&h, "%s\x00%+v\x00%dx%d\x00", f.episode, f.source, f.width, f.height)
	_ = json.NewEncoder(&h).Encode([]any{f.segments, f.marks, rotations})
	return strconv.FormatUint(h.Sum64(), 36)
}

// dress is the dress of the frame that starts at a moment, as the frame
// of the clip, or with onVideo as the frame of the video, which shows the
// episode as filmed and is no frame of the clip's.
func (f framed) dress(at float64, onVideo bool) engine.FrameDress {
	rotations := f.rotations
	if onVideo {
		rotations = nil
	}
	d := f.source.Dress(f.segments, rotations, f.marks, at, f.width, f.height)
	if onVideo {
		d.In, d.Clip = false, false
	}
	return d
}

// layerSize is the size the captions over a dressed frame are drawn at,
// in device pixels: the crop as the video preview of width by height
// shows the picture turned, or nothing where no crop stands.
func layerSize(d engine.FrameDress, width, height int) (int, int) {
	if d.Width <= 0 || d.CropWidth <= 0 {
		return 0, 0
	}
	if d.Turn%180 != 0 {
		width, height = height, width
	}
	return int(float64(d.CropWidth)/float64(d.Width)*float64(width) + 0.5), height
}
