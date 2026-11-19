package main

import (
	"context"
	"runtime"
	"testing"

	"framefairy/engine"
)

// How a choice of model stands is Go's to judge, case by case, and the
// settings only word it, #610.
func TestHowAChoiceOfModelStands(t *testing.T) {
	ok, missing := Check{OK: true, Detail: "/models/gemma.gguf"}, Check{Detail: "/models/gone.gguf"}
	for _, c := range []struct {
		name string
		got  Standing
		want Standing
	}{
		{"a cloud model with its key", cloudStanding(true), Standing{State: "ok", Says: "cloud"}},
		{"a cloud model without its key", cloudStanding(false), Standing{State: "warn", Says: "cloud"}},
		{"a model here not downloaded", hereStanding(false, true, missing, true), Standing{State: "warn", Says: "fetch"}},
		{"a model here, and llama-server", hereStanding(true, true, ok, true), Standing{State: "ok", Says: "ready"}},
		{"a model here, no llama-server", hereStanding(true, false, ok, false), Standing{State: "err", Says: "ready"}},
		// A model named by hand whose file is gone: the check says so.
		{"the model in use, its file gone", hereStanding(true, true, missing, true),
			Standing{State: "err", Says: "problem", Detail: "/models/gone.gguf"}},
		// The check is made for the model in use only, so another model
		// that is here is judged by being here.
		{"another model here, the one in use broken", hereStanding(true, false, missing, true), Standing{State: "ok", Says: "ready"}},
		{"the speech model, here", speechStanding(Check{OK: true}, false, true), Standing{State: "ok", Says: "ready"}},
		{"the speech model, not downloaded", speechStanding(Check{OK: true}, false, false), Standing{State: "warn", Says: "fetch"}},
		{"the speech model, its folder named", speechStanding(Check{OK: true, Detail: "/speech"}, true, false),
			Standing{State: "ok", Says: "folder", Detail: "/speech"}},
		{"the speech model, not found", speechStanding(Check{Detail: "not found in /speech\nmore"}, true, true),
			Standing{State: "err", Says: "problem", Detail: "not found in /speech"}},
	} {
		if c.got != c.want {
			t.Errorf("%s: %+v, want %+v", c.name, c.got, c.want)
		}
	}
}

// Setup says how each choice stands, from the machine as it is: a model
// not downloaded is a step away, a company without its key too, and the
// speech model in use is the one that is here.
func TestSetupSaysHowEachChoiceStands(t *testing.T) {
	if runtime.GOOS == "windows" {
		t.Skip("the search path works differently enough to need its own test")
	}
	s := emptyMachine(t)
	if err := s.ChoosePlanner("local"); err != nil {
		t.Fatal(err)
	}
	speech := engine.SpeechModels()[0].Name
	placeSpeechModel(t, speech)
	here := engine.LanguageModels()[0].Name
	placeLanguageModel(t, here)
	placeLlamaServer(t)
	t.Setenv("ANTHROPIC_API_KEY", "sk-ant-not-a-real-key")
	t.Setenv("OPENAI_API_KEY", "")

	state := s.Setup(context.Background())
	for _, m := range state.Language {
		want := Standing{State: "warn", Says: "fetch"}
		if m.Name == here {
			want = Standing{State: "ok", Says: "ready"}
		}
		if m.Standing != want {
			t.Errorf("%s stands %+v, want %+v", m.Name, m.Standing, want)
		}
	}
	if got := state.CloudStanding["anthropic"]; got != (Standing{State: "ok", Says: "cloud"}) {
		t.Errorf("Anthropic, with its key, stands %+v", got)
	}
	if got := state.CloudStanding["openai"]; got != (Standing{State: "warn", Says: "cloud"}) {
		t.Errorf("OpenAI, without its key, stands %+v", got)
	}
	for _, m := range state.Speech {
		if m.InUse != (m.Name == speech) {
			t.Errorf("%s is in use: %v", m.Name, m.InUse)
		}
	}
	if state.Speaking != (Standing{State: "ok", Says: "ready"}) {
		t.Errorf("the speech model that is here stands %+v", state.Speaking)
	}
}

// The walks' machine counts its stand-in models as there after Setup
// judged the real one, so it judges them again: the model in use stands
// ready, and the settings let the app go. Judged only once, the model in
// use stood not downloaded, the settings held the app, and every walk
// that left the settings waited for good.
func TestTheStandInsStandReady(t *testing.T) {
	s := emptyMachine(t)
	state := standInsSetup(s.Setup(context.Background()), nil)
	for _, m := range state.Language {
		if m.InUse && m.Standing != (Standing{State: "ok", Says: "ready"}) {
			t.Errorf("the stand-in in use, %s, stands %+v", m.Name, m.Standing)
		}
	}
	if state.Speaking != (Standing{State: "ok", Says: "ready"}) {
		t.Errorf("the stand-in speech model stands %+v", state.Speaking)
	}
}
