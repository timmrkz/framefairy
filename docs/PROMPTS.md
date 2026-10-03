# How a search asks the model

What the model is told when it looks for clips, why each part is there,
what each costs, and what is still to decide. Every prompt is a file in
`engine/prompts/`, exactly as the model gets it, so a change to a prompt
is a change to a few lines in a pull request that can be commented on.

The numbers are from Tim's comparison of 2 October 2026: the first 30
minutes of an episode in German, six clips of 20 to 30 seconds, Gemma 4
26B A4B on an M2 Max, which wrote 67 tokens a second and read about 590
a second.

## The prompt files

| File | Used by | Answer |
| --- | --- | --- |
| [`lines.txt`](../engine/prompts/lines.txt) | the app, today | JSON |
| [`heart.txt`](../engine/prompts/heart.txt) | experiment | JSON |
| [`heart-opening.txt`](../engine/prompts/heart-opening.txt) | experiment | JSON |
| [`heart-lean.txt`](../engine/prompts/heart-lean.txt) | experiment | JSON |
| [`points.txt`](../engine/prompts/points.txt) | experiment | one line a clip, no JSON |
| [`middle.txt`](../engine/prompts/middle.txt) | experiment | three numbers a story, no JSON |

A file is the message the model is sent, as it is. The program fills in
what is between `{{` and `}}`: the transcript, `{{.Count}}` clips,
`{{.Min}}` to `{{.Max}}` seconds, the lines that are clips already and
what the video is about. Only the three files that came from the Go code,
`lines`, `heart` and `heart-opening`, send a system part too. It comes
first, after `=== system ===`, and the message after `=== user ===`.
They send word for word what the Go code sent before them. The `stories`
recipes from pull request 19 are still in Go and take no part in this.

What each file sends, filled in for a transcript of five lines, is in
[`engine/testdata/prompts`](../engine/testdata/prompts): as it is, with
a context and a line that is a clip already, `-with-context`, and the
files that take switches with `+pause` and with `+times`. A test
holds every file to them, so a change to a prompt shows there too, as
the model will read it. `go test ./engine -run
TestEveryPromptAsksWhatTestdataSays -update-prompts` writes them anew.

## The simplest way of asking: `middle`

Tim's way of asking, as little as can be said. The transcript, and after
it three sentences:

```
Find the 6 best stories in this transcript.
For each story, first pick a line somewhere in its middle, then the line where the story starts and the line where it ends. A story runs about 44 to 66 words.
Answer with one line per story, the middle, the start and the end, and nothing else, like this: 40 31 52
```

Nothing tells the model what a short is, what a video is, or what a
heart, a payoff or an opening is. Those words carry
meanings of their own, and the model may follow the word rather than the
story. It is asked for the story and its shape, nothing more.

Why the middle first. The model writes its answer one number after the
other, and each number is written knowing the ones before it. Pointing
at a story first is the easy part. The start and the end are then
written knowing which story they belong to, and only have to be found to
the left and to the right of it.

What the program does with it. It never starts a story later than the
model's start, and never ends it before the line in its middle. A story
longer than the length is ended earlier, on a whole sentence, decided by
Tim after the run of 3 October, where a story with no length to it ran
up to 2.5 minutes. One shorter than the length takes in the sentences
before it. The model names no title, so a clip is named after its first
words.

## What the run of 3 October showed

Tim's episode with Ben, the first 30 minutes, six clips of 20 to 30
seconds, Gemma 4 26B A4B on his M2 Max. Model is the seconds the model
took for every ask of the side. The thought is worked out from what the
plain sides wrote, which the report could not read yet.

| Side | Model, s | Read, tokens | Clips off the length | Clip lengths, s |
| --- | ---: | ---: | ---: | --- |
| `lines` | 94, three asks | 15,235 | 1 | 19 to 36 |
| `heart-opening` | 63 | 15,338 | 1 | 28 to 40 |
| `heart-lean` | 56 | 8,616 | 5 | 25 to 50 |
| `points` | 51 | 8,551 | 3 | 20 to 51 |
| `middle` | 42 | 8,407 | 5 | 25 to 154 |
| `middle+pause` | 40 | 8,624 | 4 | 28 to 133 |
| `middle+words` | 40 | 8,420 | 3 | 22 to 56 |
| `middle@1024` | 26 | 8,407 | 5 | 25 to 154 |

- **Reading** halves with the lean prompts, 26 s to 12 s.
- **Thinking** is most of what is left. Every side thought to its budget,
  about 2,048 tokens or 30 s. `middle@1024` thought half as long and
  named the same first three stories as `middle`.
- **Writing** is a few seconds. `middle` writes about 80 tokens, the
  JSON sides about 400.
- **`middle` finds where a story starts.** It opened the umbrella story
  on "Aber aus der Grundschule habe ich zum Beispiel eine Erinnerung", the
  line the story really begins on, which no other side did. But told
  nothing of length, a story is a whole topic to it: 50 s to 2.5 min.
