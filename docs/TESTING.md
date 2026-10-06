# Testing the interface

What runs where, and how to see the interface do what a person does with
it, without a screen. The engine's own tests, the fuzz targets and the
path tests are in [BUILD.md](BUILD.md). This page is about the interface.

## Two ways to open it in a browser

The app cannot be started in a cloud session, so the interface is opened
in Chromium, through Playwright, with something answering the calls it
makes to the Go side.

| | Answers the calls | Good for |
| --- | --- | --- |
| the preview, `frontend/preview/vite.config.ts` | `wails-stub.ts`, a stand-in written in TypeScript | the layout, a picture of any state, states the engine is slow to reach |
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
`frontend/preview/frames/cuts.mjs` plays a clip with cuts on it and checks
every frame drawn and every sample heard. The preview's own episode is
made the same way, so the same is measured in the workspace. What it
measures and how to read it is in the interface skill, under Playback and
The frame queue.

Adding a video does not run against the bridge: the bridge adds its
episode and searches it before it serves, and answers `AddEpisodes` with
nothing. So what the video preview shows while an episode just added is
heard and searched is a probe on the preview,
`frontend/preview/frames/added.mjs`, with `?growing&lagging&hear=100`:
the first search started by the Go side, job events, twelve clips landing
and the earliest chosen when it is over, then three clips picked. On
every animation frame from the click that opens the episode it reads the
playhead and the frame on the canvas from its bars, and it exits 1 when
the first picture is not the episode's first frame with the playhead at
0, when the picture goes black, when any frame holds neither the
playhead nor where it was a moment before, or when a seek's frame takes
longer than a quarter of a second. Tim saw the video preview of an episode
just added on another frame than its first, in the days of the `<video>`
element. The frame queue shows the first frame there, on the code of
e8ca612 and after it. Put back to draw 7.3 seconds in, the probe fails.

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
grey that grows lighter. Thin, so no two frames differ enough to be a
camera switch, which would split the clip the search finds.

A video picked with a rate is filmed at it, 29.97 fps or any other, for
what a render does with frames that do not start on a whole millisecond.
A short is a narrow part of the picture made larger, which would cut the
bars away, so its frame number is eleven bands one above the other across
the whole width, the highest bit at the top, four pixels each of 180,
light for one and dark for nought, with the grey below them. Light and
dark are only 100 apart, so the bands change no more of the picture than
a camera switch would. Its sound is a steady tone. `shortFrames` and
`shortSound` in `walks/bridge.mjs` read a short of it back from disk.

A video filmed by two cameras, which `/pick` makes with a switch, has the
same strip, and below it each camera looks at a chequerboard on its own
side of the picture, the first on the left against dark grey and the
second on the right against light red. The switch changes the whole
picture at once, so the search finds it and parts its clip there, and
frames each shot on its own subject, so the two pieces have crops far
apart. A shot shown through the other's crop is a plain backdrop, which
the video preview and a short both show at a glance. Its sound is a
steady tone in plain samples rather than Opus: a render seeks into each
piece, and the first 80 ms of Opus after a seek are decoded quiet, a dip
of the file's own at every piece.

In the browser, `wails-bridge.ts` takes the place of the Wails runtime: a
call is a POST to `/call`, and what the Go side tells the interface comes
as server-sent events on `/events`. `window.__calls` lists every call and
how it ended, and `window.__menu("undo")` and `window.__menu("redo")` are
the menu bar, which a browser does not have.

Calls that reach beyond the work folder are answered by the bridge with
nothing: the network, the keychain, a box from the system, another app.
So nothing a walk presses downloads a model or installs a build. The setup
counts the two stand-ins as installed, so the app opens on its episodes
rather than on its first-run screen.

The Add button asks the system for files, and the system's box is the
bridge: it hands over what a walk picked, or nothing, the way the box
answers Cancel. Besides the interface's calls, a walk can ask the bridge,
each with a POST:

| Path | What it does |
| --- | --- |
| `/pick?seconds=N` | makes a new video of N seconds, not in the library, for the Add button's box to hand over next. With `&rate=30000/1001` it is filmed at that frame rate, and with `&switch=S` by two cameras that switch S seconds in, see below |
| `/model?hang=1&fail=0` | the language model holds its answers until the search is stopped, or fails, or with both off answers |
| `/speech?ms=N` | the speech model takes N milliseconds over each piece of audio, so a transcript grows slowly enough to be seen and cancelled |
| `/reopen` | closes the app and opens it again, the way quitting and starting it does: the work stops and how it ended is read back. It answers with the jobs as the app left them, once nothing ran any more |
| `/reset` | the app as it was once its first episode was searched: the models answer quickly, every episode a walk added is gone, the work folder and the settings are what they were, and the app is opened again on them |

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
set. The bridge puts the episode back the way it was first searched
before every walk, `POST /reset`, so a walk starts from the same place
every time.

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
them back in, and some it never says.

