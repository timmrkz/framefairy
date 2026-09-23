package engine

import (
	"bytes"
	"encoding/json"
	"errors"
	"fmt"
	"math"
	"os"
	"path/filepath"
	"regexp"
	"strings"
	"sync"
	"time"
)

// object is a JSON object that remembers its key order, so an edit made by
// the app changes only what it means to change and the file stays readable
// next to a hand edit.
type object struct {
	keys   []string
	values map[string]any
}

// newObject makes an empty object that remembers its key order.
func newObject() *object { return &object{values: map[string]any{}} }

func (o *object) get(key string) (any, bool) {
	v, ok := o.values[key]
	return v, ok
}

func (o *object) set(key string, value any) {
	if _, ok := o.values[key]; !ok {
		o.keys = append(o.keys, key)
	}
	o.values[key] = value
}

func (o *object) remove(key string) {
	if _, ok := o.values[key]; !ok {
		return
	}
	delete(o.values, key)
	for i, k := range o.keys {
		if k == key {
			o.keys = append(o.keys[:i], o.keys[i+1:]...)
			break
		}
	}
}

func (o *object) MarshalJSON() ([]byte, error) {
	var buf bytes.Buffer
	buf.WriteByte('{')
	for i, k := range o.keys {
		if i > 0 {
			buf.WriteByte(',')
		}
		key, err := marshalNoEscape(k)
		if err != nil {
			return nil, err
		}
		buf.Write(key)
		buf.WriteByte(':')
		value, err := marshalNoEscape(o.values[k])
		if err != nil {
			return nil, err
		}
		buf.Write(value)
	}
	buf.WriteByte('}')
	return buf.Bytes(), nil
}

func marshalNoEscape(v any) ([]byte, error) {
	var buf bytes.Buffer
	enc := json.NewEncoder(&buf)
	enc.SetEscapeHTML(false)
	if err := enc.Encode(v); err != nil {
		return nil, err
	}
	return bytes.TrimRight(buf.Bytes(), "\n"), nil
}

// decodeOrdered reads JSON, keeping object key order and numbers as written.
func decodeOrdered(data []byte) (any, error) {
	dec := json.NewDecoder(bytes.NewReader(data))
	dec.UseNumber()
	value, err := decodeValue(dec)
	if err != nil {
		return nil, err
	}
	if dec.More() {
		return nil, fmt.Errorf("unexpected data after the JSON value")
	}
	return value, nil
}

func decodeValue(dec *json.Decoder) (any, error) {
	tok, err := dec.Token()
	if err != nil {
		return nil, err
	}
	switch t := tok.(type) {
	case json.Delim:
		switch t {
		case '{':
			o := &object{values: map[string]any{}}
			for dec.More() {
				keyTok, err := dec.Token()
				if err != nil {
					return nil, err
				}
				key, _ := keyTok.(string)
				value, err := decodeValue(dec)
				if err != nil {
					return nil, err
				}
				o.set(key, value)
			}
			_, err := dec.Token()
			return o, err
		case '[':
			list := []any{}
			for dec.More() {
				value, err := decodeValue(dec)
				if err != nil {
					return nil, err
				}
				list = append(list, value)
			}
			_, err := dec.Token()
			return list, err
		}
		return nil, fmt.Errorf("unexpected %v", t)
	default:
		return t, nil
	}
}

// editPlan loads a plan, lets change edit it, and writes it back in one
// step. The file is replaced atomically, so a crash never leaves half a
// plan. The result is checked with the same loader a render uses before it
// is written.
// Edits of one plan happen one at a time. Every edit reads the file, changes
// it and writes it back, and the app has no save button, so an edit that
// lands while another is being written must not be the one that disappears.
var planLocks sync.Map

func lockPlan(path string) func() {
	held, _ := planLocks.LoadOrStore(resolvePath(path), &sync.Mutex{})
	mu := held.(*sync.Mutex)
	mu.Lock()
	return mu.Unlock
}

func editPlan(path string, change func(top *object, clips []*object) error) error {
	defer lockPlan(path)()
	return editPlanLocked(path, change)
}

// editPlanLocked is editPlan for a caller that already holds the plan's
// lock.
func editPlanLocked(path string, change func(top *object, clips []*object) error) error {
	data, err := os.ReadFile(path)
	if err != nil {
		return err
	}
	value, err := decodeOrdered(data)
	if err != nil {
		return err
	}
	top, ok := value.(*object)
	if !ok {
		return renderErr("clip plan must be a JSON object")
	}
	var clips []*object
	if list, ok := top.values["clips"].([]any); ok {
		for _, item := range list {
			if c, ok := item.(*object); ok {
				clips = append(clips, c)
			}
		}
	}
	if err := change(top, clips); err != nil {
		return err
	}
	revision := 0
	if raw, ok := top.get("revision"); ok {
		revision, _ = toInt(raw)
	}
	top.set("revision", revision+1)

	body, err := MarshalPlan(top)
	if err != nil {
		return err
	}
	// The lock is already held, so this is the unlocked one.
	return replacePlan(path, body)
}

