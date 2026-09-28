package engine

import (
	"errors"
	"fmt"
	"math"
	"os"
	"path/filepath"
	"sort"
	"strings"
	"time"
)

// VideoExtensions are the files a library scan treats as episodes.
var VideoExtensions = []string{".mp4", ".mov", ".m4v", ".mkv"}

// PlanSummary describes one plan file without its words.
type PlanSummary struct {
	Path string  `json:"path"`
	Name string  `json:"name"`
	From float64 `json:"from"`
	To   float64 `json:"to"` // zero for a whole-episode plan
	// Parts of the window that were given back, so the model may read
	// them again. They are inside the window and never overlap.
	Removed []Window `json:"removed,omitempty"`
	Clips   int      `json:"clips"`
	Model   string   `json:"model"`
	// By is who proposed the clips, see PlanOptions.By.
	By       string    `json:"by,omitempty"`
	Modified time.Time `json:"modified"`
}

// Over is the part of the episode the clip set was made over, see
// madeOver.
func (s PlanSummary) Over(duration float64) Window {
	return madeOver(s.From, s.To, s.To > s.From, s.By, duration)
}

// EpisodeStatus is what the library shows for one episode. It is read from
// the files on disk only, so it is cheap.
type EpisodeStatus struct {
	Source   string    `json:"source"`
	Name     string    `json:"name"`
	Size     int64     `json:"size"`
	Modified time.Time `json:"modified"`
	Missing  bool      `json:"missing"`
	// Transcribed means a whole-episode transcript matches the file as it
	// is now. Stale means one exists but the file has changed since.
	Transcribed bool `json:"transcribed"`
	// Covered is how far the transcript reaches from the start without a
	// gap, in seconds, finished or not, and Heard every part it has heard,
	// from and to, which need not start at the beginning or meet: a clip
	// made by hand has the part it needs heard first.
	Covered float64      `json:"covered"`
	Heard   [][2]float64 `json:"heard"`
	// Measured is how many seconds of the loudness are measured, which is
	// the waveform, MeasuredParts which, from and to, and MeasuredAll says
	// all of it. It runs ahead of the transcript, where the clip timeline
	// looks first, see levels.go.
	Measured        float64       `json:"measured"`
	MeasuredParts   [][2]float64  `json:"measuredParts"`
	MeasuredAll     bool          `json:"measuredAll"`
	TranscriptStale bool          `json:"transcriptStale"`
	Plans           []PlanSummary `json:"plans"`
	Rendered        int           `json:"rendered"`
	Previews        int           `json:"previews"`
	// Work says whether the episode's work folder is there at all. It is
	// gone when the episode has just been added, and when someone has
	// deleted it by hand, and then there is nothing of the episode to keep
	// or to throw away.
	Work bool `json:"work"`
	// EverSearched says whether anyone has ever searched this episode for
	// clips, see EverSearched.
	EverSearched bool `json:"everSearched"`
}

// Status reads an episode's state from disk.
func Status(source, asrModelDir string) EpisodeStatus {
	st := EpisodeStatus{Source: source, Name: filepath.Base(source)}
	info, err := os.Stat(source)
	if err != nil {
		st.Missing = true
		return st
	}
	st.Size, st.Modified = info.Size(), info.ModTime()
	work := WorkDir(source)
	logs := filepath.Join(work, "logs")

	if asrModelDir == "" {
		asrModelDir = DefaultModelDir()
	}
	model := filepath.Base(filepath.Clean(asrModelDir))
	whole := filepath.Join(logs, TranscriptName(nil))
	if stamp, err := stampOf(source); err == nil {
		if file, _, _, ok := readTranscriptFile(whole, stamp, model); ok {
			st.Transcribed = file.done()
			st.Heard = file.heard()
			if len(st.Heard) > 0 && st.Heard[0][0] <= 0.005 {
				st.Covered = st.Heard[0][1]
			}
		} else if exists(whole) {
			st.TranscriptStale = true
		}
	}
	st.MeasuredParts, st.MeasuredAll = LevelsReach(source)
	for _, p := range st.MeasuredParts {
		st.Measured += p[1] - p[0]
	}
	st.Plans = PlanSummaries(logs)
	st.Rendered = countFiles(filepath.Join(work, "out"), ".mp4")
	st.Previews = countFiles(filepath.Join(work, "preview"), ".mp4")
	st.Work = exists(work)
	st.EverSearched = EverSearched(source)
	return st
}

