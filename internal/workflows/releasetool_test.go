package workflows

import (
	"fmt"
	"path"
	"path/filepath"
	"regexp"
	"slices"
	"strings"
	"testing"
)

// The tool the jobs that hold a key run, and the action that brings it,
// see docs/decisions/2026-11-24-the-release-tool-built-once.md.
const (
	releaseTool       = "cmd/framefairy-release"
	releaseToolAction = "./.github/actions/release-tool"
)

// The tool works out the key of what it is built from itself, from its
// imports, so nothing in a workflow may list what that is: a list goes
// stale the day the tool imports one more package. release-tool.yml
// starts on every push to main, and the action names no folder but the
// one it builds the tool from, see
// docs/decisions/2026-11-25-the-release-tool-keys-itself.md.
//
// On the real disk: what is read is the repository's own workflow and action.
func TestNoWorkflowListsWhatTheReleaseToolIsBuiltFrom(t *testing.T) {
	t.Parallel()
	var wf struct {
		On struct {
			Push struct {
				Branches    []string `yaml:"branches"`
				Paths       []string `yaml:"paths"`
				PathsIgnore []string `yaml:"paths-ignore"`
			} `yaml:"push"`
		} `yaml:"on"`
	}
	readYAML(t, filepath.Join("..", "..", ".github", "workflows", "release-tool.yml"), &wf)
	if !slices.Equal(wf.On.Push.Branches, []string{"main"}) {
		t.Errorf("release-tool.yml builds on a push to %v, where only main's build may be kept", wf.On.Push.Branches)
	}
	if len(wf.On.Push.Paths) > 0 || len(wf.On.Push.PathsIgnore) > 0 {
		t.Errorf("release-tool.yml starts only for some paths, %v and not %v, and only the tool knows what it is built from", wf.On.Push.Paths, wf.On.Push.PathsIgnore)
	}
	for _, s := range localAction(t)(releaseToolAction) {
		if regexp.MustCompile(`HEAD:|git\s+(rev-parse|ls-tree|ls-files)`).MatchString(s.Run) {
			t.Errorf("%s works something out of the checkout's files itself: %q. The tool says what it is built from", releaseToolAction, s.Run)
		}
		if regexp.MustCompile(`\bgo\s+(run|build)\b`).MatchString(s.Run) && !strings.Contains(s.Run, "framefairy-release self ") {
			t.Errorf("%s builds the tool with no key: %q. Build it with framefairy-release self", releaseToolAction, s.Run)
		}
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
// release-tool.yml signed it, on GitHub's own runner, and only then asks it
// whether it fits the checkout. A branch can upload any file under that
// name: a build of its own, which main never signed, or an older build of
// main's, which carries the key of older code and does not fit. Each of
// these flags is what refuses one of them, and nothing runs the download
// before the check passed.
//
// On the real disk: what is read is the repository's own action.
func TestTheReleaseToolIsCheckedAgainstMainsSignature(t *testing.T) {
	t.Parallel()
	var action struct {
		Runs struct {
			Steps []struct {
				cacheStep `yaml:",inline"`
				ID        string `yaml:"id"`
				If        string `yaml:"if"`
			} `yaml:"steps"`
		} `yaml:"runs"`
	}
	readYAML(t, filepath.Join("..", "..", filepath.FromSlash(strings.TrimPrefix(releaseToolAction, "./")), "action.yml"), &action)
	var fetch *cacheStep
	verify := ""
	ran := 0
	for _, s := range action.Runs.Steps {
		if strings.Contains(s.Run, "gh attestation verify") {
			fetch = &s.cacheStep
			verify = s.ID
		}
		if regexp.MustCompile(`\bfits\b`).MatchString(s.Run) {
			ran++
			if want := "steps." + verify + ".outcome == 'success'"; verify == "" || s.If != want {
				t.Errorf("%s runs the downloaded tool if %q, and should only if %q", releaseToolAction, s.If, want)
			}
			if fetch != nil && fmt.Sprint(s.Env["TOOL"]) != fmt.Sprint(fetch.Env["TOOL"]) {
				t.Errorf("%s runs %v, and checked %v", releaseToolAction, s.Env["TOOL"], fetch.Env["TOOL"])
			}
		}
	}
	if ran == 0 {
		t.Errorf("%s asks no downloaded tool whether it fits, so this test checks nothing", releaseToolAction)
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

// The job that signs the tool as main runs no tool it downloaded: it only
// builds one from its own checkout. A build anybody could upload, run
// there, could make an attestation that says main.
//
// On the real disk: what is read is the repository's own workflows.
func TestTheJobThatSignsTheReleaseToolRunsNothingItDownloaded(t *testing.T) {
	t.Parallel()
	files, err := filepath.Glob(filepath.Join("..", "..", ".github", "workflows", "*.y*ml"))
	if err != nil {
		t.Fatal(err)
	}
	signs := 0
	for _, f := range files {
		var wf struct {
			Permissions any                 `yaml:"permissions"`
			Jobs        map[string]cacheJob `yaml:"jobs"`
		}
		readYAML(t, f, &wf)
		for name, job := range wf.Jobs {
			perms := job.Permissions
			if perms == nil {
				perms = wf.Permissions
			}
			p, _ := perms.(map[string]any)
			if p["id-token"] != "write" {
				continue
			}
			for _, s := range job.Steps {
				if s.Uses != releaseToolAction {
					continue
				}
				signs++
				if fmt.Sprint(s.With["download"]) != "false" {
					t.Errorf("%s: the job %s can sign as this repository and runs the release tool it downloads: give %s download: \"false\"", filepath.Base(f), name, releaseToolAction)
				}
			}
		}
	}
	if signs == 0 {
		t.Error("no job that signs uses the release tool, so this test checks nothing")
	}
}

// release-tool.yml keeps a build only through framefairy-release keep,
// which takes the signature of every older build away, so only main's
// newest build can be handed to a job. A build put into the release any
// other way would leave the one it replaced signed.
//
// On the real disk: what is read is the repository's own workflow.
func TestOnlyTheNewestReleaseToolStaysSigned(t *testing.T) {
	t.Parallel()
	var wf struct {
		Jobs map[string]cacheJob `yaml:"jobs"`
	}
	readYAML(t, filepath.Join("..", "..", ".github", "workflows", "release-tool.yml"), &wf)
	kept := false
	for name, job := range wf.Jobs {
		for _, s := range job.Steps {
			if regexp.MustCompile(`gh\s+release\s+(upload|create|delete-asset)`).MatchString(s.Run) {
				t.Errorf("release-tool.yml: the job %s changes the release by hand: %q. Keep the build with framefairy-release keep", name, s.Run)
			}
			kept = kept || regexp.MustCompile(`(?m)^\s*framefairy-release keep\s`).MatchString(s.Run)
		}
	}
	if !kept {
		t.Error("release-tool.yml keeps no build with framefairy-release keep")
	}
}
