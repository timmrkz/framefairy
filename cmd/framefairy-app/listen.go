package main

import (
	"encoding/binary"
	"errors"
	"fmt"
	"math"
	"runtime"
	"strings"
	"sync"
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
	mu  sync.Mutex
	now *recording
}

// recording is one press of the button, from Listen to StopListening. Each
// keeps what it heard apart, so a recording started while the one before
// is still being written down never takes its sound.
type recording struct {
	mic microphone
	// The speech model loads while the person talks, so writing it down
	// waits only for the hearing.
	speech  chan loadedSpeech
	started time.Time
	// What was recorded, behind a lock of its own, since the microphone
	// hands it over from a thread of its own.
	mu      sync.Mutex
	samples []float32
}

type loadedSpeech struct {
	rec engine.Recognizer
	err error
}

func (r *recording) heard(samples []float32) {
	r.mu.Lock()
	defer r.mu.Unlock()
	room := listenMost - len(r.samples)
	r.samples = append(r.samples, samples[:min(len(samples), max(room, 0))]...)
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

// Listen starts recording what the person says for a report, and loads
// the speech model while they talk. A second call while one records does
// nothing.
func (f *FrameFairy) Listen() error {
	l := &f.listening
	l.mu.Lock()
	defer l.mu.Unlock()
	if l.now != nil {
		return nil
	}
	dir := ""
	if f.store != nil {
		dir = f.store.Settings().ASRModel
	}
	if standIns.Load() == nil && asr.Check(dir) != nil {
		return errNoSpeechModel
	}
	r := &recording{mic: openMicrophone(), speech: make(chan loadedSpeech, 1), started: time.Now()}
	if err := r.mic.start(r.heard); err != nil {
		theLog.line(zerolog.WarnLevel, "listen").Err(err).Msg("the microphone could not start")
		return fmt.Errorf("the microphone could not start: %s", err)
	}
	go func() {
		defer func() {
			if caught := recover(); caught != nil {
				r.speech <- loadedSpeech{err: fmt.Errorf("the speech model stopped unexpectedly: %v", caught)}
			}
		}()
		rec, err := openSpeech(dir)
		r.speech <- loadedSpeech{rec, err}
	}()
	l.now = r
	theLog.line(zerolog.InfoLevel, "listen").Msg("recording for a report")
	return nil
}

// listenLeast is the shortest recording written down, a quarter of a
// second. Anything shorter is a click on the button, not something said.
const listenLeast = engine.SampleRate / 4

// StopListening ends the recording and gives what was said, written down.
// Without a recording it gives nothing.
func (f *FrameFairy) StopListening() (text string, err error) {
	l := &f.listening
	l.mu.Lock()
	r := l.now
	l.now = nil
	l.mu.Unlock()
	if r == nil {
		return "", nil
	}
	r.mic.stop()
	r.mu.Lock()
	samples := r.samples
	r.mu.Unlock()
	seconds := float64(len(samples)) / engine.SampleRate
	loaded := <-r.speech
	if loaded.err != nil {
		theLog.line(zerolog.ErrorLevel, "listen").Err(loaded.err).Msg("the speech model could not load")
		return "", fmt.Errorf("the speech model could not load: %s", loaded.err)
	}
	defer loaded.rec.Close()
	if len(samples) < listenLeast {
		return "", nil
	}
	if silent(samples) {
		theLog.line(zerolog.WarnLevel, "listen").Float64("seconds", seconds).Msg("the microphone gave only silence")
		return "", errors.New(heardNothing())
	}
	defer func() {
		if caught := recover(); caught != nil {
			theLog.line(zerolog.ErrorLevel, "listen").Interface("panic", caught).Msg("the speech model stopped unexpectedly")
			text, err = "", fmt.Errorf("the speech model stopped unexpectedly: %v", caught)
		}
	}()
	hearing := time.Now()
	text = engine.Hear(loaded.rec, samples)
	// How much and how long, never what was said: that goes into the
	// report only if the person keeps it there.
	theLog.line(zerolog.InfoLevel, "listen").Float64("seconds", math.Round(seconds*10)/10).
		Int("words", len(strings.Fields(text))).Int64("ms", time.Since(hearing).Milliseconds()).
		Dur("recorded", time.Since(r.started).Round(time.Millisecond)).Msg("the recording was written down")
	return text, nil
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
