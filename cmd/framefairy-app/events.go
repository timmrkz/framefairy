package main

import (
	"context"
	"path/filepath"
)

// What the Go side tells the window by itself, without being asked. Each
// event is heard by its name in lib/api.ts, with the data it carries there,
// and the names are written here and nowhere else on the Go side. The walks'
// bridge tells the window through the same events and the same wiring,
// tellWindow, so a name or a part left unwired here is one the walks see.
// TestEveryEventIsHeardAndEveryHeardEventIsSent holds the names to
// lib/api.ts both ways, and TestTheInterfaceReadsTheFieldsGoSends what
// each carries.

// emitter is what tells the window: Wails' app.Event in the app, the
// bridge's bus in the walks.
type emitter interface {
	Emit(name string, data ...any) bool
}

// emitterFunc is a function that tells the window.
type emitterFunc func(name string, data ...any) bool

func (f emitterFunc) Emit(name string, data ...any) bool { return f(name, data...) }

// event is something the window is told, by the name it hears it on, with
// the data it carries.
type event[T any] string

// to tells w, when there is one.
func (e event[T]) to(w emitter, data T) {
	if w != nil {
		w.Emit(string(e), data)
	}
}

// cue is something the window is told that carries nothing.
type cue string

// to tells w, when there is one.
func (s cue) to(w emitter) {
	if w != nil {
		w.Emit(string(s))
	}
}

// Every event, in the order lib/api.ts hears them.
const (
	jobEvent              event[JobUpdate]   = "job"
	noticeEvent           event[Notice]      = "notice"
	chromeAppEvent        event[Chrome]      = "chrome"
	chromeReportEvent     event[Chrome]      = "chrome-report"
	undoEvent             event[string]      = "undo"
	quitEvent             event[string]      = "quit"
	acknowledgementsEvent cue                = "acknowledgements"
	loudnessEvent         event[Loudness]    = "loudness"
	dictationEvent        event[Dictation]   = "dictation"
	reportWindowEvent     event[bool]        = "report-window"
	reportDraftEvent      event[ReportDraft] = "report-draft"
	reportAboutEvent      event[string]      = "report-about"
	episodeEvent          event[string]      = "episode"
	droppedEvent          event[Added]       = "dropped"
	videoEvent            event[VideoState]  = "video"
	setupEvent            event[SetupState]  = "setup"
	updatesEvent          event[UpdateState] = "updates"
	licenceLinkEvent      cue                = "licence-link"
	showUpdatesEvent      cue                = "show-updates"
)

// tellWindow hands every part of the Go side that tells the window by
// itself what it tells it through, w, and keeps w for the rest. The jobs,
// a video's state, a failure, a recording, the report window and the
// models all tell it from here. It makes the job queue and the video's
// news, so it comes before anything uses them, and the report before it.
func (s *FrameFairy) tellWindow(w emitter) {
	s.tell = w
	s.jobs = newQueue(s.store, func(u JobUpdate) {
		if toWindow(u) {
			jobEvent.to(w, u)
		}
	}, func(episode string) { episodeEvent.to(w, episode) })
	// A video's state is sent whole as its work goes on, see videostate.go.
	s.news = newVideoNews(func(path string) (VideoState, error) {
		return s.State(context.Background(), path)
	}, func(v VideoState) { videoEvent.to(w, v) })
	s.jobs.news = s.told
	// A failure is said once, as it happens, see notice.go.
	s.jobs.notice = quietly(func(n Notice) { noticeEvent.to(w, n) })
	s.listening.say = quietly(func(d Dictation) { dictationEvent.to(w, d) })
	s.listening.loud = quietly(func(n Loudness) { loudnessEvent.to(w, n) })
	// The report window and its draft, see bugs.go.
	s.report.told = func(open bool) { reportWindowEvent.to(w, open) }
	s.report.said = func(d ReportDraft) { reportDraftEvent.to(w, d) }
	s.report.about = func(text string) { reportAboutEvent.to(w, text) }
	// How far the loudness is measured is in the video's state.
	s.levels = newMeasuring(s.store.machine, s.told)
	// The models changed outside the app, see tellSetup.
	s.setupNews = quietly(func(state SetupState) { setupEvent.to(w, state) })
}

// newReport makes the report window's part of the app, with its draft
// beside the settings, see bugs.go.
func (s *FrameFairy) newReport() {
	s.report = newReporting(s.store.machine, filepath.Join(s.store.dir, "report"))
	s.report.stop = s.CancelListening
}