// EverSearched says whether this episode has ever been searched for clips: it
// has a clip set made over a part of it, see madeOver, or a search has kept
// its record or its timings, see jobs.go. It stays true after the clips are
// removed again, because the timings stay, and the app searches by itself
// only for an episode nobody has searched. Clips made by hand search
// nothing. Deleting the work folder makes the episode new.
func EverSearched(source string) bool {
	for _, plan := range PlanSummaries(filepath.Join(WorkDir(source), "logs")) {
		if w := plan.Over(0); w.End > w.Start {
			return true
		}
	}
	if ReadSearch(source) != nil {
		return true
	}
	for _, t := range ReadTimings(source) {
		if t.Kind == JobSearch {
			return true
		}
	}
	return false
}

// PlanSummaries lists the plan files in a logs folder, newest first. Files
// that do not parse are left out.
func PlanSummaries(logs string) []PlanSummary {
	matches, _ := filepath.Glob(filepath.Join(logs, "clips*.json"))
	var out []PlanSummary
	for _, m := range matches {
		info, err := os.Stat(m)
		if err != nil || !info.Mode().IsRegular() {
			continue
		}
		plan, clips, err := LoadClips(m)
		if err != nil {
			continue
		}
		s := PlanSummary{Path: m, Name: filepath.Base(m), Clips: len(clips),
			Modified: info.ModTime()}
		made := plan.PlannedWith()
		if v, ok := toFloat(made["from"]); ok {
			s.From = v
		}
		if v, ok := toFloat(made["to"]); ok {
			s.To = v
		}
		if v, ok := made["model"].(string); ok {
			s.Model = v
		}
		if v, ok := made["by"].(string); ok {
			s.By = v
		}
		s.Removed = readWindows(made["removed"])
		out = append(out, s)
	}
	sort.Slice(out, func(a, b int) bool { return out[a].Modified.After(out[b].Modified) })
	return out
}

func countFiles(dir, ext string) int {
	entries, err := os.ReadDir(dir)
	if err != nil {
		return 0
	}
	n := 0
	for _, entry := range entries {
		if entry.Type().IsRegular() && strings.EqualFold(filepath.Ext(entry.Name()), ext) {
			n++
		}
	}
	return n
}

// ScanFolder lists the episodes in a folder, not its subfolders.
func ScanFolder(dir, asrModelDir string) ([]EpisodeStatus, error) {
	entries, err := os.ReadDir(dir)
	if err != nil {
		return nil, err
	}
	var out []EpisodeStatus
	for _, entry := range entries {
		name := entry.Name()
		if !entry.Type().IsRegular() || strings.HasPrefix(name, ".") || !IsVideo(name) {
			continue
		}
		out = append(out, Status(filepath.Join(dir, name), asrModelDir))
	}
	return out, nil
}

// IsVideo reports whether a file name has one of the video extensions.
func IsVideo(name string) bool {
	ext := strings.ToLower(filepath.Ext(name))
	for _, v := range VideoExtensions {
		if ext == v {
			return true
		}
	}
	return false
}

// --------------------------------------------------------------------------
// reading the transcript
// --------------------------------------------------------------------------

// ErrNoTranscript says the episode has not been transcribed yet, or its
// transcript is for an older version of the file or another speech model. It
// is a state every episode starts in, not a failure, so what only reads the
// transcript answers with nothing instead of passing it on.
var ErrNoTranscript = errors.New("this episode has no current transcript yet")

