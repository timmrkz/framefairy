package engine

import (
	"fmt"
	"math"
	"sort"
	"strconv"
	"strings"

	"github.com/spf13/afero"

	"framefairy/engine/num"
)

// Keyframes, plan row 3.26 and #750. A person puts keyframes on frames of
// a clip, the way thumbnails are put, each with the crop where it stands on
// that frame. A keyframe is a turning point: the crop switches on its frame
// and holds until the next keyframe, and before the first it is the clip's
// own. So one keyframe where the camera switches is all a switch takes,
// and the crop after it moves without the crop before it. See
// docs/ROTATE-AND-FOLLOW.md.
//
// A mark is a moment of the video, like a thumbnail, and belongs to the
// turn the picture had when the mark was placed. The keyframes that steer
// a piece are those of its turn in the whole clip, so a keyframe holds
// across a cut by the clip's clock, the time the short plays, with nothing
// of the time the cut took out. A
// rotated part is turned another way, so its crop still starts at its cut. A
// keyframe a trim or a cut left outside the clip stays in the plan and
// comes back with its piece. A part rotated again keeps its keyframes,
// which wait while the picture is turned another way and steer again once
// it is turned back, issue #348: the diamonds used to go with the turn.

// CropMark is a keyframe, a crop placed by hand on one frame: the moment
// the frame starts, in seconds of the video, and the left edge of the crop
// on the picture as it stands there, rotated if the piece is. It was named
// a crop mark until Tim chose keyframe, the word editors use, on #192, and
// the code and the plan's key, crop_marks, keep that name, so plans
// already made keep their keyframes. Turn is how far the picture was
// rotated when the keyframe was placed, the picture X is on.
type CropMark struct {
	At   float64
	X    int
	Turn int
}

// MaxCropMarks is how many keyframes one clip may have.
const MaxCropMarks = 100

// CropMarkView is a keyframe as the window reads it.
type CropMarkView struct {
	At   float64 `json:"at"`
	X    int     `json:"x"`
	Turn int     `json:"turn"`
}

func cropMarkViews(marks []CropMark) []CropMarkView {
	out := make([]CropMarkView, len(marks))
	for i, m := range marks {
		out[i] = CropMarkView{At: m.At, X: m.X, Turn: m.Turn}
	}
	return out
}

// readCropMarks takes a clip's keyframes out of a plan, which is untrusted
// like the rest of it, the way readThumbnails takes its thumbnails: a mark
// is kept only if it has a moment that is a number and a left edge that is
// a whole number in the picture's range, each moment once to the
// millisecond, in time order and no more than MaxCropMarks of them.
// Anything else is left out, and the crop is where the rest put it. A
// keyframe of a plan made before keyframes kept their turn, or with a turn
// that is no quarter, was placed on the picture turned the way the clip's
// rotations turn it at its moment, spans.
func readCropMarks(raw any, spans []Rotation) []CropMark {
	list, ok := raw.([]any)
	if !ok || len(list) == 0 {
		return nil
	}
	seen := map[int64]bool{}
	var out []CropMark
	for i, item := range list {
		if i >= 4*MaxCropMarks {
			break
		}
		entry, ok := asMap(item)
		if !ok {
			continue
		}
		_, atText := entry[keyAt].(string)
		_, xText := entry[keyX].(string)
		if atText || xText {
			continue
		}
		at, ok := num.AsFloat(entry[keyAt])
		if !ok || !num.IsFinite(at) || at < 0 || at > MaxEpisodeSeconds {
			continue
		}
		left, ok := num.AsFloat(entry[keyX])
		if !ok || left != math.Trunc(left) || left < 0 || left >= 1_000_000 {
			continue
		}
		x := int(left)
		key := int64(math.Round(at * 1000))
		if seen[key] {
			continue
		}
		seen[key] = true
		turn := RotationAt(spans, at)
		if n, ok := num.AsFloat(entry[keyTurn]); ok && n == math.Trunc(n) && validRotate(int(n)) {
			turn = int(n)
		}
		out = append(out, CropMark{At: num.RoundTo(at, 3), X: x - x%2, Turn: turn})
	}
	sort.Slice(out, func(a, b int) bool { return out[a].At < out[b].At })
	if len(out) > MaxCropMarks {
		out = out[:MaxCropMarks]
	}
	return out
}

