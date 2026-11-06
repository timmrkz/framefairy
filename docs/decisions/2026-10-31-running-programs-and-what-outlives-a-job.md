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
but the code already names a file system `files` in 16 declarations, the
app's store and reporting among them, and `NewEngine` too, and inside
each of those the name would have hidden the package. `fileio` hides nothing in the code, and says a
file read and written, which is what it does. The functions keep their
names, `SafeChild`, `SafeChildIn`, `ResolvePath`, `ResolveIn`, `OnDisk`,
`LstatIn` and `WriteAtomic`, and the four that were the engine's own
are named for what they are once outside it: `replaceFile` is `Replace`,
`lockFile` is `Lock`, `exists` is `Exists` and `isFile` is `IsFile`. The
lock's seam for tests, once the package variable `lockWaits`, is now an
argument of an unexported `lock`, which `Lock` calls with none, because
`engine/fileio` cannot reach the object above it and a seam must not
stay a package variable. The table of locks, `locks`, stays one per
program on purpose: two engines in one program that edit the same plan
must wait for each other, so the table is shared the way the disk is.

Go's own guards were weighed for `SafeChild` and do not fit. `os.Root`
and `filepath.IsLocal` judge a path on the real disk or by its text, and
`SafeChild` has to judge one in whatever file system it is handed, memory
too, and then give a path ffmpeg can open, which an `os.Root` does not.

One object in `engine` outlives a job. Each program makes it once in
`main` and every `Engine` takes it. It holds the file system, the
records, the runner, what was learned about the machine, and every
cache and seam that is a package variable today, each moved with its
area. `NewEngine` takes it in place of the file system.

The name, chosen on 1 November 2026 in that pull request, #644, is
`engine.Machine`, for what it holds: this machine's disk, its records,
the programs it runs and what was learned about it while the program
runs. `Engine` embeds it, so an engine still reads `e.Files` and
`e.Runner`, and keeps beside it only what belongs to one job. What was
learned about an ffmpeg, its encoders and its subtitle filter, is kept
by the ffmpeg, the encoder a run names and the system, because the
command line can name another ffmpeg or encoder for each run. The
`tool.Finder` holds the tools already checked and the sums, read once
from the package variable `make` writes to, and what the environment
names, so its tests make their own and run side by side. Two parts of
the app are made before `main` and keep their own until the app's own
caches move: the probes, `sources`, keep a machine on the real disk, and
the streams, `openPreviews`, a finder for the decoder only they run.

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
5. The app's own package variables that hold a cache or a seam, and
   `chosenMu` in `cmd/framefairy-app/chosen.go`, a second lock beside
   `fileio.Lock`, which then takes its place.

## What follows

- It costs about six pull requests, the first two mostly renames done
  by the type checker, the others a field and its uses each.
- It buys what a machine learned kept for the program's life, tests
  that swap a seam running side by side, and the part framing needs
  under it.
- It was wrong if a part at the bottom has to import something of an
  area to do its work, or if the object that outlives a job grows rules
  of its own rather than holding what its areas own.

## The keys and the network, #648

Added on 2 November 2026, in the pull request that moves them. Every
other seam on the machine is the real thing when left empty, the way
`http.Client` keeps its `Transport`. The keys and the network are the
two seams that reach out of the program, into the keychain and to the
companies. Today `testing.Testing()` in `engine/keys.go` swaps the
keychain for none and blanks the companies' addresses in any test
binary, which is a switch for tests in shipped code. Tim, 2 November:
the empty store is the safe one, and the programs hand in the keychain.

Measured on main at 1251f2a: the keys are read and kept in 15 places
in `keys.go` and `licence.go`, through `keys`, `oldKeys` and
`savedFirst`. The network is two clients, `httpClient` for the
companies, which no test sets, and `downloadClient` for the models,
which one test sets. `cloud` in `engine/provider_test.go` points
`providers` at its own server for the 13 tests that ask a company, which
changes the table every other test reads. The only test that would reach a company today is
the app's `SaveAPIKey` with a real provider, `setup_test.go`, kept from
it only by the blanked address.

Five candidates follow, each from a principle of its own, and each
names what it moves and what it costs.

### A. Where state lives: package variables with a safe default

The keys, `savedFirst` and both clients stay package variables, empty
by default, and each `main` sets them once. The smallest change: the
switch and the blanked addresses go, and no call changes its receiver.
But it is state a test changes for the whole process, so every test that
sets a key store or a transport runs alone, and a key one test keeps can
be read by the next. That is what the row "Never a package variable" in
`docs/ARCHITECTURE.md` and #607 take away. Loses.

### B. What changes together: the machine keeps both, empty is safe

