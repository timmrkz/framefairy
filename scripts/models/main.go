// Command models downloads the speech model and the language model into
// ~/.framefairy/models, where the programs look for them, for the command
// line, which has no window to ask in. The app fetches its own on its first
// run.
//
//	make models
//
// It goes through the engine's own installers, the ones the app uses, so
// every download is held to the size and the SHA-256 pinned beside the
// model and nothing that fails them is kept. It was a shell script that
// checked nothing, and a file from a branch that could change. A model
// already there is left alone, and a download that stopped carries on.
package main

import (
	"context"
	"fmt"
	"os"
	"os/signal"

	"framefairy/engine"
)

// language is the model fetched for the command line, the one the app
// offers a machine with room for it.
const language = "gemma-4-26B_q4_0-it.gguf"

func main() {
	ctx, stop := signal.NotifyContext(context.Background(), os.Interrupt)
	defer stop()
	log := engine.NewLog(os.Stderr, true, false)
	if err := fetch(ctx, log); err != nil {
		fmt.Fprintln(os.Stderr, "models:", err)
		os.Exit(1)
	}
	fmt.Println("Models are in", engine.ModelsDir())
}

func fetch(ctx context.Context, log *engine.Log) error {
	for _, m := range engine.SpeechModels() {
		if !m.Recommended {
			continue
		}
		if m.Installed("") {
			fmt.Println("Speech model already there")
		} else if err := engine.InstallSpeechModel(ctx, log, m, ""); err != nil {
			return err
		}
	}
	if found := engine.LocalModelFiles(engine.ModelsDir()); len(found) > 0 {
		fmt.Println("Language model already there")
		return nil
	}
	m, ok := engine.LanguageModelByName(language)
	if !ok {
		return fmt.Errorf("%s is not among the language models the engine knows", language)
	}
	fmt.Printf("Downloading %s, %.1f GB.\n", m.Title, float64(m.Download)/1e9)
	return engine.InstallLanguageModel(ctx, log, m, "")
}
