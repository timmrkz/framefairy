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
// hold is an edge that stays where it was put, because what put it there
// meant it to be a sentence's edge: In starts a clip made by hand at the
// beginning of the sentence the playhead stands in, and Out ends it at the
// end of that sentence. The model's clips hold neither.
type hold int

const (
	holdNeither hold = iota
	holdStart
	holdEnd
)

func wholeSentences(lines []Line, keep [][2]int, longest float64, seconds func([][2]int) float64,
	held hold) [][2]int {
	// Where a sentence begins or ends in line n, counted from 1, and when.
	// A line whose sentence begins or ends inside it, not at its edge, is a
	// boundary at that word, and cutKeep cuts the line there.
	begins := func(n int) (float64, bool) {
		k := sentenceStart(lines, n-1)
		if k < 0 {
			return 0, false
		}
		return lines[n-1].Cues[k].Start, true
	}
	finishes := func(n int) (float64, bool) {
		k := sentenceEnd(lines, n-1)
		if k < 0 {
			return 0, false
		}
		return lines[n-1].Cues[k].End, true
	}
	total := 0.0
	if seconds != nil {
		total = seconds(keep)
	}
	// tooLong is whether growing the clip by that much takes it past longest.
	tooLong := func(grows float64) bool {
		return seconds != nil && longest > 0 && total+grows > longest
	}
	var out [][2]int
	for r, run := range keep {
		first, last := run[0], run[1]
		if sentenceStart(lines, first-1) != 0 && !(held == holdStart && r == 0) {
			at := lines[first-1].Start()
			back, forward := -1, -1
			var backAt, forwardAt float64
			for n := first - 1; n >= 1; n-- {
				if t, ok := begins(n); ok {
					if at-t <= sentenceReach {
						back, backAt = n, t
					}
					break
				}
			}
			for n := first; n <= last; n++ {
				if t, ok := begins(n); ok && t > at {
					if t-at <= sentenceReach {
						forward, forwardAt = n, t
					}
					break
				}
			}
			switch {
			case back > 0 && forward > 0:
				first = back
				if forwardAt-at < at-backAt || tooLong(at-backAt) {
					first = forward
				}
			case back > 0:
				first = back
			case forward > 0:
				first = forward
			}
		}
		if k := sentenceEnd(lines, last-1); (k < 0 || k < len(lines[last-1].Cues)-1) &&
			!(held == holdEnd && r == len(keep)-1) {
			at := lines[last-1].End()
			back, forward := -1, -1
			var backAt, forwardAt float64
			for n := last; n >= first; n-- {
				if t, ok := finishes(n); ok && t < at {
					if at-t <= sentenceReach {
						back, backAt = n, t
					}
					break
				}
			}
			for n := last + 1; n <= len(lines); n++ {
				if t, ok := finishes(n); ok {
					if t-at <= sentenceReach {
						forward, forwardAt = n, t
					}
					break
				}
			}
			switch {
			case back > 0 && forward > 0:
				last = forward
				if at-backAt <= forwardAt-at || tooLong(forwardAt-at) {
					last = back
				}
			case back > 0:
				last = back
			case forward > 0:
				last = forward
			}
		}
		if n := len(out); n > 0 && first <= out[n-1][1] {
			out[n-1][1] = max(out[n-1][1], last)
			continue
		}
		out = append(out, [2]int{first, last})
	}
	return out
}

// sentenceStart is the word of line i, counted from 0, a sentence begins
// at: 0 when the line begins one, else the first word inside it after a
// sentence ends, and -1 when none begins in it. A line is laid out at
// pauses and lengths, not sentences, so a sentence can begin in the middle
// of one: "wie schnell sie das aufnehmen. Auf einer" is one line of Tim's
// episode, and the sentence it is about begins at "Auf". The first line of
// a transcript begins a sentence only at the start of the episode, since a
// part transcribed out of turn begins wherever it was asked to.
func sentenceStart(lines []Line, i int) int {
	if (i == 0 && lines[0].Start() < 1) || (i > 0 && endsSentence(strings.TrimSpace(lines[i-1].Text()))) {
		return 0
	}
	cues := lines[i].Cues
	for k := 1; k < len(cues); k++ {
		if endsSentence(strings.TrimSpace(cues[k-1].Text)) {
			return k
		}
	}
	return -1
}

// sentenceEnd is the word of line i a sentence ends at: its last when the
// line ends one, else the last word inside it that ends a sentence, and -1
// when none ends in it.
func sentenceEnd(lines []Line, i int) int {
	cues := lines[i].Cues
	for k := len(cues) - 1; k >= 0; k-- {
		if endsSentence(strings.TrimSpace(cues[k].Text)) {
			return k
		}
	}
	return -1
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
