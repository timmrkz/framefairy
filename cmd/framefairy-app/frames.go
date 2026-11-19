package main

import (
	"context"
	"crypto/rand"
	"encoding/binary"
	"encoding/hex"
	"errors"
	"fmt"
	"math"
	"net/http"
	"net/url"
	"os"
	"path/filepath"
	"runtime/debug"
	"slices"
	"strconv"
	"strings"
	"sync"
	"time"

	"github.com/rs/zerolog"

	"framefairy/engine"
	"framefairy/engine/runes"
	"framefairy/internal/framewire"
)

// The picture of the episode, decoded on the Go side by the episode's
// decoder, see engine.EpisodeFrames. The page opens a stream at a moment, pulls its
// frames a few at a time and closes it. ffmpeg decodes only as far as the
// page has pulled, a frame or two ahead: WebKit takes whatever a response
// writes without ever pushing back, so one long response would have let
// ffmpeg decode the whole episode into memory while the page stood paused.
// A stream left open is how a paused play goes on at once, and how the
// next frame is there when the arrow key steps to it.
//
//	/frames/look?path=<episode>&frame=<frame>&w=<width>&h=<height>&grid=<grid>&shown=<frame>&win=<window>&n=<sent>[&move=1][&follow=1]
//	                                 the frame for where the hand is, see
//	                                 serveLook and engine.Schedule
//	/frames/near?path=<episode>&frame=<frame>&w=<width>&h=<height>&grid=<grid>&has=<frame>,...
//	                                 the frames kept around a place the
//	                                 window does not hold, see serveNear
//	/frames/ahead?path=<episode>&w=<width>&h=<height>&grid=<grid>&frames=<frame>,...&cards=<frame>,...&has=<frame>,...&win=<window>&n=<sent>[&held=1]
//	                                 what to read while the hand rests,
//	                                 answered by the frames read that the
//	                                 window does not hold, see serveAhead
//	/frames/open?path=<episode>&from=<seconds>&w=<width>&h=<height>&grid=<grid>[&key=1]  {"id": "..."}
//	                                 the frames of the grid from the one that
//	                                 holds from: the grid the engine named,
//	                                 see engine.SourceInfo.Grid, as the page
//	                                 hands it back, see framewire.GridText
//	/frames/sound?path=<episode>&from=<seconds>&rate=<hz>&ch=<channels>  {"id": "..."}
//	/frames/play?path=<episode>&w=<width>&h=<height>&grid=<grid>&rate=<hz>&ch=<channels>&pieces=<start>:<end>,...&at=<seconds>[&loop=1]
//	                                 a play of a clip, its pictures and its
//	                                 sound as two streams, see servePlay
//	/frames/play/go?id=<pictures>
//	/frames/play/loop?id=<pictures>&at=<seconds>[&loop=1]
//	/frames/read?id=<id>&n=<frames>&skip=<seconds>  frames, see writeFrames
//	/frames/close?id=<id>
//	/frames/levels?path=<episode>  the loudness, see serveLoudness
//
// The sound of every episode comes the same way, from the episode's
// decoder, see engine.EpisodeFrames.Sound, read by the rule the render and
// the transcription read it by: a stream of it is read like a stream of
// frames, each frame engine.SoundChunk moments of it.
//
// previews are the streams of the video preview, one set for the app,
// which FrameFairy keeps, and the captions drawn over them. Its zero value
// is ready. The episode's decoder is found by the finder of the app's
// machine, so it is checked once for the program, as ffmpeg is.
type previews struct {
	mu   sync.Mutex
	open map[string]*preview
	// The episode's decoder of each episode a stream was asked of, see
	// engine.EpisodeFrames. One whose episode is open in the video preview
	// is held, by how many times, and is never closed for standing unused:
	// a click after an hour paused finds it as ready as the first. One
	// nobody holds is closed once it has stood unused.
	decoders map[string]*engine.EpisodeFrames
	held     map[string]int
	// The schedule of each decoder, which decides the frames of the
	// video preview while it stands still, see serveLook.
	schedules map[*engine.EpisodeFrames]*engine.Schedule
	// Started with the first stream, it closes the ones nobody pulls.
	reaping bool
	// tracks are the caption views the window was given, and layers
	// counts the caption layers drawn, see captions.go.
	tracks tracks
	layers layerTally
	// within gives the context a request for captions runs in, see
	// captionsWithin. Nil in the app. A test hands in its own, to end a
	// request's time at once, never by the clock.
	within func(context.Context) (context.Context, context.CancelFunc)
}

