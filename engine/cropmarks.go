package engine

import (
	"fmt"
	"math"
	"sort"
	"strconv"
	"strings"
)

// A crop that follows, plan row 3.26. A person puts crop marks on frames
// of a clip, the way thumbnails are put, each with the crop where it
// stands on that frame. No mark, or one, is a crop that stands still. Two
// or more make it move: it glides from each mark to the next over the
// whole time between them, easing out of the first and into the next, so
// the tempo is where the marks are, and two marks a frame apart are an
// instant move. See docs/ROTATE-AND-FOLLOW.md.
//
// A mark is a moment of the video, like a thumbnail, and belongs to the
// camera angle of the piece it lies in, the shot as the analysis framed it
// and how far it is rotated, see angleOf. The marks that steer a piece are
// those of its angle in its part, the pieces that play straight on between
// two cuts, so no glide crosses a cut and a shot cut away to and back
// keeps gliding as if it had been seen. A mark a trim or a cut left outside
// the clip stays in the plan and comes back with its piece.

// CropMark is a crop placed by hand on one frame: the moment the frame
// starts, in seconds of the video, and the left edge of the crop on the
// picture as it stands there, rotated if the piece is.
type CropMark struct {
	At float64
	X  int
}

// MaxCropMarks is how many crop marks one clip may have.
const MaxCropMarks = 100

// CropMarkView is a crop mark as the window reads it.
type CropMarkView struct {
	At float64 `json:"at"`
	X  int     `json:"x"`
}

func cropMarkViews(marks []CropMark) []CropMarkView {
	out := make([]CropMarkView, len(marks))
	for i, m := range marks {
		out[i] = CropMarkView{At: m.At, X: m.X}
	}
	return out
}

// readCropMarks takes a clip's crop marks out of a plan, which is untrusted
// like the rest of it, the way readThumbnails takes its thumbnails: a mark
// is kept only if it has a moment that is a number and a left edge that is
// a whole number in the picture's range, each moment once to the
// millisecond, in time order and no more than MaxCropMarks of them.
// Anything else is left out, and the crop is where the rest put it.
func readCropMarks(raw any) []CropMark {
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
		at, ok := toFloat(entry[keyAt])
		if !ok || !isFinite(at) || at < 0 || at > MaxEpisodeSeconds {
			continue
		}
		left, ok := toFloat(entry[keyX])
		if !ok || left != math.Trunc(left) || left < 0 || left >= 1_000_000 {
			continue
		}
		x := int(left)
		key := int64(math.Round(at * 1000))
		if seen[key] {
			continue
		}
		seen[key] = true
		out = append(out, CropMark{At: roundTo(at, 3), X: x - x%2})
	}
	sort.Slice(out, func(a, b int) bool { return out[a].At < out[b].At })
	if len(out) > MaxCropMarks {
		out = out[:MaxCropMarks]
	}
	return out
}

// markedPieces gives each piece of a clip the crop marks that steer it:
// the marks of its camera angle in its part. angles is each piece's angle,
// as angleOf gives it. A mark that lies in no piece steers nothing.
func markedPieces(segments []Segment, angles []string, marks []CropMark) {
	if len(marks) == 0 {
		return
	}
	parts := make([]int, len(segments))
	for i := 1; i < len(segments); i++ {
		parts[i] = parts[i-1]
		if segments[i].Start-segments[i-1].End > PiecesMeet {
			parts[i]++
		}
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
		for i := range segments {
			if parts[i] == parts[holder] && angles[i] == angles[holder] {
				segments[i].Marks = append(segments[i].Marks, m)
			}
		}
	}
}

// markFrame is the frame of the video a moment means, by the frame rate
// and the grid the frames are on, see nearestFrame, and the millisecond
// where the rate is not known. The glide is worked out on these.
func (s SourceInfo) markFrame(at float64) int64 {
	if s.FPSNum <= 0 || s.FPSDen <= 0 {
		return int64(math.Round(at * 1000))
	}
	return s.nearestFrame(at)
}

// frameMark is a crop mark on its frame, with its left edge kept inside
// the picture.
type frameMark struct {
	k int64
	x int
}

// onFrames is a piece's crop marks on their frames, each left edge kept
// inside a picture so wide for a crop so wide. Two marks on one frame
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

