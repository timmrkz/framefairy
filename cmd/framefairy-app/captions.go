package main

import (
	"context"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"errors"
	"fmt"
	"math"
	"net/http"
	"net/url"
	"path/filepath"
	"slices"
	"strconv"
	"sync"
	"time"

	"github.com/rs/zerolog"

	"framefairy/engine"
	"framefairy/engine/runes"
)

// The captions of the video preview, drawn by libass in the episode's
// decoder from the caption file the render burns in, see
// engine.Engine.PreviewCaptions and docs/CAPTIONS.md:
//
//	/frames/dress?path=<episode>&framing=<framing>&at=<seconds>&w=<width>&h=<height>[&on=video][&track=<track>[&part=word|rest][&typing=<caption> <word>][&<drafts>]][&dressKey=<key>][&have=<tag>]
//	                                 the dress of the frame of the episode
//	                                 that starts at, as the clip of that
//	                                 framing has it, see Framing.Name, or
//	                                 with on=video as the video shows it,
//	                                 as filmed, in the header
//	                                 X-Frame-Dress as JSON, see sentDress:
//	                                 engine.FrameDress, the ask's key, and
//	                                 with a track the layer of captions
//	                                 over it, its tag, the size it is
//	                                 drawn at, what it shows and how large
//	                                 its pill is. The layer itself, drawn
//	                                 in the crop of a video preview width
//	                                 by height, see layerSize, is the body,
//	                                 as a framewire.Layer, or nothing with
//	                                 204 where have names it, see layerTag.
//	                                 With part, only the word being spoken
//	                                 with its pill, or all but those, at
//	                                 rest, which the video preview bounces
//	                                 the word with after a word step, see
//	                                 engine.CaptionDrawing.Lifted, the
//	                                 word with the curve it bounces by,
//	                                 see engine.BounceCurve. With typing,
//	                                 the caption without the ink of that
//	                                 word, which the caption box draws as
//	                                 it is typed, see
//	                                 engine.CaptionDrawing.Typing. Every
//	                                 frame the window is sent comes with
//	                                 the same, see writeDress
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

// tracks are the drawings of the caption views the interface was given
// lately, by their tracks, each with the episode it belongs to. A view is
// asked for again whenever the captions change, so the latest few hundred
// are plenty, see recent. The video preview's streams keep them, see
// previews. Its zero value is ready.
type tracks struct {
	recent[trackOf]
}

type trackOf struct {
	episode string
	drawing *engine.CaptionDrawing
}

// keep notes what a caption view of an episode was made from, so the
// video preview can ask for its captions by its track.
func (t *tracks) keep(episode string, v *engine.CaptionsView) {
	if v == nil || v.Drawing() == nil {
		return
	}
	// A step of a drag that draws what a saved clip draws, under the same
	// track, leaves the saved drawing kept: a drawing in the hand gets no
	// whole caption file, see engine.Engine.previewTrackAt, and the clip
	// played after a drag that changed nothing would never get one.
	t.put(v.Track, trackOf{episode: episode, drawing: v.Drawing()}, func(was trackOf) bool {
		return was.episode == episode && !was.drawing.InHand() && v.Drawing().InHand()
	})
}

// recent keeps what the window was sent lately, by its name. The one
// used last goes last, so what is on screen is never what is forgotten,
// however many a drag makes. Its zero value is ready.
type recent[T any] struct {
	mu     sync.Mutex
	byName map[string]T
	order  []string
}

// recentKept is how many a recent keeps.
const recentKept = 512

// put keeps v by name, unless what is kept by that name is one keepWas
// says to keep. keepWas may be nil.
func (r *recent[T]) put(name string, v T, keepWas func(was T) bool) {
	r.mu.Lock()
	defer r.mu.Unlock()
	if r.byName == nil {
		r.byName = map[string]T{}
	}
	r.used(name)
	if was, ok := r.byName[name]; ok && keepWas != nil && keepWas(was) {
		return
	}
	r.byName[name] = v
	for len(r.order) > recentKept {
		delete(r.byName, r.order[0])
		r.order = r.order[1:]
	}
}

func (r *recent[T]) get(name string) (T, bool) {
	r.mu.Lock()
	defer r.mu.Unlock()
	v, ok := r.byName[name]
	if ok {
		r.used(name)
	}
	return v, ok
}

