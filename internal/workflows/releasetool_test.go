package workflows

import (
	"fmt"
	"go/parser"
	"go/token"
	"path"
	"path/filepath"
	"regexp"
	"slices"
	"strconv"
	"strings"
	"testing"
)

// The tool the jobs that hold a key run, and the action that brings it,
// see docs/decisions/2026-11-24-the-release-tool-built-once.md.
const (
	releaseTool       = "cmd/framefairy-release"
	releaseToolAction = "./.github/actions/release-tool"
)

// ownImports names the directories of this module's packages that the
// package in dir imports, itself and the ones they import in turn, from
// every file but the tests, whatever its build tags.
//
// On the real disk: what is read is the repository's own code.
func ownImports(t *testing.T, root, dir string) []string {
	t.Helper()
	seen := map[string]bool{}
	var walk func(dir string)
	walk = func(dir string) {
		if seen[dir] {
			return
		}
		seen[dir] = true
		files, err := filepath.Glob(filepath.Join(root, filepath.FromSlash(dir), "*.go"))
		if err != nil {
			t.Fatal(err)
		}
		for _, f := range files {
			if strings.HasSuffix(f, "_test.go") {
				continue
			}
			file, err := parser.ParseFile(token.NewFileSet(), f, nil, parser.ImportsOnly)
			if err != nil {
				t.Fatal(err)
			}
			for _, imp := range file.Imports {
				p, _ := strconv.Unquote(imp.Path.Value)
				if own, ok := strings.CutPrefix(p, "framefairy/"); ok {
					walk(own)
				}
			}
		}
	}
	walk(dir)
	dirs := make([]string, 0, len(seen))
	for d := range seen {
		dirs = append(dirs, d)
	}
	slices.Sort(dirs)
	return dirs
}

// The paths the action's key is worked out of.
var keyed = regexp.MustCompile(`HEAD:([^\s;]+)`)

// The key the action finds the tool by covers everything the tool is
// built from: go.mod, go.sum and every package of ours it imports. A
// package it imports and the key leaves out would let a change to it keep
// the old build, and release-tool.yml would not build the new one either.
//
// On the real disk: what is read is the repository's own code and workflows.
func TestTheReleaseToolIsKeyedOnWhatItIsBuiltFrom(t *testing.T) {
	t.Parallel()
	root := filepath.Join("..", "..")
	want := append([]string{"go.mod", "go.sum"}, ownImports(t, root, releaseTool)...)
	slices.Sort(want)

	var key []string
	for _, s := range localAction(t)(releaseToolAction) {
		for _, m := range keyed.FindAllStringSubmatch(s.Run, -1) {
			key = append(key, m[1])
		}
	}
	slices.Sort(key)
	if !slices.Equal(key, want) {
		t.Errorf("the release tool's key is worked out of %v, and the tool is built from %v", key, want)
	}

	var wf struct {
		On struct {
			Push struct {
				Branches []string `yaml:"branches"`
				Paths    []string `yaml:"paths"`
			} `yaml:"push"`
		} `yaml:"on"`
	}
	readYAML(t, filepath.Join(root, ".github", "workflows", "release-tool.yml"), &wf)
	if !slices.Equal(wf.On.Push.Branches, []string{"main"}) {
		t.Errorf("release-tool.yml builds on a push to %v, where only main's build may be kept", wf.On.Push.Branches)
	}
	paths := []string{".github/actions/release-tool/**", ".github/workflows/release-tool.yml"}
	for _, k := range want {
		if path.Ext(k) == "" {
			k += "/**"
		}
		paths = append(paths, k)
	}
	slices.Sort(paths)
	got := slices.Sorted(slices.Values(wf.On.Push.Paths))
	if !slices.Equal(got, paths) {
		t.Errorf("release-tool.yml starts on a push to %v, and should on %v, what the tool and its key are made of", got, paths)
	}
}

// What builds the tool from the checkout.
var buildsTheTool = regexp.MustCompile(`go\s+(run|build)\b[^\n]*` + regexp.QuoteMeta(releaseTool))

