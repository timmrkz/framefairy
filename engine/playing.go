package engine

import (
	"cmp"
	"context"
	"encoding/binary"
	"errors"
	"math"
	"slices"
	"sync"

	"framefairy/internal/framewire"
)

// A play of the video preview, run by the schedule of its video, see
// Schedule, which gives the decoder to one thing at a time: the hand, the
// reading ahead, or a play. The window pulls the play's frames and its
// sound a batch at a time, each stamped with where on the program it
// goes, and the decoder is kept no more than a few frames ahead of what
// it pulled. See docs/decisions/2026-11-19-go-plays-the-video-preview.md.

// PlayAsk is a play the window asks for: the clip's pieces, whether they
// loop, and the moment of the video the playhead is at, in frames of the
// grid at a size.
type PlayAsk struct {
	Grid   framewire.Grid
	W, H   int
	Pieces []PlayPiece
	Loop   bool
	Moment float64
	// Cue says the play waits for Go before it takes the decoder: it
	// hands over the frames kept from its first on, and its sound, so a
	// press of the space bar starts it at once, and leaves the decoder to
	// the hand and the reading ahead until then.
	Cue bool
}

// PlayFrame is a frame of a play: its shot and its picture, see
// framewire.Picture, which nobody may change. Whoever is handed it gives
// its picture back with Schedule.Give once it is written out.
type PlayFrame struct {
	Shot
	Picture []byte
	// Gen is how many times the play had taken a changed clip in when
	// the frame was made, see Retake.
	Gen int
}

// Play is a play of the video preview.
type Play struct {
	s      *Schedule
	grid   framewire.Grid
	w, h   int
	p0     float64
	ctx    context.Context
	cancel context.CancelFunc
	// going is closed while the play has the decoder, see Go, and made
	// again when it gives it back, see Hold. lease is cancelled then, which
	// stops the decoder's stream, under the schedule's lock.
	going chan struct{}
	lease context.Context
	free  context.CancelFunc
	// held says the play is paused, see Hold, under the schedule's lock.
	held bool

	mu      sync.Mutex
	program Program
	// retake is the play that takes over from this one, see Retake, and
	// stopped says this one was stopped, so it hands over to none.
	retake  *retaking
	stopped bool
	// retakes keeps one change at a time going to the play, and gens
	// counts them, see Retake.
	retakes sync.Mutex
	gens    int
}

// retaking is the play of a clip changed while a play plays, which takes
// over from it at a place on its program, at, and counts on its clock,
// its own places moved by shift. Gen is what its frames and its sound are
// stamped with. Switched says it took the decoder over, and entered that
// its frames or its sound went out, so no change takes its place.
type retaking struct {
	at       float64
	next     *Play
	shift    float64
	gen      int
	switched bool
	entered  bool
}

// errRetaken ends what a play hands over where the play of its clip
// changed takes over, see Retake.
var errRetaken = errors.New("the play of the changed clip takes over")

// Retaken is how a play took a changed clip in, see Play.Retake: from At
// on its program what it hands over is the play of the new pieces, whose
// frames and sound carry Gen, and it now ends at End, +Inf while it loops,
// its first time through the new pieces ending at Lap.
type Retaken struct {
	Gen          int
	At, End, Lap float64
}

