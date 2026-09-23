package engine

import (
	"bufio"
	"context"
	"encoding/binary"
	"errors"
	"io"
	"math"
	"os/exec"
	"sort"
	"strings"
	"time"
)

// ---------------------------------------------------------------------------
// Words from the audio
//
// Every timing in this tool comes from here. The audio is transcribed on the
// machine, which gives each word a start and an end, and each word is then
// moved onto the sound it belongs to. Nothing downstream estimates where a
// word sits. Lines, cuts and captions are all built from these words.
// ---------------------------------------------------------------------------

// Token is one piece of a word as the recogniser returns it. A token that
// begins with a space starts a new word.
type Token struct {
	Text     string
	Start    float64
	Duration float64
}

// Recognizer turns audio into timed tokens. The engine only needs this much,
// which keeps the native speech library out of it.
type Recognizer interface {
	Recognize(samples []float32, sampleRate int) []Token
	Close()
}

// SampleRate is what the recogniser is fed.
const SampleRate = 16000

// FrameSeconds is the resolution of the loudness measurement.
const FrameSeconds = 0.01

const frameSamples = SampleRate / 100

// pauseFrames is the shortest quiet, in frames, that counts as a pause when
// placing a word.
const pauseFrames = 15

// Chunks are cut at the quietest moment between these lengths. The model
// could take longer parts, but its memory use grows with the square of
// the length.
const (
	chunkMin = 15.0
	chunkMax = 30.0
)

// Transcript is everything one pass over the audio produced.
type Transcript struct {
	// Words on the episode clock, snapped to the sound.
	Words []Cue
	// RawWords are the same words as the recogniser timed them.
	RawWords []Cue
	// Loudness in dB every FrameSeconds, starting at Start.
	Frames []float32
	Start  float64
	// Silence threshold in dB. Anything quieter counts as a pause.
	Floor float64
	// Mean level in dB over the whole window.
	Mean float64
}

// Levels are the loudness readings the transcript annotations use, one per
// tenth of a second.
func (t *Transcript) Levels() []Reading {
	var out []Reading
	for i := 0; i+10 <= len(t.Frames); i += 10 {
		power := 0.0
		for _, db := range t.Frames[i : i+10] {
			power += math.Pow(10, float64(db)/10)
		}
		level := 10 * math.Log10(power/10+1e-20)
		out = append(out, Reading{roundTo(t.Start+float64(i)*FrameSeconds, 2), level})
	}
	return out
}

func frameDB(samples []float32) float32 {
	sum := 0.0
	for _, s := range samples {
		sum += float64(s) * float64(s)
	}
	return float32(10 * math.Log10(sum/float64(len(samples))+1e-20))
}

// quietestCut picks where to end a chunk: the middle of the quietest 300 ms
// between chunkMin and chunkMax.
func quietestCut(frames []float32) int {
	const window = 30
	low := int(chunkMin / FrameSeconds)
	high := min(int(chunkMax/FrameSeconds), len(frames)) - window
	if high <= low {
		return min(int(chunkMax/FrameSeconds), len(frames))
	}
	best, bestAt := math.Inf(1), low
	running := 0.0
	for i := low; i < low+window; i++ {
		running += float64(frames[i])
	}
	for i := low; i <= high; i++ {
		if running < best {
			best, bestAt = running, i
		}
		if i+window < len(frames) {
			running += float64(frames[i+window]) - float64(frames[i])
		}
	}
	return bestAt + window/2
}

// TokensToWords joins tokens into words. Punctuation that arrives as its own
// token belongs to the word before it.
func TokensToWords(tokens []Token, offset float64) []Cue {
	var words []Cue
	for _, token := range tokens {
		text := token.Text
		start := offset + token.Start
		end := start + max(token.Duration, 0)
		trimmed := strip(text)
		if trimmed == "" {
			continue
		}
		startsWord := strings.HasPrefix(text, " ") || len(words) == 0
		if !startsWord || (isPunctuation(trimmed) && len(words) > 0) {
			last := &words[len(words)-1]
			last.Text += trimmed
			if !isPunctuation(trimmed) {
				last.End = max(last.End, end)
			}
			continue
		}
		words = append(words, Cue{start, end, trimmed})
	}
	return words
}

func isPunctuation(s string) bool {
	for _, r := range s {
		if !strings.ContainsRune(".,!?;:…-–—\"'„“”»«)", r) {
			return false
		}
	}
	return true
}

