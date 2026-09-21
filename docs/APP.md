# framefairy-app, the desktop app

The app does what the command line does, with a window: it transcribes an
episode, finds the moments worth clipping, lets you check and adjust them on
a timeline and renders them as vertical shorts. It runs the same engine as
`framefairy`, so both give the same results from the same files.

Install the tools and models first, as [INSTALL.md](INSTALL.md) describes.

## Build and start

```
make run
```

That builds everything and starts the app. `make` alone builds it into
`bin/framefairy-app`. [BUILD.md](BUILD.md) covers the details and building
without make.

Building needs Node.js, because make builds the interface before the app.
The result is a plain program for now. The signed macOS app, the Windows installer and live reloading of the
interface come with the packaging work.

## Using it

### Episodes

The sidebar lists your episodes and how far each one is. **Add episode**, at
the bottom of it, opens a file dialog. Any mp4, mov, m4v or mkv works.
Transcription starts right away in the background.

The sidebar is a narrow rail until the pointer reaches the left edge, and it
opens over the workspace while it is there. The mark at the top keeps it
open, and clicking it while it is open closes it at once, even with the
pointer still on it. Hovering opens it again once the pointer has left and
come back. Closed, it leaves room for the settings column of the workspace,
which is exactly what it covers when it opens. **Add episode**, **Activity**
and **Settings** sit at the bottom of it and stay reachable as marks on the
rail. Every mark is in the same place on the rail as it is in the open
sidebar, to the pixel, so opening the sidebar never moves the mark out from
under the pointer that came for it. A dot on the Activity mark says work is
in hand. It sits over the mark, so nothing on the rail moves when a job
starts or ends.

The marks on an episode's row, on the right of it, show it in the file
manager and remove it. **Remove** asks in a box over the workspace whether to
keep the episode's files or delete them. The box names the folder they are
in, `<episode>.framefairy` beside the video. Keeping it means adding the episode
again picks up the transcript, the clip sets and the rendered clips where
this left off, which is why it is the highlighted answer. Deleting it takes
the whole folder, so nothing of the episode is left behind. Whatever is
running on that episode is stopped first and waited for, and if something
will not stop, nothing is deleted and the episode stays where it is with a
line saying so. A folder deleted under a running transcription comes back,
half written, for an episode that is no longer in the library. The video file
itself always stays, and so do the training records, which live in one
folder of their own and are thrown away in the settings and nowhere else.

The list of episodes is the app's own, in its config folder, not in those
work folders. So an episode stays in the list when its folder is deleted by
hand, and what it lost is simply gone: opening it then is the same as adding
it, and the transcription starts by itself. With no folder beside it there is
nothing to keep or to throw away, so the box says so and offers one
**Remove**.

Anything that cannot be taken back asks in that same box, and nothing else
does. Its buttons read left to right from the safest answer to the one that
cannot be taken back: **Cancel**, then what is recommended, then what is
destructive. The box opens on **Cancel**, the arrow keys and Tab move
between its buttons, Enter takes the one that is highlighted, and Escape
leaves everything as it was.

What is destructive is never the highlighted answer and never the one the
keyboard starts on. It is marked as what it does instead, in the colour of
a warning, so it is read before it is clicked rather than after. Making it
the default would mean that Enter, or a second Return still held down from
somewhere else, throws work away, and the whole reason this box exists is
that a click which throws work away has to be a click you meant.

### The window

A bar runs across the top of the window. It holds the window's own buttons
on macOS, it is what the window is dragged by, and it says what is on
screen: the name of the episode, or **Activity** or **Settings**. No screen
writes its own name below it, and the sidebar opens under it, so the name is
always there to read.

On macOS the bar **is** the title bar. macOS lays that out and centres its
three buttons in it, and nothing an app can set moves them by a pixel: the
height the window asks to leave empty only says how far down a drag still
moves the window. So the app takes the height it was given rather than
asking for one, and then the buttons are on the bar's middle because the
bar is what they were centred in. Everywhere else the system draws its own
title bar above the window and this one is an ordinary header.

