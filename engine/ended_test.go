package engine

import "testing"

// How a search ended is said once, by the engine, and the interface only
// picks the words: found, nothing, nothing because the video is shorter
// than Shortest, and stopped while its window was heard or while the
// clips were found.
func TestHowASearchEnded(t *testing.T) {
	r := JobRecord{Kind: JobSearch, From: 600, To: 1200, Min: 20}
	all := [][2]float64{{0, 3600}}
	for _, c := range []struct {
		name     string
		r        JobRecord
		done     bool
		found    int
		heard    [][2]float64
		duration float64
		want     string
	}{
		{"found clips", r, true, 3, all, 3600, EndedFound},
		{"found nothing new", r, true, 0, all, 3600, EndedNothing},
		{"a video shorter than Shortest", JobRecord{Kind: JobSearch, Min: 20}, true, 0, all, 12, EndedShort},
		{"stopped before its window was heard", r, false, 0, [][2]float64{{0, 900}}, 3600, EndedHearing},
		{"stopped while the clips were found", r, false, 0, all, 3600, EndedFinding},
		{"heard in two parts, all but a moment", r, false, 0, [][2]float64{{0, 900}, {900.2, 1199.8}}, 3600, EndedFinding},
		{"a window to the end of the video", JobRecord{Kind: JobSearch, From: 3000}, false, 0, [][2]float64{{0, 3300}}, 3600, EndedHearing},
	} {
		if got := SearchEnded(c.r, c.done, c.found, c.heard, c.duration); got != c.want {
			t.Errorf("%s: %q, want %q", c.name, got, c.want)
		}
	}
}
