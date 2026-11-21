package main

import (
	"strings"
	"testing"
)

// A week with something behind opens the issue with the table, the next
// one writes it again with what is behind then, and a week with
// everything current closes it. A week with nothing open and nothing
// behind does nothing.
func TestTheIssueOfWhatIsBehindFollowsEachWeek(t *testing.T) {
	t.Parallel()
	gh := &fakeGitHub{}
	var log strings.Builder
	for _, week := range []struct {
		status int
		table  string
	}{{0, ""}, {1, "| ffmpeg | 8.0 | 8.1 |"}, {1, "| Node | 24.1 | 24.2 |"}, {0, ""}} {
		if err := behind(gh, week.status, week.table, night, &log); err != nil {
			t.Fatal(err)
		}
	}
	want := []string{
		"open #1 \"Newer releases of the tools the app ships\" [shipping priority/important size/small]\n" +
			"These are behind their newest release upstream, as of 2026-11-22. What reads a video somebody else made needs its fixes, so raise each pin where it is set, run make, and render a short before merging.\n\n" +
			"| ffmpeg | 8.0 | 8.1 |\n\nWritten by .github/workflows/security.yml through `framefairy-release behind`, and closed by it once everything is current.\n",
		"edit #1 These are behind their newest release upstream, as of 2026-11-22. What reads a video somebody else made needs its fixes, so raise each pin where it is set, run make, and render a short before merging.\n\n| Node | 24.1 | 24.2 |\n",
		"close #1 Everything the app ships is at its newest release again.",
	}
	if len(gh.did) != len(want) {
		t.Fatalf("GitHub was asked\n%s\nwant\n%s", strings.Join(gh.did, "\n"), strings.Join(want, "\n"))
	}
	for i, w := range want {
		if !strings.HasPrefix(gh.did[i], w) {
			t.Errorf("GitHub was asked\n%s\nwant it to start\n%s", gh.did[i], w)
		}
	}
}

// upstream.sh failing for another reason says nothing about what is
// behind, so the issue is left as it is and the run fails, and so does an
// answer of something behind with nothing named.
func TestAnUnknownAnswerLeavesTheIssueBe(t *testing.T) {
	t.Parallel()
	gh := &fakeGitHub{issues: map[string]int{behindTitle: 4}}
	for _, c := range []struct {
		status int
		table  string
	}{{2, ""}, {127, "sh: not found"}, {1, "  \n"}} {
		if err := behind(gh, c.status, c.table, night, &strings.Builder{}); err == nil {
			t.Errorf("status %d with %q was taken", c.status, c.table)
		}
	}
	if len(gh.did) != 0 {
		t.Errorf("GitHub was asked %q, want nothing", gh.did)
	}
}
