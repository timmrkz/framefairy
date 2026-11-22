---
name: testing
description: How to write, fix and review tests in any project, backend, frontend or anything else, and how to find a bug before fixing it. Tests verify what matters to the people using the product, never wait on the clock, and run fast. Read it before writing, changing, fixing or reviewing a test, before fixing a reported bug, and before writing code that keeps time, waits on work or reaches the disk, a program, the network or the clock. Where another skill says otherwise about tests, this one wins.
---

# Testing

A test is not the work. The work is knowing that what people rely on in
the product does what it should, every time, on any machine, and finding
out in seconds when it stops. A test is how we know. Everything below
follows from that.

Two people are named here. **The user** is the person the agent works
for, who owns the product and decides what it should do. **The people
using the product** are the ones it is made for. A project's own
instructions, its README, its contributing guide or its agent file,
say where its fakes, its events and its checks are. This skill says how
to use them, and what to push for where they are missing.

## 1. Verify what matters

- **Start from the product, not the code.** Before a test, say in one
  sentence what a person using the product would see go wrong if this
  broke: an order charged twice, a page that never loads, a file the
  product deleted that it should have kept. That sentence is what the
  test guards. If there is no such sentence, there is no test to write.
- **What the product should do is written down.** The project's
  instructions, the doc of the part, the issue, what the user said. A
  test checks that, not what the code happens to do today.
- **When nobody can say why a behaviour is in the product, question the
  feature, not only the test.** A test that protects a behaviour nobody
  wants keeps it alive.
- **What a person can see can be measured, and what can be measured is
  tested.** A colour a little off, a box a pixel out of place, a step
  that lags behind the hand: read the pixels, the frame, the time it
  took. Measure what is painted, not the box a layout reports: an odd
  sized mark centred in an even box lands on a half pixel, which the
  engine rounds in a direction nobody chose, and two fixes that trusted
  the box moved nothing on screen. The user should never be the first to
  find it.
- **A promise to the people using the product is proven, not assumed.**
  It never deletes their files. It never reads what it was not given.
  Such a promise has a test or a check in CI that fails the day it
  breaks.
- **A test catches a break.** Name the change to the code that would make
  it fail. If only a deliberate decision can fail it, a wording, a
  constant, the shape of a private struct, it is a change detector. It
  fails on every redesign and sleeps through every bug, so it does not go
  in.
- **Expected values are written by hand.** Never work out what is
  expected with the code under test or its helpers, because then both
  sides are wrong together. A test that checks the code against a copy
  of the code's own rule proves nothing. A table of cases with literal
  wants is the usual shape.
- **Coverage is a map, not the goal.** Every behaviour that matters should
  be covered, so coverage should be high. But a covered line is not a
  verified line: a test that runs code and checks little reports 100 %
  and proves nothing, and a product can fail with every line covered.
  High coverage from weak tests is worse than lower coverage from strong
  ones, because it looks safe.
- **Every test can fail.** A test that would still pass with the code
  broken guards nothing. Mutation testing finds those: it breaks the code
  on purpose, a constant changed, a branch removed, an empty answer, and
  reports every break no test caught. That is a tool's work in CI, see
  section 12. Where CI has none yet, break the code by hand once and see
  the test fail, and push for the tool. The same holds when an existing
  test is rewritten: the rewrite must still fail where the old one did.
- **Name the line whose removal must fail it.** For every behaviour a
  change adds, name the one line, the gate, the guard, the call, whose
  removal must make a test fail. Then remove it in a scratch copy and
  watch the test fail. Reviewers who did this found, again and again, a
  guard that carried most of a change and no test that missed it.
- **Check what comes out, not that the code asked for it.** The sound
  that reaches the output, the pixels painted, the file written. A test
  that records the code starting a sound passes while nothing comes out.
  A check on the request is enough only where a hard interface guarantees
  the effect.
- **An empty result cannot pass.** Check the length before looping over
  results. Check that each setup step took effect before checking the
  end state: a test of redo passed when the undo before it did nothing.
  In a concurrency test, check that each side really ran. Never throw
  away the answers of the requests a load test makes.
- **Pick values no fallback can produce.** The path under test and every
  default or fallback must give different answers. A hand-typed value
  that happened to equal what the fallback computed made a test pass
  while the code did the opposite of its name.
- **The fixture can show the case, and the test says so.** Before
  checking "picks a different row" or "lands right among items of
  different heights", check that the fixture has two rows and unequal
  heights. A list with one row passes a pick check whatever the code
  does.
