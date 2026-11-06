package main

import (
	"context"
	"net/http"
	"net/http/httptest"
	"net/url"
	"os"
	"path/filepath"
	"strings"
	"sync"
	"testing"

	"framefairy/engine"
)

func TestLanesRunSideBySide(t *testing.T) {
	st := memoryStore()
	q := newQueue(st, func(JobUpdate) {}, nil)
	release := make(chan struct{})
	started := make(chan string, 2)
	block := func(name string) func(context.Context, *engine.Project) (string, error) {
		return func(ctx context.Context, _ *engine.Project) (string, error) {
			started <- name
			select {
			case <-release:
				return "", nil
			case <-ctx.Done():
				return "", engine.ErrCancelled
			}
		}
	}
	q.add("a.mp4", "model", "Speech model", block("model"))
	q.add("a.mp4", "llm", "Language model", block("llm"))
	// Each job holds its lane until release, so both have started only
	// when the two lanes run side by side. Lanes that took turns would
	// never start the second, and go test's own timeout ends the test and
	// shows where it hung.
	<-started
	<-started
	close(release)
}

// The store the app runs on checks a key typed in the settings with its
// company, and a store a test opens asks nobody.
//
// On the real disk: the stores read their settings in the user's config
// folder, which HOME points at a folder of the test's own. Not parallel:
// it sets HOME and http.DefaultTransport, which the whole process shares.
func TestTheAppsStoreReachesTheCompanies(t *testing.T) {
	home := t.TempDir()
	t.Setenv("HOME", home)
	t.Setenv("XDG_CONFIG_HOME", filepath.Join(home, "config"))
	refusing := &refusesEveryKey{}
	system := http.DefaultTransport
	http.DefaultTransport = refusing
	t.Cleanup(func() { http.DefaultTransport = system })
	anthropic, _ := engine.ProviderNamed("anthropic")

	if err := openStore().machine.VerifyAPIKey(context.Background(), anthropic, "sk-ant-typed"); err != nil || len(refusing.seen()) != 0 {
		t.Fatalf("a store a test opens asked %v, and said %v", refusing.seen(), err)
	}
	err := appStore().machine.VerifyAPIKey(context.Background(), anthropic, "sk-ant-typed")
	if err == nil || !strings.Contains(err.Error(), "Anthropic did not accept this key") {
		t.Errorf("the app's store said %v", err)
	}
	if asked := refusing.seen(); len(asked) != 1 || asked[0] != "api.anthropic.com/v1/models" {
		t.Errorf("the app's store asked %v", asked)
	}
}

// refusesEveryKey is a network that answers every request as a company
// does a key it does not know, and keeps where each was meant to go.
type refusesEveryKey struct {
	mu    sync.Mutex
	asked []string
}

func (r *refusesEveryKey) RoundTrip(req *http.Request) (*http.Response, error) {
	r.mu.Lock()
	r.asked = append(r.asked, req.URL.Host+req.URL.Path)
	r.mu.Unlock()
	return &http.Response{StatusCode: http.StatusUnauthorized, Body: http.NoBody, Header: http.Header{}, Request: req}, nil
}

func (r *refusesEveryKey) seen() []string {
	r.mu.Lock()
	defer r.mu.Unlock()
	return append([]string(nil), r.asked...)
}

// Nothing serves the bytes of a file to the page, not even of an episode
// in the library: the video preview reads nothing of the file itself, see
// docs/VIDEO-PREVIEW.md, step 5. The frames route takes an episode's file
// and nothing else.
//
// On the real disk: the frames route hands an episode to its decoder,
// which reads it.
func TestNothingServesTheBytesOfAFile(t *testing.T) {
	home := t.TempDir()
	t.Setenv("HOME", home)
	t.Setenv("XDG_CONFIG_HOME", filepath.Join(home, "config"))
	source := filepath.Join(home, "ep.mp4")
	other := filepath.Join(home, "secret.txt")
	if err := os.WriteFile(source, []byte("0123456789"), 0o644); err != nil {
		t.Fatal(err)
	}
	_ = os.WriteFile(other, []byte("nope"), 0o644)
	st := openStore()
	if _, err := st.AddEpisodes([]string{source}); err != nil {
		t.Fatal(err)
	}
	handler := mediaMiddleware(&FrameFairy{store: st})(http.NotFoundHandler())

	rec := httptest.NewRecorder()
	handler.ServeHTTP(rec, httptest.NewRequest("GET", "/media/?path="+url.QueryEscape(source), nil))
	if rec.Code != http.StatusNotFound || strings.Contains(rec.Body.String(), "0123") {
		t.Errorf("the episode's file was served with %d: %q", rec.Code, rec.Body.String())
	}
	rec = httptest.NewRecorder()
	handler.ServeHTTP(rec, httptest.NewRequest("GET", "/frames/release?path="+url.QueryEscape(source), nil))
	if rec.Code != http.StatusNoContent {
		t.Errorf("the episode's file was not taken by the frames route: %d", rec.Code)
	}
	for _, path := range []string{other, filepath.Join(engine.WorkDir(source), "..", "secret.txt"), "relative.mp4"} {
		rec = httptest.NewRecorder()
		handler.ServeHTTP(rec, httptest.NewRequest("GET", "/frames/release?path="+url.QueryEscape(path), nil))
		if rec.Code != http.StatusNotFound {
			t.Errorf("%s was taken with %d", path, rec.Code)
		}
	}
}

// An episode that has not been transcribed yet is a state every episode
// starts in. The reads the workspace makes on opening must answer with
// nothing rather than fail, or the log fills with errors nobody can act on.
func TestAnEpisodeWithoutATranscriptReadsEmpty(t *testing.T) {
	svc, eps := memoryLibrary(t, "ep.mp4")
	source := eps[0]

	peaks, err := svc.Waveform(source, 0, 60, 100)
	if err != nil || len(peaks) != 0 {
		t.Errorf("waveform got %d peaks, %v", len(peaks), err)
	}
	words, err := svc.Words(source, 0, 60)
	if err != nil || words == nil || len(words) != 0 {
		t.Errorf("words got %v, %v", words, err)
	}
	sentences, err := svc.Sentences(source)
	if err != nil || sentences == nil || len(sentences) != 0 {
		t.Errorf("sentences got %v, %v", sentences, err)
	}
	if room := svc.store.machine.Episode(source).Room(svc.store.Settings().options()); len(room.Lines) != 0 || room.Chars <= 0 || room.Rate != engine.SpokenChars {
		t.Errorf("room %+v", room)
	}
}
