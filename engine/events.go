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
	// EventUnderway lists the clips a job has on the way, the whole list
	// every time it changes, see Underway.
	EventUnderway EventKind = "underway"
)

// Underway is a clip on its way into a clip set: proposed, by the model or
// at the playhead, and not written yet. Every job that makes clips says
// which it has on the way the same way, so the app shows them the same way
// whoever proposed them.
type Underway struct {
	// N is which of its job's clips this is, from 1, in the order they
	// were queued to be made. It stays while the clip's step and edges
	// change, so the clip keeps its place.
	N int `json:"n"`
	// Start and End are where it lies in the episode. A clip made by hand
	// is at the playhead until the words there are known, and then Start
	// and End are the same moment.
	Start float64 `json:"start"`
	End   float64 `json:"end"`
	// Title is what it is called, once that is known.
	Title string `json:"title,omitempty"`
	// Step is what is being done to it: StepWaiting, StepHearing or
	// StepFraming.
	Step string `json:"step"`
	// Pieces are what of it is kept, from and to, once its pauses are cut,
	// which is before its crop is placed. The app draws them and lays the
	// captions of the words said in them out while the crop is placed, see
	// ArrivingCaptionsView, so the clip is built in front of the person
	// rather than appearing whole.
	Pieces [][2]float64 `json:"pieces,omitempty"`
	// Clip is the key it will have in the clip list once it is written,
	// its clip set's file name and its id, from the moment it has an id.
	// The list keeps one row for it from its card to the clip, so a card
	// becomes its clip in place rather than one row going and another
	// coming.
	Clip string `json:"clip,omitempty"`
}

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
	// From is where the part of the episode the work covers began, so what
	// it has reached is From to Covered. A transcription hears in parts,
	// wherever a search or a clip made by hand needs them first.
	From float64 `json:"from,omitempty"`
	// Found is how many clips a search has written to its plan so far. The
	// app reads the list again when it changes, rather than on a timer
	// that is always a little late. On EventUnderway it is how many the
	// job has written, sent with the clips it has on the way.
	Found int `json:"found,omitempty"`
	// Underway is every clip the job has on the way, on EventUnderway.
	Underway []Underway `json:"underway,omitempty"`
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
// encoded at all. The app would then stop hearing about that job
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
	// Every event goes to the app through here, so this is the place
	// that answers for what an event may carry.
	ev.Fraction = sane(ev.Fraction)
	ev.Remaining = sane(ev.Remaining)
	if ev.Covered = sane(ev.Covered); ev.Covered == Unknown {
		ev.Covered = 0
	}
	if ev.From = sane(ev.From); ev.From == Unknown {
		ev.From = 0
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
