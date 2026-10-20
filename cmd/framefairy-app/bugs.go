package main

import (
	"bytes"
	"encoding/base64"
	"encoding/json"
	"errors"
	"fmt"
	"image"
	"image/color"
	"image/jpeg"
	"image/png"
	"io"
	"net/http"
	"os"
	"path/filepath"
	"regexp"
	"slices"
	"strings"
	"sync"
	"unicode/utf8"

	"github.com/rs/zerolog"
	"github.com/spf13/afero"

	"framefairy/engine"
)

// Bugs, at the foot of the sidebar: Report a Problem in a window of its
// own, one at a time, so a person can talk about a problem while they go
// through it in the app. What they write and the pictures they attach are
// a draft, kept in the app's folder until a report is made from it, so
// closing the window loses nothing, and neither does the app crashing,
// which may be the problem being reported. Plan row 2.244 and
// docs/LOGGING.md.

// How many pictures and recordings a report takes, and how big a file each
// may be. They are meant for screenshots, which a Mac's screen makes at a
// few MB, and short screen recordings, see recording.go.
const (
	picturesMax     = 10
	pictureBytesMax = 10 << 20
	// The most pixels a picture may say it has before it is decoded, a
	// little over two 6K screens, so a file that claims a picture of a
	// thousand times the memory is turned away without being read.
	picturePixelsMax = 50_000_000
	// The same for a picture of sixteen bits a colour, which takes twice
	// the memory a pixel.
	picturePixelsDeepMax = 20_000_000
)

// cut makes text valid UTF-8 and cuts it to at most max bytes, between two
// letters, never in the middle of one: a German report cut in half an ä
// would end in a byte no reader can show.
func cut(text string, max int) string {
	text = strings.ToValidUTF8(text, "")
	if len(text) <= max {
		return text
	}
	for max > 0 && !utf8.RuneStart(text[max]) {
		max--
	}
	return text[:max]
}

// ReportPicture is one picture attached to a report, known by its id.
type ReportPicture struct {
	ID string `json:"id"`
	// The file it came from, without its folder, for the tile's title.
	Name   string `json:"name"`
	Width  int    `json:"width"`
	Height int    `json:"height"`
	// The ending a screen recording is kept under, "" for a picture. A
	// recording keeps a still of its first frame beside it, for the tile.
	Recording string `json:"recording,omitempty"`
	// How long a recording is, in seconds.
	Seconds float64 `json:"seconds,omitempty"`
	// Removed by the person, kept a while so it can be put back. A removed
	// picture goes in no report and counts toward no limit.
	Removed bool `json:"removed,omitempty"`
}

// ReportDraft is a report before it is made: what the person wrote and the
// pictures they attached, in the order they came.
type ReportDraft struct {
	Text     string          `json:"text"`
	Pictures []ReportPicture `json:"pictures"`
	// Why some of the pictures given last were left out. Said once, with
	// the answer to what gave them, and never kept.
	Error string `json:"error,omitempty"`
}

// draftFile is the draft as it is kept, with the removed pictures and the
// number the next picture gets.
type draftFile struct {
	Text     string          `json:"text"`
	Pictures []ReportPicture `json:"pictures"`
	Next     int             `json:"next"`
}

// reporting is the report window and its draft.
type reporting struct {
	mu    sync.Mutex
	files afero.Fs
	// Where the draft is kept: draft.json and the pictures beside it.
	dir    string
	draft  draftFile
	loaded bool
	// The open window: front brings it forward and shut closes it, both
	// nil while there is none.
	front, shut func()
	// making is set while a window is being made, and made counts the
	// windows made, so the one that closes is known, see open.
	making bool
	made   int
	// Why the window is being closed, when the app closes it, for the log.
	closing string
	// make makes the window, which calls closed once it is gone, and
	// gives what brings it forward and what closes it. Set by main.go.
	make func(closed func()) (front, shut func())
	// told says to the app's window whether the report window is open,
	// and said tells the report window what became of files dropped on
	// it. Both are events in the app.
	told func(open bool)
	said func(ReportDraft)
	// pick asks for pictures in the system's open box.
	pick func() ([]string, error)
	// stop drops the recording the window had going, once the window is
	// gone, CancelListening. Set by main.go.
	stop func()
	// The video of the last act that named one, which a report names.
	video string
}

func newReporting(files afero.Fs, dir string) *reporting {
	return &reporting{files: files, dir: dir}
}

