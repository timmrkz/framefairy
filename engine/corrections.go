package engine

import (
	"cmp"
	"fmt"
	"maps"
	"math"
	"slices"
	"sort"
	"strconv"
	"strings"
	"unicode/utf8"

	"github.com/spf13/afero"
)

// Word corrections are kept with the clip they are typed in, in its plan
// entry, keyed by the millisecond the word heard starts, see
// Clip.Corrections. They change what that clip's captions show and never
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
		ms, err := strconv.ParseInt(key, 10, 64)
		if err != nil || ms < 0 || ms > MaxEpisodeSeconds*1000 || strconv.FormatInt(ms, 10) != key {
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
	am, _ := strconv.ParseInt(a, 10, 64)
	bm, _ := strconv.ParseInt(b, 10, 64)
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

// CleanWordText checks a corrected word.
func CleanWordText(text string) (string, error) {
	text = strings.Join(strings.Fields(Scrub(text, 200)), " ")
	if text == "" {
		return "", renderErr("a word cannot be empty")
	}
	if utf8.RuneCountInString(text) > 60 {
		return "", renderErr("a word can be at most 60 characters")
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
	if strings.TrimSpace(Scrub(text, 200)) == "" {
		return RemoveWord(fsys, planPath, clipID, at, -1, t)
	}
	return editClipWords(fsys, planPath, clipID, func(corrections map[string]string, removed RemovedWords) error {
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
	return editClipWords(fsys, planPath, clipID, func(corrections map[string]string, removed RemovedWords) error {
		work := *t.Of(Clip{Corrections: corrections})
		heard, ok := work.HeardAt(at)
		if !ok {
			return renderErr("there is no word at %s", HMS(at))
		}
		if part >= max(len(strings.Fields(heard.Text)), 1) {
			return renderErr("the word at %s is not %d words", HMS(at), part+1)
		}
		var parts map[int]bool
		if part >= 0 {
			parts = map[int]bool{part: true}
		}
		return dropWords(corrections, removed, heard, parts, &work)
	})
}

// dropWords takes words of a heard word out of a clip: parts, the words a
// correction made of it, counted from nought, or all of it when parts is
// nil. Some of the words a correction made are taken out of what the
// correction reads, the way they were typed in, so the words after them
// close up: "war noch" with "noch" taken out reads "war" again. All of
// them, or a word as it was heard, is the heard word removed from the
// clip, which keeps its room. t is the clip's words, a copy of its own to
// change, and heard is one of its heard words.
func dropWords(corrections map[string]string, removed RemovedWords, heard Cue, parts map[int]bool,
	t *Transcript) error {
	if removed.removesHeard(heard) {
		return nil
	}
	var kept []string
	if parts != nil {
		for i, word := range strings.Fields(heard.Text) {
			if !parts[i] {
				kept = append(kept, word)
			}
		}
	}
	if len(kept) == 0 {
		removed.take(heard)
		return nil
	}
	return correctWord(corrections, removed, heard.Start, strings.Join(kept, " "), t)
}

// editClipWords changes what a clip's words read and which it does not
// show, and writes both to its plan entry, nothing when the change fails
// or changes nothing. Both are read from the plan as it is under its lock,
// so two edits one after the other each see the other's.
func editClipWords(fsys afero.Fs, planPath, clipID string,
	change func(corrections map[string]string, removed RemovedWords) error) error {
	return editPlan(fsys, planPath, func(_ *object, clips []*object) error {
		c, err := findClip(clips, clipID)
		if err != nil {
			return err
		}
		corrections := maps.Clone(readCorrections(plainValue(c.values[keyCorrections])))
		if corrections == nil {
			corrections = map[string]string{}
		}
		removed := readRemovedWords(c.values[keyRemovedWords]).clone()
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
	if removed.removesHeard(word) {
		removed.forget(word)
	}
	gone := func(j int) bool { return j >= 0 && j < len(t.HeardWords) && removed.removesHeard(t.HeardWords[j]) }
	for j, says := range putBack(t.HeardWords, i, text, gone) {
		if j != i {
			removed.forget(t.HeardWords[j])
		}
		// A word that reads as the recogniser heard it again keeps no
		// correction, so a word put back is the word it was before.
		key := wordKey(t.HeardWords[j].Start)
		if j < len(t.snapped) && says == strings.Join(strings.Fields(t.snapped[j].Text), " ") {
			delete(corrections, key)
		} else {
			corrections[key] = says
		}
	}
	t.Correct(corrections)
	return nil
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
func putBack(heard []Cue, i int, text string, removed func(j int) bool) map[int]string {
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
// back on its own or with an undo. A word that a correction made several
// is removed only where it is in this caption. What is left is laid out
// as it was: the caption goes and leaves its time empty, and the one
// before it goes when it did, see Captions.
func RemoveCaption(fsys afero.Fs, planPath, clipID string, first float64, t *Transcript, look *Look) error {
	view, err := ClipCaptionsView(fsys, planPath, clipID, t, look)
	if err != nil {
		return err
	}
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
	_, clip, err := planClip(fsys, planPath, clipID)
	if err != nil {
		return err
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
	return editClipWords(fsys, planPath, clipID, func(corrections map[string]string, removed RemovedWords) error {
		work := *t.Of(Clip{Corrections: corrections})
		return dropShown(corrections, removed, words, &work)
	})
}

// shownWord is a word a caption shows: the heard word it is made from,
// and which of the words a correction made of it it is, counted from
// nought, or all of it below nought.
type shownWord struct {
	heard Cue
	part  int
}

// dropShown takes words a caption shows out of a clip, the way RemoveWord
// takes one, see dropWords. t is the clip's words, a copy of its own to
// change.
func dropShown(corrections map[string]string, removed RemovedWords, words []shownWord, t *Transcript) error {
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
		// The heard word as it reads now, since taking out the words of
		// one changes no other.
		heard, ok := t.HeardAt(at)
		if !ok {
			continue
		}
		if err := dropWords(corrections, removed, heard, parts[at], t); err != nil {
			return err
		}
	}
	return nil
}

// trimCaption is what dragging an edge of a caption across its words does
// to them. Dragged in, the words the edge passes leave the caption,
// removed from the clip the way RemoveWord removes one, so pulling the
// left edge of "one two three" past "one" reads "two three", and the right
// edge past "three" reads "one two". Dragged back out, the words of the
// caption removed before that it passes come back. A word is passed when
// the edge crosses its middle, on the clip's clock, which is where the
// word is drawn: a word the start of the clip cuts into is passed in the
// part of it the clip shows. A caption keeps at least one word this way,
// because delete removes a caption whole.
//
// to is where the edge is, on the clip's clock. The caption is the one
// laid out now, with the edge where it stands. The words are changed in
// corrections and removed, the clip's, and nothing is written. It says
// how many words it took out and put back.
func trimCaption(corrections map[string]string, removed RemovedWords, clip Clip, c LaidCaption, edge string,
	to float64, t *Transcript) (CaptionEdit, error) {
	if len(clip.Segments) == 0 {
		return CaptionEdit{}, nil
	}
	t = t.Of(clip)
	from := c.Start
	if edge == "end" {
		from = c.End
	}
	low, high := math.Min(from, to), math.Max(from, to)
	in := (edge == "start") == (to > from)
	if !in && len(c.Words) > 0 {
		// An edge whose word was taken out may stand past the middle of
		// that word already. Out from there, the words past the words
		// shown are what it crosses.
		if edge == "end" {
			low = math.Min(low, c.Words[len(c.Words)-1].End)
		} else {
			high = math.Max(high, c.Words[0].Start)
		}
	}
	// The words the caption was laid out from, the removed ones among them,
	// on the clip's clock and on the episode's.
	all, was := clipWords(clip, t.clipCaptionWords(clip))
	length := clipLength(clip)
	var gone, back []Cue
	shown := 0
	for k, w := range was {
		if w.Start < c.Opens-0.0005 || w.Start > c.Closes+0.0005 {
			continue
		}
		text, isRemoved := removedText(w.Text)
		if !isRemoved {
			shown++
		}
		mid := shownMiddle(all[k], length)
		if mid <= low || mid >= high {
			continue
		}
		switch {
		case in && !isRemoved:
			gone = append(gone, w)
		case !in && isRemoved && text != "":
			back = append(back, w)
		}
	}
	if len(gone) > 0 && len(gone) == shown {
		// The word at the other edge stays.
		if edge == "end" {
			gone = gone[1:]
		} else {
			gone = gone[:len(gone)-1]
		}
	}
	for _, w := range back {
		if heard, ok := t.HeardAt(w.Start); ok {
			removed.forget(heard)
		}
	}
	var words []shownWord
	for _, w := range gone {
		heard, ok := t.HeardAt(w.Start)
		if !ok {
			continue
		}
		part := -1
		if len(strings.Fields(heard.Text)) > 1 {
			part = t.PartOf(heard, w)
		}
		words = append(words, shownWord{heard, part})
	}
	work := *t
	if err := dropShown(corrections, removed, words, &work); err != nil {
		return CaptionEdit{}, err
	}
	return CaptionEdit{Removed: len(gone), Back: len(back)}, nil
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