The Go side measures and the stylesheet lays out. `FrameFairy.Chrome` answers
four numbers in whole pixels, the title bar's height and the left edge,
right edge and middle of the window buttons, and the window is told again
on the `chrome` event whenever the answer changes: a resize, either way
through fullscreen, another display, another scale, light or dark, back
from the Dock. Nothing is ever moved, so there is nothing that can snap
back, and nothing is measured on the page. The four numbers land on the
shell as `--bar-h`, `--lights-l`, `--lights-r` and `--lights-y`, and the
bar lays itself out from them. `--bar-h` is in `app.css` as well, which is
what the first frame uses and what every other system keeps. In native
fullscreen there is no title bar and no buttons, so the height stays as it
was and the name moves to the left edge rather than the bar collapsing.

The name of what is on screen sits on the buttons' line. Its box is
trimmed to the capitals and the baseline, so it is the letters that are
centred and not the space a line of text reserves around them, and then it
is padded back out so a name with a tail below the baseline, `YouTube.mp4`,
keeps it. A name too long for the bar ends in an ellipsis.

The window's own colour is the bar's colour, `--ink-1`. It is only ever
seen where the page does not paint, which on macOS 26 is the sliver
between the window's rounded corner and the webview's, and at the top that
sliver is inside the bar.

### The workspace

Selecting an episode opens its workspace. It is three columns, the settings
on the left, the video preview in the middle and the clips on the right, with
the clip up close under all three. It fits the window without scrolling.

The picture always agrees with the playhead. Whenever the window cannot show
the moment the playhead stands on, which is what happens while the machine is
busy transcribing or searching and a seek is dropped, the frame under the
playhead is read from the file by the engine and shown instead. The moment
the window catches up it takes over by itself. The engine keeps one frame per
second of an episode in its work folder, so going back over a stretch costs
nothing.

Opening an episode that already has clips opens on one of them: the clip it
was last worked on, or the first one where it has never been opened. The
playhead goes to the start of that clip, because that is what choosing a
clip does. Where the playhead stood when the app was closed is not kept, no
editor keeps that, and the start of a clip is a place that means something.
Which clip it was lives in the episode's own folder, so it goes when the
folder goes.

Putting the playhead somewhere on the range picker is asking to look there,
so the clip timeline goes there too. With a clip chosen that counts as moving
the view by hand, and the view stays where it was put.

The **crosshair** in the row under the timeline goes to the playhead, always,
clip or no clip, and puts it in the middle of the view, because the reason to
ask is to see where it is. Going back to the clip is what clicking the clip in
the list does, the one already chosen included, so the crosshair does not do it
as well: a control that went to the clip with one chosen and to the playhead
without could not be relied on for either.

The video preview is as big as the room allows and keeps the shape of the
episode, so it grows until either the height or the width runs out. The
middle column is exactly as wide as the picture, and the settings and the
clip list share everything left over. A wider window makes those two wider
rather than leaving a strip of nothing beside the picture, and a taller
window makes the picture bigger. Once the picture is as wide as it may be,
the height left over goes to the two tracks: the clip timeline grows and the
range picker stays exactly half of it, so nothing is left empty at the foot
of the window.

All of that is one expression in the stylesheet, worked out from the window
itself and the tokens in `app.css`. Two things it cannot know come in as
custom properties, the shape of the episode and the height of an error line
above the workspace, and neither changes because the window changed. So
dragging the window edge costs no JavaScript at all and the workspace keeps
up with the edge instead of arriving a frame behind it. The waveform is the
one thing still told its size in pixels, because a canvas has to be, and
nothing is laid out from the answer. The stretch chosen on the range picker stays with the episode
while the app runs, so leaving the workspace and coming back does not throw
it away.

Two things are called what they are, here and everywhere else. The slim strip
under the video preview, the whole episode at a glance, is the **range
picker**. The waveform under the whole workspace, the episode up close, is the
**clip timeline**.

Nothing explains itself in a line of text that is always on screen. Every
control says what it is for when the pointer rests on it, and the small info
marks open a bubble that says more, on hover or on a click. A bubble is a few
sentences, never an essay, and it hangs from the window rather than from the
area it belongs to, so nothing clips it and nothing lies over it. An area
with an info mark carries no tooltip of its own: one explanation, in one
place.

