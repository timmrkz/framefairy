package main

import (
	"errors"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"framefairy/updates"
)

// fakeGitHub answers what a test says GitHub knows. A commit missing from
// a map is one GitHub has nothing on. failing makes every call fail, the
// way they do when GitHub is down.
type fakeGitHub struct {
	main     string
	compares map[string]string // base...head to GitHub's word
	runs     map[string][2]string
	readies  map[string]string
	failing  bool
}

var errDown = errors.New("GitHub is down")

func (f fakeGitHub) mainCommit() (string, error) {
	if f.failing {
		return "", errDown
	}
	return f.main, nil
}

func (f fakeGitHub) compare(base, head string) (string, error) {
	if f.failing {
		return "", errDown
	}
	return f.compares[base+"..."+head], nil
}

func (f fakeGitHub) buildRun(commit string) (string, string, error) {
	if f.failing {
		return "", "", errDown
	}
	r := f.runs[commit]
	return r[0], r[1], nil
}

func (f fakeGitHub) ready(commit string) (string, error) {
	if f.failing {
		return "", errDown
	}
	return f.readies[commit], nil
}

// building is GitHub with every commit's build still running.
func building() fakeGitHub {
	return fakeGitHub{runs: map[string][2]string{"0123456789ab": {"in_progress", ""}}}
}

// A build is published for main or a pull request only when that
// channel's history holds its commit. A tag named main, pushed at any
// commit, reads to GitHub like a push to main, #838.
func TestOnlyACommitItsChannelHoldsIsPublished(t *testing.T) {
	t.Parallel()
	gh := fakeGitHub{
		main: "m2",
		compares: map[string]string{
			"m1...m2": "ahead", "m2...m2": "identical", "x...m2": "diverged",
			"p1...p2": "ahead", "p2...p2": "identical", "old...p2": "diverged",
		},
	}
	cases := []struct {
		name, channel, commit, head string
		refused                     string
	}{
		{"main's newest commit", "main", "m2", "", ""},
		{"an older commit of main, built late", "main", "m1", "", ""},
		{"a tag called main at another commit", "main", "x", "", "is not in main's history"},
		// main is always read from GitHub, whatever head is said.
		{"a tag called main that names its own commit as main's", "main", "x", "x", "is not in main's history"},
		{"a pull request's newest commit", "pr-7", "p2", "p2", ""},
		{"an older commit of a pull request", "pr-7", "p1", "p2", ""},
		{"a commit the pull request rewrote away", "pr-7", "old", "p2", "is not in the history of pr-7's newest commit"},
		{"a pull request with no newest commit said", "pr-7", "p2", "", "say the newest commit"},
	}
	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			err := held(gh, c.channel, c.commit, c.head)
			switch {
			case c.refused == "" && err != nil:
				t.Errorf("refused: %v", err)
			case c.refused != "" && (err == nil || !strings.Contains(err.Error(), c.refused)):
				t.Errorf("said %v, want it refused because it %s", err, c.refused)
			}
		})
	}
	down := fakeGitHub{failing: true}
	for _, channel := range []string{"main", "pr-7"} {
		if err := held(down, channel, "m2", "m2"); err == nil {
			t.Errorf("with GitHub down, %s's build was taken as %s's", channel, channel)
		}
	}
}

// A build that ended without one is no longer said to be coming, so the
// Updates page stops saying "A newer commit is being built", #840. Every
// doubt says it is coming, which is how it was before.
func TestABuildThatEndedIsNotComing(t *testing.T) {
	t.Parallel()
	cases := []struct {
		name  string
		run   [2]string
		ready string
		ended bool
	}{
		{"a build that failed", [2]string{"completed", "failure"}, "", true},
		{"a build cancelled by hand", [2]string{"completed", "cancelled"}, "", true},
		{"a build that timed out", [2]string{"completed", "timed_out"}, "", true},
		{"a build whose publishing failed", [2]string{"completed", "success"}, "failure", true},
		{"a build still running", [2]string{"in_progress", ""}, "", false},
		{"a build waiting for a runner", [2]string{"queued", ""}, "", false},
		{"a build that ended well, being published", [2]string{"completed", "success"}, "", false},
		{"a pull request's build already up", [2]string{"completed", "success"}, "success", false},
		{"a commit with no build yet", [2]string{}, "", false},
	}
	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			gh := fakeGitHub{runs: map[string][2]string{"c": c.run}, readies: map[string]string{"c": c.ready}}
			if got, why := ended(gh, "c"); got != c.ended {
				t.Errorf("said %v, %s, want %v", got, why, c.ended)
			}
		})
	}
	if got, _ := ended(fakeGitHub{failing: true}, "c"); got {
		t.Error("with GitHub down, a build was said to have ended")
	}
}

