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

// Open loads the model in dir.
func Open(dir string) (engine.Recognizer, error) {
	if err := Check(dir); err != nil {
		return nil, err
	}
	provider := os.Getenv("FRAMEFAIRY_ASR_PROVIDER")
	if provider == "" {
		provider = "cpu"
	}
	config := sherpa.OfflineRecognizerConfig{}
	config.FeatConfig.SampleRate = 16000
	config.FeatConfig.FeatureDim = 80
	config.ModelConfig.Transducer.Encoder = filepath.Join(dir, "encoder.int8.onnx")
	config.ModelConfig.Transducer.Decoder = filepath.Join(dir, "decoder.int8.onnx")
	config.ModelConfig.Transducer.Joiner = filepath.Join(dir, "joiner.int8.onnx")
	config.ModelConfig.Tokens = filepath.Join(dir, "tokens.txt")
	config.ModelConfig.ModelType = "nemo_transducer"
	config.ModelConfig.NumThreads = min(runtime.NumCPU(), 8)
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
	stream := sherpa.NewOfflineStream(r.impl)
	defer sherpa.DeleteOfflineStream(stream)
	stream.AcceptWaveform(sampleRate, samples)
	r.impl.Decode(stream)
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
