package updates

import (
	"bytes"
	"encoding/binary"
	"strings"
	"testing"

	dbzip2 "github.com/dsnet/compress/bzip2"
	"github.com/gabstv/go-bsdiff/pkg/bsdiff"
)

// A program the way a change of code leaves it: a few bytes in, and every
// byte after them moved along. The two are as large as a test allows and
// as hard to pack as a real program, so a piece of one is large and a
// patch between them is small.
var (
	before = appFile{"Contents/MacOS/framefairy-app", string(chacha8(64 << 10))}
	after  = appFile{"Contents/MacOS/framefairy-app", before.body[:1000] + "a new line of code" + before.body[1000:40000] + "!" + before.body[40001:]}
)

// withPatch is a published build whose program has a patch from the
// program before, served beside it, as pack makes one.
func withPatch(t *testing.T, b published, r *releaseOf) Patch {
	t.Helper()
	patch, err := bsdiff.Bytes([]byte(before.body), []byte(after.body))
	if err != nil {
		t.Fatal(err)
	}
	i := partOf(t, b.parts, after.name)
	part := b.parts.Parts[i]
	if int64(len(patch)) >= part.Packed {
		t.Fatalf("the patch is %d bytes and the piece %d, so the test proves nothing", len(patch), part.Packed)
	}
	pt := Patch{From: sumOf([]byte(before.body)), Size: int64(len(patch))}
	b.parts.Parts[i].Patches = []Patch{pt}
	r.set("/dev/app.parts.json", mustJSON(t, b.parts))
	r.set("/dev/"+PatchName(pt.From, part.SHA256), patch)
	return pt
}

// An update whose program changed by a few bytes downloads the patch from
// the program the app has, not the program's piece, takes the rest from
// the app, and installs the new program whole. The fill says the size of
// the patch from its first report to its last.
//
// On the real disk: an update is a program that is downloaded, kept and run.
func TestAnUpdateDownloadsAPatchOfTheProgramTheAppHas(t *testing.T) {
	t.Parallel()
	key := testKey(t)
	b := publish(t, dittoZip(t, after, ffmpeg, onnx))
	var list []byte
	r := serveParts(t, func() []byte { return list }, b)
	pt := withPatch(t, b, r)
	list = listFor(t, key, r.server.URL, b)
	src, told := partsSource(t, r, running(before, ffmpeg, onnx))
	var sizes []int64
	var progressed int64
	src.Progress = func(w, total int64) { progressed = w; sizes = append(sizes, total) }

	if got := install(t, src, key); got != after.body {
		t.Fatal("the app installed is not the new program")
	}
	part := b.parts.Parts[partOf(t, b.parts, after.name)]
	if n := r.asks("/dev/" + PatchName(pt.From, part.SHA256)); n != 1 {
		t.Errorf("the patch was asked for %d times, want 1", n)
	}
	if n := r.asks("/dev/" + PieceName(part.SHA256)); n != 0 {
		t.Errorf("the program's piece was downloaded %d times though its patch was there", n)
	}
	if len(*told) != 1 || (*told)[0] != (Pieced{Kept: 2, Patched: 1, Bytes: pt.Size}) {
		t.Errorf("told %+v, want 2 files kept and the program patched", *told)
	}
	for _, size := range sizes {
		if size != pt.Size {
			t.Errorf("the fill said it downloads %d bytes, want the patch's %d", size, pt.Size)
		}
	}
	if progressed != pt.Size {
		t.Errorf("the fill ended at %d, want %d", progressed, pt.Size)
	}
}