// A stream that nobody has pulled from for this long is closed, and at
// most this many of a kind are open at once, the least recently pulled
// closed first. A play has one of frames and one of sound, and a play
// taking over from it two more for a moment.
const (
	previewIdle = 20 * time.Second
	previewMost = 6
)

type previewFrame struct {
	at float64
	// stamp is what is written after at, where the stream has more to
	// say of each frame, see servePlay.
	stamp []float64
	data  []byte
}

type preview struct {
	// "play" for a play's frames, "playsound" for its sound, see
	// servePlay. At most previewMost of a kind are open.
	kind   string
	frames chan previewFrame
	stop   context.CancelFunc
	// When it was last pulled from, as unix nanoseconds.
	pulled atomicTime
	// Closed when ffmpeg has stopped, with why in err.
	done chan struct{}
	err  error
	// How the frames code their brightness, said with every pull that has
	// a frame, see framewire.Picture. Nil for sound.
	light func() framewire.Light
	// Takes back a frame once nothing reads it, see start.
	give func([]byte)
	// The play whose frames or sound it is, see servePlay.
	play *engine.Play
}

type atomicTime struct {
	mu sync.Mutex
	t  time.Time
}

func (a *atomicTime) set(t time.Time) {
	a.mu.Lock()
	a.t = t
	a.mu.Unlock()
}

func (a *atomicTime) get() time.Time {
	a.mu.Lock()
	defer a.mu.Unlock()
	return a.t
}

// serveFrames answers the requests for the video preview's streams, its
// captions and its decoder.
func (s *FrameFairy) serveFrames(w http.ResponseWriter, r *http.Request) {
	st, p := s.store, &s.previews
	q := r.URL.Query()
	switch r.URL.Path {
	case "/frames/play":
		p.servePlay(st, w, r)
	case "/frames/play/go":
		p.servePlayGo(w, r)
	case "/frames/play/loop":
		p.servePlayLoop(w, r)
	case "/frames/look":
		p.serveLook(st, w, r)
	case "/frames/near":
		p.serveNear(st, w, r)
	case "/frames/ahead":
		p.serveAhead(st, w, r)
	case "/frames/read":
		stream := p.get(q.Get("id"))
		if stream == nil {
			http.NotFound(w, r)
			return
		}
		n, _ := strconv.Atoi(q.Get("n"))
		writeFrames(w, r, stream, min(max(n, 1), 16))
	case "/frames/captions":
		s.serveCaptions(w, r)
	case "/frames/captions/words":
		s.serveWordFrames(w, r)
	case "/frames/levels":
		s.serveLoudness(w, r)
	case "/frames/close":
		p.close(q.Get("id"))
		w.WriteHeader(http.StatusNoContent)
	case "/frames/hold", "/frames/release":
		path := q.Get("path")
		if !episodeFile(st, path) {
			http.NotFound(w, r)
			return
		}
		if r.URL.Path == "/frames/release" {
			p.release(path)
			w.WriteHeader(http.StatusNoContent)
			return
		}
		if err := p.hold(st.machine, path); err != nil {
			http.Error(w, err.Error(), http.StatusInternalServerError)
			return
		}
		w.WriteHeader(http.StatusNoContent)
	default:
		http.NotFound(w, r)
	}
}

