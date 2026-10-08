package engine

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"runtime"
	"runtime/debug"
	"sort"
	"strconv"
	"strings"
	"sync"
	"unicode"
)

// ---------------------------------------------------------------------------
// A plan built as the answer arrives
//
// Three kinds of work make a search, and they no longer wait on each other.
// The model writes clips on the graphics side of the machine. Framing a clip
// decodes video with ffmpeg on the processor. Writing a clip to the plan is
// a few kilobytes on disk. So the answer is read as it is written, each clip
// is taken the moment it is whole, framed by one of a few framers while the
// model goes on writing, and written to the plan as soon as it is framed.
// The app reads the plan while the search runs and shows each clip as it
// lands.
//
// The clips are written in the order they are finished rather than the
// order they were written in. The app lists clips by where they are in
// the episode, never by where they are in the file, and a clip that has to
// wait for a longer one before it is written is a clip shown later than it
// could have been.
// ---------------------------------------------------------------------------

// framers is how many clips are framed at once. Once the model has
// written its answer, framing is all that is left: on an M2 Max the last
// clips took 25 seconds after the answer, two at a time. The decoding is
// the system decoder's and most of the rest is looking for faces, one core
// each, and the transcription waits while a search runs, so the processor
// has room for more. A third of the cores, at least two and at most four.
//
// The first clip is framed on its own. What a person waits for is the
// first clip on screen, and every clip framed beside it takes a share of
// the cores, of the memory and of the one system decoder from it. So the
// others wait until it has landed, and then as many go at once as there
// are framers, which is what gets the last one out soonest.
var framers = min(max(runtime.NumCPU()/3, 2), 4)

type planJob struct {
	index int
	// card is the clip's number on the list of clips on the way, its place
	// in the answer. It is given when the model names the clip, so a clip
	// held back to be fitted keeps its card when it goes to the framers.
	card  int
	entry PlanEntry
	id    string
}

type planBuilder struct {
	e          *Engine
	ctx        context.Context
	cancel     context.CancelFunc
	sourcePath string
	source     SourceInfo
	lines      []Line
	// units are what the recipe numbered, each a run of lines. An answer
	// is checked in them and turned into lines before anything else.
	units []([2]int)
	opts  PlanOptions
	cropW int
	cache *cropCache
	// prefix and base make the ids: the n-th clip queued is prefix and
	// base+n, see queueLocked.
	prefix string
	base   int
	stamp  PlannedWith
	planID string
	// clock hears every clip taken and landed. Nil when no model was asked.
	clock *searchClock
	// placing counts how far placing the crop has come, for a clip made by
	// hand, which says so on its own. Nil for a search's clips.
	placing *cropWork

	queue chan planJob
	done  sync.WaitGroup
	// firstOut is closed when the first clip has been framed, landed or
	// not. Until then it is the only one being framed.
	firstOut  chan struct{}
	firstOnce sync.Once

	// mu guards what is below it, which the reading of the answer and the
	// framers both touch.
	mu      sync.Mutex
	objects int
	// repeats counts the clips left out as they arrived for keeping a
	// moment an earlier clip keeps, so the whole answer says only the rest.
	repeats int
	// taken are the lines clips were proposed for already, by an earlier
	// search or by hand, see PlanOptions.Taken, and retakes counts the
	// clips left out as they arrived for keeping mostly those.
	taken   [][2]int
	retakes int
	// order is every clip of the answer taken so far, the held ones too,
	// in the order the answer gave them.
	order []PlanEntry
	// fits is whether a clip that does not fit the length may be held back
	// and asked for again once the answer is in. held is those clips.
	fits bool
	held []PlanEntry
	// heldCards are the cards of the held clips, in the same order.
	heldCards []int
	// whole is the answer read to its end: every clip still to come is on
	// the list of clips on the way.
	whole   bool
	seen    map[string]bool
	entries []PlanEntry
	ids     []string
	clips   []PlanClip
	err     error
	closed  bool
	// underway is every clip queued and not yet written or let go, as the
	// log is told it, see Underway.
	underway []Underway

	// writing is held while a clip goes to disk, so two framers finishing
	// at once write one after the other.
	writing sync.Mutex
	written bool
	gone    bool
}

