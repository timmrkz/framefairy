package main

import (
	"bytes"
	"strings"
	"testing"
)

// Every flag of the command line keeps working, see CLAUDE.md. These are
// the rules parseArgs follows, argparse's, so that a command that worked
// once goes on working.

func TestTheCommandLineIsReadTheWayItAlwaysWas(t *testing.T) {
	cases := []struct {
		name string
		argv []string
		ok   func(p parsed) bool
	}{
		{"the source alone", []string{"ep.mp4"},
			func(p parsed) bool { return p.opts.Source == "ep.mp4" }},
		{"options before and after the source", []string{"--count", "4", "ep.mp4", "--plan-only"},
			func(p parsed) bool { return p.opts.Source == "ep.mp4" && p.opts.Count == 4 && p.opts.PlanOnly }},
		{"a value joined with =", []string{"--max=45", "ep.mp4"},
			func(p parsed) bool { return p.opts.Max == 45 }},
		{"a unique prefix is enough", []string{"--plan-o", "ep.mp4"},
			func(p parsed) bool { return p.opts.PlanOnly }},
		{"a negative number is a value, not an option", []string{"--silence-db", "-50", "ep.mp4"},
			func(p parsed) bool { return p.opts.SilenceDB != nil && *p.opts.SilenceDB == -50 }},
		{"a timecode", []string{"--from", "1:00:00", "--to", "1:30:00", "ep.mp4"},
			func(p parsed) bool { return p.opts.From == "1:00:00" && p.opts.To == "1:30:00" }},
		{"everything after -- is the source", []string{"--", "-odd-name.mp4"},
			func(p parsed) bool { return p.opts.Source == "-odd-name.mp4" }},
		{"both spellings of colour", []string{"--no-color", "ep.mp4"},
			func(p parsed) bool { return p.plain }},
		{"the short verbose", []string{"-v", "ep.mp4"},
			func(p parsed) bool { return p.verbose }},
		{"the events file", []string{"--events", "e.jsonl", "ep.mp4"},
			func(p parsed) bool { return p.events == "e.jsonl" }},
		{"help needs no source", []string{"-h"},
			func(p parsed) bool { return p.help }},
		{"nor does the version", []string{"--version"},
			func(p parsed) bool { return p.version }},
	}
	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			p, err := parseArgs(c.argv)
			if err != nil {
				t.Fatalf("%v: %v", c.argv, err)
			}
			if !c.ok(p) {
				t.Errorf("%v was read as %+v", c.argv, p.opts)
			}
		})
	}
}

func TestAMistakeInTheCommandLineSaysWhat(t *testing.T) {
	cases := []struct {
		name, want string
		argv       []string
	}{
		{"no source", "required: source", []string{"--plan-only"}},
		{"two sources", "unrecognized arguments: b.mp4", []string{"a.mp4", "b.mp4"}},
		{"an option nobody knows", "unrecognized arguments: --nope", []string{"--nope", "ep.mp4"}},
		{"a prefix that fits two", "ambiguous option", []string{"--no", "ep.mp4"}},
		{"a switch given a value", "ignored explicit argument", []string{"--replan=yes", "ep.mp4"}},
		{"a value missing", "expected one argument", []string{"ep.mp4", "--count"}},
		{"a value that is not a number", "invalid int value", []string{"--count", "four", "ep.mp4"}},
	}
	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			_, err := parseArgs(c.argv)
			if err == nil {
				t.Fatalf("%v was taken", c.argv)
			}
			if !strings.Contains(err.Error(), c.want) || !strings.HasPrefix(err.Error(), usageLine) {
				t.Errorf("%v: %q, want the usage line and %q", c.argv, err, c.want)
			}
		})
	}
}

// Every flag there is is in the help, so nothing is there that nobody can
// find.
func TestTheHelpNamesEveryFlag(t *testing.T) {
	var help bytes.Buffer
	printHelp(&help)
	for _, s := range specs() {
		for _, name := range s.names {
			if !strings.Contains(help.String(), name) {
				t.Errorf("%s is not in the help", name)
			}
		}
	}
	// Lines are wrapped to the terminal. A word longer than the column, the
	// speech model's folder, stands on a line of its own.
	for line := range strings.SplitSeq(help.String(), "\n") {
		if len(line) > 80 && len(strings.Fields(line)) > 1 {
			t.Errorf("a help line is %d characters: %q", len(line), line)
		}
	}
}
