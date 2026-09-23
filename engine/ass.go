package engine

import (
	"context"
	"fmt"
	"math"
	"os"
	"path/filepath"
	"strings"
)

// Style is the resolved caption look. Values are authored against a
// 1080x1920 frame and scaled when the output size differs, so a preview
// render looks like the final one.
type Style struct {
	Font          string
	Size          float64
	Primary       string
	OutlineColour string
	BackColour    string
	BorderStyle   int
	Outline       float64
	Shadow        float64
	Bold          float64
	MarginH       float64
	MarginV       float64
	MaxChars      float64
	WrapChars     int
	Pad           int
	Radius        float64
	BoxPadX       float64
	BoxPadY       float64
	// Highlight marks the word being spoken with a coloured pill.
	Highlight bool
	// HighlightColour is the pill colour as an ASS colour tag value.
	HighlightColour string
	// HighlightAlpha is how see-through the pill is, as the two hex digits
	// an ASS alpha tag takes. 00 is solid.
	HighlightAlpha string
}

// DefaultStyle is the caption look used unless a plan or a flag says
// otherwise.
var DefaultStyle = map[string]any{
	"font": DefaultFont,
	"size": 96.0,
	// White. ASS colours are &HAABBGGRR, and the alpha runs backwards, 00 is
	// opaque and FF is clear.
	"primary":        "&H00FFFFFF",
	"outline_colour": "&H80000000",
	// Half transparent black, the box fill.
	"back_colour": "&H80000000",
	// BorderStyle 4 draws one box behind the whole caption rather than a
	// separate one per line, which is what reads as a caption pill. Outline
	// doubles as glyph halo and box padding in that mode, so it stays at zero
	// and the padding comes from hard spaces around the text instead.
	"border_style": 4.0,
	"outline":      0.0,
	"shadow":       0.0,
	"bold":         1.0,
	"margin_h":     70.0,
	// Distance from the bottom of the frame. Shorts and Reels park their own
	// UI over roughly the bottom fifth, so captions sit above it.
	"margin_v": 300.0,
	// Characters in one caption, across at most two lines. Long enough to
	// hold a phrase, short enough that the text can be big.
	"max_chars":  38.0,
	"wrap_chars": 20.0,
	"pad":        2.0,
	// Corner radius of the caption box, in pixels of a 1080x1920 frame. Zero
	// falls back to the square box that libass draws on its own.
	"radius":    18.0,
	"box_pad_x": 26.0,
	"box_pad_y": 26.0,
	// The word being spoken sits on a pill in this colour and bounces.
	"highlight":        1.0,
	"highlight_colour": "#942192",
}

// Caption text is untrusted. It comes from YouTube's ASR, which transcribes
// whatever a guest happened to say. In ASS a brace opens an override block
// and a backslash starts a tag, so both are neutralised before the text
// reaches a Dialogue line. Otherwise a caption containing braces silently
// restyles or repositions itself.
var assText = strings.NewReplacer("{", "(", "}", ")", "\\", "/", "\n", " ", "\r", " ")

// EscapeASSText neutralises override syntax in caption text.
func EscapeASSText(text string) string { return assText.Replace(text) }

// Style fields sit in a comma-separated header line, so a comma or newline in
// a font name would shift every field after it or append arbitrary lines.
var assField = strings.NewReplacer(",", " ", "{", "", "}", "", "\n", " ", "\r", " ", ":", " ")

func cleanField(value any) string {
	return runePrefix(strip(assField.Replace(pyStr(value))), 64)
}

func cleanNumber(value any, fallback float64) float64 {
	number, ok := toFloat(value)
	if !ok || !isFinite(number) {
		return fallback
	}
	return number
}