// markedPieces gives each piece of a clip the keyframes that steer it:
// the keyframes of its turn in the whole clip. turns is how far each piece
// is rotated, see rotationOf. A keyframe that lies in no piece
// steers nothing. A keyframe is given to a piece at the moment of the video
// it would be at on that piece's stretch were nothing cut out, the clip's
// clock read from the piece's start, so the crop, which is worked out on
// frames of the video, holds across a cut the way the short plays it. In
// the piece's own part, where nothing is cut out, that is its own moment.
func markedPieces(segments []Segment, turns []int, marks []CropMark) {
	if len(marks) == 0 {
		return
	}
	parts := make([]int, len(segments))
	// Where each piece starts on the clip's clock.
	into := make([]float64, len(segments))
	for i := 1; i < len(segments); i++ {
		parts[i] = parts[i-1]
		if segments[i].Start-segments[i-1].End > PiecesMeet {
			parts[i]++
		}
		into[i] = into[i-1] + segments[i-1].End - segments[i-1].Start
	}
	for _, m := range marks {
		holder := -1
		for i, s := range segments {
			if m.At >= s.Start-frameHair && m.At < s.End-frameHair {
				holder = i
				break
			}
		}
		if holder < 0 {
			continue
		}
		onClip := into[holder] + m.At - segments[holder].Start
		for i := range segments {
			if turns[i] != m.Turn {
				continue
			}
			at := m
			if parts[i] != parts[holder] {
				at.At = segments[i].Start + onClip - into[i]
			}
			segments[i].Marks = append(segments[i].Marks, at)
		}
	}
}

// markFrame is the frame of the video a moment means, by the frame rate
// and the grid the frames are on, see nearestFrame, and the millisecond
// where the rate is not known. The crop is worked out on these.
func (s SourceInfo) markFrame(at float64) int64 {
	if s.FPSNum <= 0 || s.FPSDen <= 0 {
		return int64(math.Round(at * 1000))
	}
	return s.nearestFrame(at)
}

// frameHolding is the frame of the video that holds a moment, the one the
// video preview shows with the playhead there: the last to start at or
// before it, give or take frameHair, see frameAt in lib/flow.ts. The
// millisecond where the rate is not known.
func (s SourceInfo) frameHolding(at float64) int64 {
	if s.FPSNum <= 0 || s.FPSDen <= 0 {
		return int64(math.Round(at * 1000))
	}
	num, den := int64(s.FPSNum), int64(s.FPSDen)
	us := int64(math.Round((at+frameHair)*1_000_000)) - int64(math.Round(s.VideoStart*1_000_000))
	return floorDiv(max(us, 0)*num, den*1_000_000)
}

// frameStartHolding is the start of the frame that holds a moment, to the
// millisecond the plan keeps, or the moment itself where the rate is not
// known.
func (s SourceInfo) frameStartHolding(at float64) float64 {
	if s.FPSNum <= 0 || s.FPSDen <= 0 {
		return num.RoundTo(at, 3)
	}
	return s.keptAt(s.frameHolding(at))
}

// frameMark is a keyframe on its frame, with its left edge kept inside
// the picture.
type frameMark struct {
	k int64
	x int
}

