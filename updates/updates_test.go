package updates

import (
	"archive/zip"
	"bytes"
	"context"
	"crypto/ed25519"
	"crypto/rand"
	"encoding/base64"
	"encoding/hex"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"strings"
	"sync"
	"testing"
	"time"

	"github.com/wailsapp/wails/v3/pkg/updater"
)

func testKey(t *testing.T) ed25519.PrivateKey {
	t.Helper()
	_, key, err := ed25519.GenerateKey(rand.Reader)
	if err != nil {
		t.Fatal(err)
	}
	return key
}

// An app in a zip, the way the workflow packs one.
func appZip(t *testing.T, says string) []byte {
	t.Helper()
	var buf bytes.Buffer
	w := zip.NewWriter(&buf)
	for _, dir := range []string{"Frame Fairy.app/", "Frame Fairy.app/Contents/", "Frame Fairy.app/Contents/MacOS/"} {
		if _, err := w.Create(dir); err != nil {
			t.Fatal(err)
		}
	}
	h := &zip.FileHeader{Name: "Frame Fairy.app/Contents/MacOS/framefairy-app", Method: zip.Deflate}
	h.SetMode(0o755)
	f, err := w.CreateHeader(h)
	if err != nil {
		t.Fatal(err)
	}
	_, _ = f.Write([]byte(says))
	if err := w.Close(); err != nil {
		t.Fatal(err)
	}
	return buf.Bytes()
}

func build(t *testing.T, key ed25519.PrivateKey, channel, version, url string, data []byte) Build {
	t.Helper()
	digest, size, err := Digest(bytes.NewReader(data))
	if err != nil {
		t.Fatal(err)
	}
	return Build{
		Channel: channel, Name: channel, Version: version, Commit: "abc1234",
		URL: url, Size: size, SHA256: hex.EncodeToString(digest),
		Signature: Sign(key, digest), Published: time.Now().UTC(),
	}
}

func listJSON(t *testing.T, builds ...Build) []byte {
	t.Helper()
	data, err := json.Marshal(List{Channels: builds})
	if err != nil {
		t.Fatal(err)
	}
	return data
}

func TestParseKeepsGoodBuildsAndSortsThem(t *testing.T) {
	key := testKey(t)
	zipped := appZip(t, "x")
	good := func(ch string) Build {
		return build(t, key, ch, "0.3.0-"+ch+".1", "https://example.com/"+ch+".zip", zipped)
	}
	older := good("pr-18")
	older.Published = older.Published.Add(-time.Hour)
	older.Version = "0.3.0-pr18.0"
	bad := good("pr-7")
	bad.URL = "http://example.com/pr-7.zip"
	odd := good("pr-9")
	odd.Channel = "../pr-9"
	list, err := Parse(listJSON(t, good("pr-5"), older, good("main"), bad, good("pr-18"), odd, good("pr-20")))
	if err != nil {
		t.Fatal(err)
	}
	var got []string
	for _, b := range list.Channels {
		got = append(got, b.Channel+" "+b.Version)
	}
	want := []string{"main 0.3.0-main.1", "pr-20 0.3.0-pr-20.1", "pr-18 0.3.0-pr-18.1", "pr-5 0.3.0-pr-5.1"}
	if strings.Join(got, ", ") != strings.Join(want, ", ") {
		t.Errorf("got %v, want %v", got, want)
	}
}

func TestParseRefusesWhatIsNotAList(t *testing.T) {
	for _, data := range []string{"", "[]", "{", `{"channels": 3}`} {
		if _, err := Parse([]byte(data)); err == nil {
			t.Errorf("%q was read as a list", data)
		}
	}
	if _, err := Parse(bytes.Repeat([]byte(" "), MaxList+1)); err == nil {
		t.Error("a list over the limit was read")
	}
}

