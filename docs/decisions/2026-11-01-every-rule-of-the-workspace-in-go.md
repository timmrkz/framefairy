# Every rule of the workspace is in Go

Issue #606, from the architecture review of 29 October 2026 (#603).
Decided on 1 November 2026, in #665, the first of the pull requests that
finish #606. The others name this record. It replaces the part of
[2026-10-30-a-videos-state-in-one-session.md](2026-10-30-a-videos-state-in-one-session.md)
that put the workspace's reactions to a video's work in the interface.
The rest of that record stands: a job holds its engine record, and Go
sends a video's whole state as one message, with a revision, coalesced.

## The problem

Tim, on 1 November 2026:

> my understanding of the app is that Go is just the back end, and like
> this TypeScript thing is just the front end, and they should just talk
> with one another through some protocol. And then that's just like a
> channel of state without any logic. And then there can be
> communication back and forth, and all of the rules should just be in
> the engine and Go in that back end. And the front end shouldn't have
> any logic or rules at all. The front end should just ask for shit and
> then get a response.

Section A of the assessment of that evening, copied onto #606, counts
twelve kinds of decision the interface still makes about a video's work
and its workspace: the clip chosen in every case, the window fitted and
moved on, how a search ended, which job is the video's, which failures
are told, stopped work forgotten by a timer, two polls, the library read
again, the transcription stopped for a search and a number typed for its
window. The record this one replaces asked where in the interface those
decisions go. This one asks first which side owns them, and how the
interface gets what was decided.

### What was tried first

The first pull request of the last slice of the record before, #665,
needed each move of the session to read the clips the work left. Its
commits tried three ways, each with five simulations that asked only
where in the interface or the sender the order is kept: the job store
holding a video's news until its state, Go sending a state before the
news that moves the work on, and the session waiting with each move for
a state read after its news. The last needed the rule of which news
moves the work on twice, `movesOn` in Go and the moves in the session,
held to one by a shared `moves.cases.json`. Tim saw that and called it
wrong. An earlier draft of this record, e4c7d72, then had five
simulations that were one candidate seen from five sides, which a fresh
review found with thirteen more things wrong. All three ways were taken
out of #665 again before it was merged, 2be6525. Only the fix they had
found on the way stays: a video's states go out one at a time.

## What the code is today

Counted on main at 93a2ef6, which #665 leaves as it is but for the send
one at a time.

- A video's work reaches the interface on two channels. The `job` event
  carries every snapshot of every job, at once, 12 places in `jobs.go`
  call `q.emit`. The `video` event carries `VideoState`, six fields,
  `video`, `rev`, `status`, `clips`, `coverage` and `jobs`, read when it
  is sent, at most four times a second for each video. Every job has a
  video except the downloads of the two models, kinds `model` and `llm`.
- Because the two travel apart, each move of the session reads the
  video again when it is made, and the answers come one frame after
  another, #513.
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

## Five simulations

### 1. Who may know about whom: one channel of state

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
  A job and the clips it wrote always come in one message, so no order
  of two messages has to be kept and nothing has to know which news
  matters: `OnTheWay`'s hold, `unread`, `session.wrote` and `take`
  handing jobs to the store all go. The walk findings of #665 were about
  news held behind a state, which cannot happen with one message.
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

### 2. What changes together: outcomes as notices

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

### 3. Where state lives: a view model in Go

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

### 4. What runs together: one owner for each video

Each video's decisions are made in one place, in one order: every job
stamp, pick, drag, play and pause comes to it, and it decides the clip
chosen and the window and marks the state due. Today they are made from
several goroutines under two package locks, `chosenMu` and `windowMu`,
and an edit's lock, `history.mu`.

- Layers: the engine, then the queue, then one record of each video
  that decides, then the channel, then the interface.
- What moves: `chosenMu` and `windowMu` go, the window's placing and
  moving on and the clip chosen are made under the video's own lock.
  The queue tells the video's record of each job of it once stamped.
- The form: a goroutine for each video, fed by a mailbox, would make
  the queue wait under `q.mu` on a mailbox that is full, or drop a fact.
  The record the Go side keeps of each video already, `history`, gives
  one place for it, but not its lock: `history.mu` is held for a whole
  edit, two snapshots of the plans read from the disk included,
  `history.go`, so a fact told under it would wait behind every edit,
  the cost that rules out the mailbox. So the record is extended with a
  small lock of its own, held only to read or change what is chosen,
  never for a read of the disk. It keeps the clip chosen, who chose it,
  the window, whether the video plays and the search's choice, beside
  the undo steps, and writes what outlives the app through to
  `chosen.json`. One record of a video, and the file is only where it
  starts from again.
- The queue tells it of a job after `q.mu` is let go, the way `q.emit`
  runs, with the job's seq. Two goroutines can hand on two snapshots of
  one job the other way round, so the record keeps the seq of the
  newest it applied and takes no older one.
- Crossing the boundary: nothing new, it is how the Go side is built.
- Where it breaks: a decision that reads the plans, the earliest clip a
  search found, reads them before it takes the lock, as `videoWindow`
  probes before `windowMu` today, and applies its answer only if
  nothing newer was applied meanwhile.
- Next likely changes: a new fact from the hand is one method on the
  record.
- Cost: `chosen.go`, `window.go`, `history.go` and the queue's tell, in
  the pull request that moves the clip chosen.

### 5. What a test has to build: ask and answer

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

## What converges

- **One channel**, 1: a video's jobs travel only in its `VideoState`,
  and the `job` event carries only the downloads of the models. Every
  outcome that is a fact about the video is a field of the state:
  1, 3 and 5.
