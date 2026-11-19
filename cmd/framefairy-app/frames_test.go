package main

import (
	"context"
	"encoding/binary"
	"encoding/json"
	"fmt"
	"math"
	"net/http"
	"net/http/httptest"
	"net/url"
	"os"
	"os/exec"
	"strings"
	"sync"
	"testing"
	"testing/synctest"
	"time"

	"github.com/spf13/afero"

	"framefairy/engine"
	"framefairy/internal/ffmpegtest"
	"framefairy/internal/fixtures"
	"framefairy/internal/framewire"
)

// playOf starts a play of the video from at, of the pieces given as
// servePlay reads them, at w by h, and says the ids of its two streams.
func playOf(t *testing.T, handler http.Handler, path, pieces string, at float64, w, h int) (pictures, sound string) {
	t.Helper()
	rec := httptest.NewRecorder()
	q := url.Values{"path": {path}, "w": {fmt.Sprint(w)}, "h": {fmt.Sprint(h)}, "grid": {gridOf(t, path)}, "rate": {"48000"}, "ch": {"2"},
		"pieces": {pieces}, "at": {fmt.Sprint(at)}}
	handler.ServeHTTP(rec, httptest.NewRequest("GET", "/frames/play?"+q.Encode(), nil))
	var play struct{ Pictures, Sound string }
	if err := json.NewDecoder(rec.Body).Decode(&play); err != nil || play.Pictures == "" || play.Sound == "" {
		t.Fatalf("the play answered %d %q: %v", rec.Code, rec.Body.String(), err)
	}
	t.Cleanup(func() {
		for _, id := range []string{play.Pictures, play.Sound} {
			handler.ServeHTTP(httptest.NewRecorder(), httptest.NewRequest("GET", "/frames/close?id="+id, nil))
		}
	})
	return play.Pictures, play.Sound
}

// A play's frames are pulled and the stream closed: each frame comes once,
// in the order shown, from the frame that holds the moment the play
// starts at, and a closed stream is gone.
func TestTheFramesRouteStreamsAPlay(t *testing.T) {
	ffmpegtest.Need(t)
	needDecoder(t)
	svc, mine, _ := library(t)
	fixtures.Copy(t, "small-10s.mp4", mine)
	handler := mediaMiddleware(svc)(http.NotFoundHandler())
	get := func(path string, q url.Values) *httptest.ResponseRecorder {
		rec := httptest.NewRecorder()
		handler.ServeHTTP(rec, httptest.NewRequest("GET", path+"?"+q.Encode(), nil))
		return rec
	}
	pictures, _ := playOf(t, handler, mine, "0:10", 2.1, 64, 36)
	size := 32 + 64*36*4
	var begins []float64
	for len(begins) < 4 {
		rec := get("/frames/read", url.Values{"id": {pictures}, "n": {"4"}})
		if rec.Code != http.StatusOK {
			t.Fatalf("read answered %d", rec.Code)
		}
		if light := rec.Header().Get("X-Frames-Light"); light != "" {
			t.Errorf("a pull of standard video says its light is %q", light)
		}
		body := rec.Body.Bytes()
		if len(body) == 0 && rec.Header().Get("X-Frames-End") != "" {
			t.Fatalf("the stream ended with no frame: %s", rec.Header().Get("X-Frames-Error"))
		}
		if len(body)%size != 0 {
			t.Fatalf("%d bytes is no whole number of frames of %d", len(body), size)
		}
		for off := 0; off < len(body); off += size {
			begins = append(begins, math.Float64frombits(binary.LittleEndian.Uint64(body[off+16:])))
		}
	}
	// The frame that holds 2.1 s, on the grid of the short.
	if math.Abs(begins[0]-2.0) > 1e-6 {
		t.Errorf("the first frame begins at %.3f, want 2.0, the one that holds 2.1", begins[0])
	}
	for i := 1; i < len(begins); i++ {
		if math.Abs(begins[i]-begins[i-1]-0.2) > 1e-6 {
			t.Fatalf("frames begin at %v, not one every 0.2 s", begins)
		}
	}
	get("/frames/close", url.Values{"id": {pictures}})
	if rec := get("/frames/read", url.Values{"id": {pictures}, "n": {"1"}}); rec.Code != http.StatusNotFound {
		t.Errorf("a closed stream answered %d", rec.Code)
	}
}

