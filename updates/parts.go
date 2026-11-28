package updates

import (
	"archive/zip"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"path"
	"runtime"
	"strings"

	"github.com/klauspost/compress/zstd"
)

// An update downloads only what the app does not have yet. The zip a build
// is published as stores its files as they are, deflating none, so each
// file's bytes lie in it unchanged. Beside it the build publishes its
// parts: the zip in order, as the few bytes of zip headers written out and
// each file named by its SHA-256. Every file is published once more on its
// own, packed with zstd, as a piece named by that SHA-256, and a build
// whose file is there already publishes nothing for it.
//
// The app writes the parts in order, a file it has from its own folder
// when the bytes are the same and a downloaded piece when not, and hands
// the result to Wails' updater, which checks it against the signature as
// it checks any zip. The parts and the pieces are untrusted, like the list:
// a wrong one makes a zip whose hash fails, and the app then downloads the
// zip whole. See docs/decisions/2026-11-18-an-update-downloads-only-what-changed.md.

// Parts is a build's zip as the parts it is made of.
type Parts struct {
	// Size and SHA256 are the whole zip's, so parts are known to be of
	// the build they are read for.
	Size   int64  `json:"size"`
	SHA256 string `json:"sha256"`
	Parts  []Part `json:"parts"`
}

// Part is either bytes of the zip written out, or a file of the app.
type Part struct {
	// Bytes are the zip's own: headers, and the list of files at its end.
	Bytes []byte `json:"bytes,omitempty"`
	// Name is where the file is in the zip, Frame Fairy.app/Contents/...,
	// which says where the running app may have the same file.
	Name   string `json:"name,omitempty"`
	Size   int64  `json:"size,omitempty"`
	SHA256 string `json:"sha256,omitempty"`
	// Packed is the size of its piece, the file packed with zstd.
	Packed int64 `json:"packed,omitempty"`
}

// MaxParts is the most parts a build may have, room for thousands of
// files where the app has a dozen.
const MaxParts = 10000

// maxWritten is the most of a zip the parts may write out themselves:
// headers of about a hundred bytes a file.
const maxWritten = 4 << 20

// PieceName is the name a file is published under, packed.
func PieceName(sum string) string { return "piece-" + sum + ".zst" }

// PartsName is the name a zip's parts are published under, beside it.
func PartsName(zipName string) string { return strings.TrimSuffix(zipName, ".zip") + ".parts.json" }

// PartsURL is where the parts of the zip at this address are.
func PartsURL(zipURL string) string { return PartsName(zipURL) }

// ParseParts reads the parts of the zip with this size and SHA-256, and
// refuses parts that cannot make it.
func ParseParts(data []byte, size int64, sum string) (Parts, error) {
	if len(data) > MaxList {
		return Parts{}, errors.New("the parts are too large")
	}
	var p Parts
	if err := json.Unmarshal(data, &p); err != nil {
		return Parts{}, fmt.Errorf("the parts are not JSON: %w", err)
	}
	if p.Size != size || p.SHA256 != sum {
		return Parts{}, errors.New("the parts are of another build")
	}
	if len(p.Parts) > MaxParts {
		return Parts{}, fmt.Errorf("the build has %d parts", len(p.Parts))
	}
	var whole, written int64
	for i, part := range p.Parts {
		if err := part.check(); err != nil {
			return Parts{}, fmt.Errorf("part %d: %w", i, err)
		}
		if part.Bytes != nil {
			written += int64(len(part.Bytes))
			whole += int64(len(part.Bytes))
		} else {
			whole += part.Size
		}
		if written > maxWritten || whole > size {
			return Parts{}, errors.New("the parts make more than the build")
		}
	}
	if whole != size {
		return Parts{}, errors.New("the parts make less than the build")
	}
	return p, nil
}

func (part Part) check() error {
	if part.Bytes != nil {
		if len(part.Bytes) == 0 || part.Name != "" || part.Size != 0 || part.SHA256 != "" || part.Packed != 0 {
			return errors.New("it is bytes and a file at once")
		}
		return nil
	}
	if !sha256Pattern.MatchString(part.SHA256) {
		return errors.New("it has no checksum")
	}
	if part.Size <= 0 || part.Size > MaxBuild || part.Packed <= 0 || part.Packed > packedBound(part.Size) {
		return errors.New("its sizes are not sizes")
	}
	if _, ok := InApp(part.Name); !ok {
		return fmt.Errorf("%q is no file of an app", part.Name)
	}
	return nil
}

// InApp is where a file of the zip is inside the app, the name without
// the app's own folder, or false when the name is not one.
func InApp(name string) (string, bool) {
	top, rest, ok := strings.Cut(name, "/")
	if !ok || !strings.HasSuffix(top, ".app") || rest == "" || len(name) > 1024 ||
		path.Clean(rest) != rest || !strings.HasPrefix(rest, "Contents/") ||
		strings.ContainsAny(name, "\\\x00") || strings.Contains(rest, "../") {
		return "", false
	}
	return rest, true
}

