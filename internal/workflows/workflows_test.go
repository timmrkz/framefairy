package workflows

import (
	"fmt"
	"io/fs"
	"os"
	"path"
	"path/filepath"
	"regexp"
	"strings"
	"testing"

	"gopkg.in/yaml.v3"
)

// The files GitHub runs: every workflow and the setup the CI jobs share.
func actionFiles(t *testing.T) []string {
	t.Helper()
	root := filepath.Join("..", "..", ".github")
	var files []string
	for _, ext := range []string{"*.yml", "*.yaml"} {
		found, err := filepath.Glob(filepath.Join(root, "workflows", ext))
		if err != nil {
			t.Fatal(err)
		}
		files = append(files, found...)
	}
	if len(files) == 0 {
		t.Fatal("found no workflow in .github/workflows")
	}
	actions, err := filepath.Glob(filepath.Join(root, "actions", "*", "action.yml"))
	if err != nil {
		t.Fatal(err)
	}
	return append(files, actions...)
}

// Every workflow reads as YAML. Most of them run only on main, at night or
// by hand, so a broken one would otherwise be seen only after the merge.
//
// On the real disk: what is read is the repository's own workflows.
func TestEveryWorkflowReads(t *testing.T) {
	t.Parallel()
	for _, f := range actionFiles(t) {
		b, err := os.ReadFile(f)
		if err != nil {
			t.Fatal(err)
		}
		var doc map[string]any
		if err := yaml.Unmarshal(b, &doc); err != nil {
			t.Errorf("%s does not read as YAML: %v", filepath.Base(f), err)
			continue
		}
		if doc["jobs"] == nil && doc["runs"] == nil {
			t.Errorf("%s has no jobs", filepath.Base(f))
		}
	}
}

var (
	uses = regexp.MustCompile(`^\s*-?\s*uses:\s*(\S+)(.*)$`)
	// A commit, and its full version beside it, the way Dependabot moves
	// the two together.
	pinned    = regexp.MustCompile(`^[\w.-]+/[\w./-]+@[0-9a-f]{40}$`)
	versioned = regexp.MustCompile(`^\s+# v\d+\.\d+\.\d+\s*$`)
)

// Every action is pinned to a commit, with its full version in the comment
// beside it: a tag can be moved to other code, a commit cannot. Ours, by
// their path, are the exception.
//
// On the real disk: what is read is the repository's own workflows.
func TestEveryActionIsPinned(t *testing.T) {
	t.Parallel()
	for _, f := range actionFiles(t) {
		b, err := os.ReadFile(f)
		if err != nil {
			t.Fatal(err)
		}
		for i, line := range strings.Split(string(b), "\n") {
			m := uses.FindStringSubmatch(line)
			if m == nil || strings.HasPrefix(m[1], "./") || strings.HasPrefix(m[1], "docker://") {
				continue
			}
			if !pinned.MatchString(m[1]) || !versioned.MatchString(m[2]) {
				t.Errorf("%s:%d: pin %s to a commit and name its full version, like @<commit> # v7.0.2", filepath.Base(f), i+1, strings.TrimSpace(line))
			}
		}
	}
}

// Node is pinned once, in .node-version, which every workflow reads and
// the CI image's Node agrees with.
//
// On the real disk: what is read is the repository's own files.
func TestNodeIsPinnedOnce(t *testing.T) {
	t.Parallel()
	b, err := os.ReadFile(filepath.Join("..", "..", ".node-version"))
	if err != nil {
		t.Fatal(err)
	}
	node := strings.TrimSpace(string(b))
	if !regexp.MustCompile(`^\d+\.\d+\.\d+$`).MatchString(node) {
		t.Errorf(".node-version holds %q, want a full version like 22.23.4", node)
	}
	for _, f := range actionFiles(t) {
		b, err := os.ReadFile(f)
		if err != nil {
			t.Fatal(err)
		}
		if regexp.MustCompile(`(?m)^\s*node-version:`).Match(b) {
			t.Errorf("%s names a Node of its own: take it from node-version-file: .node-version", filepath.Base(f))
		}
	}
	docker, err := os.ReadFile(filepath.Join("..", "..", ".github", "ci-image", "Dockerfile"))
	if err != nil {
		t.Fatal(err)
	}
	image := "none"
	if m := regexp.MustCompile(`(?m)^FROM node:([0-9.]+)-`).FindSubmatch(docker); m != nil {
		image = string(m[1])
	}
	if image != node {
		t.Errorf("the CI image lends Node %s, and .node-version says %s", image, node)
	}
}

