package engine

import (
	"math"
	"slices"

	"framefairy/internal/framewire"
)

// What a play of the video preview plays, worked out where the render's
// rules are, see
// docs/decisions/2026-11-19-go-plays-the-video-preview.md. The window says
// which pieces, whether they loop and where the playhead is, and is sent
// the frames and the sound, each stamped with where on the program it
// goes. It keeps the sound card's clock and draws the frame whose moment
// has come, and knows nothing of frames, cuts or fades.
//
// The words used here:
//
//   - The program is what a play plays: the pieces of the clip one after
//     the other, from the first again while it loops. A place on it is in
//     seconds from the start of its first piece, and it goes on growing
//     through a loop, so it never goes back while playing.
//   - A visit is one piece played once. With a loop the second time round
//     is another visit of the same piece.
//   - A shot is a frame of the grid drawn at a place on the program: where
//     that frame begins, carried onto the program through its piece. A
//     visit's first shot is where the visit begins on the program, or
//     where the play began, with the frame that holds that moment. Its
//     last is the frame that holds the moment a frame before the piece
//     ends, the frame before the one its end means, see cutOf. So the
//     last frame before a cut is followed straight by the first frame
//     after it, both drawn, whatever moment the play began at.
//   - A stretch is the sound of one visit, cut at the sample where the
//     visit begins and ends on the program, and faded at both edges of its
//     piece by Fade, the way the render cuts and fades it.

// PlayPiece is a piece of a clip a play plays, from Start to End, in
// seconds of the video.
type PlayPiece struct {
	Start, End float64
}

// Program is the pieces a play plays, one after the other, once or round
// and round.
type Program struct {
	pieces []PlayPiece
	// Where each piece starts on one time through, and the length after
	// the last.
	offsets []float64
	// How many times through it plays, or -1 while it loops.
	times int
}

// NewProgram is the program of a clip's pieces, in order, without any
// that is nothing.
func NewProgram(pieces []PlayPiece, loop bool) Program {
	p := Program{times: 1}
	if loop {
		p.times = -1
	}
	for _, q := range pieces {
		if q.End > q.Start && !math.IsNaN(q.Start) && !math.IsInf(q.End, 0) {
			p.pieces = append(p.pieces, q)
		}
	}
	slices.SortStableFunc(p.pieces, func(a, b PlayPiece) int {
		switch {
		case a.Start < b.Start:
			return -1
		case a.Start > b.Start:
			return 1
		}
		return 0
	})
	at := 0.0
	for _, q := range p.pieces {
		p.offsets = append(p.offsets, at)
		at += q.End - q.Start
	}
	p.offsets = append(p.offsets, at)
	return p
}

// Length is how long one time through lasts.
func (p Program) Length() float64 {
	return p.offsets[len(p.offsets)-1]
}

// Loops says it goes round and round.
func (p Program) Loops() bool {
	return p.times < 0
}

// End is where the program ends, as a place on it, or +Inf while it loops.
func (p Program) End() float64 {
	if p.times < 0 {
		return math.Inf(1)
	}
	return float64(p.times) * p.Length()
}

// Loop switches the loop while a play plays, at a place on the program:
// on, it goes round from here on, and off, it ends at the end of the time
// through that place is in.
func (p *Program) Loop(on bool, at float64) {
	if on {
		p.times = -1
		return
	}
	p.times = 1
	if n := p.Length(); n > 0 {
		p.times = int(math.Floor(math.Max(at, 0)/n)) + 1
	}
}

// Visit is one piece played once: which time it is played, which piece,
// where it starts on the program, and the piece's own edges in the video.
type Visit struct {
	V, Piece         int
	From, Start, End float64
}

// Visit is the v-th piece played, and false past the end of a program
// that does not loop.
func (p Program) Visit(v int) (Visit, bool) {
	n := len(p.pieces)
	if n == 0 || v < 0 || (p.times >= 0 && v >= p.times*n) {
		return Visit{}, false
	}
	time, piece := v/n, v%n
	q := p.pieces[piece]
	return Visit{V: v, Piece: piece, From: float64(time)*p.Length() + p.offsets[piece], Start: q.Start, End: q.End}, true
}

// Locate is the visit a place on the program is in and the moment of the
// video it stands for, and false at or past the end of a program that does
// not loop.
func (p Program) Locate(at float64) (v int, moment float64, ok bool) {
	n, length := len(p.pieces), p.Length()
	if n == 0 || length <= 0 || at >= p.End() || math.IsNaN(at) {
		return 0, 0, false
	}
	time := math.Floor(math.Max(at, 0) / length)
	within := math.Max(at-time*length, 0)
	piece := 0
	for piece+1 < n && p.offsets[piece+1] <= within {
		piece++
	}
	q := p.pieces[piece]
	return int(time)*n + piece, math.Min(q.Start+(within-p.offsets[piece]), q.End), true
}

// Place is where on the program a moment of the video is, for a play that
// starts there. Inside a piece it is its own place. In a cut, or before
// the first piece, it is the start of the piece after it, and past the
// last piece the end.
func (p Program) Place(moment float64) float64 {
	for i, q := range p.pieces {
		if moment < q.Start {
			return p.offsets[i]
		}
		if moment < q.End {
			return p.offsets[i] + (moment - q.Start)
		}
	}
	return p.Length()
}