// highlightColour accepts #RRGGBB, RRGGBB or an ASS colour and returns the
// value an ASS colour tag takes.
func highlightColour(value any, fallback string) string {
	text := strings.ToUpper(strip(pyStr(value)))
	var rgb string
	switch {
	case strings.HasPrefix(text, "#"):
		rgb = text[1:]
	case strings.HasPrefix(text, "&H"):
		raw := strings.TrimSuffix(text[2:], "&")
		if len(raw) < 6 {
			return fallback
		}
		bgr := raw[len(raw)-6:]
		rgb = bgr[4:6] + bgr[2:4] + bgr[0:2]
	default:
		rgb = text
	}
	if len(rgb) != 6 {
		return fallback
	}
	for _, r := range rgb {
		if !strings.ContainsRune("0123456789ABCDEF", r) {
			return fallback
		}
	}
	return "&H" + rgb[4:6] + rgb[2:4] + rgb[0:2] + "&"
}

// LooksLikeColour says whether a value is one the engine and the app can
// both use: #RRGGBB, RRGGBB or an ASS colour. The app asks before keeping a
// colour a person typed or a settings file carried.
func LooksLikeColour(value string) bool {
	return highlightColour(value, "") != ""
}

// AssColour turns a colour picked in the app, #RRGGBB, and how opaque it
// is, from 0 to 1, into the &HAABBGGRR the render takes. ASS counts the
// alpha backwards: 00 is opaque and FF is clear.
func AssColour(hex string, opacity float64) (string, bool) {
	if !strings.HasPrefix(hex, "#") || len(hex) != 7 || !isHex(hex[1:]) || !isFinite(opacity) {
		return "", false
	}
	clear := int(math.Round((1 - math.Max(0, math.Min(opacity, 1))) * 255))
	rgb := strings.ToUpper(hex[1:])
	return fmt.Sprintf("&H%02X%s%s%s", clear, rgb[4:6], rgb[2:4], rgb[0:2]), true
}

// isAssColour says whether a value is a whole &HAABBGGRR colour.
func isAssColour(value string) bool {
	return len(value) == 10 && strings.HasPrefix(value, "&H") && isHex(value[2:])
}

func isHex(text string) bool {
	for _, r := range text {
		if !strings.ContainsRune("0123456789abcdefABCDEF", r) {
			return false
		}
	}
	return text != ""
}

func cleanColour(value any, fallback string) string {
	text := strip(pyStr(value))
	if text == "" || runeLen(text) > 12 {
		return fallback
	}
	for _, r := range text {
		if !strings.ContainsRune("0123456789abcdefABCDEF&Hh", r) {
			return fallback
		}
	}
	return text
}

// ResolveStyle merges overrides onto the defaults and validates every value
// by type. Unknown keys are ignored and colours must look like colours.
func ResolveStyle(overrides map[string]any) Style {
	s := map[string]any{}
	for k, v := range DefaultStyle {
		s[k] = v
	}
	for k, v := range overrides {
		if _, known := DefaultStyle[k]; known && v != nil {
			s[k] = v
		}
	}
	number := func(key string) float64 {
		return cleanNumber(s[key], DefaultStyle[key].(float64))
	}
	font := cleanField(s["font"])
	if font == "" {
		font = DefaultStyle["font"].(string)
	}
	border := int(number("border_style"))
	if border != 1 && border != 3 && border != 4 {
		border = 4
	}
	return Style{
		Font:          font,
		Size:          math.Max(8, math.Min(number("size"), 400)),
		Primary:       cleanColour(s["primary"], DefaultStyle["primary"].(string)),
		OutlineColour: cleanColour(s["outline_colour"], DefaultStyle["outline_colour"].(string)),
		BackColour:    cleanColour(s["back_colour"], DefaultStyle["back_colour"].(string)),
		BorderStyle:   border,
		Outline:       number("outline"),
		Shadow:        number("shadow"),
		Bold:          number("bold"),
		MarginH:       number("margin_h"),
		MarginV:       number("margin_v"),
		MaxChars:      number("max_chars"),
		WrapChars:     max(8, min(int(number("wrap_chars")), 200)),
		Pad:           max(0, min(int(number("pad")), 8)),
		Radius:        number("radius"),
		BoxPadX:       number("box_pad_x"),
		BoxPadY:       number("box_pad_y"),
		Highlight:     number("highlight") != 0,
		HighlightColour: highlightColour(s["highlight_colour"],
			highlightColour(DefaultStyle["highlight_colour"], "&H6F23B4&")),
		HighlightAlpha: highlightAlpha(s["highlight_colour"]),
	}
}

