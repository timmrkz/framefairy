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
	// Kept is how many files came from the running app, Fetched how many
	// were downloaded as pieces, and Bytes what those pieces weighed.
	Kept, Fetched int
	Bytes         int64
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
// write out, each file the running app has with the same bytes, and a
// downloaded piece for every other. It says how far it has come in the
// bytes of the pieces it downloads, and says what it took from where.
// Whatever goes wrong is an error, and the build is then downloaded whole,
// see fetchOf.
//
// It reads bytes nobody has checked yet, the parts and the pieces, so a
// panic in it is the error of this build and not the end of the app,
// which would otherwise end at every launch on the same piece and never
// install the update that fixes it.
//
// With cheaper, the size of the zip packed with zstd, it writes nothing
// when the pieces it would download weigh more.
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
	var of int64
	for i, part := range p.Parts {
		if part.Bytes != nil {
			continue
		}
		if have[i] = s.own(part) != nil; !have[i] {
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
		switch {
		case part.Bytes != nil:
		case b != nil:
			done.Kept++
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

// own is the running app's file a part names, when it holds the same
// bytes, or nil.
func (s *Source) own(part Part) []byte {
	rest, ok := inApp(part.Name)
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
	return unpackPiece(packed.Bytes(), part)
}

type bufferSink struct{ b *bytes.Buffer }

func (bs bufferSink) write(b []byte) error {
	_, err := bs.b.Write(b)
	return err
}
