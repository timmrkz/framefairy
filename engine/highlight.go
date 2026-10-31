package engine

import (
	"context"
	"fmt"
	"math"
	"strconv"
	"strings"

	"framefairy/engine/num"
	"framefairy/engine/runes"
	"framefairy/engine/timed"
)

// Caption is one caption on a clip's timeline, with the words it is made of.
// The words drive the highlight, which moves from word to word as they are
// spoken.
type Caption struct {
	Start float64
	End   float64
	Text  string
	Words []timed.Cue
	// Opens and Closes are when the first and the last word it was laid
	// out from start in the episode, a word removed among them. They are
	// the words its edges are kept against, see CaptionTimes, and they stay
	// the same when a word at either end is removed, so the caption is
	// still known by them, to remove it or to put the word back.
	Opens, Closes float64
	// trimStart and trimEnd say the word it opens or closes on was
	// removed, so that edge is where the words shown put it, unless the
	// hand put it elsewhere, see moveCaptions.
	trimStart, trimEnd bool
	// startMoved and endMoved say the edge is where it was put by hand.
	startMoved, endMoved bool
}

// Cue is the caption without its words.
func (c Caption) Cue() timed.Cue { return timed.Cue{Start: c.Start, End: c.End, Text: c.Text} }

// ---------------------------------------------------------------------------
// The bouncing highlight
//
// Every word is placed by measurement. Each caption is rendered once per
// word with only that word visible, so the ink of every word is known exactly
// where libass puts it in the finished line. The active word is then drawn on
// its own layer, on top of the same line with that word hidden, so the words
// around it never move.
//
// libass scales a line around its anchor point, which would carry a growing
// word sideways. Moving the anchor by (1 - scale) times the distance from the
// anchor to the word's centre cancels that exactly, so the word grows and
// shrinks around its own centre.
//
// The word being spoken and its pill are one thing that comes towards the
// viewer: both grow by one scale around one centre, the middle of the
// pill, in every direction alike, then settle back to rest. It is the
// karaoke "grow" of Aegisub's templates, a word set on its own centre and
// scaled up and back down.
//
// The pop is laid out on the frames of the short, popScales, because the
// frames are all anybody sees. It starts at rest on the word's first
// frame, reaches popPeak on a frame of its own, and is back at rest by
// the word's last frame, in no more than popLength. Each frame holds one
// scale, from the hundredth its frame starts in, so libass draws exactly
// that scale on that frame. A curve in milliseconds, sampled wherever the
// frames fell, put the peak on a frame or between two by chance, and cut
// a short word off on its way up: Tim saw some words pop fully, some
// barely and some rise and snap back, in one caption at 25 fps.
//
// #201 had made the bounce a few pixels in height only, the pill
// stretched up or down to keep off the other line and the word grown
// 3 pixels a side, so Tim saw the pill move up or down and the word not
// at all. At its peak a long word and its pill reach over the space to
// the next word for a frame or two, as in every caption tool, and the
// pop is over in a fifth of a second. The pill and its word lie over the
// words beside them, a layer of their own coming forward: Tim saw the
// letters of the next word reach into the pill at its peak, and wanted
// the pop to stand out over everything round it.
//
// At rest the pill leaves room all round its word: as tall as the line,
// from the top of the capitals and the tall letters the face draws to the
// bottom of the deepest, and over any letter of the line that reaches
// further, an accent on a capital, with pillPadY over that, the same for
// every word of the line whatever its letters. Where another line of the caption stands over or
// under it, the pills of its line rest short of that line's letters, all
// at one height, see wordsLaid.lineBand.
// ---------------------------------------------------------------------------

