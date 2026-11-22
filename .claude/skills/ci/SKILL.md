---
name: ci
description: How to keep continuous integration fast, trustworthy and cheap in any project. Which jobs a change starts, caches, scarce runners, job limits, nightly runs, measuring CI speed on shared runners, hearing how a run went without polling, and keeping the pipeline safe. Read it before changing a workflow, a CI job, a cache, a test runner's settings or anything that decides which tests run, and before claiming CI got faster.
---

# CI

CI is where every change waits, and the user waits with it. It is also
the proof that counts: a rule only holds once a machine checks it on
every change. So CI has to be two things at once, fast enough that
nobody is tempted to go round it, and strict enough that green means
the change works. Everything below serves one of the two, and nothing
below trades the second for the first.

**The user** is the person the agent works for, who decides what
happens. The testing skill says how each test is written. This skill
says how the tests are run.

## 1. One job, one command

- **Every CI job runs one command a developer can run on their own
  machine,** with only what its machine needs around it: the checkout,
  one shared setup step and that command. The same command on a laptop,
  in an agent's session and in CI. A failure in CI is then reproduced by
  typing the same thing.
- **No script picks, runs or reports tests.** Glue in a workflow runs
  differently from a local run, cannot be tested, and redoes what the
  tools already do. A new check joins a command, not a workflow step.
- **A decision CI makes lives in a tested program,** in the project's
  own language: whether to build, what to publish, which issue to open.
  The hosting service sits behind an interface the program is handed, and
  plain unit tests drive it with a fake. A workflow step only runs the
  program, in a line or two. Shell that decides things is where the bugs
  of a pipeline live, and a workflow that runs only from the main branch
  shows its mistakes only after the merge.
- **A failure is never masked.** No `|| true`, no `continue-on-error` on
  a step whose result matters. A step that ended in `|| true` once
  recorded nothing for weeks and stayed green.
- **The failing test is the last thing in the log.** Use the test
  runner's own summary, which repeats every failure at the end, so the
  person or agent reading a red check sees the test and what it printed
  without scrolling.
- **Warnings fail through each tool's own flag:** the compiler's
  warnings as errors, the linker's fatal warnings, the bundler's count of
  warnings. Never by searching the output for a word.
- **A build machine installs nothing by itself.** What a job has is what
  its workflow asked for, so a convenience that installs a missing tool
  on a laptop is turned off in CI.
- **A copied job becomes one reusable job.** The same job in four
  workflows is four edits for every change and four chances to drift. A
  guard test then checks the one call, not each copy.

## 2. Run only what a change reaches

- **Coarse path filters, from the CI tool itself.** A change to the
  backend starts the backend's tests, a change to the interface starts
  the interface's, and the end-to-end tests start for either. A doc-only
  change runs only the checks that read docs, and the build. Prefer the
  tool's own filter to a script of ours.
- **Measure what selection buys before keeping it.** Sixteen scripts that
  chose tests file by file bought 4 % against a couple of path filters
  and the test runner's own cache, inside the noise. The cache already
  skips most of what did not change.
- **The filter fails open.** A file it does not know starts everything.
  A file a test reads across a boundary, a doc, a fixture, a file of the
  other side, starts that test. A test scans the test files for the paths
  they read and fails when the filter misses one, because a miss shows
  only as a test that passes on its pull request and fails on main.
- **The main branch runs everything.** Selection is for pull requests.
- **Selection is guaranteed, never conventional.** Jobs are picked by
  kind of file, or by a boundary a compiler or a linter enforces, like
  platform code kept in files named for the platform. Never by test name
  suffixes or lists someone has to remember. Where selection cannot be
  guaranteed, everything runs.
- **Correctness beats speed.** A speed-up that could let changed content
  reuse an earlier pass is a correctness bug, however much it saves. A
  change once rewrote file times so a test cache would survive fresh
  checkouts, and a merged file kept its old size and time, so its test
  would not have run at all. The diff already says what changed.
- **Shards are split by patterns that never change,** like the first
  letters of test names with a complementary skip so every test runs
  once. A generated list of names changes every shard's list when one
  test is added, and the cache is keyed on that list.

## 3. Caches

- **Use every cache there is,** for dependencies, the build and test
  results, and never turn one off across the board to be safe.