// writePlanFile puts a whole new plan where a plan goes. A search that has
// just finished uses it, and so does a search over a window that was
// searched before, which lands on a file that is already there.
//
// It takes the same lock every edit takes, because the file has two
// writers: the search that makes a plan and the app that edits one.
// Without the lock a search can land in the middle of an edit reading the
// file, changing it and writing it back.
func writePlanFile(path string, body []byte) error {
	defer lockPlan(path)()
	if err := os.MkdirAll(filepath.Dir(path), 0o755); err != nil {
		return err
	}
	return replacePlan(path, body)
}

// replacePlan puts new contents in a plan file in one step. The caller
// holds the lock for that plan.
//
// Writing over the file itself would be wrong twice. The app reads the
// clips and the coverage about once a second while a search runs, and it
// would read a plan that is half there: the clip list empties itself, and
// the range picker reports a part it has already searched as free and
// offers it to the model a second time. And a write that goes wrong part
// of the way through would have taken the plan that was there with it.
//
// So it is written beside the file and moved onto it, which is one step as
// far as any reader is concerned, and only after it has been read back as
// a plan. The temporary name starts with a dot, because what lists the
// plans of an episode globs for clips*.json and a half written plan must
// never be one of them.
func replacePlan(path string, body []byte) error {
	tmp, err := os.CreateTemp(filepath.Dir(path), ".clips-*.json")
	if err != nil {
		return err
	}
	name := tmp.Name()
	_, err = tmp.Write(body)
	if closeErr := tmp.Close(); err == nil {
		err = closeErr
	}
	if err == nil {
		_, _, err = LoadClips(name)
	}
	if err != nil {
		os.Remove(name)
		return err
	}
	return os.Rename(name, path)
}

// errNotAdded is a clip that was not added because the plan no longer has
// room for it: the part it lies in was given back while it was on its
// way.
var errNotAdded = errors.New("the part this clip lies in was removed")

// appendClip adds a clip to a plan that a search is still writing. It goes
// through editPlan, so an edit the app makes at the same moment is kept,
// and so is the plan's shape.
//
// Removing part of a search while it runs leaves a hole in the plan, and a
// clip that arrives afterwards inside that hole is not added, because it
// would bring back what was just taken away. A plan that is gone
// altogether is reported as the missing file it is.
func appendClip(path string, clip PlanClip) error {
	body, err := MarshalPlan(clip)
	if err != nil {
		return err
	}
	value, err := decodeOrdered(body)
	if err != nil {
		return err
	}
	added, ok := value.(*object)
	if !ok {
		return renderErr("a clip must be a JSON object")
	}
	start, end := 0.0, 0.0
	if len(clip.Segments) > 0 {
		start, end = float64(clip.Segments[0].Start), float64(clip.Segments[len(clip.Segments)-1].End)
	}
	refused := false
	err = editPlan(path, func(top *object, clips []*object) error {
		if made, ok := top.values["planned_with"].(*object); ok {
			for _, hole := range orderedWindows(made.values["removed"]) {
				if end > hole.Start && start < hole.End {
					refused = true
					return errNotAdded
				}
			}
		}
		for i, c := range clips {
			fallback := twoDigits(i + 1)
			text := fallback
			if raw, ok := c.get("id"); ok {
				text = pyStr(raw)
			}
			if SanitiseName(text, fallback) == clip.ID {
				// Already there, which a search that was asked again
				// for the same answer would otherwise write twice.
				refused = true
				return errNotAdded
			}
		}
		list, _ := top.values["clips"].([]any)
		top.set("clips", append(list, added))
		return nil
	})
	if refused {
		return errNotAdded
	}
	return err
}

func findClip(clips []*object, id string) (*object, error) {
	for i, c := range clips {
		fallback := twoDigits(i + 1)
		text := fallback
		if raw, ok := c.get("id"); ok {
			text = pyStr(raw)
		}
		if SanitiseName(text, fallback) == id {
			return c, nil
		}
	}
	return nil, renderErr("no clip %s in the plan", id)
}