// A pull request that was merged goes from the list, and a build that
// followed it follows main.
func TestFollowFallsBackToMain(t *testing.T) {
	key := testKey(t)
	zipped := appZip(t, "x")
	list := List{Channels: []Build{
		build(t, key, "main", "1", "https://example.com/main.zip", zipped),
		build(t, key, "pr-20", "2", "https://example.com/pr-20.zip", zipped),
	}}
	for _, c := range []struct{ picked, own, want string }{
		{"pr-20", "", "pr-20"},
		{"pr-18", "", "main"},
		{"", "pr-20", "pr-20"},
		{"", "pr-18", "main"},
		{"", "", "main"},
		{"main", "pr-20", "main"},
	} {
		b, ok := list.Follow(c.picked, c.own)
		if !ok || b.Channel != c.want {
			t.Errorf("picked %q, own %q: followed %q, want %q", c.picked, c.own, b.Channel, c.want)
		}
	}
	if _, ok := (List{}).Follow("pr-20", "main"); ok {
		t.Error("an empty list gave a channel")
	}
}

func TestSignAndVerify(t *testing.T) {
	key := testKey(t)
	b := build(t, key, "main", "1", "https://example.com/main.zip", appZip(t, "x"))
	if err := Verify(key.Public().(ed25519.PublicKey), b); err != nil {
		t.Error(err)
	}
	other := testKey(t)
	if err := Verify(other.Public().(ed25519.PublicKey), b); err == nil {
		t.Error("a build verified against somebody else's key")
	}
}

func TestKeysRoundTrip(t *testing.T) {
	key := testKey(t)
	seed := key.Seed()
	priv, err := PrivateKey(" " + b64(seed) + "\n")
	if err != nil || !priv.Equal(key) {
		t.Fatalf("private key: %v", err)
	}
	pub, err := PublicKey(b64(key.Public().(ed25519.PublicKey)) + "\n")
	if err != nil || !pub.Equal(key.Public()) {
		t.Fatalf("public key: %v", err)
	}
	if pub, err := PublicKey("\n"); pub != nil || err != nil {
		t.Error("an empty file is a key")
	}
	if _, err := PublicKey("not a key"); err == nil {
		t.Error("text that is not a key was read as one")
	}
}

// An updater with nothing to show it to.
type quietHost struct {
	mu     sync.Mutex
	events []string
}

func (h *quietHost) Emit(name string, _ ...any) bool {
	h.mu.Lock()
	h.events = append(h.events, name)
	h.mu.Unlock()
	return false
}
func (h *quietHost) OnEvent(string, func(any)) func()                      { return func() {} }
func (h *quietHost) OpenWindow(updater.WindowOptions) updater.WindowHandle { return nil }
func (h *quietHost) Quit()                                                 {}

type served struct {
	list  []byte
	zip   []byte
	extra int
}

func serve(t *testing.T, s *served) *httptest.Server {
	t.Helper()
	srv := httptest.NewTLSServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		switch r.URL.Path {
		case "/dev/channels.json":
			_, _ = w.Write(s.list)
		case "/dev/app.zip":
			_, _ = w.Write(s.zip)
			if s.extra > 0 {
				_, _ = w.Write(make([]byte, s.extra))
			}
		default:
			http.NotFound(w, r)
		}
	}))
	t.Cleanup(srv.Close)
	return srv
}

func newUpdater(t *testing.T, src *Source, current string, key ed25519.PrivateKey) *updater.Updater {
	t.Helper()
	u := updater.New(&quietHost{})
	err := u.Init(updater.Config{
		CurrentVersion: current,
		Providers:      []updater.Provider{src},
		PublicKey:      key.Public().(ed25519.PublicKey),
		Window:         updater.WindowNone,
	})
	if err != nil {
		t.Fatal(err)
	}
	return u
}

