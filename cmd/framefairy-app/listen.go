package main

import (
	"cmp"
	"encoding/binary"
	"errors"
	"fmt"
	"math"
	"runtime"
	"slices"
	"strings"
	"sync"
	"sync/atomic"
	"time"

	"github.com/gen2brain/malgo"
	"github.com/rs/zerolog"

	"framefairy/asr"
	"framefairy/engine"
)

// Report a Problem's microphone: what the person says is recorded by the
// app and written down by the speech model it already has, into the
// report, where they correct it. Plan row 2.189, docs/LOGGING.md.
//
// The app records, not the window. WKWebView on macOS asks for the
// microphone in a box of its own on every launch, naming the page as
// wails://wails, and Wails has no way to answer it on a Mac. A recording
// made by the app gets the one prompt from macOS, with the words of
// NSMicrophoneUsageDescription in scripts/bundle-macos.sh, and macOS
// remembers the answer.

// listenMost is the longest recording kept, ten minutes, 38 MB of samples.
// A report is a few sentences, so this is only a bound.
const listenMost = 10 * 60 * engine.SampleRate

// microphone records, mono at engine.SampleRate. The tests put in one that
// plays samples, see openMicrophone.
type microphone interface {
	// start hands what it records to heard, from a goroutine of its own,
	// until stop.
	start(heard func([]float32)) error
	stop()
}

// openMicrophone gives the microphone a recording uses.
var openMicrophone = func() microphone { return &systemMicrophone{} }

// listening holds the recording in hand, at most one at a time.
type listening struct {
	mu    sync.Mutex
	now   *recording
	count int
	// say tells the window what is being written down, see Dictation.
	say func(Dictation)
	// every is how often what is being said is written down again, 0 for
	// listenEvery. The tests take it shorter.
	every time.Duration
}

// listenEvery is how often the words appear while somebody talks. A pass
// writes down at most dictateLongest seconds, which the speech model does
// in a fraction of this on a Mac, so the words keep up with the voice.
const listenEvery = 400 * time.Millisecond

// Dictation is what the window is told while somebody dictates. Said is
// every piece written down for good so far, in order, and Hearing is what
// follows them, written down again on every pass as more of it is heard.
// Every message carries all the pieces, because an event the window
// misses is gone, so a missed one costs a piece nothing. Plan row 2.189.
type Dictation struct {
	Recording int      `json:"recording"`
	Said      []string `json:"said"`
	Hearing   string   `json:"hearing"`
	// Error is why the recording ended by itself.
	Error string `json:"error,omitempty"`
}

// recording is one press of the button, from Listen to StopListening or
// CancelListening. Each keeps what it heard apart, so a recording started
// while the one before is still being written down never takes its sound.
type recording struct {
	id      int
	mic     microphone
	started time.Time
	// What was recorded, behind a lock of its own, since the microphone
	// hands it over from a thread of its own. It only ever grows, so a
	// slice of it taken under the lock can be read after.
	mu      sync.Mutex
	samples []float32
	// ended is closed when the person stops, kept says whether what they
	// said is wanted, and written is what the hearing gives at the end.
	ended   chan struct{}
	kept    bool
	dropped atomic.Bool
	written chan writtenDown
}

type writtenDown struct {
	said []string
	err  error
}

func (r *recording) heard(samples []float32) {
	r.mu.Lock()
	defer r.mu.Unlock()
	room := listenMost - len(r.samples)
	r.samples = append(r.samples, samples[:min(len(samples), max(room, 0))]...)
}

func (r *recording) recorded() []float32 {
	r.mu.Lock()
	defer r.mu.Unlock()
	return r.samples
}

// openSpeech loads one copy of the speech model, which is enough for a
// few sentences, or the stand-in the tests put in.
func openSpeech(dir string) (engine.Recognizer, error) {
	if f := standIns.Load(); f != nil {
		var e engine.Engine
		var o engine.Options
		(*f)(&e, &o)
		return e.OpenRecognizer(dir)
	}
	pool, err := asr.OpenCopies(dir, 1, asr.Options{})
	if err != nil {
		return nil, err
	}
	return pool, nil
}

// errNoSpeechModel is what Listen says when there is nothing to write
// down with.
var errNoSpeechModel = errors.New("the speech model is not installed. Install it in Settings, then try again.")

