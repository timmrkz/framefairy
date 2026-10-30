package engine

import (
	"fmt"
	"math"
	"sort"

	"framefairy/engine/num"
	"framefairy/engine/runes"
)

// A phone turned while it records keeps writing the picture the way it
// began, so the footage after the turn lies on its side until the phone is
// turned back. That is a fact about the video's time, not about how a
// clip is cut, so a clip keeps its rotations as stretches of the video's
// time, in the plan as rotations. Cutting, joining and trimming never
// touch them: a frame is turned by the stretch its moment lies in, in the
// render and in the video preview alike. Plan row 3.25, and the review of
// #188, which found the rotation kept on each piece lost when a cut was
// put back.

// Rotation is a stretch of the video, from From to To, that a person
// rotated Deg degrees to the left: 90, 180 or 270. A stretch that runs to
// the video's start or end has From of minus infinity or To of plus
// infinity, so dragging the clip's edge out over it keeps it turned.
type Rotation struct {
	From float64
	To   float64
	Deg  int
}

// RotationAt is how far the frame that starts at a moment is rotated. A
// moment a hair before a stretch starts counts as in it, since the frames
// of a file stand a little off the millisecond its edges are kept to.
func RotationAt(spans []Rotation, at float64) int {
	for _, r := range spans {
		if at >= r.From-frameHair && at < r.To-frameHair {
			return r.Deg
		}
	}
	return 0
}

// readRotations checks a clip's rotations as the plan holds them: each a
// stretch with a start before its end, rotated a quarter, a half or three
// quarters, and in order with none overlapping. A missing from or to runs
// to that end of the video. A plan that breaks any of that is refused
// rather than read some other way.
func readRotations(value any) ([]Rotation, error) {
	if value == nil {
		return nil, nil
	}
	list, ok := value.([]any)
	if !ok {
		return nil, fmt.Errorf("rotations must be a list")
	}
	var spans []Rotation
	for _, item := range list {
		entry, ok := asMap(item)
		if !ok {
			return nil, fmt.Errorf("a rotation must be an object")
		}
		r := Rotation{From: math.Inf(-1), To: math.Inf(1)}
		if v, present := entry[keyFrom]; present && v != nil {
			t, err := ParseTime(v)
			if err != nil || t < 0 || t > MaxEpisodeSeconds {
				return nil, fmt.Errorf("a rotation starts at %s, which is no moment of a video", runes.Scrub(quoteValue(v), 40))
			}
			r.From = t
		}
		if v, present := entry[keyTo]; present && v != nil {
			t, err := ParseTime(v)
			if err != nil || t < 0 || t > MaxEpisodeSeconds {
				return nil, fmt.Errorf("a rotation ends at %s, which is no moment of a video", runes.Scrub(quoteValue(v), 40))
			}
			r.To = t
		}
		deg, present := entry[keyDeg]
		f, isNumber := num.AsFloat(deg)
		_, isText := deg.(string)
		if !present || isText || !isNumber || f != math.Trunc(f) || !validRotate(int(f)) || f == 0 {
			return nil, fmt.Errorf("a stretch is rotated by %s, which is not 90, 180 or 270", runes.Scrub(quoteValue(deg), 40))
		}
		r.Deg = int(f)
		if r.To <= r.From {
			return nil, fmt.Errorf("a rotation ends before it starts")
		}
		if len(spans) > 0 && r.From < spans[len(spans)-1].To {
			return nil, fmt.Errorf("two rotations overlap, or are out of order")
		}
		spans = append(spans, r)
	}
	return spans, nil
}

// asMap is a plan object as a map, whether it was read plainly or in the
// order it was written, see editPlan.
func asMap(item any) (map[string]any, bool) {
	switch v := item.(type) {
	case map[string]any:
		return v, true
	case *object:
		return v.values, true
	}
	return nil, false
}

// rotationsOf is the rotations a clip of a plan being edited has. The
// plan was read by LoadClips before it could be edited, which refuses
// rotations that do not hold, so none are found here.
func rotationsOf(c *object) []Rotation {
	spans, _ := readRotations(c.values[keyRotations])
	return spans
}

