package engine

import (
	"context"
	"fmt"
	"math"
	"os"
	"path/filepath"
	"regexp"
	"strconv"
	"strings"

	"framefairy/engine/pynum"
	"framefairy/engine/pytext"

	"framefairy/internal/framewire"
)

// Fade is the audio fade at each cut and at the clip's two ends, in
// seconds. It kills the click a hard audio cut makes.
const Fade = 0.015

// soundLead is how much earlier than where it is wanted sound is read
// after a seek, in microseconds, which atrim then cuts off by its
// timestamps. A render reads each piece's sound this early, and so does a
// reading of the episode's audio for the loudness and the transcription,
// see audioFrom, which counts it in frames as levelsLead.
//
// After a seek, ffmpeg 8.1 kept the part of the first audio packet after
// the seek point, and 9.0 drops that packet whole, up to 21 ms of 48 kHz
// AAC. The timestamps stay right, but a sound made to start at its first
// sample, asetpts=PTS-STARTPTS, then came early against the picture, by
// a different amount at every cut. Read from earlier and cut by its
// timestamps before anything else, it lands on the sample in both. A
// fifth of a second is longer than any packet of sound and the decoder's
// start after a seek.
const soundLead = 200_000

// RenderSettings are the encoder choices for one run.
type RenderSettings struct {
	OutW, OutH   int
	CRF          int
	Preset       string
	AudioBitrate string
	ScaleUp      bool
	ScaleFlags   string
	// Rate is the frames a second of the short, which the render sets
	// from the video, for an encoder asked for a bit rate rather than a
	// quality, see onProcessor.
	Rate float64
}

// RenderSettings are the choices a render makes with these options. Whatever
// is held to what a render makes takes them from here, the video preview's
// captions among them, see Engine.PreviewCaptions.
func (o Options) RenderSettings() RenderSettings {
	rs := RenderSettings{OutW: o.Width, OutH: o.Height, CRF: o.CRF,
		Preset: o.Preset, AudioBitrate: o.AudioBitrate, ScaleUp: !o.NoUpscale}
	if o.Preview {
		rs.OutW, rs.OutH = o.Width/2, o.Height/2
		rs.CRF, rs.Preset = 30, "veryfast"
	}
	return rs
}

// FrameSize is the size of the short's frames for a piece: the size asked
// for, or the crop's own where the render does not make it larger. The
// captions are laid out at the size asked for either way, and libass
// draws them on frames of this size.
func (rs RenderSettings) FrameSize(source SourceInfo, seg Segment) (int, int) {
	if rs.ScaleUp {
		return rs.OutW, rs.OutH
	}
	return CropWindow(source.Rotated(seg.Rotate), rs.OutW, rs.OutH)
}

// BuildFilterGraph makes one filter chain per segment, each reading its own
// seeked input.
//
// Every segment is a separate -i with its own -ss, so ffmpeg seeks to a
// key frame near the segment and decodes only what the clip needs, instead of
// decoding the episode from frame zero. A segment has two inputs, its
// picture from input 2i and its sound from input 2i+1, see soundLead.
func (e *Engine) BuildFilterGraph(ctx context.Context, clip Clip, source SourceInfo,
	rs RenderSettings, assName string) (graph, videoLabel, audioLabel string, err error) {
	flags := rs.ScaleFlags
	if flags == "" {
		flags = "lanczos"
	}
	var parts []string
	for i, seg := range clip.Segments {
		cut := source.cutOf(seg)
		// A rotated piece is rotated first, and its crop is placed on the
		// picture as it stands then.
		upright := source.Rotated(seg.Rotate)
		cropW, cropH := CropWindow(upright, rs.OutW, rs.OutH)
		cropY := (upright.Height - cropH) / 2
		cropY -= cropY % 2
		if len(seg.Marks) > 0 {
			e.Log.Detail("clip %s, piece %d at %ss: the crop glides over the marks %s",
				clip.ID, i+1, pynum.Fixed(seg.Start, 3), marksText(seg.Marks))
		}

		chain := []string{cut.picture + "setpts=PTS-STARTPTS"}
		if rotate := framewire.Rotate(seg.Rotate); rotate != "" {
			chain = append(chain, rotate)
		}
		// Standard video in 8 bits, HDR in 10, each with the episode's own
		// colour tags on every frame, see colourParams.
		pixels, deep := "yuv420p", "yuv444p"
		if source.Light() != framewire.SDR {
			pixels, deep = "yuv420p10le", "yuv444p10le"
		}
		outW, outH := rs.FrameSize(source, seg)
		chain = append(chain, source.cropFilters(seg, cropW, cropH, cropY, upright.Width, outW, outH, "flags="+flags, deep)...)
		chain = append(chain, "fps="+source.FPSString(), "format="+pixels, "setsar=1")
		if tags := source.colourParams(); tags != "" {
			chain = append(chain, tags)
		}
		parts = append(parts, fmt.Sprintf("[%d:v]%s[v%d];", 2*i, strings.Join(chain, ","), i))

		// A piece that runs straight on from the one before, the way a
		// clip is parted where a part of it is rotated, is heard straight
		// on.
		// Nothing is cut there, and a fade out and in would be a dip in
		// the sound of 30 ms.
		fadeOutAt := math.Max(0, seg.Duration()-Fade)
		achain := []string{
			cut.sound,
			"aformat=sample_fmts=fltp:sample_rates=48000:channel_layouts=stereo",
		}
		if i == 0 || seg.Start-clip.Segments[i-1].End > PiecesMeet {
			achain = append(achain, fmt.Sprintf("afade=t=in:st=0:d=%s", pynum.FloatRepr(Fade)))
		}
		if i+1 == len(clip.Segments) || clip.Segments[i+1].Start-seg.End > PiecesMeet {
			achain = append(achain, fmt.Sprintf("afade=t=out:st=%s:d=%s", pynum.Fixed(fadeOutAt, 4), pynum.FloatRepr(Fade)))
		}
		parts = append(parts, fmt.Sprintf("[%d:a]%s[a%d];", 2*i+1, strings.Join(achain, ","), i))
	}

	n := len(clip.Segments)
	if n == 1 {
		videoLabel, audioLabel = "[v0]", "[a0]"
	} else {
		var pairs strings.Builder
		for i := 0; i < n; i++ {
			fmt.Fprintf(&pairs, "[v%d][a%d]", i, i)
		}
		parts = append(parts, fmt.Sprintf("%sconcat=n=%d:v=1:a=1[vcat][acat];", pairs.String(), n))
		videoLabel, audioLabel = "[vcat]", "[acat]"
	}

	if assName != "" {
		template, err := e.SubtitleFilter(ctx)
		if err != nil {
			return "", "", "", err
		}
		// The subtitle filter hands libass the frame's time in whole
		// milliseconds, worked out in floating point and cut down rather
		// than rounded (vf_subtitles.c). After concat the clock counts in
		// microseconds, and 200000 of them come out as 199.999... ms, so
		// a caption or a highlight due on the frame at 0.20 s showed on
		// the frame after it. That was one frame boundary in three. The
		// clock is put in microseconds for every clip and moved on half a
		// millisecond while libass reads it, then back, so every frame
		// reads as the millisecond it is.
		parts = append(parts, videoLabel+"settb=1/1000000,setpts=PTS+500,"+
			subtitleChain(template, assName)+",setpts=PTS-500[vout];")
		videoLabel = "[vout]"
	}
	graph = strings.TrimRight(strings.Join(parts, ""), ";")
	return graph, videoLabel, audioLabel, nil
}

