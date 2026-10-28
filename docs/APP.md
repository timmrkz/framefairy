# framefairy-app, the desktop app

The app does what the command line does, on screen: it transcribes an
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

### The first run

A new copy of the app on a machine with nothing on it needs two things, and
only one of them is a question. Until both are answered the setup is the
whole app: no sidebar, no workspace, nothing to press that would not work.

**Speech** is always local. No speech model ships with the app, so the app
fetches one. There is one today, which is not a choice, so the app says what
it is about to do and does it: the download starts by itself and reports how
far it has come, with the same fill every other piece of work in the app
wears, and **Cancel** stops it. The row says what the model is, what it
covers, what the download costs and what it costs on disk, before anything
starts. It is a job like any other, in the lane of hearing, and a search that
comes to hearing waits for the model rather than failing on it.

**Finding clips** is the question. A model in the cloud, Anthropic's or
OpenAI's with a key of your own, works on any machine and
costs a few cents a video. A model on this machine is free to run and
needs the memory to hold it. Either way only the words are read: the video
and the audio never leave the machine. The answer is saved the moment it is
given and can be changed later in the settings.

Choosing the cloud opens a choice of model, Claude Sonnet 5 or GPT-6 Sol, and a field for that company's key, which goes in the macOS keychain
and nowhere else, never in the settings file.

Choosing **On this machine** shows the models that can be installed, each
with its maker, what it costs to fetch, what it costs in memory to run and
what this machine can do with it. That last part is the point of the list.
A model runs from memory, so a machine too small for one will swap, and a
model that swaps takes minutes to answer rather than seconds. The app reads
how much memory the machine has and says, beside each model, **Best for
this machine**, **Fits this machine**, **Tight on this machine** or **Too
big for this machine**, with the machine's own figure above the list so the
judgement can be checked. The best one is the largest model the machine
can hold comfortably, largest by the model and not by the memory it
takes, because the two are not the same thing: see PACKAGING.md.

Nothing is hidden and nothing is refused: a model the app thinks is too big
can still be installed, because a machine's memory can be read wrong and it
is not the app's place to decide. A machine that will not say how much
memory it has is offered the smallest, because that is the one most likely
to run, and nothing else is promised.

A `.gguf` already in `~/.framefairy/models` is used as it is, whether the
app fetched it or not.

**Installing is work in hand, and it looks it.** The button it was started
from carries it, the way every control that starts work does: the beam
round it and the fill for how far the download has come, and it reads
**Cancel** while it runs. What has arrived stays, and **Install** carries on
from it. The row itself does not light up or move. The line under the name
that says what the model costs says instead how far it has come, how fast
and how long is left, *Fetching 1.8 GB of 5.2 GB at 42 MB a second, 0:13
left*, in the same one line, so the row keeps its height. The numbers are
counted in thousands, the way the size to fetch is, so a model said to be
5.2 GB arrives as 5.2 GB of 5.2 GB.

A model is only half of the local way. `llama-server`, from llama.cpp, is
what runs it, and a model without one is fifteen gigabytes that answer
nothing. So the app looks for it and says so while it is missing, and it
does not call itself ready on a machine that has the download and no way
to open it, which it used to. It is taken the way ffmpeg is: the one
beside the app, checked to be the file the app was built with, and no
other, see **The tools are the app's own** below.

The last button is **Finish setup**, and that is all it does. Adding an
episode has one way of being done and it is the plus in the sidebar, so a
second way here would be a second way of doing one thing, bought for a
click that is only ever saved once. While the speech model is still coming
the button says so and waits, because there is nothing to be done with the
app until it is there.

A key found in the environment is not an answer to the question. The app
never decides on somebody's behalf, so an app that has never been asked asks.

### Episodes

