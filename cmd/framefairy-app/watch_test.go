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
