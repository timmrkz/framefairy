package main

import (
	"archive/zip"
	"bytes"
	"encoding/json"
	"image/png"
	"io"
	"math"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"testing"

	"github.com/spf13/afero"

	"framefairy/internal/ffmpegtest"
)

// What a file is, is told by its first bytes: a MOV or an MP4, a WebM or
// an MKV is read as a recording, a HEIC or AVIF picture in the same box is
// not, and neither is anything else.
func TestWhatARecordingIsIsToldByItsFirstBytes(t *testing.T) {
	box := func(kind, brand string) []byte { return []byte("\x00\x00\x00\x18" + kind + brand + "\x00\x00\x00\x00") }
	for _, c := range []struct {
		name string
		data []byte
		want string
	}{
		{"Screen Recording.mov", box("ftyp", "qt  "), "mov"},
		{"phone.mp4", box("ftyp", "isom"), "mov"},
		{"old.mov", box("wide", "\x00\x00\x00\x00"), "mov"},
		{"recording.webm", []byte{0x1a, 0x45, 0xdf, 0xa3, 0x9f, 0x42, 0x86, 0x81}, "matroska"},
		{"screen.heic", box("ftyp", "heic"), ""},
		{"screen.avif", box("ftyp", "avif"), ""},
		{"notes.txt", []byte("#EXTM3U\nhttp://example.com/a.ts\n"), ""},
		{"short", []byte("ftyp"), ""},
	} {
		if got := sniffRecording(c.data); got != c.want {
			t.Errorf("%s is read as %q, want %q", c.name, got, c.want)
		}
	}
}

// A screen recording of each kind the usual recorders make is attached:
// its size and length are known, the tile has a still of its first
// frame, and the copy kept holds its picture and its sound and nothing
// the file said about itself, not the place, the title or the device. A
// report holds the copy under recordings/, and a recording removed and
// let go of leaves no file behind.
//
// On the real disk: ffmpeg makes the recordings, and ffprobe and ffmpeg
// read them.
func TestScreenRecordingsAreAttachedWithoutWhatElseTheyHeld(t *testing.T) {
	ffmpegtest.Need(t)
	t.Parallel()
	dir := t.TempDir()
	for _, c := range []struct {
		name   string
		args   []string
		ending string
	}{
		{"Screen Recording 2026-10-15 at 14.02.11.mov", []string{"-c:v", "mpeg4", "-c:a", "aac", "-f", "mov"}, "mov"},
		{"phone.mp4", []string{"-c:v", "mpeg4", "-c:a", "aac", "-f", "mp4"}, "mp4"},
		// The ffmpeg the app ships, which the Mac runner has, reads WebM
		// and makes none, so this one was made once, with VP9 and Opus and
		// a place, the way recordingOf makes the others, and is kept.
		{"gnome.webm", nil, "webm"},
		{"obs.mkv", []string{"-c:v", "mpeg4", "-c:a", "aac", "-f", "matroska"}, "mkv"},
	} {
		t.Run(c.ending, func(t *testing.T) {
			path := filepath.Join(dir, c.name)
			if c.args == nil {
				kept, err := os.ReadFile(filepath.Join("testdata", c.name))
				if err != nil {
					t.Fatal(err)
				}
				if err := os.WriteFile(path, kept, 0o600); err != nil {
					t.Fatal(err)
				}
			} else {
				recordingOf(t, path, c.args...)
			}
			if c.ending == "mp4" {
				// A phone held upright, whose picture lies on its side.
				turned := filepath.Join(dir, "upright.mp4")
				if out, err := exec.Command("ffmpeg", "-loglevel", "error", "-y", "-display_rotation", "90", "-i", path,
					"-map", "0", "-c", "copy", "-movflags", "use_metadata_tags", turned).CombinedOutput(); err != nil {
					t.Fatalf("ffmpeg: %v\n%s", err, out)
				}
				path = turned
			}
			data, err := os.ReadFile(path)
			if err != nil {
				t.Fatal(err)
			}
			if !bytes.Contains(data, []byte("52.52")) {
				t.Fatalf("the test recording %s does not hold the place it should be copied without", c.name)
			}
			r := newReporting(afero.NewMemMapFs(), "/config/report")
			d := r.attach("drop", []handed{{name: c.name, data: data}})
			if d.Error != "" || len(d.Pictures) != 1 {
				t.Fatalf("%s gave %+v", c.name, d)
			}
			p := d.Pictures[0]
			want := [2]int{160, 90}
			if c.ending == "mp4" {
				want = [2]int{90, 160}
			}
			if p.Recording != c.ending || [2]int{p.Width, p.Height} != want || math.Abs(p.Seconds-1) > 0.1 {
				t.Fatalf("%s is kept as %+v, want a recording of %v kept as %s, one second long", c.name, p, want, c.ending)
			}
			still, err := afero.ReadFile(r.files, "/config/report/"+p.ID+".png")
			if err != nil {
				t.Fatal(err)
			}
			cfg, err := png.DecodeConfig(bytes.NewReader(still))
			if err != nil || [2]int{cfg.Width, cfg.Height} != want {
				t.Fatalf("the still of %s is %dx%d, %v, want %v", c.name, cfg.Width, cfg.Height, err, want)
			}
			ids, sent := r.reportPictures()
			if len(sent) != 1 || sent[0].ending != c.ending {
				t.Fatalf("a report would hold %+v", sent)
			}
			copied := filepath.Join(t.TempDir(), "copy."+c.ending)
			if err := os.WriteFile(copied, sent[0].data, 0o600); err != nil {
				t.Fatal(err)
			}
			kinds, tags := probeTags(t, copied)
			if kinds != "video audio" {
				t.Fatalf("the copy of %s holds %q, want its picture and its sound", c.name, kinds)
			}
			if bytes.Contains(sent[0].data, []byte("52.52")) || bytes.Contains(sent[0].data, []byte("Tim's desk")) {
				t.Fatalf("the copy of %s still holds the place or the title it was recorded with", c.name)
			}
			for _, tag := range []string{"location", "title", "comment", "com.apple.quicktime.location.ISO6709"} {
				if _, ok := tags[tag]; ok {
					t.Fatalf("the copy of %s still says its %s: %v", c.name, tag, tags)
				}
			}
			// Sent, it is let go of with its still.
			r.clear(ids)
			for _, name := range []string{p.ID + ".png", p.ID + "." + c.ending} {
				if ok, _ := afero.Exists(r.files, "/config/report/"+name); ok {
					t.Fatalf("%s is still kept after the report", name)
				}
			}
		})
	}
}

