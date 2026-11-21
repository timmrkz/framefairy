package engine

import (
	"bufio"
	"bytes"
	"context"
	"encoding/binary"
	"encoding/json"
	"errors"
	"io"
	"math"
	"os"
	"path/filepath"
	"runtime"
	"slices"
	"strconv"
	"strings"
	"sync"
	"time"

	"github.com/spf13/afero"

	"framefairy/engine/fileio"
	"framefairy/engine/num"
	"framefairy/engine/timed"
	"framefairy/engine/tool"
)

// ---------------------------------------------------------------------------
// The loudness of the whole episode
//
// The waveform used to come from the transcription, which measures the
// loudness as it hears. The transcription hears only what a search or a
// clip made by hand needs, so the rest of the clip timeline stayed bare
// until one reached it, and the first minutes stayed bare until the speech
// model had loaded.
//
// Measuring the loudness needs no speech model. It is the audio decoded and
// a sum of squares every 10 ms, the same frames the transcription measures,
// from the same 16 kHz mono samples. Audio decodes at a few hundred times
// real time, so a whole episode is measured in seconds, and the waveform
// fills in as it goes.
//
// It goes where the clip timeline looks first. Measured from the start
// alone, a playhead put near the end of a four hour episode waited for the
// three hours and more before it. So the measuring is told what the clip
// timeline shows, measures that first, then on from there, then whatever
// is left, and when the view moves to a part not measured yet it stops and
// starts again there. What it has is kept as parts, so nothing measured is
// measured twice and a measuring cut off carries on where it was.
// ---------------------------------------------------------------------------

// levelsVersion changes whenever what the files hold changes meaning. Up
// to 1 the frames ran from the start with no gaps.
const levelsVersion = 2

// levelsDefault is how often the frames measured so far are written, so
// the waveform grows while the measuring runs, and how often the measuring
// asks where the clip timeline is looking, unless the engine counts reads
// instead, see Engine.levelsReads.
const levelsDefault = 500 * time.Millisecond

// levelsFile is what logs/levels.json holds. The frames are in
// logs/levels.frames, written before it, so a reader never sees a part the
// frames do not reach.
type levelsFile struct {
	Version int         `json:"version"`
	Source  sourceStamp `json:"source"`
	// Parts are what is measured, as frame numbers from and to, the second
	// not included, in order and apart.
	Parts [][2]int `json:"parts"`
	// End is how many frames the audio has, once a measuring has reached
	// its end, and 0 until then.
	End int `json:"end,omitempty"`
	// Done says the parts are the whole audio.
	Done bool `json:"done"`
	// Loudest is the loudest frame, in dB, once all of it is measured.
	Loudest *float32 `json:"loudest,omitempty"`
}

// silent says the whole audio is measured and none of it is as loud as
// the loudness the clip timeline is sent can carry, see Loudness: there is
// nothing to draw but a flat line.
func (f levelsFile) silent() bool {
	return f.Done && f.Loudest != nil && *f.Loudest < loudnessFloor
}

func levelsPaths(logsDir string) (string, string) {
	return filepath.Join(logsDir, "levels.json"), filepath.Join(logsDir, "levels.frames")
}

// Levels is the loudness of an episode as far as it has been measured.
type Levels struct {
	// Frames are dB every FrameSeconds from the start of the episode, as
	// far as the furthest part measured. A frame outside the parts is -90.
	Frames []float32
	// Parts are the frames measured, from and to, the second not included.
	Parts [][2]int
	// Done says they are the whole audio.
	Done bool
}

// To is how far the furthest part measured reaches, in seconds.
func (l Levels) To() float64 { return float64(len(l.Frames)) * timed.FrameSeconds }

// Seconds are the parts measured, in seconds.
func (l Levels) Seconds() [][2]float64 { return partSeconds(l.Parts) }

func partSeconds(parts [][2]int) [][2]float64 {
	out := make([][2]float64, len(parts))
	for i, p := range parts {
		out[i] = [2]float64{num.RoundTo(float64(p[0])*timed.FrameSeconds, 2), num.RoundTo(float64(p[1])*timed.FrameSeconds, 2)}
	}
	return out
}

// Over lays the loudness measured here over a transcript's, which measures
// the same frames from the same samples as it hears, so the waveform has
// whatever either has measured. Either may be missing.
func (l Levels) Over(t *Transcript) *Transcript {
	if len(l.Frames) == 0 {
		return t
	}
	if t == nil || l.Done {
		return &Transcript{Frames: l.Frames}
	}
	at := int(math.Round(t.Start / timed.FrameSeconds))
	frames := make([]float32, max(len(l.Frames), at+len(t.Frames)))
	for i := range frames {
		frames[i] = -90
	}
	copy(frames[at:], t.Frames)
	for _, p := range l.Parts {
		copy(frames[p[0]:p[1]], l.Frames[p[0]:p[1]])
	}
	return &Transcript{Frames: frames}
}

