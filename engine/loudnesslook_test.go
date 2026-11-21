package engine

import (
	"bytes"
	"context"
	"encoding/binary"
	"errors"
	"testing"

	"framefairy/engine/timed"
)

// lookOn starts a look at a schedule and hands back where its answer
// comes, once the look waits or has its answer.
func lookOn(s *LoudnessSchedule, v View) <-chan lookAnswer[[]byte] {
	got := make(chan lookAnswer[[]byte], 1)
	s.mu.Lock()
	waiting := s.waits.WaitLocked()
	s.mu.Unlock()
	go func() {
		p, err := s.Look(context.Background(), v)
		got <- lookAnswer[[]byte]{got: p, err: err}
	}()
	select {
	case <-waiting:
	case a := <-got:
		got <- a
	}
	return got
}

// pieceOf reads a piece the way the window does: the version, the
// readings it covers, whether the view is measured whole, and each level's
// bytes of them.
func pieceOf(t *testing.T, p []byte) (version uint32, first, last int, whole bool, levels [][]byte) {
	t.Helper()
	if len(p) < loudnessPieceHead {
		t.Fatalf("a piece of %d bytes", len(p))
	}
	count := int(binary.LittleEndian.Uint32(p))
	version = binary.LittleEndian.Uint32(p[8:])
	first, last = int(binary.LittleEndian.Uint32(p[12:])), int(binary.LittleEndian.Uint32(p[16:]))
	whole = p[20] == 1
	at := loudnessPieceHead
	for k, n := 0, count; n > 0; k, n = k+1, (n+1)/2 {
		if last > first {
			size := (last-1)>>k - first>>k + 1
			levels = append(levels, p[at:at+size])
			at += size
		}
		if n == 1 {
			break
		}
	}
	if at != len(p) {
		t.Fatalf("a piece of %d bytes holds %d", len(p), at)
	}
	return version, first, last, whole, levels
}

// The window's look at its view is answered with the readings of the
// view and the levels above them, at once where they are measured, and
// as soon as the measuring brings them where not. Each answer is what
// Loudness makes of the same readings, so the window lays it over what
// it holds and draws the same waveform as from the whole. A look whose
// window holds all of its view is answered with nothing new, #968.
func TestTheWindowIsAnsweredWithTheLoudnessOfItsView(t *testing.T) {
	at := func(s float64) int { return int(s / timed.FrameSeconds) }
	count := at(600)
	s := NewLoudnessSchedule()
	// A look before the measuring starts waits for it.
	before := lookOn(s, View{From: 0, To: 10})
	s.Start(nil, count)
	a := <-before
	if a.err != nil {
		t.Fatal(a.err)
	}
	if _, first, last, _, _ := pieceOf(t, a.got); first != 0 || last != count {
		t.Fatalf("a window holding nothing was sent %d to %d, not all %d readings", first, last, count)
	}
	version, _, _, _, _ := pieceOf(t, a.got)

	// The view at 300 s, not measured: the look waits, and readings
	// elsewhere do not answer it.
	view := View{From: 300, To: 360, Has: version, Count: count}
	waiting := lookOn(s, view)
	frames := make([]float32, at(2))
	for i := range frames {
		frames[i] = -float32(i % 60)
	}
	s.Put(at(100), frames)
	select {
	case a := <-waiting:
		t.Fatalf("readings at 100 s answered the view at 300 s: %v", a.err)
	default:
	}
	// The first read in the view answers it with the view's readings.
	s.Put(at(300), frames)
	a = <-waiting
	if a.err != nil {
		t.Fatal(a.err)
	}
	version, first, last, whole, levels := pieceOf(t, a.got)
	if first != at(300) || last != at(360) || whole {
		t.Fatalf("the view was answered with %d to %d, whole %v", first, last, whole)
	}
	if !bytes.Equal(levels[0][:len(frames)], wantBytes(frames)) {
		t.Error("the readings sent are not the ones measured")
	}

	// The rest of the view measured, the window holding that version has
	// all of it: nothing new.
	rest := make([]float32, at(58))
	s.Put(at(302), rest)
	view.Has = version
	a = <-lookOn(s, view)
	if a.err != nil {
		t.Fatal(a.err)
	}
	version, _, _, whole, _ = pieceOf(t, a.got)
	if !whole {
		t.Error("a view measured whole says it is not")
	}
	view.Has = version
	if a := <-lookOn(s, view); !errors.Is(a.err, ErrNothingNew) {
		t.Errorf("a window holding all of its view was answered with %v", a.err)
	}

	// Each level the schedule keeps is what Loudness makes of the same
	// readings, so the levels above a reading are kept right as it comes.
	all := make([]float32, count)
	for i := range all {
		all[i] = -90
	}
	copy(all[at(100):], frames)
	copy(all[at(300):], frames)
	copy(all[at(302):], rest)
	want := Loudness(&Transcript{Frames: all})
	s.mu.Lock()
	held := append([]byte(nil), s.held...)
	s.mu.Unlock()
	if !bytes.Equal(held, want) {
		t.Error("the loudness held differs from what Loudness makes of the same readings")
	}
}

// A newer look takes the place of one that waits, which is answered as
// overtaken, as a look of the video preview is.
func TestANewerViewOvertakesOneThatWaits(t *testing.T) {
	s := NewLoudnessSchedule()
	s.Start(nil, 60000)
	old := lookOn(s, View{From: 100, To: 110, Count: 60000, Has: s.version})
	newer := lookOn(s, View{From: 200, To: 210, Count: 60000, Has: s.version})
	if a := <-old; !errors.Is(a.err, ErrOvertaken) {
		t.Errorf("the older look was answered with %v", a.err)
	}
	s.Ended()
	if a := <-newer; a.err != nil {
		t.Errorf("the newer look, once the measuring ended: %v", a.err)
	}
}

func wantBytes(frames []float32) []byte {
	out := make([]byte, len(frames))
	for i, f := range frames {
		out[i] = loudnessByte(f)
	}
	return out
}