// BuildCommand is the whole ffmpeg invocation for one clip, with the
// encoder a render starts with.
func (e *Engine) BuildCommand(ctx context.Context, clip Clip, sourcePath string,
	source SourceInfo, outPath string, rs RenderSettings, assName string) ([]string, error) {
	encoder, err := e.encoderFor(ctx, source.Light())
	if err != nil {
		return nil, err
	}
	return e.buildCommand(ctx, clip, sourcePath, source, outPath, rs, assName, encoder)
}

// buildCommand is BuildCommand with the encoder said.
func (e *Engine) buildCommand(ctx context.Context, clip Clip, sourcePath string,
	source SourceInfo, outPath string, rs RenderSettings, assName string, encoder Encoder) ([]string, error) {
	graph, videoLabel, audioLabel, err := e.BuildFilterGraph(ctx, clip, source, rs, assName)
	if err != nil {
		return nil, err
	}
	cmd := append([]string{e.FFmpeg, "-hide_banner", "-loglevel", "error", "-stats", "-y"},
		pieceInputs(clip, sourcePath, source)...)
	light := source.Light()
	cmd = append(cmd,
		"-filter_complex", graph,
		"-map", videoLabel, "-map", audioLabel,
	)
	rs.Rate = source.FPS()
	cmd = append(cmd, encoder.args(rs)...)
	if light == framewire.SDR {
		cmd = append(cmd, "-profile:v", "high", "-pix_fmt", "yuv420p")
	}
	cmd = append(cmd,
		"-fps_mode", "cfr",
		"-r", source.FPSString(),
		"-c:a", "aac", "-b:a", rs.AudioBitrate, "-ar", "48000",
		"-movflags", "+faststart",
	)
	for _, tag := range source.Colour {
		cmd = append(cmd, "-"+tag[0], tag[1])
	}
	return append(cmd, outPath), nil
}

// pieceInputs are the inputs of BuildFilterGraph's graph, each piece's
// picture and then its sound, read from the episode at sourcePath.
func pieceInputs(clip Clip, sourcePath string, source SourceInfo) []string {
	abs, err := filepath.Abs(sourcePath)
	if err != nil {
		abs = sourcePath
	}
	var args []string
	if source.ownTimes() {
		args = append(args, "-copyts")
	}
	for _, seg := range clip.Segments {
		// -ss before -i seeks rather than decoding up to the point, and -t
		// limits how much is read. Both are input options on purpose. The
		// sound is read again from a little earlier, see soundLead.
		cut := source.cutOf(seg)
		if cut.fromKey {
			args = append(args, "-noaccurate_seek", "-ss", cut.seek, "-an", "-i", abs)
		} else {
			args = append(args, "-ss", cut.seek, "-t", cut.read, "-an", "-i", abs)
		}
		if cut.soundSeek != "" {
			args = append(args, "-ss", cut.soundSeek)
		}
		args = append(args, "-t", cut.soundRead, "-vn", "-i", abs)
	}
	return args
}

