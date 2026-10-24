package updates

import (
	"bytes"
	"context"
	"crypto/ed25519"
	"crypto/rand"
	"crypto/sha256"
	"encoding/hex"
	"errors"
	"io"
	"net/http"
	"os"
	"path/filepath"
	"strings"
	"sync"
	"testing"
)

// install checks the list and downloads the channel's build, the way the
// app does, and says what the app it unpacked says.
//
// On the real disk: an update is unpacked as a program on the disk.
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

// On the real disk: the updater keeps its builds in a folder on the disk.
func cachedSource(t *testing.T, srv string) *Source {
	t.Helper()
	return &Source{URL: srv + "/dev/channels.json", Own: "main", Cache: filepath.Join(t.TempDir(), "builds")}
}

// Going back to a channel whose build was downloaded before takes it from
// the cache: no second download, and the same app arrives.
//
// On the real disk: an update is a program that is downloaded, kept and run.
func TestAChannelPickedAgainIsNotDownloadedAgain(t *testing.T) {
	t.Parallel()
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

// How far a download has come only ever goes forward. The build arrives in
// the cache and is then handed to the updater from there, and that hand
// over used to count again from nothing, so the fill ran back towards
// empty just before the end and then jumped to full. Found by Tim on the
// Updates page (#334).
//
// On the real disk: an update is a program that is downloaded, kept and run.
func TestADownloadsProgressNeverGoesBack(t *testing.T) {
	t.Parallel()
	key := testKey(t)
	// Big enough that the hand over from the cache reports many times.
	noise := make([]byte, 4<<20)
	_, _ = rand.Read(noise)
	s := &served{zip: appZip(t, string(noise)), half: make(chan struct{})}
	srv := serve(t, s)
	s.list = listJSON(t, build(t, key, "pr-30", "0.3.0-pr30.abc1234", srv.URL+"/dev/app.zip", s.zip))
	src := cachedSource(t, srv.URL)
	src.Client = srv.Client()
	src.Picked = func() string { return "pr-30" }
	var (
		mu   sync.Mutex
		seen []int64
		once sync.Once
	)
	// Never left holding the build back, or the server could not close.
	t.Cleanup(func() { once.Do(func() { close(s.half) }) })
	src.Progress = func(w, _ int64) {
		mu.Lock()
		seen = append(seen, w)
		mu.Unlock()
		// The download is seen half way, then let finish.
		if w > 0 {
			once.Do(func() { close(s.half) })
		}
	}

	if got := install(t, src, key); got != string(noise) {
		t.Fatal("the build is not the build")
	}
	mu.Lock()
	defer mu.Unlock()
	for i := 1; i < len(seen); i++ {
		if seen[i] < seen[i-1] {
			t.Fatalf("progress went back from %d to %d of %d", seen[i-1], seen[i], len(s.zip))
		}
	}
	if len(seen) == 0 || seen[len(seen)-1] != int64(len(s.zip)) {
		t.Errorf("progress ended at %v, want %d", seen[max(0, len(seen)-1):], len(s.zip))
	}
}

// A kept build that was changed on disk is never installed: it is thrown
// away and the build downloaded again.
//
// On the real disk: an update is a program that is downloaded, kept and run.
func TestAChangedKeptBuildIsDownloadedAgain(t *testing.T) {
	t.Parallel()
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
//
// On the real disk: the updater keeps its builds in a folder on the disk.
func TestKeptBuildsGoWhenTheListDropsThem(t *testing.T) {
	t.Parallel()
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

// A download let go of half way, because another channel was picked, goes
// on to the end and is kept, so going back has the build at once. It used
// to be stopped and thrown away, and a look at another channel cost the
// whole download again.
//
// On the real disk: an update is a program that is downloaded, kept and run.
func TestADownloadLetGoOfGoesOnAndIsKept(t *testing.T) {
	t.Parallel()
	key := testKey(t)
	// Big enough to arrive in many reads, so the wait ends half way.
	noise := make([]byte, 4<<20)
	_, _ = rand.Read(noise)
	s := &served{zip: appZip(t, string(noise))}
	srv := serve(t, s)
	b := build(t, key, "pr-30", "0.3.0-pr30.abc1234", srv.URL+"/dev/app.zip", s.zip)
	s.list = listJSON(t, b)
	src := cachedSource(t, srv.URL)
	src.Client = srv.Client()
	src.Picked = func() string { return "pr-30" }
	// Let go of before a byte has arrived: the download has begun all the
	// same, and goes on.
	ctx, cancel := context.WithCancel(context.Background())
	cancel()
	u := newUpdater(t, src, "0.3.0-main.4", key)
	if _, err := u.Check(context.Background()); err != nil {
		t.Fatal(err)
	}
	if err := u.DownloadAndInstall(ctx); err == nil {
		t.Fatal("the wait was not cut off")
	}
	// The download began before the wait was cut off, so it is on its way
	// now, and the cache says when it has arrived.
	<-src.Arrived(b.SHA256)
	if _, err := os.Stat(filepath.Join(src.Cache, b.SHA256+".zip")); err != nil {
		left, _ := os.ReadDir(src.Cache)
		t.Fatalf("the download let go of was never kept, the cache holds %v", left)
	}
	if got := install(t, src, key); got != string(noise) {
		t.Fatal("the kept build is not the build")
	}
	if n := s.zips.Load(); n != 1 {
		t.Errorf("the build was downloaded %d times", n)
	}
}

// Asked for again while it is still on its way, a build is not downloaded
// a second time: the wait joins the download there is.
func TestADownloadOnItsWayIsJoined(t *testing.T) {
	t.Parallel()
	key := testKey(t)
	noise := make([]byte, 4<<20)
	_, _ = rand.Read(noise)
	s := &served{zip: appZip(t, string(noise))}
	srv := serve(t, s)
	s.list = listJSON(t, build(t, key, "pr-30", "0.3.0-pr30.abc1234", srv.URL+"/dev/app.zip", s.zip))
	src := cachedSource(t, srv.URL)
	src.Client = srv.Client()
	src.Picked = func() string { return "pr-30" }
	ctx, cancel := context.WithCancel(context.Background())
	cancel()
	u := newUpdater(t, src, "0.3.0-main.4", key)
	if _, err := u.Check(context.Background()); err != nil {
		t.Fatal(err)
	}
	_ = u.DownloadAndInstall(ctx)
	if got := install(t, src, key); got != string(noise) {
		t.Fatal("the build is not the build")
	}
	if n := s.zips.Load(); n != 1 {
		t.Errorf("the build was downloaded %d times", n)
	}
}

// Without a cache to go on into, a download is the wait, and a wait let go
// of stops it.
func TestWithoutACacheAWaitLetGoOfStopsTheDownload(t *testing.T) {
	t.Parallel()
	key := testKey(t)
	noise := make([]byte, 4<<20)
	_, _ = rand.Read(noise)
	s := &served{zip: appZip(t, string(noise))}
	srv := serve(t, s)
	s.list = listJSON(t, build(t, key, "pr-30", "0.3.0-pr30.abc1234", srv.URL+"/dev/app.zip", s.zip))
	src := &Source{URL: srv.URL + "/dev/channels.json", Own: "main", Client: srv.Client()}
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
}

// A download called off stops at the next part, even when every part is
// already there to be read. On CI's Mac the 4 MB of the test above came
// over the loopback before the transport saw the cancel, so the download
// read to the end and counted as arrived. Here the body is in memory, the
// way a socket that holds the whole build hands it over.
func TestADownloadCalledOffStopsWhenTheBuildIsAlreadyThere(t *testing.T) {
	t.Parallel()
	build := make([]byte, 4<<20)
	src := &Source{Client: &http.Client{Transport: roundTrip(func(r *http.Request) (*http.Response, error) {
		return &http.Response{
			StatusCode: http.StatusOK,
			Status:     "200 OK",
			Body:       io.NopCloser(bytes.NewReader(build)),
			Request:    r,
		}, nil
	})}}
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()
	var got bytes.Buffer
	err := src.stream(ctx, "https://example.invalid/app.zip", int64(len(build)), writerSink{&got}, func(int64) { cancel() })
	if !errors.Is(err, context.Canceled) {
		t.Fatalf("the download was not cut off: %v, %d of %d bytes", err, got.Len(), len(build))
	}
	if got.Len() == len(build) {
		t.Error("the whole build was read after the download was called off")
	}
}

type roundTrip func(*http.Request) (*http.Response, error)

func (f roundTrip) RoundTrip(r *http.Request) (*http.Response, error) { return f(r) }

// A build that arrived whole but could not be kept says so. Its cache
// folder went while it downloaded, and the download used to count as
// arrived, so the wait for it found nothing and said the build went from
// the cache as it arrived.
//
// On the real disk: the updater keeps its builds in a folder on the disk.
func TestABuildThatCannotBeKeptSaysSo(t *testing.T) {
	t.Parallel()
	data := []byte("a build")
	sum := sha256.Sum256(data)
	src := &Source{Cache: filepath.Join(t.TempDir(), "builds")}
	k := src.keep(hex.EncodeToString(sum[:]))
	if k == nil {
		t.Fatal("no keeper")
	}
	if err := k.write(data); err != nil {
		t.Fatal(err)
	}
	if err := os.RemoveAll(src.Cache); err != nil {
		t.Fatal(err)
	}
	if err := k.done(true); err == nil || !strings.Contains(err.Error(), "could not be kept") {
		t.Errorf("done: %v", err)
	}
}
