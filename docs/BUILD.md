# Building with make

One command, from the top of the repository:

```
make
```

It builds all three programs into `bin/`: `bin/framefairy`, `bin/framefairy-app` and
`bin/framefairy-train`. That works on a fresh copy of the repository, too. Wipe
the folder, copy the new files in and run `make` again.

## What make does

1. Installs what this machine is missing. On macOS that is Homebrew doing
   Go, Node.js and the few tools that build ffmpeg and llama.cpp. Elsewhere
   it says what to install, because those need administrator rights.
2. Builds the two programs framefairy ships beside itself, if they are not
   there yet: ffmpeg and llama-server. Several minutes, once. Every build
   after this one copies them beside the programs, and from then on the app
   renders through the exact ffmpeg a customer gets and runs a local model
   through the exact llama-server a customer gets, rather than whatever
   Homebrew happens to have.
3. Checks for Go 1.27 or newer and a C compiler, and stops with the install
   command if one is still missing.
4. Resolves the project's Go modules and writes `go.sum`. This needs the
   network the first time.
5. Installs the interface's packages into `frontend/node_modules`, exactly as
   locked in `package-lock.json`, but only when the interface has to be built.
6. Builds the interface into `cmd/framefairy-app/dist/app/`, when it is missing
   or something in `frontend/` changed. This needs Node.js. The built
   interface is not in the repository.
