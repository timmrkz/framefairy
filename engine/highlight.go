package engine

import (
	"context"
	"fmt"
	"math"
	"strconv"
	"strings"
)

// Caption is one caption on a clip's timeline, with the words it is made of.
// The words drive the highlight, which moves from word to word as they are
// spoken.
type Caption struct {
	Start float64
	End   float64
	Text  string
	Words []Cue
	// Opens and Closes are when the first and the last word it was laid
	// out from start in the episode, a word removed among them. They are
	// the words its edges are kept against, see CaptionTimes, and they stay
	// the same when a word at either end is removed, so the caption is
	// still known by them, to remove it or to put the word back.
	Opens, Closes float64
	// trimStart and trimEnd say the word it opens or closes on was
	// removed, so that edge is where the words shown put it, unless the
	// hand put it in past the middle of the word removed there, which is
	// at startPast or endPast on the clip's clock, see moveCaptions.
	trimStart, trimEnd bool
	startPast, endPast float64
	// startMoved and endMoved say the edge is where it was put by hand.
	startMoved, endMoved bool
}

// Cue is the caption without its words.
func (c Caption) Cue() Cue { return Cue{c.Start, c.End, c.Text} }

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
// shrinks around its own centre. Both the scale and the move are linear in
// time within each piece of the pop, which keeps them in step frame by
// frame.
//
// The word being spoken and its pill are one thing that comes towards the
// viewer: both grow by one scale around one centre, the middle of the
// pill, in every direction alike, then settle back to rest. It is the
// karaoke "grow" of Aegisub's templates, a word set on its own centre with
// \t taking \fscx and \fscy up and back down, and the same pop the
// video preview played as a CSS animation before #201: a rise to the peak
// in popRise of the time, then a settle, each eased out, CSS's ease-out.
//
// libass moves along \move and \t in a straight line, and the word stays
// on its centre only while its scale and its move keep in step, so the
// eased curve is drawn as short straight pieces, popKnot hundredths of a
// second each, the same for the word and the pill. popDrawn is that curve
// as drawn, and the subtitle file is written from it.
//
// #201 had made the bounce a few pixels in height only, the pill
// stretched up or down to keep off the other line and the word grown
// 3 pixels a side, so Tim saw the pill move up or down and the word not
// at all. At its peak a long word and its pill reach over the space to
// the next word for a frame or two, as in every caption tool: the words
// beside it are drawn over the pill, and the pop is over in a fifth of a
// second.
//
// At rest the pill leaves room all round its word: as tall as the line,
// from the top of the tallest letter the face draws to the bottom of the
// deepest, with pillPadY over that, the same for every word of the line
// whatever its letters. Where another line of the caption stands over or
// under it, the pills of its line rest short of that line's letters, all
// at one height, see wordsLaid.lineBand.
// ---------------------------------------------------------------------------

const (
	// popLength is how long the pop lasts, in milliseconds, and popRise
	// the share of it the word takes to reach popPeak, its scale at the
	// top. It is the same for every word: a word lit for less shows as
	// much of the pop as it is lit for, and goes back to its size as the
	// next word lights, the way the CSS animation did before #201.
	popLength = 220
	popRise   = 0.55
	popPeak   = 1.15
	// popKnot is how long each straight piece of the drawn curve lasts,
	// in hundredths of a second.
	popKnot = 2
	// pillPadX is the room the pill leaves at the sides of its word, and
	// pillPadY above and below its line, as shares of the caption size.
	pillPadX = 0.14
	pillPadY = 0.10
	// lineClear is the least room the pill at rest leaves to a letter of
	// another line of its caption, as a share of the caption size, see
	// wordsLaid.lineBand.
	lineClear = 0.04
	// lineReach is the tallest and the deepest letters a line can hold,
	// whose ink says how tall the line is, see pillLines.
	lineReach = "ÉÅÁgjpqy"
)

// easeOut is CSS's ease-out, cubic-bezier(0, 0, 0.58, 1), at p from 0 to
// 1: fast at first and slowing to a stop.
func easeOut(p float64) float64 {
	if p <= 0 {
		return 0
	}
	if p >= 1 {
		return 1
	}
	x := func(u float64) float64 { return 3*(1-u)*u*u*0.58 + u*u*u }
	lo, hi := 0.0, 1.0
	for range 40 {
		mid := (lo + hi) / 2
		if x(mid) < p {
			lo = mid
		} else {
			hi = mid
		}
	}
	u := (lo + hi) / 2
	return 3*(1-u)*u*u + u*u*u
}

// popAt is the scale of the word since milliseconds after its pop
// started, on the eased curve.
func popAt(since float64) float64 {
	switch {
	case since <= 0 || since >= popLength:
		return 1
	case since < popLength*popRise:
		return 1 + (popPeak-1)*easeOut(since/(popLength*popRise))
	}
	return popPeak - (popPeak-1)*easeOut((since-popLength*popRise)/(popLength*(1-popRise)))
}

