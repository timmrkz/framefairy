package engine

import (
	"bytes"
	"context"
	"encoding/binary"
	"fmt"
	"math"
	"os"
	"path/filepath"
	"time"
)

// transcriptVersion changes whenever the stored transcript changes meaning,
// so an old file is transcribed again rather than misread. Version 2 keeps
// a number apart from the word before it, which version 1 glued on.
const transcriptVersion = 2

type sourceStamp struct {
	Name     string `json:"name"`
	Size     int64  `json:"size"`
	Modified string `json:"modified"`
}

// transcriptFile is what logs/words*.json holds. The words are stored as the
// recogniser returned them, and snapping happens on every load, so a better
// snapping rule never needs a new transcription.
type transcriptFile struct {
	Version int         `json:"version"`
	Source  sourceStamp `json:"source"`
	Model   string      `json:"model"`
	From    PyFloat     `json:"from"`
	To      PyFloat     `json:"to"`
	Mean    PyFloat     `json:"mean"`
	// Partial marks a transcription still in progress, or stopped before
	// the end. To is then how far it got.
	Partial bool `json:"partial,omitempty"`
	// Resume is where carrying on starts, when that is before To. A
	// transcription stopped at the end of a window leaves out the word that
	// ran across it, and hears it whole when it carries on from here.
	Resume PyFloat  `json:"resume,omitempty"`
	Words  [][3]any `json:"words"`
}

func stampOf(path string) (sourceStamp, error) {
	info, err := os.Stat(path)
	if err != nil {
		return sourceStamp{}, err
	}
	return sourceStamp{Name: filepath.Base(path), Size: info.Size(),
		Modified: info.ModTime().UTC().Format(time.RFC3339Nano)}, nil
}

// TranscriptName is the cache file for a window of the episode.
func TranscriptName(window *Window) string {
	if window == nil {
		return "words.json"
	}
	return fmt.Sprintf("words-%d-%d.json", int(window.Start), int(window.End))
}

// TranscriptFiles are the files a whole-episode transcript is read from,
// corrections included. What the app keeps in memory is only the current
// transcript while none of them has changed underneath it.
func TranscriptFiles(logsDir string) []string {
	words := filepath.Join(logsDir, TranscriptName(nil))
	return []string{words, framesPath(words), correctionsPath(logsDir)}
}

func framesPath(jsonPath string) string {
	return jsonPath[:len(jsonPath)-len(filepath.Ext(jsonPath))] + ".frames"
}

// writeTranscript saves the frames first and the words last, each replaced
// in one step. The words file says how far the transcript goes, and the
// frames only ever grow, so a reader never sees words without their frames.
func writeTranscript(path string, file transcriptFile, frames []float32) error {
	body, err := MarshalPlan(file)
	if err != nil {
		return err
	}
	var raw bytes.Buffer
	if err := binary.Write(&raw, binary.LittleEndian, frames); err != nil {
		return err
	}
	if err := writeAtomic(framesPath(path), raw.Bytes()); err != nil {
		return err
	}
	return writeAtomic(path, body)
}

func writeAtomic(path string, body []byte) error {
	tmp, err := os.CreateTemp(filepath.Dir(path), "."+filepath.Base(path)+"-*")
	if err != nil {
		return err
	}
	name := tmp.Name()
	_, err = tmp.Write(body)
	if closeErr := tmp.Close(); err == nil {
		err = closeErr
	}
	if err == nil {
		err = os.Rename(name, path)
	}
	if err != nil {
		os.Remove(name)
	}
	return err
}

// meanOfFrames is the mean level over loudness frames, in dB.
func meanOfFrames(frames []float32) float64 {
	if len(frames) == 0 {
		return -90
	}
	power := 0.0
	for _, f := range frames {
		power += math.Pow(10, float64(f)/10)
	}
	return 10 * math.Log10(power/float64(len(frames))+1e-20)
}

