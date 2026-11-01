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

## Slice 4, first part

Slice 4 is three pull requests, in this order: the moves made with the
state they come with, one `startJob` for the pending state of every
control, and both polls going with #513.

### The question

The workspace read the video again at every move, and the answers came
one frame after another. Made with no read, a move has to read a state
that holds what the work left. But a job's news and its video's state
travel apart: `q.emit` sends the news at once and makes the state due,
and `videoNews` sends the state up to a quarter second later. So at the
news the session's state is from before the work: a search is over with
no clip chosen yet and its window drawn back where it was for a frame.
Where does the rule live that brings the two together?

The first two commits of #665 were built before this question was asked
as one. The review of #665 found the first, a copy of the search kept as
it ran, `held`, made Cancelling turn back into Cancel. The third, the
job store holding the news of a video's job until a state read after it
is shown, passed CI. Tim asked on 1 November for the five simulations
the change to `docs/ARCHITECTURE.md` needed before any code. They were
run on the code at b0eefbf, and their answer replaced the third.

### Five simulations

Counted at b0eefbf: `q.emit` is called from 12 places in
`cmd/framefairy-app/jobs.go`, each after `q.mu` is let go. The window
reads a video's jobs from the store in `App.svelte`, 12 times, and in
`Episode.svelte`, 14, and applies the job a call answered with 5 times.
Reading a state took 0.1 ms in #663, measured five times.

1. **Who may know about whom.** Go knows the order its news is made in,
   the window only the order it arrives in. The window holding news back
   to put it in order is the window working out what Go knew, which
   #606 set out to end. So Go sends them in the order the window needs:
   the state read after a job's change first, then the job's news. The
   window knows nothing of the order. Crossing the boundary: nothing new.
   Cost: `q.emit` and `videoNews`, one pull request, and the window's
   holding taken out.
2. **What changes together.** A job's end and the clips it left change
   together and should reach the window as one. Either the news carries
   the state, a field `State` on `JobUpdate`, or the state goes out
   right before the news. The field is a second way a state reaches the
   window, beside the `video` event, with a type crossing the boundary
   for it, and the window then has two places to take a state from. The
   order needs no new name. Both are Go's.
3. **Where state lives.** The window has two homes for a video's work,
   the store for its jobs and the session for its state. Holding news in
   the store is a third: the news held, the rev of each video's last
   state and the videos a workspace is open on, 3 fields, 5 methods and a function
   in `lib/state.svelte.ts`, with a rule, which news waits, that copies
   what the session's moves are made of. A copy of the job in the
   session, the first commit, is a second home of the job itself. Go
   sending the state first adds no state anywhere.
4. **What runs together.** Wails delivers the events one Go goroutine
   emits in the order emitted: `EventProcessor.Emit` in Wails
   v3.0.0-beta.23 hands every event to one mailbox, `frontendEvents`,
   which "serializes delivery to all ... transports", and the walks'
   bridge sends each browser its events through one channel, `bus`. So
   the order holds if one goroutine sends a video's state and then the
   news read before it. Reading the state on the job's own goroutine
   would hold the work back for a read, a probe of the video the first
   time. So the news is handed to the video's sender, `videoNews`, which
   already sends one state at a time, and it sends the state and then the
   news it was read after. The worker does not wait. Built that way, with
   every news after its state, the walks found the news of work going on
   cannot wait the quarter second the state may: the row of a search
   carried on said its window for a moment in place of how much of it was
   heard, "Continue fills the transcription from nothing over what is
   left to hear", and the frame of Transcribe whole video shed no motes,
   "a video added is transcribed to its end from its menu". Sending a
   state at once before every news would read the state for every line
   the engine logs. So only the news that moves the work on, what the
   window's moves are made of, goes after a state read after it, which
   goes at once. The rest goes as it comes, unless news of the video is
   still waiting, and then after it, so the order holds.
