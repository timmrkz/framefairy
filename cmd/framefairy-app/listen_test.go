package main

import (
	"errors"
	"math"
	"os"
	"path/filepath"
	"strings"
	"sync"
	"testing"
	"time"

	"framefairy/asr"
	"framefairy/engine"
)

// playingMicrophone stands in for the microphone: it hands over its
// samples a tenth of a second at a time, from a goroutine of its own the
// way the system's does. Live, it hands them over as fast as they would
// be said, until it is stopped. Otherwise it hands over all of them at
// once, before it can be stopped.
type playingMicrophone struct {
	samples []float32
	live    bool
	stopped chan struct{}
	done    chan struct{}
}

func (m *playingMicrophone) start(heard func([]float32)) error {
	m.stopped, m.done = make(chan struct{}), make(chan struct{})
	go func() {
		defer close(m.done)
		for at := 0; at < len(m.samples); at += engine.SampleRate / 10 {
			if m.live {
				select {
				case <-m.stopped:
					return
				case <-time.After(100 * time.Millisecond):
				}
			}
			heard(m.samples[at:min(at+engine.SampleRate/10, len(m.samples))])
		}
		<-m.stopped
	}()
	return nil
}

func (m *playingMicrophone) stop() {
	close(m.stopped)
	<-m.done
}

// tone is seconds of a quiet hum, which a microphone in a room hears.
func tone(seconds float64) []float32 {
	samples := make([]float32, int(seconds*engine.SampleRate))
	for i := range samples {
		samples[i] = float32(0.05 * math.Sin(2*math.Pi*150*float64(i)/engine.SampleRate))
	}
	return samples
}

// listenWith puts in the stand-in speech model and a microphone that
// plays samples, and gives the app to talk to and what its log holds.
func listenWith(t *testing.T, samples []float32) (*FrameFairy, func() []map[string]any) {
	t.Helper()
	_, read := testLog(t)
	s := &speech{}
	fakes := func(e *engine.Engine, o *engine.Options) { e.OpenRecognizer = s.open }
	standIns.Store(&fakes)
	was := openMicrophone
	openMicrophone = func() microphone { return &playingMicrophone{samples: samples} }
	t.Cleanup(func() {
		standIns.Store(nil)
		openMicrophone = was
	})
	f := &FrameFairy{}
	f.listening.every = 10 * time.Millisecond
	return f, read
}

// What the person says into the microphone is written down by the speech
// model and handed back, and the log says how long it was and how many
// words, never what they were, under the act that started it. Plan row
// 2.189.
func TestWhatIsSaidIsWrittenDown(t *testing.T) {
	f, read := listenWith(t, tone(3))
	// Its lines carry the act that started it, not the one after it.
	first, next := "a1", "a2"
	theLog.act.Store(&first)
	id, err := f.Listen()
	if err != nil {
		t.Fatal(err)
	}
	theLog.act.Store(&next)
	if again, err := f.Listen(); err != nil || again != id {
		t.Errorf("a second Listen while one records gives %d, %v, want %d", again, err, id)
	}
	d, err := f.StopListening()
	if err != nil {
		t.Fatal(err)
	}
	// The stand-in hears a word every 0.6 s.
	if text := strings.Join(d.Said, " "); text != "wort wort wort wort wort" || d.Recording != id {
		t.Errorf("recording %d written down as %q", d.Recording, text)
	}
	lines := read()
	has(t, find(lines, "the recording was written down"), map[string]any{"from": "listen", "seconds": 3, "words": 5, "act": "a1", "recording": id})
	for _, l := range lines {
		if strings.Contains(l["msg"].(string), "wort") {
			t.Errorf("the log holds what was said: %v", l)
		}
	}
	if d, err := f.StopListening(); len(d.Said) != 0 || err != nil {
		t.Errorf("stopping with nothing recording gives %q, %v", d.Said, err)
	}
}

// A microphone the app may not use gives nothing but zeros on a Mac, and
// that is said, rather than an empty report. Without a speech model the
// microphone does not start at all, so nobody talks for a minute to be
// told so at the end. Plan row 2.189.
func TestListeningSaysWhyNothingWasWrittenDown(t *testing.T) {
	f, _ := listenWith(t, make([]float32, 2*engine.SampleRate))
	if _, err := f.Listen(); err != nil {
		t.Fatal(err)
	}
	if _, err := f.StopListening(); err == nil || !strings.Contains(err.Error(), "heard nothing") {
		t.Errorf("silence gives %v, want that the microphone heard nothing", err)
	}

	standIns.Store(nil)
	started := false
	openMicrophone = func() microphone { started = true; return &playingMicrophone{} }
	f = &FrameFairy{store: &store{dir: t.TempDir(), settings: defaultSettings()}}
	f.store.settings.ASRModel = t.TempDir()
	if _, err := f.Listen(); err != errNoSpeechModel || started {
		t.Errorf("without a speech model Listen gives %v and the microphone started %v", err, started)
	}
}

