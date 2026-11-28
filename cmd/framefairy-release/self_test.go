package main

import (
	"debug/macho"
	"errors"
	"fmt"
	"io"
	"io/fs"
	"os"
	"os/exec"
	"path/filepath"
	"slices"
	"strings"
	"testing"
)

// The tool is keyed on what it is built from, found the way the app is:
// go.mod, go.sum and every package of ours it imports or its go generate
// runs, and nothing it does not, wherever the folders are.
func TestTheToolIsKeyedOnWhatItIsBuiltFrom(t *testing.T) {
	t.Parallel()
	cases := []struct {
		name  string
		files map[string]string
		dir   string
		want  []string
	}{
		{
			name: "the packages it imports, and those in turn",
			files: map[string]string{
				"cmd/framefairy-release/main.go":      "package main\nimport \"framefairy/updates\"\n",
				"cmd/framefairy-release/main_test.go": "package main\nimport \"framefairy/internal/standin\"\n",
				"updates/updates.go":                  "package updates\nimport \"framefairy/updates/feed\"\n",
				"updates/feed/feed.go":                "package feed\n",
				"engine/run.go":                       "package engine\n",
				"internal/standin/standin.go":         "package standin\n",
			},
			dir:  "cmd/framefairy-release",
			want: []string{"go.mod", "go.sum", "cmd/framefairy-release", "updates", "updates/feed"},
		},
		{
			name: "moved, it is found where it is",
			files: map[string]string{
				"tools/release/main.go": "package main\nimport \"framefairy/updates\"\n",
				"updates/updates.go":    "package updates\n",
			},
			dir:  "tools/release",
			want: []string{"go.mod", "go.sum", "tools/release", "updates"},
		},
		{
			name: "what its go generate runs",
			files: map[string]string{
				"cmd/framefairy-release/main.go":     "package main\n\n//go:generate go run ./gen\n",
				"cmd/framefairy-release/gen/main.go": "package main\nimport \"framefairy/updates\"\n",
				"updates/updates.go":                 "package updates\n",
			},
			dir:  "cmd/framefairy-release",
			want: []string{"go.mod", "go.sum", "cmd/framefairy-release", "cmd/framefairy-release/gen", "updates"},
		},
	}
	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			got, err := toolInputs(goSource(c.files), c.dir)
			if err != nil {
				t.Fatal(err)
			}
			want := append(c.want[:2:2], slices.Sorted(slices.Values(c.want[2:]))...)
			if !slices.Equal(got, want) {
				t.Errorf("the tool is keyed on %v, want %v", got, want)
			}
		})
	}
}

// Go that cannot be read, or a go generate line that cannot be judged,
// gives no key, and so the tool is built here.
func TestAToolThatCannotBeKeyedIsBuilt(t *testing.T) {
	t.Parallel()
	for name, files := range map[string]map[string]string{
		"Go that cannot be read": {
			"cmd/framefairy-release/main.go": "package main\nimport (\n",
		},
		"a go generate line that is no go run of ours": {
			"cmd/framefairy-release/main.go": "package main\n\n//go:generate sh make.sh\n",
		},
	} {
		t.Run(name, func(t *testing.T) {
			_, err := toolInputs(goSource(files), "cmd/framefairy-release")
			if err == nil {
				t.Fatal("the tool got a key")
			}
			if ok, _ := fits("abc", "", err); ok {
				t.Error("a tool with no key fits")
			}
		})
	}
}

// Only a build of the checkout's code says it fits and goes on the job's
// path. Any other answer leaves the path alone, so the job builds the tool.
//
// On the real disk: GitHub reads a job's outputs and its path from files
// it names, and those files are what is checked.
func TestOnlyABuildOfTheCheckoutsCodeFits(t *testing.T) {
	t.Parallel()
	cases := []struct {
		name, stamped, key string
		keyErr             error
		want               bool
	}{
		{"stamped with the checkout's key", "0123456789abcdef", "0123456789abcdef", nil, true},
		{"stamped with another key", "0123456789abcdef", "fedcba9876543210", nil, false},
		{"not stamped", "", "0123456789abcdef", nil, false},
		{"no key for the checkout", "0123456789abcdef", "", errors.New("no commit"), false},
		{"neither", "", "", errors.New("no commit"), false},
	}
	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			t.Parallel()
			tmp := t.TempDir()
			outputs, path := filepath.Join(tmp, "outputs"), filepath.Join(tmp, "path")
			if err := answerFits(io.Discard, c.stamped, c.key, c.keyErr, "/downloaded", outputs, path); err != nil {
				t.Fatal(err)
			}
			said, err := os.ReadFile(outputs)
			if err != nil {
				t.Fatal(err)
			}
			if want := fmt.Sprintf("fits=%v\n", c.want); string(said) != want {
				t.Errorf("the job's outputs say %q, want %q", said, want)
			}
			onPath, err := os.ReadFile(path)
			switch {
			case c.want && string(onPath) != "/downloaded\n":
				t.Errorf("a build that fits is not on the path, which holds %q (%v)", onPath, err)
			case !c.want && !errors.Is(err, fs.ErrNotExist):
				t.Errorf("a build that does not fit is put on the path: %q", onPath)
			}
		})
	}
}

