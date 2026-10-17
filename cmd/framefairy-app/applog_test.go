package main

import (
	"bufio"
	"bytes"
	"compress/gzip"
	"context"
	"encoding/json"
	"fmt"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"strings"
	"sync"
	"testing"
	"time"

	"github.com/rs/zerolog"
	"github.com/spf13/afero"

	"framefairy/engine"
)

// testLog opens the app's log in a folder of the test's own and gives
// what it holds, a line each, read as JSON. A line that is not whole JSON
// fails the test.
//
// On the real disk: lumberjack writes the log and knows only the disk.
func testLog(t *testing.T) (path string, read func() []map[string]any) {
	t.Helper()
	path = filepath.Join(t.TempDir(), "logs", "app.log")
	theLog.open(path)
	t.Cleanup(func() {
		theLog.close()
		theLog.recent.forget()
		theLog.act.Store(nil)
	})
	return path, func() []map[string]any {
		t.Helper()
		f, err := os.Open(path)
		if err != nil {
			t.Fatal(err)
		}
		defer f.Close()
		var lines []map[string]any
		scan := bufio.NewScanner(f)
		scan.Buffer(nil, 1<<20)
		for scan.Scan() {
			var line map[string]any
			if err := json.Unmarshal(scan.Bytes(), &line); err != nil {
				t.Fatalf("a line of the log is not whole JSON: %v\n%s", err, scan.Text())
			}
			lines = append(lines, line)
		}
		return lines
	}
}

// find is the first line whose message starts so, or nil.
func find(lines []map[string]any, msg string) map[string]any {
	for _, l := range lines {
		if s, _ := l["msg"].(string); strings.HasPrefix(s, msg) {
			return l
		}
	}
	return nil
}

// has says whether a line holds every field given, as the log wrote it.
func has(t *testing.T, line map[string]any, want map[string]any) {
	t.Helper()
	if line == nil {
		t.Errorf("no line for %v", want)
		return
	}
	for k, v := range want {
		if fmt.Sprint(line[k]) != fmt.Sprint(v) {
			t.Errorf("%s is %v, want %v, in %v", k, line[k], v, line)
		}
	}
}

// The window's lines reach the app's log as JSON, each under the moment
// the window said it, with its level, the video it is about and the run
// of the app, with what one call may write held to a few hundred lines.
// Several windows' calls at once keep their lines whole. A trace line is
// kept in memory, not in the file. Plan rows 2.185 and 2.188.
func TestSaidWritesTheAppLog(t *testing.T) {
	_, read := testLog(t)

	f := &FrameFairy{}
	before := time.Now().Add(-3 * time.Second).Truncate(time.Millisecond)
	f.Said([]SaidLine{
		{At: float64(before.UnixMilli()), Level: "error", Text: "frame queue: The app could not decode the picture."},
		{Video: "/videos/start.mp4", Text: "video preview: opening\n"},
		{Level: "trace", Text: "a frame"},
	})
	var lots []SaidLine
	for i := range 430 {
		lots = append(lots, SaidLine{Text: fmt.Sprintf("line %d", i)})
	}
	f.Said(lots)
	var wg sync.WaitGroup
	for i := range 8 {
		wg.Go(func() { f.Said([]SaidLine{{Text: fmt.Sprintf("at once %d %s", i, strings.Repeat("x", 3000))}}) })
	}
	wg.Wait()

	lines := read()
	err := find(lines, "frame queue:")
	has(t, err, map[string]any{"level": "error", "from": "window", "run": theLog.run})
	if at, _ := time.Parse(zerolog.TimeFieldFormat, fmt.Sprint(err["time"])); !at.Equal(before) {
		t.Errorf("the line stands at %v, want the moment the window said it, %v", err["time"], before)
	}
	has(t, find(lines, "video preview: opening"), map[string]any{"level": "debug", "video": "start.mp4", "msg": "video preview: opening"})
	if find(lines, "a frame") != nil {
		t.Error("a trace line was written to the file")
	}
	if !strings.Contains(string(theLog.recent.all()), `"level":"trace","time"`) || !strings.Contains(string(theLog.recent.all()), `"msg":"a frame"`) {
		t.Errorf("the trace line is not kept in memory: %s", theLog.recent.all())
	}
	has(t, find(lines, "line 399"), map[string]any{"level": "debug"})
	has(t, find(lines, "and 30 lines more"), map[string]any{"level": "warn", "more": 30})
	if find(lines, "line 400") != nil {
		t.Error("one call wrote more than 400 lines")
	}
	for i := range 8 {
		start := fmt.Sprintf("at once %d ", i)
		if l := find(lines, start); l == nil || l["msg"] != start+strings.Repeat("x", 2000-len(start))+"…" {
			t.Errorf("the long line %d is not whole and cut at 2000 characters", i)
		}
	}
}

