package engine

import (
	"encoding/json"
	"fmt"
	"os"
	"strings"
	"testing"
)

// A clip a search finds is in pieces that meet at every camera switch, and
// each piece carries the crop of its own shot. Putting back what a gesture
// took out, a cut or an edge, has to bring the switch back with it, or the
// second shot is shown through the first shot's crop.

// shotsPlan is one clip of two shots that meet at a camera switch at
// 12.80, the way ClipSegments writes them.
const shotsPlan = `{"clips": [{"id": "01", "segments": [
  {"start": 9.6, "end": 12.8, "crop_x": 300},
  {"start": 12.8, "end": 16.0, "crop_x": 900}]}]}`

func shotsTranscript() *Transcript {
	return &Transcript{Words: []Cue{
		{9.8, 10.3, "eins"}, {11, 11.5, "zwei"}, {12.2, 12.6, "drei"},
		{13.0, 13.4, "vier"}, {14, 14.5, "fünf"}, {15.2, 15.7, "sechs"},
	}}
}

// shotsAre fails unless a clip is in exactly these pieces, each with its
// crop, and the crop moved by hand where moved says so.
func shotsAre(t *testing.T, path, when string, want []Segment) {
	t.Helper()
	got := clipByID(t, path, "01").Segments
	show := func(s []Segment) string {
		out := ""
		for _, p := range s {
			crop := "centre"
			if p.CropX != nil {
				crop = fmt.Sprint(*p.CropX)
			}
			out += fmt.Sprintf(" [%.3f %.3f %s moved=%v]", p.Start, p.End, crop, p.Moved)
		}
		return out
	}
	if show(got) != show(want) {
		t.Errorf("%s the clip is%s, not%s", when, show(got), show(want))
	}
}

func twoShots() []Segment {
	return []Segment{{Start: 9.6, End: 12.8, CropX: intPtr(300)}, {Start: 12.8, End: 16, CropX: intPtr(900)}}
}

// A cut made across the switch and put back leaves the clip as it was: two
// pieces meeting at 12.80, each with its own crop.
func TestACutPutBackKeepsTheCameraSwitch(t *testing.T) {
	tr := shotsTranscript()
	for _, snap := range []Snap{ToFrames, ToWords} {
		path := writePlanAt(t, shotsPlan)
		g := Gesture{Kind: "cut", From: 12.0, To: 13.5, ToWords: bool(snap), Frame: 0.04}
		if err := Reshape(path, "01", g, tr, 0.1); err != nil {
			t.Fatal(err)
		}
		cut := ClipCuts(clipByID(t, path, "01"))
		if len(cut) != 1 || cut[0].From >= 12.8 || cut[0].To <= 12.8 {
			t.Fatalf("the cut does not cross the switch: %+v", cut)
		}
		// What the hand sees while it puts the cut back is what is saved.
		join := Gesture{Kind: "join", From: (cut[0].From + cut[0].To) / 2}
		shaped, err := ShapeClip(path, "01", join, tr, 0.1)
		if err != nil {
			t.Fatal(err)
		}
		if err := Reshape(path, "01", join, tr, 0.1); err != nil {
			t.Fatal(err)
		}
		shotsAre(t, path, fmt.Sprintf("with snap %v, after the cut is put back", snap), twoShots())
		saved := clipByID(t, path, "01").Segments
		if len(shaped.Pieces) != len(saved) {
			t.Fatalf("shown %+v, saved %+v", shaped.Pieces, saved)
		}
		for i, p := range shaped.Pieces {
			if p.Start != saved[i].Start || p.End != saved[i].End {
				t.Errorf("shown %+v, saved %+v", shaped.Pieces, saved)
			}
		}
	}
}

// A crop placed by hand on the second shot while the cut was there stays
// on the second shot when the cut is put back.
func TestACutPutBackKeepsACropPlacedByHand(t *testing.T) {
	path := writePlanAt(t, shotsPlan)
	tr := shotsTranscript()
	if err := Reshape(path, "01", Gesture{Kind: "cut", From: 12.0, To: 13.5, Frame: 0.04}, tr, 0.1); err != nil {
		t.Fatal(err)
	}
	if err := SetCrop(path, "01", 15, 700); err != nil {
		t.Fatal(err)
	}
	if err := Reshape(path, "01", Gesture{Kind: "join", From: 12.75}, tr, 0.1); err != nil {
		t.Fatal(err)
	}
	shotsAre(t, path, "after the cut is put back", []Segment{
		{Start: 9.6, End: 12.8, CropX: intPtr(300)},
		{Start: 12.8, End: 16, CropX: intPtr(700), Moved: true}})
}

// An edge trimmed past the switch takes the whole other shot with it, and
// putting the edge back where the clip was found brings that shot back,
// with its own crop, from the switch.
func TestAnEdgePutBackKeepsTheCameraSwitch(t *testing.T) {
	tr := shotsTranscript()
	for _, c := range []struct {
		edge     string
		to, back float64
	}{{"end", 12.0, 16.0}, {"start", 13.6, 9.6}} {
		path := writePlanAt(t, shotsPlan)
		trim := Gesture{Kind: "trim", Edge: c.edge, From: c.to, Frame: 0.04}
		if err := Reshape(path, "01", trim, tr, 0.1); err != nil {
			t.Fatal(err)
		}
		if n := len(clipByID(t, path, "01").Segments); n != 1 {
			t.Fatalf("trimming the %s to %v left %d pieces", c.edge, c.to, n)
		}
		back := Gesture{Kind: "trim", Edge: c.edge, From: c.back, Frame: 0.04}
		if err := Reshape(path, "01", back, tr, 0.1); err != nil {
			t.Fatal(err)
		}
		shotsAre(t, path, "after the "+c.edge+" is put back", twoShots())
	}
}