// A patch that cannot make its file costs that file's piece, never the
// update, and says why.
//
// On the real disk: an update is a program that is downloaded, kept and run.
func TestAPatchThatCannotMakeItsFileCostsThePiece(t *testing.T) {
	t.Parallel()
	for name, spoil := range map[string]func(r *releaseOf, at string){
		"gone":           func(r *releaseOf, at string) { r.set(at, nil) },
		"not bsdiff":     func(r *releaseOf, at string) { r.set(at, []byte(strings.Repeat("x", 64))) },
		"another file's": func(r *releaseOf, at string) { r.set(at, mustDiff(t, before.body, before.body+"more")) },
		"cut short":      func(r *releaseOf, at string) { r.set(at, mustDiff(t, before.body, after.body)[:40]) },
	} {
		t.Run(name, func(t *testing.T) {
			t.Parallel()
			key := testKey(t)
			b := publish(t, dittoZip(t, after, ffmpeg, onnx))
			var list []byte
			r := serveParts(t, func() []byte { return list }, b)
			pt := withPatch(t, b, r)
			part := b.parts.Parts[partOf(t, b.parts, after.name)]
			spoil(r, "/dev/"+PatchName(pt.From, part.SHA256))
			list = listFor(t, key, r.server.URL, b)
			src, told := partsSource(t, r, running(before, ffmpeg, onnx))

			if got := install(t, src, key); got != after.body {
				t.Fatal("the app installed is not the new program")
			}
			if n := r.asks("/dev/" + PieceName(part.SHA256)); n != 1 {
				t.Errorf("the program's piece was downloaded %d times, want 1", n)
			}
			if n := r.asks("/dev/app.zip.zst"); n != 0 {
				t.Errorf("the packed zip was downloaded %d times for one patch", n)
			}
			if len(*told) != 1 || (*told)[0].Fetched != 1 || (*told)[0].Kept != 2 || (*told)[0].PatchErr == nil {
				t.Errorf("told %+v, want the program fetched and why its patch failed", *told)
			}
		})
	}
}

// An app whose program is from none of the builds the patches start from,
// one made on the Mac, downloads the piece.
//
// On the real disk: an update is a program that is downloaded, kept and run.
func TestAnAppWithAnotherProgramDownloadsThePiece(t *testing.T) {
	t.Parallel()
	key := testKey(t)
	b := publish(t, dittoZip(t, after, ffmpeg, onnx))
	var list []byte
	r := serveParts(t, func() []byte { return list }, b)
	pt := withPatch(t, b, r)
	list = listFor(t, key, r.server.URL, b)
	src, told := partsSource(t, r, running(oldApp, ffmpeg, onnx))

	if got := install(t, src, key); got != after.body {
		t.Fatal("the app installed is not the new program")
	}
	part := b.parts.Parts[partOf(t, b.parts, after.name)]
	if n := r.asks("/dev/" + PatchName(pt.From, part.SHA256)); n != 0 {
		t.Errorf("a patch from another program was downloaded %d times", n)
	}
	if len(*told) != 1 || (*told)[0] != (Pieced{Kept: 2, Fetched: 1, Bytes: part.Packed}) {
		t.Errorf("told %+v, want the program fetched as its piece", *told)
	}
}

func mustDiff(t *testing.T, from, to string) []byte {
	t.Helper()
	patch, err := bsdiff.Bytes([]byte(from), []byte(to))
	if err != nil {
		t.Fatal(err)
	}
	return patch
}

// What bsdiff makes, the app's own code applies, for the changes a build
// makes to a program: bytes in, bytes out, bytes changed, and none.
func TestApplyPatchMakesWhatBsdiffMade(t *testing.T) {
	t.Parallel()
	old := before.body
	for name, to := range map[string]string{
		"bytes in":      after.body,
		"bytes out":     old[:500] + old[9000:],
		"bytes changed": old[:30000] + strings.Repeat("z", 100) + old[30100:],
		"none":          old,
		"empty":         "",
		"from nothing":  "a file new in this build",
	} {
		t.Run(name, func(t *testing.T) {
			t.Parallel()
			from := old
			if name == "from nothing" {
				from = ""
			}
			got, err := ApplyPatch([]byte(from), mustDiff(t, from, to), int64(len(to)))
			if err != nil {
				t.Fatal(err)
			}
			if !bytes.Equal(got, []byte(to)) {
				t.Error("the patch made another file")
			}
		})
	}
}

