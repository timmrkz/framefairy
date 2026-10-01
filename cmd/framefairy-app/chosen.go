package main

import (
	"encoding/json"
	"errors"
	"math"
	"os"
	"strings"
	"sync"

	"framefairy/engine"
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

// chosenMu keeps two changes to the same file from crossing, a clip chosen
// while the window is let go, each reading the file and writing it back.
var chosenMu sync.Mutex

func readChosen(path string) chosen {
	var held chosen
	file, err := engine.SafeChild(engine.WorkDir(path), chosenFile)
	if err != nil {
		return held
	}
	data, err := os.ReadFile(file)
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

// changeChosen reads the file, changes it and writes it back whole, through
// a file of its own, so it is never found half written.
func changeChosen(path string, change func(*chosen)) error {
	chosenMu.Lock()
	defer chosenMu.Unlock()
	held := readChosen(path)
	change(&held)
	dir := engine.WorkDir(path)
	if err := os.MkdirAll(dir, 0o755); err != nil {
		return err
	}
	file, err := engine.SafeChild(dir, chosenFile)
	if err != nil {
		return err
	}
	body, err := json.Marshal(held)
	if err != nil {
		return err
	}
	hold, err := os.CreateTemp(dir, "chosen-*.json")
	if err != nil {
		return err
	}
	tmp := hold.Name()
	if _, err := hold.Write(body); err != nil {
		hold.Close()
		os.Remove(tmp)
		return err
	}
	if err := hold.Close(); err != nil {
		os.Remove(tmp)
		return err
	}
	if err := os.Rename(tmp, file); err != nil {
		os.Remove(tmp)
		return err
	}
	return nil
}

// ChooseClip remembers which clip of an episode is being worked on, so
// opening the episode again opens on the same one. An empty key forgets it.
//
// The key names a clip set and a clip inside it. It arrives from the
// interface, so it is never joined onto a path and never used to reach a
// file: it is written down as it is and only ever compared with the keys
// the app works out for itself. Anything longer than a key could be, or
// carrying anything a key never carries, is refused rather than stored.
func (s *FrameFairy) ChooseClip(path, key string) error {
	if !s.store.Known(path) {
		return errNotInLibrary
	}
	if !looksLikeClipKey(key) {
		return errors.New("that is not the key of a clip")
	}
	return changeChosen(path, func(c *chosen) { c.Clip = key })
}

// ChosenClip gives back the clip an episode was last worked on, or an empty
// string where there is none or where what is written down is not a key.
// The interface checks it against the clips it has either way: a clip set
// that has been searched again no longer holds it.
func (s *FrameFairy) ChosenClip(path string) string {
	if !s.store.Known(path) {
		return ""
	}
	return readChosen(path).Clip
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
	was := readChosen(path).Window
	if err := changeChosen(path, func(c *chosen) { c.Window = &w }); err != nil {
		return err
	}
	if byHand && (was == nil || *was != w) {
		h.undo = append(h.undo, step{window: &[2]*KeptWindow{was, &w}})
		if len(h.undo) > historyDepth {
			h.undo = h.undo[len(h.undo)-historyDepth:]
		}
		h.redo = nil
	}
	return nil
}

// ChosenWindow gives back the window an episode was left with, or nil where
// there is none or what is written down is not a window. The interface
// checks it against the episode's length, which it knows and this does not.
func (s *FrameFairy) ChosenWindow(path string) *KeptWindow {
	if !s.store.Known(path) {
		return nil
	}
	return readChosen(path).Window
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
