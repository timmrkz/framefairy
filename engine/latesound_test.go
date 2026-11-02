package engine

import (
	"bytes"
	"encoding/binary"
	"math"
	"os/exec"
	"testing"

	"framefairy/engine/runlog"
	"framefairy/engine/timed"
	"framefairy/internal/ffmpegtest"
	"framefairy/internal/fixtures"
)

// A reading of the sound resumed before a late picture starts hears the
// sound that is there, not the sound from where the picture starts. ffmpeg
// seeks every stream to the key frame of the picture, so a seek to before
// the picture starts hands back sound only from where the picture starts,
// and what was wanted before it was missing and the rest out of step.
// Plan row 2.138.
//
// On the real disk: ffmpeg reads the episode.
func TestSoundResumedBeforeALatePictureIsTheSoundThere(t *testing.T) {
	t.Parallel()
	ffmpegtest.Need(t)
	// A tone whose pitch climbs, so sound from another moment does not
	// match, and a picture a second late, by an edit list.
	path := fixtures.Path(t, "sound-first.mov")
	e := NewEngine(runlog.New(nil, false, false), NewMachine(disk))
	e.FFmpeg, e.FFprobe = "ffmpeg", "ffprobe"
	picture := e.pictureStart(t.Context(), path)
	if math.Abs(picture-1) > 0.001 {
		t.Fatalf("the picture starts at %.3f, not 1.000", picture)
	}
	read := func(start int) []float32 {
		t.Helper()
		raw, err := exec.Command("ffmpeg", audioFrom(path, start, picture)...).Output()
		if err != nil {
			t.Fatal(err)
		}
		samples := make([]float32, len(raw)/4)
		if err := binary.Read(bytes.NewReader(raw), binary.LittleEndian, samples); err != nil {
			t.Fatal(err)
		}
		return samples
	}
	whole := read(0)
	// How far the sound read from start is shifted against the same sound
	// read whole, in samples: the shift where the two differ least, past
	// the first moments a decoder takes to settle. A sample a millisecond
	// away on a climbing tone is far off, so being in step is plain.
	shift := func(got, want []float32) int {
		best, least := 0, math.Inf(1)
		for lag := -2000; lag <= 2000; lag++ {
			sum := 0.0
			for i := 2000; i < 6000; i += 3 {
				j := i + lag
				if j < 0 || j >= len(want) || i >= len(got) {
					sum = math.Inf(1)
					break
				}
				sum += math.Abs(float64(got[i] - want[j]))
			}
			if sum < least {
				best, least = lag, sum
			}
		}
		return best
	}
	// Readings that want sound from before the picture starts at 1 s, past
	// the lead a reading decodes and throws away, which a seek lost, and
	// two that want it after.
	for _, start := range []int{levelsLead + 10, 50, 90, 120, 200} {
		got := read(start)
		want := whole[start*SampleRate/100:]
		if lag := shift(got, want); lag != 0 || len(got) != len(want) {
			t.Errorf("read from %.2f s: %d samples where %d are left, %d samples out of step, %.1f ms",
				float64(start)*timed.FrameSeconds, len(got), len(want), lag, float64(lag)*1000/SampleRate)
		}
	}
}
