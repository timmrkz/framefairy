package engine

import (
	"context"
	"strings"
	"sync/atomic"
	"time"

	"framefairy/engine/runes"
)

// PreviewTimes is where the time of a stream of frames goes, counted from
// the moment it was asked for: until the decoder has the request, until it
// is at the place asked for, and until the first frame is out. Each is in
// milliseconds and 0 until it has happened. File says whether getting to
// the place meant opening the file and a decoder, or only moving a cursor
// that had them, and WentOn whether the cursor read on from where it stood
// without a seek, so a stream asked for its key frame first hands over
// none, issue #511. A jump in the video preview costs all of it, and knowing
// which part is large decides what makes it quicker, see
// docs/VIDEO-PREVIEW.md, Speed.
//
// The episode's decoder, EpisodeFrames, has the request at once where it
// runs and after starting it where it did not, and is at the place when it
// says the open is done.
type PreviewTimes struct {
	asked                  time.Time
	Started, Opened, First atomic.Int64
	File, WentOn           atomic.Bool
}

// NewPreviewTimes starts counting now.
func NewPreviewTimes() *PreviewTimes {
	return &PreviewTimes{asked: time.Now()}
}

// mark notes a moment once, on a stream that counts its times at all, so
// a stream that starts over still counts from its first.
func (t *PreviewTimes) mark(at func(*PreviewTimes) *atomic.Int64) {
	if t != nil {
		at(t).CompareAndSwap(0, max(1, time.Since(t.asked).Milliseconds()))
	}
}

func timeStarted(t *PreviewTimes) *atomic.Int64 { return &t.Started }
func timeOpened(t *PreviewTimes) *atomic.Int64  { return &t.Opened }
func timeFirst(t *PreviewTimes) *atomic.Int64   { return &t.First }

type previewTimesKey struct{}

// WithPreviewTimes has the episode's decoder note its times in t.
func WithPreviewTimes(ctx context.Context, t *PreviewTimes) context.Context {
	return context.WithValue(ctx, previewTimesKey{}, t)
}

func previewTimesOf(ctx context.Context) *PreviewTimes {
	t, _ := ctx.Value(previewTimesKey{}).(*PreviewTimes)
	return t
}

// tail keeps the last lines ffmpeg said that were not about a frame, for
// the reason when it fails.
type tail struct {
	lines []string
}

func (t *tail) add(line string) {
	t.lines = append(t.lines, line)
	if len(t.lines) > 8 {
		t.lines = t.lines[1:]
	}
}

func (t *tail) String() string {
	return runes.Scrub(strings.Join(t.lines, " "), 400)
}
