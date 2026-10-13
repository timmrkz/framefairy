package main

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"net/http"
	"net/http/httptest"
	"net/url"
	"os"
	"os/exec"
	"path/filepath"
	"strconv"
	"sync"
	"testing"
	"time"

	"framefairy/engine"
	"framefairy/internal/ffmpegtest"
	"framefairy/internal/framewire"
)

// The video preview asks for the captions of the view it was given, by
// its track, and gets the layer libass draws for the frame: ink where the
// words are, more of it when a colour is drafted brighter, nothing for a
// track of another episode or none at all.
func TestTheCaptionsRouteDrawsAViewsCaptions(t *testing.T) {
	// On the real disk: the decoder is a program of its own and reads the
	// episode and the caption file from there.
	svc, mine, view := captionsEpisode(t)
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
	// How large the pill of the word is drawn on it.
	if s, err := strconv.ParseFloat(rec.Header().Get("X-Caption-Scale"), 64); err != nil || s < 1 || s > 1.2 {
		t.Errorf("the captions say the pill is drawn at %q", rec.Header().Get("X-Caption-Scale"))
	}
	// With the word being typed left out.
	typing := url.Values{}
	for k, v := range ask {
		typing[k] = v
	}
	typing.Set("typing", "0 1")
	if rec, l := draw(typing); rec.Code != http.StatusOK || l.W == 0 {
		t.Errorf("with a word being typed the captions answered %d with a layer %dx%d", rec.Code, l.W, l.H)
	}
	// The word with its pill and the rest apart, at rest, which the video
	// preview bounces the word with after a word step: the word is a part
	// of the caption.
	part := url.Values{}
	for k, v := range ask {
		part[k] = v
	}
	for _, p := range []string{"word", "rest"} {
		part.Set("part", p)
		rec, pl := draw(part)
		if rec.Code != http.StatusOK || pl.W == 0 || pl.W > l.W || rec.Header().Get("X-Caption-Scale") != "1.0000" {
			t.Errorf("the %s apart answered %d with a layer %dx%d of %dx%d at %q", p, rec.Code, pl.W, pl.H, l.W, l.H,
				rec.Header().Get("X-Caption-Scale"))
		}
		// Only the word says how it bounces.
		if bounce := rec.Header().Get("X-Caption-Bounce"); (p == "word") != (bounce == "220 0.55 1.15") {
			t.Errorf("the %s apart says it bounces by %q", p, bounce)
		}
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
		"no track":       {"path": {mine}, "track": {"0123"}, "at": {"10.2"}, "w": {"270"}, "h": {"480"}},
		"another path":   {"path": {"/not/mine.mp4"}, "track": {view.Track}, "at": {"10.2"}, "w": {"270"}, "h": {"480"}},
		"no size":        {"path": {mine}, "track": {view.Track}, "at": {"10.2"}},
		"no colour":      {"path": {mine}, "track": {view.Track}, "at": {"10.2"}, "w": {"270"}, "h": {"480"}, "text": {"red"}},
		"no such part":   {"path": {mine}, "track": {view.Track}, "at": {"10.2"}, "w": {"270"}, "h": {"480"}, "part": {"box"}},
		"typing nothing": {"path": {mine}, "track": {view.Track}, "at": {"10.2"}, "w": {"270"}, "h": {"480"}, "typing": {"x"}},
	} {
		if rec, _ := draw(q); rec.Code == http.StatusOK {
			t.Errorf("%s was answered", name)
		}
	}
}