// serveLook answers where the hand is on a paused video preview with a
// frame, see engine.Schedule: frame is the frame of the grid under the
// hand, move says the hand moves over it, and shown is the frame on
// screen, or -1. The answer is the frame as writeFrames writes one, with
// X-Frames-Exact where it is the frame of the place and not one on the
// way, X-Frames-Kept where nothing was decoded for it, and X-Frames-Near
// where frames around it are worth asking for, see serveNear. A look a newer one took the place of is answered with no content,
// and one the decoder failed with X-Frames-Error and no frame.
func (p *previews) serveLook(st *store, w http.ResponseWriter, r *http.Request) {
	q := r.URL.Query()
	path, hand, ok := handArgs(st, w, r, true)
	if !ok {
		return
	}
	shown, err := strconv.Atoi(q.Get("shown"))
	if err != nil || shown < 0 || shown >= hand.Grid.Count {
		shown = -1
	}
	hand.Shown, hand.Moving = shown, q.Get("move") == "1"
	frame := hand.Frame
	dec, err := p.decoder(st.machine, path)
	if err != nil {
		w.Header().Set("X-Frames-Error", runes.Scrub(err.Error(), 300))
		w.WriteHeader(http.StatusOK)
		return
	}
	seen, err := p.schedule(dec).Look(r.Context(), hand)
	switch {
	case errors.Is(err, engine.ErrOvertaken):
		w.WriteHeader(http.StatusNoContent)
		return
	case r.Context().Err() != nil:
		return
	case err != nil:
		if !errors.Is(err, engine.ErrFramesClosed) {
			theLog.line(zerolog.WarnLevel, "files").Str("video", filepath.Base(path)).Int("frame", frame).Err(err).Msg("a look failed")
		}
		w.Header().Set("X-Frames-Error", runes.Scrub(err.Error(), 300))
		w.WriteHeader(http.StatusOK)
		return
	}
	w.Header().Set("Content-Type", "application/octet-stream")
	w.Header().Set("Cache-Control", "no-store")
	if seen.Exact {
		w.Header().Set("X-Frames-Exact", "1")
	}
	if seen.Kept {
		w.Header().Set("X-Frames-Kept", "1")
	}
	if seen.Exact && p.schedule(dec).HasNear(hand) {
		w.Header().Set("X-Frames-Near", "1")
	}
	writeSeen(w, dec, []engine.Seen{seen})
}

// writeSeen writes frames a schedule sent, each as writeFrames writes one.
func writeSeen(w http.ResponseWriter, dec *engine.EpisodeFrames, seen []engine.Seen) {
	if light := dec.Light(); light != framewire.SDR {
		w.Header().Set("X-Frames-Light", string(light))
	}
	head := make([]byte, 8)
	for _, f := range seen {
		binary.LittleEndian.PutUint64(head, math.Float64bits(f.At))
		if _, err := w.Write(head); err != nil {
			return
		}
		if _, err := w.Write(f.Picture); err != nil {
			return
		}
	}
}

// handArgs reads the episode, the size, the grid and, with frame, the
// frame of a request about the hand, and answers a request that names a
// file outside the library, or a size, grid or frame it cannot have.
func handArgs(st *store, w http.ResponseWriter, r *http.Request, frame bool) (string, engine.Hand, bool) {
	q := r.URL.Query()
	path := q.Get("path")
	if !episodeFile(st, path) {
		http.NotFound(w, r)
		return "", engine.Hand{}, false
	}
	width, _ := strconv.Atoi(q.Get("w"))
	height, _ := strconv.Atoi(q.Get("h"))
	if width < 2 || height < 2 || width > 7680 || height > 4320 {
		http.Error(w, "frames need a width and a height", http.StatusBadRequest)
		return "", engine.Hand{}, false
	}
	grid, ok := framewire.ReadGrid(q.Get("grid"))
	if !ok {
		http.Error(w, "frames need the grid they count on", http.StatusBadRequest)
		return "", engine.Hand{}, false
	}
	hand := engine.Hand{Grid: grid, W: width &^ 1, H: height &^ 1, Shown: -1, Sent: sentOf(q)}
	if frame {
		f, err := strconv.Atoi(q.Get("frame"))
		if err != nil || f < 0 || f >= grid.Count {
			http.Error(w, "a look needs a frame of the grid", http.StatusBadRequest)
			return "", engine.Hand{}, false
		}
		hand.Frame = f
	}
	return path, hand, true
}

