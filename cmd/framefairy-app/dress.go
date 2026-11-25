package main

import (
	"context"
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

// pieces is what a play of the framing's clip plays, see servePlay.
func (f framed) pieces() []engine.PlayPiece {
	out := make([]engine.PlayPiece, len(f.segments))
	for i, seg := range f.segments {
		out[i] = engine.PlayPiece{Start: seg.Start, End: seg.End}
	}
	return out
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

// dressSent is a frame's dress as the window is sent it, with the dress
// the frame has once R turns the part that holds it, see Turns.
type dressSent struct {
	engine.FrameDress
	Turned *turnedDress `json:"turned,omitempty"`
}

// turnedDress is a frame's dress with its part turned a quarter further,
// and the framing that turns it so, which the clip has once R is pressed.
// The video preview turns the frame R is pressed on by it, in the frame
// it is pressed in, until the frame's own dress under that framing comes.
// It was read ahead for the frame at the playhead after every step, a
// layer of captions libass drew for each, and the next step's frame
// waited behind it.
type turnedDress struct {
	Framing string `json:"framing"`
	engine.FrameDress
}

// dressSent is the dress of the frame at a moment, d, of the clip whose
// framing is named name, with its dress turned where a piece of the clip
// holds the frame and R would turn that piece's part.
func (s *FrameFairy) dressSent(ctx context.Context, path, name string, clip framed, d engine.FrameDress, at float64) dressSent {
	out := dressSent{FrameDress: d}
	if !d.In || d.Piece < 0 || d.Piece >= len(clip.segments) {
		return out
	}
	parts, ok := s.previews.turns.get(name)
	if !ok {
		parts = s.turnsFor(ctx, path, name)
	}
	turned := parts[clip.segments[d.Piece].Part]
	if turned == "" {
		return out
	}
	if g, ok := s.previews.framings.get(turned); ok && g.episode == clip.episode {
		out.Turned = &turnedDress{Framing: turned, FrameDress: g.dress(at, false)}
	}
	return out
}

// clipRef is the clip of a plan a framing of the clip list is of.
type clipRef struct{ plan, id string }

// turnsFor works out the framings R turns each part of the clip whose
// framing is named name to, and keeps them by it, for a clip whose frame
// was dressed before a state said them: a clip whose card was read ahead
// and then clicked, or the answer of an edit. R pressed on its frame
// turned it only once the turned layer came. Kept empty where there are
// none to be had, so a play of a clip asks the plan once, not each frame.
func (s *FrameFairy) turnsFor(ctx context.Context, path, name string) map[int]string {
	none := map[int]string{}
	ref, ok := s.previews.clips.get(name)
	if !ok {
		return none
	}
	entry, err := s.clipEntry(ctx, path, ref.plan, ref.id)
	info, perr := s.probe(ctx, path)
	if err != nil || perr != nil || entry.Framing != name {
		s.previews.turns.put(name, none, nil)
		return none
	}
	s.turnsOf(path, s.plansOf(path), entry.Key, []ClipEntry{entry}, info)
	if parts, ok := s.previews.turns.get(name); ok {
		return parts
	}
	return none
}
