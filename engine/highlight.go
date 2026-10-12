package engine

import (
	"context"
	"fmt"
	"math"
	"os"
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
// ---------------------------------------------------------------------------

const (
	bounceUp   = 80  // ms to grow to the peak
	bounceDown = 100 // ms to settle back
	// bounceGrow is how far the pill reaches above and below where it
	// rests at the peak, and how far inside it starts on every side, as a
	// share of the caption size.
	bounceGrow = 0.08
	// wordRoom is how far the word may grow out on either side, as a share
	// of the caption size: half the pill's padding at the sides, so a word
	// stays inside its pill. A short word stops at wordPeak.
	wordRoom = 0.07
	wordPeak = 1.15
)

// Bounce is where the bounce of the word being spoken stands. Word runs
// from 0 to 1 and back to 0, the word's share of its growth. Tall runs
// from -1 to 1 and back to 0, how far the pill reaches above and below its
// rest, in bounceGrow. Wide runs from -1 to 0 and stays there, the same at
// its sides, so the pill opens to its rest and goes no further.
type Bounce struct {
	Word float64 `json:"word"`
	Wide float64 `json:"wide"`
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
		return Bounce{Word: f, Wide: f - 1, Tall: 2*f - 1}
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

// writeHighlighted writes the caption track with the bouncing highlight. It
// reports false, without an error, when the captions cannot be measured, and
// the plain track is written instead.
func (e *Engine) writeHighlighted(ctx context.Context, captions []LaidCaption, path string,
	width, height int, s Style) (bool, error) {
	shown := shownTag(s.Primary)
	scale := float64(height) / 1920.0
	size := max(12, pyround(s.Size*scale))
	outline := max(1, pyround(s.Outline*scale))
	shadow := max(0, pyround(s.Shadow*scale))
	marginH := pyround(s.MarginH * scale)
	marginV := pyround(s.MarginV * scale)

	type laid struct {
		words []string
		lines [][]int
		boxes []*inkBox
	}
	layouts := make([]laid, len(captions))
	var blocks []string
	for i, c := range captions {
		if len(c.Words) == 0 {
			e.Log.Detail("a caption has no word timings, so the highlight is off for this clip")
			return false, nil
		}
		words := make([]string, len(c.Words))
		for k, w := range c.Words {
			words[k] = EscapeASSText(w.Text)
		}
		lines := lineIndices(c.Lines)
		layouts[i] = laid{words: words, lines: lines}
		for k := range words {
			k := k
			// Measured opaque, whatever the text colour: a word is found by
			// its ink, and a see-through one would leave little to find.
			blocks = append(blocks, taggedText(words, lines, func(j int) bool { return j == k }, shownTag("")))
		}
	}
	boxes, ok := e.measureMany(ctx, blocks, s, width, size)
	if !ok {
		e.Log.Detail("could not measure the caption words, the highlight is off for this clip")
		return false, nil
	}
	at := 0
	for i := range layouts {
		n := len(layouts[i].words)
		layouts[i].boxes = boxes[at : at+n]
		at += n
		for _, b := range layouts[i].boxes {
			if b == nil {
				e.Log.Detail("a caption word could not be measured, the highlight is off for this clip")
				return false, nil
			}
		}
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
	pillPadX := math.Round(float64(size) * 0.14)
	pillPadY := math.Round(float64(size) * 0.10)
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
		// Extent of each line, and of the whole caption.
		lineTop := make([]int, len(L.lines))
		lineBottom := make([]int, len(L.lines))
		lineOf := make([]int, len(L.words))
		full := inkBox{math.MaxInt32, math.MaxInt32, math.MinInt32, math.MinInt32}
		for r, line := range L.lines {
			lineTop[r], lineBottom[r] = math.MaxInt32, math.MinInt32
			for _, k := range line {
				b := L.boxes[k]
				lineOf[k] = r
				lineTop[r] = min(lineTop[r], b.top)
				lineBottom[r] = max(lineBottom[r], b.bottom)
				full.left, full.right = min(full.left, b.left), max(full.right, b.right)
				full.top, full.bottom = min(full.top, b.top), max(full.bottom, b.bottom)
			}
		}
		if drawBox {
			shape := roundedRect(ax+float64(full.left)-padX, ay+float64(full.top)-padY,
				ax+float64(full.right)+padX, ay+float64(full.bottom)+padY, radius)
			add(0, start, end, "Box", fmt.Sprintf(
				`{\p1\pos(0,0)\1c%s\1a&H%s&\bord0\shad0}%s{\p0}`, boxFill, boxAlpha, shape))
		}

		// When each word is the active one. A word stays active until the
		// next one starts, so a short word never flickers.
		begins := make([]int, len(L.words))
		for k, w := range c.Words {
			begins[k] = min(max(centiseconds(w.Start), start), end)
		}
		for k := 1; k < len(begins); k++ {
			begins[k] = max(begins[k], begins[k-1])
		}
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
			b := L.boxes[k]
			r := lineOf[k]
			// The word's centre: the middle of its ink across, the middle of
			// its line down, so every word on a line bounces from one height.
			cx := ax + float64(b.left+b.right)/2
			cy := ay + float64(lineTop[r]+lineBottom[r])/2

			upCS, downCS := bouncePhases(to - from)
			grow := bounceGrow * float64(size)
			peakScale := WordPeak(s, c.Words[k].Text)

			// The pill behind the word, bouncing around its own centre: each
			// phase from where BounceAt has it at the phase's start to where
			// it has it at its end.
			w := float64(b.right-b.left) + 2*pillPadX
			h := float64(lineBottom[r]-lineTop[r]) + 2*pillPadY
			pill := roundedRect(0, 0, w, h, math.Min(h*0.3, radius+2))
			pillScale := func(at Bounce) (string, string) {
				return num((w + 2*grow*at.Wide) / w * 100), num((h + 2*grow*at.Tall) / h * 100)
			}
			pillEvent := func(from, to int, a, z Bounce) {
				fromX, fromY := pillScale(a)
				toX, toY := pillScale(z)
				add(1, from, to, "Box", fmt.Sprintf(
					`{\an5\pos(%s,%s)\p1\1c%s\1a&H%s&\bord0\shad0\fscx%s\fscy%s\t(0,%d,\fscx%s\fscy%s)}%s{\p0}`,
					num(cx), num(cy), s.HighlightColour, alphaOrSolid(s.HighlightAlpha), fromX, fromY,
					(to-from)*10, toX, toY, pill))
			}
			opening, peak, rest := BounceAt(0, to-from), Bounce{Word: 1, Tall: 1}, Bounce{}
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
	return true, os.WriteFile(path, []byte(header+strings.Join(events, "\n")+"\n"), 0o644)
}