A key kept and a key shown to its company change together: the key
store, the model list's address and the client that carries the key
came in together and are read together in `VerifyAPIKey`. Downloads
change with the setup. So the keys, the order they are read in and one
network for both become unexported fields of `engine.Machine`, made
once in each `main`. Empty, the machine keeps no key and every request
is refused with an error that says so, and a key being checked then
passes, the way it does on a train. `UseSystem` hands it the system's.
The rule that a request carrying a key never follows a redirect stays
on the engine's own client, over whatever transport it is given, so a
test that hands in its own transport tests the same rule. It moves the
15 reads and writes of the keys in `keys.go` and `licence.go` and the
two downloads onto the machine, and every caller in the app, the
command line and `scripts/models` with them. A test sets the fields
from inside the package.

### C. What Go and its libraries already have: exported fields

The machine already shows its seams as exported fields that are the
real thing when empty, `Files`, `Runner` and `Tools`, the way
`http.Client` shows `Transport`. So `Keys` and `Network` would be
exported too, with an exported `KeyStore` interface, and safe when
nil, unlike the others. The app's tests could then hand the app's
machine a store in memory, and `savedLicence` and `saveLicence` in
`cmd/framefairy-app/licence.go` would stop being package variables a
test swaps. The cost: a store a package outside the engine can make
needs exported methods, and then anything that holds the machine can
read a key straight off it, past the rules for the order the keys are
read in, the move of the first ones into the new item and the hint.
The app never reads a key back, see `Copy` in `licence.go`, and C
would make that a habit to keep instead of a thing that cannot be done.
The licence seams become fields of `FrameFairy` with the app's other
seams in #650, which needs no key store from outside the engine. Loses
to B on what it opens, for a gain #650 brings anyway.

### D. Who may know about whom: the engine knows no keychain

Only `main` knows the machine it runs on, so each front end keeps its
own keys, and the engine is handed a key in its options and a client
for the network. The engine then asks nothing about the program it is
in. But the move of the first keys into the new item, the hints, the
order a saved key and the environment are read in, and the licence
store would leave the engine for the app and be written again for the
command line and `scripts/models`, against "a rule lives once, in the
engine". Three front ends would hold three copies. Loses.

### E. What a test has to build: the real thing by default

Every other seam on the machine is the real thing when empty, so the
keys and the network would be too, and the tests would make their
machines through a helper that hands them memory and no network. The
programs then change by not one line. But a test that forgets the
helper writes into the keychain of the developer's Mac, or of CI's,
and shows a key to a company, the box nobody answers, which is what
the switch was there to prevent. Nothing can make a test call the
helper. Loses.

### What converges

A, B, C and E keep the store and the network on something `main` sets,
and B, C and E put them on the machine. A loses on shared state, D on a
rule kept three times, E on a mistake one forgotten line makes. B and C
are as safe as each other, and C opens the keys to every package that
holds the machine. So B: the keys and the network are the two seams
that are safe when empty, because they are the two that reach out of
the program. Every other seam stays the real thing when empty.

`Machine.UseSystem` hands a machine the system's keychain and the
network, and the app, the command line and `scripts/models` call it
once in `main`. `testing.Testing()` and the blanked addresses go. The
model downloads become methods of the machine, `InstallSpeechModel` and
`InstallLanguageModel`, so they go through its network. `downloadStall`
is a constant now, since no test sets it. `httpClient` becomes the
machine's client for the companies, over its transport. `models`, the
facts of each cloud model, and `providers`, the companies' addresses,
are tables no test sets, and stay tables: a test that asks a company
hands its machine a network that carries the real addresses to a server
of its own, `cloud` in `engine/provider_test.go`, and checks the address
each request was meant for.

## The app's caches and seams, #649 and #650

Added on 5 November 2026, in the pull request that moves them. The
decision above already says each cache and seam goes onto a field, each
with its area, and that what the engine learns goes on the machine. What
was left to decide is where the app's own go, the caches only the app
keeps and the seams only the app's tests set. Measured on main at
669dd1a: six caches, `sources`, `unprobed`, `openPreviews`,
`captionTracks`, `layers` and `failures`, and thirteen seams in nine
files, which the tests set in 77 places.

### A. Where state lives: one struct of seams on FrameFairy

Every seam goes into one `seams` struct on `FrameFairy`, the way the
machine holds the engine's. One place to look. But the seams belong to
parts that `FrameFairy` only holds, `listening`, `chromeWatch`,
`updating`, `reporting`, and each of those would reach up to the app
for a field of its own, which none of them does for anything else.
Loses.

### B. What changes together: a field of the part that uses it

