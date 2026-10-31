package engine

import (
	"bytes"
	"errors"
	"os"
	"sort"
	"strings"

	"github.com/spf13/afero"

	"framefairy/engine/fileio"
	"framefairy/engine/num"
)

// ---------------------------------------------------------------------------
// Undo
//
// Everything a person does to an episode's clips lands in its plans, the
// words a clip corrects and removes included. So an edit is remembered as
// those files before it and after it, and undoing it puts back what it
// changed.
//
// Only what it changed, never the whole file. A search writes its clips into
// a plan while the app edits the clips already in it, and putting back
// the whole plan from before an edit would take away every clip that landed
// since. So the two copies are compared, and what differs between them, a
// clip or a field of the plan, is put back one by one. If
// any of those has changed again since, by another edit or by a search, it
// is left alone and the undo says so rather than writing over it.
// ---------------------------------------------------------------------------

// ErrChangedSince is an undo or a redo that would write over something that
// changed after the edit it belongs to.
var ErrChangedSince = errors.New("what this would put back has changed since")

type fileState struct {
	there bool
	body  []byte
}

// Snapshot is the files an edit of an episode can change, as they were.
type Snapshot struct {
	fsys    afero.Fs
	records *Records
	logsDir string
	files   map[string]fileState
}

// editableFiles are an episode's plans.
func editableFiles(fsys afero.Fs, logsDir string) []string {
	var paths []string
	for _, p := range PlanSummaries(fsys, logsDir) {
		paths = append(paths, p.Path)
	}
	return paths
}

// TakeSnapshot reads the files an edit of the episode in logsDir can
// change. An undo of the change it is the start of is recorded in rec, the
// way the edit was.
func TakeSnapshot(fsys afero.Fs, rec *Records, logsDir string) *Snapshot {
	s := &Snapshot{fsys: fsys, records: rec, logsDir: logsDir, files: map[string]fileState{}}
	for _, path := range editableFiles(fsys, logsDir) {
		s.files[path] = readState(fsys, path)
	}
	return s
}

func readState(fsys afero.Fs, path string) fileState {
	body, err := afero.ReadFile(fsys, path)
	if err != nil {
		return fileState{}
	}
	return fileState{there: true, body: body}
}

// Change is what one edit did to an episode.
type Change struct {
	fsys    afero.Fs
	records *Records
	logsDir string
	// paths are the files it changed, in a fixed order, with what each
	// held before and after.
	paths  []string
	before map[string]fileState
	after  map[string]fileState
}

// ChangeBetween says what changed between two snapshots of one episode.
// Nothing changed is nil. A file in either is compared, so a plan an edit
// removed and a plan that appeared are both part of it.
func ChangeBetween(before, after *Snapshot) *Change {
	c := &Change{fsys: before.fsys, records: before.records, logsDir: before.logsDir, before: map[string]fileState{}, after: map[string]fileState{}}
	seen := map[string]bool{}
	for _, snap := range []*Snapshot{before, after} {
		for path := range snap.files {
			if seen[path] {
				continue
			}
			seen[path] = true
			was, now := before.files[path], after.files[path]
			if was.there == now.there && bytes.Equal(was.body, now.body) {
				continue
			}
			c.paths = append(c.paths, path)
			c.before[path], c.after[path] = was, now
		}
	}
	if len(c.paths) == 0 {
		return nil
	}
	sort.Strings(c.paths)
	return c
}

