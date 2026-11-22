package workflows

import (
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"gopkg.in/yaml.v3"
)

// A step of a job or of a composite action, as far as caches go.
type cacheStep struct {
	Uses string         `yaml:"uses"`
	With map[string]any `yaml:"with"`
}

// A job, as far as what it holds and what it restores go.
type cacheJob struct {
	Uses        string      `yaml:"uses"`
	Environment any         `yaml:"environment"`
	Permissions any         `yaml:"permissions"`
	Steps       []cacheStep `yaml:"steps"`
}

// holdsAKey says whether a job holds something that writes or signs: an
// environment, which is where a key is kept, or a token that may write.
// A job takes the workflow's permissions when it names none, and with
// none named anywhere it gets the repository's default token, which a
// setting can make one that writes, so that counts as writing too.
func holdsAKey(job cacheJob, workflow any) bool {
	if job.Environment != nil {
		return true
	}
	perms := job.Permissions
	if perms == nil {
		perms = workflow
	}
	switch p := perms.(type) {
	case nil:
		return true
	case string:
		return p != "read-all"
	case map[string]any:
		for _, v := range p {
			if fmt.Sprint(v) == "write" {
				return true
			}
		}
	}
	return false
}

// restoresACache names the cache a step restores, or says "" when it
// restores none. A composite action of this repository restores what its
// own steps restore, which local reads.
func restoresACache(s cacheStep, local func(uses string) []cacheStep) string {
	action, _, _ := strings.Cut(s.Uses, "@")
	cache, named := s.With["cache"]
	switch {
	// setup-go keeps a cache unless told not to.
	case action == "actions/setup-go":
		if !named || fmt.Sprint(cache) != "false" {
			return "setup-go's cache"
		}
	// The other setup actions keep one only when told which.
	case strings.HasPrefix(action, "actions/setup-"):
		if named && fmt.Sprint(cache) != "" {
			return action + "'s cache"
		}
	case action == "actions/cache", action == "actions/cache/restore":
		return action
	case strings.Contains(fmt.Sprint(s.With["cache-from"]), "type=gha"):
		return "the Docker layers in the Actions cache"
	case strings.HasPrefix(s.Uses, "./"):
		for _, inner := range local(s.Uses) {
			if c := restoresACache(inner, local); c != "" {
				return s.Uses + ", which restores " + c
			}
		}
	}
	return ""
}

// localAction reads the steps of a composite action of this repository.
//
// On the real disk: what is read is the repository's own actions.
func localAction(t *testing.T) func(uses string) []cacheStep {
	return func(uses string) []cacheStep {
		t.Helper()
		var action struct {
			Runs struct {
				Steps []cacheStep `yaml:"steps"`
			} `yaml:"runs"`
		}
		dir := filepath.Join("..", "..", filepath.FromSlash(strings.TrimPrefix(uses, "./")))
		for _, name := range []string{"action.yml", "action.yaml"} {
			b, err := os.ReadFile(filepath.Join(dir, name))
			if os.IsNotExist(err) {
				continue
			}
			if err != nil {
				t.Fatal(err)
			}
			if err := yaml.Unmarshal(b, &action); err != nil {
				t.Fatalf("%s: %v", uses, err)
			}
			return action.Runs.Steps
		}
		t.Fatalf("%s names an action with no action.yml", uses)
		return nil
	}
}

