package engine

import (
	"archive/tar"
	"bytes"
	"context"
	"io"
	"net/http"
	"net/http/httptest"
	"os"
	"os/exec"
	"path/filepath"
	"strconv"
	"strings"
	"testing"
)

// A tar.bz2 the way the real one comes: one folder named after the model,
// with tokens.txt inside it. bzip2 has no writer in the standard library,
// so the system's is used and the test skips without it.
func modelArchive(t *testing.T, entries map[string]string) []byte {
	t.Helper()
	if _, err := exec.LookPath("bzip2"); err != nil {
		t.Skip("bzip2 is not installed")
	}
	var plain bytes.Buffer
	w := tar.NewWriter(&plain)
	for name, body := range entries {
		head := &tar.Header{Name: name, Mode: 0o644, Size: int64(len(body)), Typeflag: tar.TypeReg}
		if body == "" && strings.HasSuffix(name, "/") {
			head = &tar.Header{Name: name, Mode: 0o755, Typeflag: tar.TypeDir}
		}
		if err := w.WriteHeader(head); err != nil {
			t.Fatal(err)
		}
		if head.Typeflag == tar.TypeReg {
			if _, err := w.Write([]byte(body)); err != nil {
				t.Fatal(err)
			}
		}
	}
	if err := w.Close(); err != nil {
		t.Fatal(err)
	}
	cmd := exec.Command("bzip2", "-c")
	cmd.Stdin = &plain
	packed, err := cmd.Output()
	if err != nil {
		t.Fatal(err)
	}
	return packed
}

// Serves an archive, so the whole install runs without reaching the real
// internet. Tests need no network, which is a rule.
func serving(t *testing.T, body []byte) *httptest.Server {
	t.Helper()
	s := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("content-length", strconv.Itoa(len(body)))
		w.Write(body)
	}))
	t.Cleanup(s.Close)
	return s
}

func quietLog() *Log { return NewLog(io.Discard, false, false) }

