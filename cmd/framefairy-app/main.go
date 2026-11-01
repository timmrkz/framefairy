// Command app is the desktop app for framefairy. It drives the same engine as the
// command line.
package main

import (
	"bytes"
	"embed"
	"errors"
	"fmt"
	"image"
	"io"
	"io/fs"
	"net/http"
	"os"
	"path/filepath"
	"runtime"
	"runtime/debug"
	"strings"
	"sync"
	"sync/atomic"
	"time"

	"github.com/rs/zerolog"
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
	// The fonts installed on the Mac are offered for captions beside ours.
	// Their folders are read once now, so the Font list opens at once.
	engine.UseMachineFonts()
	go engine.RefreshFonts()
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
	theLog.started()
	svc := &FrameFairy{store: st, keeping: newKeeping()}
	// The model is kept loaded while the app is open, and kept again
	// when the settings change it or a search ends, see keep.go. It is
	// first loaded once a server the last run left behind is stopped,
	// below.
	st.changed = svc.keeping.ask
	svc.jobs = newQueue(st, emit, notify)
	svc.jobs.searched = svc.keeping.ask
	svc.listening.say = quietly(func(d Dictation) {
		if app != nil {
			app.Event.Emit("dictation", d)
		}
	})
	svc.listening.loud = quietly(func(n Loudness) {
		if app != nil {
			app.Event.Emit("loudness", n)
		}
	})
	// The report window and its draft, see bugs.go. The draft is kept
	// beside the settings.
	svc.report = newReporting(st.files, filepath.Join(st.dir, "report"))
	svc.report.stop = svc.CancelListening
	svc.report.told = func(open bool) {
		if app != nil {
			app.Event.Emit("report-window", open)
		}
	}
	svc.report.said = func(d ReportDraft) {
		if app != nil {
			app.Event.Emit("report-draft", d)
		}
	}
	svc.report.about = func(text string) {
		if app != nil {
			app.Event.Emit("report-about", text)
		}
	}
	// The searches and renders that were cut off or failed the last time
	// say so where their work was.
	svc.restoreJobs(st.Episodes())
	// Nothing the app started outlives it: a model loaded or still loading
	// is stopped and no other is loaded, and the jobs are stopped, and with
	// them any ffmpeg they run. On macOS app.Run never returns: Cmd+Q ends
	// the process from inside Cocoa once the shutdown hooks have run, so
	// this has to be one of them. Code after app.Run only runs on the other
	// systems, and left a llama-server behind on every Mac that quit during
	// a search.
	svc.levels = newMeasuring(svc.store.files, func(episode string) {
		if app != nil {
			app.Event.Emit("levels", episode)
		}
	})
	quit := sync.OnceFunc(func() {
		engine.CloseModels()
		svc.stopAll()
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
		Description: "Turns a long video into vertical shorts",
		Services:    []application.Service{application.NewService(svc)},
		Assets: application.AssetOptions{
			Handler: interfaceHandler(),
			Middleware: func(next http.Handler) http.Handler {
				return probeMiddleware(mediaMiddleware(st))(reportMiddleware(svc.report)(next))
			},
		},
		Mac: application.MacOptions{
			ApplicationShouldTerminateAfterLastWindowClosed: true,
		},
		OnShutdown: quit,
		ShouldQuit: leave.shouldQuit,
		// What Wails says goes into the app's log. Its debug lines are one
		// for every request the window makes, which the log has its own
		// lines for, see logged.
		Logger: theLog.slog("wails", zerolog.InfoLevel),
		// A panic Wails catches, in a window's event, an event of the app
		// or a call from the window, goes into the app's log and the app
		// goes on. Without a handler of ours Wails ends the app, issue
		// #604. The window's requests for frames, sound and fonts Wails
		// does not recover, see mediaMiddleware.
		PanicHandler: wailsPanicked,
	})
	svc.app = app
	svc.leave = leave
	// A framefairy:// link, from a browser, a mail or any other app, when
	// the app is started by it and when it is already running.
	app.Event.OnApplicationEvent(events.Common.ApplicationLaunchedWithUrl, func(e *application.ApplicationEvent) {
		probeLink()
		svc.openedWith(e.Context().URL())
	})
	svc.updates = newUpdating(app.Updater, app.Quit, st, svc.jobs.closeIfIdle, func(u UpdateState) {
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
		// A video dropped anywhere on the app is added, the way Add adds
		// one. The window marks where a drop lands, the whole app once
		// the setup is done, see App.svelte, and says what came of it.
		EnableFileDrop: true,
	})
	svc.window.OnWindowEvent(events.Common.WindowFilesDropped, func(e *application.WindowEvent) {
		files := e.Context().DroppedFiles()
		// Deciding whether a picture can be decoded takes a moment for
		// each video, which is not the window's to wait on.
		go func() { app.Event.Emit("dropped", svc.dropFiles(files)) }()
	})
	svc.chrome = watchChrome(app, svc.window, "app")
	freeRefresh(svc.window)
	keepDrawing(svc.window)
	watchSight(svc.window)
	// Only in the build the checks from outside the app use, see
	// probe_outside.go.
	startProbe(svc)
	svc.window.OnWindowEvent(events.Common.WindowClosing, func(*application.WindowEvent) {
		leave.closing()
		svc.report.closeWindow("app")
	})
	svc.report.make = func(closed func()) (front, shut func()) {
		return openReportWindow(app, svc.window, svc.report, closed, func(w *application.WebviewWindow) {
			svc.reportChrome.Store(watchChrome(app, w, "report"))
		})
	}

	// A llama-server the last run left behind, because it crashed or was
	// killed, is stopped before this run loads a model of its own.
	if engine.StopLeftoverServer() {
		theLog.line(zerolog.InfoLevel, "model").Msg("stopped a llama-server the last run of the app left behind")
	}
	go svc.keeping.run(st)
	svc.keeping.ask()
	svc.updates.start()
	err := app.Run()
	quit()
	if err != nil {
		theLog.line(zerolog.ErrorLevel, "app").Err(err).Msg("the app stopped")
		fmt.Fprintln(os.Stderr, err)
		os.Exit(1)
	}
}

