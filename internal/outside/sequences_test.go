package outside

import (
	"slices"
	"strconv"
	"testing"
)

// The sequences are read on every machine, in make unit, so a step that
// is misspelt, has a word too few or names a key nobody bought fails here
// rather than on the Mac at the end of a CI run.
func TestEveryStepIsOneTheRunnerKnows(t *testing.T) {
	names := map[string]bool{}
	for _, seq := range sequences {
		if seq.name == "" || names[seq.name] {
			t.Errorf("a sequence needs a name of its own: %q", seq.name)
		}
		names[seq.name] = true
		if len(seq.steps) == 0 {
			t.Errorf("%s has no steps", seq.name)
		}
		bought := 0
		for i, s := range seq.steps {
			at := func(format string, a ...any) {
				t.Errorf("%s, step %d, %v: "+format, append([]any{seq.name, i + 1, s}, a...)...)
			}
			if len(s) == 0 {
				at("an empty step")
				continue
			}
			n, ok := verbs[s[0]]
			if !ok {
				at("no such verb")
				continue
			}
			if words := len(s) - 1; words < n[0] || words > n[1] {
				at("takes %d to %d words after the verb, not %d", n[0], n[1], words)
				continue
			}
			switch s[0] {
			case "buy":
				seats, err := strconv.Atoi(s[1])
				if err != nil || seats < 1 {
					at("buys a number of seats")
				}
				bought += seats
			case "open link":
				if k, err := strconv.Atoi(s[1]); err != nil || k < 1 || k > bought {
					at("names key %s, and %d were bought before it", s[1], bought)
				}
			case "field":
				if s[1] != "empty" {
					if k, err := strconv.Atoi(s[1]); err != nil || k < 1 || k > bought {
						at("names key %s, and %d were bought before it", s[1], bought)
					}
				}
			case "mark":
				if !slices.Contains([]string{"ok", "err", "none"}, s[1]) {
					at("a mark is ok, err or none")
				}
			case "saved", "waiting":
				if s[1] != "yes" && s[1] != "no" {
					at("yes or no")
				}
			}
		}
	}
}

// A sequence fails the way it is read: the step that broke, as written.
func TestAStepReadsAsWritten(t *testing.T) {
	if got := (step{"open link", "1", "&more=1"}).String(); got != "open link 1 &more=1" {
		t.Errorf("%q", got)
	}
}