// A file that starts like a recording and is none, and a recording with
// no picture in it, are left out with the reason, and a picture beside
// them is still attached.
//
// On the real disk: ffprobe reads what was handed over.
func TestWhatIsNoRecordingIsLeftOut(t *testing.T) {
	ffmpegtest.Need(t)
	t.Parallel()
	sound := filepath.Join(t.TempDir(), "voice.m4a")
	if out, err := exec.Command("ffmpeg", "-loglevel", "error", "-y", "-f", "lavfi", "-i", "sine=f=440:d=1",
		"-c:a", "aac", "-f", "mp4", sound).CombinedOutput(); err != nil {
		t.Fatalf("ffmpeg: %v\n%s", err, out)
	}
	voice, err := os.ReadFile(sound)
	if err != nil {
		t.Fatal(err)
	}
	r := newReporting(afero.NewMemMapFs(), "/config/report")
	d := r.attach("drop", []handed{
		{name: "broken.mov", data: []byte("\x00\x00\x00\x18ftypqt  \x00\x00\x00\x00 and then nothing")},
		{name: "voice.m4a", data: voice},
		{name: "fine.png", data: pngOf(t, 8, 8)},
	})
	for _, why := range []string{"broken.mov is not a picture or a screen recording", "voice.m4a is not a picture or a screen recording"} {
		if !strings.Contains(d.Error, why) {
			t.Errorf("the reason %q does not say %q", d.Error, why)
		}
	}
	if len(d.Pictures) != 1 || d.Pictures[0].Name != "fine.png" {
		t.Fatalf("the one good picture among them gave %+v", d.Pictures)
	}
}

