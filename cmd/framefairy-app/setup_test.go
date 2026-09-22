package main

import (
	"context"
	"os"
	"path/filepath"
	"strings"
	"sync"
	"testing"

	"framefairy/engine"
)

// A new copy of the app on a machine with nothing on it. Everything here
// turns on the state a customer opens the app in for the first time, which
// is the one state nobody who has the repository ever sees.
func emptyMachine(t *testing.T) *FrameFairy {
	t.Helper()
	home := t.TempDir()
	t.Setenv("HOME", home)
	t.Setenv("XDG_CONFIG_HOME", filepath.Join(home, "config"))
	// The key lookup reads the environment first, so an empty one is what
	// makes the test about a machine with no key rather than about
	// whatever is on this one.
	t.Setenv("ANTHROPIC_API_KEY", "")
	st := openStore()
	return &FrameFairy{store: st, jobs: newQueue(st, func(JobUpdate) {}, func(string) {})}
}

// Pretends a speech model is installed, by putting there what Installed
// looks at.
func placeSpeechModel(t *testing.T, name string) {
	t.Helper()
	dir := filepath.Join(engine.ModelsDir(), name)
	if err := os.MkdirAll(dir, 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(dir, "tokens.txt"), []byte("a 0\n"), 0o644); err != nil {
		t.Fatal(err)
	}
}

func TestANewCopyOfTheAppKnowsWhatItStillNeeds(t *testing.T) {
	t.Run("nothing installed and nothing chosen", func(t *testing.T) {
		s := emptyMachine(t)
		state := s.Setup(context.Background())
		if state.HasSpeech {
			t.Error("said a speech model was there on an empty machine")
		}
		if state.Chosen {
			t.Error("said the question had been answered before it was asked")
		}
		if state.Ready {
			t.Error("said it was ready to make a short with nothing installed")
		}
		if len(state.Speech) == 0 {
			t.Fatal("offered no speech model, so the window has nothing to show")
		}
		// The window shows these before asking anybody to agree to a
		// download, so they have to survive the trip.
		first := state.Speech[0]
		if first.Title == "" || first.Download <= 0 {
			t.Errorf("a model the window cannot describe: %+v", first)
		}
	})

	t.Run("a speech model alone is not ready", func(t *testing.T) {
		s := emptyMachine(t)
		placeSpeechModel(t, engine.SpeechModels()[0].Name)
		state := s.Setup(context.Background())
		if !state.HasSpeech {
			t.Fatal("did not see the model that is there")
		}
		if state.Ready {
			t.Error("ready without any way to find clips")
		}
	})

	// Choosing the API and having a key is one whole answer. Choosing it
	// without a key is not, and saying it is would send somebody to an
	// episode that fails after transcribing the lot.
	t.Run("the api needs a key", func(t *testing.T) {
		s := emptyMachine(t)
		placeSpeechModel(t, engine.SpeechModels()[0].Name)
		if err := s.ChoosePlanner("api"); err != nil {
			t.Fatal(err)
		}
		if state := s.Setup(context.Background()); state.Ready {
			t.Error("ready to call the API with no key")
		}
		t.Setenv("ANTHROPIC_API_KEY", "sk-ant-testtesttest")
		state := s.Setup(context.Background())
		if !state.HasKey {
			t.Fatal("did not see the key")
		}
		if !state.Ready {
			t.Error("a speech model, the api chosen and a key, and still not ready")
		}
	})

	t.Run("local needs a model on the machine", func(t *testing.T) {
		s := emptyMachine(t)
		placeSpeechModel(t, engine.SpeechModels()[0].Name)
		if err := s.ChoosePlanner("local"); err != nil {
			t.Fatal(err)
		}
		if state := s.Setup(context.Background()); state.Ready {
			t.Error("ready to run a local model that is not there")
		}
		gguf := filepath.Join(engine.ModelsDir(), "something-q4.gguf")
		if err := os.WriteFile(gguf, []byte("not really a model"), 0o644); err != nil {
			t.Fatal(err)
		}
		state := s.Setup(context.Background())
		if !state.HasLocalModel {
			t.Fatal("did not see the model file")
		}
		if !state.Ready {
			t.Error("a speech model, local chosen and a model file, and still not ready")
		}
	})

	// A key that is there does not mean the question was answered. Without
	// this the app would decide for somebody who has a key lying about in
	// their environment for something else.
	t.Run("a key is not an answer on its own", func(t *testing.T) {
		s := emptyMachine(t)
		placeSpeechModel(t, engine.SpeechModels()[0].Name)
		t.Setenv("ANTHROPIC_API_KEY", "sk-ant-testtesttest")
		state := s.Setup(context.Background())
		if state.Ready {
			t.Error("chose the API on the customer's behalf")
		}
	})

	t.Run("an answer is remembered", func(t *testing.T) {
		s := emptyMachine(t)
		if err := s.ChoosePlanner("local"); err != nil {
			t.Fatal(err)
		}
		state := s.Setup(context.Background())
		if !state.Chosen || state.Planner != "local" {
			t.Errorf("chosen %v, planner %q", state.Chosen, state.Planner)
		}
	})

	t.Run("a planner nobody offers is refused", func(t *testing.T) {
		s := emptyMachine(t)
		if err := s.ChoosePlanner("magic"); err == nil {
			t.Error("took a planner that does not exist")
		}
		if state := s.Setup(context.Background()); state.Chosen {
			t.Error("and recorded it as an answer")
		}
	})
}

