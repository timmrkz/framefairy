package main

import (
	"context"
	"fmt"
	"math"

	"framefairy/engine"
)

// SetCrop places the crop of the shot at a moment of a clip by hand, as the
// left edge in source pixels, and returns the clip as it is now.
func (s *FrameFairy) SetCrop(ctx context.Context, path, plan, clipID string, at float64, left int) (ClipEntry, error) {
	if !s.store.PlanOf(path, plan) {
		return ClipEntry{}, errNotInLibrary
	}
	info, err := s.probe(ctx, path)
	if err != nil {
		return ClipEntry{}, err
	}
	o := s.store.Settings().options()
	cw, _ := engine.CropWindow(info, o.Width, o.Height)
	left = max(0, min(left, info.Width-cw))
	if err := s.edit(path, func() error { return engine.SetCrop(plan, clipID, at, left) }); err != nil {
		return ClipEntry{}, err
	}
	return s.clipEntry(ctx, path, plan, clipID)
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
	return s.edit(path, func() error { return engine.SetCaptionStyle(plan, values) })
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
	return s.edit(path, func() error { return engine.SetCaptionStyle(plan, values) })
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
		return engine.SetCaptionStyle(plan, map[string]any{which: on})
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
	for _, plan := range plansOf(path) {
		if !s.store.PlanOf(path, plan.Path) {
			continue
		}
		if _, err := engine.ClearCaptionY(plan.Path); err != nil {
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
		return engine.SetThumbnail(plan, clipID, from, to)
	}); err != nil {
		return ClipEntry{}, err
	}
	return s.clipEntry(ctx, path, plan, clipID)
}

// SetCaptionTime moves the caption of a clip that begins or ends on a word,
// to appear or go at a moment of the episode. A moment below nought puts it
// back where its words put it, because JSON has no way to say not a number.
func (s *FrameFairy) SetCaptionTime(ctx context.Context, path, plan, clipID string, word float64, edge string, at float64) (ClipEntry, error) {
	if !s.store.PlanOf(path, plan) {
		return ClipEntry{}, errNotInLibrary
	}
	if at < 0 {
		at = math.NaN()
	}
	t, err := s.words(path)
	if err != nil {
		return ClipEntry{}, err
	}
	if err := s.edit(path, func() error {
		return engine.SetCaptionTime(plan, clipID, word, edge, at, t)
	}); err != nil {
		return ClipEntry{}, err
	}
	return s.clipEntry(ctx, path, plan, clipID)
}

// ResetCrop brings back the automatic crop for the shot at a moment of a
// clip, and returns the clip as it is now.
func (s *FrameFairy) ResetCrop(ctx context.Context, path, plan, clipID string, at float64) (ClipEntry, error) {
	if !s.store.PlanOf(path, plan) {
		return ClipEntry{}, errNotInLibrary
	}
	if err := s.edit(path, func() error { return engine.ResetCrop(plan, clipID, at) }); err != nil {
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
	if err := s.edit(path, func() error { return engine.SetRejected(plan, clipID, removed) }); err != nil {
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
	g.FrameStart = s.frameStart(context.Background(), path)
	return engine.ShapeClipView(plan, clipID, g, t, s.store.Settings().options().KeepPause,
		s.captionOverrides(plan))
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
	g.FrameStart = s.frameStart(ctx, path)
	if err := s.editMoving(path, &playhead, func() error {
		return engine.Reshape(plan, clipID, g, t, s.store.Settings().options().KeepPause)
	}); err != nil {
		return ClipEntry{}, err
	}
	return s.clipEntry(ctx, path, plan, clipID)
}