// Listen starts recording what the person says for a report, and gives
// the recording's number, which every Dictation about it carries. The
// speech model loads while they talk and writes down what they say as
// they say it. A second call while one records gives the same number.
func (f *FrameFairy) Listen() (int, error) {
	l := &f.listening
	l.mu.Lock()
	defer l.mu.Unlock()
	if l.now != nil {
		return l.now.id, nil
	}
	// The folder the settings name, or the installed model's usual place
	// when they name none, the way a search finds it.
	dir := ""
	if f.store != nil {
		dir = f.store.Settings().ASRModel
	}
	if dir == "" {
		dir = engine.DefaultModelDir()
	}
	if standIns.Load() == nil && asr.Check(dir) != nil {
		return 0, errNoSpeechModel
	}
	l.count++
	r := &recording{id: l.count, mic: openMicrophone(), started: time.Now(),
		ended: make(chan struct{}), written: make(chan writtenDown, 1)}
	if err := r.mic.start(r.heard); err != nil {
		theLog.line(zerolog.WarnLevel, "listen").Err(err).Msg("the microphone could not start")
		return 0, fmt.Errorf("the microphone could not start: %s", err)
	}
	l.now = r
	every := cmp.Or(l.every, listenEvery)
	go l.write(r, dir, every)
	theLog.line(zerolog.InfoLevel, "listen").Msg("recording for a report")
	return r.id, nil
}

// take ends the recording in hand, if it is r, or whichever it is when r
// is nil, and gives it.
func (l *listening) take(r *recording) *recording {
	l.mu.Lock()
	defer l.mu.Unlock()
	now := l.now
	if now == nil || (r != nil && now != r) {
		return nil
	}
	l.now = nil
	return now
}

// tell says how a recording is coming along, unless it was dropped.
func (l *listening) tell(r *recording, d Dictation) {
	if r.dropped.Load() || l.say == nil {
		return
	}
	d.Recording = r.id
	l.say(d)
}

// write writes down what is said while the recording runs, a pass every
// so often: what came before the last pause is settled and never written
// again, and what follows it is heard again on the next pass, with more
// of it. When the person stops, the rest is written down.
func (l *listening) write(r *recording, dir string, every time.Duration) {
	var out writtenDown
	defer func() { r.written <- out }()
	defer func() {
		if caught := recover(); caught != nil {
			theLog.line(zerolog.ErrorLevel, "listen").Interface("panic", caught).Msg("the speech model stopped unexpectedly")
			out = writtenDown{err: fmt.Errorf("the speech model stopped unexpectedly: %v", caught)}
			l.give(r, out.err)
		}
	}()
	rec, err := openSpeech(dir)
	if err != nil {
		theLog.line(zerolog.ErrorLevel, "listen").Err(err).Msg("the speech model could not load")
		out.err = fmt.Errorf("the speech model could not load: %s", err)
		l.give(r, out.err)
		return
	}
	defer rec.Close()
	var said []string
	settled, passed, hearing := 0, 0, ""
	tick := time.NewTicker(every)
	defer tick.Stop()
	for {
		select {
		case <-r.ended:
			out = l.finish(r, rec, said, settled)
			return
		case <-tick.C:
		}
		samples := r.recorded()
		// Nothing new since the last pass, or too little yet to be a word.
		if len(samples)-passed < listenLeast/2 || len(samples)-settled < listenLeast {
			continue
		}
		passed = len(samples)
		cut := engine.Settled(samples[settled:])
		before, now := heardAround(rec, samples, settled, settled+cut)
		changed := false
		if cut > 0 {
			if before != "" {
				said = append(said, before)
				changed = true
			}
			settled += cut
		}
		if changed || now != hearing {
			hearing = now
			l.tell(r, Dictation{Said: slices.Clone(said), Hearing: now})
		}
	}
}

// listenAround is how much of what was settled is heard again before
// what follows it, in samples. Written down on its own, a piece starts
// with a capital and ends with a full stop wherever it was cut, so the
// speech model is given the words around it too: the last few seconds
// before it, and all that follows it.
const listenAround = 5 * engine.SampleRate

// heardAround writes down what was said from settled on, in two: what
// came before cut, and what came after it. A cut at or before settled
// leaves the first empty. The words are told apart by when they start,
// and a cut is always in a pause, where no word is.
func heardAround(rec engine.Recognizer, samples []float32, settled, cut int) (before, after string) {
	from := max(settled-listenAround, 0)
	if len(samples)-settled < listenLeast {
		return "", ""
	}
	// A word that starts this close before settled was cut off there and
	// belongs to what follows.
	const slack = 0.05
	start := float64(settled-from)/engine.SampleRate - slack
	end := float64(cut-from) / engine.SampleRate
	var first, second []engine.Cue
	for _, w := range engine.HearWords(rec, samples[from:]) {
		switch {
		case w.Start < start:
		case cut > settled && w.Start < end:
			first = append(first, w)
		default:
			second = append(second, w)
		}
	}
	return engine.Joined(first), engine.Joined(second)
}

