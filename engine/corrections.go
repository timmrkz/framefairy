package engine

import (
	"cmp"
	"fmt"
	"maps"
	"math"
	"slices"
	"sort"
	"strings"
	"unicode/utf8"

	"github.com/spf13/afero"

	"framefairy/engine/runes"
	"framefairy/engine/timed"
)

// Word corrections are kept with the clip they are typed in, in its plan
// entry, keyed by the millisecond the recogniser heard the word start, see
// Clip.Corrections and heardIndex. They change what that clip's captions show and never
// the stored transcript or another clip, so the model's prompts stay the
// same, the recogniser's own output is never lost, and a clip removed and
// made again starts from what was heard. They used to be kept for the
// whole episode in logs/corrections.json, and Tim chose to have them go
// with the clip. That file is no longer read.

func wordKey(start float64) string {
	return fmt.Sprintf("%d", int64(math.Round(start*1000)))
}

// maxCorrections is how many words one clip may have corrected, far more
// than a clip of any length says.
const maxCorrections = 5000

// readCorrections takes the corrections of a clip out of its plan entry,
// which is untrusted like the rest of it. A key that is not a millisecond
// of an episode and a text a word could not be corrected to are left out.
func readCorrections(raw any) map[string]string {
	fields, ok := raw.(map[string]any)
	if !ok || len(fields) == 0 {
		return nil
	}
	out := map[string]string{}
	// The first by when the word starts, so a plan with too many keeps the
	// same ones every time it is read.
	for _, key := range slices.SortedFunc(maps.Keys(fields), byMillisecond) {
		if len(out) == maxCorrections {
			break
		}
		value := fields[key]
		if _, _, ok := parseWordKey(key); !ok {
			continue
		}
		text, ok := value.(string)
		if !ok {
			continue
		}
		if clean, err := CleanWordText(text); err == nil && clean == text {
			out[key] = text
		}
	}
	if len(out) == 0 {
		return nil
	}
	return out
}

// byMillisecond orders keys of words, the millisecond each starts, by when
// they start, and keys that read as the same number by their text, so the
// order is the same every time.
func byMillisecond(a, b string) int {
	am, _, _ := parseWordKey(a)
	bm, _, _ := parseWordKey(b)
	return cmp.Or(cmp.Compare(am, bm), cmp.Compare(a, b))
}

// putCorrections keeps the corrections of a clip in its plan entry, and
// keeps no key for a clip with none.
func putCorrections(c *object, corrections map[string]string) {
	if len(corrections) == 0 {
		c.remove(keyCorrections)
		return
	}
	fixes := newObject()
	for _, key := range slices.SortedFunc(maps.Keys(corrections), byMillisecond) {
		fixes.set(key, corrections[key])
	}
	c.set(keyCorrections, fixes)
}

// wordMost is the most characters a corrected word may have, all of it
// when it reads as several words.
const wordMost = 60

// CleanWordText checks a corrected word.
func CleanWordText(text string) (string, error) {
	text = strings.Join(strings.Fields(runes.Scrub(text, 200)), " ")
	if text == "" {
		return "", renderErr("a word cannot be empty")
	}
	if utf8.RuneCountInString(text) > wordMost {
		return "", renderErr("a word can be at most %d characters", wordMost)
	}
	return text, nil
}

// SetWordText corrects one word of a clip of a plan, the word heard at a
// moment, typed in the caption box. The correction is kept with the clip,
// against the word the recogniser heard, and the clip's words are made
// with it, see Transcript.Of. Another clip over the same word still reads
// what was heard, and the correction goes when the clip goes.
//
// A word corrected to nothing is removed from the clip, see RemoveWord,
// and a word removed from the clip typed back in beside its neighbour
// comes back in it, see putBack.
func SetWordText(fsys afero.Fs, planPath, clipID string, at float64, text string, t *Transcript) error {
	if strings.TrimSpace(runes.Scrub(text, 200)) == "" {
		return RemoveWord(fsys, planPath, clipID, at, -1, t)
	}
	return editClipWords(fsys, planPath, clipID, t, func(corrections map[string]string, removed RemovedWords) error {
		// A copy, so the words read by everything else stay as they were.
		work := *t.Of(Clip{Corrections: corrections})
		return correctWord(corrections, removed, at, text, &work)
	})
}

