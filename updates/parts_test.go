package updates

import (
	"archive/zip"
	"bytes"
	"context"
	"crypto/ed25519"
	"encoding/json"
	"fmt"
	"io"
	"io/fs"
	"math/rand/v2"
	"net/http"
	"net/http/httptest"
	"os"
	"path"
	"path/filepath"
	"strings"
	"sync"
	"testing"
	"testing/fstest"
)

// A file of an app, its name inside Frame Fairy.app and what it holds.
type appFile struct{ name, body string }

// dittoZip is an app zipped the way the build workflow zips one: every
// folder and file deflated, with Unix modes.
func dittoZip(t *testing.T, files ...appFile) []byte {
	t.Helper()
	var buf bytes.Buffer
	w := zip.NewWriter(&buf)
	dirs := map[string]bool{}
	for _, f := range files {
		for d := path.Dir("Frame Fairy.app/" + f.name); d != "."; d = path.Dir(d) {
			dirs[d] = true
		}
	}
	for _, d := range []string{"Frame Fairy.app", "Frame Fairy.app/Contents", "Frame Fairy.app/Contents/MacOS", "Frame Fairy.app/Contents/Frameworks"} {
		if !dirs[d] {
			continue
		}
		h := &zip.FileHeader{Name: d + "/"}
		h.SetMode(os.ModeDir | 0o755)
		if _, err := w.CreateHeader(h); err != nil {
			t.Fatal(err)
		}
	}
	for _, f := range files {
		h := &zip.FileHeader{Name: "Frame Fairy.app/" + f.name, Method: zip.Deflate}
		h.SetMode(0o755)
		fw, err := w.CreateHeader(h)
		if err != nil {
			t.Fatal(err)
		}
		_, _ = fw.Write([]byte(f.body))
	}
	if err := w.Close(); err != nil {
		t.Fatal(err)
	}
	return buf.Bytes()
}

// published is a build as the release holds it once published in parts:
// the zip the build left, and of the stored zip its parts, a piece for
// each of its files and the stored zip packed with zstd. The stored zip
// itself, zip, is never published.
type published struct {
	ditto  []byte
	zip    []byte
	parts  Parts
	pieces map[string][]byte
	packed []byte
}

// packer packs every test's pieces: an encoder at the best level takes
// tens of megabytes to set up.
var packer = NewPacker()

// publish packs a zip the way the publish workflow does.
func publish(t *testing.T, ditto []byte) published {
	t.Helper()
	zr, err := zip.NewReader(bytes.NewReader(ditto), int64(len(ditto)))
	if err != nil {
		t.Fatal(err)
	}
	var stored bytes.Buffer
	if err := Pack(zr, &stored); err != nil {
		t.Fatal(err)
	}
	p, err := Split(bytes.NewReader(stored.Bytes()), int64(stored.Len()))
	if err != nil {
		t.Fatal(err)
	}
	pieces := map[string][]byte{}
	at := int64(0)
	for i, part := range p.Parts {
		if part.Bytes != nil {
			at += int64(len(part.Bytes))
			continue
		}
		piece := packer.Pack(stored.Bytes()[at : at+part.Size])
		pieces[part.SHA256] = piece
		p.Parts[i].Packed = int64(len(piece))
		at += part.Size
	}
	return published{ditto: ditto, zip: stored.Bytes(), parts: p, pieces: pieces, packed: packer.Pack(stored.Bytes())}
}

// releaseOf serves a published build at /dev/app.zip, with its parts and
// pieces beside it, and counts what is asked for.
type releaseOf struct {
	mu     sync.Mutex
	files  map[string][]byte
	asked  map[string]int
	server *httptest.Server
}

