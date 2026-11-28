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
	// done says the crop was put back, and after is then the framing
	// that left, so a second click on Put the crop back changes nothing.
	done bool
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
// a moment: the crop brought to where the app places it, where a click
// would place it, see resetOf, otherwise putting it back where the reset
// moved it from, while the clip is as the reset left it and putting back
// would move the crop on this frame, otherwise nothing.
func (s *FrameFairy) corner(a *dressAsk, d engine.FrameDress, at float64) string {
	if a.video || !d.In {
		return ""
	}
	if _, _, ok := resetOf(a.clip, d, at); ok {
		return cornerAuto
	}
	b, ok := s.historyOf(a.path).crops.get()
	if !ok || b.done || b.after != a.name {
		return ""
	}
	f := b.before
	was := f.source.DressAlone(f.segments, f.rotations, f.marks, at, f.width, f.height)
	if was.Left != d.Left || was.CropWidth != d.CropWidth || was.Turn != d.Turn {
		return cornerBack
	}
	return ""
}

// resetOf is where Automatic crop places the crop that the frame dressed d
// at a moment of the framing f shows, and the dress of the frame it is
// placed on: where it stands anywhere but where the app places it, on the
// frame itself where a drag there would place it, or on the switch
// keyframe that holds it, where a drag would place what this frame shows.
// A keyframe placed on the picture turned another way waits, and a drag
// on its frame would take it over, so no click places the crop there.
func resetOf(f framed, d engine.FrameDress, at float64) (float64, engine.FrameDress, bool) {
	if !d.In || !d.Moved {
		return 0, d, false
	}
	if d.Movable && !keyframeWaits(d) {
		return at, d, true
	}
	if !d.Held {
		return 0, d, false
	}
	on := d.Keyframe
	k := f.source.DressAlone(f.segments, f.rotations, f.marks, on, f.width, f.height)
	if !k.In {
		// A keyframe carried past a cut stands in the cut, and carries
		// the crop of the last keyframe of its turn before it, the one
		// the hand put.
		for i := len(f.marks) - 1; i >= 0; i-- {
			if m := f.marks[i]; m.At < on && m.Turn == d.Turn {
				on = m.At
				k = f.source.DressAlone(f.segments, f.rotations, f.marks, on, f.width, f.height)
				break
			}
		}
	}
	if !k.In || !k.Movable || keyframeWaits(k) || k.Left != d.Left || k.Auto != d.Auto || k.Turn != d.Turn {
		return 0, d, false
	}
	return on, k, true
}

// keyframeWaits says the keyframe on a frame was placed on the picture turned
// another way than the frame is.
func keyframeWaits(d engine.FrameDress) bool {
	return d.Mark != nil && d.Mark.Turn != d.Turn
}

var (
	// errNoCropBack answers Put the crop back where there is nothing to
	// put back, or the clip changed since the crop was brought back.
	errNoCropBack = errors.New("the crop has changed since, so it cannot be put back")
	// errFramingGone answers a click whose clip's framing was let go of
	// between being worked out and being read, with 512 others worked
	// out meanwhile.
	errFramingGone = errors.New("the clip changed while the crop was being placed, try again")
)

// ResetCrop brings the crop on the frame at a moment of a clip to where
// the app places it, the way a drag there or on the keyframe that holds it
// would, and keeps the way back. It puts and removes no keyframe. Where
// the corner offers no reset, the crop already standing there or a drag
// placing nothing, it changes nothing, so a second click before the
// frame's new dress has come is no second reset. The clip is read and the
// way back kept inside the edit, so no other edit lands between them, and
// no frame of the framing it leaves is dressed without it. It returns the
// clip as it is now.
func (s *FrameFairy) ResetCrop(ctx context.Context, path, plan, clipID string, at float64) (ClipEntry, error) {
	if !s.store.PlanOf(path, plan) {
		return ClipEntry{}, errNotInLibrary
	}
	info, err := s.probe(ctx, path)
	if err != nil {
		return ClipEntry{}, err
	}
	crops := &s.historyOf(path).crops
	err = s.edit(path, func() error {
		now, err := s.clipEntry(ctx, path, plan, clipID)
		if err != nil {
			return err
		}
		before, ok := s.previews.framings.get(now.Framing)
		if !ok {
			return errFramingGone
		}
		on, d, ok := resetOf(before, before.dress(at, false), at)
		if !ok {
			return nil
		}
		if err := engine.SetCrop(s.store.files, plan, clipID, on, int(math.Round(d.Auto)), info); err != nil {
			return err
		}
		back := &cropBack{plan: plan, clip: clipID, at: on, left: int(math.Round(d.Left)), before: before}
		crops.set(s.leaves(ctx, path, back))
		return nil
	})
	if err != nil {
		return ClipEntry{}, err
	}
	return s.answer(ctx, path, plan, clipID)
}

// PutCropBack puts the crop back where ResetCrop moved it from, while the
// clip is as the reset left it, and returns the clip as it is now. A
// second click before the frame's new dress has come finds it put back
// and changes nothing.
func (s *FrameFairy) PutCropBack(ctx context.Context, path, plan, clipID string) (ClipEntry, error) {
	if !s.store.PlanOf(path, plan) {
		return ClipEntry{}, errNotInLibrary
	}
	info, err := s.probe(ctx, path)
	if err != nil {
		return ClipEntry{}, err
	}
	crops := &s.historyOf(path).crops
	err = s.edit(path, func() error {
		b, ok := crops.get()
		if !ok || b.plan != plan || b.clip != clipID {
			return errNoCropBack
		}
		now, err := s.clipEntry(ctx, path, plan, clipID)
		if err != nil {
			return err
		}
		if now.Framing != b.after {
			return errNoCropBack
		}
		if b.done {
			return nil
		}
		if err := engine.SetCrop(s.store.files, plan, clipID, b.at, b.left, info); err != nil {
			return err
		}
		crops.set(s.leaves(ctx, path, &cropBack{plan: plan, clip: clipID, done: true}))
		return nil
	})
	if err != nil {
		return ClipEntry{}, err
	}
	return s.answer(ctx, path, plan, clipID)
}

// leaves is the way back b with the name of the framing the edit leaves
// its clip with, or none where the clip cannot be read.
func (s *FrameFairy) leaves(ctx context.Context, path string, b *cropBack) *cropBack {
	after, err := s.clipEntry(ctx, path, b.plan, b.clip)
	if err != nil {
		return nil
	}
	b.after = after.Framing
	return b
}
