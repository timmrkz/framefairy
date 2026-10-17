package main

import (
	"encoding/json"
	"fmt"
	"io"
	"net"
	"net/http"
	"net/http/httptest"
	"net/url"
	"os"
	"os/exec"
	"path/filepath"
	"testing"

	"github.com/spf13/afero"

	"framefairy/internal/ffmpegtest"
)

// harnessFrames is the Go side's own routes of the picture and the sound,
// mediaMiddleware, for the episodes at paths and nothing else. The preview
// harness answers every other call of the page with its stand-in,
// frontend/preview/wails-stub.ts, but the frames come from here: a second
// decoder written for the harness would drift from the first, and the one
// it had stood in only for the sound, so the harness's video preview
// showed no picture at all. Issue #255.
func harnessFrames(paths []string) (http.Handler, error) {
	st := openStoreIn(afero.NewMemMapFs(), "/config")
	if _, err := st.AddEpisodes(paths); err != nil {
		return nil, err
	}
	return mediaMiddleware(st)(http.NotFoundHandler()), nil
}

// TestHarnessFrames serves harnessFrames until it is stopped, for
// frontend/preview/open.mjs, which starts it and passes the page's
// /frames/ calls on to it. It only runs when asked:
//
//	FRAMEFAIRY_HARNESS_FRAMES=127.0.0.1:0 FRAMEFAIRY_HARNESS_EPISODES=/tmp/a.mp4:/tmp/b.mp4 \
//	  go test -run TestHarnessFrames -timeout 0 ./cmd/framefairy-app
//
// It closes when its standard input does, so a probe that ended leaves no
// Go side behind.
//
// On the real disk: the episode's decoder reads the episodes the harness made.
func TestHarnessFrames(t *testing.T) {
	addr := os.Getenv("FRAMEFAIRY_HARNESS_FRAMES")
	if addr == "" {
		t.Skip("FRAMEFAIRY_HARNESS_FRAMES is not set")
	}
	handler, err := harnessFrames(filepath.SplitList(os.Getenv("FRAMEFAIRY_HARNESS_EPISODES")))
	if err != nil {
		t.Fatal(err)
	}
	server := &http.Server{Handler: handler}
	listener, err := net.Listen("tcp", addr)
	if err != nil {
		t.Fatal(err)
	}
	go func() {
		_, _ = io.Copy(io.Discard, os.Stdin)
		_ = server.Close()
	}()
	fmt.Printf("frames on http://%s\n", listener.Addr())
	if err := server.Serve(listener); err != nil && err != http.ErrServerClosed {
		t.Fatal(err)
	}
}

// The harness's routes give the picture of its episode, and only of it.
//
// On the real disk: ffmpeg makes the episode and the episode's decoder reads it.
func TestTheHarnessGetsItsFramesFromTheDecoder(t *testing.T) {
	ffmpegtest.Need(t)
	needDecoder(t)
	dir := t.TempDir()
	mine := filepath.Join(dir, "harness.mp4")
	if out, err := exec.Command("ffmpeg", "-loglevel", "error", "-y", "-f", "lavfi",
		"-i", "testsrc2=s=160x90:r=5:d=1", "-c:v", "mpeg4", "-f", "mp4", mine).CombinedOutput(); err != nil {
		t.Fatalf("making the episode: %s %s", err, out)
	}
	handler, err := harnessFrames([]string{mine})
	if err != nil {
		t.Fatal(err)
	}
	get := func(route string, q url.Values) *httptest.ResponseRecorder {
		rec := httptest.NewRecorder()
		handler.ServeHTTP(rec, httptest.NewRequest("GET", route+"?"+q.Encode(), nil))
		return rec
	}
	rec := get("/frames/open", url.Values{"path": {mine}, "w": {"32"}, "h": {"18"}, "grid": {gridOf(t, mine)}})
	var opened struct{ ID string }
	if err := json.NewDecoder(rec.Body).Decode(&opened); err != nil || opened.ID == "" {
		t.Fatalf("open answered %d: %v", rec.Code, err)
	}
	defer get("/frames/close", url.Values{"id": {opened.ID}})
	read := get("/frames/read", url.Values{"id": {opened.ID}, "n": {"2"}})
	if read.Code != http.StatusOK || read.Body.Len() == 0 || read.Body.Len()%(8+32*18*4) != 0 {
		t.Fatalf("read answered %d with %d bytes: %s", read.Code, read.Body.Len(), read.Header().Get("X-Frames-Error"))
	}
	// A file beside it that is no episode of the harness is not served.
	other := filepath.Join(dir, "other.mp4")
	if err := os.Link(mine, other); err != nil {
		t.Fatal(err)
	}
	if rec := get("/frames/open", url.Values{"path": {other}, "w": {"32"}, "h": {"18"}, "grid": {gridOf(t, mine)}}); rec.Code != http.StatusNotFound {
		t.Errorf("a file that is not the harness's answered %d, want 404", rec.Code)
	}
}
