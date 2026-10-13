package main

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"math"
	"net/http"
	"path/filepath"
	"slices"
	"strconv"
	"sync"
	"time"

	"github.com/rs/zerolog"

	"framefairy/engine"
	"framefairy/internal/framewire"
)

// The captions of the video preview, drawn by libass in the episode's
// decoder from the caption file the render burns in, see
// engine.Engine.PreviewCaptions and docs/CAPTIONS.md:
//
//	/frames/captions?path=<episode>&track=<track>&at=<seconds>&w=<width>&h=<height>[&part=word|rest][&typing=<caption> <word>][&<drafts>]
//	                                 the captions of a view, by its track,
//	                                 on the frame of the episode that starts
//	                                 at, at width by height, as a
//	                                 framewire.Layer, with the caption and
//	                                 the word it shows in the header
//	                                 X-Caption-Showing, "<caption> <word>",
//	                                 see engine.CaptionShowing, and in
//	                                 X-Caption-Scale how large the pill of
//	                                 that word is drawn on it, 1 at rest,
//	                                 see engine.PopScale. With part, only
//	                                 the word being spoken with its pill,
//	                                 or all but those, at rest, which the
//	                                 video preview bounces the word with
//	                                 after a word step, see
//	                                 engine.CaptionDrawing.Lifted, the
//	                                 word with the curve in the header
//	                                 X-Caption-Bounce, "<ms> <rise>
//	                                 <peak>", see engine.BounceCurve. With
//	                                 typing, the caption without the ink
//	                                 of that word, which the caption box
//	                                 draws as it is typed, see
//	                                 engine.CaptionDrawing.Typing
//
//	/frames/captions/words?path=<episode>&track=<track>[&<drafts>]
//	                                 where the words of a view stand in the
//	                                 short, as JSON, see engine.WordFrame:
//	                                 for each caption its words in order,
//	                                 each the box its pill takes at rest
//
// The drafts are what the hand changes while it moves and the video
// preview shows on the way: text, box and highlight, each a colour #RRGGBB
// with its opacity from 0 to 1 in textOpacity, boxOpacity and
// highlightOpacity, the switches textOn, boxOn and highlightOn, 0 or 1,
// and y, where the captions sit, as SetCaptionsHeight takes it. Each is
// checked as the edit that saves it checks it.

// captionTracks are the drawings of the caption views the interface was
// given lately, by their tracks, each with the episode it belongs to. A
// view is asked for again whenever the captions change, so the latest few
// hundred are plenty. The one used last goes last, so the captions on
// screen are never the ones forgotten, however many a drag makes.
var captionTracks = &tracks{drawings: map[string]trackOf{}}

type tracks struct {
	mu       sync.Mutex
	drawings map[string]trackOf
	order    []string
}

type trackOf struct {
	episode string
	drawing *engine.CaptionDrawing
}

const tracksKept = 512

// keep notes what a caption view of an episode was made from, so the
// video preview can ask for its captions by its track.
func (t *tracks) keep(episode string, v *engine.CaptionsView) {
	if v == nil || v.Drawing() == nil {
		return
	}
	t.mu.Lock()
	defer t.mu.Unlock()
	t.used(v.Track)
	t.drawings[v.Track] = trackOf{episode: episode, drawing: v.Drawing()}
	for len(t.order) > tracksKept {
		delete(t.drawings, t.order[0])
		t.order = t.order[1:]
	}
}

func (t *tracks) get(track string) (trackOf, bool) {
	t.mu.Lock()
	defer t.mu.Unlock()
	d, ok := t.drawings[track]
	if ok {
		t.used(track)
	}
	return d, ok
}

// used puts a track last in the order, as the one used most lately.
func (t *tracks) used(track string) {
	if i := slices.Index(t.order, track); i >= 0 {
		t.order = slices.Delete(t.order, i, i+1)
	}
	t.order = append(t.order, track)
}

