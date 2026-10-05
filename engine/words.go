package engine

import (
	"math"
	"sort"
	"strings"
	"unicode/utf8"
)

// The words an episode says, see docs/WORDS.md. There is one list of them,
// Transcript.Words, and it is made here and nowhere else: the recogniser's
// words moved onto the sound, with the corrections applied, and a
// correction that reads as several words split into those words. Every
// reader gets the same list, the prompt, the edits, the captions and the
// app alike.

// hear makes the words from what the recogniser heard, moved onto the
// sound, with no corrections yet.
func (t *Transcript) hear(raw []Cue) {
	t.RawWords = raw
	t.snapped = SnapWords(raw, t.Frames, t.Start, t.Floor)
	t.Correct(nil)
}

// Correct makes the words again with an episode's corrections, keyed by the
// millisecond a word starts, as corrections.json keeps them. A word
// corrected to nothing is removed: it stays among the words heard, so it
// can be corrected again, and is left out of the words said.
func (t *Transcript) Correct(corrections map[string]string) {
	heard := make([]Cue, 0, len(t.snapped))
	words := make([]Cue, 0, len(t.snapped))
	for _, w := range t.snapped {
		if text, ok := corrections[wordKey(w.Start)]; ok {
			w.Text = text
		}
		heard = append(heard, w)
		if strings.TrimSpace(w.Text) == "" {
			continue
		}
		words = append(words, splitWord(w)...)
	}
	t.HeardWords, t.Words = heard, words
	t.Language = wordsLanguage(words)
}

// captionWords are the words with the words removed among them, with no
// text, in the order they were said. A removed word is not shown, but the
// captions still know it was said, see Captions.
func (t *Transcript) captionWords() []Cue {
	out := t.Words
	for _, w := range t.HeardWords {
		if strings.TrimSpace(w.Text) == "" {
			if len(out) == len(t.Words) {
				out = append([]Cue(nil), t.Words...)
			}
			out = append(out, w)
		}
	}
	if len(out) != len(t.Words) {
		sort.SliceStable(out, func(i, j int) bool { return out[i].Start < out[j].Start })
	}
	return out
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
func HoldsWord(start, end float64, w Cue) bool {
	return math.Min(end, w.End)-math.Max(start, w.Start) > wordTouch
}

// ClipCaptions makes a clip's captions, laid out in the lines they are
// drawn in. It is the one way captions are made, for the render and for the
// app alike: from the words the clip says, in the clip's style, hyphenated
// for the episode's language.
func ClipCaptions(clip Clip, t *Transcript, s Style) []LaidCaption {
	captions := Captions(clip, t.captionWords(), max(8, int(s.MaxChars)), TooWide(s))
	return LayOutCaptions(captions, s, t.Language)
}