func (e *Engine) newPlanBuilder(ctx context.Context, sourcePath string, source SourceInfo,
	lines []Line, units [][2]int, opts PlanOptions, planID string) *planBuilder {
	ctx, cancel := context.WithCancel(ctx)
	cropW, _ := CropWindow(source, opts.OutW, opts.OutH)
	b := &planBuilder{e: e, ctx: ctx, cancel: cancel, sourcePath: sourcePath, source: source,
		lines: lines, units: units, opts: opts, cropW: cropW, cache: newCropCache(),
		seen: map[string]bool{}, planID: planID, firstOut: make(chan struct{}),
		// Never more clips than were asked for are taken, so the queue
		// never makes the reading of the answer wait.
		queue: make(chan planJob, max(opts.Count, 1))}
	b.stamp = PlannedWith{Count: opts.Count, Min: PyFloat(opts.MinLen),
		Max: PyFloat(opts.MaxLen), Model: opts.Model, By: opts.By}
	b.prefix = opts.IDPrefix
	b.taken = takenLines(lines, opts.Taken)
	// A second pass over a later part of the episode must not reuse 01..04,
	// or its clips would overwrite the first pass's output. Seconds, not
	// minutes, so two windows inside the same minute still differ.
	if opts.Window != nil {
		if b.prefix == "" {
			b.prefix = fmt.Sprintf("t%d-", int(opts.Window.Start))
			if opts.Pass > 1 {
				b.prefix = fmt.Sprintf("t%d-%d-", int(opts.Window.Start), opts.Pass)
			}
		}
		from, to := PyFloat(roundTo(opts.Window.Start, 3)), PyFloat(roundTo(opts.Window.End, 3))
		b.stamp.From, b.stamp.To = &from, &to
	}
	// A set that grows numbers on from the clips it has.
	if opts.Grows && opts.PlanPath != "" {
		b.base = lastNumber(opts.PlanPath, b.prefix)
	}
	for range framers {
		b.done.Add(1)
		go b.frameAll()
	}
	return b
}

// take is one clip of the answer, as text, the moment it is whole. It is
// checked exactly as a clip of a whole answer is, and a clip that fails the
// checks is left for the reading of the whole answer to report.
func (b *planBuilder) take(raw string) {
	decoder := json.NewDecoder(strings.NewReader(raw))
	decoder.UseNumber()
	var value any
	if decoder.Decode(&value) != nil {
		return
	}
	b.mu.Lock()
	if b.closed || len(b.order) >= b.opts.Count {
		b.mu.Unlock()
		return
	}
	b.objects++
	entry, _, ok := readEntry(value, b.objects, b.units)
	b.mu.Unlock()
	if ok {
		b.propose(entry)
	}
}

// propose takes a clip, whoever proposed it: the model, through take, or a
// person with I or O. It is shaped onto sentences, left out when it keeps a
// moment one before it keeps, and otherwise held back to be fitted or
// queued to be framed and written, the same way for every clip.
func (b *planBuilder) propose(entry PlanEntry) {
	b.mu.Lock()
	if b.closed || len(b.order) >= b.opts.Count {
		b.mu.Unlock()
		return
	}
	entry, fitted := b.shapedFitted(entry)
	if fitted != "" {
		b.e.Log.Info("%s %s around its heart to %ss", entry.Slug, fitted, fixed(b.seconds(entry.Keep), 1))
	}
	if b.retaken(entry) {
		b.retakes++
		b.mu.Unlock()
		b.e.Log.Warn("%s", retakeNote(entry))
		return
	}
	if earlier, repeated := sameMomentAs(b.order, entry); repeated {
		b.repeats++
		b.mu.Unlock()
		b.e.Log.Warn("%s", repeatNote(entry, earlier))
		return
	}
	b.acceptLocked(entry)
	b.mu.Unlock()
}

// shaped is a clip as it is cut: every edge on a sentence, see edges.go,
// and runs that follow each other one run when the recipe leaves the
// pauses to the engine.
func (b *planBuilder) shaped(entry PlanEntry) PlanEntry {
	entry, _ = b.shapedFitted(entry)
	return entry
}

