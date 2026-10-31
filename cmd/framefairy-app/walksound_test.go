package main

import (
	"context"
	"errors"
	"fmt"
	"slices"
	"sync"
	"sync/atomic"
	"testing"
	"time"

	"framefairy/engine"
)

// sound is the bridge's microphone and the clocks of the recording in
// hand, which a walk drives, #551. Dictate on Report a Problem records two
// seconds of a hum, then a pause and a second and a half more, over and
// over, and the stand-in speech model hears it as words: what came before
// a pause is settled while the recording runs, and there is more to be
// heard until the recording stops, or until a walk has the person fall
// silent. A step hands the app a tenth of a second of it, a reading of how
// loud it is and a pass of the speech model, and nothing moves between
// steps. A walk asks for steps until the app has told the window what it
// waits for, see until, so it never waits on the app's own timers: a
// tenth of a second every 100 ms of the clock and a pass every 400 ms made
// every dictation walk wait seconds for words.
type sound struct {
	mu   sync.Mutex
	turn *sync.Cond
	// mic is the microphone of the latest recording, nil before the first.
	mic *bridgeMicrophone
	// silent says the person dictating has stopped talking.
	silent atomic.Bool
	// still says the quiet is over, once the person has fallen silent and
	// the phrase they were on is said, see listening.still.
	still chan struct{}
	// The clocks the app's recordings run on, see listenBeats.
	levels, passes   chan time.Time
	measured, passed chan struct{}
	// What the app told the window of the latest recording: its number,
	// what it told last, whether it told words being heard since a walk
	// last asked, and how many times the speech model settled words heard,
	// as against a click in them settling them, which also counts in
	// Asked.
	recording int
	told      Dictation
	hearing   bool
	spoken    int
}

func newSound() *sound {
	s := &sound{still: make(chan struct{}, 1), levels: make(chan time.Time), passes: make(chan time.Time),
		measured: make(chan struct{}, 1), passed: make(chan struct{}, 1)}
	s.turn = sync.NewCond(&s.mu)
	return s
}

func (s *sound) beats() *listenBeats {
	return &listenBeats{levels: s.levels, passes: s.passes, measured: s.measured, passed: s.passed}
}

// open is the microphone a new recording gets.
func (s *sound) open() microphone {
	// A quiet left over from a recording before is not this one's.
	select {
	case <-s.still:
	default:
	}
	return &bridgeMicrophone{s: s, samples: tone(2), stopped: make(chan struct{})}
}

// tell hears what the app tells the window of a recording. A recording
// with a lower number than the one it heard last is one before it, which
// may still say how it ended.
func (s *sound) tell(d Dictation) {
	s.mu.Lock()
	defer s.mu.Unlock()
	if d.Recording < s.recording {
		return
	}
	if d.Recording > s.recording {
		s.recording, s.told, s.hearing, s.spoken = d.Recording, Dictation{}, false, 0
	}
	if d.Hearing != "" {
		s.hearing = true
	}
	if d.Asked == s.told.Asked && len(d.Said) > len(s.told.Said) {
		s.spoken++
	}
	s.told = d
}

// started gives the microphone of the recording in hand once there is
// one. A walk asks for steps right after it pressed Dictate, before the
// app may have started the recording, so no recording yet is no reason to
// answer. A walk that asks when none is coming hangs until it gives up,
// and then the request is cancelled and started gives up with it, so the
// bridge holds nothing over into the next walk.
func (s *sound) started(ctx context.Context) (*bridgeMicrophone, error) {
	stop := context.AfterFunc(ctx, func() {
		s.mu.Lock()
		defer s.mu.Unlock()
		s.turn.Broadcast()
	})
	defer stop()
	s.mu.Lock()
	defer s.mu.Unlock()
	for s.mic == nil || s.mic.over() {
		if err := ctx.Err(); err != nil {
			return nil, fmt.Errorf("no recording to take steps of: %w", err)
		}
		s.turn.Wait()
	}
	return s.mic, nil
}

// until takes steps of the recording in hand until the app has told the
// window what want names, or the recording is over, and says how many.
// A recording is at most a minute on the bridge, see bridge, so it ends
// in 600 steps however it is asked.
func (s *sound) until(ctx context.Context, want string) (int, error) {
	var spoken int
	reached := map[string]func() bool{
		"hearing": func() bool { return s.hearing },
		"said":    func() bool { return s.spoken > spoken },
		"settled": func() bool { return len(s.told.Said) > 0 },
		"ended":   func() bool { return false },
		"tenth":   func() bool { return true },
	}[want]
	if reached == nil {
		return 0, fmt.Errorf("no such step of a recording: %q", want)
	}
	m, err := s.started(ctx)
	if err != nil {
		return 0, err
	}
	s.mu.Lock()
	s.hearing = false
	spoken = s.spoken
	s.mu.Unlock()
	// Something settled may have been settled before.
	s.mu.Lock()
	done := want == "settled" && reached()
	s.mu.Unlock()
	if done {
		return 0, nil
	}
	for steps := 1; ; steps++ {
		if !s.step(m) {
			return steps, nil
		}
		s.mu.Lock()
		done := reached()
		s.mu.Unlock()
		if done {
			return steps, nil
		}
	}
}

