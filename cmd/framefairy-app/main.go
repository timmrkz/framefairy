// Command app is the desktop app for framefairy. It drives the same engine as the
// command line.
package main

import (
	"embed"
	"errors"
	"io"
	"io/fs"
	"log"
	"net/http"
	"os"
	"path/filepath"
	"runtime"
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
	svc.levels = newMeasuring(func(episode string) {
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
		asked: func() bool { return svc.updates.relaunchingNow() },
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
			Middleware: probeMiddleware(mediaMiddleware(st)),
		},
		Mac: application.MacOptions{
			ApplicationShouldTerminateAfterLastWindowClosed: true,
		},
		OnShutdown: quit,
		ShouldQuit: leave.shouldQuit,
	})
	svc.app = app
	svc.leave = leave
	// A framefairy:// link, from a browser, a mail or any other app, when
	// the app is started by it and when it is already running.
	app.Event.OnApplicationEvent(events.Common.ApplicationLaunchedWithUrl, func(e *application.ApplicationEvent) {
		probeLink()
		svc.openedWith(e.Context().URL())
	})
	svc.updates = newUpdating(app.Updater, app.Quit, st, svc.jobs.busy, func(u UpdateState) {
		app.Event.Emit("updates", u)
	})
	app.Menu.Set(appMenu(app, func() {
		go svc.updates.checkNow()
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
	// Only in the build the checks from outside the app use, see
	// probe_outside.go.
	startProbe(svc)
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
			if strings.HasPrefix(r.URL.Path, "/frames/") {
				openPreviews.serve(st, w, r)
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

// errNotInLibrary is notInLibrary as the error a call returns. It says
// what happened, where os.ErrNotExist said "file does not exist" about a
// file that may well exist, and it is still a file that does not exist to
// errors.Is, so nothing that asks that way changes.
var errNotInLibrary error = libraryError{}

type libraryError struct{}

func (libraryError) Error() string { return notInLibrary }

func (libraryError) Is(target error) bool { return target == fs.ErrNotExist }

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
	// What Cmd+Q does, see quit.go.
	leave  *leaving
	mu     sync.Mutex
	probed map[string]engine.SourceInfo
	// The transcript of the episode being worked on, kept while the files
	// it was read from stay as they were.
	said   *engine.Transcript
	saidBy string
	// histories are the undo and redo of each episode, see history.go.
	histories map[string]*history
	// link is what became of the last framefairy:// link, waiting for
	// the settings, see licence.go.
	link *LicenceLink
	// clipboard stands in for the system's in tests, see copyText.
	clipboard func(string) bool
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

// CheckForUpdates reads the channel list again and looks for a newer build
// now, and downloads it.
func (s *FrameFairy) CheckForUpdates() { go s.updates.checkNow() }

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

// StayOpen is the question Cmd+Q asked taken away, with Escape or a click,
// so the next Cmd+Q asks again rather than quitting.
func (s *FrameFairy) StayOpen() {
	if s.leave != nil {
		s.leave.stay()
	}
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