- **Go owns every decision**: 1 to 5. The interface sends what the hand
  did, a pick, a drag of the window, a number typed, play and pause, and
  draws the newest state.
- **One record of each video in the app's Go side**, 4: `history` is
  extended into the record of the video's workspace, and keeps the clip
  chosen and who chose it, the window, whether it plays and the search's
  choice, behind a small lock of its own, apart from the lock of an
  edit. The queue tells it of a job after `q.mu` is let go, and it takes
  no snapshot older than the newest it applied. It writes the clip
  chosen and the window through to `chosen.json`, which is read only as
  the record is made. `chosenMu` and `windowMu` go. A state reads the
  record, not the file.
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

## The rules Go takes over

Each rule is placed in the lowest layer that can hold it. What is about
the video and its search belongs to the engine, where the command line
reaches it too: fitting the window, what a window may reach, and how a
search ended. What is about the workspace of the app belongs to the
app's Go side, `cmd/framefairy-app/`: the clip chosen, whether the video
plays, the notices, and forgetting stopped work.

**The clip chosen**, the app's Go side. Go keeps the clip chosen and who chose it, the
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

**The window.** The engine places, moves on and fits it, through
`engine.Project`, and the app's Go side keeps it and calls it. Go places it, moves it on after a search, puts it on a
search Go started by itself, and fits it: `ChooseWindow` answers with
the window fitted to what one search can read, and a change of what one
search can read refits it in Go. For a drag, which has to follow the
hand in the same frame, Go sends with the state what a window starting
at each point may reach, the least length and the most clips, with a
version, only when it changed. Its size is measured on a video of four
hours before it is sent, in the pull request that moves the window.
`fit`, `keepWindow`, `rememberWindow`, `keepTarget`, `fitWindow`,
`leastWindow`, `mostClips` and `longestShortest` go, and so does the
effect `followed`. A number of clips typed for a window is the target of
that window and no other, which Go keeps with the window, so `typed` in
`Episode.svelte`, a copy of `Settings.targetFor`, goes. The window and
the clips a search found come in the same state, so they are drawn in
the same frame, and `ChosenWindow` and the `searched` move's read of it
go in the same pull request.

**How a search ended**, the engine, on its job: found, found nothing, too short for
Shortest, stopped while the window was heard, stopped while the clips
were found, and the rows still to come. `halted`, `foundNothing`,
`emptied`, `coming` and `unread` read it instead of working it out.
`JobStore.search`, `clips` and `transcription` read which job is the
video's from the state.

**Stopped work forgotten** is done by the app's Go side when its time
is up, and the interface's timer in `callOff` and `send` goes.

**The transcription stopped for a search** is said by Go: asking for
work on the clips stops the video's transcription, `stopTranscribing`,
and the job says it is stopping from that moment, so `JobStore.clipsAsked`
goes.

**A press shown at once.** What the hand pressed is the interface's to
show until Go answers, the way a pick is: one `startJob`, which keeps
the press until the state holds the job, as Tim's comment of 29 October
on #606 asks, so no control keeps a flag of its own, `stopping`, `renderAsked`,
`renderStopping` and `pressed`.

**The polls go.** One channel needs no job poll to put a lost event
right, because the next state carries every job of the video. The
workspace's 2 s read of the clips and the room goes once the clips and
the reach come in the state. `onchange` reading the library goes,
because Go sends library news at the end of every job.

## The decision, as layers

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

## What follows

It costs seven pull requests, most of them removing code, and buys an
interface that decides nothing about a video's work: what a person sees
of a search, its clips, the clip chosen and the window come from one
place and change in one frame. It would show it was wrong if the state
grew so large that sending it four times a second for a video slowed
the workspace, which is why what a drag may reach is measured before it
is sent, or if the hand had to wait for Go to see its own click, which
a pick answered with a rev and one `startJob` are there to prevent.

Each pull request removes what it replaces, so every merge leaves the app working
and fewer rules in the interface. Each is listed on #606 as it opens.

1. **One send at a time**, #665: a video's states go out one at a time,
   with at most one more waiting however slow a read is, which fixes
   `TestPathWhatChangesAVideoSendsItsState` on the Mac. And this record,
   and the two lines of `docs/ARCHITECTURE.md` that let a rule of the
   engine into the interface held by a case file. The moves stay as main
   has them, reading the video again, until they go. One channel would
   make the order of a job and its clips hold by itself, but while the
   moves are still made in the interface it would need waiting moves
   again, so it comes once they are gone, in 6.
2. **The clip chosen and the window kept in Go**: the record of each
   video in memory, with its own lock, written through, `ChooseClip`
   answering with a rev, `chosenMu` and `windowMu` gone.
3. **Go chooses the clip**: the clip chosen in `VideoState`, rules 1 to
   7 above, `Playing`, `ChosenClip`, the `wrote` and `made` moves and
   their nine values gone.
4. **Go fits the window**: the window and the reach in the state,
   measured first, the target typed kept with its window, the fitting in
   the interface, `typed`, `followed` and `ChosenWindow` gone.
5. **Failures as notices**: the `failed` move, `told`, and App's watch
   of transcription failures gone.
6. **One channel, and the moves gone**: a video's jobs only in its
   state, how a search ended from Go, stopped work forgotten by Go, the
   transcription said stopping by Go, one `startJob`, `EpisodeSession`'s
   moves, `WorkMoves`, `onchange` from them and both polls gone. #513
   with it. It may split in two or three once it is worked out.
7. **The architecture's own lines**: the move row and the "what is on
   screen" row rewritten, and invariant 5 held by the design. Each
   earlier pull request already changes the lines it makes wrong, so
   this one is what is left.
