package main

import (
	"archive/zip"
	"cmp"
	"flag"
	"fmt"
	"io"
	"net/http"
	"os"
	"path/filepath"
	"runtime"
	"slices"
	"strings"
	"sync"
	"time"

	"framefairy/updates"
)

// The zip the build workflow leaves deflates every file, so a file's bytes
// cannot be taken out of it and a change to one file is a new zip of 67
// MB. pack writes it again as the zip an app that knows parts updates to,
// each file stored as it is, with its parts, a piece for every file and
// the stored zip packed with zstd, so the app downloads only the files it
// does not have. The stored zip itself is signed and never uploaded: an
// app puts it together or unpacks it. The deflated zip stays where it was
// for the apps from before. See updates/parts.go and
// docs/decisions/2026-11-18-an-update-downloads-only-what-changed.md.

// storedZip, partsFile, the zip packed and the pieces are what pack leaves
// in its folder.
const (
	storedZip = "app.zip"
	partsFile = "parts.json"
)

// packedWhole is the stored zip packed with zstd, which the app downloads
// when the parts do not work, see updates/whole.go.
var packedWhole = updates.PackedName(storedZip)

func packCommand(args []string) error {
	fs := flag.NewFlagSet("pack", flag.ExitOnError)
	zipPath := fs.String("zip", "", "the zip the build left")
	out := fs.String("out", "", "the folder the packed build goes in")
	names := fs.String("bases", "", "the parts files of the builds to patch against, by name, the first first, see planBases")
	room := fs.Int("room", 0, "how many patches the build may have at most")
	from := fs.String("from", updates.EntriesURL, "the folder of the release the bases are read from")
	_ = fs.Parse(args)
	b := readBases(strings.Split(*names, ","), *from, &http.Client{}, *room, os.Stdout)
	b.until = time.Now().Add(patchTime)
	return pack(*zipPath, *out, updates.NewPacker(), b, os.Stdout)
}

// pack writes the build in zipPath to out as the stored zip, its parts,
// its pieces, its patches against the bases and the stored zip packed,
// with packer, and says what an update of it weighs.
func pack(zipPath, out string, packer updates.Packer, b bases, log io.Writer) error {
	src, err := zip.OpenReader(zipPath)
	if err != nil {
		return err
	}
	defer src.Close()
	if err := os.MkdirAll(out, 0o755); err != nil {
		return err
	}
	stored, err := os.Create(filepath.Join(out, storedZip))
	if err != nil {
		return err
	}
	defer stored.Close()
	if err := updates.Pack(&src.Reader, stored); err != nil {
		return err
	}
	size, err := stored.Seek(0, io.SeekEnd)
	if err != nil {
		return err
	}
	p, err := updates.Split(stored, size)
	if err != nil {
		return err
	}
	// Each file is packed once, however many parts name it, and the files
	// are packed side by side: zstd at its best level packs one file on
	// one core, and the build waits on the publish.
	type job struct {
		part   int
		at     int64
		packed int64
		err    error
	}
	var jobs []*job
	first := map[string]*job{}
	at := int64(0)
	for i, part := range p.Parts {
		if part.Bytes != nil {
			at += int64(len(part.Bytes))
			continue
		}
		if first[part.SHA256] == nil {
			j := &job{part: i, at: at}
			first[part.SHA256] = j
			jobs = append(jobs, j)
		}
		at += part.Size
	}
	work := make(chan *job)
	var wg sync.WaitGroup
	// The whole zip packed is the longest job by far, so it starts first,
	// beside the pieces.
	var wholeSize int64
	whole := make(chan error, 1)
	wg.Go(func() {
		var err error
		wholeSize, err = packWhole(packer, stored, size, out)
		whole <- err
	})
	for range runtime.NumCPU() {
		wg.Go(func() {
			for j := range work {
				j.packed, j.err = packPiece(packer, stored, p.Parts[j.part], j.at, out)
			}
		})
	}
	// The largest first, so the last to finish is a small one.
	for _, j := range slices.SortedFunc(slices.Values(jobs), func(a, b *job) int {
		return cmp.Compare(p.Parts[b.part].Size, p.Parts[a.part].Size)
	}) {
		work <- j
	}
	close(work)
	wg.Wait()
	if err := <-whole; err != nil {
		return err
	}
	fmt.Fprintf(log, "%-64s %12s %12s\n", "file", "bytes", "packed")
	var pieces int64
	for _, j := range jobs {
		if j.err != nil {
			return j.err
		}
		part := p.Parts[j.part]
		pieces += j.packed
		fmt.Fprintf(log, "%-64s %12d %12d\n", part.Name, part.Size, j.packed)
	}
	for i, part := range p.Parts {
		if part.Bytes == nil {
			p.Parts[i].Packed = first[part.SHA256].packed
		}
	}
	fmt.Fprintf(log, "the stored zip is %d bytes, %d packed, and an app with none of its files downloads %d in pieces\n", size, wholeSize, pieces)
	if len(b.parts) > 0 {
		addPatches(&p, stored, b, out, log)
		logPatches(p, b, log)
	}
	return writeJSON(filepath.Join(out, partsFile), p)
}

// packWhole writes the stored zip, size bytes, packed with zstd to out,
// and says what it weighs.
func packWhole(packer updates.Packer, stored io.ReaderAt, size int64, out string) (int64, error) {
	data := make([]byte, size)
	if _, err := stored.ReadAt(data, 0); err != nil {
		return 0, err
	}
	packed := packer.Pack(data)
	return int64(len(packed)), os.WriteFile(filepath.Join(out, packedWhole), packed, 0o644)
}

// packPiece writes the piece of the file a part names, which starts at at
// in the stored zip, to out, and says what it weighs.
func packPiece(packer updates.Packer, stored io.ReaderAt, part updates.Part, at int64, out string) (int64, error) {
	data := make([]byte, part.Size)
	if _, err := stored.ReadAt(data, at); err != nil {
		return 0, err
	}
	piece := packer.Pack(data)
	if err := os.WriteFile(filepath.Join(out, updates.PieceName(part.SHA256)), piece, 0o644); err != nil {
		return 0, err
	}
	return int64(len(piece)), nil
}
