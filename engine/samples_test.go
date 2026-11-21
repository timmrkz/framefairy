package engine

import (
	"bytes"
	"context"
	"encoding/binary"
	"io"
	"math"
	"testing"
)

// cutReader hands out what it holds in reads of an odd size, the way a
// pipe hands out ffmpeg's output, so almost every read cuts a sample in
// two.
type cutReader struct {
	data []byte
	at   int
	size int
}

func (c *cutReader) Read(p []byte) (int, error) {
	if c.at >= len(c.data) {
		return 0, io.EOF
	}
	n := copy(p[:min(len(p), c.size)], c.data[c.at:])
	c.at += n
	return n, nil
}

// sound is seconds of a tone as the 16 kHz float samples ffmpeg writes.
func sound(seconds float64) ([]float32, []byte) {
	samples := make([]float32, int(seconds*SampleRate))
	for i := range samples {
		samples[i] = float32(math.Sin(float64(i)/7) * 0.3)
	}
	var raw bytes.Buffer
	_ = binary.Write(&raw, binary.LittleEndian, samples)
	return samples, raw.Bytes()
}

// Reads that cut samples in two, by one, two or three bytes, give back
// every sample whole and in order, and measuring them gives the frames
// the samples give.
func TestSamplesCutBetweenReadsComeBackWhole(t *testing.T) {
	samples, raw := sound(1.5)
	var want []float32
	for at := 0; at < len(samples); at += frameSamples {
		want = append(want, frameDB(samples[at:min(at+frameSamples, len(samples))]))
	}
	for _, size := range []int{1, 2, 3, 5, 6, 7, 4097, 65537, len(raw)} {
		var got []float32
		e := &Engine{}
		end, err := e.readLevels(context.Background(), &cutReader{data: raw, size: size},
			func(f float32) bool { got = append(got, f); return true }, func(bool) bool { return true })
		if err != nil || !end {
			t.Fatal(size, err, end)
		}
		if len(got) != len(want) {
			t.Fatalf("reads of %d bytes: %d frames, want %d", size, len(got), len(want))
		}
		for i := range want {
			if got[i] != want[i] {
				t.Fatalf("reads of %d bytes: frame %d is %v, want %v", size, i, got[i], want[i])
			}
		}

		var back []float32
		r := newSampleReader(&cutReader{data: raw, size: size}, 64)
		for {
			whole, err := r.Read()
			for i := 0; i < len(whole); i += 4 {
				back = append(back, math.Float32frombits(binary.LittleEndian.Uint32(whole[i:])))
			}
			if err != nil {
				break
			}
		}
		if len(back) != len(samples) {
			t.Fatalf("reads of %d bytes: %d samples, want %d", size, len(back), len(samples))
		}
		for i := range samples {
			if back[i] != samples[i] {
				t.Fatalf("reads of %d bytes: sample %d is %v, want %v", size, i, back[i], samples[i])
			}
		}
	}
}

// Ten minutes of sound measured into frames, read the way a pipe hands
// it out, in pieces that cut samples in two.
func BenchmarkMeasureSoundReadInPieces(b *testing.B) {
	_, raw := sound(600)
	e := &Engine{}
	b.SetBytes(int64(len(raw)))
	b.ReportAllocs()
	for b.Loop() {
		e.readLevels(context.Background(), &cutReader{data: raw, size: 65537},
			func(float32) bool { return true }, func(bool) bool { return true })
	}
}
