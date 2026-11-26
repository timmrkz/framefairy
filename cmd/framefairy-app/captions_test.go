package main

import (
	"bytes"
	"context"
	"encoding/binary"
	"encoding/json"
	"fmt"
	"math"
	"net/http"
	"net/http/httptest"
	"net/url"
	"os"
	"path/filepath"
	"reflect"
	"slices"
	"strconv"
	"strings"
	"sync"
	"testing"

	"framefairy/engine"
	"framefairy/internal/ffmpegtest"
	"framefairy/internal/fixtures"
	"framefairy/internal/framewire"

	"github.com/spf13/afero"
)

// The video preview asks for the captions of the view it was given, by
// its track, and gets the layer libass draws for the frame: ink where the
// words are, more of it when a colour is drafted brighter, nothing for a
// track of another episode or none at all.
func TestTheCaptionsRouteDrawsAViewsCaptions(t *testing.T) {
	// On the real disk: the decoder is a program of its own and reads the
	// episode and the caption file from there.
	svc, mine, view := captionsEpisode(t)
	handler := mediaMiddleware(svc)(http.NotFoundHandler())
	draw := func(q url.Values) (*httptest.ResponseRecorder, framewire.Layer) {
		t.Helper()
		rec := httptest.NewRecorder()
		handler.ServeHTTP(rec, httptest.NewRequest("GET", "/frames/dress?"+q.Encode(), nil))
		if rec.Code != http.StatusOK {
			return rec, framewire.Layer{}
		}
		l, err := framewire.ReadLayer(rec.Body.Bytes())
		if err != nil {
			t.Fatal(err)
		}
		return rec, l
	}
	ask := url.Values{"path": {mine}, "framing": {clipFraming(t, svc, mine)}, "track": {view.Track}, "at": {"10.2"}, "w": {"864"}, "h": {"480"}}
	rec, l := draw(ask)
	if rec.Code != http.StatusOK || l.W == 0 || l.H == 0 || l.X+l.W > 270 || l.Y+l.H > 480 {
		t.Fatalf("the captions answered %d %q, a layer at %d,%d of %dx%d", rec.Code, rec.Body.String(), l.X, l.Y, l.W, l.H)
	}
	// How large the pill of the word is drawn on it.
	if s := dressIn(t, rec).Scale; s < 1 || s > 1.2 {
		t.Errorf("the captions say the pill is drawn at %q", dressIn(t, rec).scale())
	}
	// The frame round the word grows with a pill that does, and stays
	// still where no pill is drawn: with the highlight off it grew to 1.15
	// over a word that did not move, found by the second review.
	popped := url.Values{}
	for k, v := range ask {
		popped[k] = v
	}
	grown := ""
	for at := 10.0; at < 12 && grown == ""; at += 0.02 {
		popped.Set("at", fmt.Sprintf("%.2f", at))
		if rec, _ := draw(popped); dressIn(t, rec).scale() > "1.0100" {
			grown = popped.Get("at")
		}
	}
	if grown == "" {
		t.Fatalf("no frame from 10 s to 12 s says its pill is grown")
	}
	popped.Set("highlightOn", "0")
	if rec, _ := draw(popped); rec.Code != http.StatusOK || dressIn(t, rec).scale() != "1.0000" {
		t.Errorf("at %s with the highlight off the captions say the pill is drawn at %q", grown, dressIn(t, rec).scale())
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
		if rec.Code != http.StatusOK || pl.W == 0 || pl.W > l.W || dressIn(t, rec).scale() != "1.0000" {
			t.Errorf("the %s apart answered %d with a layer %dx%d of %dx%d at %q", p, rec.Code, pl.W, pl.H, l.W, l.H,
				dressIn(t, rec).scale())
		}
		// Only the word says how it bounces.
		if bounce := dressIn(t, rec).Bounce; (p == "word") != (bounce != nil && *bounce == [3]float64{220, 0.55, 1.15}) {
			t.Errorf("the %s apart says it bounces by %v", p, bounce)
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
		"no track":       {"path": {mine}, "framing": {clipFraming(t, svc, mine)}, "track": {"0123"}, "at": {"10.2"}, "w": {"864"}, "h": {"480"}},
		"another path":   {"path": {"/not/mine.mp4"}, "framing": {clipFraming(t, svc, mine)}, "track": {view.Track}, "at": {"10.2"}, "w": {"864"}, "h": {"480"}},
		"no size":        {"path": {mine}, "framing": {clipFraming(t, svc, mine)}, "track": {view.Track}, "at": {"10.2"}},
		"no colour":      {"path": {mine}, "framing": {clipFraming(t, svc, mine)}, "track": {view.Track}, "at": {"10.2"}, "w": {"864"}, "h": {"480"}, "text": {"red"}},
		"no such part":   {"path": {mine}, "framing": {clipFraming(t, svc, mine)}, "track": {view.Track}, "at": {"10.2"}, "w": {"864"}, "h": {"480"}, "part": {"box"}},
		"typing nothing": {"path": {mine}, "framing": {clipFraming(t, svc, mine)}, "track": {view.Track}, "at": {"10.2"}, "w": {"864"}, "h": {"480"}, "typing": {"x"}},
	} {
		if rec, _ := draw(q); rec.Code == http.StatusOK {
			t.Errorf("%s was answered", name)
		}
	}
	// A size the wire to the decoder cannot carry is the ask's fault, and
	// is refused before the decoder is asked for it.
	big := url.Values{"path": {mine}, "framing": {clipFraming(t, svc, mine)}, "track": {view.Track}, "at": {"10.2"}, "w": {"7680"}, "h": {"7680"}}
	if rec, _ := draw(big); rec.Code != http.StatusBadRequest {
		t.Errorf("captions at 7680x7680 answered %d, want %d", rec.Code, http.StatusBadRequest)
	}
}

// A frame the caption file draws the same as the frame before is answered
// unchanged by the tag of the layer the video preview has, with nothing
// in its body: between two words of a playing clip that was
// every other frame, issue #896. Wherever it is answered so, the layer
// drawn would have been the layer the video preview has, byte for byte. A
// tag of another size, track or draft is never the layer of a frame. The
// layer is drawn at the canvas's size, and the dress comes alone when
// asked so.
//
// On the real disk: the decoder is a program of its own and reads the
// episode and the caption file from there.
func TestALayerTheVideoPreviewHasIsNotDrawnAgain(t *testing.T) {
	svc, mine, view := captionsEpisode(t)
	handler := mediaMiddleware(svc)(http.NotFoundHandler())
	ask := func(q url.Values, tag string) *httptest.ResponseRecorder {
		t.Helper()
		rec := httptest.NewRecorder()
		asked := url.Values{}
		for k, v := range q {
			asked[k] = v
		}
		if tag != "" {
			asked.Set("have", tag)
		}
		handler.ServeHTTP(rec, httptest.NewRequest("GET", "/frames/dress?"+asked.Encode(), nil))
		if rec.Code != http.StatusOK && rec.Code != http.StatusNoContent {
			t.Fatalf("at %s the captions answered %d %q", q.Get("at"), rec.Code, rec.Body.String())
		}
		return rec
	}
	q := url.Values{"path": {mine}, "framing": {clipFraming(t, svc, mine)}, "track": {view.Track}, "w": {"864"}, "h": {"480"}}
	var had *httptest.ResponseRecorder
	unchanged, changed := 0, 0
	for at := 10.0; at < 11.4; at += 0.02 {
		q.Set("at", fmt.Sprintf("%.2f", at))
		if had == nil {
			had = ask(q, "")
			continue
		}
		rec := ask(q, dressIn(t, had).Tag)
		whole := ask(q, "")
		if rec.Code == http.StatusNoContent {
			unchanged++
			if !bytes.Equal(whole.Body.Bytes(), had.Body.Bytes()) || rec.Body.Len() != 0 {
				t.Fatalf("at %s the layer was answered unchanged, and libass draws another", q.Get("at"))
			}
			continue
		}
		changed++
		had = whole
	}
	// The pill pops on each word's first frames and rests on the rest.
	if unchanged < 20 || changed < 5 {
		t.Errorf("from 10.0 s to 11.4 s %d frames were answered unchanged and %d drawn anew", unchanged, changed)
	}
	q.Set("at", "10.8")
	tag := dressIn(t, ask(q, "")).Tag
	for name, change := range map[string][2]string{
		"another size":  {"w", "870"},
		"another track": {"track", "0123"},
		"another draft": {"text", "#ff0000"},
		"a word typed":  {"typing", "0 1"},
		"a part":        {"part", "rest"},
	} {
		other := url.Values{}
		for k, v := range q {
			other[k] = v
		}
		other.Set(change[0], change[1])
		other.Set("have", tag)
		rec := httptest.NewRecorder()
		handler.ServeHTTP(rec, httptest.NewRequest("GET", "/frames/dress?"+other.Encode(), nil))
		if rec.Code == http.StatusNoContent {
			t.Errorf("%s was answered unchanged by the tag of the layer before", name)
		}
	}
	// The crop moved draws the same captions over the frame: the framing
	// the clip has then is answered unchanged, with its own dress.
	f, _ := svc.previews.framings.get(q.Get("framing"))
	moved := f
	moved.segments = slices.Clone(f.segments)
	x := 3
	moved.segments[0].CropX = &x
	svc.previews.framings.put("moved", moved, nil)
	before := ask(q, "").Header().Get("X-Frame-Dress")
	q.Set("framing", "moved")
	if rec := ask(q, tag); rec.Code != http.StatusNoContent || rec.Header().Get("X-Frame-Dress") == before {
		t.Errorf("the crop moved answered %d with the dress %s, before it %s", rec.Code, rec.Header().Get("X-Frame-Dress"), before)
	}
	// The captions are drawn at the canvas's size where the window says
	// it.
	q.Set("lw", "900")
	q.Set("lh", "500")
	if size := dressIn(t, ask(q, "")).Size; size[1] != 500 {
		t.Errorf("the captions over a canvas of 900x500 were drawn %v", size)
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
	handler := mediaMiddleware(svc)(http.NotFoundHandler())
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
	svc, mine, home := library(t)
	// A caption file the video preview goes on making after its request,
	// the clip's whole file after the captions showing, is written before
	// the test's folder is removed.
	t.Cleanup(func() { svc.store.machine.HoldPreviews(home)() })
	plan := filepath.Join(engine.LogsDir(mine), "clips.json")
	if err := os.MkdirAll(filepath.Dir(plan), 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(plan, []byte(historyPlan), 0o644); err != nil {
		t.Fatal(err)
	}
	fixtures.Copy(t, "picture-24s.mp4", mine)
	heardEpisode(t, svc, mine, 24, [][3]any{{10.1, 10.5, "eins"}, {10.5, 11.2, "respektvoll"}, {20.1, 20.6, "zwei"}})
	view, err := svc.Captions(mine, plan, "01")
	if err != nil || view.Track == "" || len(view.Captions) == 0 {
		t.Fatalf("the view %+v: %v", view, err)
	}
	clipFraming(t, svc, mine)
	return svc, mine, view
}

// framingsOf are the framings clipFraming found, by the video.
var framingsOf sync.Map

// clipFraming is the name of the framing of the clip 01 of a video, as
// the clip list sends it, which the video preview asks the dress of its
// frames by. It is found once for each video, when captionsEpisode makes
// it, and is the same after, whatever a test does to the video's folder.
func clipFraming(t *testing.T, svc *FrameFairy, video string) string {
	if name, ok := framingsOf.Load(video); ok {
		return name.(string)
	}
	t.Helper()
	clips, err := svc.Clips(context.Background(), video)
	if err != nil {
		t.Fatal(err)
	}
	for _, c := range clips {
		if c.ID == "01" && c.Framing != "" {
			framingsOf.Store(video, c.Framing)
			return c.Framing
		}
	}
	t.Fatalf("the clip 01 of %s has no framing in %+v", video, clips)
	return ""
}

// Once the bridge has closed the app, nothing it started writes into a
// folder it leaves: the video preview's caption ask is told the video is
// gone and makes no caption file, and a draft of Report a Problem saves
// nothing. The bridge removes the folders next, and the part of the app
// that went on writing failed the removal with "directory not empty",
// each time one the reset had not named, issue #713.
//
// On the real disk: the decoder is a program of its own and reads the
// episode there.
func TestNothingWritesOnceTheBridgeHasClosedTheApp(t *testing.T) {
	svc, mine, view := captionsEpisode(t)
	svc.report = newReporting(svc.store.machine, filepath.Join(t.TempDir(), "report"))
	handler := mediaMiddleware(svc)(http.NotFoundHandler())
	ask := func() *httptest.ResponseRecorder {
		q := url.Values{"path": {mine}, "framing": {clipFraming(t, svc, mine)}, "track": {view.Track}, "at": {"10.2"}, "w": {"864"}, "h": {"480"}}
		rec := httptest.NewRecorder()
		handler.ServeHTTP(rec, httptest.NewRequest("GET", "/frames/dress?"+q.Encode(), nil))
		return rec
	}
	if rec := ask(); rec.Code != http.StatusOK {
		t.Fatalf("the captions before the app closed answered %d %s", rec.Code, rec.Body.String())
	}
	before := filesUnder(t, engine.WorkDir(mine))
	if !quit(svc) {
		t.Fatal("the app did not stop its work in time")
	}
	// Another colour, so its caption file would be a new one.
	q := url.Values{"path": {mine}, "framing": {clipFraming(t, svc, mine)}, "track": {view.Track}, "at": {"10.2"}, "w": {"864"}, "h": {"480"}, "text": {"#00ff00"}}
	rec := httptest.NewRecorder()
	handler.ServeHTTP(rec, httptest.NewRequest("GET", "/frames/dress?"+q.Encode(), nil))
	if rec.Code != http.StatusNotFound {
		t.Errorf("a caption ask once the app closed answered %d %s", rec.Code, rec.Body.String())
	}
	if err := svc.SaveReportText("typed as the app quit"); err == nil {
		t.Error("a draft of the report was saved once the app closed")
	}
	if after := filesUnder(t, engine.WorkDir(mine)); !reflect.DeepEqual(after, before) {
		t.Errorf("the work folder held %v before the app closed and %v after", before, after)
	}
}

// filesUnder names every file under dir.
//
// On the real disk: it reads the folder the decoder and libass write to.
func filesUnder(t *testing.T, dir string) []string {
	t.Helper()
	var files []string
	err := filepath.WalkDir(dir, func(path string, d os.DirEntry, err error) error {
		if err == nil && !d.IsDir() {
			files = append(files, path)
		}
		return err
	})
	if err != nil {
		t.Fatal(err)
	}
	return files
}

// A request for captions whose time runs out says what it was waiting
// on, the probe or the drawing, the caption file or the decoder, in its
// answer and so in the app's log. Four asks once answered only "context
// deadline exceeded", and nothing said what had held them, issue #687.
// The test ends each request's time at the step it chooses, by its own
// hand, never by the clock.
//
// On the real disk: the decoder is a program of its own and reads the
// episode there.
func TestACaptionRequestOutOfTimeSaysWhatItWaitedOn(t *testing.T) {
	svc, mine, view := captionsEpisode(t)
	var timeUp context.CancelFunc
	svc.previews.within = func(asked context.Context) (context.Context, context.CancelFunc) {
		ctx, cancel := context.WithCancel(asked)
		timeUp = cancel
		return ctx, cancel
	}
	handler := mediaMiddleware(svc)(http.NotFoundHandler())
	for i, c := range []struct {
		step  string
		probe func(ctx context.Context, path string) (engine.SourceInfo, error)
	}{
		{"probing the video", func(ctx context.Context, _ string) (engine.SourceInfo, error) {
			timeUp()
			<-ctx.Done()
			return engine.SourceInfo{}, ctx.Err()
		}},
		{"drawing the captions", func(_ context.Context, path string) (engine.SourceInfo, error) {
			info, err := svc.probeOnDisk(context.Background(), path)
			timeUp()
			return info, err
		}},
	} {
		svc.probeVideo = c.probe
		// Another colour for each, so its caption file is a new one.
		q := url.Values{"path": {mine}, "framing": {clipFraming(t, svc, mine)}, "track": {view.Track}, "at": {"10.2"}, "w": {"864"}, "h": {"480"}, "text": {fmt.Sprintf("#00ff%02d", i)}}
		rec := httptest.NewRecorder()
		handler.ServeHTTP(rec, httptest.NewRequest("GET", "/frames/dress?"+q.Encode(), nil))
		said := rec.Body.String()
		if rec.Code != http.StatusInternalServerError || !strings.HasPrefix(said, c.step+" took more than the 10s a request for captions may take") {
			t.Errorf("the captions out of time while %s answered %d %q", c.step, rec.Code, said)
		}
	}
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
	// No time limit: what this proves is the answers among many, not how
	// long a busy machine takes. A request that waits on something that
	// never answers holds the test until go test's -timeout, which dumps
	// every goroutine, and so says what it waited on, issue #687.
	svc.previews.within = func(asked context.Context) (context.Context, context.CancelFunc) {
		return context.WithCancel(asked)
	}
	handler := mediaMiddleware(svc)(http.NotFoundHandler())
	type ask struct {
		at, size, draft int
	}
	query := func(a ask) string {
		q := url.Values{"path": {mine}, "framing": {clipFraming(t, svc, mine)}, "track": {view.Track}, "at": {fmt.Sprintf("%.2f", 10.2+0.04*float64(a.at))}}
		q.Set("w", []string{"864", "576"}[a.size])
		q.Set("h", []string{"480", "320"}[a.size])
		switch a.draft {
		case 1:
			q.Set("text", "#ff0000")
		case 2:
			q.Set("y", "120")
		}
		return "/frames/dress?" + q.Encode()
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
					// Called off before it asks, or wherever it has come
					// to by the time the cancel runs.
					ctx, cancel := context.WithCancel(context.Background())
					if round%3 == 0 {
						cancel()
					} else {
						go cancel()
					}
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
			if _, err := svc.Captions(mine, filepath.Join(engine.LogsDir(mine), "clips.json"), "01"); err != nil {
				t.Error(err)
			}
		}
	})
	wg.Wait()
	// A colour never asked for before, so its caption file is made while
	// two ask for it, and the first is called off as soon as the second
	// request's goroutine runs. The channels only say that each goroutine
	// has started, not how far its request has come, but the cancel lands
	// while the file is made in most rounds. The second answered 500
	// "context canceled" before, found by the second review. With a wait on the clock between the steps this caught it in
	// about 6 rounds of 40, and without one in about 35. That exact order
	// is proven every time by TestACaptionFileOutlivesTheRequestThatAskedFirst
	// in engine.
	for i := range 40 {
		q := url.Values{"path": {mine}, "framing": {clipFraming(t, svc, mine)}, "track": {view.Track}, "at": {"10.20"}, "w": {"864"}, "h": {"480"},
			"text": {fmt.Sprintf("#%02x8040", 16+i)}}
		ctx, cancel := context.WithCancel(context.Background())
		rec := httptest.NewRecorder()
		var both sync.WaitGroup
		asked := make(chan struct{})
		both.Go(func() {
			close(asked)
			handler.ServeHTTP(httptest.NewRecorder(), httptest.NewRequest("GET", "/frames/dress?"+q.Encode(), nil).WithContext(ctx))
		})
		<-asked
		second := make(chan struct{})
		both.Go(func() {
			close(second)
			handler.ServeHTTP(rec, httptest.NewRequest("GET", "/frames/dress?"+q.Encode(), nil))
		})
		<-second
		cancel()
		both.Wait()
		if rec.Code != http.StatusOK {
			t.Errorf("a new colour asked for while the first ask was called off answered %d %s", rec.Code, rec.Body.String())
		}
	}
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
	handler := mediaMiddleware(svc)(http.NotFoundHandler())
	if err := os.RemoveAll(engine.WorkDir(mine)); err != nil {
		t.Fatal(err)
	}
	ask := url.Values{"path": {mine}, "framing": {clipFraming(t, svc, mine)}, "track": {view.Track}, "at": {"10.2"}, "w": {"864"}, "h": {"480"}}
	for _, part := range []string{"", "word", "rest"} {
		if part != "" {
			ask.Set("part", part)
		}
		rec := httptest.NewRecorder()
		handler.ServeHTTP(rec, httptest.NewRequest("GET", "/frames/dress?"+ask.Encode(), nil))
		if rec.Code != http.StatusNotFound {
			t.Errorf("the captions of a video whose folder is gone answered %d, part %q", rec.Code, part)
		}
	}
	if _, err := os.Stat(engine.WorkDir(mine)); !os.IsNotExist(err) {
		t.Errorf("the video's folder was written back: %v", err)
	}
}

// A drag on the clip timeline asks for the captions' look on every step.
// A video that cannot be read, on a drive that is not plugged in, was
// asked of ffprobe on every one of them, and every one wrote the same
// warning. It is asked once and said once, and asked again only after a
// while, see unprobed.
func TestTheLookOfAVideoThatCannotBeReadIsAskedOnce(t *testing.T) {
	// On the real disk: ffprobe is asked for the video, which is not there.
	// Not parallel: it reads what the package's log wrote.
	svc, mine, _ := library(t)
	_, read := testLog(t)
	plan := filepath.Join(engine.LogsDir(mine), "clips.json")
	asked := 0
	svc.probeVideo = func(ctx context.Context, path string) (engine.SourceInfo, error) {
		asked++
		return svc.probeOnDisk(ctx, path)
	}
	for range 20 {
		if look := svc.captionLook(mine, plan); look.Source.FPSNum != 0 {
			t.Fatalf("a video that is not there has frames: %+v", look.Source)
		}
	}
	if asked != 1 {
		t.Errorf("the video was asked for %d times in a drag", asked)
	}
	said := 0
	for _, l := range read() {
		if l["msg"] == "the video's frames are not known, so its captions are timed on the plan's clock" {
			said++
		}
	}
	if said != 1 {
		t.Errorf("the warning was said %d times in a drag", said)
	}
}

// The drafts of a drag come in the query string, and only what makes a
// caption file is taken from it: a colour as #rrggbb with an opacity
// from 0 to 1, a part on or off as 0 or 1, a height snapped to its step.
// Anything else is refused or left out, so nothing in a query reaches the
// caption file as anything but a colour, a switch or a number.
func TestCaptionDraftsTakeOnlyWhatMakesACaption(t *testing.T) {
	t.Parallel()
	for _, c := range []struct {
		name  string
		query string
		want  map[string]any
		bad   bool
	}{
		{"nothing", "", map[string]any{}, false},
		{"a colour", "text=%23ff0080", map[string]any{"primary": "&H008000FF"}, false},
		{"a colour half seen", "box=%23000000&boxOpacity=0.5", map[string]any{"back_colour": "&H80000000"}, false},
		{"an opacity past 1", "highlight=%23942192&highlightOpacity=7", map[string]any{"highlight_colour": "&H00922194"}, false},
		{"an opacity that is not a number", "text=%23ffffff&textOpacity=x", map[string]any{"primary": "&H00FFFFFF"}, false},
		{"an opacity of NaN", "text=%23ffffff&textOpacity=NaN", map[string]any{"primary": "&H00FFFFFF"}, false},
		{"an opacity without end", "text=%23ffffff&textOpacity=Inf", nil, true},
		{"a colour by name", "text=red", nil, true},
		{"a colour of three", "text=%23fff", nil, true},
		{"a colour with a tag", "box=%23{\\b1}0", nil, true},
		{"a colour and a tag", "highlight=%23ffffff{\\b1}", nil, true},
		{"switched off and on", "textOn=0&boxOn=1", map[string]any{"text": 0.0, "box": 1.0}, false},
		{"switched to something else", "textOn=2&boxOn=yes&highlightOn=-1", map[string]any{}, false},
		{"a height", "y=0.2", map[string]any{"margin_v": engine.SnapCaptionY(0.2)}, false},
		{"a height without end", "y=Inf", map[string]any{}, false},
		{"a height that is not a number", "y=NaN", map[string]any{}, false},
		{"a height far out", "y=1e300", map[string]any{"margin_v": engine.SnapCaptionY(1e300)}, false},
		{"names it does not know", "font=Comic&size=900&margin_v=-5", map[string]any{}, false},
	} {
		q, err := url.ParseQuery(c.query)
		if err != nil {
			t.Fatal(err)
		}
		got, err := captionDrafts(q.Get)
		if (err != nil) != c.bad {
			t.Errorf("%s: %v", c.name, err)
			continue
		}
		if !c.bad && !reflect.DeepEqual(got, c.want) {
			t.Errorf("%s: drafts %v, want %v", c.name, got, c.want)
		}
	}
}

// A step of a drag that draws what the clip as saved draws has the same
// track, and leaves the saved drawing kept for it: a step gets no whole
// caption file, so the clip played after a drag that changed nothing
// would never have one.
func TestAStepThatChangedNothingLeavesTheSavedCaptionsKept(t *testing.T) {
	t.Parallel()
	svc, mine := memoryLibrary(t, "ep.mp4")
	video := mine[0]
	svc.jobs = newQueue(svc.store, func(JobUpdate) {}, func(string) {})
	plan := filepath.Join(engine.LogsDir(video), "clips.json")
	if err := afero.WriteFile(svc.store.files, plan, []byte(`{"source": "ep.mp4", "clips": [
	  {"id": "01", "slug": "eins", "segments": [{"start": 10.0, "end": 14.0}],
	   "words": [[10.1, 10.5, "eins"], [11.0, 11.4, "zwei"], [12.6, 13.0, "drei"]]}]}`), 0o644); err != nil {
		t.Fatal(err)
	}
	saved, err := svc.Captions(video, plan, "01")
	if err != nil {
		t.Fatal(err)
	}
	shaped, err := svc.Shape(video, plan, "01", engine.Gesture{Kind: "trim", Edge: "end", From: 14, To: 14}, "")
	if err != nil {
		t.Fatal(err)
	}
	if shaped.Captions.Track != saved.Track {
		t.Fatalf("the step that changed nothing has the track %q, the clip saved %q", shaped.Captions.Track, saved.Track)
	}
	if kept, ok := svc.previews.tracks.get(saved.Track); !ok || kept.drawing.InHand() {
		t.Errorf("the saved clip's track is kept %v, with a drawing in the hand %v, want the saved drawing", ok, ok && kept.drawing.InHand())
	}
}

// dressIn is the dress an answer of /frames/dress sent in its header.
func dressIn(t *testing.T, rec *httptest.ResponseRecorder) sentDress {
	t.Helper()
	var d sentDress
	if err := json.Unmarshal([]byte(rec.Header().Get("X-Frame-Dress")), &d); err != nil {
		t.Fatalf("the dress %q: %v", rec.Header().Get("X-Frame-Dress"), err)
	}
	return d
}

// scale is how large the pill of the word is drawn, as the window was
// told it once, four decimals.
func (d sentDress) scale() string {
	return strconv.FormatFloat(d.Scale, 'f', 4, 64)
}

// dressedFrame is a frame as the window is sent it, with its dress and
// the layer of captions over it, see writeDress.
type dressedFrame struct {
	at    float64
	dress *sentDress
	layer []byte
}

// dressedFrames reads the frames of an answer, each with a head of head
// bytes, a picture of pixels bytes and its dress after.
func dressedFrames(t *testing.T, body []byte, head, pixels int) []dressedFrame {
	t.Helper()
	var out []dressedFrame
	for len(body) > 0 {
		if len(body) < head+pixels+4 {
			t.Fatalf("%d bytes left are no frame", len(body))
		}
		f := dressedFrame{at: math.Float64frombits(binary.LittleEndian.Uint64(body))}
		body = body[head+pixels:]
		n := int(binary.LittleEndian.Uint32(body))
		if n > 0 {
			f.dress = &sentDress{}
			if err := json.Unmarshal(body[4:4+n], f.dress); err != nil {
				t.Fatalf("the dress %q: %v", body[4:4+n], err)
			}
		}
		body = body[4+n:]
		m := int(binary.LittleEndian.Uint32(body))
		f.layer = body[4 : 4+m]
		body = body[4+m:]
		out = append(out, f)
	}
	return out
}

// A frame the video preview is sent comes with its dress and the
// captions over it, in the same answer, so the window never waits for a
// frame's captions apart from the frame: a look, and each picture of a
// play. A layer the window has is not sent again, the one it names or
// the one the frame before brought, and the frames a word lasts send it
// once. A change of the clip that plays the same pieces, a crop moved
// while the clip plays, dresses the play's frames from then on, with no
// new play.
//
// On the real disk: the decoder is a program of its own and reads the
// episode and the caption file from there.
func TestAFrameComesWithItsDress(t *testing.T) {
	svc, mine, view := captionsEpisode(t)
	handler := mediaMiddleware(svc)(http.NotFoundHandler())
	grid := gridOf(t, mine)
	g, _ := framewire.ReadGrid(grid)
	frameAt := func(at float64) int {
		f := 0
		for f+1 < g.Count && g.At(f+1) <= at {
			f++
		}
		return f
	}
	framing := clipFraming(t, svc, mine)
	const w, h = 64, 36
	look := func(at float64, have string) []dressedFrame {
		t.Helper()
		q := url.Values{"path": {mine}, "w": {fmt.Sprint(w)}, "h": {fmt.Sprint(h)}, "grid": {grid}, "frame": {fmt.Sprint(frameAt(at))},
			"shown": {"-1"}, "framing": {framing}, "track": {view.Track}, "have": {have}}
		rec := httptest.NewRecorder()
		handler.ServeHTTP(rec, httptest.NewRequest("GET", "/frames/look?"+q.Encode(), nil))
		if rec.Code != http.StatusOK {
			t.Fatalf("the look at %v s answered %d", at, rec.Code)
		}
		return dressedFrames(t, rec.Body.Bytes(), 8, w*h*4)
	}
	first := look(10.2, "")
	if len(first) != 1 || first[0].dress == nil || !first[0].dress.Clip || first[0].dress.Tag == "" || len(first[0].layer) == 0 {
		t.Fatalf("the look at 10.2 s came with %+v", first)
	}
	if l, err := framewire.ReadLayer(first[0].layer); err != nil || l.W == 0 {
		t.Fatalf("the layer that came with the frame: %+v, %v", l, err)
	}
	// The layer named is not sent again.
	again := look(10.2, first[0].dress.Tag)
	if len(again) != 1 || again[0].dress == nil || again[0].dress.Tag != first[0].dress.Tag || len(again[0].layer) != 0 {
		t.Errorf("the look naming the layer it has came with %d bytes of layer and the tag %q", len(again[0].layer), again[0].dress.Tag)
	}
	// Before the clip, the frame is the video's, with nothing over it.
	if before := look(2, ""); before[0].dress == nil || before[0].dress.Clip || len(before[0].layer) != 0 {
		t.Errorf("a frame before the clip came with %+v", before[0])
	}

	// A play of the clip: each picture with its dress, a layer sent once
	// for the frames it lasts.
	rec := httptest.NewRecorder()
	q := url.Values{"path": {mine}, "w": {fmt.Sprint(w)}, "h": {fmt.Sprint(h)}, "grid": {grid}, "rate": {"48000"}, "ch": {"2"},
		"program": {framing}, "framing": {framing}, "track": {view.Track}, "at": {"10.0"}}
	handler.ServeHTTP(rec, httptest.NewRequest("GET", "/frames/play?"+q.Encode(), nil))
	var play struct{ Pictures, Sound string }
	if err := json.NewDecoder(rec.Body).Decode(&play); err != nil || play.Pictures == "" {
		t.Fatalf("the play answered %d %q: %v", rec.Code, rec.Body.String(), err)
	}
	defer svc.previews.close(play.Pictures)
	defer svc.previews.close(play.Sound)
	read := func(n int) []dressedFrame {
		t.Helper()
		rec := httptest.NewRecorder()
		handler.ServeHTTP(rec, httptest.NewRequest("GET", fmt.Sprintf("/frames/read?id=%s&n=%d", play.Pictures, n), nil))
		return dressedFrames(t, rec.Body.Bytes(), 40, w*h*4)
	}
	var pictures []dressedFrame
	for len(pictures) < 8 {
		pictures = append(pictures, read(8-len(pictures))...)
	}
	layers := 0
	for i, f := range pictures {
		if f.dress == nil || f.dress.Tag == "" {
			t.Fatalf("picture %d of the play came with %+v", i, f.dress)
		}
		if len(f.layer) > 0 {
			layers++
		} else if i == 0 || pictures[i-1].dress.Tag != f.dress.Tag {
			t.Errorf("picture %d came with no layer, and its tag is not the one before's", i)
		}
	}
	if layers == 0 || layers == len(pictures) {
		t.Errorf("%d of %d pictures came with a layer of their own", layers, len(pictures))
	}
	// The crop moved while it plays: the same pieces, a new framing. The
	// play goes on, and dresses its frames as the new framing says.
	f, _ := svc.previews.framings.get(framing)
	f.segments = slices.Clone(f.segments)
	left := 50
	if x := f.segments[0].CropX; x != nil {
		left += *x
	}
	f.segments[0].CropX = &left
	svc.previews.framings.put("moved", f, nil)
	take := httptest.NewRecorder()
	handler.ServeHTTP(take, httptest.NewRequest("GET", "/frames/play/take?"+url.Values{"id": {play.Pictures}, "program": {"moved"}, "framing": {"moved"},
		"heard": {"0.3"}, "w": {fmt.Sprint(w)}, "h": {fmt.Sprint(h)}, "track": {view.Track}}.Encode(), nil))
	if take.Code != http.StatusNoContent {
		t.Fatalf("a crop moved while it plays answered %d %q", take.Code, take.Body.String())
	}
	was := pictures[len(pictures)-1].dress.Left
	for range 4 {
		for _, p := range read(4) {
			if p.dress.Left != was {
				return
			}
		}
	}
	t.Errorf("the play's frames after the crop moved are still dressed with its left edge at %v", was)
}