// sentOf reads where a request stands among what its window sent, see
// engine.Sent: the window's name, at most 64 characters, and how many it
// had sent. A request without them stands nowhere.
func sentOf(q url.Values) engine.Sent {
	win := q.Get("win")
	n, err := strconv.ParseUint(q.Get("n"), 10, 64)
	if win == "" || len(win) > 64 || err != nil {
		return engine.Sent{}
	}
	return engine.Sent{Window: win, N: n}
}

// frameList reads a list of frames of the grid, "1,2,3", leaving out what
// is no frame of it, and no more than most.
func frameList(text string, grid framewire.Grid, most int) []int {
	var out []int
	for f := range strings.SplitSeq(text, ",") {
		n, err := strconv.Atoi(f)
		if err != nil || n < 0 || n >= grid.Count {
			continue
		}
		if out = append(out, n); len(out) == most {
			break
		}
	}
	return out
}

// serveNear answers with the frames kept around a place the hand rests on
// that the window does not hold, has, once the decoder has read them,
// each as writeFrames writes one. See engine.Schedule.Near.
func (p *previews) serveNear(st *store, w http.ResponseWriter, r *http.Request) {
	path, hand, ok := handArgs(st, w, r, true)
	if !ok {
		return
	}
	dec, err := p.decoder(st.machine, path)
	if err != nil {
		w.WriteHeader(http.StatusNoContent)
		return
	}
	has := frameList(r.URL.Query().Get("has"), hand.Grid, 64)
	near := p.schedule(dec).Near(r.Context(), hand, func(f int) bool { return slices.Contains(has, f) })
	if r.Context().Err() != nil {
		return
	}
	w.Header().Set("Content-Type", "application/octet-stream")
	w.Header().Set("Cache-Control", "no-store")
	writeSeen(w, dec, near)
}

// serveAhead hands the schedule what to read while the hand rests, see
// engine.Ahead, and answers once it is read with the frames the window
// does not hold, each as writeFrames writes one, with no content when a
// newer ask took its place, and with X-Frames-Error when the decoder
// could not be opened.
func (p *previews) serveAhead(st *store, w http.ResponseWriter, r *http.Request) {
	path, hand, ok := handArgs(st, w, r, false)
	if !ok {
		return
	}
	q := r.URL.Query()
	dec, err := p.decoder(st.machine, path)
	if err != nil {
		w.Header().Set("X-Frames-Error", runes.Scrub(err.Error(), 300))
		w.WriteHeader(http.StatusOK)
		return
	}
	seen, err := p.schedule(dec).Ahead(r.Context(), engine.Ahead{Grid: hand.Grid, W: hand.W, H: hand.H,
		Frames: frameList(q.Get("frames"), hand.Grid, 16), Cards: frameList(q.Get("cards"), hand.Grid, 256),
		Has: frameList(q.Get("has"), hand.Grid, 512), Held: q.Get("held") == "1", Sent: hand.Sent})
	switch {
	case errors.Is(err, engine.ErrOvertaken):
		w.WriteHeader(http.StatusNoContent)
	case r.Context().Err() != nil:
	case err != nil:
		w.Header().Set("X-Frames-Error", runes.Scrub(err.Error(), 300))
		w.WriteHeader(http.StatusOK)
	default:
		writeSeen(w, dec, seen)
	}
}

// schedule is the schedule of the decoder, made with its first look.
func (p *previews) schedule(dec *engine.EpisodeFrames) *engine.Schedule {
	p.mu.Lock()
	defer p.mu.Unlock()
	if p.schedules == nil {
		p.schedules = map[*engine.EpisodeFrames]*engine.Schedule{}
	}
	s := p.schedules[dec]
	if s == nil {
		s = engine.NewSchedule(dec)
		p.schedules[dec] = s
	}
	return s
}

// closeDecoder closes a decoder and its schedule, on a goroutine of its
// own, since a decoder takes a moment to stop. Under the lock.
func (p *previews) closeDecoder(d *engine.EpisodeFrames) {
	s := p.schedules[d]
	delete(p.schedules, d)
	go func() {
		if s != nil {
			s.Close()
		}
		d.Close()
	}()
}

