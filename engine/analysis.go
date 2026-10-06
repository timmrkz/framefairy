package engine

import (
	"bytes"
	"context"
	"fmt"
	"io"
	"math"
	"sort"
	"strconv"
	"strings"
	"sync"
	"time"
)

func tailRunes(s string, n int) string {
	r := []rune(s)
	if len(r) <= n {
		return s
	}
	return string(r[len(r)-n:])
}

// decodeFlags asks ffmpeg to decode on the system's own video decoder
// where there is one. On macOS that is VideoToolbox, which our ffmpeg is
// built with, and it takes the decoding off the processor, which is most of
// what framing costs and most of what spins the fans. A Windows or Linux
// build gets whatever decoder it is built with, and today that is none, in
// which case ffmpeg decodes on the processor by itself. Frames come back to
// memory either way, so the filters after it never know the difference.
//
// If it fails on a machine, for a file or a driver that will not have it,
// the same work is done again on the processor and every decode after it
// goes there straight away.
func (e *Engine) decodeFlags() []string {
	if e.softDecode.Load() {
		return nil
	}
	return []string{"-hwaccel", "auto"}
}

// decoderUsed says which decoder ffmpeg really uses for this file, by
// decoding one frame the way framing does and reading what ffmpeg says
// about it. It is found once per file and written to the log, so a search
// says whether the system's decoder did the work or the processor did.
// "auto" picks the system's decoder without a word unless asked to talk,
// and falls back to the processor for a file it will not take, which is
// why this asks rather than assumes.
func (e *Engine) decoderUsed(ctx context.Context, path string, at float64) string {
	if found, ok := e.decoders.Load(path); ok {
		return found.(string)
	}
	used := "the processor"
	if flags := e.decodeFlags(); flags != nil {
		args := append([]string{"-hide_banner", "-loglevel", "verbose"}, flags...)
		args = append(args, "-ss", fixed(at, 3), "-i", path, "-an", "-frames:v", "1", "-f", "null", "-")
		said := run(ctx, "", e.FFmpeg, args...)
		if ctx.Err() != nil {
			return ""
		}
		used = decoderIn(said.Stderr)
	}
	if _, known := e.decoders.LoadOrStore(path, used); !known {
		e.Log.Info("framing decodes video on %s", used)
	}
	return used
}

// SystemDecoders lists the system video decoders this ffmpeg can hand
// decoding to, as ffmpeg -hwaccels names them. Whether a file really goes
// through one is decided per file, and is in the log of the search.
func (e *Engine) SystemDecoders(ctx context.Context) []string {
	said := run(ctx, "", e.FFmpeg, "-hide_banner", "-hwaccels")
	if said.Code != 0 {
		return nil
	}
	return hwaccelsIn(said.Stdout)
}

func hwaccelsIn(out string) []string {
	var names []string
	listed := false
	for _, line := range strings.Split(out, "\n") {
		line = strings.TrimSpace(line)
		switch {
		case strings.HasPrefix(line, "Hardware acceleration methods"):
			listed = true
		case listed && line != "" && runeLen(line) <= 20 && !strings.ContainsAny(line, " :"):
			names = append(names, line)
		}
	}
	return names
}

// DecoderName is how a system decoder is called where a person reads it.
func DecoderName(name string) string {
	switch name {
	case "videotoolbox":
		return "VideoToolbox"
	case "d3d11va", "d3d12va", "dxva2":
		return "DirectX " + name
	case "vaapi":
		return "VA-API"
	case "cuda":
		return "CUDA"
	}
	return Scrub(name, 20)
}

// decoderIn reads the decoder from what ffmpeg says with -loglevel verbose.
func decoderIn(stderr string) string {
	const marker = "Using auto hwaccel type "
	if strings.Contains(stderr, "Failed setup for format") {
		return "the processor"
	}
	at := strings.Index(stderr, marker)
	if at < 0 {
		return "the processor"
	}
	words := fields(stderr[at+len(marker):])
	if len(words) == 0 {
		return "the processor"
	}
	return DecoderName(words[0])
}

