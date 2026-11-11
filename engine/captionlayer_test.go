package engine

import (
	"bytes"
	"context"
	"errors"
	"fmt"
	"io"
	"math"
	"os"
	"path/filepath"
	"reflect"
	"slices"
	"strings"
	"sync"
	"testing"
	"testing/synctest"
	"time"

	"github.com/spf13/afero"

	"framefairy/engine/fileio"
	"framefairy/engine/runlog"
	"framefairy/engine/timed"
	"framefairy/internal/ffmpegtest"
	"framefairy/internal/fixtures"
	"framefairy/internal/framewire"
)

// The captions the episode's decoder draws for the video preview are the
// short's, frame by frame, the bounce included: the short's own frames,
// made by the render's own graph, are its frames without captions with
// the decoder's layer for that frame laid over them, at the moment
// CaptionMoment names. Tim saw the video preview draw the captions
// otherwise, because a browser drew them there and libass in the short.
//
// The render is run with the ffmpeg built beside the decoder, whose libass
// the decoder has, and only its graph, without an encoder, which that
// ffmpeg has none of on Linux. A caption file in a folder whose name
// ffmpeg would read as part of its chain proves the names reach the
// decoder whole.
//
// On the real disk: ffmpeg and the decoder read the episode and the
// caption file.
func TestTheVideoPreviewDrawsTheShortsCaptions(t *testing.T) {
	program := os.Getenv("FRAMEFAIRY_FRAMES")
	if program == "" {
		ffmpegtest.Unusable(t, "FRAMEFAIRY_FRAMES names no framefairy-frames, which make frames builds")
	}
	ffmpeg := builtFFmpeg(program)
	if _, err := os.Stat(ffmpeg); err != nil {
		ffmpegtest.Unusable(t, "the ffmpeg built beside the decoder is not at %s", ffmpeg)
	}
	ctx := context.Background()
	e := NewEngine(runlog.New(&bytes.Buffer{}, false, false), NewMachine(disk))
	e.FFmpeg, e.FFprobe = ffmpeg, filepath.Join(filepath.Dir(ffmpeg), "ffprobe")
	if _, err := e.SubtitleFilter(ctx); err != nil {
		t.Fatalf("the built ffmpeg cannot burn captions in: %v", err)
	}
	// Grey with something moving, so a frame of the short and a frame of
	// the layer are each one frame and no other, in BT.709 the way HD
	// video is.
	dir := t.TempDir()
	source := fixtures.Path(t, "lossless.mkv")
	info, err := e.Probe(ctx, source)
	if err != nil {
		t.Fatal(err)
	}
	words := []timed.Cue{{Start: 0.20, End: 0.40, Text: "Das"}, {Start: 0.50, End: 1.00, Text: "respektvoll"}, {Start: 1.00, End: 1.30, Text: "ist."},
		{Start: 2.00, End: 2.30, Text: "Und"}, {Start: 2.30, End: 2.70, Text: "dann?"}}
	// Two pieces with a cut between them, so the short's clock is not the
	// episode's.
	clip := info.OnFrames(Clip{ID: "01", Slug: "layer",
		Segments: []Segment{{Start: 0.1, End: 1.4}, {Start: 1.9, End: 2.8}}})
	style := new(Fonts).Style(nil)
	laid := ClipCaptions(clip, &Transcript{Words: words, Language: "de"}, style)
	const w, h = 540, 960
	rs := RenderSettings{OutW: w, OutH: h, ScaleUp: true}

	captions := filepath.Join(dir, "it's [a], b; c:d")
	if err := os.MkdirAll(captions, 0o755); err != nil {
		t.Fatal(err)
	}
	track, err := e.CaptionTrack(ctx, laid, w, h, nil, info)
	if err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(captions, "01.ass"), track, 0o644); err != nil {
		t.Fatal(err)
	}
	if err := new(Fonts).Install(captions, style.Font); err != nil {
		t.Fatal(err)
	}
	// The short's frames, with captions and without, through the render's
	// own graph.
	frames := func(assName string) [][]byte {
		t.Helper()
		return graphFrames(t, e, clip, source, info, rs, captions, assName, "", w, h)
	}
	with, without := frames("01.ass"), frames("")
	if len(with) != len(without) || len(with) < 100 {
		t.Fatalf("the short has %d frames with captions and %d without", len(with), len(without))
	}

	dec := newEpisodeFrames(program, source)
	defer dec.Close()
	fonts := filepath.Join(captions, FontsFolder)
	// The episode's moment each frame of the short shows.
	var moments []float64
	for _, seg := range clip.Segments {
		first, end := info.pieceFrames(seg.Start, seg.End)
		for k := first; k < end; k++ {
			moments = append(moments, info.frameBegins(k))
		}
	}
	if len(moments) != len(with) {
		t.Fatalf("the clip has %d frames and its short %d", len(moments), len(with))
	}
	laidOver := func(ms int64, under []byte) []byte {
		t.Helper()
		l, err := dec.Captions(ctx, framewire.Captions{Width: w, Height: h, ShortW: w, ShortH: h,
			File: filepath.Join(captions, "01.ass"), Fonts: fonts}, ms)
		if err != nil {
			t.Fatal(err)
		}
		return over(under, w, l)
	}
	inked, apart := 0, 0
	for k, at := range moments {
		ms := info.CaptionMoment(clip, at)
		got := laidOver(ms, without[k])
		diff, most := lumaDiff(got, with[k])
		if diff > 0.1 || most > 20 {
			t.Errorf("frame %d, %.3f s of the episode at %d ms: the video preview's captions differ "+
				"from the short's by %.3f on average and %d at most", k, at, ms, diff, most)
		}
		if !bytes.Equal(without[k], with[k]) {
			inked++
		}
		// The frame before and after are told apart: a layer a frame off in
		// the bounce does not pass for the right one.
		if k > 0 {
			if before, _ := lumaDiff(laidOver(info.CaptionMoment(clip, moments[k-1]), without[k]), with[k]); before > 2*diff+0.01 {
				apart++
			}
		}
	}
	if inked < 60 {
		t.Errorf("only %d frames of the short have captions, the test proves little", inked)
	}
	if apart < 10 {
		t.Errorf("only %d frames differ from the layer of the frame before, so a frame off would pass", apart)
	}
}

// lumaDiff is how far apart two pictures of red, green and blue are in
// brightness, on average over every pixel and at most, in steps of 8
// bits. Brightness, because the short keeps its colour at a quarter of
// its pixels, so beside a sharp edge its colours run into each other in a
// way a layer laid over the picture by the pixel does not, and neither
// the eye nor anything else here reads that.
func lumaDiff(a, b []byte) (float64, int) {
	sum, most := 0, 0
	for i := 0; i+2 < len(a); i += 3 {
		ya := 2126*int(a[i]) + 7152*int(a[i+1]) + 722*int(a[i+2])
		yb := 2126*int(b[i]) + 7152*int(b[i+1]) + 722*int(b[i+2])
		d := (max(ya-yb, yb-ya) + 5000) / 10000
		sum += d
		most = max(most, d)
	}
	return float64(sum) / float64(len(a)/3), most
}