// The loudness as the clip timeline keeps it, see Loudness: a reading of
// -90 to 0 dB as 0 to 180, half a decibel a step. The waveform is drawn
// from -60 dB up, a pixel or two for each decibel at most, so half a
// decibel is finer than any column can show.
const (
	loudnessFloor = -90
	loudnessSteps = 2
	loudnessTop   = -loudnessFloor * loudnessSteps
	// loudnessHead is how long the head of the bytes is.
	loudnessHead = 8
)

// Loudness is the loudness of an episode for the clip timeline, which
// keeps the whole of it and draws every view from it, so a swipe or a
// pinch asks the Go side for nothing, see
// docs/decisions/2026-11-18-the-window-holds-the-loudness.md.
//
// The head says how to read the rest, so the window keeps no copy of how
// loud a step is: the number of readings as four bytes and how long one
// is in microseconds as two, both little end first, then the floor in dB
// as a signed byte and how many steps make a decibel. Then a byte for
// every reading from the start of the episode, then the levels for
// zooming out, one after another: each is the loudest of every two of
// the one before it, a last one alone kept as it is, down to a level of
// one. Together they are about as long again as the readings, two hours
// of them 1.4 MB, and a column of the clip timeline zoomed out to a whole
// four hour video is then a few of them rather than the thousands of
// readings it covers.
//
// A transcript that starts after the start of the episode is silent
// before it, and one that is nil has no readings.
func Loudness(t *Transcript) []byte {
	var frames []float32
	lead := 0
	if t != nil {
		frames = t.Frames
		lead = max(int(math.Round(t.Start/timed.FrameSeconds)), 0)
	}
	n := lead + len(frames)
	size := loudnessHead
	for k := n; k > 0; k = (k + 1) / 2 {
		size += k
		if k == 1 {
			break
		}
	}
	out := make([]byte, size)
	binary.LittleEndian.PutUint32(out, uint32(n))
	binary.LittleEndian.PutUint16(out[4:], uint16(math.Round(timed.FrameSeconds*1e6)))
	floor := int8(loudnessFloor)
	out[6] = byte(floor)
	out[7] = loudnessSteps
	level := out[loudnessHead : loudnessHead+n]
	for i, f := range frames {
		level[lead+i] = loudnessByte(f)
	}
	at := loudnessHead + n
	for len(level) > 1 {
		next := out[at : at+(len(level)+1)/2]
		for i := range next {
			next[i] = level[2*i]
			if 2*i+1 < len(level) {
				next[i] = max(next[i], level[2*i+1])
			}
		}
		at += len(next)
		level = next
	}
	return out
}

// loudnessByte is a reading in dB as Loudness keeps it, rounded to the
// nearest step. A reading that is no number is silence: the comparison
// that keeps it at the floor is false for NaN too. It is worked out for
// every reading of a video each time the window reads the loudness, so it
// is plain arithmetic rather than math.Round and math.IsNaN.
func loudnessByte(db float32) byte {
	step := (float64(db) - loudnessFloor) * loudnessSteps
	if !(step > 0) {
		return 0
	}
	if step >= loudnessTop {
		return loudnessTop
	}
	return byte(step + 0.5)
}

// LevelsFiles are the files the loudness of an episode is read from.
func LevelsFiles(source string) []string {
	meta, frames := levelsPaths(filepath.Join(WorkDir(source), "logs"))
	return []string{meta, frames}
}

// readLevelsFile reads what logs/levels.json says, when it is about the
// file as it is now and its parts are in order.
func readLevelsFile(fsys afero.Fs, source string) (levelsFile, sourceStamp, bool) {
	stamp, err := stampOf(fsys, source)
	if err != nil {
		return levelsFile{}, stamp, false
	}
	meta, _ := levelsPaths(filepath.Join(WorkDir(source), "logs"))
	var file levelsFile
	data, err := afero.ReadFile(fsys, meta)
	if err != nil || decodeJSON(data, &file) != nil ||
		file.Version != levelsVersion || file.Source != stamp {
		return levelsFile{}, stamp, false
	}
	last := 0
	for _, p := range file.Parts {
		if p[0] < last || p[1] <= p[0] {
			return levelsFile{}, stamp, false
		}
		last = p[1]
	}
	return file, stamp, true
}

