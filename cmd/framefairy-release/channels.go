package main

import (
	"crypto/ed25519"
	"encoding/json"
	"fmt"
	"io"
	"path"
	"sort"
	"strconv"
	"strings"
	"time"

	"framefairy/updates"
)

// ended says whether the build of a commit ended without a build to
// update to, and why: it failed, it was cancelled, or its publishing
// failed and marked the commit. Then no build is coming until the next
// push, and the list must not say one is, #840. Every doubt answers that
// one is still coming, which is how it was before.
func ended(gh gitHub, commit string) (bool, string) {
	state, err := gh.ready(commit)
	if err != nil {
		return false, fmt.Sprintf("the statuses of %s cannot be read: %v", commit, err)
	}
	if state == "failure" {
		return true, commit + " was not published, see its Ready to update to"
	}
	status, conclusion, err := gh.buildRun(commit)
	switch {
	case err != nil:
		return false, fmt.Sprintf("the builds of %s cannot be read: %v", commit, err)
	case status == "":
		return false, "the build of " + commit + " is not there yet"
	case status != "completed":
		return false, "the build of " + commit + " is " + strings.ReplaceAll(status, "_", " ")
	case conclusion == "success":
		return false, "the build of " + commit + " ended well and is being published"
	default:
		return true, "the build of " + commit + " ended as " + strings.ReplaceAll(conclusion, "_", " ")
	}
}

// makeList is the channel list from every channel's entry. Each entry is
// read with the same rules as the app reads the list, and checked against
// the public key, so the list never promises a build no app will take.
// newest holds each channel's commit still to be built, and conflict the
// pull requests that no longer merge into main: the workflow knows both
// when it writes the list, and the entry, written when the build was
// made, cannot. It says what it left out, and why, in log.
func makeList(entries map[string][]byte, newest map[string]string, conflict map[string]bool, public ed25519.PublicKey, now time.Time, log io.Writer) (updates.List, error) {
	names := make([]string, 0, len(entries))
	for n := range entries {
		names = append(names, n)
	}
	sort.Strings(names)
	var l updates.List
	for _, name := range names {
		var b updates.Build
		if err := json.Unmarshal(entries[name], &b); err != nil {
			fmt.Fprintf(log, "left out %s: %v\n", name, err)
			continue
		}
		if err := b.Check(); err != nil {
			fmt.Fprintf(log, "left out %s: %v\n", name, err)
			continue
		}
		if err := updates.Verify(public, b); err != nil {
			fmt.Fprintf(log, "left out %s: %v\n", name, err)
			continue
		}
		// An entry from before claims were made is kept, for the apps
		// that do not check them yet, and the apps that do leave it alone.
		// One whose claim is there and wrong is no build of ours.
		if b.Claim != "" {
			if err := updates.VerifyClaim(public, b); err != nil {
				fmt.Fprintf(log, "left out %s: %v\n", name, err)
				continue
			}
		}
		if n := newest[b.Channel]; n != "" {
			// Twelve characters, the way the app keeps a commit.
			b.Newest = n[:min(len(n), 12)]
		}
		b.Conflict = conflict[b.Channel]
		l.Channels = append(l.Channels, b)
	}
	// When it was written, so the app can tell the newer of the list's
	// two copies, see updates.ListCopies.
	l.Written = now.UTC().Truncate(time.Second)
	data, err := json.Marshal(l)
	if err != nil {
		return l, err
	}
	// The same reading the app does, so what is written is what it keeps,
	// and a newest commit that is not one is not written.
	return updates.Parse(data)
}