5. **What a test has to build.** The order in Go is proven by a Go test
   with the send recorded: every news goes after a state whose rev is past
   it. The window then needs no test of its own for the order, and the
   session's tests take each state before the news it holds. Holding
   news in the store needed a Vitest of the store and of the session
   together, in both orders, and the walks to prove the two agree.

### What converges

- Go sends a video's state before the news of its job that the state
  was read after, for the news that moves the work on: 1, 2 and 4, and 3
  and 5 by what they spare. The order
  comes from the one sender of each video, `videoNews`, which 4 needs
  and 2 allows.
- The window holds nothing back and decides nothing about the order: 1,
  3 and 5. The session makes each move as the store keeps the job, which
  is when the state that holds it has just been kept.
- A field `State` on `JobUpdate` is only in 2, and is dropped for the
  second way into the window it makes.
- The job store holding news, which was built, is in none of the five,
  and goes.

```
screens/Episode.svelte     a view: reads the session, asks Go through api.ts
lib/session.svelte.ts      one video's state, kept by revision, and what its work did
lib/state.svelte.ts        every video's jobs, each snapshot kept handed on
lib/api.ts                 the calls and the events of Go
------------------------   events in the order Go sent them
videostate.go videoNews    a video's state, then the news of its jobs read before it
jobs.go q.emit             a job's news, and whether it moves the work on
```

### As built

- `q.emit` hands the news of a video's job to `videoNews.after`, with
  whether it moves the work on, `movesOn` in `jobs.go`: the job over, a
  clip written, or its hearing over, from what the window was told of
  the job before. Such news goes out right after a state of the video
  read after it, which is sent at once. The news of a job started or
  going on goes out as it comes, unless news of the video waits for its
  state, and then right after it, in the order it came. The news of work
  with no video, the models, goes out at once. A video that left the
  library, or whose state cannot be read, still has its news sent.
  Nothing is sent once the app quits.
- One send at a time for each video, so the states leave in the order
  they were read, b0eefbf, found by the Mac's CI on main.
- `EpisodeSession.take` hands the state's jobs to the store after it
  kept the state, and the store tells the session, which makes each move
  then, with what the state holds. The news that follows says nothing
  newer and is not kept. No move asks for the video again. `wrote` puts
  the first clip on screen from the state, `made` hands the card to its
  clip, and `searched`, `heard`, `transcribed` and `rendered` read only
  what no state holds, the room for windows and the window moved on to,
  in `workMoved`. `load` keeps its read for what no state says: the file
  gone or back, the app woken, an undo. A move that goes wrong is said in
  the log and stops none after it.
- While an edit holds the states that come, their jobs still reach the
  store, so a move made then reads the clips from before the edit's
  state. An edit is written in milliseconds, and the next state puts the
  clips right.
- The stand-in in `frontend/preview/wails-stub.ts` sends a video's state
  before the news of its jobs, the way the Go side does.

## Slice 4, first part, after its second review

### The question

The second review of #665 found four ways a move still reads a state
from before its work, and one rule kept twice. A move is made when the
store keeps a job's news. That is right only when the state the session
keeps was read after that news, and four things break it:

- An edit holds the states that come while it is written, `holds` in
  `EpisodeSession`, and their jobs still reach the store, so the move is
  made with the clips from before.
- A state read before an edit's answer is dropped, `floor`, and its
  jobs reach the store the same way.
- `q.emit` runs after `q.mu` is let go, from 12 places, so two
  goroutines can hand on two news of one job the other way round, and
  the later can go out before the state the earlier asked for.
- Which news moves the work on is decided twice: `movesOn` in
  `jobs.go` and the moves in `EpisodeSession.seen`. A move added to the
  session and not to Go would have its news go out first. No test holds
  the two to one.

So the question: what makes a move read the state its work left in
every case, and where does the rule live of which news moves the work
on?

### Five simulations

Counted at 5379ac4. `seen` makes five moves from a job's news, from
three things about it: whether it is going, how many clips it wrote and
whether it is hearing, and `asked` adds the work asked for here.
`movesOn` decides from the same three, kept in `q.told`, a map beside
`q.jobs` with a mutex of its own, never emptied. A state's `rev` and a
job's `seq` come from one clock, the queue's, and both reach the window
already. Every news of a video's job asks for a state, at once or
within a quarter second.

