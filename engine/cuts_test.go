package engine

import (
	"encoding/json"
	"math"
	"os"
	"path/filepath"
	"strings"
	"testing"
)

// A clip is a set of kept parts, so every cut is a gap between two of
// them. These tests hold the properties that make that safe to hand to a
// person: the framing never moves because a cut was made, the words always
// say what the clip now holds, and an edit that would leave something the
// render cannot make is refused with the plan untouched.

// cutsPlan has one clip in one piece, long enough to cut into, and one clip
// already in two pieces with a cut between them.
const cutsPlan = `{
  "source": "ep.mp4",
  "plan_id": "p1",
  "custom": {"kept": "yes"},
  "clips": [
    {"id": "01", "slug": "eins", "note": "mine",
     "segments": [{"start": 10.0, "end": 16.0, "crop_x": 120, "crop_x_auto": 80}],
     "words": [[10.0, 10.5, "eins"], [15.0, 15.5, "zwei"]]},
    {"id": "02", "slug": "zwei",
     "segments": [{"start": 20.0, "end": 21.0, "crop_x": 300},
                  {"start": 24.0, "end": 26.0, "crop_x": 900}],
     "words": [[20.1, 20.6, "drei"]]}
  ]
}`

func cutsTranscript() *Transcript {
	return &Transcript{Words: []Cue{
		{10, 10.5, "eins"}, {11, 11.5, "zwei"}, {14, 14.5, "drei"}, {15.2, 15.8, "vier"},
		{20.1, 20.6, "fünf"}, {20.7, 21, "sechs"},
		{24.1, 24.6, "sieben"}, {25, 25.6, "acht"},
	}}
}

func cutsPlanPath(t *testing.T) string {
	t.Helper()
	return writePlanAt(t, cutsPlan)
}

func writePlanAt(t *testing.T, body string) string {
	t.Helper()
	dir := t.TempDir() + "/ep.framefairy/logs"
	if err := os.MkdirAll(dir, 0o755); err != nil {
		t.Fatal(err)
	}
	path := dir + "/clips.json"
	if err := os.WriteFile(path, []byte(body), 0o644); err != nil {
		t.Fatal(err)
	}
	return path
}

func clipByID(t *testing.T, path, id string) Clip {
	t.Helper()
	_, clips, err := LoadClips(path)
	if err != nil {
		t.Fatal(err)
	}
	for _, c := range clips {
		if c.ID == id {
			return c
		}
	}
	t.Fatalf("clip %s is not in the plan", id)
	return Clip{}
}

// A cut in the middle of a piece leaves two, and the framing is on both,
// because the shot did not change. This is the whole reason a cut copies
// the piece rather than making a bare one.
func TestACutInsideAPieceLeavesTwoWithTheSameFraming(t *testing.T) {
	path := cutsPlanPath(t)
	if err := CutClip(path, "01", 12, 13.5, cutsTranscript(), 0.1, ToWords); err != nil {
		t.Fatal(err)
	}
	c := clipByID(t, path, "01")
	if len(c.Segments) != 2 {
		t.Fatalf("segments %+v", c.Segments)
	}
	for i, s := range c.Segments {
		if s.CropX == nil || *s.CropX != 120 {
			t.Errorf("piece %d lost its framing: %+v", i, s)
		}
		if !s.Moved {
			t.Errorf("piece %d lost the automatic framing behind it: %+v", i, s)
		}
	}
	if c.Segments[0].Start != 10 || c.Segments[1].End != 16 {
		t.Errorf("the clip's own edges moved: %+v", c.Segments)
	}
	if c.Segments[0].End >= c.Segments[1].Start {
		t.Errorf("the cut takes nothing out: %+v", c.Segments)
	}
}

// The edges of a cut land where the render cuts: after the last word that
// is kept, and before the first word that comes back.
func TestACutSnapsToTheWordsAroundIt(t *testing.T) {
	path := cutsPlanPath(t)
	tr := cutsTranscript()
	// 12 to 13.5 is the pause between "zwei" at 11.5 and "drei" at 14.
	if err := CutClip(path, "01", 12, 13.5, tr, 0.1, ToWords); err != nil {
		t.Fatal(err)
	}
	cuts := ClipCuts(clipByID(t, path, "01"))
	if len(cuts) != 1 {
		t.Fatalf("cuts %+v", cuts)
	}
	if math.Abs(cuts[0].From-11.6) > 0.001 {
		t.Errorf("the cut starts at %v, a tenth after zwei ends at 11.5", cuts[0].From)
	}
	if math.Abs(cuts[0].To-13.9) > 0.001 {
		t.Errorf("the cut ends at %v, a tenth before drei starts at 14", cuts[0].To)
	}
}