// LevelsReach says what of an episode's loudness has been measured, in
// seconds, and whether all of it, from the small json alone. The library
// asks it for every episode, so it never reads the frames.
func LevelsReach(fsys afero.Fs, source string) ([][2]float64, bool) {
	file, _, ok := readLevelsFile(fsys, source)
	if !ok {
		return nil, false
	}
	return partSeconds(file.Parts), file.Done
}

// ReadLevels reads what has been measured of an episode's loudness. An
// episode never measured, or measured before its file changed, has none.
func ReadLevels(fsys afero.Fs, source string) Levels {
	file, _, ok := readLevelsFile(fsys, source)
	if !ok || len(file.Parts) == 0 {
		return Levels{}
	}
	_, framesAt := levelsPaths(filepath.Join(WorkDir(source), "logs"))
	raw, err := afero.ReadFile(fsys, framesAt)
	if err != nil || len(raw)%4 != 0 {
		return Levels{}
	}
	frames := make([]float32, len(raw)/4)
	if binary.Read(bytes.NewReader(raw), binary.LittleEndian, frames) != nil {
		return Levels{}
	}
	// The frames may reach further than the parts the json gives, never
	// less far: they are written first. What the json gives is what is
	// certain.
	want := file.Parts[len(file.Parts)-1][1]
	if want > len(frames) {
		return Levels{}
	}
	frames = frames[:want]
	in, next := false, 0
	for i, f := range frames {
		for next < len(file.Parts) && i >= file.Parts[next][1] {
			next++
		}
		in = next < len(file.Parts) && i >= file.Parts[next][0]
		if !in || math.IsNaN(float64(f)) || math.IsInf(float64(f), 0) {
			frames[i] = -90
		}
	}
	return Levels{Frames: frames, Parts: file.Parts, Done: file.Done}
}

// levelState is what a measuring has, frame by frame.
type levelState struct {
	frames []float32
	got    []bool
	// end is how many frames the audio has, or -1 while nobody knows.
	end int
	// fresh is the frames put since they were last written, as runs from
	// and to, the second not included, one for each place being measured,
	// see writeLevels. whole says the frames file holds nothing of this
	// measuring yet, so it is written whole, and written how far the parts
	// written last reach, which the file holds at least.
	fresh   [][2]int
	whole   bool
	written int
}

func (s *levelState) put(n int, f float32) {
	for len(s.frames) <= n {
		s.frames = append(s.frames, -90)
		s.got = append(s.got, false)
	}
	s.frames[n], s.got[n] = f, true
	// A frame is put once, so it is in no run yet. Several places are
	// measured at once, and one run from the first to the last fresh frame
	// rewrote all that lies between them at every write.
	for i, r := range s.fresh {
		switch n {
		case r[1]:
			s.fresh[i][1]++
			return
		case r[0] - 1:
			s.fresh[i][0]--
			return
		}
	}
	s.fresh = append(s.fresh, [2]int{n, n + 1})
}

// ends says the sound ends at frame n at the latest, and drops what lies
// past it.
func (s *levelState) ends(n int) {
	if s.end >= 0 {
		n = min(n, s.end)
	}
	s.end = n
	s.frames, s.got = s.frames[:min(n, len(s.frames))], s.got[:min(n, len(s.got))]
}

func (s *levelState) has(n int) bool { return n < len(s.got) && s.got[n] }

// gap is the first frame not measured from from on, before to.
func (s *levelState) gap(from, to int) (int, bool) {
	if s.end >= 0 {
		to = min(to, s.end)
	}
	for n := max(from, 0); n < to; n++ {
		if !s.has(n) {
			return n, true
		}
	}
	return 0, false
}

// next is where to measure next, for a clip timeline showing from to to:
// the first gap in what it shows, then on from there, then from the start.
func (s *levelState) next(from, to float64) (int, bool) {
	first := int(math.Floor(from / timed.FrameSeconds))
	last := int(math.Ceil(to / timed.FrameSeconds))
	if last > first {
		if n, ok := s.gap(first, math.MaxInt); ok {
			return n, true
		}
	}
	return s.gap(0, math.MaxInt)
}

// shown says whether a frame is in what the clip timeline shows.
func shown(n int, from, to float64) bool {
	return to > from && float64(n)*timed.FrameSeconds >= from-timed.FrameSeconds && float64(n)*timed.FrameSeconds < to
}

// leave says whether a measuring at frame at goes elsewhere: when the clip
// timeline shows a part not measured yet and at is not in what it shows.
func (s *levelState) leave(at int, focus func() (float64, float64)) bool {
	from, to := focus()
	want, ok := s.next(from, to)
	return ok && want != at && shown(want, from, to) && !shown(at, from, to)
}