The rules, after every step, once no call has been on its way for a
quarter of a second:

| Rule | What it would have caught |
| --- | --- |
| No call failed and the page threw nothing | |
| The caption box shows one of the engine's captions exactly, the same words in the same order at the same moments, whenever no word is open | a word typed in beside another shown twice, "weil ein \| ein" |
| Enter opens the word in the frame | Enter opening the word at the playhead instead, found by the first walks |
| One word at most wears the frame, and one at most is open | |
| An edit stays where it was made: every word more than three seconds of the episode from the word corrected keeps its text and its time | removing a word typed in beside another taking the other with it |
| Taking a word out changes no caption: every caption still there appears and goes when it did, and only one whose words were all removed may go, the one before it then staying up through its time | the caption that broke where a removed word had been, and the next caption's first word pulled up into the room a removed word left |
| Undo puts back the engine's captions from before the step it takes back, and Redo those from after it | |

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
pieces as well as its captions.

A walk is a script in the folder that walks with `walk.mjs`, which runs
the loop: look, pick a gesture that can be made, make it, check the
rules. `TestWalks` runs every one it finds, for the same seeds.

### `playback.mjs`: playing a clip

The gestures: the space bar, playing for a moment and pausing, playing a
clip to its end from a moment before its last cut, or before its end
when it has none, a double-click that cuts a part out, one that puts a cut
back, a click on the clip timeline, and Shift and an arrow. While a clip
plays, the walk records every frame the video preview puts on screen, by
the frame number read back from the bars on its canvas on every animation
frame, not by the clock and not by what the frame queue says it drew.
Where the playhead is, it reads off `data-playhead` on `.screen`, and
whether it plays off the play button, Play or Pause. The rules, besides
every walk's:

| Rule | What it would have caught |
| --- | --- |
| Every frame put on screen while a clip plays is in one of its pieces, a frame of give either side | the part just cut played when the space bar was pressed with the playhead in it |
| The space bar plays: a frame comes, or the playhead moves | a press of the space bar lost while the file was still being read |
| Paused, the picture stays where it was paused: the playhead stays, and the frame on screen is the one that holds it, or at the clip's end the one that ends on it | |
| A clip played to its end stops at its end | |

The bridge's episode runs at five frames a second, so a frame of give is
a fifth of a second. Whether a press plays the clip or the episode is
read off the app, the chosen clip dimmed on the clip timeline while the
playhead is on the video, never worked out by the walk: one that decided
it by itself was a second idea of where play starts, and drifted from the
app's the day 2.121 made it a state.

### `searching.mjs`: from a new video to its clips, and a search

The gestures: adding a video with Add, New, Cancel, Continue, the model
holding its answers, failing or answering again, the speech slow or
quick, the app closed and opened again, another episode opened, and a
wait. With the speech quick and the model answering, adding a video waits
for its first clips. With the speech slow the walk goes on while the
video is transcribed, so the next steps land in the middle of it. The
rules, besides every walk's:

| Rule | |
| --- | --- |
| A new video gets its first clips with no click | while the models answer and the speech is quick |
| Cancel shows at once: in the frame after the click the head no longer says Cancel, or a row says Stopping | held to a press of the button while it said Cancel |
| The clip list says what the engine's jobs do: Cancel while a search runs, Continue with "Stopped", "Interrupted" or "Failed. Click Continue" after one that stopped, New otherwise | asked again for a few seconds, since a row is held a moment to be read |
| No row speaks of a failure the engine did not have | |
| At rest, as many cards and the count beside Clips as the engine has clips | |
| How work ended stays after a restart: a search the closing cut off is interrupted, one that stopped or failed stays as it was, one that was done leaves nothing | by how the engine left the search, which `/reopen` answers with |

The clip on screen is the card the clip list marks as the current one,
not the captions the interface last asked for: an episode just opened
shows its clip before that, and a walk then held it to another episode's.

#### A walk waits on what a rule is about

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

