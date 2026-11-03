package engine

import (
	"errors"
	"fmt"
	"path/filepath"
	"sync"
)

// Episode is what the program holds of one video's files. The files are
// the truth, and anyone may write them, the app, the command line, a
// person in Finder, so what is held is a copy that knows when it no longer
// matches: each read is kept with the stamp of the files it was read
// from, their size and time, and every question looks at the stamps first
// and reads again only what changed, once however many ask at once. What
// it hands out is shared and only read. Machine.Episode gives the one of
// a video. It holds the transcript so far. See
// docs/decisions/2026-11-03-one-episode-per-video.md.
type Episode struct {
	m      *Machine
	source string
	// transcript is the transcript as it was last read, guarded by the
	// machine's held, and used when it was last asked for, by its count.
	transcript *keptRead[*TranscriptRead]
	used       uint64
}

// keptTranscripts is how many videos' transcripts a program holds, the
// ones asked about last, the same as KEPT in App.svelte. A transcript of
// four hours is megabytes of words and levels.
const keptTranscripts = 10

// Episode is the one Episode of the video at source, made the first time
// it is asked for. A program keeps one for each video it asks about, a few
// words each, and holds a transcript for the last keptTranscripts.
func (m *Machine) Episode(source string) *Episode {
	source = filepath.Clean(source)
	m.held.Lock()
	defer m.held.Unlock()
	if ep := m.episodes[source]; ep != nil {
		return ep
	}
	if m.episodes == nil {
		m.episodes = map[string]*Episode{}
	}
	ep := &Episode{m: m, source: source}
	m.episodes[source] = ep
	return ep
}

// TranscriptRead is a video's transcript as it was read, shared by all
// who asked for it and only read, with what is worked out from it the
// first time it is asked for. Transcript.Heard are the parts heard.
type TranscriptRead struct {
	*Transcript
	// Done says the transcript has heard the whole video.
	Done    bool
	said    func() *[2]float64
	weighed func() RoomAndLines
}

// Said is from the start of the first line heard to the end of the last,
// nil while no word is, see EpisodeStatus.Said. It is shared and not to
// be changed.
func (r *TranscriptRead) Said() *[2]float64 { return r.said() }

// Weighed is Weigh of the transcript, worked out once for each read of
// it: four hours of lines take most of 55 ms to weigh. Its lines are
// shared and not to be changed.
func (r *TranscriptRead) Weighed() RoomAndLines { return r.weighed() }

// Transcript is the transcript of the episode as far as it is heard, read
// again only when the video or a file of its transcript changed, or the
// speech model or the floor asked for is another. modelDir and silenceDB
// are those of Options. It returns ErrNoTranscript before there is one.
func (ep *Episode) Transcript(modelDir string, silenceDB *float64) (*TranscriptRead, error) {
	fsys := ep.m.Files
	video, err := stampOf(fsys, ep.source)
	if err != nil {
		return nil, err
	}
	model := modelName(modelDir)
	floor := "auto"
	if silenceDB != nil {
		floor = fmt.Sprint(*silenceDB)
	}
	stamp := fmt.Sprintf("%v|%s|%s", video, model, floor)
	logs := LogsDir(ep.source)
	for _, file := range TranscriptFiles(logs) {
		info, err := fsys.Stat(file)
		if err != nil {
			stamp += "|-"
			continue
		}
		stamp += fmt.Sprintf("|%d,%d", info.Size(), info.ModTime().UnixNano())
	}
	return ep.m.transcriptHeld(ep, stamp, func() (*TranscriptRead, error) {
		t, file, err := readSaved(fsys, video, logs, model, silenceDB)
		if err != nil {
			return nil, err
		}
		return newTranscriptRead(t, file.done()), nil
	})
}

// newTranscriptRead is t as it is held, done when it has heard the whole
// video.
func newTranscriptRead(t *Transcript, done bool) *TranscriptRead {
	return &TranscriptRead{Transcript: t, Done: done,
		said:    sync.OnceValue(func() *[2]float64 { return saidOf(BuildLines(t.Words, t.Levels(), nil)) }),
		weighed: sync.OnceValue(func() RoomAndLines { return Weigh(t) }),
	}
}

// transcriptHeld is what read gives for ep's transcript, read again only
// when stamp changed, once however many ask at once. A failure is not
// kept: the next ask reads again. Once more than keptTranscripts episodes
// hold one, the one asked for longest ago lets go of its own.
func (m *Machine) transcriptHeld(ep *Episode, stamp string, read func() (*TranscriptRead, error)) (*TranscriptRead, error) {
	m.held.Lock()
	m.uses++
	ep.used = m.uses
	if r := ep.transcript; r != nil && r.stamp == stamp {
		m.held.Unlock()
		<-r.done
		return r.v, r.err
	}
	r := &keptRead[*TranscriptRead]{stamp: stamp, done: make(chan struct{})}
	ep.transcript = r
	m.letGoOfTranscripts()
	m.held.Unlock()
	// However the read ends, a panic too, whoever waits is let go and a
	// failure is not kept. The failure is taken out first, so a waiter let
	// go that asks again reads again rather than finding it.
	defer func() {
		if r.err != nil {
			m.held.Lock()
			if ep.transcript == r {
				ep.transcript = nil
			}
			m.held.Unlock()
		}
		close(r.done)
	}()
	r.err = errReadStopped
	r.v, r.err = read()
	return r.v, r.err
}

// errReadStopped is a read that panicked, as its waiters are told.
var errReadStopped = errors.New("the read stopped before it finished")

// letGoOfTranscripts lets the episodes asked about longest ago go of their
// transcripts, so no more than keptTranscripts hold one. The caller holds
// held.
func (m *Machine) letGoOfTranscripts() {
	for {
		holding, oldest := 0, (*Episode)(nil)
		for _, ep := range m.episodes {
			if ep.transcript == nil {
				continue
			}
			holding++
			if oldest == nil || ep.used < oldest.used {
				oldest = ep
			}
		}
		if holding <= keptTranscripts {
			return
		}
		oldest.transcript = nil
	}
}

// keptRead is one read of a file kept: as stamp says the file was, or
// still being read until done is closed.
type keptRead[T any] struct {
	stamp string
	v     T
	err   error
	done  chan struct{}
}
