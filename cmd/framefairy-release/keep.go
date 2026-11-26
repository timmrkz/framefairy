package main

import (
	"crypto/sha256"
	"encoding/hex"
	"errors"
	"flag"
	"fmt"
	"io"
	"net/http"
	"net/url"
	"os"
	"strings"
)

// Only main's newest build of the tool is signed. A job takes the tool it
// downloads only when its signature says main's release-tool.yml made it,
// and the build then says itself whether it fits the checkout, see
// self.go. So every older build main signed has its signature taken away
// the moment a newer one is kept, and nobody can put an older build back
// for a job to run. See
// docs/decisions/2026-11-25-the-release-tool-keys-itself.md.

// toolName is the one file of the release release-tool.
const toolName = "framefairy-release"

// toolStore is what keeping the tool asks of GitHub.
type toolStore interface {
	// assets are the files of the release, with their digests.
	assets() ([]asset, error)
	// download is the content of a file of the release.
	download(a asset) ([]byte, error)
	// upload puts a file into the release, in place of one of the same
	// name.
	upload(name string, data []byte) error
	// remove takes a file away from the release.
	remove(a asset) error
	// unsign deletes every signature GitHub keeps for a file by its
	// digest, "sha256:" and hex. One that is gone already is no error.
	unsign(digest string) error
}

// digestOf is a file's digest the way GitHub names it.
func digestOf(data []byte) string {
	sum := sha256.Sum256(data)
	return "sha256:" + hex.EncodeToString(sum[:])
}

// keepTool puts a new build in the release and takes away every other
// file, and the signature of each one that is not this very build.
func keepTool(gh toolStore, data []byte, out io.Writer) error {
	old, err := gh.assets()
	if err != nil {
		return err
	}
	// Every older build's digest first, while each file is still there
	// to be read when GitHub names none.
	was := make([]string, len(old))
	for i, a := range old {
		was[i] = a.digest
		if was[i] != "" {
			continue
		}
		// GitHub names a digest for every file it took since 2025. For
		// one it does not, the file says it.
		b, err := gh.download(a)
		if err != nil {
			return fmt.Errorf("%s has no digest and cannot be read, so its signature would stay: %w", a.name, err)
		}
		was[i] = digestOf(b)
	}
	digest := digestOf(data)
	if err := gh.upload(toolName, data); err != nil {
		return err
	}
	fmt.Fprintf(out, "Kept %s, %s.\n", toolName, digest)
	var errs []error
	for i, a := range old {
		if was[i] != digest {
			if err := gh.unsign(was[i]); err != nil {
				errs = append(errs, fmt.Errorf("the signature of %s, %s, stays: %w", a.name, was[i], err))
			} else {
				fmt.Fprintf(out, "Took the signature of %s, %s, away.\n", a.name, was[i])
			}
		}
		if a.name != toolName {
			if err := gh.remove(a); err != nil {
				errs = append(errs, err)
			}
		}
	}
	return errors.Join(errs...)
}

// keepCommand keeps a build, for release-tool.yml on main.
func keepCommand(args []string, gh toolStore, out io.Writer) error {
	data, err := fileFlag("keep", args)
	if err != nil {
		return err
	}
	return keepTool(gh, data, out)
}

// unsignCommand takes the signature of a build away, for release-tool.yml
// started by hand on a branch: a branch's build is never taken, and
// taking its signature away again shows the token may do it, before main
// relies on it.
func unsignCommand(args []string, gh toolStore, out io.Writer) error {
	data, err := fileFlag("unsign", args)
	if err != nil {
		return err
	}
	digest := digestOf(data)
	if err := gh.unsign(digest); err != nil {
		return err
	}
	fmt.Fprintf(out, "Took the signature of %s away.\n", digest)
	return nil
}

func fileFlag(command string, args []string) ([]byte, error) {
	fs := flag.NewFlagSet(command, flag.ExitOnError)
	file := fs.String("file", "", "the build")
	_ = fs.Parse(args)
	if *file == "" {
		return nil, fmt.Errorf("%s needs -file", command)
	}
	return os.ReadFile(*file)
}

// unsign deletes the signatures of a file by its digest. GitHub keeps them
// under the account that owns the repository.
func (g *restGitHub) unsign(digest string) error {
	owner, _, ok := strings.Cut(g.repo, "/")
	if !ok {
		return fmt.Errorf("GH_REPO %q names no owner", g.repo)
	}
	err := g.call(http.MethodDelete, g.api+"/users/"+url.PathEscape(owner)+"/attestations/digest/"+url.PathEscape(digest), nil, "", nil)
	if errors.Is(err, errNotFound) {
		return nil
	}
	return err
}
