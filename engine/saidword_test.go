package engine

import (
	"math"
	"math/rand/v2"
	"testing"

	"framefairy/engine/timed"
)

// SaidWord looks only at the words either side of a moment, and gives the
// same word a look at every word gave, on clips with cuts and pauses,
// words that touch and words a breath apart. It looked at every word for
// every word of a clip's captions, so an hour's clip took 240 ms for each
// step of a drag, Tim's report of 11 November.
func TestSaidWordFindsTheWordEveryWordWouldHave(t *testing.T) {
	for seed := range uint64(20) {
		r := rand.New(rand.NewPCG(seed, 1))
		var words []timed.Cue
		at := 0.0
		for range 400 {
			at += []float64{0, 0.01, 0.02, 0.03, 0.4, 2}[r.IntN(6)]
			end := at + 0.05 + r.Float64()*0.6
			words = append(words, timed.Cue{Start: at, End: end, Text: "w"})
			at = end
		}
		clip := Clip{Segments: []Segment{{Start: 0, End: at / 3}, {Start: at / 2, End: at}}}
		said := Said(clip, words)
		for i := range 2000 {
			moment := float64(i) / 2000 * clipLength(clip)
			got, ok := SaidWord(clip, said, moment)
			want, wantOK := everyWord(clip, said, moment)
			if got != want || ok != wantOK {
				t.Fatalf("seed %d, at %.3f s: got %v %v, every word gives %v %v", seed, moment, got, ok, want, wantOK)
			}
		}
	}
}

// everyWord is SaidWord as it was, looking at every word.
func everyWord(clip Clip, words []timed.Cue, at float64) (timed.Cue, bool) {
	when := EpisodeTime(clip, at)
	near, off := timed.Cue{}, math.Inf(1)
	for _, w := range words {
		if when >= w.Start && when < w.End {
			return w, true
		}
		away := math.Max(w.Start-when, when-w.End)
		if away < off {
			near, off = w, away
		}
	}
	return near, off <= 0.02
}
