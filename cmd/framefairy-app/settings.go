package main

import (
	"encoding/json"
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"sort"
	"strings"
	"sync"

	"framefairy/engine"
)

// Settings are what the settings screen edits. Empty paths mean the same
// defaults the command line uses.
type Settings struct {
	FFmpeg    string  `json:"ffmpeg"`
	LLMServer string  `json:"llmServer"`
	LLMModel  string  `json:"llmModel"`
	ASRModel  string  `json:"asrModel"`
	Planner   string  `json:"planner"`
	APIModel  string  `json:"apiModel"`
	Count     int     `json:"count"`
	Min       float64 `json:"min"`
	Max       float64 `json:"max"`
	// The colour of the pill behind the word being spoken, in the framefairy
	// the engine renders. It belongs to the short, not to the app.
	HighlightColour string `json:"highlightColour"`
	// The colour the app itself is picked out in: the chosen clip, the
	// window on the range picker, a button that matters. It belongs to the
	// app, not to the short, so the two are set apart from each other and
	// neither follows the other.
	AppColour string `json:"appColour"`
	OutputDir string `json:"outputDir"`
	// The one folder the training records of every episode go in. Empty
	// means the default, ~/.framefairy/training. It is not kept with an
	// episode, so letting go of a video leaves the records alone.
	TrainingDir string `json:"trainingDir"`
	// Where the captions sit, as the distance from the bottom of a
	// 1080x1920 frame. It belongs to the person, not to an episode: a place
	// that suits one video suits the next one.
	CaptionY float64 `json:"captionY"`
}

// defaultColour is what both the app and the word highlight start out as.
// One colour, so an app that has never been touched looks of a piece with
// the framefairy it makes.
const defaultColour = "#942192"

// wasDefaultColour is what the word highlight used to start out as. A
// setting still holding it was never chosen, it was only never changed, so
// it follows the default to the new one.
const wasDefaultColour = "#B4236F"

func defaultSettings() Settings {
	d := engine.DefaultOptions()
	return Settings{Planner: d.Planner, APIModel: d.Model, Count: d.Count, Min: d.Min,
		Max: d.Max, HighlightColour: defaultColour, AppColour: defaultColour,
		CaptionY: engine.DefaultCaptionY}
}

// tidy fills in what a settings file written by an earlier version does not
// have, and refuses a colour that is not one. What arrives from the window
// is not to be trusted any more than what arrives from a file.
func (s *Settings) tidy() {
	if strings.EqualFold(s.HighlightColour, wasDefaultColour) {
		s.HighlightColour = defaultColour
	}
	if !engine.LooksLikeColour(s.HighlightColour) {
		s.HighlightColour = defaultColour
	}
	if !engine.LooksLikeColour(s.AppColour) {
		s.AppColour = defaultColour
	}
}

// options turns the settings into what the engine takes.
func (s Settings) options() engine.Options {
	o := engine.DefaultOptions()
	o.FFmpeg = s.FFmpeg
	o.LLMServer = s.LLMServer
	o.LLMModel = s.LLMModel
	o.ASRModel = s.ASRModel
	if s.Planner != "" {
		o.Planner = s.Planner
	}
	if s.APIModel != "" {
		o.Model = s.APIModel
	}
	if s.Count > 0 {
		o.Count = s.Count
	}
	if s.Min > 0 {
		o.Min = s.Min
	}
	if s.Max > 0 {
		o.Max = s.Max
	}
	o.HighlightColour = s.HighlightColour
	if s.CaptionY > 0 {
		y := int(engine.SnapCaptionY(s.CaptionY))
		o.MarginV = &y
	}
	o.Out = s.OutputDir
	o.TrainingDir = s.TrainingDir
	return o
}

// store keeps the settings and the list of episodes in the user's
// configuration folder. Plain JSON files, no database.
type store struct {
	mu       sync.Mutex
	dir      string
	settings Settings
	episodes []string
}