- **The transcription is the head of the clip list**, in the shape
  everything else in that pane has: the head says **Transcribing**, the one
  button says **Pause**, and the line under them fills up as it goes. Paused,
  the head says so and the button says **Continue**, which picks up where it
  stopped, also after a restart. The info mark beside the head says what is
  happening and how long it has to go, and goes back to saying what the clip
  list is once there are clips to list.
- **What is not there yet says so by waiting.** The part of the clip
  timeline the transcript has not reached and the rows the clip list will
  have are places waiting to be filled: a soft light passes over them, the
  same light the window on the range picker shows while clips are being
  found for it. On the clip timeline the edge is taken from the waveform
  itself, so the light never lies over a waveform that is already drawn. The
  range picker itself stays quiet: what is not transcribed is darker, with a
  line of the muted grey at the edge, and the edge moves as the transcript
  grows. The line is what carries it. Two greys this dark are about 1.2 to 1
  against each other however far apart they are put, measured off the
  pixels, because lightness is compressed at this end of the scale, so the
  shade alone can never say where the transcript has got to. A line can: it
  carries its contrast in the step across it rather than in the area, and it
  is the edge the eye follows as the transcript grows. It moves with the work, not
  with the saving of it: the transcript is written to disk every few
  seconds, but every chunk the recogniser finishes says how far it has come,
  and the edge follows that. The only thing on that track that moves of its
  own accord is the window while a search runs.
- **The clip list is a stack of cards**, each clip its own, with air
  between them and the app's colour down the edge of the chosen one. It has
  no box around it: the ends go under a veil, so a card scrolling out of
  sight fades rather than being cut off, which is what says there is more.
  What a search is doing fills the button it was started from, behind its
  own words, so nothing is drawn across the list for it.
- **The playhead** is a thin line in the app's colour with a head at the
  top, the way an editor draws one, the same on both tracks. It is drawn
  over the track rather than inside it, because a track clips what is in it
  to keep the waveform inside its rounded corners, so the head stands above
  the track and is whole even at the very start or the very end. It is the one
  thing in the app with a hue that is not the accent, so it can be found on
  a dark track, on a waveform and inside a clip alike.
- **The arrows up and down walk the clip list**, the way left and right walk
  the episode: each step takes the next clip and puts the playhead at its
  start.
- **The pane is the clip list and nothing else.** It says **Clips** and
  carries **New**, whatever else is happening. The transcription is not in
  it at any point: it has a place of its own, on the range picker, at the
  edge it moves. The two run in lanes of their own, and a head that carried
  both is how it came to say **Transcribing** over a list of clips.
- **New waits until a search could run.** A search reads the transcript off
  disk, so **New** is off until the saved transcript reaches the end of the
  chosen stretch, and says how far it has got and how far it needs to go.
  It goes by what is written down rather than by what has been heard,
  because a search started on the second one would read a transcript that
  stops short of the stretch it was asked for.
- **The transcription is worked from the edge it moves.** On the range
  picker, at the transcript's edge, a mark appears while the pointer is on
  the track and does the one thing there is to do: pause it while it reads,
  carry on while it is stopped part way. It travels with the edge and glides
  with it, so the two read as one thing: both are carried by a transform, so
  the distance between them never changes. It is not there when the episode
  is read to the end, because then there is nothing to do.
- **What is running shows in the button it was started from.** Work that
  knows how far along it is fills the button, with a line of the app's
  colour at the front of the fill. Work that cannot say sends a band of that
  colour travelling across it instead.
- **The chosen card is always in view.** Walking the list with the arrows
  brings it far enough in to clear the veil at either end.
- **The clip timeline goes to what is playing.** Pressing play, or the space
  bar, takes the view to the clip, the way an editor's timeline follows
  playback. A view moved by hand is let go of then, and only then.
- **The clip list waits in the shape it will have.** While the transcript is
  still coming, and while a search runs, the list holds as many rows as the
  search was asked for, with the light passing over them, so it does not
  fill out from four rows to twelve. Nothing is written in the empty space:
  what is happening is in the info mark at the head.
