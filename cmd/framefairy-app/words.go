package main

import (
	"context"
	"errors"
	"fmt"
	"os"

	"framefairy/engine"
)

// transcript is the whole-episode transcript, read once and kept, for the
// two calls the timeline makes over and over as it is moved. A four hour
// transcript is megabytes of words and levels, and reading it for every
// swipe made the timeline crawl and the waveform arrive in steps.
//
// What it hands out is shared, so only what reads goes through here.
func (s *FrameFairy) transcript(p *engine.Project) (*engine.Transcript, error) {
	stamp := p.Source
	for _, file := range engine.TranscriptFiles(p.LogsDir()) {
		info, err := os.Stat(file)
		if err != nil {
			stamp += "|-"
			continue
		}
		stamp += fmt.Sprintf("|%d,%d", info.Size(), info.ModTime().UnixNano())
	}
	s.mu.Lock()
	if s.said != nil && s.saidBy == stamp {
		kept := s.said
		s.mu.Unlock()
		return kept, nil
	}
	s.mu.Unlock()
	t, err := p.Transcript()
	if err != nil {
		return nil, err
	}
	s.mu.Lock()
	s.said, s.saidBy = t, stamp
	s.mu.Unlock()
	return t, nil
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
