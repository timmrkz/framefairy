package workflows

import (
	"errors"
	"fmt"
	"io/fs"
	"os"
	"path"
	"path/filepath"
	"regexp"
	"slices"
	"strings"
	"testing"
	"testing/fstest"

	"gopkg.in/yaml.v3"
)

// The files GitHub runs: every workflow and the setup the CI jobs share.
//
// On the real disk: what is read is the repository's own workflows.
func actionFiles(t *testing.T) []string {
	t.Helper()
	root := filepath.Join("..", "..", ".github")
	found, err := gitHubRuns(os.DirFS(root))
	if err != nil {
		t.Fatal(err)
	}
	files := make([]string, len(found))
	for i, f := range found {
		files[i] = filepath.Join(root, filepath.FromSlash(f))
	}
	return files
}

// gitHubRuns lists what GitHub runs in a .github folder, a workflow or a
// composite action, each named either way GitHub reads it, .yml or .yaml.
func gitHubRuns(github fs.FS) ([]string, error) {
	var files []string
	for _, glob := range []string{"workflows/*.yml", "workflows/*.yaml", "actions/*/action.yml", "actions/*/action.yaml"} {
		found, err := fs.Glob(github, glob)
		if err != nil {
			return nil, err
		}
		files = append(files, found...)
	}
	if !slices.ContainsFunc(files, func(f string) bool { return strings.HasPrefix(f, "workflows/") }) {
		return nil, errors.New("found no workflow in .github/workflows")
	}
	return files, nil
}

