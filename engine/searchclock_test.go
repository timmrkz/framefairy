package engine

import (
	"bytes"
	"context"
	"os"
	"path/filepath"
	"sync"
	"sync/atomic"
	"testing"
)

// useSpeedFile gives a test a record of timings of its own, empty.
func useSpeedFile(t *testing.T) {
	was := speedFile()
	SetSpeedFile(filepath.Join(t.TempDir(), "speed.json"))
	t.Cleanup(func() { SetSpeedFile(was) })
}

// A search measured against the one before it: 20 seconds loading, 30
// reading, 4 a clip for twelve clips, 3 of framing at the end. 101 in all.
var lastTime = searchSpeed{Load: 20, Read: 1000, Clip: 4, Tail: 3, Runs: 1}

func TestASearchWithNothingToGoOnSaysNoShare(t *testing.T) {
	f, r := searchProgress(searchNow{Part: partReading, Chars: 30000, Count: 12, Local: true}, searchSpeed{}, false)
	if f != Unknown || r != Unknown {
		t.Errorf("share %v, left %v", f, r)
	}
}

// Whatever happens, in whatever order the parts report, the share never
// goes back and never claims the whole before the search is over.
func TestTheShareOnlyGrows(t *testing.T) {
	now := searchNow{Chars: 30000, Count: 12, Local: true, Loads: true}
	last := -1.0
	check := func(what string) {
		t.Helper()
		f, r := searchProgress(now, lastTime, true)
		if f < last-1e-9 {
			t.Errorf("%s: share went back from %.4f to %.4f", what, last, f)
		}
		if f < 0 || f > 0.99 || r < 0 {
			t.Errorf("%s: share %.4f, left %.1f", what, f, r)
		}
		last = f
	}
	now.Part = partLoading
	for _, at := range []float64{0, 5, 19, 25, 60} {
		now.InPart = at
		check("loading")
	}
	now.Part, now.InPart = partReading, 0
	for done := 0; done <= 9000; done += 1500 {
		now.ReadDone, now.ReadOf = done, 9000
		now.InPart += 3
		check("reading")
	}
	now.InPart = 80 // reading is done and the model is thinking
	check("thinking")
	now.Part, now.InPart = partWriting, 0
	for clip := 0; clip < 12; clip++ {
		// Some clips come quickly and some take far longer than before.
		// Time only goes forward.
		for _, wait := range []float64{0, 2, 4, 24} {
			now.InPart += wait
			check("writing")
		}
		now.Taken++
		if clip > 1 {
			now.Landed++
		}
		check("a clip taken")
	}
	now.Part, now.InPart = partFraming, 0
	for now.Landed < 12 {
		now.Landed++
		now.InPart++
		check("framing")
	}
}

func TestTheModelsOwnCountBeatsTheClock(t *testing.T) {
	now := searchNow{Part: partReading, Chars: 30000, Count: 12, Local: true, Loads: true,
		InPart: 3, ReadDone: 8000, ReadOf: 10000}
	byCount, _ := searchProgress(now, lastTime, true)
	now.ReadOf = 0
	byClock, _ := searchProgress(now, lastTime, true)
	if byCount <= byClock {
		t.Errorf("four fifths read reads %.3f, three seconds of thirty reads %.3f", byCount, byClock)
	}
}

func TestWritingNeverRunsPastTheClipBeingWritten(t *testing.T) {
	now := searchNow{Part: partWriting, Chars: 30000, Count: 12, Taken: 2, InPart: 1000}
	f, _ := searchProgress(now, lastTime, true)
	now.Taken = 3
	after, _ := searchProgress(now, lastTime, true)
	if f >= after {
		t.Errorf("waiting on the third clip reads %.3f, the third clip taken reads %.3f", f, after)
	}
}

func TestAServerAlreadyRunningHasNothingToLoad(t *testing.T) {
	now := searchNow{Part: partReading, Chars: 30000, Count: 12, Local: true, Loads: false}
	_, left := searchProgress(now, lastTime, true)
	// 30 reading, 48 writing, 3 framing, and none of the 20 loading.
	if left < 80 || left > 82 {
		t.Errorf("left %.1f", left)
	}
}

