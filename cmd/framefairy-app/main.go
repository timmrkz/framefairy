// Command app is the desktop app for framefairy. It drives the same engine as the
// command line.
package main

import (
	"bufio"
	"context"
	"embed"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"io/fs"
	"log"
	"math"
	"net/http"
	"os"
	"os/exec"
	"path/filepath"
	"runtime"
	"sort"
	"strings"
	"sync"
	"time"

	"github.com/wailsapp/wails/v3/pkg/application"
	"github.com/wailsapp/wails/v3/pkg/events"

	"framefairy/engine"
	"framefairy/notices"
	"framefairy/updates"
)

// dist/app/ holds the interface that make builds from frontend/. It is not
// in the repository. dist/.keep is, so this compiles before the first build.
//
//go:embed all:dist
var dist embed.FS

// interfaceHandler serves the built interface, or a page that says how to
// build it.
func interfaceHandler() http.Handler {
	built, err := fs.Sub(dist, "dist/app")
	if err == nil {
		if _, err = fs.Stat(built, "index.html"); err == nil {
			return application.AssetFileServerFS(built)
		}
	}
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "text/html; charset=utf-8")
		_, _ = io.WriteString(w, notBuiltPage)
	})
}

const notBuiltPage = `<!DOCTYPE html>
<html lang="en"><head><meta charset="utf-8"><title>Frame Fairy</title>
<style>body{font-family:-apple-system,system-ui,sans-serif;background:#16171a;color:#ebe8e3;
margin:0;display:grid;place-items:center;height:100vh}main{max-width:32rem;padding:2rem}
code{background:#262930;padding:2px 6px;border-radius:4px}</style></head>
<body><main><h1>The interface is not built</h1>
<p>This program was built without its interface. Build it with <code>make</code> from the
top of the repository, which builds the interface first, then start <code>bin/framefairy-app</code>.</p>
</main></body></html>`

func main() {
	// Started again to put a new build in place once the app has quit.
	installIfAsked()
	go tidyTemp(os.TempDir(), time.Now())
	widenPath()
	engine.PreferSavedKeys()
	st := openStore()

	var app *application.App
	emit := func(u JobUpdate) {
		if app != nil {
			app.Event.Emit("job", u)
		}
	}
	notify := func(episode string) {
		if app != nil {
			app.Event.Emit("episode", episode)
		}
	}
	svc := &FrameFairy{store: st}
	svc.jobs = newQueue(st, emit, notify)
	// The searches and renders that were cut off or failed the last time
	// say so where their work was.
	svc.jobs.restore(st.Episodes())
	// Nothing the app started outlives it: a model loaded or still loading
	// is stopped and no other is loaded, and the jobs are stopped, and with
	// them any ffmpeg they run. On macOS app.Run never returns: Cmd+Q ends
	// the process from inside Cocoa once the shutdown hooks have run, so
	// this has to be one of them. Code after app.Run only runs on the other
	// systems, and left a llama-server behind on every Mac that quit during
	// a search.
	svc.levels = newMeasuring(func() string { return st.Settings().FFmpeg }, func(episode string) {
		if app != nil {
			app.Event.Emit("levels", episode)
		}
	})
	quit := sync.OnceFunc(func() {
		engine.CloseModels()
		svc.jobs.shutDown()
		svc.levels.shutDown()
		// A build that is ready goes in place once the app is gone.
		svc.updates.installOnQuit()
	})
	// What Cmd+Q does, see quit.go. The hook above stays for whatever
	// ends the app without asking, a signal from the terminal among them.
	leave := &leaving{
		busy: svc.jobs.busy,
		say: func(what string) {
			if app != nil {
				app.Event.Emit("quit", what)
			}
		},
		stop: quit,
		quit: func() { app.Quit() },
	}

	app = application.New(application.Options{
		Name:        "Frame Fairy",
		Description: "Turns a podcast episode into vertical clips",
		Services:    []application.Service{application.NewService(svc)},
		Assets: application.AssetOptions{
			Handler:    interfaceHandler(),
			Middleware: mediaMiddleware(st),
		},
		Mac: application.MacOptions{
			ApplicationShouldTerminateAfterLastWindowClosed: true,
		},
		OnShutdown: quit,
		ShouldQuit: leave.shouldQuit,
	})
	svc.app = app
	svc.updates = newUpdating(app.Updater, st, svc.jobs.busy, func(u UpdateState) {
		app.Event.Emit("updates", u)
	})
	app.Menu.Set(appMenu(app, func() {
		go svc.updates.check()
		app.Event.Emit("show-updates", nil)
	}))
	// The commit of the build on the Updates page is a link, so a right
	// click on it offers what a right click on a link offers in Safari.
	// It is drawn as a button, and without this the webview would offer
	// nothing a link needs.
	if commit := updates.CommitURL(buildCommit); commit != "" {
		link := app.ContextMenu.New()
		link.Add("Open Link").OnClick(func(*application.Context) { _ = svc.OpenCommit() })
		link.Add("Copy Link").OnClick(func(*application.Context) { app.Clipboard.SetText(commit) })
		app.ContextMenu.Add("commit", link)
	}

	svc.window = app.Window.NewWithOptions(application.WebviewWindowOptions{
		Title:     "Frame Fairy",
		Width:     1280,
		Height:    820,
		MinWidth:  960,
		MinHeight: 640,
		// The bar at the top of the app, --ink-1 in app.css. The colour
		// of the app's window itself is only ever seen where the page does
		// not paint, which on macOS 26 is the sliver between its rounded
		// corner and the webview's. At the top that sliver is
		// inside the bar, so it is the bar's colour or it is a notch.
		BackgroundColour: application.NewRGB(29, 31, 35),
		URL:              "/",
		Mac: application.MacWindow{
			// How far down a click still drags the app. Read once when the
			// app's window is made, so it cannot follow the measured bar,
			// and it is set to the standard title bar rather than over it:
			// any more than that and it eats clicks on the workspace.
			InvisibleTitleBarHeight: 28,
			// Hidden, not HiddenInset. The difference is one flag inside
			// them, UseToolbar, and it is the whole reason the app did not
			// look like a Mac's.
			//
			// A toolbar makes the title bar taller and macOS then insets
			// the three buttons further to centre them in it. Measured
			// against Terminal, VS Code and Chrome in the same screenshot,
			// all at the same scale: their close button sits 16, 17 and 20
			// points below their top edge and 16, 18 and 20 points in from
			// their left. Ours sat at 26 and 26. Six to ten points out in
			// both directions, every time, which is exactly
			// the amount that reads as wrong without being nameable.
			//
			// Without the toolbar macOS lays out an ordinary title bar and
			// puts the buttons where it puts everybody's, which is
			// Terminal's 16 and 16. The bar's height follows by itself,
			// because the app measures it rather than choosing it.
			TitleBar: application.MacTitleBarHidden,
		},
	})
	svc.chrome = watchChrome(app, svc.window)
	svc.window.OnWindowEvent(events.Common.WindowClosing, func(*application.WindowEvent) {
		leave.closing()
	})

	// A llama-server the last run left behind, because it crashed or was
	// killed, is stopped before this run loads a model of its own.
	if engine.StopLeftoverServer() {
		log.Printf("stopped a llama-server the last run of the app left behind")
	}
	svc.updates.start()
	err := app.Run()
	quit()
	if err != nil {
		log.Fatal(err)
	}
}

