---
name: testing
description: How tests are written, fixed and reviewed in Frame Fairy, the Go tests, the interface tests and the walks. Read it before writing, changing, fixing or reviewing any test, and before code that keeps time, waits on work or reaches the disk, a program or the clock. Where another skill says otherwise about tests, this one wins.
---

# Testing in Frame Fairy

A test is not the work. The work is knowing that what a person relies on
in the app does what it should, every time, on any machine, and finding
out in seconds when it stops. A test is how we know. Everything below
follows from that.

CLAUDE.md holds the rules, under Engineering rules and Keeping the tests
quick. This skill says how to keep them and why. Where a general skill
says something else about tests, this one wins. The list of those places
is at the end.

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
- **A test catches a break.** Name the change to the code that would make
  it fail. If only a deliberate decision can fail it, a wording, a
  constant, the shape of a private struct, it is a change detector. It
  fails on every redesign and sleeps through every bug, so it does not go
  in.
- **Expected values are written by hand.** Never work out what is
  expected with the code under test or its helpers, because then both
  sides are wrong together. A table of cases with literal wants is the
  usual shape.
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
  verifies, or a test that cannot fail.
- **Not every line earns a test.** Glue that only passes values on, code
  a library already tests, a log line's wording: leave them. Spend tests
  where a mistake would reach a person.

## 2. Is it already tested?

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
- **One behaviour, one level.** Test a thing at the lowest level that can
  see it. Where a clip's edge lands is the engine's, in a Go test. What
  the person sees of it is the interface's, in an interface test or a
  walk. A walk does not check again what an engine test proves, it
  checks what the person sees and does.

## 3. Tests never wait

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
`synctest.Sleep` since Go 1.27, or a clock handed in, see section 4. In
an interface test, Vitest's fake timers, as `captions.test.ts` does. In a
walk, Playwright's own clock through `fakeClock`, `holdClock`, `runClock`
and `letClockGo` in `walks/bridge.mjs`, see docs/TESTING.md. No clock
library besides these.

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

## 4. Design for testing: hand things in

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

## 5. When a test fails, or needs fixing

A failing test is a question about the product first and about the test
second.

1. **Read what should happen.** The spec of the part: CLAUDE.md, the doc
   in `docs/`, the issue, what Tim said. Then read the code and the
   test.
2. **Decide which of three is wrong:**
   - **The code.** Fix the code, the test stays as it is.
   - **The test.** It checked something nobody specified, read the page
     before the step had happened, or depended on time. Fix it so it
     checks what matters, on an event.
   - **The design.** The behaviour the test protects does not serve a
     person, or the spec contradicts itself, or the code is shaped so the
     right behaviour cannot be tested without waiting. Tests on a design
     that is broken protect the broken design. Say so to Tim plainly,
     log an issue, and fix the design when that is what the pull request
     is for.
3. **Find the cause.** A test that fails now and then fails for a reason:
   find the event it misses or the order it assumes. Run it many times,
   under `-race`, on a loaded machine, `go test -count=50 -run '^TestX$'`.
   It is a real failure that shows only sometimes.

Never, to get green: loosen a check, raise a timeout, add a sleep or a
retry, skip, quarantine or remove a test, or change the expected value to
what the code now says without knowing why it changed.

## 6. Fast, and cheap to run

A test suite is run thousands of times. Every second in it is paid on
every push, by Tim waiting on CI and by the machines.

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
  log an issue with what it would save. The walks today run with
  `-count=1`, because Go cannot see the files the browser reads.
- **Fast by design.** No waits, section 3. The lowest level that can see
  the behaviour, section 2. The smallest and shortest test video that
  shows it. `t.Parallel()` for a test over a second, unless it changes
  what the whole process shares, and then a comment says which. Off the
  disk, with `afero.NewMemMapFs()`.
- **Measure before saying faster.** Compare with main, the same way, in a
  table, as CLAUDE.md says for measurements.

## 7. Tests that last

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

## 8. Go tests

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
  caption file, a licence key, a webhook, the update feed, a path. A
  crash found goes into the target's `testdata/fuzz/` with its fix.
- `make lint` fails a new `time.Sleep`, `time.After`, timer or ticker in
  a test. Inside a `synctest` bubble the line says
  `//nolint:forbidigo // synctest bubble`.

## 9. Walks and interface tests

- A walk does what Tim did and checks what he saw, against the real Go
  side. How walks are built, run and read is docs/TESTING.md. What to
  measure is the interface skill.
- Walks use the `playwright` library with our own runner, not the
  Playwright test runner. Advice written for `playwright.config`,
  `expect(locator)` or test retries does not apply as written.
- **No retries.** A test that passes on its second try is a failing test.
- **A walk only proves what its video contains.** Make the test video the
  way real episodes are: HEVC with frames stored after their key frame,
  phone video stored on its side, uneven frame times, Matroska from OBS.
  The bridge's `/pick` options are in docs/TESTING.md.
- A fix to the interface comes with a walk that fails on main and passes
  with the fix.
- ESLint in `make interface` fails a new `waitForTimeout`, `setTimeout`,
  `setInterval`, `Date.now` or `performance.now` in a walk or an
  interface test.
- Chromium in the cloud. WebKit for what only goes wrong on the Mac, see
  the interface skill. The Chrome DevTools server that
  browser-testing-with-devtools needs is not set up, use the preview
  harness in `frontend/preview/`.

## 10. Where other skills say otherwise

The general skills in a cloud session are good advice on the whole. On
these points they are wrong for this project, and this skill wins:

| Skill | Says | Here |
| --- | --- | --- |
| golang-stretchr-testify | `assert.Eventually`, `EventuallyWithT` | a poll with a deadline, never. We do not use testify |
| golang-testing | a 5 s sleep for a service to start, a timeout helper on `time.After`, the clockwork library, build tags for integration tests | an event, `go test -timeout`, synctest or a clock handed in, a skip without ffmpeg |
| golang-continuous-integration | `-count=1` to turn the test cache off, a coverage threshold in CI | keep the cache, section 6. Coverage is a map, section 1 |
| golang-troubleshooting | `go test -count=1 ./...` to get past the cache | `-count=N` on one test to find a flake is fine, never across the board |
| golang-dependency-injection | container libraries | plain constructor injection, section 4 |
| constraint-driven-development | a coverage number as the bar | what matters verified, section 1 |
| test-driven-development | flaky tests: "use deterministic assertions", Chrome DevTools for the browser | events and fake clocks, section 3. The preview harness |
| browser-testing-with-devtools | the Chrome DevTools server | not set up here |
| Anthropic's webapp-testing, superpowers' condition-based-waiting, if ever installed | `wait_for_timeout`, `networkidle`, a `waitFor` that polls up to a deadline | never, section 3 |

## Before saying done

1. The test guards a behaviour a person relies on, named in one sentence.
2. Nothing already tested it, or the old test was strengthened instead.
3. It failed on main, for the reason given, and passes now. The mutation
   check made it fail too.
4. It never waits. No new clock line in the diff, this prints nothing:

   ```
   git diff origin/main -- '*_test.go' '*.test.ts' frontend/preview/walks \
     | grep -E '^\+.*(time\.Sleep|time\.After|waitForTimeout|setTimeout|setInterval|Date\.now|performance\.now|Eventually)'
   ```

5. Nothing in it defeats the test cache.
6. `make changed` passes, and the reply says so.
