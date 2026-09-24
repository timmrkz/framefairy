package main

import (
	"errors"
	"os"

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
	// Heard is how far the transcript reaches, silence at its end
	// included. Past it a second weighs Rate.
	Heard float64 `json:"heard"`
	Rate  float64 `json:"rate"`
}

// Room says what a search on this episode can read, with the settings as
// they are now. Before the first transcription there are no lines, which
// is an answer and not a failure.
func (s *FrameFairy) Room(path string) (RoomView, error) {
	if !s.store.Known(path) {
		return RoomView{}, os.ErrNotExist
	}
	opts := s.store.Settings().options()
	p := engine.NewProject(nil, path, opts)
	room := engine.SearchRoom(opts, p.LogsDir())
	out := RoomView{Chars: room.Chars, By: room.By, Lines: []engine.LineWeight{},
		Rate: engine.SpokenChars}
	t, err := s.transcript(p)
	if errors.Is(err, engine.ErrNoTranscript) {
		return out, nil
	}
	if err != nil {
		return RoomView{}, err
	}
	out.Lines = engine.WeighLines(t)
	out.Heard = t.Start + t.Duration()
	out.Rate = engine.RateOf(out.Lines)
	return out, nil
}
