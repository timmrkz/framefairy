package main

import (
	"context"
	"math"
	"slices"
	"strings"
	"testing"

	"framefairy/engine"
	"framefairy/engine/timed"
)

// madeByHand are the clips of an episode made with I and O.
func (d *desk) madeByHand(path string) []ClipEntry {
	var out []ClipEntry
	for _, c := range d.clips(path) {
		if strings.HasPrefix(c.Key, engine.HandPlanName+"/") {
			out = append(out, c)
		}
	}
	return out
}

// waitFor waits until a job is over, told by the queue the moment it is,
// and hands back how it ended.
func (d *desk) waitFor(id string) Job {
	d.t.Helper()
	var ended Job
	d.svc.jobs.until(func(jobs []*Job) bool {
		i := slices.IndexFunc(jobs, func(j *Job) bool { return j.ID == id && !j.going() })
		if i >= 0 {
			ended = *jobs[i]
		}
		return i >= 0
	}, nil)
	return ended
}

// I and O pressed while a search is still finding make their clips there
// and then: each has its card from the moment it is pressed, at the
// playhead, none waits for the search, and each lands in the clips made by
// hand with an id of its own.
func TestPathClipsMadeByHandWhileASearchFinds(t *testing.T) {
	d := open(t)
	d.model.hangs(true)
	asked := d.model.asking()
	ep := d.add("hand", minutes5)
	<-asked

	var jobs []Job
	for _, press := range []struct {
		at  float64
		out bool
	}{{20, false}, {90, false}, {200, true}} {
		var j Job
		if press.out {
			j = d.out(ep, press.at)
		} else {
			j = d.in(ep, press.at)
		}
		if j.State != JobRunning || len(j.Underway) != 1 || j.Underway[0].Start != press.at {
			t.Fatalf("pressed at %v, the job is %+v", press.at, j)
		}
		jobs = append(jobs, j)
	}
	var keys []string
	for _, j := range jobs {
		done := d.waitFor(j.ID)
		if done.State != JobDone || len(done.Underway) != 0 {
			t.Fatalf("a clip made by hand ended %+v", done)
		}
		keys = append(keys, done.Result)
	}
	if s, ok := d.svc.jobs.find(ep, engine.JobSearch); !ok || s.State != JobRunning {
		t.Errorf("the search is %+v, still finding", s)
	}
	made := d.madeByHand(ep)
	if len(made) != 3 {
		t.Fatalf("%d clips made by hand", len(made))
	}
	listed := map[string]bool{}
	for _, c := range made {
		listed[c.Key] = true
	}
	for _, key := range keys {
		if !listed[key] {
			t.Errorf("%s is not in the list: %v", key, listed)
		}
	}
	d.cancel(ep)
	d.idle(ep)
	d.model.hangs(false)
}

// The app closed on a clip made by hand while its part of the episode is
// being heard says so where the clip would have been, after it opens
// again, and Continue makes the clip, hearing nothing twice.
//
// On the real disk: ffmpeg and the episode's decoder read the episode.
func TestPathClosedWhileAClipIsHeard(t *testing.T) {
	d := open(t)
	ep := d.episode("late", minutes5)
	if _, err := d.svc.store.AddEpisodes([]string{ep}); err != nil {
		t.Fatal(err)
	}
	in, let := d.speech.hold()
	j := d.in(ep, 250)
	<-in
	d.closeWhileIn(j, let)
	given := d.speech.seconds()

	d.reopen()
	var stopped *Job
	for _, k := range d.svc.jobs.list() {
		if k.Episode == ep && k.Kind == engine.JobClip {
			stopped = &k
		}
	}
	if stopped == nil || stopped.State != JobInterrupted || stopped.At != 250 ||
		len(stopped.Underway) != 1 || stopped.Underway[0].Start != 250 {
		t.Fatalf("after the app closed on %s: %+v", j.ID, stopped)
	}
	again := d.svc.Continue(stopped.ID, "")
	done := d.waitFor(again.ID)
	if done.State != JobDone {
		t.Fatalf("Continue ended %+v", done)
	}
	if made := d.madeByHand(ep); len(made) != 1 || made[0].Start > 250 {
		t.Fatalf("made %+v", made)
	}
	if heard := d.speech.seconds(); heard > 300+30 {
		t.Errorf("%.0f s heard for a 300 s episode, %.0f s of them before the app closed", heard, given)
	}
	if left := engine.ReadJobs(disk, ep); len(left) != 0 {
		t.Errorf("records left: %+v", left)
	}
}