// serveCaptions answers /frames/captions. The engine's caption code runs
// here outside a job, so a panic in it fails the request, not the app.
func (p *previews) serveCaptions(st *store, w http.ResponseWriter, r *http.Request) {
	defer func() {
		if v := recover(); v != nil {
			theLog.line(zerolog.ErrorLevel, "captions").Str("panic", fmt.Sprint(v)).Msg("drawing the captions failed")
			http.Error(w, "the captions could not be drawn", http.StatusInternalServerError)
		}
	}()
	q := r.URL.Query()
	path := q.Get("path")
	kept, ok := captionTracks.get(q.Get("track"))
	if !ok || kept.episode != path || !episodeFile(st, path) {
		http.NotFound(w, r)
		return
	}
	at, err := strconv.ParseFloat(q.Get("at"), 64)
	width, _ := strconv.Atoi(q.Get("w"))
	height, _ := strconv.Atoi(q.Get("h"))
	part := q.Get("part")
	if part != "" && part != engine.LiftWord && part != engine.LiftRest {
		err = fmt.Errorf("part is %q", engine.Scrub(part, 20))
	}
	// The word being typed in the caption box, which libass leaves out.
	typing := engine.CaptionShowing{Caption: -1, Word: -1}
	if err == nil && q.Has("typing") {
		var n int
		if n, err = fmt.Sscanf(q.Get("typing"), "%d %d", &typing.Caption, &typing.Word); err == nil && n != 2 {
			err = fmt.Errorf("typing is %q", q.Get("typing"))
		}
	}
	if err != nil || math.IsNaN(at) || math.IsInf(at, 0) || width < 1 || height < 1 || width > 7680 || height > 7680 {
		http.Error(w, "captions need a moment, a width and a height", http.StatusBadRequest)
		return
	}
	drafts, err := captionDrafts(q.Get)
	if err != nil {
		http.Error(w, err.Error(), http.StatusBadRequest)
		return
	}
	started := time.Now()
	ctx, cancel := context.WithTimeout(r.Context(), 10*time.Second)
	defer cancel()
	source, err := sources.probe(ctx, path)
	if err == nil {
		var dec *engine.EpisodeFrames
		dec, err = p.decoder(path)
		if err == nil {
			// The short's size as the render takes it, which is what the
			// caption file is laid out at.
			e := engine.NewEngine(engine.NewLog(io.Discard, false, false), st.files)
			o := jobOptions(st, e)
			drawing := kept.drawing.Typing(typing).Lifted(part)
			var l framewire.Layer
			// A video whose folder was deleted outside the app is let go
			// as the app comes back to the front, see letGoOfRemoved, and
			// until then its captions write nothing into a folder that is
			// gone, which would bring the folder back. The engine checks
			// again as it writes, see engine.ErrNoWorkFolder.
			if !marked(st.files, path) {
				http.NotFound(w, r)
				return
			}
			l, err = e.PreviewCaptions(ctx, dec, engine.WorkDir(path), drawing, drafts, source, o.RenderSettings(), at, width, height)
			if errors.Is(err, engine.ErrNoWorkFolder) {
				http.NotFound(w, r)
				return
			}
			if err == nil {
				// A layer slower than a frame is a pill behind the sound
				// while the clip plays, and on a customer's machine the log
				// is the only place that shows it.
				if took := time.Since(started); source.FPSNum > 0 &&
					took > time.Duration(float64(time.Second)*float64(source.FPSDen)/float64(source.FPSNum)) {
					theLog.line(zerolog.DebugLevel, "captions").Str("video", filepath.Base(path)).
						Int64("ms", took.Milliseconds()).Msg("a caption layer took longer than a frame")
				}
				// What the layer shows, which the caption box over it shows.
				showing := kept.drawing.Showing(source, at)
				w.Header().Set("Content-Type", "application/octet-stream")
				w.Header().Set("X-Caption-Showing", fmt.Sprintf("%d %d", showing.Caption, showing.Word))
				// A part is drawn at rest, and the word comes with the
				// curve the video preview bounces it by.
				scale := 1.0
				if part == "" {
					scale = drawing.PopScale(source, at)
				}
				if part == engine.LiftWord {
					length, rise, peak := engine.BounceCurve()
					w.Header().Set("X-Caption-Bounce", fmt.Sprintf("%g %g %g", length, rise, peak))
				}
				w.Header().Set("X-Caption-Scale", strconv.FormatFloat(scale, 'f', 4, 64))
				_, _ = w.Write(l.Body())
				return
			}
		}
	}
	if r.Context().Err() == nil {
		theLog.line(zerolog.WarnLevel, "captions").Err(err).Msg("the captions could not be drawn")
		http.Error(w, err.Error(), http.StatusInternalServerError)
	}
}

