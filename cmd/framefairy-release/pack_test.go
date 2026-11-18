package main

import (
	"encoding/json"
	"io"
	"os"
	"path/filepath"
	"slices"
	"strings"
	"testing"
	"time"

	"framefairy/updates"
)

// packed packs an app the way publish.yml does and returns the folder.
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
	if err := pack(in, out, io.Discard); err != nil {
		t.Fatal(err)
	}
	return out
}

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
	appProgram = "Frame Fairy.app/Contents/MacOS/framefairy-app"
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
	first := packed(t, map[string][]byte{appProgram: []byte("build 1"), ffmpegFile: ffmpeg})
	second := packed(t, map[string][]byte{appProgram: []byte("build 2"), ffmpegFile: ffmpeg})
	entry := filepath.Join(t.TempDir(), "entry.json")
	if err := os.WriteFile(entry, []byte("{}"), 0o644); err != nil {
		t.Fatal(err)
	}
	gh := &fakeGitHub{}

	if err := publish(gh, filepath.Join(first, packedZip), "app-main-1.zip", first, entry, "main", "c1"); err != nil {
		t.Fatal(err)
	}
	p1 := partsIn(t, first)
	pieces := []string{pieceOf(t, p1, appProgram), pieceOf(t, p1, ffmpegFile)}
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
	if err := publish(gh, filepath.Join(second, packedZip), "app-main-2.zip", second, entry, "main", "c2"); err != nil {
		t.Fatal(err)
	}
	p2 := partsIn(t, second)
	want = []string{"upload " + pieceOf(t, p2, appProgram), "upload app-main-2.zip", "upload app-main-2.zip.zst", "upload app-main-2.parts.json", "upload channel-main.json"}
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
	listed := packed(t, map[string][]byte{appProgram: []byte("listed"), ffmpegFile: ffmpeg})
	gone := packed(t, map[string][]byte{appProgram: []byte("gone"), ffmpegFile: ffmpeg})
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
	gh.put(pieceOf(t, p, appProgram), []byte("piece"), old)
	gh.put(pieceOf(t, p, ffmpegFile), []byte("piece"), old)
	gh.put(pieceOf(t, g, appProgram), []byte("piece"), old)
	fresh := updates.PieceName(strings.Repeat("f", 64))
	gh.put(fresh, []byte("piece"), now.Add(-10*time.Minute))
	l := updates.List{Channels: []updates.Build{{Channel: "main", URL: "https://example.com/dev/app-main-1.zip"}}}

	if err := removeUnreachable(gh, l, now, io.Discard); err != nil {
		t.Fatal(err)
	}
	for _, name := range []string{"app-pr-9-1.zip", "app-pr-9-1.zip.zst", "app-pr-9-1.parts.json", pieceOf(t, g, appProgram)} {
		if _, ok := gh.release[name]; ok {
			t.Errorf("%s, which no listed build reaches, is still there", name)
		}
	}
	for _, name := range []string{"app-main-1.zip", "app-main-1.zip.zst", "app-main-1.parts.json", pieceOf(t, p, appProgram), pieceOf(t, p, ffmpegFile), fresh} {
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

// A packed build signed with its parts says where they will be, and the
// app finds it whole: its own check passes and its parts make its zip.
//
// On the real disk: the release tool reads the packed build and writes the entry as files.
func TestASignedBuildSaysWhereItsPartsAre(t *testing.T) {
	release(t)
	dir := packed(t, theApp("main"))
	out := filepath.Join(t.TempDir(), "entry.json")
	err := sign([]string{"-zip", filepath.Join(dir, packedZip), "-parts", filepath.Join(dir, partsFile),
		"-channel", "main", "-name", "main", "-version", "0.1.0-main.abc1234", "-commit", "abc1234def56",
		"-url", "https://example.com/dev/app-main-1.zip", "-out", out})
	if err != nil {
		t.Fatal(err)
	}
	b := readBuild(t, out)
	if b.Parts != "https://example.com/dev/app-main-1.parts.json" {
		t.Errorf("the entry says its parts are at %q", b.Parts)
	}
	whole, err := os.ReadFile(filepath.Join(dir, packedWhole))
	if err != nil {
		t.Fatal(err)
	}
	if b.Packed != "https://example.com/dev/app-main-1.zip.zst" || b.PackedSize != int64(len(whole)) {
		t.Errorf("the entry says its packed zip is at %q, %d bytes, want %d", b.Packed, b.PackedSize, len(whole))
	}
	if err := b.Check(); err != nil {
		t.Errorf("the app refuses the entry: %v", err)
	}

	// Parts of another build are no parts of this one.
	other := packed(t, theApp("pr-7"))
	err = signBuild(filepath.Join(dir, packedZip), "main", "main", "0.1.0-main.abc1234", "abc1234def56",
		"https://example.com/dev/app-main-1.zip", filepath.Join(other, partsFile), out)
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
	err = signBuild(filepath.Join(dir, packedZip), "main", "main", "0.1.0-main.abc1234", "abc1234def56",
		"https://example.com/dev/app-main-1.zip", filepath.Join(dir, partsFile), out)
	if err == nil {
		t.Error("a build was signed with the packed zip of another")
	}
}