// builtFFmpeg is the ffmpeg make builds, whose libraries the episode's
// decoder is built on: in .build/ffmpeg beside .build/frames.
func builtFFmpeg(decoder string) string {
	return filepath.Join(filepath.Dir(filepath.Dir(decoder)), "ffmpeg", "bin", "ffmpeg")
}

// over lays a layer of captions over a picture of red, green and blue as
// the video preview does: its colour plus the picture times one less its
// alpha, see framewire.Layer.
func over(picture []byte, width int, l framewire.Layer) []byte {
	out := append([]byte(nil), picture...)
	for y := 0; y < l.H; y++ {
		for x := 0; x < l.W; x++ {
			p := l.Pixels[(y*l.W+x)*4:]
			a := float64(p[3]) / 255
			o := ((l.Y+y)*width + l.X + x) * 3
			for c := range 3 {
				out[o+c] = byte(math.Round(math.Min(255, float64(p[c])+float64(out[o+c])*(1-a))))
			}
		}
	}
	return out
}

// The video preview asks for captions by the name of what they are drawn
// from, and keeps what it was given under it. The same captions keep their
// name however often they are made, the crop and the height of the
// captions in the clip included, and other captions get another. The name
// was made from the drawing formatted as Go formats it, which gives a
// pointer's address, so every view of the same captions had a new one.
func TestACaptionTrackIsNamedByWhatItIs(t *testing.T) {
	words := []timed.Cue{{Start: 0.2, End: 0.6, Text: "Das"}, {Start: 0.6, End: 1.2, Text: "respektvoll"}, {Start: 1.2, End: 1.5, Text: "ist."}}
	view := func(said string) string {
		x, y := 120, 0.4
		clip := Clip{ID: "01", Slug: "name", CaptionY: &y,
			Segments: []Segment{{Start: 0, End: 2, CropX: &x}}}
		ws := slices.Clone(words)
		ws[1].Text = said
		return captionsView(Plan{}, clip, &Transcript{Words: ws, Language: "de"}, nil).Track
	}
	first, again, other := view("respektvoll"), view("respektvoll"), view("respektlos")
	if first == "" || first != again {
		t.Errorf("the same captions are named %q and %q", first, again)
	}
	if other == first {
		t.Errorf("another word kept the name %q", first)
	}
}

// Several requests of the video preview ask for one caption file at once.
// It is made once, and every one of them gets it whole. Each wrote it
// itself before, through one name for its half written file, and a few in
// a hundred found that file gone under them or read it empty.
//
// On the real disk: ffmpeg measures the captions and libass reads the file.
func TestOneCaptionFileIsMadeOnceForManyAtOnce(t *testing.T) {
	t.Parallel()
	ffmpegtest.Need(t)
	ctx := context.Background()
	e := NewEngine(runlog.New(&bytes.Buffer{}, false, false), NewMachine(disk))
	clip := Clip{ID: "01", Slug: "once", Segments: []Segment{{Start: 0, End: 2}}}
	tr := &Transcript{Words: []timed.Cue{{Start: 0.2, End: 0.6, Text: "Das"}, {Start: 0.6, End: 1.2, Text: "respektvoll"}, {Start: 1.2, End: 1.5, Text: "ist."}}, Language: "de"}
	view := captionsView(Plan{}, clip, tr, nil)
	d := view.Drawing()
	want, err := e.CaptionTrack(ctx, d.Laid, 540, 960, d.Style, SourceInfo{})
	if err != nil {
		t.Fatal(err)
	}
	root := t.TempDir()
	for round := range 20 {
		work := filepath.Join(root, fmt.Sprint(round))
		if err := os.Mkdir(work, 0o755); err != nil {
			t.Fatal(err)
		}
		dir := filepath.Join(work, PreviewCaptionsFolder)
		var wg sync.WaitGroup
		files := make([]string, 6)
		errs := make([]error, 6)
		for i := range files {
			wg.Go(func() {
				var made previewFile
				made, errs[i] = e.previewTrack(ctx, work, d, d.Style, new(Fonts).Style(d.Style).Font, SourceInfo{}, 540, 960)
				files[i] = made.file
			})
		}
		wg.Wait()
		for i, file := range files {
			if errs[i] != nil {
				t.Fatalf("round %d, request %d: %v", round, i, errs[i])
			}
			got, err := os.ReadFile(file)
			if err != nil || !bytes.Equal(got, want) {
				t.Fatalf("round %d, request %d: the file %s holds %d bytes of %d: %v", round, i, file, len(got), len(want), err)
			}
		}
		if left, _ := filepath.Glob(filepath.Join(dir, "*"+partial)); len(left) > 0 {
			t.Fatalf("round %d left %v half written", round, left)
		}
	}
}

// The video preview draws the captions at the size the crop has on the
// screen, smaller than the short, from the short's own caption file, and
// libass draws them there as the short's frames would show them made that
// small. Drawn at a third of the short, the layer is the short's layer
// made a third as big, closer than a frame of the bounce apart.
//
// On the real disk: the decoder reads the caption file and its fonts.
func TestTheVideoPreviewDrawsTheCaptionsSmallerAsTheShortWouldShowThem(t *testing.T) {
	program := os.Getenv("FRAMEFAIRY_FRAMES")
	if program == "" {
		ffmpegtest.Unusable(t, "FRAMEFAIRY_FRAMES names no framefairy-frames, which make frames builds")
	}
	ctx := context.Background()
	e := NewEngine(runlog.New(&bytes.Buffer{}, false, false), NewMachine(disk))
	words := []timed.Cue{{Start: 0.20, End: 0.40, Text: "Das"}, {Start: 0.50, End: 1.00, Text: "respektvoll"}, {Start: 1.00, End: 1.30, Text: "ist."}}
	clip := Clip{ID: "01", Slug: "small", Segments: []Segment{{Start: 0, End: 1.4}}}
	laid := ClipCaptions(clip, &Transcript{Words: words, Language: "de"}, new(Fonts).Style(nil))
	const W, H, by = 1080, 1920, 3
	track, err := e.CaptionTrack(ctx, laid, W, H, nil, SourceInfo{})
	if err != nil {
		t.Fatal(err)
	}
	dir := t.TempDir()
	file := filepath.Join(dir, "01.ass")
	if err := os.WriteFile(file, track, 0o644); err != nil {
		t.Fatal(err)
	}
	if err := new(Fonts).Install(dir, new(Fonts).Style(nil).Font); err != nil {
		t.Fatal(err)
	}
	// The decoder is the video's, so it opens one, though no frame of it
	// is drawn on.
	source := fixtures.Path(t, "lossless.mkv")
	dec := newEpisodeFrames(program, source)
	defer dec.Close()
	draw := func(w, h, shortW, shortH int, ms int64) framewire.Layer {
		t.Helper()
		l, err := dec.Captions(ctx, framewire.Captions{Width: w, Height: h, ShortW: shortW, ShortH: shortH,
			File: file, Fonts: filepath.Join(dir, FontsFolder)}, ms)
		if err != nil {
			t.Fatal(err)
		}
		if l.W == 0 {
			t.Fatalf("nothing drawn at %d ms at %dx%d", ms, w, h)
		}
		return l
	}
	// A word at rest, one in the middle of its bounce, and the last.
	const limit = 2.5
	for _, ms := range []int64{350, 550, 900} {
		want := shrunk(draw(W, H, W, H, ms), W, H, by)
		if d := alphaDiff(draw(W/by, H/by, W, H, ms), want, W/by, H/by); d > limit {
			t.Errorf("at %d ms the captions drawn a third as big differ from the short's made a third as big by %.1f", ms, d)
		}
	}
	// The measure tells a frame apart: in the bounce, the next frame of
	// the pop, laid out at 30 frames a second without a video to count, is
	// more than twice as far off as the right frame. Both moments are in
	// the middle of their frames, away from where one scale gives way to
	// the next.
	want := shrunk(draw(W, H, W, H, 550), W, H, by)
	right := alphaDiff(draw(W/by, H/by, W, H, 550), want, W/by, H/by)
	if off := alphaDiff(draw(W/by, H/by, W, H, 590), want, W/by, H/by); off < 2*right {
		t.Errorf("a frame off in the bounce differs by %.1f and the right frame by %.1f, so the measure proves little", off, right)
	}
}