// A patch made wrong, or made to do harm, is refused before it writes a
// byte outside the file it says it makes, or spins.
func TestApplyPatchRefusesWhatCannotBeAPatch(t *testing.T) {
	t.Parallel()
	good := mustDiff(t, before.body, after.body)
	size := int64(len(after.body))
	header := func(ctrl, diff, newSize int64) []byte {
		h := []byte("BSDIFF40")
		for _, v := range []int64{ctrl, diff, newSize} {
			h = binary.LittleEndian.AppendUint64(h, uint64(v))
		}
		return h
	}
	for name, patch := range map[string][]byte{
		"empty":                {},
		"another magic":        append([]byte("BSDIFF41"), good[8:]...),
		"another size":         append(header(offtin(good[8:16]), offtin(good[16:24]), size+1), good[32:]...),
		"blocks past its end":  append(header(1<<40, 0, size), good[32:]...),
		"a negative block":     append(header(-1, 0, size), good[32:]...),
		"no control block":     header(0, 0, size),
		"a control block cut":  append(header(10, 0, size), good[32:42]...),
		"a size past MaxBuild": append(header(0, 0, MaxBuild+1), good[32:]...),
	} {
		t.Run(name, func(t *testing.T) {
			t.Parallel()
			want := size
			if name == "a size past MaxBuild" {
				want = MaxBuild + 1
			}
			if _, err := ApplyPatch([]byte(before.body), patch, want); err == nil {
				t.Error("it was applied")
			}
		})
	}
	// A control block of nothing but steps that write nothing, a few
	// bytes packed, spins for as long as it unpacks unless it is stopped
	// once it has taken more steps than the file has bytes.
	var blocks [3]bytes.Buffer
	for i, raw := range [][]byte{make([]byte, 24<<20), nil, nil} {
		w, err := dbzip2.NewWriter(&blocks[i], nil)
		if err != nil {
			t.Fatal(err)
		}
		_, _ = w.Write(raw)
		if err := w.Close(); err != nil {
			t.Fatal(err)
		}
	}
	spin := header(int64(blocks[0].Len()), int64(blocks[1].Len()), 4)
	for i := range blocks {
		spin = append(spin, blocks[i].Bytes()...)
	}
	if _, err := ApplyPatch(nil, spin, 4); err == nil || !strings.Contains(err.Error(), "steps") {
		t.Errorf("a patch that only spins said %v", err)
	}
}

// The parts refuse patches that cannot be a file's.
func TestParsePartsRefusesPatchesThatCannotBeTheFiles(t *testing.T) {
	t.Parallel()
	b := publish(t, dittoZip(t, after, ffmpeg, onnx))
	i := partOf(t, b.parts, after.name)
	part := b.parts.Parts[i]
	other := sumOf([]byte("another file"))
	for name, patches := range map[string][]Patch{
		"no checksum":         {{From: "abc", Size: 100}},
		"from itself":         {{From: part.SHA256, Size: 100}},
		"twice from one":      {{From: other, Size: 100}, {From: other, Size: 200}},
		"as large as a piece": {{From: other, Size: part.Packed}},
		"empty":               {{From: other, Size: 0}},
		"too many": func() []Patch {
			var ps []Patch
			for n := range MaxPatches + 1 {
				ps = append(ps, Patch{From: sumOf([]byte{byte(n)}), Size: 100})
			}
			return ps
		}(),
	} {
		t.Run(name, func(t *testing.T) {
			t.Parallel()
			p := b.parts
			p.Parts = append([]Part(nil), b.parts.Parts...)
			p.Parts[i].Patches = patches
			if _, err := ParseParts(mustJSON(t, p), p.Size, p.SHA256); err == nil {
				t.Error("the parts were read")
			}
		})
	}
	// Written bytes carry no patches.
	p := b.parts
	p.Parts = append([]Part(nil), b.parts.Parts...)
	p.Parts[0].Patches = []Patch{{From: other, Size: 100}}
	if p.Parts[0].Bytes == nil {
		t.Fatal("the first part is a file")
	}
	if _, err := ParseParts(mustJSON(t, p), p.Size, p.SHA256); err == nil {
		t.Error("bytes with a patch were read")
	}
}

// Whatever bytes a patch holds, applying it ends, makes the size it was
// asked for or an error, and never panics.
func FuzzApplyPatch(f *testing.F) {
	f.Add([]byte("old"), mustDiffF(f, "old file", "new file"), int64(8))
	f.Add([]byte(before.body[:4096]), mustDiffF(f, before.body[:4096], after.body[:4200]), int64(4200))
	f.Add([]byte{}, []byte("BSDIFF40"), int64(0))
	f.Fuzz(func(t *testing.T, old, patch []byte, size int64) {
		if size < 0 || size > 1<<20 {
			return
		}
		got, err := ApplyPatch(old, patch, size)
		if err == nil && int64(len(got)) != size {
			t.Errorf("it made %d bytes, want %d", len(got), size)
		}
	})
}

func mustDiffF(f *testing.F, from, to string) []byte {
	patch, err := bsdiff.Bytes([]byte(from), []byte(to))
	if err != nil {
		f.Fatal(err)
	}
	return patch
}