// highlightAlpha is the alpha of a highlight colour given as a whole
// &HAABBGGRR, which is how the app keeps one with an opacity. A colour
// given as #RRGGBB, or one that is no colour at all, is solid.
func highlightAlpha(value any) string {
	text := strings.ToUpper(strip(pyStr(value)))
	if !strings.HasPrefix(text, "&H") || highlightColour(text, "") == "" {
		return "00"
	}
	return alphaOf(text)
}

// HighlightWeb is the highlight colour with its opacity, as the video
// preview draws it.
func (s Style) HighlightWeb() string {
	bgr := strings.TrimSuffix(strings.TrimPrefix(s.HighlightColour, "&H"), "&")
	alpha := s.HighlightAlpha
	if alpha == "" {
		alpha = "00"
	}
	return WebColour("&H" + alpha + bgr)
}

// Where the captions may sit, as the distance from the bottom of a
// 1080x1920 frame. A drag in the app lands on one of these steps, which
// keeps the captions of an episode in line with each other.
const (
	CaptionYStep = 40
	CaptionYMin  = 120
	CaptionYMax  = 1560
)

// DefaultCaptionY is where the captions sit when nobody has placed them.
var DefaultCaptionY = DefaultStyle["margin_v"].(float64)

// SnapCaptionY puts a hand-placed caption line on the nearest step and
// inside the frame.
func SnapCaptionY(y float64) float64 {
	if !isFinite(y) {
		return DefaultStyle["margin_v"].(float64)
	}
	y = math.Round(y/CaptionYStep) * CaptionYStep
	return math.Max(CaptionYMin, math.Min(y, CaptionYMax))
}

func wrapText(text string, limit, pad int) string {
	var lines []string
	current := ""
	for _, word := range fields(text) {
		candidate := strip(current + " " + word)
		if runeLen(candidate) > limit && current != "" {
			lines = append(lines, current)
			current = word
		} else {
			current = candidate
		}
	}
	if current != "" {
		lines = append(lines, current)
	}
	if pad > 0 {
		spaces := strings.Repeat(`\h`, pad)
		for i, line := range lines {
			lines[i] = spaces + line + spaces
		}
	}
	return strings.Join(lines, `\N`)
}

// Caption measures are authored against a frame this wide. Everything is
// scaled from there when the output size differs.
const captionFrameW = 1080.0

// CaptionRoom is how wide a caption line may be, in pixels of a 1080x1920
// frame: the frame less the margins at the sides and the padding inside the
// box.
func CaptionRoom(s Style) float64 {
	return math.Max(captionFrameW-2*s.MarginH-2*s.BoxPadX, 80)
}

// LaidCaption is a caption with the lines it is drawn in.
type LaidCaption struct {
	Caption
	Lines [][]Cue
}

// LayOutCaptions breaks captions into the lines they are drawn in and gives
// back the style with a size they all fit at.
//
// A line is broken where the text would otherwise run wider than the frame,
// which is what keeps a caption inside its box whatever face and size are
// chosen. When even a single word is too wide for the frame, the size comes
// down until it fits, for the whole clip, so the captions of one short stay
// one size. A face the program does not carry cannot be measured, and then
// the style's character count decides the breaks, as it always did.
func LayOutCaptions(captions []Caption, s Style) ([]LaidCaption, Style) {
	room := CaptionRoom(s)
	size := s.Size
	laid := make([]LaidCaption, len(captions))
	for round := 0; round < 4; round++ {
		widest := 0.0
		for i, c := range captions {
			lines := captionLines(c, s.Font, size, room, s.WrapChars)
			laid[i] = LaidCaption{Caption: c, Lines: lines}
			for _, line := range lines {
				if width, ok := TextWidth(s.Font, cueText(line), size); ok {
					widest = math.Max(widest, width)
				}
			}
		}
		if widest <= room {
			break
		}
		next := math.Max(12, math.Floor(size*room/widest))
		if next >= size {
			break
		}
		size = next
	}
	s.Size = size
	return laid, s
}

