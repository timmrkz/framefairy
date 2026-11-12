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
// that frame. A keyframe is a turning point: from its frame on the crop is
// the keyframe's, until the next keyframe, and before the first it is the
// clip's own. A keyframe switches the crop on its frame, so one keyframe
// where the camera switches is all a switch takes, or it is a circle,
// which glides: from a circle the crop glides into the next circle, over
// the whole time between them, and reaches it on its frame. A circle
// after a diamond, or the first, starts a glide from the crop before it
// and has no crop of its own, see opened, because a diamond is a switch
// with nothing happening before or after it, and a circle never makes the
// crop jump. Tim
// chose the two kinds, a diamond put with K and a circle put with G, from
// mockups on #750, the switch keyframe on S since #778, and that a glide runs only between circles. See
// docs/ROTATE-AND-FOLLOW.md.
//
// A mark is a moment of the video, like a thumbnail, and belongs to the
// turn the picture had when the mark was placed. The keyframes that steer
// a piece are those of its turn in the whole clip, so a keyframe holds
// across a cut by the clip's clock, the time the short plays, with nothing
// of the time the cut took out. A
// rotated part is turned another way, so its crop still starts at its cut. A
// keyframe a trim or a cut left outside the clip stays in the plan, still
// switches the crop from the next frame kept, and its diamond comes back
// with its piece. A part rotated again keeps its keyframes,
// which wait while the picture is turned another way and steer again once
// it is turned back, issue #348: the diamonds used to go with the turn.

// CropMark is a keyframe, a crop placed by hand on one frame: the moment
// the frame starts, in seconds of the video, and the left edge of the crop
// on the picture as it stands there, rotated if the piece is. It was named
// a crop mark until Tim chose keyframe, the word editors use, on #192, and
// the code and the plan's key, crop_marks, keep that name, so plans
// already made keep their keyframes. Turn is how far the picture was
// rotated when the keyframe was placed, the picture X is on. Glide is a
// circle, which the crop glides into from a circle before it, rather than
// switching on its frame.
//
// Aside is a keyframe an edge or a cut covers that was set aside when the
// one it carried was removed: it is not carried, and steers nothing, until
// the edge leaves it inside the clip again, see carry. Last is never in a
// plan: it is a keyframe the clip's end carries, which stands on the last
// frame before At, the end, rather than on At.
//
// Opens is a circle that starts a glide, one no circle of its turn comes
// right before. It has no crop of its own: the glide starts from the crop
// before it, so X is that crop, and FromClip says it is the piece's own
// crop, with no keyframe of its turn before. Neither is in a plan, see
// opened. Alone is a circle no circle of its turn stands beside among the
// keyframes the clip shows, so no glide runs into or out of it and it does
// nothing yet, see carried.
type CropMark struct {
	At       float64
	X        int
	Turn     int
	Glide    bool
	Aside    bool
	Last     bool
	Opens    bool
	FromClip bool
	Alone    bool
}

// MaxCropMarks is how many keyframes one clip may have.
const MaxCropMarks = 100

// CropMarkView is a keyframe as the window reads it. Last is one the
// clip's end carries, see CropMark, until MarksOnFrames puts it on its frame.
type CropMarkView struct {
	At       float64 `json:"at"`
	X        int     `json:"x"`
	Turn     int     `json:"turn"`
	Glide    bool    `json:"glide"`
	Opens    bool    `json:"opens,omitempty"`
	Alone    bool    `json:"alone,omitempty"`
	FromClip bool    `json:"fromClip,omitempty"`
	Last     bool    `json:"-"`
}

// CropMarkViews is keyframes as the window reads them.
func CropMarkViews(marks []CropMark) []CropMarkView {
	out := make([]CropMarkView, len(marks))
	for i, m := range marks {
		out[i] = CropMarkView{At: m.At, X: m.X, Turn: m.Turn, Glide: m.Glide, Opens: m.Opens, Alone: m.Alone, Last: m.Last, FromClip: m.FromClip}
	}
	return out
}

// MarksOnFrames is keyframes as the window is sent them, each on the moment
// its frame starts: one the clip's end carries on the clip's last frame,
// unless it gives way there, see givesWay.
func (s SourceInfo) MarksOnFrames(marks []CropMarkView) []CropMarkView {
	all := make([]CropMark, len(marks))
	for i, m := range marks {
		all[i] = CropMark{At: m.At, Last: m.Last}
	}
	out := make([]CropMarkView, 0, len(marks))
	for i, m := range marks {
		if m.Last {
			if s.givesWay(all[i], all) {
				continue
			}
			m.At, m.Last = s.frameAt(s.markFrame(m.At)-1), false
		}
		out = append(out, m)
	}
	return out
}

