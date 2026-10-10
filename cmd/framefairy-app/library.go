package main

import (
	"context"
	"errors"
	"fmt"
	"os/exec"
	"path/filepath"
	"runtime"
	"strings"
	"time"

	"framefairy/engine"
)

// Library lists the episodes with their state.
func (s *FrameFairy) Library() []engine.EpisodeStatus {
	asrDir := s.store.Settings().ASRModel
	out := []engine.EpisodeStatus{}
	for _, ep := range s.store.Episodes() {
		out = append(out, engine.Status(ep, asrDir))
	}
	return out
}

// Episode returns one episode's state.
func (s *FrameFairy) Episode(path string) engine.EpisodeStatus {
	if !s.store.Known(path) {
		return engine.EpisodeStatus{}
	}
	return engine.Status(path, s.store.Settings().ASRModel)
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

// AddEpisodes asks for video files and adds them to the library.
func (s *FrameFairy) AddEpisodes() ([]string, error) {
	dialog := s.app.Dialog.OpenFile().
		SetTitle("Add episodes").
		CanChooseFiles(true).
		AddFilter("Video", "*.mp4;*.mov;*.m4v;*.mkv")
	paths, err := dialog.PromptForMultipleSelection()
	if err != nil || len(paths) == 0 {
		return nil, err
	}
	var videos []string
	for _, p := range paths {
		if engine.IsVideo(p) {
			videos = append(videos, p)
		}
	}
	// One file that cannot be added is not a reason to drop the others, so
	// what was added is started either way and the reason travels with it.
	return s.addEpisodes(videos)
}

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
		if why := undecodable(v); why != "" {
			refused = append(refused, fmt.Sprintf("the picture of %s cannot be decoded, %s", filepath.Base(v), why))
			continue
		}
		take = append(take, v)
	}
	added, err := s.store.AddEpisodes(take)
	if len(refused) > 0 {
		why := strings.Join(refused, ", ") + ", so it was left out"
		if err != nil {
			why = err.Error() + ". And " + why
		}
		err = errors.New(why)
	}
	for _, v := range added {
		s.jobs.openEpisode(v)
		s.levels.start(v)
		s.firstSearch(context.Background(), v)
	}
	return added, err
}

// undecodable says why the picture of a video cannot be decoded, or ""
// where it can: the episode's decoder is asked for its first frame. That
// decoder is the one the video preview reads from when the episode opens,
// so asking costs no start of it later.
func undecodable(video string) string {
	abs, err := filepath.Abs(video)
	if err != nil {
		return err.Error()
	}
	dec, err := openPreviews.decoder(abs)
	if err != nil {
		return strings.TrimSuffix(err.Error(), ".")
	}
	ctx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
	defer cancel()
	frames := 0
	err = dec.Stream(ctx, 0, 64, 36, func(float64, []byte) error {
		frames++
		return errFirstFrame
	})
	switch {
	case frames > 0:
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
			return errors.New("something is still running on this episode and would not stop, " +
				"so nothing was deleted. Remove the episode again in a moment")
		}
		if err := engine.DeleteWork(path); err != nil {
			return err
		}
	}
	s.forget(path)
	if err := s.store.RemoveEpisode(path); err != nil {
		return err
	}
	removed = true
	return nil
}

// Reveal shows a file in Finder, Explorer or the file manager.
func (s *FrameFairy) Reveal(path string) error {
	if !s.store.Known(path) {
		return errNotInLibrary
	}
	switch runtime.GOOS {
	case "darwin":
		return startReaped(exec.Command("open", "-R", path))
	case "windows":
		// One argument, not two: with a space after the comma Explorer
		// opens its default folder and selects nothing.
		return startReaped(exec.Command("explorer", "/select,"+path))
	}
	dir := path
	if fileExists(path) {
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