// pieceCut is how one piece of a clip is read from the episode: where the
// read starts and how long it runs, as ffmpeg takes them, once for the
// picture and once for the sound, and the filters that keep what belongs
// to the piece out of what was read. A read with no length runs until its
// filters have what they keep.
type pieceCut struct {
	seek, read           string
	soundSeek, soundRead string
	picture, sound       string
	// Whether the picture is read from the key frame at or before seek,
	// every frame of it, rather than from seek.
	fromKey bool
	// seek and read in microseconds, read nought where there is none.
	// seekUS is where the picture's times count from, which is the seek
	// but for a read moved back to a key frame, see readFrom.
	seekUS, readUS int64
	// soundSeek in microseconds, nought where there is none.
	soundSeekUS int64
}

// readFrom is the cut with its picture read from a key frame keyedPieces
// found before the piece, in a file with no index of its key frames: read
// from at, in microseconds after the start of the file, without dropping
// a frame at the seek. The frames keep the file's own times, see
// onOwnClock, so whichever frame the decoder starts from, each is where
// it is in the file, and the cut keeps them by its own rule: from its
// seek on where a seek would have dropped those before it, and the frame
// that holds its first moment where it reads from a key frame anyway.
func (c pieceCut) readFrom(at int64) pieceCut {
	if at > c.seekUS {
		return c
	}
	if !c.fromKey {
		c.picture = "trim=start=0," + c.picture
	}
	c.fromKey = true
	c.seek = micros(at)
	c.read, c.readUS = "", 0
	return c
}

// onOwnClock is the cut of a file read with its own times, see ownTimes,
// with each picture and sound counted from its seek again, the way ffmpeg
// counts them without -copyts.
func (s SourceInfo) onOwnClock(c pieceCut) pieceCut {
	origin := int64(math.Round(s.origin * 1_000_000))
	c.picture = fmt.Sprintf("settb=1/1000000,setpts=PTS%+d,", -origin-c.seekUS) + c.picture
	c.sound = fmt.Sprintf("asetpts=PTS%s/TB,", signedMicros(-origin-c.soundSeekUS)) + c.sound
	return c
}

// ownTimes tells whether a file is read with its own times, -copyts,
// rather than counted from each seek by ffmpeg: every file whose clock
// Probe read. Counted by ffmpeg, the times after a seek were moved by the
// seek and the start of the file rounded to the stream's ticks, see
// byTimes, and in a file with no index of its key frames, see unindexed,
// by the seek alone in one build and by both in another, and a frame with
// no stamp of its own got one worked out from where the decoder started.
// Read with the file's own times, every frame of a seek has the time it
// has when the file is played from the start, #602.
func (s SourceInfo) ownTimes() bool {
	return s.hasClock
}

// nearestFrame is the frame of the episode an edge at a moment means,
// counted from the picture's first frame: the frame whose start is
// nearest it, and of two equally near the later. It is the one rule for
// which frame an edge is on. Every edge is put on the start of its frame
// as it enters the plan, see PieceOnFrames, and the render cuts each
// piece on the frames its edges mean, see cutOf.
//
// It is worked out in whole microseconds and whole numbers, the way cutOf
// works out where a frame starts, so a moment exactly half way between two
// frames is half way and goes to the later one however the numbers are
// written. That holds before the picture's first frame too, for a file
// whose picture starts after its sound, where math.Round sends half way
// the other way, to the earlier frame. No allowance is needed.
func (s SourceInfo) nearestFrame(at float64) int64 {
	num, den := int64(s.FPSNum), int64(s.FPSDen)
	us := int64(math.Round(at*1_000_000)) - int64(math.Round(s.VideoStart*1_000_000))
	// us·num/(den·10⁶) frames, and half a frame more, rounded down.
	return floorDiv(2*us*num+den*1_000_000, 2*den*1_000_000)
}

// floorDiv is a over b rounded down, for b above nought, where Go's own
// division rounds towards nought.
func floorDiv(a, b int64) int64 {
	q := a / b
	if a%b != 0 && a < 0 {
		q--
	}
	return q
}

// frameBegins is when frame k of the episode begins, in seconds after the
// start of the file.
func (s SourceInfo) frameBegins(k int64) float64 {
	return s.VideoStart + float64(k)*float64(s.FPSDen)/float64(s.FPSNum)
}

// Grid is the frames of the episode as the video preview shows them,
// the frames of its short: on the grid of the rate, from the first frame
// of it inside the file, see pieceFrames, to the end of the file, each
// the frame of the file that holds its moment, the way byTimes and cutOf
// take it. The episode's decoder makes them by it, so the video preview
// counts on the frames the engine names and reads nothing of the file
// itself. See docs/VIDEO-PREVIEW.md, step 5. No grid without a rate, and
// none longer than framewire.MaxFrames.
func (s SourceInfo) Grid() framewire.Grid {
	if s.FPSNum <= 0 || s.FPSDen <= 0 {
		return framewire.Grid{}
	}
	first, end := s.firstFrame(), s.endFrame()
	// The grid names the clock it counts from, so the decoder counts from
	// the moment the engine does and never reads it a second time.
	return framewire.Grid{Origin: s.origin, Start: s.frameBegins(first), Num: s.FPSNum, Den: s.FPSDen,
		Count: int(min(end-first, framewire.MaxFrames))}
}

