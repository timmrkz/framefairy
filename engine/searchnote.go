package engine

import (
	"encoding/json"
	"errors"
	"fmt"
	"io/fs"
	"math"
	"os"
	"path/filepath"
	"strings"
	"time"
)

// How the last search of an episode ended, when it did not end with clips.
//
// A search that stops before it is done leaves an empty clip list and, until
// this, no word about why: the reason was shown while the screen that asked
// for it was open, and gone after that, and a search the app was closed in
// the middle of left nothing at all. So a search writes down that it is
// running when it starts, and what became of it when it ends. It takes the
// note away again when it finds its clips or is called off by hand, because
// then there is nothing to say. A note that still says running when the
// episode is opened again is a search the app was closed or fell over in.

// SearchNote is what the work folder remembers about the last search.
type SearchNote struct {
	// State is "running" while a search runs, and still says so when the
	// app went away in the middle of one, or "failed" with its reason.
	State string `json:"state"`
	// From and To are the window it was asked about, in seconds. To is 0
	// for the end of the episode.
	From  float64   `json:"from"`
	To    float64   `json:"to"`
	Error string    `json:"error,omitempty"`
	At    time.Time `json:"at"`
}

const searchNoteName = "search.json"

// noteLimit is the most of a reason a note keeps. A reason is one line
// said to a person, and a note is read by the app every time it opens.
const noteLimit = 500

func searchNotePath(source string) string {
	return filepath.Join(WorkDir(source), searchNoteName)
}

func writeSearchNote(source string, note SearchNote) error {
	if err := os.MkdirAll(WorkDir(source), 0o755); err != nil {
		return err
	}
	body, err := json.Marshal(note)
	if err != nil {
		return err
	}
	return writeAtomic(searchNotePath(source), body)
}

// ClearSearchNote forgets how the last search ended.
func ClearSearchNote(source string) error {
	err := os.Remove(searchNotePath(source))
	if errors.Is(err, fs.ErrNotExist) {
		return nil
	}
	return err
}

// ReadSearchNote gives how the last search ended, or nil when there is
// nothing to say. The file is read like anything else on disk, as
// untrusted: a note that is not one of the two it can be is no note.
func ReadSearchNote(source string) *SearchNote {
	body, err := os.ReadFile(searchNotePath(source))
	if err != nil || len(body) > 64<<10 {
		return nil
	}
	var note SearchNote
	if json.Unmarshal(body, &note) != nil {
		return nil
	}
	if note.State != "running" && note.State != "failed" {
		return nil
	}
	finite := func(v float64) bool { return !math.IsNaN(v) && !math.IsInf(v, 0) && v >= 0 }
	if !finite(note.From) || !finite(note.To) || (note.To > 0 && note.To <= note.From) {
		return nil
	}
	note.Error = strings.TrimSpace(note.Error)
	if r := []rune(note.Error); len(r) > noteLimit {
		note.Error = string(r[:noteLimit])
	}
	return &note
}

// noted runs a search and writes down how it ended. A search that panics
// is noted as failed with what it panicked with, and the panic goes on to
// whoever catches it, so the job it runs in still fails itself.
func (p *Project) noted(from, to float64, search func() error) (err error) {
	note := SearchNote{State: "running", From: from, To: to, At: time.Now()}
	if werr := writeSearchNote(p.Source, note); werr != nil {
		p.engine.Log.Warn("could not note the search: %s", werr)
	}
	defer func() {
		if r := recover(); r != nil {
			note.State, note.Error = "failed", fmt.Sprint(r)
			_ = writeSearchNote(p.Source, note)
			panic(r)
		}
		switch {
		case err == nil, errors.Is(err, ErrCancelled):
			_ = ClearSearchNote(p.Source)
		default:
			note.State = "failed"
			note.Error = p.LastError()
			if note.Error == "" {
				note.Error = err.Error()
			}
			_ = writeSearchNote(p.Source, note)
		}
	}()
	return search()
}
