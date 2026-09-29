package engine

import (
	"fmt"
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
	dir := filepath.Join(t.TempDir(), "llama-servers")
	was := serverNoteDir
	serverNoteDir = func() string { return dir }
	t.Cleanup(func() { serverNoteDir = was })
	return dir
}

// gone is the number of a process that has ended, the owner of a note
// whose program crashed.
func gone(t *testing.T) int {
	t.Helper()
	cmd := exec.Command("true")
	if err := cmd.Run(); err != nil {
		t.Fatal(err)
	}
	return cmd.Process.Pid
}

func exited(s *standingServer, within time.Duration) bool {
	select {
	case <-s.done:
		return true
	case <-time.After(within):
		return false
	}
}

// A server a run that has gone left running is stopped when the app
// starts, and its note goes.
func TestALeftoverServerIsStopped(t *testing.T) {
	dir := noteIn(t)
	cmd := leftover(t, "-m", "/models/gemma.gguf", "--host", "127.0.0.1", "--port", "41234")
	pid := cmd.cmd.Process.Pid
	noteServer(serverNote{PID: pid, Port: 41234, Model: "/models/gemma.gguf", Owner: gone(t)})
	time.Sleep(100 * time.Millisecond)

	if !StopLeftoverServer() {
		t.Fatal("the server left behind was not stopped")
	}
	if !exited(cmd, 3*time.Second) {
		t.Error("the server left behind is still running")
	}
	if _, err := os.Stat(noteFile(dir, pid)); !os.IsNotExist(err) {
		t.Error("the note is still there")
	}
}

// A server whose program is still running is in use, by the command line
// or by another copy of the app, and is left alone with its note.
func TestAServerInUseIsLeftAlone(t *testing.T) {
	dir := noteIn(t)
	cmd := leftover(t, "-m", "/models/gemma.gguf", "--port", "41234")
	pid := cmd.cmd.Process.Pid
	// Owned by this test, which is running.
	noteServer(serverNote{PID: pid, Port: 41234, Model: "/models/gemma.gguf"})
	time.Sleep(100 * time.Millisecond)
	if StopLeftoverServer() {
		t.Error("a server in use was stopped")
	}
	if exited(cmd, 300*time.Millisecond) {
		t.Error("a server in use ended")
	}
	if _, err := os.Stat(noteFile(dir, pid)); err != nil {
		t.Error("the note of a server in use was taken away")
	}
}

// Two programs that each run a server keep a note each, so the one does
// not write over the other's and leave a leftover nobody finds.
func TestEveryServerKeepsItsOwnNote(t *testing.T) {
	noteIn(t)
	cli := leftover(t, "-m", "/models/a.gguf", "--port", "41001")
	app := leftover(t, "-m", "/models/b.gguf", "--port", "41002")
	noteServer(serverNote{PID: cli.cmd.Process.Pid, Port: 41001, Model: "/models/a.gguf", Owner: gone(t)})
	noteServer(serverNote{PID: app.cmd.Process.Pid, Port: 41002, Model: "/models/b.gguf", Owner: gone(t)})
	time.Sleep(100 * time.Millisecond)
	if !StopLeftoverServer() {
		t.Fatal("nothing was stopped")
	}
	if !exited(cli, 3*time.Second) || !exited(app, 3*time.Second) {
		t.Error("a server left behind is still running")
	}
}

// A model whose path has a space in it is still the same model.
func TestAModelPathWithASpaceIsFound(t *testing.T) {
	noteIn(t)
	model := "/Users/tim/My Models/gemma.gguf"
	cmd := leftover(t, "-m", model, "--port", "41234")
	noteServer(serverNote{PID: cmd.cmd.Process.Pid, Port: 41234, Model: model, Owner: gone(t)})
	time.Sleep(100 * time.Millisecond)
	if !StopLeftoverServer() {
		t.Fatal("the server left behind was not stopped")
	}
	if !exited(cmd, 3*time.Second) {
		t.Error("the server left behind is still running")
	}
}

// The one note of the versions before a note had an owner is still read,
// so a server one of them left behind is found after an update.
func TestTheNoteOfAnOlderVersionIsRead(t *testing.T) {
	dir := noteIn(t)
	cmd := leftover(t, "-m", "/models/gemma.gguf", "--port", "41234")
	old := filepath.Join(filepath.Dir(dir), "llama-server.json")
	body := fmt.Sprintf(`{"pid":%d,"port":41234,"model":"/models/gemma.gguf"}`, cmd.cmd.Process.Pid)
	if err := os.WriteFile(old, []byte(body), 0o644); err != nil {
		t.Fatal(err)
	}
	time.Sleep(100 * time.Millisecond)
	if !StopLeftoverServer() {
		t.Fatal("the server left behind was not stopped")
	}
	if !exited(cmd, 3*time.Second) {
		t.Error("the server left behind is still running")
	}
	if _, err := os.Stat(old); !os.IsNotExist(err) {
		t.Error("the old note is still there")
	}
}

// Whatever runs under a number the note names but is not that server is
// left alone: the system gives numbers out again.
func TestSomebodyElsesProcessIsLeftAlone(t *testing.T) {
	noteIn(t)
	cmd := leftover(t, "-m", "/models/other.gguf", "--port", "41234")
	noteServer(serverNote{PID: cmd.cmd.Process.Pid, Port: 41234, Model: "/models/gemma.gguf", Owner: gone(t)})
	time.Sleep(100 * time.Millisecond)
	if StopLeftoverServer() {
		t.Error("a process that is not the server was stopped")
	}
	if exited(cmd, 300*time.Millisecond) {
		t.Error("a process that is not the server ended")
	}
}

// A server stopped the normal way takes its note with it, so the next
// start has nothing to stop, and only its own note.
func TestAServerStoppedTheNormalWayLeavesNoNote(t *testing.T) {
	dir := noteIn(t)
	noteServer(serverNote{PID: 123456, Port: 1, Model: "m"})
	noteServer(serverNote{PID: 654321, Port: 1, Model: "m"})
	forgetServer(123456)
	if _, err := os.Stat(noteFile(dir, 123456)); !os.IsNotExist(err) {
		t.Error("the note outlived its server")
	}
	if n, ok := readServerNote(noteFile(dir, 654321)); !ok || n.PID != 654321 {
		t.Error("the note of another server was taken away")
	}
}