// A report holds a recording under recordings/, stored as it is, and the
// pictures under pictures/, each counted on its own.
//
// On the real disk: ffmpeg makes the recording, and the report is saved
// on the Desktop.
func TestAReportHoldsTheRecordings(t *testing.T) {
	ffmpegtest.Need(t)
	// Not side by side with the others: it sets HOME for the whole process,
	// and showReport, a package variable.
	home := t.TempDir()
	t.Setenv("HOME", home)
	if err := os.Mkdir(filepath.Join(home, "Desktop"), 0o755); err != nil {
		t.Fatal(err)
	}
	testLog(t)
	was := showReport
	showReport = func(string) error { return nil }
	t.Cleanup(func() { showReport = was })
	path := filepath.Join(t.TempDir(), "Screen Recording.mov")
	recordingOf(t, path, "-c:v", "mpeg4", "-c:a", "aac", "-f", "mov")
	film, err := os.ReadFile(path)
	if err != nil {
		t.Fatal(err)
	}
	st := &store{dir: t.TempDir(), files: disk, settings: defaultSettings()}
	f := &FrameFairy{store: st, report: newReporting(afero.NewMemMapFs(), "/config/report")}
	d := f.report.attach("drop", []handed{{name: "a.png", data: pngOf(t, 12, 8)}, {name: "Screen Recording.mov", data: film}})
	if d.Error != "" {
		t.Fatal(d.Error)
	}
	made, err := f.ReportProblem("It froze.", "")
	if err != nil {
		t.Fatal(err)
	}
	z, err := zip.OpenReader(made)
	if err != nil {
		t.Fatal(err)
	}
	defer z.Close()
	got := map[string]*zip.File{}
	for _, file := range z.File {
		got[file.Name] = file
	}
	if got["Frame Fairy report/pictures/1.png"] == nil {
		t.Fatalf("the report holds no picture: %v", zipNames(got))
	}
	rec := got["Frame Fairy report/recordings/1.mov"]
	if rec == nil || rec.Method != zip.Store {
		t.Fatalf("the report holds no recording stored as it is: %v", zipNames(got))
	}
	rc, err := rec.Open()
	if err != nil {
		t.Fatal(err)
	}
	defer rc.Close()
	body, _ := io.ReadAll(rc)
	if sniffRecording(body) != "mov" {
		t.Fatal("the recording in the report is not a MOV")
	}
}

// recordingOf makes a recording of one second, 160 by 90, with a sound,
// and a place, a title and a comment the way a phone or a recorder
// writes them.
func recordingOf(t *testing.T, path string, args ...string) {
	t.Helper()
	all := append([]string{"-loglevel", "error", "-y",
		"-f", "lavfi", "-i", "testsrc2=size=160x90:rate=30:duration=1",
		"-f", "lavfi", "-i", "sine=f=440:d=1",
		"-metadata", "title=Tim's desk", "-metadata", "comment=Made on Tim's Mac",
		"-metadata", "location=+52.5200+013.4050/", "-metadata", "com.apple.quicktime.location.ISO6709=+52.5200+013.4050/",
		"-movflags", "use_metadata_tags", "-pix_fmt", "yuv420p", "-shortest"}, args...)
	all = append(all, path)
	if out, err := exec.Command("ffmpeg", all...).CombinedOutput(); err != nil {
		t.Fatalf("ffmpeg: %v\n%s", err, out)
	}
}

// probeTags says what kinds of streams a file holds, in order, and the
// tags of the file.
func probeTags(t *testing.T, path string) (string, map[string]string) {
	t.Helper()
	out, err := exec.Command("ffprobe", "-v", "error", "-show_entries", "stream=codec_type:format_tags",
		"-of", "json", path).Output()
	if err != nil {
		t.Fatalf("ffprobe %s: %v", path, err)
	}
	var data struct {
		Streams []struct {
			Kind string `json:"codec_type"`
		} `json:"streams"`
		Format struct {
			Tags map[string]string `json:"tags"`
		} `json:"format"`
	}
	if err := json.Unmarshal(out, &data); err != nil {
		t.Fatal(err)
	}
	var kinds []string
	for _, s := range data.Streams {
		kinds = append(kinds, s.Kind)
	}
	return strings.Join(kinds, " "), data.Format.Tags
}

func zipNames(files map[string]*zip.File) []string {
	var all []string
	for name := range files {
		all = append(all, name)
	}
	return all
}