const (
	// popLength is the longest the pop lasts, from rest to rest, in
	// milliseconds, popRise the share of it it takes to reach popPeak,
	// its scale at the top.
	popLength = 220
	popRise   = 0.55
	popPeak   = 1.15
	// pillPadX is the room the pill leaves at the sides of its word, and
	// pillPadY above and below its line, as shares of the caption size.
	pillPadX = 0.14
	pillPadY = 0.10
	// lineClear is the least room the pill at rest leaves to a letter of
	// another line of its caption, as a share of the caption size, see
	// wordsLaid.lineBand.
	lineClear = 0.04
	// lineReach is the capitals, the tall letters and the deepest letters
	// of a line, whose ink says how tall the line is, see pillLines. An
	// accent over a capital reaches higher, and only a line that has one
	// makes room for it, see wordsLaid.lineBand: measured with accents, the
	// pill of every line stood high over its words, "so'n" twice as far
	// under its top as over its bottom, which Tim saw.
	lineReach = "Hbdfhklgjpqy"
)

// BounceCurve is the pop as the video preview plays it after a word step,
// at the screen's own rate rather than on the frames of a short, see
// CaptionDrawing.Lifted: the same curve, length long in milliseconds,
// rising like a sine for the share rise of it to peak and settling like a
// cosine back to rest.
func BounceCurve() (length, rise, peak float64) {
	return popLength, popRise, popPeak
}

// popScales is the scale of a word lit for lit frames of a short of fps
// frames a second, frame by frame from its first: at rest on the first,
// popPeak on one frame, at rest again by its last, eased in and out like a
// sine on either side of the peak, and over in popLength or the frames it
// has, whichever is less. A word of two frames shows its peak on the
// second. A word of one frame shows its peak on that frame: the frame
// before it is still the word before, and the frame after it the word
// after, so the rise and the settle are the hand-over between them, the
// way karaoke effects run past a syllable's own time, see retime in
// Aegisub's kara-templater.
func popScales(lit int, fps float64) []float64 {
	if lit < 1 {
		return []float64{1}
	}
	if lit == 1 {
		return []float64{popPeak}
	}
	n := min(lit-1, max(1, int(math.Round(popLength*fps/1000))))
	if n == 1 {
		return []float64{1, popPeak}
	}
	p := min(max(int(math.Round(popRise*float64(n))), 1), n-1)
	scales := make([]float64, n+1)
	for k := range scales {
		if k <= p {
			scales[k] = 1 + (popPeak-1)*math.Sin(math.Pi/2*float64(k)/float64(p))
		} else {
			scales[k] = 1 + (popPeak-1)*math.Cos(math.Pi/2*float64(k-p)/float64(n-p))
		}
	}
	scales[0], scales[p], scales[n] = 1, popPeak, 1
	return scales
}

// popGrid is the frames the pop is laid out on: the short's, or 30 a
// second where there are none to count, as for a caption file written on
// its own.
func (s SourceInfo) popGrid() SourceInfo {
	if s.FPSNum <= 0 || s.FPSDen <= 0 {
		return SourceInfo{FPSNum: 30, FPSDen: 1}
	}
	return s
}

// frameOf is the frame of the short showing the moment ms: the last frame
// that starts at or before it.
func (s SourceInfo) frameOf(ms int64) int64 {
	k := max(0, int64(math.Floor(float64(ms)*float64(s.FPSNum)/(float64(s.FPSDen)*1000)))-1)
	for s.shortMoment(k+1) <= ms {
		k++
	}
	return k
}

// wordPop is the pop of a word lit from from to to, hundredths of a second
// of the short: its first frame, the first that starts at or after from,
// and its scale frame by frame from there, see popScales.
func (s SourceInfo) wordPop(from, to int) (first int64, scales []float64) {
	g := s.popGrid()
	first = g.frameOf(int64(from) * 10)
	if g.shortMoment(first) < int64(from)*10 {
		first++
	}
	lit := 0
	for lit < 64 && g.shortMoment(first+int64(lit)) < int64(to)*10 {
		lit++
	}
	return first, popScales(lit, float64(g.FPSNum)/float64(g.FPSDen))
}

// popSpan is a stretch of hundredths of a second of the short in which
// the pill of a word is drawn at one scale.
type popSpan struct {
	from, to int
	scale    float64
}

