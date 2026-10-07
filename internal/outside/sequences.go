package outside

// The sequences make outside runs on a Mac: what starts outside the app,
// step by step, and what has to come of it. This file is the whole of what
// a sequence does. How a step is done is in run_test.go, and is the same
// for every sequence. See From outside the app in docs/TESTING.md.
//
// A step is a list, its verb first. The verbs that do something:
//
//   {"set up"}             makes this Mac one whose setup is done: a speech
//                          model in place and the question how clips are
//                          found answered. A new copy of the app holds a
//                          link until its setup is done
//   {"buy", n}             buys n seats at the local dispenser's checkout
//                          and reads the keys out of the letter on its dev
//                          page. The keys are numbered from 1, in the order
//                          bought, across every buy in the sequence
//   {"quit"}               quits the app if it runs, and waits until it is
//                          gone
//   {"open link", n}       hands key n's Unlock link to macOS, the way a
//                          browser or a mail app does once the link is
//                          clicked, and waits until the app has it. A closed
//                          app is started by it
//   {"open link", n, more} the same link with more after it, "&more=1"
//   {"bring forward", app} opens another app, "Calculator", and waits until
//                          it is in front
//   {"click", words}       presses the button on screen that says this
//
// and what has to come of them. Each waits until it holds, and the step
// fails when it does not within 20 seconds:
//
//   {"front", app}         this app is the one in front
//   {"head", text}         the Licence row is called this, "Licence key",
//                          or "Licensed" once a key is kept
//   {"field", n}           the Licence field holds key n, or nothing for
//                          "empty", which is also what a row with no field
//                          reads as
//   {"line", text}         the words under Licence key read this
//   {"line has", text}     the words under Licence key have this in them
//   {"mark", m}            the mark beside them is "ok", "err" or "none"
//   {"saved", yes}         a key is in the keychain, "yes" or "no"
//   {"waiting", yes}       a key from a link waits for the settings to take
//                          it, "yes" or "no"
//
// The sequences run in order on one Mac, and what one leaves behind, a key
// in the keychain or the setup, is there for the next. Each starts by
// quitting the app.

import "strings"

type step []string

type sequence struct {
	name  string
	steps []step
}

var sequences = []sequence{
	{
		name: "the Unlock link in the letter, with the app closed and open",
		steps: []step{
			{"set up"},
			{"buy", "2"},
			{"quit"},

			// The app is closed and no key is kept. The first key's Unlock
			// in the mail opens the app and unlocks it, with nothing to
			// press, and the settings say so.
			{"open link", "1"},
			{"front", "Frame Fairy"},
			{"saved", "yes"},
			{"head", "Licensed"},
			{"mark", "ok"},
			{"line has", "Unlocked from the link"},
			{"field", "empty"},

			// The same key again changes nothing.
			{"open link", "1"},
			{"line has", "already on this Mac"},

			// Another app in front, and the second key's Unlock. The app
			// comes back to the front with the second key in the field: a
			// key kept is never replaced by a link alone, because any page
			// can open one.
			{"bring forward", "Calculator"},
			{"open link", "2"},
			{"front", "Frame Fairy"},
			{"field", "2"},
			{"line has", "Unlock puts it in place"},
			{"click", "Unlock"},
			{"field", "empty"},
			{"line has", "a test key"},

			// A link with more in it than one key changes nothing.
			{"open link", "1", "&more=1"},
			{"field", "empty"},
			{"waiting", "no"},
		},
	},
}

// verbs is every verb a step can start with, and how many words may follow
// it, at least and at most.
var verbs = map[string][2]int{
	"set up":        {0, 0},
	"buy":           {1, 1},
	"quit":          {0, 0},
	"open link":     {1, 2},
	"bring forward": {1, 1},
	"click":         {1, 1},
	"front":         {1, 1},
	"head":          {1, 1},
	"field":         {1, 1},
	"line":          {1, 1},
	"line has":      {1, 1},
	"mark":          {1, 1},
	"saved":         {1, 1},
	"waiting":       {1, 1},
}

func (s step) String() string { return strings.Join(s, " ") }
