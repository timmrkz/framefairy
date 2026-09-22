package engine

import (
	"context"
	"fmt"
	"math"
	"sort"
	"strconv"
	"strings"
	"sync"
)

func tailRunes(s string, n int) string {
	r := []rune(s)
	if len(r) <= n {
		return s
	}
	return string(r[len(r)-n:])
}

// DetectShots finds camera switches inside one span, as absolute times.
//
// Only the spans that survived selection get scanned. The switches are hard
// cuts between locked-off cameras, so detection is close to exact.
func (e *Engine) DetectShots(ctx context.Context, path string, start, end float64) ([]float64, error) {
	const threshold = 0.30
	from := math.Max(0, start-0.5)
	args := []string{"-hide_banner",
		"-ss", fixed(from, 3), "-to", fixed(end+0.5, 3),
		"-i", path,
		"-vf", fmt.Sprintf("scale=256:-2,select='gt(scene,%s)',showinfo", pyFloatRepr(threshold)),
		"-an", "-fps_mode", "passthrough", "-f", "null", "-"}
	code, stderr := e.RunFFmpeg(ctx, args, "finding camera switches", end-start+1.0, "")
	if ctx.Err() != nil {
		return nil, ctx.Err()
	}
	if code != 0 {
		return nil, renderErr("shot detection failed:\n%s", tailRunes(strip(stderr), 400))
	}
	seen := map[float64]bool{}
	var cuts []float64
	for _, line := range strings.Split(stderr, "\n") {
		if !strings.Contains(line, "pts_time:") {
			continue
		}
		after := strings.SplitN(line, "pts_time:", 2)[1]
		words := fields(after)
		if len(words) == 0 {
			continue
		}
		value, ok := parsePyFloat(words[0])
		if !ok {
			continue
		}
		absolute := from + value
		if start < absolute && absolute < end {
			key := roundTo(absolute, 3)
			if !seen[key] {
				seen[key] = true
				cuts = append(cuts, key)
			}
		}
	}
	sort.Float64s(cuts)
	return cuts, nil
}

// sampleGray pulls a handful of small greyscale frames from one shot.
func (e *Engine) sampleGray(ctx context.Context, path string, start, duration float64,
	width, height, frames int) [][]byte {
	rate := math.Max(1.0, float64(frames)/math.Max(duration, 0.1))
	raw, _ := runBytes(ctx, "", e.FFmpeg, "-hide_banner", "-loglevel", "error",
		"-ss", fixed(start, 3), "-t", fixed(duration, 3), "-i", path,
		"-vf", fmt.Sprintf("fps=%s,scale=%d:%d,format=gray", fixed(rate, 4), width, height),
		"-frames:v", strconv.Itoa(frames), "-f", "rawvideo", "-")
	size := width * height
	var out [][]byte
	for i := 0; i+size <= len(raw); i += size {
		out = append(out, raw[i:i+size])
	}
	return out
}

// ---------------------------------------------------------------------------
// Framing
//
// The question a crop answers is which of the 9:16 windows available in this
// frame holds the most of whatever the camera was pointed at.
//
// Two things are deliberately not assumed. Nothing about where in the frame a
// subject sits, because that is a choice about how something is shot and it
// can change tomorrow. And nothing about a subject holding still, because a
// person in a chair leans, turns and shifts, and a 9:16 window is narrow
// enough that a lean is the difference between centred and half out of frame.
//
// What is assumed is only that the camera was focused on the thing that
// matters. Detail is therefore the signal. A subject in focus carries fine
// texture, a backdrop carries less, and that holds whether the backdrop is
// black or white and wherever in the frame the subject happens to be.
// ---------------------------------------------------------------------------

// Frame sizes used for framing. Faces are looked for at this size and the
// in-focus fallback works on the same frames.
const (
	FaceWidth  = 480
	FaceHeight = 270
)

