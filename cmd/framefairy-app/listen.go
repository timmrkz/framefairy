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

	"framefairy/engine/timed"

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
	// last is how the latest recording ended when it ended by itself,
	// until StopListening hands it over or another recording starts. The
	// person may press Stop while it is being written down, and Stop then
	// gives this, so a recording's end reaches the window whichever way
	// it goes.
	last *Dictation
	// say tells the window what is being written down, see Dictation.
	say func(Dictation)
	// loud tells the window how loud the microphone is, see Loudness.
	loud func(Loudness)
	// every is how often what is being said is written down again, 0 for
	// listenEvery, quiet how long a recording goes on with no new words,
	// 0 for listenQuiet, and most the longest a recording is, in samples,
	// 0 for listenMost. The tests take them shorter.
	every time.Duration
	quiet time.Duration
	most  int
	// still, when set, says when the quiet is over, in place of quiet of
	// the clock: a recording stops by itself for want of words the pass
	// after it says so. The bridge sets it, so a walk does not wait the
	// quiet out, see bridge_test.go.
	still <-chan struct{}
	// beats, when set, stands in for the two clocks of a recording, see
	// listenBeats. The bridge sets it, so a walk hears a recording at its
	// own pace rather than at the pace of the app's timers, #551.
	beats *listenBeats
	// running is every goroutine a recording started, which goes on for a
	// moment after the recording ends and may still write a line to the
	// log. A test waits on it before it lets go of what they use.
	running sync.WaitGroup
}

// listenBeats is what a recording runs on in place of its two clocks: a
// reading of how loud the microphone is for each value on levels, in place
// of ten a second, and a pass of the speech model for each on passes, in
// place of one every listenEvery. Once a reading or a pass is over it says
// so on measured or passed, when there is room, so whoever hands the beats
// knows the window has been told what it found, and that the next sound
// goes into the next reading.
type listenBeats struct {
	levels, passes   <-chan time.Time
	measured, passed chan<- struct{}
}

// listenQuiet is how long a recording goes on with no new words before it
// stops by itself and keeps what was said. Tim left one running and came
// back to find it had heard nothing for minutes. One minute is too short
// to think about what to say next, he said, so it is two.
const listenQuiet = 2 * time.Minute

// Loudness is how loud the microphone is, from 0 for a quiet room to 1
// for a raised voice, told to the window ten times a second while it
// records, so the button can move with the voice. Plan row 2.189.
type Loudness struct {
	Recording int     `json:"recording"`
	Level     float64 `json:"level"`
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
	// Error is why the recording could not go on, and Ended why it
	// stopped by itself, with Said all of what was said.
	Error string `json:"error,omitempty"`
	Ended string `json:"ended,omitempty"`
	// Unheard is why the microphone has handed over nothing but zeros
	// for a while, said while it records, so a Mac that may not use it
	// says so before the person has talked for minutes. It goes on
	// recording, since the prompt that allows it may still be open.
	Unheard string `json:"unheard,omitempty"`
	// Asked is how many of the window's SettleListening calls are in
	// Said, so it can tell an answer from before its last one.
	Asked int `json:"asked"`
}

// listenUnheard is how long a microphone may hand over nothing but zeros
// before the window says so, in samples. One that is there hears its
// room at once.
const listenUnheard = 2 * engine.SampleRate