func storedWords(words []Cue) [][3]any {
	out := make([][3]any, len(words))
	for i, w := range words {
		out[i] = [3]any{PyFloat(roundTo(w.Start, 3)), PyFloat(roundTo(w.End, 3)), w.Text}
	}
	return out
}

// Coverage says how far the whole-episode transcript of a source reaches,
// in seconds, and whether it is finished.
func Coverage(source, asrModelDir string) (float64, bool) {
	stamp, err := stampOf(source)
	if err != nil {
		return 0, false
	}
	if asrModelDir == "" {
		asrModelDir = DefaultModelDir()
	}
	path := filepath.Join(WorkDir(source), "logs", TranscriptName(nil))
	file, _, _, ok := readTranscriptFile(path, stamp, filepath.Base(filepath.Clean(asrModelDir)))
	if !ok || float64(file.From) > 0.001 {
		return 0, false
	}
	return float64(file.To), !file.Partial
}

// readTranscript loads a cached transcript if it belongs to this source,
// model and window. A mismatch is not an error, just a reason to transcribe.
func readTranscript(path string, stamp sourceStamp, model string, window Window,
	silenceDB *float64) (*Transcript, bool) {
	file, words, frames, ok := readTranscriptFile(path, stamp, model)
	if !ok || file.Partial || math.Abs(float64(file.From)-window.Start) > 0.001 ||
		math.Abs(float64(file.To)-window.End) > 0.001 {
		return nil, false
	}
	return fromStored(words, frames, window.Start, float64(file.Mean), silenceDB), true
}

// sliceTranscript cuts a window out of the whole-episode transcript, so
// choosing a time range in the app never transcribes the same audio twice.
// The noise floor stays the one measured over the whole episode.
func sliceTranscript(path string, stamp sourceStamp, model string, window Window,
	silenceDB *float64) (*Transcript, bool) {
	file, words, frames, ok := readTranscriptFile(path, stamp, model)
	if !ok || float64(file.From) > 0.001 || float64(file.To) < window.End-0.05 {
		return nil, false
	}
	first := max(0, int(math.Round((window.Start-float64(file.From))/FrameSeconds)))
	last := min(len(frames), int(math.Round((window.End-float64(file.From))/FrameSeconds)))
	if first >= last {
		return nil, false
	}
	var inside []Cue
	for _, w := range words {
		if w.Start >= window.Start && w.End <= window.End {
			inside = append(inside, w)
		}
	}
	start := float64(file.From) + float64(first)*FrameSeconds
	return fromStored(inside, frames[first:last], start, float64(file.Mean), silenceDB), true
}

func readTranscriptFile(path string, stamp sourceStamp, model string) (transcriptFile, []Cue,
	[]float32, bool) {
	var file transcriptFile
	data, err := os.ReadFile(path)
	if err != nil || decodeJSON(data, &file) != nil {
		return file, nil, nil, false
	}
	if file.Version != transcriptVersion || file.Source != stamp || file.Model != model {
		return file, nil, nil, false
	}
	raw, err := os.ReadFile(framesPath(path))
	if err != nil || len(raw)%4 != 0 {
		return file, nil, nil, false
	}
	frames := make([]float32, len(raw)/4)
	if binary.Read(bytes.NewReader(raw), binary.LittleEndian, frames) != nil {
		return file, nil, nil, false
	}
	var words []Cue
	for _, item := range file.Words {
		low, ok1 := toFloat(item[0])
		high, ok2 := toFloat(item[1])
		text, ok3 := item[2].(string)
		if !ok1 || !ok2 || !ok3 {
			return file, nil, nil, false
		}
		words = append(words, Cue{low, high, text})
	}
	return file, words, frames, true
}

