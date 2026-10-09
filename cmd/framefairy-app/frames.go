package main

import (
	"context"
	"crypto/rand"
	"encoding/binary"
	"encoding/hex"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"math"
	"net/http"
	"os"
	"path/filepath"
	"strconv"
	"sync"
	"sync/atomic"
	"time"

	"framefairy/engine"
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
//	/frames/open?path=<episode>&from=<seconds>&w=<width>&h=<height>  {"id": "..."}
//	/frames/sound?path=<episode>&from=<seconds>&rate=<hz>&ch=<channels>  {"id": "..."}
//	/frames/read?id=<id>&n=<frames>&skip=<seconds>  frames, see writeFrames
//	/frames/close?id=<id>
//
// The sound of every episode comes the same way, from the episode's
// decoder, see engine.EpisodeFrames.Sound, read by the rule the render and
// the transcription read it by: a stream of it is read like a stream of
// frames, each frame engine.SoundChunk moments of it.
//
// openPreviews are the streams of the app, one set however many times the
// handler is made.
var openPreviews = &previews{}

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
	// Started with the first stream, it closes the ones nobody pulls.
	reaping bool
}

// A stream that nobody has pulled from for this long is closed, and at
// most this many are open at once, the least recently pulled closed first.
// The frame queue uses two, one a decoder, and a third while it changes
// over.
const (
	previewIdle = 20 * time.Second
	previewMost = 6
)

type previewFrame struct {
	at   float64
	data []byte
}

type preview struct {
	// "sound" for a stream of sound, "" for one of frames.
	kind   string
	frames chan previewFrame
	stop   context.CancelFunc
	// When it was last pulled from, as unix nanoseconds.
	pulled atomicTime
	// Closed when ffmpeg has stopped, with why in err.
	done chan struct{}
	err  error
	// Where a stream of frames' time went before its first frame, told
	// once, with the first pull that has a frame. Nil for sound.
	times *engine.PreviewTimes
	told  atomic.Bool
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

func (p *previews) serve(st *store, w http.ResponseWriter, r *http.Request) {
	q := r.URL.Query()
	switch r.URL.Path {
	case "/frames/open", "/frames/sound":
		p.serveOpen(st, w, r)
	case "/frames/read":
		s := p.get(q.Get("id"))
		if s == nil {
			http.NotFound(w, r)
			return
		}
		n, _ := strconv.Atoi(q.Get("n"))
		skip, err := strconv.ParseFloat(q.Get("skip"), 64)
		if err != nil || math.IsNaN(skip) || math.IsInf(skip, 0) {
			skip = 0
		}
		writeFrames(w, r, s, min(max(n, 1), 16), skip)
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
		if err := p.hold(path); err != nil {
			http.Error(w, err.Error(), http.StatusInternalServerError)
			return
		}
		w.WriteHeader(http.StatusNoContent)
	default:
		http.NotFound(w, r)
	}
}

func (p *previews) serveOpen(st *store, w http.ResponseWriter, r *http.Request) {
	q := r.URL.Query()
	path := q.Get("path")
	if !episodeFile(st, path) {
		http.NotFound(w, r)
		return
	}
	from, err := strconv.ParseFloat(q.Get("from"), 64)
	if err != nil || math.IsNaN(from) || math.IsInf(from, 0) || from < 0 {
		from = 0
	}
	e := engine.NewEngine(engine.NewLog(io.Discard, false, false))
	var run func(ctx context.Context, got func(at float64, data []byte) error) error
	var times *engine.PreviewTimes
	if r.URL.Path == "/frames/sound" {
		rate, _ := strconv.Atoi(q.Get("rate"))
		channels, _ := strconv.Atoi(q.Get("ch"))
		if rate < 8000 || rate > 192000 || channels < 1 || channels > 8 {
			http.Error(w, "a sound stream needs a rate and a number of channels", http.StatusBadRequest)
			return
		}
		dec, err := p.decoder(path)
		run = func(ctx context.Context, got func(float64, []byte) error) error {
			if err != nil {
				return err
			}
			return dec.Sound(ctx, e, from, rate, channels, got)
		}
	} else {
		width, _ := strconv.Atoi(q.Get("w"))
		height, _ := strconv.Atoi(q.Get("h"))
		if width < 2 || height < 2 || width > 7680 || height > 4320 {
			http.Error(w, "a preview needs a width and a height", http.StatusBadRequest)
			return
		}
		times = engine.NewPreviewTimes()
		dec, err := p.decoder(path)
		run = func(ctx context.Context, got func(float64, []byte) error) error {
			if err != nil {
				return err
			}
			return dec.Stream(engine.WithPreviewTimes(ctx, times), from, width&^1, height&^1, got)
		}
	}
	kind := ""
	if r.URL.Path == "/frames/sound" {
		kind = "sound"
	}
	id := p.start(kind, times, run)
	w.Header().Set("Content-Type", "application/json")
	_ = json.NewEncoder(w).Encode(map[string]string{"id": id})
}

// start runs ffmpeg for a stream. It decodes a frame and waits until the
// page has pulled it before it decodes the next, so the channel holds one.
// Streams of sound and of frames are counted apart, so that seeks of a
// play, each with a sound stream of its own, never close the frames the
// video preview is drawing, nor frames the sound.
func (p *previews) start(kind string, times *engine.PreviewTimes, run func(ctx context.Context, got func(at float64, data []byte) error) error) string {
	var raw [16]byte
	_, _ = rand.Read(raw[:])
	id := hex.EncodeToString(raw[:])
	ctx, stop := context.WithCancel(context.Background())
	s := &preview{kind: kind, frames: make(chan previewFrame), stop: stop, done: make(chan struct{}), times: times}
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
		err := run(ctx, func(at float64, frame []byte) error {
			// A frame is handed over in a buffer of its own, see
			// engine.PreviewFrames, and kept as it is. Sound comes in one
			// buffer read into again and again, so it is copied.
			if kind == "sound" {
				frame = append([]byte(nil), frame...)
			}
			f := previewFrame{at: at, data: frame}
			select {
			case s.frames <- f:
				return nil
			case <-ctx.Done():
				return ctx.Err()
			}
		})
		// Stopped from here, whatever ffmpeg said as it was killed, or the
		// episode's decoder closed under it: the page asks another stream.
		if ctx.Err() != nil || errors.Is(err, engine.ErrFramesClosed) {
			err = context.Canceled
		}
		s.err = err
	}()
	return id
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
func (p *previews) hold(path string) error {
	d, err := p.decoder(path)
	if err != nil {
		return err
	}
	p.mu.Lock()
	if p.held == nil {
		p.held = map[string]int{}
	}
	p.held[path]++
	p.mu.Unlock()
	return d.Ready()
}

// release lets go of a hold. The decoder is then closed once it has stood
// unused, see reap.
func (p *previews) release(path string) {
	p.mu.Lock()
	defer p.mu.Unlock()
	if p.held[path] > 1 {
		p.held[path]--
	} else {
		delete(p.held, path)
	}
}

// decoder is the episode's decoder for the episode at path, started by a
// hold or on its first stream. It ships beside the app, as ffmpeg does, and an app
// without it says so the way it says a missing ffmpeg: there is no second
// way to the frames.
func (p *previews) decoder(path string) (*engine.EpisodeFrames, error) {
	program, err := engine.FindTool("FRAMEFAIRY_FRAMES", "framefairy-frames")
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
		d = engine.NewEpisodeFrames(program, path)
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
		p.reapOnce(previewIdle)
	}
}