// A word is in the clip when the clip holds the moment it is spoken, so
// cutting a part out has to take its words with it. The captions are
// built from the words, so a clip whose words still list what was cut
// would burn in a line nobody says.
func TestCuttingTakesTheWordsWithIt(t *testing.T) {
	path := cutsPlanPath(t)
	if err := CutClip(path, "01", 12, 13.5, cutsTranscript(), 0.1, ToWords); err != nil {
		t.Fatal(err)
	}
	c := clipByID(t, path, "01")
	var said []string
	for _, w := range c.Words {
		said = append(said, w.Text)
	}
	got := strings.Join(said, " ")
	if got != "eins zwei drei vier" {
		t.Fatalf("the clip says %q", got)
	}

	// Now cut the part the last two words are spoken in, and they go.
	if err := CutClip(path, "01", 13.95, 16, cutsTranscript(), 0.1, ToWords); err != nil {
		t.Fatal(err)
	}
	said = nil
	for _, w := range clipByID(t, path, "01").Words {
		said = append(said, w.Text)
	}
	if got := strings.Join(said, " "); got != "eins zwei" {
		t.Errorf("after cutting the end the clip says %q", got)
	}
}

// Taking a pause out is the job, so a cut dropped anywhere inside one takes
// the whole pause rather than the sliver the hand drew. It means a rough
// drag does the right thing, and the edges are there to be moved after.
func TestANudgeInsideAPauseTakesTheWholePause(t *testing.T) {
	path := cutsPlanPath(t)
	// The pause on clip 01 runs from the end of zwei at 11.5 to the start of
	// drei at 14. The drag is a tenth of a second in the middle of it.
	if err := CutClip(path, "01", 12.6, 12.7, cutsTranscript(), 0.1, ToWords); err != nil {
		t.Fatal(err)
	}
	cuts := ClipCuts(clipByID(t, path, "01"))
	if len(cuts) != 1 {
		t.Fatalf("cuts %+v", cuts)
	}
	if math.Abs(cuts[0].From-11.6) > 0.001 || math.Abs(cuts[0].To-13.9) > 0.001 {
		t.Errorf("the cut is %v to %v, not the whole pause", cuts[0].From, cuts[0].To)
	}
}

// A cut may take speech, not only silence, and then it takes whole words.
// A cut that stopped half way through a word would leave a sound nobody
// said, and the captions are built from the words, so it would be seen too.
func TestACutOverSpeechTakesWholeWords(t *testing.T) {
	path := cutsPlanPath(t)
	// The drag starts inside "drei", which runs 14 to 14.5, and ends inside
	// "vier", which runs 15.2 to 15.8. Both go.
	if err := CutClip(path, "01", 14.2, 15.5, cutsTranscript(), 0.1, ToWords); err != nil {
		t.Fatal(err)
	}
	c := clipByID(t, path, "01")
	var said []string
	for _, w := range c.Words {
		said = append(said, w.Text)
	}
	if got := strings.Join(said, " "); got != "eins zwei" {
		t.Errorf("the clip says %q, so a word was left half spoken", got)
	}
	for _, cut := range ClipCuts(c) {
		for _, w := range cutsTranscript().Words {
			if w.Start < cut.To && w.End > cut.From && (w.Start < cut.From || w.End > cut.To) {
				t.Errorf("the cut %v to %v lands inside %q", cut.From, cut.To, w.Text)
			}
		}
	}
}

