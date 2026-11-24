package workflows

import (
	"go/parser"
	"go/token"
	"os"
	"path"
	"path/filepath"
	"slices"
	"strconv"
	"strings"
	"testing"

	"gopkg.in/yaml.v3"
)

// The walks start for the Go the app builds, and not for a program that
// runs on no customer's machine: the filter walked in ci.yml leaves out
// only folders that neither the app nor its decoder imports, in any file,
// their tests and every system's files included. Each folder it leaves
// out is there, so a folder that moved does not stay left out by a name
// nothing has. See One way to test in docs/BUILD.md.
//
// On the real disk: what is read is the repository's own workflows and
// the Go files the app imports.
func TestTheWalksStartForTheGoTheAppBuilds(t *testing.T) {
	t.Parallel()
	root := filepath.Join("..", "..")
	var wf struct {
		Jobs map[string]struct {
			Outputs map[string]string `yaml:"outputs"`
			Steps   []struct {
				ID   string            `yaml:"id"`
				With map[string]string `yaml:"with"`
			} `yaml:"steps"`
		} `yaml:"jobs"`
	}
	readYAML(t, filepath.Join(root, ".github", "workflows", "ci.yml"), &wf)
	var filters map[string][]string
	for _, s := range wf.Jobs["changes"].Steps {
		if s.ID == "filter" {
			if err := yaml.Unmarshal([]byte(s.With["filters"]), &filters); err != nil {
				t.Fatal(err)
			}
		}
	}
	walked, golang := filters["walked"], filters["go"]
	if len(walked) == 0 || len(golang) == 0 {
		t.Fatal("ci.yml's job changes has no filter walked or go, so this test reads it wrong")
	}
	if app := wf.Jobs["changes"].Outputs["app"]; !strings.Contains(app, "steps.filter.outputs.walked == 'true'") || strings.Contains(app, "outputs.go ") {
		t.Errorf("the output app is %q, not started by the filter walked", app)
	}
	if !slices.Equal(walked[:min(len(golang), len(walked))], golang) {
		t.Errorf("the filter walked begins %q, not with the filter go, %q, which it leaves more out of", walked, golang)
	}
	var out []string
	for _, g := range walked[len(golang):] {
		dir, ok := strings.CutSuffix(strings.TrimPrefix(g, "!"), "/**")
		if !strings.HasPrefix(g, "!") || !ok || strings.ContainsAny(dir, "*?{[") {
			t.Errorf("the filter walked leaves out %q, not one folder as !folder/**", g)
			continue
		}
		if fi, err := os.Stat(filepath.Join(root, filepath.FromSlash(dir))); err != nil || !fi.IsDir() {
			t.Errorf("the filter walked leaves out %s, which is no folder", dir)
		}
		out = append(out, dir)
	}
	for dir := range imported(t, root, "cmd/framefairy-app", "cmd/framefairy-frames") {
		for _, o := range out {
			if dir == o || strings.HasPrefix(dir, o+"/") {
				t.Errorf("the filter walked leaves out %s, and the app imports %s: a change there must start the walks", o, dir)
			}
		}
	}
}

// imported is every folder of this module the given ones import, they
// themselves too, read from all their Go files whatever system or tag
// each is for, tests included.
func imported(t *testing.T, root string, from ...string) map[string]bool {
	t.Helper()
	seen := map[string]bool{}
	todo := slices.Clone(from)
	for len(todo) > 0 {
		dir := todo[len(todo)-1]
		todo = todo[:len(todo)-1]
		if seen[dir] {
			continue
		}
		seen[dir] = true
		files, err := filepath.Glob(filepath.Join(root, filepath.FromSlash(dir), "*.go"))
		if err != nil || len(files) == 0 {
			t.Fatalf("%s has no Go file to read: %v", dir, err)
		}
		for _, f := range files {
			file, err := parser.ParseFile(token.NewFileSet(), f, nil, parser.ImportsOnly)
			if err != nil {
				t.Fatal(err)
			}
			for _, spec := range file.Imports {
				p, _ := strconv.Unquote(spec.Path.Value)
				if rest, ok := strings.CutPrefix(p, "framefairy/"); ok {
					todo = append(todo, path.Clean(rest))
				}
			}
		}
	}
	return seen
}
