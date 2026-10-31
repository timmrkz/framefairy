package main

import (
	"archive/zip"
	"bytes"
	"encoding/base64"
	"encoding/binary"
	"fmt"
	"hash/crc32"
	"image"
	"image/color"
	"image/jpeg"
	"image/png"
	"io"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"runtime"
	"slices"
	"strings"
	"sync"
	"sync/atomic"
	"testing"
	"unicode/utf8"

	"github.com/spf13/afero"
)

// Bugs in the sidebar opens one report window. Asked again while it is
// open, it brings that window forward rather than making a second, and
// once the window is gone the next ask makes a new one. The app's window
// is told each time the report window opens and closes. Plan row 2.244.
func TestBugsOpensOneReportWindow(t *testing.T) {
	r := newReporting(afero.NewMemMapFs(), "/config/report")
	made, fronted, shut := 0, 0, 0
	var closeIt func()
	r.make = func(closed func()) (func(), func()) {
		made++
		closeIt = closed
		return func() { fronted++ }, func() { shut++; closed() }
	}
	var told []bool
	r.told = func(open bool) { told = append(told, open) }
	s := &FrameFairy{report: r}

	s.OpenReport("bugs")
	s.OpenReport("bugs")
	if made != 1 || fronted != 1 || !s.ReportWindowOpen() {
		t.Fatalf("two asks made %d windows and brought one forward %d times, open %v", made, fronted, s.ReportWindowOpen())
	}
	// The window closes itself, Cmd+W or its red button, and says so
	// twice, the way more than one event can.
	closeIt()
	closeIt()
	if s.ReportWindowOpen() {
		t.Fatal("the window closed and is still taken for open")
	}
	s.OpenReport("bugs")
	if made != 2 {
		t.Fatalf("after the window closed, Bugs made %d windows in all, want 2", made)
	}
	// The app's window closing takes the report window with it.
	r.closeWindow("app")
	if shut != 1 || s.ReportWindowOpen() {
		t.Fatalf("closing the app's window shut the report window %d times, open %v", shut, s.ReportWindowOpen())
	}
	if want := []bool{true, false, true, false}; fmt.Sprint(told) != fmt.Sprint(want) {
		t.Fatalf("the app's window was told %v, want %v", told, want)
	}
}

// Two quick clicks on Bugs are two calls at once, each on a goroutine of
// its own, and still make one window. Found in the review of #217. The
// second click comes while the first is still making its window, which
// waits until that click has had its answer.
func TestTwoQuickClicksOnBugsMakeOneWindow(t *testing.T) {
	r := newReporting(afero.NewMemMapFs(), "/config/report")
	var made atomic.Int32
	making, second := make(chan struct{}), make(chan struct{})
	r.make = func(closed func()) (func(), func()) {
		if made.Add(1) == 1 {
			close(making)
			<-second
		}
		return func() {}, closed
	}
	s := &FrameFairy{report: r}
	var wg sync.WaitGroup
	wg.Go(func() { s.OpenReport("bugs") })
	<-making
	wg.Go(func() {
		s.OpenReport("bugs")
		close(second)
	})
	wg.Wait()
	if n := made.Load(); n != 1 {
		t.Fatalf("two quick clicks on Bugs made %d windows", n)
	}
	if !s.ReportWindowOpen() {
		t.Fatal("the window made is not taken for open")
	}
}

// A window gone before it was made, closed at once, is not taken for open,
// or Bugs would bring forward a window that is not there and nothing would
// open until the app started again. Found in the review of #217.
func TestAWindowGoneBeforeItWasMadeIsNotOpen(t *testing.T) {
	r := newReporting(afero.NewMemMapFs(), "/config/report")
	made := 0
	var told []bool
	r.told = func(open bool) { told = append(told, open) }
	r.make = func(closed func()) (func(), func()) {
		made++
		if made == 1 {
			closed()
		}
		return func() {}, closed
	}
	s := &FrameFairy{report: r}
	s.OpenReport("bugs")
	if s.ReportWindowOpen() {
		t.Fatal("a window gone before it was made is taken for open")
	}
	s.OpenReport("bugs")
	if made != 2 || !s.ReportWindowOpen() {
		t.Fatalf("the next click made %d windows in all, open %v, want 2 and open", made, s.ReportWindowOpen())
	}
	if want := []bool{false, true}; fmt.Sprint(told) != fmt.Sprint(want) {
		t.Fatalf("the app's window was told %v, want %v", told, want)
	}
}

