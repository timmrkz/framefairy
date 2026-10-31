package engine

import (
	"context"
	"math"

	"framefairy/engine/num"
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
// ink, each line's reach, see pillLines, and the ink of a capital on each
// line, see baseLines, all from the caption's anchor, the middle of its
// bottom edge.
type wordsLaid struct {
	words  []string
	lines  [][]int
	lineOf []int
	boxes  []*inkBox
	reach  []*inkBox
	caps   []*inkBox
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
		blocks = append(blocks, baseLines(len(lines))...)
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
		layouts[i].caps = boxes[at+n+rows : at+n+2*rows]
		at += n + 2*rows
		for _, b := range boxes[at-n-2*rows : at] {
			if b == nil {
				e.Log.Detail("a caption word could not be measured, the highlight is off for this clip")
				return nil, false
			}
		}
	}
	return layouts, true
}

// pillRadius is the radius of the corners of a pill h high, in a style
// whose corners are radius, both in pixels of the short: three tenths of its
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

// litRun is the words first to last of a caption, counted across its
// lines, lit together from the hundredth of a second from of the short to
// the hundredth to.
type litRun struct{ first, last, from, to int }

// litRuns is how the words of a caption that shows from start to end are
// lit on the frames of the short s, in their order, see popGrid: each word
// from when it begins, see wordBegins, on the frames that start before
// the next one begins. A word too short to have a frame start in its time
// has no frame of its own, and is lit on the frame showing when it
// begins together with the words lit there, so the frame lights them
// together and they pop together, #280. That run of words is lit on that
// one frame only: the word before it is still lit alone on its frames
// before. A word with no frame before the first word with one shares that
// word's first frame, and so does a word of no length whose hundredth a
// frame starts on, which shows the word after it. Words of no length at
// the end of a caption share its last frame.
//
// A word typed into a word a frame or two long was lit on no frame, so it
// was never seen lit and Shift and the arrows could not reach it. Tim
// asked that how many words go into so little time stays the person's
// choice, so the time is not stretched, the words share the frame.
func (s SourceInfo) litRuns(c LaidCaption, start, end int) []litRun {
	g := s.popGrid()
	begins := wordBegins(c, start, end)
	// The first frame of the short that starts at or after the hundredth
	// cs.
	firstFrame := func(cs int) int64 {
		k := g.frameOf(int64(cs) * 10)
		if g.shortMoment(k) < int64(cs)*10 {
			k++
		}
		return k
	}
	var runs []litRun
	// share lights the words up to k on the frame showing the moment at,
	// in milliseconds, with the words lit there, the last run, until the
	// hundredth to. The words of that run are lit alone until the frame
	// they share, from the hundredth that frame starts in. False where
	// the run would light no frame: the frame starts on the very
	// hundredth the words end on.
	share := func(k int, at int64, to int) bool {
		last := &runs[len(runs)-1]
		shared := g.frameOf(at)
		if shared <= firstFrame(last.from) {
			last.last, last.to = k, to
			return true
		}
		cut := int(g.shortMoment(shared) / 10)
		if cut >= to {
			return false
		}
		last.to = cut
		runs = append(runs, litRun{last.first, k, cut, to})
		return true
	}
	waiting := -1
	for k, from := range begins {
		to := end
		if k+1 < len(begins) {
			to = begins[k+1]
		}
		own := firstFrame(from)
		switch {
		case to > from && g.shortMoment(own) < int64(to)*10:
			if waiting < 0 {
				runs = append(runs, litRun{k, k, from, to})
				continue
			}
			// The words waiting share this word's first frame, and it is
			// lit alone on the frames after. Above 100 frames a second the
			// next frame can start in the same hundredth, and lights them
			// too.
			cut := to
			if next := g.shortMoment(own + 1); next < int64(to)*10 {
				cut = min(to, max(int(next/10), int(g.shortMoment(own)/10)+1))
			}
			runs = append(runs, litRun{waiting, k, begins[waiting], cut})
			if cut < to {
				runs = append(runs, litRun{k, k, cut, to})
			}
			waiting = -1
		case len(runs) > 0 && share(k, int64(from)*10, to):
			// Any word waiting is in the run shared, which ends on this
			// one.
			waiting = -1
		default:
			// No frame before it lights a word of this caption, or it is
			// a word of no length whose hundredth a frame starts on, which
			// shows the word after: it waits for the next word with a
			// frame, and shares that word's first frame.
			if waiting < 0 {
				waiting = k
			}
		}
	}
	if waiting >= 0 && (len(runs) == 0 || !share(len(begins)-1, int64(end)*10-1, end)) {
		runs = append(runs, litRun{waiting, len(begins) - 1, begins[waiting], end})
	}
	return runs
}

