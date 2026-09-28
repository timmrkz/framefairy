package engine

import (
	"bytes"
	"context"
	"encoding/binary"
	"fmt"
	"math"
	"os"
	"path/filepath"
	"sort"
	"time"
)

// transcriptVersion changes whenever the stored transcript changes meaning,
// so an old file is transcribed again rather than misread. Version 2 keeps
// a number apart from the word before it, which version 1 glued on.
// Version 3 keeps the parts heard, which need not start at the beginning
// or meet, where the versions before ran from the start with no gaps. An
// older file is read as the one part it is, see heard.
const transcriptVersion = 3

// oldestTranscript is the oldest version still read as it is. Version 1 is
// laid out the same and means the same, only its numbers stick to the word
// before them. Treating it as stale left every episode transcribed before
// version 2 with no waveform and no words, and waiting for hours of
// transcription to split a few numbers apart.
const oldestTranscript = 1

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
	// Versions 1 and 2 only.
	Resume PyFloat `json:"resume,omitempty"`
	// Parts are what is heard, in seconds, in order and apart, and End how
	// long the audio is. Version 3: From, To, Partial and Resume say
	// nothing there, see heard.
	Parts [][2]PyFloat `json:"parts,omitempty"`
	End   PyFloat      `json:"end,omitempty"`
	Words [][3]any     `json:"words"`
}

// heard is what the file has heard, in seconds, in order and apart. An
// older file ran from its start to where it got to, or to where carrying
// on would start again, whichever is earlier.
func (f transcriptFile) heard() [][2]float64 {
	if f.Version >= 3 {
		out := make([][2]float64, 0, len(f.Parts))
		for _, p := range f.Parts {
			out = append(out, [2]float64{float64(p[0]), float64(p[1])})
		}
		return out
	}
	to := float64(f.To)
	if r := float64(f.Resume); r > 0 && r < to {
		to = r
	}
	if to <= float64(f.From) {
		return nil
	}
	return [][2]float64{{float64(f.From), to}}
}

