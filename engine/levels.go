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
	"strings"
	"time"
)

// ---------------------------------------------------------------------------
// The loudness of the whole episode
//
// The waveform used to come from the transcription, which measures the
// loudness as it hears. The transcription stops at the end of the window
// the first search needs, so the rest of the clip timeline stayed bare
// until a search reached it, and the first minutes stayed bare until the
// speech model had loaded.
//
// Measuring the loudness needs no speech model. It is the audio decoded and
// a sum of squares every 10 ms, the same frames the transcription measures,
// from the same 16 kHz mono samples. Audio decodes at a few hundred times
// real time, so a whole episode is measured in seconds, and the waveform
// fills in from the start as it goes.
// ---------------------------------------------------------------------------

// levelsVersion changes whenever what the files hold changes meaning.
const levelsVersion = 1

// levelsEvery is how often the frames measured so far are written, so the
// waveform grows while the measuring runs.
const levelsEvery = 500 * time.Millisecond

// levelsFile is what logs/levels.json holds. The frames are in
// logs/levels.frames, written before it, so a reader never sees a length
// the frames do not reach.
type levelsFile struct {
	Version int         `json:"version"`
	Source  sourceStamp `json:"source"`
	// To is how far the frames reach, in seconds.
	To PyFloat `json:"to"`
	// Done says the frames reach the end of the audio.
	Done bool `json:"done"`
}

func levelsPaths(logsDir string) (string, string) {
	return filepath.Join(logsDir, "levels.json"), filepath.Join(logsDir, "levels.frames")
}

// Levels is the loudness of an episode as far as it has been measured.
type Levels struct {
	// Frames are dB every FrameSeconds from the start of the episode.
	Frames []float32
	// Done says they reach the end of the audio.
	Done bool
}

// To is how far the levels reach, in seconds.
func (l Levels) To() float64 { return float64(len(l.Frames)) * FrameSeconds }

// ReadLevels reads what has been measured of an episode's loudness. An
// episode never measured, or measured before its file changed, has none.
func ReadLevels(source string) Levels {
	stamp, err := stampOf(source)
	if err != nil {
		return Levels{}
	}
	meta, framesAt := levelsPaths(filepath.Join(WorkDir(source), "logs"))
	var file levelsFile
	data, err := os.ReadFile(meta)
	if err != nil || decodeJSON(data, &file) != nil ||
		file.Version != levelsVersion || file.Source != stamp {
		return Levels{}
	}
	raw, err := os.ReadFile(framesAt)
	if err != nil || len(raw)%4 != 0 {
		return Levels{}
	}
	frames := make([]float32, len(raw)/4)
	if binary.Read(bytes.NewReader(raw), binary.LittleEndian, frames) != nil {
		return Levels{}
	}
	// The frames may run ahead of the length the json gives, never behind:
	// they are written first. What the json gives is what is certain.
	want := int(math.Round(float64(file.To) / FrameSeconds))
	if want > len(frames) {
		return Levels{}
	}
	for i, f := range frames[:want] {
		if math.IsNaN(float64(f)) || math.IsInf(float64(f), 0) {
			frames[i] = -90
		}
	}
	return Levels{Frames: frames[:want], Done: file.Done}
}

// MeasureLevels measures the loudness of the whole episode, writing what it
// has every half second, and tells progress how far it has come. An
// episode already measured is left as it is. It starts from the beginning
// when it was cut off, since the whole of it takes seconds.
func (e *Engine) MeasureLevels(ctx context.Context, source string, progress func(to float64)) error {
	if ReadLevels(source).Done {
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
	args := []string{"-hide_banner", "-loglevel", "error", "-nostats", "-i", source,
		"-map", "0:a:0", "-ac", "1", "-ar", itoa(SampleRate), "-f", "f32le", "-"}
	cmd := exec.CommandContext(ctx, e.FFmpeg, args...)
	stdout, err := cmd.StdoutPipe()
	if err != nil {
		return err
	}
	var stderr strings.Builder
	cmd.Stderr = &stderr
	if err := cmd.Start(); err != nil {
		return renderErr("cannot start %s: %s", e.FFmpeg, err)
	}
	frames, readErr := e.readLevels(ctx, bufio.NewReaderSize(stdout, 1<<20), func(frames []float32) {
		if err := writeLevels(logs, stamp, frames, false); err == nil && progress != nil {
			progress(float64(len(frames)) * FrameSeconds)
		}
	})
	waitErr := cmd.Wait()
	if ctx.Err() != nil {
		return ctx.Err()
	}
	if readErr != nil {
		return readErr
	}
	if waitErr != nil {
		return renderErr("ffmpeg could not read the audio: %s", strings.TrimSpace(stderr.String()))
	}
	if err := writeLevels(logs, stamp, frames, true); err != nil {
		return err
	}
	if progress != nil {
		progress(float64(len(frames)) * FrameSeconds)
	}
	return nil
}

// readLevels turns 16 kHz mono float samples into a frame every 10 ms,
// handing what it has to keep every levelsEvery.
func (e *Engine) readLevels(ctx context.Context, r io.Reader, keep func([]float32)) ([]float32, error) {
	var frames []float32
	frame := make([]float32, 0, frameSamples)
	buf := make([]byte, frameSamples*4*200)
	carry := []byte{}
	last := time.Now()
	for {
		if ctx.Err() != nil {
			return frames, ctx.Err()
		}
		n, err := r.Read(buf)
		data := append(carry, buf[:n]...)
		whole := len(data) / 4 * 4
		for i := 0; i < whole; i += 4 {
			frame = append(frame, math.Float32frombits(binary.LittleEndian.Uint32(data[i:])))
			if len(frame) == frameSamples {
				frames = append(frames, frameDB(frame))
				frame = frame[:0]
			}
		}
		carry = append([]byte{}, data[whole:]...)
		if time.Since(last) >= levelsEvery {
			last = time.Now()
			keep(frames)
		}
		if errors.Is(err, io.EOF) {
			// A last part shorter than a frame is measured as it is, so
			// the levels reach the end of the audio.
			if len(frame) > 0 {
				frames = append(frames, frameDB(frame))
			}
			return frames, nil
		}
		if err != nil {
			return frames, err
		}
	}
}

// writeLevels writes the frames and then the json that says how far they
// reach, each replaced in one step.
func writeLevels(logs string, stamp sourceStamp, frames []float32, done bool) error {
	meta, framesAt := levelsPaths(logs)
	var raw bytes.Buffer
	if err := binary.Write(&raw, binary.LittleEndian, frames); err != nil {
		return err
	}
	if err := writeAtomic(framesAt, raw.Bytes()); err != nil {
		return err
	}
	body, err := json.Marshal(levelsFile{Version: levelsVersion, Source: stamp,
		To: PyFloat(roundTo(float64(len(frames))*FrameSeconds, 2)), Done: done})
	if err != nil {
		return err
	}
	return writeAtomic(meta, body)
}
