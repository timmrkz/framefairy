package engine

import (
	"context"
	"errors"
	"io"
	"sync"
	"sync/atomic"
	"testing"
	"time"
)

// standIn replaces llama-server for a test: it counts the servers started
// and stopped and the ones running, and loads in the given time.
type standIn struct {
	started, stopped atomic.Int32
	running          atomic.Int32
	// peak is the most servers ever running at once, loading included.
	peak    atomic.Int32
	up      atomic.Int32
	loading time.Duration
	fail    atomic.Bool
}

// rise counts a server that starts loading, and keeps the peak.
func (s *standIn) rise() {
	n := s.up.Add(1)
	for {
		p := s.peak.Load()
		if n <= p || s.peak.CompareAndSwap(p, n) {
			return
		}
	}
}

func useStandIn(t *testing.T, loading time.Duration) *standIn {
	t.Helper()
	s := &standIn{loading: loading}
	was := launch
	launch = func(_ *Engine, ctx context.Context, m LocalModel, size int, _ string) (string, func(), error) {
		// A llama-server is in memory from the moment it starts loading.
		s.rise()
		select {
		case <-time.After(s.loading):
		case <-ctx.Done():
			s.up.Add(-1)
			return "", nil, ctx.Err()
		}
		if s.fail.Load() {
			s.up.Add(-1)
			return "", nil, errors.New("the model would not load")
		}
		s.started.Add(1)
		s.running.Add(1)
		var once sync.Once
		return "http://" + m.Model + "/" + itoa(size), func() {
			once.Do(func() {
				s.stopped.Add(1)
				s.running.Add(-1)
				s.up.Add(-1)
			})
		}, nil
	}
	t.Cleanup(func() {
		StopModels()
		launch = was
	})
	StopModels()
	return s
}

func quietEngine() *Engine { return NewEngine(NewLog(io.Discard, false, false)) }

var gemma = LocalModel{Model: "gemma"}

// A model loaded ahead of a search is the one the search uses, and the
// search lets go of it when it is done.
func TestTheSearchFindsTheModelLoaded(t *testing.T) {
	s := useStandIn(t, 10*time.Millisecond)
	e := quietEngine()
	ctx := context.Background()

	_, release, err := e.holdModel(ctx, gemma, 65536, "")
	if err != nil {
		t.Fatal(err)
	}
	release(warmKeep)
	if !modelReady("gemma", 32768) {
		t.Fatal("the model loaded ahead is not ready for a search that fits it")
	}
	if modelReady("gemma", 131072) || modelReady("other", 32768) {
		t.Error("a model too small, or another one, reads as ready")
	}
	url, release, err := e.holdModel(ctx, gemma, 32768, "")
	if err != nil {
		t.Fatal(err)
	}
	if url != "http://gemma/65536" || s.started.Load() != 1 {
		t.Errorf("the search got %s after %d starts", url, s.started.Load())
	}
	release(0)
	if s.running.Load() != 0 {
		t.Error("the model stayed loaded after the search")
	}
}

// A search that needs more room than the model was loaded with starts it
// again, and one model runs at a time.
func TestABiggerSearchStartsTheModelAgain(t *testing.T) {
	s := useStandIn(t, time.Millisecond)
	e := quietEngine()
	ctx := context.Background()
	_, release, _ := e.holdModel(ctx, gemma, 32768, "")
	release(warmKeep)
	url, release, err := e.holdModel(ctx, gemma, 131072, "")
	if err != nil {
		t.Fatal(err)
	}
	if url != "http://gemma/131072" || s.started.Load() != 2 || s.running.Load() != 1 {
		t.Errorf("%s, %d started, %d running", url, s.started.Load(), s.running.Load())
	}
	release(0)
}

// A model loaded ahead and never asked for goes by itself.
func TestAWarmModelNobodyUsesGoes(t *testing.T) {
	s := useStandIn(t, time.Millisecond)
	_, release, err := quietEngine().holdModel(context.Background(), gemma, 32768, "")
	if err != nil {
		t.Fatal(err)
	}
	release(20 * time.Millisecond)
	deadline := time.Now().Add(2 * time.Second)
	for s.running.Load() != 0 && time.Now().Before(deadline) {
		time.Sleep(5 * time.Millisecond)
	}
	if s.running.Load() != 0 {
		t.Error("the model stayed loaded with nobody to use it")
	}
}

// A search that arrives while the model is still loading waits for it
// rather than loading a second one. If the load it waited on fails, it
// loads the model itself.
func TestASearchWaitsForTheModelBeingLoaded(t *testing.T) {
	s := useStandIn(t, 100*time.Millisecond)
	e := quietEngine()
	warm, cancel := context.WithCancel(context.Background())
	done := make(chan struct{})
	go func() {
		defer close(done)
		if _, release, err := e.holdModel(warm, gemma, 65536, ""); err == nil {
			release(warmKeep)
		}
	}()
	time.Sleep(20 * time.Millisecond)
	cancel()
	url, release, err := e.holdModel(context.Background(), gemma, 32768, "")
	<-done
	if err != nil {
		t.Fatalf("the search failed with the warm-up it waited on: %v", err)
	}
	if url == "" || s.running.Load() != 1 {
		t.Errorf("%q, %d running", url, s.running.Load())
	}
	release(0)
}