// onFrames is a piece's keyframes on their frames, each left edge kept
// inside a picture so wide for a crop so wide. Two keyframes on one frame
// keep the later.
func (s SourceInfo) onFrames(marks []CropMark, cropW, width int) []frameMark {
	var out []frameMark
	for _, m := range marks {
		x := m.X
		f := frameMark{k: s.markFrame(m.At), x: ClampCropX(&x, cropW, width)}
		if n := len(out); n > 0 && out[n-1].k == f.k {
			out[n-1] = f
			continue
		}
		out = append(out, f)
	}
	return out
}

// heldX is the left edge of the crop on frame k: that of the last keyframe
// on or before it, or base, the crop of the piece, before the first. A
// keyframe is a turning point, Tim's word on #750: the crop switches on its
// frame and holds until the next keyframe, so one keyframe at a camera
// switch is all a switch takes. The crop used to glide from one keyframe
// to the next, so one keyframe stood still and a switch took two, a frame
// apart.
//
// The render works the same choice out in ffmpeg, see holdExpr, and the
// video preview in lib/keyframes.ts, so the three agree on every frame.
func heldX(marks []frameMark, k int64, base int) int {
	x := base
	for _, m := range marks {
		if m.k > k {
			break
		}
		x = m.x
	}
	return x
}

// holdExpr is heldX as an ffmpeg expression for the crop filter of a piece
// whose first frame is frame first of the video, starting at the left edge
// start and switching to each of steps on its frame. The crop filter counts
// the frames it is given as n, from nought, so frame k of the video is
// n+first. The switches are found by halving, so the expression nests as
// deep as the number of keyframes has binary digits: ffmpeg refuses one
// nested deeper than a hundred, and one if per keyframe made a clip of
// ninety keyframes unrenderable.
func holdExpr(start int, steps []frameMark, first int64) string {
	k := "(n+" + strconv.FormatInt(first, 10) + ")"
	if first < 0 {
		k = "(n-" + strconv.FormatInt(-first, 10) + ")"
	}
	// value i is start for nought and the left edge of steps[i-1] after.
	value := func(i int) int {
		if i == 0 {
			return start
		}
		return steps[i-1].x
	}
	var pick func(lo, hi int) string
	pick = func(lo, hi int) string {
		if lo == hi {
			return strconv.Itoa(value(lo))
		}
		mid := (lo + hi + 1) / 2
		return fmt.Sprintf("if(lt(%s,%d),%s,%s)", k, steps[mid-1].k, pick(lo, mid-1), pick(mid, hi))
	}
	return pick(0, len(steps))
}

// cropFilters are the filters that crop a piece for the render and scale
// it to outW by outH, or leave it as cropped where those are the crop's
// own size. Every left edge is on an even pixel, the way the colours of a
// picture are shared between two columns, so a crop that switches at its
// keyframes is one crop filter whose left edge is worked out on every
// frame, see holdExpr.
func (s SourceInfo) cropFilters(seg Segment, cropW, cropH, cropY, width, outW, outH int, scale string) []string {
	scaled := func(crop string) []string {
		out := []string{crop}
		if outW != cropW || outH != cropH {
			out = append(out, fmt.Sprintf("scale=%d:%d:%s", outW, outH, scale))
		}
		return out
	}
	base := ClampCropX(seg.CropX, cropW, width)
	if len(seg.Marks) == 0 {
		return scaled(fmt.Sprintf("crop=%d:%d:%d:%d", cropW, cropH, base, cropY))
	}
	marks := s.onFrames(seg.Marks, cropW, width)
	if s.FPSNum <= 0 || s.FPSDen <= 0 {
		return scaled(fmt.Sprintf("crop=%d:%d:%d:%d", cropW, cropH, heldX(marks, s.markFrame(seg.Start), base), cropY))
	}
	first, end := s.pieceFrames(seg.Start, seg.End)
	start := heldX(marks, first, base)
	// The switches inside the piece, each to a crop other than the one
	// before it.
	var steps []frameMark
	x := start
	for _, m := range marks {
		if m.k <= first || m.k >= end || m.x == x {
			continue
		}
		steps = append(steps, m)
		x = m.x
	}
	if len(steps) == 0 {
		return scaled(fmt.Sprintf("crop=%d:%d:%d:%d", cropW, cropH, start, cropY))
	}
	return scaled(fmt.Sprintf("crop=%d:%d:'%s':%d", cropW, cropH, holdExpr(start, steps, first), cropY))
}

