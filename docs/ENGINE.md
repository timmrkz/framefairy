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

CoreML is half as fast here too, needs 5 s before the first piece and
changes up to 20 words in three minutes, so it stays out even with pieces
of one length. Hearing two pieces in one pass gains 3 % at most and changes
words, so it stays out as well. The thread count does not change a single
word.

**Several copies hear side by side.** One copy gains little from more
threads, 38 times real time with 4 and 46 with 8, so most of a big machine
sat idle while it heard. The app now loads several copies of the model,
`asr.Pool`, and each hears the next piece as it finishes one. The pieces
are cut exactly as before, and what they hear is taken in the order it was
said: a piece that is done early waits for the ones before it, so the
transcript and every save of it reach only as far as all of it has been
heard. The end of a part, a pause and the end of the audio each wait for
every piece still being heard. Measured on the same M2 Max, over
ten minutes of an episode, `make speechbench`:

| copies | threads each | real time | an hour takes |
| -----: | -----------: | --------: | ------------: |
|      1 |            8 |     47.3x |          76 s |
|      2 |            4 |     70.1x |          51 s |
|      3 |            3 |     76.2x |          47 s |
|      4 |            2 |     82.6x |          44 s |
|      4 |            3 |     76.5x |          47 s |

The same 1206 words every way. How many copies, with how many threads, is
decided by the machine the app runs on, `asr.Mix`: the threads in all are
its performance cores, where macOS tells them apart, or every core where
the system does not. With 6 or more they go two to a copy, with fewer one
to a copy, which was fastest on the 3 cores of an M1 and the 4 of a cloud
machine too. Every copy holds the model once more, about 0.9 GB, so there
is a copy for every 8 GB of memory, and four at most, the most measured.
Where memory allows fewer copies, each takes more of the threads. On the
M2 Max with 32 GB that is four copies of 2 threads, an hour in about 44 s
where it took 76.

A smaller model was tried against it: NeMo's multilingual FastConformer
transducer, in the same library, with German among its ten languages and
about a fifth of Parakeet's size. On 4 cores it heard twice as fast, 23
times real time where Parakeet heard 11, but it heard "Alles hat ein Ende,
nur die Wurst hat zwei" as "Alice had an end. No divorce hath thy", where
Parakeet wrote it down right. A podcast in German needs the German heard,
so Parakeet stays. Of the other models the library carries, Parakeet v2
hears English only, and Canary and Whisper were not tried: they are a
different kind of model, which writes its answer after hearing rather than
as it hears, and whether they give the word times the captions need, and
how fast they are here, is still to be measured.

### Heard in parts, where it is needed first

The transcript is one file for the whole episode, `logs/words.json` with
its loudness in `logs/words.frames`, and it holds the parts of the episode
heard so far, `parts`, which need not start at the beginning or meet. A
search needs its window heard and a clip made by hand the minute or so
around the playhead, so `LoadTranscript` is asked for a window and hears
only what of it is not heard yet, wherever that is. A playhead near the end
of a four hour episode no longer waits for the hours before it.

A part is read the way the loudness is, `audioFrom`: ffmpeg seeks before
the input to the frame, decodes a fifth of a second early and throws that
away. It lands on the sample: read from 4 s on, a tone gated five times a
second reads the same as the whole episode at the same moment, frame for
frame, but for a frame or two a dB apart on an edge of the tone, where a
shift of 10 ms differs on every edge, `TestAPartOfAudioLinesUpWithTheWholeEpisode`.
Before the parts, carrying on counted samples from the start instead, and
the reason given was that a seek lands on the packet and differs by up to
23 ms between builds. With the seek before the input, ffmpeg trims what it
decoded before the point, and the measurement says it lands.

Each part is read with 3 s of audio on either side, `hearingPad`, so every
word in it is heard whole: audio cut inside a word is heard as another
word. Where two parts meet, `addHeard` keeps each word once. A word belongs
to the part it starts in, and a word of the new part that starts within a
tenth of a second of the neighbour's word across the join is that same word
heard twice, and left out. Two hearings of one part at once each add only
what the other has not, under the file's lock. `TestPartsMeetWithEveryWordOnce`
cuts its parts inside words on purpose and gets every word once.