// OpenReport opens the report window, or brings it forward when it is open
// already, with how the person asked for it.
func (s *FrameFairy) OpenReport(how string) {
	if s.report != nil {
		s.report.open(how)
	}
}

// ReportWindowOpen says whether the report window is open, for Bugs in the
// sidebar to show.
func (s *FrameFairy) ReportWindowOpen() bool {
	if s.report == nil {
		return false
	}
	s.report.mu.Lock()
	defer s.report.mu.Unlock()
	return s.report.front != nil
}

func (r *reporting) open(how string) {
	r.mu.Lock()
	if front := r.front; front != nil {
		r.mu.Unlock()
		theLog.line(zerolog.InfoLevel, "report").Str("how", how).Msg("the report window is brought forward")
		front()
		return
	}
	// Wails runs each call on a goroutine of its own, so a double-click on
	// Bugs asks twice at once. The window is marked as being made before
	// the lock is let go, and the second ask leaves it to the first, which
	// brings it forward as it opens.
	if r.making || r.make == nil {
		r.mu.Unlock()
		return
	}
	r.making = true
	r.made++
	which, makeIt := r.made, r.make
	r.mu.Unlock()
	theLog.line(zerolog.InfoLevel, "report").Str("how", how).Msg("the report window opens")
	// Whether this window is gone, kept under r.mu. It can go before make
	// returns, and then it is never taken for open.
	gone := false
	var once sync.Once
	front, shut := makeIt(func() {
		once.Do(func() { r.closed(which, &gone) })
	})
	r.mu.Lock()
	r.making = false
	// Read once, under the lock: closed writes it under the lock too.
	open := !gone
	if open {
		r.front, r.shut = front, shut
	}
	r.mu.Unlock()
	if open && r.told != nil {
		r.told(true)
	}
}

// closed is what happens once the window numbered which is gone, by its
// red button, Cmd+W, a report or the app's window closing.
func (r *reporting) closed(which int, gone *bool) {
	r.mu.Lock()
	*gone = true
	// Only the window that is open lets go of it, never one before it.
	if r.made == which {
		r.front, r.shut = nil, nil
	}
	how := r.closing
	r.closing = ""
	r.mu.Unlock()
	if how == "" {
		how = "window"
	}
	theLog.line(zerolog.InfoLevel, "report").Str("how", how).Msg("the report window closed")
	// A recording is the window's, and goes with it. Tim closed the window
	// with Cmd+W while he talked, and the recording ran on: the next
	// window took it up again, and every word he had said came back grey
	// into the text the draft had kept, with the cursor somewhere in it.
	// What was written down is in the draft already.
	if r.stop != nil {
		r.stop()
	}
	if r.told != nil {
		r.told(false)
	}
}

// closeWindow closes the report window, with why: "report" once a report
// is made from it, "app" when the app's own window closes, since a report
// window left alone would keep the app running with nothing to work in.
func (r *reporting) closeWindow(how string) {
	if r == nil {
		return
	}
	r.mu.Lock()
	shut := r.shut
	if shut != nil {
		r.closing = how
	}
	r.mu.Unlock()
	if shut != nil {
		shut()
	}
}

// The report window's size, in points, and the least it may be.
const (
	reportWidth, reportHeight       = 560, 640
	reportMinWidth, reportMinHeight = 440, 480
	// The room between the app's window and the report window beside it.
	reportBeside = 16
)

// beside is where the report window opens: to the right of the app's
// window where the screen has room, otherwise over the middle of it, and
// always inside the screen's work area.
func beside(app, work image.Rectangle, w, h int) image.Point {
	at := image.Pt(app.Max.X+reportBeside, app.Min.Y)
	if at.X+w > work.Max.X {
		at = image.Pt(app.Min.X+(app.Dx()-w)/2, app.Min.Y+(app.Dy()-h)/2)
	}
	at.X = max(work.Min.X, min(at.X, work.Max.X-w))
	at.Y = max(work.Min.Y, min(at.Y, work.Max.Y-h))
	return at
}

// sawVideo keeps the video an act named, see Acted.
func (r *reporting) sawVideo(video string) {
	if r == nil || video == "" {
		return
	}
	r.mu.Lock()
	r.video = video
	r.mu.Unlock()
}

// lastVideo is the video of the last act that named one.
func (r *reporting) lastVideo() string {
	if r == nil {
		return ""
	}
	r.mu.Lock()
	defer r.mu.Unlock()
	return r.video
}

