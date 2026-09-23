package main

import (
	"context"
	"net/http"
	"net/http/httptest"
	"net/url"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"framefairy/engine"
)

// The window may only reach the episodes in the library and what belongs to
// them. Every call that takes a path is a way in, so every one of them is
// tried here with a path that is not the library's.

func library(t *testing.T) (*FrameFairy, string, string) {
	t.Helper()
	home := t.TempDir()
	t.Setenv("HOME", home)
	t.Setenv("XDG_CONFIG_HOME", filepath.Join(home, "config"))
	mine := filepath.Join(home, "ep.mp4")
	if err := os.WriteFile(mine, []byte("video"), 0o644); err != nil {
		t.Fatal(err)
	}
	st := openStore()
	if _, err := st.AddEpisodes([]string{mine}); err != nil {
		t.Fatal(err)
	}
	svc := &FrameFairy{store: st}
	svc.jobs = newQueue(st, func(JobUpdate) {}, func(string) {})
	return svc, mine, home
}

func TestNothingOutsideTheLibraryIsTouched(t *testing.T) {
	svc, _, home := library(t)
	ctx := context.Background()
	other := filepath.Join(home, "not-mine.mp4")
	if err := os.WriteFile(other, []byte("someone else's"), 0o644); err != nil {
		t.Fatal(err)
	}

	if status := svc.Episode(other); status.Source != "" {
		t.Errorf("Episode answered for %s: %+v", other, status)
	}
	if _, err := svc.Source(ctx, other); err == nil {
		t.Error("Source answered for a file that is not in the library")
	}
	if _, err := svc.Clips(ctx, other); err == nil {
		t.Error("Clips answered for a file that is not in the library")
	}
	if _, err := svc.Words(other, 0, 10); err == nil {
		t.Error("Words answered for a file that is not in the library")
	}
	if _, err := svc.Waveform(other, 0, 10, 100); err == nil {
		t.Error("Waveform answered for a file that is not in the library")
	}
	if _, err := svc.Coverage(ctx, other, 20); err == nil {
		t.Error("Coverage answered for a file that is not in the library")
	}
	if job := svc.Transcribe(other); job.State != JobFailed {
		t.Errorf("Transcribe queued %s: %s", other, job.State)
	}
	if job := svc.Plan(other, engine.PlanRequest{To: 10}); job.State != JobFailed {
		t.Errorf("Plan queued %s: %s", other, job.State)
	}
	if job := svc.Render(other, engine.RenderRequest{}); job.State != JobFailed {
		t.Errorf("Render queued %s: %s", other, job.State)
	}
	// A plan of another episode is a path of its own, and it is read and
	// written to, so it is checked as well.
	elsewhere := filepath.Join(home, "elsewhere.json")
	if err := os.WriteFile(elsewhere, []byte(`{"clips": []}`), 0o644); err != nil {
		t.Fatal(err)
	}
	mine := svc.store.Episodes()[0]
	if job := svc.Render(mine, engine.RenderRequest{Plan: elsewhere}); job.State != JobFailed {
		t.Errorf("Render used a plan outside the library: %s", job.State)
	}
	if _, err := svc.RemoveSearch(ctx, other, 0, 10); err == nil {
		t.Error("RemoveSearch answered for a file that is not in the library")
	}
	if !fileExists(elsewhere) {
		t.Error("a file outside the library was deleted")
	}
	// Nothing may have been written beside a file that is not an episode.
	if _, err := os.Stat(engine.WorkDir(other)); err == nil {
		t.Errorf("a work folder was made at %s", engine.WorkDir(other))
	}
}

// A link inside a work folder points wherever it likes. What is served has
// to be what it says it is, so the link is followed before it is judged.
func TestTheMediaRouteDoesNotFollowLinksOutOfTheLibrary(t *testing.T) {
	svc, mine, home := library(t)
	secret := filepath.Join(home, "secret.txt")
	if err := os.WriteFile(secret, []byte("not for the window"), 0o644); err != nil {
		t.Fatal(err)
	}
	work := engine.WorkDir(mine)
	if err := os.MkdirAll(work, 0o755); err != nil {
		t.Fatal(err)
	}
	link := filepath.Join(work, "clip.mp4")
	if err := os.Symlink(secret, link); err != nil {
		t.Skipf("no symlinks here: %s", err)
	}
	handler := mediaMiddleware(svc.store)(http.NotFoundHandler())
	rec := httptest.NewRecorder()
	handler.ServeHTTP(rec, httptest.NewRequest("GET", "/media/?path="+url.QueryEscape(link), nil))
	if rec.Code != http.StatusNotFound {
		t.Errorf("a link out of the work folder was served with %d: %q", rec.Code, rec.Body.String())
	}
	if !svc.store.Known(link) {
		return
	}
	t.Error("Known says a link out of the work folder belongs to the episode")
}

