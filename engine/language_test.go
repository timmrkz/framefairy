package engine

import (
	"bytes"
	"context"
	"crypto/sha256"
	"fmt"
	"net/http"
	"net/http/httptest"
	"net/url"
	"os"
	"path/filepath"
	"strings"
	"sync/atomic"
	"testing"
)

// A model file as far as anything here is concerned: the four bytes that
// tell one from a page saying no, and then some weight.
func ggufBytes(n int) []byte {
	body := append([]byte("GGUF"), make([]byte, n)...)
	for i := 4; i < len(body); i++ {
		body[i] = byte(i)
	}
	return body
}

// What the catalogue has to be true of, whatever is in it. A model nobody
// can describe cannot be offered, and a model with no home cannot be
// fetched.
func TestEveryLanguageModelCanBeOffered(t *testing.T) {
	models := LanguageModels()
	if len(models) == 0 {
		t.Fatal("no language models at all, so nobody can choose one")
	}
	seen := map[string]bool{}
	for _, m := range models {
		if m.Name == "" || m.Title == "" || m.Maker == "" || m.About == "" {
			t.Errorf("a model the app cannot describe: %+v", m)
		}
		if !strings.HasSuffix(m.Name, ".gguf") {
			t.Errorf("%s does not land as a .gguf, which is the only thing llama-server reads", m.Name)
		}
		if m.Download <= 0 || m.Needs <= 0 {
			t.Errorf("%s says nothing about what it costs: %d to fetch, %d to run",
				m.Title, m.Download, m.Needs)
		}
		// A model is never smaller in memory than it is on disk: the
		// weights are the file, and the context sits on top of them.
		if m.Needs < m.Download {
			t.Errorf("%s claims to need less memory (%d) than it is big (%d)",
				m.Title, m.Needs, m.Download)
		}
		parsed, err := url.Parse(m.URL)
		if err != nil || parsed.Scheme != "https" || parsed.Host == "" {
			t.Errorf("%s has no home to be fetched from: %q", m.Title, m.URL)
		}
		if seen[m.Name] {
			t.Errorf("two models land as the same file, %s", m.Name)
		}
		seen[m.Name] = true
		// A checksum, because this is a file off the internet that is then
		// fed to a program. Sixty four hex characters or nothing.
		if len(m.SHA256) != 64 {
			t.Errorf("%s has no checksum: %q", m.Title, m.SHA256)
		}
		// Whoever finds a model by name has to find this one.
		if got, ok := LanguageModelByName(m.Name); !ok || got.Title != m.Title {
			t.Errorf("%s cannot be found by its own name", m.Title)
		}
	}
	if _, ok := LanguageModelByName("no-such-model.gguf"); ok {
		t.Error("found a model that does not exist")
	}
}

// The whole point of the memory part: a machine is never offered a model
// it cannot hold. The numbers are made up here on purpose, because the
// question is the rule and not the catalogue.
func TestWhatAMachineCanHold(t *testing.T) {
	const gb = 1 << 30
	model := LanguageModel{Title: "Test", Needs: 18 * gb}

	for _, c := range []struct {
		machine int64
		want    Fit
		why     string
	}{
		{0, FitUnknown, "a machine that will not say"},
		{-1, FitUnknown, "a machine that says something silly"},
		{8 * gb, TooBig, "half what it needs"},
		{16 * gb, TooBig, "just under what it needs"},
		{18 * gb, FitsTight, "exactly what it needs and nothing over"},
		{24 * gb, FitsTight, "over, but not the headroom"},
		{26 * gb, FitsWell, "exactly the headroom"},
		{32 * gb, FitsWell, "Tim's machine"},
		{128 * gb, FitsWell, "a machine that laughs at it"},
	} {
		if got := model.FitsIn(c.machine); got != c.want {
			t.Errorf("%d GB, %s: %q, wanted %q", c.machine/gb, c.why, got, c.want)
		}
	}
}