func TestTimingsAreKeptPerModelAndBlended(t *testing.T) {
	useSpeedFile(t)
	if _, known := pastSpeed("local:gemma"); known {
		t.Fatal("a record out of nowhere")
	}
	keepSpeed("local:gemma", searchSpeed{Load: 20, Read: 1000, Clip: 4, Tail: 2, Runs: 1}, true)
	keepSpeed("local:gemma", searchSpeed{Load: 40, Read: 3000, Clip: 6, Tail: 4, Runs: 1}, true)
	// Against a server that was already running: loading is not measured.
	keepSpeed("local:gemma", searchSpeed{Load: 0, Read: 2000, Clip: 5, Tail: 3, Runs: 1}, false)
	got, known := pastSpeed("local:gemma")
	want := searchSpeed{Load: 30, Read: 2000, Clip: 5, Tail: 3, Runs: 3}
	if !known || got != want {
		t.Errorf("got %+v, want %+v", got, want)
	}
	if _, known := pastSpeed("claude-sonnet-5"); known {
		t.Error("one model's timings were taken for another's")
	}

	// A record nobody could have measured is no record.
	keepSpeed("broken", searchSpeed{Read: 0, Clip: 5, Runs: 1}, false)
	if _, known := pastSpeed("broken"); known {
		t.Error("a search with no reading was kept")
	}
	path := speedFile()
	if err := os.WriteFile(path, []byte("{not json"), 0o644); err != nil {
		t.Fatal(err)
	}
	if _, known := pastSpeed("local:gemma"); known {
		t.Error("a broken file was read as a record")
	}
	keepSpeed("local:gemma", searchSpeed{Load: 20, Read: 1000, Clip: 4, Tail: 2, Runs: 1}, true)
	if _, known := pastSpeed("local:gemma"); !known {
		t.Error("a broken file was not replaced")
	}
}

// Many searches finishing at once, each on its own goroutine, keep every
// timing they report.
func TestTimingsKeptFromSeveralSearchesAtOnce(t *testing.T) {
	useSpeedFile(t)
	var wg sync.WaitGroup
	for i := range 16 {
		wg.Add(1)
		go func() {
			defer wg.Done()
			keepSpeed("model", searchSpeed{Read: float64(1000 + i), Clip: 4, Runs: 1}, false)
			pastSpeed("model")
		}()
	}
	wg.Wait()
	if got, _ := pastSpeed("model"); got.Runs != 16 {
		t.Errorf("%d of 16 searches kept", got.Runs)
	}
}

// A real search against the fake model, twice. The first has nothing to
// go on and says so, and leaves its timings. The second says how far it
// is, and every clip that lands is in the progress the window hears.
func TestASearchSaysHowFarItIsTheSecondTime(t *testing.T) {
	useSpeedFile(t)
	source := testEpisode(t, "40")
	SetTrainingDir(t.TempDir())
	var heard, asked int32
	server := fakeModel(t, &asked)
	defer server.Close()

	var mu sync.Mutex
	var progress []Event
	log := NewLog(&bytes.Buffer{}, false, false)
	log.SetSink(func(ev Event) {
		if ev.Kind == EventProgress {
			mu.Lock()
			progress = append(progress, ev)
			mu.Unlock()
		}
	})
	e := NewEngine(log)
	e.OpenRecognizer = func(string) (Recognizer, error) { return fakeRecognizer{&heard}, nil }
	base := DefaultOptions()
	base.LLMURL = server.URL
	base.ASRModel = t.TempDir()
	base.Width, base.Height = 360, 640
	p := NewProject(e, source, base)
	ctx := context.Background()
	if err := p.Transcribe(ctx); err != nil {
		t.Fatal(err)
	}

	search := func(from float64) []Event {
		t.Helper()
		mu.Lock()
		progress = nil
		mu.Unlock()
		if _, err := p.Plan(ctx, PlanRequest{From: from, To: from + 20, Count: 1}); err != nil {
			t.Fatalf("plan: %v %s", err, p.LastError())
		}
		mu.Lock()
		defer mu.Unlock()
		return append([]Event(nil), progress...)
	}

	first := search(0)
	found := 0
	for _, ev := range first {
		if ev.Fraction != Unknown {
			t.Errorf("the first search claimed a share with nothing to go on: %+v", ev)
		}
		found = max(found, ev.Found)
	}
	if found != 1 {
		t.Errorf("the clip that landed was not in the progress: %+v", first)
	}
	if _, known := pastSpeed(plannerName(base)); !known {
		t.Fatalf("the first search left no timings under %s", plannerName(base))
	}

	second := search(20)
	shared := false
	for _, ev := range second {
		if ev.Fraction != Unknown {
			shared = true
			if ev.Fraction < 0 || ev.Fraction > 0.99 {
				t.Errorf("share %v", ev.Fraction)
			}
		}
	}
	if !shared || atomic.LoadInt32(&asked) != 2 {
		t.Errorf("the second search never said how far it was: %+v", second)
	}
}
