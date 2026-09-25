package engine

import (
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"sync"
	"time"
)

// ---------------------------------------------------------------------------
// How far a search has come
//
// A search is five parts one after the other: loading the model, the model
// reading the transcript, the model thinking, the model writing its clips,
// and the framing of the last of them once it has stopped. How long each
// takes depends on the machine, the model and the window, so the parts are
// timed on every search that finishes and kept, per model, and the next
// search on this machine is measured against them. Inside a part, whatever
// the model counts itself beats the clock: the tokens of the transcript it
// has read, the tokens it has thought against its budget, the clips it has
// written.
//
// A model on this machine that has never been timed here is measured
// against a search timed on an M2 Max until it has been, which is close
// enough to say how far it is and is corrected by the first search that
// finishes. The API has no such stand-in: it says what it is doing without
// saying how far it is, until it has been timed once.
// ---------------------------------------------------------------------------

// speedVersion is raised whenever what a record measures changes. A record
// of another version measured something else and is no record.
const speedVersion = 2

// searchSpeed is how long the parts of a search took on this machine, for
// one model.
type searchSpeed struct {
	Version int `json:"version"`
	// Load is the seconds it took to load the model. Nothing for the API.
	Load float64 `json:"load"`
	// Read is transcript characters read per second, up to the first word
	// of thought or of the answer.
	Read float64 `json:"read"`
	// Thought is the seconds the model thought, and Rate the tokens it
	// thought a second, where it can be counted.
	Thought float64 `json:"thought"`
	Rate    float64 `json:"rate"`
	// Clip is the seconds each clip took to write.
	Clip float64 `json:"clip"`
	// Tail is the seconds from the end of the answer to the last clip in
	// the plan, which is the framing still going when the model stops.
	Tail float64 `json:"tail"`
	Runs int     `json:"runs"`
}

// measuredLocal stands in for a local model this machine has not timed yet:
// Gemma 4 26B A4B on an M2 Max with 32 GB, reading a half hour window of
// German, 16,000 tokens.
var measuredLocal = searchSpeed{Version: speedVersion, Load: 24, Read: 1170,
	Thought: 260, Rate: 47, Clip: 1.1, Tail: 28, Runs: 1}

var (
	// speedMu guards where the file is. speedWrite is held while it is
	// read, changed and written back, so two searches that finish at once
	// do not each write over the other.
	speedMu    sync.Mutex
	speedPath  string
	speedWrite sync.Mutex
)

// SetSpeedFile says where the timings of past searches are kept. Empty is
// the default, beside the models in ~/.framefairy. It is a file of its own
// rather than a setting, because it describes the machine, not a choice.
func SetSpeedFile(path string) {
	speedMu.Lock()
	defer speedMu.Unlock()
	speedPath = path
}

func speedFile() string {
	speedMu.Lock()
	defer speedMu.Unlock()
	if speedPath != "" {
		return speedPath
	}
	home, err := os.UserHomeDir()
	if err != nil {
		return ""
	}
	return filepath.Join(home, ".framefairy", "speed.json")
}

func readSpeeds(path string) map[string]searchSpeed {
	speeds := map[string]searchSpeed{}
	if path == "" {
		return speeds
	}
	data, err := os.ReadFile(path)
	if err != nil {
		return speeds
	}
	if json.Unmarshal(data, &speeds) != nil {
		return map[string]searchSpeed{}
	}
	return speeds
}

// pastSpeed is what searches with this model took before, and whether
// there is anything to measure against. A local model never timed here is
// measured against the stand-in.
func pastSpeed(model string, local bool) (searchSpeed, bool) {
	past, ok := readSpeeds(speedFile())[model]
	if ok && past.usable() {
		return past, true
	}
	if local {
		return measuredLocal, true
	}
	return searchSpeed{}, false
}