- **No retries.** A test passes on its first try or it has failed. A
  retry, a runner's retry setting, a rerun in CI or a loop that reads or
  acts again until the answer suits it, wherever it is written, lets a
  test pass with the code broken, and hides the very order a flaky test
  is about. A runner's retry setting stays at zero. What is left is a
  gesture that corrects itself the way a person does, a hand that aims
  again after a near miss, a check of a product's own retries, or a
  comparison the test makes again because the state it compared has
  moved on since, never because the same state gave an answer it did
  not like. Each one says so where it is, and the project keeps a
  complete list of them, so the next reader does not have to judge each
  loop again.
- **Not every line earns a test.** Glue that only passes values on, code
  a library already tests, a log line's wording: leave them. Spend tests
  where a mistake would reach a person.

## 2. What a test covers

A reported bug is one case of a kind. The test covers the kind.

- **Every variant.** Both edges, in and out, all four corners and all
  four sides, left to right and right to left, first and last.
- **Every place the behaviour appears.** A key that closes one list closes
  every list in the product, so the fix and its test cover all of them,
  not only the one in the report. The same gesture behaves the same on
  every kind of element.
- **Every path that shares the cause.** The undo of the action, every
  writer of the same file, every other thing that moves the same view. A
  fix in one of them is a partial fix, and reviews found the cause left
  in a sibling path more often than almost anything else.
- **One step for each sentence of the report.** "It stays where it is, and
  then scrolling works again" is two checks. A test of the first half
  leaves the second free to break.
- **What must stay the same.** A change to one item leaves its neighbours
  where they were. A fix to one thing leaves the layout, the wording and
  the timing of the rest alone. Check what the fix must not touch as well
  as what it changes. A fix that changes something nobody asked for is a
  new bug.
- **Two paths to one result agree.** When two gestures or two calls are
  meant to end the same way, a test runs both and compares them.
- **Things that are separate stay separate.** Two parts that have nothing
  to do with each other are proven not to move each other.
- **The whole class of action sequences.** Try the orders around the one
  that was reported: remove then undo, undo then edit, edit while work
  runs, the same thing again from another state. State that survives
  where it should have been cleared shows up this way.
- **Again, and fast.** A gesture repeated, a double click after a drag, a
  button pressed ten times, two requests at once.
- **The edges of the input.** Empty, one, very many, very short, very
  long, the first and the last, a session left open for an hour.
- **What changes together changes in the same frame.** When one action
  changes three parts of a screen, a test reads the frames and checks
  they moved together. Lag is behaviour too: a view that trails the hand,
  an animation that stutters, is measured frame by frame and fails a
  test.
- **The core path is always guarded.** The one thing everything else
  stands on, signing in, opening a document, playing a file, has a test
  that runs on every change.
- **Rules after every step.** Some breaks can happen anywhere: an error
  in the console, a request that failed, two items selected where one may
  be. An end-to-end test checks those after every step, so every new test
  gets them for free.
- **Invalid states are made impossible, not tested.** Where the types or
  the data can rule out a state, they do, and there is no error path to
  test.

## 3. Is it already tested?

Issues describe the same behaviour in different words, so the same thing
gets tested twice by people who did not know of each other. Two tests of
one thing cost two maintenances and say nothing new. Before writing one:

- **Search the tests** for the function, the gesture and the words of the
  behaviour.
- **Ask the coverage** which lines the existing tests already run, and
  what one test reaches on its own, so two tests of the same lines are
  found.
- **Strengthen rather than add.** When a test covers the code but missed
  the bug, the test was weak: add the case to its table, or make its
  check exact. A second test beside it is a copy that drifts.
- **A regression asks why the tests missed it.** Say in the pull request
  which test should have caught it and why it did not, and fix that test.
  A behaviour settled once needs a guard so it is never settled again.
- **One behaviour, one level.** Test a thing at the lowest level that can
  see it. The logic lives in the core, and the interface shows what the
  core says, so a rule is tested in the core. What the person sees of it
  is tested in the interface. An end-to-end test does not check again
  what a unit test proves, it checks what a person sees and does. When a
  bug lives in logic in the interface, the logic moves to the core, under
  a unit test.

## 4. Tests never wait

This is the rule that matters most, and the one most often broken.

**A test runs straight through.** It sets up the world, takes a step and
checks the result, and then the next step. There is no moment where it
stands by in the hope that some work has finished. Everything that moves
is in its hands: the clock, the order work is done in, what a service
answers, what the disk holds, what the backend sends.

**Event-driven means the test moves on events, not on time.** An event
is something that has happened and says so. A test gets to the next step
in one of two ways, best first:

