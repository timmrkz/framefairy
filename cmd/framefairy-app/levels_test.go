package main

import (
	"os"
	"path/filepath"
	"sync"
	"sync/atomic"
	"testing"

	"github.com/spf13/afero"

	"framefairy/engine"
	"framefairy/internal/ffmpegtest"
)

// levelsLibrary is a library with n short episodes in it and a service
// that measures their loudness the way the app does. The tests that use
// it are not parallel, though they take up to a second or two, because it
// sets HOME and XDG_CONFIG_HOME for the whole process.
//
// On the real disk: ffmpeg reads the episodes.
func levelsLibrary(t *testing.T, n int) (*FrameFairy, []string, *atomic.Int64) {
	t.Helper()
	ffmpegtest.Need(t)
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
	s.levels = newMeasuring(s.store.files, func(string) { told.Add(1) })
	t.Cleanup(s.levels.shutDown)
	return s, paths, told
}

// measured waits until the measuring of a video has ended, on the channel
// it closes when it does, and fails the test when it did not measure the
// video whole.
// A measuring that is not running has ended already, because start adds
// it before it returns.
//
// On the real disk: ffmpeg measures the levels from the episode on the disk.
func measured(t *testing.T, s *FrameFairy, path string) {
	t.Helper()
	s.levels.mu.Lock()
	run := s.levels.running[path]
	s.levels.mu.Unlock()
	if run != nil {
		<-run.done
	}
	if _, done := engine.LevelsReach(disk, path); !done {
		t.Fatalf("%s was never measured", filepath.Base(path))
	}
}

// A measuring that fails or panics says so in the app's log, under the
// video. Both went to the standard error, which nobody reads on a Mac
// app opened from Finder, issue #605. A video that is not there makes it
// fail, and a notice that panics makes it panic.
//
// On the real disk: lumberjack writes the log and knows only the disk.
func TestAMeasuringThatGoesWrongIsInTheAppsLog(t *testing.T) {
	_, read := testLog(t)
	m := newMeasuring(afero.NewMemMapFs(), func(string) { panic("a waveform nobody drew before") })
	t.Cleanup(m.shutDown)
	path := "/home/tim/videos/gone.mp4"
	// Every slot is taken until the test holds the measuring, so it
	// cannot fail and leave the list before the test has seen it: a
	// video that is not there fails at once, and once in CI it was gone
	// before the test looked.
	for range cap(m.slots) {
		m.slots <- struct{}{}
	}
	m.start(path)
	m.mu.Lock()
	run := m.running[path]
	m.mu.Unlock()
	for range cap(m.slots) {
		<-m.slots
	}
	if run == nil {
		t.Fatal("the measuring did not start")
	}
	<-run.done
	lines := read()
	has(t, find(lines, "the loudness could not be measured"), map[string]any{"level": "warn", "from": "levels", "video": "gone.mp4"})
	has(t, find(lines, "measuring the loudness stopped unexpectedly"), map[string]any{
		"level": "error", "from": "levels", "video": "gone.mp4", "msg": "measuring the loudness stopped unexpectedly: a waveform nobody drew before",
	})
}

// An episode has a waveform before anything has heard a word of it: asking
// for the waveform is enough to have it measured, and it arrives whole.
func TestTheWaveformComesBeforeTheTranscript(t *testing.T) {
	s, paths, told := levelsLibrary(t, 1)
	path := paths[0]
	if _, err := s.Waveform(path, 0, 20, 200); err != nil {
		t.Fatal(err)
	}
	measured(t, s, path)
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
	// The interface is told after the waveform is written, and before the
	// measuring says it has ended, so it has been told by now.
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
//
// On the real disk: ffmpeg reads the episodes.
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
		measured(t, s, path)
	}
	// Removing waits for its measuring to end, so nothing is left of it
	// that could write the folder back after this.
	s.levels.mu.Lock()
	still := s.levels.running[gone] != nil
	s.levels.mu.Unlock()
	if still {
		t.Error("the removed episode is still measured")
	}
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
	measured(t, s, path)
	s.levels.stop(path)
	if from, to := s.levels.lookingAt(path); from != 0 || to != 0 {
		t.Errorf("a stopped episode is still looked at, %v to %v", from, to)
	}
}