// done says whether the file has heard the whole episode.
func (f transcriptFile) done() bool {
	parts := f.heard()
	if f.Version < 3 {
		return !f.Partial && len(parts) == 1 && parts[0][0] <= 0.001
	}
	end := float64(f.End)
	return end > 0 && len(parts) == 1 && parts[0][0] <= 0.005 && parts[0][1] >= end-0.05
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

// Coverage says how far the transcript reaches from the start of the
// episode without a gap, in seconds, and whether it has heard all of it.
// What is heard further on, where a search or a clip made by hand needed
// it first, is in HeardParts.
func Coverage(source, asrModelDir string) (float64, bool) {
	file, _, _, ok := readEpisodeTranscript(source, asrModelDir)
	if !ok {
		return 0, false
	}
	covered := 0.0
	if parts := file.heard(); len(parts) > 0 && parts[0][0] <= 0.005 {
		covered = parts[0][1]
	}
	return covered, file.done()
}

// HeardParts is every part of the episode its transcript has heard, in
// seconds, in order and apart.
func HeardParts(source, asrModelDir string) [][2]float64 {
	file, _, _, ok := readEpisodeTranscript(source, asrModelDir)
	if !ok {
		return nil
	}
	return file.heard()
}

// Unheard is what of a span of the episode its transcript has not heard
// yet, in order. A gap too short to hold a word is left.
func Unheard(source, asrModelDir string, span Window) []Window {
	return gapsIn(HeardParts(source, asrModelDir), span)
}

func gapsIn(parts [][2]float64, span Window) []Window {
	var gaps []Window
	at := span.Start
	for _, p := range parts {
		if p[1] <= at || p[0] >= span.End {
			continue
		}
		if p[0] > at {
			gaps = append(gaps, Window{at, p[0]})
		}
		at = max(at, p[1])
	}
	if at < span.End {
		gaps = append(gaps, Window{at, span.End})
	}
	out := gaps[:0]
	for _, g := range gaps {
		if g.End-g.Start >= 0.1 {
			out = append(out, g)
		}
	}
	return out
}

// readEpisodeTranscript reads an episode's transcript, heard or not, for
// the speech model in asrModelDir, the default one when that is empty.
func readEpisodeTranscript(source, asrModelDir string) (transcriptFile, []Cue, []float32, bool) {
	stamp, err := stampOf(source)
	if err != nil {
		return transcriptFile{}, nil, nil, false
	}
	if asrModelDir == "" {
		asrModelDir = DefaultModelDir()
	}
	path := filepath.Join(WorkDir(source), "logs", TranscriptName(nil))
	return readTranscriptFile(path, stamp, filepath.Base(filepath.Clean(asrModelDir)))
}

// sliceTranscript cuts a window out of the episode's transcript, so no part
// of the audio is ever heard twice. It has to have heard all of the window.
// The noise floor stays the one measured over all that was heard.
func sliceTranscript(path string, stamp sourceStamp, model string, window Window,
	silenceDB *float64) (*Transcript, bool) {
	file, words, frames, ok := readTranscriptFile(path, stamp, model)
	if !ok || len(gapsIn(file.heard(), window)) > 0 {
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
	if file.Version < oldestTranscript || file.Version > transcriptVersion ||
		file.Source != stamp || file.Model != model {
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
	// The parts are what the file says it heard, in order and apart. A
	// file that says otherwise was not written here.
	parts := file.heard()
	last := 0.0
	for _, p := range parts {
		if p[0] < last || p[1] <= p[0] || math.IsNaN(p[0]) || math.IsInf(p[1], 0) {
			return file, nil, nil, false
		}
		last = p[1]
	}
	if file.Version >= 3 && float64(file.From) != 0 {
		return file, nil, nil, false
	}
	// And no more than the frames reach. The words and the frames are two
	// files written one after the other, so a machine that stops in
	// between, or a copy made by hand, can leave them out of step. What
	// the frames do not reach is heard again.
	reach := float64(file.From) + float64(len(frames))*FrameSeconds
	if last > reach+0.005 {
		var within [][2]PyFloat
		for _, p := range parts {
			if p[0] < reach {
				within = append(within, [2]PyFloat{PyFloat(p[0]), PyFloat(min(p[1], reach))})
			}
		}
		if file.Version >= 3 {
			file.Parts = within
		} else {
			file.To, file.Partial = PyFloat(reach), true
		}
		parts = file.heard()
	}
	var words []Cue
	for _, item := range file.Words {
		low, ok1 := toFloat(item[0])
		high, ok2 := toFloat(item[1])
		text, ok3 := item[2].(string)
		if !ok1 || !ok2 || !ok3 {
			return file, nil, nil, false
		}
		// Only the words of what it heard. A file of version 2 stopped
		// where a search waited kept words up to where it stopped, and the
		// word across that edge is heard again, whole.
		if inParts(parts, low) {
			words = append(words, Cue{low, high, text})
		}
	}
	return file, words, frames, true
}

// inParts says whether a moment is in one of the parts heard. A word may
// start a hair before the part it was heard in, since a part starts on a
// frame and a word's time is rounded to the millisecond.
func inParts(parts [][2]float64, at float64) bool {
	for _, p := range parts {
		if at >= p[0]-0.005 && at < p[1] {
			return true
		}
	}
	return len(parts) > 0 && at < 0.005 && parts[0][0] <= 0.005
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

// hearingPad is how much audio a hearing reads on each side of what it is
// to hear. A part read from the middle of a word hears half a word, which
// the speech model hears as another word. With this much before and after,
// every word of the part is heard whole, and what is made of the half
// words at the ends is left out, see addHeard.
const hearingPad = 3.0

// LoadTranscript returns the words for a window of the episode. What of the
// window the transcript has not heard yet is heard first and added to it,
// so no part of the audio is ever heard twice. The transcript is one file
// for the whole episode, whatever parts of it were heard, see TranscriptName.
func (e *Engine) LoadTranscript(ctx context.Context, source string, window Window, duration float64,
	logsDir, modelDir string, silenceDB *float64) (*Transcript, error) {
	stamp, err := stampOf(source)
	if err != nil {
		return nil, err
	}
	path := filepath.Join(logsDir, TranscriptName(nil))
	model := filepath.Base(filepath.Clean(modelDir))
	if t, ok := sliceTranscript(path, stamp, model, window, silenceDB); ok {
		e.Log.OK("reusing the transcript in %s", filepath.Base(path))
		return t, nil
	}
	if e.OpenRecognizer == nil {
		return nil, renderErr("this build has no speech recogniser")
	}
	if err := os.MkdirAll(logsDir, 0o755); err != nil {
		return nil, err
	}
	// On frames, outward, so what is heard lines up with the loudness.
	span := Window{math.Floor(window.Start/FrameSeconds+1e-6) * FrameSeconds,
		math.Min(math.Ceil(window.End/FrameSeconds-1e-6)*FrameSeconds, duration)}
	file, _, _, ok := readTranscriptFile(path, stamp, model)
	var had [][2]float64
	if ok {
		had = file.heard()
	}
	gaps := gapsIn(had, span)
	todo := 0.0
	for _, g := range gaps {
		todo += g.End - g.Start
	}
	if len(had) > 0 && len(gaps) > 0 {
		e.Log.Info("hearing %s of the window that is not heard yet, from %s", HMS(todo), HMS(gaps[0].Start))
	}

	err = e.Log.Step("transcribing the audio", func() error {
		e.Log.Progress("loading the speech model")
		rec, err := e.OpenRecognizer(modelDir)
		e.Log.ClearProgress()
		if err != nil {
			return renderErr("%s\n%s", err, ModelHelp(modelDir))
		}
		defer rec.Close()
		started := time.Now()
		words, before := 0, 0.0
		for _, gap := range gaps {
			read := Window{math.Max(0, gap.Start-hearingPad), math.Min(duration, gap.End+hearingPad)}
			read.Start = math.Floor(read.Start/FrameSeconds+1e-6) * FrameSeconds
			save := func(raw []Cue, frames []float32, covered float64) {
				if covered <= gap.Start {
					return
				}
				part := Window{gap.Start, math.Min(covered, gap.End)}
				if err := addHeard(path, stamp, model, duration, part, raw, frames, read.Start); err != nil {
					e.Log.Detail("could not save the transcript so far: %s", err)
				}
			}
			t, err := e.transcribe(ctx, source, read, rec, silenceDB, save, hearingShare{gap, before, todo})
			if err != nil {
				return err
			}
			if err := addHeard(path, stamp, model, duration, gap, t.RawWords, t.Frames, read.Start); err != nil {
				return renderErr("could not save the transcript: %s", err)
			}
			words += len(t.RawWords)
			before += gap.End - gap.Start
		}
		e.Log.Info("%s words from %s min of audio, %sx real time",
			commas(words), fixed(todo/60, 1),
			fixed(todo/math.Max(time.Since(started).Seconds(), 0.001), 1))
		return nil
	})
	if err != nil {
		return nil, err
	}
	if t, ok := sliceTranscript(path, stamp, model, window, silenceDB); ok {
		return t, nil
	}
	return nil, renderErr("the finished transcript could not be read back from %s", path)
}

// addHeard adds a part heard to the episode's transcript: the words and
// frames of a reading of the audio from read on, of which part is what was
// to be heard. What the transcript has heard already stays as it is, and
// only what it has not is added, so two hearings of the same audio at once
// both land and neither is kept twice.
//
// Where a part meets what was heard before, both heard the audio across the
// join, each with hearingPad of it past its own end. A word belongs to the
// part it starts in. The same word heard twice can start a few hundredths
// apart, so a word of the new part that starts within a tenth of a second
// of the neighbour's word across the join is that word again, and left out.
func addHeard(path string, stamp sourceStamp, model string, duration float64, part Window,
	raw []Cue, frames []float32, read float64) error {
	defer lockFile(path)()
	file, had, hadFrames, ok := readTranscriptFile(path, stamp, model)
	var parts [][2]float64
	if ok {
		parts = file.heard()
	} else {
		had, hadFrames = nil, nil
	}
	// The frames, from the start of the episode, with what is not heard
	// yet at -90.
	all := make([]float32, max(int(math.Round(duration/FrameSeconds)), len(hadFrames)))
	for i := range all {
		all[i] = -90
	}
	heardFrame := func(n int) bool { return inParts(parts, (float64(n)+0.5)*FrameSeconds) }
	for n, f := range hadFrames {
		if heardFrame(n) {
			all[n] = f
		}
	}
	first := int(math.Round(read / FrameSeconds))
	for i, f := range frames {
		if n := first + i; n < len(all) && !heardFrame(n) {
			all[n] = f
		}
	}

	words := append([]Cue(nil), had...)
	for _, gap := range gapsIn(parts, part) {
		lo, hi := gap.Start, gap.End
		if lo <= 0.005 {
			lo = math.Inf(-1)
		}
		if hi >= duration-0.005 {
			hi = math.Inf(1)
		}
		for _, p := range parts {
			if math.Abs(p[1]-gap.Start) < 0.005 {
				// The last word heard before the join.
				for i := len(had) - 1; i >= 0; i-- {
					if had[i].Start < gap.Start {
						lo = math.Max(lo, had[i].Start+0.1)
						break
					}
				}
			}
			if math.Abs(p[0]-gap.End) < 0.005 {
				// The first word heard after it.
				for _, w := range had {
					if w.Start >= gap.End-0.005 {
						hi = math.Min(hi, w.Start-0.1)
						break
					}
				}
			}
		}
		for _, w := range raw {
			w = Cue{roundTo(w.Start, 3), roundTo(w.End, 3), w.Text}
			if w.Start >= lo && w.Start < hi {
				words = append(words, w)
			}
		}
	}
	sort.SliceStable(words, func(i, j int) bool { return words[i].Start < words[j].Start })
	parts = joinParts(append(parts, [2]float64{roundTo(part.Start, 3), roundTo(part.End, 3)}))

	var heard []float32
	for n, f := range all {
		if heardFrame(n) || inParts(parts, (float64(n)+0.5)*FrameSeconds) {
			heard = append(heard, f)
		}
	}
	stored := make([][2]PyFloat, len(parts))
	for i, p := range parts {
		stored[i] = [2]PyFloat{PyFloat(p[0]), PyFloat(p[1])}
	}
	end := PyFloat(roundTo(duration, 3))
	return writeTranscript(path, transcriptFile{Version: transcriptVersion, Source: stamp, Model: model,
		To: end, Mean: PyFloat(roundTo(meanOfFrames(heard), 3)), Parts: stored, End: end,
		Words: storedWords(words)}, all)
}

// joinParts puts parts in order and makes one of any that touch or overlap.
func joinParts(parts [][2]float64) [][2]float64 {
	sort.Slice(parts, func(i, j int) bool { return parts[i][0] < parts[j][0] })
	var out [][2]float64
	for _, p := range parts {
		if n := len(out); n > 0 && p[0] <= out[n-1][1]+0.005 {
			out[n-1][1] = max(out[n-1][1], p[1])
			continue
		}
		out = append(out, p)
	}
	return out
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
