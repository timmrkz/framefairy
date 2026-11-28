package main

import (
	"context"
	"encoding/binary"
	"encoding/json"
	"errors"
	"fmt"
	"hash/maphash"
	"io"
	"net/url"
	"slices"
	"strconv"
	"sync"
	"time"

	"github.com/rs/zerolog"

	"framefairy/engine"
	"framefairy/internal/framewire"
)

// framed is what the frames of a clip are dressed from, as the window
// was sent it in a Framing: the episode, its picture, the clip's pieces
// with the keyframes that steer each, the keyframes on their frames, its
// rotations and the size of the short, see engine.SourceInfo.Dress.
type framed struct {
	episode   string
	source    engine.SourceInfo
	segments  []engine.SegmentView
	marks     []engine.CropMarkView
	rotations []engine.Rotation
	width     int
	height    int
}

// pieces is what a play of the framing's clip plays, see servePlay.
func (f framed) pieces() []engine.PlayPiece {
	out := make([]engine.PlayPiece, len(f.segments))
	for i, seg := range f.segments {
		out[i] = engine.PlayPiece{Start: seg.Start, End: seg.End}
	}
	return out
}

// name is what tells this framing from any other: all it dresses a frame
// from, so a clip that changed anything over its frames has a new name,
// and one that changed nothing keeps it.
func (f framed) name(rotations []engine.RotationView) string {
	var h maphash.Hash
	h.SetSeed(clipSeed)
	fmt.Fprintf(&h, "%s\x00%+v\x00%dx%d\x00", f.episode, f.source, f.width, f.height)
	_ = json.NewEncoder(&h).Encode([]any{f.segments, f.marks, rotations})
	return strconv.FormatUint(h.Sum64(), 36)
}

// dress is the dress of the frame that starts at a moment, as the frame
// of the clip, or with onVideo as the frame of the video, which shows the
// episode as filmed and is no frame of the clip's.
func (f framed) dress(at float64, onVideo bool) engine.FrameDress {
	rotations := f.rotations
	if onVideo {
		rotations = nil
	}
	d := f.source.Dress(f.segments, rotations, f.marks, at, f.width, f.height)
	if onVideo {
		d.In, d.Clip = false, false
	}
	return d
}

// layerSize is the size the captions over a dressed frame are drawn at,
// in device pixels: the crop as the video preview of width by height
// shows the picture turned, or nothing where no crop stands.
func layerSize(d engine.FrameDress, width, height int) (int, int) {
	if d.Width <= 0 || d.CropWidth <= 0 {
		return 0, 0
	}
	if d.Turn%180 != 0 {
		width, height = height, width
	}
	return int(float64(d.CropWidth)/float64(d.Width)*float64(width) + 0.5), height
}

// dressAsk is what the frames of an answer are dressed with, read from
// the request that asks for them, see readDress: the framing of a clip,
// see Framing.Name, on the clip or, with video, on the video, which shows
// the episode as filmed, and the captions of a track over a frame of the
// clip, with the drafts of the clip set, the word being typed left out,
// and, for the bounce after a word step, one part of the caption alone.
// The captions are drawn width by height, the canvas's size.
type dressAsk struct {
	path    string
	name    string
	clip    framed
	video   bool
	drawing *engine.CaptionDrawing
	typing  engine.CaptionShowing
	part    string
	drafts  map[string]any
	// named is what names the layers of captions it draws, see layerTag.
	named         url.Values
	width, height int
	// key and track are what the window named the ask by, and the track
	// of its captions, which each frame's dress says back, so the window
	// knows what a frame it keeps was dressed for, see sentDress.
	key, track string
}

// dressWords are the words of a request that say how its frames are
// dressed, see /frames/dress. dressKey is not one: it is what the window
// names the ask by, said back with each frame.
var dressWords = []string{"framing", "on", "track", "part", "typing", "text", "textOpacity", "box", "boxOpacity",
	"highlight", "highlightOpacity", "textOn", "boxOn", "highlightOn", "y", "lw", "lh"}

