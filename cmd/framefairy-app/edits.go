package main

import (
	"context"
	"fmt"
	"math"
	"path/filepath"
	"time"

	"github.com/rs/zerolog"

	"framefairy/engine"
)

// editLevel is the level of a line about an edit: a warning for one that
// was refused, and the person's own act otherwise.
func editLevel(err error) zerolog.Level {
	if err != nil {
		return zerolog.ErrorLevel
	}
	return zerolog.InfoLevel
}

// editLine starts a line of the log about an edit to a clip, see
// LOGGING.md. It names the video and the clip, and never the words: a
// report holds no word of a video.
func editLine(level zerolog.Level, path, clipID string, took time.Duration) *zerolog.Event {
	return theLog.line(level, "edits").Str("video", filepath.Base(path)).Str("clip", clipID).
		Int64("ms", took.Milliseconds())
}

// SetCrop places the crop of a clip by hand, for the pieces turned the way
// the one at a moment is, as the left edge in source pixels, and returns
// the clip as it is now.
func (s *FrameFairy) SetCrop(ctx context.Context, path, plan, clipID string, at float64, left int) (ClipEntry, error) {
	if !s.store.PlanOf(path, plan) {
		return ClipEntry{}, errNotInLibrary
	}
	info, err := s.probe(ctx, path)
	if err != nil {
		return ClipEntry{}, err
	}
	// The crop of a rotated piece is on the picture rotated, so it is kept
	// inside that.
	upright := info
	if entry, err := s.clipEntry(ctx, path, plan, clipID); err == nil {
		if i := pieceAt(entry.Segments, at); i >= 0 {
			upright = info.Rotated(entry.Segments[i].Rotate)
		}
	}
	cw := s.cropWidth(upright)
	left = max(0, min(left, upright.Width-cw))
	if err := s.edit(path, func() error { return engine.SetCrop(s.store.files, plan, clipID, at, left, info, cw) }); err != nil {
		return ClipEntry{}, err
	}
	return s.clipEntry(ctx, path, plan, clipID)
}

// RotatePart rotates the part of a clip between two cuts that a moment is
// in a quarter turn to the left, and returns the clip as it is now.
func (s *FrameFairy) RotatePart(ctx context.Context, path, plan, clipID string, at float64) (ClipEntry, error) {
	if !s.store.PlanOf(path, plan) {
		return ClipEntry{}, errNotInLibrary
	}
	info, err := s.probe(ctx, path)
	if err != nil {
		return ClipEntry{}, err
	}
	began := time.Now()
	err = s.edit(path, func() error { return engine.RotatePart(s.store.files, plan, clipID, at, info) })
	line := editLine(editLevel(err), path, clipID, time.Since(began)).Err(err).Float64("at", at)
	if err != nil {
		line.Msg("rotate a part")
		return ClipEntry{}, err
	}
	entry, err := s.clipEntry(ctx, path, plan, clipID)
	// The turn the part has now and the crop it plays with, so a report
	// shows whether it took the crop of a part turned alike, #402.
	if i := pieceAt(entry.Segments, at); err == nil && i >= 0 {
		line = line.Int("turn", entry.Segments[i].Rotate).Bool("crop_placed", entry.Segments[i].Moved)
	}
	line.Msg("rotate a part")
	return entry, err
}

// pieceAt is the piece of a clip that holds a moment, or the nearest, the
// way engine.SetCrop finds it. Minus one for a clip with none.
func pieceAt(pieces []engine.SegmentView, at float64) int {
	best, gap := -1, math.Inf(1)
	for i, p := range pieces {
		d := 0.0
		if at < p.Start {
			d = p.Start - at
		} else if at >= p.End {
			d = at - p.End
		}
		if d < gap {
			best, gap = i, d
		}
	}
	return best
}

// SetCaptionStyle changes the face and the size of the captions of a whole
// clip set, which is what the workspace offers next to the clip.
func (s *FrameFairy) SetCaptionStyle(ctx context.Context, path, plan, font string, size float64) error {
	if !s.store.PlanOf(path, plan) {
		return errNotInLibrary
	}
	values := map[string]any{}
	if font != "" {
		values["font"] = font
	}
	if size > 0 {
		values["size"] = size
	}
	return s.edit(path, func() error { return engine.SetCaptionStyle(s.store.files, plan, values) })
}

