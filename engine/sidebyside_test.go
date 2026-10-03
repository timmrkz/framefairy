package engine

import (
	"bytes"
	"context"
	"math"
	"math/rand/v2"
	"path/filepath"
	"sync"
	"sync/atomic"
	"testing"
	"time"
)

// sideBySide is a recogniser with several copies, the way asr.Pool is, each
// piece taking a time of its own so they finish in any order, the way copies
// on a busy machine do.
type sideBySide struct {
	inner  Recognizer
	copies int
	// The inner recogniser is heard one piece at a time. The waiting is
	// outside it, so the pieces still overtake each other.
	mu   sync.Mutex
	busy atomic.Int32
	most atomic.Int32
	seen func()
	fail int32
	n    atomic.Int32
	// paired closes once two pieces are being heard at once, or once the
	// first gave up waiting for a second.
	pairOnce sync.Once
	makeOnce sync.Once
	paired   chan struct{}
}

func (s *sideBySide) pairedChan() chan struct{} {
	s.makeOnce.Do(func() { s.paired = make(chan struct{}) })
	return s.paired
}

func (s *sideBySide) Copies() int { return s.copies }

func (s *sideBySide) Recognize(samples []float32, rate int) []Token {
	now := s.busy.Add(1)
	defer s.busy.Add(-1)
	// The first piece is held until a second one arrives, so the two are
	// heard at once by how the engine hands them out, not by how fast this
	// machine decodes the next piece while the first one sleeps. An engine
	// that hears one piece at a time never sends the second while the first
	// is held, so the wait has a limit: once it runs out, no piece waits
	// again, and the test fails as it should.
	release := func() { s.pairOnce.Do(func() { close(s.pairedChan()) }) }
	if now >= 2 {
		release()
	} else {
		select {
		case <-s.pairedChan():
		case <-time.After(10 * time.Second):
			release()
		}
	}
	for {
		most := s.most.Load()
		if now <= most || s.most.CompareAndSwap(most, now) {
			break
		}
	}
	if s.fail > 0 && s.n.Add(1) == s.fail {
		panic("a copy of the speech model broke")
	}
	// Each piece takes a time of its own, so they finish in any order.
	time.Sleep(time.Duration(200+rand.IntN(200)) * time.Millisecond)
	s.mu.Lock()
	out := s.inner.Recognize(samples, rate)
	s.mu.Unlock()
	if s.seen != nil {
		s.seen()
	}
	return out
}

func (s *sideBySide) Close() {}

// Heard by four copies at once, an episode comes out word for word as one
// copy hears it, and every save on the way reaches only as far as all of
// it has been heard, never past a piece still being heard.
func TestHearingSideBySideWritesTheSameTranscript(t *testing.T) {
	t.Parallel()
	source := testEpisode(t, "70")
	base := DefaultOptions()
	base.ASRModel = t.TempDir()

	var calls int32
	e := NewEngine(NewLog(&bytes.Buffer{}, false, false))
	e.checkpointEvery = time.Nanosecond
	e.OpenRecognizer = func(string) (Recognizer, error) { return fakeRecognizer{&calls}, nil }
	one := NewProject(e, source, base)
	if err := one.Transcribe(context.Background()); err != nil {
		t.Fatalf("%v %s", err, one.LastError())
	}
	alone, err := one.Transcript()
	if err != nil {
		t.Fatal(err)
	}
	// The same episode again, made the same way, so nothing heard before
	// is at hand.
	source = testEpisode(t, "70")
	before := calls

	path := filepath.Join(WorkDir(source), "logs", "words.json")
	stamp, _ := stampOf(source)
	var mu sync.Mutex
	var problems []string
	last := 0.0
	copies := &sideBySide{inner: fakeRecognizer{&calls}, copies: 4}
	copies.seen = func() {
		file, words, _, ok := readTranscriptFile(path, stamp, filepath.Base(base.ASRModel))
		if !ok {
			return
		}
		mu.Lock()
		defer mu.Unlock()
		if float64(file.To) < last-0.001 {
			problems = append(problems, "a save went back")
		}
		last = float64(file.To)
		for _, w := range words {
			if w.End > float64(file.To)+0.001 {
				problems = append(problems, "a save holds a word past how far it says it reaches")
				return
			}
		}
	}
	e.OpenRecognizer = func(string) (Recognizer, error) { return copies, nil }
	many := NewProject(e, source, base)
	if err := many.Transcribe(context.Background()); err != nil {
		t.Fatalf("%v %s", err, many.LastError())
	}
	together, err := many.Transcript()
	if err != nil {
		t.Fatal(err)
	}

	if calls == before {
		t.Fatal("the second episode was never heard")
	}
	if copies.most.Load() < 2 {
		t.Fatal("the copies never heard two pieces at once")
	}
	if len(together.RawWords) != len(alone.RawWords) || len(together.Frames) != len(alone.Frames) {
		t.Fatalf("%d words and %d frames side by side, %d and %d with one copy",
			len(together.RawWords), len(together.Frames), len(alone.RawWords), len(alone.Frames))
	}
	for i := range alone.RawWords {
		if together.RawWords[i] != alone.RawWords[i] {
			t.Fatalf("word %d is %v side by side, %v with one copy", i, together.RawWords[i], alone.RawWords[i])
		}
	}
	mu.Lock()
	defer mu.Unlock()
	if len(problems) > 0 {
		t.Errorf("%d bad saves, the first: %s", len(problems), problems[0])
	}
}

