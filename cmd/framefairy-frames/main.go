//go:build ffmpeglibs

// Command framefairy-frames is the episode's decoder: one program per open
// episode, which keeps the file open, its index read and decoders ready,
// so a jump in the video preview is a request to it and not a program
// that has to start first. It decodes with ffmpeg's own libraries, the
// code the ffmpeg program runs, so its frames are the frames the render
// takes. It is a program of its own, so a crash ends only it. See
// docs/VIDEO-PREVIEW.md, The episode's decoder.
//
//	framefairy-frames <episode>
//
// It reads requests on standard input and answers on standard output, see
// internal/framewire. Each cursor works on a goroutine of its own, so a
// cursor that decodes from far back does not hold up one that plays.
//
// It is built only against the ffmpeg make builds, with the build tag
// ffmpeglibs. Without it the program says so and stops, see none.go.
package main

import (
	"bufio"
	"errors"
	"fmt"
	"math"
	"os"
	"runtime"
	"strconv"
	"strings"
	"sync"
	"sync/atomic"

	"github.com/asticode/go-astiav"

	"framefairy/internal/framewire"
)

func main() {
	if len(os.Args) != 2 {
		fmt.Fprintln(os.Stderr, "usage: framefairy-frames <episode>")
		os.Exit(2)
	}
	astiav.SetLogLevel(astiav.LogLevelError)
	// What ffmpeg says when something fails is kept, so a failure says why
	// in ffmpeg's own words and not only in an error number.
	astiav.SetLogCallback(func(c astiav.Classer, l astiav.LogLevel, format, msg string) {
		said := strings.TrimSpace(msg)
		if c != nil {
			if cl := c.Class(); cl != nil {
				said = cl.Name() + ": " + said
			}
		}
		lastSaid.Store(said)
		fmt.Fprintln(os.Stderr, said)
	})
	d := &decoder{path: os.Args[1], out: bufio.NewWriterSize(os.Stdout, 1<<20), cursors: map[uint32]*cursor{}}
	// The episode is opened once here, so a file that cannot be read says so
	// before anything is asked of it.
	probe, err := openVideo(d.path)
	if err != nil {
		fmt.Fprintln(os.Stderr, err)
		os.Exit(1)
	}
	probe.close()
	if runtime.GOOS == "darwin" {
		if t := astiav.FindHardwareDeviceTypeByName("videotoolbox"); t != astiav.HardwareDeviceTypeNone {
			d.hardware, _ = astiav.CreateHardwareDeviceContext(t, "", nil, 0)
		}
	}
	lines := bufio.NewScanner(os.Stdin)
	for lines.Scan() {
		d.request(strings.Fields(lines.Text()))
	}
	d.closeAll()
}

// lastSaid is the last thing ffmpeg said at the level of an error.
var lastSaid atomic.Value

// withSaid is err with what ffmpeg last said, where it said anything.
func withSaid(err error) error {
	if s, _ := lastSaid.Load().(string); s != "" {
		return fmt.Errorf("%w (ffmpeg: %s)", err, s)
	}
	return err
}

type decoder struct {
	path     string
	hardware *astiav.HardwareDeviceContext
	// Set once scaling on the graphics chip has failed or given nothing,
	// so every cursor scales on the processor from then on.
	noChipScale atomic.Bool
	mu          sync.Mutex
	out         *bufio.Writer
	cursors     map[uint32]*cursor
}

// answer writes a record and hands it on at once, under the lock, so the
// records of two cursors never interleave.
func (d *decoder) answer(r framewire.Record) {
	d.mu.Lock()
	defer d.mu.Unlock()
	if framewire.Write(d.out, r) == nil {
		_ = d.out.Flush()
	}
}

