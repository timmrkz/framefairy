package main

import (
	"errors"
	"fmt"
	"io"
	"log"
	"os"
	"os/exec"
	"path/filepath"
	"strconv"
	"strings"
	"time"
)

// A build that is ready is put in place by a step of our own, on quit
// and on Relaunch: the same program, started again with the environment
// below, which waits for the app to be gone, checks the new build loads,
// keeps the build before, moves the new app into its place, and on
// Relaunch opens it and waits for it to say it started. On quit it opens
// nothing, the way Chrome does it: the next time the app is opened it is
// the new build. Wails' own helper kept nothing, see trial.go.
//
// All it is told is in its environment, never in its arguments, so a flag
// given to the app is never mistaken for it.
const (
	envInstallTarget  = "FRAMEFAIRY_INSTALL_TARGET"  // the app to replace, a .app
	envInstallFrom    = "FRAMEFAIRY_INSTALL_FROM"    // the new app, unpacked by the updater
	envInstallPID     = "FRAMEFAIRY_INSTALL_PID"     // the app that quit, waited for
	envInstallVersion = "FRAMEFAIRY_INSTALL_VERSION" // the new build's version, for its trial
	envInstallOpen    = "FRAMEFAIRY_INSTALL_OPEN"    // 1 on Relaunch: open it and wait for it
)

// installGrace is how long the step waits for the app to be gone. An app
// still there after it is one that did not quit after all, and replacing
// it under itself is what must not happen.
const installGrace = 60 * time.Second

// installIfAsked does the step and ends the program, when this program was
// started to do it. Otherwise it returns at once. main calls it first.
func installIfAsked() {
	target := os.Getenv(envInstallTarget)
	if target == "" {
		return
	}
	pid, _ := strconv.Atoi(os.Getenv(envInstallPID))
	exe, _ := os.Executable()
	s := installStep{
		target:  target,
		from:    os.Getenv(envInstallFrom),
		version: os.Getenv(envInstallVersion),
		open:    os.Getenv(envInstallOpen) == "1",
		pid:     pid,
		folder:  appFolder(),
		program: filepath.Base(exe),
		gone:    gone,
		loads:   loads,
		launch:  launchApp,
		started: saysStarted,
		stop:    stop,
	}
	for _, k := range []string{envInstallTarget, envInstallFrom, envInstallPID, envInstallVersion, envInstallOpen} {
		_ = os.Unsetenv(k)
	}
	// A log only for a step that went wrong: one that went right has
	// nothing to say, and would only be left lying in the temporary folder.
	logPath := filepath.Join(os.TempDir(), fmt.Sprintf("framefairy-install-%d.log", os.Getpid()))
	if f, err := os.Create(logPath); err == nil {
		log.SetOutput(f)
	}
	if err := s.run(); err != nil {
		log.Printf("install: %v", err) //nolint:forbidigo // this step runs before the app's log is open, into a file of its own
		os.Exit(1)
	}
	_ = os.Remove(logPath)
	os.Exit(0)
}

// installStep is the step, with what it reaches beyond the files handed
// to it, so a test hands in its own.
type installStep struct {
	target, from, version string
	// open is set on Relaunch, which opens the new build and waits for it.
	open bool
	pid  int
	// folder is the app's folder, where the build before is kept.
	folder string
	// program is the name of the app's program in Contents/MacOS.
	program string
	gone    func(pid int, within time.Duration) bool
	loads   func(program string) error
	launch  func(app string) error
	started func(folder string, within time.Duration) bool
	stop    func(pid int)
}