// shrunk is a layer of a picture width by height made by times smaller,
// each pixel the mean of the by by by pixels it covers, as a scaler that
// averages does. The alpha is multiplied in, so the mean is right.
func shrunk(l framewire.Layer, width, height, by int) []byte {
	w, h := width/by, height/by
	out := make([]byte, w*h*4)
	sum := make([]int, w*h*4)
	for y := 0; y < l.H; y++ {
		for x := 0; x < l.W; x++ {
			sx, sy := (l.X+x)/by, (l.Y+y)/by
			if sx >= w || sy >= h {
				continue
			}
			for c := range 4 {
				sum[(sy*w+sx)*4+c] += int(l.Pixels[(y*l.W+x)*4+c])
			}
		}
	}
	for i, s := range sum {
		out[i] = byte((s + by*by/2) / (by * by))
	}
	return out
}

// alphaDiff is how far a layer's alpha is from a whole picture's of width
// by height, on average over the pixels where either has any, in steps of
// 8 bits.
func alphaDiff(l framewire.Layer, picture []byte, width, height int) float64 {
	have := make([]byte, width*height)
	for y := 0; y < l.H; y++ {
		for x := 0; x < l.W; x++ {
			have[(l.Y+y)*width+l.X+x] = l.Pixels[(y*l.W+x)*4+3]
		}
	}
	sum, n := 0, 0
	for i, a := range have {
		b := picture[i*4+3]
		if a == 0 && b == 0 {
			continue
		}
		sum += max(int(a)-int(b), int(b)-int(a))
		n++
	}
	if n == 0 {
		return 0
	}
	return float64(sum) / float64(n)
}

// A word being typed in the caption box is left out of the captions
// libass draws, in its place: the caption file is the same file with that
// word's ink taken away, and its pill while it is lit, with the highlight
// and without, so nothing else of the caption moves. The caption box draws
// the pill, so it grows with what is typed, which Tim missed. Tim saw the
// whole caption grow as a word was opened, because the browser drew all
// of it in its own font.
//
// On the real disk: ffmpeg measures the captions to lay out the pills.
func TestTheWordBeingTypedIsLeftOutInItsPlace(t *testing.T) {
	t.Parallel()
	ffmpegtest.Need(t)
	ctx := context.Background()
	e := NewEngine(runlog.New(&bytes.Buffer{}, false, false), NewMachine(disk))
	clip := Clip{ID: "01", Slug: "typing", Segments: []Segment{{Start: 0, End: 2}}}
	tr := &Transcript{Words: []timed.Cue{{Start: 0.2, End: 0.6, Text: "Das"}, {Start: 0.6, End: 1.2, Text: "respektvoll"}, {Start: 1.2, End: 1.5, Text: "ist."}}, Language: "de"}
	for _, style := range []map[string]any{nil, {"highlight": false}} {
		view := captionsView(Plan{}, clip, tr, nil)
		d := view.Drawing()
		d.Style = style
		shown := shownTag(new(Fonts).Style(style).Primary)
		normal, err := e.CaptionTrack(ctx, d.Laid, 540, 960, style, SourceInfo{})
		if err != nil {
			t.Fatal(err)
		}
		typed := d.Typing(CaptionShowing{Caption: 0, Word: 1})
		typing, err := e.CaptionTrack(ctx, typed.Laid, 540, 960, style, SourceInfo{})
		if err != nil {
			t.Fatal(err)
		}
		if d.Laid[0].Hidden != nil {
			t.Fatalf("typing hid a word of the drawing itself")
		}
		if style == nil && (strings.Contains(string(typing), shown+"respektvoll") || !strings.Contains(string(normal), shown+"respektvoll")) {
			t.Errorf("highlight %v: the word being typed is drawn, or the word was never drawn:\n%s", style, typing)
		}
		// With the highlight, the word's pill and its ink over the pill are
		// left out while it is lit, because the caption box draws the pill
		// growing with what is typed, and the line keeps the word hidden.
		var kept []string
		for _, line := range strings.SplitAfter(string(normal), "\n") {
			var layer int
			var from string
			if n, _ := fmt.Sscanf(strings.ReplaceAll(line, ",", " "), "Dialogue: %d %s", &layer, &from); n == 2 &&
				layer >= 3 && from >= "0:00:00.60" && from < "0:00:01.20" {
				continue
			}
			kept = append(kept, line)
		}
		want := strings.ReplaceAll(strings.Join(kept, ""), shown+"respektvoll", hidden+"respektvoll")
		if style == nil && string(typing) != want {
			t.Errorf("with the highlight the caption file is more than the word's pill and ink apart:\n%s\nnot\n%s", typing, want)
		}
		if style == nil && len(kept) == len(strings.SplitAfter(string(normal), "\n")) {
			t.Errorf("the word's pill was never drawn:\n%s", normal)
		}
		// Without the highlight the caption file is the same file with the
		// word hidden, its box too. The box was measured without the word,
		// 72 pixels narrower, and the words after it got the text's alpha
		// on their outline and shadow, found by the second review.
		if style != nil && string(typing) != strings.Replace(string(normal), "respektvoll", hidden+"respektvoll"+`{\r}`, 1) {
			t.Errorf("without the highlight the word is not hidden in its place, and only that:\n%s\nnot\n%s", typing, normal)
		}
	}
}

