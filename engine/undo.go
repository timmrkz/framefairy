package engine

import (
	"bytes"
	"encoding/json"
	"errors"
	"os"
	"path/filepath"
	"sort"
	"strings"
)

// ---------------------------------------------------------------------------
// Undo
//
// Everything a person does to an episode's clips lands in one of two kinds
// of file: its plans, and its word corrections. So an edit is remembered as
// those files before it and after it, and undoing it puts back what it
// changed.
//
// Only what it changed, never the whole file. A search writes its clips into
// a plan while the app edits the clips already in it, and putting back
// the whole plan from before an edit would take away every clip that landed
// since. So the two copies are compared, and what differs between them, a
// clip, a field of the plan, a corrected word, is put back one by one. If
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
	logsDir string
	files   map[string]fileState
}

// editableFiles are an episode's plans and its word corrections.
func editableFiles(logsDir string) []string {
	var paths []string
	for _, p := range PlanSummaries(logsDir) {
		paths = append(paths, p.Path)
	}
	return append(paths, correctionsPath(logsDir))
}

// TakeSnapshot reads the files an edit of the episode in logsDir can
// change.
func TakeSnapshot(logsDir string) *Snapshot {
	s := &Snapshot{logsDir: logsDir, files: map[string]fileState{}}
	for _, path := range editableFiles(logsDir) {
		s.files[path] = readState(path)
	}
	return s
}

func readState(path string) fileState {
	body, err := os.ReadFile(path)
	if err != nil {
		return fileState{}
	}
	return fileState{there: true, body: body}
}

// Change is what one edit did to an episode.
type Change struct {
	logsDir string
	// paths are the files it changed, in a fixed order, with what each
	// held before and after.
	paths  []string
	before map[string]fileState
	after  map[string]fileState
}

