package main

import (
	"bufio"
	"bytes"
	"cmp"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"os"
	"os/exec"
	"path/filepath"
	"runtime/coverage"
	"slices"
	"strconv"
	"strings"
	"sync/atomic"
	"testing"
)

// The map of what each run of the walks goes through, which picks the
// walks a change to the interface or the Go side needs, see
// frontend/preview/walks/walkmap.mjs. make walks WALKRECORD=folder records
// it: the bridge is built with Go's coverage, and each run writes down the
// lines of Go its bridge ran and the lines of the interface its pages ran.
// make walkmap makes the map from what the runs wrote. It is recorded at
// night, see walks-nightly.yml.

// coverHandler is the bridge's /cover: the counts of what the Go side ran
// since it was last asked, written into the folder the body names and
// begun again, or with no folder only begun again. Only a bridge built
// with -cover has counts, and one without answers why.
func coverHandler(w http.ResponseWriter, r *http.Request) {
	dir, err := io.ReadAll(r.Body)
	if err == nil && len(dir) > 0 {
		err = coverage.WriteMetaDir(string(dir))
		if err == nil {
			err = coverage.WriteCountersDir(string(dir))
		}
	}
	if err == nil {
		err = coverage.ClearCounters()
	}
	if err != nil {
		http.Error(w, err.Error(), http.StatusInternalServerError)
	}
}

// recorder writes what each run went through into its folder: a file of
// its own for every run, under runs/, and what code there is at all, once
// for each walker, so a line no run ran is known from a line that is no
// code.
type recorder struct {
	dir     string
	module  string
	goTool  string
	walkers atomic.Int32
	runs    atomic.Int32
}

// newRecorder is the recorder for WALKRECORD, or nil when it is not set.
//
// On the real disk: the runs' pages and bridges are programs of their own,
// which write what they ran into the folder.
func newRecorder(t *testing.T) *recorder {
	dir := os.Getenv("WALKRECORD")
	if dir == "" {
		return nil
	}
	goTool, err := exec.LookPath("go")
	if err != nil {
		t.Fatal("recording the walks needs go for go tool covdata")
	}
	mod, err := os.ReadFile("../../go.mod")
	if err != nil {
		t.Fatal(err)
	}
	module, _, _ := strings.Cut(strings.TrimPrefix(string(mod), "module "), "\n")
	if err := os.MkdirAll(filepath.Join(dir, "runs"), 0o755); err != nil {
		t.Fatal(err)
	}
	return &recorder{dir: dir, module: strings.TrimSpace(module), goTool: goTool}
}

// walkerRecord is one walker's: its bridge, and the folder its runs write
// into before what they wrote is kept.
type walkerRecord struct {
	rec    *recorder
	n      int
	url    string
	work   string
	js     string
	known  bool
	client http.Client
}

// walker starts the record of a walker on the bridge at url, and gives
// the environment its pages are told where to write in.
//
// On the real disk: the walker's pages are a browser of their own, which
// writes what they ran into this folder.
func (rc *recorder) walker(url string) (*walkerRecord, []string, error) {
	n := int(rc.walkers.Add(1))
	work := filepath.Join(rc.dir, fmt.Sprintf("walker-%d", n))
	if err := os.MkdirAll(work, 0o755); err != nil {
		return nil, nil, err
	}
	w := &walkerRecord{rec: rc, n: n, url: url, work: work, js: filepath.Join(work, "js.json")}
	return w, []string{"WALKCOVER=" + w.js}, nil
}

func (w *walkerRecord) cover(dir string) error {
	res, err := w.client.Post(w.url+"cover", "text/plain", strings.NewReader(dir))
	if err != nil {
		return err
	}
	defer res.Body.Close()
	if res.StatusCode != http.StatusOK {
		said, _ := io.ReadAll(res.Body)
		return fmt.Errorf("the bridge counts nothing it runs: %s", bytes.TrimSpace(said))
	}
	return nil
}

// begin forgets what the bridge ran before the run.
//
// On the real disk: the pages wrote what they ran there, see walker.
func (w *walkerRecord) begin() error {
	_ = os.Remove(w.js)
	return w.cover("")
}