// After a word step the video preview lays the rest of the caption over
// the frame and the word with its pill over that, and scales the word by
// itself, see CaptionDrawing.Lifted. At rest the two together are the
// caption the short shows, pixel for pixel near enough, over black and
// over white, so the bounce starts and ends on the caption as it is. The
// word alone is its pill and its ink, and the rest has neither. The bounce
// played from libass's frames at the video's rate stuttered, which Tim saw.
//
// On the real disk: ffmpeg measures the captions, and the decoder reads
// the episode and the caption files.
func TestTheWordLiftedOutAndTheRestAreTheCaptionAtRest(t *testing.T) {
	program := os.Getenv("FRAMEFAIRY_FRAMES")
	if program == "" {
		ffmpegtest.Unusable(t, "FRAMEFAIRY_FRAMES names no framefairy-frames, which make frames builds")
	}
	ffmpeg := builtFFmpeg(program)
	if _, err := os.Stat(ffmpeg); err != nil {
		ffmpegtest.Unusable(t, "the ffmpeg built beside the decoder is not at %s", ffmpeg)
	}
	t.Parallel()
	ctx := context.Background()
	e := NewEngine(runlog.New(&bytes.Buffer{}, false, false), NewMachine(disk))
	e.FFmpeg, e.FFprobe = ffmpeg, filepath.Join(filepath.Dir(ffmpeg), "ffprobe")
	dir := t.TempDir()
	source := fixtures.Path(t, "lossless.mkv")
	info, err := e.Probe(ctx, source)
	if err != nil {
		t.Fatal(err)
	}
	clip := info.OnFrames(Clip{ID: "01", Slug: "lift", Segments: []Segment{{Start: 0, End: 2.5}}})
	tr := &Transcript{Words: []timed.Cue{{Start: 0.2, End: 0.6, Text: "Das"}, {Start: 0.6, End: 1.6, Text: "respektvoll"}, {Start: 1.6, End: 2.0, Text: "ist."}}, Language: "de"}
	d := captionsView(Plan{}, clip, tr, nil).Drawing()
	dec := newEpisodeFrames(program, source)
	defer dec.Close()
	const w, h = 270, 480
	rs := RenderSettings{OutW: 540, OutH: 960, ScaleUp: true}
	// Well after "respektvoll" has popped, at rest.
	at := 1.3
	layer := func(dd *CaptionDrawing) framewire.Layer {
		t.Helper()
		l, err := e.PreviewCaptions(ctx, dec, dir, dd, nil, info, rs, at, w, h)
		if err != nil {
			t.Fatal(err)
		}
		return l.Layer
	}
	whole, rest, word := layer(d), layer(d.Lifted(LiftRest)), layer(d.Lifted(LiftWord))
	if word.W == 0 || rest.W == 0 {
		t.Fatalf("the word is %dx%d and the rest %dx%d", word.W, word.H, rest.W, rest.H)
	}
	if word.W >= whole.W*2/3 {
		t.Errorf("the word alone is %d wide, of a caption %d wide", word.W, whole.W)
	}
	for _, shade := range []byte{0, 255} {
		picture := bytes.Repeat([]byte{shade}, w*h*3)
		apart := over(over(picture, w, rest), w, word)
		mean, most := lumaDiff(over(picture, w, whole), apart)
		if mean > 0.05 || most > 40 {
			t.Errorf("over %d the word and the rest are %.3f apart on average, %d at most, from the caption", shade, mean, most)
		}
	}
	if d.Laid[0].Lift != "" {
		t.Errorf("lifting a part changed the captions it was made from")
	}
}

// A video's folder deleted outside the app while the video preview still
// asks for its captions is not written back: the caption file goes only
// into a work folder that is there.
//
// On the real disk: ffmpeg measures the captions, and the folder being
// gone from the disk is what the test is about.
func TestACaptionFileIsNeverWrittenIntoAWorkFolderThatIsGone(t *testing.T) {
	t.Parallel()
	ffmpegtest.Need(t)
	ctx := context.Background()
	e := NewEngine(runlog.New(&bytes.Buffer{}, false, false), NewMachine(disk))
	clip := Clip{ID: "01", Slug: "gone", Segments: []Segment{{Start: 0, End: 2}}}
	tr := &Transcript{Words: []timed.Cue{{Start: 0.2, End: 0.6, Text: "Das"}, {Start: 0.6, End: 1.2, Text: "respektvoll"}, {Start: 1.2, End: 1.5, Text: "ist."}}, Language: "de"}
	d := captionsView(Plan{}, clip, tr, nil).Drawing()
	work := filepath.Join(t.TempDir(), "folge.framefairy")
	_, err := e.previewTrack(ctx, work, d, d.Style, new(Fonts).Style(d.Style).Font, SourceInfo{}, 540, 960)
	if !errors.Is(err, ErrNoWorkFolder) {
		t.Errorf("a caption file for a work folder that is gone gave %v", err)
	}
	if _, err := os.Stat(work); !os.IsNotExist(err) {
		t.Errorf("the work folder was written back: %v", err)
	}
}

// What writes a caption file or its face never makes the work folder,
// whatever happens to it between a look and the write. A folder made
// with everything above it brought back a work folder deleted while the
// video preview still asked for captions, which CI's walk caught as the
// deletion and the write crossed.
//
// On the real disk: the caption files are for libass, which reads them
// from the disk.
func TestWritingCaptionsNeverMakesTheWorkFolder(t *testing.T) {
	t.Parallel()
	work := filepath.Join(t.TempDir(), "folge.framefairy")
	dir := filepath.Join(work, PreviewCaptionsFolder)
	if err := foldersIn(work, PreviewCaptionsFolder); !errors.Is(err, ErrNoWorkFolder) {
		t.Errorf("the folders in a work folder that is gone gave %v", err)
	}
	if err := writeWhole(dir, filepath.Join(dir, "a.ass"), []byte("[Script Info]\n")); err == nil {
		t.Errorf("a caption file was written into a folder that is gone")
	}
	if err := new(Fonts).Install(dir, DefaultFont); err == nil {
		t.Errorf("a face was written into a folder that is gone")
	}
	if _, err := os.Stat(work); !os.IsNotExist(err) {
		t.Fatalf("the work folder was written back: %v", err)
	}
	if err := os.Mkdir(work, 0o755); err != nil {
		t.Fatal(err)
	}
	if err := foldersIn(work, PreviewCaptionsFolder); err != nil {
		t.Fatal(err)
	}
	if err := foldersIn(work, PreviewCaptionsFolder); err != nil {
		t.Errorf("the folders made twice gave %v", err)
	}
	if info, err := os.Stat(dir); err != nil || !info.IsDir() {
		t.Errorf("the folders in the work folder were not made: %v", err)
	}
}

// quiet is the log of the tests that make caption files by hand.
var quiet = runlog.New(io.Discard, false, false)

// A caption file whose layout panics fails the requests waiting for it,
// with one line that says why, the stack in the log, and the next request
// tries again. The goroutine that made it had no recover, so a panic in
// the layout of a word a person typed ended the whole app, and this test
// with it, issue #604.
//
// It runs in a bubble, so the test knows when both requests wait.
func TestACaptionFileThatPanicsFailsItsRequestsNotTheApp(t *testing.T) {
	t.Parallel()
	synctest.Test(t, func(t *testing.T) {
		c := newTrackCache()
		var said []runlog.Event
		log := runlog.New(io.Discard, false, false)
		log.SetSink(func(ev runlog.Event) { said = append(said, ev) })
		release := make(chan struct{})
		panics := func(context.Context) (previewFile, error) {
			<-release
			panic("a word nobody laid out before")
		}
		errs := make(chan error, 2)
		for range 2 {
			go func() {
				_, err := c.once(context.Background(), log, "/w", "k", panics)
				errs <- err
			}()
		}
		synctest.Wait()
		close(release)
		for range 2 {
			err := <-errs
			if err == nil || !strings.Contains(err.Error(), "a word nobody laid out before") || strings.Contains(err.Error(), "\n") {
				t.Errorf("a request waiting for the file that panicked got %v, want one line with the reason", err)
			}
		}
		var stack bool
		for _, ev := range said {
			stack = stack || ev.Kind == runlog.EventDetail && strings.Contains(ev.Text, "goroutine")
		}
		if !stack {
			t.Errorf("the stack of the panic is not in the log, which said %v", said)
		}
		made, err := c.once(context.Background(), log, "/w", "k", func(context.Context) (previewFile, error) {
			return previewFile{file: "a.ass"}, nil
		})
		if err != nil || made.file != "a.ass" {
			t.Errorf("the request after the panic got %q, %v, want the file made again", made.file, err)
		}
	})
}

