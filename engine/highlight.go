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

func captionCues(captions []Caption) []Cue {
	out := make([]Cue, len(captions))
	for i, c := range captions {
		out[i] = c.Cue()
	}
	return out
}

// The words of every caption are kept next to its srt file, so that a hand
// edit to the text does not lose the timing of the words it left alone.

const wordsFileVersion = 1

type wordsFile struct {
	Version  int            `json:"version"`
	Captions []wordsCaption `json:"captions"`
}

type wordsCaption struct {
	Start PyFloat  `json:"start"`
	End   PyFloat  `json:"end"`
	Text  string   `json:"text"`
	Words [][3]any `json:"words"`
}

// WordsPath is the words file that belongs to an srt file.
func WordsPath(srtPath string) string {
	return strings.TrimSuffix(srtPath, ".srt") + ".words.json"
}

// WriteCaptions writes a clip's captions as srt, plus their words.
func WriteCaptions(captions []Caption, srtPath string) error {
	file := wordsFile{Version: wordsFileVersion, Captions: []wordsCaption{}}
	for _, c := range captions {
		entry := wordsCaption{Start: PyFloat(roundTo(c.Start, 3)), End: PyFloat(roundTo(c.End, 3)),
			Text: c.Text, Words: [][3]any{}}
		for _, w := range c.Words {
			entry.Words = append(entry.Words, [3]any{PyFloat(roundTo(w.Start, 3)),
				PyFloat(roundTo(w.End, 3)), w.Text})
		}
		file.Captions = append(file.Captions, entry)
	}
	body, err := MarshalPlan(file)
	if err != nil {
		return err
	}
	if err := WriteSRT(captionCues(captions), srtPath); err != nil {
		return err
	}
	return os.WriteFile(WordsPath(srtPath), body, 0o644)
}

func readWordsFile(path string) []Caption {
	data, err := os.ReadFile(path)
	if err != nil {
		return nil
	}
	var file wordsFile
	if decodeJSON(data, &file) != nil || file.Version != wordsFileVersion {
		return nil
	}
	var out []Caption
	for _, entry := range file.Captions {
		c := Caption{Start: float64(entry.Start), End: float64(entry.End), Text: entry.Text}
		for _, item := range entry.Words {
			low, ok1 := toFloat(item[0])
			high, ok2 := toFloat(item[1])
			text, ok3 := item[2].(string)
			if ok1 && ok2 && ok3 {
				c.Words = append(c.Words, Cue{low, high, text})
			}
		}
		out = append(out, c)
	}
	return out
}

// LoadCaptions reads a clip's srt file, which you may have edited, and gives
// every caption its words back.
//
// Each caption is matched to the saved caption it overlaps most in time.
// Words you left alone keep their timing. Words you changed or added share
// the time between the unchanged words around them, in proportion to their
// length. Without a saved words file the words share the caption's time.
func LoadCaptions(srtPath string) ([]Caption, error) {
	cues, err := LoadSRT(srtPath)
	if err != nil {
		return nil, err
	}
	saved := readWordsFile(WordsPath(srtPath))
	out := make([]Caption, len(cues))
	for i, cue := range cues {
		var original []Cue
		best := 0.0
		for _, s := range saved {
			overlap := min(cue.End, s.End) - max(cue.Start, s.Start)
			if overlap > best {
				best, original = overlap, s.Words
			}
		}
		out[i] = Caption{Start: cue.Start, End: cue.End, Text: cue.Text,
			Words: AlignWords(cue.Text, cue.Start, cue.End, original)}
	}
	return out, nil
}

func normWord(s string) string {
	return strings.ToLower(strings.Trim(s, ".,!?;:…-–—\"'„“”»«()"))
}

