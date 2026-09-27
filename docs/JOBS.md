# Jobs

How the app runs work. Each piece of work is one thing in the code, the
way it is one thing for the person using it.

## Why it is built this way

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

The code used to see it another way. The app ran a search as two jobs in
two lanes, a transcription and a Find clips job, and the Find clips job
watched the transcription in the other lane: it read the transcript file
once a second, looked at the transcription job every 20 ms, paused it when
it had heard the window, and started it again when it stopped. Renders
shared a lane with Find clips, so a render waited for a search and a
search for a render. A render lived only in memory, so the app closed
during one lost it with nothing said. And before a search job existed at
all, the interface held the search in memory while the transcript was not
there yet, and decided by itself when to ask for it and where the
transcription was to stop.

One search had its state in three places, each knowing a part of it:

| where | what it kept |
| --- | --- |
| the interface, in memory | a search asked for and not started, where the transcription stops, whether the first search has happened, and seven rules worked out from those |
| the app's Go side | two jobs in two lanes, the stop point of each transcription, which transcriptions a search paused, and thirteen functions that stitched the two jobs together |
| the work folder | `looked`, `search.json`, the transcript, the plans |

Every feature that touched finding clips had to find all three, and the
bugs of those weeks were each a place that was missed: a search asked for
before the transcript was there left no note, because that search only
existed in the interface's memory, and the app closing looked like
Cancel, because nothing could tell the two apart.

## What a job is

A job is one piece of work a person started, or the app started for them:
a **search** or a **render**. It is owned by the Go side. It has what it
was asked for and a step it is in. An episode has at most one search at a
time, and any number of renders.

A search goes **waiting**, **hearing**, **finding**, **done**. A render
goes **waiting**, **rendering**, **done**. Waiting is its turn behind
other work. A search whose window has been heard already goes straight
from waiting to finding.

Every job can stop before done in one of two ways:

- **Failed**, with the reason.
- **Stopped**, by Cancel. A search says so where its work was, with
  **Continue**, the same way as one the app was closed on: what it heard
  stays and it can be carried on, and an empty column after Cancel read as
  work that had vanished. A render called off goes, and its Render button
  carries it on.

A job whose record says it was running when the app starts was
**interrupted**: the app was closed or fell over. Nobody writes that
state. It is what a running job looks like to an app that did not start
it. It says so where its work was, with **Continue**, and so does a
stopped search, after a restart too. The sidebar gives both the colour of
a warning, and a failed search the colour of an error.

**There is no Pause.** Pause and Cancel did nearly the same thing under
two names, and Pause only while transcribing. What made pausing worth
having was that it kept what was heard, and every step now keeps what it
left, however it stops. So Cancel is the one way to stop work, and nothing
done is done again:

- A search called off while hearing keeps the transcript it heard. The
  next search goes on from where the transcript ends.
- A search called off while finding keeps the clips that landed.
- A render called off keeps the shorts it finished.

Continue carries a job on from what it left. Hearing carries on from the
end of the transcript. Finding asks the model again for the whole window
and replaces the clips that landed, because an answer cannot be taken up
halfway. Rendering renders the clips that are not finished yet.

If using the app shows a pause is missed after all, it is a step that
stops without being called off, and adding it is small.

## Three lanes

The queue has a lane for each step's machinery: **hearing**, **finding**
and **rendering**. Each runs one job at a time, and `lanes.go` hands out
the turns in the order they were asked for. A search asks for a turn in
the lane of each step as it comes to it, and lets the last one go. A
render is in the lane of rendering and waits for no search.

One rule is kept in `lanes.go`, where it can be seen: while a search finds,
no search hears, because the speech model and the language model want the
same memory and the same graphics chip, and each is slower for sharing
them. A search that comes to finding takes the lane of hearing back from a
search that hears, which saves what it heard, waits its turn and carries
on after. Work that is not a step of a search, a model being installed, is
neither taken back nor held up.

Model installs share the lane of the step that needs them: the speech
model the lane of hearing, the language model the lane of finding.

## The record

Every job keeps one record in its episode's work folder, in `jobs/`,
written atomically on every change of step. The search is
`jobs/search.json`, because an episode has one at a time, and a render is
`jobs/render-<id>.json`. The code is `engine/jobs.go`.

```json
{
  "id": "search",
  "kind": "search",
  "from": 1800, "to": 3600,
  "count": 12, "min": 20, "max": 30,
  "step": "hearing",
  "asked": "2026-09-26T19:05:00Z",
  "steps": [
    {"step": "waiting", "from": "2026-09-26T19:05:00Z", "to": "2026-09-26T19:05:02Z"},
    {"step": "hearing", "from": "2026-09-26T19:05:02Z"}
  ]
}
```

A render has the plan and the clips instead of the window and the
numbers, and `done`, the clips it has finished, written as each one is.

- `step` is one of `waiting`, `hearing`, `finding`, `rendering`,
  `stopped` and `failed`, with `error` beside a failed one. A search
  called off says `stopped`, written as it ends. A job that is done, and a
  render called off, has no record: the transcript, the plans and the
  shorts say what it made.
- How far hearing got is not in the record. The transcript says it, and
  saying it twice is two things that can disagree.
- `steps` is when each step began and ended: how long a search waited for
  its turn, heard and found, and how long a render took. When a job is
  done, its timings are added to `jobs/timings.jsonl` before the record
  goes, with the model that found the clips. They stay on the machine.
  `searchclock.go` goes on timing the model's own parts for the progress
  fill.
- It is read as untrusted, like every file on disk: a record that is not
  one is no record, and a render's plan has to be one of the episode's
  own plans.