// firstFrame is the first frame of the grid inside the file, see Grid.
func (s SourceInfo) firstFrame() int64 {
	first := s.nearestFrame(0)
	if s.frameBegins(first) < 0 {
		first++
	}
	return first
}

// endFrame is the frame after the last one of the grid inside the file,
// see Grid: where the video ends, and so where every piece ends at the
// latest. A file of no known length has a frame after its first.
func (s SourceInfo) endFrame() int64 {
	first := s.firstFrame()
	frames := math.Ceil((s.Duration-s.frameBegins(first))*float64(s.FPSNum)/float64(s.FPSDen) - 1e-6)
	if math.IsNaN(frames) || frames < 1 {
		frames = 1
	}
	return first + int64(min(frames, math.MaxInt32))
}

// End is when the video ends, on the frame after its last, kept to the
// millisecond the way a plan keeps every moment, see PieceOnFrames. It is
// the furthest any edge of a clip goes. Nought where the length is not
// known, and the length itself where the frame rate is not.
func (s SourceInfo) End() float64 {
	if s.Duration <= 0 || math.IsNaN(s.Duration) {
		return 0
	}
	if s.FPSNum <= 0 || s.FPSDen <= 0 {
		return pynum.RoundTo(s.Duration, 3)
	}
	return s.keptAt(s.endFrame())
}

// PieceOnFrames is a piece of a clip with both its edges on the start of
// the frame they mean, see nearestFrame, kept to the millisecond the way
// a plan keeps every moment. A piece holds at least one frame. A grid
// that begins after the file does reaches before it too, where there is
// nothing to read, so a piece starts on the first frame of the grid inside
// the file at the earliest. In the same way it ends where the video does
// at the latest, see End, and one that starts after that is the last
// frame. An end dragged past the video was saved there, and the short
// then came out shorter than its clip and failed its check. Without a
// frame rate the piece is left where it is, but for the end of the video.
//
// Every piece enters a plan through it: the pieces a search or a clip made
// by hand is made of, see planBuilder.frame, and the pieces a gesture
// leaves, see checkedPieces. So every edge in a plan lies within half a
// millisecond of a frame's start, and the frame the video preview shows
// for it, the one that holds it, is the frame the render cuts on. Put on
// frames again, an edge stays where it is.
func (s SourceInfo) PieceOnFrames(start, end float64) (float64, float64) {
	if s.FPSNum <= 0 || s.FPSDen <= 0 {
		if over := s.End(); over > 0 {
			end = math.Min(end, over)
		}
		return pynum.RoundTo(start, 3), pynum.RoundTo(end, 3)
	}
	first, last := s.pieceFrames(start, end)
	return s.keptAt(first), s.keptAt(last)
}

// FrameAt is a moment on the start of the frame it means, see
// nearestFrame, inside the video: from its first frame to its end, see
// End. It is where a single edge lands, the edge of a cut or the start or
// end of a clip, and without a frame rate it is the moment to the
// millisecond.
func (s SourceInfo) FrameAt(at float64) float64 {
	if s.FPSNum <= 0 || s.FPSDen <= 0 {
		if over := s.End(); over > 0 {
			at = math.Min(at, over)
		}
		return pynum.RoundTo(at, 3)
	}
	k := max(s.nearestFrame(at), s.firstFrame())
	if s.Duration > 0 {
		k = min(k, s.endFrame())
	}
	return s.keptAt(k)
}

// keptAt is when frame k begins, to the millisecond and half a millisecond
// up, worked out in whole numbers, so that a start exactly half way
// between two milliseconds goes up however the numbers are written.
func (s SourceInfo) keptAt(k int64) float64 {
	num, den := int64(s.FPSNum), int64(s.FPSDen)
	v0 := int64(math.Round(s.VideoStart * 1_000_000))
	// (v0 + k·den·10⁶/num) microseconds, in milliseconds.
	return float64(floorDiv(2*(v0*num+k*den*1_000_000)+1000*num, 2000*num)) / 1000
}

// pieceFrames is the frame a piece starts on and the frame after its
// last, see PieceOnFrames.
func (s SourceInfo) pieceFrames(start, end float64) (int64, int64) {
	first := max(s.nearestFrame(start), s.firstFrame())
	last := s.nearestFrame(end)
	if s.Duration > 0 {
		over := s.endFrame()
		first, last = min(first, over-1), min(last, over)
	}
	return first, max(last, first+1)
}

// cutOf works out a piece on the frames of the episode, by their number
// and not by a time rounded to a millisecond, which is how a render cuts.
//
// The piece is the frames from the one its start means up to the one
// before the one its end means, see pieceFrames. Its edges lie on the
// starts of those frames, see PieceOnFrames, so these are the frames that
// hold them, as the video preview shows them. Its sound is the sound of exactly those frames. ffmpeg takes a
// time to the microsecond, and most frames at 29.97 fps land on none, so
// a time rounded to the millisecond fell a little after a frame as often
// as before it. The frame at the start was then lost, the next piece's
// first frame came in at the end, and the sound ran 30 ms over its picture
// at every cut, which the joining filled with silence. So the read starts
// half a frame before the first frame, where no rounding can move it past
// a frame, and runs a frame longer than the piece. The picture keeps as
// many frames as the piece holds, and the sound is cut where the first
// frame starts and where the frame after the last one starts, to the
// sample. Both are worked out from the frame numbers, so no piece is a
// sample longer than its frames, however many there are.
//
// Frame k begins k over the rate after the picture's first frame, which
// is where the file starts for most files and later for one whose picture
// starts after its sound. Where the frames are not on that grid, see
// SourceInfo.Variable or a piece unevenPieces marked, or the piece starts
// before the picture does, the frames are counted by their own timestamps
// instead, see byTimes.
func (s SourceInfo) cutOf(seg Segment) pieceCut {
	cut := s.plainCut(seg)
	if !s.ownTimes() {
		return cut
	}
	if at, ok := s.keyAtOf(seg); ok {
		cut = cut.readFrom(at)
	}
	return s.onOwnClock(cut)
}

