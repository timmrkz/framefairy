package engine

import (
	"context"
	"errors"
	"sync"
	"time"

	"framefairy/internal/wake"
)

// ---------------------------------------------------------------------------
// The local model, kept running between the asks that want it
//
// Loading the model takes llama-server 9 to 24 seconds on an M2 Max. A
// search used to start it, ask it and stop it, so every search began with
// those seconds, after the transcript had reached the end of the window.
// The app now asks for the model while the transcript is still on its way,
// and the search that follows finds it loaded.
//
// So the running server belongs to the program, not to one search: the app
// makes an engine per job, and a model loaded by one has to be there for
// the next. One model runs at a time, because two of them do not fit in
// the memory of the machines this is for. Whoever holds it says how long
// it may stay when they are done: a search lets go of it at once, and a
// warm-up keeps it a few minutes for the search it was loaded for.
//
// The app keeps its model loaded for as long as it is open, see KeepModel,
// so even the first search of a new video finds it loaded. Loaded
// ahead only when a video was added, it was still loading when a half hour
// had been heard. A model kept idle costs less than its size: llama.cpp
// maps the weights from the file and holds them wired for the graphics
// chip only for three minutes after they were last used, so the system can
// take the memory back for another program, and the model reads the
// weights from the disk again when it is next asked.
// ---------------------------------------------------------------------------

// idleFor starts the wait a model nobody holds is kept for, which runs
// gone at its end, and gives what calls the wait off. It is the host's
// idle when a test takes the wait in hand itself, and time.AfterFunc when
// it is empty.
func (host *modelHost) idleFor(keep time.Duration, gone func()) (callOff func() bool) {
	if host.idle != nil {
		return host.idle(keep, gone)
	}
	return time.AfterFunc(keep, gone).Stop
}

// warmKeep is how long a model loaded ahead of a search waits for it.
const warmKeep = 5 * time.Minute

// launchFor starts llama-server, through the host's launch when a test
// put a stand-in there, so what is held and let go of can be tested
// without a model, and through Engine.startServer when it is empty.
func (host *modelHost) launchFor(e *Engine, ctx context.Context, m LocalModel, size int) (string, func(), error) {
	if host.launch != nil {
		return host.launch(e, ctx, m, size)
	}
	return e.startServer(ctx, m, size)
}

// errModelBusy is a warm-up finding another model in use. A warm-up is
// only a head start, so it gives up rather than wait or load a second one.
var errModelBusy = errors.New("another model is in use")

type hostedModel struct {
	// host is the one that loaded it.
	host  *modelHost
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
	idle  func() bool
}

// One model at a time, never two. Two of them do not fit in the memory of
// the machines this is for: a second llama-server beside the first is
// swapping, a frozen machine or one of the two killed for memory. So an
// ask that needs another model, or more room, waits until the one loaded
// is free and then takes its place. There is one host on each machine, so
// one in each program, see Machine.host.
type modelHost struct {
	mu    sync.Mutex
	model *hostedModel
	// changed wakes whoever waits for the model whenever it is let go of,
	// stopped or loaded, so they look again.
	changed wake.Wake
	// news wakes a test on every change to the host, an ask that starts
	// waiting, joins a load or lets go included, so the test goes on when
	// that happens rather than after a guess at the time. changed is not
	// told of those, since an ask woken by another ask starting to wait
	// would only wait again, and two asks would wake each other forever.
	news wake.Wake
	// down ends every load under way when the app stops the models.
	down    context.Context
	stopAll context.CancelFunc
	// closed is set when the app quits. No model is loaded after it.
	closed bool
	// kept is the model that stays loaded when nobody holds it, see
	// KeepModel, empty for none.
	kept string
	// waiting is how many asks wait for the model in use to be let go
	// of, which a test waits for on news.
	waiting int
	// idled is called by an idle timer once it has decided whether the
	// model goes, so a test waits for that rather than for the clock.
	idled func()
	// launch and idle are the host's seams, see launchFor and idleFor.
	// A test puts its own on its own machine.
	launch func(e *Engine, ctx context.Context, m LocalModel, size int) (string, func(), error)
	idle   func(keep time.Duration, gone func()) (callOff func() bool)
}

