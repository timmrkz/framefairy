package runlog

import (
	"bytes"
	"encoding/json"
	"errors"
	"strings"
	"sync"
	"testing"
)

type recorder struct {
	mu     sync.Mutex
	events []Event
}

func (r *recorder) sink(ev Event) {
	r.mu.Lock()
	defer r.mu.Unlock()
	r.events = append(r.events, ev)
}

func (r *recorder) kinds() []string {
	out := make([]string, len(r.events))
	for i, ev := range r.events {
		out[i] = string(ev.Kind)
	}
	return out
}

func TestEventsFollowTheLog(t *testing.T) {
	var out bytes.Buffer
	log := New(&out, true, false)
	rec := &recorder{}
	log.SetSink(rec.sink)

	log.Info("hello %d", 1)
	log.Warn("careful")
	log.Detail("hidden in the terminal")
	err := log.Step("transcribing the audio", func() error {
		log.ProgressOf("transcribing", 0.5, 12)
		log.Detail("a line in between keeps the progress open")
		log.ClearProgress()
		log.ClearProgress()
		return nil
	})
	if err != nil {
		t.Fatal(err)
	}
	_ = log.Step("planning", func() error { return errors.New("no model") })

	want := "info warn detail step-start progress detail idle step-done step-start step-failed"
	if got := strings.Join(rec.kinds(), " "); got != want {
		t.Fatalf("kinds\n got %s\nwant %s", got, want)
	}
	if rec.events[0].Text != "hello 1" {
		t.Errorf("text %q", rec.events[0].Text)
	}
	if strings.Contains(rec.events[1].Text, "\033") {
		t.Errorf("warn text carries colour codes: %q", rec.events[1].Text)
	}
	if strings.Contains(out.String(), "hidden") {
		t.Errorf("detail reached the terminal without --verbose")
	}
	progress := rec.events[4]
	if progress.Stage != "transcribing the audio" || progress.Fraction != 0.5 ||
		progress.Remaining != 12 {
		t.Errorf("progress event %+v", progress)
	}
	if rec.events[0].Stage != "" || rec.events[0].Fraction != Unknown {
		t.Errorf("event outside a step %+v", rec.events[0])
	}
	if failed := rec.events[9]; failed.Text != "no model" || failed.Stage != "planning" {
		t.Errorf("failed step %+v", failed)
	}
}

func TestProgressIsClamped(t *testing.T) {
	log := New(&bytes.Buffer{}, false, false)
	rec := &recorder{}
	log.SetSink(rec.sink)
	log.ProgressOf("rendering", 1.7, Unknown)
	log.ProgressOf("rendering", Unknown, Unknown)
	if rec.events[0].Fraction != 1 || rec.events[0].Remaining != Unknown {
		t.Errorf("clamped %+v", rec.events[0])
	}
	if rec.events[1].Fraction != Unknown {
		t.Errorf("unknown %+v", rec.events[1])
	}
}

func TestJSONLines(t *testing.T) {
	var buf bytes.Buffer
	log := New(&bytes.Buffer{}, false, false)
	log.SetSink(JSONLines(&buf))
	log.OK("done <3 & more")
	log.SetSink(nil)
	log.Info("not recorded")

	lines := strings.Split(strings.TrimSpace(buf.String()), "\n")
	if len(lines) != 1 {
		t.Fatalf("lines %q", lines)
	}
	var ev Event
	if err := json.Unmarshal([]byte(lines[0]), &ev); err != nil {
		t.Fatal(err)
	}
	if ev.Kind != EventOK || ev.Text != "done <3 & more" || ev.Time.IsZero() {
		t.Errorf("decoded %+v", ev)
	}
	if !strings.Contains(lines[0], "<3 & more") {
		t.Errorf("html escaped: %s", lines[0])
	}
}

// While a search holds the progress line, nothing that runs inside it ends
// the line: not a step that finishes, not ffmpeg clearing its own progress
// when it is done with a clip. Each did, the app took the search's fill
// away, and it came back with the search's next report.
func TestAHeldLineIsOnlyEverLetGoByWhoHoldsIt(t *testing.T) {
	log := New(&bytes.Buffer{}, false, false)
	rec := &recorder{}
	log.SetSink(rec.sink)
	log.HoldProgress(true)
	log.ProgressFound("Finding clips", 0.4, 30, 2)
	_ = log.Step("fitting 2 clip(s) to the length", func() error {
		log.ProgressOf("finding camera switches", 0.5, 1)
		log.ClearProgress()
		return nil
	})
	log.ClearProgress()
	if got, want := strings.Join(rec.kinds(), " "), "progress step-start step-done"; got != want {
		t.Fatalf("kinds while held\n got %s\nwant %s", got, want)
	}
	log.HoldProgress(false)
	log.ClearProgress()
	if got := rec.kinds(); got[len(got)-1] != "idle" {
		t.Errorf("let go, the line was not cleared: %v", got)
	}
	// Outside a hold a step that ends still ends its progress.
	rec.events = nil
	_ = log.Step("rendering", func() error {
		log.ProgressOf("rendering", 0.5, 1)
		return nil
	})
	if got, want := strings.Join(rec.kinds(), " "), "step-start progress idle step-done"; got != want {
		t.Errorf("kinds\n got %s\nwant %s", got, want)
	}
}
