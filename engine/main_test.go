package engine

import (
	"flag"
	"fmt"
	"net"
	"net/http"
	"os"
	"os/exec"
	"os/signal"
	"testing"

	"github.com/spf13/afero"

	"framefairy/internal/hangdump"
)

// The tests never write into the home folder of whoever runs them. An
// engine on memory keeps its records there, and one on the disk keeps
// them in a home of the tests' own, see Records, which a test that reads
// them back replaces with records of its own, see ownRecords.
//
// On the real disk: the tests' home is a folder, removed when they end.
func TestMain(m *testing.M) {
	if os.Getenv("FRAMEFAIRY_STUBBORN_SERVER") != "" {
		stubbornServer()
		return
	}
	toolsFromThePath()
	home, err := os.MkdirTemp("", "framefairy-home-")
	if err != nil {
		fmt.Fprintln(os.Stderr, err)
		os.Exit(1)
	}
	os.Setenv("HOME", home)
	os.Setenv("USERPROFILE", home)
	os.Unsetenv("FRAMEFAIRY_TRAINING")
	// A test that hangs on the programs it started, the decoder, ffmpeg
	// or ffprobe, leaves what each of them was doing in the log.
	flag.Parse()
	stop := hangdump.BeforeTimeout()
	code := m.Run()
	stop()
	_ = os.RemoveAll(home)
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

// toolsFromThePath names the ffmpeg, ffprobe and llama-server on the search
// path in the environment, where the programs take a tool from by choice.
// A program never falls back to the search path by itself, see
// tool.Finder, and a test binary has nothing beside it. One already
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

// memoryUnasked is m starting its fake servers whatever the machine the
// tests run on has free, so what is free is not asked of the system.
// TestTheMemoryFreeNow asks it.
func memoryUnasked(m *Machine) *Machine {
	m.free = func() int64 { return 0 }
	return m
}
