package main

import (
	"bytes"
	"cmp"
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net/http"
	"os"
	"path"
	"path/filepath"
	"runtime"
	"slices"
	"strconv"
	"strings"
	"sync"
	"time"

	"github.com/gabstv/go-bsdiff/pkg/bsdiff"

	"framefairy/updates"
)

// A file that changed between two builds changes by a few bytes: a change
// to the code moves every address after it and leaves the rest. So pack
// also writes a patch of each file against the same file of the builds an
// app is likely to run, bsdiff's, and the parts name it by the file it
// starts from. The app downloads the patch when it runs one of those
// builds, see updates/patch.go and
// docs/decisions/2026-11-28-an-update-downloads-the-bytes-that-changed.md.
//
// Which builds are patched against is worked out by publish-plan, which
// may read the release's list of files, and handed to pack as names. pack
// reads their parts and the pieces those name from the release's public
// addresses, as an app does, in a step that holds no token. Whatever of
// them cannot be had costs only its patches, never the publish.

// leftBases is how many builds that left the channel list a new build is
// patched against. A build leaves the list when its pull request is
// merged or a newer commit replaces it, and the app that runs it is then
// the one most likely to update next, to main after a merge above all.
// Each costs the publish a download and a bsdiff of a few seconds for each
// file that changed. The user runs one app, so the last few cover it.
const leftBases = 3

// maxAssets is how many files GitHub lets one release hold. An upload
// past it is refused, and Speaker Trail's publishes all failed for most
// of an hour on 27 November 2026 when its release reached it.
const maxAssets = 1000

// keepFree is how many files of the release a build's patches leave free:
// for its own pieces, zips and parts, and for the publishes of other
// channels that run beside it and counted the same files.
const keepFree = 200

// patchesPerBuild is how many patches one build has at most. Every build
// gets patches to every listed build, so without a bound the patches grow
// with the square of the open pull requests.
const patchesPerBuild = 60

// patchTakes is the longest reading one file of the release may take.
const patchTakes = 5 * time.Minute

// maxPatched is the largest file a patch is made of. bsdiff sorts every
// suffix of the file it starts from, in about 17 bytes for each of its
// bytes, and pack makes a patch on every core at once. The app program
// is about 20 MB, and the larger libraries keep their bytes from build
// to build anyway, see kept.go.
const maxPatched = 64 << 20

// basesOf names the parts files of the builds a new build is patched
// against: every build the list names, and the newest leftBases that left
// it, among the files the release holds, each once. The listed builds of
// the channels first come first, in that order, and the rest the newest
// first. A new build has patches from the first ones when it may not have
// all: an app that runs the channel's build before is the one most likely
// to update, then one that runs main, then one that runs a build made
// lately.
func basesOf(l updates.List, all []asset, first ...string) []string {
	holds := map[string]time.Time{}
	for _, a := range all {
		holds[a.name] = a.created
	}
	var out []string
	add := func(name string) {
		if _, ok := holds[name]; ok && !slices.Contains(out, name) {
			out = append(out, name)
		}
	}
	for _, b := range l.Channels {
		add(updates.PartsName(path.Base(b.URL)))
	}
	listed := len(out)
	newest := slices.SortedFunc(slices.Values(all), func(a, b asset) int { return b.created.Compare(a.created) })
	for _, a := range newest {
		if len(out)-listed == leftBases {
			break
		}
		if strings.HasPrefix(a.name, "app-") && strings.HasSuffix(a.name, ".parts.json") {
			add(a.name)
		}
	}
	slices.SortStableFunc(out, func(a, b string) int { return holds[b].Compare(holds[a]) })
	var front []string
	for _, channel := range first {
		for _, b := range l.Channels {
			name := updates.PartsName(path.Base(b.URL))
			if b.Channel == channel && slices.Contains(out, name) && !slices.Contains(front, name) {
				front = append(front, name)
			}
		}
	}
	return append(front, slices.DeleteFunc(out, func(name string) bool { return slices.Contains(front, name) })...)
}

// patchRoom is how many patches a new build may have when the release
// holds n files: patchesPerBuild, fewer once it holds more than
// maxAssets less keepFree less patchesPerBuild, and none from maxAssets
// less keepFree.
func patchRoom(n int) int {
	return max(0, min(patchesPerBuild, maxAssets-keepFree-n))
}