func (d *decoder) request(f []string) {
	if len(f) < 2 {
		return
	}
	id64, err := strconv.ParseUint(f[1], 10, 32)
	if err != nil {
		return
	}
	id := uint32(id64)
	switch {
	case f[0] == "open" && len(f) == 5:
		from, err1 := strconv.ParseFloat(f[2], 64)
		w, err2 := strconv.Atoi(f[3])
		h, err3 := strconv.Atoi(f[4])
		if err1 != nil || err2 != nil || err3 != nil || math.IsNaN(from) || math.IsInf(from, 0) ||
			w < 2 || h < 2 || w > 7680 || h > 4320 || w%2 != 0 || h%2 != 0 {
			d.answer(framewire.Record{Cursor: id, Kind: framewire.Failed, Body: []byte("an open the decoder cannot take")})
			return
		}
		c := d.cursors[id]
		if c == nil {
			c = &cursor{d: d, id: id, work: make(chan func(), 4)}
			d.cursors[id] = c
			go c.run()
		}
		c.work <- func() { c.open(max(0, from), w, h) }
	case f[0] == "next" && len(f) == 4:
		n, err1 := strconv.Atoi(f[2])
		skip, err2 := strconv.ParseFloat(f[3], 64)
		c := d.cursors[id]
		if c == nil || err1 != nil || err2 != nil || n < 1 || math.IsNaN(skip) {
			d.answer(framewire.Record{Cursor: id, Kind: framewire.Failed, Body: []byte("a next for no open cursor")})
			return
		}
		c.work <- func() { c.next(min(n, 16), skip) }
	case f[0] == "close":
		if c := d.cursors[id]; c != nil {
			delete(d.cursors, id)
			close(c.work)
		}
	}
}

func (d *decoder) closeAll() {
	for id, c := range d.cursors {
		delete(d.cursors, id)
		close(c.work)
	}
}

// video is the episode's picture opened for reading: the file, its first
// picture stream and a decoder for it.
type video struct {
	fc     *astiav.FormatContext
	stream *astiav.Stream
	dec    *astiav.CodecContext
}

func openVideo(path string) (*video, error) {
	fc := astiav.AllocFormatContext()
	if fc == nil {
		return nil, errors.New("no memory to open the episode")
	}
	if err := fc.OpenInput(path, nil, nil); err != nil {
		fc.Free()
		return nil, fmt.Errorf("the episode cannot be opened: %w", err)
	}
	v := &video{fc: fc}
	if err := fc.FindStreamInfo(nil); err != nil {
		v.close()
		return nil, fmt.Errorf("the episode cannot be read: %w", err)
	}
	for _, s := range fc.Streams() {
		if s.CodecParameters().MediaType() == astiav.MediaTypeVideo {
			v.stream = s
			break
		}
	}
	if v.stream == nil {
		v.close()
		return nil, errors.New("the episode has no picture")
	}
	return v, nil
}

func (v *video) close() {
	if v.dec != nil {
		v.dec.Free()
	}
	v.fc.CloseInput()
	v.fc.Free()
}

// openDecoder opens a decoder for the picture, on the graphics chip where
// the system has one and it takes the file.
func (v *video) openDecoder(hardware *astiav.HardwareDeviceContext) error {
	codec := astiav.FindDecoder(v.stream.CodecParameters().CodecID())
	if codec == nil {
		return errors.New("there is no decoder for this picture")
	}
	dec := astiav.AllocCodecContext(codec)
	if dec == nil {
		return errors.New("no memory for a decoder")
	}
	if err := v.stream.CodecParameters().ToCodecContext(dec); err != nil {
		dec.Free()
		return fmt.Errorf("the decoder cannot take this picture: %w", err)
	}
	if hardware != nil {
		dec.SetHardwareDeviceContext(hardware)
		dec.SetPixelFormatCallback(func(pfs []astiav.PixelFormat) astiav.PixelFormat {
			for _, pf := range pfs {
				if pf == astiav.PixelFormatVideotoolbox {
					return pf
				}
			}
			// The chip does not take it: the first the processor can.
			return pfs[0]
		})
	} else {
		dec.SetThreadCount(0)
	}
	if err := dec.Open(codec, nil); err != nil {
		dec.Free()
		return fmt.Errorf("the decoder could not be opened: %w", err)
	}
	v.dec = dec
	return nil
}

