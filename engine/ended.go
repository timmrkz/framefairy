package engine

// How a search ended, worked out once here for every front end, so the
// interface only picks the words for it, see
// docs/decisions/2026-11-01-every-rule-of-the-workspace-in-go.md.
const (
	// EndedFound is a search done that wrote clips, or whose search it
	// carried on did.
	EndedFound = "found"
	// EndedNothing is a search done that found no moment beyond the
	// clips there are.
	EndedNothing = "nothing"
	// EndedShort is a search done that found nothing because the video
	// is shorter than Shortest, so it holds one clip, already found.
	EndedShort = "short"
	// EndedHearing is a search that stopped, was cut off or failed
	// before its window was heard: Continue hears the rest of it and
	// then finds.
	EndedHearing = "hearing"
	// EndedFinding is one that stopped, was cut off or failed once its
	// window was heard, while the clips were found: Continue looks again.
	EndedFinding = "finding"
)

// SearchEnded says how the search r ended. done is whether it is done,
// found how many clips it and the search it carried on wrote, heard the
// parts of the video its transcript holds and duration how long the
// video is. A search that is not over is told as if it stopped now.
func SearchEnded(r JobRecord, done bool, found int, heard [][2]float64, duration float64) string {
	if done {
		switch {
		case found > 0:
			return EndedFound
		case duration > 0 && duration < r.Min:
			return EndedShort
		default:
			return EndedNothing
		}
	}
	to := r.To
	if to <= 0 {
		to = duration
	}
	if heardOver(heard, r.From, to) {
		return EndedFinding
	}
	return EndedHearing
}

// heardOver says whether the parts heard cover from..to, all but half a
// second of it, the room a window cut at the end of the video leaves.
func heardOver(parts [][2]float64, from, to float64) bool {
	in := 0.0
	for _, p := range parts {
		a, b := max(p[0], from), min(p[1], to)
		if b > a {
			in += b - a
		}
	}
	return in >= to-from-0.5
}
