// Package framewire is how the Go side and the episode's decoder,
// cmd/framefairy-frames, talk: requests as lines of text on the decoder's
// standard input, answers as records on its standard output. See
// docs/VIDEO-PREVIEW.md, The episode's decoder.
//
// A request names a cursor, one place in the episode the decoder keeps
// open with a decoder of its own, by a number the Go side chooses:
//
//	open <cursor> <frame> <width> <height> <origin> <grid> [key]
//	                                        the cursor goes to a frame of the
//	                                        grid, from the key frame before it
//	                                        with key, see Grid and OpenRequest
//	sound <cursor> <seek> <from> <rate> <channels>
//	                                        a cursor of sound seeks to seek, or
//	                                        reads from the start where it is 0,
//	                                        and starts at from
//	next <cursor> <n> <skip>                up to n frames, none before skip,
//	                                        or n chunks of sound
//	stop <cursor>                           a cursor of picture ends what it
//	                                        was asked before, between two
//	                                        frames, and answers its batch done
//	close <cursor>
//
// A cursor is of picture or of sound for as long as it is open. A stop is
// read at once, not after the work it ends, so a cursor sent to a place
// the page has left decodes no further towards it.
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
	// A frame: at is where it starts in the episode, in seconds, on the
	// grid of the cursor's open, and the body is the picture in colours, 4 bytes a pixel as Picture makes
	// them, at the cursor's size, or a chunk of sound, 32-bit floats with
	// the channels of a moment side by side.
	Frame byte = iota
	// The batch a next asked for is done.
	Done
	// The episode has ended, nothing more comes from this cursor.
	End
	// The cursor failed, the body says why.
	Failed
	// An open is done: the cursor is at its place. The body says how it
	// got there, see Reached, and a cursor of picture adds its Light after
	// a space: "file hlg", " pq", "on".
	Opened
	// The cursor's decoder hung, and the decoder gave up on it: nothing
	// more comes from it. A cursor opened after it decodes on the
	// processor, so the stream goes on from a new one.
	Hung
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

// Write writes a record, its header and then its body. Records written
// from several goroutines never interleave as long as each Write is under
// one lock, as the episode's decoder writes them. The body is written as
// it is: a frame is 5.8 MB at 1600 by 900, and copying it behind a header
// made every frame cost twice.
func Write(w io.Writer, r Record) error {
	var head [HeaderSize]byte
	binary.LittleEndian.PutUint32(head[0:], r.Cursor)
	head[4] = r.Kind
	binary.LittleEndian.PutUint64(head[5:], math.Float64bits(r.At))
	binary.LittleEndian.PutUint32(head[13:], uint32(len(r.Body)))
	if _, err := w.Write(head[:]); err != nil {
		return err
	}
	if len(r.Body) == 0 {
		return nil
	}
	_, err := w.Write(r.Body)
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

// Reached is how an open got its cursor to the place asked for, the first
// word of an Opened record.
type Reached string

const (
	// The cursor had the file and a decoder, and sought.
	Sought Reached = ""
	// The open opened the file and a decoder first, and sought.
	OpenedFile Reached = "file"
	// The cursor stood before the place with no key frame between, and
	// reads on from there without a seek. So an open with key hands over
	// no key frame before the frame asked for, issue #511.
	WentOn Reached = "on"
)

// OpenedBody is the body of an Opened record: how the open got there, and
// the light of the cursor's picture.
func OpenedBody(how Reached, light Light) []byte {
	body := string(how)
	if light != SDR {
		body += " " + string(light)
	}
	return []byte(body)
}

// ReadOpened reads the body of an Opened record. A way or a light it does
// not know is a seek of standard video, which is what an open said before
// it said either.
func ReadOpened(body []byte) (how Reached, light Light) {
	word, rest, _ := strings.Cut(string(body), " ")
	switch Reached(word) {
	case OpenedFile, WentOn:
		how = Reached(word)
	}
	switch Light(rest) {
	case PQ, HLG:
		light = Light(rest)
	}
	return how, light
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
// file's own range and matrix: red, green and blue with 10 bits each,
// X2BGR10 in little endian, red lowest, 4 bytes a pixel.
//
// 10 bits for every file, standard video too, though most of it has 8.
// Colours made from 8-bit video in 8 bits lose steps: video range spreads
// 220 values over 256, and the look of the video preview, QuickTime's,
// lifts the shadows further, so in a dark gradient one value of the file
// became a step of 3 on the screen, banding Tim saw on start.mp4 where
// QuickTime showed none. In 10 bits every value of the file keeps its own
// colour, and the look is put on in the video preview, on the GPU in
// floats, see frontend/src/lib/frames/light.ts.
//
// Standard video is still in the screen's curve and HDR in the file's
// own, PQ or HLG, with its own primaries, which the video preview turns
// into light on the screen. 10 bits is what an HDR file has.
//
// The short is never changed by any of it: its numbers are the file's.
// The episode's decoder builds its chain from here, and so does the ffmpeg
// program the engine's tests compare it with.
func Picture(width, height int) string {
	return fmt.Sprintf("scale=%d:%d:flags=bilinear,format=x2bgr10le", width, height)
}

// Rotate is the ffmpeg filter that rotates a picture so many degrees to
// the left, 90, 180 or 270, or nothing. It stands up the picture of a file
// that asks to be turned, the way ffmpeg does by itself, and it rotates a
// piece of a clip a person rotated.
func Rotate(degrees int) string {
	switch degrees {
	case 90:
		return "transpose=cclock"
	case 180:
		return "hflip,vflip"
	case 270:
		return "transpose=clock"
	}
	return ""
}

// Quarter is the quarter turn to the left, 0, 90, 180 or 270, that a
// display matrix's rotation asks for, given in degrees to the left the way
// ffprobe reports it, or nought for a turn that is not within a degree of
// a quarter. The engine reads it from ffprobe and the episode's decoder
// from the stream, and both stand a picture up by it with Rotate.
func Quarter(left float64) int {
	if math.IsNaN(left) || math.IsInf(left, 0) {
		return 0
	}
	q := math.Round(left / 90)
	if math.Abs(left-q*90) > 1 {
		return 0
	}
	return ((int(q)%4 + 4) % 4) * 90
}

// MaxBody is the largest body a record may have: a frame of 7680 by 4320.
const MaxBody = 7680 * 4320 * 4

// Read reads the next record. The body is a buffer of its own, which the
// caller may keep. The body of a frame is taken from pool where it keeps
// one of its size, and whoever reads the frame last may give it back
// there, see Pool.
func Read(r io.Reader, pool *Pool) (Record, error) {
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
	if rec.Kind > Hung {
		return Record{}, errors.New("a record of a kind nobody writes")
	}
	if rec.Kind == Frame {
		rec.Body = pool.take(int(size))
	}
	if rec.Body == nil {
		rec.Body = make([]byte, size)
	}
	if _, err := io.ReadFull(r, rec.Body); err != nil {
		pool.Give(rec.Body)
		return Record{}, err
	}
	return rec, nil
}
