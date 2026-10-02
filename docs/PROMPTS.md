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

| File | Used by | Parts |
| --- | --- | --- |
| [`lines.txt`](../engine/prompts/lines.txt) | the app, today | system part and request |
| [`heart.txt`](../engine/prompts/heart.txt) | experiment | system part and request |
| [`heart-opening.txt`](../engine/prompts/heart-opening.txt) | experiment | system part and request |
| [`heart-lean.txt`](../engine/prompts/heart-lean.txt) | experiment | one message |
| [`points.txt`](../engine/prompts/points.txt) | experiment | one message |

A file is what the model is sent, in order: a system part after
`=== system ===` where there is one, and the request after
`=== user ===`. The program fills in what is between `{{` and `}}`: the
transcript, `{{.Count}}` clips, `{{.Min}}` to `{{.Max}}` seconds, the
lines that are clips already and what the video is about. The three older
files send word for word what the Go code sent before them, which a test
holds them to. The `stories` recipes from pull request 19 are still in Go
and take no part in this.

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

So the evidence says: the program decides the length, and the model is
not told about seconds at all. `points` with times is the one side that
would test the untried way, and can be left out.

What the program can do with the length depends on what the model
names. With an opening it never starts after and a payoff it never cuts,
a story whose stretch from opening to payoff runs 45 seconds has nothing
the program may take away. The model hardly ever leaves anything out
inside a clip: all 24 clips of the first comparison were one piece. **So
the open decision is what happens to such a story**:

1. It stays whole and long, and is flagged for a hand to trim.
2. The program cuts setup after the opening anyway, and the start may be
   weaker.
3. It is left out, as not a short.
4. The model gets the times, and is told to choose stories that fit.

## The request

- **One message, no system part.** Chat models are trained with a role
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

The lean prompts mark 1 s and more, which is half the lines. 2 s marks
one line in nine, closer to where a thought really ends. Or no mark at
all, for the fewest characters.

## The answer

1,416 characters for six clips in `heart@0`: the reason 41 %, the title
14 %, the slug 12 %, the line numbers 9 %, and the rest JSON, its keys,
quotes and brackets. The lean prompts already leave out the slug, which
the program makes from the title, and ask for a reason of at most twelve
words.

Without JSON, one line a clip:

```
14 17 18 | Der Regenschirm | Ein Kind wehrt sich mit Judo, und der Schirm zerbricht.
```

That is the line numbers, the title and the reason, and nothing else.
llama-server holds the model to a shape while it writes, today to JSON,
and can hold it to this line just as well with a grammar. The program
then reads the clips line by line as they arrive, the way it reads the
JSON now, with a reader of its own that is tested against every answer a
model could write. Which numbers stand at the start of the line follows
from the decision above: an opening and a payoff, or start, payoff and
end.

## Thinking

The budget follows the window, 2,048 tokens for half an hour and in
proportion for others, at least 512 and at most 4,096, `SuggestedThink`
in `engine/suggest.go`. That rule stays. What can change is the 2,048:
a comparison side written `points@1024` thinks a fixed 1,024, and once a
plainer prompt does as well with less, the rule's number comes down.