func (s *levelState) parts() [][2]int {
	var out [][2]int
	for n, got := range s.got {
		switch {
		case !got:
		case len(out) > 0 && out[len(out)-1][1] == n:
			out[len(out)-1][1] = n + 1
		default:
			out = append(out, [2]int{n, n + 1})
		}
	}
	return out
}

func (s *levelState) measured() float64 {
	total := 0
	for _, p := range s.parts() {
		total += p[1] - p[0]
	}
	return float64(total) * timed.FrameSeconds
}

func (s *levelState) done() bool {
	_, left := s.gap(0, math.MaxInt)
	return s.end >= 0 && !left
}

// LevelsLook is where the clip timeline looks, which the measuring goes to
// first, and is handed every reading as it is measured, the way
// LoudnessSchedule answers the window with them.
type LevelsLook interface {
	View() (from, to float64)
	Put(first int, frames []float32)
}

// nowhere looks at nothing and is handed nothing: a measuring from the
// start, for the disk alone.
type nowhere struct{}

func (nowhere) View() (float64, float64) { return 0, 0 }
func (nowhere) Put(int, []float32)       {}

// MeasureLevels measures the loudness of the whole episode, writing what it
// has every half second, and tells progress how many seconds of it are
// measured. It measures where look says the clip timeline looks first,
// follows it when it moves, and hands look each reading as it is measured.
// A look that is nil, or shows nothing, has it measure from the start. An
// episode measured already is left as it is, and one measured in part
// carries on.
func (e *Engine) MeasureLevels(ctx context.Context, source string, look LevelsLook,
	progress func(measured float64)) error {
	if look == nil {
		look = nowhere{}
	}
	had := ReadLevels(e.Files, source)
	if had.Done {
		return nil
	}
	stamp, err := stampOf(e.Files, source)
	if err != nil {
		return err
	}
	logs := filepath.Join(WorkDir(source), "logs")
	if err := MakeDir(e.Files, logs); err != nil {
		return err
	}
	st := &levelState{end: -1}
	if file, _, ok := readLevelsFile(e.Files, source); ok && file.End > 0 {
		st.end = file.End
	}
	for _, p := range had.Parts {
		for n := p[0]; n < p[1]; n++ {
			st.put(n, had.Frames[n])
		}
	}
	// What was measured before is in the frames file already. Without it,
	// the file is of a measuring of the file before it changed, or of none.
	st.fresh, st.whole = nil, len(had.Parts) == 0
	if len(had.Parts) > 0 {
		st.written = had.Parts[len(had.Parts)-1][1]
	}
	m := &measuring{st: st, at: map[int]int{}}
	// tell is called with m.mu held, by whichever measurer writes.
	tell := func() {
		if err := writeLevels(e.Files, logs, stamp, st); err == nil && progress != nil {
			progress(st.measured())
		}
	}
	duration, soundless := e.soundless(ctx, source)
	if !soundless {
		m.length = int(math.Ceil(duration / timed.FrameSeconds))
	}
	if soundless {
		// No sound to measure, so it is silence all through: the clip
		// timeline draws the video as silent rather than as not measured,
		// and nothing asks ffmpeg for a sound that is not there, #831.
		st.end = int(math.Round(duration / timed.FrameSeconds))
		silence := make([]float32, st.end)
		for n := range st.end {
			silence[n] = noSoundDB
			if !st.has(n) {
				st.put(n, noSoundDB)
			}
		}
		look.Put(0, silence)
	}
	run, stop := context.WithCancel(ctx)
	defer stop()
	var (
		wg     sync.WaitGroup
		once   sync.Once
		failed error
	)
	for id := range e.measurers() {
		wg.Go(func() {
			defer func() {
				m.mu.Lock()
				delete(m.at, id)
				m.mu.Unlock()
			}()
			for {
				from, to := look.View()
				m.mu.Lock()
				start, ok := m.claim(id, from, to)
				m.mu.Unlock()
				if !ok {
					return
				}
				moved, err := e.measureFrom(run, source, m, id, start, look, tell)
				if err != nil {
					once.Do(func() { failed = err })
					stop()
					return
				}
				// A part that ended between two ticks is written when it
				// ends, not at the first tick of the next one. The
				// measurer that left for the view has it as its place
				// already, so no other leaves for it too.
				m.mu.Lock()
				if !moved {
					delete(m.at, id)
				}
				tell()
				m.mu.Unlock()
			}
		})
	}
	wg.Wait()
	if ctx.Err() != nil {
		return ctx.Err()
	}
	if failed != nil {
		return failed
	}
	if err := writeLevels(e.Files, logs, stamp, st); err != nil {
		return err
	}
	if progress != nil {
		progress(st.measured())
	}
	// Said once it is all measured, so a waveform that looks empty can be
	// told apart in the log: a video that is silent, or one never
	// measured, #831.
	if loudest, ok := st.loudest(); ok {
		e.Log.Info("measured the loudness of %.1f s: the loudest %.1f dB", st.measured(), loudest)
	}
	return nil
}

