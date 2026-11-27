# Go keeps the playhead, and the window draws one

The last step of [The video preview draws what Go sends](2026-11-25-the-video-preview-draws-what-go-sends.md),
its step 3, together with what is left of the issue that record works
on: rules of the engine written again in the window.

## The problem

The user, 27 November, on the first version of this pull request, which
made the playhead's line on the clip timeline glide with the sound:

> you forgot the same type of animation in the range picker, which is
> especially noticeable on very short videos ... the fact that there is
> a bunch of code changed while apparently the problem of the stuttering
> playhead is only fixed in one place tells me that the software design
> is again broken, because there's like one behavior that's used in
> different places, and then there should be just one fix necessary in
> order to fix it everywhere ... fix the architecture ... fix and
> resolve this issue ... I don't want this dumb issue to be open any
> longer.

Read on the branch at bc9e9f7, main f2e63e5.

**The playhead is drawn twice.** The range picker draws a line at
`left: {on(playhead, inner)}` in `RangeWindow.svelte`, the clip timeline
another at `.at` in `ClipTimeline.svelte`. Each is handed the moment by
`Episode.svelte` as a prop, `time`, which moves once a frame of the
video while it plays. The first version of this pull request taught the
clip timeline's line to follow the sound through a function of its own,
`sound()`, called by `Player.svelte` through `onsound` and `Episode`,
and left the range picker's line stepping. A third place that draws the
playhead would need a third fix.

**The window still decides where the playhead is and what it is on.**
`lib/playhead.ts`, 216 lines: whether the playhead is on the clip, at
its end or on the video, `placeOf`, `placeFor`, `playedToEnd`, what the
space bar plays from there, `playFrom`, and where an arrow key goes,
`arrowStep`, with `wordStep`, `intoWord` and `landings` in `api.ts`.
`Player.svelte` keeps the place, `placed`, and works out the clip's own
clock, `inClipTime`.

**The window still turns moments between the clip's clock and the
episode's.** `lib/flow.ts`: `inEpisode`, `wordInEpisode`,
`endInEpisode`, `inClip`, `cutsOf`, `insideClip` and `litWord`, used in
`ClipTimeline.svelte` 19 times and in `Episode.svelte` 4 times, to put
a caption's block, a word and a cut on the clip timeline, and to light
the word under the playhead.

**The window still counts frames.** `frameAt`, `frameMiddle`,
`frameBegins`, `heldMicros` and `FRAME_HAIR` in `flow.ts`, and `rankAt`
in `frames/movie.ts`, held to Go by `lib/testdata/frame.cases.json`,
which the issue's rule of 1 November rules out. Used to draw the paused
playhead on its frame's start, to light a thumbnail or a keyframe the
playhead is on, for the time field, for where a thumbnail and a
keyframe go, and by the frame queue to name a frame.

**The window still words how a choice of model stands.**
`finderStanding` and `speechStanding` in `lib/finding.ts` turn Go's
`Standing` into the line under each list.

## Five simulations

### 1. Who may know about whom

The window knows the hand, the view of each timeline in pixels, and the
sound card's clock. It knows no clip, no frame grid and no word. Go
knows them all. So every answer about a moment, which frame holds it,
which word is lit, whether it is on the clip, where a step goes, is
Go's, and the window is told it. Drawing a moment inside a view is the
window's, because only it knows the view. Layers: the window draws a
moment in a view, the app keeps each video's playhead and answers the
hand, the engine holds the rules. What crosses: a look carries what the
gesture was about and, for a key, the step, and the answer carries the
playhead. Next change: a new fact about the moment on screen is a field
of the playhead. Cost: the place, the step and the clock move to the
engine, the window's copies go.

### 2. What changes together

The two lines, the time field, the lit word and the crop frame's place
are one moment shown five ways. They change together, so they come from
one value: the moment the video preview shows, and what Go says of it.
While it plays, the lines change on every refresh and the rest on every
frame of the video. So there are two writers, each one: the sound
card's clock writes the moment the lines stand on, once a refresh, into
one custom property that every playhead reads, and the frame on screen
brings the rest. A line is one component, drawn by its view. Layers as
in the first. Next change: a third view that shows the playhead uses the
component and is right at once. Cost: one component, one property.

### 3. Where state lives

The place of the playhead is a state: a gesture sets it, a play keeps
it and a play that reaches the clip's end changes it. Gestures and
plays both reach Go now, through looks and plays. So the place lives
there, one for each video, beside the play the app keeps, and the
window holds none. The moment lives in the sound card while it plays,
which is the window's, and in Go while it rests. Layers: the app keeps
`playhead` for each video, `engine.Playhead` holds what it is, the
window keeps the last one it was sent. What crosses: the playhead, with
the frame's dress. Next change: the command line playing a preview
would use the same playhead. Cost: as the first.

### 4. What runs together