// recording is one press of the button, from Listen to StopListening or
// CancelListening. Each keeps what it heard apart, so a recording started
// while the one before is still being written down never takes its sound.
type recording struct {
	id      int
	mic     microphone
	started time.Time
	// act is what the person did that started it, which its log lines
	// carry, not whatever they are at by the time it is written down.
	act string
	// What was recorded, behind a lock of its own, since the microphone
	// hands it over from a thread of its own. It only ever grows, so a
	// slice of it taken under the lock can be read after.
	mu      sync.Mutex
	samples []float32
	most    int
	// hushed stops the microphone once, whoever ends the recording.
	hushed sync.Once
	// What the window asked to be settled, see SettleListening, behind
	// mu, taken by the goroutine that writes it down.
	settles []settle
	// How many passes the speech model made and the slowest, kept by the
	// goroutine that writes it down, for the log.
	passes  int
	slowest time.Duration
	// The loudest the microphone was since it was last asked, in dB, and
	// whether the recording is as long as it may be.
	peak float64
	full atomic.Bool
	// voiced says whether the microphone was ever as loud as a voice,
	// listenVoice. Until it was, nobody spoke, and nothing is written down.
	voiced atomic.Bool
	// over is closed once the recording is written down or dropped.
	over chan struct{}
	// ended is closed when the person stops, kept says whether what they
	// said is wanted, and written is what the hearing gives at the end.
	ended   chan struct{}
	kept    bool
	dropped atomic.Bool
	written chan writtenDown
}

// settle is the window's ask to settle the words it shows as being heard,
// text, as the piece after the first pieces it has.
type settle struct {
	pieces int
	text   string
}

type writtenDown struct {
	said []string
	err  error
}

func (r *recording) heard(samples []float32) {
	r.mu.Lock()
	defer r.mu.Unlock()
	room := r.most - len(r.samples)
	r.samples = append(r.samples, samples[:min(len(samples), max(room, 0))]...)
	if len(r.samples) >= r.most {
		r.full.Store(true)
	}
	power := 0.0
	for _, s := range samples {
		power += float64(s) * float64(s)
	}
	if len(samples) > 0 {
		db := 10 * math.Log10(power/float64(len(samples))+1e-20)
		r.peak = max(r.peak, db)
		if db >= listenVoice {
			r.voiced.Store(true)
		}
	}
}

// listenVoice is how loud the microphone has to be, in dB, for somebody to
// be speaking into it: 10 dB over a quiet room, see loudness. The speech
// model hears words in a room where nobody speaks. Tim pressed Dictate and
// Stop a few times in a row, said nothing, and was given "yeah yeah".
const listenVoice = -45.0

// loudness is how loud the microphone was since it was last asked, from
// 0 at -55 dB, a quiet room, to 1 at -15 dB, a raised voice.
func (r *recording) loudness() float64 {
	r.mu.Lock()
	defer r.mu.Unlock()
	db := r.peak
	r.peak = -200
	return math.Round(min(max((db+55)/40, 0), 1)*100) / 100
}

// line starts a log line about the recording.
func (r *recording) line(level zerolog.Level) *zerolog.Event {
	return withAct(theLog.lineAt(level, "listen", time.Now()), r.act).Int("recording", r.id)
}

// hush stops the microphone, the first time it is asked.
func (r *recording) hush() { r.hushed.Do(r.mic.stop) }

func (r *recording) recorded() []float32 {
	r.mu.Lock()
	defer r.mu.Unlock()
	return r.samples
}

// openSpeech loads the speech model for a recording, the way a job gets
// it, but one copy of it rather than the pool a transcription hears with,
// see asr.OpenOne.
func openSpeech(dir string) (engine.Recognizer, error) {
	var e engine.Engine
	setUp(&e, &engine.Options{})
	if standIns.Load() == nil {
		e.OpenRecognizer = asr.OpenOne
	}
	return e.OpenRecognizer(dir)
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
	l.last = nil
	r := &recording{id: l.count, mic: openMicrophone(), started: time.Now(), act: theLog.acting(), most: cmp.Or(l.most, listenMost),
		peak: -200, over: make(chan struct{}), ended: make(chan struct{}), written: make(chan writtenDown, 1)}
	if err := r.mic.start(r.heard); err != nil {
		theLog.line(zerolog.WarnLevel, "listen").Err(err).Msg("the microphone could not start")
		return 0, fmt.Errorf("the microphone could not start: %s", err)
	}
	l.now = r
	every := cmp.Or(l.every, listenEvery)
	quiet := cmp.Or(l.quiet, listenQuiet)
	l.running.Go(func() { l.write(r, dir, every, quiet) })
	l.running.Go(func() { l.measure(r) })
	r.line(zerolog.InfoLevel).Msg("recording for a report")
	return r.id, nil
}

