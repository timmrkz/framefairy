# Design for testing

If the design of a solution is broken, tests help very little. Code that
is hard to test is usually code that is shaped wrong, and the fix is the
shape. Section 5 of the testing skill sums this up.

## Hand things in

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
| hardware or a system service outside the code, a hardware codec, a driver, a keychain | a value the program hands in, absent by default, see below |
| the end of work | a channel, callback or event the caller gives |

- The program hands in the real thing. A test hands in one it drives.
- What a test hands in lives with the tests and is handed in by the
  tests' own setup. The product keeps only the seam and the real thing
  as its default, never the test's double or a flag, setting or global
  that picks between the two. A switch can be set wrong in a build, and
  then a customer runs the test's clock.
- Plain constructor injection, the simplest form. No container library.
- A new piece of code that keeps time, reads a file, calls a service or
  does work on its own thread takes these from outside from the start.
  Adding it later is a refactor nobody gets round to, and the test that
  needed it got a sleep instead.
- The interface the same: a module that keeps time takes its timer, or
  is tested under fake timers.
- **Nothing handed in means the harmless thing.** When nothing is handed
  in, a key store keeps no key, a network reaches nothing, a file system
  writes nowhere real. Only the program's own entry point hands in the
  real one, so a test that forgets to hand one in never writes into a
  developer's keychain.
- **A seam is a field of the thing that uses it,** never a package
  variable or a global a test swaps. A global races with the tests that
  need the real thing and forces them to run one at a time.
- **Hooks in the product that only tests read are a cost** carried into
  every build: markers, counters, globals. Keep them few and listed, and
  never ask in shipped code whether it runs under a test.
- **Every background task has an owner that hears it end,** and shutdown
  waits for it. Work fired and forgotten leaves a test nothing but a
  poll, and hides the bugs of its end: a cleanup on a goroutine of its
  own was polled for twenty seconds, and once it had an end signal, a
  real bug showed, quitting in the middle of a delete.
- **Code that restarts or starts the program itself runs the test binary
  again.** Hand the restarter in, and let the test's entry point leave at
  once when it is started as a helper. A test that reached a real
  restart once started copies of the suite until the machine stood
  still.

## Test up to the boundary you own

A product hands work to systems it does not own: the operating system's
media engine, a graphics driver, a keychain, a service in the cloud. What
the product owns is what it hands over, the arguments, the request, the
order of the calls. What the system then does with them is the system's
code, on the machine the test happens to run on, which is rarely a
machine people use. A test that runs it tests the system: a virtual
machine's media engine that hangs says nothing about the product. This
is ports and adapters, Alistair Cockburn's hexagonal architecture: the
product meets the outside at a port, the real system is the adapter
behind it, and a test stands in for the adapter.

- **The product does not reach for the system by itself.** It is handed
  it, the way it is handed the network: the program's own `main` hands
  the real one, and what a test makes gets none, so it runs the plain
  path in the product's own code or a stand-in. Code that picks the
  system by the operating system it finds itself on reaches it from
  every test on that system too.
- **What is handed over is the test.** A stand-in records the arguments
  or the request, and the test checks them against what the real system
  took on a real machine, argument by argument. It runs on every machine
  in milliseconds, and a change to what the system is asked shows there
  first.
- **The line is what differs between machines.** A library the project
  builds itself and runs the same everywhere may run in a test, because
  what it makes of the product's input is the product's behaviour. What
  changes from one machine to the next, hardware, a driver, a system
  framework, is behind the port.
- **When a real machine breaks the contract,** the input that broke it
  goes into the test of what is handed over, and the adapter is fixed
  for it. A run of the real system in CI is not the answer, because CI's
  machine is not the one that broke.
- Something that fails on one machine and passes on another is first a
  question of whether the test reaches past the boundary, before it is a
  question of the product, section 6 of the skill.

## Shapes that test well

- **Concerns apart, behind small interfaces.** A part with a clear
  interface is tested alone, and a change to it runs only its own tests,
  section 8 of the skill. Two jobs mixed in one place are hard to test and hard to
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
- **One contract for the channel between two halves.** Where a backend
  and an interface, or two processes, talk by named messages, the names
  and fields come from one definition, and a fast test holds both sides
  to it: every message sent is heard, every one heard is sent, with the
  same fields, found by their full names. A misspelled event once passed
  every test because the test harness wired the halves with its own
  copy of the names. The harness runs the product's own wiring.
- **A format that crosses a boundary has one golden file.** Bytes one side
  writes and the other reads are held together by a committed file the
  writer's test writes and the reader's test reads, with expectations
  typed by hand. Tests on each side that build their own bytes stay green
  while the sides drift apart.
- **A list of fields written by hand has a guard.** A cache key, a copy,
  a serializer or an equality that names a type's fields one by one gets
  a test that reads the type's fields and fails on one that is neither
  handled nor on a list of those skipped on purpose.
- **The producer says what it produced.** Never decide which files a run
  wrote by comparing their times with the clock. File times are coarse,
  and one such comparison lost a result in seven runs of twenty.
- **A timeout says nothing about the input.** Code that gives up waiting
  reports a wait that ran out, never that the file cannot be read. On a
  slow machine a valid video was once marked as broken that way.
- **The failure is counted where a test can read it.** Counters a test
  reads count the bad event itself, the restart, the drop, the double
  send. A restart counted as an ordinary start hid a stutter on every
  edit.