// Compare says what changed between two snapshots of one episode. Nothing
// changed is nil. A file in either is compared, so a plan an edit removed
// and a plan that appeared are both part of it.
func Compare(before, after *Snapshot) *Change {
	c := &Change{logsDir: before.logsDir, before: map[string]fileState{}, after: map[string]fileState{}}
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

// LeaveOutNewClips takes out of the change every clip that was not there
// before it, and every plan that was not there before it. An edit changes
// clips, it never makes them, so a clip that appears between the two
// snapshots of an edit is a search landing it at that moment. Kept in the
// change, it was the edit's, and undoing the edit took away a clip the
// person had never touched. What is left is nil when the edit itself
// changed nothing.
func (c *Change) LeaveOutNewClips() *Change {
	if c == nil {
		return nil
	}
	var kept []string
	for _, path := range c.paths {
		if path == correctionsPath(c.logsDir) {
			kept = append(kept, path)
			continue
		}
		was, now := c.before[path], c.after[path]
		if !was.there {
			// A plan that appeared: a search wrote its first clip.
			delete(c.before, path)
			delete(c.after, path)
			continue
		}
		if now.there {
			if trimmed, ok := withoutNewClips(was, now); ok {
				now = trimmed
				c.after[path] = now
			}
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

// withoutNewClips is the plan now with only the clips it had before.
func withoutNewClips(was, now fileState) (fileState, bool) {
	before, err := splitPlan(was)
	if err != nil || before == nil {
		return now, false
	}
	after, err := splitPlan(now)
	if err != nil || after == nil {
		return now, false
	}
	var list []any
	dropped := false
	for _, id := range after.ids {
		if before.clips[id] == nil {
			dropped = true
			continue
		}
		list = append(list, after.clips[id])
	}
	if !dropped {
		return now, false
	}
	if list == nil {
		list = []any{}
	}
	after.top.values["clips"] = list
	body, err := marshalNoEscape(after.top)
	if err != nil {
		return now, false
	}
	return fileState{there: true, body: body}, true
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
		if shown.ID == "" && path != correctionsPath(c.logsDir) {
			if ids := changedClips(from[path], to[path]); len(ids) > 0 {
				shown = ClipRef{Plan: path, ID: ids[0]}
			}
		}
	}
	return shown, nil
}

func (c *Change) applyOne(path string, from, to fileState, dryRun bool) error {
	if path == correctionsPath(c.logsDir) {
		return applyCorrections(path, from, to, dryRun)
	}
	return applyPlan(path, from, to, dryRun)
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
	list, _ := top.values["clips"].([]any)
	for i, item := range list {
		c, ok := item.(*object)
		if !ok {
			continue
		}
		fallback := twoDigits(i + 1)
		id := fallback
		if raw, ok := c.get("id"); ok {
			id = pyStr(raw)
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

// fieldsOf is a plan's own fields, less the clips and the count of edits,
// which every edit moves on.
func fieldsOf(p *planParts) map[string]any {
	out := map[string]any{}
	if p == nil {
		return out
	}
	for _, k := range p.top.keys {
		if k != "clips" && k != "revision" {
			out[k] = p.top.values[k]
		}
	}
	return out
}

func applyPlan(path string, from, to fileState, dryRun bool) error {
	defer lockPlan(path)()
	src, err := splitPlan(from)
	if err != nil {
		return err
	}
	dst, err := splitPlan(to)
	if err != nil {
		return err
	}
	cur, err := splitPlan(readState(path))
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
			dropCaptionFiles(path, src.ids, src)
			if err := os.Remove(path); err != nil && !os.IsNotExist(err) {
				return err
			}
			return nil
		}
		if err := replacePlan(path, to.body); err != nil {
			return err
		}
		dropCaptionFiles(path, dst.ids, dst)
		return nil
	}

	fromFields, toFields, curFields := fieldsOf(src), fieldsOf(dst), fieldsOf(cur)
	if cur == nil {
		return ErrChangedSince
	}
	var fields []string
	for _, k := range append(append([]string(nil), src.top.keys...), dst.top.keys...) {
		if k == "clips" || k == "revision" {
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

	err = editPlanLocked(path, func(top *object, _ []*object) error {
		for _, k := range fields {
			if v, ok := toFields[k]; ok {
				top.set(k, v)
			} else {
				top.remove(k)
			}
		}
		list, _ := top.values["clips"].([]any)
		for _, id := range ids {
			list = putClip(list, id, dst)
		}
		top.set("clips", list)
		return nil
	})
	if err != nil {
		return err
	}
	dropCaptionFiles(path, ids, src)
	recordRestored(path, ids, src, dst)
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
		if raw, ok := c.get("id"); ok {
			cid = pyStr(raw)
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

// dropCaptionFiles removes the caption files of clips an undo changed, as
// any edit does, so the next render builds them from the plan again.
func dropCaptionFiles(planPath string, ids []string, parts *planParts) {
	if parts == nil {
		return
	}
	dir := filepath.Join(filepath.Dir(filepath.Dir(planPath)), "captions")
	for _, id := range ids {
		c := parts.clips[id]
		if c == nil {
			continue
		}
		slug := ""
		if raw, ok := c.get("slug"); ok {
			slug = pyStr(raw)
		}
		clip := Clip{ID: id, Slug: SanitiseName(slug, "")}
		for _, ext := range []string{".srt", ".ass"} {
			if path, err := SafeChild(dir, clip.Basename()+ext); err == nil {
				_ = os.Remove(path)
			}
		}
	}
}

// recordRestored records what an undo did to a clip the way the edit it
// undoes was recorded, so the training records end on the clip as it is.
// A clip taken out or put back is a decision, a clip trimmed or cut is an
// edit, and anything else was never recorded to begin with.
func recordRestored(planPath string, ids []string, from, to *planParts) {
	for _, id := range ids {
		was, now := from.clips[id], to.clips[id]
		if now == nil {
			continue
		}
		rejected := func(c *object) bool {
			if c == nil {
				return false
			}
			v, ok := c.get("rejected")
			return ok && v == true
		}
		segments := func(c *object) any {
			if c == nil {
				return nil
			}
			v, _ := c.get("segments")
			return v
		}
		switch {
		case rejected(was) != rejected(now):
			event := DecisionKept
			if rejected(now) {
				event = DecisionRejected
			}
			_ = RecordDecision(planPath, id, event, nil)
		case !equalJSON(segments(was), segments(now)):
			_ = RecordDecision(planPath, id, DecisionEdited, nil)
		}
	}
}

// applyCorrections puts back the corrected words an edit changed, word by
// word.
func applyCorrections(path string, from, to fileState, dryRun bool) error {
	words := func(state fileState) (map[string]string, error) {
		if !state.there {
			return map[string]string{}, nil
		}
		var file correctionsFile
		if err := json.Unmarshal(state.body, &file); err != nil {
			return nil, err
		}
		if file.Words == nil {
			file.Words = map[string]string{}
		}
		return file.Words, nil
	}
	trainingMu.Lock()
	defer trainingMu.Unlock()
	src, err := words(from)
	if err != nil {
		return err
	}
	dst, err := words(to)
	if err != nil {
		return err
	}
	cur, err := words(readState(path))
	if err != nil {
		return err
	}
	var keys []string
	for _, m := range []map[string]string{src, dst} {
		for k := range m {
			a, okA := src[k]
			b, okB := dst[k]
			if (okA != okB || a != b) && !contains(keys, k) {
				keys = append(keys, k)
			}
		}
	}
	for _, k := range keys {
		a, okA := src[k]
		c, okC := cur[k]
		if okA != okC || a != c {
			return ErrChangedSince
		}
	}
	if dryRun || len(keys) == 0 {
		return nil
	}
	for _, k := range keys {
		if b, ok := dst[k]; ok {
			cur[k] = b
		} else {
			delete(cur, k)
		}
	}
	body, err := marshalNoEscape(correctionsFile{Version: 1, Words: cur})
	if err != nil {
		return err
	}
	return writeAtomic(path, append(body, '\n'))
}