// setRotations writes a clip's rotations, or takes the key away when
// there are none.
func setRotations(c *object, spans []Rotation) {
	if len(spans) == 0 {
		c.remove(keyRotations)
		return
	}
	list := make([]any, len(spans))
	for i, r := range spans {
		o := newObject()
		if !math.IsInf(r.From, -1) {
			o.set(keyFrom, num.RoundTo(r.From, 3))
		}
		if !math.IsInf(r.To, 1) {
			o.set(keyTo, num.RoundTo(r.To, 3))
		}
		o.set(keyDeg, r.Deg)
		list[i] = o
	}
	c.set(keyRotations, list)
}

// turnStretch is the rotations with the stretch from from to to rotated
// deg degrees, whatever it was before, and stretches that meet rotated
// alike made one.
func turnStretch(spans []Rotation, from, to float64, deg int) []Rotation {
	var out []Rotation
	for _, r := range spans {
		if r.To <= from || r.From >= to {
			out = append(out, r)
			continue
		}
		if r.From < from {
			out = append(out, Rotation{r.From, from, r.Deg})
		}
		if r.To > to {
			out = append(out, Rotation{to, r.To, r.Deg})
		}
	}
	if deg != 0 {
		out = append(out, Rotation{from, to, deg})
	}
	sort.Slice(out, func(i, j int) bool { return out[i].From < out[j].From })
	var merged []Rotation
	for _, r := range out {
		if n := len(merged); n > 0 && merged[n-1].Deg == r.Deg && merged[n-1].To >= r.From {
			merged[n-1].To = math.Max(merged[n-1].To, r.To)
			continue
		}
		merged = append(merged, r)
	}
	return merged
}

// rotationOf is how far a piece of a clip is rotated: by the stretch its
// first frame lies in. A piece never runs over the edge of a stretch, see
// partedAtTurns.
func rotationOf(seg *object, spans []Rotation) int {
	return RotationAt(spans, number(seg.values[keyStart]))
}

// partedAtTurns is a clip's pieces parted where a rotation starts or ends
// inside one, so that every piece is turned one way throughout, which the
// render needs: its crop is placed on the picture as that piece stands. A
// piece only runs over such an edge when a cut that held it is put back,
// or an edge is dragged out past a stretch rotated on its own.
//
// The part that is turned differently from the piece it came from takes
// the crop of a piece turned that way, or none, which stands in the
// middle.
func partedAtTurns(pieces []*object, spans []Rotation) []*object {
	if len(spans) == 0 {
		return pieces
	}
	var edges []float64
	for _, r := range spans {
		for _, t := range []float64{r.From, r.To} {
			if !math.IsInf(t, 0) {
				edges = append(edges, t)
			}
		}
	}
	var out []*object
	for _, seg := range pieces {
		rest := seg
		for _, t := range edges {
			start, end := number(rest.values[keyStart]), number(rest.values[keyEnd])
			if t <= start+frameHair || t >= end-frameHair {
				continue
			}
			before := copyObject(rest)
			before.set(keyEnd, num.RoundTo(t, 3))
			after := copyObject(rest)
			after.set(keyStart, num.RoundTo(t, 3))
			if rotationOf(after, spans) != rotationOf(before, spans) {
				after.remove(keyCropXTurned)
				for _, other := range pieces {
					if v, ok := other.get(keyCropXTurned); ok && rotationOf(other, spans) == rotationOf(after, spans) {
						after.set(keyCropXTurned, v)
						break
					}
				}
			}
			out = append(out, before)
			rest = after
		}
		out = append(out, rest)
	}
	return out
}

// frameStartNear is the start of the frame nearest a moment, to the
// millisecond the plan keeps, or the moment itself where the frame rate is
// not known.
func (s SourceInfo) frameStartNear(at float64) float64 {
	start, _ := s.PieceOnFrames(at, at)
	return start
}

// RotationView is a stretch of a clip's rotations as the window reads it.
// From or To is null where the stretch runs to that end of the video.
type RotationView struct {
	From *float64 `json:"from"`
	To   *float64 `json:"to"`
	Deg  int      `json:"deg"`
}

func rotationView(r Rotation) RotationView {
	v := RotationView{Deg: r.Deg}
	if !math.IsInf(r.From, 0) {
		from := r.From
		v.From = &from
	}
	if !math.IsInf(r.To, 0) {
		to := r.To
		v.To = &to
	}
	return v
}