// planBases sets what pack patches against for a build of channel, in p:
// bases, the parts files by name, and room, how many patches it may have.
// A release that cannot be read gives no patches and says why, and the
// build is published all the same.
func planBases(gh gitHub, p *plan, channel string) {
	none := func(why error) {
		p.set("bases", "")
		p.set("room", "0")
		p.notice = strings.TrimSpace(p.notice + " This build has no patches: " + why.Error())
	}
	all, err := gh.assets()
	if err != nil {
		none(err)
		return
	}
	i := slices.IndexFunc(all, func(a asset) bool { return a.name == "channels.json" })
	if i < 0 {
		p.set("bases", "")
		p.set("room", "0")
		return
	}
	data, err := gh.download(all[i])
	if err != nil {
		none(err)
		return
	}
	l, err := updates.Parse(data)
	if err != nil {
		none(err)
		return
	}
	p.set("bases", strings.Join(basesOf(l, all, channel, "main"), ","))
	p.set("room", strconv.Itoa(patchRoom(len(all))))
}

// bases is what pack patches against: the parts of builds the release
// holds, the first ones first, the folder of the release their pieces
// are read from, and how many patches the build may have at most. The
// patches from the first parts are the ones made when not all may be.
type bases struct {
	parts  []updates.Parts
	names  []string
	from   string
	client *http.Client
	most   int
}

// readBases reads the parts files named, from the release's folder from,
// by the app's own rules. One that cannot be read is left out and said,
// as a warning on the workflow's run. A name that is no parts file of a
// build is never read.
func readBases(names []string, from string, client *http.Client, most int, log io.Writer) bases {
	b := bases{from: from, client: client, most: most}
	for _, name := range names {
		if name == "" {
			continue
		}
		if !strings.HasPrefix(name, "app-") || !strings.HasSuffix(name, ".parts.json") || strings.ContainsAny(name, "/\\?#%") {
			fmt.Fprintf(log, "::warning::%q is no parts file of a build, so it is not patched against\n", name)
			continue
		}
		data, err := b.get(name, updates.MaxList)
		var p updates.Parts
		if err == nil {
			err = json.Unmarshal(data, &p)
		}
		if err == nil {
			p, err = updates.ParseParts(data, p.Size, p.SHA256)
		}
		if err != nil {
			fmt.Fprintf(log, "::warning::%s could not be read, so it is not patched against: %v\n", name, err)
			continue
		}
		b.parts = append(b.parts, p)
		b.names = append(b.names, strings.TrimSuffix(name, ".parts.json"))
	}
	return b
}

// addPatches writes the patches of the files of p, which lie in stored,
// against the same files of the bases, to out, where they weigh less than
// the file's piece, and names them in p.
func addPatches(p *updates.Parts, stored io.ReaderAt, b bases, out string, log io.Writer) {
	// The files of every build patched against, by where they are in the
	// app, each once, and the first build that has each.
	olds := map[string]map[string]updates.Part{}
	rank := map[string]int{}
	for i, bp := range b.parts {
		for _, part := range bp.Parts {
			inside, ok := updates.InApp(part.Name)
			if part.Bytes != nil || !ok {
				continue
			}
			if olds[inside] == nil {
				olds[inside] = map[string]updates.Part{}
			}
			olds[inside][part.SHA256] = part
			if _, ok := rank[part.SHA256]; !ok {
				rank[part.SHA256] = i
			}
		}
	}
	type job struct {
		part  int
		at    int64
		old   updates.Part
		patch updates.Patch
		ok    bool
	}
	var jobs []*job
	at := int64(0)
	for i, part := range p.Parts {
		if part.Bytes != nil {
			at += int64(len(part.Bytes))
			continue
		}
		inside, _ := updates.InApp(part.Name)
		for sum, old := range olds[inside] {
			if sum != part.SHA256 && part.Size <= maxPatched && old.Size <= maxPatched {
				jobs = append(jobs, &job{part: i, at: at, old: old})
			}
		}
		at += part.Size
	}
	// At most b.most, the patches from the builds patched against first,
	// and of those the largest files first, whose patches save the most.
	slices.SortStableFunc(jobs, func(a, c *job) int {
		return cmp.Or(cmp.Compare(rank[a.old.SHA256], rank[c.old.SHA256]),
			cmp.Compare(p.Parts[c.part].Size, p.Parts[a.part].Size),
			cmp.Compare(a.part, c.part), cmp.Compare(a.old.SHA256, c.old.SHA256))
	})
	if len(jobs) > b.most {
		fmt.Fprintf(log, "%d patches could be made, and %d are, from the builds patched against first\n", len(jobs), max(b.most, 0))
		jobs = jobs[:max(b.most, 0)]
	}
	work := make(chan *job)
	var wg sync.WaitGroup
	for range runtime.NumCPU() {
		wg.Go(func() {
			for j := range work {
				part := p.Parts[j.part]
				patch, err := b.patch(stored, j.at, part, j.old)
				if err == nil && int64(len(patch)) >= part.Packed {
					err = fmt.Errorf("it is %d bytes, the piece %d", len(patch), part.Packed)
				}
				if err == nil {
					err = os.WriteFile(filepath.Join(out, updates.PatchName(j.old.SHA256, part.SHA256)), patch, 0o644)
				}
				if err != nil {
					fmt.Fprintf(log, "no patch of %s from %.12s: %v\n", part.Name, j.old.SHA256, err)
					continue
				}
				j.patch, j.ok = updates.Patch{From: j.old.SHA256, Size: int64(len(patch))}, true
			}
		})
	}
	// The largest first, so the last to finish is a small one.
	for _, j := range slices.SortedFunc(slices.Values(jobs), func(a, c *job) int {
		return cmp.Compare(p.Parts[c.part].Size, p.Parts[a.part].Size)
	}) {
		work <- j
	}
	close(work)
	wg.Wait()
	for _, j := range jobs {
		if j.ok {
			p.Parts[j.part].Patches = append(p.Parts[j.part].Patches, j.patch)
		}
	}
	for i := range p.Parts {
		slices.SortFunc(p.Parts[i].Patches, func(a, c updates.Patch) int { return cmp.Compare(a.From, c.From) })
		if len(p.Parts[i].Patches) > updates.MaxPatches {
			p.Parts[i].Patches = p.Parts[i].Patches[:updates.MaxPatches]
		}
	}
}