// Transcript returns the whole-episode transcript from the cache. It never
// transcribes, so it returns ErrNoTranscript if Transcribe has not run.
func (p *Project) Transcript() (*Transcript, error) {
	stamp, err := stampOf(p.Source)
	if err != nil {
		return nil, err
	}
	modelDir := p.Base.ASRModel
	if modelDir == "" {
		modelDir = DefaultModelDir()
	}
	path := filepath.Join(p.LogsDir(), TranscriptName(nil))
	file, words, frames, ok := readTranscriptFile(path, stamp,
		filepath.Base(filepath.Clean(modelDir)))
	if !ok {
		return nil, ErrNoTranscript
	}
	t := fromStored(words, frames, float64(file.From), float64(file.Mean), p.Base.SilenceDB)
	t.Heard = file.heard()
	// Corrected words show corrected, in the app and in every clip made or
	// changed from here on.
	ApplyCorrections(t.Words, LoadCorrections(p.LogsDir()))
	return t, nil
}

// Duration of the transcript in seconds.
func (t *Transcript) Duration() float64 {
	return float64(len(t.Frames)) * FrameSeconds
}

// Peaks gives the loudest reading in each of buckets equal parts of a
// part, in dB, for drawing a waveform at any zoom. Parts outside the
// transcript are silent at -90 dB.
//
// It never gives more parts than it measured. Loudness is read every
// FrameSeconds and no finer, so five buckets inside one reading are five
// copies of that reading, and whoever draws them cannot tell that from
// five readings that happen to agree: a line drawn between them comes out
// flat where the sound was rising. Answering with what there is says how
// fine the measurement was, so the drawing can be as fine as the truth and
// no finer.
func (t *Transcript) Peaks(from, to float64, buckets int) []float32 {
	if to > from {
		buckets = min(buckets, max(int(math.Ceil((to-from)/FrameSeconds)), 1))
	}
	out := make([]float32, max(buckets, 0))
	if buckets <= 0 || to <= from {
		return out
	}
	step := (to - from) / float64(buckets)
	for i := range out {
		first := int(math.Floor((from + float64(i)*step - t.Start) / FrameSeconds))
		// A hair off the end before rounding up. A bucket exactly one
		// reading wide works out to a boundary like 7.000000000000001, and
		// rounding that up takes in the reading after it: every part came
		// back as the louder of itself and its neighbour, which fattens a
		// waveform and fills in the dips between words. The hair is far
		// smaller than any real boundary and far larger than the error.
		last := int(math.Ceil((from+float64(i+1)*step-t.Start)/FrameSeconds - 1e-9))
		peak := float32(-90)
		if last <= 0 || first >= len(t.Frames) {
			out[i] = peak
			continue
		}
		first, last = max(first, 0), min(max(last, first+1), len(t.Frames))
		for k := first; k < last; k++ {
			peak = max(peak, t.Frames[k])
		}
		out[i] = peak
	}
	return out
}

// Silences are the parts quieter than the floor that last at least
// minimum seconds. Dragging a cut snaps to them.
func (t *Transcript) Silences(minimum float64) []Span {
	var out []Span
	floor := float32(t.Floor)
	start := -1
	flush := func(end int) {
		if start >= 0 && float64(end-start)*FrameSeconds >= minimum {
			out = append(out, Span{t.Start + float64(start)*FrameSeconds,
				t.Start + float64(end)*FrameSeconds})
		}
		start = -1
	}
	for i, level := range t.Frames {
		if level < floor {
			if start < 0 {
				start = i
			}
		} else {
			flush(i)
		}
	}
	flush(len(t.Frames))
	return out
}

