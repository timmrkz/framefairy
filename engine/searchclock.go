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
// A search is four parts one after the other: loading the model, the model
// reading the transcript, the model writing its clips, and the framing of
// the last of them once it has stopped. How long each takes depends on the
// machine, the model and the stretch, so nothing here is a guess. The parts
// are timed on every search that finishes and kept, per model, and the next
// search on this machine is measured against them. The very first one has
// nothing to be measured against and says what it is doing without saying
// how far it is.
// ---------------------------------------------------------------------------

// searchSpeed is how long the parts of a search took on this machine, for
// one model.
type searchSpeed struct {
	// Load is the seconds it took to load the model. Nothing for the API.
	Load float64 `json:"load"`
	// Read is transcript characters read per second, up to the first word
	// of the answer, thinking included.
	Read float64 `json:"read"`
	// Clip is the seconds each clip took to write.
	Clip float64 `json:"clip"`
	// Tail is the seconds from the end of the answer to the last clip in
	// the plan, which is the framing still going when the model stops.
	Tail float64 `json:"tail"`
	Runs int     `json:"runs"`
}

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
// there has been one worth measuring against.
func pastSpeed(model string) (searchSpeed, bool) {
	past, ok := readSpeeds(speedFile())[model]
	return past, ok && past.usable()
}

// usable says whether a record can measure a search at all. A file edited
// by hand, or a number nobody could have measured, is no record.
func (s searchSpeed) usable() bool {
	return s.Runs > 0 && s.Read > 0 && s.Clip > 0 && s.Load >= 0 && s.Tail >= 0 &&
		s.Read < 1e9 && s.Clip < 1e6 && s.Load < 1e5 && s.Tail < 1e5
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
	if path == "" || !took.usable() {
		return
	}
	speedWrite.Lock()
	defer speedWrite.Unlock()
	speeds := readSpeeds(path)
	was, known := speeds[model]
	known = known && was.usable()
	now := searchSpeed{
		Load: was.Load,
		Read: blend(was.Read, took.Read, known),
		Clip: blend(was.Clip, took.Clip, known),
		Tail: blend(was.Tail, took.Tail, known),
		Runs: was.Runs + 1,
	}
	// A search against a server that was already running loaded nothing,
	// which says nothing about how long loading takes.
	if loaded {
		now.Load = blend(was.Load, took.Load, known && was.Load > 0)
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
	partLoading = "loading"
	partReading = "reading"
	partWriting = "writing"
	partFraming = "framing"
)

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
	Taken, Landed    int
	Count            int
}

// searchProgress says how far a search is, as a share and the seconds
// left, against how long the same parts took before. Without a past search
// neither can be known.
func searchProgress(now searchNow, past searchSpeed, known bool) (float64, float64) {
	if !known || now.Count <= 0 {
		return Unknown, Unknown
	}
	load := 0.0
	if now.Local && now.Loads {
		load = past.Load
	}
	read := float64(now.Chars) / past.Read
	write := float64(now.Count) * past.Clip
	tail := past.Tail
	total := load + read + write + tail
	if total <= 0 {
		return Unknown, Unknown
	}
	// Each part gives its share done and its own time left. A share never
	// reaches the whole of a part by the clock alone: a part that runs
	// longer than it did before waits at the end of its share rather than
	// running into the next.
	byClock := func(took float64, whole float64) float64 {
		if whole <= 0 {
			return 1
		}
		return min(took/whole, 0.95)
	}
	var before, share, part float64
	switch now.Part {
	case partLoading:
		part = load
		share = byClock(now.InPart, load)
	case partReading:
		before, part = load, read
		share = byClock(now.InPart, read)
		if now.ReadOf > 0 {
			// The model's own count, which beats the clock. Once it has
			// read it all it may still be thinking, which it does not
			// count, so the clock takes over at the end.
			share = max(share, 0.97*float64(now.ReadDone)/float64(now.ReadOf))
		}
	case partWriting:
		before, part = load+read, write
		written := float64(now.Taken) / float64(now.Count)
		// Between two clips the clock moves the share on, never past the
		// clip that is being written.
		share = min(max(written, byClock(now.InPart, write)),
			float64(now.Taken+1)/float64(now.Count)-0.001)
		share = max(share, written)
	case partFraming:
		before, part = load+read+write, tail
		share = byClock(now.InPart, tail)
		if now.Taken > 0 {
			share = max(share, float64(now.Landed)/float64(now.Taken))
		}
	default:
		return Unknown, Unknown
	}
	share = min(max(share, 0), 1)
	done := before + share*part
	left := (1 - share) * part
	switch now.Part {
	case partLoading:
		left += read + write + tail
	case partReading:
		left += write + tail
	case partWriting:
		left += tail
	}
	return min(done/total, 0.99), max(left, 0)
}

// searchLabel is what the search is doing, in the words the window shows.
func searchLabel(now searchNow) string {
	switch now.Part {
	case partLoading:
		return "Loading the model"
	case partReading:
		return "Reading the transcript"
	case partWriting:
		if now.Landed > 0 {
			return fmt.Sprintf("%d of %d found", now.Landed, now.Count)
		}
		return "Choosing the moments"
	case partFraming:
		return fmt.Sprintf("%d of %d found", now.Landed, max(now.Taken, now.Landed))
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
	started  map[string]time.Time
	took     map[string]float64
	answered time.Time
	lastLand time.Time
	stopped  chan struct{}
	once     sync.Once
}

func newSearchClock(log *Log, model string, local bool, chars, count int) *searchClock {
	past, known := pastSpeed(model)
	c := &searchClock{log: log, model: model, past: past, known: known,
		started: map[string]time.Time{}, took: map[string]float64{},
		stopped: make(chan struct{})}
	c.now = searchNow{Local: local, Chars: chars, Count: count}
	return c
}

// run reports until stop.
func (c *searchClock) run() {
	ticker := time.NewTicker(500 * time.Millisecond)
	defer ticker.Stop()
	for {
		select {
		case <-c.stopped:
			return
		case <-ticker.C:
			c.report()
		}
	}
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

// enter begins a part. A part once left is never entered again.
func (c *searchClock) enter(part string) {
	c.mu.Lock()
	defer c.mu.Unlock()
	if c.now.Part == part {
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
	c.started[part] = at
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

// answered is the model finished, with the framing of its last clips still
// going.
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
	took := searchSpeed{
		Load: c.took[partLoading],
		Read: float64(c.now.Chars) / readTook,
		Clip: writeTook / float64(c.now.Taken),
		Tail: tail,
		Runs: 1,
	}
	loaded := c.now.Loads && took.Load > 0
	keepSpeed(c.model, took, loaded)
}
