package engine

import (
	"context"
	"fmt"
	"io"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"sync/atomic"
	"testing"

	"github.com/spf13/afero"

	"framefairy/engine/runlog"
	"framefairy/engine/timed"
)

// warmProject is a project of a two minute video in memory that finds
// its clips with the local model.
func warmProject(t *testing.T) *Project {
	t.Helper()
	files := afero.NewMemMapFs()
	source := "/videos/warm.mp4"
	if err := afero.WriteFile(files, source, []byte("the video"), 0o644); err != nil {
		t.Fatal(err)
	}
	base := DefaultOptions()
	base.Planner, base.LLMModel, base.ASRModel = "local", "/models/gemma.gguf", "/models/parakeet"
	return NewProject(NewEngine(runlog.New(io.Discard, false, false), NewMachine(files)), source, base)
}

// hearPart writes the part from from to to into p's transcript, the way
// the speech model writes each part it has heard: a word every half
// second, loud where a word is and quiet between.
func hearPart(t *testing.T, p *Project, from, to float64) {
	t.Helper()
	var words []timed.Cue
	for n, at := 1, from+0.1; at+0.35 <= to; n, at = n+1, at+0.5 {
		words = append(words, timed.Cue{Start: at, End: at + 0.35, Text: fmt.Sprintf("wort%d", n)})
	}
	frames := make([]float32, int((to-from)/timed.FrameSeconds+0.5))
	for i := range frames {
		frames[i] = -80
		at := from + (float64(i)+0.5)*timed.FrameSeconds
		for _, w := range words {
			if at >= w.Start && at < w.End {
				frames[i] = -20
			}
		}
	}
	stamp, err := stampOf(p.Files, p.Source)
	if err != nil {
		t.Fatal(err)
	}
	path := filepath.Join(p.LogsDir(), TranscriptName(nil))
	if err := p.engine.addHeard(path, stamp, "parakeet", 120, timed.Window{Start: from, End: to}, words, frames,
		from); err != nil {
		t.Fatal(err)
	}
}

// A search loads the model while it hears only when some of its window is
// still to be heard. A window heard whole goes straight to finding, which
// loads the model itself.
func TestASearchLoadsTheModelAheadOnlyOfAWindowStillToBeHeard(t *testing.T) {
	t.Parallel()
	p := warmProject(t)
	window := timed.Window{Start: 0, End: 60}
	if !p.loadsAhead(window) {
		t.Error("a window nothing of which is heard does not load the model ahead")
	}
	hearPart(t, p, 0, 40)
	if !p.loadsAhead(window) {
		t.Error("a window heard to 40 s of 60 does not load the model ahead")
	}
	hearPart(t, p, 40, 60)
	if p.loadsAhead(window) {
		t.Error("a window heard whole loads the model ahead")
	}
	if !p.loadsAhead(timed.Window{Start: 30, End: 90}) {
		t.Error("a window reaching past what is heard does not load the model ahead")
	}
}

// A server that is already running, given with --llm-url, is left alone:
// there is nothing to load and nothing is asked of it before the search.
func TestNothingIsLoadedAheadForARunningServer(t *testing.T) {
	t.Parallel()
	var asked atomic.Int32
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		asked.Add(1)
		http.NotFound(w, nil)
	}))
	defer server.Close()
	p := warmProject(t)
	p.Base.LLMURL = server.URL
	var launched atomic.Int32
	p.engine.host.launch = func(_ *Engine, _ context.Context, _ LocalModel, _ int) (string, func(), error) {
		launched.Add(1)
		return server.URL, func() {}, nil
	}
	if err := p.warmModel(context.Background(), PlanRequest{From: 0, To: 60, Count: 1, Pass: 1}); err != nil {
		t.Errorf("loading ahead of a running server said %v", err)
	}
	if n := launched.Load(); n != 0 {
		t.Errorf("a model was loaded %d times beside a running server", n)
	}
	if n := asked.Load(); n != 0 {
		t.Errorf("the running server was asked %d times", n)
	}
}

// heldRecognizer hears nothing until it is let go, so a test sees what a
// search does while its window is still being heard.
type heldRecognizer struct {
	fakeRecognizer
	letGo <-chan struct{}
}

func (r heldRecognizer) Recognize(samples []float32, rate int) []Token {
	<-r.letGo
	return r.fakeRecognizer.Recognize(samples, rate)
}

// A search of a window still to be heard loads the model while it hears,
// with no front end asking for it, so the app and the command line both
// find the model loaded once the window is heard. The model loads here
// before anything is heard. A search that did not load it ahead would
// load it only after hearing, and the test would wait until go test ends
// it.
//
// On the real disk: ffmpeg reads the episode, and the model has to be a
// file that can be read for the search to load it.
func TestASearchOfAWindowStillToBeHeardLoadsTheModelAsItHears(t *testing.T) {
	t.Parallel()
	server := httptest.NewServer(http.NotFoundHandler())
	p := passProject(t, "")
	memoryUnasked(p.engine.Machine)
	loaded := make(chan struct{}, 4)
	p.engine.host.launch = func(_ *Engine, _ context.Context, _ LocalModel, _ int) (string, func(), error) {
		loaded <- struct{}{}
		return server.URL, func() {}, nil
	}
	t.Cleanup(func() {
		p.engine.StopModels()
		server.Close()
	})
	letGo := make(chan struct{})
	var heard int32
	p.engine.OpenRecognizer = func(string) (Recognizer, error) {
		return heldRecognizer{fakeRecognizer{&heard}, letGo}, nil
	}
	model := filepath.Join(t.TempDir(), "model.gguf")
	if err := os.WriteFile(model, nil, 0o644); err != nil {
		t.Fatal(err)
	}
	p.Base.Planner, p.Base.LLMModel, p.Base.LLMServer = "local", model, os.Args[0]

	searched := make(chan error, 1)
	go func() {
		_, err := p.Search(context.Background(), PlanRequest{From: 0, To: 40, Count: 1, Min: 1, Max: 30}, Host{})
		searched <- err
	}()
	<-loaded
	if n := atomic.LoadInt32(&heard); n != 0 {
		t.Errorf("the model loaded after %d parts were heard, not while the first was", n)
	}
	close(letGo)
	<-searched
}
