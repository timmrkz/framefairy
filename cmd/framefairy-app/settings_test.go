package main

import "testing"

// The app and the framefairy it makes are picked out in two colours, and a
// settings file may carry anything at all, so what comes out of one is
// always a colour.
func TestTheColoursAreAlwaysColours(t *testing.T) {
	cases := []struct {
		what       string
		in         Settings
		app, words string
	}{
		{"a file from before there were two colours",
			Settings{HighlightColour: wasDefaultColour}, defaultColour, defaultColour},
		{"the old default follows the new one, because nobody chose it",
			Settings{HighlightColour: "#b4236f"}, defaultColour, defaultColour},
		{"a colour somebody did choose is left alone",
			Settings{HighlightColour: "#00FF88", AppColour: "#123456"}, "#123456", "#00FF88"},
		{"nothing at all",
			Settings{}, defaultColour, defaultColour},
		{"something that is not a colour",
			Settings{HighlightColour: "rm -rf /", AppColour: "javascript:x"}, defaultColour, defaultColour},
		{"a colour without its hash",
			Settings{HighlightColour: "00FF88", AppColour: "123456"}, "123456", "00FF88"},
	}
	for _, c := range cases {
		set := c.in
		set.tidy()
		if set.AppColour != c.app || set.HighlightColour != c.words {
			t.Errorf("%s: app %q words %q, wanted %q and %q",
				c.what, set.AppColour, set.HighlightColour, c.app, c.words)
		}
	}
}