// shapedFitted is shaped, and for a recipe that names the heart of every
// clip, the clip fitted to the length around it, see heart.go. It says
// what fitting did.
func (b *planBuilder) shapedFitted(entry PlanEntry) (PlanEntry, string) {
	recipe := b.opts.recipe()
	if recipe.Hearts {
		// The heart is whole sentences and kept, before the edges are put
		// on sentences, so they never cut a line of it. Put back after,
		// a heart ending on a comma ended the clip there.
		entry.Heart = wholeHeart(b.lines, entry.Heart)
		if entry.Opening > 0 {
			entry.Opening = sentenceStart(b.lines, min(entry.Opening, len(b.lines)))
		}
		if entry.Heart[0] > 0 {
			entry.Keep = withRun(entry.Keep, entry.Heart)
		}
	}
	entry.Keep = wholeSentences(b.lines, entry.Keep, b.opts.MaxLen, b.seconds)
	if recipe.Joins {
		entry.Keep = joinRuns(entry.Keep)
	}
	if !recipe.Hearts {
		return b.named(entry), ""
	}
	taken := func(n int) bool {
		for _, t := range b.taken {
			if n >= t[0] && n <= t[1] {
				return true
			}
		}
		return false
	}
	keep, did := fitToHeart(b.lines, entry.Keep, entry.Heart, entry.Opening, b.opts.MinLen, b.opts.MaxLen,
		taken, b.seconds)
	entry.Keep = keep
	return b.named(entry), did
}

// named gives a clip the model named nothing, as middle's are, a title and
// a slug from its first words.
func (b *planBuilder) named(entry PlanEntry) PlanEntry {
	if entry.Title != "" {
		return entry
	}
	var words []string
	for _, r := range entry.Keep {
		for n := r[0]; n <= r[1] && len(words) < 20; n++ {
			words = append(words, strings.Fields(b.lines[n-1].Text())...)
		}
	}
	entry.Title = firstWords(strings.Join(words, " "), 60)
	entry.Slug = strings.ToLower(SanitiseName(entry.Title, "clip"))
	return entry
}

// acceptLocked takes a clip of the answer. One that does not fit the length
// is held back, when it can be asked for again, and every other one goes to
// the framers.
func (b *planBuilder) acceptLocked(entry PlanEntry) {
	b.order = append(b.order, entry)
	card := len(b.order)
	hold := b.fits && (b.opts.recipe().Edit || b.outside(b.seconds(entry.Keep)))
	if b.clock != nil {
		b.clock.named(hold)
	}
	if hold {
		b.held = append(b.held, entry)
		b.heldCards = append(b.heldCards, card)
		// On its way like any other, being fitted to the length first.
		first, last := entry.Keep[0][0], entry.Keep[len(entry.Keep)-1][1]
		b.underway = append(b.underway, Underway{N: card, Start: b.lines[first-1].Start(),
			End: b.lines[last-1].End(), Title: entry.Title, Step: StepFitting})
		b.sayUnderwayLocked()
		return
	}
	b.queueLocked(entry, card)
}

// seconds is how long a clip that keeps these lines runs, the way it is
// framed: filler at its edges dropped and long pauses inside it cut.
func (b *planBuilder) seconds(keep [][2]int) float64 {
	total := 0.0
	for _, s := range SegmentsFromRanges(trimFiller(b.lines, keep), b.lines, b.opts.KeepPause, b.opts.MaxPause) {
		total += s.Duration()
	}
	return total
}

// outside says whether a clip is well off the length asked for. Both
// bounds are targets, not walls: a clip a little over is a clip, and a
// warning about a tenth of a second teaches you to ignore the warning.
func (b *planBuilder) outside(seconds float64) bool {
	return seconds < b.opts.MinLen*0.9 || seconds > b.opts.MaxLen*1.2
}

// trimFiller drops a leading or trailing line that carries nothing. Whole
// lines, never part of one.
func trimFiller(lines []Line, keep [][2]int) [][2]int {
	ranges := append([][2]int(nil), keep...)
	for len(ranges) > 0 && IsFiller(lines[ranges[0][0]-1].Text()) {
		// "Und" before "irgendein Typ auf dem Schulhof" is the first word
		// of the sentence, and without it the clip starts in the middle of
		// one. The recogniser writes a sentence with a capital, so a line
		// that goes on in lower case is the rest of the filler's sentence.
		if ranges[0][0] < ranges[0][1] && startsLower(lines[ranges[0][0]].Text()) {
			break
		}
		if ranges[0][0] < ranges[0][1] {
			ranges[0][0]++
		} else {
			ranges = ranges[1:]
		}
	}
	for len(ranges) > 0 && IsFiller(lines[ranges[len(ranges)-1][1]-1].Text()) {
		last := len(ranges) - 1
		if ranges[last][0] < ranges[last][1] {
			ranges[last][1]--
		} else {
			ranges = ranges[:last]
		}
	}
	return ranges
}