// Shot is a frame of a play: the frame of the grid, where on the program
// it is drawn, and the moment of the video the play stands at there.
type Shot struct {
	Frame  int
	At     float64
	Moment float64
}

// shotsOf are the shots of one visit, of a play that began at p0 on the
// program, in the order they are drawn: one a frame, from the frame that
// holds where the visit begins, or where the play began, to the frame
// that holds the moment a frame before its piece ends.
func shotsOf(g framewire.Grid, v Visit, p0 float64) []Shot {
	from := math.Max(v.From, p0)
	to := v.From + (v.End - v.Start)
	if to-from <= 1e-9 || !g.Valid() {
		return nil
	}
	moment := v.Start + (from - v.From)
	frame := float64(g.Den) / float64(g.Num)
	a := g.Holding(moment)
	b := max(a, g.Holding(math.Max(moment, v.End-frame)))
	shots := make([]Shot, 0, b-a+1)
	shots = append(shots, Shot{Frame: a, At: from, Moment: moment})
	for r := a + 1; r <= b; r++ {
		begins := g.At(r)
		shots = append(shots, Shot{Frame: r, At: v.From + (begins - v.Start), Moment: begins})
	}
	return shots
}

// playProgram is what the shots and the sound of a play read of its
// program: a Program, or a Play, whose loop may be switched while it plays.
type playProgram interface {
	Visit(v int) (Visit, bool)
	Locate(at float64) (int, float64, bool)
}

// Shots are the shots of a play, worked out a visit at a time as they are
// asked for.
type Shots struct {
	grid    framewire.Grid
	program playProgram
	p0      float64
	next    int
	ahead   []Shot
}

// NewShots are the shots of a play of the program from p0 on it, on the
// grid of the video's frames. A program whose loop is switched while it
// plays changes what comes after the visits already worked out.
func NewShots(g framewire.Grid, program playProgram, p0 float64) *Shots {
	s := &Shots{grid: g, program: program, p0: p0, next: -1}
	if v, _, ok := program.Locate(p0); ok {
		s.next = v
	}
	return s
}

// Next is the next shot, and false once the program has ended.
func (s *Shots) Next() (Shot, bool) {
	for len(s.ahead) == 0 {
		if s.next < 0 {
			return Shot{}, false
		}
		v, ok := s.program.Visit(s.next)
		if !ok {
			s.next = -1
			return Shot{}, false
		}
		s.next++
		s.ahead = shotsOf(s.grid, v, s.p0)
	}
	shot := s.ahead[0]
	s.ahead = s.ahead[1:]
	return shot, true
}

// Stretch is the sound of one visit, in samples at the rate it is played
// at: [From, To) of the video, put on the program at Out, and the edges of
// its piece on the program, PieceOut and PieceEnd, which it fades at.
type Stretch struct {
	From, To, Out, PieceOut, PieceEnd int64
}

// stretchOf is the sound of a visit of a play that began at p0 on the
// program, at rate samples a second, and false where it has none. The
// edges are rounded to the sample on the program, so one visit ends where
// the next begins.
func stretchOf(v Visit, p0 float64, rate int) (Stretch, bool) {
	r := float64(rate)
	pieceOut := int64(math.Round(v.From * r))
	pieceEnd := int64(math.Round((v.From + v.End - v.Start) * r))
	out := max(pieceOut, int64(math.Round(p0*r)))
	if pieceEnd <= out {
		return Stretch{}, false
	}
	from := int64(math.Round(v.Start*r)) + (out - pieceOut)
	return Stretch{From: from, To: from + (pieceEnd - out), Out: out, PieceOut: pieceOut, PieceEnd: pieceEnd}, true
}

// Stretches are the stretches of a play, a visit at a time.
type Stretches struct {
	program playProgram
	p0      float64
	rate    int
	next    int
}

// NewStretches are the stretches of a play of the program from p0 on it,
// at rate samples a second.
func NewStretches(program playProgram, p0 float64, rate int) *Stretches {
	s := &Stretches{program: program, p0: p0, rate: rate, next: -1}
	if v, _, ok := program.Locate(p0); ok {
		s.next = v
	}
	return s
}

// Next is the next stretch, and false once the program has ended.
func (s *Stretches) Next() (Stretch, bool) {
	for s.next >= 0 {
		v, ok := s.program.Visit(s.next)
		if !ok {
			s.next = -1
			break
		}
		s.next++
		if st, ok := stretchOf(v, s.p0, s.rate); ok {
			return st, true
		}
	}
	return Stretch{}, false
}

// Gain is how loud sample m of the program is in the stretch: a straight
// fade over Fade at each edge of its piece, the way the render's afade
// fades a cut.
func (st Stretch) Gain(m int64, rate int) float64 {
	f := Fade * float64(rate)
	inside := math.Min(float64(m-st.PieceOut)+0.5, float64(st.PieceEnd-m)-0.5)
	if inside >= f {
		return 1
	}
	return math.Max(0, inside/f)
}