// LeaveOutNewClips takes out of the change what a search did while the
// edit was made, between its two snapshots, and keeps what the edit did.
// An edit changes clips. It never makes them, never takes one out of a
// plan and never writes a plan anew, and a search does all three: it
// lands a clip, it drops a removed clip it has made again, see
// DropRemoved, and over a window searched before it writes its plan in
// place of the one there was. So a clip that appears is the search's, a
// clip that goes is the search's, and so is a plan that appeared or was
// written anew, with another plan id. Kept in the change, undoing the edit
// took away a clip the person had never touched, or put back clips the
// search had replaced, issue #309. What is left is nil when the edit
// itself changed nothing.
func (c *Change) LeaveOutNewClips() *Change {
	if c == nil {
		return nil
	}
	var kept []string
	for _, path := range c.paths {
		was, now := c.before[path], c.after[path]
		if !was.there || writtenAnew(was, now) {
			// A plan that appeared, a search's first clip, or a plan a
			// search wrote in place of the one there was.
			delete(c.before, path)
			delete(c.after, path)
			continue
		}
		if now.there {
			was, now = clipsOfBoth(was, now)
			c.before[path], c.after[path] = was, now
			if sameState(was, now) {
				delete(c.before, path)
				delete(c.after, path)
				continue
			}
		}
		kept = append(kept, path)
	}
	c.paths = kept
	if len(kept) == 0 {
		return nil
	}
	return c
}

// writtenAnew says whether a plan there before and after is another plan
// after, by its id: a search wrote it in place of the one there was.
func writtenAnew(was, now fileState) bool {
	before, errA := splitPlan(was)
	after, errB := splitPlan(now)
	if errA != nil || errB != nil || before == nil || after == nil {
		return false
	}
	a, _ := before.top.get(keyPlanID)
	b, _ := after.top.get(keyPlanID)
	return !equalJSON(a, b)
}

// clipsOfBoth is the plan before and after with only the clips it holds in
// both: what the edit did to them, and none of what appeared or went.
func clipsOfBoth(was, now fileState) (fileState, fileState) {
	before, errA := splitPlan(was)
	after, errB := splitPlan(now)
	if errA != nil || errB != nil || before == nil || after == nil {
		return was, now
	}
	return onlyClipsOf(was, before, after), onlyClipsOf(now, after, before)
}

// onlyClipsOf is state, split as parts, with only the clips other holds
// too. It is state as it is when nothing has to go.
func onlyClipsOf(state fileState, parts, other *planParts) fileState {
	list := []any{}
	dropped := false
	for _, id := range parts.ids {
		if other.clips[id] == nil {
			dropped = true
			continue
		}
		list = append(list, parts.clips[id])
	}
	if !dropped {
		return state
	}
	parts.top.values[keyClips] = list
	body, err := marshalNoEscape(parts.top)
	if err != nil {
		return state
	}
	return fileState{there: true, body: body}
}

// sameState says whether two states of a plan hold the same fields and the
// same clips, the count of edits aside.
func sameState(a, b fileState) bool {
	x, errA := splitPlan(a)
	y, errB := splitPlan(b)
	if errA != nil || errB != nil || x == nil || y == nil {
		return false
	}
	return samePlan(x, y) && strings.Join(x.ids, "\n") == strings.Join(y.ids, "\n")
}

// Files are the files the edit changed.
func (c *Change) Files() []string { return append([]string(nil), c.paths...) }

// Undo puts back what the edit changed.
func (c *Change) Undo() (ClipRef, error) { return c.apply(c.after, c.before) }

// Redo does the edit again.
func (c *Change) Redo() (ClipRef, error) { return c.apply(c.before, c.after) }

// ClipRef names a clip of a plan: the one an undo or a redo changed, so the
// app can show it.
type ClipRef struct {
	Plan string `json:"plan"`
	ID   string `json:"id"`
}

// apply turns what is there from the state from into the state to, piece
// by piece. Everything is checked before anything is written, so an undo
// that cannot be done leaves every file as it was.
func (c *Change) apply(from, to map[string]fileState) (ClipRef, error) {
	for _, path := range c.paths {
		if err := c.applyOne(path, from[path], to[path], true); err != nil {
			return ClipRef{}, err
		}
	}
	var shown ClipRef
	for _, path := range c.paths {
		if err := c.applyOne(path, from[path], to[path], false); err != nil {
			return shown, err
		}
		if shown.ID == "" {
			if ids := changedClips(from[path], to[path]); len(ids) > 0 {
				shown = ClipRef{Plan: path, ID: ids[0]}
			}
		}
	}
	return shown, nil
}

