---
name: testing
description: How tests are written, fixed and reviewed in Frame Fairy, the Go tests, the interface tests and the walks, and how a bug is found before it is fixed. Read it before writing, changing, fixing or reviewing any test, before fixing a reported bug, and before code that keeps time, waits on work or reaches the disk, a program or the clock. Where another skill says otherwise about tests, this one wins.
---

# Testing in Frame Fairy

A test is not the work. The work is knowing that what a person relies on
in the app does what it should, every time, on any machine, and finding
out in seconds when it stops. A test is how we know. Everything below
follows from that.

CLAUDE.md holds the rules, under Engineering rules and Keeping the tests
quick. This skill says how to keep them and why, much of it in Tim's own
words from the threads where he asked for it. Where a general skill says
something else about tests, this one wins. The list of those places is
at the end.

## 1. Verify what matters

- **Start from the product, not the code.** Before a test, say in one
  sentence what a person would see go wrong if this broke: a clip that
  cuts a word in half, a caption that shows the wrong word, a search that
  never ends, a video the app removed from disk. That sentence is what
  the test guards. If there is no such sentence, there is no test to
  write.
- **What the product should do is written down.** CLAUDE.md under The
  project and the Interface rules, the doc of the part in `docs/`, the
  issue. A test checks that, not what the code happens to do today.
- **When nobody can say why a step is in the product, question the
  feature, not only the test.** Tim, on a test of crops per camera angle:
  "I don't understand why I should test this ... So I think this
  shouldn't even be a feature." A test that protects a behaviour nobody
  wants keeps it alive.
- **What a person can see can be measured, and what can be measured is
  tested.** A thumbnail a little darker than the short, a highlight that
  stopped growing, a caption that moves a pixel: read the pixels, the
  box, the frame. Tim: "why do I need to find this in the review? Why is
  this not already measured and verified automatically?"
- **A promise to customers is proven, not assumed.** The app never
  deletes a customer's video. It never listens to keys it was not given.
  Such a promise has a test or a check in CI that fails the day it
  breaks.
- **A test catches a break.** Name the change to the code that would make
  it fail. If only a deliberate decision can fail it, a wording, a
  constant, the shape of a private struct, it is a change detector. It
  fails on every redesign and sleeps through every bug, so it does not go
  in.
- **Expected values are written by hand.** Never work out what is
  expected with the code under test or its helpers, because then both
  sides are wrong together. A walk that checks the code against a copy
  of the code's own rule proves nothing. A table of cases with literal
  wants is the usual shape.
- **Coverage is a map, not the goal.** We want every behaviour that
  matters covered, so coverage should be high. But a covered line is not
  a verified line: a test that runs code and checks little reports 100 %
  and proves nothing, and a product can fail with every line covered.
  High coverage from weak tests is worse than lower coverage from strong
  ones, because it looks safe.
- **The mutation check.** Before calling a test done, break the code it
  guards on purpose: a wrong constant, a branch removed, an empty answer,
  a check on empty input left out. Run the test and see it fail, then
  put the code back. A break no test catches is a behaviour nobody
  verifies, or a test that cannot fail. Do it again when an existing
  test is rewritten, so the rewrite still proves what the old one did.
  Tim: "make sure that the refactoring of the tests still allows for the
  verification of the underlying premise".
- **Not every line earns a test.** Glue that only passes values on, code
  a library already tests, a log line's wording: leave them. Spend tests
  where a mistake would reach a person.
- **A rule that matters is enforced by a check, not only written down.**
  Tim: "sometimes you tell me that something is locked and from now on
  we do it this way, but then maybe it works and maybe it doesn't." The
  lint that fails a sleep in a test, `scripts/plan-frozen.sh`,
  `scripts/pins.sh` and `internal/offdisk` are such checks.

## 2. What a test covers

A bug Tim reports is one case of a kind. The test covers the kind.

- **Every variant.** Both edges, in and out, all four corners and all
  four sides, left to right and right to left. Tim, on a caption edge:
  "make sure that this works from all sides and all directions."