// The settings name no speech model folder until somebody types one in,
// and the installed model is then the one in its usual place, as for a
// search. Tim pressed Dictate with Parakeet installed and was told it was
// not. Plan row 2.189.
func TestListeningFindsTheInstalledSpeechModel(t *testing.T) {
	listenWith(t, nil)
	standIns.Store(nil)
	t.Setenv("HOME", t.TempDir())
	t.Setenv("FRAMEFAIRY_ASR_MODEL", "")
	dir := engine.DefaultModelDir()
	if err := os.MkdirAll(dir, 0o755); err != nil {
		t.Fatal(err)
	}
	for _, name := range asr.Files {
		if err := os.WriteFile(filepath.Join(dir, name), []byte("x"), 0o644); err != nil {
			t.Fatal(err)
		}
	}
	// The microphone does not start, so nothing tries to load the files.
	openMicrophone = func() microphone { return brokenMicrophone{} }
	f := &FrameFairy{store: &store{dir: t.TempDir(), settings: defaultSettings()}}
	f.store.settings.ASRModel = ""
	if _, err := f.Listen(); err == nil || err == errNoSpeechModel {
		t.Errorf("with the speech model in its usual place Listen gives %v, want it to reach the microphone", err)
	}
}

// brokenMicrophone is a microphone that cannot start.
type brokenMicrophone struct{}

func (brokenMicrophone) start(func([]float32)) error { return errors.New("no input device") }
func (brokenMicrophone) stop()                       {}

// The button can be pressed again and again, from a window that sends its
// calls without waiting: every recording ends with words or nothing, never
// a hang, a panic or a race. Plan row 2.189.
func TestListeningAtOnce(t *testing.T) {
	f, _ := listenWith(t, tone(1))
	// Some recordings stop by themselves while they are stopped or
	// dropped.
	f.listening.quiet = 30 * time.Millisecond
	var wg sync.WaitGroup
	for range 8 {
		wg.Go(func() {
			for i := range 10 {
				_, _ = f.Listen()
				if i%3 == 0 {
					f.CancelListening()
					continue
				}
				if d, err := f.StopListening(); err != nil || (len(d.Said) > 0 && !strings.HasPrefix(d.Said[0], "wort")) {
					t.Errorf("gave %q, %v", d.Said, err)
				}
			}
		})
	}
	wg.Wait()
	f.listening.mu.Lock()
	left := f.listening.now
	f.listening.mu.Unlock()
	if left != nil {
		t.Errorf("recording %d was left running", left.id)
	}
}

// talkingMicrophone hands over what the test says into it, when it says it.
type talkingMicrophone struct {
	mu    sync.Mutex
	heard func([]float32)
}

func (m *talkingMicrophone) start(heard func([]float32)) error {
	m.mu.Lock()
	defer m.mu.Unlock()
	m.heard = heard
	return nil
}

func (*talkingMicrophone) stop() {}

func (m *talkingMicrophone) say(samples ...[]float32) {
	m.mu.Lock()
	heard := m.heard
	m.mu.Unlock()
	for _, s := range samples {
		heard(s)
	}
}

// hush is seconds of a room with nobody talking in it.
func hush(seconds float64) []float32 {
	samples := tone(seconds)
	for i := range samples {
		samples[i] /= 100
	}
	return samples
}

// dictating starts a recording on a microphone the test talks into, and
// gives what the window is told about it.
func dictating(t *testing.T, set ...func(*listening)) (*FrameFairy, *talkingMicrophone, chan Dictation) {
	t.Helper()
	f, _ := listenWith(t, nil)
	mic := &talkingMicrophone{}
	openMicrophone = func() microphone { return mic }
	told := make(chan Dictation, 100)
	f.listening.say = func(d Dictation) { told <- d }
	for _, s := range set {
		s(&f.listening)
	}
	if _, err := f.Listen(); err != nil {
		t.Fatal(err)
	}
	return f, mic, told
}

// awaitTold waits for the window to be told something that passes ok.
func awaitTold(t *testing.T, told chan Dictation, ok func(Dictation) bool) Dictation {
	t.Helper()
	deadline := time.After(10 * time.Second)
	for {
		select {
		case d := <-told:
			if ok(d) {
				return d
			}
		case <-deadline:
			t.Fatal("the window was never told")
		}
	}
}