// Retake hands a play the pieces of its clip changed while it plays, at a
// place on its program the sound heard has not reached yet: from there on
// it plays the new pieces, from the moment of the video it stands at
// there, counted on the same clock, so the picture and the sound go on
// with no stop. The frames and the sound it handed over from there on are
// the old pieces' and make way, which the Gen of the new ones tells. The
// play of the new pieces waits as a cue until the frames reach the place,
// and takes the decoder there, see Go, so the frames kept for it come
// first. A change that comes before the last one took over takes its
// place, at its place. False where the new pieces do not hold the moment,
// or the play is stopped, paused or past its end there, and the play goes
// on as it was. Only the play the window started is handed changes. See
// docs/decisions/2026-11-25-the-video-preview-draws-what-go-sends.md.
func (p *Play) Retake(pieces []PlayPiece, loop bool, at float64) (Retaken, bool) {
	p.retakes.Lock()
	defer p.retakes.Unlock()
	if playing, _ := p.Current(); playing.isHeld() {
		return Retaken{}, false
	}
	cur, shift := p, 0.0
	for {
		r, entered := cur.entered()
		if entered {
			at = math.Max(at, r.at+shift)
			shift += r.shift
			cur = r.next
			continue
		}
		// The play that plays now. A change that has not taken over yet
		// makes way for this one, at its place.
		place := at - shift
		if r != nil {
			place = r.at
		}
		moment, ok := cur.MomentAt(place)
		if !ok || cur.ctx.Err() != nil || !NewProgram(pieces, loop).Holds(moment) {
			return Retaken{}, false
		}
		next := p.s.Play(PlayAsk{Grid: p.grid, W: p.w, H: p.h, Pieces: pieces, Loop: loop, Moment: moment, Cue: true})
		p.gens++
		mine := &retaking{at: place, next: next, shift: place - next.From(), gen: p.gens}
		cur.mu.Lock()
		same := cur.retake == r && (r == nil || !r.entered)
		if same {
			cur.retake = mine
		}
		cur.mu.Unlock()
		if !same {
			// The change before took over meanwhile: this one goes to
			// the play that took over.
			next.end()
			continue
		}
		if r != nil {
			r.next.end()
		}
		if _, entered := cur.entered(); cur.ctx.Err() != nil && !entered {
			next.end()
			return Retaken{}, false
		}
		return Retaken{Gen: mine.gen, At: place + shift, End: next.End() + mine.shift + shift, Lap: next.Length() + mine.shift + shift}, true
	}
}

// retaking is the play of a changed clip that takes over from this one,
// or nil, and whether it took the decoder over.
func (p *Play) retaking() (*retaking, bool) {
	p.mu.Lock()
	defer p.mu.Unlock()
	return p.retake, p.retake != nil && p.retake.switched
}

// entered is the play of a changed clip that takes over from this one, or
// nil, and whether its frames or its sound went out, see enter.
func (p *Play) entered() (*retaking, bool) {
	p.mu.Lock()
	defer p.mu.Unlock()
	return p.retake, p.retake != nil && p.retake.entered
}

// enter says the frames or the sound of the play that takes over from
// this one go out, and is that play.
func (p *Play) enter() *retaking {
	p.mu.Lock()
	defer p.mu.Unlock()
	p.retake.entered = true
	return p.retake
}

// retaken is the play that takes over from this one, or nil.
func (p *Play) retaken() *retaking {
	p.mu.Lock()
	defer p.mu.Unlock()
	return p.retake
}

// isHeld says the play is paused, see Hold.
func (p *Play) isHeld() bool {
	p.s.mu.Lock()
	defer p.s.mu.Unlock()
	return p.held
}

// Current is the play that plays now: this one, or the play of the
// changed clip that took over from it, see Retake, with where its places
// stand on this one's program.
func (p *Play) Current() (*Play, float64) {
	cur, shift := p, 0.0
	for {
		r, switched := cur.retaking()
		if !switched {
			return cur, shift
		}
		shift += r.shift
		cur = r.next
	}
}

// handOver gives the decoder to the play that takes over, where this one
// has it, and stops this one: its frames from the place on are the new
// play's. A play stopped hands over to none, and says so.
func (p *Play) handOver() bool {
	p.mu.Lock()
	if p.stopped {
		p.mu.Unlock()
		return false
	}
	r := p.retake
	r.switched, r.entered = true, true
	p.mu.Unlock()
	p.s.mu.Lock()
	going := p.s.play == p
	p.s.mu.Unlock()
	if going {
		r.next.Go()
	}
	p.cancel()
	return true
}

