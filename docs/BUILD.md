# Building with make

One command, from the top of the repository:

```
make
```

It builds all three programs into `bin/`: `bin/framefairy`, `bin/framefairy-app` and
`bin/framefairy-train`. That works on a fresh copy of the repository, too. Wipe
the folder, copy the new files in and run `make` again.

## What make does

1. Checks for Go 1.27 or newer and a C compiler, and stops with the install
   command if one is missing.
2. Resolves the project's Go modules and writes `go.sum`. This needs the
   network the first time.
3. Installs the interface's packages into `frontend/node_modules`, exactly as
   locked in `package-lock.json`, but only when the interface has to be built.
4. Builds the interface into `cmd/framefairy-app/dist/app/`, when it is missing
   or something in `frontend/` changed. This needs Node.js. The built
   interface is not in the repository.
5. Builds the three programs.
6. Lists anything this machine still needs to run them, such as a missing
   model.

Module resolution and the interface only run when their inputs changed, and
Go rebuilds only what changed, so a second `make` takes a moment. The module
check compares file contents, not dates, so copied files never fool it.
Output from Go and npm is only shown when something fails.

`make` changes nothing outside the repository. System tools and models are
installed only by the two targets below, and only when you call them.

## Targets

| Command | What it does |
| --- | --- |
| `make` | everything above |
| `make run` | builds, then starts the app |
| `make test` | everything below: `unit`, `fuzz` and `interface` |
| `make unit` | every Go test under the race detector, the fuzz seeds included |
| `make fuzz` | every fuzz target, `FUZZTIME` executions each, looking for new cases |
| `make interface` | a type check of the interface and its own tests. Needs only Node |
| `make check` | what this machine has and what it still needs, with the command for each |
| `make tools` | macOS: installs what is missing with Homebrew: Go, llama.cpp, Node.js, and ffmpeg with libass from the ffmpeg tap. Elsewhere it points to [INSTALL.md](INSTALL.md) |
| `make models` | downloads the speech model and the language model into `~/.framefairy/models`, unless they are there. An interrupted download resumes |
| `make clean` | removes `bin/`, `.build/` and `frontend/node_modules/` |
| `make help` | this list |

`make TIDY=0` skips step 2, for a machine without network whose modules are
already in place.

A new Mac, from nothing to a running app:

```
make tools
make models
make run
```

## Tests

`make test` runs every Go test under the race detector, type checks the
interface and runs the interface's own tests with vitest,
`frontend/src/**/*.test.ts`. The app is a queue of jobs on their own
goroutines and a window asking them things from another, so the tests of
anything asynchronous use it from several goroutines at once and let
`-race` judge. The fuzzing runs without the detector: it is the same code,
many more times over. Those cover
the rules the app follows by itself, in `frontend/src/lib/flow.ts`: that a
new episode transcribes itself and that the first clips are found as soon as
the transcript covers the chosen stretch. Both have broken before, so they
are written as plain functions with tests beside them. It all needs no
model, no network and no API key. Speech comes from a fake recogniser and
planning from a fake llama-server, both in `engine/project_test.go`. The
tests that really render skip themselves when ffmpeg is missing, so the
suite still passes on a machine without it.

The parts that read what a model, a plan file or a caption file contains
also have fuzz targets, named `Fuzz...` next to the ordinary tests. `make
test` fuzzes every one of the 15 of them, because a fuzz target that only
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
| `macos` | macOS | `make` with no warnings allowed, then `make unit` |
| `macos-fuzz` | macOS | `make fuzz` |

Run one after another this is about six minutes. Run together the answer
comes when the slowest one does, which is the macOS build and tests.

Nothing is left out to make it quick. Every test that ran before still runs,
on the same platforms, under the race detector, with the same `FUZZTIME`.
The fuzzing is on both platforms because two of the targets are about paths
and a case-insensitive filesystem is a different thing to explore.

A second push to a branch cancels the run the first one started, because its
answer is about code nobody is waiting on any more. Pushes to main are never
cancelled: every commit's result there is worth having on its own.

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
```

On macOS, put the settings above in front to avoid the warnings:

```
export MACOSX_DEPLOYMENT_TARGET=13.0
export CGO_CFLAGS="-O2 -g -mmacosx-version-min=13.0"
go build -ldflags=-extldflags=-mmacosx-version-min=13.0 -o bin/framefairy-app ./cmd/framefairy-app
```

On Windows, add `.exe` to the output names and copy the speech library next
to them, as [INSTALL.md](INSTALL.md#7-before-the-first-build) describes.
`make` does that copy itself when it runs under MSYS2.
