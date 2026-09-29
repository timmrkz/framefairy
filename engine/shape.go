package engine

import (
	"math"
	"os"
)

// What a hand does to a clip on the clip timeline, and what it makes of the
// clip, see docs/WORDS.md. A gesture is worked out here and nowhere else:
// where a dragged edge lands, on a frame or on a word, how far it may go,
// where the playhead stands while it is dragged, and which pieces the clip
// is left with. ShapeClip answers while the hand moves and writes nothing.
// Reshape saves it. Both go through the same functions, so what is drawn is
// what is saved, and the interface keeps no rules of its own.

// Gesture is what the hand does.
type Gesture struct {
	// Kind is "trim", an edge of the clip; "cut", a part taken out; "move",
	// the edges of a cut moved; "join", a cut put back; or "restore", a part
	// taken out again exactly as it was before it was put back.
	Kind string `json:"kind"`
	// Edge is the edge a trim moves, "start" or "end", or "both".
	Edge string `json:"edge"`
	// Index is the cut a move moves, counted from the first.
	Index int `json:"index"`
	// From and To are where the hand put things on the episode's clock: the
	// edge of a trim in From (both edges for "both"), the part of a cut, the
	// new edges of a moved cut, the moment of a cut put back in From.
	From float64 `json:"from"`
	To   float64 `json:"to"`
	// ToWords puts edges on words, the way the captions show them.
	// Otherwise they land on the frame.
	ToWords bool `json:"toWords"`
	// Frame is one frame of the episode in seconds. Nought leaves edges put
	// on frames where they were put, to the millisecond.
	Frame float64 `json:"frame"`
}

// PieceView is one kept part of a clip as a gesture leaves it.
type PieceView struct {
	Start float64 `json:"start"`
	End   float64 `json:"end"`
}

// Shaped is what a gesture makes of a clip.
type Shaped struct {
	Pieces []PieceView `json:"pieces"`
	// Playhead is where the playhead goes while an edge is dragged, so the
	// video preview shows the frame the clip now starts or ends on. Below
	// nought it stays where it is.
	Playhead float64 `json:"playhead"`
	// Clip is the clip as the gesture leaves it, for its captions.
	Clip Clip `json:"-"`
}

// ShapeClip works out what a gesture makes of a clip, and writes nothing.
func ShapeClip(planPath, clipID string, g Gesture, t *Transcript, keepPause float64) (Shaped, error) {
	plan, clip, err := planClip(planPath, clipID)
	if err != nil {
		return Shaped{}, err
	}
	change, playhead, err := g.change(plan, clip, t, keepPause)
	if err != nil {
		return Shaped{}, err
	}
	data, err := os.ReadFile(planPath)
	if err != nil {
		return Shaped{}, err
	}
	value, err := decodeOrdered(data)
	if err != nil {
		return Shaped{}, err
	}
	top, _ := value.(*object)
	if top == nil {
		return Shaped{}, renderErr("clip plan must be a JSON object")
	}
	var clips []*object
	if list, ok := top.values["clips"].([]any); ok {
		for _, item := range list {
			if c, ok := item.(*object); ok {
				clips = append(clips, c)
			}
		}
	}
	c, err := findClip(clips, clipID)
	if err != nil {
		return Shaped{}, err
	}
	out, err := checkedPieces(change, segmentObjects(c))
	if err != nil {
		return Shaped{}, err
	}
	shaped := Shaped{Playhead: playhead, Clip: clip}
	shaped.Clip.Segments = nil
	for _, seg := range out {
		start, end := number(seg.values["start"]), number(seg.values["end"])
		shaped.Pieces = append(shaped.Pieces, PieceView{start, end})
		shaped.Clip.Segments = append(shaped.Clip.Segments, Segment{Start: start, End: end})
	}
	return shaped, nil
}

// Reshape makes a gesture's change to a clip and saves it, the same change
// ShapeClip shows while the hand moves.
func Reshape(planPath, clipID string, g Gesture, t *Transcript, keepPause float64) error {
	plan, clip, err := planClip(planPath, clipID)
	if err != nil {
		return err
	}
	change, _, err := g.change(plan, clip, t, keepPause)
	if err != nil {
		return err
	}
	return editPieces(planPath, clipID, change)
}