// A clip made with I or O is the clip chosen from the press, before the
// workspace picks its card, and once it is written it is the clip it was
// written as, said with the card it was. So no state of the video between
// the press and the pick has the card gone and nothing chosen, which a
// busy machine showed when the clip was written before the pick came,
// issue #697.
//
// On the real disk: ffmpeg and the episode's decoder read the episode.
func TestPathAClipMadeByHandIsChosenFromThePress(t *testing.T) {
	d := open(t)
	ep := d.episode("chosen", minutes5)
	if _, err := d.svc.store.AddEpisodes([]string{ep}); err != nil {
		t.Fatal(err)
	}
	ctx := context.Background()
	in, let := d.speech.hold()
	j := d.out(ep, 40)
	<-in
	card := j.ID + "/1"
	st, err := d.svc.State(ctx, ep)
	if err != nil {
		t.Fatal(err)
	}
	if st.Chosen != card {
		t.Errorf("while the clip made with O was on its way, %q was chosen, not its card %q", st.Chosen, card)
	}
	let()
	done := d.waitFor(j.ID)
	if done.State != JobDone {
		t.Fatalf("made %+v", done)
	}
	st, err = d.svc.State(ctx, ep)
	if err != nil {
		t.Fatal(err)
	}
	if st.Chosen != done.Result || st.ChosenCard != card {
		t.Errorf("once the clip %q was written, %q was chosen, from the card %q", done.Result, st.Chosen, st.ChosenCard)
	}
}

// An edit to a clip made by hand is undone and redone like an edit to any
// clip, and a clip made by hand after it, which goes into the same set,
// is not an edit and does not stand in the way.
func TestPathAClipMadeByHandIsEditedAndUndone(t *testing.T) {
	d := open(t)
	ep := d.episode("undo", minutes5)
	if _, err := d.svc.store.AddEpisodes([]string{ep}); err != nil {
		t.Fatal(err)
	}
	if done := d.waitFor(d.in(ep, 30).ID); done.State != JobDone {
		t.Fatalf("made %+v", done)
	}
	made := d.madeByHand(ep)
	if len(made) != 1 {
		t.Fatalf("%d clips", len(made))
	}
	c := made[0]
	ctx := context.Background()
	if _, err := d.svc.Reshape(ctx, ep, c.Plan, c.ID, engine.Gesture{Kind: "trim", Edge: "start", From: c.Start + 2},
		[2]float64{c.Start + 7, c.Start + 2}); err != nil {
		t.Fatal(err)
	}
	if done := d.waitFor(d.in(ep, 150).ID); done.State != JobDone {
		t.Fatalf("made %+v", done)
	}
	start := func() float64 {
		for _, m := range d.madeByHand(ep) {
			if m.Key == c.Key {
				return m.Start
			}
		}
		t.Fatalf("%s is gone", c.Key)
		return 0
	}
	if math.Abs(start()-(c.Start+2)) > 0.5 {
		t.Fatalf("trimmed to %.2f, from %.2f", start(), c.Start)
	}
	// The edge took the playhead along, so taking the edge back takes the
	// playhead back to where the hand found it.
	done, err := d.svc.Undo(ep)
	if err != nil || !done.Done {
		t.Fatalf("undo %+v %v", done, err)
	}
	if done.Playhead == nil || *done.Playhead != c.Start+7 {
		t.Errorf("undo put the playhead at %v, want %.2f", done.Playhead, c.Start+7)
	}
	if math.Abs(start()-c.Start) > 0.05 {
		t.Errorf("undone to %.2f, want %.2f", start(), c.Start)
	}
	done, err = d.svc.Redo(ep)
	if err != nil || !done.Done {
		t.Fatalf("redo %+v %v", done, err)
	}
	if done.Playhead == nil || *done.Playhead != c.Start+2 {
		t.Errorf("redo put the playhead at %v, want %.2f", done.Playhead, c.Start+2)
	}
	if math.Abs(start()-(c.Start+2)) > 0.5 {
		t.Errorf("redone to %.2f", start())
	}
	if n := len(d.madeByHand(ep)); n != 2 {
		t.Errorf("%d clips made by hand after undo and redo, want both", n)
	}
}

