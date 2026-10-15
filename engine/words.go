package engine

import (
	"cmp"
	"maps"
	"math"
	"slices"
	"sort"
	"strconv"
	"strings"
	"unicode/utf8"
)

// The words an episode says, see docs/WORDS.md. There is one list of them,
// Transcript.Words, and it is made here and nowhere else: the recogniser's
// words moved onto the sound. The prompt, a search and the app read it as
// it was heard. A clip reads it through Of, with its own corrections, and
// a correction that reads as several words is split into those words.

// hear makes the words from what the recogniser heard, moved onto the
// sound, with no corrections yet.
func (t *Transcript) hear(raw []Cue) {
	t.RawWords = raw
	t.snapped = SnapWords(raw, t.Frames, t.Start, t.Floor)
	t.keys, t.byKey = nil, nil
	if len(raw) == len(t.snapped) {
		t.keys = make([]string, len(raw))
		t.byKey = make(map[string]int, len(raw))
		for i, w := range raw {
			t.keys[i] = wordKey(w.Start)
			t.byKey[t.keys[i]] = i
		}
	}
	t.Correct(nil)
	t.Language = wordsLanguage(t.Words)
}

// Correct makes the words again with a clip's corrections, keyed by the
// heard word they correct, see heardIndex and Clip.Corrections. A
// correction says what a word reads, and one that reads as several words
// splits the word into them. Removing a word is the clip's too, see
// RemovedWords, and no correction takes a word out. The language stays
// the episode's, whatever one clip spells.
func (t *Transcript) Correct(corrections map[string]string) {
	fixes := make(map[int]string, len(corrections))
	for key, text := range corrections {
		i, ok := t.heardIndex(key)
		if !ok || strings.TrimSpace(text) == "" {
			continue
		}
		// A word kept by its own key and by the key from before, see
		// heardIndex, reads what its own key says.
		if _, twice := fixes[i]; !twice || key == t.keyAt(i) {
			fixes[i] = text
		}
	}
	heard := make([]Cue, 0, len(t.snapped))
	words := make([]Cue, 0, len(t.snapped))
	for i, w := range t.snapped {
		if text, ok := fixes[i]; ok {
			w.Text = text
		}
		heard = append(heard, w)
		if strings.TrimSpace(w.Text) == "" {
			continue
		}
		words = append(words, splitWord(w)...)
	}
	t.HeardWords, t.Words = heard, words
	t.fixes = maps.Clone(corrections)
}

// Of is the words as a clip has them: what was heard, with the clip's own
// corrections. Every clip reads its words through here, its captions, its
// edges and the edits of its words alike, so a word corrected in one clip
// reads as it was heard in every other, and in a clip made again where a
// removed one was. A clip with no corrections reads the episode's words
// as they are, and nothing is copied.
//
// A clip's captions are laid out through here on every pointer move on an
// edge, two or three times, so only the words a clip corrects are made
// again, see correctFew. Making every word of the episode again took 13 ms
// and 6 MB a time for four hours of words.
func (t *Transcript) Of(clip Clip) *Transcript {
	if maps.Equal(t.fixes, clip.Corrections) || (len(t.fixes) == 0 && len(clip.Corrections) == 0) {
		return t
	}
	own := *t
	if len(t.fixes) > 0 || !own.correctFew(clip.Corrections) {
		own.Correct(clip.Corrections)
	}
	return &own
}

// correctFew is Correct for a transcript that has no corrections yet: the
// words are copied, and only the ones the corrections name are made
// again, in their place. It says false, and changes nothing, when the
// words are not laid out the way it expects, and Correct makes them all.
func (t *Transcript) correctFew(corrections map[string]string) bool {
	type fix struct {
		i    int
		text string
	}
	fixes := make([]fix, 0, len(corrections))
	seen := make(map[int]bool, len(corrections))
	for key, text := range corrections {
		if strings.TrimSpace(text) == "" {
			continue
		}
		i, ok := t.heardIndex(key)
		if !ok || i >= len(t.HeardWords) {
			// Correct keeps no correction of a word that is not there.
			continue
		}
		if seen[i] || (i+1 < len(t.HeardWords) && t.HeardWords[i+1].Start == t.HeardWords[i].Start) {
			return false
		}
		seen[i] = true
		fixes = append(fixes, fix{i, text})
	}
	slices.SortFunc(fixes, func(a, b fix) int { return cmp.Compare(a.i, b.i) })
	heard := slices.Clone(t.HeardWords)
	words := make([]Cue, 0, len(t.Words)+len(fixes))
	from := 0
	for _, f := range fixes {
		was := t.HeardWords[f.i]
		// The words a heard word makes stand together, the first where it
		// starts, see splitWord.
		at := sort.Search(len(t.Words), func(i int) bool { return t.Words[i].Start >= was.Start })
		n := 0
		if strings.TrimSpace(was.Text) != "" {
			n = max(len(strings.Fields(was.Text)), 1)
		}
		if at < from || at+n > len(t.Words) || (n > 0 && t.Words[at].Start != was.Start) {
			return false
		}
		words = append(words, t.Words[from:at]...)
		heard[f.i].Text = f.text
		words = append(words, splitWord(heard[f.i])...)
		from = at + n
	}
	words = append(words, t.Words[from:]...)
	t.HeardWords, t.Words = heard, words
	t.fixes = maps.Clone(corrections)
	return true
}