// Pictures are decoded before the draft is taken, so while ten are being
// read, Bugs, the window closing and the text being kept wait for none of
// them. Found in the review of #217. It holds a decode with decoding, a
// package variable, so it runs alone.
func TestNothingWaitsOnAPictureBeingDecoded(t *testing.T) {
	r := newReporting(afero.NewMemMapFs(), "/config/report")
	s := &FrameFairy{report: r}
	hold, held := make(chan struct{}), make(chan struct{})
	was := decoding
	decoding = func() { close(held); <-hold }
	defer func() { decoding = was }()
	done := make(chan ReportDraft)
	// Made here, because pngOf fails the test with t.Fatal, which ends
	// only the goroutine it is called in.
	picture := pngOf(t, 8, 8)
	go func() { done <- r.attach("drop", []handed{{name: "a.png", data: picture}}) }()
	<-held
	asked := make(chan struct{})
	go func() {
		_ = s.ReportWindowOpen()
		_ = s.SaveReportText("words")
		close(asked)
	}()
	// A draft that waits on the decode never answers, and go test's own
	// timeout ends the test and shows where it hung.
	<-asked
	close(hold)
	if d := <-done; len(d.Pictures) != 1 || d.Text != "words" {
		t.Fatalf("after the decode the draft is %+v", d)
	}
}

// A picture dropped or pasted while a report is being made goes into
// none, and is the start of the next report rather than lost. Found in the
// review of #217.
func TestAPictureAttachedWhileAReportIsMadeIsKept(t *testing.T) {
	r := newReporting(afero.NewMemMapFs(), "/config/report")
	s := &FrameFairy{report: r}
	r.attach("drop", []handed{{name: "a.png", data: pngOf(t, 8, 8)}, {name: "b.png", data: pngOf(t, 8, 8)}})
	_ = s.RemovePicture(s.ReportDraft().Pictures[1].ID)
	_ = s.SaveReportText("it froze")
	sent, _ := r.reportPictures()
	r.attach("pasted", []handed{{name: "c.png", data: pngOf(t, 8, 8)}})
	r.clear(sent)
	if d := s.ReportDraft(); names(d) != "c.png" || d.Text != "" {
		t.Fatalf("after the report the draft holds %q and %q, want c.png alone", names(d), d.Text)
	}
	if _, pics := r.reportPictures(); len(pics) != 1 {
		t.Fatalf("the next report would hold %d pictures, want 1", len(pics))
	}
}

// Text is cut between two letters, never in the middle of one, when it is
// kept, read again and put in a report. Found in the review of #217.
func TestTextIsCutBetweenLetters(t *testing.T) {
	files := afero.NewMemMapFs()
	s := &FrameFairy{report: newReporting(files, "/config/report")}
	long := strings.Repeat("ä", reportWordsMax/2) + "x" + "ä"
	if err := s.SaveReportText("x" + long); err != nil {
		t.Fatal(err)
	}
	kept := s.ReportDraft().Text
	if !utf8.ValidString(kept) || len(kept) > reportWordsMax || len(kept) < reportWordsMax-1 {
		t.Fatalf("the kept text is %d bytes, valid %v", len(kept), utf8.ValidString(kept))
	}
	// A draft written by hand, too long, read again.
	body := fmt.Sprintf(`{"text": %q, "pictures": []}`, "x"+long)
	if err := afero.WriteFile(files, "/config/report/draft.json", []byte(body), 0o600); err != nil {
		t.Fatal(err)
	}
	s = &FrameFairy{report: newReporting(files, "/config/report")}
	if read := s.ReportDraft().Text; !utf8.ValidString(read) || len(read) > reportWordsMax {
		t.Fatalf("the text read again is %d bytes, valid %v", len(read), utf8.ValidString(read))
	}
	if got := cut("aä", 2); got != "a" {
		t.Fatalf("cutting aä at 2 bytes gave %q, want a", got)
	}
}

