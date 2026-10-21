package main

import (
	"strings"
	"testing"

	"github.com/spf13/afero"
)

// Any change of the settings asks for the model to be kept again, so a
// model chosen, or the API chosen in its place, is kept or given up
// without a restart. Asks that pile up while one is worked on are one.
func TestAChangeOfTheSettingsKeepsTheModelAgain(t *testing.T) {
	t.Parallel()
	st := openStoreIn(afero.NewMemMapFs(), "/config")
	k := newKeeping()
	st.changed = k.ask
	for _, planner := range []string{"local", "api"} {
		if err := st.UpdateSettings(func(s *Settings) { s.Planner = planner }); err != nil {
			t.Fatal(err)
		}
	}
	<-k.wake
	select {
	case <-k.wake:
		t.Error("two changes asked twice")
	default:
	}
	// An app made without it, as the tests make it, asks nothing.
	var none *keeping
	none.ask()
}

// The keeper says why it keeps no model, and the reason it returns is what
// keeps it from saying the same again after every search. The API needs
// no model, so there is nothing to say.
func TestTheKeeperSaysWhyItKeepsNoModel(t *testing.T) {
	t.Parallel()
	st := openStoreIn(afero.NewMemMapFs(), "/config")
	if err := st.UpdateSettings(func(s *Settings) {
		s.Planner = "local"
		s.LLMModel = "/nowhere/gemma.gguf"
	}); err != nil {
		t.Fatal(err)
	}
	k := newKeeping()
	said := k.keep(st, "")
	if !strings.Contains(said, "/nowhere/gemma.gguf") {
		t.Errorf("a model that is not there was kept, or said as %q", said)
	}
	if again := k.keep(st, said); again != said {
		t.Errorf("the same reason came back as %q", again)
	}
	if err := st.UpdateSettings(func(s *Settings) { s.Planner = "api" }); err != nil {
		t.Fatal(err)
	}
	if said := k.keep(st, said); said != "" {
		t.Errorf("with the API there is a reason to keep no model: %q", said)
	}
}
