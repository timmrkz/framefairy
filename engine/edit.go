package engine

import (
	"bytes"
	"encoding/json"
	"errors"
	"fmt"
	"maps"
	"math"
	"os"
	"path/filepath"
	"regexp"
	"slices"
	"sort"
	"strings"

	"github.com/spf13/afero"

	"framefairy/engine/fileio"
	"framefairy/engine/num"
	"framefairy/engine/runes"
	"framefairy/engine/timed"
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

// errUnchanged is what a change to a plan says when it changes nothing,
// so the plan is not written and keeps its revision.
var errUnchanged = errors.New("the plan is unchanged")

// editPlan loads a plan, lets change edit it, and writes it back in one
// step. The file is replaced atomically, so a crash never leaves half a
// plan. The result is checked with the same loader a render uses before it
// is written. Edits of one file are one at a time, see fileio.Lock.
func editPlan(fsys afero.Fs, path string, change func(top *object, clips []*object) error) error {
	defer fileio.Lock(fsys, path)()
	return editPlanLocked(fsys, path, change)
}

// editPlanLocked is editPlan for a caller that already holds the plan's
// lock.
func editPlanLocked(fsys afero.Fs, path string, change func(top *object, clips []*object) error) error {
	data, err := afero.ReadFile(fsys, path)
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
	if list, ok := top.values[keyClips].([]any); ok {
		for _, item := range list {
			if c, ok := item.(*object); ok {
				clips = append(clips, c)
			}
		}
	}
	if err := change(top, clips); err != nil {
		if errors.Is(err, errUnchanged) {
			return nil
		}
		return err
	}
	revision := 0
	if raw, ok := top.get(keyRevision); ok {
		revision, _ = num.AsInt(raw)
	}
	top.set(keyRevision, revision+1)

	body, err := MarshalPlan(top)
	if err != nil {
		return err
	}
	// The lock is already held, so this is the unlocked one.
	return replacePlan(fsys, path, body)
}

// writePlanFile puts a whole new plan where a plan goes. A search that has
// just finished uses it, and so does a search over a window that was
// searched before, which lands on a file that is already there.
//
// It takes the same lock every edit takes, because the file has two
// writers: the search that makes a plan and the app that edits one.
// Without the lock a search can land in the middle of an edit reading the
// file, changing it and writing it back.
func writePlanFile(fsys afero.Fs, path string, body []byte) error {
	defer fileio.Lock(fsys, path)()
	if err := fsys.MkdirAll(filepath.Dir(path), 0o755); err != nil {
		return err
	}
	return replacePlan(fsys, path, body)
}

// replacePlan puts new contents in a plan file in one step. The caller
// holds the lock for that plan.
//
// Writing over the file itself would be wrong twice. The app reads the
// clips and the coverage about once a second while a search runs, and it
// would read a plan that is half there, and the clip list empties itself.
// And a write that goes wrong part of the way through would have taken the
// plan that was there with it.
//
// So it is written beside the file and moved onto it, which is one step as
// far as any reader is concerned, and only after it has been read back as
// a plan. The temporary name starts with a dot, because what lists the
// plans of an episode globs for clips*.json and a half written plan must
// never be one of them.
func replacePlan(fsys afero.Fs, path string, body []byte) error {
	return fileio.Replace(fsys, path, ".clips-*.json", body, func(tmp string) error {
		_, _, err := LoadClips(fsys, tmp)
		return err
	})
}

// errNotAdded is a clip that was not added because the plan has it
// already.
var errNotAdded = errors.New("the clip is in the plan already")

// appendClip adds a clip to a plan that a search is still writing. It goes
// through editPlan, so an edit the app makes at the same moment is kept,
// and so is the plan's shape. A plan that is gone is reported as the
// missing file it is.
func appendClip(fsys afero.Fs, path string, clip PlanClip) error {
	defer fileio.Lock(fsys, path)()
	return appendClipLocked(fsys, path, clip)
}

// appendClipLocked is appendClip for a caller that holds the plan's lock.
func appendClipLocked(fsys afero.Fs, path string, clip PlanClip) error {
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
	refused := false
	err = editPlanLocked(fsys, path, func(top *object, clips []*object) error {
		for i, c := range clips {
			fallback := twoDigits(i + 1)
			text := fallback
			if raw, ok := c.get(keyID); ok {
				text = num.FormatValue(raw)
			}
			if SanitiseName(text, fallback) == clip.ID {
				// Already there, which a search that was asked again
				// for the same answer would otherwise write twice.
				refused = true
				return errNotAdded
			}
		}
		list, _ := top.values[keyClips].([]any)
		top.set(keyClips, append(list, added))
		return nil
	})
	if refused {
		return errNotAdded
	}
	return err
}

// addClip adds a clip to a clip set that grows, numbered after the clips
// it has, prefix and the next number, and makes the set, as set with this
// one clip in it, when it is not there yet. The number is taken under the
// set's lock, so two clips made at the same moment never take the same
// one. It answers with the clip as it went in.
func addClip(fsys afero.Fs, path string, set PlanFile, clip PlanClip, prefix string) (PlanClip, error) {
	defer fileio.Lock(fsys, path)()
	clip.ID = fmt.Sprintf("%s%02d", prefix, lastNumber(fsys, path, prefix)+1)
	if _, err := fsys.Stat(path); !os.IsNotExist(err) {
		return clip, appendClipLocked(fsys, path, clip)
	}
	set.Clips = []PlanClip{clip}
	body, err := MarshalPlan(set)
	if err != nil {
		return clip, err
	}
	if err := fsys.MkdirAll(filepath.Dir(path), 0o755); err != nil {
		return clip, err
	}
	return clip, replacePlan(fsys, path, body)
}

func findClip(clips []*object, id string) (*object, error) {
	for i, c := range clips {
		fallback := twoDigits(i + 1)
		text := fallback
		if raw, ok := c.get(keyID); ok {
			text = num.FormatValue(raw)
		}
		if SanitiseName(text, fallback) == id {
			return c, nil
		}
	}
	return nil, renderErr("no clip %s in the plan", id)
}

// SetRejected marks a clip as rejected or kept. A rejected clip stays in the
// plan, and a render of the whole plan leaves it out. Reasons are recorded
// with a rejection for training, in rec.
func SetRejected(fsys afero.Fs, rec *Records, planPath, clipID string, rejected bool, reasons ...string) error {
	// Checked before the plan is touched. A reason the format does not know
	// would throw the whole record away further down, without a word.
	if _, err := CheckReasons(reasons); rejected && err != nil {
		return err
	}
	err := editPlan(fsys, planPath, func(_ *object, clips []*object) error {
		c, err := findClip(clips, clipID)
		if err != nil {
			return err
		}
		if rejected {
			c.set(keyRejected, true)
		} else {
			c.remove(keyRejected)
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
	_ = RecordDecision(fsys, rec, planPath, clipID, event, reasons)
	return nil
}

// DropRemoved takes a removed clip out of its plan for good. Only the one
// clip of an episode shorter than a clip is ever taken out, once a search
// has made it again: there is one moment, and the clip made again is it.
// A clip that is not removed, or no longer there, is left as it is.
func DropRemoved(fsys afero.Fs, planPath, clipID string) error {
	return editPlan(fsys, planPath, func(top *object, clips []*object) error {
		c, err := findClip(clips, clipID)
		if err != nil {
			return nil
		}
		if rejected, _ := c.get(keyRejected); rejected != true {
			return nil
		}
		list, _ := top.values[keyClips].([]any)
		kept := []any{}
		for _, item := range list {
			if item != c {
				kept = append(kept, item)
			}
		}
		top.set(keyClips, kept)
		return nil
	})
}

func number(v any) float64 {
	f, _ := num.AsFloat(v)
	return f
}

// SnapStart is where a clip starting at the word nearest to at begins: a
// little before the word, never reaching back into the word before it.
func SnapStart(words []timed.Cue, at, keepPause float64) float64 {
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
	return num.RoundTo(math.Max(0, edge), 3)
}

// SnapEnd is where a clip ending at the word nearest to at ends: a little
// after the word, never reaching into the word after it. Where a caption
// goes, one of goes, is a place to end too, when it is nearer than the word
// and in the pause after it.
func SnapEnd(words []timed.Cue, goes []float64, at, keepPause float64) float64 {
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
	near := math.Abs(words[best].End - at)
	for _, g := range goes {
		if math.Abs(g-at) < near && inPause(words, g, keepPause) {
			edge, near = g, math.Abs(g-at)
		}
	}
	return num.RoundTo(edge, 3)
}

// inPause says whether at is in the air between two words, more than
// keepPause after the one before and before the next begins.
func inPause(words []timed.Cue, at, keepPause float64) bool {
	i := sort.Search(len(words), func(j int) bool { return words[j].End > at }) - 1
	if i < 0 || at <= words[i].End+keepPause {
		return false
	}
	return i+1 == len(words) || at < words[i+1].Start
}

// MaxClipSpan keeps a trimmed clip to a sensible length.
const MaxClipSpan = 600.0

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
		start, end := number(seg.values[keyStart]), number(seg.values[keyEnd])
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
	if value, ok := seg.get(keyCropXAuto); ok {
		return value
	}
	value, _ := seg.get(keyCropX)
	return value
}

// RotatePart rotates the part of a clip a moment is in a quarter turn to
// the left, the way Preview and Photos rotate. A part is the pieces between
// two cuts, or between a cut and an edge of the clip: what plays straight
// on. What turns is the stretch of the video the part shows, see Rotation,
// from the middle of the cut before it to the middle of the cut after,
// where the phone was turned and the cut took the turn out. A part at an
// edge of the clip turns as far as that end of the video, so its edge
// dragged out shows the footage turned. Each edge of the stretch stands
// on the start of a frame, the only place a video is cut. Four turns bring
// it back.
//
// The crop of a rotated piece is the one the pieces turned the same way
// already have, or stands in the middle of the picture rotated until it is
// placed, and the crop it has unrotated is kept as it was, so a part
// rotated back has it again. So are the keyframes of the stretch:
// each was placed on the picture turned one way and waits while it is
// turned another, see CropMark, so four turns bring the glide back. Tim
// found the diamonds removed with the turn, issue #348. Framing is not a
// judgement of the clip, so nothing is recorded for training.
func RotatePart(fsys afero.Fs, planPath, clipID string, at float64, frames SourceInfo) error {
	return editPlan(fsys, planPath, func(_ *object, clips []*object) error {
		c, err := findClip(clips, clipID)
		if err != nil {
			return err
		}
		pieces := segmentObjects(c)
		target := segmentAt(pieces, at)
		if target < 0 {
			return renderErr("clip %s has no pieces", clipID)
		}
		spans := rotationsOf(c)
		// The keyframes are read with the clip turned as it was, so each
		// keeps the turn it was placed on, written down before the turn.
		marks := cropMarksOf(c)
		first, last := partOf(pieces, target)
		from, to := math.Inf(-1), math.Inf(1)
		if first > 0 {
			from = frames.frameStartNear((number(pieces[first-1].values[keyEnd]) + number(pieces[first].values[keyStart])) / 2)
		}
		if last+1 < len(pieces) {
			to = frames.frameStartNear((number(pieces[last].values[keyEnd]) + number(pieces[last+1].values[keyStart])) / 2)
		}
		turned := (rotationOf(pieces[target], spans) + 90) % 360
		spans = turnStretch(spans, from, to, turned)
		setRotations(c, spans)
		if len(marks) > 0 {
			setCropMarks(c, marks)
		}
		var stretch []*object
		for _, seg := range pieces {
			if number(seg.values[keyEnd]) > from && number(seg.values[keyStart]) < to {
				seg.remove(keyCropXTurned)
				stretch = append(stretch, seg)
			}
		}
		// The pieces turned alike share one crop, so a part turned the
		// way another already is takes the crop placed there. Left in
		// the middle, the clip had two crops turned one way, #402.
		_, like := framingOf(pieces, spans)
		for _, seg := range stretch {
			copyFraming(seg, like[turned], keyCropXTurned)
		}
		return nil
	})
}

// PiecesMeet is the most two pieces of a clip may stand apart and still
// play straight on, with no cut between them: the edge of a rotated part,
// not a cut. The window reads pieces by the same rule,
// partOf in lib/rotate.ts.
const PiecesMeet = 0.0005

// partOf is the first and last of the pieces around one that play straight
// on from each other, with no cut between them.
func partOf(pieces []*object, i int) (int, int) {
	first, last := i, i
	for first > 0 && number(pieces[first].values[keyStart])-number(pieces[first-1].values[keyEnd]) <= PiecesMeet {
		first--
	}
	for last+1 < len(pieces) && number(pieces[last+1].values[keyStart])-number(pieces[last].values[keyEnd]) <= PiecesMeet {
		last++
	}
	return first, last
}

func segmentObjects(c *object) []*object {
	list, _ := c.values[keySegments].([]any)
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
			cuts = append(cuts, Cut{num.RoundTo(from, 3), num.RoundTo(to, 3)})
		}
	}
	return cuts
}

// pieceSpan is how much of the source a set of pieces holds.
func pieceSpan(pieces []*object) float64 {
	lengths := make([]float64, len(pieces))
	for i, seg := range pieces {
		lengths[i] = number(seg.values[keyEnd]) - number(seg.values[keyStart])
	}
	return num.Sum(lengths)
}

// pieceChange is what a gesture does to a clip's pieces. It is handed the
// pieces in order and answers with the pieces that replace them.
type pieceChange func(pieces []*object) ([]*object, error)

// editPieces is the one way the pieces of a clip are rebuilt. The change is
// handed the pieces in order and answers with the pieces that replace them.
// What comes back has to be a clip a person can still watch and the render
// can still make, so it is checked before anything is written. The words are
// taken again from the transcript, the caption file goes, because the
// captions are built from the words, and the edit is recorded for training,
// in rec.
func editPieces(fsys afero.Fs, rec *Records, planPath, clipID string, change pieceChange, frames SourceInfo) error {
	err := editPlan(fsys, planPath, func(_ *object, clips []*object) error {
		c, err := findClip(clips, clipID)
		if err != nil {
			return err
		}
		before := segmentObjects(c)
		out, err := checkedPieces(change, before, rotationsOf(c), frames)
		if err != nil {
			return err
		}
		// The pieces a clip was found with were kept, once, to frame what
		// a gesture put back by the camera it showed. A clip has one crop
		// now, #396, so a plan that still has them loses them.
		c.remove(keyFoundSegments)
		// Where the clip's edges were before anything changed them, kept
		// the first time anything does, so a double-click on an edge can
		// put it back. A clip that has it already keeps what it has.
		if _, ok := c.values[keyFound]; !ok && len(before) > 0 {
			start, err1 := ParseTime(before[0].values[keyStart])
			end, err2 := ParseTime(before[len(before)-1].values[keyEnd])
			if err1 == nil && err2 == nil && end > start {
				c.set(keyFound, []any{num.RoundTo(start, 3), num.RoundTo(end, 3)})
			}
		}
		kept := make([]any, len(out))
		for i, seg := range out {
			kept[i] = seg
		}
		c.set(keySegments, kept)
		return nil
	})
	if err != nil {
		return err
	}
	_ = RecordDecision(fsys, rec, planPath, clipID, DecisionEdited, nil)
	return nil
}

// checkedPieces makes a change to a clip's pieces, puts their edges on
// the episode's frames, see PieceOnFrames, and checks that what comes
// back is a clip a person can still watch and the render can still make.
func checkedPieces(change pieceChange, pieces []*object, spans []Rotation, frames SourceInfo) ([]*object, error) {
	out, err := change(pieces)
	if err != nil {
		return nil, err
	}
	out = partedAtTurns(oneFraming(out, spans), spans)
	if out, err = piecesOnFrames(out, frames); err != nil {
		return nil, err
	}
	if len(out) == 0 {
		return nil, renderErr("that would leave the clip with nothing in it")
	}
	if len(out) > MaxSegments {
		return nil, renderErr("a clip can hold at most %d pieces", MaxSegments)
	}
	if span := pieceSpan(out); span < MinClip {
		return nil, renderErr("a clip needs at least one second, that would leave %s", num.Fixed(span, 2))
	}
	return out, nil
}

// oneFraming gives every piece of a clip the clip's crop, and makes pieces
// that meet framed alike one piece. A clip has one crop, and one for each
// way a part of it is turned, #396. One framed before that has a crop for
// each camera it shows, and takes the one it already plays with from its
// next change on, see framingOf.
func oneFraming(pieces []*object, spans []Rotation) []*object {
	upright, turned := framingOf(pieces, spans)
	var out []*object
	for _, seg := range pieces {
		like := turned[rotationOf(seg, spans)]
		if !sameFraming(seg, upright, keyCropX, keyCropXAuto) || !sameFraming(seg, like, keyCropXTurned) {
			seg = copyObject(seg)
			copyFraming(seg, upright, keyCropX, keyCropXAuto)
			copyFraming(seg, like, keyCropXTurned)
		}
		out = meet(out, []*object{seg})
	}
	return out
}

// framingOf is the piece a clip takes its crop from, and for each way a
// part of it is turned the piece that part takes its crop from. The crop
// as filmed, crop_x, is the whole clip's, turned or not, since a part
// turned back plays with it. Only the crop on the picture turned,
// crop_x_turned, is a turn's own. Of a clip framed before #396, with a
// crop for each camera, the first piece placed by hand is taken, since a
// crop a person placed is a decision and one the analysis chose a guess,
// and the first piece where none was. A turn takes its first piece placed
// on the picture turned, or its first piece. A piece standing as filmed
// has no crop on the picture turned, so it takes one with none. Given the
// crop of the first piece when that was turned, the pieces as filmed of a
// clip whose first part was turned were written with it, #413.
func framingOf(pieces []*object, spans []Rotation) (*object, map[int]*object) {
	var upright *object
	turned := map[int]*object{0: newObject()}
	for _, seg := range pieces {
		if upright == nil || (movedByHand(seg) && !movedByHand(upright)) {
			upright = seg
		}
		turn := rotationOf(seg, spans)
		if turn == 0 {
			continue
		}
		_, placed := seg.get(keyCropXTurned)
		if like, ok := turned[turn]; !ok {
			turned[turn] = seg
		} else if _, had := like.get(keyCropXTurned); placed && !had {
			turned[turn] = seg
		}
	}
	return upright, turned
}

// movedByHand says whether a piece's crop as filmed was placed by hand.
func movedByHand(seg *object) bool {
	_, moved := seg.get(keyCropXAuto)
	return moved
}

// copyFraming gives a piece the keys of another's framing, or takes them
// away where the other has none.
func copyFraming(seg, like *object, keys ...string) {
	for _, key := range keys {
		if v, ok := like.get(key); ok {
			seg.set(key, v)
		} else {
			seg.remove(key)
		}
	}
}

// piecesOnFrames is a clip's pieces with their edges on the episode's
// frames, see PieceOnFrames. A piece that moves is a copy, because the
// one it was made from can still be the clip's own. A cut narrower than a
// frame would close, and two pieces that meet are the edge of a rotated
// part, not a cut, so that is refused. Without a frame rate the pieces are left as
// they are.
func piecesOnFrames(pieces []*object, frames SourceInfo) ([]*object, error) {
	if frames.FPSNum <= 0 || frames.FPSDen <= 0 {
		return pieces, nil
	}
	out := make([]*object, len(pieces))
	for i, seg := range pieces {
		start, end := number(seg.values[keyStart]), number(seg.values[keyEnd])
		out[i] = seg
		s, e := frames.PieceOnFrames(start, end)
		if i > 0 && start > number(pieces[i-1].values[keyEnd]) && s <= number(out[i-1].values[keyEnd]) {
			return nil, renderErr("a cut has to take out more than that")
		}
		if s != start || e != end {
			out[i] = copyObject(seg)
			out[i].set(keyStart, s)
			out[i].set(keyEnd, e)
		}
	}
	return out, nil
}

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
//
// Its left edge stops where a caption goes too, one of goes, when that is
// in the pause before it: a caption stays up a little after its last word,
// and the cut then begins where its block ends rather than inside it.
func snapCut(words []timed.Cue, goes []float64, from, to, keepPause float64) (float64, float64) {
	// What the cut touches goes with it.
	swallowedFrom, swallowedTo := math.Inf(1), math.Inf(-1)
	for _, w := range words {
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
	for _, w := range words {
		if w.End <= start {
			before = math.Max(before, w.End)
		}
		if w.Start >= end {
			after = math.Min(after, w.Start)
		}
	}
	if !math.IsInf(before, -1) {
		start = math.Min(before+keepPause, swallowedFrom)
		for _, g := range goes {
			if g > start && g <= math.Min(from, swallowedFrom) && inPause(words, g, keepPause) {
				start = g
			}
		}
	}
	if !math.IsInf(after, 1) {
		end = math.Max(after-keepPause, swallowedTo)
	}
	return num.RoundTo(math.Max(0, start), 3), num.RoundTo(end, 3)
}

// applyCut takes a part out of a set of pieces. A piece the cut straddles
// is split, and the half that is kept on each side is a copy of the whole,
// so the framing and the automatic framing behind it travel with both.
func applyCut(pieces []*object, from, to float64) []*object {
	var out []*object
	for _, seg := range pieces {
		start, end := number(seg.values[keyStart]), number(seg.values[keyEnd])
		if from <= start && to >= end {
			continue
		}
		if to <= start || from >= end {
			out = append(out, seg)
			continue
		}
		if from > start {
			head := copyObject(seg)
			head.set(keyEnd, num.RoundTo(from, 3))
			out = append(out, head)
		}
		if to < end {
			tail := copyObject(seg)
			tail.set(keyStart, num.RoundTo(to, 3))
			out = append(out, tail)
		}
	}
	return out
}

// SetCrop places the crop of a clip by hand, as the left edge in source
// pixels, at a moment of it. A clip has one crop for the picture as
// filmed, which every piece keeps, turned or not, and one for each way a
// part of it is turned, which the pieces turned alike share. The
// automatic placement is kept, so ResetCrop can bring it back. Framing is
// not a judgement of the clip, so nothing is recorded for training.
//
// With keyframes, see CropMark, a crop placed on the frame of a mark moves
// that mark alone. Placed anywhere else, every mark of the turn moves by
// as far as the crop shown there did, so a glide keeps its shape. frames
// is the video, for the frame a moment means, and cropW how wide the crop
// is on the picture as the piece at the moment stands, for where a crop
// that was never placed stands.
func SetCrop(fsys afero.Fs, planPath, clipID string, at float64, left int, frames SourceInfo, cropW int) error {
	if left < 0 {
		return renderErr("the crop cannot start left of the picture")
	}
	left -= left % 2
	return editPlan(fsys, planPath, func(_ *object, clips []*object) error {
		c, err := findClip(clips, clipID)
		if err != nil {
			return err
		}
		pieces := segmentObjects(c)
		target := segmentAt(pieces, at)
		if target < 0 {
			return renderErr("clip %s has no pieces", clipID)
		}
		spans := rotationsOf(c)
		turn := rotationOf(pieces[target], spans)
		marks := cropMarksOf(c)
		ofTurn := marksTurned(marks, turn)
		// The keyframe on the frame takes the crop, also one that waits,
		// placed on the picture turned another way: it is the diamond
		// under the playhead, and from now on it is on the picture as it
		// stands.
		for i := range marks {
			if frames.markFrame(marks[i].At) == frames.frameHolding(at) {
				marks[i].X = left
				marks[i].Turn = rotationOf(pieces[target], spans)
				setCropMarks(c, marks)
				return nil
			}
		}
		if len(ofTurn) > 0 {
			width := frames.Rotated(turn).Width
			shown := (width - cropW) / 2
			if x := cropShown(c, pieces, target, at, frames, cropW, width); x != nil {
				shown = *x
			}
			// A mark moved with the rest stays in the picture, or the
			// render would hold it at the edge while the others move on,
			// and the next drag would move it from a place nobody sees.
			for i := range marks {
				if ofTurn[i] {
					x := max(0, marks[i].X+left-shown)
					if width > cropW {
						x = min(x, width-cropW)
					}
					marks[i].X = x - x%2
				}
			}
			setCropMarks(c, marks)
		}
		// The crop as filmed is the whole clip's, so a piece that is
		// turned keeps it too, for when it is turned back. A rotated
		// piece's crop is on the picture rotated, kept apart from the one
		// it has as filmed, and only the pieces turned alike share it, see
		// keyCropXTurned. Placed on the pieces as filmed alone, a part
		// turned and turned back again brought its old crop back, and the
		// clip played with that one, found by the review of #398.
		upright, _ := framingOf(pieces, spans)
		auto := autoCrop(upright)
		for _, seg := range pieces {
			if turn != 0 {
				if rotationOf(seg, spans) == turn {
					seg.set(keyCropXTurned, left)
				}
				continue
			}
			seg.set(keyCropXAuto, auto)
			seg.set(keyCropX, left)
		}
		return nil
	})
}

// SetCaptionStyle changes how the captions of a whole clip set look. Only
// the settings the workspace offers can be changed here, and a value the
// render would quietly replace with its default is refused rather than
// written to the plan.
func SetCaptionStyle(fsys afero.Fs, planPath string, values map[string]any) error {
	if len(values) == 0 {
		return nil
	}
	checked := ResolveStyle(values)
	for key, value := range values {
		switch key {
		case "font":
			if text, ok := value.(string); !ok || text != checked.Font {
				return renderErr("%s cannot be used as a caption font", runes.Scrub(num.FormatValue(value), 40))
			}
		case "size":
			if size, ok := num.AsFloat(value); !ok || size != checked.Size {
				return renderErr("%s is not a caption size between 8 and 400",
					runes.Scrub(num.FormatValue(value), 40))
			}
		case "highlight_colour":
			// The pill behind the word being spoken, as #RRGGBB, or as
			// &HAABBGGRR when it is given an opacity.
			text, ok := value.(string)
			hex := ok && len(text) == 7 && strings.HasPrefix(text, "#") && isHex(text[1:])
			if !hex && (!ok || !isAssColour(text)) {
				return renderErr("%s is not a highlight colour", runes.Scrub(num.FormatValue(value), 40))
			}
		case "highlight", "text", "box":
			// The word being spoken on its pill, or the words alone. The
			// captions at all, and the box behind them. The render reads a
			// number, so that is what is written.
			on, ok := value.(bool)
			if !ok {
				return renderErr("%s is neither on nor off", runes.Scrub(num.FormatValue(value), 40))
			}
			values[key] = 0.0
			if on {
				values[key] = 1.0
			}
		case "primary", "back_colour":
			// The colour of the text and of the box behind it, written the
			// way the render takes them, &HAABBGGRR.
			text, ok := value.(string)
			if !ok || !isAssColour(text) {
				return renderErr("%s is not a caption colour", runes.Scrub(num.FormatValue(value), 40))
			}
		default:
			return renderErr("the caption setting %s cannot be changed here", runes.Scrub(key, 40))
		}
	}
	return editPlan(fsys, planPath, func(top *object, _ []*object) error {
		style, _ := top.get(keyCaptionStyle)
		into, ok := style.(*object)
		if !ok {
			into = &object{values: map[string]any{}}
			top.set(keyCaptionStyle, into)
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
func SetCaptionY(fsys afero.Fs, planPath, clipID string, y float64) error {
	if !num.IsFinite(y) {
		return renderErr("the captions cannot sit at %v", y)
	}
	return editPlan(fsys, planPath, func(_ *object, clips []*object) error {
		c, err := findClip(clips, clipID)
		if err != nil {
			return err
		}
		c.set(keyCaptionY, SnapCaptionY(y))
		return nil
	})
}

// CaptionEdit is what moving an edge of a caption did to the words it
// shows: how many it took out of the caption and how many it put back.
// The words stay in the clip, see moveCaptions.
type CaptionEdit struct {
	Out, Back int
}

// SetCaptionTime moves the caption that begins or ends on a word of a clip,
// where the timing of the words is a little off from what is heard. word
// is when that word starts in the episode, edge is "start" or "end", and at
// is when the caption should appear or go, also in the episode. A time that
// is not a number puts the caption back where its words put it. toWords
// puts the edge on the word nearest at, the way shift does, see
// captionStop. look is
// how the captions are made, see Look. It makes the change ShapeCaptionTime shows
// while the hand moves, see moveCaptionEdge, and says what it did to the
// words, for the app's log.
func SetCaptionTime(fsys afero.Fs, planPath, clipID string, word float64, edge string, at float64,
	toWords bool, t *Transcript, look *Look) (CaptionEdit, error) {
	moved, err := moveCaptionEdge(fsys, planPath, clipID, word, edge, at, toWords, t, look)
	if err != nil {
		return CaptionEdit{}, err
	}
	key := moved.key
	err = editPlan(fsys, planPath, func(_ *object, clips []*object) error {
		c, err := findClip(clips, clipID)
		if err != nil {
			return err
		}
		times, _ := c.values[keyCaptionTimes].(*object)
		if times == nil {
			times = newObject()
		}
		// The edges kept from before, by where their words start on the
		// sound, are kept by their words' keys from now on.
		for old, own := range moved.rekeyed {
			was, _ := times.values[old].(*object)
			if was == nil {
				continue
			}
			now, _ := times.values[own].(*object)
			if now == nil {
				now = newObject()
			}
			for _, name := range was.keys {
				if _, set := now.values[name]; !set {
					now.set(name, was.values[name])
				}
			}
			times.remove(old)
			times.set(own, now)
		}
		edges, _ := times.values[key].(*object)
		if edges == nil {
			edges = newObject()
		}
		if math.IsNaN(moved.at) {
			edges.remove(edge)
		} else {
			edges.set(edge, moved.at)
		}
		if len(edges.keys) == 0 {
			times.remove(key)
		} else {
			times.set(key, edges)
		}
		if len(times.keys) == 0 {
			c.remove(keyCaptionTimes)
		} else {
			c.set(keyCaptionTimes, times)
		}
		return nil
	})
	if err != nil {
		return CaptionEdit{}, err
	}
	return captionEdit(moved.was, look.drawing(moved.plan, moved.clip, t).Laid), nil
}

// captionEdit is what became of the words caption was showing, among the
// captions laid now.
func captionEdit(was LaidCaption, laid []LaidCaption) CaptionEdit {
	i := slices.IndexFunc(laid, func(c LaidCaption) bool { return c.Opens == was.Opens })
	if i < 0 {
		return CaptionEdit{Out: len(was.Words)}
	}
	shown := func(c LaidCaption) map[float64]bool {
		at := make(map[float64]bool, len(c.Words))
		for _, w := range c.Words {
			at[w.Start] = true
		}
		return at
	}
	before, now := shown(was), shown(laid[i])
	var done CaptionEdit
	for at := range before {
		if !now[at] {
			done.Out++
		}
	}
	for at := range now {
		if !before[at] {
			done.Back++
		}
	}
	return done
}

// captionEdge is what moving an edge of a caption makes of a clip.
type captionEdge struct {
	plan Plan
	// clip has the edge where it is moved to, in its caption times.
	clip Clip
	// at is where the edge is kept, on the episode's clock, or not a number
	// when it is where its words put it.
	at float64
	// key is what the edge is kept by in the clip's caption times, the
	// word's own, see saidKey, and rekeyed the keys from before it keeps
	// anew, each by the key it is kept by now.
	key     string
	rekeyed map[string]string
	// was is the caption as it was laid out before the edge moved.
	was LaidCaption
}

// startFloor is the earliest the start of caption c may be moved to, on
// the clip's clock: the end of the caption before. Where the two meet,
// because the speech runs on, it may go back to the middle of the last
// word the caption before shows, where an edge passes a word, and the
// caption before then goes where this one appears. A word is heard in
// running speech up to about one of the recogniser's 80 ms steps off, and
// Tim heard a word start earlier than its block, which stopped at the
// caption before, there the start itself, and could not be moved there,
// #597. After a pause the caption before keeps its end, as #189 has it.
func startFloor(before, c LaidCaption) float64 {
	if before.End < c.Start-0.0005 || len(before.Words) == 0 {
		return before.End
	}
	last := before.Words[len(before.Words)-1]
	return min(before.End, max((last.Start+last.End)/2, before.Start+shortestMoved))
}

// moveCaptionEdge works out what moving the edge of a caption does, the
// one thing both ShapeCaptionTime and SetCaptionTime ask, so what is drawn
// while the hand moves is what is saved when it lets go. It moves when the
// caption is shown and nothing else: the words the edge passes are not
// shown while it stands past them, see moveCaptions, and stay in the clip,
// so moving the edge back shows them again. A word removed in the caption
// box is a word fixed, like a typo, and no edge brings it back. Tim
// removed a word, dragged the start of the caption after it out to where
// he heard its next word begin, and the word came back, #598. A drag used
// to remove the words it passed, and a drag back put back every removed
// word it passed, however it had been removed.
//
// An edge put back, at not a number, is where all its words put it, and
// shows every word at that edge the caption box did not remove.
//
// The edge stays where it is put, never past the caption beside it. Kept
// against the caption beside it as it is now, the edge never moves another
// caption, see moveCaptions.
//
// It is kept against the word rather than the caption, because captions
// break in other places when the face or the size changes, and a word stays
// what it is. The word is the one the caption was laid out from, a word
// removed among them, see Caption.Opens.
func moveCaptionEdge(fsys afero.Fs, planPath, clipID string, word float64, edge string,
	at float64, toWords bool, t *Transcript, look *Look) (captionEdge, error) {
	if edge != "start" && edge != "end" {
		return captionEdge{}, renderErr("a caption has a start and an end, not %s", runes.Scrub(edge, 20))
	}
	reset := math.IsNaN(at)
	if !reset && (math.IsInf(at, 0) || at < 0 || at > MaxEpisodeSeconds) {
		return captionEdge{}, renderErr("a caption cannot be moved to %s", num.Fixed(at, 3))
	}
	plan, clip, err := planClip(fsys, planPath, clipID)
	if err != nil {
		return captionEdge{}, err
	}
	// Only a caption of the clip that begins or ends on the word. A caption
	// is made of its words and of nothing else.
	key := wordKey(word)
	// The captions as they are drawn, the ones the hand took hold of, and
	// the clip they are drawn on, put on the video's frames. Its clock is
	// the one an edge is moved on, see TestAnEdgeIsMovedOnTheClockItIsDrawnOn.
	drawing := look.drawing(plan, clip, t)
	laid, framed := drawing.Laid, drawing.Clip
	i := slices.IndexFunc(laid, func(c LaidCaption) bool {
		anchor := c.Opens
		if edge == "end" {
			anchor = c.Closes
		}
		return len(c.Words) > 0 && wordKey(anchor) == key
	})
	if i < 0 {
		return captionEdge{}, renderErr("the clip has no caption that %ss on the word at %s", edge, HMS(word))
	}
	out := captionEdge{plan: plan, clip: clip, at: math.NaN(), was: laid[i]}
	if !reset {
		to := ClipTime(framed, at)
		if toWords {
			to = captionStop(framed, t, laid[i], edge, to)
			at = EpisodeTime(framed, to)
		}
		if edge == "start" && i > 0 {
			if floor := startFloor(laid[i-1], laid[i]); to < floor {
				at = EpisodeTime(framed, floor)
			}
		}
		if edge == "end" && i+1 < len(laid) && to > laid[i+1].Start {
			at = EpisodeTime(framed, laid[i+1].Start)
		}
		out.at = num.RoundTo(at, 3)
	}
	times := maps.Clone(clip.CaptionTimes)
	if times == nil {
		times = map[string]CaptionTime{}
	}
	// Kept by the word's own key, so a change of the silence level, which
	// moves where the word starts, leaves it on its word. The clip's edges
	// kept from before by where their words start are kept that way from
	// now on, see rekeyCaptionTimes.
	said := t.Of(clip)
	out.rekeyed = said.rekeyCaptionTimes(times)
	if own, ok := said.saidKey(word); ok {
		key = own
	} else if own, ok := out.rekeyed[key]; ok {
		key = own
	}
	out.key = key
	edges := times[key]
	var put *float64
	if !reset {
		put = &out.at
	}
	if edge == "start" {
		edges.Start = put
	} else {
		edges.End = put
	}
	if edges.Start == nil && edges.End == nil {
		delete(times, key)
	} else {
		times[key] = edges
	}
	out.clip.CaptionTimes = times
	return out, nil
}

// captionStop is where an edge of caption c lands, dragged with shift to
// hand on the clip's clock: where the nearest word starts, for a start, or
// ends, for an end, the way an edge of the clip lands on words. The words
// are the ones the captions are laid out from, those taken out among them,
// so a word an edge took out is a word to land on going back over it. A
// caption put on a word appears or goes where the word does, as one laid
// out from its words does, so it lands there and not on a frame. It goes
// only the way the hand goes, see toward. Tim dragged a caption's edge
// with shift and it moved a frame at a time, as it does without.
func captionStop(clip Clip, t *Transcript, c LaidCaption, edge string, hand float64) float64 {
	was := c.Start
	if edge == "end" {
		was = c.End
	}
	words, _ := clipWords(clip, t.clipCaptionWords(clip))
	length := clipLength(clip)
	landed, gap := was, math.Inf(1)
	for _, w := range words {
		if text, _ := removedText(w.Text); text == "" {
			continue
		}
		stop := math.Max(w.Start, 0)
		if edge == "end" {
			stop = math.Min(w.End, length)
		}
		if d := math.Abs(stop - hand); d < gap {
			landed, gap = stop, d
		}
	}
	return toward(was, hand, landed)
}

// SetThumbnail adds, moves or removes a thumbnail of a clip: a moment of
// the episode the render takes a picture of the short at. from is the
// thumbnail as it is and to where it goes, both in seconds of the episode.
// A from below nought adds one at to, and a to below nought removes the one
// at from. A moment has to be inside a piece the clip keeps, because a
// moment that was cut is not in the short.
func SetThumbnail(fsys afero.Fs, planPath, clipID string, from, to float64) error {
	adding, removing := from < 0, to < 0
	if adding && removing {
		return renderErr("a thumbnail has to be somewhere")
	}
	for _, at := range []float64{from, to} {
		if math.IsNaN(at) || math.IsInf(at, 0) || at > MaxEpisodeSeconds {
			return renderErr("a thumbnail cannot be at %s", num.Fixed(at, 3))
		}
	}
	ms := func(at float64) int64 { return int64(math.Round(at * 1000)) }
	return editPlan(fsys, planPath, func(_ *object, clips []*object) error {
		c, err := findClip(clips, clipID)
		if err != nil {
			return err
		}
		list, _ := c.values[keyThumbnails].([]any)
		var kept []any
		found := false
		for _, item := range list {
			if !found && !adding && ms(number(item)) == ms(from) {
				found = true
				continue
			}
			kept = append(kept, item)
		}
		if !adding && !found {
			return renderErr("the clip has no thumbnail at %s", HMS(from))
		}
		if !removing {
			var pieces []Segment
			for _, seg := range segmentObjects(c) {
				// A piece can be written as a timecode, the way LoadClips
				// reads it.
				start, err1 := ParseTime(seg.values[keyStart])
				end, err2 := ParseTime(seg.values[keyEnd])
				if err1 == nil && err2 == nil {
					pieces = append(pieces, Segment{Start: start, End: end})
				}
			}
			if !insidePieces(pieces, to) {
				return renderErr("%s is not in the clip", HMS(to))
			}
			for _, item := range kept {
				if ms(number(item)) == ms(to) {
					return renderErr("the clip already has a thumbnail at %s", HMS(to))
				}
			}
			if len(readThumbnails(kept, pieces)) >= MaxThumbnails {
				return renderErr("a clip can have %d thumbnails", MaxThumbnails)
			}
			kept = append(kept, num.RoundTo(to, 3))
		}
		sort.SliceStable(kept, func(a, b int) bool { return number(kept[a]) < number(kept[b]) })
		if len(kept) == 0 {
			c.remove(keyThumbnails)
		} else {
			c.set(keyThumbnails, kept)
		}
		return nil
	})
}

// ResetCaptionY puts the captions of a clip back where the caption style
// puts them.
func ResetCaptionY(fsys afero.Fs, planPath, clipID string) error {
	return editPlan(fsys, planPath, func(_ *object, clips []*object) error {
		c, err := findClip(clips, clipID)
		if err != nil {
			return err
		}
		c.remove(keyCaptionY)
		return nil
	})
}

// ClearCaptionY takes the hand-placed caption line off every clip of a
// plan, so they all follow the one place the app keeps. It answers with
// how many clips changed.
func ClearCaptionY(fsys afero.Fs, planPath string) (int, error) {
	changed := 0
	err := editPlan(fsys, planPath, func(_ *object, clips []*object) error {
		for _, c := range clips {
			if _, ok := c.get(keyCaptionY); ok {
				c.remove(keyCaptionY)
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

// ResetCrop brings back the automatic crop of a clip for the way the piece
// at a moment is turned, and removes every keyframe of that turn. As
// filmed, that is the crop of every piece, turned or not, and on a picture
// turned, the crop of the pieces turned alike. A clip framed before #396
// can have pieces with different automatic crops, one for each camera,
// and they all take the one of the piece framingOf names.
func ResetCrop(fsys afero.Fs, planPath, clipID string, at float64) error {
	return editPlan(fsys, planPath, func(_ *object, clips []*object) error {
		c, err := findClip(clips, clipID)
		if err != nil {
			return err
		}
		pieces := segmentObjects(c)
		target := segmentAt(pieces, at)
		if target < 0 {
			return renderErr("clip %s has no pieces", clipID)
		}
		spans := rotationsOf(c)
		turn := rotationOf(pieces[target], spans)
		marks := cropMarksOf(c)
		if ofTurn := marksTurned(marks, turn); len(ofTurn) > 0 {
			var kept []CropMark
			for i, m := range marks {
				if !ofTurn[i] {
					kept = append(kept, m)
				}
			}
			setCropMarks(c, kept)
		}
		upright, _ := framingOf(pieces, spans)
		auto := autoCrop(upright)
		for _, seg := range pieces {
			if turn != 0 {
				if rotationOf(seg, spans) == turn {
					seg.remove(keyCropXTurned)
				}
				continue
			}
			if auto == nil {
				seg.remove(keyCropX)
			} else {
				seg.set(keyCropX, auto)
			}
			seg.remove(keyCropXAuto)
		}
		return nil
	})
}

// IsPlanFile reports whether a path is named the way plan files are named,
// so a record that names a plan names one and not, say, the transcript
// beside it.
func IsPlanFile(path string) bool {
	return planNameRe.MatchString(filepath.Base(path))
}

// planNameRe is what a clip set is called: clips.json for a search of the
// whole episode, clips-<from>-<to>.json for a search of a window,
// clips-<from>-<to>-<pass>.json for a later search of it, see PassName,
// and HandPlanName for the clips made by hand.
var planNameRe = regexp.MustCompile(`^clips(-\d+-\d+(-\d+)?|-hand)?\.json$`)

// madeOver is the part of the episode a clip set was made over, which
// counts as one more search of that part, see SearchPasses. A search of a
// window was made over that window, and one without a window over the
// whole episode. The clips made by hand were made over no part of it,
// since no model read anything for them.
func madeOver(from, to float64, windowed bool, by string, duration float64) timed.Window {
	switch {
	case by == ByHand:
		return timed.Window{}
	case windowed:
		return timed.Window{Start: from, End: to}
	case duration > 0:
		return timed.Window{Start: 0, End: duration}
	}
	return timed.Window{Start: 0, End: math.Inf(1)}
}
