package main

import (
	"net/http"
	"net/http/httptest"
	"testing"
	"testing/synctest"
)

// The reset waits for a call the page before it sent, and turns away a
// call that comes while it resets. A call let through a gate by the reset
// wrote into a video's work folder while the reset removed it, and the
// reset failed with "directory not empty", issue #709.
func TestTheResetWaitsForTheCallsOfThePageBefore(t *testing.T) {
	synctest.Test(t, func(t *testing.T) {
		var calls pageCalls
		inside, leave := make(chan struct{}), make(chan struct{})
		wrote := false
		writes := calls.serve(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			inside <- struct{}{}
			<-leave
			wrote = true
		}))
		call := func() *httptest.ResponseRecorder {
			rec := httptest.NewRecorder()
			writes.ServeHTTP(rec, httptest.NewRequest(http.MethodPost, "/call", nil))
			return rec
		}

		// A call of the page before is running.
		first := make(chan *httptest.ResponseRecorder)
		go func() { first <- call() }()
		<-inside

		// The reset waits for it.
		stopped := make(chan func())
		go func() { stopped <- calls.stop() }()
		synctest.Wait()
		select {
		case <-stopped:
			t.Fatal("the reset went on while a call of the page before still ran")
		default:
		}

		// A call that comes meanwhile is turned away and writes nothing.
		if rec := call(); rec.Code != http.StatusGone {
			t.Fatalf("a call that came while the reset waited answered %d, want %d", rec.Code, http.StatusGone)
		}

		close(leave)
		if rec := <-first; rec.Code != http.StatusOK || !wrote {
			t.Fatalf("the call that ran answered %d, wrote %v", rec.Code, wrote)
		}
		(<-stopped)()

		// A call after the reset runs.
		go func() { <-inside }()
		if rec := call(); rec.Code != http.StatusOK {
			t.Fatalf("a call after the reset answered %d", rec.Code)
		}
	})
}