// WordsBetween are the words that lie inside a part.
func (t *Transcript) WordsBetween(from, to float64) []Cue {
	first := sort.Search(len(t.Words), func(i int) bool { return t.Words[i].End > from })
	var out []Cue
	for _, w := range t.Words[first:] {
		if w.Start >= to {
			break
		}
		out = append(out, w)
	}
	return out
}

// --------------------------------------------------------------------------
// reading a plan
// --------------------------------------------------------------------------

// ClipView is one candidate as the app shows it.
type ClipView struct {
	ID       string        `json:"id"`
	Slug     string        `json:"slug"`
	Basename string        `json:"basename"`
	Title    string        `json:"title"`
	Reason   string        `json:"reason"`
	Duration float64       `json:"duration"`
	Start    float64       `json:"start"`
	End      float64       `json:"end"`
	Segments []SegmentView `json:"segments"`
	Words    []WordView    `json:"words"`
	Rejected bool          `json:"rejected"`
	Rendered string        `json:"rendered,omitempty"`
	Preview  string        `json:"preview,omitempty"`
	// CaptionY is where the caption line of this clip sits, as the distance
	// from the bottom of a 1080x1920 frame, when it was placed by hand.
	CaptionY float64 `json:"captionY"`
	// CaptionYMoved means the caption line was placed by hand.
	CaptionYMoved bool `json:"captionYMoved"`
	// Thumbnails are the moments of the episode the render takes a picture
	// of the short at, in time order.
	Thumbnails []float64 `json:"thumbnails"`
}

// SegmentView is one kept part of the source.
type SegmentView struct {
	Start float64 `json:"start"`
	End   float64 `json:"end"`
	CropX *int    `json:"cropX"`
	// Moved means the crop was placed by hand.
	Moved bool `json:"moved"`
}

// WordView is one word on the source clock.
type WordView struct {
	Start float64 `json:"start"`
	End   float64 `json:"end"`
	Text  string  `json:"text"`
}

// PlanView is a plan with everything the candidates screen needs.
type PlanView struct {
	Summary PlanSummary `json:"summary"`
	Clips   []ClipView  `json:"clips"`
}

// ReadPlan loads a plan for display.
func ReadPlan(path string) (*PlanView, error) {
	plan, clips, err := LoadClips(path)
	if err != nil {
		return nil, err
	}
	view := &PlanView{}
	for _, s := range PlanSummaries(filepath.Dir(path)) {
		if s.Path == path {
			view.Summary = s
		}
	}
	view.Summary.Path, view.Summary.Name = path, filepath.Base(path)

	extras := map[string]map[string]any{}
	if list, ok := plan.Raw["clips"].([]any); ok {
		for i, item := range list {
			entry, ok := item.(map[string]any)
			if !ok {
				continue
			}
			fallback := twoDigits(i + 1)
			id := fallback
			if value, present := entry["id"]; present {
				id = pyStr(value)
			}
			extras[SanitiseName(id, fallback)] = entry
		}
	}
	work := filepath.Dir(filepath.Dir(path))
	for _, c := range clips {
		v := ClipView{ID: c.ID, Slug: c.Slug, Basename: c.Basename(), Title: c.Title,
			Duration: roundTo(c.Duration(), 3), Start: c.Segments[0].Start,
			End: c.Segments[len(c.Segments)-1].End, Rejected: c.Rejected,
			CaptionYMoved: c.CaptionY != nil, Thumbnails: c.Thumbnails}
		if v.Thumbnails == nil {
			v.Thumbnails = []float64{}
		}
		if c.CaptionY != nil {
			v.CaptionY = *c.CaptionY
		}
		if extra := extras[c.ID]; extra != nil {
			if reason, ok := extra["reason"].(string); ok {
				v.Reason = Scrub(reason, 300)
			}
		}
		for _, s := range c.Segments {
			v.Segments = append(v.Segments, SegmentView{s.Start, s.End, s.CropX, s.Moved})
		}
		for _, w := range c.Words {
			v.Words = append(v.Words, WordView{w.Start, w.End, w.Text})
		}
		if out := filepath.Join(work, "out", v.Basename+".mp4"); isFile(out) {
			v.Rendered = out
		}
		if prev := filepath.Join(work, "preview", v.Basename+".mp4"); isFile(prev) {
			v.Preview = prev
		}
		view.Clips = append(view.Clips, v)
	}
	return view, nil
}

