# Design review

A review of the design of the whole app, made on 6 October 2026 against
main at `bd691ef`, asked for by Tim. The question: where is a part built
in a way that makes it harder, slower, less reliable or less safe than it
needs to be, and which kinds of bug keep coming back because of how
something is built rather than how it was written.

[ASSESSMENT.md](ASSESSMENT.md) is the review of bugs and engineering
practice from 29 September, and [ROBUSTNESS.md](ROBUSTNESS.md) the audit of
the handoffs between the parts. Nothing here repeats them, except where a
design they name is still producing new bugs.

How it was made: five reviews side by side, one each for the history of
the pull requests, the engine, the app's Go side, the interface and
security. Each read the code and the docs, and the engine, app and
interface reviews measured on the cloud machine, a 4-core Xeon at 2.1 GHz.
An M2 Max is about twice as fast per core, so the times there are lower,
and the ratios hold. Every finding says whether it was **measured**,
**proved** by running it, or **read** from the code. The bugs in the first
part were checked again by hand.

Line numbers are as found and drift as the code moves.

## The short answer

Tim's hunch is right. Most of the bugs of the last month come from six
design decisions, and each of them has been patched place by place
instead of changed once. The one time a design was changed instead, the
video preview's own frame queue in #110, a family of about ten pull
requests ended and has not come back.

1. **A moment is a float of seconds rounded to the millisecond, and five
   places each decide which frame it falls in.** That is the clock family:
   captions against sound, frames lost at a cut, an edge a frame off. It
   is still live, see the first bug below.
2. **There is no episode object.** Nothing holds an episode's transcript,
   plans and levels in memory. Every question is answered by reading and
   parsing the files again, and one function, `engine.Status`, does far
   too much and is used for everything. That is most of what makes edits
   slow on a long episode.
3. **The interface rebuilds Go's state by itself.** Go sends whole job
   objects and pings that carry only a path. The interface guesses when to
   read everything again, with timers, tickets and a pending flag for each
   control. `Episode.svelte` is 3,209 lines with 18 effects, and it keeps
   growing.
4. **Every question about the media is a new ffmpeg run that seeks and
   decodes again.** Framing decodes every kept second twice, a render does
   its setup again for every clip, and every ffmpeg release moves the sound
   a little after a seek.
5. **The library is checked by path strings, 37 times, and the webview has
   no second wall.** A work folder that is itself a link takes its whole
   target into the library.
6. **What is tested is not what ships.** The walks run in Chromium, the app
   runs in WebKit. Linux CI and the walks use Ubuntu's ffmpeg 6.1, the app
   ships 9.0.2.

Each of these has one change that ends its family. They are listed in
order at the end.

## Bugs found on the way

These are live today, independent of any redesign, and small to fix.

