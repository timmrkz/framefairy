package updates

import (
	"context"
	"net/http"
	"net/http/httptest"
	"sync"
	"sync/atomic"
	"testing"
	"time"
)

// flaky serves the channel list, answering with code for the first misses
// asks and with the list after that, the way the release file is missing
// for a moment while the publish workflow replaces it.
func flaky(t *testing.T, list []byte, code int, misses int32) (*httptest.Server, *atomic.Int32) {
	t.Helper()
	var asked atomic.Int32
	srv := httptest.NewTLSServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if asked.Add(1) <= misses {
			http.Error(w, http.StatusText(code), code)
			return
		}
		_, _ = w.Write(list)
	}))
	t.Cleanup(srv.Close)
	return srv, &asked
}

var quick = []time.Duration{time.Millisecond, time.Millisecond, time.Millisecond}

func TestAListBeingReplacedIsWaitedOut(t *testing.T) {
	key := testKey(t)
	list := listJSON(t, build(t, key, "main", "0.3.0-main.1", "https://example.com/main.zip", appZip(t, "x")))
	srv, asked := flaky(t, list, http.StatusNotFound, 2)
	src := &Source{URL: srv.URL, Client: srv.Client(), Retries: quick}
	got, err := src.Fetch(context.Background())
	if err != nil {
		t.Fatal(err)
	}
	if len(got.Channels) != 1 || got.Channels[0].Channel != "main" {
		t.Errorf("got %+v", got.Channels)
	}
	if n := asked.Load(); n != 3 {
		t.Errorf("asked %d times, want 3", n)
	}
}

func TestAListThatStaysMissingGivesUp(t *testing.T) {
	srv, asked := flaky(t, nil, http.StatusNotFound, 1000)
	src := &Source{URL: srv.URL, Client: srv.Client(), Retries: quick}
	_, err := src.Fetch(context.Background())
	if err == nil || err.Error() != "the channel list answered 404 Not Found" {
		t.Errorf("got %v", err)
	}
	if n := asked.Load(); n != int32(len(quick)+1) {
		t.Errorf("asked %d times, want %d", n, len(quick)+1)
	}
}

// Only a missing list is the moment it is replaced. Anything else is said
// at once.
func TestOtherAnswersAreNotTriedAgain(t *testing.T) {
	srv, asked := flaky(t, nil, http.StatusInternalServerError, 1000)
	src := &Source{URL: srv.URL, Client: srv.Client(), Retries: quick}
	if _, err := src.Fetch(context.Background()); err == nil {
		t.Error("no error")
	}
	if n := asked.Load(); n != 1 {
		t.Errorf("asked %d times, want 1", n)
	}
}

func TestACheckCalledOffStopsWaiting(t *testing.T) {
	srv, _ := flaky(t, nil, http.StatusNotFound, 1000)
	src := &Source{URL: srv.URL, Client: srv.Client(), Retries: []time.Duration{time.Hour}}
	ctx, cancel := context.WithTimeout(context.Background(), 50*time.Millisecond)
	defer cancel()
	start := time.Now()
	if _, err := src.Fetch(ctx); err == nil {
		t.Error("no error")
	}
	if waited := time.Since(start); waited > 5*time.Second {
		t.Errorf("waited %v", waited)
	}
}

// Several checks at once, while the list is missing and after it is back.
func TestChecksAtOnceWhileTheListIsReplaced(t *testing.T) {
	key := testKey(t)
	list := listJSON(t, build(t, key, "main", "0.3.0-main.1", "https://example.com/main.zip", appZip(t, "x")))
	srv, _ := flaky(t, list, http.StatusNotFound, 5)
	var seen atomic.Int32
	src := &Source{
		URL: srv.URL, Client: srv.Client(),
		Retries: []time.Duration{time.Millisecond, 5 * time.Millisecond, 20 * time.Millisecond, 50 * time.Millisecond},
		Seen:    func(List) { seen.Add(1) },
	}
	var wg sync.WaitGroup
	errs := make(chan error, 8)
	for range 8 {
		wg.Go(func() {
			if _, err := src.Fetch(context.Background()); err != nil {
				errs <- err
			}
		})
	}
	wg.Wait()
	close(errs)
	for err := range errs {
		t.Error(err)
	}
	if n := seen.Load(); n != 8 {
		t.Errorf("seen %d lists, want 8", n)
	}
}
