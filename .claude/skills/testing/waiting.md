# Tests never wait, in detail

Section 4 of the testing skill says the rule. This is how it looks: a
wait on the clock and an event side by side, where the event has to sit,
the helpers every step goes through, and the fake clocks.

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

Where the event sits, and what the test does with it:

- **Wait for the step to end, then compare.** A wait for the exact value
  expected turns a wrong value into a hang with no message. Wait for the
  event that says the step is over, then check the value and fail with
  what it was.
- **The event comes after the last stage that can drop the result.** A
  signal sent before a filter, a merge or a later stage lets the test
  read a state that never reached the screen.
- **Wait on that step's own event, not on a general one.** A general
  "caught up" marker can overtake the last event of a job, and a wait for
  "the next event" hangs when nothing more happens, because events come
  only on change. Where the code has no event for the step, it gains one.
- **Read and act in one step.** A test that reads where something is and
  then clicks there in a second call across a process boundary races the
  product, which may have moved it in between. Do both in one call
  inside the product, or hold its events between them.
- **Prove that something did not happen without waiting.** A try-lock, a
  file lock taken without blocking, or reasoning from what the code has
  already decided, then one check. Never wait a while and see nothing.
- **Time a reaction inside the product,** from the input event to the
  first frame that shows it. A time taken in the test driver holds its
  own round trips, which grow on a busy machine: one read 538 ms where
  the page had answered in two frames.
- **A framework's default timeout is a deadline nobody wrote.** Set it to
  off, or to the runner's own limit, so a hang is ended where it is
  reported best.
- **A fake clock does not decide the order of threads.** It moves time,
  it does not schedule. Give each step its own moment, or hold it with a
  gate the test opens. Start from a state where no timer is due now, or
  the scheduler picks the order the test asserts.
- **A watchdog on something outside stays on the real clock.** A timer
  that gives up on a stalled stream or a hung program must keep running
  while a test holds the product's clock. It is tested under a fake clock
  of its own.

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
in, see section 5 of the skill. A duration in a test is fine, it is a number. A check
that bans the clock in tests must never block a clock the test controls.
