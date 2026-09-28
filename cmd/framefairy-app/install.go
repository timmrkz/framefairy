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

// A build that is ready when the app quits is put in place on the way out,
// the way Chrome does it: the next time the app is opened it is the new
// build, and nothing opens by itself in between. Relaunch does the same at
// once and opens the app again, through Wails' updater, whose own step
// always relaunches. So quitting starts a step of our own instead: the
// same program, started again with the environment below, which waits for
// the app to be gone, moves the new app into its place and ends.
//
// All it is told is in its environment, never in its arguments, so a flag
// given to the app is never mistaken for it.
const (
	envInstallTarget = "FRAMEFAIRY_INSTALL_TARGET" // the app to replace, a .app
	envInstallFrom   = "FRAMEFAIRY_INSTALL_FROM"   // the new app, unpacked by the updater
	envInstallPID    = "FRAMEFAIRY_INSTALL_PID"    // the app that quit, waited for
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
	from := os.Getenv(envInstallFrom)
	pid, _ := strconv.Atoi(os.Getenv(envInstallPID))
	for _, k := range []string{envInstallTarget, envInstallFrom, envInstallPID} {
		_ = os.Unsetenv(k)
	}
	// A log only for a step that went wrong: one that went right has
	// nothing to say, and would only be left lying in the temporary folder.
	logPath := filepath.Join(os.TempDir(), fmt.Sprintf("framefairy-install-%d.log", os.Getpid()))
	if f, err := os.Create(logPath); err == nil {
		log.SetOutput(f)
	}
	if err := installWhenGone(target, from, pid, gone); err != nil {
		log.Printf("install on quit: %v", err)
		os.Exit(1)
	}
	_ = os.Remove(logPath)
	os.Exit(0)
}

// installWhenGone waits for the app to be gone and then puts the new one
// in its place. Only an app bundle is replaced, only with one the updater
// unpacked, so the environment cannot point it anywhere else.
func installWhenGone(target, from string, pid int, wait func(pid int, within time.Duration) bool) error {
	if !strings.HasSuffix(target, ".app") || !filepath.IsAbs(target) {
		return fmt.Errorf("%q is not an app to replace", target)
	}
	if filepath.Ext(from) != ".app" || !strings.HasPrefix(filepath.Base(filepath.Dir(from)), "wails-update-") {
		return fmt.Errorf("%q is not a build the updater unpacked", from)
	}
	if pid <= 0 || !wait(pid, installGrace) {
		return errors.New("the app did not quit, so it was left as it is")
	}
	if err := replaceBundle(target, from); err != nil {
		return err
	}
	// The folder the updater unpacked into is empty now.
	_ = os.RemoveAll(filepath.Dir(from))
	return nil
}

// replaceBundle puts the app at from where target is. The old app is kept
// beside it until the new one is whole, and put back when it is not.
func replaceBundle(target, from string) error {
	if _, err := os.Stat(from); err != nil {
		return fmt.Errorf("the new build is not there: %w", err)
	}
	old := target + ".old"
	_ = os.RemoveAll(old)
	if err := os.Rename(target, old); err != nil {
		return fmt.Errorf("the app could not be moved aside: %w", err)
	}
	err := os.Rename(from, target)
	if err != nil {
		// Another volume, where a folder cannot be renamed across.
		err = copyTree(from, target)
	}
	if err != nil {
		_ = os.RemoveAll(target)
		if back := os.Rename(old, target); back != nil {
			return fmt.Errorf("the new build did not go in (%v), and the app could not be put back: %w", err, back)
		}
		return fmt.Errorf("the new build did not go in, the app is as it was: %w", err)
	}
	return os.RemoveAll(old)
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

// startInstall starts the step, on its own, so it outlives the app.
func startInstall(target, from string) error {
	exe, err := os.Executable()
	if err != nil {
		return err
	}
	cmd := exec.Command(exe)
	cmd.Env = append(os.Environ(),
		envInstallTarget+"="+target,
		envInstallFrom+"="+from,
		envInstallPID+"="+strconv.Itoa(os.Getpid()),
	)
	detach(cmd)
	if err := cmd.Start(); err != nil {
		return err
	}
	return cmd.Process.Release()
}

// runningApp is the .app this program runs from, or empty.
var runningApp = func() string {
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

// What updating leaves in the temporary folder, and how long it may stay
// there. macOS clears the folder by itself only of what has not been
// touched for days, so the app clears its own when it starts: the log of
// a step on quit that went wrong, kept a week so it can still be sent,
// Wails' log of a Relaunch, and a build Wails unpacked and never used,
// because a download was cut off or the app ended before it went in.
// Wails names those the same for every app made with it, so only the ones
// about Frame Fairy are ours to remove.
func tidyTemp(dir string, now time.Time) {
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