1. **Who may know about whom.** Go knows the order its news was made in.
   The window knows two numbers Go gave it: the rev of the state it
   keeps and the seq of each job. A state whose rev is past a job's seq
   was read after that news and holds what the work left. The session
   checking that before it makes a move works out nothing Go knew, it
   reads Go's own numbers. Then the order Go sends in decides only how
   soon a move is made, never whether it reads the right state. Crossing
   the boundary: nothing new.
2. **What changes together.** A move and the rule for which news makes
   it change together, so they would live in one place. Go deciding and
   sending what moved, a field on the job, fails on what the store
   keeps: a state hands the store a job several stamps on, seq 8 where
   the store had 5, and the news of 6 and 7 then says nothing newer. A
   flag of one stamp loses the moves of the stamps between. Flags that
   survive that have to add up, and what adds up is already sent: the
   clips written, the step, whether it is going. So a move is a
   comparison of two snapshots, and only the window has the two it
   compares. Go needs the rule only to know when to read a state first.
   The copy stays, held to one by a table of cases both sides read, the
   form `docs/ARCHITECTURE.md` gives a rule kept on both sides.
3. **Where state lives.** `q.told` is a second record of every job
   beside `q.jobs`, kept by hand in step with it. What the last stamp
   said of a job belongs on the job, where the stamp is: two fields of
   `Job` not sent to the window, no map and no mutex. In the session, the
   moves whose state is not in yet are one list, emptied as states are
   kept. It replaces nothing in `holds` or `floor`, which still keep an
   edit on screen, and makes them safe for the moves.
4. **What runs together.** Judging `movesOn` at the stamp, under `q.mu`,
   makes the judgement follow the order of the stamps, but the news is
   still handed on after the lock is let go, so two goroutines can still
   send it the other way round. Nothing that runs in Go can hold the
   order to the far end, through Wails, the bridge or the stand-in. Only
   a check at the far end, on the numbers, holds whatever runs. It costs
   a wait only when the order broke: the next state is due within a
   quarter second, and an edit's answer in milliseconds.
5. **What a test has to build.** Today the agreement of the two rules is
   proven by nothing, and a walk would only see it as a flash. A table
   read by a Go test of `movesOn` and by a Vitest of the session proves
   it in milliseconds, and a row added for a new move fails on the side
   not changed. The check in the session is proven by Vitest alone: news
   before its state makes no move until the state, an edit holding its
   states, a state dropped under the edit's answer.

### What converges

- The session makes a move once the state it keeps was read after the
  job's news, rev past seq: 1, 4 and 5. Until then the move waits, in
  the order its news came. Every case the review found becomes a wait,
  never a move with the wrong clips.
- Go's rule decides how soon, not whether: 1 and 4. It is decided as
  the job is stamped and kept on the job, and `q.told` goes: 3 and 4.
- One table, `frontend/src/lib/moves.cases.json`, holds the two rules
  to one, and a new move takes a row there: 2 and 5.
- Go sending what moved is dropped, by 2.

### As built

- `EpisodeSession.seen` puts each snapshot of a video's job in a list,
  and the moves of the first are made once the state kept was read
  after it, `knows`, then the next. `take` makes the waiting moves as it
  keeps a state, before the moves of the jobs the state hands on.
  `asked` puts a job the store has over in the same list. This replaces
  the bullet above on an edit holding the states: a move made then now
  waits for the state taken after the edit.
- A move whose news came before its state, with no edit holding the
  states, is said in the app's log as a warning: the Go side's rule
  missed it, or the order broke on the way.
- `movesOn` in `jobs.go` is decided in `stampLocked`, from what the
  job's last stamp said, kept on the job, and `q.told` and its mutex
  are gone. It now says what the table says for a search first heard of
  as it runs, which moves the work on when it has written clips, and
  for work first heard of as over, which moves it on only when it could
  have been asked for here, a clip made by hand or a render.