- Whether an episode has ever been searched is `EverSearched`: it has a
  plan or a `jobs/` folder. That took the place of `looked`.

The record is the truth. The app keeps a job nowhere else, and the
interface keeps none at all.

Sending timings home from customers' machines is a separate decision,
made with the packaging: it has to be asked for, and it is worth nothing
without somewhere to send it.

## Who does what

**The engine** has `Project.Search`, which hears and then finds and writes
the record as it goes, and `Project.RenderJob`, which renders clips one at
a time and writes its record the same way. Before each step they ask the
app for their turn in that step's lane. `Project.Search` takes the same two
steps as the command line's `Run`, transcribing and then planning. The
command line keeps its flags and its own run, and keeps no records.

**The app** runs every job on a goroutine of its own, in the lanes above.
When it starts, it reads the records of the episodes in its library: a
record in a running step is a job in the list in the state `interrupted`,
a failed one a job that `failed`, with its reason. Clearing the finished
jobs leaves them, because they are not finished. Adding a video asks for
its first search, of the first half hour or all of a shorter episode, and
no longer than the model can read at once, see `firstSearch` in
`cmd/framefairy-app/search.go`. Cancel on a search that runs marks its
record stopped as it ends. Cancel on one that stopped, which only Activity
offers, takes its record away. New on an episode waits for a search that
was just called off to be on its way out first, because both write the
same record.

**The interface** sends what was clicked and shows what it is told. There
is no logic about jobs in TypeScript.

| click | call |
| --- | --- |
| New | `Search(path, request)` |
| Render | `Render(path, request)` |
| Continue, on a search or a render that stopped | `Continue(job)` |
| Cancel, on one that runs or one that stopped | `CancelJob(job)` |

It gets the state of every job in one event, `job`, whenever it changes,
and reads the list once as it starts. Everything it shows about work comes
from that: the row the next clip will appear in, New, Continue and Cancel,
the fill on the range picker, the Render button of a clip, the note of a
job that stopped. While a search runs, the range picker shows its window.

## What went away

In the interface: `chosen.asked`, `chosen.held`, `chosen.looked`,
`chosen.warmed`, `lookPending`, `autoLook`, `stillWaiting`, `readyToLook`,
the effects that started a search or held the transcription, and
`shouldLook`, `shouldWarm`, `shouldTranscribe` with their tests, which
became the path tests of the Go side. The Pause button of a transcription.

In the app's Go side: `Transcribe` and `Plan` as calls,
`HoldTranscription`, `holdOf`, `releaseHold`, `waitForTranscript`,
`untilNews`, `pauseTranscriptions`, `carryOn`, `stopped`,
`stopTranscription`, `AskSearch`, `ForgetSearch`,
`transcribeForFirstSearch`, `WarmModel` as a call, the job kinds
`transcribe` and `plan`, and the loop per lane: each job runs on a
goroutine of its own and takes its turns from `lanes.go`.

In the engine: `MarkLooked`, `Looked`, `SearchNote` and its functions,
folded into the record.

## What stayed as it was

What a search finds and how: the prompt, the room a window has, the plans
and their files, the clips as they land, the training records. How a clip
is rendered. Editing a clip, the clip timeline, the range picker's
drawing. The command line and its flags. The look of work in hand.

## How it is tested

The tests before this were mostly tests of the parts, and most of the
parts went away. Deleting them and writing new ones after would have
thrown away what they proved. So the proof came first and stayed:

**Path tests, written against the old code, before anything changed.**
Each one is what a person does, told through a small driver with the
words of the app: add a video, press New, press Cancel, press Render,
close the app, open it again, press Continue. The driver,
`cmd/framefairy-app/driver_test.go`, is the one place that knows which
calls do that. The tests, in `paths_test.go`, ran green on the old code
and were committed that way. The refactor then changed the driver and
nothing in the tests, and they stayed green through every batch after.
Everything but the two models is real: the queue, the engine, ffmpeg and
the files.

The paths:

- A video added, and the first search going by itself through every step
  to clips in the list.
- The app closed while it hears and while it finds, which in a test is a
  new queue reading the same work folders, and the search reported as
  interrupted.
- Continue after that, carrying on from where it was, with nothing heard
  twice.
- Cancel while it hears and while it finds, what was heard kept, and the
  search saying Stopped with Continue, after a restart too. This one
  changed on purpose after the refactor: it said nothing at first.
- A failure while it hears and while it finds, with its reason.
- A render to the end, and one that fails.
- Two episodes at once, and a render while a search runs, under the race
  detector.

Written against the old code, they found one bug on the way: a search
whose speech model could not be loaded said "step failed" instead of why.

Beside them: the engine's own tests of the records and of a search whose
lane is taken back, in `engine/jobs_test.go`, with a fuzz target for
reading a record, the lanes' tests in `lanes_test.go`, and
`steps_test.go`, which does everything the window can do to searches at
once. Writing those found two bugs of the new queue before they shipped: a
job called off at the moment its turn came still ran, and a turn given up
at the moment its lane came free still took it.

The interface's harness has one fake search that goes through the same
steps from the same kind of record, see the interface skill.

## Batches

One pull request, a commit per batch, merged once it has been tested on
Tim's machine and works.

1. The design.
2. The path tests and their driver, green on the old code.
3. The records, their timings, `Project.Search` and the render's record
   in the engine.
4. The app runs jobs through them in lanes, reads the records at start,
   asks for the first search when a video is added, and takes the calls
   above. The driver switched over, and the path tests stayed green.
5. The interface switched over, Pause gone, rendering in a lane of its
   own, and the harness to one fake search.
6. What went away, went, and the docs say how it is now.
