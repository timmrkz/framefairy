// Package asr runs NVIDIA's Parakeet speech recogniser on the machine, through
// sherpa-onnx. It is the only package that needs cgo, which keeps the engine
// itself plain Go.
package asr

import (
	"fmt"
	"os"
	"path/filepath"
	"runtime"

	sherpa "github.com/k2-fsa/sherpa-onnx-go/sherpa_onnx"

	"framefairy/engine"
)

// Files the model folder has to contain.
var Files = []string{"encoder.int8.onnx", "decoder.int8.onnx", "joiner.int8.onnx", "tokens.txt"}

// Check reports what is missing from a model folder, if anything.
func Check(dir string) error {
	var missing []string
	for _, name := range Files {
		if info, err := os.Stat(filepath.Join(dir, name)); err != nil || info.IsDir() {
			missing = append(missing, name)
		}
	}
	if len(missing) > 0 {
		return fmt.Errorf("the speech model folder %s is missing %v", dir, missing)
	}
	return nil
}

// Recognizer wraps one loaded model.
type Recognizer struct {
	impl *sherpa.OfflineRecognizer
}

// Options choose how the model runs. The zero value is what the app uses.
type Options struct {
	// Provider is cpu, or coreml for Apple's route to the graphics chip and
	// the Neural Engine. Empty reads FRAMEFAIRY_ASR_PROVIDER, then cpu.
	Provider string
	// Threads for the model's own work. 0 is the processor's cores, at most 8.
	Threads int
}

// Open loads the model in dir the way the app runs it on this machine: as
// many copies hearing side by side, with as many threads each, as Mix says
// for its cores and its memory.
func Open(dir string) (engine.Recognizer, error) {
	copies, threads := Mix(ThisMachine())
	p, err := OpenCopies(dir, copies, Options{Threads: threads})
	if err != nil {
		// A nil *Pool in the interface would not read as nil.
		return nil, err
	}
	return p, nil
}

// Machine is what the pool is sized from.
type Machine struct {
	// Cores is every core, and Fast the performance cores where the system
	// tells them apart, zero where it does not.
	Cores, Fast int
	// Memory in bytes, zero when the system does not say.
	Memory int64
}

// ThisMachine is the machine the app is running on.
func ThisMachine() Machine {
	return Machine{Cores: runtime.NumCPU(), Fast: fastCores(), Memory: engine.MachineMemory()}
}

// copyMemory is the memory one copy of the model is allowed: it takes about
// 0.9 GB, and a machine that finds clips with a local model needs most of
// the rest for that. So a copy for every 8 GB.
const copyMemory = 8 << 30

// maxCopies is the most copies measured to help.
const maxCopies = 4

// Mix is how many copies of the model hear side by side on a machine, and
// with how many threads each. It is decided by the machine the app runs
// on, from what was measured with make speechbench on three kinds of it:
//
//   - The threads in all are the performance cores, or every core where
//     the system does not tell them apart. One copy with more threads than
//     that was slower on an M2 Max, whose 8 performance cores heard
//     fastest, and on a machine that cannot tell, every core is what there
//     is.
//   - Those threads go to several copies rather than one, because one copy
//     does not use them: on the M2 Max, one copy of 8 threads heard 47
//     times real time and four copies of 2 heard 83. A machine with fewer
//     than 6 heard best with one thread a copy: 17 times against 16 on the
//     3 cores of an M1 runner, 15 against 11.5 on 4 cores of a cloud
//     machine. The words were the same in every mix.
//   - Every copy holds the model once more, so there is a copy for every
//     8 GB of memory, and at most four, the most measured. Where memory
//     allows fewer copies, each takes more of the threads.
func Mix(m Machine) (copies, threads int) {
	total := m.Fast
	if total <= 0 {
		total = m.Cores
	}
	total = max(total, 1)
	threads = 1
	if total >= 6 {
		threads = 2
	}
	copies = min(max(total/threads, 1), maxCopies)
	if m.Memory > 0 {
		// To the nearest 8 GB: a machine of 16 GB tells Linux a little
		// less than that, and it is still a machine of 16 GB.
		copies = min(copies, max(int((m.Memory+copyMemory/2)/copyMemory), 1))
	}
	threads = max(threads, total/copies)
	return copies, threads
}

// Pool is several copies of the model, each hearing one piece at a time.
// Recognize may be called from as many goroutines at once as there are
// copies, and waits for one to be free.
type Pool struct {
	all  []*Recognizer
	free chan *Recognizer
}