// A request of the video preview that is called off while it makes a
// caption file leaves the others that wait for the same file their
// answer. The file was made in the context of the request that came
// first, and every other got its "context canceled" with its own still
// live, answered 500, found by the second review 20 times out of 20. And
// a request called off while it waits stops waiting.
//
// It runs in a bubble, so the test knows when every request waits.
func TestACaptionFileOutlivesTheRequestThatAskedFirst(t *testing.T) {
	t.Parallel()
	synctest.Test(t, func(t *testing.T) {
		c := newTrackCache()
		started, release := make(chan struct{}), make(chan struct{})
		build := func(ctx context.Context) (previewFile, error) {
			close(started)
			<-release
			return previewFile{file: "a.ass"}, ctx.Err()
		}
		// A request that comes after the file is made makes it again, since no
		// file of that name is there, and gets it as well.
		again := func(ctx context.Context) (previewFile, error) { return previewFile{file: "a.ass"}, ctx.Err() }

		first, cancelFirst := context.WithCancel(context.Background())
		firstDone := make(chan error, 1)
		go func() {
			_, err := c.once(first, quiet, "/w", "k", build)
			firstDone <- err
		}()
		<-started
		type answer struct {
			made previewFile
			err  error
		}
		other := make(chan answer, 1)
		go func() {
			made, err := c.once(context.Background(), quiet, "/w", "k", again)
			other <- answer{made, err}
		}()
		leaving, cancelLeaving := context.WithCancel(context.Background())
		leftDone := make(chan error, 1)
		go func() {
			_, err := c.once(leaving, quiet, "/w", "k", again)
			leftDone <- err
		}()

		cancelFirst()
		if err := <-firstDone; !errors.Is(err, context.Canceled) {
			t.Errorf("the request called off gave %v", err)
		}
		cancelLeaving()
		if err := <-leftDone; !errors.Is(err, context.Canceled) {
			t.Errorf("a waiting request called off gave %v", err)
		}
		// The live request waits for the file being made.
		synctest.Wait()
		close(release)
		if got := <-other; got.err != nil || got.made.file != "a.ass" {
			t.Errorf("a live request waiting for the file got %q, %v", got.made.file, got.err)
		}
	})
}

// A video removed with its folder right after a word of it was deleted
// is deleted only once the caption file the video preview asked for is
// written: the file goes on being made after the request that asked for
// it is gone, and written into the folder while it was deleted it failed
// the deletion with "directory not empty", or was left behind in it.
// Found as the bridge's reset failing in CI, issue #561.
//
// It runs in a bubble, so the test knows when the deletion waits.
func TestAWorkFolderIsDeletedOnceItsCaptionFileIsWritten(t *testing.T) {
	t.Parallel()
	synctest.Test(t, func(t *testing.T) {
		fsys, source := afero.NewMemMapFs(), "/held/deleted-while-captioned.mp4"
		work := WorkDir(source)
		file := filepath.Join(work, PreviewCaptionsFolder, "a.ass")
		machine := NewMachine(fsys)
		_, previewTracks := machine.captions()
		started, release := make(chan struct{}), make(chan struct{})
		go func() {
			_, _ = previewTracks.once(context.Background(), quiet, work, file, func(context.Context) (previewFile, error) {
				close(started)
				<-release
				return previewFile{file: file}, afero.WriteFile(fsys, file, []byte("[Script Info]"), 0o644)
			})
		}()
		<-started
		deleted := make(chan error, 1)
		go func() { deleted <- machine.DeleteWork(source) }()
		synctest.Wait()
		select {
		case err := <-deleted:
			t.Fatalf("the folder was deleted while its caption file was being written, %v", err)
		default:
		}
		close(release)
		if err := <-deleted; err != nil {
			t.Fatal(err)
		}
		if fileio.Exists(fsys, work) {
			t.Errorf("the caption file written while the folder went is still there")
		}
	})
}

// While a folder is being deleted no caption file is started inside it,
// and the request is told the folder is gone, as it is when the folder
// was deleted outside the app. Every other folder makes its files, and
// the folder's own again once it is let go.
func TestNoCaptionFileIsStartedInAFolderBeingDeleted(t *testing.T) {
	t.Parallel()
	c := newTrackCache()
	built := 0
	build := func(context.Context) (previewFile, error) {
		built++
		return previewFile{file: "x.ass"}, nil
	}
	release := c.hold("/videos/a.framefairy")
	for _, work := range []string{"/videos/a.framefairy", "/videos/a.framefairy/"} {
		if _, err := c.once(context.Background(), quiet, work, work+"\x00held", build); !errors.Is(err, ErrNoWorkFolder) {
			t.Errorf("a caption file in %s while it is deleted gave %v", work, err)
		}
	}
	if built != 0 {
		t.Errorf("%d caption files were made in a folder being deleted", built)
	}
	for i, work := range []string{"/videos/b.framefairy", "/videos/a.framefairy2"} {
		if _, err := c.once(context.Background(), quiet, work, fmt.Sprint(i), build); err != nil {
			t.Errorf("a caption file in %s beside the folder being deleted gave %v", work, err)
		}
	}
	release()
	release()
	if _, err := c.once(context.Background(), quiet, "/videos/a.framefairy", "after", build); err != nil {
		t.Errorf("a caption file in the folder once it was let go gave %v", err)
	}
	if built != 3 {
		t.Errorf("made %d caption files, want 3", built)
	}
}

// Once the app is on its way out no caption file is started anywhere, and
// ClosePreviews returns only after the one on its way is made, so nothing
// is written into a folder the walks' bridge then removes, issue #713.
func TestNoCaptionFileIsStartedOnceTheAppCloses(t *testing.T) {
	t.Parallel()
	synctest.Test(t, func(t *testing.T) {
		c := newTrackCache()
		started, let := make(chan struct{}), make(chan struct{})
		go func() {
			_, _ = c.once(context.Background(), quiet, "/videos/a.framefairy", "on its way", func(context.Context) (previewFile, error) {
				close(started)
				<-let
				return previewFile{file: "a.ass"}, nil
			})
		}()
		<-started
		closed := make(chan struct{})
		go func() {
			c.close()
			close(closed)
		}()
		synctest.Wait()
		select {
		case <-closed:
			t.Fatal("the previews closed while a caption file was being written")
		default:
		}
		close(let)
		<-closed
		built := 0
		build := func(context.Context) (previewFile, error) {
			built++
			return previewFile{file: "x.ass"}, nil
		}
		for _, work := range []string{"/videos/a.framefairy", "/videos/b.framefairy", "/elsewhere/c.framefairy"} {
			if _, err := c.once(context.Background(), quiet, work, work, build); !errors.Is(err, ErrNoWorkFolder) {
				t.Errorf("a caption file in %s once the app closed gave %v", work, err)
			}
		}
		if built != 0 {
			t.Errorf("%d caption files were made once the app closed", built)
		}
	})
}