// The caption box frames each word where libass put it, and asks where
// that is by the view's track: for every caption its words in order, each
// inside the short, lower when the captions are drafted lower, none when
// they are off, and nothing for a track of another episode.
func TestTheWordFramesRouteSaysWhereLibassPutTheWords(t *testing.T) {
	// On the real disk: the words are measured by ffmpeg, which reads the
	// caption file from there.
	svc, mine, view := captionsEpisode(t)
	handler := mediaMiddleware(svc.store)(http.NotFoundHandler())
	ask := func(q url.Values) (int, [][]engine.WordFrame) {
		t.Helper()
		rec := httptest.NewRecorder()
		handler.ServeHTTP(rec, httptest.NewRequest("GET", "/frames/captions/words?"+q.Encode(), nil))
		if rec.Code != http.StatusOK {
			return rec.Code, nil
		}
		var frames [][]engine.WordFrame
		if err := json.Unmarshal(rec.Body.Bytes(), &frames); err != nil {
			t.Fatalf("%v in %q", err, rec.Body.String())
		}
		return rec.Code, frames
	}
	base := url.Values{"path": {mine}, "track": {view.Track}}
	with := func(k, v string) url.Values {
		q := url.Values{}
		for key, vs := range base {
			q[key] = vs
		}
		q.Set(k, v)
		return q
	}
	code, frames := ask(base)
	if code != http.StatusOK || len(frames) != len(view.Captions) {
		t.Fatalf("answered %d with %d captions of frames, the view has %d", code, len(frames), len(view.Captions))
	}
	for i, c := range view.Captions {
		n := 0
		for _, line := range c.Lines {
			n += len(line.Words)
		}
		if len(frames[i]) != n {
			t.Fatalf("caption %d has %d words and %d frames", i, n, len(frames[i]))
		}
		for k, f := range frames[i] {
			if f.X < 0 || f.Y < 0 || f.W <= 0 || f.H <= 0 || f.X+f.W > 1 || f.Y+f.H > 1 {
				t.Errorf("caption %d, word %d stands at %+v, not inside the short", i, k, f)
			}
			if k > 0 && frames[i][k-1].Y == f.Y && frames[i][k-1].X+frames[i][k-1].W > f.X+f.W/2 {
				t.Errorf("caption %d, word %d at %+v overlaps the word before it at %+v", i, k, f, frames[i][k-1])
			}
		}
	}
	if _, low := ask(with("y", "100")); len(low) == 0 || low[0][0].Y <= frames[0][0].Y {
		t.Errorf("drafted lower, the first word stands at %+v, not under %+v", low, frames[0][0])
	}
	if code, off := ask(with("textOn", "0")); code != http.StatusOK || len(off) != 0 {
		t.Errorf("switched off, the words still stand at %+v", off)
	}
	for name, q := range map[string]url.Values{
		"no track":     {"path": {mine}, "track": {"0123"}},
		"another path": {"path": {"/not/mine.mp4"}, "track": {view.Track}},
		"no colour":    with("text", "red"),
	} {
		if code, _ := ask(q); code == http.StatusOK {
			t.Errorf("%s was answered", name)
		}
	}
}

// captionsEpisode is an episode of the library with a clip and its words,
// and the caption view of that clip, ready for the video preview to ask
// for its captions.
//
// On the real disk: the decoder is a program of its own and reads the
// episode and the caption file from there.
func captionsEpisode(t *testing.T) (*FrameFairy, string, *engine.CaptionsView) {
	t.Helper()
	ffmpegtest.Need(t)
	needDecoder(t)
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
	return svc, mine, view
}