// SetCaptionColours changes the colour of the caption text, of the box
// behind it and of the pill behind the word being spoken, each with how
// opaque it is from 0 to 1, for a whole clip set, beside the face and the
// size. Colours come as #RRGGBB, and an empty one is left as it is.
func (s *FrameFairy) SetCaptionColours(ctx context.Context, path, plan, text string,
	textOpacity float64, box string, boxOpacity float64, highlight string,
	highlightOpacity float64) error {
	if !s.store.PlanOf(path, plan) {
		return errNotInLibrary
	}
	values := map[string]any{}
	if text != "" {
		colour, ok := engine.AssColour(text, textOpacity)
		if !ok {
			return fmt.Errorf("%s is not a colour", engine.Scrub(text, 20))
		}
		values["primary"] = colour
	}
	if box != "" {
		colour, ok := engine.AssColour(box, boxOpacity)
		if !ok {
			return fmt.Errorf("%s is not a colour", engine.Scrub(box, 20))
		}
		values["back_colour"] = colour
	}
	if highlight != "" {
		colour, ok := engine.AssColour(highlight, highlightOpacity)
		if !ok {
			return fmt.Errorf("%s is not a colour", engine.Scrub(highlight, 20))
		}
		values["highlight_colour"] = colour
	}
	return s.edit(path, func() error { return engine.SetCaptionStyle(s.store.files, plan, values) })
}

// SetCaptionSwitch turns one of the switches of the captions column on or
// off for a whole clip set: "text", captions burned in at all, "box", the
// box behind them, and "highlight", the pill behind the word being spoken
// and the bounce it makes. Anything else is refused.
func (s *FrameFairy) SetCaptionSwitch(ctx context.Context, path, plan, which string, on bool) error {
	if !s.store.PlanOf(path, plan) {
		return errNotInLibrary
	}
	switch which {
	case "text", "box", "highlight":
	default:
		return fmt.Errorf("%q is no switch of the captions", which)
	}
	return s.edit(path, func() error {
		return engine.SetCaptionStyle(s.store.files, plan, map[string]any{which: on})
	})
}

// SetCaptionsHeight puts the captions where the box was dragged to, as the
// distance from the bottom of a 1080x1920 frame. There is one place for
// every clip of every episode, because a place that suits one video suits
// the next one, so this is a setting and not an edit of a clip.
//
// Clips that were placed by hand before are put back on it, or the picture
// would say one thing and the render do another.
func (s *FrameFairy) SetCaptionsHeight(path string, y float64) error {
	if !s.store.Known(path) {
		return errNotInLibrary
	}
	return s.edit(path, func() error {
		if err := s.store.UpdateSettings(func(set *Settings) { set.CaptionY = engine.SnapCaptionY(y) }); err != nil {
			return err
		}
		return s.followTheHeight(path)
	})
}

// ResetCaptionsHeight puts the captions back where the app puts them.
func (s *FrameFairy) ResetCaptionsHeight(path string) error {
	if !s.store.Known(path) {
		return errNotInLibrary
	}
	return s.edit(path, func() error {
		if err := s.store.UpdateSettings(func(set *Settings) { set.CaptionY = engine.DefaultCaptionY }); err != nil {
			return err
		}
		return s.followTheHeight(path)
	})
}

// SetSearch keeps how many clips a search looks for and how long they may
// be. They are set in the workspace, where the episode they are about is,
// and kept for the next episode as well, because a person who wants short
// clips wants them everywhere. A target of 0 follows the window. The
// numbers are held to the same range the controls offer, because what
// arrives here is not to be trusted.
func (s *FrameFairy) SetSearch(target int, window, min, max float64) error {
	return s.store.UpdateSettings(func(set *Settings) {
		set.Target, set.TargetWindow = 0, 0
		if target > 0 && window > 0 {
			set.Target = int(hold(float64(target), 1, 30))
			set.TargetWindow = hold(window, 1, engine.MaxEpisodeSeconds)
		}
		set.Min = hold(min, 5, 180)
		set.Max = hold(max, 5, 180)
		if set.Min > set.Max {
			set.Max = set.Min
		}
	})
}