func serveParts(t *testing.T, list func() []byte, b published) *releaseOf {
	t.Helper()
	r := &releaseOf{files: map[string][]byte{}, asked: map[string]int{}}
	r.files["/dev/app.zip"] = b.ditto
	r.files["/dev/app.parts.json"] = mustJSON(t, b.parts)
	r.files["/dev/app.zip.zst"] = b.packed
	for sum, piece := range b.pieces {
		r.files["/dev/"+PieceName(sum)] = piece
	}
	r.server = httptest.NewTLSServer(http.HandlerFunc(func(w http.ResponseWriter, req *http.Request) {
		if req.URL.Path == "/dev/channels.json" {
			_, _ = w.Write(list())
			return
		}
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
	t.Cleanup(r.server.Close)
	return r
}

func (r *releaseOf) set(name string, data []byte) {
	r.mu.Lock()
	defer r.mu.Unlock()
	if data == nil {
		delete(r.files, name)
		return
	}
	r.files[name] = data
}

func (r *releaseOf) asks(name string) int {
	r.mu.Lock()
	defer r.mu.Unlock()
	return r.asked[name]
}

// running is the app that runs, as the folder Source.App reads.
func running(files ...appFile) fstest.MapFS {
	app := fstest.MapFS{}
	for _, f := range files {
		app[f.name] = &fstest.MapFile{Data: []byte(f.body), Mode: 0o755}
	}
	return app
}

// The app of every test: a program that changes from build to build, and
// two that do not, as large as a test allows.
var (
	ffmpeg    = appFile{"Contents/MacOS/ffmpeg", strings.Repeat("the libraries of ffmpeg ", 2000)}
	onnx      = appFile{"Contents/Frameworks/libonnxruntime.dylib", strings.Repeat("speech ", 3000)}
	oldApp    = appFile{"Contents/MacOS/framefairy-app", "main, the build before"}
	newApp    = appFile{"Contents/MacOS/framefairy-app", "pull request 30"}
	installed = []appFile{oldApp, ffmpeg, onnx}
)

// partsSource follows pr-30 with a cache, as the app does.
//
// On the real disk: the updater keeps its builds in a folder on the disk.
func partsSource(t *testing.T, r *releaseOf, app fstest.MapFS) (*Source, *[]Pieced) {
	t.Helper()
	var mu sync.Mutex
	told := &[]Pieced{}
	src := &Source{
		URL: r.server.URL + "/dev/channels.json", Own: "main", Client: r.server.Client(),
		Cache: filepath.Join(t.TempDir(), "builds"), Picked: func() string { return "pr-30" },
		Pieced: func(p Pieced) { mu.Lock(); *told = append(*told, p); mu.Unlock() },
	}
	if app != nil {
		src.App = app
	}
	return src, told
}

// listFor is the channel list with the published build as pr-30.
func listFor(t *testing.T, key ed25519.PrivateKey, srv string, b published) []byte {
	t.Helper()
	return listJSON(t, entryFor(t, key, srv, b))
}

// entryFor is the published build's entry as pr-30, signed as the zip the
// build left and as the stored zip.
func entryFor(t *testing.T, key ed25519.PrivateKey, srv string, b published) Build {
	t.Helper()
	entry := build(t, key, "pr-30", "0.3.0-pr30.abc1234", srv+"/dev/app.zip", b.ditto)
	st := build(t, key, "pr-30", "0.3.0-pr30.abc1234", srv+"/dev/app.zip", b.zip)
	entry.Stored = &Stored{Size: st.Size, SHA256: st.SHA256, Signature: st.Signature, Claim: st.Claim, PackedSize: int64(len(b.packed))}
	return entry
}

// An update of a build whose app program alone changed downloads that
// program's piece, takes ffmpeg and the speech library from the app that
// runs, and installs the new app whole.
//
// On the real disk: an update is a program that is downloaded, kept and run.
func TestAnUpdateDownloadsOnlyTheFilesTheAppDoesNotHave(t *testing.T) {
	t.Parallel()
	key := testKey(t)
	b := publish(t, dittoZip(t, newApp, ffmpeg, onnx))
	var list []byte
	r := serveParts(t, func() []byte { return list }, b)
	list = listFor(t, key, r.server.URL, b)
	src, told := partsSource(t, r, running(installed...))
	var progressed, of int64
	var sizes []int64
	src.Progress = func(w, total int64) { progressed, of = w, total; sizes = append(sizes, total) }

	if got := install(t, src, key); got != newApp.body {
		t.Fatalf("the app installed says %q", got)
	}
	appPiece := b.parts.Parts[partOf(t, b.parts, newApp.name)]
	if n := r.asks("/dev/" + PieceName(appPiece.SHA256)); n != 1 {
		t.Errorf("the app program's piece was asked for %d times, want 1", n)
	}
	for _, f := range []appFile{ffmpeg, onnx} {
		p := b.parts.Parts[partOf(t, b.parts, f.name)]
		if n := r.asks("/dev/" + PieceName(p.SHA256)); n != 0 {
			t.Errorf("%s was downloaded %d times though the app has it", f.name, n)
		}
	}
	if n := r.asks("/dev/app.zip"); n != 0 {
		t.Errorf("the whole zip was downloaded %d times", n)
	}
	if len(*told) != 1 || (*told)[0] != (Pieced{Kept: 2, Fetched: 1, Bytes: appPiece.Packed}) {
		t.Errorf("told %+v, want 2 files kept and the app program fetched", *told)
	}
	// The fill is of what was downloaded, and runs to its end.
	if progressed != appPiece.Packed || of != appPiece.Packed {
		t.Errorf("the fill ended at %d of %d, want %d of %d", progressed, of, appPiece.Packed, appPiece.Packed)
	}
	// It says that size from the start, never the size of the whole zip.
	for _, size := range sizes {
		if size != appPiece.Packed {
			t.Errorf("the fill said it downloads %d bytes, want %d", size, appPiece.Packed)
		}
	}
}

// changing is the app with ffmpeg replaced between the look at what the
// app has and the moment its bytes are written.
type changing struct {
	fstest.MapFS
	mu    sync.Mutex
	reads int
}

func (c *changing) ReadFile(name string) ([]byte, error) {
	b, err := c.MapFS.ReadFile(name)
	if name != ffmpeg.name || err != nil {
		return b, err
	}
	c.mu.Lock()
	defer c.mu.Unlock()
	if c.reads++; c.reads > 1 {
		return bytes.ToUpper(b), nil
	}
	return b, nil
}

// A file that changes in the app while the update is put together, an
// app updated by hand underneath it, is downloaded as its piece, and the
// update still does not download the whole zip.
//
// On the real disk: an update is a program that is downloaded, kept and run.
func TestAFileThatChangesInTheAppIsDownloadedAsItsPiece(t *testing.T) {
	t.Parallel()
	key := testKey(t)
	b := publish(t, dittoZip(t, newApp, ffmpeg, onnx))
	var list []byte
	r := serveParts(t, func() []byte { return list }, b)
	list = listFor(t, key, r.server.URL, b)
	src, told := partsSource(t, r, nil)
	src.App = &changing{MapFS: running(installed...)}
	var progressed, of int64
	src.Progress = func(w, total int64) { progressed, of = w, total }

	if got := install(t, src, key); got != newApp.body {
		t.Fatalf("the app installed says %q", got)
	}
	ffmpegPiece := b.parts.Parts[partOf(t, b.parts, ffmpeg.name)]
	if n := r.asks("/dev/" + PieceName(ffmpegPiece.SHA256)); n != 1 {
		t.Errorf("ffmpeg's piece was asked for %d times, want 1", n)
	}
	if n := r.asks("/dev/app.zip"); n != 0 {
		t.Errorf("the whole zip was downloaded %d times", n)
	}
	if len(*told) != 1 || (*told)[0].Kept != 1 || (*told)[0].Fetched != 2 || (*told)[0].Err != nil {
		t.Errorf("told %+v, want the speech library kept and two files fetched", *told)
	}
	want := ffmpegPiece.Packed + b.parts.Parts[partOf(t, b.parts, newApp.name)].Packed
	if progressed != want || of != want {
		t.Errorf("the fill ended at %d of %d, want %d of %d", progressed, of, want, want)
	}
}

// panicking is an app whose files cannot be looked at without a panic,
// the way a piece that trips the decoder would.
type panicking struct{ fstest.MapFS }

func (panicking) Open(string) (fs.File, error)     { panic("a file that cannot be read") }
func (panicking) Stat(string) (fs.FileInfo, error) { panic("a file that cannot be read") }
func (panicking) ReadFile(string) ([]byte, error)  { panic("a file that cannot be read") }

// A panic while the build is put together is that build's error: the app
// downloads the packed zip and says why, and goes on running.
//
// On the real disk: an update is a program that is downloaded, kept and run.
func TestAPanicPuttingTheBuildTogetherDownloadsItWhole(t *testing.T) {
	t.Parallel()
	key := testKey(t)
	b := publish(t, dittoZip(t, newApp, ffmpeg, onnx))
	var list []byte
	r := serveParts(t, func() []byte { return list }, b)
	list = listFor(t, key, r.server.URL, b)
	src, told := partsSource(t, r, nil)
	src.App = panicking{}

	if got := install(t, src, key); got != newApp.body {
		t.Fatalf("the app installed says %q", got)
	}
	if n, m := r.asks("/dev/app.zip.zst"), r.asks("/dev/app.zip"); n != 1 || m != 0 {
		t.Errorf("the packed zip was downloaded %d times and the stored zip %d, want 1 and 0", n, m)
	}
	if len(*told) != 1 || (*told)[0].Packed == 0 || (*told)[0].Err == nil || !strings.Contains((*told)[0].Err.Error(), "panicked") {
		t.Errorf("told %+v, want the panic and the packed zip", *told)
	}
}

// An app with little to take from, one made on the Mac or one whose files
// are not the same, downloads whichever is smaller: the pieces it lacks or
// the packed zip, and still installs.
//
// On the real disk: an update is a program that is downloaded, kept and run.
func TestAnAppWithLittleToTakeDownloadsWhicheverIsSmaller(t *testing.T) {
	t.Parallel()
	// Three programs with the same libraries linked into each, the way
	// ffmpeg's are. A piece packs one of them, and the packed zip reaches
	// across all three, so it is smaller.
	libraries := string(chacha8(4096))
	linked := []appFile{newApp, {"Contents/MacOS/ffmpeg", libraries + "ffmpeg"},
		{"Contents/MacOS/ffprobe", libraries + "ffprobe"}, {"Contents/MacOS/framefairy-frames", libraries + "frames"}}
	for _, piecesSmaller := range []bool{false, true} {
		t.Run(fmt.Sprintf("pieces smaller %v", piecesSmaller), func(t *testing.T) {
			t.Parallel()
			key := testKey(t)
			b := publish(t, dittoZip(t, linked...))
			var pieces int64
			for _, part := range b.parts.Parts {
				pieces += part.Packed
			}
			if int64(len(b.packed)) >= pieces {
				t.Fatalf("the packed zip, %d bytes, is no smaller than the pieces, %d", len(b.packed), pieces)
			}
			var list []byte
			r := serveParts(t, func() []byte { return list }, b)
			entry := entryFor(t, key, r.server.URL, b)
			if piecesSmaller {
				// A packed zip the list says is larger than every piece.
				entry.Stored.PackedSize = pieces + 1
			}
			list = listJSON(t, entry)
			src, told := partsSource(t, r, running(oldApp))

			if got := install(t, src, key); got != newApp.body {
				t.Fatalf("the app installed says %q", got)
			}
			want := Pieced{Packed: int64(len(b.packed))}
			if piecesSmaller {
				want = Pieced{Fetched: 4, Bytes: pieces}
			}
			if len(*told) != 1 || (*told)[0] != want {
				t.Errorf("told %+v, want %+v", *told, want)
			}
			if n, m := r.asks("/dev/app.zip.zst"), r.asks("/dev/app.zip"); n != boolInt(!piecesSmaller) || m != 0 {
				t.Errorf("the packed zip was downloaded %d times and the zip %d", n, m)
			}
		})
	}
}

// chacha8 is n bytes that do not pack, the same every run.
func chacha8(n int) []byte {
	b := make([]byte, n)
	_, _ = rand.NewChaCha8([32]byte{}).Read(b)
	return b
}

func boolInt(b bool) int {
	if b {
		return 1
	}
	return 0
}

// Whatever goes wrong with the parts, the update still arrives: the app
// downloads the zip packed with zstd and says why.
//
// On the real disk: an update is a program that is downloaded, kept and run.
func TestPartsThatDoNotWorkDownloadThePackedZip(t *testing.T) {
	t.Parallel()
	cases := []struct {
		name   string
		break_ func(r *releaseOf, b published)
		says   string
	}{
		{"a piece is gone", func(r *releaseOf, b published) {
			r.set("/dev/"+PieceName(b.parts.Parts[partOf(t, b.parts, newApp.name)].SHA256), nil)
		}, "404"},
		{"a piece is another file", func(r *releaseOf, b published) {
			sum := b.parts.Parts[partOf(t, b.parts, newApp.name)].SHA256
			r.set("/dev/"+PieceName(sum), packer.Pack([]byte("pull request 31")))
		}, "piece"},
		{"the parts are gone", func(r *releaseOf, _ published) { r.set("/dev/app.parts.json", nil) }, "parts"},
		{"the parts are of another build", func(r *releaseOf, b published) {
			p := b.parts
			p.SHA256 = strings.Repeat("0", 64)
			r.set("/dev/app.parts.json", mustJSON(t, p))
		}, "another build"},
		{"the parts write other bytes", func(r *releaseOf, b published) {
			p := b.parts
			p.Parts = append([]Part(nil), p.Parts...)
			p.Parts[0].Bytes = bytes.Repeat([]byte{'x'}, len(p.Parts[0].Bytes))
			r.set("/dev/app.parts.json", mustJSON(t, p))
		}, "is not the build"},
	}
	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			t.Parallel()
			key := testKey(t)
			b := publish(t, dittoZip(t, newApp, ffmpeg, onnx))
			var list []byte
			r := serveParts(t, func() []byte { return list }, b)
			list = listFor(t, key, r.server.URL, b)
			c.break_(r, b)
			src, told := partsSource(t, r, running(installed...))

			if got := install(t, src, key); got != newApp.body {
				t.Fatalf("the app installed says %q", got)
			}
			if n, m := r.asks("/dev/app.zip.zst"), r.asks("/dev/app.zip"); n != 1 || m != 0 {
				t.Errorf("the packed zip was downloaded %d times and the zip %d, want 1 and 0", n, m)
			}
			if len(*told) != 1 || (*told)[0].Err == nil || (*told)[0].Packed == 0 {
				t.Fatalf("told %+v, want why the parts did not work, and the packed zip", *told)
			}
			if c.says != "is not the build" && !strings.Contains((*told)[0].Err.Error(), c.says) {
				t.Errorf("told %q, want it to say %q", (*told)[0].Err, c.says)
			}
		})
	}
}