// loudest is the loudest of what was measured, in dB, or false while
// nothing is measured. A video that is silent all through is about -200.
func (s *levelState) loudest() (float32, bool) {
	measured := false
	loudest := float32(math.Inf(-1))
	for n, f := range s.frames {
		if s.got[n] {
			measured = true
			loudest = max(loudest, f)
		}
	}
	return loudest, measured
}

// measureFrom measures from frame start on, until it reaches a frame
// measured already, the end of the audio, or a clip timeline that moved to
// a part not measured yet.
func (e *Engine) measureFrom(ctx context.Context, source string, m *measuring, id, start int,
	look LevelsLook, tell func()) (moved bool, err error) {
	st := m.st
	run, stop := context.WithCancel(ctx)
	defer stop()
	cmd := e.Runner.Command(run, e.FFmpeg, audioFrom(source, start, e.pictureStart(ctx, source))...)
	var stderr strings.Builder
	said := tool.NewCutWriter(&stderr)
	cmd.Stderr = said
	reading, err := e.Runner.StartReading(run, cmd, said)
	if err != nil {
		return false, renderErr("cannot start %s: %s", e.FFmpeg, err)
	}
	n := start
	// What a read brought is handed to look after the read, so the window
	// is answered with it at once, rather than at the next write to the
	// disk and the news of it, issue #968.
	fresh := make([]float32, 0, 256)
	hand := func() {
		look.Put(n-len(fresh), fresh)
		fresh = fresh[:0]
	}
	// Where the clip timeline looks is asked after every read, a lock and
	// two numbers, and weighed again only when it moved, or at a write.
	from, to := look.View()
	end, readErr := e.readLevels(run, bufio.NewReaderSize(reading.Stdout, 1<<20), func(f float32) bool {
		m.mu.Lock()
		defer m.mu.Unlock()
		if st.has(n) {
			// Measured already, by a measurer that started there.
			return false
		}
		st.put(n, f)
		fresh = append(fresh, f)
		n++
		m.at[id] = n
		return true
	}, func(due bool) bool {
		changed := false
		if now, until := look.View(); now != from || until != to {
			from, to, changed = now, until, true
		}
		m.mu.Lock()
		defer m.mu.Unlock()
		hand()
		if due {
			tell()
		}
		if (changed || due) && m.leave(id, n, from, to) {
			moved = true
			return false
		}
		return true
	})
	m.mu.Lock()
	hand()
	m.mu.Unlock()
	if !end {
		// Stopped before the end: ffmpeg is still writing, and is told to
		// stop rather than left to write into a pipe nobody reads.
		stop()
	}
	left, waitErr := reading.Finish()
	if ctx.Err() != nil {
		return false, ctx.Err()
	}
	if left {
		// Stopped once enough was read, so what was read stands.
		e.Log.Warn("ffmpeg %s", tool.LeftBehind)
	}
	if readErr != nil && !errors.Is(readErr, context.Canceled) {
		return false, readErr
	}
	m.mu.Lock()
	defer m.mu.Unlock()
	switch {
	case end && n <= start && start > 0:
		// Nothing past start: the audio ends before it, or where it
		// ends, if another measurer found that already.
		st.ends(start)
	case end && waitErr != nil:
		return false, renderErr("ffmpeg could not read the audio: %s", strings.TrimSpace(stderr.String()))
	case end:
		st.ends(n)
	case moved:
		e.Log.Detail("the clip timeline moved on, so the loudness is measured there next")
	}
	return moved, nil
}

// measurers is how many ffmpeg measure the loudness of a video at once,
// unless the engine says otherwise, see Engine.levelsMeasurers. Decoding
// the sound is one core's work for each, so half the cores measure a four
// hour video in a few seconds, where one took half a minute, and leave the
// rest to the hearing that runs beside it, #968.
func (e *Engine) measurers() int {
	if e.levelsMeasurers > 0 {
		return e.levelsMeasurers
	}
	return min(max(1, runtime.GOMAXPROCS(0)/2), 8)
}

// measuring is the loudness of a video measured by several ffmpeg at
// once: what they have between them, and where each of them is. Every
// measurer does what it does with them with mu held.
type measuring struct {
	mu sync.Mutex
	st *levelState
	// at is the frame each measurer puts next, by its number.
	at map[int]int
	// length is how many frames the probe says the video has, or 0 when
	// it said nothing. The measuring itself finds the end of the sound,
	// st.end, which may differ by a packet.
	length int
}

