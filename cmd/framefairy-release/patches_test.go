package main

import (
	"bytes"
	"encoding/base64"
	"encoding/hex"
	"fmt"
	"io"
	"math/rand/v2"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"slices"
	"strings"
	"testing"
	"time"

	"framefairy/updates"
)

// programOf is the app's program with code of its own after it, as hard to
// pack as real code, so a piece is large and a patch between two builds is
// small.
func programOf(code []byte) []byte { return append(app.bytes(), code...) }

// code is what a build compiles to, as random as compiled code.
var code = func() []byte {
	b := make([]byte, 96<<10)
	_, _ = rand.NewChaCha8([32]byte{1}).Read(b)
	return b
}()

// changed is the code after a change: a few bytes in, and every byte after
// them moved along.
var changed = append(append(append([]byte{}, code[:5000]...), "a new line of code"...), code[5000:]...)

// published is the folder pack wrote, under the names publish.yml uploads
// it as, for a zip of this name.
//
// On the real disk: pack leaves the build as files, and the workflow uploads them.
func published(t *testing.T, dir, zipName string) map[string][]byte {
	t.Helper()
	files := map[string][]byte{}
	entries, err := os.ReadDir(dir)
	if err != nil {
		t.Fatal(err)
	}
	for _, e := range entries {
		data, err := os.ReadFile(filepath.Join(dir, e.Name()))
		if err != nil {
			t.Fatal(err)
		}
		switch name := e.Name(); {
		case name == partsFile:
			files[updates.PartsName(zipName)] = data
		case strings.HasPrefix(name, "piece-"), strings.HasPrefix(name, "patch-"):
			files[name] = data
		}
	}
	return files
}

// releaseServing serves files as the release dev does.
func releaseServing(t *testing.T, files map[string][]byte) *httptest.Server {
	t.Helper()
	srv := httptest.NewTLSServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		data, ok := files[strings.TrimPrefix(r.URL.Path, "/dev/")]
		if !ok {
			http.NotFound(w, r)
			return
		}
		_, _ = w.Write(data)
	}))
	t.Cleanup(srv.Close)
	return srv
}

// basesFrom is pack's bases, read from the release srv serves.
func basesFrom(srv *httptest.Server, names []string, log io.Writer) bases {
	return readBases(names, srv.URL+"/dev/", srv.Client(), patchesPerBuild, log)
}

// A build is packed with a patch of each file that changed against the
// same file of the builds the release holds, which makes the file the
// app's own code makes from it, weighs less than the piece, and is named
// in the parts by the file it starts from. A file that did not change has
// none, and the build signs with them.
//
// On the real disk: the release tool reads the zip, and writes the packed build, as files.
func TestABuildIsPackedWithPatchesAgainstTheBuildsListed(t *testing.T) {
	release(t)
	app := appFile
	files := theApp("main")
	files[app] = programOf(code)
	first := packed(t, files)
	srv := releaseServing(t, published(t, first, "app-main-1.zip"))

	files[app] = programOf(changed)
	dir := t.TempDir()
	in := filepath.Join(dir, "build.zip")
	if err := os.WriteFile(in, zipped(t, files), 0o644); err != nil {
		t.Fatal(err)
	}
	second := filepath.Join(dir, "packed")
	var log bytes.Buffer
	b := basesFrom(srv, []string{"app-main-1.parts.json"}, &log)
	if err := pack(in, second, packer, b, &log); err != nil {
		t.Fatal(err)
	}
	p := partsIn(t, second)
	for _, part := range p.Parts {
		switch {
		case part.Bytes != nil:
		case part.Name == app:
			old := programOf(code)
			if len(part.Patches) != 1 || part.Patches[0].From != sum(old) {
				t.Fatalf("the program has the patches %+v, want one from the program before", part.Patches)
			}
			pt := part.Patches[0]
			if pt.Size >= part.Packed {
				t.Errorf("the patch is %d bytes, the piece %d", pt.Size, part.Packed)
			}
			patch, err := os.ReadFile(filepath.Join(second, updates.PatchName(pt.From, part.SHA256)))
			if err != nil || int64(len(patch)) != pt.Size {
				t.Fatalf("the patch is %d bytes, %v", len(patch), err)
			}
			made, err := updates.ApplyPatch(old, patch, part.Size)
			if err != nil || !bytes.Equal(made, programOf(changed)) {
				t.Errorf("the patch does not make the program: %v", err)
			}
		default:
			if part.Patches != nil {
				t.Errorf("%s did not change and has the patches %+v", part.Name, part.Patches)
			}
		}
	}
	entry := filepath.Join(t.TempDir(), "entry.json")
	if err := signBuild(in, "main", "main", "0.1.0-main.def5678", "def5678abc12",
		srv.URL+"/dev/app-main-2.zip", filepath.Join(second, partsFile), entry, time.Time{}); err != nil {
		t.Fatalf("a build with patches was not signed: %v\n%s", err, log.String())
	}
	// The patch goes up before the entry, so an app that sees the entry
	// finds it there.
	gh := &fakeGitHub{}
	if err := publish(gh, in, "app-main-2.zip", second, entry, "main", "def5678abc12"); err != nil {
		t.Fatal(err)
	}
	patch := slices.IndexFunc(gh.did, func(d string) bool { return strings.HasPrefix(d, "upload patch-") })
	if patch < 0 || patch > slices.Index(gh.did, "upload channel-main.json") {
		t.Errorf("the publish uploaded\n  %s\nwant the patch before the entry", strings.Join(gh.did, "\n  "))
	}
}

