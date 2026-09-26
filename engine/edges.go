package engine

import "strings"

// ---------------------------------------------------------------------------
// Whole sentences
//
// A model that has found a story is still loose about its edges. The same
// story came back from five searches with three different first words and
// four different last ones, and most of them in the middle of a sentence:
// the line it stopped on ended on a comma, and the sentence went on over
// three more. A short that starts or stops mid-sentence sounds cut off,
// however good the story is.
//
// So every edge of a clip, its start, its end and the cuts inside it, is
// moved onto the nearest place a sentence begins or ends, as long as that
// is no more than sentenceReach away. Further than that the model's edge
// stands, because a transcript without punctuation for a while has no
// sentence to move to. The moment is the model's. Where it begins and ends,
// to the word, is the engine's.
// ---------------------------------------------------------------------------

// sentenceReach is the furthest an edge moves to land on a sentence, in
// seconds of speech and pause.
const sentenceReach = 8.0

// wholeSentences moves the edges of each run, lines numbered from 1, onto
// sentence boundaries, and makes one run of runs that then overlap.
func wholeSentences(lines []Line, keep [][2]int) [][2]int {
	ends := func(n int) bool { return endsSentence(strings.TrimSpace(lines[n-1].Text())) }
	starts := func(n int) bool { return n == 1 || ends(n-1) }
	var out [][2]int
	for _, run := range keep {
		first, last := run[0], run[1]
		if !starts(first) {
			back, forward := -1, -1
			for n := first - 1; n >= 1 && lines[first-1].Start()-lines[n-1].Start() <= sentenceReach; n-- {
				if starts(n) {
					back = n
					break
				}
			}
			for n := first + 1; n <= last && lines[n-1].Start()-lines[first-1].Start() <= sentenceReach; n++ {
				if starts(n) {
					forward = n
					break
				}
			}
			first = nearer(first, back, forward, func(n int) float64 { return lines[n-1].Start() })
		}
		if !ends(last) {
			back, forward := -1, -1
			for n := last + 1; n <= len(lines) && lines[n-1].End()-lines[last-1].End() <= sentenceReach; n++ {
				if ends(n) {
					forward = n
					break
				}
			}
			for n := last - 1; n >= first && lines[last-1].End()-lines[n-1].End() <= sentenceReach; n-- {
				if ends(n) {
					back = n
					break
				}
			}
			last = nearer(last, back, forward, func(n int) float64 { return lines[n-1].End() })
		}
		if n := len(out); n > 0 && first <= out[n-1][1] {
			out[n-1][1] = max(out[n-1][1], last)
			continue
		}
		out = append(out, [2]int{first, last})
	}
	return out
}

// nearer is whichever of back and forward, -1 for none, lies nearer to at
// in time, or at itself when there is neither.
func nearer(at, back, forward int, time func(int) float64) int {
	switch {
	case back < 0 && forward < 0:
		return at
	case back < 0:
		return forward
	case forward < 0:
		return back
	}
	if time(at)-time(back) <= time(forward)-time(at) {
		return back
	}
	return forward
}