// popSpans is the pop of a word lit from from to to, hundredths of a
// second of the short, as the caption file writes it: at rest until its
// first frame, each frame's scale from the hundredth its frame starts in
// to the hundredth the next starts in, and at rest after the pop. Spans of
// no length are left out, since libass draws nothing for them: above 100
// fps two frames can start in one hundredth, and the first of them shows
// the second's scale. PopScale reads the same spans, so what it says is
// what libass draws on every frame.
func (s SourceInfo) popSpans(from, to int) []popSpan {
	first, scales := s.wordPop(from, to)
	grid := s.popGrid()
	edge := func(j int) int {
		return min(to, max(from, int(grid.shortMoment(first+int64(j))/10)))
	}
	var spans []popSpan
	add := func(scale float64, a, b int) {
		if b > a {
			spans = append(spans, popSpan{a, b, scale})
		}
	}
	add(1, from, edge(0))
	for j, scale := range scales {
		add(scale, edge(j), edge(j+1))
	}
	add(1, edge(len(scales)), to)
	return spans
}

// pillLines are the blocks that measure how tall each line of a caption of
// so many lines is: the tallest and the deepest letters the face draws,
// lineReach, on that line alone. libass gives every line of a face the
// same height, whatever its letters, so a line's pill is the same height
// for every word on it, a word of small letters as much as a word with a
// tail.
func pillLines(rows int) []string { return lineBlocks(rows, lineReach) }

// baseLines are the blocks that find where each line of a caption of so
// many lines stands: a capital H on that line alone, whose ink ends on
// the line's baseline, see wordsLaid.baseline.
func baseLines(rows int) []string { return lineBlocks(rows, "H") }

// lineBlocks are blocks of a caption of so many lines with text on one
// line of each and nothing on the others.
func lineBlocks(rows int, text string) []string {
	words := make([]string, rows)
	lines := make([][]int, rows)
	for r := range rows {
		words[r] = text
		lines[r] = []int{r}
	}
	out := make([]string, rows)
	for r := range rows {
		out[r] = taggedText(words, lines, func(j int) bool { return j == r }, shownTag(""))
	}
	return out
}

// lineIndices turns laid out lines back into the word numbers they hold,
// which is what the tagging below works with.
func lineIndices(lines [][]timed.Cue) [][]int {
	out := make([][]int, len(lines))
	at := 0
	for i, line := range lines {
		for range line {
			out[i] = append(out[i], at)
			at++
		}
	}
	return out
}

// WebColour turns an ASS colour, &HAABBGGRR or &HBBGGRR&, into the CSS
// colour the app can paint with. The alpha runs backwards in ASS, 00 is
// opaque, so it is turned around here.
func WebColour(value string) string {
	raw := strings.ToUpper(strings.TrimSuffix(strings.TrimPrefix(runes.Trim(value), "&H"), "&"))
	if len(raw) < 6 || strings.TrimLeft(raw, "0123456789ABCDEF") != "" {
		return "rgba(255, 255, 255, 1)"
	}
	alpha := 1.0
	if len(raw) >= 8 {
		if v, err := strconv.ParseInt(raw[len(raw)-8:len(raw)-6], 16, 0); err == nil {
			alpha = 1 - float64(v)/255
		}
	}
	bgr := raw[len(raw)-6:]
	channel := func(part string) int64 {
		v, _ := strconv.ParseInt(part, 16, 0)
		return v
	}
	return fmt.Sprintf("rgba(%d, %d, %d, %s)", channel(bgr[4:6]), channel(bgr[2:4]),
		channel(bgr[0:2]), strconv.FormatFloat(num.RoundTo(alpha, 3), 'f', -1, 64))
}

const hidden = `{\alpha&HFF&}`

// shownTag is how a word is shown: as see-through as the text colour is.
// A word is hidden and shown by its alpha, and a shown word written as
// fully opaque would throw away the opacity the text colour was given.
func shownTag(primary string) string {
	return `{\alpha&H` + alphaOf(primary) + `&}`
}

// alphaOrSolid is an alpha as a tag takes it, solid when there is none.
func alphaOrSolid(alpha string) string {
	if len(alpha) != 2 || !isHex(alpha) {
		return "00"
	}
	return strings.ToUpper(alpha)
}

