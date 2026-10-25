//go:build unix

package engine

import (
	"context"
	"path/filepath"
	"sync"
	"testing"
	"time"

	"framefairy/internal/standin"
)

// stuckFFmpeg is an ffmpeg that never finishes when it decodes through the
// system's decoder, and, if always is set, never finishes at all. When it
// does finish it reports one camera switch, at 5 s into what it read.
//
// The time limit of whatever it runs for ends the moment it says it
// hangs, see timeLimit, so a test waits on no limit of the clock. Not
// parallel: timeLimit is a package variable.
//
// On the real disk: the stand-in ffmpeg is a program that is run.
func stuckFFmpeg(t *testing.T, always bool) string {
	t.Helper()
	pipe, hangs := standin.Hangs(t)
	var mu sync.Mutex
	var last context.CancelFunc
	was := timeLimit
	timeLimit = func(ctx context.Context, _ time.Duration) (context.Context, context.CancelFunc) {
		ctx, cancel := context.WithCancel(ctx)
		mu.Lock()
		last = cancel
		mu.Unlock()
		return ctx, cancel
	}
	t.Cleanup(func() { timeLimit = was })
	go func() {
		for range hangs {
			mu.Lock()
			end := last
			mu.Unlock()
			if end != nil {
				end()
			}
		}
	}()
	hang := `case " $* " in *" -hwaccel "*) echo hangs > '` + pipe + `'; exec sleep 600 ;; esac`
	if always {
		hang = `echo hangs > '` + pipe + `'; exec sleep 600`
	}
	path := filepath.Join(t.TempDir(), "ffmpeg")
	script := "#!/bin/sh\n" + hang + "\n" +
		"echo '[Parsed_showinfo_2 @ 0x0] n:   0 pts:  125 pts_time:5      duration:1' >&2\n"
	if err := standin.Write(path, []byte(script), 0o755); err != nil {
		t.Fatal(err)
	}
	return path
}
