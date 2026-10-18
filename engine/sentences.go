package engine

import (
	"slices"
	"sort"
	"strings"
)

// SentenceView is one card of what an episode says, for reading the
// transcript in the app and searching it: when it is said and its words.
type SentenceView struct {
	Start float64 `json:"start"`
	End   float64 `json:"end"`
	Text  string  `json:"text"`
}

// cardSeconds is how long a card of what was said grows before it ends
// at the end of a line, though its sentence goes on. Sentences of half a
// minute made cards of twenty seconds and more, too much to read as one
// place in the video, found by Tim.
const cardSeconds = 8.0

// cardSecondsMost is as long as a card ever runs. A line runs up to 14
// seconds, so a card that reaches this inside a line ends at a word.
const cardSecondsMost = 12.0

// Sentences is the transcript as cards to read and search, sentence by
// sentence. A sentence is made of the lines the model reads, grouped the
// way the stories recipe groups them, so a sentence in the app is a
// sentence to the engine too: it ends where a line ends on a full stop, a
// question or an exclamation, or after sentenceSeconds of speech with
// none. A card is a sentence, or as much of it as cardSeconds holds, ended
// at a line, and never more than cardSecondsMost. A line holding the end
// of one sentence and the start of the next is two cards.
//
// A card is also cut at every time in cuts, the edges of the parts the
// clips keep, so a card is all in a clip or all out of it. A card of a
// clip held the end of the clip and then a quarter of a minute the clip
// leaves out, found by Tim. A word is on the side of a cut its middle is.
func Sentences(t *Transcript, cuts []float64) []SentenceView {
	cuts = slices.Sorted(slices.Values(cuts))
	lines := BuildLines(t.Words, nil, nil)
	out := make([]SentenceView, 0, len(lines)/2+1)
	var card []Cue
	end := func() {
		if len(card) == 0 {
			return
		}
		texts := make([]string, len(card))
		for i, c := range card {
			texts[i] = c.Text
		}
		out = append(out, SentenceView{Start: card[0].Start, End: card[len(card)-1].End,
			Text: Scrub(strings.Join(texts, " "), maxLineChars)})
		card = card[:0:0]
	}
	middle := func(c Cue) float64 { return (c.Start + c.End) / 2 }
	// Whether a cut falls between two words one after the other.
	cutBetween := func(a, b Cue) bool {
		i := sort.SearchFloat64s(cuts, middle(a))
		for i < len(cuts) && cuts[i] <= middle(a) {
			i++
		}
		return i < len(cuts) && cuts[i] <= middle(b)
	}
	for _, u := range sentenceUnits(lines) {
		for _, l := range lines[u[0] : u[1]+1] {
			for _, c := range l.Cues {
				if len(card) > 0 && (cutBetween(card[len(card)-1], c) || c.End-card[0].Start > cardSecondsMost) {
					end()
				}
				card = append(card, c)
				// A sentence ended inside a line ends its card too.
				if endsSentence(strings.TrimSpace(c.Text)) {
					end()
				}
			}
			if len(card) > 0 && card[len(card)-1].End-card[0].Start >= cardSeconds {
				end()
			}
		}
		end()
	}
	return out
}