// popKnots are the hundredths of a second after the pop started where the
// straight pieces of its curve meet, the first 0 and the last its end.
func popKnots() []int {
	end := int(math.Ceil(popLength / 10))
	var knots []int
	for at := 0; at < end; at += popKnot {
		knots = append(knots, at)
	}
	return append(knots, end)
}

// popDrawn is the scale libass draws the word at, since milliseconds after
// its pop started: popAt at the knots, and a straight line between them.
// The same for every word, however long it is lit.
func popDrawn(since float64) float64 {
	knots := popKnots()
	for i := 1; i < len(knots); i++ {
		a, b := float64(knots[i-1]*10), float64(knots[i]*10)
		if since < b {
			if since < a {
				return 1
			}
			return popAt(a) + (popAt(b)-popAt(a))*(since-a)/(b-a)
		}
	}
	return 1
}

// popStart is the hundredth of a second the pop of a word that lights at
// from, a hundredth of a second of the short, starts at: the first frame
// of the short that shows the word, which then shows it at rest, the
// same look as every frame after the pop. A step onto a word lands on
// that frame, so the video preview stands on the word at rest and plays
// the whole pop over it, see CaptionDrawing.PopFrames. Started where the
// word lights, its first frame showed it up to a frame into its pop,
// grown by a few hundredths, and the pill stood bigger than the frame
// round it. Without frames to count, from.
func (s SourceInfo) popStart(from int) int {
	if s.FPSNum <= 0 || s.FPSDen <= 0 {
		return from
	}
	ms := int64(from) * 10
	k := max(0, int64(math.Floor(float64(ms)*float64(s.FPSNum)/(float64(s.FPSDen)*1000)))-1)
	for s.shortMoment(k) < ms {
		k++
	}
	return int((s.shortMoment(k) + 9) / 10)
}

// PopAgain is how long the short goes on past since milliseconds after
// the pop of a word lit for span hundredths of a second started until its
// pop looks again as it does then, in milliseconds: from a moment of the
// rise, through the peak, to the moment of the settle with the same
// scale, or from before the pop to its end, back at rest. A word lit for
// less than the pop goes as far as it is lit. Nothing from the peak on. The video preview plays that much of the short over a frame it
// is paused on when it lands on a word, so the word pops as it would
// while playing and comes to rest on the frame it is paused on, see
// CaptionDrawing.PopFrames.
func PopAgain(since float64, span int) float64 {
	length := min(popLength, float64(span*10))
	// The peak as drawn, the knot the pieces rise to.
	peak := 0.0
	for _, k := range popKnots() {
		if popDrawn(float64(k*10)) > popDrawn(peak) {
			peak = float64(k * 10)
		}
	}
	if since >= peak || since >= length {
		return 0
	}
	now := popDrawn(since)
	for t := peak + 1; t < length; t++ {
		if popDrawn(t) <= now {
			return t - since
		}
	}
	return length - since
}