// The video's file and frames say in the log what was asked and how it
// went, with the video and the stream as fields of their own. A pull of
// a stream that plays as it should is a trace line, kept in memory for a
// report, so a play does not fill the file. Plan rows 2.185 and 2.188.
func TestFramesRequestsAreLogged(t *testing.T) {
	_, read := testLog(t)

	serve := func(target string, h func(w http.ResponseWriter, r *http.Request)) {
		logged(httptest.NewRecorder(), httptest.NewRequest("GET", target, nil), h)
	}
	pull := func(w http.ResponseWriter, r *http.Request) { _, _ = w.Write(make([]byte, 64)) }
	serve("/frames/open?path=/videos/start.mp4&from=1.5&w=874&h=492", func(w http.ResponseWriter, r *http.Request) {
		_, _ = w.Write([]byte(`{"id":"1dcdec3203858e5a"}`))
	})
	serve("/frames/read?id=1dcdec3203858e5a&n=4&skip=0", pull)
	serve("/frames/read?id=1dcdec3203858e5a&n=4&skip=1", func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("X-Frames-End", "1")
		w.Header().Set("X-Frames-Error", "the picture could not be decoded")
		w.WriteHeader(http.StatusOK)
	})
	serve("/frames/close?id=none", func(w http.ResponseWriter, r *http.Request) { w.WriteHeader(http.StatusNoContent) })
	// A caption layer is bytes, one for every frame drawn: a trace line,
	// without them.
	serve("/frames/captions?path=/videos/start.mp4&at=1.5&w=270&h=480", func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/octet-stream")
		_, _ = w.Write(make([]byte, 2048))
	})
	// The same failure for the same video again, once a frame: written
	// once, and counted after. It wrote two lines a frame before, found by
	// the second review. failures is the package's own, so it is cleared
	// for this test, which nothing else runs beside.
	failures.mu.Lock()
	clear(failures.last)
	failures.mu.Unlock()
	for _, at := range []string{"1.6", "1.64", "1.68"} {
		serve("/frames/captions?path=/videos/start.mp4&at="+at+"&w=270&h=480", func(w http.ResponseWriter, r *http.Request) {
			http.Error(w, "the captions could not be drawn", http.StatusInternalServerError)
		})
	}
	serve("/frames/hold?path=/videos/start.mp4", func(w http.ResponseWriter, r *http.Request) {
		http.Error(w, "the video's decoder could not start", http.StatusInternalServerError)
	})

	lines := read()
	has(t, find(lines, "open "), map[string]any{
		"level": "debug", "from": "files", "video": "start.mp4", "msg": "open from 1.5 at 874x492",
		"status": 200, "bytes": 25, "said": `{"id":"1dcdec3203858e5a"}`,
	})
	has(t, find(lines, "read n 4 skip 1"), map[string]any{
		"level": "warn", "stream": "1dcdec", "msg": "read n 4 skip 1, failed: the picture could not be decoded", "status": 200,
	})
	has(t, find(lines, "hold"), map[string]any{
		"level": "warn", "video": "start.mp4", "status": 500, "said": "the video's decoder could not start",
	})
	has(t, find(lines, "captions at 1.6"), map[string]any{
		"level": "warn", "video": "start.mp4", "status": 500, "said": "the captions could not be drawn",
	})
	if find(lines, "captions at 1.64") != nil || find(lines, "captions at 1.68") != nil {
		t.Error("the same failure of the captions was written again straight after")
	}
	if find(lines, "captions at 1.5") != nil || strings.Contains(string(theLog.recent.all()), "captions at 1.5") {
		t.Error("a caption layer that came as it should was written, to the file or to memory")
	}
	if kept := string(theLog.recent.all()); strings.Contains(kept, `\u0000`) {
		t.Errorf("the memory holds the bytes of a caption layer: %s", kept)
	}
	if find(lines, "read n 4 skip 0") != nil {
		t.Error("a pull that went as it should was written to the file")
	}
	if kept := string(theLog.recent.all()); !strings.Contains(kept, `"msg":"read n 4 skip 0"`) || strings.Contains(kept, "skip 1") {
		t.Errorf("the memory holds %s, want the pull that went as it should and only that", kept)
	}
	if find(lines, "close") != nil {
		t.Error("the worker's reach probe was written")
	}
}

