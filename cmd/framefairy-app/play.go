package main

import (
	"context"
	"encoding/json"
	"math"
	"net/http"
	"strconv"
	"strings"

	"framefairy/engine"
)

// servePlay starts a play of the video preview, see engine.Play: of a
// clip, by the name of its framing, see Framing.Name, as program, apart
// from the framing of the dress words its frames are dressed with, which
// is the clip's on the video too, or of the video,
// by its pieces, whether it loops, and the moment of the video the
// playhead is at, with the size, the grid and the sound card's rate and
// channels. It answers with two streams, read with /frames/read like any
// other: the play's pictures, each written with where on the program it is
// drawn, the moment of the video the play stands at there, where its frame
// begins, 1 where it is an edge, see engine.Shot, and how many changes of
// the clip the play had taken in, see servePlayTake, and its sound, each
// chunk with the sample of the program it starts at, how many moments it
// holds and the changes taken in. And where on the program the play
// starts and ends, -1 while it loops, and how long one time through
// lasts. A newer play, or a look of the hand, stops it. A cue waits for
// /frames/play/go before it takes the decoder, see engine.PlayAsk, and a
// play paused gives it back with /frames/play/hold until /frames/play/go.
// It answers with the moment it starts from and where its first frame
// begins, -1 for none, too.
//
//	/frames/play?path=<episode>&w=<width>&h=<height>&grid=<grid>&rate=<hz>&ch=<channels>&(program=<framing>|pieces=<start>:<end>,...)&at=<seconds>[&loop=1][&cue=1][&<dress words>]
//	  {"pictures": "...", "sound": "...", "from": <seconds>, "end": <seconds>, "length": <seconds>, "moment": <seconds>, "first": <seconds>}
//	/frames/play/go?id=<pictures>
//	/frames/play/hold?id=<pictures>
//	/frames/play/loop?id=<pictures>&at=<seconds>[&loop=1]  {"end": <seconds>}
//	/frames/play/take?id=<pictures>&program=<framing>&heard=<seconds>&w=<width>&h=<height>[&loop=1][&<dress words>]  {"gen": <n>, "at": <seconds>, "end": <seconds>, "lap": <seconds>, "video": <bool>}
func (p *previews) servePlay(st *store, w http.ResponseWriter, r *http.Request, d *dresser) {
	path, hand, ok := handArgs(st, w, r, false)
	if !ok {
		return
	}
	q := r.URL.Query()
	rate, _ := strconv.Atoi(q.Get("rate"))
	channels, _ := strconv.Atoi(q.Get("ch"))
	if rate < 8000 || rate > 192000 || channels < 1 || channels > 8 {
		http.Error(w, "a play needs the sound card's rate and its number of channels", http.StatusBadRequest)
		return
	}
	pieces, ok := playPieces(q.Get("pieces"))
	if name := q.Get("program"); name != "" {
		f, known := p.framings.get(name)
		ok = known && f.episode == path && len(f.segments) > 0
		pieces = f.pieces()
	}
	at, err := strconv.ParseFloat(q.Get("at"), 64)
	if !ok || err != nil || math.IsNaN(at) || math.IsInf(at, 0) {
		http.Error(w, "a play needs its clip's framing or its pieces, and where the playhead is", http.StatusBadRequest)
		return
	}
	loop := q.Get("loop") == "1"
	dec, err := p.decoder(st.machine, path)
	if err != nil {
		http.Error(w, err.Error(), http.StatusInternalServerError)
		return
	}
	sched := p.schedule(dec)
	play := sched.Play(engine.PlayAsk{Grid: hand.Grid, W: hand.W, H: hand.H, Pieces: pieces, Loop: loop, Moment: at,
		Cue: q.Get("cue") == "1"})
	grid := hand.Grid
	// The pictures are the schedule's, kept for a play started there
	// again, and go back to it once written. Each is dressed as it is made,
	// as the change of the clip it was made after asks, see dresser.
	pictures, s := p.startStamped("play", dec.Light, sched.Give, func(ctx context.Context, got func(previewFrame) error) error {
		d.ctx = ctx
		return play.Pictures(ctx, func(f engine.PlayFrame) error {
			edge := 0.0
			if f.Edge {
				edge = 1
			}
			return got(previewFrame{at: f.At, stamp: []float64{f.Moment, grid.At(f.Frame), edge, float64(f.Gen)}, data: f.Picture,
				dressed: d.dress(f.Gen, f.Frame, grid.At(f.Frame))})
		})
	})
	s.play, s.episode, s.dresser = play, path, d
	e := engine.NewEngine(engineLog("files", path), st.machine)
	sound, _ := p.startStamped("playsound", nil, dec.Give, func(ctx context.Context, got func(previewFrame) error) error {
		source := func(ctx context.Context, from float64, rate, channels int, got func(float64, []byte) error) error {
			return dec.Sound(ctx, e, from, rate, channels, got)
		}
		return play.Sound(ctx, source, rate, channels, func(out int64, gen int, chunk []byte) error {
			return got(previewFrame{at: float64(out), stamp: []float64{float64(len(chunk) / (4 * channels)), float64(gen)}, data: chunk})
		})
	})
	first := -1.0
	if f, ok := play.First(); ok {
		first = grid.At(f)
	}
	w.Header().Set("Content-Type", "application/json")
	_ = json.NewEncoder(w).Encode(map[string]any{"pictures": pictures, "sound": sound, "from": play.From(), "end": endOf(play.End()),
		"length": play.Length(), "moment": at, "first": first})
}

