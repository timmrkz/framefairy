package main

import (
	"context"
	"fmt"
	"math"
	"path/filepath"
	"runtime/debug"
	"sync"

	"github.com/rs/zerolog"

	"framefairy/engine"
	"framefairy/engine/timed"
)

// measuring measures the loudness of each episode, the waveform of the clip
// timeline, from the moment it is added, see engine.MeasureLevels. It is
// not a job: nobody starts it, waits for it or stops it by hand, and it
// takes seconds, so it is not in the job list. What it has measured is on
// screen as it goes.
//
// At most two episodes are measured at a time, so adding a season does not
// start a decoder for every episode at once, and one goes on while the
// transcription has the speech model.
type measuring struct {
	// machine is the store's, whose files the levels are kept on.
	machine *engine.Machine
	notify  func(episode string)

	// measure is what measures one episode, engine.MeasureLevels in the
	// app. A test hands in its own, so it decides when a measuring writes.
	measure func(ctx context.Context, path string, look engine.LevelsLook, progress func(float64)) error
	// base is the loudness of an episode as the window reads it whole,
	// and length how many readings its sound has, as the probe says: what
	// a measuring's schedule starts from, see engine.LoudnessSchedule.
	base   func(path string) []byte
	length func(ctx context.Context, path string) int

	mu      sync.Mutex
	running map[string]*measure
	// Where the clip timeline of each episode looks, which is measured
	// first, see look.
	looking map[string][2]float64
	// The episodes being removed, which start no measuring, see
	// closeEpisode. Counted, because two removals of one episode can
	// overlap.
	shut   map[string]int
	slots  chan struct{}
	closed bool
	// What was read last, kept while its files stay as they were, because
	// a video opened again reads it from here rather than from the disk.
	kept   engine.Levels
	keptBy string
	// The episodes a look starts nothing for: measured whole, as the disk
	// said when start last read it, or measured as far as a run could,
	// one that ended by itself. The clip timeline says where it looks on
	// every move of the hand, so a look reads this and never the disk,
	// see look. start reads the disk again, and forgets an episode the
	// disk says is not measured whole.
	settled map[string]bool
}

type measure struct {
	stop context.CancelFunc
	done chan struct{}
	// live answers the clip timeline's looks with what the measuring
	// brings, see lookLoudness.
	live *engine.LoudnessSchedule
}

// lookOf is what a measuring is told of the clip timeline: where it looks,
// as the window said last, and where each reading goes as it is measured.
type lookOf struct {
	m    *measuring
	path string
	live *engine.LoudnessSchedule
}

func (l lookOf) View() (float64, float64)        { return l.m.lookingAt(l.path) }
func (l lookOf) Put(first int, frames []float32) { l.live.Put(first, frames) }

func newMeasuring(machine *engine.Machine, notify func(string)) *measuring {
	m := &measuring{machine: machine, notify: notify, running: map[string]*measure{},
		looking: map[string][2]float64{}, shut: map[string]int{}, settled: map[string]bool{}, slots: make(chan struct{}, 2)}
	m.measure = func(ctx context.Context, path string, look engine.LevelsLook, progress func(float64)) error {
		return engine.NewEngine(engineLog("levels", path), m.machine).MeasureLevels(ctx, path, look, progress)
	}
	m.length = func(ctx context.Context, path string) int {
		info, err := engine.NewEngine(engineLog("levels", path), m.machine).Probe(ctx, path)
		if err != nil {
			return 0
		}
		return int(math.Ceil(info.Duration / timed.FrameSeconds))
	}
	return m
}

// start measures an episode unless it is measured already, being
// measured or being removed. Whether it is measured it reads from the
// small file of its levels, which is cheap enough for every time the
// loudness is asked for, but not for every move of the hand, see look.
// Like every method here it does nothing on a nil measuring, which is what
// a test that builds the service by hand has.
func (m *measuring) start(path string) {
	if m == nil {
		return
	}
	m.mu.Lock()
	if !m.mayStartLocked(path) {
		m.mu.Unlock()
		return
	}
	m.mu.Unlock()
	_, done := engine.LevelsReach(m.machine.Files, path)
	m.mu.Lock()
	defer m.mu.Unlock()
	if done {
		m.settled[path] = true
		return
	}
	delete(m.settled, path)
	if !m.mayStartLocked(path) {
		return
	}
	ctx, cancel := context.WithCancel(context.Background())
	run := &measure{stop: cancel, done: make(chan struct{}), live: engine.NewLoudnessSchedule()}
	m.running[path] = run
	go m.run(ctx, path, run)
}

func (m *measuring) mayStartLocked(path string) bool {
	return !m.closed && m.shut[path] == 0 && m.running[path] == nil
}