// SetRejected marks a clip as rejected or kept. A rejected clip stays in the
// plan, and a render of the whole plan leaves it out. Reasons are recorded
// with a rejection for training.
func SetRejected(planPath, clipID string, rejected bool, reasons ...string) error {
	// Checked before the plan is touched. A reason the format does not know
	// would throw the whole record away further down, without a word.
	if _, err := CheckReasons(reasons); rejected && err != nil {
		return err
	}
	err := editPlan(planPath, func(_ *object, clips []*object) error {
		c, err := findClip(clips, clipID)
		if err != nil {
			return err
		}
		if rejected {
			c.set("rejected", true)
		} else {
			c.remove("rejected")
		}
		return nil
	})
	if err != nil {
		return err
	}
	event := DecisionKept
	if rejected {
		event = DecisionRejected
	}
	// The decision is only a record. Failing to write it does not undo the
	// change the person made.
	if !rejected {
		reasons = nil
	}
	_ = RecordDecision(planPath, clipID, event, reasons)
	return nil
}

func number(v any) float64 {
	f, _ := toFloat(v)
	return f
}

// SnapStart is where a clip starting at the word nearest to at begins: a
// little before the word, never reaching back into the word before it.
func SnapStart(words []Cue, at, keepPause float64) float64 {
	if len(words) == 0 {
		return at
	}
	best := 0
	for i, w := range words {
		if math.Abs(w.Start-at) < math.Abs(words[best].Start-at) {
			best = i
		}
	}
	edge := words[best].Start - keepPause
	if best > 0 {
		edge = math.Max(edge, words[best-1].End)
	}
	return roundTo(math.Max(0, edge), 3)
}

// SnapEnd is where a clip ending at the word nearest to at ends: a little
// after the word, never reaching into the word after it.
func SnapEnd(words []Cue, at, keepPause float64) float64 {
	if len(words) == 0 {
		return at
	}
	best := 0
	for i, w := range words {
		if math.Abs(w.End-at) < math.Abs(words[best].End-at) {
			best = i
		}
	}
	edge := words[best].End + keepPause
	if best+1 < len(words) {
		edge = math.Min(edge, words[best+1].Start)
	}
	return roundTo(edge, 3)
}

// MaxClipSpan keeps a trimmed clip to a sensible length.
const MaxClipSpan = 600.0

// TrimClip moves a clip's first and last edge, snapped to the nearest words.
// Pieces that end up outside are dropped, the first and last piece keep
// their framing, and the clip's words are taken again from the transcript.
// The change is recorded as an edit for training.
func TrimClip(planPath, clipID string, start, end float64, t *Transcript, keepPause float64) error {
	start = SnapStart(t.Words, start, keepPause)
	end = SnapEnd(t.Words, end, keepPause)
	if end-start < 1 {
		return renderErr("a clip needs at least one second")
	}
	if end-start > MaxClipSpan {
		return renderErr("a clip can span at most %s minutes of the episode", fixed(MaxClipSpan/60, 0))
	}
	err := editPlan(planPath, func(_ *object, clips []*object) error {
		c, err := findClip(clips, clipID)
		if err != nil {
			return err
		}
		list, _ := c.values["segments"].([]any)
		var kept []any
		var first, last *object
		for _, item := range list {
			seg, ok := item.(*object)
			if !ok {
				continue
			}
			segStart, _ := seg.get("start")
			segEnd, _ := seg.get("end")
			if number(segEnd) <= start || number(segStart) >= end {
				continue
			}
			if first == nil {
				first = seg
			}
			last = seg
			kept = append(kept, seg)
		}
		if first == nil {
			// The new edges hold none of the old pieces, so the whole
			// part becomes one piece with the framing of the nearest one.
			var nearest *object
			for _, item := range list {
				if seg, ok := item.(*object); ok {
					segStart, _ := seg.get("start")
					if nearest == nil || math.Abs(number(segStart)-start) < math.Abs(number(nearest.values["start"])-start) {
						nearest = seg
					}
				}
			}
			if nearest == nil {
				return renderErr("clip %s has no pieces", clipID)
			}
			first, last = nearest, nearest
			kept = []any{nearest}
		}
		first.set("start", roundTo(start, 3))
		last.set("end", roundTo(end, 3))
		c.set("segments", kept)

		refreshWords(c, kept, t)
		return nil
	})
	if err != nil {
		return err
	}
	dropCaptionFile(planPath, clipID)
	_ = RecordDecision(planPath, clipID, DecisionEdited, nil)
	return nil
}

// refreshWords takes a clip's words again from the transcript: every word
// whose middle lies inside one of its pieces.
func refreshWords(c *object, pieces []any, t *Transcript) {
	spoken := []any{}
	for _, w := range t.Words {
		mid := (w.Start + w.End) / 2
		for _, item := range pieces {
			seg, ok := item.(*object)
			if !ok {
				continue
			}
			if mid >= number(seg.values["start"]) && mid < number(seg.values["end"]) {
				spoken = append(spoken, []any{roundTo(w.Start, 3), roundTo(w.End, 3), w.Text})
				break
			}
		}
	}
	c.set("words", spoken)
}