1. **It causes the event itself.** It calls the function and has the
   answer, it moves the fake clock forward, it hands the queue a job and
   steps it. Nothing runs that the test did not start, so there is
   nothing to wait for.
2. **It receives the event the code sends for exactly that step.** A
   channel the code closes when the job is done, a callback, a message,
   a program writing that it is ready, the page resolving a promise when
   the step has been drawn. Where work crosses into another program, a
   browser, a database, a worker, this is how the test hears it ended.
   The event comes the moment the work is done, so this is not a wait,
   it is a reply.

**Never the clock.** A test that sleeps, or looks again and again until
a deadline, or gives a step some milliseconds to finish, is time-based,
and time-based tests are wrong:

- A wait long enough on one machine is too short on a busy one, so the
  test fails now and then, on someone else's machine or in CI, for no
  reason in the code.
- The waits that are long enough add up to minutes of CI on every push.
- It checks something nobody asked for, that the work usually ends
  within so many milliseconds, and not whether it is right.
- A poll with a deadline is the same guess in a loop. It is not the
  cure for a sleep, it is a sleep.

**Load may make a test slower, never make it fail.** A test that passes
alone and fails beside others, or on a busy runner, is waiting on time
somewhere. That is the bug, not the load.

**The details are in [waiting.md](waiting.md):** what a wait on the clock
and an event look like side by side, where the event has to sit, the
helpers every step goes through, and the fake clocks. Read it before
writing a test that waits on anything. In short:

- **Wait for the step to end, then compare,** and wait on that step's own
  event, sent after the last stage that can drop the result.
- **Read and act in one step,** inside the product, and time a reaction
  inside the product too.
- **Prove that something did not happen without waiting:** a try-lock, or
  what the code already decided, then one check.
- **A framework's default timeout is a deadline nobody wrote.** Set it to
  the runner's own.
- **Time the product itself defines runs on a clock the test moves,** the
  one the language or the tool already has. A fake clock moves time, it
  does not order threads.

**A test that has to wait is a finding, not a test to fix by waiting
longer.** When a test can only go on after work that takes a while, ask
two questions before anything else:

- **Why is this work slow?** If it is slow for a person too, that is a
  bug or a performance issue in the product. Log it as an issue.
- **Is this the right test from the right place?** A test that waits for
  a slow service to check how its answer is shown tests the service
  through the interface. Hand the interface an answer, from a fake, and
  test how it is shown. The slow part gets its own test at its own level,
  if it needs one.

**Where the code gives no event, the code gains one.** That is part of
the fix, not a detour from it. A test is never the place to work around
code that cannot say when it is done.

**The same holds for whoever runs the tests.** An agent starts a long
test run in the background, ends its turn, and is woken when the run
ends. It never sleeps on it and never polls a CI run, it subscribes to
the run's events and acts when they come.

## 5. Design for testing

If the design of a solution is broken, tests help very little. Code that
is hard to test is usually code that is shaped wrong, and the fix is the
shape. **The details are in [design.md](design.md).** Read it before
writing code that keeps time, waits on work, or reaches the disk, a
program, the network or the clock. In short:

- **Hand things in.** What a test must control, the time, timers, the
  disk, randomness, a service, a program, the end of work, comes from
  outside as a field or a parameter. The program hands in the real thing,
  a test one it drives, and nothing handed in means the harmless thing.
  The test's double never ships, nor a flag that picks it.
- **Test up to the boundary you own.** What differs between machines,
  hardware, drivers, system frameworks, is behind a port, and a test
  checks what is handed over to it.
- **Shapes that test well:** concerns apart behind small interfaces, one
  source for each piece of state, one mechanism used everywhere, one path
  for every input, one contract for the channel between two halves, a
  golden file for a format that crosses a boundary, every background task
  owned by something that hears it end.
- **A fix makes the design simpler, or it is the wrong fix.**

## 6. When a test fails, or a bug is reported

A failing test asks what should have happened, and what kept it from
happening there. A bug report asks the same. **The steps are in
[failures.md](failures.md),** read before fixing any failure or bug. In
short:

1. **Reproduce it first, as it happened,** with the reporter's input, at
   the moment their log names, and see the exact failure before the fix.
2. **Read what should happen,** then the code and the test.
3. **Find the cause.** Bisect what changed. Look for one cause behind
   several symptoms. A test that fails now and then has a source of
   entropy in the test or its environment first.
4. **Decide what is wrong,** in this order: the environment, the test,
   the design, the code.
5. **Size it, look around it, and fix it for every machine people use.**