// A build made by self, the way the action makes it, carries the key of
// the checkout and says it fits.
//
// On the real disk: go builds the tool from this repository, and the
// build reads this repository's own commit with git.
func TestABuildOfTheCheckoutFitsIt(t *testing.T) {
	t.Parallel()
	tmp := t.TempDir()
	tool := filepath.Join(tmp, "built", "framefairy-release")
	env := append(os.Environ(), "GITHUB_PATH="+filepath.Join(tmp, "path"), "GITHUB_OUTPUT="+filepath.Join(tmp, "outputs"), "CGO_ENABLED=0")
	build := exec.Command("go", "run", "-trimpath", ".", "self", "-out", tool)
	build.Env = env
	if out, err := build.CombinedOutput(); err != nil {
		t.Fatalf("self: %v\n%s", err, out)
	}
	ask := exec.Command(tool, "fits")
	ask.Env = env
	out, err := ask.CombinedOutput()
	if err != nil {
		t.Fatalf("fits: %v\n%s", err, out)
	}
	said, err := os.ReadFile(filepath.Join(tmp, "outputs"))
	if err != nil {
		t.Fatal(err)
	}
	if string(said) != "fits=true\n" {
		t.Errorf("a build of the checkout says %q, and printed:\n%s", said, out)
	}
}

// self builds the tool for a Mac too, for the job that signs with the
// Developer ID, and leaves that build off this job's path, where it could
// not run.
//
// On the real disk: go builds the tool from this repository, and the
// build reads this repository's own commit with git.
func TestABuildForAMacIsAMacProgramOffThePath(t *testing.T) {
	t.Parallel()
	tmp := t.TempDir()
	tool := filepath.Join(tmp, "darwin", "framefairy-release")
	paths := filepath.Join(tmp, "path")
	build := exec.Command("go", "run", "-trimpath", ".", "self", "-for", "darwin/arm64", "-out", tool)
	build.Env = append(os.Environ(), "GITHUB_PATH="+paths, "CGO_ENABLED=0")
	if out, err := build.CombinedOutput(); err != nil {
		t.Fatalf("self: %v\n%s", err, out)
	}
	f, err := macho.Open(tool)
	if err != nil {
		t.Fatalf("the build for a Mac is no Mac program: %v", err)
	}
	defer f.Close()
	if f.Cpu != macho.CpuArm64 {
		t.Errorf("the build for a Mac is for %v, want arm64", f.Cpu)
	}
	if _, err := os.Stat(paths); !errors.Is(err, os.ErrNotExist) {
		t.Errorf("self put a build for another system on the job's path: %v", err)
	}
}

// The key of this repository's own tool covers every package of ours Go
// builds into it.
//
// On the real disk: git reads this repository's own commit, and go list
// its own packages, with no network.
func TestTheToolsKeyCoversWhatGoBuildsIntoIt(t *testing.T) {
	t.Parallel()
	s, err := gitSource("HEAD")
	if err != nil {
		t.Fatal(err)
	}
	const dir = "cmd/framefairy-release"
	inputs, err := toolInputs(s, dir)
	if err != nil {
		t.Fatal(err)
	}
	cmd := exec.Command("go", "list", "-e", "-deps", "-f", "{{if not .Standard}}{{.ImportPath}}{{end}}", "framefairy/"+dir)
	cmd.Env = append(cmd.Environ(), "GOPROXY=off", "GOFLAGS=-mod=mod")
	out, err := cmd.Output()
	if err != nil {
		t.Fatalf("go list: %v", err)
	}
	for line := range strings.Lines(string(out)) {
		own, ok := strings.CutPrefix(strings.TrimSpace(line), "framefairy/")
		if ok && !slices.Contains(inputs, own) {
			t.Errorf("Go builds %s into the release tool and its key leaves it out", own)
		}
	}

	key, err := gitKey("HEAD", inputs)
	if err != nil {
		t.Fatal(err)
	}
	if len(key) != 16 {
		t.Errorf("the key %q is not 16 hex digits", key)
	}
}

// The key changes with what any input holds, and with where it is: a
// tool built from other code never fits.
func TestTheKeyChangesWithWhatTheToolIsBuiltFrom(t *testing.T) {
	t.Parallel()
	inputs := []string{"go.mod", "go.sum", "cmd/framefairy-release", "updates"}
	objects := []string{"a1", "b2", "c3", "d4"}
	key := keyOf(inputs, objects)
	if keyOf(slices.Clone(inputs), slices.Clone(objects)) != key {
		t.Fatal("the same code gives another key")
	}
	for i := range inputs {
		changed := slices.Clone(objects)
		changed[i] = "e5"
		if keyOf(inputs, changed) == key {
			t.Errorf("a change to %s keeps the key", inputs[i])
		}
		moved := slices.Clone(inputs)
		moved[i] = "elsewhere"
		if keyOf(moved, objects) == key {
			t.Errorf("%s moved keeps the key", inputs[i])
		}
	}
	more := keyOf(append(slices.Clone(inputs), "engine"), append(slices.Clone(objects), "f6"))
	if more == key {
		t.Error("a package more keeps the key")
	}
}