// An HDR episode's frames say their light with every pull, so the page
// draws them as HDR, and a standard one's say nothing, see
// framewire.Picture. Step 4 of 2.156.
func TestTheFramesRouteSaysAnHDREpisodesLight(t *testing.T) {
	ffmpegtest.Need(t)
	needDecoder(t)
	svc, mine, _ := library(t)
	// The tags are what says HDR, so any picture tagged HLG will do.
	fixtures.Copy(t, "small-hlg.mp4", mine)
	handler := mediaMiddleware(svc)(http.NotFoundHandler())
	pictures, _ := playOf(t, handler, mine, "0:1", 0, 64, 36)
	for range 2 {
		rec := httptest.NewRecorder()
		handler.ServeHTTP(rec, httptest.NewRequest("GET", "/frames/read?id="+pictures+"&n=2", nil))
		if rec.Code != http.StatusOK || rec.Body.Len() == 0 {
			t.Fatalf("read answered %d with %d bytes: %s", rec.Code, rec.Body.Len(), rec.Header().Get("X-Frames-Error"))
		}
		if rec.Body.Len()%(32+64*36*4) != 0 {
			t.Errorf("%d bytes is no whole number of frames of 4 bytes a pixel", rec.Body.Len())
		}
		if light := rec.Header().Get("X-Frames-Light"); light != "hlg" {
			t.Errorf("a pull of an HLG episode says its light is %q, want hlg", light)
		}
	}
}

// A play that has run to its end says so.
func TestTheFramesRouteSaysWhenAPlayEnds(t *testing.T) {
	ffmpegtest.Need(t)
	needDecoder(t)
	svc, mine, _ := library(t)
	fixtures.Copy(t, "small-1s.mp4", mine)
	handler := mediaMiddleware(svc)(http.NotFoundHandler())
	pictures, _ := playOf(t, handler, mine, "0:1", 0, 32, 18)
	frames := 0
	for range 20 {
		rec := httptest.NewRecorder()
		handler.ServeHTTP(rec, httptest.NewRequest("GET", "/frames/read?id="+pictures+"&n=16", nil))
		frames += rec.Body.Len() / (32 + 32*18*4)
		if rec.Header().Get("X-Frames-End") != "" {
			if frames != 5 {
				t.Errorf("the end came after %d frames of a second at five a second", frames)
			}
			return
		}
	}
	t.Errorf("no end after %d frames", frames)
}

// A stream the Go side stops, to make room for newer ones or because it
// stood unused, says it was closed, not that the play ended, so the page
// starts the play again. Taken for the end, a play paused for long ended
// as the space bar went on with it.
func TestTheFramesRouteSaysAStoppedStreamWasClosed(t *testing.T) {
	ffmpegtest.Need(t)
	needDecoder(t)
	svc, mine, _ := library(t)
	fixtures.Copy(t, "small-10s.mp4", mine)
	handler := mediaMiddleware(svc)(http.NotFoundHandler())
	pictures, _ := playOf(t, handler, mine, "0:10", 0, 32, 18)
	// Held by a pull that is on its way as the stream is stopped.
	s := svc.previews.get(pictures)
	if s == nil {
		t.Fatal("the stream is not open")
	}
	svc.previews.close(pictures)
	<-s.done
	read := httptest.NewRecorder()
	writeFrames(read, httptest.NewRequest("GET", "/frames/read?id="+pictures, nil), s, 4)
	if read.Header().Get("X-Frames-Closed") == "" || read.Header().Get("X-Frames-Error") != "" {
		t.Errorf("a stopped stream answered %v, want it said closed and no error", read.Header())
	}
	// A pull after it is gone finds nothing, which the page takes the same way.
	late := httptest.NewRecorder()
	handler.ServeHTTP(late, httptest.NewRequest("GET", "/frames/read?id="+pictures+"&n=1", nil))
	if late.Code != http.StatusNotFound {
		t.Errorf("a pull of a closed stream answered %d, want 404", late.Code)
	}
}