// Removing a part takes the clips in it out of the plans. Anything else
// in the work folder, a render or a transcript, stays where it is.
func TestRemoveSearchOnlyTouchesPlans(t *testing.T) {
	svc, mine, _ := library(t)
	ctx := context.Background()
	work := engine.WorkDir(mine)
	logs := filepath.Join(work, "logs")
	if err := os.MkdirAll(logs, 0o755); err != nil {
		t.Fatal(err)
	}
	words := filepath.Join(logs, "words.json")
	if err := os.WriteFile(words, []byte(`{"version": 3, "words": []}`), 0o644); err != nil {
		t.Fatal(err)
	}
	rendered := filepath.Join(work, "01_clip.mp4")
	if err := os.WriteFile(rendered, []byte("finished"), 0o644); err != nil {
		t.Fatal(err)
	}
	plan := filepath.Join(logs, "clips-0-600.json")
	if err := os.WriteFile(plan, []byte(`{"planned_with": {"from": 0, "to": 600},
		"clips": [{"id": "01", "slug": "one", "title": "One",
		"segments": [{"start": 10, "end": 40}]}]}`), 0o644); err != nil {
		t.Fatal(err)
	}
	gone, err := svc.RemoveSearch(ctx, mine, 0, 600)
	if err != nil {
		t.Fatal(err)
	}
	if gone != 1 {
		t.Errorf("%d clips went, not 1", gone)
	}
	if fileExists(plan) {
		t.Error("the plan with nothing left of its window stayed")
	}
	for _, path := range []string{words, rendered} {
		if !fileExists(path) {
			t.Errorf("%s was deleted", filepath.Base(path))
		}
	}
}

// The media route is for files. A folder would be answered with a listing
// of what is in it, which is nobody's business but the machine's.
func TestTheMediaRouteServesFilesOnly(t *testing.T) {
	svc, mine, _ := library(t)
	work := engine.WorkDir(mine)
	if err := os.MkdirAll(filepath.Join(work, "logs"), 0o755); err != nil {
		t.Fatal(err)
	}
	handler := mediaMiddleware(svc.store)(http.NotFoundHandler())
	for _, path := range []string{work, filepath.Join(work, "logs")} {
		rec := httptest.NewRecorder()
		handler.ServeHTTP(rec, httptest.NewRequest("GET", "/media/?path="+url.QueryEscape(path), nil))
		if rec.Code != http.StatusNotFound {
			t.Errorf("%s was served with %d: %q", path, rec.Code, rec.Body.String())
		}
	}
}

// A job that goes wrong in a way nobody planned for must not take the app
// with it. The window, the other lane and whatever is being transcribed all
// hang on this process staying alive.
func TestAJobThatPanicsOnlyFailsItself(t *testing.T) {
	svc, mine, _ := library(t)
	updates := make(chan Job, 100)
	svc.jobs = newQueue(svc.store, func(u JobUpdate) { updates <- u.Job }, func(string) {})

	bad := svc.jobs.add(mine, "render", "Render", func(context.Context, *engine.Project) (string, error) {
		var clips []string
		return clips[3], nil
	})
	good := svc.jobs.add(mine, "render", "Render", func(context.Context, *engine.Project) (string, error) {
		return "still here", nil
	})
	states := map[string]string{}
	for deadline := time.Now().Add(10 * time.Second); time.Now().Before(deadline); {
		select {
		case job := <-updates:
			states[job.ID] = job.State
			if job.Error != "" && job.ID == bad.ID && !strings.Contains(job.Error, "unexpectedly") {
				t.Errorf("the reason was %q", job.Error)
			}
		case <-time.After(100 * time.Millisecond):
		}
		if states[bad.ID] == JobFailed && states[good.ID] == JobDone {
			return
		}
	}
	t.Fatalf("the panicking job ended as %q and the one after it as %q",
		states[bad.ID], states[good.ID])
}

// Everything about an episode lives in a folder named after it without its
// extension. Two files that would share one cannot both be in the library,
// or they would share a transcript, clip sets and rendered names.
func TestTwoEpisodesCannotShareAWorkFolder(t *testing.T) {
	svc, mine, home := library(t)
	twin := filepath.Join(home, "ep.mov")
	if err := os.WriteFile(twin, []byte("the same episode, another wrapper"), 0o644); err != nil {
		t.Fatal(err)
	}
	if engine.WorkDir(twin) != engine.WorkDir(mine) {
		t.Skip("work folders are not named after the file here")
	}
	_, err := svc.store.AddEpisodes([]string{twin})
	if err == nil {
		t.Error("the second file was added without a word")
	} else if !strings.Contains(err.Error(), "share the folder") {
		t.Errorf("the reason was %q", err)
	}
	for _, ep := range svc.store.Episodes() {
		if ep == twin {
			t.Fatal("both files are in the library")
		}
	}
}
