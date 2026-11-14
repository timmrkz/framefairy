package main

import (
	"errors"
	"fmt"
	"io"
	"net/http"
	"net/http/httptest"
	"slices"
	"sort"
	"strings"
	"sync"
	"testing"
	"time"
)

// fakeGitHub answers what a test says GitHub knows, and keeps the release
// dev in memory. A commit missing from a map is one GitHub has nothing
// on. down makes every call fail, the way they do when GitHub is down.
type fakeGitHub struct {
	mu       sync.Mutex
	main     string
	shorts   map[string]string    // a shortened commit to its whole name
	compares map[string]string    // base...head to GitHub's word
	runs     map[string][2]string // a commit's newest build: status, conclusion
	readies  map[string]string    // a commit's Ready to update to
	open     []pull
	dirty    map[int]bool
	zips     map[string]int // a run of builds.yml to the builds it left
	files    map[string]string
	release  map[string]asset
	data     map[string][]byte
	stuck    []waitingRun
	down     bool
	// What was done, in order, for the tests to read.
	did []string
	ids int64
}

var errDown = errors.New("GitHub is down")

func (f *fakeGitHub) failing() error {
	if f.down {
		return errDown
	}
	return nil
}

func (f *fakeGitHub) mainCommit() (string, error) { return f.main, f.failing() }

func (f *fakeGitHub) commit(short string) (string, error) {
	if s, ok := f.shorts[short]; ok {
		return s, f.failing()
	}
	return "", errors.Join(f.failing(), errNotFound)
}

func (f *fakeGitHub) compare(base, head string) (string, error) {
	return f.compares[base+"..."+head], f.failing()
}

func (f *fakeGitHub) buildRun(commit string) (string, string, error) {
	r := f.runs[commit]
	return r[0], r[1], f.failing()
}

func (f *fakeGitHub) ready(commit string) (string, error) { return f.readies[commit], f.failing() }

func (f *fakeGitHub) setReady(commit, state, description, target string) error {
	f.mu.Lock()
	defer f.mu.Unlock()
	if f.readies == nil {
		f.readies = map[string]string{}
	}
	f.readies[commit] = state
	f.did = append(f.did, fmt.Sprintf("mark %s %s: %s %s", commit, state, description, target))
	return f.failing()
}

func (f *fakeGitHub) pulls() ([]pull, error) { return f.open, f.failing() }

func (f *fakeGitHub) conflicts(number int) (bool, error) { return f.dirty[number], f.failing() }

func (f *fakeGitHub) builds(run string) (int, error) { return f.zips[run], f.failing() }

func (f *fakeGitHub) fileAt(name, commit string) ([]byte, error) {
	if d, ok := f.files[name+"@"+commit]; ok {
		return []byte(d), f.failing()
	}
	return nil, errNotFound
}

func (f *fakeGitHub) assets() ([]asset, error) {
	f.mu.Lock()
	defer f.mu.Unlock()
	var all []asset
	for _, a := range f.release {
		all = append(all, a)
	}
	sort.Slice(all, func(i, j int) bool { return all[i].name < all[j].name })
	return all, f.failing()
}

func (f *fakeGitHub) download(a asset) ([]byte, error) {
	f.mu.Lock()
	defer f.mu.Unlock()
	return f.data[a.name], f.failing()
}

// put is a file in the release, made at a time the test says.
func (f *fakeGitHub) put(name string, data []byte, at time.Time) {
	f.mu.Lock()
	defer f.mu.Unlock()
	if f.release == nil {
		f.release, f.data = map[string]asset{}, map[string][]byte{}
	}
	f.ids++
	f.release[name] = asset{id: f.ids, name: name, created: at, updated: at}
	f.data[name] = data
}

// now is when the fake says a file is uploaded.
var now = time.Date(2026, 11, 14, 12, 0, 0, 0, time.UTC)

func (f *fakeGitHub) upload(name string, data []byte) error {
	if err := f.failing(); err != nil {
		return err
	}
	f.put(name, data, now)
	f.mu.Lock()
	f.did = append(f.did, "upload "+name)
	f.mu.Unlock()
	return nil
}