// storedForm is the transcript as it is written to disk. A fresh
// transcription goes through the same rounding, so a run that transcribes
// and a later run that reads the cache produce the same prompt, and a reply
// already paid for is found again.
func storedForm(t *Transcript) ([][3]any, float64) {
	raw := make([][3]any, len(t.RawWords))
	for i, w := range t.RawWords {
		raw[i] = [3]any{PyFloat(roundTo(w.Start, 3)), PyFloat(roundTo(w.End, 3)), w.Text}
	}
	return raw, roundTo(t.Mean, 3)
}

func fromStored(words []Cue, frames []float32, start, mean float64, silenceDB *float64) *Transcript {
	t := &Transcript{Frames: frames, Start: start, Mean: mean, RawWords: words}
	t.Floor = NoiseFloor(mean)
	if silenceDB != nil {
		t.Floor = *silenceDB
	}
	t.Words = SnapWords(words, frames, start, t.Floor)
	return t
}

// DefaultModelDir is where the speech model is looked for unless told
// otherwise.
func DefaultModelDir() string {
	if dir := os.Getenv("FRAMEFAIRY_ASR_MODEL"); dir != "" {
		return dir
	}
	home, err := os.UserHomeDir()
	if err != nil {
		return ModelName
	}
	return filepath.Join(home, ".framefairy", "models", ModelName)
}

// ModelName is the speech model this version is built and tested against.
const ModelName = "sherpa-onnx-nemo-parakeet-tdt-0.6b-v3-int8"

