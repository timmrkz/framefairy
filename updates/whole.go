package updates

import (
	"context"
	"errors"
	"fmt"
	"io"
	"runtime/debug"

	"github.com/klauspost/compress/zstd"
)

// The zip a build is published as stores its files as they are, so its
// parts can name their bytes, see parts.go, and it is twice the size of a
// zip that deflates them. A build that cannot be put together from its
// parts would download all of it, so beside it the build publishes the
// same zip packed with zstd, a quarter of it, and the app downloads that
// and unpacks it on the way into the cache. Only a build without it, or
// one whose packed zip fails too, downloads the stored zip itself.

// PackedName is the name the zip with this name is published under,
// packed with zstd.
func PackedName(zipName string) string { return zipName + ".zst" }

// PackedURL is where the zip at this address is, packed with zstd.
func PackedURL(zipURL string) string { return PackedName(zipURL) }

// PackZip packs a build's zip the way a piece is packed, so it unpacks in
// the same memory.
func PackZip(zip []byte) []byte { return PackPiece(zip) }

// unpackWhole downloads the zip packed with zstd from where, size bytes,
// and writes it unpacked to dst, total bytes and not one more. It says how
// far it has come in the packed bytes. It reads bytes nobody has checked
// yet, so a panic in the unpacking is its error.
func (s *Source) unpackWhole(ctx context.Context, where string, size, total int64, dst sink, progress func(got int64)) error {
	r, w := io.Pipe()
	unpacked := make(chan error, 1)
	go func() {
		err := unpackInto(r, total, dst)
		// Whatever stopped the unpacking stops the download too.
		r.CloseWithError(errors.Join(err, errors.New("the packed zip stopped unpacking")))
		unpacked <- err
	}()
	err := s.stream(ctx, where, size, writerSink{w}, progress)
	w.CloseWithError(err)
	if uerr := <-unpacked; uerr != nil {
		return uerr
	}
	return err
}

func unpackInto(r io.Reader, total int64, dst sink) (err error) {
	defer func() {
		if caught := recover(); caught != nil {
			err = fmt.Errorf("unpacking the packed zip panicked: %v\n%s", caught, debug.Stack())
		}
	}()
	dec, err := zstd.NewReader(r, zstd.WithDecoderMaxWindow(pieceWindow), zstd.WithDecoderConcurrency(1))
	if err != nil {
		return err
	}
	defer dec.Close()
	buf := make([]byte, 256<<10)
	var written int64
	for {
		n, rerr := dec.Read(buf)
		if n > 0 {
			if written+int64(n) > total {
				return errors.New("the packed zip unpacks to more than the build")
			}
			if err := dst.write(buf[:n]); err != nil {
				return err
			}
			written += int64(n)
		}
		if rerr == io.EOF {
			break
		}
		if rerr != nil {
			return fmt.Errorf("the packed zip does not unpack: %w", rerr)
		}
	}
	if written != total {
		return fmt.Errorf("the packed zip unpacks to %d of %d bytes", written, total)
	}
	return nil
}
