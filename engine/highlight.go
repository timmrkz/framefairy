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
// time within each phase, which keeps them in step frame by frame.
//
// How far the bounce has come at a moment is BounceAt, and the subtitle
// file is written from its phases.
//
// The bounce reaches out by a few pixels, the same for every word, never
// by a share of the word. At rest the pill already takes nearly the whole
// space between two words, 0.14 of the size against a space of 0.165 in
// Inter Black, so it grows above and below and never past its own sides,
// and the word grows inside its pill. A share of the word carried a long
// word into the next: "respektvoll" grew 34 pixels a side and its pill
// covered "ist", which Tim saw on a thumbnail.
//
// The pill is never smaller than at rest, and at rest it leaves room all
// round its word: as tall as the line, from the top of the tallest letter
// the face draws to the bottom of the deepest, with pillPadY over that,
// the same for every word of the line whatever its letters. It once
// opened from inside its rest on every side, so the first frame of a word,
// which is where a click on the word puts the playhead, showed the pill
// hugging the letters, and at rest it reached only pillPadY past the
// tallest letter of the words themselves. Tim saw both on #201. Where
// another line of the caption stands over or under it, the pills of its
// line stop short of that line's letters, all at one height, and bounce
// into the room there is, see wordsLaid.lineBand and wordsLaid.bounce.
// ---------------------------------------------------------------------------

const (
	bounceUp   = 80  // ms to grow to the peak
	bounceDown = 100 // ms to settle back
	// bounceGrow is how far the pill reaches above and below where it
	// rests at the peak, as a share of the caption size.
	bounceGrow = 0.16
	// wordRoom is how far the word may grow out on either side, as a share
	// of the caption size: a little of the pill's room at its sides, so the
	// word keeps room in its pill at the top of the bounce too. A short
	// word stops at wordPeak.
	wordRoom = 0.03
	wordPeak = 1.15
	// pillPadX is the room the pill leaves at the sides of its word, and
	// pillPadY above and below its line, as shares of the caption size.
	pillPadX = 0.14
	pillPadY = 0.10
	// lineClear is the least room the pill leaves to a letter of another
	// line of its caption, at rest and at the top of its bounce, as a
	// share of the caption size, see wordsLaid.lineBand.
	lineClear = 0.04
	// lineReach is the tallest and the deepest letters a line can hold,
	// whose ink says how tall the line is, see pillLines.
	lineReach = "ÉÅÁgjpqy"
)

// Bounce is where the bounce of the word being spoken stands. Word runs
// from 0 to 1 and back to 0, the word's share of its growth. Tall runs
// from 0 to 1 and back to 0, how far the pill reaches above and below its
// rest, in bounceGrow. Its sides never move.
type Bounce struct {
	Word float64 `json:"word"`
	Tall float64 `json:"tall"`
}

// bouncePhases is how long a word lit for span hundredths of a second
// grows and settles back, in hundredths, the subtitle file's clock. A
// word too short for both gets each in proportion.
func bouncePhases(span int) (up, down int) {
	ms := span * 10
	u, d := bounceUp, bounceDown
	if ms < u+d {
		u = ms * bounceUp / (bounceUp + bounceDown)
		d = ms - u
	}
	return u / 10, d / 10
}

// BounceAt is the bounce of a word since seconds after it lit, lit for
// span hundredths of a second. Within each phase it is linear in time,
// which is how libass moves from one value to the next.
func BounceAt(since float64, span int) Bounce {
	up, down := bouncePhases(span)
	ms := math.Max(since, 0) * 1000
	upMS, downMS := float64(up*10), float64(down*10)
	switch {
	case ms < upMS:
		f := ms / upMS
		return Bounce{Word: f, Tall: f}
	case ms < upMS+downMS:
		f := (ms - upMS) / downMS
		return Bounce{Word: 1 - f, Tall: 1 - f}
	}
	return Bounce{}
}