func (c *Change) applyOne(path string, from, to fileState, dryRun bool) error {
	return applyPlan(c.fsys, c.records, path, from, to, dryRun)
}

// equalJSON says whether two decoded values are the same, written out.
func equalJSON(a, b any) bool {
	x, errA := marshalNoEscape(a)
	y, errB := marshalNoEscape(b)
	return errA == nil && errB == nil && bytes.Equal(x, y)
}

// planParts is a plan taken apart: its fields other than the clips, and
// its clips by id, in order.
type planParts struct {
	top   *object
	ids   []string
	clips map[string]*object
}

func splitPlan(state fileState) (*planParts, error) {
	if !state.there {
		return nil, nil
	}
	value, err := decodeOrdered(state.body)
	if err != nil {
		return nil, err
	}
	top, ok := value.(*object)
	if !ok {
		return nil, renderErr("clip plan must be a JSON object")
	}
	parts := &planParts{top: top, clips: map[string]*object{}}
	list, _ := top.values[keyClips].([]any)
	for i, item := range list {
		c, ok := item.(*object)
		if !ok {
			continue
		}
		fallback := twoDigits(i + 1)
		id := fallback
		if raw, ok := c.get(keyID); ok {
			id = num.FormatValue(raw)
		}
		id = SanitiseName(id, fallback)
		parts.ids = append(parts.ids, id)
		parts.clips[id] = c
	}
	return parts, nil
}

// changedClips are the ids of the clips that differ between two states of
// one plan, in the order they come.
func changedClips(from, to fileState) []string {
	a, errA := splitPlan(from)
	b, errB := splitPlan(to)
	if errA != nil || errB != nil {
		return nil
	}
	var ids []string
	seen := map[string]bool{}
	for _, parts := range []*planParts{b, a} {
		if parts == nil {
			continue
		}
		for _, id := range parts.ids {
			if seen[id] {
				continue
			}
			seen[id] = true
			var x, y any
			if a != nil && a.clips[id] != nil {
				x = a.clips[id]
			}
			if b != nil && b.clips[id] != nil {
				y = b.clips[id]
			}
			if !equalJSON(x, y) {
				ids = append(ids, id)
			}
		}
	}
	return ids
}

// fieldsOf is a plan's own fields, less the clips, the count of edits,
// which every edit moves on, and where the shorts went, which a render
// writes and no edit does, so an undo never takes it back or stops at it.
func fieldsOf(p *planParts) map[string]any {
	out := map[string]any{}
	if p == nil {
		return out
	}
	for _, k := range p.top.keys {
		if k != keyClips && k != keyRevision && k != keyShorts {
			out[k] = p.top.values[k]
		}
	}
	return out
}