// hold keeps a number inside the range the interface offers. What arrives
// from it is not to be trusted, here no more than anywhere else.
func hold(n, low, high float64) float64 {
	if !(n >= low) {
		return low
	}
	if n > high {
		return high
	}
	return n
}

// followTheHeight takes the hand-placed caption line off the clips of an
// episode, so every one of them sits where the setting says.
func (s *FrameFairy) followTheHeight(path string) error {
	for _, plan := range plansOf(s.store.files, path) {
		if !s.store.PlanOf(path, plan.Path) {
			continue
		}
		if _, err := engine.ClearCaptionY(s.store.files, plan.Path); err != nil {
			return err
		}
	}
	return nil
}

// SetThumbnail adds, moves or removes a thumbnail of a clip, a moment of
// the episode the render takes a picture of the short at, and returns the
// clip as it is now. A from below nought adds one at to, and a to below
// nought removes the one at from.
func (s *FrameFairy) SetThumbnail(ctx context.Context, path, plan, clipID string, from, to float64) (ClipEntry, error) {
	if !s.store.PlanOf(path, plan) {
		return ClipEntry{}, errNotInLibrary
	}
	if err := s.edit(path, func() error {
		return engine.SetThumbnail(s.store.files, plan, clipID, from, to)
	}); err != nil {
		return ClipEntry{}, err
	}
	return s.clipEntry(ctx, path, plan, clipID)
}

// SetCropMark adds, moves or removes a keyframe of a clip, a crop placed
// by hand on one frame, see engine.CropMark, and returns the clip as it is
// now. A from below nought adds one at to with its crop at x, a to below
// nought removes the one at from, and both moments move the mark with its
// crop.
func (s *FrameFairy) SetCropMark(ctx context.Context, path, plan, clipID string, from, to float64, x int) (ClipEntry, error) {
	if !s.store.PlanOf(path, plan) {
		return ClipEntry{}, errNotInLibrary
	}
	info, err := s.probe(ctx, path)
	if err != nil {
		return ClipEntry{}, err
	}
	if err := s.edit(path, func() error {
		return engine.SetCropMark(s.store.files, plan, clipID, from, to, x, info)
	}); err != nil {
		return ClipEntry{}, err
	}
	return s.clipEntry(ctx, path, plan, clipID)
}

// SetCaptionTime moves the caption of a clip that begins or ends on a word,
// to appear or go at a moment of the episode, and takes the words it is
// moved in over out of it, or puts back the ones it is moved back over, in
// one step to undo. A moment below nought puts it back where its words put
// it, because JSON has no way to say not a number. toWords puts the edge on
// the word nearest at, the way shift does on the clip timeline.
func (s *FrameFairy) SetCaptionTime(ctx context.Context, path, plan, clipID string, word float64, edge string, at float64,
	toWords bool) (ClipEntry, error) {
	if !s.store.PlanOf(path, plan) {
		return ClipEntry{}, errNotInLibrary
	}
	if at < 0 {
		at = math.NaN()
	}
	// The words it reads are the ones kept: moving a caption changes the
	// clip, the words it removes included, and never what the video says.
	t, err := s.words(path)
	if err != nil {
		return ClipEntry{}, err
	}
	// The look is taken before the edit, which holds the video's other
	// edits while it runs, so ffprobe reading the video holds none of them.
	look := s.captionLook(path, plan)
	began := time.Now()
	var done engine.CaptionEdit
	err = s.edit(path, func() error {
		var err error
		done, err = engine.SetCaptionTime(s.store.files, plan, clipID, word, edge, at, toWords, t, look)
		return err
	})
	// Which edge, of the caption on which word, where to, and what it did
	// to the words: a caption that ended up other than the person meant is
	// read back from these.
	line := s.wordCounts(editLine(editLevel(err), path, clipID, time.Since(began)).Err(err), plan, clipID)
	line = line.Str("edge", edge).Float64("word", word)
	if math.IsNaN(at) {
		line = line.Bool("reset", true)
	} else {
		line = line.Float64("at", at).Bool("words", toWords)
	}
	line.Int("out", done.Removed).Int("back", done.Back).Msg("a caption edge was moved")
	if err != nil {
		return ClipEntry{}, err
	}
	return s.clipEntry(ctx, path, plan, clipID)
}

