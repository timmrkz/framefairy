package main

import (
	"context"
	"encoding/json"
	"net/url"
	"os"
	"os/exec"
	"path/filepath"
	"runtime"
	"strings"
	"sync"
	"testing"
	"time"

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
	t.Setenv("OPENAI_API_KEY", "")
	// And nothing here may reach the machine's own keychain. Storing a key
	// writes to it, macOS puts a box on screen when it is locked, and a box
	// nobody answers is a test that hangs. That is what happened: the macOS
	// job sat for the full ten minutes a test binary is given, with the
	// security command still waiting when the runner was cleaned up. An
	// empty folder for a search path is the whole of the fix, and the check
	// under it is there so that a test which can reach the keychain again
	// says so instead of hanging again.
	t.Setenv("PATH", filepath.Join(home, "no-tools"))
	if _, err := exec.LookPath("security"); err == nil {
		t.Fatal("this test can still reach the keychain")
	}
	st := openStore()
	// A queue that holds work and never does it. Every test here is about
	// what gets asked for, and asking for a model install really fetches a
	// model: half a gigabyte, from the internet, on every run.
	return &FrameFairy{store: st, jobs: newIdleQueue(st, func(JobUpdate) {}, func(string) {})}
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

// The guard on all of the above. If the queue these tests use ever starts
// running its work, every one of them fetches half a gigabyte from the
// internet and leaves a part file in a folder the test is about to take
// away. That is how a build runner found this the first time.
func TestTheQueueTheseTestsUseNeverRunsAnything(t *testing.T) {
	s := emptyMachine(t)
	ran := make(chan struct{})
	s.jobs.add("", "plan", "Test", func(ctx context.Context, p *engine.Project) (string, error) {
		close(ran)
		return "", nil
	})
	select {
	case <-ran:
		t.Fatal("the queue did the work, so every test in this file fetches a model")
	case <-time.After(300 * time.Millisecond):
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
			t.Fatal("offered no speech model, so the interface has nothing to show")
		}
		// The interface shows these before asking anybody to agree to a
		// download, so they have to survive the trip.
		first := state.Speech[0]
		if first.Title == "" || first.Download <= 0 {
			t.Errorf("a model the interface cannot describe: %+v", first)
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

	// The local way needs both halves: the model that answers and the
	// server that runs it. Neither one alone makes a short.
	t.Run("local needs a model on the machine", func(t *testing.T) {
		if runtime.GOOS == "windows" {
			t.Skip("the search path works differently enough to need its own test")
		}
		s := emptyMachine(t)
		placeSpeechModel(t, engine.SpeechModels()[0].Name)
		placeLlamaServer(t)
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
			t.Error("a speech model, local chosen, a model file and a server, and still not ready")
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
		if got := laneFor("model"); got != LaneHearing {
			t.Errorf("a model install runs in the %q lane", got)
		}
		if got := laneFor("search"); got != LaneFinding {
			t.Errorf("finding clips runs in the %q lane", got)
		}
	})

	// The interface can be asked twice by being opened twice, and two
	// installs of one model would write over each other's unpacking folder.
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
	})
}

// The key never goes in the settings, which are plain JSON in the config
// folder and get copied about. On a machine with no keychain saving fails,
// and the point of the test is that nothing was written either way.
func TestTheKeyNeverLandsInTheSettingsFile(t *testing.T) {
	const secret = "sk-ant-secret-value-here"
	s := emptyMachine(t)
	_ = s.SaveAPIKey("anthropic", secret)
	_ = s.SaveAPIKey("openai", secret)
	_ = s.ChoosePlanner("api")

	body, err := os.ReadFile(filepath.Join(s.store.dir, "settings.json"))
	if err != nil {
		t.Skip("no settings file was written")
	}
	if strings.Contains(string(body), secret) {
		t.Errorf("the key is in %s", filepath.Join(s.store.dir, "settings.json"))
	}
	// And the state the interface reads never carries it either.
	state := s.Setup(context.Background())
	if raw, _ := json.Marshal(state); strings.Contains(string(raw), secret) {
		t.Error("the key came back in the setup state")
	}
}

