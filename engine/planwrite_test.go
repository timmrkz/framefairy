package engine

import (
	"fmt"
	"io"
	"os"
	"path/filepath"
	"strings"
	"sync"
	"sync/atomic"
	"testing"

	"github.com/spf13/afero"
)

// A plan of the size the app really makes: twelve clips, each with its
// segments and all its words.
func aRealisticPlan(clips int) string {
	var b strings.Builder
	b.WriteString(`{"source":"ep.mp4","plan_id":"p1","planned_with":{"from":0,"to":1800},"clips":[`)
	for i := 0; i < clips; i++ {
		if i > 0 {
			b.WriteString(",")
		}
		start := float64(i * 120)
		fmt.Fprintf(&b, `{"id":"%02d","slug":"clip-%02d","note":"why this one was chosen",`, i+1, i+1)
		fmt.Fprintf(&b, `"segments":[{"start":%.1f,"end":%.1f,"crop_x":120}],"words":[`, start, start+24)
		for w := 0; w < 60; w++ {
			if w > 0 {
				b.WriteString(",")
			}
			at := start + float64(w)*0.4
			fmt.Fprintf(&b, `[%.2f,%.2f,"wort%d"]`, at, at+0.35, w)
		}
		b.WriteString("]}")
	}
	b.WriteString("]}")
	return b.String()
}

// On the real disk: the tests that use it are about the system's rename and lock.
func planOnDisk(t *testing.T, clips int) (string, []byte) {
	t.Helper()
	path := filepath.Join(t.TempDir(), "clips.json")
	body := []byte(aRealisticPlan(clips))
	if err := os.WriteFile(path, body, 0o644); err != nil {
		t.Fatal(err)
	}
	if _, _, err := LoadClips(disk, path); err != nil {
		t.Fatalf("the plan this test writes is not a plan: %v", err)
	}
	return path, body
}

// A finished search puts a whole plan where a plan goes, and the app is
// reading that folder about once a second while it does. The replacement
// has to be one step, so a reader sees the plan that was there or the plan
// that is there and never half of either.
//
// This is the whole of it, said in a way that does not depend on timing: a
// reader that already had the file open goes on reading what it opened. A
// write that empties the file and fills it again cannot do that, because
// there is only ever one file.
//
// On the real disk: what is tested is a file held open while it is replaced.
func TestAPlanIsReplacedRatherThanOverwritten(t *testing.T) {
	path, before := planOnDisk(t, 12)
	after := []byte(aRealisticPlan(6))

	reader, err := os.Open(path)
	if err != nil {
		t.Fatal(err)
	}
	defer reader.Close()

	if err := writePlanFile(disk, path, after); err != nil {
		t.Fatal(err)
	}

	held, err := io.ReadAll(reader)
	if err != nil {
		t.Fatal(err)
	}
	if string(held) != string(before) {
		t.Errorf("a reader that had the plan open lost it: it held %d bytes of the %d it opened",
			len(held), len(before))
	}
	now, err := os.ReadFile(path)
	if err != nil || string(now) != string(after) {
		t.Errorf("the new plan is not the one on disk")
	}
}

// What lists the plans of an episode globs for clips*.json, so a plan
// being written must not be named like one until it is whole.
func TestAPlanBeingWrittenIsNotOneOfThePlans(t *testing.T) {
	fsys, logs := afero.NewMemMapFs(), "/home/tim/ep.framefairy/logs"
	real := filepath.Join(logs, "clips-0-1800.json")
	if err := afero.WriteFile(fsys, real, []byte(aRealisticPlan(3)), 0o644); err != nil {
		t.Fatal(err)
	}
	// A temporary file left behind by a write that never finished, named
	// the way writePlanFile names one.
	tmp, err := afero.TempFile(fsys, logs, ".clips-*.json")
	if err != nil {
		t.Fatal(err)
	}
	tmp.WriteString(`{"source":"ep.mp4","clips":[`)
	tmp.Close()

	if got := PlanSummaries(fsys, logs); len(got) != 1 {
		names := []string{}
		for _, s := range got {
			names = append(names, s.Name)
		}
		t.Errorf("the plans of the episode are %v, wanted only the whole one", names)
	}
}

