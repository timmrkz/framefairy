package engine

import (
	"errors"
	"fmt"
	"strings"
	"testing"
)

// A missing model, or memory a model needs, is said in the app with the
// settings as the way on, never with a command to run or a flag to pass,
// which the person in the app has nowhere to type. The command line says
// its own way on, issue #411.
func TestAMissingModelSaysTheWayOnOfEachFrontEnd(t *testing.T) {
	t.Parallel()
	_, none := onlyModel("/home/tim/.framefairy/models", nil)
	_, several := onlyModel("/home/tim/.framefairy/models",
		[]string{"/home/tim/.framefairy/models/a.gguf", "/home/tim/.framefairy/models/b.gguf"})
	cases := []struct {
		name      string
		err       error
		app, line string
	}{
		{"no speech model", noSpeechModel("/nowhere/sherpa",
			errors.New("the speech model folder /nowhere/sherpa is missing [tokens.txt]")),
			"Download it in the settings, under Speech model.", "--asr-model"},
		{"no clip model", none, "Download one in the settings, under Clip model.", "--llm-model"},
		{"several clip models", several, "Choose one in the settings, under Clip model.", "--llm-model"},
		{"killed while loading", stoppedLoading("llama-server", errors.New("signal: killed"), "/home/tim/logs"),
			"choose a smaller clip model in the settings", "pgrep -fl llama-server"},
		{"another server holds memory", tooLittleMemory(2e9, 1e9, []string{"4534"}),
			"Quit the program that started it.", "kill 4534"},
		{"too little memory", tooLittleMemory(2e9, 1e9, nil),
			"choose a smaller clip model in the settings", "choose a smaller model"},
	}
	commands := []string{"--", "kill ", "pgrep", "curl", "mkdir", "tar ", "docs/"}
	for _, c := range cases {
		said := c.err.Error()
		if !strings.Contains(said, c.app) {
			t.Errorf("%s: the app says %q, without %q", c.name, said, c.app)
		}
		for _, command := range commands {
			if strings.Contains(said, command) {
				t.Errorf("%s: the app says %q, which has %q in it", c.name, said, command)
			}
		}
		// Wrapped the way a failed step is, the command line still says
		// its own way on, in the same place.
		line := ForCommandLine(fmt.Errorf("planning failed: %w", c.err))
		if !strings.HasPrefix(line, "planning failed: ") || !strings.Contains(line, c.line) ||
			strings.Contains(line, "in the settings") {
			t.Errorf("%s: the command line says %q, want %q and no settings", c.name, line, c.line)
		}
	}
	// What has no way of its own is said the same in both.
	plain := errors.New("planning failed: x")
	if ForCommandLine(plain) != plain.Error() {
		t.Errorf("a plain failure on the command line is %q", ForCommandLine(plain))
	}
}