// shotLimit is how long finding the camera switches of a span of so many
// seconds may take before it is ended: a minute, and four times the span on
// top, where an honest run takes a few seconds. Tests make it shorter.
var shotLimit = func(span float64) time.Duration {
	return time.Minute + time.Duration(4*span*float64(time.Second))
}

// DetectShots finds camera switches inside one span, as absolute times.
//
// Only the spans a clip keeps get scanned. The switches are hard cuts
// between locked-off cameras, so detection is close to exact.
func (e *Engine) DetectShots(ctx context.Context, path string, start, end float64) ([]float64, error) {
	read, err := e.readSpan(ctx, path, start, end, false)
	return read.cuts, err
}

// frameStep is how far apart the frames framing measures a shot on are.
const frameStep = 0.4

// grayFrame is a small grey frame of the episode at a moment of it.
type grayFrame struct {
	at  float64
	pix []byte
}

// spanRead is what one read of a span a clip keeps gives its framing: the
// camera switches inside it, and, when asked for, grey frames of
// FaceWidth by FaceHeight, one every frameStep from its start and the one
// a tenth of a second before its end, each with its moment.
//
// It is one decode. The switches, the frames the faces are looked for on
// and the frames either side of a gap, to tell whether a clip comes back to
// the camera it left, were three reads of the same seconds, eleven ffmpeg
// runs for a clip in three pieces that decoded every kept second twice:
// 18.1 s for a clip of 24 s on the cloud machine, 8.7 s as one read.
type spanRead struct {
	cuts   []float64
	frames []grayFrame
}

