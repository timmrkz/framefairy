package main

import (
	"context"
	"errors"
	"os/exec"
	"path/filepath"
	"runtime"

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
	added, err := s.store.AddEpisodes(videos)
	for _, v := range added {
		s.jobs.openEpisode(v)
		s.levels.start(v)
		s.firstSearch(context.Background(), v)
	}
	return added, err
}

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
				"so nothing was deleted. Stop it in Activity and remove the episode again")
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