A sequence is a named list of steps and of what has to come of them,
written as data at the top of the file, so a case found by hand is kept
as it was found. The steps are the walk's gestures by name, `frame` a
word, `click` it, `press` a key, `type`, `undo`, `redo`, `mark` the
engine's captions and pieces under a label, and on the clip timeline
`cut at` a share of the clip, `join` a cut, `trim` an edge by some
pixels and `reset` an edge. What has to come of them: `box`, what
the caption box reads, `open`, which word is open, `same`, the engine's
captions and pieces as they were at a mark, `spans`, every caption
appearing and going when it did at a mark, and `cuts`, how many cuts the
clip timeline shows. For finding clips: `model` holds, fails or answers,
`add` a video, press the clip list's `head` button and `restart` the app,
and then `wait for` the head to say a word, a `row` to say something, and
how many `cards` the list holds. For a render: `add` a video filmed at a
frame rate, press `render` and wait for the short, and `short`, the short
read back from disk holding exactly the frames of the clip's pieces, each
the right frame of the episode by its bands, with its sound as long as
its picture and quiet at each cut for no longer than the render's fade.
For a clip of two shots: `add cameras`, a video filmed by two cameras,
`cut switch`, a double-click where two shots meet, `trim past` a switch,
which drags an edge two seconds beyond it, and `look` at every piece,
which puts the playhead in its middle with a click on the clip timeline
and remembers the crop frame there and what the picture shows inside it.
Then `pieces`, how many pieces the clip timeline draws, `framed`, the
crop frame on every piece where it stood at a look and showing what it
showed, and `shots`, the short read back from disk with every frame
showing what the crop frame showed on its shot and the sound straight on
where two shots meet. Every step is also checked against the walk's
rules, so a sequence asks for its own result and gets the rest for
nothing.

Each sequence starts from the bridge's episode as it was first searched,
in a page of its own.

`a cut or an edge put back keeps the camera switch` is Tim's test of
#116 done the way he would do it. It adds a video of 30 seconds whose
camera switches at 12, and its search finds one clip in two pieces that
meet there. It looks at both pieces, double-clicks across the switch to
cut it, and double-clicks the cut to put it back. Then the clip timeline
has to draw two pieces again, and with the playhead in the second shot
the crop frame has to stand where it stood, on the second shot's subject,
with the picture inside it what it was. It drags the clip's end to two
seconds before the switch, so the second shot is gone, double-clicks the
end to put it back, and asks the same. Last it presses Render and reads
the short back from disk: every frame has to look like what the crop
frame showed on its shot, by its colour and its detail, so a frame of the
second shot through the first shot's crop, a plain red, fails. And the
sound has to run straight on where the two shots meet: no 10 ms within a
tenth of a second of the switch quieter than 80% of the tone.

Against main's engine it fails at the first put-back: with the playhead
in the second shot, the crop frame stands 2.5% in rather than 65%, over a
plain red with no detail rather than the chequerboard, because the cut
was put back as one piece in the first shot's crop. Without the first
put-back it fails the same way at the second. With only main's render it
fails at the short: by the switch the sound is down to 19% of the tone,
the fade out and in of a cut where nothing is cut.

### Where they run

`make walks` runs the sequences and every walk for seeds 1 to `WALKS`, 3
unless set, each walk as long as it says, 60 steps, or 25 for playback,
which plays in real time, or 30 for searching, unless `STEPS` is set, through the Go test `TestWalks`,
which fails with what a sequence or a walk printed. The seeds are the
same every time, so a walk that breaks a rule breaks it again on the
next run. `make walks WALKS=40` looks further.

They run side by side, as many at once as the machine has cores, or
`WALKERS`. Each runs against a bridge of its own, which `TestWalks`
starts as a process of its own on a free port, because a desk sets
`HOME` for the whole process it is in. On four cores, the cloud's and
CI's, the thirteen runs take a little over two minutes, where one after
another they took nine, and each takes as long as it does alone.

Where a walk's time goes, measured: a walk of 60 steps takes about 30
seconds, and more than half of that is waiting after each step until no call
has been on its way for a quarter of a second. That is as long as the
app's own short delays, the 150 ms before the clip timeline reads its
waveform and the 240 ms before a newly chosen clip is shown, and a
shorter wait would check a step before the app had finished it. Opening
the browser takes a second or two. The rest is the gestures, and the
ones that play wait in real time: playing a clip to its end once took
most of the playback walk, so it now starts a moment before the clip's
last cut.

`make changed` runs it whenever the interface changed, the preview's own
stand-in aside, or a package the app reaches, the engine among them. CI
runs it in the job `walks`, the only one that installs Chromium, on the
same rules as the build.

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

Ported to the frame queue, 2.123, the playback walk found within two
steps that a clip played from a playhead inside a frame stopped at its
end with the frame before its last on screen, 24.40 on a clip that ends
at 24.72. The queue draws frames on a grid of frames from where the play
began, so the last frame never came up. A play that ends now draws the
last frame of what played.