// usable says whether a record can measure a search at all. A file edited
// by hand, a record of what an older version measured, or a number nobody
// could have measured, is no record.
func (s searchSpeed) usable() bool {
	return s.Version == speedVersion && s.Runs > 0 && s.Read > 0 && s.Clip > 0 &&
		s.Load >= 0 && s.Tail >= 0 && s.Thought >= 0 && s.Rate >= 0 &&
		s.Read < 1e9 && s.Clip < 1e6 && s.Load < 1e5 && s.Tail < 1e5 &&
		s.Thought < 1e5 && s.Rate < 1e6
}

// blend takes a new timing into the record. Half of it and half of what
// was there, so one slow search on a busy machine moves the next estimate
// without taking it over.
func blend(was, now float64, known bool) float64 {
	if !known || was <= 0 {
		return now
	}
	return (was + now) / 2
}

// keepSpeed records how long the parts of a finished search took.
func keepSpeed(model string, took searchSpeed, loaded bool) {
	path := speedFile()
	took.Version = speedVersion
	if path == "" || !took.usable() {
		return
	}
	speedWrite.Lock()
	defer speedWrite.Unlock()
	speeds := readSpeeds(path)
	was, known := speeds[model]
	known = known && was.usable()
	if !known {
		was = searchSpeed{}
	}
	now := searchSpeed{
		Version: speedVersion,
		Load:    was.Load,
		Read:    blend(was.Read, took.Read, known),
		Thought: blend(was.Thought, took.Thought, known),
		Rate:    was.Rate,
		Clip:    blend(was.Clip, took.Clip, known),
		Tail:    blend(was.Tail, took.Tail, known),
		Runs:    was.Runs + 1,
	}
	// A search against a server that was already running loaded nothing,
	// which says nothing about how long loading takes. One that did not
	// think says nothing about how fast it thinks.
	if loaded {
		now.Load = blend(was.Load, took.Load, known && was.Load > 0)
	}
	if took.Rate > 0 {
		now.Rate = blend(was.Rate, took.Rate, known && was.Rate > 0)
	}
	speeds[model] = now
	body, err := json.MarshalIndent(speeds, "", "  ")
	if err != nil {
		return
	}
	if os.MkdirAll(filepath.Dir(path), 0o755) != nil {
		return
	}
	tmp, err := os.CreateTemp(filepath.Dir(path), ".speed-*.json")
	if err != nil {
		return
	}
	_, err = tmp.Write(body)
	if closeErr := tmp.Close(); err == nil {
		err = closeErr
	}
	if err != nil || os.Rename(tmp.Name(), path) != nil {
		os.Remove(tmp.Name())
	}
}

// The parts of a search, in the order they come.
const (
	partLoading  = "loading"
	partReading  = "reading"
	partThinking = "thinking"
	partWriting  = "writing"
	partFraming  = "framing"
)

var partOrder = []string{partLoading, partReading, partThinking, partWriting, partFraming}

func partIndex(part string) int {
	for i, p := range partOrder {
		if p == part {
			return i
		}
	}
	return -1
}

// searchNow is where a search is, as far as anything outside it can tell.
type searchNow struct {
	Part string
	// InPart is the seconds since the part began.
	InPart float64
	// Local is a model on this machine, which has to be loaded first. One
	// that was running already skips it.
	Local, Loads bool
	// ReadDone and ReadOf are the tokens read and to read, where the model
	// says. The API does not.
	ReadDone, ReadOf int
	Chars            int
	// Thought is the tokens the model has thought so far, and Budget the
	// most it may think, negative for no limit. Only a local model counts.
	Thought, Budget int
	Taken, Landed   int
	Count           int
}

