package engine

import "testing"

// How a video stands in the sidebar, decided here and drawn there: a file
// gone or a failed search first, then what needs a look, then clips made.
func TestAVideoStandsInTheSidebarAsItsFilesAndSearchSay(t *testing.T) {
	plans := []PlanSummary{{}}
	for _, c := range []struct {
		name   string
		st     EpisodeStatus
		search int
		want   string
	}{
		{"a new video", EpisodeStatus{}, SearchQuiet, ""},
		{"a video with clips", EpisodeStatus{Plans: plans}, SearchQuiet, StandsMade},
		{"a file gone, with clips", EpisodeStatus{Missing: true, Plans: plans}, SearchQuiet, StandsFailed},
		{"a failed search, with clips", EpisodeStatus{Plans: plans}, SearchFailed, StandsFailed},
		{"a search that stopped, with clips", EpisodeStatus{Plans: plans}, SearchStopped, StandsLook},
		{"a file changed since it was transcribed", EpisodeStatus{TranscriptStale: true, Plans: plans}, SearchQuiet, StandsLook},
		{"a file gone with a search that stopped", EpisodeStatus{Missing: true}, SearchStopped, StandsFailed},
		{"a failed search on a changed file", EpisodeStatus{TranscriptStale: true}, SearchFailed, StandsFailed},
	} {
		if got := c.st.Standing(c.search); got != c.want {
			t.Errorf("%s stands as %q, want %q", c.name, got, c.want)
		}
	}
}

// The line under a video's name says how far it is transcribed, as said
// here: whole, changed since, from the start to a moment, in parts, or
// not at all.
func TestTheSidebarSaysHowFarAVideoIsTranscribed(t *testing.T) {
	for _, c := range []struct {
		name string
		st   EpisodeStatus
		want string
	}{
		{"whole", EpisodeStatus{Transcribed: true, Heard: [][2]float64{{0, 60}}, Covered: 60}, TranscriptWhole},
		{"changed since", EpisodeStatus{TranscriptStale: true}, TranscriptChanged},
		{"from the start", EpisodeStatus{Heard: [][2]float64{{0, 40}}, Covered: 40}, TranscriptTo},
		{"one part not at the start", EpisodeStatus{Heard: [][2]float64{{20, 40}}}, TranscriptParts},
		{"two parts", EpisodeStatus{Heard: [][2]float64{{0, 10}, {20, 40}}, Covered: 10}, TranscriptParts},
		{"nothing", EpisodeStatus{Heard: [][2]float64{}}, TranscriptNone},
	} {
		if got := c.st.transcript(); got != c.want {
			t.Errorf("a video transcribed %s says %q, want %q", c.name, got, c.want)
		}
	}
}
