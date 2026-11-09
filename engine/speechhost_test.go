package engine

import (
	"context"
	"io"
	"sync"
	"sync/atomic"
	"testing"

	"github.com/spf13/afero"

	"framefairy/engine/runlog"
)

// countedSpeech is a speech model that counts how often it is loaded and
// closed, and how many of it are loaded now.
type countedSpeech struct {
	loads, closes atomic.Int32
}

func (c *countedSpeech) open(string) (Recognizer, error) {
	c.loads.Add(1)
	return &countedRecognizer{c: c}, nil
}

type countedRecognizer struct {
	c      *countedSpeech
	closed atomic.Bool
}

func (r *countedRecognizer) Recognize([]float32, int) []Token { return nil }

func (r *countedRecognizer) Close() {
	if r.closed.Swap(true) {
		panic("a speech model was closed twice")
	}
	r.c.closes.Add(1)
}

func speechEngine(m *Machine, c *countedSpeech) *Engine {
	e := NewEngine(runlog.New(io.Discard, false, false), m)
	e.OpenRecognizer = c.open
	return e
}

// hearOnce takes the speech model, as a hearing does, and lets it go.
func hearOnce(t *testing.T, e *Engine, dir string) {
	t.Helper()
	rec, release, err := e.speech(context.Background(), dir)
	if err != nil {
		t.Fatal(err)
	}
	rec.Recognize(nil, SampleRate)
	release()
}

// The speech model kept is loaded once, and every hearing after uses it:
// a part of a clip heard in well under a second waited about 5 s for the
// model to load first, every time, Tim's report of 9 November. Not kept,
// it goes when the hearing ends, as it did before.
func TestTheSpeechModelKeptIsLoadedOnce(t *testing.T) {
	t.Parallel()
	var c countedSpeech
	m := NewMachine(afero.NewMemMapFs())
	e := speechEngine(m, &c)
	hearOnce(t, e, "speech")
	hearOnce(t, e, "speech")
	if got := c.loads.Load(); got != 2 {
		t.Fatalf("not kept, two hearings loaded the model %d times, want 2", got)
	}
	opts := DefaultOptions()
	opts.ASRModel = "speech"
	opts.Planner = "api"
	loaded, err := e.KeepModel(context.Background(), opts, t.TempDir())
	if err != nil || !loaded {
		t.Fatalf("keeping the speech model: loaded %v, %v", loaded, err)
	}
	for range 3 {
		hearOnce(t, speechEngine(m, &c), "speech")
	}
	if loaded, _ := e.KeepModel(context.Background(), opts, t.TempDir()); loaded {
		t.Error("the speech model kept was loaded again")
	}
	if got := c.loads.Load(); got != 3 {
		t.Errorf("kept, three more hearings loaded it %d times in all, want 3", got)
	}
	if got := c.closes.Load(); got != 2 {
		t.Errorf("closed %d times while kept, want 2, the two not kept", got)
	}
	m.CloseModels()
	if got := c.closes.Load(); got != 3 {
		t.Errorf("closed %d times once the app quit, want 3", got)
	}
	if loaded, _ := e.KeepModel(context.Background(), opts, t.TempDir()); loaded {
		t.Error("a speech model was kept after the app quit")
	}
}

// A model given up while a hearing holds it stays until that hearing is
// done, and another model kept goes in its place once nobody hears.
func TestTheSpeechModelGivenUpWaitsForTheHearing(t *testing.T) {
	t.Parallel()
	var c countedSpeech
	m := NewMachine(afero.NewMemMapFs())
	e := speechEngine(m, &c)
	if _, err := e.keepSpeech(context.Background(), "one"); err != nil {
		t.Fatal(err)
	}
	rec, release, err := e.speech(context.Background(), "one")
	if err != nil {
		t.Fatal(err)
	}
	m.KeepNone()
	if c.closes.Load() != 0 {
		t.Fatal("the model was closed under a hearing")
	}
	rec.Recognize(nil, SampleRate)
	release()
	release()
	if got := c.closes.Load(); got != 1 {
		t.Fatalf("closed %d times after the hearing, want 1", got)
	}
	if _, err := e.keepSpeech(context.Background(), "one"); err != nil {
		t.Fatal(err)
	}
	if _, err := e.keepSpeech(context.Background(), "two"); err != nil {
		t.Fatal(err)
	}
	if loads, closes := c.loads.Load(), c.closes.Load(); loads != 3 || closes != 2 {
		t.Errorf("another model kept: %d loads and %d closes, want 3 and 2", loads, closes)
	}
}

// Hearings, keeps and gives up from several goroutines at once, the way
// the jobs, the keeper and the settings reach it, leave every model
// loaded closed once nobody holds one and none is kept, and none closed
// under a hearing, which the stand-in panics on.
func TestTheSpeechModelIsSharedByHearingsAtOnce(t *testing.T) {
	t.Parallel()
	var c countedSpeech
	m := NewMachine(afero.NewMemMapFs())
	var wg sync.WaitGroup
	for i := range 40 {
		wg.Go(func() {
			e := speechEngine(m, &c)
			switch i % 5 {
			case 0:
				_, _ = e.keepSpeech(context.Background(), "speech")
			case 1:
				m.KeepNone()
			case 2:
				hearOnce(t, e, "other")
			default:
				hearOnce(t, e, "speech")
			}
		})
	}
	wg.Wait()
	m.CloseModels()
	if loads, closes := c.loads.Load(), c.closes.Load(); loads != closes {
		t.Errorf("%d loads and %d closes once nobody hears and the app quit", loads, closes)
	}
}