// The first caption file written into a folder in a run removes what an
// earlier run left half written, but not the file another build is
// writing beside it at that moment, for other captions: removed, its
// rename failed, and so did the request that asked for it. The first word
// step after the app starts asks for two at once, the word and the rest.
//
// On the real disk: the half written files are files of the folder libass
// reads its caption files from.
func TestTheFirstCaptionFileLeavesAnotherBuildsFileAlone(t *testing.T) {
	t.Parallel()
	dir := t.TempDir()
	building := filepath.Join(dir, "b.ass.123"+partial)
	left := filepath.Join(dir, "c.ass.456"+partial)
	for _, f := range []string{building, left} {
		if err := os.WriteFile(f, []byte("[Script"), 0o644); err != nil {
			t.Fatal(err)
		}
	}
	long := time.Now().Add(-time.Hour)
	if err := os.Chtimes(left, long, long); err != nil {
		t.Fatal(err)
	}
	c := newTrackCache()
	c.written(dir, filepath.Join(dir, "a.ass"))
	if _, err := os.Stat(building); err != nil {
		t.Errorf("the file another build is writing was removed: %v", err)
	}
	if _, err := os.Stat(left); !os.IsNotExist(err) {
		t.Errorf("the file a run that ended left half written stayed: %v", err)
	}
}

// The caption files kept in a folder are those used last, not written
// last: a colour dragged across the wheel wrote a file for every colour,
// and removed the one the clip on screen shows.
func TestTheCaptionFilesKeptAreTheOnesUsedLast(t *testing.T) {
	t.Parallel()
	c := newTrackCache()
	dir := "/nowhere"
	c.dirs[dir] = []string{"/nowhere/a.ass", "/nowhere/b.ass", "/nowhere/c.ass"}
	c.used("/nowhere/a.ass")
	if got := c.dirs[dir]; !slices.Equal(got, []string{"/nowhere/b.ass", "/nowhere/c.ass", "/nowhere/a.ass"}) {
		t.Errorf("after a was used the folder keeps %v", got)
	}
}

// A step of the caption size draws the captions that show on the frame
// from a file of their own, measured in a moment, while the clip's whole
// file is made, issue #360. That file draws on its frames exactly what
// the whole file draws there, every frame of a clip of three captions,
// each word lit in turn, the pop included.
//
// On the real disk: ffmpeg measures the words and the decoder reads the
// caption files and their fonts.
func TestTheCaptionsShowingDrawnAloneAreTheClipsWholeFileThere(t *testing.T) {
	program := os.Getenv("FRAMEFAIRY_FRAMES")
	if program == "" {
		ffmpegtest.Unusable(t, "FRAMEFAIRY_FRAMES names no framefairy-frames, which make frames builds")
	}
	t.Parallel()
	ctx := context.Background()
	e := NewEngine(runlog.New(&bytes.Buffer{}, false, false), NewMachine(disk))
	// The captions showing are measured on a machine of their own, alone,
	// as a step of the size measures them before the clip's whole file,
	// so none of their words comes from the measuring of the whole file.
	first := NewEngine(runlog.New(&bytes.Buffer{}, false, false), NewMachine(disk))
	d := threeCaptions(t, "01", nil)
	style := new(Fonts).Style(nil)
	const w, h = 540, 960
	dir := t.TempDir()
	if err := new(Fonts).Install(dir, style.Font); err != nil {
		t.Fatal(err)
	}
	written := map[string]string{}
	file := func(laid []LaidCaption, alone bool) string {
		t.Helper()
		by := e
		if alone {
			by = first
		}
		track, _, err := by.captionTrack(ctx, laid, w, h, nil, SourceInfo{}, alone)
		if err != nil {
			t.Fatal(err)
		}
		name := filepath.Join(dir, fmt.Sprintf("%d.ass", len(written)))
		if was, ok := written[string(track)]; ok {
			return was
		}
		if err := os.WriteFile(name, track, 0o644); err != nil {
			t.Fatal(err)
		}
		written[string(track)] = name
		return name
	}
	whole := file(d.Laid, false)
	dec := newEpisodeFrames(program, fixtures.Path(t, "lossless.mkv"))
	defer dec.Close()
	draw := func(file string, ms int64) framewire.Layer {
		t.Helper()
		l, err := dec.Captions(ctx, framewire.Captions{Width: w, Height: h, ShortW: w, ShortH: h,
			File: file, Fonts: filepath.Join(dir, FontsFolder)}, ms)
		if err != nil {
			t.Fatal(err)
		}
		return l
	}
	moments := 0
	for ms := int64(0); ms < 4200; ms += 40 {
		part := d.showing(ms)
		if part == nil {
			continue
		}
		moments++
		want, got := draw(whole, ms), draw(file(part.Laid, true), ms)
		if got.X != want.X || got.Y != want.Y || got.W != want.W || got.H != want.H || !bytes.Equal(got.Pixels, want.Pixels) {
			t.Errorf("at %d ms the %d captions showing drew %dx%d at %d,%d, and the clip's whole file %dx%d at %d,%d, or other pixels",
				ms, len(part.Laid), got.W, got.H, got.X, got.Y, want.W, want.H, want.X, want.Y)
		}
	}
	if moments < 50 {
		t.Errorf("only %d moments were drawn from the captions showing alone", moments)
	}
}

// threeCaptions is a clip of three captions or more, of a German sentence
// a word every 0.3 s, laid out in the style that overrides make.
func threeCaptions(t *testing.T, id string, overrides map[string]any) *CaptionDrawing {
	t.Helper()
	var words []timed.Cue
	at := 0.2
	for _, said := range strings.Fields("Das ist respektvoll und dann haben wir einfach weitergemacht, weil es keinen anderen Weg gab.") {
		words = append(words, timed.Cue{Start: at, End: at + 0.25, Text: said})
		at += 0.3
	}
	clip := Clip{ID: id, Slug: "alone", Segments: []Segment{{Start: 0, End: 4.2}}}
	d := &CaptionDrawing{Laid: ClipCaptions(clip, &Transcript{Words: words, Language: "de"}, new(Fonts).Style(overrides)), Clip: clip}
	if len(d.Laid) < 3 {
		t.Fatalf("the clip has %d captions, the test wants three or more", len(d.Laid))
	}
	return d
}

