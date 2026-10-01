package updates

import (
	"crypto/sha256"
	"encoding/hex"
	"errors"
	"fmt"
	"hash"
	"io"
	"os"
	"path/filepath"
	"strings"
)

// The builds already downloaded are kept, so picking a channel again, after
// a look at another, has its build at once rather than downloading it a
// second time. Each is kept under its SHA-256, the one the channel list
// gives it, in a folder of the app's own among the user's caches.
//
// A kept build is only ever used when its bytes still hash to that SHA-256,
// and what it hands the updater is checked by the updater again, checksum
// and signature, the same as a download. So a file changed on disk is never
// installed: it is removed, and the build is downloaded.
//
// Every time the list is read, whatever it no longer names is removed: the
// build of a pull request that was merged or closed, and a build a newer
// push has replaced. Nothing is kept for a channel that has gone.

// cached is where a build is kept, and where it is written while it
// downloads. Empty when there is no cache or the SHA-256 is not one.
func (s *Source) cached(sum string) (kept, part string) {
	if s.Cache == "" || !sha256Pattern.MatchString(sum) {
		return "", ""
	}
	return filepath.Join(s.Cache, sum+".zip"), filepath.Join(s.Cache, sum+".part")
}

// fromCache hands dst the kept build, when there is one and it is still
// the build it was kept as. False when it has to be downloaded instead, in
// which case nothing has been written.
func (s *Source) fromCache(sum string, size int64, dst io.Writer, report func(written, total int64)) (bool, error) {
	kept, _ := s.cached(sum)
	if kept == "" {
		return false, nil
	}
	info, err := os.Lstat(kept)
	if err != nil {
		return false, nil
	}
	if !info.Mode().IsRegular() || info.Size() != size || hashOf(kept) != sum {
		_ = os.Remove(kept)
		return false, nil
	}
	f, err := os.Open(kept)
	if err != nil {
		return false, nil
	}
	defer f.Close()
	var written int64
	buf := make([]byte, 256<<10)
	for {
		n, rerr := f.Read(buf)
		if n > 0 {
			if _, err := dst.Write(buf[:n]); err != nil {
				return true, err
			}
			written += int64(n)
			report(written, size)
		}
		if rerr == io.EOF {
			break
		}
		if rerr != nil {
			return true, rerr
		}
	}
	if written != size {
		return true, errors.New("the kept build changed while it was read")
	}
	return true, nil
}

func hashOf(path string) string {
	f, err := os.Open(path)
	if err != nil {
		return ""
	}
	defer f.Close()
	h := sha256.New()
	if _, err := io.Copy(h, f); err != nil {
		return ""
	}
	return hex.EncodeToString(h.Sum(nil))
}

// keeper writes a download into the cache as it arrives, and keeps it only
// once all of it is there and it hashes to what the list said. When the
// cache cannot be written at all, there is no keeper and the build is
// downloaded straight to the updater, see fetch.go.
type keeper struct {
	f          *os.File
	h          hash.Hash
	part, kept string
	sum        string
}

func (s *Source) keep(sum string) *keeper {
	kept, part := s.cached(sum)
	if kept == "" {
		return nil
	}
	if err := os.MkdirAll(s.Cache, 0o700); err != nil {
		return nil
	}
	f, err := os.OpenFile(part, os.O_CREATE|os.O_WRONLY|os.O_TRUNC, 0o600)
	if err != nil {
		return nil
	}
	return &keeper{f: f, h: sha256.New(), part: part, kept: kept, sum: sum}
}

func (k *keeper) write(b []byte) error {
	if k == nil || k.f == nil {
		return errors.New("the build could not be written to the cache")
	}
	k.h.Write(b)
	if _, err := k.f.Write(b); err != nil {
		k.drop()
		return fmt.Errorf("the build could not be written to the cache: %w", err)
	}
	return nil
}

// whole says whether what arrived hashes to what the list said.
func (k *keeper) whole() bool {
	return k != nil && k.f != nil && hex.EncodeToString(k.h.Sum(nil)) == k.sum
}

// done keeps the build when the download arrived whole, and drops it when
// it did not.
func (k *keeper) done(whole bool) {
	if k == nil || k.f == nil {
		return
	}
	if !whole || hex.EncodeToString(k.h.Sum(nil)) != k.sum {
		k.drop()
		return
	}
	err := k.f.Close()
	k.f = nil
	if err != nil || os.Rename(k.part, k.kept) != nil {
		_ = os.Remove(k.part)
	}
}

func (k *keeper) drop() {
	_ = k.f.Close()
	k.f = nil
	_ = os.Remove(k.part)
}

// prune removes every kept build the list does not name.
func (s *Source) prune(l List) {
	if s.Cache == "" {
		return
	}
	named := map[string]bool{}
	for _, b := range l.Channels {
		named[b.SHA256] = true
	}
	entries, err := os.ReadDir(s.Cache)
	if err != nil {
		return
	}
	for _, e := range entries {
		sum := strings.TrimSuffix(strings.TrimSuffix(e.Name(), ".zip"), ".part")
		// A build on its way is left to arrive, whatever the list says.
		if !named[sum] && !s.inFlight(sum) {
			_ = os.RemoveAll(filepath.Join(s.Cache, e.Name()))
		}
	}
}
