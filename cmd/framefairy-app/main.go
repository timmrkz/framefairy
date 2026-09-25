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
	widenPath()
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
	// Nothing the app started outlives it: a model loaded or still loading
	// is stopped and no other is loaded, and the jobs are stopped, and with
	// them any ffmpeg they run. On macOS app.Run never returns: Cmd+Q ends
	// the process from inside Cocoa once the shutdown hooks have run, so
	// this has to be one of them. Code after app.Run only runs on the other
	// systems, and left a llama-server behind on every Mac that quit during
	// a search.
	quit := sync.OnceFunc(func() {
		engine.CloseModels()
		svc.jobs.shutDown()
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
	app.Menu.Set(appMenu(app))

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
	mu     sync.Mutex
	probed map[string]engine.SourceInfo
	// The transcript of the episode being worked on, kept while the files
	// it was read from stay as they were.
	said   *engine.Transcript
	saidBy string
	// histories are the undo and redo of each episode, see history.go.
	histories map[string]*history
	// holds are where each episode's transcription stops for now: the end
	// of the window its first search is waiting for, see HoldTranscription.
	holds map[string]float64
}

// Version of the engine.
func (s *FrameFairy) Version() string { return engine.Version }

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
	if opts.FFmpeg != "" {
		e.FFmpeg = opts.FFmpeg
		if guess := filepath.Join(filepath.Dir(opts.FFmpeg), "ffprobe"); fileExists(guess) {
			e.FFprobe = guess
		}
	}
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
	speech.OK = fileExists(filepath.Join(asrDir, "tokens.txt"))
	if !speech.OK {
		speech.Detail = "not found in " + asrDir + "\n" + engine.ModelHelp(asrDir)
	}
	out = append(out, speech)

	if opts.Planner == "api" {
		key := Check{Name: "Claude API key"}
		if _, err := engine.ReadAPIKey(ctx); err != nil {
			key.Detail = err.Error()
		} else {
			key.OK, key.Detail = true, "found"
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
				"clips with under Finding clips, with Use.", len(found))
		}
	}
	if model != "" {
		lm.OK = fileExists(model)
		lm.Detail = model
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
	added, err := s.store.AddEpisodes(videos)
	// A new episode's first search starts by itself, so its transcription
	// starts right away, in the background, and goes as far as that
	// search's window and no further. The window is the first half hour,
	// or the whole of a shorter episode, which the workspace holds it to
	// once it is open. firstLook is a part of every first window, so the
	// transcription never runs past the one it is for. An episode that
	// has been searched before transcribes when a search needs it.
	for _, v := range added {
		s.jobs.openEpisode(v)
		s.transcribeForFirstSearch(v)
	}
	return added, err
}

// transcribeForFirstSearch starts the transcription of an episode that has
// never been searched, held at the start of its first window.
func (s *FrameFairy) transcribeForFirstSearch(path string) {
	covered, done := engine.Coverage(path, s.store.Settings().ASRModel)
	if done || engine.Looked(path) || covered >= firstLook {
		return
	}
	_ = s.HoldTranscription(path, firstLook)
	s.Transcribe(path)
}