// CaptionShowing is what libass shows of the captions at a moment of the
// short, in milliseconds, by the times their caption file gives them:
// the caption, -1 for none, and the words of it lit, Word to Last,
// counted across its lines, both -1 for none yet. It is one word but where
// words too short for a frame of their own are lit together, see
// litRuns. The caption box over the video preview shows what the layer
// under it shows by it, see /frames/captions.
type CaptionShowing struct {
	Caption int `json:"caption"`
	Word    int `json:"word"`
	Last    int `json:"last"`
}

// ShowingAt is what libass shows of laid at the moment ms of a short of
// the frames of source, see CaptionShowing.
func ShowingAt(laid []LaidCaption, source SourceInfo, ms int64) CaptionShowing {
	return showing(laid, source.runsOf(laid), ms)
}

// runsOf is how the words of each caption of laid are lit on the frames
// of the short s, see litRuns.
func (s SourceInfo) runsOf(laid []LaidCaption) [][]litRun {
	runs := make([][]litRun, len(laid))
	for i, c := range laid {
		if start, end := centiseconds(c.Start), centiseconds(c.End); end > start {
			runs[i] = s.litRuns(c, start, end)
		}
	}
	return runs
}

// showing is what libass shows of laid, its words lit in runs, see
// runsOf, at the moment ms, see ShowingAt. Worked out once, the runs
// serve every frame of a clip, see WordsLit.
func showing(laid []LaidCaption, runs [][]litRun, ms int64) CaptionShowing {
	for i, c := range laid {
		start, end := centiseconds(c.Start), centiseconds(c.End)
		if end <= start || ms < int64(start)*10 || ms >= int64(end)*10 {
			continue
		}
		on := CaptionShowing{Caption: i, Word: -1, Last: -1}
		for _, r := range runs[i] {
			if ms >= int64(r.from)*10 {
				on.Word, on.Last = r.first, r.last
			}
		}
		return on
	}
	return CaptionShowing{Caption: -1, Word: -1, Last: -1}
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

// runPill is the pill of the words of run at rest, from the caption's
// anchor, at size: a part on each line they stand on, from the left of the
// pill of the first of them on it to the right of the pill of the last,
// see pill, in the order of the lines. Words lit together are one thing
// lit, so they sit in one pill that pops from its middle, #280. Nothing
// where every word of it is in hidden, being typed: the caption box draws
// the pill of a word typed, see CaptionDrawing.Typing. Among other words
// the place of a word typed stays in the pill, so it grows round the
// middle the caption box knows, see WordFrame.Grow, and the pill the
// caption box draws for it is never smaller than that place.
func (l wordsLaid) runPill(run litRun, hidden map[int]bool, size int) [][4]float64 {
	shown := false
	for k := run.first; k <= run.last; k++ {
		shown = shown || !hidden[k]
	}
	if !shown {
		return nil
	}
	var parts [][4]float64
	for _, line := range l.lines {
		part, found := [4]float64{}, false
		for _, k := range line {
			if k < run.first || k > run.last {
				continue
			}
			left, top, right, bottom := l.pill(k, size)
			if !found {
				part, found = [4]float64{left, top, right, bottom}, true
				continue
			}
			part[0], part[2] = math.Min(part[0], left), math.Max(part[2], right)
		}
		if found {
			parts = append(parts, part)
		}
	}
	return parts
}

// lineBand is how far down the pills of line r reach at rest, the same for
// every word of the line, so the pill keeps its height as it goes from
// word to word: the line's reach, and any letter of the line that reaches
// further, with pillPadY above and below, but never
// nearer than lineClear to a letter of the line above or below, over the
// whole width of the line. Where the letters of the two lines come so near
// that this leaves the line's own letters less than lineClear of room, the
// pill keeps that room and touches the other line instead.
//
// What one side gives up, the other gives up too, so the pill stays round
// the middle of its line: stopped short at one side only, the pills of the
// top line of two stood high over their words and the pills of the bottom
// line low under them, and Tim saw the words pushed up and down in them.
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
	inkTop, inkBottom := l.lineInk(r)
	high, low := math.Min(float64(line.top), inkTop), math.Max(float64(line.bottom), inkBottom)
	top, bottom = high-padY, low+padY
	if above, ok := l.nearestInk(r, r-1, size); ok {
		top = math.Max(top, math.Min(above+1+clear, inkTop-clear))
	}
	if below, ok := l.nearestInk(r, r+1, size); ok {
		bottom = math.Min(bottom, math.Max(below-clear, inkBottom+1+clear))
	}
	mid := (high + low) / 2
	half := math.Min(mid-top, bottom-mid)
	top, bottom = math.Ceil(mid-half), math.Floor(mid+half)
	return math.Min(top, inkTop-clear), math.Max(bottom, inkBottom+1+clear)
}