// writeChannels writes the channel list again from every channel's entry
// in the release dev, and uploads it. publish.yml runs it the moment a
// build starts, so the list says a newer commit is on its way for the
// whole of its build, and whenever a build ends, however it ended. It
// also takes away the entry of a pull request that is not open any more,
// and every zip no entry points at that is over an hour old.
func writeChannels(gh gitHub, t trees, public ed25519.PublicKey, now time.Time, log io.Writer) error {
	all, err := gh.assets()
	if err != nil {
		return err
	}
	entries := map[string][]byte{}
	for _, a := range all {
		if strings.HasPrefix(a.name, "channel-") && strings.HasSuffix(a.name, ".json") {
			data, err := gh.download(a)
			if err != nil {
				return err
			}
			entries[a.name] = data
		}
	}
	builtFrom := func(name string) string {
		var b updates.Build
		_ = json.Unmarshal(entries[name], &b)
		return b.Commit
	}
	newest := map[string]string{}
	conflict := map[string]bool{}
	// Only when the open pull requests could be read, or a failure to read
	// them would take every entry away.
	if open, err := gh.pulls(); err != nil {
		fmt.Fprintf(log, "the open pull requests cannot be read, so every entry stays: %v\n", err)
	} else {
		heads := map[string]pull{}
		for _, p := range open {
			heads[fmt.Sprintf("pr-%d", p.number)] = p
		}
		for _, a := range all {
			channel, ok := strings.CutPrefix(strings.TrimSuffix(a.name, ".json"), "channel-")
			if !ok || !strings.HasPrefix(channel, "pr-") || !strings.HasSuffix(a.name, ".json") {
				continue
			}
			p, open := heads[channel]
			// The entry of a pull request that is not open any more goes.
			// GitHub runs nothing for one closed while it conflicts with
			// main, so it kept its entry for good, and the app went on
			// offering it, #24.
			if !open {
				fmt.Fprintf(log, "taking away the entry of %s, which is not open\n", channel)
				if err := gh.remove(a); err != nil {
					return err
				}
				delete(entries, a.name)
				continue
			}
			// One that no longer merges into main is still listed, because
			// its build is of the branch as it is, and the app says it
			// conflicts.
			if c, err := gh.conflicts(p.number); err == nil && c {
				fmt.Fprintf(log, "%s no longer merges into main\n", channel)
				conflict[channel] = true
			}
			// A newer commit than the one built, with a build to come, by
			// the same rule the build goes by, unless its build already
			// ended without one.
			if answer, _ := needsBuild(t, builtFrom(a.name), p.head); answer == needed {
				if gone, why := ended(gh, p.head); gone {
					fmt.Fprintf(log, "%s: %s\n", channel, why)
				} else {
					fmt.Fprintf(log, "%s has %s still to be built\n", channel, p.head)
					newest[channel] = p.head
				}
			}
		}
	}
	// main's newest commit, when it is not the one built. Every push to
	// main is built, docs too, so any other commit is one still to come,
	// unless its build ended without one. Without this the page said main
	// was up to date for three days while every publish of main waited
	// behind one that never started.
	if _, ok := entries["channel-main.json"]; ok {
		if head, err := gh.mainCommit(); err == nil {
			if from := builtFrom("channel-main.json"); from == "" || !strings.HasPrefix(head, from) {
				if gone, why := ended(gh, head); gone {
					fmt.Fprintf(log, "main: %s\n", why)
				} else {
					fmt.Fprintf(log, "main has %s still to be built\n", head)
					newest["main"] = head
				}
			}
		}
	}
	l, err := makeList(entries, newest, conflict, public, now, log)
	if err != nil {
		return err
	}
	data, err := json.MarshalIndent(l, "", "  ")
	if err != nil {
		return err
	}
	data = append(data, '\n')
	fmt.Fprintf(log, "%s", data)
	if err := uploadList(gh, data, log); err != nil {
		return err
	}
	return removeUnreachable(gh, l, now, log)
}

// uploadList puts the list where the apps read it. GitHub cannot replace
// a release file in one step: the old one is deleted before the new one
// is up, and every app that checked then was answered 404. So the apps
// read two copies, channels-a.json and channels-b.json, and keep the one
// written last, see ListCopies in updates/updates.go. Only the older copy
// is replaced, so the newer one stays where it is the whole time. The
// workflow writes the list once at a time, and every writing takes half
// a minute, so the copy left alone is never one GitHub is still catching
// up on. channels.json is written too, for the builds made before the
// copies: under a name of its own and then renamed into place, so it is
// missing only between the delete and the rename.
func uploadList(gh gitHub, data []byte, log io.Writer) error {
	if err := gh.upload("channels.next.json", data); err != nil {
		return err
	}
	all, err := gh.assets()
	if err != nil {
		return err
	}
	var next asset
	copies := map[string]time.Time{}
	for _, a := range all {
		switch a.name {
		case "channels.next.json":
			next = a
		case "channels.json":
			if err := gh.remove(a); err != nil {
				return err
			}
		case "channels-a.json", "channels-b.json":
			copies[a.name] = a.updated
		}
	}
	if err := gh.rename(next, "channels.json"); err != nil {
		fmt.Fprintf(log, "renaming the list failed, uploading it in place instead: %v\n", err)
		if err := gh.upload("channels.json", data); err != nil {
			return err
		}
		_ = gh.remove(next)
	}
	older := "channels-a.json"
	if copies["channels-b.json"].Before(copies["channels-a.json"]) {
		older = "channels-b.json"
	}
	fmt.Fprintf(log, "replacing %s, the older copy of the list\n", older)
	return gh.upload(older, data)
}

