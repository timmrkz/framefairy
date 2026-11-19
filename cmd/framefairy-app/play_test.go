package main

import (
	"encoding/binary"
	"encoding/json"
	"fmt"
	"math"
	"net/http"
	"net/http/httptest"
	"net/url"
	"testing"

	"framefairy/internal/ffmpegtest"
	"framefairy/internal/fixtures"
	"framefairy/internal/framewire"
)

// A play of two pieces hands over every frame of each, in the order they
// are drawn, each stamped with where on the program it goes, the moment
// it stands for and where its frame begins, the same frames a look at
// them answers with, and the sound of both pieces, end to end on the
// program. A play with no pieces is refused, a play paused gives the
// decoder to a look and goes on after, and its loop is switched while it
// plays.
//
// Not parallel: library sets the environment the store reads.
func TestThePlayRouteStreamsAClipsFramesAndSound(t *testing.T) {
	ffmpegtest.Need(t)
	needDecoder(t)
	svc, mine, _ := library(t)
	fixtures.Copy(t, "small-4s.mp4", mine)
	handler := mediaMiddleware(svc)(http.NotFoundHandler())
	text := gridOf(t, mine)
	g, _ := framewire.ReadGrid(text)
	get := func(path string, q url.Values) *httptest.ResponseRecorder {
		rec := httptest.NewRecorder()
		handler.ServeHTTP(rec, httptest.NewRequest("GET", path+"?"+q.Encode(), nil))
		return rec
	}
	ask := url.Values{"path": {mine}, "w": {"32"}, "h": {"18"}, "grid": {text}, "rate": {"48000"}, "ch": {"2"}, "at": {fmt.Sprint(g.At(2))},
		"pieces": {fmt.Sprintf("%v:%v,%v:%v", g.At(2), g.At(7), g.At(12), g.At(17))}}
	bad := url.Values{}
	for k, v := range ask {
		bad[k] = v
	}
	bad["pieces"] = []string{"1:x"}
	if rec := get("/frames/play", bad); rec.Code != http.StatusBadRequest {
		t.Errorf("a play of no pieces answered %d", rec.Code)
	}
	rec := get("/frames/play", ask)
	var play struct {
		Pictures, Sound string
		From, End       float64
	}
	if err := json.NewDecoder(rec.Body).Decode(&play); err != nil || play.Pictures == "" || play.Sound == "" {
		t.Fatalf("the play answered %d %q: %v", rec.Code, rec.Body.String(), err)
	}
	defer get("/frames/close", url.Values{"id": {play.Pictures}})
	defer get("/frames/close", url.Values{"id": {play.Sound}})
	length := 10 * (g.At(1) - g.At(0))
	if play.From != 0 || math.Abs(play.End-length) > 1e-9 {
		t.Errorf("the play goes from %v to %v", play.From, play.End)
	}

	size := 32 + 32*18*4
	// Paused, the play gives the decoder back: a look has it meanwhile,
	// and the play goes on after with no frame lost or twice.
	pictures := get("/frames/read", url.Values{"id": {play.Pictures}, "n": {"3"}}).Body.Bytes()
	if rec := get("/frames/play/hold", url.Values{"id": {play.Pictures}}); rec.Code != http.StatusNoContent {
		t.Errorf("the play held answered %d", rec.Code)
	}
	held := get("/frames/look", url.Values{"path": {mine}, "w": {"32"}, "h": {"18"}, "grid": {text}, "frame": {"15"}, "shown": {"-1"}})
	if held.Code != http.StatusOK || held.Body.Len() != 8+32*18*4 {
		t.Errorf("a look while the play is held answered %d with %d bytes", held.Code, held.Body.Len())
	}
	if rec := get("/frames/play/go", url.Values{"id": {play.Pictures}}); rec.Code != http.StatusNoContent {
		t.Errorf("the play let go answered %d", rec.Code)
	}
	for {
		rec := get("/frames/read", url.Values{"id": {play.Pictures}, "n": {"16"}})
		pictures = append(pictures, rec.Body.Bytes()...)
		if rec.Header().Get("X-Frames-End") != "" {
			if e := rec.Header().Get("X-Frames-Error"); e != "" {
				t.Fatal(e)
			}
			break
		}
	}
	if len(pictures) != 10*size {
		t.Fatalf("the play handed over %d bytes, not 10 frames of %d", len(pictures), size)
	}
	for k := range 10 {
		frame := 2 + k
		if k >= 5 {
			frame = 7 + k
		}
		head := pictures[k*size:]
		at := math.Float64frombits(binary.LittleEndian.Uint64(head))
		moment := math.Float64frombits(binary.LittleEndian.Uint64(head[8:]))
		begins := math.Float64frombits(binary.LittleEndian.Uint64(head[16:]))
		edge := math.Float64frombits(binary.LittleEndian.Uint64(head[24:]))
		if wantEdge := k == 0 || k == 4 || k == 5 || k == 9; (edge == 1) != wantEdge {
			t.Errorf("frame %d says edge %v", k, edge)
		}
		if math.Abs(at-float64(k)*length/10) > 1e-6 || math.Abs(moment-g.At(frame)) > 1e-6 || math.Abs(begins-g.At(frame)) > 1e-6 {
			t.Errorf("frame %d is drawn at %v, stands at %v and begins at %v", k, at, moment, begins)
		}
		look := get("/frames/look", url.Values{"path": {mine}, "w": {"32"}, "h": {"18"}, "grid": {text}, "frame": {fmt.Sprint(frame)}, "shown": {"-1"}})
		if string(look.Body.Bytes()[8:]) != string(head[32:size]) {
			t.Errorf("frame %d of the play is not the frame a look at %d answers with", k, frame)
		}
	}

	// The sound: a new play, since the looks above took the decoder.
	rec = get("/frames/play", ask)
	if err := json.NewDecoder(rec.Body).Decode(&play); err != nil {
		t.Fatal(err)
	}
	next := int64(0)
	for {
		rec := get("/frames/read", url.Values{"id": {play.Sound}, "n": {"16"}})
		body := rec.Body.Bytes()
		for off := 0; off < len(body); {
			out := int64(math.Float64frombits(binary.LittleEndian.Uint64(body[off:])))
			n := int(math.Float64frombits(binary.LittleEndian.Uint64(body[off+8:])))
			if out != next {
				t.Fatalf("a chunk starts at %d, the one before ended at %d", out, next)
			}
			next, off = out+int64(n), off+16+n*8
		}
		if rec.Header().Get("X-Frames-End") != "" {
			if e := rec.Header().Get("X-Frames-Error"); e != "" {
				t.Fatal(e)
			}
			break
		}
	}
	if want := int64(math.Round(length * 48000)); math.Abs(float64(next-want)) > 1 {
		t.Errorf("the play's sound is %d moments, the clip lasts %d", next, want)
	}

	// A play that takes over from this one starts at the moment it stands
	// at, where the new pieces hold it, and is refused where they do not.
	over := url.Values{}
	for k, v := range ask {
		over[k] = v
	}
	over["after"], over["place"] = []string{play.Pictures}, []string{"0.4"}
	rec = get("/frames/play", over)
	var took struct{ Moment, From float64 }
	if err := json.NewDecoder(rec.Body).Decode(&took); err != nil || math.Abs(took.Moment-(g.At(2)+0.4)) > 1e-9 || math.Abs(took.From-0.4) > 1e-9 {
		t.Errorf("the play taking over answered %d %q", rec.Code, rec.Body.String())
	}
	over["pieces"] = []string{fmt.Sprintf("%v:%v", g.At(12), g.At(17))}
	if rec := get("/frames/play", over); rec.Code != http.StatusConflict {
		t.Errorf("a play taking over where its pieces do not hold the playhead answered %d", rec.Code)
	}

	loop := get("/frames/play/loop", url.Values{"id": {play.Pictures}, "at": {"0"}, "loop": {"1"}})
	var looped struct{ End float64 }
	if err := json.NewDecoder(loop.Body).Decode(&looped); err != nil || looped.End != -1 {
		t.Errorf("the loop switched on answered %d %q", loop.Code, loop.Body.String())
	}
}