- **`+words` brings `middle` close to the length**, 22 to 56 s, and
  changes which stories it picks. Its starts are worse: two begin in the
  middle of a thought.
- **`+pause` changed little.**
- **`heart-opening` read best**: every story with its setup, the payoff
  kept, 28 to 40 s. It is also the slowest of the sides that ask once.
- **`points` ignores the language** it is told to title in, and wrote
  English titles for a German episode.

## What the second run of 3 October showed

The same window, with the length said in words in the lean prompts, and
`middle+times` to try seconds against it. What the model named, before
the program fitted it, against what it was asked for:

| Side | Asked | Within it | What the model named |
| --- | --- | ---: | --- |
| `middle` | 44 to 66 words | 4 of 6 | 51 to 136 words |
| `middle@1024` | 44 to 66 words | 2 of 6 | 30 to 129 words |
| `points` | 44 to 66 words | 1 of 6 | 25 to 124 words |
| `middle+times` | 20 to 30 seconds | 0 of 6 | 44 to 83 seconds |

| Side | Model, s | Clips off the length |
| --- | ---: | ---: |
| `heart-opening` | 63 | 1 |
| `heart-lean` | 45 | 0 |
| `points` | 46 | 2 |
| `middle` | 46 | 0 |
| `middle+times` | 46 | 1 |
| `middle@1024` | 26 | 0 |

- **Words, not seconds.** Told the seconds and given the time of every
  line, the model named no story within the length. Told the words, it
  named 4 of 6, and the program fitted the rest.
- **The lean prompts now keep the length.** `heart-lean` and `middle`
  had no clip off it, where the run before had 5 each.
- **But they lose the setup.** The umbrella story starts on "Und
  irgendein Typ auf dem Schulhof gemobbt" in `heart-lean`, `middle` and
  `middle@1024`, without the judo and the second grade before it, and
  the orchid story starts on "Aber für diesen Markt gab, ne?".
  `heart-opening` keeps the setup of every story, and found the insights
  the lean prompts missed: "Sind unsere Erinnerungen echt?", "Warum ich
  für andere kämpfe", "Die Sehnsucht nach Einzigartigkeit".
- **Half the thinking, about the same picks.** `middle@1024` took 26 s
  where `middle` took 46, named the same umbrella and orchid stories,
  and kept every clip within the length.

## Where a search spends its time

| Part | Seconds | What it is |
| --- | ---: | --- |
| Reading the request | 26 | 15,303 tokens, 38,824 characters |
| Thinking | about 30 | 2,048 tokens, the budget for half an hour |
| Writing the answer | about 7 | six clips, 1,416 characters |
| Asking again, `lines` only | 7 | clips well off the length, and the framing they wait for |

Reading and thinking are most of it. The answer is a tenth.

## The question everything else follows from: who decides the length?

A short has to run 20 to 30 seconds. Either the model chooses a stretch
that long, or it chooses the story and the program cuts it to length.
How the transcript looks, what the prompt says and what the answer
holds all follow from which.

What is known:

- **Each line's length, `lines`.** The model was shown how long every
  line runs and asked for clips of 20 to 30 seconds. It cannot add up
  fifteen numbers: the same prompt gave clips of 10 s and of 60 s. Two
  of six were still off the length after asking it again.
- **A time every 45 seconds, `stories`.** The first version of `stories`
  gave a time at the start of each paragraph. 8 of 10 clips ran far past
  30 s.
- **No counting at all, `heart`.** The model named the heart and the
  story around it, and the program cut it to length. Thinking, all 12
  clips were within the length. Without thinking, one of 6 ran 55 s,
  because the model named the whole story as its heart. And it started
  stories later than `lines`, by its own choice.
- **A time on every line.** Never tried. A clip's length would be one
  subtraction, the time of its last line less the time of its first,
  where `lines` asked for fifteen additions. That is the only reason to
  try it again, against the two results above.

So the evidence of the first run said: the program decides the length,
and the model is not told about seconds at all. The run of 3 October
showed what that costs. Told nothing of the length, `heart-lean`,
`points` and `middle` named whole topics, 5, 3 and 5 of 6 clips off the
length, `middle` up to 351 words. Told the length in words, Tim's idea,
`middle` named 4 of 6 stories within the words asked for. `lines`, told
each line's seconds, had 4 of 6 within the length on its first answer
too, from a transcript twice the size. So the lean prompts now say the
length in words, from how fast the speaker talks in the window. A time
on every line, `+times`, says it in seconds instead, and is the switch
left to try against it.

What the program can do with the length depends on what the model
names. With an opening it never starts after and a payoff it never cuts,
a story whose stretch from opening to payoff runs 45 seconds has nothing
the program may take away. The model hardly ever leaves anything out
inside a clip: all 24 clips of the first comparison were one piece.
**Decided by Tim: such a story stays whole.** If it is a good story, the
person cuts in the app what they do not want in it, and the app makes
that easy. Neither is the length a fixed 30 seconds: it is a setting in
the app, so how far a story runs over depends on it too.

## The switches

