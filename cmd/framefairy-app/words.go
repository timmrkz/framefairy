package main

import (
	"context"
	"errors"
	"fmt"
	"os"
	"sync"

	"framefairy/engine"
)

// transcript is the whole-episode transcript, read once and kept, for the
// calls the timeline makes over and over as it is moved. A four hour
// transcript is megabytes of words and levels, and reading it for every
// swipe made the timeline crawl and the waveform arrive in steps.
//
// The transcripts of the last keptTranscripts episodes are kept, as many as
// the app keeps workspaces of, so going back to one reads nothing. One was
// kept, so every episode picked read its transcript again. And it is read
// once however many ask at once: an episode opening asks for its words, its
// waveform, its captions and its room together, and each read the whole
// file for itself, up to seven times.
//
// What it hands out is shared, so only what reads goes through here.
func (s *FrameFairy) transcript(p *engine.Project) (*engine.Transcript, error) {
	stamp := ""
	for _, file := range engine.TranscriptFiles(p.LogsDir()) {
		info, err := os.Stat(file)
		if err != nil {
			stamp += "|-"
			continue
		}
		stamp += fmt.Sprintf("|%d,%d", info.Size(), info.ModTime().UnixNano())
	}
	return s.said.get(p.Source, stamp, p.Transcript)
}

// keptTranscripts is how many episodes' transcripts are kept, the same as
// KEPT in App.svelte.
const keptTranscripts = 10

// keptReads keeps what was read for the last few keys, each with the stamp
// of what it was read from, and reads a key once however many ask at once.
// The zero value keeps keptTranscripts.
type keptReads[T any] struct {
	mu   sync.Mutex
	held map[string]*keptRead[T]
	uses uint64
}

// keptRead is one key's read: as stamp says its source was, or still being
// read until done is closed.
type keptRead[T any] struct {
	stamp string
	v     T
	err   error
	done  chan struct{}
	used  uint64
}

// get gives what read gives for key, read again only when stamp has
// changed. A failure is not kept: the next ask reads again.
func (k *keptReads[T]) get(key, stamp string, read func() (T, error)) (T, error) {
	k.mu.Lock()
	k.uses++
	if r := k.held[key]; r != nil && r.stamp == stamp {
		r.used = k.uses
		k.mu.Unlock()
		<-r.done
		return r.v, r.err
	}
	r := &keptRead[T]{stamp: stamp, used: k.uses, done: make(chan struct{})}
	if k.held == nil {
		k.held = map[string]*keptRead[T]{}
	}
	k.held[key] = r
	for len(k.held) > keptTranscripts {
		oldest := ""
		for other, o := range k.held {
			if oldest == "" || o.used < k.held[oldest].used {
				oldest = other
			}
		}
		delete(k.held, oldest)
	}
	k.mu.Unlock()
	// However the read ends, a panic too, whoever waits is let go and a
	// failure is not kept. The failure is taken out first, so a waiter let
	// go that asks again reads again rather than finding it.
	defer func() {
		if r.err != nil {
			k.mu.Lock()
			if k.held[key] == r {
				delete(k.held, key)
			}
			k.mu.Unlock()
		}
		close(r.done)
	}()
	r.err = fmt.Errorf("the read stopped before it finished")
	r.v, r.err = read()
	return r.v, r.err
}

// words is what an episode says, see engine/words.go, from the transcript
// read once and kept. An episode not transcribed yet says nothing, which
// is an empty answer and not a failure.
func (s *FrameFairy) words(path string) (*engine.Transcript, error) {
	t, err := s.transcript(engine.NewProject(nil, path, s.store.Settings().options()))
	if errors.Is(err, engine.ErrNoTranscript) {
		return &engine.Transcript{}, nil
	}
	return t, err
}

// Words returns the words said in a part, for walking the playhead from
// word to word. Before the first transcription there are none, which is an
// empty answer and not a failure.
func (s *FrameFairy) Words(path string, from, to float64) ([]engine.WordView, error) {
	if !s.store.Known(path) {
		return nil, errNotInLibrary
	}
	t, err := s.words(path)
	if err != nil {
		return nil, err
	}
	out := []engine.WordView{}
	// One word either side, so a step can reach past the part.
	for _, w := range t.WordsBetween(from-5, to+5) {
		out = append(out, engine.WordView{Start: w.Start, End: w.End, Text: w.Text})
	}
	return out, nil
}

func (s *FrameFairy) SetWord(ctx context.Context, path, plan, clipID string, start float64, text string) (ClipEntry, error) {
	if !s.store.PlanOf(path, plan) {
		return ClipEntry{}, errNotInLibrary
	}
	p := engine.NewProject(nil, path, s.store.Settings().options())
	// Read fresh, not from what Waveform and Words keep: an edit works on
	// the words themselves, and nothing else may be holding them.
	t, err := p.Transcript()
	if err != nil {
		return ClipEntry{}, err
	}
	if err := s.edit(path, func() error {
		return engine.SetWordText(p.LogsDir(), start, text, t)
	}); err != nil {
		return ClipEntry{}, err
	}
	return s.clipEntry(ctx, path, plan, clipID)
}
