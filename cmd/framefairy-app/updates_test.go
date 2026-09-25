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
	hits atomic.Int32
}

func newChannelServer(t *testing.T) *channelServer {
	t.Helper()
	_, key, err := ed25519.GenerateKey(rand.Reader)
	if err != nil {
		t.Fatal(err)
	}
	cs := &channelServer{key: key, zips: map[string][]byte{}}
	cs.srv = httptest.NewTLSServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		cs.mu.Lock()
		list, data, ok := cs.list, cs.zips[r.URL.Path], false
		_, ok = cs.zips[r.URL.Path]
		slow := cs.slow
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
		l.Channels = append(l.Channels, updates.Build{
			Channel: channel, Name: "#" + channel, Version: version, Commit: "abc1234",
			URL: cs.srv.URL + path, Size: size, SHA256: hex.EncodeToString(digest),
			Signature: updates.Sign(cs.key, digest), Published: time.Now(),
		})
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

// An updating as the app makes it, reading from the server above.
func newTestUpdating(t *testing.T, cs *channelServer, busy bool) (*updating, *[]UpdateState) {
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
		load: func() string { return picked },
		save: func(ch string) error { picked = ch; return nil },
	}
	src := &updates.Source{URL: cs.srv.URL + "/channels.json", Client: cs.srv.Client()}
	c.setUp(updater.New(quietHost{}), cs.publicKey(), src, inApp, "darwin")
	if c.state.Off != "" {
		t.Fatalf("off: %s", c.state.Off)
	}
	return c, &sent
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
	if s.Next != "0.3.0-pr20.9" || s.Follows != "pr-20" || s.Picked != "pr-20" || len(s.Channels) != 2 {
		t.Errorf("ready as %+v", s)
	}
	if s.Written != s.Total || s.Total == 0 {
		t.Errorf("the fill ended at %d of %d", s.Written, s.Total)
	}
	var phases []string
	for _, st := range *sent {
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
// it is offered main.
func TestAMergedPullRequestFollowsMain(t *testing.T) {
	cs := newChannelServer(t)
	cs.publish(t, [3]string{"main", "0.3.0-main.5", "main"})
	c, _ := newTestUpdating(t, cs, false)
	cleanStaged(t, c)
	_ = c.Follow("pr-18")
	s := waitFor(t, c, "ready", func(s UpdateState) bool { return s.Phase == "ready" })
	if s.Follows != "main" || s.Picked != "pr-18" || s.Next != "0.3.0-main.5" {
		t.Errorf("%+v", s)
	}
}

// The running build is the channel's build, so there is nothing to fetch.
func TestTheRunningBuildIsCurrent(t *testing.T) {
	cs := newChannelServer(t)
	cs.publish(t, [3]string{"main", runningVersion(), "main"})
	c, _ := newTestUpdating(t, cs, false)
	c.check()
	if s := c.State(); s.Phase != "current" || s.Next != "" {
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
	if s.Phase != "failed" || !strings.HasPrefix(s.Problem, "The check did not get through. The channel list is not readable") {
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
	c.check()
	if c.State().Phase != "ready" {
		t.Fatalf("%+v", c.State())
	}
	if err := c.Restart(); err == nil || !strings.Contains(err.Error(), "still running") {
		t.Errorf("restarted while work ran: %v", err)
	}
}

// A channel picked while a download runs is not lost: the check that is
// running looks again when it is done, and fetches the new pick.
func TestAPickDuringADownloadIsFetchedAfter(t *testing.T) {
	cs := newChannelServer(t)
	cs.publish(t, [3]string{"main", "0.3.0-main.5", "main"}, [3]string{"pr-20", "0.3.0-pr20.9", "twenty"})
	cs.slow = make(chan struct{})
	c, _ := newTestUpdating(t, cs, false)
	cleanStaged(t, c)
	_ = c.Follow("main")
	waitFor(t, c, "downloading", func(s UpdateState) bool { return s.Phase == "downloading" })
	_ = c.Follow("pr-20")
	close(cs.slow)
	s := waitFor(t, c, "ready with pr-20", func(s UpdateState) bool { return s.Phase == "ready" && s.Next == "0.3.0-pr20.9" })
	if s.Follows != "pr-20" {
		t.Errorf("%+v", s)
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
	if len(s.Channels) != 2 || s.Phase != "" || s.Follows != "main" {
		t.Errorf("%+v", s)
	}
	if n := cs.hits.Load(); n != 1 {
		t.Errorf("read the list %d times", n)
	}
}
