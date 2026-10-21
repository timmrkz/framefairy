package main

import (
	"context"
	"errors"
	"io"
	"path/filepath"
	"time"

	"github.com/rs/zerolog"

	"framefairy/engine"
)

// keeping keeps the local model loaded while the app is open, see
// KeepModel in engine/modelhost.go: from the start, again whenever the
// settings or an install change which model the app finds clips with, and
// after every search, which may have put it aside for another.
// A video added then finds the model loaded, and its transcript is read
// into it while it is heard, so its first search starts at once.
type keeping struct {
	// wake asks for the model the settings name to be kept. One ask
	// waiting is enough, whatever changed since.
	wake chan struct{}
}

func newKeeping() *keeping {
	return &keeping{wake: make(chan struct{}, 1)}
}

// ask has the model the settings name kept, or none, in the background.
func (k *keeping) ask() {
	if k == nil {
		return
	}
	select {
	case k.wake <- struct{}{}:
	default:
	}
}

// run keeps what each ask finds in the settings, one ask at a time, for
// as long as the app runs.
func (k *keeping) run(st *store) {
	said := ""
	for range k.wake {
		said = k.keep(st, said)
	}
}

// keep keeps the model the settings name, and says what it did in the
// app's log: the load and its parts when it loaded one, and why not when
// it could not, once for each kind of reason rather than after every
// search. Too little memory is one kind, whatever the numbers. said is
// the reason said last, and what is said now is returned.
func (k *keeping) keep(st *store, said string) string {
	defer func() {
		if v := recover(); v != nil {
			theLog.line(zerolog.ErrorLevel, "model").Interface("panic", v).
				Msg("keeping the model loaded stopped unexpectedly")
		}
	}()
	log := engine.NewLog(io.Discard, false, false)
	log.SetSink(func(ev engine.Event) {
		// The engine says how long the load took and its parts.
		if ev.Kind == engine.EventInfo {
			theLog.line(zerolog.InfoLevel, "model").Msg(ev.Text)
		}
	})
	e := engine.NewEngine(log, st.files)
	opts := jobOptions(st, e)
	started := time.Now()
	loaded, err := e.KeepModel(context.Background(), opts, filepath.Dir(appLogPath()))
	switch {
	case err != nil:
		reason := err.Error()
		if errors.Is(err, engine.ErrNoRoom) {
			reason = engine.ErrNoRoom.Error()
		}
		if reason != said {
			theLog.line(zerolog.InfoLevel, "model").Err(err).Msg("the model is not loaded now")
		}
		return reason
	case loaded:
		theLog.line(zerolog.InfoLevel, "model").Int64("ms", time.Since(started).Milliseconds()).
			Msg("the model is loaded and kept while the app is open")
	}
	return ""
}
