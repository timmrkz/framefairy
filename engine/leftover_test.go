package engine

import (
	"os"
	"os/exec"
	"path/filepath"
	"runtime"
	"testing"
	"time"
)

// leftover starts a stand-in for a llama-server that outlived its app: a
// script that runs until it is interrupted, with the arguments a real one
// is started with.
type standingServer struct {
	cmd  *exec.Cmd
	done chan struct{}
}

func leftover(t *testing.T, args ...string) *standingServer {
	t.Helper()
	if runtime.GOOS == "windows" {
		t.Skip("no ps on Windows")
	}
	dir := t.TempDir()
	script := filepath.Join(dir, "llama-server")
	body := "#!/bin/sh\ntrap 'exit 0' INT TERM\nwhile true; do sleep 0.05; done\n"
	if err := os.WriteFile(script, []byte(body), 0o755); err != nil {
		t.Fatal(err)
	}
	cmd := exec.Command(script, args...)
	if err := cmd.Start(); err != nil {
		t.Fatal(err)
	}
	// One goroutine waits for it, and everything else asks that one.
	s := &standingServer{cmd: cmd, done: make(chan struct{})}
	go func() {
		_ = cmd.Wait()
		close(s.done)
	}()
	t.Cleanup(func() {
		_ = cmd.Process.Kill()
		<-s.done
	})
	return s
}

func noteIn(t *testing.T) string {
	t.Helper()
	path := filepath.Join(t.TempDir(), "llama-server.json")
	was := serverNoteFile
	serverNoteFile = func() string { return path }
	t.Cleanup(func() { serverNoteFile = was })
	return path
}

func exited(s *standingServer, within time.Duration) bool {
	select {
	case <-s.done:
		return true
	case <-time.After(within):
		return false
	}
}

// A server the last run of the app left running is stopped when the app
// starts, and its note goes.
func TestALeftoverServerIsStopped(t *testing.T) {
	path := noteIn(t)
	cmd := leftover(t, "-m", "/models/gemma.gguf", "--host", "127.0.0.1", "--port", "41234")
	noteServer(serverNote{PID: cmd.cmd.Process.Pid, Port: 41234, Model: "/models/gemma.gguf"})
	time.Sleep(100 * time.Millisecond)

	if !StopLeftoverServer() {
		t.Fatal("the server left behind was not stopped")
	}
	if !exited(cmd, 3*time.Second) {
		t.Error("the server left behind is still running")
	}
	if _, err := os.Stat(path); !os.IsNotExist(err) {
		t.Error("the note is still there")
	}
}

// Whatever runs under a number the note names but is not that server is
// left alone: the system gives numbers out again.
func TestSomebodyElsesProcessIsLeftAlone(t *testing.T) {
	noteIn(t)
	cmd := leftover(t, "-m", "/models/other.gguf", "--port", "41234")
	noteServer(serverNote{PID: cmd.cmd.Process.Pid, Port: 41234, Model: "/models/gemma.gguf"})
	time.Sleep(100 * time.Millisecond)
	if StopLeftoverServer() {
		t.Error("a process that is not the server was stopped")
	}
	if exited(cmd, 300*time.Millisecond) {
		t.Error("a process that is not the server ended")
	}
}

// A server stopped the normal way takes its note with it, so the next
// start has nothing to stop.
func TestAServerStoppedTheNormalWayLeavesNoNote(t *testing.T) {
	path := noteIn(t)
	noteServer(serverNote{PID: 123456, Port: 1, Model: "m"})
	forgetServer(123456)
	if _, err := os.Stat(path); !os.IsNotExist(err) {
		t.Error("the note outlived its server")
	}
	// A note for another server is not taken by mistake.
	noteServer(serverNote{PID: 654321, Port: 1, Model: "m"})
	forgetServer(123456)
	if n, ok := readServerNote(path); !ok || n.PID != 654321 {
		t.Error("the note of another server was taken away")
	}
	if StopLeftoverServer() {
		t.Error("a server that is not running was stopped")
	}
}
