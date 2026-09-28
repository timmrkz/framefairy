//go:build !windows

package main

import (
	"os/exec"
	"syscall"
	"time"
)

// gone says whether a process ends within the time given.
func gone(pid int, within time.Duration) bool {
	deadline := time.Now().Add(within)
	for time.Now().Before(deadline) {
		if syscall.Kill(pid, 0) != nil {
			return true
		}
		time.Sleep(100 * time.Millisecond)
	}
	return false
}

// detach puts the step in a session of its own, so it is not ended with
// the app it waits for.
func detach(cmd *exec.Cmd) {
	cmd.SysProcAttr = &syscall.SysProcAttr{Setsid: true}
}
