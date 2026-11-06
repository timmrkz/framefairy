package main

import (
	"path/filepath"
	"testing"

	"framefairy/engine"
)

// Which paths in and over the models folder are the app's own download
// at work, left unheard, and which can change the models the setup shows.
func TestWhatTheWatchHearsOfTheModels(t *testing.T) {
	t.Parallel()
	over := filepath.Join("/home", "tim", ".framefairy")
	models := filepath.Join(over, "models")
	ws := &watching{models: models}
	speech := filepath.Join(models, engine.SpeechModels()[0].Name)
	for _, c := range []struct {
		path                        string
		downloading, models, folder bool
	}{
		{models, false, true, true},
		{over, false, false, false},
		{filepath.Join(over, "speed.json"), false, false, false},
		{filepath.Join(over, "training"), false, false, false},
		{filepath.Join(models, "gemma.gguf"), false, true, false},
		{filepath.Join(models, "gemma.gguf.part"), true, true, false},
		{filepath.Join(models, "parakeet.unpacking"), true, true, false},
		{filepath.Join(models, "parakeet", "tokens.txt"), false, true, false},
		{filepath.Join(models, "parakeet", "x.part"), false, true, false},
		{filepath.Join(over, "models2", "gemma.gguf"), false, false, false},
		{filepath.Join(over, "models2"), false, false, false},
	} {
		heard, folder := ws.modelsIn([]string{c.path})
		if got := ws.downloading(c.path); got != c.downloading || heard != c.models || folder != c.folder {
			t.Errorf("%s: downloading %v, models %v, folder %v, want %v, %v, %v",
				c.path, got, heard, folder, c.downloading, c.models, c.folder)
		}
	}
	if _, folder := ws.modelsIn([]string{speech}); !folder {
		t.Errorf("the folder of a speech model is not followed: %s", speech)
	}
	var none *watching
	if m, f := none.modelsIn([]string{models}); m || f || none.downloading(models+"/x.part") {
		t.Error("no watch hears the models")
	}
}

// No watcher watches a folder and the folder it is in: on macOS the
// watcher of a folder sets a watch of its own on every folder made in it,
// in the same moment the app adds that folder, and when it came last the
// app heard nothing written in the new folder, issue #722. So every
// folder the app watches, for a video and for the models, is in the other
// watcher from the folder it is in.
func TestAFolderIsNeverWatchedByTheWatcherOfTheFolderItIsIn(t *testing.T) {
	t.Parallel()
	ws := newWatching(func([]string) {}, filepath.Join("/home", "tim", ".framefairy", "models"))
	if ws == nil {
		t.Skip("this system watches nothing")
	}
	t.Cleanup(ws.stop)
	video := filepath.Join("/home", "tim", "Movies", "episode.mp4")
	work := engine.WorkDir(video)
	folders := append(watchedOf(video, []string{filepath.Join(work, "shorts", "clip-1.mp4")}), ws.modelFolders()...)
	if ws.byDepth[0] == ws.byDepth[1] {
		t.Fatal("both depths have the same watcher")
	}
	for _, dir := range folders {
		if ws.watcherOf(dir) == ws.watcherOf(filepath.Dir(dir)) {
			t.Errorf("%s is watched by the watcher of %s, the folder it is in", dir, filepath.Dir(dir))
		}
	}
}