// CaptionLines splits one caption into the lines the render draws it in.
func CaptionLines(c Caption, s Style) [][]Cue {
	return captionLines(c, s.Font, s.Size, CaptionRoom(s), s.WrapChars)
}

func captionLines(c Caption, font string, size, room float64, wrapChars int) [][]Cue {
	fits := func(text string) bool {
		if width, ok := TextWidth(font, text, size); ok {
			return width <= room
		}
		return runeLen(text) <= wrapChars
	}
	if len(c.Words) == 0 {
		// A caption that came back from a file without word timings.
		words := fields(c.Text)
		var out [][]Cue
		for _, line := range wrapWords(words, fits) {
			parts := make([]string, len(line))
			for i, at := range line {
				parts[i] = words[at]
			}
			out = append(out, []Cue{{c.Start, c.End, strings.Join(parts, " ")}})
		}
		return out
	}
	words := make([]string, len(c.Words))
	for i, w := range c.Words {
		words[i] = w.Text
	}
	var out [][]Cue
	for _, line := range wrapWords(words, fits) {
		row := make([]Cue, 0, len(line))
		for _, at := range line {
			row = append(row, c.Words[at])
		}
		out = append(out, row)
	}
	return out
}

// wrapWords groups words into lines, each line as long as it may be.
func wrapWords(words []string, fits func(string) bool) [][]int {
	var lines [][]int
	var current []int
	text := ""
	for i, w := range words {
		candidate := strip(text + " " + w)
		if len(current) > 0 && !fits(candidate) {
			lines = append(lines, current)
			current, text = []int{i}, w
			continue
		}
		current = append(current, i)
		text = candidate
	}
	if len(current) > 0 {
		lines = append(lines, current)
	}
	return lines
}

func cueText(line []Cue) string {
	parts := make([]string, len(line))
	for i, c := range line {
		parts[i] = c.Text
	}
	return strings.Join(parts, " ")
}

// assText is the lines of a caption as one ASS text, with the hard spaces
// that pad a caption when no box is drawn.
func (l LaidCaption) assText(pad int) string {
	spaces := strings.Repeat(`\h`, pad)
	rows := make([]string, len(l.Lines))
	for i, line := range l.Lines {
		rows[i] = spaces + EscapeASSText(cueText(line)) + spaces
	}
	return strings.Join(rows, `\N`)
}

type inkBox struct{ left, top, right, bottom int }