// patch is the patch from the file old names to the file part names,
// which lies at at in stored, once it is known to make that file.
func (b bases) patch(stored io.ReaderAt, at int64, part, old updates.Part) ([]byte, error) {
	packed, err := b.get(updates.PieceName(old.SHA256), old.Packed)
	if err != nil {
		return nil, err
	}
	from, err := updates.UnpackPiece(packed, old)
	if err != nil {
		return nil, err
	}
	to := make([]byte, part.Size)
	if _, err := stored.ReadAt(to, at); err != nil {
		return nil, err
	}
	patch, err := bsdiff.Bytes(from, to)
	if err != nil {
		return nil, err
	}
	// The app reads a patch with code of its own, so what is published is
	// what that code makes the file from.
	made, err := updates.ApplyPatch(from, patch, part.Size)
	if err != nil {
		return nil, fmt.Errorf("the app cannot apply it: %w", err)
	}
	if !bytes.Equal(made, to) {
		return nil, errors.New("the app would make another file from it")
	}
	return patch, nil
}

// get reads a file of the release, at most limit bytes.
func (b bases) get(name string, limit int64) ([]byte, error) {
	ctx, cancel := context.WithTimeout(context.Background(), patchTakes)
	defer cancel()
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, b.from+name, nil)
	if err != nil {
		return nil, err
	}
	client := b.client
	if client == nil {
		client = http.DefaultClient
	}
	resp, err := client.Do(req)
	if err != nil {
		return nil, err
	}
	defer resp.Body.Close()
	if resp.StatusCode != http.StatusOK {
		return nil, fmt.Errorf("%s answered %s", name, resp.Status)
	}
	data, err := io.ReadAll(io.LimitReader(resp.Body, limit+1))
	if err != nil {
		return nil, err
	}
	if int64(len(data)) > limit {
		return nil, fmt.Errorf("%s is larger than it can be", name)
	}
	return data, nil
}

// logPatches says, for each build patched against, what an app that runs
// it downloads to update to p: the patch of a file it can have one for,
// the piece of any other file it does not have, and nothing for a file it
// has.
func logPatches(p updates.Parts, b bases, log io.Writer) {
	fmt.Fprintf(log, "%-40s %12s %12s\n", "an app that runs", "pieces", "patched")
	for i, bp := range b.parts {
		has := map[string]bool{}
		for _, part := range bp.Parts {
			has[part.SHA256] = true
		}
		var pieces, patched int64
		for _, part := range p.Parts {
			if part.Bytes != nil || has[part.SHA256] {
				continue
			}
			pieces += part.Packed
			cost := part.Packed
			for _, patch := range part.Patches {
				if has[patch.From] {
					cost = min(cost, patch.Size)
				}
			}
			patched += cost
		}
		fmt.Fprintf(log, "%-40s %12d %12d\n", b.names[i], pieces, patched)
	}
}