// give ends a recording that cannot go on, and says why.
func (l *listening) give(r *recording, err error) {
	if l.take(r) != nil {
		r.mic.stop()
	}
	l.tell(r, Dictation{Error: err.Error()})
}

// finish writes down the rest of a recording the person stopped.
func (l *listening) finish(r *recording, rec engine.Recognizer, said []string, settled int) writtenDown {
	if !r.kept {
		return writtenDown{}
	}
	samples := r.recorded()
	seconds := float64(len(samples)) / engine.SampleRate
	if len(samples) < listenLeast {
		return writtenDown{}
	}
	if silent(samples) {
		theLog.line(zerolog.WarnLevel, "listen").Float64("seconds", seconds).Msg("the microphone gave only silence")
		return writtenDown{err: errors.New(heardNothing())}
	}
	hearing := time.Now()
	if _, rest := heardAround(rec, samples, settled, settled); rest != "" {
		said = append(said, rest)
	}
	// How much and how long, never what was said: that goes into the
	// report only if the person keeps it there.
	words := 0
	for _, piece := range said {
		words += len(strings.Fields(piece))
	}
	theLog.line(zerolog.InfoLevel, "listen").Float64("seconds", math.Round(seconds*10)/10).
		Int("words", words).Int("pieces", len(said)).Int64("ms", time.Since(hearing).Milliseconds()).
		Dur("recorded", time.Since(r.started).Round(time.Millisecond)).Msg("the recording was written down")
	return writtenDown{said: said}
}

// listenLeast is the shortest recording written down, a quarter of a
// second. Anything shorter is a click on the button, not something said.
const listenLeast = engine.SampleRate / 4

// StopListening ends the recording and gives what was said, written down,
// every piece of it, the last included. Without a recording it gives
// nothing.
func (f *FrameFairy) StopListening() (Dictation, error) {
	l := &f.listening
	r := l.take(nil)
	if r == nil {
		return Dictation{Said: []string{}}, nil
	}
	r.mic.stop()
	r.kept = true
	close(r.ended)
	out := <-r.written
	if out.err != nil {
		return Dictation{Recording: r.id, Said: []string{}}, out.err
	}
	return Dictation{Recording: r.id, Said: append([]string{}, out.said...)}, nil
}

// CancelListening ends the recording and drops what was said, at once,
// without waiting for the speech model. Escape on Report a Problem.
func (f *FrameFairy) CancelListening() {
	l := &f.listening
	r := l.take(nil)
	if r == nil {
		return
	}
	r.dropped.Store(true)
	r.mic.stop()
	close(r.ended)
	theLog.line(zerolog.InfoLevel, "listen").Msg("the recording was dropped")
}

// silent says whether a recording is nothing but zeros. A microphone that
// is there always hears a little noise, and macOS hands over zeros when
// the app is not allowed to use it.
func silent(samples []float32) bool {
	for _, s := range samples {
		if s != 0 {
			return false
		}
	}
	return true
}

func heardNothing() string {
	if runtime.GOOS == "darwin" {
		return "the microphone heard nothing. If Frame Fairy may not use it, allow it in System Settings, Privacy & Security, Microphone."
	}
	return "the microphone heard nothing. Check that one is connected and that Frame Fairy may use it."
}

// systemMicrophone is the machine's own microphone, the one the system
// has chosen, through miniaudio: Core Audio on a Mac, WASAPI on Windows,
// PulseAudio or ALSA on Linux. It asks for the speech model's own format,
// so miniaudio does the converting.
type systemMicrophone struct {
	ctx *malgo.AllocatedContext
	dev *malgo.Device
}

func (m *systemMicrophone) start(heard func([]float32)) error {
	ctx, err := malgo.InitContext(nil, malgo.ContextConfig{}, nil)
	if err != nil {
		return err
	}
	config := malgo.DefaultDeviceConfig(malgo.Capture)
	config.Capture.Format = malgo.FormatF32
	config.Capture.Channels = 1
	config.SampleRate = engine.SampleRate
	dev, err := malgo.InitDevice(ctx.Context, config, malgo.DeviceCallbacks{
		Data: func(_, in []byte, frames uint32) {
			samples := make([]float32, min(int(frames), len(in)/4))
			for i := range samples {
				samples[i] = math.Float32frombits(binary.LittleEndian.Uint32(in[i*4:]))
			}
			heard(samples)
		},
	})
	if err != nil {
		_ = ctx.Uninit()
		ctx.Free()
		return err
	}
	if err := dev.Start(); err != nil {
		dev.Uninit()
		_ = ctx.Uninit()
		ctx.Free()
		return err
	}
	m.ctx, m.dev = ctx, dev
	return nil
}

func (m *systemMicrophone) stop() {
	m.dev.Uninit()
	_ = m.ctx.Uninit()
	m.ctx.Free()
}