// What cannot be had of a build patched against, its pieces, or its parts
// when they are gone, no parts or say another build, costs only its
// patches, never the publish, and the run is warned.
//
// On the real disk: the release tool reads the zip, and writes the packed build, as files.
func TestWhatCannotBeHadToPatchAgainstCostsOnlyThePatches(t *testing.T) {
	release(t)
	app := appFile
	files := theApp("main")
	files[app] = programOf(code)
	first := packed(t, files)
	// The parts are there, the pieces they name are gone.
	served := map[string][]byte{}
	for name, data := range published(t, first, "app-main-1.zip") {
		if strings.HasSuffix(name, ".parts.json") {
			served[name] = data
		}
	}
	served["app-pr-2.parts.json"] = []byte("{")
	p := partsIn(t, first)
	p.Size++
	served["app-pr-3.parts.json"] = mustMarshal(t, p)
	srv := releaseServing(t, served)

	files[app] = programOf(changed)
	dir := t.TempDir()
	in := filepath.Join(dir, "build.zip")
	if err := os.WriteFile(in, zipped(t, files), 0o644); err != nil {
		t.Fatal(err)
	}
	out := filepath.Join(dir, "packed")
	var log bytes.Buffer
	b := basesFrom(srv, []string{"app-main-1.parts.json", "app-pr-2.parts.json", "app-pr-3.parts.json", "app-pr-4.parts.json"}, &log)
	if len(b.parts) != 1 {
		t.Fatalf("%d parts were read, want the one that is parts\n%s", len(b.parts), log.String())
	}
	for _, name := range []string{"app-pr-2", "app-pr-3", "app-pr-4"} {
		if !strings.Contains(log.String(), "::warning::"+name+".parts.json could not be read") {
			t.Errorf("the run is not warned that %s.parts.json could not be read:\n%s", name, log.String())
		}
	}
	if err := pack(in, out, packer, b, &log); err != nil {
		t.Fatal(err)
	}
	for _, part := range partsIn(t, out).Parts {
		if part.Patches != nil {
			t.Errorf("%s has the patches %+v from builds that cannot be had", part.Name, part.Patches)
		}
	}
	if !strings.Contains(log.String(), "no patch of "+app) {
		t.Errorf("the log does not say why the program has no patch:\n%s", log.String())
	}
}