// removeUnreachable takes away every zip no entry points at, the parts
// and the packed zip beside it, and every piece no remaining parts name. Not at once: a
// publish that runs beside this may have uploaded a zip, parts or pieces
// whose entry this list has not seen yet, so only a file over an hour old
// goes. A piece that goes all the same, because a publish found it there
// and did not upload it again, costs the app a whole download of that one
// build, never the update, and the next publish that needs it uploads it
// again.
func removeUnreachable(gh gitHub, l updates.List, now time.Time, log io.Writer) error {
	kept := map[string]bool{}
	for _, b := range l.Channels {
		kept[path.Base(b.URL)] = true
		kept[updates.PartsName(path.Base(b.URL))] = true
		kept[updates.PackedName(path.Base(b.URL))] = true
	}
	all, err := gh.assets()
	if err != nil {
		return err
	}
	// The pieces every kept parts file names. A parts file that cannot be
	// read keeps every piece, since nothing then says which may go.
	pieces, known := map[string]bool{}, true
	for _, a := range all {
		if !kept[a.name] || !strings.HasSuffix(a.name, ".parts.json") {
			continue
		}
		data, err := gh.download(a)
		var p updates.Parts
		if err == nil {
			err = json.Unmarshal(data, &p)
		}
		if err != nil {
			fmt.Fprintf(log, "reading %s failed, so every piece stays: %v\n", a.name, err)
			known = false
			break
		}
		for _, part := range p.Parts {
			if part.Bytes == nil {
				pieces[updates.PieceName(part.SHA256)] = true
			}
		}
	}
	for _, a := range all {
		build := strings.HasPrefix(a.name, "app-") &&
			(strings.HasSuffix(a.name, ".zip") || strings.HasSuffix(a.name, ".zip.zst") || strings.HasSuffix(a.name, ".parts.json"))
		piece := known && strings.HasPrefix(a.name, "piece-") && strings.HasSuffix(a.name, ".zst")
		if !(build && !kept[a.name]) && !(piece && !pieces[a.name]) {
			continue
		}
		if now.Sub(a.created) > time.Hour {
			fmt.Fprintf(log, "removing %s\n", a.name)
			if err := gh.remove(a); err != nil {
				fmt.Fprintf(log, "removing %s failed: %v\n", a.name, err)
			}
		}
	}
	return nil
}

// unstick cancels any publish that has waited to enter the environment
// for half an hour. updates has no reviewers and no wait timer, so a
// publish waits there for seconds, but on 7 October one of main waited
// for good. publish never stops half way, so it holds its channel's queue
// while it waits, and every publish of main after it waited behind it and
// was replaced by the next: main stayed on that day's build for three days
// while the page said it was up to date. A wait is not a running job, so
// timeout-minutes does not end it.
func unstick(gh gitHub, now time.Time, log io.Writer) error {
	runs, err := gh.waiting()
	if err != nil {
		return err
	}
	for _, r := range runs {
		if now.Sub(r.created) > 30*time.Minute {
			fmt.Fprintf(log, "::warning::Publish run %s has waited for the environment since %s, so it is cancelled.\n",
				strconv.FormatInt(r.id, 10), r.created.Format(time.RFC3339))
			if err := gh.cancel(r.id); err != nil {
				fmt.Fprintf(log, "cancelling %d failed: %v\n", r.id, err)
			}
		}
	}
	return nil
}