// end stops the play, and the play of a changed clip waiting to take
// over from it.
func (p *Play) end() {
	p.mu.Lock()
	p.stopped = true
	r := p.retake
	switched := r != nil && r.switched
	p.mu.Unlock()
	if r != nil && !switched {
		r.next.end()
	}
	p.cancel()
}

// playDepth is how many frames of a run of a play are decoded ahead of
// what the window pulled, and how many of the next run's first frames are
// waiting for it, so a cut never waits for a seek.
const playDepth = 4

// Play starts a play of the video, which takes the decoder from the hand
// and from the reading ahead, at once or with Go where it is a cue: what
// they wait for is answered with ErrOvertaken. A newer play, or a look of
// the hand, stops it once it has the decoder.
func (s *Schedule) Play(a PlayAsk) *Play {
	program := NewProgram(a.Pieces, a.Loop)
	ctx, stop := context.WithCancel(context.Background())
	p := &Play{s: s, grid: a.Grid, w: a.W, h: a.H, program: program, p0: program.Place(a.Moment), ctx: ctx, cancel: stop, going: make(chan struct{})}
	if !a.Cue {
		p.Go()
	}
	return p
}

// Go gives the decoder to a cued play, see PlayAsk.Cue, or to one held,
// see Hold. A play that has it already, or was stopped, is left as it is.
func (p *Play) Go() {
	s := p.s
	s.mu.Lock()
	defer s.mu.Unlock()
	if p.ctx.Err() != nil {
		return
	}
	if s.play == p {
		return
	}
	if p.free != nil {
		p.free()
	}
	p.held = false
	p.lease, p.free = context.WithCancel(p.ctx)
	select {
	case <-p.going:
	default:
		close(p.going)
	}
	if old := s.play; old != nil {
		old.end()
	}
	s.play = p
	s.overtake()
	if a := s.ahead; a != nil {
		s.ahead = nil
		a.done <- aheadDone{err: ErrOvertaken}
	}
	s.stopRun()
}

// Hold gives the decoder back while the play is paused, so the hand and
// the reading ahead have it, and keeps everything else: the frames and the
// sound handed over stay the window's, and Go takes the decoder again and
// goes on from the next frame. Kept, a paused play held the decoder, and
// nothing was read ahead until it was closed.
func (p *Play) Hold() {
	s := p.s
	s.mu.Lock()
	defer s.mu.Unlock()
	if s.play != p {
		return
	}
	p.free()
	p.held = true
	p.going = make(chan struct{})
	s.play = nil
	s.plan()
}

// stopPlay stops the play that has the decoder, for the hand. Under the
// lock.
func (s *Schedule) stopPlay() {
	if p := s.play; p != nil {
		p.end()
		s.play = nil
	}
}

// From is where on the program the play starts.
func (p *Play) From() float64 {
	return p.p0
}

// MomentAt is the moment of the video a place on the program stands for,
// and false at or past its end.
func (p *Play) MomentAt(at float64) (float64, bool) {
	_, moment, ok := p.Locate(at)
	return moment, ok
}

// First is the frame the play starts with, and false where it has none.
func (p *Play) First() (int, bool) {
	s, ok := NewShots(p.grid, p, p.p0).Next()
	return s.Frame, ok
}

// Length is how long one time through the program lasts.
func (p *Play) Length() float64 {
	p.mu.Lock()
	defer p.mu.Unlock()
	return p.program.Length()
}

// End is where on the program the play ends, +Inf while it loops.
func (p *Play) End() float64 {
	p.mu.Lock()
	defer p.mu.Unlock()
	return p.program.End()
}

// Loop switches the loop while the play plays, at a place on the program,
// see Program.Loop, and says where it now ends.
func (p *Play) Loop(on bool, at float64) float64 {
	cur, shift := p.Current()
	cur.mu.Lock()
	cur.program.Loop(on, at-shift)
	end := cur.program.End() + shift
	r := cur.retake
	cur.mu.Unlock()
	// A change waiting to take over loops as this one does.
	if r != nil {
		r.next.mu.Lock()
		r.next.program.Loop(on, at-shift-r.shift)
		r.next.mu.Unlock()
	}
	return end
}

