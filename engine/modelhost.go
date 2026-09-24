package engine

import (
	"context"
	"errors"
	"sync"
	"time"
)

// ---------------------------------------------------------------------------
// The local model, kept running between the asks that want it
//
// Loading the model takes llama-server about 24 seconds on an M2 Max. A
// search used to start it, ask it and stop it, so every search began with
// those seconds, after the transcript had reached the end of the window.
// The app now asks for the model while the transcript is still on its way,
// and the search that follows finds it loaded.
//
// So the running server belongs to the program, not to one search: the app
// makes an engine per job, and a model loaded by one has to be there for
// the next. One model runs at a time, because two of them do not fit in
// the memory of the machines this is for. Whoever holds it says how long
// it may stay when they are done: a search lets go of it at once, because
// a model that sits in memory for days between two searches is memory
// taken from everything else, and a warm-up keeps it a few minutes for the
// search it was loaded for.
// ---------------------------------------------------------------------------

// warmKeep is how long a model loaded ahead of a search waits for it.
const warmKeep = 5 * time.Minute

// launch starts llama-server. Tests put a stand-in here, so what is held
// and let go of can be tested without a model.
var launch = (*Engine).startServer

// errModelBusy is a warm-up finding another model in use. A warm-up is
// only a head start, so it gives up rather than wait or load a second one.
var errModelBusy = errors.New("another model is in use")

type hostedModel struct {
	model string
	// size is the context the server was started with. An ask that needs
	// more starts it again.
	size int
	url  string
	stop func()
	err  error
	// ready is closed when the server has loaded the model, or failed to.
	ready chan struct{}
	users int
	idle  *time.Timer
}

// One model at a time, never two. Two of them do not fit in the memory of
// the machines this is for: a second llama-server beside the first is
// swapping, a frozen machine or one of the two killed for memory. So an
// ask that needs another model, or more room, waits until the one loaded
// is free and then takes its place.
var host struct {
	mu    sync.Mutex
	model *hostedModel
	// changed is closed and made again whenever the model is let go of,
	// stopped or loaded, so whoever waits for it looks again.
	changed chan struct{}
	// down ends every load under way when the app stops the models.
	down    context.Context
	stopAll context.CancelFunc
	// closed is set when the app quits. No model is loaded after it.
	closed bool
}

func init() {
	host.changed = make(chan struct{})
	host.down, host.stopAll = context.WithCancel(context.Background())
}

// changedLocked wakes whoever waits on the model. host.mu is held.
func changedLocked() {
	close(host.changed)
	host.changed = make(chan struct{})
}

func loaded(h *hostedModel) bool {
	select {
	case <-h.ready:
		return true
	default:
		return false
	}
}

// modelReady says whether this model is loaded already, big enough for an
// ask of size, so the ask has nothing to wait for.
func modelReady(model string, size int) bool {
	host.mu.Lock()
	defer host.mu.Unlock()
	h := host.model
	if h == nil || h.model != model || h.size < size || !loaded(h) {
		return false
	}
	return h.err == nil
}

// holdModel gives the address of llama-server with this model loaded,
// starting it if nobody has, and the release that lets go of it. keep is
// how long it may stay loaded after the last holder lets go.
func (e *Engine) holdModel(ctx context.Context, m LocalModel, size int,
	logDir string) (string, func(keep time.Duration), error) {
	return e.hold(ctx, m, size, logDir, false)
}

// warmModel is holdModel for a warm-up. It never waits for another model
// in use and never puts one aside: it gives up with errModelBusy.
func (e *Engine) warmModel(ctx context.Context, m LocalModel, size int,
	logDir string) (string, func(keep time.Duration), error) {
	return e.hold(ctx, m, size, logDir, true)
}

