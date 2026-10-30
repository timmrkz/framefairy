package main

import (
	"errors"

	"framefairy/engine"
)

// RoomView is how much of an episode one search can read, with what the
// interface needs to turn it into the longest window it lets you draw: the
// weight of every line the transcript has so far, and how heavy a second
// is where there is no transcript yet. The range picker adds the weights
// up itself while a window is dragged, so nothing crosses to the Go side
// at every step of a hand.
type RoomView struct {
	Chars int                 `json:"chars"`
	By    string              `json:"by"`
	Lines []engine.LineWeight `json:"lines"`
	// Heard are the parts the transcript has heard, from and to, silence
	// included. Outside them a second weighs Rate.
	Heard [][2]float64 `json:"heard"`
	Rate  float64      `json:"rate"`
}

// Room says what a search on this episode can read, with the settings as
// they are now. Before the first transcription there are no lines, which
// is an answer and not a failure. It is the engine's RoomAndLines, which
// the window the engine places is fitted to as well.
func (s *FrameFairy) Room(path string) (RoomView, error) {
	if !s.store.Known(path) {
		return RoomView{}, errNotInLibrary
	}
	p := s.project(path)
	t, err := s.transcript(p)
	switch {
	case errors.Is(err, engine.ErrNoTranscript):
		t = nil
	case err != nil:
		return RoomView{}, err
	}
	r := p.RoomAndLines(t)
	out := RoomView{Chars: r.Chars, By: r.By, Lines: r.Lines, Heard: r.Heard, Rate: r.Rate}
	if out.Lines == nil {
		out.Lines = []engine.LineWeight{}
	}
	if out.Heard == nil {
		out.Heard = [][2]float64{}
	}
	return out, nil
}
