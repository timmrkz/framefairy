package main

import (
	"context"
	"encoding/json"
	"maps"
	"net/http"
	"net/http/httptest"
	"net/url"
	"path/filepath"
	"sync"
	"testing"

	"framefairy/engine"

	"github.com/spf13/afero"
)

// cropArrow is a video with the user's clip, a switch keyframe halfway,
// whose crop was moved on it, and a way to ask what the crop frame's
// corner offers on a frame, the way the window asks for a frame's dress.
type cropArrow struct {
	t     *testing.T
	svc   *FrameFairy
	video string
	plan  string
}

func newCropArrow(t *testing.T) cropArrow {
	t.Helper()
	return newCropArrowOf(t, `
	  {"id": "01", "slug": "eins", "segments": [{"start": 10.0, "end": 14.0, "crop_x": 40}], "words": [[10.1, 10.5, "eins"]],
	   "crop_marks": [{"at": 12.0, "x": 300}]}`)
}

// newCropArrowOf is a video with the clips given.
func newCropArrowOf(t *testing.T, clips string) cropArrow {
	t.Helper()
	svc, mine := memoryLibrary(t, "ep.mp4")
	video := mine[0]
	source := engine.SourceInfo{Duration: 60, Width: 480, Height: 270, FPSNum: 25, FPSDen: 1}
	svc.probeVideo = func(context.Context, string) (engine.SourceInfo, error) { return source, nil }
	plan := filepath.Join(engine.LogsDir(video), "clips.json")
	if err := afero.WriteFile(svc.store.files, plan, []byte(`{"source": "ep.mp4", "clips": [`+clips+`]}`), 0o644); err != nil {
		t.Fatal(err)
	}
	return cropArrow{t: t, svc: svc, video: video, plan: plan}
}

// clip is the clip as the window is sent it.
func (c cropArrow) clip() ClipEntry {
	c.t.Helper()
	entry, err := c.svc.clipEntry(context.Background(), c.video, c.plan, "01")
	if err != nil || entry.Framing == "" {
		c.t.Fatalf("the clip %+v: %v", entry, err)
	}
	return entry
}

// dress is the frame at a moment dressed under the clip's framing now.
func (c cropArrow) dress(at string) sentDress {
	c.t.Helper()
	return c.dressOf("01", at)
}

// dressOf is the frame at a moment dressed under the framing of a clip.
func (c cropArrow) dressOf(clip, at string) sentDress {
	c.t.Helper()
	entry, err := c.svc.clipEntry(context.Background(), c.video, c.plan, clip)
	if err != nil {
		c.t.Fatal(err)
	}
	q := url.Values{"path": {c.video}, "framing": {entry.Framing}, "at": {at}, "w": {"874"}, "h": {"492"}}
	rec := httptest.NewRecorder()
	mediaMiddleware(c.svc)(http.NotFoundHandler()).ServeHTTP(rec, httptest.NewRequest("GET", "/frames/dress?"+q.Encode(), nil))
	if rec.Code != http.StatusOK {
		c.t.Fatalf("the dress at %s s answered %d", at, rec.Code)
	}
	var d sentDress
	if err := json.Unmarshal([]byte(rec.Header().Get("X-Frame-Dress")), &d); err != nil {
		c.t.Fatalf("the dress %q: %v", rec.Header().Get("X-Frame-Dress"), err)
	}
	return d
}

// corners says what the corner offers before the keyframe, on it and
// after it, as the window is sent it.
func (c cropArrow) corners(when string, want [3]string) {
	c.t.Helper()
	for i, at := range []string{"11.0", "12.0", "13.0"} {
		if got := c.dress(at).Corner; got != want[i] {
			c.t.Errorf("%s, the frame at %s s offers %q in the crop frame's corner, want %q", when, at, got, want[i])
		}
	}
}

