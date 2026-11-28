package updates

import (
	"bytes"
	"context"
	"errors"
	"fmt"
	"io/fs"
	"path"
	"runtime/debug"
)

// Pieced is how one build was put together from its parts.
type Pieced struct {
	// Kept is how many files came from the running app, Patched how many
	// were made from the running app's file and a downloaded patch, and
	// Fetched how many were downloaded as pieces. Bytes is what the
	// patches and pieces weighed.
	Kept, Patched, Fetched int
	Bytes                  int64
	// PatchErr is why a patch could not make its file, which was then
	// downloaded as its piece.
	PatchErr error
	// Packed is what the zip packed with zstd weighed, when that was
	// downloaded instead: because the pieces the app lacked weighed more,
	// or because the build could not be put together, and then Err says
	// why. Err with no Packed is a build that could not be had either way.
	Packed int64
	Err    error
}

// errNoParts is a build published without parts, which is downloaded
// whole and is no failure.
var errNoParts = errors.New("the build has no parts")

// errPackedIsSmaller is a build whose pieces the app lacks weigh more than
// the zip packed with zstd, which is downloaded instead and is no failure.
// An app with few of the files, one made on the Mac or one after ffmpeg's
// libraries changed, downloads less that way.
var errPackedIsSmaller = errors.New("the packed zip is smaller than the pieces")

func (s *Source) tell(p Pieced) {
	if s.Pieced != nil {
		s.Pieced(p)
	}
}

// assemble writes the build its parts describe to dst: the bytes the parts
// write out, each file the running app has with the same bytes, a file
// made from the running app's own and a downloaded patch where the parts
// have a patch from it, and a downloaded piece for every other. It says
// how far it has come in the bytes it downloads, and says what it took
// from where. Whatever goes wrong is an error, and the build is then
// downloaded whole, see fetchOf, except a patch that cannot make its
// file, which costs that file's piece.
//
// It reads bytes nobody has checked yet, the parts, the patches and the
// pieces, so a panic in it is the error of this build and not the end of
// the app, which would otherwise end at every launch on the same piece and
// never install the update that fixes it.
//
// With cheaper, the size of the zip packed with zstd, it writes nothing
// when what it would download weighs more.
func (s *Source) assemble(ctx context.Context, where, sum string, size, cheaper int64, dst sink, progress func(got, of int64)) (done Pieced, err error) {
	defer func() {
		if caught := recover(); caught != nil {
			done, err = Pieced{}, fmt.Errorf("putting the build together panicked: %v\n%s", caught, debug.Stack())
		}
	}()
	data, err := s.read(ctx, where)
	if err != nil {
		return Pieced{}, fmt.Errorf("the parts: %w", err)
	}
	p, err := ParseParts(data, size, sum)
	if err != nil {
		return Pieced{}, err
	}
	// What the app has is looked at first, so the download is known
	// before it starts and its fill runs from empty to full once. Each
	// file is read again as it is written, and checked again, rather than
	// held: the app is a few hundred megabytes.
	have := make([]bool, len(p.Parts))
	patches := make([]*Patch, len(p.Parts))
	var of int64
	for i, part := range p.Parts {
		if part.Bytes != nil {
			continue
		}
		mine := s.ownFile(part)
		// Each file of the app is hashed once, and the sum says both
		// whether it is the file and which patch starts from it.
		var sum string
		if mine != nil {
			sum = sumOf(mine)
		}
		switch pt, ok := part.patchFrom(sum); {
		case mine != nil && int64(len(mine)) == part.Size && sum == part.SHA256:
			have[i] = true
		case mine != nil && ok:
			patches[i] = &pt
			of += pt.Size
		default:
			of += part.Packed
		}
	}
	if cheaper > 0 && of > cheaper {
		return Pieced{}, errPackedIsSmaller
	}
	var got int64
	progress(0, of)
	for i, part := range p.Parts {
		if err := ctx.Err(); err != nil {
			return Pieced{}, err
		}
		b := part.Bytes
		if b == nil && have[i] {
			// A file that changed in the app since it was looked at is
			// downloaded as its piece, like any file the app does not have.
			if b = s.own(part); b == nil {
				of += part.Packed
				progress(got, of)
			}
		}
		if b == nil && patches[i] != nil {
			pt := *patches[i]
			b, err = s.patched(ctx, where, part, pt, func(n int64) { progress(got+n, of) })
			if err == nil {
				got += pt.Size
				done.Patched++
				done.Bytes += pt.Size
			} else {
				// The patch costs its piece, never the update.
				done.PatchErr = errors.Join(done.PatchErr, err)
				of += part.Packed - pt.Size
				progress(got, of)
			}
		}
		switch {
		case part.Bytes != nil:
		case b != nil && (have[i] || patches[i] != nil):
			if have[i] {
				done.Kept++
			}
		default:
			b, err = s.piece(ctx, where, part, func(n int64) { progress(got+n, of) })
			if err != nil {
				return Pieced{}, err
			}
			got += part.Packed
			done.Fetched++
			done.Bytes += part.Packed
		}
		if err := dst.write(b); err != nil {
			return Pieced{}, err
		}
	}
	progress(got, of)
	return done, nil
}

