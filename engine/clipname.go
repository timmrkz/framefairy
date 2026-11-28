package engine

import (
	"maps"
	"slices"
	"strconv"
	"strings"

	"framefairy/engine/runes"
)

// A clip is called by its first words: its title on its card, and the
// name of its short and of the pictures taken of it, see Clip.ShortName.
// They are made by one rule, here, from the words the clip shows now, so a
// typo corrected in the caption box or a start moved renames the clip.
// They were written into the plan once, when the clip was made, and a clip
// whose first words changed kept the old ones, the user's report on issue
// #1015.
//
// The plan still keeps a title and a slug. The slug names the clip's own
// files in the work folder, see Clip.Basename, and the short a render
// wrote, see recordShort, which writes the name the clip had then.

// titleMost is how long a clip's title may be, in letters, said whole
// words at a time.
const titleMost = 60

// clipTitle is what a clip of these words is called: as many of its first
// words as fit in titleMost letters, and always the first. The words of a
// clip can be typed in the caption box, so they are made safe to print and
// to store, the way the model's titles are.
func clipTitle(words []string) string {
	return runes.Scrub(firstWords(strings.Join(words, " "), titleMost), 200)
}

// clipSlug is the name of a clip's files, from its title.
func clipSlug(title string) string {
	return strings.ToLower(SanitiseName(title, "clip"))
}

// titleWords is how many words a title is looked for in. A title of 60
// letters holds fewer.
const titleWords = 20

// Named is the title and the slug of a clip by the words it shows now:
// what the clip says, with its own corrections, and without the words it
// removed. A clip that shows no words, or a transcript with none, keeps
// the ones its plan has.
func (t *Transcript) Named(c Clip) (title, slug string) {
	if t == nil || len(t.Words) == 0 {
		return c.Title, c.Slug
	}
	c.Corrections = t.correctionsOfFirst(c)
	// The clip list names every clip on every read of a video's state. A
	// correction makes the words of the whole episode again, see
	// Transcript.Of, about half a millisecond and a megabyte for four
	// hours of words, and a clip without one still takes a few
	// microseconds. So a name is kept with the transcript read, by
	// everything the clip's first words depend on.
	key := namedKey(c)
	l := t.lookups
	if l == nil {
		return t.namedNow(c)
	}
	l.namedMu.Lock()
	got, ok := l.named[key]
	l.namedMu.Unlock()
	if ok {
		return got[0], got[1]
	}
	title, slug = t.namedNow(c)
	l.namedMu.Lock()
	if l.named == nil || len(l.named) >= namedKept {
		l.named = map[string][2]string{}
	}
	l.named[key] = [2]string{title, slug}
	l.namedMu.Unlock()
	return title, slug
}

// namedKept is how many names of clips with corrections a transcript read
// keeps, a few for every clip a person edits, and then starts again.
const namedKept = 1024

// namedKey is everything a clip's name depends on besides the transcript:
// its pieces, the corrections of its first words, the words it removed
// and the name its plan gives it.
func namedKey(c Clip) string {
	b := make([]byte, 0, 64)
	for _, s := range c.Segments {
		b = strconv.AppendFloat(b, s.Start, 'g', -1, 64)
		b = append(b, '-')
		b = strconv.AppendFloat(b, s.End, 'g', -1, 64)
		b = append(b, ',')
	}
	for _, k := range slices.Sorted(maps.Keys(c.Corrections)) {
		b = append(b, '|')
		b = strconv.AppendQuote(b, k)
		b = strconv.AppendQuote(b, c.Corrections[k])
	}
	for _, k := range slices.Sorted(maps.Keys(c.RemovedWords)) {
		b = append(b, '|', '-')
		b = strconv.AppendQuote(b, k)
	}
	b = append(b, '|')
	b = strconv.AppendQuote(b, c.Title)
	b = strconv.AppendQuote(b, c.Slug)
	return string(b)
}

// namedNow is Named worked out.
func (t *Transcript) namedNow(c Clip) (title, slug string) {
	var words []string
	for _, w := range Said(c, t.clipCaptionWords(c)) {
		if strings.HasPrefix(w.Text, removedMark) {
			continue
		}
		words = append(words, strings.Fields(w.Text)...)
		if len(words) >= titleWords {
			break
		}
	}
	if len(words) == 0 {
		return c.Title, c.Slug
	}
	title = clipTitle(words)
	return title, clipSlug(title)
}

// correctionsOfFirst is the corrections of a clip that can change its
// name: those of its first words. A correction makes the words of the
// whole episode again, see Transcript.Of, which the clip list cannot do for
// every clip it lists, and a correction further on changes no title.
func (t *Transcript) correctionsOfFirst(c Clip) map[string]string {
	if len(c.Corrections) == 0 {
		return nil
	}
	said := Said(c, t.Words)
	if len(said) == 0 {
		return nil
	}
	// The heard word is where the recogniser heard it, which moving it
	// onto the sound shifts by a little, so a second is left either way.
	last := said[min(len(said), titleWords)-1].End + 1
	var out map[string]string
	for key, text := range c.Corrections {
		if i, ok := t.heardIndex(key); ok && i < len(t.HeardWords) && t.HeardWords[i].Start <= last {
			if out == nil {
				out = map[string]string{}
			}
			out[key] = text
		}
	}
	return out
}
