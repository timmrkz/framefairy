package engine

import (
	"sync"
	"sync/atomic"

	"github.com/spf13/afero"

	"framefairy/engine/tool"
)

// Machine is what a program keeps for its whole life and shares with every
// job it runs: where files are read and written, the records, how the
// programs it calls are found and run, and what was learned about this
// machine on the way, which video decoder works here, which encoders and
// subtitle filter its ffmpeg has, and the face detector. Each program
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

	// softDecode is set once decoding through the system's own video
	// decoder has failed on this machine, and every decode after it, by
	// any job, is done on the processor.
	softDecode atomic.Bool
	// decoders holds, per episode file, which decoder framing was found to
	// use, so it is looked up and reported once.
	decoders sync.Map
	// learned is what each ffmpeg was found to offer, by what was asked,
	// see learnedFor.
	learned sync.Map

	facesOnce sync.Once
	faces     *faceDetector
}

// NewMachine is the machine of a program whose files are on files, with
// the records in their default places and the tools beside the program.
func NewMachine(files afero.Fs) *Machine {
	return &Machine{Files: files, Records: NewRecords(files, "", ""), Tools: tool.NewFinder()}
}

// toolAsk is what decides what an ffmpeg offers: the ffmpeg, the encoder
// the run names, if it names one, and the system its encoders are for.
type toolAsk struct {
	ffmpeg, want, system string
}

// learnedTool is what one ffmpeg was found to offer, each found the first
// time a job needs it.
type learnedTool struct {
	mu               sync.Mutex
	encoder          Encoder
	hdrEncoder       Encoder
	subtitleTemplate string
}

// learnedFor is what this engine's ffmpeg was found to offer, kept on the
// machine. A run that names another ffmpeg or another encoder learns its
// own.
func (e *Engine) learnedFor() *learnedTool {
	ask := toolAsk{ffmpeg: e.FFmpeg, want: e.WantEncoder, system: e.system}
	if found, ok := e.learned.Load(ask); ok {
		return found.(*learnedTool)
	}
	found, _ := e.learned.LoadOrStore(ask, &learnedTool{})
	return found.(*learnedTool)
}
