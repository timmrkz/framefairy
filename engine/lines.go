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

// Caption timing.
const (
	// A caption shorter than this is merged into the one before it.
	minCaption = 0.35
	// A caption stays up this long after its last word when a pause follows.
	captionHold = 0.4
	// A pause this long between words ends a caption.
	captionPause = 0.6
	// A caption that goes this close to the end of a clip is on screen at
	// the end, two frames at the rates people film at.
	captionAtEnd = 0.08
)

// Captions gives a clip's captions on the clip's own timeline.
//
// A caption is a run of words up to maxChars long, ending early at a pause
// or at a sentence end once it has some substance. It appears when its first
// word is spoken. It stays up until the next caption appears, unless a pause
// comes first, in which case it goes shortly after its last word.
//
// A pause is measured in the episode, not in the clip. A cut takes out what
// lies in it and nothing else: a cut in a pause between two captions left a
// pause too short to part them on the clip's clock, so the two became one,
// and a caption held up for the pause it no longer had. The captions are the
// ones the episode has there, less what the cut took. A caption that would
// stay up past where a cut begins goes where the cut does.
//
// A word alone says is too wide for a line gets a caption of its own, which
// is how it is read as two lines of one caption, hyphenated, rather than
// as a third line under the words around it. alone may be nil.
//
// A word with no text, or with its text behind removedMark, is a word
// removed, see Transcript.Correct. It is not shown, but it was still said,
// so the captions are laid out as if it were there, and then it is left
// out of them: the time it holds is no pause, the room it took on a line
// is still taken, and a sentence it ended still ends there. Taking a word
// out takes out the word and changes no caption. Without this, removing a
// word left a gap that read as a pause, so the caption ended at the word
// before it and the rest moved on to the next one, and once the gap was
// closed, the room it left pulled the first word of the next caption up.
func Captions(clip Clip, said []Cue, maxChars int, alone func(string) bool) []Caption {
	said = nearClip(clip, said)
	all, allWas := clipWords(clip, said)
	// The words shown, the same words on the episode's clock, and for each
	// where the words removed straight after it end, on either clock, or
	// where it ends when none were.
	var words, was []Cue
	var reach, wasReach []float64
	// For each of all the words, what it takes on a line, and where it is
	// among the words shown, or -1 for a word removed.
	laid := make([]string, len(all))
	shownAt := make([]int, len(all))
	for k, w := range all {
		text, gone := removedText(w.Text)
		laid[k], shownAt[k] = text, -1
		if !gone {
			shownAt[k] = len(words)
			words, was = append(words, w), append(was, allWas[k])
			reach, wasReach = append(reach, w.End), append(wasReach, allWas[k].End)
		} else if len(reach) > 0 {
			reach[len(reach)-1] = math.Max(reach[len(reach)-1], w.End)
			wasReach[len(wasReach)-1] = math.Max(wasReach[len(wasReach)-1], allWas[k].End)
		}
	}
	if len(words) == 0 {
		return nil
	}
	shown := make([]Cue, 0, len(said))
	for _, w := range said {
		if _, gone := removedText(w.Text); !gone {
			shown = append(shown, w)
		}
	}
	// The pause after a word, as long as it is in the episode, from where
	// the words removed after it end.
	pauseAfter := func(i int) float64 { return was[i+1].Start - wasReach[i] }
	type group struct {
		words []Cue
		alone bool
		// Where its first and last word are in the words shown.
		first, last int
		// Where its first word is said on the clip's clock, a word removed
		// among them, since a caption is on screen from there as it was.
		from float64
	}
	var groups []group
	text := func(ws []Cue) string {
		parts := make([]string, len(ws))
		for i, w := range ws {
			parts[i] = w.Text
		}
		return strings.Join(parts, " ")
	}
	// The words are grouped as laid, the removed ones among them, and a
	// group is then the words shown in it. One with none shown is no
	// caption.
	var current []int
	line := func(ks []int) string {
		parts := make([]string, 0, len(ks))
		for _, k := range ks {
			if laid[k] != "" {
				parts = append(parts, laid[k])
			}
		}
		return strings.Join(parts, " ")
	}
	closeGroup := func(isAlone bool) {
		g := group{alone: isAlone, first: -1, from: all[current[0]].Start}
		for _, k := range current {
			if i := shownAt[k]; i >= 0 {
				if g.first < 0 {
					g.first = i
				}
				g.last = i
				g.words = append(g.words, words[i])
			}
		}
		if len(g.words) > 0 {
			groups = append(groups, g)
		}
		current = nil
	}
	for k := range all {
		if alone != nil && laid[k] != "" && alone(laid[k]) {
			if len(current) > 0 {
				closeGroup(false)
			}
			current = []int{k}
			closeGroup(true)
			continue
		}
		if len(current) > 0 && runeLen(line(append(current[:len(current):len(current)], k))) > maxChars {
			closeGroup(false)
		}
		current = append(current, k)
		last := k == len(all)-1
		pauseNext := !last && allWas[k+1].Start-allWas[k].End >= captionPause
		sentence := endsWithBreak(laid[k]) &&
			float64(runeLen(line(current))) >= float64(maxChars)*0.55
		if last || pauseNext || sentence {
			closeGroup(false)
		}
	}

	var out []Caption
	// Whether the caption made last is a word on its own, which nothing
	// may ride with.
	lastAlone := false
	// Where a caption appears: with its first word, or where a cut ends
	// when the cut took a word it ran straight on from. A caption was on
	// screen from that word, so a cut that takes it shows the caption from
	// where the clip comes back, the way a cut into the word itself does.
	// It used to wait for its next word: a cut moved a frame further into
	// a short first word took the word, and the caption jumped past the
	// pause after it, though it had been on screen there a moment before.
	saidHere := make(map[float64]bool, len(allWas))
	for _, w := range allWas {
		saidHere[w.Start] = true
	}
	appears := func(k int) float64 {
		at := words[k].Start
		i := sort.Search(len(said), func(j int) bool { return said[j].Start >= was[k].Start })
		if i == 0 || len(clip.Segments) == 0 {
			return at
		}
		before := said[i-1]
		if saidHere[before.Start] || before.End <= clip.Segments[0].Start ||
			was[k].Start-before.End >= captionPause {
			return at
		}
		// From where the piece its first word is in begins: the word cut
		// out may end a hair inside that piece, too little of it to be
		// said, and the caption would then wait for that hair.
		piece := clip.Segments[0].Start
		for _, seg := range clip.Segments {
			if seg.Start <= was[k].Start+wordTouch {
				piece = seg.Start
			}
		}
		return math.Min(at, ClipTime(clip, math.Max(piece, before.Start)))
	}

	for i, g := range groups {
		start := math.Min(appears(g.first), g.from)
		if len(out) > 0 {
			start = math.Max(start, out[len(out)-1].Start)
		}
		// Held for as long as it is in the episode, and gone where a cut
		// begins inside that hold.
		end := math.Max(reach[g.last], ClipTime(clip, wasReach[g.last]+captionHold))
		if i+1 < len(groups) {
			next := math.Min(appears(groups[i+1].first), groups[i+1].from)
			if pauseAfter(g.last) < captionPause {
				end = next
			} else {
				end = math.Min(end, next)
			}
		}
		if len(out) > 0 && end-start < minCaption && !g.alone && !lastAlone {
			// Too brief to read on its own, so it rides with the one before.
			prev := out[len(out)-1]
			out[len(out)-1] = Caption{Start: prev.Start, End: end,
				Text: prev.Text + " " + text(g.words), Words: append(prev.Words, g.words...)}
			continue
		}
		out = append(out, Caption{Start: start, End: end, Text: text(g.words), Words: g.words})
		lastAlone = g.alone
	}
	// Held a second past the end when it is on screen at the end. Constant
	// frame rate output usually lands a frame or two beyond the planned
	// length, and a caption that stops exactly at the end leaves those
	// frames bare. libass stops with the video. Only then: the last caption
	// was held to the end whatever it was, so trimming the end over the last
	// word turned the caption before it, which had gone after its own word,
	// into one that stayed to the end, and on the clip timeline two captions
	// looked to become one.
	length := clipLength(clip)
	last := out[len(out)-1]
	if last.End >= length-captionAtEnd {
		out[len(out)-1].End = math.Max(last.End, length+1.0)
	}
	if out[0].Start < 0.12 {
		out[0].Start = 0
	}
	return moveCaptions(clip, shown, out)
}