// widenPath adds the usual install folders. An app started from Finder does
// not get the shell's PATH, so Homebrew's ffmpeg and llama-server would not
// be found otherwise.
func widenPath() {
	var extra []string
	switch runtime.GOOS {
	case "darwin":
		extra = []string{"/opt/homebrew/bin", "/usr/local/bin"}
	case "linux":
		extra = []string{"/usr/local/bin", "/home/linuxbrew/.linuxbrew/bin"}
	}
	current := os.Getenv("PATH")
	parts := filepath.SplitList(current)
	for _, dir := range extra {
		found := false
		for _, p := range parts {
			if p == dir {
				found = true
			}
		}
		if !found {
			current += string(os.PathListSeparator) + dir
		}
	}
	_ = os.Setenv("PATH", current)
}

// mediaMiddleware serves episode files and their outputs under /media/ so
// the interface can show thumbnails and play clips. Only files that belong
// to an episode in the library are served.
func mediaMiddleware(st *store) application.Middleware {
	return func(next http.Handler) http.Handler {
		return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			if r.URL.Path == "/font" {
				// The interface writes the captions in the same face as the
				// render, which is one of the faces built into the program.
				data, ok := engine.FontBytes(r.URL.Query().Get("name"))
				if !ok {
					http.NotFound(w, r)
					return
				}
				w.Header().Set("Content-Type", "font/ttf")
				w.Header().Set("Cache-Control", "max-age=86400")
				_, _ = w.Write(data)
				return
			}
			if !strings.HasPrefix(r.URL.Path, "/media/") {
				next.ServeHTTP(w, r)
				return
			}
			path := r.URL.Query().Get("path")
			if path == "" || !filepath.IsAbs(path) || !st.Known(path) {
				http.NotFound(w, r)
				return
			}
			// Files, and nothing else. A folder would be answered with a
			// listing of what is in it.
			if info, err := os.Stat(path); err != nil || !info.Mode().IsRegular() {
				http.NotFound(w, r)
				return
			}
			http.ServeFile(w, r, path)
		})
	}
}

// notInLibrary is what a call is told when it names a file that does not
// belong to an episode in the library. The interface only ever names files
// it was given, so this is the last line rather than the first.
const notInLibrary = "this file does not belong to an episode in the library"

// FrameFairy is everything the interface can ask for.
type FrameFairy struct {
	app *application.App
	// The app's only window, kept so the bar can ask macOS where it put
	// the title bar and its buttons, and the watch that follows them.
	window *application.WebviewWindow
	chrome *chromeWatch
	store  *store
	jobs   *queue
	// The loudness of each episode, measured from the moment it is
	// added, which is the waveform, see levels.go.
	levels *measuring
	// Finding and installing a newer build of the app, see updates.go.
	updates *updating
	mu      sync.Mutex
	probed  map[string]engine.SourceInfo
	// The transcript of the episode being worked on, kept while the files
	// it was read from stay as they were.
	said   *engine.Transcript
	saidBy string
	// histories are the undo and redo of each episode, see history.go.
	histories map[string]*history
}

// Updates says which build is running, which channel it follows and how far
// a newer build has come. See docs/UPDATES.md.
// Asking also reads the channel list, at most once a minute, so the
// settings can offer the channels before the app has looked by itself.
func (s *FrameFairy) Updates() UpdateState {
	go s.updates.refreshList()
	return s.updates.State()
}

// FollowChannel picks the channel to update from, main or a pull request,
// and looks at once. Empty goes back to the channel the build came from.
func (s *FrameFairy) FollowChannel(channel string) error { return s.updates.Follow(channel) }

// CheckForUpdates looks for a newer build now, and downloads it.
func (s *FrameFairy) CheckForUpdates() { go s.updates.check() }

// OpenCommit opens the commit the running build was made from, on
// GitHub, in the browser. Only that commit, never one the interface names.
func (s *FrameFairy) OpenCommit() error {
	u := updates.CommitURL(buildCommit)
	if u == "" {
		return errors.New("this build was made on this Mac and has no commit")
	}
	if s.app == nil {
		return errors.New("there is no app to open it from")
	}
	return s.app.Browser.OpenURL(u)
}

// RestartToUpdate quits into the build that is ready.
func (s *FrameFairy) RestartToUpdate() error { return s.updates.Restart() }

// Platform is darwin, windows or linux.
func (s *FrameFairy) Platform() string { return runtime.GOOS }

// Licences is the notice of every piece of other people's work the app is
// made of or brings with it, for Acknowledgements in the Help menu.
func (s *FrameFairy) Licences() ([]notices.Notice, error) { return notices.All() }

// LicenceText is one of the texts a notice names. Only the texts built
// into the app can be read this way, whatever name is asked for.
func (s *FrameFairy) LicenceText(name string) (string, error) { return notices.Text(name) }

// Chrome says where macOS put its own furniture, or all zeros where the
// system draws its own title bar. The interface asks once and is
// told again on the "chrome" event whenever the answer changes.
func (s *FrameFairy) Chrome() Chrome {
	if s.chrome == nil {
		return Chrome{}
	}
	return s.chrome.measure()
}

// GetSettings returns the saved settings.
func (s *FrameFairy) GetSettings() Settings { return s.store.Settings() }

// SaveSettings takes the whole settings object back from the interface, so
// anything the interface does not know about would be lost on every save.
// Chosen is one of those: it is not a setting anybody edits, it is the
// record that the one setup question was answered, and losing it would put
// a customer back in the setup screen every time they changed a colour.
func (s *FrameFairy) SaveSettings(v Settings) error {
	return s.store.UpdateSettings(func(set *Settings) {
		chosen := set.Chosen
		*set = v
		set.Chosen = chosen
	})
}

// Check is one line of the setup check.
type Check struct {
	Name   string `json:"name"`
	OK     bool   `json:"ok"`
	Detail string `json:"detail"`
}

