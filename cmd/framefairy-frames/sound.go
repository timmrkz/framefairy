//go:build ffmpeglibs

package main

import (
	"errors"
	"fmt"
	"math"

	"github.com/asticode/go-astiav"

	"framefairy/internal/framewire"
)

// soundChunk is how many moments of sound a record holds, one sample of
// every channel each, engine.SoundChunk. The last of a stream may hold
// fewer.
const soundChunk = 1024

// openSound puts a cursor of sound where a reading of the sound from from
// on begins, the way the ffmpeg program reads it for the render and the
// transcription, see engine.soundFrom: seeked to seek, or read from the
// file's start where seek is 0, and cut at from, to the sample, by the
// samples' own timestamps. Both are counted from the file's start, as the
// ffmpeg program counts them. Its samples come as 32-bit floats at rate in
// so many channels, the channels of one moment side by side.
func (c *cursor) openSound(seek, from float64, rate, channels int) {
	c.failedWhy = ""
	opened := ""
	// From the file's start, the ffmpeg program does not seek at all, and
	// a seek there loses what the file says to skip of its first packet,
	// the 1024 samples an AAC encoder puts before the sound. A cursor that
	// has read on opens the file again instead.
	if seek == 0 && c.v != nil && c.read {
		c.freeGraph()
		c.disown(&c.keys)
		c.v.close()
		c.v = nil
	}
	if c.v == nil {
		opened = "file"
		v, err := openMedia(c.d.path, astiav.MediaTypeAudio)
		if err == nil {
			err = v.openDecoder(nil, c.say)
			if err != nil {
				v.close()
			}
		}
		if err != nil {
			c.fail(err.Error())
			return
		}
		c.v = v
		c.read = false
		c.own(&c.keys, v.fc, v.dec)
		if c.pkt == nil {
			c.pkt = astiav.AllocPacket()
			c.decoded = astiav.AllocFrame()
			c.filtered = astiav.AllocFrame()
		}
	}
	// The ffmpeg program's -ss seeks every stream to the key frame of the
	// picture before it, from the file's start, and reads from the start
	// where there is no -ss.
	start := c.v.fc.StartTime()
	if start == astiav.NoPtsValue {
		start = 0
	}
	if seek > 0 {
		if err := c.v.fc.SeekFrame(-1, start+int64(math.Round(seek*1e6)), astiav.NewSeekFlags(astiav.SeekFlagBackward)); err != nil {
			c.fail(fmt.Sprintf("the sound cannot be read from %.3f s: %s", seek, err))
			return
		}
		flushDecoder(c.v.dec)
		c.read = true
	}
	// The cut is in the chain, so the chain is made for every open.
	c.freeGraph()
	c.from = from + float64(start)/1e6
	c.rate, c.channels = rate, channels
	c.pcm, c.chunks, c.soundEnded = c.pcm[:0], 0, false
	c.sentEOF, c.graphEOF = false, false
	// The moment the first chunk starts at, on the clock the ffmpeg
	// program reads by.
	c.soundFrom = from
	how := "moved"
	if opened != "" {
		how = "opened the file"
	}
	c.say(fmt.Sprintf("%s, from %.3f s, seek %.3f s, at %d Hz, %d channels", how, from, seek, rate, channels))
	c.d.answer(framewire.Record{Cursor: c.id, Kind: framewire.Opened, Body: []byte(opened)})
}

// nextSound answers up to n chunks of sound from where the cursor is.
func (c *cursor) nextSound(n int) {
	if c.failedWhy != "" || c.v == nil {
		if c.failedWhy == "" {
			c.fail("a next before the cursor was opened")
		} else {
			c.fail(c.failedWhy)
		}
		return
	}
	size := soundChunk * c.channels * 4
	for sent := 0; sent < n; {
		if len(c.pcm) >= size || (c.soundEnded && len(c.pcm) > 0) {
			k := min(size, len(c.pcm))
			k -= k % (c.channels * 4)
			if k == 0 {
				c.pcm = c.pcm[:0]
				continue
			}
			body := make([]byte, k)
			copy(body, c.pcm[:k])
			c.pcm = append(c.pcm[:0], c.pcm[k:]...)
			at := c.soundFrom + float64(c.chunks*soundChunk)/float64(c.rate)
			c.chunks++
			c.d.answer(framewire.Record{Cursor: c.id, Kind: framewire.Frame, At: at, Body: body})
			sent++
			continue
		}
		if c.soundEnded {
			c.d.answer(framewire.Record{Cursor: c.id, Kind: framewire.End})
			return
		}
		c.read = true
		data, err := c.soundFrame()
		if errors.Is(err, astiav.ErrEof) {
			c.soundEnded = true
			continue
		}
		if err != nil {
			c.fail(err.Error())
			return
		}
		c.pcm = append(c.pcm, data...)
	}
	c.d.answer(framewire.Record{Cursor: c.id, Kind: framewire.Done})
}

// soundFrame is the next stretch of samples out of the chain.
func (c *cursor) soundFrame() ([]byte, error) {
	for {
		if c.graph != nil {
			err := c.sink.GetFrame(c.filtered, astiav.NewBuffersinkFlags())
			if err == nil {
				body, berr := c.filtered.Data().Bytes(1)
				c.filtered.Unref()
				if berr != nil {
					return nil, fmt.Errorf("the sound could not be read out: %w", berr)
				}
				return body, nil
			}
			if errors.Is(err, astiav.ErrEof) {
				return nil, astiav.ErrEof
			}
			if !errors.Is(err, astiav.ErrEagain) {
				return nil, c.withSaid(fmt.Errorf("the sound could not be converted: %w", err))
			}
		}
		err := c.v.dec.ReceiveFrame(c.decoded)
		switch {
		case err == nil:
			if pts := bestEffort(c.decoded); pts != astiav.NoPtsValue {
				c.decoded.SetPts(pts)
			}
			if c.graph == nil {
				if err := c.makeSoundGraph(); err != nil {
					c.decoded.Unref()
					return nil, err
				}
			}
			err := c.src.AddFrame(c.decoded, astiav.NewBuffersrcFlags())
			c.decoded.Unref()
			if err != nil {
				return nil, c.withSaid(fmt.Errorf("the sound could not be converted: %w", err))
			}
		case errors.Is(err, astiav.ErrEof):
			if c.graph == nil {
				return nil, astiav.ErrEof
			}
			if !c.graphEOF {
				c.graphEOF = true
				if err := c.src.AddFrame(nil, astiav.NewBuffersrcFlags()); err != nil {
					return nil, astiav.ErrEof
				}
			}
		case errors.Is(err, astiav.ErrEagain):
			if err := c.feed(); err != nil {
				return nil, err
			}
		default:
			return nil, c.withSaid(fmt.Errorf("the sound could not be decoded: %w", err))
		}
	}
}

// makeSoundGraph builds the chain for the sound: cut at from by the
// samples' timestamps, before anything else, then made 32-bit floats at
// the rate and in the channels asked for, the way the ffmpeg program's
// atrim, -ar and -ac do it.
func (c *cursor) makeSoundGraph() error {
	return c.buildGraph(fmt.Sprintf("atrim=start=%.6f,aformat=sample_fmts=flt:sample_rates=%d:channel_layouts=%s",
		c.from, c.rate, defaultLayout(c.channels)))
}
