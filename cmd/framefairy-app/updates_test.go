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
	// conflict is a pull request the list says no longer merges into main.
	conflict map[string]bool
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
			Newest: cs.newest[channel], Conflict: cs.conflict[channel],
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
// goroutines, so they are read through sent, under the same lock. It is a
// build made on the Mac, with nothing in updates.json.
func newTestUpdating(t *testing.T, cs *channelServer, busy bool) (*updating, func() []UpdateState) {
	t.Helper()
	return testUpdating(t, cs, busy, "", "")
}

// testUpdating is newTestUpdating for a build from the channel own, or
// made on the Mac when own is empty, with stored in updates.json, picked
// by a run before.
func testUpdating(t *testing.T, cs *channelServer, busy bool, own, stored string) (*updating, func() []UpdateState) {
	t.Helper()
	var mu sync.Mutex
	var sent []UpdateState
	picked := stored
	c := &updating{
		own:  own,
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
	// Nothing it started goes on into the next test.
	t.Cleanup(c.shutDown)
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

// waitDone is waitFor that also waits for the check that got there to
// end. A check says what it found before it has finished: the build a
// newer one replaced is removed after the state says the newer one is
// ready, and a check asked for until then only leaves word for the one
// running and returns at once.
func waitDone(t *testing.T, c *updating, what string, ok func(UpdateState) bool) UpdateState {
	t.Helper()
	return waitFor(t, c, what, func(s UpdateState) bool {
		c.mu.Lock()
		defer c.mu.Unlock()
		return !c.checking && ok(s)
	})
}

// cleanStaged removes what was unpacked once nothing can unpack more.
func cleanStaged(t *testing.T, c *updating) {
	t.Cleanup(func() {
		c.shutDown()
		if p := c.staged(); p != "" {
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
	if got, _ := os.ReadFile(filepath.Join(c.staged(), "Contents/MacOS/framefairy-app")); string(got) != "twenty" {
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
	if s.Gone != "pr-18" || s.Follows != "" || s.Next != "" || c.staged() != "" {
		t.Errorf("%+v, staged %q", s, c.staged())
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
	// Nothing looks again by itself while the test runs.
	c.next = func(UpdateState) time.Duration { return time.Hour }
	_ = c.Follow("pr-20")
	// The check below is the test's own only once the pick's has ended.
	s := waitDone(t, c, "ready", func(s UpdateState) bool { return s.Phase == "ready" })
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

// A pull request that no longer merges into main is listed and says so,
// and its build still downloads: it is the branch as it is.
func TestAPullRequestInConflictIsListedAndSaysSo(t *testing.T) {
	cs := newChannelServer(t)
	cs.conflict = map[string]bool{"pr-20": true}
	cs.publish(t, [3]string{"main", "0.3.0-main.4", "main"}, [3]string{"pr-20", "0.3.0-pr20.9", "twenty"})
	c, _ := newTestUpdating(t, cs, false)
	cleanStaged(t, c)
	c.next = func(UpdateState) time.Duration { return time.Hour }
	_ = c.Follow("pr-20")
	s := waitDone(t, c, "ready", func(s UpdateState) bool { return s.Phase == "ready" })
	if s.Next != "0.3.0-pr20.9" {
		t.Errorf("the build of a pull request in conflict did not download: %+v", s)
	}
	said := map[string]bool{}
	for _, ch := range s.Channels {
		said[ch.ID] = ch.Conflict
	}
	if !said["pr-20"] || said["main"] {
		t.Errorf("the channels say %v, want pr-20 in conflict and main not", said)
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
	c.mu.Lock()
	c.state.Picked = "main"
	c.mu.Unlock()
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
	c.mu.Lock()
	c.state.Picked = "main"
	c.mu.Unlock()
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
	for _, bad := range []string{"", "../x", "pr-", "pr-0", "Main", "pr-18 "} {
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
	if got, _ := os.ReadFile(filepath.Join(c.staged(), "Contents/MacOS/framefairy-app")); string(got) != "main" {
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
	if got, _ := os.ReadFile(filepath.Join(c.staged(), "Contents/MacOS/framefairy-app")); string(got) != "main" {
		t.Errorf("staged %q", got)
	}
}

// A channel picked as the check before ends is looked for. A check asked
// for while another ran only left word for that one to look again, and
// the one running could already have read that there was nothing more,
// so the pick waited in Checking until the next look, ten minutes later.
// The test holds the first check where it has decided it is done, picks
// another channel there, and only then lets it return.
func TestAPickAsACheckEndsIsLookedFor(t *testing.T) {
	cs := newChannelServer(t)
	cs.publish(t, [3]string{"main", "0.3.0-main.5", "main"}, [3]string{"pr-20", "0.3.0-pr20.9", "twenty"})
	c, _ := newTestUpdating(t, cs, false)
	c.src.Cache = t.TempDir()
	cleanStaged(t, c)
	// Nothing looks again by itself while the test runs.
	c.next = func(UpdateState) time.Duration { return time.Hour }
	decided, goOn := make(chan struct{}), make(chan struct{})
	var once sync.Once
	c.ended = func() {
		once.Do(func() {
			close(decided)
			<-goOn
		})
	}
	let := sync.OnceFunc(func() { close(goOn) })
	t.Cleanup(let)
	_ = c.Follow("main")
	<-decided
	if s := c.State(); s.Phase != "ready" || s.Next != "0.3.0-main.5" {
		t.Fatalf("the first check ended with %+v", s)
	}
	_ = c.Follow("pr-20")
	// The pick has been heard: it ran a check of its own, or it left word
	// for the one held.
	waitFor(t, c, "the pick heard", func(s UpdateState) bool {
		c.mu.Lock()
		defer c.mu.Unlock()
		return c.again || s.Phase == "ready"
	})
	let()
	waitFor(t, c, "ready with pr-20", func(s UpdateState) bool {
		return s.Phase == "ready" && s.Next == "0.3.0-pr20.9"
	})
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
	let := sync.OnceFunc(func() { close(release) })
	t.Cleanup(let)
	c, _ := newTestUpdating(t, cs, false)
	c.src.Cache = t.TempDir()
	cleanStaged(t, c)
	c.next = func(UpdateState) time.Duration { return time.Hour }
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
	// The download arrives, and the check ends with it, so nothing goes on
	// into the next test.
	let()
	waitDone(t, c, "ready", func(s UpdateState) bool { return s.Phase == "ready" && s.Next == "0.3.0-main.5" })
}

// The app holds the newest build of the channel it follows. A build that
// is ready and not yet installed is replaced by a newer one once the newer
// one has fully arrived and passed its checks, and the one it replaces is
// removed. Until then, quitting installs the one that is ready: a newer
// build that fails, or is still on its way, never leaves the app with
// nothing to install. The second download used to throw the ready build
// away before it began.
func TestANewerBuildReplacesTheOneReady(t *testing.T) {
	was := runningApp
	runningApp = func() string { return "/Applications/Frame Fairy.app" }
	t.Cleanup(func() { runningApp = was })

	cs := newChannelServer(t)
	cs.publish(t, [3]string{"main", "0.3.0-main.5", "five"})
	c, _ := newTestUpdating(t, cs, false)
	cleanStaged(t, c)
	var mu sync.Mutex
	var from string
	c.install = func(_, staged string) error {
		mu.Lock()
		from = staged
		mu.Unlock()
		return nil
	}
	// What quitting now would put in place, and what that build says.
	quit := func() (string, string) {
		t.Helper()
		mu.Lock()
		from = ""
		mu.Unlock()
		c.installOnQuit()
		// The app did not really quit, so it goes on as before.
		c.mu.Lock()
		c.leaving = false
		c.mu.Unlock()
		mu.Lock()
		defer mu.Unlock()
		if from == "" {
			return "", ""
		}
		got, _ := os.ReadFile(filepath.Join(from, "Contents/MacOS/framefairy-app"))
		return from, string(got)
	}
	// Nothing looks again by itself while the test runs, and every check
	// below is the test's own: each waits for the one before to end.
	c.next = func(UpdateState) time.Duration { return time.Hour }
	_ = c.Follow("main")
	waitDone(t, c, "ready", func(s UpdateState) bool { return s.Phase == "ready" && s.Next == "0.3.0-main.5" })
	first, says := quit()
	if says != "five" {
		t.Fatalf("ready with main.5, quitting installs %q saying %q", first, says)
	}

	// A newer build that does not arrive whole leaves the one that is
	// ready as it was.
	cs.publish(t, [3]string{"main", "0.3.0-main.6", "six"})
	cs.mu.Lock()
	broken := append([]byte(nil), cs.zips["/main-0.3.0-main.6.zip"]...)
	broken[len(broken)/2] ^= 0xff
	cs.zips["/main-0.3.0-main.6.zip"] = broken
	cs.mu.Unlock()
	c.check()
	if s := c.State(); s.Phase != "ready" || s.Next != "0.3.0-main.5" {
		t.Errorf("after a newer build failed: %+v", s)
	}
	if at, says := quit(); at != first || says != "five" {
		t.Errorf("after a newer build failed, quitting installs %q saying %q", at, says)
	}

	// While the next one is on its way, the page still says the one that
	// is ready, and quitting installs it.
	release := make(chan struct{})
	let := sync.OnceFunc(func() { close(release) })
	t.Cleanup(let)
	cs.mu.Lock()
	cs.slow, cs.slowOnly = release, "/main-0.3.0-main.7"
	cs.mu.Unlock()
	cs.publish(t, [3]string{"main", "0.3.0-main.7", "seven"})
	go c.check()
	deadline := time.Now().Add(10 * time.Second)
	for cs.fetches("/main-0.3.0-main.7.zip") == 0 {
		if time.Now().After(deadline) {
			t.Fatal("main.7 was never asked for")
		}
		time.Sleep(10 * time.Millisecond)
	}
	if s := c.State(); s.Phase != "ready" || s.Next != "0.3.0-main.5" {
		t.Errorf("while main.7 downloads: %+v", s)
	}
	if at, says := quit(); at != first || says != "five" {
		t.Errorf("while main.7 downloads, quitting installs %q saying %q", at, says)
	}

	// Once it is here, it is the one, and the one before is gone. It is
	// removed after the state says main.7 is ready, so the check is waited
	// for to the end.
	let()
	waitDone(t, c, "ready with main.7", func(s UpdateState) bool { return s.Phase == "ready" && s.Next == "0.3.0-main.7" })
	if at, says := quit(); at == "" || says != "seven" {
		t.Errorf("with main.7 ready, quitting installs %q saying %q", at, says)
	}
	if _, err := os.Stat(first); !os.IsNotExist(err) {
		t.Errorf("the build main.7 replaced is still there: %v", err)
	}
}

// The regular looks go on while a build is ready, and the newer build
// they find takes its place without anybody clicking anything.
func TestARegularLookReplacesTheBuildReady(t *testing.T) {
	cs := newChannelServer(t)
	cs.publish(t, [3]string{"main", "0.3.0-main.5", "five"})
	c, _ := newTestUpdating(t, cs, false)
	cleanStaged(t, c)
	c.next = func(UpdateState) time.Duration { return 20 * time.Millisecond }
	_ = c.Follow("main")
	waitFor(t, c, "ready with main.5", func(s UpdateState) bool { return s.Phase == "ready" && s.Next == "0.3.0-main.5" })
	cs.publish(t, [3]string{"main", "0.3.0-main.6", "six"})
	waitFor(t, c, "ready with main.6", func(s UpdateState) bool { return s.Phase == "ready" && s.Next == "0.3.0-main.6" })
}

// A newer build that arrives while quitting is putting the build ready in
// place does not remove it: the step that puts it in place reads it once
// the app is gone.
func TestABuildBeingPutInPlaceStays(t *testing.T) {
	was := runningApp
	runningApp = func() string { return "/Applications/Frame Fairy.app" }
	t.Cleanup(func() { runningApp = was })
	cs := newChannelServer(t)
	cs.publish(t, [3]string{"main", "0.3.0-main.5", "five"})
	c, _ := newTestUpdating(t, cs, false)
	cleanStaged(t, c)
	c.install = func(string, string) error { return nil }
	_ = c.Follow("main")
	waitFor(t, c, "ready", func(s UpdateState) bool { return s.Phase == "ready" })
	first := c.staged()
	c.installOnQuit()
	cs.publish(t, [3]string{"main", "0.3.0-main.6", "six"})
	c.check()
	waitFor(t, c, "ready with main.6", func(s UpdateState) bool { return s.Phase == "ready" && s.Next == "0.3.0-main.6" })
	if _, err := os.Stat(first); err != nil {
		t.Errorf("the build going in place was removed: %v", err)
	}
	t.Cleanup(func() { _ = os.RemoveAll(filepath.Dir(first)) })
}

// A pull request followed that has left the list is closed, and the state
// says so in one place: the phase is gone whenever gone is set. Reading
// the list for the page used to set gone and leave the phase as it was,
// so after a restart a build made on the Mac showed the pull request
// closed in the list and Not checked yet under it.
func TestAChannelGoneIsSaidInOnePlace(t *testing.T) {
	cs := newChannelServer(t)
	cs.publish(t, [3]string{"main", "0.3.0-main.5", "main"}, [3]string{"pr-20", "0.3.0-pr20.9", "twenty"})
	// Pull request 138 picked in a run before, as updates.json gives it
	// back to a build from a channel after a restart. Nothing has started
	// it looking, so the only read is the page asking for the channels.
	c, _ := testUpdating(t, cs, false, "main", "pr-138")
	cleanStaged(t, c)
	c.refreshList()
	if s := c.State(); s.Gone != "pr-138" || s.Phase != "gone" {
		t.Errorf("after a restart the list says %q is gone and the phase is %q", s.Gone, s.Phase)
	}

	// A build that is ready, of a pull request that is then closed, is
	// not installed on the way out while the page says it is closed.
	was := runningApp
	runningApp = func() string { return "/Applications/Frame Fairy.app" }
	t.Cleanup(func() { runningApp = was })
	var started int
	c.install = func(string, string) error { started++; return nil }
	_ = c.Follow("pr-20")
	waitFor(t, c, "ready", func(s UpdateState) bool { return s.Phase == "ready" })
	cs.publish(t, [3]string{"main", "0.3.0-main.5", "main"})
	c.mu.Lock()
	c.listed = time.Time{}
	c.mu.Unlock()
	c.refreshList()
	s := c.State()
	if s.Gone != "pr-20" || s.Phase != "gone" || s.Next != "" {
		t.Errorf("closed while ready: %+v", s)
	}
	c.installOnQuit()
	if started != 0 {
		t.Error("a closed pull request's build went in on the way out")
	}
}

// A build made on the Mac that follows nothing, asked to look from the app
// menu, reads the list and still follows nothing. It said it was the
// newest build of main.
func TestFollowingNothingIsNeverUpToDate(t *testing.T) {
	cs := newChannelServer(t)
	cs.publish(t, [3]string{"main", "0.3.0-main.5", "main"})
	c, _ := newTestUpdating(t, cs, false)
	c.checkNow()
	if s := c.State(); s.Phase != "" || s.Follows != "" || s.Gone != "" || len(s.Channels) != 1 {
		t.Errorf("%+v", s)
	}
}

// A build made on the Mac starts out following nothing, whatever an
// earlier build picked: Tim made one with make install and found it
// following pull request 143, picked by a build before it, and offering
// a Check that would never look by itself. It does not look when it
// starts, and Check from the app menu has nothing to look for. Picking a
// channel follows it fully: it looks at once, the newest build downloads,
// and it goes on looking after that.
func TestABuildMadeOnTheMacStartsOutFollowingNothing(t *testing.T) {
	cs := newChannelServer(t)
	cs.publish(t, [3]string{"main", "0.3.0-main.5", "main"}, [3]string{"pr-143", "0.3.0-pr143.2", "pr"})
	c, _ := testUpdating(t, cs, false, "", "pr-143")
	cleanStaged(t, c)
	c.next = func(UpdateState) time.Duration { return 20 * time.Millisecond }
	c.start()
	c.check()
	c.mu.Lock()
	looking := c.looking
	c.mu.Unlock()
	if looking {
		t.Error("it started looking now and then")
	}
	if s := c.State(); s.Picked != "" || s.Follows != "" || s.Phase != "" || cs.hits.Load() != 0 {
		t.Errorf("started as %+v, the list read %d times", s, cs.hits.Load())
	}
	// The page lists the channels, and still nothing is followed.
	c.refreshList()
	if s := c.State(); s.Picked != "" || s.Follows != "" || s.Gone != "" || s.Phase != "" || len(s.Channels) != 2 {
		t.Errorf("with the list read: %+v", s)
	}

	if err := c.Follow("main"); err != nil {
		t.Fatal(err)
	}
	s := waitFor(t, c, "ready with main", func(s UpdateState) bool { return s.Phase == "ready" && s.Next == "0.3.0-main.5" })
	if s.Picked != "main" || s.Follows != "main" {
		t.Errorf("ready as %+v", s)
	}
	read := cs.hits.Load()
	deadline := time.Now().Add(10 * time.Second)
	for cs.hits.Load() < read+3 {
		if time.Now().After(deadline) {
			t.Fatalf("after the pick it looked %d times more, and stopped", cs.hits.Load()-read)
		}
		time.Sleep(10 * time.Millisecond)
	}
}

// A build from a channel goes on following what was picked in a run
// before, and looks for it as it starts.
func TestABuildFromAChannelFollowsWhatWasPicked(t *testing.T) {
	cs := newChannelServer(t)
	cs.publish(t, [3]string{"main", "0.3.0-main.5", "main"}, [3]string{"pr-20", "0.3.0-pr20.9", "twenty"})
	c, _ := testUpdating(t, cs, false, "main", "pr-20")
	cleanStaged(t, c)
	if s := c.State(); s.Picked != "pr-20" || s.Channel != "main" {
		t.Errorf("started as %+v", s)
	}
	c.start()
	s := waitFor(t, c, "ready with pr-20", func(s UpdateState) bool { return s.Phase == "ready" })
	if s.Next != "0.3.0-pr20.9" || s.Follows != "pr-20" {
		t.Errorf("%+v", s)
	}
}

// Shutting down ends the looks: the check in hand lets go of a download
// that never comes, the loop stops, and nothing looks after. A loop left
// looking every twenty milliseconds after its test read whichever later
// test's server was given the same port, and the later test counted reads
// it never made.
func TestShuttingDownEndsTheLooks(t *testing.T) {
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
	c.next = func(UpdateState) time.Duration { return 0 }
	var ends atomic.Int32
	c.ended = func() { ends.Add(1) }
	_ = c.Follow("main")
	waitFor(t, c, "downloading main", func(s UpdateState) bool { return s.Phase == "downloading" })
	done := make(chan struct{})
	go func() {
		c.shutDown()
		close(done)
	}()
	select {
	case <-done:
	case <-time.After(10 * time.Second):
		t.Fatal("shutting down waited for the download")
	}
	c.mu.Lock()
	checking := c.checking
	c.mu.Unlock()
	n := ends.Load()
	if checking || n != 1 {
		t.Errorf("after shutting down a check runs: %v, %d ended", checking, n)
	}
	// Nothing starts a check after it.
	c.check()
	_ = c.Follow("main")
	c.checkNow()
	if got := ends.Load(); got != n {
		t.Errorf("%d checks ran after shutting down", got-n)
	}
}
