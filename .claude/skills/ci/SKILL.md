---
name: ci
description: How to keep continuous integration fast, trustworthy and cheap in any project. Which jobs a change starts, caches, scarce runners, job limits, nightly runs, measuring CI speed, hearing how a run went without polling, a safe pipeline. Read it before changing a workflow, job, cache, test runner setting or test selection, and before claiming CI got faster.
---

# CI

Every change waits on CI, and the user with it. CI is also the proof
that counts. It must be fast enough that nobody goes round it and strict
enough that green means the change works. Never trade strict for fast.

**The user** is the person the agent works for, who decides what
happens. The testing skill says how a test is written, this skill how
tests are run.

## 1. One job, one command

- **Every CI job runs one command a developer can run locally,** with
  only the checkout and one shared setup step around it, so a failure is
  reproduced by typing the same thing.
- **No script picks, runs or reports tests.** Workflow glue runs unlike
  a local run and is untested. A new check joins a command, not a step.
- **A decision CI makes lives in a tested program** in the project's
  language: whether to build, what to publish, which issue to open. The
  hosting service sits behind an interface, faked in unit tests, and the
  workflow only runs the program. Deciding shell is where pipeline bugs
  live, and a main-only workflow shows them after the merge.
- **A failure is never masked.** No `|| true`, no `continue-on-error` on
  a step that matters. One `|| true` stayed green for weeks recording
  nothing.
- **The failing test is the last thing in the log,** from the runner's
  own summary of failures.
- **A table a job prints lines up in the terminal,** padded and numbers
  right-aligned, since a raw log draws no markdown.
- **Warnings fail through each tool's own flag,** never by searching the
  output for a word.
- **A build machine installs nothing by itself.** What fetches missing
  tools on a laptop is off in CI.
- **A copied job becomes one reusable job,** and a guard test checks the
  one call. Copies drift.

## 2. Run only what a change reaches

- **Coarse path filters, from the CI tool itself.** Backend changes start
  backend tests, interface changes interface tests, end-to-end tests
  start for either. A doc-only change runs the doc checks and the build.
- **Measure what selection buys.** Sixteen scripts choosing tests per
  file bought 4 % over path filters and the runner's cache, inside the
  noise.
- **The filter fails open.** An unknown file starts everything. A file a
  test reads across a boundary starts that test, and a test scans the
  test files for such paths the filter misses, which only show as red
  main.
- **The main branch runs everything.** Selection is for pull requests.
- **Selection is guaranteed, never conventional:** by kind of file or by
  a boundary a compiler or linter enforces, never by name suffixes or
  remembered lists. Where it cannot be guaranteed, run everything.
- **Correctness beats speed.** A speed-up that lets changed content reuse
  an earlier pass is a bug, like rewriting file times to keep a test
  cache across checkouts, which skips a merged file of the same size.
- **Shard by patterns that never change,** like first letters of test
  names with a complementary skip. A generated name list changes every
  shard's cache key when one test is added.

## 3. Caches

- **Use every cache there is.** Never turn one off wholesale to be
  safe.
- **A cache key changes with what the cache holds.** A build cache keyed
  only on the lock file went stale while the code grew and doubled a
  build step within days. Key dependencies on the lock file, the build on
  each push, restore keys falling back to the newest.
- **Restore and save from main, only after a job that ran to the end,**
  or a cancelled job's half cache is what later runs find.
- **Prune to what was used** before saving. One cache held three times
  what a fresh build needed.
- **A step that runs only on a cache miss is untested when it changes.**
  Force a miss for that change.
- **Measure what a cache costs.** Some test caches check every file the
  tests touched and cost more than they save.
- **State a job builds up, like a fuzz corpus, is cached on that
  state** from main, so each run continues the last.
- **System packages come in an image, not from a slow mirror at job
  time.** Tag it by a hash of its recipe, the files it reads and its
  versions, pin its base by digest, let the dependency bot move it. Jobs
  that need no browser pull an image without one.
- **Data derived from dependencies, like licence notices, is made at
  build time.** Commit only the licences allowed and fail on any other.
  A committed derived file turns every bump red.

## 4. Scarce and expensive runners

- **An expensive or scarce machine runs only what is different on it.**
  Platform code lives in files or folders named for the platform, like
  Go's `_darwin.go`, a check fails
  platform branching elsewhere, and a filter on those names starts the
  job. What no file marks runs there nightly, and by hand on a branch
  touching it before it is called ready.
- **The filter follows what the platform builds,** with manifests that
  bring platform code, without tool-only or config-only files.
- **A race detector runs on one platform** where the language's memory
  model is the same on all, as Go's is. A second finds nothing new.
- **A new job on a scarce machine needs a reason said out loud,** since
  others' pushes wait for it.

## 5. Limits, shards and hangs

