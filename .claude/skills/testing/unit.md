# Unit tests

Section 10 of the testing skill in detail.

- **Concurrent code is tested from several threads at once,** under the
  race detector. A queue above all gets a test that does everything
  callers can do to it at once, while work runs.
- **The standard test library first.** Assertion libraries add little,
  and `Eventually` adds a poll.
- **No build tags or flags to hide slow tests.** A test needing a missing
  program skips with a reason, and CI installs it.
- **Off the disk and the network,** unless the test says on itself why it
  needs them.
- **Fuzz only what reads bytes from outside**: a file the product opens,
  a service's answer, a key, a webhook, a path. The rest gets table
  tests. A fuzzer's crash is kept as a case with its fix. A target whose
  only input is a random seed runs over fixed seeds instead.
- **CI's seeds and fuzz budget are a smoke test.** After touching code
  under a random or fuzz test, run many more seeds, or fuzz for a minute:
  sixteen seeds passed in CI where two hundred found fourteen failures.
  Every failing seed is kept and played on every run.
- **A concurrency test fails without its lock,** because the competing
  action lands inside the window the lock guards.
- **No fatal assertion off the test's own thread.** In Go a `Fatal` from
  another goroutine leaves the test running or hung. Prepare inputs
  before starting workers, and give helpers a form that records an error
  and returns.
- **A fake keeps the real one's limits,** its pacing, one request at a
  time, parallel streams. A generous fake hides the real order's bugs. A
  simulation used as proof is a test too: see it fail against a
  deliberately bad version.
- **What the machine decides is tested where it ships.** Permission tests
  pass for nothing as root, so they run unprivileged. A rounding rule is
  tested on the processor the product ships on.
