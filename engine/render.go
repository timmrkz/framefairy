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
}

// BuildFilterGraph makes one filter chain per segment, each reading its own
// seeked input.
//
// Every segment is a separate -i with its own -ss, so ffmpeg seeks to a
// keyframe near the segment and decodes only what the clip needs, instead of
// decoding the episode from frame zero. A segment has two inputs, its
// picture from input 2i and its sound from input 2i+1, see soundLead.
func (e *Engine) BuildFilterGraph(ctx context.Context, clip Clip, source SourceInfo,
	rs RenderSettings, assName string) (graph, videoLabel, audioLabel string, err error) {
	cropW, cropH := CropWindow(source, rs.OutW, rs.OutH)
	flags := rs.ScaleFlags
	if flags == "" {
		flags = "lanczos"
	}
	var parts []string
	for i, seg := range clip.Segments {
		cut := source.cutOf(seg)
		cropY := (source.Height - cropH) / 2
		cropY -= cropY % 2
		cropX := ClampCropX(seg.CropX, cropW, source.Width)

		chain := []string{
			cut.picture + "setpts=PTS-STARTPTS",
			fmt.Sprintf("crop=%d:%d:%d:%d", cropW, cropH, cropX, cropY),
		}
		if rs.ScaleUp && (cropW != rs.OutW || cropH != rs.OutH) {
			chain = append(chain, fmt.Sprintf("scale=%d:%d:flags=%s", rs.OutW, rs.OutH, flags))
		}
		chain = append(chain, "fps="+source.FPSString(), "format=yuv420p", "setsar=1")
		parts = append(parts, fmt.Sprintf("[%d:v]%s[v%d];", 2*i, strings.Join(chain, ","), i))

		// A piece that runs straight on from the one before, the way a
		// search parts a clip at a camera switch, is heard straight on.
		// Nothing is cut there, and a fade out and in would be a dip in
		// the sound of 30 ms.
		fadeOutAt := math.Max(0, seg.Duration()-Fade)
		achain := []string{
			cut.sound,
			"aformat=sample_fmts=fltp:sample_rates=48000:channel_layouts=stereo",
		}
		if i == 0 || seg.Start-clip.Segments[i-1].End > 0.0005 {
			achain = append(achain, fmt.Sprintf("afade=t=in:st=0:d=%s", pyFloatRepr(Fade)))
		}
		if i+1 == len(clip.Segments) || clip.Segments[i+1].Start-seg.End > 0.0005 {
			achain = append(achain, fmt.Sprintf("afade=t=out:st=%s:d=%s", fixed(fadeOutAt, 4), pyFloatRepr(Fade)))
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

// BuildCommand is the whole ffmpeg invocation for one clip.
func (e *Engine) BuildCommand(ctx context.Context, clip Clip, sourcePath string,
	source SourceInfo, outPath string, rs RenderSettings, assName string) ([]string, error) {
	graph, videoLabel, audioLabel, err := e.BuildFilterGraph(ctx, clip, source, rs, assName)
	if err != nil {
		return nil, err
	}
	abs, err := filepath.Abs(sourcePath)
	if err != nil {
		abs = sourcePath
	}
	cmd := []string{e.FFmpeg, "-hide_banner", "-loglevel", "error", "-stats", "-y"}
	for _, seg := range clip.Segments {
		// -ss before -i seeks rather than decoding up to the point, and -t
		// limits how much is read. Both are input options on purpose. The
		// sound is read again from a little earlier, see soundLead.
		cut := source.cutOf(seg)
		if cut.fromKey {
			cmd = append(cmd, "-noaccurate_seek", "-ss", cut.seek, "-an", "-i", abs)
		} else {
			cmd = append(cmd, "-ss", cut.seek, "-t", cut.read, "-an", "-i", abs)
		}
		if cut.soundSeek != "" {
			cmd = append(cmd, "-ss", cut.soundSeek)
		}
		cmd = append(cmd, "-t", cut.soundRead, "-vn", "-i", abs)
	}
	video, err := e.VideoArgs(ctx, rs)
	if err != nil {
		return nil, err
	}
	cmd = append(cmd,
		"-filter_complex", graph,
		"-map", videoLabel, "-map", audioLabel,
	)
	cmd = append(cmd, video...)
	cmd = append(cmd,
		"-profile:v", "high",
		"-pix_fmt", "yuv420p",
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
}

// cutOf works out a piece on the frames of the episode, by their number
// and not by a time rounded to a millisecond, which is how a render cuts.
//
// The piece is the frames from the one its start is on up to the one
// before its end, the frame that holds each moment, as the video preview
// shows it. Its sound is the sound of exactly those frames. ffmpeg takes a
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
	if s.FPSNum <= 0 || s.FPSDen <= 0 {
		seek := int64(math.Round(seg.Start * 1_000_000))
		read := int64(math.Round(seg.Duration() * 1_000_000))
		lead := min(seek, soundLead)
		return pieceCut{
			seek: micros(seek), read: micros(read),
			soundSeek: micros(seek - lead), soundRead: micros(read + lead),
			sound: fmt.Sprintf("atrim=start=%s,asetpts=PTS-STARTPTS", micros(lead)),
		}
	}
	num, den := int64(s.FPSNum), int64(s.FPSDen)
	rate := float64(num) / float64(den)
	first := int64(math.Round((seg.Start - s.VideoStart) * rate))
	end := max(int64(math.Round((seg.End-s.VideoStart)*rate)), first+1)
	v0 := int64(math.Round(s.VideoStart * 1_000_000))
	// When frame k starts, in microseconds: k·den/num seconds after the
	// picture's first frame.
	start := func(k int64) int64 { return v0 + (2*k*den*1_000_000+num)/(2*num) }
	if s.Variable || first < 0 || s.unevenOver(seg) {
		return s.byTimes(start(first), start(end), end-first)
	}
	seek := max(0, v0+(2*first-1)*den*1_000_000/(2*num))
	read := ((end-first+1)*den*1_000_000 + num - 1) / num
	lead := s.leadOf(seek)
	return pieceCut{
		seek:      micros(seek),
		read:      micros(read),
		soundSeek: s.soundSeekOf(seek - lead),
		soundRead: micros(read + lead),
		picture:   fmt.Sprintf("trim=end_frame=%d,", end-first),
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
// sound is cut at the two moments, as for a piece counted by frames. The
// times are moved in microseconds, since moved in the ticks of the stream,
// 1/600 s from a phone, the hair is lost in the rounding.
func (s SourceInfo) byTimes(from, to, n int64) pieceCut {
	frame := int64(s.FPSDen) * 1_000_000 / int64(s.FPSNum)
	seek := max(0, from-frame/2)
	lead := s.leadOf(seek)
	hair := int64(frameHair * 1_000_000)
	return pieceCut{
		seek:      micros(seek),
		soundSeek: s.soundSeekOf(seek - lead),
		soundRead: micros(to - seek + frame + lead),
		fromKey:   true,
		picture: fmt.Sprintf("settb=1/1000000,setpts=PTS-%s/TB,fps=fps=%s:start_time=0:round=up,"+
			"tpad=stop_mode=clone:stop_duration=%s,trim=end_frame=%d,",
			micros(from-seek+hair), s.FPSString(), micros(to-from), n),
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

// OnFrames is a clip with every edge of its pieces moved to the nearest
// frame of the source, which is how it is rendered and how its captions
// for the render are made.
//
// A piece becomes a whole number of frames in the short, so a piece that
// was not one came out longer than its own length, and its sound with it.
// The captions add the pieces up as they are, so at every cut they ran a
// little more ahead of the words they show: a tenth of a second by the
// sixth piece. On the frames, the two add up to the same. An edge moves by
// half a frame at most, and a piece keeps at least one frame.
func (s SourceInfo) OnFrames(clip Clip) Clip {
	if s.FPSNum <= 0 || s.FPSDen <= 0 {
		return clip
	}
	fps := s.FPS()
	frame := func(t float64) float64 { return s.VideoStart + math.Round((t-s.VideoStart)*fps)/fps }
	out := clip
	out.Segments = make([]Segment, len(clip.Segments))
	for i, seg := range clip.Segments {
		seg.Start = frame(seg.Start)
		// A grid that begins after the file does reaches before it too,
		// where there is nothing to read.
		if seg.Start < 0 {
			seg.Start += 1 / fps
		}
		seg.End = math.Max(frame(seg.End), seg.Start+1/fps)
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
	return e.renderShort(ctx, clip, sourcePath, source, outDir, clip.Basename(), cues, rs,
		style, captionDir, dryRun)
}

// renderShort writes one finished short as <stem>.mp4 in outDir. The stem
// is the clip's name, or that with a number in a folder for shorts where
// the name was taken, see shortStem.
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
		if err := e.WriteASS(ctx, cues, assPath, rs.OutW, rs.OutH, style); err != nil {
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
	cmd, err := e.BuildCommand(ctx, clip, sourcePath, source, tmp, rs, assName)
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

	code, stderr := e.RunFFmpeg(ctx, cmd[1:], "rendering "+clip.Basename(),
		clip.Duration(), resolvePath(captionDir))
	if ctx.Err() != nil {
		os.Remove(tmp)
		return "", ctx.Err()
	}
	if code != 0 {
		os.Remove(tmp)
		tail := strip(stderr)
		if len(tail) > 600 {
			tail = tail[len(tail)-600:]
		}
		return "", renderErr("ffmpeg failed rendering %s:\n%s", clip.Basename(), tail)
	}

	// An exit code of zero is not proof that the clip is right. A filter
	// graph that quietly drops a segment still exits cleanly, and the result
	// looks like a finished file until someone plays it.
	check := run(ctx, "", e.FFprobe, "-v", "error", "-show_entries",
		"format=duration", "-of", "csv=p=0", tmp)
	written, ok := parsePyFloat(strip(check.Stdout))
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

// thumbnailName is how the pictures of a short are called, beside it:
// <name>-1.jpg, <name>-2.jpg and on, in time order.
func thumbnailName(basename string, n int) string {
	return fmt.Sprintf("%s-%d.jpg", basename, n)
}

// WriteThumbnails takes a picture of a short that was just written at each
// of its clip's thumbnails and puts them beside it, as <name>-1.jpg and on.
// A picture is a frame of the short itself, so it is exactly what the short
// shows at that moment, crop and captions included. Pictures of an earlier
// render that are no longer asked for are removed, so the folder holds what
// the plan asks for and nothing else.
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
	for i, at := range clip.Thumbnails {
		path, err := SafeChild(dir, thumbnailName(stem, i+1))
		if err != nil {
			return err
		}
		// Where the moment falls in the short, which is a different clock
		// from the episode's once anything was cut out before it.
		// A moment on the very last frame is held a little inside, because
		// a seek to the end of a file finds nothing after it to show.
		t := math.Max(0, math.Min(ClipTime(clip, at), clip.Duration()-0.05))
		tmp := path + partial + ".jpg"
		res := run(ctx, "", e.FFmpeg, "-hide_banner", "-loglevel", "error", "-y",
			"-ss", fixed(t, 3), "-i", short, "-frames:v", "1", "-update", "1",
			"-vf", "scale=out_range=full,format=yuvj420p", "-q:v", "2", tmp)
		if ctx.Err() != nil {
			os.Remove(tmp)
			return ctx.Err()
		}
		if res.Code != 0 {
			os.Remove(tmp)
			return renderErr("could not take thumbnail %d of %s at %s: %s", i+1,
				clip.Basename(), HMS(at), tailRunes(strip(res.Stderr), 200))
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
