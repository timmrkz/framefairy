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
	var frames []float32
	end, err := e.readLevels(context.Background(), &raw, func(f float32) bool {
		frames = append(frames, f)
		return true
	}, func() bool { return true })
	if err != nil || !end {
		t.Fatal(err, end)
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
	err = e.MeasureLevels(context.Background(), source, nil, func(to float64) { reached = append(reached, to) })
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
	if err := e.MeasureLevels(context.Background(), source, nil, nil); err != nil {
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
	if err := e.MeasureLevels(ctx, source, nil, nil); err == nil {
		t.Error("a measuring called off said it finished")
	}
	if ReadLevels(source).Done {
		t.Error("levels called off claim to be done")
	}
}

// How far the loudness reaches is read from the json alone, and agrees with
// the frames.
func TestLevelsReachAgreesWithTheFrames(t *testing.T) {
	if _, err := exec.LookPath("ffmpeg"); err != nil {
		t.Skip("ffmpeg is not installed")
	}
	source := filepath.Join(t.TempDir(), "episode.m4a")
	out, err := exec.Command("ffmpeg", "-loglevel", "error", "-y",
		"-f", "lavfi", "-i", "sine=frequency=440:duration=2", "-c:a", "aac", source).CombinedOutput()
	if err != nil {
		t.Fatalf("%v: %s", err, out)
	}
	if parts, done := LevelsReach(source); len(parts) != 0 || done {
		t.Fatalf("an episode never measured has %v, done %v", parts, done)
	}
	e := NewEngine(NewLog(&bytes.Buffer{}, false, false))
	e.FFmpeg = "ffmpeg"
	if err := e.MeasureLevels(context.Background(), source, nil, nil); err != nil {
		t.Fatal(err)
	}
	parts, done := LevelsReach(source)
	if !done || len(parts) != 1 || parts[0][0] != 0 || math.Abs(parts[0][1]-ReadLevels(source).To()) > 1e-9 {
		t.Errorf("parts %v done %v, frames %.2f", parts, done, ReadLevels(source).To())
	}
	if st := Status(source, ""); !st.MeasuredAll || st.Measured != parts[0][1] {
		t.Errorf("the library says %.2f, all %v", st.Measured, st.MeasuredAll)
	}
}

// sweep makes an episode whose loudness rises and falls every few seconds,
// so a frame measured in the wrong place shows.
func sweep(t *testing.T, seconds string) string {
	t.Helper()
	if _, err := exec.LookPath("ffmpeg"); err != nil {
		t.Skip("ffmpeg is not installed")
	}
	source := filepath.Join(t.TempDir(), "episode.m4a")
	out, err := exec.Command("ffmpeg", "-loglevel", "error", "-y", "-f", "lavfi", "-i",
		"aevalsrc=sin(2*PI*440*t)*(0.5+0.45*sin(2*PI*0.37*t)):s=16000:d="+seconds,
		"-c:a", "aac", source).CombinedOutput()
	if err != nil {
		t.Fatalf("%v: %s", err, out)
	}
	return source
}

// Where the clip timeline looks is measured first, and when it moves to a
// part not measured yet, the measuring follows it there. What comes out is
// the same loudness a measuring from the start gives, joins and all.
func TestMeasureLevelsGoesWhereTheClipTimelineLooks(t *testing.T) {
	source := sweep(t, "300")
	saved := levelsEvery
	levelsEvery = 50 * time.Millisecond
	defer func() { levelsEvery = saved }()
	e := NewEngine(NewLog(&bytes.Buffer{}, false, false))
	e.FFmpeg = "ffmpeg"

	// The playhead near the end when the episode is added, and then moved
	// to the middle once the first of it is on screen.
	var mu sync.Mutex
	view := [2]float64{250, 260}
	focus := func() (float64, float64) {
		mu.Lock()
		defer mu.Unlock()
		return view[0], view[1]
	}
	var first, moved [][2]int
	err := e.MeasureLevels(context.Background(), source, focus, func(float64) {
		l := ReadLevels(source)
		if first == nil && len(l.Parts) > 0 {
			first = l.Parts
			mu.Lock()
			view = [2]float64{150, 160}
			mu.Unlock()
		}
		if moved == nil && covers(l.Parts, 150, 160) {
			moved = l.Parts
		}
	})
	if err != nil {
		t.Fatal(err)
	}
	if len(first) != 1 || first[0][0] != int(250/FrameSeconds) {
		t.Errorf("measured first %v, want from 250 s, where the clip timeline showed", partSeconds(first))
	}
	if moved == nil || covers(moved, 0, 10) || covers(moved, 100, 110) {
		t.Errorf("when the view moved to 150 s the parts were %v, want it there before the start",
			partSeconds(moved))
	}

	jumped := ReadLevels(source)
	if !jumped.Done || len(jumped.Parts) != 1 {
		t.Fatalf("done %v, parts %v", jumped.Done, partSeconds(jumped.Parts))
	}
	// The same episode measured from the start, in one go.
	again := filepath.Join(t.TempDir(), "again.m4a")
	raw, _ := os.ReadFile(source)
	if err := os.WriteFile(again, raw, 0o644); err != nil {
		t.Fatal(err)
	}
	if err := e.MeasureLevels(context.Background(), again, nil, nil); err != nil {
		t.Fatal(err)
	}
	whole := ReadLevels(again)
	if len(whole.Frames) != len(jumped.Frames) {
		t.Fatalf("%d frames from the start, %d where the view went", len(whole.Frames), len(jumped.Frames))
	}
	worst := 0.0
	for i := range whole.Frames {
		worst = max(worst, math.Abs(float64(whole.Frames[i]-jumped.Frames[i])))
	}
	if worst > 1 {
		t.Errorf("frames differ by up to %.2f dB between the two", worst)
		for i := range whole.Frames {
			if d := math.Abs(float64(whole.Frames[i] - jumped.Frames[i])); d > 0.3 {
				t.Logf("frame %d (%.2f s): %.2f from the start, %.2f jumped", i, float64(i)*FrameSeconds, whole.Frames[i], jumped.Frames[i])
			}
		}
	}
}

// A measuring cut off keeps what it has, and the next carries on from
// there rather than starting again.
func TestMeasureLevelsCarriesOn(t *testing.T) {
	source := sweep(t, "120")
	saved := levelsEvery
	levelsEvery = 20 * time.Millisecond
	defer func() { levelsEvery = saved }()
	e := NewEngine(NewLog(&bytes.Buffer{}, false, false))
	e.FFmpeg = "ffmpeg"
	// Cut off at the first write, which is part of what the clip timeline
	// shows and on from there.
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()
	if err := e.MeasureLevels(ctx, source, func() (float64, float64) { return 60, 70 },
		func(measured float64) {
			if measured > 0 {
				cancel()
			}
		}); err == nil {
		t.Fatal("a measuring cut off said it finished")
	}
	levelsEvery = saved
	had := ReadLevels(source)
	if had.Done || len(had.Parts) != 1 || had.Parts[0][0] != int(60/FrameSeconds) {
		t.Fatalf("cut off with %v, done %v", partSeconds(had.Parts), had.Done)
	}
	if st := Status(source, ""); st.MeasuredAll || st.Measured <= 0 || len(st.MeasuredParts) == 0 {
		t.Errorf("the library says %.2f in %v, all %v", st.Measured, st.MeasuredParts, st.MeasuredAll)
	}
	var calls int
	if err := e.MeasureLevels(context.Background(), source, nil, func(float64) { calls++ }); err != nil {
		t.Fatal(err)
	}
	l := ReadLevels(source)
	if !l.Done || len(l.Parts) != 1 {
		t.Fatalf("done %v, parts %v", l.Done, partSeconds(l.Parts))
	}
	for _, p := range had.Parts {
		for n := p[0]; n < p[1]; n++ {
			if l.Frames[n] != had.Frames[n] {
				t.Fatalf("frame %d was measured again", n)
			}
		}
	}
}

// The measuring picks what the clip timeline shows, then on from there,
// then the start.
func TestWhereToMeasureNext(t *testing.T) {
	st := &levelState{end: -1}
	at := func(s float64) int { return int(s / FrameSeconds) }
	for n := at(10); n < at(20); n++ {
		st.put(n, -20)
	}
	for _, c := range []struct {
		from, to float64
		want     int
	}{
		{0, 0, 0},
		{12, 30, at(20)},
		{25, 30, at(25)},
		{10, 20, at(20)},
	} {
		if got, ok := st.next(c.from, c.to); !ok || got != c.want {
			t.Errorf("showing %v to %v: next %v, want %v", c.from, c.to, got, c.want)
		}
	}
	st.end = at(20)
	for n := range at(10) {
		st.put(n, -20)
	}
	if _, ok := st.next(12, 30); ok || !st.done() {
		t.Error("an episode measured to its end has more to measure")
	}
	if parts := st.parts(); len(parts) != 1 || parts[0] != [2]int{0, at(20)} {
		t.Errorf("parts %v", parts)
	}
}

func covers(parts [][2]int, from, to float64) bool {
	for _, p := range parts {
		if float64(p[0])*FrameSeconds <= from && float64(p[1])*FrameSeconds >= to {
			return true
		}
	}
	return false
}

// The waveform has whatever either measured: the loudness measured on its
// own where it has been, the transcription's where only it has heard.
func TestLevelsOverATranscript(t *testing.T) {
	l := Levels{Frames: []float32{-90, -90, -10, -11, -90, -12}, Parts: [][2]int{{2, 4}, {5, 6}}}
	heard := &Transcript{Start: 0.01, Frames: []float32{-30, -31, -32, -33, -34, -35, -36}}
	got := l.Over(heard).Frames
	want := []float32{-90, -30, -10, -11, -33, -12, -35, -36}
	if len(got) != len(want) {
		t.Fatalf("%v, want %v", got, want)
	}
	for i := range want {
		if got[i] != want[i] {
			t.Fatalf("%v, want %v", got, want)
		}
	}
	if l.Over(nil).Frames[2] != -10 || (Levels{}).Over(heard) != heard {
		t.Error("with only one of them, that one is the waveform")
	}
}

// A measuring leaves for the view only when the view has a part not
// measured and it is not in it, judged by where it measures and not by the
// part before it that it decodes and throws away. Judged by that, one that
// started slowly left for where it already was, again and again.
func TestAMeasuringLeavesOnlyForAView(t *testing.T) {
	st := &levelState{end: -1}
	view := func() (float64, float64) { return 10, 20 }
	start := int(10 / FrameSeconds)
	if st.leave(start, view) {
		t.Error("a measuring at the start of the view left it")
	}
	if !st.leave(start-levelsLead, view) {
		t.Error("the part thrown away is outside the view, which is what the measuring must not judge by")
	}
	if st.leave(max(start-levelsLead, start), view) {
		t.Error("a measuring still in the part it throws away left for where it was going")
	}
	if !st.leave(int(50/FrameSeconds), view) {
		t.Error("a measuring elsewhere stayed while the view had nothing")
	}
	for n := start; n < int(20/FrameSeconds); n++ {
		st.put(n, -20)
	}
	if st.leave(int(50/FrameSeconds), view) {
		t.Error("a measuring left for a view measured already")
	}
}
