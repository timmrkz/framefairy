package main

import (
	"context"
	"encoding/json"
	"os"
	"path/filepath"
	"strings"
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

	// One not downloaded yet can be chosen too. It is the one chosen, the
	// list says it is not here, and the check says what is missing.
	later := models[1]
	if _, err := svc.UseLanguageModel(later.Name); err != nil {
		t.Fatalf("use before download: %v", err)
	}
	if got := inUse(); len(got) != 1 || got[0] != later.Name {
		t.Errorf("chosen before download, in use: %v", got)
	}
	for _, m := range svc.Setup(context.Background()).Language {
		if m.Name == later.Name && m.Installed {
			t.Error("a model chosen before download reads as installed")
		}
	}
	var lm Check
	for _, c := range svc.CheckSetup(context.Background()) {
		if c.Name == "Language model" {
			lm = c
		}
	}
	if lm.OK || !strings.Contains(lm.Detail, "not downloaded yet") {
		t.Errorf("check for a model not downloaded: %+v", lm)
	}
	if _, err := svc.UseLanguageModel(second.Name); err != nil {
		t.Fatal(err)
	}

	// Not while clips are being found: the search may be reading it.
	release := make(chan struct{})
	svc.jobs.add("", engine.JobSearch, "Find clips", func(ctx context.Context, p *engine.Project) (string, error) {
		<-release
		return "", nil
	})
	if err := svc.RemoveLanguageModel(second.Name); err == nil {
		t.Error("removed while clips were being found")
	}
	close(release)
	for svc.busyWith(engine.JobSearch) {
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
	svc.jobs.add("", engine.JobClip, "Make a clip", func(ctx context.Context, p *engine.Project) (string, error) {
		<-release
		return "", nil
	})
	if err := svc.RemoveSpeechModel(model.Name); err == nil {
		t.Error("removed while an episode was being transcribed")
	}
	close(release)
	for svc.busyWith(engine.JobClip) {
		time.Sleep(time.Millisecond)
	}
	if err := svc.RemoveSpeechModel(model.Name); err != nil {
		t.Fatal(err)
	}
	if model.Installed(filepath.Join(home, ".framefairy", "models")) {
		t.Error("still installed")
	}
}

// A second model arriving keeps the first in use, so the download does not
// leave two models and a search that cannot start.
func TestAnInstallKeepsTheModelInUse(t *testing.T) {
	svc, _, home := library(t)
	dir := filepath.Join(home, ".framefairy", "models")
	if err := os.MkdirAll(dir, 0o755); err != nil {
		t.Fatal(err)
	}
	models := engine.LanguageModels()
	first, second := models[0], models[len(models)-1]
	if err := os.WriteFile(filepath.Join(dir, first.Name), []byte("GGUF"), 0o644); err != nil {
		t.Fatal(err)
	}
	// What InstallLanguageModel reads before it starts: the only model
	// there, in use by being the only one.
	before := modelInUse(svc.store.Settings())
	if before != first.Name {
		t.Fatalf("in use before the install: %q", before)
	}
	if err := os.WriteFile(filepath.Join(dir, second.Name), []byte("GGUF"), 0o644); err != nil {
		t.Fatal(err)
	}
	// The check at the top of the settings says so in the app's words.
	checks := svc.CheckSetup(context.Background())
	for _, c := range checks {
		if c.Name == "Language model" && (c.OK || !strings.Contains(c.Detail, "none is in use")) {
			t.Errorf("two and none in use: %+v", c)
		}
		if strings.Contains(c.Detail, "--llm-model") {
			t.Errorf("a flag in the app: %s", c.Detail)
		}
	}
	if err := svc.keepInUse(before); err != nil {
		t.Fatal(err)
	}
	if got := modelInUse(svc.store.Settings()); got != first.Name {
		t.Errorf("after the install, in use: %q", got)
	}
	// A model chosen by hand is never overridden.
	if _, err := svc.UseLanguageModel(second.Name); err != nil {
		t.Fatal(err)
	}
	if err := svc.keepInUse(first.Name); err != nil {
		t.Fatal(err)
	}
	if got := modelInUse(svc.store.Settings()); got != second.Name {
		t.Errorf("a choice made by hand went: %q", got)
	}
}

// From the moment a search starts, hearing included, what it uses stays
// as it is: another model, another way of finding clips, another key,
// another speech model, and removing either model, are all refused, and
// the settings stay what they were. Tim chose another model in the
// settings while a search ran, and nothing stopped him. A clip made by
// hand hears with the speech model, so it holds that one and nothing else.
func TestNothingASearchUsesChangesUnderIt(t *testing.T) {
	svc, _, home := library(t)
	dir := filepath.Join(home, ".framefairy", "models")
	if err := os.MkdirAll(dir, 0o755); err != nil {
		t.Fatal(err)
	}
	models := engine.LanguageModels()
	first, second := models[0], models[len(models)-1]
	for _, m := range []engine.LanguageModel{first, second} {
		if err := os.WriteFile(filepath.Join(dir, m.Name), []byte("GGUF"), 0o644); err != nil {
			t.Fatal(err)
		}
	}
	if _, err := svc.UseLanguageModel(first.Name); err != nil {
		t.Fatal(err)
	}
	speech := engine.SpeechModels()[0]
	if err := os.MkdirAll(filepath.Join(dir, speech.Name), 0o755); err != nil {
		t.Fatal(err)
	}
	before := svc.store.Settings()

	hold := func(kind string) func() {
		release := make(chan struct{})
		svc.jobs.add("", kind, "Held", func(ctx context.Context, p *engine.Project) (string, error) {
			<-release
			return "", nil
		})
		return func() {
			close(release)
			for svc.busyWith(kind) {
				time.Sleep(time.Millisecond)
			}
		}
	}
	type change struct {
		what string
		do   func() error
	}
	language := []change{
		{"use another model", func() error { _, err := svc.UseLanguageModel(second.Name); return err }},
		{"remove a model", func() error { return svc.RemoveLanguageModel(second.Name) }},
		{"choose a model in the cloud", func() error { return svc.ChooseCloudModel("another-model") }},
		{"choose the cloud", func() error { return svc.ChoosePlanner("api") }},
		{"save the cloud", func() error { return svc.SaveSettings(raw(t, map[string]any{"planner": "api"})) }},
		{"save another model", func() error {
			return svc.SaveSettings(raw(t, map[string]any{"llmModel": filepath.Join(dir, second.Name)}))
		}},
		{"save a key", func() error { return svc.SaveAPIKey("anthropic", "") }},
	}
	hearing := []change{
		{"remove the speech model", func() error { return svc.RemoveSpeechModel(speech.Name) }},
		{"save another speech model", func() error {
			return svc.SaveSettings(raw(t, map[string]any{"asrModel": filepath.Join(dir, "other")}))
		}},
	}

	done := hold(engine.JobSearch)
	for _, c := range append(language, hearing...) {
		if err := c.do(); err == nil {
			t.Errorf("while a search runs, %s was not refused", c.what)
		}
	}
	// What does not change a model goes on as before.
	if err := svc.SaveSettings(raw(t, map[string]any{"min": 22})); err != nil {
		t.Errorf("while a search runs, the shortest clip could not be saved: %v", err)
	}
	if _, err := svc.UseLanguageModel(first.Name); err != nil {
		t.Errorf("while a search runs, the model it uses could not be chosen again: %v", err)
	}
	after := svc.store.Settings()
	if after.LLMModel != before.LLMModel || after.Planner != before.Planner ||
		after.APIModel != before.APIModel || after.ASRModel != before.ASRModel {
		t.Errorf("the settings changed under a search: %+v, were %+v", after, before)
	}
	if _, err := os.Stat(filepath.Join(dir, second.Name)); err != nil {
		t.Error("a language model was removed under a search")
	}
	done()

	done = hold(engine.JobClip)
	for _, c := range hearing {
		if err := c.do(); err == nil {
			t.Errorf("while a clip made by hand hears, %s was not refused", c.what)
		}
	}
	if _, err := svc.UseLanguageModel(second.Name); err != nil {
		t.Errorf("a clip made by hand held the language model: %v", err)
	}
	done()
}

// raw is a change to the settings the way the interface sends one.
func raw(t *testing.T, changed map[string]any) map[string]json.RawMessage {
	t.Helper()
	out := map[string]json.RawMessage{}
	for k, v := range changed {
		b, err := json.Marshal(v)
		if err != nil {
			t.Fatal(err)
		}
		out[k] = b
	}
	return out
}
