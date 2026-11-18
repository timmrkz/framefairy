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
	// The head says how to read the rest: a reading is 10000 µs, and its
	// byte is half decibels up from -90 dB.
	if each, floor, steps := binary.LittleEndian.Uint16(got[4:]), int8(got[6]), got[7]; each != 10000 || floor != -90 || steps != 2 {
		t.Errorf("the head says a reading is %d µs, from %d dB in %d steps a decibel", each, floor, steps)
	}
	readings := got[8:18]
	// Half a decibel a step from -90: -45.25 rounds to -45.0, below -90
	// is -90, above 0 is 0, and a reading that is no number is silence.
	want := []byte{0, 0, 0, 60, 120, 180, 90, 0, 180, 0}
	if !slices.Equal(readings, want) {
		t.Errorf("readings %v, want %v", readings, want)
	}
	// Each level the loudest of two of the one before, the last alone
	// kept as it is: 10, 5, 3, 2, 1.
	levels := [][]byte{{0, 60, 180, 90, 180}, {60, 180, 180}, {180, 180}, {180}}
	at := 18
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
		if got := Loudness(tr); len(got) != 8 || binary.LittleEndian.Uint32(got) != 0 {
			t.Errorf("nothing measured gives %v", got)
		}
	}
	if got := Loudness(&Transcript{Frames: []float32{-30}}); !slices.Equal(got, []byte{1, 0, 0, 0, 0x10, 0x27, 0xa6, 2, 120}) {
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

// The clip timeline reads the loudness again when the stamp changes and
// only then, so it changes with everything Loudness gives and with
// nothing else: more measured, more heard while the measuring has not
// reached the end, and not what is heard once it has.
func TestTheLoudnessStampChangesWithTheLoudness(t *testing.T) {
	some := loudnessStamp([][2]float64{{0, 10}}, false, nil)
	if more := loudnessStamp([][2]float64{{0, 12}}, false, nil); more == some {
		t.Error("more measured kept the stamp")
	}
	if heard := loudnessStamp([][2]float64{{0, 10}}, false, [][2]float64{{0, 30}}); heard == some {
		t.Error("more heard before the end was measured kept the stamp")
	}
	all := loudnessStamp([][2]float64{{0, 60}}, true, [][2]float64{{0, 30}})
	if heard := loudnessStamp([][2]float64{{0, 60}}, true, [][2]float64{{0, 45}}); heard != all {
		t.Errorf("more heard once all was measured changed the stamp, %q to %q", all, heard)
	}
	if again := loudnessStamp([][2]float64{{0, 10}}, false, nil); again != some {
		t.Errorf("the same loudness gave %q and %q", some, again)
	}
}
