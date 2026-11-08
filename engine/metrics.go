package engine

// How wide a caption comes out, without a text shaping library.
//
// A caption has to be broken into lines that fit inside the frame, and the
// app and the render have to break it in the same places. The app cannot
// call ffmpeg for every keystroke, so the widths are read straight out of
// the font file the captions are drawn from: the table that says how wide each
// glyph is, and the table that says which glyph a character uses.
//
// Kerning and shaping are left out. Kerning almost always pulls letters
// closer, so this measure comes out a shade wider than what libass draws,
// and a line that fits here fits there. A face neither the program nor the
// machine has cannot be measured at all, and then the caption style's character count
// decides, as it did before.

import (
	"encoding/binary"
	"sync"
)

type faceMetrics struct {
	mu   sync.Mutex
	upem float64
	// scale is how much smaller a face is drawn than its size suggests. The
	// size in a caption style is the height of the face, its ascent plus its
	// descent, and not the em square. VSFilter did it that way and libass
	// copies it, so a caption at size 96 in a face whose ascent and descent
	// come to 1.21 em is drawn at 79 pixels to the em.
	scale float64
	numH  int
	hmtx  []byte
	cmap  []byte
	cache map[rune]float64
}

func u16(b []byte, at int) (uint16, bool) {
	if at < 0 || at+2 > len(b) {
		return 0, false
	}
	return binary.BigEndian.Uint16(b[at:]), true
}

func u32(b []byte, at int) (uint32, bool) {
	if at < 0 || at+4 > len(b) {
		return 0, false
	}
	return binary.BigEndian.Uint32(b[at:]), true
}

// table finds one table of a font file by its four letter tag.
func table(data []byte, tag string) []byte {
	count, ok := u16(data, 4)
	if !ok || len(tag) != 4 {
		return nil
	}
	for i := 0; i < int(count); i++ {
		at := 12 + 16*i
		if at+16 > len(data) {
			return nil
		}
		if string(data[at:at+4]) != tag {
			continue
		}
		offset, ok1 := u32(data, at+8)
		length, ok2 := u32(data, at+12)
		if !ok1 || !ok2 || int(offset)+int(length) > len(data) {
			return nil
		}
		return data[offset : offset+length]
	}
	return nil
}

// bestCmap picks the character map to read: the one that covers the most,
// which is a full Unicode table where there is one.
func bestCmap(cmap []byte) []byte {
	count, ok := u16(cmap, 2)
	if !ok {
		return nil
	}
	best, bestRank := []byte(nil), -1
	for i := 0; i < int(count); i++ {
		at := 4 + 8*i
		platform, ok1 := u16(cmap, at)
		encoding, ok2 := u16(cmap, at+2)
		offset, ok3 := u32(cmap, at+4)
		if !ok1 || !ok2 || !ok3 || int(offset) >= len(cmap) {
			continue
		}
		sub := cmap[offset:]
		format, ok := u16(sub, 0)
		if !ok {
			continue
		}
		rank := -1
		switch {
		case format == 12 && platform == 3 && encoding == 10:
			rank = 3
		case format == 12:
			rank = 2
		case format == 4 && platform == 3 && encoding == 1:
			rank = 1
		case format == 4:
			rank = 0
		}
		if rank > bestRank {
			best, bestRank = sub, rank
		}
	}
	return best
}

// metricsCache is what was read of each face by its name. A name may be
// another file once the machine's fonts are read again, so Fonts keeps
// it and forgets it then.
type metricsCache struct {
	mu    sync.Mutex
	faces map[string]*faceMetrics
}

// forget drops what was read of every face.
func (c *metricsCache) forget() {
	c.mu.Lock()
	c.faces = nil
	c.mu.Unlock()
}

// face reads the tables of a font once and keeps them. A face the
// program carries is read from bundledMetrics, whichever Fonts asks, a nil
// one too. One of the machine's is kept by the Fonts that found it.
func (fonts *Fonts) face(name string) (*faceMetrics, bool) {
	if bundled, ok := bundledMetrics[name]; ok {
		f := bundled()
		return f, f != nil
	}
	if fonts == nil {
		return nil, false
	}
	c := &fonts.metrics
	c.mu.Lock()
	f, seen := c.faces[name]
	c.mu.Unlock()
	if seen {
		return f, f != nil
	}
	// Read without the lock, because finding a face of the machine may read
	// its font folders, which forgets every face read before.
	f = fonts.readFace(name)
	c.mu.Lock()
	if c.faces == nil {
		c.faces = map[string]*faceMetrics{}
	}
	c.faces[name] = f
	c.mu.Unlock()
	return f, f != nil
}