// startsLower is true for text whose first letter is a small one.
func startsLower(text string) bool {
	for _, r := range text {
		if unicode.IsLetter(r) {
			return unicode.IsLower(r)
		}
	}
	return false
}

// sameMomentAs finds a clip among these that keeps the same moment as
// entry: more than half the lines of the shorter of the two are in both.
// The model sometimes gives one moment twice, a line apart, and two clips
// of one moment are one clip and a slot gone.
func sameMomentAs(entries []PlanEntry, entry PlanEntry) (PlanEntry, bool) {
	for _, e := range entries {
		shared := 0
		for _, a := range e.Keep {
			for _, b := range entry.Keep {
				shared += max(0, min(a[1], b[1])-max(a[0], b[0])+1)
			}
		}
		if 2*shared > min(keptLines(e), keptLines(entry)) {
			return e, true
		}
	}
	return PlanEntry{}, false
}

// takenLines are the runs of lines, numbered from 1 the way the model
// reads them, that lie mostly inside the parts given: more than half of a
// line's time.
func takenLines(lines []Line, parts []Window) [][2]int {
	var out [][2]int
	if len(parts) == 0 {
		return nil
	}
	for i, line := range lines {
		inside := 0.0
		for _, w := range parts {
			inside += max(0, min(w.End, line.End())-max(w.Start, line.Start()))
		}
		if 2*inside <= line.Duration() || line.Duration() <= 0 {
			continue
		}
		n := i + 1
		if last := len(out) - 1; last >= 0 && out[last][1] == n-1 {
			out[last][1] = n
			continue
		}
		out = append(out, [2]int{n, n})
	}
	return out
}

// retaken says whether a clip keeps mostly lines a clip was proposed for
// already, which is the same measure as two clips of one answer keeping
// the same moment, see sameMomentAs. The model is told to leave those
// lines, and a clip that keeps them all the same would be a moment the
// list has, or one that was removed from it.
func (b *planBuilder) retaken(entry PlanEntry) bool {
	if len(b.taken) == 0 {
		return false
	}
	shared := 0
	for _, a := range b.taken {
		for _, k := range entry.Keep {
			shared += max(0, min(a[1], k[1])-max(a[0], k[0])+1)
		}
	}
	return 2*shared > keptLines(entry)
}

func retakeNote(entry PlanEntry) string {
	return fmt.Sprintf("the model gave %q, a moment an earlier clip has. It is left out.", entry.Title)
}

func keptLines(entry PlanEntry) int {
	n := 0
	for _, run := range entry.Keep {
		n += run[1] - run[0] + 1
	}
	return n
}

// distinctMoments is the entries without those that keep a moment one
// before them keeps, and the ones left out, each beside the one it repeats.
func distinctMoments(entries []PlanEntry) (kept []PlanEntry, repeats [][2]PlanEntry) {
	for _, entry := range entries {
		if earlier, repeated := sameMomentAs(kept, entry); repeated {
			repeats = append(repeats, [2]PlanEntry{entry, earlier})
			continue
		}
		kept = append(kept, entry)
	}
	return kept, repeats
}

func repeatNote(entry, earlier PlanEntry) string {
	return fmt.Sprintf("the model gave the moment of %q twice. %q is left out.",
		earlier.Title, entry.Title)
}

// queueLocked gives an entry its id and hands it to the framers. The id is
// its place among the usable clips of the answer, as it always was, after
// the clips a set that grows already has.
func (b *planBuilder) queueLocked(entry PlanEntry, card int) {
	position := len(b.entries) + 1
	uniqueSlug(b.seen, &entry, position)
	id := fmt.Sprintf("%s%02d", b.prefix, b.base+position)
	b.entries = append(b.entries, entry)
	b.ids = append(b.ids, id)
	first, last := entry.Keep[0][0], entry.Keep[len(entry.Keep)-1][1]
	on := Underway{N: card, Start: b.lines[first-1].Start(), End: b.lines[last-1].End(),
		Title: entry.Title, Step: StepFraming, Clip: b.clipKey(id)}
	// A clip that was held back already has its card, and keeps it.
	placed := false
	for i := range b.underway {
		if b.underway[i].N == card {
			b.underway[i], placed = on, true
		}
	}
	if !placed {
		b.underway = append(b.underway, on)
	}
	b.sayUnderwayLocked()
	b.queue <- planJob{index: position, card: card, entry: entry, id: id}
	if b.clock != nil {
		b.clock.taken()
	}
}

