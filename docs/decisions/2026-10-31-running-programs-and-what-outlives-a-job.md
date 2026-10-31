# Running programs, and what outlives a job

Step 3 of splitting the engine into packages, after
[the log of a run](2026-10-30-the-log-of-a-run.md), and the design of
#607. Decided on 31 October 2026, in the first pull request that builds
it. The step takes several pull requests, and each names this record.

## The problem

Tim said go on step 3 on 31 October. The plan for the split gave it
"files and running programs: `SafeChild`, `WriteAtomic`, running ffmpeg
and ffprobe, and the narrow parts `Engine` holds. The caches that outlive
a job move onto them, which closes #607." #607, from the architecture
review of 29 October, says the app makes a new `Engine` for every job and
probe, so what an engine learns is thrown away, and what had to be shared
became package variables instead. Tim, 29 October: objects made again
and again and thrown away are a design fault to fix.

Measured on main at 7634f17:

- `engine.NewEngine` is called in 10 places outside tests: 9 in the app,
  for every job, caption request, frame stream, kept model, loudness
  measure, media probe, recording and setup, and 1 in the command line.
- An `Engine` holds three kinds of thing. What belongs to one job: the
  log, what it spent on the cloud, how much a hearing set out to hear,
  the read-ahead gate. What belongs to the machine and is learned again
  by every engine: whether the system's decoder failed here, which
  decoder each video used, the encoders, the subtitle template, the face
  detector. And what was handed in: the paths of ffmpeg and ffprobe, the
  file system, the records, the switches of the command line.
- The engine has 107 package variables outside its tests and the app 33.
  Read one by one, about 19 of the engine's hold a cache or something
  learned, the probes, the caption tracks of the video preview, the
  measured captions, the model host, the keys, and about 24 are seams a
  test swaps, the stop of a program and the wait after it, the idle time
  of the model, the free memory, the clients and their stalls.
- Running a program is six names in `engine/ffmpeg.go`, `run`,
  `runBytes`, `command`, `finish`, `cutWriter` and `result`, used in 28
  places in 9 files, with three seams beside them, `leaveAfter`, `leaving`
  and `stopProcess`. Finding the programs, `FindTool`, `ToolPath`, the
  sums `make` writes into the build and the checks of each copy, is
  `engine/tools.go`, 312 lines. `shellQuote`, a command written out for a
  shell, is the last helper in `engine/util.go`, used 5 times. Four more
  runs of ffmpeg in the engine, which read its pipe as it goes, and the
  app's three for a screen recording called `os/exec` themselves, with an
  engine in hand.
- Two tests swap the stop seams and so cannot run beside the others,
  `TestASystemDecoderThatWillNotEndIsLeftBehind` and
  `TestAServerThatWillNotStopIsKilledAtOnce`, and `command` copies
  `stopProcess` because a stop once read the one a test had put back.

## Five simulations

### 1. Who may know about whom

Running a program knows nothing of episodes, plans or clips. It knows a
path, its arguments, a context and how to stop. So it sits at the
bottom, beside the log, under every area that runs ffmpeg, ffprobe,
llama-server or a program of the system. Finding and checking our own
copies of those programs is the same knowledge one step earlier, and it
goes with it. The files, `SafeChild`, `WriteAtomic`, the plan's lock,
know paths and an `afero.Fs` and nothing above, so they sit at the
bottom too, in a package of their own: a program run and a file written
are used by different areas and change for different reasons. What
outlives a job knows every area, the probes, the captions, the model, so
it cannot sit at the bottom. It sits at the top, in `engine`, and hands
the bottom parts down.

### 2. What changes together

The stop of a program and the wait after it came in together in #337,
for ffmpeg that went on sitting in VideoToolbox on CI's Mac after it was
killed, and the wait became a seam in #548, so a test need not wait for
it. llama-server's stop reads the same wait. The three seams are one
thing, how a program is stopped, and belong on one value. The caches change with the area they serve: the
probe cache with the probes, the caption tracks with the video preview.
So each cache moves with its area's code, onto the object that outlives
a job, as a field the area owns, not into one bag that every change
touches. The things an engine learns about the machine, the decoder,
the encoders, change with the media code and move as one.

### 3. Where state lives

One job's state lives with the job: the log, the money spent, the
hearing, the read-ahead. The machine's state lives once per program for
its whole life. Today both are on `Engine`, and the app throws the
second away nine times over. Three shapes keep them apart:

- `Engine` lives as long as the program, and every method takes the job
  it works for. That touches all 90 methods and every call of them.
- `Engine` stays one job's, and takes the object that outlives it. The
  methods keep their receiver, and what is learned is read through it.
- No object over the parts: each program hands every part to every
  engine. Ten calls of `NewEngine` then grow by every new part.

The second keeps the job's state where it is and gives the machine's a
home, so it wins here. Package variables go, because a package variable
is state that lives once per process whether a test wants it or not.

### 4. What Go and its libraries already have