// A composite action named action.yaml is run by GitHub as one named
// action.yml is, so every rule reads it too.
func TestEveryNameGitHubRunsIsRead(t *testing.T) {
	t.Parallel()
	github := fstest.MapFS{
		"workflows/ci.yml":               {},
		"workflows/nightly.yaml":         {},
		"actions/prepare/action.yml":     {},
		"actions/setup/action.yaml":      {},
		"actions/setup/README.md":        {},
		"actions/setup/inner/action.yml": {},
		"workflows/notes.txt":            {},
	}
	got, err := gitHubRuns(github)
	if err != nil {
		t.Fatal(err)
	}
	slices.Sort(got)
	want := []string{"actions/prepare/action.yml", "actions/setup/action.yaml", "workflows/ci.yml", "workflows/nightly.yaml"}
	if !slices.Equal(got, want) {
		t.Errorf("read %q, want %q", got, want)
	}
	if _, err := gitHubRuns(fstest.MapFS{"actions/prepare/action.yml": {}}); err == nil {
		t.Error("a .github folder with no workflow passed")
	}
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
	docker, err := os.ReadFile(filepath.Join("..", "..", "ci-image", "Dockerfile"))
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
	docker, err := os.ReadFile(filepath.Join("..", "..", "ci-image", "Dockerfile"))
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

// publish.yml reads a branch's build before it signs it, and the reading
// runs in a step of its own that holds no secret and no token: the zip is
// code nobody has reviewed, and nothing needs the update key or a token
// that writes until it has passed.
// The step that signs comes after it, and runs only when it passed.
//
// On the real disk: what is read is the repository's own workflows.
func TestTheBuildIsCheckedBeforeTheKeyIsThere(t *testing.T) {
	t.Parallel()
	b, err := os.ReadFile(filepath.Join("..", "..", ".github", "workflows", "publish.yml"))
	if err != nil {
		t.Fatal(err)
	}
	var wf struct {
		Jobs map[string]struct {
			Env   map[string]string `yaml:"env"`
			Steps []struct {
				ID   string            `yaml:"id"`
				Name string            `yaml:"name"`
				If   string            `yaml:"if"`
				Env  map[string]string `yaml:"env"`
				Run  string            `yaml:"run"`
			} `yaml:"steps"`
		} `yaml:"jobs"`
	}
	if err := yaml.Unmarshal(b, &wf); err != nil {
		t.Fatal(err)
	}
	signed := false
	for job, j := range wf.Jobs {
		checked := ""
		for _, s := range j.Steps {
			switch {
			case strings.Contains(s.Run, "framefairy-release check"):
				checked = s.ID
				held := func(v string) bool { return strings.Contains(v, "secrets.") || strings.Contains(v, "github.token") }
				for k, v := range j.Env {
					if held(v) {
						t.Errorf("publish.yml's %s job holds %s in every step, the check of the build too", job, k)
					}
				}
				for k, v := range s.Env {
					if held(v) {
						t.Errorf("%q holds %s while it reads the build", s.Name, k)
					}
				}
			case strings.Contains(s.Run, "framefairy-release sign"):
				signed = true
				if checked == "" {
					t.Errorf("%q signs a build no step before it checked, or the check has no id to be asked by", s.Name)
				} else if want := "steps." + checked + ".outcome == 'success'"; strings.TrimSpace(s.If) != want {
					t.Errorf("%q runs if %s, where only if: %s says it signs nothing the check did not pass", s.Name, s.If, want)
				}
			}
		}
	}
	if !signed {
		t.Error("found no step in publish.yml that runs framefairy-release sign")
	}
}

// A job that shares its concurrency group with another job can be
// cancelled by GitHub before it starts: a group keeps one job waiting and
// cancels the one before. So such a job only does what the next one in
// the group does again, like writing the channel list, and never marks a
// commit, by GitHub's statuses or by framefairy-release publish or
// not-published, which mark one. A cancelled list in publish.yml took the
// Ready to update to of two pull requests with it.
//
// On the real disk: what is read is the repository's own workflows.
func TestNoJobInASharedQueueMarksACommit(t *testing.T) {
	t.Parallel()
	for _, f := range actionFiles(t) {
		b, err := os.ReadFile(f)
		if err != nil {
			t.Fatal(err)
		}
		var wf struct {
			Jobs map[string]struct {
				Concurrency any `yaml:"concurrency"`
				Steps       []struct {
					Name string `yaml:"name"`
					Run  string `yaml:"run"`
				} `yaml:"steps"`
			} `yaml:"jobs"`
		}
		if err := yaml.Unmarshal(b, &wf); err != nil {
			t.Fatal(err)
		}
		// The group is a word, or a map with a group in it.
		group := func(c any) string {
			if m, ok := c.(map[string]any); ok {
				return fmt.Sprint(m["group"])
			}
			if c == nil {
				return ""
			}
			return fmt.Sprint(c)
		}
		jobs := map[string]int{}
		for _, j := range wf.Jobs {
			if g := group(j.Concurrency); g != "" {
				jobs[g]++
			}
		}
		for name, j := range wf.Jobs {
			if jobs[group(j.Concurrency)] < 2 {
				continue
			}
			for _, s := range j.Steps {
				if strings.Contains(s.Run, "/statuses/") || marks.MatchString(s.Run) {
					t.Errorf("%s: the job %s shares the queue %s, where GitHub can cancel it before it starts, and %q marks a commit, which nothing marks again", filepath.Base(f), name, group(j.Concurrency), s.Name)
				}
			}
		}
	}
}

// The commands of framefairy-release that mark a commit.
var marks = regexp.MustCompile(`framefairy-release (publish|not-published)(\s|$)`)

// publish.yml signs only what builds.yml built on a push, and only a
// commit the channel's own history holds. GitHub starts it for a run of
// any workflow with the name it waits on, and a tag pushed with the name
// main gives a record that reads like a push to main, at any commit.
//
// On the real disk: what is read is the repository's own workflows.
func TestOnlyACommitOfTheChannelIsPublished(t *testing.T) {
	t.Parallel()
	b, err := os.ReadFile(filepath.Join("..", "..", ".github", "workflows", "publish.yml"))
	if err != nil {
		t.Fatal(err)
	}
	var wf struct {
		Jobs map[string]struct {
			If    string `yaml:"if"`
			Steps []struct {
				Run string `yaml:"run"`
			} `yaml:"steps"`
		} `yaml:"jobs"`
	}
	if err := yaml.Unmarshal(b, &wf); err != nil {
		t.Fatal(err)
	}
	publish, ok := wf.Jobs["publish"]
	if !ok {
		t.Fatal("publish.yml has no job called publish")
	}
	for _, want := range []string{
		"github.event.workflow_run.event == 'push'",
		"github.event.workflow_run.path == '.github/workflows/builds.yml'",
	} {
		if !strings.Contains(publish.If, want) {
			t.Errorf("publish runs without %s, so a run of another workflow file, or of no push, is signed", want)
		}
	}
	// The step that asks whether the channel holds the commit comes before
	// the build is read or signed.
	asked := false
	for _, s := range publish.Steps {
		if strings.Contains(s.Run, "framefairy-release publish-plan") {
			asked = true
		}
		if strings.Contains(s.Run, "framefairy-release check") || strings.Contains(s.Run, "framefairy-release sign") {
			if !asked {
				t.Errorf("publish runs %q before it asks whether the channel holds the commit, with framefairy-release publish-plan", s.Run)
			}
			break
		}
	}
	if !asked {
		t.Error("publish never asks whether the channel holds the commit, so a tag named main is signed as main's build")
	}
}

// A publish that fails says so on the commit that was built, whatever
// failed, so the pull request shows why and the channel list stops
// saying a build is coming, see ended in cmd/framefairy-release. The step that
// says so is the last of the job, so it hears every step before it.
//
// On the real disk: what is read is the repository's own workflows.
func TestAFailedPublishMarksItsCommit(t *testing.T) {
	t.Parallel()
	b, err := os.ReadFile(filepath.Join("..", "..", ".github", "workflows", "publish.yml"))
	if err != nil {
		t.Fatal(err)
	}
	var wf struct {
		Jobs map[string]struct {
			Steps []struct {
				Name string `yaml:"name"`
				If   string `yaml:"if"`
				Run  string `yaml:"run"`
			} `yaml:"steps"`
		} `yaml:"jobs"`
	}
	if err := yaml.Unmarshal(b, &wf); err != nil {
		t.Fatal(err)
	}
	steps := wf.Jobs["publish"].Steps
	if len(steps) == 0 {
		t.Fatal("publish.yml has no job called publish")
	}
	last := steps[len(steps)-1]
	if strings.TrimSpace(last.If) != "failure()" || !strings.Contains(last.Run, "framefairy-release not-published") {
		t.Errorf("the last step of publish, %q, is not one that marks the commit when any step before it failed", last.Name)
	}
}

// No package of the interface runs code of its own while it is installed.
// frontend/.npmrc says so for every npm ci and npm install in frontend/, in
// make, in CI and by hand. An install script runs before anybody has read
// the release it came with, which is how the npm worms of 2026 spread.
//
// On the real disk: what is read is the repository's own npm settings.
func TestNoPackageRunsCodeWhenInstalled(t *testing.T) {
	t.Parallel()
	b, err := os.ReadFile(filepath.Join("..", "..", "frontend", ".npmrc"))
	if err != nil {
		t.Fatal(err)
	}
	if !regexp.MustCompile(`(?m)^\s*ignore-scripts\s*=\s*true\s*$`).Match(b) {
		t.Error("frontend/.npmrc does not say ignore-scripts=true, so npm ci runs every package's install script")
	}
}

// dependabotConfig is what the tests below read of .github/dependabot.yml.
type dependabotConfig struct {
	Groups  map[string]any `yaml:"multi-ecosystem-groups"`
	Updates []struct {
		Ecosystem   string   `yaml:"package-ecosystem"`
		Directory   string   `yaml:"directory"`
		Directories []string `yaml:"directories"`
		Group       string   `yaml:"multi-ecosystem-group"`
		Patterns    []string `yaml:"patterns"`
		Allow       []struct {
			Type string `yaml:"dependency-type"`
		} `yaml:"allow"`
		Ignore []struct {
			Name string `yaml:"dependency-name"`
		} `yaml:"ignore"`
	} `yaml:"updates"`
}

// readDependabot reads the repository's Dependabot settings.
//
// On the real disk: what is read is the repository's own Dependabot settings.
func readDependabot(t *testing.T) dependabotConfig {
	t.Helper()
	b, err := os.ReadFile(filepath.Join("..", "..", ".github", "dependabot.yml"))
	if err != nil {
		t.Fatal(err)
	}
	var config dependabotConfig
	if err := yaml.Unmarshal(b, &config); err != nil {
		t.Fatal(err)
	}
	return config
}

// Dependabot moves every Go module, the ones go.mod marks indirect too.
// Without allow: dependency-type: all it moves only the modules go.mod
// requires by name, and golang.org/x/text kept a known flaw for months.
//
// On the real disk: what is read is the repository's own Dependabot settings.
func TestDependabotMovesEveryGoModule(t *testing.T) {
	t.Parallel()
	found := false
	for _, u := range readDependabot(t).Updates {
		if u.Ecosystem != "gomod" || u.Group != "" {
			continue
		}
		found = true
		all := false
		for _, a := range u.Allow {
			all = all || a.Type == "all"
		}
		if !all {
			t.Error("dependabot.yml's gomod entry does not allow dependency-type all, so the indirect modules never move")
		}
	}
	if !found {
		t.Fatal("dependabot.yml has no gomod entry of its own, so this test reads it wrong")
	}
}

// Dependabot moves Wails, the Go module and @wailsio/runtime together, in
// one pull request, and never one without the other. The build fails when
// the two differ, see notices/gen. Wails was left out of Dependabot
// altogether before, and sat on a beta with published security advisories.
//
// On the real disk: what is read is the repository's own Dependabot settings.
func TestDependabotMovesWailsInOnePullRequest(t *testing.T) {
	t.Parallel()
	config := readDependabot(t)
	wails := map[string]string{"gomod": "github.com/wailsapp/wails/v3", "npm": "@wailsio/runtime"}
	group := map[string]string{}
	for _, u := range config.Updates {
		name, ok := wails[u.Ecosystem]
		if !ok {
			continue
		}
		if u.Group != "" {
			for _, p := range u.Patterns {
				if p == name {
					group[u.Ecosystem] = u.Group
				}
			}
			continue
		}
		ignored := false
		for _, i := range u.Ignore {
			ignored = ignored || i.Name == name
		}
		if !ignored {
			t.Errorf("dependabot.yml's %s entry for %s does not ignore %s, so it can come without its other half", u.Ecosystem, u.Directory, name)
		}
	}
	if group["gomod"] == "" || group["npm"] == "" || group["gomod"] != group["npm"] {
		t.Errorf("dependabot.yml moves Wails in the groups %q for Go and %q for npm, want one multi-ecosystem group for both", group["gomod"], group["npm"])
	}
	if _, ok := config.Groups[group["gomod"]]; !ok && group["gomod"] != "" {
		t.Errorf("dependabot.yml names the group %q, which multi-ecosystem-groups does not define", group["gomod"])
	}
}

// Dependabot opens no pull request that changes a file under .github/
// for any ecosystem but the actions: GitHub answers "invalid or
// unauthorized changes", and the update fails in its log, where nobody
// reads it. The CI image's Dockerfile lay under .github/ and got no
// update for a month that way.
//
// On the real disk: what is read is the repository's own Dependabot settings.
func TestDependabotMovesNothingUnderGitHubButTheActions(t *testing.T) {
	t.Parallel()
	updates := readDependabot(t).Updates
	if len(updates) == 0 {
		t.Fatal("dependabot.yml has no updates, so this test reads it wrong")
	}
	for _, u := range updates {
		if u.Ecosystem == "github-actions" {
			continue
		}
		for _, d := range append([]string{u.Directory}, u.Directories...) {
			// Dependabot reads a directory with or without its leading slash.
			if p := strings.TrimPrefix(d, "/"); p == ".github" || strings.HasPrefix(p, ".github/") {
				t.Errorf("dependabot.yml's %s entry reads %s, under .github/, where GitHub refuses its pull requests: move the files out of .github/", u.Ecosystem, d)
			}
		}
	}
}

// conflicts.yml tells a pull request that still conflicts again once main
// has moved, so one comment lost on its way to the session that owns the
// pull request is not the last it hears of it. What it was told is kept
// in a hidden mark in the comment, and the mark holds both commits, the
// pull request's and main's, so a new commit of either tells it again and
// the same two never tell it twice.
//
// On the real disk: what is read is the repository's own workflow.
func TestAConflictIsToldAgainWhenMainMoves(t *testing.T) {
	t.Parallel()
	b, err := os.ReadFile(filepath.Join("..", "..", ".github", "workflows", "conflicts.yml"))
	if err != nil {
		t.Fatal(err)
	}
	var marks []string
	for _, line := range strings.Split(string(b), "\n") {
		if strings.Contains(line, "mark=") {
			marks = append(marks, strings.TrimSpace(line))
		}
	}
	if len(marks) != 1 {
		t.Fatalf("conflicts.yml sets its mark in %d places, want 1: %q", len(marks), marks)
	}
	for _, commit := range []string{"$head", "$(git rev-parse HEAD)"} {
		if !strings.Contains(marks[0], commit) {
			t.Errorf("the mark %s does not hold %s, so a pull request still in conflict is not told again when that commit is new", marks[0], commit)
		}
	}
}

// CI tests only the newest commit of a branch, main as well. A run left to
// finish for a commit already outdated, often with a Mac job still waiting
// in the Mac queue, held the newest commit of main back each time pull
// requests were merged one after another.
//
// On the real disk: what is read is the repository's own workflows.
func TestCIStopsTheRunOfAnOutdatedCommit(t *testing.T) {
	t.Parallel()
	b, err := os.ReadFile(filepath.Join("..", "..", ".github", "workflows", "ci.yml"))
	if err != nil {
		t.Fatal(err)
	}
	var wf struct {
		Concurrency struct {
			Group  string `yaml:"group"`
			Cancel any    `yaml:"cancel-in-progress"`
		} `yaml:"concurrency"`
	}
	if err := yaml.Unmarshal(b, &wf); err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(wf.Concurrency.Group, "github.ref") {
		t.Errorf("ci.yml queues its runs in %q, not one queue per branch", wf.Concurrency.Group)
	}
	if wf.Concurrency.Cancel != true {
		t.Errorf("ci.yml says cancel-in-progress: %v, so a newer commit waits for the run of an older one", wf.Concurrency.Cancel)
	}
}

// A push takes two Mac jobs at most, see A Mac only for what is
// different on a Mac in CLAUDE.md: the Go tests on a Mac in ci.yml, when
// it changed code only a Mac builds, and the build to update to in
// builds.yml. The account runs five Mac jobs at once for every branch
// together, and when a push took three, two pushes close together filled
// the queue. A Mac named in a matrix and one in a workflow a job calls
// count too.
//
// On the real disk: what is read is the repository's own workflows.
func TestAPushTakesTwoMacJobs(t *testing.T) {
	t.Parallel()
	var macs []string
	for _, f := range actionFiles(t) {
		if !strings.Contains(filepath.ToSlash(f), "/workflows/") {
			continue
		}
		var wf struct {
			On map[string]any `yaml:"on"`
		}
		readYAML(t, f, &wf)
		_, push := wf.On["push"]
		_, pr := wf.On["pull_request"]
		if push || pr {
			macs = append(macs, macJobs(t, f)...)
		}
	}
	if len(macs) != 2 {
		t.Errorf("a push or a pull request starts %d Mac jobs, %v, where it should take two at most: the Go tests when the Mac's code changed, and the build to update to", len(macs), macs)
	}
}

// macJobs names the Mac jobs a run of the workflow at path starts: one
// for each job on a Mac, one for each Mac in a job's matrix, and the
// Mac jobs of a workflow of this repository a job calls.
//
// On the real disk: what is read is the repository's own workflows.
func macJobs(t *testing.T, path string) []string {
	t.Helper()
	var wf struct {
		Jobs map[string]struct {
			RunsOn   any    `yaml:"runs-on"`
			Uses     string `yaml:"uses"`
			Strategy struct {
				Matrix any `yaml:"matrix"`
			} `yaml:"strategy"`
		} `yaml:"jobs"`
	}
	readYAML(t, path, &wf)
	var macs []string
	for name, j := range wf.Jobs {
		name = filepath.Base(path) + " " + name
		switch {
		case strings.HasPrefix(j.Uses, "./.github/workflows/"):
			macs = append(macs, macJobs(t, filepath.Join("..", "..", j.Uses))...)
		case strings.Contains(fmt.Sprint(j.RunsOn), "${{"):
			for range countMacs(j.Strategy.Matrix) {
				macs = append(macs, name)
			}
		case strings.Contains(fmt.Sprint(j.RunsOn), "macos"):
			macs = append(macs, name)
		}
	}
	return macs
}

// countMacs counts the values of a matrix that name a Mac.
func countMacs(v any) int {
	switch v := v.(type) {
	case string:
		if strings.Contains(v, "macos") {
			return 1
		}
	case []any:
		n := 0
		for _, e := range v {
			n += countMacs(e)
		}
		return n
	case map[string]any:
		n := 0
		for k, e := range v {
			if k != "exclude" {
				n += countMacs(e)
			}
		}
		return n
	}
	return 0
}

// readYAML reads the workflow at path into v.
//
// On the real disk: what is read is the repository's own workflows.
func readYAML(t *testing.T, path string, v any) {
	t.Helper()
	b, err := os.ReadFile(path)
	if err != nil {
		t.Fatal(err)
	}
	if err := yaml.Unmarshal(b, v); err != nil {
		t.Fatalf("%s: %v", path, err)
	}
}

// What only a Mac can tell runs every night on main, and by hand on any
// branch: every Go test on a Mac and the app opened from outside. No
// workflow a push or a pull request starts opens the app from outside,
// since it would take a Mac on every push for code most pushes never
// touch.
//
// On the real disk: what is read is the repository's own workflows.
func TestWhatOnlyAMacTellsRunsEveryNight(t *testing.T) {
	t.Parallel()
	type workflow struct {
		On   map[string]any `yaml:"on"`
		Jobs map[string]struct {
			RunsOn any `yaml:"runs-on"`
			Steps  []struct {
				Run string `yaml:"run"`
			} `yaml:"steps"`
		} `yaml:"jobs"`
	}
	runsOnAMac := func(wf workflow, command string) bool {
		for _, j := range wf.Jobs {
			if !strings.Contains(fmt.Sprint(j.RunsOn), "macos") {
				continue
			}
			for _, s := range j.Steps {
				if strings.HasPrefix(s.Run, command) {
					return true
				}
			}
		}
		return false
	}
	for _, f := range actionFiles(t) {
		if !strings.Contains(filepath.ToSlash(f), "/workflows/") {
			continue
		}
		var wf workflow
		readYAML(t, f, &wf)
		_, push := wf.On["push"]
		_, pr := wf.On["pull_request"]
		if (push || pr) && runsOnAMac(wf, "make outside") {
			t.Errorf("%s opens the app from outside on a push", filepath.Base(f))
		}
	}
	var night workflow
	readYAML(t, filepath.Join("..", "..", ".github", "workflows", "mac-nightly.yml"), &night)
	for _, on := range []string{"schedule", "workflow_dispatch"} {
		if _, ok := night.On[on]; !ok {
			t.Errorf("mac-nightly.yml does not run on %s", on)
		}
	}
	for _, command := range []string{"make unit", "make outside"} {
		if !runsOnAMac(night, command) {
			t.Errorf("mac-nightly.yml does not run %s on a Mac", command)
		}
	}
}

// A pull request runs the Go tests on a Mac when it changes code only a
// Mac builds, and only then. internal/maccode keeps every such file of
// ours named *_darwin.go or *_darwin_test.go, and the modules' own come in
// through go.mod and go.sum, so the filter mac of the job changes looks
// for exactly those. It is one pattern, since the filter needs every line
// of it to match.
//
// On the real disk: what is read is the repository's own workflows.
func TestThePullRequestsMacTestsFollowItsMacCode(t *testing.T) {
	t.Parallel()
	var wf struct {
		Jobs map[string]struct {
			If    string `yaml:"if"`
			Steps []struct {
				ID   string         `yaml:"id"`
				With map[string]any `yaml:"with"`
			} `yaml:"steps"`
		} `yaml:"jobs"`
	}
	readYAML(t, filepath.Join("..", "..", ".github", "workflows", "ci.yml"), &wf)
	if got := wf.Jobs["macos"].If; got != "needs.changes.outputs.mac == 'true'" {
		t.Errorf("the job macos runs if %q, not when the filter mac matched", got)
	}
	for _, s := range wf.Jobs["changes"].Steps {
		if s.ID != "filter" {
			continue
		}
		var filters map[string][]string
		if err := yaml.Unmarshal([]byte(fmt.Sprint(s.With["filters"])), &filters); err != nil {
			t.Fatal(err)
		}
		if got := filters["mac"]; !slices.Equal(got, []string{"{**/*_darwin*.go,go.mod,go.sum}"}) {
			t.Errorf("the filter mac is %q, not the files internal/maccode keeps the Mac's code in", got)
		}
		return
	}
	t.Error("ci.yml has no step filter in its job changes")
}

// A workflow that runs on a schedule runs where nobody waits on it, so
// when it fails on main it tells of itself: its last job, tell, needs
// every other job, runs only when one of them failed on main, and calls
// tell.yml, the one place that has framefairy-release open the
// workflow's issue or add the run to it. A run by hand on a branch is its
// session's to read, and a run that was stopped is no failure. See
// failed.go in cmd/framefairy-release.
//
// On the real disk: what is read is the repository's own workflows.
func TestAScheduledRunThatFailsTellsOfItself(t *testing.T) {
	t.Parallel()
	type workflow struct {
		On          map[string]any    `yaml:"on"`
		Permissions map[string]string `yaml:"permissions"`
		Jobs        map[string]struct {
			Needs       any               `yaml:"needs"`
			If          string            `yaml:"if"`
			Uses        string            `yaml:"uses"`
			Permissions map[string]string `yaml:"permissions"`
			Steps       []struct {
				Run string `yaml:"run"`
			} `yaml:"steps"`
		} `yaml:"jobs"`
	}
	root := filepath.Join("..", "..", ".github", "workflows")
	var tellYML workflow
	readYAML(t, filepath.Join(root, "tell.yml"), &tellYML)
	told := false
	for _, j := range tellYML.Jobs {
		for _, s := range j.Steps {
			told = told || strings.HasPrefix(s.Run, "framefairy-release failed ")
		}
	}
	if _, called := tellYML.On["workflow_call"]; !called || !told {
		t.Error("tell.yml is not a workflow to call that runs framefairy-release failed")
	}
	scheduled := 0
	for _, f := range actionFiles(t) {
		if !strings.Contains(filepath.ToSlash(f), "/workflows/") {
			continue
		}
		var wf workflow
		readYAML(t, f, &wf)
		if _, ok := wf.On["schedule"]; !ok {
			continue
		}
		scheduled++
		name := filepath.Base(f)
		tell, ok := wf.Jobs["tell"]
		if !ok {
			t.Errorf("%s runs on a schedule and has no job tell, so a failure on main reaches nobody", name)
			continue
		}
		var needs []string
		switch n := tell.Needs.(type) {
		case string:
			needs = []string{n}
		case []any:
			for _, x := range n {
				needs = append(needs, fmt.Sprint(x))
			}
		}
		for job := range wf.Jobs {
			if job != "tell" && !slices.Contains(needs, job) {
				t.Errorf("%s: tell does not need %s, so its failure is told by nobody", name, job)
			}
		}
		if want := "failure() && github.ref == 'refs/heads/main'"; tell.If != want {
			t.Errorf("%s: tell runs if %q, want %q", name, tell.If, want)
		}
		if tell.Uses != "./.github/workflows/tell.yml" {
			t.Errorf("%s: tell uses %q, not tell.yml", name, tell.Uses)
		}
		// A called workflow may ask for no more than its caller grants,
		// or the run fails before any job starts and tells nobody.
		for what, may := range tellYML.Permissions {
			if tell.Permissions[what] != may {
				t.Errorf("%s: tell grants %s %q, tell.yml asks for %q", name, what, tell.Permissions[what], may)
			}
		}
	}
	if scheduled < 4 {
		t.Errorf("found %d workflows on a schedule, fewer than the four there are", scheduled)
	}
}