// CaptionLineView is one line of a caption, with the words it is made of.
type CaptionLineView struct {
	Words []WordView `json:"words"`
}

// CaptionView is one caption on the clip's own clock, in the lines the
// render draws it in.
type CaptionView struct {
	Start float64           `json:"start"`
	End   float64           `json:"end"`
	Lines []CaptionLineView `json:"lines"`
	// First and Last are when the word the caption begins on and the word
	// it ends on start in the episode, which is what a caption moved by
	// hand is kept against. Nought where there is no such word.
	First float64 `json:"first"`
	Last  float64 `json:"last"`
	// StartMoved and EndMoved say the caption appears or goes where it was
	// put by hand rather than where its words put it.
	StartMoved bool `json:"startMoved,omitempty"`
	EndMoved   bool `json:"endMoved,omitempty"`
}

// CaptionStyleView is the caption look with every measure as a share of the
// frame height, which is how the render scales it too. The app can then
// draw the captions over a picture of any size.
type CaptionStyleView struct {
	Font string `json:"font"`
	// Size is the em size to draw at, which is not the size in the caption
	// style: libass takes that as the height of the face, ascent plus
	// descent, while a browser takes a font size as the em square.
	Size float64 `json:"size"`
	// LineHeight is the space from one line to the next, as a share of the
	// em size.
	LineHeight float64 `json:"lineHeight"`
	// ChosenSize is the size the caption style asks for, which is what the
	// size control shows. It is also the size drawn at: a word too wide for
	// the frame is hyphenated rather than made smaller.
	ChosenSize      float64 `json:"chosenSize"`
	Bold            bool    `json:"bold"`
	MarginV         float64 `json:"marginV"`
	MarginH         float64 `json:"marginH"`
	PadX            float64 `json:"padX"`
	PadY            float64 `json:"padY"`
	Radius          float64 `json:"radius"`
	Primary         string  `json:"primary"`
	Box             string  `json:"box"`
	Highlight       bool    `json:"highlight"`
	HighlightColour string  `json:"highlightColour"`
	// Text and BoxOn are the switches in the captions column: captions at
	// all, and the box behind them. Box keeps its colour either way, which
	// is what the column shows.
	Text  bool `json:"text"`
	BoxOn bool `json:"boxOn"`
}

// CaptionsView is one clip's captions with the look to draw them in.
type CaptionsView struct {
	Captions []CaptionView    `json:"captions"`
	Style    CaptionStyleView `json:"style"`
}

// ClipCaptionsView gives the captions of one clip of a plan, on the clip's
// own clock, in the lines and the look the render uses. It is what the app
// draws over the picture, so that nothing about captions has to be decided
// twice.
func ClipCaptionsView(planPath, clipID string, overrides map[string]any) (*CaptionsView, error) {
	return clipCaptionsView(planPath, clipID, overrides, nil, nil)
}

// DraftCaptionsView gives the captions a clip would have with other pieces,
// while an edge of it or of a cut is being dragged, so the clip timeline
// draws them under the hand rather than when it lets go. Nothing is
// written. The pieces are where the edges are drawn, already snapped, and
// the words are taken from the transcript the way an edit takes them. A
// caption file written by hand is not read, because the edit that lands
// removes it.
func DraftCaptionsView(planPath, clipID string, pieces [][2]float64, t *Transcript,
	overrides map[string]any) (*CaptionsView, error) {
	if err := saneDraft(pieces); err != nil {
		return nil, err
	}
	if t == nil {
		t = &Transcript{}
	}
	return clipCaptionsView(planPath, clipID, overrides, pieces, t)
}

