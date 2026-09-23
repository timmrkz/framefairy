package main

import (
	"errors"
	"os"
	"path/filepath"
	"sync"

	"framefairy/engine"
)

// ---------------------------------------------------------------------------
// Undo and redo
//
// Everything a person does to an episode's clips can be taken back: a trim,
// a cut, the crop frame, the caption box, a word, the caption face and size,
// a clip removed, a search removed. Each is one step, and the steps of an
// episode are kept while the app is open. Jobs are not in it, because they
// make things rather than change them, and neither is moving about the
// episode or anything in the settings but the caption height, which is
// moved in the workspace like everything else here.
//
// Nothing is kept when the app closes. Everything is saved the moment it is
// done, so there is nothing to lose by closing, and a history that outlived
// the session would have to survive searches that replace what it knew.
// ---------------------------------------------------------------------------

// historyDepth is how many steps back an episode goes. Each step is a copy
// of the plans it touched, a few kilobytes, so this is generous.
const historyDepth = 200

type step struct {
	change *engine.Change
	// captionY is the caption height before and after, when the step
	// moved it. It is an app setting rather than a file of the episode.
	captionY *[2]float64
}

// history is the undo and redo of one episode.
type history struct {
	// mu is held for a whole edit, from before it to after, so two edits
	// of one episode never take their pictures of the files in between
	// each other, and an undo never lands in the middle of an edit.
	mu   sync.Mutex
	undo []step
	redo []step
}

func (s *FrameFairy) historyOf(path string) *history {
	s.mu.Lock()
	defer s.mu.Unlock()
	if s.histories == nil {
		s.histories = map[string]*history{}
	}
	h := s.histories[path]
	if h == nil {
		h = &history{}
		s.histories[path] = h
	}
	return h
}

func (s *FrameFairy) forget(path string) {
	s.mu.Lock()
	defer s.mu.Unlock()
	delete(s.histories, path)
}

// edit runs one edit of an episode and remembers what it changed. An edit
// that changed nothing is no step. One that failed part of the way, like a
// search removed from some of its plans but not all, is a step for what it
// did change.
func (s *FrameFairy) edit(path string, fn func() error) error {
	h := s.historyOf(path)
	h.mu.Lock()
	defer h.mu.Unlock()
	logs := engine.NewProject(nil, path, s.store.Settings().options()).LogsDir()
	before := engine.TakeSnapshot(logs)
	heightWas := s.store.Settings().CaptionY
	err := fn()
	change := engine.Compare(before, engine.TakeSnapshot(logs))
	heightNow := s.store.Settings().CaptionY
	st := step{change: change}
	if heightWas != heightNow {
		st.captionY = &[2]float64{heightWas, heightNow}
	}
	if change != nil || st.captionY != nil {
		h.undo = append(h.undo, st)
		if len(h.undo) > historyDepth {
			h.undo = h.undo[len(h.undo)-historyDepth:]
		}
		h.redo = nil
	}
	return err
}

// Undone is what an undo or a redo did: whether there was anything to do,
// and the clip it changed, so the window can show it.
type Undone struct {
	Done bool   `json:"done"`
	Clip string `json:"clip,omitempty"`
}

// Undo takes back the last thing done to an episode's clips.
func (s *FrameFairy) Undo(path string) (Undone, error) {
	return s.step(path, true)
}

// Redo does again what the last undo took back.
func (s *FrameFairy) Redo(path string) (Undone, error) {
	return s.step(path, false)
}

func (s *FrameFairy) step(path string, back bool) (Undone, error) {
	if !s.store.Known(path) {
		return Undone{}, os.ErrNotExist
	}
	h := s.historyOf(path)
	h.mu.Lock()
	defer h.mu.Unlock()
	from, to := &h.undo, &h.redo
	if !back {
		from, to = to, from
	}
	if len(*from) == 0 {
		return Undone{}, nil
	}
	st := (*from)[len(*from)-1]

	// Only the files of this episode, judged by where they lead, the way
	// every other call is.
	if st.change != nil {
		for _, file := range st.change.Files() {
			if !s.store.Known(file) {
				return Undone{}, os.ErrNotExist
			}
		}
	}
	if st.captionY != nil {
		want := st.captionY[1]
		if !back {
			want = st.captionY[0]
		}
		if s.store.Settings().CaptionY != want {
			return Undone{}, s.lost(h)
		}
	}
	var shown engine.ClipRef
	if st.change != nil {
		var err error
		if back {
			shown, err = st.change.Undo()
		} else {
			shown, err = st.change.Redo()
		}
		if errors.Is(err, engine.ErrChangedSince) {
			return Undone{}, s.lost(h)
		}
		if err != nil {
			return Undone{}, err
		}
	}
	if st.captionY != nil {
		set := s.store.Settings()
		set.CaptionY = st.captionY[0]
		if !back {
			set.CaptionY = st.captionY[1]
		}
		if err := s.store.SetSettings(set); err != nil {
			return Undone{}, err
		}
	}
	*from = (*from)[:len(*from)-1]
	*to = append(*to, st)
	done := Undone{Done: true}
	if shown.ID != "" {
		done.Clip = filepath.Base(shown.Plan) + "/" + shown.ID
	}
	return done, nil
}

// lost is a step that cannot be taken, because what it would put back was
// changed since by something the history does not know about, a search
// over the same part above all. Everything before it may rest on it, so
// the history of the episode starts again from here.
func (s *FrameFairy) lost(h *history) error {
	h.undo, h.redo = nil, nil
	return errors.New("this has changed since, so it cannot be taken back")
}
