package engine

import (
	"bytes"
	"context"
	"encoding/binary"
	"math"
	"os"
	"os/exec"
	"path/filepath"
	"sync"
	"sync/atomic"
	"testing"
	"time"
)

// A frame is 10 ms of 16 kHz samples, measured the way the transcription
// measures it, and a last part shorter than a frame is measured too, so the
// levels reach the end of the audio.
func TestLevelsAreTheTranscriptionsFrames(t *testing.T) {
	samples := make([]float32, SampleRate+SampleRate/2+80)
	for i := range SampleRate {
		samples[i] = 0.5
	}
	var raw bytes.Buffer
	binary.Write(&raw, binary.LittleEndian, samples)
	e := NewEngine(NewLog(&bytes.Buffer{}, false, false))
	frames, err := e.readLevels(context.Background(), &raw, func([]float32) {})
	if err != nil {
		t.Fatal(err)
	}
	if len(frames) != 151 {
		t.Fatalf("%d frames for 1.505 s", len(frames))
	}
	if loud := frames[0]; math.Abs(float64(loud)-10*math.Log10(0.25)) > 0.01 {
		t.Errorf("a frame of 0.5 is %.2f dB", loud)
	}
	if frames[0] != frameDB(samples[:frameSamples]) {
		t.Error("not the transcription's own measure")
	}
	if frames[120] > -150 {
		t.Errorf("silence is %.1f dB", frames[120])
	}
}

// A whole episode is measured, written where the waveform reads it, and
// forgotten when the file changes. Readers that come while it is written
// never see frames the json does not vouch for.
func TestMeasureLevelsOfAnEpisode(t *testing.T) {
	if _, err := exec.LookPath("ffmpeg"); err != nil {
		t.Skip("ffmpeg is not installed")
	}
	dir := t.TempDir()
	source := filepath.Join(dir, "episode.m4a")
	// A second of tone, at the eighth of full scale ffmpeg plays it, about
	// -21 dB, and then two of silence.
	out, err := exec.Command("ffmpeg", "-loglevel", "error", "-y",
		"-f", "lavfi", "-i", "sine=frequency=440:duration=1",
		"-f", "lavfi", "-i", "anullsrc=r=48000:cl=mono:d=2",
		"-filter_complex", "[0:a][1:a]concat=n=2:v=0:a=1", "-c:a", "aac", source).CombinedOutput()
	if err != nil {
		t.Fatalf("%v: %s", err, out)
	}
	if got := ReadLevels(source); len(got.Frames) != 0 || got.Done {
		t.Fatal("levels out of nowhere")
	}
	e := NewEngine(NewLog(&bytes.Buffer{}, false, false))
	e.FFmpeg = "ffmpeg"

	var stop atomic.Bool
	var wg sync.WaitGroup
	for range 4 {
		wg.Add(1)
		go func() {
			defer wg.Done()
			for !stop.Load() {
				l := ReadLevels(source)
				if l.To() > 3.1 {
					t.Errorf("levels reach %.2f s of 3", l.To())
				}
			}
		}()
	}
	var reached []float64
	err = e.MeasureLevels(context.Background(), source, func(to float64) { reached = append(reached, to) })
	stop.Store(true)
	wg.Wait()
	if err != nil {
		t.Fatal(err)
	}

	l := ReadLevels(source)
	if !l.Done || math.Abs(l.To()-3) > 0.05 {
		t.Fatalf("done %v, %.2f s", l.Done, l.To())
	}
	if len(reached) == 0 || math.Abs(reached[len(reached)-1]-l.To()) > 1e-9 {
		t.Errorf("progress said %v", reached)
	}
	if l.Frames[50] < -30 || l.Frames[250] > -60 {
		t.Errorf("tone %.1f dB, silence %.1f dB", l.Frames[50], l.Frames[250])
	}

	// Measured once is measured: a second call reads nothing again.
	before, _ := os.Stat(filepath.Join(WorkDir(source), "logs", "levels.frames"))
	if err := e.MeasureLevels(context.Background(), source, nil); err != nil {
		t.Fatal(err)
	}
	after, _ := os.Stat(filepath.Join(WorkDir(source), "logs", "levels.frames"))
	if !after.ModTime().Equal(before.ModTime()) {
		t.Error("an episode measured already was measured again")
	}

	// A file that changed has levels of its own to measure.
	later := time.Now().Add(time.Minute)
	if err := os.Chtimes(source, later, later); err != nil {
		t.Fatal(err)
	}
	if got := ReadLevels(source); len(got.Frames) != 0 {
		t.Error("the levels of the file before were read for the file now")
	}
}

// Stopped part way, the measuring says so and leaves nothing that claims
// to be done.
func TestMeasureLevelsStops(t *testing.T) {
	if _, err := exec.LookPath("ffmpeg"); err != nil {
		t.Skip("ffmpeg is not installed")
	}
	dir := t.TempDir()
	source := filepath.Join(dir, "episode.m4a")
	out, err := exec.Command("ffmpeg", "-loglevel", "error", "-y",
		"-f", "lavfi", "-i", "sine=frequency=440:duration=5", "-c:a", "aac", source).CombinedOutput()
	if err != nil {
		t.Fatalf("%v: %s", err, out)
	}
	e := NewEngine(NewLog(&bytes.Buffer{}, false, false))
	e.FFmpeg = "ffmpeg"
	ctx, cancel := context.WithCancel(context.Background())
	cancel()
	if err := e.MeasureLevels(ctx, source, nil); err == nil {
		t.Error("a measuring called off said it finished")
	}
	if ReadLevels(source).Done {
		t.Error("levels called off claim to be done")
	}
}
