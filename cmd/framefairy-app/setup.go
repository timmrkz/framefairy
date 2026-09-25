package main

import (
	"context"
	"fmt"
	"os"
	"path/filepath"

	"framefairy/engine"
)

// What a new copy of the app needs before it can turn an episode into
// shorts, and how far along that is.
//
// Two things are needed and only one of them is a question. Speech is
// always local and there is one model, so the app says what it is about to
// do and does it. Finding clips is a real choice between an Anthropic API
// key and a local model, with different costs either way, so it is asked
// once and can be changed later in the settings.
//
// See docs/PACKAGING.md.

// SpeechModelView is one installable model as the interface sees it, which
// is the engine's entry plus whether it is already here.
type SpeechModelView struct {
	engine.SpeechModel
	Installed bool `json:"installed"`
}

// LanguageModelView is the same for a model that finds clips, and it says
// one thing more: whether this machine can hold it. A model runs from
// memory, so a machine too small for one is a machine that swaps, and a
// model that swaps is minutes an answer rather than seconds. Better to say
// so before the download than after it.
type LanguageModelView struct {
	engine.LanguageModel
	Installed bool `json:"installed"`
	// Fit is "fits", "tight", "too big" or "unknown". See engine.Fit.
	Fit string `json:"fit"`
	// Recommended marks the one to offer this machine, which is the
	// largest it can hold. It belongs to the machine and not to the model,
	// which is why it is worked out here rather than written into the
	// catalogue.
	Recommended bool `json:"recommended"`
	// InUse marks the one a search runs on: the model file named in the
	// settings, or with none named the only one there is.
	InUse bool `json:"inUse"`
}

// SetupState is what the interface needs to decide whether to ask anything.
type SetupState struct {
	// Speech is every model that can be installed, best first.
	Speech []SpeechModelView `json:"speech"`
	// HasSpeech is true once any of them is installed. Until then nothing
	// can be transcribed, which is the first thing every episode needs.
	HasSpeech bool `json:"hasSpeech"`
	// Language is every model that can find clips on this machine, best
	// first, each saying whether the machine can hold it.
	Language []LanguageModelView `json:"language"`
	// Memory is what this machine has, in bytes, or zero where it would
	// not say. The interface says it out loud beside the models, because a
	// model being called too big is only believable next to the number it
	// was judged against.
	Memory int64 `json:"memory"`
	// Planner is "api" or "local", as the settings have it.
	Planner string `json:"planner"`
	// HasKey is true when an Anthropic key can be found. It never carries
	// the key itself.
	HasKey bool `json:"hasKey"`
	// HasLocalModel is true when a language model is on the machine.
	HasLocalModel bool `json:"hasLocalModel"`
	// HasServer is true when a llama-server can be found and run. A model
	// without one is fifteen gigabytes that answer nothing, so the interface
	// says so before the download rather than after it.
	HasServer bool `json:"hasServer"`
	// Chosen is true once somebody has answered the one question, so the
	// app knows the settings are a decision rather than a default.
	Chosen bool `json:"chosen"`
	// Ready is true when nothing more is needed to make a short.
	Ready bool `json:"ready"`
	// Installing is the id of a model install in hand, so the interface,
	// opened again while one runs, picks it up rather than starting a
	// second.
	Installing string `json:"installing,omitempty"`
}