func copyObject(o *object) *object {
	out := &object{keys: append([]string(nil), o.keys...), values: map[string]any{}}
	for k, v := range o.values {
		out.values[k] = v
	}
	return out
}

// segmentAt returns the index of the piece that holds a moment, or the
// nearest piece.
func segmentAt(pieces []*object, at float64) int {
	best, bestDistance := -1, math.Inf(1)
	for i, seg := range pieces {
		start, end := number(seg.values["start"]), number(seg.values["end"])
		distance := 0.0
		if at < start {
			distance = start - at
		} else if at >= end {
			distance = at - end
		}
		if distance < bestDistance {
			best, bestDistance = i, distance
		}
	}
	return best
}

// autoCrop is the crop the framing analysis chose for a piece.
func autoCrop(seg *object) any {
	if value, ok := seg.get("crop_x_auto"); ok {
		return value
	}
	value, _ := seg.get("crop_x")
	return value
}

func segmentObjects(c *object) []*object {
	list, _ := c.values["segments"].([]any)
	var pieces []*object
	for _, item := range list {
		if seg, ok := item.(*object); ok {
			pieces = append(pieces, seg)
		}
	}
	return pieces
}

// MinCut is the shortest part worth taking out of a clip.
const MinCut = 0.05

// MinClip is the least a clip may be left holding.
const MinClip = 1.0

// Cut is one part a clip leaves out, the gap between two pieces.
type Cut struct {
	From float64
	To   float64
}

// ClipCuts are the parts a clip leaves out, in order. They are the gaps
// between its pieces, so a clip in one piece has none.
func ClipCuts(c Clip) []Cut {
	var cuts []Cut
	for i := 1; i < len(c.Segments); i++ {
		from, to := c.Segments[i-1].End, c.Segments[i].Start
		if to > from {
			cuts = append(cuts, Cut{roundTo(from, 3), roundTo(to, 3)})
		}
	}
	return cuts
}

// pieceSpan is how much of the source a set of pieces holds.
func pieceSpan(pieces []*object) float64 {
	lengths := make([]float64, len(pieces))
	for i, seg := range pieces {
		lengths[i] = number(seg.values["end"]) - number(seg.values["start"])
	}
	return pysum(lengths)
}

// editPieces is the one way the pieces of a clip are rebuilt. The change is
// handed the pieces in order and answers with the pieces that replace them.
// What comes back has to be a clip a person can still watch and the render
// can still make, so it is checked before anything is written. The words are
// taken again from the transcript, the caption file goes, because the
// captions are built from the words, and the edit is recorded for training.
func editPieces(planPath, clipID string, t *Transcript,
	change func(pieces []*object) ([]*object, error)) error {
	err := editPlan(planPath, func(_ *object, clips []*object) error {
		c, err := findClip(clips, clipID)
		if err != nil {
			return err
		}
		out, err := change(segmentObjects(c))
		if err != nil {
			return err
		}
		if len(out) == 0 {
			return renderErr("that would leave the clip with nothing in it")
		}
		if len(out) > MaxSegments {
			return renderErr("a clip can hold at most %d pieces", MaxSegments)
		}
		if span := pieceSpan(out); span < MinClip {
			return renderErr("a clip needs at least one second, that would leave %s",
				fixed(span, 2))
		}
		kept := make([]any, len(out))
		for i, seg := range out {
			kept[i] = seg
		}
		c.set("segments", kept)
		refreshWords(c, kept, t)
		return nil
	})
	if err != nil {
		return err
	}
	dropCaptionFile(planPath, clipID)
	_ = RecordDecision(planPath, clipID, DecisionEdited, nil)
	return nil
}

// Snap says whether a cut's edges are moved onto the words around them.
//
// ToWords is what the engine proposes and what a first drag does, because a
// cut that lands between words is right nearly every time and nobody wants
// to place one by hand. ToFrames leaves the edges exactly where they were
// put, for the times when a word has to be clipped a little or a breath
// kept, and then the picture is the only thing that says where the cut
// belongs. A cut made to frames may stop inside a word, which is the whole
// point of it.
type Snap bool

const (
	ToWords  Snap = true
	ToFrames Snap = false
)