- **Settings column:** what the model looks for and how the captions look.
  **New clips** holds how many clips to find and how long they may be, and
  **Captions** holds the face and the size for every clip of the episode, and
  the height the captions sit at, which is kept for every episode. The
  height follows the black box in the picture as it is dragged, not when it
  is let go, and a mark at the end of the **Captions** row puts the captions
  back as they start out, the font, the size and the height together, because
  it is beside the head of the whole group and not beside one row of it. It
  turns anticlockwise for that, and once they are back it turns clockwise
  instead and puts them as you had them, so one click is undone by one click.
  It is not there when there is nothing to undo either way, and the whole group appears once a clip is selected. How
  many clips a search looks for and how long they may be are set here and
  nowhere else, and they are kept for the next episode too. Every setting is one row, the name
  on the left and the control on the right, all of them the same width. A
  unit stands inside its field, right beside the number. Which stretch is
  searched is chosen on the track, not here.
- **Video preview:** the episode, with one **Play** button that plays from where
  the playhead stands. The space bar does the same, unless a field has the
  keyboard. Clicking the video preview plays or pauses.
    - For the selected clip, everything outside its vertical crop is dimmed
      and the frame sits where the render will put it. The frame is dashed
      while the playhead is in a part the clip cuts out.
    - Playing a clip jumps over its cuts and stops where the clip ends,
      however it was started. **Loop** starts it over instead, which is how
      a clip is judged. It stays on until it is switched off again.
    - While the playhead is inside the clip, its captions are drawn inside
      the crop, in the font, size, place and colours the render burns in,
      with the spoken word on its pill. The engine hands over the lines and
      the look, so a correction shows up here at once.
- **Moving the captions:** drag the caption box up or down. It lands on a
  step of a grid that appears while it moves, and it stays inside the frame.
  **There is one place for every clip of every episode**, because a place
  that suits one video suits the next one, so the drag saves it as a setting
  rather than as an edit of that clip. It stands as **Height** with the other
  caption settings on the left, where it can be typed instead, and **Put the
  captions back** appears under it once it is not the standard 300. A click
  on the box without dragging plays or pauses.
- **Moving the crop:** drag the frame sideways when the automatic crop is
  off. The new place applies to every piece of the clip filmed from the same
  camera angle. **Automatic crop** appears once a crop was moved and brings
  back the automatic placement for that angle, and **Put the crop back**
  takes that click back again. A click on the frame without dragging plays
  or pauses.
- **Clip list:** beside the player, exactly as tall as the video preview with
  the range picker under it, every clip found, in time order, with its start
  and length. A green dot marks a rendered clip. Click one to select it,
  which also puts the clip timeline back on it.
    - The clips lie on a surface of their own, the same grey as the clip
      timeline, parted from each other by lighter lines than the ones that
      part one area of the workspace from another.
    - **A new episode finds its first clips by itself.** Adding a video is
      all it takes: the transcription starts, and the moment it covers the
      stretch chosen on the track the first search runs. Until then the info
      mark beside the head says so, and the stretch can still be moved. It
      happens only for an episode nobody has ever searched. The episode
      remembers that somebody looked, so removing every clip again does not
      bring a search of its own back: a search is the machine's time, and
      nobody asked for it. Deleting the work folder makes the episode new,
      and then it starts over as a new one does. The rule and its tests are
      in `frontend/src/lib/flow.ts`.
    - **What a search is doing is the head of the list itself.** **New**
      becomes **Cancel** while work runs, and the line under it fills up as
      the work goes on. Nothing is added to the column, so the clips never
      move down. A render shows there in the same way. The model answers
      with the whole set at once, so the clips all land together, and they
      are in the list the moment the answer is written rather than when the
      job has wrapped up.
    - **New**, above the list, finds clips in the stretch chosen on the
      track, and says so while it looks. The clips it finds join the ones
      already there. Choosing a stretch that was searched before and asking
      again replaces its clips and everything done to them, so it asks
      first.
    - The trash can on a row removes that clip. Its mark leaves the track at
      once, and its row stays in place for ten seconds, in red with a trash
      can, saying **Removed** and offering **Put it back**. Nothing above or
      below it moves while it is there, and when it goes the list closes
      over it. The clip stays in the plan with everything done to it, and a
      render of the whole plan leaves it out.