// ReportDraft gives the draft: what was written and the pictures attached.
func (s *FrameFairy) ReportDraft() ReportDraft {
	if s.report == nil {
		return ReportDraft{Pictures: []ReportPicture{}}
	}
	s.report.mu.Lock()
	defer s.report.mu.Unlock()
	return s.report.shown("")
}

// SaveReportText keeps what is written in the report's field, as it
// changes.
func (s *FrameFairy) SaveReportText(text string) error {
	if s.report == nil {
		return nil
	}
	text = cut(text, reportWordsMax)
	r := s.report
	r.mu.Lock()
	defer r.mu.Unlock()
	r.load()
	if r.draft.Text == text {
		return nil
	}
	r.draft.Text = text
	return r.keep()
}

// AttachPictures asks for pictures in the system's open box and attaches
// them. Cancelled, it attaches nothing and says nothing.
func (s *FrameFairy) AttachPictures() ReportDraft {
	if s.report == nil {
		return ReportDraft{Pictures: []ReportPicture{}}
	}
	pick := s.report.pick
	if pick == nil && s.app != nil {
		pick = func() ([]string, error) {
			return s.app.Dialog.OpenFile().
				SetTitle("Attach screenshots or recordings").
				CanChooseFiles(true).
				AddFilter("Screenshots and recordings", "*.png;*.jpg;*.jpeg;"+recordingPattern()).
				PromptForMultipleSelection()
		}
	}
	if pick == nil {
		return s.ReportDraft()
	}
	paths, err := pick()
	if err != nil {
		d := s.ReportDraft()
		d.Error = sentence(err.Error())
		return d
	}
	return s.report.attachFiles("picked", paths)
}

// AttachPasted attaches a picture pasted into the report's field, its
// bytes in base64, the way the window has them.
func (s *FrameFairy) AttachPasted(name, data string) ReportDraft {
	if s.report == nil {
		return ReportDraft{Pictures: []ReportPicture{}}
	}
	if base64.StdEncoding.DecodedLen(len(data)) > pictureBytesMax+3 {
		return s.report.attach("pasted", []handed{{name: name, err: errTooBig}})
	}
	b, err := base64.StdEncoding.DecodeString(data)
	return s.report.attach("pasted", []handed{{name: name, data: b, err: err}})
}

// RemovePicture removes a picture from the draft. It can be put back.
func (s *FrameFairy) RemovePicture(id string) ReportDraft {
	return s.markPicture(id, true)
}

// PutBackPicture puts a removed picture back where it was.
func (s *FrameFairy) PutBackPicture(id string) ReportDraft {
	return s.markPicture(id, false)
}

// ClearReport empties the draft, what was written and every picture and
// recording, once Clear has asked first. The app's own copies of the
// pictures go, never the files they were handed over from. Issue #415.
func (s *FrameFairy) ClearReport() ReportDraft {
	if s.report == nil {
		return ReportDraft{Pictures: []ReportPicture{}}
	}
	r := s.report
	r.mu.Lock()
	defer r.mu.Unlock()
	r.load()
	for _, p := range r.draft.Pictures {
		r.forget(p)
	}
	theLog.line(zerolog.InfoLevel, "report").Int("characters", len(r.draft.Text)).Int("pictures", len(r.draft.Pictures)).Msg("the report was cleared")
	// Next stays, so an id is never given twice in a run.
	r.draft = draftFile{Pictures: []ReportPicture{}, Next: r.draft.Next}
	if err := r.keep(); err != nil {
		return r.shown(sentence(err.Error()))
	}
	return r.shown("")
}

func (s *FrameFairy) markPicture(id string, removed bool) ReportDraft {
	if s.report == nil {
		return ReportDraft{Pictures: []ReportPicture{}}
	}
	r := s.report
	r.mu.Lock()
	defer r.mu.Unlock()
	r.load()
	i := slices.IndexFunc(r.draft.Pictures, func(p ReportPicture) bool { return p.ID == id })
	if i < 0 {
		return r.shown("")
	}
	if !removed && r.count() >= picturesMax {
		return r.shown(fmt.Sprintf("A report takes %d pictures and recordings at most", picturesMax))
	}
	r.draft.Pictures[i].Removed = removed
	what := "a picture"
	if r.draft.Pictures[i].Recording != "" {
		what = "a recording"
	}
	how := what + " was put back"
	if removed {
		how = what + " was removed"
	}
	theLog.line(zerolog.InfoLevel, "report").Str("picture", id).Msg(how)
	if err := r.keep(); err != nil {
		return r.shown(sentence(err.Error()))
	}
	return r.shown("")
}

