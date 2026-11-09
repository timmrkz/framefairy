package engine

import (
	"errors"
	"maps"
	"net/http"
	"runtime"
	"sync"
	"sync/atomic"

	"github.com/spf13/afero"

	"framefairy/engine/tool"
)

// Machine is what a program keeps for its whole life and shares with every
// job it runs: where files are read and written, the records, how the
// programs it calls are found and run, and what was learned about this
// machine on the way, which encoders and subtitle filter its ffmpeg has, and the face detector. Each program
// makes one in main and hands it to every Engine it makes, so what one job
// learned the next one knows. An Engine keeps only what belongs to its one
// job. See docs/decisions/2026-10-31-running-programs-and-what-outlives-a-job.md.
type Machine struct {
	// Files is where the files of episodes are read and written, the real
	// disk in the programs, see files.go.
	Files afero.Fs
	// Records is where the training records and the timings of past
	// searches are kept, on Files in the default places unless the
	// program hands it its own, see records.go.
	Records *Records
	// Runner runs ffmpeg, ffprobe and the other programs, and says how a
	// program whose work is called off is stopped. A test sets its own
	// stop on its own machine.
	Runner tool.Runner
	// Tools finds the programs the engine runs and checks each is the one
	// the program was built with, once while the file stays as it is.
	Tools *tool.Finder
	// Said hears what the programs the engine starts write for the log,
	// see ProgramSaid. It is set once, before any program starts, and
	// nil hears nothing.
	Said ProgramSaid
	// Fonts are the faces captions can be written in, the bundled ones
	// and, once the program calls UseMachine, the machine's own, read
	// from its font folders and kept with what was measured of each.
	Fonts Fonts

	// learned is what each ffmpeg was found to offer, by what was asked,
	// see learnedFor.
	learned sync.Map

	// held guards episodes, one for each video asked about, and what each
	// holds, uses counts the asks, see Episode.
	held     sync.Mutex
	episodes map[string]*Episode
	uses     uint64

	facesOnce sync.Once
	faces     *faceDetector

	// probes is what ffmpeg and ffprobe said, kept while they and the
	// file asked about stay as they are, see probed.go.
	probes probeCache

	// measured is the ink of the caption blocks libass measured, see
	// measureMany, and previewTracks the caption files of the video
	// preview, see previewTrack, each made the first time it is asked for.
	captionsOnce  sync.Once
	measured      *inkCache
	previewTracks *trackCache
	// grows wakes whoever waits for a transcript to grow, see
	// transcriptGrows. The job that hears and the search that reads ahead
	// share the machine, so the news goes from one to the other.
	grows transcriptNews
	// framing turns an entry of a model's answer into a clip, see
	// planBuilder.work. Empty, it is planBuilder.frame. A test puts in one
	// that panics.
	framing func(*planBuilder, planJob) (PlanClip, bool, error)

	// host is the one local model this program keeps loaded, see
	// modelhost.go.
	host modelHost
	// speechHost is the one speech model it keeps loaded, see
	// speechhost.go.
	speechHost speechHost
	// llama is how the program reaches the llama-servers it starts, and
	// notes are what it writes down of each, see leftover.go.
	llama llamaLink
	notes serverNotes
	// modelChecks lets one check of a model's file at a time read and
	// write what the checks found, see checkModel.
	modelChecks sync.Mutex
	// memory, free and servers are what is known of this machine's
	// memory: how much it has, how much it can give a program now, and
	// the llama-servers it runs, each the system's when it is empty, see
	// room.go and freemem.go. A test makes its machine any machine in any
	// state.
	memory  func() int64
	free    func() int64
	servers func() []string

	// keys is where the keys are kept, and oldKeys where the first ones
	// were, see keys.go, and savedFirst reads a saved key before one in
	// the environment. network is how a request leaves the program, to a
	// company or for a model's download. These two reach out of the
	// program, so a machine NewMachine makes has neither: it keeps no key
	// and reaches nothing until UseSystem hands it the system's, and a
	// machine a test makes can never write into somebody's keychain or
	// show a key to a company. See the decision record above.
	keys, oldKeys keyStore
	savedFirst    atomic.Bool
	network       http.RoundTripper
	// media is the system's media engine, which a render and the episode's
	// decoder use, see Media. Nil in a machine NewMachine makes,
	// whose videos are decoded and encoded by ffmpeg's own code on the
	// processor, so a test never waits on the system's, which GitHub's
	// virtual Mac now and then never answers. What an ffmpeg offers is
	// learned for the media it was asked with, see learnedFor.
	media *Media
}

