package engine

import (
	"bufio"
	"embed"
	"io/fs"
	"strconv"
	"strings"
	"sync"
	"unicode"

	"github.com/abadojack/whatlanggo"
	"github.com/speedata/hyphenation"
)

// A caption word too wide for a line is broken the way TeX breaks it:
// Liang's algorithm over the hyphenation patterns of hyph-utf8, the same
// patterns LibreOffice and Firefox break words with. The patterns are per
// language, and the episode's language is not written anywhere, so it is
// read off the captions' own words.
//
// The patterns here are the languages the speech model hears whose
// patterns may ship in a paid app. Which files those are is decided by the
// licences their own headers name, and the notices refuse a file that
// cannot ship, see notices/gen. Czech is under the GPL alone, Latvian
// under the LGPL or the GPL, and Romanian has no licence at all, so those
// three are not here, and a word in them is broken where the line ends.
// Beside each language's patterns is the top of the file they came from,
// which says who made them, under what licence, and how few letters TeX
// leaves either side of a hyphen.
//
//go:embed hyphenation/*.txt
var hyphenationFiles embed.FS

// hyphenator breaks the words of one language.
type hyphenator struct {
	lang *hyphenation.Lang
	// left and right are the fewest letters before and after a hyphen.
	left, right int
}

var (
	hyphenatorsMu sync.Mutex
	hyphenators   = map[string]*hyphenator{}
)

// patternFile is the pattern file of a language, as ISO 639-1: the one
// named for it, or the one whose tag begins with it, like de-1996 for
// German. Empty when none is here.
func patternFile(code string) string {
	if code == "" {
		return ""
	}
	names, _ := fs.Glob(hyphenationFiles, "hyphenation/hyph-"+code+".pat.txt")
	if len(names) == 0 {
		names, _ = fs.Glob(hyphenationFiles, "hyphenation/hyph-"+code+"-*.pat.txt")
	}
	if len(names) != 1 {
		return ""
	}
	return strings.TrimSuffix(names[0], ".pat.txt")
}

// hyphenatorFor gives the hyphenator of a language, as ISO 639-1, or nil
// when there are no patterns for it. Each is read once.
func hyphenatorFor(code string) *hyphenator {
	hyphenatorsMu.Lock()
	defer hyphenatorsMu.Unlock()
	if h, ok := hyphenators[code]; ok {
		return h
	}
	var h *hyphenator
	name := patternFile(code)
	if patterns, err := hyphenationFiles.Open(name + ".pat.txt"); name != "" && err == nil {
		defer patterns.Close()
		if lang, err := hyphenation.New(patterns); err == nil {
			h = &hyphenator{lang: lang, left: 2, right: 2}
			if head, err := hyphenationFiles.ReadFile(name + ".head.txt"); err == nil {
				h.left, h.right = hyphenMins(string(head), h.left, h.right)
			}
		}
	}
	hyphenators[code] = h
	return h
}

// hyphenMins reads the fewest letters either side of a hyphen that a
// pattern file asks for when typesetting.
func hyphenMins(head string, left, right int) (int, int) {
	inside := false
	scan := bufio.NewScanner(strings.NewReader(head))
	for scan.Scan() {
		line := strings.TrimSpace(strings.TrimLeft(scan.Text(), "%"))
		switch {
		case line == "typesetting:":
			inside = true
		case inside && strings.HasPrefix(line, "left:"):
			if n, err := strconv.Atoi(strings.TrimSpace(strings.TrimPrefix(line, "left:"))); err == nil {
				left = n
			}
		case inside && strings.HasPrefix(line, "right:"):
			if n, err := strconv.Atoi(strings.TrimSpace(strings.TrimPrefix(line, "right:"))); err == nil {
				right = n
			}
		case inside:
			return left, right
		}
	}
	return left, right
}

// spokenLanguages are the languages the speech model hears that the
// detector knows. Slovak is not among them and is read as Czech.
var spokenLanguages = map[whatlanggo.Lang]bool{
	whatlanggo.Bul: true, whatlanggo.Ces: true, whatlanggo.Dan: true, whatlanggo.Deu: true,
	whatlanggo.Ell: true, whatlanggo.Eng: true, whatlanggo.Est: true, whatlanggo.Fin: true,
	whatlanggo.Fra: true, whatlanggo.Hrv: true, whatlanggo.Hun: true, whatlanggo.Ita: true,
	whatlanggo.Lav: true, whatlanggo.Lit: true, whatlanggo.Nld: true, whatlanggo.Pol: true,
	whatlanggo.Por: true, whatlanggo.Ron: true, whatlanggo.Rus: true, whatlanggo.Slv: true,
	whatlanggo.Spa: true, whatlanggo.Swe: true, whatlanggo.Ukr: true,
}

