# The app's log

How the app keeps a record of what it did, so that a fault on a customer's
Mac is found by reading what happened, not by asking them to try again.
Plan rows 2.185, where the first log came in, 2.188, which is this design,
and R.8, the report a customer sends.

## What a customer does

Nothing beforehand. A customer who runs into a problem picks **Help →
Report a Problem…**, and the app writes one file with everything we need.
They send it to us. The steps are below, and they do not ask the customer
to switch anything on first or to do the same thing again.

So the detail has to be there already when the fault happens. A fault
like start.mp4's, a picture that never came, raised no error at all. The
only thing that found it was the record of each step a frame takes. A log
that writes errors only, with the steps behind a switch, would have held
nothing. The customer would have had to turn the switch on, make it
happen again and send the log, which is the round trip this log is there
to save.

So every line, the steps included, is always written, and what keeps the
disk free is a cap, not a switch. The log is at most 10 MB of lines in
use and three older files compressed beside it, a few MB more. That
holds many hours of work. It is never more, whatever happens.

The report is one zip file on the Desktop, shown in Finder:

- the log and its older files
- the build, the macOS version, the chip and the memory
- the settings, with every key removed
- the jobs folder of the video in front, which holds what a search sent
  to the model and what it got back

It never holds the video, its transcript or its captions. Paths under the
home folder are written as `~`, so the Mac's user name does not travel.
The customer sees what is in it before sending: Finder shows the zip, and
the zip holds plain text.

Where reports are sent is still open, see R.8. Until there is an address,
the customer attaches the zip to an email.

## The library

[zerolog](https://github.com/rs/zerolog) writes the lines, and
[lumberjack](https://github.com/natefinch/lumberjack) keeps the file to
its size. Both are MIT licensed and widely used. Nothing of our own does
either job. The hand-written rotation of 2.185 goes.

| | zerolog | zap | log/slog |
| --- | --- | --- | --- |
| Speed and allocations | fastest, none per line | close behind, few | slower, some per line |
| Fields on every line of a part | `With()` | `With()` | `With()` |
| Levels | trace to panic | debug to fatal | debug to error |
| What it adds to the build | itself, its two small modules are in already | itself and Uber's multierr | nothing, it is Go's |
| Takes lines from code that speaks slog | `zerolog.NewSlogHandler` | `zapslog` | is it |

zerolog it is. All three would carry this log, which is a few lines a
second at most. What tips it to zerolog: it allocates nothing per line,
so the steps of a play cost nothing measurable while the play runs. It
brings only itself into the build, since the two modules it needs are in
it already, and it has a trace level below debug for lines that come
with every frame. Since 1.35 it also takes slog's lines. Wails, the
library that runs the app's window, logs through slog, and its lines go
into the same file that way.

## What a line holds

Each line is one JSON object, so it can be filtered by any field rather
than read with the eye:

```json
{"level":"debug","time":"2026-10-10T22:41:07.512+02:00","run":"9f3a01","from":"window","video":"start.mp4","act":"a41","msg":"video preview: the first frame and sound are in, waiting for the sound card"}
```

| Field | What it is |
| --- | --- |
| `time` | the moment, to the millisecond, with the Mac's offset from UTC |
| `level` | `error`, `warn`, `info`, `debug` or `trace` |
| `run` | one start of the app, so a restart is seen and two runs never mix |
| `from` | the part that wrote it: `app`, `window`, `files`, `decoder`, `engine`, `wails` |
| `video` | the video it is about, by file name, where there is one |
| `act` | what the person did that led to it, see below |
| `msg` | what happened, in words |

Other fields come where they mean something: `stream` for the frames,
`job` for a search or a render, `ms` for how long something took,
`status`, `bytes` and `said` for a call, `build` when the app starts.

### Acts

What the person did is the thread a fault is followed by. Each act is a
line at `info` with a short id of its own: a video picked, added or
removed, play, pause, a jump of the playhead, a search started, a cut, a
render. Everything that act sets off carries its id, in the window, on
the Go side and in the decoder. The window sends it with every call and
every request for frames, and the Go side hands it on.

So the report of a picture that never came reads as: the person picked
start.mp4, act a41. Then every line with `act=a41`: the workspace
opening, the stream from 46.064 s, the decoder's first frame at
46.124 s, and the queue still waiting for 46.084 s three seconds later.
Before acts, the lines of start.mp4 had to be matched up by time and by
guesswork.

### Levels

| Level | What it is for |
| --- | --- |
| `error` | something failed: a decoder that stopped, ffmpeg's errors, a call that failed, an exception in the window |
| `warn` | something went wrong and the app carried on: the window stood still, a play that has not started after 3 s, a slow call |
| `info` | the acts, the app starting, a video opening, a job starting and ending |
| `debug` | the steps between: a stream opened, a first frame, the sound card's state, a decoder held and let go |
| `trace` | lines that come with every frame or every pull |

Everything from `debug` up is always written, see above. `trace` is
written only while **Help → Detailed Log** is ticked, for chasing
something like a play that stutters. It stays as it was left when the
app starts again, and the report says whether it was on.

## Reading a report

The log is JSON lines, read with [jq](https://jqlang.org). For example:

```sh
# every error and warning
jq -c 'select(.level == "error" or .level == "warn")' app.log
# everything one act set off
jq -c 'select(.act == "a41")' app.log
# the last run's lines about one video
jq -c 'select(.video == "start.mp4")' app.log
```

## The steps

1. Done. zerolog and lumberjack in place of the hand-written log: JSON
   lines, the levels above, `run`, `from` and `video`, and Wails's own
   lines, from info up, in the same file. Its debug lines are one for
   every request the window makes, which `files` has lines for already.
2. Acts: an id for each, sent with every call and request for frames,
   and carried by the Go side and the decoder.
3. Help → Report a Problem…, the zip above. It is R.8 without the
   address.
