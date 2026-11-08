# One make target per CI job, started by what changed

Issue #688. Six pull requests carry it out, the first is #758, and each
of the others names this record.

## The problem

Tim, 3 November: CI runs on "a hot mess" of shell glue, much of it
brittle, and its test selection hardly selects. Tim, 8 November: "why we
need any shell script to run tests, because we have a bunch of CI
workflows in GitHub and we have a make file. So what else do we need to
do other than just run a make file command either on or local machines
as engineers or inside of a job in CI so that everyone can run the same
things the same way?" And all of it "drastically simplified while making
sure that the tests run fast and the coverage remains high."

Measured on main at 02dd83c. 16 scripts, 1894 lines, choose, run and
report on tests or keep the repository's rules: `changed.sh` and its
test, `test-failures.sh` and its test, `leftovers.sh`, `machine.sh`,
`fuzz.sh`, `lint-test.sh`, `pins.sh`, `plan-frozen.sh` and its test,
`needs-build.sh` and its test, `main-moved.sh` and its test, and
`ci-packages.sh`. `ci.yml` has 385 lines that are not comments.

Over the successful CI runs of 5 to 7 November, 15 pushes to main, which
run everything, and 47 pull request runs, which run what `changed.sh`
picked. Medians, the spread half the range between the quartiles of the
pull request runs:

| Measure | everything, main | picked, pull requests | change |
| --- | ---: | ---: | --- |
| time to the last job's end, s | 289 | 277 | 4 % shorter, ± 12 % |
| machine time per run, min | 32.9 | 29.0 | 12 % less, ± 21 % |
| Mac time per run, min | 6.2 | 4.6 | 26 % less |

The walks ran in all 47 pull request runs and are the longest job. The
`plan` job that asks `changed.sh` takes 20 s, and every job waits for
it. In the last push to main each of the four Go jobs reran only
`engine`, `cmd/framefairy-app` and a few small packages, and got 18 to
29 packages back from Go's test cache.

## Five simulations

### 1. Who may know about whom

A workflow knows only make targets, make knows only tools, nothing sits
between them. Every pull request runs what main runs. Layers: job,
target, tool. What moves: `changed.sh` and the `plan` job go, every job
calls its target. Names that cross: the target's name and its variables,
`UNITSHARD`, `WALKSHARD`. Next changes: a new kind of test is a target
and a job. Cost: about 14 % more Linux machine time, projected from the
47 runs, and Go tests on a pull request that changed no Go.

### 2. What changes together

The rule for when a job runs sits beside the job, as a path filter in
`ci.yml`, `dorny/paths-filter`: one filter for Go, one for the
interface, and the walks for either. Layers: the same three, with a
small `changes` job that the jobs with a filter wait for. Names that
cross: two outputs, `go` and `interface`. Next changes: a new kind of
file is a glob. Cost: one action and two lines of globs. A filter can be
wrong like any rule, but it is two lines, read in one look.

### 3. Where state lives

What was tested already lives in Go's build cache, which CI restores
from main, so `go test ./...` skips every package whose inputs did not
change. Layers: the same three. Nothing moves, main relies on it today.
Names that cross: none. Cost: nothing new. It is what makes running all
Go packages in a Go job cheap.

### 4. What runs together

One workflow per kind of test, each with GitHub's own `paths` trigger,
so no filter action. Layers: three workflows, each with its jobs. What
moves: `ci.yml` splits in three. Names that cross: the image job's
outputs, called three times. Next changes: a new kind of test is a
fourth workflow. Cost: the setup and the image job three times over,
and three places to look for one push's result.

### 5. What a test has to build

Keep a selector, rewritten in Go and tested with `go test`, so `make
changed` still exists everywhere. Layers: job, `make changed`, the
selector, target, tool. Names that cross: the parts and the shares the
selector prints. Next changes: every new kind of file is a case in the
selector and in its test. Cost: a program to keep right, for 4 % of
waiting.

## What converges

One make target per job, the same command on a Mac, in a cloud session
and in CI, and no script that decides, came back in 1, 2, 3 and 4. Go's
cache picking the packages inside a Go job came back in 1 and 3. Only 5
keeps a selector of its own, and the measurement says it buys 12 s, so
it is dropped. Between 1 and 2, Tim, 8 November: "if no Go code changes,
no Go tests need to run. And that translates as a principle to
everything else. If no JavaScript, TypeScript changes, we don't need to
run the TypeScript tests, probably with the exception that the walks
still have to run", because the walks test the whole app with its Go
side. So 2 decides which jobs start, and 3 which Go packages run in
them. 4 is dropped for its repeated setup.

## The decision

```
ci.yml job      machine, caches, and when it starts: Go, the interface, or either
  make target   lint, interface, unit, fuzz, walks, outside, the build
    tools       go test through gotestsum, Vitest, ESLint, golangci-lint,
                Playwright, clang and the linker
```

Each layer uses only the one under it. Go is every file outside
`frontend/` and the docs, so a script, the Makefile or a workflow counts
as Go. The interface is `frontend/`, the Makefile, the workflows and
`.node-version`. The jobs:

| Job | Runs on a pull request when |
| --- | --- |
| `interface` | the interface changed |
| `build`, `walks`, `outside` | Go or the interface changed |
| `linux`, `fuzz`, `macos` | Go changed |

A push to main runs every job. A change to the docs alone runs nothing.

The six pull requests:

1. Warnings fail the build through the compiler, the linker and Vite,
   `WERROR=1`, not a grep in `changed.sh`.
2. Every job runs its make target, started by the two filters. `plan`,
   `changed.sh` and its test, and `make changed` go. The row "add a
   check to CI" in ARCHITECTURE.md changes with it.
3. Failures are named on the check by gotestsum's JUnit file and
   `mikepenz/action-junit-report`. `test-failures.sh`, `leftovers.sh`
   and `machine.sh` go.
4. One local composite action prepares every job.
5. The repository's rules are tests: `pins.sh` in `internal/workflows`,
   `plan-frozen.sh` and `lint-test.sh` as Go and Vitest tests, `fuzz.sh`
   a Go program.
6. The Main moved notice through
   `eps1lon/actions-label-merge-conflict`, `needs-build.sh` into
   `framefairy-release`, `ci-packages.sh` into the Dockerfile.

The scripts that configure, compile and pack ffmpeg, llama-server and
the app stay shell. They are the build, not the CI.

## What follows

It costs a filter that can miss a file, which shows as a test that
passes on its pull request and fails on main. It costs `internal/naming`
on pull requests that change only the interface or the docs, since that
Go test reads both and runs on main after the merge. Projected from the
47 runs, machine time stays where it is, 28.8 min median against 29.0,
and a pull request comes back about 12 s sooner, because the filter
takes about 8 s where `plan` took 20. Each pull request measures its own
CI time against main. It was wrong if pull requests come back slower, if
main fails on what its pull request passed more than once a week, or if
a script that chooses or reports a test comes back.