// ShapeCaptionTime works out the captions of a clip with the edge of one
// where the hand has it on the clip timeline, the words it crosses taken
// out or put back, and writes nothing. It is what SetCaptionTime saves when
// the hand lets go, so the timeline and the video preview show while the
// hand moves what is saved, and the interface keeps no rule of its own
// about which words an edge crosses. See engine.ShapeCaptionTime.
func (s *FrameFairy) ShapeCaptionTime(path, plan, clipID string, word float64, edge string, at float64,
	toWords bool) (*engine.CaptionsView, error) {
	if !s.store.PlanOf(path, plan) {
		return nil, errNotInLibrary
	}
	if at < 0 {
		at = math.NaN()
	}
	t, err := s.words(path)
	if err != nil {
		return nil, err
	}
	view, err := engine.ShapeCaptionTime(s.store.files, plan, clipID, word, edge, at, toWords, t, s.captionLook(path, plan))
	captionTracks.keep(path, view)
	return view, err
}

// ResetCrop brings back the automatic crop of a clip, for the pieces turned
// the way the one at a moment is, and returns the clip as it is now.
func (s *FrameFairy) ResetCrop(ctx context.Context, path, plan, clipID string, at float64) (ClipEntry, error) {
	if !s.store.PlanOf(path, plan) {
		return ClipEntry{}, errNotInLibrary
	}
	if err := s.edit(path, func() error { return engine.ResetCrop(s.store.files, plan, clipID, at) }); err != nil {
		return ClipEntry{}, err
	}
	return s.clipEntry(ctx, path, plan, clipID)
}

// RemoveClip takes a clip out of the list, or puts it back. The clip stays
// in the plan either way, so putting it back loses nothing of what was done
// to it. A render of the whole plan leaves a removed clip out.
func (s *FrameFairy) RemoveClip(ctx context.Context, path, plan, clipID string, removed bool) (ClipEntry, error) {
	if !s.store.PlanOf(path, plan) {
		return ClipEntry{}, errNotInLibrary
	}
	began := time.Now()
	err := s.edit(path, func() error { return engine.SetRejected(s.store.files, s.store.records, plan, clipID, removed) })
	what := "a clip was put back"
	if removed {
		what = "a clip was removed"
	}
	editLine(editLevel(err), path, clipID, time.Since(began)).Err(err).Msg(what)
	if err != nil {
		return ClipEntry{}, err
	}
	return s.clipEntry(ctx, path, plan, clipID)
}

// Shape works out what a gesture on the clip timeline makes of a clip,
// with its captions, while the hand moves, and writes nothing. See
// engine/shape.go.
func (s *FrameFairy) Shape(path, plan, clipID string, g engine.Gesture) (*engine.ShapedView, error) {
	if !s.store.PlanOf(path, plan) {
		return nil, errNotInLibrary
	}
	t, err := s.words(path)
	if err != nil {
		return nil, err
	}
	g.Frames = s.frames(context.Background(), path)
	view, err := engine.ShapeClipView(s.store.files, plan, clipID, g, t, s.store.Settings().options().KeepPause,
		s.captionLook(path, plan))
	if err == nil {
		captionTracks.keep(path, view.Captions)
	}
	return view, err
}

// Reshape makes the change a gesture on the clip timeline showed while the
// hand moved, and returns the clip as it is now. Playhead is where the
// playhead stood when the hand took hold and where the gesture left it, so
// an undo puts both the clip and the playhead back.
func (s *FrameFairy) Reshape(ctx context.Context, path, plan, clipID string, g engine.Gesture, playhead [2]float64) (ClipEntry, error) {
	if !s.store.PlanOf(path, plan) {
		return ClipEntry{}, errNotInLibrary
	}
	t, err := s.words(path)
	if err != nil {
		return ClipEntry{}, err
	}
	g.Frames = s.frames(ctx, path)
	if err := s.editMoving(path, &playhead, func() error {
		return engine.Reshape(s.store.files, s.store.records, plan, clipID, g, t, s.store.Settings().options().KeepPause)
	}); err != nil {
		return ClipEntry{}, err
	}
	return s.clipEntry(ctx, path, plan, clipID)
}
