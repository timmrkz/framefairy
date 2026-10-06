package main

import (
	"encoding/binary"
	"encoding/json"
	"math"
	"net/http"
	"net/http/httptest"
	"net/url"
	"os/exec"
	"strings"
	"testing"
)

// A stream the page opens, pulls from and closes: each frame comes once,
// in the order shown, from the moment it was opened at, a pull with skip
// drops what comes before it, and a closed stream is gone.
func TestTheFramesRouteStreamsAnEpisode(t *testing.T) {
	if _, err := exec.LookPath("ffmpeg"); err != nil {
		t.Skip("ffmpeg is not installed")
	}
	svc, mine, _ := library(t)
	// Ten seconds at five frames a second, a picture any ffmpeg writes.
	if out, err := exec.Command("ffmpeg", "-loglevel", "error", "-y", "-f", "lavfi",
		"-i", "testsrc2=s=160x90:r=5:d=10", "-c:v", "mpeg4", "-g", "10", "-f", "mp4", mine).CombinedOutput(); err != nil {
		t.Fatalf("making the episode: %s %s", err, out)
	}
	handler := mediaMiddleware(svc.store)(http.NotFoundHandler())
	get := func(path string, q url.Values) *httptest.ResponseRecorder {
		rec := httptest.NewRecorder()
		handler.ServeHTTP(rec, httptest.NewRequest("GET", path+"?"+q.Encode(), nil))
		return rec
	}
	rec := get("/frames/open", url.Values{"path": {mine}, "from": {"2.1"}, "w": {"64"}, "h": {"36"}})
	var opened struct{ ID string }
	if err := json.NewDecoder(rec.Body).Decode(&opened); err != nil || opened.ID == "" {
		t.Fatalf("open answered %d %q: %v", rec.Code, rec.Body.String(), err)
	}
	defer get("/frames/close", url.Values{"id": {opened.ID}})
	size := 8 + 64*36*3/2
	read := func(q url.Values) []float64 {
		t.Helper()
		q.Set("id", opened.ID)
		rec := get("/frames/read", q)
		if rec.Code != http.StatusOK {
			t.Fatalf("read answered %d", rec.Code)
		}
		body := rec.Body.Bytes()
		if len(body) == 0 && rec.Header().Get("X-Frames-End") != "" {
			t.Fatalf("the stream ended with no frame: %s", rec.Header().Get("X-Frames-Error"))
		}
		if len(body)%size != 0 {
			t.Fatalf("%d bytes is no whole number of frames of %d", len(body), size)
		}
		var ats []float64
		for off := 0; off < len(body); off += size {
			ats = append(ats, math.Float64frombits(binary.LittleEndian.Uint64(body[off:])))
		}
		return ats
	}
	var all []float64
	for len(all) < 4 {
		all = append(all, read(url.Values{"n": {"4"}})...)
	}
	if math.Abs(all[0]-2.2) > 1e-6 {
		t.Errorf("the first frame starts at %.3f, want 2.2, the first from 2.1 on", all[0])
	}
	for i := 1; i < len(all); i++ {
		if math.Abs(all[i]-all[i-1]-0.2) > 1e-6 {
			t.Fatalf("frames at %v, not one every 0.2 s", all)
		}
	}
	// Further on in the same stream, with what comes before dropped.
	last := all[len(all)-1]
	skipped := read(url.Values{"n": {"1"}, "skip": {"5.9"}})
	if len(skipped) != 1 || math.Abs(skipped[0]-6) > 1e-6 {
		t.Errorf("after %.1f, a pull skipping to 5.9 gave %v, want the frame at 6", last, skipped)
	}
	get("/frames/close", url.Values{"id": {opened.ID}})
	if rec := get("/frames/read", url.Values{"id": {opened.ID}, "n": {"1"}}); rec.Code != http.StatusNotFound {
		t.Errorf("a closed stream answered %d", rec.Code)
	}
}

// A stream that has run to the end of the episode says so.
func TestTheFramesRouteSaysWhenAnEpisodeEnds(t *testing.T) {
	if _, err := exec.LookPath("ffmpeg"); err != nil {
		t.Skip("ffmpeg is not installed")
	}
	svc, mine, _ := library(t)
	if out, err := exec.Command("ffmpeg", "-loglevel", "error", "-y", "-f", "lavfi",
		"-i", "testsrc2=s=160x90:r=5:d=1", "-c:v", "mpeg4", "-f", "mp4", mine).CombinedOutput(); err != nil {
		t.Fatalf("making the episode: %s %s", err, out)
	}
	handler := mediaMiddleware(svc.store)(http.NotFoundHandler())
	rec := httptest.NewRecorder()
	handler.ServeHTTP(rec, httptest.NewRequest("GET", "/frames/open?"+url.Values{"path": {mine}, "w": {"32"}, "h": {"18"}}.Encode(), nil))
	var opened struct{ ID string }
	if err := json.NewDecoder(rec.Body).Decode(&opened); err != nil {
		t.Fatal(err)
	}
	frames := 0
	for range 20 {
		rec := httptest.NewRecorder()
		handler.ServeHTTP(rec, httptest.NewRequest("GET", "/frames/read?id="+opened.ID+"&n=16", nil))
		if rec.Header().Get("X-Frames-End") != "" {
			if frames != 5 {
				t.Errorf("the end came after %d frames of a second at five a second", frames)
			}
			return
		}
		frames += rec.Body.Len() / (8 + 32*18*3/2)
	}
	t.Errorf("no end after %d frames", frames)
}

// Where the system has no decoder of its own, opening one says so and the
// page takes ffmpeg's streams. A batch for a decoder nobody opened is
// refused.
func TestTheFramesRouteSaysWhenThereIsNoSystemDecoder(t *testing.T) {
	svc, _, _ := library(t)
	handler := mediaMiddleware(svc.store)(http.NotFoundHandler())
	rec := httptest.NewRecorder()
	handler.ServeHTTP(rec, httptest.NewRequest("POST", "/frames/native?codec=vp09&cw=320&ch=180&w=160&h=90", strings.NewReader("config")))
	if rec.Code != http.StatusNotImplemented {
		t.Errorf("a decoder for VP9 was answered with %d: %q", rec.Code, rec.Body.String())
	}
	rec = httptest.NewRecorder()
	handler.ServeHTTP(rec, httptest.NewRequest("POST", "/frames/decode?id=nobody", strings.NewReader("")))
	if rec.Code != http.StatusNotFound {
		t.Errorf("samples for no decoder were answered with %d", rec.Code)
	}
	rec = httptest.NewRecorder()
	handler.ServeHTTP(rec, httptest.NewRequest("GET", "/frames/native?codec=hvc1&cw=320&ch=180&w=160&h=90", nil))
	if rec.Code != http.StatusMethodNotAllowed {
		t.Errorf("a decoder opened with GET was answered with %d", rec.Code)
	}
}