// mediaMiddleware serves the captions' faces and the frames and sound of
// an episode, see previews. Nothing serves the bytes of a file to the
// page: the video preview reads nothing of the file itself, see
// docs/VIDEO-PREVIEW.md, step 5. Only files that belong to an episode in
// the library are decoded.
func mediaMiddleware(st *store) application.Middleware {
	return func(next http.Handler) http.Handler {
		return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			// Wails serves each request on a goroutine of its own with no
			// recover, and a frame, a sound, a caption or a font is our
			// code over a video or a word nobody checked. A panic in one
			// ended the whole app. Now it fails that request, issue #604.
			defer func() {
				if caught := recover(); caught != nil {
					theLog.line(zerolog.ErrorLevel, "files").Str("path", r.URL.Path).Str("stack", string(debug.Stack())).
						Msgf("the window's request stopped unexpectedly: %v", caught)
					http.Error(w, fmt.Sprintf("the request stopped unexpectedly: %v", caught), http.StatusInternalServerError)
				}
			}()
			if r.URL.Path == "/font" {
				// The interface writes the word being typed in the same face
				// as the render: one built into the program, or the copy of
				// one installed on the machine. Only a face of that list is
				// served, found by its name, never by a path.
				data, ok := engine.FontBytes(r.URL.Query().Get("name"))
				if !ok {
					http.NotFound(w, r)
					return
				}
				kind := "font/ttf"
				if bytes.HasPrefix(data, []byte("OTTO")) {
					kind = "font/otf"
				}
				w.Header().Set("Content-Type", kind)
				w.Header().Set("Cache-Control", "max-age=86400")
				_, _ = w.Write(data)
				return
			}
			if strings.HasPrefix(r.URL.Path, "/frames/") {
				logged(w, r, func(w http.ResponseWriter, r *http.Request) { openPreviews.serve(st, w, r) })
				return
			}
			next.ServeHTTP(w, r)
		})
	}
}

// notInLibrary is what a call is told when it names a file that does not
// belong to an episode in the library. The interface only ever names files
// it was given, so this is the last line rather than the first.
const notInLibrary = "this file does not belong to a video in the library"

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
	// The same for the report window, the one made last, see bugs.go.
	reportChrome atomic.Pointer[chromeWatch]
	store        *store
	jobs         *queue
	// The loudness of each episode, measured from the moment it is
	// added, which is the waveform, see levels.go.
	levels *measuring
	// Finding and installing a newer build of the app, see updates.go.
	updates *updating
	// What Cmd+Q does, see quit.go.
	leave *leaving
	// The recording for a report, see listen.go.
	listening listening
	// The report window and the draft of a report, see bugs.go.
	report *reporting
	// The local model kept loaded while the app is open, see keep.go.
	keeping *keeping
	mu      sync.Mutex
	// The transcripts of the episodes worked on last, each kept while the
	// files it was read from stay as they were, see transcript.
	said keptReads[*engine.Transcript]
	// toldOld is the videos whose old corrections.json was said to be
	// left unread, so it is said once, see oldCorrections.
	toldOld sync.Map
	// histories are the undo and redo of each episode, see history.go.
	histories map[string]*history
	// clearing counts the clearLetGo calls under way. Wait returns once
	// every one started has returned, whether it deleted what was written
	// back or gave up, see clearLetGo.
	clearing sync.WaitGroup
	// quitting is set under clearMu once the app is on its way out, see
	// stopAll. From then on a video let go is cleared where it is let go,
	// since no clearLetGo may start on clearing while it is waited for.
	clearMu  sync.Mutex
	quitting bool
	// link is what became of the last framefairy:// link, waiting for
	// the settings, see licence.go.
	link *LicenceLink
	// clipboard stands in for the system's in tests, see copyText.
	clipboard func(string) bool
	// saveBox stands in for the system's save box in tests, see saveAs.
	saveBox func(name string) (string, error)
	// frameLate, when set, says a video's first frame is late, and Add
	// takes the video without it then as well as after firstFrameWait of
	// the clock, see undecodable. A test sets it, so it waits for no
	// limit of the clock.
	frameLate <-chan struct{}
	// now is the time of day an edit is timed by, time.Now unless a test
	// hands in a clock it moves, see clock.
	now func() time.Time
}

