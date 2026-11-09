package workflows

import (
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
	files, err := filepath.Glob(filepath.Join(root, "workflows", "*.yml"))
	if err != nil {
		t.Fatal(err)
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

// The workflows the repository's Actions policy allows to run on
// pull_request_target. GitHub blocks that event in a public repository
// unless a policy names the workflow file, from 2 December 2026, and in
// brownouts before. A workflow added here needs a line in that policy too,
// see When main moves in docs/WORKFLOW.md.
var targetAllowed = map[string]bool{"conflicts.yml": true}

// Only the workflows the policy names run on pull_request_target, and
// they check out main alone. The event runs with a token that writes,
// whatever the pull request holds, so a checkout of the pull request's
// code there would run a stranger's code with it.
//
// On the real disk: what is read is the repository's own workflows.
func TestPullRequestTargetOnlyWhereAllowed(t *testing.T) {
	t.Parallel()
	files, err := filepath.Glob(filepath.Join("..", "..", ".github", "workflows", "*.yml"))
	if err != nil {
		t.Fatal(err)
	}
	for _, f := range files {
		b, err := os.ReadFile(f)
		if err != nil {
			t.Fatal(err)
		}
		var wf struct {
			On   map[string]any `yaml:"on"`
			Jobs map[string]struct {
				Steps []struct {
					Uses string            `yaml:"uses"`
					With map[string]string `yaml:"with"`
				} `yaml:"steps"`
			} `yaml:"jobs"`
		}
		if err := yaml.Unmarshal(b, &wf); err != nil {
			// The event list written as a word or a list: no
			// pull_request_target in a form this test reads, and
			// TestEveryWorkflowReads says whether it is YAML.
			if strings.Contains(string(b), "pull_request_target") {
				t.Errorf("%s names pull_request_target in a form this test cannot read", filepath.Base(f))
			}
			continue
		}
		if _, ok := wf.On["pull_request_target"]; !ok {
			continue
		}
		name := filepath.Base(f)
		if !targetAllowed[name] {
			t.Errorf("%s runs on pull_request_target, which GitHub blocks unless the Actions policy names it: add it there and to targetAllowed, or use pull_request", name)
		}
		for job, j := range wf.Jobs {
			for _, s := range j.Steps {
				if strings.HasPrefix(s.Uses, "actions/checkout@") && s.With["ref"] != "main" {
					t.Errorf("%s: job %s checks out %q on pull_request_target, where only main is safe", name, job, s.With["ref"])
				}
			}
		}
	}
}