// measureASS renders the caption text once and reads back where the pixels
// actually are.
//
// libass has no rounded box, so the box has to be drawn by hand, and drawing
// it means knowing how wide the text comes out. The text is rendered to one
// tall frame with each caption in its own band and the bounding boxes are
// read straight off the pixels. Exact for any font, any language, any weight,
// and it costs one frame per clip.
func (e *Engine) measureASS(ctx context.Context, blocks []string, s Style,
	width, size int) ([]*inkBox, bool) {
	if len(blocks) == 0 {
		return []*inkBox{}, true
	}
	// Each caption gets a band sized for its own line count, so a caption
	// that needs three lines is measured whole. Heights are even because an
	// odd frame height gets rounded down by ffmpeg.
	slots := make([]int, len(blocks))
	offsets := make([]int, len(blocks))
	running := 0
	for i, block := range blocks {
		slot := bandSlot(block, size)
		slots[i] = slot
		offsets[i] = running
		running += slot
	}
	height := running
	if height > 16000 {
		return nil, false
	}

	lines := []string{
		"[Script Info]", "ScriptType: v4.00+",
		fmt.Sprintf("PlayResX: %d", width), fmt.Sprintf("PlayResY: %d", height),
		"WrapStyle: 2", "ScaledBorderAndShadow: yes", "",
		"[V4+ Styles]",
		"Format: Name, Fontname, Fontsize, PrimaryColour, SecondaryColour, " +
			"OutlineColour, BackColour, Bold, Italic, Underline, StrikeOut, " +
			"ScaleX, ScaleY, Spacing, Angle, BorderStyle, Outline, Shadow, " +
			"Alignment, MarginL, MarginR, MarginV, Encoding",
		fmt.Sprintf("Style: M,%s,%d,&H00FFFFFF,&H00FFFFFF,&H00FFFFFF,"+
			"&H00FFFFFF,%d,0,0,0,100,100,0,0,1,0,0,2,0,0,0,1", s.Font, size, int(s.Bold)),
		"", "[Events]",
		"Format: Layer, Start, End, Style, Name, MarginL, MarginR, MarginV, " +
			"Effect, Text",
	}
	// Alignment 2 and an explicit anchor, exactly as the real caption is
	// placed. The ink offset from the anchor is what carries over to the
	// render, and it only transfers if both use the same anchor.
	anchors := make([]int, len(blocks))
	for i := range blocks {
		anchors[i] = offsets[i] + int(float64(slots[i])*0.76)
	}
	for i, block := range blocks {
		lines = append(lines, fmt.Sprintf(
			`Dialogue: 0,0:00:00.00,0:00:10.00,M,,0,0,0,,{\pos(%d,%d)}%s`,
			width/2, anchors[i], block))
	}

	tmp, err := os.MkdirTemp("", "framefairy-")
	if err != nil {
		return nil, false
	}
	defer os.RemoveAll(tmp)
	name := "measure.ass"
	if err := os.WriteFile(filepath.Join(tmp, name),
		[]byte(strings.Join(lines, "\n")+"\n"), 0o644); err != nil {
		return nil, false
	}
	// Measuring has to use the same face as the render, or the pills behind
	// the words come out the wrong size.
	if err := InstallFont(tmp, s.Font); err != nil {
		return nil, false
	}
	template, err := e.SubtitleFilter(ctx)
	if err != nil {
		return nil, false
	}
	chain := subtitleChain(template, name)
	raw, code := runBytes(ctx, tmp, e.FFmpeg, "-hide_banner", "-loglevel", "error",
		"-nostats", "-f", "lavfi",
		"-i", fmt.Sprintf("color=c=black:s=%dx%d:d=0.04:r=25", width, height),
		"-filter_complex", "[0:v]"+chain+",format=gray[v]",
		"-map", "[v]", "-frames:v", "1", "-f", "rawvideo", "-")
	if code != 0 || len(raw) < width*height {
		return nil, false
	}

	boxes := make([]*inkBox, 0, len(blocks))
	for i := range blocks {
		left, right, top, bottom := width, -1, height, -1
		bandStart := offsets[i]
		bandEnd := min(bandStart+slots[i], height)
		for row := bandStart; row < bandEnd; row++ {
			rowBytes := raw[row*width : row*width+width]
			first := -1
			for col := 0; col < width; col++ {
				if rowBytes[col] > 40 {
					first = col
					break
				}
			}
			if first < 0 {
				continue
			}
			last := width - 1
			for last > first && rowBytes[last] <= 40 {
				last--
			}
			left = min(left, first)
			right = max(right, last)
			top = min(top, row)
			bottom = max(bottom, row)
		}
		switch {
		case right < 0:
			boxes = append(boxes, nil)
		case top <= bandStart || bottom >= bandEnd-1:
			// The text reached the edge of its band, so the measurement may
			// be clipped and cannot be trusted.
			e.Log.Detail("caption %d did not fit its measuring band", i+1)
			boxes = append(boxes, nil)
		default:
			// Offsets from the anchor point, which is what carries over to
			// the real render regardless of where the caption ends up.
			ax, ay := width/2, anchors[i]
			boxes = append(boxes, &inkBox{left - ax, top - ay, right - ax, bottom - ay})
		}
	}
	return boxes, true
}

