# One clock on the machine

13 November 2026, for #813.

## The problem

Tim, 12 November 2026: "if there is still something similar like linear
time being spent in the walks, or in any test for that matter, that
should be refactored so that linear time is rather simulated using a
controllable fake clock. And I guess there are other primitives that
work similarly. It's just that time and clock are those evergreen
examples."

Measured on main at 7134f6e:

- Three kinds of work in the Go code keep their own time and a test or a
  walk needs to move it:
  - a search says how far it has come every 500 ms, on a real ticker in
    `engine/searchclock.go`, so a walk waited half a second a report,
  - a removed row can be put back for a while, on `timeKeeper` in
    `cmd/framefairy-app/removal.go`, an interface of the app's own that
    only the queue held, which the bridge filled with `walkTime`,
  - a request to a company is asked again after 2, 4, 8 s or what the
    company says, on `time.After` in `engine/api.go`, which no test
    reached because it took seconds.
- Waits of other shapes were there already: `tool.Runner.Leave`, a
  `func(time.Duration) <-chan time.Time`, and `modelHost.idle`, a
  function a test sets, each for one use.
- 22 lines of `engine/` and `cmd/framefairy-app/` outside the tests call
  `time.AfterFunc` or `time.After`. Most are bounds a busy machine
  reaches, the 10 s a model server has to stop, the 2 s a decoder has to
  end, which no test waits on.
- The walks run a real process, the bridge, so its time cannot be a
  `testing/synctest` bubble. It has to be handed in.

## Five simulations

### 1. Who may know about whom: a package of its own

`engine/clock/`, beside `engine/tool/`, holds `Clock` and the system's.
The engine and the app import it, and the machine holds one.

- Layers: a new small package under the engine, used by the engine and
  the app's Go side.
- What moves: the interface and `SystemClock` out of the engine.
- Names that cross: `clock.Clock`, `clock.System`, 2 names.
- Next changes: another user of the clock imports one more package.
- Cost: a package for two names, and a row of its own here.

### 2. What changes together: each use keeps its own wait

Every use takes a function of its own shape, the way `Runner.Leave` and
`modelHost.idle` do: `retryWait` on the engine, a `tick` on the search,
`timeKeeper` stays in the app.

- Layers: none new.
- What moves: nothing.
- Names that cross: one field per use, three now, more with each use.
- Next changes: every new timer adds a field and a fake of its own. The
  walks' bridge has to fill each one with the same `walkTime`, so holding
  the walks' time means finding every field.
- Cost: least code now, and the drift Tim warned of later, each fake
  slightly different.

### 3. Where state lives: one clock on `engine.Machine`

The clock is what the program keeps for its whole life and shares with
every job, which is what the machine is for. `engine.Clock`, `Now` and
`AfterFunc`, in `engine/clock.go`, and `Machine.Clock`, the system's
from `NewMachine`. The search, the retries and the removal all read it.
The bridge sets it once to `walkTime`.

- Layers: none new. The app already reaches the machine through its store.
- What moves: `timeKeeper` leaves the app, the engine's `time.After`
  and ticker read the machine's clock.
- Names that cross: `engine.Clock`, `engine.SystemClock`,
  `Machine.Clock`, 3 names.
- Next changes: a new timer that a walk must hold reads `Machine.Clock`,
  one line, and the bridge needs nothing new.
- Cost: one file in the engine, one field, three users changed.

### 4. What runs together: the clock on the `Engine` of a job

A job's engine carries the clock, the way it carries `stuck`.

- Layers: none new.
- What moves: the same as 3, but onto `Engine`.
- Names that cross: the same.
- Next changes: a removal is not a job and has no engine, so the app
  keeps a second clock for it, and the walks hand both.
- Cost: two homes for one thing, which the rule One primitive per thing
  forbids.

### 5. What a test has to build: no clock at all, bubbles only

Every test that needs time runs in a `testing/synctest` bubble, whose
clock is the system's to the code under test, and nothing is handed in.

- Layers: none.
- What moves: nothing in the code, the tests change.
- Names that cross: none.
- Next changes: none for unit tests.
- Cost: the walks cannot use it. The bridge is a process that serves a
  browser, which a bubble does not hold, so the search would report on
  real half seconds there again.

## What converges

One interface, `Now` and `AfterFunc`, comes back in 1, 3, 4 and 5 (5
gives it to the tests through the bubble). One home shared by every job
comes back in 1 and 3, and 4 shows why a job's engine is not it. The
type lives with the engine, which uses it and is under the app, so 1's
package of its own buys nothing for two names. Bubbles from 5 stay for
the unit tests, which keep using them: a bubble's clock is the system's
clock the machine holds by default.

Kept apart: `updates.Source.Wait` and `tool.Runner.Leave` sit beside or
under the engine and have no machine, so they keep the wait function of
`Runner.Leave`. The rule: code that has a machine keeps time on
`Machine.Clock`, a package that has none takes a
`func(time.Duration) <-chan time.Time`.

## The decision

Candidate 3.

```
app's Go side   removal.go reads Machine.Clock      bridge sets Machine.Clock = walkTime
      |
engine          Machine.Clock  <- searchClock, waitToRetry
      |
engine/clock.go Clock (Now, AfterFunc), SystemClock
```

## What follows

- It costs a field on the machine and one default, set by `NewMachine`.
- It buys one clock a walk holds for all of the Go side's own timers it
  waits on, and tests of the retries that spend no time.
- The bounds no test waits on, the 10 s a model server has to stop and
  the like, stay on the system's clock until a test needs to move them,
  and then read `Machine.Clock`.
- It was wrong if a walk needs two clocks at once, one held and one
  running, which no walk does today.
