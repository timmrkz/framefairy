package engine

import (
	"bytes"
	"context"
	"fmt"
	"math"
	"sort"
	"strconv"
	"strings"
	"time"

	"framefairy/engine/num"
	"framefairy/engine/runes"
	"framefairy/engine/timed"
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
		args = append(args, "-ss", num.Fixed(at, 3), "-i", path, "-an", "-frames:v", "1", "-f", "null", "-")
		probeCtx, cancel := timeLimit(ctx, frameLimit)
		said := run(probeCtx, "", e.FFmpeg, args...)
		cancel()
		if ctx.Err() != nil {
			return ""
		}
		used = decoderIn(said.Stderr)
		if probeCtx.Err() != nil {
			// A system decoder that does not give one frame in that time
			// has stopped working. VideoToolbox on CI's virtual Mac did,
			// and the framing waited on it until the test run was killed.
			// One that went on even once it was stopped, as it did on the
			// same Mac, is left behind, see leaveAfter.
			e.softDecode.Store(true)
			if said.Left {
				e.Log.Warn("the system's video decoder gave no frame in %s and %s, decoding on the processor from now on",
					frameLimit, leftBehind)
			} else {
				e.Log.Warn("the system's video decoder gave no frame in %s, decoding on the processor from now on", frameLimit)
			}
			used = "the processor"
		}
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
		case listed && line != "" && runes.Len(line) <= 20 && !strings.ContainsAny(line, " :"):
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
	return runes.Scrub(name, 20)
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
	words := runes.Fields(stderr[at+len(marker):])
	if len(words) == 0 {
		return "the processor"
	}
	return DecoderName(words[0])
}

// frameLimit is how long decoding a single frame may take before the
// system's decoder is taken to have stopped working: far past the second
// an honest decode takes, of any file.
var frameLimit = 15 * time.Second

// timeLimit ends work that has run out of its time limit, see frameLimit
// and spanLimit. It is context.WithTimeout unless a test ends the work
// itself, once the stand-in for ffmpeg says it hangs, so no test waits
// for a limit on the real clock.
var timeLimit = context.WithTimeout

// spanLimit is how long reading the frames of a span of so many seconds
// may take before it is ended: a minute, and four times the span on top,
// where an honest run takes a few seconds.
var spanLimit = func(span float64) time.Duration {
	return time.Minute + time.Duration(4*span*float64(time.Second))
}

// frameStep is how far apart the frames the crop is measured on are.
const frameStep = 0.4

// grayFrame is a small grey frame of the episode at a moment of it.
type grayFrame struct {
	at  float64
	pix []byte
}

