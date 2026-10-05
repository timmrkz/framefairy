package engine

import (
	"fmt"
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
	// Edge is the edge a trim moves, "start" or "end", or "both", and the
	// edge of a cut a move moves, "from" or "to", or "" for both. The edge
	// that is not moved stays exactly where it is.
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
	if list, ok := top.values[keyClips].([]any); ok {
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
	out, err := checkedPieces(change, segmentObjects(c), foundPieces(c))
	if err != nil {
		return Shaped{}, err
	}
	shaped := Shaped{Playhead: playhead, Clip: clip}
	shaped.Clip.Segments = nil
	for _, seg := range out {
		start, end := number(seg.values[keyStart]), number(seg.values[keyEnd])
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
	pieceChange, float64, error) {
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
	found := [2]float64{first, last}
	if clip.Found != nil {
		found = *clip.Found
	}
	switch g.Kind {
	case "trim":
		start, end := first, last
		var words []Cue
		var goes []float64
		if g.ToWords {
			words = stops(math.Min(g.From, first), math.Max(g.To, last))
			if g.Edge == "end" {
				goes = captionsGo(plan, runOn(clip, g.From), t)
			}
		}
		switch g.Edge {
		case "start":
			start = g.onFound(g.landStart(words, g.From, keepPause), found[0])
			start = math.Min(start, end-MinClip)
			start = math.Max(start, end-MaxClipSpan)
		case "end":
			end = g.onFound(g.landEnd(words, goes, g.From, keepPause), found[1])
			end = math.Max(end, start+MinClip)
			end = math.Min(end, start+MaxClipSpan)
		case "both":
			start, end = g.landStart(words, g.From, keepPause), g.landEnd(words, nil, g.To, keepPause)
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
		return func(pieces []*object, found []foundPiece) ([]*object, error) {
			return trimPieces(pieces, found, start, end)
		}, playhead, nil

	case "cut", "restore":
		from, to := math.Min(g.From, g.To), math.Max(g.From, g.To)
		switch {
		case g.Kind == "restore":
			from, to = roundTo(math.Max(0, from), 3), roundTo(to, 3)
		case g.ToWords:
			from, to = snapCut(stops(from, to), nil, from, to, keepPause)
		default:
			var ok bool
			if from, to, ok = g.cutOnFrames(clip, from, to); !ok {
				return nil, -1, renderErr("there is no room for a cut there")
			}
		}
		if to-from < MinCut-1e-6 {
			return nil, -1, renderErr("a cut has to take out more than that")
		}
		return func(pieces []*object, _ []foundPiece) ([]*object, error) {
			out := applyCut(pieces, from, to)
			if len(out) == len(pieces) && pieceSpan(out) >= pieceSpan(pieces) {
				return nil, renderErr("that cut falls outside the clip")
			}
			return out, nil
		}, -1, nil

	case "move":
		// A cut is a gap between two pieces. Two pieces that meet have
		// none, which is where a search parts a clip at a camera switch,
		// and moving that edge would pull one shot over the other.
		i := g.Index
		if i < 0 || i+1 >= len(clip.Segments) || clip.Segments[i+1].Start <= clip.Segments[i].End {
			return nil, -1, renderErr("this clip has no cut number %d", i+1)
		}
		from, to := g.From, g.To
		if g.ToWords {
			// Where the captions go is read with this cut left out, the
			// way they go when the edge is dragged back over them.
			joined := clip
			joined.Segments = append(append([]Segment{}, clip.Segments[:i]...),
				Segment{Start: clip.Segments[i].Start, End: clip.Segments[i+1].End})
			joined.Segments = append(joined.Segments, clip.Segments[i+2:]...)
			from, to = snapCut(stops(from, to), captionsGo(plan, joined, t), from, to, keepPause)
		} else {
			from, to = g.onFrame(from), g.onFrame(to)
		}
		// A cut lives between the two pieces it parts, and neither may be
		// squeezed out of existence, so the edges are held inside them.
		low := clip.Segments[i].Start + MinCut
		high := clip.Segments[i+1].End - MinCut
		// Moving one edge never moves the other. The other edge is where
		// it was, not put on a word too, and the edge moved stops at it
		// rather than pushing it along. Shift on the right edge of a cut
		// put the left one on a word as well, and Tim saw it jump.
		switch g.Edge {
		case "from":
			to = clip.Segments[i+1].Start
			from = math.Min(math.Max(from, low), to-MinCut)
		case "to":
			from = clip.Segments[i].End
			to = math.Max(math.Min(to, high), from+MinCut)
		case "":
			from = math.Min(math.Max(from, low), high-MinCut)
			to = math.Max(math.Min(to, high), from+MinCut)
		default:
			return nil, -1, renderErr("a cut has a from and a to edge, not %s", Scrub(g.Edge, 20))
		}
		from, to = roundTo(from, 3), roundTo(to, 3)
		return func(pieces []*object, _ []foundPiece) ([]*object, error) {
			if i+1 >= len(pieces) || number(pieces[i+1].values[keyStart]) <= number(pieces[i].values[keyEnd]) {
				return nil, renderErr("this clip has no cut number %d", i+1)
			}
			out := append([]*object{}, pieces...)
			out[i] = copyObject(pieces[i])
			out[i].set(keyEnd, from)
			out[i+1] = copyObject(pieces[i+1])
			out[i+1].set(keyStart, to)
			return out, nil
		}, -1, nil

	case "join":
		// The part the cut left out comes back framed by the shots it
		// shows, so a cut made across a camera switch and put back leaves
		// the switch where it was, and each shot its own crop. Pieces of
		// one shot become one piece again.
		at := g.From
		return func(pieces []*object, found []foundPiece) ([]*object, error) {
			for i := 0; i+1 < len(pieces); i++ {
				end := number(pieces[i].values[keyEnd])
				next := number(pieces[i+1].values[keyStart])
				if at >= end && at <= next && next > end {
					back := meet(meet(pieces[i:i+1], framedBack(end, next, pieces, found)), pieces[i+1:i+2])
					out := append([]*object{}, pieces[:i]...)
					out = append(out, back...)
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

// onFound is where an edge lands that the hand put where the clip was
// found, or within half a frame of it: there exactly. A search finds a
// clip on the episode's own clock, not on its frames, so putting an edge
// back, which is a trim to where it was found, landed on the frame nearest
// and not where it had been: 24.80 where the clip had ended at 24.72. A
// sequence found it.
func (g Gesture) onFound(at, found float64) float64 {
	if !g.ToWords && math.Abs(g.From-found) <= g.Frame/2 {
		return found
	}
	return at
}

// landStart and landEnd are where a clip's first and last edge land: on the
// nearest word, a little before or after it, or on the frame.
func (g Gesture) landStart(words []Cue, at, keepPause float64) float64 {
	if g.ToWords {
		return SnapStart(words, at, keepPause)
	}
	return math.Max(0, g.onFrame(at))
}

func (g Gesture) landEnd(words []Cue, goes []float64, at, keepPause float64) float64 {
	if g.ToWords {
		return SnapEnd(words, goes, at, keepPause)
	}
	return g.onFrame(at)
}

// captionsGo is where the captions of a clip go off the screen, on the
// episode's clock: the ends of the blocks the clip timeline draws. An edge
// put on words stops there as well as at the words, because a caption
// stays up a little after its last word and its block shows that. An edge
// that only knew the words went past the end of the block Tim was looking
// at, into it, and the block shrank under the edge. Only an end inside a
// piece is one: a caption a cut cuts short goes where the cut is.
func captionsGo(plan Plan, clip Clip, t *Transcript) []float64 {
	s := ResolveStyle(captionStyle(plan, clip, nil))
	var ends []float64
	for _, c := range ClipCaptions(clip, t, s) {
		at := EpisodeTime(clip, c.End)
		for _, p := range clip.Segments {
			if at > p.Start && at < p.End {
				ends = append(ends, roundTo(at, 3))
				break
			}
		}
	}
	return ends
}

// runOn is a clip as it would be with its end dragged out past at, a
// minute on, so the captions around at go where they would with nothing
// after them cut.
func runOn(clip Clip, at float64) Clip {
	var out []Segment
	for _, s := range clip.Segments {
		if len(out) == 0 || s.Start < at {
			out = append(out, s)
		}
	}
	out[len(out)-1].End = math.Max(out[len(out)-1].End, at) + 60
	clip.Segments = out
	return clip
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
// Pieces left outside go, and the first and last piece keep their framing.
// An edge moved out over what the clip had left out puts it back framed by
// the shots it shows, see framedBack, so an edge put back where the clip was
// found brings back a shot it had trimmed away, with its own crop, from
// the camera switch. Edges that hold none of the old pieces are framed the
// same way, by the shots between them.
func trimPieces(pieces []*object, found []foundPiece, start, end float64) ([]*object, error) {
	var kept []*object
	for _, seg := range pieces {
		if number(seg.values[keyEnd]) <= start || number(seg.values[keyStart]) >= end {
			continue
		}
		kept = append(kept, seg)
	}
	if len(kept) == 0 {
		kept = framedBack(start, end, pieces, found)
	} else {
		first := number(kept[0].values[keyStart])
		last := number(kept[len(kept)-1].values[keyEnd])
		kept = meet(meet(framedBack(start, first, pieces, found), kept), framedBack(last, end, pieces, found))
	}
	if len(kept) == 0 {
		var nearest *object
		for _, seg := range pieces {
			if nearest == nil || math.Abs(number(seg.values[keyStart])-start) <
				math.Abs(number(nearest.values[keyStart])-start) {
				nearest = seg
			}
		}
		if nearest == nil {
			return nil, renderErr("the clip has no pieces")
		}
		kept = []*object{nearest}
	}
	out := make([]*object, len(kept))
	for i, seg := range kept {
		out[i] = copyObject(seg)
	}
	out[0].set(keyStart, start)
	out[len(out)-1].set(keyEnd, end)
	return out, nil
}

// framedBack is the pieces that bring back the part of the episode from from
// to to, which the clip leaves out, each with the framing of the shot it
// shows. The shots are the pieces the clip was found with, which a search
// parts at every camera switch: a shot runs from the start of its piece to
// the start of the next, so a part the search had already left out between
// two shots goes with the one before it, and a part before the first or
// after the last goes with that one. A shot a piece of the clip still
// shows takes that piece's framing, so a crop placed by hand comes back
// with it.
func framedBack(from, to float64, pieces []*object, found []foundPiece) []*object {
	var out []*object
	for k, f := range found {
		low, high := math.Inf(-1), math.Inf(1)
		if k > 0 {
			low = f.start
		}
		if k+1 < len(found) {
			high = found[k+1].start
		}
		a, b := roundTo(math.Max(from, low), 3), roundTo(math.Min(to, high), 3)
		if b <= a {
			continue
		}
		like := f.seg
		for _, seg := range pieces {
			if fmt.Sprint(autoCrop(seg)) == fmt.Sprint(autoCrop(f.seg)) {
				like = seg
				break
			}
		}
		piece := copyObject(like)
		piece.set(keyStart, a)
		piece.set(keyEnd, b)
		out = meet(out, []*object{piece})
	}
	return out
}

// meet is two runs of pieces one after the other, with the last of the
// first and the first of the second made one piece when they meet and are
// framed the same: one shot, which a cut had parted. Two pieces that meet
// at a camera switch stay two.
func meet(a, b []*object) []*object {
	out := append([]*object{}, a...)
	if len(a) == 0 || len(b) == 0 {
		return append(out, b...)
	}
	last, next := a[len(a)-1], b[0]
	if number(next.values[keyStart]) > number(last.values[keyEnd]) || !sameFraming(last, next) {
		return append(out, b...)
	}
	one := copyObject(last)
	one.set(keyEnd, next.values[keyEnd])
	out[len(out)-1] = one
	return append(out, b[1:]...)
}

// sameFraming says whether two pieces are framed alike: the same crop, and
// the same automatic crop behind it.
func sameFraming(a, b *object) bool {
	for _, key := range []string{keyCropX, keyCropXAuto} {
		x, okA := a.get(key)
		y, okB := b.get(key)
		if okA != okB || fmt.Sprint(x) != fmt.Sprint(y) {
			return false
		}
	}
	return true
}
