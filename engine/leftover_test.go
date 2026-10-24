package engine

import (
	"bufio"
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"runtime"
	"slices"
	"sync"
	"testing"

	"framefairy/internal/standin"
)

// leftover starts a stand-in for a llama-server that outlived its app: a
// script that runs until it is interrupted, with the arguments a real one
// is started with. done is closed once it has ended.
type standingServer struct {
	cmd  *exec.Cmd
	done chan struct{}
}

// On the real disk: the stand-in llama-server is a program that is run.
func leftover(t *testing.T, args ...string) *standingServer {
	t.Helper()
	if runtime.GOOS == "windows" {
		t.Skip("no ps on Windows")
	}
	dir := t.TempDir()
	script := filepath.Join(dir, "llama-server")
	// It says it is up once it runs, with its command line and the
	// interrupt it stops on. Until then ps sees the program that started
	// it, and the app would take it for somebody else's.
	body := "#!/bin/sh\ntrap 'exit 0' INT TERM\necho up\nwhile true; do sleep 0.05; done\n"
	if err := standin.Write(script, []byte(body), 0o755); err != nil {
		t.Fatal(err)
	}
	cmd := exec.Command(script, args...)
	out, err := cmd.StdoutPipe()
	if err != nil {
		t.Fatal(err)
	}
	if err := cmd.Start(); err != nil {
		t.Fatal(err)
	}
	s := &standingServer{cmd: cmd, done: make(chan struct{})}
	line, err := bufio.NewReader(out).ReadString('\n')
	if err != nil || line != "up\n" {
		_ = cmd.Process.Kill()
		_ = cmd.Wait()
		t.Fatalf("the stand-in said %q, %v", line, err)
	}
	// One goroutine waits for it, and everything else asks that one. It
	// starts once the line is read, since Wait closes the pipe.
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

// asked records the servers the app asks to stop, by process number, and
// still asks them.
func asked(t *testing.T) func() []int {
	t.Helper()
	var mu sync.Mutex
	var pids []int
	was := interrupt
	interrupt = func(proc *os.Process) error {
		mu.Lock()
		pids = append(pids, proc.Pid)
		mu.Unlock()
		return was(proc)
	}
	t.Cleanup(func() { interrupt = was })
	return func() []int {
		mu.Lock()
		defer mu.Unlock()
		return slices.Clone(pids)
	}
}

// On the real disk: the note is read by the next program to start.
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

// A server a run that has gone left running is stopped when the app
// starts, and its note goes.
//
// On the real disk: the note is read by the next program to start.
func TestALeftoverServerIsStopped(t *testing.T) {
	dir := noteIn(t)
	cmd := leftover(t, "-m", "/models/gemma.gguf", "--host", "127.0.0.1", "--port", "41234")
	pid := cmd.cmd.Process.Pid
	noteServer(serverNote{PID: pid, Port: 41234, Model: "/models/gemma.gguf", Owner: gone(t)})

	if !StopLeftoverServer() {
		t.Fatal("the server left behind was not stopped")
	}
	// A server that is still running hangs here until go test's -timeout.
	<-cmd.done
	if _, err := os.Stat(noteFile(dir, pid)); !os.IsNotExist(err) {
		t.Error("the note is still there")
	}
}

// A server whose program is still running is in use, by the command line
// or by another copy of the app, and is left alone with its note.
//
// On the real disk: the note is read by the next program to start.
func TestAServerInUseIsLeftAlone(t *testing.T) {
	dir := noteIn(t)
	cmd := leftover(t, "-m", "/models/gemma.gguf", "--port", "41234")
	pid := cmd.cmd.Process.Pid
	// Owned by this test, which is running.
	noteServer(serverNote{PID: pid, Port: 41234, Model: "/models/gemma.gguf"})
	stops := asked(t)
	if StopLeftoverServer() {
		t.Error("a server in use was stopped")
	}
	if slices.Contains(stops(), pid) {
		t.Error("a server in use was asked to stop")
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
	if !StopLeftoverServer() {
		t.Fatal("nothing was stopped")
	}
	// A server that is still running hangs here until go test's -timeout.
	<-cli.done
	<-app.done
}

// A model whose path has a space in it is still the same model.
func TestAModelPathWithASpaceIsFound(t *testing.T) {
	noteIn(t)
	model := "/Users/tim/My Models/gemma.gguf"
	cmd := leftover(t, "-m", model, "--port", "41234")
	noteServer(serverNote{PID: cmd.cmd.Process.Pid, Port: 41234, Model: model, Owner: gone(t)})
	if !StopLeftoverServer() {
		t.Fatal("the server left behind was not stopped")
	}
	// A server that is still running hangs here until go test's -timeout.
	<-cmd.done
}

// The one note of the versions before a note had an owner is still read,
// so a server one of them left behind is found after an update.
//
// On the real disk: the note is read by the next program to start.
func TestTheNoteOfAnOlderVersionIsRead(t *testing.T) {
	dir := noteIn(t)
	cmd := leftover(t, "-m", "/models/gemma.gguf", "--port", "41234")
	old := filepath.Join(filepath.Dir(dir), "llama-server.json")
	body := fmt.Sprintf(`{"pid":%d,"port":41234,"model":"/models/gemma.gguf"}`, cmd.cmd.Process.Pid)
	if err := os.WriteFile(old, []byte(body), 0o644); err != nil {
		t.Fatal(err)
	}
	if !StopLeftoverServer() {
		t.Fatal("the server left behind was not stopped")
	}
	// A server that is still running hangs here until go test's -timeout.
	<-cmd.done
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
	stops := asked(t)
	if StopLeftoverServer() {
		t.Error("a process that is not the server was stopped")
	}
	if slices.Contains(stops(), cmd.cmd.Process.Pid) {
		t.Error("a process that is not the server was asked to stop")
	}
}

// A server stopped the normal way takes its note with it, so the next
// start has nothing to stop, and only its own note.
//
// On the real disk: the note is read by the next program to start.
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
