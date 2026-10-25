package main

import (
	"bytes"
	"crypto/sha256"
	"encoding/hex"
	"errors"
	"io"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"testing"

	"github.com/spf13/afero"
)

// walkVideos keeps every video a bridge makes for a walk, by what it was
// asked for, in the folder FRAMEFAIRY_WALK_VIDEOS names, which make walks
// and TestWalks share between the bridges of a run and CI keeps between
// runs, or else in a folder of this bridge's own. A walk that adds the
// same video again gets a copy of it rather than waiting for ffmpeg to
// make it again: the 54 videos the sequences added took 100 s, and the
// one of ten minutes 9 s every time, issue #527. ffmpeg makes the same
// bytes from the same arguments, so a copy is the video the walk would
// have had.
//
// On the real disk: ffmpeg writes the videos and the app reads them, in another program too.
func walkVideos(t *testing.T) *made {
	dir := os.Getenv("FRAMEFAIRY_WALK_VIDEOS")
	if dir == "" {
		dir = t.TempDir()
	}
	return &made{fs: afero.NewOsFs(), dir: dir, recipe: videoRecipe()}
}

// videoRecipe is the sum of what makes the bridge's videos: this
// package's bridge_test.go, which holds every function that makes one,
// and the ffmpeg on the PATH, by the line that says its version. Empty
// when either cannot be read, and then the videos are kept for this
// bridge only.
//
// On the real disk: the source of the test and the ffmpeg that runs it are files.
func videoRecipe() string {
	source, err := os.ReadFile("bridge_test.go")
	if err != nil {
		return ""
	}
	said, err := exec.Command("ffmpeg", "-version").Output()
	if err != nil {
		return ""
	}
	version, _, _ := bytes.Cut(said, []byte("\n"))
	sum := sha256.Sum256(append(append(source, 0), version...))
	return hex.EncodeToString(sum[:6])
}

// copy puts a copy of the video asked for as what at path, made by build
// the first time, so the walk can do with it what it likes.
func (m *made) copy(path, what string, build func(path string) error) error {
	kept, err := m.video(what, filepath.Ext(path), build)
	if err != nil {
		return err
	}
	src, err := m.fs.Open(kept)
	if err != nil {
		return err
	}
	defer src.Close()
	dst, err := m.fs.Create(path)
	if err != nil {
		return err
	}
	if _, err := io.Copy(dst, src); err != nil {
		_ = dst.Close()
		return err
	}
	return dst.Close()
}

// A video asked for again is the one made the first time, without making
// it again, also by another program that keeps its videos in the same
// folder by the same recipe. A video asked for otherwise is made, and a
// change of recipe makes it anew and lets the old go when the folder is
// pruned. One that failed to be made is made when asked for again.
func TestAVideoAskedForAgainIsTheOneMadeBefore(t *testing.T) {
	fsys := afero.NewMemMapFs()
	n := 0
	build := func(path string) error {
		n++
		return afero.WriteFile(fsys, path, []byte{byte(n)}, 0o644)
	}
	read := func(path string) byte {
		t.Helper()
		b, err := afero.ReadFile(fsys, path)
		if err != nil || len(b) != 1 {
			t.Fatalf("%s holds %v, %v", path, b, err)
		}
		return b[0]
	}
	one := &made{fs: fsys, dir: "/kept", recipe: "aaaa"}
	beside := &made{fs: fsys, dir: "/kept", recipe: "aaaa"}
	next := &made{fs: fsys, dir: "/kept", recipe: "bbbb"}
	alone := &made{fs: fsys, dir: "/alone"}
	for _, step := range []struct {
		m      *made
		path   string
		what   string
		video  byte
		builds int
	}{
		{one, "/home/folge-1.mp4", "seconds=120", 1, 1},
		{one, "/home/folge-2.mp4", "seconds=120", 1, 1},
		{one, "/home/folge-3/erste-erinnerung.mp4", "seconds=120", 1, 1},
		{one, "/home/folge-4.mp4", "seconds=600", 2, 2},
		{beside, "/home/folge-5.mp4", "seconds=600", 2, 2},
		{beside, "/home/folge-6.mp4", "seconds=120", 1, 2},
		{next, "/home/folge-7.mp4", "seconds=120", 3, 3},
		{alone, "/home/folge-8.mp4", "seconds=120", 4, 4},
		{alone, "/home/folge-9.mp4", "seconds=120", 4, 4},
	} {
		if err := fsys.MkdirAll(filepath.Dir(step.path), 0o755); err != nil {
			t.Fatal(err)
		}
		if err := step.m.copy(step.path, step.what, build); err != nil {
			t.Fatalf("%s: %v", step.path, err)
		}
		if got := read(step.path); got != step.video || n != step.builds {
			t.Errorf("%s as %q holds video %d after %d made, want video %d after %d", step.path, step.what, got, n, step.video, step.builds)
		}
	}
	// A copy is the walk's own: what it does to it leaves the one kept.
	if err := afero.WriteFile(fsys, "/home/folge-1.mp4", []byte{99}, 0o644); err != nil {
		t.Fatal(err)
	}
	if err := one.copy("/home/folge-10.mp4", "seconds=120", build); err != nil || read("/home/folge-10.mp4") != 1 {
		t.Errorf("a video changed by a walk changed the one kept: %v", err)
	}

	fails := func(string) error { return errors.New("ffmpeg failed") }
	if err := next.copy("/home/folge-11.mp4", "seconds=30", fails); err == nil {
		t.Error("a video that failed to be made came back without the failure")
	}
	if err := next.copy("/home/folge-11.mp4", "seconds=30", build); err != nil || read("/home/folge-11.mp4") != 5 {
		t.Errorf("a video asked for again after it failed was not made anew: %v", err)
	}

	if err := (&made{fs: fsys, dir: "/kept", recipe: "bbbb"}).prune(); err != nil {
		t.Fatal(err)
	}
	entries, err := afero.ReadDir(fsys, "/kept")
	if err != nil {
		t.Fatal(err)
	}
	var left []string
	for _, e := range entries {
		left = append(left, e.Name())
	}
	if len(left) != 2 || !strings.HasPrefix(left[0], "bbbb-") || !strings.HasPrefix(left[1], "bbbb-") {
		t.Errorf("after pruning for recipe bbbb the folder holds %v, want the two videos bbbb made", left)
	}
	if err := (&made{fs: fsys, dir: "/nothing", recipe: "bbbb"}).prune(); err != nil {
		t.Errorf("pruning a folder that is not there gives %v", err)
	}
}
