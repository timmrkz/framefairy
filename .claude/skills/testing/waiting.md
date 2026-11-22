# Tests never wait, in detail

Section 4 of the testing skill in detail. Wrong, in Go and Playwright:

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

// Wrong: a test's own timeout. The runner ends a hang, and prints
// every stack.
select {
case <-done:
case <-time.After(2 * time.Second):
	t.Fatal("too slow")
}
```

```js
// Wrong, all three: a guess about time.
await page.waitForTimeout(500);
await page.waitForLoadState("networkidle");
await new Promise((r) => setTimeout(r, 300)); // or Date.now, performance.now
```

Event-driven tests:

```go
// The code says when the job ends, and the test hears it.
done := make(chan Job, 1)
q := NewQueue(WithOnEnd(func(j Job) { done <- j }))
q.Add(job)
check(t, <-done)
```

```js
// The page resolves a promise when the step is done.
await page.click("text=Save");
await page.evaluate(() => window.saved);
```

The names are made up. Find the real event, or add one to the code.
`waitForFunction`, `Eventually` and `waitFor` are polls, only for reading
a state the page already shows.

Where the event sits, and what the test does with it:

- **Wait for the step to end, then compare.** Waiting for the expected
  value turns a wrong value into a hang with no message.
- **The event comes after the last stage that can drop the result.** A
  signal sent before a filter or merge lets the test read a state that
  never reached the screen.
- **Wait on that step's own event, not a general one.** A "caught up"
  marker can overtake a job's last event, and "the next event" hangs when
  nothing changes. Where the step has no event, the code gains one.
- **Read and act in one step.** Reading where something is, then
  clicking there in a second call across a process boundary, races the
  product. Do both in one call inside the product, or hold its events
  between them.
- **Prove that something did not happen without waiting:** a try-lock, a
  non-blocking file lock, or what the code already decided, then one
  check.
- **Time a reaction inside the product,** from the input event to the
  first frame that shows it. The test driver's time includes its own
  round trips: one read 538 ms where the page answered in two frames.
- **A framework's default timeout is a deadline nobody wrote.** Set it
  off, or to the runner's limit, which reports a hang best.
- **A fake clock moves time, it does not order threads.** Give each step
  its own moment, or a gate the test opens. Start with no timer due now,
  or the scheduler picks the order the test asserts.
- **A watchdog on something outside,** a stalled stream, a hung program,
  stays on the real clock while a test holds the product's, and is tested
  under a fake clock of its own.

**A helper every step goes through is lean.** It runs thousands of times
a run, so an idle callback or a hidden duration in it costs minutes.

**Time the product defines runs on a clock the test moves.** A retry
every 30 s or a 150 ms delay is behaviour, checked by moving a fake clock
past the moment, with no real time passing:

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

Use the fake clock the tool already has: Go's `testing/synctest`,
Vitest's or Jest's fake timers, Playwright's `page.clock`, `freezegun` or
`time-machine` in Python, or a clock handed in, see section 5 of the
skill. A duration in a test is just a number. A check that bans the clock
in tests never blocks one the test controls.