// The report window opens to the right of the app's window where the
// screen has room, over the middle of it where it has not, and never off
// the screen.
func TestTheReportWindowOpensBesideTheApp(t *testing.T) {
	work := image.Rect(0, 25, 1728, 1117)
	for _, c := range []struct {
		name string
		app  image.Rectangle
		want image.Point
	}{
		{"room on the right", image.Rect(40, 60, 1000, 700), image.Pt(1016, 60)},
		{"no room on the right", image.Rect(200, 100, 1480, 920), image.Pt(560, 190)},
		{"the app filling the screen", image.Rect(0, 25, 1728, 1117), image.Pt(584, 251)},
		{"the app low on the screen", image.Rect(40, 900, 1000, 1500), image.Pt(1016, 477)},
	} {
		if got := beside(c.app, work, reportWidth, reportHeight); got != c.want {
			t.Errorf("%s: the report window opens at %v, want %v", c.name, got, c.want)
		}
	}
}

// What is written in the report and the pictures attached are a draft that
// outlives the window and the app, and a report made from it lets go of
// it, so the next one starts empty.
func TestTheDraftOutlivesTheWindowUntilAReportIsMade(t *testing.T) {
	files := afero.NewMemMapFs()
	dir := "/config/report"
	s := &FrameFairy{report: newReporting(files, dir)}
	if err := s.SaveReportText("The picture froze at 46 s."); err != nil {
		t.Fatal(err)
	}
	d := s.report.attach("drop", []handed{{name: "Screenshot 1.png", data: pngOf(t, 40, 30)}})
	if d.Error != "" || len(d.Pictures) != 1 {
		t.Fatalf("one picture dropped gave %+v", d)
	}

	// The app starts again.
	s = &FrameFairy{report: newReporting(files, dir)}
	d = s.ReportDraft()
	if d.Text != "The picture froze at 46 s." || len(d.Pictures) != 1 || d.Pictures[0].Name != "Screenshot 1.png" ||
		d.Pictures[0].Width != 40 || d.Pictures[0].Height != 30 {
		t.Fatalf("after a restart the draft is %+v", d)
	}

	sent, _ := s.report.reportPictures()
	s.report.clear(sent)
	if d := s.ReportDraft(); d.Text != "" || len(d.Pictures) != 0 {
		t.Fatalf("after a report the draft is %+v, want it empty", d)
	}
	s = &FrameFairy{report: newReporting(files, dir)}
	if d := s.ReportDraft(); d.Text != "" || len(d.Pictures) != 0 {
		t.Fatalf("after a report and a restart the draft is %+v, want it empty", d)
	}
	if left, _ := afero.Glob(files, filepath.Join(dir, "*.png")); len(left) != 0 {
		t.Fatalf("a report left pictures behind: %v", left)
	}
}

// A picture goes in a report as a PNG of its pixels alone. A JPEG photo's
// place and camera, in its EXIF, and a PNG's text are left behind.
func TestAPictureGoesWithoutWhatElseTheFileHeld(t *testing.T) {
	r := newReporting(afero.NewMemMapFs(), "/config/report")
	photo := withEXIF(t, jpegOf(t, 64, 48), "GPS 52.52N 13.40E")
	shot := withText(t, pngOf(t, 32, 20), "Author", "Tim")
	d := r.attach("picked", []handed{{name: "photo.jpg", data: photo}, {name: "shot.png", data: shot}})
	if d.Error != "" || len(d.Pictures) != 2 {
		t.Fatalf("a photo and a screenshot gave %+v", d)
	}
	_, pics := r.reportPictures()
	if len(pics) != 2 {
		t.Fatalf("the report holds %d pictures, want 2", len(pics))
	}
	for i, p := range pics {
		cfg, format, err := image.DecodeConfig(bytes.NewReader(p.data))
		if err != nil || format != "png" {
			t.Fatalf("picture %d is %q, %v, want a PNG", i+1, format, err)
		}
		if want := [2]int{64, 48}; i == 1 {
			want = [2]int{32, 20}
			if [2]int{cfg.Width, cfg.Height} != want {
				t.Fatalf("picture %d is %dx%d, want %v", i+1, cfg.Width, cfg.Height, want)
			}
		} else if [2]int{cfg.Width, cfg.Height} != want {
			t.Fatalf("picture %d is %dx%d, want %v", i+1, cfg.Width, cfg.Height, want)
		}
		if bytes.Contains(p.data, []byte("GPS")) || bytes.Contains(p.data, []byte("Author")) || bytes.Contains(p.data, []byte("Exif")) {
			t.Fatalf("picture %d still holds what the file held beside its pixels", i+1)
		}
	}
}