// glideX is the left edge of the crop on frame k, gliding over marks on
// frames in order. Before the first mark it stands where the first is,
// after the last where the last is. Between two it moves the whole way,
// by smoothstep, u²(3-2u), which starts and settles gently the way a
// camera operator pans, and lands on an even pixel.
//
// The render works the same sum out in ffmpeg, see glideExpr, and the
// video preview in lib/glide.ts, step by step in the same order, each a
// double rounded on its own, so the three agree to the pixel. Each step
// is converted on its own here, which keeps Go from fusing a multiply and
// an add into one instruction, as it may on arm64.
func glideX(marks []frameMark, k int64) int {
	if k <= marks[0].k {
		return marks[0].x
	}
	for i := 1; i < len(marks); i++ {
		b := marks[i]
		if k >= b.k {
			continue
		}
		a := marks[i-1]
		u := float64(k-a.k) / float64(b.k-a.k)
		ease := float64(float64(u*u) * float64(3-float64(2*u)))
		x := float64(a.x) + float64(float64(b.x-a.x)*ease)
		return 2 * int(math.Floor(float64(x/2)+0.5))
	}
	return marks[len(marks)-1].x
}

// glideExpr is glideX as an ffmpeg expression for the crop filter of a
// piece whose first frame is frame first of the video. The crop filter
// counts the frames it is given as n, from nought, so frame k of the video
// is n+first. Every step is written in the order glideX takes it.
func glideExpr(marks []frameMark, first int64) string {
	k := "(n+" + strconv.FormatInt(first, 10) + ")"
	if first < 0 {
		k = "(n-" + strconv.FormatInt(-first, 10) + ")"
	}
	expr := strconv.Itoa(marks[len(marks)-1].x)
	for i := len(marks) - 1; i >= 1; i-- {
		a, b := marks[i-1], marks[i]
		u := fmt.Sprintf("((%s-%d)/%d)", k, a.k, b.k-a.k)
		step := fmt.Sprintf("2*floor((%d+%d*(%s*%s*(3-2*%s)))/2+0.5)", a.x, b.x-a.x, u, u, u)
		expr = fmt.Sprintf("if(lt(%s,%d),%s,%s)", k, b.k, step, expr)
	}
	return fmt.Sprintf("if(lte(%s,%d),%d,%s)", k, marks[0].k, marks[0].x, expr)
}

// cropXOf is the left edge of a piece's crop for the render: a number for
// a crop that stands still, and an ffmpeg expression in the frame for one
// that glides, quoted for the filter graph.
func (s SourceInfo) cropXOf(seg Segment, cropW, width int) string {
	if len(seg.Marks) == 0 {
		return strconv.Itoa(ClampCropX(seg.CropX, cropW, width))
	}
	marks := s.onFrames(seg.Marks, cropW, width)
	first := s.markFrame(seg.Start)
	if s.FPSNum > 0 && s.FPSDen > 0 {
		first, _ = s.pieceFrames(seg.Start, seg.End)
	}
	if len(marks) == 1 || s.FPSNum <= 0 || s.FPSDen <= 0 {
		return strconv.Itoa(glideX(marks, first))
	}
	return "'" + glideExpr(marks, first) + "'"
}

// CropAt is the left edge of a piece's crop on the frame a moment means,
// as the render places it: still, or gliding over its marks.
func (s SourceInfo) CropAt(seg Segment, at float64, cropW, width int) int {
	if len(seg.Marks) == 0 {
		return ClampCropX(seg.CropX, cropW, width)
	}
	return glideX(s.onFrames(seg.Marks, cropW, width), s.markFrame(at))
}

// cropMarksOf is the crop marks a clip of a plan being edited has, as the
// plan holds them.
func cropMarksOf(c *object) []CropMark {
	return readCropMarks(c.values[keyCropMarks])
}

// setCropMarks writes a clip's crop marks in time order, or takes the key
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
		o.set(keyAt, roundTo(m.At, 3))
		o.set(keyX, m.X)
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

// marksOfAngle is which of a clip's crop marks lie in a piece of one camera
// angle, by their index.
func marksOfAngle(marks []CropMark, pieces []*object, spans []Rotation, angle string) map[int]bool {
	out := map[int]bool{}
	for i, m := range marks {
		if h := pieceHolding(pieces, m.At); h >= 0 && angleOf(pieces[h], spans) == angle {
			out[i] = true
		}
	}
	return out
}

