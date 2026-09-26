# The search

Design for making finding clips one thing in the code, the way it is one
thing for the person using the app. It is a plan: nothing here is built
yet. The batches at the end say in what order it is built, and each one is
marked in [GUI-PLAN.md](GUI-PLAN.md) as it lands.

## Why

A person asks for clips in a window of an episode and gets clips, or is
told why not. In between the episode is transcribed as far as the window
reaches, and a model reads the transcript and answers. To the person that
is one piece of work. The command line has always done it as one run:
`Run` transcribes and then plans.

The app took it apart. The transcription runs as a job in a lane of its
own, and the search is a second job in the other lane that waits for it,
reads the transcript file once a second, looks at the transcription job
every 20 ms, pauses it when it has heard the window, carries it on when
the pause fell short, and starts it again when it stopped. Before the
search job even exists, the interface holds the search in memory while
the transcript is not there yet, and decides by itself when to ask for it
and where the transcription is to stop.

So one piece of work has its state in three places that each know a part
of it:

| where | what it keeps |
| --- | --- |
| the interface, in memory | a search asked for and not started, where the transcription stops, whether the first search has happened, whether a search is starting, which jobs it waits on, about fifteen values in `Episode.svelte` |
| the app's Go side | two jobs in two lanes, the stop point of each transcription, which transcriptions a search paused, twelve functions that stitch the two jobs together |
| the work folder | `looked`, `search.json`, the transcript, the plans |

Every feature that touches finding clips has to find all three, and the
bugs of the last weeks were each a place that was missed: a search asked
for before the transcript was there left no note, because that search only
existed in the interface's memory, and the app closing looked like Cancel,
because nothing could tell the two apart.

## What a search is

A search is one piece of work on one episode, owned by the Go side. It
has a window, the numbers it was asked for, and a step it is in. There is
at most one search per episode at a time.

**Steps**, in order:

1. **Waiting.** Asked for, and waiting its turn behind other work on the
   machine.
2. **Transcribing.** The episode is transcribed from where the transcript
   ends to the end of the window, and no further. The transcription is
   this step, not a job of its own that the search waits for.
3. **Finding.** The model reads the window and answers, and the clips
   land one at a time as they do today.
4. **Done.** It found its clips.

And three ways to stop before that:

- **Paused**, by a hand, only while transcribing. It keeps how far it got
  and carries on from there. It is what the pause mark on the range
  picker does today, and it becomes a pause of the search.
- **Failed**, with the reason.
- **Called off**, by Cancel. The search is gone, and nothing is said.

A search whose record says transcribing or finding when the app starts is
a search that was **interrupted**: the app was closed or fell over. It is
not a separate state anybody writes. It is what a running search looks
like to an app that did not start it.

## The record

Every search keeps one record in its episode's work folder,
`search.json`, written on every change of step and on every report of how
far it has come, atomically, the way plans are written:

```json
{
  "id": "s-1790449…",
  "from": 1800, "to": 3600,
  "count": 12, "min": 20, "max": 30, "replan": false,
  "step": "transcribing",
  "covered": 2712.4,
  "found": 0,
  "error": "",
  "asked": "2026-09-26T19:05:00Z",
  "changed": "2026-09-26T19:12:31Z"
}
```

- `step` is one of `waiting`, `transcribing`, `finding`, `paused`,
  `failed`. A search that is done or called off has no record: there is
  nothing to say about it, and the plans say what it found.
- It is read as untrusted, like every file on disk: a record that is not
  one is no record.
- It replaces `looked` and today's `search.json`. Whether an episode has
  ever been searched is whether it has a plan or a record.

The record is the truth. The app never keeps a search anywhere else, and
the interface never keeps one at all.

## Who does what

**The engine** gets `Project.Search`, which runs the steps in order and
writes the record as it goes. It is the command line's `Run` split at the
seam it already has, transcribing and then planning, with the record
written around both. The command line keeps its flags and gets the same
record for free.

**The app** runs searches in its queue. A search is one job, and the queue
knows which of the machine's two resources each step needs: the speech
model for transcribing, the language model for finding. The two lanes stay,
so a transcription can still run while something is rendered, but a search
moves from one lane to the other as it goes from one step to the next,
instead of being two jobs that watch each other. When the app starts, it
reads the records of the episodes in its library, and a record that says
transcribing or finding is reported as interrupted.

Adding a video asks for the first search of its first window, which starts
in the waiting step. That is what the interface decides today with
`shouldTranscribe` and `shouldLook`, and it moves to the Go side, where the
episode is added.

**The interface** sends intents and shows state:

| intent | call |
| --- | --- |
| New, or Continue | `Search(path, window, numbers)` |
| Cancel | `CancelSearch(path)` |
| the pause mark | `PauseSearch(path)`, `Search` again carries it on |

It gets the state of an episode's search in one event, `search`, whenever
it changes, and on opening an episode it asks for it once. Everything it
shows about finding clips comes from that: the row the next clip will
appear in, New, Continue and Cancel, the fill on the range picker, the
note of a search that stopped. It holds no search of its own and decides
nothing about when anything starts or stops.

## What goes away

In the interface: `chosen.asked`, `chosen.held`, `chosen.looked`,
`lookPending`, `autoLook`, `stillWaiting` as a decision, `readyToLook` as
a decision, the effects that start a search or hold the transcription,
`shouldLook`, `shouldWarm`, `shouldTranscribe` and their tests, which
become tests of the Go side.

In the app's Go side: `Transcribe` as something the interface calls,
`HoldTranscription`, `holdOf`, `releaseHold`, `waitForTranscript`,
`pauseTranscriptions`, `carryOn`, `stopTranscription`, `AskSearch`,
`ForgetSearch`, the separate job kinds `transcribe` and `plan`.

In the engine: `MarkLooked`, `Looked`, `lookedName`, `SearchNote` and its
functions, folded into the record.

## What stays as it is

What a search finds and how: the prompt, the room a window has, the plans
and their files, the clips as they land, the training records. Rendering,
editing a clip, the clip timeline, the range picker's drawing. The command
line and its flags. The look of work in hand.

## How it is tested

The steps are one function on the Go side, so they are tested there, with
the fake speech model and the fake language model, the way searches are
tested today. And not one call at a time but the paths a person takes:

- New on an episode not transcribed, and the search going through every
  step to done.
- The app closed in each step, which in a test is a new queue reading the
  same records, and the search reported as interrupted, with how far it
  got.
- Continue after that, carrying on from where it was.
- Cancel in each step, and nothing left.
- Pause while transcribing, and carrying on.
- A failure in each step, with its reason.
- Adding a video, and the first search starting by itself.
- Two episodes at once, and a render while a search transcribes, under
  the race detector.

The interface's harness gets one fake search that goes through the same
steps from the same record, so what a probe sees is what the Go side
reports rather than a state made up for the probe.

## Batches

One pull request, a commit per batch, merged only once it has been tested
on Tim's machine and works.

1. This design.
2. The record and `Project.Search` in the engine, with its tests. Nothing
   uses it yet.
3. The app runs searches through it: one job, moving between the lanes,
   the records read at start, the first search asked for when a video is
   added, and the calls and the event above. The old calls stay for the
   moment, so the interface still works.
4. The interface switched over to the search's state and intents, and the
   harness to one fake search.
5. What goes away, goes, and the docs say how it is now.