A frame, its dress and the playhead are one moment on screen. Every
frame Go sends comes dressed already, from a look or a play, so the
playhead rides in the dress, and a play's place changes in the frame
that reaches the clip's end. The lines run with the screen's refresh,
apart from the frames, and so they are moved by the clock alone,
through the stylesheet, not by Svelte working the workspace out again.
Next change: anything drawn in step with a frame comes in its dress.
Cost: a field of the dress, and the frame queue hands the moment of
each refresh to one place.

### 5. What a test has to build

A rule in the window is tested by a walk, or by a TypeScript test of a
copy. Place, step, clip clock, lit word and frame as engine functions
are table tests in Go with no decoder, beside the render's own rules
they must equal. `frame.cases.json` goes, because no second side counts
frames. The window's tests of `playhead.ts`, `flow.ts`'s clocks and
frames and `movie.ts`'s `rankAt` go, and a walk checks what a person
sees: both lines glide, and stand where Go says once paused. Cost: Go
tests in, TypeScript tests out.

## What converges

All five move the place, the step, the clip clock, the lit word and the
frame count to Go, and leave drawing, the hand and the clock in the
window. Four of five send the playhead with the frame's dress, because
every frame already comes dressed. Two of five, the second and the
fourth, find that the lines must move on the refresh and the rest on the
frame, and both answer it with one writer of one custom property and
one component for every line. Only the third keeps the place in the
app rather than in the engine: the rule is the engine's, the place of
each video is kept by the app beside its play, as the dress's framings
are.

Dropped: a playhead in `VideoState`, which is sent at most four times a
second and would lag every gesture, and a line moved by JavaScript on
each refresh in each view, which is the first version of this pull
request, one writer for each view.

## The decision

In one pull request, a commit each.

1. **One playhead on screen.** `components/Playhead.svelte` is the only
   line that shows the playhead, in the range picker and in the clip
   timeline, each placing it in its own view. At rest it stands on the
   moment its view hands it. While the video plays it stands at
   `--sound`, the moment in seconds, which the video preview writes once
   a refresh through `lib/sounding.ts`, the one writer, on each playhead
   of its workspace and on nothing else: written on the workspace, the
   number would be inherited by every element in it, and the browser
   would work all their styles out again on every refresh. Playing, a
   line lands on a pixel of the screen, at rest on a whole one.
2. **Go keeps the playhead.** `engine.Playhead` is the moment shown, the
   start of its frame, the place, video, clip or end, the clip it is on,
   the moment on the clip's clock and the word lit. The engine says it,
   `PlaceOf`, `PlayFrom` and `PlayedToEnd`, from the clip's framing, and
   the app keeps one for each video. A look says what its gesture was
   about, and its answer and every frame of a play carry the playhead in
   the dress. The window shows the place it is given. `placeOf`,
   `placeFor`, `playFrom`, `playedToEnd` and `inClipTime` go.
3. **Go answers the keyboard.** The space bar asks for a play, and Go
   plays the clip or the video from the playhead it keeps. An arrow is a
   look with the step it asks for, a frame, a word or a skip, and Go
   answers with the frame there, or says the step left the clip.
   `arrowStep`, `wordStep`, `intoWord`, `landings` and `insideClip` go,
   and so does the window asking for the frames of the words either
   side: Go reads them ahead from its own playhead.
4. **The clip's clock in Go.** A clip's captions come with each block and
   each word placed in the episode, and its cuts named. `inEpisode`,
   `wordInEpisode`, `endInEpisode`, `inClip`, `cutsOf` and `litWord` go.
5. **Frames counted in Go.** The playhead carries its frame's start, and
   a thumbnail or a keyframe is lit by the frame Go says it is on. A
   thumbnail and a keyframe are put on their frames by the engine. A
   look names its moment, and Go counts the frame. `frameAt`,
   `frameMiddle`, `frameBegins`, `heldMicros`, `FRAME_HAIR`, `rankAt`
   and `frame.cases.json` go.
6. **The line under a model list in Go.** `Standing` carries the line
   and its tone, and `finding.ts` keeps only what lists the models.

```
window      draws a moment in a view, the hand, the sound card's clock
   |        Playhead.svelte reads --sound, written by the video preview alone
routes      /frames/look (about, step), /frames/play, /frames/read
   |
app         keeps each video's playhead beside its play and its framings
   |
engine      Playhead: place, play from, step, clip clock, lit word, frame
```

## What follows

What it costs: the engine gains the place, the step and the clip's
clock, which it partly has already for the render and the captions, and
the dress gains the playhead. A key waits for Go's answer, which a step
already did, since its frame comes from Go.

What it buys: one line to fix for the playhead, wherever it is drawn,
and no rule of the engine left in the window, so the issue this record
works on closes and the line under Not to copy in ARCHITECTURE.md
goes. The window's code and its tests get smaller, counted in the pull
request.

What would show it was wrong: a key or a click that answers later than
on main, a play whose lines stutter where the first version did not, or
a frame step slower at the 90th percentile. All are measured on the
bridge before and after.
