package main

import (
	"context"
	"errors"
	"fmt"
	"os/exec"
	"path/filepath"
	"runtime/debug"
	"slices"
	"strings"
	"time"
	"unicode/utf8"

	"github.com/rs/zerolog"

	"framefairy/engine"
	"framefairy/internal/framewire"
)

// Library lists the episodes with their state. The library is brought in
// step with the disk first, see reconcile, so a video whose folder was
// deleted outside the app is not listed.
func (s *FrameFairy) Library() []engine.EpisodeStatus {
	// After the first look, so the library opens with what it let go
	// gone and what it restored there, see lookFirst.
	s.store.afterFirstLook()
	s.reconcile(nil)
	opts := s.store.Settings().options()
	out := []engine.EpisodeStatus{}
	// Every video's jobs under one lock, the way a video's state picks
	// them, see ofVideo.
	var jobs map[string][]Job
	if s.jobs != nil {
		jobs = s.jobs.ofVideos()
	}
	for _, ep := range s.store.Episodes() {
		out = append(out, s.statusWith(ep, opts, jobs[ep]))
	}
	return out
}

// Episode returns one episode's state.
// The workspace reads it in the state of the video, see State, and calls
// it no more. The walks and the tests ask it.
func (s *FrameFairy) Episode(path string) engine.EpisodeStatus {
	if !s.store.Known(path) {
		return engine.EpisodeStatus{}
	}
	return s.episode(path, s.videoJobs(path))
}

// episode is the state of the episode at path, whose jobs are jobs, how it
// stands in the sidebar with them.
func (s *FrameFairy) episode(path string, jobs []Job) engine.EpisodeStatus {
	if !s.store.Known(path) {
		return engine.EpisodeStatus{}
	}
	return s.statusWith(path, s.store.Settings().options(), jobs)
}

// statusWith is the state of the episode at path as its files say it,
// read with opts, and how it stands with its jobs, see searchStands.
func (s *FrameFairy) statusWith(path string, opts engine.Options, jobs []Job) engine.EpisodeStatus {
	st := s.store.machine.Episode(path).Status(opts.ASRModel, opts.SilenceDB)
	st.Stands = standing(st, searchStands(jobs))
	return st
}

// videoJobs is the jobs of the video at path as the workspace shows them,
// none where no queue runs, as in a test that builds the service by hand.
func (s *FrameFairy) videoJobs(path string) []Job {
	if s.jobs == nil {
		return nil
	}
	jobs, _ := s.jobs.ofVideo(path)
	return jobs
}

// ChooseFolder asks for a folder, the way a Mac app asks where to save
// things, and says which one was chosen, or nothing when the person
// cancelled. It changes nothing: the settings keep the answer when they
// are saved.
func (s *FrameFairy) ChooseFolder(title string) (string, error) {
	return s.app.Dialog.OpenFile().
		SetTitle(title).
		SetButtonText("Choose").
		CanChooseFiles(false).
		CanChooseDirectories(true).
		CanCreateDirectories(true).
		PromptForSingleSelection()
}

// AddEpisodes asks for video files and adds them to the library, the way
// a drop does, see takeFiles. Cancelled, it adds nothing and says nothing.
func (s *FrameFairy) AddEpisodes() Added {
	dialog := s.app.Dialog.OpenFile().
		SetTitle("Add videos").
		CanChooseFiles(true).
		AddFilter("Video", engine.VideoPattern())
	paths, err := dialog.PromptForMultipleSelection()
	if err != nil {
		return Added{Added: []string{}, Error: err.Error()}
	}
	if len(paths) == 0 {
		return Added{Added: []string{}}
	}
	// One file that cannot be added is not a reason to drop the others, so
	// what was added is started either way and the reason travels with it.
	return s.takeFiles("add", paths)
}

// VideoNames says which files the app takes as videos, the way a person
// reads them, for the window to say so. The list is
// engine.VideoExtensions.
func (s *FrameFairy) VideoNames() string { return engine.VideoNames() }

