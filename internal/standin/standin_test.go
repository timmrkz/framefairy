package standin

import (
	"fmt"
	"os/exec"
	"path/filepath"
	"runtime"
	"sync"
	"testing"
)

// A program written while other programs start can be run the moment it
// is written. Written with os.WriteFile, 130 to 146 of these 800 were
// refused as busy in each of three runs under the race detector in a
// cloud session.
//
// On the real disk: a program that is run is a file on the disk.
func TestAProgramWrittenWhileOthersStartRunsAtOnce(t *testing.T) {
	t.Parallel()
	if runtime.GOOS == "windows" {
		t.Skip("the stand-ins are shell scripts")
	}
	dir := t.TempDir()
	var wg sync.WaitGroup
	refused := make(chan error, 16*50)
	for side := range 16 {
		wg.Go(func() {
			for n := range 50 {
				path := filepath.Join(dir, fmt.Sprintf("program-%d-%d", side, n))
				if err := Write(path, []byte("#!/bin/sh\nexit 0\n"), 0o755); err != nil {
					refused <- err
					return
				}
				if err := exec.Command(path).Run(); err != nil {
					refused <- err
				}
			}
		})
	}
	wg.Wait()
	close(refused)
	for err := range refused {
		t.Error(err)
	}
}