// cursor is one place in the episode with a decoder of its own. Its work,
// an open or a next, runs on its own goroutine, one after another.
type cursor struct {
	d    *decoder
	id   uint32
	work chan func()

	v        *video
	pkt      *astiav.Packet
	decoded  *astiav.Frame
	filtered *astiav.Frame
	// The chain that makes a decoded frame the size of the canvas, 8-bit
	// and in full range, the same as engine.PreviewFrames asks of ffmpeg.
	graph    *astiav.FilterGraph
	src      *astiav.BuffersrcFilterContext
	sink     *astiav.BuffersinkFilterContext
	graphGPU bool
	width    int
	height   int
	from     float64
	sentEOF  bool
	graphEOF bool
	// Frames into the chain since it was built, and frames out of it.
	fed, got  int
	ended     bool
	failedWhy string
}

func (c *cursor) run() {
	for job := range c.work {
		job()
	}
	c.free()
}

func (c *cursor) free() {
	c.freeGraph()
	for _, f := range []*astiav.Frame{c.decoded, c.filtered} {
		if f != nil {
			f.Free()
		}
	}
	if c.pkt != nil {
		c.pkt.Free()
	}
	if c.v != nil {
		c.v.close()
	}
}

func (c *cursor) freeGraph() {
	if c.graph != nil {
		c.graph.Free()
		c.graph, c.src, c.sink = nil, nil, nil
	}
}

func (c *cursor) fail(why string) {
	c.failedWhy = why
	c.d.answer(framewire.Record{Cursor: c.id, Kind: framewire.Failed, Body: []byte(why)})
}

// open moves the cursor to from: to the key frame before it, with what the
// decoder held dropped. The file and the decoder stay open, which is the
// whole point: the first open of a cursor opens them, every one after it
// only seeks.
func (c *cursor) open(from float64, w, h int) {
	c.failedWhy = ""
	if c.v == nil {
		v, err := openVideo(c.d.path)
		if err == nil {
			err = v.openDecoder(c.d.hardware)
			if err != nil {
				v.close()
			}
		}
		if err != nil {
			c.fail(err.Error())
			return
		}
		c.v = v
		c.pkt = astiav.AllocPacket()
		c.decoded = astiav.AllocFrame()
		c.filtered = astiav.AllocFrame()
	}
	if err := c.seek(from); err != nil {
		c.fail(err.Error())
		return
	}
	// The chain holds no frames between two, so it is kept unless the size
	// changed or it was told the episode had ended.
	if c.graph != nil && (c.width != w || c.height != h || c.graphEOF) {
		c.freeGraph()
	}
	c.width, c.height, c.from = w, h, from
	c.sentEOF, c.graphEOF, c.ended = false, false, false
}

// seek goes to the key frame before from, with what the decoder held
// dropped.
func (c *cursor) seek(from float64) error {
	tb := c.v.stream.TimeBase()
	ts := int64(math.Floor(from / tb.Float64()))
	if err := c.v.fc.SeekFrame(c.v.stream.Index(), ts, astiav.NewSeekFlags(astiav.SeekFlagBackward)); err != nil {
		return fmt.Errorf("the episode cannot be read from %.3f s: %s", from, err)
	}
	flushDecoder(c.v.dec)
	c.sentEOF, c.graphEOF = false, false
	return nil
}

// offChip stops scaling on the graphics chip, for this cursor and every
// other, and starts again from where the cursor was opened, so no frame
// the chip swallowed is missed.
func (c *cursor) offChip(why string) error {
	fmt.Fprintln(os.Stderr, "scaling on the graphics chip "+why+", scaling on the processor from now on")
	c.d.noChipScale.Store(true)
	c.freeGraph()
	return c.seek(c.from)
}

// next answers up to n frames from where the cursor is, none that starts
// before skip or before where it was opened at.
func (c *cursor) next(n int, skip float64) {
	if c.failedWhy != "" || c.v == nil {
		if c.failedWhy == "" {
			c.fail("a next before the cursor was opened")
		} else {
			c.fail(c.failedWhy)
		}
		return
	}
	for sent := 0; sent < n; {
		at, body, err := c.frame()
		if err != nil {
			if errors.Is(err, astiav.ErrEof) {
				c.ended = true
				c.d.answer(framewire.Record{Cursor: c.id, Kind: framewire.End})
			} else {
				c.fail(err.Error())
			}
			return
		}
		// Before the moment it was opened at, the way a seek with ffmpeg's
		// -ss drops them, and before the skip the page asked for.
		if at < c.from-1e-6 || at < skip-1e-6 {
			continue
		}
		c.d.answer(framewire.Record{Cursor: c.id, Kind: framewire.Frame, At: at, Body: body})
		sent++
	}
	c.d.answer(framewire.Record{Cursor: c.id, Kind: framewire.Done})
}

