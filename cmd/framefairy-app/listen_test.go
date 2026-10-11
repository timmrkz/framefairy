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
// words, never what they were. Plan row 2.189.
func TestWhatIsSaidIsWrittenDown(t *testing.T) {
	f, read := listenWith(t, tone(3))
	id, err := f.Listen()
	if err != nil {
		t.Fatal(err)
	}
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
	has(t, find(lines, "the recording was written down"), map[string]any{"from": "listen", "seconds": 3, "words": 5})
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
	if d, err := f.StopListening(); len(d.Said) != 0 || err != nil {
		t.Errorf("a recording was left running: %q, %v", d.Said, err)
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
func dictating(t *testing.T) (*FrameFairy, *talkingMicrophone, chan Dictation) {
	t.Helper()
	f, _ := listenWith(t, nil)
	mic := &talkingMicrophone{}
	openMicrophone = func() microphone { return mic }
	told := make(chan Dictation, 100)
	f.listening.say = func(d Dictation) { told <- d }
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