// maxDraftPieces is more pieces than any clip is cut into by hand.
const maxDraftPieces = 256

// saneDraft checks pieces that came from the interface: in order, apart,
// and no longer than a trim may make a clip.
func saneDraft(pieces [][2]float64) error {
	if len(pieces) == 0 || len(pieces) > maxDraftPieces {
		return renderErr("a clip needs between 1 and %d pieces", maxDraftPieces)
	}
	prev := 0.0
	for _, p := range pieces {
		if !isFinite(p[0]) || !isFinite(p[1]) || p[0] < prev || p[1] <= p[0] {
			return renderErr("the pieces of a clip have to be in order and apart")
		}
		prev = p[1]
	}
	if pieces[len(pieces)-1][1]-pieces[0][0] > MaxClipSpan {
		return renderErr("a clip can span at most %s minutes of the episode", fixed(MaxClipSpan/60, 0))
	}
	return nil
}

// WordStops gives words the way a clip's captions split them: a correction
// that reads as two words is two, and a word too wide for a line is its
// hyphenated halves, each with its share of the word's time, the same share
// LayOutCaptions gives them. They are the words that light up one by one,
// so they are where an edge dragged with shift stops, and they are known
// before the edge reaches a word, not only once it is in the clip.
func WordStops(planPath, clipID string, words []Cue, overrides map[string]any) ([]Cue, error) {
	plan, clips, err := LoadClips(planPath)
	if err != nil {
		return nil, err
	}
	var clip *Clip
	for i := range clips {
		if clips[i].ID == clipID || clips[i].Basename() == clipID {
			clip = &clips[i]
			break
		}
	}
	if clip == nil {
		return nil, fmt.Errorf("no clip %s in %s", Scrub(clipID, 60), filepath.Base(planPath))
	}
	r := roomFor(captionStyleOf(plan.CaptionStyle(), *clip, overrides))
	split := SplitCorrected(words)
	var h *hyphenator
	for _, w := range split {
		if !r.fits(w.Text) {
			// The language the clip's captions are read in, as LayOutCaptions
			// reads it off them.
			h = hyphenatorFor(languageOf([]Caption{{Words: SplitCorrected(clip.Words)}}))
			break
		}
	}
	return hyphenate(split, r, h), nil
}

func clipCaptionsView(planPath, clipID string, overrides map[string]any,
	pieces [][2]float64, t *Transcript) (*CaptionsView, error) {
	plan, clips, err := LoadClips(planPath)
	if err != nil {
		return nil, err
	}
	var clip *Clip
	for i := range clips {
		if clips[i].ID == clipID || clips[i].Basename() == clipID {
			clip = &clips[i]
			break
		}
	}
	if clip == nil {
		return nil, fmt.Errorf("no clip %s in %s", Scrub(clipID, 60), filepath.Base(planPath))
	}
	if pieces != nil {
		draft := *clip
		draft.Segments = make([]Segment, len(pieces))
		for i, p := range pieces {
			draft.Segments[i] = Segment{Start: p[0], End: p[1]}
		}
		draft.Words = nil
		for _, w := range t.Words {
			for _, p := range pieces {
				if HoldsWord(p[0], p[1], w) {
					draft.Words = append(draft.Words, Cue{roundTo(w.Start, 3), roundTo(w.End, 3), w.Text})
					break
				}
			}
		}
		clip = &draft
	}

	s := captionStyleOf(plan.CaptionStyle(), *clip, overrides)
	work := filepath.Dir(filepath.Dir(planPath))
	var captions []Caption
	if pieces != nil {
		captions = Captions(*clip, max(8, int(s.MaxChars)), TooWide(s))
	} else {
		captions, err = ClipCaptions(*clip, filepath.Join(work, "captions"), max(8, int(s.MaxChars)),
			TooWide(s))
		if err != nil {
			return nil, err
		}
	}
	return captionsView(*clip, captions, s), nil
}