// Setup reports what is still needed. It reads, it never changes anything.
func (s *FrameFairy) Setup(ctx context.Context) SetupState {
	settings := s.store.Settings()
	dir := engine.ModelsDir()

	state := SetupState{Planner: settings.Planner, Chosen: settings.Chosen}
	for _, m := range engine.SpeechModels() {
		view := SpeechModelView{SpeechModel: m, Installed: m.Installed(dir)}
		if view.Installed {
			state.HasSpeech = true
		}
		state.Speech = append(state.Speech, view)
	}
	state.Memory = engine.MachineMemory()
	best, hasBest := engine.RecommendedFor(state.Memory)
	using := modelInUse(settings)
	for _, m := range engine.LanguageModels() {
		view := LanguageModelView{LanguageModel: m, Installed: m.Installed(dir),
			Fit:         string(m.FitsIn(state.Memory)),
			Recommended: hasBest && m.Name == best.Name}
		view.InUse = view.Installed && using == m.Name
		if view.Installed {
			state.HasLocalModel = true
		}
		state.Language = append(state.Language, view)
	}
	if _, err := engine.ReadAPIKey(ctx); err == nil {
		state.HasKey = true
	}
	state.HasServer = engine.HasLlamaServer()
	// A model put there by hand counts too. The catalogue is a convenience,
	// not the only way in: somebody who already has a .gguf they like keeps
	// using it.
	if named := settings.LLMModel; named != "" {
		if info, err := os.Stat(named); err == nil && !info.IsDir() {
			state.HasLocalModel = true
		}
	} else if found, err := engine.DefaultLocalModel(); err == nil && found != "" {
		state.HasLocalModel = true
	}
	for _, job := range s.jobs.list() {
		if (job.Kind == "model" || job.Kind == "llm") &&
			(job.State == JobQueued || job.State == JobRunning) {
			state.Installing = job.ID
		}
	}

	// Ready means a short could be made now. Clips can be found either way,
	// so whichever was chosen has to be usable, and without a choice the
	// app does not guess on the customer's behalf.
	canPlan := false
	switch {
	case !state.Chosen:
	case state.Planner == "api":
		canPlan = state.HasKey
	case state.Planner == "local":
		// Both halves, because either one alone makes nothing. The model is
		// what answers and llama-server is what runs it, and until this was
		// checked the app could call itself ready on a machine that had the
		// download and no way to open it.
		canPlan = state.HasLocalModel && state.HasServer
	}
	state.Ready = state.HasSpeech && canPlan
	return state
}

// InstallSpeechModel fetches a model and unpacks it, as a job, so it wears
// the same beam and the same fill as every other piece of work and can be
// cancelled the same way.
func (s *FrameFairy) InstallSpeechModel(name string) Job {
	model, ok := engine.SpeechModelByName(name)
	if !ok {
		return s.jobs.refuse("", "model", "Speech model", "there is no speech model called "+name)
	}
	if model.Installed(engine.ModelsDir()) {
		return s.jobs.refuse("", "model", model.Title, model.Title+" is already installed")
	}
	// One at a time. Two installs would write over each other's unpacking
	// folder, and the interface can ask twice by being opened twice.
	return s.jobs.addOnce("", "model", model.Title,
		func(ctx context.Context, p *engine.Project) (string, error) {
			return "", engine.InstallSpeechModel(ctx, p.Log(), model, engine.ModelsDir())
		})
}

// InstallLanguageModel fetches a model that can find clips, as a job, the
// same as the speech model and with the same beam and fill.
//
// It runs on the work lane rather than the transcribe lane, because it is
// what finding clips needs: a search queued behind it then waits for the
// model instead of failing on it, and a transcription carries on in the
// other lane meanwhile.
//
// A model the machine cannot hold is still installable. Saying no on
// somebody's behalf is not this app's job, and a machine's memory can be
// read wrong: what the interface does is say what it thinks before the
// download rather than refuse after it.
func (s *FrameFairy) InstallLanguageModel(name string) Job {
	model, ok := engine.LanguageModelByName(name)
	if !ok {
		return s.jobs.refuse("", "llm", "Language model", "there is no language model called "+name)
	}
	if model.Installed(engine.ModelsDir()) {
		return s.jobs.refuse("", "llm", model.Title, model.Title+" is already installed")
	}
	// The model in use before this one arrives stays in use. With none
	// named, one model on the machine is the one in use by being the only
	// one, and a second arriving would take that away without a word: the
	// engine will not guess between two, so every search after the
	// download would fail. Naming the one that was in use keeps things as
	// they were, and Use is how the new one takes over.
	before := modelInUse(s.store.Settings())
	// One at a time, however often it is asked for.
	return s.jobs.addOnce("", "llm", model.Title,
		func(ctx context.Context, p *engine.Project) (string, error) {
			if err := engine.InstallLanguageModel(ctx, p.Log(), model, engine.ModelsDir()); err != nil {
				return "", err
			}
			return "", s.keepInUse(before)
		})
}

// keepInUse names the model that was in use before an install, when the
// settings name none, so the install does not leave two models and no
// choice between them.
func (s *FrameFairy) keepInUse(before string) error {
	settings := s.store.Settings()
	if settings.LLMModel != "" || before == "" {
		return nil
	}
	settings.LLMModel = filepath.Join(engine.ModelsDir(), before)
	return s.store.SetSettings(settings)
}

