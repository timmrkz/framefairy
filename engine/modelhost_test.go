package engine

import (
	"context"
	"errors"
	"io"
	"runtime"
	"sync"
	"sync/atomic"
	"testing"
	"time"

	"github.com/spf13/afero"
)

// standIn replaces llama-server for a test: it counts the servers started
// and stopped and the ones running, and loads at once unless it is held.
type standIn struct {
	started, stopped atomic.Int32
	running          atomic.Int32
	// peak is the most servers ever running at once, loading included.
	peak atomic.Int32
	up   atomic.Int32
	fail atomic.Bool
	// began has a value for every server that starts loading, so a test
	// knows a load is under way rather than sleeping until it may be.
	began chan struct{}
	// holding keeps the next load loading until hold is closed or the
	// load is called off. A nil hold keeps it loading until it is called
	// off.
	holding atomic.Bool
	hold    chan struct{}
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

func useStandIn(t *testing.T) *standIn {
	t.Helper()
	s := &standIn{began: make(chan struct{}, 64)}
	was := launch
	launch = func(_ *Engine, ctx context.Context, m LocalModel, size int, _ string) (string, func(), error) {
		// A llama-server is in memory from the moment it starts loading.
		s.rise()
		select {
		case s.began <- struct{}{}:
		default:
		}
		if s.holding.CompareAndSwap(true, false) {
			select {
			case <-s.hold:
			case <-ctx.Done():
				s.up.Add(-1)
				return "", nil, ctx.Err()
			}
		} else {
			// Others get their turn while this one loads.
			runtime.Gosched()
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

func quietEngine() *Engine {
	return NewEngine(NewLog(io.Discard, false, false), afero.NewMemMapFs())
}

var gemma = LocalModel{Model: "gemma"}

// A model loaded ahead of a search is the one the search uses, and the
// search lets go of it when it is done.
func TestTheSearchFindsTheModelLoaded(t *testing.T) {
	s := useStandIn(t)
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
	s := useStandIn(t)
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

// idleWait is the wait a model nobody holds is kept for, in a test's
// hand: it hears how long the wait is, and ends it with up.
type idleWait struct {
	mu   sync.Mutex
	keep time.Duration
	gone func()
}

// idleInHand puts in an idleFor whose waits the test ends. Not parallel:
// idleFor is a package variable.
func idleInHand(t *testing.T) *idleWait {
	w := &idleWait{}
	was := idleFor
	idleFor = func(keep time.Duration, gone func()) func() bool {
		w.mu.Lock()
		defer w.mu.Unlock()
		w.keep, w.gone = keep, gone
		return func() bool { return true }
	}
	t.Cleanup(func() { idleFor = was })
	return w
}

func (w *idleWait) kept() time.Duration {
	w.mu.Lock()
	defer w.mu.Unlock()
	return w.keep
}

// up ends the last wait as its minutes would, with host.mu not held.
func (w *idleWait) up() {
	w.mu.Lock()
	gone := w.gone
	w.mu.Unlock()
	gone()
}

// A model loaded ahead and never asked for stays for warmKeep and then
// goes by itself. Not parallel: idleFor is a package variable.
func TestAWarmModelNobodyUsesGoes(t *testing.T) {
	s := useStandIn(t)
	idle := idleInHand(t)
	_, release, err := quietEngine().holdModel(context.Background(), gemma, 32768, "")
	if err != nil {
		t.Fatal(err)
	}
	release(warmKeep)
	if kept := idle.kept(); kept != warmKeep || s.running.Load() != 1 {
		t.Fatalf("a model nobody uses is kept for %s, and %d run until then, want %s and one", kept, s.running.Load(), warmKeep)
	}
	// The minutes are up now.
	idle.up()
	// Should it stay, the test hangs here until go test's -timeout.
	untilHost(func() bool { return s.running.Load() == 0 })
}

// A search that arrives while the model is still loading waits for it
// rather than loading a second one. If the load it waited on fails, it
// loads the model itself.
func TestASearchWaitsForTheModelBeingLoaded(t *testing.T) {
	s := useStandIn(t)
	// The warm-up loads until it is called off, so the search finds it
	// loading however slow the machine.
	s.holding.Store(true)
	e := quietEngine()
	warm, cancel := context.WithCancel(context.Background())
	done := make(chan struct{})
	go func() {
		defer close(done)
		if _, release, err := e.holdModel(warm, gemma, 65536, ""); err == nil {
			release(warmKeep)
		}
	}()
	<-s.began
	type answer struct {
		url     string
		release func(time.Duration)
		err     error
	}
	answered := make(chan answer, 1)
	go func() {
		url, release, err := e.holdModel(context.Background(), gemma, 32768, "")
		answered <- answer{url, release, err}
	}()
	// The search waits on the load before the load is called off.
	untilHost(func() bool { return host.model != nil && host.model.users == 2 })
	cancel()
	a := <-answered
	url, release, err := a.url, a.release, a.err
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
// searches, reads ahead, the app keeping a model and giving it up, the app
// closing. Whatever order they come in, no more than one model is ever in
// memory, none is left running once everybody has let go, and none loads
// after the app has closed them.
func TestTheModelHeldFromEverywhereAtOnce(t *testing.T) {
	s := useStandIn(t)
	t.Cleanup(func() {
		host.mu.Lock()
		host.closed = false
		host.mu.Unlock()
	})
	e := quietEngine()
	var wg sync.WaitGroup
	for i := range 48 {
		wg.Add(1)
		go func() {
			defer wg.Done()
			size := 32768 << (i % 2)
			m := gemma
			if i%5 == 0 {
				m = LocalModel{Model: "qwen"}
			}
			switch i % 8 {
			case 1:
				_, _ = e.keep(context.Background(), m, 0, "")
				return
			case 3:
				keepOnly("")
				return
			case 5:
				ctx, giveWay := context.WithCancelCause(context.Background())
				if _, release, ok := holdIdle(m.Model, size, giveWay); ok {
					<-ctx.Done()
					release(0)
				}
				return
			}
			hold := e.holdModel
			if i%4 == 0 {
				hold = e.warmModel
			}
			_, release, err := hold(context.Background(), m, size, "")
			if err != nil {
				return
			}
			runtime.Gosched()
			if i%3 == 0 {
				release(time.Millisecond)
			} else {
				release(0)
			}
			modelReady("gemma", size)
		}()
	}
	wg.Wait()
	// A read that no search called off is let go of before the app closes.
	host.mu.Lock()
	if h := host.model; h != nil && h.giveWay != nil {
		h.giveWay(errModelBusy)
		h.giveWay = nil
	}
	host.mu.Unlock()
	CloseModels()
	if loaded, _ := e.keep(context.Background(), gemma, 0, ""); loaded {
		t.Error("the model was kept after the app had closed them")
	}
	host.mu.Lock()
	kept := host.kept
	host.mu.Unlock()
	if kept != "" {
		t.Errorf("%s is still kept after the app closed the models", kept)
	}
	// Closing them stops them before it returns, whatever was still let
	// go of with a moment to stay.
	if s.running.Load() != 0 {
		t.Errorf("%d models left running", s.running.Load())
	}
	if s.peak.Load() > 1 {
		t.Errorf("%d models were in memory at once, there is room for one", s.peak.Load())
	}
}

func TestStopModelsStopsTheModel(t *testing.T) {
	s := useStandIn(t)
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
	s := useStandIn(t)
	// The warm-up loads until the search waits for it, so the search
	// comes while it loads however slow the machine.
	s.hold = make(chan struct{})
	s.holding.Store(true)
	e := quietEngine()
	warming := make(chan struct{})
	go func() {
		defer close(warming)
		if _, release, err := e.warmModel(context.Background(), gemma, 32768, ""); err == nil {
			release(warmKeep)
		}
	}()
	<-s.began
	// The load goes on once the search waits for it.
	go func() {
		defer close(s.hold)
		untilHost(func() bool { return host.waiting == 1 })
	}()
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
	s := useStandIn(t)
	e := quietEngine()
	_, release, err := e.holdModel(context.Background(), gemma, 32768, "")
	if err != nil {
		t.Fatal(err)
	}
	// A warm-up that waited would wait for the release below, and hang
	// until go test's -timeout.
	if _, _, err := e.warmModel(context.Background(), gemma, 131072, ""); !errors.Is(err, errModelBusy) {
		t.Errorf("a warm-up beside a model in use: %v", err)
	}
	if s.started.Load() != 1 {
		t.Errorf("the warm-up started a server, %d started", s.started.Load())
	}
	release(0)
}

// Stopping the models while one is still loading stops it too, before
// StopModels returns. It was left to its loader, which never ran again
// once the app had quit.
func TestStopModelsStopsAModelStillLoading(t *testing.T) {
	s := useStandIn(t)
	// The load goes on until it is called off.
	s.holding.Store(true)
	e := quietEngine()
	failed := make(chan error, 1)
	go func() {
		_, _, err := e.warmModel(context.Background(), gemma, 32768, "")
		failed <- err
	}()
	<-s.began
	StopModels()
	if s.up.Load() != 0 {
		t.Errorf("%d servers still in memory after the app stopped them", s.up.Load())
	}
	// A loader that never heard it was stopped hangs here until go
	// test's -timeout.
	if err := <-failed; err == nil {
		t.Error("a load stopped by the app said it loaded")
	}
}

// A search waiting for the model in use gives up when it is stopped.
func TestASearchWaitingForAnotherModelCanBeStopped(t *testing.T) {
	useStandIn(t)
	e := quietEngine()
	_, release, err := e.holdModel(context.Background(), gemma, 32768, "")
	if err != nil {
		t.Fatal(err)
	}
	defer release(0)
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()
	got := make(chan error, 1)
	go func() {
		_, release, err := e.holdModel(ctx, LocalModel{Model: "qwen"}, 32768, "")
		if err == nil {
			release(0)
		}
		got <- err
	}()
	// Stopped while it waits, not before it asks.
	untilHost(func() bool { return host.waiting == 1 })
	cancel()
	if err := <-got; !errors.Is(err, context.Canceled) {
		t.Errorf("a search stopped while it waited for the model in use gave %v", err)
	}
}

// Once the app has closed the models, an ask that was waiting for the model
// in use loads none of its own. With StopModels alone it woke, found no
// model and started a llama-server after the app had stopped the last one,
// and nothing was left to stop it.
func TestNoModelIsLoadedAfterTheAppClosesThem(t *testing.T) {
	s := useStandIn(t)
	t.Cleanup(func() {
		host.mu.Lock()
		host.closed = false
		host.mu.Unlock()
	})
	e := quietEngine()
	_, release, err := e.holdModel(context.Background(), gemma, 32768, "")
	if err != nil {
		t.Fatal(err)
	}
	defer release(0)
	waited := make(chan error, 1)
	go func() {
		_, release, err := e.holdModel(context.Background(), LocalModel{Model: "qwen"}, 32768, "")
		if err == nil {
			release(0)
		}
		waited <- err
	}()
	// The ask is waiting when the app closes the models, or this would
	// test an ask that came after.
	untilHost(func() bool { return host.waiting == 1 })
	CloseModels()
	// An ask that never heard the app closed the models hangs here until
	// go test's -timeout.
	if err := <-waited; err == nil {
		t.Error("an ask loaded a model after the app had closed them")
	}
	if s.started.Load() != 1 || s.running.Load() != 0 {
		t.Errorf("%d started, %d still running after the app closed them",
			s.started.Load(), s.running.Load())
	}
}

// untilHost goes on once ok says yes, and asks it again each time the
// host has news, never on the clock. ok is asked with host.mu held. What
// never comes is go test's -timeout to end, which shows where it hangs.
func untilHost(ok func() bool) {
	host.mu.Lock()
	defer host.mu.Unlock()
	for !ok() {
		news := host.news.WaitLocked()
		host.mu.Unlock()
		<-news
		host.mu.Lock()
	}
}

// The model the app keeps stays loaded after every search and warm-up
// that holds it, so the next search loads nothing. Given up, or another
// model kept in its place, it goes at once.
func TestAKeptModelStaysBetweenSearches(t *testing.T) {
	s := useStandIn(t)
	e := quietEngine()
	ctx := context.Background()
	if _, err := e.keep(ctx, gemma, 0, ""); err != nil {
		t.Fatal(err)
	}
	for i := 0; i < 3; i++ {
		_, release, err := e.holdModel(ctx, gemma, 32768, "")
		if err != nil {
			t.Fatal(err)
		}
		release(0)
		_, release, err = e.warmModel(ctx, gemma, 32768, "")
		if err != nil {
			t.Fatal(err)
		}
		release(warmKeep)
	}
	if s.started.Load() != 1 || s.running.Load() != 1 {
		t.Errorf("%d loads and %d running after three searches of a kept model, want 1 and 1",
			s.started.Load(), s.running.Load())
	}
	// Keeping it again loads nothing.
	if loaded, err := e.keep(ctx, gemma, 0, ""); err != nil || loaded || s.started.Load() != 1 {
		t.Errorf("keeping the kept model again loaded it again: %d loads, %v", s.started.Load(), err)
	}

	// Another model kept in its place loads in the place of the first,
	// never beside it.
	if _, err := e.keep(ctx, LocalModel{Model: "other"}, 0, ""); err != nil {
		t.Fatal(err)
	}
	if s.started.Load() != 2 || s.running.Load() != 1 || s.peak.Load() != 1 {
		t.Errorf("after keeping another model: %d loads, %d running, at most %d at once",
			s.started.Load(), s.running.Load(), s.peak.Load())
	}
	// None kept, the API chosen in the settings: it goes at once.
	if _, err := e.KeepModel(ctx, Options{Planner: "api"}, ""); err != nil {
		t.Fatal(err)
	}
	if s.running.Load() != 0 {
		t.Error("the model stayed loaded after it was given up")
	}
}

// A model kept is held by nobody, so the transcript can be read into it
// ahead of a search, and a search of another size replaces it with one
// that is kept in turn.
func TestAKeptModelIsFreeToReadInto(t *testing.T) {
	s := useStandIn(t)
	e := quietEngine()
	ctx := context.Background()
	if _, err := e.keep(ctx, gemma, 0, ""); err != nil {
		t.Fatal(err)
	}
	size := warmSize("gemma", 0, 0)
	_, release, ok := holdIdle("gemma", size, func(error) {})
	if !ok {
		t.Fatal("the kept model cannot be read into")
	}
	release(warmKeep)
	_, release, err := e.holdModel(ctx, gemma, size*2, "")
	if err != nil {
		t.Fatal(err)
	}
	release(0)
	if s.started.Load() != 2 || s.running.Load() != 1 || !modelReady("gemma", size*2) {
		t.Errorf("after a bigger search: %d loads, %d running", s.started.Load(), s.running.Load())
	}
}

// A search that needs more room than a read holds, or another model,
// calls the read off and has the model once the read lets go of it. It
// used to wait for the read to end by itself, which can take as long as
// reading the whole transcript of another video again.
func TestABiggerSearchCallsOffARead(t *testing.T) {
	s := useStandIn(t)
	e := quietEngine()
	ctx := context.Background()
	if _, release, err := e.warmModel(ctx, gemma, 32768, ""); err != nil {
		t.Fatal(err)
	} else {
		release(warmKeep)
	}
	reading, giveWay := context.WithCancelCause(ctx)
	_, release, ok := holdIdle("gemma", 32768, giveWay)
	if !ok {
		t.Fatal("the idle model cannot be read into")
	}
	got := make(chan error, 1)
	go func() {
		_, release, err := e.holdModel(ctx, gemma, 65536, "")
		if err == nil {
			release(0)
		}
		got <- err
	}()
	// The read ends when it is called off, as readHeard does.
	<-reading.Done()
	if !errors.Is(context.Cause(reading), errModelBusy) {
		t.Errorf("the read was called off with %v", context.Cause(reading))
	}
	release(0)
	if err := <-got; err != nil {
		t.Fatal(err)
	}
	if s.started.Load() != 2 || s.peak.Load() != 1 {
		t.Errorf("%d loads, at most %d at once, want 2 and 1", s.started.Load(), s.peak.Load())
	}
}

// Between a search's answer and its question about the clips that did not
// fit, the model is the search's still: a read ahead of another video
// would put its own prompt in the one slot, and the question would read
// the whole prompt again. Once the search is done with it, it can be read
// into, and a model the app keeps the same.
func TestAReadWaitsForTheSearchToComeBack(t *testing.T) {
	useStandIn(t)
	e := quietEngine()
	ctx := context.Background()
	if _, err := e.keep(ctx, gemma, 0, ""); err != nil {
		t.Fatal(err)
	}
	size := warmSize("gemma", 0, 0)
	_, release, err := e.holdModel(ctx, gemma, size, "")
	if err != nil {
		t.Fatal(err)
	}
	release(fitKeep)
	if _, _, ok := holdIdle("gemma", size, func(error) {}); ok {
		t.Error("a read took the model the search is coming back for")
	}
	_, release, err = e.holdModel(ctx, gemma, size, "")
	if err != nil {
		t.Fatal(err)
	}
	release(0)
	_, release, ok := holdIdle("gemma", size, func(error) {})
	if !ok {
		t.Fatal("the model is not free to read into once the search is done")
	}
	release(0)
	// A warm-up keeps the model for the search to come, and that search
	// starts with the transcript, so it may be read into.
	_, release, err = e.warmModel(ctx, gemma, size, "")
	if err != nil {
		t.Fatal(err)
	}
	release(warmKeep)
	if _, release, ok := holdIdle("gemma", size, func(error) {}); !ok {
		t.Error("a warmed model cannot be read into")
	} else {
		release(0)
	}
}

// The model kept is left as it is when the settings change, at any size,
// so a change of the caption colour does not load it again.
func TestKeepingTheModelLoadedSmallerLoadsNothing(t *testing.T) {
	s := useStandIn(t)
	e := quietEngine()
	ctx := context.Background()
	_, release, err := e.warmModel(ctx, gemma, warmSize("gemma", 0, 0)/2, "")
	if err != nil {
		t.Fatal(err)
	}
	release(warmKeep)
	if loaded, err := e.keep(ctx, gemma, 0, ""); err != nil || loaded {
		t.Errorf("keeping it loaded it again: %v, %v", loaded, err)
	}
	if s.started.Load() != 1 || s.running.Load() != 1 {
		t.Errorf("%d loads and %d running, want 1 and 1", s.started.Load(), s.running.Load())
	}
}

// A model let go of with a few minutes to stay, and then kept by the app,
// stays when those minutes are up. It used to be stopped by its timer.
func TestAModelKeptAfterItWasLetGoOfStays(t *testing.T) {
	s := useStandIn(t)
	idle := idleInHand(t)
	e := quietEngine()
	ctx := context.Background()
	_, release, err := e.warmModel(ctx, gemma, 32768, "")
	if err != nil {
		t.Fatal(err)
	}
	release(warmKeep)
	keepOnly("gemma")
	decided := make(chan struct{})
	host.mu.Lock()
	host.idled = func() { close(decided) }
	host.mu.Unlock()
	// The minutes are up now.
	idle.up()
	t.Cleanup(func() {
		host.mu.Lock()
		host.idled = nil
		host.mu.Unlock()
	})
	<-decided
	if s.running.Load() != 1 || !modelReady("gemma", 32768) {
		t.Error("the model kept was stopped when its minutes were up")
	}
}

// Stopping the models gives up the model kept as well, so the next search
// stops what it loads when it is done.
func TestStopModelsGivesUpTheModelKept(t *testing.T) {
	s := useStandIn(t)
	e := quietEngine()
	ctx := context.Background()
	if _, err := e.keep(ctx, gemma, 0, ""); err != nil {
		t.Fatal(err)
	}
	StopModels()
	_, release, err := e.holdModel(ctx, gemma, 32768, "")
	if err != nil {
		t.Fatal(err)
	}
	release(0)
	if s.running.Load() != 0 {
		t.Error("the model stayed loaded after the models were stopped and a search was done with it")
	}
}
