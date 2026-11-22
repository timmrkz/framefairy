# End-to-end and interface tests

Section 11 of the testing skill in detail.

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
- **Only what the window can show.** An interface test checks two
  things: that a key or a click does what the person meant, and that
  the screen shows what the core sent. Rules the core decides, file
  formats and long lists of values go in unit tests of the core, and the
  interface test checks once that the screen shows the result. Time in
  an interface test runs on a clock the test moves, the sound card and
  the timers too, so a one minute video plays as fast as its frames can
  be drawn.
- **Add a step, not a test.** Every interface test starts the product
  from scratch, which costs seconds on every run. A new case goes into
  a test that is already there, and a new one says why no test there
  could take its steps.
- **Read everything a rule judges at one moment.** In a product that
  keeps changing by itself, read the button in the same frame as the
  click, and the list and the backend's state in one evaluation. To line
  up with the backend, ask it for a marker behind every event already
  sent, and go on once the interface has heard the marker. A random test
  that failed one run in three had four such causes and no bug.
- **The barrier every step passes through is built from signals:** no
  request in flight, no product timer due soon, nothing moving, each
  heard as an event, never sampled. Logging and telemetry are not work
  it waits for, and work the product delays on purpose tells it it is
  coming. A barrier that sampled "quiet for 250 ms" cost a sixth of a
  suite and returned before delayed saves had started.
- **Motion is judged by the frames that were drawn,** "on every frame
  drawn while it lasted", never by a count of frames or milliseconds a
  busy machine cannot give. Better still, hand the component its frames
  and step them.
- **What hides a wait is tested with the wait held.** A read ahead, an
  answer shown before it arrives, an order of answers: hold the
  dependency's answer on purpose, act while it is held, then let it go.
  A test that settles after every step proves only the case where the
  answer comes at once.
- **Set aside only what the product marks as spoiled.** Where the
  environment spoils part of a measurement, sound fed late on a busy
  machine for example, the product counts exactly where, the test sets
  aside only those parts, and separate counters prove the product's own
  mistakes stay at zero.
- **Random exploration finds what scripts miss.** Seeded random steps, with
  rules checked after every step, found real bugs within tens of steps.
  The seed comes from the commit, a failure prints its seed and steps so
  it can be replayed, every seed that failed is played on every run, and
  a failure at a new seed is a bug, not a flake.
- **Ask the platform for reduced motion by default,** with motion on for
  the steps that check it, named in a list that fails on a name that is
  not a step, and run everything with motion on at night.
- **A table of named steps refuses a name twice.** Otherwise the second
  silently never runs.