// keyframes is the clip's keyframes, which no arrow may put or remove.
func (c cropArrow) keyframes(when string, x int) {
	c.t.Helper()
	marks := c.clip().CropMarks
	if len(marks) != 1 || marks[0].X != x {
		c.t.Errorf("%s, the clip has the keyframes %+v, want one at 12 s with its crop at %d", when, marks, x)
	}
}

// The corner of the crop frame offers the automatic crop only where the
// crop stands away from where the app places it: on the keyframe and
// after it, where the keyframe holds the crop, and never before it, where
// the crop is the app's. A click moves the keyframe's crop and keeps the
// keyframe, and the corner then offers to put it back on every frame
// where that would move the crop, until it is put back. The user saw the
// arrow before the keyframe, and its click removed the keyframe, and
// wanted it over the whole later half, where the crop is moved.
func TestTheCropArrowIsDecidedByGo(t *testing.T) {
	t.Parallel()
	c := newCropArrow(t)
	ctx := context.Background()
	c.corners("as found", [3]string{"", cornerAuto, cornerAuto})

	if _, err := c.svc.ResetCrop(ctx, c.video, c.plan, "01", 12.0); err != nil {
		t.Fatal(err)
	}
	c.keyframes("after Automatic crop", 40)
	c.corners("after Automatic crop", [3]string{"", cornerBack, cornerBack})

	if _, err := c.svc.PutCropBack(ctx, c.video, c.plan, "01"); err != nil {
		t.Fatal(err)
	}
	c.keyframes("after Put the crop back", 300)
	c.corners("after Put the crop back", [3]string{"", cornerAuto, cornerAuto})

	// After the keyframe the click moves the keyframe's crop too.
	if _, err := c.svc.ResetCrop(ctx, c.video, c.plan, "01", 13.0); err != nil {
		t.Fatal(err)
	}
	c.keyframes("after Automatic crop after the keyframe", 40)
	c.corners("after Automatic crop after the keyframe", [3]string{"", cornerBack, cornerBack})
	if _, err := c.svc.PutCropBack(ctx, c.video, c.plan, "01"); err != nil {
		t.Fatal(err)
	}
	c.keyframes("after Put the crop back after the keyframe", 300)
	// A second click, sent before the frame's new dress came, finds the
	// crop put back and changes nothing, with no message to say so.
	if _, err := c.svc.PutCropBack(ctx, c.video, c.plan, "01"); err != nil {
		t.Errorf("a second click on Put the crop back was refused: %v", err)
	}
	c.keyframes("after a second click on Put the crop back", 300)
	c.corners("after a second click on Put the crop back", [3]string{"", cornerAuto, cornerAuto})
}

// A keyframe placed on the picture turned another way waits, and a drag
// on its frame would take it over, turned the way the picture is now. So
// the corner offers nothing on its frame, and a click there changes
// nothing, while beside it the clip's own crop is brought to the app's
// place and the keyframe keeps its turn and its crop. Found by the review
// of the arrow's move to Go.
func TestAKeyframeThatWaitsIsNotTakenOver(t *testing.T) {
	t.Parallel()
	c := newCropArrowOf(t, `
	  {"id": "01", "slug": "eins", "segments": [{"start": 10.0, "end": 14.0, "crop_x": 100, "crop_x_auto": 40}], "words": [[10.1, 10.5, "eins"]],
	   "crop_marks": [{"at": 12.0, "x": 30, "turn": 90}]}`)
	ctx := context.Background()
	c.corners("as found", [3]string{cornerAuto, "", cornerAuto})
	if _, err := c.svc.ResetCrop(ctx, c.video, c.plan, "01", 12.0); err != nil {
		t.Fatal(err)
	}
	if _, err := c.svc.ResetCrop(ctx, c.video, c.plan, "01", 13.0); err != nil {
		t.Fatal(err)
	}
	clip := c.clip()
	if marks := clip.CropMarks; len(marks) != 1 || marks[0].X != 30 || marks[0].Turn != 90 {
		t.Errorf("after Automatic crop the clip has the keyframes %+v, want the one at 12 s turned 90 with its crop at 30", marks)
	}
	if got := c.dress("13.0").Left; got != 40 {
		t.Errorf("after Automatic crop the crop stands at %v, not the app's 40", got)
	}
}

