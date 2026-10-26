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
  that lags behind the hand: read the pixels, the box, the frame, the
  time it took. The user should never be the first to find it.
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

What time-based tests look like, all of them wrong:

```go
// Wrong: hopes the job is done after 100 ms.
q.Add(job)
time.Sleep(100 * time.Millisecond)
check(t, q.Find(id))

// Wrong: a poll with a deadline is the same guess, repeated.
deadline := time.Now().Add(5 * time.Second)
for q.Find(id).State != Done {
	if time.Now().After(deadline) {
		t.Fatal("timed out")
	}
	time.Sleep(10 * time.Millisecond)
}

// Wrong: a test's own timeout. A hang is the test runner's to end,
// and Go's prints every goroutine when it does.
select {
case <-done:
case <-time.After(2 * time.Second):
	t.Fatal("too slow")
}
```

```js
// Wrong: gives the click half a second.
await page.click("text=Save");
await page.waitForTimeout(500);

// Wrong: "nothing happened for 250 ms" is a guess about time too, and
// so is waiting for the network to be idle.
await page.waitForLoadState("networkidle");

// Wrong: a setTimeout, Date.now or performance.now to decide when to
// go on.
await new Promise((r) => setTimeout(r, 300));
```

What event-driven tests look like:

```go
// The code says when the job ends, and the test hears it. No time.
done := make(chan Job, 1)
q := NewQueue(WithOnEnd(func(j Job) { done <- j }))
q.Add(job)
got := <-done
check(t, got)
```

```js
// The page resolves a promise when the step is done, and the test
// awaits it. It returns the moment the work ends, not a moment later.
await page.click("text=Save");
await page.evaluate(() => window.saved);
```

The names in these are made up. Find the real event where the test is,
and where there is none, add it to the code.

A helper that checks a condition again and again until it holds, like
`waitForFunction`, `Eventually` or `waitFor`, is a poll. Use it only to
read a state the page already shows. To go on when something happens,
await the event that says it happened.

**A helper every step goes through is lean.** A function run after every
step of every test, to let the app settle, is paid thousands of times a
run. A callback in it that does nothing, or a duration hidden in it,
costs minutes.

**Time the product itself defines runs on a clock the test moves.** A
message that leaves after 10 s, a retry every 30 s, a delay of 150 ms
before a search starts: that time is behaviour, and the test checks it,
but it never lets real time pass. It moves a fake clock past the moment
and checks before and after, in a fraction of a second:

```go
// The retry every 30 s, checked over 90 s of fake time, in no time.
synctest.Test(t, func(t *testing.T) {
	r := startRetries(fake)
	time.Sleep(90 * time.Second) // the bubble's clock is fake
	synctest.Wait()
	if r.tries() != 4 {
		t.Fatalf("tried %d times in 90 s, want 4", r.tries())
	}
})
```

Use the fake clock the language or the test tool already has: Go's
`testing/synctest`, the fake timers of Vitest or Jest, Playwright's
`page.clock`, `freezegun` or `time-machine` in Python, or a clock handed
in, see section 5. A duration in a test is fine, it is a number. A check
that bans the clock in tests must never block a clock the test controls.

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
shape.

### Hand things in

Code that touches what a test must control takes it from outside instead
of reaching for it itself. That is dependency injection, and it means
nothing more than a field, a parameter or an option:

| What | Handed in as |
| --- | --- |
| the time of day | a `now` function, or a clock interface |
| timers and timeouts | a clock that can be moved, or the language's fake clock around the test |
| the disk | a file system interface, with one in memory for tests |
| random numbers | a source of randomness |
| a network service, a model, a database | an interface, with a fake the test drives |
| a program the code runs | its path or a runner |
| the end of work | a channel, callback or event the caller gives |

- The program hands in the real thing. A test hands in one it drives.
- Plain constructor injection, the simplest form. No container library.
- A new piece of code that keeps time, reads a file, calls a service or
  does work on its own thread takes these from outside from the start.
  Adding it later is a refactor nobody gets round to, and the test that
  needed it got a sleep instead.
- The interface the same: a module that keeps time takes its timer, or
  is tested under fake timers.

### Shapes that test well

- **Concerns apart, behind small interfaces.** A part with a clear
  interface is tested alone, and a change to it runs only its own tests,
  section 8. Two jobs mixed in one place are hard to test and hard to
  even talk about.
- **One source for each piece of state.** When one part of the product
  knows something happened and another does not, they disagree. State
  kept in two places is a bug waiting for an order of events, and no
  test finds every order.
- **One mechanism, used everywhere, not a copy.** A countdown of 8 s in
  one place and 10 s in the rest, a button styled anew in each view: a
  copy starts out the same and drifts. One mechanism has one set of
  tests and is right everywhere at once. One renderer for the preview and
  the final output makes a mismatch impossible, which beats a test that
  compares two renderers. A test that fails when a second copy appears
  keeps it that way.
- **One path for every input.** When every input goes through the same
  steps, one set of tests covers them all. A special path for one kind of
  input needs its own tests and drifts.