// SnapWords moves each word onto the sound it belongs to.
//
// The recogniser works on an 80 ms grid and folds silence into the words
// around it, so a word after a pause tends to start early and a word before
// one tends to end late. The loudness frames know where the sound actually
// is. A start that lands in silence moves forward to where the sound begins,
// a start just after a real onset moves back onto it, and an end that runs
// into a pause is pulled back to where the sound stopped.
func SnapWords(words []Cue, frames []float32, start, floor float64) []Cue {
	if len(frames) == 0 {
		return words
	}
	loud := func(f int) bool { return f >= 0 && f < len(frames) && float64(frames[f]) > floor }
	frameAt := func(t float64) int {
		return max(0, min(len(frames)-1, int(math.Floor((t-start)/FrameSeconds+1e-9))))
	}
	timeAt := func(f int) float64 { return start + float64(f)*FrameSeconds }

	out := make([]Cue, 0, len(words))
	for i, w := range words {
		low := start
		if len(out) > 0 {
			low = out[len(out)-1].End
		}
		next := math.Inf(1)
		if i+1 < len(words) {
			next = words[i+1].Start
		}
		begin, end := w.Start, w.End
		f := frameAt(begin)
		if !loud(f) {
			// Stamped in a pause, so the word starts where the sound does. A
			// quiet frame alone is not a pause, since plenty of words begin
			// softly, so the quiet has to last.
			// The model can place a short word entirely inside the pause
			// before it, so the search runs past the word's own end, up to
			// the end of the next word.
			nextEnd := end + 0.6
			if i+1 < len(words) {
				nextEnd = words[i+1].End
			}
			limit := frameAt(math.Min(begin+0.6, math.Max(nextEnd, end)))
			g := f
			for g < limit && !loud(g) {
				g++
			}
			back := f
			for back > frameAt(low) && !loud(back-1) {
				back--
			}
			if loud(g) && g-back >= pauseFrames {
				begin = timeAt(g)
			}
		} else {
			// Inside sound. If the sound began just before, that is the start.
			floorFrame := max(frameAt(begin-0.15), frameAt(low))
			g := f
			for g > floorFrame && loud(g-1) {
				g--
			}
			if !loud(g - 1) {
				begin = timeAt(g)
			}
		}
		// A silence of 120 ms or more inside the word is where it ended.
		run := 0
		for g := frameAt(begin); g < frameAt(end); g++ {
			if loud(g) {
				run = 0
				continue
			}
			run++
			if run >= 12 {
				end = timeAt(g - run + 1)
				break
			}
		}
		begin = math.Max(begin, low)
		if next > begin+0.02 {
			end = math.Min(end, next)
		}
		end = math.Max(end, begin+0.05)
		out = append(out, Cue{roundTo(begin, 3), roundTo(end, 3), w.Text})
	}
	return out
}

// NoiseFloor turns the mean level into a silence threshold. A fixed value
// treats a lowered voice as silence, and a voice dropping for the serious
// part of a story is exactly the material worth keeping, so the threshold
// sits a fixed distance below the window's own level.
func NoiseFloor(mean float64) float64 {
	return math.Max(-55.0, math.Min(mean-22, -35.0))
}

// Transcribe reads the audio once and returns the words with their timings.
func (e *Engine) Transcribe(ctx context.Context, path string, window Window,
	rec Recognizer, silenceDB *float64) (*Transcript, error) {
	return e.transcribe(ctx, path, window, rec, silenceDB, nil)
}

// checkpointEvery is how often, in wall time, a long transcription saves what
// it has so far.
var checkpointEvery = 8 * time.Second

// checkpoint receives the words and loudness frames of everything up to
// covered, which always falls on a chunk boundary.
type checkpoint func(raw []Cue, frames []float32, covered float64)