// CheckSetup looks for the tools and models the engine needs.
func (s *FrameFairy) CheckSetup(ctx context.Context) []Check {
	set := s.store.Settings()
	opts := set.options()
	var out []Check

	e := engine.NewEngine(engine.NewLog(io.Discard, false, false))
	e.UseTools(opts.FFmpeg, "")
	ff := Check{Name: "ffmpeg"}
	if err := e.Preflight(ctx); err != nil {
		ff.Detail = err.Error()
	} else {
		ff.OK = true
		ff.Detail = lookPath(e.FFmpeg)
	}
	out = append(out, ff)

	// Which of the system's own video decoders framing can use. It is not
	// a requirement, the processor decodes where there is none, so it is
	// never a problem. What a file really went through is in the log of
	// its search.
	decoding := Check{Name: "Video decoding", OK: true, Detail: "on the processor"}
	if ff.OK {
		var names []string
		for _, name := range e.SystemDecoders(ctx) {
			names = append(names, engine.DecoderName(name))
		}
		if len(names) > 0 {
			decoding.Detail = strings.Join(names, ", ") + ", falling back to the processor for a " +
				"file it does not take. The log of a search says which one it used"
		}
	}
	out = append(out, decoding)

	// Nothing to install: the faces are inside the program and are written
	// out next to the captions before every render.
	fonts := Check{Name: "Caption fonts", OK: true}
	var names []string
	for _, font := range engine.CaptionFonts() {
		names = append(names, font.Name)
	}
	fonts.Detail = "built in: " + strings.Join(names, ", ")
	out = append(out, fonts)

	asrDir := opts.ASRModel
	if asrDir == "" {
		asrDir = engine.DefaultModelDir()
	}
	speech := Check{Name: "Speech model", Detail: asrDir}
	speech.OK = engine.SpeechModelReady(asrDir)
	if !speech.OK {
		speech.Detail = "not found in " + asrDir + "\n" + engine.ModelHelp(asrDir)
	}
	out = append(out, speech)

	if opts.Planner == "api" {
		// The key of whichever company the model in the settings belongs to.
		p := engine.ProviderFor(opts.Model)
		key := Check{Name: p.Title + " API key"}
		if err := engine.CheckAPIKey(p); err != nil {
			key.Detail = err.Error()
		} else {
			key.OK, key.Detail = true, "found in the "+engine.KeySource(p)
		}
		return append(out, key)
	}

	server := opts.LLMServer
	if server == "" {
		server = engine.LlamaServerPath()
	}
	ls := Check{Name: "llama-server"}
	if found := lookPath(server); found != "" {
		ls.OK, ls.Detail = true, found
	} else {
		ls.Detail = server + " was not found. Install llama.cpp as docs/INSTALL.md describes, or set its path."
	}
	out = append(out, ls)

	lm := Check{Name: "Language model"}
	model := opts.LLMModel
	// Said in the app's own words: the engine's are for the command line,
	// and a flag to pass means nothing to somebody using the app.
	if model == "" {
		switch found := engine.LocalModelFiles(engine.ModelsDir()); len(found) {
		case 0:
			lm.Detail = "None is installed. Install one under Finding clips, or use the Claude API."
		case 1:
			model = found[0]
		default:
			lm.Detail = fmt.Sprintf("%d are installed and none is in use. Choose the one to find "+
				"clips with under Finding clips.", len(found))
		}
	}
	if model != "" {
		lm.OK = fileExists(model)
		lm.Detail = model
		// Chosen and not fetched yet, which is a step still to take rather
		// than a file gone missing.
		if known, ok := engine.LanguageModelByName(filepath.Base(model)); !lm.OK && ok {
			lm.Detail = known.Title + " is not downloaded yet."
		}
	}
	return append(out, lm)
}

func lookPath(name string) string {
	found, err := exec.LookPath(name)
	if err != nil {
		return ""
	}
	return found
}

func fileExists(path string) bool {
	info, err := os.Stat(path)
	return err == nil && info.Mode().IsRegular()
}

// Library lists the episodes with their state.
func (s *FrameFairy) Library() []engine.EpisodeStatus {
	asrDir := s.store.Settings().ASRModel
	out := []engine.EpisodeStatus{}
	for _, ep := range s.store.Episodes() {
		out = append(out, engine.Status(ep, asrDir))
	}
	return out
}

// Episode returns one episode's state.
func (s *FrameFairy) Episode(path string) engine.EpisodeStatus {
	if !s.store.Known(path) {
		return engine.EpisodeStatus{}
	}
	return engine.Status(path, s.store.Settings().ASRModel)
}

// ChooseFolder asks for a folder, the way a Mac app asks where to save
// things, and says which one was chosen, or nothing when the person
// cancelled. It changes nothing: the settings keep the answer when they
// are saved.
func (s *FrameFairy) ChooseFolder(title string) (string, error) {
	return s.app.Dialog.OpenFile().
		SetTitle(title).
		SetButtonText("Choose").
		CanChooseFiles(false).
		CanChooseDirectories(true).
		CanCreateDirectories(true).
		PromptForSingleSelection()
}

// AddEpisodes asks for video files and adds them to the library.
func (s *FrameFairy) AddEpisodes() ([]string, error) {
	dialog := s.app.Dialog.OpenFile().
		SetTitle("Add episodes").
		CanChooseFiles(true).
		AddFilter("Video", "*.mp4;*.mov;*.m4v;*.mkv")
	paths, err := dialog.PromptForMultipleSelection()
	if err != nil || len(paths) == 0 {
		return nil, err
	}
	var videos []string
	for _, p := range paths {
		if engine.IsVideo(p) {
			videos = append(videos, p)
		}
	}
	// One file that cannot be added is not a reason to drop the others, so
	// what was added is started either way and the reason travels with it.
	return s.addEpisodes(videos)
}

// addEpisodes adds videos to the library. A new episode's first search
// starts by itself, and hears the episode as far as its window and no
// further. An episode that has been searched before waits for a search to
// be asked for.
func (s *FrameFairy) addEpisodes(videos []string) ([]string, error) {
	added, err := s.store.AddEpisodes(videos)
	for _, v := range added {
		s.jobs.openEpisode(v)
		s.levels.start(v)
		s.firstSearch(context.Background(), v)
	}
	return added, err
}

// RemoveEpisode takes an episode out of the library. With deleteWork, it
// also deletes everything made for it, so adding it again starts from
// nothing. The episode file itself always stays.
func (s *FrameFairy) RemoveEpisode(path string, deleteWork bool) error {
	if !s.store.Known(path) {
		return os.ErrNotExist
	}
	// Its work stops, and nothing new starts on it until it is out of the
	// library, whether its files go or stay. A removed episode that went on
	// transcribing held the one transcription lane for hours, and every
	// episode added after it waited without a word.
	// It stays closed once it is gone, and opens again only if it stays in
	// the library or when it is added again.
	reopen := s.jobs.closeEpisode(path)
	// The loudness stops being measured before anything is deleted, so
	// nothing writes the work folder back. An episode that stays in the
	// library is measured again the next time its waveform is asked for.
	s.levels.stop(path)
	removed := false
	defer func() {
		if !removed {
			reopen()
		}
	}()
	if deleteWork {
		// Nothing is deleted while something is still writing it. A job
		// that will not stop leaves the episode where it is, files and
		// all, and says so, because the alternative is a folder deleted
		// under a running transcription which then writes it back: an
		// episode the person removed, still there, with half a transcript
		// in it. Removing it again once the work has stopped does what it
		// says.
		if !s.jobs.waitEpisode(path) {
			return errors.New("something is still running on this episode and would not stop, " +
				"so nothing was deleted. Stop it in Activity and remove the episode again")
		}
		if err := engine.DeleteWork(path); err != nil {
			return err
		}
	}
	s.forget(path)
	if err := s.store.RemoveEpisode(path); err != nil {
		return err
	}
	removed = true
	return nil
}