// change is what a gesture does to a clip's pieces, with its edges where
// they land, and where the playhead goes while it is made.
func (g Gesture) change(plan Plan, clip Clip, t *Transcript, keepPause float64) (
	func([]*object) ([]*object, error), float64, error) {
	if !isFinite(g.From) || !isFinite(g.To) || !isFinite(g.Frame) || g.Frame < 0 {
		return nil, -1, renderErr("a gesture has to say where, in numbers")
	}
	if len(clip.Segments) == 0 {
		return nil, -1, renderErr("clip %s has no pieces", clip.ID)
	}
	// The words an edge lands on are the words the captions show, halves
	// of a hyphenated word and all, see ShowWords.
	stops := func(from, to float64) []Cue {
		s := ResolveStyle(captionStyle(plan, clip, nil))
		return ShowWords(t.WordsBetween(from-60, to+60), s, t.Language)
	}
	first := clip.Segments[0].Start
	last := clip.Segments[len(clip.Segments)-1].End
	switch g.Kind {
	case "trim":
		start, end := first, last
		var words []Cue
		if g.ToWords {
			words = stops(math.Min(g.From, first), math.Max(g.To, last))
		}
		switch g.Edge {
		case "start":
			start = g.landStart(words, g.From, keepPause)
			start = math.Min(start, end-MinClip)
			start = math.Max(start, end-MaxClipSpan)
		case "end":
			end = g.landEnd(words, g.From, keepPause)
			end = math.Max(end, start+MinClip)
			end = math.Min(end, start+MaxClipSpan)
		case "both":
			start, end = g.landStart(words, g.From, keepPause), g.landEnd(words, g.To, keepPause)
			if end-start < MinClip {
				return nil, -1, renderErr("a clip needs at least one second")
			}
			if end-start > MaxClipSpan {
				return nil, -1, renderErr("a clip can span at most %s minutes of the episode",
					fixed(MaxClipSpan/60, 0))
			}
		default:
			return nil, -1, renderErr("a clip has a start and an end, not %s", Scrub(g.Edge, 20))
		}
		start, end = roundTo(math.Max(0, start), 3), roundTo(end, 3)
		playhead := -1.0
		if g.Edge != "both" {
			playhead = g.edgePlayhead(words, start, end)
		}
		return func(pieces []*object) ([]*object, error) {
			return trimPieces(pieces, start, end)
		}, playhead, nil

	case "cut", "restore":
		from, to := math.Min(g.From, g.To), math.Max(g.From, g.To)
		switch {
		case g.Kind == "restore":
			from, to = roundTo(math.Max(0, from), 3), roundTo(to, 3)
		case g.ToWords:
			from, to = snapCut(stops(from, to), from, to, keepPause)
		default:
			var ok bool
			if from, to, ok = g.cutOnFrames(clip, from, to); !ok {
				return nil, -1, renderErr("there is no room for a cut there")
			}
		}
		if to-from < MinCut-1e-6 {
			return nil, -1, renderErr("a cut has to take out more than that")
		}
		return func(pieces []*object) ([]*object, error) {
			out := applyCut(pieces, from, to)
			if len(out) == len(pieces) && pieceSpan(out) >= pieceSpan(pieces) {
				return nil, renderErr("that cut falls outside the clip")
			}
			return out, nil
		}, -1, nil

	case "move":
		i := g.Index
		if i < 0 || i+1 >= len(clip.Segments) {
			return nil, -1, renderErr("this clip has no cut number %d", i+1)
		}
		from, to := g.From, g.To
		if g.ToWords {
			from, to = snapCut(stops(from, to), from, to, keepPause)
		} else {
			from, to = g.onFrame(from), g.onFrame(to)
		}
		// A cut lives between the two pieces it parts, and neither may be
		// squeezed out of existence, so the edges are held inside them.
		low := clip.Segments[i].Start + MinCut
		high := clip.Segments[i+1].End - MinCut
		from = math.Min(math.Max(from, low), high-MinCut)
		to = math.Max(math.Min(to, high), from+MinCut)
		from, to = roundTo(from, 3), roundTo(to, 3)
		return func(pieces []*object) ([]*object, error) {
			if i+1 >= len(pieces) {
				return nil, renderErr("this clip has no cut number %d", i+1)
			}
			out := append([]*object{}, pieces...)
			out[i] = copyObject(pieces[i])
			out[i].set("end", from)
			out[i+1] = copyObject(pieces[i+1])
			out[i+1].set("start", to)
			return out, nil
		}, -1, nil

	case "join":
		at := g.From
		return func(pieces []*object) ([]*object, error) {
			for i := 0; i+1 < len(pieces); i++ {
				end := number(pieces[i].values["end"])
				next := number(pieces[i+1].values["start"])
				if at >= end && at <= next {
					joined := copyObject(pieces[i])
					joined.set("end", pieces[i+1].values["end"])
					out := append([]*object{}, pieces[:i]...)
					out = append(out, joined)
					return append(out, pieces[i+2:]...), nil
				}
			}
			return nil, renderErr("there is no cut at %s", fixed(at, 2))
		}, -1, nil
	}
	return nil, -1, renderErr("there is no gesture called %s", Scrub(g.Kind, 20))
}

