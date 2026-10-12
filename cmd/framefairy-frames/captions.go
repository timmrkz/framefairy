//go:build ffmpeglibs

package main

import (
	"bytes"
	"errors"
	"fmt"
	"os"
	"strings"

	"github.com/asticode/go-astiav"

	"framefairy/internal/framewire"
)

// layer is a cursor of captions: the short's caption file held open in
// the filter the render burns it in with, ass, on a clear picture of a
// size, so the video preview shows the captions libass draws and not a
// copy of them. See framewire.CaptionsRequest and docs/CAPTIONS.md.
type layer struct {
	d    *decoder
	id   uint32
	work chan func()

	width, height int
	graph         *astiav.FilterGraph
	src           *astiav.BuffersrcFilterContext
	sink          *astiav.BuffersinkFilterContext
	blank         []byte
	in, out       *astiav.Frame
	// Whether the filter puts the colours in video range, see videoRange.
	video bool
}

// layer is the cursor of captions of this number, made where there is
// none. A number of a cursor of picture or sound is no cursor of captions.
func (d *decoder) layer(id uint32) *layer {
	if _, taken := d.cursors[id]; taken {
		d.answer(framewire.Record{Cursor: id, Kind: framewire.Failed, Body: []byte("a cursor of picture or sound asked for captions")})
		return nil
	}
	l := d.layers[id]
	if l == nil {
		l = &layer{d: d, id: id, work: make(chan func(), 4)}
		d.layers[id] = l
		go func() {
			for job := range l.work {
				job()
			}
			l.free()
		}()
	}
	return l
}

func (l *layer) free() {
	if l.graph != nil {
		l.graph.Free()
		l.graph, l.src, l.sink = nil, nil, nil
	}
	for _, f := range []*astiav.Frame{l.in, l.out} {
		if f != nil {
			f.Free()
		}
	}
	l.in, l.out = nil, nil
}

func (l *layer) fail(why string) {
	info("captions %d failed: %s", l.id, why)
	l.d.answer(framewire.Record{Cursor: l.id, Kind: framewire.Failed, Body: []byte(why)})
}

// open holds a caption file open, drawn at a size. A cursor that held one
// lets it go first, so a caption file written again is read again.
func (l *layer) open(c framewire.Captions) {
	l.free()
	l.width, l.height = c.Width, c.Height
	head, err := os.ReadFile(c.File)
	if err != nil {
		l.fail(fmt.Sprintf("the captions could not be read: %v", err))
		return
	}
	l.video = videoRange(head)
	if err := l.build(c); err != nil {
		l.free()
		l.fail(err.Error())
		return
	}
	l.in, l.out = astiav.AllocFrame(), astiav.AllocFrame()
	info("captions %d: %dx%d of %dx%d from %s", l.id, c.Width, c.Height, c.ShortW, c.ShortH, c.File)
	l.d.answer(framewire.Record{Cursor: l.id, Kind: framewire.Opened})
}

