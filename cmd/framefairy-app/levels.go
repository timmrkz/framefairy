package main

import (
	"context"
	"fmt"
	"io"
	"log"
	"os"
	"sync"
	"time"

	"framefairy/engine"
)

// measuring measures the loudness of each episode, the waveform of the clip
// timeline, from the moment it is added, see engine.MeasureLevels. It is
// not a job: nobody starts it, waits for it or stops it by hand, and it
// takes seconds, so it has no row in Activity. What it has measured is on
// screen as it goes.
//
// At most two episodes are measured at a time, so adding a season does not
// start a decoder for every episode at once, and one goes on while the
// transcription has the speech model.
type measuring struct {
	ffmpeg func() string
	notify func(episode string)

	mu      sync.Mutex
	running map[string]*measure
	// Where the clip timeline of each episode looks, which is measured
	// first, see look.
	looking map[string][2]float64
	slots   chan struct{}
	closed  bool
	// What was read last, kept while its files stay as they were, because
	// the timeline asks for the waveform on every swipe.
	kept   engine.Levels
	keptBy string
}

type measure struct {
	stop context.CancelFunc
	done chan struct{}
}

func newMeasuring(ffmpeg func() string, notify func(string)) *measuring {
	return &measuring{ffmpeg: ffmpeg, notify: notify,
		running: map[string]*measure{}, looking: map[string][2]float64{}, slots: make(chan struct{}, 2)}
}

// start measures an episode unless it is measured already or being
// measured. It is cheap to call as often as the waveform is asked for.
// Like every method here it does nothing on a nil measuring, which is what
// a test that builds the service by hand has.
func (m *measuring) start(path string) {
	if m == nil {
		return
	}
	m.mu.Lock()
	if m.closed || m.running[path] != nil {
		m.mu.Unlock()
		return
	}
	m.mu.Unlock()
	if _, done := engine.LevelsReach(path); done {
		return
	}
	m.mu.Lock()
	defer m.mu.Unlock()
	if m.closed || m.running[path] != nil {
		return
	}
	ctx, cancel := context.WithCancel(context.Background())
	run := &measure{stop: cancel, done: make(chan struct{})}
	m.running[path] = run
	go m.run(ctx, path, run)
}

func (m *measuring) run(ctx context.Context, path string, run *measure) {
	defer func() {
		// A measuring that goes wrong ends itself, not the app, and the
		// waveform stays as the transcription measures it.
		if r := recover(); r != nil {
			log.Printf("measuring the loudness of %s: %v", path, r)
		}
		m.mu.Lock()
		if m.running[path] == run {
			delete(m.running, path)
		}
		m.mu.Unlock()
		run.stop()
		close(run.done)
	}()
	select {
	case m.slots <- struct{}{}:
		defer func() { <-m.slots }()
	case <-ctx.Done():
		return
	}
	e := engine.NewEngine(engine.NewLog(io.Discard, false, false))
	if ff := m.ffmpeg(); ff != "" {
		e.FFmpeg = ff
	}
	// The interface is told twice a second at most, which is as often as
	// the engine writes what it has.
	var told time.Time
	err := e.MeasureLevels(ctx, path, func() (float64, float64) { return m.lookingAt(path) }, func(float64) {
		if time.Since(told) >= 500*time.Millisecond {
			told = time.Now()
			m.notify(path)
		}
	})
	if err != nil && ctx.Err() == nil {
		log.Printf("measuring the loudness of %s: %v", path, err)
	}
	m.notify(path)
}

// look says what the clip timeline of an episode shows, from and to, which
// its measuring goes to next. It is cheap to call on every swipe.
func (m *measuring) look(path string, from, to float64) {
	if m == nil {
		return
	}
	m.mu.Lock()
	m.looking[path] = [2]float64{from, to}
	m.mu.Unlock()
}

func (m *measuring) lookingAt(path string) (float64, float64) {
	m.mu.Lock()
	defer m.mu.Unlock()
	at := m.looking[path]
	return at[0], at[1]
}

// stop ends the measuring of an episode and waits until it has, so its
// work folder can be deleted without anything writing it back.
func (m *measuring) stop(path string) {
	if m == nil {
		return
	}
	m.mu.Lock()
	run := m.running[path]
	delete(m.looking, path)
	m.mu.Unlock()
	if run == nil {
		return
	}
	run.stop()
	<-run.done
}

// shutDown ends every measuring and starts no other.
func (m *measuring) shutDown() {
	if m == nil {
		return
	}
	m.mu.Lock()
	m.closed = true
	var runs []*measure
	for _, run := range m.running {
		runs = append(runs, run)
	}
	m.mu.Unlock()
	for _, run := range runs {
		run.stop()
		<-run.done
	}
}

// read is the loudness measured so far, from the files, kept while they
// stay as they were.
func (m *measuring) read(path string) engine.Levels {
	if m == nil {
		return engine.ReadLevels(path)
	}
	// The episode's own file is in the stamp: when it changes, what was
	// measured of it before is no longer its loudness.
	stamp := path
	for _, file := range append([]string{path}, engine.LevelsFiles(path)...) {
		info, err := os.Stat(file)
		if err != nil {
			stamp += "|-"
			continue
		}
		stamp += fmt.Sprintf("|%d,%d", info.Size(), info.ModTime().UnixNano())
	}
	m.mu.Lock()
	if m.keptBy == stamp {
		kept := m.kept
		m.mu.Unlock()
		return kept
	}
	m.mu.Unlock()
	l := engine.ReadLevels(path)
	m.mu.Lock()
	m.kept, m.keptBy = l, stamp
	m.mu.Unlock()
	return l
}