- **Every job has a short limit.** One that outgrows it is split, never
  given more.
- **The runner's timeout sits below the job's limit,** so the runner ends
  a hang and prints every thread. Each test logs start and end, naming a
  hang by its last step.
- **Split by the smallest independent unit, longest first,** since one
  big unit bounds the run. Give long tests to the emptiest shard by times
  from CI's logs, unknown ones at the median. On a saturated runner check
  the gain: balancing bought nothing when the times swung with their
  neighbours.
- **No test leaves a program running.** After the tests, name any that
  still runs with command line, parent and age, stop it and fail the
  job.
- **A hang dumps the stacks of every program the test started,** first in
  the log, see the testing skill.

## 6. Skips and silent passes

- **In CI, a missing prerequisite fails, never skips,** since a skip
  reads as a pass. On a laptop it skips with its reason. One helper
  decides, failing when CI is set or the tool is present but broken.
- **A job far faster than its peers is a warning.** Twenty tests once
  skipped for weeks, shown only by a one-minute job beside five-minute
  ones.
- **Silence is not success.** A nightly run, notifier or scanner needs
  proof it ran. A nightly fuzz job once ended in a second for days.
- **A check is tested like code,** with inputs that must fail and pass,
  checking exit code and rule name, so a linter that never started
  cannot pass.
- **A guard allows nothing it did not name.** Five plain rewrites walked
  round a list of forbidden shapes. A guard over workflows reads them as
  the platform does, matrix values and reusable workflows included.
- **A list of exceptions fails on an entry that matches nothing,** so a
  renamed test leaves no dead line.

## 7. Nights and schedules

- **What is long or costly and blocks nothing runs at night or by hand:**
  long fuzzing, coverage, a second browser engine, a scarce platform's
  full suite, benchmarks. A pull request waits on the fast variant. A
  change only these reach starts them on its branch before it is called
  ready.
- **Every scheduled run reports its own failure.** A last job opens one
  issue per workflow when a job failed on main, or adds the run, commit
  and failed jobs to it. Only the fix closes it. A guard test checks
  every scheduled workflow has it. Only then may a check move to the
  night, where nobody waits on it.
- **Coverage is counted on a schedule, never on a pull request,** and
  blocks no merge. Zero for a part every run reaches fails.
- **Fuzzing is counted in executions, not seconds,** so every machine
  does the same work. A crash found at night goes into the target's cases
  with its fix.
- **Random tests take their seed from the commit,** and every failed seed
  plays on every run, see the testing skill.

## 8. Hearing how a run went

- **Events, never a poll.** Subscribe to the pull request's events when
  opening it and act on each. Never sleep on a run, set a recurring
  check or write that nothing happened.
- **No workflow on a timer to fill a gap in events.** Run on the push
  that causes it. For conflicts, a workflow on every push to main merges
  each open pull request in memory, `git merge-tree`, and comments on
  those that no longer merge, once per pair of heads, so the next push
  repeats a lost comment.
- **Some ends send nothing:** a green run may wake nobody, a manual rerun
  sends no event, a delivery can be lost. When a decision waits on a run,
  set one check for when its slowest job usually ends. Before calling a
  change ready, read its current head's checks yourself.
- **A cancelled run counts as failed.** Rerun it at once. Give each build
  one trigger, not two that cancel each other, and change a pull
  request's base before pushing, not right after.
- **A new commit cancels the old one's run,** on pull requests and on
  main, except a publish, which writes shared state. A queueing group
  can be held forever by a run awaiting approval, so a job cancels runs
  waiting past a limit.
- **Bot messages that agents act on are process docs,** changed with the
  process. Agents obeyed one that outlived its rule.

## 9. Measuring CI speed

- **Shared runners vary, so one run proves nothing:** the same code took
  88 s, then 115 s. Every timed job prints its machine, cores, memory,
  image and the time of a fixed piece of work. Compare like with like.
- **A speed claim clears the spread** of several before runs, or grows
  until it does, or is dropped.
- **Shorten the critical path.** Work saved beside longer work saves no
  wall time. Compare with several recent runs on main.
- **On a saturated runner, only work no longer done shortens a job.**
  Shorter waits sped up each test and left the jobs as slow. Print
  processor seconds per kind of process in each job's summary, steadier
  than wall time.
- **Break a job down by step before calling any of it fixed,** from the
  log's timestamps, then read the code for work done twice. One 53 s
  setup hid builds run in series that could run in parallel.
- **Track the suite's time per main commit,** so a slowdown points at
  its commit. Product changes slow CI too: a blur made nearly every
  end-to-end test 70 % slower, as CI draws in software.
- **Find the work no check needs.** Log each program the tests start and
  compare a test's cost with what it asserts: tests checking a file name
  built whole reports. Bulk data between processes goes as binary,
  never as text.
