package main

import (
	"context"
	"os"

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

// SpeechModelView is one installable model as the window sees it, which is
// the engine's entry plus whether it is already here.
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
}

// SetupState is what the window needs to decide whether to ask anything.
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
	// not say. The window says it out loud beside the models, because a
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
	// Chosen is true once somebody has answered the one question, so the
	// app knows the settings are a decision rather than a default.
	Chosen bool `json:"chosen"`
	// Ready is true when nothing more is needed to make a short.
	Ready bool `json:"ready"`
	// Installing is the id of a model install in hand, so a window opened
	// again while one runs picks it up rather than starting a second.
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
	for _, m := range engine.LanguageModels() {
		view := LanguageModelView{LanguageModel: m, Installed: m.Installed(dir),
			Fit: string(m.FitsIn(state.Memory))}
		if view.Installed {
			state.HasLocalModel = true
		}
		state.Language = append(state.Language, view)
	}
	if _, err := engine.ReadAPIKey(ctx); err == nil {
		state.HasKey = true
	}
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
		canPlan = state.HasLocalModel
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
	// folder, and the window can ask twice by being opened twice.
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
// read wrong: what the window does is say what it thinks before the
// download rather than refuse after it.
func (s *FrameFairy) InstallLanguageModel(name string) Job {
	model, ok := engine.LanguageModelByName(name)
	if !ok {
		return s.jobs.refuse("", "llm", "Language model", "there is no language model called "+name)
	}
	if model.Installed(engine.ModelsDir()) {
		return s.jobs.refuse("", "llm", model.Title, model.Title+" is already installed")
	}
	// One at a time, however often it is asked for.
	return s.jobs.addOnce("", "llm", model.Title,
		func(ctx context.Context, p *engine.Project) (string, error) {
			return "", engine.InstallLanguageModel(ctx, p.Log(), model, engine.ModelsDir())
		})
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
	settings := s.store.Settings()
	settings.Planner = planner
	settings.Chosen = true
	return s.store.SetSettings(settings)
}
