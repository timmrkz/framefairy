package engine

import (
	"cmp"
	"maps"
	"slices"
	"strconv"
	"strings"

	"framefairy/engine/timed"
)

// The words a clip does not show. Removing a word or a caption is done to
// a clip, the way a trim or a moved caption is, so it is kept with the
// clip, in its plan entry, and goes with it: a clip removed and made again
// shows every word it says, and another clip over the same words shows
// them too. It used to be kept for the whole video, as a correction to
// nothing, so every clip over the word lost it and the words said lost it,
// the prompt and the lines a clip is made from included. Tim removed a
// caption, removed its clip, made the clip again with I where it had been,
// and the caption was still gone.
//
// What a word reads is the clip's in the same way, see corrections.go.

// RemovedWords is the heard words a clip does not show, each kept by the
// millisecond the recogniser heard it start, as a correction is, see
// heardIndex. One of the words a correction made of a heard word, removed
// with its caption, is kept by the heard word's key, a slash and which of
// them it is, counted from nought, "h61200/1". The correction still reads
// it, so the captions keep the room it took, the way they do for a heard
// word removed, and removing the caption changes no other. Before, the
// words after it moved up into its room, from the caption after into the
// one before, issue #820. Removed alone in the caption box, it is taken
// out of what the correction reads instead, see RemoveWord.
type RemovedWords map[string]bool

// partKey is the key of one of the words a correction made of the heard
// word kept by key.
func partKey(key string, part int) string {
	return key + "/" + strconv.Itoa(part)
}

// splitRemoved is the key of the heard word a key of RemovedWords is about,
// and which of the words a correction made of it, or below nought for all
// of it. It is not ok for a key that is neither.
func splitRemoved(key string) (word string, part int, ok bool) {
	word, digits, isPart := strings.Cut(key, "/")
	if _, _, ok := parseWordKey(word); !ok {
		return "", 0, false
	}
	if !isPart {
		return word, -1, true
	}
	part, err := strconv.Atoi(digits)
	if err != nil || part < 0 || part >= wordMost || strconv.Itoa(part) != digits {
		return "", 0, false
	}
	return word, part, true
}

// byRemovedKey orders keys of RemovedWords by when their word starts, and
// the words a correction made of one in their order after it.
func byRemovedKey(a, b string) int {
	aw, ap, _ := splitRemoved(a)
	bw, bp, _ := splitRemoved(b)
	return cmp.Or(byMillisecond(aw, bw), cmp.Compare(ap, bp))
}

// maxRemovedWords is how many words one clip may keep removed, far more
// than a clip of any length says.
const maxRemovedWords = 5000

// readRemovedWords takes the words a clip does not show out of its plan
// entry, which is untrusted like the rest of it. A key that is not a
// millisecond of an episode, or one with a part that is no count of the
// words a correction may make, is left out.
func readRemovedWords(raw any) RemovedWords {
	list, ok := raw.([]any)
	if !ok || len(list) == 0 {
		return nil
	}
	out := RemovedWords{}
	for _, item := range list[:min(len(list), maxRemovedWords)] {
		key, ok := item.(string)
		if !ok {
			continue
		}
		if _, _, ok := splitRemoved(key); !ok {
			continue
		}
		out[key] = true
	}
	if len(out) == 0 {
		return nil
	}
	return out
}

// list is the keys in the order they are kept in a plan, by when the
// word starts, so a plan reads the same whatever order they were removed
// in.
func (r RemovedWords) list() []any {
	keys := slices.SortedFunc(maps.Keys(r), byRemovedKey)
	out := make([]any, len(keys))
	for i, key := range keys {
		out[i] = key
	}
	return out
}

// clone is a copy to change, never nil.
func (r RemovedWords) clone() RemovedWords {
	out := make(RemovedWords, len(r))
	for key := range r {
		out[key] = true
	}
	return out
}

// hides says whether a word said is made from a heard word the clip does
// not show.
func (r RemovedWords) hides(t *Transcript, w timed.Cue) bool {
	if len(r) == 0 {
		return false
	}
	heard, ok := t.HeardAt(w.Start)
	if !ok {
		return false
	}
	// Kept now, or from before 2.254 by where the word starts on the
	// sound, see heardIndex.
	key, was := t.keyOf(heard), wordKey(heard.Start)
	if r[key] || r[was] {
		return true
	}
	// One of the words a correction made of it.
	if !strings.Contains(heard.Text, " ") {
		return false
	}
	part := t.PartOf(heard, w)
	return r[partKey(key, part)] || r[partKey(was, part)]
}

// The ones below are for a clip's words as they are edited, whose keys
// are all kept the way they are now, see editClipWords.

// take removes a heard word from the clip, all of it.
func (r RemovedWords) take(t *Transcript, heard timed.Cue) {
	key := t.keyOf(heard)
	r.dropParts(key)
	r[key] = true
}

// takePart removes one of the words a correction made of a heard word
// from the clip, counted from nought.
func (r RemovedWords) takePart(t *Transcript, heard timed.Cue, part int) {
	r[partKey(t.keyOf(heard), part)] = true
}

// forget shows a heard word in the clip again, all of it.
func (r RemovedWords) forget(t *Transcript, heard timed.Cue) {
	key := t.keyOf(heard)
	r.dropParts(key)
	delete(r, key)
}

// partsOf is which of the words a correction made of a heard word the
// clip does not show, of those it reads now: a key for a word it no
// longer has, from a plan, counts for nothing.
func (r RemovedWords) partsOf(t *Transcript, heard timed.Cue) []int {
	key := t.keyOf(heard)
	var out []int
	n := len(strings.Fields(heard.Text))
	for k := range r {
		if word, part, ok := splitRemoved(k); ok && part >= 0 && part < n && word == key {
			out = append(out, part)
		}
	}
	slices.Sort(out)
	return out
}

// dropParts forgets every word a correction made of the heard word kept
// by key.
func (r RemovedWords) dropParts(key string) {
	for k := range r {
		if word, part, ok := splitRemoved(k); ok && part >= 0 && word == key {
			delete(r, k)
		}
	}
}

// removesHeard says whether the clip removes a heard word.
func (r RemovedWords) removesHeard(t *Transcript, heard timed.Cue) bool {
	return r[t.keyOf(heard)]
}
