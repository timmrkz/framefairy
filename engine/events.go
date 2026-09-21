package engine

import (
	"encoding/json"
	"io"
	"math"
	"sync"
	"time"
)

// EventKind says what an event reports.
type EventKind string

// The kinds of event a run produces. Every line the log writes becomes one
// event, and progress becomes events too, so an app can show everything the
// command line shows without reading text.
const (
	EventInfo       EventKind = "info"
	EventOK         EventKind = "ok"
	EventWarn       EventKind = "warn"
	EventError      EventKind = "error"
	EventDetail     EventKind = "detail"
	EventStepStart  EventKind = "step-start"
	EventStepDone   EventKind = "step-done"
	EventStepFailed EventKind = "step-failed"
	EventProgress   EventKind = "progress"
	EventIdle       EventKind = "idle"
)

// Unknown marks a fraction or a remaining time that cannot be given yet.
const Unknown = -1.0

// Event is one thing that happened during a run.
type Event struct {
	Kind EventKind `json:"kind"`
	// Stage is the step the event belongs to, empty outside a step.
	Stage string `json:"stage,omitempty"`
	// Text is plain, without colour codes.
	Text string `json:"text"`
	// Fraction is the share done, from 0 to 1, or Unknown.
	Fraction float64 `json:"fraction"`
	// Remaining is the estimated seconds left, or Unknown.
	Remaining float64 `json:"remaining"`
	// Covered is the second of the episode the work has reached, where
	// that means anything. A transcription sets it on every chunk it
	// finishes, which is far more often than it saves what it has, so the
	// range picker can follow the transcript as it grows.
	Covered float64 `json:"covered,omitempty"`
	// Duration is how long a finished or failed step took, in seconds.
	Duration float64 `json:"duration,omitempty"`
	// Elapsed is the seconds since the log was made.
	Elapsed float64   `json:"elapsed"`
	Time    time.Time `json:"time"`
}

// Sink receives events. It is called from whatever goroutine produced the
// event, one call at a time, and should return quickly.
type Sink func(Event)

// SetSink makes the log pass every event to fn. Nil switches it off.
func (l *Log) SetSink(fn Sink) {
	l.sinkMu.Lock()
	defer l.sinkMu.Unlock()
	l.sink = fn
}

// sane makes a number one JSON can carry. A share worked out from a
// duration nobody could measure is 0/0, and a rate from no elapsed time is
// an infinity, and either of those in an event is an event that cannot be
// encoded at all. The window would then stop hearing about that job
// entirely, progress and finish alike, and the button it was started from
// would say it is working for ever. Whatever cannot be a number is simply
// not known.
func sane(v float64) float64 {
	if math.IsNaN(v) || math.IsInf(v, 0) {
		return Unknown
	}
	return v
}

func (l *Log) send(ev Event) {
	// Every event goes to the window through here, so this is the place
	// that answers for what an event may carry.
	ev.Fraction = sane(ev.Fraction)
	ev.Remaining = sane(ev.Remaining)
	if ev.Covered = sane(ev.Covered); ev.Covered == Unknown {
		ev.Covered = 0
	}
	if ev.Duration = sane(ev.Duration); ev.Duration == Unknown {
		ev.Duration = 0
	}
	l.sinkMu.Lock()
	defer l.sinkMu.Unlock()
	if l.sink == nil {
		return
	}
	// Idle is only worth sending after progress, because the log clears
	// progress far more often than it shows any.
	switch ev.Kind {
	case EventProgress:
		l.busy = true
	case EventIdle:
		if !l.busy {
			return
		}
		l.busy = false
	}
	now := time.Now()
	ev.Time = now
	ev.Elapsed = roundTo(now.Sub(l.t0).Seconds(), 3)
	if ev.Stage == "" {
		ev.Stage = l.currentStage()
	}
	l.sink(ev)
}

func (l *Log) currentStage() string {
	l.stageMu.Lock()
	defer l.stageMu.Unlock()
	if len(l.stages) == 0 {
		return ""
	}
	return l.stages[len(l.stages)-1]
}

func (l *Log) pushStage(name string) {
	l.stageMu.Lock()
	defer l.stageMu.Unlock()
	l.stages = append(l.stages, name)
}

func (l *Log) popStage() {
	l.stageMu.Lock()
	defer l.stageMu.Unlock()
	if len(l.stages) > 0 {
		l.stages = l.stages[:len(l.stages)-1]
	}
}

// JSONLines returns a sink that writes each event as one line of JSON. The
// command line uses it for --events.
func JSONLines(w io.Writer) Sink {
	var mu sync.Mutex
	enc := json.NewEncoder(w)
	enc.SetEscapeHTML(false)
	return func(ev Event) {
		mu.Lock()
		defer mu.Unlock()
		_ = enc.Encode(ev)
	}
}
