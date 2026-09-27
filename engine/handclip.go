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
	ranges, spans, said := sketch.ranges, sketch.Spans, sketch.Words
	o := p.Base

	e := p.engine
	source, err := e.Probe(ctx, p.Source)
	if err != nil {
		return "", "", err
	}
	cropW, _ := CropWindow(source, o.Width, o.Height)
	tight := make([]Span, len(spans))
	for k, s := range spans {
		tight[k] = Span{s.Start, s.End}
	}
	segments, err := e.ClipSegments(ctx, p.Source, tight, source, cropW, newCropCache())
	if err != nil {
		return "", "", err
	}
	if len(segments) == 0 {
		return "", "", renderErr("there is nothing to make a clip of at %s", HMS(at))
	}

	title := sketch.Title
	clip := PlanClip{Title: title, Slug: strings.ToLower(SanitiseName(title, "clip")),
		Keep: ranges, Words: [][3]any{}}
	for _, w := range said {
		clip.Words = append(clip.Words, [3]any{PyFloat(roundTo(w.Start, 3)),
			PyFloat(roundTo(w.End, 3)), w.Text})
	}
	for _, s := range segments {
		seg := PlanSegment{Start: PyFloat(roundTo(s.Start, 3)), End: PyFloat(roundTo(s.End, 3)),
			CropX: "center"}
		if s.CropX != nil {
			seg.CropX = *s.CropX
		}
		clip.Segments = append(clip.Segments, seg)
	}

	path := p.HandPlanPath()
	if err := os.MkdirAll(p.LogsDir(), 0o755); err != nil {
		return "", "", err
	}
	id, err := addHandClip(path, filepath.Base(p.Source), clip)
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
	ranges   [][2]int
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
	length := func(first, last int) float64 {
		total := 0.0
		for _, s := range SegmentsFromRanges([][2]int{{first + 1, last + 1}}, lines, o.KeepPause, o.MaxPause) {
			total += s.Duration()
		}
		return total
	}
	first, last := here, here
	if backward {
		for first > 0 && length(first, last) < o.Min {
			if length(first-1, last) > o.Max {
				break
			}
			first--
		}
	} else {
		for last+1 < len(lines) && length(first, last) < o.Min {
			if length(first, last+1) > o.Max {
				break
			}
			last++
		}
	}
	ranges := [][2]int{{first + 1, last + 1}}
	spans := SegmentsFromRanges(ranges, lines, o.KeepPause, o.MaxPause)
	if len(spans) == 0 {
		return ClipSketch{}, renderErr("there is nothing to make a clip of at %s", HMS(at))
	}

	var said []Cue
	for n := first; n <= last; n++ {
		said = append(said, lines[n].Cues...)
	}
	sketch := ClipSketch{Spans: make([]Span, len(spans)), Words: said, Title: handTitle(said), ranges: ranges}
	clip := Clip{Words: said}
	for k, s := range spans {
		sketch.Spans[k] = Span{s.Start, s.End}
		clip.Segments = append(clip.Segments, Segment{Start: s.Start, End: s.End})
	}
	sketch.Captions = Captions(clip, 38)
	return sketch, nil
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