// Snapping to words is right nearly every time, and the times it is not are
// the reason ToFrames exists: a word clipped a little short, a breath kept.
// Then the edges stay exactly where they were put.
func TestACutToFramesStaysWhereItWasPut(t *testing.T) {
	path := cutsPlanPath(t)
	// 12.34 to 13.21 is inside the pause but nowhere near either word, so
	// snapping would move both edges and this must not.
	if err := CutClip(path, "01", 12.34, 13.21, cutsTranscript(), 0.1, ToFrames); err != nil {
		t.Fatal(err)
	}
	cuts := ClipCuts(clipByID(t, path, "01"))
	if len(cuts) != 1 {
		t.Fatalf("cuts %+v", cuts)
	}
	if cuts[0].From != 12.34 || cuts[0].To != 13.21 {
		t.Errorf("the cut is %v to %v, not where it was put", cuts[0].From, cuts[0].To)
	}
}

// To frames a cut may stop inside a word, which is what makes taking a
// little more or a little less possible at all. The engine must not quietly
// widen it back out to the whole word.
func TestACutToFramesMayStopInsideAWord(t *testing.T) {
	path := cutsPlanPath(t)
	// "drei" runs 14 to 14.5. This clips the last tenth of it and no more.
	if err := CutClip(path, "01", 14.35, 14.9, cutsTranscript(), 0.1, ToFrames); err != nil {
		t.Fatal(err)
	}
	cuts := ClipCuts(clipByID(t, path, "01"))
	if len(cuts) != 1 {
		t.Fatalf("cuts %+v", cuts)
	}
	if cuts[0].From != 14.35 {
		t.Errorf("the cut starts at %v, so it did not stop inside drei", cuts[0].From)
	}
	// A word is in the clip when the clip holds the moment it is spoken,
	// which is the middle of it. That rule does not change because the cut
	// was made to frames: drei is mostly still there, so it is still said.
	var said []string
	for _, w := range clipByID(t, path, "01").Words {
		said = append(said, w.Text)
	}
	if got := strings.Join(said, " "); got != "eins zwei drei vier" {
		t.Errorf("the clip says %q", got)
	}
}

// Where a clipped word stops being said is the middle of it, because that is
// the moment the clip either holds or does not. It is worth pinning: it is
// what decides whether a caption shows a word the render only half plays.
func TestAWordIsSaidWhileItsMiddleIsKept(t *testing.T) {
	// "drei" runs 14 to 14.5, so its middle is at 14.25.
	for _, c := range []struct {
		name string
		from float64
		said bool
	}{
		{"cut after the middle", 14.3, true},
		{"cut before the middle", 14.2, false},
	} {
		t.Run(c.name, func(t *testing.T) {
			path := cutsPlanPath(t)
			if err := CutClip(path, "01", c.from, 14.9, cutsTranscript(), 0.1, ToFrames); err != nil {
				t.Fatal(err)
			}
			has := false
			for _, w := range clipByID(t, path, "01").Words {
				if w.Text == "drei" {
					has = true
				}
			}
			if has != c.said {
				t.Errorf("cutting from %v says drei: %v", c.from, has)
			}
		})
	}
}

// Walking an edge a frame at a time is what the arrow keys do, so a run of
// small moves has to land exactly where the arithmetic says. Snapping would
// pull every one of them back to the same word and the edge would never
// move at all.
func TestACutEdgeWalksAFrameAtATime(t *testing.T) {
	path := cutsPlanPath(t)
	tr := cutsTranscript()
	if err := CutClip(path, "01", 12, 13.5, tr, 0.1, ToWords); err != nil {
		t.Fatal(err)
	}
	// A frame at 25 per second, the step an arrow key takes.
	const frame = 0.04
	at := ClipCuts(clipByID(t, path, "01"))[0]
	want := at.From
	for i := 0; i < 5; i++ {
		want = roundTo(want-frame, 3)
		if err := MoveCut(path, "01", 0, want, at.To, tr, 0.1, ToFrames); err != nil {
			t.Fatalf("step %d: %v", i, err)
		}
		got := ClipCuts(clipByID(t, path, "01"))[0]
		if got.From != want {
			t.Fatalf("step %d put the edge at %v, not %v", i, got.From, want)
		}
		if got.To != at.To {
			t.Errorf("step %d moved the other edge to %v", i, got.To)
		}
	}
	// Five frames back from where the snapping left it.
	if math.Abs(at.From-want-5*frame) > 0.001 {
		t.Errorf("the edge walked from %v to %v", at.From, want)
	}
}

