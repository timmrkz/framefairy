package engine

import (
	"bytes"
	"context"
	"errors"
	"fmt"
	"io"
	"math"
	"os/exec"
)

// SoundChunk is how many sample frames, one sample of every channel each,
// PreviewSound hands over at a time. The last of a stream may be shorter.
const SoundChunk = 1024

// PreviewSound decodes the sound of an episode for the video preview, from
// a moment on, as 32-bit float samples at rate, the channels of one moment
// side by side, little endian. It is read the way the transcription reads
// it, see soundFrom, so the video preview hears what ffmpeg decodes, the
// same sound the render and the transcript are made of, whatever decoder
// the webview has: the Mac's own decoder of AAC had its own idea of where
// the sound of a file starts.
//
// Every chunk is SoundChunk sample frames and says the moment of the
// episode its first one is at. It runs until got says no more, the
// context ends or the episode does, and a slow reader slows ffmpeg down.
func (e *Engine) PreviewSound(ctx context.Context, path string, from float64, rate, channels int,
	got func(at float64, chunk []byte) error) error {
	if e.FFmpeg == "" {
		return errors.New("there is no ffmpeg to decode the sound with")
	}
	if rate < 8000 || rate > 192000 || channels < 1 || channels > 8 {
		return fmt.Errorf("sound at %d Hz in %d channels cannot be made", rate, channels)
	}
	if math.IsNaN(from) || math.IsInf(from, 0) || from < 0 {
		from = 0
	}
	ctx, cancel := context.WithCancel(ctx)
	defer cancel()
	cmd := exec.CommandContext(ctx, e.FFmpeg, soundFrom(path, from, e.pictureStart(ctx, path), rate, channels)...)
	out, err := cmd.StdoutPipe()
	if err != nil {
		return err
	}
	var said bytes.Buffer
	cmd.Stderr = &said
	if err := cmd.Start(); err != nil {
		return err
	}
	chunk := make([]byte, SoundChunk*channels*4)
	var failed error
	for n := 0; ; n++ {
		k, err := io.ReadFull(out, chunk)
		// A last chunk that is not whole is handed over as far as it has
		// whole moments.
		if k -= k % (channels * 4); k > 0 {
			if err := got(from+float64(n*SoundChunk)/float64(rate), chunk[:k]); err != nil {
				failed = err
				break
			}
		}
		if err != nil {
			if !errors.Is(err, io.EOF) && !errors.Is(err, io.ErrUnexpectedEOF) {
				failed = err
			}
			break
		}
	}
	cancel()
	_, _ = io.Copy(io.Discard, out)
	waited := cmd.Wait()
	if failed != nil {
		return failed
	}
	if waited != nil && ctx.Err() == nil {
		return fmt.Errorf("ffmpeg could not decode the sound: %s", Scrub(said.String(), 400))
	}
	return nil
}