// DetailProfile measures how much fine texture each column of the frame
// holds. Local gradient, summed down the column. In focus reads high, out of
// focus reads low, and neither depends on how bright anything is.
func DetailProfile(frame []byte, width, height int) []float64 {
	profile := make([]float64, width)
	for row := 0; row < height-1; row += 2 {
		base := row * width
		below := base + width
		previous := int(frame[base])
		for col := 1; col < width; col++ {
			value := int(frame[base+col])
			profile[col] += float64(absInt(value-previous) + absInt(value-int(frame[below+col])))
			previous = value
		}
	}
	// A little horizontal smoothing, so one noisy column cannot decide where
	// the window goes.
	smoothed := make([]float64, width)
	for col := 0; col < width; col++ {
		low := max(0, col-2)
		high := min(width, col+3)
		smoothed[col] = pysum(profile[low:high]) / float64(high-low)
	}
	return smoothed
}

func absInt(x int) int {
	if x < 0 {
		return -x
	}
	return x
}

// BestWindow says where the window goes, and how clear the answer was.
//
// Two steps, because containing the subject and framing it are not the same
// question. Finding the window that holds the most detail answers the first.
// On a two-shot it settles on one person rather than landing between two and
// showing neither. But holding the most is satisfied by a window shoved to
// one side with the subject against its edge, which is the offset framing
// nobody would choose by hand.
//
// So the window then settles onto the middle of what it found, by repeatedly
// moving to the centre of the detail inside it. Detail is squared first, so a
// concentrated cluster outweighs a broad faint one. A face has far more
// texture per column than a shoulder, but a shoulder is wider, and without
// the weighting the wide faint thing wins on sheer area.
func BestWindow(profile []float64, window int) (float64, float64) {
	const sharpness = 2.0
	width := len(profile)
	window = max(1, min(window, width))
	weighted := make([]float64, width)
	for i, v := range profile {
		weighted[i] = math.Pow(v, sharpness)
	}

	running := pysum(weighted[:window])
	bestSum, bestAt := running, 0
	sums := []float64{running}
	for left := 1; left < width-window+1; left++ {
		running += weighted[left+window-1] - weighted[left-1]
		sums = append(sums, running)
		if running > bestSum {
			bestSum, bestAt = running, left
		}
	}
	if bestSum <= 0 {
		return float64(width-window) / 2, 0
	}

	left := float64(bestAt)
	for step := 0; step < 6; step++ {
		low := int(left)
		high := min(width, int(left)+window)
		section := weighted[low:high]
		mass := pysum(section)
		if mass <= 0 {
			break
		}
		moments := make([]float64, len(section))
		for i, v := range section {
			moments[i] = float64(i) * v
		}
		centre := float64(low) + pysum(moments)/mass
		target := math.Max(0, math.Min(centre-float64(window)/2, float64(width-window)))
		if math.Abs(target-left) < 0.4 {
			left = target
			break
		}
		left = target
	}

	sorted := append([]float64(nil), sums...)
	sort.Float64s(sorted)
	typical := sorted[len(sorted)/2]
	return left, (bestSum - typical) / bestSum
}

type timed struct {
	at    float64
	value float64
}

// samplePositions measures where the window should sit across a shot.
//
// A face is the answer when one can be found, because in an interview the
// subject is a person and their face is the thing a viewer looks at. Detail
// is the fallback for frames where no face is detected, which keeps the tool
// working on footage of something that is not a person.
func (e *Engine) samplePositions(ctx context.Context, path string, start, end float64,
	source SourceInfo, cropW int) []timed {
	const step = 0.4
	duration := math.Max(end-start, 0.05)
	count := max(2, min(int(duration/step)+1, 200))
	frames := e.sampleGray(ctx, path, start, duration, FaceWidth, FaceHeight, count)
	if len(frames) == 0 {
		return nil
	}
	scale := float64(source.Width) / FaceWidth
	limit := float64(source.Width - cropW)
	windowSmall := max(1, pyround(float64(cropW)/float64(source.Width)*FaceWidth))

	detector := e.faceDetector()
	var faces, details []timed
	for index, frame := range frames {
		if len(frame) < FaceWidth*FaceHeight {
			continue
		}
		when := start + duration*float64(index)/float64(max(len(frames)-1, 1))

		if detector != nil {
			if centre, ok := detector.largestFace(frame, FaceWidth, FaceHeight); ok {
				faces = append(faces, timed{when,
					math.Max(0, math.Min((centre-float64(windowSmall)/2)*scale, limit))})
				continue
			}
		}
		profile := DetailProfile(frame, FaceWidth, FaceHeight)
		left, confidence := BestWindow(profile, windowSmall)
		if confidence >= 0.04 {
			details = append(details, timed{when, math.Max(0, math.Min(left*scale, limit))})
		}
	}

	// The two estimators do not agree on where a person is, and they should
	// not, since one points at a face and the other at whatever is in focus.
	// Mixing them in one shot produced a framing that slid across as face
	// detections took over. Whichever answered for most of the shot is used,
	// and the other is discarded rather than averaged in.
	if len(faces) >= max(2, len(frames)/5) {
		e.Log.Detail("shot at %ss: framed on a face (%d/%d frames)",
			fixed(start, 1), len(faces), len(frames))
		return faces
	}
	if len(faces) > 0 {
		e.Log.Detail("shot at %ss: only %d face detections, falling back to "+
			"detail for the whole shot", fixed(start, 1), len(faces))
	}
	return details
}

