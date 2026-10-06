package main

import (
	"context"
	"errors"
	"os"
	"path/filepath"
	"strings"
	"sync"
	"testing"
	"time"

	"framefairy/engine"
)

// The store is the app's whole memory: the settings and the list of
// episodes, as plain files. It also decides which files the interface is
// allowed to read, so what it says is known matters beyond convenience.

func configHome(t *testing.T) string {
	t.Helper()
	home := t.TempDir()
	t.Setenv("HOME", home)
	t.Setenv("XDG_CONFIG_HOME", filepath.Join(home, "config"))
	return home
}

func TestStoreRemembersSettingsAndEpisodes(t *testing.T) {
	home := configHome(t)
	st := openStore()
	if st.Settings().Target != 0 {
		t.Errorf("a fresh store starts at %+v", st.Settings())
	}

	if err := st.UpdateSettings(func(set *Settings) {
		set.Target = 7
		set.HighlightColour = "#123456"
	}); err != nil {
		t.Fatal(err)
	}
	first := filepath.Join(home, "b.mp4")
	second := filepath.Join(home, "a.mp4")
	if _, err := st.AddEpisodes([]string{first, second, first}); err != nil {
		t.Fatal(err)
	}

	// A second store reads the same files back.
	again := openStore()
	if again.Settings().Target != 7 || again.Settings().HighlightColour != "#123456" {
		t.Errorf("settings came back as %+v", again.Settings())
	}
	episodes := again.Episodes()
	if len(episodes) != 2 || episodes[0] != first || episodes[1] != second {
		t.Errorf("episodes %v, want them in the order added and without the repeat", episodes)
	}
	// What the settings become for the engine.
	// A target typed is the search's to take, for its window, see
	// TestATargetIsForItsWindow. The engine's count follows the window.
	o := again.Settings().options()
	if o.Count != 0 || o.HighlightColour != "#123456" || o.Min != engine.DefaultOptions().Min {
		t.Errorf("options %+v", o)
	}

	if err := again.RemoveEpisode(second); err != nil {
		t.Fatal(err)
	}
	if got := openStore().Episodes(); len(got) != 1 || got[0] != first {
		t.Errorf("after removing %v", got)
	}
}

func TestKnownOnlyAcceptsFilesOfListedEpisodes(t *testing.T) {
	home := configHome(t)
	episode := filepath.Join(home, "ep.mp4")
	st := openStore()
	if _, err := st.AddEpisodes([]string{episode}); err != nil {
		t.Fatal(err)
	}
	work := engine.WorkDir(episode)
	known := []string{
		episode,
		filepath.Join(work, "logs", "clips.json"),
		filepath.Join(work, "out", "01_a.mp4"),
		filepath.Join(work, "logs", "..", "out", "01_a.mp4"),
	}
	for _, path := range known {
		if !st.Known(path) {
			t.Errorf("%s should be known", path)
		}
	}
	unknown := []string{
		filepath.Join(home, "andere.mp4"),
		filepath.Join(home, "ep.mp4.txt"),
		// A folder whose name only starts like the work folder.
		work + "-fremd/x.mp4",
		filepath.Join(work, "..", "geheim.txt"),
		work,
		home,
		"",
		"relativ.mp4",
	}
	for _, path := range unknown {
		if st.Known(path) {
			t.Errorf("%s should not be served", path)
		}
	}
}

// FuzzKnownStaysInTheLibrary checks the rule the interface relies on from
// the outside: a file is only served when it is an episode in the list or
// sits inside that episode's work folder.
func FuzzKnownStaysInTheLibrary(f *testing.F) {
	f.Add("ep.mp4", "ep.framefairy/out/01.mp4")
	f.Add("ep.mp4", "../../etc/passwd")
	f.Add("ep.mp4", "ep.framefairy-fremd/x")
	f.Add("a b.mp4", "a b.framefairy/logs/clips.json")
	f.Fuzz(func(t *testing.T, name, asked string) {
		if name == "" || strings.ContainsRune(name, 0) || strings.ContainsRune(asked, 0) {
			t.Skip()
		}
		home := configHome(t)
		episode := filepath.Join(home, name)
		if !filepath.IsLocal(name) {
			t.Skip()
		}
		st := openStore()
		if _, err := st.AddEpisodes([]string{episode}); err != nil {
			t.Skip()
		}
		path := filepath.Join(home, asked)
		if !st.Known(path) {
			return
		}
		if filepath.Clean(path) == filepath.Clean(episode) {
			return
		}
		rel, err := filepath.Rel(engine.WorkDir(episode), path)
		if err != nil || rel == "." || rel == ".." ||
			strings.HasPrefix(rel, ".."+string(filepath.Separator)) {
			t.Fatalf("%q is served but is not %q nor inside %q",
				path, episode, engine.WorkDir(episode))
		}
	})
}