func TestInstallingTheSpeechModelIsAJobLikeAnyOther(t *testing.T) {
	name := engine.SpeechModels()[0].Name

	t.Run("a model nobody has heard of fails as a job", func(t *testing.T) {
		s := emptyMachine(t)
		job := s.InstallSpeechModel("no-such-model")
		if job.State != JobFailed {
			t.Errorf("state %q", job.State)
		}
		if job.Error == "" {
			t.Error("failed without saying why")
		}
	})

	t.Run("one already installed is not fetched again", func(t *testing.T) {
		s := emptyMachine(t)
		placeSpeechModel(t, name)
		job := s.InstallSpeechModel(name)
		if job.State != JobFailed {
			t.Errorf("started a download for a model that is already here: %q", job.State)
		}
	})

	// The queue runs a model install on the transcribe lane, so a
	// transcription queued behind it waits for the model rather than
	// failing on it, and finding clips in an episode that already has a
	// transcript carries on in the other lane.
	t.Run("it shares the lane with transcribing", func(t *testing.T) {
		if got := laneFor("model"); got != LaneTranscribe {
			t.Errorf("a model install runs in the %q lane", got)
		}
		if got := laneFor("plan"); got != LaneWork {
			t.Errorf("finding clips runs in the %q lane", got)
		}
	})

	// The window can be asked twice by being opened twice, and two installs
	// of one model would write over each other's unpacking folder.
	t.Run("two asks at once are one install", func(t *testing.T) {
		s := emptyMachine(t)
		var wg sync.WaitGroup
		ids := make([]string, 8)
		for i := range ids {
			wg.Add(1)
			go func(i int) {
				defer wg.Done()
				ids[i] = s.InstallSpeechModel(name).ID
			}(i)
		}
		wg.Wait()
		running := 0
		for _, job := range s.jobs.list() {
			if job.Kind == "model" && (job.State == JobQueued || job.State == JobRunning) {
				running++
			}
		}
		if running > 1 {
			t.Errorf("%d installs of the same model at once", running)
		}
		for _, job := range s.jobs.list() {
			if job.Kind == "model" && job.State == JobQueued || job.State == JobRunning {
				s.jobs.cancel(job.ID)
			}
		}
	})
}

// The key never goes in the settings, which are plain JSON in the config
// folder and get copied about. On a machine with no keychain saving fails,
// and the point of the test is that nothing was written either way.
func TestTheKeyNeverLandsInTheSettingsFile(t *testing.T) {
	const secret = "sk-ant-secret-value-here"
	s := emptyMachine(t)
	_ = s.SaveAPIKey(secret)
	_ = s.ChoosePlanner("api")

	body, err := os.ReadFile(filepath.Join(s.store.dir, "settings.json"))
	if err != nil {
		t.Skip("no settings file was written")
	}
	if strings.Contains(string(body), secret) {
		t.Errorf("the key is in %s", filepath.Join(s.store.dir, "settings.json"))
	}
	// And the state the window reads never carries it either.
	state := s.Setup(context.Background())
	if strings.Contains(state.Planner, secret) {
		t.Error("the key came back in the setup state")
	}
}

// The window hands the whole settings object back on every save, and it
// does not know about Chosen. Without care that turns every colour change
// into another round of the setup screen.
func TestSavingSettingsKeepsTheAnswer(t *testing.T) {
	s := emptyMachine(t)
	if err := s.ChoosePlanner("api"); err != nil {
		t.Fatal(err)
	}
	// What a window that has never heard of Chosen would send.
	set := s.store.Settings()
	set.Chosen = false
	set.AppColour = "#112233"
	if err := s.SaveSettings(set); err != nil {
		t.Fatal(err)
	}
	after := s.store.Settings()
	if !after.Chosen {
		t.Error("a colour change took the setup answer with it")
	}
	if after.AppColour != "#112233" {
		t.Errorf("the colour did not save: %q", after.AppColour)
	}
}