func (f *fakeGitHub) remove(a asset) error {
	f.mu.Lock()
	defer f.mu.Unlock()
	delete(f.release, a.name)
	delete(f.data, a.name)
	f.did = append(f.did, "remove "+a.name)
	return f.failing()
}

func (f *fakeGitHub) rename(a asset, name string) error {
	f.mu.Lock()
	defer f.mu.Unlock()
	f.release[name], f.data[name] = asset{id: a.id, name: name, created: a.created, updated: now}, f.data[a.name]
	delete(f.release, a.name)
	delete(f.data, a.name)
	f.did = append(f.did, "rename "+a.name+" "+name)
	return f.failing()
}

func (f *fakeGitHub) waiting() ([]waitingRun, error) { return f.stuck, f.failing() }

func (f *fakeGitHub) cancel(run int64) error {
	f.did = append(f.did, fmt.Sprint("cancel ", run))
	return f.failing()
}

// A build is published for main or a pull request only when that
// channel's history holds its commit. A tag named main, pushed at any
// commit, reads to GitHub like a push to main, #838.
func TestOnlyACommitItsChannelHoldsIsPublished(t *testing.T) {
	t.Parallel()
	gh := &fakeGitHub{
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
	down := &fakeGitHub{down: true}
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
			gh := &fakeGitHub{runs: map[string][2]string{"c": c.run}, readies: map[string]string{"c": c.ready}}
			if got, why := ended(gh, "c"); got != c.ended {
				t.Errorf("said %v, %s, want %v", got, why, c.ended)
			}
		})
	}
	if got, _ := ended(&fakeGitHub{down: true}, "c"); got {
		t.Error("with GitHub down, a build was said to have ended")
	}
}

