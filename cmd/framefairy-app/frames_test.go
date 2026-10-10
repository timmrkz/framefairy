package main

import (
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
	"testing"
	"time"

	"framefairy/engine"
	"framefairy/internal/ffmpegtest"
)

// A stream the page opens, pulls from and closes: each frame comes once,
// in the order shown, from the moment it was opened at, a pull with skip
// drops what comes before it, and a closed stream is gone.
func TestTheFramesRouteStreamsAnEpisode(t *testing.T) {
	ffmpegtest.Need(t)
	needDecoder(t)
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
	size := 8 + 64*36*4
	var told []string
	read := func(q url.Values) []float64 {
		t.Helper()
		q.Set("id", opened.ID)
		rec := get("/frames/read", q)
		if rec.Code != http.StatusOK {
			t.Fatalf("read answered %d", rec.Code)
		}
		told = append(told, rec.Header().Get("X-Frames-Times"))
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
	// The first pull says where the stream's time went before its first
	// frame, in milliseconds, and no pull after it says it again.
	// The first stream of a decoder nobody held opens the file.
	var started, opened2, first, file int
	if n, _ := fmt.Sscanf(told[0], "%d,%d,%d,%d", &started, &opened2, &first, &file); n != 4 ||
		started <= 0 || opened2 < started || first < opened2 || file != 1 {
		t.Errorf("the first pull told %q as the stream's times, want the request had, the place reached and the first frame, in order, and the file opened", told[0])
	}
	for _, again := range told[1:] {
		if again != "" {
			t.Errorf("a later pull told the times again, %q", again)
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

// An HDR episode's frames say their light with every pull, so the page
// draws them as HDR, and a standard one's say nothing, see
// framewire.Picture. Step 4 of 2.156.
func TestTheFramesRouteSaysAnHDREpisodesLight(t *testing.T) {
	ffmpegtest.Need(t)
	needDecoder(t)
	svc, mine, _ := library(t)
	// The tags are what says HDR, so any picture tagged HLG will do. The
	// colr box carries them in an MP4 whatever the codec.
	if out, err := exec.Command("ffmpeg", "-loglevel", "error", "-y", "-f", "lavfi",
		"-i", "testsrc2=s=160x90:r=5:d=2", "-c:v", "mpeg4", "-color_primaries", "bt2020",
		"-color_trc", "arib-std-b67", "-colorspace", "bt2020nc", "-movflags", "+write_colr",
		"-f", "mp4", mine).CombinedOutput(); err != nil {
		t.Fatalf("making the episode: %s %s", err, out)
	}
	handler := mediaMiddleware(svc.store)(http.NotFoundHandler())
	get := func(path string, q url.Values) *httptest.ResponseRecorder {
		rec := httptest.NewRecorder()
		handler.ServeHTTP(rec, httptest.NewRequest("GET", path+"?"+q.Encode(), nil))
		return rec
	}
	rec := get("/frames/open", url.Values{"path": {mine}, "from": {"0"}, "w": {"64"}, "h": {"36"}})
	var opened struct{ ID string }
	if err := json.NewDecoder(rec.Body).Decode(&opened); err != nil || opened.ID == "" {
		t.Fatalf("open answered %d %q: %v", rec.Code, rec.Body.String(), err)
	}
	defer get("/frames/close", url.Values{"id": {opened.ID}})
	for range 2 {
		rec := get("/frames/read", url.Values{"id": {opened.ID}, "n": {"2"}})
		if rec.Code != http.StatusOK || rec.Body.Len() == 0 {
			t.Fatalf("read answered %d with %d bytes: %s", rec.Code, rec.Body.Len(), rec.Header().Get("X-Frames-Error"))
		}
		if rec.Body.Len()%(8+64*36*4) != 0 {
			t.Errorf("%d bytes is no whole number of frames of 4 bytes a pixel", rec.Body.Len())
		}
		if light := rec.Header().Get("X-Frames-Light"); light != "hlg" {
			t.Errorf("a pull of an HLG episode says its light is %q, want hlg", light)
		}
	}
}

// A stream that has run to the end of the episode says so.
func TestTheFramesRouteSaysWhenAnEpisodeEnds(t *testing.T) {
	ffmpegtest.Need(t)
	needDecoder(t)
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
		frames += rec.Body.Len() / (8 + 32*18*4)
	}
	t.Errorf("no end after %d frames", frames)
}

// A stream the Go side stops, to make room for newer ones or because it
// stood unused, says it was closed, not that the episode ended, so the
// page asks another stream for what it waited on. Taken for the end, the
// page settled for a frame near the one wanted, and Tim saw the wrong
// frame after a drag along the clip timeline. Plan row 2.156.
func TestTheFramesRouteSaysAStoppedStreamWasClosed(t *testing.T) {
	ffmpegtest.Need(t)
	needDecoder(t)
	svc, mine, _ := library(t)
	if out, err := exec.Command("ffmpeg", "-loglevel", "error", "-y", "-f", "lavfi",
		"-i", "testsrc2=s=160x90:r=5:d=10", "-c:v", "mpeg4", "-f", "mp4", mine).CombinedOutput(); err != nil {
		t.Fatalf("making the episode: %s %s", err, out)
	}
	handler := mediaMiddleware(svc.store)(http.NotFoundHandler())
	rec := httptest.NewRecorder()
	handler.ServeHTTP(rec, httptest.NewRequest("GET", "/frames/open?"+url.Values{"path": {mine}, "w": {"32"}, "h": {"18"}}.Encode(), nil))
	var opened struct{ ID string }
	if err := json.NewDecoder(rec.Body).Decode(&opened); err != nil {
		t.Fatal(err)
	}
	// Held by a pull that is on its way as the stream is stopped.
	s := openPreviews.get(opened.ID)
	if s == nil {
		t.Fatal("the stream is not open")
	}
	openPreviews.close(opened.ID)
	<-s.done
	read := httptest.NewRecorder()
	writeFrames(read, httptest.NewRequest("GET", "/frames/read?id="+opened.ID, nil), s, 4, 0)
	if read.Header().Get("X-Frames-Closed") == "" || read.Header().Get("X-Frames-Error") != "" {
		t.Errorf("a stopped stream answered %v, want it said closed and no error", read.Header())
	}
	// A pull after it is gone finds nothing, which the page takes the same way.
	late := httptest.NewRecorder()
	handler.ServeHTTP(late, httptest.NewRequest("GET", "/frames/read?id="+opened.ID+"&n=1", nil))
	if late.Code != http.StatusNotFound {
		t.Errorf("a pull of a closed stream answered %d, want 404", late.Code)
	}
}

// There is one way to the frames, the episode's decoder: the routes of
// the Mac's own decoder, which the page fed with the file's samples, are
// gone with it. Plan row 2.156, step 3 of docs/VIDEO-PREVIEW.md.
func TestTheFramesRouteHasNoSecondDecoder(t *testing.T) {
	svc, _, _ := library(t)
	handler := mediaMiddleware(svc.store)(http.NotFoundHandler())
	for _, route := range []string{"/frames/native?codec=hvc1&cw=320&ch=180&w=160&h=90", "/frames/decode?id=nobody"} {
		rec := httptest.NewRecorder()
		handler.ServeHTTP(rec, httptest.NewRequest("POST", route, strings.NewReader("config")))
		if rec.Code != http.StatusNotFound {
			t.Errorf("%s answered %d, want 404", route, rec.Code)
		}
	}
}

// A sound stream is read the way a stream of frames is, and what it holds,
// put together, is the episode's sound from where it was opened, as ffmpeg
// decodes it, to its end.
func TestTheFramesRouteStreamsSound(t *testing.T) {
	ffmpegtest.Need(t)
	needDecoder(t)
	svc, mine, _ := library(t)
	if out, err := exec.Command("ffmpeg", "-loglevel", "error", "-y",
		"-f", "lavfi", "-i", "testsrc2=s=160x90:r=5:d=4",
		"-f", "lavfi", "-i", "aevalsrc=sin(2*PI*(200*t+300*t*t))|0.5*sin(2*PI*900*t):s=48000:d=4",
		"-c:v", "mpeg4", "-c:a", "aac", "-f", "mp4", mine).CombinedOutput(); err != nil {
		t.Fatalf("making the episode: %s %s", err, out)
	}
	handler := mediaMiddleware(svc.store)(http.NotFoundHandler())
	get := func(path string, q url.Values) *httptest.ResponseRecorder {
		rec := httptest.NewRecorder()
		handler.ServeHTTP(rec, httptest.NewRequest("GET", path+"?"+q.Encode(), nil))
		return rec
	}
	if rec := get("/frames/sound", url.Values{"path": {mine}, "from": {"1"}, "rate": {"48000"}, "ch": {"9"}}); rec.Code != http.StatusBadRequest {
		t.Errorf("nine channels were opened: %d", rec.Code)
	}
	rec := get("/frames/sound", url.Values{"path": {mine}, "from": {"1.5"}, "rate": {"48000"}, "ch": {"2"}})
	var opened struct{ ID string }
	if err := json.NewDecoder(rec.Body).Decode(&opened); err != nil || opened.ID == "" {
		t.Fatalf("open answered %d %q: %v", rec.Code, rec.Body.String(), err)
	}
	defer get("/frames/close", url.Values{"id": {opened.ID}})
	whole := 8 + engine.SoundChunk*2*4
	var heard []byte
	var ats []float64
	for {
		rec := get("/frames/read", url.Values{"id": {opened.ID}, "n": {"16"}})
		body := rec.Body.Bytes()
		for off := 0; off < len(body); off += whole {
			ats = append(ats, math.Float64frombits(binary.LittleEndian.Uint64(body[off:])))
			heard = append(heard, body[off+8:min(off+whole, len(body))]...)
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
		t.Fatalf("the stream held %d moments, the sound from 1.5 s has %d", len(heard)/8, len(want)/8)
	}
	worst := 0.0
	for i := 0; i+4 <= len(heard)-48000/10*8; i += 4 {
		a := math.Float32frombits(binary.LittleEndian.Uint32(heard[i:]))
		b := math.Float32frombits(binary.LittleEndian.Uint32(want[i:]))
		worst = math.Max(worst, math.Abs(float64(a-b)))
	}
	if worst > 1e-4 {
		t.Errorf("the stream's sound is off by %.5f", worst)
	}
	for k, at := range ats {
		if w := 1.5 + float64(k*engine.SoundChunk)/48000; math.Abs(at-w) > 1e-9 {
			t.Fatalf("chunk %d says %.6f, it is at %.6f", k, at, w)
		}
	}
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

// An episode open in the video preview holds its decoder, which is never
// closed for standing unused while it is held: a click after a long pause
// finds it as ready as the first. Let go, it is closed once it has stood
// unused. Plan row 2.156.
func TestTheFramesRouteKeepsTheDecoderOfAnOpenEpisode(t *testing.T) {
	ffmpegtest.Need(t)
	needDecoder(t)
	svc, mine, _ := library(t)
	if out, err := exec.Command("ffmpeg", "-loglevel", "error", "-y", "-f", "lavfi",
		"-i", "testsrc2=s=160x90:r=5:d=4", "-c:v", "mpeg4", "-f", "mp4", mine).CombinedOutput(); err != nil {
		t.Fatalf("making the episode: %s %s", err, out)
	}
	handler := mediaMiddleware(svc.store)(http.NotFoundHandler())
	get := func(route string) int {
		rec := httptest.NewRecorder()
		handler.ServeHTTP(rec, httptest.NewRequest("GET", route+"?"+url.Values{"path": {mine}}.Encode(), nil))
		return rec.Code
	}
	if code := get("/frames/hold"); code != http.StatusNoContent {
		t.Fatalf("hold answered %d", code)
	}
	has := func() bool {
		openPreviews.mu.Lock()
		defer openPreviews.mu.Unlock()
		return openPreviews.decoders[mine] != nil
	}
	if !has() {
		t.Fatal("a hold started no decoder")
	}
	// Reaped as if everything had stood unused for ever.
	openPreviews.reapOnce(-time.Hour)
	if !has() {
		t.Fatal("the decoder of an episode held open was closed for standing unused")
	}
	if code := get("/frames/release"); code != http.StatusNoContent {
		t.Fatalf("release answered %d", code)
	}
	openPreviews.reapOnce(-time.Hour)
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
