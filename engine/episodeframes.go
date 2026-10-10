package engine

import (
	"bufio"
	"context"
	"errors"
	"fmt"
	"io"
	"math"
	"os/exec"
	"strings"
	"sync"
	"time"

	"framefairy/internal/framewire"
)

// DecoderSaid, when set, hears every line an episode's decoder writes on
// its error stream, and when it starts and stops, which the app keeps in
// its log. It is set once, before any decoder starts.
var DecoderSaid func(video, line string)

// DecoderInfo starts a line the decoder writes only for the log: what it
// opened and how, what it is doing and how long it took. Only its other
// lines, ffmpeg's errors, say why it failed.
const DecoderInfo = "info: "

func decoderSaid(video, line string) {
	if DecoderSaid != nil {
		DecoderSaid(video, line)
	}
}

// EpisodeFrames is the episode's decoder as the Go side sees it: one
// framefairy-frames for one episode, started when the episode opens in the
// video preview, see Ready, and kept while it is open, with the file open
// and decoders ready. A stream takes a cursor of it, and a cursor a stream has finished
// with is kept for the next, so a jump moves a decoder that is already
// open instead of starting a program and reading the file's index again.
// See docs/VIDEO-PREVIEW.md, The episode's decoder.
type EpisodeFrames struct {
	program string
	path    string

	mu   sync.Mutex
	cmd  *exec.Cmd
	in   io.WriteCloser
	gone chan struct{}
	// The gone of a program Close stopped, so its streams say closed and
	// not failed.
	closed chan struct{}
	err    error
	nextID uint32
	// The cursors kept for the next stream, of picture and of sound: a
	// cursor reads one stream of the file and stays that kind.
	idle    [2][]uint32
	answers map[uint32]chan framewire.Record
	used    time.Time
	// When the picture starts, which says how the sound is read, see
	// soundSeek, found once.
	picture      float64
	pictureKnown bool
	// How the picture's brightness is coded, as the decoder found it in
	// the file when a cursor of picture first opened it.
	light framewire.Light
}

// The two kinds of cursor.
const (
	pictureCursor = 0
	soundCursor   = 1
)

// ErrFramesClosed ends a stream whose decoder was closed under it, which
// is no failure: the page asks another stream, which starts the decoder
// again.
var ErrFramesClosed = errors.New("the video's decoder was closed")

// NewEpisodeFrames is the episode's decoder for the episode at path, run
// from program. Nothing starts until a stream is asked for.
func NewEpisodeFrames(program, path string) *EpisodeFrames {
	return &EpisodeFrames{program: program, path: path, answers: map[uint32]chan framewire.Record{}, used: time.Now()}
}

// cursorsKept is how many cursors of picture a decoder keeps open for the
// next stream: the frame queue's two decoders and one more for a jump. Of
// sound it keeps two, the play's and the next one's.
var cursorsKept = [2]int{pictureCursor: 3, soundCursor: 2}

// ahead is how many frames a stream asks for at a time, so the decoder
// makes the next frames while the page takes the ones before.
const ahead = 4

// Ready starts the decoder if it does not run, with its cursors opened on
// the file, so the first frame asked for only moves one. The app calls it
// as an episode opens in the video preview.
func (f *EpisodeFrames) Ready() error {
	f.mu.Lock()
	defer f.mu.Unlock()
	f.used = time.Now()
	return f.start()
}

