package main

import (
	"fmt"
	"testing"

	"framefairy/engine"
)

// The window the engine places on a video fits the reach the range
// picker works out from Room, where speech is uneven too, so the
// workspace has nothing to cut and write back. The two were once worked
// out apart and disagreed, the fourth review of #458, and both now come
// from engine.RoomAndLines.
func TestThePlacedWindowFitsTheRoomTheRangePickerReads(t *testing.T) {
	svc, eps := memoryLibrary(t, "ep.mp4")
	source := eps[0]
	const duration = 3600.0
	// Slow speech for the first half hour, fast and long words after it.
	var words [][3]any
	for at := 0.0; at+1 <= duration; at += 0.5 {
		word := "ja"
		if at >= 1800 {
			word = "Donaudampfschifffahrtsgesellschaft"
		}
		words = append(words, [3]any{at, at + 0.4, fmt.Sprintf("%s%d", word, len(words))})
	}
	heardEpisode(t, svc, source, duration, words)

	room, err := svc.Room(source)
	if err != nil {
		t.Fatal(err)
	}
	reach := engine.RoomAndLines{Room: engine.Room{Chars: room.Chars}, Lines: room.Lines, Heard: room.Heard, Rate: room.Rate}.Longest(duration)
	if reach >= engine.SuggestedWindow(duration) {
		t.Fatalf("one search reads %v s, no less than a window, so this proves nothing", reach)
	}
	got := svc.placedWindow(source, duration)
	if got.To-got.From > reach || got.Length > reach {
		t.Errorf("the window %+v is longer than the range picker's reach of %v s", got, reach)
	}
	if got.From != 0 || got.Length <= 0 {
		t.Errorf("the window %+v is not the first of the video", got)
	}
}
