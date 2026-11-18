package main

import (
	"archive/zip"
	"cmp"
	"flag"
	"fmt"
	"io"
	"os"
	"path/filepath"
	"runtime"
	"slices"
	"sync"

	"framefairy/updates"
)

// The zip the build workflow leaves deflates every file, so a file's bytes
// cannot be taken out of it and a change to one file is a new zip of 67
// MB. pack writes it again as the zip the app updates to, each file stored
// as it is, with its parts and a piece for every file, so the app
// downloads only the files it does not have. See updates/parts.go and
// docs/decisions/2026-11-18-an-update-downloads-only-what-changed.md.

// packedZip, partsFile and the pieces are what pack leaves in its folder.
const (
	packedZip = "app.zip"
	partsFile = "parts.json"
)

func packCommand(args []string) error {
	fs := flag.NewFlagSet("pack", flag.ExitOnError)
	zipPath := fs.String("zip", "", "the zip the build left")
	out := fs.String("out", "", "the folder the packed build goes in")
	_ = fs.Parse(args)
	return pack(*zipPath, *out, os.Stdout)
}

// pack writes the build in zipPath to out as the zip it is published as,
// its parts and its pieces, and says what an update of it weighs.
func pack(zipPath, out string, log io.Writer) error {
	src, err := zip.OpenReader(zipPath)
	if err != nil {
		return err
	}
	defer src.Close()
	if err := os.MkdirAll(out, 0o755); err != nil {
		return err
	}
	stored, err := os.Create(filepath.Join(out, packedZip))
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
	for range runtime.NumCPU() {
		wg.Go(func() {
			for j := range work {
				j.packed, j.err = packPiece(stored, p.Parts[j.part], j.at, out)
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
	fmt.Fprintf(log, "the zip is %d bytes, an app with none of its files downloads %d\n", size, pieces)
	return writeJSON(filepath.Join(out, partsFile), p)
}

// packPiece writes the piece of the file a part names, which starts at at
// in the stored zip, to out, and says what it weighs.
func packPiece(stored io.ReaderAt, part updates.Part, at int64, out string) (int64, error) {
	data := make([]byte, part.Size)
	if _, err := stored.ReadAt(data, at); err != nil {
		return 0, err
	}
	piece := updates.PackPiece(data)
	if err := os.WriteFile(filepath.Join(out, updates.PieceName(part.SHA256)), piece, 0o644); err != nil {
		return 0, err
	}
	return int64(len(piece)), nil
}
