package updates

import (
	"context"
	"errors"
	"fmt"
	"io"
	"net/http"
	"sync"
	"sync/atomic"
	"time"
)

// A download that has begun goes on to the end, into the cache, whoever is
// still waiting for it. Picking another channel while a build downloads
// only stops the wait: the download carries on beside the new channel's,
// and going back finds it further along, or kept and there at once. It
// used to be stopped with the wait, and what it had was thrown away, so
// looking at another channel for a moment cost the whole download again.
//
// Without a cache there is nowhere for a download to go on to, and it is
// what it always was: the download is the wait.

// fetchTakes is the longest a download may take before it is given up.
const fetchTakes = 30 * time.Minute

// fetchQuiet is how long a download may bring nothing before it is given
// up. A connection that went quiet without closing, a laptop that changed
// its Wi-Fi or slept, used to be waited on for all of fetchTakes, and
// every later check waited on it with it. Long enough for a server that
// is slow to begin, the same as a speech model's download, downloadStall
// in the engine.
const fetchQuiet = time.Minute

// fetching is one build on its way into the cache.
type fetching struct {
	// total is the size of the build.
	total int64
	mu    sync.Mutex
	// got is how much has been downloaded of of. A build downloaded whole
	// counts its bytes, one put together from its parts counts the bytes
	// of the pieces it downloads, since the files the app has take no
	// time worth showing.
	got, of int64
	err     error
	done    chan struct{}
}

func (f *fetching) written() (got, of int64) {
	f.mu.Lock()
	defer f.mu.Unlock()
	return f.got, f.of
}

func (f *fetching) downloaded(got, of int64) {
	f.mu.Lock()
	f.got, f.of = got, of
	f.mu.Unlock()
}

// from is where a build can be had: its parts and its zip packed with
// zstd when it was published with them, and otherwise its zip, whole.
type from struct {
	whole, parts, packed string
	packedSize           int64
}

// fetchOf is the download of the build with this SHA-256 into the cache,
// the one already on its way or one begun now. Nil when the cache cannot
// be written, and the caller downloads it itself.
//
// A build with parts is put together from them, see assemble.go. When
// that fails for any reason, or when the pieces it lacks weigh more than
// the zip packed with zstd, it downloads that, see whole.go. So parts that
// are wrong or a piece that is gone cost time and never the update. A
// build without parts is downloaded whole.
func (s *Source) fetchOf(sum string, from from, total int64) *fetching {
	s.fetchMu.Lock()
	defer s.fetchMu.Unlock()
	if f := s.fetches[sum]; f != nil {
		return f
	}
	k := s.keep(sum)
	if k == nil {
		return nil
	}
	// A build put together from its parts knows what it downloads only
	// once it has looked at the app, and until then says nothing: the fill
	// first said the size of the whole zip, and then jumped to a tenth.
	f := &fetching{total: total, of: total, done: make(chan struct{})}
	if from.parts != "" {
		f.of = 0
	}
	if s.fetches == nil {
		s.fetches = map[string]*fetching{}
	}
	s.fetches[sum] = f
	go func() {
		ctx, cancel := context.WithTimeout(context.Background(), fetchTakes)
		defer cancel()
		err := errNoParts
		if from.parts != "" {
			var done Pieced
			cheaper := int64(0)
			if from.packed != "" {
				cheaper = from.packedSize
			}
			done, err = s.assemble(ctx, from.parts, sum, total, cheaper, k, f.downloaded)
			if err == nil && !k.whole() {
				err = errors.New("the build put together from its parts is not the build the list says")
			}
			if err == nil {
				s.tell(done)
			}
		}
		why := err
		if why == errNoParts || why == errPackedIsSmaller {
			why = nil
		}
		if err != nil && from.packed != "" {
			f.downloaded(0, from.packedSize)
			if err = k.restart(); err == nil {
				err = s.unpackWhole(ctx, from.packed, from.packedSize, total, k, func(got int64) { f.downloaded(got, from.packedSize) })
			}
			if err == nil && !k.whole() {
				err = errors.New("the packed zip unpacks to another build than the list says")
			}
			if err != nil {
				why = errors.Join(why, fmt.Errorf("the packed zip: %w", err))
			} else {
				s.tell(Pieced{Packed: from.packedSize, Err: why})
				why = nil
			}
		}
		if why != nil {
			s.tell(Pieced{Err: why})
		}
		if err != nil && from.whole != "" {
			f.downloaded(0, total)
			if err = k.restart(); err == nil {
				err = s.stream(ctx, from.whole, total, k, func(written int64) { f.downloaded(written, total) })
			}
		}
		if err == nil && !k.whole() {
			err = errors.New("the build did not arrive as the channel list says it is")
		}
		if kerr := k.done(err == nil); err == nil {
			err = kerr
		}
		s.fetchMu.Lock()
		delete(s.fetches, sum)
		s.fetchMu.Unlock()
		f.mu.Lock()
		f.err = err
		f.mu.Unlock()
		close(f.done)
	}()
	return f
}