- `frontend/src/lib/moves.cases.json` has 22 cases, read by
  `TestTheNewsThatMovesTheWorkOnIsTheNewsTheWindowMovesOn` in Go and by
  `session.test.ts`, each with the moves made for work asked for
  elsewhere and for work asked for here.
- `videoNews.tell` waits for the send before it first, so the changes
  and the news that come meanwhile join it: one send runs and one waits,
  review finding 1.
- Simulations 2 and 3 of the part before said the order needed no new
  name and added no state. The build added both: `movesOn`, with its
  record of every job, and the news a video's sender holds. The first now
  lives on the job, the second is in Where state lives.

## Slice 4, every rule in Go

### The question

Tim, on 1 November: Go is the back end and the interface the front end,
they talk through a channel of state with no logic in it, every rule is
in the engine and Go, and the front end has no rules at all, it asks and
shows the answer. Section A of the assessment of that evening,
`/mnt/project-files/reviews/frontend-rules-assessment.md`, copied onto
#606, counts twelve kinds of decision the interface still makes about a
video's work and its workspace, among them the clip chosen in every
case, the window fitted and moved on, how a search ended, which failures
are told, the card of a clip made by hand, and two polls. The parts
before this one asked where in the interface those decisions go. This
part asks first which side owns them, and how the interface gets what
was decided.

An earlier version of this section, in e4c7d72, had five simulations
that were one candidate seen from five sides. A fresh review found that,
and thirteen more things wrong with it, all worked in here.

### What the code is today

Counted at e4c7d72.

- A video's work reaches the interface on two channels. The `job` event
  carries every snapshot of every job, at once, 12 places in `jobs.go`
  call `q.emit`. The `video` event carries `VideoState`, six fields,
  `video`, `rev`, `status`, `clips`, `coverage` and `jobs`, read when it
  is sent, at most four times a second for each video. Every job has a
  video except the downloads of the two models, kinds `model` and `llm`.
- Because the two travel apart, #665 had to send a state before the news
  that moves the work on, and to know which news that is, `movesOn`, a
  copy of the session's moves held to them by `moves.cases.json`.
- The session makes seven moves. In `Episode.svelte` they use nine
  values kept for them, `listedBefore`, `pickedBefore`, `shownFirst`,
  `chosenByFind`, `selected`, `paused`, `movingOn`, `madeHere` and
  `landing`, and call the Go side four times, `chosenWindow`, `room`,
  `source` and, through `onchange`, `library`.
- The clip chosen and the window live in `chosen.json`, behind two
  package locks, `chosenMu` and `windowMu`. `ChooseClip(path, key)`
  writes it on every pick and says nothing back. `ChooseWindow` writes
  the window the interface fitted, with `byHand`. `windowOn` moves the
  window on after a search, and `ChosenWindow` reads it back. A video's
  undo steps live in memory, in `history`, one for each video, behind its
  own lock, `historyOf`.
- The interface fits the window itself with `fitWindow`, `leastWindow`,
  `mostClips` and `longestShortest` from `RoomView`, which carries a
  weight for every line of the transcript.
- `Episode.svelte` has 121 derived values and 62 kept ones.

### Five simulations

#### 1. Who may know about whom: one channel of state

The interface knows a video only from its `VideoState`. A video's jobs
travel only in it, and the `job` event carries only the two downloads.
Go decides every outcome and puts it in the state as a field: the clip
chosen, the window, and on each job how it ended. The interface sends
what the hand did and draws the newest state.

- Layers: the engine, then the Go side, which keeps and decides, then
  the channel, then the interface, which draws. Nothing in the interface
  compares two snapshots.
- What moves: the seven moves and their nine values go. The clip chosen
  and the window are decided where the clips and the window are written.
  A job and the clips it wrote always come in one message, so `movesOn`,
  `videoNews.after` with `later` and `held`, the session's waiting moves,
  `moves.cases.json`, `OnTheWay`'s hold, `unread`, `session.wrote`, the
  comparison of rev with seq in the session and `take` handing jobs to
  the store all go. The slice 3 walk findings were about news held
  behind a state, which cannot happen with one message.
