package engine

import (
	"context"
	"encoding/binary"
	"errors"
	"math"
	"testing"
)

// A play is a function of the pieces and of what the decoder hands over,
// so these tests hand over the frames themselves, with the held decoder of
// schedule_test.go, and never wait on the clock.

// playOf starts a play of the pieces from a moment and collects its frames
// as it hands them over, with how it ended.
func playOf(s *Schedule, pieces []PlayPiece, moment float64) (*Play, chan PlayFrame, chan error) {
	p := s.Play(PlayAsk{Grid: lookGrid, W: 4, H: 2, Pieces: pieces, Moment: moment})
	frames, ended := make(chan PlayFrame, 64), make(chan error, 1)
	go func() {
		ended <- p.Pictures(context.Background(), func(f PlayFrame) error {
			frames <- f
			return nil
		})
	}()
	return p, frames, ended
}

// streamsFrom are the next two streams the schedule opens, by where they
// start.
func streamsFrom(t *testing.T, d *heldDecoder) map[int]*heldStream {
	t.Helper()
	a, b := d.next(t), d.next(t)
	return map[int]*heldStream{a.from: a, b.from: b}
}

func TestAPlayBringsItsFramesInOrderAndTheNextPiecesBeforeTheCut(t *testing.T) {
	t.Parallel()
	d := newHeldDecoder()
	s := newSchedule(d)
	_, frames, ended := playOf(s, []PlayPiece{{1, 1.2}, {2, 2.2}}, 1)
	// Both pieces' streams start at once: the second waits at its first
	// frame while the first plays, so the cut waits for no seek.
	streams := streamsFrom(t, d)
	first, second := streams[25], streams[50]
	if first == nil || second == nil {
		t.Fatalf("the streams start at %v", streams)
	}
	for f := 50; f < 53; f++ {
		if err := second.give(t, f); err != nil {
			t.Fatalf("frame %d of the second piece: %v", f, err)
		}
	}
	for f := 25; f < 30; f++ {
		err := first.give(t, f)
		if f < 29 && err != nil || f == 29 && !errors.Is(err, errRunDone) {
			t.Fatalf("frame %d of the first piece: %v", f, err)
		}
	}
	for f := 53; f < 55; f++ {
		if err := second.give(t, f); f == 54 != errors.Is(err, errRunDone) {
			t.Fatalf("frame %d of the second piece: %v", f, err)
		}
	}
	if err := <-ended; err != nil {
		t.Fatalf("the play ended with %v", err)
	}
	close(frames)
	var got []int
	var at []float64
	for f := range frames {
		got = append(got, f.Frame)
		at = append(at, f.At)
	}
	want := []int{25, 26, 27, 28, 29, 50, 51, 52, 53, 54}
	if len(got) != len(want) {
		t.Fatalf("the play brought %v", got)
	}
	for i := range want {
		if got[i] != want[i] || math.Abs(at[i]-float64(i)*0.04) > 1e-9 {
			t.Errorf("frame %d is %d at %v, not %d at %v", i, got[i], at[i], want[i], float64(i)*0.04)
		}
	}
	d.none(t)
}

func TestAPlayStartsWithTheFramesKept(t *testing.T) {
	t.Parallel()
	d := newHeldDecoder()
	s := newSchedule(d)
	for _, f := range []int{25, 26} {
		s.kept.put(keptKey{grid: lookGrid, w: 4, h: 2, frame: f}, lookGrid.At(f), []byte{byte(f)})
	}
	_, frames, ended := playOf(s, []PlayPiece{{1, 1.2}}, 1)
	for _, f := range []int{25, 26} {
		if got := <-frames; got.Frame != f || got.Picture[0] != byte(f) {
			t.Fatalf("the play starts with %d, not the frame kept for %d", got.Frame, f)
		}
	}
	// The decoder starts after them.
	st := d.next(t)
	if st.from != 27 {
		t.Fatalf("the stream starts at %d", st.from)
	}
	_ = st.give(t, 27)
	_ = st.give(t, 28)
	if err := st.give(t, 29); !errors.Is(err, errRunDone) {
		t.Fatalf("the last frame said %v", err)
	}
	if err := <-ended; err != nil {
		t.Fatal(err)
	}
	// What it decoded is kept, so a play started there again starts at
	// once.
	if !s.kept.has(keptKey{grid: lookGrid, w: 4, h: 2, frame: 29}) {
		t.Error("the frames a play decoded are not kept")
	}
}

func TestTheHandTakesTheDecoderFromAPlayAndAPlayFromTheHand(t *testing.T) {
	t.Parallel()
	d := newHeldDecoder()
	s := newSchedule(d)
	// A look waiting for its frame is answered once a play starts.
	looking := lookFor(s, rest(100))
	waits(s, rest(100))
	run := d.next(t)
	_, _, ended := playOf(s, []PlayPiece{{1, 1.2}}, 1)
	if got := <-looking; !errors.Is(got.err, ErrOvertaken) {
		t.Fatalf("the look was answered with %v", got.err)
	}
	run.stopped(t)
	play := d.next(t)
	if play.from != 25 {
		t.Fatalf("the play's stream starts at %d", play.from)
	}
	// The hand takes the playhead: the play stops, and the look gets the
	// decoder.
	looking = lookFor(s, rest(200))
	play.stopped(t)
	if err := <-ended; !errors.Is(err, context.Canceled) {
		t.Fatalf("the play ended with %v", err)
	}
	if st := d.next(t); st.from != 199 {
		t.Fatalf("the look's stream starts at %d", st.from)
	}
	_ = looking
	// A play that ends by itself lets the reading ahead go on.
	s.mu.Lock()
	defer s.mu.Unlock()
	if s.play != nil {
		t.Error("the play still has the decoder")
	}
}

