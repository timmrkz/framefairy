// Package framewire is how the Go side and the episode's decoder,
// cmd/framefairy-frames, talk: requests as lines of text on the decoder's
// standard input, answers as records on its standard output. See
// docs/VIDEO-PREVIEW.md, The episode's decoder.
//
// A request names a cursor, one place in the episode the decoder keeps
// open with a decoder of its own, by a number the Go side chooses:
//
//	open <cursor> <from> <width> <height>   the cursor goes to from, in seconds
//	next <cursor> <n> <skip>                up to n frames, none before skip
//	close <cursor>
//
// Every answer is a record of a header and a body. A next is answered by
// up to n frame records and then one record that ends the batch, or one
// that says the episode has ended or what failed. An open and a close are
// not answered.
package framewire

import (
	"encoding/binary"
	"errors"
	"fmt"
	"io"
	"math"
)

// What a record holds.
const (
	// A frame: at is where it starts in the episode, in seconds, and the
	// body is the picture in 8-bit I420 at the cursor's size.
	Frame byte = iota
	// The batch a next asked for is done.
	Done
	// The episode has ended, nothing more comes from this cursor.
	End
	// The cursor failed, the body says why.
	Failed
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

// MaxBody is the largest body a record may have: a frame of 7680 by 4320.
const MaxBody = 7680 * 4320 * 3 / 2

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
	if rec.Kind > Failed {
		return Record{}, errors.New("a record of a kind nobody writes")
	}
	rec.Body = make([]byte, size)
	if _, err := io.ReadFull(r, rec.Body); err != nil {
		return Record{}, err
	}
	return rec, nil
}