- **Range picker:** one slim strip for the whole episode, under the video
  preview and exactly as wide as it. It is where the stretch to search is
  chosen, and it shows what has been searched, where the clips are, how far
  the transcript has come and where the playhead stands.
    - Drag across it to choose the stretch to search, or drag the window or
      its edges. Long episodes open with the first 30 minutes chosen. The
      window is a box on all four sides, placed in whole pixels, so every
      edge of it is drawn the same. The border of the box is the edge, and
      there is nothing drawn beside it: the room to take hold of an edge is
      there but not seen, and the whole box brightens when an edge is under
      the pointer or holds the keyboard focus.
    - **Edges land on a round step**, the smallest one that is still about
      eight pixels wide: ten seconds for a short episode, five minutes for a
      four hour one. A wall wins over the step, so a stretch that runs into
      a searched one ends exactly at it. While a stretch is drawn or moved
      it says what it is, in a pill over the track.
    - While clips are being found for it, the window cannot be moved and a
      soft light passes through it every couple of seconds, which is the
      track saying work is in hand. How far the search has come is on the
      line under the head of the clip list, so the window only has to say
      that something is running.
    - A window drawn over a stretch that was searched already is a window
      onto what that stretch would be without it: the plain track, as it
      looks where nobody has looked yet, and the clips inside it are not
      drawn. So what the trash can in its corner does is plain before it is
      pressed. The trash can waits until the window is under the pointer,
      and the times are drawn over everything, so nothing laid on the track
      ever hides where you are.
    - A stretch that has been searched is marked. **The window may be drawn
      anywhere**, over a mark, part of one or none at all: the window is the
      stretch you mean, and what happens to it is decided by the button you
      press. The model still never gets the same material twice by
      accident, because a window that lies over a mark says so before
      anything is searched.
    - **Looking again.** **New** over material that was searched asks
      first. It names the stretch and how many clips are in it, and on Look
      again those clips are removed and the model reads the stretch as if
      for the first time. Clips outside the window stay as they are.
    - **Removing what the window covers.** A window that lies over a mark
      wears a trash can in its top right corner, just outside it when the
      window is too narrow to hold it. It asks first, because the clips in
      the stretch leave the list along with every trim, crop and caption
      place. Clips already rendered stay as files on disk, and the caption
      files of the clips are moved aside rather than deleted. Afterwards
      that stretch is free again, even when it is the middle of a longer
      search: the plan keeps the rest of its window and notes the part it
      gave back.
    - A mark for every clip sits inside the track, green once rendered.
      Click a mark to select that clip.
    - A click without dragging moves the player there, wherever it lands,
      marked or not. A press that wobbles a few pixels is still a click, so
      nothing is drawn by accident.
    - Double-click it for the whole episode.
    - The part not yet transcribed is darker, with a line at the edge where
      the transcript has got to, and a light passing over it while the
      transcription runs.
    - On the edge, while the pointer is on the track, the mark that pauses
      the transcription or carries it on.
    - The window is locked while clips are being found.
- **Nothing sits under the range picker.** The line that parts the workspace
  from the clip up close runs right below it, and the workspace is exactly as
  tall as the video preview and the range picker need. If the transcript has
  not reached the end of the chosen stretch yet, a search waits for it and
  starts by itself.
- **One mark explains one thing, where that thing is.** The video preview,
  the range picker and the clip timeline each carry their own info mark in
  their top right corner, and a mark only appears while the pointer is on
  the area it belongs to. The same goes for the marks beside **New clips**,
  **Captions** and the head of the clip list. No mark explains two areas,
  and nothing floats at the end of a row of its own.