// readSpan reads one span, see spanRead.
func (e *Engine) readSpan(ctx context.Context, path string, start, end float64, frames bool) (spanRead, error) {
	const threshold = 0.30
	from := math.Max(0, start-0.5)
	scene := fmt.Sprintf("scale=256:-2,select='gt(scene,%s)',showinfo", pyFloatRepr(threshold))
	// The frames are cut to the span, one every frameStep from its
	// start, and the first at a tenth of a second before its end, the
	// frame the clip leaves the camera on.
	last := math.Max(end-0.1, start) - from
	grey := fmt.Sprintf("trim=start=%s:end=%s,"+
		"select='isnan(prev_selected_t)+gte(t-prev_selected_t,%s)+gte(t,%s)*lt(prev_t,%s)',"+
		"scale=%d:%d,format=gray,showinfo@frames",
		fixed(start-from, 6), fixed(end-from, 6), fixed(frameStep-0.001, 3),
		fixed(last, 6), fixed(last, 6), FaceWidth, FaceHeight)
	// A scan that runs out of time is ended. ffmpeg has hung here once,
	// decoding through the system's decoder, and a search waited on it for
	// good. It takes seconds, so the limit is far past any honest run.
	limit := shotLimit(end - start)
	timedOut := false
	var raw bytes.Buffer
	scan := func(flags []string) (int, string) {
		raw.Reset()
		args := append([]string{"-hide_banner"}, flags...)
		args = append(args, "-ss", fixed(from, 3), "-to", fixed(end+0.5, 3), "-i", path, "-an")
		var out io.Writer
		if frames {
			// The switches go out with the first frame read as well,
			// which lies before the span and is never taken for one. An
			// output that gets nothing fails the whole run from ffmpeg 9
			// on, and a span with no switch gave this one nothing.
			cuts := fmt.Sprintf("scale=256:-2,select='gt(scene,%s)+eq(n,0)',showinfo", pyFloatRepr(threshold))
			args = append(args,
				"-filter_complex", "[0:v]split=2[s][f];[s]"+cuts+"[cuts];[f]"+grey+"[frames]",
				"-map", "[cuts]", "-fps_mode", "passthrough", "-f", "null", "-",
				"-map", "[frames]", "-fps_mode", "passthrough", "-f", "rawvideo", "pipe:1")
			out = &raw
		} else {
			args = append(args, "-vf", scene, "-fps_mode", "passthrough", "-f", "null", "-")
		}
		scanCtx, cancel := context.WithTimeout(ctx, limit)
		defer cancel()
		code, stderr := e.runFFmpegTo(scanCtx, args, "finding camera switches", end-start+1.0, "", out)
		timedOut = ctx.Err() == nil && scanCtx.Err() != nil
		return code, stderr
	}
	flags := e.decodeFlags()
	code, stderr := scan(flags)
	// A system decoder that fails, or that says it worked and gave no
	// frames, is gone round: the same read on the processor, and every
	// read after it.
	if flags != nil && ctx.Err() == nil && (code != 0 || frames && raw.Len() < FaceWidth*FaceHeight) {
		e.softDecode.Store(true)
		if timedOut {
			e.Log.Warn("finding camera switches stopped moving with the system's video decoder, decoding on the processor from now on")
		} else {
			e.Log.Detail("the system's video decoder did not take this file, decoding on the processor")
		}
		code, stderr = scan(nil)
	}
	if ctx.Err() != nil {
		return spanRead{}, ctx.Err()
	}
	if code != 0 && timedOut {
		// No switches found is a crop that does not follow a camera change.
		// No clip at all is far worse.
		e.Log.Warn("finding camera switches between %s and %s took longer than %s and was ended, so none are used there",
			HMS(start), HMS(end), limit)
		return spanRead{}, nil
	}
	if code != 0 {
		return spanRead{}, renderErr("shot detection failed:\n%s", tailRunes(strip(stderr), 400))
	}
	var read spanRead
	seen := map[float64]bool{}
	var times []float64
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
		if strings.Contains(line, "showinfo@frames") {
			times = append(times, absolute)
			continue
		}
		if start < absolute && absolute < end {
			key := roundTo(absolute, 3)
			if !seen[key] {
				seen[key] = true
				read.cuts = append(read.cuts, key)
			}
		}
	}
	sort.Float64s(read.cuts)
	size := FaceWidth * FaceHeight
	pix := raw.Bytes()
	for i, at := range times {
		if (i+1)*size > len(pix) {
			break
		}
		read.frames = append(read.frames, grayFrame{at: at, pix: pix[i*size : (i+1)*size]})
	}
	return read, nil
}

// sampleGray pulls a handful of small greyscale frames from one part.
func (e *Engine) sampleGray(ctx context.Context, path string, start, duration float64,
	width, height, frames int) [][]byte {
	rate := math.Max(1.0, float64(frames)/math.Max(duration, 0.1))
	pull := func(flags []string) []byte {
		args := append([]string{"-hide_banner", "-loglevel", "error"}, flags...)
		args = append(args,
			"-ss", fixed(start, 3), "-t", fixed(duration, 3), "-i", path,
			"-vf", fmt.Sprintf("fps=%s,scale=%d:%d,format=gray", fixed(rate, 4), width, height),
			"-frames:v", strconv.Itoa(frames), "-f", "rawvideo", "-")
		raw, _ := runBytes(ctx, "", e.FFmpeg, args...)
		return raw
	}
	flags := e.decodeFlags()
	raw := pull(flags)
	size := width * height
	if len(raw) < size && flags != nil && ctx.Err() == nil {
		if raw = pull(nil); len(raw) >= size {
			e.softDecode.Store(true)
		}
	}
	var out [][]byte
	for i := 0; i+size <= len(raw); i += size {
		out = append(out, raw[i:i+size])
	}
	return out
}

// shotChange is how different two frames have to be to be two cameras: a
// mean difference of 30 in 255. It is the same measure and the same line
// as the 0.30 the scene filter is given in DetectShots.
const shotChange = 30.0