// When the packed zip does not work either, the update fails and says
// why. The zip at the build's address is the deflated one, for the apps
// from before parts, which is not the zip this app installs.
//
// On the real disk: an update is a program that is downloaded and kept.
func TestAnUpdateWhosePartsAndPackedZipFailSaysWhy(t *testing.T) {
	t.Parallel()
	other := packer.Pack(publish(t, dittoZip(t, oldApp, ffmpeg, onnx)).zip)
	cases := map[string]func(b published) []byte{
		"it is gone":             func(published) []byte { return nil },
		"it is not zstd":         func(b published) []byte { return bytes.Repeat([]byte{'x'}, len(b.packed)) },
		"it is cut short":        func(b published) []byte { return b.packed[:len(b.packed)/2] },
		"it is of another build": func(published) []byte { return other },
	}
	for name, packed := range cases {
		t.Run(name, func(t *testing.T) {
			t.Parallel()
			key := testKey(t)
			b := publish(t, dittoZip(t, newApp, ffmpeg, onnx))
			var list []byte
			r := serveParts(t, func() []byte { return list }, b)
			r.set("/dev/app.parts.json", nil)
			r.set("/dev/app.zip.zst", packed(b))
			// The list says the packed zip's size as it is served, so it
			// is what it holds that fails, not its size.
			if served := packed(b); served != nil {
				b.packed = served
			}
			list = listFor(t, key, r.server.URL, b)
			src, told := partsSource(t, r, running(installed...))
			u := newUpdater(t, src, "0.3.0-main.4", key)
			if rel, err := u.Check(context.Background()); err != nil || rel == nil {
				t.Fatalf("check: %v, %v", rel, err)
			}

			if err := u.DownloadAndInstall(context.Background()); err == nil {
				t.Fatal("a build that could not be had was installed")
			}
			if n := r.asks("/dev/app.zip"); n != 0 {
				t.Errorf("the deflated zip was downloaded %d times", n)
			}
			if len(*told) != 1 || (*told)[0].Packed != 0 || (*told)[0].Err == nil ||
				!strings.Contains((*told)[0].Err.Error(), "parts") || !strings.Contains((*told)[0].Err.Error(), "packed zip") {
				t.Errorf("told %+v, want why the parts and the packed zip did not work", *told)
			}
		})
	}
}

