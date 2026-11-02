package main

import (
	"context"
	"encoding/json"
	"errors"
	"math"
	"sort"
	"strings"
	"sync"
	"time"

	"github.com/spf13/afero"

	"framefairy/engine"
	"framefairy/engine/fileio"
)

// chosenFile is where an episode keeps what was last chosen in it, the
// clip worked on and the window on the range picker. It is about that
// episode and nothing else, so it sits in the episode's own folder and
// goes when the folder goes.
const chosenFile = "chosen.json"

// chosen is the whole of that file. A struct rather than a bare string so
// a later version can keep more without the older one choking on it, which
// is how the window came to be kept beside the clip.
type chosen struct {
	Clip   string      `json:"clip"`
	Window *KeptWindow `json:"window,omitempty"`
	// Opened is when the episode was last put on screen by a person, in
	// Unix milliseconds, see OpenEpisode.
	Opened int64 `json:"opened,omitempty"`
}

// KeptWindow is the window as it was left: where it starts and ends, and
// how long it was made, which is longer than it is when the end of the
// episode cut it short.
type KeptWindow struct {
	From   float64 `json:"from"`
	To     float64 `json:"to"`
	Length float64 `json:"length"`
}

// ok says whether a window could be one: numbers, in order, inside the
// longest episode there is. It arrives from the interface, and it is read
// back from a file anything could have written.
func (w KeptWindow) ok() bool {
	for _, v := range []float64{w.From, w.To, w.Length} {
		if math.IsNaN(v) || math.IsInf(v, 0) || v < 0 || v > engine.MaxEpisodeSeconds {
			return false
		}
	}
	return w.To > w.From && w.Length >= w.To-w.From-0.001
}

func readChosen(files afero.Fs, path string) chosen {
	var held chosen
	file, err := fileio.SafeChildIn(files, engine.WorkDir(path), chosenFile)
	if err != nil {
		return held
	}
	data, err := afero.ReadFile(files, file)
	if err != nil || json.Unmarshal(data, &held) != nil {
		return chosen{}
	}
	if !looksLikeClipKey(held.Clip) {
		held.Clip = ""
	}
	if held.Window != nil && !held.Window.ok() {
		held.Window = nil
	}
	return held
}

// writeChosen writes the file whole, through a file of its own, so it is
// never found half written.
func writeChosen(files afero.Fs, path string, held chosen) error {
	dir := engine.WorkDir(path)
	if err := files.MkdirAll(dir, 0o755); err != nil {
		return err
	}
	file, err := fileio.SafeChildIn(files, dir, chosenFile)
	if err != nil {
		return err
	}
	body, err := json.Marshal(held)
	if err != nil {
		return err
	}
	return fileio.WriteAtomic(files, file, body)
}

// picked is what is chosen in a video while the app runs: the clip worked
// on and the window, kept in memory beside the video's undo steps, see
// history, and written through to chosen.json, which is read only the
// first time, so the app opens on them again after a restart. A state of
// the video reads them here, never from the file. Its lock is its own,
// held only to read or change what is kept, never for a read of the disk
// and never for an edit, which holds the history's lock for as long as it
// writes the plans. The file is written after the lock is let go, the
// newest version only, so what is on the disk is never older than what
// was written before it.
type picked struct {
	mu      sync.Mutex
	loaded  bool
	kept    chosen
	version uint64
	// writing is held while the file is written, and written the version
	// last written.
	writing sync.Mutex
	written uint64
	// What the app chooses a clip by, see choose.go: whether the video
	// plays, the search it follows, the clip each removal chose instead of
	// the one it removed, whether a state was read yet and the rev of the
	// newest state it chose from.
	playing bool
	search  *followed
	instead map[string]string
	opened  bool
	decided uint64
}

// chosenIn is what is chosen in the video at path, see picked.
func (s *FrameFairy) chosenIn(path string) *picked {
	return &s.historyOf(path).picked
}

// get gives back what is kept, read from the file the first time.
func (p *picked) get(files afero.Fs, path string) chosen {
	p.load(files, path)
	p.mu.Lock()
	defer p.mu.Unlock()
	held := p.kept
	if held.Window != nil {
		w := *held.Window
		held.Window = &w
	}
	return held
}

// load reads the file once, before the lock is taken.
func (p *picked) load(files afero.Fs, path string) {
	p.mu.Lock()
	loaded := p.loaded
	p.mu.Unlock()
	if loaded {
		return
	}
	held := readChosen(files, path)
	p.mu.Lock()
	if !p.loaded {
		p.kept, p.loaded = held, true
	}
	p.mu.Unlock()
}

// change changes what is kept, when change says it changed it, and writes
// it through. It says whether it changed.
func (p *picked) change(files afero.Fs, path string, change func(*chosen) bool) (bool, error) {
	return p.with(files, path, func(p *picked) bool { return change(&p.kept) })
}

// with runs change with the lock held, on everything picked keeps, and
// writes what is kept through when change says it changed it.
func (p *picked) with(files afero.Fs, path string, change func(*picked) bool) (bool, error) {
	p.load(files, path)
	p.mu.Lock()
	if !change(p) {
		p.mu.Unlock()
		return false, nil
	}
	p.version++
	held, version := p.kept, p.version
	p.mu.Unlock()
	p.writing.Lock()
	defer p.writing.Unlock()
	if version <= p.written {
		return true, nil
	}
	p.written = version
	return true, writeChosen(files, path, held)
}