func bandSlot(block string, size int) int {
	lines := strings.Count(block, `\N`) + 1
	slot := max(int(float64(size)*(float64(1.45*float64(lines))+1.5)), 120)
	return slot + slot%2
}

// measureMany measures any number of blocks, a frame at a time, so a clip
// with many words stays inside the height one frame can have.
func (e *Engine) measureMany(ctx context.Context, blocks []string, s Style,
	width, size int) ([]*inkBox, bool) {
	var out []*inkBox
	for start := 0; start < len(blocks); {
		end, height := start, 0
		for end < len(blocks) {
			slot := bandSlot(blocks[end], size)
			if end > start && height+slot > 15000 {
				break
			}
			height += slot
			end++
		}
		boxes, ok := e.measureASS(ctx, blocks[start:end], s, width, size)
		if !ok {
			return nil, false
		}
		out = append(out, boxes...)
		start = end
	}
	return out, true
}

// roundedRect is an ASS drawing command for a rectangle with bezier corners.
func roundedRect(x0, y0, x1, y1, radius float64) string {
	radius = math.Max(0, math.Min(radius, math.Min((x1-x0)/2, (y1-y0)/2)))
	f := func(v float64) string { return fixed(v, 0) }
	if radius <= 0 {
		return fmt.Sprintf("m %s %s l %s %s l %s %s l %s %s",
			f(x0), f(y0), f(x1), f(y0), f(x1), f(y1), f(x0), f(y1))
	}
	r := radius
	return strings.Join([]string{
		fmt.Sprintf("m %s %s", f(x0+r), f(y0)),
		fmt.Sprintf("l %s %s", f(x1-r), f(y0)),
		fmt.Sprintf("b %s %s %s %s %s %s", f(x1), f(y0), f(x1), f(y0), f(x1), f(y0+r)),
		fmt.Sprintf("l %s %s", f(x1), f(y1-r)),
		fmt.Sprintf("b %s %s %s %s %s %s", f(x1), f(y1), f(x1), f(y1), f(x1-r), f(y1)),
		fmt.Sprintf("l %s %s", f(x0+r), f(y1)),
		fmt.Sprintf("b %s %s %s %s %s %s", f(x0), f(y1), f(x0), f(y1), f(x0), f(y1-r)),
		fmt.Sprintf("l %s %s", f(x0), f(y0+r)),
		fmt.Sprintf("b %s %s %s %s %s %s", f(x0), f(y0), f(x0), f(y0), f(x0+r), f(y0)),
	}, " ")
}