// warm opens the cursors a decoder keeps, each on the file with a decoder
// of its own, and hands each to the streams once its open is done. Under
// the lock, from start.
func (f *EpisodeFrames) warm(gone chan struct{}) {
	// Every picture cursor, and one of sound: a cursor of sound needs the
	// rate it is asked at only for its chain, which is made at its first
	// sound. A file with no sound says so, and the cursor is closed.
	for i := range cursorsKept[pictureCursor] + 1 {
		kind, request := pictureCursor, "open %d 0 2 2"
		if i == cursorsKept[pictureCursor] {
			kind, request = soundCursor, "sound %d 0 0 48000 2"
		}
		f.nextID++
		id := f.nextID
		ch := make(chan framewire.Record, 1)
		f.answers[id] = ch
		if f.send(gone, request, id) != nil {
			delete(f.answers, id)
			return
		}
		go func() {
			var rec framewire.Record
			select {
			case rec = <-ch:
			case <-gone:
				return
			}
			f.mu.Lock()
			defer f.mu.Unlock()
			if f.gone != gone {
				return
			}
			delete(f.answers, id)
			if rec.Kind == framewire.Opened && len(f.idle[kind]) < cursorsKept[kind] {
				f.idle[kind] = append(f.idle[kind], id)
			} else {
				_ = f.send(gone, "close %d", id)
			}
		}()
	}
}

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
		return fmt.Errorf("the video's decoder could not start: %w", err)
	}
	decoderSaid(f.path, fmt.Sprintf("started, process %d", cmd.Process.Pid))
	f.cmd, f.in, f.gone = cmd, in, make(chan struct{})
	f.idle, f.answers, f.nextID = [2][]uint32{}, map[uint32]chan framewire.Record{}, 0
	gone := f.gone
	f.warm(gone)
	// What the decoder says on its error stream, read to its end before
	// it is waited for, as exec asks, and before what it said is read.
	saidAll := make(chan struct{})
	go func() {
		defer close(saidAll)
		lines := bufio.NewScanner(errs)
		for lines.Scan() {
			if !strings.HasPrefix(lines.Text(), DecoderInfo) {
				said.add(lines.Text())
			}
			decoderSaid(f.path, lines.Text())
		}
	}()
	go func() {
		r := bufio.NewReaderSize(out, 1<<20)
		for {
			rec, err := framewire.Read(r)
			if err != nil {
				break
			}
			// Only to the streams of this program: after a Close a new one
			// numbers its cursors from one again, and what this one still
			// says belongs to none of them.
			f.mu.Lock()
			var ch chan framewire.Record
			if f.gone == gone {
				ch = f.answers[rec.Cursor]
			}
			f.mu.Unlock()
			if ch != nil {
				ch <- rec
			}
		}
		<-saidAll
		werr := cmd.Wait()
		f.mu.Lock()
		f.err = fmt.Errorf("the video's decoder stopped: %v %s", werr, said.String())
		if gone == f.closed {
			decoderSaid(f.path, fmt.Sprintf("process %d closed", cmd.Process.Pid))
		} else {
			decoderSaid(f.path, fmt.Sprintf("process %d stopped by itself: %v", cmd.Process.Pid, werr))
		}
		close(gone)
		f.mu.Unlock()
	}()
	return nil
}

// ended is why a stream of the program whose gone this is cannot go on.
// Under the lock.
func (f *EpisodeFrames) ended(gone chan struct{}) error {
	if gone == f.closed {
		return ErrFramesClosed
	}
	select {
	case <-gone:
		return f.err
	default:
		return nil
	}
}

// send writes one request to the program whose gone this is, and to no
// other: a stream of a program that was closed, or that stopped, must not
// ask the one that started after it. Under the lock.
func (f *EpisodeFrames) send(gone chan struct{}, format string, args ...any) error {
	if f.gone != gone || f.cmd == nil {
		if why := f.ended(gone); why != nil {
			return why
		}
		return ErrFramesClosed
	}
	_, err := fmt.Fprintf(f.in, format+"\n", args...)
	return err
}

// take hands a stream a cursor of its kind: one kept from a stream before,
// or a new one. Under the lock.
func (f *EpisodeFrames) take(kind int) (uint32, chan framewire.Record) {
	var id uint32
	if n := len(f.idle[kind]); n > 0 {
		id = f.idle[kind][n-1]
		f.idle[kind] = f.idle[kind][:n-1]
	} else {
		f.nextID++
		id = f.nextID
	}
	// Room for the open's answer, a batch and the record that ends it, so
	// the reader never waits on a stream that has stopped reading.
	ch := make(chan framewire.Record, ahead+3)
	f.answers[id] = ch
	return id, ch
}

// give keeps a cursor for the next stream, or closes it when enough are
// kept or it failed. Under the lock.
func (f *EpisodeFrames) give(id uint32, kind int, failed bool) {
	delete(f.answers, id)
	if !failed && len(f.idle[kind]) < cursorsKept[kind] {
		f.idle[kind] = append(f.idle[kind], id)
		return
	}
	_ = f.send(f.gone, "close %d", id)
}

// Light is how the episode's picture codes its brightness, standard video
// or HDR, known once a stream of its frames has opened. Its frames are
// made by framewire.Picture for it.
func (f *EpisodeFrames) Light() framewire.Light {
	f.mu.Lock()
	defer f.mu.Unlock()
	return f.light
}

// Stream hands over the frames of the episode from the moment from on,
// scaled to width by height in colours as framewire.Picture makes them for
// the episode's Light, each with the moment it starts at and in a buffer
// of its own, the way PreviewFrames does with the
// ffmpeg program. It runs until got says no more, the context ends or the
// episode does.
func (f *EpisodeFrames) Stream(ctx context.Context, from float64, width, height int,
	got func(at float64, frame []byte) error) error {
	return f.stream(ctx, pictureCursor, func(id uint32) string {
		return fmt.Sprintf("open %d %.6f %d %d", id, from, width, height)
	}, got)
}

