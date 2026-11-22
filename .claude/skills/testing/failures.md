# When a test fails, or a bug is reported

Sections 6 and 7 of the testing skill in detail.

## Finding the cause

1. **Reproduce it first, as it happened.**
   - Use the reporter's input, at the moment their log names.
   - Read what they already sent, logs, recordings, a report bundle,
     before asking again.
   - Reproduce on your machine where you can. Ask the user to run only
     what only their machine shows.
   - See the exact failure before the fix and see it gone after.
2. **Read what should happen,** in the spec, the issue, the user's words.
   Then the code and the test.
3. **Find the cause.**
   - "It worked yesterday": bisect with the failing test, and explain why
     it worked before, not only why it fails now.
   - Several symptoms after a run of merges often share one cause. Find
     it before fixing each, or the fixes stack up.
   - A flaky test has a source of entropy in the test or its
     environment. Look there first, with the steps of section 7 of the
     skill. Suspect the product only once the trace shows it doing wrong
     in that order.
   - A vague report of instability is still a report. Look at the
     design, its concurrency and its order of events.
4. **Decide what is wrong**, asking in this order, and moving on only
   once the evidence rules each out:
   - **The environment.** Does it fail only where the test runs, on a CI
     runner's virtual hardware, a busy machine, a stand-in or a file made
     for the test? Then fix how the test uses them, never the product,
     starting with a test that reaches them at all, see Test up to the
     boundary you own in section 5 of the skill. But input from outside,
     like a fuzzer's bytes, or an order of the product's own work a busy
     machine only made likelier, is the product's to fix.
   - **The test.** It checked something nobody specified, read too early,
     or depended on time. Make it check what matters, on an event.
   - **The design.** The behaviour does not serve a person, the spec
     contradicts itself, or the code cannot be tested without waiting.
     Tests on a broken design protect it. Tell the user, log an issue,
     and fix the design when the change is for that. No patches on top:
     fix the root cause, refactoring if needed.
   - **The code.** Fix the code, and the test stays as it is.
5. **Size it.** How big is the difference, and where would a person see
   it? One too small to matter is closed as won't fix, with its number.
6. **Look around it.** Try the gestures and states next to the bug. Each
   new case gets its own failing test, or its own issue when outside the
   change.
7. **Fix for every machine people use.** A fix tuned to one machine's
   screen, speed or system is a bug on the next. A CI runner is not such
   a machine, see step 4.

Section 6 of the skill says a failing test changes the product only when
a person meets the same failure. Beside that:

- **CI tests the merge with the main branch.** A test that passes on the
  branch and fails in CI is reproduced on a local merge with main before
  the runner is suspected.
- **Before widening a tolerance, measure the distance to the nearest
  wrong answer,** and stay well below it. A pixel tolerance once grew past
  the difference between two states it had to tell apart, and passed.
- **A task to make tests faster changes only tests,** the harness, the
  fixtures and CI. A cost found in the product is a change of its own.

## Flaky, red and hanging checks

### Finding why a test fails now and then

A flaky test met an order of events it did not expect, even in a
single-threaded page, Node or asyncio, where no data race exists: it read
before the step finished, waited for a signal already gone, or the code
did two halves of its work in an unforeseen order. Find that order, in
these steps:

1. **Read what the failure left,** before running anything: the trace
   the runner keeps in CI too, the race detector's report, the stacks a
   hang dump prints, like Go's `go test -timeout`. Most flakes are explained there. A runner
   that keeps no trace yet is the first fix. Calls and console are nearly
   free, so on for every test. A film of the screen or page snapshots
   cost every run time, so on only for tests whose trace showed the need,
   in one list naming the issue for each, until the fix takes its line
   out. Where the network is recorded only with snapshots, as in
   Playwright, record requests and answers from outside the page.
2. **Read the code for the order it assumes.** Name the event the test
   waits for and where the code sends it. Could the test read before it,
   or could it come before the test listens? Same for the code under test.
3. **Make that one order happen on purpose.** Hold the one event back
   until the test lets it go: a request held with Playwright's
   `page.route` or a gate in the fake, a fake clock step, the browser's
   CPU throttling. A flake that fails every time is a bug with a test.
4. **Try the orders systematically.** In Go, the race detector, and
   `testing/synctest` for goroutines and timers. For async JavaScript,
   fast-check's `fc.scheduler()` in a unit test tries the orders promises
   resolve in and shrinks a failure to the shortest one.
5. **Run it again, alone, many times,** to count how often it fails,
   before and after a fix. It finds no cause by itself.
6. **Load the whole machine, last.** Busy loops change every timing at
   once, so a failure names no order and a pass proves nothing, and they
   break unrelated tests and hang cloud machines. Use it only when nothing
   above found the order, then go back to step 3.

Debug logs added to see the order are step 1 done late. A line that
would have explained the failure stays in the trace or the logs for good.

- **A hang dumps what the test waited on,** the stacks of every program
  it started, not only its own. Six hangs said only where the test
  waited, and the first dump of the other program named the stuck call.
- **The next failure explains itself.** When a rare failure cannot be
  explained yet, improve its message: what was left, by whom, how old,
  where the steps had put things. The next failure then brings its cause.
- **The same kind of test fix twice means one helper** every test goes
  through, so the next is right by construction. A reset between tests
  closes the product the way it closes itself, then removes its files.
  Stopping parts one by one misses the one nobody named.
- **Everything a test puts into the product, it takes out.** A watcher or
  a loop per frame left behind slows the product and causes flakes.
- **A program a test writes and runs at once can be refused as busy**
  when another test forks while the file is open. Write such programs
  through one helper that keeps forks out meanwhile, or commit them as
  fixtures.

### Red and hanging checks

- **Flaky tests come first.** They teach everyone to ignore red. Rank
  them by how often they failed in CI, nightly runs too, and fix the
  worst first. A nightly job that has never passed is among the worst.
- **A check that hangs is fixed at once**, in a change of its own. It is
  no reason to hold back work CI has already proven.
- **Red CI is explained before it is rerun.** Read the check's
  annotations and compare its step times with recent runs on main. A
  runner never given, or lost, says so there. A failure of ours is ours,
  whatever change it showed up on.
- **Every failure seen is written down where it is tracked.** Whoever
  meets a failing check, on any branch, comments the run, commit, test
  and output on the issue tracking it, or opens one. The reply to the
  user links it.
- **A manual step to get tests passing is a defect,** like a stale build
  rebuilt or a cache cleared by hand. Fix the build instead.
- **A guard that is removed says why.** A lint rule, check or test taken
  out says in the pull request what it guarded and why the code is better
  without it.