// start runs ffmpeg for a stream. It decodes a frame and waits until the
// page has pulled it before it decodes the next, so the channel holds one.
// Streams of sound and of frames are counted apart, so that seeks of a
// play, each with a sound stream of its own, never close the frames the
// video preview is drawing, nor frames the sound. give, where it is not
// nil, takes back a frame once it is written to the page or dropped, for
// the decoder to read a later one into, see engine.EpisodeFrames.Give.
// startStamped starts a stream, of a play's frames or of its sound, see
// servePlay. Each frame says more of itself than where it starts, see
// previewFrame.stamp.
func (p *previews) startStamped(kind string, light func() framewire.Light,
	give func([]byte), run func(ctx context.Context, got func(previewFrame) error) error) (string, *preview) {
	var raw [16]byte
	_, _ = rand.Read(raw[:])
	id := hex.EncodeToString(raw[:])
	ctx, stop := context.WithCancel(context.Background())
	if give == nil {
		give = func([]byte) {}
	}
	s := &preview{kind: kind, frames: make(chan previewFrame), stop: stop, done: make(chan struct{}), light: light, give: give}
	s.pulled.set(time.Now())
	p.mu.Lock()
	if p.open == nil {
		p.open = map[string]*preview{}
	}
	for {
		oldest, n := "", 0
		for k, o := range p.open {
			if o.kind != kind {
				continue
			}
			n++
			if oldest == "" || o.pulled.get().Before(p.open[oldest].pulled.get()) {
				oldest = k
			}
		}
		if n < previewMost {
			break
		}
		theLog.line(zerolog.DebugLevel, "files").Str("stream", oldest[:6]).Int("open", n).Msg("the stream is closed to make room")
		p.open[oldest].stop()
		delete(p.open, oldest)
	}
	p.open[id] = s
	if !p.reaping {
		p.reaping = true
		go p.reap()
	}
	p.mu.Unlock()
	go func() {
		defer close(s.done)
		err := streamSafely(func() error {
			return run(ctx, func(f previewFrame) error {
				// A frame or a chunk of sound is handed over in a buffer of
				// its own, see engine.EpisodeFrames.Stream, and kept as it
				// is until it is written to the page, see writeFrames, or
				// dropped here.
				select {
				case s.frames <- f:
					return nil
				case <-ctx.Done():
					s.give(f.data)
					return ctx.Err()
				}
			})
		})
		// Stopped from here, whatever ffmpeg said as it was killed, or the
		// episode's decoder closed under it: the page asks another stream.
		if ctx.Err() != nil || errors.Is(err, engine.ErrFramesClosed) {
			err = context.Canceled
		} else if err != nil {
			theLog.line(zerolog.WarnLevel, "files").Str("stream", id[:6]).Err(err).Msg("the stream failed")
		}
		s.err = err
	}()
	return id, s
}

// streamSafely runs a stream and turns a panic into the stream's error.
// A stream reads a video nobody has checked, on a goroutine of its own
// that outlives the request that started it, so the recover around that
// request, see mediaMiddleware, does not reach it, and a panic in it
// ended the whole app, issue #604. The page asks another stream, as it does for one that
// failed.
func streamSafely(run func() error) (err error) {
	defer func() {
		if caught := recover(); caught != nil {
			theLog.line(zerolog.ErrorLevel, "files").Str("stack", string(debug.Stack())).Msgf("the stream stopped unexpectedly: %v", caught)
			err = fmt.Errorf("the stream stopped unexpectedly: %v", caught)
		}
	}()
	return run()
}

func (p *previews) get(id string) *preview {
	p.mu.Lock()
	defer p.mu.Unlock()
	s := p.open[id]
	if s != nil {
		s.pulled.set(time.Now())
	}
	return s
}

func (p *previews) close(id string) {
	p.mu.Lock()
	s := p.open[id]
	delete(p.open, id)
	p.mu.Unlock()
	if s != nil {
		s.stop()
	}
}

