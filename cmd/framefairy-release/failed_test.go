package main

import (
	"strings"
	"testing"
	"time"
)

// night is when a nightly ran, for the line it adds.
var night = time.Date(2026, 11, 22, 2, 37, 0, 0, time.UTC)

// The first night a workflow fails opens its issue, with the run, the
// commit and the jobs that failed. Each night after it adds a line to
// that issue while it is open, and a run that failed with no job to
// blame still says where to look.
func TestAFailedNightOpensItsIssueAndTheNextAddsToIt(t *testing.T) {
	t.Parallel()
	gh := &fakeGitHub{jobs: map[int64][]job{
		101: {{name: "unit", url: "https://j/1"}, {name: "outside", url: "https://j/2"}},
	}}
	var log strings.Builder
	if err := failed(gh, "Nightly on a Mac", "https://github.com/o/r/actions/runs/101", "0123456789abcdef", night, &log); err != nil {
		t.Fatal(err)
	}
	if err := failed(gh, "Nightly on a Mac", "https://github.com/o/r/actions/runs/102", "fedcba9876543210", night.AddDate(0, 0, 1), &log); err != nil {
		t.Fatal(err)
	}
	if len(gh.did) != 2 {
		t.Fatalf("GitHub was asked %d things, want an issue and a comment:\n%s", len(gh.did), strings.Join(gh.did, "\n"))
	}
	opened, again := gh.did[0], gh.did[1]
	for _, want := range []string{
		`open #1 "Nightly on a Mac failed on main" [tests priority/critical size/medium]`,
		"## Seen\n\n- 2026-11-22, [run](https://github.com/o/r/actions/runs/101), commit 0123456: failed [`unit`](https://j/1), [`outside`](https://j/2)\n",
		"## Cause\n\nNot known yet.",
	} {
		if !strings.Contains(opened, want) {
			t.Errorf("the issue opened as\n%s\nwhich does not hold\n%s", opened, want)
		}
	}
	if want := "comment #1\nFailed again:\n\n- 2026-11-23, [run](https://github.com/o/r/actions/runs/102), commit fedcba9: no job says it failed, so read the run"; again != want {
		t.Errorf("the second night said\n%s\nwant\n%s", again, want)
	}
	if want := "::error::Nightly on a Mac failed, told in issue #1.\n::error::Nightly on a Mac failed again, told in issue #1.\n"; log.String() != want {
		t.Errorf("the log said\n%s\nwant\n%s", log.String(), want)
	}
}

// Each workflow has an issue of its own, so a fuzzing crash and a Mac
// failure are not one thing to fix.
func TestEachWorkflowFailsInAnIssueOfItsOwn(t *testing.T) {
	t.Parallel()
	gh := &fakeGitHub{issues: map[string]int{"Nightly fuzzing failed on main": 7}}
	if err := failed(gh, "Nightly on a Mac", "https://github.com/o/r/actions/runs/5", "c", night, &strings.Builder{}); err != nil {
		t.Fatal(err)
	}
	if len(gh.did) != 1 || !strings.HasPrefix(gh.did[0], `open #1 "Nightly on a Mac failed on main"`) {
		t.Errorf("GitHub was asked %q, want a new issue for the Mac", gh.did)
	}
}

// The command fails with GitHub down, so the run stays red and its log
// says why, and refuses an address that names no run.
func TestTellingAFailureFailsLoudly(t *testing.T) {
	t.Parallel()
	err := failed(&fakeGitHub{down: true}, "W", "https://github.com/o/r/actions/runs/5", "c", night, &strings.Builder{})
	if err == nil {
		t.Error("with GitHub down, the failure was said to be told")
	}
	if err := failedCommand([]string{"-workflow", "W", "-run", "https://github.com/o/r/actions/runs/", "-commit", "c"}, &fakeGitHub{}, night, &strings.Builder{}); err == nil {
		t.Error("an address with no run's number was taken")
	}
	if err := failedCommand([]string{"-workflow", "W"}, &fakeGitHub{}, night, &strings.Builder{}); err == nil {
		t.Error("a command with no run and no commit was taken")
	}
}