func TestInstallingASpeechModel(t *testing.T) {
	const name = "test-model"
	good := modelArchive(t, map[string]string{
		name + "/tokens.txt": "a 0\nb 1\n",
		name + "/model.onnx": "not really a model",
	})

	t.Run("it arrives, unpacks and is usable", func(t *testing.T) {
		dir := t.TempDir()
		s := serving(t, good)
		m := SpeechModel{Name: name, Title: "Test", URL: s.URL, Download: int64(len(good))}
		if m.Installed(dir) {
			t.Fatal("said it was there before anything happened")
		}
		if err := InstallSpeechModel(context.Background(), quietLog(), m, dir); err != nil {
			t.Fatal(err)
		}
		if !m.Installed(dir) {
			t.Fatal("installed and then said it was not there")
		}
		body, err := os.ReadFile(filepath.Join(dir, name, "tokens.txt"))
		if err != nil || len(body) == 0 {
			t.Fatalf("tokens.txt: %v", err)
		}
		// Nothing half finished is left lying about.
		for _, leftover := range []string{name + ".part", name + ".unpacking"} {
			if _, err := os.Stat(filepath.Join(dir, leftover)); err == nil {
				t.Errorf("%s was left behind", leftover)
			}
		}
	})

	t.Run("installing again does nothing and does not fetch", func(t *testing.T) {
		dir := t.TempDir()
		asked := 0
		s := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			asked++
			w.Write(good)
		}))
		defer s.Close()
		m := SpeechModel{Name: name, Title: "Test", URL: s.URL, Download: int64(len(good))}
		for i := 0; i < 2; i++ {
			if err := InstallSpeechModel(context.Background(), quietLog(), m, dir); err != nil {
				t.Fatal(err)
			}
		}
		if asked != 1 {
			t.Errorf("fetched %d times, want 1", asked)
		}
	})

	// What arrives over a network is untrusted. The checksum is the whole
	// reason the model is not unpacked straight from the socket.
	t.Run("a model that is not what it claims is refused", func(t *testing.T) {
		dir := t.TempDir()
		s := serving(t, good)
		m := SpeechModel{Name: name, Title: "Test", URL: s.URL, Download: int64(len(good)),
			SHA256: "0000000000000000000000000000000000000000000000000000000000000000"}
		err := InstallSpeechModel(context.Background(), quietLog(), m, dir)
		if err == nil {
			t.Fatal("took a model with the wrong checksum")
		}
		if m.Installed(dir) {
			t.Error("and unpacked it anyway")
		}
	})

	t.Run("an archive without the model in it is refused", func(t *testing.T) {
		dir := t.TempDir()
		wrong := modelArchive(t, map[string]string{"readme.txt": "nothing to see"})
		s := serving(t, wrong)
		m := SpeechModel{Name: name, Title: "Test", URL: s.URL, Download: int64(len(wrong))}
		if err := InstallSpeechModel(context.Background(), quietLog(), m, dir); err == nil {
			t.Fatal("took an archive with no tokens.txt")
		}
		if m.Installed(dir) {
			t.Error("and said it was installed")
		}
	})

	t.Run("a server that says no leaves nothing behind", func(t *testing.T) {
		dir := t.TempDir()
		s := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			http.Error(w, "gone", http.StatusNotFound)
		}))
		defer s.Close()
		m := SpeechModel{Name: name, Title: "Test", URL: s.URL}
		if err := InstallSpeechModel(context.Background(), quietLog(), m, dir); err == nil {
			t.Fatal("took a 404 for a model")
		}
		entries, _ := os.ReadDir(dir)
		if len(entries) != 0 {
			t.Errorf("left %d things behind", len(entries))
		}
	})

	// Cancel has to leave no model rather than half of one, because half a
	// model looks installed and fails on the first second of audio.
	t.Run("cancelling leaves no model at all", func(t *testing.T) {
		dir := t.TempDir()
		ctx, stop := context.WithCancel(context.Background())
		s := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			w.Header().Set("content-length", strconv.Itoa(len(good)*40))
			w.Write(good)
			stop()
			// Keep sending, so the copy is inside the loop when the stop
			// lands rather than already finished.
			for i := 0; i < 39; i++ {
				if _, err := w.Write(good); err != nil {
					return
				}
			}
		}))
		defer s.Close()
		m := SpeechModel{Name: name, Title: "Test", URL: s.URL}
		err := InstallSpeechModel(ctx, quietLog(), m, dir)
		if err == nil {
			t.Fatal("a cancelled install reported success")
		}
		if m.Installed(dir) {
			t.Error("a cancelled install left a model")
		}
	})
}

// A tar can name anything it likes, including a way out of the folder it is
// being written into. Nothing from a network gets to choose where it lands.
func TestAnArchiveCannotWriteOutsideWhereItIsPut(t *testing.T) {
	if _, err := exec.LookPath("bzip2"); err != nil {
		t.Skip("bzip2 is not installed")
	}
	root := t.TempDir()
	into := filepath.Join(root, "into")
	guard := filepath.Join(root, "guard.txt")
	if err := os.WriteFile(guard, []byte("untouched"), 0o644); err != nil {
		t.Fatal(err)
	}

	for _, name := range []string{
		"../guard.txt",
		"../../guard.txt",
		"a/../../guard.txt",
		"/tmp/framefairy-should-never-exist",
	} {
		t.Run(name, func(t *testing.T) {
			archive := filepath.Join(t.TempDir(), "a.tar.bz2")
			if err := os.WriteFile(archive,
				modelArchive(t, map[string]string{name: "got out"}), 0o644); err != nil {
				t.Fatal(err)
			}
			err := unpackTarBz2(context.Background(), archive, into)
			body, _ := os.ReadFile(guard)
			if string(body) != "untouched" {
				t.Fatalf("%s wrote over a file outside the folder", name)
			}
			if _, statErr := os.Stat("/tmp/framefairy-should-never-exist"); statErr == nil {
				os.Remove("/tmp/framefairy-should-never-exist")
				t.Fatalf("%s wrote an absolute path", name)
			}
			// Refusing is the wanted answer, and silently landing inside
			// the folder would be acceptable too. Writing outside is not,
			// and that is what the two checks above are for.
			_ = err
		})
	}
}

