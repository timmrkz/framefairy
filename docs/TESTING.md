# Testing the interface

What runs where, and how to see the interface do what a person does with
it, without a screen. The engine's own tests, the fuzz targets and the
path tests are in [BUILD.md](BUILD.md). This page is about the interface.
The bridge and the path tests need ffmpeg: without one they skip on a
machine of one's own and fail in CI, through `internal/ffmpegtest`, see
[BUILD.md](BUILD.md#ci).

## Two ways to open it in a browser

The app cannot be started in a cloud session, so the interface is opened
in Chromium, through Playwright, with something answering the calls it
makes to the Go side.

| | Answers the calls | Good for |
| --- | --- | --- |
| the preview, `frontend/preview/vite.config.ts` | `wails-stub.ts`, a stand-in written in TypeScript, and for the picture and the sound the Go side's own routes, `TestHarnessFrames` | the layout, a picture of any state, states the engine is slow to reach |
| the bridge, `frontend/preview/bridge.config.ts` | the real Go side, `cmd/framefairy-app/bridge_test.go` | what the app does: the words, the captions, the clips, undo |

The preview is quick and can be made to show anything, but it is a second
engine, and it drifts from the first. It laid captions out eight words at
a time and knew nothing of pauses, so a caption breaking where a removed
word had been could not be seen in it at all. Anything about what the
engine answers is tried against the bridge.

The frame queue the video preview plays from, `frontend/src/lib/frames/`,
has a third page, the preview's `preview/frames/`: one canvas playing an
episode made for it, whose picture carries its frame number and whose
sound can be laid over the episode's own sample by sample.
`frontend/preview/frames/cuts.mjs` was written to play a clip with cuts
on it and check every frame drawn and every sample heard, and shows no
picture today, see below. The preview's own episode is made the same way,
so the same is measured in the workspace. What it
measures and how to read it is in the interface skill, under Playback and
The frame queue.

Adding a video runs against the bridge: a walk asks `/pick` for a test
video, the bridge has the Add button's box hand it over, and the walk
clicks Add, `walks/searching.mjs`. The probes `frames/added.mjs` and `frames/cuts.mjs`
are older, from before the bridge could add a video. No make target or
workflow runs them, and they have shown no picture since plan row 2.156,
see #613.

## The bridge

The bridge serves the interface with the service the app runs, over a desk
from the path tests in `driver_test.go`: the real queue, engine, words,
waveform and ffmpeg, with stand-ins only for the two models. The speech
stand-in says a few German sentences over and over, with a pause after
each, so a word on screen twice can be told from the words around it. The
language model stand-in finds one clip. The episode is two minutes at
five frames a second, VP9 and Opus in an MP4, because the video preview
reads MP4 and the Chromium Playwright brings has no H.264. Its picture
says which frame it is, the recipe of the harness's own episodes: a thin
strip along the top is the frame number in ten bars, and below it is a
grey that grows lighter. Thin, so the face finder sees one picture that
barely changes, and the crop the search finds stands in the middle.

A video picked with a rate is filmed at it, 29.97 fps or any other, for
what a render does with frames that do not start on a whole millisecond.
A short is a narrow part of the picture made larger, which would cut the
bars away, so its frame number is eleven bands one above the other across
the whole width, the highest bit at the top, four pixels each of 180,
light for one and dark for nought, with the grey below them. Light and
dark are only 100 apart, so the bands change little of the picture. Its
sound is a steady tone. `shortFrames` and
`shortSound` in `walks/bridge.mjs` read a short of it back from disk.
With a timing its frames keep their numbers but not their times: `uneven`
is a phone's frames, each 0, 8 or 16 ms late in ticks of 1/600 s, and
`late` a picture that starts after its sound, by an edit list, on a
clock of 1/90000 s so that ffprobe reads it as a true 0.25 s after it,
7.49 frames at 29.97 a second. On the rate's own clock it came out a whole
7 frames, 0.233 s, which is on the grid of the file's start, so frames
counted from the file and from the picture were the same frames there and
a mistake between the two could not be seen.

A video filmed by two cameras, which `/pick` makes with a switch, has the
same strip, and below it each camera looks at a chequerboard on its own
side of the picture, the first on the left against dark grey and the
second on the right against light red. The search frames the clip in
one piece with one crop, so a camera whose subject lies outside that
crop shows its plain backdrop through it, which the video preview and a
short both show at a glance, and each camera looks the same through it
wherever it is in the clip. A search before #396 found the switch, parted
the clip there and framed each camera on its own subject. Its sound is a
steady tone in plain samples rather than Opus: a render seeked into each
piece, and the first 80 ms of Opus after a seek were decoded quiet, a dip
of the file's own at every piece. A render now reads the sound of each
piece from a fifth of a second before it, `soundLead`, see 2.128 in
[GUI-PLAN.md](GUI-PLAN.md) and [ENGINE.md](ENGINE.md), so Opus would do
here too. The walk has not been changed back.

In the browser, `wails-bridge.ts` takes the place of the Wails runtime: a
call is a POST to `/call`, and what the Go side tells the interface comes
as server-sent events on `/events`. `window.__calls` lists every call and
how it ended, and `window.__menu("undo")` and `window.__menu("redo")` are
the menu bar, which a browser does not have. A page loaded again, as after
a restart, starts its list from nought, and the watch in
`walks/rules.mjs` knows it by a mark it left on the page that is gone, so
it reads the new list from its first call. It once read on from the old
count, and a call that failed just after a restart went unchecked.

Calls that reach beyond the work folder are answered by the bridge with
nothing: the network, the keychain, a box from the system, another app.
So nothing a walk presses downloads a model or installs a build. The setup
counts the two stand-ins as installed, so the app opens on its episodes
rather than on its first-run screen, until a walk asks for that screen
with `/fresh`.

The Add button asks the system for files, and the system's box is the
bridge: it hands over what a walk picked, or nothing, the way the box
answers Cancel. Besides the interface's calls, a walk can ask the bridge,
each with a POST:

| Path | What it does |
| --- | --- |
| `/pick?seconds=N` | makes a new video of N seconds, not in the library, for the Add button's box to hand over next. With `&rate=30000/1001` it is filmed at that frame rate, with `&timing=uneven` its frames come at a phone's uneven times with `&timing=late` its picture starts a quarter of a second after its sound, and with `&timing=turned` it is stored lying on its side with a display matrix that stands it up, the way a phone that recorded upright stores it, with `&in=mkv` it is the same video copied into Matroska, the way OBS records, and with `&switch=S` by two cameras that switch S seconds in, see below. With `&box=0` it is only made, not handed to the box, for a walk to drop instead. With `&namesake=1` it has the file name of the bridge's episode, in a folder of its own. With `&size=1080` it is 1920 by 1080, the size a camera makes, and with `&size=2160` 3840 by 2160, with a key frame every two seconds and frames stored after their key frame, and carries no frame number, for `speed.mjs` |
| `/drop` | takes the files in its body, a JSON list of paths, as dropped on the app, the way `WindowFilesDropped` does in the app: adds the videos with `dropFiles` and sends the window `dropped`. A walk does not call it itself: it calls `window._wails.handleDragEnter()`, `window._wails.handleDragOver(x, y)` and `window._wails.handlePlatformFileDrop(files, x, y)`, which `wails-bridge.ts` defines the way the runtime does, so a drop reaches `/drop` only where the page marks a drop target. A walk makes the video to drop with `/pick` and `&box=0`, so Add's box does not hand it over too |
| `/drop-report` | takes the files in its body, a JSON list of paths, as dropped on the report window, the way `WindowFilesDropped` does on it: `bugs.go` attaches the pictures and tells the window. `wails-bridge.ts` sends a drop there when the page is the report window |
| `/report-window` | says how many report windows the app made and brought forward and whether one is open, and with `?close=1` closes it, the way its close button does. The walk opens the report window itself as a second page on `?window=report` |
| `/pick-picture` | makes a PNG of `w` by `h` pixels called `name`, for the paperclip's box to hand over next, or with `&box=0` only makes it, for a walk to drop. With `&kind=text` it is a file of text named like a picture, which the report has to leave out |
| `/report-made` | the names of the files in the last report on the bridge's Desktop, and with `?file=what-happened.txt` what that file in it holds |
| `/pick-sound` | makes a new file of twelve seconds of a tone and no picture, for the Add button's box to hand over next, so a walk sees Add leave it out with the reason |
| `/model?hang=1&fail=0` | the language model holds its answers until the search is stopped, or fails, or with both off answers. Told to answer again, it answers what it held too, the way a slow model answers in the end, so a search that was asked while it held does not find for good and keep every video added after it from being transcribed |
| `/speech?hold`, `/speech?step=N`, `/speech?go` | `hold` stops the speech model hearing, once the pieces it is hearing are heard, so a transcript stays where it is while a walk looks at it, cancels it or presses something during it. `step=N` lets N pieces more be heard while it is held, and a walk waits on the news of the work, not for the step. With `&save` what is heard so far is saved once the next piece is heard: the app saves a long transcription every 8 s of real time, and the bridge saves it part way only then, through `SaveHeard`, which the engine asks the speech model in place of its clock, so a walk sees a transcript saved part way on any machine. `go` lets it hear on by itself. Closing the app with `/reopen` lets every piece through while the app closes, the way a piece being heard ends as it quits, and the speech stays held after. With `&from=0` it says its sentences again from the first word, so a video added next is heard with the words of the bridge's episode and its clip has the same name |
| `/chrome?bar=30&left=12&right=72` | the app's window as macOS lays it out, in points: the title bar's height and where its buttons start and end. It goes through what the Go side does with an answer from macOS and is told to the page the way a resize is. Until it is asked, and again after `/reset`, the title bar is the one every other system draws |
| `/hold?call=Search&gate=shut&times=K&answer=1&fail=why` | with `gate=shut`, the calls of that name wait before they reach the Go side until `gate=open`, every one of them, or the next K with `times`, the way a busy machine delivers them late, so a walk can press something and look while the Go side has not heard of the call yet, and lets it through once it has looked, on any machine. A walk holds a call with `hold` in `walks/bridge.mjs` and lets it go with `letHeld`: the page sets the held call aside, so `settle` waits for everything else meanwhile, and the watch lets go of whatever is still held before it compares the screen with the engine. With `answer=1` the call reaches the Go side at once and only its answer waits for `gate=open`, so the news of what it set going comes before it, `hold(page, name, true)`. With `fail` the next call fails with that reason instead of reaching the Go side, the way a full disk refuses a save. `/reset` opens every gate |
| `/updates?from=main&stored=pr-143&list=main,pr-143` | gives the app's Updates page the app's own updating and Wails' updater, reading a channel list the bridge serves on this machine: a build from the channel `from`, or made on the Mac without it, with `stored` picked by a build before it, and the channels of `list`, each with a newer build that downloads in a moment. Until it is asked, and again after `/reset`, the Updates page's calls are answered with nothing like the rest. Relaunch always is |
| `/fresh`, `/fresh?memory=8`, `/fresh?arrive=speech`, `/fresh?arrive=clip` | makes the bridge's machine one the app never ran on: no model, no video in the app and nothing said yet about how clips are found, on a machine of 32 GB like Tim's, or of the gigabytes `memory` gives, so the setup offers the same models on every machine. A key saved there is kept by the bridge, never the keychain, and counted as there. A walk opens the page again to see the setup. Download there runs as a job like any other, through a stand-in that says it has fetched two fifths and holds there until `arrive` lets that kind of model arrive, so a walk sees both downloads at once for as long as it looks, on any machine. `/reset` makes the machine the bridge's again |
| `/reopen` | closes the app and opens it again, the way quitting and starting it does: the work stops and how it ended is read back. It answers with the jobs as the app left them, once nothing ran any more |
| `/reset` | the app as it was once its first episode was searched: the models answer quickly, every episode a walk added is gone, the work folder and the settings are what they were, the speech stand-in goes on from the word it had come to then, the episode's decoders the walk before left are closed, and the app is opened again on them. So a video a walk adds is heard with the same words, however many walks came before it, and a walk that moves the playhead far starts from no decoder rather than one left where the walk before stopped, which let a drag go on without a key frame, issue #552 |

To open it by hand:

    cd frontend && npx vite build --config preview/bridge.config.ts
    FRAMEFAIRY_BRIDGE=127.0.0.1:8123 go test -run '^TestBridge$' -timeout 0 ./cmd/framefairy-app

It hears and searches the episode first, which takes a few seconds, then
says where it is and serves until it is stopped. Port 0 is any free
port, and the line it prints says which.

## Walks

A walk is a person at the keyboard who never tires and never does the
same thing twice: it makes the gestures of one part of the interface in a
random order, and after every step it checks rules that must hold
whatever was done. The walks are in `frontend/preview/walks/`, one file
for each part, and they run against the bridge.

    SEED=12 STEPS=60 node frontend/preview/walks/words.mjs

`BRIDGE_URL` says where the bridge is, `http://127.0.0.1:8123/` unless
set. `SLOW=4` runs the page four times slower, through Chromium's own CPU
throttle, the way a busy CI runner does, to find a walk that counts on
the machine being quick. It slows every page a walk opens, through
`slowDown` in `walks/bridge.mjs`, the report window too. It does not slow
the browser's calls from Node, so a walk that times a gesture from Node
is found by putting a wait between the calls instead. The bridge puts the
episode back the way it was first searched before every walk,
`POST /reset`, so a walk starts from the same place every time.

### How a walk picks its steps

Each gesture is the real key or click, pressed in Chromium, with a test
of when it can be made and a weight for how often it comes up among those
that can. A seed decides every choice, so the same seed walks the same
way step for step: a walk that breaks a rule prints its seed and its
steps, and walking that seed again shows it happen. Without `SEED` a walk
picks one and prints it. `VERBOSE=1` prints the caption box after every
step, with the open word in square brackets and the framed one in angle
brackets.

### `words.mjs`: the words of a clip

The gestures: Shift and an arrow, a click on a word, Enter on the framed
word, typing a word after or before what a word reads, replacing it,
clearing it, Enter to save, Escape, delete on the framed word, Undo,
Redo, and playing for a moment. The words typed are some of the
episode's own, the words the walk has removed above all, so it types
them back in, and some it never says. Each Shift and arrow waits until
the video preview shows the frame of the word it stepped to before the
next is pressed, once the app has settled on the press,
`shownOnPlayhead` in `walks/bridge.mjs`. A play goes on until the
playhead has moved as far into the episode as the walk asked, or the
clip has ended, `playFor`, which pauses in the frame it sees that, and
says so when the space bar played nothing. Neither waits on the clock,
so a busy machine takes longer over a step, never another step.

The rules, after every step, once the app has settled, `settle`: no
call is on its way, no timer of the app's is set to fire in a moment and
nothing on the page moves for a moment, see below:

| Rule | What it would have caught |
| --- | --- |
| No call failed and the page threw nothing, after a restart too | a call that failed just after a restart, which went unchecked while the count of calls carried on from the page before |
| The caption box shows one of the engine's captions exactly, the same words in the same order at the same moments, whenever no word is open | a word typed in beside another shown twice, "weil ein \| ein" |
| Enter opens the word in the frame | Enter opening the word at the playhead instead, found by the first walks |
| One word at most wears the frame, and one at most is open | |
| An edit stays where it was made: every word more than three seconds of the episode from the word corrected keeps its text and its time | removing a word typed in beside another taking the other with it |
| Taking a word out changes no other caption: every caption still there appears and goes when it did, the one the word was in only draws in at the edge the word was at, and only one whose words were all removed may go, leaving its time empty | the caption that broke where a removed word had been, the next caption's first word pulled up into the room a removed word left, and the caption before a removed caption staying up through its time, found by Tim |
| A caption removed whole was there and is gone, its words are held to the two rules above, from its first word to its last | |
| Moving a caption edge moves that edge and nothing else: its other edge stays, every other caption appears and goes when it did, and every word outside the caption is as it was | the caption before growing into the room a left edge was dragged in from, found by Tim |
| Undo puts back the engine's clip from before the step it takes back, its captions, how they look, and its pieces with their crops, and Redo the clip from after it | a crop or a font that undo did not put back, which the rule could not see until it compared them too, issue #248 |
| A picture that stands still, paused or cued, was decoded at the size of the video preview's canvas, no larger than the file's own, once the frame queue says the picture stands, `standing` in `lib/frames/queue.ts`, `stillBlurred` in `rules.mjs` | the first frame of every workspace decoded at the 300 by 150 of a canvas not laid out yet, blurred until a play, found by Tim, plan row 2.187 |

The engine's captions are asked for directly, `Captions` through
`/call`, and are what the screen is compared with: the walk knows
nothing of what a correction should do. It only knows what must not
happen, which is what keeps it from being a third engine.

### `timeline.mjs`: the clip timeline

The gestures: dragging an edge of the clip, with shift or without,
double-clicking in the clip, which cuts a part out, double-clicking a
cut, which puts it back, dragging an edge of a cut, double-clicking an
edge of the clip, which puts it back where the clip was found, a click
on the clip timeline, Undo, Redo, and playing for a moment. Every
gesture lands where a hand would, high on the track, over the waveform
rather than the captions' band.

Every walk also checks, after every step, that the clip timeline shows
the clip the engine has: where it starts and ends and every cut, read
off the handles' `aria-valuenow`, to a millisecond. The engine's state a
walk compares before and after a step, for Undo and Redo, is the clip's
pieces with their crops, keyframes and turns as well as its captions and
how they look. A play goes as far into the clip as the
walk asked, by the playhead, `playFor`, the same as in `words.mjs`.

A walk is a script in the folder that walks with `walk.mjs`, which runs
the loop: look, pick a gesture that can be made, make it, check the
rules. `TestWalks` runs every one it finds, for the same seeds.

### `playback.mjs`: playing a clip

The gestures: the space bar, playing for a moment and pausing, playing a
clip to its end from a moment before its last cut, or before its end
when it has none, a drag of the clip's start that leaves room before it,
playing the episode from before the clip on into it, half the time with
Shift and the right arrow or the right arrow alone pressed on the way to
skip ahead, a double-click that
cuts a part out, one that puts a cut back, a click on the clip timeline,
and Shift and an arrow. While a clip
plays, the walk records every frame the video preview puts on screen, by
the frame number read back from the bars on its canvas on every animation
frame, not by the clock and not by what the frame queue says it drew.
Where the playhead is, it reads off `data-playhead` on `.screen`, and
whether it plays off the play button, Play or Pause. It waits on nothing
but those: a play of a moment goes on until the playhead has moved that
far into the episode, `playFor`, a play to the end and a play into the
clip until the playhead is past where they aim or the play has stopped,
`playUntil` in `walks/bridge.mjs`, which `playFor` is built on, and a
skip on the way is pressed each time the play has gone on 0.3 s of the
episode from where the last one landed. A play that goes on past its end
ends the wait once the playhead has gone on by the whole clip and a
second more, counted in the episode's time, and breaks a rule. A play
that stalls moves nothing to wake the wait, so the timeout of make walks
ends it and prints the step. Paused, the picture is read before and after the app has
settled. The rules, besides every walk's:

| Rule | What it would have caught |
| --- | --- |
| Every frame put on screen while a clip plays is in one of its pieces, a frame of give either side | the part just cut played when the space bar was pressed with the playhead in it |
| The space bar plays: a frame comes, or the playhead moves | a press of the space bar lost while the file was still being read |
| Paused, the picture stays where it was paused: the playhead stays, and the frame on screen is the one that holds it, or at the clip's end the one that ends on it | |
| A clip played to its end stops at its end, within half a second of the playhead reaching it, half the time with L pressed on the way and again a moment before the end. The playback walk does it first on every run, once with L and once without, through `first` in `walk.mjs`, so it is checked on every push whatever its seed draws | the clip's start heard for seconds after the play stood at its end, loop switched off near the end, found by Tim |
| On every animation frame of a play, on the video the video preview lays nothing over the picture, no crop frame, no shade, no captions, and on the clip a frame or more inside a piece it shows the crop frame, and the captions wherever the engine has a caption, `overlaid` in `rules.mjs` | the crop frame and the captions staying up while the episode played on through the clip, found by Tim |
| The episode played from before the clip goes on into it, and stays on the video on every animation frame, skips and all | the clip lighting up the moment Shift and the right arrow skipped into it, found by Tim |

`make walks` also walks it on an episode whose picture is HEVC in 10-bit
colour, `HEVC=1`, added with Add and searched first. The Chromium a walk
drives cannot decode HEVC at all, so every frame of it comes from the Go
side, see `lib/frames/app.ts`, and the walk fails at its start when the
picture did not come that way. The sequence "a picture the browser's
decoder fails on comes from the Go side" makes Chromium's decoder say yes
and then fail on the first frame, the way WebKit's does with Tim's
`start.mp4`. The sequence "the picture comes back after the playhead
reached the end of the episode" drags the clip's end to the end of the
episode, plays to it, and then clicks four places along the clip
timeline, each of which has to show its frame. With the episode's
decoder from before 2.166 in [GUI-PLAN.md](GUI-PLAN.md) the video
preview stays black from the end on, and it fails at the first click.
Before the clicks it checks that the playhead stopped at the end and not
past it, and that the clip timeline and the range picker both draw it
there, where it can be dragged again, 2.172.

**The layout under a dragged edge.** The sequence "the app dragged a pixel
at a time moves every part one way, the sides equal and set by the width"
drags the app's height from 1000 to 640 and its width from 1700 to 960,
the app's smallest, a pixel a step, and reads every part of the workspace on every
step: the viewer, the picture, the range picker, the clip timeline, the
two columns beside the picture, a settings field and a clip card. Each
must move only one way for the whole drag, stand on a whole pixel and
meet the app's edges, and the two sides beside the picture, measured from
the app's edges, must be the same width, or a pixel apart, and stand
still while only the height changes. A part that
went back and forth is what a person sees shake. It found three that the
eye found first, the range picker sawing two, two and six pixels, the
settings column going 212, 213, 212, and the clip list ending past the
app's edge, and three more on the way: the clip timeline a pixel taller
for one step in three hundred, the middle column growing two pixels every
fifth step, the picture standing 0, 1, 0, 1 pixels into its column, and
its height 539.99999 one step and 540 the next.
Chromium paints a resize in one pass where WebKit on the Mac may not, so
this proves the layout the stylesheet asks for, not how the Mac paints it.

The bridge's episode runs at five frames a second, so a frame of give is
a fifth of a second. Whether a press plays the clip or the episode is
read off the app, the chosen clip dimmed on the clip timeline while the
playhead is on the video, never worked out by the walk: one that decided
it by itself was a second idea of where play starts, and drifted from the
app's the day 2.121 made it a state.

### `searching.mjs`: from a new video to its clips, and a search

The gestures: adding a video with Add, Search, Cancel, Continue, the model
holding its answers, failing or answering again, the speech held or
hearing on by itself, the app closed and opened again, another episode opened, and a
wait. With the speech hearing on by itself and the model answering,
adding a video waits for its first clips. With the speech held the walk
goes on while the video is transcribed, so the next steps land in the
middle of it, and the wait lets the speech hear a piece for each report.
The wait goes on for one to four of the engine's reports of its work, a
job event each, or not at all when nothing runs that would report: no more
jobs run than the bridge's model holds answers for, `/model-holding`,
whatever step a held search is in. It never waits on the clock. The
rules, besides every walk's:

| Rule | |
| --- | --- |
| A new video gets its first clips with no click | while the models answer and the speech hears on by itself, judged once its first search has ended and nothing of the video runs, by the engine's reports. Broken too when nothing of the engine runs, the app has caught up, and the video has no search |
| Cancel shows at once: in the frame after the click the head no longer says Cancel, or a row says Stopping | held to a press of the button while it said Cancel |
| The clip list says what the engine's jobs do: Cancel while a search runs, Continue with "Stopped", "Interrupted" or "Failed. Click Continue" after one that stopped, Search otherwise | the engine is asked, and the screen read once the app has heard everything the engine told it until then and settled, `caughtUp`. Asked again while the engine's work moves on in between, up to 24 looks, and broken the first time it stood still and the two disagree |
| No row speaks of a failure the engine did not have | |
| At rest, as many cards and the count beside Clips as the engine has clips | |
| How work ended stays after a restart: a search the closing cut off is interrupted, one that stopped or failed stays as it was, one that was done leaves nothing | by how the engine left the search, which `/reopen` answers with |

The clip on screen is the card the clip list marks as the current one,
not the captions the interface last asked for: an episode just opened
shows its clip before that, and a walk then held it to another episode's.

#### A walk waits on what a rule is about

The screen is compared with the engine at one moment. `caughtUp` in
`walks/bridge.mjs` asks the bridge for a mark, `/mark` in
`bridge_test.go`, which it sends down the events behind everything the
Go side told the interface before it, and waits until the app has heard
it, `__heardMark` in `preview/wails-bridge.ts`, and settled. Every page
hears the events in the order they were sent, so what the engine sent
before the mark is on screen then, however late a busy machine
delivered it. A job's news is sent after the queue is let go of,
`queue.update` in `jobs.go`, so a list of jobs can show a change whose
news the mark overtakes. So `caughtUp` also waits until the app has had
the newest change of each job the walk compares, by the job's `seq`,
`__heardSeqs`, from its news and from the lists of jobs the app asked
for. It
asks for the mark only once the app's own stream of events is open,
`__listening`, so after a reload the mark is not sent to nobody, and the
bridge prints any event it drops for a page that did not keep up. How
many events of each name the app heard is `__heard`, which says whether
anything moved on while the walk looked.

A walk prints every step as it begins, with its seed, and a step that
throws ends the walk as a broken rule with its seed and steps, see
`walk.mjs`. Every wait these walks have is a promise the page settles
when the thing it waits for happens: `nextNews` when the news comes,
`caughtUp` when the mark and the job's change come, `playFor` when the
playhead the video preview says has gone far enough, `sidebarStill`
when the sidebar's slide ends, and `fromSidebar` when the video picked
has taken its place. None has a deadline, so news that never comes is a
walk that hangs, named by its last step.

A search goes on while the walk looks. The model the walk set to fail
fails a search the moment it comes to finding, and one that answers ends
it in well under a second, with no call of the interface's on its way. So
the app can change after it has settled, between the walk looking and
the walk pressing, and between one thing the walk reads and the next.
Nothing a rule judges is read before that moment and held to what is
read after it:

- The head button is read as it is pressed, in the same frame as the
  click, and Cancel shows at once is held to a press of Cancel. The walk
  that saw Cancel and pressed a button that said Continue by then broke
  that rule two runs in eighty.
- How a search ended before a restart is the engine's word, the jobs as
  the app left them once no work ran, which `/reopen` answers with. The
  head the walk saw before it closed the app said Cancel for a search
  that was done by the time the app closed.
- The episode on screen is read again each time the clip list is asked,
  so a video that opens a moment after Add is not held to the search of
  the episode before it.
- The clip on screen is read before and after what the clip timeline and
  the caption box show, and again if it changed in between. With no clip
  of the engine's chosen, a clip still on its way among them, the clip
  rules have nothing to hold the screen to. The clip of the episode open
  before was kept instead, and the clip timeline of a clip just found was
  held to it.

None of the four was wrong in the app, and no rule is looser for it. Each
was measured: `searching.mjs` for seeds 1 to 10 against the bridge, side
by side as `make walks` runs them, with every rule's broken state written
down and the engine's jobs beside it.

### `sequences.mjs`: the cases found by hand

    BRIDGE_URL=http://127.0.0.1:8123/ node frontend/preview/walks/sequences.mjs [part of a name]

`SEQUENCE` walks the one sequence of that name and no other, and
`--names` lists them all, one name a line, which is how `make walks`
hands them out, see Where they run. It fails when two sequences or two
steps have one name, because a step is found by its name and only one
of the two would ever be walked.

The sequences and their steps are kept in a folder for each part of the
app, beside `sequences.mjs`, which walks them:

| Folder | What its sequences are about |
| --- | --- |
| `captions/` | the caption box and the captions drawn over the video preview: words, blocks, pills, fonts and styles |
| `timeline/` | the clip timeline: edges, cuts, trims, marks, keyframes and the playhead under the hand |
| `playback/` | the video preview: playing, looping, the frames it shows and the crop frame |
| `clips/` | the clip list: cards, clips made, removed and put back, and the shorts rendered from them |
| `search/` | searching: the rows of a search, the window, the language model and the speech model |
| `videos/` | the videos in the sidebar: added, dropped, labelled and gone from the disk |
| `report/` | Report a Problem, the toasts and dictation |
| `app/` | the app as a whole: its size, its menus, the settings and the Updates page |
| `common/` | no sequences, the steps every folder takes: keys, buttons, fields, undo and redo, marks |

Each folder has its `sequences.mjs`, the sequences as data, and its
`steps.mjs`, every step said above it, with the helpers only those steps
use. A step is a function of the walk so far, its argument, what the
caption box showed before it and its own name. The walk is what
`walkSequence` holds, the page, the marks and what went wrong so far,
read and written by name. A sequence of any folder can take any step,
and a new sequence goes in the folder of the part of the app it is
about.

A sequence is a named list of steps and of what has to come of them,
written as data, so a case found by hand is kept as it was found. The steps are the walk's gestures by name, `frame` a
word, `click` it, `press` a key, `type`, `undo`, `redo`, `mark` the
engine's clip, its captions, their look and its pieces with their crops, under a label, and on the clip timeline
`cut at` a share of the clip, `join` a cut, `swipe after click`, a
click a share of the way along the clip and three swipes from there
without moving the hand, each of which has to move the clip, `trim` an edge by some
pixels, `reset` an edge and click the caption `block` holding a word,
drag an edge of the caption holding a word `edge in` over the middle of
its word at that edge, `edge out` back to where it was at the mark
`start`, or `edge nudge` a little short of any word, each held a moment
before it is let go and failing when the edge is not saved where it was
drawn under the hand, and `remove edge word`, the first or last word of
that caption removed in the caption box, with `gone last` and `gone
first`, that caption reading as it did at a mark less its last or first
word, `drawn in`, the captions reading as they did at a mark and that
caption drawn in at its first or last edge from where it was at `start`,
`others`, every other caption appearing and going when it did at a
mark, `edge put back`, a double-click on an edge of the caption holding a
word, and `count pops` and `pops`, how often a caption block popped
since. What has to come of them: `box`, what
the caption box reads, `open`, which word is open, `same`, the engine's
clip as it was at a mark, `differs from`, the clip no longer as it was at a
mark, so a step meant to change it did, `spans`, every caption
appearing and going when it did at a mark, `fewer`, so many words fewer
than at a mark and no caption fewer, `gone`, no caption holding a
word, `blocks`, a block on the clip timeline for every caption the engine
has, and `cuts`, how many cuts the clip timeline shows. For finding clips: `model` holds, fails or answers,
`add` a video, press the clip list's `head` button and `restart` the app,
and then `wait for` the head to say a word, a `row` to say something, and
how many `cards` the list holds. For a render: `add` a video filmed at a
frame rate, press `render` and wait for the short, and `short`, the short
read back from disk holding exactly the frames of the clip's pieces, each
the right frame of the episode by its bands, with its sound as long as
its picture and quiet at each cut for no longer than the render's fade,
and `rendered`, the clip saying its short is in its episode's folder
inside the folder the settings name for shorts, which the bridge sets,
with the file there, the app
serving it, Render saying Render again and Show in folder there.
For shorts: `thumbnail`, which presses T and waits for the clip to have
one more thumbnail, `keep` the clip on screen with its short, its
pictures and the shorts in its folder, `words again`, which makes the
speech stand-in say its sentences from the first word so the next
video's clip has the same name, and `add namesake`, a video of the
bridge's episode's file name from another folder. Then `apart`, the
clip on screen with a short of its own, the kept short the same file and
still its clip's, and its pictures all there and untouched, `in place`,
the clip's short rendered again at the path kept with no number, written
again, and no other short in its folder, and `pictured`, the short's
pictures beside it in its episode's folder, one for each thumbnail of
the clip and no more, and `by its words`, the short named after the
clip's words alone, `<slug>.mp4`, with nothing of its id.
`captions like the short`, after `render`, makes the video preview as
large as the short, steps to the start and the middle of a dozen words and
holds what the canvas shows to that frame of the short, and to it rather
than the frame before or after, see [CAPTIONS.md](CAPTIONS.md#the-tests).
For the settings: `settings` opens them from the sidebar and `episode`
the episode again, `pick model` opens the list of what finds clips and
picks another model if it opens, `remove model` presses the bin of the
language model or of the speech model, and `speech` holds the speech
model. Then `refused`, the card shook and no list and no box opened
and the settings name the models they named, `asks`, the box asking
whether to remove opened, `changed`, the settings name another model,
and `hearing`, a search still hears. For Report a Problem: `bugs`
clicks Bugs in the sidebar and goes on in the report window, a second
page of the browser, and `windows` and `close report` say how many the
app made and close it. `attach` presses the paperclip with a picture
waiting in the box, `drop pictures` and `paste picture` bring pictures
in the other two ways, `drop recording` drops a screen recording the
way a Mac makes one and `recording tile` reads its length off its tile
and sees its still loaded, `pictures` counts the tiles, `remove picture` presses a tile's bin, `put back`
its Put it back, and `removed goes` waits for the grid to close over it.
`report` presses Report, sees the row keep its width, and reads the zip
for the pictures, and the recordings when it is given two numbers, and
the field for nothing, and `report lines up`
measures the row and the tiles on whole pixels. Why something was left
out, in the report window or the app's own, is a toast: `toast` reads
it, or sees none with "", and sees that nothing under it moved,
`toast goes` sees it still there after nine seconds with the pointer
away, since it has the ten a removed row has, and then go by itself, `toast held` keeps the pointer on it past its time, sees it
stay and then go once the pointer leaves, `stacked` sees a toast for
each of two files dropped one after the other, the newer on top, and
`toast closed` presses the cross of the top one. `write` types into its
field and puts the cursor so many characters in, `dictate` presses
Dictate, sees it say Stop in the same frame, the words appear grey while
it listens and the microphone hear the hum, then ends it with Stop,
Enter or Escape, or with `ends` lets it stop by itself and say why, or
with `start` only starts it, with `hearing` waits for more to be said
and settled, so it is kept however the recording ends, and with
`settled` waits for the words before the pause, `microphone moves`
watches the microphone while it listens, from the hum through its pause
until the hum comes back: it grows with the voice, keeps its box and its
middle, falls back no faster than a level meter's needle and is drawn
white. It runs in one walk, not in every `dictate`, because each watch
takes seconds of the hum, see #543. `click heard` clicks
into the first word still being heard and sees it settled with the
cursor in it, with what the Go side tells the window held back from
reading the word to the click, `window.__pauseEvents` in
`wails-bridge.ts`, since the stand-in hears new words each time and the
click would land in other words than the ones it read, `report reads`,
the field reads so, with `*` for whatever the speech model heard, and
`report as before`, it reads what it did then. The bridge's microphone
plays two seconds of a hum, then a pause and a second and a half more,
over and over, and pieces are settled while it listens and there is
more to hear until it stops. It goes on only as the walk asks, never on
the app's timers, #551: a step hands the app a tenth of a second of it,
a reading of how loud it is and a pass of the speech model, through
`listening.beats` in `cmd/framefairy-app/listen.go`, and nothing moves
between steps. A walk asks with `/microphone?until=`, which takes steps
until the app has told the window what it waits for: `hearing` until
words are heard, `said` until more are settled, `settled` until some
are, `ended` until the recording is over and `tenth` for one step, see
`sound` in `cmd/framefairy-app/walksound_test.go`. `dictate` asks for
words before it looks for them, and for something settled before it ends
a recording, since the stand-in hears new words each time and the end
could otherwise write down none. `microphone moves` asks for a tenth a
frame while it watches. With `ends`
the person falls silent once the phrase they are on is said, through
`/microphone?silent=1`. A recording there stops by itself for want of
words as soon as that phrase is said, since the bridge says the quiet is
over then, `listening.still`, rather than after the app's two minutes,
which `TestListeningStopsByItself` proves. It stops after a minute at
most, so a walk that waits for words fails rather than hangs. `field` clicks a field by its selector
and types, and `field reads` says what it holds. `escape lets go` clicks a
control or field, presses Escape and sees nothing left open, nothing
holding the keyboard and no ring. For a clip of two cameras: `add cameras`, a video filmed by two cameras,
`cut switch`, a double-click on the switch, `trim past` the switch,
which drags an edge two seconds beyond it, and `look` at every piece,
and at each camera of a piece the switch lies in, which puts the playhead
in its middle with a click on the clip timeline and remembers the crop
frame there and what the picture shows inside it.
Then `pieces`, how many pieces the clip timeline draws, `framed`, the
crop frame on every piece where it stood at a look and showing what it
showed, and `shots`, the short read back from disk with every frame
showing what the crop frame showed on its shot and the sound straight on
where two shots meet. Every step is also checked against the walk's
rules, so a sequence asks for its own result and gets the rest for
nothing.

For the Updates page: `updates` opens it on a build from a channel or
made on the Mac, with a channel an earlier build picked and the channels
on the list, through `/updates`, and `follow` picks a channel. Then
`channel`, what the list says, `choices`, the channels it offers and the
one picked, `status`, the line under the build, `no check`, no button on
that line, and `at once`, the list and that line in the frame after a
pick. `a build made on the Mac follows nothing until a channel is chosen,
and then follows it` is Tim's test of #147 with `make install`. Against
the head of #147 it fails at the first `channel`: the list says Pull
request #143, the pick of a build before it.

Each sequence starts from the bridge's episode as it was first searched,
in a page of its own.

`a model a search finds clips with is not changed or removed under it`
is Tim's test of the settings while a search runs, done the way he did
it. It starts a search the model holds, opens the settings, picks another
model, and presses the bins of the language model and of the speech
model. Each time the card has to shake, no list or box may open and the
settings have to name what they named. Then it cancels the search and
asks the same again, where the box now asks and the pick now takes. `a
model is not changed or removed while a search still hears` does it while
the first search of a video just added is still hearing. Against the
interface before the change both fail at the first pick: the list opens
and nothing shakes. For the settings to stand as they do on a machine
that has searched, the bridge counts a language model as chosen and
installed, and the setup check counts both models and llama-server as
there, since the stand-ins hear and find the clips.

`the episode played through a clip shows only the video, and the clip
its frame and captions` is Tim's test of 2.173. It trims the clip's start
to leave room before it, clicks the clip timeline there and plays six
seconds, on into the clip, and then clicks inside the clip and plays four.
Through the first play the playhead has to be on the video on every
animation frame, two seconds or more of it inside the clip, with nothing
over the picture. Through the second it has to be on the clip, with the
crop frame on every frame and the captions on every frame the engine has
a caption for. Against main it fails at `only the video`: on the video at
6.8 s, the crop frame and the shade are still there, and the captions with
them.
`the episode played through a clip with cuts shows only the video, and
the clip its frame and captions` does the same after two cuts, at a
quarter and at a little over half of the clip, with plays long enough to
cross them, `across cuts`. The episode's play has to stand in two pieces
or more and go through a part cut out between them, on the video and with
nothing over the picture the whole way. The clip's play has to jump a cut
and keep its crop frame and its captions in the piece after it. Against
main it fails the same way, at 6.8 s.

`loop switched off near the end stops the play at the clip's end` is
Tim's test of 2.183. It records every sample the sound card is handed,
plays the clip, presses L to loop it, and two seconds before the clip's
end presses L five times quickly, the loop off after the last, once the
play has worked out the next time through. No press may start the play
over. The bridge's video sounds a steady tone, so the sound may not bend
by more than 3 % of its height from one sample to the next, which a stop
or a restart does, and nothing may be heard after the clip's end. The
play has to stop within 400 ms of the playhead reaching the end, and no
frame from before the clip's last second may be drawn after that. Against
main it fails with the play going on for 9 s with the playhead at the
end, and against the first fix, which started the play over on each
press, with three restarts, heard as a click at each.

`shift and the arrows skip ahead through a clip on the video, and walk its
words on the clip` is Tim's test of the skip. With the clip's start
trimmed and one cut in it, it plays eight seconds from before the clip,
pressing Shift and the right arrow every 400 ms. The playhead has to go a
second or more further than the play alone takes it, and stay on the
video with nothing over the picture on every animation frame. Then it
plays four seconds from inside the clip with the same presses, which walk
the clip's words and stay on the clip, its crop frame and its captions
shown. Before the fix it fails at `only the video`, the playhead on the
clip at 7.96 s.

`a clip made with I while the episode plays is not dimmed` is Tim's test
of 2.212. With the clip's start trimmed, it plays from before the clip,
presses I a second and a half in, and reads the clip timeline on every
animation frame until the clip made is written and chosen, two seconds
more, and a pause. From the frame the new card is chosen on, on its way
and written, playing and paused, the clip may not be dimmed. The same
with O, and with I while the video is paused, where the playhead is put
on the clip's start. Against main all three fail at the first frame of
the clip on its way. A first fix still left one frame dimmed with O, as
the card on its way became the written one, which is why every frame is
read.

`a clip filmed by two cameras has one crop, through a cut, a trim and a
drag` is Tim's decision on #396 done the way he would try it. It adds a
video of 30 seconds whose camera switches at 12, each camera on a subject
of its own, and its search has to find one clip in one piece. It looks at
the crop frame in the middle of each camera, which has to stand in one
place on both. It double-clicks across the switch to cut it and
double-clicks the cut to put it back, drags the clip's end to two seconds
before the switch and double-clicks the end to put it back, and after
each the clip timeline has to draw one piece and the crop frame has to
stand where it stood on both cameras, with the picture inside it what it
was. Then it drags the crop frame to the left on the first camera, and
the crop frame has to stand there on the second camera too. Last it
presses Render and reads the short back from disk: every frame has to
look like what the crop frame showed on its camera, by its colour and its
detail. Against the engine before #396 it would fail at the first step,
which is inferred and was not run: the search parted the clip at the
switch, and the clip timeline drew two pieces.

`a short rendered into the folder for shorts is known as rendered`
presses Render, asks `rendered`, closes and opens the app, and asks it
again. Against main's engine it fails at the first `rendered`: the clip
says it has no short, because only the episode's own `out/` was looked
in. Until then `short` and `shots` found the short in that folder by hand.

`two episodes never overwrite each other's shorts` renders the bridge's
episode, adds a video heard with the same words, so its clip has the same
name, and renders that. Against main's engine it fails at `apart`: both
episodes' shorts are the same file in the folder for shorts, so the
second wrote over the first. `two episodes of one file name number their
shorts` does the same with a video of the same file name, whose short
has to be called as the first with ` 2` after it, in the same folder.
Its first clip has two thumbnails and the second none, so a render of
the second that took the pictures by the clip's name would take the
first's away. `render again writes the short over in place, with its
pictures beside it` gives the clip two thumbnails, renders, renders
again and asks `in place` and `pictured`. With a render again that picks
a new number it fails at `in place`, and with pictures named and cleared
by the clip's name rather than the short's it fails the namesake
sequence at `apart`. See 2.133 in [GUI-PLAN.md](GUI-PLAN.md).

#### What depends on how fast the machine is

A sequence that reads the screen on every frame, `watchPills` and
`watchEdge`, gives each watch its own switch, so a watch that was
stopped stops at its next frame even when the next one has begun. It
read the next watch's switch before and went on, so every step of `a
step onto a word pops it over the paused frame` added a reading of the
canvas to every frame, and on a busy machine the screen drew a bounce on
fewer frames each step, issues #328 and #329.

How many frames the screen draws, and how soon the video preview comes
to a frame, is the machine's. So a step that works on the frame shown,
C, R and a play from the playhead, waits for that frame first,
`shownOnPlayhead`, issue #320. It waits on the page, never on the clock:
a `MutationObserver` on the video preview hears `data-playhead` and
`data-shown` change. With captions it also waits for the frame to be
drawn with its own captions over it, `data-captioned`, and says so when
a layer of captions is given up on, `data-captions-lost`, counted from
before the click. A step that reads the caption box waits for that, since
the box shows what was drawn on the frame on screen, issue #359. A click
on a caption block after a trim waits until `data-answered` on the clip
timeline's box names the track the trim saved, and fails when
`data-framed` does not, since until then a click goes by the words' own
times, issue #290. A frame that never comes is ended by the `-timeout`
of `make walks`, `WALKTIMEOUT`, 30 minutes by hand and 8 in CI, where a
share takes about two and a half, issue #461. A minute before it,
`TestWalks` stops every walk still running and fails it with what it
printed, and every walk writes `walking`, `walked` or `failed` with its
name to stderr as it starts and ends, so a walk that hung is named,
issue #443. And a check counts what the app drew against the frames the
screen drew, not against a number: a bounce is drawn on every frame the
screen drew while it lasted, and the crop frame of a glide stands in a
place of its own on every frame the play showed, so frames the screen
did not draw make no jump, issue #442.

Where how many frames the screen draws decides what a walk can see, the
walk gives the app its frames or its time instead. The bounce after a
word step is drawn on frames the walk hands the caption layers, one a
sixtieth of a second after the other, `frames` on `CaptionLayers` in
`lib/frames/captions.ts`, and the pills are read off the screen after
each: its 220 ms are 15 frames on any machine, and the one nearest its
peak is within 0.2 % of it. On the screen's own frames a busy machine
drew two and missed the peak, issues #328, #329 and #460. A frame whose
captions a walk holds back waits for them as long as they take, by a
wait the walk hands the frame queue, `dressWait` in
`lib/frames/queue.ts`, where on the browser's clock the walk's own steps
used up the 600 ms it waits, issue #468. And a glide read while it plays
goes on until the video preview has shown ten frames or come to the next
keyframe, not for two seconds, in which a busy machine showed two, issue
#464.

Every walk plays on the walk's clock, `preview/clock.ts`. The page the
walks run on hands it to every frame queue it makes, see `FrameQueue.clock`
in `lib/frames/queue.ts`, whose own clock is the sound card's. The app
never builds `preview/`, so it holds no walk's clock and no switch for
one. It moves on to the next frame only once
the one before is up, and holds the sound card while a frame is still
being decoded, so every frame of a play is shown on any machine and the
sound stays with the picture. By the sound card's clock, the app's, a busy machine showed two
frames of a glide's nine, issue #534, and a play of the captions showed
fewer frames, each longer, issue #338. Each of those walks had a clock
of its own, until a new walk that played was the next to fail, issue
#588. A sequence that checks what a play does on the sound's own time
plays on the sound card's clock instead, the way the app does: how much
sound the sound card is handed, the loop switched off as the sound comes
to the clip's end, and a playhead that never stands still while a hand
drags. It asks for it by one of those steps, `soundCard` in
`common/steps.mjs`, the way a sequence asks for motion, see Where they
run below, and a new step of that kind goes in there. `smooth.mjs` asks
`open` for it by name, `clock: "sound"`, which the page reads from
`window.__playClock` before the app starts. A walk can also hold the sound
card a moment by hand, `holdSound`, as the end dragged back past the
playing playhead does while the hand drags, issue #583. The fold of a removed row is held as Svelte makes it and stepped
through by hand, in twenty steps, and at each the card under the row
has to have risen by as much of the row and the space after it as the
row has folded. That the row leaves the list only once its fold has
finished is heard from the fold's own animation, which says so before
Svelte takes the row out. On the screen's own frames a busy machine drew
none of the fold's 200 ms, issues #442 and #469.

Where the machine itself spoils what a walk checks, the app says where,
and the walk sets aside only that. A page too busy to hand the sound
card its sound in time leaves it playing nothing until the sound comes,
which starts and ends with a click. The frame queue counts every
stretch it scheduled late, with the sound card's frames it left quiet,
`lateSoundAt` in its stats. The walk of the loop switched off near the
end takes a click for the machine's only at either end of a silence it
heard, samples of nothing at all, that lies where the queue says it was
late, and says how many it set aside. Any other click fails it. So that
the app's own mistakes cannot hide there, the queue also counts sound
it took off the sound card before it had played, `soundThrown`, and
sound it handed over twice, `soundTwice`, and the presses of L must
leave both at nought, issue #466.

### `speed.mjs`: how quickly the video preview follows the keys

`speed.mjs` is not a walk. It checks no rule and fails on no number, so
`make walks` does not run it. It is run by hand against a bridge, before
and after a change to the frame queue, and prints a table:

    BRIDGE_URL=http://127.0.0.1:8123/ node frontend/preview/walks/speed.mjs

It adds a video of a minute at 1920 by 1080 and 25 frames a second,
through `/pick` with `&size=1080`, picks the clip its search finds and
walks it the way a person does: Shift and an arrow from word to word, an
arrow a frame at a time. For each press it times, in the page, how long
from the key going down until the video preview shows the frame that
holds the playhead, and until that frame has its own captions, and how
the frames of each kind of press came: from a stream opened for them,
read on in one already open, or from the frames kept. Each round ends
with the playhead dragged over the clip timeline and back, a step every
frame of the page, and the last line says how many frames the video
preview was behind the playhead while it moved, and how long after
letting go the frame under it showed, with how many key frames were put
up on the way. A press whose frame never shows counts once the frame
queue has done all the press asked of it, `standing` in
`lib/frames/queue.ts`, never after a time, and a press that left the
playhead where it was ends the run. `REST_FRAMES` is the pause between
presses, in frames of the page, 15 unless set, a quarter of a second on
a screen of 60 Hz. It is the one pause on the clock made on purpose, a
person's pace, which is part of what is measured. `ROUNDS` is how many
times the clip is walked, each on a page opened afresh so no round finds
the frames the one before kept, 5 unless set, `SCALE` the device pixels
of a CSS pixel, 2 unless set, a Mac's screen, `CODEC=hevc` makes the
video HEVC with 10-bit colour, and `SIZE=2160` makes it 3840 by 2160,
the way a 4K camera records. The window is 1728 by 1080, a 16 inch
MacBook Pro's screen, unless `VIEWPORT` sets another, `2560x1440` for
frames about the size of Tim's. The line under the table also counts the
frames read ahead, and those of them let go of from the kept frames
before the hand asked for them. Plan row 2.269 in
[GUI-PLAN.md](GUI-PLAN.md) has the numbers it measured.

### `smooth.mjs`: how evenly the video preview plays

`smooth.mjs` is not a walk either, and is run by hand the same way,
before and after a change to how a play is drawn:

    BRIDGE_URL=http://127.0.0.1:8123/ node frontend/preview/walks/smooth.mjs

It adds the same video of a minute at 1920 by 1080, plays it to the end
`ROUNDS` times, 3 unless set, and prints the line each play ends with in
the app's log, see `lib/frames/even.ts`: the refreshes missed, how long
each frame stood, and where the time of an animation frame went, typical,
most and before a missed refresh. `GPU=1` draws on the WebGPU canvas the
app draws on rather than the 2D one the walks read frames back from, but
headless Chromium in the cloud draws WebGPU on the processor, at about
25 frames a second, so only a Mac's numbers say how the app plays.
`VIEWPORT` and `SCALE` are as for `speed.mjs`.

### Finding why a walk failed

A walk that fails now and then has met an order of events it did not
expect, see section 7 of the testing skill. Here is where each of its
tools is in this repository, in the order to use them. Loading the
machine with busy loops beside a walk comes last, if at all: it took
many runs to show anything, it never showed #358, and the walks beside
it hang, #508.

1. **The trace.** Every walk and sequence records a Playwright trace of
   every page it opens, into `WALKTRACES`, `.build/walk-traces` unless
   set. A walk that passed throws its traces away. One that failed or
   threw keeps them, one zip for each page it had open, the report
   window too, and prints `trace:` with the path under its failure. One
   that hung is stopped by `TestWalks` with an interrupt instead of a
   kill, so it keeps the traces of the step it hung in, see
   `keepTraces` in `walks/bridge.mjs`. Each step is a group in the
   trace, named the way the walk prints it. A trace holds every call of
   the walk with when it began and ended, what the page logged and
   threw. Every request to the Go side is kept beside it, in a file of
   the same name ending in `.requests.txt`, see `hearRequests` in
   `walks/bridge.mjs`, with every line the app logged written whole.
   Playwright records the network into a trace only
   along with the snapshots below, so Node hears the requests instead,
   and nothing of it runs in the page. The
   events the Go side sends come on one request that stays open, so
   they are not in it. A film of
   the screen is in it only for the walks `walks/filmed.mjs` names: it
   made every walks job in CI about a fifth longer when every walk had
   one, measured on #558. `make walks TRACE=film` films every walk for
   that run, and `TRACE=none` records no trace. Playwright's snapshots
   of the page before and after each call no walk records: eight
   sequences took 41 % longer with them in a cloud session, and with
   the film a walks job in CI went over its five minutes. In CI a walks
   job that failed keeps the folder with the run as `walk-traces-0` to
   `walk-traces-3`, one for each share, for 14 days.
   A session finds it with the GitHub tools,
   `list_workflow_run_artifacts` for the run and
   `download_workflow_run_artifact`, which gives a link to the zip. The
   zip itself lies on a host of GitHub's under `blob.core.windows.net`,
   which a cloud session can fetch only when its environment's network
   allows that host, and the log of a job too unless it is read with
   `get_job_logs`. Where it does not, the session says so, and a person
   downloads the zip from the run's page. A person opens a trace at trace.playwright.dev, which reads it in the
   browser and sends it nowhere, or with `npx playwright show-trace`. A
   session with no window reads it unpacked: `trace.trace` holds every
   call of the walk, with what the page logged and threw, one JSON
   object a line, with times in milliseconds from when the trace
   began. The `.requests.txt` beside it counts from the same start, a
   line a request: `  1532  12  ask POST /call main.FrameFairy.Seek
   [3.2]`, `  1540  12  answer 200` or `failed` with why.
   `trace.network` is empty, and `resources/` holds the screenshots of
   a filmed walk, which the Read tool shows.
2. **The order the walk assumes.** What the step waits for, `settle`,
   `caughtUp`, `nextNews`, `pageUntil`, `__standing`, and whether what
   it reads could change after that, or come before the walk listens.
   #538 and #554 read before the step had finished, #556 waited for a
   signal that had gone by.
3. **That one order, on purpose.** Hold the call the walk would race
   with `hold` and `letHeld`, the `/hold` gate of the bridge above, and
   look while it is held. Hold a request the bridge does not gate, a
   frame read, with Playwright's `page.route`, the way #531 showed #358
   with a frame read held back. Hold the app's time with `holdClock`
   and turn it on with `runClock`. Slow the page with `SLOW=4`, the
   browser's own CPU throttle, see Walks above. A walk that fails every
   time this way is a test for its fix.
4. **The orders, tried for you.** Under `make unit` every Go test runs
   with the race detector, and `testing/synctest` holds goroutines and
   timers. For the interface's async code, `lib/frames/app.ts` and the
   frame queue above all, fast-check's scheduler, `fc.scheduler()`, in
   a Vitest test tries the orders in which promises can resolve and
   cuts a failure down to the shortest. fast-check is added to
   `frontend/package.json` with the first test that uses it.
5. **The walk alone, many times,** to count how often it fails before
   and after a fix: `SEQUENCE="<name>"` with `sequences.mjs`, or one
   seed of a walk, against one bridge opened by hand, see The bridge
   above.
6. **The machine under load,** last.

A walk that fails now and then gets an issue of its own, with what was
seen and where, and the trace its failure kept, and the work that was
going on goes on. When the trace shows the order, it is fixed. When what
is missing is what the screen looked like, the walk gets a line in
`walks/filmed.mjs` with the issue's number, pushed to the pull request
whose CI it failed on, and its next failure in CI comes with the film.
The line is the one change a pull request carries that is not its own,
because without it the next failure would show no more than this one.
The pull request that fixes the walk takes its line out again with
`Fixes #N`. `sequences.mjs --names`, which every run of `make walks`
asks first, fails on a line that names no walk or sequence, so a walk
renamed or removed does not leave a line that films nothing.

### No walk retries

No walk retries, see section 1 of the testing skill. A step that read or
did something again until it liked the answer hid an order: the rules
read the clip on screen up to twenty times while a search changed it,
which `settledScreen` now reads in the one moment the app settled.
`holdClock` asked Playwright again when the moment it had read was
already past, and now goes on with the clock Playwright has held by
then. Four kinds of loop stay, because each is a gesture a person makes,
a wait on a signal, or a comparison with a state that moved:

- The steps that aim the playhead inside a frame or under a word scroll
  and click again, at most six and four times, in `playback/steps.mjs`
  and `captions/steps.mjs`, as a hand aims again after a near miss of a
  pixel or two.
- Deleting a video's folder from outside goes on until the folder is
  gone while the app writes into it, as a person deletes again when
  Finder says it is not empty.
- `firstSearch` and the steps that hear a search through read the jobs
  again on each piece of news, which is a wait on a signal.
- `agree` in `searching.mjs` compares the clip list with the engine's
  jobs, and compares again when the engine told the app something new in
  between, up to 24 looks. It is no retry: a disagreement at a state that
  did not move is a broken rule at the first look, and one that stands
  through all 24 looks is a broken rule too. What it looks again for is
  that the engine's work moved on between the two reads, so the
  expectation it compared was of the moment before.

### Where they run

`make walks` runs the sequences and every walk for seeds 1 to `WALKS`, 3
unless set, each walk as long as it says, 60 steps, or 25 for playback,
which plays in real time, or 30 for searching, unless `STEPS` is set, through the Go test `TestWalks`,
which fails with what a sequence or a walk printed. The seeds are the
same every time, so a walk that breaks a rule breaks it again on the
next run. `make walks WALKS=40` looks further.

They run side by side, half as many again as the machine has cores, or
`WALKERS`. Each runs against a bridge of its own, which `TestWalks`
starts as a process of its own on a free port, because a desk sets
`HOME` for the whole process it is in. Every sequence is a run of its
own, the same as a walk, handed to whichever bridge is free next: the
walks first, since each takes most of a minute, and the sequences behind
them, most of them a few seconds long, so no bridge is left with a long
run once the others are done. Each sequence opens the app afresh with
`/reset`, so it walks the same alone as after the one before it.

Every walker hands its sequences to one node of its own, `sequences.mjs
--serve`, which reads the name of the next sequence from its input,
says `walking` in a line of its own as it begins and how it went in
another as it ends, and keeps one Chromium for all of them. Each sequence still opens a page in a context of its own, so it
keeps nothing a sequence before it kept. Starting node, Playwright and
Chromium for every sequence was 1.45 s and 1.2 s of CPU each time, 220
times, issue #526. A sequence that throws ends its node, and the next
sequence starts a new one, so nothing a throw left behind walks on. What
the node said before `walking` belongs to no sequence, so the sequence
it was handed fails with it, and a node that ends between the two lines
fails the sequence it was walking, never the one after.

A sequence asks the system for less motion, the way a person can in the
Mac's settings, unless one of its steps looks at how the app moves: the
caption blocks and the typed word that pop, the bounce over a paused
frame, the word that grows as it is opened, the card that shakes and the
microphone that grows as it hears, `moving` in `common/steps.mjs`. The app
then leaves out what it animates, the info marks' fade and the pops
among it, so the settle waits for none of it. On main those two were
179 s of the 1976 s all runs took one after another. The sequences with
those steps and every random walk keep the motion, so the app is still
walked the way it moves on Tim's Mac. A new step that looks at motion
goes into `moving`, or it passes without ever seeing what it checks, and
`--names` fails on a name in `moving` or `soundCard` that is no step. `make walks
STILL=none` walks every sequence with the motion, and
`walks-nightly.yml` does so on main every night, so a step that looks at
motion and was left out of `moving` fails there.

A video a walk adds is made by ffmpeg the first time it is asked for and
kept, by what it was asked for and by the sum of what makes it, the
source of `walkvideos_test.go`, which holds every function that makes
one, and the version of ffmpeg, see `walkVideos`. Every bridge of a run
keeps them in one folder, `.build/walk-videos` or `WALKVIDEOS`, and CI
keeps each share's folder between runs, saved again whenever a run made
a video more, so a walk that adds the same video again gets a copy. ffmpeg makes the same bytes
from the same arguments, so the copy is the video it would have made.
Made on every add, the 54 videos the sequences added took 100 s, and the
one of ten minutes 9 s every time, issue #527.

Measured on four cores, the cloud's and CI's, with the 3 seeds: the
sequences were once one run, and took twelve minutes on one bridge while
the other three had finished the fifteen walks in four and waited. Handed
out one by one they took six and a half minutes on four bridges. A run
spends most of its time waiting, on a clip that plays in real time and
on the quiet after every step, so four bridges kept the cores busy only
part of the time. Six took five and a half minutes, the cores busy four
fifths of it. The slowest run, a sequence that renders a short and looks
at 122 of its frames in the video preview, takes a little over a minute,
so however many bridges run, the set takes at least that.

CI splits the set over four machines, `make walks WALKSHARD=1/4` to
`4/4`, walks and sequences alike, every run whatever the change, see
[Why the walks run whole](BUILD.md#why-the-walks-run-whole). Five and a half minutes on one machine,
after two of setting up, made a job of seven and a half, and on two
machines each job still took six with a cold build cache. No job may take
over five.

The runs are shared out by how long each took last, kept in
`frontend/preview/walks/walk-times.tsv`, seconds and name on each line.
The longest run goes first, to whichever share has the least so far, and
each share starts its longest runs first, so no bridge is left with a
minute of work after the others are done. Every third run, as it was,
left one machine a minute behind. A run the file does not know counts as
the median. `make walks WALKTIMES=1` writes the file again from the times
of that run, best on the whole set on a quiet machine, and keeps the time
it had for a run that the test's time stopped, since how long a run that
hung was stopped after says nothing of how long it takes. On three machines
each job still took four and three quarter minutes, setup included. More
bridges on a machine do not help: on four cores eight bridges finished a
share no sooner than six and broke three walks that wait on the clock.

Where a walk's time goes, measured: a walk of 60 steps took about 33
seconds, and 23 of them went on waiting after each step until no call
had been on its way for a quarter of a second. That quarter stood in for
the app's own short delays, the 150 ms before the clip timeline reads
its waveform and the 240 ms before a newly chosen clip is shown. Now
`settle` waits for those delays themselves, and for nothing else.
`__settled` in `preview/settled.ts` answers once no call is out, no
timer of the app's is set to fire within a second, and nothing on the
page moves for a second or less, a row sliding open or the clip
timeline gliding to its view. It is woken by the call coming back, the
timer firing or being cleared, and the movement's own end, never by
looking again on every frame. When nothing is left it lets two of the
browser's frames go by and answers if nothing came in between. The timer
that writes the log is left out, the same as its calls, marked
`logOnly` in `said.ts`. A longer timer is how long something stays on
screen, which a walk turns on itself with `holdClock`, and while the
app's time is held `settle` does not wait for timers at all. The same
walk now takes about 15 seconds, and a busy machine makes it slower
without making it check a step sooner. Steps that come sooner meet the
app's own sense of time where a person's hand has one: the clip
timeline reads two presses within half a second and six pixels as a
double-click, which cuts a part out. A walk that clicks where it clicked
a step before uses `clickOnTrack`, which lands eight pixels beside the
last spot first, so the click is one on its own. Opening
the browser takes a second or two. The rest is the gestures, and the
ones that play wait in real time: playing a clip to its end once took
most of the playback walk, so it now starts a moment before the clip's
last cut.

Time the app lets run out, the ten seconds of a toast or of a removed
picture or clip, is not waited for. A walk holds the app's clock before
that time starts, with Playwright's own fake clock, `holdClock` in
`walks/bridge.mjs`, and turns it on by hand, `runClock`: the toast is
still there after nine seconds and gone after ten and a half, in a
fraction of a second of the walk's own. On the real clock a busy machine
spent part of the ten seconds on the steps before, and the toast went
before the walk had looked, #341. Every timer, frame and
`performance.now` of the app stands still while the clock is held, so a
step that needs the app's own frames comes before `["hold the clock"]` or
after `letClockGo`. The walk's own waits, `settle` among them, keep the
real frames, kept on `window.__real`. A timer the app started
before the fake clock came is a real one and does not hold, so where the
time starts on its own, as the row a search leaves when it found nothing
new, the sequence begins with `["fake clock"]`, `fakeClock`, which runs
the app on Playwright's clock as fast as the real one until it is held,
and the step reads what is left of the time from the row's fill.

ESLint refuses a new wait on the clock in a walk, in `make interface`:
`waitForTimeout`, `setTimeout`, `setInterval`, `Date.now` and
`performance.now`, and a `timeout` or `polling` handed to a wait like
`waitForFunction`, see `frontend/eslint.config.js`. Playwright's
`page.clock` is no clock to it, so `holdClock` and `runClock` are the way
to let time pass. It reads the probes in `frontend/preview/` the same way.
Playwright's own 30 s on every action and wait is off too, `noDeadlines` in
`walks/bridge.mjs`, so a wait that never ends is ended by the `-timeout`
of `make walks`. The walks and the probes wait on no clock since #341 and
#495. What `frontend/eslint-suppressions.json` still counts for them are
reads of the clock that measure, and the stalls and late answers a step
puts on the app on purpose. One more in a file fails. A walk counts the
browser's frames only where the count is the scenario, a hand held still
and a Go side made slow on purpose, since #503. A probe fails at once
when its video preview shows no picture or its list has no clip to pick,
since #501, so a broken test video is said where it is found. See
No test waits on the clock in
[BUILD.md](BUILD.md#no-test-waits-on-the-clock).

CI runs it in the jobs `walks`, the only ones with Chromium, which comes
in the CI image they run in, see [The CI image](BUILD.md#the-ci-image), on
every pull request but one of docs alone.

A walk that breaks a rule leaves a picture of the app at that moment in
`/tmp/walk-<seed>.png`.

### Does it find anything

The walks and sequences over finding clips found nothing wrong with the
app. What they found was in the walks: a rule that wanted Stopping in the
frame after Cancel, where a search that stops within that frame already
says Continue, and the clip on screen read off the last captions asked
for. Both were the walk's, and both are put right.

Then `searching.mjs` broke about one `make walks` in three, on main as
well, at random. Against the bridge it broke 6 runs in 116, seeds 1 to
10: Cancel shows at once twice, how work ended stays after a restart
once, the clip list against the engine once and the clip timeline twice.
All four causes were the walk reading the app before or after what the
rule was about, see A walk waits on what a rule is about above, and none
was the app. Put right, the five seeds that broke ran 16 times each, 80
runs, without a rule broken.

Against the video preview from before the fixes in #106, five walks of
ten broke a rule within sixty steps, all of them showing a word typed in
beside another twice. The first walks against the code of the day found
a bug of their own within twenty steps: Enter opened the word at the
playhead rather than the word in the frame.

The sequences then found that removing a word in the middle of a caption
pulled the first word of the next caption up into the room it left, and
the walks, once the rule was that a removal changes no caption, that a
caption whose first word was removed waited for the word after it. The
engine now lays the captions out as if a removed word were still there,
`TestRemovingAWordPullsNoWordUp`. Then the walks in `make changed` found
that a word corrected to a short one and then removed took the room of
what the recogniser had heard, ten letters where it had read three, and
the caption overflowed. A removal now keeps what the word read,
`TestARemovedWordKeepsTheRoomItHad`.

The first sequences on the clip timeline found that a double-click on a
trimmed edge put it back on the frame nearest where the clip was found
rather than there: 24.80 where the clip had ended at 24.72, because a
search finds a clip on the episode's clock and not on its frames. An
edge put back now lands where the clip was found exactly,
`TestAClipKeepsWhereItWasFound`.

The playback walk found that a clip played from a playhead in a cut
played the cut: a double-click that cuts a part out leaves the playhead
in it. Every one of six walks showed it within sixteen steps. 2.121
fixed it on main at the same time, `playFrom` in `lib/playhead.ts`, and
the walk now holds it there.

The render is held to its frames by a sequence too, "a render of a
29.97 fps episode cuts on whole frames": a video of 40 seconds at 29.97
fps added with Add, its clip cut twice with a double-click, rendered with
Render, and the short read back. The cuts are where words end and start,
whole milliseconds, but the frames they land on start between two, the
case 2.125 is about. Against the render of main before 2.125 it fails
with 567 frames where the pieces hold 565, the first piece running a
frame into the part cut out, 169 frames more the wrong frame, the sound
71 ms longer than the picture, and 30 to 40 ms of quiet at each cut
where the fade is 15. With 2.125 it passes, in about 8 seconds.

Two sequences hold the render to the video preview where the frames do
not keep to their rate, plan row 2.130: "a short of a phone's uneven
frames shows what the video preview shows" and "a short of a picture
that starts after its sound shows what the video preview shows". Each
adds a video of 40 seconds made that way with Add, cuts its clip twice
with a double-click, the second with its start dragged to before the
picture begins, renders it with Render and reads the short back. Its
rate has to be 30 for the phone's frames, their average of 29.75
rounded, and 29.97 for the late picture, its own. Then, with the clip
timeline pinched in to a tenth, the playhead is put with a click on the
moments the short's frames stand for, the first and last three of every
piece and every fifth between, and the frame on the video preview's
canvas, read from its bands, has to be the short's frame. The playhead
goes after the moment and before the episode's next frame begins, by the
times ffprobe reads from the file, so the preview shows the frame that
holds the moment. 114 and 121 frames are looked at, all of them the
same, in three runs out of three. Against the render before 2.130 the
phone's short is made at 50 frames a second and the late picture's
holds 565 frames where its pieces last 564.

The late picture's sequence also checks, after the trim and the two cuts,
that every edge the hand put lands on one of the picture's own frames,
counted from where ffprobe says the picture starts, and not from the
start of the file, plan row 2.137. Against the app before 2.137 every one
of them was 0.49 of a frame from the picture's frames.

Ported to the frame queue, 2.123, the playback walk found within two
steps that a clip played from a playhead inside a frame stopped at its
end with the frame before its last on screen, 24.40 on a clip that ends
at 24.72. The queue draws frames on a grid of frames from where the play
began, so the last frame never came up. A play that ends now draws the
last frame of what played.

## From outside the app

The walks drive the interface from inside its page. Some of what the app
does starts outside it: a link in a mail that opens the app, with the app
closed or already open, and the app quitting and being started again. `make outside` checks those on a real Mac, the
way Tim did by hand, and CI runs it in the job `outside`, the only one
that needs any of what it sets up.

What it needs, and nothing else needs:

- The app as a bundle, registered with Launch Services, so macOS knows
  which app opens `framefairy://`. It is built with the build tag
  `outside`, which adds a probe, see below. Every other build leaves the
  probe out, so the app a customer gets has none of it.
- The local dispenser, `framefairy-dispenser dev`, running beside it. A
  sequence buys at its checkout and reads the keys out of the letter on
  its dev page, the letter a buyer gets.
- A keychain of its own, unlocked and made the default, because Unlock
  writes the key to the keychain, and a locked one puts a box on screen
  that nobody answers. The job makes it, and the test refuses to run
  outside CI: on a Mac of one's own it would replace the licence there.

The probe listens on 127.0.0.1:47290, for the test only. It says what the
app has: the links it was handed, whether its window is in front, and what
the settings show, read out of the page itself, and which windows it has
open. It presses a button the
way a click does, found by its words, and it quits the app. It adds a
video the way Add does once a file is picked, and asks for a search the
way Search does, and with that it can quit in the same moment, stopping
everything first the way the second Cmd+Q does. Which app is
in front comes from `lsappinfo`, which needs no permission a runner does
not have. The app's log goes to `~/Library/Logs/FrameFairy-outside.log`
as well, and the test shows it when a check fails.

No check waits on the clock, the same as every other test. Each waits to
be told:

- **The app is up.** The test listens on 127.0.0.1:47291, and the probe
  says hello there once it listens. Starting the app waits for that.
- **Something changed.** The probe counts changes: a link handed over,
  what the app tells the page, its window coming to the front or going
  to the back, and what the page shows, which the page tells it once a
  frame at most from a `MutationObserver` the probe puts there.
  `GET /state?after=N` answers once the count is past N, and a check asks
  again after each change until what it waits for holds.
- **The app is gone.** `GET /gone` answers at once and holds the answer
  open for as long as the app runs, so quitting waits for that answer to
  end with the process.
- **The letter is there.** After a checkout the test presses Deliver on
  the dispenser's dev page, which answers once Paddle's webhook is
  answered, and the letter goes out while it is.

A check that never holds is ended by `go test -timeout` in `make
outside`, and what the app had each time it changed is in the log.

The job stops after 30 minutes, so an app that hangs does not hold a Mac
for GitHub's six hours. Each check waits on what the app says, with no
limit of its own, so a check whose signal never comes is ended by go
test after `OUTSIDETIMEOUT`, 2m in CI, which prints every goroutine.

`make outside` builds the app as `make` does, then once more with the
probe into `.build/outside`, so `bin/` never has it, and builds the
dispenser beside it. It refuses to run anywhere but on a Mac with `CI`
set.

### The sequences

What is checked is written in one place, `internal/outside/sequences.go`,
the same way the walks' cases are in `sequences.mjs`: named sequences of
steps, each a verb and its words, the actions and what has to come of
them alike. The verbs are explained at the top of that file. The first
sequence reads:

```go
{"set up"},
{"buy", "2"},
{"quit"},
{"open link", "1"},
{"front", "Frame Fairy"},
{"saved", "yes"},
{"head", "Licensed"},
{"line has", "Unlocked from the link"},
...
```

The second sequence is the walk `searching.mjs` that pressed Search and
closed the app at once: it adds a video of 90 seconds, made with the
ffmpeg the app carries, waits for its first search to end, asks for a
search of 30 to 60 seconds and quits in the same moment, starts the app
again and asks the probe how that search ended. It has to be interrupted,
about the window it was asked for, which the clip list shows as
Interrupted with Continue. A search cut off there once left no record,
and nothing said it had been asked for.

The third is Bugs, plan row 2.244, which opens Report a Problem in a
window of its own, something no walk can see: the walks run in one page
of a browser. It clicks Bugs and asks the probe which windows the app has
open, `app` and `report`, clicks Bugs again and finds still the
two, closes the report window the way its close button does and finds
the app's alone, and then Bugs opens it once more. The probe names every
window the app has, from `app.Window.GetAll`, and closes one by its
name.

A link is handed to macOS with `open`, which is what a browser or a mail
app does once a link is clicked and its own question, whether to open
Frame Fairy, is answered. A check waits until it holds, 20 seconds at
most, so a step says what has to be true and never how long to wait.

Three files, each with one job:

- `sequences.go`, the sequences and the verbs. It builds everywhere, and
  `TestEveryStepIsOneTheRunnerKnows` reads it in `make unit`: a verb
  misspelt, a word too few or a key nobody bought fails on Linux in
  seconds, not on the Mac at the end of a CI run.
- `run_test.go`, how each verb is done on the Mac, the same for every
  sequence, and `TestEveryVerbIsDone`. Only `make outside` builds it.
- `cmd/framefairy-app/probe_outside.go`, what the app can be asked.

A new case is a new sequence. A new verb is a line in the list at the
top of `sequences.go`, its words in `verbs`, and how it is done in `done`
in `run_test.go`. `go test -tags outside -run 'TestSequences/<part of a
name>'` runs one.

A sequence that passes leaves one notice on the check with every step it
took, and one that fails leaves an error naming the step, as written, and
what the app said last. Either is on the pull request, so nobody has to
open the job's log.

What it cannot check: the browser's own question before it hands a link
on, which belongs to the browser, the mail app, and how any of it looks.

## Coverage

Which lines the tests run is counted once a night on main, and the
counts go to [Codecov](https://app.codecov.io/gh/timmrkz/framefairy),
which keeps them as one report of the code, the Go and the
interface, file by file and line by line, with how the whole moved from
night to night. It is the map of what no test reaches, and the way to
see whether work on the tests lowered it. A covered line is not a
checked line, see section 1 of the testing skill.

It runs in `walks-nightly.yml` and nowhere else. Counting makes a test
slower and an upload to Codecov can fail or be slow, so no pull request
counts anything or waits on Codecov, and Codecov says nothing on a pull
request or a commit, see `.github/codecov.yml`.

`COVERAGE=` a folder turns it on, for the three kinds of test, each a
flag in Codecov:

| Flag | What runs | What it writes |
| --- | --- | --- |
| `go-tests` | `make unit COVERAGE=`, without the race detector at night | `go-unit-<share>.txt`, Go's own profile |
| `interface-tests` | `make interface COVERAGE=`, Vitest with `@vitest/coverage-v8`, set in `frontend/vite.config.ts` | `interface-unit/lcov.info` |
| `walks` | `make walks COVERAGE=` | `walks-<share>-go.txt` and `walks-<share>-interface.lcov` |

Every Go package is counted for every test, `-coverpkg=framefairy/...`,
so a line of the engine that a test of the app runs counts as the
engine's. The walks count both sides of the app. The Go side runs in a
bridge, a process of its own started from the same test program, and
`bridgeArgs` in `cmd/framefairy-app/bridge_test.go` hands it the folder
`go test` gathers the counts from, so its counts are in the profile when
the test ends. Every page a walk opens counts the lines of the interface
it runs through Playwright, `counted` in `walks/bridge.mjs`, and
`preview/coverage.mjs` turns those counts into lcov of `frontend/src`
by the source maps the build leaves beside each script. Both go through
`preview/counts.mjs`, the one place that calls
monocart-coverage-reports, and `preview/counts.test.mjs` runs those
calls in `make interface`, so a new version of the package is checked
on its own pull request, not first by a night on main. That is the
only count of the components, since the interface's own tests do not
load them. A night whose walks counted no line of the job queue,
`cmd/framefairy-app/jobs.go`, which only a bridge runs, or no line of
any component fails, since every walk reaches both, and a count of
nothing means the counting broke.

Everything is counted on Linux, so the files built only for the Mac,
the `_darwin.go` files, and the tests that run only there, the Mac job
and `make outside`, are not in the report, rather than shown as not
run.

A kind of test that failed one night sends nothing, and Codecov keeps
its counts from the night before, carryforward, so a red night is not
read as the lines it would have reached going dark. Codecov is signed in
with GitHub's own OIDC token, so there is no Codecov token to keep.

To count by hand, in a cloud session or on a Mac: `make unit
COVERAGE=.build/coverage UNITPKGS=./engine`, then `go tool cover
-html=.build/coverage/go-unit.txt`.

## The testing skill in this project

The skill in `.claude/skills/testing/` is written for any project, so it
names nothing of ours. This is where its words meet this repository.

What the code takes from outside, so a test can drive it:

| What | Handed in as | Where |
| --- | --- | --- |
| the time of day | `now func() time.Time` | `signer.WithClock` in `licence/signer/signer.go`, `Now` in `licence/dispenser/engine.go`, `now` on the app, which times an edit, in `cmd/framefairy-app/main.go` |
| a wait the app sets between two things it does, where a bubble does not fit | a function that gives the channel the wait ends on | `after` on the updates in `cmd/framefairy-app/updates.go`, which a test hands the looks in through, see `TestARegularLookReplacesTheBuildReady`, `leaving` in `engine/ffmpeg.go`, how long ffmpeg or llama-server has to end once it is asked to stop, and `idleFor` in `engine/modelhost.go`, how long a model nobody holds is kept |
| a time limit on another program | a function that gives the context the limit ends | `timeLimit` in `engine/analysis.go`, which `stuckFFmpeg` in `engine/stuck_unix_test.go` ends the moment its ffmpeg says through a named pipe that it hangs |
| another program saying it hangs | a channel | `standin.Hangs` in `internal/standin/hangs_unix.go`, a named pipe a stand-in writes `hangs` into. It is heard as `stuck` on the engine, a render that has stopped making frames, in `engine/ffmpeg.go`, and as `frameLate` on the app, a video whose first frame is late, in `cmd/framefairy-app/library.go` |
| the end of the quiet of a recording | a channel | `still` on the listening in `cmd/framefairy-app/listen.go`, which the bridge's microphone says once the person in a walk has fallen silent |
| the two clocks of a recording, its level readings and its passes | channels | `beats` on the listening in `cmd/framefairy-app/listen.go`, which the bridge hands its steps through, see `TestARecordingRunsOnTheBeatsItIsHanded` |
| a loading llama-server saying it is up | its output | `hearReady` in `engine/local.go`, and `lookAgain`, the look every half second for a server that says nothing, which `TestAServerThatWillNotStopIsKilledAtOnce` takes away |
| how often work that reads from another program saves | a count of reads | `levelsReads` on the engine, `engine/levels.go`, in place of `levelsDefault` of the clock |
| the network | an `http.Client` with a transport of the test's | `localClient` in `engine/local.go`, `downloadClient` in `engine/speech.go`, with `neverAnswers`, `streamFrom` and `stopsAfter` in `engine/local_test.go`, so the stall of a model or a download is waited for in a bubble |
| timers inside goroutines | `testing/synctest` around the test | the lint fixture `scripts/testdata/lint/go/signals`, the dictation tests in `cmd/framefairy-app/listen_test.go`, `TestOnlyAFewDecodersNobodyHoldsStandRunning` in `cmd/framefairy-app/frames_test.go`, and the tests of a health check and of a model that keeps talking in `engine/local_test.go`, which ask llama-server through `neverAnswers` and `streamFrom`, transports with no network |
| the disk | `afero.Fs` | `engine/files.go`, the project and the app's store |
| random numbers | `io.Reader` | `signer.WithRandom` |
| the model, the recogniser | an interface with a fake | the fake recogniser and fake llama-server in `engine/project_test.go` |
| a program we run | its path | ffmpeg and the decoder through `FRAMEFAIRY_FRAMES` |
| asking another program to stop | a function | `interrupt` in `engine/leftover.go`, which a test wraps to hear which servers left behind were asked |
| a lock that has to wait | `lockWaits(path)`, called as the plan lock starts to wait for another holder | `engine/edit.go` and `engine/filelock_unix.go`, waited on through `waitsOn` in `engine/files_test.go`, which `engine/edit_test.go` uses too |
| the end of work | a callback the caller gives, a channel closed at every change, a channel closed when the work ends, or a `sync.WaitGroup` | `emit` in `newQueue`, and `until` on `queue.changed`, `cmd/framefairy-app/jobs.go`, `changed` on the updates in `cmd/framefairy-app/updates.go`, `done` of a measuring in `cmd/framefairy-app/levels.go`, `Arrived` in `updates/fetch.go`, `clearing` in `cmd/framefairy-app/main.go`, `news` on the model host in `engine/modelhost.go`, waited on by `untilHost`, `cursors` on the episode's decoder in `engine/episodeframes.go`, waited on by `untilCursors`. A stand-in program says it is up on its output, or ends once it has said what the test reads, as in `engine/leftover_test.go` and `engine/local_test.go` |

The fake clocks: `testing/synctest` in Go, whose `time.Sleep` line says
`//nolint:forbidigo // synctest bubble`, Vitest's fake timers in the
interface tests, as `frontend/src/lib/frames/captions.test.ts` does, and
Playwright's clock in the walks, through `fakeClock`, `holdClock`,
`runClock` and `letClockGo` in `walks/bridge.mjs`. In a bubble,
`synctest.Wait` tells a test that every goroutine of the bubble waits, as
the asks at once for a transcript in `cmd/framefairy-app/kept_test.go`
use it. The end-to-end tests
are the walks, against the real Go side through the bridge, and the
rules every walk checks after every step are in `walks/rules.mjs`.

A bubble fails its test with a deadlock when a goroutine started in it is
still blocked as the test ends. lumberjack, which keeps the app's log,
starts one with the first line it writes, and it never ends, so a test in
a bubble that reads the app's log opens it outside the bubble and writes
its first line there, `bubbleLog` in
`cmd/framefairy-app/listen_test.go`.

The skill's "ask the coverage" is the nightly report in Codecov, see
[Coverage](#coverage).

The skill's "run only what a change reaches" is `make changed` here, one
script for every machine, `scripts/changed.sh`: a pull request runs the
packages, parts and walks its change reaches, and main runs all of them,
see [One way to test](BUILD.md#one-way-to-test).

What CI holds of the skill's rules today: golangci-lint's `forbidigo` in
`make lint` for the clock, for a context with a deadline and for a check
on how long something took in a Go test, over the whole tree with no old
breaks let through and the checks from outside the app read the way they
build, on a Mac with the outside tag, `-timeout` from `OUTSIDETIMEOUT`
in `make outside`, 2m in CI, ESLint in `make
interface` for the clock in a walk or an interface test, with the old
breaks in `frontend/eslint-suppressions.json`, the race detector on every
Go test, `internal/offdisk` for a test on the real disk, and `-timeout`
from `UNITTIMEOUT` in `make unit`, 4m in CI, which ends a test whose signal
never comes. What it does not hold yet, skipped tests, polls with a
deadline that use no clock call, mutation testing and a check a pull
request cannot weaken, is #475.