// A clip made by hand keeps a minute heard on each side, so an edge
// dragged out finds its words there without waiting, and an edge dragged
// past that has what it brought in heard first, Tim on #745.
func TestPathAClipDraggedLongerIsHeardWhereItNowReaches(t *testing.T) {
	d := open(t)
	ep := d.episode("longer", minutes5)
	if _, err := d.svc.store.AddEpisodes([]string{ep}); err != nil {
		t.Fatal(err)
	}
	ctx := context.Background()
	// heard reads the state the way the window does once work ends, and
	// waits for each part it starts, and says what they heard and whether
	// each was room beside the clip.
	heard := func() (parts []Job) {
		for range 10 {
			if _, err := d.svc.State(ctx, ep); err != nil {
				t.Fatal(err)
			}
			hearing, ok := d.svc.jobs.find(ep, engine.JobTranscribe)
			if !ok || !hearing.going() {
				return parts
			}
			if done := d.waitFor(hearing.ID); done.State != JobDone {
				t.Fatalf("the part ended %+v", done)
			}
			parts = append(parts, hearing)
		}
		t.Fatalf("heard on and on: %+v", parts)
		return nil
	}
	clip := func() ClipEntry {
		made := d.madeByHand(ep)
		if len(made) != 1 {
			t.Fatalf("made %+v", made)
		}
		return made[0]
	}
	// O at 200 s hears back to 154 s, a clip's longest and two
	// sentences' reach, see engine.ClipRequest, and a little past the
	// clip. Then the rest of a minute after it, the side with less room,
	// and the rest of a minute before it.
	if done := d.waitFor(d.out(ep, 200).ID); done.State != JobDone {
		t.Fatalf("made %+v", done)
	}
	c := clip()
	room := heard()
	if len(room) != 2 || !room[0].Room || !room[1].Room || room[0].From < c.End ||
		math.Abs(room[0].To-(c.End+60)) > 0.5 || math.Abs(room[1].From-(c.Start-60)) > 0.5 || room[1].To > c.Start {
		t.Fatalf("around the clip %.1f to %.1f the room heard is %+v", c.Start, c.End, room)
	}
	// Its start dragged back 50 s, into the room: the words are there,
	// and the side under 30 s is topped up to a minute again.
	drag := func(from float64) ClipEntry {
		c := clip()
		if _, err := d.svc.Reshape(ctx, ep, c.Plan, c.ID, engine.Gesture{Kind: "trim", Edge: "start", From: from},
			[2]float64{c.Start, from}); err != nil {
			t.Fatal(err)
		}
		return clip()
	}
	if c = drag(c.Start - 50); len(c.Unheard) != 0 {
		t.Fatalf("dragged into the room, the clip has %v not heard", c.Unheard)
	}
	if view, err := d.svc.Captions(ep, c.Plan, c.ID); err != nil || len(view.Captions) == 0 || view.Captions[0].Start > 3 {
		t.Fatalf("dragged into the room, the first caption is at %+v, %v", view, err)
	}
	if more := heard(); len(more) != 1 || !more[0].Room || math.Abs(more[0].To-(c.Start-10)) > 1 {
		t.Fatalf("dragged to %.1f s, the room is topped up by %+v", c.Start, more)
	}
	// Past the room: what it brought in is heard first, in the turn of a
	// clip made by hand, and then the room around it.
	before := d.speech.seconds()
	past := c.Start - 100
	c = drag(past)
	if len(c.Unheard) != 1 || math.Abs(c.Unheard[0][0]-past) > 0.05 {
		t.Fatalf("dragged to %.1f s, past the room, not heard is %v", past, c.Unheard)
	}
	more := heard()
	if len(more) == 0 || more[0].Room || math.Abs(more[0].From-past) > 0.05 {
		t.Fatalf("dragged past the room, the clip's part is heard by %+v", more)
	}
	brought := c.Unheard[0][1] - past
	// The room before it ends at the video's start, 30 s, and a piece the
	// speech model is given may run on past a gap by up to 15 s.
	if got := d.speech.seconds() - before; got > brought+30+15 {
		t.Errorf("%.1f s heard for the %.1f s brought in and the room before it", got, brought)
	}
	if c = clip(); len(c.Unheard) != 0 {
		t.Errorf("once heard, the clip still has %v not heard", c.Unheard)
	}
	if view, err := d.svc.Captions(ep, c.Plan, c.ID); err != nil || len(view.Captions) == 0 || view.Captions[0].Start > 3 {
		t.Fatalf("dragged past the room, the first caption is at %+v, %v", view, err)
	}
}

