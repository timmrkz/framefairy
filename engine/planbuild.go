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
	"strings"
	"sync"
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
	units  []([2]int)
	opts   PlanOptions
	cropW  int
	cache  *cropCache
	offset int
	stamp  PlannedWith
	planID string
	// clock hears every clip taken and landed. Nil when no model was asked.
	clock *searchClock

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
	seen    map[string]bool
	entries []PlanEntry
	ids     []string
	clips   []PlanClip
	err     error
	closed  bool

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
		Max: PyFloat(opts.MaxLen), Model: opts.Model}
	// A second pass over a later part of the episode must not reuse 01..04,
	// or its clips would overwrite the first pass's output. Seconds, not
	// minutes, so two windows inside the same minute still differ.
	if opts.Window != nil {
		b.offset = int(opts.Window.Start)
		from, to := PyFloat(roundTo(opts.Window.Start, 3)), PyFloat(roundTo(opts.Window.End, 3))
		b.stamp.From, b.stamp.To = &from, &to
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
	if b.closed || len(b.entries) >= b.opts.Count {
		b.mu.Unlock()
		return
	}
	b.objects++
	entry, _, ok := readEntry(value, b.objects, b.units)
	if !ok {
		b.mu.Unlock()
		return
	}
	b.queueLocked(entry)
	b.mu.Unlock()
}

// queueLocked gives an entry its id and hands it to the framers. The id is
// its place among the usable clips of the answer, as it always was.
func (b *planBuilder) queueLocked(entry PlanEntry) {
	position := len(b.entries) + 1
	uniqueSlug(b.seen, &entry, position)
	id := fmt.Sprintf("%02d", position)
	if b.opts.Window != nil {
		id = fmt.Sprintf("t%d-%02d", b.offset, position)
	}
	b.entries = append(b.entries, entry)
	b.ids = append(b.ids, id)
	b.queue <- planJob{index: position, entry: entry, id: id}
	if b.clock != nil {
		b.clock.taken()
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
	if b.closed {
		return
	}
	taken := len(b.entries)
	if len(whole) < taken {
		return
	}
	for i := range taken {
		if fmt.Sprint(whole[i].Keep) != fmt.Sprint(b.entries[i].Keep) {
			b.e.Log.Warn("the whole answer reads differently from the clips taken as it " +
				"arrived. Those are kept.")
			return
		}
	}
	for _, entry := range whole[taken:] {
		if len(b.entries) >= b.opts.Count {
			return
		}
		b.queueLocked(entry)
	}
}

func (b *planBuilder) count() int {
	b.mu.Lock()
	defer b.mu.Unlock()
	return len(b.entries)
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
	if err := b.land(clip); err != nil {
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
	ranges := append([][2]int(nil), job.entry.Keep...)
	for len(ranges) > 0 && IsFiller(lines[ranges[0][0]-1].Text()) {
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
	tightSpans := make([]Span, len(spans))
	for k, s := range spans {
		tightSpans[k] = Span{s.Start, s.End}
	}

	segments, err := e.ClipSegments(b.ctx, b.sourcePath, tightSpans, b.source, b.cropW, b.cache)
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
		Words:  [][3]any{},
	}
	for _, w := range chosen {
		clip.Words = append(clip.Words, [3]any{PyFloat(roundTo(w.Start, 3)),
			PyFloat(roundTo(w.End, 3)), w.Text})
	}
	lengths := make([]float64, len(segments))
	for k, s := range segments {
		seg := PlanSegment{Start: PyFloat(roundTo(s.Start, 3)), End: PyFloat(roundTo(s.End, 3)),
			CropX: "center"}
		if s.CropX != nil {
			seg.CropX = *s.CropX
		}
		clip.Segments = append(clip.Segments, seg)
		lengths[k] = s.Duration()
	}

	total := pysum(lengths)
	// Both bounds are targets, not walls. Warning about a tenth of a
	// second teaches you to ignore the warning.
	flag, advice := "", ""
	if total < opts.MinLen*0.9 {
		flag = fmt.Sprintf("  (well under the %ss minimum)", fixed(opts.MinLen, 0))
		advice = "it may be missing context. Widen it in clips.json, or re-run with --replan"
	} else if total > opts.MaxLen*1.2 {
		flag = fmt.Sprintf("  (well over the %ss target)", fixed(opts.MaxLen, 0))
		advice = "trim it in clips.json and re-render just that clip, or re-run with --replan"
	}
	removed := loose - total
	cutNote := ""
	if removed > 0.3 {
		cutNote = fmt.Sprintf(", %ss of dead air cut", fixed(removed, 1))
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
// added through the same edit the app uses.
func (b *planBuilder) land(clip PlanClip) error {
	b.writing.Lock()
	defer b.writing.Unlock()
	if b.gone {
		return nil
	}
	if path := b.opts.PlanPath; path != "" {
		if !b.written {
			if b.opts.CaptionDir != "" {
				setStaleCaptionsAside(b.e.Log, b.opts.CaptionDir)
			}
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
				b.e.Log.Detail("%s left out: its part was removed while it was on its way", clip.ID)
				return nil
			}
			return err
		}
	}
	b.mu.Lock()
	b.clips = append(b.clips, clip)
	b.mu.Unlock()
	if b.clock != nil {
		b.clock.landed()
	}
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
		top.set("plan_id", planID)
		return nil
	})
	if os.IsNotExist(err) {
		return nil
	}
	return err
}
