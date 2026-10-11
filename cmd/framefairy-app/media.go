package main

import (
	"context"
	"errors"
	"fmt"
	"io"
	"os"
	"sync"

	"framefairy/engine"
)

// SourceView is what the player needs to know about an episode.
type SourceView struct {
	Duration   float64 `json:"duration"`
	Width      int     `json:"width"`
	Height     int     `json:"height"`
	CropWidth  int     `json:"cropWidth"`
	CropHeight int     `json:"cropHeight"`
	// TurnedCropWidth is how wide the crop is on the picture rotated a
	// quarter turn, where its width and height have swapped: the crop of a
	// piece a person rotated, see engine.Segment.Rotate.
	TurnedCropWidth int `json:"turnedCropWidth"`
	// FPS is the episode's frame rate, which is what one step of the arrow
	// keys on the clip timeline is worth.
	FPS float64 `json:"fps"`
	// VideoStart is where the picture's first frame begins in the file,
	// and with it the grid its frames are on, so the app counts frames from
	// where the render does. Nought for most files.
	VideoStart float64 `json:"videoStart"`
	// Grid is the frames the video preview shows, the frames of the short,
	// see engine.SourceInfo.Grid: the frame queue counts on them and reads
	// nothing of the file itself.
	Grid GridView `json:"grid"`
	// The sound's samples a second and its channels, both nought for a
	// video with no sound.
	SoundRate     int `json:"soundRate"`
	SoundChannels int `json:"soundChannels"`
}

// GridView is framewire.Grid as the interface reads it.
type GridView struct {
	Start float64 `json:"start"`
	Num   int     `json:"num"`
	Den   int     `json:"den"`
	Count int     `json:"count"`
}

// sources are the episodes probed, by path, size and when they were
// written, for the app and for the video preview's streams alike.
var sources = &probed{}

type probed struct {
	mu   sync.Mutex
	info map[string]engine.SourceInfo
}

func (s *FrameFairy) probe(ctx context.Context, path string) (engine.SourceInfo, error) {
	return sources.probe(ctx, path)
}

func (p *probed) probe(ctx context.Context, path string) (engine.SourceInfo, error) {
	stamp := ""
	if info, err := os.Stat(path); err == nil {
		stamp = fmt.Sprintf("%s|%d|%d", path, info.Size(), info.ModTime().UnixNano())
	}
	p.mu.Lock()
	cached, ok := p.info[stamp]
	p.mu.Unlock()
	if ok {
		return cached, nil
	}
	e := engine.NewEngine(engine.NewLog(io.Discard, false, false))
	info, err := e.Probe(ctx, path)
	if err == nil && stamp != "" {
		p.mu.Lock()
		if p.info == nil {
			p.info = map[string]engine.SourceInfo{}
		}
		p.info[stamp] = info
		p.mu.Unlock()
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
	tw, _ := engine.CropWindow(info.Rotated(90), o.Width, o.Height)
	grid := info.Grid()
	return SourceView{Duration: info.Duration, Width: info.Width, Height: info.Height,
		CropWidth: cw, CropHeight: ch, TurnedCropWidth: tw, FPS: info.FPS(), VideoStart: info.VideoStart,
		Grid:      GridView{Start: grid.Start, Num: grid.Num, Den: grid.Den, Count: grid.Count},
		SoundRate: info.SoundRate, SoundChannels: info.SoundChannels}, nil
}

// frames are an episode's frames, the grid a gesture puts its edges on,
// see engine.Gesture. None when the episode cannot be probed, and the
// edges are then left where they are put.
func (s *FrameFairy) frames(ctx context.Context, path string) engine.SourceInfo {
	info, err := s.probe(ctx, path)
	if err != nil {
		return engine.SourceInfo{}
	}
	return info
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