// end keeps what the run went through as run id, the Go side's lines with
// the interface's.
//
// On the real disk: the bridge, a program of its own, writes its counts
// there for go tool covdata to read.
func (w *walkerRecord) end(id string) error {
	counts := filepath.Join(w.work, "go")
	_ = os.RemoveAll(counts)
	if err := os.MkdirAll(counts, 0o755); err != nil {
		return err
	}
	if err := w.cover(counts); err != nil {
		return err
	}
	text := counts + ".txt"
	if out, err := exec.Command(w.rec.goTool, "tool", "covdata", "textfmt", "-i="+counts, "-o="+text).CombinedOutput(); err != nil {
		return fmt.Errorf("go tool covdata: %v\n%s", err, out)
	}
	profile, err := os.ReadFile(text)
	if err != nil {
		return err
	}
	ran, known := goLines(string(profile), w.rec.module)
	page := struct {
		Ran    map[string][][2]int `json:"ran"`
		Known  map[string][][2]int `json:"known"`
		Bundle []string            `json:"bundle"`
	}{}
	// A run whose pages wrote nothing ran nothing of the interface that
	// can be told, like one that broke before its page opened.
	if body, err := os.ReadFile(w.js); err == nil {
		if err := json.Unmarshal(body, &page); err != nil {
			return fmt.Errorf("what the pages ran: %w", err)
		}
	}
	for f, s := range page.Ran {
		ran[f] = s
	}
	k := w.rec.runs.Add(1)
	if err := writeJSON(filepath.Join(w.rec.dir, "runs", fmt.Sprintf("%d.json", k)), map[string]any{"run": id, "ran": ran}); err != nil {
		return err
	}
	// What code there is, the same for every run of the walker, kept from
	// the first run whose pages said.
	if !w.known && page.Known != nil {
		for f, s := range page.Known {
			known[f] = s
		}
		if err := writeJSON(filepath.Join(w.rec.dir, fmt.Sprintf("known-%d.json", w.n)), map[string]any{"known": known, "bundle": page.Bundle}); err != nil {
			return err
		}
		w.known = true
	}
	_ = os.RemoveAll(counts)
	_ = os.Remove(text)
	return nil
}

// writeJSON writes v into file.
//
// On the real disk: make walkmap reads the record after the test is gone.
func writeJSON(file string, v any) error {
	body, err := json.Marshal(v)
	if err != nil {
		return err
	}
	return os.WriteFile(file, body, 0o644)
}

// goLines reads a profile go tool covdata textfmt wrote: the spans of
// lines of each file a block ran on, and of every block there is. A file
// is named from the top of the repository, without the module.
func goLines(profile, module string) (ran, known map[string][][2]int) {
	ran = map[string][][2]int{}
	known = map[string][][2]int{}
	lines := bufio.NewScanner(strings.NewReader(profile))
	lines.Buffer(nil, 1<<20)
	for lines.Scan() {
		// framefairy/engine/clips.go:12.34,15.2 3 1
		line := lines.Text()
		where, rest, ok := strings.Cut(line, ":")
		if !ok || strings.HasPrefix(line, "mode:") {
			continue
		}
		fields := strings.Fields(rest)
		if len(fields) != 3 {
			continue
		}
		from, to, ok := strings.Cut(fields[0], ",")
		if !ok {
			continue
		}
		a, errA := strconv.Atoi(strings.SplitN(from, ".", 2)[0])
		b, errB := strconv.Atoi(strings.SplitN(to, ".", 2)[0])
		count, errC := strconv.Atoi(fields[2])
		if errA != nil || errB != nil || errC != nil {
			continue
		}
		file := strings.TrimPrefix(where, module+"/")
		known[file] = append(known[file], [2]int{a, b})
		if count > 0 {
			ran[file] = append(ran[file], [2]int{a, b})
		}
	}
	for _, m := range []map[string][][2]int{ran, known} {
		for f, s := range m {
			m[f] = joinSpans(s)
		}
	}
	return ran, known
}

// joinSpans sorts spans of lines and joins those that overlap or touch.
func joinSpans(spans [][2]int) [][2]int {
	slices.SortFunc(spans, func(x, y [2]int) int { return cmp.Compare(x[0], y[0]) })
	var out [][2]int
	for _, s := range spans {
		if n := len(out); n > 0 && s[0] <= out[n-1][1]+1 {
			out[n-1][1] = max(out[n-1][1], s[1])
			continue
		}
		out = append(out, s)
	}
	return out
}

// A profile is read as the lines each file ran and the lines of code it
// has, the module left off its name, blocks on the same lines joined.
func TestTheGoSideARunRanIsReadByLine(t *testing.T) {
	t.Parallel()
	profile := `mode: atomic
framefairy/engine/clips.go:12.34,15.2 3 1
framefairy/engine/clips.go:15.2,18.3 2 0
framefairy/engine/clips.go:20.1,21.5 1 4
framefairy/engine/clips.go:22.1,22.9 1 1
framefairy/cmd/framefairy-app/app.go:3.1,4.2 1 0
`
	ran, known := goLines(profile, "framefairy")
	wantRan := map[string][][2]int{"engine/clips.go": {{12, 15}, {20, 22}}}
	wantKnown := map[string][][2]int{"engine/clips.go": {{12, 18}, {20, 22}}, "cmd/framefairy-app/app.go": {{3, 4}}}
	for _, c := range []struct {
		name      string
		got, want map[string][][2]int
	}{{"ran", ran, wantRan}, {"known", known, wantKnown}} {
		if len(c.got) != len(c.want) {
			t.Errorf("%s: %v, want %v", c.name, c.got, c.want)
		}
		for f, s := range c.want {
			if !slices.Equal(c.got[f], s) {
				t.Errorf("%s %s: %v, want %v", c.name, f, c.got[f], s)
			}
		}
	}
}
