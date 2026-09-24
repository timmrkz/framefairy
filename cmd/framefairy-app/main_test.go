package main

import (
	"context"
	"net/http"
	"net/http/httptest"
	"net/url"
	"os"
	"os/exec"
	"path/filepath"
	"testing"
	"time"

	"framefairy/engine"
)

func TestPlanWaitsAndReportsAFailedTranscription(t *testing.T) {
	if _, err := exec.LookPath("ffmpeg"); err != nil {
		t.Skip("ffmpeg is not installed")
	}
	home := t.TempDir()
	t.Setenv("HOME", home)
	t.Setenv("XDG_CONFIG_HOME", filepath.Join(home, "config"))
	source := filepath.Join(home, "episode.mp4")
	out, err := exec.Command("ffmpeg", "-loglevel", "error", "-f", "lavfi", "-i",
		"testsrc=s=320x180:r=25:d=4", "-f", "lavfi", "-i", "sine=d=4", "-shortest",
		"-c:v", "libx264", "-preset", "ultrafast", "-c:a", "aac", source).CombinedOutput()
	if err != nil {
		t.Fatalf("%s %s", err, out)
	}

	st := openStore()
	settings := st.Settings()
	settings.ASRModel = filepath.Join(home, "no-model")
	if err := st.SetSettings(settings); err != nil {
		t.Fatal(err)
	}
	if _, err := st.AddEpisodes([]string{source}); err != nil {
		t.Fatal(err)
	}
	updates := make(chan JobUpdate, 1000)
	svc := &FrameFairy{store: st}
	svc.jobs = newQueue(st, func(u JobUpdate) { updates <- u }, nil)

	// Finding clips queues the transcription itself, and fails with its
	// reason when the transcription fails.
	job := svc.Plan(source, engine.PlanRequest{To: 2, Count: 1, Min: 1, Max: 2})
	if job.Lane != LaneWork {
		t.Errorf("plan lane %s", job.Lane)
	}
	deadline := time.After(30 * time.Second)
	for {
		select {
		case u := <-updates:
			if u.Job.ID != job.ID || (u.Job.State != JobFailed && u.Job.State != JobDone) {
				continue
			}
			if u.Job.State != JobFailed {
				t.Fatalf("plan ended %s", u.Job.State)
			}
			if u.Job.Error == "" {
				t.Errorf("no reason given")
			}
			tr, ok := svc.jobs.find(source, "transcribe")
			if !ok || tr.Lane != LaneTranscribe || tr.State != JobFailed {
				t.Errorf("transcription job %+v", tr)
			}
			return
		case <-deadline:
			t.Fatalf("the plan never finished: %+v", svc.jobs.list())
		}
	}
}

func TestLanesRunSideBySide(t *testing.T) {
	home := t.TempDir()
	t.Setenv("HOME", home)
	t.Setenv("XDG_CONFIG_HOME", filepath.Join(home, "config"))
	st := openStore()
	q := newQueue(st, func(JobUpdate) {}, nil)
	release := make(chan struct{})
	started := make(chan string, 2)
	block := func(name string) func(context.Context, *engine.Project) (string, error) {
		return func(ctx context.Context, _ *engine.Project) (string, error) {
			started <- name
			select {
			case <-release:
				return "", nil
			case <-ctx.Done():
				return "", engine.ErrCancelled
			}
		}
	}
	q.add("a.mp4", "transcribe", "Transcription", block("transcribe"))
	q.add("a.mp4", "plan", "Find clips", block("plan"))
	for i := 0; i < 2; i++ {
		select {
		case <-started:
		case <-time.After(5 * time.Second):
			t.Fatal("the two lanes did not run at the same time")
		}
	}
	close(release)
}

func TestMediaRouteServesRangesOfKnownFilesOnly(t *testing.T) {
	home := t.TempDir()
	t.Setenv("HOME", home)
	t.Setenv("XDG_CONFIG_HOME", filepath.Join(home, "config"))
	source := filepath.Join(home, "ep.mp4")
	other := filepath.Join(home, "secret.txt")
	if err := os.WriteFile(source, []byte("0123456789"), 0o644); err != nil {
		t.Fatal(err)
	}
	_ = os.WriteFile(other, []byte("nope"), 0o644)
	st := openStore()
	if _, err := st.AddEpisodes([]string{source}); err != nil {
		t.Fatal(err)
	}
	handler := mediaMiddleware(st)(http.NotFoundHandler())

	req := httptest.NewRequest("GET", "/media/?path="+url.QueryEscape(source), nil)
	req.Header.Set("Range", "bytes=2-5")
	rec := httptest.NewRecorder()
	handler.ServeHTTP(rec, req)
	if rec.Code != http.StatusPartialContent || rec.Body.String() != "2345" {
		t.Errorf("range got %d %q", rec.Code, rec.Body.String())
	}

	for _, path := range []string{other, filepath.Join(engine.WorkDir(source), "..", "secret.txt"), "relative.mp4"} {
		rec = httptest.NewRecorder()
		handler.ServeHTTP(rec, httptest.NewRequest("GET", "/media/?path="+url.QueryEscape(path), nil))
		if rec.Code != http.StatusNotFound {
			t.Errorf("%s served with %d", path, rec.Code)
		}
	}
}

// An episode that has not been transcribed yet is a state every episode
// starts in. The reads the workspace makes on opening must answer with
// nothing rather than fail, or the log fills with errors nobody can act on.
func TestAnEpisodeWithoutATranscriptReadsEmpty(t *testing.T) {
	home := t.TempDir()
	t.Setenv("HOME", home)
	t.Setenv("XDG_CONFIG_HOME", filepath.Join(home, "config"))
	source := filepath.Join(home, "ep.mp4")
	if err := os.WriteFile(source, []byte("not really a video"), 0o644); err != nil {
		t.Fatal(err)
	}
	st := openStore()
	if _, err := st.AddEpisodes([]string{source}); err != nil {
		t.Fatal(err)
	}
	svc := &FrameFairy{store: st}

	peaks, err := svc.Waveform(source, 0, 60, 100)
	if err != nil || len(peaks) != 0 {
		t.Errorf("waveform got %d peaks, %v", len(peaks), err)
	}
	words, err := svc.Words(source, 0, 60)
	if err != nil || len(words.Words) != 0 {
		t.Errorf("words got %d words, %v", len(words.Words), err)
	}
	if words.KeepPause <= 0 {
		t.Errorf("keepPause %v", words.KeepPause)
	}
	room, err := svc.Room(source)
	if err != nil || len(room.Lines) != 0 || room.Chars <= 0 || room.Rate != engine.SpokenChars {
		t.Errorf("room %+v, %v", room, err)
	}
}