// SourceView is what the player needs to know about an episode.
type SourceView struct {
	Duration   float64 `json:"duration"`
	Width      int     `json:"width"`
	Height     int     `json:"height"`
	CropWidth  int     `json:"cropWidth"`
	CropHeight int     `json:"cropHeight"`
	// FPS is the episode's frame rate, which is what one step of the arrow
	// keys on the clip timeline is worth.
	FPS float64 `json:"fps"`
}

func (s *FrameFairy) probe(ctx context.Context, path string) (engine.SourceInfo, error) {
	stamp := ""
	if info, err := os.Stat(path); err == nil {
		stamp = fmt.Sprintf("%s|%d|%d", path, info.Size(), info.ModTime().UnixNano())
	}
	s.mu.Lock()
	cached, ok := s.probed[stamp]
	s.mu.Unlock()
	if ok {
		return cached, nil
	}
	e := engine.NewEngine(engine.NewLog(io.Discard, false, false))
	e.UseTools(s.store.Settings().FFmpeg, "")
	info, err := e.Probe(ctx, path)
	if err == nil && stamp != "" {
		s.mu.Lock()
		if s.probed == nil {
			s.probed = map[string]engine.SourceInfo{}
		}
		s.probed[stamp] = info
		s.mu.Unlock()
	}
	return info, err
}

// Source probes an episode.
func (s *FrameFairy) Source(ctx context.Context, path string) (SourceView, error) {
	if !s.store.Known(path) {
		return SourceView{}, os.ErrNotExist
	}
	info, err := s.probe(ctx, path)
	if err != nil {
		return SourceView{}, err
	}
	o := s.store.Settings().options()
	cw, ch := engine.CropWindow(info, o.Width, o.Height)
	return SourceView{Duration: info.Duration, Width: info.Width, Height: info.Height,
		CropWidth: cw, CropHeight: ch, FPS: info.FPS()}, nil
}

// ClipEntry is one clip of any clip set of an episode, with the left edge of
// its crop resolved for every segment, as the render will place it.
type ClipEntry struct {
	engine.ClipView
	Key       string `json:"key"`
	Plan      string `json:"plan"`
	CropLefts []int  `json:"cropLefts"`
}

// Clips lists the clips of every clip set of an episode, in time order.
func (s *FrameFairy) Clips(ctx context.Context, path string) ([]ClipEntry, error) {
	if !s.store.Known(path) {
		return nil, os.ErrNotExist
	}
	info, err := s.probe(ctx, path)
	if err != nil {
		return nil, err
	}
	o := s.store.Settings().options()
	cw, _ := engine.CropWindow(info, o.Width, o.Height)
	out := []ClipEntry{}
	for _, summary := range engine.Status(path, s.store.Settings().ASRModel).Plans {
		view, err := engine.ReadPlan(summary.Path)
		if err != nil {
			continue
		}
		for _, c := range view.Clips {
			entry := ClipEntry{ClipView: c, Key: summary.Name + "/" + c.ID, Plan: summary.Path}
			for _, seg := range c.Segments {
				entry.CropLefts = append(entry.CropLefts, engine.ClampCropX(seg.CropX, cw, info.Width))
			}
			out = append(out, entry)
		}
	}
	sort.SliceStable(out, func(a, b int) bool { return out[a].Start < out[b].Start })
	return out, nil
}

// WindowView is a part of an episode, in seconds. A searched part
// also says which plans cover it and how many clips they hold, so it can be
// let go of again.
type WindowView struct {
	From  float64  `json:"from"`
	To    float64  `json:"to"`
	Plans []string `json:"plans,omitempty"`
	Clips int      `json:"clips,omitempty"`
}

// CoverageView says where the model has already looked and what is left. A
// new window may only be drawn in what is left, so the same material is
// never put to the model twice.
type CoverageView struct {
	Searched []WindowView `json:"searched"`
	Free     []WindowView `json:"free"`
	// Passes is the whole episode in parts, each with how many searches
	// have read it, see engine.SearchPasses. New goes by it.
	Passes []PassView `json:"passes"`
}

// PassView is a part of an episode and how many searches have read it.
type PassView struct {
	From  float64 `json:"from"`
	To    float64 `json:"to"`
	Times int     `json:"times"`
}

// Coverage gives the parts of an episode that have been searched for
// clips and the parts that are still free, leaving out free parts
// too short to hold a clip of least seconds.
func (s *FrameFairy) Coverage(ctx context.Context, path string, least float64) (CoverageView, error) {
	if !s.store.Known(path) {
		return CoverageView{}, os.ErrNotExist
	}
	info, err := s.probe(ctx, path)
	if err != nil {
		return CoverageView{}, err
	}
	plans := engine.Status(path, s.store.Settings().ASRModel).Plans
	looked := engine.SearchedPlans(plans, info.Duration)
	searched := make([]engine.Window, 0, len(looked))
	out := CoverageView{Searched: []WindowView{}, Free: []WindowView{}, Passes: []PassView{}}
	for _, p := range engine.SearchPasses(plans, info.Duration) {
		out.Passes = append(out.Passes, PassView{From: p.Start, To: p.End, Times: p.Times})
	}
	for _, w := range looked {
		searched = append(searched, w.Window)
		out.Searched = append(out.Searched,
			WindowView{From: w.Start, To: w.End, Plans: w.Plans, Clips: w.Clips})
	}
	for _, w := range engine.FreeWindows(searched, info.Duration, least) {
		out.Free = append(out.Free, WindowView{From: w.Start, To: w.End})
	}
	return out, nil
}

// RemoveSearch gives a part of an episode back: the clips inside it
// leave the list and the part is free to be searched again. It is a
// part, not a whole search, so a part of what was searched can go while
// the rest of it stays. A plan with nothing left of its window goes
// altogether. Caption files are moved aside rather than deleted, and
// rendered files stay where they are.
//
// It answers with how many clips went.
func (s *FrameFairy) RemoveSearch(ctx context.Context, path string, from, to float64) (int, error) {
	if !s.store.Known(path) {
		return 0, os.ErrNotExist
	}
	if to <= from {
		return 0, nil
	}
	// The length is only needed to know when a plan made over the whole
	// episode has nothing left. A file that cannot be read still lets its
	// clips go.
	duration := 0.0
	if info, err := s.probe(ctx, path); err == nil {
		duration = info.Duration
	}
	gone := 0
	err := s.edit(path, func() error {
		for _, plan := range engine.Status(path, s.store.Settings().ASRModel).Plans {
			// A plan of this episode, named the way plans are named.
			// Nothing else is touched, whatever the interface asks for.
			if !s.store.PlanOf(path, plan.Path) {
				continue
			}
			n, err := engine.RemoveRange(plan.Path, from, to, duration)
			if err != nil {
				return err
			}
			gone += n
		}
		return nil
	})
	return gone, err
}