`os/exec` already has the parts of a stop: `Cmd.Cancel` decides how a
program is stopped once its context is done, and `Cmd.WaitDelay` how
long to wait after it. But `WaitDelay` cannot leave a program behind.
Once it is up it kills the program and closes its pipes, and `Cmd.Wait`
still waits in `Process.Wait` until the program has ended, so an ffmpeg
the system will not let go of holds whoever waits for it just the same.
That is the case the stop exists for, and the reason not to trade
`Finish` for `WaitDelay` later. `WaitDelay` also waits on the clock with
no seam, so a test of a program that will not end would wait the five
seconds. So the stop stays ours, as fields: a `Stop` that `Cancel` calls and a
`Leave` that gives the wait, each a default when left empty, which is how
`http.Client` keeps its `Transport` and `CheckRedirect`. The object that
outlives a job is a plain struct made once in `main`, the way `net/http`
makes one `Server`: no container, no registry, no `sync.Once` global.
A dependency injection library would hand the same pointer down and
hide where it was made.

### 5. What a test has to build

A test that stops a program sets the stop on its own value and runs
beside the others with `t.Parallel()`, which is #607's last line. A test
of a probe or a caption track makes its own object that outlives a job,
so a cache filled by one test is never read by another. A test of
running a program builds nothing of the engine. Framing, step 6, runs
ffmpeg and needs only the part that runs it, so the part must sit under
framing by then.

## What converges

- Running programs goes to the bottom of the engine, finding them with
  it, `shellQuote` with it: 1, 2 and 5.
- The stop of a program is one value with fields, not three package
  variables: 2, 4 and 5.
- The files go to the bottom in a package of their own: 1, with 2 by how
  differently they change.
- `Engine` stays one job's and takes one object that outlives it, made
  once in each program's `main`: 3, with 4 for its shape.
- Every cache and seam moves from a package variable onto a field, each
  with its area: 2, 3 and 5.

Only simulation 3 weighed making `Engine` live as long as the program,
and only to set it aside for its cost.

## The decision

`engine/tool` finds, checks and runs the programs the engine calls:
`Path`, `Find`, `Inspect`, and a `Runner`, whose `Run`, `Bytes`,
`Command` and `Finish` take its `Stop` and `Leave`, each the default
when left empty, and `ShellQuote`, a command written out for a shell. The
sums `make` writes into the build move with it.

Two package variables come along and stay for now. `checked`, the tools
already found to be what they should be, is a cache, and `sums` is what
four tests set for the whole process. `engine/tool` cannot reach the
object above it that outlives a job, so that object holds a value of
`engine/tool`'s, a finder, made once from the sums, with the tools it has
checked on it. `sums` then stays a package variable only as the name
`make` writes to, read once when the finder is made, and a test makes a
finder with sums of its own and runs beside the others.

A package for the files at the bottom takes `SafeChild`, `WriteAtomic`,
the checks of a path in a file system and the plan's lock. Its name is
chosen in its own pull request, where its uses are counted.

The name, chosen on 31 October in that pull request, is `engine/fileio`.
Its functions are used in 88 places in 30 files. `files` said it best,
but the code already calls a file system `files` in 16 places, the
app's store and reporting among them, and `NewEngine` too, and inside
each of those the name would have hidden the package. `fileio` hides nothing in the code, and says a
file read and written, which is what it does. The functions keep their
names, `SafeChild`, `SafeChildIn`, `ResolvePath`, `ResolveIn`, `OnDisk`,
`LstatIn` and `WriteAtomic`, and the four that were the engine's own
are named for what they are once outside it: `replaceFile` is `Replace`,
`lockFile` is `Lock`, `exists` is `Exists` and `isFile` is `IsFile`. The
lock's seam for tests, `waits`, comes along as a package variable, and
moves with the others in the fourth pull request below.

One object in `engine` outlives a job. Each program makes it once in
`main` and every `Engine` takes it. It holds the file system, the
records, the runner, what was learned about the machine, and every
cache and seam that is a package variable today, each moved with its
area. `NewEngine` takes it in place of the file system.

```
engine          every area, still one package, and what outlives a job
engine/runlog   the log of a run
engine/tool     finding and running programs
engine/fileio   a path and a file in a file system
engine/num      numbers
engine/runes    text, by its characters
                engine/timed beside them, using none
```

The pull requests, in order, each one self-contained:

1. `engine/tool`, with the stop seams as fields of `Runner`. The two
   tests that swapped them set their own runner instead. They still run
   alone, for the other package variables they set, `timeLimit` and
   `lookAgain`, until those move in the fourth pull request below.
2. The package for the files, `engine/fileio`.
3. The object that outlives a job, made once in each `main`, with what
   is learned about the machine moved onto it, so `softDecode` is
   learned once per run, and the finder that holds `checked`, with the
   sums handed in.
4. The engine's caches and seams moved onto it, area by area, as many
   pull requests as their size asks, the key store handed in so
   `testing.Testing()` leaves shipped code.
5. The app's own package variables that hold a cache or a seam.

## What follows

- It costs about six pull requests, the first two mostly renames done
  by the type checker, the others a field and its uses each.
- It buys what a machine learned kept for the program's life, tests
  that swap a seam running side by side, and the part framing needs
  under it.
- It was wrong if a part at the bottom has to import something of an
  area to do its work, or if the object that outlives a job grows rules
  of its own rather than holding what its areas own.