// builtBefore publishes n builds of the app, each with a program of its
// own, as app-pr-1 to app-pr-n, and returns what the release serves and
// the program of each, in that order.
//
// On the real disk: pack writes each build as files, and the release serves them.
func builtBefore(t *testing.T, n int) (map[string][]byte, [][]byte) {
	t.Helper()
	app := appFile
	served := map[string][]byte{}
	var olds [][]byte
	for i := range n {
		files := theApp("main")
		old := programOf(append(append([]byte{}, code...), byte(i)))
		files[app] = old
		olds = append(olds, old)
		for name, data := range published(t, packed(t, files), fmt.Sprintf("app-pr-%d.zip", i+1)) {
			served[name] = data
		}
	}
	return served, olds
}

// patchesFrom are the files the patches in out start from, sorted.
func patchesFrom(t *testing.T, out string) []string {
	t.Helper()
	var got []string
	for _, part := range partsIn(t, out).Parts {
		for _, pt := range part.Patches {
			got = append(got, pt.From)
		}
	}
	slices.Sort(got)
	return got
}

// sums are the SHA-256 of each program, sorted.
func sums(programs ...[]byte) []string {
	var out []string
	for _, p := range programs {
		out = append(out, sum(p))
	}
	slices.Sort(out)
	return out
}

// A build has at most the patches it has room for, and those are from
// the builds patched against that come first. The release holds at most
// a thousand files, and patches from every build to every build filled
// it.
//
// On the real disk: the release tool reads the zip, and writes the packed build, as files.
func TestABuildHasThePatchesFromTheBuildsFirstThatItHasRoomFor(t *testing.T) {
	release(t)
	const n = 8
	served, olds := builtBefore(t, n)
	srv := releaseServing(t, served)
	// The order they are patched against in, not the order they were made.
	order := []int{5, 2, 7, 0, 3, 6, 1, 4}
	var names []string
	var first [][]byte
	for _, i := range order {
		names = append(names, fmt.Sprintf("app-pr-%d.parts.json", i+1))
		first = append(first, olds[i])
	}

	files := theApp("main")
	files[appFile] = programOf(changed)
	dir := t.TempDir()
	in := filepath.Join(dir, "build.zip")
	if err := os.WriteFile(in, zipped(t, files), 0o644); err != nil {
		t.Fatal(err)
	}
	for _, most := range []int{0, 1, 3, patchesPerBuild} {
		want := min(most, n)
		out := filepath.Join(dir, fmt.Sprint("packed-", most))
		var log bytes.Buffer
		b := basesFrom(srv, names, &log)
		b.most = most
		if err := pack(in, out, packer, b, &log); err != nil {
			t.Fatal(err)
		}
		if got := patchesFrom(t, out); !slices.Equal(got, sums(first[:want]...)) {
			t.Errorf("with room for %d, the patches are from %.12v, want from the first %d", most, got, want)
		}
		patches, _ := filepath.Glob(filepath.Join(out, "patch-*"))
		if len(patches) != want {
			t.Errorf("with room for %d, %d patches were written, want %d", most, len(patches), want)
		}
	}
}