// SaveAPIKey puts a key in the macOS keychain, which is the only place the
// app ever keeps one. An app opened from Finder has no shell environment,
// so the variable the command line reads is never set, and until now there
// was no way to give the app a key except a terminal command in an error
// message.
//
// An empty key removes the stored one rather than saving nothing.
func (s *FrameFairy) SaveAPIKey(key string) error {
	return engine.StoreAPIKey(key)
}

// ChoosePlanner records the answer to the one question, so the app knows
// the settings are a decision rather than a default and stops asking.
func (s *FrameFairy) ChoosePlanner(planner string) error {
	if planner != "api" && planner != "local" {
		return os.ErrInvalid
	}
	return s.store.UpdateSettings(func(set *Settings) {
		set.Planner = planner
		set.Chosen = true
	})
}

// modelInUse is the file name of the model a search runs on: the one named
// in the settings, or with none named the only one there is. Empty when
// there is none, or several and none named, which is a search that fails
// until one is chosen.
func modelInUse(settings Settings) string {
	if settings.LLMModel != "" {
		return filepath.Base(settings.LLMModel)
	}
	found, err := engine.DefaultLocalModel()
	if err != nil || found == "" {
		return ""
	}
	return filepath.Base(found)
}

// UseLanguageModel makes an installed model the one clips are found with,
// and says the path it is found at, so the settings on screen can show it
// without being read again over whatever else was being typed there.
//
// Two models on the machine and none named is a search that cannot start,
// because the engine will not guess which one was meant. Installing a
// second model to try it is exactly how that happens, so choosing between
// them is one click on the model rather than a path typed into a field.
func (s *FrameFairy) UseLanguageModel(name string) (string, error) {
	model, ok := engine.LanguageModelByName(name)
	if !ok {
		return "", fmt.Errorf("there is no language model called %s", name)
	}
	dir := engine.ModelsDir()
	if !model.Installed(dir) {
		return "", fmt.Errorf("%s is not installed", model.Title)
	}
	path := filepath.Join(dir, model.Name)
	settings := s.store.Settings()
	settings.LLMModel = path
	return path, s.store.SetSettings(settings)
}

// busyWith says whether a job of one of these kinds is waiting or running.
func (s *FrameFairy) busyWith(kinds ...string) bool {
	for _, job := range s.jobs.list() {
		if job.State != JobQueued && job.State != JobRunning {
			continue
		}
		for _, kind := range kinds {
			if job.Kind == kind {
				return true
			}
		}
	}
	return false
}

// RemoveLanguageModel takes a model off the machine, to give its room back.
// Not while one is being installed or clips are being found, because
// either may be reading the very file. If it was the one named in the
// settings, the settings no longer name it: a model that is not there is
// nothing to point at.
func (s *FrameFairy) RemoveLanguageModel(name string) error {
	model, ok := engine.LanguageModelByName(name)
	if !ok {
		return fmt.Errorf("there is no language model called %s", name)
	}
	if s.busyWith("llm") {
		return fmt.Errorf("a model is being installed. Remove %s once that is done", model.Title)
	}
	if s.busyWith("plan") {
		return fmt.Errorf("clips are being found. Remove %s once that is done", model.Title)
	}
	if err := engine.RemoveLanguageModel(model, engine.ModelsDir()); err != nil {
		return err
	}
	settings := s.store.Settings()
	if settings.LLMModel != "" && filepath.Base(settings.LLMModel) == model.Name {
		settings.LLMModel = ""
		return s.store.SetSettings(settings)
	}
	return nil
}

// RemoveSpeechModel takes a speech model off the machine. Not while one is
// being installed or an episode is being transcribed. Without one, the app
// asks for it again the next time it starts, because nothing can be made
// without it.
func (s *FrameFairy) RemoveSpeechModel(name string) error {
	model, ok := engine.SpeechModelByName(name)
	if !ok {
		return fmt.Errorf("there is no speech model called %s", name)
	}
	if s.busyWith("model") {
		return fmt.Errorf("a model is being installed. Remove %s once that is done", model.Title)
	}
	if s.busyWith("transcribe") {
		return fmt.Errorf("an episode is being transcribed. Remove %s once that is done", model.Title)
	}
	return engine.RemoveSpeechModel(model, engine.ModelsDir())
}