// ArrivingCaptionsView is the captions of a clip on its way, from the
// pieces and words the plan builder says it keeps, laid out the way a
// written clip's are, in the style of the clip set it goes into. It is
// nil until the builder has them.
func ArrivingCaptionsView(planPath string, u Underway, overrides map[string]any) *CaptionsView {
	if len(u.Pieces) == 0 || len(u.Words) == 0 {
		return nil
	}
	clip := Clip{Words: u.Words}
	for _, p := range u.Pieces {
		clip.Segments = append(clip.Segments, Segment{Start: p[0], End: p[1]})
	}
	var style map[string]any
	if plan, _, err := LoadClips(planPath); err == nil {
		style = plan.CaptionStyle()
	}
	s := captionStyleOf(style, clip, overrides)
	return captionsView(clip, Captions(clip, max(8, int(s.MaxChars)), TooWide(s)), s)
}

// captionStyleOf is the style a clip's captions are drawn in: the plan's,
// the clip's own place for them, and what the app puts over both.
func captionStyleOf(planStyle map[string]any, clip Clip, overrides map[string]any) Style {
	style := map[string]any{}
	for key, value := range clipStyle(planStyle, clip) {
		style[key] = value
	}
	for key, value := range overrides {
		if text, ok := value.(string); ok && text == "" {
			continue
		}
		style[key] = value
	}
	return ResolveStyle(style)
}

// captionsView lays a clip's captions out for the app.
func captionsView(clip Clip, captions []Caption, s Style) *CaptionsView {
	// The lines the render will use, so the picture in the app breaks the
	// caption in the same places.
	laid := LayOutCaptions(captions, s)

	// The render authors every measure against a 1920 pixel tall frame and
	// scales by the real height.
	const authored = 1920.0
	em := FontScale(s.Font)
	view := &CaptionsView{Captions: []CaptionView{}, Style: CaptionStyleView{
		Font: s.Font, Size: s.Size * em / authored, LineHeight: 1 / em,
		ChosenSize: s.Size, Bold: s.Bold != 0,
		MarginV: s.MarginV / authored, MarginH: s.MarginH / authored,
		PadX: s.BoxPadX / authored, PadY: s.BoxPadY / authored,
		Radius: s.Radius / authored, Primary: WebColour(s.Primary),
		Box: WebColour(s.BackColour), Highlight: s.Highlight,
		HighlightColour: s.HighlightWeb(), Text: s.Text, BoxOn: s.Box,
	}}
	if s.BorderStyle != 3 && s.BorderStyle != 4 {
		view.Style.Box = "rgba(0, 0, 0, 0)"
	}
	for _, c := range laid {
		item := CaptionView{Start: c.Start, End: c.End, Lines: []CaptionLineView{}}
		if len(c.Words) > 0 {
			first, last := c.Words[0], c.Words[len(c.Words)-1]
			if w, ok := SaidWord(clip, (first.Start+first.End)/2); ok {
				item.First = w.Start
				item.StartMoved = clip.CaptionTimes[wordKey(w.Start)].Start != nil
			}
			if w, ok := SaidWord(clip, (last.Start+last.End)/2); ok {
				item.Last = w.Start
				item.EndMoved = clip.CaptionTimes[wordKey(w.Start)].End != nil
			}
		}
		for _, line := range c.Lines {
			row := CaptionLineView{Words: []WordView{}}
			for _, w := range line {
				row.Words = append(row.Words, WordView{w.Start, w.End, Scrub(w.Text, 200)})
			}
			item.Lines = append(item.Lines, row)
		}
		view.Captions = append(view.Captions, item)
	}
	return view
}

func twoDigits(n int) string {
	if n < 10 {
		return "0" + itoa(n)
	}
	return itoa(n)
}
