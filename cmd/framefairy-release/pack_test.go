package main

import (
	"crypto/ed25519"
	"crypto/rand"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"errors"
	"io"
	"os"
	"path/filepath"
	"slices"
	"strings"
	"testing"
	"time"

	"framefairy/updates"
)

// packer packs the builds of every test: an encoder at zstd's best level
// takes tens of megabytes to set up.
var packer = updates.NewPacker()

// packed packs an app the way publish.yml does and returns the folder.
// The zip the build left is beside it, see built.
//
// On the real disk: the workflow hands the release tool the build as a file, and it leaves files for the steps after it.
func packed(t *testing.T, files map[string][]byte) string {
	t.Helper()
	dir := t.TempDir()
	in := filepath.Join(dir, "build.zip")
	if err := os.WriteFile(in, zipped(t, files), 0o644); err != nil {
		t.Fatal(err)
	}
	out := filepath.Join(dir, "packed")
	if err := pack(in, out, packer, io.Discard); err != nil {
		t.Fatal(err)
	}
	return out
}

// built is the zip the build left, which pack read to write packed.
func built(packed string) string { return filepath.Join(filepath.Dir(packed), "build.zip") }

// partsIn reads what pack wrote of a folder's parts.
//
// On the real disk: pack leaves the parts as a file.
func partsIn(t *testing.T, dir string) updates.Parts {
	t.Helper()
	data, err := os.ReadFile(filepath.Join(dir, partsFile))
	if err != nil {
		t.Fatal(err)
	}
	var p updates.Parts
	if err := json.Unmarshal(data, &p); err != nil {
		t.Fatal(err)
	}
	return p
}

func pieceOf(t *testing.T, p updates.Parts, name string) string {
	t.Helper()
	for _, part := range p.Parts {
		if part.Name == name {
			return updates.PieceName(part.SHA256)
		}
	}
	t.Fatalf("no part of %s", name)
	return ""
}

const (
	appFile    = "Frame Fairy.app/Contents/MacOS/framefairy-app"
	ffmpegFile = "Frame Fairy.app/Contents/MacOS/ffmpeg"
)

// The first build publishes every piece, the next one only the piece of
// the file that changed, and each goes up before the zip, the packed zip,
// the parts and the entry that name it.
//
// On the real disk: the release tool reads the packed build from files.
func TestAPublishUploadsOnlyThePiecesTheReleaseLacks(t *testing.T) {
	t.Parallel()
	ffmpeg := []byte(strings.Repeat("libavcodec ", 1000))
	first := packed(t, map[string][]byte{appFile: []byte("build 1"), ffmpegFile: ffmpeg})
	second := packed(t, map[string][]byte{appFile: []byte("build 2"), ffmpegFile: ffmpeg})
	entry := filepath.Join(t.TempDir(), "entry.json")
	if err := os.WriteFile(entry, []byte("{}"), 0o644); err != nil {
		t.Fatal(err)
	}
	gh := &fakeGitHub{}

	if err := publish(gh, built(first), "app-main-1.zip", first, entry, "main", "c1"); err != nil {
		t.Fatal(err)
	}
	p1 := partsIn(t, first)
	pieces := []string{pieceOf(t, p1, appFile), pieceOf(t, p1, ffmpegFile)}
	slices.Sort(pieces)
	var up []string
	for _, d := range gh.did {
		up = append(up, strings.TrimPrefix(d, "upload "))
	}
	slices.Sort(up[:2])
	want := append(pieces, "app-main-1.zip", "app-main-1.zip.zst", "app-main-1.parts.json", "channel-main.json")
	if !slices.Equal(up, want) {
		t.Errorf("the first build uploaded\n  %s\nwant\n  %s", strings.Join(up, "\n  "), strings.Join(want, "\n  "))
	}

	gh.did = nil
	if err := publish(gh, built(second), "app-main-2.zip", second, entry, "main", "c2"); err != nil {
		t.Fatal(err)
	}
	p2 := partsIn(t, second)
	want = []string{"upload " + pieceOf(t, p2, appFile), "upload app-main-2.zip", "upload app-main-2.zip.zst", "upload app-main-2.parts.json", "upload channel-main.json"}
	if !slices.Equal(gh.did, want) {
		t.Errorf("the second build uploaded\n  %s\nwant\n  %s", strings.Join(gh.did, "\n  "), strings.Join(want, "\n  "))
	}
	if string(gh.data["app-main-2.parts.json"]) == "" {
		t.Error("the parts went up empty")
	}
}