func (e *Engine) hold(ctx context.Context, m LocalModel, size int, logDir string,
	warmUp bool) (string, func(keep time.Duration), error) {
	for {
		host.mu.Lock()
		h := host.model
		switch {
		case host.closed:
			host.mu.Unlock()
			return "", nil, ErrCancelled
		case h == nil:
			return e.load(ctx, m, size, logDir)
		case h.model == m.Model && h.size >= size:
			h.users++
			if h.idle != nil {
				h.idle.Stop()
				h.idle = nil
			}
			host.mu.Unlock()
			select {
			case <-h.ready:
			case <-ctx.Done():
				h.release(0)
				return "", nil, ctx.Err()
			}
			if h.err != nil {
				// Whoever was loading it gave up or failed. Their failure is
				// not this ask's, so it tries again for itself.
				h.release(0)
				if ctx.Err() != nil {
					return "", nil, ctx.Err()
				}
				continue
			}
			return h.url, h.release, nil
		case loaded(h) && h.users == 0:
			// Another model, or this one with too little room, and nobody
			// is using it. It goes, and this one takes its place.
			h.shutLocked()
			host.model = nil
			changedLocked()
			host.mu.Unlock()
		case warmUp:
			host.mu.Unlock()
			return "", nil, errModelBusy
		default:
			// Another model in use or on its way. This ask waits for it to
			// be let go of rather than load a second one beside it.
			wait := host.changed
			host.mu.Unlock()
			select {
			case <-wait:
			case <-ctx.Done():
				return "", nil, ctx.Err()
			}
		}
	}
}

// load starts the model with nobody else holding one. host.mu is held on
// the way in and let go of here.
func (e *Engine) load(ctx context.Context, m LocalModel, size int,
	logDir string) (string, func(keep time.Duration), error) {
	h := &hostedModel{model: m.Model, size: size, ready: make(chan struct{}), users: 1}
	host.model = h
	down := host.down
	host.mu.Unlock()

	// The load ends when the ask gives up or when the app stops the models.
	loading, cancel := context.WithCancel(ctx)
	unwatch := context.AfterFunc(down, cancel)
	url, stop, err := launch(e, loading, m, size, logDir)
	unwatch()
	cancel()

	host.mu.Lock()
	if err == nil && (host.model != h || down.Err() != nil) {
		// Stopped while it loaded, because the app is closing.
		stop()
		stop, err = nil, ErrCancelled
	}
	h.url, h.stop, h.err = url, stop, err
	if err != nil {
		h.users--
		if host.model == h {
			host.model = nil
		}
	}
	close(h.ready)
	changedLocked()
	host.mu.Unlock()
	if err != nil {
		return "", nil, err
	}
	return url, h.release, nil
}

// release lets go of the model. The last one to let go decides how long it
// stays.
func (h *hostedModel) release(keep time.Duration) {
	host.mu.Lock()
	defer host.mu.Unlock()
	h.users--
	if h.users > 0 {
		return
	}
	defer changedLocked()
	if host.model != h {
		// It was replaced or stopped. Nothing of it is running.
		return
	}
	if keep <= 0 {
		h.shutLocked()
		host.model = nil
		return
	}
	h.idle = time.AfterFunc(keep, func() {
		host.mu.Lock()
		defer host.mu.Unlock()
		if host.model == h && h.users == 0 {
			h.shutLocked()
			host.model = nil
			changedLocked()
		}
	})
}

// shutLocked stops the server. host.mu is held.
func (h *hostedModel) shutLocked() {
	if h.idle != nil {
		h.idle.Stop()
		h.idle = nil
	}
	if h.stop != nil {
		h.stop()
		h.stop = nil
	}
}

// StopModels stops the local model, loaded or still loading, and returns
// once it is stopped. The app calls it on the way out, so no server is
// left holding the memory after it. A model still loading used to be left
// to its loader, which never ran again once the app had quit, and a
// llama-server held its memory until the machine was restarted.
func StopModels() {
	host.mu.Lock()
	host.stopAll()
	h := host.model
	host.model = nil
	if h != nil && loaded(h) {
		h.shutLocked()
	}
	changedLocked()
	host.mu.Unlock()
	if h != nil {
		// The loader sees it was stopped and stops its server. That takes
		// llama-server at most five seconds and then it is killed.
		select {
		case <-h.ready:
		case <-time.After(10 * time.Second):
		}
	}
	host.mu.Lock()
	host.down, host.stopAll = context.WithCancel(context.Background())
	host.mu.Unlock()
}

// CloseModels is StopModels for good, on the way out of the app. A search
// that has not yet seen that it was stopped could otherwise ask for the
// model again after StopModels and load a new llama-server with nobody
// left to stop it.
func CloseModels() {
	host.mu.Lock()
	host.closed = true
	host.mu.Unlock()
	StopModels()
}
