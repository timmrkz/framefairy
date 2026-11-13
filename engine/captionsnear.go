package engine

import (
	"maps"
	"math"
	"reflect"
	"slices"
	"sort"

	"framefairy/engine/timed"
)

// A step of a drag lays out again only the captions near what the hand
// changed, and takes the rest from the drawing of the clip as it was, see
// laidNear. Laying out every caption of the clip on each step took longer
// the longer the clip was, and Tim found dragging an edge of a long clip
// slow.

// layoutReach is how far past what a gesture changed, in seconds of the
// episode, the captions are laid out again. A caption's place depends
// only on the captions beside it, so a few of them either side of the
// change are enough to meet the captions that stay as they were.
const layoutReach = 10.0

// laidNear is the captions of clip, on frames and in the style s, as
// ClipCaptions lays them out, made from was, the drawing of the same clip
// before a gesture changed its pieces: the captions from a pause that
// parts two captions before the first piece changed to one after the last
// are laid out again, and the rest are was's, those after it moved on the
// clip's clock by as much as the change made the clip longer or shorter.
// cut says how they were put together from was's. ok is false
// when was is no drawing of this clip in this style from the same words,
// or the captions laid out again do not meet was's where they should, and
// the captions are then laid out whole.
func laidNear(was *CaptionDrawing, clip Clip, t *Transcript, s Style) (laid []LaidCaption, cut Splice, ok bool) {
	if was == nil || was.heard == nil || was.heard != t.source() || !reflect.DeepEqual(was.laidIn, s) {
		return nil, cut, false
	}
	same := clip
	same.Segments, same.CropMarks = was.Clip.Segments, was.Clip.CropMarks
	if !reflect.DeepEqual(same, was.Clip) {
		return nil, cut, false
	}
	before, after := was.Clip.Segments, clip.Segments
	if len(before) == 0 || len(after) == 0 {
		return nil, cut, false
	}
	// The pieces at the head and at the tail that stayed as they were.
	head := 0
	for head < len(before) && head < len(after) && samePiece(before[head], after[head]) {
		head++
	}
	if head == len(before) && head == len(after) {
		return slices.Clone(was.Laid), Splice{Head: len(was.Laid), Tail: len(was.Laid)}, true
	}
	tail := 0
	for tail < len(before)-head && tail < len(after)-head &&
		samePiece(before[len(before)-1-tail], after[len(after)-1-tail]) {
		tail++
	}
	// Where the pieces part: a piece whose start stayed changed only from
	// where the shorter of the two ends, and one whose end stayed only up
	// to where the later of the two starts.
	changedFrom := math.Min(pieceEdge(before, head, true), pieceEdge(after, head, true))
	if head < len(before) && head < len(after) && before[head].Start == after[head].Start {
		changedFrom = math.Min(before[head].End, after[head].End)
	}
	b, a := len(before)-1-tail, len(after)-1-tail
	changedTo := math.Max(pieceEdge(before, b, false), pieceEdge(after, a, false))
	if b >= 0 && a >= 0 && before[b].End == after[a].End {
		changedTo = math.Max(before[b].Start, after[a].Start)
	}
	// Laid out again further out where the captions there do not meet
	// was's yet: a long silence can leave one caption before the change,
	// and a short one after it rides with that one.
	for reach := layoutReach; reach <= 8*layoutReach; reach *= 2 {
		if laid, cut, ok := laidBetween(was, clip, t, s, changedFrom-reach, changedTo+reach); ok {
			return laid, cut, true
		}
	}
	return nil, cut, false
}

// laidBetween is laidNear with the captions laid out again from the last
// pause before from to the first after to.
func laidBetween(was *CaptionDrawing, clip Clip, t *Transcript, s Style, from, to float64) (laid []LaidCaption, cut Splice, ok bool) {
	words := t.Words
	from, to = pauseBefore(words, from), toPause(clip, words, to)
	near := clip
	near.Segments = nil
	for _, p := range clip.Segments {
		p.Start, p.End = math.Max(p.Start, from), math.Min(p.End, to)
		if p.End > p.Start {
			near.Segments = append(near.Segments, p)
		}
	}
	if len(near.Segments) == 0 {
		return nil, cut, false
	}
	// The captions laid out again, on the clip's clock.
	window := ClipCaptions(near, t, s)
	offset := ClipTime(clip, near.Segments[0].Start)
	for i := range window {
		window[i] = laidLater(window[i], offset)
	}
	startsAtStart := near.Segments[0].Start == clip.Segments[0].Start
	endsAtEnd := near.Segments[len(near.Segments)-1].End == clip.Segments[len(clip.Segments)-1].End

	// Where the captions laid out again meet was's before the change: the
	// first that is one of was's, not the first of them, which may be laid
	// out otherwise with nothing before it.
	first, wasFirst := 0, 0
	if !startsAtStart {
		first, wasFirst = -1, -1
		for i := 1; i < len(window) && first < 0; i++ {
			k := sort.Search(len(was.Laid), func(k int) bool { return was.Laid[k].Opens >= window[i].Opens })
			if k < len(was.Laid) && sameLaid(window[i], was.Laid[k], 0) {
				first, wasFirst = i, k
			}
		}
		if first < 0 {
			return nil, cut, false
		}
	}
	// And after it: the last that is one of was's, moved on the clip's
	// clock, not the last of them, which may be laid out otherwise with
	// nothing after it.
	last, wasLast := len(window), len(was.Laid)
	if !endsAtEnd {
		last, wasLast = -1, -1
		shift := ClipTime(clip, to) - ClipTime(was.Clip, to)
		for i := len(window) - 2; i > first && last < 0; i-- {
			k := sort.Search(len(was.Laid), func(k int) bool { return was.Laid[k].Opens >= window[i].Opens })
			if k < len(was.Laid) && sameLaid(window[i], was.Laid[k], shift) {
				last, wasLast = i, k
			}
		}
		if last < 0 {
			return nil, cut, false
		}
		laid = make([]LaidCaption, 0, wasFirst+(last-first)+len(was.Laid)-wasLast)
		laid = append(append(laid, was.Laid[:wasFirst]...), window[first:last]...)
		for _, c := range was.Laid[wasLast:] {
			laid = append(laid, laidLater(c, shift))
		}
		return laid, Splice{Head: wasFirst, Tail: wasLast, Shift: shift, fresh: last - first}, true
	}
	laid = append(append(make([]LaidCaption, 0, wasFirst+len(window)-first), was.Laid[:wasFirst]...), window[first:]...)
	return laid, Splice{Head: wasFirst, Tail: len(was.Laid), fresh: len(window) - first}, true
}

