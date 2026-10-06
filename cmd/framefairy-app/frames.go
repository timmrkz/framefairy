package main

import (
	"context"
	"crypto/rand"
	"encoding/binary"
	"encoding/hex"
	"encoding/json"
	"errors"
	"io"
	"math"
	"net/http"
	"os"
	"path/filepath"
	"strconv"
	"sync"
	"time"

	"framefairy/engine"
)

// The picture of a file the webview cannot decode, decoded on the Go side,
// see engine.PreviewFrames. The page opens a stream at a moment, pulls its
// frames a few at a time and closes it. ffmpeg decodes only as far as the
// page has pulled, a frame or two ahead: WebKit takes whatever a response
// writes without ever pushing back, so one long response would have let
// ffmpeg decode the whole episode into memory while the page stood paused.
// A stream left open is how a paused play goes on at once, and how the
// next frame is there when the arrow key steps to it.
//
//	/frames/open?path=<episode>&from=<seconds>&w=<width>&h=<height>  {"id": "..."}
//	/frames/read?id=<id>&n=<frames>  frames, see writeFrames
//	/frames/close?id=<id>
//
// openPreviews are the streams of the app, one set however many times the
// handler is made.
var openPreviews = &previews{}

type previews struct {
	mu   sync.Mutex
	open map[string]*preview
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
	frames chan previewFrame
	stop   context.CancelFunc
	// When it was last pulled from, as unix nanoseconds.
	pulled atomicTime
	// Closed when ffmpeg has stopped, with why in err.
	done chan struct{}
	err  error
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
	case "/frames/open":
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
	default:
		http.NotFound(w, r)
	}
}

func (p *previews) serveOpen(st *store, w http.ResponseWriter, r *http.Request) {
	q := r.URL.Query()
	path := q.Get("path")
	if path == "" || !filepath.IsAbs(path) || !st.Known(path) {
		http.NotFound(w, r)
		return
	}
	if info, err := os.Stat(path); err != nil || !info.Mode().IsRegular() {
		http.NotFound(w, r)
		return
	}
	from, err := strconv.ParseFloat(q.Get("from"), 64)
	if err != nil || math.IsNaN(from) || math.IsInf(from, 0) || from < 0 {
		from = 0
	}
	width, _ := strconv.Atoi(q.Get("w"))
	height, _ := strconv.Atoi(q.Get("h"))
	if width < 2 || height < 2 || width > 7680 || height > 4320 {
		http.Error(w, "a preview needs a width and a height", http.StatusBadRequest)
		return
	}
	id := p.start(path, from, width&^1, height&^1)
	w.Header().Set("Content-Type", "application/json")
	_ = json.NewEncoder(w).Encode(map[string]string{"id": id})
}

// start runs ffmpeg for a stream. It decodes a frame and waits until the
// page has pulled it before it decodes the next, so the channel holds one.
func (p *previews) start(path string, from float64, width, height int) string {
	var raw [16]byte
	_, _ = rand.Read(raw[:])
	id := hex.EncodeToString(raw[:])
	ctx, stop := context.WithCancel(context.Background())
	s := &preview{frames: make(chan previewFrame), stop: stop, done: make(chan struct{})}
	s.pulled.set(time.Now())
	p.mu.Lock()
	if p.open == nil {
		p.open = map[string]*preview{}
	}
	for len(p.open) >= previewMost {
		oldest := ""
		for k, o := range p.open {
			if oldest == "" || o.pulled.get().Before(p.open[oldest].pulled.get()) {
				oldest = k
			}
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
		e := engine.NewEngine(engine.NewLog(io.Discard, false, false))
		s.err = e.PreviewFrames(ctx, path, from, width, height, func(at float64, frame []byte) error {
			f := previewFrame{at: at, data: append([]byte(nil), frame...)}
			select {
			case s.frames <- f:
				return nil
			case <-ctx.Done():
				return ctx.Err()
			}
		})
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

func (p *previews) reap() {
	for {
		time.Sleep(previewIdle / 4)
		p.mu.Lock()
		for id, s := range p.open {
			if time.Since(s.pulled.get()) > previewIdle {
				s.stop()
				delete(p.open, id)
			}
		}
		p.mu.Unlock()
	}
}

// writeFrames answers a pull: the next frame once ffmpeg has it, and up to
// n in all, as many as are there by then without waiting for more. Each is
// the moment of the episode it starts at, 8 bytes, a float64 in little
// endian, then the frame in 8-bit I420, from the first that starts at
// skip or later. A stream that has ended answers
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
			if !put(f) {
				return
			}
			waiting = false
		case <-s.done:
			w.Header().Set("X-Frames-End", "1")
			if s.err != nil && !errors.Is(s.err, context.Canceled) {
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