// Captions gives the captions of one clip, on the clip's own clock and in
// the look the render draws them in, so the interface can lay them over the
// picture while the clip plays.
func (s *FrameFairy) Captions(path, planPath, clipID string) (*engine.CaptionsView, error) {
	if !s.store.PlanOf(path, planPath) {
		return nil, os.ErrNotExist
	}
	t, err := s.words(path)
	if err != nil {
		return nil, err
	}
	return engine.ClipCaptionsView(planPath, clipID, t, s.captionOverrides(planPath))
}

// captionOverrides are the settings the render puts on top of a plan's
// caption style, so the picture shows what the file will hold.
func (s *FrameFairy) captionOverrides(planPath string) map[string]any {
	set := s.store.Settings()
	overrides := map[string]any{"margin_v": engine.SnapCaptionY(set.CaptionY)}
	// The highlight colour of the settings is for a plan that was not given
	// one of its own in the captions column, the same as the render has it.
	plan, _, err := engine.LoadClips(planPath)
	if err != nil {
		overrides["highlight_colour"] = set.HighlightColour
	} else if _, own := plan.CaptionStyle()["highlight_colour"]; !own {
		overrides["highlight_colour"] = set.HighlightColour
	}
	return overrides
}

// ArrivingCaptions are the captions of a clip on its way, the nth of a
// job, laid out the way they will be once it is written, in the style of
// the clip set it goes into. Nil until the job knows what the clip keeps.
// The workspace draws them on the clip timeline while the crop is placed.
func (s *FrameFairy) ArrivingCaptions(jobID string, n int) (*engine.CaptionsView, error) {
	for _, j := range s.jobs.list() {
		if j.ID != jobID {
			continue
		}
		if !s.store.Known(j.Episode) {
			return nil, os.ErrNotExist
		}
		logs := filepath.Join(engine.WorkDir(j.Episode), "logs")
		plan := filepath.Join(logs, engine.HandPlanName)
		if j.Kind == engine.JobSearch {
			// The search's own record says which pass of its window it
			// is, and so which plan it writes.
			rec := engine.JobRecord{From: j.From, To: j.To}
			if r := engine.ReadSearch(j.Episode); r != nil {
				rec = *r
			}
			duration := rec.To
			if info, err := s.probe(context.Background(), j.Episode); err == nil {
				duration = info.Duration
			}
			plan = filepath.Join(logs, rec.PlanName(duration))
		}
		for _, u := range j.Underway {
			if u.N == n {
				t, err := s.words(j.Episode)
				if err != nil {
					return nil, err
				}
				return engine.ArrivingCaptionsView(plan, u, t, s.captionOverrides(plan)), nil
			}
		}
		return nil, nil
	}
	return nil, nil
}

// Fonts are the faces the captions can be written in. They travel with the
// program, so every one of them renders on any machine.
func (s *FrameFairy) Fonts() []engine.CaptionFont { return engine.CaptionFonts() }

// transcript is the whole-episode transcript, read once and kept, for the
// two calls the timeline makes over and over as it is moved. A four hour
// transcript is megabytes of words and levels, and reading it for every
// swipe made the timeline crawl and the waveform arrive in steps.
//
// What it hands out is shared, so only what reads goes through here.
func (s *FrameFairy) transcript(p *engine.Project) (*engine.Transcript, error) {
	stamp := p.Source
	for _, file := range engine.TranscriptFiles(p.LogsDir()) {
		info, err := os.Stat(file)
		if err != nil {
			stamp += "|-"
			continue
		}
		stamp += fmt.Sprintf("|%d,%d", info.Size(), info.ModTime().UnixNano())
	}
	s.mu.Lock()
	if s.said != nil && s.saidBy == stamp {
		kept := s.said
		s.mu.Unlock()
		return kept, nil
	}
	s.mu.Unlock()
	t, err := p.Transcript()
	if err != nil {
		return nil, err
	}
	s.mu.Lock()
	s.said, s.saidBy = t, stamp
	s.mu.Unlock()
	return t, nil
}

// words is what an episode says, see engine/words.go, from the transcript
// read once and kept. An episode not transcribed yet says nothing, which
// is an empty answer and not a failure.
func (s *FrameFairy) words(path string) (*engine.Transcript, error) {
	t, err := s.transcript(engine.NewProject(nil, path, s.store.Settings().options()))
	if errors.Is(err, engine.ErrNoTranscript) {
		return &engine.Transcript{}, nil
	}
	return t, err
}

// Waveform returns the loudest level in each of buckets pieces of a part
// of the episode, from its loudness measured on its own or from its
// transcript, whichever reaches further. An episode not measured yet has no
// waveform yet, which is an empty answer and not a failure.
//
// Peaks never answers with more buckets than it measured, so the interface
// is told how fine the measurement was and can draw that finely and no
// finer.
func (s *FrameFairy) Waveform(path string, from, to float64, buckets int) ([]float32, error) {
	if !s.store.Known(path) {
		return nil, os.ErrNotExist
	}
	// An episode shown is an episode measured: one added before the
	// loudness had a job of its own is measured the first time it is
	// opened. What the clip timeline asks for is what it shows, and the
	// measuring goes there first.
	s.levels.look(path, from, to)
	s.levels.start(path)
	p := engine.NewProject(nil, path, s.store.Settings().options())
	t, err := s.transcript(p)
	if err != nil && !errors.Is(err, engine.ErrNoTranscript) {
		return nil, err
	}
	// The loudness measured on its own and the transcription's are the
	// same frames from the same samples, so the waveform has whatever
	// either has measured. The measuring runs ahead of the transcription,
	// and a transcript made before it existed is there before it has run.
	t = s.levels.read(path).Over(t)
	if t == nil {
		return []float32{}, nil
	}
	if to <= from {
		to = t.Duration()
	}
	return t.Peaks(from, to, min(max(buckets, 1), 4000)), nil
}

// windowLength is how long the window of a search is, in seconds. A search
// of the whole episode is taken as an hour, which only sizes the model's
// memory, and the search starts it again if it needs more.
func windowLength(req engine.PlanRequest) float64 {
	if req.To > req.From {
		return req.To - req.From
	}
	return 3600
}

// Still returns a frame of the episode at a moment, as a media path.
func (s *FrameFairy) Still(ctx context.Context, path string, at float64, width int) (string, error) {
	if !s.store.Known(path) {
		return "", os.ErrNotExist
	}
	// The frame rate says which frame the moment falls in, the one the
	// video preview shows there. It is read once per episode and kept.
	fps := 0.0
	if info, err := s.probe(ctx, path); err == nil && info.FPSDen > 0 {
		fps = info.FPS()
	}
	e := engine.NewEngine(engine.NewLog(io.Discard, false, false))
	if ff := s.store.Settings().FFmpeg; ff != "" {
		e.FFmpeg = ff
	}
	return e.Still(ctx, path, at, fps, width)
}

// Render queues rendering clips of a plan. The render keeps a record of
// the clips it has finished, so one that is cut off carries on with the
// rest, see search.go.
func (s *FrameFairy) Render(path string, req engine.RenderRequest) Job {
	return s.render(path, req, nil)
}