// CropAt is the left edge of a piece's crop on the frame that holds a
// moment, on the picture: that of the keyframe it holds from, or the
// piece's own.
func (s SourceInfo) CropAt(seg Segment, at float64, cropW, width int) int {
	base := ClampCropX(seg.CropX, cropW, width)
	if len(seg.Marks) == 0 {
		return base
	}
	return heldX(s.onFrames(seg.Marks, cropW, width), s.frameHolding(at), base)
}

// cropMarksOf is the keyframes a clip of a plan being edited has, as the
// plan holds them.
func cropMarksOf(c *object) []CropMark {
	return readCropMarks(c.values[keyCropMarks], rotationsOf(c))
}

// setCropMarks writes a clip's keyframes in time order, or takes the key
// away when there are none.
func setCropMarks(c *object, marks []CropMark) {
	if len(marks) == 0 {
		c.remove(keyCropMarks)
		return
	}
	sort.Slice(marks, func(a, b int) bool { return marks[a].At < marks[b].At })
	list := make([]any, len(marks))
	for i, m := range marks {
		o := newObject()
		o.set(keyAt, num.RoundTo(m.At, 3))
		o.set(keyX, m.X)
		o.set(keyTurn, m.Turn)
		list[i] = o
	}
	c.set(keyCropMarks, list)
}

// pieceHolding is the piece of a plan being edited that holds a moment, as
// the render cuts it, or minus one for a moment in a cut or outside the
// clip.
func pieceHolding(pieces []*object, at float64) int {
	for i, seg := range pieces {
		if at >= number(seg.values[keyStart])-frameHair && at < number(seg.values[keyEnd])-frameHair {
			return i
		}
	}
	return -1
}

// marksTurned is which of a clip's keyframes belong to one turn of the
// picture, by their index, so a crop placed or put back for that turn
// takes them along too.
func marksTurned(marks []CropMark, turn int) map[int]bool {
	out := map[int]bool{}
	for i, m := range marks {
		if m.Turn == turn {
			out[i] = true
		}
	}
	return out
}

// SetCropMark adds, moves or removes a keyframe of a clip, the way
// SetThumbnail does a thumbnail: from is the mark as it is and to where it
// goes, both in seconds of the video. A from below nought adds one at to
// with its crop at x, and a to below nought removes the one at from. A
// keyframe lands on the start of the frame that holds its moment, the frame
// the video preview shows there, and has to be inside a piece the clip
// keeps, because a moment that was cut is not in the short.
// Framing is not a judgement of the clip, so nothing is recorded for
// training.
func SetCropMark(fsys afero.Fs, planPath, clipID string, from, to float64, x int, frames SourceInfo) error {
	adding, removing := from < 0, to < 0
	if adding && removing {
		return renderErr("a keyframe has to be somewhere")
	}
	for _, at := range []float64{from, to} {
		if math.IsNaN(at) || math.IsInf(at, 0) || at > MaxEpisodeSeconds {
			return renderErr("a keyframe cannot be at %s", num.Fixed(at, 3))
		}
	}
	if adding && (x < 0 || x >= 1_000_000) {
		return renderErr("the crop cannot start left of the picture")
	}
	return editPlan(fsys, planPath, func(_ *object, clips []*object) error {
		c, err := findClip(clips, clipID)
		if err != nil {
			return err
		}
		marks := cropMarksOf(c)
		pieces := segmentObjects(c)
		var kept []CropMark
		var moving *CropMark
		if !adding {
			// from is the keyframe's own moment, so it is found on its own
			// frame, wherever in the frame a hand edit or an older plan
			// put it. Two keyframes a hand edit put on one frame go together,
			// the later one moving, because the crop only ever used it.
			for i := range marks {
				if frames.markFrame(marks[i].At) == frames.markFrame(from) {
					m := marks[i]
					moving = &m
					continue
				}
				kept = append(kept, marks[i])
			}
			if moving == nil {
				return renderErr("the clip has no keyframe at %s", HMS(from))
			}
		} else {
			kept = marks
		}
		if !removing {
			at := frames.frameStartHolding(to)
			if pieceHolding(pieces, at) < 0 {
				return renderErr("%s is not in the clip", HMS(to))
			}
			for _, m := range kept {
				if frames.markFrame(m.At) == frames.markFrame(at) {
					return renderErr("the clip already has a keyframe at %s", HMS(to))
				}
			}
			if len(kept) >= MaxCropMarks {
				return renderErr("a clip can have %d keyframes", MaxCropMarks)
			}
			mark := CropMark{At: at, X: x - x%2, Turn: RotationAt(rotationsOf(c), at)}
			if moving != nil {
				mark.X, mark.Turn = moving.X, moving.Turn
			}
			kept = append(kept, mark)
		}
		setCropMarks(c, kept)
		return nil
	})
}