// A panic while the packed zip is unpacked is that download's error.
func TestAPanicUnpackingThePackedZipIsItsError(t *testing.T) {
	t.Parallel()
	b := publish(t, dittoZip(t, newApp, ffmpeg))
	err := unpackInto(bytes.NewReader(b.packed), int64(len(b.zip)), panicSink{})
	if err == nil || !strings.Contains(err.Error(), "panicked") {
		t.Errorf("unpacking into a sink that panics said %v", err)
	}
}

type panicSink struct{}

func (panicSink) write([]byte) error { panic("a disk that cannot be written") }

// A build published whole, as before parts, downloads whole and says
// nothing went wrong.
//
// On the real disk: an update is a program that is downloaded, kept and run.
func TestABuildWithoutPartsDownloadsWhole(t *testing.T) {
	t.Parallel()
	key := testKey(t)
	b := publish(t, dittoZip(t, newApp, ffmpeg))
	var list []byte
	r := serveParts(t, func() []byte { return list }, b)
	list = listJSON(t, build(t, key, "pr-30", "0.3.0-pr30.abc1234", r.server.URL+"/dev/app.zip", b.ditto))
	src, told := partsSource(t, r, running(installed...))

	if got := install(t, src, key); got != newApp.body {
		t.Fatalf("the app installed says %q", got)
	}
	if n := r.asks("/dev/app.zip"); n != 1 {
		t.Errorf("the whole zip was downloaded %d times, want 1", n)
	}
	if len(*told) != 0 {
		t.Errorf("told %+v about a build that has no parts", *told)
	}
}