- **A cache key changes with what the cache holds.** A build cache keyed
  only on the lock file is saved once and then restored for weeks while
  the code grows, and a job recompiles almost everything. One such cache
  doubled a build step within days. Key the dependency cache on the lock
  file and the build cache on each push, with restore keys that fall back
  to the newest.
- **Restore from main, save from main, and only after a job that ran to
  the end.** A cancelled job's half cache is otherwise the first one
  every later run finds.
- **Prune to what was used.** Mark what was restored as old, and before
  saving drop what the job did not touch, or the cache keeps everything
  ever built. One restored three times what a fresh build needed.
- **A step that runs only on a cache miss is untested when it changes.**
  Force a miss for the change that touches it.
- **Measure what a cache costs.** On some systems a test cache checks
  every file the tests touched and costs more than it saves.
- **A fuzz corpus or other state a job builds up lives in a cache keyed
  on that state,** saved from main, so each run goes on from where the
  last stopped instead of finding the same easy inputs again.
- **System packages come in an image, not from a mirror at job time.**
  Build one image from a recipe, tag it by a hash of the recipe and the
  versions in it, pin its base by digest, and let the dependency bot move
  it. A file the recipe reads joins the hash. Slow mirrors once held jobs
  for twelve to fifteen minutes. Jobs that need no browser pull an image
  without one.
- **Data derived from the dependencies is made at build time,** licence
  notices for example. Commit only the decision, the licences allowed,
  and fail the build on anything else. A committed derived file turns
  every dependency bump red.

## 4. Scarce and expensive runners

- **An expensive or scarce machine runs only what is different on it.**
  Platform code goes in files named for the platform, a check fails code
  that branches on the platform anywhere else, and a filter on those
  names starts the platform's job. What no file marks as the platform's
  runs on that platform every night, and on a branch whose change
  touches it, started by hand before the pull request is called ready.
- **The filter follows what the platform builds,** dependency manifests
  that bring platform code included, and keeps tool-only and config-only
  files out.
- **The race detector runs on one platform.** The same code finds nothing
  new on a second and costs most of a job again.
- **A new job on a scarce machine needs a reason that can be said out
  loud.** Each one is a push of someone else's that waits.

## 5. Limits, shards and hangs

- **Every job has a short limit, and a job that grows past it is split,**
  never given a longer limit.
- **The test runner's own timeout sits below the job's limit,** so the
  runner, not the job, ends a hang and prints every thread when it does.
  Each test or scenario says when it starts and ends, so a hang is named
  by its last step.
- **Split by the smallest independent unit, longest first.** One unit
  that holds most of the work bounds the whole run. Share long tests
  across shards by their recorded times, longest first to the shard with
  the least, unknown ones at the median, with times read from CI's own
  logs, not from a laptop. On a saturated runner, check the gain: shares
  balanced from recorded times bought nothing when the times themselves
  swung with what ran beside them.
- **No test leaves a program running.** After the tests, look for any
  program a test started that still runs, name it with its command line,
  parent and age, stop it and fail the job. A leftover child once held a
  job for half an hour without saying which test started it.
- **A hang dumps what it waited on.** Before the runner ends a hung test,
  the stacks of every program the test started are written out, not only
  the test's own threads, and first in the log. One such dump ended a
  chain of guesses about a hang.

## 6. Skips and silent passes

- **In CI, a missing prerequisite fails, never skips.** A test may skip
  with its reason on a laptop without the tool. In CI a skip reads as a
  pass, since nobody lists skips. Route every such skip through one
  helper that fails when CI is set, and fails on a tool that is there but
  broken.
- **A job far faster than its peers is a warning.** Twenty tests once
  skipped for weeks on one platform, and the only sign was a job of one
  minute beside one of five.
- **Silence is not success.** Anything whose silence means all is well,
  a nightly run, a notifier, a scanner, needs proof that it ran. A
  nightly fuzz job ended within a second every night for days, because a
  check before it failed, and never fuzzed.
- **A check is tested like code.** A new lint rule or guard comes with
  inputs that must fail and inputs that must pass, and the test checks
  the exit code and the rule's name, so a linter that did not start
  cannot pass.
