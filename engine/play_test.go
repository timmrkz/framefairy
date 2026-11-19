package engine

import (
	"fmt"
	"math"
	"slices"
	"testing"

	"framefairy/internal/framewire"
)

// The clip of the proof in the harness: three pieces, two cuts, no edge on
// a frame's start. A plan keeps every edge on a frame's start, see
// PieceOnFrames, and edges off them show each rule on its own: a piece
// starts with the frame that holds its start and ends with the frame that
// holds the moment a frame before its end.
var playClip = []PlayPiece{{57, 59.53}, {60.31, 62.17}, {63.05, 65.0}}

// Ten minutes at 25 frames a second.
var playGrid = framewire.Grid{Num: 25, Den: 1, Count: 15000}

// framesFrom are the starts of the frames from one to another, a frame
// each 40 ms, both included, written to the hundredth.
func framesFrom(from, to float64) []string {
	var out []string
	for t := from; t <= to+1e-9; t += 0.04 {
		out = append(out, fmt.Sprintf("%.2f", t))
	}
	return out
}

// allShots are the shots of a play of the program from p0, up to most.
func allShots(g framewire.Grid, p *Program, p0 float64, most int) []Shot {
	var out []Shot
	s := NewShots(g, p, p0)
	for len(out) < most {
		shot, ok := s.Next()
		if !ok {
			break
		}
		out = append(out, shot)
	}
	return out
}

// drawn is where each shot's frame begins in the video, to the hundredth.
func drawn(g framewire.Grid, shots []Shot) []string {
	out := make([]string, len(shots))
	for i, s := range shots {
		out[i] = fmt.Sprintf("%.2f", g.At(s.Frame))
	}
	return out
}

func TestAProgramPutsThePiecesEndToEnd(t *testing.T) {
	p := NewProgram(playClip, false)
	if !near(p.Length(), 2.53+1.86+1.95) {
		t.Fatalf("the program lasts %v", p.Length())
	}
	if v, at, ok := p.Locate(0); !ok || v != 0 || at != 57 {
		t.Errorf("the start is visit %d at %v, %v", v, at, ok)
	}
	if v, at, ok := p.Locate(2.54); !ok || v != 1 || !near(at, 60.32) {
		t.Errorf("2.54 is visit %d at %v, %v", v, at, ok)
	}
	if _, _, ok := p.Locate(p.Length()); ok {
		t.Error("the end of a program that does not loop is in a visit")
	}
	// A moment in a cut is the start of the piece after it, one before
	// the clip its start, and one after it its end.
	for _, c := range []struct{ moment, place float64 }{{58, 1}, {60, 2.53}, {10, 0}, {70, p.Length()}} {
		if got := p.Place(c.moment); !near(got, c.place) {
			t.Errorf("%v is placed at %v, not %v", c.moment, got, c.place)
		}
	}
	// Pieces come in order, and one that is nothing is left out.
	q := NewProgram([]PlayPiece{{63.05, 65}, {10, 10}, {57, 59.53}}, false)
	if v, ok := q.Visit(1); !ok || v.Start != 63.05 || !near(v.From, 2.53) {
		t.Errorf("the second visit is %+v, %v", v, ok)
	}
}

func TestALoopGoesRoundOnTheSameClock(t *testing.T) {
	p := NewProgram(playClip, true)
	v, at, ok := p.Locate(p.Length() + 0.5)
	if !ok || v != 3 || !near(at, 57.5) {
		t.Fatalf("half a second into the second time is visit %d at %v, %v", v, at, ok)
	}
	if w, _ := p.Visit(4); !near(w.From, p.Length()+2.53) {
		t.Errorf("the fifth visit starts at %v", w.From)
	}
	// Switched off in the second time through, it ends after that time.
	p.Loop(false, p.Length()+0.5)
	if !near(p.End(), 2*p.Length()) {
		t.Errorf("switched off in the second time, it ends at %v", p.End())
	}
	if _, ok := p.Visit(6); ok {
		t.Error("a visit past the end is played")
	}
	p.Loop(true, 0)
	if !math.IsInf(p.End(), 1) {
		t.Errorf("switched on, it ends at %v", p.End())
	}
}

