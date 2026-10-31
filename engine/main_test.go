package engine

import (
	"fmt"
	"io"
	"net"
	"net/http"
	"os"
	"os/exec"
	"os/signal"
	"testing"

	"github.com/spf13/afero"
)

// The tests never write into the home folder of whoever runs them. An
// engine on memory keeps its records there, and one on the disk keeps
// them in a home of the tests' own, see Records, which a test that reads
// them back replaces with records of its own, see ownRecords.
//
// On the real disk: the episodes the tests share are made by ffmpeg, and
// removed when they end, and so is the tests' home.
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
	// The face detector finds no face in a test episode, which is a test
	// picture and a tone, and framing falls back to what is in focus. It
	// looks for one all the same, about 50 milliseconds a frame and some
	// sixty frames a clip, and that was a third of the time these tests
	// took. TestProjectSteps turns it back on, so the whole way a clip is
	// framed is still tested once.
	os.Setenv("FRAMEFAIRY_NO_FACES", "1")
	// The tests start fake servers, whatever the machine they run on has
	// free, so what is free is not asked. TestTheMemoryFreeNow asks it.
	freeMemory = func() int64 { return 0 }
	home, err := os.MkdirTemp("", "framefairy-home-")
	if err != nil {
		fmt.Fprintln(os.Stderr, err)
		os.Exit(1)
	}
	os.Setenv("HOME", home)
	os.Setenv("USERPROFILE", home)
	os.Unsetenv("FRAMEFAIRY_TRAINING")
	code := m.Run()
	_ = os.RemoveAll(home)
	if episodes.dir != "" {
		_ = os.RemoveAll(episodes.dir)
	}
	os.Exit(code)
}

// ownRecords gives an engine records of its own, in memory, for a test
// that reads back what it recorded, and returns them. An engine on memory
// has records of its own already, one on the disk shares the tests' home.
func ownRecords(e *Engine) *Records {
	e.Records = NewRecords(afero.NewMemMapFs(), "/home/tim/.framefairy/training",
		"/home/tim/.framefairy/speed.json")
	return e.Records
}

// stubbornServer is this test binary run as a llama-server that says it is
// ready and ignores being asked to stop, the way a real one can hang on the
// way out while one of its threads waits for an answer. It says it is
// listening once it is, in llama-server's words.
func stubbornServer() {
	signal.Ignore(os.Interrupt)
	port := ""
	for i, a := range os.Args {
		if a == "--port" && i+1 < len(os.Args) {
			port = os.Args[i+1]
		}
	}
	listener, err := net.Listen("tcp", "127.0.0.1:"+port)
	if err != nil {
		fmt.Fprintln(os.Stderr, err)
		os.Exit(1)
	}
	http.HandleFunc("/health", func(w http.ResponseWriter, _ *http.Request) {})
	fmt.Printf("main: server is listening on http://127.0.0.1:%s - starting the main loop\n", port)
	_ = http.Serve(listener, nil)
}

// holdLock is this test binary run as another program that edits a file:
// it takes the file's lock, says so, and holds it until its input closes.
//
// On the real disk: it stands for another program, and the lock between
// two programs is the system's.
func holdLock(path string) {
	release := lockFile(disk, path)
	fmt.Println("held")
	_, _ = io.Copy(io.Discard, os.Stdin)
	release()
}

// toolsFromThePath names the ffmpeg, ffprobe and llama-server on the search
// path in the environment, where the programs take a tool from by choice.
// A program never falls back to the search path by itself, see
// tool.Find, and a test binary has nothing beside it. One already
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