// Sound hands over the sound of the episode from the moment from on, as
// 32-bit float samples at rate in so many channels, the channels of one
// moment side by side, little endian, in chunks of SoundChunk moments,
// each with the moment it starts at and in a buffer of its own, the way
// PreviewSound does with the ffmpeg program: read by the same rule, see
// soundSeek, so the video preview hears the sound the render and the
// transcript are made of. e finds where the picture starts, once.
func (f *EpisodeFrames) Sound(ctx context.Context, e *Engine, from float64, rate, channels int,
	got func(at float64, chunk []byte) error) error {
	if rate < 8000 || rate > 192000 || channels < 1 || channels > 8 {
		return fmt.Errorf("sound at %d Hz in %d channels cannot be made", rate, channels)
	}
	if math.IsNaN(from) || math.IsInf(from, 0) || from < 0 {
		from = 0
	}
	f.mu.Lock()
	known, picture := f.pictureKnown, f.picture
	f.mu.Unlock()
	if !known {
		picture = e.pictureStart(ctx, f.path)
		f.mu.Lock()
		f.picture, f.pictureKnown = picture, true
		f.mu.Unlock()
	}
	seek, _ := soundSeek(from, picture)
	return f.stream(ctx, soundCursor, func(id uint32) string {
		return fmt.Sprintf("sound %d %.6f %.6f %d %d", id, seek, from, rate, channels)
	}, got)
}

// stream runs one stream on a cursor of its kind, opened by the request
// open makes for the cursor's number.
func (f *EpisodeFrames) stream(ctx context.Context, kind int, open func(id uint32) string,
	got func(at float64, data []byte) error) error {
	times := previewTimesOf(ctx)
	f.mu.Lock()
	if err := f.start(); err != nil {
		f.mu.Unlock()
		return err
	}
	f.used = time.Now()
	id, ch := f.take(kind)
	gone := f.gone
	err := f.send(gone, "%s", open(id))
	if err == nil {
		err = f.send(gone, "next %d %d 0", id, ahead)
	}
	if err != nil {
		if why := f.ended(gone); why != nil {
			err = why
		} else {
			err = fmt.Errorf("the video's decoder could not be asked: %w", err)
		}
	}
	f.mu.Unlock()
	// The decoder runs and has the request: at once where it ran already,
	// after starting it where it did not.
	times.mark(timeStarted)
	if err != nil {
		return err
	}
	// A batch asked for and not yet ended: the cursor is not handed on
	// before its last record has come, or the next stream would get it. A
	// cursor that failed is closed rather than handed on, since what it
	// still says about its failure would reach the next stream.
	pending, failed := true, false
	finish := func() {
		go func() {
			for pending {
				select {
				case rec := <-ch:
					pending = rec.Kind == framewire.Frame || rec.Kind == framewire.Opened
				case <-gone:
					return
				}
			}
			f.mu.Lock()
			if f.gone == gone {
				f.give(id, kind, failed)
			}
			f.mu.Unlock()
		}()
	}
	for {
		select {
		case rec := <-ch:
			switch rec.Kind {
			case framewire.Opened:
				// The cursor is at from: a file opened, or a cursor moved.
				file, light := framewire.ReadOpened(rec.Body)
				if times != nil && file {
					times.File.Store(true)
				}
				if kind == pictureCursor {
					f.mu.Lock()
					f.light = light
					f.mu.Unlock()
				}
				times.mark(timeOpened)
			case framewire.Frame:
				times.mark(timeFirst)
				if err := got(rec.At, rec.Body); err != nil {
					finish()
					return err
				}
			case framewire.Done:
				f.mu.Lock()
				f.used = time.Now()
				err := f.send(gone, "next %d %d 0", id, ahead)
				if err != nil {
					if why := f.ended(gone); why != nil {
						err = why
					} else {
						err = fmt.Errorf("the video's decoder could not be asked: %w", err)
					}
				}
				f.mu.Unlock()
				if err != nil {
					return err
				}
			case framewire.End:
				pending = false
				finish()
				return nil
			case framewire.Failed:
				pending, failed = false, true
				finish()
				return errors.New(string(rec.Body))
			}
		case <-ctx.Done():
			finish()
			return ctx.Err()
		case <-gone:
			f.mu.Lock()
			err := f.ended(gone)
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
		f.closed = gone
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
