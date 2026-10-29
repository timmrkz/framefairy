# Architecture

Where each kind of thing belongs in Frame Fairy, and what must stay true.
Read it before a change, and put the change where it says. When a change
has no place here, its place is decided first, in the pull request, and
this file gains the line. It says nothing about how a part works inside,
that is the part's own doc.

The shape is [matklad's ARCHITECTURE.md](https://matklad.github.io/2021/02/06/ARCHITECTURE.md.html),
which rust-analyzer and many other projects keep: short, and only about
what rarely changes. The five views it was chosen from, and why this one,
are in #603.

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
                                     |
  3  the engine           engine/, reached through engine.Project
            |
  4  at the edge          work folders, models, ffmpeg, llama-server,
                          the frames decoder cmd/framefairy-frames/
```

Inside the engine, a video goes through the same steps in both front
ends: it is heard into a transcript, searched into a plan of clips,
shaped by a person's gestures, and rendered into shorts. An edit takes
one path through all four layers: the interface asks, a binding passes
it on, the engine works out the answer and saves it through `editPlan`,
and Go tells the interface what changed.

## Code map

One row for each box above, in the same order.

| Layer | Code | Owns | Never holds |
| --- | --- | --- | --- |
| 1 the interface | `frontend/src/screens/`, `components/` | layout and wiring | logic worth a test, which goes in `lib/` |
| 1 | `frontend/src/lib/` | what the interface keeps and works out, tested with Vitest | a rule of the engine, except the few held to Go by a shared `.cases.json` |
| 1 | `frontend/preview/walks/` | the walks, which do what a person does against the real Go side | |
| 1 the command line | `cmd/framefairy/` | flags into `engine.Run`, and exit codes | a rule of its own |
| 2 the app's Go side | `cmd/framefairy-app/` | the bindings, the job queue and its lanes, the library and the settings, updates, the log | a rule of the engine. A binding asks the engine and passes the answer on |
| 2 | `updates/` | finding, checking and fetching a newer build | |
| 3 the engine | `engine/` | every rule about a video: words, clips, edges, windows, captions, crops, rendering, the models, the files of a work folder | anything about the screen, a binding or a click |
| 3 | `engine.Project` | the steps one video takes, the same for both front ends | a second way to run a step |
| 3 | `asr/`, `licence/` | the speech recogniser, the format of a licence key | |
| 4 at the edge | `cmd/framefairy-frames/`, `internal/framewire/` | the decoder that turns a video into the frames, sound and captions of the video preview, and the wire to it | anything but decoding. It is a child process |

Outside the picture, not shipped in the app: `train/` and
`cmd/framefairy-train/` read the training records, `cmd/framefairy-release/`
and `cmd/framefairy-signer/` sign builds, `licence/dispenser/` is the
licence server, and `internal/` holds the checks and stand-ins the tests
share.

## Where state lives

Every piece of state has one home and one writer. A second copy is a bug
waiting for the day the two disagree.

| State | Home | Writer |
| --- | --- | --- |
| everything about a video | its work folder, `<video>.framefairy/` | the engine |
| what was last chosen in a video, its clip and window | `chosen.json` in its work folder | the app's Go side |
| the library and the settings | the app's config folder | the app's Go side |
| the models, notes about running servers | `~/.framefairy/` | the engine |
| what runs now | the job queue, in memory | the app's Go side |
| what is on screen | the interface's stores | from what Go sends, never worked out |

## The programs that run

One app process holds the interface in the webview, WebKit on a Mac, and
the Go side, with the speech model in the same process. A panic anywhere
in the Go side ends all of it. The engine starts and stops every child:
one frames decoder for each video in the video preview, ffmpeg and
ffprobe for a render, a measure or a probe, and llama-server for the
local model, kept loaded. A cloud model is reached with the person's own
key.

## Where does it go?

| I need to | It goes |
| --- | --- |
| decide where an edge, a word or a window lands | the engine: `engine/shape.go` for a gesture, `engine.Project` for a window. The interface asks and draws the answer |
| show the interface a new fact about a video | on what Go sends about the video, see #606. Not a new event of its own, not a poll |
| keep something between two jobs, a cache or a fact about the machine | on the one object a program keeps for its life, see #607. Not a package variable, not a new `Engine` |
| read a file of a video | through the `afero.Fs` the code was given, and the video's `Episode` once #608 is built. Not `os`, not a path checked by hand |
| change a plan | `editPlan` in `engine/edit.go` |
| start a goroutine | through the helper that turns a panic into that work's error, see #604 |
| say that something went wrong | an error that keeps its kind with `%w`, returned to the caller. The app turns it into a row or a toast, and logs it through `theLog`, never `log.Printf`, see #605 |
| give the interface a new type | generated from the Go struct, see #611. Not typed again in `api.ts` |
| copy a rule of the engine into TypeScript | don't. If the interface must answer before Go does, the copy and a `.cases.json` both sides read go in together |
| fake something in a test | the clock, the disk and the models are handed in. Use the fakes there are, in `engine/project_test.go` and `internal/standin/` |

## Invariants

What must stay true, and what holds it: a check in CI, or the review,
which reads this file.

1. The engine knows nothing of the layers over it. `engine/` imports
   nothing from `cmd/`. Held by Go's imports.
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
   around every job, and around every goroutine once #604 is built.
7. Nothing is learned twice. What outlives a job lives on the program's
   object. Held by the review, and by the design once #607 is built.
8. Every path is judged by where it really leads, inside the library.
   Held by `SafeChild` and `PlanOf`.
9. Untrusted input goes through one door each, `ReadPlan`, `LoadClips`,
   `SafeChild`, `framewire.Read`. Held by the fuzz targets.
10. A test waits on a signal, never on the clock, and stays off the disk.
    Held by forbidigo in `make lint` and by `internal/offdisk`.

## Boundaries

Where one layer meets another. A change to one of these changes what the
other side relies on, and the pull request says so.

- The bindings on `*FrameFairy` and `frontend/src/lib/api.ts`.
- `engine.Project`, the steps a front end may take.
- The files of a work folder and of the config folder, which an older
  and a newer build both read, `chosen.json` and the settings among
  them, see #612.
- `internal/framewire`, between the decoder and the engine.
- The update feed and the licence key, which the builds of customers
  read.

## Not to copy

Code that breaks this map today, with the issue that fixes it. A session
copies what it finds nearby, so these are not the pattern for new code.
A line goes when its issue is closed.

- The search window placed in `cmd/framefairy-app/window.go`, #610.
- Rules of the engine in `frontend/src/lib/flow.ts` and `rotate.ts`, #610.
- The transcript cache in `cmd/framefairy-app/words.go`, #608.
- A new `engine.NewEngine` for each job or probe, and the package
  variables that hold caches and test seams, #607.
- The effects in `Episode.svelte` that compare an old value with a new
  one to notice that work ended, #606.
- The types typed by hand in `api.ts`, #611.