// Pieces of one shot still become one piece when the cut between them is
// put back.
func TestACutPutBackInOneShotLeavesOnePiece(t *testing.T) {
	path := writePlanAt(t, `{"clips": [{"id": "01", "segments": [
      {"start": 9.6, "end": 16.0, "crop_x": 300}]}]}`)
	tr := shotsTranscript()
	if err := Reshape(path, "01", Gesture{Kind: "cut", From: 12.0, To: 13.5, Frame: 0.04}, tr, 0.1); err != nil {
		t.Fatal(err)
	}
	if err := Reshape(path, "01", Gesture{Kind: "join", From: 12.75}, tr, 0.1); err != nil {
		t.Fatal(err)
	}
	shotsAre(t, path, "after the cut is put back", []Segment{{Start: 9.6, End: 16, CropX: intPtr(300)}})
}

// A cut that swallowed a whole shot, a cutaway between two pieces of the
// same camera, brings the cutaway back when it is put back, with its own
// crop, and the camera either side of it keeps its own.
func TestACutPutBackBringsBackAShotItSwallowed(t *testing.T) {
	path := writePlanAt(t, `{"clips": [{"id": "01", "segments": [
      {"start": 9.6, "end": 12.8, "crop_x": 300},
      {"start": 12.8, "end": 13.4, "crop_x": 900},
      {"start": 13.4, "end": 16.0, "crop_x": 300}]}]}`)
	tr := shotsTranscript()
	if err := Reshape(path, "01", Gesture{Kind: "cut", From: 12.0, To: 14.0, Frame: 0.04}, tr, 0.1); err != nil {
		t.Fatal(err)
	}
	if n := len(clipByID(t, path, "01").Segments); n != 2 {
		t.Fatalf("the cut left %d pieces, not 2", n)
	}
	if err := Reshape(path, "01", Gesture{Kind: "join", From: 13}, tr, 0.1); err != nil {
		t.Fatal(err)
	}
	shotsAre(t, path, "after the cut is put back", []Segment{
		{Start: 9.6, End: 12.8, CropX: intPtr(300)},
		{Start: 12.8, End: 13.4, CropX: intPtr(900)},
		{Start: 13.4, End: 16, CropX: intPtr(300)}})
}

// Two pieces that meet have no cut between them, so there is nothing to
// put back or move there, and a gesture that names the switch is refused.
func TestASwitchIsNoCut(t *testing.T) {
	tr := shotsTranscript()
	for _, g := range []Gesture{
		{Kind: "join", From: 12.8},
		{Kind: "move", Index: 0, From: 12.0, To: 13.5, Frame: 0.04},
	} {
		path := writePlanAt(t, shotsPlan)
		if err := Reshape(path, "01", g, tr, 0.1); err == nil {
			t.Errorf("a %s at the switch was made", g.Kind)
		}
		shotsAre(t, path, "after a "+g.Kind+" at the switch", twoShots())
	}
}

// The pieces a clip was found with are read from the plan, which anyone
// can write. Pieces out of order, or not pieces at all, are not read, and
// the pieces the clip has stand in for them.
func TestFoundPiecesThatAreNotPiecesAreNotRead(t *testing.T) {
	tr := shotsTranscript()
	for _, found := range []string{
		`"found_segments": "the whole episode"`,
		`"found_segments": [{"start": 12.8, "end": 16.0}, {"start": 9.6, "end": 12.8}]`,
		`"found_segments": [{"start": 9.6, "end": 9.6}]`,
		`"found_segments": [7]`,
	} {
		path := writePlanAt(t, strings.Replace(shotsPlan, `{"id": "01", `, `{"id": "01", `+found+`, `, 1))
		if err := Reshape(path, "01", Gesture{Kind: "cut", From: 12.0, To: 13.5, Frame: 0.04}, tr, 0.1); err != nil {
			t.Fatal(err)
		}
		cut := ClipCuts(clipByID(t, path, "01"))[0]
		if err := Reshape(path, "01", Gesture{Kind: "join", From: 12.75}, tr, 0.1); err != nil {
			t.Fatal(err)
		}
		// The switch was cut out before anything knew where it was, so the
		// part goes with the shot before it, and each shot keeps its crop.
		shotsAre(t, path, "with "+found, []Segment{
			{Start: 9.6, End: cut.To, CropX: intPtr(300)}, {Start: cut.To, End: 16, CropX: intPtr(900)}})
	}
}

// The pieces a clip was found with are kept the first time anything
// changes it, and never after.
func TestTheFoundPiecesAreKeptOnce(t *testing.T) {
	path := writePlanAt(t, shotsPlan)
	tr := shotsTranscript()
	for _, g := range []Gesture{
		{Kind: "trim", Edge: "end", From: 14, Frame: 0.04},
		{Kind: "trim", Edge: "start", From: 13.6, Frame: 0.04},
	} {
		if err := Reshape(path, "01", g, tr, 0.1); err != nil {
			t.Fatal(err)
		}
	}
	data, err := os.ReadFile(path)
	if err != nil {
		t.Fatal(err)
	}
	var plan struct {
		Clips []struct {
			Found []struct {
				Start float64 `json:"start"`
				End   float64 `json:"end"`
				CropX int     `json:"crop_x"`
			} `json:"found_segments"`
		} `json:"clips"`
	}
	if err := json.Unmarshal(data, &plan); err != nil {
		t.Fatal(err)
	}
	if got := fmt.Sprint(plan.Clips[0].Found); got != "[{9.6 12.8 300} {12.8 16 900}]" {
		t.Errorf("the clip keeps %s as the pieces it was found with", got)
	}
}