// bundledMetrics are the tables of the faces the program carries, each
// read the first time it is measured. They are read from the files built
// into the program and are the same for every machine and every test, as
// the hyphenation patterns are, so a test that measures one fills nothing
// another could see differently. A nil Fonts measures them without
// reading a face again for every line, which the word model's tests do
// thousands of times.
var bundledMetrics = func() map[string]func() *faceMetrics {
	table := map[string]func() *faceMetrics{}
	for _, font := range captionFonts {
		table[font.Name] = sync.OnceValue(func() *faceMetrics {
			data, err := fontBundle.ReadFile(FontsFolder + "/" + font.File)
			if err != nil {
				return nil
			}
			return readMetrics(data)
		})
	}
	return table
}()

func (fonts *Fonts) readFace(name string) *faceMetrics {
	data, ok := fonts.Bytes(name)
	if !ok {
		return nil
	}
	return readMetrics(data)
}

// readMetrics reads what measuring text needs of a font file.
func readMetrics(data []byte) *faceMetrics {
	head := table(data, "head")
	hhea := table(data, "hhea")
	hmtx := table(data, "hmtx")
	cmap := table(data, "cmap")
	if head == nil || hhea == nil || hmtx == nil || cmap == nil {
		return nil
	}
	upem, ok1 := u16(head, 18)
	numH, ok2 := u16(hhea, 34)
	sub := bestCmap(cmap)
	if !ok1 || !ok2 || upem == 0 || numH == 0 || sub == nil {
		return nil
	}
	scale := 1.0
	if os2 := table(data, "OS/2"); os2 != nil {
		ascent, ok1 := u16(os2, 74)
		descent, ok2 := u16(os2, 76)
		if ok1 && ok2 && ascent+descent > 0 {
			scale = float64(upem) / float64(int(ascent)+int(descent))
		}
	}
	return &faceMetrics{upem: float64(upem), scale: scale, numH: int(numH),
		hmtx: hmtx, cmap: sub, cache: map[rune]float64{}}
}

// glyph is the glyph a character uses, or zero when the face has none.
func (f *faceMetrics) glyph(r rune) int {
	format, _ := u16(f.cmap, 0)
	switch format {
	case 4:
		return f.glyph4(r)
	case 12:
		return f.glyph12(r)
	}
	return 0
}

func (f *faceMetrics) glyph4(r rune) int {
	if r > 0xFFFF {
		return 0
	}
	code := uint16(r)
	segX2, ok := u16(f.cmap, 6)
	if !ok || segX2 == 0 {
		return 0
	}
	segments := int(segX2) / 2
	ends, starts := 14, 16+int(segX2)
	deltas, ranges := starts+int(segX2), starts+2*int(segX2)
	for i := 0; i < segments; i++ {
		end, ok := u16(f.cmap, ends+2*i)
		if !ok || code > end {
			continue
		}
		start, ok := u16(f.cmap, starts+2*i)
		if !ok || code < start {
			return 0
		}
		delta, _ := u16(f.cmap, deltas+2*i)
		offset, _ := u16(f.cmap, ranges+2*i)
		if offset == 0 {
			return int(code + delta)
		}
		at := ranges + 2*i + int(offset) + 2*int(code-start)
		id, ok := u16(f.cmap, at)
		if !ok || id == 0 {
			return 0
		}
		return int(id + delta)
	}
	return 0
}

func (f *faceMetrics) glyph12(r rune) int {
	groups, ok := u32(f.cmap, 12)
	if !ok {
		return 0
	}
	low, high := 0, int(groups)-1
	for low <= high {
		middle := (low + high) / 2
		at := 16 + 12*middle
		start, ok1 := u32(f.cmap, at)
		end, ok2 := u32(f.cmap, at+4)
		first, ok3 := u32(f.cmap, at+8)
		if !ok1 || !ok2 || !ok3 {
			return 0
		}
		switch {
		case uint32(r) < start:
			high = middle - 1
		case uint32(r) > end:
			low = middle + 1
		default:
			return int(first + uint32(r) - start)
		}
	}
	return 0
}

// advance is how far the pen moves for a character, as a share of the em.
func (f *faceMetrics) advance(r rune) float64 {
	f.mu.Lock()
	defer f.mu.Unlock()
	if width, seen := f.cache[r]; seen {
		return width
	}
	index := f.glyph(r)
	if index >= f.numH {
		index = f.numH - 1
	}
	raw, ok := u16(f.hmtx, 4*index)
	if !ok {
		raw, _ = u16(f.hmtx, 0)
	}
	width := float64(raw) / f.upem
	f.cache[r] = width
	return width
}

// TextWidth is how wide a line of text comes out at a size, in the same
// unit as the size. It reports false for a face neither the program nor the
// machine has.
func (fonts *Fonts) TextWidth(font, text string, size float64) (float64, bool) {
	f, ok := fonts.face(font)
	if !ok {
		return 0, false
	}
	total := 0.0
	for _, r := range text {
		total += f.advance(r)
	}
	return total * size * f.scale, true
}

// Scale is the em size a caption size comes out as, as a share of that
// size. The app draws captions itself and needs it, because a font size
// in a browser is the em square.
func (fonts *Fonts) Scale(font string) float64 {
	f, ok := fonts.face(font)
	if !ok {
		return 1
	}
	return f.scale
}