// Arrived is closed once the build with this SHA-256 that is on its way
// into the cache has arrived there, or failed to, whoever still waits for
// it. With none on its way it is closed already. Nothing in the app waits
// for a download nobody waits for, so only the tests ask.
func (s *Source) Arrived(sum string) <-chan struct{} {
	s.fetchMu.Lock()
	defer s.fetchMu.Unlock()
	if f := s.fetches[sum]; f != nil {
		return f.done
	}
	done := make(chan struct{})
	close(done)
	return done
}

// inFlight says whether a build is on its way into the cache, so pruning
// leaves it alone.
func (s *Source) inFlight(sum string) bool {
	s.fetchMu.Lock()
	defer s.fetchMu.Unlock()
	return s.fetches[sum] != nil
}

// await waits for a download into the cache, telling report how far it
// has come, and then hands dst the kept build. A wait called off leaves
// the download going.
//
// Handing the kept build over is not more of the download, so it does not
// count from nothing again: the fill ran back towards empty just before
// the end and then jumped to full (#334). It says only that the whole
// build is there, once it has been handed over.
func (s *Source) await(ctx context.Context, f *fetching, sum string, dst io.Writer, report func(written, total int64)) error {
	tick := time.NewTicker(100 * time.Millisecond)
	defer tick.Stop()
	lastGot, lastOf := int64(-1), int64(-1)
	for {
		select {
		case <-ctx.Done():
			return ctx.Err()
		case <-f.done:
			f.mu.Lock()
			err := f.err
			f.mu.Unlock()
			if err != nil {
				return err
			}
			found, err := s.fromCache(sum, f.total, dst, func(int64, int64) {})
			if !found {
				return errors.New("the build went from the cache as it arrived")
			}
			if err == nil {
				_, of := f.written()
				report(of, of)
			}
			return err
		case <-tick.C:
			if w, of := f.written(); of > 0 && (w != lastGot || of != lastOf) {
				lastGot, lastOf = w, of
				report(w, of)
			}
		}
	}
}

// sink is where a download goes: the cache, or the updater itself.
type sink interface{ write([]byte) error }

type writerSink struct{ w io.Writer }

func (ws writerSink) write(b []byte) error {
	_, err := ws.w.Write(b)
	return err
}

// stream downloads a build to dst, and refuses more bytes than the list
// said there would be, so a server that never stops sending fills nothing.
// It asks the context after every part, because a cancel only reaches the
// transport on its way, and the parts the socket already holds still read:
// a fast machine read the whole build over the loopback before it noticed
// the download had been called off, and called it arrived.
func (s *Source) stream(ctx context.Context, where string, total int64, dst sink, progress func(written int64)) error {
	outer := ctx
	ctx, cancel := context.WithCancel(ctx)
	defer cancel()
	// The same watch as a speech model's download, downloadStall in the
	// engine: each read that brings something starts its wait again.
	var quiet atomic.Bool
	watch := time.AfterFunc(fetchQuiet, func() {
		quiet.Store(true)
		cancel()
	})
	defer watch.Stop()
	stopped := func(err error) error {
		if quiet.Load() && outer.Err() == nil {
			return fmt.Errorf("the download stopped: nothing came for %d seconds", int(fetchQuiet.Seconds()))
		}
		return fmt.Errorf("the download stopped: %w", err)
	}
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, where, nil)
	if err != nil {
		return err
	}
	resp, err := s.client().Do(req)
	if err != nil {
		if quiet.Load() && outer.Err() == nil {
			return stopped(err)
		}
		return fmt.Errorf("the build could not be downloaded: %w", err)
	}
	defer resp.Body.Close()
	if resp.StatusCode != http.StatusOK {
		return fmt.Errorf("the build answered %s", resp.Status)
	}
	body := io.LimitReader(resp.Body, total+1)
	buf := make([]byte, 256<<10)
	var written int64
	for {
		n, rerr := body.Read(buf)
		if n > 0 {
			watch.Reset(fetchQuiet)
			if written+int64(n) > total {
				return errors.New("the build is larger than the channel list says")
			}
			if err := dst.write(buf[:n]); err != nil {
				return err
			}
			written += int64(n)
			progress(written)
		}
		if err := ctx.Err(); err != nil {
			return stopped(err)
		}
		if rerr == io.EOF {
			break
		}
		if rerr != nil {
			return stopped(rerr)
		}
	}
	if written != total {
		return fmt.Errorf("the download stopped after %d of %d bytes", written, total)
	}
	return nil
}