// Everything that makes a clip unrenderable is still refused to frames.
// Turning the snapping off is a choice about where the edges land, not a
// way around the checks.
func TestACutToFramesIsStillChecked(t *testing.T) {
	tr := cutsTranscript()
	refused := []struct {
		name string
		run  func(path string) error
	}{
		{"a cut that takes out nothing", func(p string) error {
			return CutClip(p, "01", 12, 12.01, tr, 0.1, ToFrames)
		}},
		{"a cut that leaves the clip with nothing", func(p string) error {
			return CutClip(p, "01", 9, 17, tr, 0.1, ToFrames)
		}},
		{"a cut that swallows the piece before it", func(p string) error {
			return MoveCut(p, "02", 0, 19.5, 24.8, tr, 0.1, ToFrames)
		}},
		{"a cut that swallows the piece after it", func(p string) error {
			return MoveCut(p, "02", 0, 20.65, 26.5, tr, 0.1, ToFrames)
		}},
	}
	for _, c := range refused {
		t.Run(c.name, func(t *testing.T) {
			path := cutsPlanPath(t)
			before, _ := os.ReadFile(path)
			if err := c.run(path); err == nil {
				t.Fatalf("the edit was accepted")
			}
			after, _ := os.ReadFile(path)
			if string(before) != string(after) {
				t.Errorf("the plan changed:\n%s", after)
			}
		})
	}
}

// A cut wide enough to swallow a piece takes the piece, rather than leaving
// an empty one behind for the render to trip over.
func TestACutThatSwallowsAPieceDropsIt(t *testing.T) {
	path := writePlanAt(t, `{"clips": [{"id": "01", "segments": [
      {"start": 10.0, "end": 11.0}, {"start": 12.0, "end": 13.0},
      {"start": 14.0, "end": 16.0}], "words": []}]}`)
	tr := &Transcript{Words: []Cue{{10, 10.8, "eins"}, {14.2, 15.8, "zwei"}}}
	if err := CutClip(path, "01", 11.5, 13.5, tr, 0.1, ToWords); err != nil {
		t.Fatal(err)
	}
	c := clipByID(t, path, "01")
	if len(c.Segments) != 2 {
		t.Fatalf("segments %+v", c.Segments)
	}
	for _, s := range c.Segments {
		if s.Start >= 12 && s.End <= 13 {
			t.Errorf("the swallowed piece is still there: %+v", c.Segments)
		}
	}
}

// Putting a cut back is the undo of making one, so the two pieces become
// one again and the clip plays the part it used to leave out.
func TestJoiningACutPutsThePartBack(t *testing.T) {
	path := cutsPlanPath(t)
	before := clipByID(t, path, "02")
	if len(ClipCuts(before)) != 1 {
		t.Fatalf("the fixture should start with one cut: %+v", before.Segments)
	}
	if err := JoinCut(path, "02", 22, cutsTranscript()); err != nil {
		t.Fatal(err)
	}
	c := clipByID(t, path, "02")
	if len(c.Segments) != 1 {
		t.Fatalf("segments %+v", c.Segments)
	}
	if c.Segments[0].Start != 20 || c.Segments[0].End != 26 {
		t.Errorf("the joined piece runs %v to %v", c.Segments[0].Start, c.Segments[0].End)
	}
	// The joined piece opens on the shot the first piece opened on.
	if c.Segments[0].CropX == nil || *c.Segments[0].CropX != 300 {
		t.Errorf("the joined piece took the wrong framing: %+v", c.Segments[0])
	}
	if len(ClipCuts(c)) != 0 {
		t.Errorf("there is still a cut: %+v", ClipCuts(c))
	}
}

