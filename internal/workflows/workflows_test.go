package workflows

import (
	"os"
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