// LoadTranscript returns the words for a window of the episode, from the
// cache when it is still valid and from the recogniser otherwise.
func (e *Engine) LoadTranscript(ctx context.Context, source string, window Window,
	logsDir, modelDir string, silenceDB *float64, windowed bool) (*Transcript, error) {
	stamp, err := stampOf(source)
	if err != nil {
		return nil, err
	}
	var named *Window
	if windowed {
		named = &window
	}
	path := filepath.Join(logsDir, TranscriptName(named))
	model := filepath.Base(filepath.Clean(modelDir))
	if t, ok := readTranscript(path, stamp, model, window, silenceDB); ok {
		e.Log.OK("reusing the transcript in %s", filepath.Base(path))
		return t, nil
	}
	if windowed {
		whole := filepath.Join(logsDir, TranscriptName(nil))
		if t, ok := sliceTranscript(whole, stamp, model, window, silenceDB); ok {
			e.Log.OK("using %s to %s of the transcript in %s", HMS(window.Start),
				HMS(window.End), filepath.Base(whole))
			return t, nil
		}
	}
	if e.OpenRecognizer == nil {
		return nil, renderErr("this build has no speech recogniser")
	}

	// A whole-episode transcription that stopped part way carries on from
	// where it got to.
	var doneWords []Cue
	var doneFrames []float32
	todo := window
	if !windowed {
		if file, words, frames, ok := readTranscriptFile(path, stamp, model); ok && file.Partial &&
			float64(file.From) <= 0.001 && float64(file.To) < window.End {
			covered := float64(file.To)
			if r := float64(file.Resume); r > 0 && r < covered {
				covered = r
			}
			doneFrames = frames[:min(len(frames), int(math.Round(covered/FrameSeconds)))]
			todo = Window{float64(len(doneFrames)) * FrameSeconds, window.End}
			// It carries on where the loudness ends. The words and the
			// loudness are two files written one after the other, and when
			// the loudness falls short, the words past its end are heard
			// again, so they are not kept twice.
			doneWords = words
			if todo.Start < covered-0.001 {
				doneWords = nil
				for _, w := range words {
					if w.End <= todo.Start+0.001 {
						doneWords = append(doneWords, w)
					}
				}
			}
			e.Log.Info("carrying on from %s, where the last transcription stopped", HMS(todo.Start))
		}
	}
	if err := os.MkdirAll(logsDir, 0o755); err != nil {
		return nil, err
	}
	save := func(words []Cue, frames []float32, covered, resume float64, partial bool) error {
		allWords := append(append([]Cue(nil), doneWords...), words...)
		allFrames := append(append([]float32(nil), doneFrames...), frames...)
		mean := meanOfFrames(allFrames)
		file := transcriptFile{Version: transcriptVersion, Source: stamp, Model: model,
			From: PyFloat(window.Start), To: PyFloat(roundTo(covered, 3)),
			Mean: PyFloat(roundTo(mean, 3)), Partial: partial, Words: storedWords(allWords)}
		if resume < covered-0.0005 {
			file.Resume = PyFloat(roundTo(resume, 3))
		}
		return writeTranscript(path, file, allFrames)
	}
	var progress checkpoint
	if !windowed {
		progress = func(words []Cue, frames []float32, covered, resume float64) {
			if err := save(words, frames, covered, resume, true); err != nil {
				e.Log.Detail("could not save the transcript so far: %s", err)
			}
		}
	}

	var t *Transcript
	err = e.Log.Step("transcribing the audio", func() error {
		e.Log.Progress("loading the speech model")
		rec, err := e.OpenRecognizer(modelDir)
		e.Log.ClearProgress()
		if err != nil {
			return renderErr("%s\n%s", err, ModelHelp(modelDir))
		}
		defer rec.Close()
		started := time.Now()
		t, err = e.transcribe(ctx, source, todo, rec, silenceDB, progress)
		if err != nil {
			return err
		}
		length := todo.End - todo.Start
		e.Log.Info("%s words from %s min of audio, %sx real time",
			commas(len(t.Words)), fixed(length/60, 1),
			fixed(length/math.Max(time.Since(started).Seconds(), 0.001), 1))
		return nil
	})
	if err != nil {
		return nil, err
	}

	// The unsnapped words are what gets stored, so the recogniser's own
	// output is never lost.
	if doneFrames == nil {
		raw, mean := storedForm(t)
		file := transcriptFile{Version: transcriptVersion, Source: stamp, Model: model,
			From: PyFloat(window.Start), To: PyFloat(window.End), Mean: PyFloat(mean), Words: raw}
		stored := make([]Cue, len(raw))
		for i, item := range raw {
			stored[i] = Cue{float64(item[0].(PyFloat)), float64(item[1].(PyFloat)), item[2].(string)}
		}
		t = fromStored(stored, t.Frames, window.Start, mean, silenceDB)
		if err := writeTranscript(path, file, t.Frames); err != nil {
			e.Log.Warn("could not save the transcript: %s", err)
		}
		return t, nil
	}
	if err := save(t.RawWords, t.Frames, window.End, window.End, false); err != nil {
		e.Log.Warn("could not save the transcript: %s", err)
	}
	if t, ok := readTranscript(path, stamp, model, window, silenceDB); ok {
		return t, nil
	}
	return nil, renderErr("the finished transcript could not be read back from %s", path)
}

// ModelHelp says how to get the speech model.
func ModelHelp(dir string) string {
	parent := filepath.Dir(dir)
	return "  Download it once with:\n" +
		"    mkdir -p " + shellQuote(parent) + "\n" +
		"    curl -L -o /tmp/" + ModelName + ".tar.bz2 \\\n" +
		"      https://github.com/k2-fsa/sherpa-onnx/releases/download/asr-models/" + ModelName + ".tar.bz2\n" +
		"    tar -xjf /tmp/" + ModelName + ".tar.bz2 -C " + shellQuote(parent) + "\n" +
		"  or point --asr-model at a folder that has it."
}

// WriteTranscriptSRT writes the whole window as an srt file, for reading
// and checking. It is not used for rendering.
func WriteTranscriptSRT(t *Transcript, path string) error {
	lines := BuildLines(t.Words, nil, nil)
	cues := make([]Cue, len(lines))
	for i, l := range lines {
		cues[i] = Cue{l.Start(), l.End(), l.Text()}
	}
	return WriteSRT(cues, path)
}
