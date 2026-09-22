package engine

import (
	"context"
	"fmt"
	"net/http"
	"net/http/httptest"
	"net/url"
	"os"
	"path/filepath"
	"strings"
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
	recommended := 0
	for _, m := range models {
		if m.Name == "" || m.Title == "" || m.Maker == "" || m.About == "" {
			t.Errorf("a model the window cannot describe: %+v", m)
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
		if m.Recommended {
			recommended++
		}
		// Whoever finds a model by name has to find this one.
		if got, ok := LanguageModelByName(m.Name); !ok || got.Title != m.Title {
			t.Errorf("%s cannot be found by its own name", m.Title)
		}
	}
	if recommended != 1 {
		t.Errorf("%d models are recommended, and the window offers one", recommended)
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

// The recommended model has to be one a real machine can run, or the
// recommendation is a joke at the customer's expense. 32 GB is the machine
// this is built on.
func TestTheRecommendedModelRunsOnARealMachine(t *testing.T) {
	const gb = 1 << 30
	for _, m := range LanguageModels() {
		if !m.Recommended {
			continue
		}
		if fit := m.FitsIn(32 * gb); fit == TooBig {
			t.Errorf("%s is recommended and does not fit a 32 GB machine", m.Title)
		}
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
		go func() {
			for {
				if _, err := os.Stat(filepath.Join(dir, "test.gguf.part")); err == nil {
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
		if left, _ := os.ReadDir(dir); len(left) != 0 {
			t.Errorf("left %d things behind", len(left))
		}
	})
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
