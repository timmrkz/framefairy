package engine

import (
	"bytes"
	"context"
	"math"
	"path/filepath"
	"sync"
	"sync/atomic"
	"testing"
	"time"

	"framefairy/engine/timed"
)

// sideBySide is a recogniser with several copies, the way asr.Pool is,
// whose pieces finish out of order, the way copies on a busy machine do.
type sideBySide struct {
	inner  Recognizer
	copies int
	// pair holds the first piece until a second one is being heard, for
	// the test that checks two are. Only that test sets it, because a part
	// heard in one piece would wait for a second that never comes.
	pair bool
	// The inner recogniser is heard one piece at a time. The waiting is
	// outside it, so the pieces still overtake each other.
	mu   sync.Mutex
	busy atomic.Int32
	most atomic.Int32
	seen func()
	fail int32
	n    atomic.Int32
	// paired closes once two pieces are being heard at once.
	pairOnce sync.Once
	makeOnce sync.Once
	paired   chan struct{}
	// The pieces being heard, by the order they came in. A piece is done
	// only once every piece that came after it is, see Recognize.
	order  sync.Mutex
	turn   *sync.Cond
	came   int
	flying map[int]bool
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
	// machine decodes the next piece. The wait has no limit: a limit on the
	// clock ran out on a busy machine before ffmpeg had read the next
	// thirty seconds, and the test failed with nothing wrong, #486. An
	// engine that hears one piece at a time never sends the second while
	// the first is held, so it hangs here, and go test's -timeout ends it
	// with this wait in the stack of every goroutine.
	if now >= 2 || !s.pair {
		s.pairOnce.Do(func() { close(s.pairedChan()) })
	} else {
		<-s.pairedChan()
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
	s.order.Lock()
	if s.turn == nil {
		s.turn, s.flying = sync.NewCond(&s.order), map[int]bool{}
	}
	s.came++
	me := s.came
	s.flying[me] = true
	s.order.Unlock()
	s.mu.Lock()
	out := s.inner.Recognize(samples, rate)
	s.mu.Unlock()
	// A piece that came later and is still being heard finishes first, so
	// whenever two are heard at once, the later overtakes the earlier, on
	// any machine. The piece that came last never waits, so one always
	// goes on.
	s.order.Lock()
	for s.later(me) {
		s.turn.Wait()
	}
	delete(s.flying, me)
	s.turn.Broadcast()
	s.order.Unlock()
	if s.seen != nil {
		s.seen()
	}
	return out
}

// later says whether a piece that came after me is still being heard.
// Under s.order.
func (s *sideBySide) later(me int) bool {
	for n := range s.flying {
		if n > me {
			return true
		}
	}
	return false
}

func (s *sideBySide) Close() {}

// Heard by four copies at once, an episode comes out word for word as one
// copy hears it, and every save on the way reaches only as far as all of
// it has been heard, never past a piece still being heard.
//
// On the real disk: ffmpeg reads the episode the speech model hears.
func TestHearingSideBySideWritesTheSameTranscript(t *testing.T) {
	t.Parallel()
	source := testEpisode(t, "70")
	base := DefaultOptions()
	base.ASRModel = t.TempDir()

	var calls int32
	e := NewEngine(NewLog(&bytes.Buffer{}, false, false), disk)
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
	stamp, _ := stampOf(disk, source)
	var mu sync.Mutex
	var problems []string
	last := 0.0
	// The first piece is held until a second one is heard, and the second
	// goes out only once a whole chunkMax has been read after the first
	// cut, which is at most chunkMax in. So the episode holds two of them,
	// or the hold waits for ever with nothing wrong in the engine.
	if 70 <= 2*chunkMax {
		t.Fatalf("an episode of 70 s has no room for two pieces of %v s", chunkMax)
	}
	copies := &sideBySide{inner: fakeRecognizer{&calls}, copies: 4, pair: true}
	copies.seen = func() {
		file, words, _, ok := readTranscriptFile(disk, path, stamp, filepath.Base(base.ASRModel))
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
//
// On the real disk: ffmpeg reads the episode the speech model hears.
func TestHearingSideBySideHearsExactlyItsPart(t *testing.T) {
	t.Parallel()
	source := testEpisode(t, "70")
	heard := 0.0
	copies := &sideBySide{inner: straddlingRecognizer{&heard}, copies: 3}
	e := NewEngine(NewLog(&bytes.Buffer{}, false, false), disk)
	e.OpenRecognizer = func(string) (Recognizer, error) { return copies, nil }
	base := DefaultOptions()
	base.ASRModel = t.TempDir()
	p := NewProject(e, source, base)
	part := timed.Window{Start: 10, End: 51.37}
	if err := p.Hear(context.Background(), part); err != nil {
		t.Fatalf("%v %s", err, p.LastError())
	}
	if parts := HeardParts(disk, source, base.ASRModel); len(parts) != 1 || parts[0] != [2]float64{part.Start, part.End} {
		t.Fatalf("heard %v, asked for %v", parts, part)
	}
	if over := heard - (part.End - part.Start + 2*hearingPad); over > 0.02 {
		t.Errorf("heard %.2f s more than the part and the audio around it", over)
	}
	stamp, _ := stampOf(disk, source)
	_, words, _, ok := readTranscriptFile(disk, filepath.Join(p.LogsDir(), "words.json"), stamp,
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
//
// On the real disk: ffmpeg reads the episode the speech model hears.
func TestHearingSideBySideStoppedKeepsWhatItHeard(t *testing.T) {
	t.Parallel()
	var calls int32
	copies := &sideBySide{inner: fakeRecognizer{&calls}, copies: 4}
	stoppedKeepsWhatItHeard(t, copies)
	if busy := copies.busy.Load(); busy != 0 {
		t.Errorf("%d copies still hearing after the transcription returned", busy)
	}
}

// A copy that panics does so where the transcription was called, the way
// the one copy did, so the job around it fails and the app does not.
//
// On the real disk: ffmpeg reads the episode the speech model hears.
func TestACopyThatPanicsFailsTheTranscription(t *testing.T) {
	t.Parallel()
	source := testEpisode(t, "70")
	var calls int32
	copies := &sideBySide{inner: fakeRecognizer{&calls}, copies: 3, fail: 2}
	e := NewEngine(NewLog(&bytes.Buffer{}, false, false), disk)
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
