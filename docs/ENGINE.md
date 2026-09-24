# How the engine works

The engine in `engine/` does all the work. The command line and the app are
two front ends for it and call the same code. This page explains the ideas
behind it. [CLI.md](CLI.md) and [APP.md](APP.md) explain how to use them.

## Hearing the audio in pieces

The speech model is given the audio in pieces of 15 to 30 s, and the
engine chooses where to cut them, `engine/audio.go`. The model takes any
length it is given, so the length is ours to choose, for two measured
reasons. Its memory grows faster than the length: on top of the 0.9 GB the
model takes, a piece of 15 s takes 0.09 GB more, 60 s 0.4 GB, 4 minutes
1.9 GB and 6 minutes 4.2 GB, while it hears 10 times faster than real time
at 15 s and 4 times at 6 minutes. And a cut inside a word makes the model
hear that word as another word, "Bank" as "Bahn" and "Geruch" as
"Großmutter". So a piece ends at the middle of the quietest 300 ms
between 15 and 30 s. There is no threshold: some moment is always the
quietest, so a cut is never impossible.

That rule was measured against the alternatives over a whole recorded talk
of 18 minutes, 3219 words, scored against the talk's own transcript, clean
and with something under it the whole time:

| | clean | music | street noise | a second voice |
|---|---|---|---|---|
| a cut every 30 s wherever it lands | 123 wrong | 134 | 175 | 723 |
| the quietest 300 ms, the rule | 123 | 112 | 140 | 696 |
| every 30 s, keeping only the words that end 2 s before the cut and starting the next piece before the first word left out | 122 | 137 | 160 | 705 |

Speech is louder than what lies under it, so the quietest moment is still
a gap between words when there is music or traffic, and that is where the
rule is better than cutting blind. Cutting by the model's own word timings
and hearing the seams twice did not beat it and costs 8 percent more
hearing. A second voice talking the whole time costs a fifth of the words
whatever the cut, because the model writes down both speakers. The width
of the quiet moment was measured the same way, over clean, music and
street: 100 ms 393 wrong in all, 200 ms 414, 300 ms 375, 400 ms 375,
600 ms 415. 300 and 400 are the same within what one talk can tell.

How fast it hears was measured on the macOS runner, an Apple M1 with 3
cores, over three minutes of a recorded talk, `scripts/speechbench`:

| | pieces of 15 s | 30 s | 60 s |
|---|---|---|---|
| on the processor, 3 threads | 13.5 times real time | 15.0 | 9.2 |
| on the processor, 6 threads | 5.6 | 6.5 | 6.5 |
| through CoreML, 3 threads | 7.1 | 0.4 | 0.3 |

Pieces of 15 to 30 s are where it is fastest. More threads than the
machine has cores make it more than twice as slow, so the app never asks
for more than the cores it has, and at most 8. Hearing two pieces in one
pass gains nothing that holds. CoreML is slower on the runner and changes
a few words: it compiles the model again for every new length of audio,
and the pieces the app cuts are all of different lengths.

On a real Mac, an M2 Max with 12 cores, over three minutes each of three
parts of one episode, `make speechbench`:

| | pieces of 15 s | 30 s | first piece |
|---|---|---|---|
| on the processor, 4 threads | 38 times real time | 37 | 0.9 s |
| on the processor, 8 threads | 46 | 45 | 0.9 s |
| through CoreML, 8 threads | 24 | 23 | 5.0 s |

The processor with 8 threads is what the app already does, and it is the
fastest: an hour of episode is heard in about 80 s. The thread count does
not change a single word. CoreML is half as fast here too, needs 5 s
before the first piece and changes up to 20 words in three minutes, so it
stays out even with pieces of one length. Hearing two pieces in one pass
gains 3 % at most and changes words, so it stays out as well.

One cut is placed exactly: the end of the window the first search waits
for. The app gives the transcription that point, `Engine.StopAt`, and the
piece that reaches it is cut there, so nothing past the window's edge is
heard. A word that runs across the edge is left out of what is saved, the
transcript says to carry on from before it, `resume` in `words.json`, and
the next pass hears it whole.

## How words get their timing