// The video preview asks for a layer for every frame it plays, a few at
// once, and while a drag drafts colours and the height, and some of those
// asks are dropped half way when the frame they were for is no longer
// shown. Many at once, at two sizes, with drafts and with asks called off,
// while the clip's view is made again: every ask that is not called off
// gets the layer it gets when it is asked alone, and no half written
// caption file is left.
//
// On the real disk: the decoder is a program of its own and reads the
// episode and the caption file from there.
func TestTheCaptionsRouteAnswersManyAtOnce(t *testing.T) {
	svc, mine, view := captionsEpisode(t)
	handler := mediaMiddleware(svc.store)(http.NotFoundHandler())
	type ask struct {
		at, size, draft int
	}
	query := func(a ask) string {
		q := url.Values{"path": {mine}, "track": {view.Track}, "at": {fmt.Sprintf("%.2f", 10.2+0.04*float64(a.at))}}
		q.Set("w", []string{"270", "180"}[a.size])
		q.Set("h", []string{"480", "320"}[a.size])
		switch a.draft {
		case 1:
			q.Set("text", "#ff0000")
		case 2:
			q.Set("y", "120")
		}
		return "/frames/captions?" + q.Encode()
	}
	serve := func(ctx context.Context, a ask) *httptest.ResponseRecorder {
		rec := httptest.NewRecorder()
		handler.ServeHTTP(rec, httptest.NewRequest("GET", query(a), nil).WithContext(ctx))
		return rec
	}
	var asks []ask
	for at := range 3 {
		for size := range 2 {
			for draft := range 3 {
				asks = append(asks, ask{at, size, draft})
			}
		}
	}
	// Each ask alone first, as the answer to hold the rest to.
	alone := map[ask][]byte{}
	for _, a := range asks {
		rec := serve(context.Background(), a)
		if rec.Code != http.StatusOK {
			t.Fatalf("%+v alone answered %d %s", a, rec.Code, rec.Body.String())
		}
		alone[a] = rec.Body.Bytes()
	}
	var wg sync.WaitGroup
	for worker := range 6 {
		wg.Go(func() {
			for round := range len(asks) {
				a := asks[(worker*5+round)%len(asks)]
				if (worker+round)%4 == 3 {
					// Called off half way.
					ctx, cancel := context.WithTimeout(context.Background(), time.Duration(round%3)*time.Millisecond)
					serve(ctx, a)
					cancel()
					continue
				}
				rec := serve(context.Background(), a)
				if rec.Code != http.StatusOK {
					t.Errorf("%+v answered %d %s", a, rec.Code, rec.Body.String())
				} else if !bytes.Equal(rec.Body.Bytes(), alone[a]) {
					t.Errorf("%+v answered otherwise among many than alone", a)
				}
			}
		})
	}
	wg.Go(func() {
		for range 6 {
			if _, err := svc.Captions(mine, filepath.Join(svc.project(mine).LogsDir(), "clips.json"), "01"); err != nil {
				t.Error(err)
			}
		}
	})
	wg.Wait()
	left, err := filepath.Glob(filepath.Join(engine.WorkDir(mine), "captions", "preview", "*.part*"))
	if err != nil || len(left) != 0 {
		t.Errorf("half written caption files were left: %v %v", left, err)
	}
}

// A video whose folder is deleted outside the app is let go as the app
// comes back to the front, and until then the video preview may still ask
// for its captions. The caption file is not written into the folder that
// is gone: it brought the folder back, and the walk "a video whose folder
// is deleted outside the app leaves the list" found it written back.
func TestTheCaptionsOfAVideoWhoseFolderIsGoneWriteNothingBack(t *testing.T) {
	// On the real disk: the decoder is a program of its own and reads the
	// episode and the caption file from there.
	svc, mine, view := captionsEpisode(t)
	handler := mediaMiddleware(svc.store)(http.NotFoundHandler())
	if err := os.RemoveAll(engine.WorkDir(mine)); err != nil {
		t.Fatal(err)
	}
	ask := url.Values{"path": {mine}, "track": {view.Track}, "at": {"10.2"}, "w": {"270"}, "h": {"480"}}
	for _, part := range []string{"", "word", "rest"} {
		if part != "" {
			ask.Set("part", part)
		}
		rec := httptest.NewRecorder()
		handler.ServeHTTP(rec, httptest.NewRequest("GET", "/frames/captions?"+ask.Encode(), nil))
		if rec.Code != http.StatusNotFound {
			t.Errorf("the captions of a video whose folder is gone answered %d, part %q", rec.Code, part)
		}
	}
	if _, err := os.Stat(engine.WorkDir(mine)); !os.IsNotExist(err) {
		t.Errorf("the video's folder was written back: %v", err)
	}
}