// clipKey is the key a clip of this set has in the app's clip list.
func (b *planBuilder) clipKey(id string) string {
	if b.opts.PlanPath == "" {
		return ""
	}
	return filepath.Base(b.opts.PlanPath) + "/" + id
}

// arrived takes a clip off the list of clips on the way, let go. A clip
// that is written is taken off by land, in the same event that counts it.
func (b *planBuilder) arrived(job planJob) {
	if b.clock != nil {
		b.clock.framed()
	}
	b.mu.Lock()
	defer b.mu.Unlock()
	b.takeOffLocked(job.card)
}

func (b *planBuilder) takeOffLocked(card int) {
	for i, u := range b.underway {
		if u.N == card {
			b.underway = append(b.underway[:i:i], b.underway[i+1:]...)
			b.sayUnderwayLocked()
			return
		}
	}
}

// sayUnderwayLocked tells the app the clips on the way, how many are
// written, and whether the answer is whole.
func (b *planBuilder) sayUnderwayLocked() {
	b.e.Log.Underway(b.underway, len(b.clips), b.whole)
}

// answerWhole is the answer read to its end, the rest of it taken: every
// clip still to come is on the list of clips on the way, so the app holds
// no row open for a clip the model did not name.
func (b *planBuilder) answerWhole() {
	b.mu.Lock()
	defer b.mu.Unlock()
	b.whole = true
	b.sayUnderwayLocked()
}

// cutDown says what a clip on the way keeps, once its pauses are cut and
// before its crop is placed, which is the slow part.
func (b *planBuilder) cutDown(card int, spans []Span) {
	b.mu.Lock()
	defer b.mu.Unlock()
	for i := range b.underway {
		if b.underway[i].N != card {
			continue
		}
		pieces := make([][2]float64, len(spans))
		for k, s := range spans {
			pieces[k] = [2]float64{roundTo(s.Start, 3), roundTo(s.End, 3)}
		}
		b.underway[i].Pieces = pieces
		b.sayUnderwayLocked()
		return
	}
}

// answered is the model done writing. What is still being framed is the
// last part of the search.
func (b *planBuilder) answered() {
	if b.clock != nil {
		b.clock.answerDone()
	}
}

// rest takes what the whole answer holds beyond the clips already taken.
// The clips taken as it arrived are the start of the same list, read the
// same way, so only the ones after them are new. An answer that reads
// differently whole than it did in pieces keeps the pieces: they are what
// is already in the plan.
func (b *planBuilder) rest(whole []PlanEntry) {
	b.mu.Lock()
	defer b.mu.Unlock()
	// Every clip asked for was taken as the answer arrived. What follows
	// is a model that went on writing, and it is not read for repeats.
	if b.closed || len(b.order) >= b.opts.Count {
		return
	}
	// The clips taken as the answer arrived were checked for repeats in
	// the same order, so the first of the repeats were already said.
	fresh := whole[:0]
	retakes := 0
	for _, entry := range whole {
		entry = b.shaped(entry)
		if b.retaken(entry) {
			if retakes++; retakes > b.retakes {
				b.e.Log.Warn("%s", retakeNote(entry))
			}
			continue
		}
		fresh = append(fresh, entry)
	}
	b.retakes = max(b.retakes, retakes)
	whole, repeats := distinctMoments(fresh)
	for _, r := range repeats[min(b.repeats, len(repeats)):] {
		b.e.Log.Warn("%s", repeatNote(r[0], r[1]))
	}
	b.repeats = max(b.repeats, len(repeats))
	taken := len(b.order)
	if len(whole) < taken {
		return
	}
	for i := range taken {
		if fmt.Sprint(whole[i].Keep) != fmt.Sprint(b.order[i].Keep) {
			b.e.Log.Warn("the whole answer reads differently from the clips taken as it " +
				"arrived. Those are kept.")
			return
		}
	}
	for _, entry := range whole[taken:] {
		if len(b.order) >= b.opts.Count {
			return
		}
		b.acceptLocked(entry)
	}
}

