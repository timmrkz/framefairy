package engine

import (
	"math"
	"math/rand/v2"
	"strings"
	"testing"

	"framefairy/engine/timed"
)

// An edge read only near the hand lands where it lands when every word
// and every caption of the clip is read, for a start and for an end,
// over words that run on, pauses that part captions, words too wide for
// a line, words removed and cuts.
func TestAnEdgeLandsTheSameReadOnlyNearTheHand(t *testing.T) {
	t.Parallel()
	s := new(Fonts).Style(nil)
	for seed := range uint64(40) {
		r := rand.New(rand.NewPCG(seed, 7))
		var words []timed.Cue
		at := 0.5
		for at < 400 {
			text := strings.Repeat("a", 2+r.IntN(8))
			switch k := r.IntN(40); {
			case k == 0:
				text = strings.Repeat("lang", 12)
			case k == 1:
				text = removedMark + text
			}
			long := 0.15 + r.Float64()*0.3
			words = append(words, timed.Cue{Start: at, End: at + long, Text: text})
			at += long + 0.02
			switch k := r.IntN(20); {
			case k < 3:
				at += captionPause + r.Float64()*2
			case k == 3:
				at += captionPause - 0.05
			case k == 4 && seed%5 == 0:
				at += 70
			}
		}
		tr := fromStored(words, nil, 0, 0, nil)
		clip := Clip{ID: "01", Segments: []Segment{{Start: 20, End: at - 30}}}
		if seed%2 == 1 {
			mid := (20 + at - 30) / 2
			clip.Segments = []Segment{{Start: 20, End: mid - 4}, {Start: mid - 1, End: mid}, {Start: mid + 3, End: at - 30}}
		}
		for range 60 {
			hand := 10 + r.Float64()*(at-10)
			want := SnapEnd(ShowWords(tr.Words, s, tr.Language), captionsGo(runOn(clip, hand), tr, s), hand, 0.1)
			near := wordsNear(tr, s, hand)
			if got := SnapEnd(near, goesNear(clip, tr, near, hand, s), hand, 0.1); math.Abs(got-want) > 1e-6 {
				t.Errorf("seed %d: an end at %.3f lands at %.3f, read whole at %.3f", seed, hand, got, want)
			}
			want = SnapStart(ShowWords(tr.Words, s, tr.Language), hand, 0.1)
			if got := SnapStart(wordsNear(tr, s, hand), hand, 0.1); math.Abs(got-want) > 1e-6 {
				t.Errorf("seed %d: a start at %.3f lands at %.3f, read whole at %.3f", seed, hand, got, want)
			}
		}
	}
}
