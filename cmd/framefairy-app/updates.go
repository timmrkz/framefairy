package main

import (
	"context"
	_ "embed"
	"errors"
	"fmt"
	"io"
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
	// nothing else is either until another channel is picked.
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

// UpdateChannel is a channel as the interface lists it.
type UpdateChannel struct {
	ID      string `json:"id"`
	Name    string `json:"name"`
	Version string `json:"version"`
}

// updating finds, fetches and installs a newer build, through Wails'
// updater and the channel list. See docs/UPDATES.md.
type updating struct {
	// u is the updater that looks. Every build is downloaded by an updater
	// of its own, see download, and the one whose build is ready is held.
	u   *updater.Updater
	src *updates.Source
	// cfg is how every updater here is set up, and fresh makes one for a
	// download. Restart quits the app through the updater that holds the
	// build, so in the app it is the app's.
	cfg   updater.Config
	fresh func() *updater.Updater
	// every is how long the regular look waits, nextCheck but in tests.
	// looping starts it once.
	every   func(UpdateState) time.Duration
	looping sync.Once

	mu    sync.Mutex
	state UpdateState
	// held is the updater whose build is ready, nil when none is. It is
	// replaced only once a newer build has fully arrived and passed its
	// checks, so quitting always has the newest build that is whole.
	held *updater.Updater
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
	// leaving is set once quitting has started putting the build held in
	// place. From then on, as after Relaunch, nothing removes that build,
	// whatever arrives after it: the step that puts it in place reads it
	// once the app is gone.
	leaving bool
}

type updatesFile struct {
	Follow string `json:"follow"`
}

func newUpdating(u *updater.Updater, quit func(), st *store, busy func() bool, emit func(UpdateState)) *updating {
	c := &updating{
		busy: busy,
		emit: emit,
		// Every build is downloaded by an updater of its own, and Relaunch
		// quits the app through the one that holds it.
		fresh: func() *updater.Updater { return updater.New(quitHost{quit: quit}) },
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

// quitHost is what an updater of one download is attached to. The app
// shows updates through its own events, so the updater's go nowhere, and
// its window is never asked for. Restart quits the app through it.
type quitHost struct{ quit func() }

func (quitHost) Emit(string, ...any) bool                              { return false }
func (quitHost) OnEvent(string, func(any)) func()                      { return func() {} }
func (quitHost) OpenWindow(updater.WindowOptions) updater.WindowHandle { return nil }
func (h quitHost) Quit() {
	if h.quit != nil {
		h.quit()
	}
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
	src.Key = key
	src.Picked = c.picked
	src.Seen = c.seen
	src.Progress = c.progress
	c.cfg = updater.Config{
		CurrentVersion: c.state.Version,
		Providers:      []updater.Provider{src},
		PublicKey:      key,
		Window:         updater.WindowNone,
	}
	if c.fresh == nil {
		c.fresh = func() *updater.Updater { return updater.New(quitHost{}) }
	}
	if c.every == nil {
		c.every = nextCheck
	}
	err = u.Init(c.cfg)
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
// on screen yet to hear the event. A build made by make does not look when
// it starts: it is somebody working on the app, and it would otherwise
// fetch a build to replace itself with every time it started. It looks
// regularly from the moment a channel is picked in it, see Follow.
//
// The looks go on while a build is ready, so a newer commit replaces the
// build waiting for a relaunch, see checkOnce.
func (c *updating) start() {
	if c.u == nil || buildChannel == "" {
		return
	}
	c.loop(true)
}

// loop looks now and then from here on, once whoever starts it first.
func (c *updating) loop(now bool) {
	c.looping.Do(func() {
		go func() {
			if !now {
				time.Sleep(c.every(c.State()))
			}
			for {
				c.check()
				time.Sleep(c.every(c.State()))
			}
		}()
	})
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
	c.state.Follows, c.state.Gone, c.state.Building = "", "", ""
	if b, ok := l.Follow(c.state.Picked, buildChannel); ok {
		c.state.Follows = b.Channel
		c.state.Building = b.Newest
	} else {
		c.state.Gone = updates.Followed(c.state.Picked, buildChannel)
	}
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
// download itself goes on, so going back has it.
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
			if ch.ID == updates.Followed(channel, buildChannel) {
				s.Follows = ch.ID
			}
		}
	})
	c.picking.Unlock()
	go c.check()
	// A build made by make looked only when a channel was picked, so once
	// a build was ready it never heard of a newer one. Following a channel
	// is asking for its newest build, so from here on it looks as a build
	// from a channel does.
	c.loop(false)
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
		c.drop(round)
		mine(func(s *UpdateState) {
			s.Phase, s.Next, s.NextName, s.NextCommit, s.Written, s.Total = "current", "", "", "", 0, 0
			if s.Gone != "" {
				s.Phase = "gone"
			}
			s.Checked = time.Now()
		})
		return
	}
	c.mu.Lock()
	mineNow := c.round == round
	have := mineNow && c.state.Phase == "ready" && c.state.Next == rel.Version
	// A build of this channel is ready and a newer one has come. The one
	// that is ready stays ready, on the page and on the way out, until the
	// newer one is whole: Tim's rule is that the app always holds the
	// newest build of its channel, and never none while one is on its way.
	swap := mineNow && c.state.Phase == "ready"
	c.mu.Unlock()
	if have {
		mine(func(s *UpdateState) { s.Checked = time.Now() })
		return
	}
	commit, _ := rel.Metadata["commit"].(string)
	mine(func(s *UpdateState) {
		if !swap {
			s.Phase, s.Next, s.NextName, s.NextCommit = "downloading", rel.Version, rel.Name, commit
			s.Written, s.Total = 0, rel.Artifact.Size
		}
		s.Checked = time.Now()
	})
	w, err := c.download(ctx, rel)
	if err != nil {
		// Let go of because another channel was picked: nothing went
		// wrong, the download goes on into the cache, and the next check
		// is already on its way.
		if ctx.Err() == context.Canceled {
			return
		}
		log.Printf("update download: %v", err)
		// The newer build did not come whole, and the one that is ready
		// still is. The next look tries again.
		if swap {
			return
		}
		c.drop(round)
		mine(func(s *UpdateState) {
			s.Phase = "failed"
			s.Problem = "The download did not arrive whole. " + plainUpdateError(err)
		})
		return
	}
	// The new build is whole and checked: it is the one held from now on,
	// and the one it replaces is removed, in one step with the state, so
	// the page and the way out never name two different builds. Unless
	// that one is already being put in place by Relaunch or on the way
	// out: then it stays where it is until it has gone in.
	var gone *updater.Updater
	c.change(func(s *UpdateState) {
		if c.round != round {
			gone = w
			return
		}
		gone, c.held = c.held, w
		if c.relaunching || c.leaving {
			gone = nil
		}
		s.Phase, s.Next, s.NextName, s.NextCommit = "ready", rel.Version, rel.Name, commit
		s.Written, s.Total = rel.Artifact.Size, rel.Artifact.Size
	})
	unstage(gone)
	if gone != w {
		log.Printf("update ready: %s, %s", rel.Version, w.DownloadedPath())
	}
}