// errDressUnknown answers a framing or a track the app did not send the
// window, or sent for another video.
var errDressUnknown = errors.New("the framing or the track is not one the app sent for this video")

// readDress reads what the frames of the video at path are dressed with,
// for a video preview width by height, from a request, and nil where it
// names no framing: its frames come bare.
func (s *FrameFairy) readDress(path string, q url.Values, width, height int) (*dressAsk, error) {
	name := q.Get("framing")
	if name == "" {
		return nil, nil
	}
	clip, ok := s.previews.framings.get(name)
	if !ok || clip.episode != path || !s.store.Known(path) {
		return nil, errDressUnknown
	}
	a := &dressAsk{path: path, name: name, clip: clip, video: q.Get("on") == "video", typing: engine.CaptionShowing{Caption: -1, Word: -1},
		part: q.Get("part"), width: width, height: height, named: url.Values{"path": {path}}, key: q.Get("dressKey")}
	if q.Has("track") {
		a.track = q.Get("track")
		kept, ok := s.previews.tracks.get(q.Get("track"))
		if !ok || kept.episode != path || !episodeFile(s.store, path) {
			return nil, errDressUnknown
		}
		a.drawing = kept.drawing
	}
	if a.part != "" && a.part != engine.LiftWord && a.part != engine.LiftRest {
		return nil, fmt.Errorf("part is %q", a.part)
	}
	// The word being typed in the caption box, which libass leaves out.
	if q.Has("typing") {
		if n, err := fmt.Sscanf(q.Get("typing"), "%d %d", &a.typing.Caption, &a.typing.Word); err != nil || n != 2 {
			return nil, fmt.Errorf("typing is %q", q.Get("typing"))
		}
	}
	// The captions are drawn at the canvas's size, lw by lh, where the
	// window says it: frames are decoded no larger than the file, and a
	// layer drawn at their size was stretched over the canvas.
	if q.Has("lw") || q.Has("lh") {
		w, errW := strconv.Atoi(q.Get("lw"))
		h, errH := strconv.Atoi(q.Get("lh"))
		if errW != nil || errH != nil {
			return nil, fmt.Errorf("the canvas is %q by %q", q.Get("lw"), q.Get("lh"))
		}
		a.width, a.height = w, h
	}
	if !framewire.LayerFits(a.width, a.height) {
		return nil, errors.New("a dress needs the width and the height of the video preview")
	}
	drafts, err := captionDrafts(q.Get)
	if err != nil {
		return nil, err
	}
	a.drafts = drafts
	// The framing says where the crop stands and how the frame turns,
	// which the layer's size already holds, so a crop moved draws the
	// same captions, see layerTag.
	for _, k := range dressWords {
		if k != "framing" && q.Has(k) {
			a.named.Set(k, q.Get(k))
		}
	}
	a.named.Set("w", strconv.Itoa(a.width))
	a.named.Set("h", strconv.Itoa(a.height))
	return a, nil
}

// with is the ask with another framing and track, for the frame of a
// clip on a card, see serveAhead, or for the video, where a play goes on
// as a play of the video, see servePlayTake.
func (a *dressAsk) with(clip framed, drawing *engine.CaptionDrawing, track string, video bool) *dressAsk {
	b := *a
	b.clip, b.drawing, b.video = clip, drawing, video
	b.named = url.Values{}
	for k, v := range a.named {
		b.named[k] = v
	}
	b.named.Del("track")
	b.track = ""
	if drawing != nil {
		b.named.Set("track", track)
		b.track = track
	}
	b.named.Del("on")
	if video {
		b.named.Set("on", "video")
	}
	return &b
}

