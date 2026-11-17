package engine

import (
	"encoding/binary"
	"math"
	"slices"
	"testing"
)

// The clip timeline keeps the loudness Loudness gives and draws every view
// from it, so what it holds is what a person sees: every reading where it
// was measured, and for zooming out the loudest of every two, four, eight
// and so on, so a column covering thousands of readings still shows their
// loudest.
func TestLoudnessKeepsEveryReadingAndTheLoudestOfEach(t *testing.T) {
	tr := &Transcript{Frames: []float32{-90, -60, -30, 0, -45.25, -100, 6, float32(math.NaN())}, Start: 0.02}
	got := Loudness(tr)
	if n := binary.LittleEndian.Uint32(got); n != 10 {
		t.Fatalf("it says %d readings, want the 8 and the 2 before the transcript starts", n)
	}
	readings := got[4:14]
	// Half a decibel a step from -90: -45.25 rounds to -45.0, below -90
	// is -90, above 0 is 0, and a reading that is no number is silence.
	want := []byte{0, 0, 0, 60, 120, 180, 90, 0, 180, 0}
	if !slices.Equal(readings, want) {
		t.Errorf("readings %v, want %v", readings, want)
	}
	// Each level the loudest of two of the one before, the last alone
	// kept as it is: 10, 5, 3, 2, 1.
	levels := [][]byte{{0, 60, 180, 90, 180}, {60, 180, 180}, {180, 180}, {180}}
	at := 14
	for k, level := range levels {
		if !slices.Equal(got[at:at+len(level)], level) {
			t.Errorf("level %d is %v, want %v", k+1, got[at:at+len(level)], level)
		}
		at += len(level)
	}
	if at != len(got) {
		t.Errorf("%d bytes, want %d", len(got), at)
	}
}

// A video with nothing measured yet has a loudness of no readings, which
// the clip timeline draws as nothing, not as a failure.
func TestLoudnessOfNothing(t *testing.T) {
	for _, tr := range []*Transcript{nil, {}} {
		if got := Loudness(tr); len(got) != 4 || binary.LittleEndian.Uint32(got) != 0 {
			t.Errorf("nothing measured gives %v", got)
		}
	}
	if got := Loudness(&Transcript{Frames: []float32{-30}}); !slices.Equal(got, []byte{1, 0, 0, 0, 120}) {
		t.Errorf("one reading gives %v", got)
	}
}

// Two hours of readings, a frame of 10 ms each, the longest video the
// decision record weighs.
func BenchmarkLoudnessTwoHours(b *testing.B) {
	tr := &Transcript{Frames: make([]float32, 720_000)}
	for i := range tr.Frames {
		tr.Frames[i] = float32(-60 + i%50)
	}
	b.SetBytes(int64(len(tr.Frames)))
	for b.Loop() {
		Loudness(tr)
	}
}