// servePlayGo gives the decoder to a cued play, or to one held, see
// engine.Play.Go.
func (p *previews) servePlayGo(w http.ResponseWriter, r *http.Request) {
	s := p.get(r.URL.Query().Get("id"))
	if s == nil || s.play == nil {
		http.NotFound(w, r)
		return
	}
	cur, _ := s.play.Current()
	cur.Go()
	w.WriteHeader(http.StatusNoContent)
}

// servePlayHold gives the decoder back while the play is paused, see
// engine.Play.Hold.
func (p *previews) servePlayHold(w http.ResponseWriter, r *http.Request) {
	s := p.get(r.URL.Query().Get("id"))
	if s == nil || s.play == nil {
		http.NotFound(w, r)
		return
	}
	cur, _ := s.play.Current()
	cur.Hold()
	w.WriteHeader(http.StatusNoContent)
}

// servePlayLoop switches the loop of a play while it plays, at a place on
// the program, and answers where it now ends, see engine.Play.Loop.
func (p *previews) servePlayLoop(w http.ResponseWriter, r *http.Request) {
	q := r.URL.Query()
	s := p.get(q.Get("id"))
	at, err := strconv.ParseFloat(q.Get("at"), 64)
	if s == nil || s.play == nil || err != nil || math.IsNaN(at) || math.IsInf(at, 0) {
		http.NotFound(w, r)
		return
	}
	end := s.play.Loop(q.Get("loop") == "1", at)
	w.Header().Set("Content-Type", "application/json")
	_ = json.NewEncoder(w).Encode(map[string]float64{"end": endOf(end)})
}

// takeAhead is how far past the place on its program the window last
// said its sound reached a change of the clip is taken into the play,
// seconds: the frames there are the ones the play before decoded and the
// schedule kept, and its sound has to reach the window before that place
// is heard, see servePlayTake.
const takeAhead = 0.15