// sentDress is the dress of a frame as the window is sent it, see
// engine.FrameDress, with the key the window named its ask by, the track
// of its captions, and the layer of captions over it: its tag, which
// the layer is kept by and named again by when it is the one the window
// has, its size, the caption and the words it lights, see
// engine.CaptionShowing, how large the pill of that word is drawn on it,
// 1 at rest, see engine.PopScale, and for the word of a word step its
// bounce, see engine.BounceCurve. No tag where nothing lies over the frame.
type sentDress struct {
	engine.FrameDress
	Key     string      `json:"key,omitempty"`
	Track   string      `json:"track,omitempty"`
	Tag     string      `json:"tag,omitempty"`
	Size    [2]int      `json:"size,omitzero"`
	Showing [3]int      `json:"showing"`
	Scale   float64     `json:"scale,omitempty"`
	Bounce  *[3]float64 `json:"bounce,omitempty"`
	// Turned is the frame's dress once R turns the part that holds it,
	// see turnedDress.
	Turned *turnedDress `json:"turned,omitempty"`
	// Corner is what the crop frame's corner offers on the frame, the
	// crop brought to where the app places it or put back, or nothing,
	// see corner in cropback.go.
	Corner string `json:"corner,omitempty"`
}

// turnedDress is a frame's dress with its part turned a quarter further,
// and the framing that turns it so, which the clip has once R is pressed.
// The video preview turns the frame R is pressed on by it, in the frame
// it is pressed in, until the frame's own dress under that framing comes,
// which took two frames.
type turnedDress struct {
	Framing string `json:"framing"`
	engine.FrameDress
}

// turned is the dress of a frame of the clip whose framing the ask
// names, d, as R would turn it: the dress under the framing R turns the
// part that holds the frame to. Nil for a frame of a cut, or of the video.
func (s *FrameFairy) turned(a *dressAsk, d engine.FrameDress, at float64) *turnedDress {
	if a.video || !d.In || d.Piece < 0 || d.Piece >= len(a.clip.segments) {
		return nil
	}
	parts, ok := s.previews.turns.get(a.name)
	if !ok {
		parts = s.turnsFor(a.path, a.name, a.clip.source)
	}
	name := parts[a.clip.segments[d.Piece].Part]
	if name == "" {
		return nil
	}
	g, ok := s.previews.framings.get(name)
	if !ok || g.episode != a.clip.episode {
		return nil
	}
	return &turnedDress{Framing: name, FrameDress: g.dress(at, false)}
}

// clipRef is the clip of a plan a framing of the clip list is of.
type clipRef struct{ plan, id string }

// turnsFor works out the framings R turns each part of the clip whose
// framing is named name to, and keeps them by it, for a clip whose frame
// was dressed before a state said them: a clip whose card was read ahead
// and then clicked, or the answer of an edit. Kept empty where there are
// none to be had, so a play of a clip asks the plan once, not each frame.
func (s *FrameFairy) turnsFor(path, name string, info engine.SourceInfo) map[int]string {
	none := map[int]string{}
	ref, ok := s.previews.clips.get(name)
	if !ok {
		return none
	}
	view, err := s.store.machine.Episode(path).ReadPlan(ref.plan)
	i := -1
	if err == nil {
		i = slices.IndexFunc(view.Clips, func(c engine.ClipView) bool { return c.ID == ref.id })
	}
	var entry ClipEntry
	if i >= 0 {
		entry = s.entryOf(path, ref.plan, view.Clips[i], info, nil)
	}
	if entry.Framing != name {
		s.previews.turns.put(name, none, nil)
		return none
	}
	s.turnsOf(path, s.plansOf(path), entry.Key, []ClipEntry{entry}, info)
	if parts, ok := s.previews.turns.get(name); ok {
		return parts
	}
	return none
}

// dressed is a frame dressed: its dress, and the layer of captions over
// it as framewire.Layer writes it, its head and its pixels, or nothing
// where the window has it by its tag or nothing lies over the frame.
type dressed struct {
	sent  sentDress
	layer [][]byte
}