// Wails's own lines go into the same file, from info up, since its debug
// lines are one for every request the window makes. Plan row 2.188.
func TestWailsWritesTheAppLog(t *testing.T) {
	_, read := testLog(t)
	wails := theLog.slog("wails", zerolog.InfoLevel)
	wails.Debug("Asset Request:", "path", "/frames/read")
	wails.Error("the window could not be made", "error", "no display")

	lines := read()
	if find(lines, "Asset Request") != nil {
		t.Error("Wails's debug line was written")
	}
	has(t, find(lines, "the window could not be made"), map[string]any{
		"level": "error", "from": "wails", "run": theLog.run, "error": "no display",
	})
}

// The log keeps to its size: past 10 MB it starts a new file and keeps
// the one before beside it, so it never fills the disk. Lines written
// from many places at once while it does stay whole. Plan row 2.188.
//
// On the real disk: lumberjack starts a new file of the log on the disk.
func TestTheLogKeepsToItsSize(t *testing.T) {
	path, read := testLog(t)
	text := strings.Repeat("y", 1000)
	var wg sync.WaitGroup
	for range 8 {
		wg.Go(func() {
			for range 1500 {
				theLog.line(zerolog.DebugLevel, "app").Msg(text)
			}
		})
	}
	wg.Wait()
	info, err := os.Stat(path)
	if err != nil {
		t.Fatal(err)
	}
	if info.Size() > appLogMB<<20 {
		t.Errorf("the log is %d bytes, more than %d MB", info.Size(), appLogMB)
	}
	older, _ := filepath.Glob(filepath.Join(filepath.Dir(path), "app-*"))
	if len(older) == 0 {
		t.Error("no older file was kept beside the log")
	}
	read()
}

// What the person did is a line of its own, and every line it sets off
// carries its id: the window's, the Go side's and a job's, which keeps the
// act that asked for it however many acts follow while it runs. Plan row
// 2.188.
func TestActsTieTheLinesTheySetOff(t *testing.T) {
	_, read := testLog(t)
	f := &FrameFairy{}
	f.Acted(Act{ID: "a7", What: `pressed button "New"`, Video: "/videos/start.mp4"})
	job := &Job{ID: "job-3", Kind: "clip", Episode: "/videos/start.mp4", act: theLog.acting()}
	f.Said([]SaidLine{{Act: "a7", Text: "video preview: opening", Video: "/videos/start.mp4"}})
	theLog.line(zerolog.DebugLevel, "files").Msg("open from 1.5 at 874x492")
	f.Acted(Act{ID: "a8", What: "Space on canvas"})
	theLog.line(zerolog.DebugLevel, "decoder").Msg("moved to 46.084 s")
	theLog.jobLine(zerolog.InfoLevel, job).Msg("the job ended")

	lines := read()
	has(t, find(lines, `pressed button "New"`), map[string]any{"level": "info", "from": "window", "act": "a7", "video": "start.mp4"})
	has(t, find(lines, "video preview: opening"), map[string]any{"act": "a7"})
	has(t, find(lines, "open from"), map[string]any{"act": "a7", "from": "files"})
	has(t, find(lines, "moved to"), map[string]any{"act": "a8"})
	has(t, find(lines, "the job ended"), map[string]any{"act": "a7", "job": "job-3", "kind": "clip", "video": "start.mp4", "from": "jobs"})
}

// The trace lines kept in memory are the last few thousand, oldest first,
// and lines written from many places at once stay whole. Plan row 2.188.
func TestTheRecentTraceLinesAreTheLast(t *testing.T) {
	testLog(t)
	var wg sync.WaitGroup
	for w := range 4 {
		wg.Go(func() {
			for i := range recentMost / 2 {
				theLog.line(zerolog.TraceLevel, "files").Int("w", w).Int("i", i).Msg("pull")
			}
		})
	}
	wg.Wait()
	theLog.line(zerolog.TraceLevel, "files").Msg("the last")
	kept := strings.Split(strings.TrimSuffix(string(theLog.recent.all()), "\n"), "\n")
	if len(kept) != recentMost {
		t.Errorf("%d lines are kept, want %d", len(kept), recentMost)
	}
	for _, line := range kept {
		if !json.Valid([]byte(line)) {
			t.Fatalf("a kept line is not whole: %s", line)
		}
	}
	if !strings.Contains(kept[len(kept)-1], "the last") {
		t.Errorf("the newest line is not last: %s", kept[len(kept)-1])
	}
}