// There is one way to the frames, the episode's decoder: the routes of
// the Mac's own decoder, which the page fed with the file's samples, are
// gone with it, and so are the streams the window once worked a play out
// of, since a play is the Go side's. Plan row 2.156, step 3 of
// docs/VIDEO-PREVIEW.md, and the decision record of the play.
func TestTheFramesRouteHasNoSecondDecoder(t *testing.T) {
	svc, _, _ := library(t)
	handler := mediaMiddleware(svc)(http.NotFoundHandler())
	for _, route := range []string{"/frames/native?codec=hvc1&cw=320&ch=180&w=160&h=90", "/frames/decode?id=nobody", "/frames/open?path=x", "/frames/sound?path=x"} {
		rec := httptest.NewRecorder()
		handler.ServeHTTP(rec, httptest.NewRequest("POST", route, strings.NewReader("config")))
		if rec.Code != http.StatusNotFound {
			t.Errorf("%s answered %d, want 404", route, rec.Code)
		}
	}
}

// A play's sound, put together, is the video's sound from where it
// starts, as ffmpeg decodes it, to its end, faded in and out at the
// piece's edges the way the render does.
func TestThePlaysSoundIsTheVideosSound(t *testing.T) {
	ffmpegtest.Need(t)
	needDecoder(t)
	svc, mine, _ := library(t)
	fixtures.Copy(t, "small-4s.mp4", mine)
	handler := mediaMiddleware(svc)(http.NotFoundHandler())
	_, sound := playOf(t, handler, mine, "1.5:4", 1.5, 32, 18)
	var heard []byte
	for {
		rec := httptest.NewRecorder()
		handler.ServeHTTP(rec, httptest.NewRequest("GET", "/frames/read?id="+sound+"&n=16", nil))
		body := rec.Body.Bytes()
		for off := 0; off+16 <= len(body); {
			n := int(math.Float64frombits(binary.LittleEndian.Uint64(body[off+8:])))
			heard = append(heard, body[off+16:off+16+n*8]...)
			off += 16 + n*8
		}
		if rec.Header().Get("X-Frames-End") != "" {
			if e := rec.Header().Get("X-Frames-Error"); e != "" {
				t.Fatal(e)
			}
			break
		}
	}
	want, err := exec.Command("ffmpeg", "-v", "error", "-i", mine, "-map", "0:a:0", "-ac", "2", "-ar", "48000",
		"-f", "f32le", "-").Output()
	if err != nil {
		t.Fatal(err)
	}
	want = want[int(1.5*48000)*8:]
	// To the end of the sound the file says it has, as the ffmpeg the app
	// ships reads it. An older ffmpeg, the system's in CI on Linux, keeps
	// the samples the AAC encoder padded its last packet with, up to 1024.
	if extra := len(want) - len(heard); extra < 0 || extra > 1024*8 {
		t.Fatalf("the play held %d moments, the sound from 1.5 s has %d", len(heard)/8, len(want)/8)
	}
	// Past the fade in, and short of the fade out.
	fade := int(engine.Fade*48000) * 8
	worst := 0.0
	for i := fade; i+4 <= len(heard)-48000/10*8; i += 4 {
		a := math.Float32frombits(binary.LittleEndian.Uint32(heard[i:]))
		b := math.Float32frombits(binary.LittleEndian.Uint32(want[i:]))
		worst = math.Max(worst, math.Abs(float64(a-b)))
	}
	if worst > 1e-4 {
		t.Errorf("the play's sound is off by %.5f", worst)
	}
}

// plain starts a stream whose frames say nothing but where they start.
func plain(p *previews, give func([]byte), run func(ctx context.Context, got func(at float64, data []byte) error) error) string {
	id, _ := p.startStamped("frames", nil, give, func(ctx context.Context, got func(previewFrame) error) error {
		return run(ctx, func(at float64, data []byte) error { return got(previewFrame{at: at, data: data}) })
	})
	return id
}

// needDecoder fails a test of the picture where the episode's decoder is
// not built, which make frames does and the app always ships: the frames
// come from nothing else.
func needDecoder(t *testing.T) {
	t.Helper()
	if os.Getenv("FRAMEFAIRY_FRAMES") == "" {
		ffmpegtest.Unusable(t, "FRAMEFAIRY_FRAMES names no framefairy-frames, which make frames builds")
	}
}