// Words returns the words said in a part, for walking the playhead from
// word to word. Before the first transcription there are none, which is an
// empty answer and not a failure.
func (s *FrameFairy) Words(path string, from, to float64) ([]engine.WordView, error) {
	if !s.store.Known(path) {
		return nil, os.ErrNotExist
	}
	t, err := s.words(path)
	if err != nil {
		return nil, err
	}
	out := []engine.WordView{}
	// One word either side, so a step can reach past the part.
	for _, w := range t.WordsBetween(from-5, to+5) {
		out = append(out, engine.WordView{Start: w.Start, End: w.End, Text: w.Text})
	}
	return out, nil
}

func (s *FrameFairy) SetWord(ctx context.Context, path, plan, clipID string, start float64, text string) (ClipEntry, error) {
	if !s.store.PlanOf(path, plan) {
		return ClipEntry{}, os.ErrNotExist
	}
	p := engine.NewProject(nil, path, s.store.Settings().options())
	// Read fresh, not from what Waveform and Words keep: an edit works on
	// the words themselves, and nothing else may be holding them.
	t, err := p.Transcript()
	if err != nil {
		return ClipEntry{}, err
	}
	if err := s.edit(path, func() error {
		return engine.SetWordText(p.LogsDir(), start, text, t)
	}); err != nil {
		return ClipEntry{}, err
	}
	return s.clipEntry(ctx, path, plan, clipID)
}

// SetCrop places the crop of the shot at a moment of a clip by hand, as the
// left edge in source pixels, and returns the clip as it is now.
func (s *FrameFairy) SetCrop(ctx context.Context, path, plan, clipID string, at float64, left int) (ClipEntry, error) {
	if !s.store.PlanOf(path, plan) {
		return ClipEntry{}, os.ErrNotExist
	}
	info, err := s.probe(ctx, path)
	if err != nil {
		return ClipEntry{}, err
	}
	o := s.store.Settings().options()
	cw, _ := engine.CropWindow(info, o.Width, o.Height)
	left = max(0, min(left, info.Width-cw))
	if err := s.edit(path, func() error { return engine.SetCrop(plan, clipID, at, left) }); err != nil {
		return ClipEntry{}, err
	}
	return s.clipEntry(ctx, path, plan, clipID)
}

// SetCaptionStyle changes the face and the size of the captions of a whole
// clip set, which is what the workspace offers next to the clip.
func (s *FrameFairy) SetCaptionStyle(ctx context.Context, path, plan, font string, size float64) error {
	if !s.store.PlanOf(path, plan) {
		return os.ErrNotExist
	}
	values := map[string]any{}
	if font != "" {
		values["font"] = font
	}
	if size > 0 {
		values["size"] = size
	}
	return s.edit(path, func() error { return engine.SetCaptionStyle(plan, values) })
}

// SetCaptionColours changes the colour of the caption text, of the box
// behind it and of the pill behind the word being spoken, each with how
// opaque it is from 0 to 1, for a whole clip set, beside the face and the
// size. Colours come as #RRGGBB, and an empty one is left as it is.
func (s *FrameFairy) SetCaptionColours(ctx context.Context, path, plan, text string,
	textOpacity float64, box string, boxOpacity float64, highlight string,
	highlightOpacity float64) error {
	if !s.store.PlanOf(path, plan) {
		return os.ErrNotExist
	}
	values := map[string]any{}
	if text != "" {
		colour, ok := engine.AssColour(text, textOpacity)
		if !ok {
			return fmt.Errorf("%s is not a colour", engine.Scrub(text, 20))
		}
		values["primary"] = colour
	}
	if box != "" {
		colour, ok := engine.AssColour(box, boxOpacity)
		if !ok {
			return fmt.Errorf("%s is not a colour", engine.Scrub(box, 20))
		}
		values["back_colour"] = colour
	}
	if highlight != "" {
		colour, ok := engine.AssColour(highlight, highlightOpacity)
		if !ok {
			return fmt.Errorf("%s is not a colour", engine.Scrub(highlight, 20))
		}
		values["highlight_colour"] = colour
	}
	return s.edit(path, func() error { return engine.SetCaptionStyle(plan, values) })
}

// SetCaptionSwitch turns one of the switches of the captions column on or
// off for a whole clip set: "text", captions burned in at all, "box", the
// box behind them, and "highlight", the pill behind the word being spoken
// and the bounce it makes. Anything else is refused.
func (s *FrameFairy) SetCaptionSwitch(ctx context.Context, path, plan, which string, on bool) error {
	if !s.store.PlanOf(path, plan) {
		return os.ErrNotExist
	}
	switch which {
	case "text", "box", "highlight":
	default:
		return fmt.Errorf("%q is no switch of the captions", which)
	}
	return s.edit(path, func() error {
		return engine.SetCaptionStyle(plan, map[string]any{which: on})
	})
}

// SetCaptionsHeight puts the captions where the box was dragged to, as the
// distance from the bottom of a 1080x1920 frame. There is one place for
// every clip of every episode, because a place that suits one video suits
// the next one, so this is a setting and not an edit of a clip.
//
// Clips that were placed by hand before are put back on it, or the picture
// would say one thing and the render do another.
func (s *FrameFairy) SetCaptionsHeight(path string, y float64) error {
	if !s.store.Known(path) {
		return os.ErrNotExist
	}
	return s.edit(path, func() error {
		if err := s.store.UpdateSettings(func(set *Settings) { set.CaptionY = engine.SnapCaptionY(y) }); err != nil {
			return err
		}
		return s.followTheHeight(path)
	})
}

// ResetCaptionsHeight puts the captions back where the app puts them.
func (s *FrameFairy) ResetCaptionsHeight(path string) error {
	if !s.store.Known(path) {
		return os.ErrNotExist
	}
	return s.edit(path, func() error {
		if err := s.store.UpdateSettings(func(set *Settings) { set.CaptionY = engine.DefaultCaptionY }); err != nil {
			return err
		}
		return s.followTheHeight(path)
	})
}

// SetSearch keeps how many clips a search looks for and how long they may
// be. They are set in the workspace, where the episode they are about is,
// and kept for the next episode as well, because a person who wants short
// clips wants them everywhere. A target of 0 follows the window. The
// numbers are held to the same range the controls offer, because what
// arrives here is not to be trusted.
func (s *FrameFairy) SetSearch(target int, window, min, max float64) error {
	return s.store.UpdateSettings(func(set *Settings) {
		set.Target, set.TargetWindow = 0, 0
		if target > 0 && window > 0 {
			set.Target = int(hold(float64(target), 1, 30))
			set.TargetWindow = hold(window, 1, engine.MaxEpisodeSeconds)
		}
		set.Min = hold(min, 5, 180)
		set.Max = hold(max, 5, 180)
		if set.Min > set.Max {
			set.Max = set.Min
		}
	})
}

