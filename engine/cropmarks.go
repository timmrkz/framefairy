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
		return roundTo(at, 3)
	}
	return s.keptAt(s.frameHolding(at))
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
// camera operator pans.
//
// The edge is not rounded to a pixel of the picture. A slow glide moves
// less than a pixel a frame, and rounded it stood still for frames and
// then jumped, which was the stutter Tim saw on #192. The render places
// it to the pixel of the short instead, see glideCrop.
//
// The render works the same sum out in ffmpeg, see glideExpr, and the
// video preview in lib/glide.ts, step by step in the same order, each a
// double rounded on its own, so the three agree. Each step is converted
// on its own here, which keeps Go from fusing a multiply and an add into
// one instruction, as it may on arm64.
func glideX(marks []frameMark, k int64) float64 {
	if k <= marks[0].k {
		return float64(marks[0].x)
	}
	for i := 1; i < len(marks); i++ {
		b := marks[i]
		if k >= b.k {
			continue
		}
		a := marks[i-1]
		u := float64(k-a.k) / float64(b.k-a.k)
		ease := float64(float64(u*u) * float64(3-float64(2*u)))
		return float64(a.x) + float64(float64(b.x-a.x)*ease)
	}
	return float64(marks[len(marks)-1].x)
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
// it is not. glideExpr writes the same steps for ffmpeg.
func glideCrop(x float64, cropW, width, scaled int) (cut, off int) {
	most := width - cropW - glideMargin
	most -= most % 2
	at := min(float64(2*math.Floor(float64(x/2))), float64(most))
	in := float64(float64(float64(x-at)*float64(scaled)) / float64(cropW+glideMargin))
	return int(at), int(math.Floor(float64(in + 0.5)))
}

// glideExpr is glideX as an ffmpeg expression for the crop filter of a
// piece whose first frame is frame first of the video, see glideCrop for
// the two steps the render takes with it. The crop filter
// counts the frames it is given as n, from nought, so frame k of the video
// is n+first. Every step is written in the order glideX takes it. The
// stretches between marks are found by halving, so the expression nests
// as deep as the number of marks has binary digits: ffmpeg refuses one
// nested deeper than a hundred, and one if per mark made a clip of ninety
// marks unrenderable.
func glideExpr(marks []frameMark, first int64) string {
	k := "(n+" + strconv.FormatInt(first, 10) + ")"
	if first < 0 {
		k = "(n-" + strconv.FormatInt(-first, 10) + ")"
	}
	last := marks[len(marks)-1]
	if len(marks) == 1 {
		return strconv.Itoa(last.x)
	}
	// The glide over the stretches lo to hi, stretch i running from mark
	// i-1 to mark i.
	var between func(lo, hi int) string
	between = func(lo, hi int) string {
		if lo == hi {
			a, b := marks[lo-1], marks[lo]
			u := fmt.Sprintf("((%s-%d)/%d)", k, a.k, b.k-a.k)
			return fmt.Sprintf("(%d+%d*(%s*%s*(3-2*%s)))", a.x, b.x-a.x, u, u, u)
		}
		mid := (lo + hi + 1) / 2
		return fmt.Sprintf("if(lt(%s,%d),%s,%s)", k, marks[mid-1].k, between(lo, mid-1), between(mid, hi))
	}
	return fmt.Sprintf("if(lte(%s,%d),%d,if(gte(%s,%d),%d,%s))", k, marks[0].k, marks[0].x, k, last.k, last.x, between(1, len(marks)-1))
}

// cropFilters are the filters that crop a piece for the render and scale
// it to outW by outH, or leave it as cropped where those are the crop's
// own size. A crop that stands still is one cut on an even pixel. One
// that glides is cut glideMargin wider on an even pixel, scaled, and
// cropped again to the pixel of the short, see glideCrop, in deep, the
// picture's colours with no chroma shared between two columns, so the
// last crop can start on any of them.
func (s SourceInfo) cropFilters(seg Segment, cropW, cropH, cropY, width, outW, outH int, scale, deep string) []string {
	still := func(x int) []string {
		out := []string{fmt.Sprintf("crop=%d:%d:%d:%d", cropW, cropH, x, cropY)}
		if outW != cropW || outH != cropH {
			out = append(out, fmt.Sprintf("scale=%d:%d:%s", outW, outH, scale))
		}
		return out
	}
	if len(seg.Marks) == 0 {
		return still(ClampCropX(seg.CropX, cropW, width))
	}
	marks := s.onFrames(seg.Marks, cropW, width)
	first := s.markFrame(seg.Start)
	if s.FPSNum > 0 && s.FPSDen > 0 {
		first, _ = s.pieceFrames(seg.Start, seg.End)
	}
	moves := false
	for _, m := range marks {
		moves = moves || m.x != marks[0].x
	}
	if !moves || width-cropW < glideMargin || s.FPSNum <= 0 || s.FPSDen <= 0 {
		return still(ClampCropX(intPtr(int(glideX(marks, first))), cropW, width))
	}
	scaled := cropW + glideMargin
	if outW != cropW || outH != cropH {
		scaled = int(math.Round(float64(scaled) * float64(outW) / float64(cropW)))
	}
	most := width - cropW - glideMargin
	most -= most % 2
	x := glideExpr(marks, first)
	at := fmt.Sprintf("min(2*floor(%s/2),%d)", x, most)
	out := []string{fmt.Sprintf("crop=%d:%d:'%s':%d", cropW+glideMargin, cropH, at, cropY)}
	if scaled != cropW+glideMargin {
		out = append(out, fmt.Sprintf("scale=%d:%d:%s", scaled, outH, scale))
	}
	return append(out, "format="+deep,
		fmt.Sprintf("crop=%d:%d:'floor((%s-%s)*%d/%d+0.5)':0:exact=1", outW, outH, x, at, scaled, cropW+glideMargin))
}

// CropAt is the left edge of a piece's crop on the frame that holds a
// moment, on the picture: still, or gliding over its marks, where the
// render places it to the pixel of the short.
func (s SourceInfo) CropAt(seg Segment, at float64, cropW, width int) float64 {
	if len(seg.Marks) == 0 {
		return float64(ClampCropX(seg.CropX, cropW, width))
	}
	return glideX(s.onFrames(seg.Marks, cropW, width), s.frameHolding(at))
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

// marksOfAngle is which of a clip's crop marks belong to one camera
// angle, by their index. A mark lies in a piece of it, or, where a trim or
// a cut hides it, beside the nearest piece of it, so a crop placed or put
// back for the angle takes it along too, and a trim taken back brings
// back a mark that agrees with the rest.
func marksOfAngle(marks []CropMark, pieces []*object, spans []Rotation, angle string) map[int]bool {
	out := map[int]bool{}
	for i, m := range marks {
		h := pieceHolding(pieces, m.At)
		if h < 0 {
			h = pieceNearest(pieces, m.At)
		}
		if h >= 0 && angleOf(pieces[h], spans) == angle {
			out[i] = true
		}
	}
	return out
}

// pieceNearest is the piece of a plan being edited whose start or end is
// nearest a moment, the earlier of two as near, or minus one for a clip
// with no pieces.
func pieceNearest(pieces []*object, at float64) int {
	best, gap := -1, math.Inf(1)
	for i, seg := range pieces {
		d := max(number(seg.values[keyStart])-at, at-number(seg.values[keyEnd]), 0)
		if d < gap {
			best, gap = i, d
		}
	}
	return best
}

// SetCropMark adds, moves or removes a crop mark of a clip, the way
// SetThumbnail does a thumbnail: from is the mark as it is and to where it
// goes, both in seconds of the video. A from below nought adds one at to
// with its crop at x, and a to below nought removes the one at from. A
// mark lands on the start of the frame that holds its moment, the frame
// the video preview shows there, and has to be inside a piece the clip
// keeps, because a moment that was cut is not in the short.
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
			// from is the mark's own moment, so it is found on its own
			// frame, wherever in the frame a hand edit or an older plan
			// put it. Two marks a hand edit put on one frame go together,
			// the later one moving, because the glide only ever used it.
			for i := range marks {
				if frames.markFrame(marks[i].At) == frames.markFrame(from) {
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
			at := frames.frameStartHolding(to)
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
// shows on the frame a moment means: gliding over the marks of its angle
// in its part, each kept inside a picture so wide for a crop so wide the
// way the render keeps it, or the crop of its angle, or nil where it
// stands in the middle. A width no wider than the crop is a picture not
// known, and the marks are taken as they are.
func cropShown(c *object, pieces []*object, target int, at float64, frames SourceInfo, cropW, width int) *int {
	spans := rotationsOf(c)
	segments := make([]Segment, len(pieces))
	angles := make([]string, len(pieces))
	for i, seg := range pieces {
		segments[i] = Segment{Start: number(seg.values[keyStart]), End: number(seg.values[keyEnd])}
		angles[i] = angleOf(seg, spans)
	}
	markedPieces(segments, angles, cropMarksOf(c))
	if marks := segments[target].Marks; len(marks) > 0 {
		if width <= cropW {
			width = math.MaxInt32
		}
		left := int(math.Round(glideX(frames.onFrames(marks, cropW, width), frames.frameHolding(at))))
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
