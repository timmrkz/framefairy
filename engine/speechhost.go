package engine

import (
	"context"
	"sync"
	"time"

	"framefairy/internal/wake"
)

// speechHost is the one speech model a program keeps loaded, the way
// modelHost keeps the local model: loaded once, shared by every hearing
// at once, and kept while nobody hears when it is the one KeepModel
// named. One model is in memory at a time, never two, the same as the
// local model: a hearing that needs another waits for the one in use to
// be let go of, and a keep gives way to it. Loading it took about 5 s on
// an M2 Max, four copies of it, and a part of a clip heard in well under
// a second waited that long first, every time, Tim's report of 9
// November.
type speechHost struct {
	mu sync.Mutex
	// dir is the folder of the model loaded or loading, rec the model,
	// nil while none is loaded, and users how many hearings hold it or
	// wait for its load.
	dir   string
	rec   Recognizer
	users int
	// loading is set while the model in dir loads.
	loading bool
	// changed wakes whoever waits whenever a model is let go of, loaded,
	// or failed to load, so they look again.
	changed wake.Wake
	// kept is the folder of the model that stays loaded when nobody holds
	// it, see KeepModel, empty for none.
	kept string
	// closed is set when the app quits. No model is kept after it.
	closed bool
}

// speech hands out the speech model in dir, loaded the first time it is
// asked for and shared after that, and the release a hearing calls when
// it is done with it. A model nobody holds goes, unless it is the one
// kept. While another model is in use, the settings having changed under
// a hearing, it waits for that one to be let go of.
func (e *Engine) speech(ctx context.Context, dir string) (Recognizer, func(), error) {
	host := &e.speechHost
	host.mu.Lock()
	for {
		if host.dir == dir && (host.rec != nil || host.loading) {
			break
		}
		if host.users == 0 && !host.loading {
			// Nobody holds the model there is, which goes for this one.
			host.closeLocked()
			host.dir, host.loading = dir, true
			host.users++
			host.mu.Unlock()
			rec, err := e.loadSpeech(dir)
			host.mu.Lock()
			host.loading = false
			if err != nil {
				host.users--
				host.dir = ""
				host.changed.WakeLocked()
				host.mu.Unlock()
				return nil, nil, err
			}
			host.rec = rec
			host.changed.WakeLocked()
			host.mu.Unlock()
			return rec, host.releaser(), nil
		}
		if !host.waitLocked(ctx) {
			return nil, nil, ctx.Err()
		}
	}
	// The model asked for, loaded or on its way.
	host.users++
	for host.loading {
		if !host.waitLocked(ctx) {
			host.release()
			return nil, nil, ctx.Err()
		}
	}
	if host.rec == nil || host.dir != dir {
		// Its load failed. This hearing tries again, and says why when
		// it fails as well.
		host.users--
		host.mu.Unlock()
		return e.speech(ctx, dir)
	}
	rec := host.rec
	host.mu.Unlock()
	return rec, host.releaser(), nil
}

// waitLocked waits for the next change, with host.mu held on the way in
// and on the way out, unless ctx ends first, which it says by false, with
// host.mu let go of.
func (host *speechHost) waitLocked(ctx context.Context) bool {
	ch := host.changed.WaitLocked()
	host.mu.Unlock()
	select {
	case <-ch:
		host.mu.Lock()
		return true
	case <-ctx.Done():
		return false
	}
}

// releaser is how a hearing lets go of the model it holds, once however
// often it is called.
func (host *speechHost) releaser() func() {
	var once sync.Once
	return func() { once.Do(host.release) }
}

// loadSpeech loads the speech model in dir, and says how long it took.
func (e *Engine) loadSpeech(dir string) (Recognizer, error) {
	e.Log.Progress("loading the speech model")
	started := time.Now()
	rec, err := e.OpenRecognizer(dir)
	e.Log.ClearProgress()
	if err != nil {
		return nil, err
	}
	e.Log.Info("speech model loaded in %d ms", time.Since(started).Milliseconds())
	return rec, nil
}

// release lets go of the model one hearing held. The last to let go of
// a model that is not kept closes it.
func (host *speechHost) release() {
	host.mu.Lock()
	defer host.mu.Unlock()
	host.users--
	host.dropLocked()
	host.changed.WakeLocked()
}

// dropLocked closes the model when nobody holds it and it is not the one
// kept. host.mu is held.
func (host *speechHost) dropLocked() {
	if host.users > 0 || (host.dir == host.kept && !host.closed) {
		return
	}
	host.closeLocked()
}

// closeLocked closes the model loaded, which nobody holds. host.mu is
// held.
func (host *speechHost) closeLocked() {
	if host.rec != nil {
		host.rec.Close()
		host.rec = nil
		host.dir = ""
	}
}

// KeepSpeech loads the speech model opts hear with, if it is not loaded,
// and keeps it loaded when nobody hears, until it is called again for
// another or none is kept, or the app quits, the way KeepModel keeps the
// local model. It says whether it loaded the model now. While another
// model is in use it gives way, and the keeper is asked again when that
// hearing ends.
func (e *Engine) KeepSpeech(ctx context.Context, opts Options) (bool, error) {
	return e.keepSpeech(ctx, speechModel(opts))
}

// keepSpeech is KeepSpeech with the model's folder.
func (e *Engine) keepSpeech(ctx context.Context, dir string) (bool, error) {
	host := &e.speechHost
	host.mu.Lock()
	if host.closed {
		host.mu.Unlock()
		return false, nil
	}
	host.kept = dir
	host.dropLocked()
	there := host.dir == dir && (host.rec != nil || host.loading)
	busy := host.users > 0 || host.loading
	host.mu.Unlock()
	if there || busy || e.OpenRecognizer == nil {
		return false, nil
	}
	_, release, err := e.speech(ctx, dir)
	if err != nil {
		return false, noSpeechModel(dir, err)
	}
	release()
	return true, nil
}

// KeepNoSpeech gives up the speech model kept, and closes it now when
// nobody hears with it. The app calls it before it removes a speech
// model's files.
func (m *Machine) KeepNoSpeech() {
	m.speechHost.keepNoSpeech(false)
}

// keepNoSpeech is KeepNoSpeech, and for good when closed, on the way out
// of the app.
func (host *speechHost) keepNoSpeech(closed bool) {
	host.mu.Lock()
	defer host.mu.Unlock()
	host.kept = ""
	host.closed = host.closed || closed
	host.dropLocked()
}

// Speech hands out the speech model kept loaded, or loads it, for
// something outside a job that hears, the app's dictation. It holds the
// model until the release is called.
func (e *Engine) Speech(ctx context.Context, dir string) (Recognizer, func(), error) {
	return e.speech(ctx, dir)
}