// A report takes ten pictures of PNG or JPEG or recordings, each at most 10 MB. Anything
// else is left out with the reason, and what could be attached still is.
func TestPicturesAreLeftOutWithTheReason(t *testing.T) {
	t.Parallel()
	r := newReporting(afero.NewMemMapFs(), "/config/report")
	var many []handed
	for i := range picturesMax + 1 {
		many = append(many, handed{name: fmt.Sprintf("%d.png", i+1), data: pngOf(t, 8, 8)})
	}
	d := r.attach("drop", many)
	if len(d.Pictures) != picturesMax || !strings.Contains(d.Error, "11.png cannot be attached because it would be more than the 10 pictures and recordings") {
		t.Fatalf("eleven pictures gave %d with %q", len(d.Pictures), d.Error)
	}

	r = newReporting(afero.NewMemMapFs(), "/config/report")
	d = r.attach("drop", []handed{
		{name: "notes.txt", data: []byte("not a picture")},
		{name: "big.png", data: make([]byte, pictureBytesMax+1)},
		{name: "huge.png", data: claimsSize(t, 100_000, 100_000)},
		{name: "screen.heic", data: []byte("\x00\x00\x00\x18ftypheic")},
		{name: "fine.png", data: pngOf(t, 8, 8)},
	})
	for _, why := range []string{"notes.txt cannot be attached because it is not a picture or a screen recording", "big.png cannot be attached because it is bigger than 10 MB",
		"huge.png cannot be attached because it is too large a picture", "screen.heic cannot be attached because it is not a picture or a screen recording"} {
		if !strings.Contains(d.Error, why) {
			t.Errorf("the reason %q does not say %q", d.Error, why)
		}
	}
	if len(d.Pictures) != 1 || d.Pictures[0].Name != "fine.png" {
		t.Fatalf("the one good picture among them gave %+v", d.Pictures)
	}

	// A paste too big is turned away before it is decoded.
	s := &FrameFairy{report: r}
	huge := base64.StdEncoding.EncodeToString(make([]byte, pictureBytesMax+10))
	if d := s.AttachPasted("pasted.png", huge); !strings.Contains(d.Error, "bigger than 10 MB") || len(d.Pictures) != 1 {
		t.Fatalf("a paste of over 10 MB gave %+v", d)
	}
	if d := s.AttachPasted("pasted.png", base64.StdEncoding.EncodeToString(pngOf(t, 4, 4))); d.Error != "" || len(d.Pictures) != 2 {
		t.Fatalf("a pasted picture gave %+v", d)
	}
}

// A removed picture goes in no report and counts toward no limit, and put
// back it is where it was. Removed pictures a run left are let go of at
// the next start.
func TestARemovedPictureCanBePutBack(t *testing.T) {
	files := afero.NewMemMapFs()
	s := &FrameFairy{report: newReporting(files, "/config/report")}
	d := s.report.attach("drop", []handed{
		{name: "a.png", data: pngOf(t, 8, 8)}, {name: "b.png", data: pngOf(t, 9, 9)}, {name: "c.png", data: pngOf(t, 10, 10)},
	})
	b := d.Pictures[1].ID
	if d := s.RemovePicture(b); names(d) != "a.png c.png" {
		t.Fatalf("with b removed the draft holds %s", names(d))
	}
	if _, pics := s.report.reportPictures(); len(pics) != 2 {
		n := len(pics)
		t.Fatalf("a report would hold %d pictures with one of three removed", n)
	}
	if d := s.PutBackPicture(b); names(d) != "a.png b.png c.png" {
		t.Fatalf("put back, the draft holds %s", names(d))
	}
	s.RemovePicture(b)
	s = &FrameFairy{report: newReporting(files, "/config/report")}
	if d := s.ReportDraft(); names(d) != "a.png c.png" {
		t.Fatalf("after a restart the draft holds %s", names(d))
	}
	if _, err := files.Stat("/config/report/" + b + ".png"); err == nil {
		t.Fatal("a picture removed before a restart is still kept")
	}
}

