package engine

import (
	"encoding/json"
	"os"
	"os/exec"
	"path/filepath"
	"runtime"
	"strconv"
	"strings"
	"sync"
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
// So a running server is written down, its process, its port and its model,
// and the note goes when the server is stopped. A note still there when the
// app starts is a server the last run left behind. It is stopped if it is
// still running and is exactly that server, the same process with the same
// port and the same model. Anything else under that number is somebody
// else's and is left alone.
// ---------------------------------------------------------------------------

type serverNote struct {
	PID   int    `json:"pid"`
	Port  int    `json:"port"`
	Model string `json:"model"`
}

var (
	noteMu sync.Mutex
	// serverNoteFile is where the note lives. Tests put it elsewhere.
	serverNoteFile = func() string {
		home, err := os.UserHomeDir()
		if err != nil {
			return ""
		}
		return filepath.Join(home, ".framefairy", "llama-server.json")
	}
)

// noteServer writes down a server that has just started.
func noteServer(n serverNote) {
	noteMu.Lock()
	defer noteMu.Unlock()
	path := serverNoteFile()
	if path == "" {
		return
	}
	body, err := json.Marshal(n)
	if err != nil {
		return
	}
	_ = os.MkdirAll(filepath.Dir(path), 0o755)
	_ = writeAtomic(path, body)
}

// forgetServer takes the note away once that server has stopped.
func forgetServer(pid int) {
	noteMu.Lock()
	defer noteMu.Unlock()
	path := serverNoteFile()
	if n, ok := readServerNote(path); ok && n.PID == pid {
		_ = os.Remove(path)
	}
}

func readServerNote(path string) (serverNote, bool) {
	var n serverNote
	if path == "" {
		return n, false
	}
	data, err := os.ReadFile(path)
	if err != nil || json.Unmarshal(data, &n) != nil || n.PID <= 0 {
		return n, false
	}
	return n, true
}

// StopLeftoverServer stops the llama-server a previous run of the app left
// running, if there is one, and says whether it stopped one. The app calls
// it when it starts, before it loads a model of its own.
func StopLeftoverServer() bool {
	noteMu.Lock()
	path := serverNoteFile()
	n, ok := readServerNote(path)
	if ok {
		_ = os.Remove(path)
	}
	noteMu.Unlock()
	if !ok || runtime.GOOS == "windows" {
		return false
	}
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

// isThatServer says whether the process under the note's number is still
// the server the note was written for: its command line holds the same
// port and the same model. A number the system has since given to another
// program does not.
func isThatServer(n serverNote) bool {
	out, err := exec.Command("ps", "-p", strconv.Itoa(n.PID), "-o", "args=").Output()
	if err != nil {
		return false
	}
	args := strings.Fields(string(out))
	port, model := false, false
	for i, a := range args {
		if a == "--port" && i+1 < len(args) && args[i+1] == strconv.Itoa(n.Port) {
			port = true
		}
		if a == "-m" && i+1 < len(args) && args[i+1] == n.Model {
			model = true
		}
	}
	return port && model
}
