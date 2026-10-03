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
//
// An edge moves to the nearer boundary, unless that would take the clip
// past longest seconds and the other would not. A story the model stopped
// on a comma, one sentence after its payoff, then ends on the payoff
// rather than running on into the next thought: on Tim's episode the
// nearer boundary made the umbrella story 35 seconds long, and the length
// limit then cut the payoff out of its middle. seconds measures a clip.
// Without it, the nearer boundary is always taken.
func wholeSentences(lines []Line, keep [][2]int, longest float64, seconds func([][2]int) float64) [][2]int {
	ends := func(n int) bool { return finishesSentence(lines, n) }
	starts := func(n int) bool { return beginsSentence(lines, n) }
	total := 0.0
	if seconds != nil {
		total = seconds(keep)
	}
	// tooLong is whether growing the clip by that much takes it past longest.
	tooLong := func(grows float64) bool {
		return seconds != nil && longest > 0 && total+grows > longest
	}
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
			chosen := nearer(first, back, forward, func(n int) float64 { return lines[n-1].Start() })
			if chosen == back && forward > 0 && tooLong(lines[first-1].Start()-lines[back-1].Start()) {
				chosen = forward
			}
			first = chosen
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
			chosen := nearer(last, back, forward, func(n int) float64 { return lines[n-1].End() })
			if chosen == forward && back > 0 && tooLong(lines[forward-1].End()-lines[last-1].End()) {
				chosen = back
			}
			last = chosen
		}
		if n := len(out); n > 0 && first <= out[n-1][1] {
			out[n-1][1] = max(out[n-1][1], last)
			continue
		}
		out = append(out, [2]int{first, last})
	}
	return out
}

// sentenceStart is the line where the sentence of line n begins, and
// sentenceEnd the line where it ends, lines numbered from 1, no further off
// than sentenceReach, the reach every clip's edges have. A transcript
// without punctuation for that long has no sentence to go to, and the
// line itself stands.
func sentenceStart(lines []Line, n int) int {
	for k := n; k >= 1 && lines[n-1].Start()-lines[k-1].Start() <= sentenceReach; k-- {
		if beginsSentence(lines, k) {
			return k
		}
	}
	return n
}

func sentenceEnd(lines []Line, n int) int {
	for k := n; k <= len(lines) && lines[k-1].End()-lines[n-1].End() <= sentenceReach; k++ {
		if finishesSentence(lines, k) {
			return k
		}
	}
	return n
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

// beginsSentence says whether line n, counted from 1, begins a sentence:
// the first line does, and every line after one that ends a sentence.
func beginsSentence(lines []Line, n int) bool {
	return n == 1 || finishesSentence(lines, n-1)
}

// finishesSentence says whether line n, counted from 1, ends a sentence.
func finishesSentence(lines []Line, n int) bool {
	return endsSentence(strings.TrimSpace(lines[n-1].Text()))
}