A transcript written before version 3 ran from the start without a gap and
is read as that one part, up to where carrying on would have started.

## The loudness of the whole episode

The waveform is the loudness every 10 ms, and the transcription measures it
as it hears. But the transcription hears only what a search or a clip made
by hand needs, so for the rest of the episode there was no waveform until
one reached it. `engine/levels.go` measures it on its own:
`MeasureLevels` decodes the audio and takes the same readings from the same
16 kHz samples, with no speech model, into `logs/levels.frames` and
`logs/levels.json`. Audio decodes at a few hundred times real time, so an
hour takes seconds, but four hours still take the better part of a
minute, and a playhead put near the end waited for all of it. So
`MeasureLevels` is told what the clip timeline shows and measures that
first, then on from there, then from the start. Every half second it asks
again, and when the view has moved to a part not measured yet, it stops
ffmpeg and starts it again there, with `-ss` before the input so ffmpeg
seeks rather than decodes its way there. It starts a fifth of a second
early and throws that away, `levelsLead`: the first 40 ms after a seek
came out up to 4 dB off, because a packet of compressed audio is decoded
together with the one before it. A run ends where it meets a part
measured already, so nothing is measured twice.

What it has is written every half second, frames first and the json
after, which gives the parts measured as frame numbers and, once a run
has reached the end of the audio, how many frames it has. So `ReadLevels`
never hands out a reading the json does not vouch for, and the waveform
grows as it runs. Levels of a file that has changed since are none, and a
measuring cut off carries on with the parts it has. Version 1 of the file
ran from the start with no gaps and is measured again. `Levels.Over` lays
these over the transcript's readings, so the waveform has whatever either
has measured. They are the same numbers where both exist.

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

`words.json` carries a version. Version 2 keeps a number apart from the
word before it, where version 1 glued it on, for example "am15." for
"am 15.". A version 1 file is still read as it is, numbers and all, since
an update has to read what an older version wrote. Only a new
transcription splits the numbers.

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

## Recipes

A recipe is one way of asking the model for clips, in `engine/recipe.go`:
what it is told, how the transcript is written out for it, what shape its
answer takes, and how that answer is read back. A recipe may number what
it likes, lines or sentences or paragraphs, as long as each thing it
numbers is a run of whole lines. Its answer is turned into runs of lines,
and everything after that is the same for every recipe: the words and
their times, the cuts, the framing and the captions. So the precision of
the captions never depends on what the model was shown, and the model can
be asked in terms of the story while the engine keeps the milliseconds.

| Recipe | What the model reads | What it answers |
| --- | --- | --- |
| `lines` | every line of speech numbered, with its length, the pause before it and its level, see below, and a brief for any video: the heart of a story and its payoff are never cut, the clip starts on the least a stranger needs and ends on the payoff, the length comes after that, at most N clips, and the task said again after the transcript | at most N clips, as runs of lines |
| `stories2` | the `lines` brief with the transcript as `stories` writes it | at most N clips, as runs of sentences |
| `stories-edit` | `stories`, then in the same conversation the clips as cut, measured | the same clips again, with the edges moved where the opening or the landing is wrong, the thinking split half and half between the two asks |
| `stories` | a brief for any video, the transcript as sentences in paragraphs, a time at the start of each paragraph, three dots for a pause of a second or more, and the length asked for in words at the speaker's own rate | up to N clips, the strongest first, as runs of sentences |