// dropped attaches files dropped on the report window and tells the window.
func (r *reporting) dropped(files []string) {
	d := r.attachFiles("drop", files)
	if r.said != nil {
		r.said(d)
	}
}

// handed is a picture or a recording handed to the report, with why it
// could not be read.
type handed struct {
	name string
	data []byte
	err  error
}

var errTooBig = fmt.Errorf("is bigger than %d MB", pictureBytesMax>>20)

// attachFiles reads the files given and attaches them. The picker, a drop
// and a paste all end in attach, so they decide the same things the same
// way.
func (r *reporting) attachFiles(how string, paths []string) ReportDraft {
	var all []handed
	for _, p := range paths {
		g := handed{name: filepath.Base(p)}
		g.data, g.err = readSmall(p)
		all = append(all, g)
	}
	return r.attach(how, all)
}

// readSmall reads a picture the person picked, from the real disk, where
// the system's box and a drop find it, refusing one too big to be a
// screenshot before it reads it.
func readSmall(path string) ([]byte, error) {
	f, err := os.Open(path)
	if err != nil {
		return nil, errors.New("could not be read")
	}
	defer f.Close()
	info, err := f.Stat()
	if err != nil || !info.Mode().IsRegular() {
		return nil, errNotKept
	}
	if info.Size() > pictureBytesMax {
		return nil, errTooBig
	}
	return io.ReadAll(io.LimitReader(f, pictureBytesMax+1))
}

// attach attaches the pictures it can, in order, and says why it left out
// the others. The pictures are decoded and written again before the
// draft is taken, half a second for a 5K screenshot, so a drop of ten
// keeps nobody waiting: not Bugs, not the window closing, not the text
// being kept as it is typed.
func (r *reporting) attach(how string, all []handed) ReportDraft {
	type read struct {
		name string
		pic  ReportPicture
		body []byte
		film []byte
		err  error
	}
	got := make([]read, len(all))
	for i, g := range all {
		got[i].name = tidyName(g.name)
		got[i].pic, got[i].body, got[i].film, got[i].err = g.attachment()
	}
	r.mu.Lock()
	defer r.mu.Unlock()
	r.load()
	var why []string
	added := 0
	for i, g := range got {
		name, pic, err := g.name, g.pic, g.err
		if err == nil && r.count() >= picturesMax {
			err = fmt.Errorf("would be more than the %d pictures and recordings a report takes", picturesMax)
		}
		if err == nil {
			r.draft.Next++
			pic.ID = fmt.Sprintf("p%d", r.draft.Next)
			pic.Name = name
			if err = r.files.MkdirAll(r.dir, 0o700); err == nil {
				err = engine.WriteAtomic(r.files, filepath.Join(r.dir, pic.ID+".png"), g.body)
			}
			if err == nil && pic.Recording != "" {
				err = engine.WriteAtomic(r.files, filepath.Join(r.dir, pic.ID+"."+pic.Recording), g.film)
			}
			if err != nil {
				err = errors.New("could not be kept")
			}
		}
		if err != nil {
			why = append(why, name+" cannot be attached because it "+err.Error())
			theLog.line(zerolog.WarnLevel, "report").Str("how", how).Str("name", name).Int("bytes", len(all[i].data)).Err(err).Msg("a file was left out")
			continue
		}
		r.draft.Pictures = append(r.draft.Pictures, pic)
		added++
		what := "a picture was attached"
		if pic.Recording != "" {
			what = "a recording was attached"
		}
		theLog.line(zerolog.InfoLevel, "report").Str("how", how).Str("picture", pic.ID).Str("name", name).
			Int("bytes", len(all[i].data)).Int("width", pic.Width).Int("height", pic.Height).
			Str("recording", pic.Recording).Float64("seconds", pic.Seconds).Msg(what)
	}
	if added > 0 {
		if err := r.keep(); err != nil {
			why = append(why, sentence(err.Error()))
		}
	}
	// A reason starts with the name of the file, as it is written.
	return r.shown(strings.Join(why, ". "))
}

// decoding is told each time a picture is about to be decoded. A test
// puts its own in here, to hold a decode and see that nothing waits on it.
var decoding = func() {}

