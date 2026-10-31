package main

import (
	"archive/zip"
	"bytes"
	"compress/gzip"
	"context"
	"encoding/json"
	"fmt"
	"io"
	"os"
	"os/exec"
	"path/filepath"
	"runtime"
	"strings"
	"time"

	"github.com/rs/zerolog"
	"github.com/spf13/afero"

	"framefairy/engine"
	"framefairy/engine/fileio"
)

// A report of a problem: one zip on the Desktop that a customer sends us,
// with what they say happened, the app's log and what we need to read it.
// It never holds a video, its transcript or its captions, and paths under
// the home folder are written as ~, so the Mac's user name does not
// travel. Bugs in the sidebar, plan rows 2.188 and 2.244, bugs.go and
// docs/LOGGING.md.

// What the person may write, a few pages at most.
const reportWordsMax = 20000

// Where a report is put and how it is shown, which the tests set.
var (
	reportFolder = func() string {
		home, err := os.UserHomeDir()
		if err != nil {
			return os.TempDir()
		}
		if desk := filepath.Join(home, "Desktop"); dirExists(desk) {
			return desk
		}
		return home
	}
	showReport = showInFolder
)

func dirExists(path string) bool {
	info, err := os.Stat(path)
	return err == nil && info.IsDir()
}

// aboutMachine is what a report says about the app and the machine.
type aboutMachine struct {
	Made      string `json:"made"`
	Build     string `json:"build"`
	Run       string `json:"run"`
	OS        string `json:"os"`
	OSVersion string `json:"osVersion,omitempty"`
	Arch      string `json:"arch"`
	Chip      string `json:"chip,omitempty"`
	MemoryGB  int64  `json:"memoryGB,omitempty"`
	Video     string `json:"video,omitempty"`
}

// asked runs a short command for a report and gives what it said, or
// nothing.
func asked(name string, args ...string) string {
	ctx, cancel := context.WithTimeout(context.Background(), 2*time.Second)
	defer cancel()
	out, err := exec.CommandContext(ctx, name, args...).Output()
	if err != nil {
		return ""
	}
	return strings.TrimSpace(string(out))
}

func machine(video string) aboutMachine {
	a := aboutMachine{
		Made: time.Now().Format(time.RFC3339), Build: runningVersion(), Run: theLog.run,
		OS: runtime.GOOS, Arch: runtime.GOARCH, MemoryGB: engine.MachineMemory() >> 30,
	}
	if video != "" {
		a.Video = filepath.Base(video)
	}
	if runtime.GOOS == "darwin" {
		a.OSVersion = asked("sw_vers", "-productVersion")
		a.Chip = asked("sysctl", "-n", "machdep.cpu.brand_string")
	}
	return a
}

// homeless writes the home folder as ~ wherever it is named.
func homeless(b []byte) []byte {
	home, err := os.UserHomeDir()
	if err != nil || len(home) < 2 {
		return b
	}
	return bytes.ReplaceAll(b, []byte(home), []byte("~"))
}

// ReportProblem writes a report of a problem to the Desktop, shows it in
// Finder and closes the report window, with what the person says happened, the pictures they
// attached and the video that was in front, by its file name only, or
// without one the video last worked on. It gives the report's path, and
// lets go of the draft it was made from.
func (f *FrameFairy) ReportProblem(what, video string) (string, error) {
	what = cut(what, reportWordsMax)
	if video == "" {
		video = f.report.lastVideo()
	}
	sent, pictures := f.report.reportPictures()
	theLog.line(zerolog.InfoLevel, "report").Int("characters", len(what)).Int("pictures", len(pictures)).Msg("a report of a problem is being made")
	var buf bytes.Buffer
	z := zip.NewWriter(&buf)
	add := func(name string, data []byte) error {
		w, err := z.Create("Frame Fairy report/" + name)
		if err != nil {
			return err
		}
		_, err = w.Write(homeless(data))
		return err
	}
	if strings.TrimSpace(what) != "" {
		if err := add("what-happened.txt", []byte(strings.TrimSpace(what)+"\n")); err != nil {
			return "", err
		}
	}
	about, _ := json.MarshalIndent(machine(video), "", "  ")
	if err := add("about.json", about); err != nil {
		return "", err
	}
	if f.store != nil {
		settings, _ := json.MarshalIndent(f.store.Settings(), "", "  ")
		if err := add("settings.json", settings); err != nil {
			return "", err
		}
	}
	for _, path := range theLog.files() {
		data, err := readLog(path)
		if err != nil {
			continue
		}
		if err := add("logs/"+logName(path), data); err != nil {
			return "", err
		}
	}
	// The pictures and recordings go as they are, since home is not in a
	// picture's pixels and replacing it in them would spoil the file. A
	// recording is stored, not squeezed, since it is squeezed already.
	shots, films := 0, 0
	for _, f := range pictures {
		h := &zip.FileHeader{Method: zip.Deflate}
		if f.ending == "png" {
			shots++
			h.Name = fmt.Sprintf("Frame Fairy report/pictures/%d.png", shots)
		} else {
			films++
			h.Name = fmt.Sprintf("Frame Fairy report/recordings/%d.%s", films, f.ending)
			h.Method = zip.Store
		}
		w, err := z.CreateHeader(h)
		if err == nil {
			_, err = w.Write(f.data)
		}
		if err != nil {
			return "", err
		}
	}
	// The trace lines of the last while, kept in memory and in no file,
	// see appLog.
	if recent := theLog.recent.all(); len(recent) > 0 {
		if err := add("logs/recent-frames.log", recent); err != nil {
			return "", err
		}
	}
	if err := z.Close(); err != nil {
		return "", err
	}
	path := freeName(reportFolder(), "Frame Fairy report "+time.Now().Format("2006-01-02 15.04"), ".zip")
	if err := os.WriteFile(path, buf.Bytes(), 0o600); err != nil {
		return "", fmt.Errorf("the report could not be saved: %w", err)
	}
	theLog.line(zerolog.InfoLevel, "report").Int("bytes", buf.Len()).Int("pictures", len(pictures)).Msg("a report of a problem was made")
	f.report.clear(sent)
	_ = showReport(path)
	// Finder has what was made, and the window it was made in has done
	// its work, as Tim asked.
	f.report.closeWindow("report")
	return path, nil
}

// readLog reads one of the log's files, the older ones unpacked.
func readLog(path string) ([]byte, error) {
	data, err := os.ReadFile(path)
	if err != nil || !strings.HasSuffix(path, ".gz") {
		return data, err
	}
	r, err := gzip.NewReader(bytes.NewReader(data))
	if err != nil {
		return nil, err
	}
	defer r.Close()
	return io.ReadAll(io.LimitReader(r, 64<<20))
}

// freeName is a path in dir that nothing has yet: the name, or the name
// with a number after it.
func freeName(dir, name, ext string) string {
	path := filepath.Join(dir, name+ext)
	for i := 2; fileio.IsFile(afero.NewOsFs(), path); i++ {
		path = filepath.Join(dir, fmt.Sprintf("%s %d%s", name, i, ext))
	}
	return path
}

// logName is the name a file of the log has in a report, written like
// every other name there, in small letters with hyphens:
// app-2026-10-09T10-00-00.000.log.gz, as lumberjack keeps an older file,
// is app-2026-10-09-10-00-00.000.log.
func logName(path string) string {
	return strings.ToLower(strings.Replace(strings.TrimSuffix(filepath.Base(path), ".gz"), "T", "-", 1))
}
