package engine

import "math"

// LandOn is where a step of the arrow keys lands on each of these words,
// in the order the captions light them, each lit from its first moment
// to its second on the episode's clock, and where each begins as a place
// to land on. A step goes a frame into a word, never onto its edge, or to
// the middle of its first frame for a word lit on frames of the short,
// framed, whose frames are known: that frame shows it at rest, before its
// pop, see popScales. A word shorter than two frames is entered by half
// of itself.
//
// Words lit together on one frame begin on it together, see litRuns: the
// first of them is the place, and one that goes on lit alone after the
// others is a place from where they end. Two words begun on one frame
// were two places on one frame, and a step back from the second landed
// on the frame it stood on. A word that is no place of its own, or is
// lit on no frame, lands at -1.
func LandOn(spans [][2]float64, frame float64, framed bool) (land, since []float64) {
	land, since = make([]float64, len(spans)), make([]float64, len(spans))
	in := frame
	if framed {
		in = frame / 2
	}
	var before *[2]float64
	for i, w := range spans {
		land[i], since[i] = -1, -1
		if w[0] < 0 || w[1] < w[0] {
			continue
		}
		from := w[0]
		if before != nil && w[0] == before[0] {
			if w[1] <= before[1] {
				continue
			}
			from = before[1]
		}
		since[i] = from
		land[i] = math.Min(from+in, (from+w[1])/2)
		before = &[2]float64{from, w[1]}
	}
	return land, since
}