func (b *planBuilder) count() int {
	b.mu.Lock()
	defer b.mu.Unlock()
	return len(b.order)
}

// finish waits for every clip taken to be framed and written. It gives the
// clips in the order of their ids, every usable entry of the answer with its
// id, and the first thing that went wrong.
func (b *planBuilder) finish() ([]PlanClip, []PlanEntry, []string, error) {
	b.mu.Lock()
	if !b.closed {
		b.closed = true
		close(b.queue)
	}
	b.mu.Unlock()
	b.done.Wait()
	b.mu.Lock()
	defer b.mu.Unlock()
	place := map[string]int{}
	for i, id := range b.ids {
		place[id] = i
	}
	clips := append([]PlanClip(nil), b.clips...)
	sort.SliceStable(clips, func(i, j int) bool { return place[clips[i].ID] < place[clips[j].ID] })
	return clips, b.entries, b.ids, b.err
}

// failed is the answer going wrong part of the way. What was taken before
// that is still framed and written, unless the whole search was stopped,
// and the error says so.
func (b *planBuilder) failed(err error) error {
	clips, _, _, _ := b.finish()
	if len(clips) == 0 || b.ctx.Err() != nil || errors.Is(err, context.Canceled) {
		return err
	}
	return renderErr("%s. The %d clip(s) that arrived before that are in the plan.",
		strings.TrimRight(err.Error(), "."), len(clips))
}

// stop ends the framers whatever state the search is in. It is safe after
// finish.
func (b *planBuilder) stop() {
	b.cancel()
	b.finish()
}

func (b *planBuilder) fail(err error) {
	b.mu.Lock()
	if b.err == nil {
		b.err = err
	}
	b.mu.Unlock()
	b.cancel()
}

func (b *planBuilder) frameAll() {
	defer b.done.Done()
	for job := range b.queue {
		if job.index > 1 {
			select {
			case <-b.firstOut:
			case <-b.ctx.Done():
			}
		}
		if b.ctx.Err() != nil {
			// Stopped. The rest of the queue is let go without work.
			b.firstFramed(job)
			b.arrived(job)
			continue
		}
		b.work(job)
	}
}

// framing turns an entry of the answer into a clip. Tests put one here that
// panics.
var framing = (*planBuilder).frame

// work frames one clip and lands it. A framer is a goroutine of its own,
// so a panic in it is out of reach of the recover that guards the job, and
// it ended the whole app: the interface, the other lane and whatever was
// being transcribed. Framing reads a video nobody has checked, so a clip
// that panics is left out with a warning, and the search goes on with the
// rest.
func (b *planBuilder) work(job planJob) {
	defer func() {
		if caught := recover(); caught != nil {
			b.e.Log.Warn("   %02d: could not be framed and is left out: %v", job.index, caught)
			b.e.Log.Detail("%s", debug.Stack())
		}
		b.firstFramed(job)
		b.arrived(job)
	}()
	clip, ok, err := framing(b, job)
	b.firstFramed(job)
	if err != nil {
		b.fail(err)
		return
	}
	if !ok {
		return
	}
	if err := b.land(job.card, clip); err != nil {
		b.fail(err)
	}
}

// firstFramed lets the other framers go once the first clip is done with.
func (b *planBuilder) firstFramed(job planJob) {
	if job.index == 1 {
		b.firstOnce.Do(func() { close(b.firstOut) })
	}
}