- **A guard allows nothing it did not name.** It lists the shapes that
  are allowed and fails on the rest. A list of forbidden shapes is walked
  round by ordinary code: five plain rewrites of a forbidden comparison
  passed one such check. A guard over every workflow reads them the way
  the platform does, matrix values and reusable workflows included.
- **A list of exceptions fails on an entry that matches nothing,** so a
  renamed test cannot leave a line behind that does nothing.

## 7. Nights and schedules

- **What is long or costly and blocks nothing runs at night or by
  hand:** long fuzzing, coverage, a second browser engine, the full suite
  of a scarce platform, benchmarks. A pull request waits only on the
  fast variant. A change that only those reach starts them on its branch
  before it is called ready.
- **Every scheduled run reports its own failure.** A last job runs when
  another failed on the main branch and opens one issue for that
  workflow, or adds the run, the commit and the failed jobs to the one it
  opened. It never closes the issue on a later pass, the fix does. A
  guard test checks every scheduled workflow has that job. Moving a
  check to the night is safe only once this is in place, because nobody
  waits on a nightly run.
- **Coverage is counted on a schedule, never on a pull request,** and
  never blocks a merge. A count of zero for a part every run is known to
  reach fails, because it means the counting broke.
- **Fuzzing is counted in executions, not seconds,** so a slow machine
  does the same work as a fast one. A crash found at night goes into the
  target's cases with its fix, and every run plays it from then on.
- **Random tests take their seed from the commit,** so every push
  explores new ground and a failure is reproduced from its commit. Every
  seed that ever failed is kept and played on every run.

## 8. Hearing how a run went

- **Events, never a poll.** An agent subscribes to its pull request's
  events when it opens it and acts on each as it comes. It never sleeps
  on a run, never sets up a recurring check and never writes a message
  that says nothing happened.
- **No workflow on a timer to fill a gap in events.** When the hosting
  service sends no event for something, a pull request starting to
  conflict for example, find the push that causes it and run there. A
  workflow on every push to main can merge each open pull request in
  memory, `git merge-tree`, and comment on those that no longer merge,
  once for each pair of the branch's head and main's head, so one lost
  comment is followed by the next push's.
- **Some ends send nothing.** A green run may wake nobody, a job rerun by
  hand sends no event and a delivery can be lost. When a decision waits
  on a run, set one check for when its slowest job usually ends, not a
  loop. Before saying a change is ready, read the checks of its current
  head yourself, because an event may be about an older commit.
- **A cancelled run counts as failed** on the hosting service. Rerun it
  at once. Give each build one trigger, not a push trigger and an opened
  trigger that cancel each other, and change a pull request's base
  before pushing, not right after.
- **A run for an old commit is cancelled when a new one comes,** on pull
  requests and on main. Only what writes shared state, a publish, is
  never stopped halfway. A group that queues instead can be held forever
  by one run waiting on an approval, so a job cancels a run that has
  waited past a limit.
- **Messages a bot posts that agents act on are process docs.** When the
  process changes, search for them and change them in the same change.
  One bot message still told agents to run the whole suite after the
  rule had changed, and it was the one they obeyed.

## 9. Measuring CI speed

- **Shared runners vary, so one run proves nothing.** One runner label
  can serve two kinds of machine, and the same code took 88 s on one run
  and 115 s on the next. Every timed job prints its machine, cores,
  memory, image, and how long a fixed piece of work took that day. Read
  times against it, compare like with like, and measure several runs on
  each side.
- **A speed claim clears the spread.** The before is measured more than
  once, and a change smaller than its spread is no gain. Bundle or grow
  the change until it clears it, or drop it.
- **Shorten the critical path.** Work skipped beside longer work saves
  no wall time. Compare against the last several runs on main.
- **On a saturated runner, only work no longer done shortens a job.**
  When parallel workers keep a machine near full, a test that waits less
  only lets another use the time. Measure processor seconds by kind of
  process, which vary less than wall time, and print them in each job's
  summary. Making every wait an event made tests faster one by one and
  left the jobs as slow as before.
- **Break a job down by step before calling any of it fixed.** Split its
  time from the log's timestamps: pulling the image, the toolchain, each
  build, the tests. Then stop timing and read the code for work done
  twice. Setup reported as one lump of 53 s hid builds that ran one after
  another and could run side by side.