// attachment reads what was handed to the report: a picture, see picture,
// or a screen recording, see recording.go. It gives the picture, or the
// still of the recording, as a PNG, and the recording.
func (g handed) attachment() (ReportPicture, []byte, []byte, error) {
	if g.err != nil {
		return ReportPicture{}, nil, nil, g.err
	}
	if len(g.data) > pictureBytesMax {
		return ReportPicture{}, nil, nil, errTooBig
	}
	if reader := sniffRecording(g.data); reader != "" {
		return g.recording(reader)
	}
	pic, body, err := g.picture()
	return pic, body, nil, err
}

// picture decodes a picture handed to the report and writes it again as a
// PNG of its pixels alone, so nothing else the file held, a photo's place
// or its camera, goes with a report.
func (g handed) picture() (ReportPicture, []byte, error) {
	cfg, format, err := image.DecodeConfig(bytes.NewReader(g.data))
	if err != nil || (format != "png" && format != "jpeg") {
		return ReportPicture{}, nil, errNotKept
	}
	if cfg.Width <= 0 || cfg.Height <= 0 || cfg.Width*cfg.Height > picturePixelsMax {
		return ReportPicture{}, nil, errors.New("is too large a picture")
	}
	// Sixteen bits a colour takes twice the memory once decoded, so a flat
	// picture of that kind under 10 MB could swell the app by 400 MB.
	if deep(cfg.ColorModel) && cfg.Width*cfg.Height > picturePixelsDeepMax {
		return ReportPicture{}, nil, errors.New("is too large a picture")
	}
	decoding()
	var img image.Image
	if format == "png" {
		img, err = png.Decode(bytes.NewReader(g.data))
	} else {
		img, err = jpeg.Decode(bytes.NewReader(g.data))
	}
	if err != nil {
		return ReportPicture{}, nil, errors.New("could not be read as a picture")
	}
	var out bytes.Buffer
	if err := (&png.Encoder{CompressionLevel: png.BestSpeed}).Encode(&out, img); err != nil {
		return ReportPicture{}, nil, errors.New("could not be kept")
	}
	b := img.Bounds()
	return ReportPicture{Width: b.Dx(), Height: b.Dy()}, out.Bytes(), nil
}

// deep says whether a colour model keeps sixteen bits a colour.
func deep(m color.Model) bool {
	return m == color.RGBA64Model || m == color.NRGBA64Model || m == color.Gray16Model
}

// tidyName is a file name fit for the log and a tile's title: no folder,
// nothing that is not printable, not too long.
func tidyName(name string) string {
	name = filepath.Base(strings.ReplaceAll(name, "\\", "/"))
	name = strings.Map(func(r rune) rune {
		if r < ' ' || r == 0x7f {
			return -1
		}
		return r
	}, name)
	if name == "." || name == "/" || name == "" {
		name = "picture"
	}
	return cut(name, 120)
}

// sentence starts a reason with a capital letter.
func sentence(s string) string {
	if s == "" {
		return ""
	}
	return strings.ToUpper(s[:1]) + s[1:]
}

// count is how many pictures the draft holds that are not removed.
func (r *reporting) count() int {
	n := 0
	for _, p := range r.draft.Pictures {
		if !p.Removed {
			n++
		}
	}
	return n
}

// shown is the draft as the window sees it, with why something given was
// left out.
func (r *reporting) shown(why string) ReportDraft {
	r.load()
	d := ReportDraft{Text: r.draft.Text, Pictures: []ReportPicture{}, Error: why}
	for _, p := range r.draft.Pictures {
		if !p.Removed {
			d.Pictures = append(d.Pictures, p)
		}
	}
	return d
}

// load reads the draft once, the first time it is asked for, and lets go
// of the pictures removed in an earlier run, whose time to be put back is
// long past.
func (r *reporting) load() {
	if r.loaded {
		return
	}
	r.loaded = true
	data, err := afero.ReadFile(r.files, filepath.Join(r.dir, "draft.json"))
	if err != nil {
		return
	}
	var d draftFile
	if json.Unmarshal(data, &d) != nil {
		return
	}
	d.Text = cut(d.Text, reportWordsMax)
	var kept []ReportPicture
	for _, p := range d.Pictures {
		if !pictureID.MatchString(p.ID) {
			continue
		}
		if p.Recording != "" && !isRecordingEnding(p.Recording) {
			continue
		}
		if p.Removed {
			r.forget(p)
			continue
		}
		if _, err := r.files.Stat(filepath.Join(r.dir, p.ID+".png")); err != nil {
			continue
		}
		if p.Recording != "" {
			if _, err := r.files.Stat(filepath.Join(r.dir, p.ID+"."+p.Recording)); err != nil {
				continue
			}
		}
		p.Name = tidyName(p.Name)
		kept = append(kept, p)
		if len(kept) == picturesMax {
			break
		}
	}
	d.Pictures = kept
	r.draft = d
	if d.Text != "" || len(kept) > 0 {
		theLog.line(zerolog.InfoLevel, "report").Int("characters", len(d.Text)).Int("pictures", len(kept)).Msg("a draft of a report was kept from before")
	}
}

