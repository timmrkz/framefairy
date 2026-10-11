package main

import (
	"math"
	"strings"
	"sync"
	"testing"

	"framefairy/engine"
)

// playingMicrophone stands in for the microphone: it hands over its
// samples a tenth of a second at a time, from a goroutine of its own the
// way the system's does, and has handed over all of them by the time it
// is stopped.
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
	s := &speech{}
	fakes := func(e *engine.Engine, o *engine.Options) { e.OpenRecognizer = s.open }
	standIns.Store(&fakes)
	was := openMicrophone
	openMicrophone = func() microphone { return &playingMicrophone{samples: samples} }
	t.Cleanup(func() {
		standIns.Store(nil)
		openMicrophone = was
	})
	return &FrameFairy{}, read
}

// What the person says into the microphone is written down by the speech
// model and handed back, and the log says how long it was and how many
// words, never what they were. Plan row 2.189.
func TestWhatIsSaidIsWrittenDown(t *testing.T) {
	f, read := listenWith(t, tone(3))
	if err := f.Listen(); err != nil {
		t.Fatal(err)
	}
	if err := f.Listen(); err != nil {
		t.Errorf("a second Listen while one records: %v", err)
	}
	text, err := f.StopListening()
	if err != nil {
		t.Fatal(err)
	}
	// The stand-in hears a word every 0.6 s.
	if text != "wort wort wort wort wort" {
		t.Errorf("written down as %q", text)
	}
	lines := read()
	has(t, find(lines, "the recording was written down"), map[string]any{"from": "listen", "seconds": 3, "words": 5})
	for _, l := range lines {
		if strings.Contains(l["msg"].(string), "wort") {
			t.Errorf("the log holds what was said: %v", l)
		}
	}
	if text, err := f.StopListening(); text != "" || err != nil {
		t.Errorf("stopping with nothing recording gives %q, %v", text, err)
	}
}

// A microphone the app may not use gives nothing but zeros on a Mac, and
// that is said, rather than an empty report. Without a speech model the
// microphone does not start at all, so nobody talks for a minute to be
// told so at the end. Plan row 2.189.
func TestListeningSaysWhyNothingWasWrittenDown(t *testing.T) {
	f, _ := listenWith(t, make([]float32, 2*engine.SampleRate))
	if err := f.Listen(); err != nil {
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
	if err := f.Listen(); err != errNoSpeechModel || started {
		t.Errorf("without a speech model Listen gives %v and the microphone started %v", err, started)
	}
}

// The button can be pressed again and again, from a window that sends its
// calls without waiting: every recording ends with words or nothing, never
// a hang, a panic or a race. Plan row 2.189.
func TestListeningAtOnce(t *testing.T) {
	f, _ := listenWith(t, tone(1))
	var wg sync.WaitGroup
	for range 8 {
		wg.Go(func() {
			for range 10 {
				_ = f.Listen()
				if text, err := f.StopListening(); err != nil || (text != "" && !strings.HasPrefix(text, "wort")) {
					t.Errorf("gave %q, %v", text, err)
				}
			}
		})
	}
	wg.Wait()
	if text, err := f.StopListening(); text != "" || err != nil {
		t.Errorf("a recording was left running: %q, %v", text, err)
	}
}