// givesWay is whether a keyframe the clip's end carries gives the clip's
// last frame, where it would stand, to a keyframe of that frame's own
// among marks. The start and a cut never carry one onto a frame that has
// its own, see carry, but the end carries without knowing the frames, so
// this is where that rule is kept for it, for what the window is sent,
// what steers the crop and what a touch makes real.
func (s SourceInfo) givesWay(m CropMark, marks []CropMark) bool {
	if !m.Last {
		return false
	}
	k := s.markFrame(m.At) - 1
	for _, o := range marks {
		if !o.Last && s.markFrame(o.At) == k {
			return true
		}
	}
	return false
}

// frameAt is the moment frame k starts, as markFrame counts frames.
func (s SourceInfo) frameAt(k int64) float64 {
	if s.FPSNum <= 0 || s.FPSDen <= 0 {
		return float64(k) / 1000
	}
	return s.keptAt(k)
}

// readCropMarks takes a clip's keyframes out of a plan, which is untrusted
// like the rest of it, the way readThumbnails takes its thumbnails: a mark
// is kept only if it has a moment that is a number and a left edge that is
// a whole number in the picture's range, each moment once to the
// millisecond, in time order and no more than MaxCropMarks of them.
// Anything else is left out, and the crop is where the rest put it. A
// keyframe of a plan made before keyframes kept their turn, or with a turn
// that is no quarter, was placed on the picture turned the way the clip's
// rotations turn it at its moment, spans. A keyframe glides only where
// its glide is true, and switches for anything else.
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
		glide, _ := entry[keyGlide].(bool)
		aside, _ := entry[keyAside].(bool)
		out = append(out, CropMark{At: num.RoundTo(at, 3), X: x - x%2, Turn: turn, Glide: glide, Aside: aside})
	}
	sort.Slice(out, func(a, b int) bool { return out[a].At < out[b].At })
	if len(out) > MaxCropMarks {
		out = out[:MaxCropMarks]
	}
	return out
}

// markedPieces gives each piece of a clip the keyframes that steer it:
// the keyframes of its turn in the whole clip. turns is how far each piece
// is rotated, see rotationOf. A keyframe is given to a piece at the moment
// of the video it would be at on that piece's stretch were nothing cut
// out, the clip's clock read from the piece's start, so the crop, which is
// worked out on frames of the video, holds across a cut the way the short
// plays it. In the piece's own part, where nothing is cut out, that is its
// own moment.
//
// A keyframe in a cut, or before the clip's start, still switches the crop:
// it holds from the first frame kept after it, the way it would were the cut
// taken back. A camera switch often lies in the pause a cut takes out, and a
// keyframe left out there played the new angle with the old crop for the
// rest of the clip, found by the review of #752. A keyframe after the clip's
// end steers nothing.
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
		var onClip float64
		if m.Last {
			// On the clip's last frame, which the end carries it on.
			last := len(segments) - 1
			holder, onClip = last, into[last]+segments[last].End-segments[last].Start
		}
		for i, s := range segments {
			if holder >= 0 {
				break
			}
			if m.At < s.Start-frameHair {
				// In a cut or before the clip: from the start of the next
				// piece kept.
				holder, onClip = i, into[i]
				break
			}
			if m.At < s.End-frameHair {
				holder, onClip = i, into[i]+m.At-s.Start
				break
			}
		}
		if holder < 0 {
			continue
		}
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

// The snow shovel, #778. An edge of a clip, or a cut, that is dragged over
// keyframes carries them, Tim's word: the keyframe it reached last stands
// on the clip's first frame, its last, or the first frame after the cut,
// with its own crop, and the others it covers wait out of sight. Each
// keeps its own moment in the plan, so the edge dragged back out leaves
// each where it was. A circle carried with no circle beside it rides on
// the edge on its own, an empty ring that glides nowhere, until the edge
// reaches another keyframe and carries that one instead, Tim's report of
// 12 November: it is pushed, never dropped. Tim agreed the rules on 10 November, and that a carried keyframe keeps its
// own crop: an edge dragged in over a glide makes the glide shorter and
// faster, and dragged out longer and slower. The doc is "Keyframes at a
// dragged edge".

