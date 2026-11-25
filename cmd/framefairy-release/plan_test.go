package main

import (
	"os"
	"path/filepath"
	"slices"
	"strings"
	"testing"
)

// fakeTrees answers which files two commits differ in from a map, from
// and to joined by "..". A pair it does not know cannot be read.
type fakeTrees map[string][]string

func (f fakeTrees) changed(from, to string) ([]string, error) {
	files, ok := f[from+".."+to]
	if !ok {
		return nil, errNotFound
	}
	return files, nil
}

// source reads no Go, so a change that is not the docs is built.
func (f fakeTrees) source(string) (source, error) { return source{}, errNotFound }

// outputs reads a plan the way the workflow's next steps do.
func outputs(p plan) map[string]string {
	m := map[string]string{}
	for _, kv := range p.outputs {
		m[kv[0]] = kv[1]
	}
	return m
}

const doc = "package engine\n\n// Version is ...\nconst Version = \"0.3.0\"\n"

// What a push is a build of, and whether it needs one: builds.yml asks
// this before a Mac is started.
//
// On the real disk: the release tool reads the public key from its file in the repository.
func TestWhatAPushBuilds(t *testing.T) {
	release(t)
	commit := "0123456789abcdef0123456789abcdef01234567"
	gh := &fakeGitHub{open: []pull{{number: 7, title: "T", head: commit, branch: "b"}}}
	gh.put("channel-pr-7.json", []byte(`{"commit":"builtcommit1"}`), now)
	trees := fakeTrees{
		"builtcommit1.." + commit: {"engine/run.go"},
	}
	cases := []struct {
		name, event, branch string
		trees               fakeTrees
		want                map[string]string
	}{
		{"a push to main", "push", "main", trees, map[string]string{
			"build": "true", "channel": "main", "number": "", "sha": commit,
			"version": "0.3.0-main.0123456", "commit": "0123456789ab"}},
		{"a push to a pull request's branch that changes the app", "push", "b", trees, map[string]string{
			"build": "true", "channel": "pr-7", "number": "7", "sha": commit,
			"version": "0.3.0-pr7.0123456", "commit": "0123456789ab"}},
		{"a push of the docs alone", "push", "b", fakeTrees{"builtcommit1.." + commit: {"README.md"}},
			map[string]string{"build": "false"}},
		{"a push of the commit built", "push", "b", fakeTrees{"builtcommit1.." + commit: {}},
			map[string]string{"build": "false"}},
		{"a push whose change cannot be read", "push", "b", fakeTrees{}, map[string]string{
			"build": "true", "channel": "pr-7", "number": "7", "sha": commit,
			"version": "0.3.0-pr7.0123456", "commit": "0123456789ab"}},
		{"a push to a branch with no pull request", "push", "lonely", trees, map[string]string{"build": "false"}},
		{"a pull request that closes", "pull_request", "b", trees, map[string]string{"build": "false"}},
	}
	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			p, err := buildPlan(gh, c.trees, c.event, c.branch, commit, []byte(doc))
			if err != nil {
				t.Fatal(err)
			}
			if got := outputs(p); !equalMaps(got, c.want) {
				t.Errorf("planned %v, want %v", got, c.want)
			}
			if c.want["build"] == "false" && p.notice == "" {
				t.Error("nothing is built, and the run does not say why")
			}
		})
	}
	// Until the key is made, nothing is built.
	if err := os.WriteFile(filepath.Join(publicKeyFile), nil, 0o644); err != nil {
		t.Fatal(err)
	}
	if p, _ := buildPlan(gh, trees, "push", "main", commit, []byte(doc)); outputs(p)["build"] != "false" {
		t.Error("a push was built with no update key in the repository")
	}
}

// A build older than the one its channel has is not published: a run of
// an old commit, run again, or a tag named main at an old commit of main,
// would otherwise take the newer build away.
//
// On the real disk: the release tool reads the public key from its file in the repository.
func TestAnOlderBuildIsNotPublished(t *testing.T) {
	release(t)
	old, newer := "aaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaa", "cccccccccccccccccccccccccccccccccccccccc"
	gh := &fakeGitHub{
		main:     newer,
		zips:     map[string]int{"1": 1},
		compares: map[string]string{old + "..." + newer: "ahead", old + "...cccccccccccc": "ahead", newer + "..." + newer: "identical"},
		files:    map[string]string{"engine/doc.go@" + old: doc, "engine/doc.go@" + newer: doc},
	}
	gh.put("channel-main.json", []byte(`{"commit":"cccccccccccc"}`), now)
	got, err := publishPlan(gh, "1", "main", old)
	if err != nil || outputs(got)["run"] != "false" || !strings.Contains(got.notice, "newer build") {
		t.Errorf("an older commit of main planned %v, %v, %q", outputs(got), err, got.notice)
	}
	// The commit built already is published again: a rerun of a build
	// that did not reach the page.
	if got, err := publishPlan(gh, "1", "main", newer); err != nil || outputs(got)["run"] != "true" {
		t.Errorf("main's newest commit planned %v, %v", outputs(got), err)
	}
}

