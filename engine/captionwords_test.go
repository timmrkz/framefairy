package engine

import (
	"reflect"
	"strings"
	"testing"
)

// An edge of a caption dragged with shift lands on a word, the way an edge
// of the clip and of a cut does: a start where the nearest word starts, an
// end where the nearest word ends. Clip 01 of editablePlan reads "eins
// zwei" from 0 to 1.1 on its clock and "drei vier" from 1.2, with "drei"
// at 1.2 to 1.6 and "vier" from 2.2. Tim dragged a caption's edge with
// shift and it moved a frame at a time, as it does without.
func TestACaptionEdgeDraggedWithShiftLandsOnAWord(t *testing.T) {
	for _, c := range []struct {
		name  string
		word  float64
		edge  string
		hand  float64
		wants string
	}{
		// 2.05 on the clip's clock, nearer where "vier" starts than "drei".
		{"a start dragged in", 12, "start", 12.85, "2.20-3.30 vier"},
		// 0.7, nearer where "eins" ends than "zwei".
		{"an end dragged in", 10.6, "end", 10.7, "0.00-0.50 eins"},
		// 1.15, past the end of "eins zwei". The nearest word ends at 1.0,
		// behind the edge, which the hand went away from, so it stays.
		{"an end the hand takes away from its word", 10.6, "end", 11.95, "0.00-1.10 eins zwei"},
	} {
		t.Run(c.name, func(t *testing.T) {
			fsys, path := editablePlanPath(t)
			tr := editableTranscript()
			view, err := ShapeCaptionTime(fsys, path, "01", c.word, c.edge, c.hand, true, tr, nil)
			if err != nil {
				t.Fatal(err)
			}
			if _, err := SetCaptionTime(fsys, path, "01", c.word, c.edge, c.hand, true, tr, nil); err != nil {
				t.Fatal(err)
			}
			got := shownCaptions(t, fsys, path, tr)
			if !strings.Contains(strings.Join(got, " | "), c.wants) {
				t.Errorf("dragged with shift to %.2f: %q, want %q", c.hand, got, c.wants)
			}
			saved, err := ClipCaptionsView(fsys, path, "01", tr, nil)
			if err != nil {
				t.Fatal(err)
			}
			if !reflect.DeepEqual(view.Captions, saved.Captions) {
				t.Errorf("showed %+v and saved %+v", view.Captions, saved.Captions)
			}
		})
	}
}

// A word an edge took out is a word to land on going back, so a start
// dragged in over a word with shift and back out lands where it started.
func TestACaptionEdgeDraggedBackWithShiftLandsOnTheWordItTookOut(t *testing.T) {
	fsys, path := editablePlanPath(t)
	tr := editableTranscript()
	before := shownCaptions(t, fsys, path, tr)
	if _, err := SetCaptionTime(fsys, path, "01", 12, "start", 12.85, true, tr, nil); err != nil {
		t.Fatal(err)
	}
	// Back to 1.2 on the clip's clock, where "drei" starts.
	if _, err := SetCaptionTime(fsys, path, "01", 12, "start", 12.0, true, tr, nil); err != nil {
		t.Fatal(err)
	}
	if got := shownCaptions(t, fsys, path, tr); !reflect.DeepEqual(got, before) {
		t.Errorf("dragged back with shift: %q, was %q", got, before)
	}
}