// carriage is a keyframe a clip shows: from, its place in the plan's
// keyframes, and for one an edge or a cut carries, covers, the others that
// edge or cut covers.
type carriage struct {
	mark    CropMark
	from    int
	carried bool
	covers  []int
}

// coverOf is what covers a moment of a clip with these pieces: the cut
// before piece i, or before the start for nought, the end for the number
// of pieces, or minus one for a moment inside a piece. It is markedPieces'
// reading, give or take frameHair.
func coverOf(segments []Segment, at float64) int {
	for i, s := range segments {
		if at < s.Start-frameHair {
			return i
		}
		if at < s.End-frameHair {
			return -1
		}
	}
	return len(segments)
}

// carry is a clip's keyframes as they stand with its pieces, in time
// order: each inside a piece on its own moment, and of those the start, a
// cut or the end covers, the one it carries. The start and a cut carry
// the latest they cover, on the first frame of the piece after them, and
// the end the earliest, on the last frame of the clip. A keyframe set
// aside is never carried. One already on that frame keeps it, and nothing
// is carried there.
func carry(segments []Segment, marks []CropMark) []carriage {
	if len(segments) == 0 {
		return nil
	}
	marks = opened(marks)
	var out []carriage
	covered := map[int][]int{}
	for i, m := range marks {
		if c := coverOf(segments, m.At); c >= 0 {
			covered[c] = append(covered[c], i)
			continue
		}
		m.Aside = false
		out = append(out, carriage{mark: m, from: i})
	}
	n := len(segments)
	for c, list := range covered {
		var live []int
		for _, i := range list {
			if !marks[i].Aside {
				live = append(live, i)
			}
		}
		if len(live) == 0 {
			continue
		}
		pick := live[len(live)-1]
		at := 0.0
		if c == n {
			pick, at = live[0], segments[n-1].End
		} else {
			at = segments[c].Start
		}
		taken := false
		for _, o := range out {
			taken = taken || (!o.carried && math.Abs(o.mark.At-at) <= frameHair)
		}
		if taken {
			continue
		}
		m := marks[pick]
		m.At, m.Last, m.Aside = at, c == n, false
		var covers []int
		for _, i := range list {
			if i != pick {
				covers = append(covers, i)
			}
		}
		out = append(out, carriage{mark: m, from: pick, carried: true, covers: covers})
	}
	sort.SliceStable(out, func(a, b int) bool {
		if out[a].mark.At != out[b].mark.At {
			return out[a].mark.At < out[b].mark.At
		}
		return !out[a].mark.Last && out[b].mark.Last
	})
	return out
}

// opened is keyframes in time order with each circle that starts a glide
// marked Opens and given the crop before it, the crop of the keyframe of
// its turn before it, or FromClip where there is none. Tim on #778: "the
// first glide keyframe starts the transition based on the crop that was
// set previously", and a crop that jumps is a switch keyframe's. It is
// worked out on the clip as were nothing cut out, before an edge or a cut
// carries anything, so a circle an edge carries starts the glide from the
// same crop it did. A keyframe set aside steers nothing, so it is no crop
// before.
func opened(marks []CropMark) []CropMark {
	if len(marks) == 0 {
		return marks
	}
	out := append([]CropMark(nil), marks...)
	for i, m := range marks {
		out[i].Opens, out[i].FromClip = false, false
		if !m.Glide {
			continue
		}
		before := -1
		for j := i - 1; j >= 0; j-- {
			if marks[j].Turn == m.Turn && !marks[j].Aside {
				before = j
				break
			}
		}
		if before >= 0 && marks[before].Glide {
			continue
		}
		out[i].Opens = true
		if before < 0 {
			out[i].FromClip = true
		} else {
			out[i].X = marks[before].X
		}
	}
	return out
}

// joined is whether the keyframe shown at i has a circle of its turn
// right before or after it.
func joined(shown []carriage, i int) bool {
	turn := shown[i].mark.Turn
	for j := i - 1; j >= 0; j-- {
		if shown[j].mark.Turn == turn {
			if shown[j].mark.Glide {
				return true
			}
			break
		}
	}
	for j := i + 1; j < len(shown); j++ {
		if shown[j].mark.Turn == turn {
			return shown[j].mark.Glide
		}
	}
	return false
}