// serveWordFrames answers /frames/captions/words: where libass puts the
// words of a view, so the caption box frames the word libass drew. Like
// the captions, it runs outside a job, so a panic fails the request.
func (p *previews) serveWordFrames(st *store, w http.ResponseWriter, r *http.Request) {
	defer func() {
		if v := recover(); v != nil {
			theLog.line(zerolog.ErrorLevel, "captions").Str("panic", fmt.Sprint(v)).Msg("placing the caption words failed")
			http.Error(w, "the caption words could not be placed", http.StatusInternalServerError)
		}
	}()
	q := r.URL.Query()
	path := q.Get("path")
	kept, ok := captionTracks.get(q.Get("track"))
	if !ok || kept.episode != path || !episodeFile(st, path) {
		http.NotFound(w, r)
		return
	}
	drafts, err := captionDrafts(q.Get)
	if err != nil {
		http.Error(w, err.Error(), http.StatusBadRequest)
		return
	}
	ctx, cancel := context.WithTimeout(r.Context(), 10*time.Second)
	defer cancel()
	source, err := sources.probe(ctx, path)
	if err != nil {
		if r.Context().Err() == nil {
			theLog.line(zerolog.WarnLevel, "captions").Err(err).Msg("the caption words could not be placed")
			http.Error(w, err.Error(), http.StatusInternalServerError)
		}
		return
	}
	e := engine.NewEngine(engine.NewLog(io.Discard, false, false), st.files)
	frames := e.PreviewWordFrames(ctx, kept.drawing, drafts, source, jobOptions(st, e).RenderSettings())
	if frames == nil {
		frames = [][]engine.WordFrame{}
	}
	w.Header().Set("Content-Type", "application/json")
	_ = json.NewEncoder(w).Encode(frames)
}

// captionDrafts are the drafts of a request for captions as the engine
// takes them, see engine.CaptionDrafts.
func captionDrafts(get func(string) string) (map[string]any, error) {
	drafts := map[string]any{}
	for _, c := range []struct{ name, key string }{
		{"text", "primary"}, {"box", "back_colour"}, {"highlight", "highlight_colour"},
	} {
		hex := get(c.name)
		if hex == "" {
			continue
		}
		opacity, err := strconv.ParseFloat(get(c.name+"Opacity"), 64)
		if err != nil || math.IsNaN(opacity) {
			opacity = 1
		}
		colour, ok := engine.AssColour(hex, opacity)
		if !ok {
			return nil, fmt.Errorf("%s is not a colour", engine.Scrub(hex, 20))
		}
		drafts[c.key] = colour
	}
	for _, c := range []struct{ name, key string }{{"textOn", "text"}, {"boxOn", "box"}, {"highlightOn", "highlight"}} {
		switch get(c.name) {
		case "0":
			drafts[c.key] = 0.0
		case "1":
			drafts[c.key] = 1.0
		}
	}
	if y, err := strconv.ParseFloat(get("y"), 64); err == nil && !math.IsNaN(y) && !math.IsInf(y, 0) {
		drafts["margin_v"] = engine.SnapCaptionY(y)
	}
	return drafts, nil
}