// clock is the time of day, from now when a test handed one in.
func (s *FrameFairy) clock() time.Time {
	if s.now != nil {
		return s.now()
	}
	return time.Now()
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
// and looks at once.
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
func (s *FrameFairy) Licences() ([]notices.Notice, error) {
	built, err := builtNotices()
	if err != nil {
		return nil, err
	}
	return notices.All(built)
}

// LicenceText is one of the texts a notice names. Only the texts built
// into the app can be read this way, whatever name is asked for.
func (s *FrameFairy) LicenceText(name string) (string, error) {
	built, err := builtNotices()
	if err != nil {
		return "", err
	}
	return notices.Text(built, name)
}

// builtNotices are the notices make wrote of the Go modules and the
// interface's packages this build has, see notices/gen. A program built
// without make has none, and says so in the log.
func builtNotices() (notices.Built, error) {
	raw, err := dist.ReadFile("dist/licences.json")
	if err != nil {
		theLog.line(zerolog.WarnLevel, "app").Err(err).Msg("built without the licences of its modules and packages, run make")
		return notices.Built{}, nil
	}
	return notices.ReadBuilt(raw)
}

// Chrome says where macOS put its own furniture in a window, "app" or
// "report", or all zeros where the system draws its own title bar. The
// interface asks once and is told again on the window's event, see
// chromeEvent, whenever the answer changes.
func (s *FrameFairy) Chrome(window string) Chrome {
	c := s.chrome
	if window == "report" {
		c = s.reportChrome.Load()
	}
	if c == nil {
		return Chrome{}
	}
	return c.measure()
}

// openReportWindow makes the report window, beside the app's window, with
// the app's own page showing Report a Problem alone, see main.ts. It gives
// what brings it forward and what closes it, and calls closed once it is
// gone. Files dropped on it are attached to the report, see bugs.go.
func openReportWindow(app *application.App, main *application.WebviewWindow, report *reporting, closed func(), watch func(*application.WebviewWindow)) (front, shut func()) {
	x, y := 0, 0
	start := application.WindowCentered
	if main != nil {
		b := main.Bounds()
		at := image.Rect(b.X, b.Y, b.X+b.Width, b.Y+b.Height)
		work := at
		if screen, err := main.GetScreen(); err == nil && screen != nil {
			wa := screen.WorkArea
			work = image.Rect(wa.X, wa.Y, wa.X+wa.Width, wa.Y+wa.Height)
		}
		p := beside(at, work, reportWidth, reportHeight)
		x, y, start = p.X, p.Y, application.WindowXY
	}
	w := app.Window.NewWithOptions(application.WebviewWindowOptions{
		Name:             "report",
		Title:            "Report a Problem",
		Width:            reportWidth,
		Height:           reportHeight,
		MinWidth:         reportMinWidth,
		MinHeight:        reportMinHeight,
		InitialPosition:  start,
		X:                x,
		Y:                y,
		BackgroundColour: application.NewRGB(29, 31, 35),
		URL:              "/?window=report",
		Mac: application.MacWindow{
			InvisibleTitleBarHeight: 28,
			TitleBar:                application.MacTitleBarHidden,
		},
		// Pictures dropped anywhere on it are attached, see bugs.go.
		EnableFileDrop: true,
	})
	w.OnWindowEvent(events.Common.WindowFilesDropped, func(e *application.WindowEvent) {
		files := e.Context().DroppedFiles()
		go report.dropped(files)
	})
	w.OnWindowEvent(events.Common.WindowClosing, func(*application.WindowEvent) { closed() })
	// Its own title bar, measured on its own, before the page can ask.
	watch(w)
	w.Show()
	w.Focus()
	return func() { w.Show(); w.Focus() }, func() { w.Close() }
}
