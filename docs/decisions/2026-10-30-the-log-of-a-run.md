# The log of a run gets a package of its own

Step 2 of splitting the engine into packages, after the numbers and text
of [the first step](2026-10-30-numbers-and-text-in-packages.md). Decided
on 30 October 2026, in the pull request that makes it.

## The problem

Tim said go on step 2 on 30 October. Framing, the first area meant to
stand alone, reports its progress through the engine's log, so it cannot
leave the engine while the log lives there.

The log is two files, measured on main at 7b71cbf with the type checker:

| What | Where | Uses outside the two files |
| --- | --- | ---: |
| `Log`, `NewLog` and its methods: lines, progress, steps, the hold | `log.go` | 372 in 73 files with the rest below |
| `Event`, `EventKind` and its eleven kinds, `Sink`, `JSONLines`, `Unknown` | `events.go` | |
| `Underway`, a clip on the way, carried by an event | `events.go` | |
| `commas`, a whole number with thousands separators | `log.go` | 37 in 6 files |
| `sprintf`, formatting only when there are arguments | `log.go` | 2, in `ffmpeg.go` |
| `Version`, and the package comment of the engine | `log.go` | |

The two files build alone with nothing of the engine but `engine/num`.
In the 153 commits to `engine/` since the app was brought over,
`log.go` changed 8 times and `events.go` 6. Every change to `events.go`
came with a feature that reports progress, the clip cards, clips made by
hand, the word model, and touched the job code and the app's jobs with
it. Two front ends read the log: the command line writes it to the
terminal, and the app hears its events through the `Sink` and writes
them to its own log with zerolog.

## Five simulations

### 1. Who may know about whom

The log knows nothing of the engine above it already, so it can go to
the bottom, beside `num`, `runes` and `timed`, under every area. It
needs `num` for rounding what it sends, and it keeps the thousands and
the formatting it uses. Those two are numbers and text, so they belong
in `num` and `runes`, where `api.go`, `compare.go` and `ffmpeg.go` find
them too. `Version` and the engine's package comment are about the
engine, not its log, so they stay in it.

### 2. What changes together

The terminal side of the log, the colours, the stamps, the line, has
barely changed. What changes is the shape of an event: a new field for
a new kind of progress, together with the job that sends it. That
argues for splitting the event from the log, the event with the jobs.
But `Log.Underway` and `Log.report` build events, so the log would need
the jobs, and the jobs need the log: a cycle Go refuses. An event's
shape and its sender stay in one package, and a feature that adds a
field touches that package and its job, as it touched two files of one
package before.

### 3. Where state lives

A `Log` holds the state of one run's output: whether a progress line is
open, who holds it, the stack of steps, the sink. The engine keeps one
for each `Engine` and the app makes one for each job. Every field is
written only by the log's own methods. Moving it changes nothing about
where that state lives, and a package of its own makes it impossible for
the engine to reach into it, which one test did, reading `held` instead
of asking the log.

### 4. What Go and its libraries already have

The app logs with zerolog, by Tim's rule, and Go has `log/slog`. A
`slog.Handler` could take the lines, but not the progress line one piece
of work holds, the steps with their durations, or the clips on the way:
the log of a run is a progress report, not only a logger. Rewriting it
on slog would change what the terminal and the app show, which is no
part of moving it. The name `log` is taken twice over: by the standard
library, and by over seventy locals and parameters in the engine called
`log`, behind which a package of that name would hide. So the package is
named for what it is, the log of a run.

### 5. What a test has to build

Framing, step 6, reports progress through the log, so the log must sit
under framing by then. Four tests are about the log alone: the events
follow the lines, progress is clamped, the JSON lines, and a held line
is let go only by who holds it. They move with it and build nothing of
the engine. `events_test.go` keeps its other tests, which are about
edits, and the small recorder of events they share.

## What converges

- The log goes to the bottom of the engine, under every area: 1, 3 and
  5.
- The event stays with the log that sends it: 2, by the cycle, and 3.
- `commas` and `sprintf` go to `num` and `runes`, since they are numbers
  and text and other areas use them: 1.
- `Version` and the package comment stay in the engine: 1.
- Not rewritten on slog or zerolog, and not named `log`: 4.

No simulation argued for leaving the log in the engine.

## The decision

One package at the bottom of the engine, `engine/runlog`, the log of a
run: `Log` and `New`, `Event`, `EventKind` and its kinds, `Sink`,
`JSONLines`, `Unknown` and `Underway`. `commas` becomes
`num.Thousands`, `sprintf` becomes `runes.Format`, and `Version` with
the engine's package comment moves to `engine/doc.go`.

The layers, each using only the ones under it:

```
engine          every area, still one package
engine/runlog   the log of a run
engine/num      numbers
engine/runes    text, by its characters
                engine/timed beside them, using none
```

## What follows

- It costs one pull request of renames, about 410 uses in about 75
  files, done by the type checker.
- It buys framing's second obstacle cleared, and the log's state out of
  the engine's reach.
- It was wrong if a feature has to change `runlog` for a rule of its
  own rather than a field of an event, or if `Underway` grows rules
  about clips that belong to the jobs.