// reapOnce closes the streams nobody has pulled for idle, and the decoders
// nobody holds that have stood unused for three times that.
func (p *previews) reapOnce(idle time.Duration) {
	p.mu.Lock()
	defer p.mu.Unlock()
	for path, d := range p.decoders {
		if p.held[path] == 0 && d.Idle() > 3*idle {
			delete(p.decoders, path)
			go d.Close()
		}
	}
	for id, s := range p.open {
		if time.Since(s.pulled.get()) > idle {
			s.stop()
			delete(p.open, id)
		}
	}
}

// writeFrames answers a pull: the next frame once ffmpeg has it, and up to
// n in all, as many as are there by then without waiting for more. Each is
// the moment of the episode it starts at, 8 bytes, a float64 in little
// endian, then the frame in 8-bit I420, or the moments of sound, from the
// first that starts at skip or later. Every frame of a stream is as long
// as every other, but the last of a sound stream, which may be shorter. A stream that has ended answers
// with no frame and X-Frames-End, and with the reason when it failed.
func writeFrames(w http.ResponseWriter, r *http.Request, s *preview, n int, skip float64) {
	w.Header().Set("Content-Type", "application/octet-stream")
	w.Header().Set("Cache-Control", "no-store")
	head := make([]byte, 8)
	put := func(f previewFrame) bool {
		binary.LittleEndian.PutUint64(head, math.Float64bits(f.at))
		if _, err := w.Write(head); err != nil {
			return false
		}
		_, err := w.Write(f.data)
		return err == nil
	}
	// Frames before skip are decoded and dropped here: the page wants one
	// further on, and stepping on in a stream that is open is quicker than
	// a new one from the key frame before.
	for waiting := true; waiting; {
		select {
		case f := <-s.frames:
			if f.at < skip {
				continue
			}
			// The first frame of a stream says where its time went, in
			// milliseconds from when it was asked for: until the decoder
			// had the request, until it was at the place, until the first
			// frame was out, and then 1 where that meant opening the file
			// and 0 where a cursor was moved, see engine.PreviewTimes.
			if s.times != nil && !s.told.Swap(true) {
				file := 0
				if s.times.File.Load() {
					file = 1
				}
				w.Header().Set("X-Frames-Times", fmt.Sprintf("%d,%d,%d,%d",
					s.times.Started.Load(), s.times.Opened.Load(), s.times.First.Load(), file))
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
				w.Header().Set("X-Frames-Error", engine.Scrub(s.err.Error(), 300))
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