// carried is the keyframes a clip with these pieces shows, and that steer
// its crop, see carry. A circle no circle of its turn stands beside is
// Alone: it does nothing until a second is put, so the clip timeline
// shows it as doing nothing, Tim on #778.
func carried(segments []Segment, marks []CropMark) []CropMark {
	shown := carry(segments, marks)
	if len(shown) == 0 {
		return nil
	}
	out := make([]CropMark, len(shown))
	for i, c := range shown {
		out[i] = c.mark
		out[i].Alone = c.mark.Glide && !joined(shown, i)
	}
	return out
}

// spansOf is the pieces of a clip of a plan being edited, start and end.
func spansOf(pieces []*object) []Segment {
	out := make([]Segment, len(pieces))
	for i, seg := range pieces {
		out[i] = Segment{Start: number(seg.values[keyStart]), End: number(seg.values[keyEnd])}
	}
	return out
}

// realise makes the keyframe a clip shows on frame k one of its own there,
// when an edge or a cut carries it: Tim's rule that touching a carried
// keyframe makes it real, by placing the crop on it, pressing S or G on it
// or dragging it. Its moment in the plan becomes the frame it stands on,
// its old place is forgotten, and the others the same edge or cut covers
// are set aside, so none of them comes up on the edge when it leaves.
// Keyframes on no carried frame are left as they are.
func realise(frames SourceInfo, pieces []Segment, marks []CropMark, k int64) []CropMark {
	shown := carry(pieces, marks)
	all := make([]CropMark, len(shown))
	for i, c := range shown {
		all[i] = c.mark
	}
	for _, c := range shown {
		if !c.carried || frames.givesWay(c.mark, all) {
			continue
		}
		on := frames.markFrame(c.mark.At)
		if c.mark.Last {
			on--
		}
		if on != k {
			continue
		}
		out := append([]CropMark(nil), marks...)
		out[c.from].At, out[c.from].Aside = frames.frameAt(on), false
		for _, i := range c.covers {
			out[i].Aside = true
		}
		return out
	}
	return marks
}