| # | Bug | Where | How it was found |
| -: | --- | --- | --- |
| 1 | At 29.97 fps the video preview and the short disagree by one frame at about half of all edges. An edge is saved to the millisecond, which lands just before its frame for 1455 of 3000 frames. The interface floors and draws the frame before, the render rounds and starts on the frame itself. | `engine/shape.go:289`, `frontend/src/lib/playhead.ts:73`, `frontend/src/lib/frames/mp4.ts:770`, `engine/render.go:206` | read, and the count checked by arithmetic. Not seen in the app yet. The walks cannot see it, their episode is 5 fps, where every frame starts on a whole millisecond. |
| 2 | Removing the language model during a search, or the speech model while something is heard, is not refused. The guards ask for job kinds `plan` and `transcribe`, which no longer exist. | `cmd/framefairy-app/setup.go:382`, `:408` | proved, the model file was deleted mid-search, and checked again by hand |
| 3 | A work folder `episode.framefairy` that is itself a link to the home folder makes every file in it part of the library. The media route then serves `~/.ssh/id_ed25519`. A zip from an editor can carry such a link. | `cmd/framefairy-app/settings.go:272`, `main.go:228` | proved |
| 4 | A search the Go side refuses leaves the row saying Finding clips for good, with Cancel disabled and the reason never shown. The interface throws away the job `Search` returns and waits for a running job that never comes. | `frontend/src/screens/Episode.svelte:1752`, `:1852`, `cmd/framefairy-app/jobs.go:381` | proved in the harness, and read again by hand |
| 5 | Start a render, open another episode, come back: when the render ends, the clip list is not read again, so the rendered dot never appears and a failed render never says why. The render watch belongs to the mounted component. | `Episode.svelte:1890`, `:1998` | read |
| 6 | Put captions back is up to six Go calls and six undo steps. Cmd+Z then undoes only the colours, and "as they were" can bring back stale values. | `Episode.svelte:1609-1631`, `:1784` | read |
| 7 | The caption overrides for the caption box are a copy of the render's, and they have drifted: the render sets the margin only when a height is set and checks the colour, the copy does neither. | `cmd/framefairy-app/views.go:165`, `engine/run.go:627` | read |
| 8 | Three settings writers still read, change and write back outside the lock, the lost update ROBUSTNESS #10 removed elsewhere. | `cmd/framefairy-app/setup.go:254`, `:351`, `:391` | read |
| 9 | The licence server's thank-you page gives the keys to anyone who has the Paddle transaction id, for 24 hours. The id travels in the thank-you link, receipts and support mail. | `licence/dispenser/api.go:287`, `:326` | proved. Fixed in #146: a nonce from the checkout page, kept hashed with the sale and required, and an hour |
| 10 | Cmd+Z from the menu edits the episode behind the Remove box. Six copies of "is someone typing or is a box open" disagree. | `Episode.svelte:1212`, `:1224`, `:2034`, `ClipTimeline.svelte:743`, `Player.svelte:950`, `:982` | read |

## The families, from the history

Read from every merged pull request, the plan and the rules in CLAUDE.md,
many of which were written after a bug. A rough count of the pull
requests each has cost so far.

### The video preview's clock: ended by a design change

About ten pull requests (#5, #6, #49, #97, #98, #100, #102, #105) all came
from asking a `<video>` element what time it is, a clock that is only an
estimate. #110 replaced it with the app's own frame queue, and none has
come back. This is the proof that changing the design works where
patching does not.

### Time as rounded seconds: live, about 8 pull requests

#5 (the two ends of a cut rounded on their own), #72 (libass given 199.999
ms for 0.2 s), #111 (a reset edge a frame off), #117 (frames lost at five
frame rates), ASSESSMENT 9, and 2.130 still open for variable frame rates.

The cause: a moment is a float of seconds, saved to the millisecond
(`shape.go:291`). Which frame holds it is answered by five rules: `cutOf`
rounds with the rational rate, `OnFrames` rounds with the rate as a float,
`onFrame` rounds and then cuts to the millisecond, `playhead.ts` floors
with 1e-6 of slack and `rankAt` floors with a microsecond. The frame rate
itself comes from two places, ffprobe and the sample durations in the
file. Small tolerances, 1e-9, 1e-6, 0.0005 and 0.0015, paper over the
differences. Bug 1 above is the next one.

What ends it: an edge is a frame number once the rate is known, or a tick
of the episode's own timebase. One function in Go turns a moment into a
frame, and the TypeScript side is tested against a shared file of cases,
the way `suggest.cases.json` already works. Seconds then appear only on
screen and in ffmpeg's arguments.

### ffmpeg seek and pipe: live, about 6 pull requests

#28 (40 ms wrong after a seek), #31, #54 (looking for camera switches
hung), #117, #118 (ffmpeg 9 drops the first packet), and #122, still
open (Opus decodes 80 ms quiet after a seek).

The cause: every read of a part of the episode is a fresh ffmpeg that
seeks and pipes raw samples, which carry no time. So every codec's and
every release's way of seeking moves the sound. The 0.2 s lead in
`levels.go` and `render.go` is a patch, and may already cover #122.

What ends it: decode the sound once into a cache indexed by sample, and
serve the waveform, the transcription and the render from it. Short of
that, cut by timestamp inside the one graph everywhere, which #118 began.
And test against the ffmpeg we ship, see the last family.

### The interface rebuilds Go's state: live, about 10 pull requests

