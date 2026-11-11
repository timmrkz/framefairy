package engine

import (
	"fmt"
	"math"
	"math/rand/v2"
	"testing"

	"framefairy/engine/timed"
)

// The word said at a moment is found by halving the words when they are
// in order, and it is the same word looking at them one by one finds: the
// word the moment is in, or the nearest, the first of two as near. Words
// out of order are looked at one by one. The words are made up from
// fixed seeds, with words that touch, words of no length and words that
// end together, and the moments fall on their edges as often as between.
func TestTheWordSaidIsFoundTheSameByHalving(t *testing.T) {
	clip := Clip{Segments: []Segment{{Start: 0, End: 60}}}
	for seed := range uint64(200) {
		r := rand.New(rand.NewPCG(seed, 7))
		var words []timed.Cue
		at := 0.0
		for range r.IntN(40) {
			at += float64(r.IntN(4)) * 0.01
			end := at + float64(r.IntN(5))*0.01
			if len(words) > 0 {
				end = math.Max(end, words[len(words)-1].End)
			}
			words = append(words, timed.Cue{Start: at, End: end, Text: fmt.Sprint(len(words))})
			at = math.Max(at, end-0.02)
		}
		said := saidWordsOf(words)
		if !said.ordered {
			t.Fatalf("seed %d: words in order were taken as out of order", seed)
		}
		var moments []float64
		for _, w := range words {
			moments = append(moments, w.Start, w.End, w.Start-0.02, w.End+0.02, w.End+0.021)
		}
		for range 50 {
			moments = append(moments, r.Float64()*(at+0.5)-0.25)
		}
		for _, m := range moments {
			got, gotOK := said.at(clip, m)
			want, wantOK := wordAt(words, EpisodeTime(clip, m))
			if got != want || gotOK != wantOK {
				t.Fatalf("seed %d at %.3f: halving found %v %v, one by one %v %v", seed, m, got, gotOK, want, wantOK)
			}
		}
	}
	overlapping := []timed.Cue{{Start: 1, End: 3}, {Start: 2, End: 2.5}}
	if saidWordsOf(overlapping).ordered {
		t.Error("a word inside the one before was taken as in order")
	}
}