// A link is a way to point at something outside the folder without naming
// it in the header. The models have none, so none is written.
func TestLinksInAnArchiveAreNotWritten(t *testing.T) {
	if _, err := exec.LookPath("bzip2"); err != nil {
		t.Skip("bzip2 is not installed")
	}
	var plain bytes.Buffer
	w := tar.NewWriter(&plain)
	for _, h := range []*tar.Header{
		{Name: "tokens.txt", Mode: 0o644, Size: 2, Typeflag: tar.TypeReg},
		{Name: "escape", Mode: 0o777, Typeflag: tar.TypeSymlink, Linkname: "/etc/passwd"},
		{Name: "escape2", Mode: 0o777, Typeflag: tar.TypeLink, Linkname: "tokens.txt"},
	} {
		if err := w.WriteHeader(h); err != nil {
			t.Fatal(err)
		}
		if h.Typeflag == tar.TypeReg {
			w.Write([]byte("ok"))
		}
	}
	w.Close()
	cmd := exec.Command("bzip2", "-c")
	cmd.Stdin = &plain
	packed, err := cmd.Output()
	if err != nil {
		t.Fatal(err)
	}
	dir := t.TempDir()
	archive := filepath.Join(dir, "a.tar.bz2")
	if err := os.WriteFile(archive, packed, 0o644); err != nil {
		t.Fatal(err)
	}
	into := filepath.Join(dir, "into")
	if err := unpackTarBz2(context.Background(), archive, into); err != nil {
		t.Fatal(err)
	}
	if _, err := os.Lstat(filepath.Join(into, "escape")); err == nil {
		t.Error("a symlink was written")
	}
	if _, err := os.Lstat(filepath.Join(into, "escape2")); err == nil {
		t.Error("a hard link was written")
	}
	if _, err := os.Stat(filepath.Join(into, "tokens.txt")); err != nil {
		t.Error("the ordinary file was thrown away with them")
	}
}

// The list is what the app offers, so it has to be offerable: every
// entry needs the words a person reads before agreeing to a download, and
// exactly one is the one to reach for when nobody has chosen.
func TestEverySpeechModelCanBeOffered(t *testing.T) {
	models := SpeechModels()
	if len(models) == 0 {
		t.Fatal("no speech models at all")
	}
	recommended := 0
	seen := map[string]bool{}
	for _, m := range models {
		if m.Name == "" || m.Title == "" || m.About == "" || m.Languages == "" {
			t.Errorf("%q cannot be shown: %+v", m.Name, m)
		}
		if m.URL == "" || !strings.HasPrefix(m.URL, "https://") {
			t.Errorf("%s is not fetched over https: %q", m.Name, m.URL)
		}
		// Without this the app cannot say what the download costs, and
		// a download nobody agreed to is a download nobody wanted.
		if m.Download <= 0 {
			t.Errorf("%s does not say how big it is", m.Name)
		}
		// Untrusted input. A model with no checksum is a model nothing
		// checks, and it is unpacked straight onto the machine.
		if len(m.SHA256) != 64 {
			t.Errorf("%s has no usable checksum: %q", m.Name, m.SHA256)
		}
		if seen[m.Name] {
			t.Errorf("%s is listed twice", m.Name)
		}
		seen[m.Name] = true
		if m.Recommended {
			recommended++
		}
		if found, ok := SpeechModelByName(m.Name); !ok || found.Name != m.Name {
			t.Errorf("%s cannot be found by name", m.Name)
		}
	}
	if recommended != 1 {
		t.Errorf("%d models are recommended, want exactly 1", recommended)
	}
	if _, ok := SpeechModelByName("no-such-model"); ok {
		t.Error("found a model that does not exist")
	}
}

// The one in the catalogue has to be the one the rest of the app already
// looks for, or the app downloads a model and then says none is installed.
func TestTheRecommendedModelIsTheOneTheAppLooksFor(t *testing.T) {
	var recommended SpeechModel
	for _, m := range SpeechModels() {
		if m.Recommended {
			recommended = m
		}
	}
	if want := filepath.Base(DefaultModelDir()); recommended.Name != want {
		t.Errorf("the catalogue installs %s and the engine opens %s", recommended.Name, want)
	}
}