// The words appear while they are said, the way dictation on claude.ai
// does it: what follows the last pause is heard again and again as it
// grows, and what came before it is settled and never changes again.
// Stopping gives every piece, the last included. Tim asked for it.
// Plan row 2.189.
func TestWordsAppearWhileTheyAreSaid(t *testing.T) {
	f, mic, told := dictating(t)
	mic.say(tone(2))
	d := awaitTold(t, told, func(d Dictation) bool { return d.Hearing != "" })
	if len(d.Said) != 0 || d.Hearing != "wort wort wort" {
		t.Errorf("while talking the window was told %q settled and %q heard", d.Said, d.Hearing)
	}
	// A pause ends the thought, and the next one is heard on its own.
	mic.say(hush(1), tone(1.3))
	d = awaitTold(t, told, func(d Dictation) bool { return len(d.Said) == 1 && d.Hearing != "" })
	// The stand-in hears a word every 0.6 s, the pause's first half too.
	if d.Said[0] != "wort wort wort wort" {
		t.Errorf("settled %q", d.Said)
	}
	mic.say(tone(1))
	stopped, err := f.StopListening()
	if err != nil {
		t.Fatal(err)
	}
	if len(stopped.Said) != 2 || stopped.Said[0] != "wort wort wort wort" || !strings.HasPrefix(stopped.Said[1], "wort wort wort") {
		t.Errorf("stopping gave %q", stopped.Said)
	}
}

// Escape drops what was said: the recording ends at once, the window is
// told nothing more about it, and nothing is left to stop. Plan row 2.189.
func TestDroppingARecording(t *testing.T) {
	f, mic, told := dictating(t)
	mic.say(tone(2))
	awaitTold(t, told, func(d Dictation) bool { return d.Hearing != "" })
	f.CancelListening()
	mic.say(tone(2))
	time.Sleep(100 * time.Millisecond)
	for len(told) > 0 {
		<-told
	}
	time.Sleep(100 * time.Millisecond)
	if len(told) != 0 {
		t.Errorf("the window was told %v after the recording was dropped", <-told)
	}
	if d, err := f.StopListening(); len(d.Said) != 0 || err != nil {
		t.Errorf("stopping after dropping gave %q, %v", d.Said, err)
	}
}

// A speech model that cannot load ends the recording while the person is
// still talking, and says why, rather than letting them talk on for
// nothing. Plan row 2.189.
func TestListeningSaysAtOnceWhenTheSpeechModelFails(t *testing.T) {
	f, _ := listenWith(t, tone(1))
	broken := &speech{}
	broken.breaks(errors.New("the file is damaged"))
	fakes := func(e *engine.Engine, o *engine.Options) { e.OpenRecognizer = broken.open }
	standIns.Store(&fakes)
	told := make(chan Dictation, 10)
	f.listening.say = func(d Dictation) { told <- d }
	if _, err := f.Listen(); err != nil {
		t.Fatal(err)
	}
	d := awaitTold(t, told, func(d Dictation) bool { return d.Error != "" })
	if !strings.Contains(d.Error, "the file is damaged") {
		t.Errorf("told %q", d.Error)
	}
	if _, err := f.Listen(); err != nil {
		t.Errorf("listening again after the model failed: %v", err)
	}
	f.CancelListening()
}

// sentences stands in for a speech model that writes what it is given as
// one sentence: a word every 0.6 s, the first with a capital and the last
// with a full stop, the way Parakeet ends whatever it is handed.
type sentences struct{}

func (sentences) Recognize(samples []float32, rate int) []engine.Token {
	length := float64(len(samples)) / float64(rate)
	var tokens []engine.Token
	for at := 0.1; at+0.35 < length; at += 0.6 {
		tokens = append(tokens, engine.Token{Text: " wort", Start: at, Duration: 0.3})
	}
	if len(tokens) > 0 {
		tokens[0].Text = " Wort"
		tokens = append(tokens, engine.Token{Text: ".", Start: tokens[len(tokens)-1].Start + 0.3})
	}
	return tokens
}

func (sentences) Close() {}