- **One row under the clip up close**, so the range picker and the waveform
  stand together with nothing between them: play, loop and the crosshair
  that goes to the playhead, as the three marks anyone knows, then the
  title of the selected clip, why it was chosen and its numbers, and on the
  right what a reset threw away,
  **Render** and, once rendered, **Show in folder**. The time is not written
  anywhere else: the playhead says it on both timelines. Above that row, the
  clip up close:
    - The arrow keys step the playhead one frame of the episode, and one
      second with shift, wherever the keyboard is as long as it is not in a
      field or on an edge of the clip.
    - Drag an edge to trim. Edges snap to words the way the render cuts them.
    - Click an edge to put the playhead exactly on it, which is how a clip
      is started over.
    - The playhead carries a magnifier. Click it for the lens, which
      magnifies the words around the playhead with the one being spoken
      highlighted. Another click takes the lens away again, and so does
      Escape when nothing else is asking for it. Nothing else
      opens or closes it, so the track stays clear until the words are asked
      for. The magnifier never moves the playhead, and past the end of the
      transcript there is nothing to magnify, so it is off there and says
      so.
    - The clip lies over the waveform in the accent colour. A hatched block
      inside it is a stretch the clip leaves out, usually dead air the
      engine found: playing jumps it, and the render does too.
    - **The cuts can be changed.** Each one carries a handle on either edge,
      in the accent's lighter shade so it is not taken for the clip's own
      edge. Dragging a handle moves that edge of the cut, a double-click on
      the block puts the stretch back, and holding shift while dragging
      across the clip takes a stretch out by hand. Without shift the same
      drag moves the playhead, so nothing that worked before works
      differently, and a shift-click with no drag does nothing.
    - **A stretch put back goes back in with the same gesture.** The
      double-click that puts a cut back is remembered, so a second
      double-click in the same place takes the stretch out again, edge for
      edge. It is forgotten as soon as anything else about that clip
      changes, because a stretch put back into a clip that has moved on is
      not the stretch that was taken out.
    - **A cut lands on whole words.** The engine puts the edges where the
      render would cut them, so a cut dragged over a pause takes the whole
      pause and a cut dragged over speech takes whole words. A cut can
      never stop half way through a word, because half a word is a sound
      nobody said and the captions are built from the words. The timeline
      works the same snapping out while the hand is moving, so the block
      shown is the block the render will leave out.
    - **Alt lands on the frame instead.** Holding alt while dragging, on a
      handle or across the clip, leaves the edges exactly where the hand
      put them, rounded to a whole frame of the episode and no further, for
      a word clipped a little short or a breath worth keeping. A cut made
      that way may stop inside a word, which is the point of it. The key is
      read while the hand moves rather than when it goes down, so letting
      go of alt part way through goes back to snapping and the block says
      so before the drag ends.
    - Nothing is written over the waveform. The captions are in the video
      preview as they are spoken, and the words to correct are in the lens,
      so the waveform has the whole track to itself.
    - The waveform is drawn the way an editor draws one: one column of the
      screen per column of the picture, each the loudest reading that falls
      in it, each a whole pixel wide. Nothing is ever drawn between two
      pixels, so it keeps the same weight at every zoom instead of
      brightening and dimming as it is pinched.
    - The times sit at the top of both tracks, in the same quiet grey, a
      step above the lines around them and below the waveform inside them.
    - Click a word in the lens to correct it. Enter saves, Escape cancels.
      The correction applies to every clip with that word.
    - A correction may hold more than one word. Where the recogniser heard
      one word and two were said, writing both splits the word it measured
      between them, so the captions break and highlight them one by one.
      Writing one word again makes it one word again.
    - Drag the playhead anywhere on the track and the video preview follows.
    - Time labels along the top say where in the episode the timeline is,
      which is what a swipe needs in order to mean anything. They are at the
      top and in the same colour as the ones on the range picker: two tracks
      that both say where you are say it in the same place. A ruler is two
      layers and has to be: the line goes behind what is drawn on the
      track, so a minute falling on the edge of the window on the range
      picker never paints over its border, and the time goes in front of
      everything, because a time nothing can read is worse than no time at
      all. The time is not written inside its line, because an element with
      a z-index makes a stacking context and would hold its own time down
      there with it.
    - **Two fingers move along the episode**, the way an editing timeline
      works: a swipe left or right travels through the episode, and a pinch
      zooms around the pointer, down to two seconds across the track and out
      to the whole episode. A view moved by hand stays where it was put,
      wherever the playhead goes. The crosshair in the row under the track
      goes to the playhead and puts it in the middle. A double-click lets
      go of the view, and so does clicking a clip in the list, the one that
      is already selected included, which puts that clip back in view. The words and the waveform of the whole episode
      are read once and kept, so swiping does not wait for a file to be
      read again.
- **Before the first transcription** there are no words and no waveform.
  That is where every episode starts, so the track is simply empty. The
  words and the waveform appear as the transcript grows past them, without
  anything being clicked. Nothing about this is an error, and nothing about
  it is logged as one.
- **The timeline is always there.** With no clip selected it shows the
  minute around the playhead and follows it as the episode plays, so there
  is always something saying where you are. Trimming and corrections need a
  clip, so they appear once one is selected.

