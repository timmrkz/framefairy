package engine

import (
	"context"
	"crypto/sha256"
	"encoding/hex"
	"fmt"
	"os"
	"path/filepath"
	"sync"

	"framefairy/internal/framewire"
)

// The captions of the video preview are the short's: the caption file the
// render burns in, made by CaptionTrack from what a caption view was made
// from, drawn by libass in the episode's decoder, at the moment libass
// reads on that frame of the short. Nothing else draws a caption. See
// docs/CAPTIONS.md.

// name is a name for what the captions are drawn from, the same for the
// same captions, so the video preview asks for them by it.
func (d *CaptionDrawing) name() string {
	sum := sha256.Sum256(fmt.Appendf(nil, "%+v", *d))
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
	drafts map[string]any, source SourceInfo, rs RenderSettings, at float64, width, height int) (framewire.Layer, error) {
	style := map[string]any{}
	for k, v := range d.Style {
		style[k] = v
	}
	for _, k := range CaptionDrafts {
		if v, ok := drafts[k]; ok {
			style[k] = v
		}
	}
	resolved := ResolveStyle(style)
	if len(d.Laid) == 0 || !resolved.Text {
		return framewire.Layer{}, nil
	}
	file, err := e.previewTrack(ctx, dir, d, style, resolved.Font, source.Light(), rs.OutW, rs.OutH)
	if err != nil {
		return framewire.Layer{}, err
	}
	shortW, shortH := rs.OutW, rs.OutH
	if piece := source.PieceShowing(d.Clip, at); piece >= 0 {
		shortW, shortH = rs.FrameSize(source, d.Clip.Segments[piece])
	}
	return dec.Captions(ctx, framewire.Captions{Width: width, Height: height, ShortW: shortW, ShortH: shortH,
		File: file, Fonts: filepath.Join(dir, FontsFolder)}, source.CaptionMoment(d.Clip, at))
}

// previewTrack is the caption file of captions in a style, written into
// dir under a name made from what is in it, once.
func (e *Engine) previewTrack(ctx context.Context, dir string, d *CaptionDrawing, style map[string]any,
	font string, light framewire.Light, outW, outH int) (string, error) {
	key := fmt.Sprintf("%s\x00%s\x00%+v\x00%s\x00%dx%d", dir, d.name(), style, light, outW, outH)
	if file, ok := previewTracks.get(key); ok {
		if _, err := os.Stat(file); err == nil {
			return file, nil
		}
	}
	track, err := e.CaptionTrack(ctx, d.Laid, outW, outH, style, light)
	if err != nil {
		return "", err
	}
	sum := sha256.Sum256(track)
	file, err := SafeChild(dir, hex.EncodeToString(sum[:12])+".ass")
	if err != nil {
		return "", err
	}
	if _, err := os.Stat(file); err != nil {
		if err := os.MkdirAll(dir, 0o755); err != nil {
			return "", err
		}
		tmp := file + partial
		if err := os.WriteFile(tmp, track, 0o644); err != nil {
			return "", err
		}
		if err := os.Rename(tmp, file); err != nil {
			os.Remove(tmp)
			return "", err
		}
		previewTracks.written(dir, file)
	}
	if err := InstallFont(dir, font); err != nil {
		return "", err
	}
	previewTracks.put(key, file)
	return file, nil
}

// trackCache keeps which caption file the captions of a view in a style
// were written to, and the files written into each folder, of which it
// keeps the latest few hundred: a colour dragged across the wheel writes
// one for every colour it passes.
type trackCache struct {
	mu    sync.Mutex
	files map[string]string
	dirs  map[string][]string
}

var previewTracks = &trackCache{files: map[string]string{}, dirs: map[string][]string{}}

const previewTracksKept = 256

func (c *trackCache) get(key string) (string, bool) {
	c.mu.Lock()
	defer c.mu.Unlock()
	file, ok := c.files[key]
	return file, ok
}

func (c *trackCache) put(key, file string) {
	c.mu.Lock()
	defer c.mu.Unlock()
	if len(c.files) >= 4*previewTracksKept {
		clear(c.files)
	}
	c.files[key] = file
}

// written notes a file written into dir, and removes the oldest beyond
// what is kept.
func (c *trackCache) written(dir, file string) {
	c.mu.Lock()
	defer c.mu.Unlock()
	files := append(c.dirs[dir], file)
	for len(files) > previewTracksKept {
		os.Remove(files[0])
		files = files[1:]
	}
	c.dirs[dir] = files
}