ROBUSTNESS 5, 8 and 12, #2, #37, #52, #56, #104, #63, the rule "what
watches running work does not watch the job", and 2.131, open, and 2.132,
fixed since in #133. See the interface section below.

### Tested in Chromium, shipped in WebKit: live, about 8 pull requests

#4, #49, #100, #63 and #65 (three pull requests for one ten-second fill),
and #120, open: WebKit's `VideoDecoder` says yes to HEVC Main 10 and then
fails on the first frame. Every walk and every probe launches Chromium
(`walks/bridge.mjs:5`, `preview/open.mjs:9`), and the harness imitates
WebKit with modes written from reading its source. #65 showed that
WebKitGTK does reproduce the Mac.

What ends it: run the playback and clip timeline walks in Playwright's
WebKit as well.

### One thing written twice: live, about 8 pull requests

#24 then #31 (the clip made by hand, built beside the pipeline and then
rebuilt on it), #33 (words kept twice), #61 (a second fill that drifted),
#72 (the video preview lit words by another rule than the render).
Still there:

- `frontend/preview/wails-stub.ts`, 1,781 lines, cuts, shapes and lays
  captions out by itself. About 40 pull requests had to touch it.
- The TypeScript types and caption constants are copied by hand
  (`api.ts:880-899`, ASSESSMENT 25).
- `found` and `found_segments` keep the same fact twice
  (`engine/edit.go:610-625`).
- The loudness is kept twice, in `words.frames` and `levels.frames`, from
  two decodes of the same sound (see the engine section).
- The caption overrides, bug 7.

What ends it: generate the types and constants from Go, keep the stub for
screen states only and let the bridge answer for behaviour, work `found`
out from `found_segments`, keep one loudness.

### Which tests run, and where: live, about 8 pull requests

#44, #54, #89, #8, #4, #90, #119, and 2.129, open: the render tests skip
without a word in CI. Hand-written path rules decide what runs
(`ci-needs.sh`, and `changed.sh` at 370 lines), and 54 `t.Skip` calls turn
a missing tool into a pass.

What ends it: choose the Go tests from `go list -deps` and a checked list
of the files each test reads. In CI a list of required tools turns those
skips into failures. Run the Linux tests and the walks against the ffmpeg
`build-ffmpeg.sh` builds, which CI already caches.

### Contained

