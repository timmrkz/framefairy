package main

import (
	"errors"
	"math"
	"os"
	"path/filepath"
	"strings"
	"sync"
	"sync/atomic"
	"testing"
	"testing/synctest"
	"time"

	"github.com/rs/zerolog"
	"github.com/spf13/afero"

	"framefairy/asr"
	"framefairy/engine"
)

// playingMicrophone stands in for the microphone: it hands over all its
// samples a tenth of a second at a time, from a goroutine of its own the
// way the system's does, before it can be stopped.
type playingMicrophone struct {
	samples []float32
	stopped chan struct{}
	done    chan struct{}
}

func (m *playingMicrophone) start(heard func([]float32)) error {
	m.stopped, m.done = make(chan struct{}), make(chan struct{})
	go func() {
		defer close(m.done)
		for at := 0; at < len(m.samples); at += engine.SampleRate / 10 {
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
	return listenTo(t, samples), read
}

// listenTo is listenWith with no log open, so the lines a recording writes
// go nowhere. A test in a testing/synctest bubble opens its log outside
// the bubble, see bubbleLog.
func listenTo(t *testing.T, samples []float32) *FrameFairy {
	t.Helper()
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
	// A recording still being written down when the test ends wrote its
	// last lines into a log whose folder was being removed, and on the
	// Mac the removal then found the folder not empty, issue #441. So
	// the test lets go of the log only once nothing of a recording runs.
	t.Cleanup(func() {
		f.CancelListening()
		f.listening.running.Wait()
	})
	return f
}

// bubbleLog opens the app's log for a test whose recording runs in a
// testing/synctest bubble, and writes its first line, outside the bubble.
// A bubble fails its test with a deadlock when a goroutine started in it
// is still blocked as the test ends. lumberjack starts one with the first
// line it writes, and it never ends, so that line is written here.
func bubbleLog(t *testing.T) func() []map[string]any {
	t.Helper()
	_, read := testLog(t)
	theLog.line(zerolog.InfoLevel, "test").Msg("the log is open")
	return read
}

// What the person says into the microphone is written down by the speech
// model and handed back, and the log says how long it was and how many
// words, never what they were, under the act that started it. Plan row
// 2.189.
func TestWhatIsSaidIsWrittenDown(t *testing.T) {
	read := bubbleLog(t)
	synctest.Test(t, func(t *testing.T) {
		f := listenTo(t, tone(3))
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
	})
}

// A microphone the app may not use gives nothing but zeros on a Mac, and
// that is said, rather than an empty report. Without a speech model the
// microphone does not start at all, so nobody talks for a minute to be
// told so at the end. Plan row 2.189.
//
// On the real disk: a speech model is a folder the speech library loads.
func TestListeningSaysWhyNothingWasWrittenDown(t *testing.T) {
	bubbleLog(t)
	synctest.Test(t, func(t *testing.T) {
		f := listenTo(t, make([]float32, 2*engine.SampleRate))
		if _, err := f.Listen(); err != nil {
			t.Fatal(err)
		}
		if _, err := f.StopListening(); err == nil || !strings.Contains(err.Error(), "heard nothing") {
			t.Errorf("silence gives %v, want that the microphone heard nothing", err)
		}

		standIns.Store(nil)
		started := false
		openMicrophone = func() microphone { started = true; return &playingMicrophone{} }
		f = &FrameFairy{store: memoryStore()}
		f.store.settings.ASRModel = t.TempDir()
		if _, err := f.Listen(); err != errNoSpeechModel || started {
			t.Errorf("without a speech model Listen gives %v and the microphone started %v", err, started)
		}
	})
}

// The settings name no speech model folder until somebody types one in,
// and the installed model is then the one in its usual place, as for a
// search. Tim pressed Dictate with Parakeet installed and was told it was
// not. Plan row 2.189.
//
// On the real disk: a speech model is a folder the speech library loads.
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
	f := &FrameFairy{store: memoryStore()}
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
	bubbleLog(t)
	synctest.Test(t, func(t *testing.T) {
		f := listenTo(t, tone(1))
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
	})
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
// gives what the window is told about it. Its log is not open, so it can
// run in a testing/synctest bubble.
func dictating(t *testing.T, set ...func(*listening)) (*FrameFairy, *talkingMicrophone, chan Dictation) {
	t.Helper()
	f := listenTo(t, nil)
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

// awaitTold waits for the window to be told something that passes ok. A
// window never told is ended by go test's -timeout, which shows where it
// waited.
func awaitTold(t *testing.T, told chan Dictation, ok func(Dictation) bool) Dictation {
	t.Helper()
	for {
		if d := <-told; ok(d) {
			return d
		}
	}
}

// The words appear while they are said, the way dictation on claude.ai
// does it: what follows the last pause is heard again and again as it
// grows, and what came before it is settled and never changes again.
// Stopping gives every piece, the last included. Tim asked for it.
// Plan row 2.189.
func TestWordsAppearWhileTheyAreSaid(t *testing.T) {
	synctest.Test(t, func(t *testing.T) {
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
	})
}

// Dropping a recording drops what was said: the recording ends at once, the window is
// told nothing more about it, and nothing is left to stop. Plan row 2.189.
func TestDroppingARecording(t *testing.T) {
	synctest.Test(t, func(t *testing.T) {
		f, mic, told := dictating(t)
		mic.say(tone(2))
		awaitTold(t, told, func(d Dictation) bool { return d.Hearing != "" })
		f.listening.mu.Lock()
		r := f.listening.now
		f.listening.mu.Unlock()
		f.CancelListening()
		mic.say(tone(2))
		// The window is told only from the goroutine that writes the words
		// down, which closes over when it ends. Once it has, what it said
		// before it heard of the drop is all there will be. Waiting 100 ms
		// missed whatever came later than that. Writing the words down that
		// goes on after the drop is ended by go test's -timeout.
		<-r.over
		// What was on its way as the recording was dropped.
		for len(told) > 0 {
			<-told
		}
		mic.say(tone(1))
		if len(told) != 0 {
			t.Errorf("the window was told %v after the recording was dropped", <-told)
		}
		if d, err := f.StopListening(); len(d.Said) != 0 || err != nil {
			t.Errorf("stopping after dropping gave %q, %v", d.Said, err)
		}
	})
}

// A speech model that cannot load ends the recording while the person is
// still talking, and says why, rather than letting them talk on for
// nothing. Plan row 2.189.
func TestListeningSaysAtOnceWhenTheSpeechModelFails(t *testing.T) {
	bubbleLog(t)
	synctest.Test(t, func(t *testing.T) {
		f := listenTo(t, tone(1))
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
		again, err := f.Listen()
		if err != nil {
			t.Fatalf("listening again after the model failed: %v", err)
		}
		d = awaitTold(t, told, func(d Dictation) bool { return d.Recording == again && d.Error != "" })
		if !strings.Contains(d.Error, "the file is damaged") {
			t.Errorf("told the second time %q", d.Error)
		}
	})
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
	synctest.Test(t, func(t *testing.T) {
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
	})
}

// voices stands in for a speech model that hears a word in every 0.6 s
// somebody talks in and nothing in a quiet room. Held, each pass waits
// until it is let go, and says when it has started.
type voices struct {
	mu   sync.Mutex
	held chan struct{}
	in   chan struct{}
	// slow is how long each pass takes, nothing when zero. Only a test in
	// a testing/synctest bubble sets it, whose clock is fake.
	slow time.Duration
	// heard counts the times it was asked.
	heard atomic.Int32
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
	v.heard.Add(1)
	v.mu.Lock()
	held, in := v.held, v.in
	v.mu.Unlock()
	if held != nil {
		in <- struct{}{}
		<-held
	}
	if v.slow > 0 {
		time.Sleep(v.slow) //nolint:forbidigo // synctest bubble
	}
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
// the way a real one never stops sending, 20 ms of it every 20 ms of the
// clock of the testing/synctest bubble it runs in.
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
			case <-time.After(20 * time.Millisecond): //nolint:forbidigo // synctest bubble
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
//
// The bubble's clock is fake, so the quiet passes the moment everything
// waits on it, on any machine. It is three passes long rather than two
// minutes, which the room fills with 6000 pieces of its noise and took
// over a second.
func TestListeningStopsByItself(t *testing.T) {
	synctest.Test(t, func(t *testing.T) {
		// The longest a recording can be is short too, so a quiet stop
		// that is broken fails at once, on the wrong reason, rather than
		// after ten minutes of the bubble's clock.
		f, mic, told := dictatingTo(t, &voices{}, func(l *listening) {
			l.quiet = 3 * listenEvery
			l.most = 30 * engine.SampleRate
		})
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
	})
}

// Stop pressed while a recording that stopped by itself is still being
// written down waits for it and gives every word, rather than nothing.
// The review of #190 found the last piece lost that way. Plan row 2.189.
//
// In the bubble the quiet of two minutes passes on a fake clock, and
// synctest.Wait says when Stop is waiting for the words.
func TestStopWhileItStopsByItself(t *testing.T) {
	synctest.Test(t, func(t *testing.T) {
		v := &voices{}
		f, mic, told := dictatingTo(t, v)
		mic.say(tone(1.2))
		awaitTold(t, told, func(d Dictation) bool { return d.Hearing != "" })
		// No more sound comes, so the next pass is the last one, once the
		// recording stops by itself.
		held, in := v.hold()
		<-in
		stopped := make(chan Dictation)
		go func() {
			d, err := f.StopListening()
			if err != nil {
				t.Error(err)
			}
			stopped <- d
		}()
		// The last pass ends only once Stop has taken the recording in
		// hand and waits for its words.
		synctest.Wait()
		f.listening.mu.Lock()
		taken := f.listening.now == nil
		f.listening.mu.Unlock()
		if !taken {
			t.Fatal("Stop is waiting, and the recording is still in hand")
		}
		close(held)
		if d := <-stopped; strings.Join(d.Said, " ") != "wort wort" {
			t.Errorf("Stop gave %q", d.Said)
		}
	})
}

// A microphone that hands over nothing but zeros is said while it
// records, so a Mac that may not use it says so before the person has
// talked for minutes, and once it hears the room the window is told that
// too. The review of #190 asked for it. Plan row 2.189.
func TestAMicrophoneThatHearsNothingIsSaidAtOnce(t *testing.T) {
	synctest.Test(t, func(t *testing.T) {
		f, mic, told := dictatingTo(t, &voices{})
		defer f.CancelListening()
		mic.say(make([]float32, listenUnheard+engine.SampleRate/10))
		d := awaitTold(t, told, func(d Dictation) bool { return d.Unheard != "" })
		if !strings.Contains(d.Unheard, "heard nothing") {
			t.Errorf("told %q", d.Unheard)
		}
		mic.say(tone(1))
		awaitTold(t, told, func(d Dictation) bool { return d.Unheard == "" })
	})
}

// A click in the words still being heard settles them there and then,
// as the window shows them, so the cursor can go in and they can be
// corrected while the person talks on, and what follows is heard from
// where they ended. Tim could not click into them. Plan row 2.189.
func TestClickingWhatIsHeardSettlesIt(t *testing.T) {
	synctest.Test(t, func(t *testing.T) {
		f, mic, told := dictatingTo(t, &voices{})
		mic.say(tone(2))
		d := awaitTold(t, told, func(d Dictation) bool { return d.Hearing == "wort wort wort" })
		f.SettleListening(d.Recording, 0, d.Hearing)
		d = awaitTold(t, told, func(d Dictation) bool { return d.Asked == 1 })
		if strings.Join(d.Said, "|") != "wort wort wort" || d.Hearing != "" {
			t.Errorf("after the click the window was told %q settled and %q heard", d.Said, d.Hearing)
		}
		mic.say(tone(1.4))
		awaitTold(t, told, func(d Dictation) bool { return d.Hearing != "" })
		stopped, err := f.StopListening()
		if err != nil {
			t.Fatal(err)
		}
		if strings.Join(stopped.Said, "|") != "wort wort wort|wort wort" {
			t.Errorf("stopping gave %q, want what followed the click alone after it", stopped.Said)
		}
	})
}

// The words the window settles may have been settled in part on the Go
// side since it showed them, at a pause. The window's piece takes the
// place of that one, which it holds, so nothing is written twice.
// Plan row 2.189.
func TestClickingWhatWasHeardBeforeAPause(t *testing.T) {
	synctest.Test(t, func(t *testing.T) {
		f, mic, told := dictatingTo(t, &voices{})
		mic.say(tone(2))
		shown := awaitTold(t, told, func(d Dictation) bool { return d.Hearing == "wort wort wort" })
		mic.say(hush(1), tone(1.3))
		awaitTold(t, told, func(d Dictation) bool { return len(d.Said) == 1 })
		f.SettleListening(shown.Recording, 0, shown.Hearing)
		awaitTold(t, told, func(d Dictation) bool { return d.Asked == 1 })
		stopped, err := f.StopListening()
		if err != nil {
			t.Fatal(err)
		}
		if strings.Join(stopped.Said, "|") != "wort wort wort|wort wort" {
			t.Errorf("stopping gave %q", stopped.Said)
		}
	})
}

// A new line typed at the end of what was said cuts the recording where
// it was typed. What was said before it is the next piece, though the
// speech model had not heard it yet, and what is said after it is heard
// on its own. Tim said a lot quickly and pressed Shift+Enter, and the
// words written down after went below the new line, issue #573.
func TestANewLineCutsWhatWasSaidWhereItWasTyped(t *testing.T) {
	levels, passes := make(chan time.Time), make(chan time.Time)
	measured, passed := make(chan struct{}, 1), make(chan struct{}, 1)
	f, mic, told := dictatingTo(t, &voices{}, func(l *listening) {
		l.beats = &listenBeats{levels: levels, passes: passes, measured: measured, passed: passed}
	})
	defer f.CancelListening()
	// Three words, and no pass of the speech model since: it is behind.
	mic.say(tone(1.8))
	// A second Listen gives the recording in hand.
	recording, err := f.Listen()
	if err != nil {
		t.Fatal(err)
	}
	f.BreakListening(recording, 0)
	// Two words after the new line, with no pause between, so only the
	// cut parts them.
	mic.say(tone(1.2))
	passes <- time.Time{}
	<-passed
	d := awaitTold(t, told, func(d Dictation) bool { return d.Asked == 1 })
	if strings.Join(d.Said, "|") != "wort wort wort" {
		t.Errorf("after the new line the window was told %q settled, want the three words said before it", d.Said)
	}
	stopped, err := f.StopListening()
	if err != nil {
		t.Fatal(err)
	}
	if strings.Join(stopped.Said, "|") != "wort wort wort|wort wort" {
		t.Errorf("stopping gave %q, want the words before the new line and the words after it apart", stopped.Said)
	}
}

// A cut after a piece settled at a pause writes down only what was said
// since that piece, and a second cut after a moment nobody spoke in makes
// an empty piece without asking the speech model, which on a slow
// machine would hold up every answer after it. The review of #574 found
// the pieces' ends unread by any test.
func TestACutAfterAPauseHoldsOnlyWhatFollowedIt(t *testing.T) {
	levels, passes := make(chan time.Time), make(chan time.Time)
	measured, passed := make(chan struct{}, 1), make(chan struct{}, 1)
	v := &voices{}
	f, mic, told := dictatingTo(t, v, func(l *listening) {
		l.beats = &listenBeats{levels: levels, passes: passes, measured: measured, passed: passed}
	})
	defer f.CancelListening()
	recording, err := f.Listen()
	if err != nil {
		t.Fatal(err)
	}
	pass := func() {
		passes <- time.Time{}
		<-passed
	}
	// Three words and a pause, settled as a piece.
	mic.say(tone(1.8), hush(1.2))
	pass()
	awaitTold(t, told, func(d Dictation) bool { return len(d.Said) == 1 })
	// Two words and a cut, a moment of nobody speaking and a second cut,
	// and two words more.
	mic.say(tone(1.2))
	f.BreakListening(recording, 1)
	mic.say(hush(0.6))
	f.BreakListening(recording, 2)
	mic.say(tone(1.2))
	heard := v.heard.Load()
	pass()
	d := awaitTold(t, told, func(d Dictation) bool { return d.Asked == 2 })
	if strings.Join(d.Said, "|") != "wort wort wort|wort wort|" {
		t.Errorf("after the two cuts the window was told %q settled", d.Said)
	}
	// The first cut and the pass after the cuts, nothing for the second.
	if n := v.heard.Load() - heard; n != 2 {
		t.Errorf("the pass with the two cuts asked the speech model %d times, want 2", n)
	}
	stopped, err := f.StopListening()
	if err != nil {
		t.Fatal(err)
	}
	if strings.Join(stopped.Said, "|") != "wort wort wort|wort wort||wort wort" {
		t.Errorf("stopping gave %q", stopped.Said)
	}
}

// A machine whose speech model is slower than the words should appear is
// said in the log once, and the line at the end says how many passes
// there were and how long the slowest took, so a report from a slower
// machine shows why the words fell behind. The review of #190 asked for
// it. Plan row 2.189.
//
// Each pass takes three times listenEvery on the fake clock of a
// testing/synctest bubble, so the speech model is slower than the words on
// any machine, and by exactly as much.
func TestASlowSpeechModelIsSaidOnce(t *testing.T) {
	read := bubbleLog(t)
	synctest.Test(t, func(t *testing.T) {
		f := listenTo(t, nil)
		mic := &talkingMicrophone{}
		openMicrophone = func() microphone { return mic }
		v := &voices{slow: 3 * listenEvery}
		fakes := func(e *engine.Engine, o *engine.Options) {
			e.OpenRecognizer = func(string) (engine.Recognizer, error) { return v, nil }
		}
		standIns.Store(&fakes)
		if _, err := f.Listen(); err != nil {
			t.Fatal(err)
		}
		// A person talks on, so a pass is taken between one piece and the
		// next.
		for range 5 {
			mic.say(tone(0.3))
			time.Sleep(4 * listenEvery) //nolint:forbidigo // synctest bubble
		}
		if _, err := f.StopListening(); err != nil {
			t.Fatal(err)
		}
	})
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
	if written == nil || written["passes"].(float64) < 2 || written["slowest"].(float64) < float64((3*listenEvery).Milliseconds()) {
		t.Errorf("the recording was written down as %v", written)
	}
}

// The window is told how loud the microphone is, a few times a second, so
// the button can move with the voice and the person sees they are heard.
// Plan row 2.189.
//
// The bubble's clock is fake, so each piece said is followed by exactly
// one tenth of a second in which the window is told once.
func TestTheMicrophoneSaysHowLoud(t *testing.T) {
	synctest.Test(t, func(t *testing.T) {
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
		// told is how loud the window was told the microphone is, a tenth
		// of a second after the samples were said into it, -1 for never.
		told := func(samples []float32) float64 {
			mic.say(samples)
			time.Sleep(100 * time.Millisecond) //nolint:forbidigo // synctest bubble
			synctest.Wait()
			mu.Lock()
			defer mu.Unlock()
			if len(levels) == 0 {
				return -1
			}
			return levels[len(levels)-1]
		}
		if l := told(tone(0.1)); l <= 0.5 {
			t.Errorf("a voice was told as %v", l)
		}
		if l := told(hush(0.1)); l < 0 || l >= 0.2 {
			t.Errorf("a quiet room was told as %v", l)
		}
	})
}

// A recording handed beats from outside reads how loud the microphone is
// on each level beat and writes down what is said on each pass beat, and
// says once each reading and each pass is over, after it told the window
// what it found.
// The bridge hands them, so a walk hears a recording at its own pace,
// #551.
func TestARecordingRunsOnTheBeatsItIsHanded(t *testing.T) {
	levels, passes := make(chan time.Time), make(chan time.Time)
	measured, passed := make(chan struct{}, 1), make(chan struct{}, 1)
	loud := make(chan float64, 10)
	f, mic, told := dictating(t, func(l *listening) {
		l.beats = &listenBeats{levels: levels, passes: passes, measured: measured, passed: passed}
		l.loud = func(n Loudness) { loud <- n.Level }
	})
	defer f.CancelListening()
	// Each reading says it is over once the window has been told it, and
	// reads what was said since the one before.
	mic.say(tone(0.1))
	levels <- time.Time{}
	<-measured
	select {
	case l := <-loud:
		if l <= 0.5 {
			t.Errorf("a voice was told as %v on a level beat", l)
		}
	default:
		t.Error("the reading was over before the window was told how loud")
	}
	mic.say(hush(0.1))
	levels <- time.Time{}
	<-measured
	if l := <-loud; l >= 0.2 {
		t.Errorf("a quiet room after a voice was told as %v", l)
	}
	mic.say(tone(1))
	passes <- time.Time{}
	<-passed
	select {
	case d := <-told:
		if d.Hearing == "" {
			t.Errorf("the pass told %+v, with nothing heard", d)
		}
	default:
		t.Error("the pass was over before it told the window what it heard")
	}
	// Each pass says so once it is over, the next one too.
	mic.say(tone(1))
	passes <- time.Time{}
	<-passed
}

// A recording goes with the report window it was started in. Tim closed
// the window with Cmd+W while he talked, the recording ran on, and the
// next window took it up again with every word he had said. Plan row
// 2.244.
func TestClosingTheReportWindowDropsTheRecording(t *testing.T) {
	read := bubbleLog(t)
	synctest.Test(t, func(t *testing.T) {
		f := listenTo(t, tone(30))
		f.report = newReporting(afero.NewMemMapFs(), "/config/report")
		f.report.stop = f.CancelListening
		f.report.make = func(closed func()) (func(), func()) { return func() {}, closed }
		f.OpenReport("bugs")
		first, err := f.Listen()
		if err != nil {
			t.Fatal(err)
		}
		f.report.closeWindow("window")
		if find(read(), "the recording was dropped") == nil {
			t.Fatal("the window closed and the recording ran on")
		}
		f.OpenReport("bugs")
		next, err := f.Listen()
		if err != nil {
			t.Fatal(err)
		}
		if next == first {
			t.Fatalf("the next window took up recording %d again", first)
		}
		f.CancelListening()
	})
}

// A recording nobody spoke into is written down as nothing, though the
// speech model hears words in the hum of a room. Tim pressed Dictate and
// Stop a few times in a row, said nothing, and was given "yeah yeah".
// Plan row 2.244.
func TestNobodySpeakingIsWrittenDownAsNothing(t *testing.T) {
	read := bubbleLog(t)
	synctest.Test(t, func(t *testing.T) {
		room := tone(3)
		for i := range room {
			room[i] /= 30
		}
		f := listenTo(t, room)
		if _, err := f.Listen(); err != nil {
			t.Fatal(err)
		}
		f.listening.mu.Lock()
		r := f.listening.now
		f.listening.mu.Unlock()
		// Stopping stops the microphone before what was said is written
		// down, and the stand-in hands over all it has before it stops, so the
		// whole room is heard however busy the machine is. It was once given
		// 200 ms to play it all.
		d, err := f.StopListening()
		if err != nil {
			t.Fatal(err)
		}
		if heard := len(r.recorded()); heard != len(room) {
			t.Fatalf("%d samples of the room were heard, want all %d", heard, len(room))
		}
		if len(d.Said) != 0 {
			t.Fatalf("the hum of a room was written down as %q", d.Said)
		}
		if find(read(), "nobody spoke, so nothing was written down") == nil {
			t.Fatal("the log does not say nobody spoke")
		}
	})
}