// The streams' reaper ends once it has closed the last decoder nobody
// holds, so a set of streams the app or a test is done with leaves no
// goroutine behind, and says so, so the next stream or decoder starts it
// again. In a testing/synctest bubble, so the decoder stands unused for
// as long as it takes with no wait on the clock.
func TestTheReaperEndsOnceNothingIsLeftToReap(t *testing.T) {
	t.Parallel()
	synctest.Test(t, func(t *testing.T) {
		var p previews
		path := "/videos/start.mp4"
		p.mu.Lock()
		p.decoders = map[string]*engine.EpisodeFrames{path: engine.NewMachine(afero.NewMemMapFs()).EpisodeFrames("framefairy-frames", path)}
		p.reaping = true
		p.mu.Unlock()
		ended := make(chan struct{})
		go func() {
			p.reap()
			close(ended)
		}()
		// A reaper that never ends hangs here until go test's own timeout.
		<-ended
		p.mu.Lock()
		defer p.mu.Unlock()
		if len(p.decoders) != 0 {
			t.Errorf("the reaper ended with %d decoders open", len(p.decoders))
		}
		if p.reaping {
			t.Error("the reaper ended and still says it reaps, so no stream would start it again")
		}
	})
}

// An episode open in the video preview holds its decoder, which is never
// closed for standing unused while it is held: a click after a long pause
// finds it as ready as the first. Let go, it is closed once it has stood
// unused. Plan row 2.156.
func TestTheFramesRouteKeepsTheDecoderOfAnOpenEpisode(t *testing.T) {
	ffmpegtest.Need(t)
	needDecoder(t)
	svc, mine, _ := library(t)
	fixtures.Copy(t, "small-4s.mp4", mine)
	handler := mediaMiddleware(svc)(http.NotFoundHandler())
	get := func(route string) int {
		rec := httptest.NewRecorder()
		handler.ServeHTTP(rec, httptest.NewRequest("GET", route+"?"+url.Values{"path": {mine}}.Encode(), nil))
		return rec.Code
	}
	if code := get("/frames/hold"); code != http.StatusNoContent {
		t.Fatalf("hold answered %d", code)
	}
	has := func() bool {
		svc.previews.mu.Lock()
		defer svc.previews.mu.Unlock()
		return svc.previews.decoders[mine] != nil
	}
	if !has() {
		t.Fatal("a hold started no decoder")
	}
	// Reaped as if everything had stood unused for ever.
	svc.previews.reapOnce(-time.Hour)
	if !has() {
		t.Fatal("the decoder of an episode held open was closed for standing unused")
	}
	if code := get("/frames/release"); code != http.StatusNoContent {
		t.Fatalf("release answered %d", code)
	}
	svc.previews.reapOnce(-time.Hour)
	if has() {
		t.Error("the decoder of an episode let go of was kept")
	}
	// Only an episode of the library is held.
	rec := httptest.NewRecorder()
	handler.ServeHTTP(rec, httptest.NewRequest("GET", "/frames/hold?"+url.Values{"path": {"/etc/passwd"}}.Encode(), nil))
	if rec.Code != http.StatusNotFound {
		t.Errorf("a hold of a file outside the library answered %d, want 404", rec.Code)
	}
}

// Let go of, no more than decodersSpare decoders stand running, the ones
// used last, however many episodes were open: the app builds ten
// workspaces as it starts, each holding its decoder until it sleeps. The
// decoders are made 2 ms apart on the fake clock of a testing/synctest
// bubble, so the one used longest ago is known.
func TestOnlyAFewDecodersNobodyHoldsStandRunning(t *testing.T) {
	synctest.Test(t, func(t *testing.T) {
		var p previews
		p.decoders = map[string]*engine.EpisodeFrames{}
		p.held = map[string]int{}
		var paths []string
		for i := range 6 {
			path := fmt.Sprintf("/episode-%d.mp4", i)
			paths = append(paths, path)
			// Never started: a decoder runs nothing until it is first asked.
			p.decoders[path] = engine.NewMachine(afero.NewMemMapFs()).EpisodeFrames("framefairy-frames", path)
			p.held[path] = 1
			time.Sleep(2 * time.Millisecond) //nolint:forbidigo // synctest bubble
		}
		// Each let go of in turn, the way the workspaces go to sleep.
		for _, path := range paths[:5] {
			p.release(path)
		}
		p.mu.Lock()
		defer p.mu.Unlock()
		if len(p.decoders) != 1+decodersSpare {
			t.Fatalf("%d decoders stand, wanted the one held and %d spare", len(p.decoders), decodersSpare)
		}
		if p.decoders[paths[5]] == nil {
			t.Error("the decoder still held was closed")
		}
		// The spares are the ones used last.
		for _, path := range paths[5-decodersSpare : 5] {
			if p.decoders[path] == nil {
				t.Errorf("%s, used last, was closed", path)
			}
		}
	})
}