// Plays says whether the play, or the change of the clip it takes in
// next, plays these pieces: a clip changed in nothing a play plays, its
// crop moved or its captions changed, is no change of the play.
func (p *Play) Plays(pieces []PlayPiece) bool {
	cur, _ := p.Current()
	cur.mu.Lock()
	r := cur.retake
	cur.mu.Unlock()
	if r != nil && !r.switched {
		cur = r.next
	}
	cur.mu.Lock()
	defer cur.mu.Unlock()
	return slices.Equal(cur.program.pieces, NewProgram(pieces, false).pieces)
}

// Visit is Program.Visit, while the loop may be switched.
func (p *Play) Visit(v int) (Visit, bool) {
	p.mu.Lock()
	defer p.mu.Unlock()
	return p.program.Visit(v)
}

// Locate is Program.Locate, while the loop may be switched.
func (p *Play) Locate(at float64) (int, float64, bool) {
	p.mu.Lock()
	defer p.mu.Unlock()
	return p.program.Locate(at)
}

// Stop stops the play.
func (p *Play) Stop() {
	// And every play that took over from it since, so a play stopped by
	// the one it was started as stops whatever plays on in its place.
	for cur := p; ; {
		cur.end()
		cur.s.mu.Lock()
		if cur.s.play == cur {
			cur.s.play = nil
			cur.s.plan()
		}
		cur.s.mu.Unlock()
		r, switched := cur.retaking()
		if !switched {
			return
		}
		cur = r.next
	}
}

// Pictures hands over the play's frames in the order they are drawn, from
// the frames kept where they are, until the program ends, got says no
// more, or the play or ctx is stopped. Every run of frames that follow one
// another is one stream of the decoder, and the next run's stream starts
// while this one plays, so a cut never waits for a seek. Each frame the
// decoder makes is kept, so a play started again where this one stood, a
// clip changed under it, starts at once. A frame handed to got is got's
// to give back, whatever it answers.
func (p *Play) Pictures(ctx context.Context, got func(PlayFrame) error) error {
	cur, shift, gen := p, 0.0, 0
	for {
		at, g := shift, gen
		err := cur.pictures(ctx, func(f PlayFrame) error {
			f.At += at
			f.Gen = g
			return got(f)
		})
		if !errors.Is(err, errRetaken) {
			return err
		}
		r := cur.retaken()
		cur, shift, gen = r.next, shift+r.shift, r.gen
	}
}

// pictures is Pictures of this play alone, which ends with errRetaken
// where the play of a changed clip takes over.
func (p *Play) pictures(ctx context.Context, got func(PlayFrame) error) error {
	ctx, cancel := context.WithCancel(ctx)
	defer cancel()
	go func() {
		select {
		case <-p.ctx.Done():
			cancel()
		case <-ctx.Done():
		}
	}()
	defer func() {
		p.s.mu.Lock()
		if p.s.play == p && p.ctx.Err() == nil {
			p.s.play = nil
			p.s.plan()
		}
		p.s.mu.Unlock()
	}()
	runs := playRuns{shots: NewShots(p.grid, p, p.p0)}
	cur := p.startRun(ctx, runs.next())
	next := p.startRun(ctx, runs.next())
	// The frames on their way when the play ends go to nobody.
	defer func() {
		cancel()
		go p.drop(cur)
		go p.drop(next)
	}()
	for cur != nil {
		for f := range cur.out {
			if r := p.retaken(); r != nil && f.At >= r.at-1e-9 {
				p.s.Give(f.Picture)
				if !p.handOver() {
					return context.Canceled
				}
				return errRetaken
			}
			if err := got(f); err != nil {
				return err
			}
		}
		// The play stopped ends a run before the ctx made from it hears.
		if err := cmp.Or(ctx.Err(), p.ctx.Err()); err != nil {
			return err
		}
		if cur.err != nil {
			return cur.err
		}
		cur, next = next, p.startRun(ctx, runs.next())
	}
	if r := p.retaken(); r != nil {
		if !p.handOver() {
			return context.Canceled
		}
		return errRetaken
	}
	return nil
}

