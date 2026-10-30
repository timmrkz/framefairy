package engine

import (
	"cmp"
	"slices"
	"sort"
	"strings"

	"framefairy/engine/timed"
)

// SentenceView is one card of what an episode says, for reading the
// transcript in the app and searching it: when it is said and its words.
type SentenceView struct {
	Start float64 `json:"start"`
	End   float64 `json:"end"`
	Text  string  `json:"text"`
	// Clip is the key of the clip that shows the card, see ClipSaid, and
	// empty for what no clip keeps.
	Clip string `json:"clip,omitempty"`
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
	var card []timed.Cue
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
	middle := func(c timed.Cue) float64 { return (c.Start + c.End) / 2 }
	// Whether a cut falls between two words one after the other.
	cutBetween := func(a, b timed.Cue) bool {
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

// ClipSaid is one clip among the cards of what was said: the name the
// interface knows it by, the clip, and the captions it shows, on its own
// clock, see CaptionsView.
type ClipSaid struct {
	Key      string
	Clip     Clip
	Captions []CaptionView
}

// Said is every clip of the summary's plan that is not rejected, with the
// captions it shows in look, for SaidCards, from the plan the summary read
// already. The key of each is its ID. The plan was read a second time for
// the cards, every time they were read, and a clip set removed between the
// two reads failed the whole list, found in the review of the search.
func (s PlanSummary) Said(t *Transcript, look *Look) []ClipSaid {
	out := make([]ClipSaid, 0, len(s.clips))
	for _, c := range s.clips {
		if c.Rejected || len(c.Segments) == 0 {
			continue
		}
		out = append(out, ClipSaid{Key: c.ID, Clip: c, Captions: captionsView(s.plan, c, t, look).Captions})
	}
	return out
}

// SaidCards is the transcript as cards to read and search, see Sentences,
// with what the clips show in place of what they keep: a card for every
// caption of a clip, named for the clip, with the words the caption shows,
// corrections and all. A caption is one block in the video preview and on
// the clip timeline, so it is one card here too, and a word corrected in
// the caption box reads corrected here, found by Tim. What no clip keeps
// is in cards of sentences, cut at the edges of the clips. Where two clips
// keep the same words, each has its cards of them, because each shows them
// in captions of its own.
func SaidCards(t *Transcript, clips []ClipSaid) []SentenceView {
	var cuts []float64
	for _, c := range clips {
		for _, s := range c.Clip.Segments {
			cuts = append(cuts, s.Start, s.End)
		}
	}
	// What the clips say is theirs, a word only some of whose sound a
	// piece holds among it, see Said.
	said := map[timed.Cue]bool{}
	for _, c := range clips {
		for _, w := range Said(c.Clip, t.Words) {
			said[w] = true
		}
	}
	rest := &Transcript{Words: slices.DeleteFunc(slices.Clone(t.Words), func(w timed.Cue) bool { return said[w] })}
	out := Sentences(rest, cuts)
	for _, c := range clips {
		for _, caption := range c.Captions {
			if card, ok := captionCard(c, caption); ok {
				out = append(out, card)
			}
		}
	}
	slices.SortStableFunc(out, func(a, b SentenceView) int { return cmp.Compare(a.Start, b.Start) })
	if out == nil {
		out = []SentenceView{}
	}
	return out
}

// captionCard is one caption of a clip as a card, on the episode's clock:
// from where its first word starts to where its last word ends.
func captionCard(c ClipSaid, caption CaptionView) (SentenceView, bool) {
	var words []WordView
	for _, line := range caption.Lines {
		words = append(words, line.Words...)
	}
	if len(words) == 0 {
		return SentenceView{}, false
	}
	// A word drawn in halves, hyphenated at the end of a line, is one word
	// of the card. So is a part of a correction that reads as several.
	same := func(a, b WordView) bool {
		return a.Said != nil && b.Said != nil && *a.Said == *b.Said &&
			(a.Part == nil) == (b.Part == nil) && (a.Part == nil || *a.Part == *b.Part)
	}
	var texts []string
	for i := 0; i < len(words); {
		j := i + 1
		for j < len(words) && same(words[i], words[j]) {
			j++
		}
		text := words[i].Text
		if j-i > 1 {
			text = words[i].Whole
			if p := words[i].Part; p != nil {
				fields := strings.Fields(words[i].Whole)
				text = ""
				if *p < len(fields) {
					text = fields[*p]
				}
			}
			if text == "" {
				var halves []string
				for _, w := range words[i:j] {
					halves = append(halves, w.Text)
				}
				text = strings.Join(halves, "")
			}
		}
		texts = append(texts, text)
		i = j
	}
	start := EpisodeTime(c.Clip, words[0].Start)
	end := episodeEnd(c.Clip, words[len(words)-1].End)
	return SentenceView{Start: start, End: max(end, start), Clip: c.Key,
		Text: Scrub(strings.Join(texts, " "), maxLineChars)}, true
}

// episodeEnd is the moment of the episode a moment of a clip ends on: the
// end of a piece is the end of that piece, not the start of the next.
func episodeEnd(clip Clip, at float64) float64 {
	offset := 0.0
	for _, s := range clip.Segments {
		if at <= offset+s.Duration() {
			return s.Start + max(at-offset, 0)
		}
		offset += s.Duration()
	}
	return EpisodeTime(clip, at)
}