// An edit to a clip is a line of its own under edits, naming the video and
// the clip, at info when it was made and at warn with the reason when it
// was refused. Plan row 2.191.
func TestEditsAreLogged(t *testing.T) {
	svc, mine, plan := anEpisodeWithAPlan(t)
	heardEpisode(t, svc, mine, 30, [][3]any{{10.1, 10.4, "eins"}, {10.5, 10.9, "zwei"}, {11.0, 11.4, "drei"}})
	_, read := testLog(t)
	ctx := context.Background()
	view, err := svc.Captions(mine, plan, "01")
	if err != nil || len(view.Captions) == 0 {
		t.Fatalf("captions %+v: %v", view, err)
	}
	c := view.Captions[0]
	// The answers describe the clip for the interface, which needs the
	// video read, and this one is not a video. The edits are made either
	// way, and logged before.
	_, _ = svc.SetCaptionTime(ctx, mine, plan, "01", c.Last, "end", 11.0, false)
	_, _ = svc.SetWord(ctx, mine, plan, "01", 10.1, "")
	_, _ = svc.RemoveCaption(ctx, mine, plan, "01", c.First)
	_, _ = svc.SetCaptionTime(ctx, mine, plan, "01", 10.5, "start", 10.6, false)
	lines := read()
	moved := find(lines, "a caption edge was moved")
	if moved == nil {
		t.Fatalf("no caption edge in %v", lines)
	}
	has(t, moved, map[string]any{"level": "info", "from": "edits", "video": "ep.mp4", "clip": "01",
		"edge": "end", "word": c.Last, "at": 11.0, "out": 1, "back": 0})
	has(t, find(lines, "a word was corrected"), map[string]any{"level": "info", "word": 10.1, "removed": true})
	has(t, find(lines, "a caption was removed"), map[string]any{"level": "info", "word": c.First})
	// A word the caption does not begin on is refused, and said to be.
	refused := lines[len(lines)-1]
	has(t, refused, map[string]any{"level": "error", "msg": "a caption edge was moved", "edge": "start"})
	if refused["error"] == nil {
		t.Errorf("a refused edge says nothing of why: %v", refused)
	}
	// No line holds a word of the video.
	for _, l := range lines {
		for _, word := range []string{"eins", "zwei", "drei"} {
			if strings.Contains(fmt.Sprint(l), word) {
				t.Errorf("a line holds %q: %v", word, l)
			}
		}
	}
}

// An edit of a clip's words says how many words the clip corrects and
// removes once it is made, and never which, so a word that came back or a
// fix that went is read from the log. A video that still has the
// corrections of the whole video from before 2.249 says so once, with how
// many words they were.
func TestAnEditOfAClipsWordsSaysHowManyItChanges(t *testing.T) {
	svc, mine, plan := anEpisodeWithAPlan(t)
	heardEpisode(t, svc, mine, 30, [][3]any{{10.1, 10.4, "eins"}, {10.5, 10.9, "zwei"}, {11.0, 11.4, "drei"}})
	old := filepath.Join(svc.project(mine).LogsDir(), "corrections.json")
	if err := afero.WriteFile(svc.store.files, old, []byte(`{"10100": "Einz", "10500": ""}`), 0o644); err != nil {
		t.Fatal(err)
	}
	_, read := testLog(t)
	ctx := context.Background()
	if _, err := svc.Captions(mine, plan, "01"); err != nil {
		t.Fatal(err)
	}
	// The answers describe the clip for the interface, which needs the
	// video read, and this one is not a video. The edits are made either
	// way, and logged before.
	_, _ = svc.SetWord(ctx, mine, plan, "01", 10.5, "zwo")
	_, _ = svc.RemoveWord(ctx, mine, plan, "01", 11.0, -1)
	_, _ = svc.RemoveWord(ctx, mine, plan, "01", 11.0, -1)
	if _, err := svc.Captions(mine, plan, "01"); err != nil {
		t.Fatal(err)
	}
	lines := read()
	has(t, find(lines, "a word was corrected"), map[string]any{"level": "info", "clip": "01", "word": 10.5,
		"removed": false, "corrections": 1, "removed_words": 0})
	var removedLines []map[string]any
	for _, l := range lines {
		if l["msg"] == "a word was removed" {
			removedLines = append(removedLines, l)
		}
	}
	if len(removedLines) != 2 {
		t.Fatalf("%d lines say a word was removed: %v", len(removedLines), lines)
	}
	// The second removes nothing more, and says so by its counts.
	for _, l := range removedLines {
		has(t, l, map[string]any{"level": "info", "word": 11.0, "part": -1, "corrections": 1, "removed_words": 1})
	}
	var told []map[string]any
	for _, l := range lines {
		if s, _ := l["msg"].(string); strings.HasPrefix(s, "the corrections of the whole video") {
			told = append(told, l)
		}
	}
	if len(told) != 1 {
		t.Fatalf("the old corrections were told of %d times: %v", len(told), lines)
	}
	has(t, told[0], map[string]any{"level": "info", "video": "ep.mp4", "words": 2})
	for _, l := range lines {
		for _, word := range []string{"eins", "Einz", "zwei", "zwo", "drei"} {
			if strings.Contains(fmt.Sprint(l), word) {
				t.Errorf("a line holds %q: %v", word, l)
			}
		}
	}
}

