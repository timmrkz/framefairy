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
// ---------------------------------------------------------------------------

const (
	bounceUp   = 80  // ms to grow to the peak
	bounceDown = 100 // ms to settle back
	bounceFrom = 1.0
	bouncePeak = 1.15
	pillFrom   = 0.85
	pillPeak   = 1.15
)

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

			// Phases, shortened in proportion for a very short word.
			span := (to - from) * 10
			up, down := bounceUp, bounceDown
			if span < up+down {
				up = span * bounceUp / (bounceUp + bounceDown)
				down = span - up
			}
			upCS, downCS := up/10, down/10

			// The pill behind the word, bouncing around its own centre.
			w := float64(b.right-b.left) + 2*pillPadX
			h := float64(lineBottom[r]-lineTop[r]) + 2*pillPadY
			pill := roundedRect(0, 0, w, h, math.Min(h*0.3, radius+2))
			add(1, from, to, "Box", fmt.Sprintf(
				`{\an5\pos(%s,%s)\p1\1c%s\1a&H%s&\bord0\shad0\fscx%s\fscy%s\t(0,%d,\fscx%s\fscy%s)\t(%d,%d,\fscx100\fscy100)}%s{\p0}`,
				num(cx), num(cy), s.HighlightColour, alphaOrSolid(s.HighlightAlpha), num(pillFrom*100), num(pillFrom*100),
				up, num(pillPeak*100), num(pillPeak*100), up, up+down, pill))

			// The line with the active word hidden, so nothing else moves.
			add(2, from, to, "Caption", "{"+anchor+"}"+
				taggedText(L.words, L.lines, func(j int) bool { return j != k }, shown))

			// The active word alone, scaled around its centre.
			only := taggedText(L.words, L.lines, func(j int) bool { return j == k }, shown)
			place := func(scale float64) (string, string) {
				return num(ax + (1-scale)*(cx-ax)), num(ay + (1-scale)*(cy-ay))
			}
			x0, y0 := place(bounceFrom)
			x1, y1 := place(bouncePeak)
			peak := num(bouncePeak * 100)
			base := num(bounceFrom * 100)
			if upCS > 0 {
				add(3, from, from+upCS, "Caption", fmt.Sprintf(
					`{\an2\move(%s,%s,%s,%s,0,%d)\fscx%s\fscy%s\t(0,%d,\fscx%s\fscy%s)}%s`,
					x0, y0, x1, y1, upCS*10, base, base, upCS*10, peak, peak, only))
			}
			if downCS > 0 {
				add(3, from+upCS, from+upCS+downCS, "Caption", fmt.Sprintf(
					`{\an2\move(%s,%s,%s,%s,0,%d)\fscx%s\fscy%s\t(0,%d,\fscx%s\fscy%s)}%s`,
					x1, y1, x0, y0, downCS*10, peak, peak, downCS*10, base, base, only))
			}
			add(3, from+upCS+downCS, to, "Caption", "{"+anchor+"}"+only)
		}
	}

	header := assHeader(s, width, height, size, 1, outline, shadow, marginH, marginV)
	return true, os.WriteFile(path, []byte(header+strings.Join(events, "\n")+"\n"), 0o644)
}
