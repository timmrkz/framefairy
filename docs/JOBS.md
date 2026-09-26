# Jobs

Design for making each piece of work in the app one thing in the code,
the way it is one thing for the person using it. It is a plan: nothing
here is built yet. The batches at the end say in what order it is built,
and each one is marked in [GUI-PLAN.md](GUI-PLAN.md) as it lands.

## Why

A person does two things that take time: they ask for clips in a window
of an episode, and they render clips. Each is one piece of work to them,
and each either ends in something they can use or says why not.

Under that there are three steps, each with its own machinery and each
leaving something on disk that stays:

| step | what does it | what it leaves |
| --- | --- | --- |
| **hearing** | the speech model | the transcript, saved as it is heard |
| **finding** | the language model, and the engine framing each clip it names | the clips in the plan, as they land |
| **rendering** | the engine and ffmpeg | the finished shorts, one by one |

A search is hearing and then finding. A render is rendering. Each step can
fail, and each can be cut off by the app closing or the machine going
off, and what it left stays and is carried on from.

The code does not see it that way. The app runs a search as two jobs in
two lanes, a transcription and a Find clips job. The Find clips job waits
for the transcription in the other lane, reads the transcript file once
a second, looks at the transcription job every 20 ms, pauses it when it
has heard the window, and starts it again when it stopped. Renders share
a lane with Find clips, so a render waits for a search to end, and a
search for a render. A render lives only in memory: the app closed during
one and it is gone, with nothing said. Before a search job even exists,
the interface holds the search in memory while the transcript is not
there yet, and decides by itself when to ask for it and where the
transcription is to stop.

So one search has its state in three places that each know a part of it:

| where | what it keeps |
| --- | --- |
| the interface, in memory | a search asked for and not started (`chosen.asked`), where the transcription stops (`chosen.held`), whether the first search has happened (`chosen.looked`), and what it works out from those: `readyToLook`, `stillWaiting`, `autoLook`, `lookPending`, `shouldLook`, `shouldWarm`, `shouldTranscribe` |
| the app's Go side | two jobs in two lanes, the stop point of each transcription, which transcriptions a search paused, and the functions that stitch the two jobs together: `Transcribe`, `HoldTranscription`, `holdOf`, `releaseHold`, `waitForTranscript`, `untilNews`, `pauseTranscriptions`, `carryOn`, `stopped`, `stopTranscription`, `AskSearch`, `ForgetSearch`, `transcribeForFirstSearch` |
| the work folder | `looked`, `search.json`, the transcript, the plans |

Every feature that touches finding clips has to find all three, and the
bugs of the last weeks were each a place that was missed: a search asked
for before the transcript was there left no note, because that search only
existed in the interface's memory, and the app closing looked like Cancel,
because nothing could tell the two apart.

## What a job is

A job is one piece of work a person started, or the app started for them:
a **search** or a **render**. It is owned by the Go side. It has what it
was asked for and a step it is in. An episode has at most one search at a
time, and any number of renders.

A search goes **waiting**, **hearing**, **finding**, **done**. A render
goes **waiting**, **rendering**, **done**. Waiting is its turn behind
other work.

And every job can stop before done in one of two ways:

- **Failed**, with the reason.
- **Called off**, by Cancel. Nothing is said about it, as for any work
  called off by hand.

A job whose record says hearing, finding or rendering when the app starts
was **interrupted**: the app was closed or fell over. It is not a state
anybody writes. It is what a running job looks like to an app that did
not start it. It says so where its work was, with **Continue**.

**There is no Pause.** Cancel and Pause today do nearly the same thing,
under two names and two buttons, and only while transcribing. What made
pausing worth having is that it kept what was heard, and every step now
keeps what it left, however it stops. So Cancel is the one way to stop
work, everywhere, and nothing done is done again:

- A search called off while hearing keeps the transcript it heard. The
  next search goes on from where the transcript ends.
- A search called off while finding keeps the clips that landed.
- A render called off keeps the shorts it finished.

Continue carries a job on from what it left. Hearing carries on from the
end of the transcript. Finding asks the model again for the whole window
and replaces the clips that landed, because an answer cannot be taken up
halfway. Rendering renders the clips that are not finished yet.

If using the app says a pause is missed after all, it is a step that
stops without being called off, and adding it later is small.

## Three lanes

The queue has a lane for each step's machinery: **hearing**, **finding**
and **rendering**. Each runs one job at a time. A search moves from the
hearing lane to the finding lane when it has heard its window, instead of
being two jobs that watch each other. A render is in the rendering lane
and waits for no search.

One rule stays from today, and moves into the queue where it can be seen:
while the finding lane runs, the hearing lane waits, because the speech
model and the language model want the same memory and the same graphics
chip, and each is slower for sharing them. A hearing job that waits for
that keeps what it heard and carries on after.

Model installs share the lane of the step that needs them, the speech
model hearing's and the language model finding's, as today.

## The record

Every job keeps one record in its episode's work folder, in `jobs/`, one
file per job, written on every change of step and every report of how far
it has come, atomically, the way plans are written:

```json
{
  "id": "s-1790449…",
  "kind": "search",
  "from": 1800, "to": 3600,
  "count": 12, "min": 20, "max": 30, "replan": false,
  "step": "hearing",
  "covered": 2712.4,
  "error": "",
  "asked": "2026-09-26T19:05:00Z",
  "steps": {
    "waiting": {"from": "2026-09-26T19:05:00Z", "to": "2026-09-26T19:05:02Z"},
    "hearing": {"from": "2026-09-26T19:05:02Z"}
  }
}
```

