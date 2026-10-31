package main

import (
	"bytes"
	"encoding/json"
	"errors"
	"fmt"
	"math"
	"os"
	"path/filepath"
	"slices"
	"strings"
	"sync"
	"time"

	"github.com/spf13/afero"

	"framefairy/engine"
	"framefairy/engine/fileio"
)

// Settings are what the settings screen edits. Empty paths mean the same
// defaults the command line uses.
//
// There is no ffmpeg and no llama-server here. They were paths the
// settings could name, and the app ran whatever they named: the app's own
// tools are the only ones it runs now, see tool.Find. A settings file
// that still names one has it read past and dropped on the next save.
type Settings struct {
	LLMModel string `json:"llmModel"`
	ASRModel string `json:"asrModel"`
	Planner  string `json:"planner"`
	APIModel string `json:"apiModel"`
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
	// The folder for shorts, empty for next to each episode. Each episode
	// renders into a folder of its own in it, see engine.Options.Shorts.
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
	o.Shorts = s.OutputDir
	o.TrainingDir = s.TrainingDir
	return o
}

// store keeps the settings and the list of episodes in the user's
// configuration folder. Plain JSON files, no database.
type store struct {
	mu  sync.Mutex
	dir string
	// files is where the library's episodes and their work folders are
	// read and written, the real disk in the app. See engine/files.go.
	files afero.Fs
	// records is where the training records and the timings of past
	// searches are kept, for every engine the app makes and every edit
	// that records. The training folder follows the settings.
	records  *engine.Records
	settings Settings
	episodes []string
	// changed is told when the settings change. It is told under the
	// store's lock, so it only takes note.
	changed func()
}

func openStore() *store {
	dir, err := os.UserConfigDir()
	if err != nil {
		dir = os.TempDir()
	}
	return openStoreIn(afero.NewOsFs(), filepath.Join(dir, "FrameFairy"))
}

// openStoreIn is the store whose settings and library are in dir, with
// everything read and written through files.
func openStoreIn(files afero.Fs, dir string) *store {
	s := &store{dir: dir, files: files, records: engine.NewRecords(files, "", ""), settings: defaultSettings()}
	s.load("settings.json", &s.settings)
	s.load("library.json", &s.episodes)
	s.settings.tidy()
	s.apply(s.settings)
	s.markEarlier()
	return s
}

func (s *store) load(name string, into any) {
	data, err := afero.ReadFile(s.files, filepath.Join(s.dir, name))
	if err == nil {
		_ = json.Unmarshal(data, into)
	}
}

// save writes a file of the store whole and flushed, the way the engine
// writes its own, so a crash never leaves the library half written.
func (s *store) save(name string, value any) error {
	if err := s.files.MkdirAll(s.dir, 0o755); err != nil {
		return err
	}
	body, err := json.MarshalIndent(value, "", "  ")
	if err != nil {
		return err
	}
	return fileio.WriteAtomic(s.files, filepath.Join(s.dir, name), body)
}

func (s *store) Settings() Settings {
	s.mu.Lock()
	defer s.mu.Unlock()
	return s.settings
}

// apply puts the settings that live outside the settings where they
// belong. The training folder is read wherever the app records, so it is
// set whenever the settings are read or written.
func (s *store) apply(v Settings) {
	s.records.SetTrainingDir(v.TrainingDir)
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
	s.apply(v)
	if s.changed != nil {
		s.changed()
	}
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
		if why := linkedEpisode(s.files, abs); why != "" {
			clash = append(clash, why)
			continue
		}
		if other, used := taken[engine.WorkDir(abs)]; used {
			clash = append(clash, fmt.Sprintf("%s and %s would share the folder %s",
				filepath.Base(abs), filepath.Base(other), filepath.Base(engine.WorkDir(abs))))
			continue
		}
		if err := markAdded(s.files, abs); err != nil {
			clash = append(clash, fmt.Sprintf("the folder %s beside %s cannot be written, %v",
				filepath.Base(engine.WorkDir(abs)), filepath.Base(abs), err))
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
		return added, errors.New(strings.Join(clash, ", ") + ", so it cannot be added")
	}
	return added, nil
}

