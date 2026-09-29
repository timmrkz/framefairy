package engine

import (
	"encoding/json"
	"errors"
	"os"
	"os/exec"
	"path/filepath"
	"runtime"
	"strconv"
	"strings"
	"sync"
	"syscall"
	"time"
)

// ---------------------------------------------------------------------------
// A llama-server left behind
//
// The app stops its model when it quits. An app that crashes, or is killed
// from the Activity Monitor, stops nothing, and llama-server goes on holding
// the model in memory, 16 GB of a 32 GB machine, until the machine is
// restarted. Nothing on screen says so, and the next search loads a second
// copy beside it.
//
// So a running server is written down, its process, its port, its model
// and the process that started it, one note per server, and the note goes
// when the server is stopped. The command line and the app both start
// servers, so a note is a leftover only once the program that started it
// has gone: a server the command line is using right now is not the app's
// to stop, however it looks. A note whose owner has gone is a server a
// run left behind. It is stopped if it is still running and is exactly
// that server, the same process with the same port and the same model.
// Anything else under that number is somebody else's and is left alone.
// ---------------------------------------------------------------------------

type serverNote struct {
	PID   int    `json:"pid"`
	Port  int    `json:"port"`
	Model string `json:"model"`
	// Owner is the process that started the server. A note from before
	// there was one has none, and counts as a run that has gone.
	Owner int `json:"owner,omitempty"`
}

var (
	noteMu sync.Mutex
	// serverNoteDir is where the notes live, one file a server. Tests put
	// it elsewhere.
	serverNoteDir = func() string {
		home, err := os.UserHomeDir()
		if err != nil {
			return ""
		}
		return filepath.Join(home, ".framefairy", "llama-servers")
	}
)

// noteFile is the note of the server with this process number.
func noteFile(dir string, pid int) string {
	return filepath.Join(dir, strconv.Itoa(pid)+".json")
}

// noteServer writes down a server that has just started, owned by this
// process unless the note says otherwise.
func noteServer(n serverNote) {
	noteMu.Lock()
	defer noteMu.Unlock()
	dir := serverNoteDir()
	if dir == "" {
		return
	}
	if n.Owner == 0 {
		n.Owner = os.Getpid()
	}
	body, err := json.Marshal(n)
	if err != nil {
		return
	}
	_ = os.MkdirAll(dir, 0o755)
	_ = writeAtomic(noteFile(dir, n.PID), body)
}

// forgetServer takes the note away once that server has stopped.
func forgetServer(pid int) {
	noteMu.Lock()
	defer noteMu.Unlock()
	if dir := serverNoteDir(); dir != "" {
		_ = os.Remove(noteFile(dir, pid))
	}
}

func readServerNote(path string) (serverNote, bool) {
	var n serverNote
	data, err := os.ReadFile(path)
	if err != nil || json.Unmarshal(data, &n) != nil || n.PID <= 0 {
		return n, false
	}
	return n, true
}

// StopLeftoverServer stops the llama-servers that runs gone before left
// running, if there are any, and says whether it stopped one. The app
// calls it when it starts, before it loads a model of its own.
func StopLeftoverServer() bool {
	if runtime.GOOS == "windows" {
		return false
	}
	noteMu.Lock()
	var left []serverNote
	if dir := serverNoteDir(); dir != "" {
		paths, _ := filepath.Glob(filepath.Join(dir, "*.json"))
		// The one note of the versions before a note had an owner.
		paths = append(paths, filepath.Join(filepath.Dir(dir), "llama-server.json"))
		for _, path := range paths {
			n, ok := readServerNote(path)
			if ok && n.Owner != 0 && alive(n.Owner) {
				continue
			}
			_ = os.Remove(path)
			if ok {
				left = append(left, n)
			}
		}
	}
	noteMu.Unlock()
	stopped := false
	for _, n := range left {
		if stopServer(n) {
			stopped = true
		}
	}
	return stopped
}

// stopServer stops the server a note names, if it is still that server.
func stopServer(n serverNote) bool {
	if !isThatServer(n) {
		return false
	}
	proc, err := os.FindProcess(n.PID)
	if err != nil {
		return false
	}
	_ = proc.Signal(os.Interrupt)
	deadline := time.Now().Add(5 * time.Second)
	for time.Now().Before(deadline) {
		if !isThatServer(n) {
			return true
		}
		time.Sleep(50 * time.Millisecond)
	}
	_ = proc.Kill()
	return true
}

// alive says whether a process with this number is running. A number the
// system has given to another program since reads as alive too, which
// leaves a leftover running rather than stopping a server in use.
func alive(pid int) bool {
	proc, err := os.FindProcess(pid)
	if err != nil {
		return false
	}
	err = proc.Signal(syscall.Signal(0))
	return err == nil || errors.Is(err, syscall.EPERM)
}

// isThatServer says whether the process under the note's number is still
// the server the note was written for: its command line holds the same
// port and the same model. A number the system has since given to another
// program does not. The command line is compared as a whole, so a model
// path with a space in it still matches.
func isThatServer(n serverNote) bool {
	out, err := exec.Command("ps", "-p", strconv.Itoa(n.PID), "-o", "args=").Output()
	if err != nil {
		return false
	}
	line := " " + strings.TrimSpace(string(out)) + " "
	return strings.Contains(line, " --port "+strconv.Itoa(n.Port)+" ") &&
		strings.Contains(line, " -m "+n.Model+" ")
}