// What no entry reaches goes after an hour: the zip, packed zip and parts of a build
// that is no longer listed, and every piece no listed build's parts name.
// A piece still named, or uploaded within the hour, stays.
//
// On the real disk: the release tool reads the packed build from files.
func TestPiecesNoBuildNamesGo(t *testing.T) {
	t.Parallel()
	ffmpeg := []byte(strings.Repeat("libavcodec ", 1000))
	listed := packed(t, map[string][]byte{appFile: []byte("listed"), ffmpegFile: ffmpeg})
	gone := packed(t, map[string][]byte{appFile: []byte("gone"), ffmpegFile: ffmpeg})
	p, g := partsIn(t, listed), partsIn(t, gone)
	gh := &fakeGitHub{}
	old := now.Add(-2 * time.Hour)
	partsData, _ := os.ReadFile(filepath.Join(listed, partsFile))
	gh.put("app-main-1.zip", []byte("zip"), old)
	gh.put("app-main-1.parts.json", partsData, old)
	gh.put("app-main-1.zip.zst", []byte("zst"), old)
	gh.put("app-pr-9-1.zip", []byte("zip"), old)
	gh.put("app-pr-9-1.zip.zst", []byte("zst"), old)
	gh.put("app-pr-9-1.parts.json", []byte("{}"), old)
	gh.put(pieceOf(t, p, appFile), []byte("piece"), old)
	gh.put(pieceOf(t, p, ffmpegFile), []byte("piece"), old)
	gh.put(pieceOf(t, g, appFile), []byte("piece"), old)
	fresh := updates.PieceName(strings.Repeat("f", 64))
	gh.put(fresh, []byte("piece"), now.Add(-10*time.Minute))
	l := updates.List{Channels: []updates.Build{{Channel: "main", URL: "https://example.com/dev/app-main-1.zip"}}}

	if err := removeUnreachable(gh, l, now, io.Discard); err != nil {
		t.Fatal(err)
	}
	for _, name := range []string{"app-pr-9-1.zip", "app-pr-9-1.zip.zst", "app-pr-9-1.parts.json", pieceOf(t, g, appFile)} {
		if _, ok := gh.release[name]; ok {
			t.Errorf("%s, which no listed build reaches, is still there", name)
		}
	}
	for _, name := range []string{"app-main-1.zip", "app-main-1.zip.zst", "app-main-1.parts.json", pieceOf(t, p, appFile), pieceOf(t, p, ffmpegFile), fresh} {
		if _, ok := gh.release[name]; !ok {
			t.Errorf("%s was taken away", name)
		}
	}
}

// Parts that cannot be read keep every piece: nothing then says which a
// build still needs.
func TestUnreadablePartsKeepEveryPiece(t *testing.T) {
	t.Parallel()
	gh := &fakeGitHub{}
	old := now.Add(-2 * time.Hour)
	gh.put("app-main-1.zip", []byte("zip"), old)
	gh.put("app-main-1.parts.json", []byte("{"), old)
	piece := updates.PieceName(strings.Repeat("a", 64))
	gh.put(piece, []byte("piece"), old)
	l := updates.List{Channels: []updates.Build{{Channel: "main", URL: "https://example.com/dev/app-main-1.zip"}}}
	if err := removeUnreachable(gh, l, now, io.Discard); err != nil {
		t.Fatal(err)
	}
	if _, ok := gh.release[piece]; !ok {
		t.Error("a piece went while the parts could not be read")
	}
}

// A packed build is signed twice: as the zip the build left, which an app
// from before parts downloads, and as the stored zip, which an app that
// knows parts puts together or unpacks. Both pass the app's own check.
//
// On the real disk: the release tool reads the packed build and writes the entry as files.
func TestABuildIsSignedAsTheZipAndAsTheStoredZip(t *testing.T) {
	private := release(t)
	public := private.Public().(ed25519.PublicKey)
	dir := packed(t, theApp("main"))
	out := filepath.Join(t.TempDir(), "entry.json")
	err := sign([]string{"-zip", built(dir), "-parts", filepath.Join(dir, partsFile),
		"-channel", "main", "-name", "main", "-version", "0.1.0-main.abc1234", "-commit", "abc1234def56",
		"-url", "https://example.com/dev/app-main-1.zip", "-out", out})
	if err != nil {
		t.Fatal(err)
	}
	b := readBuild(t, out)
	sumOf := func(path string) string {
		data, err := os.ReadFile(path)
		if err != nil {
			t.Fatal(err)
		}
		s := sha256.Sum256(data)
		return hex.EncodeToString(s[:])
	}
	if b.SHA256 != sumOf(built(dir)) {
		t.Error("the entry's own checksum is not of the zip the build left, which the apps from before download")
	}
	whole, err := os.ReadFile(filepath.Join(dir, packedWhole))
	if err != nil {
		t.Fatal(err)
	}
	if b.Stored == nil || b.Stored.SHA256 != sumOf(filepath.Join(dir, storedZip)) || b.Stored.PackedSize != int64(len(whole)) {
		t.Fatalf("the entry says the stored zip is %+v", b.Stored)
	}
	if err := b.Check(); err != nil {
		t.Errorf("the app refuses the entry: %v", err)
	}
	st, _ := b.AsStored()
	if err := errors.Join(updates.Verify(public, st), updates.VerifyClaim(public, st)); err != nil {
		t.Errorf("the stored zip is not signed as the build: %v", err)
	}

	// Parts of another build are no parts of this one.
	other := packed(t, theApp("pr-7"))
	err = signBuild(built(dir), "main", "main", "0.1.0-main.abc1234", "abc1234def56",
		"https://example.com/dev/app-main-1.zip", filepath.Join(other, partsFile), out, time.Time{})
	if err == nil {
		t.Error("a build was signed with the parts of another")
	}

	// A packed zip of another build is no packed zip of this one.
	theirs, err := os.ReadFile(filepath.Join(other, packedWhole))
	if err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(dir, packedWhole), theirs, 0o644); err != nil {
		t.Fatal(err)
	}
	err = signBuild(built(dir), "main", "main", "0.1.0-main.abc1234", "abc1234def56",
		"https://example.com/dev/app-main-1.zip", filepath.Join(dir, partsFile), out, time.Time{})
	if err == nil {
		t.Error("a build was signed with the packed zip of another")
	}
}

