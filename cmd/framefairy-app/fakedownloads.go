package main

// TEMPORARY. This file comes out before the pull request is merged.
//
// On a build of a pull request, a model this Mac already has is not
// fetched from the internet again. The setup can then be stepped through
// as a new customer sees it, again and again, without waiting half an
// hour for the language model each time.
//
// When the build starts, every model in the models folder gets a hard
// link in a folder beside it, which takes no room on the disk. Removing a
// model in the settings then leaves that link, and installing the model
// again puts it back from there over a few seconds, with the same fill
// and the same words as a download. What is put back is the real model,
// so the app works afterwards. A model that was never on this Mac is
// downloaded for real, and kept the same way once it has arrived.

import (
	"context"
	"fmt"
	"io/fs"
	"os"
	"path/filepath"
	"strings"
	"time"

	"framefairy/engine"
)

// fakeTime is how long a model takes to arrive from the kept copy.
var fakeTime = 8 * time.Second

func init() {
	if !strings.Contains(runningVersion(), "-pr") {
		return
	}
	kept := filepath.Join(filepath.Dir(engine.ModelsDir()), "kept-for-setup")
	if err := keepModels(engine.ModelsDir(), kept); err != nil {
		fmt.Fprintf(os.Stderr, "keeping the models for the setup: %v\n", err)
	}
	installSpeech = fakeSpeech(kept, installSpeech)
	installLanguage = fakeLanguage(kept, installLanguage)
}

// keepModels links every model in models into kept that kept does not
// have yet.
func keepModels(models, kept string) error {
	if err := os.MkdirAll(kept, 0o755); err != nil {
		return err
	}
	for _, m := range engine.SpeechModels() {
		if m.Installed(models) && !m.Installed(kept) {
			if err := linkInto(filepath.Join(models, m.Name), kept, m.Name); err != nil {
				return err
			}
		}
	}
	for _, m := range engine.LanguageModels() {
		if m.Installed(models) && !m.Installed(kept) {
			if err := linkInto(filepath.Join(models, m.Name), kept, m.Name); err != nil {
				return err
			}
		}
	}
	return nil
}

// fakeSpeech installs a speech model from kept when it is there, and
// with real otherwise, keeping what real brought.
func fakeSpeech(kept string, real func(context.Context, *engine.Log, engine.SpeechModel, string) error) func(context.Context, *engine.Log, engine.SpeechModel, string) error {
	return func(ctx context.Context, log *engine.Log, m engine.SpeechModel, dir string) error {
		if !m.Installed(kept) {
			if err := real(ctx, log, m, dir); err != nil {
				return err
			}
			return keepModels(dir, kept)
		}
		return log.Step("speech model", func() error {
			return arrive(ctx, log, m.Download, filepath.Join(kept, m.Name), dir, m.Name)
		})
	}
}

// fakeLanguage is the same for a model that finds clips.
func fakeLanguage(kept string, real func(context.Context, *engine.Log, engine.LanguageModel, string) error) func(context.Context, *engine.Log, engine.LanguageModel, string) error {
	return func(ctx context.Context, log *engine.Log, m engine.LanguageModel, dir string) error {
		if !m.Installed(kept) {
			if err := real(ctx, log, m, dir); err != nil {
				return err
			}
			return keepModels(dir, kept)
		}
		return log.Step("language model", func() error {
			return arrive(ctx, log, m.Download, filepath.Join(kept, m.Name), dir, m.Name)
		})
	}
}

// arrive fills the way a download does for fakeTime, then links from
// into dir under name. Cancel stops it with nothing put in place.
func arrive(ctx context.Context, log *engine.Log, size int64, from, dir, name string) error {
	began := time.Now()
	tick := time.NewTicker(250 * time.Millisecond)
	defer tick.Stop()
	for time.Since(began) < fakeTime {
		share := float64(time.Since(began)) / float64(fakeTime)
		left := (fakeTime - time.Since(began)).Seconds()
		rate := float64(size) / fakeTime.Seconds()
		log.ProgressOf(fmt.Sprintf("fetching %d MB of %d MB at %d MB a second",
			int64(share*float64(size))/1e6, size/1e6, int64(rate)/1e6), share, left)
		select {
		case <-ctx.Done():
			return engine.ErrCancelled
		case <-tick.C:
		}
	}
	if ctx.Err() != nil {
		return engine.ErrCancelled
	}
	if err := os.MkdirAll(dir, 0o755); err != nil {
		return err
	}
	if err := linkInto(from, dir, name); err != nil {
		return err
	}
	log.ClearProgress()
	return nil
}

// linkInto hard links from, a file or a folder, into dir under name. It is
// built under a name of its own and moved into place whole, the way a real
// install is, so a model is never there half.
func linkInto(from, dir, name string) error {
	staging := filepath.Join(dir, name+".linking")
	if err := os.RemoveAll(staging); err != nil {
		return err
	}
	err := filepath.WalkDir(from, func(path string, d fs.DirEntry, err error) error {
		if err != nil {
			return err
		}
		rel, err := filepath.Rel(from, path)
		if err != nil {
			return err
		}
		to := filepath.Join(staging, rel)
		if d.IsDir() {
			return os.MkdirAll(to, 0o755)
		}
		return os.Link(path, to)
	})
	if err != nil {
		os.RemoveAll(staging)
		return err
	}
	return os.Rename(staging, filepath.Join(dir, name))
}