The update channel list (#36, #55, #69, #93) ended with the two copies in
#93. Layout worked out in JavaScript is mostly contained since 2.43, with
`--above` (`Episode.svelte:2186`) and `centre()` (`:2619`) left,
ASSESSMENT 28. ASSESSMENT 30 was fixed in #132.

## The engine

### Framing decodes every kept second twice

`ClipSegments` (`analysis.go:542`) runs, for each clip, `DetectShots` on
every piece (a full decode for the camera switches), `sameShot` at every
gap (two more seeks), and `samplePositions` on every part (another full
decode). A 24 s clip in three pieces is 11 ffmpeg runs and decodes
everything twice: **18.1 s measured**.

One ffmpeg per piece can decode once, scale once to a small grey picture
and split it: one branch finds the camera switches, the other samples the
frames for the faces. The frames either side of a gap are the last and
first frames of the neighbouring pieces, so `sameShot` needs no run of
its own. The same work in 3 runs took **8.7 s, measured**, 2.1 times
faster. Framing the first clip is most of the wait for it.

On a Mac, `-hwaccel auto` with no output format copies every frame back
to memory and scales it in software. Keeping the frame on the GPU and
scaling it there would shrink that further, estimated, it cannot be
measured here.

### No episode object: everything is read again

`engine.Status` (`episode.go:78`) parses the whole transcript (words and
frames), reads the levels, parses every plan twice and reads the timings.
The app calls it only to list the plans, in `Clips`, `Coverage`,
`RemoveSearch` and `followTheHeight`. `ReadPlan` then lists the plans once
more for each plan.

| On a 4-hour episode, measured | Time |
| --- | ---: |
| `Status` | 71 ms |
| `Clips`, 8 plans | 55 ms |
| `Clips`, 24 plans, 68 MB of garbage | 156 ms |
| `PlanSummaries`, which is all `Clips` needs | 0.03 ms |
| Reading each plan once | 2.5 ms |
| Remove one clip, one click | 90 ms |
| Loading the transcript | 110 ms, 50 MB |

Who pays:

- Every edit returns its clip by rebuilding the whole clip list through
  `Status`, and takes two undo snapshots that parse every plan again.
- While a search finds clips, the interface reads `Clips` every 2 s, and
  `Episode` (another `Status`) every 2 s while hearing, twice a second
  while measuring the loudness.
- Every job end, a clip made by hand included, sends an `episode` event,
  and the interface answers with `Library()`, which runs `Status` for
  every episode. Ten long episodes cost about 0.7 s for each job end.
- One word corrected reloads the transcript twice, about 220 ms.
- The transcript cache in the app has one slot for all episodes, and
  every checkpoint and every correction empties it. Several readers then
  each load it again at once.

The cost grows with the length of the episode and the size of the
library, never with what changed.

What ends it, in two steps:

1. Now, small: `Clips` and the other three list the plans with
   `PlanSummaries`, `ReadPlan` takes the summary it is given, and an edit
   returns the clip it just wrote. 156 ms down to a few ms per edit.
2. Then: an engine `Episode` that holds the parsed transcript, plans and
   levels, keyed by the files' stamps, one load in flight at a time,
   corrections applied in memory (9.5 ms against a 110 ms reload), and the
   heard parts in a small header that can be read alone. This is
   ASSESSMENT #19, the episode type, and it is where most of the speed is.

### Every gesture scans the words of the whole episode

`ClipCaptions` hands every word of the episode to `Captions`, which copies
them, and `moveCaptions` looks each caption's word up by scanning all of
them, twice per caption. Shape runs once per pointer move while an edge
is dragged, builds the captions twice for an end trim, and parses the plan
four times. **10.4 ms per move measured** on 4 hours.

Cutting the words to the clip and 30 s either side, with two binary
searches, before `Captions`: **3.6 ms measured**, with every engine test
passing. A memo of the hyphenation, by word, style and language, would
bring it near 1 ms, estimated.

### A render does its setup again for every clip

Each clip of a render job runs `prepare` again: `ffmpeg -version` and
`ffprobe -version`, a probe, a transcript reload, and a measurement of
the caption ink through libass. The app also makes a new `Engine` for
every job (`jobs.go:720`), so the encoder list and the subtitle check are
found again each time. **About 0.7 s per clip before encoding starts,
measured.** The render decodes in software too: `render.go` never uses
`decodeFlags`, which framing does. Decoding three pieces was about 7 s of
a 12.7 s render here.

What to change: one `Engine` for the app, `prepare` once per job, a cache
for the caption measuring keyed by font, size and text, the length read
from the progress ffmpeg already reports instead of another probe,
hardware decode on the render's inputs, and two clips at once on Apple
silicon, whose Max chips have two encoders, estimated 1.5 to 1.8 times on
a batch of six.

### Smaller

- Every atomic write is a full drive flush, `F_FULLFSYNC` on macOS, the
  loudness rewritten whole (5.8 MB at 4 hours) every half second while
  measuring included. A full flush is for what cannot be made again:
  plans and corrections. A cache gets a plain rename, and the loudness is
  written for the range just measured.
- The loudness is kept twice, see the families. `Levels.Over` merges the
  two by copying the whole episode on every waveform read, 9 ms at 4
  hours.
- The x264 preset is `slow`: a 24 s short took 3 min 3 s here against
  27.8 s at `veryfast`. It reaches only the command line and Linux today.
- Framers each give the face detector half the cores, so four framers
  ask for twice the machine. One pool shared by the framers fits it.
- `ENGINE.md` still says a window is transcribed by decoding from the
  start, which the section on parts contradicts.

## The app's Go side

### A job has two state machines and three ways to tell the interface

JOBS.md says the record is the truth. But `Job` (`jobs.go:31`) copies the
record field by field, and two writers keep them in step: the engine
writes the record's step, the app its own copy. `cancelled` means four
things and `interrupted` two, told apart by the step.

The interface learns of changes by full job snapshots, by `episode` and
`levels` events that carry only a path, by polls (jobs every 5 s, Episode
and Clips every 2 s) and by a counter on the job as a hint that a plan
changed. Clip lists have no change notice at all. Nothing throttles job
events: crop work reports 4 times a second, the search clock 2, ffmpeg
about 3, so several jobs at once send 10 to 20 whole snapshots a second.
Each replaces the interface's job list and re-runs everything derived
from it. That is the churn the rule "what watches running work does not
watch the job" works around.

What ends it:

- The job is a view of the record, `Job{*engine.JobRecord, State,
  Progress, Seq}`, and only the engine writes the step.
- One revision number per episode, raised under one lock on every write
  the app makes or sees: plans, transcript, levels, records. One event
  `{episode, rev, changed}` at most four times a second, with the last
  always sent. State changes go at once.
- The interface reads again only what is behind its revision. The three
  polls and the ordering tickets go.

### Smaller

- Two queue paths and five ways in (`add`, `addOnce`, `addFor`, `queue`,
  `queueFor`), one of them unused. Waiters poll every 20 to 100 ms, and
  `Search` can hold a call for 15 s. `RemoveEpisode` waits with no limit
  on a loudness measuring whose ffmpeg may hang. One `add`, and a `done`
  channel per job that waiters select on with a deadline.
- 69 bindings with no one rule for errors. Calls that start work never
  return an error and record a refusal as a failed job for good. Others
  return text the interface can only tell apart by its wording. One error
  type with a code, and job calls that return `(Job, error)`.
- Engine work done in the app: the caption overrides (bug 7), the
  "rendered" training record (only the app records it, from the plan as
  it is after the render), the plan name of a search worked out again,
  the warm-up decision in `Search`. Each belongs behind `Project`.
- Job kinds are strings typed again at every call site, which is how bug 2
  happened without a compile error. One typed `JobKind`, and "busy with"
  judged by which lane a job holds rather than its kind.

## The interface

`Episode.svelte` is 3,209 lines with 50 `$state`, 91 `$derived` and 18
`$effect`, up from ASSESSMENT 23. The biggest after it are `ClipTimeline`
(1,995), `Player` (1,461) and `frames/queue.ts` (1,424).

### Go only pings, and the interface guesses when to read everything

One search of 6 clips in the harness read the whole clip list 15 times
and the captions 10 times, while the chosen clip changed twice. The ends
of transcription, search and render are each noticed by an effect that
keeps the value from before in a plain variable. That is how bugs 4 and
5 happen.

What ends it: the revision events above, and one `EpisodeSession` store
in `lib/` that reads each revision once, drops older answers, and calls
back when a job ends. `Episode.svelte` becomes a view.

### One pending flag per control

`starting`, `stopping`, `renderAsked`, `renderStopping`, `pressed`, and
`cancelling` in Settings: each is set by a click and cleared by its own
effect. Only `makeClip` uses the job it gets back. One `startJob(call)`
helper applies the returned job, ties the pending state to its id, ends
it when the job ends and shows the job's error in its row. That fixes
bug 4 for every control at once.

### Rules about where things land still live in the interface

CLAUDE.md says `ShapeClip` and `Reshape` answer for every gesture. Still
in the interface: the caption edges (frame rounding, 0.1 s, the stop at
the caption beside, `ClipTimeline.svelte:1110`), the thumbnail frame
(`:1195`), the crop clamp (`Player.svelte:870`, a copy of
`engine.ClampCropX`), each piece's framing (`Episode.svelte:1321`), a
second clip clock (`Player.svelte:224`, a copy of `flow.ts` `inClip`),
and the step the window snaps to, worked out from the width in pixels, so
which window is searched depends on how wide the app is.

What ends it: caption, thumbnail and crop kinds of `Gesture`, through the
same `api.shape` and `reshape` the clip edges use, and the window's step
taken from the episode's length.

### One click, one edit

Put captions back and the colour pickers make several Go calls, each its
own undo step, each followed by a full clip read (bug 6). Beside Cmd+Z
there are local ways back that do not know about it: `undoCrop`,
`captionsWere`, the removed rows and the player's removed words. One
`SetCaptionLook(patch)` is one edit and one step, and a local way back is
then one of Go's undo steps, not a second history.

### Objects rebuilt stand in for "something changed"

Every clip-list read makes new objects, so effects that compare identity
throw work in hand away: the colour being dragged is dropped mid-drag,
the caption box flashes back to the saved colour during a search, and a
removed word can come back before Go answers. Several derived values also
change hidden state as they are read. The revision ends this too.

### Speed is fine

Playback in the harness, 5 s at 60 frames a second: about 1.75 ms of
script per frame, most of it drawing. Job events of another episode cost
nothing measurable. Edge drags keep one shape call in flight with the
newest gesture. Small waste: the waveform sets `canvas.width` and reads
the computed style on every view change.

## Security

What is already done well, checked: ffmpeg is built without network and
reads only files and pipes, names in filter graphs are limited to
`[A-Za-z0-9_-]` and caption text is escaped, every program is started
with an argument list and never through a shell, llama-server listens on
the loopback only with a key per run, downloads are pinned by sha256, API
keys are in the Keychain, Paddle webhooks are checked by HMAC and the
sale is then asked of Paddle itself, the dispenser keeps its tokens
hashed, and `publish.yml` signs only from main.

### The library is checked by path strings

Bug 3. `Known` resolves both sides and then accepts anything under the
resolved work folder: the rule "a link inside a work folder is no way
out" was applied to what is inside and not to the folder itself. There
are 37 separate `Known` and `PlanOf` checks on strings across the
bindings, each followed later by an open, so a link can be swapped in
between, and they drift: `ClipPlayed` uses `Known` where `PlanOf`
belongs.

What ends the class: the store hands out an episode handle, not a path.
When a video is added and when it is opened, the video and its work folder
are checked to be no links. Every file operation in a work folder goes
through an `os.Root` opened on it, which refuses an escape when the file
is opened, so there is no gap between the check and the open. Bindings
and the media route take an episode and a name inside its work folder.
This is the safe half of ASSESSMENT #19, the episode type, the same
change that brings most of the speed above.

### The webview has no second wall

No Content-Security-Policy. The media route serves any library file with a
type guessed from its name, HTML and SVG included. Every binding can be
called by any script in the page. Settings that are paths come from the
interface (`llmModel`, `outputDir` and others), and a language model
that is not a known name in the models folder skips its checksum. Safe
today because nothing in `frontend/src` uses `{@html}`, `innerHTML` or
`eval`, by convention. One injection would join bug 3 into reading any
file and running any code.

What ends it: a strict CSP, media answered with `sandbox`, `nosniff` and
a list of allowed types, models chosen by name only, and folders only
through the native dialog, kept on the Go side.

### Environment variables choose the programs a shipped app runs

`FRAMEFAIRY_FFMPEG`, `FRAMEFAIRY_FFPROBE`, `FRAMEFAIRY_LLAMA_SERVER` and
`FRAMEFAIRY_ASR_MODEL` are honoured with no checksum in a production build
(proved). On a Mac, `launchctl setenv` from any process of the same user
reaches the next launch, and the child inherits the app's privacy grants
to Documents and Desktop. Compile them out of production builds.

### The licence server

- Bug 9, the thank-you page: a nonce made on the checkout page, passed to
  Paddle as custom data, stored hashed with the sale and required on
  `/v1/thanks`, and a shorter window. Done in #146, with an hour, see
  [LICENCE.md](LICENCE.md#every-endpoint).
- Every sold key is kept as plain text, in the database and in the backup
  locked for a year (`licence/dispenser/store.go:29`). LICENCE.md says the
  worst leak is unsold keys, but a leak of the database or a backup is
  every customer's key. Encrypt the key text at rest with a key held only
  in the container's secret store, and look keys up by fingerprint.
- The lost-key limits count per connection address, and behind Scaleway's
  gateway every buyer shares one. A trusted client address in production,
  and the counts in the database.

### Updates, before customers get them

The update feed has no sequence and no expiry, and `Written` is not
signed, so an old feed can be replayed to roll the app back, and once
revocations travel in it, to restore revoked keys. The build signature is
over a bare digest with no domain prefix, and the development key signs
whatever a branch builds. The unpacked app waits in a temporary folder
until quit and is moved into place unchecked. Before the customer
channel: one signed feed with a domain prefix, a rising sequence and an
expiry, a version that only goes up, one key per purpose, approval for a
large change in revocations, and the code signature checked against the
Team ID just before the swap.

## What to change, in order

Small first, then the three changes that end families. Each is its own
pull request.

**Done so far**, in #123 with this review, measured on the cloud machine:

- Change 3: the plans are listed with `PlanSummaries`, a plan's view is
  made from what its summary read, and an edit reads back only its own
  plan. On four hours, the clip list went from 73 ms to 0.1 ms and removing
  a clip from 90 ms to 2.4 ms. A clip's captions read the words near the
  clip: a pointer move on an edge went from 12.7 ms to 3.6 ms, and from
  2.6 MB to 0.8 MB.
- Change 5: framing reads each kept span once, `readSpan`. A clip of 24 s
  in three pieces from a 1080p video went from 18.1 s to 9.9 s, with the
  same crops.

In #130:

- Change 1, bugs 2, 4, 8 and 10. The model guards ask for the job kinds
  that run, `engine.JobSearch` and `engine.JobClip`. Every settings writer
  goes through `UpdateSettings`. New and Continue read the job they get
  back, so a refused search ends the start and its row says why, and the
  refused search keeps its window. Who a key belongs to is asked in one
  place, `lib/keys.ts`, which also stopped the space bar from playing the
  episode behind the box rather than pressing the button that had the
  keyboard. A `startJob` helper for every control is still open.
- Change 2, the links half: a video or a work folder that is a link is no
  episode. `nosniff`, `sandbox` and the CSP are still open, after #120.
- Security: the app as it ships takes no program and no model from the
  `FRAMEFAIRY_*` variables.
- From Tim's test of bug 2: the guard was only half of it. The models a
  search uses are now locked from its first moment, hearing included, in
  the interface and on the Go side, and choosing another model is
  refused too, not only removing one. A walk sequence holds it.

| # | Change | Size | What it ends or gains |
| -: | --- | --- | --- |
| 1 | Bugs 2, 4, 8 and 10: typed job kinds with the model guards fixed, a `startJob` helper for every control, `UpdateSettings` everywhere, one "is someone typing" check | small | four live bugs |
| 2 | Bug 3 now: refuse a work folder or video that is a link, at add and at open, and serve media with `nosniff`, `sandbox` and allowed types. A CSP for the app. | small | the file-reading hole |
| 3 | `Clips` and the others list plans with `PlanSummaries`, `ReadPlan` takes its summary, an edit returns its clip. Words cut to the clip before captions. | small | every edit: 156 ms to a few ms on 4 hours, measured. A drag: 10.4 to 3.6 ms, measured |
| 4 | Edges as frame numbers, one frame function in Go, the interface tested against shared cases | medium | the time family, bug 1 |
| 5 | Framing in one decode per piece, `sameShot` from the edge frames | medium | 2.1 times faster framing, measured, so the first clip sooner |
| 6 | Render setup once per job, one `Engine` for the app, a cache for caption measuring, hardware decode, two clips at once on the Mac | medium | about 0.7 s per clip and the decode, measured, more in parallel, estimated |
| 7 | An episode revision and one coalesced change event from Go, an `EpisodeSession` store in the interface, the polls and tickets removed | large | the state family, bug 5, the read storms |
| 8 | The engine `Episode` type: parsed transcript, plans and levels held by stamp, one loudness, `os.Root` for every file, bindings by episode handle | large | the speed of everything that reads, and the path class of bugs |
| 9 | Caption, thumbnail and crop as gestures, `SetCaptionLook` as one edit | medium | rules out of the interface, bug 6 |
| 10 | Walks in WebKit too, Linux tests and walks on our ffmpeg, required tools in CI | medium | the WebKit and ffmpeg surprises found by CI, not by Tim |
| 11 | Licence server: thank-you nonce, keys encrypted at rest, trusted client address | medium | bug 9 and the backup leak |
| 12 | Before the customer channel: the signed feed with a sequence and expiry, keys per purpose, the signature checked at swap, the tool variables out of production | medium | rollback and replay |