// episodeFile says whether path is an episode of the library, a file.
func episodeFile(st *store, path string) bool {
	if path == "" || !filepath.IsAbs(path) || !st.Known(path) {
		return false
	}
	info, err := os.Stat(path)
	return err == nil && info.Mode().IsRegular()
}

// hold starts the episode's decoder of the episode at path, with its
// cursors opened, as the episode opens in the video preview, and keeps it
// until release. The frame queue holds it for as long as it has the
// episode, see AppFrames in lib/frames/app.ts.
func (p *previews) hold(m *engine.Machine, path string) error {
	d, err := p.decoder(m, path)
	if err != nil {
		return err
	}
	p.mu.Lock()
	if p.held == nil {
		p.held = map[string]int{}
	}
	p.held[path]++
	held := p.held[path]
	p.mu.Unlock()
	err = d.Ready()
	level := zerolog.DebugLevel
	if err != nil {
		level = zerolog.WarnLevel
	}
	theLog.line(level, "files").Str("video", filepath.Base(path)).Int("held", held).Err(err).Msg("the decoder is held")
	return err
}

// release lets go of a hold. The decoder is then closed once it has stood
// unused, see reap, or at once when more than decodersSpare stand unheld.
func (p *previews) release(path string) {
	p.mu.Lock()
	defer p.mu.Unlock()
	if p.held[path] > 1 {
		p.held[path]--
	} else {
		delete(p.held, path)
		// Its workspace is asleep, so no play is coming: the decoder
		// keeps running for a while, the frames it kept need not.
		if d := p.decoders[path]; d != nil {
			d.LetFramesGo()
		}
	}
	theLog.line(zerolog.DebugLevel, "files").Str("video", filepath.Base(path)).Int("held", p.held[path]).Msg("the decoder is let go")
	p.spare()
}

// decodersSpare is how many decoders nobody holds are kept running for an
// episode that comes back. The app keeps ten workspaces and builds them as
// it starts, each holding its decoder until it is asleep, so without a
// bound ten programs stood running for a minute after the app started.
const decodersSpare = 2

// spare closes the decoders nobody holds beyond decodersSpare, the one
// unused longest first. Under the lock.
func (p *previews) spare() {
	for {
		oldest, longest, free := "", time.Duration(-1), 0
		for path, d := range p.decoders {
			if p.held[path] > 0 {
				continue
			}
			free++
			if idle := d.Idle(); idle > longest {
				oldest, longest = path, idle
			}
		}
		if free <= decodersSpare {
			return
		}
		d := p.decoders[oldest]
		delete(p.decoders, oldest)
		theLog.line(zerolog.DebugLevel, "files").Str("video", filepath.Base(oldest)).Int("unheld", free).Msg("the decoder is closed to make room")
		p.closeDecoder(d)
	}
}

// decoder is the episode's decoder for the episode at path, started by a
// hold or on its first stream. It ships beside the app, as ffmpeg does, and an app
// without it says so the way it says a missing ffmpeg: there is no second
// way to the frames. The finder of machine m finds it, and it decodes
// on m, see engine.Media.
func (p *previews) decoder(m *engine.Machine, path string) (*engine.EpisodeFrames, error) {
	program, err := m.Tools.Find("FRAMEFAIRY_FRAMES", "framefairy-frames")
	if err != nil {
		return nil, err
	}
	p.mu.Lock()
	defer p.mu.Unlock()
	if p.decoders == nil {
		p.decoders = map[string]*engine.EpisodeFrames{}
	}
	d := p.decoders[path]
	if d == nil {
		// On the store's machine, which appStore hands the system's media
		// engine, so the app's decoder decodes on the graphics chip and a
		// test's on the processor.
		d = m.EpisodeFrames(program, path)
		p.decoders[path] = d
		if !p.reaping {
			p.reaping = true
			go p.reap()
		}
	}
	return d, nil
}

func (p *previews) reap() {
	for {
		time.Sleep(previewIdle / 4)
		if !p.reapOnce(previewIdle) {
			return
		}
	}
}