// removedMark comes before the text of a word removed, in the words the
// captions are laid out from, see Transcript.captionWords. No correction
// has it, since Scrub takes control characters out of every one, and no
// recogniser writes one. A word that did would be hidden and nothing more.
const removedMark = "\x00"

// removedText is what a word takes on a line and whether it was removed.
// A removed word with no text left takes nothing.
func removedText(text string) (string, bool) {
	if rest, ok := strings.CutPrefix(text, removedMark); ok {
		return rest, true
	}
	return text, strings.TrimSpace(text) == ""
}

// The shortest a caption moved by hand may be shown for.
const shortestMoved = 0.1

// moveCaptions puts the captions moved by hand where they were put. A
// caption shown earlier or later takes the one before it along where the
// two met, so nothing is left bare between them that was not bare before.
// A caption that goes earlier leaves the gap it makes, and one that stays
// longer stops where the next one begins. Nothing ever overlaps, because
// the picture shows one caption at a time.
func moveCaptions(clip Clip, said []Cue, captions []Caption) []Caption {
	if len(clip.CaptionTimes) == 0 || len(captions) == 0 {
		return captions
	}
	out := append([]Caption(nil), captions...)
	for i := range out {
		key, ok := captionAnchor(clip, said, out[i], false)
		if !ok {
			continue
		}
		t := clip.CaptionTimes[key]
		if t.Start == nil {
			continue
		}
		low := 0.0
		if i > 0 {
			low = out[i-1].Start + shortestMoved
		}
		high := out[i].End - shortestMoved
		if high < low {
			continue
		}
		start := min(max(ClipTime(clip, *t.Start), low), high)
		if i > 0 {
			met := math.Abs(out[i-1].End-out[i].Start) < 0.001
			if met || out[i-1].End > start {
				out[i-1].End = start
			}
		}
		out[i].Start = start
	}
	for i := range out {
		key, ok := captionAnchor(clip, said, out[i], true)
		if !ok {
			continue
		}
		t := clip.CaptionTimes[key]
		if t.End == nil {
			continue
		}
		high := math.Inf(1)
		if i+1 < len(out) {
			high = out[i+1].Start
		}
		out[i].End = min(max(ClipTime(clip, *t.End), out[i].Start+shortestMoved), high)
	}
	return out
}

