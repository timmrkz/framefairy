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

	"framefairy/engine/num"
	"framefairy/engine/runes"
	"framefairy/engine/timed"
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

const frameSamples = int(SampleRate * timed.FrameSeconds)

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
	// Words is what the episode says, see engine/words.go: the words on
	// the episode clock, snapped to the sound. It is the one list of them.
	// Of gives a clip's, with its own corrections applied and split.
	Words []timed.Cue
	// RawWords are the words as the recogniser timed them, which is what
	// is stored.
	RawWords []timed.Cue
	// HeardWords are the recogniser's words snapped to the sound, not
	// split: one for each word the recogniser heard, which is what a
	// correction is kept against. In a clip's words, see Of, they carry
	// its corrections.
	HeardWords []timed.Cue
	// Language is what the words are in, as ISO 639-1, read off them once.
	Language string
	snapped  []timed.Cue
	// keys are what each heard word is kept by, the millisecond the
	// recogniser heard it start, and byKey finds a heard word by its key,
	// see heardIndex.
	keys  []string
	byKey map[string]int
	// fixes are the corrections the words were made with, see Of.
	fixes map[string]string
	// Loudness in dB every FrameSeconds, starting at Start.
	Frames []float32
	Start  float64
	// Silence threshold in dB. Anything quieter counts as a pause.
	Floor float64
	// Mean level in dB over the whole window.
	Mean float64
	// Heard are the parts of the episode the words are for, in seconds, in
	// order and apart. The frames outside them read -90. Only the
	// episode's transcript as a whole has them, see Project.Transcript.
	Heard [][2]float64
}

