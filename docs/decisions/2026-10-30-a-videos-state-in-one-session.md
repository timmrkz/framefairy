# A video's state lives in one session in the interface

Issue #606, from the architecture review of 29 October 2026 (#603).
Decided on 30 October 2026, in the pull request that makes its first
slice. The slices after it name this record.

## The problem

Tim asked on 29 October for the issues of the architecture review that
matter most for reliability and long-term maintenance, a large one in a
pull request of its own. #606 is one of them: "Go owns a video's state
and sends it as one snapshot", so the workspace stops guessing from pings.

Measured on main at 127fbef:

- `Episode.svelte` is 4,585 lines with 35 effects. Seven of them notice
  that work moved on by keeping a value from before and comparing it
  with the next: `foundHeard`, `wasWhole`, `wasTranscribing`,
  `searchRan`, the renders in `waiting`, the clips made by hand in
  `madeHere`, and the failures, through `Failures` in `lib/failed.ts`.
  Two more notice that a search started, `followed` and `searchSeen`.
  `App.svelte` has one more, `transcribingSeen`, for Transcribe whole
  video failing.
- The clip list is read again from 11 places, `refreshClips`, and the
  whole video from 7, `load`. Three tickets, `Newest`, keep the answers
  to those reads in order, and `OnTheWay` and `unread` keep the clips
  between the job saying it wrote one and a read that has it.
- Two polls: the job list every 5 s while anything runs, in
  `lib/state.svelte.ts`, and the workspace's own timer every 2 s, which
  reads the video while it is heard and the clips while they are found.
- Go's `episode` event carries only a path. A job's news is a whole `Job`,
  sent from 12 places in `cmd/framefairy-app/jobs.go`, which copies ten
  fields of `engine.JobRecord` and writes them again as the job goes.

An effect that compares the value from before runs on every job event,
about once a second, and sees only the newest state after the batch: a
search that ends and a new one that starts before the effect runs are
one search to it. Each new kind of work has added an effect, a ticket or
a counter of its own, which is how the family of #323, #331, #431, #436,
#444, #450, #513 and #572 came about.

## Five simulations

### 1. Who may know about whom

Go knows the state of a video, the interface shows it. Go assembles one
value for a video, its status, its clips with the ones on their way, how
far it is heard and its jobs, with a revision that grows on every change,
and sends it as the `episode` event, at most four times a second with
the last change always sent. The interface keeps it in one session per
open video and the workspace is a view of it. Crossing the boundary: one
event and one type, in place of the path the event carries now. The
bindings `Episode`, `Clips` and `Coverage` stay for the first read. The
next fact about a video is a field of that type. Cost: about four pull
requests, Go and the interface each in its own.

### 2. What changes together

A job's life is told where its news lands. The job store already applies
every snapshot of a job, in order, and drops an older one. It hands
each snapshot it keeps on, with the one it replaced, and a session per
open video says what moved: a clip written, the hearing over, the search
over, the whole video heard, a clip made by hand or a render over, a
failure. No change on the Go side. One pull request moves the eight
effects. The polls and the tickets stay,
because the workspace still has to guess what to read again.

### 3. Where state lives

One home for each piece of a video's state in the interface. Today
`status`, `clips` and `coverage` are kept in `Episode.svelte`, each with
its own ticket, and the jobs in the store. All of it moves into the
session, and the components read from it. A session kept in a map in
`lib/` would let a workspace that sleeps, and the sidebar, read the same
one. It moves 3 states and 3 tickets out of `Episode.svelte`, and every
read of them, 136 lines that name `status`, `clips` or `coverage`.

### 4. What runs together

Go sends a revision and what changed, `{episode, rev, changed}`, at most
four times a second, and the interface reads only what changed, as
`docs/DESIGN-REVIEW.md` proposed on 6 October. The bindings stay what
they are. But every change costs a round trip, and two reads in the air
answer in either order, so a ticket stays, kept by revision. The
interface still decides what to read, which is what the issue asks to
end.

### 5. What a test has to build

The session is a plain class that takes the job store, later the
events, from outside, and is tested in Vitest with neither Svelte
components nor Go, the way `lib/state.svelte.ts` is. Go's snapshot is
made by one function from the queue and the work folder, tested with a
memory disk. The walks prove the two together. So the transitions are
worked out in the session from two snapshots, and nothing about them is
left in a component.

## What converges

- One session per open video in `frontend/src/lib/`, the one place a
  video's state lives in the interface: 1, 3 and 5, and 2 as its seam.
- What moved is said once, by the session, as it applies the news,
  never by an effect: 2, 5, and 1, where the session compares two
  snapshots.