// cropShown is the left edge of the crop a piece of a plan being edited
// shows on the frame a moment means: that of the keyframe of its turn in
// the clip it holds from, kept inside a picture so wide for a crop so wide
// the way the render keeps it, or the crop of its turn, or nil where it
// stands in the middle. A width no wider than the crop is a picture not
// known, and the keyframes are taken as they are.
func cropShown(c *object, pieces []*object, target int, at float64, frames SourceInfo, cropW, width int) *int {
	if i := markHolding(c, pieces, target, at, frames); i >= 0 {
		if width <= cropW {
			width = math.MaxInt32
		}
		x := cropMarksOf(c)[i].X
		left := ClampCropX(&x, cropW, width)
		return &left
	}
	spans := rotationsOf(c)
	key := keyCropX
	if rotationOf(pieces[target], spans) != 0 {
		key = keyCropXTurned
	}
	if n, ok := num.AsInt(pieces[target].values[key]); ok {
		return &n
	}
	return nil
}

// markHolding is which of a clip's keyframes the crop of a piece of a plan
// being edited holds from on the frame a moment means, by its index, or
// minus one before the first keyframe of the piece's turn, where the crop
// is the piece's own. It is the keyframe heldX would take, found on the
// clip's clock the way markedPieces gives a piece its keyframes.
func markHolding(c *object, pieces []*object, target int, at float64, frames SourceInfo) int {
	spans := rotationsOf(c)
	segments := make([]Segment, len(pieces))
	turns := make([]int, len(pieces))
	for i, seg := range pieces {
		segments[i] = Segment{Start: number(seg.values[keyStart]), End: number(seg.values[keyEnd])}
		turns[i] = rotationOf(seg, spans)
	}
	marks := cropMarksOf(c)
	// Each keyframe given on its own, so the one the piece holds from is
	// known by where it stands in the clip's list.
	k := frames.frameHolding(at)
	holding, from := -1, int64(math.MinInt64)
	for i, m := range marks {
		alone := make([]Segment, len(segments))
		copy(alone, segments)
		markedPieces(alone, turns, []CropMark{m})
		for _, on := range alone[target].Marks {
			if f := frames.markFrame(on.At); f <= k && f >= from {
				holding, from = i, f
			}
		}
	}
	return holding
}

// marksText is a clip's keyframes for the log, each its moment and left
// edge.
func marksText(marks []CropMark) string {
	parts := make([]string, len(marks))
	for i, m := range marks {
		parts[i] = num.Fixed(m.At, 3) + "@" + strconv.Itoa(m.X)
	}
	return strings.Join(parts, " ")
}
