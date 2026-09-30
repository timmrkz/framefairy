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
a **search**, a **clip** made by hand, or a **render**. It is owned by the
Go side. It has what it was asked for and a step it is in. An episode has
at most one search at a time, and any number of clips and renders.

A search goes **waiting**, **hearing**, **finding**, **done**. A clip made
by hand goes **waiting**, **hearing** where the transcript does not reach
far enough yet, **framing**, **done**. A render goes **waiting**,
**rendering**, **done**. Waiting is its turn behind
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
  next search hears only what of its window is not heard yet.
- A search called off while finding keeps the clips that landed.
- A render called off keeps the shorts it finished.

Continue carries a job on from what it left. Hearing hears what of the
job's part of the episode is still not heard. Finding asks the model again for the whole window
and replaces the clips that landed, because an answer cannot be taken up
halfway. Rendering renders the clips that are not finished yet.

If using the app shows a pause is missed after all, it is a step that
stops without being called off, and adding it is small.

## Four lanes

The queue has a lane for each step's machinery: **hearing**, **finding**,
**rendering** and **framing**. Each runs one job at a time, and `lanes.go`
hands out the turns in the order they were asked for. A search asks for a
turn in the lane of each step as it comes to it, and lets the last one go.
A render is in the lane of rendering and waits for no search. A clip made
by hand places its crop in the lane of framing, so it waits for no search
either: a search places its own clips' crops in its turn of finding.

One rule is kept in `lanes.go`, where it can be seen: while a search finds,
no search hears, because the speech model and the language model want the
same memory and the same graphics chip, and each is slower for sharing
them. A search that comes to finding takes the lane of hearing back from a
search that hears, which saves what it heard, waits its turn and carries
on after. Work that is not a step of a search, a model being installed, is
neither taken back nor held up.

And a clip made by hand hears first. Somebody pressed I or O and is
looking at its card, and it hears the minute around the playhead where a
search may hear an hour. So it goes ahead of every search waiting to hear,
takes the lane back from one that hears, the way finding does, and does not
wait for a search that finds: a minute of audio is heard in a second or
two. Clips made by hand hear in the order they were asked for, and the
search carries on after them. That is a kind of turn of its own,
`handHearingTurn`, and nothing else about a clip made by hand is special
in the queue.

Model installs share the lane of the step that needs them: the speech
model the lane of hearing, the language model the lane of finding.

## The record

Every job keeps one record in its episode's work folder, in `jobs/`,
written atomically on every change of step. The search is
`jobs/search.json`, because an episode has one at a time, a clip made by
hand is `jobs/clip-<id>.json` and a render is `jobs/render-<id>.json`. The
code is `engine/jobs.go`.

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

- `step` is one of `waiting`, `hearing`, `finding`, `framing`,
  `rendering`, `stopped` and `failed`, with `error` beside a failed one. A search
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
its first search, of its first window, see `engine.SuggestedWindow`, and
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

## Clips made by hand

The model is probabilistic and sometimes misses the moment a person wants.
**I** and **O** make a clip at the playhead, the way In and Out mark one in
every video editor: **I** starts it with the sentence the playhead stands
in and grows it forward, **O** ends it with that sentence and grows it
back, for a moment noticed once it has passed. It grows a whole sentence at
a time until it is as long as **Shortest**, never past **Longest** for one
sentence more.

A clip made by hand is not a second kind of clip. It is made by the same
parts, in the same order, as a clip a search makes, and only what differs
between the two is its own:

| part | a search | a clip made by hand |
| --- | --- | --- |
| **hearing** | the transcript up to the end of the window | the transcript up to where the clip can reach, `Longest` and a sentence past the playhead |
| **proposing** | the model names clips | the playhead names one, see `handEntry` |
| **making** | the plan builder shapes each onto sentences, places its crop, gives it an id and writes it | the same |
| **its clip set** | the window's, made anew by each search of it | the episode's clips made by hand, `clips-hand.json`, which grows |
| **searched** | the window is marked searched | nothing is, since no model looked |

So there is one transcript, heard from the start the one way it always
is, and one plan builder, which takes entries from whatever proposed them.
The builder's intake is `propose`: the scanner of the model's answer calls
it for every clip it reads, and a clip made by hand calls it once. The
hearing is `hear`, the step both jobs take, pulled out of the search.
Where a clip came from is a field of its clip set, `planned_with.by`, and
one place reads it: `madeOver`, the part of the episode a set was made
over, which for the clips made by hand is none. That is what keeps them
from marking anything searched and from going when a searched part is
given back, and it replaced a check on the file's name.

A clip set that grows is a builder option, `Grows`, with the ids after the
ones it has, `h01`, `h02`, and so on. Each clip takes its number under the
set's lock as it is written, `addClip`, so two clips made at the same
moment never take the same one.

**It is a job**, like a search and a render: `clip` in `jobs/clip-<id>.json`,
going **waiting**, **hearing** when the transcript does not reach far
enough yet, **framing**, **done**. An episode can make any number at a time,
the way it can render any number, so I and O can be pressed while a search
runs and while other clips are still being made. Framing has a lane of its
own, the fourth, because placing a crop decodes the picture and waits for
neither model: a clip made by hand never waits for a search to finish
finding. A search frames its clips in its own finding turn, as it always
did.

