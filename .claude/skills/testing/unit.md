# Unit tests

Section 10 of the testing skill in detail.

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
  kept as a case with its fix, so every run plays it. A target whose only
  input is the seed of a random generator gains nothing from fuzzing: run
  it over fixed seeds.
- **CI's seeds and fuzz budget are a smoke test.** When a change touches
  code under a random or fuzz test, run many more seeds, or fuzz for a
  minute, before calling it done. Sixteen seeds passed in CI while two
  hundred showed fourteen failures. Every seed that failed is kept and
  played on every run.
- **A concurrency test fails without its lock.** The competing action
  lands inside the window the lock guards, and removing the lock makes
  the test fail. One where most actions land before or after the window
  passes with the lock gone.
- **No fatal assertion off the test's own thread.** In Go a `Fatal` from
  another goroutine ends only that goroutine, and the test goes on or
  hangs. Prepare inputs before starting workers, and give helpers a form
  that records an error and returns.
- **A fake keeps the real one's limits,** its pacing, its one request at
  a time, its streams side by side. A fake more generous than the real
  thing hides exactly the bugs of the real order. A simulation or a
  harness used as proof is a test too: run it against a deliberately bad
  version and see it fail.
- **What the machine itself decides is tested where it ships.** Tests of
  file permissions pass for nothing when CI runs as root, so they run as
  a user without rights. Floating point can differ between processors,
  so a rounding rule is tested on the processor the product ships on.