// A model in the cloud belongs to one company, and it is that company's key
// that decides whether the app is ready, that the check looks for and that
// the check names. Somebody with only one company's key is not told the
// other's model works, and a key is saved under the company it is for.
func TestTheKeyIsTheCloudModelsProviders(t *testing.T) {
	s := emptyMachine(t)
	placeSpeechModel(t, engine.SpeechModels()[0].Name)
	if err := s.ChoosePlanner("api"); err != nil {
		t.Fatal(err)
	}
	t.Setenv("ANTHROPIC_API_KEY", "sk-ant-testtesttest")

	keyCheck := func() Check {
		for _, c := range s.CheckSetup(context.Background()) {
			if strings.HasSuffix(c.Name, "API key") {
				return c
			}
		}
		t.Fatal("no check for a key")
		return Check{}
	}

	// Claude, with an Anthropic key: ready.
	state := s.Setup(context.Background())
	if state.Provider != "anthropic" || !state.HasKey || !state.Ready {
		t.Errorf("claude with its key: %+v", state)
	}
	if c := keyCheck(); c.Name != "Anthropic API key" || !c.OK {
		t.Errorf("claude's key check: %+v", c)
	}

	// GPT, with only the Anthropic key: not ready, and the check says whose
	// key is missing.
	set := s.store.Settings()
	set.APIModel = "gpt-6-sol"
	if err := s.SaveSettings(set); err != nil {
		t.Fatal(err)
	}
	state = s.Setup(context.Background())
	if state.Provider != "openai" || state.HasKey || state.Ready {
		t.Errorf("gpt without its key: provider %s, has key %v, ready %v",
			state.Provider, state.HasKey, state.Ready)
	}
	// The Anthropic key is in the environment here, and the settings say so.
	if state.Keys["anthropic"] != engine.KeyInEnvironment || state.Keys["openai"] != "" {
		t.Errorf("keys: %v", state.Keys)
	}
	if c := keyCheck(); c.Name != "OpenAI API key" || c.OK || !strings.Contains(c.Detail, "OPENAI_API_KEY") {
		t.Errorf("gpt's key check: %+v", c)
	}

	// And with an OpenAI key it is.
	t.Setenv("OPENAI_API_KEY", "sk-proj-testtesttest")
	if state = s.Setup(context.Background()); !state.HasKey || !state.Ready {
		t.Errorf("gpt with its key: has key %v, ready %v", state.HasKey, state.Ready)
	}
	if c := keyCheck(); !c.OK {
		t.Errorf("gpt's key check with the key: %+v", c)
	}

	// Both companies and their models are offered.
	names := map[string]bool{}
	for _, m := range state.Cloud {
		names[m.Provider] = true
	}
	if !names["anthropic"] || !names["openai"] || len(state.Providers) < 2 {
		t.Errorf("offered: %+v, providers %+v", state.Cloud, state.Providers)
	}

	// The first run names the model on its own, without the rest of the
	// settings, and a name that could never be a model's is refused.
	if err := s.ChooseCloudModel("claude-sonnet-5"); err != nil {
		t.Fatal(err)
	}
	if state = s.Setup(context.Background()); state.Provider != "anthropic" || state.APIModel != "claude-sonnet-5" {
		t.Errorf("after choosing claude: %s, %s", state.Provider, state.APIModel)
	}
	for _, bad := range []string{"", "  ", "gpt 5", "../gpt", "gpt\n5"} {
		if err := s.ChooseCloudModel(bad); err == nil {
			t.Errorf("%q was taken as a model", bad)
		}
	}
	if err := s.SaveAPIKey("nobody", "sk-x"); err == nil {
		t.Error("a key was taken for a company that is not on the list")
	}
}