`lines` is what the app uses. The others are tried with `--recipe` and
compared with `--compare`, see [CLI.md](CLI.md#trying-other-ways-of-asking).
A sentence in `stories` ends where a line ends one, or once it has run
30 s, and never at a pause alone, since a sentence cut at a pause was a
place for a clip to end mid-sentence. A pause of a second or more inside
a sentence shows as three dots. A paragraph ends before a pause of 1.5 s or
once it has run 45 s. Sentence numbers say nothing of time, and the first
version of `stories`, which gave only the paragraph times, ran 8 of 10
clips far past 30 s. The second says the length in words too, from the
words and seconds of the window: at 2.2 words a second, 20 to 30 s is
about 43 to 65 words. It left sentences out for the first time, and gave
every sentence a run of its own, so every pause between two sentences was
cut and 8 of 12 clips came out short. `stories` leaves the pauses to the
engine, so since the third version runs that follow each other are one
run, `Recipe.Joins`, and the brief says a new run starts only where
something is left out. In `lines` two runs that meet still cut the pause
between them, because there the pauses are the model's.

**Tried and taken out: who speaks.** A `dialogue` recipe once told the
voices apart with two small models through the speech library, pyannote's
segmentation and NVIDIA's TitaNet, and showed the model a paragraph for
every turn with the speaker's letter. On Tim's episode, at the same
thinking, it cost 22 s more per 30 minutes (75 against 53) and not one
clip opened differently: the model began the stories where the guest
began telling them either way, and never on the question, though told a
story often starts there. It is in the history of pull request 19.

**Every edge of a clip lands on a sentence**, `engine/edges.go`. Five
searches cut the same story with three different first words and four
different last ones, most of them mid-sentence: the line the model
stopped on ended on a comma and the sentence went on over three more. So
the start, the end and every cut inside a clip move to the nearer place a
sentence begins or ends, when that is at most 8 s away, `sentenceReach`.
Further than that the model's edge stands, since a transcript can go a
while without a full stop. Runs that overlap once they are whole
sentences are one. This is done before a clip is measured, so the length
check sees the clip as it will be cut. An edge moves to the nearer
boundary unless that takes the clip past the longest length asked for,
and then it moves the other way: the model stopped the umbrella story on
the comma of the sentence after its payoff, the nearer boundary ran it to
35 s, and the length limit then cut the payoff out of its middle. Now it
ends on the payoff. A filler word at the start of a
clip goes, unless the line after it goes on in lower case: then it is the
first word of the sentence, "Und" before "irgendein Typ auf dem Schulhof",
and without it the clip would start mid-sentence.

Whatever the recipe, the model sometimes gives one moment twice, a line
apart. A clip that shares more than half the lines of the shorter of the
two with a clip before it is left out and said in the log, and the clips
after it move up, so a slot is never spent on the same moment.

**Clips are fitted to the length by measuring them.** Three runs of the
same prompt gave clips of 10 s on one and of 60 s on another. A model
cannot tell time from line or sentence numbers, and the engine knows it
to the millisecond. So with a local model, a clip under 90 % of the
minimum or over 120 % of the maximum is held back as the answer arrives,
the bounds the log has always flagged. Once the answer is in, the model is
asked once more, in the same conversation, `engine/fit.go`: how long each
held clip runs, how far off it is, and how long each line or sentence six
either side of it lasts. It gives those clips again, shortened by leaving
out what lies between the opening and the payoff, or lengthened with what
belongs to the moment. The model stays loaded for 30 s after its answer,
`fitKeep`, so the second ask shares the first one's start and llama-server
reads only the answer and the new question. The log says how many tokens
of the prompt were new. It answers without thinking: with a thousand
tokens of thought the second ask took 20 to 26 s, most of it thought, for
a question the numbers in it already answer.

Whichever of the two is nearer the length becomes the clip, so a clip is
never lost, and one that ran into another clip keeps its first form. An
answer counts for a clip only when it carries the clip's slug, or stands
in its place under a slug no other clip has, for a model that renamed it.
Asked about the mirror story, Gemma once gave the umbrella story back,
the first clip of its answer. The engine took it for the mirrors by its
place, found it was the umbrella story again, and kept the mirrors at
43 s without a word. Now a clip the answer leaves out, or gives another
clip in place of, is named in the log and asked for once more, alone,
with the request ending in `Give only "…", no other clip.` so the model
does not read the same question twice. After that it stays as it was.
Answering without thinking, the model sometimes gives a clip back
unchanged. Such a clip stays whole, flagged in the log, for a hand to trim
in the app. An automatic cut was tried and taken out again: the engine
cannot tell where the heart of a story is, and on Tim's episode it cut
the setup off the mirror story when it cut from the start, and the payoff
out of the umbrella story, twice, when it cut from the middle. A complete
story of 38 s is worth more than one of 19 s without its core.

A recipe with `Edit`, `stories-edit`, holds back every clip, not only
those off the length, and the second ask is about the edit: where each
clip opens and where it lands, what it leaves out, and its length. The
first ask thinks half the budget and the second the other half, so it
thinks no longer in all. An edit is taken unless it runs further off the
length. The clips appear once the second answer is in, not one by one. The
answer to the second ask is saved in the reply file as `fit`, and the one
to asking once more as `refit`, so a search that reuses the reply is
fitted the same way without asking. The second
ask is not recorded for training. What the user does with the fitted clip
is.

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
longer begins or ends a caption is left unused.

Thumbnails are kept per clip in `thumbnails`, the moments of the episode a
picture of the short is taken at. `readThumbnails` in `engine/clips.go`
keeps only numbers inside a kept piece, each once, in time order, at most
50, and `SetThumbnail` in `engine/edit.go` adds, moves and removes one.
`WriteThumbnails` in `engine/render.go` takes each picture from the short
that was just rendered, on the short's own clock through `ClipTime`. See
[THUMBNAILS.md](THUMBNAILS.md).

A caption is a run of up to 38
characters, ending early at a pause or at a sentence end once it has some
substance. It appears when its first word is spoken and stays until the next
caption appears, or until shortly after its last word when a pause follows.
Moving words onto a clip's timeline is plain arithmetic, so nothing is
estimated. A clip says a word, and captions it, when it holds more than a
frame of the word's sound, `HoldsWord` in `engine/lines.go`. So dragging an
edge back over a word brings its caption in at the word's last sound, the
first of it the edge reaches, and a word an edge cuts into is shown from
the clip's first frame. A word a cut parts is captioned once, in the piece
that holds the most of it. It used to take the whole word inside the clip
for a caption and the word's middle for the words a clip keeps, and a long
word stayed uncaptioned for half its length while it was plainly heard.

A word keeps its own time at the clip's first and last edge, even where
that lies outside the clip, and is clamped only at a cut. The halves of a
hyphenated word share its time by their letters, so clamped to what an
edge left of it they moved with the edge, and "liebe" was lit while
"grundschul" was still being said. `WordStops` in `engine/episode.go`
splits words the same way for the clip timeline, so the halves are where
an edge dragged with shift stops.

The recogniser's word timings are moved onto the sound when a transcript
is read, `SnapWords` in `engine/audio.go`. A word ends where its last
sound does, when 120 ms or more of silence follow it before the
recogniser's end. A silence with more of the word after it is not the
end: the recogniser hears a compound, or words said as one, as one word,
and "sweet-grundschulliebe" used to be cut off at the breath before
"liebe", which then had no caption and was never lit. The raw timings are
what is saved, so a transcript read again is snapped by the rule of the
day.

A corrected word may hold more than one word. The recogniser sometimes hears
one word where two were said, so a correction like "Und da" for "Und" turns
into two words that share the span the recogniser measured, split by how
long they are. The audio is not read again for this. The recogniser timed
the sound as a whole, and the highlight only has to run over the words
inside that part, which is exactly how they were spoken. It is the same
rule an edited srt file gets, further down. Corrections stay one entry per
recognised word, so writing one word again undoes it.

Each search is saved under the window it was made over, `clips-<from>-<to>.json`,
so passes add up instead of overwriting each other. A window searched again
is the next pass over it, `clips-<from>-<to>-<pass>.json`, with ids that
begin `t<from>-<pass>-`, and the passes before it stay as they are. A search
carried on keeps the pass it began, which its record holds. The model is
only ever shown the window it is asked about, so every search is told which
lines are in clips already, from any pass and from the clips made by hand,
the removed ones too, and asked for other moments, see version 4 in
[TRAINING.md](TRAINING.md). A clip that keeps more than half its lines from
those all the same is left out and said in the log, the same measure as a
moment given twice. A pass that brings nothing new writes a plan with no
clips, so it still counts as a pass. `SearchPasses` cuts the episode into
parts by how many searches have read them, and the app's New goes where the
fewest have been, earliest first. A plan
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

A word too wide for a line on its own gets a caption of its own, so it is
read as two lines of one caption rather than as a third line under the
words around it. It is hyphenated the way TeX hyphenates: Liang's
algorithm, from `github.com/speedata/hyphenation`, over the hyph-utf8
patterns that TeX, LibreOffice and Firefox use, which are in
`engine/hyphenation/`. Like TeX and every word processor it takes the last
break that still fits, so each line is as full as it can be.

German also knows where the parts of a compound join. The Trennmuster
team, who make the German patterns, keep a word list of half a million
words with every joint marked, and their own build learns patterns from it
that break a word only at the joints of the highest rank, which their
documentation names for ragged text. A caption is ragged text, so a joint
that fits comes first: "Suchmaschinenoptimierung" at size 96 in Inter
Black becomes "Suchmaschinen-" and "optimierung", not "Suchmaschinenopti-"
and "mierung". Over the list the joint patterns find 99.7% of the joints
and put 0.1% in the wrong place. A joint never costs a line: where the
only joint that fits would leave a word on three lines that two syllable
breaks fit in two, the two lines win. Those patterns are
`hyph-de-1996-x-major`, MIT like the list.

Each piece gets its share of the time the word was spoken in, so the
highlight runs over both, and a correction made on either piece corrects
the whole word.

Patterns are per language, and the episode's language is written nowhere,
so it is read off the clip's own words by `github.com/abadojack/whatlanggo`,
a port of the franc and whatlang detectors. Which languages ship is decided
by the files in `engine/hyphenation/` and nothing else: a language is found
by the name of its file. None of them is written by hand, `make
hyphenation` writes the folder again from the commits
`scripts/hyphenation.sh` pins. They are the languages the speech model hears
whose patterns may go into a paid app: Bulgarian, Croatian, Danish, Dutch,
English, Estonian, French, German, Greek, Hungarian, Italian, Lithuanian,
Polish, Portuguese, Russian, Slovak, Slovenian, Spanish, Swedish and
Ukrainian. Czech is under the GPL alone, Latvian under the LGPL or GPL, and
Romanian has no licence. Finnish says only "Patterns may be freely
distributed", which grants no more than passing the file on and may mean
free of charge, so two readings found it unclear for a paid app. A word in
any of those four is broken where the line ends.

The notices for the patterns come from `make notices` like every other
notice. It reads the metadata hyph-utf8 writes at the top of each file,
takes the first licence the app may ship of those the file is offered
under, MIT before BSD before the LPPL and the MPL, and fetches the full
text from the SPDX licence list where the file only names it. A file that
writes out terms of its own, which no tool can recognise, counts only once
two readings, one of them independent, found those exact terms allow a
paid app, kept by their SHA-256 in
`notices/gen`, so a changed text is refused until it is read again. A file
offered only under the GPL or the LGPL, or under nothing, stops the
notices, and has to come out of `engine/hyphenation/`.

The size is never changed: it used to come down for the whole clip until
the widest word fitted, and one long word made every caption of the short
a third smaller.

A face the program does not carry cannot be measured, and then `wrap_chars`
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

## How much one search can read

A search sends the transcript of its window to the model in one request, and
nothing is ever split into several behind anybody's back. How much one
request holds is a number of tokens, set by the model, so the longest window
is worked out from the model that is going to read it, in `engine/room.go`:

- **A local model** holds what its file says, the `context_length` in the
  GGUF header, and never more than the 262,144 the engine starts
  llama-server with. The four models on offer hold 262,144, except Qwen3 14B
  at 40,960. A model file the engine does not know is taken at 32,768. Off
  that comes the room for the answer, 16,384 tokens at most, with the
  thinking written inside it, and 2,048 more. A token is taken as two and a
  half characters of German, the same rate the context is sized by.
- **A local model also has to fit in the machine's memory**, context and
  all, by the same rule the models are offered by: a context is fine when
  the model still leaves 8 GB of headroom, and a model offered as tight,
  because it only fits without the headroom, gets the 65,536 tokens it was
  judged at and no more. A machine that does not say how much memory it
  has gets 65,536 too. llama-server is never started with a larger context
  than this, whatever the rounding up to a power of two would ask for.
  On a 32 GB Mac, Gemma 4 26B is held by its own context and Ministral 3
  8B by memory, at a little over three hours. On a 16 GB Mac, Gemma 4 12B
  and Ministral 3 8B read about 1.3 hours.
- **Through the API** a model holds its context window, less twice the
  answer ceiling, because an answer that thinks through the whole ceiling
  is asked again with twice as much and that has to fit too. What `--budget`
  buys can be less: the budget, less what the usual answer costs, pays for
  so many tokens read. A token is 1.9 characters, or what earlier searches
  on this episode measured.

What the instructions and the ask take comes off the top, and what is left
is the room in characters of numbered transcript. The engine weighs every
line of the transcript in the same characters, counted with the widest line
number a window could give it, so a window that the app adds up as fitting
is never one the engine refuses. Past the end of the transcript a second
weighs 25 characters, or 15 percent more than the episode's own average
once there are ten minutes of it, whichever is more.

In practice Gemma 4 and Ministral 3 read six hours of German at once, the
API reads any episode with Sonnet, seven hours with Opus and not quite three
with Fable at the default budget of $2, and Qwen3 14B reads about 35
minutes.

The window also has to hold the clips asked for: the count at the shortest
length, one after another. A run that asks for more is refused before it
transcribes.

## Planning on your machine

The tool starts `llama-server` with the model, waits until the model has
loaded, sends the transcript and stops the server again. The model's context
is sized to the transcript, about 64,000 tokens for an hour, which keeps
memory use down, and it is never larger than the model holds. The answer is held to the plan's JSON format while it is
written, so it always parses.

The answer is read as it is written, from llama-server and from the API
alike, in `engine/stream.go`. Each clip is taken the moment its closing
brace arrives, by a scanner that only reads objects directly inside the
list named `clips`, so a brace in a title or a sentence before the answer is
read past. What it hands out still goes through every check a whole answer
does. An API answer that breaks off before a word of it arrived is asked
for again. One that breaks off after is not, because what arrived has
already been used.

**The API is two companies, Anthropic and OpenAI,** in `engine/provider.go`.
The model asked for says which: a `claude-` model is Anthropic's, a `gpt-`
model OpenAI's, and a model nobody has heard of is taken to be Anthropic's,
which is what every model was before there was a choice. Each company has
its own address, written in the code and never derived from any input, its
own key, read from `ANTHROPIC_API_KEY` or `OPENAI_API_KEY` and then the
keychain, where `engine/keys.go` keeps it in an item only the app may
read, through the Security framework on the Mac, never through a command
line, and its own shape of request: Anthropic's Messages with the key in
`x-api-key` and an optional prefilled brace, OpenAI's Chat Completions with
the key as a bearer token, the instructions as a developer message, and
`max_completion_tokens` for the ceiling, which counts the thinking as well.
OpenAI's answer, streamed or plain, is put into the shape of Anthropic's as
it is read, so everything after reading, the retries, the cost, the clips
taken as they arrive, a ceiling spent thinking and given more room, and the
repair of a broken answer, is one path whoever answered. OpenAI keeps its
thinking to itself and only counts it, so a reply that thought and never
answered reads as a thinking block with nothing in it, the way Anthropic's
does. The app offers one model of each by name, Claude Sonnet 5 and
GPT-6 Sol, two of the same price, so the choice is of company rather
than budget. Any other model either company has can be named with `--model`.
The tests stand a fake server in for both, in `engine/provider_test.go`, down
to a whole search on a GPT model from transcript to plan.

The command line reads the environment first and the keychain after. The
app turns that round with `PreferSavedKeys` as it starts, so the key saved
in its settings is the one used, and a key left in a terminal's
environment never wins over it. `KeyHint` gives a key in short, its first
twelve characters and its last four, kept as the keychain item's comment
so it is read without the key. `VerifyAPIKey` shows a key to its company
before the app keeps it, by asking for the list of models, and a key
refused during a search is said in words, naming where it came from. The
company's own answer is kept in the `logs` folder, as every refused request
is, and in the detail lines of `--verbose`.

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
replaced by the first search that finishes. A model in the cloud listed
in `engine/api.go` is measured against `measuredCloud`, a guess rather than
a measurement, replaced the same way. One written in by hand says what it
is doing without saying how far it is until it has been timed. Claude
Sonnet 5, Opus 5 and Fable 5.1 think by themselves, and are sent
`thinking: {type: "adaptive", display: "summarized"}` so that their thought
arrives as it goes. It costs nothing more, and without it the stream is
silent until the answer begins, so the search could not tell thinking from
hanging. There is no thinking budget in the cloud: these models refuse
one, and `effort` is the only lever, left at its default. A
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
transcript 30. So the request carries `reasoning_budget_tokens`, 2,048 for
a half hour window, which is about 45 seconds: when it runs out,
llama-server closes the thought with a line telling the model to write its
answer, and it does. **The budget follows the window**: weighing a
transcript is work that grows with how much of it there is, not with how
many clips are picked from it, so a window gets the same share of 2,048
tokens as it is of half an hour, never less than 512 and never more than
4,096. A 10 minute window thinks about 680 tokens, some 15 seconds, and
its first clip comes about half a minute sooner. `--think` gives a number
instead, `-1` is no limit and `0` is no thinking. How many clips a search
looks for follows the window too, one for every twelve clip lengths
of a half hour and in proportion to the square root of other windows,
unless `--count` says. Both are in `engine/suggest.go`, with
their cases in `frontend/src/lib/suggest.cases.json`, which the workspace
is tested against too.

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

## Jobs and their records

A search, the way the app runs one, is `Project.Search` in `jobs.go`: it
hears the episode from where the transcript ends to the end of its window,
and then plans, the same two steps `Run` takes for the command line. A
render is `Project.RenderJob`, one clip at a time. Each keeps one record in
the episode's `jobs/` folder, written atomically as it goes from one step to
the next, and before each step it asks the app for its turn in that step's
lane. Done, the record goes and the job's timings are added to
`jobs/timings.jsonl`. Failed, the record says why. Stopped from outside,
the record stays as it was, which is how an app that starts knows the job
was cut off. What each step made stays however the job ends: the
transcript as far as it was heard, the clips as they landed, the shorts
that were finished. The command line does not keep records. The design is
in [JOBS.md](JOBS.md).

A clip made by hand is `Project.MakeClip` in `handclip.go`, a job of its
own kind that shares everything but what makes it one. It hears through
`hear`, the step a search takes, pulled out of `Search`, as far as the
clip can reach: `Longest` and a sentence past the playhead for I, a
sentence past it for O. The playhead proposes the clip, `handEntry`, from
the sentence the playhead stands in, grown a line at a time to
`Shortest`, and hands it to the plan builder's `propose`, the intake the
model's scanner uses too. The builder shapes, frames and writes it into
`clips-hand.json`, a set that grows, `PlanOptions.Grows`, whose clips each
take the next number under the set's lock as they are written. The set
says it was made by hand, `planned_with.by`, and `madeOver` reads that as
made over no part of the episode, so it marks nothing searched and giving
a part back leaves it alone.

Every job that makes clips says which it has on the way, `Log.Underway`,
the whole list each time it changes: the builder from the moment a clip is
queued to be framed until it is written or let go, and a clip made by hand
from the moment it is asked for, at the playhead. The app puts the list on
the job, and a record gives it back after a restart.

## The code

`asr/` wraps the speech recogniser and is the only package with native code.
Everything else is in `engine/`:

```
  audio.go      reading the audio, word timing, loudness
  transcript.go the transcript cache and the speech model location
  lines.go      lines, cuts and captions, all built from words
  highlight.go  word timings for captions and the bouncing highlight
  recipe.go     ways of asking for clips, and reading the answer back
                into lines. recipe_stories.go is the stories recipe
  compare.go    one window searched with several recipes, and the report
  fit.go        clips well off the length asked for again, measured
  edges.go      every clip edge on a sentence
  select.go     prompt, reply parsing and plan validation
  local.go      planning with llama.cpp on this machine
  stream.go     answers read as they are written, and each clip taken
                the moment it is whole
  plan.go       building the plan: the prompt, the call, the whole answer
  planbuild.go  clips framed and written as they are proposed
  handclip.go   clips made by hand with I and O
  searchclock.go how far a search has come, against how long it took before
  jobs.go       a search and a render as one job each, with their records
                and timings
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
  api.go        the APIs in the cloud, costs and usage
  provider.go   the companies in the cloud, Anthropic and OpenAI, their
                addresses, keys and the models the app offers by name
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
  levels.go     the loudness of the whole episode, measured on its own in
                seconds, which is the waveform before the transcription
  edit.go       plan edits that keep the file as it was written: keep or
                reject, trimming, correcting words, the caption look, moving
                the crop and the caption line by hand and back, and letting
                go of a whole plan
  frames.go     still frames, and deleting everything made for an episode
  corrections.go word corrections for captions
  training.go   training records of plans and decisions
```