// plainCut is cutOf without the key frame keyedPieces found.
func (s SourceInfo) plainCut(seg Segment) pieceCut {
	if s.FPSNum <= 0 || s.FPSDen <= 0 {
		seek := int64(math.Round(seg.Start * 1_000_000))
		read := int64(math.Round(seg.Duration() * 1_000_000))
		lead := min(seek, soundLead)
		return pieceCut{
			seek: micros(seek), read: micros(read), seekUS: seek, readUS: read,
			soundSeek: micros(seek - lead), soundRead: micros(read + lead), soundSeekUS: seek - lead,
			sound: fmt.Sprintf("atrim=start=%s,asetpts=PTS-STARTPTS", micros(lead)),
		}
	}
	num, den := int64(s.FPSNum), int64(s.FPSDen)
	first, end := s.pieceFrames(seg.Start, seg.End)
	v0 := int64(math.Round(s.VideoStart * 1_000_000))
	// When frame k starts, in microseconds: k·den/num seconds after the
	// picture's first frame.
	start := func(k int64) int64 { return v0 + (2*k*den*1_000_000+num)/(2*num) }
	if s.Variable || first < 0 || s.unevenOver(seg) {
		return s.byTimes(start(first), start(end), first-s.firstFrame(), end-first)
	}
	seek := max(0, v0+(2*first-1)*den*1_000_000/(2*num))
	read := ((end-first+1)*den*1_000_000 + num - 1) / num
	lead := s.leadOf(seek)
	return pieceCut{
		seek:        micros(seek),
		read:        micros(read),
		seekUS:      seek,
		readUS:      read,
		soundSeek:   s.soundSeekOf(seek - lead),
		soundSeekUS: seek - lead,
		soundRead:   micros(read + lead),
		picture:     fmt.Sprintf("trim=end_frame=%d,", end-first),
		sound: fmt.Sprintf("atrim=start=%s:end=%s,asetpts=PTS-STARTPTS",
			micros(start(first)-seek+lead), micros(start(end)-seek+lead)),
	}
}

// byTimes is a piece of n frames of the rate from one moment to another,
// in microseconds, each frame of it the frame of the episode that holds
// its moment by the frames' own timestamps: the last one to begin at or
// before it, give or take frameHair, and the picture's first frame before
// it starts. That is the frame the video preview shows there.
//
// ffmpeg's fps filter, rounding up, puts each frame on the first moment of
// the grid at or after it begins, and a later frame there takes its place,
// so every moment gets the frame that holds it. The read starts at the key
// frame before, so the frame that holds the first moment is read however
// long ago it began, and a frame held to the end is held on by tpad. The
// sound is cut at the two moments, as for a piece counted by frames.
//
// The moments are counted the way the video preview counts them, see
// framewire.Grid: from the start of the grid, a whole microsecond, at
// exactly one frame of the rate each, and the piece starts at frame first
// of the grid. Those moments are not whole microseconds, 1/30 s is
// 33333.3 of them, so the times are moved in ticks that are whole on
// both, 1/3000000 s at 30 a second. Moved in microseconds, a piece's
// moments came up to a microsecond early, and a frame that begins exactly
// a hair after one, as one of Matroska's whole milliseconds can, missed
// it, #266. In the ticks of the stream, 1/600 s from a phone, the hair is
// lost in the rounding. A rate whose ticks would not fit ffmpeg's time
// base, like 2997/100, is moved in microseconds, and a frame exactly a
// hair after its moment may miss it there. The frames come with the
// file's own times, see ownTimes, so they are moved by nothing ffmpeg
// rounds: counted from the seek by ffmpeg, they were moved by the seek
// and the start of the file rounded to the stream's ticks, a millisecond
// in Matroska, and a frame exactly a hair after its moment came a
// microsecond late.
func (s SourceInfo) byTimes(from, to, first, n int64) pieceCut {
	frame := int64(s.FPSDen) * 1_000_000 / int64(s.FPSNum)
	seek := max(0, from-frame/2)
	lead := s.leadOf(seek)
	hair := int64(frameHair * 1_000_000)
	num, den := int64(s.FPSNum), int64(s.FPSDen)
	ticks := 1_000_000 / int64(gcd(1_000_000, s.FPSNum)) * num
	if ticks > math.MaxInt32 {
		ticks = 1_000_000
	}
	grid := int64(math.Round(s.frameBegins(s.firstFrame()) * 1_000_000))
	start := grid*(ticks/1_000_000) + first*den*(ticks/num)
	if ticks%num != 0 {
		start = grid + (2*first*den*1_000_000+num)/(2*num)
	}
	return pieceCut{
		seek:        micros(seek),
		seekUS:      seek,
		soundSeek:   s.soundSeekOf(seek - lead),
		soundSeekUS: seek - lead,
		soundRead:   micros(to - seek + frame + lead),
		fromKey:     true,
		picture: fmt.Sprintf("settb=1/%d,setpts=PTS%+d,fps=fps=%s:start_time=0:round=up,"+
			"tpad=stop_mode=clone:stop_duration=%s,trim=end_frame=%d,",
			ticks, (seek-hair)*(ticks/1_000_000)-start, s.FPSString(), micros(to-from), n),
		sound: fmt.Sprintf("atrim=start=%s:end=%s,asetpts=PTS-STARTPTS",
			micros(from-seek+lead), micros(to-seek+lead)),
	}
}