// A job that holds a key or a token that writes restores no cache. A
// cache that main's runs share can be written by any run on main, one
// that builds a dependency nobody has run before included, and Go trusts
// what it finds in its build cache and its module cache without checking
// it again. So the signing job, built from such a cache, would sign what
// that run left there.
//
// On the real disk: what is read is the repository's own workflows.
func TestAJobThatHoldsAKeyRestoresNoCache(t *testing.T) {
	t.Parallel()
	files, err := filepath.Glob(filepath.Join("..", "..", ".github", "workflows", "*.y*ml"))
	if err != nil {
		t.Fatal(err)
	}
	checked := 0
	for _, f := range files {
		var wf struct {
			Permissions any                 `yaml:"permissions"`
			Jobs        map[string]cacheJob `yaml:"jobs"`
		}
		readYAML(t, f, &wf)
		for name, job := range wf.Jobs {
			// A job that calls a workflow is checked in that workflow.
			if job.Uses != "" || !holdsAKey(job, wf.Permissions) {
				continue
			}
			checked++
			for _, s := range job.Steps {
				if c := restoresACache(s, localAction(t)); c != "" {
					t.Errorf("%s: the job %s holds a key or a token that writes, and %s restores %s, which any run on main can have written: build it from nothing, with cache: false", filepath.Base(f), name, s.Uses, c)
				}
			}
		}
	}
	if checked == 0 {
		t.Error("found no job that holds a key or a token that writes, so this test checks nothing")
	}
}

// What counts as holding a key, and what counts as restoring a cache.
//
// On the real disk: one case reads the repository's own prepare action.
func TestWhatHoldsAKeyAndWhatRestoresACache(t *testing.T) {
	t.Parallel()
	holds := []struct {
		name     string
		job      string
		workflow string
		want     bool
	}{
		{"an environment", "environment: updates\npermissions: {contents: read}", "", true},
		{"a job that writes", "permissions: {contents: write}", "{contents: read}", true},
		{"a job that only reads", "permissions: {contents: read, actions: read}", "{contents: write}", false},
		{"a job that asks for nothing", "permissions: {}", "", false},
		{"the workflow's write, taken by the job", "runs-on: ubuntu-24.04", "{issues: write}", true},
		{"the workflow's read, taken by the job", "runs-on: ubuntu-24.04", "{contents: read}", false},
		{"no permissions anywhere", "runs-on: ubuntu-24.04", "", true},
		{"write-all", "permissions: write-all", "", true},
		{"read-all", "permissions: read-all", "", false},
	}
	for _, c := range holds {
		var job cacheJob
		var workflow any
		if err := yaml.Unmarshal([]byte(c.job), &job); err != nil {
			t.Fatal(err)
		}
		if err := yaml.Unmarshal([]byte(c.workflow), &workflow); err != nil {
			t.Fatal(err)
		}
		if got := holdsAKey(job, workflow); got != c.want {
			t.Errorf("%s: holds a key is %v, want %v", c.name, got, c.want)
		}
	}

	restores := []struct {
		step string
		want bool
	}{
		{"uses: actions/setup-go@x\nwith: {go-version-file: go.mod}", true},
		{"uses: actions/setup-go@x\nwith: {cache: true}", true},
		{"uses: actions/setup-go@x\nwith: {cache: false}", false},
		{"uses: actions/setup-node@x\nwith: {cache: npm}", true},
		{"uses: actions/setup-node@x\nwith: {node-version-file: .node-version}", false},
		{"uses: actions/setup-python@x\nwith: {cache: pip}", true},
		{"uses: actions/cache@x\nwith: {path: a, key: b}", true},
		{"uses: actions/cache/restore@x\nwith: {path: a, key: b}", true},
		{"uses: actions/cache/save@x\nwith: {path: a, key: b}", false},
		{"uses: docker/build-push-action@x\nwith: {cache-from: type=gha}", true},
		{"uses: docker/build-push-action@x\nwith: {push: true}", false},
		{"uses: actions/checkout@x", false},
		{"uses: ./.github/actions/prepare", true},
	}
	for _, c := range restores {
		var s cacheStep
		if err := yaml.Unmarshal([]byte(c.step), &s); err != nil {
			t.Fatal(err)
		}
		if got := restoresACache(s, localAction(t)) != ""; got != c.want {
			t.Errorf("%q: restores a cache is %v, want %v", c.step, got, c.want)
		}
	}
}
