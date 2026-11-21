package main

import (
	"os/exec"
	"testing"
	"time"
)

// On Linux, where the walks run, a program's processor time holds what
// the programs it started and waited for took: the walks summary counts
// a bridge with its ffmpeg.
func TestTheProcessorTimeOfAProgramHoldsWhatItStarted(t *testing.T) {
	t.Parallel()
	// The shell only waits, its child counts to 200000.
	cmd := exec.Command("sh", "-c", `sh -c 'i=0; while [ $i -lt 200000 ]; do i=$((i+1)); done' & wait`)
	if err := cmd.Run(); err != nil {
		t.Fatal(err)
	}
	if cpu := cpuOf(cmd); cpu < 20*time.Millisecond {
		t.Errorf("a shell whose child counted took %s, want the child's counting in it", cpu)
	}
}