// The CI image brings the Go that go.mod names, so a job in it downloads
// none. .github/actions/prepare takes the image's Go only when it is that
// version, so an image moved on to the next Go without go.mod, which
// Dependabot proposes, would quietly bring back setup-go's download to
// every job.
//
// On the real disk: what is read is the repository's own go.mod and CI image.
func TestTheCIImageBringsTheGoOfGoMod(t *testing.T) {
	t.Parallel()
	mod, err := os.ReadFile(filepath.Join("..", "..", "go.mod"))
	if err != nil {
		t.Fatal(err)
	}
	want := "none"
	if m := regexp.MustCompile(`(?m)^go (\d+\.\d+)`).FindSubmatch(mod); m != nil {
		want = string(m[1])
	}
	docker, err := os.ReadFile(filepath.Join("..", "..", ".github", "ci-image", "Dockerfile"))
	if err != nil {
		t.Fatal(err)
	}
	image := "none"
	if m := regexp.MustCompile(`(?m)^FROM golang:(\d+\.\d+)[.-]`).FindSubmatch(docker); m != nil {
		image = string(m[1])
	}
	if image != want {
		t.Errorf("the CI image brings Go %s, and go.mod names %s", image, want)
	}
}

// read is a path a Go test names to read a file of the interface or the
// docs, relative to the repository's root however many ../ lead there.
var read = regexp.MustCompile(`"(?:\.\./)*((?:frontend|docs)/[^"]+)"`)

// A pull request starts the Go tests only when it changes Go, by the
// filters of the job changes in ci.yml, and Go is no file of frontend/
// and none of the docs. So a Go test that reads one would not run on the
// pull request that changes it, and main would find out after the merge.
// Every file of the interface a Go test reads is in the filter goread,
// beside go, and only the tests that make lint runs read the docs, since
// make lint runs on every pull request. The walks, which read
// frontend/preview, start on a change to the interface.
//
// On the real disk: what is read is the repository's own tests and CI.
func TestWhatAGoTestReadsStartsIt(t *testing.T) {
	t.Parallel()
	root := filepath.Join("..", "..")
	b, err := os.ReadFile(filepath.Join(root, ".github", "workflows", "ci.yml"))
	if err != nil {
		t.Fatal(err)
	}
	var ci struct {
		Jobs map[string]struct {
			Steps []struct {
				ID   string            `yaml:"id"`
				With map[string]string `yaml:"with"`
			} `yaml:"steps"`
		} `yaml:"jobs"`
	}
	if err := yaml.Unmarshal(b, &ci); err != nil {
		t.Fatal(err)
	}
	var filters map[string][]string
	for _, s := range ci.Jobs["changes"].Steps {
		if s.ID == "filter" {
			if err := yaml.Unmarshal([]byte(s.With["filters"]), &filters); err != nil {
				t.Fatal(err)
			}
		}
	}
	if len(filters["goread"]) == 0 {
		t.Fatal("ci.yml's job changes has no filter goread, so this test reads it wrong")
	}
	starts := func(file string) bool {
		for _, p := range filters["goread"] {
			for _, glob := range braces(p) {
				if ok, _ := path.Match(glob, file); ok {
					return true
				}
			}
		}
		return false
	}
	lint := map[string]bool{"internal/workflows": true, "internal/naming": true}
	found := 0
	err = filepath.WalkDir(root, func(p string, d fs.DirEntry, err error) error {
		if err != nil {
			return err
		}
		if d.IsDir() && (d.Name() == "node_modules" || strings.HasPrefix(d.Name(), ".")) && p != root {
			return filepath.SkipDir
		}
		if d.IsDir() || !strings.HasSuffix(p, "_test.go") {
			return nil
		}
		src, err := os.ReadFile(p)
		if err != nil {
			return err
		}
		rel, _ := filepath.Rel(root, p)
		pkg := filepath.ToSlash(filepath.Dir(rel))
		for _, m := range read.FindAllStringSubmatch(string(src), -1) {
			file := m[1]
			found++
			switch {
			case lint[pkg], strings.HasPrefix(file, "frontend/preview/"):
			case strings.HasPrefix(file, "docs/"):
				t.Errorf("%s reads %s, and a pull request of the docs starts no Go test: read it in a test make lint runs, LINTPKGS", rel, file)
			case !starts(file):
				t.Errorf("%s reads %s, which ci.yml's filter goread leaves out: a pull request that changes it starts no Go test. Add it there", rel, file)
			}
		}
		return nil
	})
	if err != nil {
		t.Fatal(err)
	}
	if found == 0 {
		t.Fatal("no Go test reads a file of the interface, so this test reads them wrong")
	}
}

