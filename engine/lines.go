package engine

import (
	"fmt"
	"math"
	"sort"
	"strings"
)

// ---------------------------------------------------------------------------
// Speech lines
//
// A line is the unit the model selects by, a run of words with no real pause
// inside it. Cuts land between lines, captions are made of the same words,
// and every word carries the time it was spoken, so what is read and what is
// heard cannot drift apart.
// ---------------------------------------------------------------------------

// Line is one numbered line of the transcript the model reads.
type Line struct {
	Index int
	// Cues are the line's words.
	Cues      []Cue
	GapBefore float64
	// Level is dB relative to the speaker's usual level.
	Level float64
}

// Start of the line in seconds.
func (l Line) Start() float64 { return l.Cues[0].Start }

// End of the line in seconds.
func (l Line) End() float64 { return l.Cues[len(l.Cues)-1].End }

// Duration of the line in seconds.
func (l Line) Duration() float64 { return l.End() - l.Start() }

// maxLineChars is far above any line of speech. A line runs at most 14
// seconds, which is some 300 characters.
const maxLineChars = 10_000

// Text of the line, scrubbed.
//
// The numbered transcript is what the model reads, and one row per line is
// what makes its answer mean anything. A line break inside a word would add
// a row that no line stands behind, so the text is cleaned here rather than
// trusted to arrive clean.
func (l Line) Text() string {
	parts := make([]string, len(l.Cues))
	for i, c := range l.Cues {
		parts[i] = c.Text
	}
	return Scrub(strings.Join(parts, " "), maxLineChars)
}

// Line breaks.
const (
	// A pause at least this long always ends a line.
	splitPause = 0.45
	// A sentence end ends a line once the line has some substance.
	sentenceLine = 2.5
	// No line runs longer than this.
	maxLineSeconds = 14.0
)

// LineBreakPause is the shortest pause that ends a line, and so the
// shortest pause that can be cut or restored on its own.
func LineBreakPause(maxPause *float64) float64 {
	if maxPause != nil {
		return *maxPause
	}
	return splitPause
}

// BuildLines groups words into the numbered lines the model reads.
//
// A line ends at a pause, at a sentence end once it has some length, or when
// it would grow past 14 seconds, in which case it breaks at the last sentence
// end or comma inside it if there is one. A pause between two lines is the
// only kind of pause the model can choose to keep or drop, so a --max-pause
// also becomes the pause that ends a line.
func BuildLines(words []Cue, levels []Reading, maxPause *float64) []Line {
	breakAt := splitPause
	if maxPause != nil {
		breakAt = *maxPause
	}
	var groups [][]Cue
	for _, word := range words {
		if strip(word.Text) == "" {
			continue
		}
		if len(groups) == 0 {
			groups = append(groups, []Cue{word})
			continue
		}
		current := groups[len(groups)-1]
		previous := current[len(current)-1]
		length := previous.End - current[0].Start
		switch {
		case word.Start-previous.End >= breakAt:
			groups = append(groups, []Cue{word})
		case endsSentence(previous.Text) && length >= sentenceLine:
			groups = append(groups, []Cue{word})
		case word.End-current[0].Start > maxLineSeconds:
			split := len(current)
			for k := len(current) - 1; k >= 1; k-- {
				if endsSentence(current[k-1].Text) {
					split = k
					break
				}
			}
			if split == len(current) {
				for k := len(current) - 1; k >= 1; k-- {
					if endsWithBreak(current[k-1].Text) {
						split = k
						break
					}
				}
			}
			head := append([]Cue(nil), current[:split]...)
			tail := append(append([]Cue(nil), current[split:]...), word)
			groups[len(groups)-1] = head
			groups = append(groups, tail)
		default:
			groups[len(groups)-1] = append(current, word)
		}
	}

	var lines []Line
	for _, group := range groups {
		gap := 0.0
		if len(lines) > 0 {
			gap = group[0].Start - lines[len(lines)-1].End()
		}
		lines = append(lines, Line{Index: len(lines) + 1, Cues: group,
			GapBefore: math.Max(0, gap)})
	}
	measureLevels(lines, levels)
	return lines
}