// UseSystem hands the machine the system's keychain, its network, its
// media engine and its installed fonts. Each program calls it once, in
// main, on the machine it hands every Engine.
func (m *Machine) UseSystem() {
	m.Fonts.UseMachine()
	m.keys, m.oldKeys = systemKeys(), legacyKeys()
	m.network = http.DefaultTransport
	m.useMedia(SystemMedia(runtime.GOOS))
}

// useMedia hands the machine a media engine, the system's in UseSystem
// and another system's in a test, which checks what that one is handed
// against a stand-in ffmpeg.
func (m *Machine) useMedia(media Media) { m.media = &media }

// LeaveTheMediaEngine takes the system's media engine away from a machine
// UseSystem handed it, so it encodes and decodes with ffmpeg's own codecs
// on the processor, as a test's machine does. Only the build the checks
// from outside the app drive calls it, before anything is decoded, see
// probe_outside.go in cmd/framefairy-app: what it checks is the app,
// not Apple's media engine on a CI runner.
func (m *Machine) LeaveTheMediaEngine() { m.media = nil }

// mediaOf is the machine's media engine, or ffmpeg's own when it was
// handed none.
func (m *Machine) mediaOf() Media {
	if m.media == nil {
		return ffmpegsOwn
	}
	return *m.media
}

// transport is the machine's network, or offline when it was handed none.
func (m *Machine) transport() http.RoundTripper {
	if m.network == nil {
		return offline{}
	}
	return m.network
}

// errOffline is a request on a machine that was handed no network.
var errOffline = errors.New("this program was handed no network")

// offline is the network of a machine handed none: every request is
// refused, and nothing leaves the program.
type offline struct{}

func (offline) RoundTrip(r *http.Request) (*http.Response, error) {
	if r.Body != nil {
		r.Body.Close()
	}
	return nil, errOffline
}

// NewMachine is the machine of a program whose files are on files, with
// the records in their default places and the tools beside the program.
func NewMachine(files afero.Fs) *Machine {
	return &Machine{Files: files, Records: NewRecords(files, "", ""), Tools: tool.NewFinder()}
}

// LearnFrom hands this new machine what before learned about the computer
// both run on: which programs were checked, what each ffmpeg offers, what
// ffmpeg and ffprobe said about files that stay as they are, and the ink of
// caption blocks in the faces the program carries. Each answer is kept
// under what it depends on, so it holds here as it held there. Nothing of
// the jobs comes with it: no episode, record, key, model, caption file or
// choice of decoder. It is called before any job runs on the machine.
//
// The walks' bridge starts every walk on an app opened again on the same
// computer, and without it each walk asked ffmpeg again, twice over, what
// the walk before it had already asked.
func (m *Machine) LearnFrom(before *Machine) {
	m.Tools = before.Tools
	before.learned.Range(func(ask, found any) bool {
		m.learned.Store(ask, found)
		return true
	})
	before.probes.mu.Lock()
	answers := maps.Clone(before.probes.answers)
	before.probes.mu.Unlock()
	m.probes.mu.Lock()
	m.probes.answers = answers
	m.probes.mu.Unlock()
	ink, _ := before.captions()
	m.captionsOnce.Do(func() {
		m.measured = ink
		m.previewTracks = newTrackCache()
	})
}

// captions is the machine's caches of caption work, made once.
func (m *Machine) captions() (*inkCache, *trackCache) {
	m.captionsOnce.Do(func() {
		m.measured = &inkCache{boxes: map[string]inkBox{}}
		m.previewTracks = newTrackCache()
	})
	return m.measured, m.previewTracks
}

// toolAsk is what decides what an ffmpeg offers: the ffmpeg, the encoder
// the run names, if it names one, and the media its encoders are from.
type toolAsk struct {
	ffmpeg, want string
	media        *Media
}

// learnedTool is what one ffmpeg was found to offer, each found the first
// time a job needs it.
type learnedTool struct {
	// finding is held while the subtitle filter is found, so a second
	// ask at the same moment waits for the first one's answer instead of
	// asking ffmpeg again, see SubtitleFilter.
	finding          sync.Mutex
	mu               sync.Mutex
	encoder          Encoder
	hdrEncoder       Encoder
	subtitleTemplate string
}

// learnedFor is what this engine's ffmpeg was found to offer, kept on the
// machine. A run that names another ffmpeg, another encoder or runs on
// other media learns its own.
func (e *Engine) learnedFor() *learnedTool {
	ask := toolAsk{ffmpeg: e.FFmpeg, want: e.WantEncoder, media: e.media}
	if found, ok := e.learned.Load(ask); ok {
		return found.(*learnedTool)
	}
	found, _ := e.learned.LoadOrStore(ask, &learnedTool{})
	return found.(*learnedTool)
}
