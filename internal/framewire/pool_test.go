package framewire

import (
	"bytes"
	"io"
	"sync"
	"testing"
)

// frames writes a frame record of size bytes for each fill, every byte
// of its body the fill.
func frames(t *testing.T, size int, fills ...byte) *bytes.Buffer {
	t.Helper()
	var b bytes.Buffer
	for _, fill := range fills {
		if err := Write(&b, Record{Cursor: 1, Kind: Frame, At: float64(fill), Body: bytes.Repeat([]byte{fill}, size)}); err != nil {
			t.Fatal(err)
		}
	}
	return &b
}

// A frame given back is what the next frame of its size is read into, so
// a play makes no new memory for its frames, issue #898. A frame still
// held is never read into: its pixels stay as they came.
func TestAFrameGivenBackIsReadIntoAgain(t *testing.T) {
	pool := NewPool(1 << 20)
	r := frames(t, 1000, 1, 2, 3)
	first, err := Read(r, pool)
	if err != nil {
		t.Fatal(err)
	}
	pool.Give(first.Body)
	second, err := Read(r, pool)
	if err != nil {
		t.Fatal(err)
	}
	if &second.Body[0] != &first.Body[0] {
		t.Error("the frame after one given back was read into new memory")
	}
	third, err := Read(r, pool)
	if err != nil {
		t.Fatal(err)
	}
	if &third.Body[0] == &second.Body[0] {
		t.Fatal("a frame still held was read into")
	}
	if !bytes.Equal(second.Body, bytes.Repeat([]byte{2}, 1000)) || !bytes.Equal(third.Body, bytes.Repeat([]byte{3}, 1000)) {
		t.Error("a frame read into a body given back did not come whole")
	}
}

// Only frames come from the pool: a failure, an open's answer or a frame
// of another size gets a body of its own, whatever is kept.
func TestOnlyAFrameOfTheSameSizeTakesABodyKept(t *testing.T) {
	pool := NewPool(1 << 20)
	kept := make([]byte, 6)
	pool.Give(kept)
	var b bytes.Buffer
	for _, rec := range []Record{
		{Cursor: 1, Kind: Failed, Body: []byte("broken")},
		{Cursor: 1, Kind: Opened, Body: []byte("file p")},
		{Cursor: 1, Kind: Frame, Body: []byte("seven b")},
		{Cursor: 1, Kind: Frame, Body: []byte("6 byte")},
	} {
		if err := Write(&b, rec); err != nil {
			t.Fatal(err)
		}
	}
	for i := range 3 {
		rec, err := Read(&b, pool)
		if err != nil {
			t.Fatal(err)
		}
		if &rec.Body[0] == &kept[0] {
			t.Errorf("record %d, %q, was read into the frame kept", i, rec.Body)
		}
	}
	rec, err := Read(&b, pool)
	if err != nil {
		t.Fatal(err)
	}
	if &rec.Body[0] != &kept[0] || string(rec.Body) != "6 byte" {
		t.Errorf("the frame of the size kept came in new memory, %q", rec.Body)
	}
}

// The pool keeps no more than its limit, and the frames of a size the
// app no longer asks for go first: the video preview changes size only
// when the app does. A pool that is cleared keeps nothing.
func TestThePoolKeepsItsLimitAndLetsTheOldSizeGoFirst(t *testing.T) {
	pool := NewPool(30)
	small := [][]byte{make([]byte, 10), make([]byte, 10)}
	for _, b := range small {
		pool.Give(b)
	}
	big := [][]byte{make([]byte, 15), make([]byte, 15), make([]byte, 15)}
	for _, b := range big {
		pool.Give(b)
	}
	if got := pool.take(10); got != nil {
		t.Error("a frame of the old size was kept beside two of the new one")
	}
	for i := range 2 {
		if got := pool.take(15); got == nil || &got[0] != &big[1-i][0] {
			t.Errorf("frame %d of the new size was not kept", i)
		}
	}
	if got := pool.take(15); got != nil {
		t.Error("the pool kept more than its limit")
	}
	pool.Give(big[0])
	pool.Clear()
	if got := pool.take(15); got != nil {
		t.Error("a pool cleared kept a frame")
	}
	var none *Pool
	none.Give(big[0])
	if got := none.take(15); got != nil {
		t.Error("no pool kept a frame")
	}
}

// Frames read on one goroutine and given back on several, the way the
// decoder's reader and the page's pulls do, each keep their pixels for as
// long as they are held. A frame read into while held shows as a body
// that is not all its own fill, and as a race.
func TestFramesHeldOnSeveralGoroutinesKeepTheirPixels(t *testing.T) {
	const n, size = 400, 4096
	fills := make([]byte, n)
	for i := range fills {
		fills[i] = byte(i)
	}
	r := frames(t, size, fills...)
	pool := NewPool(8 * size)
	held := make(chan Record, 8)
	var wg sync.WaitGroup
	for range 4 {
		wg.Go(func() {
			for rec := range held {
				fill := byte(rec.At)
				for _, c := range rec.Body {
					if c != fill {
						t.Errorf("frame %v changed while it was held", rec.At)
						break
					}
				}
				pool.Give(rec.Body)
			}
		})
	}
	for range n {
		rec, err := Read(r, pool)
		if err != nil {
			t.Fatal(err)
		}
		held <- rec
	}
	close(held)
	wg.Wait()
}

// Reading a frame of 1600 by 900 through a pipe, as the app reads the
// decoder's, into a body of its own or into the one given back.
func BenchmarkReadingAFrame(b *testing.B) {
	for _, kept := range []struct {
		name string
		pool *Pool
	}{{"new body", nil}, {"body given back", NewPool(64 << 20)}} {
		b.Run(kept.name, func(b *testing.B) {
			body := make([]byte, 1600*900*4)
			r, w := io.Pipe()
			go func() {
				for {
					if Write(w, Record{Cursor: 1, Kind: Frame, Body: body}) != nil {
						return
					}
				}
			}()
			b.SetBytes(int64(len(body)))
			b.ReportAllocs()
			for b.Loop() {
				rec, err := Read(r, kept.pool)
				if err != nil {
					b.Fatal(err)
				}
				kept.pool.Give(rec.Body)
			}
			_ = r.Close()
		})
	}
}
