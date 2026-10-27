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
	"time"

	"github.com/asticode/go-astiav"

	"framefairy/internal/framewire"
)

func main() {
	if len(os.Args) != 2 {
		fmt.Fprintln(os.Stderr, "usage: framefairy-frames <episode>")
		os.Exit(2)
	}
	astiav.SetLogLevel(astiav.LogLevelError)
	astiav.SetLogCallback(logged)
	d := &decoder{path: os.Args[1], out: bufio.NewWriterSize(os.Stdout, 1<<20), cursors: map[uint32]*cursor{}, layers: map[uint32]*layer{}}
	// The episode is opened once here, so a file that cannot be read says so
	// before anything is asked of it.
	probe, err := openVideo(d.path)
	if err != nil {
		fmt.Fprintln(os.Stderr, err)
		os.Exit(1)
	}
	info("opened %s", probe.about())
	probe.close()
	if runtime.GOOS == "darwin" {
		if t := astiav.FindHardwareDeviceTypeByName("videotoolbox"); t != astiav.HardwareDeviceTypeNone {
			hw, err := astiav.CreateHardwareDeviceContext(t, "", nil, 0)
			if err != nil {
				info("the graphics chip cannot decode here: %v", err)
			} else {
				d.hardware = hw
				info("the graphics chip is ready, VideoToolbox")
			}
		} else {
			info("this ffmpeg has no VideoToolbox")
		}
	}
	lines := bufio.NewScanner(os.Stdin)
	for lines.Scan() {
		d.request(strings.Fields(lines.Text()))
	}
	d.closeAll()
}

// turnedOf is how far a stream's display matrix asks for its picture to be
// turned, in degrees to the left, 0, 90, 180 or 270, the way ffprobe and
// engine.Probe read it. A turn that is not a quarter is no turn, as for
// ffmpeg.
func turnedOf(s *astiav.Stream) int {
	m, ok := s.CodecParameters().SideData().DisplayMatrix().Get()
	if !ok {
		return 0
	}
	// astiav gives the rotation clockwise, ffprobe to the left.
	return framewire.Quarter(-m.Rotation())
}

// info writes a line for the app's log alone, see engine.DecoderInfo: what
// the decoder opened and how, and how long its work took. Its other lines
// are ffmpeg's errors, which say why a stream failed.
func info(format string, args ...any) {
	fmt.Fprintf(os.Stderr, "info: "+format+"\n", args...)
}

// warn writes a line for the app's log at the level of a warning, see
// engine.DecoderWarning: something went wrong that the decoder got round.
func warn(format string, args ...any) {
	fmt.Fprintf(os.Stderr, "warning: "+format+"\n", args...)
}

// about is the episode's picture in a few words, for the log.
func (v *video) about() string {
	p := v.stream.CodecParameters()
	frames := v.stream.NbFrames()
	rate := v.stream.AvgFrameRate()
	tb := v.stream.TimeBase()
	length := float64(v.fc.Duration()) / 1e6
	// The transfer and the primaries as ffmpeg numbers them: 1 is BT.709,
	// 16 PQ and 18 HLG, 9 BT.2020.
	return fmt.Sprintf("%s, %s profile %d, %dx%d %s, %d frames at %.3f a second, time base %d/%d, %.3f s, transfer %d, primaries %d, matrix %s, range %s, %d streams",
		v.fc.InputFormat().Name(), p.CodecID().Name(), p.Profile(), p.Width(), p.Height(), p.PixelFormat().Name(),
		frames, rate.Float64(), tb.Num(), tb.Den(), length,
		int(p.ColorTransferCharacteristic()), int(p.ColorPrimaries()), p.ColorSpace().Name(), p.ColorRange().Name(),
		len(v.fc.Streams()))
}

// What ffmpeg says when something fails is kept with the cursor it is
// about, so a failure says why in ffmpeg's own words and not only in an
// error number, and never in the words of another cursor's failure.
// ffmpeg names the object it speaks of, a decoder, a file, a chain or a
// filter in one, and owners finds the cursor that object belongs to.
var (
	saidMu sync.Mutex
	owners = map[string]*cursor{}
)

// saidKept is how much of what ffmpeg said a cursor keeps, the latest.
const saidKept = 1000

// logged is what ffmpeg says, at the level of an error. A failure is
// often told in two lines, the second only that it failed and the first
// why, so a cursor keeps every line since it was last asked, up to
// saidKept bytes of the latest.
func logged(c astiav.Classer, _ astiav.LogLevel, _, msg string) {
	said := strings.TrimSpace(msg)
	var cl *astiav.Class
	if c != nil {
		cl = c.Class()
	}
	if cl != nil {
		said = cl.Name() + ": " + said
	}
	fmt.Fprintln(os.Stderr, said)
	saidMu.Lock()
	defer saidMu.Unlock()
	// The object itself, or what it is part of.
	for i := 0; cl != nil && i < 4; i, cl = i+1, cl.Parent() {
		if owner := owners[classKey(cl)]; owner != nil {
			if owner.said != "" {
				said = owner.said + "; " + said
			}
			owner.said = said[max(0, len(said)-saidKept):]
			return
		}
	}
}

// classKey is the address ffmpeg's log names an object by.
func classKey(cl *astiav.Class) string {
	if cl == nil {
		return ""
	}
	s := cl.String()
	if i := strings.LastIndex(s, " @ "); i >= 0 {
		return s[i+3:]
	}
	return ""
}

// own has what ffmpeg says about these objects kept with the cursor, under
// keys that disown lets go of.
func (c *cursor) own(keys *[]string, objects ...astiav.Classer) {
	saidMu.Lock()
	defer saidMu.Unlock()
	for _, o := range objects {
		if k := classKey(o.Class()); k != "" {
			owners[k] = c
			*keys = append(*keys, k)
		}
	}
}

func (c *cursor) disown(keys *[]string) {
	saidMu.Lock()
	defer saidMu.Unlock()
	for _, k := range *keys {
		if owners[k] == c {
			delete(owners, k)
		}
	}
	*keys = nil
}

