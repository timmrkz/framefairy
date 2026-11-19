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

// servePlay starts a play of the video preview, see engine.Play: the
// clip's pieces, whether they loop, and the moment of the video the
// playhead is at, with the size, the grid and the sound card's rate and
// channels. It answers with two streams, read with /frames/read like any
// other: the play's pictures, each written with where on the program it is
// drawn, the moment of the video the play stands at there, where its frame
// begins and 1 where it is an edge, see engine.Shot, and its sound, each
// chunk with the sample of the program it starts at and how many moments
// it holds. And where on the program the play starts and ends, -1 while it
// loops, and how long one time through lasts. A newer play, or a look of the hand, stops it. A cue waits for
// /frames/play/go before it takes the decoder, see engine.PlayAsk.
//
// It answers with the moment it starts from and where its first frame
// begins, -1 for none, too. One that takes over from a play under way,
// after, starts at the moment that play stands at at a place on its
// program, and is refused with 409 where the new pieces do not hold it.
//
//	/frames/play?path=<episode>&w=<width>&h=<height>&grid=<grid>&rate=<hz>&ch=<channels>&pieces=<start>:<end>,...&at=<seconds>[&loop=1][&cue=1][&after=<pictures>&place=<seconds>]
//	  {"pictures": "...", "sound": "...", "from": <seconds>, "end": <seconds>, "length": <seconds>, "moment": <seconds>, "first": <seconds>}
//	/frames/play/go?id=<pictures>
//	/frames/play/loop?id=<pictures>&at=<seconds>[&loop=1]  {"end": <seconds>}
func (p *previews) servePlay(st *store, w http.ResponseWriter, r *http.Request) {
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
	at, err := strconv.ParseFloat(q.Get("at"), 64)
	if !ok || err != nil || math.IsNaN(at) || math.IsInf(at, 0) {
		http.Error(w, "a play needs its pieces and where the playhead is", http.StatusBadRequest)
		return
	}
	loop := q.Get("loop") == "1"
	if after := q.Get("after"); after != "" {
		// A play that takes over from the one before, the clip changed
		// while it plays: from the moment that one stands at, at the place
		// on its program the window names, where the new pieces hold it.
		// The window goes on with the one before where they do not.
		old := p.get(after)
		place, err := strconv.ParseFloat(q.Get("place"), 64)
		var moment float64
		ok := old != nil && old.play != nil && err == nil
		if ok {
			moment, ok = old.play.MomentAt(place)
		}
		if !ok || !engine.NewProgram(pieces, loop).Holds(moment) {
			w.WriteHeader(http.StatusConflict)
			return
		}
		at = moment
	}
	dec, err := p.decoder(st.machine, path)
	if err != nil {
		http.Error(w, err.Error(), http.StatusInternalServerError)
		return
	}
	play := p.schedule(dec).Play(engine.PlayAsk{Grid: hand.Grid, W: hand.W, H: hand.H, Pieces: pieces, Loop: loop, Moment: at,
		Cue: q.Get("cue") == "1"})
	grid := hand.Grid
	// The pictures are the schedule's, kept for a play started there
	// again, so none is given back.
	pictures, s := p.startStamped("play", dec.Light, nil, func(ctx context.Context, got func(previewFrame) error) error {
		return play.Pictures(ctx, func(f engine.PlayFrame) error {
			edge := 0.0
			if f.Edge {
				edge = 1
			}
			return got(previewFrame{at: f.At, stamp: []float64{f.Moment, grid.At(f.Frame), edge}, data: f.Picture})
		})
	})
	s.play = play
	e := engine.NewEngine(engineLog("files", path), st.machine)
	sound, _ := p.startStamped("playsound", nil, dec.Give, func(ctx context.Context, got func(previewFrame) error) error {
		source := func(ctx context.Context, from float64, rate, channels int, got func(float64, []byte) error) error {
			return dec.Sound(ctx, e, from, rate, channels, got)
		}
		return play.Sound(ctx, source, rate, channels, func(out int64, chunk []byte) error {
			return got(previewFrame{at: float64(out), stamp: []float64{float64(len(chunk) / (4 * channels))}, data: chunk})
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

// servePlayGo gives the decoder to a cued play, see engine.Play.Go.
func (p *previews) servePlayGo(w http.ResponseWriter, r *http.Request) {
	s := p.get(r.URL.Query().Get("id"))
	if s == nil || s.play == nil {
		http.NotFound(w, r)
		return
	}
	s.play.Go()
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
