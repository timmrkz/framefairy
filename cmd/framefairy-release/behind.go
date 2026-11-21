package main

import (
	"errors"
	"flag"
	"fmt"
	"io"
	"os"
	"strings"
	"time"
)

// The weekly look at what the app ships but we did not write, ffmpeg and
// the libraries it is built with, llama.cpp, the speech library and Node,
// keeps one issue open while any of them is behind its newest release,
// and closes it once all are current. scripts/upstream.sh --behind
// compares them, and this says what its answer means for the issue.

// behindTitle is the issue's title, which is how the next week finds it.
const behindTitle = "Newer releases of the tools the app ships"

// behindLabels are the labels the issue opens with: a fix in what reads
// a video somebody else made is soon, and a pin raised is one place.
var behindLabels = []string{"shipping", "priority/important", "size/small"}

func behindBody(table string, now time.Time) string {
	return "These are behind their newest release upstream, as of " + now.UTC().Format("2006-01-02") +
		". What reads a video somebody else made needs its fixes, so raise each pin where it is set, run make, and render a short before merging.\n\n" +
		strings.TrimSpace(table) + "\n\n" +
		"Written by .github/workflows/security.yml through `framefairy-release behind`, and closed by it once everything is current.\n"
}

// behind keeps the issue as upstream.sh found things: status 0 is
// everything current, which closes an open issue, and status 1 is
// something behind, listed in table, which opens the issue or writes it
// again. Any other status is upstream.sh failing, which says nothing about
// what is behind, so it leaves the issue as it is and fails.
func behind(gh gitHub, status int, table string, now time.Time, log io.Writer) error {
	if status != 0 && status != 1 {
		return fmt.Errorf("scripts/upstream.sh ended with %d, so what is behind is not known", status)
	}
	if status == 1 && strings.TrimSpace(table) == "" {
		return errors.New("scripts/upstream.sh says something is behind and names nothing")
	}
	number, err := gh.openIssue(behindTitle)
	if err != nil {
		return err
	}
	switch {
	case status == 0 && number == 0:
		fmt.Fprintln(log, "Everything the app ships is at its newest release.")
		return nil
	case status == 0:
		fmt.Fprintf(log, "Everything the app ships is at its newest release again, so issue #%d is closed.\n", number)
		return gh.closeIssue(number, "Everything the app ships is at its newest release again.")
	case number != 0:
		fmt.Fprintf(log, "Issue #%d lists what is behind now.\n", number)
		return gh.editIssue(number, behindBody(table, now))
	}
	number, err = gh.newIssue(behindTitle, behindBody(table, now), behindLabels)
	if err == nil {
		fmt.Fprintf(log, "Issue #%d lists what is behind.\n", number)
	}
	return err
}

func behindCommand(args []string, gh gitHub, now time.Time, log io.Writer) error {
	fs := flag.NewFlagSet("behind", flag.ContinueOnError)
	status := fs.Int("status", -1, "how scripts/upstream.sh --behind ended")
	tableFile := fs.String("table", "", "the file it wrote what is behind to")
	if err := fs.Parse(args); err != nil {
		return err
	}
	if *tableFile == "" {
		return errors.New("behind needs -status and -table")
	}
	table, err := os.ReadFile(*tableFile)
	if err != nil {
		return err
	}
	return behind(gh, *status, string(table), now, log)
}