// SetCropMark adds, moves or removes a crop mark of a clip, the way
// SetThumbnail does a thumbnail: from is the mark as it is and to where it
// goes, both in seconds of the video. A from below nought adds one at to
// with its crop at x, and a to below nought removes the one at from. A
// mark lands on the start of the frame it means, and has to be inside a
// piece the clip keeps, because a moment that was cut is not in the short.
// Framing is not a judgement of the clip, so nothing is recorded for
// training.
func SetCropMark(planPath, clipID string, from, to float64, x int, frames SourceInfo) error {
	adding, removing := from < 0, to < 0
	if adding && removing {
		return renderErr("a crop mark has to be somewhere")
	}
	for _, at := range []float64{from, to} {
		if math.IsNaN(at) || math.IsInf(at, 0) || at > MaxEpisodeSeconds {
			return renderErr("a crop mark cannot be at %s", fixed(at, 3))
		}
	}
	if adding && (x < 0 || x >= 1_000_000) {
		return renderErr("the crop cannot start left of the picture")
	}
	return editPlan(planPath, func(_ *object, clips []*object) error {
		c, err := findClip(clips, clipID)
		if err != nil {
			return err
		}
		marks := cropMarksOf(c)
		pieces := segmentObjects(c)
		var kept []CropMark
		var moving *CropMark
		if !adding {
			for i := range marks {
				if moving == nil && frames.markFrame(marks[i].At) == frames.markFrame(from) {
					m := marks[i]
					moving = &m
					continue
				}
				kept = append(kept, marks[i])
			}
			if moving == nil {
				return renderErr("the clip has no crop mark at %s", HMS(from))
			}
		} else {
			kept = marks
		}
		if !removing {
			at := frames.frameStartNear(to)
			if pieceHolding(pieces, at) < 0 {
				return renderErr("%s is not in the clip", HMS(to))
			}
			for _, m := range kept {
				if frames.markFrame(m.At) == frames.markFrame(at) {
					return renderErr("the clip already has a crop mark at %s", HMS(to))
				}
			}
			if len(kept) >= MaxCropMarks {
				return renderErr("a clip can have %d crop marks", MaxCropMarks)
			}
			left := x - x%2
			if moving != nil {
				left = moving.X
			}
			kept = append(kept, CropMark{At: at, X: left})
		}
		setCropMarks(c, kept)
		return nil
	})
}

// cropShown is the left edge of the crop a piece of a plan being edited
// shows on the frame a moment means, before it is kept inside the picture:
// gliding over the marks of its angle in its part, or the crop of its
// angle, or nil where it stands in the middle.
func cropShown(c *object, pieces []*object, target int, at float64, frames SourceInfo) *int {
	spans := rotationsOf(c)
	segments := make([]Segment, len(pieces))
	angles := make([]string, len(pieces))
	for i, seg := range pieces {
		segments[i] = Segment{Start: number(seg.values[keyStart]), End: number(seg.values[keyEnd])}
		angles[i] = angleOf(seg, spans)
	}
	markedPieces(segments, angles, cropMarksOf(c))
	if marks := segments[target].Marks; len(marks) > 0 {
		var on []frameMark
		for _, m := range marks {
			on = append(on, frameMark{k: frames.markFrame(m.At), x: m.X})
		}
		left := glideX(on, frames.markFrame(at))
		return &left
	}
	key := keyCropX
	if rotationOf(pieces[target], spans) != 0 {
		key = keyCropXTurned
	}
	if n, ok := toInt(pieces[target].values[key]); ok {
		return &n
	}
	return nil
}

// angleKey is the camera angle of a piece as LoadClips reads it, the same
// as angleOf gives for a plan being edited.
func angleKey(seg map[string]any, rotate int) string {
	auto, ok := seg[keyCropXAuto]
	if !ok {
		auto = seg[keyCropX]
	}
	return fmt.Sprint(auto) + "/" + strconv.Itoa(rotate)
}

// marksText is a clip's crop marks for the log, each its moment and left
// edge.
func marksText(marks []CropMark) string {
	parts := make([]string, len(marks))
	for i, m := range marks {
		parts[i] = fixed(m.At, 3) + "@" + strconv.Itoa(m.X)
	}
	return strings.Join(parts, " ")
}
