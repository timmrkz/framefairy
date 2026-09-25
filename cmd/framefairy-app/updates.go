package main

import (
	"context"
	_ "embed"
	"errors"
	"fmt"
	"log"
	"net/http"
	"os"
	"path/filepath"
	"runtime"
	"strings"
	"sync"
	"time"

	"github.com/wailsapp/wails/v3/pkg/updater"

	"framefairy/engine"
	"framefairy/updates"
)

// What the build workflow says about a build, with -ldflags -X. A build
// made by make leaves them empty: it has no channel, and its version is the
// engine's with -local after it.
var (
	buildVersion string
	buildChannel string
	buildCommit  string
)

// The public half of the update key. Every build trusts this key and no
// other. Empty until the key is made, see docs/UPDATES.md.
//
//go:embed update-key.txt
var updateKeyText string

// checkEvery is how often a build from a channel looks for a newer one. The
// channel list is a few hundred bytes, and a pull request gets a new build
// a few minutes after each push.
const checkEvery = 10 * time.Minute

func runningVersion() string {
	if buildVersion != "" {
		return buildVersion
	}
	return engine.Version + "-local"
}

// UpdateState is everything the interface shows about updates.
type UpdateState struct {
	Version string `json:"version"`
	Commit  string `json:"commit"`
	// Channel is where the running build came from, empty for one made by
	// make.
	Channel string `json:"channel"`
	// Off says why this build does not update itself, empty when it does.
	Off      string          `json:"off"`
	Channels []UpdateChannel `json:"channels"`
	// Picked is the channel picked in the app, and Follows the one followed
	// now, which is main once a picked pull request has gone.
	Picked  string `json:"picked"`
	Follows string `json:"follows"`
	// Phase is "", checking, current, downloading, ready or failed.
	Phase    string `json:"phase"`
	Next     string `json:"next"`
	NextName string `json:"nextName"`
	Written  int64  `json:"written"`
	Total    int64  `json:"total"`
	Problem  string `json:"problem"`
}

// UpdateChannel is a channel as the interface lists it.
type UpdateChannel struct {
	ID      string `json:"id"`
	Name    string `json:"name"`
	Version string `json:"version"`
}

// updating finds, fetches and installs a newer build, through Wails'
// updater and the channel list. See docs/UPDATES.md.
type updating struct {
	u   *updater.Updater
	src *updates.Source

	mu    sync.Mutex
	state UpdateState
	// again is set when a check was asked for while one ran, so the one
	// running looks once more when it is done. Picking a channel during a
	// download must not be lost.
	again    bool
	lastSent time.Time
	// listed is when the channel list was last read for the settings.
	listed time.Time

	run sync.Mutex
	// picking keeps two picks from writing updates.json at once.
	picking sync.Mutex
	emit    func(UpdateState)
	busy    func() bool
	// load and save keep the picked channel, in updates.json beside the
	// settings. Not in the settings themselves: the settings screen saves
	// what it read, and would put back a channel picked since.
	load func() string
	save func(string) error
}

type updatesFile struct {
	Follow string `json:"follow"`
}

func newUpdating(u *updater.Updater, st *store, busy func() bool, emit func(UpdateState)) *updating {
	c := &updating{
		busy: busy,
		emit: emit,
		load: func() string {
			var f updatesFile
			st.load("updates.json", &f)
			return f.Follow
		},
		save: func(ch string) error { return st.save("updates.json", updatesFile{Follow: ch}) },
	}
	exe, _ := os.Executable()
	c.setUp(u, updateKeyText, &updates.Source{URL: updates.ListURL, Client: &http.Client{}}, exe, runtime.GOOS)
	return c
}

// setUp decides whether this build can update itself at all, and if it
// can, hands Wails' updater the source and the key.
func (c *updating) setUp(u *updater.Updater, keyText string, src *updates.Source, exe, goos string) {
	c.state = UpdateState{
		Version: runningVersion(),
		Commit:  buildCommit,
		Channel: buildChannel,
		Picked:  c.load(),
	}
	key, err := updates.PublicKey(keyText)
	switch {
	case err != nil:
		c.state.Off = "The update key built into this app is not a key."
	case key == nil:
		c.state.Off = "This build has no update key yet, so it cannot tell a build of ours from anybody else's."
	case goos != "darwin":
		c.state.Off = "Only the Mac app updates itself, for now."
	case !strings.Contains(filepath.ToSlash(exe), ".app/"):
		c.state.Off = "Only the app updates itself. This is the program on its own, not Frame Fairy.app."
	}
	if c.state.Off != "" {
		return
	}
	src.Own = buildChannel
	src.Picked = c.picked
	src.Seen = c.seen
	src.Progress = c.progress
	err = u.Init(updater.Config{
		CurrentVersion: c.state.Version,
		Providers:      []updater.Provider{src},
		PublicKey:      key,
		Window:         updater.WindowNone,
	})
	if err != nil {
		c.state.Off = "The updater did not start: " + err.Error()
		return
	}
	c.u, c.src = u, src
}

// start looks now and then, for a build from a channel. A build made by make
// only looks when asked: it is somebody working on the app, and it would
// otherwise fetch a build to replace itself with every time it started.
func (c *updating) start() {
	if c.u == nil || buildChannel == "" {
		return
	}
	go func() {
		time.Sleep(5 * time.Second)
		for {
			c.check()
			time.Sleep(checkEvery)
		}
	}()
}

func (c *updating) State() UpdateState {
	c.mu.Lock()
	defer c.mu.Unlock()
	s := c.state
	s.Channels = append([]UpdateChannel(nil), c.state.Channels...)
	return s
}