// leadOf is how much earlier than its picture, read from seek, a piece's
// sound is read, see soundLead. ffmpeg seeks every stream of a file to the
// key frame of its picture, so sound from before the picture's first frame
// is not found by a seek at all. Sound that reaches back there is read
// from the start of the file instead, which is never far.
func (s SourceInfo) leadOf(seek int64) int64 {
	lead := min(seek, soundLead)
	if seek-lead < int64(math.Round(s.VideoStart*1_000_000)) {
		return seek
	}
	return lead
}

// soundSeekOf is where the sound of a piece is read from, as ffmpeg takes
// it, and nothing for the start of a file whose picture starts after its
// sound, see leadOf.
func (s SourceInfo) soundSeekOf(at int64) string {
	if at == 0 && s.VideoStart > 0 {
		return ""
	}
	return micros(at)
}

// micros is a number of microseconds as seconds, written out exactly.
func micros(us int64) string {
	return fmt.Sprintf("%d.%06d", us/1_000_000, us%1_000_000)
}

// signedMicros is micros with its sign written out, for a number that
// may be below nought.
func signedMicros(us int64) string {
	if us < 0 {
		return "-" + micros(-us)
	}
	return "+" + micros(us)
}

// OnFrames is a clip with every edge of its pieces on the start of the
// frame it means, exactly and not to the millisecond, which is how it is
// rendered and how its captions for the render are made. The edges of a
// plan are on frames already, see PieceOnFrames, kept to the millisecond,
// and this is where they are, to the microsecond.
//
// A piece becomes a whole number of frames in the short, so a piece that
// was not one came out longer than its own length, and its sound with it.
// The captions add the pieces up as they are, so at every cut they ran a
// little more ahead of the words they show: a tenth of a second by the
// sixth piece. On the frames, the two add up to the same.
func (s SourceInfo) OnFrames(clip Clip) Clip {
	if s.FPSNum <= 0 || s.FPSDen <= 0 {
		return clip
	}
	out := clip
	out.Segments = make([]Segment, len(clip.Segments))
	for i, seg := range clip.Segments {
		first, last := s.pieceFrames(seg.Start, seg.End)
		seg.Start, seg.End = s.frameBegins(first), s.frameBegins(last)
		out.Segments[i] = seg
	}
	return out
}

// partial marks a file still being written, beside the one it becomes:
// 01_name.part.mp4 for 01_name.mp4. Nothing that counts or finds files
// takes one for the finished file.
const partial = ".part"

// RenderClip writes one finished short, named after its clip.
func (e *Engine) RenderClip(ctx context.Context, clip Clip, sourcePath string,
	source SourceInfo, outDir string, cues []LaidCaption, rs RenderSettings,
	style map[string]any, captionDir string, dryRun bool) (string, error) {
	return e.renderShort(ctx, clip, sourcePath, source, outDir, clip.ShortName(), cues, rs,
		style, captionDir, dryRun)
}

