package engine

import (
	"context"
	"math"

	"framefairy/internal/framewire"
)

// Where the words of a caption stand, as libass draws them. The pill of
// the word being spoken is placed from it, and so is the frame the caption
// box puts round a word under the hand or the keyboard, so the frame is
// where the pill is and round the word libass drew, not round a word the
// browser laid out on its own. Tim saw the frame stand off its word on
// #201, once the words in the video preview were libass's. See
// docs/CAPTIONS.md.

// wordsLaid is where libass puts the words of one caption: each word's
// ink, and each line's reach, see pillLines, both from the caption's
// anchor, the middle of its bottom edge.
type wordsLaid struct {
	words  []string
	lines  [][]int
	lineOf []int
	boxes  []*inkBox
	reach  []*inkBox
}

// layWords measures where libass puts every word of the captions, drawn
// in s at size in a short width wide. It is false when a caption has no
// word timings, or a word or a line could not be measured.
func (e *Engine) layWords(ctx context.Context, captions []LaidCaption, s Style, width, size int) ([]wordsLaid, bool) {
	layouts := make([]wordsLaid, len(captions))
	var blocks []string
	for i, c := range captions {
		if len(c.Words) == 0 {
			e.Log.Detail("a caption has no word timings, so the highlight is off for this clip")
			return nil, false
		}
		words := make([]string, len(c.Words))
		for k, w := range c.Words {
			words[k] = EscapeASSText(w.Text)
		}
		lines := lineIndices(c.Lines)
		lineOf := make([]int, len(words))
		for r, line := range lines {
			for _, k := range line {
				lineOf[k] = r
			}
		}
		layouts[i] = wordsLaid{words: words, lines: lines, lineOf: lineOf}
		for k := range words {
			k := k
			// Measured opaque, whatever the text colour: a word is found by
			// its ink, and a see-through one would leave little to find.
			blocks = append(blocks, taggedText(words, lines, func(j int) bool { return j == k }, shownTag("")))
		}
		blocks = append(blocks, pillLines(len(lines))...)
	}
	boxes, ok := e.measureMany(ctx, blocks, s, width, size)
	if !ok {
		e.Log.Detail("could not measure the caption words, the highlight is off for this clip")
		return nil, false
	}
	at := 0
	for i := range layouts {
		n, rows := len(layouts[i].words), len(layouts[i].lines)
		layouts[i].boxes = boxes[at : at+n]
		layouts[i].reach = boxes[at+n : at+n+rows]
		at += n + rows
		for _, b := range boxes[at-n-rows : at] {
			if b == nil {
				e.Log.Detail("a caption word could not be measured, the highlight is off for this clip")
				return nil, false
			}
		}
	}
	return layouts, true
}

// pillRadius is the radius of the corners of a pill h high, in a style
// whose corners are radius, both in pixels of the short: a third of its
// height, or the box's corners and two pixels, whichever is less. The
// frame round a word in the caption box has the same corners, see
// WordFrame.
func pillRadius(h, radius float64) float64 { return math.Min(h*0.3, radius+2) }

// wordBegins is the centisecond each word of a caption that shows from
// start to end becomes the word being spoken: when it starts, inside the
// caption, and never before the word before it. A word stays the one
// spoken until the next one starts, so a short word never flickers.
func wordBegins(c LaidCaption, start, end int) []int {
	begins := make([]int, len(c.Words))
	for k, w := range c.Words {
		begins[k] = min(max(centiseconds(w.Start), start), end)
	}
	for k := 1; k < len(begins); k++ {
		begins[k] = max(begins[k], begins[k-1])
	}
	return begins
}

// CaptionShowing is what libass shows of the captions at a moment of the
// short, in milliseconds, by the times their caption file gives them:
// the caption, -1 for none, and the word of it being spoken, counted
// across its lines, -1 for none yet. The caption box over the video
// preview shows what the layer under it shows by it, see
// /frames/captions.
type CaptionShowing struct {
	Caption int `json:"caption"`
	Word    int `json:"word"`
}

