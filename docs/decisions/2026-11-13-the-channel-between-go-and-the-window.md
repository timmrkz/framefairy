# The channel between Go and the window, named once and tested alone

## The problem

Tim, 13 November: the walks should only check what a person does on
screen and what they see. "Stuff that relates to the communication
channel between the backend and the front end should maybe be verified
in isolation there, so that we can ensure the interfaces between all of
those components, backend, front end, and the communication channels
between those should be tested in isolation as much as possible." And
then: "Start an agent for verifying the channel between the backend and
the front end. Something has to happen there."

Read on main at 7134f6e:

- The Go side told the window by itself in 19 places, `app.Event.Emit`
  with the event's name written out each time, in `main.go`, `menu.go`,
  `licence.go` and `chrome.go`. `lib/api.ts` listens for each in 18
  helpers, the name written out again.
- The walks' bridge did not run that wiring. `driver_test.go` wrote it
  again, with its own names, for 9 of the events, and `bridge_test.go`
  sent 3 more itself. A name wrong in `main.go` passed every test and
  every walk and broke the app. `report-about`, `quit`, `show-updates`
  and `licence-link` were sent by no test at all.
- `bindings_test.go` held each call in `api.ts` to a method and its
  number of arguments, and the job states to `JobState`. No test
  compared a field. `api.ts` declared `Notice` twice, once for the
  notices of the licences and once for a failure told as a toast, and
  TypeScript merges two interfaces of one name into one, so each read as
  having the fields of both.
- The bridge found a method by the part of its name after the last dot,
  so a wrong `main.FrameFairy` in `api.ts` passed every walk and failed
  in the app.

## Five simulations

### 1. Who may know about whom

The parts that tell the window, the queue, the video's news, the report,
the recording, the models, know only a function of their own and nothing
of Wails. What they tell goes through one small interface, `emitter`,
which is all a part needs to know of the window: Wails' `app.Event` has
it, and the bridge's bus can have it. The names live in one file beside
it. Crossing the new boundary: `Emit(name string, data ...any) bool`,
one method. The next event added touches that file and `api.ts`. Cost:
one file, two changes of signature, `appMenu` and `watchChrome`.

### 2. What changes together

A name, the data it carries and the type `api.ts` reads it as change
together, across two languages, so they are written once and the other
side generated: Wails' own binding generator writes the TypeScript from
the Go types and from events registered with `RegisterEvent`. This was
#611, closed as not planned on 1 November: no field had drifted, and the
generator brings a build step, files written by a program into the
repository and a check that they are fresh. Every helper in `api.ts`
would be rewritten around the generated names. Cost: many files, a new
step in `make`, for a drift seen once, `Notice`.

### 3. Where state lives

The names are kept where Wails keeps them: `RegisterEvent[T](name)` at
start, so Wails refuses at run time an event sent with the wrong type.
The check then happens in the app, on Tim's Mac, where a refused event
is an event the window never gets and a line in the log, not in a test
before the merge. And the registry is a package variable of Wails',
filled in `init`. Cost: small, but the failure lands in the wrong place.

### 4. What runs together

The bridge runs `main` itself, so the walks run exactly the app's wiring,
the menu and the windows included. `main` would be split around a
`*application.App` the bridge could make. Wails' app is a concrete type
that starts the platform when it is made, on Linux GTK, which needs a
display, and its bindings refuse to be made without it. Cost: large,
and every walk would start more of the app than it needs.

### 5. What a test has to build

The two sides are held together by tests that build as little as
possible. Go tests read `api.ts` as text, the way `bindings_test.go`
already does, and compare it with the names in Go and with the Go types
by reflection: the names both ways, the type each listener reads the
data as, and every field of every type that crosses, both ways. The
agent of the research suggested example messages written by Go into
files the TypeScript tests read, the `frame.cases.json` shape, which is
a file both sides must keep agreeing on and was refused for #665. Cost:
two test files and no change to the app.

## What converges

- One place names every event, with the Go type it carries. 1, 2, 3 and
  5 have it, in Go code, in generated code, in Wails' registry or in a
  table a test reads.
- The parts tell the window through something handed in, not through
  Wails directly. 1 and 4 have it, and it is what lets the bridge run the
  app's wiring rather than a copy of it.
- What crosses is checked by a test, before the merge, not in the app. 2
  and 5 have it, and 3 is refused for doing it in the app.
- Only 2 needs a generator, and only 4 needs Wails' app in a test. Both
  are dropped for what they cost.

## The decision

The window is told through `emitter`, handed to `tellWindow` in
`cmd/framefairy-app/events.go`, which wires every part that tells the
window by itself. `main` hands it Wails' events, the bridge its bus.
Each event is a constant in that file whose type says what it carries,
`event[JobUpdate]`, so a part cannot send the wrong data on a name, and
no name is written anywhere else on the Go side. Go tests read `api.ts`
and hold to it the names, both ways, the type each listener reads, and
the fields of every type an event or a call carries, both ways, with
whether `api.ts` may find them missing. `api.ts` declares one interface
to a name, so the licences' notices are `LicenceNotice`, and a word the
workspace lights, which Go never sends, is `LitWord`.

```
lib/api.ts                      the window: Events.On, call<T>
   ^           ^
Wails' events  the bridge's bus emitter, Emit(name, data)
   ^           ^
cmd/framefairy-app/events.go    the names, their types, tellWindow
   ^
the parts                       the queue, the news, the report, the
                                recording, the models, the menu
```

## What follows

- A name spelled differently on the two sides, a listener with no
  sender, an event no part sends, a field renamed or left out on one
  side, fail `make unit` on Linux in a few seconds. They no longer need a
  walk to be found.
- The walks run the app's own wiring, so the walk that reaches an event
  reaches the code the app runs.
- `api.ts` keeps writing its types by hand. The test is what keeps them
  true. If the test grows rules of its own for every new kind of type,
  that is the sign 2 should be taken up again.
- The bridge finds a method by the whole name Wails gives it in the app,
  `main.FrameFairy.<Method>`, and `bindings_test.go` holds `api.ts` to
  the same. The bridge cannot call through Wails' own dispatch, its
  `Bindings`, which log through the app Wails makes and so cannot be
  made without it, see 4.
- The menu, the licence link and the title bar of a window still tell
  the window only in the app. They go through the same constants, and
  the test checks that each is sent somewhere.