// The whole way, as the app goes it: read the list, pick the channel,
// download, check the signature, unpack the app.
func TestAnUpdateArrivesUnpacked(t *testing.T) {
	key := testKey(t)
	s := &served{zip: appZip(t, "pull request 20")}
	srv := serve(t, s)
	s.list = listJSON(t,
		build(t, key, "main", "0.3.0-main.4", srv.URL+"/dev/main.zip", appZip(t, "main")),
		build(t, key, "pr-20", "0.3.0-pr20.9", srv.URL+"/dev/app.zip", s.zip),
	)
	var seen List
	var progressed int64
	src := &Source{
		URL: srv.URL + "/dev/channels.json", Client: srv.Client(), Own: "main",
		Picked:   func() string { return "pr-20" },
		Seen:     func(l List) { seen = l },
		Progress: func(w, _ int64) { progressed = w },
	}
	u := newUpdater(t, src, "0.3.0-main.4", key)
	rel, err := u.Check(context.Background())
	if err != nil || rel == nil {
		t.Fatalf("check: %v, %v", rel, err)
	}
	if rel.Version != "0.3.0-pr20.9" || len(seen.Channels) != 2 {
		t.Errorf("offered %s from a list of %d", rel.Version, len(seen.Channels))
	}
	if err := u.DownloadAndInstall(context.Background()); err != nil {
		t.Fatal(err)
	}
	if progressed != int64(len(s.zip)) {
		t.Errorf("progress stopped at %d of %d", progressed, len(s.zip))
	}
	staged := u.DownloadedPath()
	t.Cleanup(func() { _ = os.RemoveAll(filepath.Dir(staged)) })
	if filepath.Base(staged) != "Frame Fairy.app" {
		t.Fatalf("staged %s", staged)
	}
	program := filepath.Join(staged, "Contents", "MacOS", "framefairy-app")
	got, err := os.ReadFile(program)
	if err != nil || string(got) != "pull request 20" {
		t.Errorf("the app in it says %q, %v", got, err)
	}
	// A program that lost its mode on the way would not start.
	if info, err := os.Stat(program); err != nil || info.Mode().Perm()&0o100 == 0 {
		t.Errorf("the program unpacked as %v, %v", info.Mode(), err)
	}
}

// The build running is the channel's build, so there is nothing to do.
func TestTheSameBuildIsNoUpdate(t *testing.T) {
	key := testKey(t)
	s := &served{zip: appZip(t, "main")}
	srv := serve(t, s)
	s.list = listJSON(t, build(t, key, "main", "0.3.0-main.4", srv.URL+"/dev/app.zip", s.zip))
	src := &Source{URL: srv.URL + "/dev/channels.json", Client: srv.Client(), Own: "main"}
	rel, err := newUpdater(t, src, "0.3.0-main.4", key).Check(context.Background())
	if err != nil || rel != nil {
		t.Errorf("offered %v, %v", rel, err)
	}
}

// Sideways is allowed: a build that is older by its number is still
// installed when it is the channel's.
func TestAnOlderNumberIsStillOffered(t *testing.T) {
	key := testKey(t)
	s := &served{zip: appZip(t, "pr 5")}
	srv := serve(t, s)
	s.list = listJSON(t, build(t, key, "pr-5", "0.3.0-pr5.1", srv.URL+"/dev/app.zip", s.zip))
	src := &Source{URL: srv.URL + "/dev/channels.json", Client: srv.Client(), Picked: func() string { return "pr-5" }}
	rel, err := newUpdater(t, src, "0.3.0-pr20.9", key).Check(context.Background())
	if err != nil || rel == nil || rel.Version != "0.3.0-pr5.1" {
		t.Errorf("offered %v, %v", rel, err)
	}
}