// ChooseClip remembers which clip of a video a hand chose, so the state
// of the video says it and opening it again opens on the same one. An
// empty key chooses none. Every clip the app chooses by itself it chooses
// in choose.go, so a clip chosen here is always a hand's. It answers with
// a number from the clock of the states: a state read before the pick
// has a smaller rev, so the workspace keeps the pick on screen until a
// state past it comes, see EpisodeSession.pick.
//
// The key names a clip set and a clip inside it. It arrives from the
// interface, so it is never joined onto a path and never used to reach a
// file: it is written down as it is and only ever compared with the keys
// the app works out for itself. Anything longer than a key could be, or
// carrying anything a key never carries, is refused rather than stored.
func (s *FrameFairy) ChooseClip(path, key string) (uint64, error) {
	if !s.store.Known(path) {
		return 0, errNotInLibrary
	}
	if !looksLikeClipKey(key) {
		return 0, errors.New("that is not the key of a clip")
	}
	if _, err := s.chosenIn(path).change(s.store.files, path, func(c *chosen) bool {
		c.Clip = key
		return true
	}); err != nil {
		return 0, err
	}
	rev := s.stamp()
	s.told(path)
	return rev, nil
}

// ChooseWindow remembers the window on an episode's range picker as it was
// left, so the app opens on it again after a restart rather than on one of
// its own choosing. length is how long the window was made. A window moved
// by a hand, dragged or put back with a double-click, is a step that undo
// takes back. One the app moved by itself, after a search, is not.
func (s *FrameFairy) ChooseWindow(path string, from, to, length float64, byHand bool) error {
	if !s.store.Known(path) {
		return errNotInLibrary
	}
	w := KeptWindow{From: from, To: to, Length: length}
	if !w.ok() {
		return errors.New("that is not a window")
	}
	h := s.historyOf(path)
	h.mu.Lock()
	defer h.mu.Unlock()
	var was *KeptWindow
	if _, err := h.picked.change(s.store.files, path, func(c *chosen) bool {
		was, c.Window = c.Window, &w
		return true
	}); err != nil {
		return err
	}
	if byHand && (was == nil || *was != w) {
		h.undo = append(h.undo, step{window: &[2]*KeptWindow{was, &w}})
		if len(h.undo) > historyDepth {
			h.undo = h.undo[len(h.undo)-historyDepth:]
		}
		h.redo = nil
	}
	s.markAside(path)
	return nil
}

// ChosenWindow gives back the window of a video, the one the workspace
// draws: as it was left, after a restart too, and where there is none, or
// what is written down is not a window of this video, the one the app
// places, see videoWindow. nil while the video cannot be read.
func (s *FrameFairy) ChosenWindow(path string) *KeptWindow {
	if !s.store.Known(path) {
		return nil
	}
	w, ok := s.videoWindow(context.Background(), path)
	if !ok {
		return nil
	}
	return &w
}

// looksLikeClipKey reports whether a string could be the key of a clip: a
// clip set name, a slash, and the clip's own name. An empty key is allowed
// and means no clip.
func looksLikeClipKey(key string) bool {
	if key == "" {
		return true
	}
	if len(key) > 300 || strings.Count(key, "/") != 1 {
		return false
	}
	for _, r := range key {
		if r < 0x20 || r == 0x7f {
			return false
		}
	}
	name, id, _ := strings.Cut(key, "/")
	return name != "" && id != ""
}

// OpenEpisode notes that the episode at path was put on screen, so the app
// opens on it again as it starts, and builds the workspaces of those opened
// before it behind it, see Opened.
func (s *FrameFairy) OpenEpisode(path string) error {
	if !s.store.Known(path) {
		return errNotInLibrary
	}
	now := time.Now().UnixMilli()
	_, err := s.chosenIn(path).change(s.store.files, path, func(c *chosen) bool {
		c.Opened = max(now, c.Opened)
		return true
	})
	return err
}

// Opened lists the episodes of the library opened last, newest first, at
// most n of them. The app opens the first as it starts, the episode left
// open, and builds the workspaces of the rest behind it, so the first ones
// picked are on screen in one frame, see App.svelte. An episode never
// opened is not listed. One opened only before OpenEpisode was there comes
// after every one opened since, in the order its chosen.json was last
// written, because opening an episode chose a clip on it. That file is also
// written by a workspace built at start, which is not a person opening it.
func (s *FrameFairy) Opened(n int) []string {
	type seen struct {
		path   string
		marked bool
		at     int64
	}
	// A video deleted outside the app is let go first, so the app never
	// opens on it as it starts: Tim deleted two videos with their folders,
	// an earlier version wrote the chosen clip back into one, and the app
	// opened on it after an update and said it was not in the library.
	s.letGoOfRemoved()
	var all []seen
	for _, ep := range s.store.Episodes() {
		file, err := fileio.SafeChildIn(s.store.files, engine.WorkDir(ep), chosenFile)
		if err != nil {
			continue
		}
		info, err := s.store.files.Stat(file)
		if err != nil || !info.Mode().IsRegular() {
			continue
		}
		if at := s.chosenIn(ep).get(s.store.files, ep).Opened; at > 0 {
			all = append(all, seen{ep, true, at})
		} else {
			all = append(all, seen{ep, false, info.ModTime().UnixMilli()})
		}
	}
	sort.SliceStable(all, func(a, b int) bool {
		if all[a].marked != all[b].marked {
			return all[a].marked
		}
		return all[a].at > all[b].at
	})
	out := []string{}
	for _, e := range all[:min(max(n, 0), len(all))] {
		out = append(out, e.path)
	}
	return out
}