// Moving a cut is how a suggested cut is made to fit, so both its edges
// move at once and the pieces either side give way.
func TestMovingACutMovesBothItsEdges(t *testing.T) {
	path := cutsPlanPath(t)
	tr := cutsTranscript()
	if err := MoveCut(path, "02", 0, 20.65, 24.8, tr, 0.1, ToWords); err != nil {
		t.Fatal(err)
	}
	c := clipByID(t, path, "02")
	if len(c.Segments) != 2 {
		t.Fatalf("segments %+v", c.Segments)
	}
	cuts := ClipCuts(c)
	if len(cuts) != 1 {
		t.Fatalf("cuts %+v", cuts)
	}
	// The kept side of each edge still lands on a word.
	if math.Abs(cuts[0].From-20.7) > 0.001 {
		t.Errorf("the cut starts at %v, a tenth after fünf ends at 20.6", cuts[0].From)
	}
	if math.Abs(cuts[0].To-24.9) > 0.001 {
		t.Errorf("the cut ends at %v, a tenth before acht starts at 25", cuts[0].To)
	}
	// The clip's own edges are not what a cut moves.
	if c.Segments[0].Start != 20 || c.Segments[1].End != 26 {
		t.Errorf("the clip's edges moved: %+v", c.Segments)
	}
}

// Every one of these would leave a clip the render cannot make, or names
// something that is not there. The app has no save button, so a refused
// edit has to leave the file byte for byte as it was.
func TestARefusedCutLeavesThePlanAlone(t *testing.T) {
	tr := cutsTranscript()
	refused := []struct {
		name string
		run  func(path string) error
	}{
		{"a cut that takes out nothing", func(p string) error {
			// With no words there is no pause for the cut to grow into, so
			// this stays the hair's breadth it was asked for.
			return CutClip(p, "01", 12, 12.01, &Transcript{}, 0.1, ToWords)
		}},
		{"a cut that falls outside the clip", func(p string) error {
			return CutClip(p, "01", 40, 45, tr, 0.1, ToWords)
		}},
		{"a cut that leaves the clip with nothing", func(p string) error {
			return CutClip(p, "01", 9, 17, tr, 0.1, ToWords)
		}},
		{"a cut that leaves the clip under a second", func(p string) error {
			return CutClip(p, "02", 20.65, 25.9, tr, 0.1, ToWords)
		}},
		{"a clip that is not there", func(p string) error {
			return CutClip(p, "99", 12, 13.5, tr, 0.1, ToWords)
		}},
		{"joining where there is no cut", func(p string) error {
			return JoinCut(p, "02", 25.5, tr)
		}},
		{"a cut number the clip has not got", func(p string) error {
			return MoveCut(p, "02", 4, 21.5, 23, tr, 0.1, ToWords)
		}},
		{"a cut that swallows the piece before it", func(p string) error {
			return MoveCut(p, "02", 0, 19, 24.8, tr, 0.1, ToWords)
		}},
		{"a cut that swallows the piece after it", func(p string) error {
			return MoveCut(p, "02", 0, 20.65, 27, tr, 0.1, ToWords)
		}},
	}
	for _, c := range refused {
		t.Run(c.name, func(t *testing.T) {
			path := cutsPlanPath(t)
			before, _ := os.ReadFile(path)
			if err := c.run(path); err == nil {
				t.Fatalf("the edit was accepted")
			}
			after, _ := os.ReadFile(path)
			if string(before) != string(after) {
				t.Errorf("the plan changed:\n%s", after)
			}
		})
	}
}

// A plan holds fields the loader knows nothing about, and a cut must not
// cost them. It is the same promise every other edit makes.
func TestCuttingKeepsWhatTheLoaderDoesNotKnow(t *testing.T) {
	path := cutsPlanPath(t)
	if err := CutClip(path, "01", 12, 13.5, cutsTranscript(), 0.1, ToWords); err != nil {
		t.Fatal(err)
	}
	body, _ := os.ReadFile(path)
	for _, want := range []string{`"custom"`, `"note": "mine"`, `"source": "ep.mp4"`, `"slug": "eins"`} {
		if !strings.Contains(string(body), want) {
			t.Errorf("%s was lost:\n%s", want, body)
		}
	}
}

// A cut is a change to the clip, so the same things follow it as follow a
// trim: the plan's revision goes up, and the captions are built again.
func TestACutRaisesTheRevision(t *testing.T) {
	path := cutsPlanPath(t)
	revision := func() int {
		plan, _, err := LoadClips(path)
		if err != nil {
			t.Fatal(err)
		}
		n, _ := toInt(plan.Raw["revision"])
		return n
	}
	tr := cutsTranscript()
	steps := []func() error{
		func() error { return CutClip(path, "01", 12, 13.5, tr, 0.1, ToWords) },
		func() error { return MoveCut(path, "01", 0, 11.7, 13.8, tr, 0.1, ToWords) },
		func() error { return JoinCut(path, "01", 12.5, tr) },
	}
	for i, step := range steps {
		if err := step(); err != nil {
			t.Fatalf("step %d: %v", i, err)
		}
		if revision() != i+1 {
			t.Errorf("after step %d the revision is %d", i, revision())
		}
	}
}

