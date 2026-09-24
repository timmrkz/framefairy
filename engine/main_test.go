package engine

import (
	"net/http"
	"os"
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