// inView says whether a measurer other than id is in what the clip
// timeline shows, from to to.
func (m *measuring) inView(id int, from, to float64) bool {
	for other, at := range m.at {
		if other != id && shown(at, from, to) {
			return true
		}
	}
	return false
}

// leave says whether measurer id, at frame at, goes to what the clip
// timeline shows: a part not measured yet that no other measurer is in.
// It takes that place at once, so no other measurer leaves for it too.
func (m *measuring) leave(id, at int, from, to float64) bool {
	if m.inView(id, from, to) || !m.st.leave(at, func() (float64, float64) { return from, to }) {
		return false
	}
	want, _ := m.st.next(from, to)
	m.at[id] = want
	return true
}

// claim is where measurer id starts next, and takes that place. The view
// comes first, when it shows a part not measured yet that no measurer is
// in. Then the longest work left: a part nobody measures, from its start,
// or the half still ahead of a measurer, from its middle, so two share it.
// Half a part is shared only when it is longer than levelsShare, about
// what an ffmpeg takes to start. It says false when there is nothing worth
// starting for.
func (m *measuring) claim(id int, from, to float64) (int, bool) {
	st := m.st
	if want, ok := st.next(from, to); ok && shown(want, from, to) && !m.inView(id, from, to) {
		m.at[id] = want
		return want, true
	}
	length := m.length
	if st.end >= 0 {
		length = st.end
	}
	best, work := 0, 0
	var at []int
	for other, n := range m.at {
		if other != id {
			at = append(at, n)
		}
	}
	slices.Sort(at)
	n := 0
	for n < length {
		gap, ok := st.gap(n, length)
		if !ok {
			break
		}
		end := gap
		for end < length && !st.has(end) {
			end++
		}
		// The measurers in this part, in order, each with the frames up
		// to the next one or to the part's end ahead of it.
		first := end
		for i, p := range at {
			if p < gap || p >= end {
				continue
			}
			first = min(first, p)
			next := end
			if i+1 < len(at) && at[i+1] < end {
				next = at[i+1]
			}
			if half := (next - p) / 2; half > levelsShare && half > work {
				best, work = p+half, half
			}
		}
		if first > gap && first-gap > work {
			best, work = gap, first-gap
		}
		n = end
	}
	if work == 0 && st.end < 0 && len(at) == 0 {
		// Where the sound ends is not known yet, and the probe said
		// nothing or too little: one measurer from the first gap on,
		// which finds it.
		var ok bool
		if best, ok = st.next(from, to); !ok {
			return 0, false
		}
	} else if work == 0 {
		return 0, false
	}
	m.at[id] = best
	return best, true
}

// levelsShare is how many frames, 30 s, a measurer must have ahead of it
// before another starts half way along them: 30 s of sound decode in about
// the time an ffmpeg takes to start and seek.
const levelsShare = 3000

// audioFrom is how ffmpeg reads the episode's audio as 16 kHz mono samples
// from frame start on, for the loudness and for the transcription alike,
// see soundFrom.
func audioFrom(source string, start int, picture float64) []string {
	return soundFrom(source, float64(start)*timed.FrameSeconds, picture, SampleRate, 1)
}

// soundFrom is how ffmpeg reads the episode's sound from a moment on, in
// seconds, as float samples at rate in so many channels, for the
// transcription, the loudness and the video preview alike.
//
// The seek goes before the input, so ffmpeg seeks rather than decodes its
// way there. Where the first sample lands is not left to the seek: ffmpeg
// 8.1 kept the part of the first packet after the seek point, 9.0 drops
// that packet whole, up to 64 ms of 16 kHz AAC, and the samples carry no
// time of their own on the way out. Their timestamps stay right in both,
// so the seek goes soundLead early and atrim cuts at the timestamp of
// from, to the sample. That early part is also where the decoder is
// wrong, up to 4 dB off at the join, because a packet of compressed audio
// is decoded together with the one before it.
func soundFrom(source string, from, picture float64, rate, channels int) []string {
	args := []string{"-hide_banner", "-loglevel", "error", "-nostats"}
	seek, lead := soundSeek(from, picture)
	if seek > 0 {
		args = append(args, "-ss", strconv.FormatFloat(seek, 'f', 6, 64))
	}
	args = append(args, "-i", source, "-map", "0:a:0")
	if lead > 0 {
		args = append(args, "-af", "atrim=start="+strconv.FormatFloat(lead, 'f', 6, 64))
	}
	return append(args, "-ac", itoa(channels), "-ar", itoa(rate), "-f", "f32le", "-")
}

