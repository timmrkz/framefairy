package main

import (
	"net/http"
	"net/http/httptest"
	"net/url"
	"os"
	"os/exec"
	"path/filepath"
	"strconv"
	"testing"

	"framefairy/internal/ffmpegtest"
	"framefairy/internal/framewire"
)

// The video preview asks for the captions of the view it was given, by
// its track, and gets the layer libass draws for the frame: ink where the
// words are, more of it when a colour is drafted brighter, nothing for a
// track of another episode or none at all.
func TestTheCaptionsRouteDrawsAViewsCaptions(t *testing.T) {
	ffmpegtest.Need(t)
	needDecoder(t)
	// On the real disk: the decoder is a program of its own and reads the
	// episode and the caption file from there.
	svc, mine, _ := library(t)
	plan := filepath.Join(svc.project(mine).LogsDir(), "clips.json")
	if err := os.MkdirAll(filepath.Dir(plan), 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(plan, []byte(historyPlan), 0o644); err != nil {
		t.Fatal(err)
	}
	if out, err := exec.Command("ffmpeg", "-loglevel", "error", "-y", "-f", "lavfi",
		"-i", "testsrc2=s=320x180:r=25:d=24", "-c:v", "mpeg4", "-f", "mp4", mine).CombinedOutput(); err != nil {
		t.Fatalf("making the episode: %s %s", err, out)
	}
	heardEpisode(t, svc, mine, 24, [][3]any{{10.1, 10.5, "eins"}, {10.5, 11.2, "respektvoll"}, {20.1, 20.6, "zwei"}})
	view, err := svc.Captions(mine, plan, "01")
	if err != nil || view.Track == "" || len(view.Captions) == 0 {
		t.Fatalf("the view %+v: %v", view, err)
	}
	handler := mediaMiddleware(svc.store)(http.NotFoundHandler())
	draw := func(q url.Values) (*httptest.ResponseRecorder, framewire.Layer) {
		t.Helper()
		rec := httptest.NewRecorder()
		handler.ServeHTTP(rec, httptest.NewRequest("GET", "/frames/captions?"+q.Encode(), nil))
		if rec.Code != http.StatusOK {
			return rec, framewire.Layer{}
		}
		l, err := framewire.ReadLayer(rec.Body.Bytes())
		if err != nil {
			t.Fatal(err)
		}
		return rec, l
	}
	ask := url.Values{"path": {mine}, "track": {view.Track}, "at": {"10.2"}, "w": {"270"}, "h": {"480"}}
	rec, l := draw(ask)
	if rec.Code != http.StatusOK || l.W == 0 || l.H == 0 || l.X+l.W > 270 || l.Y+l.H > 480 {
		t.Fatalf("the captions answered %d %q, a layer at %d,%d of %dx%d", rec.Code, rec.Body.String(), l.X, l.Y, l.W, l.H)
	}
	// Lower on the picture where the captions are drafted lower.
	low := url.Values{}
	for k, v := range ask {
		low[k] = v
	}
	low.Set("y", strconv.Itoa(100))
	if _, moved := draw(low); moved.Y <= l.Y {
		t.Errorf("drafted lower, the captions are at %d, not under %d", moved.Y, l.Y)
	}
	// The captions switched off draw nothing.
	off := url.Values{}
	for k, v := range ask {
		off[k] = v
	}
	off.Set("textOn", "0")
	if _, none := draw(off); none.W != 0 {
		t.Errorf("switched off, the captions still draw %dx%d", none.W, none.H)
	}
	for name, q := range map[string]url.Values{
		"no track":     {"path": {mine}, "track": {"0123"}, "at": {"10.2"}, "w": {"270"}, "h": {"480"}},
		"another path": {"path": {"/not/mine.mp4"}, "track": {view.Track}, "at": {"10.2"}, "w": {"270"}, "h": {"480"}},
		"no size":      {"path": {mine}, "track": {view.Track}, "at": {"10.2"}},
		"no colour":    {"path": {mine}, "track": {view.Track}, "at": {"10.2"}, "w": {"270"}, "h": {"480"}, "text": {"red"}},
	} {
		if rec, _ := draw(q); rec.Code == http.StatusOK {
			t.Errorf("%s was answered", name)
		}
	}
}