// A job that holds a key runs the tool the action brings, main's own
// signed build or one built with no cache, and never builds it any other
// way, where a cache could come back in.
//
// On the real disk: what is read is the repository's own workflows.
func TestAJobThatHoldsAKeyRunsTheSignedReleaseTool(t *testing.T) {
	t.Parallel()
	files, err := filepath.Glob(filepath.Join("..", "..", ".github", "workflows", "*.y*ml"))
	if err != nil {
		t.Fatal(err)
	}
	ran := 0
	for _, f := range files {
		var wf struct {
			Permissions any                 `yaml:"permissions"`
			Jobs        map[string]cacheJob `yaml:"jobs"`
		}
		readYAML(t, f, &wf)
		for name, job := range wf.Jobs {
			if job.Uses != "" || !holdsAKey(job, wf.Permissions) {
				continue
			}
			brought := false
			for _, s := range job.Steps {
				brought = brought || s.Uses == releaseToolAction
				if buildsTheTool.MatchString(s.Run) {
					t.Errorf("%s: the job %s holds a key and builds framefairy-release itself: run the one %s brings", filepath.Base(f), name, releaseToolAction)
				}
				if regexp.MustCompile(`(?m)^\s*framefairy-release\s`).MatchString(s.Run) {
					ran++
					if !brought {
						t.Errorf("%s: the job %s runs framefairy-release before %s brings it", filepath.Base(f), name, releaseToolAction)
					}
				}
			}
		}
	}
	if ran == 0 {
		t.Error("no job that holds a key runs framefairy-release, so this test checks nothing")
	}
}

// The action takes the tool it downloads only when main's own
// release-tool.yml signed it, on GitHub's own runner, under the very name
// it was downloaded by. A branch can upload any file under that name: a
// build of its own, which main never signed, or an older build of main's,
// which main signed under its own name. Each of these flags is what refuses
// one of them.
//
// On the real disk: what is read is the repository's own action.
func TestTheReleaseToolIsCheckedAgainstMainsSignature(t *testing.T) {
	t.Parallel()
	var fetch *cacheStep
	steps := localAction(t)(releaseToolAction)
	for i, s := range steps {
		if strings.Contains(s.Run, "gh attestation verify") {
			fetch = &steps[i]
		}
	}
	if fetch == nil {
		t.Fatalf("%s runs no gh attestation verify", releaseToolAction)
	}
	signer := "${{ github.server_url }}/${{ github.repository }}/.github/workflows/release-tool.yml@refs/heads/main"
	if got := fmt.Sprint(fetch.Env["SIGNER"]); got != signer {
		t.Errorf("the signer the tool is checked against is %q, want main's release-tool.yml, %q", got, signer)
	}
	for _, flag := range []string{
		`--cert-identity "$SIGNER"`,
		`--source-ref refs/heads/main`,
		`--deny-self-hosted-runners`,
		`.statement.subject[].name' | grep -xF "$NAME" >/dev/null`,
	} {
		if !strings.Contains(fetch.Run, flag) {
			t.Errorf("%s checks the tool's signature without %s", releaseToolAction, flag)
		}
	}
}

// The token of a job that uses the action may read attestations, or the
// check of every download fails and every job builds the tool itself,
// as slowly as with no signed build at all and with nothing but a warning
// to say so. A job that calls a workflow of ours hands it the token.
//
// On the real disk: what is read is the repository's own workflows.
func TestAJobThatUsesTheReleaseToolMayReadItsSignature(t *testing.T) {
	t.Parallel()
	type workflow struct {
		Permissions any                 `yaml:"permissions"`
		Jobs        map[string]cacheJob `yaml:"jobs"`
	}
	files, err := filepath.Glob(filepath.Join("..", "..", ".github", "workflows", "*.y*ml"))
	if err != nil {
		t.Fatal(err)
	}
	all := map[string]workflow{}
	for _, f := range files {
		var wf workflow
		readYAML(t, f, &wf)
		all["./.github/workflows/"+filepath.Base(f)] = wf
	}
	usesTheTool := func(job cacheJob) bool {
		return slices.ContainsFunc(job.Steps, func(s cacheStep) bool { return s.Uses == releaseToolAction })
	}
	mayRead := func(perms any) bool {
		p, ok := perms.(map[string]any)
		return ok && (p["attestations"] == "read" || p["attestations"] == "write")
	}
	checked := 0
	for file, wf := range all {
		for name, job := range wf.Jobs {
			uses := usesTheTool(job)
			if called, ok := all[job.Uses]; ok {
				for _, j := range called.Jobs {
					uses = uses || usesTheTool(j)
				}
			}
			if !uses {
				continue
			}
			checked++
			perms := job.Permissions
			if perms == nil {
				perms = wf.Permissions
			}
			if !mayRead(perms) {
				t.Errorf("%s: the job %s uses %s and its token may not read attestations: add attestations: read", path.Base(file), name, releaseToolAction)
			}
		}
	}
	if checked == 0 {
		t.Error("no job uses the release tool, so this test checks nothing")
	}
}
