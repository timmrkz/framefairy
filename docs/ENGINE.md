# How the engine works

The engine in `engine/` does all the work. The command line and the app are
two front ends for it and call the same code. This page explains the ideas
behind it. [CLI.md](CLI.md) and [APP.md](APP.md) explain how to use them.

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

Transcribing a stretch that does not start at the beginning, after an
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

Captions are made of the same words. A caption is a run of up to 38
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
inside that stretch, which is exactly how they were spoken. It is the same
rule an edited srt file gets, further down. Corrections stay one entry per
recognised word, so writing one word again undoes it.

Each search is saved under the stretch it was made over, `clips-<from>-<to>.json`,
so passes add up instead of overwriting each other. The model is only ever
shown the stretch it is asked about and knows nothing of earlier passes, so
two passes over the same material would come back with the same moments. The
app therefore lets a window be drawn only where nobody has looked yet, which
`SearchedWindows` and `FreeWindows` work out from the plans on disk. A plan
carries the window it was made over in `planned_with`, and the stretches of
it that were given back again in `planned_with.removed`, so a search is a
window with holes in it. `RemoveRange` makes a hole: the clips inside the
stretch go, their caption files are moved aside, and a plan with nothing
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

The colour is set with `--highlight-colour` or with `"highlight_colour"` in
`caption_style` in `clips.json`, as `#RRGGBB`. `--no-highlight`, or
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
held still, so removing a pause never makes the picture jump. The crop
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

A run reports how fast the model read and wrote, for instance
`read 38,210 tok at 850 tok/s`. Loading a 14 GB model takes a while, so when
you try several runs in a row, start the server once yourself and point the
tool at it:

```
llama-server -m ~/.framefairy/models/gemma-4-26B_q4_0-it.gguf -c 65536 -ngl 999 --port 8080
framefairy episode.mp4 --llm-url http://127.0.0.1:8080
```

The server's output is kept in `logs/llm-server.log`.

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
  plan.go       building the plan
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
                the window stops hearing about altogether
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