// download fetches, checks and unpacks a build with an updater of its own,
// the way every build is fetched. Wails' updater throws away the build it
// holds before it downloads another, so a build that is ready would be
// gone for as long as a newer one downloads, and for good if that one
// failed. With an updater per build, the one that is ready is not touched
// until its replacement has passed every check the first one did.
func (c *updating) download(ctx context.Context, rel *updater.Release) (*updater.Updater, error) {
	w := c.fresh()
	cfg := c.cfg
	r := *rel
	cfg.Providers = []updater.Provider{offer{src: c.src, rel: &r}}
	if err := w.Init(cfg); err != nil {
		return nil, err
	}
	if _, err := w.Check(ctx); err != nil {
		return nil, err
	}
	if err := w.DownloadAndInstall(ctx); err != nil {
		return nil, err
	}
	return w, nil
}

// offer is what an updater of one build reads from: the build the check
// found, its claim already checked, downloaded through the channel list's
// source, cache and all.
type offer struct {
	src *updates.Source
	rel *updater.Release
}

func (o offer) Name() string { return o.src.Name() }

func (o offer) Check(context.Context, updater.CheckRequest) (*updater.Release, error) {
	return o.rel, nil
}

func (o offer) Download(ctx context.Context, r *updater.Release, dst io.Writer, onProgress func(written, total int64)) error {
	return o.src.Download(ctx, r, dst, onProgress)
}

// drop lets go of the build held, when a check of this round finds there
// is nothing of its channel to install: it is current, gone, or failed.
func (c *updating) drop(round int) {
	c.mu.Lock()
	var gone *updater.Updater
	if c.round == round {
		gone, c.held = c.held, nil
	}
	if c.relaunching || c.leaving {
		gone = nil
	}
	c.mu.Unlock()
	unstage(gone)
}

// staged is where the build that is ready was unpacked, or empty.
func (c *updating) staged() string {
	c.mu.Lock()
	defer c.mu.Unlock()
	if c.held == nil {
		return ""
	}
	return c.held.DownloadedPath()
}

// unstage removes what an updater unpacked and nobody will install. Only a
// folder of Wails' own in the temporary folder, the one it unpacked into.
func unstage(w *updater.Updater) {
	if w == nil {
		return
	}
	p := w.DownloadedPath()
	if p == "" {
		return
	}
	dir := filepath.Dir(p)
	if !strings.HasPrefix(filepath.Base(dir), "wails-update-") || filepath.Dir(dir) != filepath.Clean(os.TempDir()) {
		return
	}
	_ = os.RemoveAll(dir)
}

// Restart quits into the build that is ready. Never while work runs: the
// helper that swaps the app waits for this one to end, and a search or a
// render would be stopped half way.
func (c *updating) Restart() error {
	if c.u == nil {
		return errors.New(c.state.Off)
	}
	c.mu.Lock()
	ready := c.state.Phase == "ready" && c.held != nil
	c.mu.Unlock()
	if !ready {
		return errors.New("no update is ready")
	}
	if c.busy != nil && c.busy() {
		return errors.New("work is still running. Restart when it is done")
	}
	// The build held now is the one Relaunch puts in place, and nothing
	// replaces it from here on.
	c.mu.Lock()
	held := c.held
	ready = c.state.Phase == "ready" && held != nil
	c.relaunching = ready
	c.mu.Unlock()
	if !ready {
		return errors.New("no update is ready")
	}
	if err := held.Restart(context.Background()); err != nil {
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
	ready := c.state.Phase == "ready" && !c.relaunching && c.held != nil
	staged := ""
	if ready {
		staged = c.held.DownloadedPath()
		c.leaving = true
	}
	c.mu.Unlock()
	if !ready {
		return
	}
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