// What a machine is offered. The recommendation is never a model that
// machine cannot hold: that would be a joke at the customer's expense,
// and it is the whole reason any of this reads the memory at all.
func TestWhatEachMachineIsOffered(t *testing.T) {
	const gb = 1 << 30
	for _, machine := range []int64{8 * gb, 16 * gb, 18 * gb, 24 * gb, 32 * gb, 64 * gb, 128 * gb} {
		got, ok := RecommendedFor(machine)
		if !ok {
			t.Logf("%3d GB  nothing fits", machine/gb)
			continue
		}
		fit := got.FitsIn(machine)
		t.Logf("%3d GB  %-18s %s", machine/gb, got.Title, fit)
		if fit == TooBig {
			t.Errorf("%d GB was offered %s, which does not fit it", machine/gb, got.Title)
		}
		// And it is the largest model that does, or the choice is being
		// made badly rather than not at all. The largest model, by its
		// file, and not the one that takes the most memory.
		for _, other := range LanguageModels() {
			if other.Download <= got.Download {
				continue
			}
			if other.FitsIn(machine) == fit {
				t.Errorf("%d GB was offered %s when %s fits it just as well",
					machine/gb, got.Title, other.Title)
			}
		}
	}

	// A machine that will not say gets the smallest, because that is the
	// one most likely to run and a guess should be the cautious one.
	got, ok := RecommendedFor(0)
	if !ok {
		t.Fatal("a machine that will not say was offered nothing at all")
	}
	for _, other := range LanguageModels() {
		if other.Download < got.Download {
			t.Errorf("a machine that will not say was offered %s, and %s is smaller",
				got.Title, other.Title)
		}
	}

	// A machine too small for any of them is offered none, and then the
	// app says so rather than promising something that cannot work.
	if _, ok := RecommendedFor(1 * gb); ok {
		t.Error("a 1 GB machine was offered a model")
	}
}

// Every model is fetched from whoever made it, over a connection that can
// be checked. A model is a file off the internet that is then fed to a
// program, so where it comes from is not a detail.
func TestEveryModelComesFromItsOwnMaker(t *testing.T) {
	makers := map[string]bool{}
	for _, m := range LanguageModels() {
		makers[m.Maker] = true
		if !strings.HasPrefix(m.URL, "https://huggingface.co/") {
			t.Errorf("%s comes from %s", m.Title, m.URL)
		}
	}
	// The point of a list rather than one model is that there is a choice,
	// and a choice between four of one house is not much of one.
	if len(makers) < 2 {
		t.Errorf("every model on the list is from %v", makers)
	}
}

// Reading the memory of the machine this runs on. It is allowed to say
// nothing, and what it must never do is say something impossible.
func TestTheMachineSaysHowMuchMemoryItHas(t *testing.T) {
	got := MachineMemory()
	if got < 0 {
		t.Fatalf("negative memory: %d", got)
	}
	if got == 0 {
		t.Skip("this machine does not say, which is an answer")
	}
	const gb = 1 << 30
	if got < gb || got > 8192*gb {
		t.Errorf("%d bytes is not an amount of memory a machine has", got)
	}
	t.Logf("this machine has %d GB", got/gb)
}

func TestReadingMemTotal(t *testing.T) {
	dir := t.TempDir()
	write := func(name, body string) string {
		path := filepath.Join(dir, name)
		if err := os.WriteFile(path, []byte(body), 0o644); err != nil {
			t.Fatal(err)
		}
		return path
	}
	if got := memTotal(write("ok", "MemFree: 1 kB\nMemTotal:   32768000 kB\nBuffers: 2 kB\n")); got != 32768000*1024 {
		t.Errorf("got %d", got)
	}
	if got := memTotal(write("none", "MemFree: 1 kB\n")); got != 0 {
		t.Errorf("a file with no total gave %d", got)
	}
	if got := memTotal(write("odd", "MemTotal: lots\n")); got != 0 {
		t.Errorf("a total that is not a number gave %d", got)
	}
	if got := memTotal(filepath.Join(dir, "not-there")); got != 0 {
		t.Errorf("a file that is not there gave %d", got)
	}
}

