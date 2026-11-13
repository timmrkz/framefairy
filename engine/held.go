package engine

import (
	"errors"
	"fmt"
	"path/filepath"
	"slices"
	"sort"
	"sync"

	"github.com/spf13/afero"

	"framefairy/engine/fileio"
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
	// transcript is the transcript as it was last read, and used when it
	// was last asked for, by the machine's count. heard is what Status
	// says of it. All three are guarded by the machine's held.
	transcript *keptRead[*TranscriptRead]
	used       uint64
	heard      *keptRead[heardSoFar]
	// plans is each plan file as it was last read, by its path, guarded by
	// the machine's held as well. Every plan of every video asked about is
	// kept, about ten times its file in memory: 11 KB for a plan of 24
	// clips, 97 KB for one from before the clips kept no words, measured
	// once. A video's plans go when it leaves the library, see Forget.
	plans map[string]*planSlot
}

// planSlot is where one plan file's read is kept, so held has a place
// that stays while the map grows.
type planSlot struct {
	read *keptRead[PlanSummary]
}

// keptTranscripts is how many videos' transcripts a program holds, the
// ones asked about last, the same as KEPT in App.svelte. A transcript of
// four hours is megabytes of words and levels.
const keptTranscripts = 10

// Episode is the one Episode of the video at source, made the first time
// it is asked for. A program keeps one for each video it asks about, a few
// words each, and holds a transcript for the last keptTranscripts. The
// machine only gives them out and holds the lock, the episode keeps the
// rules.
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

