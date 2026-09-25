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

// Open loads the model in dir.
func Open(dir string) (engine.Recognizer, error) {
	r, err := OpenWith(dir, Options{})
	if err != nil {
		// A nil *Recognizer in the interface would not read as nil.
		return nil, err
	}
	return r, nil
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