A render has the plan and the clips instead of the window and the
numbers, and the clips it has finished.

- `step` is one of `waiting`, `hearing`, `finding`, `rendering`, `failed`.
  A job that is done or called off has no record: the transcript, the
  plans and the shorts say what it made.
- `steps` is when each step began and ended. It is the timings: how long
  a search waited, heard, loaded the model and found, and how long a
  render took. When a job is done, its timings are added to
  `jobs/timings.jsonl` before the record goes, so `framefairy-train` can
  add them up over the library. They stay on the machine. `searchclock.go`
  keeps timing the model's own parts for the progress fill, as today.
- It is read as untrusted, like every file on disk: a record that is not
  one is no record.
- It replaces `looked` and today's `search.json`. Whether an episode has
  ever been searched is whether it has a plan or a record.

The record is the truth. The app never keeps a job anywhere else, and the
interface never keeps one at all.

Sending timings home from customers' machines is a separate decision,
made with the packaging: it has to be asked for, and it is worth nothing
without somewhere to send it.

## Who does what

**The engine** gets `Project.Search`, which runs hearing and finding in
order and writes the record as it goes, and `Project.Render` writes its
record the same way. `Project.Search` is the command line's `Run` split at
the seam it already has, transcribing and then planning. The command line
keeps its flags and gets the same records for free.

**The app** runs jobs in its three lanes. When it starts, it reads the
records of the episodes in its library, and a record in a running step is
reported as interrupted. Adding a video asks for the first search of its
first window. That is what the interface decides today with
`shouldTranscribe` and `shouldLook`, and it moves to the Go side, where
the episode is added.

**The interface** sends what was clicked and shows what it is told. There
is no logic about jobs in TypeScript.

| click | call |
| --- | --- |
| New, or Continue on a search | `Search(path, window, numbers)` |
| Render, or Continue on a render | `Render(path, plan, clips)`, as today |
| Cancel | `Cancel(job)` |

It gets the state of every job in one event, `job`, as today, whenever it
changes, and on opening an episode it asks for them once. Everything it
shows about work comes from that: the row the next clip will appear in,
New, Continue and Cancel, the fill on the range picker, the Render button
of a clip, the note of a job that stopped.

## What goes away

In the interface: `chosen.asked`, `chosen.held`, `chosen.looked`,
`lookPending`, `autoLook`, `stillWaiting` as a decision, `readyToLook` as
a decision, the effects that start a search or hold the transcription,
`shouldLook`, `shouldWarm`, `shouldTranscribe` and their tests, which
become tests of the Go side. The pause mark on the range picker, and the
Pause button of a transcription.

In the app's Go side: `Transcribe` as something the interface calls,
`HoldTranscription`, `holdOf`, `releaseHold`, `waitForTranscript`,
`untilNews`, `pauseTranscriptions`, `carryOn`, `stopped`,
`stopTranscription`, `AskSearch`, `ForgetSearch`,
`transcribeForFirstSearch`, the separate job kinds `transcribe` and
`plan`.

In the engine: `MarkLooked`, `Looked`, `lookedName`, `SearchNote` and its
functions, folded into the record.

## What stays as it is

What a search finds and how: the prompt, the room a window has, the plans
and their files, the clips as they land, the training records. How a clip
is rendered. Editing a clip, the clip timeline, the range picker's
drawing. The command line and its flags. The look of work in hand.

## How it is tested

The tests of today are mostly tests of the parts, and most of the parts
go away. Deleting them and writing new ones after would throw away what
they proved. So the proof comes first and stays:

**Path tests, written against the code of today, before anything
changes.** Each one is what a person does, told through a small driver
with the words of the app: add a video, press New, press Cancel, press
Render, close the app, open it again, press Continue. The driver is the
one place that knows which calls do that. The tests run green on today's
code and are committed that way. The refactor then changes the driver and
nothing in the tests, and they have to stay green through every batch
after. What they prove today they still prove at the end.

The paths, with the fake speech model and the fake language model:

- A video added, and the first search going by itself through every step
  to clips in the list.
- New on an episode not heard yet, and the search going through every
  step to done.
- The app closed in each step, which in a test is a new queue reading the
  same work folders, and the search reported as interrupted.
- Continue after that, carrying on from where it was, with nothing heard
  twice.
- Cancel in each step, nothing said, and what was heard kept.
- A failure in each step, with its reason.
- A render to the end, and one that fails.
- Two episodes at once, and a render while a search runs, under the race
  detector.

What today's code does not do at all, like a render that says it was
interrupted, gets its path test in the batch that brings it.

The path tests are in `cmd/framefairy-app/paths_test.go`, and the driver
in `driver_test.go`. Written against today's code, they found one bug on
the way: a search whose speech model could not be loaded said "step
failed" instead of why. That is fixed in the same commit.

The interface's harness gets fake jobs that go through the same steps
from the same records, so what a probe sees is what the Go side reports
rather than a state made up for the probe.

## Batches

One pull request, a commit per batch, merged only once it has been tested
on Tim's machine and works.

1. This design.
2. The path tests and their driver, green on today's code.
3. The records, their timings, `Project.Search` and the render's record
   in the engine, with their tests. Nothing in the app uses them yet.
4. The app runs jobs through them in three lanes, reads the records at
   start, asks for the first search when a video is added, and takes the
   calls above. The driver switches over, and the path tests stay green.
   The old calls stay for the moment, so the interface still works.
5. The interface switched over, Pause gone, and the harness to fake jobs.
6. What goes away, goes, and the docs say how it is now.