7. Builds the three programs, and puts the speech library beside them in
   `bin/lib`: sherpa-onnx's release without speech synthesis, fetched once,
   8 to 9 MB, because the one in the Go module carries espeak-ng, which is
   GPL 3. See [THIRD_PARTY.md](THIRD_PARTY.md#the-speech-library-without-speech-synthesis).
8. Lists anything this machine still needs to run them.

Steps 1 and 2 cost nothing when there is nothing to do: each one is a
`command -v` or a file test, and no package manager is started unless
something is actually missing. Module resolution and the interface only run
when their inputs changed, and Go rebuilds only what changed, so a second
`make` takes a moment. The module check compares file contents, not dates,
so copied files never fool it. Output from Go and npm is only shown when
something fails.

**The models are not make's business.** The app fetches the speech model and
the language model itself, on first run, which is what a customer does and
so is what this machine should do too. `make models` is still there for the
command line, which has nowhere to ask.

**A build runner never installs anything.** `CI` in the environment turns
step 1 and step 2 off, so what CI builds is what its own workflow asked for
and nothing else. `make INSTALL=0` does the same by hand.

## Targets

| Command | What it does |
| --- | --- |
| `make` | everything above |
| `make run` | the same, then starts the app. On macOS that is the bundle, from inside it, so the log stays in the terminal and the privacy prompts are the real ones |
| `make motion` | opens every way the app shows work in hand on one page in the browser, for looking at a change to any of them without starting a job. Preview material, never in the app |
| `make app` | assembles `bin/Frame Fairy.app` out of what is already in `bin/`. macOS only, and `make run` does it for you |
| `make install` | `make app`, then copies the app to `/Applications`, where it updates itself from the channel picked in its settings. macOS only. See [UPDATES.md](UPDATES.md#end-to-end) |
| `make update-key` | makes the key the builds to update to are signed with, once, on Tim's Mac: the public half into `cmd/framefairy-app/update-key.txt`, the private half to the clipboard. See [UPDATES.md](UPDATES.md#the-keys) |
| `make icon` | builds the `.icns` from `build/icon.png`. `make app` does it for you, so this is for looking at an icon you just changed |
| `make ffmpeg` | builds the ffmpeg framefairy ships again, from scratch, throwing away the one that is there. `make` builds it once by itself, so this is for when `scripts/build-ffmpeg.sh` changed or the last one went wrong |
| `make llama` | the same for the llama-server framefairy ships, which is what runs a local model |
| `make tools-archive` | packs both of them into one archive with a manifest, for a release. See [PACKAGING.md](PACKAGING.md#the-tools-we-ship) |
| `make notices` | writes the licence notices in `notices/` again, from the Go modules, the interface's packages and the source trees ffmpeg and llama-server are built from. Run it when a test or the interface build says a notice is missing or out of date. See [THIRD_PARTY.md](THIRD_PARTY.md) |
| `make changed` | what the branch changed against main, and only that. The check before every push, see [Checking a change](#checking-a-change) |
| `make test` | everything below: `unit`, `fuzz` and `interface` |
| `make unit` | every Go test under the race detector, the fuzz seeds included |
| `make fuzz` | every fuzz target, `FUZZTIME` executions each, looking for new cases |
| `make interface` | a type check of the interface and its own tests. Needs only Node |
| `make check` | what this machine has and what it still needs, with the command for each |
| `make tools` | the installing part of `make` and nothing else. macOS: Homebrew does Go, Node.js and what builds ffmpeg and llama.cpp. Elsewhere it points to [INSTALL.md](INSTALL.md) |
| `make models` | downloads the speech model and the language model into `~/.framefairy/models`, for the command line. The app does this itself |
| `make speechbench AUDIO=episode.mp4` | how fast the speech model hears on this machine, on the processor and through CoreML, with 4 and 8 threads, in pieces of 15 and 30 s, one or two at a time, over three minutes of the episode, and how many words each way changes. Takes a few minutes. `ARGS="-seconds 60"` passes more, see [Measuring the speech model](#measuring-the-speech-model) |
| `make clean` | removes `bin/`, `.build/`, `frontend/node_modules/` and the preview builds |
| `make help` | this list |

`make TIDY=0` skips step 2, for a machine without network whose modules are
already in place.

A new Mac, from nothing to a running app:

```
make run
```

The first one takes a while, because it installs the tools and builds
ffmpeg and llama-server. The app then walks you through the rest: the
speech model it fetches itself, and it asks once how clips should be found.

## Checking a change

`make changed` looks at what the branch changed against `origin/main`,
committed or not, new files included, and runs what those files can reach:

| Changed | Runs |
| --- | --- |
| Go code, or a file a package keeps beside it, in `testdata/` or embedded | `gofmt` on the files, then `go vet` and the tests under the race detector for the changed packages and every package that imports them. Of their fuzz targets, only those that go through a changed file, see below |
| `go.mod`, `go.sum` | every package and every fuzz target |
| `frontend/` | `make interface` |
| `Makefile`, a build script | `make` |
| `scripts/ci-needs*.sh`, `ci.yml` | `scripts/ci-needs-test.sh` |
| `scripts/changed*.sh` | `scripts/changed-test.sh` |
| a workflow | a read of its YAML |
| docs | nothing |
| anything else | `make`, so a file nobody thought of is checked rather than skipped |

Fuzzing is the slow part, ten thousand executions a target, and most
targets read one kind of input a change never goes near. So a target is
fuzzed only when its seeds, run once with coverage, go through a file that
changed, or when one of its own seeds or a test file of its package
changed. Running the seeds takes a second. A change to framing fuzzes
nothing, a change to the caption writer fuzzes the four targets that write
captions, and the rest are named as skipped.

It says what it will run before it runs it, and `sh scripts/changed.sh
--plan` says it and runs nothing. `BASE=` compares with another commit.
Measured on the cloud machine, which has four cores: a change to `updates/`
5 s, to the app 11 s, to the interface 8 s. A change to the engine reaches
every program and takes about two minutes, most of it the engine's own
tests. `make test` is ten minutes. CI still runs
everything on every pull request, so this only saves the round trips, it
does not replace the check.

`scripts/changed-test.sh` checks the rules without running anything, and
`make changed` runs it whenever the rules change.

## Tests

`make test` runs every Go test under the race detector, type checks the
interface and runs the interface's own tests with vitest,
`frontend/src/**/*.test.ts`. The app is a queue of jobs on their own
goroutines and an interface asking them things from another, so the tests of
anything asynchronous use it from several goroutines at once and let
`-race` judge. The fuzzing runs without the detector: it is the same code,
many more times over. One function is left out of the detector,
`faceDetector.classify` in `engine/faces.go`: it only reads, and it is where
framing spends nearly all its time, so the detector's bookkeeping made every
test that frames a clip six times slower. Those cover
the rules the app follows by itself, in `frontend/src/lib/flow.ts`: that a
new episode transcribes itself and that the first clips are found as soon as
the transcript covers the chosen window. Both have broken before, so they
are written as plain functions with tests beside them. It all needs no
model, no network and no API key. Speech comes from a fake recogniser and
planning from a fake llama-server, both in `engine/project_test.go`. The
tests that really render skip themselves when ffmpeg is missing, so the
suite still passes on a machine without it.

The parts that read what a model, a plan file or a caption file contains
also have fuzz targets, named `Fuzz...` next to the ordinary tests. `make
test` fuzzes every one of the 16 of them, because a fuzz target that only
ever sees its seeds is not fuzzing.

The work is a number of executions, `FUZZTIME`, 10000 per target, and not a
number of seconds. A count means the same thing on a fast machine and a slow
one, so a run here and a run in CI do the same work. Go fuzzes one target per
process, so `scripts/fuzz.sh` runs as many targets side by side as the
machine has cores, with one worker each. That is around 50 seconds on a small
four core machine and less on a laptop with more.

`FUZZTIME` also takes a duration, for a deep run by hand:

```
make test                       # 10000 executions per target
make test FUZZTIME=2000x        # quicker, while working on something else
make fuzz FUZZTIME=2m           # two minutes per target, on its own
FUZZJOBS=2 make fuzz            # leave some cores alone
```

Every run keeps the inputs it found interesting, in Go's build cache, and
replays them before it starts fuzzing. That is the point, they are a growing
regression suite, but after a few days of fuzzing the replay is what a run
mostly spends its time on. `go clean -fuzzcache`, which `make clean` also
does, puts it back to the seeds.

A failing input is written to `engine/testdata/fuzz/`. Keep it, it is the
test case for the bug, and it becomes a seed for every later run.

Tests live next to what they test, so `transcript.go` is tested by
`transcript_test.go`. The tests that measure real audio use a tone that is
gated on and off five times a second, because a steady tone measures the
same wherever you start and would hide a start that is a few milliseconds
out.

## CI

`.github/workflows/ci.yml` runs on every pull request and on every push to
main. It is six jobs on six machines, all at once, because none of them
needs any of the rest:

| Job | Machine | What it runs |
| --- | --- | --- |
| `interface` | Linux | `make interface`. Needs only Node, so it is first back by a long way |
| `build` | Linux | `make`. The programs and the interface, which is what proves they still link |
| `linux` | Linux | `make unit` |
| `fuzz` | Linux | `make fuzz` |
| `macos` | macOS | the ffmpeg we ship, built by `scripts/build-ffmpeg.sh` and kept until that script changes, then `make` with no warnings allowed, then `make unit` against that ffmpeg |
| `macos-fuzz` | macOS | `make fuzz` |

`scripts/ci-needs-test.sh` checks those rules and runs in the `build` job
whatever changed, because a mistake in them is silent: CI would go green
having run less than it should. Run it by hand with
`sh scripts/ci-needs-test.sh`.

Run one after another this is about six minutes. Run together the answer
comes when the slowest one does, which is the macOS build and tests.

Nothing is left out to make it quick. Every test that ran before still runs,
on the same platforms, under the race detector, with the same `FUZZTIME`.
The fuzzing is on both platforms because two of the targets are about paths
and a case-insensitive filesystem is a different thing to explore.

### What runs for a change

On a pull request each job asks `scripts/ci-needs.sh` whether there is
anything for it to do, so a typo in a README does not fuzz two platforms:

| What changed | What runs |
| --- | --- |
| `docs/`, any `.md`, `.vscode/`, `.claude/` | nothing |
| `frontend/` only | `interface`, `build`, `macos` |
| Go files, `go.mod`, `go.sum` only | everything but `interface` |
| anything else, or a mix | everything |

Anything else means the `Makefile`, the workflow, `scripts/` and whatever is
added next: they decide how the project is built, so none of them counts as
harmless. A file the rules do not recognise runs everything too. The rules
err towards running, because a test that runs when it need not costs a
minute and one that does not run when it should have costs a broken main.

A push to main narrows nothing. Whatever it touched, everything is built and
tested, so main is always known to be sound and a mistake in the rules can
never be what hides a break.

Every job still runs and still reports, it just does nothing when there is
nothing to do. That keeps the checks a branch rule can be built on, which a
job skipped outright would not, and it costs no waiting: a job with nothing
to do is back in seconds.

A second push to a branch cancels the run the first one started, because its
answer is about code nobody is waiting on any more. Pushes to main are never
cancelled: every commit's result there is worth having on its own.

**A test that needs ffmpeg skips itself where there is none, and in CI
that would read as a pass.** The macOS job once ran without ffmpeg, so
nothing that renders, frames or listens was tested on the system that ships
first, and the only sign was that its tests took five seconds where Linux
took three minutes. `TestCIHasFFmpeg` fails when `CI` is set and there is no
ffmpeg on the path.

## No build warnings on macOS

Go 1.27 builds its own code for macOS 13. Without further settings, clang
builds the C code of some libraries for the macOS that is running, and Wails
asks for macOS 10.13. The linker then warns that parts were built for
different versions. make sets one version for everything:

- `MACOSX_DEPLOYMENT_TARGET=13.0`
- `CGO_CFLAGS` and `CGO_CXXFLAGS` with `-mmacosx-version-min=13.0`, for C
  code that names no version itself
- `-ldflags=-extldflags=-mmacosx-version-min=13.0`, which Go puts last on the
  linker command line, so it wins over the older version Wails asks for

It also sets `CGO_ENABLED=1`, which the speech library needs, and
`GOTOOLCHAIN=local`, so Go never downloads another version of itself. A
`GOTOOLCHAIN` already set in the environment wins, which the cloud
environment in [WORKFLOW.md](WORKFLOW.md) uses.

CI checks the same on every pull request. Its macOS job fails when the build
prints any warning.

The programs therefore need macOS 13 Ventura or newer, which Go 1.27 needs
anyway.

## Without make

The commands make runs, for building by hand:

```
go mod tidy
(cd frontend && npm ci && npm run build)
go build -o bin/framefairy ./cmd/framefairy
go build -o bin/framefairy-app ./cmd/framefairy-app
go build -o bin/framefairy-train ./cmd/framefairy-train
sh scripts/carry-libs.sh bin/framefairy bin/lib
sh scripts/carry-libs.sh bin/framefairy-app bin/lib
```

The last two matter. Without them the two programs look for the speech
library in the Go module cache of the machine that built them, because
that is where the cgo directive points, and they run nowhere else. It goes
unnoticed as long as everyone who runs the app also built it. See
[PACKAGING.md](PACKAGING.md).

On macOS, put the settings above in front to avoid the warnings:

```
export MACOSX_DEPLOYMENT_TARGET=13.0
export CGO_CFLAGS="-O2 -g -mmacosx-version-min=13.0"
go build -ldflags=-extldflags=-mmacosx-version-min=13.0 -o bin/framefairy-app ./cmd/framefairy-app
```

On Windows, add `.exe` to the output names and copy the speech library next
to them, as [INSTALL.md](INSTALL.md#7-before-the-first-build) describes.
`make` does that copy itself when it runs under MSYS2.

## Measuring the speech model

`make speechbench AUDIO=episode.mp4` hears three minutes of an episode
every way the speech model can run on the machine, on the processor or
through Apple's CoreML, with more or fewer threads, in pieces of different
lengths, one or two pieces at a time, and prints how fast each was and how
many words came out unlike the processor's. It uses the speech model in
`~/.framefairy/models`. The workflow `.github/workflows/speechbench.yml`
runs the same on the macOS runner with a recorded talk, when the speech
code changes or by hand.