// What of the clip chosen is heard by itself, and when it waits, issue
// #745. What of the clip is not heard goes first, then room on each side
// of a clip made by hand. A part that failed is not asked for again, or
// every state read would ask for it and fail again, up to several times
// a second. One the app was closed on carries on by itself. The whole
// video being transcribed holds no part up, which takes the lane from it.
func TestThePartOfTheClipChosenIsHeardUnlessItFailed(t *testing.T) {
	t.Parallel()
	searched := "clips-0-300.json/c1"
	byHand := engine.HandPlanName + "/h01"
	entry := func(key string, start, end float64) ClipEntry {
		return ClipEntry{Key: key, ClipView: engine.ClipView{Start: start, End: end,
			Segments: []engine.SegmentView{{Start: start, End: end}}}}
	}
	transcription := func(id, state string, from, to float64) Job {
		j := Job{Kind: engine.JobTranscribe, State: state, Seq: 1}
		j.JobRecord.ID, j.From, j.To = id, from, to
		return j
	}
	inClip, roomBefore, roomAfter := timed.Window{Start: 120, End: 150}, timed.Window{Start: 117, End: 154},
		timed.Window{Start: 200, End: 260}
	for _, c := range []struct {
		name   string
		heard  [][2]float64
		chosen ClipEntry
		jobs   []Job
		want   timed.Window
		room   bool
	}{
		// A clip dragged back to 120 s past what was heard from 150 s.
		{"nothing runs", [][2]float64{{0, 100}, {150, 300}}, entry(searched, 120, 180), nil, inClip, false},
		{"another clip is chosen", [][2]float64{{0, 100}, {150, 300}}, entry("x", 0, 10), nil, timed.Window{}, false},
		{"the whole video is transcribed", [][2]float64{{0, 100}, {150, 300}}, entry(searched, 120, 180),
			[]Job{transcription(engine.TranscribeID, JobRunning, 0, 0)}, inClip, false},
		{"a part is heard already", [][2]float64{{0, 100}, {150, 300}}, entry(searched, 120, 180),
			[]Job{transcription(engine.TranscribePartID, JobRunning, 100, 110)}, timed.Window{}, false},
		{"the same part failed", [][2]float64{{0, 100}, {150, 300}}, entry(searched, 120, 180),
			[]Job{transcription(engine.TranscribePartID, JobFailed, 120, 150)}, timed.Window{}, false},
		{"the same part was stopped by hand", [][2]float64{{0, 100}, {150, 300}}, entry(searched, 120, 180),
			[]Job{transcription(engine.TranscribePartID, JobStopped, 120, 150)}, timed.Window{}, false},
		{"a smaller part failed", [][2]float64{{0, 100}, {150, 300}}, entry(searched, 120, 180),
			[]Job{transcription(engine.TranscribePartID, JobFailed, 130, 150)}, inClip, false},
		{"the app was closed on the same part", [][2]float64{{0, 100}, {150, 300}}, entry(searched, 120, 180),
			[]Job{transcription(engine.TranscribePartID, JobInterrupted, 120, 150)}, inClip, false},
		{"the whole video failed", [][2]float64{{0, 100}, {150, 300}}, entry(searched, 120, 180),
			[]Job{transcription(engine.TranscribeID, JobFailed, 0, 0)}, inClip, false},
		// A clip made with O at 200 s, which heard back to 154 s.
		{"a clip found by a search keeps no room", [][2]float64{{154, 200}}, entry(searched, 177, 200), nil,
			timed.Window{}, false},
		{"a clip made by hand, room after it first", [][2]float64{{154, 200}}, entry(byHand, 177, 200), nil,
			roomAfter, true},
		{"then room before it", [][2]float64{{154, 260}}, entry(byHand, 177, 200), nil, roomBefore, true},
		{"room on both sides", [][2]float64{{117, 260}}, entry(byHand, 177, 200), nil, timed.Window{}, false},
		{"its part before its room", [][2]float64{{154, 200}}, entry(byHand, 120, 200), nil,
			timed.Window{Start: 120, End: 154}, false},
		{"room waits for a part being heard", [][2]float64{{154, 200}}, entry(byHand, 177, 200),
			[]Job{transcription(engine.TranscribePartID, JobRunning, 140, 154)}, timed.Window{}, false},
	} {
		st := VideoState{Status: engine.EpisodeStatus{Heard: c.heard}, Clips: []ClipEntry{c.chosen},
			Chosen: c.chosen.Key, Jobs: c.jobs}
		if c.name == "another clip is chosen" {
			st.Chosen = "y"
		}
		got, room, hear := partToHear(st, 300)
		if hear != (c.want != timed.Window{}) || got != c.want || room != c.room {
			t.Errorf("%s: hears %v %v, room %v, want %v, room %v", c.name, hear, got, room, c.want, c.room)
		}
	}
}

// The whole video and the part of a clip are two transcriptions, which
// run side by side, and asking for either again while it runs gives the
// one that runs, issue #745.
func TestAClipsPartIsHeardBesideTheWholeVideo(t *testing.T) {
	t.Parallel()
	svc, eps := memoryLibrary(t, "ep.mp4")
	svc.jobs = newIdleQueue(svc.store, func(JobUpdate) {}, func(string) {})
	whole := svc.Transcribe(eps[0])
	part := svc.transcribe(eps[0], timed.Window{Start: 120, End: 150}, false)
	if whole.ID == part.ID || whole.JobRecord.ID != engine.TranscribeID || part.JobRecord.ID != engine.TranscribePartID {
		t.Fatalf("the whole video is %+v, the part %+v", whole, part)
	}
	if again := svc.transcribe(eps[0], timed.Window{Start: 120, End: 150}, false); again.ID != part.ID {
		t.Errorf("the part asked for again is %s, not the one that runs, %s", again.ID, part.ID)
	}
	if again := svc.Transcribe(eps[0]); again.ID != whole.ID {
		t.Errorf("the whole video asked for again is %s, not the one that runs, %s", again.ID, whole.ID)
	}
}
