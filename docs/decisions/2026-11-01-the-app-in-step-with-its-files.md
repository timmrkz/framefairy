# The app in step with its files

A decision record, see `docs/decisions/README.md`. Written 1 November 2026, on main at 93a2ef6, before any code. It goes into the repository with the first code of the change.

## The problem

Tim, 1 November 2026, in the thread on #608:

> The source of truth isn't Go. The source of truth are the files on the file system. And the files on the file system can potentially be written by many writers. We already had an issue where I was deleting files and folders in the file system, and the app did not recognize that and misbehaved. And then we had a bunch of bugs and issues that we had to fix, and now we worked around them.

> Why wouldn't you see those types of synchronization problems between disk, process, front end, and user as CRDT-based problem?

### What was measured on the code as it is

**Who writes a video's files.** Everything about a video is in its work folder, `<video>.framefairy/`, beside the video. The list of videos is `library.json` in the app's config folder. The writers:

| Writer | What it writes | Takes our lock |
| --- | --- | --- |
| the app's Go side, for a person's edits | the plans `logs/clips*.json` through `editPlan`, `label.json`, `chosen.json`, `added.json` | yes, for plans and transcripts |
| the app's Go side, for jobs | the transcript `logs/words*.json`, the loudness `logs/levels.*`, a new plan per search, the job records in `jobs/`, the shorts in `out/`, the captions, the training records | yes, for plans and transcripts |
| the command line, on the same video | the same files | yes, the same lock |
| the person, in Finder or a terminal | deletes, moves, renames or replaces the video, its work folder, or the models | no |
| a sync service, iCloud Drive or Dropbox, when the folder is in a synced place | any file, from another machine, and conflict copies such as `clips 2.json` | no |

Tim's videos live in `~/Documents`, which iCloud syncs when Desktop and Documents is turned on. Whether it is on for him is not known.

**How our own programs write.** `fileio.Lock` takes a lock inside the program and an `flock` on `.<file>.lock` across programs, so the app and the command line edit one file at a time. 6 places take it, in 3 files. `fileio.Replace` writes beside the file, flushes it with `F_FULLFSYNC`, renames it over the old one and flushes the folder, so a reader sees all of the old or all of the new. 21 writes go through it or `replacePlan`, in 14 files. Every plan edit adds one to the plan's `revision`. This is the lock, rename and flush that git and SQLite use themselves. It is sound for the writers that take the lock. Finder, a terminal and a sync service do not take it.

**How the app notices a change it did not make.** It does not watch the disk. There is no FSEvents and no fsnotify anywhere. `letGoOfRemoved` in `cmd/framefairy-app/library.go` looks for videos whose work folder is gone, and it runs only when the library is listed (as the app starts and when it comes back to the front) and when the app asks which video was open last (`Opened` in `chosen.go`). A change to a plan or a transcript made by the command line while the app is open reaches the interface only when something else makes Go read the video again.

**What Go holds between questions.** The engine keeps what it read of a transcript, `heardKept` in `engine/heard.go`, since #663. The app keeps the whole transcript of the last few videos, `transcript` in `cmd/framefairy-app/words.go`. Both are keyed by the file's size and time. Plans are read again for every question: `LoadClips` is called from 11 places in 10 files, `PlanSummaries` from 7 in 5. #608 asks for one holder per video instead.

**How the interface hears.** Only through `VideoState`, sent by `told(path)` or the job queue, at most four times a second, always after the last change. `told` is called from 3 places. The interface shows the newest state and works nothing out for itself, as #606 makes it.

**What went wrong, in Tim's own use** (his 102 report logs, 10 to 30 October):

- 13 October: a work folder deleted in Finder, the video stayed in the list, and opening it wrote a folder back (plan row 2.247). Then a video deleted with its folder stayed with a red lamp, and the app opened on it after an update with "this file does not belong to a video in the library" (2.250). 8 such lines and 10 "No such file" lines that day.
- 20 and 21 October: `start.mp4` was no longer where the library had it, 9 lines, shown as File not found with a toast.
- 29 October: a language model file removed from `~/.framefairy/models`, found only when the model was next loaded.

Each was fixed for its own case: a mark file `added.json` that tells a deleted folder from one never made, a guess about whether the video's own folder is there (`deletedOutside`), a list of the files the app writes back (`writtenBack`), and letting go first in `Opened`. That is the pattern Tim names. Nothing in the app says "the disk changed, look again".

No sighting in the logs is of two writers changing the same file at the same moment.

## Five simulations

Each starts from a principle of its own and is worked through on the code as it is.

### 1. Merge every writer's copy: a CRDT