// A step of the caption size draws its frame from a file of the captions
// showing, starts the clip's whole file after it, and draws from the
// whole file once it is there, issue #360.
//
// On the real disk: ffmpeg measures the words, and the caption files are
// written into the work folder.
func TestASizeStepDrawsTheCaptionsShowingFirstAndTheWholeFileAfter(t *testing.T) {
	ffmpegtest.Need(t)
	t.Parallel()
	ctx := context.Background()
	e := NewEngine(runlog.New(&bytes.Buffer{}, false, false), NewMachine(disk))
	work := t.TempDir()
	d := threeCaptions(t, "01", nil)
	font := e.Fonts.Style(nil).Font
	const w, h = 540, 960
	m := int64(centiseconds((d.Laid[1].Start+d.Laid[1].End)/2)) * 10
	shown, err := e.previewTrackAt(ctx, work, d, nil, font, SourceInfo{}, w, h, m)
	if err != nil {
		t.Fatal(err)
	}
	_, previewTracks := e.captions()
	previewTracks.mu.Lock()
	files, making := len(previewTracks.files), len(previewTracks.making)
	previewTracks.mu.Unlock()
	if files+making != 2 {
		t.Errorf("after the first frame %d files are kept and %d on their way, want the captions showing and the whole file", files, making)
	}
	// The whole file started is waited for here, not made again.
	whole, err := e.previewTrack(ctx, work, d, nil, font, SourceInfo{}, w, h)
	if err != nil {
		t.Fatal(err)
	}
	if lines, all := dialogues(t, shown.file), dialogues(t, whole.file); lines == 0 || lines >= all {
		t.Errorf("the first frame drew from a file of %d lines, and the whole file has %d", lines, all)
	}
	again, err := e.previewTrackAt(ctx, work, d, nil, font, SourceInfo{}, w, h, m)
	if err != nil {
		t.Fatal(err)
	}
	if again.file != whole.file {
		t.Errorf("once the whole file is there the frame drew from %s, want %s", again.file, whole.file)
	}
}

// dialogues is how many lines the caption file at path draws.
//
// On the real disk: the engine writes the caption files the decoder reads.
func dialogues(t *testing.T, path string) int {
	t.Helper()
	body, err := os.ReadFile(path)
	if err != nil {
		t.Fatal(err)
	}
	return strings.Count(string(body), "\nDialogue:")
}

// Every size of a clip is drawn in one place, so the whole file of a size
// left is called off, and another clip or another size of the short is a
// place of its own.
func TestEverySizeOfAClipIsDrawnInOnePlace(t *testing.T) {
	small, large := threeCaptions(t, "01", map[string]any{"size": 60.0}), threeCaptions(t, "01", map[string]any{"size": 140.0})
	if small.name() == large.name() {
		t.Fatal("the two sizes lay the captions out the same")
	}
	if previewPlace("/w", small, 540, 960) != previewPlace("/w", large, 540, 960) {
		t.Error("two sizes of one clip are drawn in two places")
	}
	if previewPlace("/w", small, 540, 960) == previewPlace("/w", threeCaptions(t, "02", nil), 540, 960) {
		t.Error("two clips are drawn in one place")
	}
	if previewPlace("/w", small, 540, 960) == previewPlace("/w", small, 1080, 1920) {
		t.Error("two sizes of the short are drawn in one place")
	}
}

// A whole file called off as the hand steps on says nothing: it said
// that the words could not be measured, at every step of the size.
//
// On the real disk: the measuring writes its frame for ffmpeg.
func TestACaptionFileCalledOffSaysNothing(t *testing.T) {
	t.Parallel()
	var log bytes.Buffer
	e := NewEngine(runlog.New(&log, false, true), NewMachine(disk))
	ctx, cancel := context.WithCancel(context.Background())
	cancel()
	_, err := e.buildPreviewTrack(ctx, "/nowhere", "/nowhere/captions", threeCaptions(t, "01", nil), nil,
		"", SourceInfo{}, 540, 960, false)
	if !errors.Is(err, context.Canceled) {
		t.Errorf("the file called off ended with %v", err)
	}
	if strings.Contains(log.String(), "no pill") {
		t.Errorf("the file called off logged:\n%s", log.String())
	}
}

// The captions that show are measured by the ask that wants them now, and
// not waited for while another ask measures the same words, as the word
// frames and the clip's whole file do at every step of the size: the ask
// of a few blocks then waited for a measuring of every block of the clip.
// Here the other ask never lets go, so a wait would hang the test.
//
// On the real disk: ffmpeg measures the words.
func TestTheCaptionsShowingAreMeasuredWithoutWaitingForAnotherAsk(t *testing.T) {
	ffmpegtest.Need(t)
	t.Parallel()
	ctx := context.Background()
	e := NewEngine(runlog.New(&bytes.Buffer{}, false, false), NewMachine(disk))
	s := e.Fonts.Style(nil)
	blocks := []string{"Das ist", "respektvoll"}
	const width, size = 540, 48
	keys := []string{measureKey(blocks[0], s, width, size), measureKey(blocks[1], s, width, size)}
	measured, _ := e.captions()
	if boxes, _ := measured.claim(keys); boxes[0] != nil || boxes[1] != nil {
		t.Fatal("the words were measured before")
	}
	defer measured.letGo(keys)
	s.alone = true
	boxes, ok := e.measureMany(ctx, blocks, s, width, size)
	if !ok || len(boxes) != 2 || boxes[0] == nil || boxes[1] == nil {
		t.Fatalf("measured alone: %v, %v", boxes, ok)
	}
	if boxes[1].right-boxes[1].left <= boxes[0].right-boxes[0].left {
		t.Errorf("respektvoll is %d wide and Das ist %d", boxes[1].right-boxes[1].left, boxes[0].right-boxes[0].left)
	}
}

// A size stepped with the arrow keys starts the clip's whole caption file
// at every step. The file started for a size the hand has left is called
// off, so it does not measure on beside the steps after it, unless a
// request waits for it.
//
// It runs in a bubble, so the test knows when every build waits.
func TestACaptionFileStartedForASizeLeftIsCalledOff(t *testing.T) {
	t.Parallel()
	synctest.Test(t, func(t *testing.T) {
		c := newTrackCache()
		ctx := context.Background()
		calledOff := make(chan string, 3)
		until := func(name string) func(context.Context) (previewFile, error) {
			return func(ctx context.Context) (previewFile, error) {
				<-ctx.Done()
				calledOff <- name
				return previewFile{}, ctx.Err()
			}
		}
		release := make(chan struct{})
		last := func(context.Context) (previewFile, error) {
			<-release
			return previewFile{file: "98.ass"}, nil
		}
		if err := c.start(ctx, quiet, "/w", "size 96", "clip 01", until("96")); err != nil {
			t.Fatal(err)
		}
		if err := c.start(ctx, quiet, "/w", "size 97", "clip 01", until("97")); err != nil {
			t.Fatal(err)
		}
		if got := <-calledOff; got != "96" {
			t.Errorf("the file called off is %s, want 96", got)
		}
		// Another clip's file is not this place's.
		if err := c.start(ctx, quiet, "/w", "size 97 of 02", "clip 02", until("97 of 02")); err != nil {
			t.Fatal(err)
		}
		// A request waits for the file of 97, so it is not called off.
		waited := make(chan error, 1)
		go func() {
			_, err := c.once(ctx, quiet, "/w", "size 97", nil)
			waited <- err
		}()
		synctest.Wait()
		if err := c.start(ctx, quiet, "/w", "size 98", "clip 01", last); err != nil {
			t.Fatal(err)
		}
		synctest.Wait()
		select {
		case got := <-calledOff:
			t.Errorf("the file of %s was called off", got)
		default:
		}
		close(release)
		// The files nobody called off end when their time is up.
		if err := <-waited; !errors.Is(err, context.DeadlineExceeded) {
			t.Errorf("the request waiting for 97 got %v", err)
		}
	})
}

