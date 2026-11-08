package main

import (
	"context"
	"errors"
	"fmt"
	"os/exec"
	"path/filepath"
	"runtime"
	"slices"
	"strings"
	"time"
	"unicode/utf8"

	"github.com/rs/zerolog"
	"github.com/spf13/afero"

	"framefairy/engine"
	"framefairy/engine/fileio"
	"framefairy/internal/framewire"
)

// Library lists the episodes with their state. The library is brought in
// step with the disk first, see reconcile, so a video whose folder was
// deleted outside the app is not listed.
func (s *FrameFairy) Library() []engine.EpisodeStatus {
	s.reconcile(nil)
	opts := s.store.Settings().options()
	out := []engine.EpisodeStatus{}
	for _, ep := range s.store.Episodes() {
		out = append(out, s.store.machine.Episode(ep).Status(opts.ASRModel, opts.SilenceDB))
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
	opts := s.store.Settings().options()
	return s.store.machine.Episode(path).Status(opts.ASRModel, opts.SilenceDB)
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

// addEpisodes adds videos to the library. A new episode's first search
// starts by itself, and hears the episode as far as its window and no
// further. An episode that has been searched before waits for a search to
// be asked for.
func (s *FrameFairy) addEpisodes(videos []string) ([]string, error) {
	// A file whose picture ffmpeg cannot decode is left out with the
	// reason, the way a file that clashes is: the video preview would have
	// nothing to show, and the render nothing to cut.
	var take, refused []string
	for _, v := range videos {
		if why := s.undecodable(v, s.frameLate); why != "" {
			refused = append(refused, fmt.Sprintf("the picture of %s cannot be decoded, %s", filepath.Base(v), why))
			continue
		}
		take = append(take, v)
	}
	added, err := s.store.AddEpisodes(take)
	if len(refused) > 0 {
		why := strings.Join(refused, ", ") + ", so it cannot be added"
		if err != nil {
			why = err.Error() + ". And " + why
		}
		err = errors.New(why)
	}
	for _, v := range added {
		s.jobs.openEpisode(v)
		// A video added again whose work folder was kept picks up where it
		// left off, so what its records say stopped says so again, the way
		// it does when the app starts.
		s.restoreJobs([]string{v})
		s.levels.start(v)
		s.firstSearch(context.Background(), v)
	}
	return added, err
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
func (s *FrameFairy) undecodable(video string, late <-chan struct{}) string {
	abs, err := filepath.Abs(video)
	if err != nil {
		return err.Error()
	}
	dec, err := s.previews.decoder(s.store.machine, abs)
	if err != nil {
		return strings.TrimSuffix(err.Error(), ".")
	}
	asked := time.Now()
	ctx, cancel := context.WithTimeout(context.Background(), firstFrameWait)
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
// also deletes everything made for it, so adding it again starts from
// nothing. The episode file itself always stays.
func (s *FrameFairy) RemoveEpisode(path string, deleteWork bool) error {
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
	// The loudness stops being measured before anything is deleted, so
	// nothing writes the work folder back. An episode that stays in the
	// library is measured again the next time its waveform is asked for.
	s.levels.stop(path)
	removed := false
	defer func() {
		if !removed {
			reopen()
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
			Msg("what was written back into the folder of a video let go could not be deleted")
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
	switch runtime.GOOS {
	case "darwin":
		return startReaped(exec.Command("open", "-R", path))
	case "windows":
		// One argument, not two: with a space after the comma Explorer
		// opens its default folder and selects nothing.
		return startReaped(exec.Command("explorer", "/select,"+path))
	}
	dir := path
	if fileio.IsFile(afero.NewOsFs(), path) {
		dir = filepath.Dir(path)
	}
	return startReaped(exec.Command("xdg-open", dir))
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
