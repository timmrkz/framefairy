# When a test fails, or a bug is reported

Sections 6 and 7 of the testing skill in detail: how a bug is found
before it is fixed, and how a test that fails now and then, a red check
and a hanging one are handled.

## Finding the cause

1. **Reproduce it first, as it happened.**
   - Use the input the reporter used, the same file, the same data, at
     the moment their log names, not at a moment you pick.
   - Read what the reporter already sent, logs, recordings, a report
     bundle, before asking them for anything again.
   - Reproduce it on your own machine where you can. Ask the user to run
     something only for what only their machine can show.
   - See the exact failure before the fix and see it gone after. A fix
     never seen failing has not been shown to fix anything.
2. **Read what should happen.** The spec of the part, the issue, what the
   user said. Then read the code and the test.
3. **Find the cause.**
   - "It worked yesterday" means something changed. Bisect with the
     failing test, and explain why it worked before, not only why it
     fails now.
   - Several symptoms after a run of merges often have one cause. Look
     for it before fixing each, or the fixes stack up on each other.
   - A test that passes most of the time and fails now and then has a
     source of entropy in the test or in its environment. Look for it
     there first: the event the test misses or the order it assumes,
     with the tools in section 7 of the skill, in their order. Loading the machine
     comes last. The product is suspected only once the trace shows the
     product itself doing the wrong thing in that order.
   - A vague report of instability, with no case that fails every time,
     is still a report. Look at the design, its concurrency and its order
     of events.
4. **Decide what is wrong**, asking of each in this order, and moving
   on only once the evidence rules it out:
   - **The environment.** Does it fail only where the test runs, on a
     CI runner and its virtual hardware, on a busy machine, with a
     stand-in program or a file made for the test? Look there first.
     What the environment's own hardware or programs do is fixed in how
     the test uses them, never in the product, and a test that reaches
     them at all is the first thing to change, see Test up to the
     boundary you own in section 5 of the skill. What the test fed the
     product decides it, not where it ran: input from outside, like a
     fuzzer's bytes, or an order of the product's own work that a busy
     machine only made likelier, is the product's to fix.
   - **The test.** It checked something nobody specified, read the page
     before the step had happened, or depended on time. Fix it so it
     checks what matters, on an event.
   - **The product's assumptions, the design.** The behaviour the test
     protects does not serve a person, or the spec contradicts itself,
     or the code is shaped so the right behaviour cannot be tested
     without waiting. Tests on a broken design protect the broken
     design. Say so to the user plainly, log an issue, and fix the
     design when that is what the change is for. No patches on top:
     find the root cause, and refactor if that is what it takes.
   - **How the product carries them out, the code.** Fix the code, the
     test stays as it is.
5. **Size it.** How big is the difference, and where would a person see
   it? A finding too small to matter is closed as won't fix, with its
   number.
6. **Look around it.** Where one bug was, try the gestures and states
   next to it. Each new case found gets its own failing test, or its own
   issue when it is outside the change.
7. **Fix for every machine people use.** A fix tuned to one machine, its
   screen, its speed or its system, is a bug on the next machine. A CI
   runner's virtual hardware and a stand-in are the test's environment,
   not machines people use, see step 4.

Section 6 of the skill says when a failing test changes the product at
all: only when a person meets the same failure. Beside that:

- **CI tests the merge with the main branch,** not the branch alone. A
  test that passes on an old branch and fails in CI is reproduced on a
  local merge with the main branch before the runner is suspected.
- **Before widening a tolerance, measure the distance to the nearest
  wrong answer.** The tolerance stays well below it, or the check no
  longer tells right from wrong. A pixel tolerance once grew past the
  difference between two neighbouring frames and still passed.
- **A task to make tests faster changes only tests,** the harness, the
  fixtures and CI. A cost found in the product is a change of its own,
  with its own tests and benchmarks.

## Flaky, red and hanging checks

### Finding why a test fails now and then

A test that fails now and then has run into an order of events it did
not expect. In a single-threaded language, a page, Node, Python's asyncio,
nothing touches the same memory at once, so there is no data race to
detect. What fails is the order: the test read the result before the
step had finished, or waited for a signal that had already gone by, or
the code under test did the two halves of its work in an order nobody
thought of. Find that order with these, in this order:

1. **Read what the failure left.** The runner keeps a trace of a
   test that failed, and keeps it in CI with the run. Playwright's
   trace has every action and the console and, where a project pays
   for them, a film of the screen and snapshots of the page before and
   after each action, with the network. Read what the project records before
   looking for what it left out. The race detector has its
   report, and `go test -timeout` prints every goroutine. Most flakes are
   explained there, without running anything. Where the runner keeps
   no trace of a failure yet, that is the first thing to fix. A trace
   is several records of different cost. The calls and the console
   are text the runner already has, and cost next to nothing. A film
   of the screen or a snapshot of the page costs every run real time,
   and a runner may record the network only along with the snapshots,
   as Playwright does. Then the runner can hear each request and its
   answer from outside the page and keep them beside the trace, which
   adds nothing to the page under test. Keep the cheap records on for every test, and turn
   the costly ones on only for the tests whose trace showed they are
   needed, in one list that names the issue for each. The change that
   fixes the test takes its line out.
2. **Read the code for the order it assumes.** Name the event the test
   waits for and where the code sends it. Ask whether the test could
   read before it, and whether the event could come before the test
   listens. Ask the same of the code under test.
3. **Make that one order happen on purpose.** Hold the one event back
   until the test lets it go: a request held with Playwright's
   `page.route` or by a gate in the fake, a step of the fake clock, the
   page's CPU throttled by the browser's own setting. A flake made to
   fail every time is a bug with a test, and the fix is seen to work.
4. **Try the orders systematically.** Go's race detector, and
   `testing/synctest` for goroutines and timers. For async code in
   JavaScript and TypeScript, fast-check's scheduler, `fc.scheduler()`
   in a unit test, which tries the orders in which promises and events
   can resolve, and cuts a failure down to the shortest order that
   breaks it.
5. **Run it again, alone, many times.** That counts how often it
   fails, before and after a fix. It finds no cause by itself.
6. **Load the whole machine, last.** Busy loops beside the test change
   every timing at once, so a run that fails says nothing about which
   order was wrong, and one that passes proves nothing. It takes many
   runs, it breaks unrelated tests beside it, and a cloud machine under
   load hangs. Reach for it only when nothing above has found the
   order, and then go back to step 3 with what it showed.

Debug logs added to see the order are step 1 done late: the trace
should already hold them. A log line that would have explained the
failure belongs in the trace or the product's logs for good.

- **A hang dumps what the test waited on,** the stacks of every program
  the test started and not only its own threads. Six hangs in a row said
  only where the test waited, and the first dump of the other program
  named the call that stopped.
- **The next failure explains itself.** When a rare failure cannot be
  explained yet, the change is to its message: what was left, by whom,
  how old, where the steps had put things. Then the next failure brings
  its cause along, instead of a hunt by load.
- **The same kind of test fix twice means one helper.** When a second
  test makes the same mistake, put every test through one helper, so the
  next is right by construction. A reset between tests closes the
  product the way it closes itself, then removes its files. Stopping the
  parts one by one misses the one nobody named, and failed three times
  that way.
- **Everything a test puts into the product, it takes out.** A watcher,
  an observer, a loop per frame left behind slows the product and makes
  the very flakes it was there to find.
- **A program a test writes and runs at once can be refused as busy**
  when another test forks while the file is open. Write such programs
  through one helper that keeps forks out while the file is open, or
  commit them as fixtures.

### Red and hanging checks

- **Flaky tests come first.** A test that fails now and then teaches
  everyone to ignore red. Rank them by how often they failed in CI, the
  nightly runs too, and fix the worst first. A nightly job that has never
  passed is among the worst.
- **A check that hangs is fixed at once**, in a change of its own, and
  never lived with. It is also no reason to hold back work CI has already
  proven.
- **Red CI is explained before it is rerun.** Read the check's
  annotations and compare its step times with the last runs on the main
  branch. A runner that was never given, or was lost, says so there. A
  failure of ours is ours, whatever change it showed up on.
- **Every failure seen is written down where it is tracked.** Whoever
  runs into a failing check, on their own change, on someone else's or
  on the main branch, looks for the issue that tracks it. When one is
  open, a comment there gives the run, the commit, the test and what it
  printed, so every time it happened is in one place when the issue is
  read again. When none is open, one is opened, the way the project logs
  its issues. The reply to the user links the comment or the issue.
- **A manual step to get tests passing is a defect.** A stale build
  rebuilt by hand, a cache cleared by hand: fix the build so the step is
  never needed again.
- **A guard that is removed says why.** A lint rule, a check or a test
  taken out says in the pull request what it guarded and how the code is
  better without it.
