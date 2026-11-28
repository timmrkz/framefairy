package main

import (
	"errors"
	"os"
	"path/filepath"
	"sync"
	"sync/atomic"
	"testing"
	"time"
)

// settingsSay writes the app's settings as a build leaves them.
//
// On the real disk: the settings are a file in the app's folder.
func settingsSay(t *testing.T, folder, body string) {
	t.Helper()
	if err := os.WriteFile(filepath.Join(folder, "settings.json"), []byte(body), 0o600); err != nil {
		t.Fatal(err)
	}
}

// settingsIn is what the app's settings say.
//
// On the real disk: the settings are a file in the app's folder.
func settingsIn(t *testing.T, folder string) string {
	t.Helper()
	data, err := os.ReadFile(filepath.Join(folder, "settings.json"))
	if err != nil {
		t.Fatal(err)
	}
	return string(data)
}

// relaunched is the step on Relaunch, with a new build that does what
// start says when it is opened: the app opened is counted in opened.
func relaunched(t *testing.T, start func(step installStep)) (installStep, *[]string, *[]int) {
	t.Helper()
	target, from := twoBuilds(t)
	s := testStep(t, target, from, quitAt)
	s.open = true
	var opened []string
	var stopped []int
	s.launch = func(app string) error {
		opened = append(opened, says(t, app))
		if len(opened) == 1 {
			start(s)
		}
		return nil
	}
	s.started = func(folder string, _ time.Duration) bool {
		_, on := readTrial(folder)
		return !on
	}
	s.stop = func(pid int) { stopped = append(stopped, pid) }
	settingsSay(t, s.folder, "old settings")
	return s, &opened, &stopped
}

// A build put in place by Relaunch that says it started stays, and the
// build before goes.
//
// On the real disk: an update moves a bundle.
func TestABuildThatStartsStays(t *testing.T) {
	s, opened, stopped := relaunched(t, func(s installStep) {
		if undone, err := startTrial(s.folder, s.version, 77, nil); undone || err != nil {
			t.Fatalf("its first start undid it: %v", err)
		}
		if err := endTrial(s.folder, s.version); err != nil {
			t.Fatal(err)
		}
	})
	if err := s.run(); err != nil {
		t.Fatal(err)
	}
	if len(*opened) != 1 || (*opened)[0] != "new" || len(*stopped) != 0 {
		t.Errorf("opened %v, ended %v", *opened, *stopped)
	}
	if says(t, s.target) != "new" {
		t.Errorf("the app says %q", says(t, s.target))
	}
	if _, err := os.Stat(filepath.Join(s.folder, beforeDir)); !os.IsNotExist(err) {
		t.Errorf("the build before is still kept: %v", err)
	}
	if failedBuild(s.folder) != "" {
		t.Errorf("written down as failed: %q", failedBuild(s.folder))
	}
}

// A build put in place by Relaunch that does not say it started within the
// minute is ended, and the build before goes back and is opened. The
// app's settings stay as the new build left them: an older build reads
// them and drops what it does not know. The build is written down as one
// that did not start.
//
// On the real disk: an update moves a bundle.
func TestABuildThatDoesNotStartIsUndone(t *testing.T) {
	s, opened, stopped := relaunched(t, func(s installStep) {
		if _, err := startTrial(s.folder, s.version, 77, nil); err != nil {
			t.Fatal(err)
		}
		// It wrote the settings, and then hung.
		settingsSay(t, s.folder, "new settings")
	})
	if err := s.run(); err == nil {
		t.Error("the step said nothing of a build that did not start")
	}
	if len(*stopped) != 1 || (*stopped)[0] != 77 {
		t.Errorf("ended %v, want the start of the new build", *stopped)
	}
	if len(*opened) != 2 || (*opened)[1] != "old" {
		t.Errorf("opened %v, want the new build and then the one before", *opened)
	}
	if says(t, s.target) != "old" {
		t.Errorf("the app says %q", says(t, s.target))
	}
	if got := settingsIn(t, s.folder); got != "new settings" {
		t.Errorf("the settings say %q", got)
	}
	if failedBuild(s.folder) != s.version {
		t.Errorf("written down as failed: %q", failedBuild(s.folder))
	}
	for _, left := range []string{filepath.Join(s.folder, beforeDir), s.target + ".failed"} {
		if _, err := os.Stat(left); !os.IsNotExist(err) {
			t.Errorf("%s is still there", left)
		}
	}
}