**A failing test changes the test, not the product, unless a person
meets the same failure,** and that evidence is named before any product
code changes: a report or a log from someone using the product, a
reproduction with real input on the kind of machine people use, or an
ordinary gesture at a person's speed. The maintenance skill says why.

Never, to get green: loosen a check, raise a timeout, add a sleep or a
retry, skip, quarantine or remove a test, or change the expected value to
what the code now says without knowing why it changed.

## 7. Flaky, red and hanging checks

A test that fails now and then has run into an order of events it did not
expect. **How to find that order, and what to do with red and hanging
checks, is in [failures.md](failures.md).** In short:

1. Read what the failure left, the trace, the race report, the dump of
   every program the test started. Where the runner keeps no trace, that
   is the first thing to fix.
2. Read the code for the order it assumes.
3. Make that one order happen on purpose, by holding one event back.
4. Try the orders with a tool: the race detector, a fake clock, a
   scheduler that explores orders.
5. Run it alone many times only to count. Load the machine last.

Flaky tests come first, a hang is fixed at once, red is explained before
it is rerun, and every failure seen is written down where it is tracked.
The same kind of test fix twice means one helper every test goes through.

## 8. Fast, and cheap to run

A test suite is run thousands of times. Every second in it is paid on
every push, by the people waiting on CI and by the machines.

- **Run only what a change reaches,** the tests of what changed and of
  what depends on it. How CI picks them, coarse path filters and the
  runner's own cache, is the ci skill.
- **Use the caches there are, and never defeat them.** Test runners and
  build tools cache results until their inputs change. So:
  - never turn the test cache off across the board, like Go's
    `-count=1`. It is for a test that reads what the cache cannot see.
  - no reading of the time, a random seed or a file that changes every
    run into what a test checks, or its result is never cached.
  - keep the test selection flags stable, caches are keyed on them.
- **Where a cache cannot apply yet, push for one.** Rerunning what did
  not change is a cost to remove, not a fact to accept. Say so to the
  user or log an issue with what it would save.
- **Fast by design.** No waits, section 4. The lowest level that can see
  the behaviour, section 3. The smallest and shortest fixture that shows
  it. Slow tests in parallel, unless they change what the whole process
  shares, and then a comment says which. Off the disk, with a file system
  in memory. A mockup or a screenshot to settle where something goes
  needs no realistic fixture at all.
- **Every test file and every CI job earns its place.** Say what it
  guards and what it costs. Test tooling stays small, uses the tools'
  own features, and keeps fixtures as plain files, not text inside a
  script. A fixture is made once and committed with the tests, never
  made while they run: one made every run costs its time on every run,
  and fails wherever the tool that makes it does. Share the few that
  are there before adding one.
- **Work on the tests proves itself against the main branch.** The point
  of it is tests that are faster and more reliable. A change to tests or
  CI shows its numbers before and after, on the same machine, in a table.
  What does not measure faster or more reliable is dropped. Before
  speeding anything up, measure each step and understand where the time
  goes, and look up what is already measured first. Then keep your eyes
  on the goal: once every wait is an event, a test that is no faster was
  already as fast as it could be.
- **A shortcut never changes which product code runs.** A speed-up may
  remove what the harness does twice. It may not turn off what the
  product does, an animation, a detector, a code path. Measure the
  coverage before and after, name every line lost, and keep a test with
  the behaviour on. Reduced motion in random tests saved two seconds a
  job and dropped code only those tests reached.
- **A test that reads the whole repository defeats the cache.** A test
  that reads the whole source tree or every doc ties its cached result to all
  of it. Checks over the whole repository belong with the lint.
- **A benchmark proves only the case it reaches,** against the right
  base. Check that it reaches the case reported, compare with the merge
  base of the branch, not a stale copy of the main branch, and run the
  before and the after interleaved on one machine, so a busy moment hits
  both.
- **A probe that proved a fix is committed** as a test or a benchmark in
  the same change. One kept outside the repository proves nothing the
  next time the code changes.

## 9. Tests that last

A test is code we keep. It must still be right and readable in a year.

- **Through the behaviour, not the inside.** Test what a person or a
  caller sees, through the package's own functions, so a refactor that
  keeps the behaviour keeps the tests green.
- **Each test reads on its own.** What it sets up, what it does and what
  it expects are in the test, not three helpers away. Repeating a line
  in two tests is fine when it keeps each readable.
- **Shared setup lives in one place, extended, not copied.** One fake of
  each service, one set of helpers for the end-to-end tests. A second
  fake drifts from the first.
- **The name says the behaviour.** `TestTrimKeepsAWordWholeWhenAnEdgeLandsInIt`,
  not `TestTrim3`.
- **A failure says what went wrong** in words: what was asked, what came
  back, what was wanted.

