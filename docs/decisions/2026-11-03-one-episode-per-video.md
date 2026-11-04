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

Why not `engine.Project`, which is already "the episode at path" and
already has `Transcript`? A `Project` is made for each call, with one
set of options and one `Engine`, and lives as long as the step it takes.
The app makes one for every binding. What holds a read has to live as
long as the program and take the speech model and the floor with each
question, so it hangs off the `Machine`, and a `Project` asks its
`Episode` rather than being it.

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

Layers: the watch in the app over a cache in the app over the files.
What crosses: `reconcile` hands the cache the paths it heard. Next
changes: the command line and every test would each need a watch of
their own, or a second way in. Cost: no stat per question, and a wrong
answer for as long as a lost hint goes unnoticed.

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

Layers: each reader over `fileio.Kept` over the files. What crosses:
`fileio.Kept(fsys, path, parse)`. Next changes: each new kind of file
picks its own key and its own cap, and the status still gathers them by
hand. Cost: the same stat per question as simulation 1, and three caps
to keep in step.

### 4. The app holds it, beside what is chosen

The principle: the app already keeps a record for each video, its undo
steps and, since #606, `picked`. The transcript and the plans would sit
beside them.

It is close to where they are used. But it is the second holder #608
is about: the engine's `Status` still reads for itself, and the command
line gets nothing. It breaks "one engine, two front ends", and a rule
about when a file is read again would live in the app.

Layers: the app's record of a video over the engine's reads. What
crosses: nothing new in the engine, a field on the app's record. Next
changes: the command line would need its own copy, which is the drift
"one engine, two front ends" forbids. Cost: the same as simulation 1 in
the app, and the engine's `Status` still reads twice.

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

Layers: `reconcile` over a builder over the files. What crosses: a
`Snapshot` value per video. Next changes: every new kind of file is a
field the builder fills, and a question that needs a fresh file between
two hints has no way to ask. Cost: a whole build per change, or
simulation 1 under it.

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
2. The plans: `PlanSummaries` and `ReadPlan` go through the episode,
   by each plan's stamp. The plan reads of the shaping move with part 4,
   see Parts 2 and 3 below.
3. `Status` asked of the episode, and the app's `project()` and
   `NewProject(nil, ...)` go.
4. The paths of a video's own files checked by its episode, opened
   through `os.Root` on its work folder, the third thing #608 asks for,
   and the shaping's plan reads with them. Closes #608.

## Parts 2 and 3: what the app asks in place of a Project

Tim asked on 4 November for parts 2 and 3 in one pull request. Part 2
carries out what the decision above holds: the plans are read through
the episode, each by its own stamp. Part 3 takes the app's `project()`
away, and the decision did not say what the app asks instead. Seven
calls make a `Project` with no `Engine` only to ask it something, and
what they ask is counted from the code: `LogsDir` once, the transcript
through it three times, and `RoomWith`, `Limits`, `PlaceWindow` and
`WindowAfter` for the window. Of the last four, `Limits` and
`WindowAfter` read nothing but the options and the room they are handed,
`RoomWith` reads the search's room from the logs folder, and
`PlaceWindow` reads the plans. So the question is where a rule that
needs the settings and a video's files lives, once nothing makes a
`Project` to hold the settings.

### 1. Who may know about whom: the episode answers, the settings are handed in

The app knows the `Machine` and asks `Episode`. What reads the files is
the episode's, with the options handed in: `Episode.Room(o)` for the
room and the lines weighed, `Episode.PlaceWindow(o, r, duration)` for
the window placed from the plans. What reads no file is the options':
`Options.Limits` and `Options.WindowAfter`. The app reaches the engine
through the `Machine` and its episodes for questions, and through
`Project` for steps, which is what the drawings already say.
`Project.PlaceWindow` stays for the command line and asks the episode of
its engine. What crosses: four methods, two on `Episode` and two on
`Options`, and `project()` and `projectWith` go. Cost: one pull request,
about ten files.