// Installing one, against a server of our own. No network and no real
// model: the question is what is left on disk afterwards, in each case.
func TestInstallingALanguageModel(t *testing.T) {
	body := ggufBytes(4096)

	serve := func(handler http.HandlerFunc) *httptest.Server {
		server := httptest.NewServer(handler)
		t.Cleanup(server.Close)
		return server
	}
	whole := serve(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Length", fmt.Sprint(len(body)))
		_, _ = w.Write(body)
	})

	t.Run("it lands where the engine looks", func(t *testing.T) {
		dir := t.TempDir()
		m := LanguageModel{Name: "test.gguf", Title: "Test", URL: whole.URL,
			Download: int64(len(body)), Needs: 1}
		if m.Installed(dir) {
			t.Fatal("said it was there before anything happened")
		}
		if err := InstallLanguageModel(context.Background(), NewLog(os.Stderr, false, false), m, dir); err != nil {
			t.Fatal(err)
		}
		if !m.Installed(dir) {
			t.Fatal("installed it and then could not see it")
		}
		left, _ := os.ReadDir(dir)
		if len(left) != 1 {
			for _, e := range left {
				t.Logf("left behind: %s", e.Name())
			}
			t.Errorf("%d things in the models folder, wanted the model and nothing else", len(left))
		}
		// And again is not a second download.
		if err := InstallLanguageModel(context.Background(), NewLog(os.Stderr, false, false), m, dir); err != nil {
			t.Fatal(err)
		}
	})

	t.Run("a page saying no is not a model", func(t *testing.T) {
		// The one that matters. Anything on the way to the file can answer
		// with HTML and a 200, and a browser would show it, and a file
		// called something.gguf full of HTML looks installed and fails on
		// the first prompt.
		lying := serve(func(w http.ResponseWriter, r *http.Request) {
			_, _ = w.Write([]byte("<html><body>Not found</body></html>"))
		})
		dir := t.TempDir()
		m := LanguageModel{Name: "test.gguf", Title: "Test", URL: lying.URL, Download: 35, Needs: 1}
		err := InstallLanguageModel(context.Background(), NewLog(os.Stderr, false, false), m, dir)
		if err == nil {
			t.Fatal("took a web page for a model")
		}
		if left, _ := os.ReadDir(dir); len(left) != 0 {
			t.Errorf("left %d things behind", len(left))
		}
	})

	t.Run("a checksum that does not match", func(t *testing.T) {
		dir := t.TempDir()
		m := LanguageModel{Name: "test.gguf", Title: "Test", URL: whole.URL,
			Download: int64(len(body)), Needs: 1,
			SHA256: "0000000000000000000000000000000000000000000000000000000000000000"}
		if err := InstallLanguageModel(context.Background(), NewLog(os.Stderr, false, false), m, dir); err == nil {
			t.Fatal("took a model that is not the one expected")
		}
		if left, _ := os.ReadDir(dir); len(left) != 0 {
			t.Errorf("left %d things behind", len(left))
		}
	})

	t.Run("a server that says no", func(t *testing.T) {
		gone := serve(func(w http.ResponseWriter, r *http.Request) {
			http.Error(w, "no", http.StatusNotFound)
		})
		dir := t.TempDir()
		m := LanguageModel{Name: "test.gguf", Title: "Test", URL: gone.URL, Download: 10, Needs: 1}
		if err := InstallLanguageModel(context.Background(), NewLog(os.Stderr, false, false), m, dir); err == nil {
			t.Fatal("a 404 was taken for a model")
		}
		if left, _ := os.ReadDir(dir); len(left) != 0 {
			t.Errorf("left %d things behind", len(left))
		}
	})

	t.Run("cancelled part way", func(t *testing.T) {
		slow := serve(func(w http.ResponseWriter, r *http.Request) {
			w.Header().Set("Content-Length", fmt.Sprint(len(body)*4))
			_, _ = w.Write(body)
			if f, ok := w.(http.Flusher); ok {
				f.Flush()
			}
			<-r.Context().Done()
		})
		dir := t.TempDir()
		m := LanguageModel{Name: "test.gguf", Title: "Test", URL: slow.URL,
			Download: int64(len(body) * 4), Needs: 1}
		ctx, stop := context.WithCancel(context.Background())
		// Once some of it is really on disk, not merely once the file
		// exists: the file is made before the first byte arrives, and
		// stopping there would leave nothing to carry on from and prove
		// nothing about what happens when a real download drops.
		go func() {
			for {
				if info, err := os.Stat(filepath.Join(dir, "test.gguf.part")); err == nil && info.Size() > 0 {
					stop()
					return
				}
			}
		}()
		if err := InstallLanguageModel(ctx, NewLog(os.Stderr, false, false), m, dir); err == nil {
			t.Fatal("a cancelled download reported success")
		}
		stop()
		if m.Installed(dir) {
			t.Error("half a model looks installed, which is the worst of both")
		}
		// What arrived stays, under a part name. That is not mess, it is
		// what the next try carries on from: fifteen gigabytes that drop
		// at nine tenths should not begin again at nothing.
		info, err := os.Stat(filepath.Join(dir, "test.gguf.part"))
		if err != nil {
			t.Fatalf("what was fetched was thrown away: %v", err)
		}
		if info.Size() == 0 {
			t.Error("the part file is empty, so there is nothing to carry on from")
		}
	})
}