- **Every place the behaviour appears.** Escape that closes one list
  closes every list and field in the app, so the fix and its walk cover
  all of them, not only the control in the report. Tim: "This doesn't
  only apply to this captions settings area. It applies to the entire
  app." The same gesture behaves the same on every kind of element: "why
  on earth is the click behavior in the clip timeline different
  depending on what elements you interact with".
- **What must stay the same.** A change to one caption block leaves every
  other block where it was. A fix to the bounce leaves the line height
  alone. Check what the fix must not touch as well as what it changes,
  because Tim judges a fix by what else it moved.
- **Two paths to one result agree.** Dragging the edge of a caption over
  a word and removing that word end the same way. A test runs both and
  compares them.
- **Things that are separate stay separate.** A cut and a caption block
  are distinct parts of a clip, so moving one never moves the other. A
  test proves the one does not touch the other.
- **The whole class of action sequences.** Tim, on a removed clip that
  kept its removed caption: "there are probably a couple of other
  modalities in which this type of sequence of actions may cause those
  types of problems, which I would like to get tested". Try the orders
  around the one he hit: remove then undo, undo then edit, edit during a
  search.
- **Again, and fast.** A gesture repeated, a double-click after a drag,
  New pressed ten times. "It worked once, but then I dragged it again
  and double clicked again, and then it didn't work anymore."
- **The edges of the input.** A video of 15 s and one of four hours, a
  session left open for ten minutes, an empty transcript, a clip at the
  very end of the video.
- **Everything that should change in one moment changes in the same
  frame.** The crop, the video preview and the captions of a clip card
  just clicked. A walk reads the frames and checks they moved together.
  Lag is behaviour too: a playhead that trails the hand, an animation
  that is not fluid, is measured frame by frame and fails a walk.
- **The core path is always guarded.** Adding a video and playing it is
  what everything else stands on. Tim: "how are we verifying that just
  this mere fact that you can look at a workspace and play the damn
  video, that this just works?"
- **Rules after every step.** Some breaks can happen anywhere, like a
  still frame decoded smaller than the video preview, the blurry first
  frame Tim found, now `stillBlurred` in `walks/rules.mjs`. Those are
  checked by the walks after every step, see docs/TESTING.md, so a new
  walk gets them for free.
- **Invalid states are made impossible, not tested.** Where the types or
  the data can rule out a state, they do, and there is no error path to
  test.

## 3. Is it already tested?

Issues describe the same behaviour in different words, so the same thing
gets tested twice by people who did not know of each other. Two tests of
one thing cost two maintenances and say nothing new. Before writing one:

- **Search the tests** for the function, the gesture and the words of the
  behaviour: `grep -rn 'Reshape' --include=*_test.go .`, and in walks
  `grep -rn 'trim' frontend/preview/walks/`. The sequences are listed in
  `docs/TESTING.md`.
- **Ask the coverage** which lines the existing tests already run:

  ```
  go test -coverprofile=/tmp/c.out ./engine
  go tool cover -func=/tmp/c.out | grep -i reshape
  go test -run '^TestReshape' -coverprofile=/tmp/one.out ./engine
  ```

  The last shows what one test reaches, so two tests of the same lines
  are found.
- **Strengthen rather than add.** When a test covers the code but missed
  the bug, the test was weak: add the case to its table, or make its
  check exact. A second test beside it is the copy CLAUDE.md warns of,
  one primitive per thing applies to tests too.
- **A regression Tim finds asks why the tests missed it.** "I would have
  thought that we have tests for this. So it's kind of unacceptable that
  I have to find this now again." Say in the pull request which test
  should have caught it and why it did not, and fix that test.
- **One behaviour, one level.** Test a thing at the lowest level that can
  see it. The logic lives in the engine, and the interface shows what the
  engine says, so where a clip's edge lands is a Go test. What the person
  sees of it is an interface test or a walk. A walk does not check again
  what an engine test proves, it checks what the person sees and does.
  When a bug lives in logic in the interface, the logic moves to the
  engine, under a Go test. Tim: "the idea of the front end is that there
  isn't much logic in the front end at all".