Every change is saved at once. There is no save button. A changed clip gets
new captions on its next render.

Two files that would share a work folder cannot both be in the library.
Everything about an episode lives in a folder named after it without its
extension, so `ep.mp4` and `ep.mov` side by side would share a transcript,
clip sets and rendered names. The second one is left out and the reason
says which two.

### What the window may ask for

Every call that names a file is checked against the library before anything
is read, written or started, and a path is judged by where it really leads.
The window only ever names files it was given, so this is the last line
rather than the first, but it is the line that holds when something else
asks.

### Activity

Everything that runs in the background, with progress, a log per job and
**Cancel**, which says it heard you while the job winds down. One job is one
row, parted from the next by a line across the page, and clicking a finished
row opens its log. Transcription runs in its own lane, so finding and
rendering clips never wait for it.

### Settings

Paths to ffmpeg, llama-server and the models, the planner and the output
folder. **Colours** holds two, and they are together because the whole point
is that they are two: **the app** is what the app picks things out in, the
chosen clip, the window on the range picker, a button that matters, and **the
word highlight** is the pill behind the word being spoken, burned into the
short. A taste in one is not a taste in the other. They start out the same,
`#942192`, so an app nobody has touched looks of a piece with what it makes.
The app's colour is the only one the interface has: the lighter shade under
the pointer and the wash behind a chosen clip are mixed from it, so changing
it moves all three, and it moves as the colour is picked rather than when it
is saved. Every setting in a column ends in the same place, the face of the
captions along with the numbers, and the mark that says a field opens a
list stands where a unit stands rather than where the system would draw
it. **Training data** says where the records of
every episode are, `~/.framefairy/training` unless another folder is named, how
many there are, and has the one trash can that throws them away, which asks
first. What belongs to the episode being worked on is not here: the caption
face, the size and the height, and how many clips a search looks for and how
long they may be, all sit in the workspace next to the clip, and what is set
there is kept for the next episode. Empty paths use the same defaults as the
command line. **Check again** tests the
setup and says what is missing, and lists the caption faces the app carries
inside itself.

## Where things are kept

- **Settings and the episode list:** plain JSON files in
  `~/Library/Application Support/Frame Fairy` on macOS, `%AppData%\Frame Fairy` on
  Windows and `~/.config/Frame Fairy` on Linux.
- **Everything about an episode:** next to it in `<episode>.framefairy/`, the
  same folder the command line uses. See [CLI.md](CLI.md#run).
- **Word corrections:** in `<episode>.framefairy/logs/corrections.json`. They
  change what captions show, never the stored transcript.
- **Training records:** in `<episode>.framefairy/training/`. The app writes them
  and shows nothing about them. See [TRAINING.md](TRAINING.md).

The app only shows files that belong to an episode in its list. It finds
Homebrew's ffmpeg and llama-server even when it is started from Finder.

## How the app is built

| Part | Where | What |
| --- | --- | --- |
| Go side | `cmd/framefairy-app/` | the window, the calls the interface makes, the job queue, settings |
| Interface | `frontend/` | Svelte 5 and TypeScript, no Go |
| Built interface | `cmd/framefairy-app/dist/app/` | made by make from `frontend/`, embedded into the program, not in git |

The Go side uses Wails v3, pinned to v3.0.0-beta.23. The interface calls the
methods of the `FrameFairy` type in `cmd/framefairy-app/main.go` by name, through the
typed wrappers in `frontend/src/lib/api.ts`.

The built interface lives next to the Go code because Go can only embed files
from its own folder. It is generated, so never edit it by hand.

### Changing the interface

```
make run
```

make notices the change, installs the interface's packages if they are
missing and builds the interface before the app. `make test` also checks the
interface's types.

| Folder | What |
| --- | --- |
| `frontend/src/screens/` | the workspace, activity and settings |
| `frontend/src/components/` | player, timelines, clip list, progress bar |
| `frontend/src/lib/` | calls into Go and the shared state |
| `frontend/src/app.css` | colours, sizes and the base styles |

## Tests

```
make test
```

They cover the job queue, waiting for the transcript and the media route.
The one that transcribes is skipped when ffmpeg is missing.