- **A fix makes the design simpler, or it is the wrong fix.** Ask whether
  it streamlines the design or adds an exception that works around what
  is already there. A fix that is generic and sound makes the next case
  of the same kind right by design.
- **Known problems have known answers.** An animation curve, a cache, an
  update flow, a retry: look up how established software does it before
  inventing. The spec then comes from that, and the test checks it.
- **Numbers from first principles.** Check that a cost makes sense before
  accepting it because the arithmetic adds up. Why does one small image
  take 10 MB? Why does loading take half a minute? Design from what the
  product already knows.

## 6. When a test fails, or a bug is reported

A failing test is a question about the product first and about the test
second. A bug report is the same question.

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
   - A test that fails now and then fails for a reason: find the event
     it misses or the order it assumes, with the tools in section 7,
     in their order. Loading the machine comes last.
   - A vague report of instability, with no case that fails every time,
     is still a report. Look at the design, its concurrency and its order
     of events.
4. **Decide which of three is wrong:**
   - **The code.** Fix the code, the test stays as it is.
   - **The test.** It checked something nobody specified, read the page
     before the step had happened, or depended on time. Fix it so it
     checks what matters, on an event.
   - **The design.** The behaviour the test protects does not serve a
     person, or the spec contradicts itself, or the code is shaped so the
     right behaviour cannot be tested without waiting. Tests on a broken
     design protect the broken design. Say so to the user plainly, log an
     issue, and fix the design when that is what the change is for. No
     patches on top: find the root cause, and refactor if that is what it
     takes.
5. **Size it.** How big is the difference, and where would a person see
   it? A finding too small to matter is closed as won't fix, with its
   number.
6. **Look around it.** Where one bug was, try the gestures and states
   next to it. Each new case found gets its own failing test, or its own
   issue when it is outside the change.
7. **Fix for every machine.** A fix tuned to one machine, its screen, its
   speed or its system, is a bug on the next machine.

Never, to get green: loosen a check, raise a timeout, add a sleep or a
retry, skip, quarantine or remove a test, or change the expected value to
what the code now says without knowing why it changed.

## 7. Flaky, red and hanging checks

### Finding why a test fails now and then

A test that fails now and then has met an order of events it did not
expect. In a single-threaded language, a page, Node, Python's asyncio,
nothing touches the same memory at once, so there is no data race to
detect. What fails is the order: the test read the result before the
step had finished, or waited for a signal that had already gone by, or
the code under test met the two halves of its work in an order nobody
thought of. Find that order with these, in this order:

1. **Read what the failure left.** The runner keeps a trace of a
   test that failed, and keeps it in CI with the run. Playwright's
   trace has every action, the console, the network and, where a
   project pays for them, a film of the screen and snapshots of the
   page before and after each action. Read what the project records before
   looking for what it left out. The race detector has its
   report, and `go test -timeout` prints every goroutine. Most flakes are
   explained there, without running anything. Where the runner keeps
   no trace of a failure yet, that is the first thing to fix. A trace
   is several records of different cost. The calls, the console and
   the network are text the runner already has, and cost next to
   nothing. A film of the screen or a snapshot of the page costs every
   run real time. Keep the cheap records on for every test, and turn
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
- **A manual step to get tests passing is a defect.** A stale build
  rebuilt by hand, a cache cleared by hand: fix the build so the step is
  never needed again.
- **A guard that is removed says why.** A lint rule, a check or a test
  taken out says in the pull request what it guarded and how the code is
  better without it.

## 8. Fast, and cheap to run

A test suite is run thousands of times. Every second in it is paid on
every push, by the people waiting on CI and by the machines.

- **Run only what a change reaches.** The tests of what changed and of
  what depends on it, chosen from the diff. That simple rule beats a
  clever cache. Prefer a tool's own feature, a test runner's affected
  mode, a CI path filter, to a script of ours.
- **Use the caches there are, and never defeat them.** Test runners and
  build tools cache results until their inputs change. So:
  - never turn the test cache off across the board, like Go's
    `-count=1`. It is for a test that reads what the cache cannot see.
  - no reading of the time, a random seed or a file that changes every
    run into what a test checks, or its result is never cached.
  - keep the test selection flags stable, caches are keyed on them.
  - CI keeps its build cache, its test cache, its dependencies and its
    downloads from one run to the next.
- **Where a cache cannot apply yet, push for one.** Rerunning what did
  not change is a cost to remove, not a fact to accept. Say so to the
  user or log an issue with what it would save.
- **Every CI job has a short limit, and a job that grows past it is
  split**, never given a longer limit. Expensive machines run only what
  differs on them. A run for an old commit is cancelled when a new one
  comes.
- **Fast by design.** No waits, section 4. The lowest level that can see
  the behaviour, section 3. The smallest and shortest fixture that shows
  it. Slow tests in parallel, unless they change what the whole process
  shares, and then a comment says which. Off the disk, with a file system
  in memory. A mockup or a screenshot to settle where something goes
  needs no realistic fixture at all.
- **Every test file and every CI job earns its place.** Say what it
  guards and what it costs. Test tooling stays small, uses the tools'
  own features, and keeps fixtures as plain files, not text inside a
  script.
