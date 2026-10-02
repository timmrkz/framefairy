package main

import (
	"archive/zip"
	"bytes"
	"crypto/ed25519"
	"crypto/rand"
	"encoding/base64"
	"encoding/hex"
	"encoding/json"
	"errors"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"strings"
	"sync"
	"sync/atomic"
	"testing"
	"time"

	"github.com/wailsapp/wails/v3/pkg/updater"

	"framefairy/updates"
)

const inApp = "/Applications/Frame Fairy.app/Contents/MacOS/framefairy-app"

// An updater with nothing to show it to and nothing to quit.
type quietHost struct{}

func (quietHost) Emit(string, ...any) bool                              { return false }
func (quietHost) OnEvent(string, func(any)) func()                      { return func() {} }
func (quietHost) OpenWindow(updater.WindowOptions) updater.WindowHandle { return nil }
func (quietHost) Quit()                                                 {}

// A channel list and two builds on a server of their own. slow holds each
// download until it is let go.
type channelServer struct {
	srv  *httptest.Server
	key  ed25519.PrivateKey
	mu   sync.Mutex
	list []byte
	zips map[string][]byte
	slow chan struct{}
	// slowOnly holds back only the builds whose path has this in it.
	slowOnly string
	hits     atomic.Int32
	// fetched counts the downloads of each build, by its path.
	fetched map[string]int
	// newest is a channel's commit still to be built, as the list says it.
	newest map[string]string
}

func newChannelServer(t *testing.T) *channelServer {
	t.Helper()
	_, key, err := ed25519.GenerateKey(rand.Reader)
	if err != nil {
		t.Fatal(err)
	}
	cs := &channelServer{key: key, zips: map[string][]byte{}, fetched: map[string]int{}}
	cs.srv = httptest.NewTLSServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		cs.mu.Lock()
		list, data, ok := cs.list, cs.zips[r.URL.Path], false
		_, ok = cs.zips[r.URL.Path]
		if ok {
			cs.fetched[r.URL.Path]++
		}
		slow := cs.slow
		if cs.slowOnly != "" && !strings.Contains(r.URL.Path, cs.slowOnly) {
			slow = nil
		}
		cs.mu.Unlock()
		switch {
		case r.URL.Path == "/channels.json":
			cs.hits.Add(1)
			_, _ = w.Write(list)
		case ok:
			if slow != nil {
				<-slow
			}
			_, _ = w.Write(data)
		default:
			http.NotFound(w, r)
		}
	}))
	t.Cleanup(cs.srv.Close)
	return cs
}

func (cs *channelServer) publish(t *testing.T, builds ...[3]string) {
	t.Helper()
	var l updates.List
	for _, b := range builds {
		channel, version, says := b[0], b[1], b[2]
		data := zipOf(t, says)
		digest, size, _ := updates.Digest(bytes.NewReader(data))
		path := "/" + channel + "-" + version + ".zip"
		cs.mu.Lock()
		cs.zips[path] = data
		cs.mu.Unlock()
		entry := updates.Build{
			Channel: channel, Name: "#" + channel, Version: version, Commit: "abc1234",
			URL: cs.srv.URL + path, Size: size, SHA256: hex.EncodeToString(digest),
			Signature: updates.Sign(cs.key, digest), Published: time.Now(),
			Newest: cs.newest[channel],
		}
		entry.Claim = updates.SignClaim(cs.key, entry)
		l.Channels = append(l.Channels, entry)
	}
	data, _ := json.Marshal(l)
	cs.mu.Lock()
	cs.list = data
	cs.mu.Unlock()
}

func zipOf(t *testing.T, says string) []byte {
	t.Helper()
	var buf bytes.Buffer
	w := zip.NewWriter(&buf)
	f, err := w.Create("Frame Fairy.app/Contents/MacOS/framefairy-app")
	if err != nil {
		t.Fatal(err)
	}
	_, _ = f.Write([]byte(says))
	_ = w.Close()
	return buf.Bytes()
}

func (cs *channelServer) publicKey() string {
	return base64.StdEncoding.EncodeToString(cs.key.Public().(ed25519.PublicKey))
}