// The report window's previews are served by id from the draft and from
// nowhere else, so no path the page names is ever read.
func TestThePreviewServesOnlyTheDraftsPictures(t *testing.T) {
	files := afero.NewMemMapFs()
	r := newReporting(files, "/config/report")
	d := r.attach("drop", []handed{{name: "a.png", data: pngOf(t, 8, 8)}})
	_ = afero.WriteFile(files, "/config/report/p99.png", pngOf(t, 8, 8), 0o600)
	_ = afero.WriteFile(files, "/config/settings.json", []byte("{}"), 0o600)
	handler := reportMiddleware(r)(http.NotFoundHandler())
	get := func(q string) *httptest.ResponseRecorder {
		rec := httptest.NewRecorder()
		handler.ServeHTTP(rec, httptest.NewRequest("GET", "/report-picture?"+q, nil))
		return rec
	}
	if rec := get("id=" + d.Pictures[0].ID); rec.Code != 200 || rec.Header().Get("Content-Type") != "image/png" {
		t.Fatalf("the draft's picture answered %d %s", rec.Code, rec.Header().Get("Content-Type"))
	}
	for _, q := range []string{"id=p99", "id=../settings.json", "id=p1/../../settings", "id=", "path=/config/settings.json"} {
		if rec := get(q); rec.Code != 404 {
			t.Errorf("%s answered %d, want 404", q, rec.Code)
		}
	}
}

// A report holds the draft's pictures in its order, as pictures/1.png and
// on, and a report made lets go of the draft and closes the window. Without a video named it
// names the video the last act did.
//
// On the real disk: the report is a zip on the Desktop, and holds the log
// lumberjack keeps on the disk.
func TestAReportHoldsThePictures(t *testing.T) {
	home := t.TempDir()
	t.Setenv("HOME", home)
	if err := os.Mkdir(filepath.Join(home, "Desktop"), 0o755); err != nil {
		t.Fatal(err)
	}
	testLog(t)
	was := showReport
	showReport = func(string) error { return nil }
	t.Cleanup(func() { showReport = was })

	st := &store{dir: t.TempDir(), files: disk, settings: defaultSettings()}
	f := &FrameFairy{store: st, report: newReporting(afero.NewMemMapFs(), "/config/report")}
	f.Acted(Act{ID: "a1", What: "picked a video", Video: filepath.Join(home, "Movies", "start.mp4")})
	f.Acted(Act{ID: "r1", What: "clicked Report"})
	f.report.attach("drop", []handed{{name: "a.png", data: pngOf(t, 12, 8)}, {name: "b.png", data: pngOf(t, 8, 12)}})
	shut := 0
	f.report.make = func(closed func()) (func(), func()) { return func() {}, func() { shut++; closed() } }
	f.OpenReport("bugs")
	path, err := f.ReportProblem("It froze.", "")
	if err != nil {
		t.Fatal(err)
	}
	// Finder shows the report, and the window it was made in closes, as
	// Tim asked.
	if shut != 1 || f.ReportWindowOpen() {
		t.Fatalf("after the report the window was closed %d times, open %v", shut, f.ReportWindowOpen())
	}
	z, err := zip.OpenReader(path)
	if err != nil {
		t.Fatal(err)
	}
	defer z.Close()
	got := map[string][]byte{}
	for _, file := range z.File {
		rc, _ := file.Open()
		got[file.Name], _ = io.ReadAll(rc)
		rc.Close()
	}
	for i, want := range [][2]int{{12, 8}, {8, 12}} {
		name := fmt.Sprintf("Frame Fairy report/pictures/%d.png", i+1)
		cfg, err := png.DecodeConfig(bytes.NewReader(got[name]))
		if err != nil || [2]int{cfg.Width, cfg.Height} != want {
			t.Fatalf("%s is %dx%d, %v, want %v", name, cfg.Width, cfg.Height, err, want)
		}
	}
	if !bytes.Contains(got["Frame Fairy report/about.json"], []byte(`"video": "start.mp4"`)) {
		t.Fatalf("the report does not name the video last worked on: %s", got["Frame Fairy report/about.json"])
	}
	if d := f.ReportDraft(); len(d.Pictures) != 0 || d.Text != "" {
		t.Fatalf("after the report the draft is %+v", d)
	}
}