// run waits for the app to be gone and then puts the new one in its
// place. Only an app bundle is replaced, only with one the updater
// unpacked, so the environment cannot point it anywhere else.
func (s installStep) run() error {
	if !strings.HasSuffix(s.target, ".app") || !filepath.IsAbs(s.target) {
		return fmt.Errorf("%q is not an app to replace", s.target)
	}
	if filepath.Ext(s.from) != ".app" || !strings.HasPrefix(filepath.Base(filepath.Dir(s.from)), "wails-update-") {
		return fmt.Errorf("%q is not a build the updater unpacked", s.from)
	}
	if !filepath.IsAbs(s.folder) || s.version == "" {
		return errors.New("the step was not told where the app's files are or which build it puts in")
	}
	if s.pid <= 0 || !s.gone(s.pid, installGrace) {
		return errors.New("the app did not quit, so it was left as it is")
	}
	// The build before is opened again whenever the new one does not go
	// in, so Relaunch never ends in no app at all.
	reopen := func() {
		if s.open {
			_ = s.launch(s.target)
		}
	}
	if _, err := os.Stat(s.from); err != nil {
		reopen()
		return fmt.Errorf("the new build is not there: %w", err)
	}
	if err := s.loads(filepath.Join(s.from, "Contents", "MacOS", s.program)); err != nil {
		_ = writeFailed(s.folder, s.version)
		_ = os.RemoveAll(filepath.Dir(s.from))
		reopen()
		return fmt.Errorf("the new build does not start, so the app was left as it is: %w", err)
	}
	before := filepath.Join(s.folder, beforeDir)
	if err := os.RemoveAll(before); err != nil {
		reopen()
		return fmt.Errorf("an earlier build before could not be removed, so the app was left as it is: %w", err)
	}
	if err := os.MkdirAll(before, 0o700); err != nil {
		reopen()
		return fmt.Errorf("there is no room to keep the build before, so the app was left as it is: %w", err)
	}
	if err := replaceBundle(s.target, s.from, filepath.Join(before, filepath.Base(s.target))); err != nil {
		_ = os.RemoveAll(before)
		reopen()
		return err
	}
	// The folder the updater unpacked into is empty now.
	_ = os.RemoveAll(filepath.Dir(s.from))
	if err := writeTrial(s.folder, trial{Version: s.version, App: s.target}); err != nil {
		// Without a trial nothing could put the build before back, so it
		// is not kept either.
		_ = os.RemoveAll(before)
		reopen()
		return fmt.Errorf("the new build went in, but nothing could put the build before back: %w", err)
	}
	if !s.open {
		return nil
	}
	if err := s.launch(s.target); err == nil && s.started(s.folder, startedWithin) {
		return nil
	}
	if err := undo(s.folder, s.stop, s.launch); err != nil {
		return err
	}
	// It said it started as it was ended, so it stayed and was opened
	// again.
	if failedBuild(s.folder) != s.version {
		return nil
	}
	return fmt.Errorf("%s did not say it started within %v, so the build before was put back", s.version, startedWithin)
}

// replaceBundle puts the app at from where target is, and the app that
// was there at aside. The app is put back when the new one does not go
// in whole.
func replaceBundle(target, from, aside string) error {
	if err := move(target, aside); err != nil {
		return fmt.Errorf("the app could not be moved aside: %w", err)
	}
	if err := move(from, target); err != nil {
		if back := move(aside, target); back != nil {
			return fmt.Errorf("the new build did not go in (%w), and the app is %s: %w", err, aside, back)
		}
		return fmt.Errorf("the new build did not go in, the app is as it was: %w", err)
	}
	return nil
}

// move renames a folder, or copies it and removes it where it cannot be
// renamed, across volumes.
func move(src, dst string) error {
	if err := os.Rename(src, dst); err == nil {
		return nil
	}
	if err := copyTree(src, dst); err != nil {
		_ = os.RemoveAll(dst)
		return err
	}
	return os.RemoveAll(src)
}

// copyTree copies a folder with its links and modes, which an app bundle
// needs: its frameworks are links, and its programs must stay programs.
func copyTree(src, dst string) error {
	return filepath.Walk(src, func(path string, info os.FileInfo, err error) error {
		if err != nil {
			return err
		}
		rel, err := filepath.Rel(src, path)
		if err != nil {
			return err
		}
		to := filepath.Join(dst, rel)
		switch {
		case info.Mode()&os.ModeSymlink != 0:
			link, err := os.Readlink(path)
			if err != nil {
				return err
			}
			return os.Symlink(link, to)
		case info.IsDir():
			return os.MkdirAll(to, info.Mode().Perm())
		default:
			return copyFile(path, to, info.Mode().Perm())
		}
	})
}

