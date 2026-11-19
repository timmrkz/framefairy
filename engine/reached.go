package engine

import (
	"math"
	"slices"
)

// What of a video has been heard, which is not the same as what its saved
// transcript holds. Saving rewrites the whole transcript, so it happens
// seconds apart and jumps minutes of audio at a time, while every chunk
// the recogniser finishes says where it got to, see Engine.transcribe. The
// range picker draws what has been heard, so its edge moves with the work.
//
// It only ever grows, because it says how much of the video has been read,
// and reading does not unhappen. The moment a hearing stops, what it
// reached and had not saved yet is still heard: those seconds were heard,
// they were only never written down, and the edge walked backwards by
// them when the window kept the mark, Tim's report on pausing. And a
// hearing carried on after a pause reports from where the saved transcript
// ends, behind the mark, which is no step back either. The window kept
// this mark itself once, a copy of the engine's knowledge of what it
// heard, #610.

// reach notes that a hearing of the video has heard from from to to, as
// it goes, before it saves.
func (ep *Episode) reach(from, to float64) {
	ep.m.held.Lock()
	defer ep.m.held.Unlock()
	ep.hearing = &[2]float64{from, to}
	ep.reached = joinParts(append(ep.reached, *ep.hearing))
}

// reachEnds notes that the hearing that reached is over, saved or let go.
// What it reached stays heard.
func (ep *Episode) reachEnds() {
	ep.m.held.Lock()
	defer ep.m.held.Unlock()
	ep.hearing = nil
}

// Reached is every part of the video heard, saved or reached by a hearing
// and not saved yet, in order and apart: saved, the parts its transcript
// on disk holds, with what the hearings reached, see reach. It never
// shrinks, except when restart says what was heard is about to mean
// nothing: a transcript out of date that will be read again, or a work
// folder no longer there. Then it starts over from saved and what the
// hearing running now has reached.
func (ep *Episode) Reached(saved [][2]float64, restart bool) [][2]float64 {
	ep.m.held.Lock()
	defer ep.m.held.Unlock()
	if restart {
		ep.reached = nil
		if ep.hearing != nil {
			ep.reached = [][2]float64{*ep.hearing}
		}
	}
	ep.reached = joinParts(append(ep.reached, saved...))
	return slices.Clone(ep.reached)
}

// HeardOf is how many seconds of from..to the parts hold.
func HeardOf(parts [][2]float64, from, to float64) float64 {
	sum := 0.0
	for _, p := range parts {
		sum += math.Max(math.Min(p[1], to)-math.Max(p[0], from), 0)
	}
	return sum
}

// HeardOver says whether the parts hold from..to, all but the half second
// the rest of the app allows an edge, the room a window cut at the end of
// the video leaves.
func HeardOver(parts [][2]float64, from, to float64) bool {
	return HeardOf(parts, from, to) >= to-from-0.5
}

// NotHeard is what of a video of duration seconds the parts heard do not
// hold, the parts a search cannot look in, leaving out a gap under a
// second, which is between two words.
func NotHeard(heard [][2]float64, duration float64) [][2]float64 {
	return slices.DeleteFunc(unheardIn(heard, duration), func(p [2]float64) bool { return p[1]-p[0] < 1 })
}