- **Work on the tests proves itself against the main branch.** The point
  of it is tests that are faster and more reliable. A change to tests or
  CI shows its numbers before and after, on the same machine, in a table.
  What does not measure faster or more reliable is dropped. Before
  speeding anything up, measure each step and understand where the time
  goes, and look up what is already measured first. Then keep your eyes
  on the goal: once every wait is an event, a test that is no faster was
  already as fast as it could be.

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

- **Concurrent code is tested from several threads at once,** under the
  race detector where the language has one. Anything asynchronous, a
  queue above all, has a test that does everything callers can do to it
  at the same time, while work runs.
- **The standard test library first.** An assertion library adds little,
  and the ones with `Eventually` add a poll.
- **No build tags or flags to hide slow tests.** A test that needs a
  program that is not installed skips with a reason, and CI installs it.
- **Off the disk, off the network.** A test that must touch the real
  disk or another program says why, on the test itself.
- **Fuzz only what reads bytes from outside**: a file the product opens,
  an answer from a service, a key, a webhook, a path. Everything else
  gets its basics as a table, deterministic. A crash the fuzzer finds is
  kept as a case with its fix, so every run plays it.

## 11. End-to-end and interface tests

- **An end-to-end test does what the person did and checks what they
  saw,** against the real backend. A fix to the interface comes with one
  that fails on the main branch and passes with the fix.
- **The whole product, the way a person uses it.** An end-to-end test
  goes from one part of the product to another, as a person does. A test
  of one component is no replacement for it.
- **A test only proves what its fixture contains.** Make fixtures the way
  real inputs are: the encodings, sizes and odd shapes that come in from
  outside. Know what a fixture hides: a video at a low frame rate hides
  what lasts less than a frame, a steady signal hides what only shows
  when it varies, data with one row hides what only two rows break.
- **Each shape of a thing.** The simple case and the one with every
  option, and a test meant to cross a boundary fails unless it really
  crossed it.
- **Test what ships.** The product built from the code under test, with
  the programs and libraries it ships with, set up as close to the
  customer's as the test machine allows.
- **The engine the product really runs on.** Where the product runs in a
  browser engine or a platform of its own, run the same steps there too
  before saying it works there. Typing, focus, hover, painting and timing
  are where engines differ.

## 12. CI holds the rules

Everything in this skill that a tool can check, a tool checks, in CI, on
every pull request. An agent is not asked to remember to run a command,
and the user is not asked to catch what a machine can. A rule that is
only written down slips. A rule a check enforces holds, and it protects
the agent too, because the check fails before the user has to.

What CI should hold, with established, widely used open-source tools,
never a scanner of our own:

| Rule | Checked by, for example |
| --- | --- |
| no sleep, timer or real clock in a test | a linter that bans the calls in test files: golangci-lint's `forbidigo`, ESLint's `no-restricted-globals` and `no-restricted-syntax`, `eslint-plugin-playwright`'s `no-wait-for-timeout`, Ruff's banned-api rule |
| no poll with a deadline | the same linters, banning `Eventually`, `waitForFunction` with a timeout and the like |
| no skipped, focused or disabled test slipping in | the skip and focus rules of the ESLint plugins for Playwright, Jest and Vitest, and the same ban on skip calls in other languages |
| every test can fail | mutation testing: Stryker for JavaScript and TypeScript, Gremlins for Go, mutmut for Python |
| no data race | the race detector of the language, on every run |
| no test that touches the disk or the network without saying why | a check over the test files, or a sandbox that refuses them |
| no retries | the runner's retry setting checked at zero |
| no job over its limit | the job's own timeout in the workflow |

- **Only new breaks fail.** Where old code still breaks a rule, the
  check takes a baseline from the main branch, the linter's own
  new-from-revision mode or its list of suppressions, so a pull request
  fails only on what it adds, and the old breaks are an issue to burn
  down.
- **A pull request cannot weaken the check that judges it.** Changes to
  the checks themselves, their configuration and their baselines are
  visible in review and owned by the user, and a baseline only ever
  shrinks.
- **Where a rule has no check yet, push for one.** Log an issue that
  names the rule, the tool that would hold it and what it would have
  caught. Do not make up for it by asking anyone, agent or person, to run
  a command by hand.
- **CI is the proof that counts.** Push once the tests of the change
  pass, and let CI run the rest. A local run never holds back work CI
  has already proven, and an agent never runs the whole suite again on
  its own machine after the push: CI runs it on the machines it was
  sized for and answers first. A failing run on your pull request is
  your work, and you hear of it by its event, so the agent subscribes
  to its pull request's events as it opens it.

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

1. The test guards a behaviour a person relies on, named in one sentence.
2. It covers the kind, not only the case: every variant, every place, and
   what must stay the same.
3. Nothing already tested it, or the old test was strengthened instead.
4. It failed on the main branch, for the reason given, and passes now.
5. It never waits, and nothing in it defeats the test cache.
6. A change to tests or CI has its numbers against the main branch.
7. CI passed on the last commit, and every rule here that CI does not
   check yet has an issue asking for the check.