// soundSeek is where a reading of the sound from a moment seeks to, and
// how much of what it decodes from there it cuts away before from, see
// soundFrom: soundLead early, or from the start of the file where that
// would be before the picture starts. The episode's decoder reads the
// sound by the same rule, see EpisodeFrames.Sound.
func soundSeek(from, picture float64) (seek, lead float64) {
	lead = min(from, float64(soundLead)/1_000_000)
	// ffmpeg seeks every stream of a file to the key frame of its picture,
	// so a seek to before the picture's first frame hands back the sound
	// from where the picture starts, and a reading resumed there was heard
	// up to a tenth of a second out of step, see latesound_test.go. Sound from
	// before the picture is read from the start of the file instead, which
	// is never far, the way the render reads it, see leadOf.
	if from-lead < picture {
		lead = from
	}
	return max(0, from-lead), lead
}

// readLevels turns 16 kHz mono float samples into a frame every 10 ms and
// hands each to put, which says whether to go on. After every read it
// asks tick, which says the same, and due says whether levelsDefault has
// passed since it last said so, or levelsReads reads. It says whether it
// reached the end.
func (e *Engine) readLevels(ctx context.Context, r io.Reader, put func(float32) bool,
	tick func(due bool) bool) (bool, error) {
	frame := make([]float32, 0, frameSamples)
	samples := newSampleReader(r, frameSamples*4*200)
	last, reads := time.Now(), 0
	for {
		if ctx.Err() != nil {
			return false, ctx.Err()
		}
		data, err := samples.Read()
		for i := 0; i < len(data); i += 4 {
			frame = append(frame, math.Float32frombits(binary.LittleEndian.Uint32(data[i:])))
			if len(frame) == frameSamples {
				if !put(frameDB(frame)) {
					return false, nil
				}
				frame = frame[:0]
			}
		}
		if errors.Is(err, io.EOF) {
			// A last part shorter than a frame is measured as it is, so
			// the levels reach the end of the audio.
			if len(frame) > 0 {
				put(frameDB(frame))
			}
			return true, nil
		}
		if err != nil {
			return false, err
		}
		reads++
		due := time.Since(last) >= levelsDefault
		if e.levelsReads > 0 {
			due = reads%e.levelsReads == 0
		}
		if due {
			last = time.Now()
		}
		if !tick(due) {
			return false, nil
		}
	}
}

// writeLevels writes the frames measured since it last wrote, where they
// are in the frames file, and then the json that says which are measured,
// replaced in one step. A measuring of a long video writes what it has
// twice a second, and writing all of it each time rewrote megabytes for
// the few frames that were new.
//
// Neither is flushed to the drive. The loudness can be measured again,
// and a full flush, F_FULLFSYNC on macOS, is kept for what cannot, the
// plans and the transcripts, see fileio.Replace.
func writeLevels(fsys afero.Fs, logs string, stamp sourceStamp, st *levelState) error {
	meta, framesAt := levelsPaths(logs)
	parts := st.parts()
	reach := 0
	if len(parts) > 0 {
		reach = parts[len(parts)-1][1]
	}
	// A frames file that is gone or shorter than the parts written last,
	// removed while the measuring ran or cut off by a power cut, would
	// read as 0 dB in the hole, loud and never measured again. It is
	// written whole instead. New frames past its end are no hole: what
	// lies between is in no part, and is read as not measured.
	if info, err := fsys.Stat(framesAt); err != nil || info.Size() < int64(st.written)*4 {
		st.whole = true
	}
	runs := st.fresh
	flags := os.O_WRONLY | os.O_CREATE
	if st.whole {
		// The parts alone: frames between them are measured later, by
		// another measurer, and written then.
		runs = parts
		flags |= os.O_TRUNC
	}
	if len(runs) > 0 || st.whole {
		f, err := fsys.OpenFile(framesAt, flags, 0o644)
		if err != nil {
			return err
		}
		for _, r := range runs {
			from, to := r[0], min(r[1], reach)
			if to <= from {
				continue
			}
			raw := make([]byte, 4*(to-from))
			for i, frame := range st.frames[from:to] {
				binary.LittleEndian.PutUint32(raw[4*i:], math.Float32bits(frame))
			}
			if _, err = f.WriteAt(raw, int64(from)*4); err != nil {
				break
			}
		}
		if closeErr := f.Close(); err == nil {
			err = closeErr
		}
		if err != nil {
			return err
		}
	}
	st.fresh, st.whole, st.written = st.fresh[:0], false, reach
	file := levelsFile{Version: levelsVersion, Source: stamp, Parts: parts, Done: st.done()}
	if st.end >= 0 {
		file.End = st.end
	}
	if loudest, ok := st.loudest(); ok && file.Done {
		file.Loudest = &loudest
	}
	body, err := json.Marshal(file)
	if err != nil {
		return err
	}
	return fileio.WriteUnflushed(fsys, meta, body)
}