// frame is the next frame out of the chain, with where it starts.
func (c *cursor) frame() (float64, []byte, error) {
	for {
		if c.graph != nil {
			err := c.sink.GetFrame(c.filtered, astiav.NewBuffersinkFlags())
			if err == nil {
				c.got++
				at := float64(c.filtered.Pts()) * c.sink.TimeBase().Float64()
				body, berr := c.filtered.Data().Bytes(1)
				c.filtered.Unref()
				if berr != nil {
					return 0, nil, fmt.Errorf("a frame could not be read out: %w", berr)
				}
				return at, body, nil
			}
			if errors.Is(err, astiav.ErrEof) {
				return 0, nil, astiav.ErrEof
			}
			if !errors.Is(err, astiav.ErrEagain) {
				if c.graphGPU {
					if err := c.offChip(fmt.Sprintf("failed, %v", withSaid(err))); err != nil {
						return 0, nil, err
					}
					continue
				}
				return 0, nil, withSaid(fmt.Errorf("the picture could not be scaled: %w", err))
			}
		}
		// The chain wants a decoded frame.
		err := c.v.dec.ReceiveFrame(c.decoded)
		switch {
		case err == nil:
			if c.decoded.Pts() == astiav.NoPtsValue {
				c.decoded.Unref()
				continue
			}
			if c.graph == nil {
				if err := c.makeGraph(); err != nil {
					c.decoded.Unref()
					return 0, nil, err
				}
			}
			if err := c.src.AddFrame(c.decoded, astiav.NewBuffersrcFlags()); err != nil {
				c.decoded.Unref()
				if c.graphGPU {
					if err := c.offChip(fmt.Sprintf("failed, %v", withSaid(err))); err != nil {
						return 0, nil, err
					}
					continue
				}
				return 0, nil, withSaid(fmt.Errorf("the picture could not be scaled: %w", err))
			}
			c.decoded.Unref()
			c.fed++
			// On a Mac whose graphics chip is a virtual one the chip's
			// scaler takes frames and gives none back, without an error,
			// the way the ffmpeg program found it, see engine.PreviewFrames.
			if c.graphGPU && c.got == 0 && c.fed >= 8 {
				if err := c.offChip("gave no frame for eight"); err != nil {
					return 0, nil, err
				}
			}
		case errors.Is(err, astiav.ErrEof):
			if c.graph == nil {
				return 0, nil, astiav.ErrEof
			}
			if !c.graphEOF {
				c.graphEOF = true
				if err := c.src.AddFrame(nil, astiav.NewBuffersrcFlags()); err != nil {
					return 0, nil, astiav.ErrEof
				}
			}
		case errors.Is(err, astiav.ErrEagain):
			if err := c.feed(); err != nil {
				return 0, nil, err
			}
		default:
			return 0, nil, fmt.Errorf("the picture could not be decoded: %w", err)
		}
	}
}

// feed gives the decoder the next packet of the picture, or tells it the
// file has ended.
func (c *cursor) feed() error {
	if c.sentEOF {
		return nil
	}
	for {
		err := c.v.fc.ReadFrame(c.pkt)
		if errors.Is(err, astiav.ErrEof) {
			c.sentEOF = true
			return c.v.dec.SendPacket(nil)
		}
		if err != nil {
			return fmt.Errorf("the episode could not be read: %w", err)
		}
		if c.pkt.StreamIndex() != c.v.stream.Index() {
			c.pkt.Unref()
			continue
		}
		err = c.v.dec.SendPacket(c.pkt)
		c.pkt.Unref()
		if err != nil && !errors.Is(err, astiav.ErrEagain) {
			return fmt.Errorf("the picture could not be decoded: %w", err)
		}
		return nil
	}
}

