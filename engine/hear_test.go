package engine

import (
	"math"
	"testing"
)

// dictated is a recording of a hum over room noise, with the hum stopping
// for each of the quiet stretches given, in seconds.
func dictated(seconds float64, quiet ...[2]float64) []float32 {
	samples := make([]float32, int(seconds*SampleRate))
	for i := range samples {
		at := float64(i) / SampleRate
		// Room noise, about 50 dB below the voice.
		samples[i] = float32(0.001 * math.Sin(2*math.Pi*3001*at))
		still := false
		for _, q := range quiet {
			still = still || (at >= q[0] && at < q[1])
		}
		if !still {
			samples[i] += float32(0.3 * math.Sin(2*math.Pi*220*at))
		}
	}
	return samples
}

// While somebody dictates, what they said before a pause is written down
// for good and what follows is still being heard. The pause has to be a
// real one, long enough that a sentence has ended, and has to come after
// something was said. Without one, a piece is settled before it grows
// past what the speech model takes in one go. Plan row 2.189.
func TestSettledAtAPause(t *testing.T) {
	near := func(t *testing.T, got int, want float64) {
		t.Helper()
		if math.Abs(float64(got)/SampleRate-want) > 0.15 {
			t.Errorf("settled at %.2f s, want about %.2f s", float64(got)/SampleRate, want)
		}
	}
	// A pause from 3 s to 4 s settles in its middle.
	near(t, Settled(dictated(6, [2]float64{3, 4})), 3.5)
	// The last of two pauses.
	near(t, Settled(dictated(9, [2]float64{2, 3}, [2]float64{5, 6})), 5.5)
	// A pause still going on at the end settles a little way into it.
	near(t, Settled(dictated(4, [2]float64{3, 4})), 3.3)
	// A breath between two words is no pause.
	if at := Settled(dictated(6, [2]float64{3, 3.3})); at != 0 {
		t.Errorf("a breath settled at %.2f s", float64(at)/SampleRate)
	}
	// Quiet before anything was said settles nothing, and neither does
	// room noise alone.
	if at := Settled(dictated(6, [2]float64{0, 2})); at != 0 {
		t.Errorf("quiet before the first word settled at %.2f s", float64(at)/SampleRate)
	}
	if at := Settled(dictated(6, [2]float64{0, 6})); at != 0 {
		t.Errorf("room noise settled at %.2f s", float64(at)/SampleRate)
	}
	if at := Settled(nil); at != 0 {
		t.Errorf("nothing settled at %d", at)
	}
	// Talking on without a pause is settled at the quietest moment before
	// the piece is longer than the speech model takes.
	long := dictated(dictateLongest+1, [2]float64{dictateLongest - 3, dictateLongest - 2.8})
	near(t, Settled(long), dictateLongest-2.9)
	if at := Settled(dictated(dictateLongest - 1)); at != 0 {
		t.Errorf("%.0f s without a pause settled at %.2f s", dictateLongest-1, float64(at)/SampleRate)
	}
}
