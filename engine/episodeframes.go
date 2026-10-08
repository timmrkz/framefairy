package engine

import (
	"bufio"
	"context"
	"errors"
	"fmt"
	"io"
	"os/exec"
	"sync"
	"time"

	"framefairy/internal/framewire"
)

// EpisodeFrames is the episode's decoder as the Go side sees it: one
// framefairy-frames for one episode, started on the first stream asked of
// it and kept while streams are asked, with the file open and decoders
// ready. A stream takes a cursor of it, and a cursor a stream has finished
// with is kept for the next, so a jump moves a decoder that is already
// open instead of starting a program and reading the file's index again.
// See docs/VIDEO-PREVIEW.md, The episode's decoder.
type EpisodeFrames struct {
	program string
	path    string

	mu      sync.Mutex
	cmd     *exec.Cmd
	in      io.WriteCloser
	gone    chan struct{}
	err     error
	nextID  uint32
	idle    []uint32
	answers map[uint32]chan framewire.Record
	used    time.Time
}

// NewEpisodeFrames is the episode's decoder for the episode at path, run
// from program. Nothing starts until a stream is asked for.
func NewEpisodeFrames(program, path string) *EpisodeFrames {
	return &EpisodeFrames{program: program, path: path, answers: map[uint32]chan framewire.Record{}, used: time.Now()}
}

// cursorsKept is how many cursors a decoder keeps open for the next
// stream: the frame queue's two decoders and one more for a jump.
const cursorsKept = 3

// ahead is how many frames a stream asks for at a time, so the decoder
// makes the next frames while the page takes the ones before.
const ahead = 4

// start runs the program if it does not run. Under the lock.
func (f *EpisodeFrames) start() error {
	if f.cmd != nil {
		select {
		case <-f.gone:
			// It stopped: whatever it said, a new one starts.
			f.cmd = nil
		default:
			return nil
		}
	}
	cmd := exec.Command(f.program, f.path)
	in, err := cmd.StdinPipe()
	if err != nil {
		return err
	}
	out, err := cmd.StdoutPipe()
	if err != nil {
		return err
	}
	said := &tail{}
	errs, err := cmd.StderrPipe()
	if err != nil {
		return err
	}
	if err := cmd.Start(); err != nil {
		return fmt.Errorf("the episode's decoder could not start: %w", err)
	}
	f.cmd, f.in, f.gone = cmd, in, make(chan struct{})
	f.idle, f.answers, f.nextID = nil, map[uint32]chan framewire.Record{}, 0
	gone := f.gone
	go func() {
		lines := bufio.NewScanner(errs)
		for lines.Scan() {
			said.add(lines.Text())
		}
	}()
	go func() {
		r := bufio.NewReaderSize(out, 1<<20)
		for {
			rec, err := framewire.Read(r)
			if err != nil {
				break
			}
			f.mu.Lock()
			ch := f.answers[rec.Cursor]
			f.mu.Unlock()
			if ch != nil {
				ch <- rec
			}
		}
		werr := cmd.Wait()
		f.mu.Lock()
		f.err = fmt.Errorf("the episode's decoder stopped: %v %s", werr, said.String())
		close(gone)
		f.mu.Unlock()
	}()
	return nil
}

// send writes one request. Under the lock.
func (f *EpisodeFrames) send(format string, args ...any) error {
	_, err := fmt.Fprintf(f.in, format+"\n", args...)
	return err
}

// take hands a stream a cursor: one kept from a stream before, or a new
// one. Under the lock.
func (f *EpisodeFrames) take() (uint32, chan framewire.Record) {
	var id uint32
	if n := len(f.idle); n > 0 {
		id = f.idle[n-1]
		f.idle = f.idle[:n-1]
	} else {
		f.nextID++
		id = f.nextID
	}
	// Room for a batch and the record that ends it, so the reader never
	// waits on a stream that has stopped reading.
	ch := make(chan framewire.Record, ahead+2)
	f.answers[id] = ch
	return id, ch
}

// give keeps a cursor for the next stream, or closes it when enough are
// kept. Under the lock.
func (f *EpisodeFrames) give(id uint32) {
	delete(f.answers, id)
	if len(f.idle) < cursorsKept {
		f.idle = append(f.idle, id)
		return
	}
	_ = f.send("close %d", id)
}

// Stream hands over the frames of the episode from the moment from on,
// scaled to width by height in 8-bit I420, each with the moment it starts
// at and in a buffer of its own, the way PreviewFrames does with the
// ffmpeg program. It runs until got says no more, the context ends or the
// episode does.
func (f *EpisodeFrames) Stream(ctx context.Context, from float64, width, height int,
	got func(at float64, frame []byte) error) error {
	times := previewTimesOf(ctx)
	f.mu.Lock()
	if err := f.start(); err != nil {
		f.mu.Unlock()
		return err
	}
	f.used = time.Now()
	id, ch := f.take()
	gone := f.gone
	err := f.send("open %d %.6f %d %d", id, from, width, height)
	if err == nil {
		err = f.send("next %d %d 0", id, ahead)
	}
	f.mu.Unlock()
	times.mark(timeStarted)
	times.mark(timeOpened)
	if err != nil {
		return fmt.Errorf("the episode's decoder could not be asked: %w", err)
	}
	// A batch asked for and not yet ended: the cursor is not handed on
	// before its last record has come, or the next stream would get it.
	pending := true
	finish := func() {
		go func() {
			for pending {
				select {
				case rec := <-ch:
					pending = rec.Kind == framewire.Frame
				case <-gone:
					return
				}
			}
			f.mu.Lock()
			if f.gone == gone {
				f.give(id)
			}
			f.mu.Unlock()
		}()
	}
	for {
		select {
		case rec := <-ch:
			switch rec.Kind {
			case framewire.Frame:
				times.mark(timeFirst)
				if err := got(rec.At, rec.Body); err != nil {
					finish()
					return err
				}
			case framewire.Done:
				f.mu.Lock()
				f.used = time.Now()
				err := f.send("next %d %d 0", id, ahead)
				f.mu.Unlock()
				if err != nil {
					return fmt.Errorf("the episode's decoder could not be asked: %w", err)
				}
			case framewire.End:
				pending = false
				finish()
				return nil
			case framewire.Failed:
				pending = false
				finish()
				return errors.New(string(rec.Body))
			}
		case <-ctx.Done():
			finish()
			return ctx.Err()
		case <-gone:
			f.mu.Lock()
			err := f.err
			f.mu.Unlock()
			return err
		}
	}
}

// Idle says how long nobody has asked anything of the decoder.
func (f *EpisodeFrames) Idle() time.Duration {
	f.mu.Lock()
	defer f.mu.Unlock()
	return time.Since(f.used)
}

// Close stops the program: its input closed, it closes its cursors and
// ends, and one that does not is killed.
func (f *EpisodeFrames) Close() {
	f.mu.Lock()
	cmd, gone := f.cmd, f.gone
	if cmd != nil {
		_ = f.in.Close()
	}
	f.cmd = nil
	f.mu.Unlock()
	if cmd == nil {
		return
	}
	select {
	case <-gone:
	case <-time.After(2 * time.Second):
		_ = cmd.Process.Kill()
		<-gone
	}
}