// Files picked in the open box or dropped on the report window are read
// from where they are, and one that is not a file, or too big, is left
// out before it is read.
//
// On the real disk: the open box and a drop hand over paths on the disk.
func TestPicturesPickedAreReadFromWhereTheyAre(t *testing.T) {
	dir := t.TempDir()
	shot := filepath.Join(dir, "Screenshot.png")
	if err := os.WriteFile(shot, pngOf(t, 16, 9), 0o600); err != nil {
		t.Fatal(err)
	}
	big := filepath.Join(dir, "big.png")
	if err := os.WriteFile(big, make([]byte, pictureBytesMax+1), 0o600); err != nil {
		t.Fatal(err)
	}
	s := &FrameFairy{report: newReporting(afero.NewMemMapFs(), "/config/report")}
	s.report.pick = func() ([]string, error) { return []string{shot, big, dir, filepath.Join(dir, "gone.png")}, nil }
	d := s.AttachPictures()
	if names(d) != "Screenshot.png" {
		t.Fatalf("picked, the draft holds %s", names(d))
	}
	for _, why := range []string{"big.png cannot be attached because it is bigger than 10 MB", filepath.Base(dir) + " cannot be attached because it is not a picture", "gone.png cannot be attached because it could not be read"} {
		if !strings.Contains(d.Error, why) {
			t.Errorf("the reason %q does not say %q", d.Error, why)
		}
	}
	var told []ReportDraft
	s.report.said = func(d ReportDraft) { told = append(told, d) }
	s.report.dropped([]string{shot})
	if len(told) != 1 || len(told[0].Pictures) != 2 {
		t.Fatalf("a drop told the window %+v", told)
	}
}

// Clear empties the draft, the text and every picture, removed ones too.
// Its pictures can be put back, the way Cmd+Z does it, and after a
// restart the app's copies are gone and the draft stays empty. Issue #415.
func TestClearEmptiesTheDraft(t *testing.T) {
	files := afero.NewMemMapFs()
	s := &FrameFairy{report: newReporting(files, "/config/report")}
	if err := s.SaveReportText("It froze."); err != nil {
		t.Fatal(err)
	}
	d := s.report.attach("drop", []handed{{name: "a.png", data: pngOf(t, 8, 8)}, {name: "b.png", data: pngOf(t, 9, 9)}})
	ids := []string{d.Pictures[0].ID, d.Pictures[1].ID}
	s.RemovePicture(ids[1])
	if d := s.ClearReport(); d.Text != "" || len(d.Pictures) != 0 || d.Error != "" {
		t.Fatalf("cleared, the draft is %+v", d)
	}
	if _, pics := s.report.reportPictures(); len(pics) != 0 {
		t.Fatalf("a report after Clear would hold %d pictures", len(pics))
	}
	if d := s.PutBackPicture(ids[0]); names(d) != "a.png" {
		t.Fatalf("put back after Clear, the draft holds %s", names(d))
	}
	s.ClearReport()
	s = &FrameFairy{report: newReporting(files, "/config/report")}
	if d := s.ReportDraft(); d.Text != "" || len(d.Pictures) != 0 {
		t.Fatalf("after a restart the cleared draft is %+v", d)
	}
	for _, id := range ids {
		if _, err := files.Stat("/config/report/" + id + ".png"); err == nil {
			t.Fatalf("picture %s is still kept after Clear and a restart", id)
		}
	}
	// A picture attached after Clear gets an id of its own.
	if d := s.report.attach("drop", []handed{{name: "c.png", data: pngOf(t, 8, 8)}}); len(d.Pictures) != 1 || slices.Contains(ids, d.Pictures[0].ID) {
		t.Fatalf("attached after Clear, the draft is %+v", d)
	}
}

// Clear while pictures are dropped and text is typed, from several at
// once: none of it races, and every picture the draft ends with still
// has its file. Issue #415.
func TestClearWhileTheDraftChanges(t *testing.T) {
	t.Parallel()
	files := afero.NewMemMapFs()
	s := &FrameFairy{report: newReporting(files, "/config/report")}
	pic := pngOf(t, 8, 8)
	var wg sync.WaitGroup
	for i := range 6 {
		wg.Go(func() {
			for j := range 5 {
				switch (i + j) % 3 {
				case 0:
					s.report.attach("drop", []handed{{name: "a.png", data: pic}})
				case 1:
					_ = s.SaveReportText(fmt.Sprintf("words %d %d", i, j))
				case 2:
					s.ClearReport()
				}
			}
		})
	}
	wg.Wait()
	d := s.ReportDraft()
	if len(d.Pictures) > picturesMax {
		t.Fatalf("the draft holds %d pictures", len(d.Pictures))
	}
	for _, p := range d.Pictures {
		if _, err := files.Stat("/config/report/" + p.ID + ".png"); err != nil {
			t.Fatalf("picture %s is in the draft without its file", p.ID)
		}
	}
}

