package engine

import (
	"fmt"
	"io"
	"net/http"
	"os"
	"os/exec"
	"os/signal"
	"path/filepath"
	"testing"
)

// The tests never write into the home folder of whoever runs them, so the
// one training folder is a temporary one for the whole package, and so is
// the record of how long searches take.
func TestMain(m *testing.M) {
	if os.Getenv("FRAMEFAIRY_STUBBORN_SERVER") != "" {
		stubbornServer()
		return
	}
	if path := os.Getenv("FRAMEFAIRY_HOLD_LOCK"); path != "" {
		holdLock(path)
		return
	}
	toolsFromThePath()
	// The tests start fake servers, whatever the machine they run on has
	// free, so what is free is not asked. TestTheMemoryFreeNow asks it.
	freeMemory = func() int64 { return 0 }
	dir, err := os.MkdirTemp("", "framefairy-training")
	if err != nil {
		panic(err)
	}
	SetTrainingDir(filepath.Join(dir, "training"))
	// Nor into the timings of past searches, which would then measure
	// every search on the machine against a fake model.
	SetSpeedFile(filepath.Join(dir, "speed.json"))
	code := m.Run()
	_ = os.RemoveAll(dir)
	os.Exit(code)
}

// ownTrainingDir gives a test the training folder to itself, for a test
// that reads back the records it wrote, and puts back the one before when
// it ends. The tests that run side by side write into the package's one
// folder, and a test that points it elsewhere cannot run beside them.
func ownTrainingDir(t *testing.T) string {
	t.Helper()
	was := TrainingDir()
	dir := t.TempDir()
	SetTrainingDir(dir)
	t.Cleanup(func() { SetTrainingDir(was) })
	return dir
}

// stubbornServer is this test binary run as a llama-server that says it is
// ready and ignores being asked to stop, the way a real one can hang on the
// way out while one of its threads waits for an answer.
func stubbornServer() {
	signal.Ignore(os.Interrupt)
	port := ""
	for i, a := range os.Args {
		if a == "--port" && i+1 < len(os.Args) {
			port = os.Args[i+1]
		}
	}
	http.HandleFunc("/health", func(w http.ResponseWriter, _ *http.Request) {})
	_ = http.ListenAndServe("127.0.0.1:"+port, nil)
}

// holdLock is this test binary run as another program that edits a file:
// it takes the file's lock, says so, and holds it until its input closes.
func holdLock(path string) {
	release := lockFile(path)
	fmt.Println("held")
	_, _ = io.Copy(io.Discard, os.Stdin)
	release()
}

// toolsFromThePath names the ffmpeg, ffprobe and llama-server on the search
// path in the environment, where the programs take a tool from by choice.
// A program never falls back to the search path by itself, see
// engine.FindTool, and a test binary has nothing beside it. One already
// named is left as it is.
func toolsFromThePath() {
	for env, name := range map[string]string{
		"FRAMEFAIRY_FFMPEG":       "ffmpeg",
		"FRAMEFAIRY_FFPROBE":      "ffprobe",
		"FRAMEFAIRY_LLAMA_SERVER": "llama-server",
	} {
		if os.Getenv(env) != "" {
			continue
		}
		if found, err := exec.LookPath(name); err == nil {
			_ = os.Setenv(env, found)
		}
	}
}