// build makes the chain: a picture of red, green and blue, on a clock of
// milliseconds, through the ass filter, the way the render's chain names
// it, see engine.subtitleCandidates.
func (l *layer) build(c framewire.Captions) error {
	g := astiav.AllocFilterGraph()
	if g == nil {
		return errors.New("the captions could not be drawn, no memory")
	}
	l.graph = g
	src, err := g.NewBuffersrcFilterContext(astiav.FindFilterByName("buffer"), "in")
	if err != nil {
		return fmt.Errorf("the captions could not be drawn, its source: %w", err)
	}
	sink, err := g.NewBuffersinkFilterContext(astiav.FindFilterByName("buffersink"), "out")
	if err != nil {
		return fmt.Errorf("the captions could not be drawn, its sink: %w", err)
	}
	p := astiav.AllocBuffersrcFilterContextParameters()
	defer p.Free()
	p.SetTimeBase(astiav.NewRational(1, 1000))
	p.SetWidth(l.width)
	p.SetHeight(l.height)
	p.SetPixelFormat(astiav.PixelFormatRgb24)
	p.SetSampleAspectRatio(astiav.NewRational(1, 1))
	p.SetColorRange(astiav.ColorRangeJpeg)
	if err := src.SetParameters(p); err != nil {
		return fmt.Errorf("the captions could not be drawn, its source: %w", err)
	}
	if err := src.Initialize(nil); err != nil {
		return fmt.Errorf("the captions could not be drawn, its source: %w", err)
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
	if err := g.Parse(captionChain(c), inputs, outputs); err != nil {
		return fmt.Errorf("the captions could not be drawn, its chain: %w", err)
	}
	if err := g.Configure(); err != nil {
		return fmt.Errorf("the captions could not be drawn, its chain: %w", err)
	}
	l.src, l.sink = src, sink
	return nil
}

// captionChain is the chain that draws a caption file with the faces in
// its folder of fonts: the render's filter, on a picture of red, green and
// blue, told the size of the short's frames, original_size, so it draws
// the captions as the short's frames would show them made the size of
// the picture.
func captionChain(c framewire.Captions) string {
	return "ass=filename=" + filterValue(c.File) + ":fontsdir=" + filterValue(c.Fonts) +
		fmt.Sprintf(":original_size=%dx%d", c.ShortW, c.ShortH) + ",format=rgb24"
}

// videoRange says whether the ass filter draws a caption file's colours
// in video range, 16 to 235, on a picture of red, green and blue too: for
// a file that names a matrix of TV, or none it knows, which is every
// short of standard video, see engine.Style.matrix. It is the filter's own
// rule, ass_get_color_range in vf_subtitles.c: PC is full range, and None
// is the picture's, which build makes full.
func videoRange(file []byte) bool {
	for _, line := range strings.Split(string(file), "\n") {
		name, value, ok := strings.Cut(strings.TrimSpace(line), ":")
		if ok && strings.EqualFold(strings.TrimSpace(name), "YCbCr Matrix") {
			value = strings.ToUpper(strings.TrimSpace(value))
			return !strings.HasPrefix(value, "PC.") && value != "NONE"
		}
		if strings.HasPrefix(strings.TrimSpace(line), "[V4") {
			break
		}
	}
	return true
}

// filterValue is a name as a value of a filter's option in a chain, which
// ffmpeg unescapes twice: once as a value of the filter's options, where
// a colon ends it, and once as a part of the chain, where a comma, a
// semicolon or a bracket does.
func filterValue(s string) string {
	option := strings.NewReplacer(`\`, `\\`, `'`, `\'`, `:`, `\:`).Replace(s)
	return strings.NewReplacer(`\`, `\\`, `'`, `\'`, `[`, `\[`, `]`, `\]`, `,`, `\,`, `;`, `\;`).Replace(option)
}

// draw answers with the captions at ms. The filter blends every colour
// into the picture under it, the way it does into the short's, so the
// captions drawn once on black and once on white say all there is: on
// black, each pixel is its colour times its alpha, and white shows through
// by one less the alpha. That is exact for any caption, a box that lets
// the picture through among them, where the alpha the filter writes when
// asked for one is its alpha squared.
func (l *layer) draw(ms int64) {
	if l.graph == nil {
		l.fail("a draw for captions that are not open")
		return
	}
	black, err := l.drawOn(0, ms)
	if err == nil {
		var white []byte
		white, err = l.drawOn(255, ms)
		if err == nil {
			drawn := framewire.Inked(layerOf(black, white, l.video), l.width, l.height)
			l.d.answer(framewire.Record{Cursor: l.id, Kind: framewire.Frame, At: float64(ms) / 1000, Body: drawn.Body()})
			return
		}
	}
	l.fail(err.Error())
}

// drawOn draws the captions at ms on a picture of one grey.
func (l *layer) drawOn(grey byte, ms int64) ([]byte, error) {
	l.in.SetWidth(l.width)
	l.in.SetHeight(l.height)
	l.in.SetPixelFormat(astiav.PixelFormatRgb24)
	l.in.SetSampleAspectRatio(astiav.NewRational(1, 1))
	if err := l.in.AllocBuffer(1); err != nil {
		return nil, fmt.Errorf("the captions could not be drawn: %w", err)
	}
	if len(l.blank) != l.width*l.height*3 || l.blank[0] != grey {
		l.blank = bytes.Repeat([]byte{grey}, l.width*l.height*3)
	}
	if err := l.in.Data().SetBytes(l.blank, 1); err != nil {
		l.in.Unref()
		return nil, fmt.Errorf("the captions could not be drawn: %w", err)
	}
	l.in.SetPts(ms)
	err := l.src.AddFrame(l.in, astiav.NewBuffersrcFlags())
	l.in.Unref()
	if err != nil {
		return nil, fmt.Errorf("the captions could not be drawn: %w", err)
	}
	if err := l.sink.GetFrame(l.out, astiav.NewBuffersinkFlags()); err != nil {
		return nil, fmt.Errorf("the captions could not be drawn: %w", err)
	}
	pixels, err := l.out.Data().Bytes(1)
	l.out.Unref()
	if err != nil {
		return nil, fmt.Errorf("the captions could not be read out: %w", err)
	}
	return pixels, nil
}

// layerOf is the captions drawn on black and on white as a layer of red,
// green, blue and alpha, with the alpha multiplied in, see
// framewire.Layer. The alpha is what of the white is hidden, the mean of
// the three colours. In video range, see videoRange, a colour c times its
// alpha a is put back in full range as (c − 16·a/255)·255/219, since the
// 16 the range adds is blended by the alpha too.
func layerOf(black, white []byte, video bool) []byte {
	n := len(black) / 3
	out := make([]byte, 4*n)
	for i := range n {
		b, w := black[3*i:3*i+3], white[3*i:3*i+3]
		shown := int(w[0]) - int(b[0]) + int(w[1]) - int(b[1]) + int(w[2]) - int(b[2])
		a := max(0, min(255, 255-(shown+1)/3))
		for k := range 3 {
			v := int(b[k])
			if video {
				v = (255*v - 16*a + 109) / 219
			}
			out[4*i+k] = byte(max(0, min(a, v)))
		}
		out[4*i+3] = byte(a)
	}
	return out
}