// What the release tool asks GitHub's REST API, and how it reads the
// answers, against a server that answers the way GitHub does, so the
// fake the other tests use stands for something real.
func TestGitHubIsAskedWhatTheToolNeeds(t *testing.T) {
	t.Parallel()
	var mu sync.Mutex
	var heard []string
	var srv *httptest.Server
	srv = httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		body, _ := io.ReadAll(r.Body)
		mu.Lock()
		heard = append(heard, strings.TrimSpace(r.Method+" "+r.URL.RequestURI()+" "+r.Header.Get("Content-Type")+" "+string(body)))
		mu.Unlock()
		if r.Header.Get("Authorization") != "Bearer t" && !strings.HasPrefix(r.URL.Path, "/stored/") {
			http.Error(w, "no token", http.StatusUnauthorized)
			return
		}
		answer := map[string]string{
			"/repos/o/r/git/ref/heads/main":                `{"object":{"sha":"m2"}}`,
			"/repos/o/r/commits/abc":                       `{"sha":"abcdef"}`,
			"/repos/o/r/compare/m1...m2":                   `{"status":"ahead"}`,
			"/repos/o/r/commits/c/status":                  `{"statuses":[{"context":"build","state":"success"},{"context":"Ready to update to","state":"failure"}]}`,
			"/repos/o/r/actions/workflows/builds.yml/runs": `{"workflow_runs":[{"status":"completed","conclusion":"cancelled"}]}`,
			"/repos/o/r/pulls": `[{"number":7,"title":"T","head":{"ref":"b","sha":"h","repo":{"full_name":"o/r"}}},` +
				`{"number":8,"title":"F","head":{"ref":"b","sha":"f","repo":{"full_name":"fork/r"}}},` +
				`{"number":9,"title":"G","head":{"ref":"g","sha":"g","repo":null}}]`,
			"/repos/o/r/pulls/7":                            `{"mergeable_state":"dirty"}`,
			"/repos/o/r/actions/runs/5/artifacts":           `{"artifacts":[{"name":"build","expired":false},{"name":"build","expired":true},{"name":"logs","expired":false}]}`,
			"/repos/o/r/contents/engine/doc.go":             `{"encoding":"base64","content":"Y29uc3QgVmVyc2lv\nbiA9ICIwLjMuMCIK\n"}`,
			"/repos/o/r/releases/tags/dev":                  `{"id":1,"upload_url":"` + srv.URL + `/uploads/1/assets{?name,label}","assets":[{"id":3,"name":"channels.json","created_at":"2026-11-14T10:00:00Z","updated_at":"2026-11-14T11:00:00Z"}]}`,
			"/repos/o/r/actions/workflows/publish.yml/runs": `{"workflow_runs":[{"id":44,"created_at":"2026-11-14T10:00:00Z"}]}`,
			"/stored/3": `the list`,
		}[r.URL.Path]
		switch {
		case r.URL.Path == "/repos/o/r/releases/assets/3" && r.Method == http.MethodGet:
			http.Redirect(w, r, srv.URL+"/stored/3", http.StatusFound)
		case r.Method != http.MethodGet:
			w.WriteHeader(http.StatusCreated)
			_, _ = w.Write([]byte("{}"))
		case answer == "":
			http.NotFound(w, r)
		default:
			_, _ = w.Write([]byte(answer))
		}
	}))
	t.Cleanup(srv.Close)
	gh := &restGitHub{api: srv.URL, repo: "o/r", token: "t", client: srv.Client()}

	check := func(what string, got, want any, err error) {
		t.Helper()
		if err != nil || fmt.Sprint(got) != fmt.Sprint(want) {
			t.Errorf("%s read as %v, %v, want %v", what, got, err, want)
		}
	}
	m, err := gh.mainCommit()
	check("main's commit", m, "m2", err)
	c, err := gh.commit("abc")
	check("a shortened commit", c, "abcdef", err)
	s, err := gh.compare("m1", "m2")
	check("the compare", s, "ahead", err)
	s, err = gh.ready("c")
	check("Ready to update to", s, "failure", err)
	st, con, err := gh.buildRun("c")
	check("the build", st+" "+con, "completed cancelled", err)
	ps, err := gh.pulls()
	check("the open pull requests of this repository", ps, []pull{{number: 7, title: "T", head: "h", branch: "b"}}, err)
	d, err := gh.conflicts(7)
	check("a conflict", d, true, err)
	n, err := gh.builds("5")
	check("the builds a run left", n, 1, err)
	doc, err := gh.fileAt("engine/doc.go", "c")
	check("a file at a commit", string(doc), "const Version = \"0.3.0\"\n", err)
	as, err := gh.assets()
	check("the release's files", len(as), 1, err)
	b, err := gh.download(as[0])
	check("a file of the release, where it is stored", string(b), "the list", err)
	w, err := gh.waiting()
	check("the publish runs that wait", len(w), 1, err)
	if _, err := gh.compare("x", "y"); !errors.Is(err, errNotFound) {
		t.Errorf("a compare GitHub had no answer to said %v", err)
	}

	heard = nil
	for _, err := range []error{
		gh.setReady("c", "success", "On the Updates page as pr-7", ""),
		gh.upload("channels.json", []byte("new")),
		gh.rename(as[0], "channels-a.json"),
		gh.cancel(44),
	} {
		if err != nil {
			t.Error(err)
		}
	}
	want := []string{
		`POST /repos/o/r/statuses/c application/json {"context":"Ready to update to","description":"On the Updates page as pr-7","state":"success","target_url":""}`,
		`GET /repos/o/r/releases/tags/dev`,
		// The file of the same name goes first, as gh release upload
		// --clobber did.
		`DELETE /repos/o/r/releases/assets/3`,
		`POST /uploads/1/assets?name=channels.json application/json new`,
		`PATCH /repos/o/r/releases/assets/3 application/json {"name":"channels-a.json"}`,
		`POST /repos/o/r/actions/runs/44/cancel`,
	}
	if !slices.Equal(heard, want) {
		t.Errorf("GitHub heard\n  %s\nwant\n  %s", strings.Join(heard, "\n  "), strings.Join(want, "\n  "))
	}
}
