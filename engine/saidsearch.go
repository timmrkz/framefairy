package engine

import (
	"slices"
	"strings"
	"sync"
	"unicode"
	"unicode/utf16"
)

// The search in the clip column, over the cards of what was said, see
// SaidCards. What is typed is answered here, the cards that answer and
// the words in them to set in bold, and the window shows what it is told.
// It was the window's own rule once, issue #927.

// CardFound is a card that answers what was typed: where it starts and
// the clip it is named for, which find it among the cards the window
// holds, and the words to set in bold, from and to in its text. They are
// counted in the UTF-16 units the window counts a string in, so a word
// after an emoji is bold where it stands.
type CardFound struct {
	Start float64  `json:"start"`
	Clip  string   `json:"clip,omitempty"`
	Bold  [][2]int `json:"bold"`
}

// SaidSearch is the cards of what was said, ready to be searched again at
// every letter typed: each card's text in small letters and its words,
// worked out once for the cards rather than at every letter.
type SaidSearch struct {
	ready func() readyCards
}

// readyCards are the cards made ready, and every word they say once, in
// small letters, which a word of a card names by its place.
type readyCards struct {
	cards []searchCard
	words []string
}

// searchCard is one card made ready: its text in small letters, a letter
// for a letter, and its words.
type searchCard struct {
	card  SentenceView
	have  string
	words []searchWord
}

// searchWord is one word of a card, without the marks around it, which
// are not bold: where it is in the card's text in small letters, in
// bytes, where it is in the card's own text for the window, and which of
// the words said it is, see readyCards.
type searchWord struct {
	from, to     int
	from16, to16 int
	said         int
}

// NewSaidSearch makes the cards ready to search. The work is done the
// first time something is asked, so cards that are only read cost
// nothing more.
func NewSaidSearch(cards []SentenceView) *SaidSearch {
	return &SaidSearch{ready: sync.OnceValue(func() readyCards {
		out := readyCards{cards: make([]searchCard, len(cards))}
		said := map[string]int{}
		for i, c := range cards {
			out.cards[i] = readyCard(c, func(bare string) int {
				n, ok := said[bare]
				if !ok {
					n = len(out.words)
					said[bare] = n
					out.words = append(out.words, bare)
				}
				return n
			})
		}
		return out
	})}
}

// readyCard is a card in small letters, with its words. A word is a run
// of what is not a space, from its first letter or digit to its last.
// said names a word in small letters by its place among the words said.
func readyCard(c SentenceView, said func(bare string) int) searchCard {
	var have strings.Builder
	have.Grow(len(c.Text))
	var words []searchWord
	at16 := 0
	// Where the word being read began, and where its last letter or
	// digit so far ends, in bytes of have and in UTF-16 units.
	inWord := false
	var w searchWord
	end := func() {
		if inWord {
			w.said = said(have.String()[w.from:w.to])
			words = append(words, w)
		}
		inWord = false
	}
	for _, r := range c.Text {
		if unicode.IsSpace(r) {
			end()
		}
		at := have.Len()
		have.WriteRune(unicode.ToLower(r))
		n16 := utf16.RuneLen(r)
		if n16 < 1 {
			n16 = 1
		}
		if unicode.IsLetter(r) || unicode.IsNumber(r) {
			if !inWord {
				inWord = true
				w = searchWord{from: at, from16: at16}
			}
			w.to, w.to16 = have.Len(), at16+n16
		}
		at16 += n16
	}
	end()
	return searchCard{card: c, have: have.String(), words: words}
}

