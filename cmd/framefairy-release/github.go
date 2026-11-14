package main

import (
	"bytes"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net/http"
	"net/url"
	"os"
	"path"
	"strconv"
	"strings"
	"time"
)

// gitHub is everything the release tool asks of GitHub, as the workflows
// run it. The tests hand in a fake, so they need no network and no token.
type gitHub interface {
	// mainCommit is the commit main is at now, read as refs/heads/main.
	mainCommit() (string, error)
	// commit is the whole commit a shortened one names.
	commit(short string) (string, error)
	// compare is GitHub's word for how head stands to base: identical,
	// ahead when head has base in its history, behind or diverged.
	compare(base, head string) (string, error)
	// buildRun is the status and the conclusion of the newest run of
	// builds.yml on a push of the commit, both empty when it has none.
	buildRun(commit string) (status, conclusion string, err error)
	// ready is the state of the commit's Ready to update to, empty when
	// it has none.
	ready(commit string) (string, error)
	// setReady sets the commit's Ready to update to.
	setReady(commit, state, description, target string) error
	// pulls are the open pull requests from this repository's own
	// branches. A fork's are left out: a fork can have a branch of the
	// same name.
	pulls() ([]pull, error)
	// conflicts says whether an open pull request no longer merges into
	// main. GitHub works it out after a push, and says nothing until then.
	conflicts(number int) (bool, error)
	// builds is how many builds a run of builds.yml left, as artifacts
	// called build that have not expired.
	builds(run string) (int, error)
	// fileAt is a file of the repository at a commit.
	fileAt(name, commit string) ([]byte, error)
	// assets are the files of the release dev, none when there is no
	// such release yet.
	assets() ([]asset, error)
	// download is the content of a file of the release.
	download(a asset) ([]byte, error)
	// upload puts a file into the release dev, in place of one of the
	// same name, and makes the release first if there is none.
	upload(name string, data []byte) error
	// remove takes a file away from the release.
	remove(a asset) error
	// rename gives a file of the release another name.
	rename(a asset, name string) error
	// waiting are the runs of publish.yml that wait to enter the
	// environment updates, and since when.
	waiting() ([]waitingRun, error)
	// cancel stops a run of a workflow.
	cancel(run int64) error
}

type pull struct {
	number      int
	title, head string
	branch      string
}

type asset struct {
	id      int64
	name    string
	created time.Time
	updated time.Time
}

type waitingRun struct {
	id      int64
	created time.Time
}

// restGitHub asks GitHub's REST API, as the token and the repository in
// the workflow's environment allow.
type restGitHub struct {
	api, repo, token string
	client           *http.Client
}

// gitHubFromEnv is GitHub as the workflow hands it over: GH_TOKEN and
// GH_REPO, which every job that runs the tool sets.
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
		client: &http.Client{Timeout: 5 * time.Minute}}
}

// errNotFound is GitHub's 404, which for a release or a file means it is
// not there yet.
var errNotFound = errors.New("not found")

// call sends one request to the repository's part of the API, or to a
// whole address when where starts with https, and reads the answer into
// v when v is not nil.
func (g *restGitHub) call(method, where string, body io.Reader, kind string, v any) error {
	if g.repo == "" {
		return errors.New("GH_REPO is not set")
	}
	if !strings.HasPrefix(where, "https://") && !strings.HasPrefix(where, "http://") {
		where = g.api + "/repos/" + g.repo + "/" + where
	}
	req, err := http.NewRequest(method, where, body)
	if err != nil {
		return err
	}
	req.Header.Set("Accept", "application/vnd.github+json")
	if kind != "" {
		req.Header.Set("Content-Type", kind)
	}
	if g.token != "" {
		req.Header.Set("Authorization", "Bearer "+g.token)
	}
	resp, err := g.client.Do(req)
	if err != nil {
		return err
	}
	defer resp.Body.Close()
	if resp.StatusCode == http.StatusNotFound {
		return fmt.Errorf("%s %s: %w", method, where, errNotFound)
	}
	if resp.StatusCode/100 != 2 {
		said, _ := io.ReadAll(io.LimitReader(resp.Body, 512))
		return fmt.Errorf("GitHub answered %s to %s %s: %s", resp.Status, method, where, bytes.TrimSpace(said))
	}
	if v == nil {
		return nil
	}
	if b, ok := v.(*[]byte); ok {
		*b, err = io.ReadAll(resp.Body)
		return err
	}
	return json.NewDecoder(resp.Body).Decode(v)
}