// renderShort writes one finished short as <stem>.mp4 in outDir. The stem
// is the clip's words, or those with a number where the name was taken,
// see shortStem.
func (e *Engine) renderShort(ctx context.Context, clip Clip, sourcePath string,
	source SourceInfo, outDir, stem string, cues []LaidCaption, rs RenderSettings,
	style map[string]any, captionDir string, dryRun bool) (string, error) {
	if err := os.MkdirAll(outDir, 0o755); err != nil {
		return "", err
	}
	if err := os.MkdirAll(captionDir, 0o755); err != nil {
		return "", err
	}

	assName := ""
	// Captions switched off in the app are left out like --no-captions.
	if len(cues) > 0 && !e.SkipCaptions && ResolveStyle(style).Text {
		// ffmpeg runs with the caption directory as its working directory, so
		// the ass filter can reference a bare filename. That sidesteps the
		// filter-graph escaping rules for colons and backslashes in paths.
		assPath, err := SafeChild(captionDir, clip.Basename()+".ass")
		if err != nil {
			return "", err
		}
		if err := e.writeASS(ctx, cues, assPath, rs.OutW, rs.OutH, style, source); err != nil {
			return "", err
		}
		// The face travels with the program, so the render never depends on
		// what is installed on the machine.
		if err := InstallFont(captionDir, ResolveStyle(style).Font); err != nil {
			return "", err
		}
		assName = clip.Basename() + ".ass"
	}

	outPath, err := SafeChild(outDir, stem+".mp4")
	if err != nil {
		return "", err
	}
	// ffmpeg writes beside the short, and the short takes its place only
	// once it is checked. A file under the short's own name counts as a
	// finished short, so one being written, or cut off by a crash, or a
	// render that failed its check, would count as one too, and a failed
	// render again would take the good one before it away.
	tmp, err := SafeChild(outDir, stem+partial+".mp4")
	if err != nil {
		return "", err
	}
	source = e.unevenPieces(ctx, sourcePath, source, clip.Segments)
	source = e.keyedPieces(ctx, sourcePath, source, clip.Segments)
	encoder, err := e.encoderFor(ctx, source.Light())
	if err != nil {
		return "", err
	}
	for {
		cmd, err := e.buildCommand(ctx, clip, sourcePath, source, tmp, rs, assName, encoder)
		if err != nil {
			return "", err
		}
		if dryRun {
			cmd[len(cmd)-1] = outPath
			quoted := make([]string, len(cmd))
			for i, c := range cmd {
				quoted[i] = shellQuote(c)
			}
			fmt.Println(strings.Join(quoted, " "))
			return outPath, nil
		}

		stall := e.stallFor(encoder, source.Light())
		code, stderr, stalled := e.runFFmpegWatched(ctx, cmd[1:], "rendering "+clip.Basename(),
			clip.Duration(), resolvePath(captionDir), nil, stall)
		if ctx.Err() != nil {
			os.Remove(tmp)
			return "", ctx.Err()
		}
		if code == 0 && !stalled {
			break
		}
		os.Remove(tmp)
		tail := pytext.Strip(stderr)
		if len(tail) > 600 {
			tail = tail[len(tail)-600:]
		}
		// An encoder that stops making frames, without a word, is left
		// the same way as one that fails, see encoderStall. Only an
		// encoder with another after it is given a limit at all.
		if stalled {
			next, ok := e.encoderAfter(ctx, encoder, source.Light())
			if !ok {
				return "", renderErr("rendering %s: the encoder %s made no frame for %s:\n%s",
					clip.Basename(), encoder, stall, tail)
			}
			e.Log.Warn("rendering %s: the encoder %s made no frame for %s, rendering it again with %s: %s",
				clip.Basename(), encoder, stall, next, tail)
			encoder = next
			continue
		}
		// An encoder that failed by itself, the Mac's media engine above
		// all, hands the short to the next one this system has, see
		// nextEncoder, and the short is rendered again from the start.
		// The log keeps all ffmpeg said, so a report says why.
		next, ok := e.nextEncoder(ctx, encoder, source.Light(), stderr)
		if !ok {
			return "", renderErr("ffmpeg failed rendering %s:\n%s", clip.Basename(), tail)
		}
		e.Log.Warn("rendering %s: the encoder %s failed, rendering it again with %s: %s",
			clip.Basename(), encoder, next, tail)
		encoder = next
	}

	// An exit code of zero is not proof that the clip is right. A filter
	// graph that quietly drops a segment still exits cleanly, and the result
	// looks like a finished file until someone plays it.
	check := run(ctx, "", e.FFprobe, "-v", "error", "-show_entries",
		"format=duration", "-of", "csv=p=0", tmp)
	written, ok := pynum.ParseFloat(pytext.Strip(check.Stdout))
	if !ok {
		os.Remove(tmp)
		return "", renderErr("%s was written but has no readable duration", clip.Basename())
	}
	if drift := math.Abs(written - clip.Duration()); drift > 0.5 {
		os.Remove(tmp)
		return "", renderErr("%s should be %.1fs but came out %.1fs. Something in the "+
			"filter graph dropped material, so the file is not trustworthy.",
			clip.Basename(), clip.Duration(), written)
	}
	if err := os.Rename(tmp, outPath); err != nil {
		os.Remove(tmp)
		return "", err
	}
	return outPath, nil
}

// CaptionMoment is the moment libass draws the captions at on the frame
// of a clip's short that shows the episode's moment at, in milliseconds,
// as the render hands it over: the short's frames one after another, each
// piece as many as it cuts, see pieceFrames, from nought on the grid of
// the rate, and each frame's start in microseconds put on by half a
// millisecond and cut down, see BuildFilterGraph. The video preview draws
// a frame's captions at it, so they are the short's on every frame, the
// bounce included. A clip is on frames, see OnFrames. Without a rate it
// is the moment on the clip's clock.
func (s SourceInfo) CaptionMoment(clip Clip, at float64) int64 {
	if s.FPSNum <= 0 || s.FPSDen <= 0 {
		return int64(math.Round(ClipTime(clip, at) * 1000))
	}
	k, _ := s.shortFrame(clip, at)
	return s.shortMoment(k)
}

// shortMoment is the moment libass draws the captions at on frame k of a
// short, from nought, see CaptionMoment.
func (s SourceInfo) shortMoment(k int64) int64 {
	us := int64(math.Round(float64(k) * float64(s.FPSDen) * 1_000_000 / float64(s.FPSNum)))
	return (us + 500) / 1000
}

// PieceShowing is the piece of a clip whose frames the short shows for
// the frame of the episode that holds at, see shortFrame, or -1 for a clip
// of no pieces.
func (s SourceInfo) PieceShowing(clip Clip, at float64) int {
	if len(clip.Segments) == 0 {
		return -1
	}
	if s.FPSNum <= 0 || s.FPSDen <= 0 {
		for i, seg := range clip.Segments {
			if at < seg.End {
				return i
			}
		}
		return len(clip.Segments) - 1
	}
	_, piece := s.shortFrame(clip, at)
	return piece
}