// take ends whichever recording is in hand and gives it, with how the
// one before ended if it ended by itself and nobody has been told.
func (l *listening) take() (*recording, *Dictation) {
	l.mu.Lock()
	defer l.mu.Unlock()
	r, last := l.now, l.last
	l.now, l.last = nil, nil
	return r, last
}

// endItself ends the recording in hand, if it is still r, keeps how it
// ended for StopListening, and tells the window. When the person stopped
// or dropped it meanwhile, it is theirs: StopListening gives it, or
// nobody wants it.
func (l *listening) endItself(r *recording, d Dictation) bool {
	d.Recording = r.id
	l.mu.Lock()
	if l.now != r {
		l.mu.Unlock()
		return false
	}
	l.now = nil
	l.last = &d
	l.mu.Unlock()
	r.hush()
	l.tell(r, d)
	return true
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
func (l *listening) write(r *recording, dir string, every, quiet time.Duration) {
	var out writtenDown
	defer close(r.over)
	defer func() { r.written <- out }()
	defer func() {
		if caught := recover(); caught != nil {
			r.line(zerolog.ErrorLevel).Interface("panic", caught).Msg("the speech model stopped unexpectedly")
			out = writtenDown{err: fmt.Errorf("the speech model stopped unexpectedly: %v", caught)}
			l.endItself(r, Dictation{Said: []string{}, Error: out.err.Error()})
		}
	}()
	rec, err := openSpeech(dir)
	if err != nil {
		r.line(zerolog.ErrorLevel).Err(err).Msg("the speech model could not load")
		out.err = fmt.Errorf("the speech model could not load: %s", err)
		l.endItself(r, Dictation{Said: []string{}, Error: out.err.Error()})
		return
	}
	defer rec.Close()
	var said []string
	settled, passed, hearing, unheard, asked := 0, 0, "", "", 0
	worded := time.Now()
	// The last few things told as being heard, and how far into the
	// recording each was heard, so words the window settles are settled
	// where they ended.
	type heard struct {
		text string
		end  int
	}
	var told []heard
	// settle takes what the window asked to be settled: its words become
	// the next piece, where it has them, in place of any piece settled
	// since, which they hold, and what follows them is heard from where
	// they ended.
	settle := func() bool {
		r.mu.Lock()
		asks := r.settles
		r.settles = nil
		r.mu.Unlock()
		for _, ask := range asks {
			end := passed
			for _, t := range slices.Backward(told) {
				if t.text == ask.text {
					end = t.end
					break
				}
			}
			said = append(said[:min(ask.pieces, len(said))], ask.text)
			settled, hearing, told = end, "", nil
			asked++
		}
		return len(asks) > 0
	}
	tick := time.NewTicker(every)
	defer tick.Stop()
	pass := tick.C
	if l.beats != nil {
		pass = l.beats.passes
	}
	for first := true; ; first = false {
		if !first && l.beats != nil {
			select {
			case l.beats.passed <- struct{}{}:
			default:
			}
		}
		select {
		case <-r.ended:
			settle()
			out = l.finish(r, rec, said, settled, r.kept)
			return
		case <-pass:
		}
		if settle() {
			worded = time.Now()
			l.tell(r, Dictation{Said: slices.Clone(said), Unheard: unheard, Asked: asked})
		}
		// Nobody has said anything for a long while, or the recording is
		// as long as it may be: it stops by itself and keeps what was
		// said. It stays the recording in hand until it is written down,
		// so Stop pressed meanwhile waits for it rather than finding
		// nothing.
		why := ""
		hushed := time.Since(worded) > quiet
		if l.still != nil {
			select {
			case <-l.still:
				hushed = true
			default:
				hushed = false
			}
		}
		if hushed {
			why = fmt.Sprintf("stopped listening after %s without a word", minutes(quiet))
		} else if r.full.Load() {
			why = fmt.Sprintf("stopped listening after %s, the longest a recording can be", minutes(time.Duration(r.most)*time.Second/engine.SampleRate))
		}
		if why != "" {
			r.hush()
			settle()
			out = l.finish(r, rec, said, settled, true)
			if l.endItself(r, Dictation{Said: append([]string{}, out.said...), Ended: why, Error: errText(out.err)}) {
				r.line(zerolog.InfoLevel).Str("why", why).Msg("the recording stopped by itself")
			}
			return
		}
		samples := r.recorded()
		// Nothing new since the last pass, or too little yet to be a word.
		if len(samples)-passed < listenLeast/2 || len(samples)-settled < listenLeast {
			continue
		}
		passed = len(samples)
		// The speech model is not asked until somebody speaks.
		cut, before, now := 0, "", ""
		if r.voiced.Load() {
			cut = engine.Settled(samples[settled:])
			began := time.Now()
			before, now = heardAround(rec, samples, settled, settled+cut)
			r.paced(time.Since(began), len(samples)-max(settled-listenAround, 0), every)
		}
		changed := false
		if cut > 0 {
			if before != "" {
				said = append(said, before)
				changed = true
			}
			settled += cut
		}
		deaf := ""
		if len(samples) >= listenUnheard && silent(samples) {
			deaf = heardNothing()
		}
		if changed || now != hearing || deaf != unheard {
			if changed || now != "" {
				worded = time.Now()
			}
			hearing, unheard = now, deaf
			told = append(told[max(len(told)-15, 0):], heard{now, passed})
			l.tell(r, Dictation{Said: slices.Clone(said), Hearing: now, Unheard: deaf, Asked: asked})
		}
	}
}

// paced keeps how long a pass took, every one in the recent lines and the
// slowest for the line at the end. A pass slower than every means the
// words fall behind the voice on this machine, which is said once.
func (r *recording) paced(took time.Duration, heard int, every time.Duration) {
	r.passes++
	r.line(zerolog.TraceLevel).Int64("ms", took.Milliseconds()).Float64("heard", math.Round(float64(heard)/engine.SampleRate*10)/10).Msg("a pass of the speech model")
	if took > every && r.slowest <= every {
		r.line(zerolog.WarnLevel).Int64("ms", took.Milliseconds()).Int64("every", every.Milliseconds()).
			Float64("heard", math.Round(float64(heard)/engine.SampleRate*10)/10).Msg("the speech model is slower than the words should appear")
	}
	r.slowest = max(r.slowest, took)
}

// measure tells the window how loud the microphone is until the
// recording is over.
func (l *listening) measure(r *recording) {
	tick := time.NewTicker(100 * time.Millisecond)
	defer tick.Stop()
	level := tick.C
	if l.beats != nil {
		level = l.beats.levels
	}
	for {
		select {
		case <-r.over:
			return
		case <-r.ended:
			return
		case <-level:
			if l.loud != nil && !r.dropped.Load() {
				l.loud(Loudness{Recording: r.id, Level: r.loudness()})
			}
			if l.beats != nil {
				select {
				case l.beats.measured <- struct{}{}:
				default:
				}
			}
		}
	}
}

// minutes says a duration the way a person would, "2 minutes".
func minutes(d time.Duration) string {
	if m := int(d.Round(time.Minute).Minutes()); m == 1 {
		return "a minute"
	} else if m > 1 {
		return fmt.Sprintf("%d minutes", m)
	}
	return fmt.Sprintf("%d seconds", int(d.Round(time.Second).Seconds()))
}

func errText(err error) string {
	if err == nil {
		return ""
	}
	return err.Error()
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
	var first, second []timed.Cue
	// At most about 17 s, the longest a piece grows before it is settled
	// and listenAround with it, which the speech model takes in one go.
	for _, w := range engine.TokensToWords(rec.Recognize(samples[from:], engine.SampleRate), 0) {
		switch {
		case w.Start < start:
		case cut > settled && w.Start < end:
			first = append(first, w)
		default:
			second = append(second, w)
		}
	}
	return joined(first), joined(second)
}

// joined is words as one text.
func joined(words []timed.Cue) string {
	texts := make([]string, len(words))
	for i, w := range words {
		texts[i] = w.Text
	}
	return strings.Join(texts, " ")
}

// finish writes down the rest of a recording that ended, if what was
// said is kept.
func (l *listening) finish(r *recording, rec engine.Recognizer, said []string, settled int, kept bool) writtenDown {
	if !kept || r.dropped.Load() {
		return writtenDown{}
	}
	samples := r.recorded()
	seconds := float64(len(samples)) / engine.SampleRate
	if len(samples) < listenLeast {
		return writtenDown{}
	}
	if silent(samples) {
		r.line(zerolog.WarnLevel).Float64("seconds", seconds).Msg("the microphone gave only silence")
		return writtenDown{err: errors.New(heardNothing())}
	}
	if !r.voiced.Load() {
		r.line(zerolog.InfoLevel).Float64("seconds", math.Round(seconds*10)/10).Msg("nobody spoke, so nothing was written down")
		return writtenDown{}
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
	r.line(zerolog.InfoLevel).Float64("seconds", math.Round(seconds*10)/10).
		Int("words", words).Int("pieces", len(said)).Int64("ms", time.Since(hearing).Milliseconds()).
		Dur("recorded", time.Since(r.started).Round(time.Millisecond)).Int("passes", r.passes).
		Int64("slowest", r.slowest.Milliseconds()).Msg("the recording was written down")
	return writtenDown{said: said}
}

// listenLeast is the shortest recording written down, a quarter of a
// second. Anything shorter is a click on the button, not something said.
const listenLeast = engine.SampleRate / 4

// StopListening ends the recording and gives what was said, written down,
// every piece of it, the last included. When the recording had just
// stopped by itself, it gives that, with why. Without a recording it
// gives nothing.
func (f *FrameFairy) StopListening() (Dictation, error) {
	l := &f.listening
	r, last := l.take()
	if r == nil && last != nil {
		if last.Error != "" {
			return Dictation{Recording: last.Recording, Said: []string{}}, errors.New(last.Error)
		}
		return *last, nil
	}
	if r == nil {
		return Dictation{Said: []string{}}, nil
	}
	r.hush()
	r.kept = true
	close(r.ended)
	out := <-r.written
	if out.err != nil {
		return Dictation{Recording: r.id, Said: []string{}}, out.err
	}
	return Dictation{Recording: r.id, Said: append([]string{}, out.said...)}, nil
}

// SettleListening settles the words the window shows as being heard,
// text, there and then, as the piece after the first pieces it has. A
// click in them asks it, so the cursor can go in and the words can be
// corrected while the person talks on, which Tim could not do. A
// recording that is no longer in hand has nothing to settle.
func (f *FrameFairy) SettleListening(recording, pieces int, text string) {
	l := &f.listening
	l.mu.Lock()
	r := l.now
	if r != nil && r.id == recording && text != "" {
		r.mu.Lock()
		r.settles = append(r.settles, settle{pieces, strings.TrimSpace(text)})
		r.mu.Unlock()
	}
	l.mu.Unlock()
}

// CancelListening ends the recording and drops what was said, at once,
// without waiting for the speech model, when Report a Problem goes while
// it listens. Escape keeps what was said, issue #355.
func (f *FrameFairy) CancelListening() {
	l := &f.listening
	r, _ := l.take()
	if r == nil {
		return
	}
	r.dropped.Store(true)
	r.hush()
	close(r.ended)
	r.line(zerolog.InfoLevel).Msg("the recording was dropped")
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
