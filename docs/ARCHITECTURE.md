# Architecture

Where each kind of thing belongs in Frame Fairy, and what must stay true.
Read it before a change, and put the change where it says. It says
nothing about how a part works inside, that is the part's own doc. Its
shape is [matklad's ARCHITECTURE.md](https://matklad.github.io/2021/02/06/ARCHITECTURE.md.html),
short and only about what rarely changes.

A line is added only for a new kind of thing: a new part, a new home for
state, a new boundary. A pull request that closes an issue this file
names rewrites the rows that name it and removes its line under Not to
copy. `internal/naming` checks that every path the file names exists.

## Bird's eye view

A part may use the parts under it and knows nothing of the parts over
it. That direction is the architecture: it decides where a piece of code
goes. A rule about a video belongs as low as it can go, in the engine,
where both front ends reach it. A rule found higher up, in the app or the
interface, is in the wrong place.

```
  1  the interface        frontend/                      the command line
            |  the bindings, events, /frames             cmd/framefairy/
  2  the app's Go side    cmd/framefairy-app/                  |
            |                                                  |
            +------------------------+-------------------------+
                                     |   both hand in the speech model, asr/
  3  the engine           engine/, reached through engine.Project
            |
  4  at the edge          work folders, models, ffmpeg, llama-server,
                          the frames decoder cmd/framefairy-frames/
```

Inside the engine, a video goes through the same steps in both front
ends: it is heard into a transcript, searched into a plan of clips,
shaped by a person's gestures, and rendered into shorts. An edit takes
one path through the layers: the interface asks, a binding passes it on,
the engine works out the answer and saves it through `editPlan`, and Go
tells the interface what changed.

## Code map

One row for each box above, in the same order.

| Layer | Code | Owns | Never holds |
| --- | --- | --- | --- |
| 1 the interface | `frontend/src/screens/`, `frontend/src/components/` | layout and wiring | logic worth a test, which goes in `frontend/src/lib/` |
| 1 | `frontend/src/lib/` | what the interface keeps and works out, tested with Vitest. `frontend/src/lib/frames/` is the frame queue the video preview plays from | a rule of the engine, except the few held to Go by a shared `.cases.json` |
| 1 | `frontend/preview/` | the walks in `frontend/preview/walks/`, which do what a person does against the real Go side, and the harness they run in | |
| 1 the command line | `cmd/framefairy/` | flags into `engine.Run`, and exit codes | a rule of its own |
| 2 the app's Go side | `cmd/framefairy-app/` | the bindings, the job queue and its lanes, the library and the settings, updates, the log | a rule of the engine. A binding asks the engine and passes the answer on |
| 2 | `updates/`, `notices/` | finding, checking and fetching a newer build, and the licences of what the app is made of | |
| 2 | `asr/` | the speech recogniser. It implements `engine.Recognizer`, and each front end hands it to the engine | |
| 3 the engine | `engine/` | every rule about a video: words, clips, edges, windows, captions, crops, rendering, the models, the files of a work folder | anything about the screen, a binding or a click |
| 3 | `engine/timed/` | the types every part of the engine uses for a moment of a video: a piece of text with its times, a span, a window, a loudness reading, the 10 ms grid of the sound | a function or a rule, and a type only one part uses |
| 3 | `engine.Project` | the steps one video takes, the same for both front ends | a second way to run a step |
| 3 | `licence/`, `internal/wake/` | small libraries under the engine: the format of a licence key, and how a waiter hears that something changed | |
| 4 at the edge | `cmd/framefairy-frames/`, `internal/framewire/` | the decoder that turns a video into the frames, sound and captions of the video preview, and the wire to it | anything but decoding. It is a child process |

Outside the picture, not shipped in the app: `train/` and
`cmd/framefairy-train/` read the training records, `cmd/framefairy-release/`
and `licence/signer/` sign builds and keys, `licence/dispenser/` is the
licence server, and the rest of `internal/` holds the checks and
stand-ins the tests share.

## Where state lives

Every piece of state has one home and one writer. A second copy is a bug
waiting for the day the two disagree.

| State | Home | Writer |
| --- | --- | --- |
| everything about a video | its work folder, `<video>.framefairy/` | the engine |
| what was last chosen in a video, its clip and window | `chosen.json` in its work folder | the app's Go side |
| the library and the settings | the app's config folder | the app's Go side |
| the models, notes about running servers, the training records | `~/.framefairy/` | the engine |
| what runs now | the job queue, in memory | the app's Go side |
| what is on screen | the interface's stores | from what Go sends. Today `Episode.svelte` also works out a video's state from what changed, see Not to copy |

## The programs that run

One app process holds the interface in the webview, WebKit on a Mac, and
the Go side, with the speech model in the same process. A panic anywhere
in the Go side ends all of it. The engine starts the children that do a
video's work: one frames decoder for each video in the video preview,
ffmpeg and ffprobe for a render, a measure or a probe, and llama-server
for the local model, kept loaded. The app's Go side starts a few of its
own, for a recording, a report and showing a file in Finder. A cloud
model is reached with the person's own key.

## Where does it go?

What to do today, and what changes once the issue named is built.

| I need to | It goes |
| --- | --- |
| decide where an edge, a word or a window lands | the engine: `engine/shape.go` for a gesture, `engine.Project` for a window. The interface asks and draws the answer |
| show the interface a new fact about a video | a field on what the workspace already reads for the video, its status or its clip views, never a new event of its own or a poll. Once #606 is built, on the snapshot Go sends |
| keep something between two jobs, a cache or a fact about the machine | today the caches that outlive a job are package variables, the way `engine/probed.go` keeps the probes. A new one says why in its pull request, and never comes from a new `Engine` made only to get it. #607 moves them all onto one object a program keeps |
| read a file of a video | through the `afero.Fs` the code was given, with `engine.SafeChildIn` for a path in a work folder. Not `os`. Once #608 is built, through the video's `Episode` |
| change a plan | `editPlan` in `engine/edit.go` |
| start a goroutine that runs code which can panic, like layout of words a person typed | a deferred recover that turns a panic into that work's error and logs the stack, as `engine/planbuild.go` does. #604 adds it where it is missing |
| say that something went wrong | an error that keeps its kind with `%w`, returned to the caller. The app turns it into a row or a toast, and logs through `theLog`, never `log.Printf` |
| add a call between the interface and Go | a method on `*FrameFairy` in the file of its kind in `cmd/framefairy-app/`, and its call in `frontend/src/lib/api.ts`. `bindings_test.go` checks the name and the arguments |
| give the interface a new type or field | the Go struct and its interface in `frontend/src/lib/api.ts` in the same change. Once #611 is built, generated from the Go struct |
| add a setting | a field of `Settings` in `cmd/framefairy-app/settings.go`, changed through `UpdateSettings`. What the engine needs from it reaches it through its options |
| add a kind of work to the queue | a kind in `engine/jobs.go`, started with `addSteps` in `cmd/framefairy-app/jobs.go`, so it gets the lanes, the recover and the record every job has |
| copy a rule of the engine into TypeScript | don't. If the interface must answer before Go does, the copy and a `.cases.json` both sides read go in together |
| add a check to CI | a make target, and its rule in `scripts/changed.sh` with its case in `scripts/changed-test.sh`, see BUILD.md |
| fake something in a test | the clock, the disk and the models are handed in. Use the fakes there are, in `engine/project_test.go` and `internal/standin/` |

## Invariants

What must stay true, and what holds it: a check in CI, or the review,
which reads this file.

1. The engine knows nothing of the layers over it. `engine/` imports
   nothing from `cmd/`, which Go refuses anyway, nor from `asr/`, and
   `engine/timed/` nothing of the rest of the engine. Held by Go's
   imports.
2. One engine, two front ends. A rule lives once, in the engine. Held by
   the review.
3. One word model. What a video says is `Transcript.Words`, and what a
   clip shows is made from it. A clip keeps no words, see
   [WORDS.md](WORDS.md). Held by the review.
4. One drawing of the captions, by libass, from `CaptionTrack`, see
   [CAPTIONS.md](CAPTIONS.md). Held by the review.
5. Go owns a video's state, and the interface shows it. Held by the
   review, and by its design once #606 is built.
6. A job that goes wrong fails itself, not the app. Held by the recover
   around every job, and around the goroutines #604 names once it is built.
7. Nothing is learned twice. What outlives a job is kept, not worked out
   again for each job. Held by the review, and by the design once #607
   is built.
8. Every path is judged by where it really leads, inside the library.
   Held by `SafeChild` and the store's `Known`, and their fuzz targets.
9. What comes from outside is read by code made for it, never parsed on
   the way: a model answer by the stream readers in `engine/stream.go`,
   `ExtractJSONObject` and `ValidatePlan`, a plan by `LoadClips`, a
   transcript and a job's record by their readers in `engine/transcript.go`
   and `engine/jobs.go`, a path by `SafeChild`. Held by their fuzz
   targets.
10. A test waits on a signal, never on the clock, and stays off the disk.
    Held by forbidigo in `make lint` and by `internal/offdisk`.

## Boundaries

Where one layer meets another. A change to one of these changes what the
other side relies on, and the pull request says so.

- The bindings on `*FrameFairy` and `frontend/src/lib/api.ts`.
- `engine.Project`, the steps a front end may take.
- The files of a work folder and of the config folder, which an older
  and a newer build both read, `chosen.json` and the settings among
  them. A file the app writes whole keeps the keys it does not know,
  see `cmd/framefairy-app/appfiles.go`, and a new shape goes under a new
  name, because an older build sets aside a file it cannot read.
- `internal/framewire`, between the decoder and the engine.
- The update feed and the licence key, which the builds of customers
  read.

## Not to copy

Code that breaks this map today, with the issue that fixes it. A session
copies what it finds nearby, so these are not the pattern for new code.

- The search window placed in `cmd/framefairy-app/window.go`, #610.
- Rules of the engine in `frontend/src/lib/flow.ts` and
  `frontend/src/lib/rotate.ts`, #610.
- The transcript cache in `cmd/framefairy-app/words.go`, #608.
- A new `engine.NewEngine` for each job or probe, #607.
- The effects in `frontend/src/screens/Episode.svelte` that compare an
  old value with a new one to notice that work ended, #606.