// dressFrame dresses the frame of the episode that starts at a moment, as
// the ask says: its dress, and with a track, over a frame of the clip,
// the captions libass draws there in its crop, see layerSize, unless they
// are the layer have names. The engine's caption code runs outside a job,
// so a panic in it fails the request or the stream, not the app.
func (s *FrameFairy) dressFrame(ctx context.Context, a *dressAsk, at float64, have string) (dressed, error) {
	d := a.clip.dress(at, a.video)
	out := dressed{sent: sentDress{FrameDress: d, Key: a.key, Track: a.track, Showing: [3]int{-1, -1, -1}, Turned: s.turned(a, d, at), Corner: s.corner(a, d, at)}}
	if a.drawing == nil || !d.Clip {
		return out, nil
	}
	// A layer the wire to the decoder cannot carry is the ask's fault, and
	// is refused before the decoder is asked for it.
	layerW, layerH := layerSize(d, a.width, a.height)
	if !framewire.LayerFits(layerW, layerH) {
		return out, errors.New("the captions of a frame this large cannot be drawn")
	}
	source, err := s.probe(ctx, a.path)
	if err != nil {
		return out, stepFailed(ctx, "probing the video", err)
	}
	st := s.store
	e := engine.NewEngine(engineLog("captions", a.path), st.machine)
	rs := jobOptions(st, e).RenderSettings()
	drawing := a.drawing.Typing(a.typing).Lifted(a.part)
	showing := a.drawing.Showing(source, at)
	out.sent.Tag = layerTag(a.named, drawing, source, rs, at, layerW, layerH)
	out.sent.Size = [2]int{layerW, layerH}
	out.sent.Showing = [3]int{showing.Caption, showing.Word, showing.Last}
	if a.part == engine.LiftWord {
		length, rise, peak := engine.BounceCurve()
		out.sent.Bounce = &[3]float64{length, rise, peak}
	}
	// A frame drawn the same as the layer the window has is sent without
	// it, drawn or sent again. Between two words of a playing clip that
	// was every other frame, libass drawing the same layer over and over,
	// issue #896.
	if have == out.sent.Tag {
		s.previews.layers.unchanged(a.path)
		out.sent.Scale = 1
		return out, nil
	}
	// A frame whose layer is not drawn names none, or the frames after it
	// would be sent with the tag alone, and come bare too.
	dec, err := s.previews.decoder(st.machine, a.path)
	if err != nil {
		out.sent.Tag = ""
		return out, err
	}
	// A video whose folder was deleted outside the app is let go as the
	// app hears of it, see reconcile, and until then its captions write
	// nothing into a folder that is gone, which would bring the folder
	// back. The engine checks again as it writes, see
	// engine.ErrNoWorkFolder.
	if ok, _ := marked(st.files, a.path); !ok {
		out.sent.Tag = ""
		return out, engine.ErrNoWorkFolder
	}
	l, err := e.PreviewCaptions(ctx, dec, engine.WorkDir(a.path), drawing, a.drafts, source, rs, at, layerW, layerH)
	if err != nil {
		out.sent.Tag = ""
		return out, stepFailed(ctx, "drawing the captions", err)
	}
	// A layer slower than a frame is a pill behind the sound while the
	// clip plays, and on a customer's machine the log is the only place
	// that shows it. Counted, and said once the clip stops asking, see
	// layerTally.
	if source.FPSNum > 0 {
		s.previews.layers.add(a.path, l.Drew, time.Duration(float64(time.Second)*float64(source.FPSDen)/float64(source.FPSNum)))
	}
	// A part is drawn at rest.
	out.sent.Scale = 1
	if a.part == "" {
		out.sent.Scale = l.Scale
	}
	out.layer = [][]byte{l.Head(), l.Pixels}
	return out, nil
}

// stepFailed is err, saying which step of dressing a frame it came in,
// and when the time of the request ran out, that it did, so the answer
// and the app's log say what took the time. Four asks among many once
// answered only "context deadline exceeded", and nothing said whether
// the probe, the caption file or the decoder held them, issue #687.
func stepFailed(within context.Context, step string, err error) error {
	if within.Err() != nil {
		return fmt.Errorf("%s took more than the %s a request for captions may take: %w", step, captionsWait, err)
	}
	return fmt.Errorf("%s: %w", step, err)
}

