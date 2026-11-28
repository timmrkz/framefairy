package engine

import (
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