// A build that says it started just as Relaunch gave up on it and ended
// it stays, and is opened again, since it did start. It is not written
// down as failed.
//
// On the real disk: an update moves a bundle.
func TestABuildThatSaysItStartedAsItIsEndedStays(t *testing.T) {
	s, opened, stopped := relaunched(t, func(s installStep) {
		if _, err := startTrial(s.folder, s.version, 77, nil); err != nil {
			t.Fatal(err)
		}
	})
	s.stop = func(pid int) {
		*stopped = append(*stopped, pid)
		if err := endTrial(s.folder, s.version); err != nil {
			t.Fatal(err)
		}
	}
	if err := s.run(); err != nil {
		t.Errorf("the step said a build that started did not: %v", err)
	}
	if len(*opened) != 2 || (*opened)[1] != "new" {
		t.Errorf("opened %v, want the new build twice", *opened)
	}
	if says(t, s.target) != "new" {
		t.Errorf("the app says %q", says(t, s.target))
	}
	if failedBuild(s.folder) != "" {
		t.Errorf("written down as failed: %q", failedBuild(s.folder))
	}
}

// A build the system does not load never goes in: the app stays as it
// was, is opened again on Relaunch, and the build is written down as one
// that did not start.
//
// On the real disk: an update would move a bundle.
func TestABuildThatDoesNotLoadNeverGoesIn(t *testing.T) {
	s, opened, _ := relaunched(t, func(installStep) {})
	s.loads = func(program string) error {
		if filepath.Base(program) != "framefairy-app" || says(t, filepath.Dir(filepath.Dir(filepath.Dir(program)))) != "new" {
			t.Errorf("loaded %s", program)
		}
		return errors.New("Library not loaded: @rpath/libonnxruntime.dylib")
	}
	if err := s.run(); err == nil {
		t.Error("no error")
	}
	if says(t, s.target) != "old" || len(*opened) != 1 || (*opened)[0] != "old" {
		t.Errorf("the app says %q, opened %v", says(t, s.target), *opened)
	}
	if failedBuild(s.folder) != s.version {
		t.Errorf("written down as failed: %q", failedBuild(s.folder))
	}
	if _, err := os.Stat(filepath.Join(s.folder, beforeDir)); !os.IsNotExist(err) {
		t.Errorf("something was kept: %v", err)
	}
}

// A build put in place on quit is opened later, with nobody waiting. Two
// starts that do not say they started are let be, and the third puts the
// build before back and opens it.
//
// On the real disk: an update moves a bundle.
func TestABuildPutInOnQuitIsUndoneAfterTwoFailedStarts(t *testing.T) {
	target, from := twoBuilds(t)
	s := testStep(t, target, from, quitAt)
	if err := s.run(); err != nil {
		t.Fatal(err)
	}
	var opened []string
	launch := func(app string) error { opened = append(opened, says(t, app)); return nil }
	for i := range failedStarts {
		if undone, err := startTrial(s.folder, s.version, 100+i, launch); undone || err != nil {
			t.Fatalf("start %d undid it: %v", i+1, err)
		}
	}
	undone, err := startTrial(s.folder, s.version, 200, launch)
	if !undone || err != nil {
		t.Fatalf("the third start did not undo it: %v", err)
	}
	if says(t, target) != "old" || len(opened) != 1 || opened[0] != "old" {
		t.Errorf("the app says %q, opened %v", says(t, target), opened)
	}
	if failedBuild(s.folder) != s.version {
		t.Errorf("written down as failed: %q", failedBuild(s.folder))
	}
}

