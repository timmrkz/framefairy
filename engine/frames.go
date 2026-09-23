package engine

import (
	"context"
	"fmt"
	"os"
	"path/filepath"
)

// Still returns a JPEG of the frame at a moment of the episode, made once and
// kept in the work folder. Times are rounded to whole seconds, so dragging
// across the episode reuses frames.
func (e *Engine) Still(ctx context.Context, source string, at float64, width int) (string, error) {
	if at < 0 {
		at = 0
	}
	second := int(at)
	width = max(160, min(width, 1280))
	dir := filepath.Join(WorkDir(source), "cache", "stills")
	path := filepath.Join(dir, fmt.Sprintf("%d-%06d.jpg", width, second))
	if info, err := os.Stat(path); err == nil && info.Size() > 0 {
		return path, nil
	}
	if err := os.MkdirAll(dir, 0o755); err != nil {
		return "", err
	}
	// A file of its own to write into, never one named after the frame.
	// The app asks for a frame every time the playhead moves, and
	// walking the clip list with the arrow keys moves it as fast as a key
	// repeats, so several asks for the same frame are in the air at once.
	// Sharing one temporary file meant two of them wrote over each other
	// and what was left was half of one and half of the other, kept for
	// good, because a frame once written is never made again.
	hold, err := os.CreateTemp(dir, "still-*.jpg")
	if err != nil {
		return "", err
	}
	tmp := hold.Name()
	hold.Close()
	res := run(ctx, "", e.FFmpeg, "-hide_banner", "-loglevel", "error", "-y",
		"-ss", itoa(second), "-i", source, "-frames:v", "1", "-update", "1",
		"-vf", fmt.Sprintf("scale=%d:-2:out_range=full,format=yuvj420p", width), "-q:v", "4", tmp)
	if res.Code != 0 {
		os.Remove(tmp)
		return "", renderErr("could not read a frame at %s: %s", HMS(float64(second)),
			tailRunes(strip(res.Stderr), 200))
	}
	// Moving it into place is one step, so nothing ever reads half a
	// frame. Two asks for the same frame both put a whole one there, and
	// the second is the same picture as the first.
	if err := os.Rename(tmp, path); err != nil {
		os.Remove(tmp)
		return "", err
	}
	return path, nil
}

// Log is the log the project's steps write to.
func (p *Project) Log() *Log { return p.engine.Log }

// Engine is the engine the project's steps run on.
func (p *Project) Engine() *Engine { return p.engine }

// DeleteWork removes everything made for an episode, folder and all, so
// that letting go of a video really does leave nothing behind: transcript,
// clip sets, captions, previews and renders. The training records are not
// in there, they live in one folder of their own, so they outlive it. The
// episode file itself is never touched.
func DeleteWork(source string) error {
	work := WorkDir(source)
	if filepath.Ext(work) != ".framefairy" || filepath.Clean(work) == filepath.Clean(source) {
		return renderErr("refusing to delete %s", work)
	}
	if err := os.RemoveAll(work); err != nil && !os.IsNotExist(err) {
		return err
	}
	return nil
}
