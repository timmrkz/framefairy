package engine

import (
	"cmp"
	"context"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"fmt"
	"math"
	"os"
	"path/filepath"
	"slices"
	"strings"
	"sync"

	"framefairy/internal/framewire"
)

// The captions of the video preview are the short's: the caption file the
// render burns in, made by CaptionTrack from what a caption view was made
// from, drawn by libass in the episode's decoder, at the moment libass
// reads on that frame of the short. Nothing else draws a caption. See
// docs/CAPTIONS.md.

// name is a name for what the captions are drawn from, the same for the
// same captions, so the video preview asks for them by it. It is made from
// what the captions are, written out in full: a pointer in the clip, like
// its crop or the height of its captions, is followed to its value, where
// formatting it would give its address and a new name every time.
func (d *CaptionDrawing) name() string {
	body, err := json.Marshal(d)
	if err != nil {
		// Only a number JSON cannot hold, which no caption has. The name is
		// then of this drawing alone, which is still never another's.
		body = fmt.Appendf(nil, "%p", d)
	}
	sum := sha256.Sum256(body)
	return hex.EncodeToString(sum[:12])
}

// CaptionDrafts are the parts of a caption style the hand changes while it
// moves, a colour being picked or the captions being dragged up or down,
// which the video preview shows on the way and saves when the hand lets
// go. Only these are taken from the interface, and every one is checked
// the way a plan's style is, see ResolveStyle.
var CaptionDrafts = []string{"primary", "back_colour", "highlight_colour", "highlight",
	"text", "box", "margin_v"}

// PreviewCaptions draws the captions of d over the frame of the episode
// that starts at at, at width by height, as the short shows them on that
// frame made that size: drafts on top of their style, laid out the way
// the render rs lays them out, with the light of the episode, in its
// decoder dec. The caption file is written once into dir, named by what
// is in it, with the faces it needs beside it. A style with the captions
// off draws nothing, as the render burns nothing in.
func (e *Engine) PreviewCaptions(ctx context.Context, dec *EpisodeFrames, dir string, d *CaptionDrawing,
	drafts map[string]any, source SourceInfo, rs RenderSettings, at float64, ahead, width, height int) (framewire.Layer, error) {
	style := d.drafted(drafts)
	resolved := ResolveStyle(style)
	if len(d.Laid) == 0 || !resolved.Text {
		return framewire.Layer{}, nil
	}
	file, err := e.previewTrack(ctx, dir, d, style, resolved.Font, source, rs.OutW, rs.OutH)
	if err != nil {
		return framewire.Layer{}, err
	}
	shortW, shortH := rs.OutW, rs.OutH
	if piece := source.PieceShowing(d.Clip, at); piece >= 0 {
		shortW, shortH = rs.FrameSize(source, d.Clip.Segments[piece])
	}
	return dec.Captions(ctx, framewire.Captions{Width: width, Height: height, ShortW: shortW, ShortH: shortH,
		File: file, Fonts: filepath.Join(dir, FontsFolder)}, source.captionMomentAhead(d.Clip, at, ahead))
}

// Showing is what libass shows of the captions of d on the frame of the
// episode that starts at, at the moment PreviewCaptions draws them at, so
// the caption box over the video preview shows the caption and the word
// the layer under it shows, see CaptionShowing.
func (d *CaptionDrawing) Showing(source SourceInfo, at float64) CaptionShowing {
	return ShowingAt(d.Laid, source.CaptionMoment(d.Clip, at))
}

// popping is the word lit at the moment m of the short, and the
// hundredths of a second it is lit from and to. ok is false where no word
// is lit.
func (d *CaptionDrawing) popping(m int64) (from, to int, ok bool) {
	on := ShowingAt(d.Laid, m)
	if on.Caption < 0 || on.Word < 0 {
		return 0, 0, false
	}
	c := d.Laid[on.Caption]
	start, end := centiseconds(c.Start), centiseconds(c.End)
	begins := wordBegins(c, start, end)
	from, to = begins[on.Word], end
	if on.Word+1 < len(begins) {
		to = begins[on.Word+1]
	}
	return from, to, true
}

// PopScale is the scale libass draws the pill of the word lit on the frame
// of the short ahead frames after the one that shows the frame of the
// episode that starts at, 1 at rest, see popScales. The caption box grows
// the frame round the word by it, so the frame stays on the pill's edge
// through the pop, as it did when the browser drew both.
func (d *CaptionDrawing) PopScale(source SourceInfo, at float64, ahead int) float64 {
	m := source.captionMomentAhead(d.Clip, at, ahead)
	from, to, ok := d.popping(m)
	if !ok {
		return 1
	}
	first, scales := source.wordPop(from, to)
	j := source.popGrid().frameOf(m) - first
	if j < 0 || j >= int64(len(scales)) {
		return 1
	}
	return scales[j]
}

