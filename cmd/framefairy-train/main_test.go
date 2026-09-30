package main

import (
	"slices"
	"testing"
)

// The episode may come before or after the flags, and a list of clips is
// read however it is spaced.

func TestTheEpisodeMayComeAnywhere(t *testing.T) {
	for _, argv := range [][]string{
		{"ep.mp4", "--keep", "01,04", "--reject=02"},
		{"--keep", "01,04", "ep.mp4", "--reject=02"},
		{"--keep", "01,04", "--reject=02", "ep.mp4"},
	} {
		episode, rest := splitArgs(argv)
		if episode != "ep.mp4" {
			t.Errorf("%v: the episode is %q", argv, episode)
		}
		if want := []string{"--keep", "01,04", "--reject=02"}; !slices.Equal(rest, want) {
			t.Errorf("%v: the flags are %v, want %v", argv, rest, want)
		}
	}
}

func TestAListOfClipsIsReadHoweverItIsSpaced(t *testing.T) {
	if got := ids(" 01, 04,,07 "); !slices.Equal(got, []string{"01", "04", "07"}) {
		t.Errorf("got %v", got)
	}
	if got := ids(""); len(got) != 0 {
		t.Errorf("nothing was read as %v", got)
	}
}
