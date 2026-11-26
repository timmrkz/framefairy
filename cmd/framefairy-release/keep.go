package main

import (
	"errors"
	"flag"
	"fmt"
	"io"
	"os"
)

// toolName is the one file of the release release-tool.
const toolName = "framefairy-release"

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

// keepTool puts main's new build of the tool into the release
// release-tool and takes every other file away, like the builds named by
// a key from before. What makes a job trust a build is main's signature
// and its fingerprint, see self.go, never which file the release holds.
// See docs/decisions/2026-11-25-the-release-tool-keys-itself.md.
func keepTool(gh toolStore, data []byte, out io.Writer) error {
	if err := gh.upload(toolName, data); err != nil {
		return err
	}
	fmt.Fprintf(out, "Kept %s in the release release-tool.\n", toolName)
	all, err := gh.assets()
	if err != nil {
		return err
	}
	var errs []error
	for _, a := range all {
		if a.name != toolName {
			errs = append(errs, gh.remove(a))
		}
	}
	return errors.Join(errs...)
}

// keepCommand keeps a build, for release-tool.yml on main.
func keepCommand(args []string, gh toolStore, out io.Writer) error {
	fs := flag.NewFlagSet("keep", flag.ExitOnError)
	file := fs.String("file", "", "the build")
	_ = fs.Parse(args)
	if *file == "" {
		return errors.New("keep needs -file")
	}
	data, err := os.ReadFile(*file)
	if err != nil {
		return err
	}
	return keepTool(gh, data, out)
}