// samePiece says whether a and b take the same stretch of the episode,
// which is all of a piece the captions are laid out from.
func samePiece(a, b Segment) bool { return a.Start == b.Start && a.End == b.End }

// Splice is how the captions of a clip a gesture changed are made from
// the captions it had before, as the interface has them: the first Head
// of them, then the captions laid out again, then those from Tail on,
// later on the clip's clock by Shift, in seconds. See laidNear.
type Splice struct {
	Head  int     `json:"head"`
	Tail  int     `json:"tail"`
	Shift float64 `json:"shift"`
	// fresh is how many captions were laid out again, after the first
	// Head.
	fresh int
}

// pieceEdge is where piece i of pieces begins, or ends, in the episode, or the
// edge of the pieces beside it when there is no piece i.
func pieceEdge(pieces []Segment, i int, begins bool) float64 {
	switch {
	case i < 0:
		return pieces[0].Start
	case i >= len(pieces):
		return pieces[len(pieces)-1].End
	case begins:
		return pieces[i].Start
	}
	return pieces[i].End
}

// toPause is the middle of the first pause after at long enough to part
// two captions, see fromPause, or the end of clip when there is none. The
// pause is wholly after at: the word before it ends at at or later. words
// are the clip's words in the order they are said.
func toPause(clip Clip, words []timed.Cue, at float64) float64 {
	k := sort.Search(len(words), func(i int) bool { return words[i].End >= at }) + 1
	for ; k >= 1 && k < len(words); k++ {
		if words[k].Start-words[k-1].End >= captionPause {
			return (words[k-1].End + words[k].Start) / 2
		}
	}
	return clip.Segments[len(clip.Segments)-1].End
}

// laidLater is c later on the clip's clock by by.
func laidLater(c LaidCaption, by float64) LaidCaption {
	if by == 0 {
		return c
	}
	c.Start, c.End = c.Start+by, c.End+by
	c.Words = movedCues(c.Words, by)
	lines := make([][]timed.Cue, len(c.Lines))
	for i, line := range c.Lines {
		lines[i] = movedCues(line, by)
	}
	c.Lines = lines
	return c
}

func movedCues(cues []timed.Cue, by float64) []timed.Cue {
	out := make([]timed.Cue, len(cues))
	for i, w := range cues {
		w.Start, w.End = w.Start+by, w.End+by
		out[i] = w
	}
	return out
}

// sameLaid says whether a is b laid out the same, b later on the clip's
// clock by shift.
func sameLaid(a, b LaidCaption, shift float64) bool {
	b = laidLater(b, shift)
	const near = 1e-6
	sameCues := func(x, y []timed.Cue) bool {
		if len(x) != len(y) {
			return false
		}
		for i := range x {
			if x[i].Text != y[i].Text || math.Abs(x[i].Start-y[i].Start) > near || math.Abs(x[i].End-y[i].End) > near {
				return false
			}
		}
		return true
	}
	if a.Text != b.Text || a.Opens != b.Opens || a.Closes != b.Closes || a.Lift != b.Lift ||
		a.trimStart != b.trimStart || a.trimEnd != b.trimEnd || a.startMoved != b.startMoved || a.endMoved != b.endMoved ||
		math.Abs(a.Start-b.Start) > near || math.Abs(a.End-b.End) > near ||
		!sameCues(a.Words, b.Words) || len(a.Lines) != len(b.Lines) || !maps.Equal(a.Hidden, b.Hidden) {
		return false
	}
	for i := range a.Lines {
		if !sameCues(a.Lines[i], b.Lines[i]) {
			return false
		}
	}
	return true
}