// Two clips framed alike are two clips: the way back of one is not
// offered on the other, whose click would be refused. Found by the review
// of the arrow's move to Go.
func TestTheWayBackIsOfTheClipItWasTakenOn(t *testing.T) {
	t.Parallel()
	c := newCropArrowOf(t, `
	  {"id": "01", "slug": "eins", "segments": [{"start": 10.0, "end": 14.0, "crop_x": 40}], "words": [[10.1, 10.5, "eins"]],
	   "crop_marks": [{"at": 12.0, "x": 300}]},
	  {"id": "02", "slug": "zwei", "segments": [{"start": 10.0, "end": 14.0, "crop_x": 40}], "words": [[10.1, 10.5, "eins"]],
	   "crop_marks": [{"at": 12.0, "x": 300}]}`)
	ctx := context.Background()
	if _, err := c.svc.ResetCrop(ctx, c.video, c.plan, "01", 12.0); err != nil {
		t.Fatal(err)
	}
	if _, err := c.svc.ResetCrop(ctx, c.video, c.plan, "02", 12.0); err != nil {
		t.Fatal(err)
	}
	if _, err := c.svc.PutCropBack(ctx, c.video, c.plan, "02"); err != nil {
		t.Fatal(err)
	}
	if _, err := c.svc.ResetCrop(ctx, c.video, c.plan, "02", 12.0); err != nil {
		t.Fatal(err)
	}
	for _, clip := range []string{"01", "02"} {
		want := map[string]string{"01": "", "02": cornerBack}[clip]
		if got := c.dressOf(clip, "13.0").Corner; got != want {
			t.Errorf("clip %s offers %q after the keyframe, want %q", clip, got, want)
		}
	}
}

// A second click on Automatic crop, sent before the frame's new dress
// came, finds the crop where the app places it and changes nothing, so
// the way back is still the crop the hand had. A click where the corner
// offers nothing, before the keyframe, changes nothing either.
func TestASecondAutomaticCropKeepsTheWayBack(t *testing.T) {
	t.Parallel()
	c := newCropArrow(t)
	ctx := context.Background()
	if _, err := c.svc.ResetCrop(ctx, c.video, c.plan, "01", 11.0); err != nil {
		t.Fatal(err)
	}
	c.keyframes("after Automatic crop before the keyframe", 300)
	c.corners("after Automatic crop before the keyframe", [3]string{"", cornerAuto, cornerAuto})
	for _, at := range []float64{13.0, 12.0} {
		if _, err := c.svc.ResetCrop(ctx, c.video, c.plan, "01", at); err != nil {
			t.Fatal(err)
		}
	}
	if _, err := c.svc.PutCropBack(ctx, c.video, c.plan, "01"); err != nil {
		t.Fatal(err)
	}
	c.keyframes("after two clicks and Put the crop back", 300)
}

// The way back holds while the clip is as Automatic crop left it. Any
// other change to its crop ends it, so the corner never offers to put
// back what a click would no longer put back, and an undo of the reset
// ends it too, while a redo brings it back.
func TestTheWayBackEndsWithAnyOtherChange(t *testing.T) {
	t.Parallel()
	c := newCropArrow(t)
	ctx := context.Background()
	if _, err := c.svc.ResetCrop(ctx, c.video, c.plan, "01", 12.0); err != nil {
		t.Fatal(err)
	}
	if _, err := c.svc.Undo(c.video); err != nil {
		t.Fatal(err)
	}
	c.keyframes("after Automatic crop taken back", 300)
	c.corners("after Automatic crop taken back", [3]string{"", cornerAuto, cornerAuto})
	if _, err := c.svc.Redo(c.video); err != nil {
		t.Fatal(err)
	}
	c.corners("after Automatic crop done again", [3]string{"", cornerBack, cornerBack})

	// S on another frame changes the clip's crop after it.
	if _, err := c.svc.SetCropMark(ctx, c.video, c.plan, "01", -1, 13.0, 200, false); err != nil {
		t.Fatal(err)
	}
	if got := c.dress("12.0").Corner; got != "" {
		t.Errorf("after S elsewhere, the keyframe's frame offers %q, want nothing", got)
	}
	if _, err := c.svc.PutCropBack(ctx, c.video, c.plan, "01"); err == nil {
		t.Error("the crop was put back after the clip changed")
	}
}