func (g *restGitHub) get(where string, v any) error {
	return g.call(http.MethodGet, where, nil, "", v)
}

func (g *restGitHub) mainCommit() (string, error) {
	var ref struct {
		Object struct{ SHA string } `json:"object"`
	}
	// refs/heads/main by its whole name: a tag called main is the very
	// thing the check of a commit's channel is for.
	err := g.get("git/ref/heads/main", &ref)
	return ref.Object.SHA, err
}

func (g *restGitHub) commit(short string) (string, error) {
	var c struct{ SHA string }
	err := g.get("commits/"+url.PathEscape(short), &c)
	return c.SHA, err
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
		if st.Context == readyContext {
			return st.State, nil
		}
	}
	return "", nil
}

// readyContext is the name of the status a pull request reads to know
// its build is on the Updates page.
const readyContext = "Ready to update to"

func (g *restGitHub) setReady(commit, state, description, target string) error {
	body, _ := json.Marshal(map[string]string{
		"state": state, "context": readyContext, "description": description, "target_url": target,
	})
	return g.call(http.MethodPost, "statuses/"+url.PathEscape(commit), bytes.NewReader(body), "application/json", nil)
}

func (g *restGitHub) pulls() ([]pull, error) {
	var all []pull
	for page := 1; ; page++ {
		var got []struct {
			Number int
			Title  string
			Head   struct {
				Ref, SHA string
				Repo     *struct {
					FullName string `json:"full_name"`
				}
			}
		}
		if err := g.get("pulls?state=open&per_page=100&page="+strconv.Itoa(page), &got); err != nil {
			return nil, err
		}
		for _, p := range got {
			if p.Head.Repo == nil || p.Head.Repo.FullName != g.repo {
				continue
			}
			all = append(all, pull{number: p.Number, title: p.Title, head: p.Head.SHA, branch: p.Head.Ref})
		}
		if len(got) < 100 {
			return all, nil
		}
	}
}

func (g *restGitHub) conflicts(number int) (bool, error) {
	var p struct {
		State string `json:"mergeable_state"`
	}
	err := g.get("pulls/"+strconv.Itoa(number), &p)
	// dirty is GitHub's word for a pull request with a merge conflict.
	return p.State == "dirty", err
}

func (g *restGitHub) builds(run string) (int, error) {
	var a struct {
		Artifacts []struct {
			Name    string
			Expired bool
		}
	}
	if err := g.get("actions/runs/"+url.PathEscape(run)+"/artifacts?per_page=100", &a); err != nil {
		return 0, err
	}
	n := 0
	for _, x := range a.Artifacts {
		if x.Name == "build" && !x.Expired {
			n++
		}
	}
	return n, nil
}

func (g *restGitHub) fileAt(name, commit string) ([]byte, error) {
	var f struct {
		Content  string
		Encoding string
	}
	if err := g.get("contents/"+name+"?ref="+url.QueryEscape(commit), &f); err != nil {
		return nil, err
	}
	if f.Encoding != "base64" {
		return nil, fmt.Errorf("%s at %s came as %q", name, commit, f.Encoding)
	}
	return decodeBase64Lines(f.Content)
}

// release is the release dev, the one the builds live in.
type devRelease struct {
	ID        int64  `json:"id"`
	UploadURL string `json:"upload_url"`
	Assets    []struct {
		ID      int64     `json:"id"`
		Name    string    `json:"name"`
		Created time.Time `json:"created_at"`
		Updated time.Time `json:"updated_at"`
	} `json:"assets"`
}