// used puts a name last in the order, as the one used most lately.
func (r *recent[T]) used(name string) {
	if i := slices.Index(r.order, name); i >= 0 {
		r.order = slices.Delete(r.order, i, i+1)
	}
	r.order = append(r.order, name)
}

// serveDress answers /frames/dress: the dress of a frame of a clip the
// window holds, see engine.FrameDress, in the header X-Frame-Dress as
// JSON, see sentDress, and with a track the captions over it as the body,
// or no body where have names the layer the window has, see dressFrame. A frame the window is sent comes with its dress, see
// writeDress, so this is for a frame it holds whose clip changed, and for
// the parts of a word that bounces after a word step.
func (s *FrameFairy) serveDress(w http.ResponseWriter, r *http.Request) {
	q := r.URL.Query()
	width, _ := strconv.Atoi(q.Get("w"))
	height, _ := strconv.Atoi(q.Get("h"))
	at, err := strconv.ParseFloat(q.Get("at"), 64)
	if err != nil || math.IsNaN(at) || math.IsInf(at, 0) {
		http.Error(w, "a dress needs a moment", http.StatusBadRequest)
		return
	}
	a, err := s.readDress(q.Get("path"), q, width, height)
	switch {
	case errors.Is(err, errDressUnknown) || (err == nil && a == nil):
		http.NotFound(w, r)
		return
	case err != nil:
		http.Error(w, err.Error(), http.StatusBadRequest)
		return
	}
	ctx, cancel := s.previews.captionsWithin(r.Context())
	defer cancel()
	d, err := s.dressFrame(ctx, a, at, q.Get("have"))
	if errors.Is(err, engine.ErrNoWorkFolder) {
		http.NotFound(w, r)
		return
	}
	if err != nil {
		// The app's log says it once, with the reason, see logged. A
		// request the window called off is told nothing.
		if r.Context().Err() == nil {
			http.Error(w, err.Error(), http.StatusInternalServerError)
		}
		return
	}
	head, _ := json.Marshal(d.sent)
	w.Header().Set("X-Frame-Dress", string(head))
	// The layer the window has: nothing drawn or sent again.
	if d.sent.Tag != "" && d.layer == nil {
		w.WriteHeader(http.StatusNoContent)
		return
	}
	w.Header().Set("Content-Type", "application/octet-stream")
	for _, part := range d.layer {
		if _, err := w.Write(part); err != nil {
			return
		}
	}
}

// layerTag names the layer a request for captions asks for: by all it
// asks but the frame, and by what of the frame the drawing depends on,
// the moments of the short around it over which the caption file changes
// nothing, see engine.CaptionDrawing.Steady, the size of the short
// there, and the size the layer is drawn at. Two frames with one tag are drawn the same, so a request that
// brings the tag of the layer it has is answered that it is unchanged.
// It is a header of our own, not HTTP's If-None-Match and 304: Wails
// takes If-None-Match off every request on Windows, because WebView2
// hangs on a 304, see request_windows.go in Wails' assetserver.
func layerTag(named url.Values, d *engine.CaptionDrawing, source engine.SourceInfo, rs engine.RenderSettings, at float64, layerW, layerH int) string {
	from, to := d.Steady(source, at)
	shortW, shortH := d.ShortSize(source, rs, at)
	h := sha256.New()
	fmt.Fprintf(h, "%s\x00%d %d %dx%d %dx%d", named.Encode(), from, to, shortW, shortH, layerW, layerH)
	return hex.EncodeToString(h.Sum(nil)[:12])
}

// serveWordFrames answers /frames/captions/words: where libass puts the
// words of a view, so the caption box frames the word libass drew. Like
// the captions, it runs outside a job, so a panic fails the request.
func (s *FrameFairy) serveWordFrames(w http.ResponseWriter, r *http.Request) {
	st := s.store
	q := r.URL.Query()
	path := q.Get("path")
	kept, ok := s.previews.tracks.get(q.Get("track"))
	if !ok || kept.episode != path || !episodeFile(st, path) {
		http.NotFound(w, r)
		return
	}
	drafts, err := captionDrafts(q.Get)
	if err != nil {
		http.Error(w, err.Error(), http.StatusBadRequest)
		return
	}
	ctx, cancel := s.previews.captionsWithin(r.Context())
	defer cancel()
	source, err := s.probe(ctx, path)
	if err = waitedOn(ctx, r, "probing the video", err); err != nil {
		// The app's log says it once, with the reason, see logged.
		if r.Context().Err() == nil {
			http.Error(w, err.Error(), http.StatusInternalServerError)
		}
		return
	}
	e := engine.NewEngine(engineLog("captions", path), st.machine)
	frames := e.PreviewWordFrames(ctx, kept.drawing, drafts, source, jobOptions(st, e).RenderSettings())
	if frames == nil {
		frames = [][]engine.WordFrame{}
	}
	w.Header().Set("Content-Type", "application/json")
	_ = json.NewEncoder(w).Encode(frames)
}