// A correction and a removed word are kept against the heard word they
// are about, by the millisecond the recogniser heard it start, which
// nothing moves. They used to be kept by the millisecond it starts once it
// is moved onto the sound, which the silence setting moves: a change of the
// setting moved words, and the corrections and removed words of every clip
// over them no longer met their words, without a word said. Plan row 2.254.

// keyAt is the key the i-th heard word is kept by.
func (t *Transcript) keyAt(i int) string {
	if i < len(t.keys) {
		return t.keys[i]
	}
	return wordKey(t.snapped[i].Start)
}

// heardIndex is which heard word a key is about. A key that is the start
// of no heard word is one from before 2.254, the start of a word moved
// onto the sound, and is read that way, with the silence setting of now,
// which is the one it was most likely kept with.
func (t *Transcript) heardIndex(key string) (int, bool) {
	if i, ok := t.byKey[key]; ok {
		return i, true
	}
	ms, err := strconv.ParseInt(key, 10, 64)
	if err != nil {
		return 0, false
	}
	at := float64(ms) / 1000
	i := sort.Search(len(t.snapped), func(i int) bool { return t.snapped[i].Start >= at-0.0006 })
	for ; i < len(t.snapped) && t.snapped[i].Start < at+0.0006; i++ {
		if wordKey(t.snapped[i].Start) == key {
			return i, true
		}
	}
	return 0, false
}

// keyOf is the key of a heard word, one of HeardWords.
func (t *Transcript) keyOf(heard Cue) string {
	i := sort.Search(len(t.snapped), func(i int) bool { return t.snapped[i].Start >= heard.Start-0.0005 })
	if i < len(t.snapped) && math.Abs(t.snapped[i].Start-heard.Start) < 0.0005 {
		return t.keyAt(i)
	}
	return wordKey(heard.Start)
}

// rekeyCorrections is a clip's corrections, each kept by the key of the
// word it corrects, so a key from before 2.254 is written again the way
// keys are kept now. A key of a word this transcript does not hold is
// kept as it is.
func (t *Transcript) rekeyCorrections(corrections map[string]string) map[string]string {
	out := make(map[string]string, len(corrections))
	for key, text := range corrections {
		own := key
		if i, ok := t.heardIndex(key); ok {
			own = t.keyAt(i)
		}
		if _, twice := out[own]; !twice || key == own {
			out[own] = text
		}
	}
	return out
}

// rekeyRemoved is rekeyCorrections for the words a clip does not show.
func (t *Transcript) rekeyRemoved(removed RemovedWords) RemovedWords {
	out := make(RemovedWords, len(removed))
	for key := range removed {
		if i, ok := t.heardIndex(key); ok {
			key = t.keyAt(i)
		}
		out[key] = true
	}
	return out
}

// captionWordsIn is the words a clip's captions are laid out from: the
// words said that touch from to to, which is all a clip's captions read,
// see nearClip, with the ones the clip removed among them, their text
// behind removedMark. A removed word is not shown, but the captions still
// know it was said and what it reads, so they are laid out as they were
// before it went: a word taken out takes out the word and changes no
// caption, see Captions.
//
// A clip's captions were made from every word of the episode, copied
// whenever a word had been removed: 1 MB for every pointer move on an
// edge of a clip of an episode of four hours.
func (t *Transcript) captionWordsIn(removed RemovedWords, from, to float64) []Cue {
	out := between(t.Words, from, to)
	if len(removed) == 0 {
		return out
	}
	copied := false
	for i, w := range out {
		if !removed.hides(t, w) {
			continue
		}
		if !copied {
			out, copied = append([]Cue(nil), out...), true
		}
		out[i].Text = removedMark + w.Text
	}
	return out
}