**Principle: where state lives.** Every writer keeps its own copy of a document, and copies merge without anybody waiting, the way Automerge and Yjs work.

- **Layers.** The plan becomes an Automerge document, a binary file of changes. The engine applies an edit to it and saves it. Every other writer, the command line or a second machine through a sync service, merges what it finds into its copy.
- **What moves.** `editPlan` (16 calls in 5 files) becomes applying a change. `LoadClips` (11 calls in 10 files) reads the merged document and then checks it, because a merged plan is as untrusted as a written one. The fuzz targets on the plan's JSON get a new input. The command line and `framefairy-train` read the new format.
- **Names that cross.** The Automerge document type into the engine's plan code, and the library underneath it: `automerge-go` wraps the Rust core through cgo, and has no tagged release, its last version being from 30 October 2024. It has to build for macOS, Windows and Linux.
- **What it solves.** Two writers changing one plan at the same moment, with neither edit lost. Today our programs never do that, because they wait for each other's lock. Only a writer that ignores the lock would: a second machine through a sync service, or a person editing a plan by hand.
- **What it does not solve.** A video, a folder or a model deleted, moved or replaced. There is nothing to merge, and the app still has to notice. Every sighting in Tim's logs is of this kind. It needs simulation 2 or 3 beside it.
- **Rules it meets.** "Plain files, no database" in CLAUDE.md: the plan stops being plain JSON. "Untrusted input": the checks in `LoadClips` stay and run after every merge.
- **Next changes.** Every new field of a plan needs a merge rule. Two crops set at once on two machines still end with one of them, so most fields would be last writer wins anyway.
- **Cost.** The plan format and everything that reads it, about 8 to 10 pull requests, and a native library in three builds.

### 2. Notice every change and look again: watch and reconcile

**Principle: what runs together.** One loop that runs beside the app's work: whatever changed on the disk, and whoever changed it, it compares what is there with what Go holds and brings the rest in step. A change is a hint to look, never the news itself, the way a Kubernetes controller reconciles: what it acts on is the state it reads, so a missed or doubled hint does no harm.