// searchProgress says how far a search is, as a share and the seconds
// left, against how long the same parts took before. Without a past search
// neither can be known.
func searchProgress(now searchNow, past searchSpeed, known bool) (float64, float64) {
	if !known || now.Count <= 0 || past.Read <= 0 {
		return Unknown, Unknown
	}
	at := partIndex(now.Part)
	if at < 0 {
		return Unknown, Unknown
	}
	think := past.Thought
	if now.Local && now.Budget >= 0 && past.Rate > 0 {
		think = min(think, float64(now.Budget)/past.Rate)
	}
	took := []float64{0, float64(now.Chars) / past.Read, think,
		float64(now.Count) * past.Clip, past.Tail}
	if now.Local && now.Loads {
		took[0] = past.Load
	}
	total := 0.0
	for _, t := range took {
		total += t
	}
	if total <= 0 {
		return Unknown, Unknown
	}
	// Each part gives its share done. A share never reaches the whole of a
	// part by the clock alone: a part that runs longer than it did before
	// waits at the end of its share rather than running into the next.
	byClock := func(took float64, whole float64) float64 {
		if whole <= 0 {
			return 1
		}
		return min(took/whole, 0.95)
	}
	part := took[at]
	share := byClock(now.InPart, part)
	switch now.Part {
	case partReading:
		if now.ReadOf > 0 {
			// The model's own count, which beats the clock.
			share = max(share, 0.97*float64(now.ReadDone)/float64(now.ReadOf))
		}
	case partThinking:
		if now.Local && past.Rate > 0 && part > 0 {
			// The tokens thought against the tokens it thought before, or
			// may think at most.
			share = max(share, min(float64(now.Thought)/(part*past.Rate), 0.97))
		}
	case partWriting:
		written := float64(now.Taken) / float64(now.Count)
		// Between two clips the clock moves the share on, never past the
		// clip that is being written.
		share = min(max(written, share), float64(now.Taken+1)/float64(now.Count)-0.001)
		share = max(share, written)
	case partFraming:
		if now.Taken > 0 {
			share = max(share, float64(now.Landed)/float64(now.Taken))
		}
	}
	share = min(max(share, 0), 1)
	before, after := 0.0, 0.0
	for i, t := range took {
		if i < at {
			before += t
		} else if i > at {
			after += t
		}
	}
	done := before + share*part
	left := (1-share)*part + after
	return min(done/total, 0.99), max(left, 0)
}

// searchLabel is what the search is doing, in the words the app shows. It
// says one thing until there is a clip to count. Loading, reading and
// thinking are the machine's steps, not something a person waiting for
// clips has to follow, and a line that changes its words every few seconds
// is a line nobody reads. How far it has come is the fill and the time
// left.
func searchLabel(now searchNow) string {
	if now.Landed > 0 {
		switch now.Part {
		case partWriting:
			return fmt.Sprintf("%d of %d found", now.Landed, max(now.Count, now.Landed))
		case partFraming:
			// The answer is whole, so what it holds is what there will be.
			return fmt.Sprintf("%d of %d found", now.Landed, max(now.Taken, now.Landed))
		}
	}
	return "Finding clips"
}

// searchClock watches one search and reports how far it has come about
// twice a second, which is also what moves the share on between the
// moments anything happens.
type searchClock struct {
	log   *Log
	model string
	past  searchSpeed
	known bool

	mu       sync.Mutex
	now      searchNow
	partAt   time.Time
	took     map[string]float64
	answered time.Time
	lastLand time.Time
	stopped  chan struct{}
	once     sync.Once
}

// newSearchClock starts watching a search of chars characters of
// transcript for count clips. budget is the most a local model may think,
// negative for no limit.
func newSearchClock(log *Log, model string, local bool, chars, count, budget int) *searchClock {
	past, known := pastSpeed(model, local)
	c := &searchClock{log: log, model: model, past: past, known: known,
		took: map[string]float64{}, stopped: make(chan struct{})}
	c.now = searchNow{Local: local, Chars: chars, Count: count, Budget: budget}
	return c
}

// run reports until stop, and holds the progress line meanwhile.
func (c *searchClock) run() {
	c.log.HoldProgress(true)
	defer c.log.HoldProgress(false)
	ticker := time.NewTicker(500 * time.Millisecond)
	defer ticker.Stop()
	for {
		select {
		case <-c.stopped:
			return
		case <-ticker.C:
			if !c.reportSafely() {
				return
			}
		}
	}
}