func TestAPlayDrawsEveryFrameOfEveryPieceAndNoneFromACut(t *testing.T) {
	p := NewProgram(playClip, false)
	shots := allShots(playGrid, &p, 0, 1000)
	want := slices.Concat(framesFrom(57, 59.48), framesFrom(60.28, 62.12), framesFrom(63.04, 64.96))
	if got := drawn(playGrid, shots); !slices.Equal(got, want) {
		t.Fatalf("the play draws\n%v\nnot\n%v", got, want)
	}
	// The first piece from the start of the program, a frame each 40 ms.
	for k, at := range map[int]float64{0: 0, 1: 0.04, 62: 2.48, 63: 2.53, 64: 2.54, 109: 2.53 + 1.81, 110: 2.53 + 1.86} {
		if !near(shots[k].At, at) {
			t.Errorf("shot %d is drawn at %v, not %v", k, shots[k].At, at)
		}
	}
	// The second piece begins at 2.53 with the frame that holds 60.31, and
	// stands at 60.31 there, and the frame of 60.32 begins 10 ms later.
	if s := shots[63]; drawn(playGrid, shots[63:64])[0] != "60.28" || !near(s.Moment, 60.31) {
		t.Errorf("the second piece starts with %+v", s)
	}
	if !near(shots[64].Moment, 60.32) {
		t.Errorf("its second frame stands at %v", shots[64].Moment)
	}
	// The frames either side of each cut are drawn however short a time
	// they are due for, and no other.
	var edges []int
	for k, s := range shots {
		if s.Edge {
			edges = append(edges, k)
		}
	}
	if want := []int{0, 62, 63, 109, 110, len(shots) - 1}; !slices.Equal(edges, want) {
		t.Errorf("the edges are shots %v, not %v", edges, want)
	}
}

// A play begun inside a frame drew the frames on a grid from where it
// began: each up to a frame late, and the last frame of a piece passed
// over when the piece ended less than that into it. A millisecond before
// the next frame is that frame's, see framewire.Hair, so 0.038 is the
// latest.
func TestAPlayBegunInsideAFrameDrawsEveryFrameWhereItBegins(t *testing.T) {
	p := NewProgram(playClip, false)
	for _, into := range []float64{0.001, 0.013, 0.02, 0.038} {
		p0 := p.Place(57.4 + into)
		shots := allShots(playGrid, &p, p0, 1000)
		if !near(shots[0].At, p0) || drawn(playGrid, shots[:1])[0] != "57.40" || !near(shots[0].Moment, 57.4+into) {
			t.Errorf("%v into a frame, the play starts with %+v", into, shots[0])
		}
		if !near(shots[1].At, 0.44) {
			t.Errorf("%v into a frame, the second shot is at %v", into, shots[1].At)
		}
		want := slices.Concat(framesFrom(57.4, 59.48), framesFrom(60.28, 62.12), framesFrom(63.04, 64.96))
		if got := drawn(playGrid, shots); !slices.Equal(got, want) {
			t.Errorf("%v into a frame, the play draws %v", into, got)
		}
	}
}

func TestAPlayBegunAtACutStartsOnTheNextPiecesFirstFrame(t *testing.T) {
	p := NewProgram(playClip, false)
	shots := allShots(playGrid, &p, p.Place(60), 1000)
	if got, want := drawn(playGrid, shots), slices.Concat(framesFrom(60.28, 62.12), framesFrom(63.04, 64.96)); !slices.Equal(got, want) {
		t.Errorf("from the cut the play draws %v", got)
	}
	if !near(shots[1].At, 2.54) {
		t.Errorf("its second shot is at %v", shots[1].At)
	}
	if got := allShots(playGrid, &p, p.Length(), 10); len(got) != 0 {
		t.Errorf("a play from the end draws %v", got)
	}
}

func TestAPieceEndingOnAFramesStartEndsOnTheFrameBefore(t *testing.T) {
	p := NewProgram([]PlayPiece{{10, 10.4}, {20.02, 20.1}}, false)
	shots := allShots(playGrid, &p, 0, 100)
	if got, want := drawn(playGrid, shots), append(framesFrom(10, 10.36), "20.00", "20.04"); !slices.Equal(got, want) {
		t.Errorf("the play draws %v", got)
	}
	if !near(shots[10].At, 0.4) || !near(shots[11].At, 0.42) {
		t.Errorf("the second piece is drawn at %v and %v", shots[10].At, shots[11].At)
	}
}

// The seam of a loop is a cut: the clip's last frame, then its first, at
// the place one time through ends.
func TestALoopsSeamIsACut(t *testing.T) {
	p := NewProgram(playClip, true)
	shots := allShots(playGrid, &p, 0, 170)
	got := drawn(playGrid, shots)
	seam := slices.Index(got, "64.96")
	if seam < 0 || got[seam+1] != "57.00" || !near(shots[seam+1].At, p.Length()) {
		t.Fatalf("the loop goes from %v to %v at %v", got[seam], got[seam+1], shots[seam+1].At)
	}
	// Switched off while it plays, it ends where that time through ends.
	q := NewProgram(playClip, true)
	s := NewShots(playGrid, &q, 0)
	n := 0
	for _, ok := s.Next(); ok; _, ok = s.Next() {
		if n++; n == 10 {
			q.Loop(false, 0.4)
		}
	}
	if n != len(got[:seam+1]) {
		t.Errorf("switched off in the first time through, it drew %d frames, not %d", n, seam+1)
	}
}