// An updating as the app makes it, reading from the server above, and
// what it has sent so far. The states are sent from the updating's own
// goroutines, so they are read through sent, under the same lock.
func newTestUpdating(t *testing.T, cs *channelServer, busy bool) (*updating, func() []UpdateState) {
	t.Helper()
	var mu sync.Mutex
	var sent []UpdateState
	picked := ""
	c := &updating{
		busy: func() bool { return busy },
		emit: func(s UpdateState) {
			mu.Lock()
			sent = append(sent, s)
			mu.Unlock()
		},
		load: func() string {
			mu.Lock()
			defer mu.Unlock()
			return picked
		},
		save: func(ch string) error {
			mu.Lock()
			defer mu.Unlock()
			picked = ch
			return nil
		},
	}
	src := &updates.Source{URL: cs.srv.URL + "/channels.json", Client: cs.srv.Client()}
	c.setUp(updater.New(quietHost{}), cs.publicKey(), src, inApp, "darwin")
	if c.state.Off != "" {
		t.Fatalf("off: %s", c.state.Off)
	}
	return c, func() []UpdateState {
		mu.Lock()
		defer mu.Unlock()
		return append([]UpdateState(nil), sent...)
	}
}

func waitFor(t *testing.T, c *updating, what string, ok func(UpdateState) bool) UpdateState {
	t.Helper()
	deadline := time.Now().Add(10 * time.Second)
	for time.Now().Before(deadline) {
		if s := c.State(); ok(s) {
			return s
		}
		time.Sleep(10 * time.Millisecond)
	}
	t.Fatalf("never %s: %+v", what, c.State())
	return UpdateState{}
}

func cleanStaged(t *testing.T, c *updating) {
	t.Cleanup(func() {
		if p := c.u.DownloadedPath(); p != "" {
			_ = os.RemoveAll(filepath.Dir(p))
		}
	})
}

// What stops a build from updating itself says so, in words.
func TestUpdatesSayWhyTheyAreOff(t *testing.T) {
	cs := newChannelServer(t)
	for _, c := range []struct{ key, exe, goos, says string }{
		{"", inApp, "darwin", "no update key"},
		{"nonsense", inApp, "darwin", "not a key"},
		{cs.publicKey(), inApp, "linux", "Only the Mac app"},
		{cs.publicKey(), "/Users/tim/framefairy/bin/framefairy-app", "darwin", "not Frame Fairy.app"},
	} {
		u := &updating{load: func() string { return "" }}
		u.setUp(updater.New(quietHost{}), c.key, &updates.Source{URL: cs.srv.URL}, c.exe, c.goos)
		if !strings.Contains(u.state.Off, c.says) || u.u != nil {
			t.Errorf("%s on %s with %q: off says %q", c.exe, c.goos, c.key, u.state.Off)
		}
		if err := u.Follow("main"); err == nil {
			t.Error("a build that is off followed a channel")
		}
		u.check()
	}
}

// Picking a pull request downloads its build and says when it is ready,
// with the fill on the way.
func TestPickingAChannelFetchesItsBuild(t *testing.T) {
	cs := newChannelServer(t)
	cs.publish(t, [3]string{"main", "0.3.0-main.4", "main"}, [3]string{"pr-20", "0.3.0-pr20.9", "twenty"})
	c, sent := newTestUpdating(t, cs, false)
	cleanStaged(t, c)
	if err := c.Follow("pr-20"); err != nil {
		t.Fatal(err)
	}
	s := waitFor(t, c, "ready", func(s UpdateState) bool { return s.Phase == "ready" })
	if s.Next != "0.3.0-pr20.9" || s.NextCommit != "abc1234" || s.Follows != "pr-20" || s.Picked != "pr-20" || len(s.Channels) != 2 {
		t.Errorf("ready as %+v", s)
	}
	if s.Written != s.Total || s.Total == 0 {
		t.Errorf("the fill ended at %d of %d", s.Written, s.Total)
	}
	var phases []string
	for _, st := range sent() {
		if st.Phase != "" && (len(phases) == 0 || phases[len(phases)-1] != st.Phase) {
			phases = append(phases, st.Phase)
		}
	}
	if strings.Join(phases, " ") != "checking downloading ready" {
		t.Errorf("went through %v", phases)
	}
	if got, _ := os.ReadFile(filepath.Join(c.u.DownloadedPath(), "Contents/MacOS/framefairy-app")); string(got) != "twenty" {
		t.Errorf("staged %q", got)
	}
}

