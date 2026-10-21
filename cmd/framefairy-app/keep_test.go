package main

import (
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