`heart-lean`, `points` and `middle` show the model the numbered words
of each line and the length of a clip in words, unless a comparison
switches more on after a `+` in a side's name, as in
`middle+pause2@1024`:

| Switch | What the model gets |
| --- | --- |
| `+pause` | "…" before a line after a pause of 1 s or more |
| `+pause2` | the same after 2 s or more, any number of seconds after the word |
| `+times` | the minute and second each line starts at, and the length asked for in seconds rather than words |

A comparison starts a llama-server for every side, so none reads from
the cache of the one before, and its report sets the moments the sides
found side by side, one row a moment in the order they come.

## The request

- **One message, no system part.** Only the three files from the Go
  code have one. Chat models are trained with a role
  for standing instructions, and the cloud providers recommend it. A model
  on this machine reads both parts as one text all the same: earlier
  Gemma versions had no system role and folded it into the first message.
  Nothing measured says the split helps.
- **The transcript first, the task after it, once.** A question at the
  end of a long text is answered better than one at its start, which is
  why `lines` says the task again after the transcript. Said only after
  it, the task is still the last thing read and costs half. It also lets
  a window searched a second time start with the same transcript, which
  llama-server then has in its cache, so only the task is read anew.
- **What a good moment is, in one sentence.** `lines` explains it in
  about 1,900 characters: the heart, the payoff, the opening, what to
  leave out and in what order they matter. The lean prompts say "a small,
  complete story a stranger can follow, from its setup to its payoff",
  and what to prefer in two more. Whether that is enough is what
  `heart-opening` against `heart-lean` tests. Other sentences worth a side
  each:
  - "something that happened, told so a stranger gets it in half a minute"
  - "a moment someone would send to a friend"

## The transcript

Each line of the transcript is a phrase as it was said: a new line
begins at a pause of 0.45 s or more, or after a sentence of 2.5 s, or
before 14 s.

| Format | Example | Characters, Tim's window |
| --- | --- | ---: |
| `lines` today | `[314] 4.6s (pause 0.8s, quieter) In Schlössern und so gemacht` | 35,396 |
| number and words | `[314] In Schlössern und so gemacht` | 26,385 |
| and a pause mark | `[318] … Spiegel` | 26,815 |
| and the time | `[318 19:10] … Spiegel` | 29,500 |

The words alone are 23,169 characters. In today's format the numbers in
brackets are 17 % of the request and "louder" or "quieter" 2.4 %. Nothing
says loudness helps a story, and it may pull the model towards loud
passages, so the lean formats leave it out.

**The pause mark.** "…" before a line says the speaker stopped before
it. A pause is where one thought ends and the next begins, so it may help
the model see where a story starts. Marked at every pause it says
nothing, because almost every line follows one:

| Pause before the line | Lines of 475 |
| --- | ---: |
| 0.45 s or more | 430 |
| 1 s or more | 215 |
| 1.5 s or more | 108 |
| 2 s or more | 55 |
| 3 s or more | 26 |

It may also be one more label like louder and quieter, pulling attention
to the form of the speech where the story is not. So it is a switch, and
a comparison tries it with and without: `+pause` marks 1 s and more, half
the lines, `+pause2` marks 2 s and more, one line in nine. Without the
switch there is no mark.

## The answer

1,416 characters for six clips in `heart@0`: the reason 41 %, the title
14 %, the slug 12 %, the line numbers 9 %, and the rest JSON, its keys,
quotes and brackets. The lean prompts already leave out the slug, which
the program makes from the title, and ask for a reason of at most twelve
words.

Without JSON, one line a clip, as `points` answers:

```
14 17 18 | Der Regenschirm | Ein Kind wehrt sich mit Judo, und der Schirm zerbricht.
```

That is the line numbers, the title and the reason, and nothing else.
`middle` answers with less still, three numbers a story and no title:

```
40 31 52
```

Nothing holds the model to the line, unlike JSON, which a schema holds
it to. A grammar of our own was tried first, and `points` never finished
in Tim's run of 3 October. llama-server applies such a grammar from the
model's first token, where for a schema it builds one that lets Gemma
think first. So Gemma could not open its thought, most likely wrote its
thinking into the title, which the grammar let run without a line break,
and no clip ever came.

The program reads the clips a line at a time as they arrive, each into
the clip a JSON answer gives, so everything after the reading is the
same. A line that is not a clip is passed over, so a sentence before the
answer costs nothing, and the clips past the count asked for are left
out. The reader is fuzzed, `FuzzPlainAnswer`. `points` and `middle`
always answer this way, and `lines`, `heart`, `heart-opening` and
`heart-lean` in JSON: the runs of `heart-lean` would need a notation of
their own on a line.

## Thinking

The budget follows the window, 2,048 tokens for half an hour and in
proportion for others, at least 512 and at most 4,096, `SuggestedThink`
in `engine/suggest.go`. That rule stays. What can change is the 2,048:
a comparison side written `middle@1024` thinks a fixed 1,024, and once a
plainer prompt does as well with less, the rule's number comes down.