## 4. Tests never wait

This is the rule that matters most here, and the one most often broken.

**A test runs straight through.** It sets up the world, takes a step and
checks the result, and then the next step. There is no moment where it
stands by in the hope that some work has finished. Everything that moves
is in its hands: the clock, the order work is done in, what the model
answers, what the disk holds, what the Go side sends.

**Event-driven means the test moves on events, not on time.** An event
is something that has happened and says so. A test gets to the next step
in one of two ways, best first:

1. **It causes the event itself.** It calls the function and has the
   answer, it moves the fake clock forward, it hands the queue a job and
   steps it. Nothing runs that the test did not start, so there is
   nothing to wait for.
2. **It receives the event the code sends for exactly that step.** A
   channel the code closes when the job is done, the job's news, a
   callback, a program writing that it is ready, the app resolving a
   promise when the step has been drawn. Where work crosses into another
   program, the browser or ffmpeg, this is how the test hears it ended.
   The event comes the moment the work is done, so this is not a wait,
   it is a reply.

**Never the clock.** A test that sleeps, or looks again and again until
a deadline, or gives a step some milliseconds to finish, is time-based,
and time-based tests are wrong:

- A wait long enough on one machine is too short on a busy one, so the
  test fails now and then, on someone else's machine or in CI, for no
  reason in the code. The flaky walks of #290, #324 and #325 came from
  such waits.
- The waits that are long enough add up to minutes of CI on every push.
- It checks something nobody asked for, that the work usually ends
  within so many milliseconds, and not whether it is right.
- A poll with a deadline is the same guess in a loop. It is not the
  cure for a sleep, it is a sleep.

**Load may make a test slower, never make it fail.** A test that passes
alone and fails with six bridges beside it, or on a busy runner, is
waiting on time somewhere. That is the bug, not the load.

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

// Wrong: a test's own timeout. A hang is go test -timeout's to end,
// and it prints every goroutine when it does.
select {
case <-done:
case <-time.After(2 * time.Second):
	t.Fatal("too slow")
}
```

```js
// Wrong: gives the click half a second.
await page.click("text=Search");
await page.waitForTimeout(500);

// Wrong: "nothing happened for 250 ms" is a guess about time too.
await quietFor(page, 250);