// servePlayTake hands the play under way the clip changed while it
// plays, by a drag, an undo or a redo, or its loop switched on, by the
// name of its new framing, with what its frames are dressed with now,
// the dress words of /frames/dress. The play takes it in a little past
// the place its sound reached, heard, from the moment of the video it
// stands at there, so the picture and the sound go on with no stop, and
// answers how many changes it has taken in, where this one took over,
// where the play now ends, -1 while it loops, and where its first time
// through the new pieces ends, see engine.Play.Retake. Its frames from
// there come dressed as it asks. Where the new pieces do not hold that
// moment, an edge of the clip dragged past the playhead, the play goes
// on as a play of the video from there and says so, the playhead being
// outside the clip, its frames dressed as the video's. A change that
// plays the same pieces, a crop moved or the captions changed, is no
// change of the play: its frames from now on are dressed as it asks, and
// it answers with no content. Asked with dress=1, only the dress
// changed, so a play of other pieces by now is left as it is. A framing
// of another video is not taken.
// 409 where the play is paused or past its end, and the window's next
// play plays the clip as it is now.
func (s *FrameFairy) servePlayTake(w http.ResponseWriter, r *http.Request) {
	p := &s.previews
	q := r.URL.Query()
	stream := p.get(q.Get("id"))
	f, known := p.framings.get(q.Get("program"))
	heard, err := strconv.ParseFloat(q.Get("heard"), 64)
	if stream == nil || stream.play == nil || stream.dresser == nil || !known || f.episode != stream.episode || len(f.segments) == 0 ||
		err != nil || math.IsNaN(heard) || math.IsInf(heard, 0) {
		http.NotFound(w, r)
		return
	}
	width, _ := strconv.Atoi(q.Get("w"))
	height, _ := strconv.Atoi(q.Get("h"))
	ask, err := s.readDress(stream.episode, q, width, height)
	if err != nil || ask == nil {
		http.NotFound(w, r)
		return
	}
	loop := q.Get("loop") == "1"
	if stream.play.Plays(f.pieces(), loop) {
		stream.dresser.redress(ask)
		w.WriteHeader(http.StatusNoContent)
		return
	}
	// Only the dress changed, and the play plays something else by now:
	// the change of the clip that comes after it brings its own.
	if q.Get("dress") == "1" {
		w.WriteHeader(http.StatusNoContent)
		return
	}
	at := heard + takeAhead
	var got engine.Retaken
	video := false
	ok := stream.dresser.take(func() (int, *dressAsk, bool) {
		var ok bool
		got, ok = stream.play.Retake(f.pieces(), loop, at)
		if ok || f.source.Duration <= 0 {
			return got.Gen, ask, ok
		}
		got, ok = stream.play.Retake([]engine.PlayPiece{{Start: 0, End: f.source.Duration}}, false, at)
		video = ok
		return got.Gen, ask.with(ask.clip, nil, "", true), ok
	})
	if !ok {
		w.WriteHeader(http.StatusConflict)
		return
	}
	w.Header().Set("Content-Type", "application/json")
	_ = json.NewEncoder(w).Encode(map[string]any{"gen": got.Gen, "at": got.At, "end": endOf(got.End), "lap": got.Lap, "video": video})
}

// endOf is where a play ends as the window is told it, -1 while it loops.
func endOf(end float64) float64 {
	if math.IsInf(end, 1) {
		return -1
	}
	return end
}

// playPieces reads the pieces of a play, "57:59.53,60.31:62.17", at most
// 1000 of them, and says whether they are pieces.
func playPieces(text string) ([]engine.PlayPiece, bool) {
	var out []engine.PlayPiece
	for part := range strings.SplitSeq(text, ",") {
		a, b, ok := strings.Cut(part, ":")
		start, err1 := strconv.ParseFloat(a, 64)
		end, err2 := strconv.ParseFloat(b, 64)
		if !ok || err1 != nil || err2 != nil || math.IsNaN(start) || math.IsNaN(end) || math.IsInf(start, 0) || math.IsInf(end, 0) || len(out) == 1000 {
			return nil, false
		}
		out = append(out, engine.PlayPiece{Start: start, End: end})
	}
	return out, len(out) > 0
}