// settledCrop is one crop for the shot, the middle of where the subject was.
// A still frame with the subject slightly off centre beats a frame that
// slides around behind them. The median keeps a handful of bad measurements
// from pulling the result.
func settledCrop(samples []timed) (float64, bool) {
	if len(samples) == 0 {
		return 0, false
	}
	values := make([]float64, len(samples))
	for i, s := range samples {
		values[i] = s.value
	}
	sort.Float64s(values)
	return values[len(values)/2], true
}

// cropCache is the framing of each shot, measured once and shared by every
// clip that shows it. Clips are framed side by side while the model is still
// writing, so it is locked. Two clips that reach the same shot at the same
// moment both measure it and get the same answer, which costs a little and
// is never wrong.
type cropCache struct {
	mu    sync.Mutex
	crops map[float64]*int
}

func newCropCache() *cropCache { return &cropCache{crops: map[float64]*int{}} }

func (c *cropCache) get(key float64) (*int, bool) {
	c.mu.Lock()
	defer c.mu.Unlock()
	crop, known := c.crops[key]
	return crop, known
}

func (c *cropCache) put(key float64, crop *int) {
	c.mu.Lock()
	defer c.mu.Unlock()
	c.crops[key] = crop
}

// ClipSegments turns a clip's spans into segments, with one crop per camera
// angle.
//
// The crop belongs to the shot, not to the segment. Removing a pause splits a
// span into pieces that are still the same locked-off camera, and measuring
// each piece separately made the framing jump on every internal cut even
// though nothing in the picture had changed. So the shots are found once
// across the whole clip, each piece is assigned to the shot it sits in, and
// every piece of the same shot gets the same crop.
func (e *Engine) ClipSegments(ctx context.Context, path string, spans []Span,
	source SourceInfo, cropW int, cache *cropCache) ([]Segment, error) {
	if len(spans) == 0 {
		return nil, nil
	}
	low, high := spans[0].Start, spans[0].End
	for _, s := range spans {
		low = math.Min(low, s.Start)
		high = math.Max(high, s.End)
	}
	cuts, err := e.DetectShots(ctx, path, low, high)
	if err != nil {
		return nil, err
	}
	bounds := append(append([]float64{low}, cuts...), high)

	var segments []Segment
	for _, span := range spans {
		for i := 0; i+1 < len(bounds); i++ {
			shotStart, shotEnd := bounds[i], bounds[i+1]
			pieceStart := math.Max(span.Start, shotStart)
			pieceEnd := math.Min(span.End, shotEnd)
			if pieceEnd-pieceStart < 0.25 {
				continue
			}
			key := roundTo(shotStart, 1)
			crop, known := cache.get(key)
			if !known {
				// Measured once per shot, not per segment, so removing a
				// pause never changes the framing.
				if ctx.Err() != nil {
					return nil, ctx.Err()
				}
				samples := e.samplePositions(ctx, path, shotStart, shotEnd, source, cropW)
				if settled, ok := settledCrop(samples); ok {
					x := int(settled)
					crop = intPtr(ClampCropX(&x, cropW, source.Width))
				}
				cache.put(key, crop)
			}
			segments = append(segments, Segment{Start: pieceStart, End: pieceEnd, CropX: crop})
		}
	}
	sort.SliceStable(segments, func(i, j int) bool { return segments[i].Start < segments[j].Start })
	return segments, nil
}