// The report's draft is reached from the window, a drop and the app's
// window at once: drops arrive on their own goroutine while the person
// types, removes and puts back. None of it races, and the draft ends with
// every picture that was given.
func TestTheDraftFromSeveralAtOnce(t *testing.T) {
	t.Parallel()
	r := newReporting(afero.NewMemMapFs(), "/config/report")
	// How many report windows are open at once, which is never more than
	// one, however many ask at the same time.
	var open, most atomic.Int32
	r.make = func(closed func()) (func(), func()) {
		n := open.Add(1)
		for m := most.Load(); n > m && !most.CompareAndSwap(m, n); m = most.Load() {
		}
		// Others get their turn while this one is made.
		runtime.Gosched()
		var once sync.Once
		return func() {}, func() { once.Do(func() { open.Add(-1); closed() }) }
	}
	s := &FrameFairy{report: r}
	pic := pngOf(t, 8, 8)
	var wg sync.WaitGroup
	for i := range 8 {
		wg.Go(func() {
			for j := range 5 {
				switch (i + j) % 6 {
				case 0:
					r.attach("drop", []handed{{name: "a.png", data: pic}})
				case 1:
					_ = s.SaveReportText(fmt.Sprintf("words %d %d", i, j))
				case 2:
					if d := s.ReportDraft(); len(d.Pictures) > 0 {
						s.RemovePicture(d.Pictures[0].ID)
						s.PutBackPicture(d.Pictures[0].ID)
					}
				case 3:
					s.OpenReport("bugs")
					_ = s.ReportWindowOpen()
				case 4:
					r.closeWindow("app")
					r.sawVideo("/videos/a.mp4")
				case 5:
					_, _ = r.reportPictures()
					_ = r.lastVideo()
				}
			}
		})
	}
	wg.Wait()
	if d := s.ReportDraft(); len(d.Pictures) == 0 || len(d.Pictures) > picturesMax {
		t.Fatalf("after it all the draft holds %d pictures", len(d.Pictures))
	}
	if m := most.Load(); m > 1 {
		t.Fatalf("%d report windows were open at once", m)
	}
}

func names(d ReportDraft) string {
	var n []string
	for _, p := range d.Pictures {
		n = append(n, p.Name)
	}
	return strings.Join(n, " ")
}

func pngOf(t *testing.T, w, h int) []byte {
	t.Helper()
	img := image.NewRGBA(image.Rect(0, 0, w, h))
	for y := range h {
		for x := range w {
			img.Set(x, y, color.RGBA{uint8(x * 7), uint8(y * 5), 120, 255})
		}
	}
	var b bytes.Buffer
	if err := png.Encode(&b, img); err != nil {
		t.Fatal(err)
	}
	return b.Bytes()
}

func jpegOf(t *testing.T, w, h int) []byte {
	t.Helper()
	img := image.NewRGBA(image.Rect(0, 0, w, h))
	var b bytes.Buffer
	if err := jpeg.Encode(&b, img, nil); err != nil {
		t.Fatal(err)
	}
	return b.Bytes()
}

// withEXIF puts an APP1 segment with what after a JPEG's start, the place
// a camera writes its EXIF.
func withEXIF(t *testing.T, j []byte, what string) []byte {
	t.Helper()
	body := append([]byte("Exif\x00\x00"), what...)
	seg := []byte{0xFF, 0xE1, byte((len(body) + 2) >> 8), byte(len(body) + 2)}
	return append(append(append([]byte{}, j[:2]...), append(seg, body...)...), j[2:]...)
}

// withText puts a tEXt chunk after a PNG's header chunk.
func withText(t *testing.T, p []byte, key, value string) []byte {
	t.Helper()
	data := append(append([]byte(key), 0), value...)
	return append(append(append([]byte{}, p[:33]...), chunk("tEXt", data)...), p[33:]...)
}

// claimsSize is a PNG whose header says it is w by h, with no pixels.
func claimsSize(t *testing.T, w, h uint32) []byte {
	t.Helper()
	ihdr := make([]byte, 13)
	binary.BigEndian.PutUint32(ihdr[0:], w)
	binary.BigEndian.PutUint32(ihdr[4:], h)
	ihdr[8], ihdr[9] = 8, 6
	return append(append([]byte("\x89PNG\r\n\x1a\n"), chunk("IHDR", ihdr)...), chunk("IEND", nil)...)
}