// readSpan reads the grey frames of one span a clip keeps, FaceWidth by
// FaceHeight, one every frameStep from its start and the one a tenth of a
// second before its end, each with its moment. It is one decode of the
// span.
func (e *Engine) readSpan(ctx context.Context, path string, start, end float64) ([]grayFrame, error) {
	from := math.Max(0, start-0.5)
	last := math.Max(end-0.1, start) - from
	grey := fmt.Sprintf("trim=start=%s:end=%s,"+
		"select='isnan(prev_selected_t)+gte(t-prev_selected_t,%s)+gte(t,%s)*lt(prev_t,%s)',"+
		"scale=%d:%d,format=gray,showinfo@frames",
		num.Fixed(start-from, 6), num.Fixed(end-from, 6), num.Fixed(frameStep-0.001, 3),
		num.Fixed(last, 6), num.Fixed(last, 6), FaceWidth, FaceHeight)
	// A read that runs out of time is ended. ffmpeg has hung here once,
	// decoding through the system's decoder, and a search waited on it for
	// good. It takes seconds, so the limit is far past any honest run.
	limit := spanLimit(end - start)
	timedOut := false
	var raw bytes.Buffer
	read := func(flags []string) (int, string) {
		raw.Reset()
		args := append([]string{"-hide_banner"}, flags...)
		args = append(args, "-ss", num.Fixed(from, 3), "-to", num.Fixed(end+0.5, 3), "-i", path, "-an",
			"-vf", grey, "-fps_mode", "passthrough", "-f", "rawvideo", "pipe:1")
		readCtx, cancel := timeLimit(ctx, limit)
		defer cancel()
		code, stderr := e.runFFmpegTo(readCtx, args, "reading frames for the crop", end-start+1.0, "", &raw)
		timedOut = ctx.Err() == nil && readCtx.Err() != nil
		return code, stderr
	}
	flags := e.decodeFlags()
	code, stderr := read(flags)
	// A system decoder that fails, or that says it worked and gave no
	// frames, is gone round: the same read on the processor, and every
	// read after it.
	if flags != nil && ctx.Err() == nil && (code != 0 || raw.Len() < FaceWidth*FaceHeight) {
		e.softDecode.Store(true)
		if timedOut {
			e.Log.Warn("reading frames for the crop stopped moving with the system's video decoder, decoding on the processor from now on")
		} else {
			e.Log.Detail("the system's video decoder did not take this file, decoding on the processor")
		}
		code, stderr = read(nil)
	}
	if ctx.Err() != nil {
		return nil, ctx.Err()
	}
	if code != 0 && timedOut {
		// The span is then measured a frame at a time, see
		// samplePositions. No clip at all is far worse.
		e.Log.Warn("reading frames for the crop between %s and %s took longer than %s and was ended",
			HMS(start), HMS(end), limit)
		return nil, nil
	}
	if code != 0 {
		return nil, renderErr("reading frames for the crop failed:\n%s", tailRunes(runes.Trim(stderr), 400))
	}
	var times []float64
	for _, line := range strings.Split(stderr, "\n") {
		if !strings.Contains(line, "showinfo@frames") || !strings.Contains(line, "pts_time:") {
			continue
		}
		words := runes.Fields(strings.SplitN(line, "pts_time:", 2)[1])
		if len(words) == 0 {
			continue
		}
		if value, ok := num.Parse(words[0]); ok {
			times = append(times, from+value)
		}
	}
	size := FaceWidth * FaceHeight
	pix := raw.Bytes()
	var frames []grayFrame
	for i, at := range times {
		if (i+1)*size > len(pix) {
			break
		}
		frames = append(frames, grayFrame{at: at, pix: pix[i*size : (i+1)*size]})
	}
	return frames, nil
}