// Forget lets go of everything held of the video at source, for a video
// that left the library. One still asked for after is made again from its
// files.
func (m *Machine) Forget(source string) {
	m.held.Lock()
	defer m.held.Unlock()
	delete(m.episodes, filepath.Clean(source))
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
// are those of Options. It returns ErrNoTranscript before there is one,
// and that answer is kept by the same stamp as a transcript is.
func (ep *Episode) Transcript(modelDir string, silenceDB *float64) (*TranscriptRead, error) {
	at, err := ep.transcriptStamp(modelDir, silenceDB)
	if err != nil {
		return nil, err
	}
	return held(ep, &ep.transcript, at.stamp, ep.usedTranscript, func() (*TranscriptRead, error) {
		t, file, err := readSaved(ep.m.Files, at.video, LogsDir(ep.source), at.model, silenceDB)
		if err != nil {
			return nil, err
		}
		return newTranscriptRead(t, file.done()), nil
	})
}

// Plans is every plan of the episode, newest first, each read again only
// when its file changed, by its size and time, once however many ask at
// once. Files that do not parse are left out. What it hands out is shared
// and only read: the plan and the clips each summary keeps among them.
func (ep *Episode) Plans() []PlanSummary {
	files, _ := afero.Glob(ep.m.Files, filepath.Join(LogsDir(ep.source), "clips*.json"))
	ep.forgetPlansBut(files)
	var out []PlanSummary
	for _, file := range files {
		if s, err := ep.plan(file); err == nil {
			out = append(out, s)
		}
	}
	sort.Slice(out, func(a, b int) bool { return out[a].Modified.After(out[b].Modified) })
	return out
}

// ReadPlan is the plan at path for display, see ReadPlan, read through
// the episode when it is one of the episode's own plans.
func (ep *Episode) ReadPlan(path string) (*PlanView, error) {
	s, err := ep.Plan(path)
	if err != nil {
		return nil, err
	}
	return s.View(), nil
}

// Plan is the plan at path as it is now, read through the episode when it
// is one of the episode's own plans, and from the file each time when it
// is not. What it hands out is shared and only read.
func (ep *Episode) Plan(path string) (PlanSummary, error) {
	path = filepath.Clean(path)
	if filepath.Dir(path) != LogsDir(ep.source) {
		return summaryOf(ep.m.Files, path)
	}
	return ep.plan(path)
}

// plan is the plan file at path as it is now, read again only when its
// stamp changed, see fileio.Stamp.
func (ep *Episode) plan(path string) (PlanSummary, error) {
	info, err := ep.m.Files.Stat(path)
	if err != nil {
		return PlanSummary{}, err
	}
	if !info.Mode().IsRegular() {
		return PlanSummary{}, fmt.Errorf("%s is not a file", filepath.Base(path))
	}
	stamp := fileio.Stamp(info)
	ep.m.held.Lock()
	if ep.plans == nil {
		ep.plans = map[string]*planSlot{}
	}
	slot := ep.plans[path]
	if slot == nil {
		slot = &planSlot{}
		ep.plans[path] = slot
	}
	ep.m.held.Unlock()
	return held(ep, &slot.read, stamp, nil, func() (PlanSummary, error) {
		return summaryOf(ep.m.Files, path)
	})
}

// forgetPlansBut lets go of every plan read the episode keeps that is not
// one of files, the plans there are now.
func (ep *Episode) forgetPlansBut(files []string) {
	ep.m.held.Lock()
	defer ep.m.held.Unlock()
	for path := range ep.plans {
		if !slices.Contains(files, path) {
			delete(ep.plans, path)
		}
	}
}

// heardSoFar is what Status says of a transcript: whether it is done, the
// parts heard, and from the first line to the last. Shared and not to be
// changed.
type heardSoFar struct {
	done  bool
	heard [][2]float64
	said  *[2]float64
}

// heardSoFar is what the episode's transcript has heard, kept by the same
// stamp as the transcript, for every video asked about and with no cap:
// it is a few numbers. So the library, which asks it of every video, lets
// go of no transcript a workspace holds and reads none again that did
// not change. When the transcript itself is held for the stamp, it is
// taken from that, and otherwise read for itself.
func (ep *Episode) heardSoFar(modelDir string, silenceDB *float64) (heardSoFar, error) {
	at, err := ep.transcriptStamp(modelDir, silenceDB)
	if err != nil {
		return heardSoFar{}, err
	}
	return held(ep, &ep.heard, at.stamp, nil, func() (heardSoFar, error) {
		if t := ep.heldNow(at.stamp); t != nil {
			return heardSoFar{done: t.Done, heard: t.Heard, said: t.Said()}, nil
		}
		t, file, err := readSaved(ep.m.Files, at.video, LogsDir(ep.source), at.model, silenceDB)
		if err != nil {
			return heardSoFar{}, err
		}
		return heardSoFar{done: file.done(), heard: t.Heard, said: saidOf(BuildLines(t.Words, t.Levels(), nil))}, nil
	})
}

// heldNow is the transcript the episode holds read at stamp, nil when it
// holds none or its read failed. One still being read is waited for.
func (ep *Episode) heldNow(stamp string) *TranscriptRead {
	ep.m.held.Lock()
	r := ep.transcript
	ep.m.held.Unlock()
	if r == nil || r.stamp != stamp {
		return nil
	}
	// One being read for this stamp is waited for: reading the same files
	// again beside it takes as long, and twice the memory, #800.
	<-r.done
	return r.v
}

// transcriptAt is the stamp of a transcript's files as they are now, and
// what it is read with.
type transcriptAt struct {
	video sourceStamp
	model string
	stamp string
}

// transcriptStamp is how the video and the files its transcript is read
// from are now, the size and the time of each, with the speech model and
// the floor asked for: everything a read of the transcript depends on.
func (ep *Episode) transcriptStamp(modelDir string, silenceDB *float64) (transcriptAt, error) {
	fsys := ep.m.Files
	video, err := stampOf(fsys, ep.source)
	if err != nil {
		return transcriptAt{}, err
	}
	at := transcriptAt{video: video, model: modelName(modelDir)}
	floor := "auto"
	if silenceDB != nil {
		floor = fmt.Sprint(*silenceDB)
	}
	at.stamp = fmt.Sprintf("%v|%s|%s", video, at.model, floor)
	for _, file := range TranscriptFiles(LogsDir(ep.source)) {
		info, err := fsys.Stat(file)
		if err != nil {
			at.stamp += "|-"
			continue
		}
		at.stamp += "|" + fileio.Stamp(info)
	}
	return at, nil
}

// newTranscriptRead is t as it is held, done when it has heard the whole
// video.
func newTranscriptRead(t *Transcript, done bool) *TranscriptRead {
	return &TranscriptRead{Transcript: t, Done: done,
		said:    sync.OnceValue(func() *[2]float64 { return saidOf(BuildLines(t.Words, t.Levels(), nil)) }),
		weighed: sync.OnceValue(func() RoomAndLines { return Weigh(t) }),
	}
}

// held is what read gives for the episode's slot, read again only when
// stamp changed, once however many ask at once. An answer about the files,
// ErrNoTranscript, is kept like any other, but a failure is not: the next
// ask reads again. asked, when there is one, is told of each ask with
// the machine's lock held, and whether it placed a new read.
func held[T any](ep *Episode, slot **keptRead[T], stamp string, asked func(placed bool),
	read func() (T, error)) (T, error) {
	m := ep.m
	m.held.Lock()
	if r := *slot; r != nil && r.stamp == stamp {
		if asked != nil {
			asked(false)
		}
		m.held.Unlock()
		<-r.done
		return r.v, r.err
	}
	r := &keptRead[T]{stamp: stamp, done: make(chan struct{})}
	*slot = r
	if asked != nil {
		asked(true)
	}
	m.held.Unlock()
	// However the read ends, a panic too, whoever waits is let go and a
	// failure is not kept. The failure is taken out first, so a waiter let
	// go that asks again reads again rather than finding it.
	defer func() {
		if r.err != nil && !errors.Is(r.err, ErrNoTranscript) {
			m.held.Lock()
			if *slot == r {
				*slot = nil
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

// usedTranscript counts an ask of the episode's transcript, and once one
// more is placed lets the episodes asked about longest ago go of theirs,
// so no more than keptTranscripts hold one. The machine's lock is held.
func (ep *Episode) usedTranscript(placed bool) {
	m := ep.m
	m.uses++
	ep.used = m.uses
	if !placed {
		return
	}
	for {
		holding, oldest := 0, (*Episode)(nil)
		for _, other := range m.episodes {
			if other.transcript == nil {
				continue
			}
			holding++
			if oldest == nil || other.used < oldest.used {
				oldest = other
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