// ShowingAt is what libass shows of laid at the moment ms, see
// CaptionShowing.
func ShowingAt(laid []LaidCaption, ms int64) CaptionShowing {
	for i, c := range laid {
		start, end := centiseconds(c.Start), centiseconds(c.End)
		if end <= start || ms < int64(start)*10 || ms >= int64(end)*10 {
			continue
		}
		word := -1
		for k, b := range wordBegins(c, start, end) {
			if ms >= int64(b)*10 {
				word = k
			}
		}
		return CaptionShowing{Caption: i, Word: word}
	}
	return CaptionShowing{Caption: -1, Word: -1}
}

// pill is the box the pill of word k takes at rest, from the caption's
// anchor, at size: the word's ink across with pillPadX at each side, and
// its line down, see lineBand.
func (l wordsLaid) pill(k, size int) (left, top, right, bottom float64) {
	padX := math.Round(float64(size) * pillPadX)
	b := l.boxes[k]
	top, bottom = l.lineBand(l.lineOf[k], size)
	return float64(b.left) - padX, top, float64(b.right) + padX, bottom
}

// lineBand is how far down the pills of line r reach at rest, the same for
// every word of the line, so the pill keeps its height as it goes from
// word to word: the line's reach with pillPadY above and below, but never
// nearer than lineClear to a letter of the line above or below, over the
// whole width of the line. Where the letters of the two lines come so near
// that this leaves the line's own letters less than lineClear of room, the
// pill keeps that room and touches the other line instead.
//
// libass stands the lines of a caption as close as the face is tall, so a
// pill of the whole line and its room reached into the tail of a "g" on
// the line above. Tim saw the "g" of "wichtig" stand in the pill of
// "Leute" on #201, and the lines are not to move apart for it. A pill
// stopped short word by word was a pill that jumped as it walked along a
// line.
func (l wordsLaid) lineBand(r, size int) (top, bottom float64) {
	padY := math.Round(float64(size) * pillPadY)
	clear := math.Round(float64(size) * lineClear)
	line := l.reach[r]
	top, bottom = float64(line.top)-padY, float64(line.bottom)+padY
	inkTop, inkBottom := l.lineInk(r)
	if above, ok := l.nearestInk(r, r-1, size); ok {
		top = math.Max(top, math.Min(above+1+clear, inkTop-clear))
	}
	if below, ok := l.nearestInk(r, r+1, size); ok {
		bottom = math.Min(bottom, math.Max(below-clear, inkBottom+1+clear))
	}
	return top, bottom
}

// bounce is how far the pill of word k reaches above and below its rest
// at the top of its bounce, at size: bounceGrow each way where there is
// room, and what one side has no room for goes to the other, so every
// word bounces as far. It never comes nearer than lineClear to a letter
// of another line, and every word of a line bounces alike.
func (l wordsLaid) bounce(k, size int) (up, down float64) {
	grow := bounceGrow * float64(size)
	clear := math.Round(float64(size) * lineClear)
	r := l.lineOf[k]
	top, bottom := l.lineBand(r, size)
	roomUp, roomDown := math.Inf(1), math.Inf(1)
	if above, ok := l.nearestInk(r, r-1, size); ok {
		roomUp = math.Max(0, top-(above+1+clear))
	}
	if below, ok := l.nearestInk(r, r+1, size); ok {
		roomDown = math.Max(0, below-clear-bottom)
	}
	up = math.Min(grow, roomUp)
	down = math.Min(2*grow-up, roomDown)
	up = math.Min(2*grow-down, roomUp)
	return up, down
}

// lineInk is the highest and the lowest row of the ink of the words of
// line r.
func (l wordsLaid) lineInk(r int) (top, bottom float64) {
	top, bottom = math.Inf(1), math.Inf(-1)
	for _, k := range l.lines[r] {
		top, bottom = math.Min(top, float64(l.boxes[k].top)), math.Max(bottom, float64(l.boxes[k].bottom))
	}
	return top, bottom
}