// Packing keeps every file with its name, its bytes and its mode, and the
// parts put the packed zip together again byte for byte.
func TestPackKeepsTheAppAndThePartsMakeTheZip(t *testing.T) {
	t.Parallel()
	b := publish(t, dittoZip(t, newApp, ffmpeg, onnx))
	zr, err := zip.NewReader(bytes.NewReader(b.zip), int64(len(b.zip)))
	if err != nil {
		t.Fatal(err)
	}
	want := map[string]string{
		"Frame Fairy.app/":                     "",
		"Frame Fairy.app/Contents/":            "",
		"Frame Fairy.app/Contents/MacOS/":      "",
		"Frame Fairy.app/Contents/Frameworks/": "",
		"Frame Fairy.app/" + newApp.name:       newApp.body,
		"Frame Fairy.app/" + ffmpeg.name:       ffmpeg.body,
		"Frame Fairy.app/" + onnx.name:         onnx.body,
	}
	for _, f := range zr.File {
		body, ok := want[f.Name]
		if !ok {
			t.Errorf("%s is in the zip and was not in the app", f.Name)
			continue
		}
		delete(want, f.Name)
		if f.Method != zip.Store {
			t.Errorf("%s is not stored as it is", f.Name)
		}
		if mode := f.Mode().Perm(); mode != 0o755 {
			t.Errorf("%s has the mode %o, want 755", f.Name, mode)
		}
		rc, err := f.Open()
		if err != nil {
			t.Fatal(err)
		}
		got, _ := io.ReadAll(rc)
		rc.Close()
		if string(got) != body {
			t.Errorf("%s holds %d bytes, want %d", f.Name, len(got), len(body))
		}
	}
	for name := range want {
		t.Errorf("%s went missing", name)
	}
	var again bytes.Buffer
	files := map[string]string{newApp.name: newApp.body, ffmpeg.name: ffmpeg.body, onnx.name: onnx.body}
	for _, p := range b.parts.Parts {
		if p.Bytes != nil {
			again.Write(p.Bytes)
		} else {
			again.WriteString(files[strings.TrimPrefix(p.Name, "Frame Fairy.app/")])
		}
	}
	if !bytes.Equal(again.Bytes(), b.zip) {
		t.Error("the parts do not make the zip they were split from")
	}
	if _, err := ParseParts(mustJSON(t, b.parts), int64(len(b.zip)), b.parts.SHA256); err != nil {
		t.Errorf("the parts of a packed zip are refused: %v", err)
	}
}

