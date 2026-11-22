# Design for testing

Code that is hard to test is usually shaped wrong, and the fix is the
shape. Section 5 of the testing skill sums this up.

## Hand things in

Code takes what a test must control from outside, as a field, a parameter
or an option:

| What | Handed in as |
| --- | --- |
| the time of day | a `now` function, or a clock interface |
| timers and timeouts | a clock that can be moved, or the language's fake clock |
| the disk | a file system interface, in memory for tests |
| random numbers | a source of randomness |
| a network service, a model, a database | an interface, with a fake the test drives |
| a program the code runs | its path or a runner |
| hardware or a system service, a printer, a driver, a keychain | a value the program hands in, absent by default |
| the end of work | a channel, callback or event the caller gives |

- The program hands in the real thing, a test one it drives.
- The test's double lives with the tests. The product keeps only the seam
  and the real default, never the double or a flag that picks it: a
  switch set wrong in a build gives a customer the test's clock.
- Plain constructor injection, no container library.
- New code that keeps time, reads a file, calls a service or runs its own
  thread takes these in from the start, in the interface too. Later is a
  refactor nobody does, and the test gets a sleep instead.
- **Nothing handed in means the harmless thing.** No key kept, no network
  reached, nothing written to a real disk. Only the entry point hands in
  the real one, so a test that forgets never writes a developer's
  keychain.
- **A seam is a field of the thing that uses it,** never a global a test
  swaps, which races with tests needing the real thing.
- **Hooks only tests read are a cost** in every build. Keep them few and
  listed, and never ask in shipped code whether it runs under a test.
- **Every background task has an owner that hears it end,** and shutdown
  waits for it. Fire-and-forget work leaves a test only a poll and hides
  the bugs of its end: a cleanup, once given an end signal, showed it
  quit in the middle of a delete.
- **Code that restarts the program runs the test binary again.** Hand the
  restarter in, and let the test's entry point leave at once when started
  as a helper. A real restart in a test once spawned copies of the suite
  until the machine stood still.

## Test up to the boundary you own

The product owns what it hands to a system it does not own, a payment
provider, a driver, a keychain, a cloud service: the arguments, the
request, the order of calls. What the system does with them depends on
the test machine, rarely one people use. In ports and adapters,
Alistair Cockburn's hexagonal architecture, the real system is the
adapter behind a port, and a test stands in for it.

- **The product does not reach for the system itself.** `main` hands in
  the real one, a test none, so the plain path or a stand-in runs. Code
  that picks the system by the operating system it finds reaches it from
  every test too.
- **What is handed over is the test.** A stand-in records the arguments,
  checked one by one against what the real system took on a real
  machine.
- **The line is what differs between machines.** A library the project
  builds and runs the same everywhere may run in a test, since its output
  is the product's behaviour. Hardware, drivers and system frameworks sit
  behind the port.
- **When a real machine breaks the contract,** its input goes into the
  test of what is handed over, and the adapter is fixed. Running the real
  system in CI is no answer, since CI's machine is not the one that broke.
- A test that fails on one machine and passes on another is first
  checked for reaching past the boundary, then the product, section 6 of
  the skill.

## Shapes that test well

- **Concerns apart, behind small interfaces.** Each part is tested alone,
  and a change runs only its tests, section 8 of the skill.
- **One source for each piece of state.** State in two places is a bug
  waiting for an order of events no test finds.
- **One mechanism, used everywhere, not a copy.** Copies drift, like a
  countdown of 8 s in one place and 10 s elsewhere. One template for
  the preview and the sent email makes a mismatch impossible, better than a test
  comparing two. A test that fails on a second copy keeps it so.
- **One path for every input,** so one set of tests covers all. A special
  path needs its own tests and drifts.
- **A fix makes the design simpler, or it is the wrong fix,** not an
  exception around it. A generic, sound fix makes the next case right by
  design.
- **Known problems have known answers.** For an animation curve, a cache,
  an update flow, a retry, copy how established software does it. The
  spec comes from that, and the test checks it.
- **Numbers from first principles.** Ask whether a cost makes sense, not
  only whether the arithmetic adds up: why does one small image take
  10 MB? Design from what the product already knows.
- **One contract for the channel between two halves.** Where two sides
  talk by named messages, names and fields come from one definition, and
  a fast test holds both to it: every message sent is heard and every one
  heard is sent, same fields, matched by full name. The harness runs the
  product's own wiring: a misspelled event once passed every test because
  the harness had its own copy of the names.
- **A format that crosses a boundary has one golden file,** committed,
  written by the writer's test and read by the reader's, expectations
  typed by hand. Sides that build their own bytes drift apart unseen.
- **A list of fields written by hand has a guard.** A cache key, a copy,
  a serializer or an equality that names a type's fields gets a test that
  fails on a field neither handled nor listed as skipped.
- **The producer says what it produced.** Never tell which files a run
  wrote by their times. File times are coarse, and that lost a result in
  seven runs of twenty.
- **A timeout says nothing about the input.** Report a wait that ran out,
  never an unreadable file. A slow machine once marked a valid upload
  broken that way.
- **The failure is counted where a test can read it:** the restart, the
  drop, the double send itself. A restart counted as an ordinary start
  hid a stutter on every edit.
