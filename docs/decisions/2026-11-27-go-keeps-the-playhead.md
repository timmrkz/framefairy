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
2. **Go says where on a clip a moment is.** A clip comes with the
   moments the playhead is on it from, at its end from and off it again
   from, `SourceInfo.Places`, worked out on the frames the engine counts,
   on the clip as saved and on the clip under the hand. `placeOf`
   compares the moment with them and with what the gesture was about.
   `frameAt` and `insideClip` go.
3. **Go says where a step lands.** Each word of the captions, and each
   word lit on frames of the short, comes with where a step of the arrow
   keys lands on it and where it begins as a place to land on, `LandOn`.
   `intoWord` and `landings` go. Each piece comes with where the
   playhead stands for its edges, `SegmentView.Up`, the rule a drag of
   the edge already answers with, `edgePlayhead`, so a press on an edge
   stands where its drag will.
4. **The clip's clock in Go.** A clip's captions come with each block and
   each word placed in the episode, and its cuts named. `inEpisode`,
   `wordInEpisode`, `endInEpisode`, `inClip`, `cutsOf`'s own rule and
   `litWord` go.
5. **Frames counted in Go.** Go sends, once a video, where each frame of
   its grid holds from, `framewire.Grid.From` over `/frames/froms`, and
   the frame queue, the clip timeline and the time field look a moment
   up there. A thumbnail is put on the middle of its frame by the
   engine, `SetThumbnail`. `frameMiddle`, `frameBegins`, `heldMicros`,
   `FRAME_HAIR`, `rankAt`, `rankOf` and `playMoves` go, and
   `frame.cases.json` moves to the engine's own testdata, read by Go
   alone.

What changed from the plan as first written, and why:

- **No playhead kept in Go.** The place is decided by what the gesture
  was about, which only the window knows, the moment it happens. Kept in
  Go, it would come back on the answer to a look, which travels apart
  from the binding that saves a trim, and the space bar pressed in
  between would read the place before. What the place needs from the
  engine is which frames are the clip's, and that is now data the clip
  carries, so the window decides by the gesture and counts nothing.
- **The keys are not answered by Go.** A step needs the frame and the
  word it lands on, both of which are now data, and a round trip for each
  press of a held key would make the keys wait on the app. What is left
  of `arrowStep` and `wordStep` is which way the key goes, by the word the
  playhead is in.
- **A table, not a rule, for frames.** A look could have named its
  moment and Go counted the frame, but the frame queue keeps the frames
  it has by their number, and finding a kept frame for a click is what
  makes a click show at once. Go sends where each frame holds from
  instead, eight bytes a frame, 0.8 MB for an hour at 30 frames a second,
  read once a video, and the window looks a moment up by halving.
- **No step 6.** How a choice of model stands is judged in Go already,
  `Standing`, since the pull request that moved the judging there.
  `finderStanding` and `speechStanding` only put Go's judgement into the
  words of the line, which is the interface's work, like every label.
  Moving the words to Go would move the interface's wording into the
  engine, not a rule out of the window.

## What follows

What it costs: the engine works out three more things it already had
the means for, the clip's places, the edge playheads and where a step
lands in each word, and sends them on data it sends anyway, the clip
entry, the framing and the caption words. The window fetches one table
per video, eight bytes a frame, about 0.8 MB for an hour at 30 frames a
second, read once and kept for the last four videos. No key and no click
waits for Go: everything a gesture needs is in the window before the
hand moves.

What it buys: one line to fix for the playhead, wherever it is drawn,
and no rule of the engine left in the window, so the issue this record
works on closes and the line under Not to copy in ARCHITECTURE.md
goes. The window's code and its tests get smaller, counted in the pull
request.

What would show it was wrong: a key or a click that answers later than
on main, a play whose lines stutter where the first version did not, or
a frame step slower at the 90th percentile. All are measured on the
bridge before and after.
