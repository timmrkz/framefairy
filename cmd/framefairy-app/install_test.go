package main

import (
	"os"
	"path/filepath"
	"testing"
	"time"
)

// An app bundle as small as can be: a program that says which build it is,
// and a link, the way a bundle's frameworks are links.
func bundle(t *testing.T, at, says string) {
	t.Helper()
	macos := filepath.Join(at, "Contents", "MacOS")
	if err := os.MkdirAll(macos, 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(macos, "framefairy-app"), []byte(says), 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.Symlink("MacOS/framefairy-app", filepath.Join(at, "Contents", "Current")); err != nil {
		t.Fatal(err)
	}
}

func says(t *testing.T, app string) string {
	t.Helper()
	data, err := os.ReadFile(filepath.Join(app, "Contents", "MacOS", "framefairy-app"))
	if err != nil {
		return "nothing: " + err.Error()
	}
	return string(data)
}

// The app, and a new build as the updater leaves it, unpacked in a folder
// of its own.
func twoBuilds(t *testing.T) (target, from string) {
	t.Helper()
	dir := t.TempDir()
	target = filepath.Join(dir, "Applications", "Frame Fairy.app")
	from = filepath.Join(dir, "wails-update-1", "Frame Fairy.app")
	bundle(t, target, "old")
	bundle(t, from, "new")
	return target, from
}

func quitAt(int, time.Duration) bool     { return true }
func neverQuits(int, time.Duration) bool { return false }

// Once the app is gone, the new build is where the app was, whole, and
// nothing is left beside it or where it was unpacked.
func TestAReadyBuildGoesInPlaceOfTheApp(t *testing.T) {
	target, from := twoBuilds(t)
	if err := installWhenGone(target, from, 42, quitAt); err != nil {
		t.Fatal(err)
	}
	if got := says(t, target); got != "new" {
		t.Errorf("the app says %q", got)
	}
	program := filepath.Join(target, "Contents", "MacOS", "framefairy-app")
	if info, err := os.Stat(program); err != nil || info.Mode().Perm()&0o100 == 0 {
		t.Errorf("the program is %v, %v", info, err)
	}
	if link, err := os.Readlink(filepath.Join(target, "Contents", "Current")); err != nil || link != "MacOS/framefairy-app" {
		t.Errorf("the link is %q, %v", link, err)
	}
	for _, left := range []string{target + ".old", filepath.Dir(from)} {
		if _, err := os.Stat(left); !os.IsNotExist(err) {
			t.Errorf("%s is still there", left)
		}
	}
}

// An app that did not quit after all is never replaced under itself.
func TestAnAppThatDidNotQuitIsLeftAlone(t *testing.T) {
	target, from := twoBuilds(t)
	if err := installWhenGone(target, from, 42, neverQuits); err == nil {
		t.Error("replaced an app that was still running")
	}
	if says(t, target) != "old" || says(t, from) != "new" {
		t.Errorf("the app says %q and the build %q", says(t, target), says(t, from))
	}
}

// The step only ever replaces an app, only with a build the updater
// unpacked, whatever its environment names.
func TestTheStepReplacesNothingElse(t *testing.T) {
	target, from := twoBuilds(t)
	elsewhere := filepath.Join(t.TempDir(), "Frame Fairy.app")
	bundle(t, elsewhere, "someone else's")
	for _, c := range []struct{ target, from string }{
		{filepath.Dir(target), from},
		{"Frame Fairy.app", from},
		{target, elsewhere},
		{target, filepath.Dir(from)},
	} {
		if err := installWhenGone(c.target, c.from, 42, quitAt); err == nil {
			t.Errorf("replaced %s with %s", c.target, c.from)
		}
	}
	if says(t, target) != "old" {
		t.Errorf("the app says %q", says(t, target))
	}
}

// A build that is gone by the time the app quits leaves the app as it was.
func TestAMissingBuildLeavesTheApp(t *testing.T) {
	target, from := twoBuilds(t)
	_ = os.RemoveAll(from)
	if err := installWhenGone(target, from, 42, quitAt); err == nil {
		t.Error("no error")
	}
	if says(t, target) != "old" {
		t.Errorf("the app says %q", says(t, target))
	}
}

// Across volumes a folder cannot be renamed, so it is copied, and a copy
// keeps the links and the modes an app needs.
func TestACopiedBuildKeepsItsLinksAndModes(t *testing.T) {
	_, from := twoBuilds(t)
	to := filepath.Join(t.TempDir(), "Frame Fairy.app")
	if err := copyTree(from, to); err != nil {
		t.Fatal(err)
	}
	if says(t, to) != "new" {
		t.Errorf("the copy says %q", says(t, to))
	}
	if info, _ := os.Lstat(filepath.Join(to, "Contents", "Current")); info == nil || info.Mode()&os.ModeSymlink == 0 {
		t.Error("the link became a file")
	}
	if info, _ := os.Stat(filepath.Join(to, "Contents", "MacOS", "framefairy-app")); info == nil || info.Mode().Perm()&0o100 == 0 {
		t.Error("the program lost its mode")
	}
}

func TestBundleOf(t *testing.T) {
	for exe, want := range map[string]string{
		"/Applications/Frame Fairy.app/Contents/MacOS/framefairy-app": "/Applications/Frame Fairy.app",
		"/Users/tim/framefairy/bin/framefairy-app":                    "",
	} {
		if got := bundleOf(exe); got != want {
			t.Errorf("%s: got %q", exe, got)
		}
	}
}

// Quitting with a build ready starts the step, once, for the app it runs
// from. With nothing ready, or after Relaunch, it starts nothing.
func TestQuittingWithABuildReadyInstallsIt(t *testing.T) {
	was := runningApp
	runningApp = func() string { return "/Applications/Frame Fairy.app" }
	t.Cleanup(func() { runningApp = was })

	cs := newChannelServer(t)
	cs.publish(t, [3]string{"main", "0.3.0-main.5", "main"})
	c, _ := newTestUpdating(t, cs, false)
	cleanStaged(t, c)
	var started [][2]string
	c.install = func(target, from string) error {
		started = append(started, [2]string{target, from})
		return nil
	}

	c.installOnQuit()
	if len(started) != 0 {
		t.Fatalf("started with nothing ready: %v", started)
	}
	_ = c.Follow("main")
	waitFor(t, c, "ready", func(s UpdateState) bool { return s.Phase == "ready" })
	c.installOnQuit()
	if len(started) != 1 || started[0][0] != "/Applications/Frame Fairy.app" || started[0][1] != c.u.DownloadedPath() {
		t.Fatalf("started %v", started)
	}

	started = nil
	c.mu.Lock()
	c.relaunching = true
	c.mu.Unlock()
	c.installOnQuit()
	if len(started) != 0 {
		t.Errorf("started after Relaunch: %v", started)
	}
}
