package main

import (
	"bytes"
	"encoding/json"
	"errors"
	"fmt"
	"math"
	"os"
	"path/filepath"
	"strings"
	"sync"

	"framefairy/engine"
)

// Settings are what the settings screen edits. Empty paths mean the same
// defaults the command line uses.
type Settings struct {
	FFmpeg    string `json:"ffmpeg"`
	LLMServer string `json:"llmServer"`
	LLMModel  string `json:"llmModel"`
	ASRModel  string `json:"asrModel"`
	Planner   string `json:"planner"`
	APIModel  string `json:"apiModel"`
	// Target is how many clips a search looks for when a number was typed
	// for it, and 0 when it follows the window, see engine.SuggestedCount.
	// It was count, a number every search took whatever its window: a
	// file that still has it is read as following the window.
	Target int `json:"target"`
	// TargetWindow is how long the window was that Target was typed for,
	// in seconds. Target is for that window and no other, see targetFor.
	TargetWindow float64 `json:"targetWindow,omitempty"`
	Min          float64 `json:"min"`
	Max          float64 `json:"max"`
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
	// Chosen is true once somebody has answered how clips are found. Until
	// then Planner is a default rather than a decision, and the app asks
	// on the first run instead of guessing on the customer's behalf.
	Chosen bool `json:"chosen"`
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
	return Settings{Planner: d.Planner, APIModel: d.Model, Min: d.Min,
		Max: d.Max, HighlightColour: defaultColour, AppColour: defaultColour,
		CaptionY: engine.DefaultCaptionY}
}

// tidy fills in what a settings file written by an earlier version does not
// have, and refuses a colour that is not one. What arrives from the
// interface is not to be trusted any more than what arrives from a file.
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
	return s.UpdateSettings(func(set *Settings) { *set = v })
}

// UpdateSettings changes the settings in one step: read, change, write,
// all under the store's lock. Reading, changing and writing back as three
// calls lost a change whenever two were made at once, the number of clips
// set in the workspace while the captions were being dragged, say: both
// read the same settings and the second write undid the first.
func (s *store) UpdateSettings(change func(*Settings)) error {
	s.mu.Lock()
	defer s.mu.Unlock()
	v := s.settings
	change(&v)
	v.tidy()
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
// The library keeps the order the episodes were added in, the newest last.
// It was sorted by path, which lost that order for good: start, youtube and
// end, added in that order, came back as end, start and youtube. How the
// list is shown is the sidebar's choice, by name or by when an episode was
// added, and only the order kept here can say the second.
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
// served to the interface.
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

// PlanOf reports whether plan is a plan of the episode at path: both in the
// library, and the plan in that episode's own logs folder, where plans are
// kept. Known alone takes any file of any episode, so a plan of one
// episode given with the path of another would be edited against the
// wrong video, the wrong words and the wrong history.
func (s *store) PlanOf(path, plan string) bool {
	if !s.Known(path) || !s.Known(plan) {
		return false
	}
	return filepath.Dir(engine.ResolvePath(plan)) == engine.ResolvePath(engine.LogsDir(path))
}

// targetFor is the target typed for a window as long as this one, or 0,
// which follows the window, see engine.SuggestedCount. A number typed for
// one window is no number for another: three typed for the six minutes of
// one episode went on asking for three in the half hour of the next, and
// in the first search the app starts by itself when a video is added.
func (s Settings) targetFor(window float64) int {
	if s.Target > 0 && math.Abs(window-s.TargetWindow) < 0.5 {
		return s.Target
	}
	return 0
}

// GetSettings returns the saved settings.
func (s *FrameFairy) GetSettings() Settings { return s.store.Settings() }

// SaveSettings takes what the interface changed in the settings, and only
// that, as JSON keys and their values, and lays it over what is saved. It
// took the whole object once, and whatever the Go side had changed since
// the interface last read it was lost on the next save, unless it was
// carved out by hand, the way Chosen was. Chosen is still never taken from
// the interface: it is not a setting anybody edits, it is the record that
// the one setup question was answered. A key the settings do not have is
// refused, so a misspelt one is not quietly dropped.
func (s *FrameFairy) SaveSettings(changed map[string]json.RawMessage) error {
	delete(changed, "chosen")
	patch, err := json.Marshal(changed)
	if err != nil {
		return err
	}
	var bad error
	err = s.store.UpdateSettings(func(set *Settings) {
		next := *set
		dec := json.NewDecoder(bytes.NewReader(patch))
		dec.DisallowUnknownFields()
		if bad = dec.Decode(&next); bad == nil {
			*set = next
		}
	})
	if bad != nil {
		return fmt.Errorf("the settings were not saved: %w", bad)
	}
	return err
}
