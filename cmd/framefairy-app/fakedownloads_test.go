package main

// TEMPORARY, with fakedownloads.go.

import (
	"context"
	"errors"
	"io"
	"os"
	"path/filepath"
	"testing"

	"framefairy/engine"
)

// On the real disk: what this is about is hard links, which only a real
// disk has. It changes fakeTime, so it does not run beside other tests.
func TestFakeDownloadsPutBackTheKeptModels(t *testing.T) {
	was := fakeTime
	fakeTime = 0
	t.Cleanup(func() { fakeTime = was })
	models, kept := t.TempDir(), t.TempDir()
	speech, language := engine.SpeechModels()[0], engine.LanguageModels()[0]
	write(t, filepath.Join(models, speech.Name, "tokens.txt"), "tokens")
	write(t, filepath.Join(models, speech.Name, "encoder.onnx"), "weights")
	write(t, filepath.Join(models, language.Name), "GGUF and the rest")

	if err := keepModels(models, kept); err != nil {
		t.Fatal(err)
	}
	// Removed the way the settings remove them.
	if err := engine.RemoveSpeechModel(speech, models); err != nil {
		t.Fatal(err)
	}
	if err := engine.RemoveLanguageModel(language, models); err != nil {
		t.Fatal(err)
	}
	if speech.Installed(models) || language.Installed(models) {
		t.Fatal("the models are still there after they were removed")
	}

	never := func(name string) error { return errors.New(name + " was downloaded although it was kept") }
	log := engine.NewLog(io.Discard, false, false)
	installSpeech := fakeSpeech(kept, func(context.Context, *engine.Log, engine.SpeechModel, string) error { return never("speech") })
	installLanguage := fakeLanguage(kept, func(context.Context, *engine.Log, engine.LanguageModel, string) error { return never("language") })
	if err := installSpeech(t.Context(), log, speech, models); err != nil {
		t.Fatal(err)
	}
	if err := installLanguage(t.Context(), log, language, models); err != nil {
		t.Fatal(err)
	}
	if !speech.Installed(models) || !language.Installed(models) {
		t.Fatal("the models did not come back")
	}
	same(t, filepath.Join(models, speech.Name, "encoder.onnx"), filepath.Join(kept, speech.Name, "encoder.onnx"))
	same(t, filepath.Join(models, language.Name), filepath.Join(kept, language.Name))
}

// On the real disk: hard links, as above. It changes fakeTime.
func TestFakeDownloadsCancelledPutNothingInPlace(t *testing.T) {
	was := fakeTime
	fakeTime = 0
	t.Cleanup(func() { fakeTime = was })
	models, kept := t.TempDir(), t.TempDir()
	language := engine.LanguageModels()[0]
	write(t, filepath.Join(kept, language.Name), "GGUF and the rest")

	ctx, cancel := context.WithCancel(t.Context())
	cancel()
	install := fakeLanguage(kept, nil)
	err := install(ctx, engine.NewLog(io.Discard, false, false), language, models)
	if !errors.Is(err, engine.ErrCancelled) {
		t.Fatalf("a cancelled install ended with %v", err)
	}
	if language.Installed(models) {
		t.Fatal("a cancelled install put the model in place")
	}
}

// On the real disk: hard links, as above. A model that was never kept is
// downloaded for real, and kept once it is there.
func TestFakeDownloadsKeepWhatArrivedForReal(t *testing.T) {
	models, kept := t.TempDir(), t.TempDir()
	speech := engine.SpeechModels()[0]
	real := func(_ context.Context, _ *engine.Log, m engine.SpeechModel, dir string) error {
		write(t, filepath.Join(dir, m.Name, "tokens.txt"), "tokens")
		return nil
	}
	install := fakeSpeech(kept, real)
	if err := install(t.Context(), engine.NewLog(io.Discard, false, false), speech, models); err != nil {
		t.Fatal(err)
	}
	if !speech.Installed(kept) {
		t.Fatal("a model that arrived for real was not kept")
	}
}

// On the real disk: the files the tests above link, which only a real
// disk can do.
func write(t *testing.T, path, body string) {
	t.Helper()
	if err := os.MkdirAll(filepath.Dir(path), 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(path, []byte(body), 0o644); err != nil {
		t.Fatal(err)
	}
}

// On the real disk: whether two names are one file, a hard link, which
// only a real disk has.
func same(t *testing.T, a, b string) {
	t.Helper()
	ia, err := os.Stat(a)
	if err != nil {
		t.Fatal(err)
	}
	ib, err := os.Stat(b)
	if err != nil {
		t.Fatal(err)
	}
	if !os.SameFile(ia, ib) {
		t.Fatalf("%s is a copy of %s, not a link to it", a, b)
	}
}