**A clip is on the list from the moment it is on its way.** Every job says
which clips it has on the way, `EventUnderway`, and the job carries the
list as `underway`: where each lies, what it is called once that is known,
and the step it is in. The plan builder keeps it for every job alike, from
the moment a clip is queued to be framed until it is written or let go,
and a clip made by hand is in it from the moment the key is pressed, at
the playhead, before a word of it is known. The clip list shows every clip
on the way in its place in the episode, with the beam round it, in the
words of `arrivalLine`, see `frontend/src/lib/arriving.ts`. So a clip the
model named and a clip made by hand come in the same way, and a search no
longer shows its clips only once their crops are placed.

Placing the crop is the slow part, seconds a clip, and what a clip keeps
is known before it: the builder cuts the pauses first. So the list says
what it keeps, `Pieces` and the words said in them, the moment the pauses
are cut, and the app lays its captions out from them with the code a
written clip's captions go through, `ArrivingCaptionsView`. A clip on its
way that is chosen is drawn on the clip timeline the way a clip is: its
frame, then its pieces, then its caption blocks coming in one after
another. When it is written, the same blocks stay.

A clip made by hand says how far placing its crop has come, see
`engine/cropwork.go`. The work is twice what the clip keeps in seconds,
once scanned for camera switches and once sampled for faces, and each
piece scanned and each shot sampled is that much of it done. Between two
of them the share moves on by the clock, at the speed the work has gone so
far, never past the piece in hand. The job holds the progress line while
it does, so the scans inside it neither show their own progress nor clear
it. A search's clips count nothing of their own: the search says how far
it has come as a whole, and one of its cards carries that.

A card hands over to the clip it becomes in one step: it keeps its place
until the list has read the clip, so there is never a gap where it was and
never the two of them at once. It goes in the same assignment that puts
the list on screen, from the first read asked after the clip was written,
whichever read answers first. The event that takes a written clip off the
list also counts it, `written` on the job, so the rows a search holds
open, the number asked for less what it has written and what it has on
the way, come from one event and never add a clip up twice. It said it
had found one more in one event and took the clip off the list in the
next, and between the two the list showed four cards for a search asked
for three. A clip written between two job events, never on the way in
either, holds a row of its own until a read brings it in.

A clip of the answer well off the length is held back and asked for
again, `fit`, and it is on its way like any other from the moment it is
named, its card saying *Fitting to the length*. It keeps its card, a
number given when the model names it, when it goes to be framed. Once the
answer is read to its end, the list says it is whole, `Whole` on the event
and `whole` on the job: every clip still to come has a card, so the list
holds no row open for a clip the model did not name. That row stood in for
the held clips and for the ones never named, with the fill of the whole
search stuck near its end while the cards were framed.

A clip on its way is marked on the range picker and the clip timeline
from the moment its card has its place, where the card says it lies,
breathing the way everything not there yet does, and it keeps its mark
when it is written, because the mark is known by the clip it will be. The
marks came only with the written clip, so the two tracks said where a
clip lay long after its card did.

A search says how many clips it was asked for, `count` on the job, and
the rows it holds open and the target in the workspace follow that number
while it runs, not the one the workspace would ask for now. The first
search of an episode is asked for by the Go side, and the workspace worked
out another number, so the list opened three rows for a search that looked
for six.

A card and its clip are one row of the list. The card says which clip it
will be, `Clip` on `Underway`, the clip set's file name and the id the
builder gave it when it queued it, and the list keeps the card's row for
the clip. They were two rows, the card's going and the clip's coming, and
between the two the list was a row short for an instant, which a list
scrolled down answered by jumping. The card is held from the moment it
leaves the job's list in the same pass that builds the list, `OnTheWay`
in `frontend/src/lib/arriving.ts`: an effect held it a render late, so
every card went away, left a row still to come in its place and slid back
open before it became its clip, which at the end of a search read as the
whole list blinking. A clip made by hand is chosen the moment
the key is pressed, as its card, and the card that is chosen stays chosen
as the clip, unless another has been chosen since. It was chosen only once
it landed, so for the seconds it took nothing on the clip timeline said
anything was happening.

A clip whose job was called off with Cancel, cut off by the app closing,
or failed, stays where it would have appeared, still, saying so, and a
click carries it on. Cancel at the head of the clip list stops all the
work that makes clips, the search and every clip made by hand, and each
says Stopped where it is. Continue carries all of them on. That is one
property, `makesClips`: a job that makes clips keeps its record when it is
called off, and a render called off goes. Its
record gives the card back after a restart, `JobRecord.Underway`.

**Not undone by Cmd+Z.** A search's clips are not either: undo takes back
changes to clips, and making one is work, see `LeaveOutNewClips`. A clip
made by hand is taken away the way every clip is, with its trash can, and
put back from there.

**Heard where it is needed.** The transcript is heard in parts, see
[ENGINE.md](ENGINE.md), and a clip made by hand has the part around the
playhead heard, `ClipRequest.reach`, wherever the rest of the transcript
has got to. A search has its window heard the same way, and nothing before
it. An earlier attempt heard the part around the playhead out of turn into
files of its own, islands, and read them into the transcript wherever the
transcription from the start had not come. That was a second transcript
with seams, heard again once the first arrived, and a second speech model
loaded beside the pool. The parts are the one transcript, the way the
loudness is one file with parts since the waveform went where the clip
timeline looks.

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
