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
// reading, 30 thinking at 50 tokens a second, 4 a clip for twelve clips, 3
// of framing at the end. 131 in all.
var lastTime = searchSpeed{Version: speedVersion, Load: 20, Read: 1000, Thought: 30, Rate: 50,
	Clip: 4, Tail: 3, Runs: 1}

func TestASearchWithNothingToGoOnSaysNoShare(t *testing.T) {
	f, r := searchProgress(searchNow{Part: partReading, Chars: 30000, Count: 12, Local: true}, searchSpeed{}, false)
	if f != Unknown || r != Unknown {
		t.Errorf("share %v, left %v", f, r)
	}
}

// Whatever happens, in whatever order the parts report, the share never
// goes back and never claims the whole before the search is over.
func TestTheShareOnlyGrows(t *testing.T) {
	now := searchNow{Chars: 30000, Count: 12, Local: true, Loads: true, Budget: -1}
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
	now.InPart = 80 // it has read it all and has not begun to think
	check("read")
	now.Part, now.InPart = partThinking, 0
	for thought := 0; thought < 4000; thought += 500 {
		now.Thought = thought
		now.InPart += 10
		check("thinking")
	}
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
	now := searchNow{Part: partReading, Chars: 30000, Count: 12, Local: true, Loads: false, Budget: -1}
	_, left := searchProgress(now, lastTime, true)
	// 30 reading, 30 thinking, 48 writing, 3 framing, and none of the 20
	// loading.
	if left < 110 || left > 112 {
		t.Errorf("left %.1f", left)
	}
}

// A budget is the most the model will think, however long it thought the
// time before. 500 tokens at 50 a second is 10 seconds, not 30.
func TestABudgetShortensTheThinking(t *testing.T) {
	now := searchNow{Part: partReading, Chars: 30000, Count: 12, Local: true, Budget: 500}
	if _, left := searchProgress(now, lastTime, true); left < 90 || left > 92 {
		t.Errorf("left %.1f, want 91", left)
	}
	// Halfway through the budget by its own count is halfway through the
	// thinking, whatever the clock says.
	now.Part, now.Thought, now.InPart = partThinking, 250, 1
	f, _ := searchProgress(now, lastTime, true)
	want := (30 + 5) / 91.0
	if f < want-0.01 || f > want+0.01 {
		t.Errorf("share %.3f, want %.3f", f, want)
	}
}

// A local model this machine has never timed is measured against the
// stand-in rather than not at all. A record of what an older version
// measured is no record.
func TestALocalModelSaysHowFarItIsTheFirstTime(t *testing.T) {
	useSpeedFile(t)
	if got, known := pastSpeed("local:new", true); !known || got != measuredLocal {
		t.Errorf("got %+v %v", got, known)
	}
	if _, known := pastSpeed("claude-sonnet-5", false); known {
		t.Error("the API was measured against a local model")
	}
	old := `{"local:old": {"load": 24, "read": 127, "clip": 1, "tail": 28, "runs": 1}}`
	if err := os.WriteFile(speedFile(), []byte(old), 0o644); err != nil {
		t.Fatal(err)
	}
	if got, _ := pastSpeed("local:old", true); got != measuredLocal {
		t.Errorf("an old record was read as a new one: %+v", got)
	}
}

func TestTimingsAreKeptPerModelAndBlended(t *testing.T) {
	useSpeedFile(t)
	if _, known := pastSpeed("gemma", false); known {
		t.Fatal("a record out of nowhere")
	}
	keepSpeed("gemma", searchSpeed{Load: 20, Read: 1000, Thought: 20, Rate: 40, Clip: 4, Tail: 2, Runs: 1}, true)
	keepSpeed("gemma", searchSpeed{Load: 40, Read: 3000, Thought: 40, Rate: 60, Clip: 6, Tail: 4, Runs: 1}, true)
	// Against a server that was already running, and without thinking:
	// neither loading nor the speed of thought is measured.
	keepSpeed("gemma", searchSpeed{Load: 0, Read: 2000, Thought: 0, Clip: 5, Tail: 3, Runs: 1}, false)
	got, known := pastSpeed("gemma", false)
	want := searchSpeed{Version: speedVersion, Load: 30, Read: 2000, Thought: 15, Rate: 50,
		Clip: 5, Tail: 3, Runs: 3}
	if !known || got != want {
		t.Errorf("got %+v, want %+v", got, want)
	}
	if _, known := pastSpeed("claude-sonnet-5", false); known {
		t.Error("one model's timings were taken for another's")
	}

	// A record nobody could have measured is no record.
	keepSpeed("broken", searchSpeed{Read: 0, Clip: 5, Runs: 1}, false)
	if _, known := pastSpeed("broken", false); known {
		t.Error("a search with no reading was kept")
	}
	path := speedFile()
	if err := os.WriteFile(path, []byte("{not json"), 0o644); err != nil {
		t.Fatal(err)
	}
	if _, known := pastSpeed("gemma", false); known {
		t.Error("a broken file was read as a record")
	}
	keepSpeed("gemma", searchSpeed{Load: 20, Read: 1000, Clip: 4, Tail: 2, Runs: 1}, true)
	if _, known := pastSpeed("gemma", false); !known {
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
			pastSpeed("model", false)
		}()
	}
	wg.Wait()
	if got, _ := pastSpeed("model", false); got.Runs != 16 {
		t.Errorf("%d of 16 searches kept", got.Runs)
	}
}

// A real search against the fake model, twice. The first is measured
// against the stand-in and leaves its own timings, which the second is
// measured against. Both say how far they are, and every clip that lands
// is in the progress the app hears.
func TestASearchSaysHowFarItIs(t *testing.T) {
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

	says := func(events []Event) {
		t.Helper()
		shared := false
		for _, ev := range events {
			if ev.Fraction != Unknown {
				shared = true
				if ev.Fraction < 0 || ev.Fraction > 0.99 {
					t.Errorf("share %v", ev.Fraction)
				}
			}
		}
		if !shared {
			t.Errorf("the search never said how far it was: %+v", events)
		}
	}
	first := search(0)
	says(first)
	found := 0
	for _, ev := range first {
		found = max(found, ev.Found)
	}
	if found != 1 {
		t.Errorf("the clip that landed was not in the progress: %+v", first)
	}
	if got, known := readSpeeds(speedFile())[plannerName(base)]; !known || !got.usable() {
		t.Fatalf("the first search left no timings under %s", plannerName(base))
	}
	says(search(20))
	if atomic.LoadInt32(&asked) != 2 {
		t.Errorf("the model was asked %d times", atomic.LoadInt32(&asked))
	}

	// One file per answer: the saved reply, with how the model got to it.
	replies, _ := filepath.Glob(filepath.Join(p.LogsDir(), "reply-*.json"))
	if len(replies) != 2 {
		t.Fatalf("%d saved replies", len(replies))
	}
	body, _ := os.ReadFile(replies[0])
	if !bytes.Contains(body, []byte(`"how"`)) || !bytes.Contains(body, []byte(`"completion_tokens"`)) {
		t.Errorf("the reply does not say how it was made:\n%s", body)
	}
	if _, err := os.Stat(filepath.Join(p.LogsDir(), "plan-response.json")); !os.IsNotExist(err) {
		t.Error("the answer was written twice")
	}
}