// ClipCuts is what the timeline draws, so it has to answer the gaps in
// order and say nothing about a clip that is in one piece.
func TestClipCutsAreTheGapsInOrder(t *testing.T) {
	one := Clip{Segments: []Segment{{Start: 0, End: 5}}}
	if got := ClipCuts(one); len(got) != 0 {
		t.Errorf("a clip in one piece has cuts %+v", got)
	}
	three := Clip{Segments: []Segment{
		{Start: 0, End: 5}, {Start: 7, End: 9}, {Start: 12, End: 15}}}
	got := ClipCuts(three)
	want := []Cut{{5, 7}, {9, 12}}
	if len(got) != len(want) {
		t.Fatalf("cuts %+v", got)
	}
	for i := range want {
		if got[i] != want[i] {
			t.Errorf("cut %d is %+v, not %+v", i, got[i], want[i])
		}
	}
}

// The point of the app is that a person's cutting teaches the model, so
// every change to a clip's cuts has to land in the training records as what
// it was. A cut the model did not make, a cut of its that was thrown away
// and a cut of its that was moved are three different lessons, and a clip
// left alone must not look like any of them.
//
// This drives the app's own calls and reads decisions.jsonl back, because
// the fields being right in DiffClip proves nothing about whether the edits
// reach it.
func trainingFixture(t *testing.T) (string, string) {
	t.Helper()
	dir := t.TempDir()
	was := TrainingDir()
	SetTrainingDir(dir)
	t.Cleanup(func() { SetTrainingDir(was) })

	// One clip the model proposed in two pieces, so it has a cut of its own
	// to throw away or move, and room either side to make a new one.
	path := writePlanAt(t, `{
  "source": "ep.mp4",
  "plan_id": "p-train",
  "clips": [
    {"id": "01", "slug": "eins",
     "segments": [{"start": 10.0, "end": 12.0}, {"start": 14.0, "end": 20.0}],
     "words": []}
  ]
}`)
	rec := PlanRecord{
		Schema: TrainingSchema, PlanID: "p-train",
		Candidates: []RecordCandidate{{
			CID: "01", Keep: [][2]int{{1, 4}},
			Segments: [][2]float64{{10, 12}, {14, 20}},
		}},
	}
	if err := appendRecord(filepath.Join(dir, "plans.jsonl"), rec); err != nil {
		t.Fatal(err)
	}
	return path, dir
}

func lastDecision(t *testing.T, dir string) DecisionRecord {
	t.Helper()
	var last DecisionRecord
	found := false
	_ = readRecords(filepath.Join(dir, "decisions.jsonl"), func(line []byte) {
		var d DecisionRecord
		if json.Unmarshal(line, &d) == nil {
			last, found = d, true
		}
	})
	if !found {
		t.Fatal("nothing was recorded")
	}
	return last
}

// The transcript for the fixture. The words sit either side of the model's
// cut and inside the second piece, so a cut can be made where it made none.
func trainingTranscript() *Transcript {
	return &Transcript{Words: []Cue{
		{10.1, 11.8, "eins"},
		{14.1, 15.0, "zwei"}, {17.0, 18.0, "drei"}, {18.2, 19.8, "vier"},
	}}
}