// Wrong: a setTimeout, Date.now or performance.now to decide when to
// go on, in a walk or an interface test.
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
// The app resolves a promise when the step is done, and the walk
// awaits it. It returns the moment the work ends, not a moment later.
await page.click("text=Search");
await page.evaluate(() => window.__settled);
```

These show the shape, and the names in them are made up. Find the real
event where the test is: the `emit` callback `newQueue` takes in
`cmd/framefairy-app/jobs.go`, the bridge's events in `walks/bridge.mjs`,
whose `settle` #459 turns from a quiet spell into an event, and the
fakes in `engine/project_test.go`.

`waitForFunction` checks a condition on every frame until it holds, so it
is a poll. Use it only to read a state the page already shows. To go on
when something happens, await the event that says it happened. What is
left of polls and fixed waits in the walks is #341, and in the Go tests
#394: never copy a pattern from there.

**A helper every step goes through is lean.** The walks' `settle` runs
after every step of every walk, so a callback in it that does nothing,
or a duration hidden in it, is paid thousands of times a run.

**Time the product itself defines runs on a clock the test moves.** A
toast that leaves after 10 s, a retry every 30 s, a delay of 150 ms
before the clip timeline reads its waveform: that time is behaviour, and
the test checks it, but it never lets real time pass. It moves a fake
clock past the moment and checks before and after, in a fraction of a
second:

```go
// The retry every 30 s, checked over 90 s of fake time, in no time.
synctest.Test(t, func(t *testing.T) {
	r := startRetries(fake)
	time.Sleep(90 * time.Second) //nolint:forbidigo // synctest bubble
	synctest.Wait()
	if r.tries() != 4 {
		t.Fatalf("tried %d times in 90 s, want 4", r.tries())
	}
})
```

In Go that is `testing/synctest`: `synctest.Test`, `synctest.Wait`, and
`synctest.Sleep` since Go 1.27, or a clock handed in, see section 5. In
an interface test, Vitest's fake timers, as `captions.test.ts` does. In a
walk, Playwright's own clock through `fakeClock`, `holdClock`, `runClock`
and `letClockGo` in `walks/bridge.mjs`, see docs/TESTING.md. No clock
library besides these. A `time.Duration` in a test is fine, it is a
number. A lint must never block a clock the test controls.

**A test that has to wait is a finding, not a test to fix by waiting
longer.** When a test can only go on after work that takes a while, ask
two questions before anything else:

- **Why is this work slow?** If it is slow for a person too, that is a
  bug or a performance issue in the app. Log it as an issue.
- **Is this the right test from the right place?** A test that waits for
  a transcription to check where a caption sits tests the recogniser
  through the window. Hand the window a transcript, with the fake
  recogniser, and test where the caption sits. A walk that waits for a
  search to find clips checks the engine through the browser. Give the
  bridge a model that answers at once. The slow part gets its own test at
  its own level, if it needs one.

**Where the code gives no event, the code gains one.** That is part of
the fix, not a detour from it. A test is never the place to work around
code that cannot say when it is done.

**The same holds for whoever runs the tests.** A session starts
`make changed`, the walks or the fuzzing in the background, ends its
turn, and is woken when they end. It never sleeps on them and never polls
a CI run, it is woken by the PR's events. Tim: "why is this even
time-based? Why isn't there an event that triggers the agent to respond?
This is just engineering 101."

## 5. Design for testing

Tim: "if the software design of the solution is already broken, then it
helps very little to write tests." Code that is hard to test is usually
code that is shaped wrong, and the fix is the shape.

### Hand things in

Code that touches what a test must control takes it from outside instead
of reaching for it itself. That is dependency injection, and here it
means nothing more than a field, a parameter or an option:

| What | Handed in as | Here already |
| --- | --- | --- |
| the time of day | `now func() time.Time` | `signer.WithClock` in `licence/signer/signer.go`, `Now` in `licence/dispenser/engine.go` |
| timers inside goroutines | `testing/synctest` around the test, or a clock handed in | the lint fixture `scripts/testdata/lint/go/signals` |
| the disk | `afero.Fs` | `engine/files.go`, the project and the app's store |
| random numbers | `io.Reader` | `signer.WithRandom` |
| the model, the recogniser | an interface with a fake | the fake recogniser and fake llama-server in `engine/project_test.go` |
| a program we run | the path or a runner | ffmpeg and the decoder through `FRAMEFAIRY_FRAMES` |
| the end of work | a channel or callback the caller gives | `emit` in `newQueue`, `cmd/framefairy-app/jobs.go` |

- The program hands in the real thing, `time.Now`, the disk, the real
  recogniser. A test hands in one it drives.
- Plain constructor injection, the simplest form. No container library,
  no wire, dig, fx or do, though golang-dependency-injection lists them.
- A new piece of code that keeps time, reads a file, runs a program or
  does work on its own goroutine takes these from outside from the
  start. Adding it later is a refactor nobody gets round to, and the
  test that needed it got a sleep instead.
- In the interface the same holds: a module that keeps time takes its
  timer, or is tested under fake timers, and the walks hold the app's
  clock with Playwright's.

### Shapes that test well

- **Concerns apart, behind small interfaces.** Tim asked for "more
  fine-grained and rather dedicated packages or modules that are tested
  in isolation". A part with a clear interface is tested alone, and a
  change to it runs only its own tests, section 8. Two jobs mixed in one
  place, like the navigation animation and the render that once shared
  code, are hard to test and hard to even talk about.
- **One source for each piece of state.** "If one part of the app knows
  that something happened, but the other part doesn't, then there's a
  disconnect." State kept in two places is a bug waiting for an order of
  events, and no test finds every order.
- **One mechanism, used everywhere, not a copy.** A countdown of 8 s in
  one toast and 10 s in the rest, a button styled anew in each place:
  "Did you build this all yourself again?" One mechanism has one set of
  tests and is right everywhere at once. One drawing of the captions for
  the video preview and the short makes a mismatch impossible, which
  beats a test that compares two drawings. `TestVideosAreNamedOnce` is a
  test that keeps a second copy from coming in.
- **One path for every input.** When every video goes through the same
  steps, one set of tests covers them all. A special path for one kind
  of input needs its own tests and drifts.
- **A fix makes the design simpler, or it is the wrong fix.** Tim: "would
  this streamline our software design and make it more elegant, or would
  we create an exception and work around other mechanisms that we
  already have in place?" A fix that is generic and sound makes the next
  case of the same kind right by design, "so that the next type of
  caption animation will not produce the same fault".
- **Known problems have known answers.** An animation curve, a cache of
  frames, a way to update: look up how others solve it before inventing.
  "We are not reinventing the wheel here." The spec then comes from how
  established tools do it, and the test checks that: one easing curve,
  "whether it's for one letter, for a short word, for a long word".
- **Numbers from first principles.** Check that a cost makes sense
  before accepting it because the arithmetic adds up. "Why do we have to
  allocate 10 megabytes for a single frame?" "Why does it take almost
  half a minute to load a model?" Design from what the product already
  knows, like the frames a person can jump to.

## 6. When a test fails, or a bug is reported

A failing test is a question about the product first and about the test
second. A report from Tim is the same question.

1. **Reproduce it first, here, as it happened.**
   - Use the file Tim used. Test videos he shared are in the commit
     history and his uploads, and his Report a Problem zips are in the
     project files: grep their `logs/app.log` before asking him to do
     anything again. "My test is in the report."
   - Go to the moment the log names, not a moment you pick.
   - Reproduce it in the cloud. Ask him to run a command on his Mac only
     for what only the Mac can show, like VideoToolbox, and keep that
     rare. "Why can't you run this command in the cloud?"
   - See the exact failure before the fix and see it gone after. A fix
     that was never seen failing has not been shown to fix anything.
2. **Read what should happen.** The spec of the part: CLAUDE.md, the doc
   in `docs/`, the issue, what Tim said. Then read the code and the
   test.
3. **Find the cause.**
   - "It worked this morning" means something changed. Bisect main with
     the walk, and explain why it worked before, not only why it fails
     now.
   - Several symptoms after a run of merges often have one cause. Look
     for it before fixing each, or the fixes stack up "on top of a house
     of cards".
   - A test that fails now and then fails for a reason: find the event
     it misses or the order it assumes. Run it many times, under
     `-race`, on a loaded machine, `go test -count=50 -run '^TestX$'`.
   - A vague report, "not always deterministic, I don't have a specific
     case that fails", is still a report. Look at the design, its
     concurrency and its order of events.
4. **Decide which of three is wrong:**
   - **The code.** Fix the code, the test stays as it is.
   - **The test.** It checked something nobody specified, read the page
     before the step had happened, or depended on time. Fix it so it
     checks what matters, on an event.
   - **The design.** The behaviour the test protects does not serve a
     person, or the spec contradicts itself, or the code is shaped so the
     right behaviour cannot be tested without waiting. Tests on a design
     that is broken protect the broken design. Say so to Tim plainly,
     log an issue, and fix the design when that is what the pull request
     is for. No patches on top: "Find the root cause and improve the
     design. Do some refactoring if you have to."
5. **Size it.** How big is the difference, and where would a person see
   it? A finding too small to matter is closed as won't fix, with its
   number. Tim: "We are not going to fix a 0.2 % issue in the preview."
6. **Look around it.** Where one bug was, try the gestures and states
   next to it. "Maybe you find something that's buggy or wrong by
   design." Each new case found gets its own failing test, or its own
   issue when it is outside the pull request.
7. **Fix for every machine.** A fix tuned to Tim's Mac, its refresh rate
   or its speed, is a bug on the next machine.

Never, to get green: loosen a check, raise a timeout, add a sleep or a
retry, skip, quarantine or remove a test, or change the expected value to
what the code now says without knowing why it changed.

## 7. Flaky, red and hanging checks

- **Flaky tests come first.** A test that fails now and then teaches
  everyone to ignore red. Rank them by how often they failed in CI,
  the nightly runs too, and fix the worst first. A nightly job that has
  never passed is among the worst.
- **A check that hangs is fixed at once**, in its own pull request, and
  never lived with. It is also no reason to hold back work CI has already
  proven.
- **Red CI is explained before it is rerun.** Tim: "are we doing anything
  wrong or is this just GitHub being unreliable". Read the check-run
  annotations and compare the step times with main's last runs. A runner
  that was never given, or was lost, says so there. A failure of ours is
  ours, whatever pull request it showed up on.
- **A manual step to get tests passing is a defect.** A stale decoder
  rebuilt by hand, a cache cleared by hand: fix the Makefile or the
  script so the step is never needed again.
- **A guard that is removed says why.** A lint rule, a check or a test
  taken out says in the pull request what it guarded and how the code is
  better without it. "How is the code base getting better by removing
  it?"

## 8. Fast, and cheap to run

A test suite is run thousands of times. Every second in it is paid on
every push, by Tim waiting on CI and by the machines.

- **Run only what a change reaches.** `make changed` runs the tests of
  what a branch changed and what imports it, and on a pull request CI
  starts only the jobs `.github/ci-filters.yml` says a change needs.
  That simple rule beats a clever cache: Tim closed #457, a trick with
  file times, and asked why CI does not just use what `make changed`
  knows. Prefer a tool's own feature to a script of ours.
- **Use the caches there are, and never defeat them.** Go caches a
  package's test result until its code, its test files, the files it
  opened or the environment it read change, and CI keeps that cache from
  main, see docs/BUILD.md and #452. So:
  - no `-count=1` for unit tests, in the Makefile, a script or a
    workflow. It is for a test that reads what Go cannot see.
  - no reading of the time, a random seed or a file that changes every
    run into what a test checks, or its result is never cached.
  - keep `-run` and `-skip` patterns stable, Go keys the cache on them.
  - the build cache, the fuzz corpus, npm, the apt packages and the
    ffmpeg build are all kept in the Actions cache. A new tool or
    download in CI is cached the same way.
- **Where a cache cannot apply yet, push for one.** Rerunning what did
  not change is a cost to remove, not a fact to accept. Say so to Tim or
  log an issue with what it would save.
- **No job over five minutes, and a Mac only for what differs on a
  Mac**, as CLAUDE.md says under Keeping the tests quick. A job that
  grows past it is split, it never gets a longer limit.
- **Fast by design.** No waits, section 4. The lowest level that can see
  the behaviour, section 3. The smallest and shortest test video that
  shows it. `t.Parallel()` for a test over a second, unless it changes
  what the whole process shares, and then a comment says which. Off the
  disk, with `afero.NewMemMapFs()`. A mockup or a screenshot for a
  placement question needs no four-hour video: "Don't make it so
  complicated."
- **Every test file and every CI job earns its place.** Say what it
  guards and what it costs. Test tooling stays small, uses the tools'
  own features, and keeps fixtures as plain files, not text inside a
  script.
- **Work on the tests proves itself against main.** "The idea is that
  the tests get better and faster and more reliable." A change to tests
  or CI shows before and after on the same machine, in a table as
  CLAUDE.md says for measurements. What does not measure faster or more
  reliable is dropped. Before speeding anything up, measure each step
  and understand where the time goes. Look up what is already measured
  first, the speech benchmark, CI's step times, the docs. Then keep your
  eyes on the ball: once every wait is an event, a test that is no
  faster was already as fast as it could be, and that is fine.

## 9. Tests that last

A test is code we keep. It must still be right and readable in a year.

- **Through the behaviour, not the inside.** Test what a person or a
  caller sees, through the package's own functions, so a refactor that
  keeps the behaviour keeps the tests green.
- **Each test reads on its own.** What it sets up, what it does and what
  it expects are in the test, not three helpers away. Repeating a line
  in two tests is fine when it keeps each readable.
- **Shared setup lives in one place, extended, not copied.** The fakes in
  `engine/project_test.go`, the bridge and its helpers in
  `walks/bridge.mjs`. A second fake recogniser drifts from the first.
- **The name says the behaviour.** `TestReshapeKeepsAWordWholeWhenAnEdgeLandsInIt`,
  not `TestReshape3`.
- **A failure says what went wrong** in words: what was asked, what came
  back, what was wanted.

## 10. Go tests

- Every test runs under `-race`. Anything asynchronous, the job queue
  above all, has a test that does everything the app can do to it at
  once.
- The standard library and `afero`. No testify, so ignore
  golang-stretchr-testify, `assert.Eventually` above all.
- No build tags for slower tests. A test that needs ffmpeg skips without
  it.
- A test on the real disk says why in a comment that starts with "On the
  real disk", on the test itself.
- Fuzz only what reads bytes from outside: a model answer, a plan file, a
  caption file, a licence key, a webhook, the update feed, a path.
  Everything else gets the basics as a table, deterministic. A crash
  found goes into the target's `testdata/fuzz/` with its fix.
- `make lint` fails a new `time.Sleep`, `time.After`, timer or ticker in
  a test. Inside a `synctest` bubble the line says
  `//nolint:forbidigo // synctest bubble`.