// withSaid is err with what ffmpeg said about this cursor's objects since
// it was last asked, where it said anything.
func (c *cursor) withSaid(err error) error {
	saidMu.Lock()
	said := c.said
	c.said = ""
	saidMu.Unlock()
	if said != "" {
		return fmt.Errorf("%w (ffmpeg: %s)", err, said)
	}
	return err
}

type decoder struct {
	path     string
	hardware *astiav.HardwareDeviceContext
	// Whether the graphics chip hung on a cursor, see watch. Every decoder
	// opened after it decodes on the processor.
	chipHung atomic.Bool
	mu       sync.Mutex
	out      *bufio.Writer
	cursors  map[uint32]*cursor
	layers   map[uint32]*layer
}

// chip is the graphics chip to decode on, none once it hung.
func (d *decoder) chip() *astiav.HardwareDeviceContext {
	if d.chipHung.Load() {
		return nil
	}
	return d.hardware
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
	case f[0] == "open":
		_, frame, w, h, grid, key, ok := framewire.ReadOpen(f)
		if !ok {
			d.answer(framewire.Record{Cursor: id, Kind: framewire.Failed, Body: []byte("an open the decoder cannot take")})
			return
		}
		c := d.cursor(id, false)
		if c == nil {
			return
		}
		c.work <- func() { c.open(frame, w, h, grid, key) }
	case f[0] == "sound" && len(f) == 6:
		seek, err1 := strconv.ParseFloat(f[2], 64)
		from, err2 := strconv.ParseFloat(f[3], 64)
		rate, err3 := strconv.Atoi(f[4])
		channels, err4 := strconv.Atoi(f[5])
		if err1 != nil || err2 != nil || err3 != nil || err4 != nil || math.IsNaN(seek) || math.IsInf(seek, 0) ||
			math.IsNaN(from) || math.IsInf(from, 0) || seek < 0 || from < seek ||
			rate < 8000 || rate > 192000 || channels < 1 || channels > 8 {
			d.answer(framewire.Record{Cursor: id, Kind: framewire.Failed, Body: []byte("a sound the decoder cannot take")})
			return
		}
		c := d.cursor(id, true)
		if c == nil {
			return
		}
		c.work <- func() { c.openSound(seek, from, rate, channels) }
	case f[0] == "next" && len(f) == 4:
		n, err1 := strconv.Atoi(f[2])
		skip, err2 := strconv.ParseFloat(f[3], 64)
		c := d.cursors[id]
		if c == nil || err1 != nil || err2 != nil || n < 1 || math.IsNaN(skip) {
			d.answer(framewire.Record{Cursor: id, Kind: framewire.Failed, Body: []byte("a next for no open cursor")})
			return
		}
		if c.sound {
			c.work <- func() { c.nextSound(min(n, 16)) }
			return
		}
		c.work <- func() { c.next(min(n, 16), skip) }
	case f[0] == "captions":
		_, c, ok := framewire.ReadCaptions(f)
		if !ok {
			d.answer(framewire.Record{Cursor: id, Kind: framewire.Failed, Body: []byte("captions the decoder cannot take")})
			return
		}
		if l := d.layer(id); l != nil {
			l.work <- func() { l.open(c) }
		}
	case f[0] == "draw":
		_, ms, ok := framewire.ReadDraw(f)
		l := d.layers[id]
		if !ok || l == nil {
			d.answer(framewire.Record{Cursor: id, Kind: framewire.Failed, Body: []byte("a draw for no open captions")})
			return
		}
		l.work <- func() { l.draw(ms) }
	case f[0] == "close":
		if c := d.cursors[id]; c != nil {
			delete(d.cursors, id)
			close(c.work)
		}
		if l := d.layers[id]; l != nil {
			delete(d.layers, id)
			close(l.work)
		}
	}
}

// cursor is the cursor of this number, made where there is none, of
// picture or of sound. A number is one kind for as long as it is open, and
// an open of the other kind fails.
func (d *decoder) cursor(id uint32, sound bool) *cursor {
	if _, taken := d.layers[id]; taken {
		d.answer(framewire.Record{Cursor: id, Kind: framewire.Failed, Body: []byte("a cursor of captions asked for picture or sound")})
		return nil
	}
	c := d.cursors[id]
	if c == nil {
		c = &cursor{d: d, id: id, sound: sound, work: make(chan func(), 4), hangs: make(chan struct{})}
		d.cursors[id] = c
		go c.run()
	}
	if c.sound != sound {
		d.answer(framewire.Record{Cursor: id, Kind: framewire.Failed, Body: []byte("a cursor of picture asked for sound, or of sound for picture")})
		return nil
	}
	return c
}

func (d *decoder) closeAll() {
	for id, c := range d.cursors {
		delete(d.cursors, id)
		close(c.work)
	}
	for id, l := range d.layers {
		delete(d.layers, id)
		close(l.work)
	}
}

// video is the episode opened for reading: the file, its first picture
// stream, or its first sound stream for a cursor of sound, and a decoder
// for it.
type video struct {
	fc     *astiav.FormatContext
	stream *astiav.Stream
	dec    *astiav.CodecContext
	// chip tells whether dec decodes on the graphics chip.
	chip bool
	// turned is how far the file asks for its picture to be turned to
	// stand up, in degrees to the left, which ffmpeg does by itself and
	// so the engine's sizes already are. See engine.SourceInfo.Turned.
	turned int
}

func openVideo(path string) (*video, error) {
	return openMedia(path, astiav.MediaTypeVideo)
}

