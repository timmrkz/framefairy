package main

import (
	"cmp"
	"encoding/json"
	"errors"
	"fmt"
	"net/http"
	"net/url"
	"os"
	"strings"
	"time"
)

// gitHub is what the release tool asks GitHub, as the publish workflow
// runs it. The tests hand in a fake, so they need no network and no
// token.
type gitHub interface {
	// mainCommit is the commit main is at now.
	mainCommit() (string, error)
	// compare is GitHub's word for how head stands to base: identical,
	// ahead when head has base in its history, behind or diverged.
	compare(base, head string) (string, error)
	// buildRun is the status and the conclusion of the newest run of
	// builds.yml on a push of the commit, both empty when it has none.
	buildRun(commit string) (status, conclusion string, err error)
	// ready is the state of the commit's Ready to update to, empty when
	// it has none.
	ready(commit string) (string, error)
}

// restGitHub asks GitHub's REST API, as the token and the repository in
// the workflow's environment allow.
type restGitHub struct {
	api, repo, token string
	client           *http.Client
}

// gitHubFromEnv is GitHub as the workflow hands it over: GH_TOKEN and
// GH_REPO, which every job of publish.yml sets.
func gitHubFromEnv() *restGitHub {
	api := os.Getenv("GITHUB_API_URL")
	if api == "" {
		api = "https://api.github.com"
	}
	repo := os.Getenv("GH_REPO")
	if repo == "" {
		repo = os.Getenv("GITHUB_REPOSITORY")
	}
	return &restGitHub{api: api, repo: repo, token: os.Getenv("GH_TOKEN"),
		client: &http.Client{Timeout: 30 * time.Second}}
}

func (g *restGitHub) get(path string, v any) error {
	if g.repo == "" {
		return errors.New("GH_REPO is not set")
	}
	req, err := http.NewRequest(http.MethodGet, g.api+"/repos/"+g.repo+"/"+path, nil)
	if err != nil {
		return err
	}
	req.Header.Set("Accept", "application/vnd.github+json")
	if g.token != "" {
		req.Header.Set("Authorization", "Bearer "+g.token)
	}
	resp, err := g.client.Do(req)
	if err != nil {
		return err
	}
	defer resp.Body.Close()
	if resp.StatusCode != http.StatusOK {
		return fmt.Errorf("GitHub answered %s to %s", resp.Status, path)
	}
	return json.NewDecoder(resp.Body).Decode(v)
}

func (g *restGitHub) mainCommit() (string, error) {
	var ref struct {
		Object struct{ SHA string } `json:"object"`
	}
	// refs/heads/main by its full name: a tag called main is the very
	// thing the check is for.
	err := g.get("git/ref/heads/main", &ref)
	return ref.Object.SHA, err
}

func (g *restGitHub) compare(base, head string) (string, error) {
	var c struct{ Status string }
	err := g.get("compare/"+url.PathEscape(base)+"..."+url.PathEscape(head), &c)
	return c.Status, err
}

func (g *restGitHub) buildRun(commit string) (string, string, error) {
	var runs struct {
		Runs []struct {
			Status     string  `json:"status"`
			Conclusion *string `json:"conclusion"`
		} `json:"workflow_runs"`
	}
	// GitHub lists the newest run first, and a run run again is still
	// one run.
	q := url.Values{"head_sha": {commit}, "event": {"push"}, "per_page": {"1"}}
	if err := g.get("actions/workflows/builds.yml/runs?"+q.Encode(), &runs); err != nil {
		return "", "", err
	}
	if len(runs.Runs) == 0 {
		return "", "", nil
	}
	r := runs.Runs[0]
	if r.Conclusion == nil {
		return r.Status, "", nil
	}
	return r.Status, *r.Conclusion, nil
}

func (g *restGitHub) ready(commit string) (string, error) {
	var s struct {
		Statuses []struct{ Context, State string }
	}
	if err := g.get("commits/"+url.PathEscape(commit)+"/status", &s); err != nil {
		return "", err
	}
	for _, st := range s.Statuses {
		if st.Context == "Ready to update to" {
			return st.State, nil
		}
	}
	return "", nil
}

// held says whether a build of commit belongs to its channel: main's
// history holds it, or the history of the pull request's newest commit,
// head. GitHub reports a tag pushed with the name main, or with a pull
// request's branch, as a push to that branch, at any commit, so the
// record of the run alone is not enough.
func held(gh gitHub, channel, commit, head string) error {
	if channel == "main" {
		var err error
		if head, err = gh.mainCommit(); err != nil {
			return fmt.Errorf("main's commit cannot be read, so %s is not known to be main's: %w", commit, err)
		}
	}
	if head == "" {
		return fmt.Errorf("say the newest commit of %s, so %s can be compared with it", channel, commit)
	}
	status, err := gh.compare(commit, head)
	if err != nil {
		return fmt.Errorf("%s cannot be compared with %s's newest commit, so it is not known to be %s's: %w", commit, channel, channel, err)
	}
	if status != "identical" && status != "ahead" {
		if channel == "main" {
			return fmt.Errorf("%s is not in main's history, so it is not signed as main's build. A tag named main may have been pushed: look at the tags of the repository", commit)
		}
		// A branch rewritten after the push, as Dependabot does when it
		// rebases, is the usual reason. Its newer push builds by itself.
		return fmt.Errorf("%s is not in the history of %s's newest commit, %s, so it is not published for it", commit, channel, head)
	}
	return nil
}

// ended says whether the build of a commit ended without a build to
// update to, and why: it failed, it was cancelled, or its publishing
// failed and marked the commit. Then no build is coming until the next
// push, and the list must not say one is. Every doubt answers that one
// is still coming, which is how it was before.
func ended(gh gitHub, commit string) (bool, string) {
	state, err := gh.ready(commit)
	if err != nil {
		return false, fmt.Sprintf("the statuses of %s cannot be read: %v", commit, err)
	}
	if state == "failure" {
		return true, commit + " was not published, see its Ready to update to"
	}
	status, conclusion, err := gh.buildRun(commit)
	switch {
	case err != nil:
		return false, fmt.Sprintf("the builds of %s cannot be read: %v", commit, err)
	case status != "completed":
		return false, fmt.Sprintf("the build of %s is %s", commit, strings.ReplaceAll(cmp.Or(status, "not there yet"), "_", " "))
	case conclusion == "success":
		return false, "the build of " + commit + " ended well and is being published"
	default:
		return true, "the build of " + commit + " ended as " + strings.ReplaceAll(conclusion, "_", " ")
	}
}
