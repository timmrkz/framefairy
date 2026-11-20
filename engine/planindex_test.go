package engine

import (
	"bufio"
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"sync"
	"sync/atomic"
	"testing"

	"github.com/spf13/afero"
)

// planRecordOf is a plan record of search i, answered by model, with a prompt
// long enough that reading every record costs what it costs in the app.
func planRecordOf(i int, model string) PlanRecord {
	return PlanRecord{Schema: 1, PlanID: fmt.Sprintf("p-%03d", i), ReplyKey: fmt.Sprintf("r-%03d", i),
		ExampleKey: fmt.Sprintf("e-%03d", i%3), Planner: RecordPlanner{Model: model},
		Prompt: fmt.Sprintf("%0500d", i), Candidates: []RecordCandidate{{CID: fmt.Sprintf("%02d", i)}}}
}

// trainingIn is a training folder in memory with the records of
// searches 0 to n-1, appended the way every search appends its record.
func trainingIn(t *testing.T, fsys afero.Fs, n int) *Records {
	t.Helper()
	r := NewRecords(fsys, "/training", "/speed.json")
	for i := range n {
		if err := r.appendRecord(r.plansFile(), planRecordOf(i, "m")); err != nil {
			t.Fatal(err)
		}
	}
	return r
}

func (r *Records) plansFile() string { return filepath.Join(r.TrainingDir(), "plans.jsonl") }

// A trim let go finds its plan's record, a search finds an answer it had
// before, and the same answer to the same question is found to be
// recorded already, each the newest that fits, and none when there is
// none, whatever was appended, removed or written over in between.
func TestAPlanRecordIsFoundByEachOfItsKeys(t *testing.T) {
	t.Parallel()
	fsys := afero.NewMemMapFs()
	r := trainingIn(t, fsys, 9)
	want := func(step string, got PlanRecord, ok bool, id string) {
		t.Helper()
		if id == "" && ok {
			t.Errorf("%s: found %s, want none", step, got.PlanID)
		}
		if id != "" && (!ok || got.PlanID != id) {
			t.Errorf("%s: found %q %v, want %s", step, got.PlanID, ok, id)
		}
	}
	rec, ok := r.loadPlanRecord("p-004")
	want("by its plan id", rec, ok, "p-004")
	rec, ok = r.findPlanRecord("r-007")
	want("by its reply key", rec, ok, "p-007")
	rec, ok = r.findSamePlan(planRecordOf(5, "m"))
	want("the same answer to the same question", rec, ok, "p-005")
	rec, ok = r.findSamePlan(planRecordOf(4, "other"))
	want("the same question answered by another model", rec, ok, "")
	rec, ok = r.loadPlanRecord("p-404")
	want("a plan id never recorded", rec, ok, "")

	// The same question asked again is found as the newest record.
	again := planRecordOf(2, "m")
	again.PlanID, again.ReplyKey = "p-new", "r-002"
	if err := r.appendRecord(r.plansFile(), again); err != nil {
		t.Fatal(err)
	}
	rec, ok = r.findPlanRecord("r-002")
	want("an answer recorded again", rec, ok, "p-new")
	rec, ok = r.loadPlanRecord("p-002")
	want("the older record of it", rec, ok, "p-002")

	// A record half written, by a crash in the middle of it, is passed
	// over, and so is a line not ended yet, until it is.
	f, err := fsys.OpenFile(r.plansFile(), os.O_WRONLY|os.O_APPEND, 0o644)
	if err != nil {
		t.Fatal(err)
	}
	if _, err := f.Write([]byte(`{"plan_id":"p-broken","reply_` + "\n" + `{"plan_id":"p-late"`)); err != nil {
		t.Fatal(err)
	}
	rec, ok = r.loadPlanRecord("p-broken")
	want("a record half written", rec, ok, "")
	rec, ok = r.loadPlanRecord("p-late")
	want("a record still being written", rec, ok, "")
	if _, err := f.Write([]byte(`,"reply_key":"r-late"}` + "\n")); err != nil {
		t.Fatal(err)
	}
	_ = f.Close()
	rec, ok = r.findPlanRecord("r-late")
	want("a record written to its end", rec, ok, "p-late")

	// The training removed, and records of other searches written in its
	// place, shorter than it was, and then longer.
	for _, round := range []struct {
		prefix string
		n      int
	}{{"q", 2}, {"s", 40}} {
		if err := fsys.Remove(r.plansFile()); err != nil {
			t.Fatal(err)
		}
		for i := range round.n {
			p := planRecordOf(i, "m")
			p.PlanID = fmt.Sprintf("%s-%03d", round.prefix, i)
			if err := r.appendRecord(r.plansFile(), p); err != nil {
				t.Fatal(err)
			}
		}
		step := fmt.Sprintf("%d records written in place of the ones before", round.n)
		rec, ok = r.loadPlanRecord("p-001")
		want(step+", the first ones", rec, ok, "")
		rec, ok = r.loadPlanRecord("q-001")
		want(step+", the ones before", rec, ok, map[string]string{"q": "q-001", "s": ""}[round.prefix])
		for _, i := range []int{0, round.n - 1} {
			id := fmt.Sprintf("%s-%03d", round.prefix, i)
			rec, ok = r.loadPlanRecord(id)
			want(step+", "+id, rec, ok, id)
		}
	}

	// Another training folder.
	r.SetTrainingDir("/elsewhere")
	rec, ok = r.loadPlanRecord("s-001")
	want("a record in the folder before", rec, ok, "")
}