// linkedEpisode says why a video cannot be an episode because it, or the
// work folder beside it, is a link, or "" when neither is. A link inside
// a work folder is judged by where it leads, see Known, but the folder
// itself was taken as it was found: a zip with episode.mp4 and an
// episode.framefairy that led to the home folder made every file there
// the episode's, to read, to serve and to write into.
func linkedEpisode(files afero.Fs, video string) string {
	if info, err := fileio.LstatIn(files, video); err == nil && info.Mode()&os.ModeSymlink != 0 {
		return filepath.Base(video) + " is a link, which the app does not follow"
	}
	work := engine.WorkDir(video)
	if info, err := fileio.LstatIn(files, work); err == nil && (info.Mode()&os.ModeSymlink != 0 || !info.IsDir()) {
		return filepath.Base(work) + " beside " + filepath.Base(video) +
			" is a link or not a folder, which the app does not follow"
	}
	return ""
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

// addedFile marks a work folder as the folder of a video in the library.
// It is written as the video is added, so a video whose folder no longer
// has it had its folder deleted outside the app: Tim deleted one in Finder
// to remove the video, and it stayed in the sidebar with nothing in it.
// The folder alone could not say so, because the app writes a folder back
// whenever it is asked about the video, the chosen clip and the waveform
// among other things, so a folder deleted and opened again was there
// again, with nothing in it.
const addedFile = "added.json"

// markAdded writes the mark into the work folder of a video added to the
// library, and makes the folder where there is none.
func markAdded(files afero.Fs, video string) error {
	work := engine.WorkDir(video)
	if err := files.MkdirAll(work, 0o755); err != nil {
		return err
	}
	file, err := fileio.SafeChildIn(files, work, addedFile)
	if err != nil {
		return err
	}
	if info, err := files.Stat(file); err == nil && info.Mode().IsRegular() {
		return nil
	}
	body, err := json.Marshal(struct {
		Added string `json:"added"`
	}{time.Now().UTC().Format(time.RFC3339)})
	if err != nil {
		return err
	}
	return fileio.WriteAtomic(files, file, body)
}

// marked says whether the work folder of a video carries the mark.
func marked(files afero.Fs, video string) bool {
	file, err := fileio.SafeChildIn(files, engine.WorkDir(video), addedFile)
	if err != nil {
		return false
	}
	info, err := files.Stat(file)
	return err == nil && info.Mode().IsRegular()
}

// deletedOutside says whether a video in the library was deleted outside
// the app: its folder has no mark, and the video is there, or the folder
// it was in is. A video deleted in Finder with its folder, the way Tim
// cleared two of his, leaves the folder they were in behind. A drive
// taken out or a folder moved away takes that folder too, so a video
// whose folder is gone with it is not judged: it says File not found
// until it is back.
func deletedOutside(files afero.Fs, video string) bool {
	if marked(files, video) {
		return false
	}
	info, err := files.Stat(video)
	if err == nil {
		return info.Mode().IsRegular()
	}
	if !os.IsNotExist(err) {
		return false
	}
	dir, err := files.Stat(filepath.Dir(video))
	return err == nil && dir.IsDir()
}

// writtenBack are the files the app writes into a video's folder when it
// is asked about the video, opened or its waveform drawn, whether the
// folder is there or not. A folder that holds nothing else was written
// back after it was deleted.
var writtenBack = map[string]bool{
	chosenFile:                             true,
	filepath.Join("logs", "levels.json"):   true,
	filepath.Join("logs", "levels.frames"): true,
}

// markEarlier marks the folders of the videos added before there was a
// mark, once, as the library is read: every folder that holds work of
// its own. A folder that holds only what the app writes back is not
// marked, so a folder deleted under an earlier version and written back
// since leaves the list as it would now.
func (s *store) markEarlier() {
	for _, ep := range s.Episodes() {
		if marked(s.files, ep) || linkedEpisode(s.files, ep) != "" {
			continue
		}
		work := engine.WorkDir(ep)
		own := afero.Walk(s.files, work, func(path string, info os.FileInfo, err error) error {
			if err != nil {
				return err
			}
			rel, _ := filepath.Rel(work, path)
			if info.Mode().IsRegular() && !writtenBack[rel] {
				return errOwnWork
			}
			return nil
		})
		if errors.Is(own, errOwnWork) {
			_ = markAdded(s.files, ep)
		}
	}
}

// errOwnWork ends the look through a folder at its first file of work.
var errOwnWork = errors.New("work of its own")

// letGo takes a video whose folder was deleted outside the app out of the
// library, and says whether it did: of two reads of the library at once,
// one lets it go.
func (s *store) letGo(path string) (bool, error) {
	s.mu.Lock()
	defer s.mu.Unlock()
	if !slices.Contains(s.episodes, path) || !deletedOutside(s.files, path) {
		return false, nil
	}
	s.episodes = slices.DeleteFunc(s.episodes, func(p string) bool { return p == path })
	return true, s.save("library.json", s.episodes)
}

// clearLetGo deletes what was written into the folder of a video let go
// after its folder was deleted, unless the video was added again since.
// Under the store's lock, so an Add of the same video waits for it.
func (s *store) clearLetGo(path string) (bool, error) {
	s.mu.Lock()
	defer s.mu.Unlock()
	if slices.Contains(s.episodes, path) || marked(s.files, path) {
		return false, nil
	}
	if _, err := fileio.LstatIn(s.files, engine.WorkDir(path)); err != nil {
		return false, nil
	}
	return true, engine.DeleteWork(s.files, path)
}

// Known reports whether a file belongs to an episode in the library: the
// episode itself, something in its work folder, or the short of one of its
// clips wherever it was rendered into, see engine.IsShort. Only those files
// are served to the interface and shown in a folder.
//
// A path is judged by where it really leads. A link inside a work folder
// can point anywhere, and a name is not a promise, so both the path and the
// folder it has to be inside are resolved before they are compared.
func (s *store) Known(path string) bool {
	clean := filepath.Clean(path)
	real := fileio.ResolveIn(s.files, clean)
	episodes := s.Episodes()
	for _, ep := range episodes {
		// An episode whose video or work folder has become a link since
		// it was added is no longer one: everything under the folder it
		// leads to would count as the episode's.
		if linkedEpisode(s.files, ep) != "" {
			continue
		}
		if clean == ep || real == fileio.ResolveIn(s.files, ep) {
			return true
		}
		work := fileio.ResolveIn(s.files, engine.WorkDir(ep)) + string(filepath.Separator)
		if len(real) > len(work) && real[:len(work)] == work {
			return true
		}
	}
	// A short outside every work folder, in the folder the settings name
	// for shorts, is the one file outside them that belongs to an episode.
	// The plans are only read for a file that could be one.
	if !strings.EqualFold(filepath.Ext(real), ".mp4") {
		return false
	}
	for _, ep := range episodes {
		if linkedEpisode(s.files, ep) == "" && engine.IsShort(s.files, ep, real) {
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
	return filepath.Dir(fileio.ResolveIn(s.files, plan)) == fileio.ResolveIn(s.files, engine.LogsDir(path))
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
	// Asked before the settings are, so the queue's lock is never taken
	// under the store's. See languageLocked.
	language, speech := s.languageLocked(), s.speechLocked()
	var bad, refused error
	err = s.store.UpdateSettings(func(set *Settings) {
		next := *set
		dec := json.NewDecoder(bytes.NewReader(patch))
		dec.DisallowUnknownFields()
		if bad = dec.Decode(&next); bad != nil {
			return
		}
		if next.LLMModel != set.LLMModel || next.Planner != set.Planner || next.APIModel != set.APIModel {
			if refused = language; refused != nil {
				return
			}
		}
		if next.ASRModel != set.ASRModel {
			if refused = speech; refused != nil {
				return
			}
		}
		*set = next
	})
	if bad != nil {
		return fmt.Errorf("the settings were not saved: %w", bad)
	}
	if refused != nil {
		return refused
	}
	return err
}
