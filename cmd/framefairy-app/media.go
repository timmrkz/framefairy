package main

import (
	"context"
	"errors"
	"fmt"
	"io"
	"os"

	"framefairy/engine"
)

// SourceView is what the player needs to know about an episode.
type SourceView struct {
	Duration   float64 `json:"duration"`
	Width      int     `json:"width"`
	Height     int     `json:"height"`
	CropWidth  int     `json:"cropWidth"`
	CropHeight int     `json:"cropHeight"`
	// FPS is the episode's frame rate, which is what one step of the arrow
	// keys on the clip timeline is worth.
	FPS float64 `json:"fps"`
}

func (s *FrameFairy) probe(ctx context.Context, path string) (engine.SourceInfo, error) {
	stamp := ""
	if info, err := os.Stat(path); err == nil {
		stamp = fmt.Sprintf("%s|%d|%d", path, info.Size(), info.ModTime().UnixNano())
	}
	s.mu.Lock()
	cached, ok := s.probed[stamp]
	s.mu.Unlock()
	if ok {
		return cached, nil
	}
	e := engine.NewEngine(engine.NewLog(io.Discard, false, false))
	info, err := e.Probe(ctx, path)
	if err == nil && stamp != "" {
		s.mu.Lock()
		if s.probed == nil {
			s.probed = map[string]engine.SourceInfo{}
		}
		s.probed[stamp] = info
		s.mu.Unlock()
	}
	return info, err
}

// Source probes an episode.
func (s *FrameFairy) Source(ctx context.Context, path string) (SourceView, error) {
	if !s.store.Known(path) {
		return SourceView{}, errNotInLibrary
	}
	info, err := s.probe(ctx, path)
	if err != nil {
		return SourceView{}, err
	}
	o := s.store.Settings().options()
	cw, ch := engine.CropWindow(info, o.Width, o.Height)
	return SourceView{Duration: info.Duration, Width: info.Width, Height: info.Height,
		CropWidth: cw, CropHeight: ch, FPS: info.FPS()}, nil
}

// Waveform returns the loudest level in each of buckets pieces of a part
// of the episode, from its loudness measured on its own or from its
// transcript, whichever reaches further. An episode not measured yet has no
// waveform yet, which is an empty answer and not a failure.
//
// Peaks never answers with more buckets than it measured, so the interface
// is told how fine the measurement was and can draw that finely and no
// finer.
func (s *FrameFairy) Waveform(path string, from, to float64, buckets int) ([]float32, error) {
	if !s.store.Known(path) {
		return nil, errNotInLibrary
	}
	// An episode shown is an episode measured: one added before the
	// loudness had a job of its own is measured the first time it is
	// opened. What the clip timeline asks for is what it shows, and the
	// measuring goes there first.
	s.levels.look(path, from, to)
	s.levels.start(path)
	p := engine.NewProject(nil, path, s.store.Settings().options())
	t, err := s.transcript(p)
	if err != nil && !errors.Is(err, engine.ErrNoTranscript) {
		return nil, err
	}
	// The loudness measured on its own and the transcription's are the
	// same frames from the same samples, so the waveform has whatever
	// either has measured. The measuring runs ahead of the transcription,
	// and a transcript made before it existed is there before it has run.
	t = s.levels.read(path).Over(t)
	if t == nil {
		return []float32{}, nil
	}
	if to <= from {
		to = t.Duration()
	}
	return t.Peaks(from, to, min(max(buckets, 1), 4000)), nil
}