func openMedia(path string, kind astiav.MediaType) (*video, error) {
	fc := astiav.AllocFormatContext()
	if fc == nil {
		return nil, errors.New("no memory to open the video")
	}
	if err := fc.OpenInput(path, nil, nil); err != nil {
		fc.Free()
		return nil, fmt.Errorf("the video cannot be opened: %w", err)
	}
	v := &video{fc: fc}
	if err := fc.FindStreamInfo(nil); err != nil {
		v.close()
		return nil, fmt.Errorf("the video cannot be read: %w", err)
	}
	for _, s := range fc.Streams() {
		if s.CodecParameters().MediaType() == kind {
			v.stream = s
			break
		}
	}
	if v.stream != nil {
		v.turned = turnedOf(v.stream)
	}
	if v.stream == nil {
		v.close()
		if kind == astiav.MediaTypeAudio {
			return nil, errors.New("the video has no sound")
		}
		return nil, errors.New("the video has no picture")
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
// the system has one and it takes the file. Where it does not, ffmpeg
// decodes on the processor by itself, and either way the frame comes out
// the same, see cursor.frame.
func (v *video) openDecoder(hardware *astiav.HardwareDeviceContext, say func(string)) error {
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
	// On the graphics chip where it decodes this codec, and otherwise on
	// the processor with as many threads as it has. Never both: a decoder
	// offered the chip with threads of its own failed on every frame on a
	// Mac, "Invalid argument", since the chip is chosen for each frame from
	// ffmpeg's threads.
	chip := hardware != nil && chipDecodes(v.stream.CodecParameters().CodecID())
	if chip {
		dec.SetHardwareDeviceContext(hardware)
		said := ""
		dec.SetPixelFormatCallback(func(pfs []astiav.PixelFormat) astiav.PixelFormat {
			for _, pf := range pfs {
				if pf == astiav.PixelFormatVideotoolbox {
					if said != "chip" {
						said = "chip"
						say("decodes on the graphics chip")
					}
					return pf
				}
			}
			// The chip does not take it: the first the processor can.
			if said != "processor" {
				said = "processor"
				say(fmt.Sprintf("the graphics chip did not take it, decodes on the processor as %s, one thread", pfs[0].Name()))
			}
			return pfs[0]
		})
	} else {
		dec.SetThreadCount(0)
		if hardware != nil {
			say("the graphics chip does not decode " + v.stream.CodecParameters().CodecID().Name() + ", decodes on the processor")
		} else if v.stream.CodecParameters().MediaType() == astiav.MediaTypeVideo {
			say("decodes on the processor")
		}
	}
	if err := dec.Open(codec, nil); err != nil {
		dec.Free()
		return fmt.Errorf("the decoder could not be opened: %w", err)
	}
	v.dec, v.chip = dec, chip
	return nil
}

// cursor is one place in the episode with a decoder of its own. Its work,
// an open or a next, runs on its own goroutine, one after another.
type cursor struct {
	d     *decoder
	id    uint32
	sound bool
	work  chan func()
	// Closed when the decoder is made to hang, see hangAfter, which says
	// at once what chipHang says on the clock.
	hangs chan struct{}
	// When the call into the decoder the cursor is in began, counted from
	// started and one more, or 0 while it is in none, see inDecoder.
	calling atomic.Int64

	v       *video
	pkt     *astiav.Packet
	decoded *astiav.Frame
	// The frame decoded last, held until the one after it says how many
	// frames of the grid it is shown for, see advance.
	held    *astiav.Frame
	holding bool
	// A frame decoded on the graphics chip, brought out to memory.
	brought  *astiav.Frame
	filtered *astiav.Frame
	// The chain that makes a decoded frame the size of the canvas, in
	// colours ready to draw, the same as engine.PreviewFrames asks of ffmpeg.
	graph  *astiav.FilterGraph
	src    *astiav.BuffersrcFilterContext
	sink   *astiav.BuffersinkFilterContext
	width  int
	height int
	// How the picture's brightness is coded, from the file's transfer tag.
	light framewire.Light
	// The grid of the last open, the frame of it to hand over next, and
	// the picture made for it, which is shown up to the frame until. Every
	// moment the cursor speaks of is counted from the grid's origin, the
	// file's start as the engine read it.
	grid  framewire.Grid
	rank  int
	shown []byte
	until int
	// How often this open sought again for a seek that landed too late.
	retries int
	// Whether this open hands over the key frame its decoding starts from
	// first, see open, and whether the frame handed over next is that key
	// frame. wanted is the frame asked for, which comes right after it.
	fromKey, keyNext bool
	wanted           int
	// Whether this open went on with the processor after the graphics
	// chip failed, see toProcessor.
	fellBack bool
	// Packets sent to the decoder in this open, and whether the decoder
	// was made to fail in it, see failAfter.
	sentInOpen int
	madeToFail bool
	sentEOF    bool
	graphEOF   bool
	// Packets of the picture read ahead by a jump that went on, see goOn,
	// fed to the decoder before the file is read further.
	queued []*astiav.Packet
	// When the packet shown latest of those fed since the last seek is
	// shown, see skipBefore, and how many frames this open decoded.
	latest        int64
	decodedInOpen int
	failedWhy     string
	// What ffmpeg said about this cursor's objects, under saidMu, and the
	// keys they are known by: the file and its decoder, and the chain.
	said      string
	keys      []string
	graphKeys []string
	// Sound: where the cut starts on the file's clock, the rate and
	// channels asked for, the moment its samples start, decoded samples not
	// yet handed over, how many chunks were, and whether the episode's
	// sound has ended.
	from           float64
	rate, channels int
	soundFrom      float64
	// When the last open was asked, and whether its first frame was said
	// in the log yet.
	asked     time.Time
	firstSaid bool
	// Whether it has moved from where the file was opened.
	read       bool
	pcm        []byte
	chunks     int
	soundEnded bool
}

func (c *cursor) run() {
	for job := range c.work {
		if !c.watch(job) {
			// Given up: what is still asked of it is dropped, and what
			// it holds stays with the work that hung, which may still
			// use it.
			for range c.work {
			}
			return
		}
	}
	c.free()
}

// chipHang is how long one call into the decoder may take on the
// graphics chip: a packet handed in, a frame taken out or brought off the
// chip. Each takes milliseconds, a frame of 4K too. On CI's virtual Mac
// the chip stopped once in the middle of a stream and never went on, and
// the video preview with it, issue #553. ffmpeg's calls cannot be called
// off, so the work is left where it hangs. Reading the file is not
// watched, so a slow disk or a file still coming down from iCloud is no
// hang, and neither is a long seek, which is many calls.
const chipHang = 10 * time.Second

// started is what the calls into the decoder are timed from, on the
// clock that only goes forward.
var started = time.Now()

// inDecoder makes one call into the decoder, noting when it began, so
// watch can tell a chip that hangs from work that is only long.
func (c *cursor) inDecoder(call func() error) error {
	c.calling.Store(int64(time.Since(started)) + 1)
	defer c.calling.Store(0)
	return call()
}

// stuck tells whether the cursor has been in one call into the decoder
// for chipHang or more.
func (c *cursor) stuck() bool {
	at := c.calling.Load()
	return at != 0 && time.Since(started)-time.Duration(at-1) >= chipHang
}

// watch runs one piece of the cursor's work and tells whether the cursor
// goes on. On the graphics chip it gives up on the cursor once one call
// into the decoder has taken chipHang: the stream is told the cursor
// hung, and every decoder from then on decodes on the processor. On the
// processor nothing is watched, since work there is as slow as the
// machine is.
func (c *cursor) watch(job func()) bool {
	onChip := !c.sound && c.d.chip() != nil
	if !onChip && hangAfter == 0 {
		job()
		return true
	}
	done := make(chan struct{})
	go func() {
		defer close(done)
		job()
	}()
	var tick <-chan time.Time
	if onChip {
		t := time.NewTicker(chipHang / 4)
		defer t.Stop()
		tick = t.C
	}
	for hung := false; !hung; {
		select {
		case <-done:
			return true
		case <-tick:
			hung = c.stuck()
		case <-c.hangs:
			hung = true
		}
	}
	c.d.chipHung.Store(true)
	warn("cursor %d, picture: one call into the decoder took %s or more, gives it up and decodes on the processor from now on",
		c.id, chipHang)
	c.d.answer(framewire.Record{Cursor: c.id, Kind: framewire.Hung})
	return false
}

func (c *cursor) free() {
	c.freeGraph()
	c.disown(&c.keys)
	for _, f := range []*astiav.Frame{c.decoded, c.held, c.brought, c.filtered} {
		if f != nil {
			f.Free()
		}
	}
	if c.pkt != nil {
		c.pkt.Free()
	}
	c.dropQueued()
	if c.v != nil {
		c.v.close()
	}
}

func (c *cursor) freeGraph() {
	if c.graph != nil {
		c.disown(&c.graphKeys)
		c.graph.Free()
		c.graph, c.src, c.sink = nil, nil, nil
	}
}

// say writes a line about this cursor for the app's log.
func (c *cursor) say(line string) {
	kind := "picture"
	if c.sound {
		kind = "sound"
	}
	info("cursor %d, %s: %s", c.id, kind, line)
}

func (c *cursor) fail(why string) {
	c.say("failed: " + why)
	c.failedWhy = why
	c.d.answer(framewire.Record{Cursor: c.id, Kind: framewire.Failed, Body: []byte(why)})
}

// open moves the cursor to frame of the grid: to the key frame shown at
// its moment or before, with what the decoder held dropped. The file and
// the decoder stay open, which is the whole point: the first open of a
// cursor opens them, every one after it only seeks.
//
// With key, an open that seeks hands over first the key frame the seek
// lands on, as the frame of the grid it is shown for, and the frames from
// there on, so the next batch can skip ahead to the frame asked for. A
// key frame takes its own decoding and that of the few frames stored next
// to it, the next shown frame and the B-frames before it, where the frame
// asked for takes every frame from the key frame to it, up to two seconds
// of them. The
// video preview shows the key frame while the hand drags the playhead
// faster than the frames under it can be decoded, the way established
// players scrub, issue #281. An open that goes on without a seek is near
// already, and hands over the frame asked for.
func (c *cursor) open(frame, w, h int, grid framewire.Grid, key bool) {
	c.failedWhy = ""
	c.asked, c.firstSaid = time.Now(), false
	c.sentInOpen, c.madeToFail, c.decodedInOpen = 0, false, 0
	// A cursor that fell back to the processor tries the graphics chip
	// again at the next open, see toProcessor.
	// One on a graphics chip that has since hung goes on the processor.
	reopened := false
	if c.v != nil && (c.fellBack || (c.v.chip && c.d.chip() == nil)) {
		c.fellBack, reopened = false, true
		if hw := c.d.chip(); hw != nil || c.v.chip {
			if err := c.reopenDecoder(hw); err != nil {
				c.fail(err.Error())
				return
			}
		}
	}
	opened := c.v == nil
	if c.v == nil {
		v, err := openVideo(c.d.path)
		if err == nil {
			err = v.openDecoder(c.d.chip(), c.say)
			if err != nil {
				v.close()
			}
		}
		if err != nil {
			c.fail(err.Error())
			return
		}
		c.v = v
		c.light = lightOf(v.stream.CodecParameters().ColorTransferCharacteristic())
		c.own(&c.keys, v.fc, v.dec)
		if c.pkt == nil {
			c.pkt = astiav.AllocPacket()
			c.decoded = astiav.AllocFrame()
			c.held = astiav.AllocFrame()
			c.brought = astiav.AllocFrame()
			c.filtered = astiav.AllocFrame()
		}
	}
	// The chain holds no frames between two, so it is kept unless the size
	// changed.
	if c.graph != nil && (c.width != w || c.height != h) {
		c.freeGraph()
	}
	from := grid.At(frame)
	c.fromKey, c.keyNext = false, false
	if !opened && !reopened && c.goOn(frame, w, h, grid) {
		c.say(fmt.Sprintf("went on, at frame %d, %.3f s, for %dx%d, %d ms", frame, from, w, h, time.Since(c.asked).Milliseconds()))
		c.d.answer(framewire.Record{Cursor: c.id, Kind: framewire.Opened, Body: framewire.OpenedBody(framewire.WentOn, c.light)})
		return
	}
	c.grid, c.rank, c.until, c.shown, c.retries, c.fromKey, c.keyNext = grid, frame, frame, nil, 0, key, false
	if err := c.seek(from + framewire.Hair); err != nil {
		c.fail(err.Error())
		return
	}
	c.width, c.height = w, h
	how := "moved"
	if opened {
		how = "opened the file"
	}
	if key {
		how += ", the key frame first"
	}
	c.say(fmt.Sprintf("%s, at frame %d, %.3f s, for %dx%d, %d ms", how, frame, from, w, h, time.Since(c.asked).Milliseconds()))
	reached := framewire.Sought
	if opened {
		reached = framewire.OpenedFile
	}
	c.d.answer(framewire.Record{Cursor: c.id, Kind: framewire.Opened, Body: framewire.OpenedBody(reached, c.light)})
}

// goOn takes the cursor to the frame of the grid without a seek, when that
// decodes no more than a seek would: the frame is at or after the one the
// cursor hands over next, and no key frame is shown between the frame the
// cursor decoded last and it. A seek decodes from the key frame shown at
// or before the frame, which is then no later than where the cursor
// already is. A drag forward or Shift and an arrow key jumped back to the
// key frame each time and decoded again what the cursor had just decoded,
// plan row 2.243. The packets up to the frame are read ahead to see that
// no key frame comes first, and kept for the decoder, see feed. A frame
// further ahead than goOnAhead is left to a seek, rather than read ahead
// to find out.
func (c *cursor) goOn(frame, w, h int, grid framewire.Grid) bool {
	if c.v == nil || !c.holding || c.sentEOF || grid != c.grid || w != c.width || h != c.height ||
		c.graph == nil || frame < c.rank || frame >= grid.Count {
		return false
	}
	if c.shown != nil && frame < c.until {
		// The picture made last is shown for that frame too.
		c.rank, c.retries = frame, 0
		return true
	}
	tb := c.v.stream.TimeBase().Float64()
	at := grid.At(frame) + framewire.Hair + grid.Origin
	if at-float64(c.held.Pts())*tb > goOnAhead {
		return false
	}
	keyBy := func(p *astiav.Packet) bool {
		return p.Flags().Has(astiav.PacketFlagKey) && p.Pts() != astiav.NoPtsValue && float64(p.Pts())*tb <= at
	}
	for _, p := range c.queued {
		if keyBy(p) {
			return false
		}
	}
	for len(c.queued) == 0 || c.queued[len(c.queued)-1].Dts() == astiav.NoPtsValue ||
		float64(c.queued[len(c.queued)-1].Dts())*tb <= at {
		p := astiav.AllocPacket()
		err := c.v.fc.ReadFrame(p)
		if err != nil {
			p.Free()
			if errors.Is(err, astiav.ErrEof) {
				break
			}
			return false
		}
		if p.StreamIndex() != c.v.stream.Index() {
			p.Free()
			continue
		}
		c.queued = append(c.queued, p)
		if keyBy(p) {
			return false
		}
	}
	c.rank, c.retries = frame, 0
	return true
}

// How far ahead of the frame the cursor decoded last a jump may be and
// still go on, in seconds: longer than the time between two key frames in
// almost every video, so the jumps it leaves to a seek are the ones a seek
// is faster for.
const goOnAhead = 5.0

// dropQueued lets go of the packets goOn read ahead.
func (c *cursor) dropQueued() {
	for _, p := range c.queued {
		p.Free()
	}
	c.queued = c.queued[:0]
}

// lightOf is the light of the episode's picture, from its transfer tag,
// as framewire.LightOf has it for the name.
func lightOf(trc astiav.ColorTransferCharacteristic) framewire.Light {
	switch trc {
	case astiav.ColorTransferCharacteristicSmpte2084:
		return framewire.PQ
	case astiav.ColorTransferCharacteristicAribStdB67:
		return framewire.HLG
	}
	return framewire.SDR
}

// seek goes to the key frame before from, counted from the start of the
// file, with what the decoder held dropped.
func (c *cursor) seek(from float64) error {
	tb := c.v.stream.TimeBase().Float64()
	at := from + c.grid.Origin
	ts, err := c.keyBefore(at, int64(math.Floor(at/tb)))
	if err == nil {
		err = c.v.fc.SeekFrame(c.v.stream.Index(), ts, astiav.NewSeekFlags(astiav.SeekFlagBackward))
	}
	if err != nil {
		return fmt.Errorf("the video cannot be read from %.3f s: %s", from, err)
	}
	flushDecoder(c.v.dec)
	c.held.Unref()
	c.sentEOF, c.holding = false, false
	c.dropQueued()
	c.latest = astiav.NoPtsValue
	return nil
}

// keyBefore is where to seek to for the frame at from: a moment whose key
// frame is shown at from or before it. A seek finds its key frame by when
// it is decoded, and in an open GOP, HEVC's CRA, the key frame is decoded
// before the frames shown just ahead of it, which need the GOP before to
// be decoded and are dropped when a decoder starts at that key. So a seek
// to one of those frames landed on a key frame shown after it, and the
// frame asked for never came: start.mp4 stood without a picture and would
// not play, plan row 2.186. The key frame a seek leads to is read, and
// while it is shown after from, the seek goes to the key before it.
//
// A file with no index of its key frames, MPEG-TS above all, lands a seek
// on whatever packet is stamped before the moment, mostly one between two
// key frames. The decoder drops everything up to the next key frame, and
// that was shown for every frame before it. So the key frame a seek leads
// to is the first packet with the key flag from where it landed, and where
// a seek to just before that key leads to the same key again, the seek
// goes a second further back, then two, until it finds the key before.
// A seek that lands after the last key frame of the file finds no key
// before the end, and goes back the same way: it was taken for a key
// shown in time, the decoder dropped every frame to the end, and a jump
// into the last stretch of an MPEG-TS file, or a fallback to the
// processor there, showed nothing at all. Issue #287.
func (c *cursor) keyBefore(from float64, ts int64) (int64, error) {
	tb := c.v.stream.TimeBase().Float64()
	floor := c.v.stream.StartTime()
	if floor == astiav.NoPtsValue {
		floor = math.MinInt64
	}
	landed, back := int64(math.MaxInt64), int64(math.Ceil(1/tb))
	for range 24 {
		if err := c.v.fc.SeekFrame(c.v.stream.Index(), ts, astiav.NewSeekFlags(astiav.SeekFlagBackward)); err != nil {
			return 0, err
		}
		pts, dts, err := c.firstKey()
		ended := errors.Is(err, astiav.ErrEof)
		if err != nil && !ended {
			return ts, nil
		}
		if !ended && (pts == astiav.NoPtsValue || dts == astiav.NoPtsValue || float64(pts)*tb <= from+1e-6) {
			return ts, nil
		}
		switch {
		case ended && ts <= floor:
			// No key frame anywhere in the picture.
			return ts, nil
		case ended:
			// The seek landed after the last key frame of the file, so
			// no key frame came before the end.
			c.say(fmt.Sprintf("a seek for %.3f s found no key frame before the end, going %.0f s further back",
				from-c.grid.Origin, float64(back)*tb))
			ts, back = ts-back, back*2
		case dts < landed:
			landed, ts = dts, dts-1
		case ts <= floor:
			// The first key frame of the file: the picture begins after
			// from.
			return ts, nil
		default:
			ts, back = ts-back, back*2
		}
	}
	return ts, nil
}

// firstKey is when the next key frame of the picture is shown and when it
// is decoded. It is read and let go, so a seek has to follow it.
func (c *cursor) firstKey() (pts, dts int64, err error) {
	for {
		if err := c.v.fc.ReadFrame(c.pkt); err != nil {
			return 0, 0, err
		}
		if c.pkt.StreamIndex() != c.v.stream.Index() || !c.pkt.Flags().Has(astiav.PacketFlagKey) {
			c.pkt.Unref()
			continue
		}
		pts, dts = c.pkt.Pts(), c.pkt.Dts()
		c.pkt.Unref()
		return pts, dts, nil
	}
}

// next answers up to n frames of the grid from where the cursor is, none
// for a moment before skip.
func (c *cursor) next(n int, skip float64) {
	if c.failedWhy != "" || c.v == nil {
		if c.failedWhy == "" {
			c.fail("a next before the cursor was opened")
		} else {
			c.fail(c.failedWhy)
		}
		return
	}
	if skip > 0 {
		c.rank = max(c.rank, c.grid.FirstShown(skip+framewire.Hair-1e-6))
	}
	began := time.Now()
	sent := 0
	for sent < n {
		if c.rank >= c.grid.Count {
			c.say(fmt.Sprintf("the grid ended after %d frames of %d asked", sent, n))
			c.d.answer(framewire.Record{Cursor: c.id, Kind: framewire.End})
			return
		}
		if c.shown == nil || c.rank >= c.until {
			err := c.advance()
			if err != nil && !errors.Is(err, astiav.ErrEof) && (c.v.chip || errors.Is(err, errMadeToFail)) {
				if err = c.toProcessor(err); err == nil {
					continue
				}
			}
			if err != nil {
				if errors.Is(err, astiav.ErrEof) {
					c.say(fmt.Sprintf("the video ended after %d frames of %d asked", sent, n))
					c.d.answer(framewire.Record{Cursor: c.id, Kind: framewire.End})
				} else {
					c.fail(err.Error())
				}
				return
			}
			continue
		}
		at := c.grid.At(c.rank)
		key := c.keyNext
		if key {
			// The frame asked for is said when it comes, as the first.
			c.keyNext = false
			c.say(fmt.Sprintf("key frame %d at %.3f s first, %d ms after the open, %d decoded", c.rank, at, time.Since(c.asked).Milliseconds(), c.decodedInOpen))
		} else if !c.firstSaid {
			c.firstSaid = true
			c.say(fmt.Sprintf("first frame %d at %.3f s, %d ms after the open, %d decoded", c.rank, at, time.Since(c.asked).Milliseconds(), c.decodedInOpen))
		}
		c.d.answer(framewire.Record{Cursor: c.id, Kind: framewire.Frame, At: at, Body: c.shown})
		c.rank++
		if key {
			// The frames between the key frame and the frame asked for
			// are wanted by nobody, so they are decoded and let go
			// unscaled, the way a skip lets them go.
			c.rank = max(c.rank, c.wanted)
		}
		sent++
	}
	if took := time.Since(began); took > 250*time.Millisecond {
		c.say(fmt.Sprintf("%d frames took %d ms, %d sent", n, took.Milliseconds(), sent))
	}
	c.d.answer(framewire.Record{Cursor: c.id, Kind: framewire.Done})
}

// advance makes the picture shown for the cursor's frame of the grid: the
// frame of the file that holds its moment, the last to begin at or before
// it, give or take framewire.Hair, the way the render takes it, see
// byTimes in engine/render.go. A frame is held until the next one comes
// and says from which frame of the grid on it takes over. A frame that is
// shown for none is let go unscaled: a jump decodes up to two seconds of
// frames from the key frame before it, and scaling each to the size of the
// canvas only to throw it away kept a drag waiting. Where the file ends,
// its last frame is shown to the end of the grid, as tpad holds it on in
// the render, and before the picture begins its first frame is.
func (c *cursor) advance() error {
	for {
		begins, err := c.decode()
		if errors.Is(err, astiav.ErrEof) {
			if !c.holding {
				return astiav.ErrEof
			}
			c.holding = false
			c.decoded, c.held = c.held, c.decoded
			return c.show(c.grid.Count)
		}
		if err != nil {
			return err
		}
		first := c.grid.FirstShown(begins)
		held := c.holding
		if !held && c.fromKey && first <= c.rank {
			// The key frame a seek landed on, handed over from the frame
			// of the grid it is shown for, see open. One that landed too
			// late is sought before, as any other.
			// A key frame shown before the grid begins stands for its
			// first frame, as the first frame of a file does.
			c.fromKey, c.keyNext, c.wanted = false, true, c.rank
			c.rank = max(first, 0)
		}
		if !held && first > c.rank && !c.firstOfFile() {
			// The first frame after a seek begins after the moment asked
			// for, and it is not the file's first frame, which stands for
			// the moments before the picture begins: the seek landed too
			// late, and that frame would stand for every frame before it.
			c.decoded.Unref()
			if err := c.seekBack(begins); err != nil {
				return err
			}
			continue
		}
		c.decoded, c.held = c.held, c.decoded
		c.holding = true
		if held && first > c.rank {
			return c.show(first)
		}
		c.decoded.Unref()
	}
}

// firstOfFile tells whether the frame in decoded is the first frame of
// the picture.
func (c *cursor) firstOfFile() bool {
	start := c.v.stream.StartTime()
	return start != astiav.NoPtsValue && c.decoded.Pts() <= start
}

// seekBack seeks again, a second further back than the last seek of this
// open went and twice as far each time, for the cursor's frame of the grid
// that a seek reached too late, at a frame that begins at landed.
func (c *cursor) seekBack(landed float64) error {
	if c.retries >= 6 {
		return fmt.Errorf("the frame at %.3f s cannot be reached: a seek to it lands on a frame at %.3f s", c.grid.At(c.rank), landed)
	}
	c.retries++
	back := float64(int(1) << (c.retries - 1))
	c.say(fmt.Sprintf("a seek for frame %d at %.3f s landed on a frame at %.3f s, seeking %.0f s further back",
		c.rank, c.grid.At(c.rank), landed, back))
	return c.seek(c.grid.At(c.rank) + framewire.Hair - back)
}

// decode puts the next frame out of the decoder in decoded, with when it
// begins, counted from the start of the file.
func (c *cursor) decode() (float64, error) {
	tb := c.v.stream.TimeBase().Float64()
	for {
		err := c.inDecoder(func() error { return c.v.dec.ReceiveFrame(c.decoded) })
		switch {
		case err == nil:
			// Its moment is ffmpeg's best guess, as the ffmpeg program
			// takes it, so a frame that carries none of its own still has
			// one.
			pts := bestEffort(c.decoded)
			if pts == astiav.NoPtsValue {
				c.decoded.Unref()
				continue
			}
			c.decoded.SetPts(pts)
			c.decodedInOpen++
			return float64(pts)*tb - c.grid.Origin, nil
		case errors.Is(err, astiav.ErrEof):
			return 0, astiav.ErrEof
		case errors.Is(err, astiav.ErrEagain):
			if err := c.feed(); err != nil {
				return 0, err
			}
		default:
			return 0, c.withSaid(fmt.Errorf("the picture could not be decoded: %w", err))
		}
	}
}

// failAfter makes the decoder in use fail once in every open, on the
// packet of that number it is sent, as the graphics chip does now and
// then, so a test sees the cursor go on with the processor, see
// toProcessor. On the processor it is the same reopen, so the race
// detector on Linux proves it, and on a Mac the chip's way there too.
// Nought, never, unless FRAMEFAIRY_FRAMES_FAIL_AFTER says otherwise.
var failAfter, _ = strconv.Atoi(os.Getenv("FRAMEFAIRY_FRAMES_FAIL_AFTER"))

var errMadeToFail = errors.New("made to fail by FRAMEFAIRY_FRAMES_FAIL_AFTER")

// hangAfter makes the decoder in use hang for good, once in the
// program's life, on the packet of that number a cursor of picture sends
// it, as the graphics chip did on CI's virtual Mac, so a test sees the
// stream go on from a new cursor on the processor, see watch. It says it
// hangs as it does, so the test waits for no chipHang of the clock.
// Nought, never, unless FRAMEFAIRY_FRAMES_HANG_AFTER says otherwise.
var hangAfter, _ = strconv.Atoi(os.Getenv("FRAMEFAIRY_FRAMES_HANG_AFTER"))

var madeToHang atomic.Bool

// toProcessor goes on decoding on the processor after the graphics chip
// failed, in sending a packet, receiving a frame or bringing one off the
// chip. VideoToolbox fails a frame now and then on a virtual Mac,
// "hardware accelerator failed to decode picture", and the cursor then
// failed and the video preview stood still, one CI run in four. The
// render decodes on the processor, so the frame the processor makes is
// the frame the short has. The decoder is opened again without the chip
// and the cursor seeks to the frame of the grid it is at, the way open
// does, so the frames it sends go on from where they were. It stays on
// the processor for the rest of this open and tries the chip again at the
// next, because ffmpeg starts a lost VideoToolbox session again by
// itself. The line goes to the log as a warning with all ffmpeg said, so
// a report says why the chip failed, and the frames after it say whether
// the file was fine.
func (c *cursor) toProcessor(why error) error {
	what := "the graphics chip failed"
	if !c.v.chip {
		what = "the decoder failed"
	}
	warn("cursor %d, picture: %s at frame %d, decodes on the processor for the rest of this open: %v",
		c.id, what, c.rank, c.withSaid(why))
	if err := c.reopenDecoder(nil); err != nil {
		return err
	}
	c.fellBack = true
	c.retries = 0
	return c.seek(c.grid.At(c.rank) + framewire.Hair)
}

// reopenDecoder opens the picture's decoder again, on the graphics chip
// where hardware is one. The chain is built again for the frames it
// makes. Where it cannot be opened the file is closed too, so the next
// open starts from nothing rather than from a cursor with no decoder.
func (c *cursor) reopenDecoder(hardware *astiav.HardwareDeviceContext) error {
	c.disown(&c.keys)
	c.freeGraph()
	c.v.dec.Free()
	c.v.dec = nil
	if err := c.v.openDecoder(hardware, c.say); err != nil {
		c.v.close()
		c.v = nil
		return err
	}
	c.own(&c.keys, c.v.fc, c.v.dec)
	return nil
}

// show scales the frame in decoded through the chain and shows it from
// the cursor's frame of the grid up to until.
func (c *cursor) show(until int) error {
	// The frame brought off the graphics chip takes decoded's place.
	defer func() { c.decoded.Unref() }()
	if err := c.bringOut(); err != nil {
		return err
	}
	if c.graph == nil {
		if err := c.makeGraph(); err != nil {
			return err
		}
	}
	if err := c.src.AddFrame(c.decoded, astiav.NewBuffersrcFlags()); err != nil {
		return c.withSaid(fmt.Errorf("the picture could not be scaled: %w", err))
	}
	if err := c.sink.GetFrame(c.filtered, astiav.NewBuffersinkFlags()); err != nil {
		return c.withSaid(fmt.Errorf("the picture could not be scaled: %w", err))
	}
	body, err := c.filtered.Data().Bytes(1)
	c.filtered.Unref()
	if err != nil {
		return fmt.Errorf("a frame could not be read out: %w", err)
	}
	c.shown, c.until = body, until
	return nil
}

// feed gives the decoder the next packet of the picture, or tells it the
// file has ended.
func (c *cursor) feed() error {
	if c.sentEOF {
		return nil
	}
	for {
		p := c.pkt
		if len(c.queued) > 0 {
			p = c.queued[0]
			c.queued = append(c.queued[:0], c.queued[1:]...)
			defer p.Free()
		} else {
			err := c.v.fc.ReadFrame(p)
			if errors.Is(err, astiav.ErrEof) {
				c.sentEOF = true
				return c.inDecoder(func() error { return c.v.dec.SendPacket(nil) })
			}
			if err != nil {
				return c.withSaid(fmt.Errorf("the video could not be read: %w", err))
			}
			if p.StreamIndex() != c.v.stream.Index() {
				p.Unref()
				continue
			}
		}
		c.sentInOpen++
		if hangAfter > 0 && !c.sound && c.sentInOpen == hangAfter && madeToHang.CompareAndSwap(false, true) {
			close(c.hangs)
			select {}
		}
		if failAfter > 0 && c.sentInOpen == failAfter && !c.madeToFail {
			p.Unref()
			c.madeToFail = true
			return errMadeToFail
		}
		if !c.sound {
			c.skipBefore(p)
		}
		err := c.inDecoder(func() error { return c.v.dec.SendPacket(p) })
		p.Unref()
		if err != nil && !errors.Is(err, astiav.ErrEagain) {
			return c.withSaid(fmt.Errorf("the picture could not be decoded: %w", err))
		}
		return nil
	}
}

// skipBefore tells the decoder whether to skip the frame of the packet p
// if no other frame is made from it: when p is shown before a packet fed
// since the last seek that is itself shown no later than the frame of the
// grid the cursor hands over next. The frame of the packet shown latest is
// never skipped, as nothing fed is shown after it, so a frame is decoded
// that is shown after p's and no later than that frame of the grid, and
// p's frame is shown for none the cursor still hands over. On the way to
// the frame a jump asks for, this leaves out the B-frames before it,
// which an export has two of in every three frames. The frame that holds
// the moment asked for is never left out. Plan row 2.243.
func (c *cursor) skipBefore(p *astiav.Packet) {
	pts := p.Pts()
	if pts == astiav.NoPtsValue {
		skipNonRef(c.v.dec, false)
		return
	}
	tb := c.v.stream.TimeBase().Float64()
	next := c.grid.At(c.rank) + framewire.Hair + c.grid.Origin
	skipNonRef(c.v.dec, c.latest != astiav.NoPtsValue && pts < c.latest && float64(c.latest)*tb <= next)
	if c.latest == astiav.NoPtsValue || pts > c.latest {
		c.latest = pts
	}
}

// bringOut brings a frame the graphics chip decoded out to memory as it
// is, NV12 or P010, with its moment and its colour, so the chain after it
// is the same for every frame on every system. The chip only decodes.
// Scaling there too, scale_vt, was a second chain beside this one, with a
// fallback for a chip that would not take it, which Tim's Mac did not for
// start.mp4, HEVC with 10-bit colour, and a virtual Mac does not at all.
// Under one engine there is one chain. Plan row 2.156.
func (c *cursor) bringOut() error {
	if c.decoded.PixelFormat() != astiav.PixelFormatVideotoolbox {
		return nil
	}
	err := c.inDecoder(func() error { return c.decoded.TransferHardwareData(c.brought) })
	if err == nil {
		err = copyFrameProps(c.brought, c.decoded)
	}
	c.decoded.Unref()
	if err != nil {
		c.brought.Unref()
		return c.withSaid(fmt.Errorf("the picture could not be brought off the graphics chip: %w", err))
	}
	c.decoded, c.brought = c.brought, c.decoded
	return nil
}

// makeGraph builds the chain for the frames the decoder makes, the one
// engine.PreviewFrames asks of the ffmpeg program, framewire.Picture:
// scaled to the size of the canvas and turned into colours, 10-bit red,
// green and blue. The colours are ffmpeg's, from the file's own range
// and matrix, with ffmpeg's defaults where the file says nothing. Plan
// row 2.156, step 4.
func (c *cursor) makeGraph() error {
	chain := framewire.Picture(c.width, c.height)
	if turn := framewire.Rotate(c.v.turned); turn != "" {
		chain = turn + "," + chain
	}
	c.say(fmt.Sprintf("chain from %dx%d %s, range %s, matrix %s: %s", c.decoded.Width(), c.decoded.Height(),
		c.decoded.PixelFormat().Name(), c.decoded.ColorRange().Name(), c.decoded.ColorSpace().Name(), chain))
	return c.buildGraph(chain)
}

// buildGraph builds a chain from the decoded frame's kind to chain's end,
// for a picture or for sound.
func (c *cursor) buildGraph(chain string) error {
	what, source, sinkName := "the picture could not be scaled", "buffer", "buffersink"
	if c.sound {
		what, source, sinkName = "the sound could not be converted", "abuffer", "abuffersink"
	}
	g := astiav.AllocFilterGraph()
	if g == nil {
		return errors.New(what + ", no memory")
	}
	c.own(&c.graphKeys, g)
	fail := func(part string, err error) error {
		err = c.withSaid(fmt.Errorf("%s, %s: %w", what, part, err))
		c.disown(&c.graphKeys)
		g.Free()
		return err
	}
	src, err := g.NewBuffersrcFilterContext(astiav.FindFilterByName(source), "in")
	if err != nil {
		return fail("its source", err)
	}
	sink, err := g.NewBuffersinkFilterContext(astiav.FindFilterByName(sinkName), "out")
	if err != nil {
		return fail("its sink", err)
	}
	p := astiav.AllocBuffersrcFilterContextParameters()
	defer p.Free()
	p.SetTimeBase(c.v.stream.TimeBase())
	if c.sound {
		p.SetSampleRate(c.decoded.SampleRate())
		p.SetSampleFormat(c.decoded.SampleFormat())
		p.SetChannelLayout(c.decoded.ChannelLayout())
	} else {
		p.SetWidth(c.decoded.Width())
		p.SetHeight(c.decoded.Height())
		p.SetPixelFormat(c.decoded.PixelFormat())
		p.SetSampleAspectRatio(c.decoded.SampleAspectRatio())
		p.SetColorRange(c.decoded.ColorRange())
		p.SetColorSpace(c.decoded.ColorSpace())
	}
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
	// Each filter speaks for itself when the chain is set up.
	for _, f := range g.Filters() {
		c.own(&c.graphKeys, f)
	}
	if err := g.Configure(); err != nil {
		return fail("its chain", err)
	}
	c.graph, c.src, c.sink = g, src, sink
	return nil
}