// makeGraph builds the chain for the frames the decoder makes: on the
// graphics chip where they are there, scaled, made 8-bit and brought out,
// and on the processor otherwise. The same chains as engine.PreviewFrames.
func (c *cursor) makeGraph() error {
	c.fed, c.got, c.graphGPU = 0, 0, false
	cpu := fmt.Sprintf("scale=%d:%d:flags=bilinear:out_range=pc,format=yuv420p", c.width, c.height)
	if c.decoded.PixelFormat() != astiav.PixelFormatVideotoolbox {
		return c.tryGraph(cpu, false)
	}
	if c.d.noChipScale.Load() {
		return c.tryGraph("hwdownload,format=nv12|p010le,"+cpu, true)
	}
	// On the graphics chip first. Where its scaler will not take the
	// frames, they are brought out as they are and scaled on the
	// processor, the way the ffmpeg program falls back too.
	gpuErr := c.tryGraph(fmt.Sprintf("scale_vt=w=%d:h=%d,hwdownload,format=nv12|p010le,scale=out_range=pc,format=yuv420p", c.width, c.height), true)
	if gpuErr == nil {
		c.graphGPU = true
		return nil
	}
	fmt.Fprintln(os.Stderr, "scaling on the graphics chip failed, scaling on the processor:", gpuErr)
	c.d.noChipScale.Store(true)
	if err := c.tryGraph("hwdownload,format=nv12|p010le,"+cpu, true); err != nil {
		return fmt.Errorf("%w, and on the graphics chip %v", err, gpuErr)
	}
	return nil
}

// tryGraph builds one chain, hardware says the frames come from the
// graphics chip.
func (c *cursor) tryGraph(chain string, hardware bool) error {
	lastSaid.Store("")
	g := astiav.AllocFilterGraph()
	if g == nil {
		return errors.New("no memory to scale the picture")
	}
	fail := func(what string, err error) error {
		g.Free()
		return withSaid(fmt.Errorf("the picture could not be scaled, %s: %w", what, err))
	}
	src, err := g.NewBuffersrcFilterContext(astiav.FindFilterByName("buffer"), "in")
	if err != nil {
		return fail("its source", err)
	}
	sink, err := g.NewBuffersinkFilterContext(astiav.FindFilterByName("buffersink"), "out")
	if err != nil {
		return fail("its sink", err)
	}
	p := astiav.AllocBuffersrcFilterContextParameters()
	defer p.Free()
	if hardware {
		p.SetHardwareFramesContext(c.decoded.HardwareFramesContext())
	}
	p.SetWidth(c.decoded.Width())
	p.SetHeight(c.decoded.Height())
	p.SetPixelFormat(c.decoded.PixelFormat())
	p.SetSampleAspectRatio(c.decoded.SampleAspectRatio())
	p.SetTimeBase(c.v.stream.TimeBase())
	p.SetColorRange(c.decoded.ColorRange())
	p.SetColorSpace(c.decoded.ColorSpace())
	if err := src.SetParameters(p); err != nil {
		return fail("its source", err)
	}
	if err := src.Initialize(nil); err != nil {
		return fail("its source", err)
	}
	outputs := astiav.AllocFilterInOut()
	defer outputs.Free()
	outputs.SetName("in")
	outputs.SetFilterContext(src.FilterContext())
	outputs.SetPadIdx(0)
	outputs.SetNext(nil)
	inputs := astiav.AllocFilterInOut()
	defer inputs.Free()
	inputs.SetName("out")
	inputs.SetFilterContext(sink.FilterContext())
	inputs.SetPadIdx(0)
	inputs.SetNext(nil)
	if err := g.Parse(chain, inputs, outputs); err != nil {
		return fail("its chain", err)
	}
	if hardware {
		for _, f := range g.Filters() {
			if f.Filter().Flags().Has(astiav.FilterFlagHardwareDevice) {
				f.SetHardwareDeviceContext(c.d.hardware)
			}
		}
	}
	if err := g.Configure(); err != nil {
		return fail("its chain", err)
	}
	c.graph, c.src, c.sink = g, src, sink
	return nil
}