- Crossing the boundary: three fields on `VideoState`, the clip chosen,
  the window and what a drag may reach, and one on `Job`, how it ended.
  The video's jobs leave the `job` event.
- Where it breaks: a failure told once. A field of a coalesced state is
  lost when the session drops that state, `holds` and `floor`, or it
  needs a set in the interface of the failures told, the same rule
  again. And a pick shown at once can be undone by a state read before
  Go had it.
- Next likely changes: a new outcome, a clip chosen after a render, is a
  field and a decision in Go. Clips chosen while the video plays, if Tim
  wants them, is one condition in Go.
- Cost: about 25 files, in six or seven pull requests, most of it
  removed code.

#### 2. What changes together: outcomes as notices

An outcome changes together with the moment it is decided, so Go sends
each as an event as it decides it: chose this clip, moved the window
there, this failed, this card is now that clip. The state stays what it
is, and the interface does what each notice says.

- Layers: as today, plus a third channel of notices beside `job` and
  `video`.
- What moves: the decisions move to Go as in 1. The interface keeps a
  handler for each notice.
- Crossing the boundary: four new events, or one with four kinds.
- Where it breaks: "chose clip X" can come before the state that holds
  X, which is the order problem #665 spent two rounds on, now for
  notices. A workspace that opens, or is put back after the app slept,
  has missed every notice, so it needs the clip chosen and the window in
  the state anyway. The notice is then a second way to say a field,
  which drifts.
- What it gets right: a failure is said once, at the moment it happens,
  and is not a fact about the video's state. It fits a notice and fits a
  state badly.
- Next likely changes: each new outcome is an event and a handler, and
  each handler needs to know what the state says meanwhile.
- Cost: fewer files than 1, but it keeps both channels and adds a third.

#### 3. Where state lives: a view model in Go

Go keeps everything the workspace shows: the rows and their words, the
clip chosen, the window, which buttons are on, what the info marks say.
The interface is a template that draws it and sends every gesture.

- Layers: the engine, then a view model per workspace in Go, then a
  template.
- What moves: the decisions of 1, and the 121 derived values of
  `Episode.svelte` with them, the words of `steps.ts`, the counts of the
  rows, the button labels.
- Crossing the boundary: one view per workspace, its size the size of
  the screen, sent on every change.
- Where it breaks: a click shows at once and a drag follows the hand, so
  the interface needs its own state for what the hand is doing anyway,
  and the view model then has a copy in the interface to reconcile. The
  words and the layout are the interface's to say, and the interface
  rules keep them there, in `title`, in the info marks, in the rows. Go
  would grow a second front end's worth of presentation.
- What it gets right: how a search ended is an outcome, not words. Go
  can send it as a value, found, found nothing, stopped while the
  window was heard, stopped while the clips were found, and the
  interface picks the words.
- Next likely changes: every change of a word or a row is a change to
  Go and the template.
- Cost: most of `Episode.svelte` rewritten, well over a thousand lines,
  in more pull requests than #606 has left.

#### 4. What runs together: one owner for each video

Each video's decisions are made in one place, in one order: every job
stamp, pick, drag, play and pause comes to it, and it decides the clip
chosen and the window and marks the state due. Today they are made from
several goroutines under two package locks, `chosenMu` and `windowMu`,
and an edit's lock, `history.mu`.

- Layers: the engine, then the queue, then one record of each video
  that decides, then the channel, then the interface.
- What moves: `chosenMu` and `windowMu` go, the window's placing and
  moving on and the clip chosen are made under the video's own lock.
  The queue tells the video's record when a job of it is stamped.
- The form: a goroutine for each video, fed by a mailbox, would make
  the queue wait under `q.mu` on a mailbox that is full, or drop a fact.
  The record the Go side keeps of each video already, `history`, with
  its lock, gives the same one order with none of that. It is extended:
  it keeps the clip chosen, who chose it, the window, whether the video
  plays and the search's choice, beside the undo steps, and writes what
  outlives the app through to `chosen.json`. One record of a video, and
  the file is only where it starts from again.