// measureLevels sets each line's loudness relative to the speaker's usual
// level.
func measureLevels(lines []Line, envelope []Reading) {
	if len(envelope) == 0 || len(lines) == 0 {
		return
	}
	var readings []float64
	for _, r := range envelope {
		if r.Level > -70 {
			readings = append(readings, r.Level)
		}
	}
	if len(readings) == 0 {
		return
	}
	sort.Float64s(readings)
	floor := readings[len(readings)/10]

	raw := make([]float64, len(lines))
	position := 0
	for i, line := range lines {
		for position < len(envelope) && envelope[position].At < line.Start() {
			position++
		}
		var inside []float64
		for scan := position; scan < len(envelope) && envelope[scan].At <= line.End(); scan++ {
			if envelope[scan].Level > floor {
				inside = append(inside, envelope[scan].Level)
			}
		}
		if len(inside) > 0 {
			raw[i] = pysum(inside) / float64(len(inside))
		} else {
			raw[i] = math.NaN()
		}
	}
	var usable []float64
	for _, v := range raw {
		if isFinite(v) {
			usable = append(usable, v)
		}
	}
	if len(usable) == 0 {
		return
	}
	sort.Float64s(usable)
	middle := usable[len(usable)/2]
	for i := range lines {
		if isFinite(raw[i]) {
			lines[i].Level = raw[i] - middle
		} else {
			lines[i].Level = 0
		}
	}
}

// AnnotateLines is the transcript as the model sees it, numbered, timed and
// annotated with what the audio knows.
func AnnotateLines(lines []Line) string {
	const pauseFloor, quietAt, loudAt = 0.4, -4.0, 3.0
	rows := make([]string, len(lines))
	for i, line := range lines {
		var marks []string
		if line.GapBefore >= pauseFloor {
			marks = append(marks, "pause "+fixed(line.GapBefore, 1)+"s")
		}
		if line.Level <= quietAt {
			marks = append(marks, "quieter")
		} else if line.Level >= loudAt {
			marks = append(marks, "louder")
		}
		prefix := fmt.Sprintf("[%d] %ss", line.Index, fixed(line.Duration(), 1))
		if len(marks) > 0 {
			prefix += " (" + strings.Join(marks, ", ") + ")"
		}
		rows[i] = prefix + " " + line.Text()
	}
	return strings.Join(rows, "\n")
}

// Hesitation sounds only. A trailing "und" is a dangling conjunction worth
// cutting, but a leading one often carries the sentence, so the two
// directions are not symmetric and only these are ever dropped outright.
var leadingFiller = map[string]bool{
	"äh": true, "ähm": true, "ähh": true, "öh": true, "öhm": true, "hm": true,
	"hmm": true, "mhm": true, "em": true, "ehm": true, "eh": true,
}

var trailingFiller = func() map[string]bool {
	set := map[string]bool{}
	for w := range leadingFiller {
		set[w] = true
	}
	for _, w := range []string{"und", "aber", "also", "halt", "quasi", "sozusagen",
		"irgendwie", "eben", "weil", "dass", "oder"} {
		set[w] = true
	}
	return set
}()

const fillerPunctuation = ",.!?;:"

// IsFiller is true for a line with nothing in it but hesitation.
func IsFiller(text string) bool {
	var words []string
	for _, w := range fields(text) {
		if w = strings.ToLower(strings.Trim(w, fillerPunctuation)); w != "" {
			words = append(words, w)
		}
	}
	if len(words) == 0 || runeLen(strings.Join(words, " ")) > 24 {
		return false
	}
	for _, w := range words {
		if !trailingFiller[w] {
			return false
		}
	}
	return true
}

// SegmentsFromRanges makes one segment per run of lines the model chose to
// keep.
//
// The model's answer already says what should happen to every pause. A run
// of consecutive lines spans the pauses inside it, so those pauses were
// chosen and they stay at full length. Material between runs was not chosen,
// so it goes. A ceiling is available for a pause that is a recording fault
// rather than a choice, but it is off by default.
//
// Each segment gets keepPause of air on either side, but never so much that
// it reaches into a word the model left out. Two lines can follow each other
// without any pause, and a padded cut there would let the first sound of the
// dropped word through.
func SegmentsFromRanges(ranges [][2]int, lines []Line, keepPause float64,
	ceiling *float64) []Segment {
	var segments []Segment
	for _, pair := range ranges {
		var runWords []Cue
		for number := pair[0]; number <= pair[1]; number++ {
			runWords = append(runWords, lines[number-1].Cues...)
		}
		if len(runWords) == 0 {
			continue
		}
		before := 0.0
		if pair[0] > 1 {
			before = lines[pair[0]-2].End()
		}
		after := math.Inf(1)
		if pair[1] < len(lines) {
			after = lines[pair[1]].Start()
		}
		sort.SliceStable(runWords, func(i, j int) bool { return runWords[i].Start < runWords[j].Start })
		pieces := []Span{{runWords[0].Start, runWords[0].End}}
		for i := 1; i < len(runWords); i++ {
			gap := runWords[i].Start - runWords[i-1].End
			if ceiling != nil && gap > *ceiling {
				pieces = append(pieces, Span{runWords[i].Start, runWords[i].End})
			} else {
				pieces[len(pieces)-1].End = runWords[i].End
			}
		}
		for k, p := range pieces {
			low := p.Start - keepPause
			if k == 0 {
				low = math.Max(low, before)
			} else {
				low = math.Max(low, pieces[k-1].End)
			}
			high := p.End + keepPause
			if k == len(pieces)-1 {
				high = math.Min(high, after)
			} else {
				high = math.Min(high, pieces[k+1].Start)
			}
			segments = append(segments, Segment{Start: math.Max(0, low), End: high})
		}
	}
	sort.SliceStable(segments, func(i, j int) bool { return segments[i].Start < segments[j].Start })
	// Two runs that follow each other directly cut the pause between them.
	// Where that pause is shorter than the room left around both cuts, the
	// pieces meet or overlap, and they are joined so nothing plays twice.
	var joined []Segment
	for _, seg := range segments {
		if n := len(joined); n > 0 && seg.Start <= joined[n-1].End+0.01 {
			joined[n-1].End = math.Max(joined[n-1].End, seg.End)
			continue
		}
		joined = append(joined, seg)
	}
	return joined
}

