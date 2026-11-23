---
name: testing
description: How to write, fix and review tests in any project, and how to find a bug before fixing it. Tests verify what matters to the people using the product, never wait on the clock, and run fast. Read it before writing, changing, fixing or reviewing a test, before fixing a reported bug, and before writing code that keeps time, waits on work, or reaches the disk, a program or the network. Where another skill says otherwise about tests, this one wins.
---

# Testing

A test is not the work. The work is knowing that what people rely on
works, every time, on any machine, and hearing in seconds when it stops.

**The user** is the person the agent works for, who decides what the
product does. **The people using the product** are who it is for. The
project's instructions say where its fakes, events and checks are. This
skill says how to use them, and what to push for where they are missing.

## 1. Verify what matters

- **Start from the product, not the code.** Say in one sentence what a
  person would see go wrong if this broke: an order charged twice, a page
  that never loads, a file deleted that should have been kept. The test
  guards that. No such sentence, no test.
- **Test what the product should do,** as the project's docs, the issue
  or the user say, not what the code does today.
- **When nobody can say why a behaviour exists, question the feature,**
  since its test keeps it alive.
- **Where the product has a screen, what a person can see is measured
  and tested:** a colour a little off, a box a pixel out, a step that
  lags the hand, see the interface skill. The user is never the first to
  find it.
- **A promise to the people using the product,** like never deleting
  their files, has a test or CI check that fails the day it breaks.
- **A test catches a break.** Name the code change that would fail it. A
  test only a deliberate decision fails, a wording, a constant, a private
  struct's shape, is a change detector that fails every redesign and
  sleeps through every bug. Leave it out.
- **Expected values are written by hand,** never computed with the code
  under test or its helpers, or both sides are wrong together. A table of
  cases with literal wants is the usual shape.
- **Coverage is a map, not the goal.** A covered line is not a verified
  one, and high coverage from weak tests is worse than lower coverage
  from strong ones, because it looks safe.
- **Every test can fail.** For every behaviour a change adds, name the
  line whose removal must fail a test, the gate, the guard, the call,
  remove it in a scratch copy and watch the test fail. Reviewers doing
  this kept finding a guard with no test behind it. Mutation testing does
  the same across the code, a CI tool's work, see section 12, so push for
  it where CI has none. A rewritten test still fails where the old one
  did.
- **Check what comes out, not that the code asked for it:** the bytes
  sent, the pixels painted, the file written. Checking the request
  is enough only where a hard interface guarantees the effect.
- **An empty result cannot pass.** Check the length before looping over
  results. Check each setup step took effect: a test of redo passed when
  the undo before it did nothing. In a concurrency test, check each side
  ran. Never throw away a load test's answers.
- **Pick values no fallback can produce.** A value that happened to equal
  the fallback made a test pass while the code did the opposite of its
  name.
- **Check the fixture can show the case.** "Picks a different row" needs
  a fixture with two rows.
- **No retries.** A test passes on its first try or it has failed. A
  runner's retry setting, which stays at zero, a rerun in CI, a loop that
  acts again until the answer suits it: each lets a broken test pass and
  hides the order a flaky test is about. Allowed are only a gesture that
  corrects itself the way a person's does, a check of the product's own
  retries, or a comparison redone because the state moved on, never
  because the same state gave an unwelcome answer. Each says so where it
  is, and the project keeps a complete list of them.
- **Not every line earns a test.** Glue that passes values on, code a
  library tests, a log line's wording: leave them. Spend tests where a
  mistake would reach a person.

## 2. What a test covers

A reported bug is one case of a kind. The test covers the kind.

- **Every variant.** Both edges, all corners and sides, both directions,
  first and last.
- **Every place the behaviour appears.** A key that closes one list closes
  every list, and a gesture behaves the same on every kind of element.
- **Every path that shares the cause:** the undo, every writer of the
  same file, everything that moves the same view. A cause left in a
  sibling path is the commonest review finding.
- **One check per sentence of the report.** "It stays put, and then
  scrolling works again" is two checks.
- **What must stay the same.** Neighbours stay put, the rest keeps its
  layout, wording and timing. A change nobody asked for is a new bug.
