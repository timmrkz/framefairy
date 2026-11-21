package main

import (
	"errors"
	"flag"
	"fmt"
	"io"
	"path"
	"strconv"
	"strings"
	"time"
)

// A scheduled run fails on main where nobody is looking: no pull request
// waits on it and no session hears of it. So the run tells of itself, in
// one issue for each workflow, which a session finds among the open
// issues the way it finds any failing test on main. The first failure
// opens the issue, and each one after it, while the issue is open, adds a
// line to it as a comment, so the issue reads as the history of the
// nights that failed. Nothing closes it but the fix, because a run that
// passes the next night says nothing about why one failed.

// failedLabels are the labels the issue opens with: a failing test is
// critical, see Priority and size in docs/WORKFLOW.md, and one pull
// request is the size a fix usually is.
var failedLabels = []string{"tests", "priority/critical", "size/medium"}

// failedTitle is the issue's title for a workflow, which is how the next
// failure finds it again.
func failedTitle(workflow string) string { return workflow + " failed on main" }

// failedLine is the line a failure adds: when, the run, the commit and
// the jobs that failed, each with its link.
func failedLine(when time.Time, runURL, commit string, jobs []job) string {
	short := commit
	if len(short) > 7 {
		short = short[:7]
	}
	names := make([]string, 0, len(jobs))
	for _, j := range jobs {
		names = append(names, fmt.Sprintf("[`%s`](%s)", j.name, j.url))
	}
	failed := "no job says it failed, so read the run"
	if len(names) > 0 {
		failed = "failed " + strings.Join(names, ", ")
	}
	return fmt.Sprintf("- %s, [run](%s), commit %s: %s", when.UTC().Format("2006-01-02"), runURL, short, failed)
}

// failedBody is the issue a first failure opens, laid out the way
// .github/ISSUE_TEMPLATE/issue.md says.
func failedBody(workflow, line string) string {
	return workflow + " failed on main, in a run nobody was waiting on. The workflow opened this issue itself, and adds a comment with the run each time it fails again while the issue is open.\n\n" +
		"## Seen\n\n" + line + "\n\n" +
		"## Cause\n\n" +
		"Not known yet. The log of a failed job comes first: its last lines name the test that failed and what it printed.\n\n" +
		"Opened by `framefairy-release failed`, see cmd/framefairy-release/failed.go. Close it with the fix, the workflow never does.\n"
}

// failed tells of a run of workflow that failed: it opens the issue for
// the workflow, or adds the run to the one that is open.
func failed(gh gitHub, workflow, runURL, commit string, now time.Time, log io.Writer) error {
	run, err := strconv.ParseInt(path.Base(runURL), 10, 64)
	if err != nil || run <= 0 {
		return fmt.Errorf("-run %q is no address of a run, which ends in its number", runURL)
	}
	jobs, err := gh.failedJobs(run)
	if err != nil {
		return err
	}
	line := failedLine(now, runURL, commit, jobs)
	title := failedTitle(workflow)
	number, err := gh.openIssue(title)
	if err != nil {
		return err
	}
	if number != 0 {
		if err := gh.comment(number, "Failed again:\n\n"+line); err != nil {
			return err
		}
		fmt.Fprintf(log, "::error::%s failed again, told in issue #%d.\n", workflow, number)
		return nil
	}
	if number, err = gh.newIssue(title, failedBody(workflow, line), failedLabels); err != nil {
		return err
	}
	fmt.Fprintf(log, "::error::%s failed, told in issue #%d.\n", workflow, number)
	return nil
}

func failedCommand(args []string, gh gitHub, now time.Time, log io.Writer) error {
	fs := flag.NewFlagSet("failed", flag.ContinueOnError)
	workflow := fs.String("workflow", "", "the workflow's name, as GitHub shows it")
	run := fs.String("run", "", "the address of the run that failed")
	commit := fs.String("commit", "", "the commit it ran on")
	if err := fs.Parse(args); err != nil {
		return err
	}
	if *workflow == "" || *run == "" || *commit == "" {
		return errors.New("failed needs -workflow, -run and -commit")
	}
	return failed(gh, *workflow, *run, *commit, now, log)
}