## 10. Unit tests

**The details are in [unit.md](unit.md).** In short: concurrent code is
tested from several threads at once under the race detector, and a
concurrency test fails without its lock. The standard test library first.
Off the disk and off the network. Fuzz only what reads bytes from
outside, and run many more seeds than CI does when changing code under a
random test. A fake keeps the real one's limits.

## 11. End-to-end and interface tests

**The details are in [end-to-end.md](end-to-end.md),** read before writing
or fixing one. In short: an end-to-end test does what the person did and
checks what they saw, against the real backend and the product's own
wiring, in the engine the product runs on. It checks only what the
window can show, reads everything a rule judges at one moment, waits on
a barrier built from signals, and judges motion by the frames drawn. A
new case is a step in a test that is there, not a new test.

## 12. CI holds the rules

Everything in this skill that a tool can check, a tool checks, in CI, on
every pull request: no sleep or poll in a test, no skipped or focused
test slipping in, every test can fail, no data race, no retries. A rule
that is only written down slips. Which tools hold which rule, and how CI
runs the tests, is the ci skill. **CI is the proof that counts:** push
once the tests of the change pass, and let CI run the rest. An agent
never runs the whole suite again on its own machine after the push, and
hears of a failing run by its pull request's events.

## 13. Review and handing over

- **An independent review, once the work is done.** The review skill
  says who reviews, when and what for. Of the tests, the reviewer breaks
  the code to see them fail, and looks for a test that passes by
  coincidence, a test that checks code against a copy of its own rule,
  and a wait on time.
- **Review your own work for design first.** A change of thousands of
  lines that is still broken is a sign the design is wrong. Ask why it
  needs so much code.
- **The tests prove the behaviour. The user only looks.** The steps to
  try a change by hand are a click or two. When the user only needs to
  see a state that is hard to reach, an error after retries or a message
  for a rare case, give them a quick way to see it, a temporary page or
  a temporary commit that shows every state with made-up data, removed
  before the merge. Every step they are given exists in the product as
  they have it and can be done at a person's speed. A case faster than a
  hand is proven by a named test, and the step only looks at the result.
- **Visual choices iterate fast.** Render several candidates from the
  real code side by side, rather than one build per idea. Compare a
  visual behaviour with the version before it and with the established
  way to do it, and measure it, before handing it over.
- **A proof that needs the user's machine says so.** Prove what can be
  proven on yours, against fakes, and say which number only their
  machine gives and where they will find it.
- **Logs make the product testable in the field.** Structured logs, from
  an established library, at the places a bug would be found, tagged with
  what they are about. One report from a person using the product should
  be enough to find the bug, without a second round.

## 14. Where other advice says otherwise

General testing advice is good on the whole. On these points it is
wrong, and this skill wins:

| Common advice | Why not | Instead |
| --- | --- | --- |
| `assert.Eventually`, `waitFor`, condition-based waiting up to a deadline | a poll with a deadline is a sleep in a loop | the event for that step, section 4 |
| sleep a few seconds for a service to start | a guess about time | the service says it is ready, and the test hears it |
| a test's own timeout on `time.After` or a timer | a guess about time, and it hides where it hung | the test runner's own timeout |
| `networkidle`, wait for the page to be quiet | quiet is a guess about time | the event the page sends when the step is done |
| retry a flaky test, in the runner, in CI or in a loop in the test | a test that passes on its second try is failing | no retries, section 1, and find the cause, section 7 |
| load the machine to make a flaky test fail | it changes every timing at once and proves nothing when it passes | the trace, then the one order held on purpose, section 7 |
| turn the test cache off to be safe | every run pays for every test | keep the cache, section 8 |
| a coverage number as the bar | coverage is not verification | what matters verified, and mutation testing, section 1 |
| a dependency injection container | more machinery than a constructor | plain constructor injection, section 5 |
| tags or flags to keep slow tests out of the usual run | slow tests stay slow and go unrun | make them fast, or skip with a reason when a program is missing |

## Before saying done

1. The test guards a behaviour a person relies on, named in one sentence,
   and the line whose removal makes it fail is named and was removed
   once.
2. It covers the kind, not only the case: every variant, every place, and
   what must stay the same.
3. Nothing already tested it, or the old test was strengthened instead.
4. It failed on the main branch, for the reason given, and passes now.
5. It never waits, and nothing in it defeats the test cache.
6. A change to tests or CI has its numbers against the main branch, and
   the end-to-end tests take no longer than they did there.
7. CI passed on the last commit, and every rule here that CI does not
   check yet has an issue asking for the check.