// shortFrame is the frame of the short, from nought, that shows the frame
// of the episode that holds at, and the piece it is in: a frame in a cut
// shows as the first frame after it, and one before or after the clip as
// its first or last.
func (s SourceInfo) shortFrame(clip Clip, at float64) (int64, int) {
	holds := s.frameHolding(at)
	var k int64
	for i, seg := range clip.Segments {
		first, end := s.pieceFrames(seg.Start, seg.End)
		if holds < end {
			return k + max(0, holds-first), i
		}
		k += end - first
	}
	// After the clip, its last frame.
	return max(0, k-1), len(clip.Segments) - 1
}

// thumbnailSeek is where ffmpeg is sent for the frame of a short that
// holds a moment of it, see frameHolding, the frame the video preview
// shows there. ffmpeg keeps the first frame that starts at or after where
// it was sent, and a moment is marked in the middle of its frame, so sent
// to the moment it took the frame after. Sent half a frame before the
// frame starts, it takes that frame and no rounding moves it. A moment on
// or after the last frame takes the last frame, because a seek past the
// end of a file finds nothing to show.
func (s SourceInfo) thumbnailSeek(at float64) float64 {
	if s.FPSNum <= 0 || s.FPSDen <= 0 {
		return math.Max(0, math.Min(at, s.Duration-0.05))
	}
	k := min(s.frameHolding(at), int64(s.Grid().Count)-1)
	return math.Max(0, s.frameBegins(k)-0.5/s.FPS())
}

// thumbnailColours turns a frame of a short into the colours of a JPEG.
// A short keeps its colours the way its episode does, BT.709 for HD video,
// and a JPEG is read by JFIF, full range and BT.601, by every reader there
// is. Only the range turned, the picture came out darker than the short,
// the caption purple most of all. ffmpeg reads the frame's own matrix from
// its tags, and its default where the short has none, the same default
// the video preview's decoder takes, see makeGraph.
const thumbnailColours = "scale=out_color_matrix=bt601:out_range=full,format=yuvj420p"

// thumbnailName is how the pictures of a short are called, beside it:
// <name>-1.jpg, <name>-2.jpg and on, in time order.
func thumbnailName(basename string, n int) string {
	return fmt.Sprintf("%s-%d.jpg", basename, n)
}

// WriteThumbnails takes a picture of a short that was just written at each
// of its clip's thumbnails and puts them beside it, as <name>-1.jpg and on.
// A picture is a frame of the short itself, so it is exactly what the short
// shows at that moment, crop and captions included: the frame that holds
// the moment, the one the video preview showed where it was marked, see
// thumbnailSeek, in the colours a player shows the short in, see
// thumbnailColours. Pictures of an earlier render that are no longer asked
// for are removed, so the folder holds what the plan asks for and nothing
// else.
func (e *Engine) WriteThumbnails(ctx context.Context, clip Clip, short string) error {
	dir := filepath.Dir(short)
	// Named after the short, not the clip, so a short with a number in a
	// folder for shorts never touches the pictures of the short it was
	// numbered apart from.
	stem := strings.TrimSuffix(filepath.Base(short), ".mp4")
	pattern := regexp.MustCompile("^" + regexp.QuoteMeta(stem) + `-(\d+)\.jpg$`)
	entries, err := os.ReadDir(dir)
	if err != nil {
		return err
	}
	for _, entry := range entries {
		if m := pattern.FindStringSubmatch(entry.Name()); m != nil && entry.Type().IsRegular() {
			if n, _ := strconv.Atoi(m[1]); n > len(clip.Thumbnails) || n < 1 {
				os.Remove(filepath.Join(dir, entry.Name()))
			}
		}
	}
	if len(clip.Thumbnails) == 0 {
		return nil
	}
	frames, err := e.Probe(ctx, short)
	if err != nil {
		return err
	}
	for i, at := range clip.Thumbnails {
		path, err := SafeChild(dir, thumbnailName(stem, i+1))
		if err != nil {
			return err
		}
		// Where the moment falls in the short, which is a different clock
		// from the episode's once anything was cut out before it.
		t := frames.thumbnailSeek(ClipTime(clip, at))
		tmp := path + partial + ".jpg"
		res := run(ctx, "", e.FFmpeg, "-hide_banner", "-loglevel", "error", "-y",
			"-ss", pynum.Fixed(t, 6), "-i", short, "-frames:v", "1", "-update", "1",
			"-vf", thumbnailColours, "-q:v", "2", tmp)
		if ctx.Err() != nil {
			os.Remove(tmp)
			return ctx.Err()
		}
		if res.Code != 0 {
			os.Remove(tmp)
			return renderErr("could not take thumbnail %d of %s at %s: %s", i+1,
				clip.Basename(), HMS(at), tailRunes(pytext.Strip(res.Stderr), 200))
		}
		if info, err := os.Stat(tmp); err != nil || info.Size() == 0 {
			os.Remove(tmp)
			return renderErr("thumbnail %d of %s came out empty", i+1, clip.Basename())
		}
		if err := os.Rename(tmp, path); err != nil {
			os.Remove(tmp)
			return err
		}
	}
	return nil
}