- **Layers.** The disk is the truth. The engine holds one `Episode` per video, the parsed plans, status and transcript by their file stamps, and reads a file again only when its stamp changed (#608). The app's Go side holds a watcher, and a `reconcile(path)` that reads the video's `Episode` again, lets the video go when its folder is gone, says File not found when the video is, and calls `told(path)`. The interface shows the newest `VideoState`, as now.
- **What moves.** `letGoOfRemoved` becomes part of `reconcile`, which runs as the app starts, when it comes back to the front, and on every hint from the watcher. The mark `added.json` stays, because it is how a deleted folder is told from one never made. `writtenBack` goes once nothing writes into a folder before its video is reconciled. The app's own transcript cache goes into the `Episode`.
- **Names that cross.** A `Watcher` handed to the app's Go side: add a folder, remove a folder, and a channel of paths that changed. A test hands in its own and sends the path itself, so no test waits on the clock. `reconcile(path)` and `told(path)`, which already exists. `Episode` in the engine, with what changed since the last look.
- **The watcher.** fsnotify v1.10.1 uses kqueue on macOS: one file descriptor per watched file and folder, and no watching of folders inside folders. FSEvents, which watches a whole tree cheaply, is not in fsnotify yet, and is reached from Go only through cgo. So each video watches a few folders: the one its video is in, its work folder, `logs/` and `jobs/`. How many descriptors that takes on Tim's folders is measured before choosing between the two.
- **Our own writes.** They come back as hints too. Reading by stamp makes that cheap, 0.1 ms for a plan, and `videoNews` already sends at most four states a second.
- **What it solves.** Every sighting in Tim's logs, in one place, for every writer, and a plan changed by the command line shows in the app at once.
- **What it does not solve.** Two writers that ignore the lock writing one file at the same moment: the later write wins, as it does today. A sync service's conflict copy, `clips 2.json`, would be listed as another search, because `PlanSummaries` reads `clips*.json`. `reconcile` can tell such a name and say so.
- **Cost.** 3 or 4 pull requests: the watcher and `reconcile` in place of `letGoOfRemoved`, the `Episode` holder, the models folder.

### 3. Ask the system who changed what: file coordination

**Principle: who may know about whom.** macOS already knows every writer that coordinates. A `NSFilePresenter` for each work folder hears moves, deletions and changes made by others, and every write of ours goes through `NSFileCoordinator`, so Finder and iCloud hear ours. `NSFileVersion` hands over iCloud's conflict versions. This is how document apps on a Mac work.

- **Layers.** A layer under `engine/fileio` on macOS that coordinates every write, and a presenter in the app's Go side that calls `reconcile`.
- **What moves.** All 21 writes, in 14 files, go through coordination, in the engine, so the command line coordinates too.
- **Names that cross.** Objective-C through cgo, under `fileio` and in the app.
- **What it solves.** Changes by Finder and iCloud, which coordinate (from Apple's own description, not tested here), and iCloud's conflict versions as such.
- **What it does not solve.** A writer that does not coordinate, a terminal or a script. Nothing on Windows or Linux, which have no such thing and would each need their own watcher anyway.
- **Cost.** High, and three ways of doing one thing once Windows and Linux come.

### 4. Write together what changes together: one database

**Principle: what changes together.** A video's plans, transcript and jobs change together, so they are written together, in one transaction, in one SQLite file in the work folder, as SQLite's "application file format" suggests.

- **Layers.** The engine reads and writes tables instead of files. SQLite's own locks replace `fileio.Lock`.
- **What moves.** Every reader and writer of a work folder: the 21 writes, the 11 `LoadClips`, the transcript readers, the job records, the command line, `framefairy-train`.
- **What it solves.** A change across several files at once. None of the sightings is of that kind, because each file is already replaced whole.
- **What it does not solve.** Finder and sync services: SQLite warns against keeping a database on a file system shared over a network, and a sync service copying an open database can break it. A deleted folder still needs noticing.
- **Rules it meets.** "Plain files, no database" in CLAUDE.md, which Tim chose.
- **Cost.** The largest of the five.

### 5. Keep every change in order: an event log

**Principle: what a test has to build.** A test builds a list of changes and reads the state out of it. Each video keeps `logs/events.jsonl`, every change appended under the lock, and the plans are worked out from it. Undo and the training records already keep histories of their own.

- **What moves.** `editPlan`, `undo.go` and the training export read and write the log.
- **What it solves.** Two machines' logs can be put together, which is what a CRDT does under the hood.
- **What it does not solve.** Changes nobody logged, Finder's among them.
- **Cost.** High: undo, edits and training change, and the log grows with every edit.

## What converges

- **In all five, something has to notice changes our programs did not make.** Only 2 and 3 do. 1, 4 and 5 need one of them beside them. Every sighting in Tim's logs is this.
- **In four of the five (2, 3, 4, 5), Go holds one view of a video that knows what changed:** #608's `Episode`, a database, or a log read into a state. In 1 it is the merged document.
- **In all five, the interface hears through `VideoState` and `told`,** and nothing about it changes. "Go owns a video's state" is read as: Go is the only one that reads the disk for the interface, and the disk is the truth.
- **Only 1 and 5 merge two writers at once.** No such writer is in the logs: our programs wait for each other's lock, and Tim uses one Mac. Kept for later, for a reason said below.
- **Only 3 is for one system. Only 4 writes files together.** Dropped.

## The decision

Simulation 2: watch and reconcile, with #608's `Episode` as what Go holds. The lock, the rename and the flush of `editPlan` stay as they are.

```
interface        EpisodeSession shows the newest VideoState               (unchanged)
                          ^ told(path), at most 4 a second
app's Go side    Watcher --> reconcile(path): look again, let go, File not found
                          | reads
engine           Episode per video: plans, status, transcript by file stamp
                 editPlan, fileio.Lock, fileio.Replace                    (unchanged)
                          | reads and writes
disk             the work folder, the video, the models: the truth, many writers
```

Each layer uses only the ones under it. The watcher is in the app's Go side, because the command line runs once and has nothing open to keep in step.

## What follows

- **It costs** six pull requests of the usual size, the steps of #673 in order: (1) this record and `reconcile` in place of `letGoOfRemoved`, run as the app starts, as it comes to the front and on every change heard, (2) the watch on the folders of every video, in the same pull request as step 1, (3) the `Episode` holder, held by the files' stamps, which closes #608, (4) the shorts folder watched, (5) the models folder watched, (6) the special cases going, `writtenBack` and the early let-go in `Opened`.
- **It buys** one place where every change from outside lands, whoever made it, instead of a fix per case. The special cases `writtenBack` and the early let-go in `Opened` go.
- **`docs/ARCHITECTURE.md` changes:** the State table names the outside writers and the `Episode`, Where does it go gets "notice a change made outside the app: `reconcile`", and invariant 5 says the disk is the truth and Go is its one reader for the interface.
- **It would be wrong if** watching the folders takes more file descriptors than a Mac allows on Tim's folders (then FSEvents through cgo), or if a sync service ever loses an edit or makes a conflict copy in Tim's use. That is the day to take simulation 1 up again, with Automerge for the plan.