- Crossing the boundary: nothing new, it is how the Go side is built.
- Where it breaks: a decision made under the video's lock that reads the
  plans takes the lock for a read of the disk. The read is made before
  the lock, as `videoWindow` probes before `windowMu` today.
- Next likely changes: a new fact from the hand is one method on the
  record.
- Cost: `chosen.go`, `window.go`, `history.go` and the queue's tell, in
  the pull request that moves the clip chosen.

#### 5. What a test has to build: ask and answer

Tim's words taken as they stand: the interface asks and gets a
response. Go only says that a video changed, at a rev. The interface
asks for its state, and every call that changes a video answers with
the state after it.

- Layers: the engine, then the Go side, then calls only.
- What moves: the decisions to Go, as in 1. `videoNews` sends a number
  instead of a state.
- Crossing the boundary: one small event, and every call that changes
  a video answers with a state.
- Where it breaks: every change is two trips, the news and the read,
  and the sidebar needs every video's jobs, so it asks for every video
  on every change. A state pushed is the answer to that ask, sent
  without the ask. A test of it builds the same as a test of 1, plus the
  ask.
- What it gets right: the interface can tell when its own change is in.
  A pick answered with a number from the clock of the states is kept on
  screen until a state past that number comes, which is what `floor`
  does for an edit already. Without it, a state read just before Go had
  the pick comes after the pick and puts the clip before back for a
  moment.
- What a test builds for 1, 4 and 5 alike: a Go test drives a search
  with the fake recogniser and model, or stamps a job by hand in a
  `synctest` bubble, and reads the state sent. Vitest checks that the
  workspace draws what a state says. No table both sides read.
- Cost: as 1, plus a read for each news.

### What converges

- **One channel**, 1: a video's jobs travel only in its `VideoState`,
  and the `job` event carries only the downloads of the models. Every
  outcome that is a fact about the video is a field of the state:
  1, 3 and 5.
- **Go owns every decision**: 1 to 5. The interface sends what the hand
  did, a pick, a drag of the window, a number typed, play and pause, and
  draws the newest state.
- **One record of each video in Go**, 4: `history` becomes the record of
  the video's workspace on the Go side, behind its one lock, and keeps
  the clip chosen and who chose it, the window, whether it plays and the
  search's choice. It writes the clip chosen and the window through to
  `chosen.json`, which is read only as the record is made. `chosenMu`
  and `windowMu` go. A state reads the record, not the file.
- **A call that changes a video answers with a rev**, 5: `ChooseClip`
  answers with a number from the clock, and the session keeps the pick
  until a state past it, the way `floor` keeps an edit. `ChooseWindow`
  answers with the window Go fitted and its rev.
- **Failures are notices**, 2: one event for each failure, decided where
  Go sees the job go from running to failed, or refused as it starts.
  `Toasts.svelte` shows it. It is not a fact about a video's state, so
  it does not break "a video's state travels in one message".
- **How a search ended is a value**, 3: Go says it on the job, the
  interface picks the words.
- Dropped, each by the failure its own simulation found: notices for
  facts of the state, 2, a view model in Go, 3, asking after each news,
  5, and a goroutine for each video, 4.

### The rules Go takes over

**The clip chosen.** Go keeps the clip chosen and who chose it, the
hand or the app. A `ChooseClip` from the interface is always a hand,
because every choice the app makes by itself is made in Go in the same
pull request. The app chooses only while the video is paused, which it
knows from `Playing`, so a picture never jumps from where it was playing
to a clip nobody asked for, and the clip timeline does not move while
it plays.

1. A search writes its first clip: if the clip chosen is still the one
   chosen when the search began, the earliest clip the search has
   written is chosen, and kept as the search's choice. Once for each
   search.
