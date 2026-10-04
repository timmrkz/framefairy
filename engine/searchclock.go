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
// A search is six parts one after the other: loading the model, the model
// reading the transcript, the model thinking, the model writing its clips,
// a local model asked again about the clips well off the length, and the
// framing of the last of them once that is done. How long each
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
// finishes. A model in the cloud the app knows by name is measured against
// a first guess the same way, so the first search on it moves too rather
// than showing only that it runs. A model nobody has heard of says what it
// is doing without saying how far, until it has been timed once.
// ---------------------------------------------------------------------------

// speedVersion is raised whenever what a record measures changes. A record
// of another version measured something else and is no record.
// 3 gave fitting a part of its own. Before, the tail ran from the end of
// the answer and held the fitting too. 4 measures the fitting and the
// tail per clip, because both take as long as the clips they wait on.
// 5 times searches that ask middle, which reads two thirds of what lines
// read and thinks a fraction as long. A search timed with lines would have
// the first searches with middle wait in the thinking and then jump.
const speedVersion = 5

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
	// Fit is the seconds it took a local model to answer about each clip
	// held back to be fitted to the length, when it was asked.
	Fit float64 `json:"fit"`
	// Tail is the seconds the framing took after the answer, and after
	// the fitting where there was one, for each round of the framers: the
	// clips still to frame then, as many at a time as there are framers.
	// It is the framing still going when the model stops.
	Tail float64 `json:"tail"`
	Runs int     `json:"runs"`
}

// measuredLocal stands in for a local model this machine has not timed yet:
// Gemma 4 26B A4B on an M2 Max with 32 GB, asked with middle about a half
// hour window of German in Tim's runs of 3 October. It read 27,125
// characters in about 12 seconds, thought its 2,048 tokens in about 30
// and wrote six stories in a few. The load, the fitting and the framing
// after the answer were timed with lines, and do not depend on how the
// model is asked. middle is never asked again about a clip, so its
// fitting is only there for a recipe that is.
var measuredLocal = searchSpeed{Version: speedVersion, Load: 24, Read: 2260,
	Thought: 30, Rate: 68, Clip: 0.5, Fit: 6, Tail: 16, Runs: 1}

// measuredCloud stands in for a model in the cloud this machine has not
// timed yet. It is a guess, not a measurement: a half hour window read in
// a few seconds, a minute of thought, a few seconds a clip, and the
// framing on this machine as long as it takes after a local model. The
// first search that finishes replaces half of it, and the next the rest.
var measuredCloud = searchSpeed{Version: speedVersion, Load: 0, Read: 20000,
	Thought: 60, Rate: 0, Clip: 3, Tail: 16, Runs: 1}

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
// measured against its stand-in, and a model in the cloud the app knows by
// name against its own.
func pastSpeed(model string, local bool) (searchSpeed, bool) {
	past, ok := readSpeeds(speedFile())[model]
	if ok && past.usable() {
		return past, true
	}
	if local {
		return measuredLocal, true
	}
	if _, named := models[model]; named {
		return measuredCloud, true
	}
	return searchSpeed{}, false
}

