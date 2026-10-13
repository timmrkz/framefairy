package main

import (
	"bytes"
	"encoding/base64"
	"encoding/json"
	"errors"
	"fmt"
	"image"
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

// How many pictures a report takes, and how big a file each may be. They
// are meant for screenshots, which a Mac's screen makes at a few MB.
const (
	picturesMax     = 10
	pictureBytesMax = 10 << 20
	// The most pixels a picture may say it has before it is decoded, a
	// little over two 6K screens, so a file that claims a picture of a
	// thousand times the memory is turned away without being read.
	picturePixelsMax = 50_000_000
)

// ReportPicture is one picture attached to a report, known by its id.
type ReportPicture struct {
	ID string `json:"id"`
	// The file it came from, without its folder, for the tile's title.
	Name   string `json:"name"`
	Width  int    `json:"width"`
	Height int    `json:"height"`
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
	front, makeIt := r.front, r.make
	r.mu.Unlock()
	if front != nil {
		theLog.line(zerolog.InfoLevel, "report").Str("how", how).Msg("the report window is brought forward")
		front()
		return
	}
	if makeIt == nil {
		return
	}
	theLog.line(zerolog.InfoLevel, "report").Str("how", how).Msg("the report window opens")
	var gone sync.Once
	front, shut := makeIt(func() {
		gone.Do(func() {
			r.mu.Lock()
			r.front, r.shut = nil, nil
			r.mu.Unlock()
			theLog.line(zerolog.InfoLevel, "report").Msg("the report window closed")
			if r.told != nil {
				r.told(false)
			}
		})
	})
	r.mu.Lock()
	r.front, r.shut = front, shut
	r.mu.Unlock()
	if r.told != nil {
		r.told(true)
	}
}

// closeWindow closes the report window, when the app's own window closes:
// a report window left alone would keep the app running with nothing to
// work in.
func (r *reporting) closeWindow() {
	if r == nil {
		return
	}
	r.mu.Lock()
	shut := r.shut
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
	if len(text) > reportWordsMax {
		text = text[:reportWordsMax]
	}
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
				SetTitle("Attach pictures").
				CanChooseFiles(true).
				AddFilter("Pictures", "*.png;*.jpg;*.jpeg").
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
		return r.shown(fmt.Sprintf("A report takes %d pictures at most", picturesMax))
	}
	r.draft.Pictures[i].Removed = removed
	how := "a picture was put back"
	if removed {
		how = "a picture was removed"
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

// handed is a picture handed to the report, with why it could not be read.
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
		return nil, errors.New("is not a picture")
	}
	if info.Size() > pictureBytesMax {
		return nil, errTooBig
	}
	return io.ReadAll(io.LimitReader(f, pictureBytesMax+1))
}

// attach attaches the pictures it can, in order, and says why it left out
// the others.
func (r *reporting) attach(how string, all []handed) ReportDraft {
	r.mu.Lock()
	defer r.mu.Unlock()
	r.load()
	var why []string
	added := 0
	for _, g := range all {
		name := tidyName(g.name)
		pic, body, err := g.picture()
		if err == nil && r.count() >= picturesMax {
			err = fmt.Errorf("would be more than the %d pictures a report takes", picturesMax)
		}
		if err == nil {
			r.draft.Next++
			pic.ID = fmt.Sprintf("p%d", r.draft.Next)
			pic.Name = name
			if err = r.files.MkdirAll(r.dir, 0o700); err == nil {
				err = engine.WriteAtomic(r.files, filepath.Join(r.dir, pic.ID+".png"), body)
			}
			if err != nil {
				err = errors.New("could not be kept")
			}
		}
		if err != nil {
			why = append(why, name+" "+err.Error())
			theLog.line(zerolog.WarnLevel, "report").Str("how", how).Str("name", name).Int("bytes", len(g.data)).Err(err).Msg("a picture was left out")
			continue
		}
		r.draft.Pictures = append(r.draft.Pictures, pic)
		added++
		theLog.line(zerolog.InfoLevel, "report").Str("how", how).Str("picture", pic.ID).Str("name", name).
			Int("bytes", len(g.data)).Int("width", pic.Width).Int("height", pic.Height).Msg("a picture was attached")
	}
	if added > 0 {
		if err := r.keep(); err != nil {
			why = append(why, sentence(err.Error()))
		}
	}
	// A reason starts with the name of the file, as it is written.
	return r.shown(strings.Join(why, ". "))
}

// picture decodes a picture handed to the report and writes it again as a
// PNG of its pixels alone, so nothing else the file held, a photo's place
// or its camera, goes with a report.
func (g handed) picture() (ReportPicture, []byte, error) {
	if g.err != nil {
		return ReportPicture{}, nil, g.err
	}
	if len(g.data) > pictureBytesMax {
		return ReportPicture{}, nil, errTooBig
	}
	cfg, format, err := image.DecodeConfig(bytes.NewReader(g.data))
	if err != nil || (format != "png" && format != "jpeg") {
		return ReportPicture{}, nil, errors.New("is not a PNG or JPEG picture")
	}
	if cfg.Width <= 0 || cfg.Height <= 0 || cfg.Width*cfg.Height > picturePixelsMax {
		return ReportPicture{}, nil, errors.New("is too large a picture")
	}
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
	if len(name) > 120 {
		name = name[:120]
	}
	return name
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
	d.Text = strings.ToValidUTF8(d.Text, "")
	if len(d.Text) > reportWordsMax {
		d.Text = d.Text[:reportWordsMax]
	}
	var kept []ReportPicture
	for _, p := range d.Pictures {
		if !pictureID.MatchString(p.ID) {
			continue
		}
		if p.Removed {
			_ = r.files.Remove(filepath.Join(r.dir, p.ID+".png"))
			continue
		}
		if _, err := r.files.Stat(filepath.Join(r.dir, p.ID+".png")); err != nil {
			continue
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

// reportPictures gives the pictures that go in a report, in order, as
// their PNG bytes.
func (r *reporting) reportPictures() [][]byte {
	if r == nil {
		return nil
	}
	r.mu.Lock()
	defer r.mu.Unlock()
	r.load()
	var out [][]byte
	for _, p := range r.draft.Pictures {
		if p.Removed {
			continue
		}
		if b, err := afero.ReadFile(r.files, filepath.Join(r.dir, p.ID+".png")); err == nil {
			out = append(out, b)
		}
	}
	return out
}

// clear lets go of the draft once a report is made from it, so the next
// report starts empty.
func (r *reporting) clear() {
	if r == nil {
		return
	}
	r.mu.Lock()
	defer r.mu.Unlock()
	r.load()
	for _, p := range r.draft.Pictures {
		_ = r.files.Remove(filepath.Join(r.dir, p.ID+".png"))
	}
	r.draft = draftFile{Next: r.draft.Next}
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