// heardEpisode gives an episode of the library a transcript of these
// words, as far as seconds, the way a transcription leaves it.
func heardEpisode(t *testing.T, svc *FrameFairy, path string, seconds float64, words [][3]any) {
	t.Helper()
	p := svc.project(path)
	info, err := p.Files.Stat(path)
	if err != nil {
		t.Fatal(err)
	}
	model := svc.store.Settings().options().ASRModel
	if model == "" {
		model = engine.DefaultModelDir()
	}
	file := map[string]any{
		"version": 3, "model": filepath.Base(filepath.Clean(model)),
		"source": map[string]any{"name": filepath.Base(path), "size": info.Size(),
			"modified": info.ModTime().UTC().Format(time.RFC3339Nano)},
		"from": 0, "to": seconds, "mean": -20, "parts": [][2]float64{{0, seconds}}, "end": seconds,
		"words": words,
	}
	body, err := json.Marshal(file)
	if err != nil {
		t.Fatal(err)
	}
	if err := p.Files.MkdirAll(p.LogsDir(), 0o755); err != nil {
		t.Fatal(err)
	}
	if err := afero.WriteFile(p.Files, filepath.Join(p.LogsDir(), engine.TranscriptName(nil)), body, 0o644); err != nil {
		t.Fatal(err)
	}
	frames := make([]byte, 4*int(seconds/engine.FrameSeconds))
	if err := afero.WriteFile(p.Files, filepath.Join(p.LogsDir(), "words.frames"), frames, 0o644); err != nil {
		t.Fatal(err)
	}
	if _, err := p.Transcript(); err != nil {
		t.Fatalf("the transcript written is not read: %v", err)
	}
}

// An edit that takes long enough to be felt says so, and where the time
// went. The second of two edits of one video waits for the first, and the
// line says it waited rather than worked. Plan row 2.242.
func TestASlowEditSaysWhereItsTimeWent(t *testing.T) {
	svc, mine, _ := anEpisodeWithAPlan(t)
	_, read := testLog(t)
	first := make(chan struct{})
	done := make(chan struct{})
	go func() {
		defer close(done)
		_ = svc.edit(mine, func() error {
			close(first)
			time.Sleep(2 * slowEditAfter)
			return nil
		})
	}()
	<-first
	_ = svc.edit(mine, func() error { return nil })
	<-done
	var slow []map[string]any
	for _, l := range read() {
		if l["msg"] == "an edit of the clips was slow" {
			slow = append(slow, l)
		}
	}
	if len(slow) != 2 {
		t.Fatalf("%d edits said they were slow, not both: %v", len(slow), slow)
	}
	var worked, waited map[string]any
	for _, l := range slow {
		if l["edit"].(float64) >= float64(slowEditAfter.Milliseconds()) {
			worked = l
		} else {
			waited = l
		}
	}
	if worked == nil || waited == nil {
		t.Fatalf("one edit should have worked and one waited: %v", slow)
	}
	if waited["waited"].(float64) < float64(slowEditAfter.Milliseconds()) {
		t.Errorf("the second edit does not say it waited for the first: %v", waited)
	}
	has(t, waited, map[string]any{"level": "warn", "from": "edits", "video": "ep.mp4"})
}

