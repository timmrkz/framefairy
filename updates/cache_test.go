package updates

import (
	"context"
	"crypto/ed25519"
	"crypto/rand"
	"os"
	"path/filepath"
	"testing"
)

// install checks the list and downloads the channel's build, the way the
// app does, and says what the app it unpacked says.
func install(t *testing.T, src *Source, key ed25519.PrivateKey) string {
	t.Helper()
	u := newUpdater(t, src, "0.3.0-main.4", key)
	if rel, err := u.Check(context.Background()); err != nil || rel == nil {
		t.Fatalf("check: %v, %v", rel, err)
	}
	if err := u.DownloadAndInstall(context.Background()); err != nil {
		t.Fatal(err)
	}
	staged := u.DownloadedPath()
	t.Cleanup(func() { _ = os.RemoveAll(filepath.Dir(staged)) })
	got, err := os.ReadFile(filepath.Join(staged, "Contents", "MacOS", "framefairy-app"))
	if err != nil {
		t.Fatal(err)
	}
	return string(got)
}

func cachedSource(t *testing.T, srv string) *Source {
	t.Helper()
	return &Source{URL: srv + "/dev/channels.json", Own: "main", Cache: filepath.Join(t.TempDir(), "builds")}
}

// Going back to a channel whose build was downloaded before takes it from
// the cache: no second download, and the same app arrives.
func TestAChannelPickedAgainIsNotDownloadedAgain(t *testing.T) {
	key := testKey(t)
	s := &served{zip: appZip(t, "pull request 30")}
	srv := serve(t, s)
	b := build(t, key, "pr-30", "0.3.0-pr30.abc1234", srv.URL+"/dev/app.zip", s.zip)
	s.list = listJSON(t, b)
	src := cachedSource(t, srv.URL)
	src.Client = srv.Client()
	src.Picked = func() string { return "pr-30" }
	var progressed int64
	src.Progress = func(w, _ int64) { progressed = w }

	if got := install(t, src, key); got != "pull request 30" {
		t.Fatalf("first install unpacked %q", got)
	}
	if _, err := os.Stat(filepath.Join(src.Cache, b.SHA256+".zip")); err != nil {
		t.Fatalf("the build was not kept: %v", err)
	}
	progressed = 0
	if got := install(t, src, key); got != "pull request 30" {
		t.Fatalf("second install unpacked %q", got)
	}
	if n := s.zips.Load(); n != 1 {
		t.Errorf("the build was downloaded %d times", n)
	}
	// The fill still runs to the end, from the disk.
	if progressed != int64(len(s.zip)) {
		t.Errorf("progress stopped at %d of %d", progressed, len(s.zip))
	}
}

// A kept build that was changed on disk is never installed: it is thrown
// away and the build downloaded again.
func TestAChangedKeptBuildIsDownloadedAgain(t *testing.T) {
	key := testKey(t)
	s := &served{zip: appZip(t, "the real one")}
	srv := serve(t, s)
	b := build(t, key, "main", "0.3.0-main.abc1234", srv.URL+"/dev/app.zip", s.zip)
	s.list = listJSON(t, b)
	src := cachedSource(t, srv.URL)
	src.Client = srv.Client()
	install(t, src, key)

	kept := filepath.Join(src.Cache, b.SHA256+".zip")
	swapped := appZip(t, "a changed one")
	// The same size, so only the checksum can tell.
	swapped = append(swapped[:0:0], swapped...)
	for len(swapped) < len(s.zip) {
		swapped = append(swapped, 0)
	}
	if err := os.WriteFile(kept, swapped[:len(s.zip)], 0o600); err != nil {
		t.Fatal(err)
	}
	if got := install(t, src, key); got != "the real one" {
		t.Fatalf("installed %q", got)
	}
	if n := s.zips.Load(); n != 2 {
		t.Errorf("the build was downloaded %d times, want 2", n)
	}
}

// Whatever the list no longer names goes the next time it is read: a pull
// request closed, or a build a newer push replaced. What it still names
// stays.
func TestKeptBuildsGoWhenTheListDropsThem(t *testing.T) {
	key := testKey(t)
	s := &served{zip: appZip(t, "pr 30")}
	srv := serve(t, s)
	pr30 := build(t, key, "pr-30", "0.3.0-pr30.abc1234", srv.URL+"/dev/app.zip", s.zip)
	main := build(t, key, "main", "0.3.0-main.def5678", srv.URL+"/dev/main.zip", appZip(t, "main"))
	s.list = listJSON(t, main, pr30)
	src := cachedSource(t, srv.URL)
	src.Client = srv.Client()
	src.Picked = func() string { return "pr-30" }
	install(t, src, key)
	// A download cut off earlier, and something that is no build at all.
	for _, name := range []string{main.SHA256 + ".part", "stray.zip"} {
		if err := os.WriteFile(filepath.Join(src.Cache, name), []byte("x"), 0o600); err != nil {
			t.Fatal(err)
		}
	}

	if _, err := src.Fetch(context.Background()); err != nil {
		t.Fatal(err)
	}
	left, _ := os.ReadDir(src.Cache)
	var names []string
	for _, e := range left {
		names = append(names, e.Name())
	}
	if len(names) != 2 {
		t.Errorf("while both are listed, kept %v", names)
	}

	// Pull request 30 is merged, and main has a newer build.
	s.list = listJSON(t, build(t, key, "main", "0.3.0-main.0a1b2c3", srv.URL+"/dev/main.zip", appZip(t, "newer main")))
	if _, err := src.Fetch(context.Background()); err != nil {
		t.Fatal(err)
	}
	if left, _ := os.ReadDir(src.Cache); len(left) != 0 {
		t.Errorf("kept %v after the list dropped them", left)
	}
}

// A download let go of half way, because another channel was picked,
// leaves nothing behind.
func TestACancelledDownloadKeepsNothing(t *testing.T) {
	key := testKey(t)
	// Big enough to arrive in many reads, so the cancel lands half way.
	noise := make([]byte, 4<<20)
	_, _ = rand.Read(noise)
	s := &served{zip: appZip(t, string(noise))}
	srv := serve(t, s)
	s.list = listJSON(t, build(t, key, "pr-30", "0.3.0-pr30.abc1234", srv.URL+"/dev/app.zip", s.zip))
	src := cachedSource(t, srv.URL)
	src.Client = srv.Client()
	src.Picked = func() string { return "pr-30" }
	ctx, cancel := context.WithCancel(context.Background())
	src.Progress = func(int64, int64) { cancel() }
	u := newUpdater(t, src, "0.3.0-main.4", key)
	if _, err := u.Check(context.Background()); err != nil {
		t.Fatal(err)
	}
	if err := u.DownloadAndInstall(ctx); err == nil {
		t.Fatal("the download was not cut off")
	}
	if left, _ := os.ReadDir(src.Cache); len(left) != 0 {
		t.Errorf("kept %v", left)
	}
}