The microphone changes with the recording, the measure of the title bar
with the watch of the title bar, the .app an update replaces with the
update. So each seam is a field of the part that uses it, the system's
own when it is empty, the way `http.Client` keeps its `Transport` and
the machine keeps its `Runner`. A test sets it on the part it made. The
caches go the same way: the streams of the video preview keep the
caption tracks and the tally of layers drawn over them, the log keeps
the count of the failures it writes, and `FrameFairy` keeps the probes
and the streams, each ready as its zero value, so the 48 places a test
makes a `FrameFairy` with a literal make a working one.

Its cost is a seam a test forgets. A field left empty is the system's
own, so a test that forgets the microphone listens to the real one and
a test that forgets `showReport` opens Finder. Every test that builds
the whole app goes through the desk, whose `fit` sets the seams a walk
reaches, the microphone, the report's folder and Finder, and the
downloads, so a forgotten seam is left to a test that builds one part by
hand and calls that part. Such a test names the seam it needs on the
line beside its call, where a reviewer sees it.

### C. What Go and its libraries already have: options to a constructor

`newFrameFairy(options ...)`, the functional options of many Go
libraries. A test passes the seams it needs. But `main` builds the app
in steps, each part handed what the part before it made, and the tests
build it the same way through the desk. A constructor would be a second
way to build the app, beside the one both use. Loses.

### D. Who may know about whom: interfaces handed in by main

Each system the app reaches, the microphone, the keychain, Finder, the
downloads, becomes an interface `main` hands in, and nothing has a
default. The app then knows nothing of the machine it runs on. But
every test that makes an app would have to hand in all of them, and a
test that forgets one gets a nil interface and a panic far from where
it forgot. The keychain is the one place an interface pays, since two
methods go together, and there it is used.

### E. What a test has to build: one builder that sets every stand-in

A test should build what it needs in one call and get nothing real by
accident. So one builder, `testApp(t)`, makes a `FrameFairy` with every
seam set to a stand-in that fails the test when it is used unasked,
and a test swaps in the one it means to use. A forgotten seam then
fails loudly instead of reaching the machine, which B cannot promise.
But the seams still have to live somewhere the builder can set them,
which is B's fields, so E is a builder on top of B rather than a home of
its own. And most tests build one part, a `listening`, an `updating`,
a `reporting`, not the app, so the builder would grow one variant per
part. The desk is that builder already for the tests that build the
whole app. Loses as a home, kept as the desk.

### What converges

B, with D's interface for the keychain, `keychain` in `licence.go`,
since its two methods are read and written together. The speech and
language stand-ins are the one seam that is not the app's own part's:
every engine the app makes is readied by `jobOptions` from the store,
the job queue, the captions and a recording among them, so they are a
field of the store, `standIns`, read as a recording starts. What
ffprobe said of a file was kept twice, by the machine and by the app's
`sources`, so the app's copy goes and `FrameFairy` keeps only what a
video was probed as last, for a video whose file has gone. That is not
the engine's `Episode`'s to keep: an `Episode` holds reads of the work
folder, each checked against the stamps of the files it came from, and
drops them when the files change, while what is wanted here is the last
thing known of a source file that has stopped answering, which only the
interface asks for. `stopWait`
was never set by a test, so it is a constant.

What stays a package variable on purpose, each with why:

- `theLog`, the build's version and channel, the embedded interface and
  the update key, which are the program's by nature.
- `theProbe` in `probe_outside.go`, which only the build for the checks
  from outside has, one listener on a fixed port for the one app.
- Tables that never change, `recordingEndings`,
  `transcriptFormats`, `pictureID`.
- In the engine, `disk`, the real disk, which is never swapped,
  `DecoderSaid`, the app's log for the decoder's lines, set once by
  `main`, `fileio.locks` and `tool.sums`, see above, and the tables of
  the hyphenation, the fillers and the recipes.
- The font shelf in `engine/machinefonts.go`, which is a cache and a
  seam, and is what #607 has left. Its fonts are read by the caption
  code wherever a style is resolved, so moving it reaches every caller
  of `FontByName`, `FontBytes` and `InstallFont`, and is a change of its
  own, made in #728, see the next section.

## The font shelf, #728

Added on 6 November 2026, after #720 left the shelf as the last package
variable #607 names. Tim chose to keep #607 open for it and move it in a
pull request of its own. Measured on main at 3b46bbc: `shelf` in
`engine/machinefonts.go` holds the font folders, the faces found in them
and the metrics read of each face. It is read by 35 lines of shipped code
in 15 files and by about 90 lines of tests, and two tests swap it for the
whole process with `UseFontFolders`, so they run alone.