// A size stepped to and back again starts its whole file again, while the
// one called off for it is still ending: a request that joined the one
// called off failed with its "context canceled". A folder is still held
// until the one called off has ended, so nothing is written into a work
// folder being deleted.
//
// It runs in a bubble, so the test knows when every build waits.
func TestACaptionFileCalledOffIsStartedAgainWhenItsSizeComesBack(t *testing.T) {
	t.Parallel()
	synctest.Test(t, func(t *testing.T) {
		c := newTrackCache()
		ctx := context.Background()
		calledOff, ended := make(chan struct{}), make(chan struct{})
		slow := func(ctx context.Context) (previewFile, error) {
			<-ctx.Done()
			close(calledOff)
			<-ended
			return previewFile{}, ctx.Err()
		}
		made := func(file string) func(context.Context) (previewFile, error) {
			return func(context.Context) (previewFile, error) { return previewFile{file: file}, nil }
		}
		if err := c.start(ctx, quiet, "/w", "size 96", "clip 01", slow); err != nil {
			t.Fatal(err)
		}
		if err := c.start(ctx, quiet, "/w", "size 97", "clip 01", made("97.ass")); err != nil {
			t.Fatal(err)
		}
		<-calledOff
		got, err := c.once(ctx, quiet, "/w", "size 96", made("96.ass"))
		if err != nil || got.file != "96.ass" {
			t.Errorf("the size come back got %v, %v", got, err)
		}
		held := make(chan struct{})
		go func() {
			c.hold("/w")
			close(held)
		}()
		synctest.Wait()
		select {
		case <-held:
			t.Error("the folder was held while a file called off was still ending in it")
		default:
		}
		close(ended)
		<-held
	})
}

// A step to a size whose whole file is there already calls off the whole
// file started for the size before it.
//
// On the real disk: a file kept is kept only while it is there.
func TestAWholeFileThereCallsOffTheOneStartedBeforeIt(t *testing.T) {
	t.Parallel()
	there := filepath.Join(t.TempDir(), "99.ass")
	if err := os.WriteFile(there, nil, 0o644); err != nil {
		t.Fatal(err)
	}
	synctest.Test(t, func(t *testing.T) {
		c := newTrackCache()
		c.files["size 99"] = previewFile{file: there}
		calledOff := make(chan struct{})
		slow := func(ctx context.Context) (previewFile, error) {
			<-ctx.Done()
			close(calledOff)
			return previewFile{}, ctx.Err()
		}
		if err := c.start(context.Background(), quiet, "/w", "size 98", "clip 01", slow); err != nil {
			t.Fatal(err)
		}
		if got, ok := c.keptAt("size 99", "clip 01"); !ok || got.file != there {
			t.Fatalf("the file of 99 is %v, %v", got, ok)
		}
		synctest.Wait()
		select {
		case <-calledOff:
		default:
			t.Error("the file of 98 was not called off")
		}
	})
}

// A drawing's name changes with every field of its captions, so two
// drawings that differ never share a name, and two that are the same
// always do, whatever order the words hidden are kept in. The captions are
// written out by hand for the name, see LaidCaption.appendName, so a field
// added to a caption has to be added there too, and this test says so.
func TestADrawingsNameHasEveryFieldOfItsCaptions(t *testing.T) {
	drawing := func() *CaptionDrawing {
		return &CaptionDrawing{
			Laid: []LaidCaption{{
				Caption: Caption{Start: 1, End: 2, Text: "eins zwei", Opens: 11, Closes: 12,
					Words: []timed.Cue{{Start: 1, End: 1.4, Text: "eins"}, {Start: 1.5, End: 2, Text: "zwei"}}},
				Lines:  [][]timed.Cue{{{Start: 1, End: 1.4, Text: "eins"}}, {{Start: 1.5, End: 2, Text: "zwei"}}},
				Hidden: map[int]bool{0: true, 1: true, 3: true},
				Lift:   LiftWord,
			}},
			Style: map[string]any{"size": 80.0, "primary": "&H00FFFFFF", "box": 1.0},
			Clip:  Clip{ID: "01", Segments: []Segment{{Start: 10, End: 20}}},
		}
	}
	name := drawing().name()
	for range 20 {
		if again := drawing().name(); again != name {
			t.Fatalf("the same drawing was named %s and %s", name, again)
		}
	}
	changes := map[string]func(d *CaptionDrawing){
		"Start":  func(d *CaptionDrawing) { d.Laid[0].Start = 1.04 },
		"End":    func(d *CaptionDrawing) { d.Laid[0].End = 2.04 },
		"Text":   func(d *CaptionDrawing) { d.Laid[0].Text = "eins drei" },
		"Words":  func(d *CaptionDrawing) { d.Laid[0].Words = d.Laid[0].Words[:1] },
		"Opens":  func(d *CaptionDrawing) { d.Laid[0].Opens = 11.5 },
		"Closes": func(d *CaptionDrawing) { d.Laid[0].Closes = 12.5 },
		"Lines": func(d *CaptionDrawing) {
			d.Laid[0].Lines = [][]timed.Cue{{d.Laid[0].Lines[0][0], d.Laid[0].Lines[1][0]}}
		},
		"Hidden": func(d *CaptionDrawing) { d.Laid[0].Hidden = map[int]bool{0: true} },
		"Lift":   func(d *CaptionDrawing) { d.Laid[0].Lift = LiftRest },
		"a size": func(d *CaptionDrawing) { d.Style["size"] = 81.0 },
		"a piece": func(d *CaptionDrawing) {
			d.Clip.Segments = []Segment{{Start: 10, End: 19.96}}
		},
		"a second caption": func(d *CaptionDrawing) { d.Laid = append(d.Laid, d.Laid[0]) },
	}
	// Every exported field of a laid caption, the caption in it too.
	var fields func(ty reflect.Type)
	fields = func(ty reflect.Type) {
		for f := range ty.Fields() {
			if f.Anonymous {
				fields(f.Type)
			} else if f.IsExported() && changes[f.Name] == nil {
				t.Errorf("a caption's %s is not in its drawing's name, see LaidCaption.appendName", f.Name)
			}
		}
	}
	fields(reflect.TypeFor[LaidCaption]())
	// Every field of a word, in the words and in the lines.
	for f := range reflect.TypeFor[timed.Cue]().Fields() {
		for _, where := range []string{"words", "lines"} {
			changes[fmt.Sprintf("a word's %s in the %s", f.Name, where)] = func(d *CaptionDrawing) {
				word := &d.Laid[0].Words[0]
				if where == "lines" {
					word = &d.Laid[0].Lines[0][0]
				}
				v := reflect.ValueOf(word).Elem().FieldByIndex(f.Index)
				switch v.Kind() {
				case reflect.Float64:
					v.SetFloat(v.Float() + 0.04)
				case reflect.String:
					v.SetString(v.String() + "s")
				default:
					t.Fatalf("a word's %s is a %s, which this test cannot change", f.Name, v.Kind())
				}
			}
		}
	}
	for what, change := range changes {
		d := drawing()
		change(d)
		if d.name() == name {
			t.Errorf("with %s changed the drawing keeps its name", what)
		}
	}
}
