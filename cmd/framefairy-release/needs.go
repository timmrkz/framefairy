package main

import (
	"fmt"
	"os/exec"
	"strings"
)

// The answers needsBuild gives about a commit of a pull request, against
// the commit its channel's build was made from.
const (
	// needed: the commit changes the app, or what it changes cannot be
	// read. Every doubt answers this: a build too many costs a few minutes
	// of a runner, a build too few is a commit Tim tests without having it.
	needed = "build"
	// alreadyBuilt: it is the commit built, or has the same files.
	alreadyBuilt = "built"
	// docsOnly: it only changes the docs against what is built.
	docsOnly = "docs"
)

// trees reads which files differ between two commits. git does it in a
// checkout, and the tests hand in a repository they made.
type trees interface {
	changed(from, to string) ([]string, error)
}

// needsBuild says whether a pull request's commit needs a build of its
// own, and why. The build asks it before building, and the channel list
// asks it to know whether a pull request has a commit still to come, so
// the two never disagree about a commit: the list once said a build was
// coming that the build had decided against, and the Updates page waited
// for it for good.
//
// It compares against what is built, not against the push before. A push
// that stops the build of the one before and only changes the docs would
// otherwise leave the one before with no build at all.
func needsBuild(t trees, builtFrom, newest string) (string, string) {
	if builtFrom == "" {
		return needed, "The channel has no build yet."
	}
	files, err := t.changed(builtFrom, newest)
	if err != nil {
		return needed, fmt.Sprintf("What changed since %s cannot be read: %v", builtFrom, err)
	}
	if len(files) == 0 {
		return alreadyBuilt, fmt.Sprintf("%s has the same files as the build of %s.", newest, builtFrom)
	}
	for _, f := range files {
		if !strings.HasPrefix(f, "docs/") && !strings.HasSuffix(f, ".md") {
			return needed, fmt.Sprintf("%s changes the app against the build of %s.", newest, builtFrom)
		}
	}
	return docsOnly, fmt.Sprintf("%s only changes the docs against the build of %s.", newest, builtFrom)
}

// gitTrees asks git in the checkout the tool runs in, and fetches a
// commit it does not have, one commit deep, since comparing two commits'
// files needs nothing of their history. A commit the channel list keeps
// is shortened, which git cannot fetch, so its whole name is asked of
// GitHub first.
type gitTrees struct {
	gh gitHub
}

func (g gitTrees) changed(from, to string) ([]string, error) {
	a, err := g.full(from)
	if err != nil {
		return nil, err
	}
	b, err := g.full(to)
	if err != nil {
		return nil, err
	}
	out, err := exec.Command("git", "diff", "--name-only", a, b).Output()
	if err != nil {
		return nil, fmt.Errorf("git diff %s %s: %w", a, b, err)
	}
	return strings.Fields(string(out)), nil
}

func (g gitTrees) full(commit string) (string, error) {
	if out, err := exec.Command("git", "rev-parse", "-q", "--verify", commit+"^{commit}").Output(); err == nil {
		return strings.TrimSpace(string(out)), nil
	}
	sha := commit
	if len(sha) < 40 {
		var err error
		if sha, err = g.gh.commit(commit); err != nil {
			return "", fmt.Errorf("%s cannot be found: %w", commit, err)
		}
	}
	if err := exec.Command("git", "fetch", "-q", "--depth=1", "origin", sha).Run(); err != nil {
		return "", fmt.Errorf("%s cannot be fetched: %w", sha, err)
	}
	return sha, nil
}