// publish-plan reads the channel list and the release's files, gives a
// build room for fewer patches once the release is nearly full, and
// names the channel's own build first, though it is the oldest: the app
// that follows the channel is the one that updates. pack, run with what
// the plan said, patches against that build.
//
// On the real disk: the release tool reads the zip and writes the packed build as files.
func TestPackPatchesFromTheChannelsBuildFirstOnANearlyFullRelease(t *testing.T) {
	release(t)
	served, olds := builtBefore(t, 3)
	srv := releaseServing(t, served)

	gh := &fakeGitHub{}
	l := updates.List{}
	at := now.Add(-3 * time.Hour)
	for i := range 3 {
		zip := fmt.Sprintf("app-pr-%d.zip", i+1)
		l.Channels = append(l.Channels, updates.Build{Channel: fmt.Sprintf("pr-%d", i+1), Name: zip,
			Version: fmt.Sprintf("0.3.0-pr%d.aaaaaaa", i+1), Commit: "aaaaaaaaaaaa", URL: updates.EntriesURL + zip,
			Size: 1, SHA256: strings.Repeat("a", 64), Signature: base64.StdEncoding.EncodeToString(make([]byte, 64))})
		gh.put(zip, nil, at.Add(time.Duration(i)*time.Hour))
		gh.put(updates.PartsName(zip), served[updates.PartsName(zip)], at.Add(time.Duration(i)*time.Hour))
	}
	gh.put("channels.json", mustMarshal(t, l), now)
	// 799 files leave room for a single patch.
	for i := range maxAssets - keepFree - 1 - 7 {
		gh.put(fmt.Sprintf("piece-%d.zst", i), nil, at)
	}
	var p plan
	planBases(gh, &p, "pr-1")
	got := outputs(p)
	if got["bases"] != "app-pr-1.parts.json,app-pr-3.parts.json,app-pr-2.parts.json" || got["room"] != "1" {
		t.Fatalf("the plan says bases %q and room %q, want pr-1's first, then the newest, and room for 1", got["bases"], got["room"])
	}

	dir := t.TempDir()
	files := theApp("main")
	files[appFile] = programOf(changed)
	in := filepath.Join(dir, "build.zip")
	if err := os.WriteFile(in, zipped(t, files), 0o644); err != nil {
		t.Fatal(err)
	}
	out := filepath.Join(dir, "packed")
	var log bytes.Buffer
	b := readBases(strings.Split(got["bases"], ","), srv.URL+"/dev/", srv.Client(), 1, &log)
	if err := pack(in, out, packer, b, &log); err != nil {
		t.Fatal(err)
	}
	if got, want := patchesFrom(t, out), sums(olds[0]); !slices.Equal(got, want) {
		t.Errorf("the patches are from %.12v, want only from pr-1's build, %.12v", got, want)
	}
}

// A release that cannot be read gives the build no patches and says why,
// and the build is published all the same.
func TestAReleaseThatCannotBeReadGivesNoPatches(t *testing.T) {
	t.Parallel()
	gh := &fakeGitHub{down: true}
	var p plan
	planBases(gh, &p, "main")
	if got := outputs(p); got["bases"] != "" || got["room"] != "0" || !strings.Contains(p.notice, "no patches") {
		t.Errorf("a release that cannot be read planned %v, %q", got, p.notice)
	}
}

// The builds patched against are the listed ones and the newest three
// that left the list, never a file that is no parts of a build.
func TestTheBasesAreTheListedBuildsAndTheNewestThatLeft(t *testing.T) {
	t.Parallel()
	var all []asset
	add := func(name string, hoursAgo int) {
		all = append(all, asset{name: name, created: now.Add(-time.Duration(hoursAgo) * time.Hour)})
	}
	add("app-main-9.parts.json", 1)
	add("app-pr-5-3.parts.json", 2)
	for i := range 5 {
		add(fmt.Sprintf("app-pr-%d-1.parts.json", i+1), 10+i)
	}
	add("app-pr-6-1.zip", 0)
	add("piece-aaa.zst", 0)
	l := updates.List{Channels: []updates.Build{
		{Channel: "main", URL: updates.EntriesURL + "app-main-9.zip"},
		{Channel: "pr-5", URL: updates.EntriesURL + "app-pr-5-3.zip"},
		// Listed, but the release no longer holds its parts.
		{Channel: "pr-7", URL: updates.EntriesURL + "app-pr-7-2.zip"},
	}}
	got := basesOf(l, all, "pr-5", "main")
	want := []string{"app-pr-5-3.parts.json", "app-main-9.parts.json", "app-pr-1-1.parts.json", "app-pr-2-1.parts.json", "app-pr-3-1.parts.json"}
	if !slices.Equal(got, want) {
		t.Errorf("the bases are\n  %s\nwant\n  %s", strings.Join(got, "\n  "), strings.Join(want, "\n  "))
	}
}

func sum(b []byte) string {
	d, _, _ := updates.Digest(bytes.NewReader(b))
	return hex.EncodeToString(d)
}