// snapCut puts the edges of a cut where the render would cut them. A cut is
// not a trim seen from the other side: a trim moves an edge to the nearest
// word and keeps it, while a cut takes words away, so it has to be able to
// reach a word and it must never stop inside one. Half a word is a sound
// nobody said.
//
// So the cut first swallows every word it touches, then leaves keepPause of
// air on each side that stays, without reaching into a word that is going.
// A cut dragged over a pause takes the whole pause, and a cut dragged over
// speech takes whole words.
func snapCut(t *Transcript, from, to, keepPause float64) (float64, float64) {
	// What the cut touches goes with it.
	swallowedFrom, swallowedTo := math.Inf(1), math.Inf(-1)
	for _, w := range t.Words {
		if w.End > from && w.Start < to {
			swallowedFrom = math.Min(swallowedFrom, w.Start)
			swallowedTo = math.Max(swallowedTo, w.End)
		}
	}
	start := math.Min(from, swallowedFrom)
	end := math.Max(to, swallowedTo)

	// The last word that stays before the cut, and the first that stays
	// after it. The air belongs to the side that stays.
	before, after := math.Inf(-1), math.Inf(1)
	for _, w := range t.Words {
		if w.End <= start {
			before = math.Max(before, w.End)
		}
		if w.Start >= end {
			after = math.Min(after, w.Start)
		}
	}
	if !math.IsInf(before, -1) {
		start = math.Min(before+keepPause, swallowedFrom)
	}
	if !math.IsInf(after, 1) {
		end = math.Max(after-keepPause, swallowedTo)
	}
	return roundTo(math.Max(0, start), 3), roundTo(end, 3)
}

// CutClip takes a part out of the middle of a clip. A piece the cut lands
// inside becomes two, and both keep the framing of the piece they came from,
// so cutting never moves the picture. A piece the cut swallows whole goes.
// With ToWords the edges move onto the words around them, with ToFrames they
// stay where they were put.
func CutClip(planPath, clipID string, from, to float64, t *Transcript,
	keepPause float64, snap Snap) error {
	if snap == ToWords {
		from, to = snapCut(t, from, to, keepPause)
	} else {
		from, to = roundTo(math.Max(0, from), 3), roundTo(to, 3)
	}
	if to-from < MinCut {
		return renderErr("a cut has to take out more than that")
	}
	return editPieces(planPath, clipID, t, func(pieces []*object) ([]*object, error) {
		out := applyCut(pieces, from, to)
		if len(out) == len(pieces) && pieceSpan(out) >= pieceSpan(pieces) {
			return nil, renderErr("that cut falls outside the clip")
		}
		return out, nil
	})
}

// applyCut takes a part out of a set of pieces. A piece the cut straddles
// is split, and the half that is kept on each side is a copy of the whole,
// so the framing and the automatic framing behind it travel with both.
func applyCut(pieces []*object, from, to float64) []*object {
	var out []*object
	for _, seg := range pieces {
		start, end := number(seg.values["start"]), number(seg.values["end"])
		if from <= start && to >= end {
			continue
		}
		if to <= start || from >= end {
			out = append(out, seg)
			continue
		}
		if from > start {
			head := copyObject(seg)
			head.set("end", roundTo(from, 3))
			out = append(out, head)
		}
		if to < end {
			tail := copyObject(seg)
			tail.set("start", roundTo(to, 3))
			out = append(out, tail)
		}
	}
	return out
}

// JoinCut puts back the part a clip leaves out at a moment, so the two
// pieces around it become one. The framing of the piece before the cut is
// the one the joined piece keeps, because that is the shot it opens on.
func JoinCut(planPath, clipID string, at float64, t *Transcript) error {
	return editPieces(planPath, clipID, t, func(pieces []*object) ([]*object, error) {
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
	})
}

// MoveCut moves both edges of one of a clip's cuts, counted from the first.
// The pieces either side give way to it, and neither may be squeezed out of
// existence, so a cut that would swallow its neighbour is refused rather
// than quietly dropping a piece. With ToWords the edges move onto the words
// around them, with ToFrames they stay where they were put, which is how an
// edge is walked a frame at a time.
func MoveCut(planPath, clipID string, index int, from, to float64,
	t *Transcript, keepPause float64, snap Snap) error {
	if snap == ToWords {
		from, to = snapCut(t, from, to, keepPause)
	} else {
		from, to = roundTo(math.Max(0, from), 3), roundTo(to, 3)
	}
	if to-from < MinCut {
		return renderErr("a cut has to take out more than that")
	}
	return editPieces(planPath, clipID, t, func(pieces []*object) ([]*object, error) {
		if index < 0 || index+1 >= len(pieces) {
			return nil, renderErr("this clip has no cut number %d", index+1)
		}
		before, after := pieces[index], pieces[index+1]
		if from <= number(before.values["start"]) {
			return nil, renderErr("a cut cannot swallow the piece before it")
		}
		if to >= number(after.values["end"]) {
			return nil, renderErr("a cut cannot swallow the piece after it")
		}
		out := append([]*object{}, pieces...)
		out[index] = copyObject(before)
		out[index].set("end", roundTo(from, 3))
		out[index+1] = copyObject(after)
		out[index+1].set("start", roundTo(to, 3))
		return out, nil
	})
}