// WriteASS writes the burned-in caption track for one clip.
func (e *Engine) WriteASS(ctx context.Context, captions []Caption, path string,
	width, height int, overrides map[string]any) error {
	s := ResolveStyle(overrides)
	// The lines and the size that fit the frame, worked out once for the
	// whole clip and used by both tracks.
	laid, s := LayOutCaptions(captions, s)
	if s.Highlight {
		done, err := e.writeHighlighted(ctx, laid, path, width, height, s)
		if done || err != nil {
			return err
		}
	}
	scale := float64(height) / 1920.0
	size := max(12, pyround(s.Size*scale))
	outline := max(1, pyround(s.Outline*scale))
	shadow := max(0, pyround(s.Shadow*scale))
	marginH := pyround(s.MarginH * scale)
	marginV := pyround(s.MarginV * scale)

	blocks := make([]string, len(laid))
	for i, c := range laid {
		blocks[i] = c.assText(0)
	}

	// Measured once, and one caption that cannot be measured sends the whole
	// clip to square boxes, so no caption is styled for a box that never
	// appears.
	var boxes []*inkBox
	if s.Radius > 0 && s.BorderStyle == 4 {
		measured, ok := e.measureASS(ctx, blocks, s, width, size)
		if ok {
			for _, box := range measured {
				if box == nil {
					e.Log.Detail("a caption could not be measured, using square boxes")
					ok = false
					break
				}
			}
		}
		if ok {
			boxes = measured
		} else {
			e.Log.Detail("could not measure the caption text, falling back to a square box")
		}
	}

	// The text style depends on whether a box is actually going to be drawn,
	// so it is decided here rather than from the settings alone.
	captionBorder := s.BorderStyle
	if boxes != nil {
		captionBorder = 1
	}

	header := assHeader(s, width, height, size, captionBorder, outline, shadow, marginH, marginV)

	var events []string
	if boxes == nil {
		for _, c := range laid {
			events = append(events, fmt.Sprintf("Dialogue: 0,%s,%s,Caption,,0,0,0,,%s",
				SecondsToASS(c.Start), SecondsToASS(c.End), c.assText(s.Pad)))
		}
	} else {
		padX := float64(pyround(s.BoxPadX * scale))
		padY := float64(pyround(s.BoxPadY * scale))
		radius := float64(pyround(s.Radius * scale))
		alpha := "80"
		if len(s.BackColour) >= 10 {
			alpha = s.BackColour[2:4]
		}
		fill := "&H" + s.BackColour[max(0, len(s.BackColour)-6):] + "&"
		for i, c := range laid {
			start, end := SecondsToASS(c.Start), SecondsToASS(c.End)
			box := boxes[i]
			// The caption is anchored bottom-centre at margin_v. The measured
			// offsets are relative to that same anchor, so the box is the ink
			// extent plus padding, moved into place.
			ax := float64(width) / 2
			ay := float64(height - marginV)
			shape := roundedRect(ax+float64(box.left)-padX, ay+float64(box.top)-padY,
				ax+float64(box.right)+padX, ay+float64(box.bottom)+padY, radius)
			events = append(events, fmt.Sprintf(
				`Dialogue: 0,%s,%s,Box,,0,0,0,,{\p1\pos(0,0)\1c%s\1a&H%s&\bord0\shad0}%s{\p0}`,
				start, end, fill, alpha, shape))
			events = append(events, fmt.Sprintf("Dialogue: 1,%s,%s,Caption,,0,0,0,,%s",
				start, end, blocks[i]))
		}
	}
	return os.WriteFile(path, []byte(header+strings.Join(events, "\n")+"\n"), 0o644)
}

func assHeader(s Style, width, height, size, captionBorder, outline, shadow,
	marginH, marginV int) string {
	return fmt.Sprintf(`[Script Info]
ScriptType: v4.00+
PlayResX: %d
PlayResY: %d
WrapStyle: 2
ScaledBorderAndShadow: yes
YCbCr Matrix: TV.709

[V4+ Styles]
Format: Name, Fontname, Fontsize, PrimaryColour, SecondaryColour, OutlineColour, BackColour, Bold, Italic, Underline, StrikeOut, ScaleX, ScaleY, Spacing, Angle, BorderStyle, Outline, Shadow, Alignment, MarginL, MarginR, MarginV, Encoding
Style: Caption,%s,%d,%s,%s,%s,%s,%d,0,0,0,100,100,0,0,%d,%d,%d,2,%d,%d,%d,1

Style: Box,%s,%d,%s,%s,%s,%s,0,0,0,0,100,100,0,0,1,0,0,7,0,0,0,1

[Events]
Format: Layer, Start, End, Style, Name, MarginL, MarginR, MarginV, Effect, Text
`, width, height,
		s.Font, size, s.Primary, s.Primary, s.OutlineColour, s.BackColour, int(s.Bold),
		captionBorder, outline, shadow, marginH, marginH, marginV,
		s.Font, size, s.Primary, s.Primary, s.Primary, s.Primary)

}
