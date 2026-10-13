package engine

import (
	"bufio"
	"context"
	"errors"
	"fmt"
	"io"
	"log/slog"
	"math"
	"os"
	"os/exec"
	"strings"
	"sync"
	"time"

	"framefairy/internal/framewire"
)

// DecoderSaid, when set, hears every line an episode's decoder writes on
// its error stream, and when it starts and stops, which the app keeps in
// its log, each with its level: ffmpeg's errors and a decoder that stopped
// by itself are errors, what it says for the log alone is debug. It is
// set once, before any decoder starts.
var DecoderSaid func(video string, level slog.Level, line string)

// DecoderInfo starts a line the decoder writes only for the log: what it
// opened and how, what it is doing and how long it took. Only its other
// lines, ffmpeg's errors, say why it failed.
const DecoderInfo = "info: "

// DecoderWarning starts a line the decoder writes for the log about
// something that went wrong and that it got round, like the graphics chip
// failing a frame that the processor then decoded. It is not why a
// decoder failed, so it is kept with the warnings and not with the errors.
const DecoderWarning = "warning: "

func decoderSaid(video string, level slog.Level, line string) {
	if DecoderSaid != nil {
		DecoderSaid(video, level, line)
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
	// What a test adds to the decoder's environment, and hears of all it
	// says on its error stream, the way DecoderSaid does, and what it does
	// between the decoder's end and the reason for it being known.
	env    []string
	heard  func(level slog.Level, line string)
	waited func()

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
	idle [2][]uint32
	// Where each kept cursor of picture stands: the grid and size it read
	// on and the last frame it handed over. A cursor that warmed, failed
	// or was closed has none.
	placed map[uint32]place
	// The cursors of picture whose stream has stopped and that still wait
	// for the last frames of their batch, where they stood when it stopped,
	// and what is told when one of them is kept or closed.
	finishing map[uint32]place
	finished  *sync.Cond
	answers   map[uint32]chan framewire.Record
	used      time.Time
	// When the picture starts, which says how the sound is read, see
	// soundSeek, found once.
	picture      float64
	pictureKnown bool
	// How the picture's brightness is coded, as the decoder found it in
	// the file when a cursor of picture first opened it.
	light framewire.Light
	// The cursor of captions, one at a time, see Captions.
	captionsMu sync.Mutex
	captions   *captionsCursor
}

// place is where a cursor of picture stands, or where a stream wants one:
// on grid at size w by h, at frame.
type place struct {
	grid  framewire.Grid
	w, h  int
	frame int
}

// placeAhead is how far before a stream's first frame a kept cursor may
// stand, in seconds, to be the one handed to it: the decoder reads on from
// there rather than seeking, see goOnAhead in framefairy-frames, which it
// matches.
const placeAhead = 5.0

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
	return &EpisodeFrames{program: program, path: path, answers: map[uint32]chan framewire.Record{},
		placed: map[uint32]place{}, finishing: map[uint32]place{}, used: time.Now()}
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
		f.nextID++
		id := f.nextID
		// A grid of one frame: an open moves a cursor to the grid it is
		// asked on, whatever it was on before.
		kind, request := pictureCursor, framewire.OpenRequest(id, 0, 2, 2, framewire.Grid{Num: 1, Den: 1, Count: 1})
		if i == cursorsKept[pictureCursor] {
			kind, request = soundCursor, fmt.Sprintf("sound %d 0 0 48000 2", id)
		}
		ch := make(chan framewire.Record, 1)
		f.answers[id] = ch
		if f.send(gone, "%s", request) != nil {
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
	if f.env != nil {
		cmd.Env = append(os.Environ(), f.env...)
	}
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
	decoderSaid(f.path, slog.LevelDebug, fmt.Sprintf("started, process %d", cmd.Process.Pid))
	f.cmd, f.in, f.gone = cmd, in, make(chan struct{})
	f.idle, f.answers, f.nextID = [2][]uint32{}, map[uint32]chan framewire.Record{}, 0
	f.placed, f.finishing = map[uint32]place{}, map[uint32]place{}
	gone := f.gone
	f.warm(gone)
	// What the decoder says on its error stream, read to its end before
	// it is waited for, as exec asks, and before what it said is read.
	saidAll := make(chan struct{})
	go func() {
		defer close(saidAll)
		lines := bufio.NewScanner(errs)
		for lines.Scan() {
			level, line := slog.LevelError, lines.Text()
			if rest, ok := strings.CutPrefix(line, DecoderInfo); ok {
				level, line = slog.LevelDebug, rest
			} else if rest, ok := strings.CutPrefix(line, DecoderWarning); ok {
				level, line = slog.LevelWarn, rest
			} else {
				said.add(line)
			}
			decoderSaid(f.path, level, line)
			if f.heard != nil {
				f.heard(level, line)
			}
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
		if f.waited != nil {
			f.waited()
		}
		f.mu.Lock()
		f.err = fmt.Errorf("the video's decoder stopped: %v %s", werr, said.String())
		if gone == f.closed {
			decoderSaid(f.path, slog.LevelDebug, fmt.Sprintf("process %d closed", cmd.Process.Pid))
		} else {
			decoderSaid(f.path, slog.LevelError, fmt.Sprintf("process %d stopped by itself: %v", cmd.Process.Pid, werr))
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
// ask the one that started after it. Under the lock. A send that fails is
// told by refused why.
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

// refused is why a request the program whose gone this is could not take
// failed. A write fails once the program's input is shut, and Wait shuts
// it when the program stopped, before gone is closed and the reason is
// known: a file with no picture stops its decoder at once, and a stream
// that asked in between said "file already closed" where it should have
// said that the file has no picture. So the reason is waited for. The
// wait ends with gone, when the program ends, or with ctx: a program that
// shut its input and runs on is ended by Close, and every stream has a
// context that ends. Not under the lock, which the end of the program
// takes.
func (f *EpisodeFrames) refused(ctx context.Context, gone chan struct{}, err error) error {
	f.mu.Lock()
	why := f.ended(gone)
	f.mu.Unlock()
	if why != nil {
		return why
	}
	select {
	case <-gone:
	case <-ctx.Done():
		return ctx.Err()
	}
	f.mu.Lock()
	defer f.mu.Unlock()
	if why := f.ended(gone); why != nil {
		return why
	}
	return fmt.Errorf("the video's decoder could not be asked: %w", err)
}

// take hands a stream a cursor of its kind: one kept from a stream before,
// or a new one. A stream of picture that says where it starts gets the kept
// cursor that stands closest before it, on its grid and at its size, so
// the decoder reads on from there instead of seeking back to a key frame
// and decoding up to it again. A jump a few frames on, or a play after a
// pause, is then a frame or two of work. Where the closest is a cursor
// whose stream has just stopped, the playhead it followed jumping, it
// waits for that cursor to finish the few frames of its batch: the
// decoder has them to make before it hears anything else on that cursor,
// and they cost less than a seek. Otherwise it gets the cursor kept last.
// Under the lock.
func (f *EpisodeFrames) take(kind int, want *place) (uint32, chan framewire.Record) {
	var id uint32
	for want != nil {
		best, stands := -1, -1
		for i, c := range f.idle[kind] {
			if p, ok := f.placed[c]; ok && p.before(*want) && p.frame > stands {
				best, stands = i, p.frame
			}
		}
		later := false
		for _, p := range f.finishing {
			later = later || (p.before(*want) && p.frame > stands)
		}
		if later {
			f.wait()
			continue
		}
		if best >= 0 {
			id = f.idle[kind][best]
			f.idle[kind] = append(f.idle[kind][:best], f.idle[kind][best+1:]...)
		}
		break
	}
	if n := len(f.idle[kind]); id == 0 && n > 0 {
		id = f.idle[kind][n-1]
		f.idle[kind] = f.idle[kind][:n-1]
	} else if id == 0 {
		f.nextID++
		id = f.nextID
	}
	delete(f.placed, id)
	// Room for the open's answer, a batch and the record that ends it, so
	// the reader never waits on a stream that has stopped reading.
	ch := make(chan framewire.Record, ahead+3)
	f.answers[id] = ch
	return id, ch
}

// before tells whether a cursor standing at p can read on to want: on the
// same grid at the same size, before it and no further than placeAhead.
func (p place) before(want place) bool {
	return p.frame >= 0 && p.grid == want.grid && p.w == want.w && p.h == want.h && p.frame < want.frame &&
		want.grid.At(want.frame)-want.grid.At(p.frame) <= placeAhead
}

// wait waits for a finishing cursor to be kept or closed. Under the lock,
// which it lets go of while it waits.
func (f *EpisodeFrames) wait() {
	if f.finished == nil {
		f.finished = sync.NewCond(&f.mu)
	}
	f.finished.Wait()
}

// done says a finishing cursor was kept or closed. Under the lock.
func (f *EpisodeFrames) done(id uint32, gone chan struct{}) {
	if f.gone == gone {
		delete(f.finishing, id)
	}
	if f.finished != nil {
		f.finished.Broadcast()
	}
}

// give keeps a cursor for the next stream, standing at at where it is a
// cursor of picture that handed over a frame, or closes it when it failed.
// When enough are kept already, the one kept longest is closed instead:
// the cursor a stream has just let go of stands where the next is likely
// to start, and a cursor that only warmed stands nowhere. Under the lock.
func (f *EpisodeFrames) give(id uint32, kind int, failed bool, at *place) {
	delete(f.answers, id)
	if failed {
		_ = f.send(f.gone, "close %d", id)
		return
	}
	if len(f.idle[kind]) >= cursorsKept[kind] {
		old := f.idle[kind][0]
		f.idle[kind] = f.idle[kind][1:]
		delete(f.placed, old)
		_ = f.send(f.gone, "close %d", old)
	}
	f.idle[kind] = append(f.idle[kind], id)
	if at != nil && at.frame >= 0 {
		f.placed[id] = *at
	}
}

// Light is how the episode's picture codes its brightness, standard video
// or HDR, known once a stream of its frames has opened. Its frames are
// made by framewire.Picture for it.
func (f *EpisodeFrames) Light() framewire.Light {
	f.mu.Lock()
	defer f.mu.Unlock()
	return f.light
}

// Stream hands over the frames of the episode on the grid, the frames of
// its short, see SourceInfo.Grid, from the one that holds the moment from
// on, scaled to width by height in colours as framewire.Picture makes them
// for the episode's Light, each with the moment of the grid it is for and
// in a buffer of its own. It runs until got says no more, the context
// ends or the grid does.
func (f *EpisodeFrames) Stream(ctx context.Context, grid framewire.Grid, from float64, width, height int,
	got func(at float64, frame []byte) error) error {
	if grid.Num <= 0 || grid.Den <= 0 {
		return errors.New("the video has no frame rate to count its frames by")
	}
	if !grid.Valid() {
		return fmt.Errorf("the frames of the video cannot be counted: the grid %+v", grid)
	}
	if math.IsNaN(from) || math.IsInf(from, 0) {
		from = 0
	}
	first := grid.Holding(from)
	return f.stream(ctx, pictureCursor, &place{grid: grid, w: width, h: height, frame: first}, func(id uint32) string {
		return framewire.OpenRequest(id, first, width, height, grid)
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
	return f.stream(ctx, soundCursor, nil, func(id uint32) string {
		return fmt.Sprintf("sound %d %.6f %.6f %d %d", id, seek, from, rate, channels)
	}, got)
}

// stream runs one stream on a cursor of its kind, opened by the request
// open makes for the cursor's number. A stream of picture says where it
// starts, want, and the cursor is kept standing at the last frame it
// handed over.
func (f *EpisodeFrames) stream(ctx context.Context, kind int, want *place, open func(id uint32) string,
	got func(at float64, data []byte) error) error {
	times := previewTimesOf(ctx)
	f.mu.Lock()
	if err := f.start(); err != nil {
		f.mu.Unlock()
		return err
	}
	f.used = time.Now()
	id, ch := f.take(kind, want)
	gone := f.gone
	err := f.send(gone, "%s", open(id))
	if err == nil {
		err = f.send(gone, "next %d %d 0", id, ahead)
	}
	f.mu.Unlock()
	// The decoder runs and has the request: at once where it ran already,
	// after starting it where it did not.
	times.mark(timeStarted)
	if err != nil {
		return f.refused(ctx, gone, err)
	}
	// A batch asked for and not yet ended: the cursor is not handed on
	// before its last record has come, or the next stream would get it. A
	// cursor that failed is closed rather than handed on, since what it
	// still says about its failure would reach the next stream.
	pending, failed := true, false
	// Where the cursor stands: the last frame the decoder handed over,
	// read or not.
	var at *place
	if want != nil {
		at = &place{grid: want.grid, w: want.w, h: want.h, frame: -1}
	}
	stood := func(rec framewire.Record) {
		if at != nil && rec.Kind == framewire.Frame {
			at.frame = at.grid.Holding(rec.At)
		}
	}
	finish := func() {
		if at != nil && pending {
			f.mu.Lock()
			if f.gone == gone {
				f.finishing[id] = *at
			}
			f.mu.Unlock()
		}
		go func() {
			for pending {
				select {
				case rec := <-ch:
					stood(rec)
					pending = rec.Kind == framewire.Frame || rec.Kind == framewire.Opened
				case <-gone:
					f.mu.Lock()
					f.done(id, gone)
					f.mu.Unlock()
					return
				}
			}
			f.mu.Lock()
			if f.gone == gone {
				f.give(id, kind, failed, at)
			}
			f.done(id, gone)
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
				stood(rec)
				times.mark(timeFirst)
				if err := got(rec.At, rec.Body); err != nil {
					finish()
					return err
				}
			case framewire.Done:
				f.mu.Lock()
				f.used = time.Now()
				err := f.send(gone, "next %d %d 0", id, ahead)
				f.mu.Unlock()
				if err != nil {
					return f.refused(ctx, gone, err)
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

// captionsCursor is the decoder's cursor of captions: the caption file it
// holds open, at a size, see Captions.
type captionsCursor struct {
	id   uint32
	ch   chan framewire.Record
	gone chan struct{}
	// The file and fonts and size it holds open.
	held string
}

// Captions is the captions of a caption file drawn by libass at a size,
// at ms milliseconds of the short, see SourceInfo.CaptionMoment: the layer
// the video preview lays over a frame, see framewire.Layer. The file is
// the one the render burns in, made by Engine.CaptionTrack, and the faces
// it names are in its folder of fonts. A file is held open until another
// is asked for, so the frames of a clip each only draw.
func (f *EpisodeFrames) Captions(ctx context.Context, captions framewire.Captions, ms int64) (framewire.Layer, error) {
	if !framewire.LayerFits(captions.Width, captions.Height) || !framewire.LayerFits(captions.ShortW, captions.ShortH) {
		return framewire.Layer{}, fmt.Errorf("captions cannot be drawn %d by %d of %d by %d",
			captions.Width, captions.Height, captions.ShortW, captions.ShortH)
	}
	f.captionsMu.Lock()
	defer f.captionsMu.Unlock()
	f.mu.Lock()
	if err := f.start(); err != nil {
		f.mu.Unlock()
		return framewire.Layer{}, err
	}
	f.used = time.Now()
	c := f.captions
	if c == nil || c.gone != f.gone {
		f.nextID++
		c = &captionsCursor{id: f.nextID, ch: make(chan framewire.Record, 2), gone: f.gone}
		f.answers[c.id] = c.ch
		f.captions = c
	}
	held := fmt.Sprintf("%+v", captions)
	var err error
	if c.held != held {
		c.held = ""
		err = f.send(c.gone, "%s", framewire.CaptionsRequest(c.id, captions))
	}
	if err == nil {
		err = f.send(c.gone, "%s", framewire.DrawRequest(c.id, ms))
	}
	if err != nil {
		if why := f.ended(c.gone); why != nil {
			err = why
		}
		f.mu.Unlock()
		return framewire.Layer{}, err
	}
	f.mu.Unlock()
	wait := func() (framewire.Record, error) {
		select {
		case rec := <-c.ch:
			if rec.Kind == framewire.Failed {
				return rec, errors.New(string(rec.Body))
			}
			return rec, nil
		case <-c.gone:
			f.mu.Lock()
			defer f.mu.Unlock()
			return framewire.Record{}, f.ended(c.gone)
		case <-ctx.Done():
			// The answers still on their way would be read as the next
			// draw's, so the cursor is let go of and the next draw opens
			// one of its own.
			f.mu.Lock()
			if f.captions == c {
				f.captions = nil
				delete(f.answers, c.id)
				_ = f.send(c.gone, "close %d", c.id)
			}
			f.mu.Unlock()
			return framewire.Record{}, ctx.Err()
		}
	}
	if c.held != held {
		if _, err := wait(); err != nil {
			// A draw follows a captions that failed, and fails too.
			_, _ = wait()
			return framewire.Layer{}, err
		}
		c.held = held
	}
	rec, err := wait()
	if err != nil {
		return framewire.Layer{}, err
	}
	return framewire.ReadLayer(rec.Body)
}