// Bouncing is d as the video preview bounces the word lit on the frame of
// the episode that starts at, when a word step lands on it: the caption of
// that word alone, the word lit for as long as its whole pop takes, and
// the words after it later by as much. So it pops from rest through its
// peak and back however briefly the short lights it, a word of one frame
// too. d itself where no word is lit there, or where the short lights it
// long enough already, and then the bounce is the short's own frames.
//
// The bounce is how the video preview shows a step from word to word, and
// nothing the short shows. Tim saw the pop the short burns in played again
// over every frame he stepped to, the arrow keys included, two animations
// mixed into one, and wanted a step from frame to frame to show only the
// short's frames and a step from word to word to bounce its word.
func (d *CaptionDrawing) Bouncing(source SourceInfo, at float64) *CaptionDrawing {
	m := source.CaptionMoment(d.Clip, at)
	from, to, ok := d.popping(m)
	if !ok {
		return d
	}
	// A whole pop needs its frames, one more to rest on, and the frame the
	// word may light in the middle of.
	g := source.popGrid()
	frame := float64(g.FPSDen) * 1000 / float64(g.FPSNum)
	need := from + int(math.Ceil((popLength+2*frame)/10))
	if to >= need {
		return d
	}
	later := float64(need-to) / 100
	on := ShowingAt(d.Laid, m)
	c := d.Laid[on.Caption]
	c.Words = slices.Clone(c.Words)
	for k := on.Word + 1; k < len(c.Words); k++ {
		c.Words[k].Start += later
		c.Words[k].End += later
	}
	c.End += later
	return &CaptionDrawing{Laid: []LaidCaption{c}, Style: d.Style, Clip: d.Clip}
}

// PopFrames is which frames of the bounce the video preview plays over
// the frame of the episode that starts at, paused on it after a word step,
// see Bouncing, counted from the frame of the short that shows it: from
// behind frames before it, the word's first frame, at rest, to ahead
// frames after it, the frame the bounce is at rest again on. Every word
// bounces the same, from rest through its peak and back to rest, a word of
// one frame too, though the short shows that one at its peak: Tim wanted
// the bounce the same for every word, without exception. None where no
// word is lit, past the bounce, or where the short has no frames to count.
func (d *CaptionDrawing) PopFrames(source SourceInfo, at float64) (behind, ahead int) {
	if source.FPSNum <= 0 || source.FPSDen <= 0 {
		return 0, 0
	}
	from, to, ok := d.Bouncing(source, at).popping(source.CaptionMoment(d.Clip, at))
	if !ok {
		return 0, 0
	}
	first, scales := source.wordPop(from, to)
	k, _ := source.shortFrame(d.Clip, at)
	j, end := int(k-first), len(scales)-1
	if j < 0 || j > end {
		return 0, 0
	}
	return j, end - j
}

// PreviewWordFrames is where the words of d stand in the short the render
// rs makes, with drafts on top of their style, the same captions
// PreviewCaptions draws: the frame the caption box puts round a word is
// placed from it, and when the picture lights each, see WordsLit. Nothing
// when the captions are off or cannot be measured.
func (e *Engine) PreviewWordFrames(ctx context.Context, d *CaptionDrawing, drafts map[string]any,
	source SourceInfo, rs RenderSettings) [][]WordFrame {
	style := d.drafted(drafts)
	if len(d.Laid) == 0 || !ResolveStyle(style).Text {
		return nil
	}
	frames := e.WordFrames(ctx, d.Laid, rs.OutW, rs.OutH, style, source.Light())
	lit := d.WordsLit(source)
	for i := range frames {
		for k := range frames[i] {
			frames[i][k].From, frames[i][k].To = -1, -1
			if i < len(lit) && k < len(lit[i]) {
				frames[i][k].From, frames[i][k].To = lit[i][k][0], lit[i][k][1]
			}
		}
	}
	return frames
}

// Typing is d with word of caption hidden, the word being typed in the
// caption box: libass draws the rest of the caption where it always
// does, and the browser draws only what is typed, in the word's place.
// When the browser drew the whole caption while a word was typed, it came
// out larger than libass draws it, and the caption grew as a word was
// opened and shrank as it was left. d itself when there is no such word.
func (d *CaptionDrawing) Typing(word CaptionShowing) *CaptionDrawing {
	if word.Caption < 0 || word.Caption >= len(d.Laid) || word.Word < 0 {
		return d
	}
	laid := slices.Clone(d.Laid)
	laid[word.Caption].Hidden = map[int]bool{word.Word: true}
	return &CaptionDrawing{Laid: laid, Style: d.Style, Clip: d.Clip}
}

// drafted is the style of d with drafts on top, only those of
// CaptionDrafts.
func (d *CaptionDrawing) drafted(drafts map[string]any) map[string]any {
	style := map[string]any{}
	for k, v := range d.Style {
		style[k] = v
	}
	for _, k := range CaptionDrafts {
		if v, ok := drafts[k]; ok {
			style[k] = v
		}
	}
	return style
}