// hold keeps a number inside the range the interface offers. What arrives
// from it is not to be trusted, here no more than anywhere else.
func hold(n, low, high float64) float64 {
	if !(n >= low) {
		return low
	}
	if n > high {
		return high
	}
	return n
}

// followTheHeight takes the hand-placed caption line off the clips of an
// episode, so every one of them sits where the setting says.
func (s *FrameFairy) followTheHeight(path string) error {
	for _, plan := range engine.Status(path, s.store.Settings().ASRModel).Plans {
		if !s.store.PlanOf(path, plan.Path) {
			continue
		}
		if _, err := engine.ClearCaptionY(plan.Path); err != nil {
			return err
		}
	}
	return nil
}

// SetThumbnail adds, moves or removes a thumbnail of a clip, a moment of
// the episode the render takes a picture of the short at, and returns the
// clip as it is now. A from below nought adds one at to, and a to below
// nought removes the one at from.
func (s *FrameFairy) SetThumbnail(ctx context.Context, path, plan, clipID string, from, to float64) (ClipEntry, error) {
	if !s.store.PlanOf(path, plan) {
		return ClipEntry{}, os.ErrNotExist
	}
	if err := s.edit(path, func() error {
		return engine.SetThumbnail(plan, clipID, from, to)
	}); err != nil {
		return ClipEntry{}, err
	}
	return s.clipEntry(ctx, path, plan, clipID)
}

// SetCaptionTime moves the caption of a clip that begins or ends on a word,
// to appear or go at a moment of the episode. A moment below nought puts it
// back where its words put it, because JSON has no way to say not a number.
func (s *FrameFairy) SetCaptionTime(ctx context.Context, path, plan, clipID string, word float64, edge string, at float64) (ClipEntry, error) {
	if !s.store.PlanOf(path, plan) {
		return ClipEntry{}, os.ErrNotExist
	}
	if at < 0 {
		at = math.NaN()
	}
	t, err := s.words(path)
	if err != nil {
		return ClipEntry{}, err
	}
	if err := s.edit(path, func() error {
		return engine.SetCaptionTime(plan, clipID, word, edge, at, t)
	}); err != nil {
		return ClipEntry{}, err
	}
	return s.clipEntry(ctx, path, plan, clipID)
}

// ResetCrop brings back the automatic crop for the shot at a moment of a
// clip, and returns the clip as it is now.
func (s *FrameFairy) ResetCrop(ctx context.Context, path, plan, clipID string, at float64) (ClipEntry, error) {
	if !s.store.PlanOf(path, plan) {
		return ClipEntry{}, os.ErrNotExist
	}
	if err := s.edit(path, func() error { return engine.ResetCrop(plan, clipID, at) }); err != nil {
		return ClipEntry{}, err
	}
	return s.clipEntry(ctx, path, plan, clipID)
}

// ClipPlayed records that a clip was watched in the app.
func (s *FrameFairy) ClipPlayed(plan, clipID string) error {
	if !s.store.Known(plan) {
		return os.ErrNotExist
	}
	return engine.RecordDecision(plan, clipID, engine.DecisionViewed, nil)
}

// chosenFile is where an episode keeps what was last chosen in it, the
// clip worked on and the window on the range picker. It is about that
// episode and nothing else, so it sits in the episode's own folder and
// goes when the folder goes.
const chosenFile = "chosen.json"

// chosen is the whole of that file. A struct rather than a bare string so
// a later version can keep more without the older one choking on it, which
// is how the window came to be kept beside the clip.
type chosen struct {
	Clip   string      `json:"clip"`
	Window *KeptWindow `json:"window,omitempty"`
}

// KeptWindow is the window as it was left: where it starts and ends, and
// how long it was made, which is longer than it is when the end of the
// episode cut it short.
type KeptWindow struct {
	From   float64 `json:"from"`
	To     float64 `json:"to"`
	Length float64 `json:"length"`
}

// ok says whether a window could be one: numbers, in order, inside the
// longest episode there is. It arrives from the interface, and it is read
// back from a file anything could have written.
func (w KeptWindow) ok() bool {
	for _, v := range []float64{w.From, w.To, w.Length} {
		if math.IsNaN(v) || math.IsInf(v, 0) || v < 0 || v > engine.MaxEpisodeSeconds {
			return false
		}
	}
	return w.To > w.From && w.Length >= w.To-w.From-0.001
}

// chosenMu keeps two changes to the same file from crossing, a clip chosen
// while the window is let go, each reading the file and writing it back.
var chosenMu sync.Mutex

func readChosen(path string) chosen {
	var held chosen
	file, err := engine.SafeChild(engine.WorkDir(path), chosenFile)
	if err != nil {
		return held
	}
	data, err := os.ReadFile(file)
	if err != nil || json.Unmarshal(data, &held) != nil {
		return chosen{}
	}
	if !looksLikeClipKey(held.Clip) {
		held.Clip = ""
	}
	if held.Window != nil && !held.Window.ok() {
		held.Window = nil
	}
	return held
}

// changeChosen reads the file, changes it and writes it back whole, through
// a file of its own, so it is never found half written.
func changeChosen(path string, change func(*chosen)) error {
	chosenMu.Lock()
	defer chosenMu.Unlock()
	held := readChosen(path)
	change(&held)
	dir := engine.WorkDir(path)
	if err := os.MkdirAll(dir, 0o755); err != nil {
		return err
	}
	file, err := engine.SafeChild(dir, chosenFile)
	if err != nil {
		return err
	}
	body, err := json.Marshal(held)
	if err != nil {
		return err
	}
	hold, err := os.CreateTemp(dir, "chosen-*.json")
	if err != nil {
		return err
	}
	tmp := hold.Name()
	if _, err := hold.Write(body); err != nil {
		hold.Close()
		os.Remove(tmp)
		return err
	}
	if err := hold.Close(); err != nil {
		os.Remove(tmp)
		return err
	}
	if err := os.Rename(tmp, file); err != nil {
		os.Remove(tmp)
		return err
	}
	return nil
}

// ChooseClip remembers which clip of an episode is being worked on, so
// opening the episode again opens on the same one. An empty key forgets it.
//
// The key names a clip set and a clip inside it. It arrives from the
// interface, so it is never joined onto a path and never used to reach a
// file: it is written down as it is and only ever compared with the keys
// the app works out for itself. Anything longer than a key could be, or
// carrying anything a key never carries, is refused rather than stored.
func (s *FrameFairy) ChooseClip(path, key string) error {
	if !s.store.Known(path) {
		return os.ErrNotExist
	}
	if !looksLikeClipKey(key) {
		return errors.New("that is not the key of a clip")
	}
	return changeChosen(path, func(c *chosen) { c.Clip = key })
}

// ChosenClip gives back the clip an episode was last worked on, or an empty
// string where there is none or where what is written down is not a key.
// The interface checks it against the clips it has either way: a clip set
// that has been searched again no longer holds it.
func (s *FrameFairy) ChosenClip(path string) string {
	if !s.store.Known(path) {
		return ""
	}
	return readChosen(path).Clip
}