func (e *Engine) transcribe(ctx context.Context, path string, window Window,
	rec Recognizer, silenceDB *float64, save checkpoint) (*Transcript, error) {
	// Starting part way in is done by dropping samples here, not by asking
	// ffmpeg to seek. A seek into a compressed stream lands on the packet,
	// and whether the build decodes that packet and trims it or begins at
	// the next one differs between builds, by up to 23 ms for AAC. Every
	// word after the start would carry a time that is out by that much, and
	// raw samples arrive without timestamps, so there is no way to notice
	// afterwards. Counting samples is exact everywhere, and audio decodes at
	// a few hundred times real time.
	skip := int(math.Round(window.Start * SampleRate))
	args := []string{"-hide_banner", "-loglevel", "error", "-nostats",
		"-to", fixed(window.End, 3), "-i", path,
		"-map", "0:a:0", "-ac", "1", "-ar", itoa(SampleRate), "-f", "f32le", "-"}
	e.Log.Detail("ffmpeg %s", strings.Join(args, " "))
	cmd := exec.CommandContext(ctx, e.FFmpeg, args...)
	stdout, err := cmd.StdoutPipe()
	if err != nil {
		return nil, err
	}
	var stderr strings.Builder
	cmd.Stderr = &stderr
	if err := cmd.Start(); err != nil {
		return nil, renderErr("cannot start %s: %s", e.FFmpeg, err)
	}
	reader := bufio.NewReaderSize(stdout, 1<<20)
	if skip > 0 {
		e.Log.Progress("winding forward to " + HMS(window.Start))
	}

	t := &Transcript{Start: window.Start}
	var pending []float32
	var raw []Cue
	chunkStart := window.Start
	total := window.End - window.Start
	started := time.Now()
	sumSquares, count := 0.0, 0
	buf := make([]byte, frameSamples*4*50)

	lastSave := time.Now()
	// How far the audio has been heard and how far of it is written down.
	heardTo, savedTo := window.Start, window.Start
	keep := func(covered float64) {
		frames := int(math.Round((covered - window.Start) / FrameSeconds))
		words := append([]Cue(nil), raw...)
		sort.SliceStable(words, func(i, j int) bool { return words[i].Start < words[j].Start })
		save(words, append([]float32(nil), t.Frames[:min(frames, len(t.Frames))]...), covered)
		savedTo = covered
	}
	recognise := func(samples []float32, at float64) {
		raw = append(raw, TokensToWords(rec.Recognize(samples, SampleRate), at)...)
		covered := at + float64(len(samples))/SampleRate
		heardTo = covered
		if save != nil && time.Since(lastSave) >= checkpointEvery {
			lastSave = time.Now()
			keep(covered)
		}
		done := covered - window.Start
		elapsed := time.Since(started).Seconds()
		left := 0.0
		if done > 0 {
			left = elapsed / done * (total - done)
		}
		share := math.Min(done/math.Max(total, 1), 1)
		// Every chunk says how far the audio has been heard, not only
		// every save, so what shows it moves with the work.
		e.Log.ProgressTo("transcribing", share, left, covered)
	}

	carry := []byte{}
	for {
		if ctx.Err() != nil {
			break
		}
		n, readErr := reader.Read(buf)
		data := append(carry, buf[:n]...)
		whole := len(data) / 4 * 4
		for i := 0; i < whole; i += 4 {
			if skip > 0 {
				skip--
				continue
			}
			pending = append(pending, math.Float32frombits(binary.LittleEndian.Uint32(data[i:])))
		}
		carry = append([]byte{}, data[whole:]...)

		// Loudness frames for everything that has arrived and is not yet
		// measured. Frames are kept for the whole window.
		measuredUpTo := int(math.Round((chunkStart-window.Start)/FrameSeconds)) * frameSamples
		for len(t.Frames)*frameSamples+frameSamples <= measuredUpTo+len(pending) {
			offset := len(t.Frames)*frameSamples - measuredUpTo
			frame := pending[offset : offset+frameSamples]
			t.Frames = append(t.Frames, frameDB(frame))
			for _, s := range frame {
				sumSquares += float64(s) * float64(s)
			}
			count += frameSamples
		}

		for float64(len(pending))/SampleRate >= chunkMax {
			first := int(math.Round((chunkStart - window.Start) / FrameSeconds))
			cut := quietestCut(t.Frames[first:]) * frameSamples
			recognise(pending[:cut], chunkStart)
			pending = append([]float32(nil), pending[cut:]...)
			chunkStart += float64(cut) / SampleRate
		}
		if readErr != nil {
			if !errors.Is(readErr, io.EOF) {
				err = readErr
			}
			break
		}
	}
	waitErr := cmd.Wait()
	if ctx.Err() != nil {
		// Stopped, by pause or by a search that wants the machine. What was
		// heard since the last save is written down before it goes, so
		// carrying on starts where the work really got to. Without this a
		// pause threw away up to checkpointEvery of work, minutes of audio
		// at the speed the recogniser runs, and the range picker's edge
		// stood still for all of them after it carried on.
		if save != nil && heardTo > savedTo {
			keep(heardTo)
		}
		e.Log.ClearProgress()
		return nil, ctx.Err()
	}
	if waitErr != nil || err != nil {
		e.Log.ClearProgress()
		return nil, renderErr("could not read the audio of %s:\n%s", path,
			tailRunes(strip(stderr.String()), 400))
	}
	if float64(len(pending))/SampleRate >= 0.2 {
		recognise(pending, chunkStart)
	}
	e.Log.ClearProgress()
	if count == 0 {
		return nil, renderErr("%s has no audio in that part", path)
	}

	t.Mean = 10 * math.Log10(sumSquares/float64(count)+1e-20)
	if silenceDB != nil {
		t.Floor = *silenceDB
	} else {
		t.Floor = NoiseFloor(t.Mean)
	}
	sort.SliceStable(raw, func(i, j int) bool { return raw[i].Start < raw[j].Start })
	t.RawWords = raw
	t.Words = SnapWords(raw, t.Frames, t.Start, t.Floor)
	e.Log.Detail("mean level %s dB, treating below %s dB as silence",
		fixed(t.Mean, 1), fixed(t.Floor, 0))
	return t, nil
}