// gridOf is the grid the video preview counts on for an episode, as the
// page hands it back with a stream it opens.
func gridOf(t *testing.T, path string) string {
	t.Helper()
	info, err := engine.NewEngine(engineLog("files", path), engine.NewMachine(afero.NewOsFs())).Probe(context.Background(), path)
	if err != nil {
		t.Fatal(err)
	}
	return framewire.GridText(info.Grid())
}

// A stream of the video preview that panics fails with the reason, the
// way a stream whose ffmpeg failed does, and the page asks another. It
// runs on a goroutine of its own, and a panic in it ended the whole app,
// issue #604.
func TestAStreamThatPanicsFailsItselfNotTheApp(t *testing.T) {
	// Reaping already, so no reaper of its own outlives the test.
	p := &previews{reaping: true}
	id := plain(p, nil, func(context.Context, func(float64, []byte) error) error {
		panic("a frame nobody read before")
	})
	s := p.get(id)
	<-s.done
	if s.err == nil || s.err.Error() != "the stream stopped unexpectedly: a frame nobody read before" {
		t.Errorf("the stream that panicked gave %v", s.err)
	}
}

// A request of the window that panics is answered with the reason, and
// the app goes on. Wails serves each on a goroutine with no recover, so a
// panic in a frame, a sound or a caption ended the whole app, issue #604.
// A request with no library to look in makes it panic, the shortest way
// there is to one.
func TestARequestThatPanicsFailsItselfNotTheApp(t *testing.T) {
	_, read := testLog(t)
	w := httptest.NewRecorder()
	mediaMiddleware(nil)(nil).ServeHTTP(w, httptest.NewRequest(http.MethodGet, "/frames/play?path=/home/tim/start.mp4", nil))
	if w.Code != http.StatusInternalServerError || !strings.HasPrefix(w.Body.String(), "the request stopped unexpectedly: ") {
		t.Errorf("the request that panicked was answered %d, %q", w.Code, w.Body.String())
	}
	line := find(read(), "the window's request stopped unexpectedly")
	has(t, line, map[string]any{"level": "error", "from": "files", "path": "/frames/play"})
	if stack, _ := line["stack"].(string); !strings.Contains(stack, "goroutine") {
		t.Errorf("the app's log holds no stack of the panic, %v", line)
	}
}

// A pull of frames that panics after its answer began is cut short
// where it stopped. Its status went out already, so the reason written
// after would have been read as a frame, or played as sound, issue #859.
// The log says how much went out. The panic comes from the window's end
// of the answer, the shortest way there is to one after the first byte.
func TestAPullThatPanicsAfterItsAnswerBeganIsCutShort(t *testing.T) {
	_, read := testLog(t)
	svc := &FrameFairy{}
	// Reaping already, so no reaper of its own outlives the test.
	svc.previews.reaping = true
	id := plain(&svc.previews, nil, func(_ context.Context, got func(float64, []byte) error) error {
		return got(1.5, []byte("a frame"))
	})
	t.Cleanup(func() { svc.previews.close(id) })
	w := &breaksOnSecondWrite{ResponseRecorder: httptest.NewRecorder()}
	mediaMiddleware(svc)(nil).ServeHTTP(w, httptest.NewRequest(http.MethodGet, "/frames/read?n=1&id="+id, nil))
	if w.Code != http.StatusOK || w.Body.Len() != 8 {
		t.Errorf("the pull that panicked was answered %d, %q, want 200 and only the frame's 8 bytes of time", w.Code, w.Body.String())
	}
	has(t, find(read(), "the window's request stopped unexpectedly, its answer cut short"),
		map[string]any{"level": "error", "from": "files", "path": "/frames/read", "status": 200, "bytes": 8})
}

