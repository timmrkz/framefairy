package main

import (
	"errors"
	"maps"
	"os/exec"
	"path"
	"slices"
	"strings"
	"testing"
)

// goSource is a module's Go, written out as a test needs it.
func goSource(files map[string]string) source {
	s := source{module: "framefairy", files: map[string][]byte{}}
	for name, text := range files {
		s.files[name] = []byte(text)
	}
	return s
}

// A module shaped like ours: the app and the program beside it it ships,
// the engine both import, the release tool, which imports updates too,
// and a test helper only the app's tests import.
var shaped = goSource(map[string]string{
	"cmd/framefairy-app/main.go":          "package main\nimport (\n\t\"fmt\"\n\t\"framefairy/engine\"\n\t\"framefairy/updates\"\n)\n",
	"cmd/framefairy-app/main_test.go":     "package main\nimport \"framefairy/internal/standin\"\n",
	"cmd/framefairy-app/mac_darwin.go":    "//go:build darwin\n\npackage main\nimport \"framefairy/asr\"\n",
	"cmd/framefairy-frames/main.go":       "package main\nimport \"framefairy/engine/num\"\n",
	"engine/run.go":                       "package engine\nimport \"framefairy/engine/num\"\n",
	"engine/run_test.go":                  "package engine\nimport \"framefairy/internal/fixtures\"\n",
	"engine/num/num.go":                   "package num\n",
	"engine/testdata/stub.go":             "package stub\n",
	"asr/asr.go":                          "package asr\n",
	"updates/updates.go":                  "package updates\n",
	"internal/standin/standin.go":         "package standin\n",
	"internal/fixtures/fixtures.go":       "package fixtures\n",
	"cmd/framefairy-release/main.go":      "package main\nimport \"framefairy/updates\"\n",
	"cmd/framefairy-release/main_test.go": "package main\n",
	"cmd/framefairy-dispenser/main.go":    "package main\nimport \"framefairy/licence/dispenser\"\n",
	"licence/dispenser/dispenser.go":      "package dispenser\n",
	"_old/framefairy-frames/main.go":      "package main\nimport \"framefairy/internal/standin\"\n",
	"scripts/models/main.go":              "package main\n",
})

// A change reaches the app by what the app's programs import, the way Go
// itself builds them, whatever the folders are called. Only Go is judged:
// every other file reaches the app, since what reads it cannot be known.
// The walks run the app's own tests too, and the build never ships a test.
func TestAChangeReachesTheAppByWhatItsProgramsImport(t *testing.T) {
	t.Parallel()
	build, err := appFrom(shaped, false)
	if err != nil {
		t.Fatal(err)
	}
	walks, err := appFrom(shaped, true)
	if err != nil {
		t.Fatal(err)
	}
	cases := []struct {
		file         string
		build, walks bool
	}{
		{"cmd/framefairy-app/main.go", true, true},
		{"cmd/framefairy-app/mac_darwin.go", true, true},
		{"asr/asr.go", true, true},
		{"engine/run.go", true, true},
		{"engine/num/num.go", true, true},
		{"updates/updates.go", true, true},
		{"cmd/framefairy-frames/main.go", true, true},
		{"cmd/framefairy-app/main_test.go", false, true},
		{"internal/standin/standin.go", false, true},
		{"engine/run_test.go", false, false},
		{"internal/fixtures/fixtures.go", false, false},
		{"cmd/framefairy-release/main.go", false, false},
		{"cmd/framefairy-release/main_test.go", false, false},
		{"cmd/framefairy-dispenser/main.go", false, false},
		{"licence/dispenser/dispenser.go", false, false},
		{"scripts/models/main.go", false, false},
		{"a/package/that/is/gone.go", false, false},
		{"engine/testdata/stub.go", true, true},
		{"cmd/framefairy-release/testdata/answer.json", true, true},
		{"engine/ink.c", true, true},
		{"scripts/build-ffmpeg.sh", true, true},
		{"Makefile", true, true},
		{"go.mod", true, true},
		{path.Join("frontend", "src", "App.svelte"), true, true},
		{".github/workflows/ci.yml", true, true},
		{path.Join("docs", "BUILD.md"), false, false},
		{"cmd/framefairy-app/README.md", false, false},
		{".claude/skills/testing/SKILL.md", false, false},
		{".claude/settings.json", false, false},
	}
	for _, c := range cases {
		if got := build.reaches(c.file); got != c.build {
			t.Errorf("a change to %s reaches the build: %v, want %v", c.file, got, c.build)
		}
		if got := walks.reaches(c.file); got != c.walks {
			t.Errorf("a change to %s reaches the walks: %v, want %v", c.file, got, c.walks)
		}
	}
}

