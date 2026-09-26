package asr

import (
	"fmt"
	"os"
	"path/filepath"
	"runtime"

	sherpa "github.com/k2-fsa/sherpa-onnx-go/sherpa_onnx"

	"framefairy/engine"
)

// Voices tells voices apart with the two models in engine.VoiceModels.
type Voices struct {
	impl *sherpa.OfflineSpeakerDiarization
}

// How the voices are told apart, measured on sherpa-onnx's four-speaker
// sample. A window step of half a window is five times faster than the
// default tenth and found the same turns to the tenth of a second. 0.4 is
// how alike two stretches must sound to be the same voice.
const (
	voicesShift     = 0.5
	voicesThreshold = 0.4
)

// OpenVoices loads the voice models from dir.
func OpenVoices(dir string) (engine.Diarizer, error) {
	for _, m := range engine.VoiceModels {
		if _, err := os.Stat(filepath.Join(dir, m.File)); err != nil {
			return nil, fmt.Errorf("the voice models are not in %s. make models fetches them", dir)
		}
	}
	threads := min(runtime.NumCPU(), 8)
	config := sherpa.OfflineSpeakerDiarizationConfig{}
	config.Segmentation.Pyannote.Model = filepath.Join(dir, engine.VoiceModels[0].File)
	config.Segmentation.Pyannote.WindowShiftRatio = voicesShift
	config.Segmentation.NumThreads = threads
	config.Embedding.Model = filepath.Join(dir, engine.VoiceModels[1].File)
	config.Embedding.NumThreads = threads
	config.Clustering.NumClusters = -1
	config.Clustering.Threshold = voicesThreshold
	config.MinDurationOn = 0.3
	config.MinDurationOff = 0.5
	impl := sherpa.NewOfflineSpeakerDiarization(&config)
	if impl == nil {
		return nil, fmt.Errorf("the voice models in %s could not be loaded", dir)
	}
	return &Voices{impl: impl}, nil
}

// Turns tells the voices apart in 16 kHz mono audio.
func (v *Voices) Turns(samples []float32) []engine.Turn {
	if len(samples) == 0 {
		return nil
	}
	found := v.impl.Process(samples)
	turns := make([]engine.Turn, len(found))
	for i, s := range found {
		turns[i] = engine.Turn{Start: float64(s.Start), End: float64(s.End), Speaker: s.Speaker}
	}
	return turns
}

// Close frees the models.
func (v *Voices) Close() {
	if v.impl != nil {
		sherpa.DeleteOfflineSpeakerDiarization(v.impl)
		v.impl = nil
	}
}
