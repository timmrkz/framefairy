package main

import (
	"context"
	"errors"
	"slices"
	"strings"
	"time"

	"github.com/rs/zerolog"

	"framefairy/engine"
)

// keeping keeps the models loaded while the app is open, the speech model
// and the local model, see KeepSpeech in engine/speechhost.go and
// KeepModel in engine/modelhost.go: from the start, again whenever the
// settings or an install change which models the app hears and finds
// clips with, and after every search or hearing, which may have put a
// model aside for another. A video added then finds
// both loaded: it is heard at once, and its transcript is read into the
// local model while it is heard, so its first search starts at once.
// A part of a clip heard without the speech model kept waited about 5 s
// for it to load, every time, Tim's report of 9 November.
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

// keep keeps the models the settings name, the speech model first and
// then the local model, and says what it did in the app's log, for each
// by its name: the load when it loaded one, and why not when it could
// not, once for each kind of reason rather than after every search. Too
// little memory is one kind, whatever the numbers. said is the reasons
// said last, one a line, and what is said now is returned.
func (k *keeping) keep(st *store, said string) string {
	defer func() {
		if v := recover(); v != nil {
			theLog.line(zerolog.ErrorLevel, "model").Interface("panic", v).
				Msg("keeping the models loaded stopped unexpectedly")
		}
	}()
	// The engine says how long each load took and its parts, and what
	// went wrong on the way.
	e := engine.NewEngine(engineLog("model", ""), st.machine)
	opts := jobOptions(st, e)
	models := []struct {
		name string
		keep func(context.Context, engine.Options) (bool, error)
	}{
		{"the speech model", e.KeepSpeech},
		{"the local model", e.KeepModel},
	}
	var reasons []string
	for _, m := range models {
		started := time.Now()
		loaded, err := m.keep(context.Background(), opts)
		switch {
		case err != nil:
			reason := err.Error()
			if errors.Is(err, engine.ErrNoRoom) {
				reason = engine.ErrNoRoom.Error()
			}
			if !slices.Contains(strings.Split(said, "\n"), reason) {
				theLog.line(zerolog.InfoLevel, "model").Err(err).Msgf("%s is not loaded now", m.name)
			}
			reasons = append(reasons, reason)
		case loaded:
			theLog.line(zerolog.InfoLevel, "model").Int64("ms", time.Since(started).Milliseconds()).
				Msgf("%s is loaded and kept while the app is open", m.name)
		}
	}
	return strings.Join(reasons, "\n")
}