// usable says whether a record can measure a search at all. A file edited
// by hand, a record of what an older version measured, or a number nobody
// could have measured, is no record.
func (s searchSpeed) usable() bool {
	return s.Version == speedVersion && s.Runs > 0 && s.Read > 0 && s.Clip > 0 &&
		s.Load >= 0 && s.Tail >= 0 && s.Thought >= 0 && s.Rate >= 0 && s.Fit >= 0 &&
		s.Read < 1e9 && s.Clip < 1e6 && s.Load < 1e5 && s.Tail < 1e5 && s.Fit < 1e5 &&
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
		Fit:     was.Fit,
		Tail:    was.Tail,
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
	// Nor does one that asked nothing again about how long asking takes.
	if took.Fit > 0 {
		now.Fit = blend(was.Fit, took.Fit, known && was.Fit > 0)
	}
	// Nor does one that had every clip framed by the time the model
	// stopped about how long a clip still to frame takes. It was taken as
	// no time at all, and halved the record each time, until a search
	// with four clips to frame after the fitting expected none.
	if took.Tail > 0 {
		now.Tail = blend(was.Tail, took.Tail, known && was.Tail > 0)
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
	partFitting  = "fitting"
	partFraming  = "framing"
)

var partOrder = []string{partLoading, partReading, partThinking, partWriting, partFitting,
	partFraming}

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
	// that was running already skips it. Only a local model is asked again
	// about the clips well off the length, so only its search has a part
	// for the fitting, and only with a recipe that asks: NoFit is a recipe
	// that never does, as middle, whose search steps over it.
	Local, Loads, NoFit bool
	// ReadDone and ReadOf are the tokens read and to read, where the model
	// says. The API does not.
	ReadDone, ReadOf int
	Chars            int
	// Thought is the tokens the model has thought so far, and Budget the
	// most it may think, negative for no limit. Only a local model counts.
	Thought, Budget int
	// Named is the clips read out of the answer so far, and Held those of
	// them held back to be fitted to the length. Taken is the clips handed
	// to the framers, Framed those done with, and Landed those written to
	// the plan. A clip held back is taken once it is fitted.
	Named, Held           int
	Taken, Framed, Landed int
	// FitOf is the clips the model was asked about again so far, and
	// FitDone those it has given again.
	FitOf, FitDone int
	// FramedAt is how many clips were framed when the framing began, so
	// what is framed after it is what the framing part waits on. Framers
	// is how many clips are framed at once.
	FramedAt int
	Framers  int
	Count    int
}

// rounds is how many turns the framers take over clips, as many at a time
// as there are of them.
func rounds(clips, framers int) int {
	framers = max(framers, 1)
	return (max(clips, 0) + framers - 1) / framers
}

// searchProgress says how far a search is, as a share and the seconds
// left, against how long the same parts took before. Without a past search
// neither can be known. A part that runs past what it took before has no
// time left to say by the clock, see partLeft.
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
	answered := at >= partIndex(partFitting)
	// The fitting is counted from the start, whether or not a clip will
	// need it, because a share that learned of it only once the answer was
	// in would jump. Until the answer is whole it is taken as one clip, or
	// as many as are held back already. A search with nothing to fit steps
	// over it. It takes as long as the clips the model is asked about: one
	// constant for it said About 0:05 left for a minute while the model
	// was asked about four.
	fitClips := 0
	if now.Local && !now.NoFit {
		switch {
		case now.FitOf > 0:
			fitClips = now.FitOf
		case answered:
			fitClips = now.Held
		default:
			fitClips = max(now.Held, 1)
		}
	}
	// The framing after the answer waits on the clips not yet framed when
	// the model stops: the ones the framers have not caught up with and
	// the ones held back to be fitted. While the model writes, the framers
	// catch up with all but the last clip it writes, so that one is
	// counted.
	frameClips := 0
	switch {
	case now.Part == partFraming:
		frameClips = max(now.Taken-now.FramedAt, 0)
	case answered:
		frameClips = max(now.Taken-now.Framed, 0) + fitClips
	default:
		frameClips = 1 + fitClips
	}
	fit, tail := past.Fit, past.Tail
	if fit <= 0 {
		fit = measuredLocal.Fit
	}
	if tail <= 0 {
		tail = measuredLocal.Tail
	}
	took := []float64{0, float64(now.Chars) / past.Read, think,
		float64(now.Count) * past.Clip, float64(fitClips) * fit,
		float64(rounds(frameClips, now.Framers)) * tail}
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
	part := took[at]
	// counted is the share of the part the work counts itself, where it
	// does, and below 0 where it does not.
	counted := -1.0
	switch now.Part {
	case partReading:
		if now.ReadOf > 0 {
			counted = float64(now.ReadDone) / float64(now.ReadOf)
		}
	case partThinking:
		if now.Local && past.Rate > 0 && part > 0 {
			// The tokens thought against the tokens it thought before, or
			// may think at most.
			counted = float64(now.Thought) / (part * past.Rate)
		}
	case partWriting:
		counted = float64(now.Named) / float64(now.Count)
	case partFitting:
		if now.FitOf > 0 {
			counted = float64(now.FitDone) / float64(now.FitOf)
		}
	case partFraming:
		if frameClips > 0 {
			counted = float64(now.Framed-now.FramedAt) / float64(frameClips)
		}
	}
	counted = min(counted, 1)
	// Each part gives its share done. A share never reaches the whole of a
	// part by the clock alone: a part that runs longer than it did before
	// waits at the end of its share rather than running into the next.
	share := 1.0
	if part > 0 {
		share = min(now.InPart/part, 0.95)
	}
	switch now.Part {
	case partWriting:
		// Between two clips the clock moves the share on, never past the
		// clip that is being written.
		share = min(max(counted, share), float64(now.Named+1)/float64(now.Count)-0.001)
		share = max(share, counted)
	case partReading, partThinking:
		// The model's own count, which beats the clock, short of the whole
		// until the next part begins.
		share = max(share, min(0.97*counted, 0.97))
	default:
		share = max(share, counted)
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
	fraction := min(done/total, 0.99)
	left := partLeft(now.InPart, part, share, counted)
	if left < 0 {
		return fraction, Unknown
	}
	return fraction, left + after
}