// A picture that starts after its sound: in a grid of 30 frames a second
// from 0.25, a piece that begins before it starts with the first frame,
// drawn where the piece begins.
func TestAPieceBeforeThePictureStartsWithItsFirstFrame(t *testing.T) {
	g := framewire.Grid{Start: 0.25, Num: 30, Den: 1, Count: 30}
	p := NewProgram([]PlayPiece{{0.1, 0.4}}, false)
	shots := allShots(g, &p, 0, 10)
	frames := make([]int, len(shots))
	for i, s := range shots {
		frames[i] = s.Frame
	}
	if !slices.Equal(frames, []int{0, 1, 2, 3}) {
		t.Fatalf("the play draws frames %v", frames)
	}
	if !near(shots[0].At, 0) || math.Abs(shots[1].At-(0.28333-0.1)) > 1e-4 || math.Abs(shots[3].At-(0.35-0.1)) > 1e-4 {
		t.Errorf("drawn at %v, %v and %v", shots[0].At, shots[1].At, shots[3].At)
	}
}

func TestThePlaysSoundIsCutAtTheSampleAndPutEndToEnd(t *testing.T) {
	p := NewProgram(playClip, false)
	var got []Stretch
	s := NewStretches(&p, 0, 48000)
	for st, ok := s.Next(); ok; st, ok = s.Next() {
		got = append(got, st)
	}
	want := [][3]int64{{57 * 48000, 59.53 * 48000, 0}, {60.31 * 48000, 60.31*48000 + 89280, 121440}, {63.05 * 48000, 63.05*48000 + 93600, 210720}}
	if len(got) != len(want) {
		t.Fatalf("the sound is %d stretches", len(got))
	}
	for i, st := range got {
		if [3]int64{st.From, st.To, st.Out} != want[i] {
			t.Errorf("stretch %d is %+v", i, st)
		}
		if i > 0 && st.Out != got[i-1].Out+(got[i-1].To-got[i-1].From) {
			t.Errorf("stretch %d does not start where the one before ends", i)
		}
	}
	// A play begun inside a piece starts its sound there, faded in as the
	// piece is, not from the play's start.
	from := NewStretches(&p, 1, 48000)
	if st, _ := from.Next(); st.Out != 48000 || st.From != 58*48000 || st.PieceOut != 0 {
		t.Errorf("from a second in, the sound is %+v", st)
	}
}

func TestTheSoundIsFadedAtEachEdgeOfItsPieceTheWayTheRenderFadesIt(t *testing.T) {
	r := 48000
	f := Fade * float64(r)
	st := Stretch{PieceOut: 0, PieceEnd: 48000}
	for _, c := range []struct {
		m    int64
		gain float64
	}{{0, 0.5 / f}, {int64(f), 1}, {24000, 1}, {47999, 0.5 / f}} {
		if got := st.Gain(c.m, r); math.Abs(got-c.gain) > 1e-9 {
			t.Errorf("sample %d is at %v, not %v", c.m, got, c.gain)
		}
	}
}

// The frames a play draws of a piece are the frames the render keeps of
// it, see pieceFrames and cutOf, at 29.97 frames a second, where most
// frames start on no millisecond, and with a picture that starts after
// its sound.
func TestAPlayDrawsTheFramesTheRenderCuts(t *testing.T) {
	for _, s := range []SourceInfo{
		{FPSNum: 30000, FPSDen: 1001, Duration: 600},
		{FPSNum: 25, FPSDen: 1, Duration: 600, VideoStart: 0.044},
	} {
		g := s.Grid()
		var pieces []PlayPiece
		for _, at := range [][2]float64{{12.345, 15.678}, {20.01, 20.2}, {33.333, 40}} {
			a, b := s.PieceOnFrames(at[0], at[1])
			pieces = append(pieces, PlayPiece{a, b})
		}
		p := NewProgram(pieces, false)
		shots := allShots(g, &p, 0, 10000)
		var want []int
		for _, q := range pieces {
			first, end := s.pieceFrames(q.Start, q.End)
			for k := first; k < end; k++ {
				want = append(want, int(k-s.firstFrame()))
			}
		}
		got := make([]int, len(shots))
		for i, shot := range shots {
			got[i] = shot.Frame
		}
		if !slices.Equal(got, want) {
			t.Errorf("at %d/%d the play draws frames\n%v\nand the render cuts\n%v", s.FPSNum, s.FPSDen, got, want)
		}
	}
}