// ChooseWindow remembers the window on an episode's range picker as it was
// left, so the app opens on it again after a restart rather than on one of
// its own choosing. length is how long the window was made. A window moved
// by a hand, dragged or put back with a double-click, is a step that undo
// takes back. One the app moved by itself, after a search, is not.
func (s *FrameFairy) ChooseWindow(path string, from, to, length float64, byHand bool) error {
	if !s.store.Known(path) {
		return os.ErrNotExist
	}
	w := KeptWindow{From: from, To: to, Length: length}
	if !w.ok() {
		return errors.New("that is not a window")
	}
	h := s.historyOf(path)
	h.mu.Lock()
	defer h.mu.Unlock()
	was := readChosen(path).Window
	if err := changeChosen(path, func(c *chosen) { c.Window = &w }); err != nil {
		return err
	}
	if byHand && (was == nil || *was != w) {
		h.undo = append(h.undo, step{window: &[2]*KeptWindow{was, &w}})
		if len(h.undo) > historyDepth {
			h.undo = h.undo[len(h.undo)-historyDepth:]
		}
		h.redo = nil
	}
	return nil
}

// ChosenWindow gives back the window an episode was left with, or nil where
// there is none or what is written down is not a window. The interface
// checks it against the episode's length, which it knows and this does not.
func (s *FrameFairy) ChosenWindow(path string) *KeptWindow {
	if !s.store.Known(path) {
		return nil
	}
	return readChosen(path).Window
}

// looksLikeClipKey reports whether a string could be the key of a clip: a
// clip set name, a slash, and the clip's own name. An empty key is allowed
// and means no clip.
func looksLikeClipKey(key string) bool {
	if key == "" {
		return true
	}
	if len(key) > 300 || strings.Count(key, "/") != 1 {
		return false
	}
	for _, r := range key {
		if r < 0x20 || r == 0x7f {
			return false
		}
	}
	name, id, _ := strings.Cut(key, "/")
	return name != "" && id != ""
}

func (s *FrameFairy) clipEntry(ctx context.Context, path, plan, clipID string) (ClipEntry, error) {
	clips, err := s.Clips(ctx, path)
	if err != nil {
		return ClipEntry{}, err
	}
	for _, c := range clips {
		if c.Plan == plan && c.ID == clipID {
			return c, nil
		}
	}
	return ClipEntry{}, os.ErrNotExist
}

// RemoveClip takes a clip out of the list, or puts it back. The clip stays
// in the plan either way, so putting it back loses nothing of what was done
// to it. A render of the whole plan leaves a removed clip out.
func (s *FrameFairy) RemoveClip(ctx context.Context, path, plan, clipID string, removed bool) (ClipEntry, error) {
	if !s.store.PlanOf(path, plan) {
		return ClipEntry{}, os.ErrNotExist
	}
	if err := s.edit(path, func() error { return engine.SetRejected(plan, clipID, removed) }); err != nil {
		return ClipEntry{}, err
	}
	return s.clipEntry(ctx, path, plan, clipID)
}

// Shape works out what a gesture on the clip timeline makes of a clip,
// with its captions, while the hand moves, and writes nothing. See
// engine/shape.go.
func (s *FrameFairy) Shape(path, plan, clipID string, g engine.Gesture) (*engine.ShapedView, error) {
	if !s.store.PlanOf(path, plan) {
		return nil, os.ErrNotExist
	}
	t, err := s.words(path)
	if err != nil {
		return nil, err
	}
	return engine.ShapeClipView(plan, clipID, g, t, s.store.Settings().options().KeepPause,
		s.captionOverrides(plan))
}

// Reshape makes the change a gesture on the clip timeline showed while the
// hand moved, and returns the clip as it is now.
func (s *FrameFairy) Reshape(ctx context.Context, path, plan, clipID string, g engine.Gesture) (ClipEntry, error) {
	if !s.store.PlanOf(path, plan) {
		return ClipEntry{}, os.ErrNotExist
	}
	t, err := s.words(path)
	if err != nil {
		return ClipEntry{}, err
	}
	if err := s.edit(path, func() error {
		return engine.Reshape(plan, clipID, g, t, s.store.Settings().options().KeepPause)
	}); err != nil {
		return ClipEntry{}, err
	}
	return s.clipEntry(ctx, path, plan, clipID)
}

// Jobs lists queued, running and finished jobs.
func (s *FrameFairy) Jobs() []Job { return s.jobs.list() }

// CancelJob stops a job, or takes it out of the queue.
func (s *FrameFairy) CancelJob(id string) {
	// A search or a render takes its record with it, see search.go.
	if s.cancelSteps(id) {
		return
	}
	s.jobs.cancel(id)
}

// ClearJobs forgets finished jobs.
func (s *FrameFairy) ClearJobs() { s.jobs.clear() }

// Reveal shows a file in Finder, Explorer or the file manager.
func (s *FrameFairy) Reveal(path string) error {
	if !s.store.Known(path) {
		return os.ErrNotExist
	}
	switch runtime.GOOS {
	case "darwin":
		return exec.Command("open", "-R", path).Start()
	case "windows":
		// One argument, not two: with a space after the comma Explorer
		// opens its default folder and selects nothing.
		return exec.Command("explorer", "/select,"+path).Start()
	}
	dir := path
	if fileExists(path) {
		dir = filepath.Dir(path)
	}
	return exec.Command("xdg-open", dir).Start()
}

// TrainingStatus is the one folder the records live in and how much is in
// it, for the settings screen.
type TrainingStatus struct {
	Dir       string `json:"dir"`
	Plans     int    `json:"plans"`
	Decisions int    `json:"decisions"`
}

// Training says where the records are and how many there are. The folder
// belongs to the person, not to an episode, so it is named as it is on
// disk rather than hidden behind a setting.
func (s *FrameFairy) Training() TrainingStatus {
	dir := engine.TrainingDir()
	return TrainingStatus{
		Dir:       dir,
		Plans:     countRecords(filepath.Join(dir, "plans.jsonl")),
		Decisions: countRecords(filepath.Join(dir, "decisions.jsonl")),
	}
}

// countRecords counts the lines of a record file. A missing file is none.
func countRecords(path string) int {
	f, err := os.Open(path)
	if err != nil {
		return 0
	}
	defer f.Close()
	n := 0
	scanner := bufio.NewScanner(f)
	scanner.Buffer(make([]byte, 1<<20), 64<<20)
	for scanner.Scan() {
		if len(strings.TrimSpace(scanner.Text())) > 0 {
			n++
		}
	}
	return n
}

// ClearTraining removes the records, all of them. What they were for is
// training a model that is not trained yet, so this is a person tidying up
// while the app is being built, and it cannot be taken back.
func (s *FrameFairy) ClearTraining() error {
	dir := engine.TrainingDir()
	for _, name := range []string{"plans.jsonl", "decisions.jsonl"} {
		if err := os.Remove(filepath.Join(dir, name)); err != nil && !os.IsNotExist(err) {
			return err
		}
	}
	return nil
}