// RemoveWord removes a word from a clip, the word heard at a moment, or
// one of the words a correction made of it, part counted from nought, or
// all of it with part below nought, see dropWords. It is the clip's, so
// the episode still says it, every other clip still shows it, and it goes
// when the clip goes. A word removed as it was heard keeps what it reads,
// so the captions keep the room it took, see Captions, and an undo or
// typing it back in beside its neighbour puts it back.
func RemoveWord(fsys afero.Fs, planPath, clipID string, at float64, part int, t *Transcript) error {
	return editClipWords(fsys, planPath, clipID, t, func(corrections map[string]string, removed RemovedWords) error {
		work := *t.Of(Clip{Corrections: corrections})
		heard, ok := work.HeardAt(at)
		if !ok {
			return renderErr("there is no word at %s", HMS(at))
		}
		if part >= max(len(strings.Fields(heard.Text)), 1) {
			return renderErr("the word at %s is not %d words", HMS(at), part+1)
		}
		// One of the words typed in beside another is taken out of what
		// the correction reads, the way it was typed in, so the words
		// after it close up again: "war noch" with "noch" taken out reads
		// "war". Removing it undoes the typing.
		if typed := strings.Fields(heard.Text); part >= 0 && len(typed) > 1 {
			kept := slices.Delete(slices.Clone(typed), part, part+1)
			return correctWord(corrections, removed, heard.Start, strings.Join(kept, " "), &work)
		}
		dropWords(removed, heard, nil, &work)
		return nil
	})
}

// dropWords takes words of a heard word out of a clip with the caption
// they are in: parts, the words a correction made of it, counted from
// nought, or all of it when parts is nil. Each keeps its room, the way a
// heard word removed does, so removing a caption changes no other: the
// words after them used to move up into the room they had left, from the
// caption after into the one before, issue #820. All of them is the heard
// word removed. t is the clip's words, and heard is one of its heard
// words.
func dropWords(removed RemovedWords, heard timed.Cue, parts map[int]bool, t *Transcript) {
	if removed.removesHeard(t, heard) {
		return
	}
	n := len(strings.Fields(heard.Text))
	if parts == nil || n <= 1 {
		removed.take(t, heard)
		return
	}
	for part := range parts {
		if part < n {
			removed.takePart(t, heard, part)
		}
	}
	if len(removed.partsOf(t, heard)) >= n {
		removed.take(t, heard)
	}
}

// editClipWords changes what a clip's words read and which it does not
// show, and writes both to its plan entry, nothing when the change fails
// or changes nothing. Both are read from the plan as it is under its lock,
// so two edits one after the other each see the other's. They are handed
// to the change kept by the keys of t's words, see rekeyCorrections, so a
// clip edited once keeps no key from before 2.254.
func editClipWords(fsys afero.Fs, planPath, clipID string, t *Transcript,
	change func(corrections map[string]string, removed RemovedWords) error) error {
	return editPlan(fsys, planPath, func(_ *object, clips []*object) error {
		c, err := findClip(clips, clipID)
		if err != nil {
			return err
		}
		corrections := t.rekeyCorrections(readCorrections(plainValue(c.values[keyCorrections])))
		removed := t.rekeyRemoved(readRemovedWords(c.values[keyRemovedWords]))
		hadCorrections, hadRemoved := maps.Clone(corrections), removed.clone()
		if err := change(corrections, removed); err != nil {
			return err
		}
		if maps.Equal(corrections, hadCorrections) && maps.Equal(removed, hadRemoved) {
			return errUnchanged
		}
		putCorrections(c, corrections)
		putRemovedWords(c, removed)
		return nil
	})
}

// putRemovedWords keeps the words a clip does not show in its plan entry,
// and keeps no key for a clip that shows every word.
func putRemovedWords(c *object, removed RemovedWords) {
	if len(removed) == 0 {
		c.remove(keyRemovedWords)
		return
	}
	c.set(keyRemovedWords, removed.list())
}

// correctWord keeps in corrections, a clip's, what the word heard at a
// moment says, the way SetWordText keeps it, and corrects t from them, so
// the next word is changed against this one. t is the clip's words, a copy
// of its own to change. removed is the words the clip does not show, and a
// word of them typed back in beside its neighbour comes back in it.
func correctWord(corrections map[string]string, removed RemovedWords, at float64, text string, t *Transcript) error {
	text, err := CleanWordText(text)
	if err != nil {
		return err
	}
	word, ok := t.HeardAt(at)
	if !ok {
		return renderErr("there is no word at %s", HMS(at))
	}
	i := sort.Search(len(t.HeardWords), func(i int) bool { return t.HeardWords[i].Start >= word.Start })
	// A word the clip removed whole and corrected again is shown again
	// with what it now reads.
	if removed.removesHeard(t, word) {
		removed.forget(t, word)
	}
	gone := func(j int) bool { return j >= 0 && j < len(t.HeardWords) && removed.removesHeard(t, t.HeardWords[j]) }
	for j, says := range putBack(t.HeardWords, i, text, gone) {
		if j != i {
			removed.forget(t, t.HeardWords[j])
		} else {
			keepParts(removed, t, t.HeardWords[i], says)
		}
		// A word that reads as the recogniser heard it again keeps no
		// correction, so a word put back is the word it was before.
		key := t.keyAt(j)
		if j < len(t.snapped) && says == strings.Join(strings.Fields(t.snapped[j].Text), " ") {
			delete(corrections, key)
		} else {
			corrections[key] = says
		}
	}
	t.Correct(corrections)
	return nil
}