// frameDifference is the mean absolute difference of two grey frames of the
// same size, from 0 to 255.
func frameDifference(a, b []byte) float64 {
	n := min(len(a), len(b))
	if n == 0 {
		return 255
	}
	total := 0
	for i := range n {
		d := int(a[i]) - int(b[i])
		if d < 0 {
			d = -d
		}
		total += d
	}
	return float64(total) / float64(n)
}

// sameShot says whether a clip that leaves the episode at one moment and
// comes back at another comes back to the same camera. What was cut out in
// between does not matter to the clip, only the frame it leaves on and the
// frame it comes back to. Two frames it cannot read are taken as two shots:
// a shot measured on its own is never framed for the wrong camera.
func (e *Engine) sameShot(ctx context.Context, path string, leave, back float64) bool {
	const w, h = 128, 72
	before := e.sampleGray(ctx, path, math.Max(leave-0.1, 0), 0.1, w, h, 1)
	after := e.sampleGray(ctx, path, back, 0.1, w, h, 1)
	if len(before) == 0 || len(after) == 0 {
		return false
	}
	return frameDifference(before[0], after[0]) < shotChange
}

// comesBack is sameShot for two spans already read: the frame the first
// leaves on, a tenth of a second before its end, against the first frame
// of the second, each made smaller by four the way sameShot compares them.
// Spans read without those frames are asked of sameShot.
func (e *Engine) comesBack(ctx context.Context, path string, left, came spanRead, leave, back float64) bool {
	var before, after []byte
	for _, f := range left.frames {
		if f.at >= leave-0.1-0.001 {
			before = f.pix
			break
		}
	}
	if len(came.frames) > 0 && came.frames[0].at < back+frameStep {
		after = came.frames[0].pix
	}
	if before == nil || after == nil {
		return e.sameShot(ctx, path, leave, back)
	}
	return frameDifference(quarter(before), quarter(after)) < shotChange
}