- **Two paths to one result agree,** and **separate things stay
  separate.** Test both.
- **The orders around the reported one,** remove then undo, undo then
  edit, edit while work runs, to find state that should have been
  cleared.
- **Again, and fast.** A gesture repeated, a double click after a drag,
  ten presses, two requests at once.
- **The edges of the input.** Empty, one, very many, very short, very
  long, a session left open for an hour.
- **On a screen, what changes together changes in the same frame.** Lag
  and stutter are behaviour, measured frame by frame.
- **The core path is always guarded.** Signing in, opening a document,
  paying: a test on every change.
- **Rules after every step.** End-to-end tests check after every step
  for a console error, a failed request, two items selected where one may
  be, so every new test gets them free.
- **Invalid states are made impossible, not tested,** by types or data.

## 3. Is it already tested?

The same behaviour turns up in issues in different words and gets tested
twice. Before writing a test:

- **Search the tests** for the function, the gesture and the words of the
  behaviour, and **ask the coverage** which lines existing tests run.
- **Strengthen rather than add.** A test that covers the code but missed
  the bug was weak: add the case to its table or make its check exact. A
  second test beside it drifts.
- **A regression asks why the tests missed it.** Say in the pull request
  which test should have caught it and why not, and fix that test, so a
  behaviour settled once is never settled again.
- **One behaviour, one level,** the lowest that can see it. Rules are
  tested in the core, and an interface test checks only what a person
  sees and does. Logic found in the interface moves to the core, under a
  unit test.

## 4. Tests never wait

The rule that matters most, and the one most often broken.

**A test runs straight through.** Set up, take a step, check, next step.
Everything that moves is in its hands: the clock, the order of work, what
a service answers, the disk, what the backend sends.

**It moves on events, not on time,** in one of two ways, best first:

1. **It causes the event itself.** It calls the function, moves the fake
   clock, hands the queue a job and steps it. Nothing runs that it did not
   start.
2. **It receives the event the code sends for exactly that step:** a
   channel closed when the job is done, a callback, a program writing that
   it is ready, the page resolving a promise once the step is drawn. It
   arrives the moment the work is done, so it is a reply, not a wait.

**Never the clock.** No sleep, no poll until a deadline, no milliseconds
for a step:

- A wait long enough on one machine is too short on a busy one, so the
  test fails now and then, for no reason in the code.
- Waits long enough add up to minutes of CI on every push.
- It checks that work usually ends in time, which nobody asked for.
- A poll with a deadline is a sleep in a loop.

**Load may make a test slower, never make it fail.** One that fails on a
busy runner waits on time somewhere, and that is the bug.

**The details are in [waiting.md](waiting.md),** read before writing a
test that waits on anything. In short: wait for the step to end on its own event, sent after the last stage
that can drop the result, then compare. Read and act in one step inside
the product, and time reactions there. Prove that something did not
happen with a try-lock or what the code decided, then one check. Set a
framework's default timeout to the runner's own. Time the product defines
runs on a fake clock the test moves, which does not order threads.

**A test that has to wait is a finding,** never fixed by waiting longer.
Ask:

- **Why is this work slow?** If it is slow for a person too, log it as a
  product bug.
- **Is this the right test from the right place?** Waiting on a slow
  service to check how its answer is shown tests the service. Hand the
  interface an answer from a fake, and test the slow part at its own
  level.

**Where the code gives no event, the code gains one,** as part of the
fix. A test never works around code that cannot say when it is done.

**The same holds for whoever runs the tests.** An agent runs a long suite
in the background and is woken when it ends, and subscribes to CI's
events rather than polling.

## 5. Design for testing

Code that is hard to test is usually shaped wrong, and the fix is the
shape. **The details are in [design.md](design.md),** read before writing
code that keeps time, waits on work, or reaches the disk, a program, the
network or the clock. In short:

- **Hand things in.** What a test must control, time, the disk,
  randomness, a service, a program, the end of work, comes in as a field
  or parameter. The program hands in the real thing, a test one it
  drives, and nothing handed in means the harmless thing. The test's
  double never ships, nor a flag that picks it.
- **Test up to the boundary you own.** Hardware, drivers and system
  frameworks sit behind a port, and a test checks what is handed to it.