// Parts are read from a file anybody on the way could change, and a part
// that names a file outside the app or makes another size is refused.
func TestParsePartsRefusesWhatCannotBeTheBuild(t *testing.T) {
	t.Parallel()
	sum := strings.Repeat("a", 64)
	file := func(name string, size int64) Part {
		return Part{Name: name, Size: size, SHA256: strings.Repeat("b", 64), Packed: 3}
	}
	ok := Parts{Size: 14, SHA256: sum, Parts: []Part{{Bytes: []byte("head")}, file("Frame Fairy.app/Contents/MacOS/x", 10)}}
	if _, err := ParseParts(mustJSON(t, ok), 14, sum); err != nil {
		t.Fatalf("good parts refused: %v", err)
	}
	cases := map[string]Parts{
		"out of the app":   {Size: 14, SHA256: sum, Parts: []Part{{Bytes: []byte("head")}, file("Frame Fairy.app/Contents/../../x", 10)}},
		"out of Contents":  {Size: 14, SHA256: sum, Parts: []Part{{Bytes: []byte("head")}, file("Frame Fairy.app/x", 10)}},
		"from the root":    {Size: 14, SHA256: sum, Parts: []Part{{Bytes: []byte("head")}, file("/Contents/x", 10)}},
		"no app at all":    {Size: 14, SHA256: sum, Parts: []Part{{Bytes: []byte("head")}, file("Contents/MacOS/x", 10)}},
		"too short":        {Size: 14, SHA256: sum, Parts: []Part{{Bytes: []byte("head")}, file("Frame Fairy.app/Contents/x", 9)}},
		"too long":         {Size: 14, SHA256: sum, Parts: []Part{{Bytes: []byte("head")}, file("Frame Fairy.app/Contents/x", 11)}},
		"bytes and a file": {Size: 14, SHA256: sum, Parts: []Part{{Bytes: []byte("head"), Size: 1}, file("Frame Fairy.app/Contents/x", 9)}},
		"empty bytes":      {Size: 14, SHA256: sum, Parts: []Part{{Bytes: []byte{}}, file("Frame Fairy.app/Contents/x", 14)}},
		"no checksum":      {Size: 14, SHA256: sum, Parts: []Part{{Bytes: []byte("head")}, {Name: "Frame Fairy.app/Contents/x", Size: 10, Packed: 3}}},
		"no piece":         {Size: 14, SHA256: sum, Parts: []Part{{Bytes: []byte("head")}, {Name: "Frame Fairy.app/Contents/x", Size: 10, SHA256: strings.Repeat("b", 64)}}},
		"a piece too big":  {Size: 14, SHA256: sum, Parts: []Part{{Bytes: []byte("head")}, {Name: "Frame Fairy.app/Contents/x", Size: 10, SHA256: strings.Repeat("b", 64), Packed: 1 << 30}}},
		"another build":    {Size: 14, SHA256: strings.Repeat("c", 64), Parts: ok.Parts},
		"another size":     {Size: 15, SHA256: sum, Parts: ok.Parts},
	}
	for name, p := range cases {
		if _, err := ParseParts(mustJSON(t, p), 14, sum); err == nil {
			t.Errorf("%s: parts taken", name)
		}
	}
	if _, err := ParseParts([]byte("{"), 14, sum); err == nil {
		t.Error("parts that are not JSON taken")
	}
}

