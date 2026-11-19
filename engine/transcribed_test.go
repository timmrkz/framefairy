package engine

import "testing"

// A video's status says how far it is transcribed: whole, changed since,
// from the start to a moment, in parts, or not at all. The line under its
// name in the sidebar only words it.
func TestAStatusSaysHowFarAVideoIsTranscribed(t *testing.T) {
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