func clipLength(clip Clip) float64 {
	lengths := make([]float64, len(clip.Segments))
	for i, s := range clip.Segments {
		lengths[i] = s.End - s.Start
	}
	return pysum(lengths)
}

// ClipWords puts a clip's words on the clip's own clock. A word moves by
// however much earlier material the clip contains, which is plain arithmetic
// because every word knows where it was spoken. A word corrected into
// several words becomes one cue per word, so captions break and highlight
// them one by one.
func ClipWords(clip Clip) []Cue {
	var out []Cue
	offset := 0.0
	for _, segment := range clip.Segments {
		for _, word := range clip.Words {
			if word.Start < segment.Start-0.02 || word.End > segment.End+0.02 {
				continue
			}
			start := math.Max(word.Start, segment.Start)
			end := math.Min(word.End, segment.End)
			out = append(out, Cue{offset + (start - segment.Start),
				offset + (end - segment.Start), word.Text})
		}
		offset += segment.Duration()
	}
	sort.SliceStable(out, func(i, j int) bool { return out[i].Start < out[j].Start })
	return SplitCorrected(out)
}

// Caption timing.
const (
	// A caption shorter than this is merged into the one before it.
	minCaption = 0.35
	// A caption stays up this long after its last word when a pause follows.
	captionHold = 0.4
	// A pause this long between words ends a caption.
	captionPause = 0.6
)

// Captions gives a clip's captions on the clip's own timeline.
//
// A caption is a run of words up to maxChars long, ending early at a pause
// or at a sentence end once it has some substance. It appears when its first
// word is spoken. It stays up until the next caption appears, unless a pause
// comes first, in which case it goes shortly after its last word.
func Captions(clip Clip, maxChars int) []Caption {
	words := ClipWords(clip)
	if len(words) == 0 {
		return nil
	}
	type group struct{ words []Cue }
	var groups []group
	var current []Cue
	text := func(ws []Cue) string {
		parts := make([]string, len(ws))
		for i, w := range ws {
			parts[i] = w.Text
		}
		return strings.Join(parts, " ")
	}
	for i, word := range words {
		if len(current) > 0 && runeLen(text(append(current[:len(current):len(current)], word))) > maxChars {
			groups = append(groups, group{current})
			current = nil
		}
		current = append(current, word)
		last := i == len(words)-1
		pauseNext := !last && words[i+1].Start-word.End >= captionPause
		sentence := endsWithBreak(word.Text) &&
			float64(runeLen(text(current))) >= float64(maxChars)*0.55
		if last || pauseNext || sentence {
			groups = append(groups, group{current})
			current = nil
		}
	}

	var out []Caption
	for i, g := range groups {
		start := g.words[0].Start
		lastWord := g.words[len(g.words)-1]
		end := lastWord.End + captionHold
		if i+1 < len(groups) {
			next := groups[i+1].words[0].Start
			if next-lastWord.End < captionPause {
				end = next
			} else {
				end = math.Min(end, next)
			}
		}
		if len(out) > 0 && end-start < minCaption {
			// Too brief to read on its own, so it rides with the one before.
			prev := out[len(out)-1]
			out[len(out)-1] = Caption{prev.Start, end, prev.Text + " " + text(g.words),
				append(prev.Words, g.words...)}
			continue
		}
		out = append(out, Caption{start, end, text(g.words), g.words})
	}
	// Held a second past the end. Constant frame rate output usually lands a
	// frame or two beyond the planned length, and a caption that stops
	// exactly at the end leaves those frames bare. libass stops with the video.
	length := clipLength(clip)
	last := out[len(out)-1]
	out[len(out)-1].End = math.Max(last.End, length+1.0)
	if out[0].Start < 0.12 {
		out[0].Start = 0
	}
	return out
}