// addEpisodes adds videos to the library and answers at once, so the
// window opens the video the moment it was dropped or picked. Whether its
// picture can be decoded is asked behind that, see checkAdded: the probe
// and the decoder's first frame took 0.3 to 1.4 s on Tim's Mac, with
// nothing on screen while they ran, issue #832.
func (s *FrameFairy) addEpisodes(videos []string) ([]string, error) {
	// A video added while the first look runs is weighed against the
	// library it leaves, see lookFirst.
	s.store.afterFirstLook()
	// Which of them are new, with no work folder before this add and
	// nothing put back into one, see store.addEpisodes.
	added, fresh, err := s.store.addEpisodes(videos)
	for _, v := range added {
		s.jobs.openEpisode(v)
		s.levels.openEpisode(v)
		// A video added again whose work folder was kept picks up where it
		// left off, so what its records say stopped says so again, the way
		// it does when the app starts.
		s.restoreJobs([]string{v})
		// A video with no work folder before this add was never searched,
		// and is not read for it: the answer waits on every read here.
		ctx, search, done := s.startCheck(v, fresh[v] || !s.store.machine.Episode(v).EverSearched())
		go func() {
			defer done()
			s.checkAdded(ctx, v, fresh[v], search)
		}()
	}
	return added, err
}

// check is the picture of a video just added being checked: done is
// closed once it is, and stop calls it off. search is the press the
// video's first search is asked for with once the check has passed, and
// "" for a video searched before, which waits for a search to be asked
// for. The video's state says that search is on its way for as long as
// the check runs, see searchComingLocked.
type check struct {
	done   chan struct{}
	stop   context.CancelFunc
	search string
}

// startCheck says that the picture of a video just added is being
// checked, and with first that its first search follows. It gives back
// what the check runs under, called off by stopCheck, the press that
// search is asked for with, and what says the check is done. Done, the
// window is told, so the video's state no longer says a search is on its
// way, see searchComingLocked.
func (s *FrameFairy) startCheck(video string, first bool) (context.Context, string, func()) {
	ctx, stop := context.WithCancel(context.Background())
	c := &check{done: make(chan struct{}), stop: stop}
	s.checkMu.Lock()
	defer s.checkMu.Unlock()
	if s.checking == nil {
		s.checking = map[string]*check{}
	}
	s.checks++
	if first {
		c.search = fmt.Sprintf("first search %d", s.checks)
	}
	s.checking[video] = c
	return ctx, c.search, func() {
		s.checkMu.Lock()
		if s.checking[video] == c {
			delete(s.checking, video)
		}
		stop()
		close(c.done)
		s.checkMu.Unlock()
		s.told(video)
	}
}

// checkOf is the check of a video's picture that runs, or nil.
func (s *FrameFairy) checkOf(video string) *check {
	s.checkMu.Lock()
	defer s.checkMu.Unlock()
	return s.checking[video]
}

// searchComingLocked is the press the first search of a video just added is
// asked for with, while its picture is checked, or "". From the moment Add
// answers until that search is queued nothing of the video runs, and the
// clip list offered Search, which a walk took for the search being over,
// issue #848. s.checkMu is held.
func (s *FrameFairy) searchComingLocked(video string) string {
	if c := s.checking[video]; c != nil {
		return c.search
	}
	return ""
}

// stopCheck calls off the check of a video's picture, and waits until it
// has ended, so a video removed while it is checked starts no search on
// its way out and takes nothing out after it was added again.
func (s *FrameFairy) stopCheck(video string) {
	if c := s.checkOf(video); c != nil {
		c.stop()
		<-c.done
	}
}

// checkAdded asks whether the picture of a video just added can be
// decoded. A file whose picture ffmpeg cannot decode goes out of the
// library again, with the folder its add made, and the window is told why
// the way it is told why a file given to it was left out: the video
// preview would have nothing to show, and the render nothing to cut. A
// new video's first search starts once its picture is known to be there,
// asked for with the press search, see startCheck. It hears the episode
// as far as its window and no further. An episode that has been searched
// before has no search, and waits for a search to be asked for.
func (s *FrameFairy) checkAdded(ctx context.Context, video string, fresh bool, search string) {
	defer func() {
		if caught := recover(); caught != nil {
			theLog.line(zerolog.ErrorLevel, "app").Str("video", filepath.Base(video)).Str("stack", string(debug.Stack())).
				Msgf("checking a video just added stopped unexpectedly: %v", caught)
		}
	}()
	why := s.undecodable(ctx, video, s.frameLate)
	// Removed while it was checked: nothing is left to start or take out.
	if ctx.Err() != nil || !s.store.Known(video) {
		return
	}
	if why == "" {
		s.levels.start(video)
		if search != "" {
			s.firstSearch(ctx, video, search)
		}
		return
	}
	said := fmt.Sprintf("the picture of %s cannot be decoded, %s, so it cannot be added", filepath.Base(video), why)
	theLog.line(zerolog.WarnLevel, "app").Str("video", filepath.Base(video)).Bool("work_deleted", fresh).Str("why", why).
		Msg("a video just added is taken out again, its picture cannot be decoded")
	if err := s.removeEpisode(video, fresh); err != nil {
		said += ". And it could not be taken out of the library, " + err.Error()
	}
	droppedEvent.to(s.tell, Added{Added: []string{}, Error: said})
}