// captionsWait is how long a request for captions may take. A layer that
// comes later is for a frame the video preview has long moved on from.
const captionsWait = 10 * time.Second

// captionsWithin is the context a request for captions runs in: the
// request's own, ended after captionsWait.
func (p *previews) captionsWithin(asked context.Context) (context.Context, context.CancelFunc) {
	if p.within != nil {
		return p.within(asked)
	}
	return context.WithTimeout(asked, captionsWait)
}

// waitedOn is err, saying which step of a request for captions it came
// in when the request's time ran out, so its answer and the app's log
// say what took the time. Four asks among many once answered only
// "context deadline exceeded", and nothing said whether the probe, the
// caption file or the decoder held them, issue #687. A request called off
// by the page is not told anything, see serveCaptions.
func waitedOn(within context.Context, r *http.Request, step string, err error) error {
	if err == nil || within.Err() == nil || r.Context().Err() != nil {
		return err
	}
	return fmt.Errorf("%s took more than the %s a request for captions may take: %w", step, captionsWait, err)
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
			return nil, fmt.Errorf("%s is not a colour", runes.Scrub(hex, 20))
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

// layerTally counts the caption layers drawn for each video and those
// slower than a frame, and says so in one line once a video has asked for
// none for a while, the end of a stretch of playing or stepping. A line
// for every layer slower than a frame was most of the log while a clip
// played, and the report's memory of the last lines held minutes rather
// than the hour it is sized for. The video preview's streams keep it, see
// previews. Its zero value is ready.
type layerTally struct {
	mu      sync.Mutex
	byVideo map[string]*tally
}

type tally struct {
	drawn, slow, unchanged int
	worst                  time.Duration
	quiet                  *time.Timer
}

// layersQuiet is how long a video asks for no layer before its stretch
// is said to have ended.
const layersQuiet = 2 * time.Second

// add counts a layer of the video at path the decoder drew in drew, of a
// short whose frames last frame.
func (lt *layerTally) add(path string, drew, frame time.Duration) {
	lt.mu.Lock()
	defer lt.mu.Unlock()
	t := lt.of(path)
	t.drawn++
	if drew > frame {
		t.slow++
	}
	t.worst = max(t.worst, drew)
}

// unchanged counts a layer of the video at path answered unchanged, not
// drawn, see layerTag.
func (lt *layerTally) unchanged(path string) {
	lt.mu.Lock()
	defer lt.mu.Unlock()
	lt.of(path).unchanged++
}

// of is the tally of the video at path, its stretch of asking held open
// for another layersQuiet. lt.mu is held.
func (lt *layerTally) of(path string) *tally {
	t := lt.byVideo[path]
	if t == nil {
		if lt.byVideo == nil {
			lt.byVideo = map[string]*tally{}
		}
		t = &tally{}
		lt.byVideo[path] = t
		t.quiet = time.AfterFunc(layersQuiet, func() { lt.said(path) })
	} else {
		t.quiet.Reset(layersQuiet)
	}
	return t
}

// said writes the line of a stretch that ended and starts the next.
func (lt *layerTally) said(path string) {
	lt.mu.Lock()
	t := lt.byVideo[path]
	delete(lt.byVideo, path)
	lt.mu.Unlock()
	if t == nil || t.drawn+t.unchanged == 0 {
		return
	}
	level := zerolog.TraceLevel
	if t.slow > 0 {
		level = zerolog.DebugLevel
	}
	theLog.line(level, "captions").Str("video", filepath.Base(path)).Int("drawn", t.drawn).Int("unchanged", t.unchanged).Int("slow", t.slow).
		Int64("slowest_ms", t.worst.Milliseconds()).Msg("caption layers drawn and answered unchanged, those slower than a frame counted")
}