// A build signed with any other key is refused before it is unpacked.
func TestABuildSignedByAnybodyElseIsRefused(t *testing.T) {
	ours, theirs := testKey(t), testKey(t)
	s := &served{zip: appZip(t, "not ours")}
	srv := serve(t, s)
	s.list = listJSON(t, build(t, theirs, "main", "9.9.9", srv.URL+"/dev/app.zip", s.zip))
	src := &Source{URL: srv.URL + "/dev/channels.json", Client: srv.Client()}
	u := newUpdater(t, src, "0.3.0-main.4", ours)
	if rel, err := u.Check(context.Background()); err != nil || rel == nil {
		t.Fatalf("check: %v, %v", rel, err)
	}
	if err := u.DownloadAndInstall(context.Background()); err == nil || !strings.Contains(err.Error(), "signature") {
		t.Errorf("installed a build signed by somebody else: %v", err)
	}
	if u.DownloadedPath() != "" {
		t.Error("something was staged")
	}
}

// A zip swapped on the server after it was signed fails its checksum.
func TestASwappedZipIsRefused(t *testing.T) {
	key := testKey(t)
	s := &served{}
	srv := serve(t, s)
	signed := appZip(t, "signed")
	s.list = listJSON(t, build(t, key, "main", "9.9.9", srv.URL+"/dev/app.zip", signed))
	s.zip = appZip(t, "swapped")
	src := &Source{URL: srv.URL + "/dev/channels.json", Client: srv.Client()}
	u := newUpdater(t, src, "0.3.0-main.4", key)
	if _, err := u.Check(context.Background()); err != nil {
		t.Fatal(err)
	}
	if err := u.DownloadAndInstall(context.Background()); err == nil {
		t.Error("a swapped zip was installed")
	}
}

// A server that sends more than the list promised is cut off.
func TestADownloadLargerThanPromisedStops(t *testing.T) {
	key := testKey(t)
	s := &served{zip: appZip(t, "x"), extra: 1 << 20}
	srv := serve(t, s)
	s.list = listJSON(t, build(t, key, "main", "9.9.9", srv.URL+"/dev/app.zip", s.zip))
	src := &Source{URL: srv.URL + "/dev/channels.json", Client: srv.Client()}
	u := newUpdater(t, src, "0.3.0-main.4", key)
	if _, err := u.Check(context.Background()); err != nil {
		t.Fatal(err)
	}
	if err := u.DownloadAndInstall(context.Background()); err == nil || !strings.Contains(err.Error(), "larger") {
		t.Errorf("got %v", err)
	}
}

// A list that is missing, for example while the first build is still
// being made, is an error to show, not a crash.
func TestAMissingListIsAnError(t *testing.T) {
	key := testKey(t)
	srv := serve(t, &served{})
	src := &Source{URL: srv.URL + "/nowhere.json", Client: srv.Client()}
	if _, err := newUpdater(t, src, "1", key).Check(context.Background()); err == nil {
		t.Error("no error")
	}
}

// The channel list comes off the network, so nothing in it may crash the
// app, and whatever is kept passes Check.
func FuzzParse(f *testing.F) {
	f.Add([]byte(`{"channels":[{"channel":"main","name":"main","version":"0.3.0-main.1","commit":"abc1234","url":"https://example.com/a.zip","size":10,"sha256":"` + strings.Repeat("a", 64) + `","signature":"` + b64(make([]byte, 64)) + `"}]}`))
	f.Add([]byte(`{"channels":[{"channel":"pr-0"},{"channel":"pr-1","published":"x"}]}`))
	f.Add([]byte(`{}`))
	f.Fuzz(func(t *testing.T, data []byte) {
		list, err := Parse(data)
		if err != nil {
			return
		}
		seen := map[string]bool{}
		for _, b := range list.Channels {
			if err := b.Check(); err != nil {
				t.Fatalf("kept a build that fails its check: %v", err)
			}
			if seen[b.Channel] {
				t.Fatalf("channel %s twice", b.Channel)
			}
			seen[b.Channel] = true
			if _, err := Release(b); err != nil {
				t.Fatal(err)
			}
		}
	})
}

func b64(b []byte) string { return base64.StdEncoding.EncodeToString(b) }