// The interface hands the whole settings object back on every save, and it
// does not know about Chosen. Without care that turns every colour change
// into another round of the setup screen.
func TestSavingSettingsKeepsTheAnswer(t *testing.T) {
	s := emptyMachine(t)
	if err := s.ChoosePlanner("api"); err != nil {
		t.Fatal(err)
	}
	// What an interface that has never heard of Chosen would send.
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

// Pretends a language model is installed, by putting there what Installed
// looks at: the four bytes that tell a model from a page saying no.
func placeLanguageModel(t *testing.T, name string) {
	t.Helper()
	if err := os.MkdirAll(engine.ModelsDir(), 0o755); err != nil {
		t.Fatal(err)
	}
	body := append([]byte("GGUF"), make([]byte, 64)...)
	if err := os.WriteFile(filepath.Join(engine.ModelsDir(), name), body, 0o644); err != nil {
		t.Fatal(err)
	}
}

func TestTheWindowIsToldWhatThisMachineCanHold(t *testing.T) {
	t.Run("every model says whether it fits", func(t *testing.T) {
		s := emptyMachine(t)
		state := s.Setup(context.Background())
		if len(state.Language) == 0 {
			t.Fatal("offered no language model, so the interface has nothing to show")
		}
		for _, m := range state.Language {
			if m.Title == "" || m.Maker == "" || m.Download <= 0 || m.Needs <= 0 {
				t.Errorf("a model the interface cannot describe: %+v", m)
			}
			switch m.Fit {
			case "fits", "tight", "too big", "unknown":
			default:
				t.Errorf("%s fits how? %q", m.Title, m.Fit)
			}
		}
		if state.Memory < 0 {
			t.Errorf("memory %d", state.Memory)
		}
	})

	// A model that is there is a way of finding clips, without anybody
	// having to name a path in the settings.
	t.Run("one on the machine counts as a local model", func(t *testing.T) {
		s := emptyMachine(t)
		if s.Setup(context.Background()).HasLocalModel {
			t.Fatal("said there was a model on an empty machine")
		}
		placeLanguageModel(t, engine.LanguageModels()[0].Name)
		if !s.Setup(context.Background()).HasLocalModel {
			t.Error("did not see the model that is there")
		}
	})
}

func TestInstallingALanguageModelIsAJobLikeAnyOther(t *testing.T) {
	name := engine.LanguageModels()[0].Name

	t.Run("a model nobody offers fails as a job", func(t *testing.T) {
		s := emptyMachine(t)
		job := s.InstallLanguageModel("no-such-model.gguf")
		if job.State != JobFailed || job.Error == "" {
			t.Errorf("state %q, error %q", job.State, job.Error)
		}
	})

	t.Run("one already installed is not fetched again", func(t *testing.T) {
		s := emptyMachine(t)
		placeLanguageModel(t, name)
		if job := s.InstallLanguageModel(name); job.State != JobFailed {
			t.Errorf("started a download for a model that is already here: %q", job.State)
		}
	})

	// The lane matters: finding clips is what needs this model, so a search
	// queued behind it waits for it, and a transcription carries on in the
	// other lane rather than waiting for a download it does not need.
	t.Run("it shares the lane with finding clips", func(t *testing.T) {
		if got := laneFor("llm"); got != LaneFinding {
			t.Errorf("a language model install runs in the %q lane", got)
		}
		if got := laneFor("model"); got != LaneHearing {
			t.Errorf("a speech model install runs in the %q lane", got)
		}
	})

	t.Run("two asks at once are one install", func(t *testing.T) {
		s := emptyMachine(t)
		var wg sync.WaitGroup
		for range 8 {
			wg.Add(1)
			go func() {
				defer wg.Done()
				s.InstallLanguageModel(name)
			}()
		}
		wg.Wait()
		running := 0
		for _, job := range s.jobs.list() {
			if job.Kind == "llm" && (job.State == JobQueued || job.State == JobRunning) {
				running++
			}
		}
		if running > 1 {
			t.Errorf("%d installs of the same model at once", running)
		}
	})

	// The two are different work in different lanes, so one never stands
	// in for the other in the queue's book-keeping.
	t.Run("a speech install does not count as a language install", func(t *testing.T) {
		s := emptyMachine(t)
		speech := s.InstallSpeechModel(engine.SpeechModels()[0].Name)
		language := s.InstallLanguageModel(name)
		if speech.ID == language.ID {
			t.Fatal("the same job was handed back for both")
		}
	})
}

// Pretends llama.cpp is on the machine, by putting something runnable
// under that name on the only folder the search path holds here.
func placeLlamaServer(t *testing.T) {
	t.Helper()
	dir := os.Getenv("PATH")
	if err := os.MkdirAll(dir, 0o755); err != nil {
		t.Fatal(err)
	}
	// Never started by anything in these tests, so what it would do does
	// not matter. What matters is that it can be found and could be run.
	if err := os.WriteFile(filepath.Join(dir, "llama-server"), []byte("#!/bin/sh\nexit 0\n"), 0o755); err != nil {
		t.Fatal(err)
	}
}

// The worst state the setup could leave somebody in, and until now it
// could: a language model downloaded, fifteen gigabytes of it, and nothing
// on the machine able to open it. The app called that ready, because it
// only ever looked for the model.
func TestAModelWithNothingToRunItIsNotReady(t *testing.T) {
	if runtime.GOOS == "windows" {
		t.Skip("the search path works differently enough to need its own test")
	}
	s := emptyMachine(t)
	if err := s.ChoosePlanner("local"); err != nil {
		t.Fatal(err)
	}
	placeSpeechModel(t, engine.SpeechModels()[0].Name)
	placeLanguageModel(t, engine.LanguageModels()[0].Name)

	state := s.Setup(context.Background())
	if state.HasServer {
		t.Fatal("found a llama-server although the search path is an empty folder")
	}
	if !state.HasLocalModel || !state.HasSpeech {
		t.Fatalf("the models are not where the test put them: %+v", state)
	}
	if state.Ready {
		t.Error("said it was ready to make a short with a model it cannot run")
	}

	placeLlamaServer(t)
	state = s.Setup(context.Background())
	if !state.HasServer {
		t.Fatal("did not see the llama-server that is there")
	}
	if !state.Ready {
		t.Error("has the speech model, a language model and a server, and still says no")
	}
}

// The API way is not touched by any of that. A key is the whole of it, and
// somebody who never chose the local model should never be told about a
// program they do not need.
func TestTheAPIWayDoesNotNeedAServer(t *testing.T) {
	s := emptyMachine(t)
	if err := s.ChoosePlanner("api"); err != nil {
		t.Fatal(err)
	}
	placeSpeechModel(t, engine.SpeechModels()[0].Name)
	t.Setenv("ANTHROPIC_API_KEY", "sk-ant-not-a-real-key")

	state := s.Setup(context.Background())
	if state.HasServer {
		t.Fatal("found a llama-server although the search path is an empty folder")
	}
	if !state.Ready {
		t.Error("a key and a speech model are everything the API way needs, and it said no")
	}
}

// The link beside a missing key opens only the page written for that
// company, and names no other.
func TestOpenKeysPageOpensOnlyTheCompanysPage(t *testing.T) {
	s := &FrameFairy{}
	if err := s.OpenKeysPage("nobody"); err == nil {
		t.Error("a company that is not there had a page")
	}
	for _, p := range engine.Providers() {
		u, err := url.Parse(p.KeysPage)
		if err != nil || u.Scheme != "https" || !strings.HasSuffix(u.Host, p.KeysAt) {
			t.Errorf("%s: keys are made at %q, said to be %s", p.Name, p.KeysPage, p.KeysAt)
		}
	}
}
