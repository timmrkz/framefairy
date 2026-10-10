// Package framewire is how the Go side and the episode's decoder,
// cmd/framefairy-frames, talk: requests as lines of text on the decoder's
// standard input, answers as records on its standard output. See
// docs/VIDEO-PREVIEW.md, The episode's decoder.
//
// A request names a cursor, one place in the episode the decoder keeps
// open with a decoder of its own, by a number the Go side chooses:
//
//	open <cursor> <from> <width> <height>   the cursor goes to from, in seconds
//	sound <cursor> <seek> <from> <rate> <channels>
//	                                        a cursor of sound seeks to seek, or
//	                                        reads from the start where it is 0,
//	                                        and starts at from
//	next <cursor> <n> <skip>                up to n frames, none before skip,
//	                                        or n chunks of sound
//	close <cursor>
//
// A cursor is of picture or of sound for as long as it is open.
//
// Every answer is a record of a header and a body. A next is answered by
// up to n frame records and then one record that ends the batch, or one
// that says the episode has ended or what failed. An open is answered
// once the cursor is at its place, or with what failed. A close is not
// answered.
package framewire

import (
	"encoding/binary"
	"errors"
	"fmt"
	"io"
	"math"
	"strings"
)

// What a record holds.
const (
	// A frame: at is where it starts in the episode, in seconds, and the
	// body is the picture in colours, 4 bytes a pixel as Picture makes
	// them, at the cursor's size, or a chunk of sound, 32-bit floats with
	// the channels of a moment side by side.
	Frame byte = iota
	// The batch a next asked for is done.
	Done
	// The episode has ended, nothing more comes from this cursor.
	End
	// The cursor failed, the body says why.
	Failed
	// An open is done: the cursor is at its place. The body is "file" when
	// the open had to open the file and a decoder first, and empty when it
	// moved a cursor that had them. A cursor of picture adds its Light
	// after a space: "file hlg", " pq".
	Opened
)

// HeaderSize is the size of a record's header: the cursor, 4 bytes, what
// it holds, 1, the moment, 8, and the size of the body, 4, little endian.
const HeaderSize = 17

// Record is one answer.
type Record struct {
	Cursor uint32
	Kind   byte
	At     float64
	Body   []byte
}

// Write writes a record in one call, so records written from several
// goroutines under one lock never interleave.
func Write(w io.Writer, r Record) error {
	buf := make([]byte, HeaderSize+len(r.Body))
	binary.LittleEndian.PutUint32(buf[0:], r.Cursor)
	buf[4] = r.Kind
	binary.LittleEndian.PutUint64(buf[5:], math.Float64bits(r.At))
	binary.LittleEndian.PutUint32(buf[13:], uint32(len(r.Body)))
	copy(buf[HeaderSize:], r.Body)
	_, err := w.Write(buf)
	return err
}

// Light is how a picture's brightness is coded: standard video, or HDR
// with the curve of PQ or of HLG, from the file's transfer tag. It is the
// word an open answers with after "file", see Opened.
type Light string

const (
	SDR Light = ""
	PQ  Light = "pq"
	HLG Light = "hlg"
)

// OpenedBody is the body of an Opened record: whether the open opened the
// file, and the light of the cursor's picture.
func OpenedBody(file bool, light Light) []byte {
	body := ""
	if file {
		body = "file"
	}
	if light != SDR {
		body += " " + string(light)
	}
	return []byte(body)
}

// ReadOpened reads the body of an Opened record. A light it does not know
// is standard video, which is what it was before HDR came.
func ReadOpened(body []byte) (file bool, light Light) {
	word, rest, _ := strings.Cut(string(body), " ")
	switch Light(rest) {
	case PQ, HLG:
		light = Light(rest)
	}
	return word == "file", light
}

// LightOf is the light of a file whose transfer tag is trc, as ffmpeg
// names it: smpte2084 is PQ, arib-std-b67 HLG, and everything else,
// no tag included, standard video.
func LightOf(trc string) Light {
	switch trc {
	case "smpte2084":
		return PQ
	case "arib-std-b67":
		return HLG
	}
	return SDR
}

// Picture is the chain that makes a decoded frame what the video preview
// draws, scaled to width by height and turned into colours from the
// file's own range and matrix, 4 bytes a pixel.
//
// Standard video is RGBA with 8 bits each and an opaque alpha, so a 2D
// canvas draws the bytes as they are where the webview has no WebGPU, and
// given the look Tim picked in the side-by-side test of step 4,
// docs/VIDEO-PREVIEW.md. That look is QuickTime's: the shadows and middle
// tones of standard video lifted the way the Mac shows them, measured on
// Tim's screen from start.mp4 beside QuickTime. A value v of 0 to 1 is
// shown as v to the power of 0.98 - 0.31 v, never below 0.891, which is
// 1.961 over 2.2, the Mac's curve for video over the screen's. So black
// stays black and white white, a grey of 22 is shown as 25 and one of 81
// as 92.
//
// HDR is red, green and blue with 10 bits each, X2BGR10 in little endian,
// red lowest, still in the file's own curve, PQ or HLG, and its own
// primaries. The video preview turns that into light on the screen, see
// frontend/src/lib/frames/screen.ts. 10 bits is what an HDR file has.
//
// The short is never changed by any of it: its numbers are the file's.
// The episode's decoder and engine.PreviewFrames build the same chain from
// here.
func Picture(width, height int, light Light) string {
	if light != SDR {
		return fmt.Sprintf("scale=%d:%d:flags=bilinear,format=x2bgr10le", width, height)
	}
	const lift = "clip(round(255*pow(val/255,max(0.891,0.98-0.31*val/255))),0,255)"
	return fmt.Sprintf("scale=%d:%d:flags=bilinear,format=rgb24,lutrgb=r='%s':g='%s':b='%s',format=rgba", width, height, lift, lift, lift)
}

// MaxBody is the largest body a record may have: a frame of 7680 by 4320.
const MaxBody = 7680 * 4320 * 4

// Read reads the next record. The body is a buffer of its own, which the
// caller may keep.
func Read(r io.Reader) (Record, error) {
	var head [HeaderSize]byte
	if _, err := io.ReadFull(r, head[:]); err != nil {
		return Record{}, err
	}
	rec := Record{
		Cursor: binary.LittleEndian.Uint32(head[0:]),
		Kind:   head[4],
		At:     math.Float64frombits(binary.LittleEndian.Uint64(head[5:])),
	}
	size := binary.LittleEndian.Uint32(head[13:])
	if size > MaxBody {
		return Record{}, fmt.Errorf("a record of %d bytes is larger than any frame", size)
	}
	if rec.Kind > Opened {
		return Record{}, errors.New("a record of a kind nobody writes")
	}
	rec.Body = make([]byte, size)
	if _, err := io.ReadFull(r, rec.Body); err != nil {
		return Record{}, err
	}
	return rec, nil
}