// frame turns an entry of the answer into a clip: filler trimmed off its
// ends, its words, its pieces and the framing of every shot in it.
func (b *planBuilder) frame(job planJob) (PlanClip, bool, error) {
	e, lines, opts, index := b.e, b.lines, b.opts, job.index
	// Drop a leading or trailing line that carries nothing. Whole lines,
	// never part of one.
	ranges := trimFiller(lines, job.entry.Keep)
	if len(ranges) == 0 {
		e.Log.Warn("   %02d: every line in it was filler, skipped", index)
		return PlanClip{}, false, nil
	}

	var chosen []Cue
	for _, pair := range ranges {
		for number := pair[0]; number <= pair[1]; number++ {
			chosen = append(chosen, lines[number-1].Cues...)
		}
	}
	sort.SliceStable(chosen, func(a, b int) bool { return chosen[a].Start < chosen[b].Start })

	loose := 0.0
	if len(chosen) > 0 {
		loose = chosen[len(chosen)-1].End - chosen[0].Start
	}
	spans := SegmentsFromRanges(ranges, lines, opts.KeepPause, opts.MaxPause)
	durations := make([]float64, len(spans))
	for k, s := range spans {
		durations[k] = s.Duration()
	}
	if tight := pysum(durations); loose-tight > 0.3 {
		e.Log.Detail("clip %d: %ss dropped between the %d run(s) it kept",
			index, fixed(loose-tight, 1), len(ranges))
	}
	// On the episode's frames already while the clip is on its way, so
	// its edges do not move as it lands, see PieceOnFrames.
	tightSpans := make([]Span, len(spans))
	for k, s := range spans {
		start, end := b.source.PieceOnFrames(s.Start, s.End)
		tightSpans[k] = Span{start, end}
	}
	b.cutDown(job.card, tightSpans)

	segments, err := e.ClipSegments(b.ctx, b.sourcePath, tightSpans, b.source, b.cropW, b.cache,
		b.placing)
	if err != nil {
		return PlanClip{}, false, err
	}
	angles := map[string]bool{}
	for _, s := range segments {
		key := "none"
		if s.CropX != nil {
			key = itoa(*s.CropX)
		}
		angles[key] = true
	}
	if len(angles) > 1 {
		e.Log.Detail("clip %d: %d camera angle(s) across %d segment(s)",
			index, len(angles), len(segments))
	}
	if len(segments) == 0 {
		return PlanClip{}, false, nil
	}
	if len(segments) > MaxSegments {
		e.Log.Warn("%02d discarded: %d segments is past the limit of %d",
			index, len(segments), MaxSegments)
		return PlanClip{}, false, nil
	}

	clip := PlanClip{
		ID:     job.id,
		Slug:   strings.ToLower(SanitiseName(job.entry.Slug, fmt.Sprintf("clip%d", index))),
		Title:  job.entry.Title,
		Reason: job.entry.Reason,
		Keep:   ranges,
	}
	// Each piece on the episode's frames, the way every piece enters a
	// plan, see PieceOnFrames: where a camera switch parts a span too. A
	// search finds its edges on the words' clock, between frames, and
	// this is where they are put on them, before the clip is proposed, so
	// the proposal training data compares an edit with is on frames too,
	// and a clip left alone reads as left alone.
	lengths := make([]float64, len(segments))
	for k, s := range segments {
		start, end := b.source.PieceOnFrames(s.Start, s.End)
		seg := PlanSegment{Start: PyFloat(start), End: PyFloat(end), CropX: "center"}
		if s.CropX != nil {
			seg.CropX = *s.CropX
		}
		clip.Segments = append(clip.Segments, seg)
		lengths[k] = end - start
	}

	total := pysum(lengths)
	flag, advice := "", ""
	switch {
	case !b.outside(total):
	case total < opts.MinLen:
		flag = fmt.Sprintf("  (well under the %ss minimum)", fixed(opts.MinLen, 0))
		advice = "it may be missing context. Widen it in clips.json, or re-run with --replan"
	default:
		flag = fmt.Sprintf("  (well over the %ss target)", fixed(opts.MaxLen, 0))
		advice = "trim it in clips.json and re-render just that clip, or re-run with --replan"
	}
	removed := loose - total
	cutNote := ""
	if removed > 0.3 {
		// Pauses, and with more than one run what lies between them, which
		// is the model leaving something out and no dead air at all.
		cutNote = fmt.Sprintf(", %ss cut", fixed(removed, 1))
	}
	e.Log.OK("%s %-26s %5ss  %d segment(s)%s%s", clip.ID, clip.Slug,
		fixed(total, 1), len(segments), cutNote, flag)
	if flag != "" {
		e.Log.Warn("   %s: %s", clip.Slug, advice)
	}
	return clip, true, nil
}