// firstFrameWait is how long Add waits for the first frame of a video
// before it takes the video without one.
const firstFrameWait = 30 * time.Second

// undecodable says why the picture of a video cannot be decoded, or ""
// where it can: the episode's decoder is asked for its first frame. That
// decoder is the one the video preview reads from when the episode opens,
// so asking costs no start of it later. It waits firstFrameWait for the
// frame, or until late says it is late.
//
// Only the decoder's own words leave a video out. A frame that does not
// come in time says nothing about the file, only about the machine: on
// CI's Mac, with its media engine hung, three tests had a good video left
// out as "context deadline exceeded" after 30 s, which would have told a
// person their video could not be decoded. Such a video is taken, and
// the video preview shows how its decoder does.
func (s *FrameFairy) undecodable(ctx context.Context, video string, late <-chan struct{}) string {
	abs, err := filepath.Abs(video)
	if err != nil {
		return err.Error()
	}
	dec, err := s.previews.decoder(s.store.machine, abs)
	if err != nil {
		return strings.TrimSuffix(err.Error(), ".")
	}
	asked := time.Now()
	called := ctx
	ctx, cancel := context.WithTimeout(ctx, firstFrameWait)
	defer cancel()
	if late != nil {
		go func() {
			select {
			case <-late:
				cancel()
			case <-ctx.Done():
			}
		}()
	}
	// On the grid of its short, or on one of a single frame where the
	// probe finds none, so a file with no picture is refused in the
	// decoder's own words.
	info, _ := s.probe(ctx, abs)
	grid := info.Grid()
	if !grid.Valid() {
		grid = framewire.Grid{Num: 1, Den: 1, Count: 1}
	}
	frames := 0
	err = dec.Stream(ctx, grid, 0, 64, 36, false, nil, func(float64, []byte) error {
		frames++
		return errFirstFrame
	})
	switch {
	case frames > 0:
		return ""
	case called.Err() != nil:
		// The video is being removed, and nothing came late.
		return ""
	case ctx.Err() != nil:
		theLog.line(zerolog.WarnLevel, "files").Str("video", filepath.Base(abs)).Dur("waited", time.Since(asked)).
			Msg("no first frame came in time, so the video is taken without one")
		return ""
	case err == nil:
		return "it holds no frame"
	default:
		return strings.TrimSuffix(err.Error(), ".")
	}
}

// errFirstFrame ends the stream undecodable asks for once a frame came.
var errFirstFrame = errors.New("a frame came")

// RemoveEpisode takes an episode out of the library. With deleteWork, it
// also deletes its work folder, rendered clips and all, after the engine
// kept its transcript and clip sets, so adding it again brings back those
// and nothing else, see engine.Machine.Keep. The episode file itself
// always stays.
func (s *FrameFairy) RemoveEpisode(path string, deleteWork bool) error {
	// A video removed while its picture is being checked: the check is
	// called off first, so nothing it starts outlives the video.
	s.stopCheck(path)
	return s.removeEpisode(path, deleteWork)
}