// AlignWords times the words of a caption's text, reusing the timing of
// original words wherever the text still contains them.
func AlignWords(text string, start, end float64, original []Cue) []Cue {
	tokens := fields(text)
	if len(tokens) == 0 {
		return nil
	}
	// Longest common subsequence on normalised words.
	n, m := len(tokens), len(original)
	table := make([][]int, n+1)
	for i := range table {
		table[i] = make([]int, m+1)
	}
	for i := n - 1; i >= 0; i-- {
		for j := m - 1; j >= 0; j-- {
			if normWord(tokens[i]) == normWord(original[j].Text) {
				table[i][j] = table[i+1][j+1] + 1
			} else {
				table[i][j] = max(table[i+1][j], table[i][j+1])
			}
		}
	}
	match := make([]int, n)
	for i := range match {
		match[i] = -1
	}
	for i, j := 0, 0; i < n && j < m; {
		switch {
		case normWord(tokens[i]) == normWord(original[j].Text):
			match[i] = j
			i++
			j++
		case table[i+1][j] >= table[i][j+1]:
			i++
		default:
			j++
		}
	}

	clamp := func(t float64) float64 { return max(start, min(t, end)) }
	out := make([]Cue, n)
	for i := 0; i < n; {
		if match[i] >= 0 {
			w := original[match[i]]
			out[i] = Cue{clamp(w.Start), clamp(w.End), tokens[i]}
			i++
			continue
		}
		// A run of changed words, spread over the gap it sits in.
		k := i
		for k < n && match[k] < 0 {
			k++
		}
		low, high := start, end
		if len(original) > 0 {
			low, high = clamp(original[0].Start), clamp(original[len(original)-1].End)
		}
		if i > 0 {
			low = out[i-1].End
		}
		if k < n {
			high = clamp(original[match[k]].Start)
		}
		if high-low < 0.05*float64(k-i) {
			high = min(end, low+0.05*float64(k-i))
		}
		total := 0
		for _, t := range tokens[i:k] {
			total += max(runeLen(t), 1)
		}
		at := low
		for j := i; j < k; j++ {
			share := (high - low) * float64(max(runeLen(tokens[j]), 1)) / float64(total)
			out[j] = Cue{at, at + share, tokens[j]}
			at += share
		}
		i = k
	}
	for i := 1; i < n; i++ {
		if out[i].Start < out[i-1].Start {
			out[i].Start = out[i-1].Start
		}
		out[i].End = max(out[i].End, out[i].Start)
	}
	return out
}

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

// captionLayout splits a caption's words into lines the same way the plain
// caption wraps, as lists of word indices.
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

func captionLayout(words []string, limit int) [][]int {
	return wrapWords(words, func(text string) bool { return runeLen(text) <= limit })
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

const (
	shown  = `{\alpha&H00&}`
	hidden = `{\alpha&HFF&}`
)

// taggedText is the caption with each word shown or hidden. Hidden words
// still take their place in the line.
func taggedText(words []string, lines [][]int, visible func(int) bool) string {
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
			blocks = append(blocks, taggedText(words, lines, func(j int) bool { return j == k }))
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
	drawBox := s.BorderStyle == 3 || s.BorderStyle == 4
	boxAlpha := "80"
	if len(s.BackColour) >= 10 {
		boxAlpha = s.BackColour[2:4]
	}
	boxFill := "&H" + s.BackColour[max(0, len(s.BackColour)-6):] + "&"
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
		add(2, start, begins[0], "Caption", "{"+anchor+"}"+taggedText(L.words, L.lines, all))

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
				`{\an5\pos(%s,%s)\p1\1c%s\1a&H00&\bord0\shad0\fscx%s\fscy%s\t(0,%d,\fscx%s\fscy%s)\t(%d,%d,\fscx100\fscy100)}%s{\p0}`,
				num(cx), num(cy), s.HighlightColour, num(pillFrom*100), num(pillFrom*100),
				up, num(pillPeak*100), num(pillPeak*100), up, up+down, pill))

			// The line with the active word hidden, so nothing else moves.
			add(2, from, to, "Caption", "{"+anchor+"}"+
				taggedText(L.words, L.lines, func(j int) bool { return j != k }))

			// The active word alone, scaled around its centre.
			only := taggedText(L.words, L.lines, func(j int) bool { return j == k })
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
