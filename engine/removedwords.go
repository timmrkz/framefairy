package engine

import (
	"cmp"
	"slices"
	"strconv"
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
// millisecond it starts in the episode, as a correction is. Taking out
// some of the words a correction made of a heard word is not a removal,
// it is the correction made to read the rest, see dropWords.
type RemovedWords map[string]bool

// maxRemovedWords is how many words one clip may keep removed, far more
// than a clip of any length says.
const maxRemovedWords = 5000

// readRemovedWords takes the words a clip does not show out of its plan
// entry, which is untrusted like the rest of it. A key that is not a
// millisecond of an episode is left out.
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
		at, err := strconv.ParseInt(key, 10, 64)
		if err != nil || at < 0 || at > MaxEpisodeSeconds*1000 || strconv.FormatInt(at, 10) != key {
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
	keys := make([]string, 0, len(r))
	for key := range r {
		keys = append(keys, key)
	}
	slices.SortFunc(keys, func(a, b string) int {
		am, _ := strconv.ParseInt(a, 10, 64)
		bm, _ := strconv.ParseInt(b, 10, 64)
		return cmp.Compare(am, bm)
	})
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
func (r RemovedWords) hides(t *Transcript, w Cue) bool {
	if len(r) == 0 {
		return false
	}
	heard, ok := t.HeardAt(w.Start)
	return ok && r[wordKey(heard.Start)]
}

// take removes a heard word from the clip.
func (r RemovedWords) take(heard Cue) {
	r[wordKey(heard.Start)] = true
}

// forget shows a heard word in the clip again.
func (r RemovedWords) forget(heard Cue) {
	delete(r, wordKey(heard.Start))
}

// removesHeard says whether the clip removes a heard word.
func (r RemovedWords) removesHeard(heard Cue) bool {
	return r[wordKey(heard.Start)]
}