// Levels are the loudness readings the transcript annotations use, one per
// tenth of a second.
func (t *Transcript) Levels() []timed.Reading {
	var out []timed.Reading
	for i := 0; i+10 <= len(t.Frames); i += 10 {
		power := 0.0
		for _, db := range t.Frames[i : i+10] {
			power += math.Pow(10, float64(db)/10)
		}
		level := 10 * math.Log10(power/10+1e-20)
		out = append(out, timed.Reading{At: num.RoundTo(t.Start+float64(i)*timed.FrameSeconds, 2), Level: level})
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
	return quietestBetween(frames, chunkMin, chunkMax)
}

// quietestBetween is quietestCut between any two lengths, in seconds.
func quietestBetween(frames []float32, shortest, longest float64) int {
	const window = 30
	low := int(shortest / timed.FrameSeconds)
	high := min(int(longest/timed.FrameSeconds), len(frames)) - window
	if high <= low {
		return min(int(longest/timed.FrameSeconds), len(frames))
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

// settlePause is the shortest quiet, in frames, that ends a thought while
// somebody dictates: 0.6 s, longer than a breath between two words, so a
// sentence is not cut in two and given a full stop it never had.
const settlePause = 60

// SettleSamples is settlePause in samples.
const SettleSamples = settlePause * frameSamples

// dictateLongest is how long, in seconds, a piece being dictated may grow
// without a pause before it is settled anyway, at its quietest moment
// in its second half. Until then it is written down again on every pass,
// and every pass may write it a little differently, so it is kept short:
// at 20 s Tim watched words change well behind what he was saying.
const dictateLongest = 12.0

// Settled is where the words of a recording still being made can be
// written down for good, in samples: the middle of the last pause of at
// least settlePause after something was said, or a little way into one
// still going on at the end. Without a pause it is 0, until the recording
// is longer than dictateLongest and is cut at its quietest moment. What
// counts as quiet is measured from the recording itself, a third of the
// way up from its room noise to its voice, since every microphone and
// every room is another level. Plan row 2.189.
func Settled(samples []float32) int {
	frames := make([]float32, len(samples)/frameSamples)
	if len(frames) == 0 {
		return 0
	}
	loud, noise := float32(math.Inf(-1)), float32(math.Inf(1))
	for i := range frames {
		frames[i] = frameDB(samples[i*frameSamples : (i+1)*frameSamples])
		loud = max(loud, frames[i])
		// A microphone starts with a few frames of nothing at all, which
		// is no room's noise.
		if frames[i] > -100 {
			noise = min(noise, frames[i])
		}
	}
	at := 0
	// Twelve decibels between the loudest and the quietest is less than
	// any voice over any room, so nothing was said.
	if loud-noise >= 12 {
		quiet := noise + (loud-noise)/3
		voiced := noise + 2*(loud-noise)/3
		run, spoke := 0, false
		for i, db := range frames {
			if db < quiet {
				run++
				continue
			}
			if spoke && run >= settlePause {
				at = (i - run + run/2) * frameSamples
			}
			run = 0
			spoke = spoke || db >= voiced
		}
		if spoke && run >= settlePause {
			at = (len(frames) - run + settlePause/2) * frameSamples
		}
	}
	if at == 0 && float64(len(samples))/SampleRate > dictateLongest {
		at = quietestBetween(frames, dictateLongest/2, dictateLongest) * frameSamples
	}
	return at
}

// TokensToWords joins tokens into words. Punctuation that arrives as its own
// token belongs to the word before it.
//
// A token that is only a space starts the word after it. The speech model
// has no digit that starts a word, so a number comes as a lone space and
// then its digits, and without the space "dass 5000" is "dass5000".
func TokensToWords(tokens []Token, offset float64) []timed.Cue {
	var words []timed.Cue
	spaced := false
	for _, token := range tokens {
		text := token.Text
		start := offset + token.Start
		end := start + max(token.Duration, 0)
		trimmed := runes.Trim(text)
		if trimmed == "" {
			spaced = spaced || text != ""
			continue
		}
		startsWord := spaced || strings.HasPrefix(text, " ") || len(words) == 0
		spaced = false
		if !startsWord || (isPunctuation(trimmed) && len(words) > 0) {
			last := &words[len(words)-1]
			last.Text += trimmed
			if !isPunctuation(trimmed) {
				last.End = max(last.End, end)
			}
			continue
		}
		words = append(words, timed.Cue{Start: start, End: end, Text: trimmed})
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

// holdOn is the furthest a word's end is moved on to follow its sound:
// the most the recogniser can have cut it short by, a last piece of four
// 80 ms steps and one step more. It is no further because the end of a
// word is also where a pause begins, and pauses decide where a clip cuts
// dead air and where a caption breaks. Under music or room noise loud
// enough to count as sound, an end that followed the sound without a
// limit would run on through every pause.
const holdOn = 0.4

// SnapWords moves each word onto the sound it belongs to.
//
// The recogniser works on an 80 ms grid and folds silence into the words
// around it, so a word after a pause tends to start early and a word before
// one tends to end late. The loudness frames know where the sound actually
// is. A start that lands in silence moves forward to where the sound begins,
// a start just after a real onset moves back onto it, an end that runs
// into a pause is pulled back to where the sound stopped, and an end that
// stops while the sound goes on follows it to where it stops, when it
// stops before the next word.
func SnapWords(words []timed.Cue, frames []float32, start, floor float64) []timed.Cue {
	if len(frames) == 0 {
		return words
	}
	loud := func(f int) bool { return f >= 0 && f < len(frames) && float64(frames[f]) > floor }
	frameAt := func(t float64) int {
		return max(0, min(len(frames)-1, int(math.Floor((t-start)/timed.FrameSeconds+1e-9))))
	}
	timeAt := func(f int) float64 { return start + float64(f)*timed.FrameSeconds }

	out := make([]timed.Cue, 0, len(words))
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
		// A word ends where its last sound does, when 120 ms or more of
		// silence follow that sound before the recogniser's end. A silence
		// with more of the word after it is not the end: the recogniser
		// hears a compound, or words said as one, as one word, and a breath
		// between its parts used to cut it off there, so "liebe" in
		// "sweet-grundschulliebe" lay outside its own word, with no caption
		// and no highlight.
		last := -1
		for g := frameAt(end) - 1; g >= frameAt(begin); g-- {
			if loud(g) {
				last = g
				break
			}
		}
		if last < 0 {
			if frameAt(end)-frameAt(begin) >= 12 {
				end = timeAt(frameAt(begin))
			}
		} else if frameAt(end)-(last+1) >= 12 {
			end = timeAt(last + 1)
		}
		// And the other way round: a word ends where its last sound does
		// when that sound runs on past the recogniser's end. The recogniser
		// gives a word's last piece a duration of at most four of its 80 ms
		// steps, however long the sound is held, so a word before a pause
		// ended while it was still being said. Its caption went early and
		// its block on the clip timeline stopped short of its own waveform.
		// The sound is followed to where it stops, never into the next word
		// and never more than holdOn past the recogniser's end.
		//
		// Only when it does stop. Sound that runs on without a break into
		// the next word is as likely that word's beginning as this word's
		// end, and then the recogniser's end stands. Taken for this word,
		// "ich." at the end of a sentence ran on into the "Also" that
		// began a clip, and the clip's captions began with a word its
		// sound did not hold. Sound that never stops within holdOn, music
		// or room noise, is left alone the same way.
		end = math.Max(end, begin)
		reach := end + holdOn
		if next < reach {
			reach = next
		}
		g := frameAt(end)
		for loud(g) && timeAt(g+1) <= reach+1e-9 {
			g++
		}
		if !loud(g) {
			end = math.Max(end, timeAt(g))
		}
		begin = math.Max(begin, low)
		if next > begin+0.02 {
			end = math.Min(end, next)
		}
		end = math.Max(end, begin+0.05)
		out = append(out, timed.Cue{Start: num.RoundTo(begin, 3), End: num.RoundTo(end, 3), Text: w.Text})
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
func (e *Engine) Transcribe(ctx context.Context, path string, window timed.Window,
	rec Recognizer, silenceDB *float64) (*Transcript, error) {
	return e.transcribe(ctx, path, window, rec, silenceDB, nil, hearingShare{window, 0, window.End - window.Start})
}

// checkpointDefault is how often, in wall time, a long transcription saves
// what it has so far, unless the engine says otherwise, see
// Engine.checkpointEvery.
const checkpointDefault = 8 * time.Second

// checkpoint is how often this engine's transcriptions save.
func (e *Engine) checkpoint() time.Duration {
	if e.checkpointEvery > 0 {
		return e.checkpointEvery
	}
	return checkpointDefault
}

// checkpoint receives the words and loudness frames of everything up to
// covered, which always falls on a chunk boundary.
type checkpoint func(raw []timed.Cue, frames []float32, covered float64)

// hearingShare is the part of the episode a reading of the audio is for,
// which is less than it reads, see hearingPad, how much of the hearing was
// heard before this part of it, and how much there is in all, in seconds of
// audio, so what shows how far it has come reads the parts as one piece of
// work.
type hearingShare struct {
	part          timed.Window
	before, total float64
}

// transcribe hears a window of the episode, which starts on a frame. It
// reads the audio from there the way the loudness does, see audioFrom, and
// stops reading at the window's end.
func (e *Engine) transcribe(ctx context.Context, path string, window timed.Window,
	rec Recognizer, silenceDB *float64, save checkpoint, share hearingShare) (*Transcript, error) {
	first := int(math.Round(window.Start / timed.FrameSeconds))
	window.Start = float64(first) * timed.FrameSeconds
	args := audioFrom(path, first, e.pictureStart(ctx, path))
	e.Log.Detail("ffmpeg %s", strings.Join(args, " "))
	// Its own context, so stopping at the end of the window can end ffmpeg
	// without the job being stopped.
	reading, stopReading := context.WithCancel(ctx)
	defer stopReading()
	cmd := exec.CommandContext(reading, e.FFmpeg, args...)
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

	t := &Transcript{Start: window.Start}
	var pending []float32
	var raw []timed.Cue
	chunkStart := window.Start
	started := time.Now()
	sumSquares, count := 0.0, 0
	buf := make([]byte, frameSamples*4*50)

	lastSave := time.Now()
	// How far the audio has been heard and how far of it is written down.
	heardTo, savedTo := window.Start, window.Start
	keep := func(covered float64) {
		frames := int(math.Round((covered - window.Start) / timed.FrameSeconds))
		words := append([]timed.Cue(nil), raw...)
		sort.SliceStable(words, func(i, j int) bool { return words[i].Start < words[j].Start })
		save(words, append([]float32(nil), t.Frames[:min(frames, len(t.Frames))]...), covered)
		savedTo = covered
	}
	// A piece heard, by one copy of the speech model or another.
	type heardPiece struct {
		at      float64
		samples int
		words   []timed.Cue
		// A copy that panicked, handed back so the job fails the way it
		// does when the one copy panics, rather than taking the app down
		// from a goroutine of its own.
		panicked any
	}
	// Several copies of the speech model hear side by side, each its own
	// piece, when the recogniser has more than one. One copy does not use
	// the cores of a big machine: on an M2 Max, one with 8 threads heard 47
	// times real time and four with 2 threads each 83, with the same words.
	// The pieces are cut as they always are, and what is heard is taken in
	// the order it was said: a piece that is done early waits for the ones
	// before it, so the transcript and every save of it only ever reach as
	// far as all of it has been heard.
	// A recogniser that says when what is heard so far is saved, the
	// walks' stand-in, is asked after each piece in place of the clock, so
	// a walk decides when a transcript is saved, on any machine.
	saves, _ := rec.(interface{ SaveHeard() bool })
	copies := 1
	if many, ok := rec.(interface{ Copies() int }); ok {
		copies = max(many.Copies(), 1)
	}
	results := make(chan heardPiece, copies)
	var order []float64
	done := map[float64]heardPiece{}
	inflight := 0
	// Nothing returns while a copy is still hearing: the caller closes the
	// model the moment this returns, under a copy that is using it.
	defer func() {
		for ; inflight > 0; inflight-- {
			<-results
		}
	}()
	var advance func(p heardPiece)
	collect := func() {
		p := <-results
		inflight--
		if p.panicked != nil {
			panic(p.panicked)
		}
		done[p.at] = p
		for len(order) > 0 {
			next, ok := done[order[0]]
			if !ok {
				break
			}
			delete(done, order[0])
			order = order[1:]
			advance(next)
		}
	}
	drain := func() {
		for inflight > 0 {
			collect()
		}
	}
	recognise := func(samples []float32, at float64) {
		if copies == 1 {
			advance(heardPiece{at: at, samples: len(samples),
				words: TokensToWords(rec.Recognize(samples, SampleRate), at)})
			return
		}
		for inflight >= copies {
			collect()
		}
		inflight++
		order = append(order, at)
		go func() {
			p := heardPiece{at: at, samples: len(samples)}
			defer func() {
				p.panicked = recover()
				results <- p
			}()
			p.words = TokensToWords(rec.Recognize(samples, SampleRate), at)
		}()
	}
	advance = func(p heardPiece) {
		// Stopped, by Stop or by a search that wants the machine, what is
		// still being heard is let go. Taken in, it moved the range
		// picker's edge after the stop, or it was saved without being
		// shown and the edge jumped when the work carried on, Tim's report
		// of 24 October. So what was said to be heard at the stop is what
		// is kept, and a piece is heard again when the work carries on.
		if ctx.Err() != nil {
			return
		}
		raw = append(raw, p.words...)
		covered := p.at + float64(p.samples)/SampleRate
		heardTo = covered
		due := time.Since(lastSave) >= e.checkpoint()
		if saves != nil {
			due = saves.SaveHeard()
		}
		if save != nil && due {
			lastSave = time.Now()
			keep(covered)
		}
		// How far it has come, and how long is left, over all that this
		// hearing hears and not only this part of it. The rest of the
		// episode is not in it: a search of a half hour once said almost
		// six minutes left, the time to hear the four hours of all of it.
		done := max(covered-share.part.Start, 0)
		elapsed := time.Since(started).Seconds()
		left := 0.0
		if done > 0 {
			left = elapsed / done * math.Max(share.total-share.before-done, 0)
		}
		part := math.Min((share.before+done)/math.Max(share.total, 1), 1)
		// Every chunk says how far the audio has been heard, not only
		// every save, so what shows it moves with the work.
		e.Log.ProgressTo("transcribing", part, left, share.part.Start,
			min(max(covered, share.part.Start), share.part.End))
	}

	// Where the window ends, in samples from its start. The chunk is cut
	// exactly there, so what is heard ends on the window's edge and not a
	// chunk past it.
	endAt := int(math.Round((window.End - window.Start) * SampleRate))
	read := 0
	reachedEnd := false
	carry := []byte{}
	for !reachedEnd {
		if ctx.Err() != nil {
			break
		}
		n, readErr := reader.Read(buf)
		data := append(carry, buf[:n]...)
		whole := len(data) / 4 * 4
		for i := 0; i < whole; i += 4 {
			if read == endAt {
				reachedEnd = true
				break
			}
			pending = append(pending, math.Float32frombits(binary.LittleEndian.Uint32(data[i:])))
			read++
		}
		carry = append([]byte{}, data[whole:]...)

		// Loudness frames for everything that has arrived and is not yet
		// measured. Frames are kept for the whole window.
		measuredUpTo := int(math.Round((chunkStart-window.Start)/timed.FrameSeconds)) * frameSamples
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
			first := int(math.Round((chunkStart - window.Start) / timed.FrameSeconds))
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
	if reachedEnd {
		// ffmpeg is still writing, and is told to stop rather than left to
		// write into a pipe nobody reads.
		stopReading()
	}
	waitErr := cmd.Wait()
	if reachedEnd && ctx.Err() == nil {
		waitErr = nil
	}
	// What is still being heard is waited for before anything is saved
	// or said, whichever way the reading ended, and taken in unless the
	// work was stopped, see advance.
	drain()
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
			tailRunes(runes.Trim(stderr.String()), 400))
	}
	if float64(len(pending))/SampleRate >= 0.2 {
		recognise(pending, chunkStart)
		drain()
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
	t.hear(raw)
	e.Log.Detail("mean level %s dB, treating below %s dB as silence",
		num.Fixed(t.Mean, 1), num.Fixed(t.Floor, 0))
	return t, nil
}