- **Shapes that test well:** concerns apart behind small interfaces, one
  source for each piece of state, one mechanism used everywhere, one path
  for every input, one contract for the channel between two halves, a
  golden file for a format that crosses a boundary, every background task
  owned by something that hears it end.
- **A fix makes the design simpler, or it is the wrong fix.**

## 6. When a test fails, or a bug is reported

**The steps are in [failures.md](failures.md),** read before fixing any
failure or bug. In short:

1. **Reproduce it first, as it happened,** with the reporter's input and
   moment, and see the exact failure before the fix.
2. **Read what should happen,** then the code and the test.
3. **Find the cause.** Bisect. Look for one cause behind several
   symptoms. A flaky test's entropy is in the test or its environment
   first.
4. **Decide what is wrong,** in this order: environment, test, design,
   code.
5. **Size it, look around it, and fix it for every machine people use.**

**A failing test changes the test, not the product, unless a person
meets the same failure,** and that evidence is named first: a report or
log from someone using the product, a reproduction with real input on a
machine people use, or an ordinary gesture at a person's speed. The
maintenance skill says why.

Never, to get green: loosen a check, raise a timeout, add a sleep or a
retry, skip, quarantine or remove a test, or set the expected value to
what the code now says without knowing why it changed.

## 7. Flaky, red and hanging checks

A test that fails now and then met an order of events it did not expect.
**How to find it, and how to handle red and hanging checks, is in
[failures.md](failures.md).** In short:

1. Read what the failure left: trace, race report, dump of every program
   the test started. A runner that keeps no trace is the first fix.
2. Read the code for the order it assumes.
3. Make that order happen on purpose, by holding one event back.
4. Try the orders with a tool: race detector, fake clock, a scheduler.
5. Run it alone many times only to count. Load the machine last.

Flaky tests come first, a hang is fixed at once, red is explained before
it is rerun, and every failure seen is written down where it is tracked.
The same kind of test fix twice means one helper every test goes through.

## 8. Fast, and cheap to run

Every second of a suite is paid on every push.

- **Run only what a change reaches,** its tests and those of what depends
  on it. How CI picks them is the ci skill.
- **Use the caches, never defeat them.** Never turn the test cache off
  across the board, like Go's `-count=1`, which is for a test that reads
  what the cache cannot see. Keep the time, a random seed or a file that
  changes every run out of what a test checks. Keep test selection flags
  stable, caches are keyed on them.
- **Where no cache applies yet, push for one,** in an issue that says
  what it would save.
- **Fast by design.** No waits, section 4. The lowest level, section 3.
  The smallest fixture that shows it, and none realistic for a mockup.
  Slow tests in parallel, unless they change what the whole process
  shares, and then a comment says which. A file system in memory.
- **Every test file and CI job earns its place:** say what it guards and
  what it costs. Test tooling stays small and uses the tools' own
  features. Fixtures are plain files, made once and committed, never
  while tests run, which costs time every run and fails wherever their
  tool does. Share the few there are before adding one.
- **Work on the tests proves itself against the main branch,** with
  numbers before and after, on one machine, in a table, and is dropped if
  it is not faster or more reliable. Measure each step, and read what is
  measured already, before speeding anything up. Once every wait is an
  event, a test no faster was already as fast as it can be.
- **A shortcut never changes which product code runs.** It may remove
  what the harness does twice, never turn off an animation, a detector or
  a code path. Compare coverage before and after, name every line lost,
  and keep a test with the behaviour on. Reduced motion in random tests
  saved two seconds a job and dropped code only they reached.
- **A test that reads the whole repository defeats the cache.** Such
  checks belong with the lint.
- **A benchmark proves only the case it reaches.** Check it reaches the
  reported case, compare with the branch's merge base, not a stale main,
  and interleave before and after on one machine.
- **A probe that proved a fix is committed** as a test or benchmark in the
  same change.

## 9. Tests that last

A test is code we keep, still right and readable in a year.

- **Through the behaviour, not the inside,** via the package's own
  functions, so a refactor keeps the tests green.
- **Each test reads on its own.** Setup, action and expectation are in
  the test, not three helpers away. A line repeated in two tests is fine.