The recogniser gives every word a start and a duration, on an 80 ms grid. It
also folds pauses into the words around them, so a word after a pause tends
to start early and a word before one tends to end late.

The loudness measured every 10 ms corrects that. A word stamped inside a
real pause moves forward to where its sound starts. A word stamped just after
an onset moves back onto it. A word running into a pause ends where the sound
stopped. A single quiet frame at a word start is left alone, because many
words begin softly.

Measured on test audio with known word starts, a typical start ends up 6 ms
off and 9 in 10 are within 15 ms. Inside continuous speech there is no pause
to correct against, so the recogniser's own accuracy applies, which is within
50 ms for 9 words in 10 in the tests so far.

The recogniser's own timings are what gets stored in `words.json`. The
correction is applied on every load, so improving it never needs a new
transcription.

Transcribing a window that does not start at the beginning, after an
interrupted run carried on or with `--from`, decodes from the start of the
file and drops the samples before it, rather than asking ffmpeg to seek. A
seek into a compressed stream lands on a packet, and what a build does with
that packet differs: some decode and trim it, some begin at the next one, up
to 23 ms later for AAC. Raw samples carry no timestamps, so a start that is
out cannot be noticed afterwards, and every word from there on would be
wrong by that much. Counting samples is exact everywhere, and audio decodes
at a few hundred times real time, so winding forward through an hour costs
about ten seconds.

## Lines, cuts and captions

The model reads the transcript as numbered lines. A line ends at a pause of
0.45 s or more, at a sentence end once the line is 2.5 s long, or before it
would pass 14 s. Each line shows its talking time, the pause before it, and
whether it was said noticeably quieter or louder than usual:

```
[17] 1.0s (pause 1.1s) Ich habe immer eine tolle Idee.
[19] 7.1s (pause 0.9s, quieter) bastle ich daran, weil ich da Bock drauf habe.
```

The model answers with runs of lines to keep, `"keep": [[12, 18], [24, 27]]`.
Pauses inside a run stay, material between runs goes, and each cut keeps
`--keep-pause` of air without ever reaching into a word that was left out.
A leading or trailing line holding only hesitation ("äh", "und", "also") is
trimmed off.

What comes out of that is a proposal, not the last word. Every cut a clip
carries is a gap between two of its pieces, and the app can move one, put
one back or make one, through `CutClip`, `JoinCut` and `MoveCut` in
`engine/edit.go`. They go through `editPlan` like every other edit, so the
plan keeps its shape and its unknown fields, the clip's words are taken
again from the transcript, and the caption file goes so the next render
builds it afresh. A piece a cut is made inside becomes two, and both keep
the framing of the piece they came from, so cutting never moves the
picture. A cut swallows any word it touches and then leaves `--keep-pause`
of air on each side that stays, which is why a cut dragged over a pause
takes the whole pause and one dragged over speech takes whole words.

Captions are made of the same words. Where their timing is a little off
from what is heard, a caption is moved by hand: the plan keeps, per clip,
when the caption that begins on a word appears and when the one that ends
on a word goes, in `caption_times` keyed by the millisecond that word
starts in the episode. `moveCaptions` in `engine/lines.go` puts them there
when captions are built, never over the caption beside them and never
shorter than a tenth of a second, and a timing kept against a word that no
longer begins or ends a caption is left unused. A caption is a run of up to 38
characters, ending early at a pause or at a sentence end once it has some
substance. It appears when its first word is spoken and stays until the next
caption appears, or until shortly after its last word when a pause follows.
Moving words onto a clip's timeline is plain arithmetic, so nothing is
estimated.

A corrected word may hold more than one word. The recogniser sometimes hears
one word where two were said, so a correction like "Und da" for "Und" turns
into two words that share the span the recogniser measured, split by how
long they are. The audio is not read again for this. The recogniser timed
the sound as a whole, and the highlight only has to run over the words
inside that part, which is exactly how they were spoken. It is the same
rule an edited srt file gets, further down. Corrections stay one entry per
recognised word, so writing one word again undoes it.