// nearestInk is the row of the ink of line other nearest line r, over the
// width the pills of line r take: the lowest row of a line above, the
// highest of a line below. False where there is no such line or none of
// its words stands over or under line r.
func (l wordsLaid) nearestInk(r, other, size int) (float64, bool) {
	if other < 0 || other >= len(l.lines) {
		return 0, false
	}
	padX := math.Round(float64(size) * pillPadX)
	left, right := math.Inf(1), math.Inf(-1)
	for _, k := range l.lines[r] {
		left = math.Min(left, float64(l.boxes[k].left)-padX)
		right = math.Max(right, float64(l.boxes[k].right)+padX)
	}
	nearest, ok := 0.0, false
	for _, j := range l.lines[other] {
		b := l.boxes[j]
		if float64(b.right) < left || float64(b.left) > right {
			continue
		}
		edge := float64(b.top)
		if other < r {
			edge = float64(b.bottom)
		}
		if !ok || (other < r && edge > nearest) || (other > r && edge < nearest) {
			nearest, ok = edge, true
		}
	}
	return nearest, ok
}

// WordFrame is where a word of a caption stands in the short: the box its
// pill takes at rest, as shares of the short's width and height from its
// top left corner.
type WordFrame struct {
	X float64 `json:"x"`
	Y float64 `json:"y"`
	W float64 `json:"w"`
	H float64 `json:"h"`
	// R is the radius of its corners, as a share of the short's height.
	R float64 `json:"r"`
	// From and To are when the picture lights the word, on the episode's
	// clock: from the start of the first frame of the clip libass draws it
	// lit on to the end of the last, see WordsLit. Both -1 for a word lit
	// on no frame, or where the frames are not known.
	From float64 `json:"from"`
	To   float64 `json:"to"`
}

// WordFrames is where the words of the captions stand in a short width by
// height, each caption's words in their order, as CaptionTrack lays them
// out in the style overrides make. Nothing when they cannot be measured.
func (e *Engine) WordFrames(ctx context.Context, laid []LaidCaption, width, height int,
	overrides map[string]any, light framewire.Light) [][]WordFrame {
	s := ResolveStyle(overrides).inLight(light)
	scale := float64(height) / 1920.0
	size := max(12, pyround(s.Size*scale))
	layouts, ok := e.layWords(ctx, laid, s, width, size)
	if !ok {
		return nil
	}
	ax := float64(width) / 2
	ay := float64(height - pyround(s.MarginV*scale))
	radius := float64(pyround(s.Radius * scale))
	out := make([][]WordFrame, len(layouts))
	for i, l := range layouts {
		out[i] = make([]WordFrame, len(l.words))
		for k := range l.words {
			left, top, right, bottom := l.pill(k, size)
			out[i][k] = WordFrame{X: (ax + left) / float64(width), Y: (ay + top) / float64(height),
				W: (right - left) / float64(width), H: (bottom - top) / float64(height),
				R: pillRadius(bottom-top, radius) / float64(height)}
		}
	}
	return out
}

// WordsLit is when the picture lights each word of the captions of d, on
// the episode's clock, each caption's words in their order: from the start
// of the first frame of the clip on which libass draws the word lit to the
// end of the last, read frame by frame at the moment CaptionMoment names,
// with ShowingAt. A word lit on no frame is -1 to -1. Nothing where the
// rate of the frames is not known.
//
// The caption's own times are on the clip's clock, and a piece of the
// short starts on the frame nearest its start, so a word can light up to
// half a frame before or after its time says. Shift and an arrow walked to
// a word by its time and landed, now and then, on a frame that still lit
// the word before, with the keyboard's frame already on the next. Tim saw
// it on #201.
func (d *CaptionDrawing) WordsLit(source SourceInfo) [][][2]float64 {
	if source.FPSNum <= 0 || source.FPSDen <= 0 {
		return nil
	}
	out := make([][][2]float64, len(d.Laid))
	for i, c := range d.Laid {
		out[i] = make([][2]float64, len(c.Words))
		for k := range out[i] {
			out[i][k] = [2]float64{-1, -1}
		}
	}
	var k int64
	for _, seg := range d.Clip.Segments {
		first, end := source.pieceFrames(seg.Start, seg.End)
		for n := first; n < end; n++ {
			on := ShowingAt(d.Laid, source.shortMoment(k))
			k++
			if on.Caption < 0 || on.Word < 0 || on.Word >= len(out[on.Caption]) {
				continue
			}
			span := &out[on.Caption][on.Word]
			if span[0] < 0 {
				span[0] = source.keptAt(n)
			}
			span[1] = source.keptAt(n + 1)
		}
	}
	return out
}
