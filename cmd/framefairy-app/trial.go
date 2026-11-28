package main

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io/fs"
	"os"
	"os/exec"
	"path/filepath"
	"time"

	"github.com/rs/zerolog"
	"github.com/spf13/afero"

	"framefairy/engine/fileio"
)

// A build put in place is on trial until it says it started. Wails'
// helper put the build before back only when the new one could not be
// opened at all, and removed it the moment it ran, and the step on quit
// kept nothing. So a build that opened and then crashed, or whose window
// never came up, stayed, and an app that cannot start cannot update
// itself either. Now the step that puts a build in place first starts its
// program once, told to end at once, so a library macOS refuses is seen
// before anything moves. It keeps the build before in update-before in
// the app's folder. The new build says it started once its interface has
// asked where updates stand, and the build before goes. Until then:
//
//   - On Relaunch the step opens the new build and waits a minute for it
//     to say so. When it does not, the step ends it and puts the build
//     before back.
//   - Put in place on quit, the new build is opened later, with nobody
//     waiting, so each of its starts counts itself, and its third start
//     puts the build before back, after two that did not say they
//     started.
//
// The app's files stay as they are: an older build reads them and drops
// only what it does not know. A build that did not start is written down
// in update-failed.json, and the app does not take it again by itself,
// only when its channel is picked again on the Updates page. See
// docs/decisions/2026-11-28-a-build-that-does-not-start-is-undone.md.
const (
	// envLoadCheck asks the program only to be loaded, and to end.
	envLoadCheck = "FRAMEFAIRY_LOAD_CHECK"
	beforeDir    = "update-before"
	trialName    = "trial.json"
	failedName   = "update-failed.json"
	// startedWithin is how long Relaunch gives the new build to say it
	// started. It opens its window in a second or two, and the first start
	// after an update also has macOS check its signature.
	startedWithin = time.Minute
	// failedStarts is how many starts of a build put in place on quit may
	// not say they started before the next one puts the build before back.
	failedStarts = 2
)

// appFolder is the app's own folder, where its settings, its library and
// what updating keeps lie.
func appFolder() string {
	dir, err := os.UserConfigDir()
	if err != nil {
		dir = os.TempDir()
	}
	return filepath.Join(dir, "FrameFairy")
}

// trial is the build on trial, in update-before/trial.json.
type trial struct {
	// Version is the build put in place, and App the .app it went in.
	Version string `json:"version"`
	App     string `json:"app"`
	// Starts counts the starts of it that have not said they started, and
	// PID is the newest of them, which Relaunch ends when it gives up.
	Starts int `json:"starts"`
	PID    int `json:"pid,omitempty"`
}

func trialPath(folder string) string { return filepath.Join(folder, beforeDir, trialName) }

func readTrial(folder string) (trial, bool) {
	var t trial
	data, err := os.ReadFile(trialPath(folder))
	if err != nil || json.Unmarshal(data, &t) != nil || t.Version == "" || !filepath.IsAbs(t.App) {
		return trial{}, false
	}
	return t, true
}

// saveFile writes a file of the app's folder whole, the way the store
// writes its own. The step runs before the store is open, so it writes
// through the disk itself.
func saveFile(path string, v any) error {
	body, err := json.MarshalIndent(v, "", "  ")
	if err != nil {
		return err
	}
	return fileio.WriteAtomic(afero.NewOsFs(), path, body)
}

func writeTrial(folder string, t trial) error { return saveFile(trialPath(folder), t) }

// failedBuild is the build that last did not start, empty when none did.
func failedBuild(folder string) string {
	var f struct {
		Version string `json:"version"`
	}
	data, err := os.ReadFile(filepath.Join(folder, failedName))
	if err != nil || json.Unmarshal(data, &f) != nil {
		return ""
	}
	return f.Version
}

func writeFailed(folder, version string) error {
	return saveFile(filepath.Join(folder, failedName), map[string]string{"version": version})
}

