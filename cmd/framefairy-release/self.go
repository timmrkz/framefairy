package main

import (
	"crypto/sha256"
	"encoding/hex"
	"errors"
	"flag"
	"fmt"
	"io"
	"maps"
	"os"
	"os/exec"
	"path/filepath"
	"runtime/debug"
	"slices"
	"strings"
)

// The tool knows what it is built from, the way it knows what the app is
// built from in reach.go: from its own imports, read out of the commit,
// so no list of folders says it. A build made by self carries the key of
// what it was built from, and fits asks whether a checkout is still that.
// The jobs that hold a key download main's signed build and ask it, and
// build the tool here when it no longer fits. See
// docs/decisions/2026-11-25-the-release-tool-keys-itself.md.

// stamped is the key of what this build of the tool was built from, set
// by self with -ldflags -X. Any other build leaves it empty and never
// fits.
var stamped string

// toolInputs are what the program in dir is built from in s: go.mod,
// go.sum and the folders of every package of ours it imports, or its
// go generate runs, sorted.
func toolInputs(s source, dir string) ([]string, error) {
	b, err := closure(s, programs{dirs: map[string]bool{}, mains: map[string]bool{dir: true}}, packages(s))
	if err != nil {
		return nil, err
	}
	return append([]string{"go.mod", "go.sum"}, slices.Sorted(maps.Keys(b.dirs))...), nil
}

// gitKey is a key of the inputs at a commit: the first 16 hex digits of
// a hash over each one's git object, which changes with any file in it,
// the files a package embeds and its folders below included.
func gitKey(commit string, inputs []string) (string, error) {
	top, err := exec.Command("git", "rev-parse", "--show-toplevel").Output()
	if err != nil {
		return "", fmt.Errorf("git rev-parse --show-toplevel: %w", err)
	}
	args := []string{"rev-parse"}
	for _, in := range inputs {
		if in == "." {
			in = ""
		}
		args = append(args, commit+":"+in)
	}
	cmd := exec.Command("git", args...)
	cmd.Dir = strings.TrimSpace(string(top))
	objects, err := cmd.Output()
	if err != nil {
		return "", fmt.Errorf("git rev-parse of what the tool is built from: %w", err)
	}
	found := strings.Fields(string(objects))
	if len(found) != len(inputs) {
		return "", fmt.Errorf("git named %d objects for %d inputs", len(found), len(inputs))
	}
	return keyOf(inputs, found), nil
}

// keyOf is the key of the inputs by their git objects: the first 16 hex
// digits of a hash over each input's name and object, in order.
func keyOf(inputs, objects []string) string {
	h := sha256.New()
	for i, o := range objects {
		fmt.Fprintf(h, "%s %s\n", inputs[i], o)
	}
	return hex.EncodeToString(h.Sum(nil))[:16]
}

// ownKey is the key of what this program is built from at a commit, found
// by the import path Go stamped into it, wherever its folder is.
func ownKey(commit string) (string, error) {
	info, ok := debug.ReadBuildInfo()
	if !ok || info.Path == "" {
		return "", errors.New("the tool carries no import path of its own")
	}
	s, err := gitSource(commit)
	if err != nil {
		return "", err
	}
	dir, ok := s.own(info.Path)
	if !ok {
		return "", fmt.Errorf("%s is no package of the module %s", info.Path, s.module)
	}
	inputs, err := toolInputs(s, dir)
	if err != nil {
		return "", err
	}
	return gitKey(commit, inputs)
}

// fits says whether a build stamped with one key is the build of the
// checkout, whose key is key, or why not. Every doubt builds the tool.
func fits(stamped, key string, keyErr error) (bool, string) {
	switch {
	case keyErr != nil:
		return false, "What the release tool is built from cannot be worked out, so it is built here: " + keyErr.Error()
	case stamped == "":
		return false, "The release tool says nothing of what it was built from, so it is built here."
	case stamped != key:
		return false, "The signed release tool was built from other code than this commit's, so it is built here."
	}
	return true, "The signed release tool is built from this commit's code."
}

// fitsCommand answers fits=true or fits=false for the action, and puts
// this program's folder on the job's path when it fits.
func fitsCommand(out io.Writer) error {
	key, keyErr := ownKey("HEAD")
	exe, err := os.Executable()
	if err != nil {
		return err
	}
	return answerFits(out, stamped, key, keyErr, filepath.Dir(exe), os.Getenv("GITHUB_OUTPUT"), os.Getenv("GITHUB_PATH"))
}

// answerFits says in the job's outputs whether a build stamped with stamp
// is the build of the checkout's key, and puts the build's folder on the
// job's path only when it is. A build that does not fit never reaches a
// later step.
func answerFits(out io.Writer, stamp, key string, keyErr error, dir, outputs, path string) error {
	ok, why := fits(stamp, key, keyErr)
	fmt.Fprintln(out, why)
	var p plan
	p.set("fits", fmt.Sprint(ok))
	if ok {
		if err := onPath(path, dir); err != nil {
			return err
		}
	}
	return p.write(out, outputs)
}

// selfCommand builds this program from the checkout, stamped with the key
// of what it is built from, with no C, and puts it on the job's path. A
// checkout whose key cannot be worked out fails here, on the pull request
// that made it so, rather than leaving main a build that never fits.
func selfCommand(args []string, out io.Writer) error {
	fs := flag.NewFlagSet("self", flag.ExitOnError)
	to := fs.String("out", "", "where to write the build")
	_ = fs.Parse(args)
	if *to == "" {
		return errors.New("self needs -out")
	}
	info, ok := debug.ReadBuildInfo()
	if !ok || info.Path == "" {
		return errors.New("the tool carries no import path of its own")
	}
	key, err := ownKey("HEAD")
	if err != nil {
		return fmt.Errorf("what the release tool is built from cannot be worked out, so no build of it could ever fit: %w", err)
	}
	abs, err := filepath.Abs(*to)
	if err != nil {
		return err
	}
	build := exec.Command("go", "build", "-trimpath", "-ldflags=-X main.stamped="+key, "-o", abs, info.Path)
	build.Env = append(os.Environ(), "CGO_ENABLED=0")
	build.Stdout, build.Stderr = out, os.Stderr
	if err := build.Run(); err != nil {
		return fmt.Errorf("go build %s: %w", info.Path, err)
	}
	fmt.Fprintf(out, "Built the release tool, key %q.\n", key)
	return onPath(os.Getenv("GITHUB_PATH"), filepath.Dir(abs))
}

// onPath adds a folder to the path of the job's later steps, through the
// file GitHub names in GITHUB_PATH.
func onPath(file, dir string) error {
	if file == "" {
		return errors.New("GITHUB_PATH is not set, so there is no path to put the tool on")
	}
	f, err := os.OpenFile(file, os.O_APPEND|os.O_CREATE|os.O_WRONLY, 0o644)
	if err != nil {
		return err
	}
	if _, err := fmt.Fprintln(f, dir); err != nil {
		f.Close()
		return err
	}
	return f.Close()
}