// keepParts keeps the words a correction made of a heard word that the
// clip does not show where they are when the heard word is corrected to
// read says: a word typed into one of its words leaves the words before
// the change where they were and moves the words after it along. The one
// changed was shown, so it was not one of them. When says leaves only
// words the clip does not show, the heard word is removed.
func keepParts(removed RemovedWords, t *Transcript, heard timed.Cue, says string) {
	parts := removed.partsOf(t, heard)
	if len(parts) == 0 {
		return
	}
	was, now := strings.Fields(heard.Text), strings.Fields(says)
	same := 0
	for same < min(len(was), len(now)) && was[same] == now[same] {
		same++
	}
	tail := 0
	for tail < min(len(was), len(now))-same && was[len(was)-1-tail] == now[len(now)-1-tail] {
		tail++
	}
	key := t.keyOf(heard)
	removed.dropParts(key)
	// Where each removed word is in what the word reads now, worked out
	// before anything is decided: a word left alone that was removed
	// with its caption used to come back once the word beside it was
	// deleted.
	var kept []int
	for _, part := range parts {
		switch {
		case part < same:
		case part >= len(was)-tail:
			part += len(now) - len(was)
		default:
			continue
		}
		if part >= 0 && part < len(now) {
			kept = append(kept, part)
		}
	}
	if len(kept) >= len(now) {
		removed[key] = true
		return
	}
	if len(now) < 2 {
		return
	}
	for _, part := range kept {
		removed[partKey(key, part)] = true
	}
}

// putBack is what a correction of the i-th heard word changes, by heard
// word. A word the clip removed, removed(j) for the j-th heard word, is
// put back by typing it into the word beside it,
// "weil" made "weil ein", and that is the removed word coming back, so it
// goes back where it was heard, not into its neighbour: the words typed
// after what the word read go into the removed words that follow it, and
// the words typed before into the removed words before it. Without this
// the word took its time from its neighbour, and was lit while the
// neighbour was said and not while it was. Undo put it back where it was,
// and typing it back did not, though both are the same word in the same
// place.
//
// Each removed word takes one word, the nearest first. Words typed beyond
// the removed ones there are go to the farthest of them, which is the
// word they were typed beside.
func putBack(heard []timed.Cue, i int, text string, removed func(j int) bool) map[int]string {
	put := map[int]string{i: text}
	was, now := strings.Fields(heard[i].Text), strings.Fields(text)
	if len(was) == 0 || len(now) <= len(was) {
		return put
	}
	if slices.Equal(now[:len(was)], was) && removed(i+1) {
		extra := now[len(was):]
		var slots []int
		for j := i + 1; removed(j) && len(slots) < len(extra); j++ {
			slots = append(slots, j)
		}
		put[i] = strings.Join(was, " ")
		last := len(slots) - 1
		for k, j := range slots[:last] {
			put[j] = extra[k]
		}
		put[slots[last]] = strings.Join(extra[last:], " ")
		return put
	}
	if slices.Equal(now[len(now)-len(was):], was) && removed(i-1) {
		extra := now[:len(now)-len(was)]
		var slots []int
		for j := i - 1; removed(j) && len(slots) < len(extra); j-- {
			slots = append(slots, j)
		}
		put[i] = strings.Join(was, " ")
		// slots run back from the word, the nearest first.
		last := len(slots) - 1
		for k, j := range slots[:last] {
			put[j] = extra[len(extra)-1-k]
		}
		put[slots[last]] = strings.Join(extra[:len(extra)-last], " ")
	}
	return put
}