// A pause settles the words before it, but they are written down with
// what was said around them, so a pause in the middle of a sentence gives
// it no full stop and the words after it no capital. Tim got "there
// should be some text. written right in front of me." Plan row 2.189.
func TestAPauseIsNoFullStop(t *testing.T) {
	f, mic, told := dictating(t)
	fakes := func(e *engine.Engine, o *engine.Options) {
		e.OpenRecognizer = func(string) (engine.Recognizer, error) { return sentences{}, nil }
	}
	standIns.Store(&fakes)
	f.CancelListening()
	if _, err := f.Listen(); err != nil {
		t.Fatal(err)
	}
	mic.say(tone(2), hush(1), tone(1.3))
	d := awaitTold(t, told, func(d Dictation) bool { return len(d.Said) == 1 })
	if d.Said[0] != "Wort wort wort wort" {
		t.Errorf("the words before the pause were written down as %q", d.Said[0])
	}
	mic.say(tone(1))
	stopped, err := f.StopListening()
	if err != nil {
		t.Fatal(err)
	}
	if len(stopped.Said) != 2 || !strings.HasPrefix(stopped.Said[1], "wort ") || !strings.HasSuffix(stopped.Said[1], "wort.") {
		t.Errorf("stopping gave %q, want the second piece to go on the sentence and end it", stopped.Said)
	}
}

// voices stands in for a speech model that hears a word in every 0.6 s
// somebody talks in and nothing in a quiet room. Held, each pass waits
// until it is let go, and says when it has started.
type voices struct {
	mu   sync.Mutex
	held chan struct{}
	in   chan struct{}
	// slow is how long each pass takes, nothing when zero.
	slow time.Duration
}

// hold makes the next passes wait for the channel it gives to be closed,
// and gives one that has a value each time a pass starts.
func (v *voices) hold() (chan struct{}, chan struct{}) {
	v.mu.Lock()
	defer v.mu.Unlock()
	v.held, v.in = make(chan struct{}), make(chan struct{}, 100)
	return v.held, v.in
}

func (v *voices) Recognize(samples []float32, rate int) []engine.Token {
	v.mu.Lock()
	held, in := v.held, v.in
	v.mu.Unlock()
	if held != nil {
		in <- struct{}{}
		<-held
	}
	time.Sleep(v.slow)
	step := rate * 6 / 10
	var tokens []engine.Token
	for at := 0; at+step/2 <= len(samples); at += step {
		part := samples[at:min(at+step, len(samples))]
		power := 0.0
		for _, s := range part {
			power += float64(s) * float64(s)
		}
		if power/float64(len(part)) > 1e-4 {
			tokens = append(tokens, engine.Token{Text: " wort", Start: float64(at)/float64(rate) + 0.1, Duration: 0.3})
		}
	}
	return tokens
}

func (*voices) Close() {}

// dictatingTo is dictating with the speech model heard through v.
func dictatingTo(t *testing.T, v *voices, set ...func(*listening)) (*FrameFairy, *talkingMicrophone, chan Dictation) {
	t.Helper()
	f, mic, told := dictating(t, set...)
	fakes := func(e *engine.Engine, o *engine.Options) {
		e.OpenRecognizer = func(string) (engine.Recognizer, error) { return v, nil }
	}
	standIns.Store(&fakes)
	f.CancelListening()
	if _, err := f.Listen(); err != nil {
		t.Fatal(err)
	}
	return f, mic, told
}

// room keeps a quiet room going into the microphone until the test ends,
// the way a real one never stops sending.
func room(t *testing.T, mic *talkingMicrophone) {
	done := make(chan struct{})
	over := make(chan struct{})
	t.Cleanup(func() {
		close(done)
		<-over
	})
	go func() {
		defer close(over)
		for {
			select {
			case <-done:
				return
			case <-time.After(20 * time.Millisecond):
				mic.say(hush(0.02))
			}
		}
	}()
}

// A recording nobody talks into any more stops by itself and keeps what
// was said, while the room goes on sending its noise, and so does one
// that has reached the longest a recording can be, rather than going on
// listening and hearing nothing, which is what Tim found after ten
// minutes. Stop pressed after that gives the same, once. Plan row 2.189.
func TestListeningStopsByItself(t *testing.T) {
	f, mic, told := dictatingTo(t, &voices{}, func(l *listening) { l.quiet = 300 * time.Millisecond })
	mic.say(tone(1.2))
	room(t, mic)
	d := awaitTold(t, told, func(d Dictation) bool { return d.Ended != "" })
	if !strings.Contains(d.Ended, "without a word") || strings.Join(d.Said, " ") != "wort wort" {
		t.Errorf("after the quiet the window was told %q with %q", d.Ended, d.Said)
	}
	if stopped, err := f.StopListening(); stopped.Ended != d.Ended || strings.Join(stopped.Said, " ") != "wort wort" || stopped.Recording != d.Recording || err != nil {
		t.Errorf("Stop pressed after it stopped by itself gave %+v, %v", stopped, err)
	}
	if stopped, err := f.StopListening(); len(stopped.Said) != 0 || err != nil {
		t.Errorf("Stop pressed again gave %q, %v", stopped.Said, err)
	}

	f, mic, told = dictatingTo(t, &voices{}, func(l *listening) { l.most = 2 * engine.SampleRate })
	mic.say(tone(1), tone(1), tone(1))
	d = awaitTold(t, told, func(d Dictation) bool { return d.Ended != "" })
	if !strings.Contains(d.Ended, "longest") || len(d.Said) == 0 {
		t.Errorf("at the longest the window was told %q with %q", d.Ended, d.Said)
	}
	if _, err := f.Listen(); err != nil {
		t.Errorf("listening again: %v", err)
	}
	f.CancelListening()
}

