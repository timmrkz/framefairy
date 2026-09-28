package main

import (
	"os/exec"
	"time"
)

// Only the Mac app updates itself for now, so on Windows the step never
// finds the app gone and never replaces anything.
func gone(int, time.Duration) bool { return false }

func detach(*exec.Cmd) {}