// A push whose channel's build cannot be read is built, with a notice
// that says why: a doubt about the build there is never leaves a commit
// without one.
//
// On the real disk: the release tool reads the public key from its file in the repository.
func TestAPushIsBuiltWhenItsBuildCannotBeRead(t *testing.T) {
	release(t)
	commit := "0123456789abcdef0123456789abcdef01234567"
	gh := assetsDown{&fakeGitHub{open: []pull{{number: 7, title: "T", head: commit, branch: "b"}}}}
	p, err := buildPlan(gh, fakeTrees{}, "push", "b", commit, []byte(doc))
	if err != nil {
		t.Fatal(err)
	}
	if outputs(p)["build"] != "true" || !strings.Contains(p.notice, "could not be read") {
		t.Errorf("planned %v with the notice %q", outputs(p), p.notice)
	}
}

type assetsDown struct{ *fakeGitHub }

func (assetsDown) assets() ([]asset, error) { return nil, errDown }

// What a finished build is published as, from GitHub's record of the run.
//
// On the real disk: the release tool reads the public key from its file in the repository.
func TestWhatABuildIsPublishedAs(t *testing.T) {
	release(t)
	m, p := "aaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaa", "bbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbb"
	gh := &fakeGitHub{
		main:     m,
		zips:     map[string]int{"1": 1, "2": 0},
		open:     []pull{{number: 7, title: "Fix it\r\nand more", head: p, branch: "b"}},
		compares: map[string]string{m + "..." + m: "identical", p + "..." + p: "identical"},
		files:    map[string]string{"engine/doc.go@" + m: doc, "engine/doc.go@" + p: doc},
	}
	got, err := publishPlan(gh, "1", "main", m)
	if err != nil {
		t.Fatal(err)
	}
	if want := map[string]string{"run": "true", "channel": "main", "name": "main", "version": "0.3.0-main.aaaaaaa",
		"commit": "aaaaaaaaaaaa", "zip": "app-main-0.3.0-main.aaaaaaa.zip"}; !equalMaps(outputs(got), want) {
		t.Errorf("main's build planned as %v, want %v", outputs(got), want)
	}
	got, err = publishPlan(gh, "1", "b", p)
	if err != nil {
		t.Fatal(err)
	}
	if want := map[string]string{"run": "true", "channel": "pr-7", "name": "#7 Fix it", "version": "0.3.0-pr7.bbbbbbb",
		"commit": "bbbbbbbbbbbb", "zip": "app-pr-7-0.3.0-pr7.bbbbbbb.zip"}; !equalMaps(outputs(got), want) {
		t.Errorf("a pull request's build planned as %v, want %v", outputs(got), want)
	}
	// Nothing to publish.
	for _, c := range []struct{ name, run, branch string }{
		{"a run that left no build", "2", "main"},
		{"a branch whose pull request closed", "1", "gone"},
	} {
		got, err := publishPlan(gh, c.run, c.branch, m)
		if err != nil || outputs(got)["run"] != "false" || got.notice == "" {
			t.Errorf("%s planned %v, %v, %q", c.name, outputs(got), err, got.notice)
		}
	}
	// A tag named main at a commit main does not hold fails the run, so
	// it says so on the run and on the commit.
	if _, err := publishPlan(gh, "1", "main", p); err == nil || !strings.Contains(err.Error(), "not in main's history") {
		t.Errorf("a tag named main at another commit was planned, %v", err)
	}
}

// The plan reaches the workflow's next steps as one line a value, and a
// title with a line break in it cannot add an output of its own.
//
// On the real disk: GitHub reads the outputs from a file it names.
func TestAPlanIsWrittenOneLineAValue(t *testing.T) {
	t.Parallel()
	var p plan
	p.set("run", "true")
	p.set("name", "#7 Fix it\nrun=false")
	file := filepath.Join(t.TempDir(), "outputs")
	var said strings.Builder
	if err := p.write(&said, file); err != nil {
		t.Fatal(err)
	}
	data, err := os.ReadFile(file)
	if err != nil {
		t.Fatal(err)
	}
	lines := strings.Split(strings.TrimSpace(string(data)), "\n")
	if !slices.Equal(lines, []string{"run=true", "name=#7 Fix it run=false"}) {
		t.Errorf("wrote %q", lines)
	}
}

func equalMaps(a, b map[string]string) bool {
	if len(a) != len(b) {
		return false
	}
	for k, v := range a {
		if w, ok := b[k]; !ok || w != v {
			return false
		}
	}
	return true
}
