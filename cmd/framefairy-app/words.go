package main

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"path/filepath"
	"strings"
	"sync"
	"time"

	"github.com/rs/zerolog"
	"github.com/spf13/afero"

	"framefairy/engine"
)

// transcript is the whole-episode transcript, read once and kept, for the
// calls the timeline makes over and over as it is moved. A four hour
// transcript is megabytes of words and levels, and reading it for every
// swipe made the timeline crawl and the waveform arrive in steps.
//
// project is the episode at path with the settings as they are now, for
// what reads or edits its files without running a step.
func (s *FrameFairy) project(path string) *engine.Project {
	return s.projectWith(path, s.store.Settings().options())
}

// projectWith is project with the settings given.
func (s *FrameFairy) projectWith(path string, opts engine.Options) *engine.Project {
	p := engine.NewProject(nil, path, opts)
	p.Files = s.store.files
	return p
}

// The transcripts of the last keptTranscripts episodes are kept, as many as
// the app keeps workspaces of, so going back to one reads nothing. One was
// kept, so every episode picked read its transcript again. And it is read
// once however many ask at once: an episode opening asks for its words, its
// waveform, its captions and its room together, and each read the whole
// file for itself, up to seven times.
//
// What it hands out is shared, so only what reads goes through here.
func (s *FrameFairy) transcript(p *engine.Project) (*engine.Transcript, error) {
	stamp := ""
	for _, file := range engine.TranscriptFiles(p.LogsDir()) {
		info, err := p.Files.Stat(file)
		if err != nil {
			stamp += "|-"
			continue
		}
		stamp += fmt.Sprintf("|%d,%d", info.Size(), info.ModTime().UnixNano())
	}
	return s.said.get(p.Source, stamp, func() (*engine.Transcript, error) {
		s.oldCorrections(p)
		return p.Transcript()
	})
}

// oldCorrections says once, at info, that a video still has the
// corrections.json a version before 2.249 kept for the whole video, and
// how many words it held. Corrections and removed words are a clip's now,
// and the file is left where it is and unread, so a report of a fix gone
// after the update explains itself. It never says what the words read.
func (s *FrameFairy) oldCorrections(p *engine.Project) {
	file := filepath.Join(p.LogsDir(), "corrections.json")
	data, err := afero.ReadFile(p.Files, file)
	if err != nil {
		return
	}
	if _, told := s.toldOld.LoadOrStore(file, true); told {
		return
	}
	var words map[string]any
	_ = json.Unmarshal(data, &words)
	theLog.line(zerolog.InfoLevel, "edits").Str("video", filepath.Base(p.Source)).Int("words", len(words)).
		Msg("the corrections of the whole video from before are left unread")
}

// wordCounts adds to the line of an edit of a clip's words how many words
// the clip corrects and how many it removes once the edit is made, never
// which, so a word come back or a fix gone is read from the log alone.
func (s *FrameFairy) wordCounts(line *zerolog.Event, plan, clipID string) *zerolog.Event {
	_, clips, err := engine.LoadClips(s.store.files, plan)
	if err != nil {
		return line
	}
	for _, c := range clips {
		if c.ID == clipID {
			return line.Int("corrections", len(c.Corrections)).Int("removed_words", len(c.RemovedWords))
		}
	}
	return line
}

// keptTranscripts is how many episodes' transcripts are kept, the same as
// KEPT in App.svelte.
const keptTranscripts = 10

// keptReads keeps what was read for the last few keys, each with the stamp
// of what it was read from, and reads a key once however many ask at once.
// The zero value keeps keptTranscripts.
type keptReads[T any] struct {
	mu   sync.Mutex
	held map[string]*keptRead[T]
	uses uint64
}

// keptRead is one key's read: as stamp says its source was, or still being
// read until done is closed.
type keptRead[T any] struct {
	stamp string
	v     T
	err   error
	done  chan struct{}
	used  uint64
}

// get gives what read gives for key, read again only when stamp has
// changed. A failure is not kept: the next ask reads again.
func (k *keptReads[T]) get(key, stamp string, read func() (T, error)) (T, error) {
	k.mu.Lock()
	k.uses++
	if r := k.held[key]; r != nil && r.stamp == stamp {
		r.used = k.uses
		k.mu.Unlock()
		<-r.done
		return r.v, r.err
	}
	r := &keptRead[T]{stamp: stamp, used: k.uses, done: make(chan struct{})}
	if k.held == nil {
		k.held = map[string]*keptRead[T]{}
	}
	k.held[key] = r
	for len(k.held) > keptTranscripts {
		oldest := ""
		for other, o := range k.held {
			if oldest == "" || o.used < k.held[oldest].used {
				oldest = other
			}
		}
		delete(k.held, oldest)
	}
	k.mu.Unlock()
	// However the read ends, a panic too, whoever waits is let go and a
	// failure is not kept. The failure is taken out first, so a waiter let
	// go that asks again reads again rather than finding it.
	defer func() {
		if r.err != nil {
			k.mu.Lock()
			if k.held[key] == r {
				delete(k.held, key)
			}
			k.mu.Unlock()
		}
		close(r.done)
	}()
	r.err = fmt.Errorf("the read stopped before it finished")
	r.v, r.err = read()
	return r.v, r.err
}