// sampleGray pulls a handful of small greyscale frames from one part.
func (e *Engine) sampleGray(ctx context.Context, path string, start, duration float64,
	width, height, frames int) [][]byte {
	rate := math.Max(1.0, float64(frames)/math.Max(duration, 0.1))
	pull := func(flags []string) []byte {
		args := append([]string{"-hide_banner", "-loglevel", "error"}, flags...)
		args = append(args,
			"-ss", num.Fixed(start, 3), "-t", num.Fixed(duration, 3), "-i", path,
			"-vf", fmt.Sprintf("fps=%s,scale=%d:%d,format=gray", num.Fixed(rate, 4), width, height),
			"-frames:v", strconv.Itoa(frames), "-f", "rawvideo", "-")
		// A system decoder that hangs gives nothing, and is gone round
		// below like one that failed.
		pullCtx, cancel := timeLimit(ctx, time.Duration(frames)*frameLimit)
		defer cancel()
		raw, _ := runBytes(pullCtx, "", e.FFmpeg, args...)
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
		smoothed[col] = num.Sum(profile[low:high]) / float64(high-low)
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

	running := num.Sum(weighted[:window])
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
		mass := num.Sum(section)
		if mass <= 0 {
			break
		}
		moments := make([]float64, len(section))
		for i, v := range section {
			moments[i] = float64(i) * v
		}
		centre := float64(low) + num.Sum(moments)/mass
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

type position struct {
	at    float64
	value float64
}

// samplePositions measures where the window should sit across a clip.
//
// A face is the answer when one can be found, because in an interview the
// subject is a person and their face is the thing a viewer looks at. Detail
// is the fallback for frames where no face is detected, which keeps the tool
// working on footage of something that is not a person.
func (e *Engine) samplePositions(ctx context.Context, path string, parts []timed.Span, read []grayFrame,
	source SourceInfo, cropW int) []position {
	whole := 0.0
	for _, p := range parts {
		whole += math.Max(p.End-p.Start, 0.05)
	}
	if len(parts) == 0 {
		return nil
	}
	start := parts[0].Start
	count := max(2, min(int(whole/frameStep)+1, 200))
	// The frames of every part the clip keeps, read with its span, and
	// nothing from what it leaves out. A part too short to hold one of
	// them is read on its own.
	var frames [][]byte
	var times []float64
	for _, p := range parts {
		got := 0
		for _, f := range read {
			if f.at >= p.Start-0.001 && f.at < p.End {
				frames, times = append(frames, f.pix), append(times, f.at)
				got++
			}
		}
		if got == 0 {
			duration := math.Max(p.End-p.Start, 0.05)
			for _, pix := range e.sampleGray(ctx, path, p.Start, duration, FaceWidth, FaceHeight, 1) {
				frames, times = append(frames, pix), append(times, p.Start)
			}
		}
	}
	// A long clip is measured on as many frames as before, spread over it.
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
	windowSmall := max(1, num.Round(float64(cropW)/float64(source.Width)*FaceWidth))

	detector := e.faceDetector()
	var centres []float64
	var found []bool
	if detector != nil {
		centres, found = detector.faceCentres(frames, FaceWidth, FaceHeight, faceWorkers())
	}
	var faces, details []position
	for index, frame := range frames {
		if len(frame) < FaceWidth*FaceHeight {
			continue
		}
		when := times[index]

		if detector != nil {
			if centre, ok := centres[index], found[index]; ok {
				faces = append(faces, position{when,
					math.Max(0, math.Min((centre-float64(windowSmall)/2)*scale, limit))})
				continue
			}
		}
		profile := DetailProfile(frame, FaceWidth, FaceHeight)
		left, confidence := BestWindow(profile, windowSmall)
		if confidence >= 0.04 {
			details = append(details, position{when, math.Max(0, math.Min(left*scale, limit))})
		}
	}

	// The two estimators do not agree on where a person is, and they should
	// not, since one points at a face and the other at whatever is in focus.
	// Mixing them in one clip produced a framing that slid across as face
	// detections took over. Whichever answered for most of the clip is used,
	// and the other is discarded rather than averaged in.
	if len(faces) >= max(2, len(frames)/5) {
		e.Log.Detail("clip at %ss: framed on a face (%d/%d frames)",
			num.Fixed(start, 1), len(faces), len(frames))
		return faces
	}
	if len(faces) > 0 {
		e.Log.Detail("clip at %ss: only %d face detections, falling back to "+
			"detail for the whole clip", num.Fixed(start, 1), len(faces))
	}
	return details
}

// settledCrop is one crop for the clip, the middle of where the subject was.
// A still frame with the subject slightly off centre beats a frame that
// slides around behind them. The median keeps a handful of bad measurements
// from pulling the result.
func settledCrop(samples []position) (float64, bool) {
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

// ClipSegments turns a clip's spans into segments, all with one crop.
//
// The crop belongs to the clip, not to the segment. Removing a pause splits a
// span into pieces of the same picture, and measuring each piece separately
// made the framing jump on every internal cut even though nothing in the
// picture had changed. So every piece gets the same crop, measured once over
// all of them, and only the keyframes a person sets move it. A clip used to
// be parted at every camera switch, with a crop for each camera, which Tim
// found never showed, #396.
//
// Only what the clip keeps is decoded, each kept span once.
func (e *Engine) ClipSegments(ctx context.Context, path string, spans []timed.Span,
	source SourceInfo, cropW int, work *cropWork) ([]Segment, error) {
	if len(spans) == 0 {
		return nil, nil
	}
	ordered := append([]timed.Span(nil), spans...)
	sort.SliceStable(ordered, func(i, j int) bool { return ordered[i].Start < ordered[j].Start })
	work.start(ordered)
	e.decoderUsed(ctx, path, ordered[0].Start)

	var kept []timed.Span
	var read []grayFrame
	for _, span := range ordered {
		if span.End-span.Start < 0.25 {
			continue
		}
		work.part(span.End - span.Start)
		frames, err := e.readSpan(ctx, path, span.Start, span.End)
		if err != nil {
			return nil, err
		}
		work.partDone()
		kept = append(kept, span)
		read = append(read, frames...)
	}
	if len(kept) == 0 {
		return nil, nil
	}
	whole := 0.0
	for _, span := range kept {
		whole += span.End - span.Start
	}
	work.part(whole)
	if ctx.Err() != nil {
		return nil, ctx.Err()
	}
	var crop *int
	if settled, ok := settledCrop(e.samplePositions(ctx, path, kept, read, source, cropW)); ok {
		x := int(settled)
		crop = intPtr(ClampCropX(&x, cropW, source.Width))
	}
	work.partDone()
	segments := make([]Segment, len(kept))
	for i, span := range kept {
		segments[i] = Segment{Start: span.Start, End: span.End, CropX: crop}
	}
	return segments, nil
}