// undo puts the build before back in place of the build on trial, writes
// that build down as one that did not start, and opens the build before.
// stop ends the build on trial when it still runs, nil when it is the one
// undoing itself.
func undo(folder string, stop func(pid int), launch func(app string) error) error {
	t, ok := readTrial(folder)
	if !ok {
		return nil
	}
	if stop != nil && t.PID > 0 {
		stop(t.PID)
		// It may have said it started as it was ended. Then it stays, and
		// is opened again.
		if _, still := readTrial(folder); !still {
			return launch(t.App)
		}
	}
	before := filepath.Join(folder, beforeDir)
	old := filepath.Join(before, filepath.Base(t.App))
	if _, err := os.Stat(old); err != nil {
		return fmt.Errorf("the build before is not there to put back: %w", err)
	}
	failed := t.App + ".failed"
	_ = os.RemoveAll(failed)
	if err := os.Rename(t.App, failed); err != nil {
		return fmt.Errorf("the build that did not start could not be moved aside: %w", err)
	}
	if err := move(old, t.App); err != nil {
		if back := os.Rename(failed, t.App); back != nil {
			return fmt.Errorf("the build before did not go back (%w), and the app is %s: %w", err, failed, back)
		}
		return fmt.Errorf("the build before did not go back, the app is as it was: %w", err)
	}
	_ = os.RemoveAll(failed)
	_ = os.RemoveAll(before)
	return errors.Join(writeFailed(folder, t.Version), launch(t.App))
}

// startTrial is the first thing a start does once its log is open, before
// it reads the app's files. When this build is on trial it counts the
// start, and on the start after failedStarts that did not say they
// started it puts the build before back, and the app ends. A trial of
// another build is over: whatever undid it already ran.
func startTrial(folder, version string, pid int, launch func(app string) error) (undone bool, err error) {
	t, ok := readTrial(folder)
	if !ok {
		return false, nil
	}
	if t.Version != version {
		return false, os.RemoveAll(filepath.Join(folder, beforeDir))
	}
	if t.Starts >= failedStarts {
		return true, undo(folder, nil, launch)
	}
	t.Starts++
	t.PID = pid
	return false, writeTrial(folder, t)
}

// endTrial is the build on trial saying it started: the build before
// goes. Any other build has nothing to end.
func endTrial(folder, version string) error {
	t, ok := readTrial(folder)
	if !ok || t.Version != version {
		return nil
	}
	return os.RemoveAll(filepath.Join(folder, beforeDir))
}

// saysStarted waits until the build on trial said it started, or the
// time is up. It looks four times a second: it runs in the step, after
// the app has quit, where nothing else would tell it.
func saysStarted(folder string, within time.Duration) bool {
	deadline := time.Now().Add(within)
	for {
		if _, err := os.Stat(trialPath(folder)); errors.Is(err, fs.ErrNotExist) {
			return true
		}
		if !time.Now().Before(deadline) {
			return false
		}
		time.Sleep(250 * time.Millisecond)
	}
}

// loads starts a program told only to be loaded and to end, so the
// system loads it and every library it links. A library macOS refuses
// under the hardened runtime ends a program before any of our code runs,
// so only another program can see it.
func loads(program string) error {
	ctx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
	defer cancel()
	cmd := exec.CommandContext(ctx, program)
	cmd.Env = append(os.Environ(), envLoadCheck+"=1")
	out, err := cmd.CombinedOutput()
	if err != nil {
		if len(out) > 600 {
			out = out[:600]
		}
		return fmt.Errorf("%w: %s", err, bytes.TrimSpace(out))
	}
	return nil
}

// launchApp opens an app the way Finder does, as a new instance.
func launchApp(app string) error {
	return exec.Command("open", "-n", app).Run()
}

// undoIfUnstarted is startTrial for main, in the app's folder.
func undoIfUnstarted() bool {
	undone, err := startTrial(appFolder(), runningVersion(), os.Getpid(), launchApp)
	if err != nil {
		theLog.line(zerolog.ErrorLevel, "updates").Err(err).Msg("the build on trial could not be counted or put back")
	}
	if undone {
		theLog.line(zerolog.WarnLevel, "updates").Str("build", runningVersion()).
			Msg("this build did not say it started twice, so the build before it was put back")
	}
	return undone
}
