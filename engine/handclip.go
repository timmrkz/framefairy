package engine

import (
	"context"
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"unicode"
)

// HandPlanName is the clip set clips made by hand go into. It is no search:
// it has no window, it marks nothing as searched, and giving a searched
// part back leaves its clips where they are.
const HandPlanName = "clips-hand.json"

// IsHandPlan says whether a plan file is the clip set made by hand.
func IsHandPlan(path string) bool { return filepath.Base(path) == HandPlanName }

// HandPlanPath is where this episode's clips made by hand are kept.
func (p *Project) HandPlanPath() string { return filepath.Join(p.LogsDir(), HandPlanName) }

// MakeClip makes a clip at a moment of the episode, for a part the model
// did not pick, the way an editor's In and Out marks do. Forward, it starts
// at the line the moment stands in and takes the lines after it. Backward,
// it ends at that line and takes the lines before it, for a moment noticed
// only once it has passed. Either way it grows a whole line at a time until
// it is as long as Min asks, never past Max for the sake of a line more.
// The pauses are cut and the crop is framed exactly as they are for a clip
// the model found. It answers with the clip set and the new clip's id.
func (p *Project) MakeClip(ctx context.Context, at float64, backward bool) (string, string, error) {
	sketch, err := p.SketchClip(at, backward)
	if err != nil {
		return "", "", err
	}
	o := p.Base
	e := p.engine
	source, err := e.Probe(ctx, p.Source)
	if err != nil {
		return "", "", err
	}
	cropW, _ := CropWindow(source, o.Width, o.Height)
	segments, err := e.ClipSegments(ctx, p.Source, sketch.cut.spans, source, cropW, newCropCache())
	if err != nil {
		return "", "", err
	}
	if len(segments) == 0 {
		return "", "", renderErr("there is nothing to make a clip of at %s", HMS(at))
	}
	title := sketch.Title
	clip := planClipOf("", strings.ToLower(SanitiseName(title, "clip")), title, "", sketch.cut, segments)

	path := p.HandPlanPath()
	if err := os.MkdirAll(p.LogsDir(), 0o755); err != nil {
		return "", "", err
	}
	var id string
	write := func() error {
		var err error
		id, err = addHandClip(path, filepath.Base(p.Source), clip)
		return err
	}
	if p.Edit != nil {
		err = p.Edit(write)
	} else {
		err = write()
	}
	if err != nil {
		return "", "", err
	}
	return path, id, nil
}

// ClipSketch is a clip made by hand before it is framed: the parts of the
// episode it keeps, the words said in them, what it is called and where its
// captions fall on its own clock. It is worked out from the transcript
// alone, in a moment, so the app can show the clip taking shape while the
// crop, which reads the picture, is still being placed.
type ClipSketch struct {
	Spans    []Span
	Words    []Cue
	Title    string
	Captions []Caption
	cut      cutClip
}

// SketchClip is the clip MakeClip would make at a moment, without framing
// it: the same lines, the same pauses cut, the same title and captions.
func (p *Project) SketchClip(at float64, backward bool) (ClipSketch, error) {
	t, err := p.Transcript()
	if err != nil {
		return ClipSketch{}, err
	}
	o := p.Base
	lines := BuildLines(t.Words, t.Levels(), o.MaxPause)
	if len(lines) == 0 {
		return ClipSketch{}, renderErr("nothing is said in this episode yet")
	}
	// The line the moment stands in. In a pause, that is the line after it
	// for a clip that starts here and the line before it for one that ends
	// here.
	here := -1
	for i, l := range lines {
		if l.End() > at {
			here = i
			break
		}
	}
	switch {
	case here < 0 && !backward:
		return ClipSketch{}, renderErr("nothing is said after %s", HMS(at))
	case here < 0:
		here = len(lines) - 1
	case backward && lines[here].Start() > at:
		if here == 0 {
			return ClipSketch{}, renderErr("nothing is said before %s", HMS(at))
		}
		here--
	}
	// The lines come from the playhead, where a search takes them from the
	// model's answer. From there the clip is cut the one way every clip is:
	// its edges on sentences, filler off its ends, long pauses out.
	cut := cutKeep(lines, handKeep(lines, here, at, backward, o), o.KeepPause, o.MaxPause)
	if len(cut.spans) == 0 {
		return ClipSketch{}, renderErr("there is nothing to make a clip of at %s", HMS(at))
	}
	sketch := ClipSketch{Spans: cut.spans, Words: cut.words, Title: handTitle(cut.words), cut: cut}
	clip := Clip{Words: cut.words}
	for _, s := range cut.spans {
		clip.Segments = append(clip.Segments, Segment{Start: s.Start, End: s.End})
	}
	// Broken the way the render breaks them in the style a new clip set
	// starts with, so the blocks shown while the clip is framed are the
	// ones it arrives with.
	style := ResolveStyle(nil)
	sketch.Captions = Captions(clip, max(8, int(style.MaxChars)), TooWide(style))
	return sketch, nil
}