// languageOf says which language captions are in, as ISO 639-1, read off
// all their words together.
func languageOf(captions []Caption) string {
	var text strings.Builder
	for _, c := range captions {
		if len(c.Words) == 0 {
			text.WriteString(c.Text + " ")
			continue
		}
		for _, w := range c.Words {
			text.WriteString(w.Text + " ")
		}
	}
	return whatlanggo.DetectLangWithOptions(text.String(),
		whatlanggo.Options{Whitelist: spokenLanguages}).Iso6391()
}

// captionRoom is how wide a caption line may be, and how wide a text is.
type captionRoom struct {
	font      string
	size      float64
	room      float64
	wrapChars int
}

func roomFor(s Style) captionRoom {
	return captionRoom{s.Font, s.Size, CaptionRoom(s), s.WrapChars}
}

// width is how wide a text is drawn. A face the program cannot measure is
// counted in characters, as a share of the room.
func (r captionRoom) width(text string) float64 {
	if width, ok := TextWidth(r.font, text, r.size); ok {
		return width
	}
	return float64(runeLen(text)) * r.room / float64(max(r.wrapChars, 1))
}

func (r captionRoom) fits(text string) bool { return r.width(text) <= r.room }

// TooWide says whether a word is too wide for a caption line of its own,
// so the captions can give it a caption of its own too.
func TooWide(s Style) func(string) bool {
	r := roomFor(s)
	return func(word string) bool { return !r.fits(word) }
}

// breakWord cuts a word too wide for a line into pieces that fit, every
// piece but the last ending in a hyphen. It breaks where the patterns of
// the language allow, and after a hyphen the word already has, and like
// TeX and every word processor it takes the last break that still fits,
// so each line is as full as it can be. With no patterns, or none that
// help, it is broken where the line ends. A word that fits, or that no
// break can help, comes back as it is.
func breakWord(word string, r captionRoom, h *hyphenator) []string {
	if r.fits(word) {
		return []string{word}
	}
	runes := []rune(word)
	points := breakPoints(runes, h)
	piece := func(from, to int) string {
		if runes[to-1] == '-' {
			return string(runes[from:to])
		}
		return string(runes[from:to]) + "-"
	}
	var out []string
	from := 0
	for !r.fits(string(runes[from:])) {
		cut := 0
		for _, p := range points {
			if p > from && r.fits(piece(from, p)) {
				cut = p
			}
		}
		if cut == 0 {
			break
		}
		out = append(out, piece(from, cut))
		from = cut
	}
	return append(out, string(runes[from:]))
}

// breakPoints are where a word may be broken, as the number of runes
// before the break, in order. Punctuation around the word stays with it.
func breakPoints(runes []rune, h *hyphenator) []int {
	first, last := 0, len(runes)
	for first < last && !unicode.IsLetter(runes[first]) {
		first++
	}
	for last > first && !unicode.IsLetter(runes[last-1]) {
		last--
	}
	left, right := 2, 2
	if h != nil {
		left, right = h.left, h.right
	}
	allowed := map[int]bool{}
	for i := first + 1; i < last; i++ {
		if runes[i-1] == '-' {
			allowed[i] = true
		}
	}
	if h != nil {
		for _, p := range h.lang.Hyphenate(strings.ToLower(string(runes[first:last]))) {
			allowed[first+p] = true
		}
	}
	var out []int
	for i := first + left; i <= last-right; i++ {
		// With nothing to go by, anywhere the line runs out will do.
		if allowed[i] || (h == nil && len(allowed) == 0) {
			out = append(out, i)
		}
	}
	return out
}

// hyphenate splits every word too wide for a line into pieces that fit.
// The span the word was spoken in is shared out by how long the pieces
// are, the way SplitCorrected shares out a word corrected into two. The
// words given are never changed, a split gives back a new list.
func hyphenate(words []Cue, r captionRoom, h *hyphenator) []Cue {
	var out []Cue
	for i, w := range words {
		parts := breakWord(w.Text, r, h)
		if len(parts) < 2 && out == nil {
			continue
		}
		if out == nil {
			out = append(make([]Cue, 0, len(words)+1), words[:i]...)
		}
		total := 0
		for _, part := range parts {
			total += runeLen(strings.TrimSuffix(part, "-"))
		}
		at, span := w.Start, w.End-w.Start
		for k, part := range parts {
			end := w.End
			if k < len(parts)-1 {
				end = at + span*float64(runeLen(strings.TrimSuffix(part, "-")))/float64(max(total, 1))
			}
			out = append(out, Cue{at, end, part})
			at = end
		}
	}
	if out == nil {
		return words
	}
	return out
}