// Carrying on where a stopped download left off. A language model is up to
// fifteen gigabytes and a connection that drops is an ordinary thing, so
// this is the difference between a retry and a whole evening.
func TestADownloadCarriesOnWhereItStopped(t *testing.T) {
	body := ggufBytes(60_000)

	// A server that does ranges, and counts how many bytes it was asked to
	// send, so the test can say whether anything was fetched twice. The
	// counting is atomic because the handler runs on the server's own
	// goroutine and the test reads it from its own.
	var sent, ranged atomic.Int64
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		from := int64(0)
		if h := r.Header.Get("Range"); h != "" {
			ranged.Add(1)
			fmt.Sscanf(h, "bytes=%d-", &from)
			if from >= int64(len(body)) {
				http.Error(w, "range", http.StatusRequestedRangeNotSatisfiable)
				return
			}
			w.Header().Set("Content-Range",
				fmt.Sprintf("bytes %d-%d/%d", from, len(body)-1, len(body)))
			w.Header().Set("Content-Length", fmt.Sprint(int64(len(body))-from))
			w.WriteHeader(http.StatusPartialContent)
		} else {
			w.Header().Set("Content-Length", fmt.Sprint(len(body)))
		}
		n, _ := w.Write(body[from:])
		sent.Add(int64(n))
	}))
	t.Cleanup(server.Close)

	dir := t.TempDir()
	m := LanguageModel{Name: "test.gguf", Title: "Test", URL: server.URL,
		Download: int64(len(body)), Needs: 1,
		SHA256: fmt.Sprintf("%x", sha256.Sum256(body))}

	// Half of it is already on disk, the way a stopped download leaves it.
	half := len(body) / 2
	if err := os.WriteFile(filepath.Join(dir, "test.gguf.part"), body[:half], 0o644); err != nil {
		t.Fatal(err)
	}

	if err := InstallLanguageModel(context.Background(), NewLog(os.Stderr, false, false), m, dir); err != nil {
		t.Fatal(err)
	}
	if !m.Installed(dir) {
		t.Fatal("it did not land")
	}
	if got := ranged.Load(); got != 1 {
		t.Errorf("%d range requests, wanted one", got)
	}
	if got := sent.Load(); got >= int64(len(body)) {
		t.Errorf("fetched %d bytes of a %d byte file, so it started again", got, len(body))
	}
	// And the whole file is right, which is the thing that would quietly
	// break: the checksum has to be of the bytes on disk plus the bytes
	// fetched, in that order, not of the ones fetched alone.
	got, err := os.ReadFile(filepath.Join(dir, "test.gguf"))
	if err != nil {
		t.Fatal(err)
	}
	if !bytes.Equal(got, body) {
		t.Errorf("the file came to %d bytes and should be %d", len(got), len(body))
	}
}