// A build signed with the time its run was made carries that time as its
// order, signed over the stored zip the app installs, so the app can
// refuse a build of its channel made before one it took. A time that is
// not one is refused before anything is written.
//
// On the real disk: the release tool reads the packed build and writes the entry as files.
func TestABuildIsSignedWithWhenItWasMade(t *testing.T) {
	private := release(t)
	public := private.Public().(ed25519.PublicKey)
	dir := packed(t, theApp("main"))
	out := filepath.Join(t.TempDir(), "entry.json")
	args := func(made string) []string {
		return []string{"-zip", built(dir), "-parts", filepath.Join(dir, partsFile),
			"-channel", "main", "-name", "main", "-version", "0.1.0-main.abc1234", "-commit", "abc1234def56",
			"-url", "https://example.com/dev/app-main-1.zip", "-made", made, "-out", out}
	}
	if err := sign(args("2026-11-28T13:04:05Z")); err != nil {
		t.Fatal(err)
	}
	b := readBuild(t, out)
	if want := time.Date(2026, 11, 28, 13, 4, 5, 0, time.UTC); !b.Made.Equal(want) {
		t.Errorf("the entry says it was made %v, not %v", b.Made, want)
	}
	if err := updates.VerifyOrder(public, b); err != nil {
		t.Errorf("the app refuses the order: %v", err)
	}
	if err := sign(args("yesterday")); err == nil {
		t.Error("a build was signed with a made time that is no time")
	}
}

// The list leaves out an entry whose own zip is signed right but whose
// stored zip is not, since an app that knows parts installs the stored
// zip.
//
// On the real disk: the release tool reads the packed build and writes the entry as files.
func TestTheListLeavesOutABadStoredZip(t *testing.T) {
	private := release(t)
	public := private.Public().(ed25519.PublicKey)
	dir := packed(t, theApp("main"))
	out := filepath.Join(t.TempDir(), "entry.json")
	if err := signBuild(built(dir), "main", "main", "0.1.0-main.abc1234", "abc1234def56",
		"https://example.com/dev/app-main-1.zip", filepath.Join(dir, partsFile), out, time.Time{}); err != nil {
		t.Fatal(err)
	}
	good := readBuild(t, out)
	_, other, _ := ed25519.GenerateKey(rand.Reader)
	entries := map[string][]byte{"channel-main.json": mustMarshal(t, good)}
	for name, spoil := range map[string]func(b *updates.Build){
		// Signed with another key.
		"pr-1": func(b *updates.Build) {
			digest, _ := hex.DecodeString(b.Stored.SHA256)
			b.Stored.Signature = updates.Sign(other, digest)
		},
		// Claimed as another build: the stored claim of pr-2's own.
		"pr-2": func(b *updates.Build) {
			st, _ := b.AsStored()
			st.Version = "0.1.0-main.0000000"
			b.Stored.Claim = updates.SignClaim(private, st)
		},
	} {
		b := readBuild(t, out)
		b.Channel, b.Name = name, name
		b.Claim = updates.SignClaim(private, b)
		st, _ := b.AsStored()
		b.Stored.Claim = updates.SignClaim(private, st)
		spoil(&b)
		entries["channel-"+name+".json"] = mustMarshal(t, b)
	}
	l, err := makeList(entries, nil, nil, public, time.Now(), io.Discard) //nolint:forbidigo // when the list says it was written, waits on nothing
	if err != nil {
		t.Fatal(err)
	}
	var channels []string
	for _, b := range l.Channels {
		channels = append(channels, b.Channel)
	}
	if strings.Join(channels, " ") != "main" {
		t.Errorf("the list holds %v, want main alone", channels)
	}
}

func mustMarshal(t *testing.T, v any) []byte {
	t.Helper()
	data, err := json.Marshal(v)
	if err != nil {
		t.Fatal(err)
	}
	return data
}