- Go sends the whole snapshot, with a revision, coalesced: 1 and 3. Only
  4 pulls. A pull keeps a ticket and a round trip for every change, and
  leaves the interface deciding what to read, so it is dropped.
- A `Job` holds the engine's record instead of copying it, each state
  saying one thing: 1 and 3, and asked for by the issue.
- A map of sessions shared with the sidebar is only in 3. The sidebar
  reads the library, not a video's state, so it is left until something
  needs it.

## The decision

The interface, each layer using only the ones under it:

```
screens/Episode.svelte     a view: reads the session, asks Go through api.ts
lib/session.svelte.ts      one video's state, kept by revision, and what its work did
lib/state.svelte.ts        every video's jobs, each snapshot kept handed on
lib/api.ts                 the calls and the events of Go
```

Under them the app's Go side, which sends a video's state as one
snapshot, and the engine, which keeps each job's record.

In four slices, each a pull request:

1. `EpisodeSession` in `frontend/src/lib/session.svelte.ts`, which hears
   every snapshot the job store keeps through `JobStore.watch` and says
   the moves of the video's work. The seven effects of `Episode.svelte`
   that noticed work moving on become its seven moves, unchanged in what
   they do, and `lib/failed.ts` goes. Work asked for here, a clip made by
   hand or a render, is told to the session with `asked`, which says it
   at once when it was over before its answer came. `transcribingSeen`
   in `App.svelte` is about every video, not one, so it hears
   `JobStore.watch` itself. This pull request.
2. A `Job` holds its `engine.JobRecord`, and each state says one thing.
   Go only.
3. Go sends a video's state as one snapshot with a revision, coalesced,
   and the session keeps it when its revision is newer. `status`,
   `clips`, `coverage` and the video's jobs move into the session.
4. The tickets, `OnTheWay`, `unread`, the effects that notice a start
   and both polls go. One `startJob` keeps the pending state of every
   control from the video's jobs, as Tim's comment on the issue asks.
   This fixes #513.

## What follows

- It costs four pull requests, the third the largest.
- It buys a workspace that shows what Go says instead of working it out,
  so a new kind of work is a field and a move, not an effect, a ticket
  and a counter.
- Slice 1 changes one thing the session itself does: it hears every
  snapshot, so a search that ends after the next has begun is said to be
  over, where the effect followed only the newest search. The workspace
  cannot get there today, because Search waits while a search runs and
  Go starts only a video's first search by itself. Should that change,
  what the workspace does then is decided with it.
- It was wrong if the snapshot of a video with many clips is too large
  to send four times a second, which slice 3 measures before it sends
  it, or if a component needs a value from before that the session does
  not keep.

## Slice 3, as built

Tim asked on 1 November for all of step 3 in one pull request.

- Go reads a video's state with `State` in
  `cmd/framefairy-app/videostate.go`: its status, its clips, how often
  each part was searched and its jobs, as one `VideoState`. It is sent
  as the `video` event by `videoNews`, at once when nothing was sent in
  the last quarter of a second, else at the end of it, and always once
  more after the last change. The state is read when it is sent, so a
  change while a send is due is in it.
- Its `rev` is a number of the clock the jobs already had, `Job.Seq`,
  taken as the jobs are read and before the rest. So a state whose rev is
  past a job's seq was read after that job's news and holds every clip
  the job had said it wrote. An edit's answer, `ClipEntry.Rev`, takes a
  number once the edit is written, and the session takes no state older
  than it. The video's state is told again after the answer took its
  number, `answer` in `views.go`, so the state sent after an edit is
  newer than its answer. Told only before, it could be read first, and
  the window would drop it, which the review found.
- A state holds the jobs queued or running and, of those over, the
  newest of each kind. The queue keeps every job since the app opened,
  and the news of the older ones reached the window long ago.
- A job's news goes out once its video's state is due, so a walk that
  hears the news and asks for a mark sees the state first. The app that
  closes sends no state once it has stopped, which a walk that opens it
  again met: its late state was newer than every state of the app
  opened again. One clock for the jobs, the states and the edits, where the
  workspace had three tickets, `Newest`, one for each read.
- What tells: every job's news, through the queue's `moved`, the
  loudness measured further, every edit of the clips through
  `editMoving`, and undo and redo. The `levels` event went into the
  state. The `episode` event stays the library's news, which the sidebar
  reads the whole library on, as What converges left it.
- `EpisodeSession` keeps `status`, `clips` and `coverage`, and
  `Episode.svelte` reads them from it. A state kept hands its jobs to the
  job store where the store has nothing newer, and says how many clips
  each job had written in it, which is what the list counts its rows
  still to come by. A clip changed on screen before Go has it, a
  thumbnail, a keyframe, a turn, goes through `edit`, which holds the
  states that come while it is written, because each was read before
  the edit. `OnTheWay` holds a card by the same clock: until a state is
  kept that was read after the news that took the card off its job.