// words is what an episode says, see engine/words.go, from the transcript
// read once and kept. An episode not transcribed yet says nothing, which
// is an empty answer and not a failure.
func (s *FrameFairy) words(path string) (*engine.Transcript, error) {
	t, err := s.transcript(s.project(path))
	if errors.Is(err, engine.ErrNoTranscript) {
		return &engine.Transcript{}, nil
	}
	return t, err
}

// Words returns the words said in a part, for walking the playhead from
// word to word. Before the first transcription there are none, which is an
// empty answer and not a failure.
func (s *FrameFairy) Words(path string, from, to float64) ([]engine.WordView, error) {
	if !s.store.Known(path) {
		return nil, errNotInLibrary
	}
	t, err := s.words(path)
	if err != nil {
		return nil, err
	}
	out := []engine.WordView{}
	// One word either side, so a step can reach past the part.
	for _, w := range t.WordsBetween(from-5, to+5) {
		out = append(out, engine.WordView{Start: w.Start, End: w.End, Text: w.Text})
	}
	return out, nil
}

// SetWord corrects a word of one clip, typed in its caption box. Every
// other clip still reads it as it was heard. A word typed empty is removed
// from the clip, and a word the clip removed typed back in beside its
// neighbour comes back in it. See engine.SetWordText.
func (s *FrameFairy) SetWord(ctx context.Context, path, plan, clipID string, start float64, text string) (ClipEntry, error) {
	if !s.store.PlanOf(path, plan) {
		return ClipEntry{}, errNotInLibrary
	}
	// A correction changes the clip and never what the video says, so the
	// words kept are the words to read.
	t, err := s.words(path)
	if err != nil {
		return ClipEntry{}, err
	}
	began := time.Now()
	err = s.edit(path, func() error {
		return engine.SetWordText(s.store.files, plan, clipID, start, text, t)
	})
	// Where the word is and whether it was removed, never what it reads.
	line := s.wordCounts(editLine(editLevel(err), path, clipID, time.Since(began)).Err(err), plan, clipID)
	line.Float64("word", start).Bool("removed", strings.TrimSpace(text) == "").Msg("a word was corrected")
	if err != nil {
		return ClipEntry{}, err
	}
	return s.clipEntry(ctx, path, plan, clipID)
}

// RemoveWord removes a word from the captions of one clip, the word heard
// at start, or one of the words a correction made of it, part counted
// from nought, or all of it with part below nought. The video still says
// it and every other clip still shows it. See engine.RemoveWord.
func (s *FrameFairy) RemoveWord(ctx context.Context, path, plan, clipID string, start float64, part int) (ClipEntry, error) {
	if !s.store.PlanOf(path, plan) {
		return ClipEntry{}, errNotInLibrary
	}
	// Removing a word changes the clip and never what the video says, so
	// the words kept are the words to read.
	t, err := s.words(path)
	if err != nil {
		return ClipEntry{}, err
	}
	began := time.Now()
	err = s.edit(path, func() error {
		return engine.RemoveWord(s.store.files, plan, clipID, start, part, t)
	})
	// Where the word is and which of its words, never what it reads.
	line := s.wordCounts(editLine(editLevel(err), path, clipID, time.Since(began)).Err(err), plan, clipID)
	line.Float64("word", start).Int("part", part).Msg("a word was removed")
	if err != nil {
		return ClipEntry{}, err
	}
	return s.clipEntry(ctx, path, plan, clipID)
}

// RemoveCaption removes the caption of a clip that begins on the word heard
// at first, every word in it, from that clip alone, as one step to undo.
// See engine.RemoveCaption.
func (s *FrameFairy) RemoveCaption(ctx context.Context, path, plan, clipID string, first float64) (ClipEntry, error) {
	if !s.store.PlanOf(path, plan) {
		return ClipEntry{}, errNotInLibrary
	}
	// As RemoveWord: the clip changes, and the words said do not.
	t, err := s.words(path)
	if err != nil {
		return ClipEntry{}, err
	}
	began := time.Now()
	err = s.edit(path, func() error {
		return engine.RemoveCaption(s.store.files, plan, clipID, first, t, s.captionLook(path, plan))
	})
	line := s.wordCounts(editLine(editLevel(err), path, clipID, time.Since(began)).Err(err), plan, clipID)
	line.Float64("word", first).Msg("a caption was removed")
	if err != nil {
		return ClipEntry{}, err
	}
	return s.clipEntry(ctx, path, plan, clipID)
}
