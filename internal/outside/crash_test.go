package outside

import (
	"errors"
	"io/fs"
	"path"
	"slices"
	"strings"
	"testing"
	"testing/fstest"
	"time"
)

// crashReports names the crash reports macOS wrote of the program exe in
// dir at since or after, oldest first. macOS names one for the program
// and the moment, framefairy-app-2026-11-19-120102.ips, or .crash on an
// older system. A folder that is not there holds none.
func crashReports(dir fs.FS, exe string, since time.Time) ([]string, error) {
	entries, err := fs.ReadDir(dir, ".")
	if errors.Is(err, fs.ErrNotExist) {
		return nil, nil
	}
	if err != nil {
		return nil, err
	}
	// The file's time may be kept to the second.
	since = since.Truncate(time.Second)
	type report struct {
		name string
		at   time.Time
	}
	var found []report
	for _, e := range entries {
		rest, ok := strings.CutPrefix(e.Name(), exe+"-")
		if !ok || rest == "" || rest[0] < '0' || rest[0] > '9' {
			continue
		}
		if ext := path.Ext(rest); ext != ".ips" && ext != ".crash" {
			continue
		}
		info, err := e.Info()
		if err != nil {
			return nil, err
		}
		if info.ModTime().Before(since) {
			continue
		}
		found = append(found, report{e.Name(), info.ModTime()})
	}
	slices.SortFunc(found, func(a, b report) int { return a.at.Compare(b.at) })
	names := make([]string, len(found))
	for i, r := range found {
		names[i] = r.name
	}
	return names, nil
}

// A step that fails shows the app's crash reports from that step on, and
// nothing older or of another program.
func TestTheCrashReportsOfAStep(t *testing.T) {
	step := time.Date(2026, 11, 19, 12, 1, 2, 500_000_000, time.UTC)
	file := func(at time.Time) *fstest.MapFile { return &fstest.MapFile{Data: []byte("{}"), ModTime: at} }
	dir := fstest.MapFS{
		"framefairy-app-2026-11-19-120203.ips":    file(step.Add(time.Minute)),
		"framefairy-app-2026-11-19-120102.ips":    file(step.Truncate(time.Second)),
		"framefairy-app-2026-11-19-110000.ips":    file(step.Add(-time.Hour)),
		"framefairy-app-2026-11-19-120300.crash":  file(step.Add(2 * time.Minute)),
		"framefairy-app-2026-11-19-120300.diag":   file(step.Add(2 * time.Minute)),
		"framefairy-app-helper-2026-11-19.ips":    file(step.Add(time.Minute)),
		"WebKit.WebContent-2026-11-19-120203.ips": file(step.Add(time.Minute)),
		"JetsamEvent-2026-11-19-120203.ips":       file(step.Add(time.Minute)),
	}
	got, err := crashReports(dir, "framefairy-app", step)
	if err != nil {
		t.Fatal(err)
	}
	want := []string{
		"framefairy-app-2026-11-19-120102.ips",
		"framefairy-app-2026-11-19-120203.ips",
		"framefairy-app-2026-11-19-120300.crash",
	}
	if !slices.Equal(got, want) {
		t.Errorf("crash reports since the step: %v, want %v", got, want)
	}

	// A Mac where nothing crashed yet has no such folder.
	missing, err := fs.Sub(fstest.MapFS{}, "DiagnosticReports")
	if err != nil {
		t.Fatal(err)
	}
	if got, err := crashReports(missing, "framefairy-app", step); err != nil || len(got) != 0 {
		t.Errorf("crash reports in a folder that is not there: %v, %v, want none and no error", got, err)
	}
}