func applyPlan(fsys afero.Fs, rec *Records, path string, from, to fileState, dryRun bool) error {
	defer fileio.Lock(fsys, path)()
	src, err := splitPlan(from)
	if err != nil {
		return err
	}
	dst, err := splitPlan(to)
	if err != nil {
		return err
	}
	cur, err := splitPlan(readState(fsys, path))
	if err != nil {
		return err
	}

	// The plan itself comes or goes: a search removed whole, and put back.
	// Only when nothing has happened to it since, clip for clip.
	if src == nil || dst == nil {
		if (cur == nil) != (src == nil) {
			return ErrChangedSince
		}
		if cur != nil && !samePlan(cur, src) {
			return ErrChangedSince
		}
		if dryRun {
			return nil
		}
		if dst == nil {
			if err := fsys.Remove(path); err != nil && !os.IsNotExist(err) {
				return err
			}
			return nil
		}
		if err := replacePlan(fsys, path, to.body); err != nil {
			return err
		}
		return nil
	}

	fromFields, toFields, curFields := fieldsOf(src), fieldsOf(dst), fieldsOf(cur)
	if cur == nil {
		return ErrChangedSince
	}
	var fields []string
	for _, k := range append(append([]string(nil), src.top.keys...), dst.top.keys...) {
		if k == keyClips || k == keyRevision {
			continue
		}
		if equalJSON(fromFields[k], toFields[k]) || contains(fields, k) {
			continue
		}
		if !equalJSON(curFields[k], fromFields[k]) {
			return ErrChangedSince
		}
		fields = append(fields, k)
	}
	ids := changedClips(from, to)
	for _, id := range ids {
		var was, now any
		if src.clips[id] != nil {
			was = src.clips[id]
		}
		if cur.clips[id] != nil {
			now = cur.clips[id]
		}
		if !equalJSON(was, now) {
			return ErrChangedSince
		}
	}
	if dryRun || (len(fields) == 0 && len(ids) == 0) {
		return nil
	}

	err = editPlanLocked(fsys, path, func(top *object, _ []*object) error {
		for _, k := range fields {
			if v, ok := toFields[k]; ok {
				top.set(k, v)
			} else {
				top.remove(k)
			}
		}
		list, _ := top.values[keyClips].([]any)
		for _, id := range ids {
			list = putClip(list, id, dst)
		}
		top.set(keyClips, list)
		return nil
	})
	if err != nil {
		return err
	}
	recordRestored(fsys, rec, path, ids, src, dst)
	return nil
}

func contains(list []string, s string) bool {
	for _, v := range list {
		if v == s {
			return true
		}
	}
	return false
}

// samePlan says whether a plan holds the same fields and clips as another,
// whatever the count of edits says.
func samePlan(a, b *planParts) bool {
	if !equalJSON(fieldsOf(a), fieldsOf(b)) || len(a.ids) != len(b.ids) {
		return false
	}
	for _, id := range a.ids {
		if !equalJSON(a.clips[id], b.clips[id]) {
			return false
		}
	}
	return true
}

// putClip makes the clip with this id in a plan's list what it is in to:
// replaced where it stands, taken out, or put back where it stood.
func putClip(list []any, id string, to *planParts) []any {
	at := -1
	for i, item := range list {
		c, ok := item.(*object)
		if !ok {
			continue
		}
		fallback := twoDigits(i + 1)
		cid := fallback
		if raw, ok := c.get(keyID); ok {
			cid = num.FormatValue(raw)
		}
		if SanitiseName(cid, fallback) == id {
			at = i
			break
		}
	}
	want := to.clips[id]
	switch {
	case want != nil && at >= 0:
		list[at] = want
	case want == nil && at >= 0:
		list = append(list[:at], list[at+1:]...)
	case want != nil:
		// Where it stood before, among the clips that are still there.
		place := len(list)
		for i, other := range to.ids {
			if other == id {
				place = min(i, len(list))
				break
			}
		}
		list = append(list, nil)
		copy(list[place+1:], list[place:])
		list[place] = want
	}
	return list
}

// recordRestored records what an undo did to a clip the way the edit it
// undoes was recorded, so the training records end on the clip as it is.
// A clip taken out or put back is a decision, a clip trimmed or cut is an
// edit, and anything else was never recorded to begin with.
func recordRestored(fsys afero.Fs, rec *Records, planPath string, ids []string, from, to *planParts) {
	for _, id := range ids {
		was, now := from.clips[id], to.clips[id]
		if now == nil {
			continue
		}
		rejected := func(c *object) bool {
			if c == nil {
				return false
			}
			v, ok := c.get(keyRejected)
			return ok && v == true
		}
		segments := func(c *object) any {
			if c == nil {
				return nil
			}
			v, _ := c.get(keySegments)
			return v
		}
		switch {
		case rejected(was) != rejected(now):
			event := DecisionKept
			if rejected(now) {
				event = DecisionRejected
			}
			_ = RecordDecision(fsys, rec, planPath, id, event, nil)
		case !equalJSON(segments(was), segments(now)):
			_ = RecordDecision(fsys, rec, planPath, id, DecisionEdited, nil)
		}
	}
}