// writeDress writes what a frame is dressed with after the frame: its
// dress as JSON and then its layer of captions, each after its length in
// 4 bytes, little endian. A frame with no dress has two lengths of 0.
// The layer's pixels are written as they came from the decoder, so a
// layer is not copied once more to be sent.
func writeDress(w io.Writer, d *dressed) error {
	var head []byte
	if d != nil {
		head, _ = json.Marshal(d.sent)
	}
	size := make([]byte, 4)
	binary.LittleEndian.PutUint32(size, uint32(len(head)))
	if _, err := w.Write(size); err != nil {
		return err
	}
	if _, err := w.Write(head); err != nil {
		return err
	}
	n := 0
	if d != nil {
		for _, part := range d.layer {
			n += len(part)
		}
	}
	binary.LittleEndian.PutUint32(size, uint32(n))
	if _, err := w.Write(size); err != nil {
		return err
	}
	if d == nil {
		return nil
	}
	for _, part := range d.layer {
		if _, err := w.Write(part); err != nil {
			return err
		}
	}
	return nil
}

// dresser dresses the frames of one answer, or of one play's pictures,
// in turn: a layer of captions sent with one frame is not sent again
// with the next, and one the window named it has is not sent at all. The
// frames of a play are dressed as each change of the clip it took in
// asks, see servePlayTake, and those of a card as its clip asks, see
// serveAhead. The zero value dresses nothing.
type dresser struct {
	s  *FrameFairy
	mu sync.Mutex
	// asks are what the frames are dressed with, by how many changes the
	// play had taken in when they were made, 0 for an answer's frames.
	asks map[int]*dressAsk
	// cards are what the frames of the cards are dressed with, by frame.
	cards map[int]*dressAsk
	last  string
	// told is the last failure said in the log, said once.
	told string
}

func newDresser(s *FrameFairy, ask *dressAsk, have string) *dresser {
	return &dresser{s: s, asks: map[int]*dressAsk{0: ask}, last: have}
}

// dress dresses a frame of the grid that starts at a moment, made after
// gen changes of the clip, or nil where nothing dresses it. A layer that
// fails leaves the frame with its dress alone, and the log says why.
func (d *dresser) dress(within context.Context, gen, frame int, at float64) *dressed {
	if d == nil {
		return nil
	}
	// The ask is read under the lock and the layer drawn outside it, so a
	// change of the clip a play takes in never waits for libass, see
	// take. The frames of one answer are dressed one after another, so
	// the tag sent last is theirs alone.
	d.mu.Lock()
	a := d.cards[frame]
	if a == nil {
		for g := gen; g >= 0 && a == nil; g-- {
			a = d.asks[g]
		}
	}
	last := d.last
	d.mu.Unlock()
	if a == nil {
		return nil
	}
	ctx, cancel := d.s.previews.captionsWithin(within)
	defer cancel()
	out, err := d.s.dressFrame(ctx, a, at, last)
	d.mu.Lock()
	defer d.mu.Unlock()
	if err != nil && within.Err() == nil && err.Error() != d.told {
		d.told = err.Error()
		theLog.line(zerolog.WarnLevel, "captions").Str("video", a.path).Float64("at", at).Err(err).Msg("a frame went out without its captions")
	}
	if out.sent.Tag != "" {
		d.last = out.sent.Tag
	}
	return &out
}

// take runs a change of the clip a play takes in, and has the frames
// made after it dressed as the change asks, see servePlayTake. Run under
// the dresser's lock, so no frame of the change is dressed before its
// ask is here.
func (d *dresser) take(run func() (gen int, ask *dressAsk, ok bool)) bool {
	d.mu.Lock()
	defer d.mu.Unlock()
	gen, ask, ok := run()
	if ok {
		d.asks[gen] = ask
	}
	return ok
}

// redress has every frame from now on dressed as ask says: the clip
// changed nothing a play plays, its crop or its captions only.
func (d *dresser) redress(ask *dressAsk) {
	d.mu.Lock()
	defer d.mu.Unlock()
	for g := range d.asks {
		d.asks[g] = ask
	}
}