// ownFile is what the running app holds where a part names a file, when
// it holds a file of a size a build can have there, or nil.
func (s *Source) ownFile(part Part) []byte {
	rest, ok := InApp(part.Name)
	if s.App == nil || !ok {
		return nil
	}
	info, err := fs.Stat(s.App, rest)
	if err != nil || !info.Mode().IsRegular() || info.Size() > MaxBuild {
		return nil
	}
	b, err := fs.ReadFile(s.App, rest)
	if err != nil {
		return nil
	}
	return b
}

// patched makes the file a part names from the running app's own and the
// patch from it, downloaded from beside the parts. The app's file is read
// again and checked against the patch's start, since it may have changed
// since it was looked at, and what the patch makes is checked against the
// part.
func (s *Source) patched(ctx context.Context, parts string, part Part, pt Patch, progress func(int64)) ([]byte, error) {
	old := s.ownFile(part)
	if old == nil || sumOf(old) != pt.From {
		return nil, fmt.Errorf("the app's %s changed before its patch was applied", part.Name)
	}
	where := parts[:len(parts)-len(path.Base(parts))] + PatchName(pt.From, part.SHA256)
	var patch bytes.Buffer
	patch.Grow(int(pt.Size))
	if err := s.stream(ctx, where, pt.Size, bufferSink{&patch}, progress); err != nil {
		return nil, fmt.Errorf("the patch of %s: %w", part.Name, err)
	}
	out, err := ApplyPatch(old, patch.Bytes(), part.Size)
	if err != nil {
		return nil, fmt.Errorf("the patch of %s: %w", part.Name, err)
	}
	if !equal(out, part) {
		return nil, fmt.Errorf("the patch of %s makes another file", part.Name)
	}
	return out, nil
}

// own is the running app's file a part names, when it holds the same
// bytes, or nil.
func (s *Source) own(part Part) []byte {
	rest, ok := InApp(part.Name)
	if s.App == nil || !ok {
		return nil
	}
	info, err := fs.Stat(s.App, rest)
	if err != nil || !info.Mode().IsRegular() || info.Size() != part.Size {
		return nil
	}
	b, err := fs.ReadFile(s.App, rest)
	if err != nil || !equal(b, part) {
		return nil
	}
	return b
}

// piece downloads the piece a part names, from beside its parts, and
// unpacks it.
func (s *Source) piece(ctx context.Context, parts string, part Part, progress func(int64)) ([]byte, error) {
	where := parts[:len(parts)-len(path.Base(parts))] + PieceName(part.SHA256)
	var packed bytes.Buffer
	packed.Grow(int(part.Packed))
	if err := s.stream(ctx, where, part.Packed, bufferSink{&packed}, progress); err != nil {
		return nil, fmt.Errorf("the piece of %s: %w", part.Name, err)
	}
	return UnpackPiece(packed.Bytes(), part)
}

type bufferSink struct{ b *bytes.Buffer }

func (bs bufferSink) write(b []byte) error {
	_, err := bs.b.Write(b)
	return err
}