// Everything the app and its jobs can do to the model at once: warm-ups,
// searches, the app closing. Whatever order they come in, no more than one
// model is ever left running, and none once everybody has let go.
func TestTheModelHeldFromEverywhereAtOnce(t *testing.T) {
	s := useStandIn(t, 2*time.Millisecond)
	e := quietEngine()
	var wg sync.WaitGroup
	for i := range 24 {
		wg.Add(1)
		go func() {
			defer wg.Done()
			size := 32768 << (i % 2)
			m := gemma
			if i%5 == 0 {
				m = LocalModel{Model: "qwen"}
			}
			hold := e.holdModel
			if i%4 == 0 {
				hold = e.warmModel
			}
			_, release, err := hold(context.Background(), m, size, "")
			if err != nil {
				return
			}
			time.Sleep(time.Millisecond)
			if i%3 == 0 {
				release(time.Millisecond)
			} else {
				release(0)
			}
			modelReady("gemma", size)
		}()
	}
	wg.Wait()
	StopModels()
	deadline := time.Now().Add(2 * time.Second)
	for s.running.Load() != 0 && time.Now().Before(deadline) {
		time.Sleep(5 * time.Millisecond)
	}
	if s.running.Load() != 0 {
		t.Errorf("%d models left running", s.running.Load())
	}
	if s.peak.Load() > 1 {
		t.Errorf("%d models were in memory at once, there is room for one", s.peak.Load())
	}
}

func TestStopModelsStopsTheModel(t *testing.T) {
	s := useStandIn(t, time.Millisecond)
	_, release, _ := quietEngine().holdModel(context.Background(), gemma, 32768, "")
	release(warmKeep)
	StopModels()
	if s.running.Load() != 0 {
		t.Error("the model outlived the app")
	}
}

// A search that needs more room while a warm-up is still loading waits for
// the warm-up and then takes its place. It used to see the model in use,
// start a second one beside it, and hold two in memory.
func TestABiggerSearchDuringAWarmUpNeverLoadsTwo(t *testing.T) {
	s := useStandIn(t, 50*time.Millisecond)
	e := quietEngine()
	warming := make(chan struct{})
	go func() {
		defer close(warming)
		if _, release, err := e.warmModel(context.Background(), gemma, 32768, ""); err == nil {
			release(warmKeep)
		}
	}()
	time.Sleep(10 * time.Millisecond)
	url, release, err := e.holdModel(context.Background(), gemma, 131072, "")
	<-warming
	if err != nil {
		t.Fatal(err)
	}
	if url != "http://gemma/131072" || s.peak.Load() != 1 {
		t.Errorf("%s, %d in memory at once", url, s.peak.Load())
	}
	release(0)
}

// A warm-up for one episode while another episode's search holds another
// model gives up rather than load a second one. The search it was for
// loads the model when its turn comes.
func TestAWarmUpGivesWayToAModelInUse(t *testing.T) {
	s := useStandIn(t, time.Millisecond)
	e := quietEngine()
	_, release, err := e.holdModel(context.Background(), gemma, 32768, "")
	if err != nil {
		t.Fatal(err)
	}
	began := time.Now()
	if _, _, err := e.warmModel(context.Background(), gemma, 131072, ""); !errors.Is(err, errModelBusy) {
		t.Errorf("a warm-up beside a model in use: %v", err)
	}
	if time.Since(began) > time.Second || s.started.Load() != 1 {
		t.Errorf("the warm-up waited %s or started a server, %d started",
			time.Since(began), s.started.Load())
	}
	release(0)
}

// Stopping the models while one is still loading stops it too, before
// StopModels returns. It was left to its loader, which never ran again
// once the app had quit.
func TestStopModelsStopsAModelStillLoading(t *testing.T) {
	s := useStandIn(t, time.Hour)
	e := quietEngine()
	failed := make(chan error, 1)
	go func() {
		_, _, err := e.warmModel(context.Background(), gemma, 32768, "")
		failed <- err
	}()
	deadline := time.Now().Add(2 * time.Second)
	for s.up.Load() == 0 && time.Now().Before(deadline) {
		time.Sleep(time.Millisecond)
	}
	StopModels()
	if s.up.Load() != 0 {
		t.Errorf("%d servers still in memory after the app stopped them", s.up.Load())
	}
	select {
	case err := <-failed:
		if err == nil {
			t.Error("a load stopped by the app said it loaded")
		}
	case <-time.After(2 * time.Second):
		t.Error("the loader never heard it was stopped")
	}
}

// A search waiting for the model in use gives up when it is stopped.
func TestASearchWaitingForAnotherModelCanBeStopped(t *testing.T) {
	useStandIn(t, time.Millisecond)
	e := quietEngine()
	_, release, err := e.holdModel(context.Background(), gemma, 32768, "")
	if err != nil {
		t.Fatal(err)
	}
	defer release(0)
	ctx, cancel := context.WithTimeout(context.Background(), 50*time.Millisecond)
	defer cancel()
	if _, _, err := e.holdModel(ctx, LocalModel{Model: "qwen"}, 32768, ""); err == nil {
		t.Error("a search got a second model while the first was in use")
	}
}