// braces spells out a glob's one {a,b} the way the filter reads it.
func braces(glob string) []string {
	i, j := strings.Index(glob, "{"), strings.Index(glob, "}")
	if i < 0 || j < i {
		return []string{glob}
	}
	var out []string
	for _, alt := range strings.Split(glob[i+1:j], ",") {
		out = append(out, glob[:i]+alt+glob[j+1:])
	}
	return out
}

// No workflow runs on pull_request_target, and conflicts.yml hears every
// push from main's own copy of it. pull_request_target runs main's
// workflow for a stranger's pull request with a token that writes, so one
// step that reads the pull request's code hands that token over, which is
// why GitHub blocks it in a public repository. pull_request runs on no
// pull request that conflicts. So conflicts.yml runs on a push to main,
// and on a push to any other branch through builds.yml, whose start GitHub
// reports to main's copy. A push to a branch must not start it directly,
// because then the branch's own copy, code nobody has reviewed, would get
// the token that comments.
//
// On the real disk: what is read is the repository's own workflows.
func TestNoPullRequestTarget(t *testing.T) {
	t.Parallel()
	names := map[string]string{}
	seen, waitsOn := false, ""
	for _, f := range actionFiles(t) {
		b, err := os.ReadFile(f)
		if err != nil {
			t.Fatal(err)
		}
		name := filepath.Base(f)
		var wf struct {
			Name string `yaml:"name"`
			On   any    `yaml:"on"`
		}
		if err := yaml.Unmarshal(b, &wf); err != nil {
			t.Fatalf("%s: %v", name, err)
		}
		names[name] = wf.Name
		// The events are a word, a list or a map.
		events := map[string]any{}
		switch on := wf.On.(type) {
		case string:
			events[on] = nil
		case []any:
			for _, e := range on {
				events[fmt.Sprint(e)] = nil
			}
		case map[string]any:
			events = on
		}
		if _, ok := events["pull_request_target"]; ok {
			t.Errorf("%s runs on pull_request_target, which hands a token that writes to a stranger's pull request: use pull_request, or workflow_run as conflicts.yml does", name)
		}
		if name != "conflicts.yml" {
			continue
		}
		seen = true
		push, _ := events["push"].(map[string]any)
		if b, _ := push["branches"].([]any); len(push) != 1 || len(b) != 1 || b[0] != "main" {
			t.Errorf("%s runs on a push to %v, where only main may start it: a branch's own copy would get the token that comments", name, events["push"])
		}
		run, _ := events["workflow_run"].(map[string]any)
		builds, _ := run["workflows"].([]any)
		types, _ := run["types"].([]any)
		if len(builds) != 1 || len(types) != 1 || types[0] != "requested" {
			t.Errorf("%s does not start when builds.yml starts, so a pull request that starts to conflict on its own push is never told", name)
			continue
		}
		waitsOn = fmt.Sprint(builds[0])
	}
	if !seen {
		t.Error("found no conflicts.yml, so nothing tells a pull request that it conflicts")
	}
	if waitsOn != "" && waitsOn != names["builds.yml"] {
		t.Errorf("conflicts.yml waits on the workflow %q, but builds.yml is called %q", waitsOn, names["builds.yml"])
	}
}