// clipCaptionWords is captionWordsIn for a clip, the words its captions
// can need.
func (t *Transcript) clipCaptionWords(clip Clip) []Cue {
	t = t.Of(clip)
	if len(clip.Segments) == 0 {
		return t.captionWordsIn(clip.RemovedWords, math.Inf(-1), math.Inf(1))
	}
	return t.captionWordsIn(clip.RemovedWords, clip.Segments[0].Start-captionReach,
		clip.Segments[len(clip.Segments)-1].End+captionReach)
}

// between is the words, in the order they were said, that touch from to
// to: the first that ends after from up to the first that starts at to.
func between(words []Cue, from, to float64) []Cue {
	i := sort.Search(len(words), func(i int) bool { return words[i].End > from })
	j := i + sort.Search(len(words)-i, func(k int) bool { return words[i+k].Start >= to })
	return words[i:j]
}

// captionReach is how far either side of a clip its captions look at the
// words. They need only the words the clip holds and a word found within
// 20 ms of a caption's, see SaidWord, so a second is plenty.
const captionReach = 1.0

// nearClip is the part of the words, in the order they were said, that a
// clip's captions can need. Captions read every word of the episode, and
// looked each caption's words up among all of them, twice a caption.
func nearClip(clip Clip, words []Cue) []Cue {
	if len(clip.Segments) == 0 {
		return words
	}
	return between(words, clip.Segments[0].Start-captionReach,
		clip.Segments[len(clip.Segments)-1].End+captionReach)
}

// splitWord turns a word that reads as several words into one word each. The
// recogniser heard one word where more were said, so the span it measured
// is shared out by how long the words are. The audio is not read again: the
// highlight only has to run over the words inside the part they were said
// in.
func splitWord(word Cue) []Cue {
	parts := strings.Fields(word.Text)
	if len(parts) < 2 {
		return []Cue{word}
	}
	total := 0
	for _, part := range parts {
		total += utf8.RuneCountInString(part)
	}
	out := make([]Cue, 0, len(parts))
	at, span := word.Start, word.End-word.Start
	for i, part := range parts {
		end := word.End
		if i < len(parts)-1 {
			end = at + span*float64(utf8.RuneCountInString(part))/float64(total)
		}
		out = append(out, Cue{at, end, part})
		at = end
	}
	return out
}

// HeardAt is the word the recogniser heard at a moment, the one a
// correction is kept against, with its correction. A word a correction
// split in two, or the captions hyphenated, is one word here, so correcting
// either half corrects the word.
func (t *Transcript) HeardAt(at float64) (Cue, bool) {
	i := sort.Search(len(t.HeardWords), func(i int) bool { return t.HeardWords[i].End > at })
	if i < len(t.HeardWords) && at >= t.HeardWords[i].Start-0.0015 {
		return t.HeardWords[i], true
	}
	return Cue{}, false
}

// PartOf is which of the words a correction made of a heard word a word
// is, counted from nought: the words before it that the same heard word
// made. The words a heard word makes start inside it, one after the
// other, see splitWord.
func (t *Transcript) PartOf(heard, word Cue) int {
	from := sort.Search(len(t.Words), func(i int) bool { return t.Words[i].Start >= heard.Start-0.0015 })
	to := sort.Search(len(t.Words), func(i int) bool { return t.Words[i].Start >= word.Start-0.0005 })
	return max(to-from, 0)
}

// Said gives the words a clip says, on the episode clock: every word of
// the episode some of whose sound one of the clip's pieces holds, see
// HoldsWord. A clip keeps no words of its own. It is its pieces, and what
// it says is read off the episode's words whenever it is needed, so a
// correction or a change to how words are timed reaches every clip at
// once.
func Said(clip Clip, words []Cue) []Cue {
	if len(clip.Segments) == 0 {
		return nil
	}
	from, to := clip.Segments[0].Start, clip.Segments[len(clip.Segments)-1].End
	first := sort.Search(len(words), func(i int) bool { return words[i].End > from })
	var out []Cue
	for _, w := range words[first:] {
		if w.Start >= to {
			break
		}
		for _, seg := range clip.Segments {
			if HoldsWord(seg.Start, seg.End, w) {
				out = append(out, w)
				break
			}
		}
	}
	return out
}