### 2. What changes together: one Project per video, kept by the machine

`Machine.Project(source, o)` gives a `Project` made from the machine,
with its episode, and the app asks it as it does today. The four window
rules stay on `Project`. But a `Project` is made for one set of options,
and the settings change under the app, so either the machine keeps one
per video and per set of options, or the app makes one per call as it
does now. The first is a cache keyed by options, a second holder beside
the episode. The second changes only who calls `NewProject`, and leaves
#608's complaint, a `Project` made to find a path, where it was. Dropped.

### 3. Where state lives: the episode keeps the answers too

The episode holds the room, the limits and the placed window, each by
the stamps of what it read and the options asked with. Every state of a
video asks for them, about four times a second while it plays, and the
lines are already weighed once for each read of the transcript, 0.24 ms
for four hours. What is left, the room from a small file and the plans
already held, costs microseconds. A key made of the options is a second
thing to keep right. Dropped for now. It is the step to take if a
measurement shows the window in a profile.

### 4. What runs together: one answer for everything a state needs

One `Episode.View(o, duration)` gives the room, the limits, the placed
window and the plans together, since `windowFor` asks for them together.
It saves no read, because each is held already, and it ties the window's
rules to the shape of `VideoState`, which belongs to the app. The next
field the app needs changes the engine. Dropped.

### 5. What a test has to build: rules over values, reads apart

Every rule is a function of values: `PlaceWindow(plans, o, r, duration)`
over the plans handed in, `Limits` and `WindowAfter` over the options and
the room. The app reads the plans and the room from the episode and
hands them on. A test of a rule builds values and no file system. That
is how `placeWindow` and `moveWindowOn` are tested today, under the
methods, and it stays so. But the app then knows which reads each rule
needs and in what order, a little of the engine's knowledge above it.

### What converges

1 and 5 agree that what reads no file belongs to the options, and 1, 2
and 3 that what reads the files is asked of the episode. 5's rules over
values are already there under the methods, and stay the test's way in.
So: simulation 1. `project()` and `projectWith` go, `Episode.Room` and
`Episode.PlaceWindow` read through the episode, `Options.Limits` and
`Options.WindowAfter` read nothing, and `Project.Limits` and
`Project.WindowAfter` go with their last callers in the app.
`Project.PlaceWindow`, `Project.RoomAndLines` and the `Project.RoomWith`
under it stay for the command line, and the first asks the episode.

```
app bindings                 ask Machine.Episode, hand in the options
engine.Episode               plans, transcript, status, room, placed window
engine.Options               limits, the window after a search
the parse of each file       as today
```

The plan reads of the shaping, `planClip` and the ordered read beside it
in `ShapeClip`, stay as they are in this pull request and move with part
4. Each reads the plan whole and a gesture changes it, so the episode
would have to hand out a plan the shaping copies before it changes it.
Part 4 opens every file of the video through the episode anyway, which
is where that copy belongs.

## What follows

- It costs four pull requests, three about one kind of file each and
  one about the paths.
- It buys one holder of what a video's files said, in the engine, for
  both front ends, read again whoever changed the file, and a transcript
  read once where it is read twice.
- It was wrong if a `Stat` per question shows in a measurement, or if a
  file changes without its size or time changing. A copy restored by a
  sync service has an older time and is seen. A volume that keeps times
  coarsely, HFS+ to the second or FAT to two, would hide two saves of
  the same size within one tick, and APFS keeps them to the nanosecond.
  Then the stamp takes the file's inode and change time too, or a hash
  of its contents.
- What the library lists of every video, whether its transcript is done,
  what it heard and where its lines start and end, is held for every
  video and by the same stamp, so listing more videos than transcripts
  are held lets go of none. "No transcript yet" is an answer kept by its
  stamp like a transcript, not a failure read again on every question.
