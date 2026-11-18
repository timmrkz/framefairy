package main

import (
	"archive/zip"
	"flag"
	"fmt"
	"io"
	"os"
	"path/filepath"

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
	fmt.Fprintf(log, "%-64s %12s %12s\n", "file", "bytes", "packed")
	packed := map[string]int64{}
	at := int64(0)
	var pieces int64
	for i, part := range p.Parts {
		if part.Bytes != nil {
			at += int64(len(part.Bytes))
			continue
		}
		n, ok := packed[part.SHA256]
		if !ok {
			data := make([]byte, part.Size)
			if _, err := stored.ReadAt(data, at); err != nil {
				return err
			}
			piece := updates.PackPiece(data)
			if err := os.WriteFile(filepath.Join(out, updates.PieceName(part.SHA256)), piece, 0o644); err != nil {
				return err
			}
			n = int64(len(piece))
			packed[part.SHA256] = n
			pieces += n
			fmt.Fprintf(log, "%-64s %12d %12d\n", part.Name, part.Size, n)
		}
		p.Parts[i].Packed = n
		at += part.Size
	}
	fmt.Fprintf(log, "the zip is %d bytes, an app with none of its files downloads %d\n", size, pieces)
	return writeJSON(filepath.Join(out, partsFile), p)
}