// A server that will not do ranges answers with the whole file. Then the
// part file is written again from the start, and what comes out is still
// the right thing.
func TestAServerThatWillNotResumeStillWorks(t *testing.T) {
	body := ggufBytes(20_000)
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Length", fmt.Sprint(len(body)))
		_, _ = w.Write(body)
	}))
	t.Cleanup(server.Close)

	dir := t.TempDir()
	m := LanguageModel{Name: "test.gguf", Title: "Test", URL: server.URL,
		Download: int64(len(body)), Needs: 1,
		SHA256: fmt.Sprintf("%x", sha256.Sum256(body))}
	if err := os.WriteFile(filepath.Join(dir, "test.gguf.part"), body[:1000], 0o644); err != nil {
		t.Fatal(err)
	}
	if err := InstallLanguageModel(context.Background(), NewLog(os.Stderr, false, false), m, dir); err != nil {
		t.Fatal(err)
	}
	got, err := os.ReadFile(filepath.Join(dir, "test.gguf"))
	if err != nil {
		t.Fatal(err)
	}
	if !bytes.Equal(got, body) {
		t.Errorf("the file came to %d bytes and should be %d", len(got), len(body))
	}
}

// Bytes that turned out to be the wrong ones are not carried on from, or a
// bad download would be resumed for ever and fail the same way every time.
func TestWrongBytesAreNotCarriedOnFrom(t *testing.T) {
	body := ggufBytes(4096)
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Length", fmt.Sprint(len(body)))
		_, _ = w.Write(body)
	}))
	t.Cleanup(server.Close)

	dir := t.TempDir()
	m := LanguageModel{Name: "test.gguf", Title: "Test", URL: server.URL,
		Download: int64(len(body)), Needs: 1,
		SHA256: "0000000000000000000000000000000000000000000000000000000000000000"}
	if err := InstallLanguageModel(context.Background(), NewLog(os.Stderr, false, false), m, dir); err == nil {
		t.Fatal("took a model that is not the one expected")
	}
	if _, err := os.Stat(filepath.Join(dir, "test.gguf.part")); err == nil {
		t.Error("the wrong bytes were kept, so every try from now on resumes them")
	}
}

// A file of the right name is not a model, and this is the only thing that
// tells the difference.
func TestOnlyAGGUFCountsAsInstalled(t *testing.T) {
	dir := t.TempDir()
	m := LanguageModel{Name: "test.gguf", Title: "Test", Needs: 1}
	path := filepath.Join(dir, m.Name)

	for _, c := range []struct {
		what string
		body []byte
		want bool
	}{
		{"nothing at all", nil, false},
		{"empty", []byte{}, false},
		{"a web page", []byte("<html>no</html>"), false},
		{"three bytes", []byte("GGU"), false},
		{"a model", ggufBytes(64), true},
	} {
		_ = os.Remove(path)
		if c.body != nil {
			if err := os.WriteFile(path, c.body, 0o644); err != nil {
				t.Fatal(err)
			}
		}
		if got := m.Installed(dir); got != c.want {
			t.Errorf("%s: installed %v, wanted %v", c.what, got, c.want)
		}
	}

	// A folder with the model's name is not a model either.
	_ = os.Remove(path)
	if err := os.MkdirAll(path, 0o755); err != nil {
		t.Fatal(err)
	}
	if m.Installed(dir) {
		t.Error("a folder was taken for a model")
	}
}