// reapOnce closes the streams nobody has pulled for idle, and the decoders
// nobody holds that have stood unused for three times that. It says
// whether any is left. Once none is, the reaping ends, and the next
// stream or decoder starts it again, so a set of streams the app or a
// test is done with leaves nothing running.
func (p *previews) reapOnce(idle time.Duration) bool {
	p.mu.Lock()
	defer p.mu.Unlock()
	for path, d := range p.decoders {
		if p.held[path] == 0 && d.Idle() > 3*idle {
			delete(p.decoders, path)
			theLog.line(zerolog.DebugLevel, "files").Str("video", filepath.Base(path)).Msgf("the decoder is closed, unused for %.0f s", d.Idle().Seconds())
			p.closeDecoder(d)
		}
	}
	for id, s := range p.open {
		if time.Since(s.pulled.get()) > idle {
			theLog.line(zerolog.DebugLevel, "files").Str("stream", id[:6]).Msgf("the stream is closed, not pulled for %.0f s", time.Since(s.pulled.get()).Seconds())
			s.stop()
			delete(p.open, id)
		}
	}
	if len(p.decoders) == 0 && len(p.open) == 0 {
		p.reaping = false
		return false
	}
	return true
}

// writeFrames answers a pull: the next frame once ffmpeg has it, and up to
// n in all, as many as are there by then without waiting for more. Each is
// the moment of the episode it starts at, 8 bytes, a float64 in little
// endian, then the frame in colours, RGBA with 8 bits each and an opaque alpha, or the
// moments of sound, from the
// first that starts at skip or later. Every frame of a stream is as long
// as every other, but the last of a sound stream, which may be shorter. A stream that has ended answers
// with no frame and X-Frames-End, and with the reason when it failed.
func writeFrames(w http.ResponseWriter, r *http.Request, s *preview, n int) {
	w.Header().Set("Content-Type", "application/octet-stream")
	w.Header().Set("Cache-Control", "no-store")
	head := make([]byte, 8)
	// A frame is given back once it is written, or could not be: the
	// window's end of the answer copies what it is given before Write
	// returns, so nothing reads the frame after. Wails does so in Write of
	// internal/assetserver/webview/responsewriter_darwin_shared.go, with
	// NSData's dataWithBytes, and its Linux and Windows writers copy too.
	// No test can see it, so an update of Wails checks it, see Keeping up
	// in docs/BUILD.md.
	put := func(f previewFrame) bool {
		defer s.give(f.data)
		binary.LittleEndian.PutUint64(head, math.Float64bits(f.at))
		if _, err := w.Write(head); err != nil {
			return false
		}
		for _, v := range f.stamp {
			binary.LittleEndian.PutUint64(head, math.Float64bits(v))
			if _, err := w.Write(head); err != nil {
				return false
			}
		}
		_, err := w.Write(f.data)
		return err == nil
	}
	for waiting := true; waiting; {
		select {
		case f := <-s.frames:
			// HDR frames are 10 bits a colour in the file's own curve, and
			// the page draws them as such, see framewire.Picture.
			if s.light != nil {
				if light := s.light(); light != framewire.SDR {
					w.Header().Set("X-Frames-Light", string(light))
				}
			}
			if !put(f) {
				return
			}
			waiting = false
		case <-s.done:
			w.Header().Set("X-Frames-End", "1")
			if errors.Is(s.err, context.Canceled) {
				// Stopped here, to make room or after standing unused, not
				// at the end of the episode: the page asks another stream.
				w.Header().Set("X-Frames-Closed", "1")
			} else if s.err != nil {
				w.Header().Set("X-Frames-Error", runes.Scrub(s.err.Error(), 300))
			}
			w.WriteHeader(http.StatusOK)
			return
		case <-r.Context().Done():
			return
		}
	}
	// The rest of what was asked for, as long as ffmpeg keeps up: a frame
	// it is still decoding is waited for a moment, since the page asked
	// for it, and not longer.
	for i := 1; i < n; i++ {
		select {
		case f := <-s.frames:
			if !put(f) {
				return
			}
		case <-time.After(15 * time.Millisecond):
			return
		case <-s.done:
			return
		case <-r.Context().Done():
			return
		}
	}
}