// partLeft is the seconds left of a part estimated at part seconds, in
// it for inPart, with share done and counted done by its own count, below
// 0 where nothing is counted. Inside its estimate, what is left of it is
// left. Past it the estimate was wrong, and holding what was left of it
// said About 0:05 left for a minute: what is left is then worked out from
// how fast the count has gone, and without a count it cannot be said.
func partLeft(inPart, part, share, counted float64) float64 {
	if part <= 0 {
		// A part with nothing to wait on, like the framing when every clip
		// is framed already, is over as it begins.
		return 0
	}
	if inPart <= part {
		return (1 - share) * part
	}
	if counted > 0 {
		return inPart * (1 - counted) / counted
	}
	return Unknown
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
		case partFitting, partFraming:
			// The answer is whole, so what it holds is what there will be.
			return fmt.Sprintf("%d of %d found", now.Landed, max(now.Named, now.Landed))
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

	mu     sync.Mutex
	now    searchNow
	partAt time.Time
	took   map[string]float64
	// framingAt is when the framing began, and lastFramed when the last
	// clip was framed.
	framingAt, lastFramed time.Time
	// fitAsked is that the model was asked again about clips held back.
	fitAsked bool
	// shown is the share reported last. An estimate that grows, once the
	// answer says how many clips are held back, does not take the fill
	// back: it waits there until the work catches up.
	shown   float64
	stopped chan struct{}
	once    sync.Once
}

// newSearchClock starts watching a search of chars characters of
// transcript for count clips. budget is the most a local model may think,
// negative for no limit.
func newSearchClock(log *Log, model string, local bool, chars, count, budget int) *searchClock {
	past, known := pastSpeed(model, local)
	c := &searchClock{log: log, model: model, past: past, known: known,
		took: map[string]float64{}, stopped: make(chan struct{})}
	c.now = searchNow{Local: local, Chars: chars, Count: count, Budget: budget, Framers: framers}
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
	fraction, remaining := c.measure(now)
	c.log.ProgressFound(searchLabel(now), fraction, remaining, now.Landed)
}

