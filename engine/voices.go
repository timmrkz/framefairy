package engine

import (
	"bufio"
	"context"
	"encoding/binary"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"math"
	"os"
	"os/exec"
	"path/filepath"
	"time"
)

// ---------------------------------------------------------------------------
// Voices
//
// Who speaks when. In an interview the plainest boundary between two thoughts
// is the other person speaking: a question opens one, and the answer runs
// until the next question. A transcript that says who speaks shows the
// model that shape, and lets a clip open on the question.
//
// Two small models do it through the speech library we already ship: one
// finds where somebody speaks, the other tells one voice from another. The
// window is heard once and what was found is kept beside the transcript.
// It is an experiment for now: only the dialogue recipe asks for it.
// ---------------------------------------------------------------------------

// Turn is a stretch of the audio one voice speaks, in seconds from the start
// of the episode. Speakers are numbered from 1 in the order they first speak.
type Turn struct {
	Start   float64 `json:"start"`
	End     float64 `json:"end"`
	Speaker int     `json:"speaker"`
}

// Diarizer tells voices apart in audio at SampleRate. Times are seconds
// from the start of the samples.
type Diarizer interface {
	Turns(samples []float32) []Turn
	Close()
}

// VoiceModel is one file the voices need, fetched from where it is published.
type VoiceModel struct {
	File   string
	URL    string
	SHA256 string
	Size   int64
}

// VoiceModels are the two files, in ~/.framefairy/models/voices. The
// segmentation comes as an archive with the model inside, the embedding as
// the model itself.
var VoiceModels = []VoiceModel{
	{File: "sherpa-onnx-pyannote-segmentation-3-0/model.int8.onnx",
		URL:    "https://github.com/k2-fsa/sherpa-onnx/releases/download/speaker-segmentation-models/sherpa-onnx-pyannote-segmentation-3-0.tar.bz2",
		SHA256: "24615ee884c897d9d2ba09bb4d30da6bb1b15e685065962db5b02e76e4996488", Size: 6_958_444},
	{File: "wespeaker_en_voxceleb_resnet34_LM.onnx",
		URL:    "https://github.com/k2-fsa/sherpa-onnx/releases/download/speaker-recongition-models/wespeaker_en_voxceleb_resnet34_LM.onnx",
		SHA256: "e9848563da86f263117134dfd7ad63c92355b37de492b55e325400c9d9c39012", Size: 26_530_550},
}

// VoicesDir is where the voice models live.
func VoicesDir() string { return filepath.Join(ModelsDir(), "voices") }

// voicesVersion changes when what is kept changes meaning.
const voicesVersion = 1

type voicesFile struct {
	Version int         `json:"version"`
	Source  sourceStamp `json:"source"`
	Window  Window      `json:"window"`
	Turns   []Turn      `json:"turns"`
}

// Voices gives who speaks when in the window, heard once and kept in the
// logs folder.
func (e *Engine) Voices(ctx context.Context, path string, window Window, logDir string) ([]Turn, error) {
	stamp, err := stampOf(path)
	if err != nil {
		return nil, err
	}
	keep := filepath.Join(logDir, fmt.Sprintf("voices-%s-%s.json", trimFloat(window.Start), trimFloat(window.End)))
	if data, err := os.ReadFile(keep); err == nil {
		var kept voicesFile
		if json.Unmarshal(data, &kept) == nil && kept.Version == voicesVersion &&
			kept.Source == stamp && kept.Window == window {
			e.Log.OK("reusing who speaks when in %s", filepath.Base(keep))
			return kept.Turns, nil
		}
	}
	if e.OpenDiarizer == nil {
		return nil, renderErr("this program cannot tell voices apart")
	}
	var turns []Turn
	err = e.Log.Step("telling the voices apart", func() error {
		d, err := e.OpenDiarizer(VoicesDir())
		if err != nil {
			return err
		}
		defer d.Close()
		samples, err := e.readSamples(ctx, path, window)
		if err != nil {
			return err
		}
		began := time.Now()
		turns = numberSpeakers(d.Turns(samples), window.Start)
		heard := float64(len(samples)) / SampleRate
		e.Log.Info("%d voice(s) in %d turns, %sx real time", countSpeakers(turns), len(turns),
			fixed(heard/math.Max(time.Since(began).Seconds(), 0.001), 0))
		return nil
	})
	if err != nil {
		return nil, err
	}
	if logDir != "" {
		if body, err := json.MarshalIndent(voicesFile{voicesVersion, stamp, window, turns}, "", " "); err == nil {
			_ = os.WriteFile(keep, body, 0o644)
		}
	}
	return turns, nil
}

// numberSpeakers moves turns to episode time and numbers the speakers from 1
// in the order they first speak, so A is whoever speaks first.
func numberSpeakers(turns []Turn, offset float64) []Turn {
	order := map[int]int{}
	out := make([]Turn, 0, len(turns))
	for _, t := range turns {
		if _, ok := order[t.Speaker]; !ok {
			order[t.Speaker] = len(order) + 1
		}
		out = append(out, Turn{t.Start + offset, t.End + offset, order[t.Speaker]})
	}
	return out
}

func countSpeakers(turns []Turn) int {
	seen := map[int]bool{}
	for _, t := range turns {
		seen[t.Speaker] = true
	}
	return len(seen)
}

// GiveSpeakers says for each line who speaks most of it. A line nobody's
// turn covers keeps 0, unknown.
func GiveSpeakers(lines []Line, turns []Turn) {
	for i := range lines {
		share := map[int]float64{}
		for _, w := range lines[i].Cues {
			for _, t := range turns {
				if over := math.Min(w.End, t.End) - math.Max(w.Start, t.Start); over > 0 {
					share[t.Speaker] += over
				}
			}
		}
		best, most := 0, 0.0
		for speaker, s := range share {
			if s > most || (s == most && speaker < best) {
				best, most = speaker, s
			}
		}
		lines[i].Speaker = best
	}
}

// readSamples decodes the window's audio at SampleRate, counting samples
// to its start rather than seeking, as transcribe does and for its reason.
func (e *Engine) readSamples(ctx context.Context, path string, window Window) ([]float32, error) {
	args := []string{"-hide_banner", "-loglevel", "error", "-nostats",
		"-to", fixed(window.End, 3), "-i", path,
		"-map", "0:a:0", "-ac", "1", "-ar", itoa(SampleRate), "-f", "f32le", "-"}
	cmd := exec.CommandContext(ctx, e.FFmpeg, args...)
	stdout, err := cmd.StdoutPipe()
	if err != nil {
		return nil, err
	}
	if err := cmd.Start(); err != nil {
		return nil, renderErr("cannot start %s: %s", e.FFmpeg, err)
	}
	reader := bufio.NewReaderSize(stdout, 1<<20)
	skip := int64(math.Round(window.Start*SampleRate)) * 4
	if _, err := io.CopyN(io.Discard, reader, skip); err != nil && !errors.Is(err, io.EOF) {
		_ = cmd.Wait()
		return nil, err
	}
	var samples []float32
	buf := make([]byte, 1<<16)
	for {
		n, err := io.ReadFull(reader, buf)
		for i := 0; i+4 <= n; i += 4 {
			samples = append(samples, math.Float32frombits(binary.LittleEndian.Uint32(buf[i:])))
		}
		if err != nil {
			break
		}
	}
	if err := cmd.Wait(); err != nil && ctx.Err() == nil {
		return nil, renderErr("reading the audio: %s", err)
	}
	return samples, ctx.Err()
}