Each search is saved under the window it was made over, `clips-<from>-<to>.json`,
so passes add up instead of overwriting each other. The model is only ever
shown the window it is asked about and knows nothing of earlier passes, so
two passes over the same material would come back with the same moments. The
app therefore lets a window be drawn only where nobody has looked yet, which
`SearchedWindows` and `FreeWindows` work out from the plans on disk. A plan
carries the window it was made over in `planned_with`, and the parts of
it that were given back again in `planned_with.removed`, so a search is a
window with holes in it. `RemoveRange` makes a hole: the clips inside the
part go, their caption files are moved aside, and a plan with nothing
left of its window goes altogether.

## The bouncing word

The word being spoken sits on a reddish purple pill and bounces: word and
pill grow for 80 ms and settle back over 100 ms, around the word's own
centre, while every other word stays exactly where it is. The word stays lit
until the next word starts.

To place the pill, the tool renders each caption once per word with only that
word visible, which gives the exact position of every word in the finished
line. That costs one extra short ffmpeg pass per clip. The active word is
drawn on its own layer over the line with that word hidden, which is why its
neighbours never move.

The colour is set with `"highlight_colour"` in `caption_style` in
`clips.json`, as `#RRGGBB`, or as `&HAABBGGRR` when the pill is given an
opacity, which is how the captions column of the app puts it. A pill that
lets the picture through is drawn with that alpha. `--highlight-colour` is the colour for a plan that has none. `--no-highlight`, or
`"highlight": 0` in `caption_style`, gives plain captions.

Captions sit 300 pixels above the bottom of a 1080x1920 frame, which keeps
them clear of the bar that Shorts and Reels park over the bottom fifth.
`--margin-v` moves them, for a run and for the plan it writes. A clip can
also carry its own place as `"caption_y"` in `clips.json`, in pixels from the
bottom of that frame, which beats the style. It lands on a step of 40 pixels,
between 120 and 1560. The app does not write it: it keeps one place for every
clip of every episode, and `ClearCaptionY` takes the hand-placed ones off a
plan so they all follow it.

## Captions that fit

A caption is broken into lines that fit inside the frame, which is the frame
less the side margins and the padding of the box. The width is measured from
the font file itself: the advance of every glyph, read out of the face the
program carries. Kerning and shaping are left out, and since kerning almost
always pulls letters closer, the measure comes out a shade wider than what
libass draws, so a line that fits here fits there.

When even a single word is too wide to break, the caption size comes down
for that clip until it fits, so the captions of one short stay one size. A
face the program does not carry cannot be measured, and then `wrap_chars`
decides the breaks, as it always did.

One catch worth knowing: the size in a caption style is not the em square.
libass takes it as the height of the face, its ascent plus its descent, the
way VSFilter did. Inter Black at size 96 is drawn at 79 pixels to the em,
Anton at 55. The app needs the same number to draw the captions in the
picture at the size they will be burned in at, so the engine hands it over
with the captions.

## The caption fonts

Three faces are built into the binary: Inter Black, Anton and Archivo Black.
Before a render the one in use is written into a `fonts` folder next to the
caption file, with its licence, and ffmpeg is pointed at that folder with
`fontsdir`. That folder name is relative, which also keeps every path out of
the filter graph, where colons and backslashes have their own meaning.

So a render needs nothing installed on the machine and a short looks the same
everywhere. `--font` accepts any other name too, and then it is up to the
machine to have it.

Every per-clip srt file has a `.words.json` file next to it with the timing
of each word. When you edit a caption, the words you left alone keep their
timing, and changed or added words share the time between their unchanged
neighbours. Without the words file, the words of a caption share its time
evenly.

## Clip length

The defaults ask for 20 to 30 seconds. A clip over 36 seconds or under 18
gets a warning naming it, so you can adjust it in `clips.json` and render
that clip alone.

The numbered transcript the model reads has one row per line, and the text
of a line is scrubbed before it goes in. A line break inside a word would
otherwise add a row that no line stands behind, and the model's line numbers
would mean something else than the transcript does.

A hand-edited `clips.json` is read as untrusted input. Times may be seconds
or timecodes, every piece has to move forwards, no piece may lie outside an
episode (a hundred hours is the outside), a plan holds at most a thousand
clips, two clips may not share a name, because every edit names the clip it
belongs to, and two clips may not resolve to the same file name. A plan that breaks one of those is refused
with the clip named, before anything is rendered.

