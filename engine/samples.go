package engine

import "io"

// sampleReader reads the 32-bit float samples ffmpeg writes into one
// buffer, kept for the whole reading. A pipe hands the bytes out in pieces
// of any size, so a read often ends inside a sample. Those up to three
// bytes are moved to the front of the buffer and the next read lands after
// them, so no read is ever copied whole to put a sample together.
type sampleReader struct {
	r    io.Reader
	buf  []byte
	tail []byte
}

// newSampleReader reads from r up to size bytes at a time.
func newSampleReader(r io.Reader, size int) *sampleReader {
	return &sampleReader{r: r, buf: make([]byte, size+3)}
}

// Read gives back the whole samples of the next read, as bytes in the
// reader's own buffer, good until the next call, and the reader's error.
func (s *sampleReader) Read() ([]byte, error) {
	carried := copy(s.buf, s.tail)
	n, err := s.r.Read(s.buf[carried:])
	data := s.buf[:carried+n]
	whole := len(data) / 4 * 4
	s.tail = data[whole:]
	return data[:whole], err
}