// Pack writes the app in src again as the zip a build is published as,
// each file stored as it is. The names, the times and the modes stay.
func Pack(src *zip.Reader, dst io.Writer) error {
	w := zip.NewWriter(dst)
	for _, f := range src.File {
		h := f.FileHeader
		h.Method = zip.Store
		h.CompressedSize64 = h.UncompressedSize64
		// The sizes and the checksum are known, so they go in the header
		// and nothing follows the file: its bytes end where the next
		// header starts.
		h.Flags &^= 0x8
		fw, err := w.CreateRaw(&h)
		if err != nil {
			return fmt.Errorf("%s: %w", f.Name, err)
		}
		if h.UncompressedSize64 == 0 {
			continue
		}
		rc, err := f.Open()
		if err != nil {
			return fmt.Errorf("%s: %w", f.Name, err)
		}
		_, err = io.Copy(fw, rc)
		if cerr := rc.Close(); err == nil {
			err = cerr
		}
		if err != nil {
			return fmt.Errorf("%s: %w", f.Name, err)
		}
	}
	return w.Close()
}

// Split says what parts a zip Pack wrote is made of. The pieces' sizes
// are left for whoever packs them to fill in.
func Split(r io.ReaderAt, size int64) (Parts, error) {
	zr, err := zip.NewReader(r, size)
	if err != nil {
		return Parts{}, err
	}
	h := sha256.New()
	if _, err := io.Copy(h, io.NewSectionReader(r, 0, size)); err != nil {
		return Parts{}, err
	}
	p := Parts{Size: size, SHA256: hex.EncodeToString(h.Sum(nil))}
	var at int64
	written := func(to int64) error {
		if to == at {
			return nil
		}
		b := make([]byte, to-at)
		if _, err := r.ReadAt(b, at); err != nil {
			return err
		}
		p.Parts = append(p.Parts, Part{Bytes: b})
		at = to
		return nil
	}
	for _, f := range zr.File {
		if f.UncompressedSize64 == 0 {
			continue
		}
		if f.Method != zip.Store {
			return Parts{}, fmt.Errorf("%s is not stored as it is", f.Name)
		}
		if _, ok := InApp(f.Name); !ok {
			return Parts{}, fmt.Errorf("%q is no file of an app", f.Name)
		}
		off, err := f.DataOffset()
		if err != nil {
			return Parts{}, err
		}
		if off < at {
			return Parts{}, fmt.Errorf("%s lies over the file before it", f.Name)
		}
		if err := written(off); err != nil {
			return Parts{}, err
		}
		n := int64(f.UncompressedSize64)
		fh := sha256.New()
		if _, err := io.Copy(fh, io.NewSectionReader(r, off, n)); err != nil {
			return Parts{}, err
		}
		p.Parts = append(p.Parts, Part{Name: f.Name, Size: n, SHA256: hex.EncodeToString(fh.Sum(nil))})
		at = off + n
	}
	if err := written(size); err != nil {
		return Parts{}, err
	}
	return p, nil
}

// Packer packs files as pieces, and a build's zip as its packed zip. An
// encoder at zstd's best level takes tens of megabytes to set up, so one
// packer packs every file of a build, several at a time.
type Packer struct{ enc *zstd.Encoder }

// NewPacker is a packer that packs as many files at a time as the machine
// has cores.
func NewPacker() Packer {
	enc, err := zstd.NewWriter(nil, zstd.WithEncoderLevel(zstd.SpeedBestCompression),
		zstd.WithWindowSize(pieceWindow), zstd.WithEncoderConcurrency(runtime.NumCPU()))
	if err != nil {
		// The options are constants, so this is a mistake in them.
		panic(err)
	}
	return Packer{enc}
}

// Pack packs a file as its piece, or a zip as its packed zip, which
// unpacks in the same memory.
func (p Packer) Pack(data []byte) []byte { return p.enc.EncodeAll(data, nil) }

// packedBound is the most a file of size bytes takes as its piece: what
// zstd gives a file it cannot make smaller, its bytes with a header for
// each block of them, and room to spare. A piece the parts say is bigger
// is no piece, and nothing that big is held in memory for it.
func packedBound(size int64) int64 {
	return size + size>>8 + 1<<10
}

// pieceWindow is how far back a piece may refer while it is unpacked, the
// most memory one takes beside the file itself.
const pieceWindow = 64 << 20

// unpackPiece is the file a piece holds, when it is the file the part
// names.
func unpackPiece(packed []byte, part Part) ([]byte, error) {
	dec, err := zstd.NewReader(nil, zstd.WithDecoderMaxWindow(pieceWindow),
		zstd.WithDecoderMaxMemory(uint64(part.Size)+pieceWindow), zstd.WithDecoderConcurrency(1))
	if err != nil {
		return nil, err
	}
	defer dec.Close()
	out, err := dec.DecodeAll(packed, make([]byte, 0, part.Size))
	if err != nil {
		return nil, fmt.Errorf("the piece of %s does not unpack: %w", part.Name, err)
	}
	if int64(len(out)) != part.Size || sumOf(out) != part.SHA256 {
		return nil, fmt.Errorf("the piece of %s is another file", part.Name)
	}
	return out, nil
}

func sumOf(b []byte) string {
	s := sha256.Sum256(b)
	return hex.EncodeToString(s[:])
}

// equal says whether a file of the running app holds the bytes a part
// names.
func equal(b []byte, part Part) bool {
	return int64(len(b)) == part.Size && sumOf(b) == part.SHA256
}
