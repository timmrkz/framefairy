package main

import (
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"net/url"
	"path/filepath"
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
	svc, mine := memoryLibrary(t, "ep.mp4")
	video := mine[0]
	source := engine.SourceInfo{Duration: 60, Width: 480, Height: 270, FPSNum: 25, FPSDen: 1}
	svc.probeVideo = func(context.Context, string) (engine.SourceInfo, error) { return source, nil }
	plan := filepath.Join(engine.LogsDir(video), "clips.json")
	if err := afero.WriteFile(svc.store.files, plan, []byte(`{"source": "ep.mp4", "clips": [
	  {"id": "01", "slug": "eins", "segments": [{"start": 10.0, "end": 14.0, "crop_x": 40}], "words": [[10.1, 10.5, "eins"]],
	   "crop_marks": [{"at": 12.0, "x": 300}]}]}`), 0o644); err != nil {
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
	q := url.Values{"path": {c.video}, "framing": {c.clip().Framing}, "at": {at}, "w": {"874"}, "h": {"492"}}
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
// crop stands away from where the app places it and a drag could move
// it, on the keyframe, and never before it, where the crop is the app's,
// or after it, where a drag moves nothing. A click moves the keyframe's
// crop and keeps the keyframe, and the corner then offers to put it
// back on every frame where that would move the crop, until it is put
// back. The user saw the arrow before the keyframe, and its click removed
// the keyframe.
func TestTheCropArrowIsDecidedByGo(t *testing.T) {
	t.Parallel()
	c := newCropArrow(t)
	ctx := context.Background()
	c.corners("as found", [3]string{"", cornerAuto, ""})

	if _, err := c.svc.ResetCrop(ctx, c.video, c.plan, "01", 12.0); err != nil {
		t.Fatal(err)
	}
	c.keyframes("after Automatic crop", 40)
	c.corners("after Automatic crop", [3]string{"", cornerBack, cornerBack})

	if _, err := c.svc.PutCropBack(ctx, c.video, c.plan, "01"); err != nil {
		t.Fatal(err)
	}
	c.keyframes("after Put the crop back", 300)
	c.corners("after Put the crop back", [3]string{"", cornerAuto, ""})
	if _, err := c.svc.PutCropBack(ctx, c.video, c.plan, "01"); err == nil {
		t.Error("the crop was put back a second time")
	}
}

// A second click on Automatic crop, sent before the frame's new dress
// came, finds the crop where the app places it and changes nothing, so
// the way back is still the crop the hand had. A click where the corner
// offers nothing changes nothing either.
func TestASecondAutomaticCropKeepsTheWayBack(t *testing.T) {
	t.Parallel()
	c := newCropArrow(t)
	ctx := context.Background()
	if _, err := c.svc.ResetCrop(ctx, c.video, c.plan, "01", 13.0); err != nil {
		t.Fatal(err)
	}
	c.keyframes("after Automatic crop off the keyframe", 300)
	c.corners("after Automatic crop off the keyframe", [3]string{"", cornerAuto, ""})
	for range 2 {
		if _, err := c.svc.ResetCrop(ctx, c.video, c.plan, "01", 12.0); err != nil {
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
	c.corners("after Automatic crop taken back", [3]string{"", cornerAuto, ""})
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
