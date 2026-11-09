package engine

import (
	"context"
	"sync"
	"time"
)

// speechHost is the one speech model a program keeps loaded, the way
// modelHost keeps the local model: loaded once, shared by every hearing
// at once, and kept while nobody hears when it is the one KeepModel
// named. Loading it took about 5 s on an M2 Max, four copies of it, and
// a part of a clip heard in well under a second waited that long first,
// every time, Tim's report of 9 November.
type speechHost struct {
	mu sync.Mutex
	// dir is the folder of the model loaded, rec the model, nil while
	// none is, and users how many hearings hold it now.
	dir   string
	rec   Recognizer
	users int
	// loading is closed when the load under way ends, nil when none is.
	loading chan struct{}
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
// a hearing, the one asked for is loaded for this hearing alone.
func (e *Engine) speech(ctx context.Context, dir string) (Recognizer, func(), error) {
	host := &e.speechHost
	host.mu.Lock()
	for {
		if host.rec != nil && host.dir == dir {
			host.users++
			host.mu.Unlock()
			var once sync.Once
			return host.rec, func() { once.Do(host.release) }, nil
		}
		if host.loading == nil {
			break
		}
		ch := host.loading
		host.mu.Unlock()
		select {
		case <-ch:
		case <-ctx.Done():
			return nil, nil, ctx.Err()
		}
		host.mu.Lock()
	}
	if host.rec != nil && host.users > 0 {
		host.mu.Unlock()
		rec, err := e.loadSpeech(dir)
		if err != nil {
			return nil, nil, err
		}
		return rec, rec.Close, nil
	}
	if host.rec != nil {
		host.rec.Close()
		host.rec = nil
	}
	ch := make(chan struct{})
	host.loading = ch
	host.mu.Unlock()
	rec, err := e.loadSpeech(dir)
	host.mu.Lock()
	host.loading = nil
	close(ch)
	if err != nil {
		host.mu.Unlock()
		return nil, nil, err
	}
	host.rec, host.dir = rec, dir
	host.mu.Unlock()
	return e.speech(ctx, dir)
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
}

// dropLocked closes the model when nobody holds it and it is not the one
// kept. host.mu is held.
func (host *speechHost) dropLocked() {
	if host.rec == nil || host.users > 0 || (host.dir == host.kept && !host.closed) {
		return
	}
	host.rec.Close()
	host.rec = nil
}

// keepSpeech loads the speech model in dir, if it is not loaded, and keeps
// it loaded when nobody hears, until another or none is kept, or the app
// quits. It says whether it loaded the model now.
func (e *Engine) keepSpeech(ctx context.Context, dir string) (bool, error) {
	host := &e.speechHost
	host.mu.Lock()
	if host.closed {
		host.mu.Unlock()
		return false, nil
	}
	host.kept = dir
	there := host.rec != nil && host.dir == dir
	host.dropLocked()
	host.mu.Unlock()
	if there || e.OpenRecognizer == nil {
		return false, nil
	}
	_, release, err := e.speech(ctx, dir)
	if err != nil {
		return false, noSpeechModel(dir, err)
	}
	release()
	return true, nil
}

// keepNoSpeech gives up the speech model kept, closing it now when nobody
// hears with it.
func (host *speechHost) keepNoSpeech(closed bool) {
	host.mu.Lock()
	defer host.mu.Unlock()
	host.kept = ""
	host.closed = host.closed || closed
	host.dropLocked()
}