// The caption layers of a stretch of playing are said in one line once
// the video asks for none for a while: how many were drawn, how many
// slower than a frame, and the slowest. A line for every slow layer was
// most of the log while a clip played, found by the second review.
func TestCaptionLayersAreSaidOnceAStretch(t *testing.T) {
	_, read := testLog(t)
	tally := &layerTally{byVideo: map[string]*tally{}}
	frame := 40 * time.Millisecond
	for _, drew := range []time.Duration{10, 30, 55, 20} {
		tally.add("/videos/start.mp4", drew*time.Millisecond, frame)
	}
	tally.mu.Lock()
	tally.byVideo["/videos/start.mp4"].quiet.Stop()
	tally.mu.Unlock()
	tally.said("/videos/start.mp4")
	has(t, find(read(), "caption layers drawn"), map[string]any{
		"level": "debug", "from": "captions", "video": "start.mp4", "drawn": 4, "slow": 1, "slowest_ms": 55,
	})
	if len(tally.byVideo) != 0 {
		t.Errorf("the stretch said is still counted: %v", tally.byVideo)
	}
}

// What the engine says while it draws the video preview's captions goes
// into the app's log under the video: that the words could not be
// measured and the caption file has no pill above all, which went nowhere
// before, found by the second review.
func TestThePreviewsEngineWritesIntoTheAppsLog(t *testing.T) {
	_, read := testLog(t)
	log := previewLog("/videos/start.mp4")
	log.Detail("could not measure the caption words, the highlight is off for this clip")
	log.Warn("the caption file of the video preview has no pill, the words could not be measured")
	log.Progress("measuring")
	lines := read()
	has(t, find(lines, "could not measure the caption words"), map[string]any{"level": "debug", "from": "captions", "video": "start.mp4"})
	has(t, find(lines, "the caption file of the video preview has no pill"), map[string]any{"level": "warn", "from": "captions", "video": "start.mp4"})
	if find(lines, "measuring") != nil {
		t.Error("the engine's progress was written")
	}
}

// Settings, Advanced shows the app's log, its files with the older ones,
// their size and their lines, and removes them all, and nothing else in
// their folder. The next line starts a new log, which says what went.
// Plan row 2.245.
//
// On the real disk: lumberjack writes the log and knows only the disk.
func TestTheLogCanBeRemovedWhole(t *testing.T) {
	path, read := testLog(t)
	dir := filepath.Dir(path)
	for i := range 4 {
		theLog.line(zerolog.InfoLevel, "app").Int("i", i).Msg("a line before")
	}
	var older bytes.Buffer
	z := gzip.NewWriter(&older)
	_, _ = z.Write([]byte("{}\n{}\n{}\n"))
	_ = z.Close()
	if err := os.WriteFile(filepath.Join(dir, "app-2026-10-09T10-00-00.000.log.gz"), older.Bytes(), 0o600); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(dir, "notes.txt"), []byte("mine"), 0o600); err != nil {
		t.Fatal(err)
	}
	s := &FrameFairy{}
	st := s.AppLog()
	if st.Path != path || st.Files != 2 || st.Lines != 4+3 || st.Bytes <= int64(older.Len()) {
		t.Fatalf("the log is %+v, want %s in 2 files with 7 lines", st, path)
	}

	// Lines keep coming while it is removed, from everywhere at once.
	var wg sync.WaitGroup
	for range 4 {
		wg.Go(func() {
			for range 200 {
				theLog.line(zerolog.DebugLevel, "app").Msg("a line while it goes")
			}
		})
	}
	after, err := s.RemoveAppLog()
	wg.Wait()
	if err != nil {
		t.Fatal(err)
	}
	if _, err := os.Stat(filepath.Join(dir, "app-2026-10-09T10-00-00.000.log.gz")); !os.IsNotExist(err) {
		t.Fatalf("the older file of the log is still there, %v", err)
	}
	if b, err := os.ReadFile(filepath.Join(dir, "notes.txt")); err != nil || string(b) != "mine" {
		t.Fatalf("a file beside the log that is not the log's went too, %v", err)
	}
	if after.Files != 1 || after.Path != path {
		t.Fatalf("after removing, the log is %+v, want one new file", after)
	}
	lines := read()
	if find(lines, "a line before") != nil {
		t.Fatal("a line from before is still in the log")
	}
	gone := find(lines, "the app's log was removed")
	// The lines said while it went count too, so at least the seven.
	if n, _ := gone["lines"].(float64); gone == nil || gone["files"] != float64(2) || n < 7 {
		t.Fatalf("the new log says %v, want what went", gone)
	}
}
