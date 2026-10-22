package updates

import (
	"context"
	"crypto/ed25519"
	"encoding/json"
	"fmt"
	"net/http"
	"net/http/httptest"
	"sync"
	"sync/atomic"
	"testing"
	"time"

	"github.com/wailsapp/wails/v3/pkg/updater"
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
	t.Parallel()
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
	t.Parallel()
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
	t.Parallel()
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
	t.Parallel()
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
	t.Parallel()
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

// release serves files by name, each answering with what is set for it
// now and 404 when nothing is, the way the release called dev does while
// the workflow replaces one of them. It counts the asks for each.
type release struct {
	mu    sync.Mutex
	files map[string][]byte
	asked map[string]int
}

func serveRelease(t *testing.T) (*release, *httptest.Server) {
	t.Helper()
	r := &release{files: map[string][]byte{}, asked: map[string]int{}}
	srv := httptest.NewTLSServer(http.HandlerFunc(func(w http.ResponseWriter, req *http.Request) {
		r.mu.Lock()
		data, ok := r.files[req.URL.Path]
		r.asked[req.URL.Path]++
		r.mu.Unlock()
		if !ok {
			http.NotFound(w, req)
			return
		}
		_, _ = w.Write(data)
	}))
	t.Cleanup(srv.Close)
	return r, srv
}

func (r *release) set(name string, data []byte) {
	r.mu.Lock()
	defer r.mu.Unlock()
	if data == nil {
		delete(r.files, name)
		return
	}
	r.files[name] = data
}

func (r *release) asks(name string) int {
	r.mu.Lock()
	defer r.mu.Unlock()
	return r.asked[name]
}

// A list with one channel, main at version, written at a moment.
func writtenList(t *testing.T, key ed25519.PrivateKey, version string, written time.Time) []byte {
	t.Helper()
	data, err := json.Marshal(List{
		Channels: []Build{build(t, key, "main", version, "https://example.com/"+version+".zip", appZip(t, version))},
		Written:  written,
	})
	if err != nil {
		t.Fatal(err)
	}
	return data
}

func copiesOf(srv *httptest.Server) *Source {
	return &Source{
		URL:     srv.URL + "/channels.json",
		Copies:  []string{srv.URL + "/channels-a.json", srv.URL + "/channels-b.json"},
		Client:  srv.Client(),
		Retries: quick,
	}
}

func TestTheNewerCopyOfTheListIsRead(t *testing.T) {
	t.Parallel()
	key := testKey(t)
	r, srv := serveRelease(t)
	now := time.Now().UTC().Truncate(time.Second)
	r.set("/channels-a.json", writtenList(t, key, "0.3.0-main.2", now))
	r.set("/channels-b.json", writtenList(t, key, "0.3.0-main.1", now.Add(-time.Minute)))
	got, err := copiesOf(srv).Fetch(context.Background())
	if err != nil {
		t.Fatal(err)
	}
	if v := got.Channels[0].Version; v != "0.3.0-main.2" {
		t.Errorf("read %s, want the newer copy's 0.3.0-main.2", v)
	}
	if !got.Written.Equal(now) {
		t.Errorf("written %v, want %v", got.Written, now)
	}
	// And the other way round, so it is not the first address that wins.
	r.set("/channels-a.json", writtenList(t, key, "0.3.0-main.1", now.Add(-time.Minute)))
	r.set("/channels-b.json", writtenList(t, key, "0.3.0-main.2", now))
	if got, err = copiesOf(srv).Fetch(context.Background()); err != nil || got.Channels[0].Version != "0.3.0-main.2" {
		t.Errorf("got %+v, %v", got.Channels, err)
	}
	if n := r.asks("/channels.json"); n != 0 {
		t.Errorf("the list itself was asked for %d times with both copies there", n)
	}
}

// The copy being replaced is missing, for however long GitHub takes, and
// the other one is read at once, with no waiting and no error.
func TestACopyBeingReplacedIsNoGap(t *testing.T) {
	t.Parallel()
	key := testKey(t)
	r, srv := serveRelease(t)
	r.set("/channels-b.json", writtenList(t, key, "0.3.0-main.1", time.Now().UTC()))
	src := copiesOf(srv)
	src.Retries = []time.Duration{time.Hour}
	start := time.Now()
	got, err := src.Fetch(context.Background())
	if err != nil {
		t.Fatal(err)
	}
	if got.Channels[0].Version != "0.3.0-main.1" {
		t.Errorf("got %+v", got.Channels)
	}
	if waited := time.Since(start); waited > 5*time.Second {
		t.Errorf("waited %v", waited)
	}
	if n := r.asks("/channels.json"); n != 0 {
		t.Errorf("the list itself was asked for %d times", n)
	}
}

// A release from before the copies has only the list, and it is read the
// way it always was, its 404 waited out.
func TestWithNoCopiesTheListIsRead(t *testing.T) {
	t.Parallel()
	key := testKey(t)
	r, srv := serveRelease(t)
	r.set("/channels.json", writtenList(t, key, "0.3.0-main.1", time.Time{}))
	got, err := copiesOf(srv).Fetch(context.Background())
	if err != nil {
		t.Fatal(err)
	}
	if got.Channels[0].Version != "0.3.0-main.1" {
		t.Errorf("got %+v", got.Channels)
	}
	r.set("/channels.json", nil)
	if _, err := copiesOf(srv).Fetch(context.Background()); err == nil || err.Error() != "the channel list answered 404 Not Found" {
		t.Errorf("with nothing there, got %v", err)
	}
	if n := r.asks("/channels.json"); n != 1+len(quick)+1 {
		t.Errorf("the list was asked for %d times, want %d", n, 1+len(quick)+1)
	}
}

// Checks at once at every step of the workflow writing the list again
// and again, always to the older copy: while that copy is missing and
// once it is back. No check ever fails or waits, each reads the newest
// list there is, and the list itself is never needed. The workflow writes
// once at a time and each writing takes half a minute, far longer than a
// check, so the steps are taken in turn here rather than raced.
func TestChecksAtOnceWhileTheCopiesAreReplaced(t *testing.T) {
	t.Parallel()
	key := testKey(t)
	r, srv := serveRelease(t)
	start := time.Now().UTC().Truncate(time.Second)
	lists := make([][]byte, 9)
	for i := range lists {
		lists[i] = writtenList(t, key, fmt.Sprintf("0.3.0-main.%d", i), start.Add(time.Duration(i)*time.Second))
	}
	r.set("/channels-a.json", lists[0])
	src := copiesOf(srv)
	src.Retries = []time.Duration{time.Hour}
	checks := func(want int) {
		t.Helper()
		var wg sync.WaitGroup
		for range 8 {
			wg.Go(func() {
				got, err := src.Fetch(context.Background())
				if err != nil {
					t.Error(err)
					return
				}
				if v, w := got.Channels[0].Version, fmt.Sprintf("0.3.0-main.%d", want); v != w {
					t.Errorf("read %s, want %s", v, w)
				}
			})
		}
		wg.Wait()
	}
	checks(0)
	for i := 1; i < len(lists); i++ {
		name := "/channels-b.json"
		if i%2 == 0 {
			name = "/channels-a.json"
		}
		r.set(name, nil)
		checks(i - 1)
		r.set(name, lists[i])
		checks(i)
	}
	if n := r.asks("/channels.json"); n != 0 {
		t.Errorf("the list itself was asked for %d times", n)
	}
}

// Issue #420: Tim picked a pull request two minutes after its newest build
// was published, while the list, written from every entry in a queue all
// builds share, still named the build before. That one was in the cache,
// so the app came back as it at once. The channel's own entry is uploaded
// with its build, so it is read for the channel followed, and a newer
// build there is the one offered.
func TestTheFollowedChannelsOwnEntryIsNewerThanTheList(t *testing.T) {
	t.Parallel()
	key := testKey(t)
	r, srv := serveRelease(t)
	now := time.Now().UTC().Truncate(time.Second)
	listed := build(t, key, "pr-367", "0.3.0-pr367.5ae00f1", "https://example.com/old.zip", appZip(t, "old"))
	listed.Name, listed.Published, listed.Newest = "#367 Search what was said", now.Add(-time.Hour), "abc4de25c523"
	other := build(t, key, "main", "0.3.0-main.1", "https://example.com/main.zip", appZip(t, "main"))
	list, err := json.Marshal(List{Channels: []Build{other, listed}, Written: now.Add(-2 * time.Minute)})
	if err != nil {
		t.Fatal(err)
	}
	r.set("/channels-a.json", list)
	entry := build(t, key, "pr-367", "0.3.0-pr367.abc4de2", "https://example.com/new.zip", appZip(t, "new"))
	entry.Commit, entry.Published = "abc4de25c523", now.Add(-time.Minute)
	entry.Claim = SignClaim(key, entry)
	data, err := json.Marshal(entry)
	if err != nil {
		t.Fatal(err)
	}
	r.set("/channel-pr-367.json", data)

	var seen []Build
	var told [][2]string
	src := copiesOf(srv)
	src.Entries = srv.URL + "/"
	src.Key = key.Public().(ed25519.PublicKey)
	src.Picked = func() string { return "pr-367" }
	src.Seen = func(l List) { seen = l.Channels }
	src.Newer = func(listed, entry Build) { told = append(told, [2]string{listed.Version, entry.Version}) }
	rel, err := src.Check(context.Background(), updater.CheckRequest{CurrentVersion: "0.3.0-pr412.174466c"})
	if err != nil {
		t.Fatal(err)
	}
	if rel == nil || rel.Version != "0.3.0-pr367.abc4de2" {
		t.Fatalf("offered %+v, not the entry's newer build", rel)
	}
	got, _ := (List{Channels: seen}).Find("pr-367")
	if got.Version != entry.Version || got.Name != listed.Name || got.Newest != "" {
		t.Errorf("the page was shown %+v, not the entry's build with the list's name and nothing newer to come", got)
	}
	if m, _ := (List{Channels: seen}).Find("main"); m.Version != other.Version {
		t.Errorf("main became %+v", m)
	}
	if len(told) != 1 || told[0] != [2]string{listed.Version, entry.Version} {
		t.Errorf("told %v", told)
	}
	if n := r.asks("/channel-main.json"); n != 0 {
		t.Errorf("the entry of a channel not followed was read %d times", n)
	}

	// An entry of the build the list has, or an older one, an entry that
	// is gone for a moment while it is replaced, and one that names
	// another channel all leave the list as it is.
	for name, data := range map[string][]byte{
		"older":   mustJSON(t, func() Build { b := entry; b.Published = now.Add(-2 * time.Hour); return b }()),
		"missing": nil,
		"another": mustJSON(t, func() Build { b := entry; b.Channel = "pr-368"; return b }()),
		"broken":  []byte("{"),
	} {
		r.set("/channel-pr-367.json", data)
		rel, err := src.Check(context.Background(), updater.CheckRequest{CurrentVersion: "0.3.0-pr412.174466c"})
		if err != nil || rel == nil || rel.Version != listed.Version {
			t.Errorf("with an entry %s, offered %+v, %v, not the list's build", name, rel, err)
		}
	}

	// The entry is as untrusted as the list: one whose claim is not of
	// the build it names is not installed.
	forged := entry
	forged.Version = "0.3.0-pr367.fffffff"
	r.set("/channel-pr-367.json", mustJSON(t, forged))
	if rel, err := src.Check(context.Background(), updater.CheckRequest{CurrentVersion: "0.3.0-pr412.174466c"}); err == nil {
		t.Errorf("an entry with a claim of another build was offered: %+v", rel)
	}
}

func mustJSON(t *testing.T, v any) []byte {
	t.Helper()
	data, err := json.Marshal(v)
	if err != nil {
		t.Fatal(err)
	}
	return data
}