// A pull request that was merged leaves the list, and a build that followed
// it downloads nothing: not main, not anything, until another channel is
// picked. Tim followed pull request 25, it was merged, and the app went on
// to download main by itself.
func TestAMergedPullRequestDownloadsNothing(t *testing.T) {
	cs := newChannelServer(t)
	cs.publish(t, [3]string{"main", "0.3.0-main.5", "main"})
	c, _ := newTestUpdating(t, cs, false)
	cleanStaged(t, c)
	_ = c.Follow("pr-18")
	s := waitFor(t, c, "gone", func(s UpdateState) bool { return s.Phase == "gone" })
	if s.Gone != "pr-18" || s.Follows != "" || s.Next != "" || c.u.DownloadedPath() != "" {
		t.Errorf("%+v, staged %q", s, c.u.DownloadedPath())
	}
	// Picking main is how it goes on.
	_ = c.Follow("main")
	s = waitFor(t, c, "ready with main", func(s UpdateState) bool { return s.Phase == "ready" })
	if s.Follows != "main" || s.Gone != "" || s.Next != "0.3.0-main.5" {
		t.Errorf("%+v", s)
	}
}

// A push whose build has not come yet is said beside the channel
// followed, and only that channel's. The build there is still downloads:
// it is the newest there is.
func TestACommitStillToBeBuiltIsSaid(t *testing.T) {
	cs := newChannelServer(t)
	cs.newest = map[string]string{"pr-20": "def5678abcde", "main": "0123456789ab"}
	cs.publish(t, [3]string{"main", "0.3.0-main.4", "main"}, [3]string{"pr-20", "0.3.0-pr20.9", "twenty"})
	c, _ := newTestUpdating(t, cs, false)
	cleanStaged(t, c)
	_ = c.Follow("pr-20")
	s := waitFor(t, c, "ready", func(s UpdateState) bool { return s.Phase == "ready" })
	if s.Building != "def5678abcde" || s.Next != "0.3.0-pr20.9" {
		t.Errorf("%+v", s)
	}
	// Once it is built the list says nothing more, and neither does the
	// page.
	cs.newest = nil
	cs.publish(t, [3]string{"main", "0.3.0-main.4", "main"}, [3]string{"pr-20", "0.3.0-pr20.9", "twenty"})
	c.check()
	if s := c.State(); s.Building != "" {
		t.Errorf("still says %q is being built", s.Building)
	}
}

// While a commit is being built the app looks every twenty seconds, so the
// build is downloaded soon after it lands, and every ten minutes when
// nothing is on its way.
func TestItLooksOftenWhileACommitIsBeingBuilt(t *testing.T) {
	if got := nextCheck(UpdateState{Building: "def5678abcde"}); got != 20*time.Second {
		t.Errorf("while building, looks every %v", got)
	}
	if got := nextCheck(UpdateState{}); got != 10*time.Minute {
		t.Errorf("otherwise, looks every %v", got)
	}
}

// The running build is the channel's build, so there is nothing to fetch.
func TestTheRunningBuildIsCurrent(t *testing.T) {
	cs := newChannelServer(t)
	cs.publish(t, [3]string{"main", runningVersion(), "main"})
	c, _ := newTestUpdating(t, cs, false)
	c.check()
	if s := c.State(); s.Phase != "current" || s.Next != "" || s.Checked.IsZero() {
		t.Errorf("%+v", s)
	}
	if err := c.Restart(); err == nil {
		t.Error("restarted with nothing ready")
	}
}

// A channel list nobody can reach is a problem said in words, and the app
// carries on as it was.
func TestAFailedCheckSaysWhy(t *testing.T) {
	cs := newChannelServer(t)
	c, _ := newTestUpdating(t, cs, false)
	c.check()
	s := c.State()
	if s.Phase != "failed" || !strings.HasPrefix(s.Problem, "The channel list is not readable") || s.Checked.IsZero() {
		t.Errorf("%+v", s)
	}
	if strings.Contains(s.Problem, "updater:") {
		t.Errorf("the problem reads %q", s.Problem)
	}
}

// Restart waits for work in hand: the swap happens once the app has quit,
// and quitting now would stop a search or a render half way.
func TestRestartWaitsForWork(t *testing.T) {
	cs := newChannelServer(t)
	cs.publish(t, [3]string{"main", "0.3.0-main.5", "main"})
	c, _ := newTestUpdating(t, cs, true)
	cleanStaged(t, c)
	_ = c.Follow("main")
	waitFor(t, c, "ready", func(s UpdateState) bool { return s.Phase == "ready" })
	if err := c.Restart(); err == nil || !strings.Contains(err.Error(), "still running") {
		t.Errorf("restarted while work ran: %v", err)
	}
}