func chunk(kind string, data []byte) []byte {
	out := binary.BigEndian.AppendUint32(nil, uint32(len(data)))
	out = append(out, kind...)
	out = append(out, data...)
	return binary.BigEndian.AppendUint32(out, crc32.ChecksumIEEE(append([]byte(kind), data...)))
}

// The Report button of a toast opens Report a Problem with the toast's
// words in its field, after what was written there. An open window is
// handed them to write in, and the draft it keeps is not written behind
// its back, since the window's field would save over it. Issue #311.
func TestAToastsReportButtonWritesItsWordsInTheReport(t *testing.T) {
	r := newReporting(afero.NewMemMapFs(), "/config/report")
	made := 0
	r.make = func(closed func()) (func(), func()) {
		made++
		return func() {}, func() { closed() }
	}
	var handed []string
	r.about = func(text string) { handed = append(handed, text) }
	s := &FrameFairy{report: r}

	if err := s.SaveReportText("The search stopped.\n"); err != nil {
		t.Fatal(err)
	}
	s.ReportAbout("Anthropic is busy.")
	if d := s.ReportDraft(); d.Text != "The search stopped.\n\nAnthropic is busy." {
		t.Fatalf("the report says %q", d.Text)
	}
	if made != 1 || len(handed) != 0 {
		t.Fatalf("%d windows made, %v handed to one", made, handed)
	}

	s.ReportAbout("Could not reach Anthropic.")
	if made != 1 || fmt.Sprint(handed) != "[Could not reach Anthropic.]" {
		t.Fatalf("with the window open: %d windows made, %v handed to it", made, handed)
	}
	if d := s.ReportDraft(); d.Text != "The search stopped.\n\nAnthropic is busy." {
		t.Fatalf("the draft was written behind the open window's back: %q", d.Text)
	}
}

// heldMove holds the move of a file onto its name, the last step of
// engine.WriteAtomic, until free is closed, and says at when it is there.
// moved is set once the move is done.
type heldMove struct {
	afero.Fs
	at, free chan struct{}
	moved    atomic.Bool
}

func (h *heldMove) Rename(from, to string) error {
	close(h.at)
	<-h.free
	err := h.Fs.Rename(from, to)
	h.moved.Store(true)
	return err
}

// The bridge ends the report of the app a walk ran before it removes the
// config folder: a save the page sent as the walk ended is let finish,
// whole, and a save after it writes nothing, so no file is made in the
// folder while it is removed and no draft lands in the next walk's.
// Issue #632, where a draft written during the reset failed it with
// "directory not empty".
func TestAnEndedReportWritesNothingMore(t *testing.T) {
	files := afero.NewMemMapFs()
	held := &heldMove{Fs: files, at: make(chan struct{}), free: make(chan struct{})}
	r := newReporting(held, "/config/report")
	s := &FrameFairy{report: r}

	saved := make(chan error, 1)
	go func() { saved <- s.SaveReportText("typed as the walk ended") }()
	<-held.at
	ended := make(chan bool, 1)
	go func() {
		endReport(r)
		ended <- held.moved.Load()
	}()
	close(held.free)
	if err := <-saved; err != nil {
		t.Fatalf("the save in hand failed: %v", err)
	}
	if moved := <-ended; !moved {
		t.Fatal("the report ended while a save was still writing its draft")
	}

	if err := s.SaveReportText("a save that waited on the lock"); err == nil {
		t.Fatal("a save after the report ended said it was kept")
	}
	if d := s.AttachPasted("shot.png", base64.StdEncoding.EncodeToString(pngOf(t, 4, 4))); len(d.Pictures) != 0 {
		t.Fatalf("a picture pasted after the report ended was kept: %+v", d.Pictures)
	}
	names, err := afero.ReadDir(files, "/config/report")
	if err != nil {
		t.Fatal(err)
	}
	if len(names) != 1 || names[0].Name() != "draft.json" {
		var got []string
		for _, n := range names {
			got = append(got, n.Name())
		}
		t.Fatalf("the report's folder holds %v, want only draft.json", got)
	}
	if again := newReporting(files, "/config/report"); (&FrameFairy{report: again}).ReportDraft().Text != "typed as the walk ended" {
		t.Fatal("the save in hand was not kept whole")
	}
}
