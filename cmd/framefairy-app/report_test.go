package main

import (
	"archive/zip"
	"compress/gzip"
	"io"
	"os"
	"path/filepath"
	"sort"
	"strings"
	"testing"

	"github.com/rs/zerolog"
)

// A report of a problem is one zip on the Desktop, shown in Finder, with
// what the person wrote, the log with its older files unpacked, what the
// app and the machine are and the settings. The home folder is written as
// ~ in all of it, and nothing of the video goes in but its file name. A
// second report on the same minute does not take the first one's place.
// Plan row 2.188.
func TestAReportHoldsTheLogAndNothingOfTheVideo(t *testing.T) {
	home := t.TempDir()
	t.Setenv("HOME", home)
	desk := filepath.Join(home, "Desktop")
	if err := os.Mkdir(desk, 0o755); err != nil {
		t.Fatal(err)
	}
	path, _ := testLog(t)
	video := filepath.Join(home, "Movies", "start.mp4")
	theLog.line(zerolog.WarnLevel, "files").Str("path", video).Msg("the stream failed")
	// An older file of the log, the way lumberjack keeps it.
	older, err := os.Create(filepath.Join(filepath.Dir(path), "app-2026-10-09T10-00-00.000.log.gz"))
	if err != nil {
		t.Fatal(err)
	}
	gz := gzip.NewWriter(older)
	_, _ = gz.Write([]byte(`{"level":"info","msg":"started yesterday"}` + "\n"))
	_ = gz.Close()
	_ = older.Close()

	var shown []string
	was := showReport
	showReport = func(p string) error { shown = append(shown, p); return nil }
	t.Cleanup(func() { showReport = was })

	st := &store{dir: t.TempDir(), settings: defaultSettings()}
	st.settings.OutputDir = filepath.Join(home, "Shorts")
	f := &FrameFairy{store: st}
	first, err := f.ReportProblem("  The picture stopped at 46 s in "+video+".  ", video)
	if err != nil {
		t.Fatal(err)
	}
	second, err := f.ReportProblem("", video)
	if err != nil {
		t.Fatal(err)
	}
	if filepath.Dir(first) != desk || first == second {
		t.Errorf("the reports are %s and %s, want two files on the Desktop", first, second)
	}
	if len(shown) != 2 || shown[0] != first {
		t.Errorf("Finder was asked to show %v, want each report", shown)
	}

	files := unzipped(t, first)
	var names []string
	for name := range files {
		names = append(names, name)
	}
	sort.Strings(names)
	want := []string{"about.json", "logs/app-2026-10-09T10-00-00.000.log", "logs/app.log", "settings.json", "What happened.txt"}
	sort.Strings(want)
	if strings.Join(names, ",") != strings.Join(want, ",") {
		t.Errorf("the report holds %v, want %v", names, want)
	}
	if got := files["What happened.txt"]; got != "The picture stopped at 46 s in ~/Movies/start.mp4.\n" {
		t.Errorf("what happened reads %q", got)
	}
	if !strings.Contains(files["logs/app.log"], `"path":"~/Movies/start.mp4"`) {
		t.Errorf("the log in the report does not name the video under ~:\n%s", files["logs/app.log"])
	}
	if !strings.Contains(files["logs/app-2026-10-09T10-00-00.000.log"], "started yesterday") {
		t.Error("the older file of the log is not in the report, unpacked")
	}
	if !strings.Contains(files["settings.json"], `"outputDir": "~/Shorts"`) {
		t.Errorf("the settings in the report do not write the home folder as ~:\n%s", files["settings.json"])
	}
	if !strings.Contains(files["about.json"], `"video": "start.mp4"`) || !strings.Contains(files["about.json"], `"run": "`+theLog.run+`"`) {
		t.Errorf("about.json does not name the video and the run:\n%s", files["about.json"])
	}
	for name, text := range files {
		if strings.Contains(text, home) {
			t.Errorf("%s names the home folder", name)
		}
	}
	if _, ok := unzipped(t, second)["What happened.txt"]; ok {
		t.Error("a report with nothing written holds an empty What happened.txt")
	}
}

func unzipped(t *testing.T, path string) map[string]string {
	t.Helper()
	r, err := zip.OpenReader(path)
	if err != nil {
		t.Fatal(err)
	}
	defer r.Close()
	files := map[string]string{}
	for _, f := range r.File {
		rc, err := f.Open()
		if err != nil {
			t.Fatal(err)
		}
		b, _ := io.ReadAll(rc)
		_ = rc.Close()
		files[strings.TrimPrefix(f.Name, "Frame Fairy report/")] = string(b)
	}
	return files
}