// A program is found by its name, and the folder it lives in may be
// called anything. One the app does not ship, or that Go never builds, is
// not the app, whatever it is named.
func TestTheAppsProgramsAreFoundByTheirNames(t *testing.T) {
	t.Parallel()
	moved := goSource(map[string]string{
		"apps/desktop/framefairy-app/main.go": "package main\nimport \"framefairy/core\"\n",
		"core/core.go":                        "package core\n",
		"cmd/framefairy-release/main.go":      "package main\n",
	})
	app, err := appFrom(moved, false)
	if err != nil {
		t.Fatal(err)
	}
	if !app.reaches("core/core.go") || app.reaches("cmd/framefairy-release/main.go") {
		t.Errorf("the app moved to apps/desktop is built from %v", slices.Sorted(maps.Keys(app.dirs)))
	}

	for name, s := range map[string]source{
		"no program the app ships": goSource(map[string]string{"cmd/framefairy-release/main.go": "package main\n"}),
		"a library named like one": goSource(map[string]string{"framefairy-app/app.go": "package app\n"}),
		"one Go never builds":      goSource(map[string]string{"testdata/framefairy-app/main.go": "package main\n"}),
	} {
		if _, err := appFrom(s, false); err == nil {
			t.Errorf("%s: the app was found", name)
		}
	}
	broken := goSource(map[string]string{"cmd/framefairy-app/main.go": "package main\nimport (\n"})
	if _, err := appFrom(broken, false); err == nil {
		t.Error("a program whose imports cannot be read was taken as built from nothing")
	}
}

// A program the app's build runs through go generate, like the one that
// writes the licences the app carries, is built with it, and so is
// everything that program imports. A directive that runs anything else
// cannot be judged, and everything runs.
func TestWhatTheAppsBuildRunsIsTheApp(t *testing.T) {
	t.Parallel()
	app := "package main\n\n//go:generate go -C ../.. run ./notices/gen -built cmd/framefairy-app/dist/licences.json\n//go:embed all:dist\nvar dist embed.FS\n"
	s := goSource(map[string]string{
		"cmd/framefairy-app/main.go":     app,
		"notices/gen/main.go":            "package main\nimport \"framefairy/notices/licence\"\n",
		"notices/licence/licence.go":     "package licence\n",
		"cmd/framefairy-frames/main.go":  "package main\n\n//go:generate go run framefairy/tools/table -out table.go\n",
		"tools/table/main.go":            "package main\n",
		"cmd/framefairy-release/main.go": "package main\n",
	})
	for _, tests := range []bool{false, true} {
		b, err := appFrom(s, tests)
		if err != nil {
			t.Fatal(err)
		}
		for _, f := range []string{"notices/gen/main.go", "notices/licence/licence.go", "tools/table/main.go"} {
			if !b.reaches(f) {
				t.Errorf("tests %v: the app's build runs what %s is part of, and a change to it would not reach the app", tests, f)
			}
		}
		if b.reaches("cmd/framefairy-release/main.go") {
			t.Errorf("tests %v: the release tool was taken as part of the app", tests)
		}
	}
	for _, directive := range []string{
		"stringer -type=Kind",
		"go run golang.org/x/tools/cmd/stringer -type=Kind",
		"go -C ../.. build ./notices/gen",
		"go run -tags production ./notices/gen",
		"go",
	} {
		s := goSource(map[string]string{"cmd/framefairy-app/main.go": "package main\n\n//go:generate " + directive + "\n"})
		if _, err := appFrom(s, false); err == nil {
			t.Errorf("go generate runs %q, which cannot be judged, and the app was taken as built without it", directive)
		}
	}
}