- **Shared setup lives in one place, extended, not copied.** One fake per
  service, one set of end-to-end helpers. A second fake drifts.
- **The name says the behaviour.** `TestRefundKeepsTheOrderWhenOneItemIsReturned`,
  not `TestRefund3`.
- **A failure says what went wrong:** what was asked, what came back,
  what was wanted.

## 10. Unit tests

**The details are in [unit.md](unit.md).** In short: concurrent code is
tested from several threads at once under the race detector, and a
concurrency test fails without its lock. The standard test library first.
Off the disk and the network. Fuzz only what reads bytes from outside,
and run many more seeds than CI when changing code under a random test.
A fake keeps the real one's limits.

## 11. End-to-end and interface tests

**The details are in [end-to-end.md](end-to-end.md),** read before writing
or fixing one. In short: do what the person did and check what they saw,
against the real backend and the product's own wiring, in the engine the
product runs on. Check only what the window can show, read everything a
rule judges at one moment, wait on a barrier built from signals, and
judge motion by the frames drawn. A new case is a step in an existing
test.

## 12. CI holds the rules

Every rule here a tool can check is checked in CI on every pull request,
because a rule only written down slips: no sleep or poll, no skipped or
focused test, every test can fail, no data race, no retries. Which tool
holds which is the ci skill. **CI is the proof that counts:** push once
the change's tests pass and let CI run the rest. Never rerun the whole
suite locally after the push, and hear of a failing run by the pull
request's events.

## 13. Review and handing over

- **An independent review, once the work is done,** as the review skill
  says. The reviewer breaks the code to see the tests fail, and looks for
  a test that passes by coincidence, checks code against a copy of its
  own rule, or waits on time.
- **Review your own work for design first.** Thousands of lines still
  broken mean the design is wrong.
- **The tests prove the behaviour. The user only looks,** in a click or
  two, each step in the product as they have it and at a person's speed.
  For a state hard to reach, give a temporary page or commit showing
  every state with made-up data, removed before the merge. A case faster
  than a hand is proven by a named test, and the step only looks at the
  result.
- **Visual choices iterate fast.** Render candidates from the real code
  side by side, compare with the version before and the established way,
  and measure.
- **A proof that needs the user's machine says so.** Prove what you can
  against fakes, and say which number only theirs gives, and where.
- **Logs make the product testable in the field.** Structured logs from
  an established library, tagged, where a bug would be found, so one
  report is enough to find it.

## 14. Where other advice says otherwise

General testing advice is good on the whole. On these points this skill
wins:

| Common advice | Why not | Instead |
| --- | --- | --- |
| `assert.Eventually`, `waitFor`, waiting on a condition up to a deadline | a sleep in a loop | the event for that step, section 4 |
| sleep a few seconds for a service to start | a guess about time | the service says it is ready, and the test hears it |
| a test's own timeout on `time.After` or a timer | a guess that hides where it hung | the runner's own timeout |
| `networkidle`, wait for the page to be quiet | quiet is a guess about time | the event the page sends when the step is done |
| retry a flaky test, in the runner, in CI or in a loop | passing on the second try is failing | no retries, section 1, and the cause, section 7 |
| load the machine to make a flaky test fail | changes every timing, proves nothing when it passes | the trace, then the one order held on purpose, section 7 |
| turn the test cache off to be safe | every run pays for every test | keep the cache, section 8 |
| a coverage number as the bar | coverage is not verification | what matters verified, and mutation testing, section 1 |
| a dependency injection container | more machinery than a constructor | plain constructor injection, section 5 |
| tags or flags to keep slow tests out of the usual run | slow tests stay slow and unrun | make them fast, or skip with a reason when a program is missing |

## Before saying done

1. The test guards a behaviour named in one sentence, and the line whose
   removal fails it was named and removed once.
2. It covers the kind: every variant, every place, what must stay.
3. Nothing already tested it, or the old test was strengthened instead.
4. It failed on the main branch for the reason given, and passes now.
5. It never waits, and nothing in it defeats the test cache.
6. A change to tests or CI has its numbers against the main branch, and
   the end-to-end tests take no longer than there.
7. CI passed on the last commit, and every rule here CI does not check
   yet has an issue asking for the check.