func (m *measuring) run(ctx context.Context, path string, run *measure) {
	defer func() {
		// A measuring that goes wrong ends itself, not the app, and the
		// waveform stays as the transcription measures it.
		if r := recover(); r != nil {
			theLog.line(zerolog.ErrorLevel, "levels").Str("video", filepath.Base(path)).Str("stack", string(debug.Stack())).
				Msgf("measuring the loudness stopped unexpectedly: %v", r)
		}
		m.mu.Lock()
		if m.running[path] == run {
			delete(m.running, path)
			// A run that ended by itself has measured all it could, and a
			// look does not start it again: one that failed would fail
			// again on every move of the hand. Opening the video again
			// asks start, which reads the disk.
			if ctx.Err() == nil {
				m.settled[path] = true
			}
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
	// The window is answered with each reading as it comes, by the
	// schedule, see lookLoudness, and told once the measuring has ended,
	// when it reads the loudness whole again. Told twice a second, it read
	// all of it each time, 3 MB for four hours, issue #968.
	var base []byte
	if m.base != nil {
		base = m.base(path)
	}
	length := 0
	if m.length != nil {
		length = m.length(ctx, path)
	}
	run.live.Start(base, length)
	err := m.measure(ctx, path, lookOf{m: m, path: path, live: run.live}, nil)
	run.live.Ended()
	if err != nil && ctx.Err() == nil {
		theLog.line(zerolog.WarnLevel, "levels").Str("video", filepath.Base(path)).Err(err).Msg("the loudness could not be measured")
	}
	m.notify(path)
}

// lookLoudness answers where the clip timeline of an episode looks with
// the loudness of its view the window does not hold, see
// engine.LoudnessSchedule.Look. An episode not being measured has nothing
// new: the window read it whole when it opened, and again once its
// measuring ended.
func (m *measuring) lookLoudness(ctx context.Context, path string, v engine.View) ([]byte, error) {
	if m == nil {
		return nil, engine.ErrNothingNew
	}
	m.look(path, v.From, v.To)
	m.mu.Lock()
	run := m.running[path]
	m.mu.Unlock()
	if run == nil {
		return nil, engine.ErrNothingNew
	}
	return run.live.Look(ctx, v)
}

// look says what the clip timeline of an episode shows, from and to, which
// its measuring goes to next, and measures an episode not measured yet.
// The window says it on every move of the hand, see Look, so once an
// episode is settled a look is a map write and reads nothing from the
// disk. Before, the window held the call back until the hand rested,
// because each one read the levels file, issue #937.
func (m *measuring) look(path string, from, to float64) {
	if m == nil {
		return
	}
	m.mu.Lock()
	m.looking[path] = [2]float64{from, to}
	settled := m.settled[path] || !m.mayStartLocked(path)
	m.mu.Unlock()
	if !settled {
		m.start(path)
	}
}

func (m *measuring) lookingAt(path string) (float64, float64) {
	m.mu.Lock()
	defer m.mu.Unlock()
	at := m.looking[path]
	return at[0], at[1]
}

// closeEpisode ends the measuring of an episode, waits until it has, and
// starts none for it until reopen is called or the episode is added again,
// so its work folder can be deleted without anything writing it back. The
// waveform is still asked for while a removal waits for the episode's jobs,
// and a measuring it started again made the work folder again after the
// folder was deleted.
//
// It is the measuring's side of the queue's closeEpisode, called with it.
func (m *measuring) closeEpisode(path string) (reopen func()) {
	if m == nil {
		return func() {}
	}
	m.mu.Lock()
	m.shut[path]++
	run := m.running[path]
	delete(m.looking, path)
	m.mu.Unlock()
	defer func() {
		// Forgotten after the run has ended, which may have settled it.
		m.mu.Lock()
		delete(m.settled, path)
		m.mu.Unlock()
	}()
	if run != nil {
		run.stop()
		<-run.done
	}
	var once sync.Once
	return func() {
		once.Do(func() {
			m.mu.Lock()
			if m.shut[path]--; m.shut[path] <= 0 {
				delete(m.shut, path)
			}
			m.mu.Unlock()
		})
	}
}

// openEpisode lets an episode be measured again, when it is added again
// after a removal.
func (m *measuring) openEpisode(path string) {
	if m == nil {
		return
	}
	m.mu.Lock()
	delete(m.shut, path)
	delete(m.settled, path)
	m.mu.Unlock()
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
	// The episode's own file is in the stamp: when it changes, what was
	// measured of it before is no longer its loudness.
	stamp := path
	for _, file := range append([]string{path}, engine.LevelsFiles(path)...) {
		info, err := m.machine.Files.Stat(file)
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
	l := engine.ReadLevels(m.machine.Files, path)
	m.mu.Lock()
	m.kept, m.keptBy = l, stamp
	m.mu.Unlock()
	return l
}