func TestCancellingAQueuedJobNeverRunsIt(t *testing.T) {
	configHome(t)
	st := openStore()
	updates := make(chan JobUpdate, 100)
	q := newQueue(st, func(u JobUpdate) { updates <- u }, nil)

	release := make(chan struct{})
	running := make(chan struct{}, 1)
	ran := make(chan string, 4)
	first := q.add("a.mp4", "render", "Render", func(ctx context.Context, _ *engine.Project) (string, error) {
		running <- struct{}{}
		<-release
		ran <- "first"
		return "", nil
	})
	// The second job waits in the same lane, so it can still be called off.
	second := q.add("a.mp4", "render", "Render", func(context.Context, *engine.Project) (string, error) {
		ran <- "second"
		return "", nil
	})
	select {
	case <-running:
	case <-time.After(5 * time.Second):
		t.Fatal("the first job never started")
	}
	q.cancel(second.ID)
	close(release)

	select {
	case name := <-ran:
		if name != "first" {
			t.Fatalf("%s ran", name)
		}
	case <-time.After(5 * time.Second):
		t.Fatal("the first job never finished")
	}
	select {
	case name := <-ran:
		t.Fatalf("the cancelled job ran as %s", name)
	case <-time.After(300 * time.Millisecond):
	}

	states := map[string]string{}
	for _, j := range q.list() {
		states[j.ID] = j.State
	}
	if states[first.ID] != JobDone || states[second.ID] != JobCancelled {
		t.Errorf("states %v", states)
	}
	// The newest job of a kind is the one the interface asks about, and
	// clearing keeps nothing that is finished.
	if job, ok := q.find("a.mp4", "render"); !ok || job.ID != second.ID {
		t.Errorf("find gave %+v", job)
	}
	q.clear()
	if jobs := q.list(); len(jobs) != 0 {
		t.Errorf("clear left %+v", jobs)
	}
}

func TestAFailedJobKeepsItsReason(t *testing.T) {
	configHome(t)
	st := openStore()
	done := make(chan Job, 20)
	q := newQueue(st, func(u JobUpdate) {
		if u.Job.State == JobFailed || u.Job.State == JobCancelled {
			done <- u.Job
		}
	}, nil)

	q.add("a.mp4", "plan", "Find clips", func(context.Context, *engine.Project) (string, error) {
		return "", errors.New("no model on this machine")
	})
	q.add("b.mp4", "plan", "Find clips", func(context.Context, *engine.Project) (string, error) {
		return "", engine.ErrCancelled
	})
	seen := map[string]Job{}
	for len(seen) < 2 {
		select {
		case job := <-done:
			seen[job.Episode] = job
		case <-time.After(5 * time.Second):
			t.Fatalf("only %d job(s) reported: %v", len(seen), seen)
		}
	}
	if seen["a.mp4"].State != JobFailed || seen["a.mp4"].Error != "no model on this machine" {
		t.Errorf("failed job %+v", seen["a.mp4"])
	}
	if seen["b.mp4"].State != JobCancelled || seen["b.mp4"].Error != "" {
		t.Errorf("cancelled job %+v", seen["b.mp4"])
	}
}

// Two settings changed at once both stay. Each used to read the settings,
// change its own and write the whole of them back, so the one written
// second put the other back to what it had read: the number of clips set
// in the workspace while the captions were being dragged was lost.
func TestTwoSettingsChangedAtOnceBothStay(t *testing.T) {
	for round := range 20 {
		s, path, _ := library(t)
		var wg sync.WaitGroup
		wg.Add(2)
		go func() {
			defer wg.Done()
			for i := range 30 {
				_ = s.SetSearch(i%20+1, 1800, 20, 30)
			}
			_ = s.SetSearch(7, 1800, 20, 30)
		}()
		go func() {
			defer wg.Done()
			for i := range 30 {
				_ = s.SetCaptionsHeight(path, float64(120+40*(i%10)))
			}
			_ = s.SetCaptionsHeight(path, 600)
		}()
		wg.Wait()
		if set := s.store.Settings(); set.Target != 7 || set.CaptionY != 600 {
			t.Fatalf("round %d: %d clips at %v, both were set last to 7 at 600",
				round, set.Target, set.CaptionY)
		}
	}
}

// A settings file from before the target followed the window has a count,
// which every search took whatever its window. It is read as following the
// window, and a target typed since is kept.
func TestAnOldCountFollowsTheWindow(t *testing.T) {
	home := configHome(t)
	dir := filepath.Join(home, "config", "FrameFairy")
	if err := os.MkdirAll(dir, 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(dir, "settings.json"), []byte(`{"count": 8, "min": 20, "max": 30}`), 0o600); err != nil {
		t.Fatal(err)
	}
	st := openStore()
	if set := st.Settings(); set.Target != 0 || set.options().Count != 0 {
		t.Errorf("an old count became a target: %+v", set)
	}
}

// A target typed is for the window it was typed for. Another window, of
// another episode or drawn anew, follows its own suggestion, and the
// window it was typed for keeps it. Three typed for the six minutes of one
// episode went on asking for three in the half hour of the next.
func TestATargetIsForItsWindow(t *testing.T) {
	configHome(t)
	s := &FrameFairy{store: openStore()}
	if err := s.SetSearch(3, 360, 20, 30); err != nil {
		t.Fatal(err)
	}
	set := s.store.Settings()
	if set.targetFor(360) != 3 || set.targetFor(360.2) != 3 {
		t.Errorf("the window it was typed for asks for %d", set.targetFor(360))
	}
	if n := set.targetFor(1800); n != 0 {
		t.Errorf("a half hour window asks for the %d typed for six minutes", n)
	}
	// Cleared, every window follows its own.
	if err := s.SetSearch(0, 360, 20, 30); err != nil {
		t.Fatal(err)
	}
	if n := s.store.Settings().targetFor(360); n != 0 {
		t.Errorf("a cleared target still asks for %d", n)
	}
	// A target from before windows were kept with it is for none.
	if err := s.store.UpdateSettings(func(set *Settings) { set.Target, set.TargetWindow = 5, 0 }); err != nil {
		t.Fatal(err)
	}
	if n := s.store.Settings().targetFor(1800); n != 0 {
		t.Errorf("a target kept without its window asks for %d", n)
	}
}