// alphaOf is the alpha of an &HAABBGGRR colour, 00 when it has none.
func alphaOf(colour string) string {
	raw := strings.ToUpper(strings.TrimSuffix(strings.TrimPrefix(colour, "&H"), "&"))
	if len(raw) == 8 && isHex(raw) {
		return raw[:2]
	}
	return "00"
}

// taggedText is the caption with each word shown or hidden. Hidden words
// still take their place in the line.
func taggedText(words []string, lines [][]int, visible func(int) bool, shown string) string {
	rows := make([]string, len(lines))
	for r, line := range lines {
		parts := make([]string, len(line))
		for p, i := range line {
			tag := hidden
			if visible(i) {
				tag = shown
			}
			parts[p] = tag + words[i]
		}
		rows[r] = strings.Join(parts, " ")
	}
	return strings.Join(rows, `\N`)
}

func centiseconds(t float64) int { return max(0, num.Round(t*100)) }

func csToASS(cs int) string {
	return fmt.Sprintf("%d:%02d:%02d.%02d", cs/360000, cs/6000%60, cs/100%60, cs%100)
}

// tagNumber writes v for a tag of the caption file, to two decimals and
// no more digits than it needs.
func tagNumber(v float64) string { return strconv.FormatFloat(num.RoundTo(v, 2), 'f', -1, 64) }

