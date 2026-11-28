package main

import (
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"net/url"
	"path/filepath"
	"strconv"
	"testing"

	"github.com/spf13/afero"

	"framefairy/engine"
)

// The video preview asks the dress of a frame by the name of the clip's
// framing the clip list sent it, and is answered the dress the engine
// works out for that frame of that clip: its crop gliding between two
// keyframes, the keyframe on its frame, and on the video the picture as
// filmed. A framing of another video, or none the app sent, is not
// answered.
func TestTheDressRouteAnswersTheDressOfAFrameOfAClip(t *testing.T) {
	t.Parallel()
	svc, mine := memoryLibrary(t, "ep.mp4", "other.mp4")
	video := mine[0]
	source := engine.SourceInfo{Duration: 60, Width: 480, Height: 270, FPSNum: 25, FPSDen: 1}
	svc.probeVideo = func(context.Context, string) (engine.SourceInfo, error) { return source, nil }
	plan := filepath.Join(engine.LogsDir(video), "clips.json")
	if err := afero.WriteFile(svc.store.files, plan, []byte(`{"source": "ep.mp4", "clips": [
	  {"id": "01", "slug": "eins", "segments": [{"start": 10.0, "end": 14.0, "crop_x": 40}], "words": [[10.1, 10.5, "eins"]],
	   "rotations": [{"from": 12.0, "deg": 90}],
	   "crop_marks": [{"at": 10.4, "x": 10, "glide": true}, {"at": 11.6, "x": 300, "glide": true}]}]}`), 0o644); err != nil {
		t.Fatal(err)
	}
	clips, err := svc.Clips(context.Background(), video)
	if err != nil || len(clips) != 1 || clips[0].Framing == "" {
		t.Fatalf("the clips %+v: %v", clips, err)
	}
	clip := clips[0]
	handler := mediaMiddleware(svc)(http.NotFoundHandler())
	ask := func(q url.Values) (*httptest.ResponseRecorder, engine.FrameDress) {
		t.Helper()
		rec := httptest.NewRecorder()
		handler.ServeHTTP(rec, httptest.NewRequest("GET", "/frames/dress?"+q.Encode(), nil))
		var d engine.FrameDress
		if rec.Code == http.StatusOK {
			if err := json.Unmarshal([]byte(rec.Header().Get("X-Frame-Dress")), &d); err != nil {
				t.Fatalf("the dress %q: %v", rec.Header().Get("X-Frame-Dress"), err)
			}
		}
		return rec, d
	}
	q := func(at string, more ...string) url.Values {
		v := url.Values{"path": {video}, "framing": {clip.Framing}, "at": {at}, "w": {"874"}, "h": {"492"}}
		for i := 0; i+1 < len(more); i += 2 {
			v.Set(more[i], more[i+1])
		}
		return v
	}
	for _, at := range []float64{10.0, 10.4, 11.0, 11.6, 12.2, 13.0} {
		rec, d := ask(q(strconv.FormatFloat(at, 'f', 3, 64)))
		want := source.Dress(clip.Segments, engine.Rotations(clip.Rotations), clip.CropMarks, at, 1080, 1920)
		if rec.Code != http.StatusOK || rec.Body.Len() != 0 {
			t.Fatalf("the dress at %v s answered %d with %d bytes", at, rec.Code, rec.Body.Len())
		}
		if d.Left != want.Left || d.Turn != want.Turn || d.CropWidth != want.CropWidth || !d.In || !d.Clip {
			t.Errorf("the frame at %v s is dressed %+v, the engine dresses it %+v", at, d, want)
		}
	}
	if _, d := ask(q("10.4")); d.Mark == nil || d.Mark.X != 10 || d.Movable {
		t.Errorf("the frame of the first keyframe is dressed %+v", d)
	}
	if _, d := ask(q("13.0")); d.Turn != 90 || d.Width != 270 {
		t.Errorf("a frame of the rotated stretch is dressed %+v", d)
	}
	if _, d := ask(q("13.0", "on", "video")); d.Turn != 0 || d.Width != 480 || d.In || d.Clip {
		t.Errorf("a frame of the rotated stretch, on the video, is dressed %+v", d)
	}
	for name, v := range map[string]url.Values{
		"no framing":      q("11.0", "framing", "nothing"),
		"another video":   q("11.0", "path", mine[1]),
		"no size":         {"path": {video}, "framing": {clip.Framing}, "at": {"11.0"}},
		"no moment":       q("NaN"),
		"no such caption": q("11.0", "track", "0123"),
	} {
		if rec, _ := ask(v); rec.Code == http.StatusOK {
			t.Errorf("%s was answered", name)
		}
	}
}
