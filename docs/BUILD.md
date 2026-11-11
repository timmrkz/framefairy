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
   Go, Node.js and a Python of 3.10 or newer, and
   `scripts/build-tools.sh` putting the few tools that build ffmpeg and
   llama.cpp, cmake, meson, ninja and pkg-config, in `.build/tools` at the
   versions `requirements.txt` pins as each build starts, `make ffmpeg`
   and `make llama` too. CI's Macs build with the same, and their caches
   of ffmpeg and llama-server are keyed on those pins, so a newer tool
   builds them again.
   Elsewhere it says what to install, because those need administrator
   rights.
2. Builds the two programs framefairy ships beside itself, if they are not
   there yet or their build script has changed since, which is what
   raising a pin is: ffmpeg and llama-server. Several minutes, once for
   each version. Every build
   after this one copies them beside the programs, and from then on the app
   renders through the exact ffmpeg a customer gets and runs a local model
   through the exact llama-server a customer gets. They are the only ones
   the programs run: nothing comes from the search path, and a build of
   either that fails stops `make` rather than leaving Homebrew's to answer.
   Every source they are built from is fetched and checked against its
   pinned SHA-256 or commit on every build, never reused from a folder
   unpacked before, and the SHA-256 of each finished tool is built into the
   programs, which run a tool only when it matches, see
   `scripts/tool-sums.sh` and `Find` in `engine/tool/find.go`.
   `NOTOOLS=1` builds the programs without them, for a runner that only
   proves the programs link, and those programs then run no tool.
3. Checks for Go 1.27 or newer and a C compiler, and stops with the install
   command if one is still missing.
4. Resolves the project's Go modules and writes `go.sum`. This needs the
   network the first time.