// Stop pressed while a recording that stopped by itself is still being
// written down waits for it and gives every word, rather than nothing.
// The review of #190 found the last piece lost that way. Plan row 2.189.
func TestStopWhileItStopsByItself(t *testing.T) {
	v := &voices{}
	f, mic, told := dictatingTo(t, v, func(l *listening) { l.quiet = 300 * time.Millisecond })
	mic.say(tone(1.2))
	awaitTold(t, told, func(d Dictation) bool { return d.Hearing != "" })
	// No more sound comes, so the next pass is the last one.
	held, in := v.hold()
	select {
	case <-in:
	case <-time.After(10 * time.Second):
		t.Fatal("the recording never stopped by itself")
	}
	stopped := make(chan Dictation)
	go func() {
		d, err := f.StopListening()
		if err != nil {
			t.Error(err)
		}
		stopped <- d
	}()
	time.Sleep(50 * time.Millisecond)
	close(held)
	if d := <-stopped; strings.Join(d.Said, " ") != "wort wort" {
		t.Errorf("Stop gave %q", d.Said)
	}
}

// A machine whose speech model is slower than the words should appear is
// said in the log once, and the line at the end says how many passes
// there were and how long the slowest took, so a report from a slower
// machine shows why the words fell behind. The review of #190 asked for
// it. Plan row 2.189.
func TestASlowSpeechModelIsSaidOnce(t *testing.T) {
	f, read := listenWith(t, nil)
	mic := &talkingMicrophone{}
	openMicrophone = func() microphone { return mic }
	v := &voices{slow: 30 * time.Millisecond}
	fakes := func(e *engine.Engine, o *engine.Options) {
		e.OpenRecognizer = func(string) (engine.Recognizer, error) { return v, nil }
	}
	standIns.Store(&fakes)
	if _, err := f.Listen(); err != nil {
		t.Fatal(err)
	}
	for range 5 {
		mic.say(tone(0.3))
		time.Sleep(40 * time.Millisecond)
	}
	if _, err := f.StopListening(); err != nil {
		t.Fatal(err)
	}
	lines := read()
	slow := 0
	for _, l := range lines {
		if l["msg"] == "the speech model is slower than the words should appear" {
			slow++
		}
	}
	if slow != 1 {
		t.Errorf("a slow speech model was said %d times", slow)
	}
	written := find(lines, "the recording was written down")
	if written == nil || written["passes"].(float64) < 2 || written["slowest"].(float64) < 30 {
		t.Errorf("the recording was written down as %v", written)
	}
}

// The window is told how loud the microphone is, a few times a second, so
// the button can move with the voice and the person sees they are heard.
// Plan row 2.189.
func TestTheMicrophoneSaysHowLoud(t *testing.T) {
	var mu sync.Mutex
	var levels []float64
	f, mic, _ := dictating(t, func(l *listening) {
		l.loud = func(n Loudness) {
			mu.Lock()
			levels = append(levels, n.Level)
			mu.Unlock()
		}
	})
	defer f.CancelListening()
	heard := func(ok func(float64) bool, samples []float32) bool {
		for range 50 {
			mic.say(samples)
			time.Sleep(20 * time.Millisecond)
			mu.Lock()
			last := -1.0
			if len(levels) > 0 {
				last = levels[len(levels)-1]
			}
			mu.Unlock()
			if last >= 0 && ok(last) {
				return true
			}
		}
		return false
	}
	if !heard(func(l float64) bool { return l > 0.5 }, tone(0.1)) {
		t.Errorf("a voice was told as %v", levels)
	}
	if !heard(func(l float64) bool { return l < 0.2 }, hush(0.1)) {
		t.Errorf("a quiet room was told as %v", levels)
	}
}
