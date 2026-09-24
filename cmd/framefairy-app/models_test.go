package main

import (
	"context"
	"os"
	"path/filepath"
	"testing"
	"time"

	"framefairy/engine"
)

// Two models on the machine, which is what installing a second one to try
// it leaves behind. The one in use is chosen with a click, and removing
// one gives its room back and lets go of it in the settings.
func TestTwoModelsOneInUseAndOneRemoved(t *testing.T) {
	svc, _, home := library(t)
	dir := filepath.Join(home, ".framefairy", "models")
	if err := os.MkdirAll(dir, 0o755); err != nil {
		t.Fatal(err)
	}
	models := engine.LanguageModels()
	first, second := models[0], models[len(models)-1]
	for _, m := range []engine.LanguageModel{first, second} {
		if err := os.WriteFile(filepath.Join(dir, m.Name), []byte("GGUF and the rest"), 0o644); err != nil {
			t.Fatal(err)
		}
	}
	inUse := func() []string {
		var out []string
		for _, m := range svc.Setup(context.Background()).Language {
			if m.InUse {
				out = append(out, m.Name)
			}
		}
		return out
	}
	// Two and none named: none is in use, which is the search that would
	// fail, and what the list has to let somebody settle.
	if got := inUse(); len(got) != 0 {
		t.Errorf("two installed and none named, in use: %v", got)
	}
	path, err := svc.UseLanguageModel(second.Name)
	if err != nil || path != filepath.Join(dir, second.Name) {
		t.Fatalf("use: %s, %v", path, err)
	}
	if got := inUse(); len(got) != 1 || got[0] != second.Name {
		t.Errorf("in use: %v", got)
	}
	if _, err := svc.UseLanguageModel("no-such.gguf"); err == nil {
		t.Error("a model that is not in the catalogue was used")
	}

	// Not while clips are being found: the search may be reading it.
	release := make(chan struct{})
	svc.jobs.add("", "plan", "Find clips", func(ctx context.Context, p *engine.Project) (string, error) {
		<-release
		return "", nil
	})
	if err := svc.RemoveLanguageModel(second.Name); err == nil {
		t.Error("removed while clips were being found")
	}
	close(release)
	for svc.busyWith("plan") {
		time.Sleep(time.Millisecond)
	}

	if err := svc.RemoveLanguageModel(second.Name); err != nil {
		t.Fatal(err)
	}
	if _, err := os.Stat(filepath.Join(dir, second.Name)); !os.IsNotExist(err) {
		t.Error("the file is still there")
	}
	if svc.store.Settings().LLMModel != "" {
		t.Error("the settings still name a model that is gone")
	}
	// One left, so it is the one in use.
	if got := inUse(); len(got) != 1 || got[0] != first.Name {
		t.Errorf("one left, in use: %v", got)
	}
}

func TestRemovingTheSpeechModelWaitsForTheTranscription(t *testing.T) {
	svc, _, home := library(t)
	model := engine.SpeechModels()[0]
	folder := filepath.Join(home, ".framefairy", "models", model.Name)
	if err := os.MkdirAll(folder, 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(folder, "tokens.txt"), []byte("x"), 0o644); err != nil {
		t.Fatal(err)
	}
	release := make(chan struct{})
	svc.jobs.add("", "transcribe", "Transcribe", func(ctx context.Context, p *engine.Project) (string, error) {
		<-release
		return "", nil
	})
	if err := svc.RemoveSpeechModel(model.Name); err == nil {
		t.Error("removed while an episode was being transcribed")
	}
	close(release)
	for svc.busyWith("transcribe") {
		time.Sleep(time.Millisecond)
	}
	if err := svc.RemoveSpeechModel(model.Name); err != nil {
		t.Fatal(err)
	}
	if model.Installed(filepath.Join(home, ".framefairy", "models")) {
		t.Error("still installed")
	}
}