// keep writes the draft whole.
func (r *reporting) keep() error {
	if err := r.files.MkdirAll(r.dir, 0o700); err != nil {
		return fmt.Errorf("the draft could not be kept: %w", err)
	}
	if r.draft.Pictures == nil {
		r.draft.Pictures = []ReportPicture{}
	}
	body, _ := json.MarshalIndent(r.draft, "", "  ")
	if err := engine.WriteAtomic(r.files, filepath.Join(r.dir, "draft.json"), body); err != nil {
		return fmt.Errorf("the draft could not be kept: %w", err)
	}
	return nil
}

// pictureID is what an id of a picture looks like, which is all the
// preview route serves.
var pictureID = regexp.MustCompile(`^p[0-9]{1,9}$`)

// forget lets go of the files a picture or a recording is kept in. They
// are the app's own copies, never the file the person handed over.
func (r *reporting) forget(p ReportPicture) {
	_ = r.files.Remove(filepath.Join(r.dir, p.ID+".png"))
	if isRecordingEnding(p.Recording) {
		_ = r.files.Remove(filepath.Join(r.dir, p.ID+"."+p.Recording))
	}
}

// sentFile is a picture or a recording as it goes in a report: the
// ending it goes under, "png" for a picture, and its bytes.
type sentFile struct {
	ending string
	data   []byte
}

// reportPictures gives the pictures and recordings that go in a report,
// in order, and their ids, for clear. A recording goes as itself, not as
// its still.
func (r *reporting) reportPictures() (ids []string, files []sentFile) {
	if r == nil {
		return nil, nil
	}
	r.mu.Lock()
	defer r.mu.Unlock()
	r.load()
	for _, p := range r.draft.Pictures {
		if p.Removed {
			continue
		}
		ending := "png"
		if p.Recording != "" {
			ending = p.Recording
		}
		if b, err := afero.ReadFile(r.files, filepath.Join(r.dir, p.ID+"."+ending)); err == nil {
			ids = append(ids, p.ID)
			files = append(files, sentFile{ending, b})
		}
	}
	return ids, files
}

// clear lets go of the draft once a report is made from it, so the next
// report starts empty: the text, the pictures removed from it and the
// pictures that went into the report, by their ids. A picture dropped or
// pasted while the report was being made went into none, and stays as
// the start of the next.
func (r *reporting) clear(sent []string) {
	if r == nil {
		return
	}
	r.mu.Lock()
	defer r.mu.Unlock()
	r.load()
	var kept []ReportPicture
	for _, p := range r.draft.Pictures {
		if p.Removed || slices.Contains(sent, p.ID) {
			r.forget(p)
			continue
		}
		kept = append(kept, p)
	}
	r.draft = draftFile{Pictures: kept, Next: r.draft.Next}
	_ = r.keep()
}

// servePicture answers the report window's preview of a picture, by its
// id alone, from the draft's folder and nowhere else.
func (r *reporting) servePicture(w http.ResponseWriter, req *http.Request) {
	id := req.URL.Query().Get("id")
	if r == nil || !pictureID.MatchString(id) {
		http.NotFound(w, req)
		return
	}
	r.mu.Lock()
	r.load()
	known := slices.ContainsFunc(r.draft.Pictures, func(p ReportPicture) bool { return p.ID == id })
	r.mu.Unlock()
	if !known {
		http.NotFound(w, req)
		return
	}
	data, err := afero.ReadFile(r.files, filepath.Join(r.dir, id+".png"))
	if err != nil {
		http.NotFound(w, req)
		return
	}
	w.Header().Set("Content-Type", "image/png")
	w.Header().Set("Cache-Control", "no-store")
	_, _ = w.Write(data)
}

// reportMiddleware serves the report window's previews of its pictures.
func reportMiddleware(r *reporting) func(http.Handler) http.Handler {
	return func(next http.Handler) http.Handler {
		return http.HandlerFunc(func(w http.ResponseWriter, req *http.Request) {
			if req.URL.Path == "/report-picture" {
				r.servePicture(w, req)
				return
			}
			next.ServeHTTP(w, req)
		})
	}
}
