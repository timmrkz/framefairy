package engine

import "strings"

// SentenceView is one sentence of what an episode says, for reading the
// transcript in the app and searching it: when it is said and its words.
type SentenceView struct {
	Start float64 `json:"start"`
	End   float64 `json:"end"`
	Text  string  `json:"text"`
}

// Sentences is the transcript sentence by sentence. A sentence is made of
// the lines the model reads, grouped the way the stories recipe groups
// them, so a sentence in the app is a sentence to the engine too: it ends
// where a line ends on a full stop, a question or an exclamation, or after
// sentenceSeconds of speech with none.
func Sentences(t *Transcript) []SentenceView {
	lines := BuildLines(t.Words, nil, nil)
	out := make([]SentenceView, 0, len(lines)/2+1)
	for _, u := range sentenceUnits(lines) {
		texts := make([]string, 0, u[1]-u[0]+1)
		for _, l := range lines[u[0] : u[1]+1] {
			texts = append(texts, l.Text())
		}
		out = append(out, SentenceView{Start: lines[u[0]].Start(), End: lines[u[1]].End(),
			Text: strings.Join(texts, " ")})
	}
	return out
}
