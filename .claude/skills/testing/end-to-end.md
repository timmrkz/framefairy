# End-to-end and interface tests

Section 11 of the testing skill in detail.

- **An end-to-end test does what the person did and checks what they
  saw,** against the real backend, across parts of the product as a
  person goes. A component test is no replacement. An interface fix comes
  with one that fails on the main branch and passes with the fix.
- **A test only proves what its fixture contains.** Make fixtures like
  real inputs, odd encodings, sizes and shapes included, and know what
  one hides: one page of results hides what only paging breaks, one row
  what only two rows break.
- **Each shape of a thing,** simple and with every option. A test meant
  to cross a boundary fails unless it did.
- **Test what ships,** with its own programs and libraries, set up like a
  customer's.
- **The engine the product really runs on.** Run the steps in the
  product's own browser engine or platform before saying it works there.
  Typing, focus, hover, painting and timing differ.
- **Only what the window can show:** a key or click does what the person
  meant, and the screen shows what the core sent. Rules, formats and long
  lists of values go in unit tests of the core. Time and timers run
  on a clock the test moves, so a minute-long countdown runs as fast as
  the page can draw it.
- **Add a step, not a test,** since each test starts the product afresh.
  A new test says why no existing one could take its steps.
- **Read everything a rule judges at one moment:** the button in the
  click's frame, the list and the backend's state in one evaluation. To
  line up with the backend, ask it for a marker behind every event sent
  and go on once the interface hears it. A random test failing one run in
  three had four such causes and no bug.
- **The barrier every step passes through is built from signals:** no
  request in flight, no product timer due soon, nothing moving, each
  heard as an event, never sampled, ignoring logging and telemetry.
  Work the product delays on purpose announces itself. Sampling "quiet
  for 250 ms" cost a sixth of a suite and missed delayed saves.
- **Motion is judged by the frames drawn,** "on every frame while it
  lasted", never by a count of frames or milliseconds a busy machine
  cannot give. Better, hand the component its frames and step them.
- **What hides a wait is tested with the wait held.** For a read ahead,
  an answer shown early, an order of answers: hold the dependency's
  answer, act, then release it. Settling after every step proves only an
  instant answer.
- **Set aside only what the product marks as spoiled,** like a frame
  dropped on a busy machine. The product counts where, the test sets aside
  only that, and separate counters keep the product's own mistakes at
  zero.
- **Random exploration finds what scripts miss,** often within tens of
  seeded steps, rules checked after each. The seed comes from the commit,
  a failure prints its seed and steps, failed seeds play on every run,
  and a failure at a new seed is a bug, not a flake.
- **Reduced motion by default,** with motion on for the steps that check
  it, named in a list that fails on an unknown name, and everything with
  motion on at night.
- **A table of named steps refuses a name twice,** or the second silently
  never runs.