// previewTrack is the caption file of captions in a style, written into
// dir under a name made from what is in it, once. The video preview asks
// for a few frames at once, so the same file is often asked for by several
// requests together: one makes it and the others wait for it, see
// trackCache.once, because two writing one file each found it gone under
// them or half written.
func (e *Engine) previewTrack(ctx context.Context, dir string, d *CaptionDrawing, style map[string]any,
	font string, source SourceInfo, outW, outH int) (string, error) {
	key := fmt.Sprintf("%s\x00%s\x00%+v\x00%s\x00%d/%d\x00%dx%d", dir, d.name(), style, source.Light(),
		source.FPSNum, source.FPSDen, outW, outH)
	return previewTracks.once(key, func() (string, error) {
		track, err := e.CaptionTrack(ctx, d.Laid, outW, outH, style, source)
		if err != nil {
			return "", err
		}
		// A measurement cut short leaves a track without its pill, which
		// would be kept as the real one.
		if err := ctx.Err(); err != nil {
			return "", err
		}
		sum := sha256.Sum256(track)
		file, err := SafeChild(dir, hex.EncodeToString(sum[:12])+".ass")
		if err != nil {
			return "", err
		}
		// The name is made from what is in it, so a file of that name
		// already holds it.
		if _, err := os.Stat(file); err != nil {
			if err := writeWhole(dir, file, track); err != nil {
				return "", err
			}
		}
		previewTracks.written(dir, file)
		if err := InstallFont(dir, font); err != nil {
			return "", err
		}
		return file, nil
	})
}

// writeWhole writes data to file in dir through a name of its own, so a
// reader, libass above all, never finds it half written.
func writeWhole(dir, file string, data []byte) error {
	if err := os.MkdirAll(dir, 0o755); err != nil {
		return err
	}
	tmp, err := os.CreateTemp(dir, filepath.Base(file)+".*"+partial)
	if err != nil {
		return err
	}
	_, err = tmp.Write(data)
	if cerr := tmp.Close(); err == nil {
		err = cerr
	}
	if err == nil {
		err = os.Rename(tmp.Name(), file)
	}
	if err != nil {
		os.Remove(tmp.Name())
	}
	return err
}

// trackCache keeps which caption file the captions of a view in a style
// were written to, and the files written into each folder, of which it
// keeps the latest few hundred: a colour dragged across the wheel writes
// one for every colour it passes. What a folder held before the app
// started counts too, so the files do not pile up from one run to the
// next.
type trackCache struct {
	mu     sync.Mutex
	files  map[string]string
	making map[string]*making
	dirs   map[string][]string
}

// making is a caption file on its way, which the requests that ask for it
// meanwhile wait for.
type making struct {
	done chan struct{}
	file string
	err  error
}

var previewTracks = &trackCache{files: map[string]string{}, making: map[string]*making{}, dirs: map[string][]string{}}

const previewTracksKept = 256

// once gives the file kept for key, or makes it with build, once however
// many ask for it together. A file that went away is made again, and a
// failure is not kept, so the next request tries again.
func (c *trackCache) once(key string, build func() (string, error)) (string, error) {
	c.mu.Lock()
	if file, ok := c.files[key]; ok {
		c.mu.Unlock()
		if _, err := os.Stat(file); err == nil {
			return file, nil
		}
		c.mu.Lock()
	}
	if m, ok := c.making[key]; ok {
		c.mu.Unlock()
		<-m.done
		return m.file, m.err
	}
	m := &making{done: make(chan struct{})}
	c.making[key] = m
	c.mu.Unlock()

	m.file, m.err = build()

	c.mu.Lock()
	delete(c.making, key)
	if m.err == nil {
		if len(c.files) >= 4*previewTracksKept {
			clear(c.files)
		}
		c.files[key] = m.file
	}
	c.mu.Unlock()
	close(m.done)
	return m.file, m.err
}

// written notes a file written into dir, and removes the oldest beyond
// what is kept, those of earlier runs first, and anything left half
// written by a run that ended.
func (c *trackCache) written(dir, file string) {
	c.mu.Lock()
	defer c.mu.Unlock()
	files, seen := c.dirs[dir]
	if !seen {
		files = earlierTracks(dir, file)
	}
	if !slices.Contains(files, file) {
		files = append(files, file)
	}
	for len(files) > previewTracksKept {
		os.Remove(files[0])
		files = files[1:]
	}
	c.dirs[dir] = files
}

// earlierTracks are the caption files in dir other than file, oldest
// first. What is left half written is removed.
func earlierTracks(dir, file string) []string {
	entries, err := os.ReadDir(dir)
	if err != nil {
		return nil
	}
	type dated struct {
		path string
		at   int64
	}
	var found []dated
	for _, entry := range entries {
		path := filepath.Join(dir, entry.Name())
		switch {
		case !entry.Type().IsRegular() || path == file:
		case strings.HasSuffix(entry.Name(), partial):
			os.Remove(path)
		case strings.HasSuffix(entry.Name(), ".ass"):
			if info, err := entry.Info(); err == nil {
				found = append(found, dated{path, info.ModTime().UnixNano()})
			}
		}
	}
	slices.SortFunc(found, func(a, b dated) int { return cmp.Compare(a.at, b.at) })
	out := make([]string, len(found))
	for i, f := range found {
		out[i] = f.path
	}
	return out
}