- The workspace opens with one read of the state in place of three, and
  reads it again where it read the clips or the video before, until
  slice 4. Its timer no longer reads the status while the video is heard.

Measured in a cloud session before it was sent:

- Reading the status of a video of four hours took 125 ms, measured ten
  times, from 105 to 150: 60 to read the transcript, 25 to place its
  words on the sound and 35 to make its lines. Four times a second that
  is half a core. So what the transcript says was heard is kept while its
  file stays as it was, one for each transcript, `heardOf` in
  `engine/heard.go`, and the status
  then took 0.013 ms, from 0.012 to 0.035. It is a package variable, as
  the caches that outlive a job are until #607 gives them one home.
- The state of a video of five minutes with one clip and one job was
  2.2 KB, measured once: 0.6 for the clip, 0.9 for the job and 0.6 for the
  status. Reading it took 0.1 ms, measured five times. A state of fifty
  clips made up like it is 25 KB, and the window parsed it and kept each
  clip that did not change in 0.3 ms, from 0.27 to 1.17, measured ten
  times in Node. Four times a second is about 1 ms of a second, in a
  window that read the whole clip list every two seconds while clips
  were found before.

What slice 4 takes away is still here: the reads the workspace asks for
after work moves on, the poll of the clip list while clips are found,
the poll of the job list, `unread`, and the effects that notice a start.

## Slice 4, first part, as built

Slice 4 is three pull requests, in this order: the moves made with the
state they come with, one `startJob` for the pending state of every
control, and both polls going with #513.

A job's news and its video's state travel apart, and the news comes
first. The moves used to read the video again at the news, and the
answers came one frame after another. Made at the news with no read of
their own, they read a state from before the work: a search was over
with no clip chosen yet, and its window was drawn back where it was for
a frame. So a move is made with the state read after its news. Three
designs were weighed for how the rest of the window keeps in step:

1. The session makes a move once it keeps a state past its news, and the
   rest of the window shows the job at its news. Built first, it showed
   a search over before its clips, so it needed a copy of the search as
   it ran, `held`, which only the workspace read. The store cleared the
   stop on the news, and the copy still ran, so Cancelling turned back
   into Cancel for up to a quarter second. The review of #665 found it.
   A second home for a job, and a render or a clip made by hand had the
   same gap. Dropped.
2. A video's jobs reach the window only inside its state, and the job
   event is left for work with no video. One channel. But a state holds
   only the jobs going and the newest of each kind over, so two renders
   that end within one wait would lose the first, and every caller that
   applies the job a call answered with would need another way in. It
   changes the Go side and the walks' marks, which count the jobs' news.
3. The job store shows the news of a video's job once a state of the
   video read after it is shown, `JobStore.apply` and `shown`. The news
   stays the one complete record of each job, and the state says when it
   may be shown. Every reader of the store, the workspace, the sidebar
   and the stop pressed, sees the end in the same step as the clips it
   left. News that work started or goes on shows at once, so a press
   and the progress of a row still show at once. Chosen.

As built:

- From a video's first state on, the store keeps the news of its jobs
  that moves the work on aside, the newest of each job, until a state
  read after it is shown, then shows it in the order it was sent. That is
  news of a clip written, of the hearing over and of the work over, which
  is what the moves are made of. News that work started or goes on shows
  at once. Held too, the row of a search carried on said its window for
  a moment in place of how much of it was heard, and the frame of Transcribe
  whole video shed no motes, in CI on the first commit of this. Before the first state, as the app
  opens, it shows it at once. While a workspace is open on the video,
  its session says which state it kept, from `take`. The states of the
  other videos are shown as they come. A video that left the library has
  its last news shown at once, `forget`, because no state of it comes
  again.
- So the session makes each move at the store's news, with the state it
  kept a moment before, and no move asks for the video again. `wrote`
  puts the first clip on screen from the state, `made` hands the card to
  its clip, and `searched`, `heard`, `transcribed` and `rendered` read
  only what no state holds, the room for windows and the window moved on
  to, in `workMoved`. `load` keeps its read for what no state says: the
  file gone or back, the app woken, an undo. A move that goes wrong is
  said in the log and stops none after it.
- The state that says a job is over goes out at once, `jobMoved` in
  `cmd/framefairy-app/videostate.go`, rather than at the end of the
  quarter second, so the end shows as soon as it did when it was shown at
  its news.
- The stand-in in `frontend/preview/wails-stub.ts` sends the video's
  state after the news of its jobs, the way the Go side does.