// A channel picked while a download runs takes over at once: the download
// of the channel before is stopped, the state shown is the new channel's
// from the moment of the pick, and nothing the old download says after it
// reaches the screen. Tim picked pull request 23 while main downloaded,
// and watched main's download to the end before anything changed. Here
// main's download never finishes at all, so the new pick only gets to
// ready if the old one really was let go of.
func TestAPickStopsTheDownloadBefore(t *testing.T) {
	cs := newChannelServer(t)
	cs.publish(t, [3]string{"main", "0.3.0-main.5", "main"}, [3]string{"pr-20", "0.3.0-pr20.9", "twenty"})
	cs.mu.Lock()
	cs.slow, cs.slowOnly = make(chan struct{}), "/main-"
	cs.mu.Unlock()
	t.Cleanup(func() { close(cs.slow) })
	c, sent := newTestUpdating(t, cs, false)
	cleanStaged(t, c)
	_ = c.Follow("main")
	waitFor(t, c, "downloading main", func(s UpdateState) bool {
		return s.Phase == "downloading" && s.Next == "0.3.0-main.5"
	})

	_ = c.Follow("pr-20")
	after := len(sent())
	// At once, before any check has answered.
	if s := c.State(); s.Picked != "pr-20" || s.Phase != "checking" || s.Next != "" || s.Follows != "pr-20" {
		t.Errorf("right after the pick: %+v", s)
	}
	s := waitFor(t, c, "ready with pr-20", func(s UpdateState) bool {
		return s.Phase == "ready" && s.Next == "0.3.0-pr20.9"
	})
	if s.Follows != "pr-20" || s.Problem != "" {
		t.Errorf("%+v", s)
	}
	for _, st := range sent()[after:] {
		if st.Next == "0.3.0-main.5" || st.Phase == "failed" {
			t.Errorf("after the pick the screen was told %+v", st)
		}
	}
}

// Everything the interface and the timer can do, at once.
func TestUpdatesFromEverywhereAtOnce(t *testing.T) {
	cs := newChannelServer(t)
	cs.publish(t, [3]string{"main", "0.3.0-main.5", "main"}, [3]string{"pr-20", "0.3.0-pr20.9", "twenty"})
	c, _ := newTestUpdating(t, cs, false)
	cleanStaged(t, c)
	var wg sync.WaitGroup
	for i := range 8 {
		wg.Go(func() {
			for j := range 10 {
				switch (i + j) % 4 {
				case 0:
					_ = c.Follow([]string{"main", "pr-20"}[j%2])
				case 1:
					c.check()
				case 2:
					_ = c.State()
				case 3:
					_ = c.Restart()
				}
			}
		})
	}
	wg.Wait()
	_ = c.Follow("pr-20")
	waitFor(t, c, "ready with pr-20", func(s UpdateState) bool { return s.Phase == "ready" && s.Next == "0.3.0-pr20.9" })
}

func TestFollowRefusesWhatIsNotAChannel(t *testing.T) {
	cs := newChannelServer(t)
	c, _ := newTestUpdating(t, cs, false)
	for _, bad := range []string{"../x", "pr-", "pr-0", "Main", "pr-18 "} {
		if err := c.Follow(bad); err == nil {
			t.Errorf("followed %q", bad)
		}
	}
}

func TestPlainUpdateError(t *testing.T) {
	got := plainUpdateError(errors.New("updater: all providers failed: channels: the channel list answered 404 Not Found"))
	if got != "The channel list answered 404 Not Found." {
		t.Errorf("%q", got)
	}
}

// Opening the settings lists the channels, even for a build made by make
// that has never looked, and downloads nothing.
func TestTheSettingsListTheChannelsWithoutDownloading(t *testing.T) {
	cs := newChannelServer(t)
	cs.publish(t, [3]string{"main", "0.3.0-main.5", "main"}, [3]string{"pr-20", "0.3.0-pr20.9", "twenty"})
	c, _ := newTestUpdating(t, cs, false)
	c.refreshList()
	c.refreshList()
	s := c.State()
	// A build made by make follows nothing until a channel is picked.
	if len(s.Channels) != 2 || s.Phase != "" || s.Follows != "" || s.Gone != "" {
		t.Errorf("%+v", s)
	}
	if n := cs.hits.Load(); n != 1 {
		t.Errorf("read the list %d times", n)
	}
}

// fetches is how often a build was downloaded.
func (cs *channelServer) fetches(path string) int {
	cs.mu.Lock()
	defer cs.mu.Unlock()
	return cs.fetched[path]
}