// land writes a framed clip to the plan. The first one makes the plan, in
// place of whatever plan was there for this window, and each after it is
// added through the same edit the app uses. A set that grows is never made
// anew: every clip is added to it, and the first one there makes it.
func (b *planBuilder) land(card int, clip PlanClip) error {
	b.writing.Lock()
	defer b.writing.Unlock()
	if b.gone {
		return nil
	}
	if path := b.opts.PlanPath; path != "" {
		if b.opts.Grows {
			set := PlanFile{Source: filepath.Base(b.sourcePath), PlanID: b.planID, PlannedWith: b.stamp}
			added, err := addClip(path, set, clip, b.prefix)
			if err != nil {
				return err
			}
			b.mu.Lock()
			for i, id := range b.ids {
				if id == clip.ID {
					b.ids[i] = added.ID
				}
			}
			// Another clip took its id since it was queued, so its key
			// in the list is another, and it is said before it lands.
			if added.ID != clip.ID {
				for i := range b.underway {
					if b.underway[i].N == card {
						b.underway[i].Clip = b.clipKey(added.ID)
						b.sayUnderwayLocked()
					}
				}
			}
			b.mu.Unlock()
			clip = added
			b.written = true
		} else if !b.written {
			body, err := MarshalPlan(PlanFile{Source: filepath.Base(b.sourcePath), PlanID: b.planID,
				PlannedWith: b.stamp, Clips: []PlanClip{clip}})
			if err != nil {
				return err
			}
			if err := writePlanFile(path, body); err != nil {
				return err
			}
			b.written = true
		} else if err := appendClip(path, clip); err != nil {
			switch {
			case os.IsNotExist(err):
				// The plan was removed while the search ran. Nothing more
				// goes in it, and the search is not the one to bring it
				// back.
				b.gone = true
				b.e.Log.Warn("the plan was removed while clips were still arriving. " +
					"The rest are left out.")
				return nil
			case errors.Is(err, errNotAdded):
				b.e.Log.Detail("%s left out: it is in the plan already", clip.ID)
				return nil
			}
			return err
		}
	}
	// Written: counted and off the list of clips on the way in one event,
	// before the clock says it found one more.
	b.mu.Lock()
	b.clips = append(b.clips, clip)
	b.takeOffLocked(card)
	b.mu.Unlock()
	if b.clock != nil {
		b.clock.landed()
	}
	return nil
}

// none writes the plan with no clips in it, for a search that found
// nothing beyond the clips there are, see BuildPlan.
func (b *planBuilder) none() error {
	b.writing.Lock()
	defer b.writing.Unlock()
	path := b.opts.PlanPath
	if path == "" || b.written || b.gone || b.opts.Grows {
		return nil
	}
	body, err := MarshalPlan(PlanFile{Source: filepath.Base(b.sourcePath), PlanID: b.planID,
		PlannedWith: b.stamp, Clips: []PlanClip{}})
	if err != nil {
		return err
	}
	if err := writePlanFile(path, body); err != nil {
		return err
	}
	b.written = true
	return nil
}

// settle writes the plan's id when it turned out to be another than the one
// the plan was begun with, which is a reused answer found only once it was
// whole.
func (b *planBuilder) settle(planID string) error {
	b.writing.Lock()
	defer b.writing.Unlock()
	if planID == "" || planID == b.planID || !b.written || b.gone || b.opts.PlanPath == "" {
		return nil
	}
	err := editPlan(b.opts.PlanPath, func(top *object, _ []*object) error {
		top.set(keyPlanID, planID)
		return nil
	})
	if os.IsNotExist(err) {
		return nil
	}
	return err
}

// lastNumber is the highest number among the ids in a clip set that begin
// with prefix, or 0 for a set that is not there yet. A set that grows gives
// its next clip the number after it.
func lastNumber(path, prefix string) int {
	_, clips, err := LoadClips(path)
	if err != nil {
		return 0
	}
	last := 0
	for _, c := range clips {
		rest, ok := strings.CutPrefix(c.ID, prefix)
		if !ok {
			continue
		}
		if n, err := strconv.Atoi(rest); err == nil && n > last {
			last = n
		}
	}
	return last
}

// firstWords is as many whole words of text as fit in limit characters, or
// the first word when it alone is longer.
func firstWords(text string, limit int) string {
	words := strings.Fields(text)
	out := ""
	for i, w := range words {
		next := w
		if i > 0 {
			next = out + " " + w
		}
		if i > 0 && runeLen(next) > limit {
			break
		}
		out = next
	}
	return out
}