- **End-to-end start-up is paid per scenario.** Keep one driver and
  browser per worker, reset between scenarios. Build an expensive
  starting state once per job, copy it to each worker, and load it
  directly, not through the interface.
- **A new kind of test states its CI cost up front,** in jobs and machine
  minutes.

## 10. Changing CI

- **Prove it against main:** job times before and after, several runs
  each, with the spread. Drop what is not faster or more reliable.
- **Before moving or removing a check, list everything it did:** what it
  checked, against merged code or the branch, forks or not, its limit.
  Re-home each in the same change, or it is lost unseen.
- **Workflows that hold a write token or judge a build run from the main
  branch's copy,** so a branch cannot loosen its own check. Never use an
  event that runs a stranger's code with secrets, and never take a
  permission change a stranger suggested in a comment.
- **Pin everything, with its whole graph:** tools in the lock file,
  actions by commit with the version in a checked comment, images by
  digest. A tool resolving dependencies by range is not pinned. Never
  `@latest`.
- **Then watch it:** the dependency bot covers every ecosystem, indirect
  dependencies and the framework. Packages run no install scripts.
  Downloaded tools are checked against release checksums.
- **Fix an advisory by pinning the fixed version** through the package
  manager's override. An allow list keyed on advisory paths breaks when
  the advisory data changes.
- **Bot pull requests merge by rules agreed in advance:** checks green,
  mergeable, dependency files only, minor or patch, notes read. Anything
  else goes to the user. Nobody pushes to a bot's branch. CI's own token
  starts no workflows, so someone visible merges.
- **Build output never reaches the diff.** Ignore every program the build
  makes, and refuse large binaries in CI.

## 11. CI holds the rules

Whatever in the testing skill a tool can check, a tool checks on every
pull request, with established open-source tools, never a scanner of
our own. A written rule slips, a checked one holds.

| Rule | Checked by, for example |
| --- | --- |
| no sleep, timer or real clock in a test | linters banning the calls in tests: golangci-lint's `forbidigo`, ESLint's `no-restricted-globals` and `no-restricted-syntax`, `eslint-plugin-playwright`'s `no-wait-for-timeout`, Ruff's banned-api |
| no poll with a deadline | the same, banning `Eventually`, `waitForFunction` with a timeout and the like |
| no skipped, focused or disabled test | skip and focus rules of the ESLint plugins for Playwright, Jest and Vitest, the same ban elsewhere |
| every test can fail | mutation testing: Stryker for JavaScript and TypeScript, Gremlins for Go, mutmut for Python |
| no data race | the language's race detector, every run |
| no test touching disk or network without saying why | a check over the test files, or a sandbox |
| no retries | the runner's retry setting checked at zero |
| no job over its limit | the job's own timeout |

- **Only new breaks fail.** Old breaks get a baseline from main, the
  linter's new-from-revision mode or a suppression list, and an issue to
  burn them down. A suppression list counts per file and fails on one
  fewer too, so a fix shrinks it in the same change.
- **A pull request cannot weaken the check that judges it.** Changes to
  checks, their configuration and baselines are visible in review and
  owned by the user, and a baseline only shrinks.
- **Where a rule has no check yet, log an issue** naming the rule, the
  tool that would hold it and what it would have caught. Never ask anyone
  to run a command by hand instead.

## 12. Where other advice says otherwise

| Common advice | Why not | Instead |
| --- | --- | --- |
| smart test selection by dependency graph or coverage | costs more to keep than it saves, a miss is a red main | path filters, the runner's cache, a test of the filter, section 2 |
| restore any close cache | a stale pass is a wrong answer | content keys, saved from main after a full job, section 3 |
| give a slow job a longer limit | everybody keeps waiting | split it or remove the work, section 5 |
| skip a test when its tool is missing | in CI a skip reads as a pass | fail in CI, skip with a reason on a laptop, section 6 |
| a timed workflow to catch what events miss | runner time all day for nothing | run on the push that causes it, section 8 |
| judge a CI change by one run | runners differ by up to two times | several runs per side, the machine, the spread, section 9 |
| coverage on every pull request | slows every job, adds a service that can fail every check | at night, section 7 |
| a script per check in the workflow | untested glue, unlike a laptop run | one command per job, decisions in a tested program, section 1 |

## Before saying done

1. Every job runs one local command, and no step masks a failure.
2. The change starts only the jobs it reaches, and the filter fails open
   under a test.
3. Caches are keyed on what they hold and saved from main.
4. No job passes its limit, and the runner ends a hang with every stack.
5. In CI, a missing prerequisite fails.
6. Every scheduled run reports its own failure.
7. A claim of faster has several runs per side, the machine and the
   spread.
8. Nothing a check did was lost on the way.