// reportSafely reports, and says false if reporting panicked. The clock is
// a goroutine of its own, out of reach of the recover that guards the
// job, and a panic in it ended the whole app. A clock that cannot report
// stops, and the search goes on without a fill.
func (c *searchClock) reportSafely() (alive bool) {
	defer func() {
		if caught := recover(); caught != nil {
			c.log.Detail("the search stopped reporting its progress: %v", caught)
			alive = false
		}
	}()
	c.report()
	return true
}

func (c *searchClock) stop() {
	c.once.Do(func() { close(c.stopped) })
}

func (c *searchClock) report() {
	c.mu.Lock()
	now := c.now
	if !c.partAt.IsZero() {
		now.InPart = time.Since(c.partAt).Seconds()
	}
	c.mu.Unlock()
	if now.Part == "" {
		return
	}
	fraction, remaining := searchProgress(now, c.past, c.known)
	c.log.ProgressFound(searchLabel(now), fraction, remaining, now.Landed)
}

// enter begins a part. Parts only go forward: a word of thought that comes
// after the answer has begun does not take the search back.
func (c *searchClock) enter(part string) {
	c.mu.Lock()
	defer c.mu.Unlock()
	if partIndex(part) <= partIndex(c.now.Part) {
		return
	}
	at := time.Now()
	if c.now.Part != "" {
		c.took[c.now.Part] = at.Sub(c.partAt).Seconds()
	}
	if part == partLoading {
		c.now.Loads = true
	}
	c.now.Part, c.partAt = part, at
}

// listen is what the clock hears from the model while it answers.
func (c *searchClock) listen(inner *Listener) *Listener {
	return &Listener{
		Text: func(piece string) {
			c.enter(partWriting)
			if inner != nil && inner.Text != nil {
				inner.Text(piece)
			}
		},
		Thinking: func(chars int) {
			c.enter(partThinking)
			// A local model sends its thought a token at a time.
			c.mu.Lock()
			c.now.Thought++
			c.mu.Unlock()
			if inner != nil && inner.Thinking != nil {
				inner.Thinking(chars)
			}
		},
		Reading: func(done, total int) {
			c.mu.Lock()
			c.now.ReadDone, c.now.ReadOf = done, total
			c.mu.Unlock()
		},
		Part: c.enter,
	}
}

// taken is a clip read out of the answer, and landed one written to the
// plan.
func (c *searchClock) taken() {
	c.mu.Lock()
	c.now.Taken++
	c.mu.Unlock()
}

func (c *searchClock) landed() {
	c.mu.Lock()
	c.now.Landed++
	c.lastLand = time.Now()
	c.mu.Unlock()
	c.report()
}

// answerDone is the model finished, with the framing of its last clips
// still going.
func (c *searchClock) answerDone() {
	c.enter(partFraming)
	c.mu.Lock()
	c.answered = time.Now()
	c.mu.Unlock()
}

// finish stops the clock and, for a search that went through every part,
// keeps how long each took for the next.
func (c *searchClock) finish(complete bool) {
	c.stop()
	c.mu.Lock()
	defer c.mu.Unlock()
	if !complete || c.now.Taken == 0 || c.now.Chars == 0 {
		return
	}
	readTook, read := c.took[partReading]
	writeTook, wrote := c.took[partWriting]
	if !read || !wrote || readTook <= 0 || writeTook <= 0 {
		return
	}
	tail := 0.0
	if !c.answered.IsZero() && c.lastLand.After(c.answered) {
		tail = c.lastLand.Sub(c.answered).Seconds()
	}
	thought := c.took[partThinking]
	took := searchSpeed{
		Load:    c.took[partLoading],
		Read:    float64(c.now.Chars) / readTook,
		Thought: thought,
		Clip:    writeTook / float64(c.now.Taken),
		Tail:    tail,
		Runs:    1,
	}
	if c.now.Local && thought > 0 && c.now.Thought > 0 {
		took.Rate = float64(c.now.Thought) / thought
	}
	loaded := c.now.Loads && took.Load > 0
	keepSpeed(c.model, took, loaded)
}
