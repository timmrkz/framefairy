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

// checkBuilding is how often it looks while the channel followed has a
// commit being built. A build takes a few minutes, and at the pace above
// it was found up to ten minutes after it was there. A look is one small
// file, and only a build it does not have yet is downloaded, so looking
// this often costs nothing.
const checkBuilding = 20 * time.Second

// nextCheck is how long to wait before looking again.
func nextCheck(s UpdateState) time.Duration {
	if s.Building != "" {
		return checkBuilding
	}
	return checkEvery
}

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
	// now: the one picked, or with none picked the one this build came from.
	Picked  string `json:"picked"`
	Follows string `json:"follows"`
	// Gone is the channel followed when it is not on the list any more, a
	// pull request merged or closed. Nothing is downloaded for it, and
	// nothing else is either until another channel is picked. Whenever it
	// is set, Phase is gone, see settle.
	Gone string `json:"gone"`
	// Building is the newest commit of the channel followed when its build
	// has not come yet, so the page does not offer the build there is as
	// the newest. Empty when the build is the newest.
	Building string `json:"building"`
	// Phase is "", checking, current, gone, downloading, ready or failed.
	Phase      string `json:"phase"`
	Next       string `json:"next"`
	NextName   string `json:"nextName"`
	NextCommit string `json:"nextCommit"`
	// Checked is when the last check ended, whatever it found, so the
	// interface can say a check really happened.
	Checked time.Time `json:"checked"`
	Written int64     `json:"written"`
	Total   int64     `json:"total"`
	Problem string    `json:"problem"`
}

// settle keeps the state saying one thing. A channel followed that has
// left the list is gone, whatever the last check said and whatever a
// download went on to do: the list read for the page set Gone and left the
// phase as it was, so the page called the pull request closed in the list
// and Not checked yet under it, and a build that was ready went in on the
// way out while the page said its pull request was closed. Every change to
// the state ends here, so the two can never disagree.
func (s *UpdateState) settle() {
	if s.Gone == "" {
		return
	}
	s.Phase = "gone"
	s.Next, s.NextName, s.NextCommit, s.Written, s.Total = "", "", "", 0, 0
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
	// own is the channel this build came from, empty for a build made on
	// the Mac. It never changes.
	own string

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
	// round counts picks. A check belongs to the pick it started under, and
	// once another channel is picked, whatever it goes on to find is thrown
	// away rather than shown: a download of the channel before, finishing
	// after the pick, would otherwise have the last word. stop cancels the
	// check in hand, so the new channel's check starts at once. The
	// download it waited for goes on into the cache, see updates/fetch.go,
	// and going back to that channel finds it there or on its way.
	round int
	stop  context.CancelFunc
	emit  func(UpdateState)
	busy  func() bool
	// load and save keep the picked channel, in updates.json beside the
	// settings. Not in the settings themselves: the settings screen saves
	// what it read, and would put back a channel picked since.
	load func() string
	save func(string) error
	// install starts the step that puts a ready build in place once the
	// app has quit, see install.go. relaunching is set once Relaunch has
	// begun, which puts the build in place by itself.
	install     func(target, from string) error
	relaunching bool
	// looking is set once the loop that looks now and then has started,
	// see loop. next is how long it waits between two looks.
	looking bool
	next    func(UpdateState) time.Duration
}

type updatesFile struct {
	Follow string `json:"follow"`
}

func newUpdating(u *updater.Updater, st *store, busy func() bool, emit func(UpdateState)) *updating {
	c := &updating{
		own:  buildChannel,
		busy: busy,
		emit: emit,
		load: func() string {
			var f updatesFile
			st.load("updates.json", &f)
			return f.Follow
		},
		save:    func(ch string) error { return st.save("updates.json", updatesFile{Follow: ch}) },
		install: startInstall,
	}
	exe, _ := os.Executable()
	src := &updates.Source{URL: updates.ListURL, Copies: updates.ListCopies, Client: &http.Client{}}
	// The builds already downloaded, so going back to a channel has its
	// build at once. In the user's caches, which is where macOS expects a
	// file that can always be fetched again.
	if dir, err := os.UserCacheDir(); err == nil {
		src.Cache = filepath.Join(dir, "FrameFairy", "builds")
	}
	c.setUp(u, updateKeyText, src, exe, runtime.GOOS)
	return c
}

