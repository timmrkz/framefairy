package engine

import (
	"bytes"
	"context"
	"os"
	"sync"
	"testing"
)

// The app asks for a frame every time the playhead moves, and walking
// the clip list with the arrow keys moves it as fast as a key repeats. So
// several asks for the same frame, and for frames either side of it, are
// in the air at once. Each one wrote to a temporary file named after the
// frame alone, so two asks for the same frame wrote over each other and
// what was left behind was half of one and half of the other. The app
// then showed that, and went on showing it, because a frame once written
// is kept.
func TestTheSameFrameAskedForFromEveryDirectionAtOnce(t *testing.T) {
	source := testEpisode(t, "12")
	e := NewEngine(NewLog(&bytes.Buffer{}, false, false))

	// Every one of these lands on the same few frames, which is what
	// walking a list of clips up and down does.
	seconds := []float64{1, 2, 1, 3, 2, 1, 3, 2, 1, 2, 3, 1}
	got := make([]string, len(seconds))
	errs := make([]error, len(seconds))
	var wg sync.WaitGroup
	for i, at := range seconds {
		wg.Add(1)
		go func() {
			defer wg.Done()
			got[i], errs[i] = e.Still(context.Background(), source, at, 320)
		}()
	}
	wg.Wait()

	for i, err := range errs {
		if err != nil {
			t.Fatalf("asking for the frame at %vs: %v", seconds[i], err)
		}
	}
	// Every frame that came back has to be a whole picture. A file half
	// written by one ask and half by another still has a name and a size.
	for i, path := range got {
		data, err := os.ReadFile(path)
		if err != nil {
			t.Fatalf("reading the frame at %vs: %v", seconds[i], err)
		}
		if len(data) < 128 {
			t.Errorf("the frame at %vs is %d bytes", seconds[i], len(data))
			continue
		}
		if data[0] != 0xFF || data[1] != 0xD8 {
			t.Errorf("the frame at %vs does not start like a jpeg", seconds[i])
		}
		// A jpeg ends with its own mark. Half a file does not.
		if end := data[len(data)-2:]; end[0] != 0xFF || end[1] != 0xD9 {
			t.Errorf("the frame at %vs is not a whole jpeg: it ends %x", seconds[i], end)
		}
	}

	// And nothing half written is left lying about in the folder.
	dir := got[0][:len(got[0])-len("320-000001.jpg")]
	entries, err := os.ReadDir(dir)
	if err != nil {
		t.Fatal(err)
	}
	for _, entry := range entries {
		if len(entry.Name()) > 4 && entry.Name()[len(entry.Name())-4:] != ".jpg" {
			t.Errorf("left behind: %s", entry.Name())
		}
		if bytes.Contains([]byte(entry.Name()), []byte(".part")) {
			t.Errorf("left behind: %s", entry.Name())
		}
	}
}
