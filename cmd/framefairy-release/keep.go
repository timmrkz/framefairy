package main

import (
	"errors"
	"flag"
	"fmt"
	"io"
	"maps"
	"os"
	"slices"
)

// toolName is the build of the tool for Linux in the release
// release-tool, and toolMac the one for a Mac, which the job that signs
// with the Developer ID runs, see devid.go.
const (
	toolName = "framefairy-release"
	toolMac  = "framefairy-release-darwin-arm64"
)

// toolStore is what keeping the tool asks of GitHub.
type toolStore interface {
	// assets are the files of the release.
	assets() ([]asset, error)
	// upload puts a file into the release, in place of one of the same
	// name, and makes the release first if there is none.
	upload(name string, data []byte) error
	// remove takes a file away from the release.
	remove(a asset) error
}

// keepTool puts main's new builds of the tool into the release
// release-tool, by name, and takes every other file away, like the
// builds named by a key from before. What makes a job trust a build is
// main's signature and its fingerprint, see self.go, never which file the
// release holds. See docs/decisions/2026-11-25-the-release-tool-keys-itself.md.
func keepTool(gh toolStore, builds map[string][]byte, out io.Writer) error {
	names := slices.Sorted(maps.Keys(builds))
	for _, name := range names {
		if err := gh.upload(name, builds[name]); err != nil {
			return err
		}
		fmt.Fprintf(out, "Kept %s in the release release-tool.\n", name)
	}
	all, err := gh.assets()
	if err != nil {
		return err
	}
	var errs []error
	for _, a := range all {
		if !slices.Contains(names, a.name) {
			errs = append(errs, gh.remove(a))
		}
	}
	return errors.Join(errs...)
}

// keepCommand keeps a build, for release-tool.yml on main.
func keepCommand(args []string, gh toolStore, out io.Writer) error {
	fs := flag.NewFlagSet("keep", flag.ExitOnError)
	file := fs.String("file", "", "the build for Linux")
	mac := fs.String("mac", "", "the build for a Mac")
	_ = fs.Parse(args)
	if *file == "" || *mac == "" {
		return errors.New("keep needs -file and -mac")
	}
	builds := map[string][]byte{}
	for name, f := range map[string]string{toolName: *file, toolMac: *mac} {
		data, err := os.ReadFile(f)
		if err != nil {
			return err
		}
		builds[name] = data
	}
	return keepTool(gh, builds, out)
}