// unsetAside lets the keyframes set aside that a clip's pieces hold again
// be carried once more, since the edge that covered them was dragged back
// out past them, and answers whether any changed.
func unsetAside(pieces []Segment, marks []CropMark) bool {
	changed := false
	for i := range marks {
		if marks[i].Aside && coverOf(pieces, marks[i].At) < 0 {
			marks[i].Aside, changed = false, true
		}
	}
	return changed
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
// the picture, and whether it is a circle.
type frameMark struct {
	k     int64
	x     int
	glide bool
	// fromClip is a circle whose glide starts from the piece's own crop,
	// see CropMark.FromClip.
	fromClip bool
}

// onFrames is a piece's keyframes on their frames, each left edge kept
// inside a picture so wide for a crop so wide. Two keyframes on one frame
// keep the later.
func (s SourceInfo) onFrames(marks []CropMark, cropW, width int) []frameMark {
	var out []frameMark
	for _, m := range marks {
		if s.givesWay(m, marks) {
			continue
		}
		x := m.X
		f := frameMark{k: s.markFrame(m.At), x: ClampCropX(&x, cropW, width), glide: m.Glide, fromClip: m.FromClip}
		if m.Last {
			f.k--
		}
		if n := len(out); n > 0 && out[n-1].k == f.k {
			out[n-1] = f
			continue
		}
		out = append(out, f)
	}
	return out
}

// cropRun is how the crop of a piece runs over the frames of the video:
// its keyframes on their frames, and the piece's own crop, base, before
// the first.
type cropRun struct {
	marks []frameMark
	base  int
}

// runOf is the crop of a piece as it runs over the frames, its left edges
// kept inside a picture so wide for a crop so wide.
func (s SourceInfo) runOf(seg Segment, cropW, width int) cropRun {
	r := cropRun{
		marks: s.onFrames(seg.Marks, cropW, width),
		base:  ClampCropX(seg.CropX, cropW, width),
	}
	for i := range r.marks {
		if r.marks[i].fromClip {
			r.marks[i].x = r.base
		}
	}
	return r
}

// before is where the crop stands as the stretch up to keyframe i starts:
// the keyframe before, or the piece's own crop before the first.
func (r cropRun) before(i int) frameMark {
	if i > 0 {
		return r.marks[i-1]
	}
	return frameMark{x: r.base}
}

// gliding is whether the crop glides into keyframe i: it glides only from
// a circle into the next circle, to a crop of its own over at least one
// frame. A diamond is a switch, with nothing happening before or after it,
// so a circle after a diamond, or the first of the clip, is where a glide
// starts, from the crop before it, see opened. Tim on #750: "The only
// gradient connection to be shown is between circles, ... because
// between those a glide happens, nowhere else."
func (r cropRun) gliding(i int) bool {
	if i == 0 || !r.marks[i-1].glide {
		return false
	}
	a, b := r.marks[i-1], r.marks[i]
	return b.glide && b.k > a.k && b.x != a.x
}

// at is the left edge of the crop on frame k. From a keyframe's frame on
// it is that keyframe's, until the next, and before the first it is the
// piece's own. Into a keyframe that switches, the crop jumps on its frame.
// Into a circle after a circle, it moves the whole way from that circle,
// by smoothstep, u²(3-2u), which starts and settles gently the way a
// camera operator pans, and reaches it on its frame.
//
// A glide is not rounded to a pixel of the picture. A slow glide moves
// less than a pixel a frame, and rounded it stood still for frames and
// then jumped, which was the stutter Tim saw on #192. The render places
// it to the pixel of the short instead, see glideCrop, and works the same
// sum out in ffmpeg, see expr, step by step in the same order, each a
// double rounded on its own. Each step is converted on its own here,
// which keeps Go from fusing a multiply and an add into one instruction,
// as it may on arm64. The video preview is sent the result, see CropSteps.
func (r cropRun) at(k int64) float64 {
	i := sort.Search(len(r.marks), func(i int) bool { return r.marks[i].k > k })
	a := r.before(i)
	if i < len(r.marks) && r.gliding(i) && k >= a.k {
		b := r.marks[i]
		u := float64(k-a.k) / float64(b.k-a.k)
		ease := float64(float64(u*u) * float64(3-float64(2*u)))
		return float64(a.x) + float64(float64(b.x-a.x)*ease)
	}
	return float64(a.x)
}

// glidesIn is whether the crop glides on any of the frames first to end,
// end left out.
func (r cropRun) glidesIn(first, end int64) bool {
	for i := range r.marks {
		if r.gliding(i) && r.before(i).k < end-1 && r.marks[i].k > first {
			return true
		}
	}
	return false
}

// expr is at as an ffmpeg expression for the crop filter of a piece whose
// first frame is frame first of the video. The crop filter counts the
// frames it is given as n, from nought, so frame k of the video is
// n+first. Every step is written in the order at takes it. The stretches
// between keyframes are found by halving, so the expression nests as deep
// as the number of keyframes has binary digits: ffmpeg refuses one nested
// deeper than a hundred, and one if per keyframe made a clip of ninety
// keyframes unrenderable.
func (r cropRun) expr(first int64) string {
	k := "(n+" + strconv.FormatInt(first, 10) + ")"
	if first < 0 {
		k = "(n-" + strconv.FormatInt(-first, 10) + ")"
	}
	n := len(r.marks)
	// Stretch i runs up to keyframe i, and stretch n after the last.
	value := func(i int) string {
		a := r.before(i)
		if i == n || !r.gliding(i) {
			return strconv.Itoa(a.x)
		}
		b := r.marks[i]
		u := fmt.Sprintf("((%s-%d)/%d)", k, a.k, b.k-a.k)
		return fmt.Sprintf("(%d+%d*(%s*%s*(3-2*%s)))", a.x, b.x-a.x, u, u, u)
	}
	var pick func(lo, hi int) string
	pick = func(lo, hi int) string {
		if lo == hi {
			return value(lo)
		}
		mid := (lo + hi + 1) / 2
		return fmt.Sprintf("if(lt(%s,%d),%s,%s)", k, r.marks[mid-1].k, pick(lo, mid-1), pick(mid, hi))
	}
	return pick(0, n)
}

// holdExpr is the crop of a piece that only switches, as an ffmpeg
// expression for the crop filter of a piece whose first frame is frame
// first of the video, starting at the left edge start and switching to
// each of steps on its frame, found by halving the way expr does. Every
// left edge is a whole, even pixel, so it needs nothing of the second crop
// a glide takes.
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

// heldCrops is how the crop of a piece that only switches runs over its
// frames first to end: the left edge it starts with, and each switch
// inside the piece after it, to a crop other than the one before it.
func (r cropRun) heldCrops(first, end int64) (start int, steps []frameMark) {
	start = int(r.at(first))
	x := start
	for _, m := range r.marks {
		if m.k <= first || m.k >= end || m.x == x {
			continue
		}
		steps = append(steps, m)
		x = m.x
	}
	return start, steps
}

// glideMargin is how much wider than the crop the render first cuts a
// gliding crop out of the picture, on an even pixel, so the rest of the
// way can be taken to the pixel of the short.
const glideMargin = 2

// glideCrop is how the render places a gliding crop whose left edge is x
// on a picture so wide: first a cut glideMargin wider than the crop, on
// the even pixel at or left of x, kept inside the picture, and then, once
// that is scaled to wide pixels, the crop from off pixels into it. scaled
// is how many pixels wide the cut is once scaled, and its own width where
// it is not. cropFilters writes the same steps for ffmpeg.
func glideCrop(x float64, cropW, width, scaled int) (cut, off int) {
	most := width - cropW - glideMargin
	most -= most % 2
	at := min(float64(2*math.Floor(float64(x/2))), float64(most))
	in := float64(float64(float64(x-at)*float64(scaled)) / float64(cropW+glideMargin))
	return int(at), int(math.Floor(float64(in + 0.5)))
}

// cropFilters are the filters that crop a piece for the render and scale
// it to outW by outH, or leave it as cropped where those are the crop's
// own size. A crop that stands still is one cut on an even pixel, the way
// the colours of a picture are shared between two columns, and one that
// only switches is one cut whose left edge is worked out on every frame,
// see holdExpr. One that glides is cut glideMargin wider on an even pixel,
// scaled, and cropped again to the pixel of the short, see glideCrop, in
// deep, the picture's colours with no chroma shared between two columns,
// so the last crop can start on any of them.
func (s SourceInfo) cropFilters(seg Segment, cropW, cropH, cropY, width, outW, outH int, scale, deep string) []string {
	scaled := func(crop string) []string {
		out := []string{crop}
		if outW != cropW || outH != cropH {
			out = append(out, fmt.Sprintf("scale=%d:%d:%s", outW, outH, scale))
		}
		return out
	}
	still := func(x int) []string {
		return scaled(fmt.Sprintf("crop=%d:%d:%d:%d", cropW, cropH, x, cropY))
	}
	if len(seg.Marks) == 0 {
		return still(ClampCropX(seg.CropX, cropW, width))
	}
	r := s.runOf(seg, cropW, width)
	if s.FPSNum <= 0 || s.FPSDen <= 0 {
		x := int(r.at(s.markFrame(seg.Start)))
		return still(ClampCropX(&x, cropW, width))
	}
	first, end := s.pieceFrames(seg.Start, seg.End)
	if !r.glidesIn(first, end) || width-cropW < glideMargin {
		start, steps := r.heldCrops(first, end)
		if len(steps) == 0 {
			return still(ClampCropX(&start, cropW, width))
		}
		return scaled(fmt.Sprintf("crop=%d:%d:'%s':%d", cropW, cropH, holdExpr(start, steps, first), cropY))
	}
	wide := cropW + glideMargin
	if outW != cropW || outH != cropH {
		wide = int(math.Round(float64(wide) * float64(outW) / float64(cropW)))
	}
	most := width - cropW - glideMargin
	most -= most % 2
	x := r.expr(first)
	at := fmt.Sprintf("min(2*floor(%s/2),%d)", x, most)
	out := []string{fmt.Sprintf("crop=%d:%d:'%s':%d", cropW+glideMargin, cropH, at, cropY)}
	if wide != cropW+glideMargin {
		out = append(out, fmt.Sprintf("scale=%d:%d:%s", wide, outH, scale))
	}
	return append(out, "format="+deep,
		fmt.Sprintf("crop=%d:%d:'floor((%s-%s)*%d/%d+0.5)':0:exact=1", outW, outH, x, at, wide, cropW+glideMargin))
}

// CropStep is the left edge of a piece's crop from a frame of the video
// on, until the next step, on the picture. Frame counts the video's frames
// the way the video preview counts the frame it shows, see frameAt in
// lib/flow.ts. A glide is a step on each of its frames, and not on a whole
// pixel.
type CropStep struct {
	Frame int64   `json:"frame"`
	X     float64 `json:"x"`
}

// CropSteps is how the crop of a piece runs over its frames, worked out
// here for the video preview so the window keeps no rule of its own about
// keyframes: its first frame with the left edge it starts with, and each
// frame after it where the crop moves, the same the render crops by. Nil
// for a piece whose crop is its own throughout, or a video whose frame
// rate is not known, where the window shows the piece's own crop. cropW
// and width are the crop and the picture as the piece stands, rotated or
// not.
func (s SourceInfo) CropSteps(v SegmentView, cropW, width int) []CropStep {
	if len(v.Marks) == 0 || s.FPSNum <= 0 || s.FPSDen <= 0 || width <= 0 {
		return nil
	}
	seg := segmentOf(v)
	r := s.runOf(seg, cropW, width)
	first, end := s.pieceFrames(seg.Start, seg.End)
	var out []CropStep
	if !r.glidesIn(first, end) {
		// Only switches: a step where the piece starts and one at each
		// switch, without a look at every frame between.
		start, switches := r.heldCrops(first, end)
		if len(switches) == 0 && start == r.base {
			return nil
		}
		out = append(out, CropStep{Frame: first, X: float64(start)})
		for _, m := range switches {
			out = append(out, CropStep{Frame: m.k, X: float64(m.x)})
		}
		return out
	}
	for k := first; k < end; k++ {
		if x := r.at(k); len(out) == 0 || out[len(out)-1].X != x {
			out = append(out, CropStep{Frame: k, X: x})
		}
	}
	if len(out) == 1 && out[0].X == float64(r.base) {
		return nil
	}
	return out
}

// segmentOf is a piece as the window is sent it, with its keyframes.
func segmentOf(v SegmentView) Segment {
	seg := Segment{Start: v.Start, End: v.End, CropX: v.CropX}
	for _, m := range v.Marks {
		seg.Marks = append(seg.Marks, CropMark{
			At: m.At, X: m.X, Turn: m.Turn, Glide: m.Glide, Opens: m.Opens, Alone: m.Alone, Last: m.Last, FromClip: m.FromClip,
		})
	}
	return seg
}

// GlideSpan is a stretch of the video the crop glides over, from the
// frame the glide starts on to the frame of the keyframe it glides into,
// each as the moment its frame starts.
type GlideSpan struct {
	From float64 `json:"from"`
	To   float64 `json:"to"`
}

// Glides is where the crop of a piece glides, from each circle into the
// circle after it, each glide cut to the piece's own frames, for the clip
// timeline to draw, hidden by a cut or not. Two circles in a row are
// joined even while their crops are the same and nothing moves yet, which
// is how a circle put after a circle starts, so the line comes with the
// key and not only once a crop is dragged: Tim on #750, "if there are like
// three glide frames in sequence, then we have two gradient lines". Nil
// where no circle follows a circle.
func (s SourceInfo) Glides(v SegmentView, cropW, width int) []GlideSpan {
	if len(v.Marks) == 0 || s.FPSNum <= 0 || s.FPSDen <= 0 || width <= 0 {
		return nil
	}
	r := s.runOf(segmentOf(v), cropW, width)
	first, end := s.pieceFrames(v.Start, v.End)
	var out []GlideSpan
	for i, b := range r.marks {
		if i == 0 || !r.marks[i-1].glide || !b.glide {
			continue
		}
		from, to := max(r.marks[i-1].k, first), min(b.k, end)
		if from < to {
			out = append(out, GlideSpan{From: s.keptAt(from), To: s.keptAt(to)})
		}
	}
	return out
}

// CropAt is the left edge of a piece's crop on the frame that holds a
// moment, on the picture, gliding or held.
func (s SourceInfo) CropAt(seg Segment, at float64, cropW, width int) float64 {
	if len(seg.Marks) == 0 {
		return float64(ClampCropX(seg.CropX, cropW, width))
	}
	return s.runOf(seg, cropW, width).at(s.frameHolding(at))
}

// cropMarksOf is the keyframes a clip of a plan being edited has, as the
// plan holds them.
func cropMarksOf(c *object) []CropMark {
	return settled(c, readCropMarks(c.values[keyCropMarks], rotationsOf(c)))
}

// settled is a clip's keyframes in time order with each circle that
// starts a glide keeping the crop it starts from, see opened: the crop of
// the keyframe before it, or the clip's own for its turn. A circle keeps
// no crop of its own that nobody sees, so turning it into a diamond, or
// putting a circle before it, brings back no crop from before, found by
// the review of #778. Every edit reads the keyframes and writes them
// settled.
func settled(c *object, marks []CropMark) []CropMark {
	out := opened(marks)
	for i := range out {
		if !out[i].FromClip {
			continue
		}
		if x, ok := ownCrop(c, out[i].Turn); ok {
			out[i].X = x - x%2
		}
	}
	return out
}

// ownCrop is the crop a clip of a plan being edited has of its own for
// the picture turned so far, before any keyframe: the crop as filmed, or
// the crop of the pieces turned that way, and false where the plan places
// none.
func ownCrop(c *object, turn int) (int, bool) {
	spans := rotationsOf(c)
	key := keyCropX
	if turn != 0 {
		key = keyCropXTurned
	}
	for _, seg := range segmentObjects(c) {
		if rotationOf(seg, spans) != turn {
			continue
		}
		if n, ok := num.AsInt(seg.values[key]); ok && n >= 0 {
			return n, true
		}
	}
	return 0, false
}

// setCropMarks writes a clip's keyframes in time order, or takes the key
// away when there are none.
func setCropMarks(c *object, marks []CropMark) {
	if len(marks) == 0 {
		c.remove(keyCropMarks)
		return
	}
	sort.Slice(marks, func(a, b int) bool { return marks[a].At < marks[b].At })
	marks = settled(c, marks)
	list := make([]any, len(marks))
	for i, m := range marks {
		o := newObject()
		o.set(keyAt, num.RoundTo(m.At, 3))
		o.set(keyX, m.X)
		o.set(keyTurn, m.Turn)
		if m.Glide {
			o.set(keyGlide, true)
		}
		if m.Aside {
			o.set(keyAside, true)
		}
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
// with its crop at x, one the crop glides into where glide is true, and a
// to below nought removes the one at from. A moved keyframe keeps its kind. A
// keyframe lands on the start of the frame that holds its moment, the frame
// the video preview shows there, and has to be inside a piece the clip
// keeps, because a moment that was cut is not in the short.
// Framing is not a judgement of the clip, so nothing is recorded for
// training.
func SetCropMark(fsys afero.Fs, planPath, clipID string, from, to float64, x int, glide bool, frames SourceInfo) error {
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
		pieces := segmentObjects(c)
		marks := cropMarksOf(c)
		if !adding {
			marks = realise(frames, spansOf(pieces), marks, frames.markFrame(from))
		}
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
			mark := CropMark{At: at, X: x - x%2, Turn: RotationAt(rotationsOf(c), at), Glide: glide}
			if moving != nil {
				mark.X, mark.Turn, mark.Glide = moving.X, moving.Turn, moving.Glide
			}
			kept = append(kept, mark)
		}
		setCropMarks(c, kept)
		return nil
	})
}