// What the walks would build is said for every doubt as something to
// walk: a change that cannot be read, Go that cannot be read, an app that
// cannot be found.
func TestEveryDoubtIsWalked(t *testing.T) {
	t.Parallel()
	gone := errors.New("gone")
	cases := []struct {
		name  string
		trees memTrees
		walks string
	}{
		{"the release tool alone", memTrees{files: []string{"cmd/framefairy-release/main.go"}, src: shaped}, "false"},
		{"a test of the engine and the docs", memTrees{files: []string{"engine/run_test.go", path.Join("docs", "BUILD.md")}, src: shaped}, "false"},
		{"the engine", memTrees{files: []string{"cmd/framefairy-release/main.go", "engine/run.go"}, src: shaped}, "true"},
		{"a script", memTrees{files: []string{"scripts/check.sh"}, src: shaped}, "true"},
		{"a change that cannot be read", memTrees{changedErr: gone, src: shaped}, "true"},
		{"Go that cannot be read", memTrees{files: []string{"cmd/framefairy-release/main.go"}, srcErr: gone}, "true"},
		{"no app", memTrees{files: []string{"cmd/framefairy-release/main.go"}, src: goSource(map[string]string{"cmd/framefairy-release/main.go": "package main\n"})}, "true"},
	}
	for _, c := range cases {
		p := walksPlan(c.trees, "HEAD^1", "HEAD")
		if got := outputs(p)["walks"]; got != c.walks {
			t.Errorf("%s: walks %q, %s, want %q", c.name, got, p.notice, c.walks)
		}
		if p.notice == "" {
			t.Errorf("%s: the run would not say why", c.name)
		}
	}
}

// memTrees answers from memory, one change and one commit's Go.
type memTrees struct {
	files      []string
	changedErr error
	src        source
	srcErr     error
}

func (m memTrees) changed(string, string) ([]string, error) { return m.files, m.changedErr }
func (m memTrees) source(string) (source, error)            { return m.src, m.srcErr }

// What the app is built from, worked out from this repository's own
// code, covers every package of ours that Go itself builds into the app's
// programs, for the Mac and for Linux, and into the app's tests, which the
// walks run.
//
// On the real disk: git reads this repository's own commit, and go list
// its own packages, with no network.
func TestTheAppIsBuiltFromWhatGoSaysItIs(t *testing.T) {
	t.Parallel()
	s, err := gitSource("HEAD")
	if err != nil {
		t.Fatal(err)
	}
	var mains []string
	for _, tests := range []bool{false, true} {
		app, err := appFrom(s, tests)
		if err != nil {
			t.Fatal(err)
		}
		mains = slices.Sorted(maps.Keys(app.mains))
		for _, goos := range []string{"darwin", "linux"} {
			args := []string{"list", "-e", "-deps", "-f", "{{if not .Standard}}{{.ImportPath}}{{end}}"}
			if tests {
				args = append(args, "-test")
			}
			for _, m := range mains {
				args = append(args, "framefairy/"+m)
			}
			cmd := exec.Command("go", args...)
			cmd.Env = append(cmd.Environ(), "GOOS="+goos, "GOPROXY=off", "GOFLAGS=-mod=mod")
			out, err := cmd.Output()
			if err != nil {
				t.Fatalf("go %v: %v", args, err)
			}
			for line := range strings.Lines(string(out)) {
				pkg, _, _ := strings.Cut(strings.TrimSpace(line), " ")
				own, ok := strings.CutPrefix(pkg, "framefairy/")
				if !ok || strings.HasSuffix(own, ".test") {
					continue
				}
				if !app.dirs[own] {
					t.Errorf("Go builds %s into the app on %s, tests %v, and the release tool would not see a change to it", own, goos, tests)
				}
			}
		}
	}
	if !slices.Equal(mains, []string{"cmd/framefairy-app", "cmd/framefairy-frames"}) {
		t.Errorf("the app's programs are %v", mains)
	}
}