func (c *updating) change(f func(*UpdateState)) {
	c.mu.Lock()
	f(&c.state)
	c.lastSent = time.Now()
	c.mu.Unlock()
	if c.emit != nil {
		c.emit(c.State())
	}
}

func (c *updating) picked() string {
	c.mu.Lock()
	defer c.mu.Unlock()
	return c.state.Picked
}

func (c *updating) seen(l updates.List) {
	var chans []UpdateChannel
	for _, b := range l.Channels {
		chans = append(chans, UpdateChannel{ID: b.Channel, Name: b.Name, Version: b.Version})
	}
	c.mu.Lock()
	defer c.mu.Unlock()
	c.state.Channels = chans
	if b, ok := l.Follow(c.state.Picked, buildChannel); ok {
		c.state.Follows = b.Channel
	} else {
		c.state.Follows = ""
	}
}

// progress is told about every quarter of a megabyte, and passes it on a
// few times a second, which is as often as a fill can be seen to move.
func (c *updating) progress(written, total int64) {
	c.mu.Lock()
	c.state.Written, c.state.Total = written, total
	due := time.Since(c.lastSent) > 200*time.Millisecond || written == total
	c.mu.Unlock()
	if due {
		c.change(func(*UpdateState) {})
	}
}

// refreshList reads the channel list, so the settings can offer the
// channels to a build that has not looked yet. At most once a minute.
func (c *updating) refreshList() {
	if c.u == nil {
		return
	}
	c.mu.Lock()
	due := time.Since(c.listed) > time.Minute
	if due {
		c.listed = time.Now()
	}
	c.mu.Unlock()
	if !due {
		return
	}
	if _, err := c.src.Fetch(context.Background()); err != nil {
		log.Printf("update channels: %v", err)
		return
	}
	c.change(func(*UpdateState) {})
}

// Follow picks a channel and looks at once.
func (c *updating) Follow(channel string) error {
	if channel != "" && !updates.ValidChannel(channel) {
		return fmt.Errorf("%q is not a channel", channel)
	}
	if c.u == nil {
		return errors.New(c.state.Off)
	}
	c.picking.Lock()
	if err := c.save(channel); err != nil {
		c.picking.Unlock()
		return err
	}
	c.change(func(s *UpdateState) { s.Picked = channel })
	c.picking.Unlock()
	go c.check()
	return nil
}

// check looks for the followed channel's build, and downloads it when it
// is not the one running. One at a time: a check asked for while one runs
// is done when that one ends.
func (c *updating) check() {
	if c.u == nil {
		return
	}
	if !c.run.TryLock() {
		c.mu.Lock()
		c.again = true
		c.mu.Unlock()
		return
	}
	defer c.run.Unlock()
	for {
		c.checkOnce()
		c.mu.Lock()
		again := c.again
		c.again = false
		c.mu.Unlock()
		if !again {
			return
		}
	}
}

func (c *updating) checkOnce() {
	c.change(func(s *UpdateState) {
		if s.Phase != "ready" {
			s.Phase = "checking"
		}
		s.Problem = ""
	})
	ctx, cancel := context.WithTimeout(context.Background(), 30*time.Minute)
	defer cancel()
	rel, err := c.u.Check(ctx)
	if err != nil {
		log.Printf("update check: %v", err)
		c.change(func(s *UpdateState) {
			if s.Phase != "ready" {
				s.Phase = "failed"
			}
			s.Problem = "The check did not get through. " + plainUpdateError(err)
		})
		return
	}
	if rel == nil {
		c.change(func(s *UpdateState) {
			s.Phase, s.Next, s.NextName, s.Written, s.Total = "current", "", "", 0, 0
		})
		return
	}
	c.mu.Lock()
	have := c.state.Phase == "ready" && c.state.Next == rel.Version
	c.mu.Unlock()
	if have {
		return
	}
	c.change(func(s *UpdateState) {
		s.Phase, s.Next, s.NextName = "downloading", rel.Version, rel.Name
		s.Written, s.Total = 0, rel.Artifact.Size
	})
	if err := c.u.DownloadAndInstall(ctx); err != nil {
		log.Printf("update download: %v", err)
		c.change(func(s *UpdateState) {
			s.Phase = "failed"
			s.Problem = "The build did not arrive whole. " + plainUpdateError(err)
		})
		return
	}
	log.Printf("update ready: %s, %s", rel.Version, c.u.DownloadedPath())
	c.change(func(s *UpdateState) { s.Phase = "ready" })
}

// Restart quits into the build that is ready. Never while work runs: the
// helper that swaps the app waits for this one to end, and a search or a
// render would be stopped half way.
func (c *updating) Restart() error {
	if c.u == nil {
		return errors.New(c.state.Off)
	}
	c.mu.Lock()
	ready := c.state.Phase == "ready"
	c.mu.Unlock()
	if !ready {
		return errors.New("no update is ready")
	}
	if c.busy != nil && c.busy() {
		return errors.New("work is still running. Restart when it is done")
	}
	return c.u.Restart(context.Background())
}

// plainUpdateError takes the updater's prefixes off an error, which say
// where in the updater it happened and nothing a person can act on.
func plainUpdateError(err error) string {
	msg := err.Error()
	for _, p := range []string{"updater: all providers failed: ", "updater: ", "channels: "} {
		msg = strings.ReplaceAll(msg, p, "")
	}
	msg = strings.TrimSuffix(msg, ".")
	if msg == "" {
		return ""
	}
	return strings.ToUpper(msg[:1]) + msg[1:] + "."
}