// highlighted is the caption track with the bouncing highlight, for a
// short of the frames of source, see popScales. It is nothing,
// without an error, when the captions cannot be measured, and the plain
// track is made instead.
func (e *Engine) highlighted(ctx context.Context, captions []LaidCaption,
	width, height int, s Style, source SourceInfo) ([]byte, error) {
	shown := shownTag(s.Primary)
	scale := float64(height) / 1920.0
	size := max(12, num.Round(s.Size*scale))
	outline := max(1, num.Round(s.Outline*scale))
	shadow := max(0, num.Round(s.Shadow*scale))
	marginH := num.Round(s.MarginH * scale)
	marginV := num.Round(s.MarginV * scale)

	layouts, ok := e.layWords(ctx, captions, s, width, size)
	if !ok {
		return nil, nil
	}

	ax := float64(width) / 2
	ay := float64(height - marginV)
	drawBox := (s.BorderStyle == 3 || s.BorderStyle == 4) && s.Box
	boxAlpha := "80"
	back := s.boxColour()
	if len(back) >= 10 {
		boxAlpha = back[2:4]
	}
	boxFill := "&H" + back[max(0, len(back)-6):] + "&"
	padX := float64(num.Round(s.BoxPadX * scale))
	padY := float64(num.Round(s.BoxPadY * scale))
	radius := float64(num.Round(s.Radius * scale))
	anchor := fmt.Sprintf(`\an2\pos(%s,%s)`, tagNumber(ax), tagNumber(ay))

	var events []string
	add := func(layer, from, to int, style, text string) {
		if to > from {
			events = append(events, fmt.Sprintf("Dialogue: %d,%s,%s,%s,,0,0,0,,%s",
				layer, csToASS(from), csToASS(to), style, text))
		}
	}

	for i, c := range captions {
		L := layouts[i]
		start, end := centiseconds(c.Start), centiseconds(c.End)
		if end <= start {
			continue
		}
		// Extent of the whole caption: its ink across, and down from the
		// top of its first line's reach to the bottom of its last, the
		// rows the pills of those lines stand round, see lineBand. Round
		// the ink alone, a caption with no tall or deep letter had a box
		// that hugged its letters while its pill reached down for a tail
		// it did not have, and Tim saw the pill of "Und" touch the bottom
		// of the box with room over it on #201.
		full := L.extent()
		whole, word := c.Lift == "", c.Lift == LiftWord
		if drawBox && !word {
			shape := roundedRect(ax+float64(full.left)-padX, ay+float64(full.top)-padY,
				ax+float64(full.right)+padX, ay+float64(full.bottom)+padY, radius)
			add(0, start, end, "Box", fmt.Sprintf(
				`{\p1\pos(0,0)\1c%s\1a&H%s&\bord0\shad0}%s{\p0}`, boxFill, boxAlpha, shape))
		}

		begins := wordBegins(c, start, end)
		all := func(j int) bool { return !c.Hidden[j] }
		if !word {
			add(2, start, begins[0], "Caption", "{"+anchor+"}"+taggedText(L.words, L.lines, all, shown))
		}

		for _, run := range source.litRuns(c, start, end) {
			from, to := run.from, run.to
			if to <= from {
				continue
			}
			lit := func(j int) bool { return j >= run.first && j <= run.last }

			// The line with the words lit hidden, so nothing else moves.
			if !word {
				add(2, from, to, "Caption", "{"+anchor+"}"+
					taggedText(L.words, L.lines, func(j int) bool { return !lit(j) && !c.Hidden[j] }, shown))
			}

			// The pill at rest, a part on each line the words lit stand
			// on, and the centre the words and the pill pop around: the
			// middle of the whole pill, which is the middle of one word's
			// ink across where one word is lit.
			// The word being typed has neither its ink nor its pill here:
			// the caption box draws both, the pill growing with what is
			// typed, see CaptionDrawing.Typing. libass drew the pill at the
			// word's old size, and Tim saw the word typed run out of it.
			parts := L.runPill(run, c.Hidden, size)
			if len(parts) == 0 {
				continue
			}
			pl, pt, pr, pb := parts[0][0], parts[0][1], parts[0][2], parts[0][3]
			for _, p := range parts[1:] {
				pl, pt, pr, pb = math.Min(pl, p[0]), math.Min(pt, p[1]), math.Max(pr, p[2]), math.Max(pb, p[3])
			}
			cx := ax + (pl+pr)/2
			cy := ay + (pt+pb)/2
			shapes := make([]string, len(parts))
			for n, p := range parts {
				shapes[n] = roundedRect(p[0]-pl, p[1]-pt, p[2]-pl, p[3]-pt, pillRadius(p[3]-p[1], radius))
			}
			pill := strings.Join(shapes, " ")

			// The pill, and the words lit alone over it, both at one
			// scale around one centre, one scale a frame, over the line,
			// so the words beside them never reach into the pill. The words are the
			// whole caption with the others hidden, so they stand where they
			// stand in the line, and their anchor moves with the scale to
			// keep its centre still.
			only := taggedText(L.words, L.lines, func(j int) bool { return lit(j) && !c.Hidden[j] }, shown)
			at := func(scale float64, from, to int) {
				x, y := tagNumber(ax+(1-scale)*(cx-ax)), tagNumber(ay+(1-scale)*(cy-ay))
				add(3, from, to, "Box", fmt.Sprintf(`{\an5\pos(%s,%s)\p1\1c%s\1a&H%s&\bord0\shad0\fscx%s\fscy%s}%s{\p0}`,
					tagNumber(cx), tagNumber(cy), s.HighlightColour, alphaOrSolid(s.HighlightAlpha), tagNumber(scale*100), tagNumber(scale*100), pill))
				add(4, from, to, "Caption", fmt.Sprintf(`{\an2\pos(%s,%s)\fscx%s\fscy%s}%s`,
					x, y, tagNumber(scale*100), tagNumber(scale*100), only))
			}
			// Each frame of the pop holds its scale from the hundredth its
			// frame starts in to the hundredth the next starts in, so the
			// frame shows exactly that scale. Before the first frame, and
			// after the pop, the words are at rest.
			// A part of the caption is drawn at rest, and the words only
			// with the words, see LaidCaption.Lift.
			if !whole {
				if word {
					at(1, from, to)
				}
				continue
			}
			for _, span := range source.popSpans(from, to) {
				at(span.scale, span.from, span.to)
			}
		}
	}

	header := assHeader(s, width, height, size, 1, outline, shadow, marginH, marginV)
	return []byte(header + strings.Join(events, "\n") + "\n"), nil
}
