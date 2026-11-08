# One make target per CI job, started by what changed

Issue #688, in one pull request, #758.

## The problem

Tim, 3 November: CI runs on "a hot mess" of shell glue, much of it
brittle, and its test selection hardly selects. Tim, 8 November: "why we
need any shell script to run tests, because we have a bunch of CI
workflows in GitHub and we have a make file. So what else do we need to
do other than just run a make file command either on or local machines
as engineers or inside of a job in CI so that everyone can run the same
things the same way?" Then: "if no Go code changes, no Go tests need to
run. And that translates as a principle to everything else. If no
JavaScript, TypeScript changes, we don't need to run the TypeScript
tests, probably with the exception that the walks still have to run."
And, on a first plan of six pull requests: "why can't we just create 1
pull request, rip out everything that we don't need, and just run the
damn tests in CI?", questioning what was carried over, "like this main
moved mechanism".

Measured on main at 02dd83c. 16 scripts, 1894 lines, chose, ran and
reported on tests or kept the repository's rules. `ci.yml` had 385 lines
that are not comments, and every job waited for a first job, `plan`, 20
s, which asked `scripts/changed.sh` what to run. Over the successful CI
runs of 5 to 7 November, 15 pushes to main, which run everything, and 47
pull request runs, which ran what `changed.sh` picked. Medians, the
spread half the range between the quartiles of the pull request runs:

| Measure | everything, main | picked, pull requests | change |
| --- | ---: | ---: | --- |
| time to the last job's end, s | 289 | 277 | 4 % shorter, ± 12 % |
| machine time per run, min | 32.9 | 29.0 | 12 % less, ± 21 % |

The selection bought 4 %. The walks ran in all 47 pull request runs, and
inside a Go job Go's own test cache, restored from main, already left
18 to 29 packages `(cached)`.

## Five simulations

1. **Who may know about whom.** A workflow knows only make targets, make
   only tools. Every pull request runs what main runs, no filter. Cost:
   about 14 % more Linux machine time, and Go tests on a pull request of
   the interface alone, which Tim rules out.
2. **What changes together.** The rule for when a job starts sits beside
   the job, as two path filters of one small job in `ci.yml`, Go and the
   interface. Cost: one action, `dorny/paths-filter`, and a few globs.
   A filter can miss a file, like any rule, but it is read in one look.
3. **Where state lives.** What was tested already lives in Go's build
   cache, so a Go job runs `go test ./...` and Go skips what did not
   change. Cost: nothing new, main relies on it today.
4. **What runs together.** One workflow per kind of test, each with
   GitHub's own `paths` trigger. Cost: the setup and the image job three
   times over, and three places to look for one push's answer.
5. **What a test has to build.** Keep a selector, rewritten in Go. Cost:
   a program to keep right, for 4 % of waiting.

## What converges

One make target per job, the same command everywhere, and no script that
decides or reports, came back in 1 to 4. 2 decides which jobs start and 3
which Go packages run inside them, as Tim's rule asks. 4 is dropped for
its repeated setup, 5 for buying 12 s with a program.

Asked of everything else the old CI carried, what it is for and whether
a tool already does it:

| What | Kept for | Now |
| --- | --- | --- |
| `changed.sh`, its test, `make changed`, the job `plan` | picking parts and packages | the two filters and Go's cache |
| `test-failures.sh` and its test | naming a failed test on the check | gotestsum's summary, which `make unit` already ran, ends the log with it |
| `leftovers.sh` | naming programs a test left running | removed: GitHub ends them with the job, and the job's time limit ends a hang |
| `machine.sh` | the Mac's speed beside its test times, #592 | removed, #592 is closed |
| `fuzz.sh` | running every fuzz target side by side | a recipe in `make fuzz`, with `git grep` and `xargs` |
| `pins.sh` | every action pinned to a commit, one Node | `internal/workflows`, the Go test that already read every workflow |
| `plan-frozen.sh` and its test | `docs/GUI-PLAN.md` never changes | one line of `make lint`, its git hash |
| `lint-test.sh` and its fixtures | that the linters' settings catch a wait on the clock | removed: a test of the tools' own settings |
| `main-moved.sh`, its test, `main-moved.yml` | a comment when a pull request stops merging | removed: GitHub shows a conflict on the pull request, and a session checks before it says ready |
| `ci-packages.sh` | the CI image's system packages | in the Dockerfile |
| the steps every job repeated | Go, Node, the cache, the ffmpeg we ship | `.github/actions/prepare`, once |

What stays is the build, not the CI: the scripts that configure, compile
and pack ffmpeg, llama-server and the app, the setup of cloud sessions,
and `needs-build.sh` and `channel-list.sh`, which decide what reaches the
Updates page.

## The decision

```
ci.yml job      machine, the checkout, .github/actions/prepare, and when
                it starts: Go, the interface, or either
  make target   lint, interface, unit, fuzz, walks, outside, the build
    tools       go test through gotestsum, Vitest, ESLint, golangci-lint,
                Playwright, clang and the linker
```

Each layer uses only the one under it. Go is every file outside
`frontend/`, the docs and `.claude/`, so a script, the Makefile or a
workflow counts as Go. The interface is `frontend/`, the Makefile, the
workflows and `.node-version`.

| Job | Starts on a pull request when |
| --- | --- |
| `interface` | the interface changed |
| `build`, `walks`, `outside` | Go or the interface changed |
| `linux`, `fuzz`, `macos` | Go changed |

A push to main starts every job. A change to the docs alone starts none.
A warning fails the Mac build through the compiler, the linker and Vite,
`WERROR=1`, not a grep. The no-warning rule moved out of `changed.sh`
into the tools in the first commit of this pull request.

## What follows

Of the 16 scripts, 2 are left, `needs-build.sh` and its test, which the
builds for the Updates page use. `ci.yml` has 220 lines that are not
comments where it had 385, and the setup they shared, 82, is written
once. No job waits for a 20 s `plan` any more: the filter answers in
about 8 s. A pull request of the interface alone starts no Go job, one of
the docs alone none at all.

It costs a filter that can miss a file, which shows as a test that
passes on its pull request and fails on main. It costs `internal/naming`
on pull requests that change only the interface or the docs, since it
reads both, and runs on main after the merge. It costs the proof that the
linters catch a wait on the clock, and the check that a test leaves no
program running. It was wrong if pull requests come back slower than
before, if main fails on what its pull request passed more than once a
week, or if a script that chooses or reports a test comes back.