// A write that goes wrong must leave the plan that was there. Emptying the
// file first and filling it again takes the work with it, and nothing the
// app does removes a person's work.
func TestAFailedWriteLeavesThePlanThatWasThere(t *testing.T) {
	fsys, path := afero.NewMemMapFs(), "/home/tim/ep.framefairy/logs/clips.json"
	if err := afero.WriteFile(fsys, path, []byte(aRealisticPlan(3)), 0o644); err != nil {
		t.Fatal(err)
	}
	if err := writePlanFile(fsys, path, []byte("{ this is not a plan")); err == nil {
		t.Error("a plan that is not a plan was written")
	}
	_, clips, err := LoadClips(fsys, path)
	if err != nil || len(clips) != 3 {
		t.Errorf("the plan that was there is gone: %d clips, %v", len(clips), err)
	}
	// And one that is JSON but not a plan at all.
	if err := writePlanFile(fsys, path, []byte(`["not", "a", "plan"]`)); err == nil {
		t.Error("something that is not a plan was written")
	}
	if _, clips, _ := LoadClips(fsys, path); len(clips) != 3 {
		t.Errorf("the plan that was there is gone, %d clips left", len(clips))
	}
}

// The same file has two writers: the search that makes a plan and the
// app that edits one. They take the same lock, so an edit always reads
// a whole plan and a search never lands in the middle of one.
//
// On the real disk: the lock between two writers is the system's.
func TestASearchAndAnEditDoNotOverwriteEachOther(t *testing.T) {
	t.Parallel()
	path, body := planOnDisk(t, 4)
	var bad, edits int64
	var wg sync.WaitGroup
	stop := make(chan struct{})

	wg.Add(1)
	go func() {
		defer wg.Done()
		defer close(stop)
		writeWhile(t, path, body, &edits)
	}()
	for e := 0; e < 3; e++ {
		wg.Add(1)
		go func() {
			defer wg.Done()
			for {
				select {
				case <-stop:
					return
				default:
				}
				atomic.AddInt64(&edits, 1)
				err := editPlan(disk, path, func(top *object, clips []*object) error {
					if len(clips) != 4 {
						atomic.AddInt64(&bad, 1)
					}
					return nil
				})
				if err != nil {
					atomic.AddInt64(&bad, 1)
				}
			}
		}()
	}
	wg.Wait()
	t.Logf("%d edits while the plan was written over and over", edits)
	if bad > 0 {
		t.Errorf("an edit met a plan that was not whole %d times", bad)
	}
	// And after all of it the plan is still whole.
	if _, clips, err := LoadClips(disk, path); err != nil || len(clips) != 4 {
		t.Errorf("the plan did not survive: %d clips, %v", len(clips), err)
	}
}

// What the app really does: read the clips and the coverage while a
// search writes its plan. Timing decides whether a torn read is reached,
// so this is a second pair of eyes on the test above rather than the
// guard itself.
//
// On the real disk: the system's rename is what keeps a reader from half a plan.
func TestTheWindowReadsAWholePlanWhileASearchWrites(t *testing.T) {
	t.Parallel()
	path, body := planOnDisk(t, 12)
	logs := filepath.Dir(path)
	var torn, reads int64
	var wg sync.WaitGroup
	stop := make(chan struct{})

	wg.Add(1)
	go func() {
		defer wg.Done()
		defer close(stop)
		writeWhile(t, path, body, &reads)
	}()
	for r := 0; r < 3; r++ {
		wg.Add(1)
		go func() {
			defer wg.Done()
			for {
				select {
				case <-stop:
					return
				default:
				}
				atomic.AddInt64(&reads, 1)
				if _, clips, err := LoadClips(disk, path); err != nil || len(clips) != 12 {
					atomic.AddInt64(&torn, 1)
				}
				if len(PlanSummaries(disk, logs)) != 1 {
					atomic.AddInt64(&torn, 1)
				}
			}
		}()
	}
	wg.Wait()
	t.Logf("%d reads while the plan was being written", reads)
	if torn > 0 {
		t.Errorf("%d of %d reads saw a plan that was not there in full", torn, reads)
	}
}

// writeWhile writes the plan over and over, 60 times, and on until the
// other side has done at least 30 of what it does at the same time, so
// the test has seen the two meet and does not pass on a writer that was
// done before the other side began. Each write is the test's own work,
// so it waits on no clock: an other side that never starts leaves the
// writer going until go test's -timeout ends it.
//
// On the real disk: what is tested is that a rename on the drive leaves
// no reader with half a plan.
func writeWhile(t *testing.T, path string, body []byte, other *int64) {
	for i := 0; i < 60 || atomic.LoadInt64(other) < 30; i++ {
		if err := writePlanFile(disk, path, body); err != nil {
			t.Errorf("writing the plan: %v", err)
			return
		}
	}
}
