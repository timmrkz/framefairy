# How a search asks the model

What the model is told when it looks for clips, why each part is there,
and what each costs. This is the page to argue about: every prompt the
experiments use is a file or a few lines of Go, named below, and a change
to one is a change to a few lines in a pull request that can be commented
on line by line.

The numbers are from Tim's comparison of 2 October 2026: the first 30
minutes of an episode in German, six clips of 20 to 30 seconds, Gemma 4
26B A4B on an M2 Max, which wrote 67 tokens a second and read about
590 a second.

## Where a search spends its time

| Part | Seconds | What it is |
| --- | ---: | --- |
| Reading the request | 26 | 15,303 tokens, 38,824 characters |
| Thinking | about 30 | 2,048 tokens, the budget for half an hour |
| Writing the answer | about 7 | six clips, 1,416 characters |
| Asking again, `lines` only | 7 | clips well off the length, and the framing they wait for |

So reading and thinking are most of it, and the answer is a tenth.

## What the model reads today

`lines`, the app's way of asking, sends two messages.

- **A system part**, 2,675 characters: the brief, what makes a moment
  work (the heart, the payoff, the opening), what to leave out, the
  pauses, and the answer's shape, in `engine/select.go`.
- **A user part**: the task, how to read the transcript, the
  transcript, then "That is the whole transcript." and the task again.

Every line of the transcript carries its number, its length, the pause
before it and how loud it was said:

```
[312] 1.7s (pause 1.1s) Eingangsflur. Mein Vater ist
[313] 3.0s (pause 0.8s) als Künstler nach Deutschland gekommen und hat Stuckarbeiten.
[314] 4.6s (pause 0.8s, quieter) In Schlössern und so gemacht und Fresten restauriert.
```

Of the 38,824 characters, the spoken words are 23,169. The line
numbers are 7 % of the request, the lengths 6 % and the brackets 17 %,
of which the word "pause" alone is 7 % and "louder" or "quieter" 2.4 %.
Digits cost more tokens than words, so the share in tokens is higher
still.

## Why it is like that, and what of it holds up

- **The system part.** Chat models are trained with a role for standing
  instructions, and the cloud providers recommend it. A model on this
  machine reads both parts as one text all the same: earlier Gemma
  versions had no system role at all and folded it into the first
  message. Nothing measured says the split helps. The new prompts are one
  message.
- **The task twice.** A long text with the question at its end is
  answered better than one with the question first, and "say it again at
  the end" is the cheap way to get that. The new prompts put the
  transcript first and the task after it, once, so the task is still the
  last thing read and costs half. It has a second gain for the app: a
  window searched again starts with the same transcript, which
  llama-server then has in its cache, and only the task is read anew.
- **Each line's length.** It was there so the model could count the
  seconds of a clip. It cannot: it gave clips of 10 s and of 60 s. A
  recipe that leaves the length to the engine has no use for it. One that
  needs the model to judge length is better served by the time a line
  starts at, which makes a length one subtraction.
- **Loud and quiet.** Nothing ever showed it helps a story, and it may
  pull the model towards loud passages. The new prompts leave it out.
- **The pause before a line.** It shows where a thought ends, and that
  may help find where a story begins. The new prompts keep it as "…"
  before a line, for a pause of a second or more, 215 of 475 lines here,
  without the number.
- **Cuts inside a clip.** All 24 clips of the first comparison were one
  piece. The model is told how to cut and does not. `points` asks for one
  stretch and leaves the pauses to the engine's own rule.
- **The answer.** 1,416 characters for six clips: the reason 41 %, the
  title 14 %, the slug 12 %, the line numbers 9 % and the rest JSON. The
  slug is made from the title by the program now, as for a clip made by
  hand, and the reason is asked for in at most twelve words. Both new
  prompts ask for about half the characters. The reason is shown in the
  app, so it stays. A plain line instead of JSON would save the rest, see
  the open questions.

## The ways of asking being compared

| Recipe | Mechanism | Prompt | Instructions | Transcript |
| --- | --- | --- | ---: | ---: |
| `lines` | the model keeps the length, clips well off it are asked for again | `engine/select.go`, `buildPrompt` in `engine/plan.go` | 3,305 | 35,396 |
| `heart` | the model names the heart, the engine fits the length around it | `engine/heart.go` | 3,396 | 35,396 |
| `heart-opening` | as `heart`, and the model names the opening, which no clip starts after | `engine/heart.go` | 3,670 | 35,396 |
| `heart-lean` | as `heart-opening`, in one short message, the transcript bare | [`engine/prompts/heart-lean.txt`](../engine/prompts/heart-lean.txt) | 1,452 | 26,815 |
| `points` | three lines, start, payoff and end, the transcript with the time each line starts | [`engine/prompts/points.txt`](../engine/prompts/points.txt) | 1,260 | 29,500 |

Instructions and transcript in characters, for the window above. The
transcripts look like this:

```
heart-lean
[312] Eingangsflur. Mein Vater ist
[313] als Künstler nach Deutschland gekommen und hat Stuckarbeiten.
[318] … Spiegel

points
[312 18:48] Eingangsflur. Mein Vater ist
[313 18:50] als Künstler nach Deutschland gekommen und hat Stuckarbeiten.
[318 19:10] … Spiegel
```

What each pair tells:

- `heart-opening` against `heart-lean`: the same three things asked for,
  told in 3,670 characters with an annotated transcript or in 1,452 with
  a bare one. Whether the brief and the annotations earn the time they
  take to read.
- `heart-lean` against `points`: runs and a heart with the length left
  to the engine, against three lines and the length judged by the model
  from the times.
- each at 1,024 and at 512 tokens of thought: how much thinking a lean
  prompt still needs.

Every side of a comparison starts its own llama-server, so none reads
from the cache of the one before.

## Open questions

- Does the model need the brief's word for what makes a moment work, or
  is "a small, complete story a stranger can follow" enough?
- Should the long pause stay, and at a second or at more?
- A plain line per clip instead of JSON, `12 18 19 | title | reason`,
  would halve the answer again. The model is held to JSON while it
  writes, and the same can be done for a line with a grammar, but the
  reading of it is new code that has to be as safe as the JSON's.
- Thinking: Gemma thinks before it answers unless told not to. Is a
  short, fixed budget enough once the task is this plain?