// OpenCopies loads copies of the model in dir, each running the way o says.
// They load side by side too, since each takes as long as the first.
func OpenCopies(dir string, copies int, o Options) (*Pool, error) {
	copies = max(copies, 1)
	loaded := make([]*Recognizer, copies)
	errs := make([]error, copies)
	done := make(chan int, copies)
	for i := range copies {
		go func() {
			loaded[i], errs[i] = OpenWith(dir, o)
			done <- i
		}()
	}
	for range copies {
		<-done
	}
	p := &Pool{free: make(chan *Recognizer, copies)}
	for i := range loaded {
		if errs[i] != nil {
			p.all = loaded
			p.Close()
			return nil, errs[i]
		}
	}
	p.all = loaded
	for _, r := range loaded {
		p.free <- r
	}
	return p, nil
}

// Copies says how many pieces can be heard at once, which the engine asks.
func (p *Pool) Copies() int { return len(p.all) }

// Recognize hears one piece with whichever copy is free.
func (p *Pool) Recognize(samples []float32, sampleRate int) []engine.Token {
	r := <-p.free
	defer func() { p.free <- r }()
	return r.Recognize(samples, sampleRate)
}

// Close frees every copy. Nothing may be hearing when it is called.
func (p *Pool) Close() {
	for _, r := range p.all {
		if r != nil {
			r.Close()
		}
	}
}

// OpenWith loads the model in dir to run the way o says.
func OpenWith(dir string, o Options) (*Recognizer, error) {
	if err := Check(dir); err != nil {
		return nil, err
	}
	provider := o.Provider
	if provider == "" {
		provider = os.Getenv("FRAMEFAIRY_ASR_PROVIDER")
	}
	if provider == "" {
		provider = "cpu"
	}
	threads := o.Threads
	if threads <= 0 {
		threads = min(runtime.NumCPU(), 8)
	}
	config := sherpa.OfflineRecognizerConfig{}
	config.FeatConfig.SampleRate = 16000
	config.FeatConfig.FeatureDim = 80
	config.ModelConfig.Transducer.Encoder = filepath.Join(dir, "encoder.int8.onnx")
	config.ModelConfig.Transducer.Decoder = filepath.Join(dir, "decoder.int8.onnx")
	config.ModelConfig.Transducer.Joiner = filepath.Join(dir, "joiner.int8.onnx")
	config.ModelConfig.Tokens = filepath.Join(dir, "tokens.txt")
	config.ModelConfig.ModelType = "nemo_transducer"
	config.ModelConfig.NumThreads = threads
	config.ModelConfig.Provider = provider
	config.DecodingMethod = "greedy_search"
	impl := sherpa.NewOfflineRecognizer(&config)
	if impl == nil {
		return nil, fmt.Errorf("the speech model in %s could not be loaded", dir)
	}
	return &Recognizer{impl: impl}, nil
}

// Recognize transcribes one piece of 16 kHz mono audio. Times are seconds
// from the start of the samples.
func (r *Recognizer) Recognize(samples []float32, sampleRate int) []engine.Token {
	if len(samples) == 0 {
		return nil
	}
	return r.RecognizeMany([][]float32{samples}, sampleRate)[0]
}

// RecognizeMany transcribes several pieces in one pass of the model, which
// can be faster than one after another. Each answer's times are seconds
// from the start of its own piece.
func (r *Recognizer) RecognizeMany(pieces [][]float32, sampleRate int) [][]engine.Token {
	streams := make([]*sherpa.OfflineStream, len(pieces))
	for i, samples := range pieces {
		streams[i] = sherpa.NewOfflineStream(r.impl)
		defer sherpa.DeleteOfflineStream(streams[i])
		streams[i].AcceptWaveform(sampleRate, samples)
	}
	r.impl.DecodeStreams(streams)
	out := make([][]engine.Token, len(pieces))
	for i, stream := range streams {
		out[i] = tokensOf(stream)
	}
	return out
}

func tokensOf(stream *sherpa.OfflineStream) []engine.Token {
	result := stream.GetResult()
	tokens := make([]engine.Token, 0, len(result.Tokens))
	for i, text := range result.Tokens {
		token := engine.Token{Text: text}
		if i < len(result.Timestamps) {
			token.Start = float64(result.Timestamps[i])
		}
		if i < len(result.Durations) {
			token.Duration = float64(result.Durations[i])
		}
		tokens = append(tokens, token)
	}
	return tokens
}

// Close frees the model.
func (r *Recognizer) Close() {
	if r.impl != nil {
		sherpa.DeleteOfflineRecognizer(r.impl)
		r.impl = nil
	}
}