// WordPeak is how large a word grows at the top of its bounce: as far as
// wordRoom lets it, measured from the face the way the lines are, and no
// more than wordPeak.
func WordPeak(s Style, text string) float64 {
	width := roomFor(s).width(text)
	if width <= 0 {
		return wordPeak
	}
	return math.Min(wordPeak, 1+2*wordRoom*s.Size/width)
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

// highlighted is the caption track with the bouncing highlight. It is
// nothing, without an error, when the captions cannot be measured, and the
// plain track is made instead.
func (e *Engine) highlighted(ctx context.Context, captions []LaidCaption,
	width, height int, s Style) ([]byte, error) {
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
		all := func(int) bool { return true }
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
			// The pill at rest, and the word's centre: the middle of its ink
			// across and of its pill down.
			pl, pt, pr, pb := L.pill(k, size)
			cx := ax + (pl+pr)/2
			cy := ay + (pt+pb)/2

			upCS, downCS := bouncePhases(to - from)
			up, down := L.bounce(k, size)
			peakScale := WordPeak(s, c.Words[k].Text)

			// The pill behind the word, bouncing: each phase from where
			// BounceAt has it at the phase's start to where it has it at its
			// end. It grows by up above and down below, so where one side has
			// less room its centre moves with the growth.
			w, h := pr-pl, pb-pt
			pill := roundedRect(0, 0, w, h, pillRadius(h, radius))
			pillAt := func(at Bounce) (y, scale string) {
				return num(cy + (down-up)/2*at.Tall), num((h + (up+down)*at.Tall) / h * 100)
			}
			pillEvent := func(from, to int, a, z Bounce) {
				fromY, fromScale := pillAt(a)
				toY, toScale := pillAt(z)
				add(1, from, to, "Box", fmt.Sprintf(
					`{\an5\move(%s,%s,%s,%s,0,%d)\p1\1c%s\1a&H%s&\bord0\shad0\fscx100\fscy%s\t(0,%d,\fscx100\fscy%s)}%s{\p0}`,
					num(cx), fromY, num(cx), toY, (to-from)*10, s.HighlightColour, alphaOrSolid(s.HighlightAlpha),
					fromScale, (to-from)*10, toScale, pill))
			}
			opening, peak, rest := Bounce{}, Bounce{Word: 1, Tall: 1}, Bounce{}
			if upCS > 0 {
				pillEvent(from, from+upCS, opening, peak)
			}
			if downCS > 0 {
				pillEvent(from+upCS, from+upCS+downCS, peak, rest)
			}
			pillEvent(from+upCS+downCS, to, rest, rest)

			// The line with the active word hidden, so nothing else moves.
			add(2, from, to, "Caption", "{"+anchor+"}"+
				taggedText(L.words, L.lines, func(j int) bool { return j != k }, shown))

			// The active word alone, scaled around its centre.
			only := taggedText(L.words, L.lines, func(j int) bool { return j == k }, shown)
			place := func(scale float64) (string, string) {
				return num(ax + (1-scale)*(cx-ax)), num(ay + (1-scale)*(cy-ay))
			}
			x0, y0 := place(1)
			x1, y1 := place(peakScale)
			top := num(peakScale * 100)
			if upCS > 0 {
				add(3, from, from+upCS, "Caption", fmt.Sprintf(
					`{\an2\move(%s,%s,%s,%s,0,%d)\fscx100\fscy100\t(0,%d,\fscx%s\fscy%s)}%s`,
					x0, y0, x1, y1, upCS*10, upCS*10, top, top, only))
			}
			if downCS > 0 {
				add(3, from+upCS, from+upCS+downCS, "Caption", fmt.Sprintf(
					`{\an2\move(%s,%s,%s,%s,0,%d)\fscx%s\fscy%s\t(0,%d,\fscx100\fscy100)}%s`,
					x1, y1, x0, y0, downCS*10, top, top, downCS*10, only))
			}
			add(3, from+upCS+downCS, to, "Caption", "{"+anchor+"}"+only)
		}
	}

	header := assHeader(s, width, height, size, 1, outline, shadow, marginH, marginV)
	return []byte(header + strings.Join(events, "\n") + "\n"), nil
}
