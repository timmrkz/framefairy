# One episode per video, read again only when its files changed

Step 3 of #673, which closes #608. It carries on the record
[the app in step with its files](2026-11-01-the-app-in-step-with-its-files.md),
which named an `Episode` per video in the engine without working it
through. Decided on 3 November 2026, in the first pull request that
builds it. The step takes several pull requests, and each names this
record.

## The problem

Tim said "go on with the next steps" on 3 November, once steps 1 and 2
were merged. Step 3 of #673 reads: "One `Episode` per video in the
engine that holds its plans, status and transcript by file stamp and
reads a file again only when it changed. The app's own transcript cache
goes. Closes #608." Tim on 1 November: "The source of truth are the files
on the file system. And the files on the file system can potentially be
written by many writers." So whatever holds a video's files in memory
holds a copy, and has to know when the copy no longer matches the file.

Measured on main at b8cb415:

- A video's transcript is kept twice, by two caches that each decide
  for themselves when to read it again. The engine's `heardKept` in
  `engine/heard.go`, a package variable, keeps what `Status` reads of
  the last ten transcripts, by the file's size and time. The app's
  `keptReads` in `cmd/framefairy-app/words.go` keeps the last ten whole
  transcripts with their weighed lines, by the same size and time of
  every transcript file, and reads one once however many ask at once.
- Nothing keeps a plan. `Status`, `PlanSummaries`, `ReadPlan` and
  `LoadClips` read and parse the plan files for every question. The app
  calls them from 8 places, and builds `engine.NewProject(nil, ...)` for
  its paths in 7 more, a project that panics if asked to run.
- Reading the transcript of a four hour video took 125 ms in a cloud
  session, 60 of them to read the file, see `heardKept`. A plan read
  takes 0.10 ms and shaping a clip while an edge is dragged 0.55 ms, one
  run each in a cloud session on 1 November. Plans are cheap, the
  transcript is not, and the copies are what can go wrong.
- `engine.Machine`, from #644, is made once in each program and lives as
  long as it. #646 and #649 move the other caches onto it.

## Five simulations

### 1. One holder per video in the engine, asked every time

The principle: a video has one object in the engine, and every question
about its files goes to it. `Machine.Episode(source)` gives the one
`Episode` for that video, made the first time it is asked for. The
episode holds, for each kind of file, what it read and the stamp of the
files it read it from, size and time, and on every question it looks at
the stamps, a `Stat` per file, and reads again only what changed.

Layers: the app's bindings and the command line over `Episode`, over the
parse of each file, over `fileio` and the file system. What moves:
`heardKept` becomes the episode's, the app's `keptReads` goes, the reads
of `Status`, `PlanSummaries`, `ReadPlan` and `LoadClips` go through the
episode. What crosses the new boundary: `Machine.Episode`,
`Episode.Transcript`, `Episode.Plans`, `Episode.Status`. `editPlan`
writes through the file as it does and the episode sees the new stamp on
the next question, so nothing has to tell it.

Next changes: steps 4 to 6 of #673 touch nothing of it. A third kind of
file, the loudness, is a field. Cost: a `Stat` per file per question,
microseconds, and the memory of the transcripts kept, which stays at
ten as today.

### 2. The watch says what changed, nothing asks the disk

The principle: since step 2 the app hears every change to a video's
folders, so a copy is dropped when the watch names its file, and a
question never looks at the disk at all.

It reads nothing it does not need to and stats nothing. But it holds
only where a watch runs. The command line has none, a test in memory
has none, and a change the system's queue lost leaves the copy wrong
until something else names the file, which is the very harm the review
of #675 found in a copy kept beside the disk, finding 2. The disk would
no longer be the truth, the watch would. Its hints are good for one
thing here: a copy can be dropped early, which costs nothing and
changes no answer.

### 3. One read-by-stamp primitive, no object

The principle: the smallest thing that removes the copies. `fileio`
gains `Kept`, a read of one file kept by its stamp, read once however
many ask, and each reader wraps its parse in one: the transcript, a
plan, the loudness.

It is small, and every read stays the function it is. But there is no
place that is the video: three caches, each with its own key and its
own count of what it keeps, which is what #607 called out in the
package variables. The questions that span files, `Status` reads the
transcript, the plans and the loudness, still gather them by hand in
every caller. It is the right mechanism and the wrong home.

### 4. The app holds it, beside what is chosen

The principle: the app already keeps a record for each video, its undo
steps and, since #606, `picked`. The transcript and the plans would sit
beside them.

It is close to where they are used. But it is the second holder #608
is about: the engine's `Status` still reads for itself, and the command
line gets nothing. It breaks "one engine, two front ends", and a rule
about when a file is read again would live in the app.

### 5. A snapshot made on each change, every read served from it

The principle: build the whole state of a video once, as `reconcile`
hears a change, as a value no one changes, and answer every question
from the newest one. A reader never meets a half read.

Every answer is consistent and costs nothing. But one plan changed
builds everything again, the four hour transcript too, unless the
building keeps each part by its stamp, which is simulation 1 again
under it. Shaping a clip needs the transcript's words, not a view of
them. And the snapshot lives where the watch does, in the app, with
the faults of simulation 2.

## What converges

Four of the five keep each file's read with the stamp it was read at:
1, 3, and 5 under its snapshot, and 2 only as long as no hint is lost.
Three put the holder in the engine: 1, 3 and 2. The ones that put it in
the app, 4 and 5, put a rule about the files above the engine and leave
the command line out. Only 2 trusts something other than the disk, and
only 5 gives a reader a consistent view across files, which no caller
needs today: a plan and the transcript are read for different
questions.

From 3 comes how a file is read: once however many ask, a failure not
kept, the value handed out shared and read only. From 2 comes that a
hint from the watch may drop a copy early, which is a later change and
not needed for an answer to be right.

## The decision

Simulation 1, with 3's way of reading inside it.

`Machine.Episode(source)` gives the one `Episode` of a video, made the
first time it is asked for. An episode holds what it read of each file
and the stamp it read it at, and on every question looks at the stamps
and reads again only what changed, once however many ask at once. What
it hands out is shared and only read. It keeps the transcripts of the
ten videos asked about last, as both caches do today.

```
app bindings, command line   ask the episode of a video
engine.Episode               one per video, what it read and the stamps
the parse of each file       transcript, plan, loudness, as today
engine/fileio, the files     stat and read
```

The pull requests, in order, each one self-contained:

1. This record, and `Episode` with the transcript: `heardKept` and the
   app's `keptReads` go, and `Status` and the app read the transcript
   through it.
2. The plans: `PlanSummaries`, `ReadPlan` and the plan reads of the
   shaping go through the episode, by each plan's stamp.
3. `Status` asked of the episode, and the app's `project()` and
   `NewProject(nil, ...)` go. Closes #608.

## What follows

- It costs three pull requests, each about one kind of file.
- It buys one holder of what a video's files said, in the engine, for
  both front ends, read again whoever changed the file, and a transcript
  read once where it is read twice.
- It was wrong if a `Stat` per question shows in a measurement, or if a
  writer changes a file without changing its size or time, which a sync
  service restoring an older copy could. Then the stamp takes the
  file's contents, a hash, in place of its time.