// drop gives back the frames of a run nobody takes, once its stream has
// stopped.
func (p *Play) drop(r *playRun) {
	if r == nil {
		return
	}
	for f := range r.out {
		p.s.Give(f.Picture)
	}
}

// playRuns cuts the shots of a play into runs of frames that follow one
// another, each of which is one stream of the decoder.
type playRuns struct {
	shots *Shots
	held  *Shot
}

func (r *playRuns) next() []Shot {
	var run []Shot
	if r.held != nil {
		run = append(run, *r.held)
		r.held = nil
	}
	for {
		s, ok := r.shots.Next()
		if !ok {
			return run
		}
		if len(run) > 0 && s.Frame != run[len(run)-1].Frame+1 {
			r.held = &s
			return run
		}
		run = append(run, s)
	}
}

// playRun is a run of a play's frames on its way: the frames, playDepth
// of them ahead of what was taken, and why the decoder stopped, once out
// is closed.
type playRun struct {
	out chan PlayFrame
	err error
}

// errRunDone ends the stream of a run that has its frames.
var errRunDone = errors.New("the run has its frames")

// startRun starts the frames of a run: the ones kept from its first on
// from where they are kept, and the rest from a stream of the decoder from
// the first that is not.
func (p *Play) startRun(ctx context.Context, shots []Shot) *playRun {
	if len(shots) == 0 {
		return nil
	}
	r := &playRun{out: make(chan PlayFrame, playDepth)}
	key := func(frame int) keptKey { return keptKey{grid: p.grid, w: p.w, h: p.h, frame: frame} }
	// A frame sent is counted as sent under the lock it was kept under,
	// see keptFrames.lend, and one that could not be sent is given back.
	send := func(ctx context.Context, f PlayFrame) error {
		select {
		case r.out <- f:
			return nil
		case <-ctx.Done():
			p.s.Give(f.Picture)
			return ctx.Err()
		}
	}
	go func() {
		defer close(r.out)
		i := 0
		// Once for every time the play has the decoder: a play held while
		// this run's frames were on their way goes on from the next one,
		// which the decoder kept if it had it already.
		for {
			for ; i < len(shots); i++ {
				p.s.mu.Lock()
				k, ok := p.s.kept.get(key(shots[i].Frame))
				if ok {
					p.s.kept.lend(k.picture)
				}
				p.s.mu.Unlock()
				if !ok {
					break
				}
				if send(ctx, PlayFrame{Shot: shots[i], Picture: k.picture}) != nil {
					return
				}
			}
			if i == len(shots) {
				return
			}
			p.s.mu.Lock()
			going := p.going
			p.s.mu.Unlock()
			select {
			case <-going:
			case <-ctx.Done():
				return
			}
			p.s.mu.Lock()
			lease := p.lease
			p.s.mu.Unlock()
			// Stopped, or held again before the stream began.
			if p.ctx.Err() != nil {
				return
			}
			if lease.Err() != nil {
				continue
			}
			held, stop := context.WithCancel(ctx)
			undo := context.AfterFunc(lease, stop)
			err := p.s.frames.Stream(held, p.grid, p.grid.At(shots[i].Frame), p.w, p.h, false, nil, func(at float64, picture []byte) error {
				frame := p.grid.Holding(at)
				if frame != shots[i].Frame {
					// Nobody keeps it or reads it.
					p.s.frames.Give(picture)
					return nil
				}
				p.s.mu.Lock()
				p.s.kept.put(key(frame), at, picture)
				p.s.kept.lend(picture)
				p.s.mu.Unlock()
				if err := send(held, PlayFrame{Shot: shots[i], Picture: picture}); err != nil {
					return err
				}
				if i++; i == len(shots) {
					return errRunDone
				}
				return nil
			})
			undo()
			stop()
			switch {
			case ctx.Err() != nil, p.ctx.Err() != nil, errors.Is(err, errRunDone):
				return
			case lease.Err() != nil:
				// Held: the next time through waits for Go.
			case err != nil:
				r.err = err
				return
			default:
				return
			}
		}
	}()
	return r
}