// setUp decides whether this build can update itself at all, and if it
// can, hands Wails' updater the source and the key.
//
// A build made on the Mac starts out following nothing, whatever
// updates.json says. It is a build somebody made on purpose, not one that
// came from a channel, so the channel an earlier build followed is not
// carried over to it: Tim made one and found it following the pull request
// an earlier build had picked. updates.json stays as it is, for the builds
// from a channel that read it.
func (c *updating) setUp(u *updater.Updater, keyText string, src *updates.Source, exe, goos string) {
	c.state = UpdateState{
		Version: runningVersion(),
		Commit:  buildCommit,
		Channel: c.own,
	}
	if c.own != "" {
		c.state.Picked = c.load()
	}
	if c.next == nil {
		c.next = nextCheck
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
	src.Own = c.own
	src.Key = key
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

// start looks at once, and then now and then, for a build from a channel.
// At once, because the moment the app is opened is when a newer build is
// wanted: it used to wait five seconds first, and a person who opened the
// app to try a change sat on the Updates page waiting for it to start.
// The first answer reaches the interface when it asks, since nothing is
// on screen yet to hear the event. A build made on the Mac follows
// nothing when it starts, so there is nothing to look for, and it starts
// looking once a channel is picked, see Follow.
func (c *updating) start() {
	if c.u == nil || c.own == "" {
		return
	}
	c.loop()
}

// loop starts looking at once and then now and then, the first time it is
// called. It says whether it started, so a pick that starts it does not
// look a second time.
func (c *updating) loop() bool {
	c.mu.Lock()
	started := c.looking
	c.looking = true
	c.mu.Unlock()
	if started {
		return false
	}
	go func() {
		for {
			c.check()
			time.Sleep(c.next(c.State()))
		}
	}()
	return true
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
	c.state.settle()
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
	c.state.Follows, c.state.Gone, c.state.Building = "", "", ""
	if b, ok := l.Follow(c.state.Picked, c.own); ok {
		c.state.Follows = b.Channel
		c.state.Building = b.Newest
	} else {
		c.state.Gone = updates.Followed(c.state.Picked, c.own)
	}
	c.state.settle()
}

// progress is told about every quarter of a megabyte, and passes it on a
// few times a second, which is as often as a fill can be seen to move.
func (c *updating) progress(written, total int64) {
	c.mu.Lock()
	// A download that was let go of goes on reporting for a moment until
	// it hears, and what it says is no longer anything on screen.
	if c.state.Phase != "downloading" {
		c.mu.Unlock()
		return
	}
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

// Follow picks a channel and looks at once. The wait for a download of the
// channel before is stopped and what it found forgotten, and the state
// shown is the new channel's from this moment: a click shows at once. The
// download itself goes on, so going back has it. Following nothing is not
// a choice: a build made on the Mac starts out following nothing, and once
// a channel is picked another channel is what follows it. The first pick
// on such a build follows the channel fully, the way a build from a
// channel does: it looks at once, and then every ten minutes.
func (c *updating) Follow(channel string) error {
	if !updates.ValidChannel(channel) {
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
	c.mu.Lock()
	c.round++
	if c.stop != nil {
		c.stop()
	}
	c.mu.Unlock()
	c.change(func(s *UpdateState) {
		s.Picked = channel
		s.Phase, s.Problem = "checking", ""
		s.Next, s.NextName, s.NextCommit, s.Written, s.Total = "", "", "", 0, 0
		s.Follows, s.Gone = "", ""
		for _, ch := range s.Channels {
			if ch.ID == updates.Followed(channel, c.own) {
				s.Follows = ch.ID
			}
		}
	})
	c.picking.Unlock()
	if !c.loop() {
		go c.check()
	}
	return nil
}

// changeIn is change for a check, which only counts while no other channel
// has been picked since it began.
func (c *updating) changeIn(round int, f func(*UpdateState)) {
	c.mu.Lock()
	if c.round != round {
		c.mu.Unlock()
		return
	}
	c.mu.Unlock()
	c.change(func(s *UpdateState) {
		// Asked again under the lock, since a pick may have landed between.
		if c.round == round {
			f(s)
		}
	})
}

// checkNow is Check on the Updates page and in the app menu: it reads the
// channel list at once, so a channel made a moment ago is on the list
// whatever else is going on, and then looks for the followed channel's
// build. The list used to be read only by the check, which waited for a
// check already running, and by the page at most once a minute.
func (c *updating) checkNow() {
	if c.u == nil {
		return
	}
	c.mu.Lock()
	c.listed = time.Now()
	c.mu.Unlock()
	ctx, cancel := context.WithTimeout(context.Background(), time.Minute)
	if _, err := c.src.Fetch(ctx); err != nil {
		log.Printf("update channels: %v", err)
	} else {
		c.change(func(*UpdateState) {})
	}
	cancel()
	c.check()
}

// check looks for the followed channel's build, and downloads it when it
// is not the one running. One at a time: a check asked for while one runs
// is done when that one ends. A build following nothing has nothing to
// look for.
func (c *updating) check() {
	if c.u == nil || updates.Followed(c.picked(), c.own) == "" {
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
	ctx, cancel := context.WithTimeout(context.Background(), 30*time.Minute)
	c.mu.Lock()
	round := c.round
	c.stop = cancel
	c.mu.Unlock()
	defer func() {
		cancel()
		c.mu.Lock()
		if c.round == round {
			c.stop = nil
		}
		c.mu.Unlock()
	}()
	mine := func(f func(*UpdateState)) { c.changeIn(round, f) }

	mine(func(s *UpdateState) {
		if s.Phase != "ready" {
			s.Phase = "checking"
		}
		s.Problem = ""
	})
	rel, err := c.u.Check(ctx)
	if err != nil {
		if ctx.Err() == context.Canceled {
			return
		}
		log.Printf("update check: %v", err)
		mine(func(s *UpdateState) {
			if s.Phase != "ready" {
				s.Phase = "failed"
			}
			s.Problem = plainUpdateError(err)
			s.Checked = time.Now()
		})
		return
	}
	if rel == nil {
		mine(func(s *UpdateState) {
			s.Phase, s.Next, s.NextName, s.NextCommit, s.Written, s.Total = "current", "", "", "", 0, 0
			s.Checked = time.Now()
		})
		return
	}
	c.mu.Lock()
	have := c.round == round && c.state.Phase == "ready" && c.state.Next == rel.Version
	c.mu.Unlock()
	if have {
		mine(func(s *UpdateState) { s.Checked = time.Now() })
		return
	}
	commit, _ := rel.Metadata["commit"].(string)
	mine(func(s *UpdateState) {
		s.Phase, s.Next, s.NextName, s.NextCommit = "downloading", rel.Version, rel.Name, commit
		s.Written, s.Total = 0, rel.Artifact.Size
		s.Checked = time.Now()
	})
	if err := c.u.DownloadAndInstall(ctx); err != nil {
		// Let go of because another channel was picked: nothing went
		// wrong, the download goes on into the cache, and the next check
		// is already on its way.
		if ctx.Err() == context.Canceled {
			return
		}
		log.Printf("update download: %v", err)
		mine(func(s *UpdateState) {
			s.Phase = "failed"
			s.Problem = "The download did not arrive whole. " + plainUpdateError(err)
		})
		return
	}
	log.Printf("update ready: %s, %s", rel.Version, c.u.DownloadedPath())
	mine(func(s *UpdateState) { s.Phase = "ready" })
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
	c.mu.Lock()
	c.relaunching = true
	c.mu.Unlock()
	if err := c.u.Restart(context.Background()); err != nil {
		c.mu.Lock()
		c.relaunching = false
		c.mu.Unlock()
		return err
	}
	return nil
}

// relaunchingNow says whether Relaunch is putting a new build in place, so
// the app quits into it without asking first.
func (c *updating) relaunchingNow() bool {
	if c == nil {
		return false
	}
	c.mu.Lock()
	defer c.mu.Unlock()
	return c.relaunching
}

// installOnQuit puts a build that is ready in place once the app has
// quit, the way Chrome does, so the next start is the new build and
// nothing opens by itself in between. Nothing is done when no build is
// ready, or when Relaunch is already putting it in place.
func (c *updating) installOnQuit() {
	if c == nil || c.u == nil || c.install == nil {
		return
	}
	c.mu.Lock()
	ready := c.state.Phase == "ready" && !c.relaunching
	c.mu.Unlock()
	if !ready {
		return
	}
	staged := c.u.DownloadedPath()
	target := runningApp()
	if staged == "" || target == "" {
		return
	}
	if err := c.install(target, staged); err != nil {
		log.Printf("install on quit: %v", err)
		return
	}
	log.Printf("install on quit: %s goes in place of %s once the app has quit", staged, target)
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
