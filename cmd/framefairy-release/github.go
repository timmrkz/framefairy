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
	// mergeBase is the commit of main a commit branched from.
	mergeBase(head string) (string, error)
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
	// failedJobs are the jobs of a run that failed or ran out of time.
	failedJobs(run int64) ([]job, error)
	// openIssue is the number of the open issue with exactly this title
	// that a workflow opened, 0 when there is none.
	openIssue(title string) (int, error)
	// newIssue opens an issue and gives its number.
	newIssue(title, body string, labels []string) (int, error)
	// comment adds a comment to an issue.
	comment(number int, body string) error
	// editIssue writes an issue's body again.
	editIssue(number int, body string) error
	// closeIssue closes an issue as done, with a comment that says why.
	closeIssue(number int, why string) error
}

type job struct {
	name, url string
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
// the workflow's environment allow. Its files are those of one release,
// dev unless it says another.
type restGitHub struct {
	api, repo, token string
	client           *http.Client
	release          releaseInfo
}

// releaseInfo is a release the tool keeps files in, and how it is made
// when there is none yet.
type releaseInfo struct {
	tag, title, body string
}

// devBuilds is the release dev, the builds to update to.
var devBuilds = releaseInfo{
	tag: "dev", title: "Builds to update to",
	body: "The builds of main and of each open pull request, for the app to update itself to while it is being made. Not for customers. See docs/UPDATES.md.",
}

// toolBuilds is the release release-tool, main's signed build of this
// tool, see .github/workflows/release-tool.yml.
var toolBuilds = releaseInfo{
	tag: "release-tool", title: "Release tool",
	body: "The build of framefairy-release the workflows run, see .github/workflows/release-tool.yml.",
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
		client: &http.Client{Timeout: 5 * time.Minute}, release: devBuilds}
}

// toolReleaseFromEnv is GitHub as gitHubFromEnv, with the release
// release-tool in place of dev.
func toolReleaseFromEnv() *restGitHub {
	g := gitHubFromEnv()
	g.release = toolBuilds
	return g
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

func (g *restGitHub) mergeBase(head string) (string, error) {
	var c struct {
		MergeBase struct {
			SHA string `json:"sha"`
		} `json:"merge_base_commit"`
	}
	if err := g.get("compare/main..."+url.PathEscape(head), &c); err != nil {
		return "", err
	}
	if c.MergeBase.SHA == "" {
		return "", fmt.Errorf("GitHub names no commit of main that %s branched from", head)
	}
	return c.MergeBase.SHA, nil
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
	fields := map[string]string{"state": state, "context": readyContext, "description": description}
	// Only an address: GitHub's statuses take a link or none.
	if target != "" {
		fields["target_url"] = target
	}
	body, _ := json.Marshal(fields)
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

// a release as GitHub gives it.
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

func (g *restGitHub) current() (devRelease, error) {
	var r devRelease
	err := g.get("releases/tags/"+url.PathEscape(g.release.tag), &r)
	return r, err
}

func (g *restGitHub) assets() ([]asset, error) {
	r, err := g.current()
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
	r, err := g.current()
	if errors.Is(err, errNotFound) {
		body, _ := json.Marshal(map[string]any{
			"tag_name": g.release.tag, "target_commitish": "main", "name": g.release.title,
			"prerelease": true, "make_latest": "false", "body": g.release.body,
		})
		if err := g.call(http.MethodPost, "releases", bytes.NewReader(body), "application/json", nil); err != nil {
			// Made by a run beside this one, which is as good.
			if r, err = g.current(); err != nil {
				return err
			}
		} else if r, err = g.current(); err != nil {
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

func (g *restGitHub) failedJobs(run int64) ([]job, error) {
	var jobs struct {
		Jobs []struct {
			Name       string  `json:"name"`
			Conclusion *string `json:"conclusion"`
			URL        string  `json:"html_url"`
		} `json:"jobs"`
	}
	// latest leaves out the tries before a job was run again.
	if err := g.get("actions/runs/"+strconv.FormatInt(run, 10)+"/jobs?filter=latest&per_page=100", &jobs); err != nil {
		return nil, err
	}
	var failed []job
	for _, j := range jobs.Jobs {
		if j.Conclusion != nil && (*j.Conclusion == "failure" || *j.Conclusion == "timed_out") {
			failed = append(failed, job{name: j.Name, url: j.URL})
		}
	}
	return failed, nil
}

// workflowAuthor is who an issue a workflow opens with its own token
// belongs to. Only its issues are looked through, so an issue a person
// gave the same title is never written into.
const workflowAuthor = "github-actions[bot]"

func (g *restGitHub) openIssue(title string) (int, error) {
	for page := 1; ; page++ {
		var got []struct {
			Number int
			Title  string
			Pull   *struct{} `json:"pull_request"`
		}
		q := url.Values{"state": {"open"}, "creator": {workflowAuthor}, "per_page": {"100"}, "page": {strconv.Itoa(page)}}
		if err := g.get("issues?"+q.Encode(), &got); err != nil {
			return 0, err
		}
		for _, i := range got {
			if i.Pull == nil && i.Title == title {
				return i.Number, nil
			}
		}
		if len(got) < 100 {
			return 0, nil
		}
	}
}

func (g *restGitHub) newIssue(title, body string, labels []string) (int, error) {
	fields, _ := json.Marshal(map[string]any{"title": title, "body": body, "labels": labels})
	var made struct{ Number int }
	err := g.call(http.MethodPost, "issues", bytes.NewReader(fields), "application/json", &made)
	return made.Number, err
}

func (g *restGitHub) comment(number int, body string) error {
	fields, _ := json.Marshal(map[string]string{"body": body})
	return g.call(http.MethodPost, "issues/"+strconv.Itoa(number)+"/comments", bytes.NewReader(fields), "application/json", nil)
}

func (g *restGitHub) editIssue(number int, body string) error {
	fields, _ := json.Marshal(map[string]string{"body": body})
	return g.call(http.MethodPatch, "issues/"+strconv.Itoa(number), bytes.NewReader(fields), "application/json", nil)
}

func (g *restGitHub) closeIssue(number int, why string) error {
	if err := g.comment(number, why); err != nil {
		return err
	}
	fields, _ := json.Marshal(map[string]string{"state": "closed", "state_reason": "completed"})
	return g.call(http.MethodPatch, "issues/"+strconv.Itoa(number), bytes.NewReader(fields), "application/json", nil)
}