// SetCrop places the crop of the shot at a moment of a clip by hand, as the
// left edge in source pixels. Every piece of the clip that the analysis
// framed the same way, which is the same camera angle, moves with it. The
// automatic placement is kept, so ResetCrop can bring it back. Framing is
// not a judgement of the clip, so nothing is recorded for training.
func SetCrop(planPath, clipID string, at float64, left int) error {
	if left < 0 {
		return renderErr("the crop cannot start left of the picture")
	}
	left -= left % 2
	return editPlan(planPath, func(_ *object, clips []*object) error {
		c, err := findClip(clips, clipID)
		if err != nil {
			return err
		}
		pieces := segmentObjects(c)
		target := segmentAt(pieces, at)
		if target < 0 {
			return renderErr("clip %s has no pieces", clipID)
		}
		angle := fmt.Sprint(autoCrop(pieces[target]))
		for _, seg := range pieces {
			if fmt.Sprint(autoCrop(seg)) != angle {
				continue
			}
			if _, moved := seg.get("crop_x_auto"); !moved {
				value, _ := seg.get("crop_x")
				seg.set("crop_x_auto", value)
			}
			seg.set("crop_x", left)
		}
		return nil
	})
}

// ResetCrop brings back the automatic crop for the shot at a moment of a
// clip, for every piece with that camera angle.
// SetCaptionStyle changes how the captions of a whole clip set look. Only
// the settings the workspace offers can be changed here, and a value the
// render would quietly replace with its default is refused rather than
// written to the plan.
func SetCaptionStyle(planPath string, values map[string]any) error {
	if len(values) == 0 {
		return nil
	}
	checked := ResolveStyle(values)
	for key, value := range values {
		switch key {
		case "font":
			if text, ok := value.(string); !ok || text != checked.Font {
				return renderErr("%s cannot be used as a caption font", Scrub(pyStr(value), 40))
			}
		case "size":
			if size, ok := toFloat(value); !ok || size != checked.Size {
				return renderErr("%s is not a caption size between 8 and 400",
					Scrub(pyStr(value), 40))
			}
		case "highlight_colour":
			// The pill behind the word being spoken, as #RRGGBB, or as
			// &HAABBGGRR when it is given an opacity.
			text, ok := value.(string)
			hex := ok && len(text) == 7 && strings.HasPrefix(text, "#") && isHex(text[1:])
			if !hex && (!ok || !isAssColour(text)) {
				return renderErr("%s is not a highlight colour", Scrub(pyStr(value), 40))
			}
		case "primary", "back_colour":
			// The colour of the text and of the box behind it, written the
			// way the render takes them, &HAABBGGRR.
			text, ok := value.(string)
			if !ok || !isAssColour(text) {
				return renderErr("%s is not a caption colour", Scrub(pyStr(value), 40))
			}
		default:
			return renderErr("the caption setting %s cannot be changed here", Scrub(key, 40))
		}
	}
	return editPlan(planPath, func(top *object, _ []*object) error {
		style, _ := top.get("caption_style")
		into, ok := style.(*object)
		if !ok {
			into = &object{values: map[string]any{}}
			top.set("caption_style", into)
		}
		for key, value := range values {
			into.set(key, value)
		}
		return nil
	})
}

// SetCaptionY moves the caption line of a clip, as the distance from the
// bottom of a 1080x1920 frame. It lands on the nearest step and stays
// inside the frame, so captions across an episode line up.
func SetCaptionY(planPath, clipID string, y float64) error {
	if !isFinite(y) {
		return renderErr("the captions cannot sit at %v", y)
	}
	return editPlan(planPath, func(_ *object, clips []*object) error {
		c, err := findClip(clips, clipID)
		if err != nil {
			return err
		}
		c.set("caption_y", SnapCaptionY(y))
		return nil
	})
}