## 11. Walks and interface tests

- A walk does what Tim did and checks what he saw, against the real Go
  side. How walks are built, run and read is docs/TESTING.md. What to
  measure is the interface skill.
- **Walks are the whole app, the way a person uses it.** They go from
  one part of the app to another, as a person does. A test of one
  component is no replacement for a walk.
- Walks use the `playwright` library with our own runner, not the
  Playwright test runner. Advice written for `playwright.config`,
  `expect(locator)` or test retries does not apply as written.
- **No retries.** A test that passes on its second try is a failing test.
- A fix to the interface comes with a walk that fails on main and passes
  with the fix.
- **A walk only proves what its video contains.** Make the test video the
  way real episodes are: HEVC with frames stored after their key frame,
  phone video stored on its side, uneven frame times, Matroska from OBS.
  The bridge's `/pick` options are in docs/TESTING.md. Know what a video
  hides: at 5 fps a 220 ms caption pop falls between frames, and a steady
  tone draws a flat waveform.
- **Each shape of a thing.** A straight clip and a clip with cuts. A play
  that is meant to cross a cut fails the walk unless it really crossed
  it.
- **Test what ships.** The app, the decoder and ffmpeg built from the
  code under test, set up as close to a customer's as the cloud allows.
- ESLint in `make interface` fails a new `waitForTimeout`, `setTimeout`,
  `setInterval`, `Date.now` or `performance.now` in a walk or an
  interface test.