Edits of one plan happen one at a time. Each of them reads the file, changes
it and writes it back, and the app has no save button, so an edit landing
while another is being written must not be the one that disappears.

A search is refused before it starts if its lengths cannot be met, a
shortest longer than the longest or no clips at all.

## Framing

Each camera angle in a clip gets one crop, measured across the whole shot and
held still, so removing a pause never makes the picture jump. Framing
decodes only what a clip keeps. Each kept span is searched for camera
switches on its own, and where the clip leaves the episode and comes back,
the frame it leaves on is compared with the frame it comes back to, which
answers whether it comes back to the same camera without decoding what was
cut out. The decoding is asked of the system's own video decoder,
VideoToolbox on macOS, and falls back to the processor by itself where
there is none, which today is every Windows and Linux build. If the system
decoder refuses a file, the same work is done again on the processor and
everything after it goes there too. Which decoder does the work is found
once per episode file, by decoding one frame the way framing does and
reading what ffmpeg says, and written to the log as `framing decodes
video on VideoToolbox` or `on the processor`. The crop
centres on the largest face found. Where too few frames contain a face, it
goes to the part of the frame with the most fine detail, which is whatever
the camera focused on. The built-in face detector looks for faces turned
roughly towards the camera.

## Planning on your machine

The tool starts `llama-server` with the model, waits until the model has
loaded, sends the transcript and stops the server again. The model's context
is sized to the transcript, about 64,000 tokens for an hour, which keeps
memory use down. The answer is held to the plan's JSON format while it is
written, so it always parses.

The answer is read as it is written, from llama-server and from the API
alike, in `engine/stream.go`. Each clip is taken the moment its closing
brace arrives, by a scanner that only reads objects directly inside the
list named `clips`, so a brace in a title or a sentence before the answer is
read past. What it hands out still goes through every check a whole answer
does. An API answer that breaks off before a word of it arrived is asked
for again. One that breaks off after is not, because what arrived has
already been used.

A search is three kinds of work that no longer wait on each other, in
`engine/planbuild.go`. The model writes on the graphics side of the
machine, framing decodes video with ffmpeg on the processor, and writing a
clip to the plan is a few kilobytes. So each clip is framed by one of
several framers while the model goes on writing, and written to the plan
the moment it is framed. There are as many framers as a third of the
cores, at least two and at most four: framing the last clips after the
answer took 25 seconds on an M2 Max with two. The first clip is framed on
its own, because it is the one a person waits for and every clip framed
beside it takes a share of the cores, the memory and the system decoder
from it. The others start once it has landed. While a search runs, its
clock owns the progress line, so what ffmpeg reports while it frames a
clip does not take the line from it. The first clip makes the plan, in place of whatever
plan was there for the window, and each after it goes in through
`editPlan`, so an edit the app makes to a clip that has already landed
is kept. A clip that lands in a part removed while it was on its way is
left out, and a plan removed altogether takes no more clips. The plan's
id is decided before the first clip lands, so a decision about a clip made
while the search runs is recorded against the plan it was made about.
Stopping a search keeps what it had written.

How far a search has come is measured, not guessed, in
`engine/searchclock.go`. A search is five parts one after the other:
loading the model, the model reading the transcript, the model thinking,
the model writing its clips, and the framing still going when it stops.
Every search that finishes keeps how long each part took, per model, in
`~/.framefairy/speed.json`: the seconds to load, the transcript characters
read per second, the seconds of thinking and the tokens thought a second,
the seconds per clip, and the seconds of framing after the answer. Each
new timing counts half, so one slow search on a busy machine moves the
next estimate without taking it over. The next search reports its share
and the time left against those, about twice a second. Inside a part,
what the model counts beats the clock: llama-server's count of the prompt
it has read, and the tokens it has thought against its budget. A local
model this machine has not timed yet is measured against a search timed on
an M2 Max with Gemma 4, which is close enough to say how far it is and is
replaced by the first search that finishes. The API has no stand-in, and
its first search says what it is doing without saying how far it is. A
search against a server that was already running loaded nothing, and
leaves the loading time as it was. A record from before the thinking was
timed on its own measured something else, and is replaced.