// Side by side, a part is heard exactly, with nothing missed on the way
// and no audio heard past what it reads around the part.
func TestHearingSideBySideHearsExactlyItsPart(t *testing.T) {
	t.Parallel()
	source := testEpisode(t, "70")
	heard := 0.0
	copies := &sideBySide{inner: straddlingRecognizer{&heard}, copies: 3}
	e := NewEngine(NewLog(&bytes.Buffer{}, false, false))
	e.OpenRecognizer = func(string) (Recognizer, error) { return copies, nil }
	base := DefaultOptions()
	base.ASRModel = t.TempDir()
	p := NewProject(e, source, base)
	part := Window{10, 51.37}
	if err := p.Hear(context.Background(), part); err != nil {
		t.Fatalf("%v %s", err, p.LastError())
	}
	if parts := HeardParts(source, base.ASRModel); len(parts) != 1 || parts[0] != [2]float64{part.Start, part.End} {
		t.Fatalf("heard %v, asked for %v", parts, part)
	}
	if over := heard - (part.End - part.Start + 2*hearingPad); over > 0.02 {
		t.Errorf("heard %.2f s more than the part and the audio around it", over)
	}
	stamp, _ := stampOf(source)
	_, words, _, ok := readTranscriptFile(filepath.Join(p.LogsDir(), "words.json"), stamp,
		filepath.Base(base.ASRModel))
	if !ok {
		t.Fatal("cannot read the transcript")
	}
	// Every piece of the part is in, none left behind by a copy that was
	// still hearing it: a word every 0.6 s.
	if want := int(math.Floor((part.End-part.Start)/0.6)) - 2; len(words) < want {
		t.Errorf("%d words in %v, want about %d", len(words), part, want)
	}
	for _, w := range words {
		if w.Start < part.Start || w.Start >= part.End {
			t.Errorf("a word outside the part: %v", w)
		}
	}
}

// Stopped part way, side by side, what every copy heard before the stop is
// written down, and nothing is left hearing after it has returned.
func TestHearingSideBySideStoppedKeepsWhatItHeard(t *testing.T) {
	t.Parallel()
	source := testEpisode(t, "70")

	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()
	var mu sync.Mutex
	heard := 0.0
	log := NewLog(&bytes.Buffer{}, false, false)
	log.SetSink(func(ev Event) {
		if ev.Kind != EventProgress || ev.Covered <= 0 {
			return
		}
		mu.Lock()
		defer mu.Unlock()
		heard = ev.Covered
		if heard > 20 {
			cancel()
		}
	})
	var calls int32
	copies := &sideBySide{inner: fakeRecognizer{&calls}, copies: 4}
	e := NewEngine(log)
	e.checkpointEvery = time.Hour
	e.OpenRecognizer = func(string) (Recognizer, error) { return copies, nil }
	base := DefaultOptions()
	base.ASRModel = t.TempDir()
	p := NewProject(e, source, base)
	if err := p.Transcribe(ctx); err == nil {
		t.Fatal("a stopped transcription finished")
	}
	if busy := copies.busy.Load(); busy != 0 {
		t.Errorf("%d copies still hearing after the transcription returned", busy)
	}
	mu.Lock()
	want := heard
	mu.Unlock()
	covered, done := Coverage(source, base.ASRModel)
	if done || want <= 20 || covered < want-0.001 {
		t.Errorf("stopped after hearing %.2f s, %.2f s written down (done %v)", want, covered, done)
	}
}

// A copy that panics does so where the transcription was called, the way
// the one copy did, so the job around it fails and the app does not.
func TestACopyThatPanicsFailsTheTranscription(t *testing.T) {
	t.Parallel()
	source := testEpisode(t, "70")
	var calls int32
	copies := &sideBySide{inner: fakeRecognizer{&calls}, copies: 3, fail: 2}
	e := NewEngine(NewLog(&bytes.Buffer{}, false, false))
	e.OpenRecognizer = func(string) (Recognizer, error) { return copies, nil }
	base := DefaultOptions()
	base.ASRModel = t.TempDir()
	p := NewProject(e, source, base)
	defer func() {
		if recover() == nil {
			t.Error("the panic of a copy was lost")
		}
		if busy := copies.busy.Load(); busy != 0 {
			t.Errorf("%d copies still hearing after the panic", busy)
		}
	}()
	_ = p.Transcribe(context.Background())
}
