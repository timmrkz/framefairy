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
	"os/exec"
	"path/filepath"
	"strconv"
	"strings"
	"time"
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
// asks where the clip timeline is looking, unless the engine says
// otherwise, see Engine.levelsEvery.
const levelsDefault = 500 * time.Millisecond

// levelsStep is how often this engine's measuring writes and asks.
func (e *Engine) levelsStep() time.Duration {
	if e.levelsEvery > 0 {
		return e.levelsEvery
	}
	return levelsDefault
}

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
func (l Levels) To() float64 { return float64(len(l.Frames)) * FrameSeconds }

// Seconds are the parts measured, in seconds.
func (l Levels) Seconds() [][2]float64 { return partSeconds(l.Parts) }

func partSeconds(parts [][2]int) [][2]float64 {
	out := make([][2]float64, len(parts))
	for i, p := range parts {
		out[i] = [2]float64{roundTo(float64(p[0])*FrameSeconds, 2), roundTo(float64(p[1])*FrameSeconds, 2)}
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
	at := int(math.Round(t.Start / FrameSeconds))
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

// LevelsFiles are the files the loudness of an episode is read from.
func LevelsFiles(source string) []string {
	meta, frames := levelsPaths(filepath.Join(WorkDir(source), "logs"))
	return []string{meta, frames}
}

// readLevelsFile reads what logs/levels.json says, when it is about the
// file as it is now and its parts are in order.
func readLevelsFile(source string) (levelsFile, sourceStamp, bool) {
	stamp, err := stampOf(source)
	if err != nil {
		return levelsFile{}, stamp, false
	}
	meta, _ := levelsPaths(filepath.Join(WorkDir(source), "logs"))
	var file levelsFile
	data, err := os.ReadFile(meta)
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
func LevelsReach(source string) ([][2]float64, bool) {
	file, _, ok := readLevelsFile(source)
	if !ok {
		return nil, false
	}
	return partSeconds(file.Parts), file.Done
}

// ReadLevels reads what has been measured of an episode's loudness. An
// episode never measured, or measured before its file changed, has none.
func ReadLevels(source string) Levels {
	file, _, ok := readLevelsFile(source)
	if !ok || len(file.Parts) == 0 {
		return Levels{}
	}
	_, framesAt := levelsPaths(filepath.Join(WorkDir(source), "logs"))
	raw, err := os.ReadFile(framesAt)
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
}

func (s *levelState) put(n int, f float32) {
	for len(s.frames) <= n {
		s.frames = append(s.frames, -90)
		s.got = append(s.got, false)
	}
	s.frames[n], s.got[n] = f, true
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
	first := int(math.Floor(from / FrameSeconds))
	last := int(math.Ceil(to / FrameSeconds))
	if last > first {
		if n, ok := s.gap(first, math.MaxInt); ok {
			return n, true
		}
	}
	return s.gap(0, math.MaxInt)
}

// shown says whether a frame is in what the clip timeline shows.
func shown(n int, from, to float64) bool {
	return to > from && float64(n)*FrameSeconds >= from-FrameSeconds && float64(n)*FrameSeconds < to
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
	return float64(total) * FrameSeconds
}

func (s *levelState) done() bool {
	_, left := s.gap(0, math.MaxInt)
	return s.end >= 0 && !left
}

// MeasureLevels measures the loudness of the whole episode, writing what it
// has every half second, and tells progress how many seconds of it are
// measured. It measures what focus says the clip timeline shows first, and
// follows it when it moves. A focus that is nil, or shows nothing, has it
// measure from the start. An episode measured already is left as it is,
// and one measured in part carries on.
func (e *Engine) MeasureLevels(ctx context.Context, source string, focus func() (from, to float64),
	progress func(measured float64)) error {
	if focus == nil {
		focus = func() (float64, float64) { return 0, 0 }
	}
	had := ReadLevels(source)
	if had.Done {
		return nil
	}
	stamp, err := stampOf(source)
	if err != nil {
		return err
	}
	logs := filepath.Join(WorkDir(source), "logs")
	if err := os.MkdirAll(logs, 0o755); err != nil {
		return err
	}
	st := &levelState{end: -1}
	if file, _, ok := readLevelsFile(source); ok && file.End > 0 {
		st.end = file.End
	}
	for _, p := range had.Parts {
		for n := p[0]; n < p[1]; n++ {
			st.put(n, had.Frames[n])
		}
	}
	tell := func() {
		if err := writeLevels(logs, stamp, st); err == nil && progress != nil {
			progress(st.measured())
		}
	}
	for {
		start, ok := st.next(focus())
		if !ok {
			break
		}
		if err := e.measureFrom(ctx, source, st, start, focus, tell); err != nil {
			return err
		}
		// A part that ended between two ticks is shown when it ends, not
		// at the first tick of the next one. A short part is read in less
		// than a tick, and then where the clip timeline looked went unseen
		// until the measuring had moved on.
		tell()
	}
	if err := writeLevels(logs, stamp, st); err != nil {
		return err
	}
	if progress != nil {
		progress(st.measured())
	}
	return nil
}

// measureFrom measures from frame start on, until it reaches a frame
// measured already, the end of the audio, or a clip timeline that moved to
// a part not measured yet.
func (e *Engine) measureFrom(ctx context.Context, source string, st *levelState, start int,
	focus func() (float64, float64), tell func()) error {
	run, stop := context.WithCancel(ctx)
	defer stop()
	cmd := exec.CommandContext(run, e.FFmpeg, audioFrom(source, start)...)
	stdout, err := cmd.StdoutPipe()
	if err != nil {
		return err
	}
	var stderr strings.Builder
	cmd.Stderr = &stderr
	if err := cmd.Start(); err != nil {
		return renderErr("cannot start %s: %s", e.FFmpeg, err)
	}
	n := start
	moved := false
	end, readErr := e.readLevels(run, bufio.NewReaderSize(stdout, 1<<20), func(f float32) bool {
		if st.has(n) {
			return false
		}
		st.put(n, f)
		n++
		return true
	}, func() bool {
		tell()
		if st.leave(n, focus) {
			moved = true
			return false
		}
		return true
	})
	if !end {
		// Stopped before the end: ffmpeg is still writing, and is told to
		// stop rather than left to write into a pipe nobody reads.
		stop()
	}
	waitErr := cmd.Wait()
	if ctx.Err() != nil {
		return ctx.Err()
	}
	if readErr != nil && !errors.Is(readErr, context.Canceled) {
		return readErr
	}
	switch {
	case end && n <= start && start > 0:
		// Nothing past start: the audio ends before it.
		st.end = start
		st.frames, st.got = st.frames[:min(start, len(st.frames))], st.got[:min(start, len(st.got))]
	case end && waitErr != nil:
		return renderErr("ffmpeg could not read the audio: %s", strings.TrimSpace(stderr.String()))
	case end:
		st.end = n
		st.frames, st.got = st.frames[:min(n, len(st.frames))], st.got[:min(n, len(st.got))]
	case moved:
		e.Log.Detail("the clip timeline moved on, so the loudness is measured there next")
	}
	return nil
}

// levelsLead is how many frames before where it starts a reading decodes
// and throws away, soundLead in frames. The conversion only compiles while
// soundLead is a whole number of frames.
const levelsLead = int(soundLead / (FrameSeconds * 1_000_000))

// audioFrom is how ffmpeg reads the episode's audio as 16 kHz mono samples
// from frame start on, for the loudness and for the transcription alike.
//
// The seek goes before the input, so ffmpeg seeks rather than decodes its
// way there. Where the first sample lands is not left to the seek: ffmpeg
// 8.1 kept the part of the first packet after the seek point, 9.0 drops
// that packet whole, up to 64 ms of 16 kHz AAC, and the samples carry no
// time of their own on the way out. Their timestamps stay right in both,
// so the seek goes levelsLead early and atrim cuts at the timestamp of
// start, to the sample. That early part is also where the decoder is
// wrong, up to 4 dB off at the join, because a packet of compressed audio
// is decoded together with the one before it. A frame is 10 ms, so two
// decimals are exact.
func audioFrom(source string, start int) []string {
	args := []string{"-hide_banner", "-loglevel", "error", "-nostats"}
	lead := min(start, levelsLead)
	if start > 0 {
		args = append(args, "-ss", strconv.FormatFloat(float64(start-lead)*FrameSeconds, 'f', 2, 64))
	}
	args = append(args, "-i", source, "-map", "0:a:0")
	if lead > 0 {
		args = append(args, "-af", "atrim=start="+strconv.FormatFloat(float64(lead)*FrameSeconds, 'f', 2, 64))
	}
	return append(args, "-ac", "1", "-ar", itoa(SampleRate), "-f", "f32le", "-")
}

// readLevels turns 16 kHz mono float samples into a frame every 10 ms and
// hands each to put, which says whether to go on. Every levelsEvery it asks
// tick, which says the same. It says whether it reached the end.
func (e *Engine) readLevels(ctx context.Context, r io.Reader, put func(float32) bool,
	tick func() bool) (bool, error) {
	frame := make([]float32, 0, frameSamples)
	buf := make([]byte, frameSamples*4*200)
	carry := []byte{}
	last := time.Now()
	for {
		if ctx.Err() != nil {
			return false, ctx.Err()
		}
		n, err := r.Read(buf)
		data := append(carry, buf[:n]...)
		whole := len(data) / 4 * 4
		for i := 0; i < whole; i += 4 {
			frame = append(frame, math.Float32frombits(binary.LittleEndian.Uint32(data[i:])))
			if len(frame) == frameSamples {
				if !put(frameDB(frame)) {
					return false, nil
				}
				frame = frame[:0]
			}
		}
		carry = append([]byte{}, data[whole:]...)
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
		if time.Since(last) >= e.levelsStep() {
			last = time.Now()
			if !tick() {
				return false, nil
			}
		}
	}
}

// writeLevels writes the frames and then the json that says which are
// measured, each replaced in one step.
func writeLevels(logs string, stamp sourceStamp, st *levelState) error {
	meta, framesAt := levelsPaths(logs)
	parts := st.parts()
	reach := 0
	if len(parts) > 0 {
		reach = parts[len(parts)-1][1]
	}
	var raw bytes.Buffer
	if err := binary.Write(&raw, binary.LittleEndian, st.frames[:reach]); err != nil {
		return err
	}
	if err := writeAtomic(framesAt, raw.Bytes()); err != nil {
		return err
	}
	file := levelsFile{Version: levelsVersion, Source: stamp, Parts: parts, Done: st.done()}
	if st.end >= 0 {
		file.End = st.end
	}
	body, err := json.Marshal(file)
	if err != nil {
		return err
	}
	return writeAtomic(meta, body)
}