func TestCuttingByHandIsRecordedAsTrainingData(t *testing.T) {
	t.Run("a cut the model did not make", func(t *testing.T) {
		path, dir := trainingFixture(t)
		// Between zwei and drei, inside the model's second piece, where it
		// left the audio running.
		if err := CutClip(path, "01", 15.5, 16.5, trainingTranscript(), 0.1, ToWords); err != nil {
			t.Fatal(err)
		}
		d := lastDecision(t, dir)
		if d.Event != DecisionEdited {
			t.Fatalf("recorded as %q", d.Event)
		}
		if d.Changes == nil || d.Changes.Unchanged {
			t.Fatalf("changes %+v", d.Changes)
		}
		if len(d.Changes.PausesCut) != 1 {
			t.Fatalf("the new cut was not recorded: %+v", d.Changes)
		}
		if len(d.Changes.PausesRestored) != 0 || len(d.Changes.PausesMoved) != 0 {
			t.Errorf("the new cut was counted as something else too: %+v", d.Changes)
		}
	})

	t.Run("a cut of the model's thrown away", func(t *testing.T) {
		path, dir := trainingFixture(t)
		if err := JoinCut(path, "01", 13, trainingTranscript()); err != nil {
			t.Fatal(err)
		}
		d := lastDecision(t, dir)
		if d.Changes == nil || d.Changes.Unchanged {
			t.Fatalf("changes %+v", d.Changes)
		}
		if len(d.Changes.PausesRestored) != 1 {
			t.Fatalf("throwing the cut away was not recorded: %+v", d.Changes)
		}
		if d.Changes.PausesRestored[0] != [2]float64{12, 14} {
			t.Errorf("the wrong cut was recorded: %+v", d.Changes.PausesRestored)
		}
		if len(d.Changes.PausesCut) != 0 || len(d.Changes.PausesMoved) != 0 {
			t.Errorf("it was counted as something else too: %+v", d.Changes)
		}
	})

	t.Run("a cut of the model's moved", func(t *testing.T) {
		path, dir := trainingFixture(t)
		// To frames, because moving a cut a little is exactly what the
		// snapping would undo.
		if err := MoveCut(path, "01", 0, 11.5, 14.6, trainingTranscript(), 0.1, ToFrames); err != nil {
			t.Fatal(err)
		}
		d := lastDecision(t, dir)
		if d.Changes == nil || d.Changes.Unchanged {
			t.Fatalf("a moved cut was recorded as an untouched clip: %+v", d.Changes)
		}
		if len(d.Changes.PausesMoved) != 1 {
			t.Fatalf("the move was not recorded: %+v", d.Changes)
		}
		move := d.Changes.PausesMoved[0]
		if move.Was != [2]float64{12, 14} || move.Now != [2]float64{11.5, 14.6} {
			t.Errorf("the move was recorded as %+v", move)
		}
		if len(d.Changes.PausesCut) != 0 || len(d.Changes.PausesRestored) != 0 {
			t.Errorf("a moved cut was counted as one made and one thrown away: %+v", d.Changes)
		}
	})

	t.Run("a cut thrown away and made again", func(t *testing.T) {
		// Double-clicking a cut puts it back, and double-clicking the same
		// place puts it in again. A clip that ends where it started teaches
		// nothing either way, so it has to read as untouched: the model's
		// cut is back where the model put it, and two records of a hand
		// moving through the same place must not add up to a correction.
		path, dir := trainingFixture(t)
		tr := trainingTranscript()
		if err := JoinCut(path, "01", 13, tr); err != nil {
			t.Fatal(err)
		}
		// Exactly the part that was put back, sent as it stands, which
		// is what the timeline does: snapping something already snapped
		// would move it.
		if err := CutClip(path, "01", 12, 14, tr, 0.1, ToFrames); err != nil {
			t.Fatal(err)
		}
		d := lastDecision(t, dir)
		if d.Changes == nil || !d.Changes.Unchanged {
			t.Errorf("a cut put back and made again reads as an edit: %+v", d.Changes)
		}
		_, clips, err := LoadClips(path)
		if err != nil {
			t.Fatal(err)
		}
		if got := ClipCuts(clips[0]); len(got) != 1 || got[0] != (Cut{From: 12, To: 14}) {
			t.Errorf("the cut came back as %+v", got)
		}
	})

	t.Run("a clip left alone", func(t *testing.T) {
		// The negative case, and the one that matters most: if an untouched
		// clip ever looked edited, every render would teach the model that
		// it got the cut wrong.
		path, dir := trainingFixture(t)
		if err := SetRejected(path, "01", false); err != nil {
			t.Fatal(err)
		}
		d := lastDecision(t, dir)
		if d.Changes == nil || !d.Changes.Unchanged {
			t.Errorf("an untouched clip reads as changed: %+v", d.Changes)
		}
	})
}
