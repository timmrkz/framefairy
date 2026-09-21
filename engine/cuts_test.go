package engine

import (
	"math"
	"os"
	"strings"
	"testing"
)

// A clip is a set of kept stretches, so every cut is a gap between two of
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
	if err := CutClip(path, "01", 12, 13.5, cutsTranscript(), 0.1); err != nil {
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
	if err := CutClip(path, "01", 12, 13.5, tr, 0.1); err != nil {
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
// cutting a stretch out has to take its words with it. The captions are
// built from the words, so a clip whose words still list what was cut
// would burn in a line nobody says.
func TestCuttingTakesTheWordsWithIt(t *testing.T) {
	path := cutsPlanPath(t)
	if err := CutClip(path, "01", 12, 13.5, cutsTranscript(), 0.1); err != nil {
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

	// Now cut the stretch the last two words are spoken in, and they go.
	if err := CutClip(path, "01", 13.95, 16, cutsTranscript(), 0.1); err != nil {
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
	if err := CutClip(path, "01", 12.6, 12.7, cutsTranscript(), 0.1); err != nil {
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
	if err := CutClip(path, "01", 14.2, 15.5, cutsTranscript(), 0.1); err != nil {
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

// A cut wide enough to swallow a piece takes the piece, rather than leaving
// an empty one behind for the render to trip over.
func TestACutThatSwallowsAPieceDropsIt(t *testing.T) {
	path := writePlanAt(t, `{"clips": [{"id": "01", "segments": [
      {"start": 10.0, "end": 11.0}, {"start": 12.0, "end": 13.0},
      {"start": 14.0, "end": 16.0}], "words": []}]}`)
	tr := &Transcript{Words: []Cue{{10, 10.8, "eins"}, {14.2, 15.8, "zwei"}}}
	if err := CutClip(path, "01", 11.5, 13.5, tr, 0.1); err != nil {
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
// one again and the clip plays the stretch it used to leave out.
func TestJoiningACutPutsTheStretchBack(t *testing.T) {
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
	if err := MoveCut(path, "02", 0, 20.65, 24.8, tr, 0.1); err != nil {
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
			return CutClip(p, "01", 12, 12.01, &Transcript{}, 0.1)
		}},
		{"a cut that falls outside the clip", func(p string) error {
			return CutClip(p, "01", 40, 45, tr, 0.1)
		}},
		{"a cut that leaves the clip with nothing", func(p string) error {
			return CutClip(p, "01", 9, 17, tr, 0.1)
		}},
		{"a cut that leaves the clip under a second", func(p string) error {
			return CutClip(p, "02", 20.65, 25.9, tr, 0.1)
		}},
		{"a clip that is not there", func(p string) error {
			return CutClip(p, "99", 12, 13.5, tr, 0.1)
		}},
		{"joining where there is no cut", func(p string) error {
			return JoinCut(p, "02", 25.5, tr)
		}},
		{"a cut number the clip has not got", func(p string) error {
			return MoveCut(p, "02", 4, 21.5, 23, tr, 0.1)
		}},
		{"a cut that swallows the piece before it", func(p string) error {
			return MoveCut(p, "02", 0, 19, 24.8, tr, 0.1)
		}},
		{"a cut that swallows the piece after it", func(p string) error {
			return MoveCut(p, "02", 0, 20.65, 27, tr, 0.1)
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
	if err := CutClip(path, "01", 12, 13.5, cutsTranscript(), 0.1); err != nil {
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
		func() error { return CutClip(path, "01", 12, 13.5, tr, 0.1) },
		func() error { return MoveCut(path, "01", 0, 11.7, 13.8, tr, 0.1) },
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
