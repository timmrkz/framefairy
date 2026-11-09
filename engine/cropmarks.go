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
// where the camera switches is all a switch takes, or it glides: the crop
// glides into it from the keyframe before, over the whole time between
// them, and reaches it on its frame. Tim chose the two kinds, a diamond
// put with K and a circle put with G, from mockups on #750. See
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
// keyframe the crop glides into from the keyframe before, rather than
// switching on its frame.
type CropMark struct {
	At    float64
	X     int
	Turn  int
	Glide bool
}

// MaxCropMarks is how many keyframes one clip may have.
const MaxCropMarks = 100

// CropMarkView is a keyframe as the window reads it.
type CropMarkView struct {
	At    float64 `json:"at"`
	X     int     `json:"x"`
	Turn  int     `json:"turn"`
	Glide bool    `json:"glide"`
}

func cropMarkViews(marks []CropMark) []CropMarkView {
	out := make([]CropMarkView, len(marks))
	for i, m := range marks {
		out[i] = CropMarkView{At: m.At, X: m.X, Turn: m.Turn, Glide: m.Glide}
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
		out = append(out, CropMark{At: num.RoundTo(at, 3), X: x - x%2, Turn: turn, Glide: glide})
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
//
// Each piece is also told where the crop of its turn begins, read the same
// way, which is where a glide into the first keyframe starts, see
// Segment.MarksFrom.
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
	// Where on the clip's clock the first piece of each turn starts.
	begins := map[int]float64{}
	for i := len(segments) - 1; i >= 0; i-- {
		begins[turns[i]] = into[i]
	}
	for i := range segments {
		segments[i].MarksFrom = segments[i].Start + begins[turns[i]] - into[i]
	}
	for _, m := range marks {
		holder := -1
		var onClip float64
		for i, s := range segments {
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
// the picture, and whether the crop glides into it.
type frameMark struct {
	k     int64
	x     int
	glide bool
}

// onFrames is a piece's keyframes on their frames, each left edge kept
// inside a picture so wide for a crop so wide. Two keyframes on one frame
// keep the later.
func (s SourceInfo) onFrames(marks []CropMark, cropW, width int) []frameMark {
	var out []frameMark
	for _, m := range marks {
		x := m.X
		f := frameMark{k: s.markFrame(m.At), x: ClampCropX(&x, cropW, width), glide: m.Glide}
		if n := len(out); n > 0 && out[n-1].k == f.k {
			out[n-1] = f
			continue
		}
		out = append(out, f)
	}
	return out
}

// cropRun is how the crop of a piece runs over the frames of the video:
// its keyframes on their frames, the piece's own crop, base, before the
// first, and the frame from, where the crop of its turn begins, which is
// where a glide into the first keyframe starts.
type cropRun struct {
	marks []frameMark
	base  int
	from  int64
}

// runOf is the crop of a piece as it runs over the frames, its left edges
// kept inside a picture so wide for a crop so wide.
func (s SourceInfo) runOf(seg Segment, cropW, width int) cropRun {
	return cropRun{
		marks: s.onFrames(seg.Marks, cropW, width),
		base:  ClampCropX(seg.CropX, cropW, width),
		from:  s.markFrame(seg.MarksFrom),
	}
}

// before is where the crop stands as the stretch up to keyframe i starts:
// the keyframe before, or the piece's own crop where the crop of its turn
// begins.
func (r cropRun) before(i int) frameMark {
	if i > 0 {
		return r.marks[i-1]
	}
	return frameMark{k: r.from, x: r.base}
}

// gliding is whether the crop glides into keyframe i, from the keyframe
// before it to a crop of its own over at least one frame.
func (r cropRun) gliding(i int) bool {
	a, b := r.before(i), r.marks[i]
	return b.glide && b.k > a.k && b.x != a.x
}

// at is the left edge of the crop on frame k. From a keyframe's frame on
// it is that keyframe's, until the next, and before the first it is the
// piece's own. Into a keyframe that switches, the crop jumps on its frame.
// Into one that glides, it moves the whole way from the keyframe before,
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
		glide := fmt.Sprintf("(%d+%d*(%s*%s*(3-2*%s)))", a.x, b.x-a.x, u, u, u)
		if i == 0 {
			return fmt.Sprintf("if(lt(%s,%d),%d,%s)", k, a.k, a.x, glide)
		}
		return glide
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
	seg := Segment{Start: v.Start, End: v.End, CropX: v.CropX, MarksFrom: v.MarksFrom}
	for _, m := range v.Marks {
		seg.Marks = append(seg.Marks, CropMark{At: m.At, X: m.X, Turn: m.Turn, Glide: m.Glide})
	}
	r := s.runOf(seg, cropW, width)
	first, end := s.pieceFrames(seg.Start, seg.End)
	var out []CropStep
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
		if m.Glide {
			o.set(keyGlide, true)
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
// switches on: G and K on a keyframe of the other kind, the way a
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
		marks := cropMarksOf(c)
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
// edge, and a tilde for one the crop glides into.
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
