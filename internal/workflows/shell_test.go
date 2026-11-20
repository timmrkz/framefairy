package workflows

import (
	"fmt"
	"io/fs"
	"os"
	"path/filepath"
	"slices"
	"strings"
	"testing"

	"gopkg.in/yaml.v3"
)

// Why the shell is held to what it is: a decision in a script is tested
// by nothing, or by a test in shell of its own, and the update process
// grew three of them in one pull request. See the decision record.
const shellRecord = "docs/decisions/2026-11-14-update-decisions-in-go.md"

// The scripts there are. Each builds a program we ship, sets a machine up
// or packs what make made, and none decides what a build is or where it
// goes.
var scripts = []string{
	"scripts/build-ffmpeg.sh",
	"scripts/build-llama.sh",
	"scripts/build-tools.sh",
	"scripts/bundle-macos.sh",
	"scripts/carry-libs.sh",
	"scripts/check.sh",
	"scripts/cloud-decoder.sh",
	"scripts/cloud-setup.sh",
	"scripts/cloud-webkit.sh",
	"scripts/hyphenation.sh",
	"scripts/make-icon.sh",
	"scripts/pack-source.sh",
	"scripts/pack-tools.sh",
	"scripts/pkg-config-static.sh",
	"scripts/speech-libs.sh",
	"scripts/tool-sums.sh",
	"scripts/tools.sh",
	"scripts/upstream.sh",
	"scripts/verify-tools.sh",
}

// No new shell script. What a workflow or make has to decide is a
// command of the Go program that owns it, tested with go test, the way
// cmd/framefairy-release decides everything about a build.
//
// On the real disk: what is read is the repository's own files.
func TestNoNewShellScript(t *testing.T) {
	t.Parallel()
	root := filepath.Join("..", "..")
	var found []string
	err := filepath.WalkDir(root, func(p string, d fs.DirEntry, err error) error {
		if err != nil {
			return err
		}
		if d.IsDir() && p != root {
			// What make and npm build or fetch, and git's own folder.
			n := d.Name()
			if n == "node_modules" || (strings.HasPrefix(n, ".") && n != ".github" && n != ".claude") ||
				(filepath.Dir(p) == root && (n == "bin" || n == "dist")) {
				return filepath.SkipDir
			}
			return nil
		}
		rel, _ := filepath.Rel(root, p)
		switch filepath.Ext(p) {
		case ".sh", ".bash", ".bats", ".zsh":
			found = append(found, filepath.ToSlash(rel))
		case "":
			// A script with no ending is still a script, by its first line.
			if shebang(t, p) {
				found = append(found, filepath.ToSlash(rel))
			}
		}
		return nil
	})
	if err != nil {
		t.Fatal(err)
	}
	for _, f := range found {
		if !slices.Contains(scripts, f) {
			t.Errorf("%s is a new shell script. Put what it decides into the Go program that owns it, as a command with go test tests, and have the workflow or make run that command. See %s", f, shellRecord)
		}
	}
	for _, s := range scripts {
		if !slices.Contains(found, s) {
			t.Errorf("%s is gone, so take it off the list in this test", s)
		}
	}
}

// shebang says whether a file starts the way a shell script does.
//
// On the real disk: what is read is the repository's own files.
func shebang(t *testing.T, path string) bool {
	t.Helper()
	f, err := os.Open(path)
	if err != nil {
		t.Fatal(err)
	}
	defer f.Close()
	head := make([]byte, 64)
	n, _ := f.Read(head)
	line, _, _ := strings.Cut(string(head[:n]), "\n")
	if !strings.HasPrefix(line, "#!") {
		return false
	}
	for _, sh := range []string{"/sh", "/bash", "/zsh", "/dash", " sh", " bash", " zsh"} {
		if strings.Contains(line, sh) {
			return true
		}
	}
	return false
}

// The steps that were longer than five lines of shell when the rule came,
// with their length then. They may get shorter, never longer, and a step
// that gets shorter lowers its number here.
var longSteps = map[string]int{
	".github/actions/prepare/action.yml: pinned-go":                   6,
	"ci-image.yml image: tag":                                         6,
	"ci-image.yml image: The image, unless it is there":               11,
	"mac-nightly.yml outside: A keychain of its own":                  6,
	"conflicts.yml tell: step 1":                                      22,
	"security.yml upstream: Newer releases of what we ship":           17,
	"speechbench.yml macos: A recorded talk":                          8,
	"tools.yml release: Make the release and attach everything to it": 9,
}

// The most lines of shell a workflow step runs, comments and blank lines
// aside. A command and its error, or a few commands in a row, fit. A
// decision with branches and loops does not, and goes into Go. A folded
// run, >-, is one line however it is wrapped, which is what it is for: one
// command with long arguments. Folding a decision into it to pass is
// what the review reads for.
const mostShell = 5

// A workflow step runs a command or two, never a script written into it.
//
// On the real disk: what is read is the repository's own workflows.
func TestNoWorkflowStepIsAScript(t *testing.T) {
	t.Parallel()
	seen := map[string]bool{}
	for _, f := range actionFiles(t) {
		b, err := os.ReadFile(f)
		if err != nil {
			t.Fatal(err)
		}
		type step struct {
			ID   string `yaml:"id"`
			Name string `yaml:"name"`
			Run  string `yaml:"run"`
		}
		var wf struct {
			Jobs map[string]struct {
				Steps []step `yaml:"steps"`
			} `yaml:"jobs"`
			Runs struct {
				Steps []step `yaml:"steps"`
			} `yaml:"runs"`
		}
		if err := yaml.Unmarshal(b, &wf); err != nil {
			t.Fatalf("%s: %v", f, err)
		}
		name := filepath.Base(f)
		if name == "action.yml" {
			name = filepath.ToSlash(filepath.Join(".github", "actions", filepath.Base(filepath.Dir(f)), name))
		}
		lists := map[string][]step{}
		for job, j := range wf.Jobs {
			lists[name+" "+job] = j.Steps
		}
		if len(wf.Runs.Steps) > 0 {
			lists[name] = wf.Runs.Steps
		}
		for where, steps := range lists {
			for i, s := range steps {
				label := s.Name
				if label == "" {
					label = s.ID
				}
				if label == "" {
					label = fmt.Sprintf("step %d", i)
				}
				key := where + ": " + label
				n := shellLines(s.Run)
				most, listed := longSteps[key]
				seen[key] = listed
				switch {
				case listed && n > most:
					t.Errorf("%s runs %d lines of shell, more than the %d it had when the rule came. Move what it decides into Go, see %s", key, n, most, shellRecord)
				case listed && n < most:
					t.Errorf("%s runs %d lines of shell now, fewer than the %d in longSteps: lower its number there, or take it off the list at %d or fewer", key, n, most, mostShell)
				case !listed && n > mostShell:
					t.Errorf("%s runs %d lines of shell, over %d. Put what it decides into the Go program that owns it, as a command with go test tests, and run that command in one line. See %s", key, n, mostShell, shellRecord)
				}
			}
		}
	}
	for key := range longSteps {
		if !seen[key] {
			t.Errorf("longSteps names %q, which is no step any more: take it off the list", key)
		}
	}
}

// shellLines counts the lines of a step's run that the shell runs. A
// folded >- block is one line already, however it is wrapped.
func shellLines(run string) int {
	n := 0
	for _, l := range strings.Split(run, "\n") {
		if l = strings.TrimSpace(l); l != "" && !strings.HasPrefix(l, "#") {
			n++
		}
	}
	return n
}