func (g *restGitHub) release() (devRelease, error) {
	var r devRelease
	err := g.get("releases/tags/dev", &r)
	return r, err
}

func (g *restGitHub) assets() ([]asset, error) {
	r, err := g.release()
	if errors.Is(err, errNotFound) {
		return nil, nil
	}
	if err != nil {
		return nil, err
	}
	all := make([]asset, 0, len(r.Assets))
	for _, a := range r.Assets {
		all = append(all, asset{id: a.ID, name: a.Name, created: a.Created, updated: a.Updated})
	}
	return all, nil
}

func (g *restGitHub) download(a asset) ([]byte, error) {
	req, err := http.NewRequest(http.MethodGet, g.api+"/repos/"+g.repo+"/releases/assets/"+strconv.FormatInt(a.id, 10), nil)
	if err != nil {
		return nil, err
	}
	req.Header.Set("Accept", "application/octet-stream")
	if g.token != "" {
		req.Header.Set("Authorization", "Bearer "+g.token)
	}
	// GitHub answers with a redirect to where the file is stored, and Go
	// leaves the token behind when it follows one to another host.
	resp, err := g.client.Do(req)
	if err != nil {
		return nil, err
	}
	defer resp.Body.Close()
	if resp.StatusCode != http.StatusOK {
		return nil, fmt.Errorf("GitHub answered %s to the download of %s", resp.Status, a.name)
	}
	return io.ReadAll(resp.Body)
}

func (g *restGitHub) upload(name string, data []byte) error {
	r, err := g.release()
	if errors.Is(err, errNotFound) {
		body, _ := json.Marshal(map[string]any{
			"tag_name": "dev", "target_commitish": "main", "name": "Builds to update to",
			"prerelease": true, "make_latest": "false",
			"body": "The builds of main and of each open pull request, for the app to update itself to while it is being made. Not for customers. See docs/UPDATES.md.",
		})
		if err := g.call(http.MethodPost, "releases", bytes.NewReader(body), "application/json", nil); err != nil {
			// Made by a run beside this one, which is as good.
			if r, err = g.release(); err != nil {
				return err
			}
		} else if r, err = g.release(); err != nil {
			return err
		}
	} else if err != nil {
		return err
	}
	for _, a := range r.Assets {
		if a.Name == name {
			if err := g.remove(asset{id: a.ID, name: a.Name}); err != nil {
				return err
			}
		}
	}
	at, _, _ := strings.Cut(r.UploadURL, "{")
	kind := "application/octet-stream"
	switch path.Ext(name) {
	case ".json":
		kind = "application/json"
	case ".zip":
		kind = "application/zip"
	}
	return g.call(http.MethodPost, at+"?name="+url.QueryEscape(name), bytes.NewReader(data), kind, nil)
}

func (g *restGitHub) remove(a asset) error {
	err := g.call(http.MethodDelete, "releases/assets/"+strconv.FormatInt(a.id, 10), nil, "", nil)
	if errors.Is(err, errNotFound) {
		return nil
	}
	return err
}

func (g *restGitHub) rename(a asset, name string) error {
	body, _ := json.Marshal(map[string]string{"name": name})
	return g.call(http.MethodPatch, "releases/assets/"+strconv.FormatInt(a.id, 10), bytes.NewReader(body), "application/json", nil)
}

func (g *restGitHub) waiting() ([]waitingRun, error) {
	var runs struct {
		Runs []struct {
			ID      int64     `json:"id"`
			Created time.Time `json:"created_at"`
		} `json:"workflow_runs"`
	}
	if err := g.get("actions/workflows/publish.yml/runs?status=waiting&per_page=100", &runs); err != nil {
		return nil, err
	}
	all := make([]waitingRun, 0, len(runs.Runs))
	for _, r := range runs.Runs {
		all = append(all, waitingRun{id: r.ID, created: r.Created})
	}
	return all, nil
}

func (g *restGitHub) cancel(run int64) error {
	return g.call(http.MethodPost, "actions/runs/"+strconv.FormatInt(run, 10)+"/cancel", nil, "", nil)
}