func openStore() *store {
	dir, err := os.UserConfigDir()
	if err != nil {
		dir = os.TempDir()
	}
	s := &store{dir: filepath.Join(dir, "FrameFairy"), settings: defaultSettings()}
	s.load("settings.json", &s.settings)
	s.load("library.json", &s.episodes)
	s.settings.tidy()
	s.settings.apply()
	return s
}

func (s *store) load(name string, into any) {
	data, err := os.ReadFile(filepath.Join(s.dir, name))
	if err == nil {
		_ = json.Unmarshal(data, into)
	}
}

func (s *store) save(name string, value any) error {
	if err := os.MkdirAll(s.dir, 0o755); err != nil {
		return err
	}
	body, err := json.MarshalIndent(value, "", "  ")
	if err != nil {
		return err
	}
	path := filepath.Join(s.dir, name)
	tmp := path + ".tmp"
	if err := os.WriteFile(tmp, body, 0o644); err != nil {
		return err
	}
	return os.Rename(tmp, path)
}

func (s *store) Settings() Settings {
	s.mu.Lock()
	defer s.mu.Unlock()
	return s.settings
}

// apply puts the settings that live outside this struct where they belong.
// The one training folder is read by the engine wherever it records, so it
// is set whenever the settings are read or written.
func (s Settings) apply() {
	engine.SetTrainingDir(s.TrainingDir)
}

func (s *store) SetSettings(v Settings) error {
	v.tidy()
	s.mu.Lock()
	defer s.mu.Unlock()
	s.settings = v
	v.apply()
	return s.save("settings.json", v)
}

func (s *store) Episodes() []string {
	s.mu.Lock()
	defer s.mu.Unlock()
	return append([]string(nil), s.episodes...)
}

// AddEpisodes puts files in the library, leaving out what is already there,
// and gives back the ones it added.
//
// Everything about an episode lives in a folder named after it without its
// extension, so ep.mp4 and ep.mov side by side would share one: the same
// transcript, the same clip sets, the same rendered names. The second of
// them is not added, and the caller is told which.
func (s *store) AddEpisodes(paths []string) ([]string, error) {
	s.mu.Lock()
	defer s.mu.Unlock()
	seen := map[string]bool{}
	taken := map[string]string{}
	for _, p := range s.episodes {
		seen[p] = true
		taken[engine.WorkDir(p)] = p
	}
	var clash, added []string
	for _, p := range paths {
		abs, err := filepath.Abs(p)
		if err != nil || seen[abs] {
			continue
		}
		if other, used := taken[engine.WorkDir(abs)]; used {
			clash = append(clash, fmt.Sprintf("%s and %s would share the folder %s",
				filepath.Base(abs), filepath.Base(other), filepath.Base(engine.WorkDir(abs))))
			continue
		}
		s.episodes = append(s.episodes, abs)
		seen[abs] = true
		taken[engine.WorkDir(abs)] = abs
		added = append(added, abs)
	}
	sort.Strings(s.episodes)
	if err := s.save("library.json", s.episodes); err != nil {
		return nil, err
	}
	if len(clash) > 0 {
		return added, errors.New(strings.Join(clash, ", ") + ", so it was left out")
	}
	return added, nil
}

func (s *store) RemoveEpisode(path string) error {
	s.mu.Lock()
	defer s.mu.Unlock()
	kept := s.episodes[:0]
	for _, p := range s.episodes {
		if p != path {
			kept = append(kept, p)
		}
	}
	s.episodes = kept
	return s.save("library.json", s.episodes)
}

// Known reports whether a file belongs to an episode in the library, either
// the episode itself or something in its work folder. Only those files are
// served to the window.
//
// A path is judged by where it really leads. A link inside a work folder
// can point anywhere, and a name is not a promise, so both the path and the
// folder it has to be inside are resolved before they are compared.
func (s *store) Known(path string) bool {
	clean := filepath.Clean(path)
	real := engine.ResolvePath(clean)
	for _, ep := range s.Episodes() {
		if clean == ep || real == engine.ResolvePath(ep) {
			return true
		}
		work := engine.ResolvePath(engine.WorkDir(ep)) + string(filepath.Separator)
		if len(real) > len(work) && real[:len(work)] == work {
			return true
		}
	}
	return false
}