// pillLines are the blocks that measure how tall each line of a caption of
// so many lines is: the tallest and the deepest letters the face draws,
// lineReach, on that line alone. libass gives every line of a face the
// same height, whatever its letters, so a line's pill is the same height
// for every word on it, a word of small letters as much as a word with an
// accent or a tail.
func pillLines(rows int) []string {
	words := make([]string, rows)
	lines := make([][]int, rows)
	for r := range rows {
		words[r] = lineReach
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
func lineIndices(lines [][]Cue) [][]int {
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
	raw := strings.ToUpper(strings.TrimSuffix(strings.TrimPrefix(strip(value), "&H"), "&"))
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
		channel(bgr[0:2]), strconv.FormatFloat(roundTo(alpha, 3), 'f', -1, 64))
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

func centiseconds(t float64) int { return max(0, pyround(t*100)) }

func csToASS(cs int) string {
	return fmt.Sprintf("%d:%02d:%02d.%02d", cs/360000, cs/6000%60, cs/100%60, cs%100)
}

func num(v float64) string { return strconv.FormatFloat(roundTo(v, 2), 'f', -1, 64) }

// highlighted is the caption track with the bouncing highlight, for a
// short of the frames of source, see SourceInfo.popStart. It is nothing,
// without an error, when the captions cannot be measured, and the plain
// track is made instead.
func (e *Engine) highlighted(ctx context.Context, captions []LaidCaption,
	width, height int, s Style, source SourceInfo) ([]byte, error) {
	shown := shownTag(s.Primary)
	scale := float64(height) / 1920.0
	size := max(12, pyround(s.Size*scale))
	outline := max(1, pyround(s.Outline*scale))
	shadow := max(0, pyround(s.Shadow*scale))
	marginH := pyround(s.MarginH * scale)
	marginV := pyround(s.MarginV * scale)

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
	padX := float64(pyround(s.BoxPadX * scale))
	padY := float64(pyround(s.BoxPadY * scale))
	radius := float64(pyround(s.Radius * scale))
	anchor := fmt.Sprintf(`\an2\pos(%s,%s)`, num(ax), num(ay))

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
		// Extent of the whole caption.
		full := inkBox{math.MaxInt32, math.MaxInt32, math.MinInt32, math.MinInt32}
		for _, b := range L.boxes {
			full.left, full.right = min(full.left, b.left), max(full.right, b.right)
			full.top, full.bottom = min(full.top, b.top), max(full.bottom, b.bottom)
		}
		if drawBox {
			shape := roundedRect(ax+float64(full.left)-padX, ay+float64(full.top)-padY,
				ax+float64(full.right)+padX, ay+float64(full.bottom)+padY, radius)
			add(0, start, end, "Box", fmt.Sprintf(
				`{\p1\pos(0,0)\1c%s\1a&H%s&\bord0\shad0}%s{\p0}`, boxFill, boxAlpha, shape))
		}

		begins := wordBegins(c, start, end)
		all := func(j int) bool { return !c.Hidden[j] }
		add(2, start, begins[0], "Caption", "{"+anchor+"}"+taggedText(L.words, L.lines, all, shown))

		for k := range L.words {
			from := begins[k]
			to := end
			if k+1 < len(begins) {
				to = begins[k+1]
			}
			if to <= from {
				continue
			}
			k := k
			// The pill at rest, and the centre the word and its pill pop
			// around: the middle of the pill, which is the middle of the
			// word's ink across.
			pl, pt, pr, pb := L.pill(k, size)
			cx := ax + (pl+pr)/2
			cy := ay + (pt+pb)/2
			w, h := pr-pl, pb-pt
			pill := roundedRect(0, 0, w, h, pillRadius(h, radius))

			// The line with the active word hidden, so nothing else moves.
			add(2, from, to, "Caption", "{"+anchor+"}"+
				taggedText(L.words, L.lines, func(j int) bool { return j != k && !c.Hidden[j] }, shown))

			// The pill, and the active word alone over it, popping piece by
			// piece of the curve, both at one scale around one centre. The
			// word is the whole caption with the others hidden, so it stands
			// where it stands in the line, and its anchor moves with the
			// scale to keep its centre still.
			only := taggedText(L.words, L.lines, func(j int) bool { return j == k && !c.Hidden[j] }, shown)
			place := func(scale float64) (string, string) {
				return num(ax + (1-scale)*(cx-ax)), num(ay + (1-scale)*(cy-ay))
			}
			pop := func(from, to int, a, z float64) {
				ms := (to - from) * 10
				add(1, from, to, "Box", fmt.Sprintf(
					`{\an5\pos(%s,%s)\p1\1c%s\1a&H%s&\bord0\shad0\fscx%s\fscy%s\t(0,%d,\fscx%s\fscy%s)}%s{\p0}`,
					num(cx), num(cy), s.HighlightColour, alphaOrSolid(s.HighlightAlpha),
					num(a*100), num(a*100), ms, num(z*100), num(z*100), pill))
				x0, y0 := place(a)
				x1, y1 := place(z)
				add(3, from, to, "Caption", fmt.Sprintf(
					`{\an2\move(%s,%s,%s,%s,0,%d)\fscx%s\fscy%s\t(0,%d,\fscx%s\fscy%s)}%s`,
					x0, y0, x1, y1, ms, num(a*100), num(a*100), ms, num(z*100), num(z*100), only))
			}
			// At rest until the first frame of the short that shows the
			// word, then the pop, then at rest again.
			still := func(from, to int) {
				add(1, from, to, "Box", fmt.Sprintf(`{\an5\pos(%s,%s)\p1\1c%s\1a&H%s&\bord0\shad0}%s{\p0}`,
					num(cx), num(cy), s.HighlightColour, alphaOrSolid(s.HighlightAlpha), pill))
				add(3, from, to, "Caption", "{"+anchor+"}"+only)
			}
			// A word lit for less than the pop shows the pop until the
			// next word lights, on the same curve as every other word.
			starts := min(source.popStart(from), to)
			still(from, starts)
			end := starts
			for n, knots := 1, popKnots(); n < len(knots) && starts+knots[n-1] < to; n++ {
				a, b := knots[n-1], min(knots[n], to-starts)
				pop(starts+a, starts+b, popDrawn(float64(a*10)), popDrawn(float64(b*10)))
				end = starts + b
			}
			still(end, to)
		}
	}

	header := assHeader(s, width, height, size, 1, outline, shadow, marginH, marginV)
	return []byte(header + strings.Join(events, "\n") + "\n"), nil
}
