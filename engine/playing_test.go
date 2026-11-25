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
	err := p.Sound(context.Background(), ones(started), 1000, 2, func(out int64, _ int, chunk []byte) error {
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
	if err := run.give(t, 99); err != nil {
		t.Fatalf("frame 99 of the look: %v", err)
	}
	// The run ends at the look's frame while the window draws it.
	if err := run.give(t, 100); !errors.Is(err, errLookDone) {
		t.Fatalf("frame 100 of the look: %v", err)
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

// A paused play held the decoder, and nothing was read ahead while it
// stood: held, it gives the decoder to the hand, and with Go it goes on
// from the frame after the last it handed over.
func TestAHeldPlayGivesTheDecoderBackAndGoesOnFromWhereItWas(t *testing.T) {
	t.Parallel()
	d := newHeldDecoder()
	s := newSchedule(d)
	p, frames, _ := playOf(s, []PlayPiece{{1, 2}}, 1)
	st := d.next(t)
	for _, f := range []int{25, 26} {
		if err := st.give(t, f); err != nil {
			t.Fatalf("frame %d: %v", f, err)
		}
		if got := <-frames; got.Frame != f {
			t.Fatalf("the play handed over %d, not %d", got.Frame, f)
		}
	}
	p.Hold()
	st.stopped(t)
	// The hand has the decoder while the play is held.
	looking := lookFor(s, rest(100))
	waits(s, rest(100))
	run := d.next(t)
	if err := run.give(t, 99); err != nil {
		t.Fatalf("frame 99 of the look: %v", err)
	}
	// The run ends at the look's frame while the window draws it.
	if err := run.give(t, 100); !errors.Is(err, errLookDone) {
		t.Fatalf("frame 100 of the look: %v", err)
	}
	if got := <-looking; got.err != nil || got.seen.Frame != 100 {
		t.Fatalf("the look was answered with %+v", got)
	}
	p.Go()
	run.stopped(t)
	on := d.next(t)
	if on.from != 27 {
		t.Fatalf("the play goes on from %d, not 27", on.from)
	}
	if err := on.give(t, 27); err != nil {
		t.Fatal(err)
	}
	if got := <-frames; got.Frame != 27 {
		t.Fatalf("the play handed over %d after it went on, not 27", got.Frame)
	}
	p.Stop()
}

// A play's frames go back to the decoder once the window has written
// them, and not before, however soon the schedule lets go of them: read
// into while it is written, a frame of a play would show half of a later
// one. Issue #969.
func TestAPlaysFramesGoBackToTheDecoderOnceWritten(t *testing.T) {
	t.Parallel()
	d := newHeldDecoder()
	s := newSchedule(d)
	keepOnly(s, 1)
	_, frames, ended := playOf(s, []PlayPiece{{1, 1.2}}, 1)
	st := d.next(t)
	for f := 25; f < 30; f++ {
		_ = st.give(t, f)
	}
	if err := <-ended; err != nil {
		t.Fatalf("the play ended with %v", err)
	}
	close(frames)
	var played []PlayFrame
	for f := range frames {
		played = append(played, f)
	}
	if len(played) != 5 {
		t.Fatalf("the play brought %d frames", len(played))
	}
	for _, f := range played {
		if d.given(f.Picture) || f.Picture[0] != byte(f.Frame) {
			t.Fatalf("frame %d went back to the decoder before it was written, and reads %d", f.Frame, f.Picture[0])
		}
		s.Give(f.Picture)
	}
	// All but the last, which the schedule keeps for a play started there
	// again.
	for _, f := range played[:4] {
		if !d.given(f.Picture) {
			t.Errorf("frame %d did not go back to the decoder once written", f.Frame)
		}
	}
	if d.given(played[4].Picture) {
		t.Error("frame 29 went back to the decoder while the schedule keeps it")
	}
}

// A clip changed while it plays is taken into the play at a place ahead
// of what is heard: the frames up to it are the old pieces', from it on
// the new pieces' from the moment the play stands at there, on the same
// clock, stamped with the change, and the frame the old play decoded at
// the place is the new play's first.
func TestAChangedClipIsTakenIntoThePlayAtItsPlace(t *testing.T) {
	t.Parallel()
	d := newHeldDecoder()
	s := newSchedule(d)
	p, frames, ended := playOf(s, []PlayPiece{{1, 2}}, 1)
	old := d.next(t)
	for f := 25; f < 28; f++ {
		if err := old.give(t, f); err != nil {
			t.Fatalf("frame %d: %v", f, err)
		}
	}
	got, ok := p.Retake([]PlayPiece{{1, 1.4}, {1.8, 2}}, false, 0.2)
	if !ok || got.Gen != 1 || math.Abs(got.At-0.2) > 1e-9 || math.Abs(got.End-0.6) > 1e-9 || math.Abs(got.Lap-0.6) > 1e-9 {
		t.Fatalf("the change was taken in %+v, %v", got, ok)
	}
	for f := 28; f < 31; f++ {
		_ = old.give(t, f)
	}
	streams := streamsFrom(t, d)
	first, second := streams[31], streams[45]
	if first == nil || second == nil {
		t.Fatalf("the play of the changed clip reads %v", streams)
	}
	for f := 31; f < 35; f++ {
		if err := first.give(t, f); f == 34 != errors.Is(err, errRunDone) {
			t.Fatalf("frame %d of the changed clip: %v", f, err)
		}
	}
	for f := 45; f < 50; f++ {
		if err := second.give(t, f); f == 49 != errors.Is(err, errRunDone) {
			t.Fatalf("frame %d of the changed clip: %v", f, err)
		}
	}
	if err := <-ended; err != nil {
		t.Fatalf("the play ended with %v", err)
	}
	close(frames)
	type seen struct {
		frame, gen int
		at         float64
	}
	var want []seen
	for i, f := range []int{25, 26, 27, 28, 29, 30, 31, 32, 33, 34, 45, 46, 47, 48, 49} {
		gen := 0
		if f >= 30 {
			gen = 1
		}
		want = append(want, seen{f, gen, float64(i) * 0.04})
	}
	i := 0
	for f := range frames {
		if i >= len(want) || f.Frame != want[i].frame || f.Gen != want[i].gen || math.Abs(f.At-want[i].at) > 1e-9 {
			t.Fatalf("frame %d is %d of change %d at %.2f s, want %+v", i, f.Frame, f.Gen, f.At, want)
		}
		i++
	}
	if i != len(want) {
		t.Fatalf("the play brought %d frames, not %d", i, len(want))
	}
	if cur, shift := p.Current(); cur == p || math.Abs(shift) > 1e-9 {
		t.Errorf("the play that plays now is %p, moved by %v", cur, shift)
	}
}

// The sound is cut at the place a change is taken in, at the sample, and
// goes on with the new pieces' sound on the same clock.
func TestTheSoundOfAChangedClipGoesOnFromItsPlace(t *testing.T) {
	t.Parallel()
	s := newSchedule(newHeldDecoder())
	p := s.Play(PlayAsk{Grid: lookGrid, W: 4, H: 2, Pieces: []PlayPiece{{1, 2}}, Moment: 1})
	defer p.Stop()
	if _, ok := p.Retake([]PlayPiece{{1, 1.1}, {1.5, 1.6}}, false, 0.05); !ok {
		t.Fatal("the change was not taken in")
	}
	started := make(chan float64, 4)
	gens := map[int64]int{}
	err := p.Sound(context.Background(), ones(started), 1000, 1, func(out int64, gen int, chunk []byte) error {
		for i := range int64(len(chunk) / 4) {
			if _, twice := gens[out+i]; twice {
				t.Errorf("sample %d came twice", out+i)
			}
			gens[out+i] = gen
		}
		return nil
	})
	if err != nil {
		t.Fatal(err)
	}
	if a, b, c := <-started, <-started, <-started; a != 1 || b != 1.05 || c != 1.5 {
		t.Errorf("the sound was read from %v, %v and %v", a, b, c)
	}
	if len(gens) != 200 {
		t.Fatalf("the play's sound is %d samples", len(gens))
	}
	for m := range int64(200) {
		want := 0
		if m >= 50 {
			want = 1
		}
		if gen, ok := gens[m]; !ok || gen != want {
			t.Fatalf("sample %d is of change %d, there %v", m, gen, ok)
		}
	}
}

// A second change before the first took over takes its place, at its
// place, and a change the new pieces do not hold, or one given to a play
// paused, is not taken in.
func TestAChangeBeforeTheLastTookOverTakesItsPlace(t *testing.T) {
	t.Parallel()
	d := newHeldDecoder()
	s := newSchedule(d)
	p, frames, _ := playOf(s, []PlayPiece{{1, 2}}, 1)
	defer p.Stop()
	old := d.next(t)
	_ = old.give(t, 25)
	<-frames
	if _, ok := p.Retake([]PlayPiece{{1.5, 2}}, false, 0.2); ok {
		t.Error("a change that does not hold the moment was taken in")
	}
	if _, ok := p.Retake([]PlayPiece{{1, 1.6}}, false, 0.2); !ok {
		t.Fatal("the first change was not taken in")
	}
	got, ok := p.Retake([]PlayPiece{{1, 1.3}, {1.7, 2}}, false, 0.3)
	if !ok || got.Gen != 2 || math.Abs(got.At-0.2) > 1e-9 || math.Abs(got.End-0.6) > 1e-9 {
		t.Fatalf("the second change was taken in %+v, %v", got, ok)
	}
	for f := 26; f < 31; f++ {
		_ = old.give(t, f)
	}
	streams := streamsFrom(t, d)
	if streams[31] == nil || streams[42] == nil {
		t.Fatalf("the play of the second change reads %v", streams)
	}
	for range 4 {
		<-frames
	}
	if f := <-frames; f.Frame != 30 || f.Gen != 2 {
		t.Fatalf("the frame at the place is %d of change %d", f.Frame, f.Gen)
	}
	cur, _ := p.Current()
	cur.Hold()
	if _, ok := p.Retake([]PlayPiece{{1, 2}}, false, 0.4); ok {
		t.Error("a change was taken into a play paused")
	}
}