// downLocked is what ends every load under way when the models are
// stopped, made the first time a load asks. host.mu is held.
func (host *modelHost) downLocked() context.Context {
	if host.down == nil {
		host.down, host.stopAll = context.WithCancel(context.Background())
	}
	return host.down
}

// changedLocked wakes whoever waits on the model. host.mu is held.
func (host *modelHost) changedLocked() {
	host.changed.WakeLocked()
	host.news.WakeLocked()
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
func (host *modelHost) modelReady(model string, size int) bool {
	host.mu.Lock()
	defer host.mu.Unlock()
	h := host.model
	if h == nil || h.model != model || h.size < size || !loaded(h) {
		return false
	}
	return h.err == nil
}

// modelThere says whether this model is loaded or on its way, at any size.
func (host *modelHost) modelThere(model string) bool {
	host.mu.Lock()
	defer host.mu.Unlock()
	h := host.model
	return h != nil && h.model == model && (!loaded(h) || h.err == nil)
}

// holdModel gives the address of llama-server with this model loaded,
// starting it if nobody has, and the release that lets go of it. keep is
// how long it may stay loaded after the last holder lets go.
func (e *Engine) holdModel(ctx context.Context, m LocalModel, size int) (string, func(keep time.Duration), error) {
	return e.hold(ctx, m, size, false)
}

// warmModel is holdModel for a warm-up. It never waits for another model
// in use and never puts one aside: it gives up with errModelBusy.
func (e *Engine) warmModel(ctx context.Context, m LocalModel, size int) (string, func(keep time.Duration), error) {
	return e.hold(ctx, m, size, true)
}

func (e *Engine) hold(ctx context.Context, m LocalModel, size int,
	warmUp bool) (string, func(keep time.Duration), error) {
	release := (*hostedModel).release
	host := &e.host
	for {
		host.mu.Lock()
		h := host.model
		switch {
		case host.closed:
			host.mu.Unlock()
			return "", nil, ErrCancelled
		case h == nil:
			return e.load(ctx, m, size, release)
		case h.model == m.Model && h.size >= size:
			h.users++
			host.news.WakeLocked()
			if h.idle != nil {
				h.idle()
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
			return h.url, func(keep time.Duration) { release(h, keep) }, nil
		case loaded(h) && h.users == 0:
			// Another model, or this one with too little room, and nobody
			// is using it. It goes, and this one takes its place.
			h.shutLocked()
			host.model = nil
			host.changedLocked()
			host.mu.Unlock()
		case warmUp:
			host.mu.Unlock()
			return "", nil, errModelBusy
		default:
			// Another model in use or on its way. This ask waits for it to
			// be let go of rather than load a second one beside it.
			wait := host.changed.WaitLocked()
			host.waiting++
			host.news.WakeLocked()
			host.mu.Unlock()
			select {
			case <-wait:
			case <-ctx.Done():
			}
			host.mu.Lock()
			host.waiting--
			host.news.WakeLocked()
			host.mu.Unlock()
			if ctx.Err() != nil {
				return "", nil, ctx.Err()
			}
		}
	}
}

// load starts the model with nobody else holding one. host.mu is held on
// the way in and let go of here.
func (e *Engine) load(ctx context.Context, m LocalModel, size int,
	release func(*hostedModel, time.Duration)) (string, func(keep time.Duration), error) {
	host := &e.host
	h := &hostedModel{host: host, model: m.Model, size: size, ready: make(chan struct{}), users: 1}
	host.model = h
	down := host.downLocked()
	host.mu.Unlock()

	// The load ends when the ask gives up or when the app stops the models.
	loading, cancel := context.WithCancel(ctx)
	unwatch := context.AfterFunc(down, cancel)
	url, stop, err := host.launchFor(e, loading, m, size)
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
	host.changedLocked()
	host.mu.Unlock()
	if err != nil {
		return "", nil, err
	}
	return url, func(keep time.Duration) { release(h, keep) }, nil
}

// release lets go of the model. The last one to let go decides how long it
// stays.
func (h *hostedModel) release(keep time.Duration) {
	host := h.host
	host.mu.Lock()
	defer host.mu.Unlock()
	h.users--
	host.news.WakeLocked()
	if h.users > 0 {
		return
	}
	defer host.changedLocked()
	if host.model != h {
		// It was replaced or stopped. Nothing of it is running.
		return
	}
	if host.kept == h.model {
		return
	}
	if keep <= 0 {
		h.shutLocked()
		host.model = nil
		return
	}
	h.idle = host.idleFor(keep, func() {
		host.mu.Lock()
		defer host.mu.Unlock()
		if host.model == h && h.users == 0 && host.kept != h.model {
			h.shutLocked()
			host.model = nil
			host.changedLocked()
		}
		if host.idled != nil {
			host.idled()
		}
	})
}

// shutLocked stops the server. host.mu is held.
func (h *hostedModel) shutLocked() {
	if h.idle != nil {
		h.idle()
		h.idle = nil
	}
	if h.stop != nil {
		h.stop()
		h.stop = nil
	}
}

// keepOnly makes model the one kept loaded when nobody holds it, none for
// empty. A model kept before that nobody holds now goes at once, so a
// model given up in the settings does not hold the memory for the new one.
func (host *modelHost) keepOnly(model string) {
	host.mu.Lock()
	defer host.mu.Unlock()
	if host.closed {
		return
	}
	was := host.kept
	host.kept = model
	h := host.model
	if was == "" || was == model || h == nil || h.model != was || h.users > 0 || !loaded(h) {
		return
	}
	h.shutLocked()
	host.model = nil
	host.changedLocked()
}

// KeepModel loads the local model opts ask for, if it is not loaded, and
// keeps it loaded when nobody holds it, until it is called again for
// another model or none, or the models are stopped. It says whether it
// loaded the model now. The model is loaded for a window of an hour,
// which only sizes its memory: a search that needs more loads it again
// with more, and that is kept. With the API, or a server that runs
// already, nothing is kept, and a model kept before goes. A model that
// does not fit in the memory free now is not loaded, the error says why,
// and the first search loads it when it fits.
func (e *Engine) KeepModel(ctx context.Context, opts Options) (bool, error) {
	if opts.Planner != "local" || opts.LLMURL != "" {
		e.host.keepOnly("")
		return false, nil
	}
	local, err := e.resolveLocal(opts)
	if err != nil {
		e.host.keepOnly("")
		return false, err
	}
	return e.keep(ctx, *local, opts.MaxTokens)
}

// KeepNone gives up the model kept loaded, and stops it now when nobody
// holds it. The app calls it before it removes the model's file, which a
// server that maps it could otherwise keep from being removed.
func (m *Machine) KeepNone() {
	m.host.keepOnly("")
}

// keep is KeepModel with the model found. A model loaded or loading
// already is left as it is, at whatever size: it is kept by its name, and
// loading it again at another size would cost the whole load, and what was
// read into it, for nothing a search needs.
func (e *Engine) keep(ctx context.Context, local LocalModel, maxTokens int) (bool, error) {
	e.host.keepOnly(local.Model)
	if e.host.modelThere(local.Model) {
		return false, nil
	}
	size := e.warmSize(local.Model, 0, maxTokens)
	if err := e.roomForModel(local.Model, size); err != nil {
		return false, err
	}
	_, release, err := e.warmModel(ctx, local, size)
	if errors.Is(err, errModelBusy) {
		// Another model in use, which the first search after it replaces.
		return false, nil
	}
	if err != nil {
		return false, err
	}
	release(0)
	return true, nil
}

// StopModels stops the local model, loaded or still loading, and returns
// once it is stopped. The app calls it on the way out, so no server is
// left holding the memory after it. A model still loading used to be left
// to its loader, which never ran again once the app had quit, and a
// llama-server held its memory until the machine was restarted.
func (m *Machine) StopModels() {
	host := &m.host
	host.mu.Lock()
	host.kept = ""
	if host.stopAll != nil {
		host.stopAll()
	}
	h := host.model
	host.model = nil
	if h != nil && loaded(h) {
		h.shutLocked()
	}
	host.changedLocked()
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
func (m *Machine) CloseModels() {
	m.host.mu.Lock()
	m.host.closed = true
	m.host.mu.Unlock()
	m.StopModels()
}