// breaksOnSecondWrite is the window's end of an answer that takes the
// first write and panics on the next.
type breaksOnSecondWrite struct {
	*httptest.ResponseRecorder
	writes int
}

func (b *breaksOnSecondWrite) Write(p []byte) (int, error) {
	if b.writes++; b.writes > 1 {
		panic("the window went away")
	}
	return b.ResponseRecorder.Write(p)
}

// Every frame a stream hands over is given back to its decoder once,
// when it is written to the page, or when the stream stops with it in
// hand, so the decoder reads a later frame
// into it rather than into new memory, issue #898. A frame given back
// twice would be read into while the page still had it.
func TestEveryFrameIsGivenBackOnceNothingReadsIt(t *testing.T) {
	var given [][]byte
	var mu sync.Mutex
	give := func(b []byte) {
		mu.Lock()
		given = append(given, b)
		mu.Unlock()
	}
	frames := [][]byte{[]byte("first"), []byte("second"), []byte("in hand")}
	svc := &FrameFairy{}
	// Reaping already, so no reaper of its own outlives the test.
	svc.previews.reaping = true
	id := plain(&svc.previews, give, func(_ context.Context, got func(float64, []byte) error) error {
		for i, f := range frames {
			if err := got(float64(i+1), f); err != nil {
				return err
			}
		}
		return nil
	})
	s := svc.previews.get(id)
	pull := func() string {
		w := httptest.NewRecorder()
		svc.serveFrames(w, httptest.NewRequest(http.MethodGet, "/frames/read?n=1&id="+id, nil))
		return w.Body.String()[8:]
	}
	if got := pull(); got != "first" {
		t.Fatalf("the first pull wrote %q, want first", got)
	}
	if got := pull(); got != "second" {
		t.Fatalf("the second pull wrote %q, want second", got)
	}
	// The stream holds the third frame, which nobody pulls, and stops.
	svc.previews.close(id)
	<-s.done
	mu.Lock()
	defer mu.Unlock()
	if len(given) != len(frames) {
		t.Fatalf("%d frames were given back, want each of the %d once", len(given), len(frames))
	}
	for i, f := range frames {
		if &given[i][0] != &f[0] {
			t.Errorf("frame %d given back was %q, want %q", i, given[i], f)
		}
	}
}