// onFrame puts a moment on the frame it falls on, the only place a video is
// cut. Without a frame it is left where it is, to the millisecond.
func (g Gesture) onFrame(at float64) float64 {
	if g.Frame > 0 {
		at = math.Round(at/g.Frame) * g.Frame
	}
	return roundTo(at, 3)
}

// landStart and landEnd are where a clip's first and last edge land: on the
// nearest word, a little before or after it, or on the frame.
func (g Gesture) landStart(words []Cue, at, keepPause float64) float64 {
	if g.ToWords {
		return SnapStart(words, at, keepPause)
	}
	return math.Max(0, g.onFrame(at))
}

func (g Gesture) landEnd(words []Cue, at, keepPause float64) float64 {
	if g.ToWords {
		return SnapEnd(words, at, keepPause)
	}
	return g.onFrame(at)
}

// edgePlayhead is where the playhead stands while an edge of a clip is
// dragged. On frames it is the frame at the edge: the clip's first frame,
// or its last, a frame before the end, because the end is where the clip is
// already over. On words it is inside the word the edge landed on, a frame
// into the first word or a frame before the end of the last, so that word
// is the one lit. The edge itself stands a pause away from the word, and a
// playhead put there lit the word only when the pause happened to be none.
func (g Gesture) edgePlayhead(words []Cue, start, end float64) float64 {
	frame := g.Frame
	if frame <= 0 {
		frame = 1.0 / 30
	}
	if g.Edge == "start" {
		if g.ToWords {
			for _, w := range words {
				if w.Start >= start-0.0005 {
					if w.End <= end {
						return math.Min(w.Start+frame, (w.Start+w.End)/2)
					}
					break
				}
			}
		}
		return start
	}
	if g.ToWords {
		var word *Cue
		for i := range words {
			if words[i].End <= end+0.0005 {
				word = &words[i]
			}
		}
		if word != nil && word.Start >= start {
			return math.Max(word.End-frame, (word.Start+word.End)/2)
		}
	}
	return math.Max(end-frame, start)
}

// cutOnFrames is where a cut put on frames goes. It is centred where it
// was put, both edges land on frames and it is a whole number of frames
// wide, rounded up, never narrower than the least a cut may be. Rounding
// the two ends on their own brings them closer than they were asked to be:
// at 25 frames a second a frame is 40 milliseconds against a least of 50.
// A cut in one piece stays inside it, with room left on both sides,
// because a piece squeezed to nothing is a clip that cannot be rendered,
// and it says so when there is no room for it. A cut drawn across into
// another piece takes from both.
func (g Gesture) cutOnFrames(clip Clip, from, to float64) (float64, float64, bool) {
	width := math.Max(to-from, MinCut)
	if g.Frame > 0 {
		width = math.Ceil(width/g.Frame-1e-9) * g.Frame
	}
	mid := (from + to) / 2
	a := g.onFrame(mid - width/2)
	b := a + width
	// Whether a moment lies in a piece other than the one numbered.
	inOther := func(at float64, skip int) bool {
		for j, p := range clip.Segments {
			if j != skip && at >= p.Start && at <= p.End {
				return true
			}
		}
		return false
	}
	for i, p := range clip.Segments {
		if mid < p.Start || mid >= p.End {
			continue
		}
		// A cut drawn across into another piece takes from both, and is
		// not held inside one.
		if inOther(from, i) || inOther(to, i) {
			break
		}
		low, high := p.Start+MinCut, p.End-MinCut
		if high-low < MinCut {
			return 0, 0, false
		}
		width = math.Min(width, high-low)
		if a < low {
			a = low
		}
		b = a + width
		if b > high {
			b = high
			a = math.Max(b-width, low)
		}
		break
	}
	if b-a < MinCut-1e-9 {
		return 0, 0, false
	}
	return roundTo(a, 3), roundTo(b, 3), true
}

// trimPieces gives a clip's pieces with its first and last edge moved.
// Pieces left outside go, the first and last piece keep their framing, and
// edges that hold none of the old pieces make one piece with the framing of
// the nearest.
func trimPieces(pieces []*object, start, end float64) ([]*object, error) {
	var kept []*object
	for _, seg := range pieces {
		if number(seg.values["end"]) <= start || number(seg.values["start"]) >= end {
			continue
		}
		kept = append(kept, copyObject(seg))
	}
	if len(kept) == 0 {
		var nearest *object
		for _, seg := range pieces {
			if nearest == nil || math.Abs(number(seg.values["start"])-start) <
				math.Abs(number(nearest.values["start"])-start) {
				nearest = seg
			}
		}
		if nearest == nil {
			return nil, renderErr("the clip has no pieces")
		}
		kept = []*object{copyObject(nearest)}
	}
	kept[0].set("start", start)
	kept[len(kept)-1].set("end", end)
	return kept, nil
}
