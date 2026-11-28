package main

import (
	"context"
	"errors"
	"math"
	"sync"

	"framefairy/engine"
)

// ---------------------------------------------------------------------------
// The arrow in the crop frame's corner
//
// Go decides everything about it: whether a frame offers it and which way
// it turns, the corner of each frame's dress, and what a click does,
// ResetCrop and PutCropBack. The window draws the corner it is sent and
// forwards a click with the clip and the frame's moment. See
// docs/decisions/2026-11-28-go-decides-the-crop-arrow.md.
// ---------------------------------------------------------------------------

// The corners a frame's dress can name, see sentDress.Corner: the crop
// brought to where the app places it, the arrow turning anticlockwise, and
// put back where the hand had it, clockwise.
const (
	cornerAuto = "auto"
	cornerBack = "back"
)

// cropBack is the way back from the crop brought to where the app places
// it: the clip, the moment its crop was placed on, where the crop stood
// there before, and the clip's framing before and after. It holds while
// the clip's framing is still the one the reset left, after, so any other
// change to the clip's framing ends it, an undo of the reset included,
// and a redo of the reset brings it back.
type cropBack struct {
	plan, clip string
	at         float64
	left       int
	before     framed
	after      string
}

// cropBacks is the way back of one video, with a lock of its own, since
// every frame's dress reads it while an edit may hold the history.
type cropBacks struct {
	mu   sync.Mutex
	back *cropBack
}

func (c *cropBacks) get() (cropBack, bool) {
	c.mu.Lock()
	defer c.mu.Unlock()
	if c.back == nil {
		return cropBack{}, false
	}
	return *c.back, true
}

func (c *cropBacks) set(b *cropBack) {
	c.mu.Lock()
	c.back = b
	c.mu.Unlock()
}

// corner is what the crop frame's corner offers on the frame dressed d at
// a moment: the crop brought to where the app places it where it stands
// anywhere else and a drag there or on the keyframe that holds it could
// place it, otherwise putting it back where the reset moved it from,
// while the clip is as the reset left it and putting back would move the
// crop on this frame, otherwise nothing.
func (s *FrameFairy) corner(a *dressAsk, d engine.FrameDress, at float64) string {
	if a.video || !d.In {
		return ""
	}
	if d.Moved && (d.Movable || d.Held) {
		return cornerAuto
	}
	b, ok := s.historyOf(a.path).crops.get()
	if !ok || b.after != a.name {
		return ""
	}
	f := b.before
	was := f.source.DressAlone(f.segments, f.rotations, f.marks, at, f.width, f.height)
	if was.Left != d.Left || was.CropWidth != d.CropWidth || was.Turn != d.Turn {
		return cornerBack
	}
	return ""
}

// errNoCropBack answers Put the crop back where there is nothing to put
// back, or the clip changed since the crop was brought back.
var errNoCropBack = errors.New("the crop has changed since, so it cannot be put back")

// ResetCrop brings the crop on the frame at a moment of a clip to where
// the app places it, the way a drag there would, or on the keyframe that
// holds the crop there, and keeps the way back. It puts and removes no
// keyframe. Where the corner offers no reset, the crop already standing
// there or a drag placing nothing, it changes nothing, so a second click
// before the frame's new dress has come is no second reset. It returns
// the clip as it is now.
func (s *FrameFairy) ResetCrop(ctx context.Context, path, plan, clipID string, at float64) (ClipEntry, error) {
	if !s.store.PlanOf(path, plan) {
		return ClipEntry{}, errNotInLibrary
	}
	now, err := s.clipEntry(ctx, path, plan, clipID)
	if err != nil {
		return ClipEntry{}, err
	}
	before, ok := s.previews.framings.get(now.Framing)
	if !ok {
		return now, nil
	}
	d := before.dress(at, false)
	if !d.Movable && d.Held {
		// The keyframe that holds the crop here takes it, and it shows
		// the crop this frame shows.
		at = d.Keyframe
		if k := before.dress(at, false); k.Left == d.Left && k.Auto == d.Auto && k.Turn == d.Turn {
			d = k
		}
	}
	if !d.In || !d.Moved || !d.Movable {
		return now, nil
	}
	back := cropBack{plan: plan, clip: clipID, at: at, left: int(math.Round(d.Left)), before: before}
	if err := s.placeCrop(ctx, path, plan, clipID, at, int(math.Round(d.Auto)), &back); err != nil {
		return ClipEntry{}, err
	}
	return s.answer(ctx, path, plan, clipID)
}

// PutCropBack puts the crop back where ResetCrop moved it from, while the
// clip is as the reset left it, and returns the clip as it is now.
func (s *FrameFairy) PutCropBack(ctx context.Context, path, plan, clipID string) (ClipEntry, error) {
	if !s.store.PlanOf(path, plan) {
		return ClipEntry{}, errNotInLibrary
	}
	crops := &s.historyOf(path).crops
	b, ok := crops.get()
	if !ok || b.plan != plan || b.clip != clipID {
		return ClipEntry{}, errNoCropBack
	}
	now, err := s.clipEntry(ctx, path, plan, clipID)
	if err != nil {
		return ClipEntry{}, err
	}
	if now.Framing != b.after {
		return ClipEntry{}, errNoCropBack
	}
	if err := s.placeCrop(ctx, path, plan, clipID, b.at, b.left, nil); err != nil {
		return ClipEntry{}, err
	}
	return s.answer(ctx, path, plan, clipID)
}

// placeCrop places the crop of a clip at a moment, as one edit, and keeps
// the way back given, or none, with the name of the framing the edit
// leaves. It is kept inside the edit, before the window is told of the
// clip's new framing, so no frame of that framing is dressed without it.
// The crop is one a frame's dress gave, so it lies inside the picture.
func (s *FrameFairy) placeCrop(ctx context.Context, path, plan, clipID string, at float64, left int, back *cropBack) error {
	info, err := s.probe(ctx, path)
	if err != nil {
		return err
	}
	crops := &s.historyOf(path).crops
	return s.edit(path, func() error {
		if err := engine.SetCrop(s.store.files, plan, clipID, at, left, info); err != nil {
			return err
		}
		if back == nil {
			crops.set(nil)
			return nil
		}
		after, err := s.clipEntry(ctx, path, plan, clipID)
		if err != nil {
			crops.set(nil)
			return nil
		}
		back.after = after.Framing
		crops.set(back)
		return nil
	})
}
