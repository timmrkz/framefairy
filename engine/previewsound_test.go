package engine

import (
	"bytes"
	"context"
	"encoding/binary"
	"math"
	"os/exec"
	"path/filepath"
	"testing"

	"framefairy/internal/ffmpegtest"
)

// The sound the video preview hears from any moment is ffmpeg's decode of
// the episode there, to the sample and in every channel, the same sound
// the render and the transcript are made of. Read from moments that fall
// between samples' worth of milliseconds, from the start, and from before
// a late picture starts, where a seek alone finds the sound only from the
// picture on.
func TestPreviewSoundIsTheEpisodesOwnFromAnyMoment(t *testing.T) {
	ffmpegtest.Need(t)
	path := filepath.Join(t.TempDir(), "late.mov")
	// Two channels that differ, a tone that climbs in the left and one that
	// falls in the right, so a sample from another moment or a channel
	// swapped does not match. The picture a second late, by an edit list.
	out, err := exec.Command("ffmpeg", "-loglevel", "error", "-y",
		"-itsoffset", "1", "-f", "lavfi", "-i", "color=c=black:s=320x180:r=25:d=5",
		"-f", "lavfi", "-i", "aevalsrc=sin(2*PI*(200*t+300*t*t))|0.5*sin(2*PI*(900*t-40*t*t)):s=48000:d=6",
		"-map", "0", "-map", "1", "-fps_mode", "passthrough", "-g", "50",
		"-c:v", "mpeg4", "-q:v", "5", "-c:a", "aac", path).CombinedOutput()
	if err != nil {
		ffmpegtest.Unusable(t, "ffmpeg could not make an episode: %s %s", err, out)
	}
	e := NewEngine(NewLog(nil, false, false))
	e.FFmpeg, e.FFprobe = "ffmpeg", "ffprobe"
	const rate, channels = 48000, 2

	// The whole sound, decoded from the start of the file.
	raw, err := exec.Command("ffmpeg", "-v", "error", "-i", path, "-map", "0:a:0",
		"-ac", "2", "-ar", "48000", "-f", "f32le", "-").Output()
	if err != nil {
		t.Fatal(err)
	}
	whole := floats(t, raw)

	for _, sample := range []int{0, 101, 30001, 47999, 96017, 120000} {
		from := float64(sample) / rate
		var got []float32
		var moments []float64
		err := e.PreviewSound(context.Background(), path, from, rate, channels, func(at float64, chunk []byte) error {
			moments = append(moments, at)
			got = append(got, floats(t, chunk)...)
			if len(chunk)%(channels*4) != 0 {
				t.Errorf("from sample %d a chunk of %d bytes holds no whole moments", sample, len(chunk))
			}
			return nil
		})
		if err != nil {
			t.Fatal(err)
		}
		want := whole[sample*channels:]
		if len(got) != len(want) {
			t.Errorf("from sample %d: %d samples where %d are left", sample, len(got)/channels, len(want)/channels)
		}
		// The decoder settles within the lead it decodes and throws away,
		// so what is heard is the same to rounding, not merely close. All
		// but the last tenth of a second of the file: there ffmpeg's two
		// ways of reading the sound, from the start and after a seek,
		// differ from each other by up to 4% in the last 60 ms, and the
		// render reads it after a seek the way the video preview does.
		worst, at := 0.0, 0
		for i := 0; i < min(len(got), len(want), len(want)-rate/10*channels); i++ {
			if d := math.Abs(float64(got[i] - want[i])); d > worst {
				worst, at = d, i
			}
		}
		if worst > 1e-4 {
			t.Errorf("from sample %d the sound is off by %.5f at sample %d of channel %d", sample, worst, at/channels, at%channels)
		}
		for k, m := range moments {
			if w := from + float64(k*SoundChunk)/rate; math.Abs(m-w) > 1e-9 {
				t.Errorf("from sample %d chunk %d says %.6f, it is at %.6f", sample, k, m, w)
				break
			}
		}
	}
}

func floats(t *testing.T, raw []byte) []float32 {
	t.Helper()
	out := make([]float32, len(raw)/4)
	if err := binary.Read(bytes.NewReader(raw[:len(out)*4]), binary.LittleEndian, out); err != nil {
		t.Fatal(err)
	}
	return out
}