5. Installs the interface's packages into `frontend/node_modules`, exactly as
   locked in `package-lock.json`, but only when the interface has to be built.
   `go.mod` ignores that folder, because some of them ship Go code of their
   own, and `./...` names only our packages. The job `build` in CI checks
   that, once the packages are installed.
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
| `make dispenser` | the licence dispenser on this machine, with a pretend Paddle, mail service and signer, and a dev page at `http://127.0.0.1:8090/dev` to try every sale and every failure by hand. See [LICENCE.md](LICENCE.md#on-this-machine) |
| `make icon` | builds the `.icns` from `build/icon.png`. `make app` does it for you, so this is for looking at an icon you just changed |
| `make ffmpeg` | builds the ffmpeg framefairy ships again, from scratch, throwing away the one that is there. `make` builds it by itself when it is missing or `scripts/build-ffmpeg.sh` changed, so this is for when the last one went wrong |
| `make llama` | the same for the llama-server framefairy ships, which is what runs a local model |
| `make frames` | the episode's decoder, `framefairy-frames`, built against the libraries of the ffmpeg above and put beside the programs with it. `make` builds it when it is not there or its code, the modules or that ffmpeg changed since, decided by file times, and says to run `make ffmpeg` first where that ffmpeg is not there. `make unit` and `make walks` build it first wherever that ffmpeg is there, and again when it is older than its code, so new tests never meet an old decoder or skip for want of one. See [VIDEO-PREVIEW.md](VIDEO-PREVIEW.md) |
| `make tools-archive` | packs both of them into one archive with a manifest, for a release. See [PACKAGING.md](PACKAGING.md#the-tools-we-ship) |
| `make notices` | writes the licence notices in `notices/` again, for what is pinned by hand: Go, the source trees ffmpeg and llama-server are built from, the speech runtime, the fonts, the hyphenation patterns and the models. Run it when a test says a notice is out of date. The Go modules and the interface's packages need nothing: every build writes theirs. See [THIRD_PARTY.md](THIRD_PARTY.md) |
| `make hyphenation` | writes the patterns in `engine/hyphenation/` again, which break a word too long for a caption line: hyph-utf8's for 20 languages and the German joints, built from the Trennmuster team's word list, each from the commit `scripts/hyphenation.sh` pins. It needs `patgen`, from `texlive-binaries` on Linux or `texlive` on macOS, takes about five minutes and is only for when a pin moves. Run `make notices` after it. See [ENGINE.md](ENGINE.md) |
| `make test` | everything below: `lint`, `interface`, `unit`, `fuzz` and `walks`, which is what CI runs, see [One way to test](#one-way-to-test) |
| `make lint` | everything that reads the code without running it: `gofmt`, `go vet`, that `./...` is our packages alone, golangci-lint, which refuses a new wait on the clock in a Go test, see [No test waits on the clock](#no-test-waits-on-the-clock), every script read by its shell, the test of the rule for a build, `scripts/needs-build-test.sh`, that `docs/GUI-PLAN.md` is as it was, by the hash git gives it, and the tests of the repository's own rules, `internal/workflows` and `internal/naming`, which read the workflows, the docs and the interface's words. It fetches the golangci-lint the Makefile pins into `.build/` the first time, in CI too |
| `make unit` | every Go test under the race detector, the fuzz seeds included. On a Mac without the race detector and without the licence server's packages, which run on a Linux server only. `RACE=` and `UNITSKIP=` say otherwise. `UNITSHARD=1/3` to `3/3` runs a third of the tests, which is how CI spreads them over three machines. `UNITFLAGS` adds one flag for go test, like `-run`. `UNITPKGS` names the packages, `./...` unless it says. `UNITTIMEOUT` is how long one package's tests may take before go test ends them and prints every goroutine, Go's own 10m unless it says, and 4m in CI: a test waits on signals, so one whose signal never comes hangs until then. go test runs through [gotestsum](https://github.com/gotestyourself/gotestsum), which prints what go test prints and, at the end, every test that failed with what it printed. Then every test that took over `SLOWEST`, 5s, slowest first. Every test's time is kept in `.build/unit-tests.json` for a closer look. `COVERAGE=` a folder counts the lines the tests run, see [Coverage](TESTING.md#coverage) |
| `make fuzz` | every fuzz target, `FUZZTIME` executions each, looking for new cases. `FUZZTARGETS` names some, `CORES` how many run at once |
| `make interface` | a type check of the interface, its own tests, and ESLint, which refuses a new wait on the clock in a walk or an interface test, see [No test waits on the clock](#no-test-waits-on-the-clock). Needs only Node. `COVERAGE=` a folder counts the lines its tests run |
| `make walks` | the interface in Chromium against the real Go side, or in the browser `WALKBROWSER` names: the sequences of the cases found by hand, and every walk for `WALKS` seeds, 1 unless set, worked out from the commit `WALKSEED` names, `GITHUB_SHA` in CI, and 1 to `WALKS` without it, with the seeds `walks/kept-seeds.txt` keeps, each as long as it says unless `STEPS` is set, half as many again at once as the machine has cores unless `WALKERS` is set, each sequence a run of its own and the sequences of a walker walked by one node, each with less motion unless it looks at motion or `STILL=none` is set, stopped after `WALKTIMEOUT`, 30m unless set, each keeping a Playwright trace when it fails, with a film of the screen for those `walks/filmed.mjs` names, or for every walk with `TRACE=film`, and no trace with `TRACE=none`. `WALKSWEEP=every` resizes the app a pixel at a time the whole way, as the nightly walks do. Needs Node, ffmpeg and Playwright's Chromium, or its WebKit for `WALKBROWSER=webkit`. `COVERAGE=` a folder counts the lines of the app and the interface they run. See [TESTING.md](TESTING.md) |
| `make check` | what this machine has and what it still needs, with the command for each |
| `make tools` | the installing part of `make` and nothing else. macOS: Homebrew does Go, Node.js and Python, and `scripts/build-tools.sh` what builds ffmpeg and llama.cpp. Elsewhere it points to [INSTALL.md](INSTALL.md) |
| `make models` | downloads the speech model and the language model into `~/.framefairy/models`, for the command line, through the engine's own installers, so each is held to its pinned size and SHA-256. The app does this itself |
| `make speechbench AUDIO=episode.mp4` | how fast the speech model hears on this machine, on the processor and through CoreML, with 4 and 8 threads, in pieces of 15 and 30 s, one or two at a time, over three minutes of the episode, and how many words each way changes. Takes a few minutes. `ARGS="-seconds 60"` passes more, see [Measuring the speech model](#measuring-the-speech-model) |
| `make clean` | removes `bin/`, `.build/`, `frontend/node_modules/` and the preview builds |
| `make help` | this list |

`make TIDY=0` skips step 4, for a machine without network whose modules are
already in place.

A new Mac, from nothing to a running app:

```
make run
```

The first one takes a while, because it installs the tools and builds
ffmpeg and llama-server. The app then walks you through the rest: the
speech model it fetches itself, and it asks once how clips should be found.

## One way to test

`make test` runs every test there is, in five parts, `make lint`, `make
interface`, `make unit`, `make fuzz` and `make walks`, and each part runs
on its own too. CI runs the same parts with the same commands, a job for
each, see [CI](#ci), so a test runs the same on a Mac, in a cloud session
and in CI. No script picks, runs or reports a test.

Every pull request runs `make lint` and the build, in the job `build`,
whatever it changed, because the repository's own rules read the docs
and the workflows too. Beyond that, a pull request starts only the jobs
whose code it changed, by the filters of the job `changes` in `ci.yml`:

| Changed | Starts |
| --- | --- |
| Go: any file outside `frontend/`, the docs and `.claude/`, a script, the Makefile and the workflows among them, and the files of `frontend/` a Go test reads, the filter `goread` | `linux`, `fuzz`, `macos`, the walks and `outside` |
| the interface: `frontend/`, the Makefile, `.github/workflows/`, `.github/actions/` and `.node-version` | `interface`, the walks and `outside` |
| only the docs, `docs/`, any `.md`, `.claude/` | only `build` |

A push to main starts every job. Inside a Go job Go's own test cache
picks the packages: CI restores it from main, so `go test ./...` runs
again only the packages whose code or test files changed since, and says
`(cached)` for the rest. The walks test the whole app, its Go side too,
so they start for either kind of change, and run whole.

A filter that missed a file would show as a test that passes on its
pull request and fails on main. So `internal/workflows` reads every Go
test for the files of `frontend/` and `docs/` it names: a file of the
interface a Go test reads has to be in `goread`, and only the tests
`make lint` runs may read the docs.

### Why the walks run whole

Which part of the app a walk reaches is written nowhere, so a pick of
some walks could leave out the one that breaks. A change that starts the
walks runs every walk, on four machines, and main walks every walk after
each merge too.

A cloud session runs the tests of what it changed before a push and
leaves the rest to CI, see the tests rule in CLAUDE.md.

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
the transcript covers the window. Both have broken before, so they
are written as plain functions with tests beside them. It all needs no
model, no network and no API key. Speech comes from a fake recogniser and
planning from a fake llama-server, both in `engine/project_test.go`. The
tests that really render skip themselves when ffmpeg is missing, so the
suite still passes on a machine without it, and fail in CI instead, see
[CI](#ci).

The parts that read what a model, a plan file or a caption file contains
also have fuzz targets, named `Fuzz...` next to the ordinary tests. `make
test` fuzzes every one of the 30 of them, because a fuzz target that only
ever sees its seeds is not fuzzing. `make fuzz` finds them in the source,
with `git grep`. Asking go test for them compiled the tests of every
package first, two minutes of CI's fuzz job before anything was fuzzed.

The work is a number of executions, `FUZZTIME`, 10000 per target, and not a
number of seconds. A count means the same thing on a fast machine and a slow
one, so a run here and a run in CI do the same work. Go fuzzes one target per
process, so `make fuzz` runs as many targets side by side as the
machine has cores, with one worker each. Fuzzing builds each package again
with coverage counters, and that build is most of a run that starts with an
empty cache: 3 min 37 s on a small four core machine, against 1 min 42 s
with the builds there, most of it `FuzzPlanEdits`.

One target is the exception. `FuzzSimulation`, the licence dispenser's
simulation, plays a whole history of 400 steps in one execution, a fifth
of a second, so it does `SIMTIME` executions, 300 by default, instead of
10000, which would be half an hour. Its input is only the seed of a random
generator, and one bit more is a history that has nothing to do with the
last, so coverage cannot steer it and fuzzing it is no better than playing
new seeds. CI leaves it to the nightly run with `SIMTIME=0`. Its known
seeds play in every go test.

An input that reaches new code is shrunk before it is kept, and Go shrinks
for up to a minute by the clock, in executions `FUZZTIME` does not count.
Its search tries every pair of places in the input, so a target whose
execution takes milliseconds would shrink for the whole minute every time.
`make fuzz` gives the shrinking a count too, 100 executions. `FuzzPlanEdits` also reads at most 32 edits of an
input, because every edit is a plan flushed to the disk. Without the two,
it stood still for minutes at a time in a cloud session.

`FUZZTIME` also takes a duration, for a deep run by hand:

```
make test                       # 10000 executions per target
make test FUZZTIME=2000x        # quicker, while working on something else
make fuzz FUZZTIME=2m           # two minutes per target, on its own
make fuzz CORES=2               # leave some cores alone
make fuzz FUZZTARGETS='FuzzSafeChild FuzzPlanEdits'   # only these
```

Every run keeps the inputs it found interesting, in Go's build cache, and
replays them before it starts fuzzing. That is the point, they are a growing
regression suite, but after a few days of fuzzing the replay is what a run
mostly spends its time on. `go clean -fuzzcache`, which `make clean` also
does, puts it back to the seeds.

CI keeps that cache too, in the Actions cache under `fuzz-v2-`, with the
packages built for fuzzing. A push to main and the nightly run save it, and
every pull request starts from the newest. So a run goes on from where the
last one stopped, rather than spending its 10000 executions finding the same
easy inputs as every run before it. Should the replay ever take longer than
the fuzzing, raise the `v2` in `ci.yml` and `fuzz-nightly.yml`, which starts
the cache over from the seeds. It was raised from `v1` when the fuzzing
on Linux moved into the CI image, whose home is another place. The Mac's
nightly fuzzing does not run in the image and keeps its own cache, under
`fuzz-v1-macOS-`, which is raised on its own.

`.github/workflows/fuzz-nightly.yml` is the long fuzzing, at night on main
and by hand from the Actions tab: half a minute on every target, 600 new
histories for the simulation, and two minutes each on a Mac for the two
targets about paths, `FuzzSafeChild` and `FuzzKnownStaysInTheLibrary`,
since the Mac's disk takes names in either case and `/var` is a link there.
A crash fails the run and leaves the input as an artifact called
`crashers`.

`.github/workflows/walks-nightly.yml` walks every sequence with the app's
motion, `make walks STILL=none`, at night on main and by hand, in the
same four shares and the CI image as the walks in `ci.yml`. On a pull
request a sequence asks for less motion unless it looks at motion, see
Where they run in [TESTING.md](TESTING.md#where-they-run), and this is
the run that sees a step that looks at motion and was left out. The
same night the Go tests, the interface's tests and the walks count the
lines they run on Linux, and the counts go to Codecov, see
[Coverage](TESTING.md#coverage).

A failing input is written to `engine/testdata/fuzz/`. Keep it, it is the
test case for the bug, and it becomes a seed for every later run.

### No test waits on the clock

A test never waits, and never on the clock, see CLAUDE.md and the
testing skill in `.claude/skills/testing/`, and two linters hold every
test to the part about the clock, each with that one rule:

- golangci-lint, `make lint`, with only forbidigo, and only on test files,
  see `.golangci.yml`. It refuses `time.Sleep`, `time.After`, `time.Tick`,
  `time.NewTimer`, `time.NewTicker` and `time.AfterFunc`, and a context
  with a deadline, `context.WithTimeout` and `context.WithDeadline`, and a
  check on how long something took, `time.Since` and `time.Until`, by
  type, so `time` under another name is caught too. `time.Second` and the
  other durations are fine. It runs over the whole tree, so a line main
  already has fails the same as a new one, and a second run reads
  `internal/outside` as `GOOS=darwin` with the outside tag, the only way
  it builds. The few lines that keep the
  real clock on purpose say why on the line, with
  `//nolint:forbidigo // why`, and the issue when there is one. The
  version is the one the Makefile pins, 2.14.0, which make
  fetches into `.build/` from the release and checks against the
  release's checksums, the first time and unless `INSTALL=0`. An older
  one refuses a module that says go 1.27, and the 2.5.0 a cloud session
  has in `/usr/local/bin` is one.
- ESLint, in `make interface`, see `frontend/eslint.config.js`:
  `eslint-plugin-playwright`'s `no-wait-for-timeout`, and `waitForTimeout`
  on a window of another name, `setTimeout`, `setInterval`, `Date.now`,
  `performance.now` and Node's timers by import, a `timeout` or `polling`
  handed to a wait, in the walks, `frontend/preview/walks/**/*.mjs`, and the
  probes, `frontend/preview/*.mjs` and `frontend/preview/frames/*.mjs`,
  and the same with `vi.waitFor` and `vi.waitUntil` in
  `frontend/src/**/*.test.ts`. What is left is counted in
  `frontend/eslint-suppressions.json`, ESLint's own list: reads of the
  clock that measure, and the stalls a walk puts on the app on purpose.
  One more in a file fails, and so does one fewer, until
  `npm run lint -- --prune-suppressions` in `frontend/` counts again. No
  walk, probe or interface test waits on the clock since #341 and #495.
  Since #503 a walk counts browser frames only where the count is the
  scenario: a hand held still on the clip timeline for about 0.7 s, and a
  Go side made slow on purpose. Everywhere else a watch ends on the app's
  own signals, the page settled, the picture standing, the sound card
  held, or the episode's own time.

A clock the test controls is not refused: Playwright's `page.clock`,
Vitest's `vi.useFakeTimers`, and a `time.Sleep` in a `testing/synctest`
bubble, whose line says `//nolint:forbidigo // synctest bubble`.

Tests live next to what they test, so `transcript.go` is tested by
`transcript_test.go`. The tests that measure real audio use a tone that is
gated on and off five times a second, because a steady tone measures the
same wherever you start and would hide a start that is a few milliseconds
out.

## CI

`.github/workflows/ci.yml` runs on every pull request and on every push
to main. Its first job, `changes`, reads which files the pull request
changed, and every other job starts only when what it tests changed, see
[One way to test](#one-way-to-test). A push to main starts them all. The
jobs run all at once, because none needs another, after `image`, which
the jobs in the CI image wait for. A job is its machine, the checkout,
the setup every job shares, `.github/actions/prepare`, and one make
command, the one a person runs:

| Job | Machine | Starts for | What it runs |
| --- | --- | --- | --- |
| `changes` | Linux | a pull request | `dorny/paths-filter`, which reads the files GitHub lists for the pull request. Starts nothing itself |
| `image` | Linux | Go or the interface | the CI image `build`, `linux`, `fuzz` and the walks run in, see [The CI image](#the-ci-image), by `.github/workflows/ci-image.yml`, which the nightly fuzzing and the security check call too. Asks the registry whether the image for this commit's recipe is there, and builds and pushes it only when it is not |
| `interface` | Linux | the interface | `make interface`, ESLint among it. Needs only Node, so it is first back by a long way |
| `build` | Linux, in the CI image | Go or the interface | `make lint`, then `make NOTOOLS=1`, the programs and the interface, which is what proves they still link, without the tools we ship, which nothing builds for Linux yet. In CI `make` fails on a `go.mod` that is not tidy, where it would tidy it elsewhere |
| `linux (1/3)` to `linux (3/3)` | Linux, in the CI image | Go | `make unit UNITSHARD=1/3` to `3/3`, a third of the tests each by the first letters of their names. On one machine they took six minutes |
| `walks (1/4)` to `walks (4/4)` | Linux, in the CI image | Go or the interface | `make walks WALKSHARD=1/4` to `4/4` with `WALKTIMEOUT=8m`, a quarter of the runs each by how long they took last, `walk-times.tsv`, so a run that hangs is named in minutes. After the walks, the log says how busy the walkers were and what every run took, slowest first. Every walk walks in Chromium, and every night in WebKit too, see Two browsers in [TESTING.md](TESTING.md#two-browsers). On four machines the jobs took 181 to 226 seconds in Chromium, in three runs on 5 November, and 223 to 377 while every walk walked in WebKit |
| `fuzz` | Linux, in the CI image | Go | `make fuzz SIMTIME=0`, from the fuzz cache main left, see [Tests](#tests) |
| `macos` | macOS | Go | `make unit` against the ffmpeg we ship, built by its script and kept until the script changes. On a Mac it leaves out the race detector, which Linux runs, and the licence server's packages, which never run on a Mac |
| `outside` | macOS | Go or the interface | `make outside WERROR=1`: the app built with the ffmpeg and the llama-server we ship and no warning allowed, see [No build warnings on macOS](#no-build-warnings-on-macos), then a keychain of its own, then the app opened by macOS from an Unlock link, closed and open, with the local dispenser beside it, see [TESTING.md](TESTING.md#from-outside-the-app). Stopped after 30 minutes |

Every job but `interface` starts from Go's build cache as main left it
after its last push, one cache for each job and for each machine of
`linux`, saved by main through `.github/actions/save`. It saves only
what the job used: `.github/actions/prepare` makes every entry old when
it restores the cache, Go marks what it reads, and what is still old at
the end goes. Restoring a cache and saving it again with what was new
kept everything ever built, and a Linux test job restored 669 MB where
it used about 214. Every run says in its log how much of the cache it
used. The modules have a cache of their own, the zips Go downloaded, one
for each `go.sum`. The build to update to in
`builds.yml` starts from the cache of `outside`, which runs the same
`make`, and saves none of its own, so no upload sits between a push and
its build on the Updates page. setup-go keeps the cache by `go.sum`
alone and saves it only when `go.sum` changes, so with it a job started
from a cache weeks old and compiled nearly everything again: a minute on
Linux and close to two on a Mac, the episode's decoder included.

When a Go test fails, gotestsum, which `make unit` runs go test through,
prints it again at the end of the log with what it printed, under
`=== Failed`, so the failure is the last thing on the check. A walk that
fails is named with its steps in the log of `TestWalks`, and its trace
is kept with the run, see Finding why a walk failed in
[TESTING.md](TESTING.md).

Under the Go tests, `make unit` names every test that took over five
seconds, slowest first, `SLOWEST`. The Mac runs three cores where Linux
runs four, and a test that is slow only there shows in its log without
anybody running it again. GitHub's Macs differ: of 203 runs of `macos`
from 29 October to 3 November, 179 had an Apple M1 with 3 cores and 24
an M2 Pro with 5, where the `engine` package took a third less time. So
a Mac run's seconds say more about the machine than about the code, and
a claim that tests got slower or faster needs several runs of each side,
with their spread, #592.

The jobs of the Go tests stop after 15 minutes. go test ends a package
that runs over `UNITTIMEOUT`, 4m in CI, and prints every goroutine, so
the limit only ever ends a run that even go test could not end.

Run together the answer comes when the slowest one does, and no job may
take over five minutes, see Keeping the tests quick in CLAUDE.md. A job
that grows past it is split over machines, the way `linux` is.

The Go tests are split by patterns that never change, never by a list of
test names, because Go keys its test cache on `-run`: with one list for
the whole module, a test added anywhere made every package miss the cache
on every machine. Now a package says `(cached)` unless the change reaches
it. `-skip` takes exactly what the two `-run` leave, so every test runs
once, the fuzz seeds among them. Split by package instead, the app's
tests with the small packages took 208 s on one machine, in one run,
because the app's tests run one after another, while the engine's run
side by side.

Every test runs on Linux under the race detector, with the same
`FUZZTIME`, and every test of what runs on a Mac runs on a Mac too. The
account runs five Mac jobs at once for every branch together, so a push
takes two, `macos` and `outside`, and the app is built once there. The
two fuzz targets about paths are fuzzed on a Mac at night, because a
case-insensitive filesystem is a different thing to explore. The others
read text and numbers, which Go reads the same on both. The known inputs
of every target run on a Mac in `make unit`.

No branch rule requires a check today. The Updates page does not read CI
at all: it follows the builds of `builds.yml`, see
[UPDATES.md](UPDATES.md).

A second push to a branch cancels the run the first one started, because its
answer is about code nobody is waiting on any more. Pushes to main are never
cancelled: every commit's result there is worth having on its own.

**A test that needs ffmpeg and finds none it can use fails in CI, and
skips everywhere else.** CI does not list skipped tests, so a skip there
reads as a pass. The macOS job once ran without ffmpeg, so nothing that
renders, frames or listens was tested on the system that ships first, and
the only sign was that its tests took five seconds where Linux took three
minutes. So every such test ends through one package,
`internal/ffmpegtest`: `ffmpegtest.Need` when it wants ffmpeg and ffprobe
on the path, `ffmpegtest.Unusable` with the reason when the ffmpeg there
cannot do what it needs, like `Preflight` failing or no captions burned in.
Either skips with the reason, and when `CI` is set, which GitHub Actions
sets and which the Makefile reads to install nothing, it fails with the
reason instead. That covers an ffmpeg that is there but broken as well as
one that is missing, which the one test that looked for ffmpeg on the
path before did not. `TestNoTestSkipsForFFmpegByItself` reads every test
file and fails on a skip whose reason or condition is about ffmpeg,
ffprobe, `Preflight`, `SubtitleFilter` or `VideoEncoder` that does not go
through the package, so a new test cannot skip silently again.

**A test goes to the real disk only when it says why.** Our own files,
plans, transcripts, job records, the library and the training records,
are read and written through an `afero.Fs`, see `engine/files.go`, and a
test hands it memory. A test that cannot, because ffmpeg or another
program reads its files, or because a link or a lock on the disk is what
it tests, says so in a comment that starts with "On the real disk", on
the function itself. A reason at the top of a file does not count, since
it would also cover the tests added to the file later. `TestTestsStayOffTheDisk` in
`internal/offdisk` reads every test file and fails on a function that
calls `t.TempDir`, a file function of `os`, `afero.NewOsFs` or the
engine's `disk` without one.

**A program a test writes is written through `standin.Write`.** A
stand-in for ffmpeg, ffprobe or llama-server is a shell script the test
writes and then runs. Linux refuses to run a file that is open for
writing, "text file busy", and a fork made by another test while the
script was being written kept it open in the child until that child had
started its own program. So now and then a stand-in was refused the first
time it ran, which is how a stand-in ffmpeg was once asked one question
in place of two, #410. `standin.Write` in `internal/standin` holds
`syscall.ForkLock` for reading while the file is open. Every fork in Go
creates its child while it holds the lock for writing, so no child is
created while the file is open, and none can hold it.
`TestAProgramWrittenWhileOthersStartRunsAtOnce` writes and runs 800
scripts from 16 goroutines at once, and with `os.WriteFile` 130 to 146
of them were refused in each of three runs in a cloud session.
`TestEveryProgramATestWritesIsWrittenThroughStandin` in `internal/offdisk`
reads every test file in the same walk as the check above, and fails on
an `os.WriteFile`, `os.OpenFile` or `os.Chmod` whose mode lets the file
run.

**On a Mac the tests use the ffmpeg the programs ship with.** `make unit`
and `make fuzz` put `bin/` first on the `PATH` there, so a
test finds the ffmpeg and llama-server `make` built before any other. Not
on Linux, where ours has no H.264 encoder yet and every test that renders
would fail, so a Linux machine keeps the system's. A Mac with
Homebrew's ffmpeg tested that one before, which is not the version the
app ships and not built the same way, and a test that failed there said
nothing about the app. `make walks` is the exception and keeps the
system's, because the camera's videos `speed.mjs` and `smooth.mjs` ask
for are made with x264 and x265, which ours has not. Every other video a
test reads is made once, by the system's ffmpeg, and committed, see
`internal/fixtures`, so which encoders ours has does not matter to it.

**The tests that render share their episodes and skip the face detector.**
A test episode is committed once for each length in `internal/fixtures`,
and every test gets a copy. The face detector finds no
face in a test picture, so the tests turn it off and framing falls back
to what is in focus, as it did anyway. It cost some 50 milliseconds a
frame, and with forty identical episodes made one by one it was most of
the five minutes the engine's tests took. `TestProjectSteps` still frames
with it.

**No Linux job of CI installs a system package.** Every one that
compiles Go, `build` when the app changed, `linux`, `fuzz`, the walks,
the nightly fuzzing and govulncheck, runs in the CI image below, which
has everything they use. On a day the Ubuntu mirror crawled, the fuzzing
once waited fifteen minutes for a speech synthesis library ffmpeg
depends on, and the walks up to twelve for Chromium's fonts. Only
`tools.yml`, the build of the tools we ship, still installs its own,
because it runs by hand, takes half an hour anyway and holds up nothing.

### The CI image

`build`, `linux`, `fuzz`, the walks, the nightly fuzzing and govulncheck
run in a container, the CI image, built from
`.github/ci-image/Dockerfile`: Ubuntu 24.04, the runners' own system,
with every package the Go side compiles against,
ffmpeg, meson, ninja, nasm and patchelf beside them. The walks run in a
second image of the same file, the first with the headless Chromium and
the WebKit of the Playwright version in `frontend/package-lock.json`,
with the libraries and fonts they need. With WebKit in one image for
every job, pulling it took 73 to 80 s where 24 to 30 s had done, so the
jobs that open no browser take the first. Go and Node are in neither:
setup-go and setup-node bring them as in every other job.

On some days the runners' Ubuntu mirror gave 18 to 40 kB a second, and
the libraries Chromium needs, mostly fonts, held a walks job for up to
twelve minutes, #462. Chromium itself comes from Playwright's own
servers and was always fast. The Go tests of `linux` waited the same
way, up to five minutes for their packages, and the fuzzing once
fifteen. An image is downloaded from GitHub's container registry
instead, and no job in it asks the mirror for anything. A change to the
image's Dockerfile builds a new one, and every job in it runs on that.

`.github/workflows/ci-image.yml`, which `ci.yml` calls as the job
`image`, and `fuzz-nightly.yml`, `walks-nightly.yml` and `security.yml` call too, names the image by a sum of
the Dockerfile and of the Playwright version, as `ghcr.io/timmrkz/framefairy-ci:<sum>`, and the walks' image
as `<sum>-walks`. When the registry has both tags, the job is done in
seconds, and when it does not, which is only when one of the three
changed, it builds both images and pushes them,
and the jobs in it wait for it. Both images the Dockerfile starts from
are pinned to a digest, so the same sum always means the same image, and
moving them is a change to the file. Dependabot moves both once a week,
in one pull request. A Playwright update from Dependabot likewise builds
its own image on its own pull request. A file the Dockerfile comes to
read joins the sum and the job's sparse checkout, or a change to it
would reuse the old image.

The jobs run in it as root, which is how GitHub runs a container. The
ones that run tests, `linux`, `fuzz`, the walks and the nightly fuzzing,
have an init process, which reaps what a test leaves behind, and the
walks the machine's IPC as well, as Playwright says to run Chromium in
one. `build` and govulncheck name the image alone, since they leave
nothing running. No job logs in to pull it, because the package is
public. `linux` and the walks mount the runners' work folder at the
place it has on the machine, because the ffmpeg we ship comes from the
cache `linux` fills, and its pkg-config files name the folder it was
built in. The one in the cache now was built on a runner, and the next,
when `scripts/build-ffmpeg.sh` changes, is built in the container, at a
place every job in the image has. From then on the mount can go. A
container runs a step with `sh`, so each job in the image sets
`bash -e {0}`, the shell a runner's step has, and tells git the
checkout, which is the runner's user's, is safe to read, since
`go build` asks git for the commit it stamps. Shard 1 saves Go's build
cache only after walks that passed, and `linux` and the fuzzing only
once their tests or their fuzzing ran to the end, passed or not, never
after a cancel. A job that stopped early has built little, and its
cache would be found first by every run after it.

A pull request from a fork pulls the image like any other, since the
package is public, but cannot push one: its token may not write
packages. A fork that changes the Dockerfile therefore fails at the job `image` until the change is on a branch of
the repository.

## Keeping up

What the app is made of and did not write is watched four ways, because a
scan made once is out of date the week after.

- **Dependabot**, `.github/dependabot.yml`, opens a pull request once a
  week when a Go module, a package of the interface, an action or a
  Python package in `requirements.txt` has a newer version, and when the
  Ubuntu image the CI image starts from has a newer build. Minor and patch versions come together, one pull request
  for the Go modules and one for the interface, and a new major version
  comes in a pull request of its own, so a major that breaks the build
  holds back nothing else. The actions come in one pull request, majors
  included. Every action is pinned to a commit, because a tag can be
  moved to other code and a commit cannot, and the comment beside it
  names the full version that commit is, `# v7.0.2` and never `# v7`.
  Dependabot moves the comment with the commit, so a bump shows which
  release it goes to. `internal/workflows` checks both, in `make unit`. With Dependabot security updates on in the repository's
  settings, it opens one at once for a fix to a known vulnerability.
  Wails and the speech library's Go module are left out, because each
  moves with a pin Dependabot cannot change. TypeScript stays below 7 until
  svelte-check, which checks the interface's types, accepts 7. An update
  needs no licence notice written, because the build writes the notices
  of the modules and packages it has. It stops only on a licence nobody
  allowed yet, see [THIRD_PARTY.md](THIRD_PARTY.md).
- **govulncheck** and **npm audit**, `.github/workflows/security.yml`, on
  every pull request that changes the Go modules or the interface's
  packages, and on main every Monday. govulncheck reports only what the
  code really calls, so a module that is listed and never compiled in, of
  which Wails brings several, is not a finding. npm audit runs through
  audit-ci, one of the interface's own packages, so
  `frontend/package-lock.json` locks it with everything it brings and
  Dependabot keeps it current like the rest. It fails on every warning, low ones too, in what ships and
  in the build tools, but for those `frontend/audit-ci.jsonc` lets through
  by number and by the path it comes in on, each with why: a warning with
  no fix yet in a tool that never ships, like deepmerge through the Svelte
  build plugin in #579. A warning that has a fix the package bringing it
  does not accept yet is fixed instead, by pinning the fixed version in
  `overrides` in `frontend/package.json`, like minimatch 11 under ESLint,
  and the pin is taken out once that package accepts the fix. Either
  failing is a red check.
- **The tools we build ourselves**, and the pins Dependabot cannot move,
  which nothing else watches: `scripts/upstream.sh` compares ffmpeg, the
  four libraries it is built with, llama.cpp, the speech library and Node
  with their newest releases, and says when the snapshot of Ubuntu the CI
  image's packages come from is a month old, and
  the same workflow keeps one issue open while anything is behind and
  closes it once everything is current. llama.cpp makes a build a day, so
  it counts only once it is a month behind. ffmpeg is the one that matters
  most: it reads a video somebody else made.
- **One pin for each tool CI installs**, #591. Node is `.node-version`,
  which every `setup-node` reads with `node-version-file` and the CI
  image's `node:` tag repeats. Dependabot cannot move a `.node-version`,
  so it leaves the image's Node alone too, `scripts/upstream.sh` says
  when a newer Node of the same major is out, and `internal/workflows` fails
  when the two differ or a workflow names a Node of its own. The CI
  image's packages come from Ubuntu's archive as it stood at the moment
  `SNAPSHOT` in the image's Dockerfile names, through Ubuntu's snapshot
  service, so the same Dockerfile builds the same image on any day. The
  Python packages CI and make install are in `requirements.txt`, each
  with the hashes of its files, and Dependabot moves them: pyarrow for the
  speech benchmark, and cmake, meson, ninja and pkg-config, which build
  ffmpeg and llama-server on a Mac. Homebrew installs only its newest
  version, so those came from it as they were on the day, and now
  `scripts/build-tools.sh` puts them in an environment of their own.
  nasm went with Homebrew: only ffmpeg's Intel code needs it.
- **GitHub's own alerts**, once switched on in the settings: Dependabot
  alerts, secret scanning with push protection, and private vulnerability
  reporting, so somebody who finds a hole has a place to say so.

An issue or a pull request is what reaches Tim, because GitHub tells
whoever watches the repository. Nothing here polls or writes a message
when nothing is wrong.

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

CI checks the same on every pull request. Its job `outside` builds with
`WERROR=1`, and then a warning fails the build where it comes from:
clang's, with `-Werror` in `CGO_CFLAGS` and `CGO_CXXFLAGS`, Apple's
linker's, with `-Wl,-fatal_warnings` in `CGO_LDFLAGS`, and the interface
build's, Vite's, Rolldown's, Svelte's and Node's, through
`frontend/vite.config.ts`, which counts every warning printed. Each is
printed as always. The few lines Go and make print as warnings of their
own are not counted. Rolldown's note that plugins took most of a build's
time is off, because it depends on how fast the machine is. Without
`WERROR` a warning is only printed, so a Mac of one's own builds on.

The programs therefore need macOS 13 Ventura or newer, which Go 1.27 needs
anyway.

## Without make

The commands make runs, for building by hand:

```
go mod tidy
(cd frontend && npm ci && npm run build)
go build -o bin/framefairy ./cmd/framefairy
go build -tags production -o bin/framefairy-app ./cmd/framefairy-app
go build -o bin/framefairy-train ./cmd/framefairy-train
sh scripts/carry-libs.sh bin/framefairy bin/lib
sh scripts/carry-libs.sh bin/framefairy-app bin/lib
```

The app is built with the `production` tag, which is what makes it a
release build to Wails. Without it, it is a debug build, where the
webview offers its own menu on every right click, with Reload, which
reloads the interface under the work in hand, and Inspect Element. A
release build offers that menu only where the Mac would, on text that is
selected or typed in. The app's own menus, like Open Link and Copy Link on
the commit on the Updates page, come either way.

The two carry-libs lines matter. Without them the two programs look for the speech
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
runs the same on the macOS runner with a recorded talk, only when it is
started by hand, from the Actions tab or through the GitHub API, so a
session in the cloud can measure a Mac without one.

A second table hears the same speech with several copies of the model at
once, each taking the next piece as it finishes one, which is how an
episode could be heard on a machine whose cores one copy leaves idle. It
tries every mix of copies, `-copies 1,2,3,4`, and threads for each,
`-each 1,2,3,4,6,8`, that fits the machine's cores, in pieces of
`-piece 20` seconds, and says how long an hour of episode would take with
each and how many words came out unlike one copy's. To measure only that,
over ten minutes so every copy has pieces enough to hear:

```sh
make speechbench AUDIO=episode.mp4 ARGS="-seconds 600 -providers cpu -threads 8 -pieces 20 -batch 1"
```

On the cloud machine, 4 cores of an x86 processor, over the model's own
sample speech looped to 90 s:

| copies | threads each | real time | an hour takes | words unlike one copy |
|---|---|---|---|---|
| 1 | 4 | 11.5 times | 314 s | 0 |
| 2 | 2 | 13.8 | 260 s | 0 |
| 4 | 1 | 14.9 | 242 s | 0 |

Four copies with one thread each heard 30 % faster than one copy with all
four, and not a word changed. On an M2 Max four copies of 2 threads heard
83 times real time where one copy of 8 heard 47, see
[ENGINE.md](ENGINE.md#hearing-the-audio-in-pieces). The line above the
table says what the app runs on the machine the benchmark runs on.