// What each Mac is actually offered, by name. The table above checks that
// the choice follows its own rule. This checks the rule gives the right
// answers, which is a different question, and the one that was wrong: the
// old rule told a 16 GB Mac to install Ministral 3 8B and a 24 GB Mac to
// install Qwen3 14B, each as the best for it, and neither fits.
//
// Changing the catalogue or the rule should fail this, so that somebody
// looks at what every size of Mac would now be told before it ships.
func TestWhatEveryMacIsOffered(t *testing.T) {
	const gb = 1 << 30
	for _, c := range []struct {
		memory int64
		want   string
		fit    Fit
	}{
		{8 * gb, "", ""},
		{16 * gb, "Gemma 4 12B", FitsTight},
		{18 * gb, "Gemma 4 12B", FitsWell},
		{24 * gb, "Gemma 4 12B", FitsWell},
		{32 * gb, "Gemma 4 26B A4B", FitsWell},
		{36 * gb, "Gemma 4 26B A4B", FitsWell},
		{64 * gb, "Gemma 4 26B A4B", FitsWell},
	} {
		got, ok := RecommendedFor(c.memory)
		if c.want == "" {
			if ok {
				t.Errorf("%d GB was offered %s, and nothing fits it", c.memory/gb, got.Title)
			}
			continue
		}
		if !ok || got.Title != c.want || got.FitsIn(c.memory) != c.fit {
			t.Errorf("%d GB was offered %q (%s), want %q (%s)",
				c.memory/gb, got.Title, got.FitsIn(c.memory), c.want, c.fit)
		}
	}
}

// The property the old rule could not see. A model whose layers mostly
// look back over a short window barely grows with the length of a search.
// One whose layers all look back over everything grows with every token.
func TestACacheGrowsWithTheSearchOnlyWhereTheModelLooksBackOverAllOfIt(t *testing.T) {
	byTitle := map[string]LanguageModel{}
	for _, m := range LanguageModels() {
		byTitle[m.Title] = m
	}
	growth := func(title string) float64 {
		m := byTitle[title]
		return float64(m.Shape.cacheBytes(131072)) / float64(m.Shape.cacheBytes(32768))
	}
	// Four times the context. Every layer of Qwen3 and Ministral holds all
	// of it, so their cache is four times the size.
	for _, title := range []string{"Qwen3 14B", "Ministral 3 8B"} {
		if g := growth(title); g < 3.99 || g > 4.01 {
			t.Errorf("%s grew %.2f times for four times the context, want 4", title, g)
		}
	}
	// Most of Gemma 4's layers hold only the last 1024 tokens, so its
	// cache grows much less than the context does.
	for _, title := range []string{"Gemma 4 26B A4B", "Gemma 4 12B"} {
		if g := growth(title); g > 3.5 {
			t.Errorf("%s grew %.2f times for four times the context, want well under 4", title, g)
		}
	}
}

// Worked out from the shape, and pinned to what the maker publishes, so a
// typing mistake in a shape shows up as a number that is plainly wrong.
// Qwen3 14B at 64k tokens: 40 layers, 8 heads of 128, a key and a value, two
// bytes each, is exactly ten gibibytes of cache.
func TestTheCacheIsWorkedOutFromTheShape(t *testing.T) {
	for _, m := range LanguageModels() {
		if m.Title == "Qwen3 14B" {
			if got := m.Shape.cacheBytes(65536); got != 10<<30 {
				t.Errorf("Qwen3 14B at 64k: %d bytes of cache, want exactly 10 GiB", got)
			}
		}
		if m.Needs != m.NeedsAt(judgedContext) {
			t.Errorf("%s says it needs %d, and its shape says %d", m.Title, m.Needs, m.NeedsAt(judgedContext))
		}
		if m.Shape.FullLayers+m.Shape.WindowLayers == 0 {
			t.Errorf("%s has no shape, so its cache is counted as nothing", m.Title)
		}
	}
}

// The context a model is judged at has to cover the first search the app
// makes by itself, or the check answers a question nobody asked. The first
// search is the first half hour, taken whole up to 45 minutes when that
// would leave only a scrap, see nextWindow in frontend/src/lib/flow.ts.
//
// Characters per minute of episode is an estimate: German conversation at
// around 150 words a minute, and the numbering and timings each line of
// the prompt carries. It errs high on purpose.
func TestTheJudgedContextCoversTheFirstSearch(t *testing.T) {
	const charsPerMinute = 1200
	const firstSearchMinutes = 45
	got := contextFor(charsPerMinute*firstSearchMinutes, 48000)
	if got > judgedContext {
		t.Errorf("the first search asks for %d tokens of context, and models are "+
			"judged at %d, so the check says a machine can hold what it cannot", got, judgedContext)
	}
}