- **Chromium in the cloud, and WebKit where the Mac differs.** The app on
  a Mac is WebKit. For typing, the caret, focus, hover and anything
  painted or timed, run the same steps in WebKitGTK as well before saying
  it works on Tim's Mac, see the interface skill. The Chrome DevTools
  server that browser-testing-with-devtools needs is not set up, use the
  preview harness in `frontend/preview/`.

## 12. Proof, review and handing over

- **An independent review, early.** Once the first real work is pushed,
  a separate review agent reads the design and the tests with fresh
  eyes, without waiting for CI. It gets Tim's own findings too. It
  breaks the code to see the tests fail, and looks for a test that
  passes by coincidence, a walk that checks code against a copy of its
  own rule, and a wait on time. Every finding is answered, fixed or
  argued.
- **Review your own work for design first.** A pull request of 7000
  lines that is still broken is a sign the design is wrong. "Why do we
  need so many code changes?"
- **CI is the proof that counts.** Push once the tests of the change
  pass, and run `make changed` in the background after. A local run
  never holds back work CI has already proven green. A failing CI run on
  your pull request is your work, and you hear of it by its event. When
  main no longer merges into the pull request, resolve that before
  running anything expensive, because that run would test a commit that
  cannot merge, and GitHub runs no CI on it.
