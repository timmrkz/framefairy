package engine

import (
	"slices"
	"strconv"
	"strings"
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
// What a word reads stays the video's, see corrections.go: a misheard word
// is wrong in every clip.

// RemovedWords is the words a clip does not show, each kept against the
// word heard, by the millisecond it starts in the episode, as a
// correction is. A key alone is the whole of that word. A key with a
// slash and a number is one of the words a correction made the heard word
// into, counted from nought, the way a caption's word says it with Part.
type RemovedWords map[string]bool

// maxRemovedWords is how many words one clip may keep removed, far more
// than a clip of any length says.
const maxRemovedWords = 5000

// removedKey is the key of a word heard at start, or of one of the words a
// correction made of it, part counted from nought, or the whole of it with
// part below nought.
func removedKey(start float64, part int) string {
	if part < 0 {
		return wordKey(start)
	}
	return wordKey(start) + "/" + strconv.Itoa(part)
}

// readRemovedWords takes the words a clip does not show out of its plan
// entry, which is untrusted like the rest of it. A key that is not a
// millisecond of an episode, with a part of a word that could be there,
// is left out.
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
		ms, part, split := strings.Cut(key, "/")
		at, err := strconv.ParseInt(ms, 10, 64)
		if err != nil || at < 0 || at > MaxEpisodeSeconds*1000 || strconv.FormatInt(at, 10) != ms {
			continue
		}
		if split {
			// A correction is at most 60 characters, so it is never more
			// words than that.
			n, err := strconv.Atoi(part)
			if err != nil || n < 0 || n >= 60 || strconv.Itoa(n) != part {
				continue
			}
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
	keys := make([]string, 0, len(r))
	for key := range r {
		keys = append(keys, key)
	}
	slices.SortFunc(keys, func(a, b string) int {
		am, ap := splitRemovedKey(a)
		bm, bp := splitRemovedKey(b)
		if am != bm {
			return int(am - bm)
		}
		return ap - bp
	})
	out := make([]any, len(keys))
	for i, key := range keys {
		out[i] = key
	}
	return out
}

// splitRemovedKey is the millisecond and the part of a key, the part
// below nought for the whole word.
func splitRemovedKey(key string) (int64, int) {
	ms, part, split := strings.Cut(key, "/")
	at, _ := strconv.ParseInt(ms, 10, 64)
	if !split {
		return at, -1
	}
	n, _ := strconv.Atoi(part)
	return at, n
}

// clone is a copy to change, never nil.
func (r RemovedWords) clone() RemovedWords {
	out := make(RemovedWords, len(r))
	for key := range r {
		out[key] = true
	}
	return out
}

// shownWord is the heard word a word said is made from and which of the
// words a correction made of it it is, or the whole of it, below nought,
// when it is one word.
func (t *Transcript) shownWord(w Cue) (Cue, int, bool) {
	heard, ok := t.HeardAt(w.Start)
	if !ok {
		return Cue{}, 0, false
	}
	if len(strings.Fields(heard.Text)) < 2 {
		return heard, -1, true
	}
	return heard, t.PartOf(heard, w), true
}

// hides says whether a word said is one the clip does not show.
func (r RemovedWords) hides(t *Transcript, w Cue) bool {
	if len(r) == 0 {
		return false
	}
	heard, part, ok := t.shownWord(w)
	if !ok {
		return false
	}
	return r[wordKey(heard.Start)] || (part >= 0 && r[removedKey(heard.Start, part)])
}

// take removes a word said from the clip. Once every word a correction
// made of a heard word is removed, the heard word is removed whole.
func (r RemovedWords) take(t *Transcript, w Cue) {
	heard, part, ok := t.shownWord(w)
	if !ok {
		return
	}
	r.takeHeard(heard, part)
}

// takeHeard removes a heard word from the clip, or one of the words a
// correction made of it, part counted from nought, or all of it with
// part below nought.
func (r RemovedWords) takeHeard(heard Cue, part int) {
	whole := wordKey(heard.Start)
	n := len(strings.Fields(heard.Text))
	if part < 0 || n < 2 {
		r.forget(heard)
		r[whole] = true
		return
	}
	if r[whole] {
		return
	}
	r[removedKey(heard.Start, part)] = true
	for k := range n {
		if !r[removedKey(heard.Start, k)] {
			return
		}
	}
	r.forget(heard)
	r[whole] = true
}

// put shows a word said in the clip again, and leaves the other words a
// correction made of the same heard word as they were.
func (r RemovedWords) put(t *Transcript, w Cue) {
	heard, part, ok := t.shownWord(w)
	if !ok {
		return
	}
	whole := wordKey(heard.Start)
	if part < 0 {
		r.forget(heard)
		return
	}
	if r[whole] {
		delete(r, whole)
		for k := range len(strings.Fields(heard.Text)) {
			if k != part {
				r[removedKey(heard.Start, k)] = true
			}
		}
		return
	}
	delete(r, removedKey(heard.Start, part))
}

// forget shows every word of a heard word in the clip again.
func (r RemovedWords) forget(heard Cue) {
	whole := wordKey(heard.Start)
	for key := range r {
		if key == whole || strings.HasPrefix(key, whole+"/") {
			delete(r, key)
		}
	}
}

// removesHeard says whether the clip removes all of a heard word.
func (r RemovedWords) removesHeard(heard Cue) bool {
	return r[wordKey(heard.Start)]
}