// A build downloaded once is kept: picking another channel and coming back
// has it at once, with nothing downloaded again. Tim went from a channel
// whose build was ready to another and back, nothing pushed in between,
// and downloaded it a second time.
func TestABuildDownloadedIsKeptAcrossPicks(t *testing.T) {
	cs := newChannelServer(t)
	cs.publish(t, [3]string{"main", "0.3.0-main.5", "main"}, [3]string{"pr-20", "0.3.0-pr20.9", "twenty"})
	c, _ := newTestUpdating(t, cs, false)
	c.src.Cache = t.TempDir()
	cleanStaged(t, c)
	ready := func(version string) {
		t.Helper()
		waitFor(t, c, "ready with "+version, func(s UpdateState) bool { return s.Phase == "ready" && s.Next == version })
	}
	_ = c.Follow("pr-20")
	ready("0.3.0-pr20.9")
	_ = c.Follow("main")
	ready("0.3.0-main.5")
	_ = c.Follow("pr-20")
	ready("0.3.0-pr20.9")
	_ = c.Follow("main")
	ready("0.3.0-main.5")
	if n := cs.fetches("/pr-20-0.3.0-pr20.9.zip"); n != 1 {
		t.Errorf("pull request 20 downloaded %d times", n)
	}
	if n := cs.fetches("/main-0.3.0-main.5.zip"); n != 1 {
		t.Errorf("main downloaded %d times", n)
	}
	if got, _ := os.ReadFile(filepath.Join(c.u.DownloadedPath(), "Contents/MacOS/framefairy-app")); string(got) != "main" {
		t.Errorf("staged %q", got)
	}
}

// A download let go of because another channel was picked goes on, and
// coming back to its channel has it without downloading it again. Tim
// looked at another channel while one downloaded, came back, and it
// downloaded from the start.
func TestADownloadLetGoOfIsThereOnComingBack(t *testing.T) {
	cs := newChannelServer(t)
	cs.publish(t, [3]string{"main", "0.3.0-main.5", "main"}, [3]string{"pr-20", "0.3.0-pr20.9", "twenty"})
	release := make(chan struct{})
	cs.mu.Lock()
	cs.slow, cs.slowOnly = release, "/main-"
	cs.mu.Unlock()
	let := sync.OnceFunc(func() { close(release) })
	t.Cleanup(let)
	c, _ := newTestUpdating(t, cs, false)
	c.src.Cache = t.TempDir()
	cleanStaged(t, c)
	_ = c.Follow("main")
	waitFor(t, c, "downloading main", func(s UpdateState) bool {
		return s.Phase == "downloading" && s.Next == "0.3.0-main.5"
	})
	_ = c.Follow("pr-20")
	waitFor(t, c, "ready with pr-20", func(s UpdateState) bool {
		return s.Phase == "ready" && s.Next == "0.3.0-pr20.9"
	})
	// Main's download was waiting on the server all along, and arrives.
	let()
	deadline := time.Now().Add(10 * time.Second)
	for {
		kept, _ := filepath.Glob(filepath.Join(c.src.Cache, "*.zip"))
		if len(kept) == 2 {
			break
		}
		if time.Now().After(deadline) {
			t.Fatalf("main's download was not kept, the cache holds %v", kept)
		}
		time.Sleep(10 * time.Millisecond)
	}
	_ = c.Follow("main")
	waitFor(t, c, "ready with main", func(s UpdateState) bool {
		return s.Phase == "ready" && s.Next == "0.3.0-main.5"
	})
	if n := cs.fetches("/main-0.3.0-main.5.zip"); n != 1 {
		t.Errorf("main downloaded %d times", n)
	}
	if got, _ := os.ReadFile(filepath.Join(c.u.DownloadedPath(), "Contents/MacOS/framefairy-app")); string(got) != "main" {
		t.Errorf("staged %q", got)
	}
}

// Check reads the channel list at once, even while a download is waited
// for, so a channel made a moment ago is on the list. It waited for the
// check already running.
func TestCheckReadsTheListAtOnce(t *testing.T) {
	cs := newChannelServer(t)
	cs.publish(t, [3]string{"main", "0.3.0-main.5", "main"})
	release := make(chan struct{})
	cs.mu.Lock()
	cs.slow, cs.slowOnly = release, "/main-"
	cs.mu.Unlock()
	t.Cleanup(func() { close(release) })
	c, _ := newTestUpdating(t, cs, false)
	c.src.Cache = t.TempDir()
	cleanStaged(t, c)
	_ = c.Follow("main")
	waitFor(t, c, "downloading main", func(s UpdateState) bool { return s.Phase == "downloading" })
	cs.publish(t, [3]string{"main", "0.3.0-main.5", "main"}, [3]string{"pr-21", "0.3.0-pr21.1", "twenty-one"})
	c.checkNow()
	var ids []string
	for _, ch := range c.State().Channels {
		ids = append(ids, ch.ID)
	}
	if strings.Join(ids, " ") != "main pr-21" {
		t.Errorf("the list after Check: %v", ids)
	}
}