// Answer is the cards that answer what was typed, in their order, and the
// words of each to set in bold. A card answers when it holds what was
// typed, and the words that hold it are bold. Only where no card does,
// a card answers when every word typed is a word of it typed with a slip,
// see typedWithSlip, and every word found is bold. Three letters answer a lot of
// words that way, and in a video of an hour the cards that hold them were
// lost among cards that only came near, found in the review of #367.
// Nothing typed answers every card with nothing bold.
func (s *SaidSearch) Answer(typed string) []CardFound {
	want := strings.Join(strings.Fields(strings.Map(unicode.ToLower, typed)), " ")
	ready := s.ready()
	out := []CardFound{}
	for _, c := range ready.cards {
		if bold, ok := c.holds(want); ok {
			out = append(out, CardFound{Start: c.card.Start, Clip: c.card.Clip, Bold: bold})
		}
	}
	if len(out) > 0 || want == "" {
		return out
	}
	// Whether each word said answers each word typed, asked once for
	// each: a video says the same few thousand words over and over.
	typedWords := strings.Fields(want)
	answers := make([][]bool, len(typedWords))
	for i, a := range typedWords {
		answers[i] = make([]bool, len(ready.words))
		for n, bare := range ready.words {
			answers[i][n] = typedWithSlip([]rune(bare), []rune(a))
		}
	}
	bold := []bool{}
	for _, c := range ready.cards {
		bold = slices.Grow(bold[:0], len(c.words))[:len(c.words)]
		if found, ok := c.near(answers, bold); ok {
			out = append(out, CardFound{Start: c.card.Start, Clip: c.card.Clip, Bold: found})
		}
	}
	return out
}

// holds is the words of the card that hold want, in small letters with
// one space between words, and whether there are any. Nothing wanted is
// held by every card, with nothing bold.
func (c searchCard) holds(want string) ([][2]int, bool) {
	if want == "" {
		return [][2]int{}, true
	}
	var bold [][2]int
	w := 0
	for from := 0; ; {
		i := strings.Index(c.have[from:], want)
		if i < 0 {
			break
		}
		at := from + i
		end := at + len(want)
		for w < len(c.words) && c.words[w].to <= at {
			w++
		}
		for k := w; k < len(c.words) && c.words[k].from < end; k++ {
			if n := len(bold); n == 0 || bold[n-1][0] != c.words[k].from16 {
				bold = append(bold, [2]int{c.words[k].from16, c.words[k].to16})
			}
		}
		from = end
	}
	return bold, len(bold) > 0
}

// near is the words of the card that are the words typed, each typed
// with a slip, and whether every word typed is one of them. answers says
// for each word typed which words said answer it, and bold is room for a
// mark on each word of the card.
func (c searchCard) near(answers [][]bool, bold []bool) ([][2]int, bool) {
	clear(bold)
	for _, said := range answers {
		found := false
		for k, w := range c.words {
			if said[w.said] {
				found, bold[k] = true, true
			}
		}
		if !found {
			return nil, false
		}
	}
	var out [][2]int
	for k, w := range c.words {
		if bold[k] {
			out = append(out, [2]int{w.from16, w.to16})
		}
	}
	return out, true
}

// typedWithSlip is whether a word of the video is the word typed, typed with a
// slip: a letter too many, too few, wrong, or two letters swapped. A
// word of up to seven letters may have one slip and a longer one two,
// and two letters are too few to have any. A word typed from five
// letters on may also be the start of a longer word, so a slip while
// typing does not lose the word before it is finished. The font list's
// way of finding a name, by its letters in order with gaps, is not used
// here: a transcript has thousands of words, and ernste found
// Verteidigungsministers that way.
func typedWithSlip(word, typed []rune) bool {
	if len(typed) < 3 {
		return false
	}
	slips := 2
	if len(typed) <= 7 {
		slips = 1
	}
	if absInt(len(word)-len(typed)) <= slips && slipsApart(word, typed, slips) <= slips {
		return true
	}
	return len(typed) >= 5 && len(word) > len(typed) && slipsApart(word[:len(typed)], typed, slips) <= slips
}

// slipsApart is how many slips apart two words are, letters added, taken
// away, changed or swapped with the one beside it, counted up to most
// and no further.
func slipsApart(a, b []rune, most int) int {
	rows := make([][]int, 0, len(a)+1)
	first := make([]int, len(b)+1)
	for j := range first {
		first[j] = j
	}
	rows = append(rows, first)
	for i := 1; i <= len(a); i++ {
		row := make([]int, len(b)+1)
		row[0] = i
		least := i
		for j := 1; j <= len(b); j++ {
			change := 1
			if a[i-1] == b[j-1] {
				change = 0
			}
			d := min(rows[i-1][j]+1, row[j-1]+1, rows[i-1][j-1]+change)
			if i > 1 && j > 1 && a[i-1] == b[j-2] && a[i-2] == b[j-1] {
				d = min(d, rows[i-2][j-2]+1)
			}
			row[j] = d
			least = min(least, d)
		}
		if least > most {
			return most + 1
		}
		rows = append(rows, row)
	}
	return rows[len(a)][len(b)]
}