**The model is loaded before the search needs it.** Loading takes
llama-server about 24 seconds. The running server belongs to the
program rather than to one search, in `engine/modelhost.go`: a search
that finds the model loaded uses it, and one that finds it loading waits
for it. The app loads it for the first search of a new episode while the
transcript is still on its way to the end of the window, and a search
that is waiting for the transcript loads it while it waits. A model
loaded ahead waits five minutes for its search. A search lets go of it
the moment it is done and it stops, because a model left in memory
between searches that are days apart is memory taken from everything
else. One model is in memory at a time, never two, because two do not
fit: an ask that needs another model, or more room, waits for the one in
memory to be let go of and then takes its place. A warm-up gives way to a
model in use instead of waiting, and the search it was for loads the model
when its turn comes. The app stops the model when it closes, also one
that is still loading. A running server is written down in
`~/.framefairy/llama-server.json`, so one left behind by an app that
crashed is stopped the next time the app starts, if that process is still
exactly that server, with the same port and the same model. The server runs one ask at a time (`-np 1`) with
the whole context for it.

**The local model thinks on a budget.** Left to itself, Gemma 4 thinks
about a half hour window for 12,000 tokens or more before it writes a
word of the answer. On an M2 Max that is four of the five and a half
minutes a search takes, while loading is 24 seconds and reading the
transcript 30. So the request carries `reasoning_budget_tokens`, 2,048 by
default, which is about 45 seconds: when it runs out, llama-server closes
the thought with a line telling the model to write its answer, and it
does. `--think` changes it, `-1` is no limit and `0` is no thinking.

A run reports how fast the model read and wrote, for instance
`read 38,210 tok at 850 tok/s`. Loading a 14 GB model takes a while, so when
you try several runs in a row, start the server once yourself and point the
tool at it:

```
llama-server -m ~/.framefairy/models/gemma-4-26B_q4_0-it.gguf -c 65536 -ngl 999 --port 8080
framefairy episode.mp4 --llm-url http://127.0.0.1:8080
```

The server's output is kept in `logs/llm-server.log`, at log level 4,
which is the first level where llama.cpp says what it took from memory.

## The code

`asr/` wraps the speech recogniser and is the only package with native code.
Everything else is in `engine/`:

```
  audio.go      reading the audio, word timing, loudness
  transcript.go the transcript cache and the speech model location
  lines.go      lines, cuts and captions, all built from words
  highlight.go  word timings for captions and the bouncing highlight
  select.go     prompt, reply parsing and plan validation
  local.go      planning with llama.cpp on this machine
  stream.go     answers read as they are written, and each clip taken
                the moment it is whole
  plan.go       building the plan: the prompt, the call, the whole answer
  planbuild.go  clips framed and written as the answer arrives
  searchclock.go how far a search has come, against how long it took before
  undo.go       an edit remembered as the files before and after it, and
                put back clip by clip, so what landed since stays
  analysis.go   camera switches and framing
  faces.go      the built-in face detector
  clips.go      the plan file and crop geometry
  captions.go   srt reading and writing, time formats
  fonts.go      the caption faces built into the binary
  metrics.go    how wide a caption comes out, read from the font file
  ass.go        the burned-in caption track and its measured boxes
  render.go     filter graph and ffmpeg command per clip
  ffmpeg.go     running ffmpeg, preflight checks, probing
  api.go        the Anthropic API, costs and usage
  run.go        the run loop
  log.go        the timestamped terminal log
  events.go     the same log as structured events, for the app. Every
                event goes to the app through one door, and that door
                answers for what an event may carry: a share or a time
                that is not a number becomes Unknown, because JSON
                cannot say NaN and an event nobody can encode is a job
                the app stops hearing about altogether
  project.go    one episode driven step by step, as the app does it
  episode.go    status, waveform, silences and plan views for the app,
                and the note that an episode has been searched once
  edit.go       plan edits that keep the file as it was written: keep or
                reject, trimming, correcting words, the caption look, moving
                the crop and the caption line by hand and back, and letting
                go of a whole plan
  frames.go     still frames, and deleting everything made for an episode
  corrections.go word corrections for captions
  training.go   training records of plans and decisions
```