// Automatic crop, S on another frame and the window asking for frames, all
// at once, leave a way back that a click can follow or none: the corner
// offers Put the crop back only where its click puts the crop back. The
// clip is read and the way back kept inside the edit, so S cannot land
// between them. Found by the review of the arrow's move to Go.
func TestTheWayBackHoldsWhileOtherEditsRunAtOnce(t *testing.T) {
	t.Parallel()
	c := newCropArrow(t)
	ctx := context.Background()
	var wg sync.WaitGroup
	wg.Go(func() {
		if _, err := c.svc.ResetCrop(ctx, c.video, c.plan, "01", 12.0); err != nil {
			t.Error(err)
		}
	})
	wg.Go(func() {
		if _, err := c.svc.SetCropMark(ctx, c.video, c.plan, "01", -1, 13.0, 200, false); err != nil {
			t.Error(err)
		}
	})
	wg.Go(func() {
		for range 3 {
			c.dress("13.0")
		}
	})
	wg.Wait()
	offered := c.dress("12.5").Corner == cornerBack
	_, err := c.svc.PutCropBack(ctx, c.video, c.plan, "01")
	if offered != (err == nil) {
		t.Errorf("the corner offers Put the crop back %v, and its click was refused: %v", offered, err)
	}
	if err == nil {
		c.keyframesAt("after Put the crop back", map[float64]int{12: 300, 13: 200})
	}
}

// keyframesAt says the clip's keyframes are at these moments with these
// crops.
func (c cropArrow) keyframesAt(when string, want map[float64]int) {
	c.t.Helper()
	marks := c.clip().CropMarks
	got := map[float64]int{}
	for _, m := range marks {
		got[m.At] = m.X
	}
	if !maps.Equal(got, want) {
		c.t.Errorf("%s, the clip has the keyframes %+v, want %v", when, marks, want)
	}
}

// Past a cut the crop a switch keyframe holds is still the keyframe's, and
// Automatic crop there moves the keyframe the hand put, before the cut,
// not the one the cut carries it on. Found by the review of the arrow's
// move to Go, where the corner offered it and its click did nothing.
func TestTheArrowPastACutMovesTheKeyframeBeforeIt(t *testing.T) {
	t.Parallel()
	c := newCropArrowOf(t, `
	  {"id": "01", "slug": "eins", "segments": [{"start": 10.0, "end": 12.0, "crop_x": 40}, {"start": 13.0, "end": 16.0, "crop_x": 40}],
	   "words": [[10.1, 10.5, "eins"]], "crop_marks": [{"at": 11.0, "x": 300}]}`)
	ctx := context.Background()
	if got := c.dress("14.5").Corner; got != cornerAuto {
		t.Errorf("past the cut the corner offers %q, want %q", got, cornerAuto)
	}
	if _, err := c.svc.ResetCrop(ctx, c.video, c.plan, "01", 14.5); err != nil {
		t.Fatal(err)
	}
	c.keyframesAt("after Automatic crop past the cut", map[float64]int{11: 40})
	if got := c.dress("14.5").Corner; got != cornerBack {
		t.Errorf("after Automatic crop past the cut the corner offers %q, want %q", got, cornerBack)
	}
	if _, err := c.svc.PutCropBack(ctx, c.video, c.plan, "01"); err != nil {
		t.Fatal(err)
	}
	c.keyframesAt("after Put the crop back past the cut", map[float64]int{11: 300})
}