// SetCropGlide makes the keyframe of a clip on the frame that holds a
// moment, at in seconds of the video, one the crop glides into, or one it
// switches on: G and S on a keyframe of the other kind, the way a
// keyframe is put with either. Framing is not a judgement of the clip, so
// nothing is recorded for training.
func SetCropGlide(fsys afero.Fs, planPath, clipID string, at float64, glide bool, frames SourceInfo) error {
	if math.IsNaN(at) || math.IsInf(at, 0) || at < 0 || at > MaxEpisodeSeconds {
		return renderErr("a keyframe cannot be at %s", num.Fixed(at, 3))
	}
	return editPlan(fsys, planPath, func(_ *object, clips []*object) error {
		c, err := findClip(clips, clipID)
		if err != nil {
			return err
		}
		marks := realise(frames, spansOf(segmentObjects(c)), cropMarksOf(c), frames.frameHolding(at))
		for i := range marks {
			if frames.markFrame(marks[i].At) == frames.frameHolding(at) {
				marks[i].Glide = glide
				setCropMarks(c, marks)
				return nil
			}
		}
		return renderErr("the clip has no keyframe at %s", HMS(at))
	})
}

// marksText is a clip's keyframes for the log, each its moment and left
// edge, and a tilde for a circle.
func marksText(marks []CropMark) string {
	parts := make([]string, len(marks))
	for i, m := range marks {
		parts[i] = num.Fixed(m.At, 3) + "@" + strconv.Itoa(m.X)
		if m.Glide {
			parts[i] = "~" + parts[i]
		}
	}
	return strings.Join(parts, " ")
}
