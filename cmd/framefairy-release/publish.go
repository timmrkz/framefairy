package main

import (
	"bufio"
	"bytes"
	"encoding/json"
	"fmt"
	"io"
	"os"
	"path/filepath"
	"strings"

	"framefairy/updates"
)

// publish uploads a signed build and its entry to the release dev, and
// marks a pull request's commit ready to update to.
//
// The zip goes up first and the entry only once it is there, because the
// app reads the entry of the channel it follows and would be offered a
// build it cannot download yet. Issue #420.
//
// The commit is marked here, the moment the entry is up, and not when the
// list is written: the list shares its queue with the list written as a
// build starts, GitHub keeps one waiting job a queue and cancels the one
// before, and a cancelled list took the mark with it, so the builds of
// #798 and #816 never said they were ready, #839. The app reads the entry
// of the channel it follows, so the build is there to update to once the
// entry is up, whenever the list follows.
//
// A build pack wrote, in the folder packed, also uploads its pieces and
// its parts: first every piece the release does not hold yet, then the
// zip, then the parts, then the entry, so nothing an entry names is
// missing when the app reads it. A piece is named by the SHA-256 of the
// file it holds, so one that is there already is the same file and stays.
func publish(gh gitHub, zipPath, zipName, packed, entryPath, channel, commit string) error {
	zip, err := os.ReadFile(zipPath)
	if err != nil {
		return err
	}
	entry, err := os.ReadFile(entryPath)
	if err != nil {
		return err
	}
	var parts []byte
	if packed != "" {
		if parts, err = uploadPieces(gh, packed); err != nil {
			return err
		}
	}
	if err := gh.upload(zipName, zip); err != nil {
		return err
	}
	if parts != nil {
		if err := gh.upload(updates.PartsName(zipName), parts); err != nil {
			return err
		}
	}
	if err := gh.upload("channel-"+channel+".json", entry); err != nil {
		return err
	}
	// This workflow runs from main, so its checks show on main's commit
	// and not on the pull request. The pull request learns its build is
	// ready from a status on the commit that was built.
	if channel == "main" {
		return nil
	}
	return gh.setReady(commit, "success", "On the Updates page as "+channel, "")
}

// uploadPieces uploads every piece of the build pack wrote to packed that
// the release does not hold yet, and hands back its parts.
func uploadPieces(gh gitHub, packed string) ([]byte, error) {
	data, err := os.ReadFile(filepath.Join(packed, partsFile))
	if err != nil {
		return nil, err
	}
	var p updates.Parts
	if err := json.Unmarshal(data, &p); err != nil {
		return nil, err
	}
	all, err := gh.assets()
	if err != nil {
		return nil, err
	}
	there := map[string]bool{}
	for _, a := range all {
		there[a.name] = true
	}
	for _, part := range p.Parts {
		name := updates.PieceName(part.SHA256)
		if part.Bytes != nil || there[name] {
			continue
		}
		piece, err := os.ReadFile(filepath.Join(packed, name))
		if err != nil {
			return nil, err
		}
		if err := gh.upload(name, piece); err != nil {
			return nil, err
		}
		there[name] = true
	}
	return data, nil
}

// notPublished marks a commit whose build was not published, whatever
// failed: the check or the signer refused it, or a step on the way went
// wrong. The pull request shows why on the commit, issue #369, and the
// channel list reads the mark and stops saying a build is coming, #840.
// refusal is what check or sign wrote when they refused, if anything.
func notPublished(gh gitHub, commit string, refusal []byte, run string) error {
	why := "Not published: a step of its publishing failed, see the run"
	if text := strings.TrimSpace(string(refusal)); text != "" {
		why = "Not signed: " + firstFinding(text)
	}
	return gh.setReady(commit, "failure", why[:min(len(why), 140)], run)
}

// firstFinding is the first thing a refusal names, the first indented
// line of the list check gives, or its first line.
func firstFinding(text string) string {
	s := bufio.NewScanner(strings.NewReader(text))
	first := ""
	for s.Scan() {
		line := s.Text()
		if first == "" {
			first = line
		}
		if strings.HasPrefix(line, "  ") {
			return strings.TrimSpace(line)
		}
	}
	return strings.TrimPrefix(first, "framefairy-release: ")
}

// refused keeps what a command refused in the file a later step reads,
// as well as saying it.
func refused(file string, err error, w io.Writer) error {
	if err == nil || file == "" {
		return err
	}
	var b bytes.Buffer
	fmt.Fprintf(&b, "%v\n", err)
	if werr := os.WriteFile(file, b.Bytes(), 0o644); werr != nil {
		fmt.Fprintf(w, "the refusal could not be kept in %s: %v\n", file, werr)
	}
	return err
}