// The list leaves out the newest commit of a channel whose build ended
// without one, and keeps the build there is.
//
// On the real disk: the release tool reads the entries and writes the list as files.
func TestTheListSaysNoBuildIsComingAfterOneFailed(t *testing.T) {
	release(t)
	pr := signed(t, "pr-58", "#58 Small things")
	mainEntry := signed(t, "main", "main")
	full := "0123456789abcdef0123456789abcdef01234567"
	gh := fakeGitHub{runs: map[string][2]string{
		full: {"completed", "failure"},
		"fedcba9876543210fedcba9876543210fedcba98": {"in_progress", ""},
	}}
	out := filepath.Join(t.TempDir(), "channels.json")
	err := list([]string{"-out", out, "-newest", "pr-58=" + full,
		"-newest", "main=fedcba9876543210fedcba9876543210fedcba98", pr, mainEntry}, gh)
	if err != nil {
		t.Fatal(err)
	}
	data, err := os.ReadFile(out)
	if err != nil {
		t.Fatal(err)
	}
	l, err := updates.Parse(data)
	if err != nil {
		t.Fatal(err)
	}
	if b, _ := l.Find("pr-58"); b.Newest != "" {
		t.Errorf("pr-58's build failed, and the list still says %s is coming", b.Newest)
	}
	if b, _ := l.Find("main"); b.Newest != "fedcba987654" {
		t.Errorf("main's build is running, and the list says %q is coming, want its first twelve characters", b.Newest)
	}
}

// What the release tool asks GitHub's REST API, and how it reads the
// answers, against a server that answers the way GitHub does.
func TestGitHubIsAskedWhatTheToolNeeds(t *testing.T) {
	t.Parallel()
	answers := map[string]string{
		"/repos/o/r/git/ref/heads/main":                `{"object":{"sha":"m2"}}`,
		"/repos/o/r/compare/m1...m2":                   `{"status":"ahead"}`,
		"/repos/o/r/commits/c/status":                  `{"statuses":[{"context":"build","state":"success"},{"context":"Ready to update to","state":"failure"}]}`,
		"/repos/o/r/actions/workflows/builds.yml/runs": `{"workflow_runs":[{"status":"completed","conclusion":"cancelled"}]}`,
	}
	var auth, query string
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		auth = r.Header.Get("Authorization")
		if r.URL.Path == "/repos/o/r/actions/workflows/builds.yml/runs" {
			query = r.URL.RawQuery
		}
		a, ok := answers[r.URL.Path]
		if !ok {
			http.NotFound(w, r)
			return
		}
		_, _ = w.Write([]byte(a))
	}))
	t.Cleanup(srv.Close)
	gh := &restGitHub{api: srv.URL, repo: "o/r", token: "t", client: srv.Client()}

	if m, err := gh.mainCommit(); err != nil || m != "m2" {
		t.Errorf("main's commit read as %q, %v", m, err)
	}
	if auth != "Bearer t" {
		t.Errorf("the token went as %q", auth)
	}
	if s, err := gh.compare("m1", "m2"); err != nil || s != "ahead" {
		t.Errorf("the compare read as %q, %v", s, err)
	}
	if s, err := gh.ready("c"); err != nil || s != "failure" {
		t.Errorf("Ready to update to read as %q, %v", s, err)
	}
	if s, c, err := gh.buildRun("c"); err != nil || s != "completed" || c != "cancelled" {
		t.Errorf("the build read as %q %q, %v", s, c, err)
	}
	if query != "event=push&head_sha=c&per_page=1" {
		t.Errorf("the builds were asked for with %q", query)
	}
	if _, err := gh.compare("x", "y"); err == nil {
		t.Error("a compare GitHub had no answer to was read as one")
	}
}