// SoundSource is where a play's sound comes from, the episode's decoder
// in the app, see EpisodeFrames.Sound.
type SoundSource func(ctx context.Context, from float64, rate, channels int, got func(at float64, chunk []byte) error) error

// errStretchDone ends the sound of a stretch that has its samples.
var errStretchDone = errors.New("the stretch has its sound")

// Sound hands over the play's sound in the order it is heard, as 32-bit
// float samples at rate in so many channels, the channels of one moment
// side by side, little endian, each chunk with the sample of the program
// it starts at: every piece cut at the sample and faded at its edges, see
// Stretch. It runs until the program ends, got says no more, or the play
// or ctx is stopped. A chunk is in a buffer of its own, which got may keep
// or give back, see EpisodeFrames.Give.
func (p *Play) Sound(ctx context.Context, sound SoundSource, rate, channels int, got func(out int64, gen int, chunk []byte) error) error {
	cur, places, shift, gen := p, 0.0, int64(0), 0
	for {
		at, g := shift, gen
		err := cur.sound(ctx, sound, rate, channels, func(out int64, chunk []byte) error {
			return got(out+at, g, chunk)
		})
		if !errors.Is(err, errRetaken) {
			return err
		}
		r := cur.enter()
		cur, places, gen = r.next, places+r.shift, r.gen
		shift = int64(math.Round(places * float64(rate)))
	}
}

// sound is Sound of this play alone, which ends with errRetaken where the
// play of a changed clip takes over. It goes on to that place though the
// pictures took over first and stopped the play.
func (p *Play) sound(ctx context.Context, sound SoundSource, rate, channels int, got func(out int64, chunk []byte) error) error {
	ctx, cancel := context.WithCancel(ctx)
	defer cancel()
	go func() {
		select {
		case <-p.ctx.Done():
			// Stopped where a changed clip took over, the sound goes on
			// to the place, unless the play that took over was stopped
			// too.
			if r := p.retaken(); r == nil || r.next.ctx.Err() != nil {
				cancel()
			}
		case <-ctx.Done():
		}
	}()
	// Where the play of a changed clip takes over, in samples, or -1.
	cut := func() int64 {
		if r := p.retaken(); r != nil {
			return int64(math.Round(r.at * float64(rate)))
		}
		return -1
	}
	stretches := NewStretches(p, p.p0, rate)
	for st, ok := stretches.Next(); ok; st, ok = stretches.Next() {
		if c := cut(); c >= 0 && st.Out >= c {
			return errRetaken
		}
		m, left := st.Out, st.To-st.From
		err := sound(ctx, float64(st.From)/float64(rate), rate, channels, func(_ float64, chunk []byte) error {
			n := min(int64(len(chunk)/(4*channels)), left)
			end := cut()
			retaken := end >= 0 && m+n > end
			if retaken {
				n = max(end-m, 0)
			}
			chunk = chunk[:n*int64(4*channels)]
			for i := range n {
				g := st.Gain(m+i, rate)
				if g == 1 {
					continue
				}
				for c := range channels {
					at := (int(i)*channels + c) * 4
					v := math.Float32frombits(binary.LittleEndian.Uint32(chunk[at:]))
					binary.LittleEndian.PutUint32(chunk[at:], math.Float32bits(float32(float64(v)*g)))
				}
			}
			if n > 0 {
				if err := got(m, chunk); err != nil {
					return err
				}
			}
			if retaken {
				return errRetaken
			}
			m, left = m+n, left-n
			if left == 0 {
				return errStretchDone
			}
			return nil
		})
		if err != nil && !errors.Is(err, errStretchDone) {
			return err
		}
	}
	if cut() >= 0 {
		return errRetaken
	}
	return ctx.Err()
}