// SetCaptionTime moves the caption that begins or ends on a word of a clip,
// where the timing of the words is a little off from what is heard. word
// is when that word starts in the episode, edge is "start" or "end", and at
// is when the caption should appear or go, also in the episode. A time that
// is not a number puts the caption back where its words put it.
//
// It is kept against the word rather than the caption, because captions
// break in other places when the face or the size changes, and a word stays
// what it is.
func SetCaptionTime(planPath, clipID string, word float64, edge string, at float64) error {
	if edge != "start" && edge != "end" {
		return renderErr("a caption has a start and an end, not %s", Scrub(edge, 20))
	}
	reset := math.IsNaN(at)
	if !reset && (math.IsInf(at, 0) || at < 0 || at > MaxEpisodeSeconds) {
		return renderErr("a caption cannot be moved to %s", fixed(at, 3))
	}
	key := wordKey(word)
	err := editPlan(planPath, func(_ *object, clips []*object) error {
		c, err := findClip(clips, clipID)
		if err != nil {
			return err
		}
		// Only a word of this clip. A caption is made of its words and of
		// nothing else.
		found := false
		list, _ := c.values["words"].([]any)
		for _, item := range list {
			if triple, ok := item.([]any); ok && len(triple) == 3 && wordKey(number(triple[0])) == key {
				found = true
				break
			}
		}
		if !found {
			return renderErr("the clip has no word at %s", HMS(word))
		}
		times, _ := c.values["caption_times"].(*object)
		if times == nil {
			times = newObject()
		}
		edges, _ := times.values[key].(*object)
		if edges == nil {
			edges = newObject()
		}
		if reset {
			edges.remove(edge)
		} else {
			edges.set(edge, roundTo(at, 3))
		}
		if len(edges.keys) == 0 {
			times.remove(key)
		} else {
			times.set(key, edges)
		}
		if len(times.keys) == 0 {
			c.remove("caption_times")
		} else {
			c.set("caption_times", times)
		}
		return nil
	})
	if err != nil {
		return err
	}
	dropCaptionFile(planPath, clipID)
	return nil
}

// ResetCaptionY puts the captions of a clip back where the caption style
// puts them.
func ResetCaptionY(planPath, clipID string) error {
	return editPlan(planPath, func(_ *object, clips []*object) error {
		c, err := findClip(clips, clipID)
		if err != nil {
			return err
		}
		c.remove("caption_y")
		return nil
	})
}

// ClearCaptionY takes the hand-placed caption line off every clip of a
// plan, so they all follow the one place the app keeps. It answers with
// how many clips changed.
func ClearCaptionY(planPath string) (int, error) {
	changed := 0
	err := editPlan(planPath, func(_ *object, clips []*object) error {
		for _, c := range clips {
			if _, ok := c.get("caption_y"); ok {
				c.remove("caption_y")
				changed++
			}
		}
		return nil
	})
	if err != nil {
		return 0, err
	}
	return changed, nil
}

func ResetCrop(planPath, clipID string, at float64) error {
	return editPlan(planPath, func(_ *object, clips []*object) error {
		c, err := findClip(clips, clipID)
		if err != nil {
			return err
		}
		pieces := segmentObjects(c)
		target := segmentAt(pieces, at)
		if target < 0 {
			return renderErr("clip %s has no pieces", clipID)
		}
		angle := fmt.Sprint(autoCrop(pieces[target]))
		for _, seg := range pieces {
			auto, moved := seg.get("crop_x_auto")
			if !moved || fmt.Sprint(auto) != angle {
				continue
			}
			seg.set("crop_x", auto)
			seg.remove("crop_x_auto")
		}
		return nil
	})
}

// IsPlanFile reports whether a path is named the way plan files are named.
// Removing a search deletes a file, so what it is handed has to be a plan
// and not, say, the transcript beside it.
func IsPlanFile(path string) bool {
	return planNameRe.MatchString(filepath.Base(path))
}

var planNameRe = regexp.MustCompile(`^clips(-\d+-\d+)?\.json$`)

// RemovePlan takes a whole search out of an episode: the plan file goes, and
// with it the clips it held. The part it covered is free to be searched
// again afterwards.
//
// The caption files of its clips are moved aside rather than deleted,
// because they may hold corrections made by hand, and because a later plan
// could otherwise inherit the caption text of a clip it has nothing to do
// with. Rendered files are finished work and are left alone.
func RemovePlan(planPath, captionsDir string) error {
	if !IsPlanFile(planPath) {
		return renderErr("%s is not a plan", filepath.Base(planPath))
	}
	// An edit of this plan may be halfway through writing it.
	defer lockPlan(planPath)()
	_, clips, err := LoadClips(planPath)
	if err != nil && !os.IsNotExist(err) {
		return err
	}
	if err := setCaptionsAside(captionsDir, clips); err != nil {
		return err
	}
	if err := os.Remove(planPath); err != nil && !os.IsNotExist(err) {
		return err
	}
	return nil
}