2. A search ends done: if the clip chosen is still the one chosen when
   it began, or the search's choice, the earliest clip of what it found
   is chosen. A window searched again comes back under the names its
   clips had, so what it found is the clips of its plan, `Job.Result`,
   not the clips that are new to the list.
3. A search that stopped, was cancelled or failed chooses nothing at
   its end.
4. A video opened chooses the clip chosen last, or else the first.
5. A clip removed chooses the clip beside it.
6. An undo or a redo chooses the clip it changed.
7. A clip made by hand is chosen as its card from the moment it is
   asked for, and the same write that writes the clip makes the clip the
   one chosen, as long as the card is still chosen. `madeHere`,
   `landing` and the `made` move go.

**The window.** Go places it, moves it on after a search, puts it on a
search Go started by itself, and fits it: `ChooseWindow` answers with
the window fitted to what one search can read, and a change of what one
search can read refits it in Go. For a drag, which has to follow the
hand in the same frame, Go sends with the state what a window starting
at each point may reach, the least length and the most clips, with a
version, only when it changed. Its size is measured on a video of four
hours before it is sent, in the pull request that moves the window.
`fit`, `keepWindow`, `rememberWindow`, `keepTarget`, `fitWindow`,
`leastWindow`, `mostClips` and `longestShortest` go, and so does the
effect `followed`.

**How a search ended**, on its job: found, found nothing, too short for
Shortest, stopped while the window was heard, stopped while the clips
were found, and the rows still to come. `halted`, `foundNothing`,
`emptied`, `coming` and `unread` read it instead of working it out.
`JobStore.search`, `clips` and `transcription` read which job is the
video's from the state.

**Stopped work forgotten** is done by Go when its time is up, and the
interface's timer in `callOff` and `send` goes.

**The polls go.** One channel needs no job poll to put a lost event
right, because the next state carries every job of the video. The
workspace's 2 s read of the clips and the room goes once the clips and
the reach come in the state. `onchange` reading the library goes,
because Go sends library news at the end of every job.

### The final candidate, as layers

```
 interface  frontend/            draws the newest VideoState, says notices,
                                 sends picks, drags, play and pause
     ^ VideoState (video event)  ^ notice event   | calls, answered with a rev
 channel    videoNews            one send at a time for each video,
                                 coalesced, no backlog
     ^
 Go side    the video's record   clip chosen, who chose it, window, playing,
            (history, extended)  the search's choice, undo steps,
                                 written through to chosen.json
            queue                jobs, stamped on the one clock
     ^
 engine     engine/              plans, transcripts, placing and fitting
                                 the window, how far a search can read
```

Each uses only the one under it. The interface holds no rule of the
engine and decides nothing about a video's work.

### The pull requests

Each one removes what it replaces, so every merge leaves the app working
and fewer rules in the interface. Each is listed on #606 as it opens.

1. **One channel**, #665: the serial sender with no backlog, and a
   video's jobs only in its state. `movesOn`, `videoNews.after`, the
   session's waiting moves and `moves.cases.json` go, and the moves are
   made from the jobs a state brings, for as long as they last.
2. **The clip chosen and the window kept in Go**: the record of each
   video in memory, written through, both in `VideoState`, `ChooseClip`
   answering with a rev, `ChosenClip` and `ChosenWindow` gone.
3. **Go chooses the clip**: rules 1 to 7 above, `Playing`, the `wrote`
   and `made` moves and their nine values gone.
4. **Go fits the window**: the reach in the state, measured first, the
   fitting in the interface and `followed` gone.
5. **Failures as notices**: the `failed` move, `told`, and App's watch
   of transcription failures gone.
6. **How a search ended, from Go**, with forgetting stopped work and
   the polls gone, and `EpisodeSession`'s moves, `WorkMoves` and
   `onchange` from them gone. #513 with it.
7. **The architecture's own lines**: the exception in layer 1, the copy
   row, the move row and the "what is on screen" row rewritten, and
   invariant 5 held by the design. Each earlier pull request already
   changes the lines it makes wrong, so this one is what is left.