// A paused video preview asks for where the hand is and gets the frame
// under it, the same frame a play from there hands over, said to be the
// frame of the place. A look for a frame the grid does not have, or for a
// file outside the library, is refused.
//
// Not parallel: library sets the environment the store reads.
func TestTheLookRouteAnswersWithTheFrameUnderTheHand(t *testing.T) {
	ffmpegtest.Need(t)
	needDecoder(t)
	svc, mine, _ := library(t)
	fixtures.Copy(t, "small-10s.mp4", mine)
	handler := mediaMiddleware(svc)(http.NotFoundHandler())
	grid := gridOf(t, mine)
	ask := func(frame string, more url.Values) *httptest.ResponseRecorder {
		q := url.Values{"path": {mine}, "w": {"32"}, "h": {"18"}, "grid": {grid}, "frame": {frame}, "shown": {"-1"}}
		for k, v := range more {
			q[k] = v
		}
		rec := httptest.NewRecorder()
		handler.ServeHTTP(rec, httptest.NewRequest("GET", "/frames/look?"+q.Encode(), nil))
		return rec
	}
	look := ask("30", nil)
	if look.Code != http.StatusOK || look.Header().Get("X-Frames-Exact") != "1" || look.Header().Get("X-Frames-Near") != "1" || look.Header().Get("X-Frames-Error") != "" {
		t.Fatalf("the look answered %d, %v", look.Code, look.Header())
	}
	if n := look.Body.Len(); n != 8+32*18*4 {
		t.Fatalf("the look answered %d bytes, want one frame", n)
	}
	g, _ := framewire.ReadGrid(grid)
	if at := math.Float64frombits(binary.LittleEndian.Uint64(look.Body.Bytes())); math.Abs(at-g.At(30)) > 1e-6 {
		t.Errorf("the look's frame starts at %.6f s, want %.6f", at, g.At(30))
	}

	// The frames the decoder read around the click, which the window does
	// not hold yet, the one before it held already.
	near := httptest.NewRecorder()
	handler.ServeHTTP(near, httptest.NewRequest("GET", "/frames/near?"+url.Values{"path": {mine}, "w": {"32"}, "h": {"18"}, "grid": {grid}, "frame": {"30"}, "has": {"29"}}.Encode(), nil))
	if n := near.Body.Len(); near.Code != http.StatusOK || n != 4*(8+32*18*4) {
		t.Errorf("the frames near answered %d with %d bytes, want the 4 frames after the click", near.Code, n)
	}

	// A frame read ahead is found kept, nothing decoded for it.
	ahead := httptest.NewRecorder()
	handler.ServeHTTP(ahead, httptest.NewRequest("GET", "/frames/ahead?"+url.Values{"path": {mine}, "w": {"32"}, "h": {"18"}, "grid": {grid}, "frames": {fmt.Sprint(g.Count / 2)}, "cards": {fmt.Sprint(g.Count-3) + ",x"}}.Encode(), nil))
	if ahead.Code != http.StatusOK || ahead.Header().Get("X-Frames-Error") != "" {
		t.Fatalf("reading ahead answered %d, %v", ahead.Code, ahead.Header())
	}
	// The frames read ahead come with the answer, for the window to keep:
	// the word's with the frame before and four after, and the card's.
	if size := 8 + 32*18*4; ahead.Body.Len() != 7*size {
		t.Errorf("reading ahead handed over %d bytes, not 7 frames of %d", ahead.Body.Len(), size)
	}
	for _, f := range []string{fmt.Sprint(g.Count / 2), fmt.Sprint(g.Count - 3)} {
		if kept := ask(f, nil); kept.Header().Get("X-Frames-Kept") != "1" || kept.Header().Get("X-Frames-Exact") != "1" {
			t.Errorf("a look at frame %s read ahead answered %d, %v", f, kept.Code, kept.Header())
		}
	}

	pictures, _ := playOf(t, handler, mine, "0:10", g.At(30), 32, 18)
	read := httptest.NewRecorder()
	handler.ServeHTTP(read, httptest.NewRequest("GET", "/frames/read?id="+pictures+"&n=1", nil))
	svc.previews.close(pictures)
	if got := read.Body.Bytes(); len(got) != 32+32*18*4 || string(got[32:]) != string(look.Body.Bytes()[8:]) {
		t.Error("the look's frame is not the frame a play from there hands over")
	}

	if bad := ask(fmt.Sprint(g.Count), nil); bad.Code != http.StatusBadRequest {
		t.Errorf("a look past the grid answered %d", bad.Code)
	}
	rec := httptest.NewRecorder()
	handler.ServeHTTP(rec, httptest.NewRequest("GET", "/frames/look?"+url.Values{"path": {"/elsewhere.mp4"}, "w": {"32"}, "h": {"18"}, "grid": {grid}, "frame": {"1"}}.Encode(), nil))
	if rec.Code != http.StatusNotFound {
		t.Errorf("a look at a file outside the library answered %d", rec.Code)
	}
}

// A look and an ask to read ahead say where they stand among what their
// window sent, see engine.Sent. A name too long or a count that is no
// number stands nowhere, and the schedule takes the request as it comes.
func TestARequestSaysWhereItStandsAmongWhatItsWindowSent(t *testing.T) {
	for _, c := range []struct {
		query string
		want  engine.Sent
	}{
		{"win=ab&n=7", engine.Sent{Window: "ab", N: 7}},
		{"win=ab", engine.Sent{}},
		{"n=7", engine.Sent{}},
		{"win=ab&n=-1", engine.Sent{}},
		{"win=" + strings.Repeat("a", 65) + "&n=7", engine.Sent{}},
	} {
		q, err := url.ParseQuery(c.query)
		if err != nil {
			t.Fatal(err)
		}
		if got := sentOf(q); got != c.want {
			t.Errorf("%s stands at %+v, want %+v", c.query, got, c.want)
		}
	}
}