func copyFile(src, dst string, mode os.FileMode) error {
	in, err := os.Open(src)
	if err != nil {
		return err
	}
	defer in.Close()
	out, err := os.OpenFile(dst, os.O_WRONLY|os.O_CREATE|os.O_TRUNC, mode)
	if err != nil {
		return err
	}
	if _, err := io.Copy(out, in); err != nil {
		_ = out.Close()
		return err
	}
	return out.Close()
}

// startInstall starts the step, on its own, so it outlives the app. open
// is set on Relaunch.
func startInstall(target, from, version string, open bool) error {
	exe, err := os.Executable()
	if err != nil {
		return err
	}
	opens := "0"
	if open {
		opens = "1"
	}
	cmd := exec.Command(exe)
	cmd.Env = append(os.Environ(),
		envInstallTarget+"="+target,
		envInstallFrom+"="+from,
		envInstallPID+"="+strconv.Itoa(os.Getpid()),
		envInstallVersion+"="+version,
		envInstallOpen+"="+opens,
	)
	detach(cmd)
	if err := cmd.Start(); err != nil {
		return err
	}
	return cmd.Process.Release()
}

// runningApp is the .app this program runs from, or empty.
func runningApp() string {
	exe, err := os.Executable()
	if err != nil {
		return ""
	}
	return bundleOf(exe)
}

// bundleOf is the .app a program runs from, or empty when it runs from none.
func bundleOf(exe string) string {
	parts := strings.Split(filepath.ToSlash(filepath.Clean(exe)), "/")
	for i, p := range parts {
		if strings.HasSuffix(p, ".app") {
			return filepath.FromSlash(strings.Join(parts[:i+1], "/"))
		}
	}
	return ""
}

// stagingFolder is the folder Wails' updater unpacks a build in: the one
// the app lies in, so that putting the build in place is a rename on one
// disk. Wails finds it the same way, from the .app the program runs from,
// or from the program when it runs from none.
func stagingFolder(exe string) string {
	target := bundleOf(exe)
	if target == "" {
		target = exe
	}
	return filepath.Dir(filepath.Clean(target))
}

// What updating leaves behind, and how long it may stay. The app clears
// its own when it starts, in the temporary folder, which macOS clears by
// itself only of what has not been touched for days, and in the folder
// beside the app, which nobody else clears: the log of a step on quit
// that went wrong, kept a week so it can still be sent, Wails' log of a
// Relaunch, and a build Wails unpacked and never used, because a download
// was cut off or the app ended before it went in. Wails names those the
// same for every app made with it, so only the ones about Frame Fairy are
// ours to remove.
func tidyUpdates(dir string, now time.Time) {
	old := func(name string, after time.Duration) bool {
		info, err := os.Lstat(name)
		return err == nil && now.Sub(info.ModTime()) >= after
	}
	logs, _ := filepath.Glob(filepath.Join(dir, "framefairy-install-*.log"))
	for _, name := range logs {
		if old(name, 7*24*time.Hour) {
			_ = os.Remove(name)
		}
	}
	wails, _ := filepath.Glob(filepath.Join(dir, "wails-update-*"))
	for _, name := range wails {
		if !old(name, 24*time.Hour) || !aboutUs(name) {
			continue
		}
		_ = os.RemoveAll(name)
	}
}

// aboutUs says whether something Wails' updater left is Frame Fairy's: a
// folder with the app unpacked in it, or a log of putting it in place.
func aboutUs(name string) bool {
	if strings.HasSuffix(name, ".log") {
		f, err := os.Open(name)
		if err != nil {
			return false
		}
		defer f.Close()
		head := make([]byte, 1024)
		n, _ := io.ReadFull(f, head)
		return strings.Contains(string(head[:n]), "/Frame Fairy.app")
	}
	_, err := os.Lstat(filepath.Join(name, "Frame Fairy.app"))
	return err == nil
}
