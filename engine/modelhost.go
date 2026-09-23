package engine

import (
	"context"
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

var host struct {
	mu    sync.Mutex
	model *hostedModel
}

// modelReady says whether this model is loaded already, big enough for an
// ask of size, so the ask has nothing to wait for.
func modelReady(model string, size int) bool {
	host.mu.Lock()
	defer host.mu.Unlock()
	h := host.model
	if h == nil || h.model != model || h.size < size {
		return false
	}
	select {
	case <-h.ready:
		return h.err == nil
	default:
		return false
	}
}

// holdModel gives the address of llama-server with this model loaded,
// starting it if nobody has, and the release that lets go of it. keep is
// how long it may stay loaded after the last holder lets go.
func (e *Engine) holdModel(ctx context.Context, m LocalModel, size int,
	logDir string) (string, func(keep time.Duration), error) {
	for {
		host.mu.Lock()
		h := host.model
		if h == nil {
			h = &hostedModel{model: m.Model, size: size, ready: make(chan struct{}), users: 1}
			host.model = h
			host.mu.Unlock()
			url, stop, err := launch(e, ctx, m, size, logDir)
			host.mu.Lock()
			if err == nil && host.model != h {
				// Stopped while it loaded, because the app is closing.
				stop()
				stop, err = nil, ErrCancelled
			}
			h.url, h.stop, h.err = url, stop, err
			if err != nil && host.model == h {
				host.model = nil
			}
			close(h.ready)
			host.mu.Unlock()
			if err != nil {
				return "", nil, err
			}
			return url, h.release, nil
		}
		if h.model == m.Model && h.size >= size {
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
		}
		// Another model, or this one with too little room. It goes once it
		// has finished loading and nobody holds it.
		host.mu.Unlock()
		select {
		case <-h.ready:
		case <-ctx.Done():
			return "", nil, ctx.Err()
		}
		host.mu.Lock()
		switch {
		case host.model != h:
			host.mu.Unlock()
			continue
		case h.users == 0:
			h.shutLocked()
			host.model = nil
			host.mu.Unlock()
			continue
		}
		host.mu.Unlock()
		// It is in use. Nothing waits for a search to end, so this ask
		// runs a server of its own, the way every search used to.
		url, stop, err := launch(e, ctx, m, size, logDir)
		if err != nil {
			return "", nil, err
		}
		return url, func(time.Duration) { stop() }, nil
	}
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
	if host.model != h {
		// It was replaced while it failed to load. Nothing is running.
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

// StopModels stops the local model if one is running. The app calls it on
// the way out, so no server is left holding the memory after it.
func StopModels() {
	host.mu.Lock()
	defer host.mu.Unlock()
	if h := host.model; h != nil {
		select {
		case <-h.ready:
			h.shutLocked()
		default:
			// Still loading. Its loader stops it when it sees it is gone.
		}
		host.model = nil
	}
}