What makes it more than a field to move: the shelf is read from code that
holds no machine. `ResolveStyle` asks it whether the font a style names is
there, and falls back to ours when it is not. `TextWidth`, `FontScale` and
`Thickening` read a face's metrics through it. Through those, the word
model's pure functions reach it, `Gesture.change` and `captionsGo` in
`shape.go`, `ClipDrawing` and `captionsView` in `episode.go`,
`SetCaptionStyle` in `edit.go`, none of which has an engine.

### A. Where state lives: a field of the machine, and every caller takes it

`Machine.Fonts`, ready as its zero value, holds what the shelf holds, and
every function that reads it takes the machine: `ResolveStyle(m, style)`,
`TextWidth(m, ...)`, down to the gesture. Plain and complete, but the
machine then runs through the word model, which is pure on purpose, so a
gesture could read the disk. And a test of a gesture needs a machine for
no reason of its own. Loses.

### B. What changes together: the face is resolved once, with the style

What a caption needs of its face, its name, whether it was missing, its
weight, its alias for libass and its metrics, changes together with the
style that names it. So `Fonts.Style(overrides)` on the machine resolves
the style and its face in one step, and the `Style` carries a `Face`, a
small value with the face's metrics. Everything after works from the
`Style` it is handed, the word model included, with no lookup of its own.
The machine resolves where one is at hand: an `Engine`, an `Episode`, the
app. The pure functions that resolve a plan's style today, like
`Gesture.change` and `captionsGo`, are handed the resolved `Style` by
their caller instead, which costs each one an argument but no machine.
`captionsView` already takes a `Look`, which carries it.

### C. What Go and its libraries already have: an interface handed in

A `FontSource` interface, the way `fs.FS` stands for files, handed to
whatever measures text. The machine's shelf implements it and a test hands
in a map. It keeps the pure functions free of the machine, but every one
of them gains an argument, as in A, only typed as an interface, and the
metrics cache has to sit behind it as well. Loses to B, which hands the
pure functions nothing new.

### D. Who may know about whom: a snapshot of the fonts

The machine hands out the faces found as an immutable snapshot, replaced
whole when the folders are read again, the way `atomic.Pointer` replaces a
config. Readers never lock. It is how the shelf should keep its faces
inside, and B keeps it that way: `Machine.Fonts` swaps its snapshot on
`RefreshFonts`. As a design of its own it says nothing about how the
snapshot reaches the pure functions, which is the question. Taken into B.

### E. What a test has to build: the bundled faces with nothing to set

A test of captions should see our bundled faces with nothing to build, and
a test of the machine's fonts should put faces in memory without touching
any other test. B gives both: a `Fonts` zero value offers the bundled
faces only, and a test of the machine's fonts makes a machine of its own
and points its `Fonts` at a memory folder. `UseFontFolders` goes, so the
two tests that set it run side by side.

### What converges

B, with D's snapshot inside `Fonts`. The shelf becomes `Machine.Fonts`.
`Style` carries its `Face`, and the word model reads metrics from it.
`UseMachineFonts` and `RefreshFonts` become methods of `Fonts`, called by
`main` on the program's machine. The app's font list and the window's
font requests ask the store's machine. Done when no package variable in
`engine/` or `cmd/framefairy-app/` holds a cache or a seam, which closes
#607.

### What was built, and where it left the design

`Machine.Fonts` is the shelf, a value ready as its zero value, which offers
the bundled faces alone. `UseSystem` points it at the machine's font
folders, and `Use` at any others, a test's in memory. `FontByName`,
`FontBytes`, `InstallFont`, `CaptionFonts` and the rest became its
methods, and `ResolveStyle` became `Fonts.Style`. `UseFontFolders`,
`UseMachineFonts` and `RefreshFonts` are gone, and the tests of the
machine's fonts each make a `Fonts` of their own and run side by side.

Two things came out otherwise than B said, once the code was read call by
call:

- A `Style` carries the `Fonts` it was resolved by, not a `Face` value.
  The caption code asks for faces by name after the style is resolved:
  the room for a line measures the style's face, and a caption file
  installs it beside itself for libass. A `Face` would have to carry the
  face's file as well as its metrics. Carrying the `Fonts` gives the
  later steps the same faces the style was resolved against, with no
  second lookup path.
- The pure functions that resolve a plan's style, `Gesture.change`,
  `captionsGo`, `ClipDrawing`, `ShapeClip`, `Reshape` and
  `SetCaptionStyle`, take a `*Fonts` rather than a resolved `Style`. Each
  resolves the style of the plan it is changing, after the change, so
  its caller cannot resolve it first. That is the cost C named, an
  argument each, but typed as the one concrete thing, and nil offers the
  bundled faces, so a test of a gesture still builds nothing.
  `captionsView` reads it from its `Look`.

D's snapshot was left out. `Fonts` keeps the lock the shelf had, because
nothing showed readers waiting on it, and the change was large enough
without it.