- **The walks prove the behaviour. Tim only looks.** His How to test is a
  click or two. When he only needs to see a state, an error after
  retries or a message he cannot easily cause, give him a quick way to
  see it: a temporary commit or a page under Help that shows every state
  with made-up data, removed before the merge. Every step he is given
  exists in the app as he has it and can be done at a person's speed. A
  case faster than a hand, like a search that ends at once, is proven by
  a named test, and his step only looks at the result.
- **Visual choices iterate fast.** Render several candidates from the
  real code path side by side, rather than one build per idea. Compare a
  visual behaviour with the version before it and with the known way to
  do it, and measure it, before handing it to Tim.
- **A proof that needs Tim's machine says so.** The cloud has no models
  and no VideoToolbox. Prove what can be proven here against the fakes,
  and say in the pull request which number only his Mac gives and in
  which log line he will find it.
- **Logs make the app testable in the field.** Structured logs, from an
  established library, at the places a bug would be found, tagged with
  the video and the feature. One Report a Problem from a customer should
  be enough to find the bug, without a second round.
- **A check in CI is built on established tools**, widely used and
  reviewed, never a scanner of our own, and a pull request cannot
  weaken or skip the check that judges it.

## 13. Where other skills say otherwise

The general skills in a cloud session are good advice on the whole. On
these points they are wrong for this project, and this skill wins:

| Skill | Says | Here |
| --- | --- | --- |
| golang-stretchr-testify | `assert.Eventually`, `EventuallyWithT` | a poll with a deadline, never. We do not use testify |
| golang-testing | a 5 s sleep for a service to start, a timeout helper on `time.After`, the clockwork library, build tags for integration tests | an event, `go test -timeout`, synctest or a clock handed in, a skip without ffmpeg |
| golang-continuous-integration | `-count=1` to turn the test cache off, a coverage threshold in CI | keep the cache, section 8. Coverage is a map, section 1 |
| golang-troubleshooting | `go test -count=1 ./...` to get past the cache | `-count=N` on one test to find a flake is fine, never across the board |
| golang-dependency-injection | container libraries | plain constructor injection, section 5 |
| constraint-driven-development | a coverage number as the bar | what matters verified, section 1 |
| test-driven-development | flaky tests: "use deterministic assertions", Chrome DevTools for the browser | events and fake clocks, section 4. The preview harness |
| browser-testing-with-devtools | the Chrome DevTools server | not set up here |
| Anthropic's webapp-testing, superpowers' condition-based-waiting, if ever installed | `wait_for_timeout`, `networkidle`, a `waitFor` that polls up to a deadline | never, section 4 |

## Before saying done

1. The test guards a behaviour a person relies on, named in one sentence.
2. It covers the kind, not only the case: every variant, every place, and
   what must stay the same.
3. Nothing already tested it, or the old test was strengthened instead.
4. It failed on main, for the reason given, and passes now. The mutation
   check made it fail too.
5. It never waits. No new clock line in the diff, this prints nothing:

   ```
   git diff origin/main -- '*_test.go' '*.test.ts' frontend/preview/walks \
     | grep -E '^\+.*(time\.Sleep|time\.After|waitForTimeout|setTimeout|setInterval|Date\.now|performance\.now|Eventually)'
   ```

6. Nothing in it defeats the test cache.
7. A change to tests or CI has its numbers against main.
8. `make changed` passes, and the reply says so.