- **Track the suite's time for every main commit,** so a slowdown points
  at the commit that made it. A product change slows CI as often as a
  test change: a blur added to the interface made nearly every end-to-end
  test 70 % slower, because CI draws in software.
- **Find the work no check needs.** Put a logger in front of each program
  the tests start and compare each test's cost with what its assertion
  reads. Tests that checked a file name rendered whole videos. Bulk data
  a test reads across a process boundary travels as binary, never as a
  list of numbers in text.
- **End-to-end start-up is paid per scenario.** Keep one driver and one
  browser per worker across scenarios, with a clean reset between them,
  prepare an expensive starting state once per job and copy it to each
  worker, and start a scenario that only needs data from that state
  instead of building it through the interface.
- **A new kind of test says its CI cost up front,** in jobs and machine
  minutes, and is weighed like any other cost.

## 10. Changing CI

- **Prove it against main, with numbers.** A change to CI shows its job
  times before and after, several runs each, with the spread. What does
  not measure faster or more reliable is dropped.
- **Before moving or removing a check, list everything it did.** What it
  checked, what it ran against, the merged code or the branch, forks or
  not, its limit. Re-home each item in the same change. A local script
  once was the only place a kind of file was parsed, and removing it
  dropped the check unseen.
- **A pull request cannot weaken the check that judges it.** Workflows
  that hold a write token, or judge a build, run from the main branch's
  copy. Never use an event that runs a stranger's code with secrets, and
  never take a permission change a stranger suggested in a comment.
- **Pin everything, with its whole graph.** Tools in the language's lock
  file, actions by commit with the full version as a comment that a check
  verifies, images by digest. A tool fetched by name and version that
  resolves its dependencies by range is not pinned. Never `@latest`.
- **Then watch everything you pin.** Check that the dependency bot covers
  every ecosystem, indirect dependencies and the framework itself.
  Packages run no install scripts. Downloaded tools are checked against
  their release checksums.
- **An advisory is fixed by pinning the fixed version,** through the
  package manager's override. An allow list keyed on the advisory's paths
  breaks when the advisory data changes, and turned every pull request
  red overnight once.
- **Bot pull requests are merged by rules agreed in advance:** every
  check green, mergeable, dependency files only, minor or patch, release
  notes read. A major bump, a red check, a new licence or anything odd
  goes to the user. Nobody pushes to a bot's branch. A merge made with
  CI's own token starts no workflows, so it is made by someone visible.
- **Build output never reaches the diff.** Ignore every program the build
  makes, and refuse large binaries in CI.

## 11. Where other advice says otherwise

| Common advice | Why not | Instead |
| --- | --- | --- |
| a smart test selection from a dependency graph or per-test coverage | it costs more to keep than it saves, and a miss is a red main | coarse path filters, the runner's cache, a test of the filter, section 2 |
| restore any cache that is close, to be fast | a stale or partial cache costs more than none, and a stale pass is a wrong answer | keys that change with the content, saved from main after a full job, section 3 |
| give a slow job a longer limit | slow jobs stay slow and everybody waits | split it or remove the work, section 5 |
| skip a test when its tool is missing | in CI a skip reads as a pass | fail in CI, skip with a reason on a laptop, section 6 |
| a scheduled workflow to catch what events miss | runner time spent all day for nothing | run on the push that causes it, section 8 |
| judge a CI change by one run | runners differ by up to two times | several runs each side, the machine printed, the spread, section 9 |
| coverage on every pull request | slows every job and adds a service that can fail every check | coverage at night, section 7 |
| a script per check in the workflow | untested glue that runs differently from a laptop | one command per job, decisions in a tested program, section 1 |

## Before saying done

1. Every job runs one local command, and no step masks a failure.
2. The change starts only the jobs it reaches, the filter fails open and
   a test holds it.
3. Caches are keyed on what they hold and saved from main.
4. No job runs past its limit, and a hang is ended by the runner with
   every stack written out.
5. In CI, a missing prerequisite fails.
6. Every scheduled run reports its own failure.
7. A claim of faster has several runs on each side, the machine and the
   spread.
8. Nothing a check did was lost on the way.