// The second lookup reads only the line it finds, not every record again,
// so a trim let go costs the same however many searches came before it.
func TestAPlanRecordIsReadFromItsLineAlone(t *testing.T) {
	t.Parallel()
	reads := &bytesRead{Fs: afero.NewMemMapFs()}
	r := trainingIn(t, reads, 50)
	info, err := reads.Stat(r.plansFile())
	if err != nil {
		t.Fatal(err)
	}
	if _, ok := r.loadPlanRecord("p-010"); !ok {
		t.Fatal("p-010 not found")
	}
	before := reads.n.Load()
	if _, ok := r.loadPlanRecord("p-030"); !ok {
		t.Fatal("p-030 not found")
	}
	line := info.Size() / 50
	if n := reads.n.Load() - before; n > headSize+2*line {
		t.Errorf("the second lookup read %d bytes of a file of %d, want its line of %d and the head", n,
			info.Size(), line)
	}
}

// Searches record and trims look up at the same time, from goroutines of
// their own, and every record appended before a lookup is found by it.
func TestPlanRecordsAreFoundWhileOthersAreAppended(t *testing.T) {
	t.Parallel()
	r := trainingIn(t, afero.NewMemMapFs(), 0)
	var wg sync.WaitGroup
	for w := range 4 {
		wg.Go(func() {
			for i := range 25 {
				p := planRecordOf(w*100+i, "m")
				if err := r.appendRecord(r.plansFile(), p); err != nil {
					t.Error(err)
					return
				}
				if got, ok := r.loadPlanRecord(p.PlanID); !ok || got.PlanID != p.PlanID {
					t.Errorf("%s appended and then not found: %q %v", p.PlanID, got.PlanID, ok)
				}
				if got, ok := r.findPlanRecord(p.ReplyKey); !ok || got.PlanID != p.PlanID {
					t.Errorf("%s not found by its reply key: %q %v", p.PlanID, got.PlanID, ok)
				}
			}
		})
	}
	wg.Wait()
}

// bytesRead is a disk that counts the bytes read from its files.
type bytesRead struct {
	afero.Fs
	n atomic.Int64
}

func (d *bytesRead) Open(name string) (afero.File, error) {
	f, err := d.Fs.Open(name)
	if err != nil {
		return nil, err
	}
	return &countedFile{File: f, n: &d.n}, nil
}

type countedFile struct {
	afero.File
	n *atomic.Int64
}

func (f *countedFile) Read(p []byte) (int, error) {
	n, err := f.File.Read(p)
	f.n.Add(int64(n))
	return n, err
}

func (f *countedFile) ReadAt(p []byte, off int64) (int, error) {
	n, err := f.File.ReadAt(p, off)
	f.n.Add(int64(n))
	return n, err
}

// readRecords calls fn with every line of a JSON Lines file in the
// training folder, for a test to read what was recorded. Broken lines, for example from a crash mid-write, are
// skipped.
func (r *Records) readRecords(path string, fn func(line []byte)) error {
	f, err := r.Files.Open(path)
	if err != nil {
		return err
	}
	defer f.Close()
	scanner := bufio.NewScanner(f)
	scanner.Buffer(make([]byte, 1<<20), 64<<20)
	for scanner.Scan() {
		line := scanner.Bytes()
		if json.Valid(line) {
			fn(line)
		}
	}
	return scanner.Err()
}