// RemoveCaption removes the caption of a clip that begins on a word, the
// one heard at first: every word in it is removed from the clip the way
// RemoveWord removes one, so each is still the word heard there, to put
// back on its own or with an undo. The words its edges hide go with it.
// A word that a correction made several is removed only where it is in
// this caption. What is left is laid out
// as it was: the caption goes and leaves its time empty, and the one
// before it goes when it did, see Captions.
func RemoveCaption(fsys afero.Fs, planPath, clipID string, first float64, t *Transcript, look *Look) error {
	plan, clip, err := planClip(fsys, planPath, clipID)
	if err != nil {
		return err
	}
	view := captionsView(plan, clip, t, look)
	var cue *CaptionView
	for i := range view.Captions {
		if math.Abs(view.Captions[i].First-first) < 1e-6 && len(view.Captions[i].Lines) > 0 {
			cue = &view.Captions[i]
			break
		}
	}
	if cue == nil {
		return renderErr("there is no caption at %s", HMS(first))
	}
	t = t.Of(clip)
	// The heard words of the caption, each with the part of it the
	// caption holds, or all of it. A word the captions draw in halves is
	// there twice, and is one word.
	var words []shownWord
	for _, line := range cue.Lines {
		for _, w := range line.Words {
			if w.Said == nil {
				continue
			}
			heard, ok := t.HeardAt(*w.Said)
			if !ok {
				continue
			}
			part := -1
			if w.Part != nil {
				part = *w.Part
			}
			words = append(words, shownWord{heard, part})
		}
	}
	if len(words) == 0 {
		return renderErr("the caption at %s has no words to remove", HMS(first))
	}
	// The words its edges hide, dragged in over them, are its words too.
	// Left in the clip, they came back as a caption of their own once it
	// was gone, and the caption before stayed up until it appeared. Those
	// are the words said that the clip plays before its first word shown
	// or after its last, each of the words a correction made of a heard
	// word among them: an edge dragged in over the last of them used to
	// leave it in the clip, issue #820. A word a cut leaves out is no word
	// of the clip's, and stays as it was.
	lo, hi := math.Inf(1), math.Inf(-1)
	shown := map[string]bool{}
	for i := range view.Captions {
		c := &view.Captions[i]
		for _, line := range c.Lines {
			for _, w := range line.Words {
				if w.Said == nil {
					continue
				}
				shown[wordKey(*w.Said)] = true
				if c == cue {
					lo, hi = min(lo, *w.Said), max(hi, *w.Said)
				}
			}
		}
	}
	// A word is the clip's when the clip plays any of it: a cut into the
	// start of a word still plays the rest, and the captions show it.
	played := func(w timed.Cue) bool {
		for _, s := range clip.Segments {
			if w.Start < s.End && w.End > s.Start {
				return true
			}
		}
		return false
	}
	for i, said := range t.Words {
		edge := said.Start >= cue.First-0.0015 && said.Start < lo-0.0005 || said.Start > hi+0.0005 && said.Start <= cue.Last+0.0015
		if !edge || shown[wordKey(said.Start)] {
			continue
		}
		heard, ok := t.HeardAt(said.Start)
		if !ok || !played(heard) {
			continue
		}
		part := -1
		if first, n := t.saidOf(heard); n > 1 {
			part = i - first
		}
		words = append(words, shownWord{heard, part})
	}
	return editClipWords(fsys, planPath, clipID, t, func(corrections map[string]string, removed RemovedWords) error {
		work := *t.Of(Clip{Corrections: corrections})
		dropShown(removed, words, &work)
		return nil
	})
}

// shownWord is a word a caption shows: the heard word it is made from,
// and which of the words a correction made of it it is, counted from
// nought, or all of it below nought.
type shownWord struct {
	heard timed.Cue
	part  int
}

// dropShown takes words a caption shows out of a clip, the way RemoveWord
// takes one, see dropWords. t is the clip's words.
func dropShown(removed RemovedWords, words []shownWord, t *Transcript) {
	var order []float64
	parts := map[float64]map[int]bool{}
	for _, w := range words {
		at := w.heard.Start
		had, seen := parts[at]
		if !seen {
			order = append(order, at)
		}
		switch {
		case w.part < 0 || (seen && had == nil):
			parts[at] = nil
		case !seen:
			parts[at] = map[int]bool{w.part: true}
		default:
			had[w.part] = true
		}
	}
	for _, at := range order {
		if heard, ok := t.HeardAt(at); ok {
			dropWords(removed, heard, parts[at], t)
		}
	}
}

// plainValue is an object of a plan as it is edited, which keeps its keys
// in order, as the map a plan is read into, so one reader reads both.
func plainValue(v any) any {
	o, ok := v.(*object)
	if !ok {
		return v
	}
	out := make(map[string]any, len(o.values))
	maps.Copy(out, o.values)
	return out
}