// setCaptionsAside moves the caption files of clips that are going away
// into a folder of their own, dated, rather than deleting them.
func setCaptionsAside(captionsDir string, clips []Clip) error {
	var files []string
	for _, clip := range clips {
		for _, ext := range []string{".srt", ".ass"} {
			// The basename comes out of a plan file, which is untrusted.
			name, err := SafeChild(captionsDir, clip.Basename()+ext)
			if err != nil {
				return err
			}
			if isFile(name) {
				files = append(files, name)
			}
		}
	}
	if len(files) == 0 {
		return nil
	}
	attic := filepath.Join(captionsDir, "superseded-"+time.Now().Format("20060102-150405"))
	if err := os.MkdirAll(attic, 0o755); err != nil {
		return err
	}
	for _, old := range files {
		if err := os.Rename(old, filepath.Join(attic, filepath.Base(old))); err != nil {
			return err
		}
	}
	return nil
}

// ClipSpan is the part of the episode a clip was cut from, from the
// start of its first piece to the end of its last.
func ClipSpan(c Clip) (float64, float64) {
	if len(c.Segments) == 0 {
		return 0, 0
	}
	return c.Segments[0].Start, c.Segments[len(c.Segments)-1].End
}

// RemoveRange gives a part of a plan's window back. The clips that lie
// in the part go with it, and the plan notes the part as one the
// model may read again, so the range picker shows it as free. A plan whose
// whole window is given back goes altogether.
//
// It answers with how many clips went. Caption files are moved aside, as
// they are when a whole plan goes.
func RemoveRange(planPath, captionsDir string, from, to, duration float64) (int, error) {
	if !IsPlanFile(planPath) {
		return 0, renderErr("%s is not a plan", filepath.Base(planPath))
	}
	if to <= from {
		return 0, nil
	}
	plan, clips, err := LoadClips(planPath)
	if err != nil {
		if os.IsNotExist(err) {
			return 0, nil
		}
		return 0, err
	}
	window := planWindow(plan, duration)
	start, end := math.Max(from, window.Start), math.Min(to, window.End)
	if end <= start {
		return 0, nil
	}
	var going []Clip
	for _, clip := range clips {
		a, b := ClipSpan(clip)
		if b > start && a < end {
			going = append(going, clip)
		}
	}
	// Nothing of the window is left, so the plan itself goes.
	if len(Without(window, append(readWindows(plan.PlannedWith()["removed"]), Window{start, end}))) == 0 {
		return len(going), RemovePlan(planPath, captionsDir)
	}
	if err := setCaptionsAside(captionsDir, going); err != nil {
		return 0, err
	}
	gone := map[string]bool{}
	for _, clip := range going {
		gone[clip.ID] = true
	}
	err = editPlan(planPath, func(top *object, clips []*object) error {
		kept := make([]any, 0, len(clips))
		for i, c := range clips {
			// The same name the loader gives a clip, because an id in a
			// plan file is untrusted and may not be there at all.
			fallback := twoDigits(i + 1)
			text := fallback
			if raw, ok := c.get("id"); ok {
				text = pyStr(raw)
			}
			if gone[SanitiseName(text, fallback)] {
				continue
			}
			kept = append(kept, c)
		}
		top.set("clips", kept)
		made, ok := top.values["planned_with"].(*object)
		if !ok {
			made = newObject()
			top.set("planned_with", made)
		}
		holes := append(orderedWindows(made.values["removed"]), Window{start, end})
		list := make([]any, 0, len(holes))
		for _, h := range MergeWindows(holes) {
			list = append(list, map[string]any{"from": h.Start, "to": h.End})
		}
		made.set("removed", list)
		return nil
	})
	if err != nil {
		return 0, err
	}
	return len(going), nil
}

// orderedWindows reads windows out of a plan being edited, where objects
// keep their key order. It is readWindows for that tree.
func orderedWindows(raw any) []Window {
	list, ok := raw.([]any)
	if !ok {
		return nil
	}
	var out []Window
	for _, item := range list {
		o, ok := item.(*object)
		if !ok {
			continue
		}
		start, okS := toFloat(o.values["from"])
		end, okE := toFloat(o.values["to"])
		if !okS || !okE || end <= start {
			continue
		}
		out = append(out, Window{start, end})
	}
	return MergeWindows(out)
}

// planWindow is the window a plan was made over. A plan without one was
// made over the whole episode.
func planWindow(plan Plan, duration float64) Window {
	made := plan.PlannedWith()
	from, _ := toFloat(made["from"])
	to, okTo := toFloat(made["to"])
	if !okTo || to <= from {
		if duration > 0 {
			return Window{0, duration}
		}
		return Window{0, math.Inf(1)}
	}
	return Window{from, to}
}
