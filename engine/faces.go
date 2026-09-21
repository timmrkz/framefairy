package engine

// Face detection without OpenCV.
//
// A native app cannot ask its users to install OpenCV, and binding it from Go
// needs its own native build on every platform. This is a
// pure Go pixel-intensity-comparison detector instead, adapted from pigo by
// Endre Simo (MIT licence, see docs/THIRD_PARTY.md), which is itself based on the
// pico detector by Nenad Markus. Only the upright detector is kept. The
// cascade file is embedded, so there is nothing to install.
//
// The cascade is a frontal detector, so faces turned well away from the lens
// fall back to the in-focus method.

import (
	_ "embed"
	"encoding/binary"
	"math"
	"sort"
)

//go:embed facefinder
var facefinderCascade []byte

type faceDetector struct {
	depth     int
	trees     int
	codes     []int8
	preds     []float32
	threshold []float32
}

func loadFaceDetector(packet []byte) (*faceDetector, bool) {
	if len(packet) < 16 {
		return nil, false
	}
	pos := 8
	depth := int(binary.LittleEndian.Uint32(packet[pos:]))
	pos += 4
	trees := int(binary.LittleEndian.Uint32(packet[pos:]))
	pos += 4
	if depth <= 0 || depth > 16 || trees <= 0 {
		return nil, false
	}
	leaves := 1 << depth
	codeLen := 4*leaves - 4
	d := &faceDetector{depth: depth, trees: trees}
	for t := 0; t < trees; t++ {
		if pos+codeLen+4*leaves+4 > len(packet) {
			return nil, false
		}
		d.codes = append(d.codes, 0, 0, 0, 0)
		for _, b := range packet[pos : pos+codeLen] {
			d.codes = append(d.codes, int8(b))
		}
		pos += codeLen
		for i := 0; i < leaves; i++ {
			d.preds = append(d.preds, math.Float32frombits(binary.LittleEndian.Uint32(packet[pos:])))
			pos += 4
		}
		d.threshold = append(d.threshold, math.Float32frombits(binary.LittleEndian.Uint32(packet[pos:])))
		pos += 4
	}
	return d, true
}

// faceDetector returns the engine's detector, loading it on first use. Nil
// means face detection is off.
func (e *Engine) faceDetector() *faceDetector {
	e.mu.Lock()
	defer e.mu.Unlock()
	if e.NoFaces {
		return nil
	}
	if !e.facesLoaded {
		e.facesLoaded = true
		detector, ok := loadFaceDetector(facefinderCascade)
		if ok {
			e.faces = detector
		} else {
			e.Log.Warn("the face detection data could not be read, so framing " +
				"falls back to finding the in-focus subject")
		}
	}
	return e.faces
}

func (d *faceDetector) classify(r, c, s int, pixels []byte, cols int) float32 {
	leaves := 1 << d.depth
	r *= 256
	c *= 256
	root := 0
	var out float32
	for i := 0; i < d.trees; i++ {
		idx := 1
		for j := 0; j < d.depth; j++ {
			x1 := ((r+int(d.codes[root+4*idx])*s)>>8)*cols + ((c + int(d.codes[root+4*idx+1])*s) >> 8)
			x2 := ((r+int(d.codes[root+4*idx+2])*s)>>8)*cols + ((c + int(d.codes[root+4*idx+3])*s) >> 8)
			bit := 0
			if pixels[x1] <= pixels[x2] {
				bit = 1
			}
			idx = 2*idx + bit
		}
		out += d.preds[leaves*i+idx-leaves]
		if out <= d.threshold[i] {
			return -1
		}
		root += 4 * leaves
	}
	return out - d.threshold[d.trees-1]
}

type faceHit struct {
	row, col, scale int
	q               float32
}

func (d *faceDetector) scan(pixels []byte, rows, cols, minSize, maxSize int) []faceHit {
	const shift, grow = 0.1, 1.1
	var hits []faceHit
	for scale := minSize; scale <= maxSize; {
		step := int(math.Max(shift*float64(scale), 1))
		offset := scale/2 + 1
		for row := offset; row <= rows-offset; row += step {
			for col := offset; col <= cols-offset; col += step {
				if q := d.classify(row, col, scale, pixels, cols); q > 0 {
					hits = append(hits, faceHit{row, col, scale, q})
				}
			}
		}
		scale = int(float64(scale) + math.Max(2, float64(float64(scale)*grow)-float64(scale)))
	}
	return hits
}

func cluster(hits []faceHit, iouThreshold float64) []faceHit {
	sort.SliceStable(hits, func(i, j int) bool { return hits[i].q < hits[j].q })
	iou := func(a, b faceHit) float64 {
		r1, c1, s1 := float64(a.row), float64(a.col), float64(a.scale)
		r2, c2, s2 := float64(b.row), float64(b.col), float64(b.scale)
		overRow := math.Max(0, math.Min(r1+s1/2, r2+s2/2)-math.Max(r1-s1/2, r2-s2/2))
		overCol := math.Max(0, math.Min(c1+s1/2, c2+s2/2)-math.Max(c1-s1/2, c2-s2/2))
		return overRow * overCol / (s1*s1 + s2*s2 - overRow*overCol)
	}
	assigned := make([]bool, len(hits))
	var clusters []faceHit
	for i := range hits {
		if assigned[i] {
			continue
		}
		var r, c, s, n int
		var q float32
		for j := range hits {
			if iou(hits[i], hits[j]) > iouThreshold {
				assigned[j] = true
				r += hits[j].row
				c += hits[j].col
				s += hits[j].scale
				q += hits[j].q
				n++
			}
		}
		if n > 0 {
			clusters = append(clusters, faceHit{r / n, c / n, s / n, q})
		}
	}
	return clusters
}

// equalise stretches the histogram the way OpenCV's equalizeHist does.
func equalise(frame []byte) []byte {
	var hist [256]int
	for _, v := range frame {
		hist[v]++
	}
	first := 0
	for first < 255 && hist[first] == 0 {
		first++
	}
	total := len(frame)
	out := make([]byte, len(frame))
	if hist[first] == total {
		copy(out, frame)
		return out
	}
	scale := 255.0 / float64(total-hist[first])
	var lut [256]byte
	sum := 0
	for i := first + 1; i < 256; i++ {
		sum += hist[i]
		lut[i] = byte(min(255, int(math.Round(float64(sum)*scale))))
	}
	for i, v := range frame {
		out[i] = lut[v]
	}
	return out
}

// largestFace finds the face to frame on, the largest one, and returns its
// centre column in frame coordinates.
func (d *faceDetector) largestFace(frame []byte, width, height int) (float64, bool) {
	if len(frame) < width*height {
		return 0, false
	}
	frame = frame[:width*height]
	// Both the frame as shot and a contrast-stretched copy. Equalising helps
	// a flat or dim frame and actively destroys a dark one, so both are
	// searched and whatever either finds is kept.
	variants := [][]byte{frame}
	sum := 0
	for _, v := range frame {
		sum += int(v)
	}
	if float64(sum)/float64(len(frame)) > 45 {
		variants = append(variants, equalise(frame))
	}
	minSize := max(int(float64(height)*0.07), 8)
	bestArea, bestCentre, found := -1, 0.0, false
	for _, pixels := range variants {
		for _, hit := range cluster(d.scan(pixels, height, width, minSize, height), 0.2) {
			if hit.q < 5.0 {
				continue
			}
			if area := hit.scale * hit.scale; area > bestArea {
				bestArea, bestCentre, found = area, float64(hit.col), true
			}
		}
	}
	return bestCentre, found
}