// removeEpisode is RemoveEpisode once nothing checks the video's picture
// any more, or from the check itself.
func (s *FrameFairy) removeEpisode(path string, deleteWork bool) error {
	if !s.store.Known(path) {
		return errNotInLibrary
	}
	// Its work stops, and nothing new starts on it until it is out of the
	// library, whether its files go or stay. A removed episode that went on
	// transcribing held the one transcription lane for hours, and every
	// episode added after it waited without a word.
	// It stays closed once it is gone, and opens again only if it stays in
	// the library or when it is added again.
	reopen := s.jobs.closeEpisode(path)
	// The loudness stops being measured before anything is deleted, and
	// starts again on nothing the waveform asks for while the removal
	// waits, so nothing writes the work folder back. An episode that stays
	// in the library is measured again the next time its waveform is asked
	// for.
	reopenLevels := s.levels.closeEpisode(path)
	removed := false
	defer func() {
		if !removed {
			reopen()
			reopenLevels()
		}
	}()
	if deleteWork {
		// Nothing is deleted while something is still writing it. A job
		// that will not stop leaves the episode where it is, files and
		// all, and says so, because the alternative is a folder deleted
		// under a running transcription which then writes it back: an
		// episode the person removed, still there, with half a transcript
		// in it. Removing it again once the work has stopped does what it
		// says.
		if !s.jobs.waitEpisode(path) {
			return errors.New("something is still running on this video and would not stop, " +
				"so nothing was deleted. Remove the video again in a moment")
		}
	}
	// The watch hears the folder go, and reconcile waits until the video
	// is out of the library, so it never takes this for a folder deleted
	// outside the app.
	s.reconciling.Lock()
	defer s.reconciling.Unlock()
	if deleteWork {
		// What was made of it is kept first, up to the last word, so it
		// comes back when the video is added again. If it cannot be kept,
		// nothing is deleted.
		if err := s.keep(path); err != nil {
			return fmt.Errorf("the transcript and clips of this video could not be kept, so nothing was deleted: %w", err)
		}
		if err := s.store.machine.DeleteWork(path); err != nil {
			theLog.line(zerolog.WarnLevel, "library").Str("video", filepath.Base(path)).Err(err).
				Msg("the work folder of a video being removed could not be deleted, so the video stays")
			return err
		}
	}
	s.forget(path)
	if err := s.store.RemoveEpisode(path); err != nil {
		return err
	}
	removed = true
	s.jobs.forgetEpisode(path)
	theLog.line(zerolog.InfoLevel, "library").Str("video", filepath.Base(path)).Bool("work_deleted", deleteWork).
		Msg("the video is removed")
	return nil
}

// clearLetGo waits until nothing runs on a video let go, and deletes what
// was written into its folder after the folder was deleted. A video added
// again in the meantime keeps its folder.
func (s *FrameFairy) clearLetGo(path string) {
	if !s.jobs.waitEpisode(path) {
		theLog.line(zerolog.WarnLevel, "files").Str("video", filepath.Base(path)).
			Msg("something still ran on a video let go and would not stop, so what it wrote back stays")
		return
	}
	cleared, err := s.store.clearLetGo(path)
	if err != nil {
		theLog.line(zerolog.WarnLevel, "files").Str("video", filepath.Base(path)).Err(err).
			Msg("what was written back into the folder of a video let go was not deleted")
	} else if cleared {
		theLog.line(zerolog.InfoLevel, "files").Str("video", filepath.Base(path)).
			Msg("what was written back into the folder of a video let go is deleted")
	}
}

// SetLabel gives a video in the library the words and the colour it is
// told apart by in the sidebar, and says what was kept, nothing when the
// words were taken away. Only the video's own work folder is written: the
// video keeps its name.
func (s *FrameFairy) SetLabel(path, text, colour string) (*engine.Label, error) {
	if !slices.Contains(s.store.Episodes(), path) || !s.store.Known(path) {
		return nil, errNotInLibrary
	}
	l, err := engine.WriteLabel(s.store.files, path, engine.Label{Text: text, Colour: colour})
	if err != nil {
		theLog.line(zerolog.WarnLevel, "library").Str("video", filepath.Base(path)).Err(err).
			Msg("the video's label could not be kept")
		return nil, err
	}
	if l == nil {
		theLog.line(zerolog.InfoLevel, "library").Str("video", filepath.Base(path)).
			Msg("the video's label is taken away")
	} else {
		theLog.line(zerolog.InfoLevel, "library").Str("video", filepath.Base(path)).
			Int("characters", utf8.RuneCountInString(l.Text)).Str("colour", l.Colour).
			Msg("the video has a label")
	}
	return l, nil
}

// Reveal shows a file in Finder, Explorer or the file manager.
func (s *FrameFairy) Reveal(path string) error {
	if !s.store.Known(path) {
		return errNotInLibrary
	}
	return showInFolder(path)
}

// showInFolder shows a file in Finder, Explorer or the file manager, for
// a file the app has already judged.
func showInFolder(path string) error {
	return startReaped(showCommand(path))
}

// startReaped starts a program that finishes by itself and waits for it
// out of the way. A program started and never waited for stays in the
// process table as a zombie until the app quits, one for every Reveal.
func startReaped(cmd *exec.Cmd) error {
	if err := cmd.Start(); err != nil {
		return err
	}
	go func() { _ = cmd.Wait() }()
	return nil
}

// keep has the engine copy what changed of a video's transcript and clip
// sets outside its work folder, see engine.Machine.Keep, and says in the
// log when it could not.
func (s *FrameFairy) keep(video string) error {
	err := s.store.machine.Keep(video)
	if err != nil {
		theLog.line(zerolog.WarnLevel, "library").Str("video", filepath.Base(video)).Err(err).
			Msg("the transcript and clips of a video could not be kept outside its folder")
	}
	return err
}