// quarter is a grey frame of FaceWidth by FaceHeight made four times
// smaller each way, each pixel the mean of the sixteen it covers.
func quarter(pix []byte) []byte {
	w, h := FaceWidth/4, FaceHeight/4
	out := make([]byte, w*h)
	for y := range h {
		for x := range w {
			sum := 0
			for dy := range 4 {
				row := (y*4 + dy) * FaceWidth
				for dx := range 4 {
					sum += int(pix[row+x*4+dx])
				}
			}
			out[y*w+x] = byte(sum / 16)
		}
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
func (e *Engine) samplePositions(ctx context.Context, path string, parts []Span, reads []spanRead,
	source SourceInfo, cropW int) []timed {
	whole := 0.0
	for _, p := range parts {
		whole += math.Max(p.End-p.Start, 0.05)
	}
	if len(parts) == 0 {
		return nil
	}
	start := parts[0].Start
	count := max(2, min(int(whole/frameStep)+1, 200))
	// The frames of every part of the shot the clip keeps, read with its
	// span, and nothing from what it leaves out. A part too short to hold
	// one of them is read on its own.
	var frames [][]byte
	var times []float64
	for _, p := range parts {
		got := 0
		for _, r := range reads {
			for _, f := range r.frames {
				if f.at >= p.Start-0.001 && f.at < p.End {
					frames, times = append(frames, f.pix), append(times, f.at)
					got++
				}
			}
		}
		if got == 0 {
			duration := math.Max(p.End-p.Start, 0.05)
			for _, pix := range e.sampleGray(ctx, path, p.Start, duration, FaceWidth, FaceHeight, 1) {
				frames, times = append(frames, pix), append(times, p.Start)
			}
		}
	}
	// A long shot is measured on as many frames as before, spread over it.
	if len(frames) > count {
		keptFrames, keptTimes := make([][]byte, 0, count), make([]float64, 0, count)
		for i := range count {
			k := i * (len(frames) - 1) / (count - 1)
			keptFrames, keptTimes = append(keptFrames, frames[k]), append(keptTimes, times[k])
		}
		frames, times = keptFrames, keptTimes
	}
	if len(frames) == 0 {
		return nil
	}
	scale := float64(source.Width) / FaceWidth
	limit := float64(source.Width - cropW)
	windowSmall := max(1, pyround(float64(cropW)/float64(source.Width)*FaceWidth))

	detector := e.faceDetector()
	var centres []float64
	var found []bool
	if detector != nil {
		centres, found = detector.faceCentres(frames, FaceWidth, FaceHeight, faceWorkers())
	}
	var faces, details []timed
	for index, frame := range frames {
		if len(frame) < FaceWidth*FaceHeight {
			continue
		}
		when := times[index]

		if detector != nil {
			if centre, ok := centres[index], found[index]; ok {
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
// though nothing in the picture had changed. So every piece of the same shot
// gets the same crop, measured once over all of them.
//
// Only what the clip keeps is decoded. Each kept span is searched for camera
// switches on its own, and where the clip leaves the episode and comes back,
// the frame it leaves on is compared with the frame it comes back to. The
// material cut out in between was decoded twice over before, once to find
// the switches in it and once for the framing, for a question it cannot
// answer better than those two frames: whether the clip comes back to the
// camera it left.
func (e *Engine) ClipSegments(ctx context.Context, path string, spans []Span,
	source SourceInfo, cropW int, cache *cropCache, work *cropWork) ([]Segment, error) {
	if len(spans) == 0 {
		return nil, nil
	}
	ordered := append([]Span(nil), spans...)
	sort.SliceStable(ordered, func(i, j int) bool { return ordered[i].Start < ordered[j].Start })
	work.start(ordered)
	e.decoderUsed(ctx, path, ordered[0].Start)

	type shot struct {
		key   float64
		parts []Span
	}
	var shots []*shot
	reads := make([]spanRead, len(ordered))
	for i, span := range ordered {
		work.part(span.End - span.Start)
		read, err := e.readSpan(ctx, path, span.Start, span.End, true)
		if err != nil {
			return nil, err
		}
		reads[i] = read
		cuts := read.cuts
		// Coming back from what was cut out, to the camera it left or to
		// another one.
		back := i > 0 && len(shots) > 0 &&
			e.comesBack(ctx, path, reads[i-1], read, ordered[i-1].End, span.Start)
		if ctx.Err() != nil {
			return nil, ctx.Err()
		}
		work.partDone()
		bounds := append(append([]float64{span.Start}, cuts...), span.End)
		for j := 0; j+1 < len(bounds); j++ {
			part := Span{bounds[j], bounds[j+1]}
			if part.End-part.Start < 0.25 {
				continue
			}
			if j == 0 && back {
				last := shots[len(shots)-1]
				last.parts = append(last.parts, part)
				continue
			}
			shots = append(shots, &shot{key: roundTo(part.Start, 1), parts: []Span{part}})
		}
	}

	var segments []Segment
	for _, s := range shots {
		kept := 0.0
		for _, part := range s.parts {
			kept += part.End - part.Start
		}
		work.part(kept)
		crop, known := cache.get(s.key)
		if !known {
			// Measured once per shot, not per piece, so removing a pause
			// never changes the framing.
			if ctx.Err() != nil {
				return nil, ctx.Err()
			}
			samples := e.samplePositions(ctx, path, s.parts, reads, source, cropW)
			if settled, ok := settledCrop(samples); ok {
				x := int(settled)
				crop = intPtr(ClampCropX(&x, cropW, source.Width))
			}
			cache.put(s.key, crop)
		}
		work.partDone()
		for _, part := range s.parts {
			segments = append(segments, Segment{Start: part.Start, End: part.End, CropX: crop})
		}
	}
	sort.SliceStable(segments, func(i, j int) bool { return segments[i].Start < segments[j].Start })
	return segments, nil
}
