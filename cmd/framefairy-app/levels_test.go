package main

import (
	"os"
	"os/exec"
	"path/filepath"
	"sync"
	"sync/atomic"
	"testing"
	"time"

	"framefairy/engine"
)

// levelsLibrary is a library with n short episodes in it and a service
// that measures their loudness the way the app does.
func levelsLibrary(t *testing.T, n int) (*FrameFairy, []string, *atomic.Int64) {
	t.Helper()
	if _, err := exec.LookPath("ffmpeg"); err != nil {
		t.Skip("ffmpeg is not installed")
	}
	home := t.TempDir()
	t.Setenv("HOME", home)
	t.Setenv("XDG_CONFIG_HOME", filepath.Join(home, "config"))
	var paths []string
	for i := range n {
		made, err := videos.make("levels"+string(rune('a'+i)), "20")
		if err != nil {
			t.Fatal(err)
		}
		// Each test its own copy, so one test's levels are not another's.
		path := filepath.Join(home, filepath.Base(made))
		data, err := os.ReadFile(made)
		if err != nil {
			t.Fatal(err)
		}
		if err := os.WriteFile(path, data, 0o644); err != nil {
			t.Fatal(err)
		}
		paths = append(paths, path)
	}
	st := openStore()
	if _, err := st.AddEpisodes(paths); err != nil {
		t.Fatal(err)
	}
	told := &atomic.Int64{}
	s := &FrameFairy{store: st}
	s.jobs = newQueue(st, func(JobUpdate) {}, func(string) {})
	s.levels = newMeasuring(func() string { return "" }, func(string) { told.Add(1) })
	t.Cleanup(s.levels.shutDown)
	return s, paths, told
}

func measured(t *testing.T, path string) {
	t.Helper()
	deadline := time.Now().Add(20 * time.Second)
	for {
		if _, done := engine.LevelsReach(path); done {
			return
		}
		if time.Now().After(deadline) {
			t.Fatalf("%s was never measured", filepath.Base(path))
		}
		time.Sleep(20 * time.Millisecond)
	}
}

// An episode has a waveform before anything has heard a word of it: asking
// for the waveform is enough to have it measured, and it arrives whole.
func TestTheWaveformComesBeforeTheTranscript(t *testing.T) {
	s, paths, told := levelsLibrary(t, 1)
	path := paths[0]
	if _, err := s.Waveform(path, 0, 20, 200); err != nil {
		t.Fatal(err)
	}
	measured(t, path)
	peaks, err := s.Waveform(path, 0, 20, 200)
	if err != nil {
		t.Fatal(err)
	}
	loud := 0
	for _, p := range peaks {
		if p > -40 {
			loud++
		}
	}
	if len(peaks) != 200 || loud < 190 {
		t.Errorf("%d buckets, %d of them the tone", len(peaks), loud)
	}
	// The interface is told after the waveform is written, so it can come
	// a moment after the file says it is done. It came too late for a read
	// straight after on a busy machine.
	deadline := time.Now().Add(5 * time.Second)
	for told.Load() == 0 && time.Now().Before(deadline) {
		time.Sleep(5 * time.Millisecond)
	}
	if told.Load() == 0 {
		t.Error("the interface was never told the waveform grew")
	}
	if st := s.Episode(path); !st.MeasuredAll || st.Measured < 19.9 || st.Covered != 0 {
		t.Errorf("status measured %.2f, all %v, covered %.2f", st.Measured, st.MeasuredAll, st.Covered)
	}
}

// Everything the app can do to the measuring, all at once, while it runs:
// ask for waveforms, start, stop, remove an episode with its work, read,
// and quit. Nothing races, nothing is left running, and a removed
// episode's work folder is not written back.
func TestMeasuringUnderEverythingAtOnce(t *testing.T) {
	s, paths, _ := levelsLibrary(t, 4)
	var wg sync.WaitGroup
	for round := range 3 {
		for i, path := range paths {
			wg.Add(1)
			go func() {
				defer wg.Done()
				switch (round + i) % 4 {
				case 0:
					s.levels.start(path)
				case 1:
					_, _ = s.Waveform(path, 0, 20, 100)
				case 2:
					s.levels.read(path)
					_ = s.Episode(path)
				case 3:
					s.levels.stop(path)
				}
			}()
		}
	}
	wg.Wait()

	// The last episode is removed with its work while it may be measured.
	gone := paths[3]
	s.levels.start(gone)
	if err := s.RemoveEpisode(gone, true); err != nil {
		t.Fatal(err)
	}
	for _, path := range paths[:3] {
		s.levels.start(path)
		measured(t, path)
	}
	time.Sleep(200 * time.Millisecond)
	if _, err := os.Stat(engine.WorkDir(gone)); !os.IsNotExist(err) {
		t.Errorf("the removed episode's work folder is back: %v", err)
	}

	s.levels.shutDown()
	s.levels.start(paths[0])
	s.levels.mu.Lock()
	left := len(s.levels.running)
	s.levels.mu.Unlock()
	if left != 0 {
		t.Errorf("%d measurings still running after quitting", left)
	}
}

// What the clip timeline asks the waveform of is where the measuring goes
// first, and a removed episode is forgotten.
func TestTheWaveformAskedForIsMeasuredFirst(t *testing.T) {
	s, paths, _ := levelsLibrary(t, 1)
	path := paths[0]
	if _, err := s.Waveform(path, 12, 15, 100); err != nil {
		t.Fatal(err)
	}
	if from, to := s.levels.lookingAt(path); from != 12 || to != 15 {
		t.Errorf("the measuring looks at %v to %v, the clip timeline at 12 to 15", from, to)
	}
	measured(t, path)
	s.levels.stop(path)
	if from, to := s.levels.lookingAt(path); from != 0 || to != 0 {
		t.Errorf("a stopped episode is still looked at, %v to %v", from, to)
	}
}