// firstLook is the start of every episode's first window, in seconds. The
// workspace's own is firstLook in Episode.svelte.
const firstLook = 30 * 60

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
	if ff := s.store.Settings().FFmpeg; ff != "" {
		e.FFmpeg = ff
		if guess := filepath.Join(filepath.Dir(ff), "ffprobe"); fileExists(guess) {
			e.FFprobe = guess
		}
	}
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
	out := CoverageView{Searched: []WindowView{}, Free: []WindowView{}}
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
	p := engine.NewProject(nil, path, s.store.Settings().options())
	logs := engine.ResolvePath(p.LogsDir())
	gone := 0
	err := s.edit(path, func() error {
		for _, plan := range engine.Status(path, s.store.Settings().ASRModel).Plans {
			// A plan of this episode, named the way plans are named.
			// Nothing else is touched, whatever the interface asks for.
			if !s.store.Known(plan.Path) || filepath.Dir(engine.ResolvePath(plan.Path)) != logs {
				continue
			}
			n, err := engine.RemoveRange(plan.Path, p.CaptionsDir(), from, to, duration)
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
func (s *FrameFairy) Captions(planPath, clipID string) (*engine.CaptionsView, error) {
	if !s.store.Known(planPath) {
		return nil, os.ErrNotExist
	}
	// The same overrides the render puts on top of the plan, so the picture
	// shows what the file will hold.
	set := s.store.Settings()
	overrides := map[string]any{"margin_v": engine.SnapCaptionY(set.CaptionY)}
	// The highlight colour of the settings is for a plan that was not given
	// one of its own in the captions column, the same as the render has it.
	if plan, _, err := engine.LoadClips(planPath); err == nil {
		if _, own := plan.CaptionStyle()["highlight_colour"]; !own {
			overrides["highlight_colour"] = set.HighlightColour
		}
	}
	return engine.ClipCaptionsView(planPath, clipID, overrides)
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

// Waveform returns the loudest level in each of buckets pieces of a part
// of the episode.
// An episode waiting for its first transcription has no waveform yet, which
// is an empty answer and not a failure.
//
// Peaks never answers with more buckets than it measured, so the interface
// is told how fine the measurement was and can draw that finely and no
// finer.
func (s *FrameFairy) Waveform(path string, from, to float64, buckets int) ([]float32, error) {
	if !s.store.Known(path) {
		return nil, os.ErrNotExist
	}
	p := engine.NewProject(nil, path, s.store.Settings().options())
	t, err := s.transcript(p)
	if errors.Is(err, engine.ErrNoTranscript) {
		return []float32{}, nil
	}
	if err != nil {
		return nil, err
	}
	if to <= from {
		to = t.Duration()
	}
	return t.Peaks(from, to, min(max(buckets, 1), 4000)), nil
}

// Transcribe queues a transcription of the whole episode, unless one is
// already queued or running. A stopped one carries on where it got to.
func (s *FrameFairy) Transcribe(path string) Job {
	if !s.store.Known(path) {
		return s.jobs.refuse(path, "transcribe", "Transcription", notInLibrary)
	}
	// Once, however many times it is asked for. Looking and then adding is
	// two locks with a gap between them, and two calls arriving together
	// both looked, both saw nothing and both added, which is two
	// transcriptions of one episode.
	return s.jobs.addOnce(path, "transcribe", "Transcription", func(ctx context.Context, p *engine.Project) (string, error) {
		p.StopAt(func() float64 { return s.holdOf(path) })
		return "", p.Transcribe(ctx)
	})
}

// HoldTranscription tells the transcription of an episode where to stop for
// now: the end of the window its first search is waiting for. The chunk the
// speech model hears is cut exactly there, so the transcription stops on the
// window's edge and the search starts at once, rather than a pause arriving
// from outside a chunk or two too late. 0 lets go of it, and a transcription
// that had stopped there carries on.
func (s *FrameFairy) HoldTranscription(path string, at float64) error {
	if !s.store.Known(path) {
		return os.ErrNotExist
	}
	if at > 0 {
		s.mu.Lock()
		if s.holds == nil {
			s.holds = map[string]float64{}
		}
		s.holds[path] = at
		s.mu.Unlock()
		return nil
	}
	// Letting go only lets go. The transcription runs for a search and for
	// nothing else, so one that stopped at its hold stays stopped until a
	// search needs more of the episode.
	s.releaseHold(path)
	return nil
}

func (s *FrameFairy) holdOf(path string) float64 {
	s.mu.Lock()
	defer s.mu.Unlock()
	return s.holds[path]
}

// releaseHold lets go of an episode's hold and says whether it had one.
func (s *FrameFairy) releaseHold(path string) bool {
	s.mu.Lock()
	defer s.mu.Unlock()
	_, held := s.holds[path]
	delete(s.holds, path)
	return held
}

// Plan queues finding clips in a window of an episode. It starts as soon as
// the transcript reaches the end of the window, while the rest of the
// episode is still being transcribed.
func (s *FrameFairy) Plan(path string, req engine.PlanRequest) Job {
	if !s.store.Known(path) {
		return s.jobs.refuse(path, "plan", "Find clips", notInLibrary)
	}
	// From here on this episode has been looked at, whoever asked. The app
	// only ever searches by itself for an episode nobody has searched, so
	// removing the clips again never brings a search of its own back.
	if err := engine.MarkLooked(path); err != nil {
		log.Printf("could not note the search of %s: %v", path, err)
	}
	return s.jobs.add(path, "plan", "Find clips", func(ctx context.Context, p *engine.Project) (string, error) {
		// The model loads while the transcript is still on its way, so
		// the search has nothing to wait for once it is there.
		if covered, done := engine.Coverage(p.Source, s.store.Settings().ASRModel); !done && covered < req.To-0.05 {
			go func() {
				defer func() { _ = recover() }()
				if err := p.WarmModel(ctx, windowLength(req)); err != nil && ctx.Err() == nil {
					log.Printf("could not load the model ahead of the search: %v", err)
				}
			}()
		}
		// The transcription of an episode runs for its searches and for
		// nothing else, so it goes as far as this window and stops there,
		// exactly on its edge. A hold the workspace set already says so.
		if req.To > 0 && s.holdOf(p.Source) == 0 {
			_ = s.HoldTranscription(p.Source, req.To)
		}
		// The search has the machine to itself. Another episode's
		// transcription is paused and carries on after, because it runs for
		// a search of its own. This episode's own does not: it ran for this
		// search, and it has done what it was for.
		paused, err := s.waitForTranscript(ctx, p, req)
		defer func() { s.carryOn(paused) }()
		paused = without(paused, p.Source)
		if err != nil {
			if errors.Is(err, engine.ErrCancelled) {
				// Called off while it waited. The transcription it waited
				// for stops with it.
				s.stopTranscription(p.Source)
			}
			s.releaseHold(p.Source)
			return "", err
		}
		paused = without(append(paused, s.pauseTranscriptions()...), p.Source)
		s.releaseHold(p.Source)
		if len(paused) > 0 {
			p.Log().Info("other transcriptions wait while clips are found and carry on after")
		}
		return p.Plan(ctx, req)
	})
}

// without is paths less one of them.
func without(paths []string, path string) []string {
	out := paths[:0:0]
	for _, p := range paths {
		if p != path {
			out = append(out, p)
		}
	}
	return out
}

// stopTranscription stops an episode's transcription, if it runs or waits.
func (s *FrameFairy) stopTranscription(path string) {
	if j, ok := s.jobs.find(path, "transcribe"); ok && (j.State == JobRunning || j.State == JobQueued) {
		s.jobs.cancel(j.ID)
	}
}

// pauseTranscriptions stops every transcription that is running or
// waiting to run, for the length of a search, and gives the episodes it
// stopped. Each has saved what it heard and carries on from there. A
// transcription somebody paused by hand is not running, so it is not
// among them and stays paused.
func (s *FrameFairy) pauseTranscriptions() []string {
	var paused []string
	for _, j := range s.jobs.list() {
		if j.Kind == "transcribe" && (j.State == JobRunning || j.State == JobQueued) {
			s.jobs.cancel(j.ID)
			paused = append(paused, j.Episode)
		}
	}
	return paused
}

// carryOn starts again the transcriptions a search paused, for the
// episodes still in the library. A transcription told to stop takes a
// moment to save what it heard, and one asked for while the old one is
// still saving would be taken for it and never start, so each is given
// that moment first.
//
// The moment is a second here. One that takes longer is waited for in the
// background, for as long as it takes: after a fixed wait the old job was
// still running, the new ask was taken for it, and the transcription
// stayed paused for good, with the next search failing on a pause nobody
// made. Waiting here held the lane for up to fifteen seconds per episode,
// and every render queued behind the search waited with it.
func (s *FrameFairy) carryOn(episodes []string) {
	for _, path := range episodes {
		if s.stopped(path, time.Second) {
			if s.store.Known(path) {
				s.Transcribe(path)
			}
			continue
		}
		go func() {
			defer func() { _ = recover() }()
			for !s.stopped(path, time.Minute) {
				if !s.store.Known(path) {
					return
				}
			}
			if s.store.Known(path) {
				s.Transcribe(path)
			}
		}()
	}
}

// stopped waits up to wait for the episode's transcription to be no longer
// running, and says whether it is.
func (s *FrameFairy) stopped(path string, wait time.Duration) bool {
	deadline := time.Now().Add(wait)
	for {
		if j, ok := s.jobs.find(path, "transcribe"); !ok || j.State != JobRunning {
			return true
		}
		if !time.Now().Before(deadline) {
			return false
		}
		time.Sleep(50 * time.Millisecond)
	}
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

// WarmModel loads the local model for a search the app is about to start
// by itself, the first one of a new episode, while the transcript is still
// on its way to the end of the window. It is not a job: nothing waits for
// it, and the search it was for finds the model loaded or loads it itself.
func (s *FrameFairy) WarmModel(path string, from, to float64) {
	if !s.store.Known(path) {
		return
	}
	req := engine.PlanRequest{From: from, To: to}
	go func() {
		defer func() { _ = recover() }()
		e := engine.NewEngine(engine.NewLog(io.Discard, false, false))
		p := engine.NewProject(e, path, s.store.Settings().options())
		if err := p.WarmModel(context.Background(), windowLength(req)); err != nil {
			log.Printf("could not load the model ahead of the search: %v", err)
		}
	}()
}

// waitForTranscript blocks until the transcript covers the window, and
// starts the transcription if nothing is transcribing this episode.
//
// The transcript on disk is saved seconds apart, and the recogniser hears
// minutes of audio in those seconds, so a search that waited for the file
// alone started long after the window had been heard, while the range
// picker showed the transcription running on past the end of it. So the
// wait also watches how far the audio has been heard, and pauses the
// transcriptions the moment that passes the end of the window. A paused
// transcription writes down all it heard, the search starts on that, and
// the episodes it paused are handed back to be carried on after it.
func (s *FrameFairy) waitForTranscript(ctx context.Context, p *engine.Project,
	req engine.PlanRequest) ([]string, error) {
	asrDir := s.store.Settings().ASRModel
	end := req.To
	if end <= 0 {
		info, err := s.probe(ctx, p.Source)
		if err != nil {
			if ctx.Err() != nil {
				return nil, engine.ErrCancelled
			}
			return nil, err
		}
		end = info.Duration
	}
	// The words the row in the clip list shows, which adds the window
	// itself. The log says where the transcript has to get to.
	label := "Waiting for the transcript"
	said := false
	var started *Job
	// The episodes paused because this one had heard the window, and
	// whether that has been done. It is done once: a pause whose save did
	// not reach the window carries on and the wait goes back to the file.
	var paused []string
	pausedOnce := false
	firstAt, firstCovered := time.Time{}, -1.0
	for {
		covered, done := coverage(p.Source, asrDir)
		if done || covered >= end-0.05 {
			p.Log().ClearProgress()
			return paused, nil
		}
		if !said {
			said = true
			p.Log().Info("waiting for the transcript to reach %s", engine.HMS(end))
		}
		job, ok := s.jobs.find(p.Source, "transcribe")
		switch {
		case paused != nil && ok && job.State == JobRunning:
			// Paused here and still writing down what it heard.
		case paused != nil:
			// Stopped. The file was read before the job was, so it is read
			// again: the save may have landed between the two.
			if again, _ := coverage(p.Source, asrDir); again >= end-0.05 {
				continue
			}
			// What it wrote down falls short of the window. It carries
			// on, and the wait goes on by the file.
			s.carryOn(paused)
			paused, started = nil, nil
			continue
		case !pausedOnce && ok && job.State == JobRunning && job.Progress != nil &&
			job.Progress.Covered >= end-0.05:
			pausedOnce = true
			paused = s.pauseTranscriptions()
			if len(paused) == 0 {
				paused = nil
			}
		case ok && (job.State == JobQueued || job.State == JobRunning):
		case started != nil && ok && job.ID == started.ID:
			p.Log().ClearProgress()
			reason := job.Error
			if reason == "" {
				reason = "the transcription stopped at " + engine.HMS(covered)
			}
			p.Log().Error("%s", reason)
			return nil, engine.ErrStepFailed
		default:
			queued := s.Transcribe(p.Source)
			started = &queued
		}

		// Time left, from how fast the transcript has grown while waiting.
		remaining := engine.Unknown
		now := time.Now()
		if firstCovered < 0 {
			firstAt, firstCovered = now, covered
		} else if grown := covered - firstCovered; grown > 0 {
			rate := grown / now.Sub(firstAt).Seconds()
			remaining = (end - covered) / rate
		}
		// An episode whose length could not be measured would make this
		// 0 divided by 0, which is not a share of anything.
		share := engine.Unknown
		if end > 0 {
			share = covered / end
		}
		p.Log().ProgressOf(label, share, remaining)
		// The file is read once a second, because it is the whole
		// transcript. What the queue holds, how far the audio has been
		// heard and whether the transcription still runs, is looked at
		// every 20 ms, so the pause comes the moment the window has been
		// heard and the search starts the moment the pause has written
		// down what it heard.
		if err := s.untilNews(ctx, p.Source, end, !pausedOnce, paused != nil); err != nil {
			p.Log().ClearProgress()
			return paused, engine.ErrCancelled
		}
	}
}

// coverage is how far the saved transcript reaches. Tests put a stand-in
// here, because a real transcript needs a real recogniser.
var coverage = engine.Coverage

// untilNews waits a second, or less if something has happened that the
// wait for the transcript has to act on at once: before the pause, the
// audio heard to the end of the window, and after it, the transcription
// having stopped, which is when what it heard is on disk. After the pause
// it used to sleep the whole second regardless, and the search started up
// to a second after it could have.
func (s *FrameFairy) untilNews(ctx context.Context, path string, end float64,
	toPause, pausedHere bool) error {
	deadline := time.Now().Add(time.Second)
	for time.Now().Before(deadline) {
		select {
		case <-ctx.Done():
			return ctx.Err()
		case <-time.After(20 * time.Millisecond):
		}
		job, ok := s.jobs.find(path, "transcribe")
		running := ok && job.State == JobRunning
		if pausedHere && !running {
			return nil
		}
		if toPause && running && job.Progress != nil && job.Progress.Covered >= end-0.05 {
			return nil
		}
	}
	return nil
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

// Render queues rendering clips of a plan.
func (s *FrameFairy) Render(path string, req engine.RenderRequest) Job {
	label := "Render"
	if req.Preview {
		label = "Preview"
	}
	// The plan is a path of its own, read and written to, so it is checked
	// the same way the episode is.
	if !s.store.Known(path) || (req.Plan != "" && !s.store.Known(req.Plan)) {
		return s.jobs.refuse(path, "render", label, notInLibrary)
	}
	return s.jobs.add(path, "render", label, func(ctx context.Context, p *engine.Project) (string, error) {
		if err := p.Render(ctx, req); err != nil {
			return req.Plan, err
		}
		if !req.Preview {
			// A finished render is the strongest sign a clip was right, and
			// it is recorded with the clip as it was rendered.
			ids := req.Clips
			if len(ids) == 0 {
				if view, err := engine.ReadPlan(req.Plan); err == nil {
					for _, c := range view.Clips {
						if !c.Rejected {
							ids = append(ids, c.ID)
						}
					}
				}
			}
			for _, id := range ids {
				_ = engine.RecordDecision(req.Plan, id, engine.DecisionRendered, nil)
			}
		}
		return req.Plan, nil
	})
}

// WordsView is the words of a part, with the lead-in and lead-out the
// renderer leaves around a cut, so the interface can snap edges the same
// way.
type WordsView struct {
	Words     []engine.WordView `json:"words"`
	KeepPause float64           `json:"keepPause"`
}

// Words returns the words spoken in a part. Before the first
// transcription there are none, which is an empty answer and not a failure.
func (s *FrameFairy) Words(path string, from, to float64) (WordsView, error) {
	if !s.store.Known(path) {
		return WordsView{}, os.ErrNotExist
	}
	opts := s.store.Settings().options()
	out := WordsView{Words: []engine.WordView{}, KeepPause: opts.KeepPause}
	t, err := s.transcript(engine.NewProject(nil, path, opts))
	if errors.Is(err, engine.ErrNoTranscript) {
		return out, nil
	}
	if err != nil {
		return WordsView{}, err
	}
	// One word either side, so an edge can snap past the part.
	words := t.WordsBetween(from-5, to+5)
	for _, w := range words {
		out.Words = append(out.Words, engine.WordView{Start: w.Start, End: w.End, Text: w.Text})
	}
	return out, nil
}

func (s *FrameFairy) SetWord(ctx context.Context, path, plan, clipID string, start float64, text string) (ClipEntry, error) {
	if !s.store.Known(path) || !s.store.Known(plan) {
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
		_, err := engine.SetWordText(p.LogsDir(), start, text, t)
		return err
	}); err != nil {
		return ClipEntry{}, err
	}
	return s.clipEntry(ctx, path, plan, clipID)
}

// SetCrop places the crop of the shot at a moment of a clip by hand, as the
// left edge in source pixels, and returns the clip as it is now.
func (s *FrameFairy) SetCrop(ctx context.Context, path, plan, clipID string, at float64, left int) (ClipEntry, error) {
	if !s.store.Known(path) || !s.store.Known(plan) {
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

// ResetCrop brings back the automatic crop for the shot at a moment of a
// clip, and returns the clip as it is now.
// SetCaptionStyle changes the face and the size of the captions of a whole
// clip set, which is what the workspace offers next to the clip.
func (s *FrameFairy) SetCaptionStyle(ctx context.Context, path, plan, font string, size float64) error {
	if !s.store.Known(path) || !s.store.Known(plan) {
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
	if !s.store.Known(path) || !s.store.Known(plan) {
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
// clips wants them everywhere. The numbers are held to the same range the
// controls offer, because what arrives here is not to be trusted.
func (s *FrameFairy) SetSearch(count int, min, max float64) error {
	return s.store.UpdateSettings(func(set *Settings) {
		set.Count = int(hold(float64(count), 1, 30))
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
	p := engine.NewProject(nil, path, s.store.Settings().options())
	logs := engine.ResolvePath(p.LogsDir())
	for _, plan := range engine.Status(path, s.store.Settings().ASRModel).Plans {
		if !s.store.Known(plan.Path) || filepath.Dir(engine.ResolvePath(plan.Path)) != logs {
			continue
		}
		if _, err := engine.ClearCaptionY(plan.Path); err != nil {
			return err
		}
	}
	return nil
}

// SetCaptionTime moves the caption of a clip that begins or ends on a word,
// to appear or go at a moment of the episode. A moment below nought puts it
// back where its words put it, because JSON has no way to say not a number.
func (s *FrameFairy) SetCaptionTime(ctx context.Context, path, plan, clipID string, word float64, edge string, at float64) (ClipEntry, error) {
	if !s.store.Known(path) || !s.store.Known(plan) {
		return ClipEntry{}, os.ErrNotExist
	}
	if at < 0 {
		at = math.NaN()
	}
	if err := s.edit(path, func() error {
		return engine.SetCaptionTime(plan, clipID, word, edge, at)
	}); err != nil {
		return ClipEntry{}, err
	}
	return s.clipEntry(ctx, path, plan, clipID)
}

func (s *FrameFairy) ResetCrop(ctx context.Context, path, plan, clipID string, at float64) (ClipEntry, error) {
	if !s.store.Known(path) || !s.store.Known(plan) {
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

// chosenFile is where an episode keeps the clip that was last worked on.
// It is about that episode and nothing else, so it sits in the episode's
// own folder and goes when the folder goes.
const chosenFile = "chosen.json"

// chosenClip is the whole of that file. A struct rather than a bare string
// so a later version can keep more without the older one choking on it.
type chosenClip struct {
	Clip string `json:"clip"`
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
	dir := engine.WorkDir(path)
	if err := os.MkdirAll(dir, 0o755); err != nil {
		return err
	}
	file, err := engine.SafeChild(dir, chosenFile)
	if err != nil {
		return err
	}
	body, err := json.Marshal(chosenClip{Clip: key})
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

// ChosenClip gives back the clip an episode was last worked on, or an empty
// string where there is none or where what is written down is not a key.
// The interface checks it against the clips it has either way: a clip set
// that has been searched again no longer holds it.
func (s *FrameFairy) ChosenClip(path string) string {
	if !s.store.Known(path) {
		return ""
	}
	file, err := engine.SafeChild(engine.WorkDir(path), chosenFile)
	if err != nil {
		return ""
	}
	data, err := os.ReadFile(file)
	if err != nil {
		return ""
	}
	var held chosenClip
	if json.Unmarshal(data, &held) != nil {
		return ""
	}
	if !looksLikeClipKey(held.Clip) || held.Clip == "" {
		return ""
	}
	return held.Clip
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
	if !s.store.Known(path) || !s.store.Known(plan) {
		return ClipEntry{}, os.ErrNotExist
	}
	if err := s.edit(path, func() error { return engine.SetRejected(plan, clipID, removed) }); err != nil {
		return ClipEntry{}, err
	}
	return s.clipEntry(ctx, path, plan, clipID)
}

// TrimClip moves the first and last edge of a clip and returns it as it is
// now.
func (s *FrameFairy) TrimClip(ctx context.Context, path, plan, clipID string, start, end float64) (ClipEntry, error) {
	if !s.store.Known(path) || !s.store.Known(plan) {
		return ClipEntry{}, os.ErrNotExist
	}
	opts := s.store.Settings().options()
	t, err := engine.NewProject(nil, path, opts).Transcript()
	if err != nil {
		return ClipEntry{}, err
	}
	if err := s.edit(path, func() error {
		return engine.TrimClip(plan, clipID, start, end, t, opts.KeepPause)
	}); err != nil {
		return ClipEntry{}, err
	}
	return s.clipEntry(ctx, path, plan, clipID)
}

// cutting is what every change to a clip's cuts needs: the episode and the
// plan have to belong to the library, and the transcript is what the edges
// snap to.
func (s *FrameFairy) cutting(path, plan string) (*engine.Transcript, engine.Options, error) {
	if !s.store.Known(path) || !s.store.Known(plan) {
		return nil, engine.Options{}, os.ErrNotExist
	}
	opts := s.store.Settings().options()
	t, err := engine.NewProject(nil, path, opts).Transcript()
	if err != nil {
		return nil, engine.Options{}, err
	}
	return t, opts, nil
}

// CutClip takes a part out of the middle of a clip and returns it as it
// is now. With toWords the edges land on the words around them, without it
// they stay exactly where the hand put them.
func (s *FrameFairy) CutClip(ctx context.Context, path, plan, clipID string, from, to float64, toWords bool) (ClipEntry, error) {
	t, opts, err := s.cutting(path, plan)
	if err != nil {
		return ClipEntry{}, err
	}
	if err := s.edit(path, func() error {
		return engine.CutClip(plan, clipID, from, to, t, opts.KeepPause, engine.Snap(toWords))
	}); err != nil {
		return ClipEntry{}, err
	}
	return s.clipEntry(ctx, path, plan, clipID)
}

// JoinCut puts back the part a clip leaves out at a moment and returns
// the clip as it is now.
func (s *FrameFairy) JoinCut(ctx context.Context, path, plan, clipID string, at float64) (ClipEntry, error) {
	t, _, err := s.cutting(path, plan)
	if err != nil {
		return ClipEntry{}, err
	}
	if err := s.edit(path, func() error { return engine.JoinCut(plan, clipID, at, t) }); err != nil {
		return ClipEntry{}, err
	}
	return s.clipEntry(ctx, path, plan, clipID)
}

// MoveCut moves both edges of one of a clip's cuts and returns the clip as
// it is now. With toWords the edges land on the words around them, without
// it they stay exactly where they were put, a frame at a time.
func (s *FrameFairy) MoveCut(ctx context.Context, path, plan, clipID string, index int, from, to float64, toWords bool) (ClipEntry, error) {
	t, opts, err := s.cutting(path, plan)
	if err != nil {
		return ClipEntry{}, err
	}
	if err := s.edit(path, func() error {
		return engine.MoveCut(plan, clipID, index, from, to, t, opts.KeepPause, engine.Snap(toWords))
	}); err != nil {
		return ClipEntry{}, err
	}
	return s.clipEntry(ctx, path, plan, clipID)
}

// Jobs lists queued, running and finished jobs.
func (s *FrameFairy) Jobs() []Job { return s.jobs.list() }

// CancelJob stops a job, or takes it out of the queue.
func (s *FrameFairy) CancelJob(id string) { s.jobs.cancel(id) }

// ClearJobs forgets finished jobs.
func (s *FrameFairy) ClearJobs() { s.jobs.clear() }

// ReadPlan loads a plan for the candidates screen.
func (s *FrameFairy) ReadPlan(planPath string) (*engine.PlanView, error) {
	if !s.store.Known(planPath) {
		return nil, os.ErrNotExist
	}
	return engine.ReadPlan(planPath)
}

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