// levelSpots is how many places of a video its level is measured at, and
// levelSpot how long each is, in seconds, see videoLevel.
const (
	levelSpots = 30
	levelSpot  = 10.0
	// levelReaders is how many places are measured at once.
	levelReaders = 4
)

// videoLevel is the mean level of a video's sound, in dB, which the
// silence between its words is told from, see NoiseFloor. It is measured
// before the first word is heard, at levelSpots places spread evenly over
// the video, each levelSpot long, or over the whole of a video shorter
// than they add up to. That is a few seconds of decoding however long the
// video is.
//
// It is a property of the file alone, so the same video gets the same
// level on every machine, however its hearing is cut into parts and
// whenever those are saved. Taken from what was heard so far, it moved
// with every part, and with it where the words already heard start and
// end, the lines they make and the pauses the prompt marks, issue #762.
// Taken from the first part saved, it depended on how fast the machine
// heard: a slow one could save room noise alone first, and then no pause
// was found in that video at all.
func (e *Engine) videoLevel(ctx context.Context, source string, duration float64) (float64, error) {
	if _, ok := e.soundless(ctx, source); ok {
		return noSoundDB, nil
	}
	picture := e.pictureStart(ctx, source)
	spots := levelSpotsIn(duration)
	got := make([][]float32, len(spots))
	errs := make([]error, len(spots))
	// Each place is an ffmpeg of its own, which spends most of its time
	// starting and seeking, so a few run at once. The frames are joined in
	// the order of the places, so the mean is the same to the last bit
	// whichever ends first.
	free := make(chan struct{}, levelReaders)
	var wg sync.WaitGroup
	for i, spot := range spots {
		wg.Go(func() {
			free <- struct{}{}
			defer func() { <-free }()
			got[i], errs[i] = e.levelsAt(ctx, source, spot[0], spot[1], picture)
		})
	}
	wg.Wait()
	if err := errors.Join(errs...); err != nil {
		return 0, err
	}
	var frames []float32
	for _, g := range got {
		frames = append(frames, g...)
	}
	return num.RoundTo(meanOfFrames(frames), 3), nil
}

// levelSpotsIn is where a video of duration seconds has its level
// measured, as the frame each place starts at and how many frames it is:
// levelSpots places, each in the middle of its share of the video, or the
// whole of a video too short for them.
func levelSpotsIn(duration float64) [][2]int {
	if duration <= levelSpots*levelSpot {
		return [][2]int{{0, math.MaxInt}}
	}
	step := duration / levelSpots
	frames := int(math.Round(levelSpot / timed.FrameSeconds))
	spots := make([][2]int, levelSpots)
	for i := range spots {
		spots[i] = [2]int{int(math.Floor((float64(i)*step + (step-levelSpot)/2) / timed.FrameSeconds)), frames}
	}
	return spots
}

// levelsAt measures count loudness frames from frame start on, fewer
// where the audio ends first.
func (e *Engine) levelsAt(ctx context.Context, source string, start, count int, picture float64) ([]float32, error) {
	run, stop := context.WithCancel(ctx)
	defer stop()
	cmd := e.Runner.Command(run, e.FFmpeg, audioFrom(source, start, picture)...)
	var stderr strings.Builder
	said := tool.NewCutWriter(&stderr)
	cmd.Stderr = said
	reading, err := e.Runner.StartReading(run, cmd, said)
	if err != nil {
		return nil, renderErr("cannot start %s: %s", e.FFmpeg, err)
	}
	var frames []float32
	end, readErr := e.readLevels(run, bufio.NewReaderSize(reading.Stdout, 1<<20), func(f float32) bool {
		frames = append(frames, f)
		return len(frames) < count
	}, func(bool) bool { return true })
	if !end {
		// Enough heard: ffmpeg is still writing, and is told to stop
		// rather than left to write into a pipe nobody reads.
		stop()
	}
	left, waitErr := reading.Finish()
	if ctx.Err() != nil {
		return nil, ctx.Err()
	}
	if left {
		// Stopped once enough was read, so what was read stands.
		e.Log.Warn("ffmpeg %s", tool.LeftBehind)
	}
	if readErr != nil {
		return nil, readErr
	}
	if end && waitErr != nil {
		return nil, renderErr("ffmpeg could not read the audio: %s", strings.TrimSpace(stderr.String()))
	}
	return frames, nil
}