func TestANewerPlayStopsTheOne(t *testing.T) {
	t.Parallel()
	d := newHeldDecoder()
	s := newSchedule(d)
	_, _, ended := playOf(s, []PlayPiece{{1, 1.2}}, 1)
	first := d.next(t)
	p, _, _ := playOf(s, []PlayPiece{{1, 1.2}}, 1.1)
	first.stopped(t)
	if err := <-ended; !errors.Is(err, context.Canceled) {
		t.Fatalf("the older play ended with %v", err)
	}
	if st := d.next(t); st.from != 27 {
		t.Fatalf("the newer play starts at %d", st.from)
	}
	if !math.IsInf(p.Loop(true, 0), 1) || !near(p.Loop(false, 0.1), 0.2) {
		t.Error("the loop switched while it plays says the wrong end")
	}
	p.Stop()
}

// ones is a sound of ones from any moment on, in chunks of 64 moments,
// which says where each stream started.
func ones(started chan float64) SoundSource {
	return func(ctx context.Context, from float64, rate, channels int, got func(float64, []byte) error) error {
		started <- from
		for n := 0; ; n += 64 {
			chunk := make([]byte, 64*channels*4)
			for i := 0; i < len(chunk); i += 4 {
				binary.LittleEndian.PutUint32(chunk[i:], math.Float32bits(1))
			}
			if err := got(from+float64(n)/float64(rate), chunk); err != nil {
				return err
			}
			if ctx.Err() != nil {
				return ctx.Err()
			}
		}
	}
}

func TestAPlaysSoundIsEachPieceCutAtTheSampleAndFadedAtItsEdges(t *testing.T) {
	t.Parallel()
	s := newSchedule(newHeldDecoder())
	p := s.Play(PlayAsk{Grid: lookGrid, W: 4, H: 2, Pieces: []PlayPiece{{1, 1.1}, {2, 2.1}}, Moment: 1})
	started := make(chan float64, 4)
	heard := map[int64]float32{}
	err := p.Sound(context.Background(), ones(started), 1000, 2, func(out int64, chunk []byte) error {
		for i := 0; i < len(chunk)/8; i++ {
			left := math.Float32frombits(binary.LittleEndian.Uint32(chunk[i*8:]))
			right := math.Float32frombits(binary.LittleEndian.Uint32(chunk[i*8+4:]))
			if left != right {
				t.Errorf("sample %d is %v and %v", out+int64(i), left, right)
			}
			heard[out+int64(i)] = left
		}
		return nil
	})
	if err != nil {
		t.Fatal(err)
	}
	if a, b := <-started, <-started; a != 1 || b != 2 {
		t.Errorf("the sound was read from %v and %v", a, b)
	}
	if len(heard) != 200 {
		t.Fatalf("the play's sound is %d samples", len(heard))
	}
	f := float32(Fade * 1000)
	for m, want := range map[int64]float32{0: 0.5 / f, 15: 1, 50: 1, 99: 0.5 / f, 100: 0.5 / f, 150: 1, 199: 0.5 / f} {
		if math.Abs(float64(heard[m]-want)) > 1e-6 {
			t.Errorf("sample %d is %v, not %v", m, heard[m], want)
		}
	}
}

// A cued play hands over the frames kept from its first on and leaves the
// decoder to the hand until the space bar says go: then it reads on from
// the first frame not kept.
func TestACuedPlayWaitsForGoBeforeItTakesTheDecoder(t *testing.T) {
	t.Parallel()
	d := newHeldDecoder()
	s := newSchedule(d)
	s.kept.put(keptKey{grid: lookGrid, w: 4, h: 2, frame: 25}, lookGrid.At(25), []byte{25})
	looking := lookFor(s, rest(100))
	waits(s, rest(100))
	run := d.next(t)
	p := s.Play(PlayAsk{Grid: lookGrid, W: 4, H: 2, Pieces: []PlayPiece{{1, 1.2}}, Moment: 1, Cue: true})
	frames := make(chan PlayFrame, 8)
	go func() {
		_ = p.Pictures(context.Background(), func(f PlayFrame) error {
			frames <- f
			return nil
		})
	}()
	if f := <-frames; f.Frame != 25 || !f.Edge {
		t.Fatalf("the cue starts with %+v", f)
	}
	// The hand's look still has the decoder.
	for f := 99; f <= 100; f++ {
		if err := run.give(t, f); err != nil {
			t.Fatalf("frame %d of the look: %v", f, err)
		}
	}
	if got := <-looking; got.err != nil || got.seen.Frame != 100 {
		t.Fatalf("the look was answered with %+v", got)
	}
	d.none(t)
	p.Go()
	run.stopped(t)
	if st := d.next(t); st.from != 26 {
		t.Fatalf("the play reads on from %d", st.from)
	}
	p.Stop()
}
