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

## Slice 4, first part, with every rule in Go

### The question

Tim, on 1 November: Go is the back end and the interface the front end,
they talk through a channel of state with no logic in it, every rule is
in the engine and Go, and the front end has no rules at all, it asks and
shows the answer. The part before kept the rule of which job news moves
the work on in Go and in the session, held to one by a table both read,
and the moves in `Episode.svelte` decide what the screen does: which
clip to choose when a search finds clips, where the window moves on, the
room for windows read again, which failures to tell. Where do those
decisions live, and how does the interface get them?

### Five simulations

Counted at 9c10e5c. The session makes seven moves. In `Episode.svelte`
they use nine values the workspace keeps for them, `listedBefore`,
`pickedBefore`, `shownFirst`, `chosenByFind`, `selected`, `paused`,
`movingOn`, `madeHere` and `landing`, and call the Go side three times,
`chosenWindow`, `room` and `source`. The Go side already keeps the clip
chosen and the window in `chosen.json`, written on every pick through
`ChooseClip` and `ChooseWindow`, and already decides where the window
moves on after a search, `ChosenWindow`. `VideoState` has six fields.

1. **Who may know about whom.** The interface knows only what Go sends.
   Go decides each outcome and puts it in the state: the clip chosen, the
   window and the room for windows, beside the clips. The interface draws
   the state it keeps, and a move is a field that changed, not a rule.
   Crossing the boundary: three fields on `VideoState`. What goes: the
   seven moves, `movesOn`, the order of a state before its news, the
   session's waiting moves, and `moves.cases.json`.
2. **What changes together.** A search's first clip and the choice of it
   change together, so they are decided together, where the clip is
   written: the queue hears the plan written, and if nobody chose a clip
   since the search began, which the Go side knows from `ChooseClip`, it
   writes the first new clip as chosen. The state read after holds the
   clip and its choice, so they reach the screen in one step, with no
   send at once for it and no order to keep.
3. **Where state lives.** Today the clip chosen lives twice, in the
   workspace's `selected` and in `chosen.json`, and the four values that
   say what a search found live only in the workspace. One home:
   `chosen.json` and the search's job, on the Go side. The workspace
   sends a pick and draws what the state says is chosen. Whether the
   video plays is the one value only the workspace has: a clip chosen
   while it plays is shown, and the playhead goes to it once it is
   paused, which is how it is drawn, not what is chosen.
4. **What runs together.** The choice is made on the job's goroutine as
   the plan is written, under `chosenMu`, and the next state carries it,
   at most a quarter second later, the same as the clip. Nothing waits on
   the order of two messages, so neither `sending` nor `later` is needed
   for it. One send at a time for each video stays, because the states
   of one video still go out in the order they were read.
5. **What a test has to build.** A Go test drives a search with the fake
   recogniser and model: the first clip it writes is chosen in the state
   sent after it, a clip chosen by hand during the search stays chosen,
   and the window moves on when it is done. Vitest checks only that the
   workspace draws what a state says. The walks check the screen. No
   table both sides read.

### What converges

- Go decides the clip chosen, the window and the room for windows, and
  sends them in `VideoState`: 1, 2, 3 and 5.
- The interface draws the newest state, and sends picks and drags. It
  decides nothing about the work: 1 and 3.
- `movesOn`, the news sent after its state, the session's waiting moves
  and `moves.cases.json` go: 1 and 4. One send at a time for each video,
  and no backlog behind a slow read, stay: 4.
- Left for the next slice, by the same rule: which failure is told in a
  toast, once, and the card of a clip made by hand handed to its clip.
  Both move to Go as fields of the state.