The sidebar lists your episodes and how far each one is. **Add episode**, at
the bottom of it, opens a file dialog. Which files it takes is said once,
in [Which files are videos](#which-files-are-videos).
Transcription starts right away in the background.

A video dragged from Finder and dropped anywhere on the app is added the
same way, whatever page is open, and opens. While a file is held over
the app, the app wears the frame of what is chosen, with the accent's
wash inside it. A video already in the list opens rather than being added
twice, and a file that is not a video is not added. A toast says so by
its name, "notes.pdf cannot be added because it is not a video", the
way Add says why it could not add a file, see [Toasts](#toasts). Add and
a drop are one path, `takeFiles` in
`drop.go`: the same files are taken, the same reasons given, and the
video just added opens, or a video given again. The reason stays until
it is read or the next files are taken. The window marks the whole page as where a
drop lands, `data-file-drop-target`, once the first-run setup is done,
and not while a box asks first or the app asks about quitting. Plan row
2.192.

A drop is the one way into the library that does not come from a box the
system draws. Wails hands the dropped paths to the page and the page
hands them back, so the paths pass through the page on their way. They
meet the same checks as Add's: a video's extension, a picture the
decoder can read, and no link, see `linkedEpisode`. That was decided
knowingly, since the page is the app's own.

The app opens on the episode that was open when it was left. With no
episode in the library, the page says **Add a video** and has the
**Add** button on it, since there is nothing to pick. Removing the
episode that is open goes to the one opened before it, the way closing a
tab shows the one before, and the page for no episode shows only when
there is none left to show.

The episodes are listed in the order they were added, the newest at the
foot, where one just added is brought into view. Once there are two, the
sort mark at the right of the head of the sidebar, an arrow up beside an
arrow down as Reminders, Notes and Files have it, opens a list of
**Added**, with a clock, and **Name**, with A over Z. A tick marks the one
that is on. The sidebar stays open while
that list is, and after it closes until the pointer moves away. Escape
closes one thing at a time: the list, then the sidebar hovering opened. By name, Folge 2 comes before Folge 10. The choice is kept by
the app, like whether the sidebar stays open. A library from before this
kept no order, so its episodes stay in the order they were in, sorted by
where they are on disk, and the ones added from now on go at the foot.

Three dots show at the end of a row while the pointer is on it, and open
what can be done with that video, the way Finder offers it on a
right-click: its **label**, a row about its words, **Show in folder** and
**Remove**, which asks first. The dots lie in a row, the way the Mac draws More, not one over the
other as Android does. They are the sort mark's size, lit the same way and
in its column, so the marks of the sidebar stand one under the other. A
row is lit while the pointer is anywhere on it, its dots included, and the
dots take the pointer over the row's whole height, so a pointer going down
the column of dots goes from one to the next and nothing goes dark on the
way. Tim saw the row go dark each time the pointer reached its dots. The
menu hangs under the row and is exactly as wide as it, so it reaches no
further over the workspace than the sidebar does, and the sidebar stays
open while it is. Its icons stand in the sidebar's column of icons and its
words where the sidebar's words start. Its rows are as tall as the rows of
videos above it and run from edge to edge, the way a list's do in
`Pick.svelte`.

The row about the words says the one thing that can be done with them
now, issue #432, asked for by Tim. Two rows of which one never works were
two things to read for one. Each is an action, which closes the menu the
way a menu item does on the Mac. How far the work has come is not the
menu's to show, Tim's call on #458:

- **Transcribe video** while the transcript has not reached the video's
  end, whether it never started, was stopped by hand, by a search, by the
  app closing or by failing. It hears the video to its end with no search
  after, a job of its own, `engine.JobTranscribe`, see [JOBS.md](JOBS.md),
  and carries on from what was heard. Why it failed is said in a toast as
  it fails.
- **Stop transcribing** while it runs. What was heard stays, and the menu
  says Transcribe video again from the frame it is clicked.

It and the work on the video's clips go one at a time, Tim's report of 26
October, 12.27. A search or a clip made by hand of the video stops its
transcription, the way Stop transcribing does, and it does not start
again by itself once they are done. The stop shows on the press, in the
menu, the video's dot and the range picker, though the transcription
ends only once the speech model has heard the piece it is in: the menu
said Stop transcribing for three and a half seconds after Search, Tim's
report of 26 October, 14.22. While they run, Transcribe video
cannot be clicked, and its title says to transcribe once the search is
done. The menu once offered Continue transcribing, Transcribe the rest
and Stop transcribing the rest, for a transcription that waited behind a
search and carried on after it, and nobody could read them. A search of
another video takes the speech model for a while, see below, and the
transcription carries on after it by itself.

- **Export transcript** once the transcript reaches the end. It hangs its
  formats on the right of the menu, the way Export To does in Pages, and
  a format opens the system's save box on "<video> transcript" with the
  format's ending. **Text** is the words in paragraphs, each opening with
  the time it starts at, to read or paste into show notes. **Subtitles**
  is an srt file, at most two lines of 42 characters and 7 seconds each,
  which YouTube, Apple Podcasts and every video editor take. WebVTT is
  left out because everything that takes it takes srt too. Every word
  reads the way it was corrected in the caption box of a clip that is not
  removed, `engine.CorrectedTranscript`, and where two clips correct the
  same word differently, the clip in the plan changed last wins. A word
  removed from a clip's captions is still said, so it stays. The file
  always gets the format's ending, whatever name is typed, so the save
  box can never replace the video or any other kind of file. A name typed
  without it is given the ending, and a file already there under that
  name was never asked about, so it is left alone and a toast says to
  pick another name. A file the box asked to replace is replaced whole,
  written beside it and moved in place, and a link is replaced and not
  what it leads to. It is the one file the app writes outside the
  library, where the person says in the system's own box.

While it runs, the range picker frames the part it works on the way it
frames the window of a search that hears, the same line, breath and
motes, over that part's dots, which still say what is not transcribed
yet, Tim's pick on #458. Its left edge follows the work as it hears.
Whenever it waits its turn, behind a search
of another video that finds with the local model, a clip made by hand of
another video or a model being installed, its frame stays, in less of the colour and with no motes,
still breathing, so it reads as on hold and not as gone, Tim's reports of
23 and 24 October. It goes on hold the moment that search takes the
hearing, so its dots stand still from Search on, and what the speech
model was still hearing then is let go, so nothing is kept that was not
shown and the edge does not jump when it carries on. A search with a model in the cloud holds
nothing up, so the frame stays as it is. Stop transcribing stops everything on the
range picker in the frame it is clicked: the dots stay where they are,
because what the speech model was still hearing is let go, and the frame
stands still and fades out.
The dots follow the work again once other work hears, or once the video
is transcribed to its end, which a stop on its last piece can still
bring. The video's
dot in the sidebar pulses the way it does for all work, from the frame
Transcribe video is clicked until the frame Stop transcribing is.
Nothing else says how far it has come, Tim's calls on #458: a number in
the menu, and then one in the video's row, were more to read for what
the range picker already shows. The library is read again each
time the work goes on to another part of the video, after a search or a
clip made by hand took the hearing for a while, so what it says was heard
never goes back.

A label is a few words and a colour beside the video's name, in its row
and in the title bar. It is how videos with the same file name are told
apart, a folder of episodes that are each `youtube.mp4` above all. The
video keeps its name and is never touched: the label is kept in its work
folder, `label.json`, see `engine.Label`. The colours are the accent
colours of the settings, the same row of round colours with the colour
well at its end, `Swatches.svelte`.

The menu shows the label as it is, its words chosen so typing replaces
them. A video with no label has an empty field with no colour chosen, and
the name of its folder in grey in the field, which Tab takes, the way Tab
takes a suggestion in a field on the Mac. The moment the field has words,
by Tab or by typing, a colour is chosen, one no other video wears yet, and
the pill shows in the row. A colour clicked in an empty field takes the
folder's name with it. An empty field is no label, so the field, the
colours and the row never disagree, and a menu opened for Show in folder
leaves the video as it was. A round before opened the menu on the
folder's name and a colour, which Tim read as a label the video did not
have.

Everything saves at once: a colour when it is clicked, the words with
Return, when the field is left or when the menu closes. Return closes the
menu after a colour is clicked too, and the menu wears no focus ring of
its own. The pill follows
the words while they are typed, and Escape puts back what was there. Escape
in the colour well puts back the colour from before it opened, and on a
video with no label that is no colour and no words, so a shade looked at
there leaves no label behind. The cross at the end of the field shows
while it has words and removes the label, and so does Return on a field
with no words. A label keeps at most 40 characters on one line, an emoji
counted as one. A video at the root of a drive, with no folder to name,
has its file's name as the hint.

A save the Go side refuses, a full disk or a work folder that cannot be
written, says so in a toast, and the row and the title bar go back to the
label the video still has. The pill shows only what is kept or still on
its way.

The sidebar is a narrow rail until the pointer reaches the left edge, and it
opens over the workspace while it is there. The mark at the top keeps it
open, and clicking it while it is open closes it at once, even with the
pointer still on it. Hovering opens it again once the pointer has left and
come back. It closes when the app's window loses the focus, as it does
to the report window Bugs opens, since a Mac tells the window nothing
of the pointer going then, and the next move of the pointer says where
it is. Only the keyboard going into it opens it, not the focus coming
back to the button last clicked. Hovering opens it only on a real move
of the pointer onto it, and only while the app is in front, taking in no
move made while another app is: macOS hands
the app moves that move nothing, while Mission Control shows it above
all, and a sidebar that answered them opened and closed over and over
under a still pointer. The log says every time it opens or closes and
what did it. Closed, it leaves room for the settings column of the workspace,
which is exactly what it covers when it opens. Open, it casts a shadow on
the workspace to its right and none on the title bar above it. **Add episode**,
**Settings** and **Updates** sit at the bottom of it and stay reachable as
marks on the rail. Updates says which build is running, and a still dot on
its mark says a newer one is ready. Its page has the build and its commit,
the channel it follows, where things stand, and Check or Relaunch. See
[UPDATES.md](UPDATES.md). Every mark is in the same place on the rail as it is in the open
sidebar, to the pixel, so opening the sidebar never moves the mark out from
under the pointer that came for it. There is no page of jobs: work in
hand is the lamp of the episode it is for and shows where it was started,
and a page that listed it all again said everything twice. It was
removed, see below under **Work in the background**.

The episodes stay on the rail too, as their lamps. **An episode's row is
one line and the rail's own 36 pixel box**, so shut it is a square around
its lamp, exactly like every other mark on the rail, and open it is the
same square with the name beside it. It keeps that height and its lamp
keeps its column whether the sidebar is shut or open, so the rail is one
column of marks from the top to the bottom and nothing in it moves as the
sidebar goes over. The name is what waits for the room, along with the two
marks on the row, which need a pointer on the row anyway. What the episode
has, its transcript and its clip sets, is in the row's own title: it is
one line about state on a rail of lamps, and it made the row two lines
tall, which left the lamp adrift in a box half again its size.

The marks on an episode's row, on the right of it, show it in the file
manager and remove it. **Remove** asks in a box over the workspace whether to
keep the episode's files or delete them. While it asks, it has the
keyboard: the space bar presses the button that has it, and Undo, the
arrows and I and O leave the episode behind it alone. The box names the folder they are
in, `<episode>.framefairy` beside the video. Keeping it means adding the episode
again picks up the transcript, the clip sets and the rendered clips where
this left off, which is why it is the highlighted answer. Deleting it takes
the whole folder, so nothing of the episode is left behind. Whatever is
running on that episode is stopped either way, so a removed episode never
goes on transcribing and holding up the episodes added after it. From the
moment removing begins, nothing new starts on it until it is added again,
not even the transcription a stopped search would otherwise carry on.
Once it is out of the list, nothing about how its work ended is said any
more. Added again, a video says only what its folder still holds: kept,
a search or a transcription that stopped says so again with Continue, the
way it does after a restart, and deleted, it starts from nothing. Tim
removed a video, added the same file again, and its menu offered Continue
transcribing for work the new video never had.
Deleting also waits for the work to stop, and if something will not stop,
nothing is deleted and the episode stays where it is with a line saying
so. The answer clicked says Removing at once and the box takes no second
click while the work stops, which is a moment when a search runs. The
workspace of the episode closes before the list is read again, so it asks
nothing more of an episode that is gone. A folder deleted under a running transcription comes back,
half written, for an episode that is no longer in the library. The video file
itself always stays, and so do the training records, which live in one
folder of their own and are thrown away in the settings and nowhere else.

The list of episodes is the app's own, `library.json` in its config folder,
because a video can be anywhere on disk. Everything else about a video is in
its work folder, so deleting that folder by hand, in Finder, is the same as
**Remove** with the folder deleted: the video leaves the list, its work
stops, and adding it again starts it from nothing. The app reads the list
again as it starts and whenever it comes back to the front, and lets go of
every video whose folder is gone, closing its workspace if it was open, see
`letGoOfRemoved` in `library.go`. It tells a deleted folder by a mark the
folder carries from the moment the video is added, `added.json`, and not by
the folder alone, because the app writes a folder back whenever it is asked
about the video, the chosen clip and the waveform among other things: Tim
deleted a folder, opened the video, and a folder was there again with
nothing of its own in it, plan row 2.247. A folder written back like that is
deleted again once the video is let go. Videos added before the mark get it
once, as the app starts, if their folder holds work of its own. A video deleted
together with its folder leaves the list the same way, plan row 2.250. Only
a video whose own folder is gone too, on a drive taken out or in a folder
moved away, keeps its place and says File not found until it is back, see
`deletedOutside` in `settings.go`. With no folder beside it there is nothing to
keep or to throw away, so the box says so and offers one **Remove**.

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

### Which files are videos

This is the one place in the docs that says which files the app takes as
videos, and `VideoExtensions` in `engine/episode.go` is the one place in
the code. Add's box, a drop on the app, `ScanFolder` and the words the
app shows on the Add button and the page for no video all read that list.
`TestVideosAreNamedOnce` in `engine/videos_test.go` fails when this
section and the list disagree, or when any other doc, code or interface
text names the endings itself.

A file is added as a video when all of this holds:

| Check | Where |
| --- | --- |
| Its name ends in `.mp4`, `.mov`, `.m4v` or `.mkv`, in capitals or not | `IsVideo` in `engine/episode.go` |
| The decoder reads a first frame from it, or says nothing for 30 s | `undecodable` in `cmd/framefairy-app/library.go` |
| Neither it nor the work folder beside it is a link | `linkedEpisode` in `cmd/framefairy-app/settings.go` |
| No video in the list would share its work folder, as `ep.mp4` and `ep.mov` would | `AddEpisodes` in `cmd/framefairy-app/settings.go` |

The ending decides first. A file with another ending is left out by its
name, whatever it holds, and a file with one of these endings that holds
no picture the decoder can read is left out with the decoder's reason.
A decoder that gives no frame and no reason within 30 s says nothing
about the file, only about a busy or hung machine, so the video is
taken and the log keeps a warning.

### The bar

A bar runs across the top of the app. It holds the close, minimise and zoom
buttons on macOS, it is what the app is dragged by, and it says what is on
screen: the name of the episode, or **Settings**, **Updates** or **Acknowledgements**, and in the report window **Report a Problem**. No screen
writes its own name below it, and the sidebar opens under it, so the name is
always there to read.

On macOS the bar **is** the title bar. macOS lays that out and centres its
three buttons in it, and nothing an app can set moves them by a pixel: the
height the app asks macOS to leave empty only says how far down a drag
still moves the app. So the app takes the height it was given rather than
asking for one, and then the buttons are on the bar's middle because the
bar is what they were centred in. Everywhere else the system draws its own
title bar above the app and this one is an ordinary header.

The Go side measures and the stylesheet lays out. `FrameFairy.Chrome` answers
three numbers in whole pixels, the title bar's height and the left and
right edge of those buttons, and the interface is told again
on its window's event whenever the answer changes, `chrome` for the app's
window and `chrome-report` for the report window, each measured on its own: a resize, either way
through fullscreen, another display, another scale, light or dark, back
from the Dock. Nothing is ever moved, so there is nothing that can snap
back, and nothing is measured on the page. The three numbers land on the
shell as `--bar-h`, `--lights-l` and `--lights-r`, and the bar lays itself
out from them. `--bar-h` is in `app.css` as well, which is
what the first frame uses and what every other system keeps. In native
fullscreen there is no title bar and no buttons, so the height stays as it
was and the name moves to the left edge rather than the bar collapsing.

The name of what is on screen, and its label beside it, are centred in
the bar a person sees. That is not the bar's middle: macOS draws the edge
of the window over the top 2 points of the page, and the line at the foot
takes the last point, so the name's box runs from under the edge to the
line. macOS centres its buttons under the edge alone, half a point lower,
and AppKit's frame for the close button says the middle of the whole bar,
so neither is a measure for the name. Away from the buttons, on other systems and in fullscreen, the
name is centred in the whole bar. The name's box is
trimmed to the capitals and the baseline, so it is the letters that are
centred and not the space a line of text reserves around them, and then it
is padded back out so a name with a tail below the baseline, `YouTube.mp4`,
keeps it. A name too long for the bar ends in an ellipsis.

The colour of the app's window itself is the bar's colour, `--ink-1`. It
is only ever seen where the page does not paint, which on macOS 26 is the
sliver between its rounded corner and the webview's, and at the top that
sliver is inside the bar.

### The workspace

Selecting an episode opens its workspace. It is three columns, the settings
on the left, the video preview in the middle and the clips on the right, with
the clip up close under all three. It fits the app without scrolling.

The video preview plays the episode from its own queue of frames, see
`frontend/src/lib/frames/`. The app reads the samples of the picture and
the sound straight from the episode file, a range at a time, decodes them
with the system's decoders a few frames ahead of the playhead, draws each
frame onto one canvas and hands the sound to the sound card, which keeps
the time. Each frame is drawn when the sound heard reaches where that
frame begins, through the clip's pieces, whatever moment the play began
at. A cut is frames never queued: the last frame before a cut is followed
straight by the first frame after it, both always drawn, and the sound
meets at the sample, faded over 15 ms either side the way the render
fades it. So a clip plays in the video preview the way it plays in the
short.

Between cuts the sound is one stream, as the decoder puts it out, and its
samples are counted the way any player counts them: the stamps in the
file only say where the stream starts. They need not say more to the
sample. Every AAC packet decodes to 1024 samples, but Tim's `start.mp4`
says its packets last 1008, 1056 and 1008 in turn, and a clock of 600
ticks a second, QuickTime's, says where a packet starts only to the
nearest 80 samples at 48 kHz. Placed by its stamp, a packet missed the
one before by 16 to 40 samples at most packets, and every miss was a
click, about 30 a second, heard as a crackle over the whole play. So
only the first packet out of a run goes where its stamp says, and one
after a packet that came out as nothing, where the stream itself has a
gap.

Paused, the picture is the frame that holds the playhead, exactly, wherever
the playhead was put: a click, a drag, a step of an arrow key or a word.
A moment a millisecond or less before a frame's start is in that frame,
because every moment the app keeps is kept to the millisecond. Every edge
of a clip is on the start of a frame: the engine puts it there as the
clip enters the plan, from a search, by hand or from a gesture,
`PieceOnFrames` in `engine/render.go`, and the render cuts on that frame.
So on a clip edge the frame that holds the playhead is the frame the
short has there, and both sides are held to `lib/frame.cases.json`. A
piece plays and ends with the frame that holds the moment a frame before
its end, the short's last. A clip picked shows the short's first frame,
and the playhead on the clip's end shows its last.
The play from there is already prepared, its first frames and its first
sound decoded, so the space bar starts it at once rather than decoding the
same frames again. The picture moves about 80 ms after the space bar in
the harness, most of it the sound card's own latency and the first
frame's own time on screen, since the picture waits for the sound to be
heard. While it plays, the playhead is the sound being heard, so it
jumps a cut with the sound and stands in the frame on screen. It moves
once a frame of the video, not on every refresh of the screen, because
everything that follows it is drawn by its frame: the mark on the clip
timeline, the time field and the captions. Moved on every refresh, the
whole workspace worked itself out again 120 times a second on a screen
of 120 Hz for a video of 25 frames a second, issue #446. It never goes
back while playing. A press while it plays, on the clip timeline or the range picker, holds the
play under the hand, like a nail, for as long as the button is down: the
playhead goes to the press and stays there, follows the hand once it
moves, and the play goes on from where the hand lets go, the clip's or
the episode's by where that is, across the clip's edge as well. The
button says Pause the whole time, and only the sound is held, the way a
pause holds it. Before, the play went on from the press at once and ran
away from the hand before a drag could start. Only the space bar and a click on the
clip timeline change a play. A clip's pieces changing while it plays, a
cut made, moved or put back, an edge trimmed or a step undone, moves no
playhead and changes nothing that plays: the play goes on with the pieces
it started with, and the next play, or a click, plays the clip as it is
now. A drag on a cut's edge used to start the play again on every step,
which stuttered. Loop switched on or off changes only where the play
ends, never what plays: the picture stops at that end and the sound
stops there too, in the silence the clip fades out to, however often L
is pressed on the way. The end is that of the time through the sound
is in, not the playhead: the playhead moves on with the page's frames,
so on a busy machine L can come after the sound has crossed the clip's
end and before the playhead has, and the play then ends after one more
time through rather than a moment into the clip's start.

The sound goes on while the app is behind another window. macOS stops the
page's animation frames there, and the frames are what fed the sound, a
second ahead, so it ran out a second after the app was covered and came
back with the first frame the app got. Now the sound is fed 3 s ahead the
moment the page says it is hidden, and while no frame comes a timer goes on
feeding it, since a timer behind another window may run only once a second.
The picture catches up with the first frame.

WebKit counts a window that other windows cover as a hidden page and
stops drawing it. Tim saw part of the app flicker when Mission Control
showed it after a while behind other apps, #536, and a page WebKit had to
draw again as it was shown is the likeliest cause, not proven yet. The
app's web view is told to keep its page drawn while it is covered,
`keepDrawing` in `cmd/framefairy-app/sight.go`. WebKit then asks only
whether the app is on screen at all, so a minimised or hidden app is
still a hidden page, and an app on another Space keeps drawing. The log
says each time the app goes out of sight and comes back, with how long it
was away, see [LOGGING.md](LOGGING.md).

A clip played from a playhead that stands in one of its cuts plays from
where that cut ends, see `playFrom` in `lib/playhead.ts`: the queue is
given the clip's pieces and a seek to the end of the cut before it plays,
so not one frame or sample of the cut is queued. A double-click that cuts
a part out leaves the playhead in it, and the space bar then played the
part just cut, which the render does not have. The playback walk found it
while 2.121 fixed it, and holds it fixed: it records every frame the queue
draws while a clip plays and fails on one from inside a cut.

The canvas is as many device pixels as the stylesheet makes it, and the
frame is drawn into it in its own shape, so a 4K episode is never kept at
its full size. Opening another episode closes the queue of the one before,
its decoders and its sound card with it.

The queue reads the index of MP4 and MOV, and the episode's decoder
decodes whatever picture and sound ffmpeg does.

The sound is decoded on the Go side, by ffmpeg, plan row 2.153: the same
sound the render and the transcript are made of, whatever decoder the
webview has. Tim heard picture and sound out of step in a file from
DaVinci Resolve, which keeps the encoder's 44 ms of silence at the front
of its sound and holds the picture back by an empty edit of the same 44
ms. ffmpeg and the queue's own reading of the file agreed on all of it, so
what was left was the Mac's own AAC decoder in the webview, and rather than
measure what it does with those 44 ms, the queue no longer uses it.
`GoSound` in `lib/frames/queue.ts` answers the calls of the browser's
`AudioDecoder`: a run of packets is one stream of `/frames/sound`, opened
where its first packet lies, and each packet after it is the next of its
samples, the way ffmpeg reads a piece of a render, so a flush ends a run
and the packets after it start a stream of their own. The page does not
read the sound in the file at all, plain sound in a MOV included, which
the page once turned into samples itself. The streams come from the
episode's decoder, `EpisodeFrames.Sound`, cursors of sound beside the
cursors of picture, two kept and one opened as the decoder starts, so a
play or a jump starts no ffmpeg for its sound either. A cursor reads the
sound by the rule the ffmpeg program reads it by for the render and the
transcript, `soundSeek` in `engine/levels.go`: seeked a fifth of a second
early and cut at the moment by the samples' own timestamps, or read from
the file's start where that would be before the picture starts.
`TestTheEpisodesDecoderHearsWhatFFmpegDoes` holds the two to the sample.
Read from the start, it does not seek, the way the ffmpeg program does
not: a seek there lost what the file says to skip of its first packet,
and the sound came 1024 samples late.
The Go side closes a stream nobody has read from for 20 seconds, and a
pause holds the play with its stream, so a stream that answers 404 is
opened again from the moment after the last that came: a play paused
longer said the sound had stopped decoding, Tim found. A stream that
fails anyway is dropped with the packet it failed on, and the next packet
opens one of its own. Kept, no sound came after it, a play waits for its
first sound, and the space bar seemed to do nothing. A stream the page
has closed is never opened again, and the Go side counts streams of sound
apart from streams of frames, so the seeks of a play never close the
frames the video preview draws.

A frame the page makes from what the Go side decoded is in full range,
and says so, with the picture's light as the Go side reads it from the
file's transfer tag, see `framewire.Picture`.
WebKit draws a frame made from a buffer as full range whatever the frame
says: Tim's `start.mp4`, HEVC with 10-bit colour in video range, which
the Go side decodes for WebKit, came out pale, its black a grey of 17
where QuickTime shows 1. Labelled video range it stayed so. Now
VideoToolbox is asked for full range, and ffmpeg too, `out_range=pc`, so
what WebKit assumes is what the frame holds. Every file ffmpeg reads
plays, whatever its container, Matroska and MPEG-TS among them, because
the page reads nothing of the file, see step 5 of
[VIDEO-PREVIEW.md](VIDEO-PREVIEW.md). A file the episode's decoder cannot
read, a codec it has no decoder for, says so in one sentence where the
picture would be, with the reason, and nothing else in the workspace
changes. A sound it cannot decode plays the picture without it and says
so at the foot of the picture.

Every file's picture is decoded by ffmpeg on the Go side, the first step
of [VIDEO-PREVIEW.md](VIDEO-PREVIEW.md), plan row 2.156, by the episode's
decoder, `framefairy-frames`, which make puts beside the app and the app
always ships: one program per episode on ffmpeg's libraries, which keeps
the file open and its decoders ready, so a jump moves a cursor that is
open instead of starting ffmpeg. An app without it says so the way it
says a missing ffmpeg, and there is no second way to the frames. The frame
queue holds the decoder of its episode from the moment the episode opens
in the video preview, `/frames/hold`, until it closes, `/frames/release`:
the decoder starts at once and opens its cursors on the file before any
frame is asked for, and a held decoder is never closed for standing
unused, so a click after an hour paused is as quick as the first. One
nobody holds is closed after a minute unused, and at once when more than
two stand unheld, the one used longest ago first, `decodersSpare` in
`frames.go`: every workspace kept holds its decoder while it is awake, and
the ten built as the app starts left ten programs running for a minute.
The webview's
decoders and the Mac's own below are still in the code, unused, until the
step that removes them. Each stream of frames says, with its first frame,
where its time went, in milliseconds from when it was asked for: until
the decoder had the request, at once where it ran and after starting it
where it did not, until it was at the place asked for, which it says
itself, and until the first frame was out, then 1 where reaching the
place meant opening the file and 0 where a cursor that had it was moved,
and last 1 where a cursor read on to the place from where it stood,
without a seek. Such a stream hands over no key frame first, which the
app's log says, issue #511.
That is the header `X-Frames-Times`, which `AppFrames` keeps in
`stats.opens`, for a probe to tell where the time of a jump went, see
[VIDEO-PREVIEW.md](VIDEO-PREVIEW.md), Speed. A frame asked for a place the hand has left is withdrawn when
the queue's decoder is reset for the newer place, and one new stream is
on its way at a time: while one is still coming for a place left behind,
the newest place waits and goes as soon as it has come, so a drag over
the clip timeline shows frames as fast as they can be made and starts no
ffmpeg for every place it passes. Every move of the hand is a seek, which
withdraws the frame the seek before asked for, and a frame takes longer to
come than the hand takes to move. A frame that comes for an ask withdrawn
on the way is put up while the frame for where the hand is now is still
coming, unless a frame asked for later is on screen already, `passing` in
`lib/frames/queue.ts`. Thrown away, as it was, the picture stood still
for a whole drag on a busy machine, found by Tim on an HEVC episode at
1080p. The decoder drops the frames before the moment asked for as they
come out of decoding, before they are scaled. A stream the Go side stops, to make
room or after standing unused, says `X-Frames-Closed`, and what waited on
it goes to another stream. Taken for the end of the episode, it settled
for a frame near the one wanted, and after a drag Tim saw the wrong frame
and "the app's decoder did not answer". Before, a system could say it decodes a file and then fail on
it: WebKit says yes to HEVC with 10-bit colour and its decoder then fails
on the first frame, "Decoder failure". For such a file, and for one the
system said no to, the Go side decoded the picture, plan row 2.141. The queue
asks its picture decoder the same things either way: `AppPictures` in
`lib/frames/app.ts` answers the calls of a `VideoDecoder`, like
`GoSound` stands in for the sound decoder, so drawing, the clock, cuts
and the walks are the same. Only the frames the queue will draw are asked
for, the file's picture is not read by the page at all, and like a
decoder it puts them out in the order they are shown, not the order they
were fed: fed in the order they decode, a run's B-frames came too early
and its first frames too late, and 72 frames of a play of three seconds
were dropped as late until it did.

There is one way to the frames, step 3 of
[VIDEO-PREVIEW.md](VIDEO-PREVIEW.md). Until then the webview's own
decoders, the Mac's VideoToolbox through cgo, `NativePictures` and
`engine.Pictures` with `/frames/native` and `/frames/decode`, plain sound
read and turned into samples by the page, and the choice between them
were in the code as well, each with its own way of failing. They are
removed, and the page reads nothing of the file at all, see step 5 of
[VIDEO-PREVIEW.md](VIDEO-PREVIEW.md). The frames come in streams the page
opens and pulls from, each on the grid of frames the engine named, which
the page hands back as it opens one:

    /frames/open?path=&from=&w=&h=&grid=   {"id": ...}
    /frames/read?id=&n=&skip=        up to n frames
    /frames/close?id=

A stream is a cursor of the episode's decoder, on the graphics chip where
it decodes the file, from the key frame before `from`, and hands over every
frame from `from` on, scaled to `w` by `h` in 8-bit I420, each after the
moment it starts at as a float64 in 8 bytes. ffmpeg decodes a frame ahead
of what was pulled and waits: WebKit takes whatever a response writes
without pushing back, so one long response would have let it decode the
whole episode into memory while the page stood paused. A stream nobody
pulls from for 20 seconds is closed, and at most six are open. It only
reads a file of an episode in the library.

What makes it quick, since a new stream costs moving a cursor of the
episode's decoder and decoding from the key frame before:

- A stream that will reach a frame within a second and a half is waited
  for rather than a new one started, and the frames it passes on the way
  are dropped on the Go side, `skip`, not sent. So the arrow keys stepping
  on, and a paused play going on, cost a frame each.
- The frames shown last are kept, 40 of them and never more than 192 MB,
  so stepping back or a still close by costs nothing.
- A frame is decoded at the size of the canvas and no larger than the
  file's own.
- The queue's second decoder asks for the piece after a cut while the
  first plays, so its stream is open before it is needed.
- On the Mac the graphics chip decodes, and the frame is made smaller
  and turned into 10-bit colours on the processor by the one chain every
  system uses, see
  [VIDEO-PREVIEW.md](VIDEO-PREVIEW.md).
- A pull is read in a Worker, `lib/frames/pull.worker.ts`, each frame
  straight into a buffer of its own as it arrives and kept as it is, the
  app's own frame, `lib/frames/picture.ts`, so nothing on the page waits
  for it. Where a Worker cannot reach the Go side, the same code runs on
  the page.
- The canvas is a WebGPU canvas in the extended mode wherever the webview
  has WebGPU, so the video preview can show HDR, and a 2D canvas where it
  has none, `lib/frames/screen.ts`. A frame's bytes are written to the
  GPU as they are, and the look of standard video, or the light of HDR,
  is put on there in floats, `lib/frames/light.ts`.

Measured on the cloud machine, four cores and no system decoder, with the
bridge's HEVC 10-bit episode at 320 by 180 in Chromium: a click on the
clip timeline shows its frame 65 to 115 ms later, a frame from a stream
already open comes 1 to 5 ms after it is asked for, a play of three
seconds draws every frame with none late, and the arrow keys show the next
frame and the one before within a few milliseconds. The prototype,
measured before the queue used it, with an HEVC 10-bit episode at 1920 by
1080 read as fast as it came and drawn on a canvas:

| Size | First frame after a jump | Frames a second after it | MB a second |
| ---: | ---: | ---: | ---: |
|  640 by 360 | 266 to 277 ms | 122 to 141 |  27 to 35 |
| 1280 by 720 | 145 to 337 ms |  48 to 105 |  63 to 109 |
| 1920 by 1080 | 174 to 288 ms |  66 to 89 | 187 to 223 |

A podcast needs 25 or 30 a second.

It used to be a `<video>` element, and everything about it was a
workaround. The element answers a seek before its frame is on screen,
answers a paused seek with the start of the frame it shows, runs its clock
ahead of the picture as playing starts and then stands, drops a seek while
the machine is busy, and has no clock at all before it has read the file.
A still read by the engine stood in for the picture while it caught up,
and a cut was a seek in the middle of playing, which held the last frame
before the cut for as long as the seek took. The frame queue has none of
that to work around, because the app decides every frame it draws.

**The space bar plays the clip or the video, and the playhead says which.**
The playhead is either on the chosen clip or on the video, and the gesture
that put it there decides, never a clock. Picking a clip puts it
on the clip, at its start. So does a click on the clip's start edge, a
trim, a click on a caption, and a click, a drag or a step into any frame
of the clip, cuts included, from the frame that holds its start. A click
on the end edge, a landing in the frame that holds the moment a frame
before its end, and the clip playing to its end, leave it on the clip at
its end. Any frame before or after the clip is on the video, and so is
having no clip chosen. It goes by frames and not by seconds because what
is on screen is a frame: a clip's start kept to the millisecond can lie a
hair before its frame, and a step onto that frame is on the clip. A step
taken while the episode plays on the video is the one exception: an arrow,
with Shift or without, skips through the episode and leaves the playhead
on the video, through the clip and its cuts, so a hand that skips ahead
through a dimmed clip does not light it up. Paused, or on the clip, a step
lands where its frame says.

On the clip, the space bar plays the clip from the playhead: its cuts are
jumped and it stops at its end, or with loop on goes back to its start.
From inside a cut it plays from the end of that cut, and from the clip's
end it starts the clip over, the way QuickTime starts over at the end of a
video. On the video, the space bar plays the episode straight on from the
playhead, through the chosen clip and its cuts, with no jump and no stop,
so any part of the episode can be heard with a clip chosen. Playing through
the clip leaves the playhead on the video, and loop changes nothing there.
While the playhead is on the video the chosen clip is dimmed as one thing,
wherever it is drawn: its mark on the range picker, and on the clip
timeline everything drawn for it, its frame, its pieces, its cuts and their
edges, its trim edges, its caption blocks and their edges and its
thumbnails. So it shows that the clip's rules are not in play. It is one
rule, `.chosen.dim` with `.clipmark.selected.dim` in `app.css`, at the
brightness of a button that cannot be pressed, `--dimmed`, and it changes
in the frame the playhead does. With the hand on the clip somewhere else,
its card or its mark, it is lit whole. The video preview then shows the
video alone, with no crop frame, no shade and no captions, even while the
episode plays on through the clip's own moments: what plays is the
episode, not the short. On the clip, the crop frame and the captions are
back in the same frame.

It used to be measured: the clip played when the playhead was within half
a frame of its start. The paused video element on the Mac answered with
where the frame it showed began, up to a frame before the moment it was
sent to, so a clip just picked measured as before its own start, and the
space bar played the episode straight through its cuts and past its end.
See the rules in `frontend/src/lib/playhead.ts`.

Opening an episode that already has clips opens on one of them: the clip it
was last worked on, or the first one where it has never been opened. The
playhead goes to the start of that clip, because that is what choosing a
clip does. Where the playhead stood when the app was closed is not kept, no
editor keeps that, and the start of a clip is a place that means something.
Which clip it was lives in the episode's own folder, so it goes when the
folder goes. The video preview draws nothing until the workspace knows
where it opens, `opening` in `Player.svelte`, so the first frame on screen
is the clip's. It drew the episode's first frames as it opened and then
left them for the clip, a flicker Tim saw on every episode. The queue
opens the file meanwhile, so the clip's frame comes no later. Where the
playhead opens is the first thing the workspace works out, `openEpisode`
in `Episode.svelte`: the episode, its picture, its clips and the clip last
worked on are asked at once, and the playhead is on the clip before
anything else is asked, how far the episode was searched, the room for
windows and the window, none of which the video preview needs. Those
were asked first, one after another, with the video preview waiting. The
walk `an episode opens on its clip, with no frame of its start on the
way` reads every frame drawn from the click on, and where the playhead
stood as the window was asked, which only the opening asks. The coverage
and the room are read again whenever work elsewhere ends, so they cannot
say what came first.

**Picking another episode changes the workspace in one frame.** The new
episode's workspace is built behind the one on screen, unseen and inert,
in the same place, and the two change places once the new one has the
episode on it: its window on the range picker, a frame on the canvas, its
clip timeline drawn and the captions of the clip it opened on, `ready` in
`Episode.svelte`. A second at most, so a slow disk never keeps the old
episode up. Meanwhile neither takes clicks or keys, `asleep` in
`lib/keys.ts`, and an episode left while it plays goes quiet at once. The
workspace was torn down and built again in front of the person, empty for
a moment and then a part at a time as each answer came in, and Tim saw
that as a flicker on every episode picked. It is the pending state of
React's transitions, which keep what is on screen until what replaces it
is ready rather than going back to nothing first.

**The workspaces of the last ten episodes opened are kept**, `KEPT` in
`App.svelte`, so going back to one is one frame, from another episode and
from the settings alike. Only the one in front is awake. A kept one is
taken out of the page, `shelf` in `App.svelte`, so a resize of the app
lays out one workspace and nothing that looks the workspace up by the
document finds a second one, and it keeps what is drawn on its canvases
and how far its clip list was scrolled. Its video preview lets go of
everything costly, `sleeping` in `Player.svelte`:
the frame queue is closed, with its decoder on the Go side, the frames it
kept and its sound card, and only what it played and the frame on the
canvas stay. Waking, a new queue starts from what it played, in the task
after the frame that shows the workspace again, so starting a sound card
and a decoder does not hold that frame back. The new queue leaves the
frame on the canvas alone until its own first frame replaces it: it does
not set the canvas to the size it already has, which clears it, and it
draws nothing again for the captions before it has a frame of its own.
Each of those showed black for a frame or more. What it plays is the frames
the engine names for the episode and its sound, from the source, see
`lib/frames/movie.ts`, and nothing of the file is read by the interface.
Nothing of the file is taken from the webview's cache, which took a file
written long ago as unchanged for minutes. The workspace reads the
episode and the settings again behind what it already shows. Asleep, it
reads nothing by itself. An
eleventh episode opened lets go of the one opened longest ago. Asleep,
a workspace is its page, its canvases and the frames it plays, about 10 MB,
so ten is about 100 MB. As the app starts, it opens on the episode that
was open when it was left, the way a Mac app comes back with the document
it had open, and the workspaces of the episodes opened before it are
built one at a time behind it. `Opened` on the Go side lists them by when
each was last put on screen, which `OpenEpisode` notes in the episode's
`chosen.json`. An episode opened only before that was noted comes after
the rest, by when its `chosen.json` was last written. The workspaces are
built one at a
time behind whatever is on screen and put to sleep once each has its
frame, so the first episode picked is on screen in one frame too. Building a
workspace from nothing took 70 to 215 ms in the walks' browser for an
episode of an hour, a chain of calls each waiting on the one before, and
longer on the Mac. Going back to a kept one is the layout of the
workspace put back and nothing else, 17 to 30 ms in the same browser, and
it has its picture, its clip timeline and its captions in that frame.
Only a picture is the video preview being ready: the sound stopping before
the first frame once counted, and a workspace built as the app started
went to sleep with nothing on its canvas.

An episode that is not kept is built the moment it is picked, and the
parts of that are started together rather than one after another. The
episode's decoder is asked for as the workspace starts to build,
`holdAhead` in `lib/frames/queue.ts`, rather than once the workspace
knows what the episode is. The clip timeline asks for its words and its waveform
once the episode has answered, and never twice for the same view. The
Go side keeps the transcripts of the last ten episodes read, `keptReads`
in `words.go`, and reads one once however many calls ask for it at the
same moment: a workspace opening asked for the same transcript up to
seven times, and each read it from the file. The longest window the range
picker may draw is worked out without a search from every line, which
took forty milliseconds of an hour's transcript in the frame the
workspace opened in, `anywhere` in `lib/room.ts`. An hour's episode not
kept is on screen in 200 ms in the walks' browser, from 240, and what is
left is the workspace itself being laid out.

Putting the playhead somewhere on the range picker is asking to look there,
so the clip timeline goes there too. With a clip chosen that counts as moving
the view by hand, and the view stays where it was put. **The playhead is
dragged the same way on both**: a press anywhere on the track or on the
playhead takes hold of it, and the video preview follows the hand,
one seek a frame, until it lets go. The range picker only went there once
the hand let go, and Tim asked for it to behave like the clip timeline. It
is one function, `scrub` in `frontend/src/lib/scrub.ts`, used by both. The
playhead is taken hold of by its line, a few pixels either side, and on both tracks it shows the arrows left and right under
the pointer, the way a clip's edges do. The empty track shows the hand.
The two differed: the clip timeline seemed to show the arrows on the
playhead only because the playhead so often stands on the chosen clip's
edge. **The playhead looks the same on both**, one rule, `.playhead` in
`app.css`: a line and nothing else, running 12 px past the track's top and
bottom border, 2.234. On the range picker it had a head above the track
and on the clip timeline a line alone, and Tim asked for the two to match.

The **crosshair** in the row under the timeline goes to the playhead, always,
clip or no clip, and puts it in the middle of the view, because the reason to
ask is to see where it is. Going back to the clip is what clicking the clip in
the list does, the one already chosen included, so the crosshair does not do it
as well: a control that went to the clip with one chosen and to the playhead
without could not be relied on for either.

The two sides beside the picture are measured from the app's own edges,
so the picture stands in the middle of the app: on the left the rail, a
space and the settings, on the right the clips and the edge. Each side is
a fifth of the app's width, from the width alone, so a taller or shorter
app moves neither, a wider one widens both and a narrower one gives the
picture the room, and the settings never go under 204, where their names
and fields still fit whole. So the clip list is 32 pixels wider than the
settings, the rail and a space less the edge, and on an app of 960 the
settings are 204, the picture 416 and the clips 236. The two sides are
rounded once together and then split, so they are equal or a pixel apart
and the middle column only ever moves one way. The open sidebar is the
left side, so it covers the settings column and nothing else.

On the way here the sides were once what the picture left, so a shorter
app, with a narrower picture, made them wider, unequal, and on a half
pixel every other step, so the settings fields shook. Then they were the
same width as each other and 280 at least, which made no room for the
rail and squeezed the picture on a small app to 296.

The video preview fills the middle column: a black viewer as wide as the
column, with the picture in it as big as the room allows in the shape of
the episode. Where the width holds the picture in, it is the column's
width exactly. Where the height does, on a short app, it stands in the
middle of the viewer with black either side, the way a video player
shows a picture of another shape. The crop and the captions stand on the
picture, not the viewer. Once the picture is as wide as it may be, the
height left over goes to the two tracks: the clip timeline grows and the
range picker stays half of it, to within a pixel, so nothing is left empty
at the foot of the app.

**Every size moves only the way the edge of the app moves.** Each size is
a whole number of pixels, rounded down once, and what rounding leaves goes
to the size worked out after it, never back to one before: across, the
two side columns together first, then each half of them, then the middle
column the rest. Down, the picture first, from the app alone, then the
range picker a third of what is left and the clip timeline the rest. A
part of a sum that is whole by its own arithmetic is rounded to the
nearest pixel, because the browser's arithmetic can make 463 into
462.99999. The clip timeline used to be rounded to an
even number first, so the range picker could be exactly half, and the
picture took the rest. That rest changes in steps of three pixels, so as
the app was dragged smaller the picture shrank a pixel, another, and grew
three, and the range picker under it and the clip list beside it went with
it: the shaking Tim recorded. In the harness, shrinking the app a pixel at
a time, the picture, the range picker, the clip timeline and the clip list
moved back on 48 steps of 150, and now on none. The picture growing into
that rest also pushed the clip list 1 to 4 pixels past the edge of the
app.

The two tracks are one episode seen from two distances, so they lie on one
floor, `--well`, a step darker than the workspace around them and never
black, the way the editors people know draw a timeline. What is chosen on
them, the window on the range picker and the clip on the clip timeline, is
marked by its frame and the wash in it, and nothing around it is darkened.
The range picker used to darken everything outside the window while a
search ran, so it changed colour as searches came and went and never
matched the clip timeline. The video preview still darkens what lies
outside the crop while the playhead is in a clip, because that part is cut
from the short. Anywhere else, in a cut or between clips, nothing is laid
over the video preview at all. The crop used to stay there as a grey
dashed frame with the shade round it, which laid a short over a part of
the episode that is in none.

All of that is one expression in the stylesheet, worked out from the size
of the app itself and the tokens in `app.css`. The one thing it cannot
know comes in as a custom property, the shape of the episode, and it does
not change because the app was resized. So dragging the edge of the app costs no JavaScript at all and the
workspace keeps up with the edge instead of arriving a frame behind it. The waveform is the
one thing still told its size in pixels, because a canvas has to be, and
nothing is laid out from the answer.

Two things are called what they are, here and everywhere else. The slim strip
under the video preview, the whole episode at a glance, is the **range
picker**. The waveform under the whole workspace, the episode up close, is the
**clip timeline**.

Nothing explains itself in a line of text that is always on screen. Every
control says what it is for when the pointer rests on it, and the small info
marks open a bubble that says more, on hover or on a click. A bubble is a few
sentences, never an essay, and it hangs from the page rather than from the
area it belongs to, so nothing clips it and nothing lies over it. An area
with an info mark carries no tooltip of its own: one explanation, in one
place.

**Every word of a bubble can be read, always.** It hangs under its mark,
over it, or beside it, the first of those that holds the whole of it
inside the app, and a long text that fits nowhere at the narrow width is
made wider, which makes it shorter. It never covers its own mark. Only an
app too small for it at any width gets a bubble as tall as the app that
scrolls. The rule is `placeBubble` in `frontend/src/lib/bubble.ts`, with
tests that put a mark everywhere in apps of several sizes. The clip
timeline's bubble used to fit neither under nor over its mark, went under
it anyway and ran off the foot of the app. Moving into the bubble keeps it
open: an eight pixel bridge on the side facing the mark carries the
pointer across the gap. It used to sit inside a bubble that clipped its
overflow, so over the mark the bridge never worked, and the range picker's
bubble scrolled by those eight pixels.

- **The transcription is the first step of a search, and nothing else.**
  A search is one job on the Go side, see [JOBS.md](JOBS.md): it hears the
  episode from where the transcript ends to the end of its window, stops
  exactly on its edge, and then finds. Adding a video asks for its first
  search, of its first window, so the Go side starts it and the
  workspace only shows it. Nothing carries the transcription on through
  the rest of the episode afterwards by itself. So there is nothing to
  control about it in the workspace but the search: **Search** starts it
  and **Cancel** stops it, and what was heard stays, so the next search
  goes on from there. Hearing the whole video is asked for in its menu in
  the sidebar, **Transcribe video**, see above, and a search of the
  video stops it. While another
  episode's search finds its clips, a search that hears waits and carries
  on after, because the two models want the same memory.
- **What is not there yet says so by waiting.** The part of the clip
  timeline the transcript has not reached and the rows the clip list will
  have are places waiting to be filled: the shimmer passes over them, the
  same light the window on the range picker shows while clips are being
  found for it. On the clip timeline the waveform is there for the whole
  episode within seconds of adding it, see below, and nothing is drawn
  over it for the part not heard yet: a grey band across the middle,
  where the captions stand, looked like a hole in the sound rather than
  words still to come, and said nothing the clip cards do not. The
  range picker carries none of the reading either. It once did, the part
  not transcribed darker and the part being heard filling, but how far
  the work has come is said on the clip cards, the transcribing and the
  finding alike, and the range picker said only the first half of it.
  Tim had it taken away: while clips are found the window breathes and
  the motes of work in hand rise through it. What it does show, always,
  is where the transcript has not reached yet, the dots of a part not
  transcribed, see What was not transcribed is said below. That is no
  progress, it is where a search has to transcribe first. Tim picked
  dots on what is not transcribed over dots on what is, so a video
  transcribed to its end is a plain range picker, issue #432.

  **The width it is measured at is the width it grows from.** Which way a
  list grows is the placement's to decide, and it decides from the width
  it measures, before the stylesheet has run its rules. A list told to be
  the trigger's width and then widened to fit its longest name is measured
  first and widened after, so the placement hangs the trigger's width on
  the right edge and every pixel the list then gains goes out the other
  way, over the edge every field in that column ends on. `max-content` is
  the width before anything is measured, so the placement measures what it
  will get.

  A name longer than the app has room for is the one case a list cannot
  grow into, and the whole of a name is in the row's title either way. The row under the pointer and the row the
  keyboard is on are the same row, in `--ink-3`, and what is chosen is in
  the accent with a tick that keeps its place whether or not it is there.
  The trigger keeps room for the longest thing the list can say, so a row
  never changes width as it is used. The one exception is a list that
  stands alone at the end of a row, where nothing beside it moves, and
  whose short names would sit in a box made for the longest: `fit` makes
  its trigger as wide as what it says now. The channel on the Updates page
  is one. It is built on
  [bits-ui](https://bits-ui.com), the headless half of shadcn-svelte, which
  brings the keyboard, the roles, the focus, the typeahead and the floating
  placement, and no look at all.
- **Correcting a word:** click it in the caption box, in the picture, where
  a short will show it. A word under the pointer is framed in the accent and
  the pointer becomes a caret, so what can be corrected says so without a
  word of instruction. The caret lands where the hand clicked, letters are
  typed and taken out from there, Enter saves and Escape leaves the word as
  it was. Clicking a word stops the picture, because a caption that moved on
  under the caret would leave the hand correcting a word that is no longer
  there. Playing the other way round ends the correction: a click on the
  video preview or anything else that plays saves the word the way Enter
  does, takes the caret and any highlighted letters out of it, and lets the
  captions follow the picture again. A click on the video preview lands on
  the crop frame, which refuses the pointer so it can be dragged, so the
  word used to keep the caret while the video played. Moving the playhead
  any other way ends it the same way, since the word is corrected at the
  moment it was opened at: a click on a thumbnail's camera or a keyframe's
  diamond takes no focus, and the word used to stay open with the caption
  held on the old moment while the frame moved on. The frame and the caret belong to the interface and not to the
  render: nothing of them is ever burned into a short, and **the frame is
  all the interface adds**. A word being corrected takes no background of
  its own. One would read as a second highlight pill, in the app's colour,
  on a word nobody is speaking, and on the word that is being spoken it
  would take away the colour the render really burns in. The captions in the picture are drawn by libass
  from the short's own caption file, see [CAPTIONS.md](CAPTIONS.md), and
  the caption box over them draws nothing until a word is opened. Then it
  shows the words itself, the letters typed included, until the word is
  saved. Until then each word of the caption box stands where libass drew
  it, on the pill it draws behind the word at rest, so the frame round a
  word is the edge of its pill, with the pill's corners. The caption box
  holds the caption and the word of the frame on screen, not of the
  playhead, so while a clip plays the frame and the pill go on to the
  next word on the same frame. Shift and the arrows walk to the frame where
  the picture lights the next word, and the keyboard's frame comes with the
  pill, never before it.
  - **From the keyboard:** Shift and the arrows walk the playhead from
    word to word, and the word walked to wears the same frame the pointer
    puts on a word. Enter opens that word with the whole of it selected,
    the way Enter renames what is chosen in the Finder, so typing replaces
    it. Enter again saves it, the caret goes, and the frame stays on the
    word. Escape in an open word leaves it as it was and goes back to the
    frame. Enter opens the word in the frame, wherever the playhead is: a
    word clicked and let go of keeps the frame and leaves the playhead
    where it was, and Enter used to open the word at the playhead, a word
    with no frame on it. The frame is the caption holding the keyboard: anything else
    takes it away, a press of the pointer wherever it lands, a field
    getting the keyboard, Escape, any other key, the arrows without Shift
    among them, and the clip playing. It is the same with the highlight
    off, and that is what it is for there: with no pill nothing else says
    which word is spoken.
    The caret went on blinking in a word already saved, because WebKit
    keeps the selection in a field the keyboard has left, so a word let go
    of takes its selection with it.
    A walk past the last word of a clip lands on the first word of the
    next, and past the first word on the last of the one before, and that
    word wears the frame too. The walk could not catch that landing by
    itself, because the clip changes first with no words in it, and the
    playhead only reaches the word once the clip's captions have arrived,
    so the frame was lost on the way. The workspace now tells the video
    preview where it landed, see `keyAt` in `Player.svelte`.
  - **A colour picked with the pointer lets go of the keyboard**, so Shift
    and the arrows walk the words again straight after. Left with Escape,
    it lets go too.
  - **Escape lets go of the keyboard everywhere**, issue #370. It closes a
    list, a list to search or the colour pop-up and leaves no ring on its
    trigger, see `letGo` in `Pick.svelte`. In a field being typed in, the
    size of the captions, a setting, the report, it lets go of the field
    and keeps what was typed, see `escapeLetsGo` in `lib/keys.ts`. The
    plain list once kept the keyboard on its trigger and lit its ring on
    Escape, while the list of fonts did not, and Escape did nothing in a
    field. Whatever takes Escape for itself, a word being corrected or the
    report while it listens, keeps it.
  - **A button pressed with the pointer keeps no keyboard**, the way a
    button on the Mac does not, see `letButtonGo` in `App.svelte`. In
    Chromium, on Windows and in the preview, a clicked button kept it, and
    Enter then pressed the button again instead of opening a word: a click
    on Highlight and then Enter switched the highlight back on.
  - **A correction belongs to the clip it is typed in.** It is kept in the
    clip's plan entry, `corrections`, against the word the recogniser
    heard, and the clip's words are made with it, `Transcript.Of` in
    `engine/words.go`. Every other clip over the same word reads it as it
    was heard, a search and the model read what was heard, and a clip
    removed and made again where it was starts from what was heard. Tim
    chose this: everything done to a clip goes with it. Corrections used to
    be kept for the whole episode in `logs/corrections.json`, which is no
    longer read, so the corrections made before are gone.
  - **A correction may hold more than one word.** Where the recogniser heard
    one word and two were said, writing both splits the word it measured
    between them, so the captions break and highlight them one by one.
    Writing one word again makes it one word again. Each of the words is a
    field of its own in the caption box: "weil" made "weil es", a click on
    "es" opens "es", and delete on it removes "es" and leaves "weil". The
    engine says which of the words each is, `part` in the captions. Before
    it did, the field went on reading "weil es" with "es" drawn after it as
    well, a click on either opened both, and removing either removed both.
    A word the captions hyphenate is drawn in two halves, and clicking
    either one hands back the whole of it to correct, with the other half
    out of the way while it is being typed.
  - **Removing a word:** delete, the key marked delete on the Mac and the
    forward delete key alike, removes the word in the frame, and a word
    typed empty and saved is removed the same way. The word leaves the
    caption box with the key, before the engine has answered, and comes
    back if the engine refuses. A removal is the clip's, not the
    episode's: it is kept in the clip's plan entry, `removed_words`, by
    the moment the word was heard, so the word is gone from this clip's
    captions and its render, and Cmd+Z brings it back. One of the words a
    correction made, like a word typed in beside another, is taken out of
    what the correction reads instead, so the words after it close up. The episode still says it, so every other clip
    shows it, a search and the model read it, and a clip removed and made
    again where it was has it, see `RemovedWords` in
    `engine/removedwords.go`. It used to be a correction to nothing in
    `corrections.json`, kept for the whole episode: Tim removed a caption,
    removed its clip, made it again with I where it had been, and the
    caption was still gone. A word
    removed then corrected again comes back with the new text. Delete in a word open for typing is the field's own and takes
    out letters. The caption the word stood in stays as it was, with the
    word gone from it: the captions are laid out as if the word were still
    there, reading what it read when it was removed, and then it is left
    out, see `Captions` in `engine/lines.go`. A removal changes nothing a
    word reads, so the word keeps what it read, behind a mark no typed
    word can have, in the words the captions are laid out from. The
    time it held is no pause, the room it took on its line stays taken, so
    no word of the next caption is pulled up into it, and no other caption
    moves. A caption whose first or last word it was draws in to the words
    it still shows, the same as when its edge is dragged in over the word,
    see **Caption timing**. A caption whose
    words are all removed goes and leaves its time empty, and the one
    before it goes when it did. It used to stay up through that time, so
    the block before a removed caption grew across its place on the clip
    timeline, which Tim read as two captions merging. Before, the gap it left
    read as a pause, so the caption ended at the word before it, nothing
    was on screen for a moment, and the words after it went on to a
    caption of their own. Removing a word lost its way when the words
    became one list: the engine refused an empty word and the caption box
    put the old one back.
  - **Removing a caption:** a click on a caption block on the clip
    timeline puts the playhead on its first word and picks the block, which
    wears the frame a picked word wears in the caption box. Delete, the key
    marked delete on the Mac and the forward delete key alike, then removes
    every word of that caption at once, the way removing each word would,
    in one step that Cmd+Z takes back whole. The clip keeps its time, the
    place the caption had stays empty, and every other caption appears
    and goes when it did. The block leaves with the key and
    comes back if the engine refuses. A press anywhere else, or Escape,
    lets the pick go, and so does any other key: a block clicked only to put
    the playhead there, then Shift and an arrow to a word, means delete is
    for that word, and a frame stepped with an arrow means nothing is
    picked any more. Found by Tim. The engine finds
    the caption by the moment its first word starts, `RemoveCaption` in
    `engine/corrections.go`, and removes its words from the clip as
    removing each would, so the caption goes with the clip, and a word
    typed back afterwards comes back as any removed word does.
  - **Putting a word back:** Cmd+Z, or type it into the word beside it,
    "das" made "das ein". Both end the same way: the word is back where
    it was heard, with its own time, lit while it is said, and no
    correction is left on either word. The engine sees a word typed
    before or after what its neighbour read, beside a removed word, as
    that word coming back, `putBack` in `engine/corrections.go`. Each
    removed word there takes one, the nearest first, and a word that
    reads as the recogniser heard it keeps no correction. Typed back
    otherwise, "das eine", it comes back corrected. It used to take its
    time from the neighbour it was typed into, so it was lit while the
    neighbour was said and not while it was.
  - **The captions run on the clip's clock and a correction belongs to the
    episode.** A clip's clock has its cuts taken out of it, so the two
    clocks run apart by however much the cuts hold. The middle of the
    caption word is read back through the clip's pieces to find the word of
    the episode it stands for, and the middle rather than the edge because
    both halves of a split word have to point at the one word they came
    from. `inEpisode` and `saidWord` in `frontend/src/lib/flow.ts` are the
    way back, with tests.
- **Moving the captions:** drag the caption box up or down by its handle,
  a ring around the box that draws itself in the accent the moment the
  pointer comes near and is invisible the rest of the time. The whole of
  the box that is not a word takes hold of it, the ring reaches a little
  past its edges so there is somewhere to take hold even where a word runs
  to the end of a line, and the words themselves stay one click from being
  corrected. Its pointer is the up and down one, not the hand: the crop
  frame it sits inside is moved sideways and wears the hand, and two
  things that move in different directions should not say the same thing
  about themselves. The handle is there because the box stopped being
  grabbable the day the words in it became fields: what was left was the
  padding and the spaces between words, a few pixels of a preview, with
  nothing to say where they were. It lands on a
  step of a grid that appears while it moves, and it stays inside the frame.
  **There is one place for every clip of every episode**, because a place
  that suits one video suits the next one, so the drag saves it as a setting
  rather than as an edit of that clip. It stands as **Height** with the other
  caption settings on the left, where it can be typed instead, and **Put the
  captions back** appears under it once it is not the standard 300. A click
  on the box without dragging plays or pauses.
- **Moving the crop:** drag the frame sideways when the automatic crop is
  off. The frame is there while the playhead is in the clip, and only
  then. The new place applies to the whole clip, every piece of it, since a
  clip has one crop. A part turned with Rotate is the one exception and
  keeps a crop of its own. **Automatic crop** appears once a crop was
  moved and brings back the automatic placement, removing the keyframes
  with it, and **Put the crop back** takes that click back again, crop
  marks and all. A click on the frame without dragging plays
  or pauses.
- **Clip list:** beside the player, exactly as tall as the video preview with
  the range picker under it, every clip found, in time order, with its start
  and length. A green dot marks a rendered clip. Click one to select it,
  which also puts the clip timeline back on it.
    - Put back on the clip, the clip timeline centres it under the video
      preview and the range picker, not under the middle of the app. The
      clip list is wider than the settings once the app is narrow, so the
      middle column stands a little left of the middle, and a clip centred
      on the whole track stood up to 38 px right of the picture above it.
      Where the middle column lies is read when the clip is fitted, from
      `.middle`, never kept as a size.
    - The clips lie on a surface of their own, the grey of every row in
      the app, parted from each other by lighter lines than the ones that
      part one area of the workspace from another.
    - **A new episode finds its first clips by itself.** Adding a video is
      all it takes: the Go side asks for its first search, of the first
      of the equal windows the episode is cut into, and no longer than the
      model can read at once, and the range picker goes to that window.
      **The windows grow with the episode**, as the square root of its
      length, so a short episode shows its first clips soon and a long one
      is not cut into dozens: half an hour makes three windows of 10
      minutes, an hour four of 15, two hours six of 20 and four hours
      eight of 30. None is shorter than 10 minutes, so a video of 15
      minutes is one window, and the windows are equal, so no scrap is
      left at the end. When the window moves on by itself, after a search,
      it takes the next one. See `engine/suggest.go`.
      The search hears the episode to the end of the window and then
      finds. Until it finds, the first row of the clip list says Waiting
      for the transcript, and the window is locked for as long as the
      search runs. **Cancel** calls it off, and what was heard stays. The
      local model is loaded while the episode is heard, so the search
      starts finding with it in memory. **While clips are found, no search
      hears**, on this episode or any other, so the model has the machine
      to itself: a search that hears when another comes to finding saves
      what it heard, waits its turn, and carries on after. **Hearing stops
      exactly at the end of the window**: the piece of audio the speech
      model hears is cut on the window's edge, so it hears nothing past
      it, and the search finds at once. A word that runs across the edge
      is not in the window, and is heard whole by the next search that
      needs it. The first search happens only for an episode nobody has
      ever searched, which is one with no plan and no `jobs/` folder, so
      removing every clip again does not bring a search of its own back:
      a search is the machine's time, and nobody asked for it. Deleting
      the work folder makes the episode new, and then it starts over as a
      new one does. The rule is `firstSearch` in
      `cmd/framefairy-app/search.go`, and the path tests follow it.
    - **The target follows the window.** Until it is changed, Target
      holds how many clips the next window suggests, as a number like any
      other: 6 for half an hour of clips of 20 to 30 seconds,
      one for every twelve clip lengths, and other windows in proportion
      to the square root of their length, 3 for six or ten minutes, 8 for
      an hour. A number typed there is for the window it was typed for, and
      kept with that window's length. A window of another length, drawn
      anew or the first window of the next episode, follows its own
      suggestion again, and so does the first search the app starts by
      itself when a video is added. Three typed for the six minutes of one
      episode went on asking for three in the half hour of the next.
      Cleared, the field follows the window again and shows its
      suggestion. The suggestion used to be a grey placeholder in an empty
      field, and a browser steps an empty number field from zero, so the
      down arrow on a suggested 6 landed on 1. Twelve
      because the model, asked for 8 in half an hour, gave 6: it gives
      fewer when fewer moments are strong enough, and asking for more only
      asks it to fill places with weaker ones. The square root because in
      proportion to the length, six minutes were asked for one clip, and a
      short window is still worth a choice. The local model thinks in
      proportion to the window, see [ENGINE.md](ENGINE.md), so a smaller
      window is also found sooner.
    - **What a search is doing is in the row its next clip appears in.**
      The first of the rows still to come wears the beam and the fill and
      says two things: Waiting for the transcript with the window it is
      waiting to cover, then Finding clips, then how many are found, with
      about how long is left under it. Loading the model, reading and
      thinking are the machine's steps and are not spelled out. A headline
      stays at least two and a half seconds and a count of clips at least
      one, so it can be read, while the fill and the time left move at
      once. The time left moves in steps of five seconds. Only the search
      reports on that row: what ffmpeg says while it frames a clip goes to
      the log. **Search** becomes **Cancel** the moment it is pressed, and
      Cancel takes a press from that moment on, never greyed out while the
      Go side has not answered yet. It names the episode's work and the
      press of Search it came after, not a job, so it needs nothing from the
      Go side first: a search the Go side has not heard of yet is called
      off as it arrives and never starts, and says Stopped with Continue
      like any search Cancel stopped. A search that waits for the
      transcript is called off before it starts, and one that runs stops. A render is not a search, so it leaves this head alone:
      **Search** stays Search and cannot be pressed until the render is done,
      because one lane does the work. The render shows in the **Render**
      button it was started from, which fills from the moment it is
      pressed and says **Cancel**, and a click on it stops the render.
    - **How far a search is, is measured.** The fill is the share of the
      search that is done, measured against how long the same parts took
      the last time on this machine. Inside a part, what the model counts
      itself beats the clock: the transcript it has read, what it has
      thought against its budget, the clips it has written. Asking a local
      model again about the clips well off the length is a part of its
      own, so the fill is not full while the cards still say *Fitting to
      the length*. A local model
      this machine has never timed is measured against a search timed on
      an M2 Max until its own first search has finished. A model in the
      cloud the app knows by name is measured against a first guess the
      same way, and one written in by hand shows the beam without a fill
      until it has been timed. Anthropic's models that think by
      themselves are asked for a summary of their thought as it goes, so
      the fill moves on while they think rather than waiting in silence.
      The engine keeps the timings, see [ENGINE.md](ENGINE.md).
    - **Clips arrive one at a time.** The engine writes each clip to the
      plan the moment it is framed, while the model is still writing the
      next, so the list fills in a row at a time and the rows still to come
      shrink as it does. The first clip found is put on screen as soon as
      it lands, unless another clip was picked after the search began: a
      clip taken away from the hand that is working on it is worse than
      one shown a little later. When the search is over, the earliest of
      the clips it found, the first of them in the list, is the one chosen,
      with the playhead at its start, as long as the clip chosen is still
      the one the search chose and the video is paused. The model names its
      clips strongest first, so the first to land is often not the first
      in the list. A clip that has landed can be played,
      trimmed and corrected while the rest are still coming. Stopping a
      search keeps the clips it had found.
    - **A clip on its way is a card in its place.** A clip the model has
      named is in the list, among the clips of the episode where it lies,
      from the moment it is named until it is written: the card of a clip
      with the beam round it, its title and what is being done to it,
      "Placing the crop" or "Fitting to the length". It hands
      over to the clip itself in one step, holding its place until the
      list has read the clip, so there is never a gap and never two cards.
      A clip made with I or O comes in the same way, see below, because
      every job says which clips it has on the way the same way, see
      [JOBS.md](JOBS.md).
    - **The search's fill goes on to the end.** The row still to come
      wears how far the search has come and how long it has left. Once
      every clip is named, no row is left to wear it, so one of the
      search's cards takes it on, with the fill and the time left, until
      the search is done. The fill went with the row before, at sixty per
      cent, and the search went on with nothing to say how far it was.
      Then every card of the search wore it, the same fill and time left
      three times over.
    - **A search is one fill, and it moves down its cards.** Every card
      on its way wears the beam, because each is still being worked on,
      but a search is one piece of work, so only one of its cards wears
      the fill: the first of its cards in the list. It keeps it until its
      own clip is written, even when a card is named above it, and then
      the first card left takes it on, so the fill starts at the first
      card of the batch and ends at the last. While no card of the search
      is on its way, the row still to come wears it, and once one is, that
      row wears the beam alone. The last card in the list wore it before,
      so the fill stood at the foot of the batch while the cards above it
      landed.
    - **The rows still to come stand where the window is.** Everything a
      search finds lies in its window, so its rows stand in the list where
      the window ends: after the clips before the window and before the
      clips after it. A window drawn between the second clip and the third
      opens its rows between the two, and each card that comes takes a
      row's place, so the clips after the window move down once, when the
      search starts, and not again.
    - **A card stays where it came in.** A search stands as many rows as
      it was asked for, from the start: the first says what it is doing,
      the others breathe. Each clip it names comes into the row that says
      so, with the fill it was wearing, and that row's words move on into
      the next row waiting. A card once there stays in its row until the
      search is over, the list keeps its height the whole time, and no row
      slides. Then the clips go to the order they are spoken in, the order
      of the range picker and the clip timeline, in one step. Each card
      used to come in at its moment in the episode, above the rows waiting
      or between cards already there, so every row under it slid down one
      and the last row waiting closed, each time a clip was named. Tim saw
      cards being replaced rather than filled in, and a stack that never
      stood still.
    - **A search starts in sight.** The rows still to come are often after
      the clips there are, so in a long list a search began out of sight and
      all anyone saw was Search turning into Cancel. The row the next clip
      will appear in, the one wearing the work, is brought to the top of
      the column the moment it is there, and back into view every time a
      clip lands, wherever the list has been scrolled to in between. It
      comes back with a third of the row after it showing, so it is plain
      there is more to come, and the clip that just landed stays in view
      above it. Only when a clip lands: following every report would fight
      a hand that is scrolling. The last clip a search finds leaves no row
      after it, so that clip itself is brought into view, to the foot of
      the list when it lands below. The model writes its clips in an order
      of its own, and the list is in the order they were spoken, so the
      last clip found is not always the last in the list: it is the one
      that comes into view, wherever it lands, and the rows still to come
      stay after the last of them. Once the search is over the list is in
      the order the clips were spoken, which is the order of the range
      picker and the clip timeline.
    - **What a search was asked for is held while it runs.** Target,
      Shortest and Longest go to the model with the prompt, so they are
      locked from Search until the search has run, and dimmed. Changing Target
      used to change the rows still to come while the model looked for the
      number it had been given.
    - **An empty list is never empty.** With nothing in the list and no
      search on its way, the rows Search will fill stand there as many as
      Target says, following it as it changes, and still, because nothing
      is filling them yet. When every clip there was has been removed, the
      first of them says so, **All clips removed**, **Search looks for others**,
      so the empty rows read as something that happened. It gives no
      number, because the plan keeps every clip ever removed, and a count
      of them was more than the list had just shown. The empty rows open
      only once the last removed row has closed, so they never push it
      down the column as it goes. An episode whose first search was stopped, by
      quitting among other things, had a bare column there before.
    - **A search that found nothing new says so.** A search that ends
      with no clip beyond the ones in the list, a window searched again
      or a video shorter than Shortest with its clip in the list, leaves
      one row after the clips, **Nothing new found**, with its window or
      **All clips already found** under it and why in its title. It is
      news, not a state, so it runs down in grey for as long as a removed
      clip's row can be put back, stands still under the pointer, and
      closes. Its time runs on while another video is open. Each search
      that finds nothing says so once, so Search pressed again and again
      says Finding and then this, every time. Its row used to close
      again with nothing said. It is not kept across a restart, since
      there is nothing to carry on.
    - **A search that stopped before it was done says so** in the row its
      next clip would have appeared in, in the colour of a warning, or in
      the red of an error when it failed, and **Search** becomes
      **Continue**. *Interrupted. Click Continue* is a
      search the app was closed or fell over in, with the window it was
      about under it, or with how far the transcript came when it still
      waited for the transcript, which is the first half of every search on
      an episode read only part way. *Failed. Click Continue* has the
      window under it and why in a toast. A search the app would not start, while it closes
      or while the episode is being removed, is a search that failed
      too, with the window it was asked for. The whole of it is in the
      row's title. **Continue**
      takes the window back to the one the search was about and asks for
      it again: a search cut off while it heard hears on from where it
      stopped and then finds. Nothing starts by itself. It stays until it
      is acted on, after a restart too, because the search keeps its record
      in `jobs/search.json` in the work folder, see [JOBS.md](JOBS.md), and
      the app reads the records as it starts: one in a running step is a
      search that was cut off, a failed one says why. **Cancel** says
      *Stopping* in that row the moment it is pressed, standing still, and
      then leaves the same row, *Stopped. Click Continue*, with how far the episode was
      transcribed, because what it did stays and can be carried on.
      Continue goes on saying how much of the window is transcribed, in
      the same words, *16:45 of 30:00 transcribed*, growing as it hears,
      until it can say how long it has left.
      It once said *Window 0:00 to 30:00* under Transcribing, as if
      nothing were heard yet, Tim's report of 25 October. The
      frame over the window stands still and fades out from the press, the
      way the whole video's frame does on Stop transcribing, Tim's report
      of 24 October. **Search**
      takes its place. Clips it wrote before it stopped stay, with the row
      after them. The window's mark stays on the window the search was
      about and can be dragged as at rest: moved anywhere else, the
      search is set aside, its row closes, **Continue** becomes **Search**,
      and Search starts a fresh search where the window is. Moved back, by
      hand or with Undo, Continue carries the search on as before. The Go
      side says whether a search is set aside, from the video's one
      window, see below, so it is the same after a restart. Tim cancelled a search to look somewhere else and
      could not, because the window stood locked until it was carried on. The episode's dot in the sidebar says it too: the colour
      of a warning for a search stopped or cut off, of an error for one
      that failed, the red of its row and of its toast. A search set
      aside has no row, so the dot says nothing of it either, issue #436. **Its trash can**,
      there while the pointer is on the row, removes the search when it
      is no longer wanted: the row says Removed with Put it back for ten
      seconds, the way a removed clip's row does, and the search's cards
      say it with it. Then the search and its record are forgotten, so the
      row, the cards and the dot go, also after a restart, and **Search**
      is back. The clips it wrote stay. Issue #431.
    - **Search**, above the list, finds clips in the window, as long as the
      episode's windows are, and says so while it looks. The clips it
      finds join the ones already there. After a search the window walks
      on from where the search ended, as long as it was left, so a window
      made a minute long stays a minute long, and at the end of the
      episode it starts over at the start. The last window may be cut
      short by the end of the episode, and the one after it, at the
      start, is as long as the window was made again.
      **A video has one window**, and the Go side owns it, see
      `cmd/framefairy-app/window.go`. It is kept beside the clip last
      worked on in `chosen.json`, so the workspace opens on it again,
      after a restart too. A video that has none gets the window where
      the fewest searches have been, earliest first, as long as the
      video's windows are, placed by the Go side, and a double-click on
      the window's marks puts it back there. Every search starts on the
      video's window, the first search of a video just added too, and a
      search that found its clips moves it on before it says it is done.
      The workspace draws it, and writes it only when a hand moves it or
      when it no longer fits what a search can read. The first search
      once placed its window here and the workspace placed another on the
      range picker's step, two seconds apart, so a search cancelled while
      it heard said Search with no Continue, Tim's report of 24 October. A short
      episode is one window and is searched whole each time. A window searched again keeps every
      clip there is: the model is told which lines are clips already and
      asked for other moments, and a search that brings none ends with
      nothing added. Nothing on the range picker is taken or given back:
      the window goes anywhere and Search reads it, as often as anyone
      likes. Search is never off for want of room, and its title says
      whether it looks somewhere new or again.
    - The trash can on a row removes that clip. Its mark leaves the track at
      once, and its row stays in place for ten seconds, in red with a trash
      can, saying **Removed** and offering **Put it back**. The time left is
      the app's one fill, in red, running down to nothing over those ten
      seconds, with the light over it running the same way, right to
      left, see `Busy.svelte`, and it stands still while the pointer or
      the keyboard is on the row, so the time to change your mind is seen
      and nobody is rushed. It runs on the moment the pointer is seen
      anywhere else, also when the row moved away from under a pointer
      that stood still, which WebKit never reports. A row that appears
      under the pointer, as it does under the trash can just pressed,
      starts held and full, and its time starts once the pointer leaves:
      WebKit does not count it as under the pointer until the pointer
      moves, so the time ran for a moment and stopped a little short of
      full, Tim found. Where the pointer was last seen says it. Nor does
      the time start on the events a browser makes up while the node under
      a still pointer is replaced: the pointer is let go of only where the
      page finds nothing of the row under it, and taken to have left the
      app only once nothing has said otherwise for two frames. An over on
      the tile round the row says otherwise when the row is under the
      pointer, since WebKit may send no over on the row itself. The
      pictures of a report wear the same row and the same time, see
      Report a Problem. Nothing above or
      below it moves while it is there, and when it goes the list closes
      over it. Every clip removed has its own row and its own ten seconds:
      removing another, or putting one back, leaves the others as they
      are. There was one for the whole list, and a second clip removed took
      the first one's way back with it. The ten seconds are the episode's,
      not the screen's: they run on while another episode or page is open,
      and coming back within them finds the row again with the time it has
      left. Time the pointer held it still is kept too. A list read again,
      after a search, a render or an undo, leaves the rows as they are, and
      an undo that puts the clip back makes it a clip again. Leaving the
      episode, or any of those, used to take every way back with it.
      A clip is always chosen while there is one: removing the chosen
      clip chooses the next in the list, or the one before it when it
      was the last, the way Finder and Mail move on from what was
      deleted. It used to leave nothing chosen, Tim found.
      Putting a clip back takes back the remove and nothing more: the
      clip chosen stays chosen and a play goes on where it is. Only the
      clip that was itself the one chosen when it was removed is chosen
      again, while the clip chosen in its place is still chosen, and the
      playhead goes back to where it was. Putting back used to choose the clip put back
      and play it from its start, away from the clip being watched, Tim
      found. The clip stays in the plan with everything done to it, and
      a render of the whole plan leaves it out.
- **Range picker:** one slim strip for the whole episode, under the video
  preview and exactly as wide as it. It shows where the clips are and
  where the playhead stands, and while clips are being found, the part
  being searched.
    - **At rest the window is marked at its corners, not drawn.** A bar
      lies on the top border and one on the bottom border, over them,
      across the window's width, and a triangle hangs from each end
      of them with its tip reaching into the track. Nothing else of it
      lies over the track, so it hides no clip and does not read as a
      window. It says where Search looks next, which the app decides, see
      Search above. The four triangles are the handles: the left two drag
      where the window starts and the right two where it ends. A bar
      drags the whole window and keeps its length, and a double-click on
      any of it puts it back where the app would have it. An edge lands
      on a round step, the smallest one still about eight pixels wide,
      five minutes on a four hour episode and five seconds on a six
      minute one, and the ends of the episode win over the step. Every
      window the app places itself lands on a step of the same kind,
      chosen from the length of the video alone and never from how wide
      the app is, about a 180th of the video, two minutes for four hours,
      its start and its length, so the app's own half hour of a four hour
      episode is 30:00 rather than an even eighth of it, 30:02, which
      drifted a few seconds off the ruler's lines with every search, and
      the window is the same whoever places it. The last window of
      the episode takes in a scrap shorter than a step rather than leaving
      it behind. A handle
      stops where the window would hold too few clips, room for the
      clips typed into Target at the Shortest length or for one, or more
      than the model reads at once, and the marks flash twice in the
      colour of a warning as it runs in, so a hand that keeps pulling
      knows it is the limit and not the app that stopped. The triangles'
      titles say both limits in words. Under the pointer the whole mark lights up, a bar shows
      the hand that grabs and a triangle the arrows left and right. The
      triangles lie over the playhead, which a search leaves on the
      window's start, and the bars under it. The title of Search and the
      suggested Target follow the hand. While its clips are found the
      mark turns into the window, drawn over the track, and back into
      the mark when the search is done, on the window after it, or when
      it is stopped, on the window Continue carries on. Only one of the two is ever
      drawn. Tim found a window at rest a leftover that got in the way:
      after a search of the whole of a short episode it lay over all of
      it and hid every clip just found, and an outline with handles read
      as the same window. A clip is removed from its row in the list.
    - While clips are being found for it, the window breathes and the
      motes of work in hand, the ones every control sheds, rise through
      it, which is the track saying work is in hand. Nothing else on the
      range picker moves: how far the search has come, the transcribing
      and the finding, is on its cards in the clip list. A press on the
      window is a press on the track.
    - A mark for every clip runs across the middle of the track, green once
      rendered, and the chosen clip is a frame round the wash, the way the
      clip timeline draws it. Click a mark to select that clip.
    - A press anywhere else takes hold of the playhead, see above.
- **Nothing sits under the range picker.** The line that parts the workspace
  from the clip up close runs right below it, and the workspace is exactly as
  tall as the video preview and the range picker need. If the transcript has
  not heard all of the window yet, a search has what is missing heard first
  and then finds its clips by itself.
- **One mark explains one thing, where that thing is.** The video preview,
  the range picker and the clip timeline each carry their own info mark in
  their top right corner, and a mark only appears while the pointer is on
  the area it belongs to. The same goes for the marks beside **New clips**,
  **Captions** and the head of the clip list. No mark explains two areas,
  and nothing floats at the end of a row of its own.
- **One row under the clip up close**, so the range picker and the waveform
  stand together with nothing between them: play, loop and the crosshair
  that goes to the playhead, with **L** for loop the way **T** is the
  thumbnail's, as the three marks anyone knows. Like play, which shows
  pause while it plays, the loop shows itself crossed out while it is on,
  which is what a click does then. Loop switched on or off
  while the clip plays changes nothing that plays: the loop only says
  where the play ends. The play works out the next time through before
  the clip's end, and when the loop goes off it keeps it but stops at
  the end, the picture there and every stretch of sound on the sound
  card there, in the silence the clip fades out to. When the loop comes
  back on, the same stretches play on. Started again from the playhead
  for each press, the picture and the sound stopped for a moment and the
  sound clicked, and kept going without a new end, the play stood at the
  clip's end while the sound of its start played on for seconds. Tim
  heard both. Then the
  playhead's time, in a field as tall as the buttons with a field's border:
  where the frame the playhead stands in starts, to the millisecond, so it
  steps a frame at a time and follows the hand as the playhead is dragged
  and the picture as it plays, `playheadClock` in `lib/playhead.ts`. With
  hours for a video of an hour or more, `1:05:30.417`, and without for a
  shorter one, `05:30.417`, so the field is as wide at the start of the
  video as at its end. The selected clip's title and numbers stood there
  before, and they were what its card in the clip list already says, so
  the card says them alone: where the clip starts, how long it comes out
  and in how many pieces, `1:05:30, 28 s, 1 piece`, following the hand
  while an edge or a cut is dragged, issue #361. On the
  right what a reset threw away,
  **Render** and, once rendered, **Show in folder**. Rendered means the
  clip was rendered, as its clip set noted when the short was written, so
  the green dot, the green mark and Render again stay after the short is
  uploaded and removed from its folder, issue #384. Show in folder shows
  the short where it went, in the folder **Shorts** named at the time,
  and only while it is still there: a short removed or written over by
  hand is no longer the clip's short, and a file of the same name that
  another episode put there is not this clip's. A short there is the one file outside the episode's work folder
  the app shows or opens, because a clip of the episode made it.
  Above that row, the clip up close:
    - The arrow keys step the playhead one frame of the episode, and one
      word with shift, wherever the keyboard is as long as it is not in a
      field or on an edge of the clip. **A word, because a word is what the
      picture is showing:** the caption lights up the word being spoken, so
      shift and an arrow walk that light one word on. A second was what
      they took before, and a second is nothing in particular. It lands in
      the middle of a word as often as not, it walks four words at a time
      where someone speaks quickly and none at all across a pause.
      While the episode plays on the video, they walk the words of the
      transcript and stay on the video, see the place of the playhead
      above. Found by Tim, who skipped ahead through a dimmed clip and
      had it light up.
      - **It walks the words the caption lights up, not the words the
        transcript holds.** They are not the same list. A correction that
        reads as two words is two words in the caption and one in the
        transcript, so a word added by hand stood in no list the timeline
        had and the keys stepped straight past it. A word a cut takes out
        is the other way round, in the transcript and never in the caption,
        and landing on one lit nothing. And a plan keeps the moments it was
        made with, so a transcript read again since can put the same word a
        quarter of a second elsewhere. Reading the caption makes all three
        right at once, and keeps them right, because whatever lights up is
        what these keys walk.
      - **Which word to go to is decided by the word the playhead is in,
        never by how far it is from one.** The playhead is not where it was
        put: the picture answers with the frame it is showing, which can be
        later than where the playhead was sent. Measuring a distance then
        found the word the playhead was already on and sent it to the same
        place again, so the key did nothing at all and nothing but the
        mouse got out of it.
      - The playhead lands a frame inside a word, never on its edge. A
        boundary is exactly where the question "is this word being spoken"
        has no steady answer: the caption runs on the clip's clock and the
        playhead on the episode's, and the frame the picture settles on is
        a third answer again. On the edge, two words in every five lit
        nothing at all.
      - **Past the last word of a clip they carry on into the one beside
        it**, the left arrow from the first word of a clip landing on the
        last word of the one before it and the right arrow the other way
        round. A clip's words arrive with its captions, so the clip is
        chosen first and the landing waits for them. **With loop on, the
        clip comes round instead**, the way a play of it does: past its
        last word is its first word, and before its first its last, and the
        clip stays chosen.
      - Outside a clip there is no caption, so the words that were heard
        are the only ones there are. Where nothing has been heard yet
        shift takes a second.
      - **Which list is walked is decided a whole frame either side of the
        clip**, for the same reason the crop frame is. Read exactly, a
        playhead just put at a clip's start is outside it, so the keys
        walked the transcript instead and the first press after picking a
        clip landed wherever the word before the clip happened to be.
    - Drag an edge to trim. The edge lands on the frame, the same as the
      edge of a cut, and shift puts it on the nearest word instead, the way
      the render cuts a clip the engine proposes, and then on the frame
      nearest that, where the render cuts it. Shift and not alt,
      because it is the key Tim reached for first. It used to snap to words
      always, which left no way to take a breath off the end of a clip or
      keep the first frame of a gesture before the first word.
    - **A double-click on an edge puts it back where the clip was found**,
      the way a double-click on a caption's edge puts that back where its
      words put it. Tim asked for the two to be the same. The clip keeps
      where its edges were in the plan, `found`, written by `editPieces` in
      `engine/edit.go` the first time anything changes its pieces and never
      after, so a clip never changed needs none and is where it was found.
      A clip changed before this was written keeps the edges it had at its
      first change after it. The edge's title says when it was moved by
      hand, and the double-click is a trim to the edge it was found with, so
      Undo takes it back like any other trim. It lands there exactly: a
      search puts every edge on a frame as the clip enters the plan, so
      where the clip was found is a frame's start, and the frame nearest
      the hand is that one. A search used to leave its edges between
      frames, and the end went back to 24.80 where the clip had ended at
      24.72. What the trim had taken away comes back with the clip's crop,
      the same as a cut put back, see [ENGINE.md](ENGINE.md).
    - **The playhead goes with the edge.** While either edge of a clip or
      of a cut is dragged, the playhead stands on it and the video preview
      shows that frame, the clip as it is being dragged, with its captions.
      On frames it is the clip's first frame, or its last, a frame before
      the end. A cut's left edge ends the piece before it, the way the
      clip's end ends the clip, so the playhead stands in that piece's last
      frame, and a cut's right edge starts the piece after it, the way the
      clip's start does, so it stands on that piece's first frame. One rule
      in the engine answers for all four, `edgePlayhead` in
      `engine/shape.go`. A cut's edges used to leave the playhead where it
      was, found by Tim, so the frame beside a cut could not be seen while
      it was placed.
      With shift it is the same frame, in line with the edge wherever the
      edge lands on the words. It used to stand inside the word the edge
      snapped to, a pause away from the edge, so that word was the one lit,
      and Tim saw it leave the edge: a cut's left edge dragged on past the
      last word took the playhead over the cut into the caption before it.
      The edge is put on its frame first, the way the pieces are when the
      gesture lands, so the playhead is never half a frame off the edge
      drawn. After letting go the playhead stays where it was.
      While the clip plays the playhead does not go with the edge, it
      plays on, see the playhead above.
    - **One press for the whole clip timeline.** Every press on it, on
      the track, the playhead, an edge of the clip or of a cut, a
      thumbnail, a keyframe, a caption or a caption's edge, goes through
      one handler, `press` in `ClipTimeline.svelte`, and is handled the
      same way: the playhead goes where the thing pressed says at once, a
      double-click is told from the two presses, a drag starts after two
      pixels and is measured from the press, so a press beside a line does
      not make it jump, and letting go saves. What is pressed only says
      where the playhead goes, what its drag and its double-click do. Each
      kind of thing used to have a handler of its own, and they drifted:
      the track moved the playhead on the press, an edge only once the
      engine answered, a caption's edge only when the hand let go, and
      Tim asked why a click did something different on every kind of
      thing. An edge's playhead is the frame it opens or the last frame
      before it closes, the same rule `edgePlayhead` in `engine/shape.go`
      has for the drag, worked out where it is pressed rather than asked
      for.
    - **What a press takes is the nearest thing of the first kind under
      it**, in a fixed order: a thumbnail or a keyframe, a clip edge, a
      cut's edge, a caption's edge, a caption, and only then the playhead
      and the track. What is drawn on top only decides what is seen. The
      playhead stands on an edge it was dragged with, and a press there
      took hold of the playhead instead, found by Tim. A mark comes first
      because it is small and sits on the clip's rules, often on its first
      frame, where an edge reaches over it and would leave it with no way
      to be taken hold of.
    - **An edge can be taken again at once.** The edges stay while the
      last drag is saved. The same edge taken again goes with the hand
      from where it was let go before that save is back, and the two are
      saved in the order they were let go. Another edge waits for the
      save and goes on from the clip as saved, because the engine answers
      a gesture against the clip as saved. The edges used to go for as
      long as the save took, so pressing an edge again straight after
      letting go moved the playhead instead, found by Tim. And the same
      edge used to wait too, standing still under the hand on a busy
      machine (2.259).
    - **The last press owns the playhead.** Every press takes it, and an
      answer the engine gives for an earlier one no longer moves it, so a
      click on an edge followed at once by a click on the track leaves the
      playhead where the track was clicked.
    - **Shift goes the way of the hand.** An edge dragged with shift goes
      to a word only in the direction the hand goes, and stays where it
      is until the stop nearest the hand lies ahead of where the edge
      stood, `toward` in `engine/shape.go`. The nearest word could be behind it,
      and Tim saw a cut's left edge dragged right jump left first.
    - **Shift stops at the words that light up.** An edge dragged with
      shift lands on the words the way the clip's captions split them, so
      the halves of a hyphenated word, or a correction that reads as two
      words, are two stops, the same words the arrow keys walk.
    - **Shift stops where a caption goes, too.** A caption stays up a
      little after its last word, and its block on the clip timeline is
      that long. The end of a clip and the left edge of a cut dragged with
      shift stop at the end of the block before they go on to the word,
      when it lies in the pause after it. They used to know only the word,
      so the edge went past the end of the block into it and the block
      shrank under it, and the block looked longer than the words it
      stood for.
    - **The engine answers for every drag.** The timeline sends what the
      hand is doing, `Shape`, and draws what comes back: the pieces, the
      captions and where the playhead goes. Letting go saves the same
      gesture, `Reshape`, so what was drawn is what is saved. The timeline
      keeps no rules about where an edge lands, see
      [WORDS.md](WORDS.md).
    - **The clip edge is over the caption handles.** The first caption is
      on screen from the clip's first frame, so its handle stood on the
      clip's start edge, and a hand that reached for the clip in the band
      of the captions moved the caption instead.
    - **The captions follow the drag.** While an edge of the clip or of a
      cut is dragged, the caption blocks are drawn for the clip as the hand
      has it: a word the edge reaches gets its caption under the hand, and
      a word it leaves loses it. The engine makes them with the answer to
      the gesture, and saves nothing until the hand lets go. They used to wait for the hand to let go, so a drag showed
      the old captions over the new clip. After letting go they stay until
      the saved clip's captions come back, which are the same, so nothing
      jumps. Letting go first waits for the engine's answer to where the
      hand let go, so on a busy machine, where the answers come late, the
      playhead still ends on the edge and not where an earlier answer put
      it, see plan row 2.237.
    - Click an edge to put the playhead exactly on it, which is how a clip
      is started over. While it plays, a click on an edge does nothing to
      the play.
    - **Nothing on the playhead but the playhead.** It carried a magnifier
      that opened a pill of the words around it, which was where a word was
      corrected. Words are corrected in the caption box over the picture
      now, where a short will show them, and a magnifier that could only
      read was a second place to look at the same words. So it came out,
      and the track is the waveform and the playhead and nothing else.
    - **The clip is one thing, holes and all.** One frame in the accent,
      with its corners just rounded, runs round it from its first piece to
      its last, whatever is cut out in between, so a clip with a cut in it
      reads as one clip and not as two standing in a row. It is drawn over
      the time lines, the cuts and the captions, so nothing crosses it. An
      edge of it widens under the pointer and while it is dragged. The accent wash inside the rules says
      which parts are kept. A part the clip leaves out, usually dead
      air the engine found, is the track's own background with an accent
      line at each end, the way an editor marks the place two shots were
      joined: what is not in the clip looks like everything else that is
      not in the clip. Playing jumps it, and the render does too.
    - **The cuts can be changed.** Each one carries a handle on either edge,
      in the accent's lighter shade so it is not taken for the clip's own
      edge. Dragging a handle moves that edge of the cut, and a double-click
      on the block or on either handle removes the cut, so the part plays
      again.
    - **A double-click in the clip cuts a part out** where it lands, forty
      pixels wide, which is wide enough to see and to take hold of by either
      edge and drag to size. Outside the clip a double-click does nothing,
      and nothing else on the track makes a cut. Shift and a drag used to
      draw one, and a double-click in the place a cut had just been removed
      took it out again: Tim found neither, and the second made a double-
      click in the clip cut something only sometimes. Shift means words and
      clips everywhere else in the app, so a drag with it held moves the
      playhead like any other drag.
      Forty pixels and not a quarter of a second, because what has to stay
      the same is what the hand sees: the timeline goes from a whole four
      hour episode down to a second across, and a width in seconds is
      wrong at both ends of a range that wide. Measured at the zoom the
      timeline opens at and again with a second across the track, a quarter
      of a second came out as 9 pixels and then as 187, narrower than one
      of the two 12 pixel edge handles and then a quarter of the track.
      Forty pixels comes out as 40 and 47, the 47 being the frame rounding:
      a cut is a whole number of frames wide, rounded up, so it is never
      too short for the engine to take. The view never moves: a gesture
      that cuts and zooms at the same time is a gesture nobody can aim.
    - **Moving one edge never moves the other**, of a cut or of the clip.
      A drag of a cut's edge says which edge it moves, `Edge` "from" or
      "to" in a `move` gesture, and the engine puts only that edge on a
      word with shift, and stops it at the other rather than pushing the
      other along. Shift on the right edge of a cut used to put the left
      one on a word as well. A click on an edge of a cut that does not
      drag puts the playhead on it, the way a click on an edge of the clip
      does.
    - **A caption that a cut takes the first word of** appears where the
      cut ends, when that word ran straight on into the rest of it. It
      used to wait for its next word, so a cut's right edge moved a frame
      further into a short first word made the caption jump past the
      pause after it, though it had been on screen there a moment before.
    - **A caption is drawn only where the clip is.** Its block lies over
      the parts the clip keeps and never over a cut, in as many pieces as
      it takes. Where a caption goes at a cut, its end is the end of the
      piece before, `endInEpisode` in `frontend/src/lib/flow.ts`: a moment
      at a cut is both the end of the piece before and the start of the
      one after, and an end taken as the start of the next piece drew the
      caption across the whole cut. Tim saw it moving the left edge of a
      cut into a caption, and with shift on a cut's edge, where the next
      caption begins right where the cut ends. A block has no padding, so
      one a few pixels long is a few pixels long and does not reach into
      a cut.
    - **A double-click is told from the two presses**, not from the
      browser's dblclick. The first press moves the playhead under the
      pointer, so the second lands on the playhead's line or on a cut's
      handle, and a dblclick goes to whatever the second landed on. Over
      the line it went nowhere, which is why a double-click in the clip did
      nothing while one on a cut's handle did. Two presses within half a
      second and six pixels of each other are a double-click wherever the
      second lands.
    - **A cut lands on the frame.** The edges stay where the hand put them,
      rounded to a whole frame of the episode and no further, because a
      double-click and a drag both say where exactly and moving the edges
      somewhere else is not what was asked. A cut made this way may stop
      inside a word, which is the point of it.
    - **Shift lands on whole words instead.** Holding shift while dragging a
      handle of a cut puts the edges where the render would cut them, so a
      cut dragged over a pause takes the whole pause and a cut dragged over
      speech takes whole words, and it can never stop half way through a
      word. That is the right thing when a whole phrase is to go and the
      wrong thing when a breath is: a drag of a few pixels in a silence came
      out as the whole silence, which is why this is the modifier and not
      the default. The key is read while the hand moves rather than when it
      goes down, so letting go of it part way through goes back to frames
      and the block says so before the drag ends.
    - Nothing is written over the waveform. The captions are in the video
      preview as they are spoken, and that is the one place they are
      written out, so the waveform has the whole track to itself.
    - The waveform is drawn the way an editor draws one: one column of the
      screen per column of the picture, each a whole pixel wide. Nothing is
      ever drawn between two pixels, so it keeps the same weight at every
      zoom instead of brightening and dimming as it is pinched.
    - **Zoomed out a column is the loudest reading in it**, because a peak
      that was averaged away is a peak nobody can see. **Zoomed in past the
      measurement the outline runs between the readings.** Loudness is
      read every hundredth of a second and no finer, so a second of it on
      a retina screen is a hundred readings across eight hundred pixels:
      squared off, that is eight pixels of one height, a cliff, and eight
      more. The engine says how fine its measurement was by never
      answering with more parts than it read, and the timeline joins them
      rather than squaring them off. It invents no detail, it stops
      pretending each hundredth of a second was flat.
    - The times sit at the top of both tracks, in the same quiet grey, a
      step above the lines around them and below the waveform inside them.
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
    - The range picker puts a time at every step there is room for, from
      how wide the widest time really is: a time stands 4 pixels right of
      its line and 4 pixels clear of the next, and one that would run into
      the end of the range picker is left out, see `lib/ruler.ts`. It was a
      guess, 72 pixels a time, and a range picker 370 pixels wide drew one
      time every five minutes on a six minute episode, where a time every
      minute fits. Its finest step was a minute, so an episode shorter
      than that had no line on it at all. Steps of seconds join in where
      they draw no more than eight lines, so a fifteen second episode has
      a line every two seconds and one of a few minutes keeps its line a
      minute. The clip timeline keeps its sparser ruler, a time for
      every 96 pixels: the same rule there drew a line over the waveform
      every second, and Tim preferred it as it was.
    - Everything on the range picker, the marks, the window, the playhead
      and the lines, and the captions over the video preview are placed
      by the stylesheet, in shares of the track's width and the picture's
      height, `round(share * 100cqw, 1px)` and `calc(share * 100cqh)`, so
      they move in the same layout pass as what they stand on while the
      app is resized. They were placed from a width a ResizeObserver read.
    - **Two fingers move along the episode**, the way an editing timeline
      works: a swipe left or right travels through the episode, and a pinch
      zooms around the pointer, down to two seconds across the track and out
      to the whole episode. Both are heard anywhere over the clip timeline,
      the playhead included: a click puts the playhead under
      the hand, and a swipe from there goes on, 2.230. **Both ends are walls.** A pinch that can go no
      closer does nothing at all, rather than carrying on and sliding the
      view sideways. A view moved by hand stays where it was put,
      wherever the paused playhead goes. **While it plays, the view keeps
      the playhead in sight**, the way Premiere and Resolve do by default:
      when the playhead reaches the last twentieth of the view, the view
      turns a page and the playhead stands a twentieth in, the same
      distance from the other side, so nearly the whole view is ahead of
      it. A quarter in, tried first, left Tim too little to look ahead to. It turns rather than slides, so
      the waveform holds still to be read between turns, and the view
      turned is held, like a move by hand, until the crosshair or the clip
      takes it back. The crosshair in the row under the track
      goes to the playhead and puts it in the middle. Clicking a clip in
      the list, the one that is already selected included, puts that clip
      back in view. A double-click on the track never did: one was meant
      to, and it went nowhere for the reason above. Its info bubble says
      only what the track itself does, a line each, and leaves the row
      under it to its own titles. The words and the waveform of the whole episode
      are read once and kept, so swiping does not wait for a file to be
      read again.
    - **Every clip is on it, not only the chosen one.** The other clips
      are the same marks the range picker draws, across the middle of the
      track where the captions run, in the app's colour and green once
      rendered, one style in `app.css` for both tracks. Each is the same
      share of its track's height, 16%, so the mark on the range picker is
      half as tall, as the range picker is, in solid colour with a soft
      shadow on all four sides. The waveform's grey and the app's colour are
      about as bright as each other, and such an edge seems to shimmer. The
      shadow gives it a step in brightness. A click on one chooses it. The
      chosen clip has no mark, because its frame already shows it. Clips
      may lie over each other, and on the clip timeline another clip's
      mark gives way inside the chosen clip's frame, where its captions
      are worked on. Drawn over them it hid them, and the caption being
      shown, in the app's colour, could not be told from a mark in the
      same colour. On the
      range picker the chosen clip is drawn the way it is up close, a
      frame round the wash, nearly three times as tall as the rest. Zoomed out, the
      track is the episode with all its clips, the way an editor's
      timeline shows every clip on it.
    - **A frame lies on the border of its track.** The chosen clip's frame
      on the clip timeline, and the window on the range picker, at rest
      and while it is searched, lie over the track's top and bottom border
      rather than just inside or just outside it, so no grey line runs
      beside them. Found by Tim on #192.
    - **Detail comes in as there is room for it.** The caption blocks are
      drawn only while the caption in the middle of the clip is at least
      as wide as a block is tall, 16 pixels. Narrower, a block's line has
      no room inside its padding and the blocks run into one smear over the
      waveform, so zoomed out there are the clips and the waveform and no
      captions, and zoomed in to the clip they come back.
- **The waveform comes first.** The moment an episode is added, the app
  measures its loudness on its own, without the speech model: the audio
  decoded and a reading every 10 ms, the same readings the transcription
  takes. Audio decodes at a few hundred times real time, so an hour is
  measured in seconds, and the waveform fills in as it goes, first where
  the clip timeline looks. Every time the clip timeline asks for the
  waveform it says what it shows, and the measuring goes there next: a
  playhead put near the end of a four hour episode has its waveform
  within a second rather than after everything before it. The clip
  timeline reads its view again while what it read had a gap in it and
  more has been measured. An episode added before this was there is measured the first time
  it is opened. It is not a job: nobody starts it or waits for it. At most two episodes are measured at a time.
- **Before the first transcription** there are no words. That is where
  every episode starts, so the captions band simply waits. The words
  appear as the transcript grows past them, without anything being
  clicked. Nothing about this is an error, and nothing about it is logged
  as one.
- **The timeline is always there.** With no clip selected it shows the
  minute around the playhead and follows it as the episode plays, so there
  is always something saying where you are. It never shows more than the
  episode, so a shorter episode fills it, all of it. Trimming needs a clip, so it
  appears once one is selected, and so does the caption box a word is
  corrected in.

Every change is saved at once. There is no save button. A changed clip gets
new captions on its next render.

Two files that would share a work folder cannot both be in the library.
Everything about an episode lives in a folder named after it without its
extension, so `ep.mp4` and `ep.mov` side by side would share a transcript,
clip sets and rendered names. The second one is left out and the reason
says which two.

A file whose picture ffmpeg cannot decode is left out the same way, with
ffmpeg's reason: the episode's decoder is asked for its first frame as it
is added, `undecodable` in `library.go`, and the decoder that answers is
the one the video preview reads from when the episode opens, so asking
costs no start later. A file with no picture, and one that is no video at
all, said nothing at Add before and failed in the video preview after.

### Searching what was said

The transcript button in the head of the clip column, left of
**Search**, a speech bubble with quotes in it, the mark Apple Podcasts
puts on the button that opens an episode's transcript, searches
everything the video says, as far as it has been heard (#366).
Search finds clips with the model. This finds a moment yourself: a line
found is a place in the video, and I and O make a clip there.

- **The field takes the head's place.** A click on the transcript button
  swaps the whole head, Clips, the count, the info mark and Search, for a
  search field at the same height, with the keyboard in it. It says
  Search transcript in a grey darker than a label, because the browser's
  lighter grey read like something already typed, found by Tim. The cross in
  the field gives the head and the clip cards back. There is no
  shortcut to open it, because the app has no ⌘ shortcuts.
- **A card for every caption, and for every sentence between.** While
  the field is open, the column shows what was said as cards, each with
  its time at the head and its words under it, `engine.SaidCards`. A
  clip's cards are its captions, one card a caption, so a block in the
  video preview, a block on the clip timeline and a card here are one
  thing in three places, found by Tim. A card reads what its caption
  shows, a word corrected in the caption box among it. Two clips over
  each other each have their cards of the same words. What no clip keeps
  is in sentences, the engine's, `engine.Sentences`, the lines the model
  reads grouped the way the stories recipe groups them. A long sentence
  is more than one card: a card ends at a line once it runs 8 seconds,
  and never runs more than 12. Cards of twenty seconds and more were too
  much to read as one place, found by Tim. A sentence card is also cut at
  every edge of what a clip keeps, so it never holds a word a clip shows.
- **The clips stand out.** A card of a clip wears the chosen clip's
  accent edge, the clip's name next to its time and its words a step
  below white. The edge is one and the same in both lists, `.clip-edge`
  in `app.css`: a bar 4 px wide down the card's left side, which the
  card's rounded corners cut off straight. In the transcript it curved
  round the corners and was a pixel narrower, found by Tim. Every other card is in the grey of a label, and the words
  that answer what is typed are white in every card, so they stand out.
- **Word for word first, then with a slip.** With nothing typed, every
  card is there. Typing keeps the cards that hold what was typed. Only
  where no card holds it, the cards where every word typed is a word of
  the card typed with a slip, a letter too many, too few, wrong or two
  swapped, `near` in `lib/match.ts`. A word of up to seven letters may
  have one slip and a longer one two, two letters none, and from five
  letters on it may be the start of a longer word. So arrm finds Arm and
  genomen finds genommen. The font list's way of finding a name, by its
  letters in order with gaps, found Verteidigungsministers for ernste in
  a transcript, found by Tim. The words that answer are bold. What is typed
  is read from the top of the list, wherever the hand had scrolled it.
  The field says in grey, before its cross, how many cards match, the
  number fading in anew as it changes. Where nothing matches, the list
  says No matches.
- **A card is a place.** A click puts the playhead on its words, and a
  card of a clip makes that clip the one worked on, with the list left
  where it is. The other way round, the playhead put anywhere else, on
  the clip timeline, the range picker or a caption block, brings the card
  it is in into view, or the next card where nothing is said. A part not
  transcribed yet is a place too, so the playhead in one brings its
  dashed card, not the next words heard after it. The card of the
  playhead is marked in the grey the chosen clip wears in the clip list.
  The list follows a video that plays the way the clip timeline does, by
  turning the page: while the card of the playhead is in view the list
  stands still, and once the card is out of view, or going out of it at
  the foot, it comes to the top, found by Tim. In a pause between two
  captions of a clip the caption before stays the card until the next
  begins. Only the playhead moving moves the list, so the hand can
  scroll it while the video stands. The playhead put again in the card
  it stands in, and playing, bring the card back into view, wherever the
  hand had scrolled. Cards that come in above the ones in view, as the
  transcript grows, leave the ones in view where they stand: WebKit
  keeps no anchor of its own, so the card of the playhead was pushed out
  at the foot and nothing brought it back.
  A search let go of, by Escape or by taking the words away, goes back to
  the card of the playhead, the one it stood at before or the one clicked
  in the search, never to the top of the whole transcript. The search
  opens at the playhead, and the clip list comes back where it was when
  the search closes, the chosen clip in view: the two take turns in one
  scrolled column, and each came back where the other had been.
- **The space bar still plays.** With nothing typed in the field, the
  space bar plays and stops and the field lets go of the keyboard,
  because a space at the start of nothing is no search, found by Tim.
  Stopping gives the field the keyboard back, so what is typed next is a
  search again, unless the keyboard or the hand went somewhere else
  while it played.
- **Shift and the arrows up and down walk the cards.** While the
  transcript is in the column, they walk its cards the way they walk the
  clip list when that is shown, from the field too. The playhead goes
  onto each card. A card walked to out of view turns the page, the way
  playing does: walked down, it comes to the top, and walked up, to the
  foot, so the page before is shown. A clip walked to in the clip list
  turns the page the same way. Both were brought just into view, and
  then went on standing at the foot as the walk went down, found by
  Tim. The answers to what is
  typed are walked from the first, down, or the last, up, and go round.
  The whole transcript is walked from the card of the playhead, a
  "Not transcribed yet" card among them, one card at a step either way.
  Where a card runs on a little into the part after it, the playhead put
  at the start of that part is in the part, not in the card, so a step
  down no longer stands still at the card before it and a step up no
  longer goes past it, found by Tim. A card clicked or walked to is the
  card of the playhead for as long as the playhead is in it, so a card
  that begins just before a part, a word heard at its edge, lights
  itself when it is clicked and not the part, found by Tim. A search
  walked and then let go of, by Escape, by taking the words away or by
  its cross, puts the playhead and the chosen clip back where they stood
  before the first step, the way a search through a document goes back
  to where it began, found by Tim. Enter at a card walked to stays there,
  and so does a card clicked or the playhead moved off the card walked
  to, because then the person went where they meant to go.
- **A press elsewhere closes a list.** A list open, the font list among
  them, closes the moment anything else in the app is pressed, and the
  keyboard goes to what was pressed, the way a menu on the Mac goes. The
  font list open and the transcript button pressed left both open, with
  the keyboard in the font list's field, found by Tim. See `Pick.svelte`.
- **A failed read is said.** Cards that could not be read leave a line in
  the list, The transcript could not be read, with the reason under the
  pointer, until the next read works.
- **One light.** Whatever is lit is lit one way, whether the hand is on
  it or the keys brought it there. A card under the hand takes the grey
  of the card the keys are on, in the transcript and in the clip list,
  and the clip chosen is lit on the range picker and the clip timeline
  the way the hand on its card lights it. The hand lit a card a step
  less than the keys did, and the keys lit a clip's mark and frame a
  step less than the hand did, found by Tim.
- **One clip in every place.** The hand on a card a clip keeps lights
  that clip on the range picker and the clip timeline, the way the hand
  on its clip card does. In the list only the card under the hand is
  lit, never every card of its clip with it. The space between two cards
  belongs to them, half to the card above and half to the card below, so
  the hand going from one card of a clip to the next never leaves the
  clip dark for a moment, a flicker found by Tim. A clip chosen on the range picker or the clip timeline leaves
  its first card where it is when it is in view, and otherwise puts it at
  the top of the list, where the first card stands, and choosing it
  again does it again. Every card is laid out at its own
  height, never at a height guessed for the cards out of view, because a
  guess put the place off as soon as the cards around it were laid out,
  and WebKit keeps no anchor that would have made up for it. The accent
  edge is the card's button's, so the grey a card takes under the hand
  never covers it.
- **What was not transcribed is said.** With nothing typed, a part of the
  video the transcript has not reached yet is a dashed card in its place,
  "Not transcribed yet", because the search cannot look there. With
  something typed the list holds only what matches. The range picker
  draws the same part under the minutes and the window, always, not only
  while the field is open, so where a search has to transcribe first is
  seen without opening the cards, issue #432. The part is lit from its
  card, and its card from it, only while the cards are open. A part is dots on nothing, a halftone grid, `--halftone`
  in `app.css`, that rises from the foot and thins out upwards, with none
  in the top third, so the minutes and their lines show through. A grey
  wash under the dots hid them, found by Tim, who picked this one from a
  sheet of seven. The dots are the grey of a card's edge at a share of
  its strength, so they are quieter than the dashes and the minute
  lines, because at full strength Tim found them too strong. A row and
  a column left clear round them looked cut off, found by Tim. In a card
  they are whole columns and rows, centred, with at least 2 px of room
  inside the dashes on every side, because dots right on to the edge sat
  too close to the dashes, found by Tim. The grid is worked out from the
  card's own size, and the card's dashes run at the same pitch over
  exactly the span of the dots, so every dash stands over a column of
  dots or beside a row. The dashes the browser
  drew went their own way round the dots, found by Tim. On the range
  picker a part's left and right edges are dashed, a dash beside each row
  of its dots, so where a part not transcribed begins and ends is seen
  even where its dots thin out, Tim's report of 26 October, 12.27. The
  dashes are in the dots' colour, `--halftone-ink`, with their fade, so
  each is as strong as the row beside it: in the grey of a card's edge
  they were too bright, his report of 14.22. The range picker's own border is the edge
  of a part at the start or the end of the video, and its top and foot.
  Its dots cover the whole part, to its foot and its sides,
  as whole columns centred in the part, the same space on either side,
  and the foot row a pixel up from the range picker's foot. A row and a
  column left clear round them looked cut off, a pattern that started
  wherever its grid fell looked off, and the foot row looked clutched to
  the foot, all found by Tim. Lit, the part goes up
  a step with the grey of every lit card, which the card's dashes
  go into, the way every card's edge does, and its dots go up a step
  too, `--halftone-lit`, keeping their fade. On the range picker the
  part takes half that grey, because the whole of it was far too strong
  there, Tim's report of 24 October, and the range picker's own border
  stays over the lit part. The hand on a part lights its card and the
  hand on a card lights its part, both ways round, and the keys on its
  card light the part as well, the same way. With the cards closed, the
  hand on a part on the range picker still lights it. Walked to with the keys it lit nothing on the range picker,
  found by Tim. The parts give way as the work hears, from what is heard, not
  from the transcript, which is saved in pieces: a search hearing half an
  hour took the parts away in thirds, found by Tim. While the work hears
  into a part, its dots stand still, laid from its right edge, which
  never moves, and its left edge wipes over them as it goes. Centred, the
  pattern walked back and forth as the edge passed, found by Tim. Once
  the work stops or waits, a part it heard into keeps its dots where they
  stand, whole columns from its right edge with the room left over on
  the left: centred again, every dot jumped sideways, Tim's report of 24
  October. A part nobody heard into is centred.
- **Escape steps back wherever the keyboard is**, unless the keyboard is
  in another field. The first takes back what was typed, so the whole of
  what was said is there again, and the next closes the search. Tim
  scrolled the sentences, pressed Escape and nothing happened, because
  the field no longer had the keyboard.
- **The sentences are there when it opens.** They are read once the video
  opens and kept, and read again when the transcript hears more, so the
  column never stands empty for a moment after the transcript button.

### Toasts

What goes wrong with no place of its own on screen is said in a toast,
`Toasts.svelte`, with the list in `lib/toast.svelte.ts`. A file dropped
on the app that is not a video, a call to the Go side that failed for a
video with no row or card to say it in, a file Report a Problem leaves
out. Each used to be red text put wherever there was room: a banner over
the workspace that pushed it down, a line over the video preview, three
wrapped lines beside Report. Tim found the workspace moved about by a
dropped screenshot, issues #263 and #327.

A toast lies over the app in the top right corner, under the bar, where
macOS brings its own notes in, and moves nothing under it. Each time
something goes wrong is a toast of its own, the way macOS stacks its
notes: the newest comes in on top and pushes the others down, and each
runs its own time. One toast for each place, which a newer reason
replaced, showed Tim only the last of the pictures he dropped one after
another. The same words from the same place while they show come in on
top again with their time started again, rather than twice. It is
300 px wide, one width for every toast so a stack reads as one column,
which Tim found 340 px a little too wide for. A toast about a video shows while that video is on
screen and waits while another is. Its time, the ten seconds a removed
clip has to be put back, runs down from the moment it comes, in the
same fill, and stands still while the pointer is on it. Its cross takes it away at
once. It wears the red of a removed row over the dark of the app, with
no line round it. The first one waited for the pointer before its time
began and had a thin red line round a grey note, and Tim found both
wrong: a toast counts down by itself, and a look the app has nowhere
else reads as a mistake. Every toast and how it went is in the log,
`toast:` when it came and `toast closed:` or `toast ran out:` when it
went.

What has a place of its own keeps it: a clip that failed says so on its
card, a search that stopped in its row, and what dictation did not hear
beside the microphone. Why is said in a toast, whole, issue #311. A
search that failed says Failed with Continue in its row and why in a
toast, which offers Report: Report a Problem opens with the toast's
words written in its field, after what was there, and a report window
already open is handed them to write in, see `ReportAbout` in
`bugs.go`. Cut to one line in the row, the reason lost the part that
said what to do, and Tim read "Anthropic is busy and did not a…".

A video whose file is gone says so in a toast as the workspace finds it
gone, and its video preview shows nothing of what it showed. A gone file
was once a red line over the workspace, which pushed everything down,
and a toast said it again because the clip list asked for the file. The
clip list of such a video stays as it was. The workspace reads the video
again whenever the library, read as the app comes to the front, says
otherwise than it does about the file, so a file put back in Finder
shows again in the video preview as the sidebar sees it.

### The colour picker

The colours of the captions, **Text**, **Box** and **Highlight** in the
settings column, and the accent's colour wheel in the settings open one
colour picker, `Colour.svelte`. It is the app's own, drawn on bits-ui the
way `Pick.svelte` is, because the colour field it replaces handed the click
to macOS: the pop-up was the system's, looked like another program, could
not be styled, and never said which of its colours was the one in use.

In it, from the quickest pick to the most exact:

- **The colours a caption is most often made in**, round, the way the Mac
  shows a row of colours: white, black, yellow, orange, red, pink, the
  app's purple, blue and green, from `captionColours` in
  `frontend/src/lib/colour.ts`. The one in use wears a ring.
- **A square for the shade and a strip for the hue**, for any other colour.
  The video preview and the clip timeline follow the hand as it drags, and
  the colour is saved when it lets go. The arrows move them too, Shift in
  bigger steps.
- **The pipette**, beside the hex. It closes the pop-up and makes the video
  preview a place to point at: a loupe sits on the pointer the way the
  Mac's own colour sampler does, the pixels under it drawn ten times over
  and the one in its middle marked, with its hex under it. The captions are
  drawn in that colour as the pointer moves, a click takes it, and Escape
  or a click anywhere else leaves the colour as it was. What is read is the
  episode's own frame, not the screen, so the crop, its shade and the
  captions lying over the picture never colour what is taken. The colour
  being picked keeps its ring in the meantime, so it is clear which one
  the picture is giving. See `sampleColour` in `Player.svelte`. WebKit has
  no eyedropper of its own for a page, and it is not needed: the picture
  is the app's to read.
- **The hex**, for a colour known by its number, a brand colour above all,
  with or without the #. It is drawn as it is typed and saved with Enter or
  when it is left.

Return closes the pop-up and keeps the colour it holds. Escape closes it
and puts back the colour it opened on, the way Return and Escape answer a
pop-up everywhere on the Mac. Tim found Escape keeping the shade he had
just dragged to, and Return doing nothing, the wrong way round.

The opacity stays the number beside each colour, so the row keeps its
width.

While a colour was drawn on the way and the last one was still being
saved, the next colour drawn was thrown away the moment it was drawn: the
drawing was held against the captions from before the save, and nothing
watched the save end. The colour field drew again with every movement and
hid it. The pipette draws once per colour, and showed nothing until the
saving was watched.

### Thumbnails

A clip can have any number of thumbnails, or none. A thumbnail is a frame
of the short exactly as the short shows it, captions included, and
**Render** writes each one beside the short as `<name>-1.jpg`, `<name>-2.jpg`
and on. There is no way to render the pictures alone: a change to them is a
change to the clip, and the next render writes the whole of it again. The
reasoning is in [THUMBNAILS.md](THUMBNAILS.md).

- **I and O make a clip at the playhead**, the way In and Out mark a clip in
  every video editor, for a moment the model did not pick. **I** starts
  the clip with the sentence the playhead stands in, from where that
  sentence begins, and grows it forward. **O** ends it with that sentence,
  for a moment noticed only once it has passed, and grows it back. Either
  grows a line at a time until it is as long as **Shortest**, never past
  **Longest**, and then its far edge goes onto a sentence the way every
  clip's does. Mark In and Mark Out are buttons in the row under the clip
  timeline too, with the square brackets editors draw on them, and the
  key is named in each one's title. No model is asked: the pauses are cut and the crop placed
  exactly as for a clip a search found, and after that it is an ordinary
  clip.
  - **Only where no clip is.** With the playhead anywhere from a clip's
    start to its end, cuts included, a clip in the list or one on its way,
    chosen or not, playing or not, the two buttons are off and say why in
    their title, and the keys do nothing. The clip is already there, and
    its edges are dragged to where it should be. It also means a play is
    never started over by I or O: a clip plays only with the playhead
    inside it, and outside every clip the episode plays straight on,
    which a new clip there does not change.
  - **Only where something is said.** I after the last word of the
    video and O before the first make no clip, since no sentence starts
    after the playhead or ends before it. Where the part after the
    playhead, or before it, is heard, and at the very end or the very
    start of the video whatever is heard, the button is off and says so
    in its title, and the key does nothing. Tim found I at the end of the
    video making a card that could only fail, on #456. A part not heard
    yet is heard first, so they are offered there, and a clip that then
    finds nothing said fails with Continue and why in a toast.
  - **The playhead is on the clip it makes.** The clip is made around
    the playhead, so from the frame it is there, on its way and once it
    is written, it is not dimmed, and its crop frame and its captions
    show. While the video plays, the play goes on as it began, through
    the clip's cuts and past its end, and the next press of the space
    bar plays the clip.
  - **Everything shows in the frame the key is pressed in.** The **I** or
    **O** button wears the beam for as long as its clip is on its way.
    The clip's card is in the list at once, in its place in the episode,
    wearing the beam, and it is the chosen card, so it is brought into
    view wherever the list was scrolled to. Where the transcript does not
    reach far enough yet it says Transcribing and fills as the part
    around the playhead is heard, then Placing the crop, and fills again
    as the crop is placed, with the time left. It wore the beam alone
    while its crop was placed, which is most of its wait. Any number can
    be on their way at once, beside a search too, and none waits for a
    search to finish finding.
  - **Its frame is on the clip timeline at once**, fitted, first as long
    as **Shortest** from the playhead, or up to it for **O**, then on its
    sentences as soon as they are known, with its title over it. While
    its crop is placed its pieces are drawn, and its caption blocks come
    in one after another, laid out as the render will draw them. It
    cannot be edited until it is written.
  - **It is followed while it is made.** Its card is brought all the way
    into view once it has slid open and again whenever it moves in the
    list, and with the video paused the playhead goes to where its frame
    starts, then to where its sentences start.
  - **The card becomes the clip** when it lands, still chosen, unless
    another clip has been chosen since. While the video plays it stays
    where it is playing.
  - **Search stays Search.** It is the search's button, so a clip made by hand
    does not turn it into Cancel. Cancel while a search runs stops all
    the work on the clips, the search and every clip on its way, and
    Continue carries all of it on.
  - **It searched nothing**: it counts as no search of any part. It goes
    the way every clip goes, with its trash can.
  - **Cut off by the app closing, stopped or failed**, its card stays
    where the clip would have appeared, still, and a click carries it on.
    Its trash can, there while the pointer is on the card, removes it:
    the card says Removed with Put it back for ten seconds, the way a
    removed clip's row does, and only then is the work called off and its
    record forgotten, so the clip is never made. Its mark leaves the
    range picker at once. The ten seconds are the video's, as a removed
    clip's are: they run on while another video is open, and work whose
    time ran out while it was away is called off on coming back. If the
    work cannot be called off, the card says Stopped again and why is
    said in a toast, with Report.
    Issue #257. A search's cards have a trash can too, which removes the
    search they came from, its row and every card of it at once, see
    above. Issue #431.
- **The thumbnail button**, in the row under the clip timeline, makes the
  frame under the playhead a thumbnail. **T** does the same. It is not a
  mode, so it never looks pressed. Its icon says what a click does: a
  camera adds one, and the camera crossed out, shown while the playhead
  stands on a thumbnail, removes it. It can only be pressed
  with the playhead inside the clip, because a frame that was cut is not in
  the short.
- **Each thumbnail is a mark** centred on the clip's bottom rule on the
  clip timeline, a pixel higher because the camera's body sits below the
  middle of its icon, the same camera filled in the app's light colour, with
  nothing behind it but its shape, so a click on the button is seen
  leaving its picture on the timeline. It turns white while the playhead
  is on its frame and stands centred on the playhead's line. A keyframe
  looks the same way, see Keyframes.
- **The playhead and every mark stand on the start of their frame**, the
  way editors draw them, so a mark the playhead is on is centred on its
  line however far the clip timeline is pinched in. The playhead moves a
  frame at a time, as the video preview does. Plan row 2.234.
- **With the playhead on a thumbnail**, the button or **T** removes it.
  What one click adds, one click takes away.
- **A click on a mark** puts the playhead on it, so the video preview shows
  the picture.
- **A mark is dragged** to another frame. The playhead goes with it, so the
  video preview shows the frame under the hand, and the mark stays inside
  the pieces the clip keeps. It is saved when the hand lets go.
- **A double-click on a mark** removes it, the way it removes a cut.
- **Undo** takes back adding, moving and removing one.
- A trim or a cut that leaves a thumbnail outside the clip hides it. It
  comes back with the part it was in.

### Keyframes

A crop that moves while a clip plays, for a person who shifts in the seat
or walks across the picture. Plan row 3.26, the design is in
[ROTATE-AND-FOLLOW.md](ROTATE-AND-FOLLOW.md).

- **The keyframe button**, beside the thumbnail button under the clip
  timeline, puts a keyframe on the frame under the playhead, with the
  crop where it stands there now. **C** does the same. Like the thumbnail
  button it is not a mode: its icon is a diamond with a plus, and with a
  minus while the playhead stands on a keyframe, where a click removes
  it.
- **No keyframe, or one, is a crop that stands still.** With two or more the
  crop glides from each to the next over the whole time between them,
  starting and settling gently, the way a camera operator pans. The
  tempo is where the keyframes are: two far apart are a slow drift, and
  two a frame apart are an instant move. A keyframe moves the crop of
  the whole clip, and a glide goes on across a cut by the time the
  short plays, so a keyframe before a cut and one after it glide from one
  to the other the way they would were the cut a moment of the clip, and
  nothing jumps at the cut. A glide used to stop at a cut, so those two
  did not glide at all, found by Tim. A part turned with Rotate has
  a crop of its own, so its crop still starts at its cut.
- **Dragging the crop frame places the crop, and nothing else.** With the
  playhead on a keyframe it moves that keyframe. Anywhere else it moves
  the crop of the whole clip, every keyframe of it by the same distance,
  kept inside the picture.
- **The video preview follows the glide frame by frame** while the clip
  plays, so what plays is what renders. The glide is not rounded to a
  pixel of the video, and the short places it to its own pixel, so a slow
  glide moves on every frame instead of standing and jumping.
- **Each keyframe is a diamond** centred on the clip's top rule on the
  clip timeline, drawn and lit the way a thumbnail is: filled in the
  app's light colour, and white while the playhead is on its frame, the
  clip's first and last frame included. It is 18 px, larger than it was
  at first, after Tim found it too small. A thumbnail and a keyframe on
  one frame stand at either end of the playhead's line, which runs 12 px
  on past the clip timeline's top and bottom border, past both marks. It is the thumbnail's kind of mark: a click puts the
  playhead on it, a drag moves it to another frame with its crop, and a
  double-click removes it. While it is dragged it stands on the start of
  the frame under the hand, where it stays once let go, so dragged past
  the clip's edge it stops on the first or last frame and does not jump.
- **Automatic crop** removes every keyframe of the clip and brings back
  the crop the engine found, and **Put the crop back** brings back the
  keyframes with the crop. Removing the first of two keyframes leaves the
  crop standing at the other for the whole clip, as removing a keyframe
  does in Resolve and Premiere. Rotating a part keeps its keyframes. Each
  was placed on the picture turned one way, and steers the part only while
  it is turned that way: turned another way, it waits, and four turns
  bring its glide back. A crop placed on the frame of a keyframe that
  waits takes it to the picture as it stands. Tim found the diamonds gone
  after a turn, issue #348.
- **Undo** takes back adding, moving and removing one.

### Rotate

For a phone that was turned while it recorded, so a part of the video
lies on its side. Plan row 3.25, the design is in
[ROTATE-AND-FOLLOW.md](ROTATE-AND-FOLLOW.md).

- **The Rotate button**, beside the thumbnail button under the clip
  timeline, rotates the part of the clip the playhead stands in a quarter
  turn to the left, the way Rotate Left in Preview and Photos does. **R**
  does the same. A part is what plays straight on between two cuts, or
  between a cut and an edge of the clip, so the spin of the phone is cut
  out with a double-click first, and the part after it is rotated. Four
  presses bring a part back.
- **The video preview rotates the picture at once**, in the frame the key
  is pressed in. A part rotated a quarter shows the picture the other way
  up, as big as it fits in the video preview, and the crop frame and the
  captions stand on it rotated. The picture turns with the frame on the
  video preview, not with the playhead, which runs ahead of its frames
  when it is dragged: over a cut, the picture turns when the first frame
  from the other side is there.
- **A rotation belongs to the video, not to the cuts.** Rotate turns the
  stretch of the video the part shows, from the middle of the cut before
  it to the middle of the cut after, where the phone was turned. A part at
  the edge of the clip turns to that end of the video, so dragging the
  clip's edge out keeps showing the footage turned. Putting a cut back or
  making a new one never changes how a frame is turned, and a crop placed
  on a rotated part stays with its frames.
- **The crop of a rotated part starts in the middle** of the picture
  rotated, and is dragged like any other. It moves the pieces of the clip
  rotated alike and no other. Rotated back all the way, the part has the
  crop it had before, which rotating never touched.
- **A rotated part has the Rotate icon in its middle** on the clip
  timeline, large, in the accent at half strength so the waveform shows
  through it, with a dark glow round it that parts it from the waveform
  and the empty track alike. It is as tall as the track leaves between the
  keyframes and the thumbnails, shrinks with the part as the clip timeline
  is zoomed out, and stays sharp at every size. Tim's idea, issue #348,
  after a small mark in the part's corner ran into the keyframes and an
  icon really cut out of the wash showed black in one place and grey in
  the next, the wash drawn over it a second time was too faint, and a
  solid dark was too strong. Tim chose it from a sheet of sixteen. It
  takes no click.
- **Undo** takes a rotation back like any other edit.
- **A phone's video stands up by itself.** A phone that records upright
  stores the picture lying down and says in the file how to stand it up.
  The app, the video preview and the short all stand it up, so Rotate is
  only for a turn the phone did not notice.

### Caption timing

A caption appears when its first word is said and goes when the next one
appears, or a moment after its last word when a pause follows. Where the
timing of the words is a little off from what is heard, a caption can be
moved by hand, on the clip timeline.

- **The captions run across the middle of the clip timeline**, each a
  block from where it appears to where it goes, on the same clock as
  everything else on it. A click on a block puts the playhead on that
  caption's first word, and a double-click cuts a part out there, the way
  it does anywhere in the clip. The block used to keep the press to
  itself, so a double-click on it, across the middle of the clip, did
  nothing.
- **Either edge is dragged, and moves its own caption only.** Where two
  captions meet, the line between them has two sides: the left side is
  where the one before goes, the right side where the one after appears.
  An edge dragged in leaves a gap, and dragged out it stops at the caption
  beside it. No other caption ever moves. Dragging the right side used to
  take the caption before along, so pulling a left edge in over a word
  drew the caption before into the room Tim had just cleared. Nothing
  overlaps, because the picture shows one caption at a time. An edge
  lands on a whole frame.
- **With shift an edge lands on a word**, the way an edge of the clip and
  of a cut does: a start where the nearest word starts, an end where the
  nearest word ends, and only in the direction the hand goes, see
  `captionStop` in `engine/edit.go`. Shift is read on every move, so
  letting go of it part way goes back to frames. A word the edge took out
  is a word to land on going back. The edge stands where the word does,
  as a caption laid out from its words does, not on a frame. The playhead
  goes with it, for an end onto the last frame the caption is up on, and
  for a start into its first word, the way a click on the caption goes,
  because the frame that holds a word's start begins before it, where the
  caption is not up yet. Tim saw nothing to snap to there. A click on the
  start edge puts the playhead there too. The edges of a caption used to move a frame
  at a time with shift or without, found by Tim.
- **An edge dragged over a word removes it.** Dragged in, an edge takes
  out of its caption every word whose middle it passes, the way delete
  removes a word, so pulling the left edge of "one two three" past "one"
  reads "two three", and the right edge past "three" reads "one two".
  Dragged back out over a word it took, the word comes back. The middle
  is of the part of the word the clip shows, so a word the start of the
  clip cuts into is passed in that part. The last word stays, because
  delete removes a caption whole. Which words an edge passes and where it
  stops is the engine's to say, `ShapeCaptionTime`: the clip timeline asks
  it while the hand moves and draws its answer, and the caption box in the
  video preview shows the words it leaves. When the edge is let go it is
  saved where it was drawn, `SetCaptionTime`, with the words, and Cmd+Z
  takes back the words and the edge in one step. The interface once kept a
  copy of the rule, which drifted from the engine's: an edge was drawn in
  one place and saved in another, and a word the start of the clip cut
  into left the caption box during the drag and came back when let go.
- **Removing a word at an edge draws the caption in, as dragging over it
  does.** A caption whose first or last word is removed in the caption box
  draws in to the words it shows: it appears with its first word shown and
  goes a moment after its last. An edge moved in by hand stays where it
  was put, and one moved out comes in with the word. The captions beside
  it keep their time, so the room the word took stays empty. Before, the
  caption kept the time the word had, and the two ways of taking a word
  off its edge ended differently, which Tim found. The word typed back
  takes the edge back out to where the hand put it.
- **A double-click puts an edge back where all its words put it**, the
  words taken out at that edge coming back with it, whether a drag or the
  caption box took them. It can be done to any edge moved by hand or
  drawn in with its word, so it undoes any number of drags. It used to
  leave the words out, which put a dragged edge only where the words left
  put it, about where it already stood: Tim dragged a right edge in and
  double-clicked twice, and the second time nothing moved.
- **The captions in the video preview follow the drag**, so what is
  seen while dragging is what is saved.
- **A block is its caption in the short's colours**: the box colour, as
  see-through as the box is set to be, over the waveform the way the box
  lies over the picture, with a bar in the colour of the words, so every
  colour and every opacity picked in the captions column is seen here too,
  while it is picked. The blocks lie over everything else on the clip
  timeline, the clip frame, the time lines and the playhead included. The
  colours never change with the state, because a dimmed colour is another
  colour. At rest the bar is a hairline and under the pointer it is
  thicker. The caption the video preview is showing wears the highlight
  colour over its box, with its own opacity, the pill the spoken word
  wears, and pops as that pill
  bounces, on every word as the pill does, so walking the words with shift
  and the arrow keys pops the pill and the block together. It pops when
  the playhead comes to a new word and never because an edge is dragged:
  while one is, the block is lit as the caption is saved, and a word is
  known by when it starts, so taking out a word before it is no new word.
  The block popped as a dragged left edge passed the playhead, which a
  click on the block puts at its first word, and a dragged right edge
  never came near it, so the two edges looked different, found by Tim. A block takes no focus, so no focus ring is left
  round one when the keys walk the words on. A click puts the
  playhead a frame into the caption's first word, so the video preview
  shows that word lit, the first caption of a clip included, which is on
  screen from the clip's first frame, before its first word.
- **A word is lit from its start until the next word starts**, and the
  last word of a caption until the caption goes, in the video preview, on
  the clip timeline and in the render alike: `litWord` in
  `frontend/src/lib/flow.ts` for the two in the app and the highlight in
  `engine/highlight.go` for the short. The video preview used to light a
  word only until its own end, so the highlight went out while a word was
  still being said and between every two words, where the short and the
  clip timeline went on showing it. The edge
  under the pointer, or the one being dragged, is a white line with a dark
  edge that goes again when the hand lets go. Two captions that meet have
  a gap of two pixels between them.
- **A press on an edge puts the playhead on it**, the way a clip edge does,
  which is how a caption is heard from where it appears: on the frame it
  appears on, or the last frame it is up on. While the edge is dragged
  the playhead goes with it, so the video preview shows the frame the
  caption now starts or ends on. It used to go there only when the hand
  let go, and not at all in a drag, the one thing on the clip timeline
  left that way after Tim's report, see One press for the whole clip
  timeline.

A moved caption is kept in the clip's plan against the word it begins or
ends on, by when that word starts in the episode, so it survives the
clip's pieces moving, and a size or face that breaks the captions in other
places simply leaves it unused. The engine side is `SetCaptionTime` in
`engine/edit.go` and `moveCaptions` in `engine/lines.go`.

### Undo and redo

**Everything done to an episode's clips can be taken back**, with **Undo**
and **Redo** in the Edit menu, Cmd-Z and Cmd-Y, Ctrl-Z and Ctrl-Y on Windows and Linux. That is a trim, a
cut made, moved or put back, the crop frame, the caption box moved, a word
corrected, added or removed, the caption face, size and colours, a clip removed,
a search removed, and the window on the range picker moved, made longer or
shorter, or put back with a double-click. Each is one step. The window a
search moves on by itself is no step, and undoing a step of the window
puts it back where the hand had it whatever a search did with it since. Taking one back chooses the clip it
changed, so nothing changes where nobody is looking. A new edit after an
undo starts again from there, and what was undone is gone, as in every
editor.

- **What is in it.** What a person does to the clips, and nothing the app
  does by itself. Transcribing, searching and rendering make things rather
  than change them. Where a render put a short is written in the clip set,
  but it is no edit: an undo neither takes it back nor stops at it. Moving the playhead, choosing a clip and zooming are
  not edits. The settings are not in it, apart from the caption height,
  which is moved in the workspace like everything else here.
- **The playhead goes back with the edge it moved.** Dragging an edge or a
  cut on the clip timeline takes the playhead along, so the hand moved
  both, and taking the edge back puts the playhead back where it stood
  when the hand took hold. Redo puts it where the drag left it. Moving the
  playhead alone is still no step. A drag made while the clip played moved
  no playhead, so its step has none to put back, and an undo while the
  clip plays leaves the playhead playing.
- **A field keeps its own undo.** While a word in the caption box or a
  number beside the clip is being typed in, Cmd-Z takes back the typing,
  the way it does in any text field. Once it is saved, the key goes to the
  episode.
- **One history per episode, while the app is open.** Everything saves the
  moment it is done, so closing loses nothing, and there is no history to
  come back to. It goes 200 steps back.
- **What landed since stays.** A search writes clips into a plan while its
  earlier clips are edited, so an undo puts back only what the edit
  changed, clip by clip, and never the whole plan. A clip that lands at
  the very moment an edit is saved is not part of that edit either, since
  an edit never makes a clip. A clip that has been
  changed again since by something the history does not know about, a
  search over the same part above all, is not written over: the undo
  says it cannot be taken back, and the history of the episode starts
  again from there.

The engine side is `engine/undo.go`, the app side
`cmd/framefairy-app/history.go`, and the menu `cmd/framefairy-app/menu.go`.
The menu has to be the app's own: on macOS the stock Undo takes Cmd-Z
before the page sees it and hands it to the web view, whose undo only
knows about text being typed.

### Work in hand

Everything that runs says so the same way, wherever it runs. There are four
things and no others, and each one means one thing.

- **The beam.** Light runs clockwise round the edge of the control the work
  was started from, for as long as the work runs. A round is five turns and
  no two of them alike: the comet gets away, eases right off through one
  long side, comes back hard through the next, sits down again and finishes
  quickly, so the slow part falls somewhere else on the edge each turn, and
  a broad faint wash goes round three times in the same time, so the two
  are never in the same place twice. It never stands still and it never
  goes back, because a light that hesitates on a border reads as broken and
  one that backs up reads as a stutter. Sampled out of the browser over a
  round: five turns, no step backwards, and between 0.59 and 1.63 of the
  even pace.
- **The motes.** Specks of that light drift up through the control, behind
  its own words. They are the beam's, not a thing of their own, and they
  are why a control with nothing to report is still alive to look at.
- **The fill.** How far the work has come, when that is known. Inside the
  control it is a wash of the app's colour, 30 percent at the start and 50
  at the head, with a bright line at the front, so what is done can be told
  from what is left with daylight on the screen, and a light passes over
  what is done, so a fill is never a flat wash. The fill slides by a transform rather than growing, so it
  moves with whatever moves beside it. The range picker draws the reading
  of the episode with `Busy.svelte` itself, fill, head, glow and motes,
  without the rim, and the row of the clip list that waits for the
  transcript fills by the same line, so the two never say different
  things.
- **Paused work is still, not gone.** How far it got stays true, so the
  fill stays where it is, and everything that says the work is running
  stops: no beam, no motes, no light over the fill, and the head keeps its
  line without the glow ahead of it. Running and paused are told apart by
  movement. The range picker shows a transcription stopped at a window,
  or waiting while a search has the machine, this way.
- **The shimmer.** A place that is not filled yet: the rows the clip list
  will have, which are already rows of the list and brighten under the
  pointer the way a clip's row does while they go on breathing, the part of the clip timeline the transcript has not reached,
  the window on the range picker while clips are being found for it. The
  place itself dims and comes back, two seconds, in and out. It is
  `animate-pulse`, which is what
  [shadcn/ui](https://www.shadcn-svelte.com/docs/components/skeleton) and
  Tailwind ship and what most of the web wears, and it is the whole of it:
  no band, no sweep, no light crossing anything. In a list each row starts
  a step after the row above it, so the breath runs down the column rather
  than every row rising and falling together. Four brighter ways of doing
  this were tried and looked at side by side, and this is the one that was
  chosen. They all stay in `make motion` to be compared again, where they
  cost the app nothing.
- **The pulse.** Work running somewhere else. The dot beside an episode in
  the sidebar keeps its size and its place, and a ring widens out of them and fades.

The colours are the app's own throughout, mixed from the accent, so
changing it in the settings moves the beam, the fill and the shimmer with
everything else. On a control that is already the app's colour the light is
white instead, because the app's colour cannot be seen on itself. With
**Reduce motion** on in the system nothing moves: the beam is a steady rim,
the motes are not drawn, and the shimmer and the fill stand still.

All five on one page, without starting five jobs in the app and catching
each at the right moment:

```
make motion
```

It opens a page in the browser with every one of them running, at every
share and in every kind of control, and a colour picker at the top so the
whole set can be seen in another accent. The shimmer stands there five
ways: react-loading-skeleton's sweep, MUI's wave, the breath that
shadcn/ui and Tailwind ship and that the app wears, the breath and the
wave together, and a foil of our own. It is preview material in
`frontend/preview/motion/` and never goes into the app: the product only
serves making shorts.

All of it is the stylesheet's. Nothing here runs JavaScript, nothing asks
for a frame and nothing measures anything: every moving part animates a
transform or an opacity, which the compositor carries without painting
again, and the one curve that is not a plain ease is a CSS `linear()`.
Measured in headless Chromium: 60 frames a second with 114 of them running
at once on the bench, 60 with a search running in the workspace, and no
animation at all at rest.

The one exception is time running out, the fill of a removed clip. Its
animation is still the compositor's, but it is started, held and let go
from `Busy.svelte`, and a clock there decides when the time is over. The
clock is the time. Held, the fill stands where the clock says, a plain
transform with no animation. Let go, it is a new animation from there to
nothing over the time left. An animation is never paused and played
again: WebKit, which draws the app on a Mac, plays a paused animation on
from its timeline's last frame rather than from now, so the fill jumped
ahead as the pointer left the row and back as it came in, and read as
time given back. A play state flipped by `:hover` in the stylesheet did
the same and really gave the time back.

`frontend/src/components/Busy.svelte` is the whole of the beam, the motes
and the fill, in a control and in the track. The track itself, the shimmer
and the pulse are in `frontend/src/app.css`, because they are worn by
things that are not controls. The track is `.progress` there and not
`.bar`, because the bar is the one across the top of the app, and while
the two shared a name the bar was picking up the track's rounded corners.
A way of showing work that looks like one of these is one of these, given
what differs as a prop or a colour its host sets, never a copy.

### What the interface may ask for

Every call that names a file is checked against the library before anything
is read, written or started, and a path is judged by where it really leads.
The interface only ever names files it was given, so this is the last line
rather than the first, but it is the line that holds when something else
asks.

A video that is a link, or a work folder beside it that is a link or no
folder, is no episode. It is refused when it is added, with the reason,
and an episode whose folder becomes one later drops out of the library
until it is put right. A link inside a work folder was always judged by
where it leads, but the folder itself was taken as found, and a zip with
`episode.mp4` and an `episode.framefairy` leading to the home folder made
every file there the episode's.

A plan is checked against its episode too, not only against the library.
It has to be in that episode's own logs folder, so a plan of one episode
given with the path of another is refused, rather than edited against the
wrong video, the wrong words and the wrong history.

### Work in the background

Every piece of work shows where it was started and nowhere else: a
transcription on the episode's lamp and the range picker, a search in the
clip list with **Cancel** and **Continue**, a render on its button, a
model being fetched on its row in the settings. Transcription runs in its
own lane, so finding and rendering clips never wait for it. The other way
round, a search finding clips with the model on this machine holds
transcription up while it finds, since the two models share the machine.
A model in the cloud, or llama-server on another machine given as its
address, shares nothing with it and holds nothing up, issue #276.

There used to be an **Activity** page that listed every job again, with
its progress, **Cancel**, **Clear finished** and the job's log. Nobody
used it, because everything on it was already on screen where the work
was, so it was removed. What only it had, the log, is a file now: every
job of an episode writes what it says, detail lines too, to
`jobs/<time>-<kind>.log` in the episode's work folder, beside the records of its jobs, and the newest
twenty are kept, see `engine.OpenJobLog`. That file is where a failure is
looked into. Work that belongs to no episode, a model being fetched,
keeps none.

The list of jobs is kept current by the news the Go side sends about every job.
Each piece of news carries a number that grows with every change, so a
piece that arrives late never puts a job back to where it was. While
anything runs, the list is also read again every five seconds, so a piece
of news that was lost costs a few seconds and never a job that looks as if
it runs for ever. Quitting the app stops the model and every job before
the app goes.

**Quitting.** Cmd+Q dims the app at once and says what happens next.
The first press only asks, Press ⌘Q again to quit, for three seconds, the
way Chrome does. It does so on every page and whether work runs or not:
it asked only while work ran, so the workspace, where something nearly
always runs, took two presses and the settings page quit on the first. A
click or Escape takes the question away, and the next press asks again:
the press behind a question taken away still counted once, so ⌘Q,
Escape and ⌘Q on the next page quit at once. The second press says Quitting
while the app stops everything it started, and then it goes. Closing the
app's window quits without asking, and so does **Relaunch** on the
Updates page, which was asked for with a click. The stopping happens away
from the main thread, see `cmd/framefairy-app/quit.go`: done on it, it
froze the app under the spinning wheel for five seconds.

### Settings

The settings are made the way the **Updates** page is: a column of cards,
each card one subject, each row a mark, what it is in a few words with a
line under them, and at its end the one thing to do about it. The card and
its rows are `.card` in `app.css`, shared by both pages. Everything saves
as it is changed, the way System Settings does on the Mac, so there is no
**Save** button and nothing to forget to press. A field saves a moment
after the typing stops.

**What is in the way is said where it is put right**, not in a list of
its own. A missing API key is said on the row where the key goes, a
missing llama-server under the choice of model, a missing ffmpeg under
**Shorts**, a missing speech model on its row, which offers **Install**.
A problem the page has no row for, which none of today's checks is, is
said at the top so nothing goes unsaid. The first version had one card at
the top listing every problem, and a person had to go from the top of the
page to the bottom to find where to fix what it said.

**Finding clips is one choice**, **Find clips with**, in one list: under
**In the cloud** a model from each company the app knows, Claude Sonnet 5
from Anthropic and GPT-6 Sol from OpenAI, with the company beside each,
and under **On this machine** every model that runs here, the way apps that
offer models list them by where they run. The rows that depend on the
choice are under it in the same card, the way a pop-up in the Mac's own
settings changes the rows beneath it: the key of the company whose model it
is, and with a local model what it needs and, when it is missing,
llama-server. The first version had a card of two ways and a second card of
models under it, one deciding the other, and the two read as things that
had nothing to do with each other. It also had room for one company only.
Nobody should need an account with a particular company to use the app,
so every company is a row of the same list, and a third is an entry in
`engine/provider.go` and nothing more.

**Each company's key is its own.** The key row names the company of the
model chosen, **OpenAI API key** or **Anthropic API key**, and where to get
one. Keys are kept apart in the keychain, so trying one company never costs
the other's key, and going back finds it still there. Without its key the
choice wears the warning like a model not downloaded: the triangle, the
frame round the list and the line under the key's name. A model written in
by hand under **Advanced**, **Model in the cloud**, belongs to the company
its name says, `claude-` or `gpt-`, and is in the list too, so the list
never names nothing.

The decisions behind the row, which company a model belongs to, what the
list offers, and what the line says and how it stands in every state, are
in `frontend/src/lib/finding.ts`, and `finding.test.ts` walks every state
somebody can click their way into, run by `make interface` and CI.

The line under the choice says what it costs: a few cents a video and
which company, or who made the model, the memory it needs and whether it fits.
Beside each model in the list is what it would cost to fetch, or that it is
the best here, or that it is too big for this machine. The list holds any
number of models in the room of one control.

**Choosing a model chooses it**, whether it is on this machine or not.
The settings name it, and the list ticks it. One that is not here yet
cannot find clips until it is downloaded, and everything about the row
says so in the colour of a warning: the triangle with the exclamation mark
before it, the frame round the list, and the line under it, **Not
downloaded yet** and its size. **Download** beside the list fetches it: a
download of gigabytes starts when it is pressed, never as a side effect of
looking through a list. While it runs the line says how far it has come and
how long is left, **Cancel** carries the beam and the fill, and the list
waits. When it is there the row turns to the check, with nothing more to
do, because the model was already the one chosen. A download cancelled
leaves the choice as it was, and **Download** carries on from what arrived.
Keeping the model before it in use until the download was done was tried
first, and the list then ticked one model while the line said the clips
came from another.

**The line under a name is one line.** It is short enough to fit beside
the list and a button, and anything longer is cut short with the whole of
it in the line's title, because a line that wraps when a button arrives
beside it moves the whole card. What the choice costs, who made a model
and how much it asks of the machine, is on the line. Everything else is
for the info mark. After a model is chosen the page reads the models again,
because the list says which one is in use from them: reading only the
settings once left the list on the model before, so a choice looked as if
it had done nothing.

Its info mark, at the right end of the heading, says what
each way costs and, with a local model chosen, how much memory this
machine has and what that decides. The key goes in the keychain the
moment **Save key** is pressed, not with the rest of the settings, because
it never lands in the settings file. An app opened from Finder has no shell
environment, so this is the only way to give it a key. Before it is kept,
the key is shown to its company, which asks for the list of models and
costs nothing, and while that runs the button says Checking. A key the
company refuses is said in the key's own row, in red, and not kept, so a
wrong key is found where it was typed rather than when the first search
fails. A company that cannot be reached says nothing about the key, so it
is kept. A refusal is one short line: **Anthropic did not accept this
key**, or, for something else copied from the same page, like the key's ID,
**Not an Anthropic key. Those start with sk-ant-**. The field takes it the
way the Mac's password fields take a wrong password: it shakes, keeps what
was typed, selected, with the keyboard in it, so the next paste replaces
it, and Save waits until the field holds something else, since the same
text would be refused again. Without a key the row says **None yet. Get one
at platform.claude.com**, or platform.openai.com, and the address is a link
that opens the page where keys are made in the browser. The app only ever
opens the two addresses written in `engine/provider.go`. A saved key is shown in
short, the way the companies list keys, `sk-ant-api03...MwAA`, its first
twelve characters and its last four. The short form is kept as the
keychain item's comment, which macOS lets the app read without the key,
so showing it never asks for anything. The field is narrow, since what is
typed shows as dots. The trash can beside it removes the saved key, and
asks first, because the app keeps no other copy. Without a saved key it
keeps its room, so the field does not move. A key in `ANTHROPIC_API_KEY`
or `OPENAI_API_KEY`, which an app started from a terminal has, is shown in
short too, with where it comes from in the title, until a key is saved,
and a saved key is the one used. It cannot be removed here.

A key refused in the middle of a search says so in words on the search's
row, and says where the key came from: the saved key, to be replaced in
the settings, or the variable it was read from. The company's own answer
is kept in the episode's `logs` folder, as every refused request is.

**The settings are not left while clips cannot be found.** A model
chosen and not downloaded, a model in the cloud without its key, nothing
chosen, or llama-server missing: going on from any of them is going on to
an app whose every search fails, far from the one place it can be put
right. So a move to another page, from the sidebar or the menu, is refused,
and the card comes into view and shakes, the way the Mac's own password
field does when it will not let somebody in, with the keyboard on the
list, where choosing a model that is here or the one with a key puts it
right. A download on its way holds the app too, because a model is no use
until the last of it has arrived. Only a page still reading what it has
to show does not. The
settings hold the app through `nav.hold` in `lib/state.svelte.ts`, which
every move between pages asks first, and when they hold it is decided in
`lib/finding.ts`, with its tests.

**Downloaded models** is the last row of the card, folded, with how many
there are and the room they take. It is only about that room: opened, it
lists them with their size on disk and a trash can each, quiet until the
pointer reaches it. Removing one asks first in the box over the app,
because fetching it again is gigabytes: **Cancel**, where the keyboard
starts, and **Remove**. While a model is being installed the trash cans
wait, dimmed. A model is not removed while it is being installed. Removing
the model in use lets go of it in the settings too, and the page reads the
settings again rather than saving over that.

**What a search uses stays as it is until it is done**, from the moment it
starts, while it still hears the episode too. Choosing another model in
**Find clips with**, removing a model, or saving or removing the key is
refused while a search runs, and a speech model is not removed while a
search or a clip made by hand hears with it. The list does not open and no
box asks: the card the choice belongs to shakes, the way it does when the
settings may not be left, and the control's title says why. The line under
**Find clips with** says a search is finding clips with it, and its dot
pulses, the way work running somewhere else shows. The settings are read
again when the finding begins, so a model chosen in between would have been
the one the search found with, and a model removed was a file gone from
under it. The Go side refuses the same changes, whatever asks for them,
see `languageLocked` in `setup.go`.

Two other ways of choosing a model were tried before this one and left: a
card of rows with a round mark each, which took a row per model and grew
with every model added, and a row of tiles by size, which does not hold
four models and put names like Largest on models that are only larger than
each other.

A second model arriving by itself never takes the first one's place. When
the settings name none, the one model on the machine is in use by being the
only one, and a download that finishes names it in the settings. A machine
that already has two and none named, from before this was so, says so
under the choice, and choosing one settles it.

**Speech** is one model today, so it is a row rather than a choice: a
check before it once it is there, what it covers and the room it takes.
Removing the speech model means the app asks for one again the next time it
starts, because nothing can be transcribed without it.

**Shorts** says where rendered shorts go, next to each video unless a
folder is chosen with **Choose…**, which
opens the system's own folder dialog. **Next to video** goes back to the
default. In a folder chosen here, each episode's shorts go into a folder
of their own, named after the episode's file, so `Folge 12.mp4` renders
into `Shorts/Folge 12/`. A short is named after its clip's words, without
the clip's id, `ich-merke-dass-das-immer-schwer-ist.mp4`. Wherever a short
goes, a name taken by a file the clip did not write gets a number, the way
Finder gives one, `ich-merke-dass-das-immer-schwer-ist 2.mp4`. A short already rendered stays where it
went when the folder changes, and its clip still knows it is rendered and
where, see **Where a short went** in
[ENGINE.md](ENGINE.md#where-a-short-went).

**Appearance** holds the **accent colour**, what the app picks things out
in: the chosen clip, the window on the range picker, a button that matters.
It is the Mac's own row of round colours, with the app's purple, `#942192`,
first, and a colour wheel at the end for any other, which opens the app's
colour picker, see **The colour picker** under the workspace, without
presets of its own, because the row before it is its presets. The app's colour is the
only one the interface has: the lighter shade under the pointer and the
wash behind a chosen clip are mixed from it, so changing it moves all
three, and it moves as the colour is picked. The colours burned into a
short, the words, their box and the pill behind the word being spoken, are
set together in the captions column of the workspace, where the video
preview and the clip timeline show them as they are picked. A taste in one
is not a taste in the other. A highlight colour chosen here before is still
the one a search gets until its own is picked in the captions column.

**The tools are the app's own.** The app runs the ffmpeg, ffprobe and
llama-server beside it and no others: not one on the search path, and not
one named in the settings, which it used to run whatever they named. Each
is checked before it runs to be the file the app was built with: `make`
takes the SHA-256 of each tool it puts beside the programs and builds it
into them, and a tool that does not match, or one no sum was built in
for, is not run. A tool that is missing or does not match is said in the
check under **Advanced**, in words, rather than replaced by another.

**The check says which tools these are.** Each of ffmpeg, ffprobe and
llama-server has its row, with the version it says it is beside its name,
the file, its SHA-256, and whether that is the sum built into the app.
**Check again** reads every file again rather than trusting what it read
before, so a tool changed since it last ran is caught by looking, not
only by the next run that refuses it. llama-server says its build, b and
the number, because the build passes the pinned build number to
llama.cpp: a clone of one commit had every server saying it was build 1.

**The tools come with the app.** They are inside it, so an update brings
the ones its build was made with, and there is nothing of them to update
on their own: a version raised in a build script is a new app, and the
next update is the new tools. A customer never runs older tools than the
app they have. The tests and the command line can name others in the
environment, `FRAMEFAIRY_FFMPEG`, `FRAMEFAIRY_FFPROBE`,
`FRAMEFAIRY_LLAMA_SERVER` and `FRAMEFAIRY_ASR_MODEL` for the speech model,
and the app as it ships takes none of them, see `named` in
`engine/tools.go`. On a Mac any program of the same user can set a
variable the next launch of the app reads, and what it named would run
unchecked with the app's access to the person's folders.

**Training data** says how many records there are and has the one trash
can that throws them away, which asks first.

**Advanced** is closed until it is opened, like a disclosure in Finder, and
holds the machinery: the path to the language model file, or the Claude
model, the speech model folder and the training data folder, each a name
on the left and a field on the right. There is no field for ffmpeg or
llama-server any more, see **The tools are the app's own**. Empty paths use
the same defaults as the command line. Under the paths is everything the
check looked at, found or not, with the check's own words and **Check
again**, though the check runs again by itself whenever a setting changes:
which ffmpeg,
which of the system's own video decoders it can use, VideoToolbox on a Mac
or decoding on the processor, the caption faces the app carries inside
itself, and which model files it found. Which decoder a file really went
through is in the log of its search.

Between the paths and the check is **The app's log**, see
[LOGGING.md](LOGGING.md): where the file in use is, how many files it has
with the older ones, their size and their lines, read again whenever
Advanced opens. The folder button shows it in Finder, the way Show in
folder does for a video in the sidebar, and the trash can removes every file of the
log, the older ones too and nothing else in their folder, after asking.
The app starts a new log with its next line, which says what went.

What belongs to the episode being worked on is not here: the caption face,
the size and the height, and how many clips a search looks for and how
long they may be, all sit in the workspace next to the clip, and what is
set there is kept for the next episode.

### Acknowledgements

In the Help menu, which is where apps keep them, rather than on the rail
beside the things making shorts needs. Every piece of other people's work
the app is made of or brings with it,
grouped by where it is: the app, its interface, speech recognition, ffmpeg,
llama-server, the caption fonts, and the models the app fetches from their
makers. A row is the name, the version and the licence, and it opens to what
the licence asks to be said and to the licence's own text. The list is built into the app from
`notices/`, and how it is made and kept complete is in
[THIRD_PARTY.md](THIRD_PARTY.md).

### Report a Problem

**Bugs**, with a bug, at the foot of the sidebar between Settings and
Updates, opens Report a Problem in a window of its own, plan row 2.244.
A window and not a page, so a person can talk about a problem while
they go through it in the app. It opens beside the app's window where
the screen has room, and there is one at a time: Bugs while it is open
brings it forward, and two quick clicks make one window. Bugs wears the
look of the page on screen while the window is open, behind the app's
window too, and not only while it is in front: lit, it says there is a
report waiting, and a click brings it back. Closing the app's window
closes it too. The report window has a title bar of its own, measured
on its own, so with the app's window fullscreen its name still stays
clear of its own buttons. It was in the
Help menu before, where Mac apps keep it, and Tim wanted it where a
person looks while working.

One field for what happened, and one row under it: **Dictate**, a
square with the microphone alone, **Attach**, a square with the
paperclip, and **Report**, which saves one zip file on the Desktop,
shows it in Finder and closes the window: what was written, the
pictures, the app's log, the build and the Mac, and the settings, with
the home folder written as ~ and nothing of a video but its file name.
Beside the buttons there is only what went wrong, and why a recording
stopped by itself. What the buttons do shows on them, and the info mark
beside **Report** says how the page works and what goes in a report.
Tim found standing lines there, Listening and Saved on the Desktop,
read as news that was not true yet.

**Clear**, then **Report**, the buttons of words on the right as the
buttons of icons are on the left, issue #415. Clear empties the report,
what was written and every picture and recording. It asks first in the
box over the page, "Everything you wrote and attached will be deleted.
You can undo it with Cmd+Z." Cmd+Z then brings it all back and Cmd+Y
clears it again, as Tim asked, until something else is typed, attached
or removed, or the window is closed. The words go as one step of the field's own undo and the
pictures follow it, removed on the Go side the way the bin removes one,
so the app's copies go at the next start or the next report. Report and
Clear are off while there is nothing to report, no words and no
pictures.

Tab goes from the field to Report, then through the pictures and back
to the field, and Shift+Tab the other way, issue #404, so Enter, Tab and
Enter end a dictation and make the report. The page moves the focus
itself: macOS tabs past buttons unless full keyboard access is on, so
Tab went from the field to the first picture, or with none to nothing
Tim could see. Each move asks for the focus ring, because WebKit draws
none on a focus the page moved after a click into the field. Tab pressed
while the last words of a dictation are still written down leaves the
focus on Report once they are in. Report pressed while the last words of a dictation are
still written down says Making at once and waits for them, so they are
in the report. While dictating Tab does nothing: only Enter, Escape and
the microphone end a dictation, as Tim asked.

The pictures are for screenshots and screen recordings. They come from
the paperclip's open box, from a drop anywhere on the window, or from a
paste into the field. Ten at most and 10 MB each. A picture is PNG or
JPEG, decoded and written again as PNG, so nothing the file held besides
the picture travels, a place or a camera among it. A recording is what
the usual screen recorders make, issue #326: a video the app takes,
see [Which files are videos](#which-files-are-videos), which is what the
Mac's own recorder, QuickTime, Windows, phones and OBS make, or WebM,
which the GNOME and KDE recorders and browsers make. It is judged by its
first bytes and then by the ffmpeg the app ships, made to read it as
that kind and nothing else, and copied stream by stream, its picture and
its sound untouched, without what the file said about itself, the
place, the date or the device. Its tile shows a still of its first frame
with its length in the corner, the way Photos and Finder show a video,
and the report holds the recording itself, under `recordings/`, see
`cmd/framefairy-app/recording.go`. Tim's first recording was turned
away as not a PNG or JPEG picture. A file that is not a
picture or a recording, or is too big, or would make an eleventh, is not attached and a
toast in the window's top right corner says which and why, "notes.txt
cannot be attached because it is not a picture or a screen recording", see
[Toasts](#toasts). It used to be red text in the row of the buttons,
which wrapped over three lines beside Report, Tim found. They show under the card in a
grid of three, four once the window is wide, every tile the same size
on whole pixels and 16 by 10, the picture fitted in. A bin shows on a
tile under the pointer, or on Delete with the tile focused, and the
tile then says Removed with **Put it back** for a moment before the
grid closes over it, the look the clip list uses, `Removed.svelte`.
The bin is the quiet trash can of every list, with no box of its own,
and a shade falls from its corner so it reads on any picture, while the
tile's edge lifts. On a solid grey square it looked pasted on, Tim
found.

What is written and attached is a draft, kept in the app's folder
under `report/` as it is written, so closing the window, or the app,
loses nothing. Report lets go of it, and the next report starts empty.
A recording goes with the window: closing it while somebody talks drops
the recording, and what was written down by then stays in the draft as
text. This is on purpose and not the way Stop ends one, which the first
design had. Left running, the next window took it up again, and every
word already said came back grey into the text the draft had kept, Tim
found. While a report is being made the field takes no typing, and a
picture dropped or pasted then goes into the next report.
See [LOGGING.md](LOGGING.md), `cmd/framefairy-app/report.go` and
`cmd/framefairy-app/bugs.go`.

**Dictate**, at the left of the same row, is for saying what happened
rather than typing it, plan row 2.189, the way dictation on claude.ai
works. Pressed, it says **Stop** and wears the beam while the app
listens, and the words appear where the cursor was while they are said,
grey and slanted until they are kept. Nothing is written down until the
microphone has been as loud as a voice, 10 dB over a quiet room, since
the speech model hears words in a room where nobody speaks: Tim pressed
Dictate and Stop a few times without a word and was given "yeah yeah".
The field follows the words the
way a terminal follows its output: they stay in view as they come, until
the person scrolls away to read, and then the field stays where they put
it until they scroll back to the words. The cursor stands right after
the words while nobody has put it elsewhere, drawn there by the end of
the words themselves, since WebKit drew it at the front of the last line
while the words ran on. The microphone in the button grows and shrinks with the voice it
hears, so the person sees they are heard. The Go side tells its
loudness ten times a second, and the window follows it every frame the
way a level meter's needle does, up at once and back down in about a
third of a second. It grows by a scale from a drawing at its largest
size, so it stays sharp and in its place, and it stays white. The
beam stays the sign that it listens, because running work shows the beam
on the control it was started from. What follows the last pause is still
being heard and changes as more of it is, so it is plain text in a mark
that cannot be typed in. A click in it, or just after it, settles it
there and then, as it is shown, so the cursor goes where it was put and
a word can be corrected while the person talks on, and what is heard
next follows it. The Go side is told with `SettleListening`. What is
typed right at the end of what was said settles it the same way and
becomes part of it, so what is said next goes after the typing, not in
front of it, and pushes the cursor along again once more is heard.
Shift+Enter there goes in the same way, as text, so what is said next
starts on the new line, issue #373. WebKit typed a second Shift+Enter
after the words being heard, and the kept words with new lines in them
became blocks of their own, which showed as an empty line at the end.
New lines are text in the field, wherever they come from. After every
key the field keeps the cursor in view, the least scroll that shows it,
inside its padding, the way a text field does. The browser only does
that for a cursor it moved and draws itself, and while dictating the
field puts a new line in by hand and draws the cursor, so Shift+Enter
past the foot of the field and the arrow keys left the cursor out of
view until more was said, issue #573. Typing anywhere else, wherever the arrow keys or a click
put the cursor, is plain typing, and what is said next still goes at
the end of what was said. What was said is one piece of the field that
is never split, so Stop, Enter and Escape always take all of it. It was
split once for each time somebody typed at its end, and on Tim's Mac
WebKit then typed the letters backwards, and Enter lost the pieces once
they were out of order. Nothing in the field is moved while the browser
types: typing at the end is called off and typed again into what was
said. What
came before a pause of 0.6 s is settled: it is text in the field, still
grey, and can be corrected while the person talks on. Settled words are
put in by hand rather than as typing, so Cmd+Z and Cmd+Y while talking
undo and redo only what was typed. **Stop** or Enter keeps all of it,
the last piece a moment later, written in as one step Cmd+Z takes back
and Cmd+Shift+Z brings back again. The cursor stays where the person
has it while the last piece is written in: Tim pressed Enter at once
after ending a dictation, for a new line, and the cursor was put back in
front of it, issue #373. Until then the cursor stays drawn after the
words, where it was, and the field's own cursor takes over only where a
key or a click puts it: WebKit drew its own at the front of the line or
past the words, and it jumped back once they came. Escape keeps it too, the way it ends
Dictation on a Mac. It removed everything said once, as on claude.ai,
and Tim lost a long report to an Escape meant for the app, with nothing
Cmd+Z could bring back, issue #355. It also
stops by itself, keeping what was said, after 2 minutes without a word
or once a recording is 10 minutes long, and says why under the field
where the problem would be. Stop pressed while it does that gets every
word all the same. Two short notes going up sound once the microphone is
open, and the same two going down when it stops, however it stops, so a
person talking while they look at the workspace hears that they are
heard and that it is over. macOS Dictation, Siri and the Google
Assistant do the same, so the pair needs no learning. They are two soft
sine notes a fifth apart, E5 and B5, made in the window with Web Audio
rather than played from a file, see `frontend/src/lib/chime.ts`, plan
row 2.244. The field
is a `contenteditable` of plain text rather than a textarea, because a
textarea cannot show some of its words grey.
The app records, not the window, through malgo, see
`cmd/framefairy-app/listen.go`: WKWebView on macOS would ask for the
microphone in a box of its own on every launch, naming the page as
wails://wails. The app gets the one prompt from macOS, with the words of
`NSMicrophoneUsageDescription`, and macOS remembers the answer. A Mac
that said no hands the app nothing but zeros, and after 2 s of them the
page says where to allow it, while it goes on recording, since the
prompt may still be open. One copy of the speech model loads while the
person talks, with the threads of one of the copies a transcription
uses, so a transcription beside it keeps the rest, see `asr.OpenOne`.
About every 0.4 s it writes down again what followed the last pause,
`engine.Settled` says where that is, and a stretch with no pause is
settled before it is 12 s long, so words do not change far behind the
person. It is given the 5 s before that pause
too, and everything after it, and the words are told apart by when they
start, so a pause in the middle of a sentence gives it no full stop and
the words after it no capital. Each `dictation` event carries every
settled piece, so one the window misses costs nothing. The log says how
long the recording was, how many words came of it and how long the
speech model took, never what they were, see [LOGGING.md](LOGGING.md).

## Where things are kept

- **Models:** in `~/.framefairy/models`. A speech model is a folder, a
  language model is one `.gguf` file. Neither is part of the app, so the
  app fetches them: the speech model by itself on the first run, a language
  model when somebody picks one. See [PACKAGING.md](PACKAGING.md).
- **The API keys:** in the macOS keychain, one item for each company,
  **Frame Fairy: Anthropic API key** and **Frame Fairy: OpenAI API key**,
  under the account `framefairy`. Never in a file. They are kept through
  the Security framework, so a key never passes through a command line,
  where any other program could see it while the command ran, and the
  item's access list names Frame Fairy: another program asking for the key
  gets a box from macOS first. Whether a key is there is asked without
  reading it, so opening the settings never asks for anything, and saving
  one never asks either, because an app may always write an item of its
  own. Until the app is signed with a Developer ID, every new build is a
  new app to the keychain, so a key saved by one build and read by a later
  one asks once whether the later one may use it, which **Always Allow**
  answers for that build. A key saved and used by the same build never
  asks. The first keys were kept with the `security` command, under
  `anthropic-api-key` and `openai-api-key`, where anything could read them.
  A key found there is moved into the app's own item the first time it is
  used, and the old item removed. `ANTHROPIC_API_KEY` or `OPENAI_API_KEY`
  in the environment stands in when no key is saved. The command line
  reads the environment first, which is how it is given a key, and the app
  reads the saved key first: an app started from a terminal has the
  terminal's environment, and an old key in it would otherwise win over
  the one just saved, without a word.
- **Settings and the episode list:** plain JSON files in
  `~/Library/Application Support/FrameFairy` on macOS, `%AppData%\FrameFairy` on
  Windows and `~/.config/FrameFairy` on Linux.
- **Everything about an episode:** next to it in `<episode>.framefairy/`, the
  same folder the command line uses. See [CLI.md](CLI.md#run).
- **Word corrections and removed words:** in each clip's plan entry,
  `corrections` and `removed_words`. They change what that clip's
  captions show, never the stored transcript or another clip.
- **Training records:** in `<episode>.framefairy/training/`. The app writes them
  and shows nothing about them. See [TRAINING.md](TRAINING.md).
- **The app's own log:** `~/Library/Logs/Frame Fairy/app.log` on macOS,
  where the Console app shows it, and `Frame Fairy/logs/app.log` in the
  user's cache folder elsewhere. It says enough that a fault is found by
  reading it, not by trying again. Each line is one JSON object with its
  moment, to the millisecond, its level, the run of the app, the video
  it is about where there is one, and where it came from, see
  [LOGGING.md](LOGGING.md):
  - `app`: each start, with its build.
  - `wails`: what Wails, which runs the app's window, says from info up.
  - `jobs`: each search, render and install starting and ending, with
    how it ended and how long it took.
  - `window`: the window's warnings and errors and anything thrown and
    not caught, the window going hidden and shown, the space bar with
    where the keyboard was and which workspace could hear it, where a swipe or a pinch on the clip
    timeline took the view, calls to the Go side that failed, took
    longer than half a second or came more than 60 in a second, which
    workspace is asked for and in front, each step of a workspace
    opening and what it still waits for when it is slow, and each step
    of the video preview: the canvas, what it plays, the sound card,
    the first frame, every play and pause with how it went, and what a
    play that does not start waits for.
  - `files`: every read of the video's file, every stream of frames or
    sound opened and closed, the first pull of each, a slow one and one
    that ended it, and the episode's decoder held, let go and closed.
    Every other pull is a trace line.
  - `decoder`: the episode's decoder starting and stopping, what it
    found in the file, whether it decodes on the graphics chip, every
    place it was moved to and how long its first frame took, the chain
    a frame goes through and ffmpeg's errors.

  Every click and every key that is not typing is an act, a line of its
  own with an id that every line it sets off carries, see
  [LOGGING.md](LOGGING.md#acts).
  The window sends its lines a quarter of a second after it says them,
  so they can stand a little after the Go side's lines of the same
  moment. Every line from debug up is always written. Trace lines, one
  for every pull of frames, are kept in memory, the last 8000, and a
  report of a problem carries them. The log is at most 10 MB, with
  three older files compressed beside it. It is the log plan row R.8 asks
  for, and a report of a problem will carry it. See
  `cmd/framefairy-app/applog.go` and `frontend/src/lib/said.ts`.

The app only shows files that belong to an episode in its list. It finds
Homebrew's ffmpeg and llama-server even when it is started from Finder.

## How the app is built

| Part | Where | What |
| --- | --- | --- |
| Go side | `cmd/framefairy-app/` | the app's window, the calls the interface makes, the job queue, settings |
| Interface | `frontend/` | Svelte 5 and TypeScript, no Go |
| Built interface | `cmd/framefairy-app/dist/app/` | made by make from `frontend/`, embedded into the program, not in git |

The Go side uses Wails v3, pinned to v3.0.0-beta.23. The interface calls the
methods of the `FrameFairy` type by name, through the typed wrappers in
`frontend/src/lib/api.ts`. The type is declared in
`cmd/framefairy-app/main.go`, and its methods sit in files by what they
are for, named like their tests: `library.go`, `media.go`, `views.go`,
`words.go`, `edits.go`, `chosen.go`, `training.go`, `settings.go`,
`setup.go`, `jobs.go`, `search.go` and `updates.go`.

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
| `frontend/src/screens/` | the workspace, settings, updates and acknowledgements |
| `frontend/src/components/` | player, timelines, clip list, work in hand |
| `frontend/src/lib/` | calls into Go and the shared state |
| `frontend/src/app.css` | colours, sizes and the base styles |

## Tests

```
make test
```

They cover the job queue, waiting for the transcript and the frames route.
The one that transcribes is skipped when ffmpeg is missing.