// A start that says it started, even after one that did not, ends the
// trial, and the start of another build finds none.
//
// On the real disk: the trial is a file in the app's folder.
func TestABuildThatStartsTheSecondTimeStays(t *testing.T) {
	target, from := twoBuilds(t)
	s := testStep(t, target, from, quitAt)
	if err := s.run(); err != nil {
		t.Fatal(err)
	}
	for range 2 {
		if undone, err := startTrial(s.folder, s.version, 1, nil); undone || err != nil {
			t.Fatalf("undone: %v", err)
		}
	}
	if err := endTrial(s.folder, "0.3.0-main.4"); err != nil {
		t.Fatal(err)
	}
	if _, on := readTrial(s.folder); !on {
		t.Fatal("another build ended the trial")
	}
	if err := endTrial(s.folder, s.version); err != nil {
		t.Fatal(err)
	}
	if undone, err := startTrial(s.folder, s.version, 1, nil); undone || err != nil {
		t.Errorf("a start after it said it started undid it: %v", err)
	}
	if says(t, target) != "new" {
		t.Errorf("the app says %q", says(t, target))
	}
}

// The interface asks where updates stand from several requests at once,
// and the build says it started once.
func TestABuildSaysItStartedOnce(t *testing.T) {
	t.Parallel()
	var said atomic.Int32
	c := &updating{started: func() { said.Add(1) }}
	var wg sync.WaitGroup
	for range 16 {
		wg.Go(c.saidStarted)
	}
	wg.Wait()
	if said.Load() != 1 {
		t.Errorf("said it started %d times", said.Load())
	}
}

// Relaunch puts the build in place through the app's own step, told to
// open it, and then quits.
//
// On the real disk: an update is a program that is unpacked on the disk.
func TestRelaunchGoesThroughTheStep(t *testing.T) {
	t.Parallel()
	cs := newChannelServer(t)
	cs.publish(t, [3]string{"main", "0.3.0-main.5", "main"})
	c, _ := newTestUpdating(t, cs, false)
	c.app = inApplications
	cleanStaged(t, c)
	var asked []string
	quit := 0
	c.install = func(target, from, version string, open bool) error {
		if !open {
			t.Error("Relaunch did not ask the step to open the build")
		}
		asked = append(asked, target, from, version)
		return nil
	}
	c.quit = func() { quit++ }
	_ = c.Follow("main")
	waitFor(t, c, "ready", func(s UpdateState) bool { return s.Phase == "ready" })
	if err := c.Restart(); err != nil {
		t.Fatal(err)
	}
	if len(asked) != 3 || asked[0] != "/Applications/Frame Fairy.app" || asked[1] != c.staged() || asked[2] != "0.3.0-main.5" || quit != 1 {
		t.Errorf("asked %v, quit %d", asked, quit)
	}
	if !c.relaunchingNow() {
		t.Error("not relaunching")
	}
}

// A build that went in and did not start is not downloaded again by the
// app's own look, and the page says so with the build before running. A
// pick of its channel on the Updates page takes it all the same, since
// the person may know why it did not start is gone.
//
// On the real disk: an update is a program that is unpacked on the disk.
func TestABuildThatDidNotStartIsTakenOnlyByHand(t *testing.T) {
	t.Parallel()
	cs := newChannelServer(t)
	cs.publish(t, [3]string{"main", "0.3.0-main.5", "main"})
	c, _ := newTestUpdating(t, cs, false)
	cleanStaged(t, c)
	c.failed = func() string { return "0.3.0-main.5" }
	c.mu.Lock()
	c.state.Picked = "main"
	c.mu.Unlock()
	c.check()
	s := waitFor(t, c, "unstarted", func(s UpdateState) bool { return s.Phase == "unstarted" })
	if s.Next != "0.3.0-main.5" {
		t.Errorf("%+v", s)
	}
	if c.staged() != "" {
		t.Errorf("downloaded %s", c.staged())
	}
	_ = c.Follow("main")
	waitFor(t, c, "ready", func(s UpdateState) bool { return s.Phase == "ready" && s.Next == "0.3.0-main.5" })
}