// measure is searchProgress with a share that never goes back.
func (c *searchClock) measure(now searchNow) (float64, float64) {
	fraction, remaining := searchProgress(now, c.past, c.known)
	if fraction == Unknown {
		return fraction, remaining
	}
	c.mu.Lock()
	defer c.mu.Unlock()
	c.shown = max(c.shown, fraction)
	return c.shown, remaining
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
	if part == partFraming {
		c.now.FramedAt, c.framingAt = c.now.Framed, at
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

// named is a clip read out of the answer, held back to be fitted or not,
// taken one handed to the framers, framed one done with, written or left
// out, and landed one written to the plan.
func (c *searchClock) named(held bool) {
	c.mu.Lock()
	defer c.mu.Unlock()
	c.now.Named++
	if held {
		c.now.Held++
	}
}

func (c *searchClock) taken() {
	c.mu.Lock()
	c.now.Taken++
	c.mu.Unlock()
}

func (c *searchClock) framed() {
	c.mu.Lock()
	c.now.Framed++
	c.lastFramed = time.Now()
	c.mu.Unlock()
}

func (c *searchClock) landed() {
	c.mu.Lock()
	c.now.Landed++
	c.mu.Unlock()
	c.report()
}

// fitAsk is the model asked again about count clips, and what it hears
// counts the clips it gives again as they are written.
func (c *searchClock) fitAsk(count int) *Listener {
	if c == nil {
		return nil
	}
	c.mu.Lock()
	c.now.FitOf += count
	c.mu.Unlock()
	var scanner clipScanner
	return &Listener{Text: func(piece string) {
		whole := len(scanner.feed(piece))
		if whole == 0 {
			return
		}
		c.mu.Lock()
		c.now.FitDone = min(c.now.FitDone+whole, c.now.FitOf)
		c.mu.Unlock()
	}}
}

// answerDone is the model finished, with the framing of its last clips
// still going. A local model may be asked again first, see fitted.
func (c *searchClock) answerDone() {
	c.mu.Lock()
	local := c.now.Local
	c.mu.Unlock()
	if local {
		c.enter(partFitting)
	} else {
		c.enter(partFraming)
	}
}

// fitted is the clips held back fitted to the length, asked is whether the
// model was asked about any, and what is left is the framing.
func (c *searchClock) fitted(asked bool) {
	c.mu.Lock()
	c.fitAsked = c.fitAsked || asked
	c.mu.Unlock()
	c.enter(partFraming)
}

// finish stops the clock and, for a search that went through every part,
// keeps how long each took for the next.
func (c *searchClock) finish(complete bool) {
	c.stop()
	c.mu.Lock()
	defer c.mu.Unlock()
	if !complete || c.now.Taken == 0 || c.now.Named == 0 || c.now.Chars == 0 {
		return
	}
	readTook, read := c.took[partReading]
	writeTook, wrote := c.took[partWriting]
	if !read || !wrote || readTook <= 0 || writeTook <= 0 {
		return
	}
	// The tail runs from where the framing began, which is after the
	// fitting where there was one, and is kept per round of the framers
	// it waited on.
	tail := 0.0
	if waited := rounds(c.now.Taken-c.now.FramedAt, c.now.Framers); waited > 0 &&
		!c.framingAt.IsZero() && c.lastFramed.After(c.framingAt) {
		tail = c.lastFramed.Sub(c.framingAt).Seconds() / float64(waited)
	}
	fit := 0.0
	if c.fitAsked && c.now.FitOf > 0 {
		fit = c.took[partFitting] / float64(c.now.FitOf)
	}
	thought := c.took[partThinking]
	took := searchSpeed{
		Load:    c.took[partLoading],
		Read:    float64(c.now.Chars) / readTook,
		Thought: thought,
		Clip:    writeTook / float64(c.now.Named),
		Fit:     fit,
		Tail:    tail,
		Runs:    1,
	}
	if c.now.Local && thought > 0 && c.now.Thought > 0 {
		took.Rate = float64(c.now.Thought) / thought
	}
	loaded := c.now.Loads && took.Load > 0
	keepSpeed(c.model, took, loaded)
}