// FuzzParseParts reads the parts of a build, which come from the network.
// Whatever it takes writes exactly the build's size from inside the app.
func FuzzParseParts(f *testing.F) {
	sum := strings.Repeat("a", 64)
	seed, _ := json.Marshal(Parts{Size: 14, SHA256: sum, Parts: []Part{
		{Bytes: []byte("head")},
		{Name: "Frame Fairy.app/Contents/MacOS/x", Size: 10, SHA256: strings.Repeat("b", 64), Packed: 3},
	}})
	f.Add(seed, int64(14))
	f.Add([]byte(`{"size":1,"sha256":"`+sum+`","parts":[{"bytes":"eA=="}]}`), int64(1))
	f.Fuzz(func(t *testing.T, data []byte, size int64) {
		p, err := ParseParts(data, size, sum)
		if err != nil {
			return
		}
		var whole int64
		for _, part := range p.Parts {
			if part.Bytes != nil {
				whole += int64(len(part.Bytes))
				continue
			}
			whole += part.Size
			if rest, ok := inApp(part.Name); !ok || !filepath.IsLocal(rest) {
				t.Fatalf("a part names %q", part.Name)
			}
		}
		if whole != size {
			t.Fatalf("parts of %d bytes taken for a build of %d", whole, size)
		}
	})
}

// partOf is the index of the part of an app's file.
func partOf(t *testing.T, p Parts, name string) int {
	t.Helper()
	for i, part := range p.Parts {
		if part.Name == "Frame Fairy.app/"+name {
			return i
		}
	}
	t.Fatalf("no part of %s", name)
	return -1
}
