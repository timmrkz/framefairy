package engine

import (
	"encoding/json"
	"fmt"
	"path/filepath"
	"sync"
	"testing"

	"github.com/spf13/afero"
)

// Records are written by jobs on their own goroutines while the app
// saves settings on another, and the settings say where the records go. So
// the one folder, the note beside an episode and the record files
// themselves are all reached from several places at once. make test runs
// this with the race detector.
func TestRecordingFromEveryDirectionAtOnce(t *testing.T) {
	t.Parallel()
	fsys := afero.NewMemMapFs()
	dir := "/home/tim/.framefairy/training"
	records := NewRecords(fsys, dir, "")
	source := "/home/tim/ep.mp4"
	if err := afero.WriteFile(fsys, source, []byte("video"), 0o644); err != nil {
		t.Fatal(err)
	}

	const hands, each = 8, 25
	var wg sync.WaitGroup
	for hand := 0; hand < hands; hand++ {
		wg.Add(1)
		go func(hand int) {
			defer wg.Done()
			for k := 0; k < each; k++ {
				record := DecisionRecord{Schema: TrainingSchema, PlanID: "p1",
					CID: fmt.Sprintf("%d-%d", hand, k), Event: DecisionViewed}
				if err := records.appendRecord(filepath.Join(records.TrainingDir(), "decisions.jsonl"), record); err != nil {
					t.Error(err)
				}
				if err := WriteJob(fsys, source, JobRecord{ID: SearchID, Kind: JobSearch, Step: StepHearing}); err != nil {
					t.Error(err)
				}
				if !EverSearched(fsys, source) || ReadSearch(fsys, source) == nil {
					t.Error("a search being written was not read whole")
				}
			}
		}(hand)
	}
	// The app saving its settings while all that goes on.
	wg.Add(1)
	go func() {
		defer wg.Done()
		for k := 0; k < hands*each; k++ {
			records.SetTrainingDir(dir)
			_ = records.TrainingDir()
		}
	}()
	wg.Wait()

	// Every record came out whole: appends from different goroutines never
	// land inside each other.
	seen := map[string]bool{}
	lines := 0
	_ = records.readRecords(filepath.Join(dir, "decisions.jsonl"), func(line []byte) {
		lines++
		var d DecisionRecord
		if json.Unmarshal(line, &d) != nil {
			t.Errorf("a record came out broken: %s", line)
			return
		}
		seen[d.CID] = true
	})
	if lines != hands*each || len(seen) != hands*each {
		t.Errorf("%d records, %d of them different, wanted %d", lines, len(seen), hands*each)
	}
}