// step hands the app the next tenth of a second the microphone hears, a
// reading of how loud it is, and a pass of the speech model, and returns
// once the pass is over, so what it told the window has been told. It
// says false once the recording is over.
func (s *sound) step(m *bridgeMicrophone) bool {
	if !s.read(m) {
		return false
	}
	select {
	case <-s.passed:
	default:
	}
	select {
	case s.passes <- time.Time{}:
	case <-m.stopped:
		return false
	}
	select {
	case <-s.passed:
		return true
	case <-m.stopped:
		return false
	}
}

// ahead hands the app n tenths of a second the microphone hears, each
// with its reading of how loud it is, and no pass of the speech model,
// the way a person talks faster than the speech model writes it down on
// a busy machine, #573. It says how many it handed.
func (s *sound) ahead(ctx context.Context, n int) (int, error) {
	m, err := s.started(ctx)
	if err != nil {
		return 0, err
	}
	for i := range n {
		if !s.read(m) {
			return i, nil
		}
	}
	return n, nil
}

// read hands the app the next tenth of a second and a reading of how
// loud it is, and returns once the reading is over. It says false once
// the recording is over.
func (s *sound) read(m *bridgeMicrophone) bool {
	if !m.hand() {
		return false
	}
	// The reading is over before the next tenth is handed over, so each
	// reading holds its own tenth. One that read late found the tenth
	// after it already taken and read a room with nobody in it.
	select {
	case <-s.measured:
	default:
	}
	select {
	case s.levels <- time.Time{}:
	case <-m.stopped:
		return false
	}
	select {
	case <-s.measured:
		return true
	case <-m.stopped:
		return false
	}
}

// bridgeMicrophone hands the app what the bridge's microphone hears, a
// tenth of a second at a time, when a step asks for it.
type bridgeMicrophone struct {
	s *sound
	// mu keeps a tenth being handed over and the microphone stopping
	// apart, so nothing is heard once it has stopped.
	mu      sync.Mutex
	heard   func([]float32)
	samples []float32
	stopped chan struct{}
	stop1   sync.Once
}

func (m *bridgeMicrophone) start(heard func([]float32)) error {
	m.heard = heard
	m.s.mu.Lock()
	defer m.s.mu.Unlock()
	m.s.mic = m
	// What was told belongs to the recordings before. The app opened
	// again, as every walk opens it, numbers its recordings from 1.
	m.s.recording, m.s.told, m.s.hearing, m.s.spoken = 0, Dictation{}, false, 0
	m.s.turn.Broadcast()
	return nil
}

func (m *bridgeMicrophone) stop() {
	m.stop1.Do(func() {
		m.mu.Lock()
		defer m.mu.Unlock()
		close(m.stopped)
	})
}

func (m *bridgeMicrophone) over() bool {
	select {
	case <-m.stopped:
		return true
	default:
		return false
	}
}

// hand hands over the next tenth of a second, the pause and the phrase
// after it once a phrase is said, or nothing once the person has fallen
// silent and the phrase they were on is said. It says false once the
// microphone has stopped.
func (m *bridgeMicrophone) hand() bool {
	m.mu.Lock()
	defer m.mu.Unlock()
	if m.over() {
		return false
	}
	if len(m.samples) == 0 {
		if m.s.silent.Load() {
			select {
			case m.s.still <- struct{}{}:
			default:
			}
			return true
		}
		m.samples = slices.Concat(hush(1), tone(1.5))
	}
	n := min(engine.SampleRate/10, len(m.samples))
	m.heard(m.samples[:n])
	m.samples = m.samples[n:]
	return true
}

// A walk that asks for steps when no recording is coming gives up, and the
// bridge gives up with it rather than wait into the next walk.
func TestAStepWithNoRecordingGivesUpWithItsRequest(t *testing.T) {
	s := newSound()
	ctx, cancel := context.WithCancel(t.Context())
	answered := make(chan error)
	go func() {
		_, err := s.until(ctx, "tenth")
		answered <- err
	}()
	cancel()
	if err := <-answered; !errors.Is(err, context.Canceled) {
		t.Fatalf("a step with no recording answered %v, want it to give up with its request", err)
	}
}