// captionAnchor is the word a caption begins on, or ends on, as the key its
// timing is kept under: the millisecond it starts in the episode.
func captionAnchor(clip Clip, said []Cue, c Caption, last bool) (string, bool) {
	if len(c.Words) == 0 {
		return "", false
	}
	w := c.Words[0]
	if last {
		w = c.Words[len(c.Words)-1]
	}
	word, ok := SaidWord(clip, said, (w.Start+w.End)/2)
	if !ok {
		return "", false
	}
	return wordKey(word.Start), true
}

// EpisodeTime is the moment of the episode a moment of a clip shows.
func EpisodeTime(clip Clip, at float64) float64 {
	offset := 0.0
	for _, s := range clip.Segments {
		if at < offset+s.Duration() {
			return s.Start + math.Max(at-offset, 0)
		}
		offset += s.Duration()
	}
	if len(clip.Segments) == 0 {
		return at
	}
	return clip.Segments[len(clip.Segments)-1].End
}

// ClipTime is the moment of a clip that shows a moment of the episode. A
// moment the clip cuts out is the moment the clip comes back.
func ClipTime(clip Clip, at float64) float64 {
	offset := 0.0
	for _, s := range clip.Segments {
		if at < s.Start {
			return offset
		}
		if at <= s.End {
			return offset + at - s.Start
		}
		offset += s.Duration()
	}
	return offset
}

// SaidWord is the word being said at a moment of a clip, on the episode's
// clock, out of the words the clip says.
func SaidWord(clip Clip, words []Cue, at float64) (Cue, bool) {
	when := EpisodeTime(clip, at)
	near, off := Cue{}, math.Inf(1)
	for _, w := range words {
		if when >= w.Start && when < w.End {
			return w, true
		}
		away := math.Max(w.Start-when, when-w.End)
		if away < off {
			near, off = w, away
		}
	}
	return near, off <= 0.02
}
