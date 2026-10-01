package updates

import (
	"context"
	"errors"
	"fmt"
	"io"
	"net/http"
	"sync"
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

// fetching is one build on its way into the cache.
type fetching struct {
	total int64
	mu    sync.Mutex
	got   int64
	err   error
	done  chan struct{}
}

func (f *fetching) written() int64 {
	f.mu.Lock()
	defer f.mu.Unlock()
	return f.got
}

// fetchOf is the download of the build with this SHA-256 into the cache,
// the one already on its way or one begun now. Nil when the cache cannot
// be written, and the caller downloads it itself.
func (s *Source) fetchOf(sum, where string, total int64) *fetching {
	s.fetchMu.Lock()
	defer s.fetchMu.Unlock()
	if f := s.fetches[sum]; f != nil {
		return f
	}
	k := s.keep(sum)
	if k == nil {
		return nil
	}
	f := &fetching{total: total, done: make(chan struct{})}
	if s.fetches == nil {
		s.fetches = map[string]*fetching{}
	}
	s.fetches[sum] = f
	go func() {
		ctx, cancel := context.WithTimeout(context.Background(), fetchTakes)
		defer cancel()
		err := s.stream(ctx, where, total, k, func(written int64) {
			f.mu.Lock()
			f.got = written
			f.mu.Unlock()
		})
		if err == nil && !k.whole() {
			err = errors.New("the build did not arrive as the channel list says it is")
		}
		k.done(err == nil)
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
func (s *Source) await(ctx context.Context, f *fetching, sum string, dst io.Writer, report func(written, total int64)) error {
	tick := time.NewTicker(100 * time.Millisecond)
	defer tick.Stop()
	last := int64(-1)
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
			found, err := s.fromCache(sum, f.total, dst, report)
			if !found {
				return errors.New("the build went from the cache as it arrived")
			}
			return err
		case <-tick.C:
			if w := f.written(); w != last {
				last = w
				report(w, f.total)
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
func (s *Source) stream(ctx context.Context, where string, total int64, dst sink, progress func(written int64)) error {
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, where, nil)
	if err != nil {
		return err
	}
	resp, err := s.client().Do(req)
	if err != nil {
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
			if written+int64(n) > total {
				return errors.New("the build is larger than the channel list says")
			}
			if err := dst.write(buf[:n]); err != nil {
				return err
			}
			written += int64(n)
			progress(written)
		}
		if rerr == io.EOF {
			break
		}
		if rerr != nil {
			return fmt.Errorf("the download stopped: %w", rerr)
		}
	}
	if written != total {
		return fmt.Errorf("the download stopped after %d of %d bytes", written, total)
	}
	return nil
}