// ClipWords puts a clip's words on the clip's own clock. A word moves by
// however much earlier material the clip contains, which is plain arithmetic
// because every word knows where it was spoken. A word corrected into
// several words becomes one cue per word, so captions break and highlight
// them one by one.
//
// A word is captioned when the clip holds some of its sound, the rule that
// decides which words a clip says, see HoldsWord, so a word an edge cuts
// into is captioned from the edge on for as long as any of it is heard. A
// word a cut parts is captioned once, in the piece that holds the most of
// it. It used to have to lie wholly inside a piece, so a word the clip
// said had no caption until the edge had passed its first sound.
func ClipWords(clip Clip, words []Cue) []Cue {
	out, _ := clipWords(clip, words)
	return out
}

// clipWords is ClipWords with, beside each word on the clip's clock, the
// same word on the episode's, which is where its pauses are measured: a cut
// shortens a pause on the clip's clock and leaves the episode's alone.
func clipWords(clip Clip, words []Cue) ([]Cue, []Cue) {
	type pair struct{ at, was Cue }
	var pairs []pair
	offsets := make([]float64, len(clip.Segments))
	sum := 0.0
	for i, segment := range clip.Segments {
		offsets[i] = sum
		sum += segment.Duration()
	}
	for _, word := range Said(clip, words) {
		best, most := -1, 0.0
		for i, segment := range clip.Segments {
			if !HoldsWord(segment.Start, segment.End, word) {
				continue
			}
			if held := math.Min(word.End, segment.End) - math.Max(word.Start, segment.Start); held > most {
				best, most = i, held
			}
		}
		if best < 0 {
			continue
		}
		segment := clip.Segments[best]
		// A word keeps its own time at the clip's first and last edge, even
		// where that lies outside the clip, and a caption shows from the
		// first frame whatever its first word says. Clamped there, a word an
		// edge cut into was squeezed into what was left of it, and the
		// halves of a hyphenated word, which share its time by their
		// letters, moved with the edge: "liebe" was lit where "grundschul"
		// was still being said. At a cut the word is clamped, because the
		// clock of the piece before runs on the other side of it.
		start, end := word.Start, word.End
		if best > 0 {
			start = math.Max(start, segment.Start)
		}
		if best < len(clip.Segments)-1 {
			end = math.Min(end, segment.End)
		}
		pairs = append(pairs, pair{Cue{offsets[best] + (start - segment.Start),
			offsets[best] + (end - segment.Start), word.Text}, word})
	}
	sort.SliceStable(pairs, func(i, j int) bool { return pairs[i].at.Start < pairs[j].at.Start })
	out := make([]Cue, len(pairs))
	was := make([]Cue, len(pairs))
	for i, p := range pairs {
		out[i], was[i] = p.at, p.was
	}
	return out, was
}

// wordTouch is how much of a word a piece has to hold for the clip to say
// it: a frame at the rates people film at, so a word is not counted for the
// rounding of an edge that stops where it starts.
const wordTouch = 0.02

// HoldsWord says whether a piece of a clip holds some of a word's sound,
// which is when the clip says the word. It is one rule for the words a
// clip keeps, the captions it shows and the drag that shapes it. A word is
// said as soon as any of it is heard, so dragging an edge back over a word
// brings its caption in at the word's last sound, the first of it the edge
// reaches. It used to take the word's middle, and a long word stayed
// uncaptioned for half its length while it was plainly heard.
//
// A piece that holds the whole of a word says it, however short it is.
// wordTouch is for an edge that grazes a word, and a word a correction
// made several shares its time out among them, so a short word typed into
// a word two frames long gets less than a frame. The captions left such a
// word out, and the frames it stood in showed none.
func HoldsWord(start, end float64, w Cue) bool {
	held := math.Min(end, w.End) - math.Max(start, w.Start)
	return held > wordTouch || (held > 0 && w.Start >= start && w.End <= end)
}

// ClipCaptions makes a clip's captions, laid out in the lines they are
// drawn in. It is the one way captions are made, for the render and for the
// app alike: from the words the clip says, in the clip's style, hyphenated
// for the episode's language.
func ClipCaptions(clip Clip, t *Transcript, s Style) []LaidCaption {
	t = t.Of(clip)
	captions := Captions(clip, t.clipCaptionWords(clip), max(8, int(s.MaxChars)), TooWide(s))
	return LayOutCaptions(captions, s, t.Language)
}