// extent is the box the whole caption takes: its ink across, and down
// from the top of its first line's reach, or a letter that reaches higher,
// to the bottom of its last line's reach, or a letter that reaches lower.
// It is the same height for every caption of as many lines, and the pills
// of its first and last lines stand in it with as much room over them as
// under them, see lineBand.
func (l wordsLaid) extent() inkBox {
	full := inkBox{math.MaxInt32, math.MaxInt32, math.MinInt32, math.MinInt32}
	for _, b := range l.boxes {
		full.left, full.right = min(full.left, b.left), max(full.right, b.right)
		full.top, full.bottom = min(full.top, b.top), max(full.bottom, b.bottom)
	}
	last := len(l.lines) - 1
	full.top = min(full.top, l.reach[0].top)
	full.bottom = max(full.bottom, l.reach[last].bottom)
	return full
}

// baseline is the row the letters of line r stand on, from the caption's
// anchor: the bottom edge of the ink of a capital on it.
func (l wordsLaid) baseline(r int) float64 { return float64(l.caps[r].bottom + 1) }

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
	// Base is the baseline of its line, where its letters stand, as a
	// share of the short's height from its top. The word being typed is
	// set on it, so its letters stand where libass drew them, see
	// docs/CAPTIONS.md.
	Base float64 `json:"base"`
	// Grow is where the pill of the words lit together from this one grows
	// from in the pop, the middle of the whole pill, as shares of the
	// short's width and height from its top left corner, see litRuns and
	// runPill. The caption box grows the frame round each of them from it.
	// Nil where this word is not the first of words lit together.
	Grow *GrowFrom `json:"grow,omitempty"`
	// From and To are when the picture lights the word, on the episode's
	// clock: from the start of the first frame of the clip libass draws it
	// lit on to the end of the last, see WordsLit. Both -1 for a word lit
	// on no frame, or where the frames are not known.
	From float64 `json:"from"`
	To   float64 `json:"to"`
}

// GrowFrom is a point of the short, as shares of its width and height
// from its top left corner.
type GrowFrom struct {
	X float64 `json:"x"`
	Y float64 `json:"y"`
}

// WordFrames is where the words of the captions stand in a short width by
// height, each caption's words in their order, as CaptionTrack lays them
// out in the style overrides make. Nothing when they cannot be measured.
func (e *Engine) WordFrames(ctx context.Context, laid []LaidCaption, width, height int,
	overrides map[string]any, light framewire.Light) [][]WordFrame {
	s := ResolveStyle(overrides).inLight(light)
	scale := float64(height) / 1920.0
	size := max(12, num.Round(s.Size*scale))
	layouts, ok := e.layWords(ctx, laid, s, width, size)
	if !ok {
		return nil
	}
	ax := float64(width) / 2
	ay := float64(height - num.Round(s.MarginV*scale))
	radius := float64(num.Round(s.Radius * scale))
	out := make([][]WordFrame, len(layouts))
	for i, l := range layouts {
		out[i] = make([]WordFrame, len(l.words))
		for k := range l.words {
			left, top, right, bottom := l.pill(k, size)
			out[i][k] = WordFrame{X: (ax + left) / float64(width), Y: (ay + top) / float64(height),
				W: (right - left) / float64(width), H: (bottom - top) / float64(height),
				R:    pillRadius(bottom-top, radius) / float64(height),
				Base: (ay + l.baseline(l.lineOf[k])) / float64(height)}
		}
	}
	return out
}

// WordsLit is when the picture lights each word of the captions of d, on
// the episode's clock, each caption's words in their order: from the start
// of the first frame of the clip on which libass draws the word lit to the
// end of the last, read frame by frame at the moment CaptionMoment names,
// with ShowingAt. Words lit together on a frame, see litRuns, each have
// it among theirs. A word lit on no frame is -1 to -1. Nothing where the
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
	runs := source.runsOf(d.Laid)
	var k int64
	for _, seg := range d.Clip.Segments {
		first, end := source.pieceFrames(seg.Start, seg.End)
		for n := first; n < end; n++ {
			on := showing(d.Laid, runs, source.shortMoment(k))
			k++
			if on.Caption < 0 || on.Word < 0 || on.Last >= len(out[on.Caption]) {
				continue
			}
			for w := on.Word; w <= on.Last; w++ {
				span := &out[on.Caption][w]
				if span[0] < 0 {
					span[0] = source.keptAt(n)
				}
				span[1] = source.keptAt(n + 1)
			}
		}
	}
	return out
}