// handKeep is the lines, counted from 1 the way a plan keeps them, a clip
// made by hand at line here keeps: the lines handRange picks, shaped the
// way every clip's are.
func handKeep(lines []Line, here int, at float64, backward bool, o Options) [][2]int {
	length := func(first, last int) float64 {
		return keepSeconds(lines, [][2]int{{first + 1, last + 1}}, o.KeepPause, o.MaxPause)
	}
	first, last := handRange(lines, here, at, backward, o.Min, o.Max, length)
	held := holdStart
	if backward {
		held = holdEnd
	}
	return shapeKeep(lines, [][2]int{{first + 1, last + 1}}, o.Max, o.KeepPause, o.MaxPause, false, held)
}

// handRange is the lines, counted from 0, a clip made by hand at line here
// keeps, and length is how long lines first to last come out once cut.
func handRange(lines []Line, here int, at float64, backward bool, shortest, longest float64,
	length func(first, last int) float64) (first, last int) {
	// In marks where the clip starts, so it takes the sentence the playhead
	// stands in from its beginning, even a few seconds before the playhead.
	// Out marks where it ends, so it takes that sentence to its end. From
	// there it grows to Shortest, and shapeKeep puts the other edge on a
	// sentence the way it does for the model's clips. A line ends at a pause
	// or a length and not at a sentence, and a clip that began at the line
	// the playhead stood in began in the middle of what was being said.
	// Where sentences begin and end is decided once, for every clip, see
	// sentenceStart and sentenceEnd in edges.go.
	// The sentence the playhead stands in begins at or before it and ends at
	// or after it, so a beginning later in its line, or an end earlier in
	// it, belongs to another sentence.
	starts := func(i int) bool {
		k := sentenceStart(lines, i)
		return k >= 0 && (i != here || lines[i].Cues[k].Start <= at)
	}
	ends := func(i int) bool {
		k := sentenceEnd(lines, i)
		return k >= 0 && (i != here || lines[i].Cues[k].End >= at)
	}
	// Where the sentence of line i begins and ends, no further off than
	// sentenceReach, the same reach every clip's edges have. A transcript
	// without punctuation for that long has no sentence to go to, and the
	// line itself stands.
	startOf := func(i int) int {
		for n := i; n >= 0 && lines[i].Start()-lines[n].Start() <= sentenceReach; n-- {
			if starts(n) {
				return n
			}
		}
		return i
	}
	endOf := func(i int) int {
		for n := i; n < len(lines) && lines[n].End()-lines[i].End() <= sentenceReach; n++ {
			if ends(n) {
				return n
			}
		}
		return i
	}
	first, last = here, here
	if backward {
		last = endOf(here)
		first = startOf(last)
		for first > 0 && length(first, last) < shortest {
			if length(first-1, last) > longest {
				break
			}
			first--
		}
	} else {
		first = startOf(here)
		for last+1 < len(lines) && length(first, last) < shortest {
			if length(first, last+1) > longest {
				break
			}
			last++
		}
	}
	return first, last
}

// addHandClip puts a clip into the clip set made by hand, under the first
// id that set has not used, and makes the set if it is not there yet.
func addHandClip(path, source string, clip PlanClip) (string, error) {
	for n := 1; n <= MaxClips; n++ {
		clip.ID = fmt.Sprintf("h%02d", n)
		if taken(path, clip.ID) {
			continue
		}
		made, err := makeHandPlan(path, source, clip)
		if err != nil {
			return "", err
		}
		if made {
			return clip.ID, nil
		}
		err = appendClip(path, clip)
		if err == errNotAdded {
			// Taken since it was looked at, by a click just before.
			continue
		}
		if err != nil {
			return "", err
		}
		return clip.ID, nil
	}
	return "", renderErr("a clip set holds %d clips at most", MaxClips)
}

// makeHandPlan writes the clip set with this one clip in it, when there is
// no set yet. It answers whether it did, and holds the plan's lock so two
// clips made at the same moment do not both write a new set.
func makeHandPlan(path, source string, clip PlanClip) (bool, error) {
	unlock := lockPlan(path)
	defer unlock()
	if _, err := os.Stat(path); err == nil {
		return false, nil
	}
	body, err := MarshalPlan(PlanFile{Source: source, PlannedWith: PlannedWith{By: "hand"},
		Clips: []PlanClip{clip}})
	if err != nil {
		return false, err
	}
	// Under the lock already held, which writePlanFile would take again.
	return true, replacePlan(path, body)
}

func taken(path, id string) bool {
	_, clips, err := LoadClips(path)
	if err != nil {
		return false
	}
	for _, c := range clips {
		if c.ID == id {
			return true
		}
	}
	return false
}

// handTitle is the first words said in a clip, which is what a person
// would call it until they call it something else.
func handTitle(said []Cue) string {
	var words []string
	for _, w := range said {
		text := strings.TrimFunc(w.Text, func(r rune) bool { return !unicode.IsLetter(r) && !unicode.IsDigit(r) })
		if text != "" {
			words = append(words, text)
		}
		if len(words) == 6 {
			break
		}
	}
	title := strings.Join(words, " ")
	if len(said) > len(words) && title != "" {
		title += " …"
	}
	if title == "" {
		return "Clip"
	}
	return Scrub(title, 200)
}
